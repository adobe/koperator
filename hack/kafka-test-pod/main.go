// Copyright © 2019 Cisco Systems, Inc. and/or its affiliates
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

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

const kafkaTopic = "test-topic"
const kafkaPartition = 0

const certFile = "/etc/secrets/certs/tls.crt"
const keyFile = "/etc/secrets/certs/tls.key"
const caFile = "/etc/secrets/certs/ca.crt"

const certEnvVar = "KAFKA_TLS_CERT"
const keyEnvVar = "KAFKA_TLS_KEY"
const caEnvVar = "KAFKA_TLS_CA"

var brokerAddrs = []string{"kafka-headless.kafka.svc.cluster.local:29092"}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Getenv("KAFKA_MODE"), kgo.SeedBrokers(brokerAddrs...), kgo.DialTLSConfig(getTLSConfig()))
	cancel()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Shutting down")
}

func run(ctx context.Context, mode string, opts ...kgo.Opt) error {
	switch mode {
	case "consumer":
	case "producer":
		opts = append(opts, kgo.RecordPartitioner(kgo.ManualPartitioner()), kgo.DisableIdempotentWrite(),
			kgo.RequiredAcks(kgo.NoAck()), kgo.RecordDeliveryTimeout(30*time.Second))
	default:
		return fmt.Errorf("invalid test mode: %q", mode)
	}

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return err
	}
	defer client.Close()
	if mode == "consumer" {
		return consume(ctx, client)
	}
	return produce(ctx, client)
}

func getEnvTLSConfig() (config *tls.Config, err error) {
	clientCert, err := tls.X509KeyPair(
		[]byte(os.Getenv(certEnvVar)),
		[]byte(os.Getenv(keyEnvVar)),
	)
	if err != nil {
		return
	}
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM([]byte(os.Getenv(caEnvVar)))
	return &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caPool,
	}, nil
}

func getTLSConfig() *tls.Config {
	// First see if we have an environment configuration
	if config, err := getEnvTLSConfig(); err == nil {
		return config
	}
	// Create a TLS configuration from our secret for connecting to Kafka
	clientCert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Fatal(err)
	}
	caCert, err := os.ReadFile(caFile)
	if err != nil {
		log.Fatal(err)
	}
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caCert)
	return &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caPool,
	}
}

func consume(ctx context.Context, client *kgo.Client) error {
	startupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	offsets, err := kadm.NewClient(client).ListEndOffsets(startupCtx, kafkaTopic)
	cancel()
	if ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return fmt.Errorf("could not resolve starting offset for %s/%d: %w", kafkaTopic, kafkaPartition, err)
	}
	offset, ok := offsets.Lookup(kafkaTopic, kafkaPartition)
	if !ok {
		return fmt.Errorf("no starting offset returned for %s/%d", kafkaTopic, kafkaPartition)
	}
	if offset.Err != nil {
		return fmt.Errorf("could not resolve starting offset for %s/%d: %w", kafkaTopic, kafkaPartition, offset.Err)
	}
	client.AddConsumePartitions(map[string]map[int32]kgo.Offset{
		kafkaTopic: {kafkaPartition: kgo.NewOffset().At(offset.Offset)},
	})

	for ctx.Err() == nil {
		fetches := client.PollFetches(ctx)
		for _, fetchErr := range fetches.Errors() {
			if ctx.Err() != nil {
				break
			}
			var dataLoss *kgo.ErrDataLoss
			if !errors.As(fetchErr.Err, &dataLoss) {
				return fetchErr.Err
			}
			log.Printf("Consumer resumed from a reset offset: %v", dataLoss)
		}
		fetches.EachRecord(func(msg *kgo.Record) {
			log.Printf("Consumed message offset %d - %s\n", msg.Offset, msg.Value)
		})
	}
	return nil
}

func produce(ctx context.Context, client *kgo.Client) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			log.Println("Sending message to topic")
			msg := &kgo.Record{Topic: kafkaTopic, Partition: kafkaPartition, Value: []byte("hello world")}
			if err := client.ProduceSync(ctx, msg).FirstErr(); err != nil && ctx.Err() == nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}
