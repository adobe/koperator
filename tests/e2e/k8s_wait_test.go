// Copyright © 2023 Cisco Systems, Inc. and/or its affiliates
// Copyright 2026 Adobe. All rights reserved.
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

//go:build e2e

package e2e

import (
	"testing"

	"emperror.dev/errors"
)

func TestIsTransientResourceWaitError(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error is not transient",
			err:  nil,
			want: false,
		},
		{
			name: "object deleted while waiting (rolling update) is transient",
			err:  errors.New(`error while running command: exit status 1; Error from server (NotFound): pods "kafka-2-h4grv" not found`),
			want: true,
		},
		{
			name: "selector momentarily matched no objects is transient",
			err:  errors.New("error: no matching resources found"),
			want: true,
		},
		{
			name: "condition not met before timeout is NOT transient",
			err:  errors.New("error: timed out waiting for the condition on pods/kafka-0"),
			want: false,
		},
		{
			name: "unrelated failure is NOT transient",
			err:  errors.New("error while running command: exit status 1; The connection to the server was refused"),
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTransientResourceWaitError(tc.err); got != tc.want {
				t.Errorf("isTransientResourceWaitError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
