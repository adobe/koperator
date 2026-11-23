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

package certmanagerpki

import (
	"context"
	"testing"

	"emperror.dev/errors"
	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/banzaicloud/koperator/api/v1alpha1"
	"github.com/banzaicloud/koperator/pkg/errorfactory"
)

func TestFinalizeUserCertificateCleansRegeneratedSecret(t *testing.T) {
	for _, owner := range []string{"generated", "external", "shared"} {
		t.Run(owner, func(t *testing.T) {
			ctx := context.Background()
			manager, err := newMock(newMockCluster())
			require.NoError(t, err)
			user := newMockUser()
			user.UID = "user-uid"
			certificate := &certv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name: user.Name, Namespace: user.Namespace,
					OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(user, v1alpha1.GroupVersion.WithKind("KafkaUser"))},
				},
				Spec: certv1.CertificateSpec{SecretName: user.Spec.SecretName},
			}
			secret := newMockUserSecret()
			secret.Annotations = map[string]string{certv1.CertificateNameKey: user.Name}
			if owner != "generated" {
				secret.OwnerReferences = []metav1.OwnerReference{{
					APIVersion: "v1", Kind: "ConfigMap", Name: "external-owner", UID: "external-uid",
				}}
				if owner == "shared" {
					secret.OwnerReferences = append(secret.OwnerReferences, *metav1.NewControllerRef(user, v1alpha1.GroupVersion.WithKind("KafkaUser")))
				}
			}
			require.NoError(t, manager.client.Create(ctx, certificate))
			require.NoError(t, manager.client.Create(ctx, secret))
			base := manager.client
			manager.client = interceptor.NewClient(base.(client.WithWatch), interceptor.Funcs{
				Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					if _, ok := obj.(*corev1.Secret); ok {
						err := c.Get(ctx, client.ObjectKeyFromObject(certificate), &certv1.Certificate{})
						require.True(t, apierrors.IsNotFound(err), "stop the certificate producer before deleting its Secret")
					}
					return c.Delete(ctx, obj, opts...)
				},
			})
			require.NoError(t, manager.FinalizeUserCertificate(ctx, user))
			require.True(t, apierrors.IsNotFound(base.Get(ctx, client.ObjectKeyFromObject(certificate), certificate)))
			err = base.Get(ctx, client.ObjectKeyFromObject(secret), secret)
			if owner != "generated" {
				require.NoError(t, err, "shared or externally owned Secrets must be preserved")
				require.Equal(t, "external-uid", string(secret.OwnerReferences[0].UID))
			} else {
				require.True(t, apierrors.IsNotFound(err), "generated private-key Secret must not leak")
			}
			require.NoError(t, manager.FinalizeUserCertificate(ctx, user), "cleanup must be replay-safe")
		})
	}
}

func TestFinalizeUserCertificateKeepsSecretWhileCertificateDeletes(t *testing.T) {
	ctx := context.Background()
	manager, err := newMock(newMockCluster())
	require.NoError(t, err)
	user := newMockUser()
	user.UID = "user-uid"
	owner := *metav1.NewControllerRef(user, v1alpha1.GroupVersion.WithKind("KafkaUser"))
	certificate := &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name: user.Name, Namespace: user.Namespace,
			OwnerReferences: []metav1.OwnerReference{owner},
			Finalizers:      []string{"example.com/hold"},
		},
		Spec: certv1.CertificateSpec{SecretName: user.Spec.SecretName},
	}
	secret := newMockUserSecret()
	secret.OwnerReferences = []metav1.OwnerReference{owner}
	require.NoError(t, manager.client.Create(ctx, certificate))
	require.NoError(t, manager.client.Create(ctx, secret))

	err = manager.FinalizeUserCertificate(ctx, user)
	require.True(t, errors.As(err, &errorfactory.ResourceNotReady{}), "a deleting Certificate must report not ready, got %v", err)
	require.NoError(t, manager.client.Get(ctx, client.ObjectKeyFromObject(certificate), certificate))
	require.NotNil(t, certificate.DeletionTimestamp)
	require.NoError(t, manager.client.Get(ctx, client.ObjectKeyFromObject(secret), secret),
		"the Secret must outlive its producing Certificate")

	certificate.Finalizers = nil
	require.NoError(t, manager.client.Update(ctx, certificate))
	require.NoError(t, manager.FinalizeUserCertificate(ctx, user))
	require.True(t, apierrors.IsNotFound(manager.client.Get(ctx, client.ObjectKeyFromObject(secret), secret)))
}

