# Epic 29: Bootstrap Testing Infrastructure

## Overview

Create a comprehensive testing infrastructure that validates the bootstrap experience across multiple deployment scenarios, platforms, and configurations. This epic focuses on containerized CI coverage; VM-based validation is deferred to the future release readiness epic.

**Goal**: Ensure the bootstrap experience works reliably across all supported platforms and deployment modes through automated testing.

## Success Criteria

1. Docker-based tests cover all bootstrap scenarios in CI/CD
2. Multi-platform matrix testing (Ubuntu, Debian, RHEL, Rocky, Fedora, Alpine)
3. All deployment modes tested (demo, production, enterprise)
4. Blueprint application testing
5. Upgrade and rollback testing
6. Performance benchmarks for bootstrap times

## User Stories

### US29.1: CI/CD Bootstrap Testing
**As a** developer
**I want to** run bootstrap tests in CI/CD pipelines
**So that** I catch regressions before release

**Acceptance Criteria**:
- Docker-based tests run without external dependencies
- Tests complete in reasonable time (<30 minutes)
- Coverage of all bootstrap modes
- Clear failure reporting

### US29.2: VM-Based Validation (Deferred)
**Deferred** to future release readiness epic. VM-based validation and provider integration are out of scope for Epic 29.

### US29.3: Platform Matrix Testing
**As a** maintainer
**I want to** test across all supported Linux distributions
**So that** I know the bootstrap works everywhere we claim

**Acceptance Criteria**:
- Test matrix covers Ubuntu 20.04/22.04/24.04
- Test matrix covers Debian 11/12
- Test matrix covers RHEL 8/9, Rocky 8/9
- Test matrix covers Fedora latest
- Test matrix covers Alpine latest
- Platform-specific issues detected

### US29.4: Long-Running Stability Tests
**As an** SRE
**I want to** run extended stability tests
**So that** I'm confident the deployment is stable over time

**Acceptance Criteria**:
- 24-hour stability test suite
- Chaos testing (node failures, network partitions)
- Resource usage monitoring
- Memory leak detection

## Technical Tasks

### Phase 1: Docker-Based Test Infrastructure (Weeks 1-4)

#### T1.1: Test Framework Core
```
test/bootstrap/
├── framework/
│   ├── docker.go           # Docker test environment management
│   ├── vm.go               # VM test environment management
│   ├── config.go           # Test configuration
│   ├── assertions.go       # Bootstrap-specific assertions
│   ├── reporter.go         # Test result reporting
│   └── cleanup.go          # Resource cleanup
├── containers/
│   ├── Dockerfile.ubuntu   # Ubuntu test image
│   ├── Dockerfile.debian   # Debian test image
│   ├── Dockerfile.rhel     # RHEL/Rocky test image
│   ├── Dockerfile.fedora   # Fedora test image
│   ├── Dockerfile.alpine   # Alpine test image
│   └── docker-compose.yml  # Multi-container test setup
├── scenarios/
│   ├── demo_test.go        # Demo mode tests
│   ├── production_test.go  # Production mode tests
│   ├── cluster_test.go     # Cluster formation tests
│   ├── join_test.go        # Cluster join tests
│   ├── blueprint_test.go   # Blueprint application tests
│   └── upgrade_test.go     # Upgrade scenario tests
├── platforms/
│   ├── ubuntu_test.go      # Ubuntu-specific tests
│   ├── debian_test.go      # Debian-specific tests
│   ├── rhel_test.go        # RHEL/Rocky-specific tests
│   ├── fedora_test.go      # Fedora-specific tests
│   └── alpine_test.go      # Alpine-specific tests
├── vm/
│   ├── config.yaml         # VM test configuration
│   ├── providers/
│   │   ├── ssh.go          # SSH-based VM access
│   │   ├── vagrant.go      # Vagrant provider
│   │   └── cloud.go        # Cloud VM provider (AWS, GCP, Azure)
│   └── scenarios/
│       ├── single_node_test.go
│       ├── ha_cluster_test.go
│       └── multi_region_test.go
├── Makefile                # Test orchestration
└── README.md               # Test documentation
```

