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

package cruisecontrol

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"emperror.dev/errors"

	"github.com/go-logr/logr"
	"gopkg.in/inf.v0"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	apiutil "github.com/banzaicloud/koperator/api/util"
	"github.com/banzaicloud/koperator/api/v1alpha1"
	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/resources/templates"
	"github.com/banzaicloud/koperator/pkg/util"
	kafkautils "github.com/banzaicloud/koperator/pkg/util/kafka"
	zookeeperutils "github.com/banzaicloud/koperator/pkg/util/zookeeper"
	properties "github.com/banzaicloud/koperator/properties/pkg"

	corev1 "k8s.io/api/core/v1"
)

const (
	MinLogDirSizeInMB = int64(1)

	storageConfigCPUDefaultValue   = "100"
	storageConfigNWINDefaultValue  = "125000"
	storageConfigNWOUTDefaultValue = "125000"
	defaultDoc                     = "Capacity unit used for disk is in MB, cpu is in percentage, network throughput is in KB."
)

func (r *Reconciler) configMap(clientPass string, capacityConfig string, log logr.Logger) runtime.Object {
	ccConfig := properties.NewProperties()

	// Add base Cruise Control configuration
	conf, err := properties.NewFromString(r.KafkaCluster.Spec.CruiseControlConfig.Config)
	if err != nil {
		log.Error(err, "parsing Cruise Control configuration failed", "config", r.KafkaCluster.Spec.CruiseControlConfig.Config)
	}
	ccConfig.Merge(conf)

	bootstrapServers, err := kafkautils.GetBootstrapServersService(r.KafkaCluster)
	if err != nil {
		log.Error(err, "getting Kafka bootstrap servers for Cruise Control failed")
	}
	if err = ccConfig.Set(kafkautils.KafkaConfigBoostrapServers, bootstrapServers); err != nil {
		log.Error(err, fmt.Sprintf("setting '%s' in Cruise Control configuration failed", kafkautils.KafkaConfigBoostrapServers), "config", bootstrapServers)
	}

	if r.KafkaCluster.Spec.KRaftMode {
		// Set configurations to have Cruise Control to run without Zookeeper
		if err = ccConfig.Set(kafkautils.CruiseControlConfigTopicConfigProviderClass, kafkautils.CruiseControlConfigTopicConfigProviderClassVal); err != nil {
			log.Error(err, fmt.Sprintf("setting '%s' in Cruise Control configuration failed", kafkautils.CruiseControlConfigTopicConfigProviderClass), "config", kafkautils.CruiseControlConfigTopicConfigProviderClassVal)
		}

		if err = ccConfig.Set(kafkautils.CruiseControlConfigKafkaBrokerFailureDetectionEnable, kafkautils.CruiseControlConfigKafkaBrokerFailureDetectionEnableVal); err != nil {
			log.Error(err, fmt.Sprintf("setting '%s' in Cruise Control configuration failed", kafkautils.CruiseControlConfigKafkaBrokerFailureDetectionEnable), "config", kafkautils.CruiseControlConfigKafkaBrokerFailureDetectionEnableVal)
		}
	} else {
		// Add Zookeeper configuration when we are in Zookeeper mode only
		zkConnect := zookeeperutils.PrepareConnectionAddress(r.KafkaCluster.Spec.ZKAddresses, r.KafkaCluster.Spec.GetZkPath())
		if err = ccConfig.Set(kafkautils.KafkaConfigZooKeeperConnect, zkConnect); err != nil {
			log.Error(err, fmt.Sprintf("setting '%s' in Cruise Control configuration failed", kafkautils.KafkaConfigZooKeeperConnect), "config", zkConnect)
		}
	}

	// Add SSL configuration
	sslConf := generateSSLConfig(r.KafkaCluster.Spec, clientPass, log)
	if sslConf.Len() != 0 {
		ccConfig.Merge(sslConf)
	}

	ccConfig.Sort()

	configMap := &corev1.ConfigMap{
		ObjectMeta: templates.ObjectMeta(
			fmt.Sprintf(configAndVolumeNameTemplate, r.KafkaCluster.Name),
			apiutil.MergeLabels(ccLabelSelector(r.KafkaCluster.Name), r.KafkaCluster.Labels),
			r.KafkaCluster,
		),
		Data: map[string]string{
			"cruisecontrol.properties": ccConfig.String(),
			"capacity.json":            capacityConfig,
			"clusterConfigs.json":      r.KafkaCluster.Spec.CruiseControlConfig.ClusterConfig,
			"log4j.properties":         r.KafkaCluster.Spec.CruiseControlConfig.GetCCLog4jConfig(),
		},
	}
	return configMap
}

