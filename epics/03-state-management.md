# Epic 3: State Management & Configuration

## Overview

Implement declarative state management system that enables idempotent configuration of infrastructure, inspired by Salt Project states but with modern improvements for cloud-native environments.

**Goal**: Provide a powerful, declarative configuration management system that can maintain desired state across diverse infrastructure with built-in idempotency, drift detection, and reconciliation.

## Success Criteria

- [ ] Define state using declarative YAML/HCL syntax
- [ ] Idempotent state application (safe to run multiple times)
- [ ] State compilation and dependency resolution
- [ ] Dry-run mode (preview changes before applying)
- [ ] Drift detection between desired and actual state
- [ ] Automatic remediation of drift
- [ ] State templating with variables
- [ ] Modular state organization (reusable components)
- [ ] State execution time <60s for 100 resources

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                State Management Layer                    │
│  ┌─────────────┐  ┌──────────────┐  ┌───────────────┐  │
│  │   State     │  │  Dependency  │  │   Drift       │  │
│  │   Compiler  │  │   Resolver   │  │   Detector    │  │
│  └─────────────┘  └──────────────┘  └───────────────┘  │
│  ┌─────────────┐  ┌──────────────┐  ┌───────────────┐  │
│  │  Template   │  │  Renderer    │  │  Validator    │  │
│  │  Engine     │  │              │  │               │  │
│  └─────────────┘  └──────────────┘  └───────────────┘  │
└──────────────────────────────────────────────────────────┘
                           │
                           ▼
┌──────────────────────────────────────────────────────────┐
│                  State Modules                           │
│  ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐         │
│  │ File │ │ Pkg  │ │ Svc  │ │ User │ │ K8s  │   ...   │
│  └──────┘ └──────┘ └──────┘ └──────┘ └──────┘         │
└──────────────────────────────────────────────────────────┘
                           │
                           ▼
                    Agent Executors
```

## User Stories

### US3.1: Declarative State Definition
**As a** platform engineer
**I want to** define infrastructure configuration declaratively
**So that** I can maintain consistent state across systems

**Acceptance Criteria**:
- Define states in YAML or HCL format
- Support multiple state modules (file, package, service, user, etc.)
- State files organized in directory structure
- Support includes and imports
- Validate state syntax before execution
- Clear error messages for syntax errors

**Example**:
```yaml
# states/webserver.yaml
packages:
  nginx:
    state: installed
    version: ">=1.20"

files:
  /etc/nginx/nginx.conf:
    state: present
    source: kscore://nginx/nginx.conf
    user: root
    group: root
    mode: "0644"
    require:
      - packages: nginx

services:
  nginx:
    state: running
    enable: true
    require:
      - files: /etc/nginx/nginx.conf
    watch:
      - files: /etc/nginx/nginx.conf
```

### US3.2: Idempotent Execution
**As a** platform engineer
**I want to** run state application multiple times safely
**So that** I can ensure convergence without side effects

**Acceptance Criteria**:
- State modules check current state before making changes
- Only apply changes when needed (idempotent)
- Report what changed vs what stayed the same
- No duplicate actions on repeated runs
- Safe to include in automation/cron jobs

**Example Output**:
```
State: packages.nginx
Result: No changes needed (already installed: 1.20.1)

State: files./etc/nginx/nginx.conf
Result: Updated (content changed)
Changes:
  - mode: 0755 → 0644
  - content: [diff shown]

State: services.nginx
Result: Reloaded (watched file changed)
```

### US3.3: Dependency Management
**As a** platform engineer
**I want to** define dependencies between states
**So that** they execute in the correct order

**Acceptance Criteria**:
- `require`: Wait for dependency before executing
- `watch`: Execute when dependency changes
- `onchanges`: Only execute if dependency changed
- Automatic dependency graph construction
- Cycle detection with clear errors
- Parallel execution of independent states

**Example**:
```yaml
files:
  /app/config.yml:
    state: present
    source: kscore://app/config.yml

services:
  app:
    state: running
    require:
      - files: /app/config.yml
    watch:
      - files: /app/config.yml  # Restart on config change
```

### US3.4: Dry-Run and Preview
**As a** platform engineer
**I want to** preview changes before applying them
**So that** I can validate the impact safely

**Acceptance Criteria**:
- Dry-run mode: `kscorectl state apply --dry-run`
- Show what would change without making changes
- Highlight additions, modifications, deletions
- Include diff for file content changes
- Validate state compilation and dependencies
- Estimate execution time

**Example**:
```bash
kscorectl state apply webserver --dry-run --target "role:web"

