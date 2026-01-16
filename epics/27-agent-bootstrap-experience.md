# Epic 27: Agent Bootstrap Experience

## Overview

Transform the Keystone Core getting started experience from a multi-step manual process into a single-binary, TUI-guided bootstrap that can set up anything from a demo instance to a full production cluster.

**Goal**: Download one binary (`kscore-agent`), run `kscore-agent bootstrap`, answer a few questions, and have a working Keystone Core deployment.

**Status**: Complete

## Success Criteria

1. Single binary bootstrap - no pre-requisites beyond the agent binary
2. Interactive TUI guides users through deployment options
3. All TUI options also available via CLI flags and environment variables
4. Support three deployment modes: demo/lab, production-ready, full-scale
5. Automatic package repository configuration (apt, dnf, yum, etc.)
6. Blueprint-based configuration for reproducibility
7. Works across major Linux distributions (Ubuntu, Debian, RHEL, Rocky, Fedora, Alpine)
8. Idempotent - can be re-run safely to update or repair

## User Stories

### US27.1: First-Time User Bootstrap
**As a** new user evaluating Keystone Core
**I want to** run a single command to get a working demo environment
**So that** I can evaluate the product without complex setup

**Acceptance Criteria**:
- Download agent binary, run `kscore-agent bootstrap`, select "demo", have working system
- Demo mode uses embedded NATS + SQLite (zero dependencies)
- Completes in under 5 minutes on typical hardware
- Provides clear next-steps guidance after completion

### US27.2: Production Deployment
**As a** platform engineer
**I want to** bootstrap a production-ready cluster from multiple nodes
**So that** I have a resilient, HA deployment

**Acceptance Criteria**:
- Bootstrap guides through cluster configuration (node IPs, roles)
- Automatically handles certificate generation and distribution
- Configures external NATS cluster and PostgreSQL
- Sets up proper HA with etcd coordination
- Validates configuration before applying

### US27.3: Automated Bootstrap
**As a** DevOps engineer
**I want to** bootstrap via CLI flags or environment variables
**So that** I can automate deployments in CI/CD or provisioning scripts

**Acceptance Criteria**:
- All TUI questions have corresponding CLI flags
- All CLI flags have corresponding environment variables
- `--non-interactive` mode for fully automated bootstrap
- YAML config file support for complex configurations
- Exit codes indicate success/failure appropriately

### US27.4: Existing System Integration
**As a** system administrator
**I want to** add this node to an existing Keystone Core cluster
**So that** I can scale out my deployment

**Acceptance Criteria**:
- "Join existing cluster" option in bootstrap
- Requires only cluster endpoint and join token
- Automatically discovers cluster configuration
- Handles certificate provisioning via cluster CA

## Technical Tasks

### Phase 0: Discovery & Tasking (Week 0)

#### T0.1: Inventory Existing Bootstrap Capabilities
- Review `cmd/kscore-bootstrap` and shared packages for reusable logic
- Identify overlaps with `kscore-agent bootstrap` scope
- Document gaps vs. Epic 27 requirements

#### T0.2: Bootstrap Architecture Sketch
- Define bootstrap phases and failure rollback strategy
- Decide shared packages vs. new `cmd/kscore-agent/bootstrap/`
- Outline config flow (TUI → config → execution)

#### T0.3: Configuration Contract
- Draft schema for bootstrap config file (YAML)
- Map CLI flags and environment variables to schema fields
- Define defaults per deployment mode

#### T0.4: Platform Support Matrix
- Enumerate supported distros and init systems
- Identify required package managers per distro
- Capture constraints for demo vs production modes

**Acceptance Criteria**:
- Reuse plan for existing bootstrap code is documented
- Config schema and flag/env mapping is defined
- Platform coverage and constraints are explicit

**Phase 0 Notes (Initial Findings)**:
- Existing `cmd/kscore-bootstrap` CLI already supports seed/restore/import flows with audit logging.
- Shared bootstrap primitives exist in `pkg/bootstrap` (phases, config schema, installers, handoff).
- Epic 27 can reuse `pkg/bootstrap` and decide whether to wrap or subsume `kscore-bootstrap`.

### Phase 1: Core Bootstrap Command (Weeks 1-3)

#### T1.1: Bootstrap Command Structure
- Create `cmd/kscore-agent/bootstrap/` package
- Add `bootstrap` subcommand to kscore-agent
- Define bootstrap phases: detect, configure, validate, install, verify
- Implement phase orchestration with rollback support
- Progress tracking and logging

#### T1.2: Deployment Mode Framework
- Define DeploymentMode enum: Demo, Production, FullScale, Custom
- DeploymentConfig structure for each mode
- Mode-specific default configurations
- Mode detection heuristics (existing installations, resources)

#### T1.3: System Detection
- Detect OS and distribution (reuse pkg/platform/)
- Detect available package managers
- Detect init system (systemd, etc.)
- Detect existing Keystone Core installations
- Detect available resources (CPU, memory, disk)
- Network interface and IP detection