#### T1.2: Docker Test Environment
- Base images for each supported distribution
- Systemd support in containers (where applicable)
- Network isolation between test runs
- Volume mounts for artifact collection
- Container cleanup after tests

#### T1.3: Test Configuration System
```yaml
# test/bootstrap/config.yaml
test:
  timeout: 30m
  parallel: 4
  retry_failed: 2

docker:
  registry: ghcr.io/shawnbutts/keystone-core
  cache_from: true

platforms:
  - name: ubuntu-22.04
    image: Dockerfile.ubuntu
    args:
      VERSION: "22.04"
    enabled: true
  - name: debian-12
    image: Dockerfile.debian
    args:
      VERSION: "12"
    enabled: true
  - name: rhel-9
    image: Dockerfile.rhel
    args:
      VERSION: "9"
    enabled: true
  - name: rocky-9
    image: Dockerfile.rhel
    args:
      VERSION: "rocky-9"
    enabled: true
  - name: fedora-latest
    image: Dockerfile.fedora
    enabled: true
  - name: alpine-latest
    image: Dockerfile.alpine
    enabled: true

scenarios:
  - name: demo
    enabled: true
    timeout: 10m
  - name: production-single
    enabled: true
    timeout: 15m
  - name: production-cluster
    enabled: true
    timeout: 20m
    nodes: 3
  - name: enterprise
    enabled: true
    timeout: 30m
    nodes: 5
```

#### T1.4: Platform-Specific Test Images
```dockerfile
# Dockerfile.ubuntu
FROM ubuntu:22.04

# Install systemd for service management testing
RUN apt-get update && apt-get install -y \
    systemd systemd-sysv \
    curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Prepare for systemd
RUN cd /lib/systemd/system/sysinit.target.wants/ && \
    ls | grep -v systemd-tmpfiles-setup | xargs rm -f

VOLUME ["/sys/fs/cgroup"]
CMD ["/lib/systemd/systemd"]
```

### Phase 2: Demo Mode Tests (Weeks 5-6)

#### T2.1: Basic Demo Bootstrap Test
```go
func TestBootstrapDemo(t *testing.T) {
    env := framework.NewDockerEnvironment(t, "ubuntu-22.04")
    defer env.Cleanup()

    // Copy agent binary
    env.CopyFile("bin/kscore-agent", "/usr/local/bin/kscore-agent")

    // Run bootstrap in demo mode
    result := env.Exec("kscore-agent", "bootstrap",
        "--mode", "demo",
        "--non-interactive")

    require.NoError(t, result.Error)
    require.Equal(t, 0, result.ExitCode)

    // Verify services are running
    env.AssertServiceRunning("kscore-server")
    env.AssertServiceRunning("kscore-agent")

    // Verify API is accessible
    env.AssertHTTPOK("http://localhost:8080/health")

    // Verify agent is registered
    env.AssertAgentRegistered("localhost")
}
```

#### T2.2: Demo with Examples Test
```go
func TestBootstrapDemoWithExamples(t *testing.T) {
    env := framework.NewDockerEnvironment(t, "ubuntu-22.04")
    defer env.Cleanup()

    env.CopyFile("bin/kscore-agent", "/usr/local/bin/kscore-agent")

    result := env.Exec("kscore-agent", "bootstrap",
        "--mode", "demo",
        "--apply-blueprint", "kscore/demo",
        "--non-interactive")

    require.NoError(t, result.Error)

    // Verify example states were applied
    env.AssertFileExists("/etc/keystone-core/examples/")

    // Verify dashboards are accessible
    env.AssertHTTPOK("http://localhost:3000/api/health") // Grafana
}
```

