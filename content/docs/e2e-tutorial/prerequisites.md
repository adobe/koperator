---
title: Prerequisites and Setup
weight: 10
---

# Prerequisites and Setup

Before starting this tutorial, you need to install several tools and prepare your development environment. This section will guide you through setting up everything required for the Kafka on Kubernetes deployment.

## System Requirements

### Hardware Requirements

- **CPU**: Minimum 4 cores, recommended 8+ cores
- **Memory**: Minimum 8GB RAM, recommended 16GB+ RAM
- **Storage**: At least 20GB free disk space
- **Network**: Stable internet connection for downloading container images

### Operating System Support

This tutorial has been tested on:

- **macOS**: 10.15+ (Catalina and newer)
- **Linux**: Ubuntu 18.04+, CentOS 7+, RHEL 7+
- **Windows**: Windows 10+ with WSL2

## Required Tools Installation

### 1. Docker

Docker is required to run the kind Kubernetes cluster.

#### macOS (using Homebrew)

```bash
brew install --cask docker
```

#### Linux (Ubuntu/Debian)

```bash
# Update package index
sudo apt-get update

# Install required packages
sudo apt-get install -y \
    apt-transport-https \
    ca-certificates \
    curl \
    gnupg \
    lsb-release

# Add Docker's official GPG key
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg

# Set up the stable repository
echo \
  "deb [arch=amd64 signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu \
  $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

# Install Docker Engine
sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io

# Add your user to the docker group
sudo usermod -aG docker $USER
```

#### Windows

Download and install Docker Desktop from [https://www.docker.com/products/docker-desktop](https://www.docker.com/products/docker-desktop)

**Verify Docker installation:**

```bash
docker --version
docker run hello-world
```

### 2. kubectl

kubectl is the Kubernetes command-line tool.

#### macOS (using Homebrew)

```bash
brew install kubectl
```

#### Linux

```bash
# Download the latest release
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"

# Make it executable
chmod +x kubectl

# Move to PATH
sudo mv kubectl /usr/local/bin/
```

#### Windows (using Chocolatey)

```powershell
choco install kubernetes-cli
```

**Verify kubectl installation:**

```bash
kubectl version --client
```

### 3. kind (Kubernetes in Docker)

kind is a tool for running local Kubernetes clusters using Docker containers.

#### All Platforms

```bash
# For Linux
curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.30.0/kind-linux-amd64
chmod +x ./kind
sudo mv ./kind /usr/local/bin/kind

# For macOS
curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.30.0/kind-darwin-amd64
chmod +x ./kind
sudo mv ./kind /usr/local/bin/kind

# For Windows (in PowerShell)
curl.exe -Lo kind-windows-amd64.exe https://kind.sigs.k8s.io/dl/v0.30.0/kind-windows-amd64
Move-Item .\kind-windows-amd64.exe c:\some-dir-in-your-PATH\kind.exe
```

#### macOS (using Homebrew)

```bash
brew install kind
```

**Verify kind installation:**

```bash
kind version
```

### 4. Helm

Helm is the package manager for Kubernetes.

#### macOS (using Homebrew)

```bash
brew install helm
```

#### Linux

```bash
# Download and install
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
```

#### Windows (using Chocolatey)

```powershell
choco install kubernetes-helm
```

**Verify Helm installation:**

```bash
helm version
```

### 5. Git

Git is required to clone configuration files and examples.

#### macOS (using Homebrew)

```bash
brew install git
```

#### Linux (Ubuntu/Debian)

```bash
sudo apt-get install -y git
```

#### Windows

Download and install from [https://git-scm.com/download/win](https://git-scm.com/download/win)

**Verify Git installation:**

```bash
git --version
```

## Environment Setup

### 1. Create Working Directory

Create a dedicated directory for this tutorial:

```bash
mkdir -p ~/kafka-k8s-tutorial
cd ~/kafka-k8s-tutorial
```

### 2. Set Environment Variables

Set up some useful environment variables:

```bash
# Export variables for the session
export TUTORIAL_DIR=~/kafka-k8s-tutorial
export KAFKA_NAMESPACE=kafka
export ZOOKEEPER_NAMESPACE=zookeeper
export MONITORING_NAMESPACE=default

# Make them persistent (add to your shell profile)
echo "export TUTORIAL_DIR=~/kafka-k8s-tutorial" >> ~/.bashrc
echo "export KAFKA_NAMESPACE=kafka" >> ~/.bashrc
echo "export ZOOKEEPER_NAMESPACE=zookeeper" >> ~/.bashrc
echo "export MONITORING_NAMESPACE=default" >> ~/.bashrc

# Reload your shell or source the file
source ~/.bashrc
```

### 3. Verify Docker Resources

Ensure Docker has sufficient resources allocated:

```bash
# Check Docker system info
docker system info

# Check available resources
docker system df
```

**Recommended Docker Desktop settings:**
- **Memory**: 8GB minimum, 12GB+ recommended
- **CPUs**: 4 minimum, 6+ recommended
- **Disk**: 20GB minimum

### 4. Download Tutorial Resources

Clone the reference repository for configuration files:

```bash
cd $TUTORIAL_DIR
git clone https://github.com/amuraru/k8s-kafka-the-hard-way.git
cd k8s-kafka-the-hard-way
```

## Verification Checklist

Before proceeding to the next section, verify that all tools are properly installed:

```bash
# Check Docker
echo "Docker version:"
docker --version
echo ""

# Check kubectl
echo "kubectl version:"
kubectl version --client
echo ""

# Check kind
echo "kind version:"
kind version
echo ""

# Check Helm
echo "Helm version:"
helm version
echo ""

# Check Git
echo "Git version:"
git --version
echo ""

# Check working directory
echo "Working directory:"
ls -la $TUTORIAL_DIR
```

## Troubleshooting Common Issues

### Docker Permission Issues (Linux)

If you get permission denied errors with Docker:

```bash
# Add your user to the docker group
sudo usermod -aG docker $USER

# Log out and log back in, or run:
newgrp docker
```

### kubectl Not Found

If kubectl is not found in your PATH:

```bash
# Check if kubectl is in your PATH
which kubectl

# If not found, ensure /usr/local/bin is in your PATH
echo $PATH

# Add to PATH if needed
export PATH=$PATH:/usr/local/bin
```

### kind Cluster Creation Issues

If you encounter issues creating kind clusters:

```bash
# Check Docker is running
docker ps

# Check available disk space
df -h

# Check Docker resources in Docker Desktop settings
```

## Next Steps

Once you have all prerequisites installed and verified, proceed to the [Kubernetes Cluster Setup]({{< relref "cluster-setup.md" >}}) section to create your kind cluster.

---

> **Tip**: Keep this terminal session open throughout the tutorial, as the environment variables and working directory will be used in subsequent steps.
