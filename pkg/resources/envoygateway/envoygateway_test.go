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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/resources"
)

func TestGatewayGeneration(t *testing.T) {
	cluster := &v1beta1.KafkaCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-namespace",
		},
		Spec: v1beta1.KafkaClusterSpec{
			Brokers: []v1beta1.Broker{
				{Id: 0},
				{Id: 1},
				{Id: 2},
			},
			EnvoyGatewayConfig: v1beta1.EnvoyGatewayIngressConfig{
				GatewayClassName: "test-gateway-class",
			},
		},
	}

	reconciler := &Reconciler{
		Reconciler: resources.Reconciler{
			KafkaCluster: cluster,
		},
	}

	eListener := v1beta1.ExternalListenerConfig{
		CommonListenerSpec: v1beta1.CommonListenerSpec{
			Name:          "test-listener",
			ContainerPort: 9092,
		},
		ExternalStartingPort: 19090,
	}

	ingressConfig := v1beta1.IngressConfig{
		EnvoyGatewayConfig: &cluster.Spec.EnvoyGatewayConfig,
	}

	gateway := reconciler.gateway(eListener, ingressConfig)

	gw, ok := gateway.(*gatewayv1.Gateway)
	if !ok {
		t.Fatal("Expected Gateway type")
	}

	if gw.Name != "kafka-gateway-test-listener" {
		t.Errorf("Expected gateway name 'kafka-gateway-test-listener', got '%s'", gw.Name)
	}

	if string(gw.Spec.GatewayClassName) != "test-gateway-class" {
		t.Errorf("Expected gateway class 'test-gateway-class', got '%s'", gw.Spec.GatewayClassName)
	}

	// 3 brokers + 1 anycast = 4 listeners
	if len(gw.Spec.Listeners) != 4 {
		t.Errorf("Expected 4 listeners, got %d", len(gw.Spec.Listeners))
	}

	// Check broker listeners
	for i := 0; i < 3; i++ {
		expectedName := gatewayv1.SectionName("broker-" + string(rune('0'+i)))
		if gw.Spec.Listeners[i].Name != expectedName {
			t.Errorf("Expected listener name '%s', got '%s'", expectedName, gw.Spec.Listeners[i].Name)
		}
		expectedPort := gatewayv1.PortNumber(19090 + i)
		if gw.Spec.Listeners[i].Port != expectedPort {
			t.Errorf("Expected port %d, got %d", expectedPort, gw.Spec.Listeners[i].Port)
		}
	}

	// Check anycast listener
	if gw.Spec.Listeners[3].Name != "anycast" {
		t.Errorf("Expected anycast listener name 'anycast', got '%s'", gw.Spec.Listeners[3].Name)
	}
	// Anycast listener should use the default anycast port (29092), not ExternalStartingPort
	expectedAnycastPort := gatewayv1.PortNumber(29092)
	if gw.Spec.Listeners[3].Port != expectedAnycastPort {
		t.Errorf("Expected anycast port %d, got %d", expectedAnycastPort, gw.Spec.Listeners[3].Port)
	}
}

