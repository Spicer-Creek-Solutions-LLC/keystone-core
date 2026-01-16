---
title: "Threat Model"
weight: 15
description: >
  Security threat model covering trust boundaries, data flows, attack surfaces, and mitigations
---

## Overview

This document describes the security threat model for Keystone Core, identifying trust boundaries, data flows, potential threats, and implemented mitigations. It follows the STRIDE methodology (Spoofing, Tampering, Repudiation, Information Disclosure, Denial of Service, Elevation of Privilege).

## Threat Actors

| Actor | Description | Motivation | Capability |
|-------|-------------|------------|------------|
| **External Attacker** | Remote attacker without initial access | Data theft, disruption, ransomware | Network access, exploitation tools |
| **Compromised Agent** | Attacker who has compromised a managed node | Lateral movement, data exfiltration | Agent credentials, local system access |
| **Malicious Insider** | Employee or contractor with legitimate access | Data theft, sabotage | Valid credentials, internal knowledge |
| **Supply Chain Attacker** | Attacker who compromises dependencies | Backdoor installation, data theft | Build system access, package manipulation |

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            EXTERNAL NETWORK                                 │
│                          (Untrusted Zone)                                   │
└───────────────────────────────┬─────────────────────────────────────────────┘
                                │
                    [Firewall / Load Balancer]
                                │
┌───────────────────────────────┴─────────────────────────────────────────────┐
│                           CONTROL PLANE ZONE                                │
│                          (Trusted Zone)                                     │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                     Control Plane Server                            │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────────┐  │   │
│  │  │   API       │  │   State     │  │   Event / Reactor           │  │   │
│  │  │   Server    │  │   Manager   │  │   Engine                    │  │   │
│  │  └─────────────┘  └─────────────┘  └─────────────────────────────┘  │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────────┐  │   │
│  │  │   Policy    │  │  Connection │  │   Identity                  │  │   │
│  │  │   Engine    │  │   Manager   │  │   Provider                  │  │   │
│  │  └─────────────┘  └─────────────┘  └─────────────────────────────┘  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                │                                            │
│  ┌─────────────────────────────┴─────────────────────────────────────────┐ │
│  │                         NATS Message Bus                              │ │
│  │              (mTLS required, JetStream persistence)                   │ │
│  └─────────────────────────────┬─────────────────────────────────────────┘ │
│                                │                                            │
│  ┌─────────────────────────────┴─────────────────────────────────────────┐ │
│  │                       State Database                                  │ │
│  │              (SQLite/PostgreSQL, encrypted at rest)                   │ │
│  └───────────────────────────────────────────────────────────────────────┘ │
└───────────────────────────────┬─────────────────────────────────────────────┘
                                │
                    [mTLS / SPIFFE Authentication]
                                │
┌───────────────────────────────┴─────────────────────────────────────────────┐
│                             AGENT ZONE                                      │
│                    (Semi-Trusted Zone)                                      │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐                   │
│  │   Agent 1     │  │   Agent 2     │  │   Agent N     │                   │
│  │  (K8s Node)   │  │  (VM)         │  │  (Bare Metal) │                   │
│  └───────────────┘  └───────────────┘  └───────────────┘                   │
│                                                                             │
│  ┌───────────────┐  ┌───────────────┐                                      │
│  │ Proxy Agent   │  │ Edge Agent    │                                      │
│  │ (SSH/SNMP)    │  │ (Offline)     │                                      │
│  └───────────────┘  └───────────────┘                                      │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Trust Boundaries

### TB1: External Network → Control Plane API

**Boundary**: Network perimeter protecting the control plane API.

**Data Crossing**: API requests (gRPC/REST), webhooks (GitOps, GitHub/GitLab).

**Security Controls**:
- TLS 1.3 for all connections
- API key or JWT authentication required
- Rate limiting and request throttling
- Input validation on all endpoints
- Webhook signature verification (HMAC-SHA256)

### TB2: Control Plane → NATS Message Bus

**Boundary**: Internal communication channel between control plane components.

**Data Crossing**: Commands, events, state updates, heartbeats.

**Security Controls**:
- mTLS with client certificate validation
- NATS authorization rules (publish/subscribe permissions)
- JetStream persistence with authenticated access
- Message integrity via TLS

### TB3: Control Plane → Agent

**Boundary**: Communication between control plane and managed agents.

**Data Crossing**: Commands, execution results, state applications, heartbeats.

**Security Controls**:
- mTLS with per-agent certificates (Manual mode) or SVIDs (SPIFFE mode)
- Agent identity attestation (join token, cloud metadata, K8s SAT)
- Command authorization via policy engine
- Agent targeting validation

### TB4: Control Plane → Database

**Boundary**: Access to persistent state storage.

**Data Crossing**: Agent state, job history, configuration data, audit logs.

