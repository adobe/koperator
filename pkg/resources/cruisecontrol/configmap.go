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

	// PropertiesConfigMapKey, ClusterConfigsConfigMapKey, Log4jConfigMapKey and CapacityConfigMapKey are the
	// keys under which the four hashed entries are stored in the Cruise Control ConfigMap. Exported so all three
	// sites that must agree on them - the write side (configMap), the pod-template stamp side
	// (GeneratePodAnnotations) and the operation controller's roll gate - share one source of truth instead of
	// duplicating the literals, where a rename would silently break a lookup (make the gate hash an empty
	// string, never match the deployed annotation, and defer operations forever - the #301 stall class).
	PropertiesConfigMapKey     = "cruisecontrol.properties"
	ClusterConfigsConfigMapKey = "clusterConfigs.json"
	Log4jConfigMapKey          = "log4j.properties"
	CapacityConfigMapKey       = "capacity.json"
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
			PropertiesConfigMapKey:     ccConfig.String(),
			CapacityConfigMapKey:       capacityConfig,
			ClusterConfigsConfigMapKey: r.KafkaCluster.Spec.CruiseControlConfig.ClusterConfig,
			Log4jConfigMapKey:          r.KafkaCluster.Spec.CruiseControlConfig.GetCCLog4jConfig(),
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

// GenerateCapacityConfig generates a CC capacity config with default values or returns the manually overridden value if it exists
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
			brokerId, err := brokerIDFromCapacityEntry(brokerCapacity)
			if err != nil {
				return "", err
			}
			if brokerId == "" {
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
	// During a scaling operation the CR does not carry capacity data for every broker Cruise Control still
	// knows about (a broker dropped from the spec but not yet deleted from CC), so we reuse the already
	// deployed capacity.json instead of regenerating a fallback entry for it - regenerating changes
	// capacity.json, which rolls the CC Deployment mid-scaling (see #301 and isBrokerRemovalPending). We
	// only ADD entries for brokers that have newly joined (present in the spec/status but missing from the
	// deployed config) so a mixed add+remove edit still writes the new broker's capacity before add_broker.
	// A departing broker keeps its deployed entry until its pod is gone.
	if config != nil {
		if data, ok := config.Data[CapacityConfigMapKey]; ok {
			// While a downscale is actively running there is a remove_broker task in flight on the CC pod;
			// appending a concurrently-added broker here would change capacity.json, roll the CC Deployment,
			// and kill that in-flight task - the exact #301 failure class. Reuse the deployed config verbatim
			// in that window. The added broker's capacity is written by the next full regeneration once the
			// downscale completes, and its add_broker is deferred until then by the operation controller's roll
			// gate (requeueIfCCDeploymentNotRolledOut), so nothing is lost.
			if isBrokerDeletionInProgress(kafkaCluster.Status.BrokersState) {
				return data, nil
			}
			return mergeCapacityConfig(kafkaCluster, log, data, capacityConfig.Capacities, userConfigBrokerIds)
		}
	}

	// If there was no user provided config we shall generate all configuration or
	// adding generated values to all Brokers not provided by the user.
	brokerCapacities, err := appendGeneratedBrokerCapacities(kafkaCluster, log, userConfigBrokerIds)
	if err != nil {
		return "", err
	}

	capacityConfig.Capacities = append(capacityConfig.Capacities, brokerCapacities...)
	result, err := json.MarshalIndent(capacityConfig, "", "    ")
	if err != nil {
		return "", errors.WrapIf(err, "could not marshal cruise control capacity config")
	}
	log.V(2).Info("broker capacity config generated successfully", "capacity config", string(result))
	return string(result), err
}

