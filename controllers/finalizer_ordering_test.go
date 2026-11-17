// Copyright 2026 Adobe. All rights reserved.
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
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"slices"
	"sync"
	"testing"
	"time"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/banzaicloud/koperator/api/v1alpha1"
	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/kafkaclient"
	"github.com/banzaicloud/koperator/pkg/util"
	certutil "github.com/banzaicloud/koperator/pkg/util/cert"
	pkicommon "github.com/banzaicloud/koperator/pkg/util/pki"
	"github.com/banzaicloud/koperator/pkg/webhooks"
)

type protectedKafkaClient struct {
	kafkaclient.KafkaClient
	beforeMutation   func()
	afterMutationErr error
	deletedUsers     []string
	deleteUserErr    error
}

func (c *protectedKafkaClient) CreateTopic(opts *kafkaclient.CreateTopicOptions) error {
	c.beforeMutation()
	if err := c.KafkaClient.CreateTopic(opts); err != nil {
		return err
	}
	return c.afterMutationErr
}

func (c *protectedKafkaClient) CreateUserACLs(access v1alpha1.KafkaAccessType, pattern v1alpha1.KafkaPatternType, user, topic string) error {
	c.beforeMutation()
	if err := c.KafkaClient.CreateUserACLs(access, pattern, user, topic); err != nil {
		return err
	}
	return c.afterMutationErr
}

func (c *protectedKafkaClient) DeleteUserACLs(user string, pattern v1alpha1.KafkaPatternType) error {
	c.deletedUsers = append(c.deletedUsers, user)
	if c.deleteUserErr != nil {
		return c.deleteUserErr
	}
	return c.KafkaClient.DeleteUserACLs(user, pattern)
}

