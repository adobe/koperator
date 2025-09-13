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
)

type BrokerIDAndLogDirs map[int32][]string

func (d BrokerIDAndLogDirs) MarshalParams(key string) (Params, error) {
	p := make(Params)

	for id, logdirs := range d {
		for _, dir := range logdirs {
			p.Add(key, fmt.Sprintf("%d-%s", id, dir))
		}
	}

	return p, nil
}
