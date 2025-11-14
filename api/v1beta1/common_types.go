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

package v1beta1

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// RackAwarenessState stores info about rack awareness status
type RackAwarenessState string

// CruiseControlState holds info about the state of Cruise Control
type CruiseControlState string

// CruiseControlTopicStatus holds info about the CC topic status
type CruiseControlTopicStatus string

// CruiseControlUserTaskState holds info about the CC user task state
type CruiseControlUserTaskState string

// ClusterState holds info about the cluster state
type ClusterState string

// ConfigurationState holds info about the configuration state
type ConfigurationState string

// SecurityProtocol is the protocol used to communicate with brokers.
// Valid values are: plaintext, ssl, sasl_plaintext, sasl_ssl.
type SecurityProtocol string

// SSLClientAuthentication specifies whether client authentication is required, requested, or not required.
// Valid values are: required, requested, none
type SSLClientAuthentication string

// PerBrokerConfigurationState holds info about the per-broker configuration state
type PerBrokerConfigurationState string

// ExternalListenerConfigNames type describes a collection of external listener names
type ExternalListenerConfigNames []string

// KafkaVersion type describes the kafka version and docker version
type KafkaVersion struct {
	// Version holds the current version of the broker in semver format
	Version string `json:"version,omitempty"`
	// Image specifies the current docker image of the broker
	Image string `json:"image,omitempty"`
}

// PKIBackend represents an interface implementing the PKIManager
type PKIBackend string

// CruiseControlVolumeState holds information about the state of volume rebalance
type CruiseControlVolumeState string

// IsDiskRebalanceRunning returns true if CruiseControlVolumeState indicates
// that the CC rebalance disk operation is scheduled and in-progress
func (s CruiseControlVolumeState) IsDiskRebalanceRunning() bool {
	return s == GracefulDiskRebalanceRunning ||
		s == GracefulDiskRebalanceCompletedWithError ||
		s == GracefulDiskRebalancePaused ||
		s == GracefulDiskRebalanceScheduled
}

// IsDiskRemovalRunning returns true if CruiseControlVolumeState indicates
// that the CC remove disks operation is scheduled and in-progress
func (s CruiseControlVolumeState) IsDiskRemovalRunning() bool {
	return s == GracefulDiskRemovalRunning ||
		s == GracefulDiskRemovalCompletedWithError ||
		s == GracefulDiskRemovalPaused ||
		s == GracefulDiskRemovalScheduled
}

// IsRequiredState returns true if CruiseControlVolumeState is in GracefulDiskRebalanceRequired state or GracefulDiskRemovalRequired state
func (s CruiseControlVolumeState) IsRequiredState() bool {
	return s == GracefulDiskRebalanceRequired ||
		s == GracefulDiskRemovalRequired
}

// IsDiskRebalance returns true if CruiseControlVolumeState is in disk rebalance state
// the controller needs to take care of.
func (s CruiseControlVolumeState) IsDiskRebalance() bool {
	return s.IsDiskRebalanceRunning() || s == GracefulDiskRebalanceRequired
}

// IsDiskRemoval returns true if CruiseControlVolumeState is in disk removal state
func (s CruiseControlVolumeState) IsDiskRemoval() bool {
	return s.IsDiskRemovalRunning() || s == GracefulDiskRemovalRequired
}

// IsUpscale returns true if CruiseControlState in GracefulUpscale* state.
func (r CruiseControlState) IsUpscale() bool {
	return r == GracefulUpscaleRequired ||
		r == GracefulUpscaleSucceeded ||
		r == GracefulUpscaleRunning ||
		r == GracefulUpscaleCompletedWithError ||
		r == GracefulUpscalePaused ||
		r == GracefulUpscaleScheduled
}