func TestFinalizersPrecedeProvisioning(t *testing.T) {
	for _, kind := range []string{"cluster", "topic", "user-acls", "user-certificate"} {
		for _, scenario := range []string{"first-reconcile", "failed-finalizer-write", "delete-during-finalizer-write", "restart-after-side-effect"} {
			t.Run(kind+"/"+scenario, func(t *testing.T) {
				ctx := context.Background()
				scheme := runtime.NewScheme()
				require.NoError(t, k8sscheme.AddToScheme(scheme))
				require.NoError(t, certv1.AddToScheme(scheme))
				require.NoError(t, v1alpha1.AddToScheme(scheme))
				require.NoError(t, v1beta1.AddToScheme(scheme))

				cluster := createTestKafkaCluster("protected-cluster", "default")
				var primary client.Object
				var finalizers []string
				switch kind {
				case "cluster":
					primary = cluster
					finalizers = []string{clusterFinalizer, clusterTopicsFinalizer, clusterUsersFinalizer}
				case "topic":
					primary = &v1alpha1.KafkaTopic{
						ObjectMeta: metav1.ObjectMeta{Name: "protected-topic", Namespace: cluster.Namespace},
						Spec: v1alpha1.KafkaTopicSpec{
							Name: "protected-topic", Partitions: 1, ReplicationFactor: 1,
							ClusterRef: v1alpha1.ClusterReference{Name: cluster.Name},
						},
					}
					finalizers = []string{topicFinalizer}
				default:
					primary = &v1alpha1.KafkaUser{
						ObjectMeta: metav1.ObjectMeta{Name: "protected-user", Namespace: cluster.Namespace},
						Spec: v1alpha1.KafkaUserSpec{
							SecretName: "protected-user",
							ClusterRef: v1alpha1.ClusterReference{Name: cluster.Name},
							CreateCert: util.BoolPointer(kind == "user-certificate"),
						},
					}
					if kind == "user-certificate" {
						cluster.Spec.ListenersConfig.SSLSecrets = &v1beta1.SSLSecrets{PKIBackend: v1beta1.PKIBackendCertManager}
					} else {
						primary.(*v1alpha1.KafkaUser).Spec.TopicGrants = []v1alpha1.UserTopicGrant{
							{TopicName: "protected-topic", AccessType: v1alpha1.KafkaAccessTypeRead},
						}
					}
					finalizers = []string{userFinalizer}
				}
				primary.SetFinalizers([]string{"example.com/unrelated"})
				objects := []client.Object{primary}
				if kind != "cluster" {
					objects = append(objects, cluster)
				}
				base := fake.NewClientBuilder().WithScheme(scheme).
					WithObjects(objects...).WithStatusSubresource(primary, cluster).Build()
				sentinel := errors.New("injected finalizer write failure")
				interrupted := errors.New("injected interruption after side effect")
				mutations := 0
				guard := func() {
					t.Helper()
					mutations++
					persisted := primary.DeepCopyObject().(client.Object)
					require.NoError(t, base.Get(ctx, client.ObjectKeyFromObject(primary), persisted))
					for _, f := range finalizers {
						require.Contains(t, persisted.GetFinalizers(), f, "side effect must have durable cleanup ownership")
					}
					require.Contains(t, persisted.GetFinalizers(), "example.com/unrelated")
					if kind != "cluster" {
						require.Equal(t, "protected-cluster.default", persisted.GetLabels()[clusterRefLabel],
							"cluster deletion must be able to discover partially provisioned children")
					}
					require.Nil(t, persisted.GetDeletionTimestamp(), "must not provision a deleting resource")
					require.NotEqual(t, "failed-finalizer-write", scenario)
					require.NotEqual(t, "delete-during-finalizer-write", scenario)
				}
				deleted := false
				wrapped := interceptor.NewClient(base, interceptor.Funcs{
					Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
						if client.ObjectKeyFromObject(obj) == client.ObjectKeyFromObject(primary) &&
							slices.Contains(obj.GetFinalizers(), finalizers[0]) {
							if scenario == "failed-finalizer-write" {
								return sentinel
							}
							if scenario == "delete-during-finalizer-write" && !deleted {
								deleted = true
								require.NoError(t, c.Delete(ctx, primary.DeepCopyObject().(client.Object)))
							}
						}
						return c.Update(ctx, obj, opts...)
					},
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						guard()
						err := c.Create(ctx, obj, opts...)
						if err == nil && scenario == "restart-after-side-effect" {
							return interrupted
						}
						return err
					},
				})
				broker, closeBroker, err := kafkaclient.NewMockFromCluster(base, cluster)
				require.NoError(t, err)
				t.Cleanup(closeBroker)
				protectedBroker := &protectedKafkaClient{KafkaClient: broker, beforeMutation: guard}
				if scenario == "restart-after-side-effect" {
					protectedBroker.afterMutationErr = interrupted
				}
				SetNewKafkaFromCluster(func(client.Client, *v1beta1.KafkaCluster) (kafkaclient.KafkaClient, func(), error) {
					return protectedBroker, func() {}, nil
				})
				t.Cleanup(func() { SetNewKafkaFromCluster(kafkaclient.NewFromCluster) })

				newReconciler := func(c client.Client) reconcile.Reconciler {
					switch kind {
					case "cluster":
						return &KafkaClusterReconciler{Client: c, DirectClient: base, KafkaClientProvider: kafkaclient.NewMockProvider()}
					case "topic":
						return &KafkaTopicReconciler{Client: c, Scheme: scheme}
					default:
						return &KafkaUserReconciler{Client: c, Scheme: scheme}
					}
				}
				req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(primary)}
				_, err = newReconciler(wrapped).Reconcile(ctx, req)
				switch scenario {
				case "failed-finalizer-write":
					require.ErrorIs(t, err, sentinel)
					require.Zero(t, mutations)
				case "delete-during-finalizer-write":
					require.Error(t, err, "deletion must invalidate the in-flight metadata update")
					require.Zero(t, mutations)
				default:
					require.Positive(t, mutations, "the first reconcile must continue after persisting finalizers")
					persisted := primary.DeepCopyObject().(client.Object)
					require.NoError(t, base.Get(ctx, req.NamespacedName, persisted))
					for _, f := range finalizers {
						require.Contains(t, persisted.GetFinalizers(), f)
					}
					if scenario == "restart-after-side-effect" {
						require.ErrorIs(t, err, interrupted)
						require.NoError(t, base.Delete(ctx, persisted))
						result, err := newReconciler(base).Reconcile(ctx, req)
						require.NoError(t, err)
						if kind == "topic" {
							require.Positive(t, result.RequeueAfter)
							require.NoError(t, base.Get(ctx, req.NamespacedName, persisted))
							require.Contains(t, persisted.GetFinalizers(), topicFinalizer)
							_, err = newReconciler(base).Reconcile(ctx, req)
							require.NoError(t, err)
						}
						require.NoError(t, base.Get(ctx, req.NamespacedName, persisted), "unrelated finalizer retains the object")
						for _, f := range finalizers {
							require.NotContains(t, persisted.GetFinalizers(), f)
						}
						if kind == "topic" {
							topic, err := broker.GetTopic("protected-topic")
							require.NoError(t, err)
							require.Nil(t, topic)
						}
						if kind == "user-acls" {
							acls, err := broker.ListUserACLs()
							require.NoError(t, err)
							require.Empty(t, acls)
						}
					}
				}
			})
		}
	}
}

