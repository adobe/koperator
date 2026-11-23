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
	"encoding/base64"
	"encoding/pem"
	"errors"
	"strings"
	"testing"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/banzaicloud/koperator/api/v1alpha1"
	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/kafkaclient"
	"github.com/banzaicloud/koperator/pkg/util"
	certutil "github.com/banzaicloud/koperator/pkg/util/cert"
	kafkautil "github.com/banzaicloud/koperator/pkg/util/kafka"
)

// forgeableACLPrincipalAnnotation is user-writable metadata that earlier revisions trusted for ACL cleanup.
const forgeableACLPrincipalAnnotation = "kafka.banzaicloud.io/acl-principal"

func TestUserACLCleanupIgnoresUserEditableMetadata(t *testing.T) {
	certificatePEM, _, _, err := certutil.GenerateTestCert()
	require.NoError(t, err)
	// Subject of the fixture returned by certutil.GenerateTestCert.
	const issued = "CN=test-cn,O=test-ou"
	const victim = "User:CN=victim"

	for _, tt := range []struct {
		name        string
		recorded    *string
		forged      string
		secret      bool
		wantDeleted []string
		wantBlocked bool
	}{
		{name: "recorded principal ignores forged principal", recorded: util.StringPointer(issued), forged: victim, wantDeleted: []string{issued}},
		{name: "recorded principal ignores forged empty marker", recorded: util.StringPointer(issued), forged: "", wantDeleted: []string{issued}},
		{name: "legacy user ignores forged principal", forged: victim, secret: true, wantDeleted: []string{issued}},
		{name: "legacy user ignores forged empty marker", forged: "", wantBlocked: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := userDirectReadScheme(t)
			user := &v1alpha1.KafkaUser{
				ObjectMeta: metav1.ObjectMeta{
					Name: "tampered-user", Namespace: "default", UID: "tampered-user-uid",
					Annotations: map[string]string{forgeableACLPrincipalAnnotation: tt.forged},
					Finalizers:  []string{userFinalizer, "example.com/unrelated"}, DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
				},
				Spec: v1alpha1.KafkaUserSpec{
					SecretName: "tampered-secret", ClusterRef: v1alpha1.ClusterReference{Name: "cluster"},
					TopicGrants: []v1alpha1.UserTopicGrant{{TopicName: "topic", AccessType: v1alpha1.KafkaAccessTypeRead}},
				},
				Status: v1alpha1.KafkaUserStatus{State: v1alpha1.UserStateCreated, ACLPrincipal: tt.recorded},
			}
			cluster := &v1beta1.KafkaCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default"}}
			objects := []client.Object{user, cluster}
			if tt.secret {
				objects = append(objects, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: user.Spec.SecretName, Namespace: user.Namespace, OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(user, v1alpha1.GroupVersion.WithKind("KafkaUser"))}},
					Data:       map[string][]byte{corev1.TLSCertKey: certificatePEM},
				})
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithStatusSubresource(user).Build()
			recorder := recordDeletedUsers(t, c, cluster)
			req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(user)}

			_, err := (&KafkaUserReconciler{Client: c, DirectClient: c, Scheme: scheme}).Reconcile(ctx, req)
			require.NoError(t, c.Get(ctx, req.NamespacedName, user))
			if tt.wantBlocked {
				require.Error(t, err, "an unknown identity must block deletion")
				require.Contains(t, user.Finalizers, userFinalizer, "an unknown identity must not bypass ACL cleanup")
				require.Empty(t, recorder.deletedUsers)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantDeleted, recorder.deletedUsers, "ACL cleanup must use the operator-recorded identity")
			require.Equal(t, []string{"example.com/unrelated"}, user.Finalizers)
		})
	}
}

