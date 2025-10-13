---
title: Kafka on Kubernetes - The Hard Way
shorttitle: E2E Tutorial
weight: 990
---

Inspired by Kelsey Hightower's [kubernetes-the-hard-way](https://github.com/kelseyhightower/kubernetes-the-hard-way), this comprehensive tutorial walks you through setting up a complete Kafka environment on Kubernetes using the Koperator from scratch.

## What You'll Learn

This tutorial will teach you how to:

- Set up a multi-node Kubernetes cluster using kind
- Install and configure all required dependencies manually
- Deploy a production-ready Kafka cluster with monitoring
- Test and validate your Kafka deployment
- Handle disaster recovery scenarios
- Troubleshoot common issues

## Why "The Hard Way"?

This tutorial is called "the hard way" because it walks through each step manually rather than using automated scripts or simplified configurations. This approach helps you understand:

- How each component works and interacts with others
- The dependencies and relationships between services
- How to troubleshoot when things go wrong
- The complete architecture of a Kafka deployment on Kubernetes

## Prerequisites

Before starting this tutorial, you should have:

- Basic knowledge of Kubernetes concepts (pods, services, deployments)
- Familiarity with Apache Kafka fundamentals
- A local development machine with Docker installed
- At least 8GB of RAM and 4 CPU cores available for the kind cluster

## Tutorial Structure

This tutorial is organized into the following sections:

1. **[Prerequisites and Setup]({{< relref "prerequisites.md" >}})** - Install required tools and prepare your environment
2. **[Kubernetes Cluster Setup]({{< relref "cluster-setup.md" >}})** - Create a multi-node kind cluster with proper labeling
3. **[Dependencies Installation]({{< relref "dependencies.md" >}})** - Install cert-manager, ZooKeeper operator, and Prometheus operator
4. **[Koperator Installation]({{< relref "koperator-install.md" >}})** - Install the Kafka operator and its CRDs
5. **[Kafka Cluster Deployment]({{< relref "kafka-deployment.md" >}})** - Deploy and configure a Kafka cluster with monitoring
6. **[Testing and Validation]({{< relref "testing.md" >}})** - Create topics, run producers/consumers, and performance tests
7. **[Disaster Recovery Scenarios]({{< relref "disaster-recovery.md" >}})** - Test failure scenarios and recovery procedures
8. **[Troubleshooting]({{< relref "troubleshooting.md" >}})** - Common issues and debugging techniques

## Architecture Overview

By the end of this tutorial, you'll have deployed the following architecture:

```
┌─────────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster (kind)                    │
├─────────────────────────────────────────────────────────────────┤
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │   Control Plane │  │    Worker AZ1   │  │    Worker AZ2   │  │
│  │                 │  │                 │  │                 │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘  │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │   Worker AZ3    │  │   Worker AZ1    │  │   Worker AZ2    │  │
│  │                 │  │                 │  │                 │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘  │
├─────────────────────────────────────────────────────────────────┤
│                        Applications                             │
├─────────────────────────────────────────────────────────────────┤
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │   Kafka Cluster │  │   ZooKeeper     │  │   Monitoring    │  │
│  │   (3 brokers)   │  │   (3 nodes)     │  │   Stack         │  │
│  │                 │  │                 │  │                 │  │
│  │  ┌─────────────┐│  │  ┌─────────────┐│  │  ┌─────────────┐│  │
│  │  │ Broker 101  ││  │  │    ZK-0     ││  │  │ Prometheus  ││  │
│  │  │ Broker 102  ││  │  │    ZK-1     ││  │  │ Grafana     ││  │
│  │  │ Broker 201  ││  │  │    ZK-2     ││  │  │ AlertMgr    ││  │
│  │  │ Broker 202  ││  │  └─────────────┘│  │  └─────────────┘│  │
│  │  │ Broker 301  ││  └─────────────────┘  └─────────────────┘  │
│  │  │ Broker 302  ││                                            │
│  │  └─────────────┘│                                            │
│  └─────────────────┘                                            │
├─────────────────────────────────────────────────────────────────┤
│                      Infrastructure                             │
├─────────────────────────────────────────────────────────────────┤
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │   cert-manager  │  │   Koperator     │  │   Cruise        │  │
│  │                 │  │                 │  │   Control       │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

## Key Features Demonstrated

This tutorial demonstrates:

- **Multi-AZ deployment** with rack awareness
- **SSL/TLS encryption** for secure communication
- **Monitoring and alerting** with Prometheus and Grafana
- **Automatic scaling** with Cruise Control
- **Persistent storage** with proper volume management
- **External access** configuration
- **Disaster recovery** and failure handling

## Time Commitment

Plan to spend approximately 2-3 hours completing this tutorial, depending on your familiarity with the tools and concepts involved.

## Getting Started

Ready to begin? Start with the [Prerequisites and Setup]({{< relref "prerequisites.md" >}}) section.

---

> **Note**: This tutorial is designed for learning and development purposes. For production deployments, consider using automated deployment tools and following your organization's security and operational guidelines.
