---
title: "Installation"
linkTitle: "Installation"
weight: 2
description: >
  How to install Keystone Core on your system
---

## Prerequisites

Before installing Keystone Core, ensure you have:

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
curl -LO https://github.com/shawnbutts/keystone-core/releases/latest/download/kscore-linux-amd64.tar.gz
tar xzf kscore-linux-amd64.tar.gz
sudo mv kscore-* /usr/local/bin/
sudo mv kscorectl /usr/local/bin/

# macOS (amd64)
curl -LO https://github.com/shawnbutts/keystone-core/releases/latest/download/kscore-darwin-amd64.tar.gz
tar xzf kscore-darwin-amd64.tar.gz
sudo mv kscore-* /usr/local/bin/
sudo mv kscorectl /usr/local/bin/

# macOS (arm64 / Apple Silicon)
curl -LO https://github.com/shawnbutts/keystone-core/releases/latest/download/kscore-darwin-arm64.tar.gz
tar xzf kscore-darwin-arm64.tar.gz
sudo mv kscore-* /usr/local/bin/
sudo mv kscorectl /usr/local/bin/
```

Verify installation:

```bash
kscorectl version
kscore-server --version
kscore-agent --version
```

### Method 2: Linux Packages (DEB/RPM)

Download and install DEB or RPM packages directly from GitHub releases:

**Debian/Ubuntu (DEB):**

```bash
# Download packages
curl -LO https://github.com/shawnbutts/keystone-core/releases/latest/download/kscore-server_0.10.0_linux_amd64.deb
curl -LO https://github.com/shawnbutts/keystone-core/releases/latest/download/kscore-agent_0.10.0_linux_amd64.deb
curl -LO https://github.com/shawnbutts/keystone-core/releases/latest/download/kscore-cli_0.10.0_linux_amd64.deb

# Install packages
sudo dpkg -i kscore-server_*.deb
sudo dpkg -i kscore-agent_*.deb
sudo dpkg -i kscore-cli_*.deb

# Configure and start services
sudo systemctl enable kscore-server
sudo systemctl start kscore-server
```

**RHEL/CentOS/Fedora (RPM):**

```bash
# Download packages
curl -LO https://github.com/shawnbutts/keystone-core/releases/latest/download/kscore-server-0.10.0.x86_64.rpm
curl -LO https://github.com/shawnbutts/keystone-core/releases/latest/download/kscore-agent-0.10.0.x86_64.rpm
curl -LO https://github.com/shawnbutts/keystone-core/releases/latest/download/kscore-cli-0.10.0.x86_64.rpm

# Install packages
sudo rpm -i kscore-server-*.rpm
sudo rpm -i kscore-agent-*.rpm
sudo rpm -i kscore-cli-*.rpm

# Configure and start services
sudo systemctl enable kscore-server
sudo systemctl start kscore-server
```

**Package Contents:**

| Package | Contents |
|---------|----------|
| `kscore-server` | Control plane server, systemd service, default config |
| `kscore-agent` | Agent daemon, systemd service, default config |
| `kscore-cli` | CLI tools (kscorectl, kscore-exec, kscore-state, kscore-monitor) |

### Method 3: Build from Source

Clone and build:

```bash
# Clone repository
git clone https://github.com/shawnbutts/keystone-core.git
cd keystone-core

# Build all binaries
make build

# Install to /usr/local/bin
sudo make install

# Or install to custom location
make install PREFIX=$HOME/.local
```

Build individual components:

```bash
make build-server   # Build kscore-server
make build-agent    # Build kscore-agent
make build-cli      # Build kscorectl
make build-plugins  # Build all plugins
```

### Method 4: Package Managers

#### Homebrew (macOS/Linux)

```bash
brew tap kscore/tap
brew install kscore
```

#### APT (Debian/Ubuntu)

```bash
# Add repository
curl -fsSL https://apt.kscore.dev/gpg | sudo gpg --dearmor -o /usr/share/keyrings/kscore.gpg
echo "deb [signed-by=/usr/share/keyrings/kscore.gpg] https://apt.kscore.dev stable main" | \
  sudo tee /etc/apt/sources.list.d/kscore.list

# Install
sudo apt update
sudo apt install kscore
```

#### YUM/DNF (RHEL/CentOS/Fedora)

```bash
# Add repository
sudo tee /etc/yum.repos.d/kscore.repo <<EOF
[kscore]
name=Keystone Core Repository
baseurl=https://yum.kscore.dev/stable/\$basearch
enabled=1
gpgcheck=1
gpgkey=https://yum.kscore.dev/gpg
EOF