#### T2.3: Demo Idempotency Test
```go
func TestBootstrapDemoIdempotent(t *testing.T) {
    env := framework.NewDockerEnvironment(t, "ubuntu-22.04")
    defer env.Cleanup()

    env.CopyFile("bin/kscore-agent", "/usr/local/bin/kscore-agent")

    // First bootstrap
    result1 := env.Exec("kscore-agent", "bootstrap",
        "--mode", "demo", "--non-interactive")
    require.NoError(t, result1.Error)

    // Second bootstrap (should succeed, no changes)
    result2 := env.Exec("kscore-agent", "bootstrap",
        "--mode", "demo", "--non-interactive")
    require.NoError(t, result2.Error)

    // Services should still be running
    env.AssertServiceRunning("kscore-server")
}
```

### Phase 3: Production Mode Tests (Weeks 7-8)

#### T3.1: Single-Node Production Test
```go
func TestBootstrapProductionSingleNode(t *testing.T) {
    env := framework.NewDockerEnvironment(t, "ubuntu-22.04")
    postgres := env.StartPostgres()
    defer env.Cleanup()

    env.CopyFile("bin/kscore-agent", "/usr/local/bin/kscore-agent")

    result := env.Exec("kscore-agent", "bootstrap",
        "--mode", "production",
        "--cluster-name", "test",
        "--node-role", "both",
        "--postgres-host", postgres.Host,
        "--postgres-port", postgres.Port,
        "--postgres-database", "keystone",
        "--postgres-user", "keystone",
        "--postgres-password", "testpass",
        "--generate-certs",
        "--non-interactive")

    require.NoError(t, result.Error)

    env.AssertServiceRunning("kscore-server")
    env.AssertPostgresConnected(postgres)
    env.AssertCertificatesGenerated("/etc/keystone-core/certs/")
}
```

#### T3.2: Multi-Node Cluster Formation Test
```go
func TestBootstrapProductionCluster(t *testing.T) {
    network := framework.NewDockerNetwork(t, "test-cluster")
    defer network.Cleanup()

    // Start 3 control plane nodes
    nodes := make([]*framework.DockerEnvironment, 3)
    for i := 0; i < 3; i++ {
        nodes[i] = framework.NewDockerEnvironmentWithNetwork(t,
            "ubuntu-22.04", network, fmt.Sprintf("cp%d", i+1))
        nodes[i].CopyFile("bin/kscore-agent", "/usr/local/bin/kscore-agent")
    }

    postgres := network.StartPostgres()
    defer func() {
        for _, n := range nodes { n.Cleanup() }
    }()

    // Bootstrap first node
    result := nodes[0].Exec("kscore-agent", "bootstrap",
        "--mode", "production",
        "--cluster-name", "test-cluster",
        "--node-role", "control-plane",
        "--control-plane-nodes", "cp1,cp2,cp3",
        "--postgres-host", postgres.Host,
        "--generate-certs",
        "--non-interactive")
    require.NoError(t, result.Error)

    // Get join token
    tokenResult := nodes[0].Exec("kscore-server", "token", "create")
    joinToken := strings.TrimSpace(tokenResult.Stdout)

    // Join remaining nodes
    for i := 1; i < 3; i++ {
        result := nodes[i].Exec("kscore-agent", "bootstrap",
            "--join", "https://cp1:8443",
            "--join-token", joinToken,
            "--node-role", "control-plane",
            "--non-interactive")
        require.NoError(t, result.Error)
    }

    // Verify cluster formation
    nodes[0].AssertClusterSize(3)
    nodes[0].AssertLeaderElected()
}
```