// mergeCapacityConfig returns the already-deployed capacity.json augmented with entries for brokers that
// have joined the cluster since it was written (present in the spec/status but absent from the deployed
// config). A newly added broker takes its user-provided capacity when the CR defines one, otherwise a
// generated entry, so add_broker always has capacity data - even in a mixed add+remove edit. Entries
// already in the deployed config - including brokers being removed (dropped from the spec but still in the
// status) - are preserved verbatim so their capacity is not rewritten to the fallback default; rewriting it
// would change capacity.json and roll Cruise Control mid-scaling (see #301).
//
// When no broker needs to be added the deployed config is returned byte-for-byte unchanged so no spurious
// roll happens - this preserves the pre-existing "reuse the deployed capacity.json during downscale"
// behaviour exactly, while additionally covering a mixed add+remove edit. A side effect of preserving
// deployed entries verbatim is that a capacity change to a broker that stays in the cluster (e.g. a disk
// resize) is not written until the reuse window closes and full regeneration resumes; this is invoked only
// while a broker is departing (see isBrokerRemovalPending / isBrokerDeletionInProgress in Reconcile).
//
// The reuse window is bounded only for a healthy downscale: it closes once the departing broker's pod is gone
// and it drops out of the status. If a removal wedges - the broker stays in the status but out of the spec
// indefinitely (a stuck/paused downscale that is never resolved) - isBrokerRemovalPending stays true and this
// suppression of staying-brokers' capacity changes is unbounded until an operator resolves the removal. That
// is an accepted trade-off: a wedged downscale already needs manual investigation, and rolling CC to write an
// unrelated capacity change while a removal is stuck would not help.
func mergeCapacityConfig(kafkaCluster *v1beta1.KafkaCluster, log logr.Logger, deployedCapacityConfig string, userCapacities []interface{}, userConfigBrokerIds []string) (string, error) {
	var deployed JBODInvariantCapacityConfig
	if err := json.Unmarshal([]byte(deployedCapacityConfig), &deployed); err != nil {
		// The deployed capacity.json is unexpectedly unparseable; keep reusing it verbatim rather than
		// regenerating from scratch, which could roll Cruise Control mid-scaling.
		log.Error(err, "could not parse deployed cruise control capacity config, reusing it verbatim")
		return deployedCapacityConfig, nil
	}

	// Broker ids already present in the deployed config. Their entries win (kept verbatim) so we neither
	// generate nor re-add them, which avoids rolling Cruise Control for brokers that already have capacity.
	deployedBrokerIds := make(map[string]struct{}, len(deployed.Capacities))
	for _, brokerCapacity := range deployed.Capacities {
		brokerId, err := brokerIDFromCapacityEntry(brokerCapacity)
		if err != nil {
			return "", err
		}
		if brokerId == "-1" {
			// A "-1" universal-default entry already covers every broker, so nothing needs appending and
			// appending a redundant per-broker entry would only change capacity.json and roll CC. Reuse
			// verbatim. (Unreachable via the normal flow - a user "-1" makes GenerateCapacityConfig return
			// early before merge - but guard against a deployed config that ever carries one.)
			return deployedCapacityConfig, nil
		}
		if brokerId != "" {
			deployedBrokerIds[brokerId] = struct{}{}
		}
	}

	// User-provided capacities for brokers that are not yet in the deployed config are the explicit
	// capacity for a newly added broker and must be carried over verbatim (finding: a mixed add+remove
	// edit must not drop the new broker's user-provided capacity).
	var newBrokerCapacities []interface{}
	for _, userCapacity := range userCapacities {
		brokerId, err := brokerIDFromCapacityEntry(userCapacity)
		if err != nil {
			return "", err
		}
		if brokerId == "" {
			continue
		}
		if _, ok := deployedBrokerIds[brokerId]; !ok {
			newBrokerCapacities = append(newBrokerCapacities, userCapacity)
		}
	}

	// Generate entries for the remaining newly joined brokers (missing from the deployed config and not
	// covered by a user-provided capacity), matching the non-reuse path's generation.
	coveredBrokerIds := append([]string(nil), userConfigBrokerIds...)
	for brokerId := range deployedBrokerIds {
		coveredBrokerIds = append(coveredBrokerIds, brokerId)
	}
	generatedBrokerCapacities, err := appendGeneratedBrokerCapacities(kafkaCluster, log, coveredBrokerIds)
	if err != nil {
		return "", err
	}
	newBrokerCapacities = append(newBrokerCapacities, generatedBrokerCapacities...)

	// No broker has joined since the config was deployed: reuse it verbatim so Cruise Control is not rolled.
	if len(newBrokerCapacities) == 0 {
		return deployedCapacityConfig, nil
	}

	deployed.Capacities = append(deployed.Capacities, newBrokerCapacities...)
	result, err := json.MarshalIndent(deployed, "", "    ")
	if err != nil {
		return "", errors.WrapIf(err, "could not marshal merged cruise control capacity config")
	}
	log.V(1).Info("merged newly added brokers into the deployed capacity config", "capacity config", string(result))
	return string(result), nil
}