func TestUnmanagedTopicDoesNotAcquireCleanupOwnership(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v1beta1.AddToScheme(scheme))
	topic := &v1alpha1.KafkaTopic{
		ObjectMeta: metav1.ObjectMeta{
			Name: "external-topic", Namespace: "default",
			Annotations: map[string]string{webhooks.TopicManagedByAnnotationKey: "external"},
		},
		Spec: v1alpha1.KafkaTopicSpec{ClusterRef: v1alpha1.ClusterReference{Name: "cluster"}},
	}
	cluster := &v1beta1.KafkaCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(topic, cluster).WithStatusSubresource(topic).Build()
	SetNewKafkaFromCluster(func(client.Client, *v1beta1.KafkaCluster) (kafkaclient.KafkaClient, func(), error) {
		t.Fatal("unmanaged topic must not require Kafka availability")
		return nil, nil, nil
	})
	t.Cleanup(func() { SetNewKafkaFromCluster(kafkaclient.NewFromCluster) })
	_, err := (&KafkaTopicReconciler{Client: c}).Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(topic)})
	require.NoError(t, err)
	require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(topic), topic))
	require.Empty(t, topic.Finalizers)
	require.Equal(t, "external", topic.Status.ManagedBy)
}

func TestDeletingUserDoesNotProvisionCertificate(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, k8sscheme.AddToScheme(scheme))
	require.NoError(t, certv1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v1beta1.AddToScheme(scheme))
	for _, withGrants := range []bool{false, true} {
		t.Run(map[bool]string{false: "no-grants", true: "unsigned-certificate"}[withGrants], func(t *testing.T) {
			user := &v1alpha1.KafkaUser{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pending-user", Namespace: "default", Finalizers: []string{userFinalizer},
					DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
				},
				Spec: v1alpha1.KafkaUserSpec{
					SecretName: "pending-secret", ClusterRef: v1alpha1.ClusterReference{Name: "cluster"},
				},
			}
			if withGrants {
				user.Annotations = map[string]string{userACLPrincipalAnnotation: ""}
				user.Spec.TopicGrants = []v1alpha1.UserTopicGrant{{TopicName: "topic", AccessType: v1alpha1.KafkaAccessTypeRead}}
			}
			cluster := &v1beta1.KafkaCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default"},
				Spec: v1beta1.KafkaClusterSpec{ListenersConfig: v1beta1.ListenersConfig{
					SSLSecrets: &v1beta1.SSLSecrets{PKIBackend: v1beta1.PKIBackendCertManager},
				}},
			}
			secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "pending-secret", Namespace: "default"}}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(user, cluster, secret).Build()
			guarded := interceptor.NewClient(c, interceptor.Funcs{
				Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
					t.Fatal("deletion must not provision PKI resources")
					return nil
				},
			})
			_, err := (&KafkaUserReconciler{Client: guarded, Scheme: scheme}).Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(user)})
			require.NoError(t, err)
			require.True(t, apierrors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(user), user)))
		})
	}
}

func TestDeletingUserUsesExistingCertificateIdentity(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, k8sscheme.AddToScheme(scheme))
	require.NoError(t, certv1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v1beta1.AddToScheme(scheme))
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "issued-principal", Organization: []string{"existing-organization"}},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
	require.NoError(t, err)

	user := &v1alpha1.KafkaUser{
		ObjectMeta: metav1.ObjectMeta{
			Name: "deleting-user", Namespace: "default",
			Finalizers: []string{userFinalizer}, DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
		},
		Spec: v1alpha1.KafkaUserSpec{
			SecretName: "existing-secret", ClusterRef: v1alpha1.ClusterReference{Name: "cluster"},
			TopicGrants: []v1alpha1.UserTopicGrant{{TopicName: "topic", AccessType: v1alpha1.KafkaAccessTypeRead}},
		},
	}
	cluster := &v1beta1.KafkaCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default"}}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "existing-secret", Namespace: "default"},
		Data:       map[string][]byte{corev1.TLSCertKey: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(user, cluster, secret).Build()
	broker, closeBroker, err := kafkaclient.NewMockFromCluster(c, cluster)
	require.NoError(t, err)
	t.Cleanup(closeBroker)
	protectedBroker := &protectedKafkaClient{KafkaClient: broker}
	SetNewKafkaFromCluster(func(client.Client, *v1beta1.KafkaCluster) (kafkaclient.KafkaClient, func(), error) {
		return protectedBroker, func() {}, nil
	})
	t.Cleanup(func() { SetNewKafkaFromCluster(kafkaclient.NewFromCluster) })
	_, err = (&KafkaUserReconciler{Client: c, Scheme: scheme}).Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(user)})
	require.NoError(t, err)
	require.Equal(t, []string{"CN=issued-principal,O=existing-organization"}, protectedBroker.deletedUsers)
	require.True(t, apierrors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(user), user)))
}

