# Security Review Report (Keystone Core)

Date: 2025-02-14
Reviewer: Codex (security engineer perspective)
Scope: Design documentation and selected security-sensitive code paths

## Executive Summary
The project has strong security intentions (mTLS, SPIFFE/SPIRE, RBAC, policy engine, capability-based modules). However, several defaults and implementation details can lead to insecure deployments or credential exposure. The highest risks are transport security defaults (TLS off while binding to 0.0.0.0) and cleartext join token handling/output. Addressing these should be prioritized.

## Findings and Recommendations

### 1) Transport security defaults expose control plane over plaintext
Severity: High
References:
- pkg/config/config.go:348
- pkg/config/config.go:444
- pkg/config/config.go:480
- DESIGN.md:293
- DESIGN.md:300

Issue:
- TLS is disabled by default (`tls.enabled=false`) while server defaults bind to all interfaces (`0.0.0.0`). This conflicts with the design doc stating “Manual TLS Mode (Default)”. In practice, this can result in tokens and sensitive operations traversing the network in plaintext.

Recommendations:
- Align defaults with the design doc:
  - Default TLS enabled with generated self-signed CA/certs for dev and a clear path to prod certs.
  - Or default to `127.0.0.1` unless `--insecure-bind-all` (or similar) is explicitly set.
- Add startup warnings/failsafe:
  - If `tls.enabled=false` AND listen addr is not loopback, emit a loud warning and optionally refuse to start unless `--allow-insecure` is set.
- Document a secure-by-default path and provide a “dev mode” toggle that is explicit and noisy.

---

### 2) Join token exposure and cleartext storage
Severity: High
References:
- pkg/identity/attestation.go:532
- cmd/kscore-identity/main.go:186
- cmd/kscore-identity/main.go:197
- cmd/kscore-identity/main.go:284

Issue:
- Join tokens are stored and looked up in cleartext (in-memory store shown; likely similar in a persistent store).
- CLI prints full token values and emits configuration snippets containing the token. These are easily captured in terminal logs, shell history, CI logs, or observability pipelines.

Recommendations:
- Store only a salted hash of join tokens and compare in constant time. Keep the raw token only at creation time.
- Modify CLI behavior:
  - Display full token only once on creation (with a warning), then show redacted tokens in `list`/`show`.
  - Add `--show-token` flag for intentional disclosure (with confirmation).
- Add operational guidance: treat join tokens like passwords; avoid copying into logs.

---

### 3) RNG error handling ignored for token/ID generation
Severity: Medium
References:
- pkg/identity/attestation.go:630
- pkg/identity/attestation.go:636

Issue:
- `rand.Read` errors are ignored during token and agent ID generation. In low-entropy or failure conditions, this can produce weak values and silently weaken security guarantees.

Recommendations:
- Check and propagate `rand.Read` errors; fail closed when cryptographic randomness is unavailable.
- Add tests to ensure failures are handled and surfaced.

---

### 4) API key audit metadata leaks key material
Severity: Medium
References:
- pkg/api/auth/auth.go:192
- pkg/api/auth/auth.go:200

Issue:
- The audit metadata stores the first 8 bytes of the raw API key rather than a hash. Even a partial raw key is sensitive and can aid brute-force attacks.

Recommendations:
- Store a hash prefix (e.g., SHA-256 of the full key, then truncate to 8 bytes for logs).
- Clearly mark any key-derived data as non-sensitive in documentation and logs to avoid accidental disclosure.

---

### 5) WebSocket origin policy is permissive by default
Severity: Medium
References:
- pkg/nats/websocket.go:80
- pkg/nats/websocket.go:91

Issue:
- Default WebSocket config allows all origins and disables same-origin checks. If JWT cookie authentication is used, this enables cross-site WebSocket hijacking.

Recommendations:
- Set `SameOrigin=true` by default.
- Require explicit `AllowedOrigins` configuration when `SameOrigin=false`.
- If JWT cookies are in use, document CSRF-style risks and mitigation.

---

### 6) mTLS auth flow still requires metadata credentials
Severity: Low
References:
- pkg/api/auth/interceptors.go:206
- pkg/api/auth/interceptors.go:238

Issue:
- The auth interceptor always demands metadata credentials, even when mTLS client certificates are present. This undermines a pure mTLS authentication model and may encourage disabling mTLS in practice.

Recommendations:
- Allow empty credentials when a verified client certificate exists and an mTLS authenticator is configured.
- If multi-auth is enabled, attempt mTLS auth first before requiring metadata credentials.

---

## Design Alignment and Additional Suggestions

- Manual TLS mode in DESIGN.md suggests automatic CA/cert generation. Consider ensuring this is implemented and enabled by default for non-prod, with explicit opt-in to insecure mode.
- Provide hardening guidance in docs: recommended cipher suites, TLS min version, audit logging configuration, and rotation policies.
- Consider centralized secrets handling (Vault/KMS) for token material and CA keys. The KeyProtector supports encrypted keys; recommend enabling it by default in production configurations.