func TestBlockedUserDeletionExplainsACLIdentity(t *testing.T) {
	const keyMaterial = "PRIVATE-KEY-MATERIAL-MUST-NOT-BE-LOGGED"
	for _, scenario := range []string{"missing-secret", "invalid-certificate"} {
		t.Run(scenario, func(t *testing.T) {
			scheme := userDirectReadScheme(t)
			var logs strings.Builder
			ctx := logr.NewContext(context.Background(), funcr.New(func(prefix, args string) {
				logs.WriteString(prefix + " " + args + "\n")
			}, funcr.Options{Verbosity: 10}))
			user := &v1alpha1.KafkaUser{
				ObjectMeta: metav1.ObjectMeta{
					Name: "blocked-user", Namespace: "default",
					Finalizers: []string{userFinalizer}, DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
				},
				Spec: v1alpha1.KafkaUserSpec{
					SecretName: "blocked-secret", ClusterRef: v1alpha1.ClusterReference{Name: "cluster"},
					TopicGrants: []v1alpha1.UserTopicGrant{{TopicName: "topic", AccessType: v1alpha1.KafkaAccessTypeRead}},
				},
				Status: v1alpha1.KafkaUserStatus{State: v1alpha1.UserStateCreated},
			}
			cluster := &v1beta1.KafkaCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default"}}
			objects := []client.Object{user, cluster}
			if scenario == "invalid-certificate" {
				misplacedKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte(keyMaterial)})
				objects = append(objects, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: user.Spec.SecretName, Namespace: user.Namespace},
					Data:       map[string][]byte{corev1.TLSCertKey: misplacedKey, corev1.TLSPrivateKeyKey: []byte(keyMaterial)},
				})
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithStatusSubresource(user).Build()
			recorder := recordDeletedUsers(t, c, cluster)
			req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(user)}

			_, err := (&KafkaUserReconciler{Client: c, DirectClient: c, Scheme: scheme}).Reconcile(ctx, req)
			require.Error(t, err)
			require.Contains(t, err.Error(), "ACL cleanup principal")
			require.Contains(t, err.Error(), user.Spec.SecretName)
			require.Contains(t, logs.String(), "KafkaUser deletion blocked")
			require.Contains(t, logs.String(), user.Spec.SecretName)
			for _, output := range []string{err.Error(), logs.String()} {
				require.NotContains(t, output, keyMaterial)
				require.NotContains(t, output, base64.StdEncoding.EncodeToString([]byte(keyMaterial)))
			}
			require.NoError(t, c.Get(ctx, req.NamespacedName, user))
			require.Contains(t, user.Finalizers, userFinalizer)
			require.Empty(t, recorder.deletedUsers)
		})
	}
}

func TestUserRecordedEmptyACLPrincipalSkipsKafka(t *testing.T) {
	ctx := context.Background()
	scheme := userDirectReadScheme(t)
	user := &v1alpha1.KafkaUser{
		ObjectMeta: metav1.ObjectMeta{
			Name: "never-submitted", Namespace: "default",
			Finalizers: []string{userFinalizer, "example.com/unrelated"}, DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
		},
		Spec: v1alpha1.KafkaUserSpec{
			SecretName: "never-submitted", ClusterRef: v1alpha1.ClusterReference{Name: "cluster"},
			TopicGrants: []v1alpha1.UserTopicGrant{{TopicName: "topic", AccessType: v1alpha1.KafkaAccessTypeRead}},
		},
		Status: v1alpha1.KafkaUserStatus{ACLPrincipal: util.StringPointer("")},
	}
	cluster := &v1beta1.KafkaCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(user, cluster).WithStatusSubresource(user).Build()
	SetNewKafkaFromCluster(func(client.Client, *v1beta1.KafkaCluster) (kafkaclient.KafkaClient, func(), error) {
		t.Fatal("a recorded empty identity means no ACLs were submitted")
		return nil, nil, nil
	})
	t.Cleanup(func() { SetNewKafkaFromCluster(kafkaclient.NewFromCluster) })
	req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(user)}

	_, err := (&KafkaUserReconciler{Client: c, DirectClient: c, Scheme: scheme}).Reconcile(ctx, req)
	require.NoError(t, err)
	require.NoError(t, c.Get(ctx, req.NamespacedName, user))
	require.Equal(t, []string{"example.com/unrelated"}, user.Finalizers)
}

