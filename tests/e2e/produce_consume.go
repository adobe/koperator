// Copyright © 2023 Cisco Systems, Inc. and/or its affiliates
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

package e2e

import (
	"crypto/tls"
	"fmt"
	"time"

	"github.com/gruntwork-io/terratest/modules/k8s"
	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"
	"github.com/twmb/franz-go/pkg/kgo"
)

// requireInternalProducingConsumingMessage produces and consumes messages internally through
// short-lived kcl pods and makes comparisons between the produced and consumed messages.
// When internalAddress parameter is empty, it gets the internal address from the kafkaCluster CR status.
// When tlsSecretName is set, the secret is mounted into the kcl pods and mTLS is used.
func requireInternalProducingConsumingMessage(kubectlOptions k8s.KubectlOptions, internalAddress, topicName string, tlsSecretName string) {
	ginkgo.It(fmt.Sprintf("Producing and consuming messages to/from topicName: '%s", topicName), func() {
		if internalAddress == "" {
			ginkgo.By("Getting Kafka cluster internal addresses")
			internalListenerNames, err := getK8sResources(kubectlOptions,
				[]string{kafkaKind},
				"",
				kafkaClusterName,
				kubectlArgGoTemplateInternalListenersName,
			)

			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			gomega.Expect(internalListenerNames).ShouldNot(gomega.BeEmpty())

			internalListenerAddresses, err := getK8sResources(kubectlOptions,
				[]string{kafkaKind},
				"",
				kafkaClusterName,
				fmt.Sprintf(kubectlArgGoTemplateInternalListenerAddressesTemplate, internalListenerNames[0]),
			)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

			gomega.Expect(internalListenerAddresses).ShouldNot(gomega.BeEmpty())

			internalAddress = internalListenerAddresses[0]
		}

		currentTime := time.Now()
		err := producingMessagesInternally(kubectlOptions, internalAddress, topicName, currentTime.String(), tlsSecretName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		consumedMessages, err := consumingMessagesInternally(kubectlOptions, internalAddress, topicName, tlsSecretName)

		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(consumedMessages).Should(gomega.ContainSubstring(currentTime.String()))
	})
}

// requireExternalProducingConsumingMessage gets the Kafka cluster external addresses from the kafkaCluster CR status
// when externalAddresses is not specified. It also produces and consumes messages and makes a comparison between them.
func requireExternalProducingConsumingMessage(kubectlOptions k8s.KubectlOptions, topicName, tlsSecretName string, externalAddresses ...string) { //nolint:unused // Note: unused linter disabled until External e2e tests are turned on.
	ginkgo.It("Producing and consuming messages", func() {
		if len(externalAddresses) == 0 {
			var err error
			externalAddresses, err = getExternalListenerAddresses(kubectlOptions, "", kafkaClusterName)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		}

		var tlsConfig *tls.Config
		var clientOptions []kgo.Opt
		if tlsSecretName != "" {
			var err error
			tlsConfig, err = getTLSConfigFromSecret(kubectlOptions, tlsSecretName)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			clientOptions = append(clientOptions, kgo.DialTLSConfig(tlsConfig))
		}

		message := time.Now().String()

		err := producingMessagesExternally(externalAddresses, topicName, []string{message}, clientOptions...)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

		consumedMessages, err := consumingMessagesExternally(externalAddresses, topicName, clientOptions...)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

		ginkgo.By(fmt.Sprintf("Comparing produced: '%s' and consumed message: '%s'", message, consumedMessages))
		found := false
		for i := range consumedMessages {
			if consumedMessages[i] == message {
				found = true
				break
			}
		}
		gomega.Expect(found).Should(gomega.BeTrue())
	})
}

// getExternalListenerNames gets the names of the KafkaCluster CR's external listeners.
func getExternalListenerNames(kubectlOptions k8s.KubectlOptions, kafkaClusterName string) ([]string, error) { //nolint:unused // Note: unused linter disabled until External e2e tests are turned on.
	ginkgo.By("Getting external listener names from KafkaCluster status")
	externalListenerNames, err := getK8sResources(kubectlOptions,
		[]string{kafkaKind},
		"",
		kafkaClusterName,
		kubectlArgGoTemplateExternalListenersName,
	)
	if err != nil {
		return nil, fmt.Errorf("getting external listeners name: %w", err)
	}
	return externalListenerNames, nil
}

// getExternalListenerAddresses gets the Kafka cluster external addresses from the kafkaCluster CR.
// When externalListenerName is not specified it uses the first externalListener name in the CR to get addresses.
func getExternalListenerAddresses(kubectlOptions k8s.KubectlOptions, externalListenerName, kafkaClusterName string) ([]string, error) { //nolint:unused // Note: unused linter disabled until External e2e tests are turned on.
	ginkgo.By(fmt.Sprintf("Getting Kafka cluster '%s' external listener addresses", kafkaClusterName))
	if externalListenerName == "" {
		externalListenerNames, err := getExternalListenerNames(kubectlOptions, kafkaClusterName)
		if err != nil {
			return nil, err
		}
		gomega.Expect(getExternalListenerNames).ShouldNot(gomega.BeEmpty())
		externalListenerName = externalListenerNames[0]
	}
	ginkgo.By(fmt.Sprintf("Using external listener name: '%s'", externalListenerName))
	externalListenerAddresses, err := getK8sResources(kubectlOptions,
		[]string{kafkaKind},
		"",
		kafkaClusterName,
		fmt.Sprintf(kubectlArgGoTemplateExternalListenerAddressesTemplate, externalListenerName),
	)
	if err != nil {
		return nil, fmt.Errorf("getting external listener addresses: %w", err)
	}
	if len(externalListenerAddresses) == 0 {
		return nil, fmt.Errorf("external listener address %w", ErrorNotFound)
	}

	return externalListenerAddresses, nil
}

// requireAvailableExternalKafkaAddress checks that is there any available external address for the Kafka cluster.
func requireAvailableExternalKafkaAddress(kubectlOptions k8s.KubectlOptions, externalListenerName, kafkaClusterName string) { //nolint:unused // Note: unused linter disabled until External e2e tests are turned on.
	ginkgo.It(fmt.Sprintf("Checks that the KafkaCluster '%s' has external address", kafkaClusterName), func() {
		_, err := getExternalListenerAddresses(kubectlOptions, externalListenerName, kafkaClusterName)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	})
}
