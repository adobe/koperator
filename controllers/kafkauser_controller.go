// Copyright © 2019 Cisco Systems, Inc. and/or its affiliates
// Copyright 2025 Adobe. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"emperror.dev/errors"

	apiutil "github.com/banzaicloud/koperator/api/util"

	"github.com/banzaicloud/k8s-objectmatcher/patch"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/go-logr/logr"
	certsigningreqv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlBuilder "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/banzaicloud/koperator/api/v1alpha1"
	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/errorfactory"
	"github.com/banzaicloud/koperator/pkg/k8sutil"
	"github.com/banzaicloud/koperator/pkg/pki"
	"github.com/banzaicloud/koperator/pkg/util"
	kafkautil "github.com/banzaicloud/koperator/pkg/util/kafka"
	pkicommon "github.com/banzaicloud/koperator/pkg/util/pki"
)

var userFinalizer = "finalizer.kafkausers.kafka.banzaicloud.io"

// SetupKafkaUserWithManager registers KafkaUser controller to the manager
func SetupKafkaUserWithManager(mgr ctrl.Manager, certSigningEnabled bool, certManagerEnabled bool) *ctrl.Builder {
	log := mgr.GetLogger()
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.KafkaUser{}).
		WithEventFilter(SkipClusterRegistryOwnedResourcePredicate{}).
		Named("KafkaUser")
	if certSigningEnabled {
		csrMapper := csrMapper{
			client: mgr.GetClient(),
			log:    log,
		}
		builder.Watches(
			&certsigningreqv1.CertificateSigningRequest{},
			handler.EnqueueRequestsFromMapFunc(csrMapper.mapToKafkaUser),
			ctrlBuilder.WithPredicates(certificateSigningRequestFilter(log)))
	}
	if certManagerEnabled {
		builder.Owns(&certv1.Certificate{})
	}
	return builder
}

func certificateSigningRequestFilter(log logr.Logger) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(createEvent event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			patchResult, err := patch.DefaultPatchMaker.Calculate(e.ObjectOld, e.ObjectNew)
			if err != nil {
				log.Error(err, "could not match objects", "kind", e.ObjectOld.GetObjectKind())
			} else if patchResult.IsEmpty() {
				return false
			}
			return true
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			return false
		},
	}
}

type csrMapper struct {
	client client.Reader
	log    logr.Logger
}

// mapToKafkaUser maps CertificateSigningRequest events to KafkaUser reconcile events
func (m *csrMapper) mapToKafkaUser(ctx context.Context, obj client.Object) []ctrl.Request {
	certSigningReqAnnotations := obj.GetAnnotations()
	kafkaUserResourceNamespacedName, ok := certSigningReqAnnotations[pkicommon.KafkaUserAnnotationName]
	if !ok {
		return []ctrl.Request{}
	}
	namespaceWithName := strings.Split(kafkaUserResourceNamespacedName, string(types.Separator))
	if len(namespaceWithName) != 2 {
		return []ctrl.Request{}
	}

	namespace := namespaceWithName[0]
	name := namespaceWithName[1]

	var kafkaUser v1alpha1.KafkaUser
	err := m.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &kafkaUser)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return []ctrl.Request{}
		}
		m.log.Error(err, "couldn't retrieve KafkaUser", "namespace", namespace, "name", name)
		return []ctrl.Request{}
	}

	// skip reconciling KafkaUser if owned by Cluster Registry
	if ok := util.ObjectManagedByClusterRegistry(&kafkaUser); ok {
		return []ctrl.Request{}
	}

	return []ctrl.Request{{
		NamespacedName: types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		},
	}}
}

// blank assignment to verify that KafkaUserReconciler implements reconcile.kafkaUserReconciler
var _ reconcile.Reconciler = &KafkaUserReconciler{}

// KafkaUserReconciler reconciles a KafkaUser object
type KafkaUserReconciler struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	Client client.Client
	// DirectClient reads from the apiserver. Use it only where a stale cache
	// could make a lifecycle or PKI cleanup decision from a missing object.
	DirectClient client.Reader
	Scheme       *runtime.Scheme
}

