---
title: "Installation"
linkTitle: "Installation"
weight: 2
description: >
  How to install TitanAnvil on your system
---

## Prerequisites

Before installing TitanAnvil, ensure you have:

### System Requirements
- **Operating System**: Linux, macOS, or Windows
- **Architecture**: amd64, arm64
- **Memory**: 512MB minimum (2GB recommended for control plane)
- **Disk**: 1GB for binaries and data
- **Go**: 1.21+ (if building from source)

### Network Requirements
- Control plane needs port 4222 for NATS (configurable)
- Control plane needs port 8080 for API server (configurable)
- Agents need outbound connectivity to control plane

## Installation Methods

### Method 1: Pre-built Binaries (Recommended)

Download the latest release from GitHub:

```bash
# Linux (amd64)
curl -LO https://github.com/titananvil/titan-anvil/releases/latest/download/titananvil-linux-amd64.tar.gz
tar xzf titananvil-linux-amd64.tar.gz
sudo mv titananvil-* /usr/local/bin/
sudo mv titanctl /usr/local/bin/

# macOS (amd64)
curl -LO https://github.com/titananvil/titan-anvil/releases/latest/download/titananvil-darwin-amd64.tar.gz
tar xzf titananvil-darwin-amd64.tar.gz
sudo mv titananvil-* /usr/local/bin/
sudo mv titanctl /usr/local/bin/

# macOS (arm64 / Apple Silicon)
curl -LO https://github.com/titananvil/titan-anvil/releases/latest/download/titananvil-darwin-arm64.tar.gz
tar xzf titananvil-darwin-arm64.tar.gz
sudo mv titananvil-* /usr/local/bin/
sudo mv titanctl /usr/local/bin/
```

Verify installation:

```bash
titanctl version
titananvil-server --version
titananvil-agent --version
```

### Method 2: Build from Source

Clone and build:

```bash
# Clone repository
git clone https://github.com/titananvil/titan-anvil.git
cd titan-anvil

# Build all binaries
make build

# Install to /usr/local/bin
sudo make install

# Or install to custom location
make install PREFIX=$HOME/.local
```

Build individual components:

```bash
make build-server   # Build titananvil-server
make build-agent    # Build titananvil-agent
make build-cli      # Build titanctl
make build-plugins  # Build all plugins
```

### Method 3: Package Managers

#### Homebrew (macOS/Linux)

```bash
brew tap titananvil/tap
brew install titananvil
```

#### APT (Debian/Ubuntu)

```bash
# Add repository
curl -fsSL https://apt.titananvil.dev/gpg | sudo gpg --dearmor -o /usr/share/keyrings/titananvil.gpg
echo "deb [signed-by=/usr/share/keyrings/titananvil.gpg] https://apt.titananvil.dev stable main" | \
  sudo tee /etc/apt/sources.list.d/titananvil.list

# Install
sudo apt update
sudo apt install titananvil
```

#### YUM/DNF (RHEL/CentOS/Fedora)

```bash
# Add repository
sudo tee /etc/yum.repos.d/titananvil.repo <<EOF
[titananvil]
name=TitanAnvil Repository
baseurl=https://yum.titananvil.dev/stable/\$basearch
enabled=1
gpgcheck=1
gpgkey=https://yum.titananvil.dev/gpg
EOF

# Install
sudo yum install titananvil
# or
sudo dnf install titananvil
```

### Method 4: Docker

Run the control plane in Docker:

```bash
# Pull image
docker pull titananvil/control-plane:latest

# Run control plane
docker run -d \
  --name titananvil-control-plane \
  -p 4222:4222 \
  -p 8080:8080 \
  -v titananvil-data:/data \
  titananvil/control-plane:latest
```

Run an agent:

```bash
docker run -d \
  --name titananvil-agent \
  -e CONTROL_PLANE_URL=nats://control-plane:4222 \
  titananvil/agent:latest
```

### Method 5: Kubernetes

Deploy using Helm:

```bash
# Add Helm repository
helm repo add titananvil https://charts.titananvil.dev
helm repo update

# Install control plane
helm install titananvil-control-plane titananvil/control-plane \
  --namespace titananvil-system \
  --create-namespace

# Install agent (DaemonSet)
helm install titananvil-agent titananvil/agent \
  --namespace titananvil-system
```

Or use kubectl with manifests:

```bash
kubectl apply -f https://github.com/titananvil/titan-anvil/releases/latest/download/titananvil-k8s.yaml
```

