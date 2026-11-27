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
	"io"
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/banzaicloud/koperator/pkg/errorfactory"
)

func TestCreateUserACLs(t *testing.T) {
	// kfake uses the empty username for an unauthenticated connection.
	cluster := kfake.MustCluster(kfake.NumBrokers(1), kfake.EnableACLs(), kfake.Superuser("PLAIN", "", ""))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	var invalid errorfactory.InternalError
	require.ErrorAs(t, client.CreateUserACLs("invalid", "literal", "CN=reader", "test-topic"), &invalid)
	require.ErrorAs(t, client.CreateUserACLs("read", "invalid", "CN=reader", "test-topic"), &invalid)
	require.ErrorIs(t, client.CreateUserACLs("read", "any", "CN=reader", "test-topic"), kerr.InvalidRequest)
	require.ErrorIs(t, client.CreateUserACLs("read", "match", "CN=reader", "test-topic"), kerr.InvalidRequest)
	require.NoError(t, client.CreateUserACLs("read", "", "CN=reader", "test-topic"))
	require.NoError(t, client.CreateUserACLs("write", "prefixed", "CN=writer", "test-"))

	observer, err := kgo.NewClient(kgo.SeedBrokers(cluster.ListenAddrs()...))
	require.NoError(t, err)
	t.Cleanup(observer.Close)
	req := kmsg.NewPtrDescribeACLsRequest()
	req.ResourceType = kmsg.ACLResourceTypeAny
	req.ResourcePatternType = kmsg.ACLResourcePatternTypeAny
	req.Operation = kmsg.ACLOperationAny
	req.PermissionType = kmsg.ACLPermissionTypeAny
	resp, err := req.RequestWith(context.Background(), observer)
	require.NoError(t, err)
	require.Zero(t, resp.ErrorCode)
	var grants []string
	for _, resource := range resp.Resources {
		for _, binding := range resource.ACLs {
			grants = append(grants, resource.ResourceType.String()+"/"+resource.ResourceName+"/"+resource.ResourcePatternType.String()+"/"+
				binding.Principal+"/"+binding.Host+"/"+binding.PermissionType.String()+"/"+binding.Operation.String())
		}
	}
	require.ElementsMatch(t, []string{
		"TOPIC/test-topic/LITERAL/User:CN=reader/*/ALLOW/DESCRIBE",
		"TOPIC/test-topic/LITERAL/User:CN=reader/*/ALLOW/DESCRIBE_CONFIGS",
		"TOPIC/test-topic/LITERAL/User:CN=reader/*/ALLOW/READ",
		"GROUP/*/LITERAL/User:CN=reader/*/ALLOW/READ",
		"TOPIC/test-/PREFIXED/User:CN=writer/*/ALLOW/DESCRIBE",
		"TOPIC/test-/PREFIXED/User:CN=writer/*/ALLOW/DESCRIBE_CONFIGS",
		"TOPIC/test-/PREFIXED/User:CN=writer/*/ALLOW/WRITE",
		"TOPIC/test-/PREFIXED/User:CN=writer/*/ALLOW/CREATE",
	}, grants)
}

func TestDeleteUserACLsKeepsLegacyFilter(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	filters := make(chan []kmsg.DeleteACLsRequestFilter, 2)
	var requests atomic.Int32
	cluster.ControlKey(int16(kmsg.DeleteACLs), func(req kmsg.Request) (kmsg.Response, error, bool) {
		cluster.KeepControl()
		select {
		case filters <- req.(*kmsg.DeleteACLsRequest).Filters:
		default:
		}
		resp := req.ResponseKind().(*kmsg.DeleteACLsResponse)
		resp.Results = []kmsg.DeleteACLsResponseResult{{}}
		if requests.Add(1) == 1 {
			resp.Results[0].ErrorCode = kerr.NotController.Code
		}
		return resp, nil, true
	})
	require.NoError(t, client.DeleteUserACLs("CN=reader", "prefixed"))
	require.Equal(t, int32(2), requests.Load())
	want := []kmsg.DeleteACLsRequestFilter{{
		ResourceType: kmsg.ACLResourceTypeTopic, ResourcePatternType: kmsg.ACLResourcePatternTypePrefixed,
		Principal: kmsg.StringPtr("CN=reader"), Operation: kmsg.ACLOperationDelete, PermissionType: kmsg.ACLPermissionTypeAny,
	}}
	require.Equal(t, want, <-filters)
	require.Equal(t, want, <-filters)
}