#### T1.4: Package Repository Configuration
- Apt repository setup (Debian, Ubuntu)
- DNF/YUM repository setup (RHEL, Rocky, Fedora, CentOS)
- Zypper repository setup (SUSE, openSUSE)
- APK repository setup (Alpine)
- GPG key management for repositories
- Repository URL templates with version substitution

### Phase 2: Interactive TUI (Weeks 4-6)

#### T2.1: TUI Framework
- Bubble Tea-based interactive TUI
- Consistent styling with kscore-monitor
- Screen flow management (wizard-style)
- Input validation with helpful error messages
- Help text and documentation links

#### T2.2: Deployment Mode Selection Screen
- Mode cards with descriptions
- Resource requirements for each mode
- Recommended mode based on system detection
- "Custom" option for advanced users

#### T2.3: Configuration Screens
- **Demo mode**: Minimal questions (just confirm)
- **Production mode**:
  - Cluster name
  - Node role (control-plane, agent, both)
  - Cluster endpoints (for joining)
  - Storage backend (PostgreSQL connection or auto-provision)
  - NATS configuration (embedded cluster or external)
- **Full-scale mode**:
  - All production options plus:
  - Multi-region configuration
  - High availability settings
  - Observability backend configuration
  - Identity provider configuration

#### T2.4: Cluster Join Screen
- Join token input (or generate new)
- Cluster discovery from endpoint
- Node labeling and tagging
- Role selection for this node

#### T2.5: Confirmation and Progress Screen
- Summary of all selected options
- Estimated completion time
- Real-time progress with phase indicators
- Log viewer (expandable)
- Success/failure with next steps

### Phase 3: CLI and Environment Variables (Weeks 7-8)

#### T3.1: CLI Flag System
```
kscore-agent bootstrap [flags]

Deployment Mode:
  --mode string              Deployment mode: demo, production, fullscale, custom
  --non-interactive          Run without TUI prompts

Cluster Configuration:
  --cluster-name string      Cluster name (default: "keystone")
  --node-role string         Node role: control-plane, agent, both
  --node-name string         Node name (default: hostname)
  --node-labels strings      Node labels (key=value)

Join Existing Cluster:
  --join string              Cluster endpoint to join
  --join-token string        Join token for authentication

Network:
  --advertise-address string Address to advertise to other nodes
  --bind-address string      Address to bind services

Storage:
  --storage-backend string   Storage backend: sqlite, postgres
  --postgres-host string     PostgreSQL host
  --postgres-port int        PostgreSQL port (default: 5432)
  --postgres-database string PostgreSQL database name
  --postgres-user string     PostgreSQL username
  --postgres-password string PostgreSQL password

NATS:
  --nats-mode string         NATS mode: embedded, external, cluster
  --nats-urls strings        External NATS URLs

Security:
  --tls-cert-file string     TLS certificate file
  --tls-key-file string      TLS key file
  --ca-cert-file string      CA certificate file
  --generate-certs           Generate self-signed certificates

Blueprints:
  --blueprints-dir string    Directory containing blueprints
  --apply-blueprint strings  Blueprints to apply after bootstrap

Output:
  --config-file string       Write configuration to file
  --dry-run                  Show what would be done without doing it
  --verbose                  Verbose output
  --json                     Output progress as JSON
```

#### T3.2: Environment Variable Support
- `KSCORE_BOOTSTRAP_MODE`
- `KSCORE_CLUSTER_NAME`
- `KSCORE_NODE_ROLE`
- `KSCORE_JOIN_ENDPOINT`
- `KSCORE_JOIN_TOKEN`
- `KSCORE_POSTGRES_*` variables
- `KSCORE_NATS_*` variables
- Environment variable documentation

#### T3.3: Configuration File Support
- YAML configuration file for complex setups
- `--config` flag to load from file
- Config file generation from TUI selections
- Config file validation

### Phase 4: Installation Engine (Weeks 9-11)

#### T4.1: Package Installation
- Package manager abstraction (reuse and extend pkg/platform/)
- Package dependency resolution
- Version pinning support
- Rollback on failure

#### T4.2: Service Configuration
- Systemd unit file generation/installation
- Service enablement and startup
- Configuration file templating
- Environment file generation

#### T4.3: Certificate Management
- Self-signed CA generation (for demo/standalone)
- CSR generation and signing (for cluster join)
- Certificate distribution to services
- Certificate renewal setup

#### T4.4: Database Setup
- SQLite initialization (demo mode)
- PostgreSQL connection validation
- Schema migration execution
- Initial data seeding

#### T4.5: NATS Configuration
- Embedded NATS configuration
- NATS cluster formation (multi-node)
- Leaf node configuration
- JetStream enablement

### Phase 5: Blueprint Integration (Weeks 12-13)