func TestUserACLPrincipalIsRecordedInStatusBeforeACLCreation(t *testing.T) {
	ctx := context.Background()
	scheme := userDirectReadScheme(t)
	cluster := &v1beta1.KafkaCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default"}}
	user := &v1alpha1.KafkaUser{
		ObjectMeta: metav1.ObjectMeta{
			Name: "recorded-user", Namespace: "default",
			Annotations: map[string]string{forgeableACLPrincipalAnnotation: "User:CN=victim"},
		},
		Spec: v1alpha1.KafkaUserSpec{
			SecretName: "recorded-secret", ClusterRef: v1alpha1.ClusterReference{Name: cluster.Name},
			CreateCert:  util.BoolPointer(false),
			TopicGrants: []v1alpha1.UserTopicGrant{{TopicName: "topic", AccessType: v1alpha1.KafkaAccessTypeRead}},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(user, cluster).WithStatusSubresource(user).Build()
	req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(user)}
	broker, closeBroker, err := kafkaclient.NewMockFromCluster(c, cluster)
	require.NoError(t, err)
	t.Cleanup(closeBroker)
	recorder := &protectedKafkaClient{KafkaClient: broker, beforeMutation: func() {
		persisted := &v1alpha1.KafkaUser{}
		require.NoError(t, c.Get(ctx, req.NamespacedName, persisted))
		require.NotNil(t, persisted.Status.ACLPrincipal, "ACL creation must follow a durable operator-owned identity record")
		require.Equal(t, "CN=recorded-user", *persisted.Status.ACLPrincipal)
	}}
	SetNewKafkaFromCluster(func(client.Client, *v1beta1.KafkaCluster) (kafkaclient.KafkaClient, func(), error) {
		return recorder, func() {}, nil
	})
	t.Cleanup(func() { SetNewKafkaFromCluster(kafkaclient.NewFromCluster) })

	_, err = (&KafkaUserReconciler{Client: c, DirectClient: c, Scheme: scheme}).Reconcile(ctx, req)
	require.NoError(t, err)
	require.NoError(t, c.Get(ctx, req.NamespacedName, user))
	require.Equal(t, v1alpha1.UserStateCreated, user.Status.State)
	require.NotEmpty(t, user.Status.ACLs)
	require.Equal(t, "CN=recorded-user", *user.Status.ACLPrincipal, "the final status write must keep the identity record")
	require.Equal(t, "User:CN=victim", user.Annotations[forgeableACLPrincipalAnnotation], "user metadata is neither trusted nor rewritten")

	require.NoError(t, c.Delete(ctx, user))
	_, err = (&KafkaUserReconciler{Client: c, DirectClient: c, Scheme: scheme}).Reconcile(ctx, req)
	require.NoError(t, err)
	require.True(t, apierrors.IsNotFound(c.Get(ctx, req.NamespacedName, user)))
	require.Equal(t, []string{"CN=recorded-user"}, recorder.deletedUsers)
}

func userACLsOf(t *testing.T, broker kafkaclient.KafkaClient, principal string) int {
	t.Helper()
	acls, err := broker.ListUserACLs()
	require.NoError(t, err)
	count := 0
	for _, resourceAcls := range acls {
		for _, acl := range resourceAcls.Acls {
			if acl.Principal == "User:"+principal {
				count++
			}
		}
	}
	return count
}

func TestUserDeletionUsesControllerWrittenACLStatus(t *testing.T) {
	const previous, current = "CN=previous,O=org", "CN=current,O=org"
	grant := v1alpha1.UserTopicGrant{TopicName: "topic", AccessType: v1alpha1.KafkaAccessTypeRead, PatternType: v1alpha1.KafkaPatternTypeLiteral}
	for _, tt := range []struct {
		name     string
		recorded *string
		statusOf string
		revoked  []string
	}{
		// Written by an older operator; its certificate Secret is already gone.
		{name: "legacy user without its Secret", statusOf: previous, revoked: []string{previous}},
		// A newer principal was recorded but its ACLs were never confirmed.
		{name: "previous principal is still revoked", recorded: util.StringPointer(current), statusOf: previous, revoked: []string{current, previous}},
		// An empty record cannot hide ACLs that status shows were created.
		{name: "empty record with created ACLs", recorded: util.StringPointer(""), statusOf: previous, revoked: []string{previous}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := userDirectReadScheme(t)
			user := &v1alpha1.KafkaUser{
				ObjectMeta: metav1.ObjectMeta{
					Name: "status-user", Namespace: "default", UID: "status-user-uid",
					Finalizers: []string{userFinalizer, "example.com/unrelated"}, DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
				},
				Spec: v1alpha1.KafkaUserSpec{
					SecretName: "deleted-secret", ClusterRef: v1alpha1.ClusterReference{Name: "cluster"},
					TopicGrants: []v1alpha1.UserTopicGrant{grant},
				},
				Status: v1alpha1.KafkaUserStatus{
					State: v1alpha1.UserStateCreated, ACLPrincipal: tt.recorded,
					ACLs: kafkautil.GrantsToACLStrings(tt.statusOf, []v1alpha1.UserTopicGrant{grant}),
				},
			}
			cluster := &v1beta1.KafkaCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default"}}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(user, cluster).WithStatusSubresource(user).Build()
			recorder := recordDeletedUsers(t, c, cluster)
			require.NoError(t, recorder.KafkaClient.CreateUserACLs(grant.AccessType, grant.PatternType, previous, grant.TopicName))
			req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(user)}

			_, err := (&KafkaUserReconciler{Client: c, DirectClient: c, Scheme: scheme}).Reconcile(ctx, req)
			require.NoError(t, err)
			require.ElementsMatch(t, tt.revoked, recorder.deletedUsers)
			require.Zero(t, userACLsOf(t, recorder.KafkaClient, previous), "ACLs recorded in status must be revoked")
			require.NoError(t, c.Get(ctx, req.NamespacedName, user))
			require.Equal(t, []string{"example.com/unrelated"}, user.Finalizers)
		})
	}
}