Preview of changes:
  [web-01] Would update 3 states:
    - files./etc/nginx/nginx.conf: content would change
    - services.nginx: would reload
  [web-02] No changes needed
  [web-03] Would update 2 states:
    - packages.nginx: would install

Continue? [y/N]
```

### US3.5: Drift Detection
**As an** SRE
**I want to** detect when actual state diverges from desired state
**So that** I can identify unauthorized changes

**Acceptance Criteria**:
- Compare current state with desired state
- Report drift for each resource
- Identify source of drift (manual change, failed deployment)
- Track drift over time
- Alert on critical drift
- Schedule periodic drift checks

**Example**:
```bash
kscorectl state drift --target "role:web"

Drift detected on 2/10 agents:
  [web-03]
    - files./etc/nginx/nginx.conf:
        mode: expected=0644, actual=0777 (drift since 2024-01-15 10:30)
    - services.nginx: expected=running, actual=stopped

kscorectl state drift --fix  # Auto-remediate
```

### US3.6: Templating and Variables
**As a** platform engineer
**I want to** use templates and variables in state definitions
**So that** I can reuse configurations across environments

**Acceptance Criteria**:
- Template engine support (Go templates, Jinja2-like)
- Variable substitution from multiple sources
- Vars (configuration data)
- Facts (agent metadata)
- Environment-specific overrides
- Conditional logic in templates

**Example**:
```yaml
# states/app.yaml
files:
  /app/config.yml:
    state: present
    contents: |
      database:
        host: {{ vars.db_host }}
        port: {{ vars.db_port }}
      environment: {{ facts.environment }}
      replicas: {{ vars.replicas | default(3) }}
```

```yaml
# vars/production.yaml
db_host: db.prod.example.com
db_port: 5432
replicas: 5

# vars/staging.yaml
db_host: db.staging.example.com
db_port: 5432
replicas: 2
```

### US3.7: State Modules Library
**As a** platform engineer
**I want to** use pre-built state modules for common tasks
**So that** I don't reinvent the wheel

**Acceptance Criteria**:
- Core modules: file, package, service, user, group, cron
- Cloud modules: aws_s3, gcp_storage, azure_blob
- Kubernetes modules: k8s_deployment, k8s_service, k8s_configmap
- Container modules: docker_image, docker_container
- Extensible module system for custom modules
- Module documentation and examples

**Module Examples**:
```yaml
# Package management
packages:
  nginx:
    state: installed
    version: latest

  old-package:
    state: removed

# Service management
services:
  nginx:
    state: running
    enable: true

  deprecated-service:
    state: stopped
    enable: false

# User management
users:
  appuser:
    state: present
    uid: 1000
    groups: [docker, sudo]
    shell: /bin/bash

# Kubernetes resources
k8s_deployments:
  nginx:
    state: present
    namespace: default
    replicas: 3
    image: nginx:1.20