#### T3.3: Cluster Join as Agent Test
```go
func TestBootstrapJoinAsAgent(t *testing.T) {
    // Setup cluster first...
    cluster := setupTestCluster(t, 1)
    defer cluster.Cleanup()

    // Create agent node
    agent := framework.NewDockerEnvironmentWithNetwork(t,
        "ubuntu-22.04", cluster.Network, "agent1")
    agent.CopyFile("bin/kscore-agent", "/usr/local/bin/kscore-agent")
    defer agent.Cleanup()

    joinToken := cluster.CreateJoinToken()

    result := agent.Exec("kscore-agent", "bootstrap",
        "--join", cluster.Endpoint(),
        "--join-token", joinToken,
        "--node-role", "agent",
        "--node-labels", "role=webserver,env=test",
        "--non-interactive")

    require.NoError(t, result.Error)

    cluster.AssertAgentRegistered("agent1")
    cluster.AssertAgentLabels("agent1", map[string]string{
        "role": "webserver",
        "env":  "test",
    })
}
```

### Phase 4: VM-Based Test Infrastructure (Weeks 9-12) — Deferred to future release readiness epic

#### T4.1: VM Configuration System
```yaml
# test/bootstrap/vm/config.yaml
# This file can be populated by the user or CI/CD system

vm_provider: ssh  # ssh, vagrant, aws, gcp, azure

ssh:
  # User-provided VMs for testing
  nodes:
    - name: cp1
      host: 192.168.1.100
      port: 22
      user: root
      key_file: ~/.ssh/id_rsa
      # Or password: secret
      os: ubuntu-22.04
      role: control-plane

    - name: cp2
      host: 192.168.1.101
      port: 22
      user: root
      key_file: ~/.ssh/id_rsa
      os: ubuntu-22.04
      role: control-plane

    - name: cp3
      host: 192.168.1.102
      port: 22
      user: root
      key_file: ~/.ssh/id_rsa
      os: ubuntu-22.04
      role: control-plane

    - name: agent1
      host: 192.168.1.110
      port: 22
      user: root
      key_file: ~/.ssh/id_rsa
      os: rocky-9
      role: agent

    - name: agent2
      host: 192.168.1.111
      port: 22
      user: root
      key_file: ~/.ssh/id_rsa
      os: debian-12
      role: agent

  # Optional: PostgreSQL server
  postgres:
    host: 192.168.1.200
    port: 5432
    user: keystone
    password: secret
    database: keystone_test

vagrant:
  # Vagrant-based VMs (auto-provisioned)
  box_prefix: "generic/"
  boxes:
    - name: cp1
      box: ubuntu2204
      memory: 2048
      cpus: 2
      role: control-plane
    - name: cp2
      box: ubuntu2204
      memory: 2048
      cpus: 2
      role: control-plane
    - name: agent1
      box: rocky9
      memory: 1024
      cpus: 1
      role: agent

aws:
  region: us-west-2
  instance_type: t3.medium
  key_name: keystone-test
  security_group: sg-xxx
  subnet: subnet-xxx
  ami_map:
    ubuntu-22.04: ami-xxx
    debian-12: ami-xxx
    rhel-9: ami-xxx
    rocky-9: ami-xxx
```

#### T4.2: VM Provider Interface
```go
// test/bootstrap/vm/providers/provider.go
type VMProvider interface {
    // Setup prepares the VM environment
    Setup(ctx context.Context) error

    // GetNode returns a node by name
    GetNode(name string) (*VMNode, error)

    // ListNodes returns all configured nodes
    ListNodes() []*VMNode

    // Cleanup destroys/releases VMs
    Cleanup(ctx context.Context) error
}

type VMNode struct {
    Name     string
    Host     string
    Port     int
    User     string
    OS       string
    Role     string

    // For SSH access
    SSHClient *ssh.Client
}

func (n *VMNode) Exec(cmd string, args ...string) (*ExecResult, error)
func (n *VMNode) CopyFile(local, remote string) error
func (n *VMNode) FetchFile(remote, local string) error
func (n *VMNode) WaitForSSH(timeout time.Duration) error
```