// CapacityConfigContainsBrokers reports whether the given capacity.json defines a capacity entry for every
// broker id in brokerIDs. A "-1" universal-default entry counts as covering every broker. It lets a caller
// confirm a newly added broker's capacity has actually been written before acting on it.
func CapacityConfigContainsBrokers(capacityConfigJSON string, brokerIDs []string) (bool, error) {
	var capacityConfig JBODInvariantCapacityConfig
	if err := json.Unmarshal([]byte(capacityConfigJSON), &capacityConfig); err != nil {
		return false, errors.Wrap(err, "could not unmarshal capacity config")
	}
	present := make(map[string]struct{}, len(capacityConfig.Capacities))
	for _, entry := range capacityConfig.Capacities {
		brokerID, err := brokerIDFromCapacityEntry(entry)
		if err != nil {
			return false, err
		}
		if brokerID == "-1" {
			// Universal default: every broker is covered.
			return true, nil
		}
		if brokerID != "" {
			present[brokerID] = struct{}{}
		}
	}
	for _, id := range brokerIDs {
		if _, ok := present[id]; !ok {
			return false, nil
		}
	}
	return true, nil
}

// brokerIDFromCapacityEntry extracts the "brokerId" field from a capacity.json entry. It returns an empty
// string (no error) when the entry is not a JSON object or has no broker id, matching how the rest of the
// capacity handling tolerates heterogeneous JBOD/non-JBOD entries.
func brokerIDFromCapacityEntry(entry interface{}) (string, error) {
	entryMap, ok := entry.(map[string]interface{})
	if !ok {
		return "", nil
	}
	brokerId, ok, err := unstructured.NestedString(entryMap, v1beta1.BrokerIdLabelKey)
	if err != nil {
		return "", errors.WrapIfWithDetails(err,
			"could not retrieve broker Id from broker capacity configuration",
			"capacity configuration", entryMap)
	}
	if !ok {
		return "", nil
	}
	return brokerId, nil
}

func appendGeneratedBrokerCapacities(kafkaCluster *v1beta1.KafkaCluster, log logr.Logger, userConfigBrokerIds []string) ([]interface{}, error) {
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
				brokerDisks, err := generateBrokerDisks(broker, kafkaCluster.Spec, log)
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
		// When removing a broker it still needs to have values assigned in capacity config
		// although it doesn't really matter what the values are, so we are setting defaults
		// here, this way we don't have to deal with a universal default.
		if !brokerFoundInSpec {
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

func generateBrokerDisks(brokerState v1beta1.Broker, kafkaClusterSpec v1beta1.KafkaClusterSpec, log logr.Logger) (map[string]string, error) {
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
	for path, conf := range storageConfigs {
		size := parseMountPathWithSize(conf)
		log.V(1).Info(fmt.Sprintf("broker log.dir %s size in MB: %d", path, size), v1beta1.BrokerIdLabelKey, brokerState.Id)

		if size < MinLogDirSizeInMB {
			return nil, errors.Errorf("broker log.dir %s size is %dMB which is less than the minimum %dMB",
				path, size, MinLogDirSizeInMB)
		}

		logDir := util.StorageConfigKafkaMountPath(path)
		logDirs[logDir] = fmt.Sprintf("%d", size)
	}

	return logDirs, nil
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