## Testing Gaps / Validation Ideas

- Add unit tests for token hashing and comparison.
- Add integration tests validating that plaintext token values do not appear in logs or list/show endpoints.
- Add tests for “TLS disabled + non-loopback bind” warnings/failsafe.
- Add mTLS-only authentication flow tests.

## Notes on Scope
- This review focused on security design alignment and selected auth/identity/TLS paths. It did not include a full cryptographic audit, dependency vulnerability scan, or infrastructure deployment review.

---

## Documentation Gaps Review

Date: 2025-02-14
Scope: Docs vs code alignment (config, CLI, behavior)

### Findings

1) Identity configuration schema mismatch
- Docs describe `identity.mode` with `embedded/spire/cloud/mesh`, but the runtime config struct does not include `Identity` at the top level. The identity package uses a different schema (`provider.type`, `trust_domain`, `svid`, `attestation`, etc.). The documented YAML will not map to actual config loading.
References:
  - docs/content/en/docs/reference/configuration.md:310
  - pkg/config/config.go:43
  - pkg/identity/config.go:8

Recommendations:
- Either add an `Identity` field to `pkg/config.Config` and wire it into loading/validation, or update docs to state identity config is separate and provide the correct schema.

2) Security mode config in DESIGN.md does not exist in code
- DESIGN.md references `security.mode` and `security.allow_legacy_tls`, but there is no `security` config in code. TLS is configured via `tls.*` and defaults to disabled.
References:
  - DESIGN.md:293
  - DESIGN.md:467
  - pkg/config/config.go:43
  - pkg/config/config.go:480

Recommendations:
- Update DESIGN.md to reflect current `tls`/`auth` configuration, or implement the `security.*` config it describes.

3) Identity CLI docs don’t match flags/output or implementation
- Docs for `kscorectl identity token` show `--path` and `--uses`, but the CLI implements `--agent-id` and `--label`. The docs also imply real API-backed data, while list/show output is currently demo data.
References:
  - docs/content/en/docs/reference/cli.md:2155
  - docs/content/en/docs/reference/cli.md:2197
  - cmd/kscore-identity/main.go:144
  - cmd/kscore-identity/main.go:213

Recommendations:
- Update docs to match current flags/output and clearly mark list/show as placeholders until API integration exists.

4) Module resolve docs imply registry-backed resolution but code uses mock registry
- Docs claim `module resolve` queries a registry and resolves dependencies, but implementation uses a mock registry client and fails without a lock file or manual cache population.
References:
  - docs/content/en/docs/reference/cli.md:736
  - cmd/kscore-module/cmd_resolve.go:107
  - cmd/kscore-module/cmd_resolve.go:140

Recommendations:
- Mark the current `resolve` behavior as limited/placeholder or implement the real registry client path.

---

## Scaling Concerns Review

Date: 2025-02-14
Scope: Code and production architecture scaling risks

### Findings

1) Pagination incomplete or missing in API list endpoints
- `ListAgents` only slices by `PageSize` and ignores page tokens. `ListCommands` now parses `page_token`, but `ListAgents` still needs parity. Large fleets will see full scans without paging on agents.
References:
  - pkg/api/server/controlplane_server.go:73
  - pkg/api/server/controlplane_server.go:94
  - pkg/api/server/controlplane_server.go:188

Recommendations:
- Implement `page_token` parsing and return `next_page_token` consistently across list endpoints. Prefer indexed queries in the persistent store path.

2) Production deployments can default to embedded NATS + SQLite with small resource caps
- Defaults use embedded NATS and SQLite, and NATS limits are sized for small deployments (1GB JetStream, 1000 connections). This conflicts with production-scale expectations and can cause bottlenecks if not explicitly reconfigured.
References:
  - pkg/config/config.go:354
  - pkg/config/config.go:361
  - pkg/config/config.go:454
  - pkg/config/config.go:456
  - pkg/config/config.go:457
  - DESIGN.md:148
  - DESIGN.md:182

Recommendations:
- Add production-mode validation/warnings when node counts or connection counts exceed embedded limits. Update docs with explicit scaling thresholds and required external NATS/PostgreSQL guidance.

3) In-memory proxy device registry limits scale and HA
- Proxy agent uses an in-memory device registry; large fleets and multi-instance deployments will suffer from data loss on restart, no sharing, and memory growth.
References:
  - pkg/proxy/manager.go:83

Recommendations:
- Add a persistent/shared registry backend (database or NATS KV) and document operational limits for the in-memory default.

4) In-memory join token store is not HA or large-scale safe
- Join tokens are stored in memory; multi-instance control planes will have inconsistent token views and increased memory usage over time.
References:
  - pkg/identity/attestation.go:532

Recommendations:
- Implement a persistent join token store and document that in-memory mode is dev-only.

---

## Threat Modeling Review

Date: 2025-02-14
Scope: Architecture-level trust boundaries and attack surface (desktop review)

### Findings

