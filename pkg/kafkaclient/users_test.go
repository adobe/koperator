// Copyright © 2019 Cisco Systems, Inc. and/or its affiliates
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
	"testing"

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
	filters := make(chan []kmsg.DeleteACLsRequestFilter, 1)
	cluster.ControlKey(int16(kmsg.DeleteACLs), func(req kmsg.Request) (kmsg.Response, error, bool) {
		filters <- req.(*kmsg.DeleteACLsRequest).Filters
		resp := req.ResponseKind().(*kmsg.DeleteACLsResponse)
		resp.Results = []kmsg.DeleteACLsResponseResult{{}}
		return resp, nil, true
	})
	require.NoError(t, client.DeleteUserACLs("CN=reader", "prefixed"))
	require.Equal(t, []kmsg.DeleteACLsRequestFilter{{
		ResourceType: kmsg.ACLResourceTypeTopic, ResourcePatternType: kmsg.ACLResourcePatternTypePrefixed,
		Principal: kmsg.StringPtr("CN=reader"), Operation: kmsg.ACLOperationDelete, PermissionType: kmsg.ACLPermissionTypeAny,
	}}, <-filters)
}