func TestBlockedLegacyDeletionResumesAfterCertificateRestore(t *testing.T) {
	ctx := context.Background()
	scheme := userDirectReadScheme(t)
	certificatePEM, _, _, err := certutil.GenerateTestCert()
	require.NoError(t, err)
	// Subject of the fixture returned by certutil.GenerateTestCert.
	const principal = "CN=test-cn,O=test-ou"
	grant := v1alpha1.UserTopicGrant{TopicName: "topic", AccessType: v1alpha1.KafkaAccessTypeWrite}
	user := &v1alpha1.KafkaUser{
		ObjectMeta: metav1.ObjectMeta{
			Name: "legacy-user", Namespace: "default", UID: "legacy-user-uid",
			Finalizers: []string{userFinalizer, "example.com/unrelated"}, DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
		},
		Spec: v1alpha1.KafkaUserSpec{
			SecretName: "legacy-secret", ClusterRef: v1alpha1.ClusterReference{Name: "cluster"},
			TopicGrants: []v1alpha1.UserTopicGrant{grant},
		},
	}
	cluster := &v1beta1.KafkaCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(user, cluster).WithStatusSubresource(user).Build()
	recorder := recordDeletedUsers(t, c, cluster)
	require.NoError(t, recorder.KafkaClient.CreateUserACLs(grant.AccessType, grant.PatternType, principal, grant.TopicName))
	r := &KafkaUserReconciler{Client: c, DirectClient: c, Scheme: scheme}
	req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(user)}

	_, err = r.Reconcile(ctx, req)
	require.Error(t, err, "an unknown legacy identity must block deletion")
	require.NoError(t, c.Get(ctx, req.NamespacedName, user))
	require.Contains(t, user.Finalizers, userFinalizer)
	require.Positive(t, userACLsOf(t, recorder.KafkaClient, principal), "blocked deletion must leave the ACLs for a retry")

	require.NoError(t, c.Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: user.Spec.SecretName, Namespace: user.Namespace,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(user, v1alpha1.GroupVersion.WithKind("KafkaUser"))},
		},
		Data: map[string][]byte{corev1.TLSCertKey: certificatePEM},
	}))
	_, err = r.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Zero(t, userACLsOf(t, recorder.KafkaClient, principal))
	require.NoError(t, c.Get(ctx, req.NamespacedName, user))
	require.Equal(t, []string{"example.com/unrelated"}, user.Finalizers)
}

func TestUserIdentityComesOnlyFromItsOwnSecret(t *testing.T) {
	certificatePEM, _, _, err := certutil.GenerateTestCert()
	require.NoError(t, err)
	for _, owner := range []string{"unowned", "other-owner"} {
		t.Run(owner, func(t *testing.T) {
			ctx := context.Background()
			scheme := userDirectReadScheme(t)
			user := &v1alpha1.KafkaUser{
				ObjectMeta: metav1.ObjectMeta{Name: "curious-user", Namespace: "default", UID: "curious-user-uid"},
				Spec: v1alpha1.KafkaUserSpec{
					SecretName: "someone-elses-secret", ClusterRef: v1alpha1.ClusterReference{Name: "cluster"},
					TopicGrants: []v1alpha1.UserTopicGrant{{TopicName: "topic", AccessType: v1alpha1.KafkaAccessTypeRead}},
				},
			}
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: user.Spec.SecretName, Namespace: user.Namespace},
				Data:       map[string][]byte{corev1.TLSCertKey: certificatePEM},
			}
			if owner == "other-owner" {
				other := &v1alpha1.KafkaUser{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: user.Namespace, UID: "other-uid"}}
				secret.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(other, v1alpha1.GroupVersion.WithKind("KafkaUser"))}
			}
			// No PKI is configured, so provisioning stops after the finalizer.
			cluster := &v1beta1.KafkaCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default"}}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(user, cluster, secret).WithStatusSubresource(user).Build()
			req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(user)}

			_, err := (&KafkaUserReconciler{Client: c, DirectClient: c, Scheme: scheme}).Reconcile(ctx, req)
			require.Error(t, err)
			require.NoError(t, c.Get(ctx, req.NamespacedName, user))
			require.Contains(t, user.Finalizers, userFinalizer)
			require.NotNil(t, user.Status.ACLPrincipal)
			require.Empty(t, *user.Status.ACLPrincipal, "the subject of a Secret the user does not own must not be published")
		})
	}
}

