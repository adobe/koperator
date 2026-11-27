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
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/banzaicloud/koperator/pkg/errorfactory"
)

type ConfigEntry struct {
	Name  string
	Value string
}

func (k *kafkaClient) AlterClusterWideConfig(configChange map[string]*string, validateOnly bool) error {
	return k.alterConfig(kmsg.ConfigResourceTypeBroker, "", configChange, validateOnly)
}

func (k *kafkaClient) DescribeClusterWideConfig() ([]ConfigEntry, error) {
	return k.describeBrokerConfig("", nil)
}

func (k *kafkaClient) AlterPerBrokerConfig(brokerID int32, configChange map[string]*string, validateOnly bool) error {
	if err := k.requireBroker(brokerID); err != nil {
		return err
	}
	return k.alterConfig(kmsg.ConfigResourceTypeBroker, strconv.Itoa(int(brokerID)), configChange, validateOnly)
}

func (k *kafkaClient) DescribePerBrokerConfig(brokerID int32, keys []string) ([]ConfigEntry, error) {
	if err := k.requireBroker(brokerID); err != nil {
		return nil, err
	}
	return k.describeBrokerConfig(strconv.Itoa(int(brokerID)), keys)
}

func (k *kafkaClient) requireBroker(brokerID int32) error {
	if !slices.ContainsFunc(k.brokers, func(broker Broker) bool { return broker.ID == brokerID }) {
		return errorfactory.New(errorfactory.BrokersNotReady{}, errors.New("brokerNotReady"), fmt.Sprintf("could not get %d broker", brokerID))
	}
	return nil
}

func (k *kafkaClient) alterConfig(kind kmsg.ConfigResourceType, name string, changes map[string]*string, validateOnly bool) error {
	ctx, cancel := k.requestContext()
	defer cancel()
	resource := kmsg.NewAlterConfigsRequestResource()
	resource.ResourceType = kind
	resource.ResourceName = name
	for key, value := range changes {
		resource.Configs = append(resource.Configs, kmsg.AlterConfigsRequestResourceConfig{Name: key, Value: value})
	}
	req := kmsg.NewPtrAlterConfigsRequest()
	req.Resources = []kmsg.AlterConfigsRequestResource{resource}
	req.ValidateOnly = validateOnly
	// franz-go routes broker resources with an ID to that broker.
	response, err := req.RequestWith(ctx, k.client)
	if err != nil {
		return err
	}
	for _, result := range response.Resources {
		if result.ResourceType == kind && result.ResourceName == name {
			return brokerResponseError(result.ErrorCode, result.ErrorMessage)
		}
	}
	return fmt.Errorf("missing alter config response for %s %q", kind, name)
}

func (k *kafkaClient) describeBrokerConfig(name string, keys []string) ([]ConfigEntry, error) {
	ctx, cancel := k.requestContext()
	defer cancel()
	req := kmsg.NewPtrDescribeConfigsRequest()
	req.Resources = []kmsg.DescribeConfigsRequestResource{{
		ResourceType: kmsg.ConfigResourceTypeBroker,
		ResourceName: name,
		ConfigNames:  keys,
	}}
	response, err := req.RequestWith(ctx, k.client)
	if err != nil {
		return nil, err
	}
	for _, result := range response.Resources {
		if result.ResourceType != kmsg.ConfigResourceTypeBroker || result.ResourceName != name {
			continue
		}
		if err := brokerResponseError(result.ErrorCode, result.ErrorMessage); err != nil {
			return nil, err
		}
		entries := make([]ConfigEntry, 0, len(result.Configs))
		for _, config := range result.Configs {
			entry := ConfigEntry{Name: config.Name}
			if config.Value != nil {
				entry.Value = *config.Value
			}
			entries = append(entries, entry)
		}
		return entries, nil
	}
	if name == "" {
		return nil, nil
	}
	return nil, fmt.Errorf("missing describe config response for broker %s", name)
}

func brokerResponseError(code int16, message *string) error {
	err := kerr.ErrorForCode(code)
	if err != nil && message != nil && *message != "" {
		return fmt.Errorf("%s: %w", *message, err)
	}
	return err
}
