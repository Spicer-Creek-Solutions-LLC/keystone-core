# Keystone Core Documentation Inventory

## Summary

| Metric | Value |
|--------|-------|
| Total Documentation Files | 61 |
| Docs with Examples | 53 (86.9%) |
| Total Packages | 45 |
| Average Godoc Coverage | 89.0% |
| Packages Fully Documented | 24 |
| Packages No Documentation | 4 |
| Total Exported Symbols | 5109 |
| Documented Symbols | 4932 |

## Package Godoc Coverage

| Package | Exported Types | Documented Types | Exported Funcs | Documented Funcs | Coverage |
|---------|---------------|------------------|----------------|------------------|----------|
| agent | 43 | 43 | 123 | 123 | ✅ 100.0% |
| api | 0 | 0 | 0 | 0 | ❌ 0.0% |
| audit | 22 | 22 | 56 | 54 | ✅ 97.4% |
| backup | 76 | 76 | 195 | 191 | ✅ 98.5% |
| bootstrap | 43 | 43 | 61 | 55 | ✅ 94.2% |
| cloud | 12 | 12 | 24 | 22 | ✅ 94.4% |
| cluster | 102 | 102 | 305 | 305 | ✅ 100.0% |
| config | 23 | 23 | 7 | 7 | ✅ 100.0% |
| container | 13 | 13 | 19 | 18 | ✅ 96.9% |
| controlplane | 15 | 15 | 52 | 50 | ✅ 97.0% |
| credentials | 45 | 45 | 105 | 105 | ✅ 100.0% |
| edge | 9 | 9 | 23 | 22 | ✅ 96.9% |
| events | 87 | 87 | 262 | 205 | ✅ 83.7% |
| execution | 23 | 23 | 57 | 37 | ⚠️ 75.0% |
| files | 41 | 41 | 24 | 24 | ✅ 100.0% |
| gateway | 34 | 34 | 11 | 11 | ✅ 100.0% |
| gitops | 0 | 0 | 0 | 0 | ❌ 0.0% |
| hardware | 9 | 9 | 15 | 15 | ✅ 100.0% |
| health | 20 | 20 | 49 | 49 | ✅ 100.0% |
| identity | 109 | 109 | 196 | 196 | ✅ 100.0% |
| k8s | 57 | 57 | 73 | 73 | ✅ 100.0% |
| logging | 32 | 32 | 87 | 79 | ✅ 93.3% |
| metrics | 20 | 20 | 78 | 78 | ✅ 100.0% |
| module | 0 | 0 | 0 | 0 | ❌ 0.0% |
| nats | 197 | 197 | 625 | 566 | ✅ 92.8% |
| netutil | 6 | 6 | 38 | 38 | ✅ 100.0% |
| platform | 8 | 8 | 34 | 34 | ✅ 100.0% |
| plugin | 4 | 4 | 8 | 8 | ✅ 100.0% |
| policy | 44 | 44 | 55 | 55 | ✅ 100.0% |
| profiling | 5 | 5 | 6 | 6 | ✅ 100.0% |
| proto | 0 | 0 | 0 | 0 | ❌ 0.0% |
| protocols | 21 | 21 | 29 | 29 | ✅ 100.0% |
| proxy | 41 | 41 | 97 | 97 | ✅ 100.0% |
| query | 28 | 28 | 27 | 27 | ✅ 100.0% |
| security | 1 | 1 | 7 | 7 | ✅ 100.0% |
| selfmgmt | 28 | 28 | 88 | 82 | ✅ 94.8% |
| servicemesh | 13 | 13 | 23 | 22 | ✅ 97.2% |
| state | 19 | 19 | 49 | 49 | ✅ 100.0% |
| statemgmt | 174 | 174 | 475 | 474 | ✅ 99.8% |
| targeting | 9 | 9 | 15 | 15 | ✅ 100.0% |
| tracing | 18 | 18 | 88 | 86 | ✅ 98.1% |
| upgrade | 54 | 54 | 61 | 57 | ✅ 96.5% |
| vendors | 18 | 18 | 19 | 18 | ✅ 97.3% |
| version | 1 | 1 | 2 | 2 | ✅ 100.0% |
| visualization | 12 | 12 | 5 | 5 | ✅ 100.0% |