#### T4.3: SSH Provider
```go
// test/bootstrap/vm/providers/ssh.go
type SSHProvider struct {
    config *SSHConfig
    nodes  map[string]*VMNode
}

func NewSSHProvider(configPath string) (*SSHProvider, error) {
    cfg, err := loadSSHConfig(configPath)
    if err != nil {
        return nil, err
    }

    return &SSHProvider{
        config: cfg,
        nodes:  make(map[string]*VMNode),
    }, nil
}

func (p *SSHProvider) Setup(ctx context.Context) error {
    for _, nodeCfg := range p.config.Nodes {
        node, err := p.connectNode(ctx, nodeCfg)
        if err != nil {
            return fmt.Errorf("failed to connect to %s: %w", nodeCfg.Name, err)
        }

        // Optionally clean the node before testing
        if p.config.CleanNodes {
            if err := p.cleanNode(ctx, node); err != nil {
                return fmt.Errorf("failed to clean %s: %w", nodeCfg.Name, err)
            }
        }

        p.nodes[nodeCfg.Name] = node
    }
    return nil
}
```

#### T4.4: VM Test Runner
```go
// test/bootstrap/vm/runner.go
func RunVMTests(t *testing.T, configPath string) {
    if os.Getenv("KSCORE_VM_TESTS") != "1" {
        t.Skip("VM tests disabled (set KSCORE_VM_TESTS=1)")
    }

    provider, err := LoadProvider(configPath)
    require.NoError(t, err)

    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
    defer cancel()

    err = provider.Setup(ctx)
    require.NoError(t, err)
    defer provider.Cleanup(ctx)

    // Run test scenarios
    t.Run("SingleNode", func(t *testing.T) {
        testVMSingleNode(t, provider)
    })

    t.Run("HACluster", func(t *testing.T) {
        testVMHACluster(t, provider)
    })

    t.Run("MixedPlatforms", func(t *testing.T) {
        testVMMixedPlatforms(t, provider)
    })
}
```

### Phase 5: Platform Matrix Tests (Weeks 13-14)

#### T5.1: Platform Test Matrix
```go
var platformMatrix = []struct {
    name    string
    image   string
    pkgMgr  string
    initSys string
}{
    {"ubuntu-20.04", "Dockerfile.ubuntu", "apt", "systemd"},
    {"ubuntu-22.04", "Dockerfile.ubuntu", "apt", "systemd"},
    {"ubuntu-24.04", "Dockerfile.ubuntu", "apt", "systemd"},
    {"debian-11", "Dockerfile.debian", "apt", "systemd"},
    {"debian-12", "Dockerfile.debian", "apt", "systemd"},
    {"rhel-8", "Dockerfile.rhel", "dnf", "systemd"},
    {"rhel-9", "Dockerfile.rhel", "dnf", "systemd"},
    {"rocky-8", "Dockerfile.rhel", "dnf", "systemd"},
    {"rocky-9", "Dockerfile.rhel", "dnf", "systemd"},
    {"fedora-latest", "Dockerfile.fedora", "dnf", "systemd"},
    {"alpine-latest", "Dockerfile.alpine", "apk", "openrc"},
}

func TestBootstrapAllPlatforms(t *testing.T) {
    for _, platform := range platformMatrix {
        t.Run(platform.name, func(t *testing.T) {
            t.Parallel()

            env := framework.NewDockerEnvironment(t, platform.name)
            defer env.Cleanup()

            env.CopyFile("bin/kscore-agent", "/usr/local/bin/kscore-agent")

            result := env.Exec("kscore-agent", "bootstrap",
                "--mode", "demo",
                "--non-interactive")

            require.NoError(t, result.Error,
                "Bootstrap failed on %s: %s", platform.name, result.Stderr)

            env.AssertServiceRunning("kscore-server")
            env.AssertServiceRunning("kscore-agent")
        })
    }
}
```