// +kubebuilder:rbac:groups=kafka.banzaicloud.io,resources=kafkausers,verbs=get;list;watch;create;update;patch;delete;deletecollection
// +kubebuilder:rbac:groups=kafka.banzaicloud.io,resources=kafkausers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kafka.banzaicloud.io,resources=kafkausers/finalizers,verbs=create;update;patch;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=issuers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=clusterissuers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests/approval,verbs=update
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=signers,verbs=approve

// Reconcile reads that state of the cluster for a KafkaUser object and makes changes based on the state read
// and what is in the KafkaUser.Spec
func (r *KafkaUserReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := logr.FromContextOrDiscard(ctx)
	reqLogger.Info("Reconciling KafkaUser")
	var err error

	// Fetch the KafkaUser instance
	instance := &v1alpha1.KafkaUser{}
	if err = r.Client.Get(ctx, request.NamespacedName, instance); err != nil {
		if apierrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			return reconciled()
		}
		// Error reading the object - requeue the request.
		return requeueWithError(reqLogger, err.Error(), err)
	}
	instance.SetGroupVersionKind(v1alpha1.GroupVersion.WithKind("KafkaUser"))

	// Get the referenced kafkacluster
	clusterNamespace := getClusterRefNamespace(instance.Namespace, instance.Spec.ClusterRef)
	var cluster *v1beta1.KafkaCluster
	if cluster, err = k8sutil.LookupKafkaCluster(ctx, r.Client, instance.Spec.ClusterRef.Name, clusterNamespace); err != nil {
		// This shouldn't trigger anymore, but leaving it here as a safetybelt
		if apierrors.IsNotFound(err) && k8sutil.IsMarkedForDeletion(instance.ObjectMeta) {
			reqLogger.Info("Cluster is gone already, there is nothing we can do")
			if err = r.removeFinalizer(ctx, instance); err != nil {
				return requeueWithError(reqLogger, "failed to remove finalizer from kafkauser", err)
			}
			return reconciled()
		}
		return requeueWithError(reqLogger, "failed to lookup referenced cluster", err)
	}

	if k8sutil.IsMarkedForDeletion(instance.ObjectMeta) {
		return r.finalizeUser(ctx, cluster, instance)
	}

	if instance, err = r.ensureClusterLabel(ctx, cluster, instance); err != nil {
		return requeueWithError(reqLogger, "failed to ensure kafkacluster label on user", err)
	}
	if instance, err = r.ensureUserFinalizer(ctx, instance); err != nil {
		return requeueWithError(reqLogger, "failed to update kafkauser with finalizer", err)
	}

	var kafkaUser string

	if instance.Spec.GetIfCertShouldBeCreated() {
		// Validate the KafkaUser instance annotations before creating a certificate request
		err := instance.Spec.ValidateAnnotations()
		if err != nil {
			return requeueWithError(reqLogger, "failed to reconcile kafkauser while validating annotations", err)
		}

		// Avoid panic if the user wants to create a kafka user but the cluster is in plaintext mode
		// TODO: refactor this and use webhook to validate if the cluster is eligible to create a kafka user
		if cluster.Spec.ListenersConfig.SSLSecrets == nil && instance.Spec.PKIBackendSpec == nil {
			return requeueWithError(reqLogger, "could not create kafka user since user specific PKI not configured", errors.New("failed to create kafka user"))
		}
		var backend v1beta1.PKIBackend
		if instance.Spec.PKIBackendSpec != nil {
			backend = v1beta1.PKIBackend(instance.Spec.PKIBackendSpec.PKIBackend)
		} else {
			backend = v1beta1.PKIBackendProvided
		}

		pkiManager := pki.GetPKIManager(r.Client, cluster, backend)

		user, err := pkiManager.ReconcileUserCertificate(ctx, instance, r.Scheme, cluster.Spec.GetKubernetesClusterDomain())
		if err != nil {
			switch {
			case errors.As(err, &errorfactory.ResourceNotReady{}):
				reqLogger.Info("generated secret not found, may not be ready")
				return ctrl.Result{
					RequeueAfter: time.Duration(5) * time.Second,
				}, nil
			case errors.As(err, &errorfactory.FatalReconcileError{}):
				// TODO: (tinyzimmer) - Sleep for longer for now to give user time to see the error
				// But really we should catch these kinds of issues in a pre-admission hook in a future PR
				// The user can fix while this is looping and it will pick it up next reconcile attempt
				reqLogger.Error(err, "Fatal error attempting to reconcile the user certificate.")
				return ctrl.Result{
					RequeueAfter: time.Duration(15) * time.Second,
				}, nil
			default:
				return requeueWithError(reqLogger, "failed to reconcile user secret", err)
			}
		}
		kafkaUser, err = user.GetDistinguishedName()
		if err != nil {
			reqLogger.Error(err, "could not get Distinguished Name from the generated TLS certificate", "cert", string(user.Certificate))
			return ctrl.Result{}, err
		}
	} else {
		kafkaUser = fmt.Sprintf("CN=%s", instance.Name)
	}

	// If topic grants supplied, grab a broker connection and set ACLs
	if len(instance.Spec.TopicGrants) > 0 {
		broker, close, err := newKafkaFromCluster(r.Client, cluster)
		if err != nil {
			return checkBrokerConnectionError(reqLogger, err)
		}
		defer close()
		// Record the identity once Kafka is reachable and before the first ACL is created.
		if instance.Status.ACLPrincipal == nil || *instance.Status.ACLPrincipal != kafkaUser {
			instance.Status.ACLPrincipal = &kafkaUser
			if instance, err = r.updateStatusAndFetchLatest(ctx, instance); err != nil {
				return requeueWithError(reqLogger, "failed to record ACL cleanup identity", err)
			}
		}

		// TODO (tinyzimmer): Should probably take this opportunity to see if we are removing any ACLs
		for _, grant := range instance.Spec.TopicGrants {
			reqLogger.Info(fmt.Sprintf("Ensuring %s ACLs for User: %s -> Topic: %s", grant.AccessType, kafkaUser, grant.TopicName))
			// CreateUserACLs returns no error if the ACLs already exist
			if err = broker.CreateUserACLs(grant.AccessType, grant.PatternType, kafkaUser, grant.TopicName); err != nil {
				return requeueWithError(reqLogger, "failed to ensure ACLs for kafkauser", err)
			}
		}
	}

	// set user status, keeping the recorded ACL cleanup identity
	instance.Status.State = v1alpha1.UserStateCreated
	instance.Status.ACLs = nil
	if len(instance.Spec.TopicGrants) > 0 {
		instance.Status.ACLs = kafkautil.GrantsToACLStrings(kafkaUser, instance.Spec.TopicGrants)
	}
	if err := r.Client.Status().Update(ctx, instance); err != nil {
		return requeueWithError(reqLogger, "failed to update kafkauser status", err)
	}

	return reconciled()
}