func generateSSLConfig(kafkaCluster v1beta1.KafkaClusterSpec, clientPass string, log logr.Logger) *properties.Properties {
	config := properties.NewProperties()
	if kafkaCluster.IsClientSSLSecretPresent() && util.IsSSLEnabledForInternalCommunication(kafkaCluster.ListenersConfig.InternalListeners) {
		keyStoreLoc := keystoreVolumePath + "/" + v1alpha1.TLSJKSKeyStore
		trustStoreLoc := keystoreVolumePath + "/" + v1alpha1.TLSJKSTrustStore

		sslConfig := map[string]string{
			kafkautils.KafkaConfigSecurityProtocol:      "SSL",
			kafkautils.KafkaConfigSSLTrustStoreType:     "JKS",
			kafkautils.KafkaConfigSSLKeystoreType:       "JKS",
			kafkautils.KafkaConfigSSLTrustStoreLocation: trustStoreLoc,
			kafkautils.KafkaConfigSSLKeyStoreLocation:   keyStoreLoc,
			kafkautils.KafkaConfigSSLKeyStorePassword:   clientPass,
			kafkautils.KafkaConfigSSLTrustStorePassword: clientPass,
		}

		for k, v := range sslConfig {
			if err := config.Set(k, v); err != nil {
				log.Error(err, fmt.Sprintf("setting '%s' parameter in Cruise Control configuration resulted an error", k))
			}
		}
	}
	return config
}

type CapacityConfig struct {
	BrokerCapacities []BrokerCapacity `json:"brokerCapacities"`
}
type BrokerCapacity struct {
	BrokerID string   `json:"brokerId"`
	Capacity Capacity `json:"capacity"`
	Doc      string   `json:"doc"`
}
type Capacity struct {
	DISK  map[string]string `json:"DISK"`
	CPU   string            `json:"CPU"`
	NWIN  string            `json:"NW_IN"`
	NWOUT string            `json:"NW_OUT"`
}

type JBODInvariantCapacityConfig struct {
	Capacities []interface{} `json:"brokerCapacities"`
}

type previousBrokerCapacity struct {
	raw       json.RawMessage
	diskSizes map[string]string
}

// ErrCapacityDegraded is logged (never returned) for a disk that had to be kept in Cruise Control's
// capacity config while its removal/rebalance is pending, but whose exact capacity could not be
// recovered from the previous capacity.json, so a fallback size was used instead. Cruise Control's
// capacity model is then temporarily inaccurate for that one disk, which is materially less harmful
// than failing the whole reconcile: the disk is being drained anyway, so its capacity ceiling barely
// matters, while a hard error would wedge Cruise Control reconciliation with no self-healing path (the
// missing size is never repopulated by anything else).
var ErrCapacityDegraded = errors.New("could not recover exact capacity for a disk pending a Cruise Control operation, using a fallback capacity until it completes")