func TestDeletingLegacyUserCheckpointsACLIdentity(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "legacy-principal", Organization: []string{"legacy-organization"}},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
	require.NoError(t, err)
	const principal = "CN=legacy-principal,O=legacy-organization"

	for _, scenario := range []string{"acl-failure-then-restart", "checkpoint-failure"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			scheme := runtime.NewScheme()
			require.NoError(t, k8sscheme.AddToScheme(scheme))
			require.NoError(t, certv1.AddToScheme(scheme))
			require.NoError(t, v1alpha1.AddToScheme(scheme))
			require.NoError(t, v1beta1.AddToScheme(scheme))
			user := &v1alpha1.KafkaUser{
				ObjectMeta: metav1.ObjectMeta{
					Name: "legacy-user", Namespace: "default", UID: "legacy-user-uid",
					Finalizers: []string{userFinalizer, "example.com/unrelated"}, DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
				},
				Spec: v1alpha1.KafkaUserSpec{
					SecretName: "legacy-secret", ClusterRef: v1alpha1.ClusterReference{Name: "cluster"},
					CreateCert: util.BoolPointer(true),
					TopicGrants: []v1alpha1.UserTopicGrant{{
						TopicName: "topic", AccessType: v1alpha1.KafkaAccessTypeRead, PatternType: v1alpha1.KafkaPatternTypeLiteral,
					}},
				},
				Status: v1alpha1.KafkaUserStatus{State: v1alpha1.UserStateCreated},
			}
			cluster := &v1beta1.KafkaCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default"},
				Spec: v1beta1.KafkaClusterSpec{ListenersConfig: v1beta1.ListenersConfig{
					SSLSecrets: &v1beta1.SSLSecrets{PKIBackend: v1beta1.PKIBackendCertManager},
				}},
			}
			owner := *metav1.NewControllerRef(user, v1alpha1.GroupVersion.WithKind("KafkaUser"))
			cert := &certv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{Name: user.Name, Namespace: user.Namespace, OwnerReferences: []metav1.OwnerReference{owner}},
				Spec:       certv1.CertificateSpec{SecretName: user.Spec.SecretName},
			}
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: user.Spec.SecretName, Namespace: user.Namespace, OwnerReferences: []metav1.OwnerReference{owner}},
				Data:       map[string][]byte{corev1.TLSCertKey: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})},
			}
			base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(user, cluster, cert, secret).WithStatusSubresource(user).Build()
			checkpointFailure := errors.New("injected ACL identity checkpoint failure")
			failCheckpoint := scenario == "checkpoint-failure"
			wrapped := interceptor.NewClient(base, interceptor.Funcs{
				Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
					if _, ok := obj.(*v1alpha1.KafkaUser); ok && failCheckpoint && obj.GetAnnotations()[userACLPrincipalAnnotation] == principal {
						return checkpointFailure
					}
					return c.Update(ctx, obj, opts...)
				},
			})
			broker, closeBroker, err := kafkaclient.NewMockFromCluster(base, cluster)
			require.NoError(t, err)
			t.Cleanup(closeBroker)
			require.NoError(t, broker.CreateUserACLs(v1alpha1.KafkaAccessTypeRead, v1alpha1.KafkaPatternTypeLiteral, principal, "topic"))
			aclFailure := errors.New("injected ACL deletion failure")
			protectedBroker := &protectedKafkaClient{KafkaClient: broker}
			if scenario == "acl-failure-then-restart" {
				protectedBroker.deleteUserErr = aclFailure
			}
			SetNewKafkaFromCluster(func(client.Client, *v1beta1.KafkaCluster) (kafkaclient.KafkaClient, func(), error) {
				return protectedBroker, func() {}, nil
			})
			t.Cleanup(func() { SetNewKafkaFromCluster(kafkaclient.NewFromCluster) })
			req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(user)}
			_, err = (&KafkaUserReconciler{Client: wrapped, Scheme: scheme}).Reconcile(ctx, req)
			if failCheckpoint {
				require.ErrorIs(t, err, checkpointFailure)
				require.NoError(t, base.Get(ctx, client.ObjectKeyFromObject(cert), cert), "failed checkpoint must preserve the Certificate")
				require.NoError(t, base.Get(ctx, client.ObjectKeyFromObject(secret), secret), "failed checkpoint must preserve the identity Secret")
				require.Empty(t, protectedBroker.deletedUsers, "failed checkpoint must stop ACL cleanup")
			} else {
				require.ErrorIs(t, err, aclFailure)
				require.True(t, apierrors.IsNotFound(base.Get(ctx, client.ObjectKeyFromObject(cert), cert)))
				require.True(t, apierrors.IsNotFound(base.Get(ctx, client.ObjectKeyFromObject(secret), secret)))
			}
			require.NoError(t, base.Get(ctx, req.NamespacedName, user))
			require.Contains(t, user.Finalizers, userFinalizer)
			require.Contains(t, user.Finalizers, "example.com/unrelated")
			if failCheckpoint {
				require.NotContains(t, user.Annotations, userACLPrincipalAnnotation)
			}
			acls, err := broker.ListUserACLs()
			require.NoError(t, err)
			require.NotEmpty(t, acls, "failed cleanup must leave ACLs available for retry")

			failCheckpoint = false
			protectedBroker.deleteUserErr = nil
			_, err = (&KafkaUserReconciler{Client: wrapped, Scheme: scheme}).Reconcile(ctx, req)
			require.NoError(t, err, "restart must recover the principal even after the owned Secret was deleted")
			require.NoError(t, base.Get(ctx, req.NamespacedName, user))
			require.Equal(t, principal, user.Annotations[userACLPrincipalAnnotation])
			require.Equal(t, []string{"example.com/unrelated"}, user.Finalizers)
			if scenario == "acl-failure-then-restart" {
				require.Equal(t, []string{principal, principal}, protectedBroker.deletedUsers)
			} else {
				require.Equal(t, []string{principal}, protectedBroker.deletedUsers)
			}
			acls, err = broker.ListUserACLs()
			require.NoError(t, err)
			require.Empty(t, acls)
			require.True(t, apierrors.IsNotFound(base.Get(ctx, client.ObjectKeyFromObject(cert), cert)))
			require.True(t, apierrors.IsNotFound(base.Get(ctx, client.ObjectKeyFromObject(secret), secret)))
		})
	}
}

