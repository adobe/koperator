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

import (
	"fmt"
	"strconv"
)

type BrokerStats struct {
	Version

	Hosts   []HostLoadStats   `json:"hosts"`
	Brokers []BrokerLoadStats `json:"brokers"`
}

const (
	BrokerStateUndefined BrokerState = iota
	BrokerStateAlive
	BrokerStateDead
	BrokerStateNew
	BrokerStateDemoted
	BrokerStateBadDisks
)

type BrokerState int8

func (s BrokerState) String() string {
	switch s {
	case BrokerStateAlive:
		return "ALIVE"
	case BrokerStateDead:
		return "DEAD"
	case BrokerStateNew:
		return "NEW"
	case BrokerStateDemoted:
		return "DEMOTED"
	case BrokerStateBadDisks:
		return "BAD_DISKS"
	case BrokerStateUndefined:
		fallthrough
	default:
		return Undefined
	}
}

func (s BrokerState) MarshalJSON() ([]byte, error) {
	return []byte(addQuotes(s.String())), nil
}

func (s *BrokerState) UnmarshalJSON(data []byte) error {
	d := removeQuotes(string(data))

	switch d {
	case BrokerStateAlive.String():
		*s = BrokerStateAlive
	case BrokerStateDead.String():
		*s = BrokerStateDead
	case BrokerStateNew.String():
		*s = BrokerStateNew
	case BrokerStateDemoted.String():
		*s = BrokerStateDemoted
	case BrokerStateBadDisks.String():
		*s = BrokerStateBadDisks
	default:
		*s = BrokerStateUndefined
	}
	return nil
}

func (s *BrokerState) UnmarshalText(data []byte) error {
	return s.UnmarshalJSON(data)
}

type HostLoadStats struct {
	FollowerNwInRate   float64 `json:"FollowerNwInRate"`
	NwOutRate          float64 `json:"NwOutRate"`
	NumCore            float64 `json:"NumCore"`
	Host               string  `json:"Host"`
	CPUPct             float64 `json:"CpuPct"`
	Replicas           int32   `json:"Replicas"`
	NetworkInCapacity  float64 `json:"NetworkInCapacity"`
	Rack               string  `json:"Rack"`
	Leaders            int32   `json:"Leaders"`
	DiskCapacityMB     float64 `json:"DiskCapacityMB"`
	DiskMB             float64 `json:"DiskMB"`
	PnwOutRate         float64 `json:"PnwOutRate"`
	NetworkOutCapacity float64 `json:"NetworkOutCapacity"`
	LeaderNwInRate     float64 `json:"LeaderNwInRate"`
	DiskPct            float64 `json:"DiskPct"`
}

type BrokerLoadStats struct {
	FollowerNwInRate   float64              `json:"FollowerNwInRate"`
	BrokerState        BrokerState          `json:"BrokerState"`
	Broker             int32                `json:"Broker"`
	NwOutRate          float64              `json:"NwOutRate"`
	NumCore            float64              `json:"NumCore"`
	Host               string               `json:"Host"`
	CPUPct             float64              `json:"CpuPct"`
	Replicas           int32                `json:"Replicas"`
	NetworkInCapacity  float64              `json:"NetworkInCapacity"`
	Rack               string               `json:"Rack"`
	Leaders            int32                `json:"Leaders"`
	DiskCapacityMB     float64              `json:"DiskCapacityMB"`
	DiskMB             float64              `json:"DiskMB"`
	PnwOutRate         float64              `json:"PnwOutRate"`
	NetworkOutCapacity float64              `json:"NetworkOutCapacity"`
	LeaderNwInRate     float64              `json:"LeaderNwInRate"`
	DiskPct            float64              `json:"DiskPct"`
	DiskState          map[string]DiskStats `json:"DiskState"`
}

const (
	DiskUsageStatDead = "DEAD"
)

type DiskStats struct {
	DiskMB            DiskUsageStat `json:"DiskMB"`
	DiskPct           DiskUsageStat `json:"DiskPct"`
	NumLeaderReplicas int32         `json:"NumLeaderReplicas"`
	NumReplicas       int32         `json:"NumReplicas"`
}

type DiskUsageStat struct {
	Dead  bool
	Usage float64
}

func (s *DiskUsageStat) UnmarshalJSON(data []byte) error {
	d := removeQuotes(string(data))

	s.Usage = 0.0
	s.Dead = false

	if d == DiskUsageStatDead {
		s.Dead = true
	} else {
		var err error
		s.Usage, err = strconv.ParseFloat(d, 64) //nolint:gomnd
		if err != nil {
			return fmt.Errorf("failed to parse disk usage: %w", err)
		}
	}
	return nil
}