// GenerateCapacityConfig generates a CC capacity config with default values or returns the manually overridden value if it exists.
// If an old config is available, it is used in two additive ways so Cruise Control's capacity.json
// never disagrees with what the broker/Kafka are actually still using:
//   - for brokers still present in the desired spec, per-broker disk sizes are recovered from the old
//     config to additively keep disks whose Cruise Control disk removal/rebalance is still in progress
//     (see generateBrokerDisks);
//   - for brokers no longer present in the desired spec (broker deletion in progress), the old config's
//     full broker capacity entry is reused as-is instead of inventing default/placeholder values, since
//     regenerating from spec is not possible once the broker has been removed from it.
//
// Brokers covered by a user-provided Spec.CruiseControlConfig.CapacityConfig entry are left entirely to
// that user-provided config and are therefore NOT subject to the disk-keeping logic above: if such a
// broker has a disk removal/rebalance in progress, the user is responsible for keeping the disk listed
// in their pinned capacity config until it completes, otherwise Cruise Control reports
// "Missing disk information" for the still-mounted log dir.
func GenerateCapacityConfig(kafkaCluster *v1beta1.KafkaCluster, log logr.Logger, config *corev1.ConfigMap) (string, error) {
	var err error

	log.V(2).Info("generating capacity config")

	var capacityConfig JBODInvariantCapacityConfig
	var userConfigBrokerIds []string
	// If there is already a config added manually, use that one
	if kafkaCluster.Spec.CruiseControlConfig.CapacityConfig != "" {
		userProvidedCapacityConfig := kafkaCluster.Spec.CruiseControlConfig.CapacityConfig
		err := json.Unmarshal([]byte(userProvidedCapacityConfig), &capacityConfig)
		if err != nil {
			return "", errors.Wrap(err, "could not unmarshal the user-provided broker capacity config")
		}
		for _, brokerCapacity := range capacityConfig.Capacities {
			brokerCapacityMap, ok := brokerCapacity.(map[string]interface{})
			if !ok {
				continue
			}
			brokerId, ok, err := unstructured.NestedString(brokerCapacityMap, v1beta1.BrokerIdLabelKey)
			if err != nil {
				return "", errors.WrapIfWithDetails(err,
					"could not retrieve broker Id from broker capacity configuration",
					"capacity configuration", brokerCapacityMap)
			}
			if !ok {
				continue
			}
			// If the -1 default exists we don't have to do anything else here since all brokers will have values.
			if brokerId == "-1" {
				log.V(2).Info("Using user provided capacity config because it has universal default defined", "capacity config", userProvidedCapacityConfig)
				return userProvidedCapacityConfig, nil
			}
			userConfigBrokerIds = append(userConfigBrokerIds, brokerId)
		}
	}

	// Recover the old per-broker capacity so it can be reused/merged below: full entries for brokers no
	// longer in the desired spec (broker deletion in progress), and just disk sizes for brokers still in
	// spec whose disk removal/rebalance is in progress.
	var oldBrokerCapacities map[string]previousBrokerCapacity
	if config != nil {
		if data, ok := config.Data["capacity.json"]; ok {
			oldBrokerCapacities, err = parseCapacityConfigBrokerCapacities(data, log)
			if err != nil {
				log.Error(err, "could not parse old Cruise Control capacity config, disks pending removal may not be preserved")
			}
		}
	}

	// If there was no user provided config we shall generate all configuration or
	// adding generated values to all Brokers not provided by the user.
	brokerCapacities, err := appendGeneratedBrokerCapacities(kafkaCluster, log, userConfigBrokerIds, oldBrokerCapacities)
	if err != nil {
		return "", err
	}

	capacityConfig.Capacities = append(capacityConfig.Capacities, brokerCapacities...)
	result, err := json.MarshalIndent(capacityConfig, "", "    ")
	if err != nil {
		return "", errors.WrapIf(err, "could not marshal cruise control capacity config")
	}
	log.V(2).Info("broker capacity config generated successfully", "capacity config", string(result))
	return string(result), nil
}