**Security Controls**:
- TLS for PostgreSQL connections
- Database authentication (username/password or certificate)
- Encryption at rest (database-level or filesystem)
- Backup encryption with age/KMS

### TB5: Agent → Managed System

**Boundary**: Agent executing commands on the local system.

**Data Crossing**: File operations, process execution, system configuration.

**Security Controls**:
- Least-privilege agent user (non-root when possible)
- Command allowlists for restricted operations
- State module validation
- Audit logging of all operations

### TB6: Proxy Agent → Unmanaged Devices

**Boundary**: Proxy agent communicating with devices that cannot run native agents.

**Data Crossing**: SSH commands, SNMP queries, REST API calls, WinRM commands.

**Security Controls**:
- Credential encryption at rest (AES-256-GCM)
- Credential rotation policies
- Protocol-specific security (SSH keys, SNMPv3 auth)
- Device authentication validation

## Data Flows

### DF1: Agent Registration

```
Agent → [mTLS] → NATS → Control Plane → Database
```

**Data**: Agent ID, metadata (OS, hostname, labels), attestation evidence.

**Sensitivity**: Medium - Contains infrastructure metadata.

**Protection**: mTLS encryption, attestation validation, stored hashed.

### DF2: Command Execution

```
User → [TLS+Auth] → API → Control Plane → [mTLS] → NATS → Agent
```

**Data**: Command text, arguments, environment variables, working directory.

**Sensitivity**: High - May contain credentials or sensitive data.

**Protection**: TLS encryption, authorization checks, audit logging, secrets redacted in logs.

### DF3: State Application

```
Control Plane → [mTLS] → NATS → Agent → Local System
```

**Data**: State declarations, file contents, user/group configurations.

**Sensitivity**: High - Contains system configuration details.

**Protection**: TLS encryption, state validation, idempotent operations.

### DF4: Heartbeat

```
Agent → [mTLS] → NATS → Control Plane → Database
```

**Data**: Agent status, resource metrics (CPU, memory, disk).

**Sensitivity**: Low - Operational metrics only.

**Protection**: mTLS encryption, rate limiting.

### DF5: Webhook Ingress

```
External Service → [TLS+HMAC] → API → Control Plane → Event Bus
```

**Data**: Deployment events, git push notifications, CI/CD status.

**Sensitivity**: Medium - Contains deployment metadata.

**Protection**: HMAC signature verification, input validation, rate limiting.

### DF6: Secrets Retrieval

```
Agent → [mTLS] → NATS → Control Plane → [mTLS] → Vault/KMS
```

**Data**: Secret values, credentials, certificates.

**Sensitivity**: Critical - Secrets must never be logged or persisted unencrypted.

**Protection**: TLS encryption, short-lived caching, audit logging, automatic redaction.

## STRIDE Analysis

### Spoofing

| Threat | Component | Mitigation |
|--------|-----------|------------|
| S1: Attacker impersonates agent | Agent registration | Join token attestation, SPIFFE SVIDs with workload attestation, cloud metadata validation |
| S2: Attacker impersonates control plane | NATS communication | mTLS with CA validation, certificate pinning |
| S3: Attacker forges API requests | API server | JWT/API key authentication, request signing |
| S4: Attacker spoofs webhook source | Webhook receiver | HMAC signature verification, source IP allowlisting |

### Tampering

| Threat | Component | Mitigation |
|--------|-----------|------------|
| T1: Modified commands in transit | NATS messaging | TLS encryption, message integrity |
| T2: Tampered state files | State management | Checksum verification, source validation |
| T3: Modified database records | Database | TLS connections, audit logging, backup verification |
| T4: Tampered module code | Module system | Cosign signatures, SumDB transparency log, hash verification |

### Repudiation

| Threat | Component | Mitigation |
|--------|-----------|------------|
| R1: User denies executing command | Command execution | Audit logging with user identity, immutable logs |
| R2: Agent denies receiving command | NATS messaging | JetStream acknowledgment, correlation IDs |
| R3: Administrator denies policy change | Policy engine | Policy audit log with timestamps and user |

### Information Disclosure

| Threat | Component | Mitigation |
|--------|-----------|------------|
| I1: Credential exposure in logs | Logging system | Automatic secret redaction, audit-safe log formats |
| I2: Agent metadata leakage | API responses | Role-based access control, minimal data exposure |
| I3: Database breach | State storage | Encryption at rest, backup encryption |
| I4: Network sniffing | All communications | TLS 1.3 for all connections |
| I5: Join token exposure | Token store | Salted hash storage, tokens never persisted in plaintext |

### Denial of Service