```

## Technical Tasks

### Phase 1: State DSL and Parser (Week 1-2)

**T1.1: State Definition Schema**
- Define YAML schema for states
- Support HCL as alternative format
- Create schema validation
- Implement state file loading
- Add include/import support

**T1.2: State Parser**
- Parse YAML/HCL state definitions
- Build internal state representation
- Validate state structure
- Report syntax errors with line numbers
- Support multiple file formats

**T1.3: Dependency Graph**
- Build directed acyclic graph (DAG) from states
- Implement topological sort for execution order
- Detect circular dependencies
- Calculate parallel execution opportunities
- Visualize dependency graph (optional)

### Phase 2: Template Engine (Week 3)

**T2.1: Template Support**
- Integrate Go template engine
- Add custom template functions
- Support Jinja2-like syntax (compatible with Salt)
- Implement template caching
- Add template debugging

**T2.2: Vars System**
- Design vars data structure
- Implement vars rendering
- Support environment-based vars
- Add vars encryption (optional)
- Create vars merging logic

**T2.3: Facts System**
- Collect agent metadata (OS, arch, IP, etc.)
- Support custom facts
- Implement fact matching for targeting
- Cache facts data
- Allow fact refresh

### Phase 3: State Modules (Week 4-5)

**T3.1: Module Framework**
- Define module interface
- Implement module registration
- Create module lifecycle (check, apply, test)
- Add module helpers
- Support module plugins

**T3.2: Core Modules**
- `file` - File and directory management
- `package` - Package installation (apt, yum, apk, brew)
- `service` - Service management (systemd, init.d, launchd)
- `user` - User and group management
- `cmd` - Command execution (for custom tasks)
- `git` - Git repository management
- `cron` - Cron job management

**T3.3: Cloud-Native Modules**
- `k8s_*` - Kubernetes resource management
- `docker_*` - Docker container/image management
- `helm` - Helm chart deployment
- `file_line` - Ensure line in file (for config edits)

### Phase 4: State Execution (Week 6)

**T4.1: State Compiler**
- Compile states into execution plan
- Resolve templates and variables
- Flatten includes and imports
- Validate all references
- Optimize execution order

**T4.2: State Runner**
- Execute states according to dependency order
- Implement parallel execution
- Handle state failures (fail-fast vs continue)
- Collect state results
- Generate execution report

**T4.3: Idempotency Logic**
- Implement "check before apply" pattern
- Track state changes
- Skip unchanged states
- Report what changed
- Optimize for repeated runs

### Phase 5: Drift Detection (Week 7)

**T5.1: State Snapshot**
- Capture current state of resources
- Store snapshots for comparison
- Implement efficient diff algorithm
- Track snapshot history

**T5.2: Drift Detection**
- Compare desired vs actual state
- Identify drift sources
- Calculate drift severity
- Generate drift reports
- Schedule periodic drift checks

**T5.3: Auto-Remediation**
- Trigger state application on drift
- Configurable remediation policies
- Alert before remediation
- Track remediation history
- Support approval workflows

### Phase 6: CLI and API (Week 8)

**T6.1: CLI Commands**
- `kscorectl state apply` - Apply states
- `kscorectl state compile` - Compile and validate
- `kscorectl state drift` - Detect drift
- `kscorectl state show` - Show compiled state
- `kscorectl vars get` - View vars data

**T6.2: API Endpoints**
- `POST /api/v1/state/apply` - Apply state
- `GET /api/v1/state/drift` - Get drift report
- `POST /api/v1/state/compile` - Compile state
- `GET /api/v1/vars` - Get vars data

## Dependencies

- **Epic 1**: Core Infrastructure
- **Epic 2**: Remote Execution
- **Go Libraries**:
  - `gopkg.in/yaml.v3` - YAML parsing
  - `github.com/hashicorp/hcl/v2` - HCL parsing
  - `text/template` - Go templates
  - `github.com/yourbasic/graph` - Graph algorithms
  - `github.com/pmezard/go-difflib` - Diff generation

## Risks & Mitigations

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Template complexity leads to errors | High | Medium | Extensive validation, linting, dry-run |
| Circular dependencies | High | Low | Detect at compile time, clear errors |
| State execution performance | Medium | Medium | Parallel execution, caching, profiling |
| Module compatibility across OS | High | High | Extensive cross-platform testing |
| Vars data security | Critical | Medium | Encryption at rest, access controls |

## Metrics & Monitoring

### Key Metrics
- State compilation time (ms)
- State execution time per module (ms)
- Number of states changed vs unchanged
- Drift detection frequency
- Remediation success rate
- Template rendering time

### Alerts
- State application failure rate >5%
- Critical drift detected
- Circular dependency detected
- Template rendering errors
- Module execution timeout

## Testing Strategy

### Unit Tests
- State parser with various YAML/HCL
- Dependency graph construction
- Template rendering with edge cases
- Each state module independently
- Idempotency verification

### Integration Tests
- Multi-state application
- Cross-module dependencies
- Drift detection and remediation
- Template + vars + facts integration
- Error scenarios

### Platform Tests
- Test on Ubuntu, CentOS, Alpine, macOS, Windows
- Package managers: apt, yum, apk, brew, chocolatey
- Service managers: systemd, init.d, launchd, Windows services

## Documentation Requirements

- [ ] State syntax reference
- [ ] Module documentation for each module
- [ ] Templating guide with examples
- [ ] Vars best practices
- [ ] Dependency management guide
- [ ] Drift detection setup
- [ ] State organization patterns
- [ ] Migration guide from Salt Project/Ansible

## Definition of Done

- [ ] All user stories implemented
- [ ] Core modules working on major platforms
- [ ] Unit test coverage >85%
- [ ] Integration tests passing
- [ ] Documentation complete
- [ ] Performance benchmarks met
- [ ] Example states for common scenarios
- [ ] Ready for production use
