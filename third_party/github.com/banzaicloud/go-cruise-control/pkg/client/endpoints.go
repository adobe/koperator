/*
Copyright © 2021 Cisco and/or its affiliates. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package client

import (
	"context"
	"net/http"

	"github.com/banzaicloud/go-cruise-control/pkg/api"
)

func (c *Client) AddBroker(ctx context.Context, r *api.AddBrokerRequest) (*api.AddBrokerResponse, error) {
	resp := &api.AddBrokerResponse{}
	return resp, c.request(ctx, r, resp, api.EndpointAddBroker, http.MethodPost)
}

func (c *Client) Admin(ctx context.Context, r *api.AdminRequest) (*api.AdminResponse, error) {
	resp := &api.AdminResponse{}
	return resp, c.request(ctx, r, resp, api.EndpointAdmin, http.MethodPost)
}

func (c *Client) Bootstrap(ctx context.Context, r *api.BootstrapRequest) (*api.BootstrapResponse, error) {
	resp := &api.BootstrapResponse{}
	return resp, c.request(ctx, r, resp, api.EndpointBootstrap, http.MethodGet)
}

func (c *Client) DemoteBroker(ctx context.Context, r *api.DemoteBrokerRequest) (*api.DemoteBrokerResponse, error) {
	resp := &api.DemoteBrokerResponse{}
	return resp, c.request(ctx, r, resp, api.EndpointDemoteBroker, http.MethodPost)
}

func (c *Client) FixOfflineReplicas(ctx context.Context, r *api.FixOfflineReplicasRequest) (*api.FixOfflineReplicasResponse, error) {
	resp := &api.FixOfflineReplicasResponse{}
	return resp, c.request(ctx, r, resp, api.EndpointFixOfflineReplicas, http.MethodPost)
}

func (c *Client) KafkaClusterLoad(ctx context.Context, r *api.KafkaClusterLoadRequest) (*api.KafkaClusterLoadResponse, error) {
	resp := &api.KafkaClusterLoadResponse{}
	return resp, c.request(ctx, r, resp, api.EndpointKafkaClusterLoad, http.MethodGet)
}

func (c *Client) KafkaClusterState(ctx context.Context, r *api.KafkaClusterStateRequest) (*api.KafkaClusterStateResponse, error) {
	resp := &api.KafkaClusterStateResponse{}
	return resp, c.request(ctx, r, resp, api.EndpointKafkaClusterState, http.MethodGet)
}

func (c *Client) KafkaPartitionLoad(ctx context.Context, r *api.KafkaPartitionLoadRequest) (*api.KafkaPartitionLoadResponse, error) {
	resp := &api.KafkaPartitionLoadResponse{}
	return resp, c.request(ctx, r, resp, api.EndpointKafkaPartitionLoad, http.MethodGet)
}

func (c *Client) PauseSampling(ctx context.Context, r *api.PauseSamplingRequest) (*api.PauseSamplingResponse, error) {
	resp := &api.PauseSamplingResponse{}
	return resp, c.request(ctx, r, resp, api.EndpointPauseSampling, http.MethodPost)
}

func (c *Client) Proposals(ctx context.Context, r *api.ProposalsRequest) (*api.ProposalsResponse, error) {
	resp := &api.ProposalsResponse{}
	return resp, c.request(ctx, r, resp, api.EndpointProposals, http.MethodGet)
}

func (c *Client) Rebalance(ctx context.Context, r *api.RebalanceRequest) (*api.RebalanceResponse, error) {
	resp := &api.RebalanceResponse{}
	return resp, c.request(ctx, r, resp, api.EndpointRebalance, http.MethodPost)
}

func (c *Client) RemoveBroker(ctx context.Context, r *api.RemoveBrokerRequest) (*api.RemoveBrokerResponse, error) {
	resp := &api.RemoveBrokerResponse{}
	return resp, c.request(ctx, r, resp, api.EndpointRemoveBroker, http.MethodPost)
}

func (c *Client) RemoveDisks(ctx context.Context, r *api.RemoveDisksRequest) (*api.RemoveDisksResponse, error) {
	resp := &api.RemoveDisksResponse{}
	return resp, c.request(ctx, r, resp, api.EndpointRemoveDisks, http.MethodPost)
}

func (c *Client) ResumeSampling(ctx context.Context, r *api.ResumeSamplingRequest) (*api.ResumeSamplingResponse, error) {
	resp := &api.ResumeSamplingResponse{}
	return resp, c.request(ctx, r, resp, api.EndpointResumeSampling, http.MethodPost)
}

func (c *Client) Review(ctx context.Context, r *api.ReviewRequest) (*api.ReviewResponse, error) {
	resp := &api.ReviewResponse{}
	return resp, c.request(ctx, r, resp, api.EndpointReview, http.MethodPost)
}

// ReviewBoard returns a list of Cruise Control requests with their review state.
func (c *Client) ReviewBoard(ctx context.Context, r *api.ReviewBoardRequest) (*api.ReviewBoardResponse, error) {
	resp := &api.ReviewBoardResponse{}
	return resp, c.request(ctx, r, resp, api.EndpointReviewBoard, http.MethodGet)
}

// Rightsize allows manually invoke provisioner rightsizing of the cluster.
func (c *Client) Rightsize(ctx context.Context, r *api.RightsizeRequest) (*api.RightsizeResponse, error) {
	resp := &api.RightsizeResponse{}
	return resp, c.request(ctx, r, resp, api.EndpointRightsize, http.MethodPost)
}

// State reports back the Cruise Control state.
func (c *Client) State(ctx context.Context, r *api.StateRequest) (*api.StateResponse, error) {
	resp := &api.StateResponse{}
	return resp, c.request(ctx, r, resp, api.EndpointState, http.MethodGet)
}

// StopProposalExecution invoke stopping of ongoing proposal execution in Cruise Control.
func (c *Client) StopProposalExecution(ctx context.Context, r *api.StopProposalExecutionRequest) (*api.StopProposalExecutionResponse, error) { //nolint:lll
	resp := &api.StopProposalExecutionResponse{}
	return resp, c.request(ctx, r, resp, api.EndpointStopProposalExecution, http.MethodPost)
}

// TopicConfiguration allows changing Kafka topic configuration using Cruise Control.
func (c *Client) TopicConfiguration(ctx context.Context, r *api.TopicConfigurationRequest) (*api.TopicConfigurationResponse, error) {
	resp := &api.TopicConfigurationResponse{}
	return resp, c.request(ctx, r, resp, api.EndpointTopicConfiguration, http.MethodPost)
}

// Train Cruise Control to better model broker cpu usage.
func (c *Client) Train(ctx context.Context, r *api.TrainRequest) (*api.TrainResponse, error) {
	resp := &api.TrainResponse{}
	return resp, c.request(ctx, r, resp, api.EndpointTrain, http.MethodGet)
}

// UserTasks returns the list of recent tasks performed by Cruise Control.
func (c *Client) UserTasks(ctx context.Context, r *api.UserTasksRequest) (*api.UserTasksResponse, error) {
	resp := &api.UserTasksResponse{}
	return resp, c.request(ctx, r, resp, api.EndpointUserTasks, http.MethodGet)
}
