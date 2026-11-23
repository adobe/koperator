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
	"testing"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/banzaicloud/koperator/api/v1alpha1"
	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/kafkaclient"
	"github.com/banzaicloud/koperator/pkg/util"
	certutil "github.com/banzaicloud/koperator/pkg/util/cert"
)

func userDirectReadScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, k8sscheme.AddToScheme(scheme))
	require.NoError(t, certv1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v1beta1.AddToScheme(scheme))
	return scheme
}

// staleCache simulates an informer cache that has not observed Secrets or Certificates yet.
func staleCache(base client.WithWatch) client.Client {
	return interceptor.NewClient(base, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			switch obj.(type) {
			case *corev1.Secret:
				return apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, key.Name)
			case *certv1.Certificate:
				return apierrors.NewNotFound(schema.GroupResource{Group: certv1.SchemeGroupVersion.Group, Resource: "certificates"}, key.Name)
			}
			return c.Get(ctx, key, obj, opts...)
		},
	})
}

func recordDeletedUsers(t *testing.T, base client.Client, cluster *v1beta1.KafkaCluster) *protectedKafkaClient {
	t.Helper()
	broker, closeBroker, err := kafkaclient.NewMockFromCluster(base, cluster)
	require.NoError(t, err)
	t.Cleanup(closeBroker)
	recorder := &protectedKafkaClient{KafkaClient: broker, beforeMutation: func() {}}
	SetNewKafkaFromCluster(func(client.Client, *v1beta1.KafkaCluster) (kafkaclient.KafkaClient, func(), error) {
		return recorder, func() {}, nil
	})
	t.Cleanup(func() { SetNewKafkaFromCluster(kafkaclient.NewFromCluster) })
	return recorder
}

func TestUserACLIdentityUsesFreshSecretReads(t *testing.T) {
	certificatePEM, _, _, err := certutil.GenerateTestCert()
	require.NoError(t, err)
	// Subject of the fixture returned by certutil.GenerateTestCert.
	const principal = "CN=test-cn,O=test-ou"

	for _, scenario := range []string{"finalizer-initialization", "legacy-deletion"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			scheme := userDirectReadScheme(t)
			user := &v1alpha1.KafkaUser{
				ObjectMeta: metav1.ObjectMeta{Name: "fresh-read-user", Namespace: "default", UID: "fresh-read-user-uid"},
				Spec: v1alpha1.KafkaUserSpec{
					SecretName: "fresh-read-secret", ClusterRef: v1alpha1.ClusterReference{Name: "cluster"},
					TopicGrants: []v1alpha1.UserTopicGrant{{TopicName: "topic", AccessType: v1alpha1.KafkaAccessTypeRead}},
				},
			}
			if scenario == "legacy-deletion" {
				user.Finalizers = []string{userFinalizer, "example.com/unrelated"}
				user.DeletionTimestamp = &metav1.Time{Time: metav1.Now().Time}
				user.Status = v1alpha1.KafkaUserStatus{State: v1alpha1.UserStateCreated}
			}
			// No PKI is configured, so the identity can only come from the existing Secret.
			cluster := &v1beta1.KafkaCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default"}}
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: user.Spec.SecretName, Namespace: user.Namespace, OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(user, v1alpha1.GroupVersion.WithKind("KafkaUser"))}},
				Data:       map[string][]byte{corev1.TLSCertKey: certificatePEM},
			}
			base := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(user, cluster, secret).WithStatusSubresource(user).Build()
			recorder := recordDeletedUsers(t, base, cluster)
			r := &KafkaUserReconciler{Client: staleCache(base), DirectClient: base, Scheme: scheme}
			req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(user)}

			if scenario == "finalizer-initialization" {
				_, err := r.Reconcile(ctx, req)
				require.Error(t, err, "cluster is not configured for certificate issuance")
				require.NoError(t, base.Get(ctx, req.NamespacedName, user))
				require.Contains(t, user.Finalizers, userFinalizer)
				user.Finalizers = append(user.Finalizers, "example.com/unrelated")
				require.NoError(t, base.Update(ctx, user))
				require.NoError(t, base.Delete(ctx, user))
			}

			_, err := r.Reconcile(ctx, req)
			require.NoError(t, err)
			require.Equal(t, []string{principal}, recorder.deletedUsers,
				"an existing certificate hidden by a stale cache must still own ACL cleanup")
			require.NoError(t, base.Get(ctx, req.NamespacedName, user))
			require.Equal(t, []string{"example.com/unrelated"}, user.Finalizers)
		})
	}
}

