// Copyright 2025 Adobe. All rights reserved.
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

package envoygateway

import (
	"strconv"
	"strings"
)

const (
	// IngressControllerName name for envoy gateway ingress controller
	IngressControllerName = "envoygateway"

	// GatewayNameTemplate template for Gateway resource name
	GatewayNameTemplate = "kafka-gateway-%s"

	// TLSRouteNameTemplate template for TLSRoute resource name
	TLSRouteNameTemplate = "kafka-tlsroute-%s-%s"

	// TCPRouteNameTemplate template for TCPRoute resource name
	TCPRouteNameTemplate = "kafka-tcproute-%s-%s"
)

// GetBrokerHostname returns the broker hostname for the given broker ID
// by replacing %id in the template with the actual broker ID
func GetBrokerHostname(template string, brokerId int32) string {
	return strings.Replace(template, "%id", strconv.Itoa(int(brokerId)), 1)
}