func TestFinalizeUserCertificateLeavesForeignCertificate(t *testing.T) {
	for _, owner := range []string{"unowned", "foreign-controller"} {
		t.Run(owner, func(t *testing.T) {
			ctx := context.Background()
			manager, err := newMock(newMockCluster())
			require.NoError(t, err)
			user := newMockUser()
			user.UID = "user-uid"
			certificate := &certv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{Name: user.Name, Namespace: user.Namespace},
				Spec:       certv1.CertificateSpec{SecretName: user.Spec.SecretName},
			}
			if owner == "foreign-controller" {
				foreign := &v1alpha1.KafkaUser{ObjectMeta: metav1.ObjectMeta{Name: "other-user", Namespace: user.Namespace, UID: "other-uid"}}
				certificate.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(foreign, v1alpha1.GroupVersion.WithKind("KafkaUser"))}
			}
			secret := newMockUserSecret()
			secret.Annotations = map[string]string{certv1.CertificateNameKey: user.Name}
			require.NoError(t, manager.client.Create(ctx, certificate))
			require.NoError(t, manager.client.Create(ctx, secret))
			wantOwners := certificate.DeepCopy().OwnerReferences

			require.NoError(t, manager.FinalizeUserCertificate(ctx, user))

			require.NoError(t, manager.client.Get(ctx, client.ObjectKeyFromObject(certificate), certificate),
				"a Certificate the user does not control must be preserved")
			require.Equal(t, wantOwners, certificate.OwnerReferences)
			require.NoError(t, manager.client.Get(ctx, client.ObjectKeyFromObject(secret), secret),
				"the foreign Certificate's Secret must be preserved")
			require.Empty(t, secret.OwnerReferences, "the foreign Certificate's Secret must not be adopted")
		})
	}
}

func TestFinalizeUserCertificateWithoutCertificateAPI(t *testing.T) {
	user := newMockUser()
	user.UID = "user-uid"
	owner := *metav1.NewControllerRef(user, v1alpha1.GroupVersion.WithKind("KafkaUser"))
	notServed := &meta.NoKindMatchError{GroupKind: certv1.SchemeGroupVersion.WithKind("Certificate").GroupKind()}
	for _, tt := range []struct {
		name       string
		lookupErr  error
		owners     []metav1.OwnerReference
		wantErr    bool
		wantSecret bool
	}{
		{name: "owned secret is deleted", lookupErr: notServed, owners: []metav1.OwnerReference{owner}},
		{name: "unowned secret is preserved", lookupErr: notServed, wantSecret: true},
		{name: "transient discovery failure retries", lookupErr: errors.New("discovery timed out"), owners: []metav1.OwnerReference{owner}, wantErr: true, wantSecret: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			manager, err := newMock(newMockCluster())
			require.NoError(t, err)
			secret := newMockUserSecret()
			secret.OwnerReferences = tt.owners
			require.NoError(t, manager.client.Create(ctx, secret))
			base := manager.client
			manager.client = interceptor.NewClient(base.(client.WithWatch), interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if _, ok := obj.(*certv1.Certificate); ok {
						return tt.lookupErr
					}
					return c.Get(ctx, key, obj, opts...)
				},
			})

			err = manager.FinalizeUserCertificate(ctx, user)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err, "an unserved Certificate API means no Certificate can still issue the Secret")
			}
			err = base.Get(ctx, client.ObjectKeyFromObject(secret), secret)
			if tt.wantSecret {
				require.NoError(t, err)
			} else {
				require.True(t, apierrors.IsNotFound(err))
			}
		})
	}
}

