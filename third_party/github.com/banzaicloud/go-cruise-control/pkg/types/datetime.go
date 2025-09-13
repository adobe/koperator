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
	"time"
)

type DateTime struct {
	time.Time
}

func (d *DateTime) UnmarshalJSON(b []byte) error {
	t, err := strconv.Atoi(removeQuotes(string(b)))
	if err != nil {
		return fmt.Errorf("failed to parse datetime: %w", err)
	}
	d.Time = time.UnixMilli(int64(t))
	return nil
}