func (r *KafkaUserReconciler) finalizeUser(ctx context.Context, cluster *v1beta1.KafkaCluster, user *v1alpha1.KafkaUser) (reconcile.Result, error) {
	if !apiutil.StringSliceContains(user.Finalizers, userFinalizer) {
		return reconciled()
	}
	principals := []string{fmt.Sprintf("CN=%s", user.Name)}
	if user.Spec.GetIfCertShouldBeCreated() {
		if len(user.Spec.TopicGrants) > 0 && !k8sutil.IsMarkedForDeletion(cluster.ObjectMeta) {
			var err error
			principals, err = r.userACLPrincipals(ctx, user)
			if err != nil {
				// Never log Secret data: the error names only the Secret and the decode failure.
				return requeueWithError(logr.FromContextOrDiscard(ctx).WithValues("secret", user.Spec.SecretName),
					"KafkaUser deletion blocked: ACL cleanup principal is not recorded and cannot be read from the user certificate Secret",
					errors.WrapIf(err, "resolve ACL cleanup principal from certificate Secret "+user.Spec.SecretName))
			}
			if user.Status.ACLPrincipal == nil && len(principals) > 0 {
				// Checkpoint the legacy identity before PKI cleanup removes its certificate.
				user.Status.ACLPrincipal = &principals[0]
				if user, err = r.updateStatusAndFetchLatest(ctx, user); err != nil {
					return ctrl.Result{}, err
				}
			}
		}
		if cluster.Spec.ListenersConfig.SSLSecrets != nil || user.Spec.PKIBackendSpec != nil {
			backend := v1beta1.PKIBackendProvided
			if user.Spec.PKIBackendSpec != nil {
				backend = v1beta1.PKIBackend(user.Spec.PKIBackendSpec.PKIBackend)
			}
			if err := pki.GetPKIManager(freshReadClient{Client: r.Client, reader: r.DirectClient}, cluster, backend).FinalizeUserCertificate(ctx, user); err != nil {
				if errors.As(err, &errorfactory.ResourceNotReady{}) {
					return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
				}
				return ctrl.Result{}, err
			}
		}
	}
	if len(principals) == 0 {
		return ctrl.Result{}, r.removeFinalizer(ctx, user)
	}
	return r.checkFinalizers(ctx, cluster, user, principals)
}

