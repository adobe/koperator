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
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
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
	req := kmsg.NewPtrCreateACLsRequest()
	req.Creations = bindings
	return k.retryACLControllerChange(func(ctx context.Context, broker *kgo.Broker) error {
		resp, err := req.RequestWith(ctx, broker)
		if err != nil {
			return err
		}
		if len(resp.Results) != len(req.Creations) {
			return fmt.Errorf("received %d results for %d ACL creations", len(resp.Results), len(req.Creations))
		}
		var rejected []kmsg.CreateACLsRequestCreation
		for i, result := range resp.Results {
			switch err := brokerResponseError(result.ErrorCode, result.ErrorMessage); {
			case err == nil:
			case errors.Is(err, kerr.NotController):
				rejected = append(rejected, req.Creations[i])
			default:
				return err
			}
		}
		if len(rejected) > 0 {
			req.Creations = rejected
			return kerr.NotController
		}
		return nil
	})
}

func (k *kafkaClient) ListUserACLs() ([]ACL, error) {
	// Preserve the legacy empty filter (UNKNOWN fields).
	req := kmsg.NewPtrDescribeACLsRequest()
	req.ResourcePatternType = kmsg.ACLResourcePatternTypeUnknown
	var resp *kmsg.DescribeACLsResponse
	if err := k.retryACLControllerChange(func(ctx context.Context, broker *kgo.Broker) error {
		response, err := req.RequestWith(ctx, broker)
		if err != nil {
			return err
		}
		resp = response
		return brokerResponseError(resp.ErrorCode, resp.ErrorMessage)
	}); err != nil {
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
	req := kmsg.NewPtrDeleteACLsRequest()
	req.Filters = []kmsg.DeleteACLsRequestFilter{{
		Principal:           &dn,
		ResourcePatternType: aclPatternType(patternType),
		Operation:           kmsg.ACLOperationDelete,
		ResourceType:        kmsg.ACLResourceTypeTopic,
		PermissionType:      kmsg.ACLPermissionTypeAny,
	}}
	var result kmsg.DeleteACLsResponseResult
	if err := k.retryACLControllerChange(func(ctx context.Context, broker *kgo.Broker) error {
		resp, err := req.RequestWith(ctx, broker)
		if err != nil {
			return err
		}
		if len(resp.Results) != 1 {
			return fmt.Errorf("received %d results for one ACL delete filter", len(resp.Results))
		}
		result = resp.Results[0]
		return brokerResponseError(result.ErrorCode, result.ErrorMessage)
	}); err != nil {
		return err
	}
	for _, match := range result.MatchingACLs {
		if err := brokerResponseError(match.ErrorCode, match.ErrorMessage); err != nil {
			return err
		}
	}
	return nil
}

func (k *kafkaClient) retryACLControllerChange(request func(context.Context, *kgo.Broker) error) error {
	const maxRetries = 5
	for attempt := 0; ; attempt++ {
		var err error
		if attempt > 0 {
			err = k.refreshACLController()
		}
		if err == nil {
			ctx, cancel := k.requestContext()
			err = request(ctx, k.client.Broker(int(k.controllerID.Load())))
			cancel()
		}
		// A dial failure happens before the write. A lost reply does not permit replay.
		if attempt == maxRetries || (!errors.Is(err, kerr.NotController) && !isBrokerDialError(err)) {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (k *kafkaClient) refreshACLController() error {
	ctx, cancel := k.requestContext()
	defer cancel()
	meta, err := brokerMetadataRequest().RequestWith(ctx, k.client)
	if err != nil {
		return err
	}
	id, err := metadataController(meta)
	if err != nil {
		return kerr.NotController
	}
	k.controllerID.Store(id)
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
