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
	ProposalDataSourceUndefined ProposalDataSource = iota
	ProposalDataSourceValidWindows
	ProposalDataSourceValidPartitions
)

type ProposalDataSource int8

func (s ProposalDataSource) String() string {
	switch s {
	case ProposalDataSourceValidWindows:
		return "VALID_WINDOWS"
	case ProposalDataSourceValidPartitions:
		return "VALID_PARTITIONS"
	case ProposalDataSourceUndefined:
		fallthrough
	default:
		return Undefined
	}
}

func (s *ProposalDataSource) MarshalJSON() ([]byte, error) {
	return []byte(addQuotes(s.String())), nil
}

func (s *ProposalDataSource) UnmarshalJSON(data []byte) error {
	switch removeQuotes(string(data)) {
	case ProposalDataSourceValidWindows.String():
		*s = ProposalDataSourceValidWindows
	case ProposalDataSourceValidPartitions.String():
		*s = ProposalDataSourceValidPartitions
	case ProposalDataSourceUndefined.String():
		fallthrough
	default:
		*s = ProposalDataSourceUndefined
	}
	return nil
}

func (s *ProposalDataSource) UnmarshalText(data []byte) error {
	return s.UnmarshalJSON(data)
}