| Threat | Component | Mitigation |
|--------|-----------|------------|
| D1: API flood | API server | Rate limiting, request throttling, circuit breakers |
| D2: Agent heartbeat flood | Connection manager | Per-agent rate limiting, heartbeat intervals |
| D3: Event storm | Event system | Event throttling, debouncing, queue limits |
| D4: Database exhaustion | State storage | Connection pooling, query timeouts, retention policies |
| D5: NATS resource exhaustion | Message bus | JetStream limits, slow consumer detection |

### Elevation of Privilege

| Threat | Component | Mitigation |
|--------|-----------|------------|
| E1: Agent gains control plane access | Authorization | Separate agent/server credentials, RBAC enforcement |
| E2: Read-only user executes commands | API authorization | Policy-based access control, action validation |
| E3: Module escapes sandbox | Module runtime | Capability-based access, WASM/Starlark sandboxing |
| E4: Compromised agent pivots | Agent isolation | Least-privilege execution, network segmentation |

## Security Controls Summary

### Authentication

| Component | Method | Rotation |
|-----------|--------|----------|
| API clients | API key or JWT | Manual or automatic via IdP |
| Agents (Manual mode) | mTLS certificates | Manual rotation |
| Agents (SPIFFE mode) | SVID certificates | Automatic (hourly) |
| Database | Username/password or certificate | Configurable |
| NATS | mTLS certificates | Manual or SPIFFE-managed |

### Authorization

| Resource | Access Control | Default |
|----------|----------------|---------|
| Agent registration | Join token or attestation | Deny all |
| Command execution | OPA/CEL policies | Allow authenticated |
| State application | Target matching | Allow if authorized |
| API endpoints | RBAC roles | Read-only by default |
| Module loading | Trust policies | Deny unsigned |

### Encryption

| Data State | Protection | Algorithm |
|------------|------------|-----------|
| In transit (API) | TLS 1.3 | AES-256-GCM, ChaCha20-Poly1305 |
| In transit (NATS) | TLS 1.3 | AES-256-GCM, ChaCha20-Poly1305 |
| At rest (database) | Transparent encryption | AES-256 |
| At rest (backups) | Age encryption | X25519, ChaCha20-Poly1305 |
| Secrets | Vault/KMS | Provider-specific |

### Auditing

| Event Type | Logged Data | Retention |
|------------|-------------|-----------|
| Authentication | User, method, result, IP | 90 days |
| Command execution | User, command, target, result | 1 year |
| State changes | Resource, before/after, user | 1 year |
| Policy evaluations | Policy, input, result | 90 days |
| Agent lifecycle | Agent ID, event type, metadata | 1 year |

## Deployment Recommendations

### Development/Testing

- Use Manual TLS mode with generated certificates
- InMemoryTokenStore acceptable (data loss on restart)
- Single control plane instance
- SQLite for state storage

### Production (Single-Site)

- Use SPIFFE mode with embedded provider
- SQLiteTokenStore for token persistence
- Multiple control plane instances with etcd
- PostgreSQL for state storage
- TLS for all connections
- Backup encryption enabled

### Production (Multi-Site)

- Use SPIFFE mode with SPIRE integration
- Trust federation between sites
- NATS superclusters with gateways
- PostgreSQL with replication
- Geographic routing for traffic
- Cross-site backup replication

## Incident Response

### Compromised Agent

1. Immediately revoke agent credentials
2. Quarantine affected node (network isolation)
3. Review audit logs for lateral movement
4. Re-attest agent after remediation
5. Rotate any secrets the agent had access to

### Compromised Control Plane

1. Isolate affected control plane instance
2. Revoke all API keys and tokens
3. Rotate database credentials
4. Review all recent commands and state changes
5. Restore from verified backup if necessary
6. Re-issue all agent certificates

### Compromised Database

1. Isolate database instance
2. Assess data exposure
3. Restore from encrypted backup
4. Rotate all credentials stored in database
5. Review and re-apply state configurations

### Supply Chain Attack

1. Identify affected modules/versions
2. Block affected module hashes in SumDB
3. Revoke signing keys if compromised
4. Audit all systems that loaded affected modules
5. Force module re-verification on all agents

## Compliance Mapping

| Framework | Relevant Controls |
|-----------|-------------------|
| **SOC 2** | Access control, encryption, audit logging, change management |
| **PCI-DSS** | Network segmentation, encryption, access control, logging |
| **HIPAA** | Encryption, access control, audit trails, integrity controls |
| **GDPR** | Data encryption, access logging, right to erasure |
| **CIS Benchmarks** | Hardening guides for Linux, Windows, Kubernetes |

## References

- [STRIDE Threat Modeling](https://docs.microsoft.com/en-us/azure/security/develop/threat-modeling-tool-threats)
- [SPIFFE/SPIRE](https://spiffe.io/)
- [OWASP Threat Modeling](https://owasp.org/www-community/Threat_Modeling)
- [NIST Cybersecurity Framework](https://www.nist.gov/cyberframework)