// IsDownscale returns true if CruiseControlState in GracefulDownscale* state.
func (r CruiseControlState) IsDownscale() bool {
	return r == GracefulDownscaleRequired ||
		r == GracefulDownscaleSucceeded ||
		r == GracefulDownscaleRunning ||
		r == GracefulDownscaleCompletedWithError ||
		r == GracefulDownscalePaused ||
		r == GracefulDownscaleScheduled
}

// IsRunningState returns true if CruiseControlState indicates
// that the CC operation is scheduled and in-progress
func (r CruiseControlState) IsRunningState() bool {
	return r == GracefulUpscaleRunning ||
		r == GracefulUpscaleCompletedWithError ||
		r == GracefulUpscalePaused ||
		r == GracefulUpscaleScheduled ||
		r == GracefulDownscaleRunning ||
		r == GracefulDownscaleCompletedWithError ||
		r == GracefulDownscalePaused ||
		r == GracefulDownscaleScheduled
}

// IsRequiredState returns true if CruiseControlVolumeState indicates that either upscaling or downscaling
// (GracefulDownscaleRequired or GracefulUpscaleRequired) operation needs to be performed.
func (r CruiseControlState) IsRequiredState() bool {
	return r == GracefulDownscaleRequired ||
		r == GracefulUpscaleRequired
}

// IsActive returns true if CruiseControlState is in active state
// the controller needs to take care of.
func (r CruiseControlState) IsActive() bool {
	return r.IsRunningState() || r.IsRequiredState()
}

// IsSucceeded returns true if CruiseControlState is succeeded
func (r CruiseControlState) IsSucceeded() bool {
	return r == GracefulDownscaleSucceeded ||
		r == GracefulUpscaleSucceeded
}

// IsDiskRebalanceSucceeded returns true if CruiseControlVolumeState is disk rebalance succeeded
func (s CruiseControlVolumeState) IsDiskRebalanceSucceeded() bool {
	return s == GracefulDiskRebalanceSucceeded
}

// IsDiskRemovalSucceeded returns true if CruiseControlVolumeState is disk removal succeeded
func (s CruiseControlVolumeState) IsDiskRemovalSucceeded() bool {
	return s == GracefulDiskRemovalSucceeded
}

// IsSSL determines if the receiver is using SSL
func (r SecurityProtocol) IsSSL() bool {
	return r.Equal(SecurityProtocolSaslSSL) || r.Equal(SecurityProtocolSSL)
}

// IsSasl determines if the receiver is using Sasl
func (r SecurityProtocol) IsSasl() bool {
	return r.Equal(SecurityProtocolSaslSSL) || r.Equal(SecurityProtocolSaslPlaintext)
}

// IsPlaintext determines if the receiver is using plaintext
func (r SecurityProtocol) IsPlaintext() bool {
	return r.Equal(SecurityProtocolPlaintext) || r.Equal(SecurityProtocolSaslPlaintext)
}

// ToUpperString converts SecurityProtocol to an upper string
func (r SecurityProtocol) ToUpperString() string {
	return strings.ToUpper(string(r))
}

// Equal checks the equality between two SecurityProtocols
func (r SecurityProtocol) Equal(s SecurityProtocol) bool {
	return r.ToUpperString() == s.ToUpperString()
}

const (
	// PKIBackendCertManager invokes cert-manager for user certificate management
	PKIBackendCertManager PKIBackend = "cert-manager"
	// PKIBackendProvided used to point the operator to use the PKI set in the cluster CR
	// for admin and users required for the cluster to run
	PKIBackendProvided PKIBackend = "pki-backend-provided"
	// PKIBackendK8sCSR invokes kubernetes csr API for user certificate management
	PKIBackendK8sCSR PKIBackend = "k8s-csr"
)

// GracefulActionState holds information about GracefulAction State
type GracefulActionState struct {
	// CruiseControlState holds the information about graceful action state
	CruiseControlState CruiseControlState `json:"cruiseControlState"`
	// CruiseControlOperationReference refers to the created CruiseControlOperation to execute a CC task
	CruiseControlOperationReference *corev1.LocalObjectReference `json:"cruiseControlOperationReference,omitempty"`
	// VolumeStates holds the information about the CC disk rebalance states and CruiseControlOperation reference
	VolumeStates map[string]VolumeState `json:"volumeStates,omitempty"`
}