func TestLegacyUserDeletionRejectsSecretItDoesNotOwn(t *testing.T) {
	ctx := context.Background()
	scheme := userDirectReadScheme(t)
	certificatePEM, _, _, err := certutil.GenerateTestCert()
	require.NoError(t, err)
	user := &v1alpha1.KafkaUser{
		ObjectMeta: metav1.ObjectMeta{
			Name: "legacy-user", Namespace: "default", UID: "legacy-user-uid",
			Finalizers: []string{userFinalizer}, DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
		},
		Spec: v1alpha1.KafkaUserSpec{
			SecretName: "unowned-secret", ClusterRef: v1alpha1.ClusterReference{Name: "cluster"},
			TopicGrants: []v1alpha1.UserTopicGrant{{TopicName: "topic", AccessType: v1alpha1.KafkaAccessTypeRead}},
		},
	}
	cluster := &v1beta1.KafkaCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default"}}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: user.Spec.SecretName, Namespace: user.Namespace},
		Data:       map[string][]byte{corev1.TLSCertKey: certificatePEM},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(user, cluster, secret).WithStatusSubresource(user).Build()
	recorder := recordDeletedUsers(t, c, cluster)
	req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(user)}

	_, err = (&KafkaUserReconciler{Client: c, DirectClient: c, Scheme: scheme}).Reconcile(ctx, req)
	require.Error(t, err, "a Secret the user does not own cannot prove its ACL identity")
	require.Empty(t, recorder.deletedUsers)
	require.NoError(t, c.Get(ctx, req.NamespacedName, user))
	require.Contains(t, user.Finalizers, userFinalizer)
	require.Nil(t, user.Status.ACLPrincipal)
}

func TestNewUserWithUnreadableCertificateStillProvisions(t *testing.T) {
	ctx := context.Background()
	scheme := userDirectReadScheme(t)
	user := &v1alpha1.KafkaUser{
		ObjectMeta: metav1.ObjectMeta{Name: "damaged-user", Namespace: "default", UID: "damaged-user-uid"},
		Spec: v1alpha1.KafkaUserSpec{
			SecretName: "damaged-secret", ClusterRef: v1alpha1.ClusterReference{Name: "cluster"},
			TopicGrants: []v1alpha1.UserTopicGrant{{TopicName: "topic", AccessType: v1alpha1.KafkaAccessTypeRead}},
		},
	}
	cluster := &v1beta1.KafkaCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default"},
		Spec: v1beta1.KafkaClusterSpec{ListenersConfig: v1beta1.ListenersConfig{
			SSLSecrets: &v1beta1.SSLSecrets{PKIBackend: v1beta1.PKIBackendCertManager},
		}},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: user.Spec.SecretName, Namespace: user.Namespace,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(user, v1alpha1.GroupVersion.WithKind("KafkaUser"))},
		},
		Data: map[string][]byte{corev1.TLSCertKey: []byte("not a certificate")},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(user, cluster, secret).WithStatusSubresource(user).Build()
	req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(user)}

	_, err := (&KafkaUserReconciler{Client: c, DirectClient: c, Scheme: scheme}).Reconcile(ctx, req)
	require.NoError(t, c.Get(ctx, req.NamespacedName, user))
	require.Contains(t, user.Finalizers, userFinalizer, "an unreadable certificate must not stop cleanup ownership")
	require.Nil(t, user.Status.ACLPrincipal, "an unreadable certificate does not prove that no ACLs exist")
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: user.Namespace, Name: user.Name}, &certv1.Certificate{}),
		"cert-manager must get the Certificate that can repair the Secret, reconcile error: %v", err)
}

