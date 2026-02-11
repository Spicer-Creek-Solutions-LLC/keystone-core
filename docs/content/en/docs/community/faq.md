---
title: "FAQ"
weight: 50
description: >
  Frequently asked questions about Keystone Core.
---

## General Questions

### What is Keystone Core?

Keystone Core is a cloud-native runtime infrastructure control plane. It fills the gap between GitOps/IaC tools that deploy infrastructure and the ongoing operational needs of keeping that infrastructure running correctly. Think of it as: "GitOps deploys it. Keystone Core keeps it running."

### How is Keystone Core different from Salt/Ansible/Puppet?

While Keystone Core draws inspiration from Salt Project, it's designed from the ground up for cloud-native environments:

| Feature | Keystone Core | Salt/Ansible/Puppet |
|---------|---------------|---------------------|
| **Message Bus** | NATS (embedded or external) | ZeroMQ/SSH/Custom |
| **Kubernetes Native** | CRDs, operators, pod execution | Bolted on |
| **GitOps Integration** | ArgoCD/Flux webhooks, verification | Limited |
| **Policy Enforcement** | OPA/CEL built-in | Add-on |
| **Plugin Architecture** | Starlark/WASM sandboxed | Python/Ruby |
| **State Storage** | SQLite/PostgreSQL | Files/Redis |
| **Cross-Compilation** | Pure Go, single binary | Python/Ruby dependencies |

### What environments does Keystone Core support?

- **Kubernetes**: Native CRDs, DaemonSets, pod execution
- **Virtual Machines**: Any Linux, Windows, macOS
- **Bare Metal**: Full hardware detection, BMC/IPMI support
- **Edge/IoT**: Offline mode, local caching, resource constraints
- **Cloud**: AWS, GCP, Azure with automatic detection

### Is Keystone Core production-ready?

Keystone Core has completed 25 Epics and has extensive test coverage. However, review the [Known Implementation Gaps](/docs/community/roadmap/) before production deployment. The project is actively developed with focus on security and stability.

## Installation & Setup

### What are the minimum requirements?

**Control Plane:**

- 2 CPU cores, 2GB RAM (minimal)
- 4 CPU cores, 4GB RAM (production)
- Linux, Windows, or macOS

**Agent:**

- 1 CPU core, 256MB RAM
- Linux, Windows, or macOS
- Network connectivity to control plane (or offline mode)

### Can I run without external dependencies?

Yes! Keystone Core supports "embedded mode" with:

- **Embedded NATS**: In-process message bus
- **SQLite**: File-based state storage

This is perfect for development, testing, home labs, or small deployments (<100 nodes).

### How do I migrate from embedded to external mode?

1. Deploy external NATS cluster
2. Deploy PostgreSQL (if desired)
3. Use `kscorectl migrate` tool for database migration:

   ```bash
   kscorectl migrate run \
     --source sqlite:///var/lib/keystone/keystone.db \
     --target postgres://keystone:pass@localhost/keystone
   ```

4. Update control plane configuration to point to external services
5. Restart control plane

### How do agents find the control plane?

Agents support multiple discovery methods:

- **Static**: Configure control plane URL directly
- **DNS**: SRV record lookup for `_kscore._tcp.domain`
- **Kubernetes**: Service discovery via endpoints
- **Consul**: Service registry lookup
- **mDNS**: Local network discovery (development)

## State Management

### What's the difference between states, modules, and blueprints?

- **States**: Individual resource declarations (file, package, service)
- **Modules**: Code-based extensions written in Starlark or WASM
- **Blueprints**: Pre-packaged collections of states with parameters

Example hierarchy:

```
Blueprint (lamp-stack)
├── Uses States (file, package, service)
└── May use Modules (custom behavior)
```

### How does drift detection work?

1. States declare desired configuration
2. Keystone Core periodically checks actual state
3. Differences are reported as "drift" with severity levels:
   - **Critical**: Security-sensitive changes (permissions, ownership)
   - **High**: Service-affecting changes (config files, packages)
   - **Medium**: Operational changes
   - **Low**: Cosmetic changes

4. Optional: Auto-remediation based on policy

### Can I do dry-run before applying changes?

Yes, use the `check` command:

```bash
kscorectl state check myconfig.yaml
```

This shows what would change without making any modifications.

### How do I handle secrets in states?

Keystone Core supports multiple secret backends:

```yaml
# In state files
db_password: !secret databases/prod/password

# Agent configuration for Vault
secrets:
  backend: vault
  vault:
    address: https://vault.example.com
    auth:
      method: kubernetes
```

Supported backends:

- HashiCorp Vault
- AWS Secrets Manager
- GCP Secret Manager
- Azure Key Vault
- Kubernetes Secrets
- Environment variables

## Remote Execution

### How do I target specific agents?

Use targeting expressions:

```bash
# By hostname glob
kscorectl exec run "web-*" -- uptime

# By label
kscorectl exec run "role=database" -- df -h

# By expression
kscorectl exec run "os=linux AND cloud.provider=aws" -- cat /etc/os-release

# Compound targeting
kscorectl exec run "environment=prod AND NOT role=database" -- uptime
```

### What shells are supported?

- **Linux/macOS**: bash, sh, zsh
- **Windows**: PowerShell, cmd.exe

The shell is auto-detected based on the OS. If you need a specific shell, invoke it explicitly:

```bash
kscorectl exec run "windows-*" -- powershell -Command "Get-Process"
```