func TestDeletionRetainsFinalizerOnClusterReadFailure(t *testing.T) {
	for _, kind := range []string{"topic", "user"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			scheme := runtime.NewScheme()
			require.NoError(t, v1alpha1.AddToScheme(scheme))
			require.NoError(t, v1beta1.AddToScheme(scheme))
			meta := metav1.ObjectMeta{
				Name: "deleting", Namespace: "default", DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
			}
			var obj client.Object
			if kind == "topic" {
				meta.Finalizers = []string{topicFinalizer}
				obj = &v1alpha1.KafkaTopic{ObjectMeta: meta, Spec: v1alpha1.KafkaTopicSpec{
					ClusterRef: v1alpha1.ClusterReference{Name: "cluster"},
				}}
			} else {
				meta.Finalizers = []string{userFinalizer}
				obj = &v1alpha1.KafkaUser{ObjectMeta: meta, Spec: v1alpha1.KafkaUserSpec{
					ClusterRef: v1alpha1.ClusterReference{Name: "cluster"},
				}}
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(obj).Build()
			readFailure := errors.New("temporary cluster read failure")
			wrapped := interceptor.NewClient(c, interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, object client.Object, opts ...client.GetOption) error {
					if _, ok := object.(*v1beta1.KafkaCluster); ok {
						return readFailure
					}
					return c.Get(ctx, key, object, opts...)
				},
			})
			var r reconcile.Reconciler = &KafkaTopicReconciler{Client: wrapped}
			if kind == "user" {
				r = &KafkaUserReconciler{Client: wrapped}
			}
			_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(obj)})
			require.ErrorIs(t, err, readFailure)
			current := obj.DeepCopyObject().(client.Object)
			require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(obj), current))
			require.Equal(t, meta.Finalizers, current.GetFinalizers())
		})
	}
}