#### T5.2: Package Manager Tests
```go
func TestPackageManagerIntegration(t *testing.T) {
    platforms := []struct {
        name   string
        pkgMgr string
        pkgs   []string
    }{
        {"ubuntu-22.04", "apt", []string{"kscore-server", "kscore-agent"}},
        {"rhel-9", "dnf", []string{"kscore-server", "kscore-agent"}},
        {"alpine-latest", "apk", []string{"kscore-server", "kscore-agent"}},
    }

    for _, p := range platforms {
        t.Run(p.name, func(t *testing.T) {
            env := framework.NewDockerEnvironment(t, p.name)
            defer env.Cleanup()

            // Bootstrap should configure repo
            env.CopyFile("bin/kscore-agent", "/usr/local/bin/kscore-agent")
            env.Exec("kscore-agent", "bootstrap", "--mode", "demo", "--non-interactive")

            // Verify repo is configured
            env.AssertRepoConfigured("keystone-core")

            // Verify packages can be installed/upgraded
            for _, pkg := range p.pkgs {
                env.AssertPackageInstalled(pkg)
            }
        })
    }
}
```

### Phase 6: Blueprint Tests (Weeks 15-16)

#### T6.1: Blueprint Application Tests
```go
func TestBootstrapWithBlueprints(t *testing.T) {
    blueprints := []string{
        "kscore/demo",
        "kscore/monitoring-stack",
        "kscore/security-baseline",
    }

    for _, bp := range blueprints {
        t.Run(bp, func(t *testing.T) {
            env := framework.NewDockerEnvironment(t, "ubuntu-22.04")
            defer env.Cleanup()

            env.CopyFile("bin/kscore-agent", "/usr/local/bin/kscore-agent")
            env.CopyDir("blueprints/", "/usr/share/kscore/blueprints/")

            result := env.Exec("kscore-agent", "bootstrap",
                "--mode", "demo",
                "--blueprints-dir", "/usr/share/kscore/blueprints/",
                "--apply-blueprint", bp,
                "--non-interactive")

            require.NoError(t, result.Error)

            // Blueprint-specific assertions
            assertBlueprintApplied(t, env, bp)
        })
    }
}
```

#### T6.2: Production Cluster Blueprint Test
```go
func TestProductionClusterBlueprint(t *testing.T) {
    if os.Getenv("KSCORE_VM_TESTS") != "1" {
        t.Skip("Requires VM tests")
    }

    provider, _ := LoadProvider("vm/config.yaml")
    defer provider.Cleanup(context.Background())

    cp1 := provider.GetNode("cp1")
    cp2 := provider.GetNode("cp2")
    cp3 := provider.GetNode("cp3")

    // Bootstrap with production-cluster blueprint
    cp1.Exec("kscore-agent", "bootstrap",
        "--mode", "production",
        "--apply-blueprint", "kscore/production-cluster",
        "--non-interactive")

    // Join other nodes
    // ... (join logic)

    // Verify full cluster
    assertClusterHealthy(t, provider, 3)
    assertBlueprintApplied(t, cp1, "kscore/production-cluster")
}
```

### Phase 7: CI/CD Integration (Weeks 17-18)

#### T7.1: GitHub Actions Workflow
```yaml
# .github/workflows/bootstrap-tests.yml
name: Bootstrap Tests

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
  schedule:
    - cron: '0 2 * * *'  # Nightly full test

jobs:
  docker-tests:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        platform:
          - ubuntu-22.04
          - debian-12
          - rocky-9
          - alpine-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - name: Build binaries
        run: make build

      - name: Build test image
        run: |
          docker build -t bootstrap-test:${{ matrix.platform }} \
            -f test/bootstrap/containers/Dockerfile.${{ matrix.platform }} .

      - name: Run bootstrap tests
        run: |
          KSCORE_TEST_PLATFORM=${{ matrix.platform }} \
          go test -v ./test/bootstrap/scenarios/... \
            -timeout 30m

      - name: Upload logs
        if: failure()
        uses: actions/upload-artifact@v4
        with:
          name: logs-${{ matrix.platform }}
          path: test/bootstrap/logs/

  cluster-tests:
    runs-on: ubuntu-latest
    needs: docker-tests
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - name: Build binaries
        run: make build

      - name: Run cluster tests
        run: |
          go test -v ./test/bootstrap/scenarios/cluster_test.go \
            -timeout 45m

  # VM tests run on self-hosted runners with VM access (deferred to future release readiness epic)
  vm-tests:
    runs-on: self-hosted
    if: github.event_name == 'schedule' || github.event_name == 'workflow_dispatch'
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - name: Build binaries
        run: make build

      - name: Run VM tests
        env:
          KSCORE_VM_TESTS: "1"
          KSCORE_VM_CONFIG: ${{ secrets.VM_CONFIG }}
        run: |
          echo "$KSCORE_VM_CONFIG" > test/bootstrap/vm/config.yaml
          go test -v ./test/bootstrap/vm/... -timeout 2h
```