### How do I run commands as a different user?

Use the `--user` flag:

```bash
kscorectl exec run --user postgres "db-*" -- psql -c "SELECT 1"
```

Note: Requires the agent to run as root (Linux) or with appropriate privileges (Windows).

## Security

### How is communication secured?

All communication uses:

- **mTLS**: Mutual TLS for agent-to-control-plane
- **NATS Security**: TLS + authentication for message bus
- **SPIFFE/SPIRE**: Workload identity (optional)

### What authentication methods are supported?

- **API Keys**: For CLI and automation
- **JWT Tokens**: For web/API access
- **mTLS Certificates**: For agent authentication
- **SPIFFE SVIDs**: Workload-based identity

### How does policy enforcement work?

Keystone Core supports OPA (Rego) and CEL policies:

```yaml
# Example OPA policy
package keystone.execution

default allow = false

allow {
  input.action == "execute"
  input.user.groups[_] == "operators"
  not contains(input.command, "rm -rf")
}
```

Policies can:

- Block dangerous operations
- Require approvals
- Enforce resource quotas
- Audit all actions

## Clustering & High Availability

### How many control plane nodes should I run?

| Deployment | Nodes | NATS | State Storage |
|------------|-------|------|---------------|
| Development | 1 | Embedded | SQLite |
| Small (<100 agents) | 1-2 | Embedded | SQLite/PostgreSQL |
| Production | 3+ | External cluster | PostgreSQL |

### What happens if a control plane node fails?

With 3+ nodes:

1. etcd leader election promotes a new leader
2. Agents automatically reconnect to healthy nodes
3. Work is redistributed (consistent hashing)
4. No manual intervention required

### How are agents distributed across control plane nodes?

Agents are assigned to control plane nodes using consistent hashing. When a node fails:

1. Affected agents are automatically reassigned
2. Agent state is recovered from etcd/database
3. Commands in-flight are retried on the new node

## Troubleshooting

### Agent won't connect

1. **Check network connectivity**:

   ```bash
   curl -k https://control-plane:8443/health
   ```

2. **Verify TLS certificates**:

   ```bash
   openssl s_client -connect control-plane:8443
   ```

3. **Check agent logs**:

   ```bash
   journalctl -u kscore-agent -f
   ```

4. **Verify NATS connectivity**:

   ```bash
   nats-server -c /etc/keystone-core/nats.conf --test
   ```

### State application fails

1. **Run in check mode first**:

   ```bash
   kscorectl state check myconfig.yaml
   ```

2. **Check for requisite failures**:

   ```bash
   kscorectl state apply myconfig.yaml
   ```

3. **Verify module availability**:

   ```bash
   kscorectl module list
   ```

### Commands time out

1. **Check agent health**:

   ```bash
   kscorectl agent status agent-id
   ```

2. **Increase timeout**:

   ```bash
   kscorectl exec run --command-timeout 300 "target" -- long-running-command
   ```

3. **Track a job by ID**:

   ```bash
   kscorectl exec run --job-id job-123 "target" -- long-running-command
   kscorectl exec status job-123
   ```

## Performance

### How many agents can a single control plane handle?

- **Embedded mode**: ~100 agents
- **External NATS + PostgreSQL**: 1,000+ agents per control plane node
- **Clustered**: 10,000+ agents with 3+ control plane nodes

### What's the latency for command execution?

Typical latency:

- Same datacenter: 10-50ms
- Cross-region: 50-200ms
- Edge (with buffering): Variable (buffered until connectivity)

### How do I optimize for large deployments?

1. **Use external NATS cluster** with JetStream
2. **Use PostgreSQL** instead of SQLite
3. **Deploy multiple control plane nodes** behind load balancer
4. **Use regional NATS leaf nodes** for geographic distribution
5. **Enable connection pooling** in database configuration

## Integration

### How do I integrate with GitOps tools?

Keystone Core provides webhook endpoints for:

- **ArgoCD**: Deployment verification, health checks
- **Flux**: Kustomization sync events
- **GitHub/GitLab**: Deployment events, PR comments

Example ArgoCD integration:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  annotations:
    notifications.argoproj.io/subscribe.on-sync-succeeded.keystone: ""
```

### Can I use Keystone Core with Terraform?

Yes, use Keystone Core for runtime operations while Terraform handles infrastructure provisioning. They're complementary:

- **Terraform**: Provision VMs, networks, cloud resources
- **Keystone Core**: Configure and maintain those resources

### How do I integrate with monitoring?

Keystone Core exposes:

- **Prometheus metrics**: `/metrics` endpoint
- **OpenTelemetry traces**: OTLP export
- **Structured logs**: JSON, logfmt, or text

Pre-built Grafana dashboards are available in `deploy/grafana/dashboards/`.

## Contributing

### How do I report bugs?

Open an issue at [github.com/shawnbutts/keystone-core/issues](https://github.com/shawnbutts/keystone-core/issues) with:

- Keystone Core version (`kscorectl version`)
- Steps to reproduce
- Expected vs actual behavior
- Relevant logs

### How do I contribute code?

1. Fork the repository
2. Create a feature branch
3. Follow the [development guide](/docs/community/development/)
4. Submit a pull request

### Where can I get help?

- **GitHub Discussions**: General questions
- **GitHub Issues**: Bug reports, feature requests
- **Documentation**: [docs.keystone-core.io](https://docs.keystone-core.io)
