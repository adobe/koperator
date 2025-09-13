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

package client

import "strings"

type ContentType struct {
	MIMEType string
	ChartSet string
	Boundary string
}

func parseContentType(s string) ContentType {
	contentType := ContentType{}

	ct := strings.Split(s, ";")
	contentType.MIMEType = strings.ToLower(strings.TrimSpace(ct[0]))
	if len(ct) > 1 {
		for _, field := range ct[1:] {
			f := strings.ToLower(strings.TrimSpace(field))
			switch {
			case strings.HasPrefix(f, "charset="):
				contentType.ChartSet = strings.Split(f, "=")[1]
			case strings.HasPrefix(f, "boundary="):
				contentType.Boundary = strings.Split(f, "=")[1]
			default:
				continue
			}
		}
	}
	return contentType
}
