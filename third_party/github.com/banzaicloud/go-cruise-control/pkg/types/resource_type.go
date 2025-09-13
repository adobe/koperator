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
	ResourceTypeUndefined ResourceType = iota
	ResourceTypeCPU
	ResourceTypeDisk
	ResourceTypeNetworkIn
	ResourceTypeNetworkOut
)

type ResourceType int8

func (g ResourceType) String() string {
	switch g {
	case ResourceTypeCPU:
		return "cpu"
	case ResourceTypeDisk:
		return "disk"
	case ResourceTypeNetworkIn:
		return "networkInbound"
	case ResourceTypeNetworkOut:
		return "networkOutbound"
	case ResourceTypeUndefined:
		fallthrough
	default:
		return Undefined
	}
}

func (g *ResourceType) MarshalJSON() ([]byte, error) {
	return []byte(addQuotes(g.String())), nil
}

func (g *ResourceType) UnmarshalJSON(data []byte) error {
	switch removeQuotes(string(data)) {
	case ResourceTypeCPU.String():
		*g = ResourceTypeCPU
	case ResourceTypeDisk.String():
		*g = ResourceTypeDisk
	case ResourceTypeNetworkIn.String():
		*g = ResourceTypeNetworkIn
	case ResourceTypeNetworkOut.String():
		*g = ResourceTypeNetworkOut
	case ResourceTypeUndefined.String():
		fallthrough
	default:
		*g = ResourceTypeUndefined
	}

	return nil
}

func (g *ResourceType) UnmarshalText(data []byte) error {
	return g.UnmarshalJSON(data)
}
