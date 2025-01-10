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

package kafka

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"

	apiutil "github.com/banzaicloud/koperator/api/util"
	"github.com/banzaicloud/koperator/pkg/resources/templates"
	"github.com/banzaicloud/koperator/pkg/util"

	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *Reconciler) podDisruptionBudgetBrokers(log logr.Logger) (runtime.Object, error) {
	var podSelectorLabels map[string]string
	minAvailable, err := r.computeMinAvailable(log)

	if err != nil {
		return nil, err
	}

	if r.KafkaCluster.Spec.KRaftMode {
		podSelectorLabels = apiutil.LabelsForBroker(r.KafkaCluster.Name)
	} else {
		podSelectorLabels = apiutil.LabelsForKafka(r.KafkaCluster.Name)
	}

	return r.podDisruptionBudget(fmt.Sprintf("%s-pdb", r.KafkaCluster.Name),
		podSelectorLabels,
		minAvailable)
}

func (r *Reconciler) podDisruptionBudgetControllers(log logr.Logger) (runtime.Object, error) {
	if !r.KafkaCluster.Spec.KRaftMode {
		return nil, errors.New("PDB for controllers is only applicable when in KRaft mode")
	}

	minAvailable, err := r.computeControllerMinAvailable()

	if err != nil {
		log.Error(err, "error occurred during computing minAvailable for controllers PDB")
		return nil, err
	}

	return r.podDisruptionBudget(fmt.Sprintf("%s-controller-pdb", r.KafkaCluster.Name),
		apiutil.LabelsForController(r.KafkaCluster.Name),
		minAvailable)
}

func (r *Reconciler) podDisruptionBudget(name string, podSelectorLabels map[string]string, minAvailable intstr.IntOrString) (runtime.Object, error) {
	return &policyv1.PodDisruptionBudget{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PodDisruptionBudget",
			APIVersion: "policy/v1",
		},
		ObjectMeta: templates.ObjectMetaWithAnnotations(
			name,
			apiutil.MergeLabels(podSelectorLabels, r.KafkaCluster.Labels),
			r.KafkaCluster.Spec.ListenersConfig.GetServiceAnnotations(),
			r.KafkaCluster,
		),
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailable,
			Selector: &metav1.LabelSelector{
				MatchLabels: podSelectorLabels,
			},
		},
	}, nil
}

func (r *Reconciler) getControllerCount(controllerRoleOnly bool) (int, error) {
	controllerCount := 0
	for _, broker := range r.KafkaCluster.Spec.Brokers {
		brokerConfig, err := broker.GetBrokerConfig(r.KafkaCluster.Spec)
		if err != nil {
			return -1, err
		}
		if controllerRoleOnly {
			if brokerConfig.IsControllerOnlyNode() {
				controllerCount++
			}
		} else if brokerConfig.IsControllerNode() {
			controllerCount++
		}
	}
	return controllerCount, nil
}

// Calculate minAvailable as max between brokerCount - 1 (so we only allow 1 controller to be disrupted)
// and 1 (case when there is only 1 controller)
func (r *Reconciler) computeControllerMinAvailable() (intstr.IntOrString, error) {
	controllerCount, err := r.getControllerCount(false)
	if err != nil {
		return intstr.FromInt(-1), err
	}
	minAvailable := int(math.Max(float64(controllerCount-1), float64(1)))
	return intstr.FromInt(minAvailable), nil
}

// Calculate maxUnavailable as max between brokerCount - 1 (so we only allow 1 broker to be disrupted)
// and 1 (to cover for 1 broker clusters)
func (r *Reconciler) computeMinAvailable(log logr.Logger) (intstr.IntOrString, error) {
	/*
		budget = r.KafkaCluster.Spec.DisruptionBudget.budget (string) ->
		- can either be %percentage or static number

		Logic:

		Max(1, brokers-budget) - for a static number budget

		Max(1, brokers-brokers*percentage) - for a percentage budget

	*/

	controllerCount, err := r.getControllerCount(true)
	if err != nil {
		log.Error(err, "error occurred during get controller count")
		return intstr.FromInt(-1), err
	}

	// number of brokers in the KafkaCluster.  Controllers are reported in the BrokerState so we must deduct it.
	brokers := len(r.KafkaCluster.Status.BrokersState) - controllerCount

	// configured budget in the KafkaCluster
	disruptionBudget := r.KafkaCluster.Spec.DisruptionBudget.Budget

	budget := 0

	// treat percentage budget
	if strings.HasSuffix(disruptionBudget, "%") {
		percentage, err := strconv.ParseFloat(disruptionBudget[:len(disruptionBudget)-1], 32)
		if err != nil {
			log.Error(err, "error occurred during parsing the disruption budget")
			return intstr.FromInt(-1), err
		}
		budget = int(math.Floor((percentage * float64(brokers)) / 100))
	} else {
		// treat static number budget
		staticBudget, err := strconv.ParseInt(disruptionBudget, 10, 0)
		if err != nil {
			log.Error(err, "error occurred during parsing the disruption budget")
			return intstr.FromInt(-1), err
		}
		budget = int(staticBudget)
	}

	return intstr.FromInt(util.Max(1, brokers-budget)), nil
}
