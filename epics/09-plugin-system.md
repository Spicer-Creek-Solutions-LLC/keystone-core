# Epic 9: Plugin System & Extensibility

## Overview

Implement a secure, sandboxed plugin system that enables users to extend Keystone Core functionality through custom state modules, execution handlers, policy rules, and reactors using Starlark or WebAssembly with capability-based security and cryptographic verification.

**Goal**: Enable safe extensibility without compromising security, with deterministic execution, minimal attack surface, and complete auditability of plugin capabilities.

## Success Criteria

- [ ] **Module Format**:
  - [ ] `module.yaml` manifest with dependencies, capabilities, limits
  - [ ] `module.lock` for reproducible builds
  - [ ] Structured directory layout (states/, providers/, tests/)
- [ ] **Dependency Management**:
  - [ ] SemVer version constraint resolution
  - [ ] Transitive dependency resolution
  - [ ] Circular dependency detection
  - [ ] Conflict resolution (MVS algorithm)
- [ ] **Registry & Distribution**:
  - [ ] Hybrid OCI + HTTP proxy registry
  - [ ] Go-mod-style HTTP endpoints
  - [ ] SumDB-style transparency log
  - [ ] Air-gapped mirroring support
- [ ] **Cryptographic Verification**:
  - [ ] Cosign signature verification for all modules
  - [ ] SHA256 hash verification via SumDB
  - [ ] Merkle proof verification
- [ ] **Runtimes**:
  - [ ] Starlark runtime with sandboxing
  - [ ] WASM runtime with WASI interface
  - [ ] Deterministic, side-effect-free execution
- [ ] **Security**:
  - [ ] Capability-based security model (no ambient authority)
  - [ ] Minimal, audited host capability interfaces
  - [ ] Policy-controlled capability grants
  - [ ] Resource isolation (CPU, memory, execution time limits)
  - [ ] SPIFFE identity-based module authentication (Epic 6 integration)
  - [ ] SPIFFE selector validation for capability grants
  - [ ] Module attestation via SPIRE workload API
- [ ] **Developer Experience**:
  - [ ] Module CLI (init, build, sign, publish, resolve, install)
  - [ ] SDKs for Starlark and Rust
  - [ ] Example modules and templates
  - [ ] Standard library modules
- [ ] **Performance**:
  - [ ] Module execution overhead: <10ms per invocation
  - [ ] Dependency resolution: <5s for 100 modules
  - [ ] Hot-reload without restart

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                Plugin Distribution                       │
│  ┌────────────────┐  ┌────────────────┐                 │
│  │  Transparency  │  │   Cosign       │                 │
│  │  Log (SumDB)   │  │   Signatures   │                 │
│  └────────────────┘  └────────────────┘                 │
└─────────────────┬────────────────────────────────────────┘
                  │ (verify & download)
                  ▼
┌──────────────────────────────────────────────────────────┐
│              Plugin Registry & Loader                    │
│  ┌────────────┐  ┌────────────┐  ┌─────────────────┐   │
│  │  Manifest  │  │ Signature  │  │   Capability    │   │
│  │  Parser    │  │ Verifier   │  │   Validator     │   │
│  └────────────┘  └────────────┘  └─────────────────┘   │
└──────────────────┬───────────────────────────────────────┘
                   │
        ┌──────────┴──────────┐
        │                     │
        ▼                     ▼
┌──────────────┐      ┌──────────────┐
│   Starlark   │      │     WASM     │
│   Runtime    │      │   Runtime    │
│              │      │  (wasmtime)  │
└──────┬───────┘      └──────┬───────┘
       │                     │
       └──────────┬──────────┘
                  │
                  ▼
┌──────────────────────────────────────────────────────────┐
│           Host Capability Layer                          │
│  ┌──────┐ ┌──────┐ ┌──────┐ ┌────────┐ ┌─────┐        │
│  │  fs  │ │ http │ │ exec │ │secrets │ │ log │  ...   │
│  └──────┘ └──────┘ └──────┘ └────────┘ └─────┘        │
│  (each capability registered only if policy allows)     │
└──────────────────────────────────────────────────────────┘
```

## Module Format & Structure

Keystone Core plugins are distributed as **modules** - versioned, signed packages with explicit dependencies and capabilities.

### Module Directory Structure

```
my_module/
├── module.yaml          # Manifest (capabilities, deps, limits, entrypoints)
├── module.lock          # Pinned dependency versions (optional, generated)
├── states/              # Starlark state definitions
│   ├── apply.star
│   └── verify.star
├── providers/           # WASM executables
│   └── executor.wasm
├── tests/               # Local tests
│   ├── test_apply.star
│   └── test_verify.star
├── SBOM/                # Optional: Software Bill of Materials
└── PROVENANCE/          # Optional: Build provenance attestation
```

### Module Manifest (`module.yaml`)

```yaml
schemaVersion: 1
name: vendor/pkg_apt              # Module identifier (namespaced)
version: v1.2.3                   # Semantic version

# Module dependencies with version constraints
dependencies:
  - module: std/files
    version: ">=1.0 <2.0"         # SemVer range
  - module: std/exec
    version: "^1.5.0"