#### T5.1: Blueprint Discovery
- Local blueprint directory scanning
- Blueprint validation before application
- Dependency ordering for blueprints

#### T5.2: Post-Bootstrap Blueprint Application
- Apply selected blueprints after core setup
- Blueprint parameter substitution from bootstrap config
- Blueprint application progress tracking
- Rollback if blueprint fails

#### T5.3: Standard Blueprint Hooks
- Pre-bootstrap hooks (system preparation)
- Post-bootstrap hooks (additional configuration)
- Verification hooks (health checks)

### Phase 6: Verification and Completion (Week 14)

#### T6.1: Health Verification
- Service health checks (all components running)
- Connectivity verification (NATS, PostgreSQL)
- API endpoint validation
- Cluster membership verification (if clustered)

#### T6.2: Completion Report
- Summary of installed components
- Service status
- Important file locations
- Next steps documentation
- Quick command reference

#### T6.3: Troubleshooting Integration
- Diagnostic data collection on failure
- Common problem detection and suggestions
- Log aggregation for support

## Command Examples

```bash
# Interactive TUI bootstrap (recommended for first-time users)
kscore-agent bootstrap

# Quick demo mode (non-interactive)
kscore-agent bootstrap --mode demo --non-interactive

# Production cluster - first node (control plane)
kscore-agent bootstrap \
  --mode production \
  --cluster-name prod \
  --node-role control-plane \
  --postgres-host db.example.com \
  --generate-certs \
  --non-interactive

# Production cluster - join additional control plane
kscore-agent bootstrap \
  --join https://cp1.example.com:8443 \
  --join-token eyJhbGciOiJIUzI1NiIs... \
  --node-role control-plane \
  --non-interactive

# Production cluster - join agent node
kscore-agent bootstrap \
  --join https://cp1.example.com:8443 \
  --join-token eyJhbGciOiJIUzI1NiIs... \
  --node-role agent \
  --node-labels environment=production,role=webserver \
  --non-interactive

# Full-scale with external NATS and specific blueprints
kscore-agent bootstrap \
  --mode fullscale \
  --cluster-name enterprise \
  --nats-mode external \
  --nats-urls nats://nats1:4222,nats://nats2:4222,nats://nats3:4222 \
  --apply-blueprint monitoring-stack \
  --apply-blueprint security-baseline \
  --non-interactive

# Dry run to see what would happen
kscore-agent bootstrap --mode production --dry-run

# Generate config file from TUI selections
kscore-agent bootstrap --config-file bootstrap-config.yaml
```

## Environment Variable Examples

```bash
# Demo mode via environment
export KSCORE_BOOTSTRAP_MODE=demo
kscore-agent bootstrap --non-interactive

# Production with PostgreSQL via environment
export KSCORE_BOOTSTRAP_MODE=production
export KSCORE_CLUSTER_NAME=prod
export KSCORE_NODE_ROLE=control-plane
export KSCORE_STORAGE_BACKEND=postgres
export KSCORE_POSTGRES_HOST=db.example.com
export KSCORE_POSTGRES_USER=keystone
export KSCORE_POSTGRES_PASSWORD=secret
kscore-agent bootstrap --non-interactive

# Join existing cluster via environment
export KSCORE_JOIN_ENDPOINT=https://cp1.example.com:8443
export KSCORE_JOIN_TOKEN=eyJhbGciOiJIUzI1NiIs...
export KSCORE_NODE_ROLE=agent
kscore-agent bootstrap --non-interactive
```

## Dependencies

- **Epic 23** (Self-Management): Bootstrap infrastructure, backup/restore
- **Epic 25** (Blueprints): Blueprint runtime for post-bootstrap configuration
- **Epic 17** (SPIFFE Identity): Certificate management and identity
- **Epic 11** (Clustering): HA cluster formation

## Risks and Mitigations

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Distribution compatibility issues | Medium | High | Extensive testing matrix, fallback options |
| Complex TUI state management | Medium | Medium | Well-defined state machine, comprehensive testing |
| Network configuration complexity | Medium | High | Sensible defaults, clear error messages |
| Partial failure during bootstrap | Medium | High | Transaction-like phases with rollback |
| Version skew in cluster joins | Low | Medium | Version compatibility checks |

## Testing Strategy

- **Unit Tests**: All bootstrap components, configuration parsing
- **Integration Tests**: Package installation, service management
- **E2E Tests**: Full bootstrap scenarios (covered in Epic 29)
- **TUI Tests**: Screen flow, input validation

## Definition of Done

- [x] `kscore-agent bootstrap` command implemented
- [x] Interactive TUI with all deployment modes
- [x] All CLI flags and environment variables working
- [x] Package repository configuration for major distros
- [x] Certificate generation and distribution
- [x] Service installation and configuration
- [x] Health verification after bootstrap
- [x] Documentation complete
- [x] Unit and integration tests passing
- [x] Code review approved
