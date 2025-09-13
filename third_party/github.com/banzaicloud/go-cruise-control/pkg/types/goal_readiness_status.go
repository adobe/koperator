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
	GoalReadinessStatusUndefined GoalReadinessStatus = iota
	GoalReadinessStatusNotReady
	GoalReadinessStatusReady
)

type GoalReadinessStatus int8

func (g GoalReadinessStatus) String() string {
	switch g {
	case GoalReadinessStatusNotReady:
		return "notReady"
	case GoalReadinessStatusReady:
		return "ready"
	case GoalReadinessStatusUndefined:
		fallthrough
	default:
		return Undefined
	}
}

func (g *GoalReadinessStatus) MarshalJSON() ([]byte, error) {
	return []byte(addQuotes(g.String())), nil
}

func (g *GoalReadinessStatus) UnmarshalJSON(data []byte) error {
	switch removeQuotes(string(data)) {
	case GoalReadinessStatusNotReady.String():
		*g = GoalReadinessStatusNotReady
	case GoalReadinessStatusReady.String():
		*g = GoalReadinessStatusReady
	case GoalReadinessStatusUndefined.String():
		fallthrough
	default:
		*g = GoalReadinessStatusUndefined
	}
	return nil
}

func (g *GoalReadinessStatus) UnmarshalText(data []byte) error {
	return g.UnmarshalJSON(data)
}
