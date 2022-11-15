// Copyright © 2020 Banzai Cloud
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
	"context"

	"github.com/banzaicloud/go-cruise-control/pkg/api"
	"github.com/banzaicloud/go-cruise-control/pkg/client"
	"github.com/banzaicloud/go-cruise-control/pkg/types"
	"github.com/go-logr/logr"

	"github.com/magiconair/properties"
	"golang.org/x/exp/maps"

	"github.com/banzaicloud/koperator/api/v1beta1"
)

var configToAnomalyMapper = map[string]types.AnomalyType{
	"self.healing.broker.failure.enabled":    types.AnomalyTypeBrokerFailure,
	"self.healing.disk.failure.enabled":      types.AnomalyTypeDiskFailure,
	"self.healing.goal.violation.enabled":    types.AnomalyTypeGoalViolation,
	"self.healing.maintenance.event.enabled": types.AnomalyTypeMaintenanceEvent,
	"self.healing.metric.anomaly.enabled":    types.AnomalyTypeMetricAnomaly,
	"self.healing.topic.anomaly.enabled":     types.AnomalyTypeTopicAnomaly,
}

type cruiseControlHealer struct {
	CruiseControlHealer

	log                 logr.Logger
	cruiseControlConfig v1beta1.CruiseControlConfig
	client              *client.Client
}

var newCruiseControlHealer = createNewCruiseControlHealer

func NewCruiseControlHealer(ctx context.Context, instance *v1beta1.KafkaCluster) (CruiseControlHealer, error) {
	return newCruiseControlHealer(ctx, instance)
}

func createNewCruiseControlHealer(ctx context.Context, instance *v1beta1.KafkaCluster) (CruiseControlHealer, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("CCHealerStatus")

	serverURL := CruiseControlURLFromKafkaCluster(instance)

	cfg := &client.Config{
		ServerURL: serverURL,
		UserAgent: "koperator",
	}

	cruisecontrol, err := client.NewClient(ctx, cfg)
	if err != nil {
		log.Error(err, "creating Cruise Control client failed")
		return nil, err
	}

	return &cruiseControlHealer{
		log:                 log,
		client:              cruisecontrol,
		cruiseControlConfig: instance.Spec.CruiseControlConfig,
	}, nil
}

func (cc *cruiseControlHealer) PauseSelfHealing() error {
	req := api.AdminRequest{}

	req.DisableSelfHealingFor = maps.Values(configToAnomalyMapper)

	cc.log.Info("Disabling self healing", "anomalyTypes", req.DisableSelfHealingFor)

	_, err := cc.client.Admin(&req)
	if err != nil {
		cc.log.Error(err, "failed to disable self healing")
		return err
	}

	return nil
}
func (cc *cruiseControlHealer) ResumeSelfHealing() error {
	req := api.AdminRequest{}
	req.EnableSelfHealingFor = []types.AnomalyType{}

	properties := properties.MustLoadString(cc.cruiseControlConfig.Config)

	selfHealingEnabled := properties.GetBool("self.healing.enabled", false)

	for prop, anomaly := range configToAnomalyMapper {
		if properties.GetBool(prop, selfHealingEnabled) {
			req.EnableSelfHealingFor = append(req.EnableSelfHealingFor, anomaly)
		}
	}

	cc.log.Info("Enabling self healing", "anomalyTypes", req.EnableSelfHealingFor)

	_, err := cc.client.Admin(&req)
	if err != nil {
		cc.log.Error(err, "failed to enable self healing")
		return err
	}

	return nil
}