func TestJKSPasswordSecretIsOwnedFromCreation(t *testing.T) {
	ctx := context.Background()
	manager, err := newMock(newMockCluster())
	require.NoError(t, err)
	user := newMockUser()
	user.UID = "user-uid"
	base := manager.client
	certificateWrite := errors.New("injected Certificate write failure")
	failCertificate := true
	manager.client = interceptor.NewClient(base.(client.WithWatch), interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if _, ok := obj.(*certv1.Certificate); ok && failCertificate {
				return certificateWrite
			}
			return c.Create(ctx, obj, opts...)
		},
	})

	_, err = manager.ReconcileUserCertificate(ctx, user, scheme.Scheme, "cluster.local")
	require.ErrorIs(t, err, certificateWrite)
	secret := &corev1.Secret{}
	require.NoError(t, base.Get(ctx, client.ObjectKey{Namespace: user.Namespace, Name: user.Spec.SecretName}, secret))
	require.True(t, metav1.IsControlledBy(secret, user), "the password Secret must be owned by the user from creation")

	failCertificate = false
	_, err = manager.ReconcileUserCertificate(ctx, user, scheme.Scheme, "cluster.local")
	require.True(t, errors.As(err, &errorfactory.ResourceNotReady{}), "provisioning must resume from its own password Secret, got %v", err)
	require.NoError(t, base.Get(ctx, client.ObjectKey{Namespace: user.Namespace, Name: user.Name}, &certv1.Certificate{}))

	require.NoError(t, base.Delete(ctx, &certv1.Certificate{ObjectMeta: metav1.ObjectMeta{Namespace: user.Namespace, Name: user.Name}}))
	require.NoError(t, manager.FinalizeUserCertificate(ctx, user))
	require.True(t, apierrors.IsNotFound(base.Get(ctx, client.ObjectKeyFromObject(secret), secret)),
		"cleanup must remove the operator-created password Secret")
}

func TestJKSPasswordProvisioningLeavesForeignSecret(t *testing.T) {
	ctx := context.Background()
	manager, err := newMock(newMockCluster())
	require.NoError(t, err)
	user := newMockUser()
	user.UID = "user-uid"
	foreign := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: user.Spec.SecretName, Namespace: user.Namespace},
		Data:       map[string][]byte{v1alpha1.PasswordKey: []byte("someone-else")},
	}
	require.NoError(t, manager.client.Create(ctx, foreign))

	_, err = manager.ReconcileUserCertificate(ctx, user, scheme.Scheme, "cluster.local")
	require.Error(t, err)
	require.NoError(t, manager.client.Get(ctx, client.ObjectKeyFromObject(foreign), foreign))
	require.Empty(t, foreign.OwnerReferences, "a Secret the user does not own must not be adopted")
	require.Equal(t, "someone-else", string(foreign.Data[v1alpha1.PasswordKey]))
	require.True(t, apierrors.IsNotFound(manager.client.Get(ctx, client.ObjectKey{Namespace: user.Namespace, Name: user.Name}, &certv1.Certificate{})),
		"no Certificate may be created to write into a foreign Secret")
}

func TestFinalizeUserCertificateCleansCertificateTarget(t *testing.T) {
	for _, scenario := range []string{"renamed-secret-reference", "issued-during-deletion"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			manager, err := newMock(newMockCluster())
			require.NoError(t, err)
			user := newMockUser()
			user.UID = "user-uid"
			owner := *metav1.NewControllerRef(user, v1alpha1.GroupVersion.WithKind("KafkaUser"))
			target := user.Spec.SecretName
			if scenario == "renamed-secret-reference" {
				// The Certificate still writes the Secret named before spec.secretName changed.
				target = "original-secret"
			}
			certificate := &certv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{Name: user.Name, Namespace: user.Namespace, OwnerReferences: []metav1.OwnerReference{owner}},
				Spec:       certv1.CertificateSpec{SecretName: target},
			}
			require.NoError(t, manager.client.Create(ctx, certificate))
			issued := newMockUserSecret()
			issued.Name = target
			issued.Annotations = map[string]string{certv1.CertificateNameKey: user.Name}
			if scenario == "renamed-secret-reference" {
				require.NoError(t, manager.client.Create(ctx, issued))
			}
			base := manager.client
			manager.client = interceptor.NewClient(base.(client.WithWatch), interceptor.Funcs{
				Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					if _, ok := obj.(*certv1.Certificate); ok && scenario == "issued-during-deletion" {
						// An issuance already in flight publishes the Secret without an owner.
						require.NoError(t, c.Create(ctx, issued.DeepCopy()))
					}
					return c.Delete(ctx, obj, opts...)
				},
			})

			require.NoError(t, manager.FinalizeUserCertificate(ctx, user))
			require.True(t, apierrors.IsNotFound(base.Get(ctx, client.ObjectKeyFromObject(certificate), certificate)))
			require.True(t, apierrors.IsNotFound(base.Get(ctx, client.ObjectKey{Namespace: user.Namespace, Name: target}, &corev1.Secret{})),
				"the private-key Secret issued by the user's Certificate must not leak")
		})
	}
}

