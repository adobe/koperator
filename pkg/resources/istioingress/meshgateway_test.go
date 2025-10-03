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

package istioingress

import (
	"testing"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/resources"
)

func TestMeshgatewayContainerConfiguration(t *testing.T) {
	// Create a minimal KafkaCluster for testing
	kafkaCluster := &v1beta1.KafkaCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kafka",
			Namespace: "test-namespace",
		},
		Spec: v1beta1.KafkaClusterSpec{
			Brokers: []v1beta1.Broker{
				{Id: 0},
			},
		},
	}

	// Create reconciler
	reconciler := &Reconciler{
		Reconciler: resources.Reconciler{
			KafkaCluster: kafkaCluster,
		},
	}

	// Create test external listener config
	externalListenerConfig := v1beta1.ExternalListenerConfig{
		CommonListenerSpec: v1beta1.CommonListenerSpec{
			Type:          "plaintext",
			Name:          "external",
			ContainerPort: 9094,
		},
		ExternalStartingPort: 19090,
	}

	// Create test ingress config
	ingressConfig := v1beta1.IngressConfig{
		IstioIngressConfig: &v1beta1.IstioIngressConfig{},
	}

	// Call the meshgateway function
	result := reconciler.meshgateway(
		logr.Discard(),
		externalListenerConfig,
		ingressConfig,
		"test-config",
		"default-config",
		"",
	)

	// Cast to Deployment
	deployment, ok := result.(*appsv1.Deployment)
	if !ok {
		t.Fatalf("Expected *appsv1.Deployment, got %T", result)
	}

	// Verify deployment has one container
	if len(deployment.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("Expected 1 container, got %d", len(deployment.Spec.Template.Spec.Containers))
	}

	container := deployment.Spec.Template.Spec.Containers[0]

	// Verify container name
	if container.Name != "istio-proxy" {
		t.Errorf("Expected container name 'istio-proxy', got '%s'", container.Name)
	}

	// Verify container image
	if container.Image != v1beta1.DefaultIstioProxyImage {
		t.Errorf("Expected image '%s', got '%s'", v1beta1.DefaultIstioProxyImage, container.Image)
	}

	// Verify container has command
	if len(container.Command) == 0 {
		t.Error("Expected container to have command, but it's empty")
	} else if container.Command[0] != "/usr/local/bin/pilot-agent" {
		t.Errorf("Expected command '/usr/local/bin/pilot-agent', got '%s'", container.Command[0])
	}

	// Verify container has args
	if len(container.Args) == 0 {
		t.Error("Expected container to have args, but it's empty")
	} else if container.Args[0] != "proxy" {
		t.Errorf("Expected first arg 'proxy', got '%s'", container.Args[0])
	}

	// Verify container has required environment variables
	envVarMap := make(map[string]string)
	for _, env := range container.Env {
		envVarMap[env.Name] = env.Value
	}

	requiredEnvVars := []string{
		"PILOT_CERT_PROVIDER",
		"CA_ADDR",
		"PROXY_CONFIG",
		"ISTIO_META_CLUSTER_ID",
		"ISTIO_META_INTERCEPTION_MODE",
		"TRUST_DOMAIN",
	}

	for _, envVar := range requiredEnvVars {
		if _, exists := envVarMap[envVar]; !exists {
			t.Errorf("Expected environment variable '%s' to be set", envVar)
		}
	}

	// Verify container has required ports
	if len(container.Ports) != 2 {
		t.Errorf("Expected 2 container ports, got %d", len(container.Ports))
	}

	portMap := make(map[string]int32)
	for _, port := range container.Ports {
		portMap[port.Name] = port.ContainerPort
	}

	if port, exists := portMap["http-envoy-prom"]; !exists || port != 15090 {
		t.Errorf("Expected port 'http-envoy-prom' on 15090, got %d", port)
	}

	if port, exists := portMap["status-port"]; !exists || port != 15021 {
		t.Errorf("Expected port 'status-port' on 15021, got %d", port)
	}
}

func TestGetIstioProxyEnvVars(t *testing.T) {
	gatewayName := "test-gateway"
	namespace := "test-namespace"

	envVars := getIstioProxyEnvVars(gatewayName, namespace)

	// Verify we have environment variables
	if len(envVars) == 0 {
		t.Fatal("Expected environment variables, got none")
	}

	// Create a map for easier testing
	envVarMap := make(map[string]corev1.EnvVar)
	for _, env := range envVars {
		envVarMap[env.Name] = env
	}

	// Test specific environment variables
	testCases := []struct {
		name          string
		expectedValue string
	}{
		{"PILOT_CERT_PROVIDER", "istiod"},
		{"CA_ADDR", "istiod.istio-system.svc:15012"},
		{"PROXY_CONFIG", "{}"},
		{"ISTIO_META_CLUSTER_ID", "Kubernetes"},
		{"ISTIO_META_INTERCEPTION_MODE", "REDIRECT"},
		{"ISTIO_META_WORKLOAD_NAME", gatewayName},
		{"ISTIO_META_MESH_ID", "cluster.local"},
		{"TRUST_DOMAIN", "cluster.local"},
	}

	for _, tc := range testCases {
		if env, exists := envVarMap[tc.name]; !exists {
			t.Errorf("Expected environment variable '%s' to exist", tc.name)
		} else if env.Value != tc.expectedValue {
			t.Errorf("Expected '%s' to have value '%s', got '%s'", tc.name, tc.expectedValue, env.Value)
		}
	}

	// Test environment variables with field references
	fieldRefVars := []string{"POD_NAME", "POD_NAMESPACE", "INSTANCE_IP", "SERVICE_ACCOUNT", "HOST_IP"}
	for _, varName := range fieldRefVars {
		if env, exists := envVarMap[varName]; !exists {
			t.Errorf("Expected environment variable '%s' to exist", varName)
		} else if env.ValueFrom == nil || env.ValueFrom.FieldRef == nil {
			t.Errorf("Expected '%s' to have FieldRef, but it doesn't", varName)
		}
	}
}