# Required capabilities with scoping
capabilities:
  fs.read:
    - /etc/apt/*.list
    - /var/lib/apt/lists/**
  fs.write:
    - /etc/apt/sources.list.d/**
  exec:
    - /usr/bin/apt-get
    - /usr/bin/dpkg

# Resource limits
limits:
  time_ms: 5000                   # Max execution time
  mem_pages: 512                  # WASM memory pages (32KB each)
  cpu_shares: 100                 # CPU weight

# Starlark entrypoints
starlark:
  entrypoints:
    check: "states/verify.star:check"
    apply: "states/apply.star:apply"

# WASM exports (optional, for WASM modules)
wasm:
  binary: "providers/executor.wasm"
  exports: ["check", "apply", "rollback"]

# Cryptographic verification
signatures:
  - keyid: "vendor-signing-key-2024"
    algorithm: cosign
    signature: "MEUCIQC..."

# Build metadata
build:
  timestamp: "2024-01-15T10:30:00Z"
  reproducible: true
  builder: "github.com/vendor/pkg_apt@v1.2.3"
```

### Module Lock File (`module.lock`)

Generated by the resolver to pin exact versions:

```yaml
schemaVersion: 1
modules:
  - name: vendor/pkg_apt
    version: v1.2.3
    hash: sha256:abc123def456...

  - name: std/files
    version: v1.4.2
    hash: sha256:789xyz012abc...
    resolved: 2024-01-15T10:30:00Z

  - name: std/exec
    version: v1.5.1
    hash: sha256:fed456cba987...
    resolved: 2024-01-15T10:30:00Z
```

### Module Dependencies

Modules can depend on other modules, similar to Go modules or npm packages:

- **Version Constraints**: Use SemVer ranges (`>=1.0 <2.0`, `^1.5.0`, `~1.2.3`)
- **Dependency Resolution**: Resolver constructs a DAG and finds compatible versions
- **Lock File**: Pins exact versions for reproducible builds
- **Content Addressing**: Modules verified by SHA256 hash
- **Transitive Dependencies**: Automatically resolved and verified

**Starlark Usage**:
```python
# Load from dependency
load("std/strings@v1:strings.star", "to_upper", "split")
load("std/files@v1:files.star", "read_file")

def apply(pkg_name):
    content = read_file("/etc/apt/sources.list")
    lines = split(content, "\n")
    # ...
```

**WASM Usage**:
```rust
// Call other modules via host API
let result = host::invoke(
    "std/exec",
    "run_command",
    json!({"cmd": "apt-get", "args": ["update"]})
)?;
```

## Security Model

### Capability-Based Security

```yaml
# Plugin requests capabilities explicitly
# Host grants only what's needed and allowed by policy
# No ambient authority - plugin can't access anything not granted

Plugin manifest declares:
  required_capabilities: [fs.read, http.get, log.info]

Policy evaluates:
  - Is plugin signed by trusted key?
  - Is signature in transparency log?
  - Are requested capabilities allowed for this plugin?
  - Does the risk profile match security posture?

Host grants:
  fs.read -> scoped to /tmp/plugin-workspace only
  http.get -> scoped to allowlisted domains only
  log.info -> rate-limited to prevent DoS
```

### Determinism Guarantees

```
Plugins must be:
- **Pure functions**: Same input → same output
- **No side effects**: Can't modify global state
- **No non-deterministic APIs**: No random(), Date.now() without explicit capability
- **Reproducible**: Same plugin version + input = identical output

This enables:
- Audit replay (reproduce plugin behavior exactly)
- Deterministic testing
- Safe parallel execution
- Predictable resource usage
```

## User Stories

### US9.1: Starlark Plugin Development
**As a** platform engineer
**I want to** write plugins in Starlark
**So that** I can extend Keystone Core with simple, safe scripts

**Acceptance Criteria**:
- Write plugins in Starlark (Python-like syntax)
- Starlark runtime with sandboxing
- No arbitrary code execution outside sandbox
- Deterministic execution (no random, time without capability)
- Plugin can declare capabilities in manifest
- Hot-reload on plugin file change

**Example Starlark Plugin**:
```python
# plugins/custom-health-check.star
"""
Custom health check plugin for web applications.
Manifest: health-check-manifest.yaml
"""

# Plugin metadata (embedded in file)
plugin = {
    "name": "custom-health-check",
    "version": "1.0.0",
    "author": "security-team",
    "capabilities": ["http.get", "log.info"]
}

def check_health(url, timeout=30):
    """
    Check if web application is healthy.

    Args:
        url: Application URL
        timeout: Request timeout in seconds

    Returns:
        dict with status and details
    """
    # Use granted http.get capability
    response = http.get(url, timeout=timeout)

    # Log the check (capability: log.info)
    log.info("Health check for %s: %d" % (url, response.status))

    # Deterministic logic only
    if response.status == 200:
        body = response.json()
        if body.get("status") == "healthy":
            return {"healthy": True, "latency_ms": response.duration}

    return {"healthy": False, "status_code": response.status}

# Export the main function
exports = {
    "check_health": check_health
}
```

**Plugin Manifest**:
```yaml
# plugins/health-check-manifest.yaml
apiVersion: plugin.kscore.io/v1
kind: PluginManifest
metadata:
  name: custom-health-check
  version: 1.0.0
  author: security-team@example.com

spec:
  runtime: starlark
  source: custom-health-check.star

  # Explicit capability declarations
  capabilities:
    http.get:
      allowed_domains:
        - "*.example.com"
        - "localhost"
      timeout_max: 30s
    log.info:
      rate_limit: 100/minute

  # Resource limits
  limits:
    memory: 10MB
    cpu: 100m
    execution_time: 30s

  # Cryptographic verification
  signatures:
    - algorithm: cosign
      keyid: "security-team-key-2024"
      signature: "MEUCIQC..."

  # Dependencies
  dependencies: []
```

### US9.2: WASM Plugin Development
**As a** plugin developer
**I want to** write high-performance plugins in any language
**So that** I can use Rust/Go/C++ for complex logic

**Acceptance Criteria**:
- Compile plugins to WASM (from Rust, Go, C++, etc.)
- WASM runtime with WASI interface
- Capability grants via WASI imports
- Memory isolation
- Instruction metering (prevent infinite loops)

**Example Rust WASM Plugin**:
```rust
// plugins/advanced-parser/src/lib.rs
use kscore_plugin_sdk::prelude::*;

#[kscore_plugin]
pub struct LogParser {
    // Plugin state (isolated per invocation)
}

#[kscore_plugin_impl]
impl LogParser {
    /// Parse nginx access logs
    /// Capabilities: fs.read, log.debug
    pub fn parse_nginx_logs(&self, path: &str) -> Result<Vec<LogEntry>> {
        // Capability check done by host
        let content = fs::read_to_string(path)?;

        let mut entries = Vec::new();
        for line in content.lines() {
            if let Some(entry) = self.parse_line(line) {
                entries.push(entry);
            }
        }

        log::debug("Parsed {} log entries", entries.len());
        Ok(entries)
    }

    fn parse_line(&self, line: &str) -> Option<LogEntry> {
        // Complex parsing logic in Rust for performance
        // Pure function - no side effects
        // ...
    }
}

// Export plugin metadata
#[no_mangle]
pub fn plugin_metadata() -> PluginMetadata {
    PluginMetadata {
        name: "nginx-log-parser",
        version: "1.0.0",
        capabilities: vec!["fs.read", "log.debug"],
    }
}
```

**WASM Plugin Manifest**:
```yaml
apiVersion: plugin.kscore.io/v1
kind: PluginManifest
metadata:
  name: nginx-log-parser
  version: 1.0.0

spec:
  runtime: wasm
  source: nginx-log-parser.wasm

  capabilities:
    fs.read:
      allowed_paths:
        - "/var/log/nginx/**"
      max_file_size: 100MB
    log.debug:
      rate_limit: 1000/minute

  limits:
    memory: 50MB
    cpu: 500m
    execution_time: 60s
    instructions: 1_000_000_000  # Prevent infinite loops

  signatures:
    - algorithm: cosign
      keyid: "dev-team-key-2024"
      signature: "MEYCIQD..."
```

### US9.3: Plugin Signature Verification
**As a** security engineer
**I want to** verify plugin authenticity cryptographically
**So that** only trusted plugins can execute

**Acceptance Criteria**:
- Cosign signature verification before loading
- Support for multiple signing keys (key rotation)
- Signature verification failures prevent plugin load
- Integration with organization's signing infrastructure
- Audit log of all signature verifications

**Verification Flow**:
```bash
# Developer signs plugin
cosign sign --key security-team.key \
  plugin.star \
  manifest.yaml

# Plugin uploaded to registry with signature
kscorectl plugin publish custom-health-check \
  --manifest manifest.yaml \
  --signature manifest.yaml.sig

# Keystone Core verifies before loading
kscorectl plugin install custom-health-check
→ Downloading plugin...
→ Verifying signature with key security-team-key-2024...
→ ✓ Signature valid
→ Checking transparency log...
→ ✓ Found in transparency log (index: 12345)
→ Verifying capabilities against policy...
→ ✓ All capabilities allowed
→ Installing plugin...
→ ✓ Plugin installed successfully
```

### US9.4: Transparency Log (SumDB-style)
**As a** security team
**I want to** maintain a transparency log of all plugins
**So that** malicious plugin updates are detectable

**Acceptance Criteria**:
- Append-only log of all plugin versions
- Cryptographically verifiable (Merkle tree)
- Public audit log (or private for enterprise)
- Detect if plugin registry serves different versions
- Integration with existing SumDB infrastructure (Go-style)

**Transparency Log Structure**:
```
Keystone Core Plugin Transparency Log

Entry 12345:
  plugin: custom-health-check
  version: 1.0.0
  hash: sha256:abc123...
  signature: cosign:MEUCIQC...
  timestamp: 2024-01-15T10:30:00Z
  tree_hash: sha256:def456...

Merkle Proof:
  Level 0: abc123...
  Level 1: [abc123, 789xyz] → hash1
  Level 2: [hash1, hash2] → root

Root Hash: sha256:root123...
Signed by: transparency-log-key
```

**Detection of Tampering**:
```bash
# Client checks if registry serves same plugin everyone else sees
kscorectl plugin verify custom-health-check@1.0.0
→ Fetching from registry...
→ Hash: sha256:abc123...
→ Checking transparency log...
→ ✓ Hash matches log entry 12345
→ ✓ Merkle proof valid
→ Plugin is authentic

# If registry tampered with plugin:
→ ✗ Hash mismatch!
→ Registry served: sha256:malicious...
→ Transparency log has: sha256:abc123...
→ ⚠️  SECURITY WARNING: Possible compromise
```

### US9.4a: Hybrid Module Registry (OCI + HTTP Proxy)
**As a** module author
**I want to** publish modules to a registry with multiple access methods
**So that** users can fetch modules via OCI or Go-mod-style HTTP APIs

**Acceptance Criteria**:
- OCI registry as source of truth (stores signed module ZIPs)
- Go-mod-style HTTP proxy API for easy consumption
- SumDB integration for digest verification
- Support for air-gapped environments (registry mirroring)
- Version listing and metadata queries
- Bandwidth-efficient (delta updates, compression)

**Registry Architecture**:

```
┌─────────────────────────────────────────────────────────┐
│              Keystone Core Module Registry                 │
│                                                          │
│  ┌────────────────────────────────────────────────────┐ │
│  │           OCI Registry (Source of Truth)           │ │
│  │  • Stores module ZIPs as OCI artifacts             │ │
│  │  • Cosign signatures attached as layers            │ │
│  │  • SBOM and provenance attestations                │ │
│  │  • Content-addressed storage (SHA256)              │ │
│  └──────────────────┬─────────────────────────────────┘ │
│                     │                                    │
│  ┌──────────────────▼─────────────────────────────────┐ │
│  │         HTTP Proxy (Go-mod style API)             │ │
│  │  • Read-only interface for module resolution      │ │
│  │  • Caches OCI content for fast access             │ │
│  │  • Serves version lists, metadata, ZIPs           │ │
│  └──────────────────┬─────────────────────────────────┘ │
│                     │                                    │
│  ┌──────────────────▼─────────────────────────────────┐ │
│  │              SumDB (Transparency Log)              │ │
│  │  • Append-only log of module@version→hash         │ │
│  │  • Merkle tree for efficient proofs               │ │
│  │  • Prevents serving different versions to users   │ │
│  └────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
```

**HTTP Proxy Endpoints** (Go-mod style):

```bash
# List all versions of a module
GET /<module>/@v/list
→ v1.0.0
  v1.1.0
  v1.2.3

# Get version metadata
GET /<module>/@v/<version>.info
→ {"Version":"v1.2.3","Time":"2024-01-15T10:30:00Z"}

# Get module manifest (module.yaml)
GET /<module>/@v/<version>.mod
→ <module.yaml contents>

# Download module ZIP
GET /<module>/@v/<version>.zip
→ <binary module ZIP>

# SumDB lookup (verify hash)
GET /sumdb/lookup/<module>@<version>
→ <module>@<version> sha256:abc123...
  <merkle proof>
  <signed tree head>
```

**Module Publishing Flow**:

```bash
# 1. Developer builds and signs module
kscorectl module build
→ Building vendor/pkg_apt@v1.2.3...
→ ✓ Module built: dist/vendor_pkg_apt_v1.2.3.zip

kscorectl module sign --key vendor.key dist/vendor_pkg_apt_v1.2.3.zip
→ Signing with cosign...
→ ✓ Signature: dist/vendor_pkg_apt_v1.2.3.zip.sig

# 2. Publish to registry (pushes to OCI + updates SumDB)
kscorectl module publish vendor/pkg_apt@v1.2.3
→ Uploading to OCI registry...
→ ✓ Pushed to registry.kscore.io/vendor/pkg_apt:v1.2.3
→ Recording in transparency log...
→ ✓ SumDB entry: index 45678
→ Published successfully!
```

**Module Installation Flow**:

```bash
# User installs module
kscorectl module install vendor/pkg_apt@v1.2.3

# Resolver workflow:
# 1. Query HTTP proxy for version info
GET /vendor/pkg_apt/@v/v1.2.3.info

# 2. Resolve dependencies (fetch .mod files)
GET /vendor/pkg_apt/@v/v1.2.3.mod
GET /std/files/@v/v1.4.2.mod
GET /std/exec/@v/v1.5.1.mod

# 3. Verify hashes with SumDB
GET /sumdb/lookup/vendor/pkg_apt@v1.2.3
GET /sumdb/lookup/std/files@v1.4.2
GET /sumdb/lookup/std/exec@v1.5.1

# 4. Download ZIPs (if not cached)
GET /vendor/pkg_apt/@v/v1.2.3.zip
GET /std/files/@v/v1.4.2.zip
GET /std/exec/@v/v1.5.1.zip

# 5. Verify signatures (cosign)
# 6. Store in content-addressed cache
# 7. Generate module.lock with pinned versions
```

**Air-Gapped Environment Support**:

```bash
# Export modules for air-gapped environment
kscorectl module mirror --source https://registry.kscore.io \
                        --dest ./module-mirror \
                        vendor/pkg_apt@v1.2.3

# Import in air-gapped environment
kscorectl module mirror --import ./module-mirror \
                        --registry localhost:5000
```

### US9.4b: Module Resolver
**As a** Keystone Core user
**I want to** install modules with automatic dependency resolution
**So that** I don't have to manually manage transitive dependencies

**Acceptance Criteria**:
- Parses `module.yaml` and optional `module.lock`
- Resolves version ranges via registry HTTP proxy
- Constructs dependency DAG (detects cycles)
- Verifies all modules with SumDB and cosign
- Downloads canonical ZIPs into content-addressed storage
- Generates reproducible `module.lock`
- Handles version conflicts (minimum version selection)
- Caches modules locally for offline use

**Resolver Algorithm**:

```
1. Parse module.yaml
   → Extract dependencies with version constraints

2. For each dependency:
   a. Query registry: GET /<module>/@v/list
   b. Filter versions matching constraint
   c. Select version (minimum version selection or latest)
   d. Fetch module.yaml: GET /<module>/@v/<ver>.mod
   e. Recursively resolve transitive dependencies

3. Build dependency DAG
   → Detect circular dependencies (error if found)
   → Identify version conflicts

4. Resolve conflicts:
   → Use minimum version selection (MVS) algorithm
   → Ensure all constraints satisfied

5. Verify all modules:
   a. Query SumDB: GET /sumdb/lookup/<module>@<ver>
   b. Download ZIP: GET /<module>/@v/<ver>.zip
   c. Verify SHA256 hash matches SumDB
   d. Verify cosign signature
   e. Check transparency log proof

6. Store in content-addressed cache:
   → ~/.kscore/modules/<hash>/
   → Enables reproducible builds

7. Generate module.lock:
   → Pin exact versions of all dependencies
   → Include SHA256 hashes
   → Timestamp resolution
```

**Resolver CLI**:

```bash
# Resolve dependencies and generate lock file
kscorectl module resolve
→ Resolving vendor/pkg_apt@v1.2.3...
→ Found dependency: std/files (>=1.0 <2.0)
→   Resolved to: std/files@v1.4.2
→ Found dependency: std/exec (^1.5.0)
→   Resolved to: std/exec@v1.5.1
→ Building dependency graph...
→ ✓ No circular dependencies
→ Verifying with SumDB...
→ ✓ All hashes verified
→ Verifying signatures...
→ ✓ All signatures valid
→ Writing module.lock...
→ ✓ Dependencies resolved successfully

# Install from lock file (reproducible)
kscorectl module install --locked
→ Reading module.lock...
→ Installing 3 modules...
→ ✓ vendor/pkg_apt@v1.2.3 (cached)
→ ✓ std/files@v1.4.2 (cached)
→ ✓ std/exec@v1.5.1 (downloading...)
→ Installation complete

# Update dependencies to latest compatible versions
kscorectl module update
→ Checking for updates...
→ std/files: v1.4.2 → v1.5.0 (compatible)
→ std/exec: v1.5.1 (up to date)
→ Updating module.lock...
→ ✓ Updated

# Show dependency tree
kscorectl module tree
vendor/pkg_apt@v1.2.3
├── std/files@v1.4.2
│   └── std/strings@v1.0.0
└── std/exec@v1.5.1
    └── std/process@v1.2.1
```

**Resolver Implementation** (`pkg/resolver/`):

```go
// Resolver handles module dependency resolution
type Resolver struct {
    registry   RegistryClient
    sumdb      SumDBClient
    cache      ModuleCache
    verifier   SignatureVerifier
}

// Resolve resolves all dependencies for a module
func (r *Resolver) Resolve(manifest *Manifest) (*DependencyGraph, error) {
    graph := NewDependencyGraph()

    // Build DAG
    if err := r.resolveDependencies(manifest, graph); err != nil {
        return nil, err
    }

    // Detect cycles
    if cycle := graph.FindCycle(); cycle != nil {
        return nil, fmt.Errorf("circular dependency: %v", cycle)
    }

    // Resolve version conflicts (MVS)
    if err := r.resolveConflicts(graph); err != nil {
        return nil, err
    }

    return graph, nil
}

// Verify verifies all modules in the graph
func (r *Resolver) Verify(graph *DependencyGraph) error {
    for _, node := range graph.Nodes() {
        // 1. Verify hash with SumDB
        hash, proof, err := r.sumdb.Lookup(node.Module, node.Version)
        if err != nil {
            return fmt.Errorf("sumdb lookup failed: %w", err)
        }
        if err := r.sumdb.VerifyProof(proof); err != nil {
            return fmt.Errorf("invalid merkle proof: %w", err)
        }

        // 2. Download ZIP
        zip, err := r.registry.Download(node.Module, node.Version)
        if err != nil {
            return fmt.Errorf("download failed: %w", err)
        }

        // 3. Verify hash
        actualHash := sha256.Sum256(zip)
        if actualHash != hash {
            return fmt.Errorf("hash mismatch for %s@%s", node.Module, node.Version)
        }

        // 4. Verify signature
        if err := r.verifier.Verify(zip); err != nil {
            return fmt.Errorf("signature verification failed: %w", err)
        }

        // 5. Store in CAS
        if err := r.cache.Store(actualHash, zip); err != nil {
            return fmt.Errorf("cache store failed: %w", err)
        }
    }

    return nil
}

// GenerateLockFile creates a reproducible lock file
func (r *Resolver) GenerateLockFile(graph *DependencyGraph) (*LockFile, error) {
    lock := &LockFile{
        SchemaVersion: 1,
        Modules:       make([]LockedModule, 0, len(graph.Nodes())),
    }

    for _, node := range graph.Nodes() {
        hash, _, _ := r.sumdb.Lookup(node.Module, node.Version)
        lock.Modules = append(lock.Modules, LockedModule{
            Name:     node.Module,
            Version:  node.Version,
            Hash:     hash,
            Resolved: time.Now(),
        })
    }

    // Sort for reproducibility
    sort.Slice(lock.Modules, func(i, j int) bool {
        return lock.Modules[i].Name < lock.Modules[j].Name
    })

    return lock, nil
}
```

### US9.5: Capability-Based Host Interfaces
**As a** platform architect
**I want to** provide minimal, audited host capabilities
**So that** plugins can't escape sandbox

**Acceptance Criteria**:
- Well-defined capability interfaces
- Each capability independently grantable
- Scoped capabilities (e.g., fs.read only to specific paths)
- Rate limiting and resource quotas per capability
- Audit log of all capability invocations
- Policy can deny capabilities by plugin/environment

**Host Capabilities**:

```go
// Capability: fs.read
type FileSystemReadCapability interface {
    // Read file contents
    // Scoped to allowed_paths in manifest
    ReadFile(path string) ([]byte, error)

    // List directory
    // Scoped to allowed_paths
    ListDir(path string) ([]FileInfo, error)

    // Check if file exists
    Exists(path string) (bool, error)
}

// Capability: http.get
type HttpGetCapability interface {
    // Make HTTP GET request
    // Scoped to allowed_domains in manifest
    // Timeout enforced by manifest limit
    Get(url string, timeout Duration) (*Response, error)
}

// Capability: exec
type ExecCapability interface {
    // Execute command
    // Scoped to allowed_commands in manifest
    // Timeout and resource limits enforced
    Run(command string, args []string, timeout Duration) (*ExecResult, error)
}

// Capability: secrets.read
type SecretsReadCapability interface {
    // Read secret value
    // Scoped to allowed_secret_paths
    // All accesses audited
    GetSecret(path string) (string, error)
}

// Capability: log
type LogCapability interface {
    // Structured logging
    // Rate limited per manifest
    Debug(msg string, fields map[string]interface{})
    Info(msg string, fields map[string]interface{})
    Warn(msg string, fields map[string]interface{})
    Error(msg string, fields map[string]interface{})
}

// Capability: time (normally deterministic - no time!)
type TimeCapability interface {
    // Get current time
    // Only granted for specific use cases
    // Breaks determinism!
    Now() (time.Time, error)
}

// Capability: kv (plugin-scoped key-value storage)
type KeyValueCapability interface {
    // Get value
    // Scoped to plugin's namespace only
    Get(key string) (string, error)

    // Set value
    // Size limits enforced
    Set(key string, value string) error
}
```

**Capability Scoping Examples**:
```yaml
# Filesystem capability with path scoping
fs.read:
  allowed_paths:
    - "/var/log/app/**"           # Glob patterns
    - "/etc/app/config.yaml"      # Specific files
  denied_paths:
    - "/var/log/app/secrets.log"  # Explicit denials
  max_file_size: 100MB

# HTTP capability with domain scoping
http.get:
  allowed_domains:
    - "api.example.com"
    - "*.internal.corp"
  denied_domains:
    - "admin.internal.corp"       # Explicit denial
  timeout_max: 30s
  rate_limit: 100/minute

# Exec capability with command allowlist
exec:
  allowed_commands:
    - "/usr/bin/jq"
    - "/usr/bin/grep"
  timeout_max: 10s
  cpu_limit: 100m
  memory_limit: 50MB

# Secrets with path scoping
secrets.read:
  allowed_paths:
    - "app/database/*"
    - "app/api-keys/external-service"
  audit_all: true                 # Every access logged
```

### US9.6: Policy-Controlled Plugin Loading
**As a** security engineer
**I want to** control which plugins can load via policy
**So that** unauthorized plugins can't execute

**Acceptance Criteria**:
- Policy engine (OPA) evaluates plugin manifests
- Deny plugins by signature key, author, capabilities
- Different policies per environment (dev, staging, prod)
- Audit trail of policy decisions
- Policy testing framework

**Plugin Policy Examples**:
```rego
# policies/plugin-authorization.rego
package plugin.authorization

# Deny plugins not signed by trusted keys
deny[msg] {
    not input.manifest.signatures[_].keyid in data.trusted_keys
    msg := sprintf("Plugin not signed by trusted key: %v", [input.manifest.metadata.name])
}

# Deny plugins requesting dangerous capabilities in production
deny[msg] {
    input.environment == "production"
    input.manifest.spec.capabilities.exec
    msg := sprintf("Plugin '%s' requests exec capability in production",
                   [input.manifest.metadata.name])
}

# Deny plugins with excessive resource limits
deny[msg] {
    memory_mb := parse_memory(input.manifest.spec.limits.memory)
    memory_mb > 100
    msg := sprintf("Plugin requests too much memory: %dMB > 100MB", [memory_mb])
}

# Require specific author for privileged capabilities
deny[msg] {
    has_privileged_capability(input.manifest)
    not input.manifest.metadata.author in data.privileged_authors
    msg := "Privileged capabilities require authorized author"
}

# Helper functions
has_privileged_capability(manifest) {
    privileged := ["exec", "secrets.write", "fs.write"]
    manifest.spec.capabilities[cap]
    cap in privileged
}
```

### US9.7: Plugin Use Cases
**As a** user
**I want to** extend Keystone Core for my use cases
**So that** I'm not limited to built-in functionality

**Plugin Types**:

1. **Custom State Modules**
```python
# Plugin: state module for HashiCorp Vault
def vault_secret(path, key):
    """Ensure secret exists in Vault"""
    current = vault.read(path)

    if current.get(key) == None:
        return {"changed": True, "action": "create"}

    return {"changed": False}
```

2. **Custom Execution Handlers**
```rust
// Plugin: execute commands in Firecracker microVMs
pub fn exec_in_firecracker(vm_id: &str, command: &str) -> Result<Output> {
    // Complex Firecracker API interaction
    // Compiled to WASM for performance
}
```

3. **Custom Policy Rules**
```python
# Plugin: custom compliance rule
def check_pci_dss_requirement_8_2_3(resource):
    """Check password complexity requirements"""
    if resource.type != "user":
        return {"compliant": True}

    password_policy = resource.password_policy
    return {
        "compliant": (
            password_policy.min_length >= 12 and
            password_policy.require_special_chars and
            password_policy.require_numbers
        ),
        "details": password_policy
    }
```

4. **Custom Reactors**
```python
# Plugin: custom incident response
def handle_security_incident(event):
    """Automated incident response"""
    if event.severity == "critical":
        # Gather diagnostics
        logs = fs.read("/var/log/app/error.log")

        # Notify team
        http.post("https://oncall.example.com/incidents", {
            "severity": "critical",
            "logs": logs,
            "event": event
        })

        # Isolate affected systems
        return {"action": "isolate", "target": event.agent_id}
```

5. **Custom Verification Steps (GitOps)**
```python
# Plugin: custom deployment verification
def verify_canary_deployment(app_name, canary_version):
    """Verify canary deployment health"""
    # Get metrics from Prometheus
    error_rate = prometheus.query(
        f'rate(http_errors{{app="{app_name}",version="{canary_version}"}}[5m])'
    )

    latency_p95 = prometheus.query(
        f'histogram_quantile(0.95, http_latency{{app="{app_name}"}})'
    )

    # Deterministic decision logic
    if error_rate > 0.01:  # >1% errors
        return {"approved": False, "reason": "High error rate"}

    if latency_p95 > 200:  # >200ms p95
        return {"approved": False, "reason": "High latency"}

    return {"approved": True}
```

## Technical Tasks

### Phase 1: CLI Infrastructure & Plugin Runtime Foundation (Week 1-2)

**T1.0: kscorectl Plugin Dispatcher**
- Implement main `kscorectl` binary (lightweight dispatcher)
- Plugin discovery: search $PATH for `kscore-*` binaries
- Execute plugin with remaining arguments: `kscorectl module install` → `kscore-module install`
- Handle plugin not found errors with helpful messages
- Support `--help` that shows both built-in and discovered plugin commands
- Support `kscorectl plugin list` to show all available plugins
- Version compatibility checking (optional)
- Similar implementation to `kubectl` plugin system or `git` command dispatch

**T1.1: Starlark Runtime**
- Integrate Starlark Go library
- Configure sandbox settings
- Implement deterministic mode (disable random, time)
- Create plugin loader for .star files
- Add hot-reload capability

**T1.2: WASM Runtime**
- Integrate wasmtime (WASM runtime)
- Configure WASI interface
- Implement capability injection via WASI imports
- Add instruction metering (prevent infinite loops)
- Memory isolation and limits

**T1.3: Plugin Manifest Parser**
- Define manifest schema (YAML)
- Parse and validate manifests
- Extract capability requirements
- Extract resource limits
- Validate dependencies

### Phase 2: Capability System (Week 3-4)

**T2.1: Capability Interface Design**
- Define Go interfaces for each capability
- Implement capability registry
- Create capability granting mechanism
- Add scoping logic (paths, domains, etc.)
- Implement rate limiting per capability

**T2.2: Host Capabilities Implementation**
- fs.read / fs.write capabilities
- http.get / http.post capabilities
- exec capability
- secrets.read / secrets.write capabilities
- log capability
- time capability (with warnings)
- kv (key-value store) capability

**T2.3: Capability Auditing**
- Log all capability invocations
- Include plugin name, capability, parameters
- Structured audit log format
- Integration with Keystone Core audit system

### Phase 3: Cryptographic Verification (Week 5-6)

**T3.1: Cosign Integration**
- Integrate cosign library
- Verify signatures on plugin files
- Verify signatures on manifests
- Support multiple signing keys
- Key rotation support

**T3.2: Transparency Log**
- Implement append-only log (Merkle tree)
- REST API for transparency log
- Client verification of entries
- Integration with existing SumDB if possible
- Signed tree heads

**T3.3: Hybrid Module Registry (OCI + HTTP Proxy)**
- **OCI Registry Integration**:
  - Configure OCI registry backend (Harbor, Docker Registry, or cloud provider)
  - Store modules as OCI artifacts (ZIP in blob layer)
  - Attach signatures as cosign layers
  - Store SBOM and provenance attestations
- **HTTP Proxy Implementation**:
  - Go-mod-style endpoints (/@v/list, /@v/<ver>.info, .mod, .zip)
  - Caching layer (Redis/memcached for metadata, local disk for ZIPs)
  - Version listing with SemVer sorting
  - Bandwidth optimization (compression, range requests)
- **SumDB Integration**:
  - /sumdb/lookup endpoint for hash verification
  - Merkle proof generation and verification
  - Signed tree head updates
- **Registry CLI**:
  - Upload modules to OCI registry
  - Update SumDB with new entries
  - Air-gapped mirroring support

### Phase 4: Module Resolver & Dependency Management (Week 7)

**T4.1: Module Manifest Parser**
- Parse `module.yaml` (schema v1)
- Validate module metadata and dependencies
- Parse version constraints (SemVer ranges)
- Parse capability declarations
- Parse resource limits
- Validate manifest schema

**T4.2: Dependency Resolution Engine**
- **Version Constraint Solver**:
  - Implement SemVer parsing and matching
  - Support version ranges (>=, <, ^, ~)
  - Handle pre-release and build metadata
- **Dependency Graph Builder**:
  - Recursive dependency fetching
  - DAG construction
  - Cycle detection algorithm
  - Topological sorting for install order
- **Conflict Resolution (MVS)**:
  - Minimum Version Selection algorithm
  - Constraint satisfaction
  - Backtracking on conflicts

**T4.3: Module Resolver Implementation**
- **Registry Client**:
  - HTTP client for Go-mod-style endpoints
  - Version list fetching (/@v/list)
  - Metadata fetching (/@v/<ver>.info, .mod)
  - ZIP download with progress tracking
  - Retry logic and timeout handling
- **SumDB Client**:
  - Hash lookup (/sumdb/lookup)
  - Merkle proof verification
  - Tree head signature verification
  - Detect registry tampering
- **Content-Addressed Storage**:
  - Local cache (~/.kscore/modules)
  - Store by SHA256 hash
  - Deduplication
  - Cache pruning (LRU or size-based)
- **Lock File Generation**:
  - Generate reproducible module.lock
  - Pin exact versions and hashes
  - Timestamp resolution
  - Deterministic ordering

**T4.4: Resolver CLI Commands (via kscorectl → kscore-module)**
- `kscorectl module init` - Initialize module.yaml
- `kscorectl module resolve` - Resolve dependencies, generate lock
- `kscorectl module install` - Install from lock file
- `kscorectl module update` - Update to latest compatible versions
- `kscorectl module tree` - Display dependency tree
- `kscorectl module verify` - Verify all dependencies
- `kscorectl module clean` - Clean local cache
- `kscorectl module mirror` - Mirror for air-gapped environments

**Note**: `kscorectl` dispatches to `kscore-module` binary using Git-style plugin pattern

### Phase 5: Policy Integration (Week 8)

**T5.1: Plugin Policy Engine**
- Define policy schema for plugins
- Integration with OPA (Epic 6)
- Evaluate manifests against policy
- Policy decision logging
- Policy testing framework

**T5.2: Environment-Specific Policies**
- Dev environment policies (permissive)
- Staging environment policies (moderate)
- Production environment policies (strict)
- Policy inheritance and overrides

### Phase 6: Plugin SDK & Developer Experience (Week 9)

**T6.1: Plugin SDK (Multiple Languages)**
- Starlark SDK (built-in functions, helpers)
- Rust SDK for WASM (kscore-plugin-sdk crate)
- Go SDK for WASM (if needed)
- Type definitions and documentation

**T6.2: Developer Tooling (via kscorectl → kscore-module)**
- `kscorectl module init` - scaffold new module
- `kscorectl module build` - build module ZIP
- `kscorectl module test` - test module locally
- `kscorectl module sign` - sign module with cosign
- `kscorectl module publish` - upload to registry
- `kscorectl module validate` - validate module.yaml
- Integration with resolver commands (resolve, install, etc.)
- Plugin discovery mechanism in `kscorectl` (searches $PATH for kscore-*)

**T6.3: Module Examples & Templates**
- Example Starlark modules
- Example Rust WASM modules
- State module template
- Policy rule template
- Reactor template
- Verification step template
- Standard library modules (std/files, std/exec, std/strings)

### Phase 7: Runtime & Performance (Week 10)

**T7.1: Resource Limits Enforcement**
- CPU limits (cgroups integration)
- Memory limits (WASM memory, Starlark heap)
- Execution timeout
- Instruction counting (WASM)
- Storage quota (kv capability)

**T7.2: Performance Optimization**
- Module caching and preloading
- WASM compilation caching
- Starlark bytecode caching
- Parallel module execution
- Benchmarking suite
- Dependency graph optimization

**T7.3: Error Handling & Recovery**
- Module crash isolation
- Graceful degradation
- Error reporting to users
- Module blacklisting on repeated failures
- Dependency resolution error messages

## Dependencies

- **Epic 3**: State Management (for state module plugins)
- **Epic 4**: Event System (for reactor plugins)
- **Epic 5**: GitOps Integration (for verification plugins)
- **Epic 6**: Policy Enforcement (for policy plugins and plugin authorization)
- **Go Libraries**:
  - `go.starlark.net` - Starlark runtime
  - `github.com/bytecodealliance/wasmtime-go` - WASM runtime
  - `github.com/sigstore/cosign` - Signature verification
  - `github.com/transparency-dev/merkle` - Merkle tree for transparency log
  - `github.com/Masterminds/semver/v3` - SemVer parsing and constraint solving
  - `github.com/google/go-containerregistry` - OCI registry client
  - `gopkg.in/yaml.v3` - YAML parsing for module.yaml
  - `github.com/spf13/afero` - Filesystem abstraction for testing

## Risks & Mitigations

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Module escape from sandbox | Critical | Low | Defense in depth: runtime sandbox + capability limits + policy |
| Performance overhead | Medium | Medium | WASM for perf-critical, caching, benchmarking |
| Malicious module signed by compromised key | Critical | Low | Transparency log detects different versions, key rotation |
| Module compatibility breaking changes | High | Medium | Semantic versioning, deprecation policy, SDK versioning |
| Determinism violations | High | Medium | Runtime enforcement, API design, extensive testing |
| Dependency resolution conflicts | Medium | Medium | MVS algorithm, clear error messages, manual overrides |
| Registry availability (SPoF) | High | Medium | Registry mirroring, local caching, air-gapped support |
| Transitive dependency vulnerabilities | High | Medium | SBOM generation, vulnerability scanning, dependency pinning |
| Module namespace conflicts | Medium | Low | Namespaced modules (vendor/name), registry validation |
| Lock file drift | Medium | Medium | Lock file verification, CI/CD integration, automated updates |

## Metrics & Monitoring

### Key Metrics
- Module execution latency (p50, p95, p99)
- Module load time
- Capability invocation rate per module
- Module failure rate
- Signature verification time
- Transparency log query latency
- Dependency resolution time
- Registry response time (HTTP proxy)
- Cache hit rate (modules, manifests)
- Module download bandwidth

### Alerts
- Module signature verification failures
- Module resource limit violations
- Capability authorization denials
- Module crash rate >1%
- Transparency log unavailable
- Dependency resolution failures >5%
- Registry HTTP proxy errors
- SumDB hash mismatches (critical!)
- Circular dependency detected

## Testing Strategy

### Unit Tests
- Starlark runtime sandboxing
- WASM runtime isolation
- Capability scoping logic
- Signature verification
- Manifest parsing (module.yaml, module.lock)
- SemVer constraint parsing and matching
- Dependency graph construction
- DAG cycle detection
- MVS conflict resolution
- Content-addressed storage

### Integration Tests
- End-to-end module installation with dependencies
- Policy-controlled loading
- Transparency log verification
- Multi-module scenarios
- Module hot-reload
- Registry HTTP proxy endpoints
- Resolver with real registry
- Lock file reproducibility
- Air-gapped mirroring

### Security Tests
- Sandbox escape attempts
- Capability bypass attempts
- Resource exhaustion attacks
- Malicious module detection
- Signature tampering detection
- SumDB hash mismatch detection
- Transitive dependency vulnerabilities
- Namespace conflict handling

### Performance Tests
- Dependency resolution with 100+ modules
- Concurrent module execution
- Cache performance (hit rate >90%)
- Registry bandwidth optimization
- Module load time (<100ms)
- Resolver performance (<5s for 100 modules)

## Documentation Requirements

- [ ] **Module Development**:
  - [ ] Module development guide (Starlark)
  - [ ] Module development guide (Rust WASM)
  - [ ] Module development guide (Go WASM)
  - [ ] Module manifest reference (module.yaml)
  - [ ] Dependency management guide
  - [ ] Standard library API reference
- [ ] **Registry & Distribution**:
  - [ ] Registry setup guide (OCI + HTTP proxy)
  - [ ] Module publishing guide
  - [ ] Module signing guide (cosign)
  - [ ] Air-gapped deployment guide
  - [ ] SumDB transparency log documentation
- [ ] **Security**:
  - [ ] Capability reference documentation
  - [ ] Security model documentation
  - [ ] Policy examples for modules
  - [ ] Threat model and security guarantees
- [ ] **Operations**:
  - [ ] Module CLI reference (all commands)
  - [ ] Troubleshooting guide
  - [ ] Performance tuning guide
  - [ ] Migration guide (upgrading modules)
- [ ] **Examples**:
  - [ ] Example modules repository
  - [ ] Module templates (state, policy, reactor, verification)
  - [ ] Standard library modules documentation

## Definition of Done

- [ ] All user stories implemented
- [ ] **Module Format**:
  - [ ] module.yaml and module.lock implemented
  - [ ] Structured directory format supported
- [ ] **Dependency Management**:
  - [ ] SemVer constraint resolution working
  - [ ] Transitive dependencies resolved
  - [ ] Circular dependency detection working
  - [ ] MVS conflict resolution implemented
- [ ] **Registry & Distribution**:
  - [ ] OCI registry integration complete
  - [ ] HTTP proxy endpoints working
  - [ ] SumDB transparency log operational
  - [ ] Air-gapped mirroring working
- [ ] **Runtimes**:
  - [ ] Starlark runtime working with sandboxing
  - [ ] WASM runtime working with WASI
  - [ ] Deterministic execution enforced
- [ ] **Security**:
  - [ ] Capability system functional
  - [ ] Cosign signature verification operational
  - [ ] Policy integration complete
  - [ ] Security audit passed
- [ ] **Developer Experience**:
  - [ ] Module CLI functional (all commands)
  - [ ] Module SDK available (Rust, Starlark)
  - [ ] Example modules available
  - [ ] Standard library modules complete
- [ ] **Performance**:
  - [ ] Module execution overhead <10ms
  - [ ] Dependency resolution <5s for 100 modules
  - [ ] Cache hit rate >90%
- [ ] **Documentation**:
  - [ ] All documentation complete
  - [ ] Module development guides published
  - [ ] Registry setup guide published
- [ ] **Production Readiness**:
  - [ ] All tests passing (unit, integration, security, performance)
  - [ ] Production deployment tested
  - [ ] Monitoring and alerting configured