func appendGeneratedBrokerCapacities(kafkaCluster *v1beta1.KafkaCluster, log logr.Logger, userConfigBrokerIds []string, oldBrokerCapacities map[string]previousBrokerCapacity) ([]interface{}, error) {
	var brokerCapacities []interface{}

	brokerIdFromStatus := make([]string, 0, len(kafkaCluster.Status.BrokersState))
	for brokerId := range kafkaCluster.Status.BrokersState {
		brokerIdFromStatus = append(brokerIdFromStatus, brokerId)
	}
	// Since maps aren't ordered we need to order this list before using it
	sort.Strings(brokerIdFromStatus)

	for _, userConfigBrokerId := range userConfigBrokerIds {
		brokerIdFromStatus = util.StringSliceRemove(brokerIdFromStatus, userConfigBrokerId)
	}

	if len(brokerIdFromStatus) == 0 {
		return nil, nil
	}

	for _, brokerId := range brokerIdFromStatus {
		brokerCapacity := BrokerCapacity{}
		brokerFoundInSpec := false
		for _, broker := range kafkaCluster.Spec.Brokers {
			if brokerId == strconv.Itoa(int(broker.Id)) {
				brokerFoundInSpec = true
				volumeStates := kafkaCluster.Status.BrokersState[brokerId].GracefulActionState.VolumeStates
				brokerDisks, err := generateBrokerDisks(broker, kafkaCluster.Spec, volumeStates, oldBrokerCapacities[brokerId].diskSizes, log)
				if err != nil {
					return nil, errors.WrapIfWithDetails(err, "could not generate broker disks config for broker", v1beta1.BrokerIdLabelKey, broker.Id)
				}
				brokerCapacity = BrokerCapacity{
					BrokerID: strconv.Itoa(int(broker.Id)),
					Capacity: Capacity{
						DISK:  brokerDisks,
						CPU:   generateBrokerCPU(broker, kafkaCluster.Spec, log),
						NWIN:  generateBrokerNetworkIn(broker, kafkaCluster.Spec, log),
						NWOUT: generateBrokerNetworkOut(broker, kafkaCluster.Spec, log),
					},
					Doc: defaultDoc,
				}
			}
		}
		// When removing a broker it still needs to have values assigned in capacity config. Reuse the
		// broker's last known real capacity entry from the old config if available (it can no longer be
		// regenerated from spec once the broker has been removed from it), otherwise fall back to
		// placeholder defaults, since it doesn't really matter what the values are in that case.
		if !brokerFoundInSpec {
			if oldCapacity, found := oldBrokerCapacities[brokerId]; found {
				log.V(1).Info("broker spec not found, reusing last known capacity config", v1beta1.BrokerIdLabelKey, brokerId)
				brokerCapacities = append(brokerCapacities, oldCapacity.raw)
				continue
			}
			log.Info("broker spec not found, using default fallback")
			brokerCapacity = generateDefaultBrokerCapacityWithId(brokerId)
		}
		log.V(1).Info("capacity config successfully generated for broker", "capacity config", brokerCapacity)

		brokerCapacities = append(brokerCapacities, &brokerCapacity)
	}
	return brokerCapacities, nil
}

// Generate default broker capacity
// This value is used by every broker not in the spec, for example when deleting a broker
func generateDefaultBrokerCapacityWithId(brokerId string) BrokerCapacity {
	return BrokerCapacity{
		BrokerID: brokerId,
		Capacity: Capacity{
			DISK: map[string]string{
				"/kafka-logs/kafka": "10737",
			},
			CPU:   "100",
			NWIN:  "125000",
			NWOUT: "125000",
		},
		Doc: defaultDoc,
	}
}

func generateBrokerNetworkIn(broker v1beta1.Broker, kafkaClusterSpec v1beta1.KafkaClusterSpec, log logr.Logger) string {
	brokerConfig, err := broker.GetBrokerConfig(kafkaClusterSpec)
	if err != nil {
		log.V(warnLevel).Info("could not get incoming network resource limits falling back to default value")
		return storageConfigNWINDefaultValue
	}
	if brokerConfig.NetworkConfig != nil && brokerConfig.NetworkConfig.IncomingNetworkThroughPut != "" {
		return brokerConfig.NetworkConfig.IncomingNetworkThroughPut
	}

	log.Info("incoming network throughput is not set falling back to default value")
	return storageConfigNWINDefaultValue
}

func generateBrokerNetworkOut(broker v1beta1.Broker, kafkaClusterSpec v1beta1.KafkaClusterSpec, log logr.Logger) string {
	brokerConfig, err := broker.GetBrokerConfig(kafkaClusterSpec)
	if err != nil {
		log.V(warnLevel).Info("could not get outgoing network resource limits falling back to default value")
		return storageConfigNWOUTDefaultValue
	}
	if brokerConfig.NetworkConfig != nil && brokerConfig.NetworkConfig.OutgoingNetworkThroughPut != "" {
		return brokerConfig.NetworkConfig.OutgoingNetworkThroughPut
	}

	log.Info("outgoing network throughput is not set falling back to default value")
	return storageConfigNWOUTDefaultValue
}

func generateBrokerCPU(broker v1beta1.Broker, kafkaClusterSpec v1beta1.KafkaClusterSpec, log logr.Logger) string {
	brokerConfig, err := broker.GetBrokerConfig(kafkaClusterSpec)
	if err != nil {
		log.V(warnLevel).Info("could not get cpu resource limits falling back to default value")
		return storageConfigCPUDefaultValue
	}

	return strconv.Itoa(int(brokerConfig.GetResources().Limits.Cpu().ScaledValue(-2)))
}