#### T7.2: Test Reporting
- JUnit XML output for CI/CD systems
- HTML test reports
- Performance metrics collection
- Test coverage reporting

### Phase 8: Documentation and Maintenance (Weeks 19-20)

#### T8.1: Test Documentation
- README with test overview
- How to run tests locally
- How to configure VM tests
- How to add new test scenarios
- Troubleshooting guide

#### T8.2: Makefile Targets
```makefile
# test/bootstrap/Makefile

.PHONY: test-docker test-vm test-all

# Run all Docker-based tests
test-docker:
	go test -v ./scenarios/... -timeout 30m

# Run tests for specific platform
test-platform:
	KSCORE_TEST_PLATFORM=$(PLATFORM) go test -v ./platforms/... -timeout 30m

# Run VM tests (requires config.yaml)
test-vm:
	KSCORE_VM_TESTS=1 go test -v ./vm/... -timeout 2h

# Run full test suite
test-all: test-docker test-vm

# Run quick smoke tests
test-smoke:
	go test -v ./scenarios/demo_test.go -timeout 10m

# Build test images
build-images:
	docker build -t bootstrap-test:ubuntu-22.04 -f containers/Dockerfile.ubuntu --build-arg VERSION=22.04 .
	docker build -t bootstrap-test:debian-12 -f containers/Dockerfile.debian --build-arg VERSION=12 .
	docker build -t bootstrap-test:rocky-9 -f containers/Dockerfile.rhel --build-arg VERSION=rocky-9 .
	docker build -t bootstrap-test:alpine-latest -f containers/Dockerfile.alpine .

# Clean up test artifacts
clean:
	docker compose -f containers/docker-compose.yml down -v
	rm -rf logs/ reports/
```

## Dependencies

- **Epic 27** (Agent Bootstrap): Bootstrap command to test
- **Epic 28** (Standard Blueprints): Blueprints to test
- **Epic 12** (E2E Testing): Existing E2E infrastructure

## Risks and Mitigations

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Flaky tests due to timing | High | Medium | Retry logic, proper waits |
| VM access issues | Medium | High | Multiple provider options |
| Platform-specific failures | High | Medium | Comprehensive error reporting |
| Long test times | Medium | Medium | Parallel execution, caching |

## Testing Strategy

This epic IS the testing strategy. Meta-testing includes:
- Test framework unit tests
- Provider integration tests
- CI/CD pipeline validation

## Definition of Done

- [ ] Docker-based test framework implemented
- [ ] VM-based test framework with SSH provider (deferred to future release readiness epic)
- [ ] Test images for all supported platforms
- [ ] Demo mode test suite passing
- [ ] Production mode test suite passing
- [ ] Cluster formation test suite passing
- [ ] Blueprint application tests passing
- [ ] Platform matrix tests passing (all distributions)
- [ ] GitHub Actions workflow configured
- [ ] VM test configuration documented (deferred to future release readiness epic)

## Deferred to Future Release Readiness Epic

The following items are deferred to the future release readiness epic:
- VM-based validation (SSH provider, VM scenarios, and self-hosted CI job)
- [ ] Test documentation complete
- [ ] Code review approved
