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

import "fmt"

type Params map[string][]string

func (p Params) Get(key string) string {
	if p == nil {
		return ""
	}
	k := p[key]
	if len(k) == 0 {
		return ""
	}
	return k[0]
}

func (p Params) Add(key string, value ...string) {
	p[key] = append(p[key], value...)
}

func (p Params) Set(key string, value ...string) {
	p[key] = value
}

func (p Params) Del(key string) {
	delete(p, key)
}

func (p Params) Has(key string) bool {
	_, ok := p[key]
	return ok
}

func (p Params) Values(key string) []string {
	if v, ok := p[key]; ok {
		return v
	}
	return nil
}

func (p Params) Merge(m Params) {
	for k, v := range m {
		if p.Has(k) {
			p[k] = append(p[k], v...)
			continue
		}
		p[k] = v
	}
}

func (p Params) List() []string {
	l := make([]string, 0)

	for k, v := range p {
		for _, vv := range v {
			l = append(l, fmt.Sprintf("%s-%s", k, vv))
		}
	}
	return l
}
