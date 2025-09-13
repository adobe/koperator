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
	ConcurrencyTypeUndefined ConcurrencyType = iota
	ConcurrencyTypeInterBrokerReplica
	ConcurrencyTypeLeadership
	ConcurrencyTypeIntraBrokerReplica
)

type ConcurrencyType int8

func (s ConcurrencyType) String() string {
	switch s {
	case ConcurrencyTypeInterBrokerReplica:
		return "INTER_BROKER_REPLICA"
	case ConcurrencyTypeLeadership:
		return "LEADERSHIP"
	case ConcurrencyTypeIntraBrokerReplica:
		return "INTRA_BROKER_REPLICA"
	case ConcurrencyTypeUndefined:
		fallthrough
	default:
		return Undefined
	}
}

func (s *ConcurrencyType) MarshalJSON() ([]byte, error) {
	return []byte(addQuotes(s.String())), nil
}

func (s *ConcurrencyType) UnmarshalJSON(data []byte) error {
	switch removeQuotes(string(data)) {
	case ConcurrencyTypeInterBrokerReplica.String():
		*s = ConcurrencyTypeInterBrokerReplica
	case ConcurrencyTypeLeadership.String():
		*s = ConcurrencyTypeLeadership
	case ConcurrencyTypeIntraBrokerReplica.String():
		*s = ConcurrencyTypeIntraBrokerReplica
	case ConcurrencyTypeUndefined.String():
		fallthrough
	default:
		*s = ConcurrencyTypeUndefined
	}
	return nil
}

func (s *ConcurrencyType) UnmarshalText(data []byte) error {
	return s.UnmarshalJSON(data)
}
