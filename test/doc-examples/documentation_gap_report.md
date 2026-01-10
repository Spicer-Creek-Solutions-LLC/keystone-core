# Documentation Gap Analysis Report

## Overview

This report identifies gaps between implemented features in the codebase and their documentation coverage. The analysis compares packages in `pkg/`, CLIs in `cmd/`, and the corresponding documentation in `docs/content/en/docs/`.

## Package to Documentation Mapping

### Core Packages - Well Documented ✅

| Package | Documentation | Status |
|---------|--------------|--------|
| `pkg/agent` | `concepts/agents.md` | ✅ Documented |
| `pkg/config` | `reference/configuration.md` | ✅ Documented |
| `pkg/controlplane` | `concepts/control-plane.md` | ✅ Documented |
| `pkg/events` | `concepts/events.md`, `concepts/reactors.md` | ✅ Documented |
| `pkg/execution` | `concepts/remote-execution.md` | ✅ Documented |
| `pkg/gitops` | `concepts/gitops.md` | ✅ Documented |
| `pkg/identity` | `concepts/identity.md` | ✅ Documented |
| `pkg/nats` | `concepts/message-bus.md`, `concepts/nats-mesh.md` | ✅ Documented |
| `pkg/policy` | `concepts/policy.md` | ✅ Documented |
| `pkg/statemgmt` | `concepts/state-management.md`, `reference/modules.md` | ✅ Documented |
| `pkg/gateway` | `operations/gateway.md` | ✅ Documented |
| `pkg/cluster` | N/A | ⚠️ Partially documented in operations |

### Epic 21: Proxy Agents - Documented ✅

| Package | Documentation | Status |
|---------|--------------|--------|
| `pkg/proxy` | `concepts/proxy-agents.md` | ✅ Documented |
| `pkg/protocols/ssh` | `concepts/proxy-agents.md` | ✅ Documented |
| `pkg/protocols/snmp` | `concepts/proxy-agents.md` | ✅ Documented |
| `pkg/protocols/rest` | `concepts/proxy-agents.md` | ✅ Documented |
| `pkg/protocols/winrm` | `concepts/proxy-agents.md` | ✅ Documented |
| `pkg/vendors` | `concepts/proxy-agents.md` | ✅ Documented |
| `pkg/credentials` | `concepts/proxy-agents.md` | ✅ Documented |

### Epic 22: File Distribution - Documented ✅

| Package | Documentation | Status |
|---------|--------------|--------|
| `pkg/files` | `concepts/file-distribution.md`, `reference/file-backends.md` | ✅ Documented |

### Epic 23: Self-Management - Documented ✅

| Package | Documentation | Status |
|---------|--------------|--------|
| `pkg/backup` | `concepts/self-management.md` | ✅ Documented |
| `pkg/bootstrap` | `concepts/self-management.md` | ✅ Documented |
| `pkg/selfmgmt` | `concepts/self-management.md` | ✅ Documented |
| `pkg/upgrade` | `concepts/self-management.md` | ✅ Documented |

### Infrastructure Packages - Varying Coverage

| Package | Documentation | Status |
|---------|--------------|--------|
| `pkg/audit` | `concepts/observability.md` (brief mention) | ⚠️ Needs expansion |
| `pkg/cloud` | `reference/configuration.md` (cloud detection) | ⚠️ Minimal |
| `pkg/container` | Not documented separately | ⚠️ Needs docs |
| `pkg/edge` | `concepts/agents.md` (edge mode) | ⚠️ Brief mention |
| `pkg/hardware` | Not documented | ❌ Missing |
| `pkg/health` | `operations/monitoring.md` (health endpoints) | ⚠️ Minimal |
| `pkg/k8s` | `reference/configuration.md` (k8s integration) | ⚠️ Brief |
| `pkg/logging` | `concepts/observability.md` | ⚠️ Minimal |
| `pkg/metrics` | `reference/metrics.md` | ✅ Documented |
| `pkg/module` | `reference/registry.md`, module system | ✅ Documented |
| `pkg/netutil` | Not documented (internal) | N/A Internal |
| `pkg/platform` | Not documented (internal) | N/A Internal |
| `pkg/plugin` | Not documented separately | ⚠️ Brief in CLI |
| `pkg/profiling` | Not documented | ❌ Missing |
| `pkg/query` | Not documented | ❌ Missing |
| `pkg/security` | `operations/security.md` | ✅ Documented |
| `pkg/servicemesh` | Not documented | ❌ Missing |
| `pkg/state` | `operations/maintenance.md` (database) | ⚠️ Minimal |
| `pkg/targeting` | `concepts/remote-execution.md` | ✅ Documented |
| `pkg/tracing` | `concepts/observability.md` | ⚠️ Brief |
| `pkg/visualization` | Not documented | ❌ Missing |

