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

type AdminResult struct {
	GenericResponse

	SelfHealingEnabledBefore map[AnomalyType]bool `json:"selfHealingEnabledBefore"`
	SelfHealingEnabledAfter  map[AnomalyType]bool `json:"selfHealingEnabledAfter"`

	OngoingConcurrencyChangeRequest string `json:"ongoingConcurrencyChangeRequest"`
	DropRecentBrokersRequest        string `json:"dropRecentBrokersRequest"`

	ConcurrencyAdjusterEnabledBefore map[string]bool `json:"concurrencyAdjusterEnabledBefore"`
	ConcurrencyAdjusterEnabledAfter  map[string]bool `json:"concurrencyAdjusterEnabledAfter"`

	MinIsrBasedConcurrencyAdjustmentRequest string `json:"minIsrBasedConcurrencyAdjustmentRequest"`
}