// generateBrokerDisks computes the DISK map used in Cruise Control's capacity.json for a single
// broker. Besides the desired storage configs, it additively keeps any disk that is no longer in the
// desired spec but whose Cruise Control disk removal/rebalance is still in progress
// (kafkautils.KeepRemovedVolume), mirroring the behavior of the broker's own log.dirs
// (pkg/resources/kafka/configmap.go) and mounted PVCs (pkg/resources/kafka/kafka.go). Without this,
// Cruise Control's model would be missing a Disk entry for a log dir that Kafka/the broker pod still
// actively use, causing a permanent "Missing disk information" error while removal is pending.
// The exact size for a kept disk is recovered from oldDiskSizes, the previous capacity.json entry for
// this broker/disk. When that is unavailable (previous ConfigMap deleted, unparseable, or holding a
// non-JBOD scalar DISK value), generation does NOT fail: it falls back to the largest desired disk of
// the same broker, or to MinLogDirSizeInMB when the broker has no desired disk left, and reports a
// CapacityDegradation. A temporarily inaccurate capacity ceiling on a disk that is being drained
// anyway is far less harmful than a hard error, which would wedge Cruise Control reconciliation
// permanently: nothing else ever repopulates the missing size, so every later reconcile would fail
// identically until a human hand-edits the ConfigMap. Omitting the disk is not an option either, as it
// recreates Cruise Control's permanent "Missing disk information" failure. Because the fallback is
// silent as far as reconciliation is concerned, it is logged as an error (ErrCapacityDegraded).
func generateBrokerDisks(brokerState v1beta1.Broker, kafkaClusterSpec v1beta1.KafkaClusterSpec, volumeStates map[string]v1beta1.VolumeState, oldDiskSizes map[string]string, log logr.Logger) (map[string]string, error) {
	storageConfigs := make(map[string]v1beta1.StorageConfig)

	// Get disks from the BrokerConfigGroup if it's in use
	if brokerState.BrokerConfigGroup != "" {
		if b, ok := kafkaClusterSpec.BrokerConfigGroups[brokerState.BrokerConfigGroup]; ok {
			for _, c := range b.StorageConfigs {
				storageConfigs[c.MountPath] = c
			}
		}
	}

	// Get disks from the BrokerConfig itself
	if brokerState.BrokerConfig != nil {
		for _, c := range brokerState.BrokerConfig.StorageConfigs {
			storageConfigs[c.MountPath] = c
		}
	}

	// Generate log dir configuration
	logDirs := make(map[string]string, len(storageConfigs))
	largestDesiredSize := int64(0)
	for path, conf := range storageConfigs {
		size := parseMountPathWithSize(conf)
		log.V(1).Info(fmt.Sprintf("broker log.dir %s size in MB: %d", path, size), v1beta1.BrokerIdLabelKey, brokerState.Id)

		if size < MinLogDirSizeInMB {
			return nil, errors.Errorf("broker log.dir %s size is %dMB which is less than the minimum %dMB",
				path, size, MinLogDirSizeInMB)
		}

		logDir := util.StorageConfigKafkaMountPath(path)
		sizeStr := fmt.Sprintf("%d", size)
		logDirs[logDir] = sizeStr
		largestDesiredSize = max(largestDesiredSize, size)
	}

	// Additively keep disks whose removal/rebalance is still in progress, even though they are no
	// longer part of the desired storage configs above. Iterate in a deterministic order so the
	// generated ConfigMap (and its content hash) never churns between reconciles.
	pendingMountPaths := make([]string, 0, len(volumeStates))
	for mountPath := range volumeStates {
		pendingMountPaths = append(pendingMountPaths, mountPath)
	}
	sort.Strings(pendingMountPaths)

	for _, mountPath := range pendingMountPaths {
		if _, alreadyPresent := storageConfigs[mountPath]; alreadyPresent {
			continue
		}
		if !kafkautils.KeepRemovedVolume(volumeStates, mountPath) {
			continue
		}

		logDir := util.StorageConfigKafkaMountPath(mountPath)
		if size, ok := oldDiskSizes[logDir]; ok {
			logDirs[logDir] = size
			continue
		}

		// The exact previous capacity is unrecoverable. Keep the disk with a best-effort size rather
		// than failing the whole Cruise Control reconcile (see the function doc).
		fallbackSize := max(largestDesiredSize, MinLogDirSizeInMB)
		fallbackSizeStr := strconv.FormatInt(fallbackSize, 10)
		logDirs[logDir] = fallbackSizeStr
		log.Error(ErrCapacityDegraded, "Cruise Control capacity config is temporarily inaccurate for a disk pending removal",
			v1beta1.BrokerIdLabelKey, brokerState.Id,
			"mountPath", mountPath,
			"volumeState", volumeStates[mountPath].CruiseControlVolumeState,
			"fallbackCapacityMB", fallbackSizeStr)
	}

	return logDirs, nil
}

