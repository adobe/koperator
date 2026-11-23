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
	"fmt"

	"github.com/IBM/sarama"

	"github.com/banzaicloud/koperator/api/v1alpha1"
	"github.com/banzaicloud/koperator/pkg/errorfactory"
)

// AclPatternTypeMapping maps patternType from v1alpha1.KafkaPatternType to sarama.AclResourcePatternType
func AclPatternTypeMapping(patternType v1alpha1.KafkaPatternType) sarama.AclResourcePatternType {
	switch patternType {
	case v1alpha1.KafkaPatternTypeAny:
		return sarama.AclPatternAny
	case v1alpha1.KafkaPatternTypeMatch:
		return sarama.AclPatternMatch
	case v1alpha1.KafkaPatternTypeLiteral:
		return sarama.AclPatternLiteral
	case v1alpha1.KafkaPatternTypePrefixed:
		return sarama.AclPatternPrefixed
	default:
		return sarama.AclPatternUnknown
	}
}

// CreateUserACLs creates Kafka ACLs for the given access type and user
// `literal` patternType will be used if patternType == ""
func (k *kafkaClient) CreateUserACLs(accessType v1alpha1.KafkaAccessType, patternType v1alpha1.KafkaPatternType, dn string, topic string) (err error) {
	bindings, err := userACLBindings(accessType, patternType, dn, topic)
	if err != nil {
		return err
	}
	return k.admin.CreateACLs(bindings)
}

func (k *kafkaClient) ListUserACLs() ([]sarama.ResourceAcls, error) {
	acls, err := k.admin.ListAcls(sarama.AclFilter{})
	if err != nil {
		return nil, err
	}
	return acls, nil
}

// DeleteUserACLs removes the ACL bindings that CreateUserACLs creates for the same
// access type, pattern type, user and topic. Other bindings of the user are kept.
func (k *kafkaClient) DeleteUserACLs(accessType v1alpha1.KafkaAccessType, patternType v1alpha1.KafkaPatternType, dn string, topic string) error {
	bindings, err := userACLBindings(accessType, patternType, dn, topic)
	if err != nil {
		return err
	}
	for _, binding := range bindings {
		for _, acl := range binding.Acls {
			matches, err := k.admin.DeleteACL(sarama.AclFilter{
				ResourceType:              binding.ResourceType,
				ResourceName:              &binding.ResourceName,
				ResourcePatternTypeFilter: binding.ResourcePatternType,
				Principal:                 &acl.Principal,
				Host:                      &acl.Host,
				Operation:                 acl.Operation,
				PermissionType:            acl.PermissionType,
			}, false)
			if err != nil {
				return err
			}
			for _, x := range matches {
				if x.Err != sarama.ErrNoError {
					return x.Err
				}
			}
		}
	}
	return nil
}

// userACLBindings returns the ACL bindings that grant a user the given access to a topic.
func userACLBindings(accessType v1alpha1.KafkaAccessType, patternType v1alpha1.KafkaPatternType, dn string, topic string) ([]*sarama.ResourceAcls, error) {
	principal := fmt.Sprintf("User:%s", dn)
	if patternType == "" {
		patternType = v1alpha1.KafkaPatternTypeDefault
	}
	aclPatternType := AclPatternTypeMapping(patternType)
	if aclPatternType == sarama.AclPatternUnknown {
		return nil, errorfactory.New(errorfactory.InternalError{}, fmt.Errorf("unknown type: %s", patternType), "unrecognized pattern type")
	}
	allow := func(operations ...sarama.AclOperation) []*sarama.Acl {
		acls := make([]*sarama.Acl, 0, len(operations))
		for _, operation := range operations {
			acls = append(acls, &sarama.Acl{Principal: principal, Host: "*", Operation: operation, PermissionType: sarama.AclPermissionAllow})
		}
		return acls
	}
	topicResource := sarama.Resource{ResourceType: sarama.AclResourceTopic, ResourceName: topic, ResourcePatternType: aclPatternType}
	bindings := []*sarama.ResourceAcls{
		{Resource: topicResource, Acls: allow(sarama.AclOperationDescribe, sarama.AclOperationDescribeConfigs)},
	}
	switch accessType {
	case v1alpha1.KafkaAccessTypeRead:
		bindings = append(bindings,
			&sarama.ResourceAcls{Resource: topicResource, Acls: allow(sarama.AclOperationRead)},
			&sarama.ResourceAcls{
				Resource: sarama.Resource{ResourceType: sarama.AclResourceGroup, ResourceName: "*", ResourcePatternType: sarama.AclPatternLiteral},
				Acls:     allow(sarama.AclOperationRead),
			})
	case v1alpha1.KafkaAccessTypeWrite:
		bindings = append(bindings, &sarama.ResourceAcls{Resource: topicResource, Acls: allow(sarama.AclOperationWrite, sarama.AclOperationCreate)})
	default:
		return nil, errorfactory.New(errorfactory.InternalError{}, fmt.Errorf("unknown type: %s", accessType), "unrecognized access type")
	}
	return bindings, nil
}