## Documentation by Category

### _index.Md (1 files)

| File | Lines | Has Examples |
|------|-------|-------------|
| _index.md | 77 | No |

### Community (6 files)

| File | Lines | Has Examples |
|------|-------|-------------|
| community/_index.md | 55 | No |
| community/contributing.md | 429 | Yes |
| community/development.md | 963 | Yes |
| community/roadmap.md | 750 | Yes |
| community/support.md | 432 | Yes |
| community/windows-development.md | 365 | Yes |

### Concepts (15 files)

| File | Lines | Has Examples |
|------|-------|-------------|
| concepts/_index.md | 74 | No |
| concepts/agents.md | 785 | Yes |
| concepts/control-plane.md | 637 | Yes |
| concepts/events.md | 687 | Yes |
| concepts/file-distribution.md | 442 | Yes |
| concepts/gitops.md | 880 | Yes |
| concepts/identity.md | 1105 | Yes |
| concepts/message-bus.md | 826 | Yes |
| concepts/modules.md | 601 | Yes |
| concepts/nats-mesh.md | 714 | Yes |
| concepts/observability.md | 1362 | Yes |
| concepts/policy.md | 1186 | Yes |
| concepts/reactors.md | 714 | Yes |
| concepts/remote-execution.md | 882 | Yes |
| concepts/state-management.md | 705 | Yes |

### Executive-Summary (1 files)

| File | Lines | Has Examples |
|------|-------|-------------|
| executive-summary/_index.md | 108 | No |

### Getting-Started (5 files)

| File | Lines | Has Examples |
|------|-------|-------------|
| getting-started/_index.md | 74 | No |
| getting-started/architecture.md | 492 | Yes |
| getting-started/installation.md | 445 | Yes |
| getting-started/overview.md | 245 | Yes |
| getting-started/quick-start.md | 353 | Yes |

### Operations (14 files)

| File | Lines | Has Examples |
|------|-------|-------------|
| operations/_index.md | 194 | No |
| operations/deployment.md | 1190 | Yes |
| operations/gateway.md | 262 | Yes |
| operations/ipv6.md | 613 | Yes |
| operations/maintenance.md | 1147 | Yes |
| operations/monitoring.md | 911 | Yes |
| operations/nats-mesh-deployment.md | 821 | Yes |
| operations/nats-mesh-operations.md | 663 | Yes |
| operations/registry.md | 782 | Yes |
| operations/security.md | 1157 | Yes |
| operations/self-management.md | 893 | Yes |
| operations/troubleshooting.md | 966 | Yes |
| operations/windows-installation.md | 465 | Yes |
| operations/windows.md | 352 | Yes |

### Reference (10 files)

| File | Lines | Has Examples |
|------|-------|-------------|
| reference/_index.md | 37 | No |
| reference/api.md | 1310 | Yes |
| reference/cli.md | 3175 | Yes |
| reference/configuration.md | 723 | Yes |
| reference/events.md | 856 | Yes |
| reference/file-backends.md | 318 | Yes |
| reference/ipv6.md | 487 | Yes |
| reference/metrics.md | 890 | Yes |
| reference/modules.md | 8802 | Yes |
| reference/nats-mesh.md | 522 | Yes |

### Runbooks (9 files)

| File | Lines | Has Examples |
|------|-------|-------------|
| runbooks/README.md | 50 | No |
| runbooks/backup-restore.md | 259 | Yes |
| runbooks/bootstrap-new-cluster.md | 194 | Yes |
| runbooks/certificate-rotation.md | 244 | Yes |
| runbooks/disaster-recovery.md | 330 | Yes |
| runbooks/emergency-rollback.md | 219 | Yes |
| runbooks/scheduled-maintenance.md | 235 | Yes |
| runbooks/troubleshooting.md | 524 | Yes |
| runbooks/upgrade-cluster.md | 317 | Yes |