func TestCreateUserACLsRetriesControllerChange(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(2))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	metadata, err := brokerMetadataRequest().RequestWith(t.Context(), client.(*kafkaClient).client)
	require.NoError(t, err)
	// kfake starts with node 1 as the controller.
	originalController := metadata.ControllerID
	require.Equal(t, int32(1), originalController)
	const newController int32 = 0
	var rejected, accepted atomic.Int32
	cluster.ControlKey(int16(kmsg.Metadata), func(req kmsg.Request) (kmsg.Response, error, bool) {
		if rejected.Load() == 0 {
			return nil, nil, false
		}
		cluster.KeepControl()
		resp := req.ResponseKind().(*kmsg.MetadataResponse)
		resp.ControllerID = newController
		resp.Brokers = metadata.Brokers
		return resp, nil, true
	})
	cluster.ControlKey(int16(kmsg.CreateACLs), func(req kmsg.Request) (kmsg.Response, error, bool) {
		cluster.KeepControl()
		resp := req.ResponseKind().(*kmsg.CreateACLsResponse)
		resp.Results = make([]kmsg.CreateACLsResponseResult, len(req.(*kmsg.CreateACLsRequest).Creations))
		if cluster.CurrentNode() == originalController {
			rejected.Add(1)
			for i := range resp.Results {
				resp.Results[i].ErrorCode = kerr.NotController.Code
			}
		} else {
			accepted.Add(1)
		}
		return resp, nil, true
	})
	require.NoError(t, client.CreateUserACLs("read", "literal", "CN=reader", "test-topic"))
	require.Equal(t, int32(1), rejected.Load())
	require.Equal(t, int32(2), accepted.Load())
}

func TestCreateUserACLsReusesStartupController(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	var metadataReads atomic.Int32
	cluster.ControlKey(int16(kmsg.Metadata), func(kmsg.Request) (kmsg.Response, error, bool) {
		metadataReads.Add(1)
		return nil, nil, false
	})
	cluster.ControlKey(int16(kmsg.CreateACLs), func(req kmsg.Request) (kmsg.Response, error, bool) {
		cluster.KeepControl()
		resp := req.ResponseKind().(*kmsg.CreateACLsResponse)
		resp.Results = make([]kmsg.CreateACLsResponseResult, len(req.(*kmsg.CreateACLsRequest).Creations))
		return resp, nil, true
	})
	require.NoError(t, client.CreateUserACLs("read", "literal", "CN=reader", "test-topic"))
	require.Zero(t, metadataReads.Load())
}