# Install
sudo yum install kscore
# or
sudo dnf install kscore
```

### Method 5: Docker

Run the control plane in Docker:

```bash
# Pull image
docker pull kscore/control-plane:latest

# Run control plane
docker run -d \
  --name kscore-control-plane \
  -p 4222:4222 \
  -p 8080:8080 \
  -v kscore-data:/data \
  kscore/control-plane:latest
```

Run an agent:

```bash
docker run -d \
  --name kscore-agent \
  -e CONTROL_PLANE_URL=nats://control-plane:4222 \
  kscore/agent:latest
```

### Method 6: Kubernetes

Deploy using Helm:

```bash
# Add Helm repository
helm repo add keystonecore https://charts.kscore.dev
helm repo update

# Install control plane
helm install kscore-control-plane kscore/control-plane \
  --namespace kscore-system \
  --create-namespace

# Install agent (DaemonSet)
helm install kscore-agent kscore/agent \
  --namespace kscore-system
```

Or use kubectl with manifests:

```bash
kubectl apply -f https://github.com/shawnbutts/keystone-core/releases/latest/download/kscore-k8s.yaml
```

## Configuration

### Control Plane Configuration

Create `/etc/kscore/server.yaml`:

```yaml
# Minimal configuration (uses embedded NATS and SQLite)
nats:
  mode: embedded
  listen: "0.0.0.0:4222"

api:
  listen: "0.0.0.0:8080"

storage:
  type: sqlite
  path: /var/lib/kscore/state.db
```

Advanced configuration (external NATS, PostgreSQL):

```yaml
nats:
  mode: external
  urls:
    - nats://nats1.example.com:4222
    - nats://nats2.example.com:4222
    - nats://nats3.example.com:4222
  credentials: /etc/kscore/nats.creds

api:
  listen: "0.0.0.0:8080"
  tls:
    enabled: true
    cert_file: /etc/kscore/tls/server.crt
    key_file: /etc/kscore/tls/server.key

storage:
  type: postgresql
  connection_string: "postgres://user:pass@postgres.example.com:5432/kscore?sslmode=require"
```

### Agent Configuration

Create `/etc/kscore/agent.yaml`:

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

## Running Keystone Core

### Start Control Plane

```bash
# Foreground (for testing)
kscore-server --config /etc/kscore/server.yaml

# Background (systemd)
sudo systemctl start kscore-server
sudo systemctl enable kscore-server

# Docker
docker-compose up -d control-plane
```

### Start Agent

```bash
# Foreground
kscore-agent --config /etc/kscore/agent.yaml

# Background (systemd)
sudo systemctl start kscore-agent
sudo systemctl enable kscore-agent

# Kubernetes (automatic via DaemonSet)
kubectl rollout status daemonset/kscore-agent -n kscore-system
```

## Systemd Service Files

### Control Plane Service

Create `/etc/systemd/system/kscore-server.service`:

```ini
[Unit]
Description=Keystone Core Control Plane
After=network.target

[Service]
Type=simple
User=kscore
Group=kscore
ExecStart=/usr/local/bin/kscore-server --config /etc/kscore/server.yaml
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

### Agent Service

Create `/etc/systemd/system/kscore-agent.service`:

```ini
[Unit]
Description=Keystone Core Agent
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/kscore-agent --config /etc/kscore/agent.yaml
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable kscore-server kscore-agent
sudo systemctl start kscore-server kscore-agent
```

## Verification

Check that everything is running:

```bash
# Check control plane
curl http://localhost:8080/health/live
curl http://localhost:8080/health/ready

# List connected agents
kscorectl agent list

# Check agent status
systemctl status kscore-agent

# View logs
journalctl -u kscore-server -f
journalctl -u kscore-agent -f
```

## Troubleshooting

### Control Plane Won't Start

```bash
# Check logs
journalctl -u kscore-server -n 50

# Common issues:
# - Port 4222 already in use (NATS conflict)
# - Port 8080 already in use (API conflict)
# - Database connection failed (PostgreSQL not reachable)
# - NATS connection failed (external NATS not reachable)

# Test configuration
kscore-server --config /etc/kscore/server.yaml --test-config
```

### Agent Won't Connect

```bash
# Check logs
journalctl -u kscore-agent -n 50

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

Now that Keystone Core is installed:

1. **[Quick Start](../quick-start/)** - Deploy your first agent and run commands
2. **[Architecture](../architecture/)** - Understand how the components work together
3. **[Configuration Reference](../../reference/configuration/)** - Explore all configuration options

Or dive into specific features:
- [Remote Execution Tutorial](../../tutorials/remote-commands/)
- [State Management Tutorial](../../tutorials/state-application/)
- [Monitoring Setup](../../tutorials/monitoring/)