// parseCapacityConfigBrokerCapacities parses a previously generated capacity.json into independent
// per-broker entries. It preserves each raw entry so supported scalar DISK configurations can be
// reused for removed brokers, while extracting per-logdir disk sizes only from JBOD entries. It is used to:
//   - recover the size of a disk that must still be additively kept in the newly generated capacity
//     config while its Cruise Control disk removal/rebalance is in progress (see generateBrokerDisks);
//   - reuse the last known real capacity entry, as-is, for a broker that is no longer present in the
//     desired spec (broker deletion in progress), instead of inventing placeholder values.
func parseCapacityConfigBrokerCapacities(oldCapacityConfig string, log logr.Logger) (map[string]previousBrokerCapacity, error) {
	var parsed struct {
		BrokerCapacities []json.RawMessage `json:"brokerCapacities"`
	}
	if err := json.Unmarshal([]byte(oldCapacityConfig), &parsed); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal old Cruise Control capacity config")
	}

	capacitiesByBroker := make(map[string]previousBrokerCapacity, len(parsed.BrokerCapacities))
	for _, rawBrokerCapacity := range parsed.BrokerCapacities {
		var brokerCapacity struct {
			BrokerID string `json:"brokerId"`
			Capacity struct {
				DISK json.RawMessage `json:"DISK"`
			} `json:"capacity"`
		}
		if err := json.Unmarshal(rawBrokerCapacity, &brokerCapacity); err != nil {
			// Skipping the entry only degrades this one broker (placeholder values on deletion, or a
			// fallback disk size), so log it instead of failing the whole config generation.
			log.V(1).Info("skipping unparseable broker entry in old Cruise Control capacity config",
				"entry", string(rawBrokerCapacity), "error", err.Error())
			continue
		}
		if brokerCapacity.BrokerID == "" {
			log.V(1).Info("skipping broker entry without a brokerId in old Cruise Control capacity config",
				"entry", string(rawBrokerCapacity))
			continue
		}

		previousCapacity := previousBrokerCapacity{raw: rawBrokerCapacity}
		if err := json.Unmarshal(brokerCapacity.Capacity.DISK, &previousCapacity.diskSizes); err != nil {
			// Non-JBOD (scalar) or absent DISK value: the raw entry is still reusable as-is for a
			// removed broker, but no per-logdir sizes can be recovered for a pending disk removal.
			log.V(1).Info("no per-logdir disk sizes recoverable from old Cruise Control capacity config entry",
				v1beta1.BrokerIdLabelKey, brokerCapacity.BrokerID, "error", err.Error())
			previousCapacity.diskSizes = nil
		}
		capacitiesByBroker[brokerCapacity.BrokerID] = previousCapacity
	}
	return capacitiesByBroker, nil
}

func parseMountPathWithSize(storage v1beta1.StorageConfig) int64 {
	var q *resource.Quantity
	if storage.PvcSpec != nil {
		q = util.QuantityPointer(storage.PvcSpec.Resources.Requests["storage"])
	} else if storage.EmptyDir != nil {
		q = storage.EmptyDir.SizeLimit
	}

	var tmpDec = inf.NewDec(0, 0)
	tmpDec.Round(q.AsDec(), -1*inf.Scale(resource.Mega), inf.RoundDown)

	return resource.NewQuantity(tmpDec.UnscaledBig().Int64(), q.Format).Value()
}
