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

	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/banzaicloud/koperator/api/v1alpha1"
	"github.com/banzaicloud/koperator/pkg/errorfactory"
)

type ACL struct {
	ResourceType string
	ResourceName string
	PatternType  string
	Principal    string
	Host         string
	Operation    string
	Permission   string
}

func aclPatternType(patternType v1alpha1.KafkaPatternType) kmsg.ACLResourcePatternType {
	switch patternType {
	case v1alpha1.KafkaPatternTypeAny:
		return kmsg.ACLResourcePatternTypeAny
	case v1alpha1.KafkaPatternTypeMatch:
		return kmsg.ACLResourcePatternTypeMatch
	case "", v1alpha1.KafkaPatternTypeLiteral:
		return kmsg.ACLResourcePatternTypeLiteral
	case v1alpha1.KafkaPatternTypePrefixed:
		return kmsg.ACLResourcePatternTypePrefixed
	default:
		return kmsg.ACLResourcePatternTypeUnknown
	}
}

// CreateUserACLs creates Kafka ACLs for the given access type and user
// `literal` patternType will be used if patternType == ""
func (k *kafkaClient) CreateUserACLs(accessType v1alpha1.KafkaAccessType, patternType v1alpha1.KafkaPatternType, dn string, topic string) error {
	common, specific, err := userACLs(accessType, patternType, dn, topic)
	if err != nil {
		return err
	}
	if err := k.createACLs(common); err != nil {
		return err
	}
	return k.createACLs(specific)
}

func (k *kafkaClient) createACLs(bindings []kmsg.CreateACLsRequestCreation) error {
	ctx, cancel := k.requestContext()
	defer cancel()
	req := kmsg.NewPtrCreateACLsRequest()
	req.Creations = bindings
	resp, err := req.RequestWith(ctx, k.client)
	if err != nil {
		return err
	}
	if len(resp.Results) != len(bindings) {
		return fmt.Errorf("received %d results for %d ACL creations", len(resp.Results), len(bindings))
	}
	for _, result := range resp.Results {
		if err := brokerResponseError(result.ErrorCode, result.ErrorMessage); err != nil {
			return err
		}
	}
	return nil
}

func (k *kafkaClient) ListUserACLs() ([]ACL, error) {
	ctx, cancel := k.requestContext()
	defer cancel()
	// Preserve the legacy empty filter (UNKNOWN fields).
	req := kmsg.NewPtrDescribeACLsRequest()
	req.ResourcePatternType = kmsg.ACLResourcePatternTypeUnknown
	resp, err := req.RequestWith(ctx, k.client)
	if err != nil {
		return nil, err
	}
	if err := brokerResponseError(resp.ErrorCode, resp.ErrorMessage); err != nil {
		return nil, err
	}
	var acls []ACL
	for _, resource := range resp.Resources {
		for _, binding := range resource.ACLs {
			acls = append(acls, ACL{
				ResourceType: resource.ResourceType.String(), ResourceName: resource.ResourceName, PatternType: resource.ResourcePatternType.String(),
				Principal: binding.Principal, Host: binding.Host, Operation: binding.Operation.String(), Permission: binding.PermissionType.String(),
			})
		}
	}
	return acls, nil
}

// DeleteUserACLs sends the existing topic DELETE filter for the given principal.
func (k *kafkaClient) DeleteUserACLs(dn string, patternType v1alpha1.KafkaPatternType) error {
	ctx, cancel := k.requestContext()
	defer cancel()
	req := kmsg.NewPtrDeleteACLsRequest()
	req.Filters = []kmsg.DeleteACLsRequestFilter{{
		Principal:           &dn,
		ResourcePatternType: aclPatternType(patternType),
		Operation:           kmsg.ACLOperationDelete,
		ResourceType:        kmsg.ACLResourceTypeTopic,
		PermissionType:      kmsg.ACLPermissionTypeAny,
	}}
	resp, err := req.RequestWith(ctx, k.client)
	if err != nil {
		return err
	}
	if len(resp.Results) != 1 {
		return fmt.Errorf("received %d results for one ACL delete filter", len(resp.Results))
	}
	result := resp.Results[0]
	if err := brokerResponseError(result.ErrorCode, result.ErrorMessage); err != nil {
		return err
	}
	for _, match := range result.MatchingACLs {
		if err := brokerResponseError(match.ErrorCode, match.ErrorMessage); err != nil {
			return err
		}
	}
	return nil
}

func userACLs(access v1alpha1.KafkaAccessType, pattern v1alpha1.KafkaPatternType, dn, topic string) (common, specific []kmsg.CreateACLsRequestCreation, err error) {
	patternType := aclPatternType(pattern)
	if patternType == kmsg.ACLResourcePatternTypeUnknown {
		return nil, nil, errorfactory.New(errorfactory.InternalError{}, fmt.Errorf("unknown type: %s", pattern), "unrecognized pattern type")
	}
	grant := kmsg.CreateACLsRequestCreation{
		ResourceType: kmsg.ACLResourceTypeTopic, ResourceName: topic, ResourcePatternType: patternType,
		Principal: "User:" + dn, Host: "*", PermissionType: kmsg.ACLPermissionTypeAllow,
	}
	grant.Operation = kmsg.ACLOperationDescribe
	common = append(common, grant)
	grant.Operation = kmsg.ACLOperationDescribeConfigs
	common = append(common, grant)
	switch access {
	case v1alpha1.KafkaAccessTypeRead:
		grant.Operation = kmsg.ACLOperationRead
		specific = append(specific, grant)
		grant.ResourceType, grant.ResourceName, grant.ResourcePatternType = kmsg.ACLResourceTypeGroup, "*", kmsg.ACLResourcePatternTypeLiteral
	case v1alpha1.KafkaAccessTypeWrite:
		grant.Operation = kmsg.ACLOperationWrite
		specific = append(specific, grant)
		grant.Operation = kmsg.ACLOperationCreate
	default:
		return nil, nil, errorfactory.New(errorfactory.InternalError{}, fmt.Errorf("unknown type: %s", access), "unrecognized access type")
	}
	return common, append(specific, grant), nil
}
