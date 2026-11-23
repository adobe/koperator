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

package kafkaclient

import (
	"testing"

	"github.com/IBM/sarama"

	"github.com/banzaicloud/koperator/api/v1alpha1"
)

func TestCreateUserACLs(t *testing.T) {
	client := newOpenedMockClient()

	validAccessTypes := []v1alpha1.KafkaAccessType{
		"read",
		"write"}
	invalidAccessTypes := []v1alpha1.KafkaAccessType{
		"",
		"helloWorld"}
	allAccessTypes := append([]v1alpha1.KafkaAccessType{}, validAccessTypes...)
	allAccessTypes = append(allAccessTypes, invalidAccessTypes...)

	validPatternTypes := []v1alpha1.KafkaPatternType{
		"any",
		"literal",
		"match",
		"prefixed",
		""}
	invalidPatternTypes := []v1alpha1.KafkaPatternType{
		"helloWorld"}
	allPatternTypes := append([]v1alpha1.KafkaPatternType{}, validPatternTypes...)
	allPatternTypes = append(allPatternTypes, invalidPatternTypes...)

	// Test all valid combinations of accessType and patternType
	for _, accessType := range validAccessTypes {
		for _, patternType := range validPatternTypes {
			if err := client.CreateUserACLs(accessType, patternType, "test-user", "test-topic"); err != nil {
				t.Error("Expected no error, got:", err)
			}
		}
	}
	// Test invalid accessTypes against all patternTypes
	for _, accessType := range invalidAccessTypes {
		for _, patternType := range allPatternTypes {
			if err := client.CreateUserACLs(accessType, patternType, "test-user", "test-topic"); err == nil {
				t.Error("Expected error, got nil")
			}
		}
	}
	// Test invalid patternTypes against all accessTypes
	for _, patternType := range invalidPatternTypes {
		for _, accessType := range allAccessTypes {
			if err := client.CreateUserACLs(accessType, patternType, "test-user", "test-topic"); err == nil {
				t.Error("Expected error, got nil")
			}
		}
	}

	client.admin, _ = newMockClusterAdminFailOps([]string{}, sarama.NewConfig())
	// Test all combinations of accessType and patternType
	for _, accessType := range allAccessTypes {
		for _, patternType := range allPatternTypes {
			if err := client.CreateUserACLs(accessType, patternType, "test-user", "test-topic"); err == nil {
				t.Error("Expected error, got nil")
			}
		}
	}
}

func TestDeleteUserACLs(t *testing.T) {
	client := newOpenedMockClient()

	if err := client.DeleteUserACLs(v1alpha1.KafkaAccessTypeRead, "", withErrorTopicName, "test-topic"); err == nil {
		t.Error("Expected error, got nil")
	}
	if err := client.DeleteUserACLs("helloWorld", "", "test-user", "test-topic"); err == nil {
		t.Error("Expected error for an unknown access type, got nil")
	}
	if err := client.DeleteUserACLs(v1alpha1.KafkaAccessTypeRead, "helloWorld", "test-user", "test-topic"); err == nil {
		t.Error("Expected error for an unknown pattern type, got nil")
	}

	client.admin, _ = newMockClusterAdminFailOps([]string{}, sarama.NewConfig())
	if err := client.DeleteUserACLs(v1alpha1.KafkaAccessTypeRead, "", "test-userr", "test-topic"); err == nil {
		t.Error("Expected error, got nil")
	}
}

func TestDeleteUserACLsRemovesOnlyTheBindingsItCreated(t *testing.T) {
	client := newOpenedMockClient()
	const user, other = "CN=user,O=org", "CN=other,O=org"
	grants := []struct {
		access  v1alpha1.KafkaAccessType
		pattern v1alpha1.KafkaPatternType
		topic   string
	}{
		{v1alpha1.KafkaAccessTypeRead, v1alpha1.KafkaPatternTypeLiteral, "orders"},
		{v1alpha1.KafkaAccessTypeWrite, v1alpha1.KafkaPatternTypePrefixed, "events-"},
	}
	for _, g := range grants {
		if err := client.CreateUserACLs(g.access, g.pattern, user, g.topic); err != nil {
			t.Fatal(err)
		}
		if err := client.CreateUserACLs(g.access, g.pattern, other, g.topic); err != nil {
			t.Fatal(err)
		}
	}
	unrelated := sarama.Resource{ResourceType: sarama.AclResourceTopic, ResourceName: "audit", ResourcePatternType: sarama.AclPatternLiteral}
	if err := client.admin.CreateACLs([]*sarama.ResourceAcls{{Resource: unrelated, Acls: []*sarama.Acl{{
		Principal: "User:" + user, Host: "*", Operation: sarama.AclOperationAlter, PermissionType: sarama.AclPermissionAllow,
	}}}}); err != nil {
		t.Fatal(err)
	}
	before := countBindings(t, client, "User:"+other)

	for _, g := range grants {
		if err := client.DeleteUserACLs(g.access, g.pattern, user, g.topic); err != nil {
			t.Fatal(err)
		}
	}

	remaining := bindingsOf(t, client, "User:"+user)
	if len(remaining) != 1 || remaining[0].resource != unrelated || remaining[0].acl.Operation != sarama.AclOperationAlter {
		t.Fatalf("expected only the unrelated binding to remain for the user, got %+v", remaining)
	}
	if after := countBindings(t, client, "User:"+other); after != before {
		t.Fatalf("another principal's bindings changed: before %d, after %d", before, after)
	}
}

type aclBinding struct {
	resource sarama.Resource
	acl      sarama.Acl
}

func bindingsOf(t *testing.T, client *kafkaClient, principal string) []aclBinding {
	t.Helper()
	acls, err := client.ListUserACLs()
	if err != nil {
		t.Fatal(err)
	}
	var out []aclBinding
	for _, resourceAcls := range acls {
		for _, acl := range resourceAcls.Acls {
			if acl.Principal == principal {
				out = append(out, aclBinding{resource: resourceAcls.Resource, acl: *acl})
			}
		}
	}
	return out
}

func countBindings(t *testing.T, client *kafkaClient, principal string) int {
	t.Helper()
	return len(bindingsOf(t, client, principal))
}