// userACLPrincipals returns every principal that may still hold ACLs for the user. Both
// sources live in the status subresource, which only the operator writes: the recorded
// cleanup identity and the principal in the ACLs the operator last reported as created.
// An empty result means the operator recorded that no ACLs were submitted.
func (r *KafkaUserReconciler) userACLPrincipals(ctx context.Context, user *v1alpha1.KafkaUser) ([]string, error) {
	var principals []string
	if user.Status.ACLPrincipal != nil && *user.Status.ACLPrincipal != "" {
		principals = append(principals, *user.Status.ACLPrincipal)
	}
	for _, principal := range kafkautil.PrincipalsFromACLStrings(user.Status.ACLs) {
		if !apiutil.StringSliceContains(principals, principal) {
			principals = append(principals, principal)
		}
	}
	if len(principals) > 0 || user.Status.ACLPrincipal != nil {
		return principals, nil
	}
	secret := &corev1.Secret{}
	if err := r.DirectClient.Get(ctx, client.ObjectKey{Namespace: user.Namespace, Name: user.Spec.SecretName}, secret); err != nil {
		return nil, err
	}
	if !metav1.IsControlledBy(secret, user) {
		return nil, errors.New("certificate Secret is not owned by this KafkaUser")
	}
	certificate := &pkicommon.UserCertificate{Certificate: secret.Data[corev1.TLSCertKey]}
	principal, err := certificate.GetDistinguishedName()
	if err != nil {
		return nil, err
	}
	return []string{principal}, nil
}

func (r *KafkaUserReconciler) ensureUserFinalizer(ctx context.Context, user *v1alpha1.KafkaUser) (*v1alpha1.KafkaUser, error) {
	if apiutil.StringSliceContains(user.Finalizers, userFinalizer) {
		return user, nil
	}
	if user.Status.ACLPrincipal == nil && user.Status.State == "" && len(user.Status.ACLs) == 0 {
		principal, known, err := r.initialACLPrincipal(ctx, user)
		if err != nil {
			return nil, err
		}
		if known {
			// Empty records that no ACL submission has started; absence is legacy/unknown.
			// Record it before the finalizer, so a finalized user always has a known identity.
			user.Status.ACLPrincipal = &principal
			if user, err = r.updateStatusAndFetchLatest(ctx, user); err != nil {
				return nil, err
			}
		}
	}
	r.addFinalizer(logr.FromContextOrDiscard(ctx), user)
	return r.updateAndFetchLatest(ctx, user)
}

// initialACLPrincipal returns the ACL identity of a user that the operator sees for the
// first time. Only a certificate in a Secret this user controls can have been used for
// ACLs, so other Secrets are not read further. known is false when the user's own
// certificate cannot be read: that does not prove that no ACLs exist.
func (r *KafkaUserReconciler) initialACLPrincipal(ctx context.Context, user *v1alpha1.KafkaUser) (principal string, known bool, err error) {
	if !user.Spec.GetIfCertShouldBeCreated() || len(user.Spec.TopicGrants) == 0 {
		return "", true, nil
	}
	secret := &corev1.Secret{}
	err = r.DirectClient.Get(ctx, client.ObjectKey{Namespace: user.Namespace, Name: user.Spec.SecretName}, secret)
	if apierrors.IsNotFound(err) {
		return "", true, nil
	}
	if err != nil {
		return "", false, err
	}
	if !metav1.IsControlledBy(secret, user) || len(secret.Data[corev1.TLSCertKey]) == 0 {
		return "", true, nil
	}
	certificate := &pkicommon.UserCertificate{Certificate: secret.Data[corev1.TLSCertKey]}
	if principal, err = certificate.GetDistinguishedName(); err != nil {
		// Let the PKI backend repair the Secret; the principal is recorded before any ACL is created.
		logr.FromContextOrDiscard(ctx).Info("user certificate is unreadable; ACL identity stays unknown until it is reissued",
			"secret", user.Spec.SecretName)
		return "", false, nil
	}
	return principal, true, nil
}