type VolumeState struct {
	// CruiseControlVolumeState holds the information about CC disk rebalance state
	CruiseControlVolumeState CruiseControlVolumeState `json:"cruiseControlVolumeState"`
	// CruiseControlOperationReference refers to the created CruiseControlOperation to execute a CC task
	CruiseControlOperationReference *corev1.LocalObjectReference `json:"cruiseControlOperationReference,omitempty"`
}

// BrokerState holds information about broker state
type BrokerState struct {
	// RackAwarenessState holds info about rack awareness status
	RackAwarenessState RackAwarenessState `json:"rackAwarenessState"`
	// GracefulActionState holds info about cc action status
	GracefulActionState GracefulActionState `json:"gracefulActionState"`
	// ConfigurationState holds info about the config
	ConfigurationState ConfigurationState `json:"configurationState"`
	// PerBrokerConfigurationState holds info about the per-broker (dynamically updatable) config
	PerBrokerConfigurationState PerBrokerConfigurationState `json:"perBrokerConfigurationState"`
	// ExternalListenerConfigNames holds info about what listener config is in use with the broker
	ExternalListenerConfigNames ExternalListenerConfigNames `json:"externalListenerConfigNames,omitempty"`
	// Version holds the current version of the broker in semver format
	Version string `json:"version,omitempty"`
	// Image specifies the current docker image of the broker
	Image string `json:"image,omitempty"`
	// Compressed data from broker configuration to restore broker pod in specific cases
	ConfigurationBackup string `json:"configurationBackup,omitempty"`
}

