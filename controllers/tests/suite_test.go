// Copyright © 2020 Cisco Systems, Inc. and/or its affiliates
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

/*

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

package tests

import (
	"context"
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apiv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	csrclient "k8s.io/client-go/kubernetes/typed/certificates/v1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	cmv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"

	istioclientv1beta1 "github.com/banzaicloud/istio-client-go/pkg/networking/v1beta1"
	banzaiistiov1alpha1 "github.com/banzaicloud/istio-operator/api/v2/v1alpha1"
	contour "github.com/projectcontour/contour/apis/projectcontour/v1"

	banzaicloudv1alpha1 "github.com/banzaicloud/koperator/api/v1alpha1"
	banzaicloudv1beta1 "github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/controllers"
	controllerMocks "github.com/banzaicloud/koperator/controllers/tests/mocks"
	"github.com/banzaicloud/koperator/pkg/jmxextractor"
	"github.com/banzaicloud/koperator/pkg/kafkaclient"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var k8sClient client.Client
var csrClient *csrclient.CertificatesV1Client
var testEnv *envtest.Environment
var mockKafkaClients map[types.NamespacedName]kafkaclient.KafkaClient
var cruiseControlOperationReconciler controllers.CruiseControlOperationReconciler
var kafkaClusterCCReconciler controllers.CruiseControlTaskReconciler

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func(ctx SpecContext) {

	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))

	By("bootstrapping test environment")
	timeout := 2 * time.Minute
	testEnv = &envtest.Environment{
		ErrorIfCRDPathMissing: true,
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "base", "crds"),
			filepath.Join("..", "..", "config", "test", "crd", "cert-manager"),
			filepath.Join("..", "..", "config", "test", "crd", "projectcontour"),
			filepath.Join("..", "..", "config", "test", "crd", "istio"),
		},
		ControlPlaneStartTimeout: timeout,
		ControlPlaneStopTimeout:  timeout,
		AttachControlPlaneOutput: false,
	}
	apiServer := testEnv.ControlPlane.GetAPIServer()
	apiServer.Configure().Set("service-cluster-ip-range", "10.0.0.0/16")

	var cfg *rest.Config
	var err error
	done := make(chan interface{})
	go func() {
		defer GinkgoRecover()
		cfg, err = testEnv.Start()
		close(done)
	}()
	Eventually(done).WithContext(ctx).WithTimeout(timeout).Should(BeClosed())
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	scheme := runtime.NewScheme()

	Expect(banzaiistiov1alpha1.AddToScheme(scheme)).To(Succeed())
	Expect(k8sscheme.AddToScheme(scheme)).To(Succeed())
	Expect(apiv1.AddToScheme(scheme)).To(Succeed())
	Expect(cmv1.AddToScheme(scheme)).To(Succeed())
	Expect(banzaicloudv1alpha1.AddToScheme(scheme)).To(Succeed())
	Expect(banzaicloudv1beta1.AddToScheme(scheme)).To(Succeed())
	Expect(istioclientv1beta1.AddToScheme(scheme)).To(Succeed())
	Expect(contour.AddToScheme(scheme)).To(Succeed())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	csrClient, err = csrclient.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())
	Expect(csrClient).NotTo(BeNil())

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: "0",
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Port: 8443,
		}),
		LeaderElection: false,
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(mgr).ToNot(BeNil())

	jmxextractor.NewMockJMXExtractor()

	mockKafkaClients = make(map[types.NamespacedName]kafkaclient.KafkaClient)

	// mock the creation of Kafka clients
	controllers.SetNewKafkaFromCluster(
		func(k8sclient client.Client, cluster *banzaicloudv1beta1.KafkaCluster) (kafkaclient.KafkaClient, func(), error) {
			client, closeFunc := getMockedKafkaClientForCluster(cluster)
			return client, closeFunc, nil
		})

	kafkaClusterReconciler := controllers.KafkaClusterReconciler{
		Client:              mgr.GetClient(),
		DirectClient:        mgr.GetAPIReader(),
		KafkaClientProvider: kafkaclient.NewMockProvider(),
	}

	err = controllers.SetupKafkaClusterWithManager(mgr).Complete(&kafkaClusterReconciler)
	Expect(err).NotTo(HaveOccurred())

	kafkaTopicReconciler := &controllers.KafkaTopicReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}

	err = controllers.SetupKafkaTopicWithManager(mgr, 10).Complete(kafkaTopicReconciler)
	Expect(err).NotTo(HaveOccurred())

	// Create a new  kafka user reconciler
	kafkaUserReconciler := controllers.KafkaUserReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}

	err = controllers.SetupKafkaUserWithManager(mgr, true, true).Complete(&kafkaUserReconciler)
	Expect(err).NotTo(HaveOccurred())

	kafkaClusterCCReconciler = controllers.CruiseControlTaskReconciler{
		Client:       mgr.GetClient(),
		DirectClient: mgr.GetAPIReader(),
		Scheme:       mgr.GetScheme(),
		ScaleFactory: controllerMocks.NewNoopScaleFactory(),
	}

	err = controllers.SetupCruiseControlWithManager(mgr).Complete(&kafkaClusterCCReconciler)
	Expect(err).NotTo(HaveOccurred())

	cruiseControlOperationReconciler = controllers.CruiseControlOperationReconciler{
		Client:       mgr.GetClient(),
		DirectClient: mgr.GetAPIReader(),
		Scheme:       mgr.GetScheme(),
		ScaleFactory: controllerMocks.NewNoopScaleFactory(),
	}

	err = controllers.SetupCruiseControlOperationWithManager(mgr).Complete(&cruiseControlOperationReconciler)
	Expect(err).NotTo(HaveOccurred())

	cruiseControlOperationTTLReconciler := controllers.CruiseControlOperationTTLReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}

	err = controllers.SetupCruiseControlOperationTTLWithManager(mgr).Complete(&cruiseControlOperationTTLReconciler)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:builder
	go func() {
		defer GinkgoRecover()
		ctrl.Log.Info("starting manager")
		err = mgr.Start(ctrl.SetupSignalHandler())
		Expect(err).ToNot(HaveOccurred())
	}()

	k8sClient, err = client.New(cfg, client.Options{Scheme: mgr.GetScheme()})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	crd := &apiv1.CustomResourceDefinition{}

	err = k8sClient.Get(ctx, types.NamespacedName{Name: "kafkaclusters.kafka.banzaicloud.io"}, crd)
	Expect(err).NotTo(HaveOccurred())
	Expect(crd.Spec.Names.Kind).To(Equal("KafkaCluster"))

	err = k8sClient.Get(ctx, types.NamespacedName{Name: "cruisecontroloperations.kafka.banzaicloud.io"}, crd)
	Expect(err).NotTo(HaveOccurred())
	Expect(crd.Spec.Names.Kind).To(Equal("CruiseControlOperation"))

	err = k8sClient.Get(ctx, types.NamespacedName{Name: "kafkatopics.kafka.banzaicloud.io"}, crd)
	Expect(err).NotTo(HaveOccurred())
	Expect(crd.Spec.Names.Kind).To(Equal("KafkaTopic"))

	err = k8sClient.Get(ctx, types.NamespacedName{Name: "kafkausers.kafka.banzaicloud.io"}, crd)
	Expect(err).NotTo(HaveOccurred())
	Expect(crd.Spec.Names.Kind).To(Equal("KafkaUser"))

})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

var (
	nodePortMutex sync.Mutex
	nodePorts     = make(map[int32]bool)
)

func GetNodePort(portAmount int32) int32 {
	portAmount--
	nodePortMutex.Lock()
	defer nodePortMutex.Unlock()

	const minPort, maxPort = 30000, 32767
	portRange := maxPort - minPort + 1

	fmt.Println("GetNodePort: Looking for an available nodeport")

	if k8sClient == nil {
		fmt.Println("WARNING: k8sClient not initialized yet skipping Kubernetes service check")
	} else {
		var serviceList corev1.ServiceList
		if err := k8sClient.List(context.Background(), &serviceList); err == nil {
			fmt.Printf("GetNodePort: Found %d services to check for nodeports\n", len(serviceList.Items))
			for _, service := range serviceList.Items {
				if service.Spec.Type == corev1.ServiceTypeNodePort {
					for _, port := range service.Spec.Ports {
						if port.NodePort > 0 {
							nodePorts[port.NodePort] = true
							fmt.Printf("GetNodePort: Found existing nodeport %d in service %s/%s\n",
								port.NodePort, service.Namespace, service.Name)
						}
					}
				}
			}
		} else {
			fmt.Printf("ERROR: Failed to list services: %v\n", err)
		}
	}

	attempts := 0
	for attempts = 0; attempts < 100; attempts++ {
		port := minPort + rand.Int32N(int32(portRange))

		// Avoid the problematic range around 32030 that often causes conflicts
		if port >= 32025 && port <= 32035 {
			continue
		}

		// Ensure the port range doesn't cross into the high conflict zone
		if port+portAmount >= 32025 && port <= 32035 {
			continue
		}

		allAvailable := true
		for i := int32(0); i <= portAmount; i++ {
			if nodePorts[port+i] {
				allAvailable = false
				break
			}
		}

		if allAvailable {
			for i := int32(0); i <= portAmount; i++ {
				nodePorts[port+i] = true
			}
			fmt.Printf("GetNodePort: Successfully allocated NodePort range %d-%d after %d attempts\n",
				port, port+portAmount, attempts+1)
			return port
		}
	}

	fmt.Printf("WARNING: No free NodePorts found after %d attempts, returning 0 for auto-assignment\n", attempts)
	return 0
}

func ReleaseNodePort(port int32) {
	nodePortMutex.Lock()
	defer nodePortMutex.Unlock()
	delete(nodePorts, port)
	fmt.Printf("Released NodePort %d\n", port)
}