func (r *KafkaUserReconciler) ensureClusterLabel(ctx context.Context, cluster *v1beta1.KafkaCluster, user *v1alpha1.KafkaUser) (*v1alpha1.KafkaUser, error) {
	labels := applyClusterRefLabel(cluster, user.GetLabels())
	if !reflect.DeepEqual(labels, user.GetLabels()) {
		user.SetLabels(labels)
		return r.updateAndFetchLatest(ctx, user)
	}
	return user, nil
}

func (r *KafkaUserReconciler) updateAndFetchLatest(ctx context.Context, user *v1alpha1.KafkaUser) (*v1alpha1.KafkaUser, error) {
	typeMeta := user.TypeMeta
	err := r.Client.Update(ctx, user)
	if err != nil {
		return nil, err
	}
	user.TypeMeta = typeMeta
	return user, nil
}

// updateStatusAndFetchLatest writes the status subresource, which ordinary KafkaUser
// edits cannot change, and keeps the TypeMeta that PKI ownership checks rely on.
func (r *KafkaUserReconciler) updateStatusAndFetchLatest(ctx context.Context, user *v1alpha1.KafkaUser) (*v1alpha1.KafkaUser, error) {
	typeMeta := user.TypeMeta
	if err := r.Client.Status().Update(ctx, user); err != nil {
		return nil, err
	}
	user.TypeMeta = typeMeta
	return user, nil
}

func (r *KafkaUserReconciler) checkFinalizers(ctx context.Context, cluster *v1beta1.KafkaCluster, instance *v1alpha1.KafkaUser, principals []string) (reconcile.Result, error) {
	reqLogger := logr.FromContextOrDiscard(ctx)
	// run finalizers
	var err error
	if apiutil.StringSliceContains(instance.GetFinalizers(), userFinalizer) {
		if len(instance.Spec.TopicGrants) > 0 {
			for _, principal := range principals {
				for _, topicGrant := range instance.Spec.TopicGrants {
					if err = r.finalizeKafkaUserACLs(reqLogger, cluster, principal, topicGrant); err != nil {
						return requeueWithError(reqLogger, "failed to finalize kafkauser", err)
					}
				}
			}
		}
		// remove finalizer
		if err = r.removeFinalizer(ctx, instance); err != nil {
			return requeueWithError(reqLogger, "failed to remove finalizer from kafkauser", err)
		}
	}
	return reconciled()
}

func (r *KafkaUserReconciler) removeFinalizer(ctx context.Context, user *v1alpha1.KafkaUser) error {
	user.SetFinalizers(util.StringSliceRemove(user.GetFinalizers(), userFinalizer))
	_, err := r.updateAndFetchLatest(ctx, user)
	return err
}

func (r *KafkaUserReconciler) finalizeKafkaUserACLs(reqLogger logr.Logger, cluster *v1beta1.KafkaCluster, user string, grant v1alpha1.UserTopicGrant) error {
	if k8sutil.IsMarkedForDeletion(cluster.ObjectMeta) {
		reqLogger.Info("Cluster is being deleted, skipping ACL deletion")
		return nil
	}
	var err error
	reqLogger.Info("Deleting user ACLs from kafka")
	broker, close, err := newKafkaFromCluster(r.Client, cluster)
	if err != nil {
		return err
	}
	defer close()
	if err = broker.DeleteUserACLs(grant.AccessType, grant.PatternType, user, grant.TopicName); err != nil {
		return err
	}
	return nil
}

func (r *KafkaUserReconciler) addFinalizer(reqLogger logr.Logger, user *v1alpha1.KafkaUser) {
	reqLogger.Info("Adding Finalizer for the KafkaUser")
	user.SetFinalizers(append(user.GetFinalizers(), userFinalizer))
}

// freshReadClient writes through the embedded client but reads from the apiserver,
// so PKI cleanup does not delete a Secret because a cached Certificate is missing.
type freshReadClient struct {
	client.Client
	reader client.Reader
}

func (c freshReadClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return c.reader.Get(ctx, key, obj, opts...)
}

func (c freshReadClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return c.reader.List(ctx, list, opts...)
}
