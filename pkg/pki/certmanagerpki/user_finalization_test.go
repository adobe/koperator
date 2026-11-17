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

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/banzaicloud/koperator/api/v1alpha1"
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
