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

package types

const (
	UndefinedGoal Goal = iota
	CPUCapacityGoal
	CPUUsageDistributionGoal
	DiskCapacityGoal
	DiskUsageDistributionGoal
	IntraBrokerDiskCapacityGoal
	IntraBrokerDiskUsageDistributionGoal
	LeaderBytesInDistributionGoal
	LeaderReplicaDistributionGoal
	MinTopicLeadersPerBrokerGoal
	NetworkInboundCapacityGoal
	NetworkInboundUsageDistributionGoal
	NetworkOutboundCapacityGoal
	NetworkOutboundUsageDistributionGoal
	PotentialNwOutGoal
	PreferredLeaderElectionGoal
	RackAwareDistributionGoal
	RackAwareGoal
	ReplicaCapacityGoal
	ReplicaDistributionGoal
	TopicReplicaDistributionGoal
	BrokerSetAwareGoal
	KafkaAssignerDiskUsageDistributionGoal
	KafkaAssignerEvenRackAwareGoal
)

type Goal int8

func (g Goal) String() string { // nolint:cyclop
	var goal string

	switch g {
	case CPUCapacityGoal:
		goal = "CpuCapacityGoal"
	case CPUUsageDistributionGoal:
		goal = "CpuUsageDistributionGoal"
	case DiskCapacityGoal:
		goal = "DiskCapacityGoal"
	case DiskUsageDistributionGoal:
		goal = "DiskUsageDistributionGoal"
	case IntraBrokerDiskCapacityGoal:
		goal = "IntraBrokerDiskCapacityGoal"
	case IntraBrokerDiskUsageDistributionGoal:
		goal = "IntraBrokerDiskUsageDistributionGoal"
	case LeaderBytesInDistributionGoal:
		goal = "LeaderBytesInDistributionGoal"
	case LeaderReplicaDistributionGoal:
		goal = "LeaderReplicaDistributionGoal"
	case MinTopicLeadersPerBrokerGoal:
		goal = "MinTopicLeadersPerBrokerGoal"
	case NetworkInboundCapacityGoal:
		goal = "NetworkInboundCapacityGoal"
	case NetworkInboundUsageDistributionGoal:
		goal = "NetworkInboundUsageDistributionGoal"
	case NetworkOutboundCapacityGoal:
		goal = "NetworkOutboundCapacityGoal"
	case NetworkOutboundUsageDistributionGoal:
		goal = "NetworkOutboundUsageDistributionGoal"
	case PotentialNwOutGoal:
		goal = "PotentialNwOutGoal"
	case PreferredLeaderElectionGoal:
		goal = "PreferredLeaderElectionGoal"
	case RackAwareDistributionGoal:
		goal = "RackAwareDistributionGoal"
	case RackAwareGoal:
		goal = "RackAwareGoal"
	case ReplicaCapacityGoal:
		goal = "ReplicaCapacityGoal"
	case ReplicaDistributionGoal:
		goal = "ReplicaDistributionGoal"
	case TopicReplicaDistributionGoal:
		goal = "TopicReplicaDistributionGoal"
	case BrokerSetAwareGoal:
		goal = "BrokerSetAwareGoal"
	case KafkaAssignerDiskUsageDistributionGoal:
		goal = "KafkaAssignerDiskUsageDistributionGoal"
	case KafkaAssignerEvenRackAwareGoal:
		goal = "KafkaAssignerEvenRackAwareGoal"
	case UndefinedGoal:
		fallthrough
	default:
		goal = "UndefinedGoal"
	}
	return goal
}

func (g *Goal) MarshalJSON() ([]byte, error) {
	return []byte(addQuotes(g.String())), nil
}

func (g *Goal) UnmarshalJSON(data []byte) error {
	switch removeQuotes(string(data)) {
	case CPUCapacityGoal.String():
		*g = CPUCapacityGoal
	case CPUUsageDistributionGoal.String():
		*g = CPUUsageDistributionGoal
	case DiskCapacityGoal.String():
		*g = DiskCapacityGoal
	case DiskUsageDistributionGoal.String():
		*g = DiskUsageDistributionGoal
	case IntraBrokerDiskCapacityGoal.String():
		*g = IntraBrokerDiskCapacityGoal
	case IntraBrokerDiskUsageDistributionGoal.String():
		*g = IntraBrokerDiskUsageDistributionGoal
	case LeaderBytesInDistributionGoal.String():
		*g = LeaderBytesInDistributionGoal
	case LeaderReplicaDistributionGoal.String():
		*g = LeaderReplicaDistributionGoal
	case MinTopicLeadersPerBrokerGoal.String():
		*g = MinTopicLeadersPerBrokerGoal
	case NetworkInboundCapacityGoal.String():
		*g = NetworkInboundCapacityGoal
	case NetworkInboundUsageDistributionGoal.String():
		*g = NetworkInboundUsageDistributionGoal
	case NetworkOutboundCapacityGoal.String():
		*g = NetworkOutboundCapacityGoal
	case NetworkOutboundUsageDistributionGoal.String():
		*g = NetworkOutboundUsageDistributionGoal
	case PotentialNwOutGoal.String():
		*g = PotentialNwOutGoal
	case PreferredLeaderElectionGoal.String():
		*g = PreferredLeaderElectionGoal
	case RackAwareDistributionGoal.String():
		*g = RackAwareDistributionGoal
	case RackAwareGoal.String():
		*g = RackAwareGoal
	case ReplicaCapacityGoal.String():
		*g = ReplicaCapacityGoal
	case ReplicaDistributionGoal.String():
		*g = ReplicaDistributionGoal
	case TopicReplicaDistributionGoal.String():
		*g = TopicReplicaDistributionGoal
	case BrokerSetAwareGoal.String():
		*g = BrokerSetAwareGoal
	case KafkaAssignerDiskUsageDistributionGoal.String():
		*g = KafkaAssignerDiskUsageDistributionGoal
	case KafkaAssignerEvenRackAwareGoal.String():
		*g = KafkaAssignerEvenRackAwareGoal
	default:
		*g = UndefinedGoal
	}
	return nil
}

func (g *Goal) UnmarshalText(data []byte) error {
	return g.UnmarshalJSON(data)
}

func (g Goal) All() []Goal {
	return []Goal{
		CPUCapacityGoal,
		CPUUsageDistributionGoal,
		DiskCapacityGoal,
		DiskUsageDistributionGoal,
		IntraBrokerDiskCapacityGoal,
		IntraBrokerDiskUsageDistributionGoal,
		LeaderBytesInDistributionGoal,
		LeaderReplicaDistributionGoal,
		MinTopicLeadersPerBrokerGoal,
		NetworkInboundCapacityGoal,
		NetworkInboundUsageDistributionGoal,
		NetworkOutboundCapacityGoal,
		NetworkOutboundUsageDistributionGoal,
		PotentialNwOutGoal,
		PreferredLeaderElectionGoal,
		RackAwareDistributionGoal,
		RackAwareGoal,
		ReplicaCapacityGoal,
		ReplicaDistributionGoal,
		TopicReplicaDistributionGoal,
		BrokerSetAwareGoal,
		KafkaAssignerDiskUsageDistributionGoal,
		KafkaAssignerEvenRackAwareGoal,
	}
}