func issuedUserFixture(t *testing.T) (*v1alpha1.KafkaUser, *v1beta1.KafkaCluster, *certv1.Certificate, *corev1.Secret) {
	t.Helper()
	certificatePEM, _, _, err := certutil.GenerateTestCert()
	require.NoError(t, err)
	user := &v1alpha1.KafkaUser{
		ObjectMeta: metav1.ObjectMeta{Name: "issued-user", Namespace: "default", UID: "issued-user-uid"},
		Spec: v1alpha1.KafkaUserSpec{
			SecretName: "issued-secret", ClusterRef: v1alpha1.ClusterReference{Name: "cluster"},
			TopicGrants: []v1alpha1.UserTopicGrant{{TopicName: "topic", AccessType: v1alpha1.KafkaAccessTypeRead}},
		},
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
		Data: map[string][]byte{
			corev1.TLSCertKey: certificatePEM, corev1.TLSPrivateKeyKey: []byte("key"), v1alpha1.CoreCACertKey: certificatePEM,
		},
	}
	return user, cluster, cert, secret
}

func TestUnreachableBrokerLeavesNoACLCleanupPending(t *testing.T) {
	ctx := context.Background()
	scheme := userDirectReadScheme(t)
	user, cluster, cert, secret := issuedUserFixture(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(user, cluster, cert).WithStatusSubresource(user).Build()
	r := &KafkaUserReconciler{Client: c, DirectClient: c, Scheme: scheme}
	req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(user)}

	// cert-manager has not issued the Secret yet.
	result, err := r.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Positive(t, result.RequeueAfter)
	require.NoError(t, c.Create(ctx, secret))

	unreachable := errors.New("injected broker connection failure")
	SetNewKafkaFromCluster(func(client.Client, *v1beta1.KafkaCluster) (kafkaclient.KafkaClient, func(), error) {
		return nil, nil, unreachable
	})
	t.Cleanup(func() { SetNewKafkaFromCluster(kafkaclient.NewFromCluster) })
	_, err = r.Reconcile(ctx, req)
	require.ErrorIs(t, err, unreachable)
	require.NoError(t, c.Get(ctx, req.NamespacedName, user))
	require.NotNil(t, user.Status.ACLPrincipal)
	require.Empty(t, *user.Status.ACLPrincipal, "no ACL was submitted, so none needs cleanup")

	require.NoError(t, c.Delete(ctx, user))
	forbidKafkaClient(t, "a user that never reached Kafka must be deletable while Kafka is unreachable")
	_, err = r.Reconcile(ctx, req)
	require.NoError(t, err)
	require.True(t, apierrors.IsNotFound(c.Get(ctx, req.NamespacedName, user)))
}

func TestFailedACLIdentityRecordStopsACLCreation(t *testing.T) {
	ctx := context.Background()
	scheme := userDirectReadScheme(t)
	user, cluster, cert, secret := issuedUserFixture(t)
	user.Finalizers = []string{userFinalizer}
	user.Labels = applyClusterRefLabel(cluster, nil)
	user.Status.ACLPrincipal = util.StringPointer("")
	// Subject of the fixture returned by certutil.GenerateTestCert.
	const principal = "CN=test-cn,O=test-ou"
	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(user, cluster, cert, secret).WithStatusSubresource(user).Build()
	recordFailure := errors.New("injected ACL identity record failure")
	failRecord := true
	c := interceptor.NewClient(base, interceptor.Funcs{
		SubResourceUpdate: func(ctx context.Context, c client.Client, subresource string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			if u, ok := obj.(*v1alpha1.KafkaUser); ok && failRecord && u.Status.ACLPrincipal != nil && *u.Status.ACLPrincipal == principal {
				return recordFailure
			}
			return c.SubResource(subresource).Update(ctx, obj, opts...)
		},
	})
	broker, closeBroker, err := kafkaclient.NewMockFromCluster(base, cluster)
	require.NoError(t, err)
	t.Cleanup(closeBroker)
	creations := 0
	useKafkaClient(t, &protectedKafkaClient{KafkaClient: broker, beforeMutation: func() { creations++ }})
	r := &KafkaUserReconciler{Client: c, DirectClient: base, Scheme: scheme}
	req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(user)}

	_, err = r.Reconcile(ctx, req)
	require.ErrorIs(t, err, recordFailure)
	require.Zero(t, creations, "no ACL may be created before its cleanup identity is durable")
	require.NoError(t, base.Get(ctx, req.NamespacedName, user))
	require.Contains(t, user.Finalizers, userFinalizer)
	require.Empty(t, *user.Status.ACLPrincipal)

	failRecord = false
	_, err = r.Reconcile(ctx, req)
	require.NoError(t, err)
	require.Positive(t, creations)
	require.NoError(t, base.Get(ctx, req.NamespacedName, user))
	require.Equal(t, principal, *user.Status.ACLPrincipal)
}