const (
	// Configured states the broker is running
	Configured RackAwarenessState = "Configured"
	// WaitingForRackAwareness states the broker is waiting for the rack awareness config
	WaitingForRackAwareness RackAwarenessState = "WaitingForRackAwareness"

	// GracefulUpscaleRequired indicates that a broker upscale operation is needed.
	// This is the initial state when new brokers are added to the cluster and Cruise Control
	// needs to rebalance partitions to distribute load across the new brokers.
	// Transition: Required -> Scheduled -> Running -> Succeeded/CompletedWithError/Paused
	GracefulUpscaleRequired CruiseControlState = "GracefulUpscaleRequired"
	// GracefulUpscaleRunning indicates that the broker upscale task is actively executing in Cruise Control.
	// During this state, CC is moving partition replicas to the new brokers to balance the cluster load.
	// The operation may take significant time depending on cluster size and data volume.
	GracefulUpscaleRunning CruiseControlState = "GracefulUpscaleRunning"
	// GracefulUpscaleScheduled indicates that a CruiseControlOperation resource has been created
	// for the broker upscale task and is waiting in the queue for execution.
	// This state occurs when CC is busy with other operations or waiting for prerequisites.
	GracefulUpscaleScheduled CruiseControlState = "GracefulUpscaleScheduled"
	// GracefulUpscaleSucceeded indicates that the broker upscale completed successfully and
	// partitions have been rebalanced across the new brokers. This is also the state for brokers
	// that are part of the initial cluster creation while the Cruise Control topic is being created.
	GracefulUpscaleSucceeded CruiseControlState = "GracefulUpscaleSucceeded"
	// GracefulUpscaleCompletedWithError indicates that the broker upscale task finished but
	// encountered errors during execution. The operation may be retried automatically depending
	// on the error type and retry policy configuration.
	GracefulUpscaleCompletedWithError CruiseControlState = "GracefulUpscaleCompletedWithError"
	// GracefulUpscalePaused indicates that the broker upscale task encountered an error and
	// has been paused. The operation will not be automatically retried and requires manual
	// intervention to resolve the issue before resuming.
	GracefulUpscalePaused CruiseControlState = "GracefulUpscalePaused"

	// Downscale cruise control states

	// GracefulDownscaleRequired indicates that a broker downscale operation is needed.
	// This state is set when brokers are being removed from the cluster and Cruise Control
	// must migrate all partition replicas off the brokers before they can be safely decommissioned.
	// Transition: Required -> Scheduled -> Running -> Succeeded/CompletedWithError/Paused
	GracefulDownscaleRequired CruiseControlState = "GracefulDownscaleRequired"
	// GracefulDownscaleScheduled indicates that a CruiseControlOperation resource has been created
	// for the broker downscale task and is waiting in the queue for execution.
	// This state occurs when CC is busy with other operations or waiting for prerequisites.
	GracefulDownscaleScheduled CruiseControlState = "GracefulDownscaleScheduled"
	// GracefulDownscaleRunning indicates that the broker downscale task is actively executing in Cruise Control.
	// During this state, CC is moving all partition replicas off the brokers being removed to ensure
	// no data loss occurs. This operation may take significant time depending on data volume.
	GracefulDownscaleRunning CruiseControlState = "GracefulDownscaleRunning"
	// GracefulDownscaleSucceeded indicates that the broker downscale completed successfully.
	// All partition replicas have been migrated off the removed brokers and they can be safely
	// decommissioned without data loss or service interruption.
	GracefulDownscaleSucceeded CruiseControlState = "GracefulDownscaleSucceeded"
	// GracefulDownscaleCompletedWithError indicates that the broker downscale task finished but
	// encountered errors during execution. The operation may be retried automatically depending
	// on the error type and retry policy configuration.
	GracefulDownscaleCompletedWithError CruiseControlState = "GracefulDownscaleCompletedWithError"
	// GracefulDownscalePaused indicates that the broker downscale task encountered an error and
	// has been paused. The operation will not be automatically retried and requires manual intervention.
	// Note: In this state, further downscale tasks can still be executed for other brokers.
	GracefulDownscalePaused CruiseControlState = "GracefulDownscalePaused"

	// Disk removal cruise control states

	// GracefulDiskRemovalRequired indicates that a broker volume needs to be removed from the cluster.
	// This state is set when storage volumes are being decommissioned and Cruise Control must migrate
	// all partition replicas off the volume before it can be safely removed.
	// Transition: Required -> Scheduled -> Running -> Succeeded/CompletedWithError/Paused
	GracefulDiskRemovalRequired CruiseControlVolumeState = "GracefulDiskRemovalRequired"
	// GracefulDiskRemovalRunning indicates that a Cruise Control disk removal operation is actively
	// executing for the broker volume. During this state, CC is moving all partition replicas from
	// the volume to other available disks to ensure no data loss occurs.
	GracefulDiskRemovalRunning CruiseControlVolumeState = "GracefulDiskRemovalRunning"
	// GracefulDiskRemovalSucceeded indicates that the broker volume removal completed successfully.
	// All partition replicas have been migrated off the volume and it can be safely removed
	// from the broker without data loss or service interruption.
	GracefulDiskRemovalSucceeded CruiseControlVolumeState = "GracefulDiskRemovalSucceeded"
	// GracefulDiskRemovalScheduled indicates that a CruiseControlOperation resource has been created
	// for the volume removal task and is waiting in the queue for execution.
	// This state occurs when CC is busy with other operations or waiting for prerequisites.
	GracefulDiskRemovalScheduled CruiseControlVolumeState = "GracefulDiskRemovalScheduled"
	// GracefulDiskRemovalCompletedWithError indicates that the broker volume removal task finished
	// but encountered errors during execution. The operation may be retried automatically depending
	// on the error type and retry policy configuration.
	GracefulDiskRemovalCompletedWithError CruiseControlVolumeState = "GracefulDiskRemovalCompletedWithError"
	// GracefulDiskRemovalPaused indicates that the broker volume removal task encountered an error
	// and has been paused. The operation will not be automatically retried and requires manual
	// intervention to resolve the issue before resuming.
	GracefulDiskRemovalPaused CruiseControlVolumeState = "GracefulDiskRemovalPaused"

	// Disk rebalance cruise control states

	// GracefulDiskRebalanceRequired indicates that a broker volume needs disk rebalancing.
	// This state is set when storage utilization is uneven across volumes and Cruise Control
	// should redistribute partition replicas to achieve better balance and performance.
	// Transition: Required -> Scheduled -> Running -> Succeeded/CompletedWithError/Paused
	GracefulDiskRebalanceRequired CruiseControlVolumeState = "GracefulDiskRebalanceRequired"
	// GracefulDiskRebalanceRunning indicates that a Cruise Control disk rebalance operation is
	// actively executing for the broker volume. During this state, CC is moving partition replicas
	// between volumes to achieve more even storage utilization and improve performance.
	GracefulDiskRebalanceRunning CruiseControlVolumeState = "GracefulDiskRebalanceRunning"
	// GracefulDiskRebalanceSucceeded indicates that the broker volume rebalance completed successfully.
	// Partition replicas have been redistributed across volumes to achieve better storage balance
	// and the volume is now optimally utilized.
	GracefulDiskRebalanceSucceeded CruiseControlVolumeState = "GracefulDiskRebalanceSucceeded"
	// GracefulDiskRebalanceScheduled indicates that a CruiseControlOperation resource has been created
	// for the volume rebalance task and is waiting in the queue for execution.
	// This state occurs when CC is busy with other operations or waiting for prerequisites.
	GracefulDiskRebalanceScheduled CruiseControlVolumeState = "GracefulDiskRebalanceScheduled"
	// GracefulDiskRebalanceCompletedWithError indicates that the broker volume rebalance task finished
	// but encountered errors during execution. The operation may be retried automatically depending
	// on the error type and retry policy configuration.
	GracefulDiskRebalanceCompletedWithError CruiseControlVolumeState = "GracefulDiskRebalanceCompletedWithError"
	// GracefulDiskRebalancePaused indicates that the broker volume rebalance task encountered an error
	// and has been paused. The operation will not be automatically retried and requires manual
	// intervention to resolve the issue before resuming.
	GracefulDiskRebalancePaused CruiseControlVolumeState = "GracefulDiskRebalancePaused"

	// CruiseControlTopicNotReady indicates that the Cruise Control metrics topic has not been created yet.
	// This internal topic is required for CC to collect and store broker metrics. Operations cannot
	// proceed until this topic is successfully created and ready.
	CruiseControlTopicNotReady CruiseControlTopicStatus = "CruiseControlTopicNotReady"
	// CruiseControlTopicReady indicates that the Cruise Control metrics topic has been successfully created
	// and is ready to receive broker metrics. This is a prerequisite for CC operations to execute.
	CruiseControlTopicReady CruiseControlTopicStatus = "CruiseControlTopicReady"
	// CruiseControlTaskActive indicates that a Cruise Control task has been scheduled and is waiting
	// in the queue but has not yet started execution. This occurs when CC is processing other tasks
	// or waiting for required conditions to be met.
	CruiseControlTaskActive CruiseControlUserTaskState = "Active"
	// CruiseControlTaskInExecution indicates that a Cruise Control task is currently executing.
	// During this state, CC is actively performing the requested operation such as rebalancing
	// partitions, adding/removing brokers, or moving replicas between disks.
	CruiseControlTaskInExecution CruiseControlUserTaskState = "InExecution"
	// CruiseControlTaskCompleted indicates that a Cruise Control task finished successfully.
	// The requested operation has been completed without errors and the cluster is in the desired state.
	CruiseControlTaskCompleted CruiseControlUserTaskState = "Completed"
	// CruiseControlTaskCompletedWithError indicates that a Cruise Control task finished but encountered
	// errors during execution. The task may have partially completed or failed entirely. Check the
	// error details to determine if retry or manual intervention is needed.
	CruiseControlTaskCompletedWithError CruiseControlUserTaskState = "CompletedWithError"
	// KafkaClusterReconciling indicates that the Kafka cluster is in the reconciliation phase.
	// During this state, the operator is working to bring the cluster to the desired state by
	// creating, updating, or deleting resources. This is a transitional state during initial
	// cluster creation or when applying configuration changes.
	KafkaClusterReconciling ClusterState = "ClusterReconciling"
	// KafkaClusterRollingUpgrading indicates that the Kafka cluster is performing a rolling upgrade.
	// Brokers are being restarted one at a time to apply configuration changes, version upgrades,
	// or other updates while maintaining cluster availability and minimizing service disruption.
	KafkaClusterRollingUpgrading ClusterState = "ClusterRollingUpgrading"
	// KafkaClusterRunning indicates that the Kafka cluster is in a healthy running state.
	// All brokers are operational, configurations are in sync, and the cluster is ready to
	// handle producer and consumer traffic without ongoing maintenance operations.
	KafkaClusterRunning ClusterState = "ClusterRunning"

	// ConfigInSync indicates that the generated broker configuration matches the actual configuration
	// running on the broker. No configuration changes are pending and the broker is operating with
	// the desired settings.
	ConfigInSync ConfigurationState = "ConfigInSync"
	// ConfigOutOfSync indicates that the generated broker configuration differs from the actual
	// configuration running on the broker. A rolling restart or dynamic update may be required
	// to apply the pending configuration changes.
	ConfigOutOfSync ConfigurationState = "ConfigOutOfSync"
	// PerBrokerConfigInSync indicates that the generated per-broker dynamic configuration is in sync
	// with the broker's actual configuration. Per-broker configs are broker-specific settings that
	// can be updated dynamically without requiring a broker restart.
	PerBrokerConfigInSync PerBrokerConfigurationState = "PerBrokerConfigInSync"
	// PerBrokerConfigOutOfSync indicates that the generated per-broker dynamic configuration differs
	// from the broker's actual configuration. The operator will attempt to apply these changes
	// dynamically using Kafka's configuration API without restarting the broker.
	PerBrokerConfigOutOfSync PerBrokerConfigurationState = "PerBrokerConfigOutOfSync"
	// PerBrokerConfigError indicates that the operator failed to apply the per-broker dynamic
	// configuration to the broker. This may be due to invalid configuration values, broker API
	// errors, or permission issues. Manual intervention may be required to resolve the error.
	PerBrokerConfigError PerBrokerConfigurationState = "PerBrokerConfigError"

	// SecurityProtocolSSL enables SSL/TLS encryption for broker communication without SASL authentication.
	// This protocol provides encryption in transit but relies on SSL/TLS certificates for authentication.
	SecurityProtocolSSL SecurityProtocol = "ssl"
	// SecurityProtocolPlaintext uses unencrypted communication with no authentication.
	// This protocol should only be used in trusted networks as all data is transmitted in clear text.
	SecurityProtocolPlaintext SecurityProtocol = "plaintext"
	// SecurityProtocolSaslSSL combines SASL authentication with SSL/TLS encryption.
	// This protocol provides both strong authentication (via SASL mechanisms like SCRAM, GSSAPI, etc.)
	// and encryption in transit, making it the most secure option for production environments.
	SecurityProtocolSaslSSL SecurityProtocol = "sasl_ssl"
	// SecurityProtocolSaslPlaintext enables SASL authentication over unencrypted connections.
	// This protocol provides authentication but transmits data in clear text, including credentials
	// during the authentication handshake. Use with caution and only in trusted networks.
	SecurityProtocolSaslPlaintext SecurityProtocol = "sasl_plaintext"

	// SSLClientAuthRequired states that the client authentication is required when SSL is enabled
	SSLClientAuthRequired SSLClientAuthentication = "required"
)