1) No explicit threat model document or STRIDE/LINDDUN-style analysis found
- The design doc describes architecture and security modes, but there is no dedicated threat model that enumerates trust boundaries, assets, or attacker capabilities.
References:
  - DESIGN.md:113
  - DESIGN.md:144
  - DESIGN.md:291

Recommendations:
- Add a threat model doc that defines trust boundaries (agent, control plane, NATS, storage, identity provider), key assets (tokens, SVIDs, state), and mitigations per boundary.
- Include a data flow diagram with trust boundaries for the embedded vs external NATS/Postgres deployment modes.

2) Webhook and event ingress surfaces are described but lack explicit threat boundary mapping
- Webhook receiver configuration exists, but the design does not describe authentication requirements or threat boundaries in a consolidated way.
References:
  - pkg/config/config.go:293
  - pkg/config/config.go:491

Recommendations:
- Document the expected auth requirements and rate limits for webhook ingress and event ingestion.

---

## Dependency and Vulnerability Review

Date: 2025-02-14
Scope: Dependency scanning posture and hygiene (desktop review; no external scans run)

### Findings

1) CI runs govulncheck and gosec but does not fail the build
- Security scans are marked `continue-on-error: true`, which can allow known issues to ship without visibility in release gating.
References:
  - .github/workflows/ci.yml:118
  - .github/workflows/ci.yml:124

Recommendations:
- Make security scans blocking for release branches or add a policy that requires triage/waivers for failures.
- Add SBOM generation and container image scanning if containers are distributed.

---

## AuthZ and RBAC Review

Date: 2025-02-14
Scope: Authorization coverage and role mapping

### Findings

1) RBAC permissions explicitly cover only ControlPlaneService and ClusterService
- Other gRPC services (e.g., PolicyService) are not enumerated in the default permission map and will default to admin-only. That may be intended but is undocumented and untested.
References:
  - pkg/api/auth/authorizer.go:37
  - api/proto/policy.proto:10

Recommendations:
- Add explicit permission mappings for all public services and document intended access levels.
- Add tests that enumerate all RPCs and assert they are in the permissions map (or explicitly in bypass).

---

## Data Retention and PII Review

Date: 2025-02-14
Scope: Audit logs and data retention behavior (desktop review)

### Findings

1) Policy audit logs are in-memory with a fixed cap and no persistence
- The PolicyAuditor keeps the most recent 10k entries in memory, dropping older records without persistence or export hooks.
References:
  - pkg/policy/audit.go:27
  - pkg/policy/audit.go:35
  - pkg/policy/audit.go:65

Recommendations:
- Add a persistent audit sink (database, object storage, or external SIEM) and document retention/rotation behavior.
- Document which fields may contain PII or secrets and provide redaction guidance.

---

## Performance and Load Review

Date: 2025-02-14
Scope: Performance-critical paths and load behavior

### Findings

1) Embedded NATS/SQLite defaults and small resource caps are not aligned with large fleet loads
- Defaults are tuned for small deployments; without explicit guidance, operators can inadvertently run production workloads on embedded components.
References:
  - pkg/config/config.go:354
  - pkg/config/config.go:361
  - pkg/config/config.go:454

Recommendations:
- Add a production readiness checklist that surfaces required switches (external NATS/PostgreSQL) and recommended JetStream/storage settings.
- Provide load test targets and published baseline numbers for node counts and command throughput.

---

## HA and Failover Review

Date: 2025-02-14
Scope: High availability posture and failure recovery (desktop review)

### Findings

1) HA recommendations exist in design docs but are not enforced or validated at runtime
- The design document recommends external NATS and PostgreSQL for production and HA, but no runtime validation or guardrails are present.
References:
  - DESIGN.md:159
  - DESIGN.md:193
  - pkg/config/config.go:354
  - pkg/config/config.go:361

Recommendations:
- Add startup checks for HA mode (e.g., external NATS + Postgres required when `cluster.enabled` or multi-control-plane is configured).
- Document failover drills and include runbooks for NATS cluster loss, Postgres failover, and control plane leader election.

---

## Session Notes - Phase 1 T1.1 (time.Sleep removal)

Progress:
- Added wait helper: `pkg/testing/helpers/wait.go`.
- Replaced `time.Sleep` with condition-based waits across these areas: events tests, health tests, api/auth, visualization, profiling, container, identity tests, platform/hardware/cloud/policy, targeting, cluster leader test, k8s controller tests, proxy observability, servicemesh, blueprint snapshot tests, module capabilities, metrics tests, credentials cache TTL.

Remaining `time.Sleep` hot spots to continue:
- `pkg/controlplane/*_test.go`, `pkg/agent/*_test.go`, `pkg/nats/*_test.go`, `pkg/files/*_test.go`, `pkg/gitops/*_test.go`, plus some non-test sleeps in `pkg/identity/spire/client.go`, `pkg/logging/syslog.go`, `pkg/files/mirror/sync.go`, and `pkg/upgrade/rolling.go`.