func TestDeletingUserAfterProvisioningFailsBeforeSecretCreation(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, k8sscheme.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v1beta1.AddToScheme(scheme))
	user := &v1alpha1.KafkaUser{
		ObjectMeta: metav1.ObjectMeta{Name: "unprovisioned", Namespace: "default"},
		Spec: v1alpha1.KafkaUserSpec{
			SecretName: "never-created", ClusterRef: v1alpha1.ClusterReference{Name: "cluster"},
			TopicGrants: []v1alpha1.UserTopicGrant{{TopicName: "topic", AccessType: v1alpha1.KafkaAccessTypeRead}},
		},
	}
	cluster := &v1beta1.KafkaCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(user, cluster).WithStatusSubresource(user).Build()
	r := &KafkaUserReconciler{Client: c, Scheme: scheme}
	req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(user)}
	_, err := r.Reconcile(ctx, req)
	require.Error(t, err, "cluster is not configured for certificate issuance")
	require.NoError(t, c.Get(ctx, req.NamespacedName, user))
	require.Contains(t, user.Finalizers, userFinalizer)
	require.NoError(t, c.Delete(ctx, user))
	_, err = (&KafkaUserReconciler{Client: c, Scheme: scheme}).Reconcile(ctx, req)
	require.NoError(t, err)
	require.True(t, apierrors.IsNotFound(c.Get(ctx, req.NamespacedName, user)))
}

func TestDeletingUserAfterACLsCreatedAndStatusWriteFails(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, k8sscheme.AddToScheme(scheme))
	require.NoError(t, certv1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v1beta1.AddToScheme(scheme))
	user := &v1alpha1.KafkaUser{
		ObjectMeta: metav1.ObjectMeta{Name: "partial-user", Namespace: "default", UID: "partial-user-uid"},
		Spec: v1alpha1.KafkaUserSpec{
			SecretName: "partial-secret", ClusterRef: v1alpha1.ClusterReference{Name: "cluster"},
			TopicGrants: []v1alpha1.UserTopicGrant{{TopicName: "topic", AccessType: v1alpha1.KafkaAccessTypeRead}},
		},
	}
	cluster := &v1beta1.KafkaCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default"},
		Spec: v1beta1.KafkaClusterSpec{ListenersConfig: v1beta1.ListenersConfig{
			SSLSecrets: &v1beta1.SSLSecrets{PKIBackend: v1beta1.PKIBackendCertManager},
		}},
	}
	certificatePEM, _, _, err := certutil.GenerateTestCert()
	require.NoError(t, err)
	certificate := &pkicommon.UserCertificate{Certificate: certificatePEM}
	principal, err := certificate.GetDistinguishedName()
	require.NoError(t, err)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "partial-secret", Namespace: "default"},
		Data: map[string][]byte{
			corev1.TLSCertKey: certificatePEM, corev1.TLSPrivateKeyKey: []byte("key"), v1alpha1.CoreCACertKey: certificatePEM,
		},
	}
	cert := &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name: user.Name, Namespace: user.Namespace,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(user, v1alpha1.GroupVersion.WithKind("KafkaUser"))},
		},
		Spec: certv1.CertificateSpec{SecretName: user.Spec.SecretName},
	}
	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(user, cluster, secret, cert).WithStatusSubresource(user).Build()
	stale := false
	writeFailure := errors.New("status write failed after ACL creation")
	wrapped := interceptor.NewClient(base, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if err := c.Get(ctx, key, obj, opts...); err != nil {
				return err
			}
			if s, ok := obj.(*corev1.Secret); ok && stale {
				s.Data = nil
			}
			return nil
		},
		SubResourceUpdate: func(ctx context.Context, c client.Client, subresource string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			if _, ok := obj.(*v1alpha1.KafkaUser); ok && stale {
				return writeFailure
			}
			return c.SubResource(subresource).Update(ctx, obj, opts...)
		},
	})
	broker, closeBroker, err := kafkaclient.NewMockFromCluster(base, cluster)
	require.NoError(t, err)
	t.Cleanup(closeBroker)
	protectedBroker := &protectedKafkaClient{KafkaClient: broker, beforeMutation: func() { stale = true }}
	SetNewKafkaFromCluster(func(client.Client, *v1beta1.KafkaCluster) (kafkaclient.KafkaClient, func(), error) {
		return protectedBroker, func() {}, nil
	})
	t.Cleanup(func() { SetNewKafkaFromCluster(kafkaclient.NewFromCluster) })
	req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(user)}
	_, err = (&KafkaUserReconciler{Client: wrapped, Scheme: scheme}).Reconcile(ctx, req)
	require.ErrorIs(t, err, writeFailure)
	require.NoError(t, base.Get(ctx, req.NamespacedName, user))
	require.Empty(t, user.Status.ACLs)
	require.NoError(t, base.Delete(ctx, user))
	_, err = (&KafkaUserReconciler{Client: wrapped, Scheme: scheme}).Reconcile(ctx, req)
	require.NoError(t, err)
	require.Equal(t, []string{principal}, protectedBroker.deletedUsers)
}