func TestACLWriteRefreshesControllerAfterDialFailure(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster).(*kafkaClient)
	var metadataReads atomic.Int32
	cluster.ControlKey(int16(kmsg.Metadata), func(kmsg.Request) (kmsg.Response, error, bool) {
		metadataReads.Add(1)
		return nil, nil, false
	})
	attempts := 0
	err := client.retryACLControllerChange(func(context.Context, *kgo.Broker) error {
		attempts++
		if attempts == 1 {
			return &net.OpError{Op: "dial", Err: net.ErrClosed}
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, attempts)
	require.Equal(t, int32(1), metadataReads.Load())
}

func TestListUserACLsBoundsControllerRetries(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	var requests atomic.Int32
	cluster.ControlKey(int16(kmsg.DescribeACLs), func(req kmsg.Request) (kmsg.Response, error, bool) {
		cluster.KeepControl()
		requests.Add(1)
		resp := req.ResponseKind().(*kmsg.DescribeACLsResponse)
		resp.ErrorCode = kerr.NotController.Code
		return resp, nil, true
	})
	_, err := client.ListUserACLs()
	require.ErrorIs(t, err, kerr.NotController)
	require.Equal(t, int32(6), requests.Load())
}

func TestListUserACLsFlattensBrokerResponse(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	cluster.ControlKey(int16(kmsg.DescribeACLs), func(req kmsg.Request) (kmsg.Response, error, bool) {
		resp := req.ResponseKind().(*kmsg.DescribeACLsResponse)
		resp.Resources = []kmsg.DescribeACLsResponseResource{
			{
				ResourceType: kmsg.ACLResourceTypeTopic, ResourceName: "events-",
				ResourcePatternType: kmsg.ACLResourcePatternTypePrefixed,
				ACLs: []kmsg.DescribeACLsResponseResourceACL{
					{Principal: "User:reader", Host: "*", Operation: kmsg.ACLOperationRead, PermissionType: kmsg.ACLPermissionTypeAllow},
					{Principal: "User:writer", Host: "*", Operation: kmsg.ACLOperationWrite, PermissionType: kmsg.ACLPermissionTypeDeny},
				},
			},
			{
				ResourceType: kmsg.ACLResourceTypeGroup, ResourceName: "readers",
				ResourcePatternType: kmsg.ACLResourcePatternTypeLiteral,
				ACLs: []kmsg.DescribeACLsResponseResourceACL{
					{Principal: "User:reader", Host: "127.0.0.1", Operation: kmsg.ACLOperationRead, PermissionType: kmsg.ACLPermissionTypeAllow},
				},
			},
		}
		return resp, nil, true
	})
	bindings, err := client.ListUserACLs()
	require.NoError(t, err)
	require.ElementsMatch(t, []ACL{
		{ResourceType: "TOPIC", ResourceName: "events-", PatternType: "PREFIXED", Principal: "User:reader", Host: "*", Operation: "READ", Permission: "ALLOW"},
		{ResourceType: "TOPIC", ResourceName: "events-", PatternType: "PREFIXED", Principal: "User:writer", Host: "*", Operation: "WRITE", Permission: "DENY"},
		{ResourceType: "GROUP", ResourceName: "readers", PatternType: "LITERAL", Principal: "User:reader", Host: "127.0.0.1", Operation: "READ", Permission: "ALLOW"},
	}, bindings)
}

func TestDeleteUserACLsDoesNotReplayLostReply(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	// Warm the broker connection before injecting a lost reply.
	_, _, err := client.DescribeCluster()
	require.NoError(t, err)
	var requests atomic.Int32
	cluster.ControlKey(int16(kmsg.DeleteACLs), func(req kmsg.Request) (kmsg.Response, error, bool) {
		cluster.KeepControl()
		if requests.Add(1) == 1 {
			// The broker may have processed the delete before the connection closed.
			return nil, io.EOF, true
		}
		resp := req.ResponseKind().(*kmsg.DeleteACLsResponse)
		resp.Results = []kmsg.DeleteACLsResponseResult{{}}
		return resp, nil, true
	})
	require.Error(t, client.DeleteUserACLs("CN=reader", "prefixed"))
	require.Equal(t, int32(1), requests.Load())
}

func TestDeleteUserACLsReturnsMatchingACLError(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	cluster.ControlKey(int16(kmsg.DeleteACLs), func(req kmsg.Request) (kmsg.Response, error, bool) {
		resp := req.ResponseKind().(*kmsg.DeleteACLsResponse)
		resp.Results = []kmsg.DeleteACLsResponseResult{{
			MatchingACLs: []kmsg.DeleteACLsResponseResultMatchingACL{{ErrorCode: kerr.UnknownServerError.Code}},
		}}
		return resp, nil, true
	})
	require.ErrorIs(t, client.DeleteUserACLs("CN=reader", "prefixed"), kerr.UnknownServerError)
}

func TestCreateUserACLsRetriesOnlyRejectedBindings(t *testing.T) {
	cluster := kfake.MustCluster(kfake.NumBrokers(1))
	t.Cleanup(cluster.Close)
	client := openTestClient(t, cluster)
	requests := make(chan []kmsg.CreateACLsRequestCreation, 3)
	var reads atomic.Int32
	cluster.ControlKey(int16(kmsg.CreateACLs), func(req kmsg.Request) (kmsg.Response, error, bool) {
		cluster.KeepControl()
		creations := req.(*kmsg.CreateACLsRequest).Creations
		select {
		case requests <- creations:
		default:
		}
		resp := req.ResponseKind().(*kmsg.CreateACLsResponse)
		resp.Results = make([]kmsg.CreateACLsResponseResult, len(creations))
		if reads.Add(1) == 1 {
			resp.Results[1].ErrorCode = kerr.NotController.Code
		}
		return resp, nil, true
	})
	require.NoError(t, client.CreateUserACLs("read", "literal", "CN=reader", "test-topic"))
	require.Equal(t, int32(3), reads.Load())
	require.Len(t, requests, 3)
	original, retry := <-requests, <-requests
	require.Equal(t, original[1:], retry)
	require.ElementsMatch(t, []kmsg.CreateACLsRequestCreation{
		{
			ResourceType: kmsg.ACLResourceTypeTopic, ResourceName: "test-topic", ResourcePatternType: kmsg.ACLResourcePatternTypeLiteral,
			Principal: "User:CN=reader", Host: "*", Operation: kmsg.ACLOperationRead, PermissionType: kmsg.ACLPermissionTypeAllow,
		},
		{
			ResourceType: kmsg.ACLResourceTypeGroup, ResourceName: "*", ResourcePatternType: kmsg.ACLResourcePatternTypeLiteral,
			Principal: "User:CN=reader", Host: "*", Operation: kmsg.ACLOperationRead, PermissionType: kmsg.ACLPermissionTypeAllow,
		},
	}, <-requests)
}

func TestACLWriteHasDeadlineSeparateFromControllerDiscovery(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		cluster, client, dialer := newTimedKafka(t, 2, kfake.NumBrokers(1))
		require.NoError(t, client.open(dialer))
		cluster.ControlKey(int16(kmsg.Metadata), func(kmsg.Request) (kmsg.Response, error, bool) {
			time.Sleep(1200 * time.Millisecond)
			return nil, nil, false
		})
		var writes atomic.Int32
		cluster.ControlKey(int16(kmsg.DeleteACLs), func(req kmsg.Request) (kmsg.Response, error, bool) {
			cluster.KeepControl()
			resp := req.ResponseKind().(*kmsg.DeleteACLsResponse)
			resp.Results = []kmsg.DeleteACLsResponseResult{{}}
			if writes.Add(1) == 1 {
				resp.Results[0].ErrorCode = kerr.NotController.Code
			} else {
				time.Sleep(1200 * time.Millisecond)
			}
			return resp, nil, true
		})
		require.NoError(t, client.DeleteUserACLs("CN=reader", "prefixed"))
		require.Equal(t, int32(2), writes.Load())
	})
}