func TestTCPRouteGeneration(t *testing.T) {
	cluster := &v1beta1.KafkaCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-namespace",
		},
		Spec: v1beta1.KafkaClusterSpec{
			Brokers: []v1beta1.Broker{
				{Id: 0},
			},
		},
	}

	reconciler := &Reconciler{
		Reconciler: resources.Reconciler{
			KafkaCluster: cluster,
		},
	}

	eListener := v1beta1.ExternalListenerConfig{
		CommonListenerSpec: v1beta1.CommonListenerSpec{
			Name:          "test-listener",
			ContainerPort: 9092,
		},
		ExternalStartingPort: 19090,
	}

	ingressConfig := v1beta1.IngressConfig{
		EnvoyGatewayConfig: &v1beta1.EnvoyGatewayIngressConfig{},
	}

	route := reconciler.tcpRoute(0, eListener, ingressConfig)

	tcpRoute, ok := route.(*gatewayv1alpha2.TCPRoute)
	if !ok {
		t.Fatal("Expected TCPRoute type")
	}

	if tcpRoute.Name != "kafka-tcproute-test-listener-0" {
		t.Errorf("Expected route name 'kafka-tcproute-test-listener-0', got '%s'", tcpRoute.Name)
	}

	if len(tcpRoute.Spec.ParentRefs) != 1 {
		t.Errorf("Expected 1 parent ref, got %d", len(tcpRoute.Spec.ParentRefs))
	}

	if string(tcpRoute.Spec.ParentRefs[0].Name) != "kafka-gateway-test-listener" {
		t.Errorf("Expected parent gateway 'kafka-gateway-test-listener', got '%s'", tcpRoute.Spec.ParentRefs[0].Name)
	}

	if len(tcpRoute.Spec.Rules) != 1 {
		t.Errorf("Expected 1 rule, got %d", len(tcpRoute.Spec.Rules))
	}

	if len(tcpRoute.Spec.Rules[0].BackendRefs) != 1 {
		t.Errorf("Expected 1 backend ref, got %d", len(tcpRoute.Spec.Rules[0].BackendRefs))
	}

	if string(tcpRoute.Spec.Rules[0].BackendRefs[0].Name) != "test-cluster-all-broker" {
		t.Errorf("Expected backend 'test-cluster-all-broker', got '%s'", tcpRoute.Spec.Rules[0].BackendRefs[0].Name)
	}
}

func TestGatewayGenerationWithTLS(t *testing.T) {
	cluster := &v1beta1.KafkaCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-namespace",
		},
		Spec: v1beta1.KafkaClusterSpec{
			Brokers: []v1beta1.Broker{
				{Id: 0},
				{Id: 1},
				{Id: 2},
			},
			EnvoyGatewayConfig: v1beta1.EnvoyGatewayIngressConfig{
				GatewayClassName: "test-gateway-class",
				TLSSecretName:    "test-tls-secret",
			},
		},
	}

	reconciler := &Reconciler{
		Reconciler: resources.Reconciler{
			KafkaCluster: cluster,
		},
	}

	eListener := v1beta1.ExternalListenerConfig{
		CommonListenerSpec: v1beta1.CommonListenerSpec{
			Name:          "test-listener",
			ContainerPort: 9092,
		},
		ExternalStartingPort: -1, // TLS enabled
	}

	ingressConfig := v1beta1.IngressConfig{
		EnvoyGatewayConfig: &cluster.Spec.EnvoyGatewayConfig,
	}

	gateway := reconciler.gateway(eListener, ingressConfig)

	gw, ok := gateway.(*gatewayv1.Gateway)
	if !ok {
		t.Fatal("Expected Gateway type")
	}

	// 3 brokers + 1 anycast = 4 listeners
	if len(gw.Spec.Listeners) != 4 {
		t.Errorf("Expected 4 listeners, got %d", len(gw.Spec.Listeners))
	}

	// When TLS is enabled (externalStartingPort == -1), all broker listeners should use the anycast port
	expectedPort := gatewayv1.PortNumber(29092) // default anycast port
	for i := 0; i < 3; i++ {
		if gw.Spec.Listeners[i].Port != expectedPort {
			t.Errorf("Expected broker %d port %d (anycast port when TLS enabled), got %d", i, expectedPort, gw.Spec.Listeners[i].Port)
		}
		if gw.Spec.Listeners[i].Protocol != gatewayv1.TLSProtocolType {
			t.Errorf("Expected broker %d protocol TLS, got %s", i, gw.Spec.Listeners[i].Protocol)
		}
		if gw.Spec.Listeners[i].TLS == nil {
			t.Errorf("Expected broker %d to have TLS config", i)
		}
	}

	// Check anycast listener also uses the same port
	if gw.Spec.Listeners[3].Port != expectedPort {
		t.Errorf("Expected anycast port %d, got %d", expectedPort, gw.Spec.Listeners[3].Port)
	}
}