func TestDeletingUserAfterAsynchronousCertificateIssuance(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, k8sscheme.AddToScheme(scheme))
	require.NoError(t, certv1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v1beta1.AddToScheme(scheme))
	// Subject of the fixture returned by certutil.GenerateTestCert.
	const principal = "CN=test-cn,O=test-ou"
	user := &v1alpha1.KafkaUser{
		ObjectMeta: metav1.ObjectMeta{Name: "async-user", Namespace: "default", UID: "async-user-uid"},
		Spec: v1alpha1.KafkaUserSpec{
			SecretName: "async-secret", ClusterRef: v1alpha1.ClusterReference{Name: "cluster"},
			TopicGrants: []v1alpha1.UserTopicGrant{{TopicName: "topic", AccessType: v1alpha1.KafkaAccessTypeRead}},
		},
	}
	cluster := &v1beta1.KafkaCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default"},
		Spec: v1beta1.KafkaClusterSpec{ListenersConfig: v1beta1.ListenersConfig{
			SSLSecrets: &v1beta1.SSLSecrets{PKIBackend: v1beta1.PKIBackendCertManager},
		}},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(user, cluster).WithStatusSubresource(user).Build()
	broker, closeBroker, err := kafkaclient.NewMockFromCluster(c, cluster)
	require.NoError(t, err)
	t.Cleanup(closeBroker)
	req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(user)}
	protectedBroker := &protectedKafkaClient{KafkaClient: broker, beforeMutation: func() {
		persisted := &v1alpha1.KafkaUser{}
		require.NoError(t, c.Get(ctx, req.NamespacedName, persisted))
		require.Equal(t, principal, persisted.Annotations[userACLPrincipalAnnotation],
			"ACL creation must follow a durable record of the issued principal")
	}}
	SetNewKafkaFromCluster(func(client.Client, *v1beta1.KafkaCluster) (kafkaclient.KafkaClient, func(), error) {
		return protectedBroker, func() {}, nil
	})
	t.Cleanup(func() { SetNewKafkaFromCluster(kafkaclient.NewFromCluster) })
	r := &KafkaUserReconciler{Client: c, Scheme: scheme}

	// cert-manager has not issued the Secret yet.
	result, err := r.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Positive(t, result.RequeueAfter)
	acls, err := broker.ListUserACLs()
	require.NoError(t, err)
	require.Empty(t, acls)

	certificatePEM, _, _, err := certutil.GenerateTestCert()
	require.NoError(t, err)
	require.NoError(t, c.Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: user.Spec.SecretName, Namespace: user.Namespace},
		Data: map[string][]byte{
			corev1.TLSCertKey: certificatePEM, corev1.TLSPrivateKeyKey: []byte("key"), v1alpha1.CoreCACertKey: certificatePEM,
		},
	}))
	_, err = r.Reconcile(ctx, req)
	require.NoError(t, err)
	acls, err = broker.ListUserACLs()
	require.NoError(t, err)
	require.NotEmpty(t, acls)

	require.NoError(t, c.Delete(ctx, user))
	for attempt := 0; attempt < 3 && !apierrors.IsNotFound(c.Get(ctx, req.NamespacedName, user)); attempt++ {
		_, err = r.Reconcile(ctx, req)
		require.NoError(t, err)
	}
	require.True(t, apierrors.IsNotFound(c.Get(ctx, req.NamespacedName, user)))
	require.Equal(t, []string{principal}, protectedBroker.deletedUsers)
	acls, err = broker.ListUserACLs()
	require.NoError(t, err)
	require.Empty(t, acls, "deleting an asynchronously issued user must not leak its ACLs")
}