## Configuration

### Control Plane Configuration

Create `/etc/titananvil/server.yaml`:

```yaml
# Minimal configuration (uses embedded NATS and SQLite)
nats:
  mode: embedded
  listen: "0.0.0.0:4222"

api:
  listen: "0.0.0.0:8080"

storage:
  type: sqlite
  path: /var/lib/titananvil/state.db
```

Advanced configuration (external NATS, PostgreSQL):

```yaml
nats:
  mode: external
  urls:
    - nats://nats1.example.com:4222
    - nats://nats2.example.com:4222
    - nats://nats3.example.com:4222
  credentials: /etc/titananvil/nats.creds

api:
  listen: "0.0.0.0:8080"
  tls:
    enabled: true
    cert_file: /etc/titananvil/tls/server.crt
    key_file: /etc/titananvil/tls/server.key

storage:
  type: postgresql
  connection_string: "postgres://user:pass@postgres.example.com:5432/titananvil?sslmode=require"
```

### Agent Configuration

Create `/etc/titananvil/agent.yaml`:

```yaml
control_plane:
  url: "nats://control-plane.example.com:4222"

agent:
  id: ""  # Auto-generated if empty
  datacenter: "us-east-1"
  environment: "production"
  role: "web"
  tags:
    - "nginx"
    - "frontend"
```

## Running TitanAnvil

### Start Control Plane

```bash
# Foreground (for testing)
titananvil-server --config /etc/titananvil/server.yaml

# Background (systemd)
sudo systemctl start titananvil-server
sudo systemctl enable titananvil-server

# Docker
docker-compose up -d control-plane
```

### Start Agent

```bash
# Foreground
titananvil-agent --config /etc/titananvil/agent.yaml

# Background (systemd)
sudo systemctl start titananvil-agent
sudo systemctl enable titananvil-agent

# Kubernetes (automatic via DaemonSet)
kubectl rollout status daemonset/titananvil-agent -n titananvil-system
```

## Systemd Service Files

### Control Plane Service

Create `/etc/systemd/system/titananvil-server.service`:

```ini
[Unit]
Description=TitanAnvil Control Plane
After=network.target

[Service]
Type=simple
User=titananvil
Group=titananvil
ExecStart=/usr/local/bin/titananvil-server --config /etc/titananvil/server.yaml
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

### Agent Service

Create `/etc/systemd/system/titananvil-agent.service`:

```ini
[Unit]
Description=TitanAnvil Agent
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/titananvil-agent --config /etc/titananvil/agent.yaml
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable titananvil-server titananvil-agent
sudo systemctl start titananvil-server titananvil-agent
```

## Verification

Check that everything is running:

```bash
# Check control plane
curl http://localhost:8080/health/live
curl http://localhost:8080/health/ready

# List connected agents
titanctl agent list

# Check agent status
systemctl status titananvil-agent

# View logs
journalctl -u titananvil-server -f
journalctl -u titananvil-agent -f
```

## Troubleshooting

### Control Plane Won't Start

```bash
# Check logs
journalctl -u titananvil-server -n 50

# Common issues:
# - Port 4222 already in use (NATS conflict)
# - Port 8080 already in use (API conflict)
# - Database connection failed (PostgreSQL not reachable)
# - NATS connection failed (external NATS not reachable)

# Test configuration
titananvil-server --config /etc/titananvil/server.yaml --test-config
```

### Agent Won't Connect

```bash
# Check logs
journalctl -u titananvil-agent -n 50

# Common issues:
# - Control plane URL incorrect
# - Network connectivity blocked
# - NATS authentication failed

# Test connectivity
nc -zv control-plane.example.com 4222
```

### Command Not Found

```bash
# Ensure binaries are in PATH
echo $PATH
ls -la /usr/local/bin/titan*

# Add to PATH if needed
export PATH="/usr/local/bin:$PATH"
echo 'export PATH="/usr/local/bin:$PATH"' >> ~/.bashrc
```

## Next Steps

Now that TitanAnvil is installed:

1. **[Quick Start](../quick-start/)** - Deploy your first agent and run commands
2. **[Architecture](../architecture/)** - Understand how the components work together
3. **[Configuration Reference](../../reference/configuration/)** - Explore all configuration options

Or dive into specific features:
- [Remote Execution Tutorial](../../tutorials/remote-commands/)
- [State Management Tutorial](../../tutorials/state-application/)
- [Monitoring Setup](../../tutorials/monitoring/)