func TestFinalizeUserCertificateAdoptsOnlyAnnotatedSecret(t *testing.T) {
	for name, annotations := range map[string]map[string]string{
		"missing annotation": nil,
		"other certificate":  {certv1.CertificateNameKey: "other-certificate"},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			manager, err := newMock(newMockCluster())
			require.NoError(t, err)
			user := newMockUser()
			user.UID = "user-uid"
			certificate := &certv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name: user.Name, Namespace: user.Namespace,
					OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(user, v1alpha1.GroupVersion.WithKind("KafkaUser"))},
				},
				Spec: certv1.CertificateSpec{SecretName: user.Spec.SecretName},
			}
			secret := newMockUserSecret()
			secret.Annotations = annotations
			require.NoError(t, manager.client.Create(ctx, certificate))
			require.NoError(t, manager.client.Create(ctx, secret))

			require.NoError(t, manager.FinalizeUserCertificate(ctx, user))
			require.True(t, apierrors.IsNotFound(manager.client.Get(ctx, client.ObjectKeyFromObject(certificate), certificate)))
			require.NoError(t, manager.client.Get(ctx, client.ObjectKeyFromObject(secret), secret),
				"an ownerless Secret not issued for this Certificate must be preserved")
			require.Empty(t, secret.OwnerReferences)
		})
	}
}

func TestFinalizeUserCertificateDeletesAreConditional(t *testing.T) {
	ctx := context.Background()
	manager, err := newMock(newMockCluster())
	require.NoError(t, err)
	user := newMockUser()
	user.UID = "user-uid"
	owner := *metav1.NewControllerRef(user, v1alpha1.GroupVersion.WithKind("KafkaUser"))
	certificate := &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{Name: user.Name, Namespace: user.Namespace, UID: "certificate-uid", OwnerReferences: []metav1.OwnerReference{owner}},
		Spec:       certv1.CertificateSpec{SecretName: user.Spec.SecretName},
	}
	secret := newMockUserSecret()
	secret.UID = "secret-uid"
	secret.OwnerReferences = []metav1.OwnerReference{owner}
	require.NoError(t, manager.client.Create(ctx, certificate))
	require.NoError(t, manager.client.Create(ctx, secret))
	base := manager.client
	changeSecretBeforeDelete := true
	manager.client = interceptor.NewClient(base.(client.WithWatch), interceptor.Funcs{
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			deleteOptions := &client.DeleteOptions{}
			deleteOptions.ApplyOptions(opts)
			require.NotNil(t, deleteOptions.Preconditions, "deletes must be conditional on the inspected object")
			require.Equal(t, obj.GetUID(), *deleteOptions.Preconditions.UID)
			require.NotEmpty(t, obj.GetUID())
			stored := obj.DeepCopyObject().(client.Object)
			require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(obj), stored))
			require.Equal(t, stored.GetResourceVersion(), *deleteOptions.Preconditions.ResourceVersion)
			if s, ok := obj.(*corev1.Secret); ok && changeSecretBeforeDelete {
				changeSecretBeforeDelete = false
				changed := s.DeepCopy()
				changed.Labels = map[string]string{"changed": "after-inspection"}
				require.NoError(t, c.Update(ctx, changed))
			}
			return c.Delete(ctx, obj, opts...)
		},
	})

	err = manager.FinalizeUserCertificate(ctx, user)
	require.True(t, apierrors.IsConflict(err), "a Secret changed after inspection must not be deleted, got %v", err)
	require.NoError(t, base.Get(ctx, client.ObjectKeyFromObject(secret), secret), "the Secret must remain for a retry")
	require.NoError(t, manager.FinalizeUserCertificate(ctx, user))
	require.True(t, apierrors.IsNotFound(base.Get(ctx, client.ObjectKeyFromObject(secret), secret)))
}