type pendingTopicDeletionClient struct {
	kafkaclient.KafkaClient
	releaseWait <-chan struct{}
	submitted   chan<- string
	deleteErr   error
}

func (c *pendingTopicDeletionClient) DeleteTopic(name string, wait bool) error {
	c.submitted <- name
	if c.deleteErr != nil {
		return c.deleteErr
	}
	if wait {
		<-c.releaseWait
	}
	return nil
}

func TestDeletingTopicYieldsWhileDeletionPending(t *testing.T) {
	for _, scenario := range []string{"stalled-wait", "accepted-but-still-visible", "deletion-error-then-retry"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			scheme := runtime.NewScheme()
			require.NoError(t, v1alpha1.AddToScheme(scheme))
			require.NoError(t, v1beta1.AddToScheme(scheme))
			cluster := createTestKafkaCluster("pending-deletion-cluster", "default")
			topic := &v1alpha1.KafkaTopic{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pending-topic", Namespace: cluster.Namespace,
					Finalizers: []string{topicFinalizer, "example.com/unrelated"},
				},
				Spec: v1alpha1.KafkaTopicSpec{
					Name: "pending-topic", Partitions: 1, ReplicationFactor: 1,
					ClusterRef: v1alpha1.ClusterReference{Name: cluster.Name},
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(topic, cluster).Build()
			require.NoError(t, c.Delete(ctx, topic))
			broker, closeBroker, err := kafkaclient.NewMockFromCluster(c, cluster)
			require.NoError(t, err)
			t.Cleanup(closeBroker)
			require.NoError(t, broker.CreateTopic(&kafkaclient.CreateTopicOptions{
				Name: topic.Spec.Name, Partitions: 1, ReplicationFactor: 1,
			}))
			releaseWait := make(chan struct{})
			stopWaiting := sync.OnceFunc(func() { close(releaseWait) })
			t.Cleanup(stopWaiting)
			if scenario != "stalled-wait" {
				stopWaiting()
			}
			deletionFailure := errors.New("injected topic deletion failure")
			var deleteErr error
			if scenario == "deletion-error-then-retry" {
				deleteErr = deletionFailure
			}
			submitted := make(chan string, 2)
			SetNewKafkaFromCluster(func(client.Client, *v1beta1.KafkaCluster) (kafkaclient.KafkaClient, func(), error) {
				return &pendingTopicDeletionClient{
					KafkaClient: broker, releaseWait: releaseWait, submitted: submitted, deleteErr: deleteErr,
				}, func() {}, nil
			})
			t.Cleanup(func() { SetNewKafkaFromCluster(kafkaclient.NewFromCluster) })
			req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(topic)}
			type reconcileOutcome struct {
				result ctrl.Result
				err    error
			}
			for attempt := 0; attempt < 2; attempt++ {
				done := make(chan reconcileOutcome, 1)
				var outcome reconcileOutcome
				go func() {
					result, err := (&KafkaTopicReconciler{Client: c, Scheme: scheme}).Reconcile(ctx, req)
					done <- reconcileOutcome{result: result, err: err}
				}()
				select {
				case outcome = <-done:
				case <-time.After(time.Second):
					stopWaiting()
					<-done
					t.Fatal("Reconcile held a worker while topic deletion remained pending")
				}
				require.NoError(t, c.Get(ctx, req.NamespacedName, topic))
				require.Contains(t, topic.Finalizers, topicFinalizer, "restart must retain cleanup ownership until observed absence")
				require.Contains(t, topic.Finalizers, "example.com/unrelated")
				if deleteErr != nil {
					require.ErrorIs(t, outcome.err, deletionFailure)
				} else {
					require.NoError(t, outcome.err)
					require.Positive(t, outcome.result.RequeueAfter, "pending deletion must schedule another observation")
				}
				require.Len(t, submitted, 1, "each pending reconcile must submit a bounded deletion request")
				require.Equal(t, "pending-topic", <-submitted)
				deleteErr = nil
			}

			require.NoError(t, broker.DeleteTopic(topic.Spec.Name, false))
			result, err := (&KafkaTopicReconciler{Client: c, Scheme: scheme}).Reconcile(ctx, req)
			require.NoError(t, err)
			require.Zero(t, result.RequeueAfter)
			require.NoError(t, c.Get(ctx, req.NamespacedName, topic))
			require.Equal(t, []string{"example.com/unrelated"}, topic.Finalizers)
			require.Empty(t, submitted, "observed absence must not submit another deletion")
		})
	}
}