func TestUserCertificateCleanupUsesFreshCertificateReads(t *testing.T) {
	ctx := context.Background()
	scheme := userDirectReadScheme(t)
	user := &v1alpha1.KafkaUser{
		ObjectMeta: metav1.ObjectMeta{
			Name: "fresh-cert-user", Namespace: "default", UID: "fresh-cert-user-uid",
			Finalizers: []string{userFinalizer, "example.com/unrelated"}, DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
		},
		Spec: v1alpha1.KafkaUserSpec{SecretName: "fresh-cert-secret", ClusterRef: v1alpha1.ClusterReference{Name: "cluster"}},
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
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name: user.Spec.SecretName, Namespace: user.Namespace, OwnerReferences: []metav1.OwnerReference{owner},
	}}
	base := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(user, cluster, cert, secret).WithStatusSubresource(user).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				if _, ok := obj.(*corev1.Secret); ok {
					err := c.Get(ctx, client.ObjectKeyFromObject(cert), &certv1.Certificate{})
					require.True(t, apierrors.IsNotFound(err), "stop the certificate producer before deleting its Secret")
				}
				return c.Delete(ctx, obj, opts...)
			},
		}).Build()
	r := &KafkaUserReconciler{Client: staleCache(base), DirectClient: base, Scheme: scheme}
	req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(user)}

	_, err := r.Reconcile(ctx, req)
	require.NoError(t, err)
	require.True(t, apierrors.IsNotFound(base.Get(ctx, client.ObjectKeyFromObject(cert), cert)))
	require.True(t, apierrors.IsNotFound(base.Get(ctx, client.ObjectKeyFromObject(secret), secret)))
	require.NoError(t, base.Get(ctx, req.NamespacedName, user))
	require.Equal(t, []string{"example.com/unrelated"}, user.Finalizers)
}

func TestSteadyStateUserReconcileReadsFromCache(t *testing.T) {
	ctx := context.Background()
	scheme := userDirectReadScheme(t)
	cluster := &v1beta1.KafkaCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default"}}
	user := &v1alpha1.KafkaUser{
		ObjectMeta: metav1.ObjectMeta{
			Name: "steady-user", Namespace: "default", Finalizers: []string{userFinalizer},
			Labels: applyClusterRefLabel(cluster, nil),
		},
		Spec: v1alpha1.KafkaUserSpec{
			SecretName: "steady-secret", ClusterRef: v1alpha1.ClusterReference{Name: cluster.Name},
			CreateCert:  util.BoolPointer(false),
			TopicGrants: []v1alpha1.UserTopicGrant{{TopicName: "topic", AccessType: v1alpha1.KafkaAccessTypeRead}},
		},
	}
	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(user, cluster).WithStatusSubresource(user).Build()
	recordDeletedUsers(t, base, cluster)
	direct := interceptor.NewClient(base, interceptor.Funcs{
		Get: func(_ context.Context, _ client.WithWatch, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
			t.Fatalf("steady-state reconcile must use the cache, read %T %s directly", obj, key)
			return nil
		},
		List: func(_ context.Context, _ client.WithWatch, list client.ObjectList, _ ...client.ListOption) error {
			t.Fatalf("steady-state reconcile must use the cache, listed %T directly", list)
			return nil
		},
	})
	r := &KafkaUserReconciler{Client: base, DirectClient: direct, Scheme: scheme}
	req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(user)}
	for range 2 {
		_, err := r.Reconcile(ctx, req)
		require.NoError(t, err)
	}
	require.NoError(t, base.Get(ctx, req.NamespacedName, user))
	require.Equal(t, v1alpha1.UserStateCreated, user.Status.State)
}