## CLI Documentation Gaps

### Verified CLIs with Complete Documentation ✅

- `kscorectl` - Main CLI dispatcher
- `kscore-exec` - Remote execution
- `kscore-state` - State management
- `kscore-identity` - Identity management
- `kscore-monitor` - TUI monitoring

### CLIs Needing Documentation Updates

| CLI | Issue | Priority |
|-----|-------|----------|
| `kscore-exec` | `version` subcommand undocumented | Low |
| `kscore-module` | Full command reference needed | Medium |
| `kscore-policy` | Full command reference needed | Medium |
| `kscore-gitops` | Full command reference needed | Medium |
| `kscore-cluster` | Full command reference needed | Medium |
| `kscore-bootstrap` | Not documented | High |
| `kscore-files` | Not documented as standalone | Medium |

## Configuration Documentation Gaps

### Issues Found in T5.2

1. **Logging Configuration** (Fixed)
   - Documentation mentioned file output which isn't supported
   - Updated to show stdout and syslog options

### Remaining Issues

| Section | Issue | Status |
|---------|-------|--------|
| NATS listen format | Uses HCL-style in code vs YAML in docs | ⚠️ Minor |
| Agent IPv6 | Missing AddressFamily documentation | ⚠️ Minor |
| Storage sqlite | max_connections not actually implemented | ⚠️ Minor |

## Recommended Additions

### High Priority

1. **Cluster Operations Guide**
   - `docs/content/en/docs/operations/cluster-management.md`
   - Document HA setup, failover, recovery
   - Leader election and work distribution

2. **Hardware Detection Documentation**
   - Add section to multi-environment or new page
   - Cover CPU, memory, disk, BMC/IPMI detection

3. **Service Mesh Integration**
   - Add to multi-environment documentation
   - Cover Istio, Linkerd, Consul integration

### Medium Priority

4. **Advanced Profiling**
   - Document pprof endpoints
   - Performance tuning guide

5. **Query API Reference**
   - Document Prometheus, Loki, Jaeger queries
   - Unified query interface

6. **Visualization API**
   - Document topology visualization
   - WebSocket real-time updates

### Low Priority

7. **Internal packages** (container, platform, netutil)
   - Only needed for contributors
   - Could be in development guide

## Documentation Quality Issues

### Code Example Validation (T5.1)

- **Total code blocks**: 2,103
- **Valid blocks**: 1,556
- **Skipped blocks**: 505 (output examples, diagrams)
- **Invalid blocks fixed**: 42 → 0

### Fixed Issues

1. NATS config blocks using `yaml` instead of `conf` (7 instances)
2. Intentional bad YAML example marked as `text`
3. Logging section updated for syslog support

## Summary

| Category | Complete | Partial | Missing |
|----------|----------|---------|---------|
| Core Concepts | 10 | 2 | 0 |
| Reference Docs | 7 | 3 | 0 |
| Operations Docs | 6 | 1 | 1 |
| CLI Documentation | 5 | 6 | 2 |
| **Total** | 28 | 12 | 3 |

**Documentation Coverage**: ~65% complete, ~28% partial, ~7% missing

## Next Steps

1. Complete CLI reference for all plugins (Medium priority)
2. Add cluster management operations guide (High priority)
3. Document hardware detection features (High priority)
4. Add service mesh integration guide (Medium priority)
5. Document advanced profiling and query APIs (Low priority)

## Generated

Date: 2026-01-10
Epic: 24 - Document Review
Phase: 6 - Gap Analysis & Remediation
