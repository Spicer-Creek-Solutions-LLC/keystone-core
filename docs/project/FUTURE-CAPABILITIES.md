# Future Capabilities — Unscheduled

This catalog preserves the direction and lessons of Generation 1 without making
a version or delivery promise. Every item below is `Future / Unscheduled` unless
an accepted RFC explicitly promotes it to the active roadmap.

After archive task R05, canonical historical links will resolve under these
locations:

- `archive/2026-09-pre-v0.6-reboot:FEATURES.md`
- `archive/2026-09-pre-v0.6-reboot:PROJECT-DETAILS.md`
- `archive/2026-09-pre-v0.6-reboot:docs/project/ROADMAP.md`
- `archive/2026-09-pre-v0.6-reboot:epics/`

They are code-form placeholders, not broken forward links. R03 has expanded
this catalog to item-level coverage; every source reference below is pinned to
the final Generation 1 commit `93eb147f7`. R05 turns the
convenience archive locations into links.

## Promotion policy

Catalog presence means “retain the idea,” not “planned.” Promotion requires the
gate in `REBOOT-EXECUTION-PLAN.md`. An archived implementation can inform a new
design but is not automatically copied; its production wiring, threat model,
NATS use, and black-box acceptance must be re-established.

## Scope subtraction rule

Some Generation 1 domains also contain the narrow foundations explicitly
required by RFC 0001—for example NATS accounts, a server database, an agent
runtime, exact-agent execution, and lifecycle audit. Those minimum slices are
active v0.6 work and are excluded from `Future / Unscheduled`. The catalog
entries below retain only capabilities beyond the accepted RFC/P/C workstreams,
even when a paragraph names the broader historical domain for context. R03 has
marked every item either `v0.6` or `Future`, and no item carries both.

## How to read the catalog

Each entry has a stable `CAP-*` identifier. Later work cites these rather than
quoting prose, the way `ARCH-*` identifiers work in
[`ARCHITECTURE-INVARIANTS.md`](ARCHITECTURE-INVARIANTS.md).

**Status** records what Generation 1 actually reached, not what it intended:

- **implemented** — evidence exists: a declared source path present in the final
  tree, a completed roadmap entry, or a ticked epic acceptance item.
- **partial** — implemented but with a gap the archive documents. The gap is
  listed under the domain table.
- **planned** — described but never built, or a declared source path that is
  absent from the final tree.
- **unknown** — described as in scope for Generation 1, with no evidence either
  way in the archived sources. This is deliberate: RFC 0001 records that
  package-level coverage coexisted with missing production wiring, so absence of
  evidence is reported rather than resolved into a guess.
- **reference** — not a capability. Domain overviews and the Generation 1
  reconstruction-phase plan are catalogued so their source items are accounted
  for, but they describe or schedule work rather than being work.

**Scope** applies the subtraction rule above. `v0.6` marks the narrow slice
RFC 0001 accepts into the first Generation 2 release, which is therefore *not* a
Future candidate. `Future` marks everything else. No entry is both.

**Source** is the archived location of the item that created the entry, pinned
to `93eb147f7`. Items that refine, duplicate, or restate an entry
are recorded in
[`../transition/capability-coverage.json`](../transition/capability-coverage.json)
rather than repeated here.

### Totals

| | |
|---|---|
| Catalog entries | 703 |
| Source items mapped | 1064 |
| implemented / partial / planned / unknown | 259 / 56 / 260 / 88 |
| reference (non-capability entries) | 40 |
| `v0.6` / `Future` | 50 / 653 |

## Catalog

### Foundations and build system

| ID | Capability | Status | Scope | Source |
|---|---|---|---|---|
| `CAP-FOUND-001` | Cross-platform CLI builds | implemented | Future | `FEATURES.md L51` |
| `CAP-FOUND-002` | Pure-Go build (CGO_ENABLED=0) | implemented | v0.6 | `FEATURES.md L52` |
| `CAP-FOUND-003` | Buf-based proto codegen | implemented | Future | `FEATURES.md L53` |
| `CAP-FOUND-004` | Build-time version injection | implemented | v0.6 | `FEATURES.md L54` |
| `CAP-FOUND-005` | Semver library | implemented | Future | `FEATURES.md L55` |
| `CAP-FOUND-006` | Cancelable wait/poll utilities | implemented | Future | `FEATURES.md L56` |
| `CAP-FOUND-007` | koanf-based config | implemented | v0.6 | `FEATURES.md L57` |
| `CAP-FOUND-008` | Structured logging | implemented | v0.6 | `FEATURES.md L58` |
| `CAP-FOUND-009` | Standard error model | implemented | v0.6 | `FEATURES.md L59` |
| `CAP-FOUND-010` | Make targets | implemented | v0.6 | `FEATURES.md L60` |
| `CAP-FOUND-011` | Goreleaser snapshot | implemented | v0.6 | `FEATURES.md L61` |
| `CAP-FOUND-012` | Pre-commit hooks | implemented | Future | `FEATURES.md L62` |
| `CAP-FOUND-013` | Baseline lint set | implemented | v0.6 | `FEATURES.md L63` |
| `CAP-FOUND-014` | Single-topology E2E | implemented | v0.6 | `FEATURES.md L64` |
| `CAP-FOUND-015` | Documentation site | partial | Future | `FEATURES.md L68` |
| `CAP-FOUND-016` | Syslog logging output | planned | Future | `FEATURES.md L72` |
| `CAP-FOUND-017` | PDF export of the docs site | planned | Future | `FEATURES.md L73` |
| `CAP-FOUND-018` | HA / IPv6 / HA+IPv6 E2E topologies | planned | Future | `FEATURES.md L74` |
| `CAP-FOUND-019` | Hot-reload dev server (air) | planned | Future | `FEATURES.md L75` |
| `CAP-FOUND-020` | Repository generation (DNF/APT) | planned | Future | `FEATURES.md L76` |
| `CAP-FOUND-021` | VM bootstrap test harness | planned | v0.6 | `FEATURES.md L77` |
| `CAP-FOUND-022` | Full security scanning suite | planned | Future | `FEATURES.md L78` |
| `CAP-FOUND-023` | Goreleaser signing ceremony / multi-party release | planned | Future | `FEATURES.md L79` |
| `CAP-FOUND-024` | Benchmark suite | planned | Future | `FEATURES.md L80` |
| `CAP-FOUND-025` | Air-gapped repo packaging (kscore-bootstrap) | implemented | Future | `FEATURES.md L81` |
| `CAP-FOUND-026` | Rotation orchestrator | planned | Future | `docs/project/ROADMAP.md L504` |
| `CAP-FOUND-027` | WASM module runtime | planned | Future | `docs/project/ROADMAP.md L1355` |
| `CAP-FOUND-028` | K8s operator | planned | Future | `docs/project/ROADMAP.md L1431` |
| `CAP-FOUND-029` | All 19 epics complete with their acceptance criteria | planned | Future | `epics/00-meta-reconstruction-plan.md L58` |
| `CAP-FOUND-030` | make build produces three binaries in build/bin/$GOOS/$GOARCH/ | implemented | Future | `epics/01-foundations.md L61` |
| `CAP-FOUND-031` | kscore-server --version prints version + commit + build date | implemented | Future | `epics/01-foundations.md L63` |
| `CAP-FOUND-032` | make proto round-trips an empty proto file successfully | implemented | Future | `epics/01-foundations.md L67` |
| `CAP-FOUND-033` | Domain overview and architecture notes | reference | Future | `PROJECT-DETAILS.md L100` |

Scope and status notes:

- `CAP-FOUND-002` — Status established by hand audit of the v0.6 scope set: .goreleaser.yaml sets CGO_ENABLED=0.
- `CAP-FOUND-004` — Status established by hand audit of the v0.6 scope set: pkg/version/.
- `CAP-FOUND-007` — Status established by hand audit of the v0.6 scope set: internal/config/ is koanf-based.
- `CAP-FOUND-009` — Status established by hand audit of the v0.6 scope set: pkg/api/apierror/.
- `CAP-FOUND-010` — Status established by hand audit of the v0.6 scope set: Makefile.
- `CAP-FOUND-021` — Status established by hand audit of the v0.6 scope set: No VM harness exists in the tree; test/ has no vagrant, libvirt or qemu driver.
- `CAP-FOUND-025` — FEATURES.md declares no source path; the capability is evidenced by `cmd/kscore-bootstrap/`.

Known gaps and limitations:

- `CAP-FOUND-015` — Hugo docs site.
- `CAP-FOUND-020` — Native package repositories — APT, DNF/YUM.
- `CAP-FOUND-023` — Release signing ceremony — signed tags + checksums + SBOMs.
- `CAP-FOUND-026` — The backlog records this as outstanding work; the entry names no reference paths, so the archive shows what was wanted rather than what was built.
- `CAP-FOUND-027` — The backlog records this as outstanding work; the entry names no reference paths, so the archive shows what was wanted rather than what was built.
- `CAP-FOUND-028` — The backlog records this as outstanding work; the entry names no reference paths, so the archive shows what was wanted rather than what was built.

### NATS messaging

| ID | Capability | Status | Scope | Source |
|---|---|---|---|---|
| `CAP-NATS-001` | Embedded NATS server | implemented | Future | `FEATURES.md L89` |
| `CAP-NATS-002` | External NATS cluster | implemented | Future | `FEATURES.md L90` |
| `CAP-NATS-003` | Subject hierarchy | implemented | v0.6 | `FEATURES.md L91` |
| `CAP-NATS-004` | Direct TCP + TLS connection strategies | implemented | v0.6 | `FEATURES.md L92` |
| `CAP-NATS-005` | Multi-endpoint failover with health checks | unknown | Future | `FEATURES.md L93` |
| `CAP-NATS-006` | Per-endpoint circuit breaker | implemented | Future | `FEATURES.md L94` |
| `CAP-NATS-007` | Message envelope | implemented | v0.6 | `FEATURES.md L95` |
| `CAP-NATS-008` | JetStream enablement | implemented | v0.6 | `FEATURES.md L96` |
| `CAP-NATS-009` | Bootstrap registration with minimal-permission credentials | partial | v0.6 | `FEATURES.md L97` |
| `CAP-NATS-010` | Connection state machine | unknown | Future | `FEATURES.md L98` |
| `CAP-NATS-011` | Static endpoint configuration | unknown | Future | `FEATURES.md L99` |
| `CAP-NATS-012` | Health check hooks | implemented | Future | `FEATURES.md L100` |
| `CAP-NATS-013` | Leaf node mode | planned | Future | `FEATURES.md L104` |
| `CAP-NATS-014` | Supercluster / gateway | planned | Future | `FEATURES.md L105` |
| `CAP-NATS-015` | WebSocket / WSS transport | planned | Future | `FEATURES.md L106` |
| `CAP-NATS-016` | Auto-discovery | planned | Future | `FEATURES.md L107` |
| `CAP-NATS-017` | NAT traversal via reverse leaf | planned | Future | `FEATURES.md L108` |
| `CAP-NATS-018` | Exactly-once delivery | planned | Future | `FEATURES.md L109` |
| `CAP-NATS-019` | Boostrap PSK consumption: in-memory tracking only | planned | Future | `docs/project/ROADMAP.md L591` |
| `CAP-NATS-020` | Bootstrap protocol versioning | planned | v0.6 | `docs/project/ROADMAP.md L760` |
| `CAP-NATS-021` | Active dial-time circuit breaker eviction | planned | Future | `docs/project/ROADMAP.md L983` |
| `CAP-NATS-022` | Health() reports unhealthy when NATS down; recovers when up | implemented | Future | `epics/05-nats-messaging.md L72` |
| `CAP-NATS-023` | Coverage >80% on internal/nats | implemented | Future | `epics/05-nats-messaging.md L74` |
| `CAP-NATS-024` | Domain overview and architecture notes | reference | Future | `PROJECT-DETAILS.md L214` |

Scope and status notes:

- `CAP-NATS-004` — Status established by hand audit of the v0.6 scope set: internal/nats/strategy.go builds the TLS connection strategy.
- `CAP-NATS-006` — FEATURES.md declares no source path; the capability is evidenced by `internal/webhook/outbound/circuit_breaker.go`.
- `CAP-NATS-008` — Status established by hand audit of the v0.6 scope set: internal/nats/jetstream.go.
- `CAP-NATS-012` — FEATURES.md declares a source path that no longer resolves, but the identifiers it quotes are present in the tree: `/health/ready`.
- `CAP-NATS-020` — Status established by hand audit of the v0.6 scope set: The roadmap entry states a protocol_version field would make compatibility explicit; it was not built.

Known gaps and limitations:

- `CAP-NATS-009` — Bootstrap credential handling exists under internal/nats/, but the archived roadmap records the minimal-permission scoping as unfinished.
- `CAP-NATS-013` — Embedded NATS / hybrid mode / leaf node / endpoint advertiser / supercluster / WebSocket.
- `CAP-NATS-019` — The backlog item's own text states this had not been built: Boostrap PSK consumption: in-memory tracking only.
- `CAP-NATS-020` — Referenced code exists, but this backlog item describes work that had not landed: `AgentCredentials.ProtocolVersion uint32` field; bootstrap response carries the current version; agents check + log on mismatch.
- `CAP-NATS-021` — The backlog item's own text states this had not been built: An endpoint with breaker OPEN is skipped during reconnect attempts until the breaker half-opens.

### Storage

| ID | Capability | Status | Scope | Source |
|---|---|---|---|---|
| `CAP-STORE-001` | SQLite backend | implemented | v0.6 | `FEATURES.md L117` |
| `CAP-STORE-002` | PostgreSQL backend | implemented | Future | `FEATURES.md L118` |
| `CAP-STORE-003` | Pure-Go drivers | implemented | Future | `FEATURES.md L119` |
| `CAP-STORE-004` | Auto-schema initialization | implemented | Future | `FEATURES.md L120` |
| `CAP-STORE-005` | Repository pattern | unknown | Future | `FEATURES.md L121` |
| `CAP-STORE-006` | Direct parametrized SQL | unknown | Future | `FEATURES.md L122` |
| `CAP-STORE-007` | Connection pooling tuned per-backend | unknown | Future | `FEATURES.md L123` |
| `CAP-STORE-008` | JSON-encoded complex columns | unknown | Future | `FEATURES.md L124` |
| `CAP-STORE-009` | SQLite → PostgreSQL migration tool | implemented | Future | `FEATURES.md L125` |
| `CAP-STORE-010` | Migration features | implemented | Future | `FEATURES.md L126` |
| `CAP-STORE-011` | IPv6-safe DSN building | implemented | Future | `FEATURES.md L127` |
| `CAP-STORE-012` | Schema versioning / golang-migrate | planned | Future | `FEATURES.md L131` |
| `CAP-STORE-013` | Encryption at rest | planned | Future | `FEATURES.md L132` |
| `CAP-STORE-014` | Multi-table transaction wrapper (Tx) | planned | Future | `FEATURES.md L133` |
| `CAP-STORE-015` | Backup/restore as Store API methods | implemented | Future | `FEATURES.md L134` |
| `CAP-STORE-016` | Query backends — Loki/Prometheus/Jaeger integration | planned | Future | `FEATURES.md L135` |
| `CAP-STORE-017` | Cloud KMS for storage encryption keys | planned | Future | `FEATURES.md L136` |
| `CAP-STORE-018` | Store interface fully wired for both backends; all sub-interfaces stub-or-real | implemented | Future | `epics/02-storage-layer.md L47` |
| `CAP-STORE-019` | Connection pools configured per backend | implemented | Future | `epics/02-storage-layer.md L49` |
| `CAP-STORE-020` | JSON unmarshal errors return errors, not empty maps | implemented | Future | `epics/02-storage-layer.md L51` |
| `CAP-STORE-021` | continue-on-error records errors in txlog and continues | implemented | Future | `epics/02-storage-layer.md L54` |
| `CAP-STORE-022` | Domain overview and architecture notes | reference | Future | `PROJECT-DETAILS.md L257` |

Scope and status notes:

- `CAP-STORE-001` — Status established by hand audit of the v0.6 scope set: SQLite stores exist per domain, for example internal/webhook/outbound/store_sqlite.go and internal/gitops/rollback/store_sqlite.go.
- `CAP-STORE-003` — FEATURES.md declares a source path that no longer resolves, but the identifiers it quotes are present in the tree: `modernc.org/sqlite`.
- `CAP-STORE-015` — FEATURES.md declares no source path; the capability is evidenced by `cmd/kscore-backup/`.

Known gaps and limitations:

- `CAP-STORE-012` — Schema versioning via `golang-migrate`.

### Control plane core

| ID | Capability | Status | Scope | Source |
|---|---|---|---|---|
| `CAP-CTRL-001` | kscore-server daemon | implemented | v0.6 | `FEATURES.md L144` |
| `CAP-CTRL-002` | Connection Manager | implemented | Future | `FEATURES.md L145` |
| `CAP-CTRL-003` | Command Dispatcher | implemented | Future | `FEATURES.md L146` |
| `CAP-CTRL-004` | Batch Dispatcher | implemented | Future | `FEATURES.md L147` |
| `CAP-CTRL-005` | Listen ports | unknown | Future | `FEATURES.md L148` |
| `CAP-CTRL-006` | Dual-stack (IPv4 + IPv6) listeners | unknown | Future | `FEATURES.md L149` |
| `CAP-CTRL-007` | Health endpoints | implemented | v0.6 | `FEATURES.md L150` |
| `CAP-CTRL-008` | Middleware chain | implemented | Future | `FEATURES.md L151` |
| `CAP-CTRL-009` | Graceful shutdown sequence | implemented | v0.6 | `FEATURES.md L152` |
| `CAP-CTRL-010` | 30s status ticker logging | unknown | Future | `FEATURES.md L153` |
| `CAP-CTRL-011` | Production warnings on startup | unknown | Future | `FEATURES.md L154` |
| `CAP-CTRL-012` | Default zero-config startup | unknown | Future | `FEATURES.md L155` |
| `CAP-CTRL-013` | gRPC reflection / channelz | planned | Future | `FEATURES.md L159` |
| `CAP-CTRL-014` | Webhook receiver port (8081) | planned | Future | `FEATURES.md L160` |
| `CAP-CTRL-015` | K8s operator wiring | planned | Future | `FEATURES.md L161` |
| `CAP-CTRL-016` | Profiling endpoint defaults-on | planned | Future | `FEATURES.md L162` |
| `CAP-CTRL-017` | Cluster leader-check boot wiring (kscore-server) | planned | Future | `docs/project/ROADMAP.md L279` |
| `CAP-CTRL-018` | kscore-server run --config dev.yaml starts in &lt;2s on a laptop | implemented | Future | `epics/04-control-plane-core.md L53` |
| `CAP-CTRL-019` | First-run banner includes versions, ports, auth mode, production warnings | implemented | Future | `epics/04-control-plane-core.md L54` |
| `CAP-CTRL-020` | All 7 v1.0 gRPC services register (with nil-guarded conditional services for cluster/policy/etc.) | planned | Future | `epics/04-control-plane-core.md L55` |
| `CAP-CTRL-021` | All v1.0 REST handler routes registered (with nil-guard for not-yet-implemented domains) | implemented | Future | `epics/04-control-plane-core.md L56` |
| `CAP-CTRL-022` | /health/live → 200 always | implemented | Future | `epics/04-control-plane-core.md L57` |
| `CAP-CTRL-023` | /health/ready → 503 during grace period, 200 after when deps healthy, 503 if NATS or DB unreachable | implemented | Future | `epics/04-control-plane-core.md L58` |
| `CAP-CTRL-024` | Auth interceptor logs denials and includes principal info on accepted requests | implemented | Future | `epics/04-control-plane-core.md L60` |
| `CAP-CTRL-025` | SIGTERM produces ordered shutdown logs; integration test verifies no goroutine leaks (goleak package) | implemented | Future | `epics/04-control-plane-core.md L61` |
| `CAP-CTRL-026` | Domain overview and architecture notes | reference | Future | `PROJECT-DETAILS.md L312` |

Scope and status notes:

- `CAP-CTRL-003` — FEATURES.md declares no source path; the capability is evidenced by `internal/controlplane/command_dispatcher.go`.
- `CAP-CTRL-004` — FEATURES.md declares no source path; the capability is evidenced by `internal/controlplane/batch_dispatcher.go`.
- `CAP-CTRL-007` — FEATURES.md declares a source path that no longer resolves, but the identifiers it quotes are present in the tree: `/health/live`, `/health/ready`, `/health/status`, `/api/status`.
- `CAP-CTRL-007` — Status established by hand audit of the v0.6 scope set: internal/health/checkers.go.
- `CAP-CTRL-009` — Status established by hand audit of the v0.6 scope set: Ordered shutdown is exercised by internal/controlplane/bootstrap_integration_test.go.

Known gaps and limitations:

- `CAP-CTRL-017` — Referenced code exists, but this backlog item describes work that had not landed: Cluster leader-check boot wiring (kscore-server).
- `CAP-CTRL-017` — Server boot-integration foundation (kscore-server lifecycle wiring).

### API surface

| ID | Capability | Status | Scope | Source |
|---|---|---|---|---|
| `CAP-API-001` | AgentService | planned | Future | `FEATURES.md L170` |
| `CAP-API-002` | ControlPlaneService | implemented | Future | `FEATURES.md L171` |
| `CAP-API-003` | StateService | implemented | Future | `FEATURES.md L172` |
| `CAP-API-004` | EventService | implemented | Future | `FEATURES.md L173` |
| `CAP-API-005` | PolicyService | implemented | Future | `FEATURES.md L174` |
| `CAP-API-006` | SecretsService | implemented | Future | `FEATURES.md L175` |
| `CAP-API-007` | ClusterService | implemented | Future | `FEATURES.md L176` |
| `CAP-API-008` | CoordinationService | implemented | Future | `FEATURES.md L177` |
| `CAP-API-009` | AuthN | implemented | Future | `FEATURES.md L178` |
| `CAP-API-010` | REST endpoints (v1.0 wired) | implemented | Future | `FEATURES.md L179` |
| `CAP-API-011` | Streaming patterns | unknown | Future | `FEATURES.md L180` |
| `CAP-API-012` | Standardized pagination | unknown | Future | `FEATURES.md L181` |
| `CAP-API-013` | Standard error model | implemented | Future | `FEATURES.md L182` |
| `CAP-API-014` | Versioning registry | implemented | Future | `FEATURES.md L183` |
| `CAP-API-015` | gRPC ↔ REST mapping | unknown | Future | `FEATURES.md L184` |
| `CAP-API-016` | MaintenanceService | planned | Future | `FEATURES.md L188` |
| `CAP-API-017` | ScheduleService | planned | Future | `FEATURES.md L189` |
| `CAP-API-018` | RunbookService gRPC | partial | Future | `FEATURES.md L190` |
| `CAP-API-019` | WebhookService | planned | Future | `FEATURES.md L191` |
| `CAP-API-020` | GitOps webhook REST handlers wiring | planned | Future | `FEATURES.md L192` |
| `CAP-API-021` | MirrorService / DiscoveryService | planned | Future | `FEATURES.md L193` |
| `CAP-API-022` | Full RBAC (pkg/api/rbac) | planned | Future | `FEATURES.md L194` |
| `CAP-API-023` | gRPC-gateway adoption | planned | Future | `FEATURES.md L195` |
| `CAP-API-024` | OpenAPI auto-generation from protos | planned | Future | `FEATURES.md L196` |
| `CAP-API-025` | Maintenance + Schedule gRPC | planned | Future | `docs/project/ROADMAP.md L452` |
| `CAP-API-026` | Encrypted-file per-secret TTL expiry | planned | Future | `docs/project/ROADMAP.md L1326` |
| `CAP-API-027` | All 8 protos compile via make proto | implemented | Future | `epics/03-api-surface.md L53` |
| `CAP-API-028` | buf lint passes; buf breaking against main clean | implemented | Future | `epics/03-api-surface.md L54` |
| `CAP-API-029` | CoordinationService rejects non-mTLS callers | implemented | Future | `epics/03-api-surface.md L57` |
| `CAP-API-030` | API key generation returns cleartext only on creation; storage holds hash only | implemented | Future | `epics/03-api-surface.md L59` |
| `CAP-API-031` | Bypass list (health, registration, coordination internal) works without credentials | implemented | Future | `epics/03-api-surface.md L60` |
| `CAP-API-032` | Domain overview and architecture notes | reference | Future | `PROJECT-DETAILS.md L386` |

Scope and status notes:

- `CAP-API-002` — Registered on the gRPC server and implemented outside generated code.
- `CAP-API-003` — Registered on the gRPC server and implemented outside generated code.
- `CAP-API-004` — Registered on the gRPC server and implemented outside generated code.
- `CAP-API-005` — Registered on the gRPC server and implemented outside generated code.
- `CAP-API-006` — Registered on the gRPC server and implemented outside generated code.
- `CAP-API-007` — Registered on the gRPC server and implemented outside generated code.
- `CAP-API-008` — Registered on the gRPC server and implemented outside generated code.
- `CAP-API-010` — FEATURES.md declares a source path that no longer resolves, but the identifiers it quotes are present in the tree: `/api/status`, `/api/v1/agents`, `/api/v1/policies`, `/api/v1/apikeys`.
- `CAP-API-013` — FEATURES.md declares a source path that no longer resolves, but the identifiers it quotes are present in the tree: `pkg/api/apierror`.
- `CAP-API-013` — Status established by hand audit of the v0.6 scope set: pkg/api/apierror/apierror.go.
- `CAP-API-014` — FEATURES.md declares a source path that no longer resolves, but the identifiers it quotes are present in the tree: `pkg/api/versioning`.

Known gaps and limitations:

- `CAP-API-001` — The proto and generated stubs exist, but nothing outside generated and test code implements its server interface and it is never registered on the gRPC server. RFC 0001 names this exact pattern -- package-level presence mistaken for production wiring -- as founding evidence for the reboot.
- `CAP-API-016` — The proto and generated stubs exist, but nothing outside generated and test code implements its server interface and it is never registered on the gRPC server. RFC 0001 names this exact pattern -- package-level presence mistaken for production wiring -- as founding evidence for the reboot.
- `CAP-API-017` — The proto and generated stubs exist, but nothing outside generated and test code implements its server interface and it is never registered on the gRPC server. RFC 0001 names this exact pattern -- package-level presence mistaken for production wiring -- as founding evidence for the reboot.
- `CAP-API-018` — The proto and generated stubs exist, but it is never registered on the gRPC server. RFC 0001 names this exact pattern -- package-level presence mistaken for production wiring -- as founding evidence for the reboot.
- `CAP-API-019` — The proto and generated stubs exist, but nothing outside generated and test code implements its server interface and it is never registered on the gRPC server. RFC 0001 names this exact pattern -- package-level presence mistaken for production wiring -- as founding evidence for the reboot.
- `CAP-API-021` — The proto and generated stubs exist, but nothing outside generated and test code implements its server interface and it is never registered on the gRPC server. RFC 0001 names this exact pattern -- package-level presence mistaken for production wiring -- as founding evidence for the reboot.
- `CAP-API-023` — gRPC-gateway annotation-driven REST + OpenAPI auto-gen.
- `CAP-API-025` — The backlog item's own text states this had not been built: gRPC clients call `MaintenanceService.{Plan,Apply,Status}` and `ScheduleService.{Create,List,Run}`.
- `CAP-API-026` — Referenced code exists, but this backlog item describes work that had not landed: encrypted-file backend reaps entries whose `Metadata["ttl_seconds"]` has elapsed since `UpdatedAt`; `GetSecret` on an expired path returns `ErrSecretNotFound` even without an expli.

### Agent runtime

| ID | Capability | Status | Scope | Source |
|---|---|---|---|---|
| `CAP-AGENT-001` | kscore-agent daemon | implemented | v0.6 | `FEATURES.md L204` |
| `CAP-AGENT-002` | Agent registration | implemented | v0.6 | `FEATURES.md L205` |
| `CAP-AGENT-003` | Heartbeat loop with system metrics | implemented | v0.6 | `FEATURES.md L206` |
| `CAP-AGENT-004` | Continuous metadata collection | implemented | Future | `FEATURES.md L207` |
| `CAP-AGENT-005` | Command execution engine | implemented | v0.6 | `FEATURES.md L208` |
| `CAP-AGENT-006` | Security enforcement | implemented | Future | `FEATURES.md L209` |
| `CAP-AGENT-007` | TUI-guided bootstrap | partial | Future | `FEATURES.md L210` |
| `CAP-AGENT-008` | Non-interactive bootstrap | implemented | v0.6 | `FEATURES.md L211` |
| `CAP-AGENT-009` | Bootstrap phases | unknown | Future | `FEATURES.md L212` |
| `CAP-AGENT-010` | Systemd service install + management | implemented | v0.6 | `FEATURES.md L213` |
| `CAP-AGENT-011` | Self-signed CA bootstrap path | unknown | Future | `FEATURES.md L214` |
| `CAP-AGENT-012` | Agent config on disk | implemented | v0.6 | `FEATURES.md L215` |
| `CAP-AGENT-013` | Graceful shutdown | implemented | v0.6 | `FEATURES.md L216` |
| `CAP-AGENT-014` | Reconnect with exponential backoff | implemented | v0.6 | `FEATURES.md L217` |
| `CAP-AGENT-015` | Plugin host integration | unknown | Future | `FEATURES.md L218` |
| `CAP-AGENT-016` | State runner integration | unknown | Future | `FEATURES.md L219` |
| `CAP-AGENT-017` | Embedded NATS / hybrid mode (agent as host or leaf) | planned | Future | `FEATURES.md L223` |
| `CAP-AGENT-018` | Endpoint advertiser + reverse-leaf NAT traversal | planned | Future | `FEATURES.md L224` |
| `CAP-AGENT-019` | Windows agent | planned | Future | `FEATURES.md L225` |
| `CAP-AGENT-020` | macOS agent | planned | Future | `FEATURES.md L226` |
| `CAP-AGENT-021` | Interactive shell sessions | planned | Future | `FEATURES.md L227` |
| `CAP-AGENT-022` | VM-based bootstrap test harness | planned | v0.6 | `FEATURES.md L228` |
| `CAP-AGENT-023` | Auto-rotation of NATS creds in memory | planned | Future | `FEATURES.md L229` |
| `CAP-AGENT-024` | Replay protection on agent commands | planned | Future | `docs/project/ROADMAP.md L428` |
| `CAP-AGENT-025` | AWS decorrelated jitter for fleet-scale reconnect storms | planned | Future | `docs/project/ROADMAP.md L1447` |
| `CAP-AGENT-026` | kscore-agent service start\|stop subcommands | planned | Future | `docs/project/ROADMAP.md L1503` |
| `CAP-AGENT-027` | Bootstrap auto-installs systemd unit (production mode) | planned | Future | `docs/project/ROADMAP.md L1527` |
| `CAP-AGENT-028` | Type=notify systemd integration (sd_notify) | planned | Future | `docs/project/ROADMAP.md L1535` |
| `CAP-AGENT-029` | Bootstrap wizard: storage backend + blueprint selection screens | planned | Future | `docs/project/ROADMAP.md L1543` |
| `CAP-AGENT-030` | Bootstrap: no rollback / transactional revert | planned | Future | `docs/project/ROADMAP.md L1551` |
| `CAP-AGENT-031` | Dedicated kscore system user auto-creation | partial | v0.6 | `docs/project/ROADMAP.md L1559` |
| `CAP-AGENT-032` | Cluster-wide HMAC secret (vs per-agent) | planned | Future | `docs/project/ROADMAP.md L1600` |
| `CAP-AGENT-033` | HMAC-invalid command rejected with audit log entry | implemented | Future | `epics/06-agent-runtime.md L73` |
| `CAP-AGENT-034` | Re-running bootstrap is idempotent (no duplicate systemd units, no broken state) | implemented | Future | `epics/06-agent-runtime.md L77` |
| `CAP-AGENT-035` | Coverage >75% on internal/agent | implemented | Future | `epics/06-agent-runtime.md L79` |
| `CAP-AGENT-036` | Domain overview and architecture notes | reference | Future | `PROJECT-DETAILS.md L437` |
| `CAP-AGENT-037` | Command execution extras beyond the argv-only boundary: caller-selected working directory, caller-provided environment, and user switching | implemented | Future | `FEATURES.md L208` |

Scope and status notes:

- `CAP-AGENT-002` — Status established by hand audit of the v0.6 scope set: internal/agent/agent.go.
- `CAP-AGENT-003` — Status established by hand audit of the v0.6 scope set: internal/agent/agent.go with internal/agent/metadata.go.
- `CAP-AGENT-005` — Status established by hand audit of the v0.6 scope set: internal/agent/executor.go.
- `CAP-AGENT-005` — RFC 0001 confines the first release to argv-only, non-interactive execution. The caller-selected working directory, caller-provided environment and user-switching behaviour named in the archived description are outside that boundary; admitting them requires an RFC amendment rather than ordinary promotion.
- `CAP-AGENT-008` — Status established by hand audit of the v0.6 scope set: cmd/kscore-agent/main.go registers --non-interactive with the accompanying flags.
- `CAP-AGENT-010` — Status established by hand audit of the v0.6 scope set: internal/agent/systemd/install.go.
- `CAP-AGENT-012` — FEATURES.md declares a source path that no longer resolves, but the identifiers it quotes are present in the tree: `/etc/kscore/agent.yaml`.
- `CAP-AGENT-012` — Status established by hand audit of the v0.6 scope set: internal/agent/bootstrap/paths.go.
- `CAP-AGENT-014` — Status established by hand audit of the v0.6 scope set: internal/nats/backoff.go with internal/nats/connection_manager.go.
- `CAP-AGENT-017` — FEATURES.md declares source path(s) that are absent from the final tree: internal/agent/nats_server.go
- `CAP-AGENT-022` — Status established by hand audit of the v0.6 scope set: No VM harness exists in the tree; test/ has no vagrant, libvirt or qemu driver.
- `CAP-AGENT-022` — Duplicate of `CAP-FOUND-021`; both describe the same capability and carry the same scope.
- `CAP-AGENT-037` — The part of `CAP-AGENT-005` that RFC 0001 places outside the argv-only, non-interactive execution boundary. Catalogued separately so it is retained as a Future candidate rather than falling outside both buckets.

Known gaps and limitations:

- `CAP-AGENT-007` — Bootstrap: demo mode only (TUI + non-interactive).
- `CAP-AGENT-017` — Server-side heartbeat / metadata NATS subscriber → agent registry.
- `CAP-AGENT-023` — Auto-rotation of in-memory NATS creds.
- `CAP-AGENT-024` — Referenced code exists, but this backlog item describes work that had not landed: Replayed `CommandRequest` (same nonce within window) is rejected with a typed error and audit log; legitimate commands inside the window pass.
- `CAP-AGENT-025` — Referenced code exists, but this backlog item describes work that had not landed: `reconnectDelay` (or a sibling) implements decorrelated jitter; benchmark/sim shows tighter reconnect-time distribution at 1000+ agent scale; opt-in via a config knob (`reconnectji.
- `CAP-AGENT-026` — The backlog records this as outstanding work; the entry names no reference paths, so the archive shows what was wanted rather than what was built.
- `CAP-AGENT-027` — Referenced code exists, but this backlog item describes work that had not landed: Bootstrap auto-installs systemd unit (production mode).
- `CAP-AGENT-028` — Referenced code exists, but this backlog item describes work that had not landed: Type=notify systemd integration (sd_notify).
- `CAP-AGENT-029` — The backlog records this as outstanding work; the entry names no reference paths, so the archive shows what was wanted rather than what was built.
- `CAP-AGENT-030` — The backlog item's own text states this had not been built: Failed bootstrap re-runs cleanly; for true rollback, operator runs `kscore-agent bootstrap --rollback`.
- `CAP-AGENT-031` — The service install path requires the user to already exist; there is no auto-create.
- `CAP-AGENT-031` — The backlog records this as outstanding work; the entry names no reference paths, so the archive shows what was wanted rather than what was built.
- `CAP-AGENT-032` — The backlog item's own text states this had not been built: Cluster-wide HMAC secret (vs per-agent).

### Remote execution and targeting

| ID | Capability | Status | Scope | Source |
|---|---|---|---|---|
| `CAP-EXEC-001` | Single-agent execution | implemented | v0.6 | `FEATURES.md L237` |
| `CAP-EXEC-002` | Batch execution across target expressions | implemented | Future | `FEATURES.md L238` |
| `CAP-EXEC-003` | Streaming output protocol | unknown | Future | `FEATURES.md L239` |
| `CAP-EXEC-004` | Targeting by hostname glob | implemented | Future | `FEATURES.md L240` |
| `CAP-EXEC-005` | Targeting by labels | implemented | Future | `FEATURES.md L241` |
| `CAP-EXEC-006` | Targeting by built-in fields | unknown | Future | `FEATURES.md L242` |
| `CAP-EXEC-007` | Compound expressions | implemented | Future | `FEATURES.md L243` |
| `CAP-EXEC-008` | Dry-run mode | implemented | Future | `FEATURES.md L244` |
| `CAP-EXEC-009` | Job tracking | implemented | v0.6 | `FEATURES.md L245` |
| `CAP-EXEC-010` | Cancellation | partial | v0.6 | `FEATURES.md L246` |
| `CAP-EXEC-011` | Cross-platform shell abstraction | implemented | Future | `FEATURES.md L247` |
| `CAP-EXEC-012` | Continue-on-failure flag | unknown | Future | `FEATURES.md L248` |
| `CAP-EXEC-013` | Command policy | implemented | v0.6 | `FEATURES.md L249` |
| `CAP-EXEC-014` | kscorectl exec run\|async\|status\|list\|cancel\|output\|script CLI | implemented | v0.6 | `FEATURES.md L250` |
| `CAP-EXEC-015` | Fact-based selectors | planned | Future | `FEATURES.md L254` |
| `CAP-EXEC-016` | Percentage-based / rolling batches | planned | Future | `FEATURES.md L255` |
| `CAP-EXEC-017` | Output archival to object storage (S3/GCS) cold-tier | planned | Future | `FEATURES.md L256` |
| `CAP-EXEC-018` | Interactive shell over stream | planned | Future | `FEATURES.md L257` |
| `CAP-EXEC-019` | Agent-side cancel propagation (SIGTERM to in-flight commands) | planned | v0.6 | `docs/project/ROADMAP.md L396` |
| `CAP-EXEC-020` | Unified single + batch dispatch persistence | planned | Future | `docs/project/ROADMAP.md L404` |
| `CAP-EXEC-021` | Server-side target expression compile (proto extension) | planned | Future | `docs/project/ROADMAP.md L412` |
| `CAP-EXEC-022` | Batch job retention (DeleteBatchJobsBefore + cascading FK) | planned | Future | `docs/project/ROADMAP.md L420` |
| `CAP-EXEC-023` | Live mid-execution stdout / stderr streaming | partial | Future | `docs/project/ROADMAP.md L784` |
| `CAP-EXEC-024` | Eval-time observability for malformed targeting patterns | planned | Future | `docs/project/ROADMAP.md L792` |
| `CAP-EXEC-025` | Output truncation works at configured limits | implemented | v0.6 | `epics/07-remote-execution.md L62` |
| `CAP-EXEC-026` | Domain overview and architecture notes | reference | Future | `PROJECT-DETAILS.md L490` |
| `CAP-EXEC-027` | kscorectl exec async, list and script subcommands (batch fan-out and script execution) | implemented | Future | `FEATURES.md L250` |

Scope and status notes:

- `CAP-EXEC-001` — Status established by hand audit of the v0.6 scope set: internal/controlplane/command_dispatcher.go.
- `CAP-EXEC-007` — FEATURES.md declares a source path that no longer resolves, but the identifiers it quotes are present in the tree: `expr-lang/expr`.
- `CAP-EXEC-009` — Status established by hand audit of the v0.6 scope set: internal/controlplane/batch_dispatcher.go.
- `CAP-EXEC-014` — FEATURES.md declares no source path; the capability is evidenced by `cmd/kscorectl/`.
- `CAP-EXEC-014` — Status established by hand audit of the v0.6 scope set: All seven subcommands exist under internal/cli/exec/; only the declared cmd/kscore-exec/ binary is stale, the commands having been folded into kscorectl.
- `CAP-EXEC-014` — RFC 0001 admits run, status, output and cancel. The async, list and script subcommands named here involve batch fan-out and script execution, which the RFC places outside the first-release boundary; admitting them requires an RFC amendment.
- `CAP-EXEC-019` — Status established by hand audit of the v0.6 scope set: Cancel persists CANCELLED server-side only; agent-side in-flight processes run to completion.
- `CAP-EXEC-027` — The part of `CAP-EXEC-014` that RFC 0001 places outside the argv-only, non-interactive execution boundary. Catalogued separately so it is retained as a Future candidate rather than falling outside both buckets.

Known gaps and limitations:

- `CAP-EXEC-010` — Cancellation persists CANCELLED server-side; the signal never reaches the agent process, as CAP-EXEC-019 records from the same archived source.
- `CAP-EXEC-016` — Percentage-based / rolling batch execution.
- `CAP-EXEC-017` — Output archival to object storage cold-tier.
- `CAP-EXEC-019` — Referenced code exists, but this backlog item describes work that had not landed: `kscorectl exec cancel &lt;id>` mid-batch results in agent-side processes receiving SIGTERM (then SIGKILL after KillGrace); per-agent batch_agent_result rows record the cancelled stat.
- `CAP-EXEC-020` — Referenced code exists, but this backlog item describes work that had not landed: Batch dispatches write to a single row family (TBD: extend batch_agent_results, OR keep commands and drop batch_agent_results, OR linked via a `commands.batch_id` FK). Pick depends.
- `CAP-EXEC-021` — Referenced code exists, but this backlog item describes work that had not landed: `kscorectl exec run "uptime" --target "os:linux AND (role:web OR role:cache)"` resolves correctly against a stretched fleet; the AND-of-labels-plus-hostname-glob path remains a val.
- `CAP-EXEC-022` — Referenced code exists, but this backlog item describes work that had not landed: `BatchJobStore.DeleteBatchJobsBefore(t)` deletes both `batch_jobs` rows and dependent `batch_agent_results` rows in a single tx (or cascade); `BatchDispatcher` has a configurable r.
- `CAP-EXEC-023` — A long-running command (e.g., `tail -F`) streams partial stdout to a connected `kscorectl exec` over gRPC; agent disconnect mid-stream surfaces as `AGENT_FAILED` with the partial b.
- `CAP-EXEC-024` — Referenced code exists, but this backlog item describes work that had not landed: A target expression with a literal-asterisk pattern (`role:*`-with-meta-on-empty-value) or a bad CIDR logs once per dispatch with the offending pattern, evaluation count, and targe.

### State management and standard library

| ID | Capability | Status | Scope | Source |
|---|---|---|---|---|
| `CAP-STATE-001` | Declarative state DSL | implemented | Future | `FEATURES.md L265` |
| `CAP-STATE-002` | Module interface | unknown | Future | `FEATURES.md L266` |
| `CAP-STATE-003` | Requisite system | unknown | Future | `FEATURES.md L267` |
| `CAP-STATE-004` | DAG resolver with cycle detection | unknown | Future | `FEATURES.md L268` |
| `CAP-STATE-005` | Go template rendering | unknown | Future | `FEATURES.md L269` |
| `CAP-STATE-006` | Drift detection with severity | unknown | Future | `FEATURES.md L270` |
| `CAP-STATE-007` | Cross-platform dispatch | unknown | Future | `FEATURES.md L271` |
| `CAP-STATE-008` | Dry-run / check mode | unknown | Future | `FEATURES.md L272` |
| `CAP-STATE-009` | Audit + event emission per state apply | unknown | Future | `FEATURES.md L273` |
| `CAP-STATE-010` | History store | implemented | Future | `FEATURES.md L274` |
| `CAP-STATE-011` | State runner pipeline | unknown | Future | `FEATURES.md L275` |
| `CAP-STATE-012` | Saga / checkpoint integration | unknown | Future | `FEATURES.md L276` |
| `CAP-STATE-013` | Container modules | planned | Future | `FEATURES.md L294` |
| `CAP-STATE-014` | Web servers | planned | Future | `FEATURES.md L295` |
| `CAP-STATE-015` | Database admin | planned | Future | `FEATURES.md L296` |
| `CAP-STATE-016` | Windows-native modules | planned | Future | `FEATURES.md L300` |
| `CAP-STATE-017` | macOS-specific scheduling | planned | Future | `FEATURES.md L301` |
| `CAP-STATE-018` | Kubernetes modules | planned | Future | `FEATURES.md L302` |
| `CAP-STATE-019` | DNS provider modules | planned | Future | `FEATURES.md L303` |
| `CAP-STATE-020` | Niche networking | planned | Future | `FEATURES.md L304` |
| `CAP-STATE-021` | Vendor-specific modules | planned | Future | `FEATURES.md L305` |
| `CAP-STATE-022` | service stdlib module — OpenRC / sysvinit / launchd backends | partial | Future | `docs/project/ROADMAP.md L41` |
| `CAP-STATE-023` | firewalld stdlib module — whole-zone management, masquerade/forward-port, direct rules | partial | Future | `docs/project/ROADMAP.md L64` |
| `CAP-STATE-024` | firewall abstraction — deny action, IPv6 on iptables, chain/table overrides, service catalog expansion | implemented | Future | `docs/project/ROADMAP.md L77` |
| `CAP-STATE-025` | security stdlib module — AppArmor, SELinux file contexts / ports / modules / logins, absent semantics | partial | Future | `docs/project/ROADMAP.md L92` |
| `CAP-STATE-026` | lvm stdlib module — existing-VG PV-set mgmt, LV resize, metadata, thin/cache/snapshot | partial | Future | `docs/project/ROADMAP.md L120` |
| `CAP-STATE-027` | bond stdlib module — in-place attribute / member reconciliation, persistent configuration, slave attributes | planned | Future | `docs/project/ROADMAP.md L178` |
| `CAP-STATE-028` | Cross-distro state stdlib docker matrix harness | partial | Future | `docs/project/ROADMAP.md L216` |
| `CAP-STATE-029` | Salt-faithful prereq direction in statemgmt resolver | planned | Future | `docs/project/ROADMAP.md L480` |
| `CAP-STATE-030` | git stdlib module — authentication, submodules, advanced clone | planned | Future | `docs/project/ROADMAP.md L800` |
| `CAP-STATE-031` | link stdlib module — relative-target normalisation | planned | Future | `docs/project/ROADMAP.md L813` |
| `CAP-STATE-032` | cron stdlib module — per-field schedule, cron.d, env lines | planned | Future | `docs/project/ROADMAP.md L820` |
| `CAP-STATE-033` | config stdlib module — more formats, separators, uncomment-aware updates | planned | Future | `docs/project/ROADMAP.md L844` |
| `CAP-STATE-034` | at stdlib module — replace-on-change, per-user queues, batch | planned | Future | `docs/project/ROADMAP.md L874` |
| `CAP-STATE-035` | kscorectl state apply tests/webserver.yaml --target role:web applies on matching agents | implemented | Future | `epics/08-state-management.md L82` |
| `CAP-STATE-036` | Idempotency verified: same state apply twice produces zero changes on second run for every module | implemented | Future | `epics/08-state-management.md L87` |
| `CAP-STATE-037` | Coverage >80% per stdlib module; >85% on engine | implemented | Future | `epics/08-state-management.md L88` |
| `CAP-STATE-038` | Requisite cycles detected with full cycle path in error message | implemented | Future | `epics/08-state-management.md L89` |
| `CAP-STATE-039` | Domain overview and architecture notes | reference | Future | `PROJECT-DETAILS.md L543` |
| `CAP-STATE-040` | Status taxonomy | reference | Future | `docs/project/STATE-SUPPORT-MATRIX.md L16` |
| `CAP-STATE-041` | Parameters every module shares | reference | Future | `docs/project/STATE-SUPPORT-MATRIX.md L39` |
| `CAP-STATE-042` | file | implemented | Future | `docs/project/STATE-SUPPORT-MATRIX.md L103` |
| `CAP-STATE-043` | link | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L117` |
| `CAP-STATE-044` | archive | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L130` |
| `CAP-STATE-045` | config | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L144` |
| `CAP-STATE-046` | git | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L161` |
| `CAP-STATE-047` | package | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L181` |
| `CAP-STATE-048` | langpkg | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L197` |
| `CAP-STATE-049` | service | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L215` |
| `CAP-STATE-050` | cron | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L233` |
| `CAP-STATE-051` | at | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L246` |
| `CAP-STATE-052` | systemd_timer | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L259` |
| `CAP-STATE-053` | user | implemented | Future | `docs/project/STATE-SUPPORT-MATRIX.md L278` |
| `CAP-STATE-054` | group | implemented | Future | `docs/project/STATE-SUPPORT-MATRIX.md L298` |
| `CAP-STATE-055` | ssh | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L307` |
| `CAP-STATE-056` | sysctl | implemented | Future | `docs/project/STATE-SUPPORT-MATRIX.md L325` |
| `CAP-STATE-057` | kmod | implemented | Future | `docs/project/STATE-SUPPORT-MATRIX.md L336` |
| `CAP-STATE-058` | hostname | implemented | Future | `docs/project/STATE-SUPPORT-MATRIX.md L344` |
| `CAP-STATE-059` | timezone | implemented | Future | `docs/project/STATE-SUPPORT-MATRIX.md L349` |
| `CAP-STATE-060` | system | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L354` |
| `CAP-STATE-061` | swap | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L371` |
| `CAP-STATE-062` | disk | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L388` |
| `CAP-STATE-063` | mount | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L403` |
| `CAP-STATE-064` | lvm | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L420` |
| `CAP-STATE-065` | network | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L447` |
| `CAP-STATE-066` | route | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L463` |
| `CAP-STATE-067` | bond | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L479` |
| `CAP-STATE-068` | bridge | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L494` |
| `CAP-STATE-069` | vlan | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L507` |
| `CAP-STATE-070` | firewall | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L526` |
| `CAP-STATE-071` | firewalld | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L541` |
| `CAP-STATE-072` | iptables | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L556` |
| `CAP-STATE-073` | nftables | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L572` |
| `CAP-STATE-074` | security | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L592` |
| `CAP-STATE-075` | pki | implemented | Future | `docs/project/STATE-SUPPORT-MATRIX.md L614` |
| `CAP-STATE-076` | command | partial | Future | `docs/project/STATE-SUPPORT-MATRIX.md L642` |

Known gaps and limitations:

- `CAP-STATE-009` — `state.apply.skip` event taxonomy + wiring.
- `CAP-STATE-012` — Saga/checkpoint advanced features.
- `CAP-STATE-022` — `package` stdlib module — dnf, apk, zypper, pacman backends.
- `CAP-STATE-022` — `systemd_timer` stdlib module — generated service, user timers, more [Timer] knobs.
- `CAP-STATE-023` — `route` stdlib module — persistent configuration, route attributes, source-routing rules, multipath.
- `CAP-STATE-023` — `ssh` stdlib module — key validation, options-set compare, whole-file management.
- `CAP-STATE-023` — `iptables` stdlib module — structured rules, ordering, both-family, distro persistence.
- `CAP-STATE-023` — `nftables` stdlib module — structured rules, ordering, table/chain management, comment matching.
- `CAP-STATE-025` — `system` stdlib module — reboot disconnect-tolerance, cross-distro reboot detection, locale dual-file, absent semantics for reboot/locale.
- `CAP-STATE-026` — an `lv:` decl with a new size larger than the live LV runs `lvextend` (with `--resizefs` when `resize_fs: true`) **(met — LV resize landed)**; a `vg:` decl with a different `pvs:`.
- `CAP-STATE-026` — `disk` stdlib module — partition mgmt, resize, label/UUID, encryption, fstype catalog expansion.
- `CAP-STATE-026` — `network` stdlib module — boot-survive configuration, per-family addresses, DNS / NTP / domain mgmt.
- `CAP-STATE-026` — `swap` stdlib module — UUID sources, resize, fallocate, custom opts.
- `CAP-STATE-027` — Referenced code exists, but this backlog item describes work that had not landed: a `mode:` change on a live bond reconciles via `echo &lt;mode> > /sys/class/net/&lt;bond>/bonding/mode` (or down-up-cycle if required) and reports the before→after in the Diff; `members:.
- `CAP-STATE-027` — `bridge` stdlib module — in-place attribute / port reconciliation, per-port attributes, persistent configuration.
- `CAP-STATE-027` — `vlan` stdlib module — in-place attribute reconciliation, QinQ, VLAN ranges, persistent configuration.
- `CAP-STATE-028` — The backlog item records this as partly landed without naming what remains.
- `CAP-STATE-029` — Referenced code exists, but this backlog item describes work that had not landed: Resolver applies a per-key direction policy where `prereq` and `prereq_in` use the Salt-faithful convention while keeping `require` / `watch` / `onchanges` (and their `_in` variant.
- `CAP-STATE-030` — Referenced code exists, but this backlog item describes work that had not landed: a `credentials:` / `identity_file:` param flows an SSH key or credential helper into the clone/fetch; `submodules: true` recurses; the integration test clones a private repo over S.
- `CAP-STATE-031` — Referenced code exists, but this backlog item describes work that had not landed: `link` stdlib module — relative-target normalisation.
- `CAP-STATE-032` — Referenced code exists, but this backlog item describes work that had not landed: `minute: "*/5"` etc. compose into a schedule; `cron_d: true` writes `/etc/cron.d/&lt;name>` with a `user` column; an `env:` map emits `KEY=value` lines above the entry; malformed fiel.
- `CAP-STATE-033` — Referenced code exists, but this backlog item describes work that had not landed: `case_insensitive: true` matches `Key`/`key`; `separator: " "` round-trips an `sshd_config` directive; an inline `# comment` survives a value change; setting a directive that exist.
- `CAP-STATE-033` — `archive` stdlib module — absent state, clean mode, safe symlinks, more formats.
- `CAP-STATE-033` — `x509` stdlib module — combined PEM, encrypted keys, more issuance options.
- `CAP-STATE-033` — `langpkg` stdlib module — per-user/per-project installs, lockfiles, semver ranges, more ecosystems.
- `CAP-STATE-034` — Referenced code exists, but this backlog item describes work that had not landed: a `present` declaration whose command changed re-queues the job (old one removed); a `user:` param queues a job for that user; `batch: true` submits via `batch`; the queue scan can.
- `CAP-STATE-034` — `mount` stdlib module — remount-on-change, escaping, swap, crypttab.
- `CAP-STATE-043` — The support matrix lists parameters not yet supported for this module.
- `CAP-STATE-044` — The support matrix lists parameters not yet supported for this module.
- `CAP-STATE-045` — The support matrix lists parameters not yet supported for this module.
- `CAP-STATE-046` — The support matrix lists parameters not yet supported for this module.
- `CAP-STATE-047` — The support matrix marks some parameters of this module experimental rather than stable.
- `CAP-STATE-048` — The support matrix lists parameters not yet supported for this module.
- `CAP-STATE-049` — The support matrix marks some parameters of this module experimental rather than stable.
- `CAP-STATE-050` — The support matrix lists parameters not yet supported for this module.
- `CAP-STATE-051` — The support matrix lists parameters not yet supported for this module.
- `CAP-STATE-052` — The support matrix lists parameters not yet supported for this module.
- `CAP-STATE-055` — The support matrix lists parameters not yet supported for this module.
- `CAP-STATE-060` — The support matrix lists parameters not yet supported for this module.
- `CAP-STATE-061` — The support matrix lists parameters not yet supported for this module.
- `CAP-STATE-062` — The support matrix lists parameters not yet supported for this module.
- `CAP-STATE-063` — The support matrix lists parameters not yet supported for this module.
- `CAP-STATE-064` — The support matrix lists parameters not yet supported for this module.
- `CAP-STATE-065` — The support matrix lists parameters not yet supported for this module.
- `CAP-STATE-066` — The support matrix lists parameters not yet supported for this module.
- `CAP-STATE-067` — The support matrix lists parameters not yet supported for this module.
- `CAP-STATE-068` — The support matrix lists parameters not yet supported for this module.
- `CAP-STATE-069` — The support matrix lists parameters not yet supported for this module.
- `CAP-STATE-070` — The support matrix lists parameters not yet supported for this module.
- `CAP-STATE-071` — The support matrix lists parameters not yet supported for this module.
- `CAP-STATE-072` — The support matrix lists parameters not yet supported for this module.
- `CAP-STATE-073` — The support matrix lists parameters not yet supported for this module.
- `CAP-STATE-074` — The support matrix lists parameters not yet supported for this module.
- `CAP-STATE-076` — The support matrix marks some parameters of this module experimental rather than stable.

### Events and automation

| ID | Capability | Status | Scope | Source |
|---|---|---|---|---|
| `CAP-EVENT-001` | Event bus on NATS JetStream | implemented | Future | `FEATURES.md L313` |
| `CAP-EVENT-002` | Event types | unknown | Future | `FEATURES.md L314` |
| `CAP-EVENT-003` | Event struct | unknown | Future | `FEATURES.md L315` |
| `CAP-EVENT-004` | EventStore | unknown | Future | `FEATURES.md L316` |
| `CAP-EVENT-005` | EventPublisher / EventSubscriber interfaces | unknown | Future | `FEATURES.md L317` |
| `CAP-EVENT-006` | Filter expressions | implemented | Future | `FEATURES.md L318` |
| `CAP-EVENT-007` | gRPC EventService | unknown | Future | `FEATURES.md L319` |
| `CAP-EVENT-008` | CLI | partial | Future | `FEATURES.md L320` |
| `CAP-EVENT-009` | Audit emission integration | implemented | Future | `FEATURES.md L321` |
| `CAP-EVENT-010` | Retention policies | implemented | Future | `FEATURES.md L322` |
| `CAP-EVENT-011` | Correlation IDs | unknown | Future | `FEATURES.md L323` |
| `CAP-EVENT-012` | Severity levels | unknown | Future | `FEATURES.md L324` |
| `CAP-EVENT-013` | Reactor engine | planned | Future | `FEATURES.md L328` |
| `CAP-EVENT-014` | Lifecycle tracking | planned | Future | `FEATURES.md L329` |
| `CAP-EVENT-015` | Enrichment pipeline | planned | Future | `FEATURES.md L330` |
| `CAP-EVENT-016` | Dead-letter queue | planned | Future | `FEATURES.md L331` |
| `CAP-EVENT-017` | Kafka integration | planned | Future | `FEATURES.md L335` |
| `CAP-EVENT-018` | CloudEvents 1.0 marshaling | planned | Future | `FEATURES.md L336` |
| `CAP-EVENT-019` | Inbound webhook receiver for events | planned | Future | `FEATURES.md L337` |
| `CAP-EVENT-020` | Object-storage archival (S3/GCS) | planned | Future | `FEATURES.md L338` |
| `CAP-EVENT-021` | Multi-region replication | planned | Future | `FEATURES.md L339` |
| `CAP-EVENT-022` | Policy enforcement side-effects (Warn events + Enforce violation handlers) | planned | Future | `docs/project/ROADMAP.md L652` |
| `CAP-EVENT-023` | kscore-events retention subcommand | planned | Future | `docs/project/ROADMAP.md L1262` |
| `CAP-EVENT-024` | Strict audit-on-access via Auditor.Emit error return | planned | Future | `docs/project/ROADMAP.md L1302` |
| `CAP-EVENT-025` | Slow consumer (handler >30s) triggers redelivery up to 3 times | implemented | Future | `epics/11-events.md L75` |
| `CAP-EVENT-026` | Coverage >80% on internal/events | implemented | Future | `epics/11-events.md L76` |
| `CAP-EVENT-027` | Domain overview and architecture notes | reference | Future | `PROJECT-DETAILS.md L636` |

Scope and status notes:

- `CAP-EVENT-006` — FEATURES.md declares a source path that no longer resolves, but the identifiers it quotes are present in the tree: `google/cel-go`.
- `CAP-EVENT-008` — FEATURES.md declares no source path; the capability is evidenced by `cmd/kscore-events/`.
- `CAP-EVENT-009` — Catalogued twice; evidence for this capability sits on `CAP-SECRET-008`.

Known gaps and limitations:

- `CAP-EVENT-008` — kscore-events query subcommand (CEL post-filter).
- `CAP-EVENT-022` — Referenced code exists, but this backlog item describes work that had not landed: `policy.enforcement_enabled=true` is operator-settable (config plumbing); `Warn` mode emits a `policy.warn` (or §4.9-canonical) event through the events bus on a denying verdict an.
- `CAP-EVENT-023` — Referenced code exists, but this backlog item describes work that had not landed: `kscore-events retention show` prints the active policy table; `kscore-events retention apply --type agent.heartbeat --max-age 24h --max-count 10000` invokes a one-shot retention p.
- `CAP-EVENT-023` — kscore-events analyze subcommand.
- `CAP-EVENT-024` — Referenced code exists, but this backlog item describes work that had not landed: `secrets.Auditor.Emit` returns `error`; the broker has an `AuditFailurePolicy` config (`fail_open` / `fail_closed`); the bridge propagates publish errors; integration test confirms.

### Identity and authorization

| ID | Capability | Status | Scope | Source |
|---|---|---|---|---|
| `CAP-IDENT-001` | API keys | implemented | Future | `FEATURES.md L347` |
| `CAP-IDENT-002` | API key rotation + revocation | implemented | Future | `FEATURES.md L348` |
| `CAP-IDENT-003` | mTLS | implemented | Future | `FEATURES.md L349` |
| `CAP-IDENT-004` | Embedded CA | implemented | Future | `FEATURES.md L350` |
| `CAP-IDENT-005` | SPIFFE-shaped identities from day 1 | implemented | Future | `FEATURES.md L351` |
| `CAP-IDENT-006` | Embedded identity provider | unknown | Future | `FEATURES.md L352` |
| `CAP-IDENT-007` | JWT | unknown | Future | `FEATURES.md L353` |
| `CAP-IDENT-008` | Cluster join tokens | implemented | Future | `FEATURES.md L354` |
| `CAP-IDENT-009` | RBAC | planned | Future | `FEATURES.md L355` |
| `CAP-IDENT-010` | Auth interceptor chain | unknown | Future | `FEATURES.md L356` |
| `CAP-IDENT-011` | kscore-identity CLI | implemented | Future | `FEATURES.md L357` |
| `CAP-IDENT-012` | Cert auto-rotation at ~50% lifetime | unknown | Future | `FEATURES.md L358` |
| `CAP-IDENT-013` | TLS 1.3 default | implemented | v0.6 | `FEATURES.md L359` |
| `CAP-IDENT-014` | Full RBAC role/permission CRUD with per-resource permissions | planned | Future | `FEATURES.md L363` |
| `CAP-IDENT-015` | Trust federation | planned | Future | `FEATURES.md L364` |
| `CAP-IDENT-016` | SPIRE integration | planned | Future | `FEATURES.md L365` |
| `CAP-IDENT-017` | Cloud workload identity | planned | Future | `FEATURES.md L369` |
| `CAP-IDENT-018` | Service mesh integration | planned | Future | `FEATURES.md L370` |
| `CAP-IDENT-019` | Multi-party CA / certificate issuance | planned | Future | `FEATURES.md L371` |
| `CAP-IDENT-020` | Encrypt CA material at rest | partial | Future | `docs/project/ROADMAP.md L224` |
| `CAP-IDENT-021` | Pre-rotation signing-CA retention in the bootstrap trust bundle | planned | Future | `docs/project/ROADMAP.md L388` |
| `CAP-IDENT-022` | Direct signing-cert accessor on CAManager | planned | Future | `docs/project/ROADMAP.md L768` |
| `CAP-IDENT-023` | Join-token "any agent" mode (AgentID-less binding) | planned | Future | `docs/project/ROADMAP.md L776` |
| `CAP-IDENT-024` | kscore-identity federation subcommands | planned | Future | `docs/project/ROADMAP.md L1334` |
| `CAP-IDENT-025` | SPIRE-backed identity provider | planned | Future | `docs/project/ROADMAP.md L1423` |
| `CAP-IDENT-026` | TLS 1.3 enforced on gRPC; 1.2 only when explicitly configured | implemented | Future | `epics/09-identity-auth.md L75` |
| `CAP-IDENT-027` | Domain overview and architecture notes | reference | Future | `PROJECT-DETAILS.md L693` |

Scope and status notes:

- `CAP-IDENT-009` — FEATURES.md declares source path(s) that are absent from the final tree: pkg/api/rbac/
- `CAP-IDENT-013` — Status established by hand audit of the v0.6 scope set: internal/identity/tls.go pins VersionTLS13.

Known gaps and limitations:

- `CAP-IDENT-009` — Full RBAC role/permission CRUD.
- `CAP-IDENT-020` — met — `EncryptedFileCAStorage` round-trips with `FileCAStorage` (key-only encryption; wrong-key + tamper rejected via the envelope's fingerprint guard + GCM auth); the `ca encrypt`.
- `CAP-IDENT-021` — Referenced code exists, but this backlog item describes work that had not landed: `CAManager.RotateSigningCA` records prior signing CAs into a retention list; `BuildTrustBundle` includes the active root + retained pre-rotation signing CAs; retention window is co.
- `CAP-IDENT-022` — Referenced code exists, but this backlog item describes work that had not landed: `IdentityGRPCServer.ExportCA WHAT_SIGNING` returns the same PEM without invoking `IssueCertificate`; `GetStatus.SigningExpiresAt` reads from `SigningCAExpiresAt()`; the throwaway-p.
- `CAP-IDENT-023` — Referenced code exists, but this backlog item describes work that had not landed: a `kscore-identity token create --any-agent` creates a token with empty `AgentID`; agents pass `--agent-id agent-1` on attestation; the attestor binds the claimed AgentID atomicall.
- `CAP-IDENT-024` — Referenced code exists, but this backlog item describes work that had not landed: `kscore-identity federation add-domain spiffe://peer.example.org/` registers + fetches the peer bundle; `kscore-identity federation list` shows registered domains + last-refresh ti.
- `CAP-IDENT-025` — The backlog records this as outstanding work; the entry names no reference paths, so the archive shows what was wanted rather than what was built.

### Secrets

| ID | Capability | Status | Scope | Source |
|---|---|---|---|---|
| `CAP-SECRET-001` | Encrypted-file backend | implemented | Future | `FEATURES.md L379` |
| `CAP-SECRET-002` | HashiCorp Vault backend | implemented | Future | `FEATURES.md L380` |
| `CAP-SECRET-003` | SecretBroker with path-prefix routing | implemented | Future | `FEATURES.md L381` |
| `CAP-SECRET-004` | CRUD via REST + gRPC + CLI | implemented | Future | `FEATURES.md L382` |
| `CAP-SECRET-005` | Lease management | implemented | Future | `FEATURES.md L383` |
| `CAP-SECRET-006` | Transit operations | partial | Future | `FEATURES.md L384` |
| `CAP-SECRET-007` | Encrypted in-memory cache | implemented | Future | `FEATURES.md L385` |
| `CAP-SECRET-008` | Audit emission integration | implemented | Future | `FEATURES.md L386` |
| `CAP-SECRET-009` | CLI | partial | Future | `FEATURES.md L387` |
| `CAP-SECRET-010` | Secret masking in API responses + logs | unknown | Future | `FEATURES.md L388` |
| `CAP-SECRET-011` | Rotation orchestration with strategies | planned | Future | `FEATURES.md L392` |
| `CAP-SECRET-012` | Cron-based rotation scheduling + Slack/PagerDuty notifications | planned | Future | `FEATURES.md L393` |
| `CAP-SECRET-013` | Compliance reports + anomaly detection | planned | Future | `FEATURES.md L394` |
| `CAP-SECRET-014` | AWS Secrets Manager backend | planned | Future | `FEATURES.md L398` |
| `CAP-SECRET-015` | Azure Key Vault backend | planned | Future | `FEATURES.md L399` |
| `CAP-SECRET-016` | GCP Secret Manager backend | planned | Future | `FEATURES.md L400` |
| `CAP-SECRET-017` | Cloud KMS for master keys | planned | Future | `FEATURES.md L401` |
| `CAP-SECRET-018` | Hardware HSM support | planned | Future | `FEATURES.md L402` |
| `CAP-SECRET-019` | L2 KMS-backed cache | planned | Future | `FEATURES.md L403` |
| `CAP-SECRET-020` | Vault AWS IAM auth method | planned | Future | `docs/project/ROADMAP.md L512` |
| `CAP-SECRET-021` | Vault auto re-authentication on token expiry | planned | Future | `docs/project/ROADMAP.md L752` |
| `CAP-SECRET-022` | kscore-secrets dynamic subcommand | planned | Future | `docs/project/ROADMAP.md L1238` |
| `CAP-SECRET-023` | Secrets transit GenerateDataKey on the wire | planned | Future | `docs/project/ROADMAP.md L1318` |
| `CAP-SECRET-024` | Domain overview and architecture notes | reference | Future | `PROJECT-DETAILS.md L758` |

Scope and status notes:

- `CAP-SECRET-009` — FEATURES.md declares no source path; the capability is evidenced by `cmd/kscore-secrets/`.

Known gaps and limitations:

- `CAP-SECRET-006` — Secrets transit batch API on the wire.
- `CAP-SECRET-009` — kscore-secrets backends subcommand.
- `CAP-SECRET-009` — kscore-secrets audit subcommand.
- `CAP-SECRET-009` — kscore-secrets cache subcommand.
- `CAP-SECRET-009` — kscore-secrets template subcommand.
- `CAP-SECRET-020` — Referenced code exists, but this backlog item describes work that had not landed: `Auth.Method=aws` with an `AWSAuthConfig{Role, Region, IAMServerIDHeader, ...}` round-trips against a `vault dev` server with the AWS auth method enabled; the AWS SDK dep is brough.
- `CAP-SECRET-021` — Referenced code exists, but this backlog item describes work that had not landed: On `auth/token/renew-self` failure, the renewer re-invokes the configured auth method to obtain a fresh token and resumes the renewal loop on success; method-specific lockout heuri.
- `CAP-SECRET-022` — Referenced code exists, but this backlog item describes work that had not landed: proto adds `IssueDynamicSecret` RPC; CLI subcommand exists; ENG round-trips an actual Vault DB credential round-trip in the integration test.
- `CAP-SECRET-023` — Referenced code exists, but this backlog item describes work that had not landed: `api/proto/keystone/core/v1/secrets.proto` adds `GenerateDataKey` RPC with a `Mode` enum field (`PLAINTEXT` / `WRAPPED`) + `bits` + `context`; the gRPC service forwards to `Transit.

### Audit and policy

| ID | Capability | Status | Scope | Source |
|---|---|---|---|---|
| `CAP-AUDIT-001` | Audit logger | implemented | v0.6 | `FEATURES.md L411` |
| `CAP-AUDIT-002` | Audit storage | partial | v0.6 | `FEATURES.md L412` |
| `CAP-AUDIT-003` | Audit query API | implemented | v0.6 | `FEATURES.md L413` |
| `CAP-AUDIT-004` | Audit export | implemented | Future | `FEATURES.md L414` |
| `CAP-AUDIT-005` | kscore-audit CLI | partial | Future | `FEATURES.md L415` |
| `CAP-AUDIT-006` | Policy engine infrastructure | implemented | Future | `FEATURES.md L416` |
| `CAP-AUDIT-007` | OPA Rego evaluator | implemented | Future | `FEATURES.md L417` |
| `CAP-AUDIT-008` | CEL evaluator | implemented | Future | `FEATURES.md L418` |
| `CAP-AUDIT-009` | Builtin policies | unknown | Future | `FEATURES.md L419` |
| `CAP-AUDIT-010` | Policy{ID, Name, Type, Category, Severity, EnforcementMode, Code, Enabled, Tags} | unknown | Future | `FEATURES.md L420` |
| `CAP-AUDIT-011` | PolicySet | implemented | Future | `FEATURES.md L421` |
| `CAP-AUDIT-012` | Bindings | implemented | Future | `FEATURES.md L422` |
| `CAP-AUDIT-013` | Policy evaluation API | unknown | Future | `FEATURES.md L423` |
| `CAP-AUDIT-014` | Compliance reports | implemented | Future | `FEATURES.md L424` |
| `CAP-AUDIT-015` | Compliance framework mappings | unknown | Future | `FEATURES.md L425` |
| `CAP-AUDIT-016` | PolicyService gRPC | implemented | Future | `FEATURES.md L426` |
| `CAP-AUDIT-017` | kscore-policy CLI v1.0 subset | partial | Future | `FEATURES.md L427` |
| `CAP-AUDIT-018` | Enforcement modes: Enforce + Warn (active blocking) | planned | Future | `FEATURES.md L431` |
| `CAP-AUDIT-019` | Enforcement actions | planned | Future | `FEATURES.md L432` |
| `CAP-AUDIT-020` | Pre/post-execution hooks | planned | Future | `FEATURES.md L433` |
| `CAP-AUDIT-021` | Approval workflows for policy violations | planned | Future | `FEATURES.md L434` |
| `CAP-AUDIT-022` | kscore-policy create\|update\|delete\|activate\|deactivate\|remediate\|monitor | partial | Future | `FEATURES.md L435` |
| `CAP-AUDIT-023` | Policy persistence | planned | Future | `FEATURES.md L436` |
| `CAP-AUDIT-024` | Continuous compliance scan scheduler | planned | Future | `FEATURES.md L440` |
| `CAP-AUDIT-025` | CEL custom function library | planned | Future | `FEATURES.md L441` |
| `CAP-AUDIT-026` | Anomaly detection (audit log analysis) | planned | Future | `FEATURES.md L442` |
| `CAP-AUDIT-027` | Module system boot wiring (loader PolicyChecker/Hosts/trust-policy + runtime registration) | planned | Future | `docs/project/ROADMAP.md L251` |
| `CAP-AUDIT-028` | Rate-limit: Auditor hook for rejections | planned | Future | `docs/project/ROADMAP.md L1190` |
| `CAP-AUDIT-029` | Redaction strips configured patterns (e.g., password=*) from exported metadata | implemented | Future | `epics/12-audit-policy.md L105` |
| `CAP-AUDIT-030` | v1.0 critical: even when policies return Allowed=false, operations still proceed (audit-mode-only) | implemented | Future | `epics/12-audit-policy.md L110` |
| `CAP-AUDIT-031` | CRUD RPCs return Unimplemented cleanly (clients can probe) | implemented | Future | `epics/12-audit-policy.md L111` |
| `CAP-AUDIT-032` | Domain overview and architecture notes | reference | Future | `PROJECT-DETAILS.md L830` |

Scope and status notes:

- `CAP-AUDIT-001` — FEATURES.md declares a source path that no longer resolves, but the identifiers it quotes are present in the tree: `internal/audit/`.
- `CAP-AUDIT-003` — FEATURES.md declares a source path that no longer resolves, but the identifiers it quotes are present in the tree: `AuditFilter`.
- `CAP-AUDIT-003` — Status established by hand audit of the v0.6 scope set: internal/audit/.
- `CAP-AUDIT-005` — FEATURES.md declares no source path; the capability is evidenced by `cmd/kscore-audit/`.
- `CAP-AUDIT-007` — FEATURES.md declares a source path that no longer resolves, but the identifiers it quotes are present in the tree: `open-policy-agent/opa`, `v1/rego`.
- `CAP-AUDIT-008` — FEATURES.md declares a source path that no longer resolves, but the identifiers it quotes are present in the tree: `google/cel-go`.
- `CAP-AUDIT-011` — FEATURES.md declares a source path that no longer resolves, but the identifiers it quotes are present in the tree: `PolicySet`.
- `CAP-AUDIT-012` — FEATURES.md declares a source path that no longer resolves, but the identifiers it quotes are present in the tree: `Bindings`.
- `CAP-AUDIT-016` — FEATURES.md declares a source path that no longer resolves, but the identifiers it quotes are present in the tree: `PolicyService`.
- `CAP-AUDIT-017` — FEATURES.md declares no source path; the capability is evidenced by `cmd/kscore-policy/`.
- `CAP-AUDIT-022` — FEATURES.md declares no source path; the capability is evidenced by `cmd/kscore-policy/`.

Known gaps and limitations:

- `CAP-AUDIT-002` — kscore-audit export `--redaction-config` file.
- `CAP-AUDIT-005` — kscore-audit search subcommand.
- `CAP-AUDIT-005` — kscore-audit analyze / timeline / watch subcommands.
- `CAP-AUDIT-017` — kscore-policy check / test subcommands.
- `CAP-AUDIT-018` — Policy enforcement (Enforce + Warn modes).
- `CAP-AUDIT-022` — Policy CRUD via gRPC (`CreatePolicy`/`UpdatePolicy`/`DeletePolicy`/`activate`/`deactivate`/`remediate`/`monitor`).
- `CAP-AUDIT-027` — Referenced code exists, but this backlog item describes work that had not landed: Module system boot wiring (loader PolicyChecker/Hosts/trust-policy + runtime registration).
- `CAP-AUDIT-028` — Referenced code exists, but this backlog item describes work that had not landed: New `WithAuditor(fn)` option on the HTTP middleware + gRPC interceptor; auditor fires synchronously on each deny carrying key + reason + observed RPS.

### GitOps

| ID | Capability | Status | Scope | Source |
|---|---|---|---|---|
| `CAP-GITOPS-001` | Webhook receiver | implemented | Future | `FEATURES.md L450` |
| `CAP-GITOPS-002` | ArgoCD webhook handler | unknown | Future | `FEATURES.md L451` |
| `CAP-GITOPS-003` | Flux webhook handler | unknown | Future | `FEATURES.md L452` |
| `CAP-GITOPS-004` | GitHub webhook handler | implemented | Future | `FEATURES.md L453` |
| `CAP-GITOPS-005` | GitLab webhook handler | unknown | Future | `FEATURES.md L454` |
| `CAP-GITOPS-006` | Webhook authentication | unknown | Future | `FEATURES.md L455` |
| `CAP-GITOPS-007` | Event normalization | unknown | Future | `FEATURES.md L456` |
| `CAP-GITOPS-008` | Verification engine | implemented | Future | `FEATURES.md L457` |
| `CAP-GITOPS-009` | Verification workflow execution | implemented | Future | `FEATURES.md L458` |
| `CAP-GITOPS-010` | Manual rollback API + CLI | implemented | Future | `FEATURES.md L459` |
| `CAP-GITOPS-011` | Rollback executors | partial | Future | `FEATURES.md L460` |
| `CAP-GITOPS-012` | Approval workflow for rollback | unknown | Future | `FEATURES.md L461` |
| `CAP-GITOPS-013` | Verification result storage + REST list/get | implemented | Future | `FEATURES.md L462` |
| `CAP-GITOPS-014` | kscore-gitops CLI | implemented | Future | `FEATURES.md L463` |
| `CAP-GITOPS-015` | Multi-env promotion pipelines | planned | Future | `FEATURES.md L467` |
| `CAP-GITOPS-016` | Promotion state machine + REST API | planned | Future | `FEATURES.md L468` |
| `CAP-GITOPS-017` | Basic remediation strategies | planned | Future | `FEATURES.md L469` |
| `CAP-GITOPS-018` | Canary deployments | planned | Future | `FEATURES.md L473` |
| `CAP-GITOPS-019` | Threshold evaluation per canary step | planned | Future | `FEATURES.md L474` |
| `CAP-GITOPS-020` | Advanced remediation | planned | Future | `FEATURES.md L475` |
| `CAP-GITOPS-021` | Diagnostic collection on remediation | planned | Future | `FEATURES.md L476` |
| `CAP-GITOPS-022` | Git sync orchestration + multi-repo coordination | planned | Future | `FEATURES.md L477` |
| `CAP-GITOPS-023` | Helm/Kustomize-native integration | planned | Future | `FEATURES.md L478` |
| `CAP-GITOPS-024` | Deployment dependency graph | planned | Future | `FEATURES.md L479` |
| `CAP-GITOPS-025` | Webhook timestamp validation + nonce dedup | planned | Future | `FEATURES.md L480` |
| `CAP-GITOPS-026` | Outbound webhook Manager boot wiring (kscore-server) | planned | Future | `docs/project/ROADMAP.md L379` |
| `CAP-GITOPS-027` | HMAC signature validates on receiver side | implemented | Future | `epics/16-gitops-webhooks.md L113` |
| `CAP-GITOPS-028` | Domain overview and architecture notes | reference | Future | `PROJECT-DETAILS.md L888` |

Scope and status notes:

- `CAP-GITOPS-009` — FEATURES.md declares a source path that no longer resolves, but the identifiers it quotes are present in the tree: `Verifier`.
- `CAP-GITOPS-010` — FEATURES.md declares no source path; the capability is evidenced by `cmd/kscore-gitops/`.
- `CAP-GITOPS-013` — FEATURES.md declares a source path that no longer resolves, but the identifiers it quotes are present in the tree: `/api/v1/gitops/verifications`.
- `CAP-GITOPS-014` — FEATURES.md declares no source path; the capability is evidenced by `cmd/kscore-gitops/`.

Known gaps and limitations:

- `CAP-GITOPS-011` — K8s rollout-undo client-go adapter + GitOps rollback boot wiring.
- `CAP-GITOPS-026` — Referenced code exists, but this backlog item describes work that had not landed: Outbound webhook Manager boot wiring (kscore-server).

### Outbound webhooks and integrations

| ID | Capability | Status | Scope | Source |
|---|---|---|---|---|
| `CAP-HOOK-001` | Persistent webhook subscriptions | implemented | Future | `FEATURES.md L488` |
| `CAP-HOOK-002` | Event filter on subscriptions | unknown | Future | `FEATURES.md L489` |
| `CAP-HOOK-003` | HMAC-SHA256 signing | unknown | Future | `FEATURES.md L490` |
| `CAP-HOOK-004` | Custom HTTP headers per subscription | unknown | Future | `FEATURES.md L491` |
| `CAP-HOOK-005` | Exponential backoff retry | unknown | Future | `FEATURES.md L492` |
| `CAP-HOOK-006` | Delivery history with audit trail | unknown | Future | `FEATURES.md L493` |
| `CAP-HOOK-007` | Per-endpoint circuit breaker | implemented | Future | `FEATURES.md L494` |
| `CAP-HOOK-008` | Per-subscription delivery timeout | unknown | Future | `FEATURES.md L495` |
| `CAP-HOOK-009` | Secret masking in API responses | unknown | Future | `FEATURES.md L496` |
| `CAP-HOOK-010` | REST API | implemented | Future | `FEATURES.md L497` |
| `CAP-HOOK-011` | kscore-webhook outbound CLI | implemented | Future | `FEATURES.md L498` |
| `CAP-HOOK-012` | NATS event-bus consumer | unknown | Future | `FEATURES.md L499` |
| `CAP-HOOK-013` | Manager-driven async delivery | unknown | Future | `FEATURES.md L500` |
| `CAP-HOOK-014` | Inbound webhooks for non-GitOps event sources | planned | Future | `FEATURES.md L504` |
| `CAP-HOOK-015` | Webhook body templating | planned | Future | `FEATURES.md L505` |
| `CAP-HOOK-016` | Per-destination rate limiting | planned | Future | `FEATURES.md L506` |
| `CAP-HOOK-017` | Auto-cleanup of old delivery history | planned | Future | `FEATURES.md L507` |
| `CAP-HOOK-018` | Domain overview and architecture notes | reference | Future | `PROJECT-DETAILS.md L938` |

Scope and status notes:

- `CAP-HOOK-007` — FEATURES.md declares no source path; the capability is evidenced by `internal/webhook/outbound/circuit_breaker.go`.
- `CAP-HOOK-010` — FEATURES.md declares a source path that no longer resolves, but the identifiers it quotes are present in the tree: `/api/v1/webhooks/subscriptions`.
- `CAP-HOOK-011` — FEATURES.md declares no source path; the capability is evidenced by `cmd/kscore-webhook/`.

### Clustering and high availability

| ID | Capability | Status | Scope | Source |
|---|---|---|---|---|
| `CAP-CLUSTER-001` | 3-node cluster formation | implemented | Future | `FEATURES.md L515` |
| `CAP-CLUSTER-002` | Embedded etcd mode | unknown | Future | `FEATURES.md L516` |
| `CAP-CLUSTER-003` | External etcd mode | unknown | Future | `FEATURES.md L517` |
| `CAP-CLUSTER-004` | etcd-based membership | implemented | Future | `FEATURES.md L518` |
| `CAP-CLUSTER-005` | Heartbeat mechanism | unknown | Future | `FEATURES.md L519` |
| `CAP-CLUSTER-006` | Member status state machine | unknown | Future | `FEATURES.md L520` |
| `CAP-CLUSTER-007` | etcd-based leader election | unknown | Future | `FEATURES.md L521` |
| `CAP-CLUSTER-008` | Voluntary leadership resignation + transfer | unknown | Future | `FEATURES.md L522` |
| `CAP-CLUSTER-009` | Automatic failover | unknown | Future | `FEATURES.md L523` |
| `CAP-CLUSTER-010` | Consistent hashing for agent assignment | implemented | Future | `FEATURES.md L524` |
| `CAP-CLUSTER-011` | Rebalancing on member join/leave | unknown | Future | `FEATURES.md L525` |
| `CAP-CLUSTER-012` | Singleton-task manager (leader-only) | unknown | Future | `FEATURES.md L526` |
| `CAP-CLUSTER-013` | Recovery workflow | unknown | Future | `FEATURES.md L527` |
| `CAP-CLUSTER-014` | Graceful shutdown | planned | Future | `FEATURES.md L528` |
| `CAP-CLUSTER-015` | Split-brain prevention via quorum | unknown | Future | `FEATURES.md L529` |
| `CAP-CLUSTER-016` | Lease + epoch fencing | unknown | Future | `FEATURES.md L530` |
| `CAP-CLUSTER-017` | Server-to-server coordination | partial | Future | `FEATURES.md L531` |
| `CAP-CLUSTER-018` | Health monitor with consecutive-failure threshold | unknown | Future | `FEATURES.md L532` |
| `CAP-CLUSTER-019` | Cluster backup/restore | partial | Future | `FEATURES.md L533` |
| `CAP-CLUSTER-020` | ClusterService gRPC + REST | implemented | Future | `FEATURES.md L534` |
| `CAP-CLUSTER-021` | kscore-cluster CLI | implemented | Future | `FEATURES.md L535` |
| `CAP-CLUSTER-022` | HA resilience tests in CI | implemented | Future | `FEATURES.md L536` |
| `CAP-CLUSTER-023` | Performance targets | implemented | Future | `FEATURES.md L537` |
| `CAP-CLUSTER-024` | Backup automation/scheduling | planned | Future | `FEATURES.md L541` |
| `CAP-CLUSTER-025` | Comprehensive HA dashboard | planned | Future | `FEATURES.md L542` |
| `CAP-CLUSTER-026` | Read-only replicas | planned | Future | `FEATURES.md L543` |
| `CAP-CLUSTER-027` | Auto-scaling | planned | Future | `FEATURES.md L544` |
| `CAP-CLUSTER-028` | Multi-region clustering / federation | planned | Future | `FEATURES.md L548` |
| `CAP-CLUSTER-029` | Dynamic shard splitting under load | planned | Future | `FEATURES.md L549` |
| `CAP-CLUSTER-030` | Advanced topology (gateway / proxy members) | planned | Future | `FEATURES.md L550` |
| `CAP-CLUSTER-031` | Fencing guard wired around server write paths | planned | Future | `docs/project/ROADMAP.md L271` |
| `CAP-CLUSTER-032` | HA E2E multi-process / iptables-partition form | planned | Future | `docs/project/ROADMAP.md L288` |
| `CAP-CLUSTER-033` | Blueprint e2e real docker-compose convergence form | planned | Future | `docs/project/ROADMAP.md L353` |
| `CAP-CLUSTER-034` | GitOps webhook receiver boot registration (kscore-server) | planned | Future | `docs/project/ROADMAP.md L361` |
| `CAP-CLUSTER-035` | kscore-cluster watch subcommand | planned | Future | `docs/project/ROADMAP.md L1053` |
| `CAP-CLUSTER-036` | Restart of failed member: rejoins cluster in &lt;15s; reclaims its shards from shard map | implemented | Future | `epics/13-clustering-ha.md L123` |
| `CAP-CLUSTER-037` | Domain overview and architecture notes | reference | Future | `PROJECT-DETAILS.md L995` |

Scope and status notes:

- `CAP-CLUSTER-017` — FEATURES.md declares a source path that no longer resolves, but the identifiers it quotes are present in the tree: `CoordinationService`.
- `CAP-CLUSTER-020` — FEATURES.md declares a source path that no longer resolves, but the identifiers it quotes are present in the tree: `ClusterService`.

Known gaps and limitations:

- `CAP-CLUSTER-017` — Cluster gRPC services boot registration (ClusterService/CoordinationService + mTLS listener).
- `CAP-CLUSTER-019` — kscore-cluster-backup schedule subcommand.
- `CAP-CLUSTER-031` — Referenced code exists, but this backlog item describes work that had not landed: write-bearing gRPC/REST handlers acquire `FencingManager.Guard(OpWrite)` (release on completion); the HA E2E suite (task 17) verifies a minority partition rejects writes within 1s.
- `CAP-CLUSTER-032` — Referenced code exists, but this backlog item describes work that had not landed: HA E2E multi-process / iptables-partition form.
- `CAP-CLUSTER-033` — Referenced code exists, but this backlog item describes work that had not landed: Blueprint e2e real docker-compose convergence form.
- `CAP-CLUSTER-034` — Referenced code exists, but this backlog item describes work that had not landed: GitOps webhook receiver boot registration (kscore-server).
- `CAP-CLUSTER-035` — Referenced code exists, but this backlog item describes work that had not landed: `kscore-cluster watch [--leadership]` consumes `ClusterServiceClient.WatchMembership`/`WatchLeadership` and renders events until interrupted, like `kscore-events watch`.

### Observability

| ID | Capability | Status | Scope | Source |
|---|---|---|---|---|
| `CAP-OBS-001` | Structured logging | implemented | v0.6 | `FEATURES.md L558` |
| `CAP-OBS-002` | Prometheus metrics registry | implemented | v0.6 | `FEATURES.md L559` |
| `CAP-OBS-003` | /metrics HTTP endpoint | implemented | v0.6 | `FEATURES.md L560` |
| `CAP-OBS-004` | OpenTelemetry tracing | partial | Future | `FEATURES.md L561` |
| `CAP-OBS-005` | Health endpoints | implemented | v0.6 | `FEATURES.md L562` |
| `CAP-OBS-006` | Pre-built Grafana dashboards | implemented | Future | `FEATURES.md L563` |
| `CAP-OBS-007` | pprof profiling endpoints | implemented | Future | `FEATURES.md L564` |
| `CAP-OBS-008` | Correlation ID propagation | implemented | Future | `FEATURES.md L565` |
| `CAP-OBS-009` | kscore-monitor TUI | planned | Future | `FEATURES.md L569` |
| `CAP-OBS-010` | TUI extras | planned | Future | `FEATURES.md L570` |
| `CAP-OBS-011` | Drill-downs, vim navigation, alert bar, connection health indicators, themes, search filters | planned | Future | `FEATURES.md L571` |
| `CAP-OBS-012` | NATS telemetry transport | planned | Future | `FEATURES.md L572` |
| `CAP-OBS-013` | CLI audit logging to syslog/journald | planned | Future | `FEATURES.md L573` |
| `CAP-OBS-014` | kscore-telemetry-gateway standalone service | planned | Future | `FEATURES.md L577` |
| `CAP-OBS-015` | HA gateway | planned | Future | `FEATURES.md L578` |
| `CAP-OBS-016` | Helm chart for gateway | planned | Future | `FEATURES.md L579` |
| `CAP-OBS-017` | Adaptive sampling tied to error metrics | planned | Future | `FEATURES.md L583` |
| `CAP-OBS-018` | pprof visualization UI | planned | Future | `FEATURES.md L584` |
| `CAP-OBS-019` | SIEM export (CEF/LEEF) | planned | Future | `FEATURES.md L585` |
| `CAP-OBS-020` | Real-time alerting from TUI | planned | Future | `FEATURES.md L586` |
| `CAP-OBS-021` | Zipkin tracing exporter: do not freeze into the v1.0 surface | planned | Future | `docs/project/ROADMAP.md L676` |
| `CAP-OBS-022` | Telemetry gateway | planned | Future | `docs/project/ROADMAP.md L1455` |
| `CAP-OBS-023` | Domain overview and architecture notes | reference | Future | `PROJECT-DETAILS.md L1119` |

Scope and status notes:

- `CAP-OBS-001` — Status established by hand audit of the v0.6 scope set: internal/logging/.
- `CAP-OBS-003` — FEATURES.md declares a source path that no longer resolves, but the identifiers it quotes are present in the tree: `/metrics`.
- `CAP-OBS-014` — FEATURES.md declares source path(s) that are absent from the final tree: internal/gateway/, cmd/kscore-telemetry-gateway/

Known gaps and limitations:

- `CAP-OBS-004` — Zipkin tracing exporter: deprecation warning → OTLP.
- `CAP-OBS-009` — TUI monitor (`kscore-monitor`).
- `CAP-OBS-021` — Referenced code exists, but this backlog item describes work that had not landed: by the v1.0 contract freeze, `tracing.exporter: zipkin` is either removed (operators on OTLP) or documented as a non-frozen/experimental option exempt from the SemVer freeze; the O.
- `CAP-OBS-022` — The backlog records this as outstanding work; the entry names no reference paths, so the archive shows what was wanted rather than what was built.

### Blueprints and runbooks

| ID | Capability | Status | Scope | Source |
|---|---|---|---|---|
| `CAP-BLUE-001` | Blueprint manifest format | implemented | Future | `FEATURES.md L594` |
| `CAP-BLUE-002` | Blueprint apply | unknown | Future | `FEATURES.md L595` |
| `CAP-BLUE-003` | Blueprint feature flags + multi-instance namespacing (as:) | unknown | Future | `FEATURES.md L596` |
| `CAP-BLUE-004` | Standard blueprint catalog (~6 v1.0) | implemented | Future | `FEATURES.md L597` |
| `CAP-BLUE-005` | kscore-blueprint CLI | implemented | Future | `FEATURES.md L598` |
| `CAP-BLUE-006` | Blueprint storage | unknown | Future | `FEATURES.md L599` |
| `CAP-BLUE-007` | Runbook YAML model | implemented | Future | `FEATURES.md L600` |
| `CAP-BLUE-008` | Runbook step types (v1.0 subset) | unknown | Future | `FEATURES.md L601` |
| `CAP-BLUE-009` | Runbook step dependencies | unknown | Future | `FEATURES.md L602` |
| `CAP-BLUE-010` | Runbook variable templating | implemented | Future | `FEATURES.md L603` |
| `CAP-BLUE-011` | Runbook execution + status | implemented | Future | `FEATURES.md L604` |
| `CAP-BLUE-012` | Saga coordinator (minimal) | implemented | Future | `FEATURES.md L605` |
| `CAP-BLUE-013` | StateMachine library | implemented | Future | `FEATURES.md L606` |
| `CAP-BLUE-014` | Runbook + blueprint storage | unknown | Future | `FEATURES.md L607` |
| `CAP-BLUE-015` | kscore-schedule CLI + ScheduleService | planned | Future | `FEATURES.md L611` |
| `CAP-BLUE-016` | Maintenance windows + change-window awareness | planned | Future | `FEATURES.md L612` |
| `CAP-BLUE-017` | Schedule + maintenance gRPC + REST APIs | planned | Future | `FEATURES.md L613` |
| `CAP-BLUE-018` | Runbook conditional steps | planned | Future | `FEATURES.md L617` |
| `CAP-BLUE-019` | Per-step approvals + delegations | planned | Future | `FEATURES.md L618` |
| `CAP-BLUE-020` | Manual interventions | planned | Future | `FEATURES.md L619` |
| `CAP-BLUE-021` | Runbook dry-run mode | planned | Future | `FEATURES.md L620` |
| `CAP-BLUE-022` | Rollback step type with auto-compensation | planned | Future | `FEATURES.md L621` |
| `CAP-BLUE-023` | Standard catalog expansion | planned | Future | `FEATURES.md L625` |
| `CAP-BLUE-024` | Saga checkpoint resume | planned | Future | `FEATURES.md L626` |
| `CAP-BLUE-025` | Blueprint signature + signed bundles | implemented | Future | `FEATURES.md L627` |
| `CAP-BLUE-026` | Blueprint mirror for air-gap | planned | Future | `FEATURES.md L628` |
| `CAP-BLUE-027` | Blueprint applied-runs store (durable) | planned | Future | `docs/project/ROADMAP.md L296` |
| `CAP-BLUE-028` | Durable runbook execution store | planned | Future | `docs/project/ROADMAP.md L345` |
| `CAP-BLUE-029` | Blueprint and runbook REST handler packages not mounted | planned | Future | `docs/project/ROADMAP.md L668` |
| `CAP-BLUE-030` | Blueprint dependency cycle detected with cycle path in error | implemented | Future | `epics/15-blueprints-runbooks.md L98` |
| `CAP-BLUE-031` | Domain overview and architecture notes | reference | Future | `PROJECT-DETAILS.md L1163` |

Scope and status notes:

- `CAP-BLUE-005` — FEATURES.md declares no source path; the capability is evidenced by `cmd/kscore-blueprint/`.
- `CAP-BLUE-011` — FEATURES.md declares no source path; the capability is evidenced by `cmd/kscore-runbook/`.
- `CAP-BLUE-012` — FEATURES.md declares a source path that no longer resolves, but the identifiers it quotes are present in the tree: `pkg/saga`.
- `CAP-BLUE-013` — FEATURES.md declares a source path that no longer resolves, but the identifiers it quotes are present in the tree: `pkg/statemachine`.
- `CAP-BLUE-025` — FEATURES.md declares no source path; the capability is evidenced by `cmd/kscore-blueprint/`.

Known gaps and limitations:

- `CAP-BLUE-027` — Referenced code exists, but this backlog item describes work that had not landed: Blueprint applied-runs store (durable).
- `CAP-BLUE-028` — Referenced code exists, but this backlog item describes work that had not landed: Durable runbook execution store.
- `CAP-BLUE-029` — Referenced code exists, but this backlog item describes work that had not landed: `pkg/api/server/options.go` (or equivalent registration site) mounts both `pkg/api/blueprint` and `pkg/api/runbook` handlers alongside the existing domain handlers. The orphaned-im.

### Plugin and module system

| ID | Capability | Status | Scope | Source |
|---|---|---|---|---|
| `CAP-PLUG-001` | Starlark runtime | implemented | Future | `FEATURES.md L638` |
| `CAP-PLUG-002` | Module manifest format | implemented | Future | `FEATURES.md L639` |
| `CAP-PLUG-003` | Capability-based security (9 core capabilities) | partial | Future | `FEATURES.md L640` |
| `CAP-PLUG-004` | Capability scoping | implemented | Future | `FEATURES.md L641` |
| `CAP-PLUG-005` | Cosign signature verification | implemented | Future | `FEATURES.md L642` |
| `CAP-PLUG-006` | SHA-256 content addressing | unknown | Future | `FEATURES.md L643` |
| `CAP-PLUG-007` | Module resolver | implemented | Future | `FEATURES.md L644` |
| `CAP-PLUG-008` | Module lockfile | implemented | Future | `FEATURES.md L645` |
| `CAP-PLUG-009` | Filesystem-backed registry | implemented | Future | `FEATURES.md L646` |
| `CAP-PLUG-010` | Module loader pipeline | implemented | Future | `FEATURES.md L647` |
| `CAP-PLUG-011` | Module audit logging | implemented | Future | `FEATURES.md L648` |
| `CAP-PLUG-012` | Plugin discovery | implemented | Future | `FEATURES.md L649` |
| `CAP-PLUG-013` | Starlark SDK | partial | Future | `FEATURES.md L650` |
| `CAP-PLUG-014` | kscore-module CLI | implemented | Future | `FEATURES.md L651` |
| `CAP-PLUG-015` | Module test framework | implemented | Future | `FEATURES.md L652` |
| `CAP-PLUG-016` | Module policy hooks | unknown | Future | `FEATURES.md L653` |
| `CAP-PLUG-017` | WASM runtime | planned | Future | `FEATURES.md L657` |
| `CAP-PLUG-018` | Rust SDK | planned | Future | `FEATURES.md L658` |
| `CAP-PLUG-019` | Go (TinyGo) SDK | planned | Future | `FEATURES.md L659` |
| `CAP-PLUG-020` | OCI registry backend | planned | Future | `FEATURES.md L660` |
| `CAP-PLUG-021` | S3/GCS/Azure storage backends | planned | Future | `FEATURES.md L661` |
| `CAP-PLUG-022` | kscore-module mirror | implemented | Future | `FEATURES.md L662` |
| `CAP-PLUG-023` | kscore-module update | implemented | Future | `FEATURES.md L663` |
| `CAP-PLUG-024` | SumDB transparency log | planned | Future | `FEATURES.md L667` |
| `CAP-PLUG-025` | Fine-grained capability model | planned | Future | `FEATURES.md L668` |
| `CAP-PLUG-026` | Module vulnerability scanning + SBOM generation | planned | Future | `FEATURES.md L669` |
| `CAP-PLUG-027` | C++ SDK | planned | Future | `FEATURES.md L673` |
| `CAP-PLUG-028` | Federated module registries | planned | Future | `FEATURES.md L674` |
| `CAP-PLUG-029` | Module signing: cosign keyless / Rekor transparency / encrypted cosign keyfile interop | planned | Future | `docs/project/ROADMAP.md L1363` |
| `CAP-PLUG-030` | Module registry publish authentication | planned | Future | `docs/project/ROADMAP.md L1371` |
| `CAP-PLUG-031` | Starlark hard heap-bytes cap | planned | Future | `docs/project/ROADMAP.md L1379` |
| `CAP-PLUG-032` | Module attempting fs.write outside allowed paths fails with clear error + audit entry | implemented | Future | `epics/14-plugin-module-system.md L153` |
| `CAP-PLUG-033` | Re-running install with same lockfile produces identical resolution (reproducible) | implemented | Future | `epics/14-plugin-module-system.md L155` |
| `CAP-PLUG-034` | Cosign signature mismatch causes load to fail | implemented | Future | `epics/14-plugin-module-system.md L156` |
| `CAP-PLUG-035` | kscorectl module dispatches correctly to kscore-module | implemented | Future | `epics/14-plugin-module-system.md L157` |
| `CAP-PLUG-036` | Coverage >80% on pkg/module/ | implemented | Future | `epics/14-plugin-module-system.md L158` |
| `CAP-PLUG-037` | Domain overview and architecture notes | reference | Future | `PROJECT-DETAILS.md L1231` |

Scope and status notes:

- `CAP-PLUG-014` — FEATURES.md declares no source path; the capability is evidenced by `cmd/kscore-module/`.
- `CAP-PLUG-022` — FEATURES.md declares no source path; the capability is evidenced by `cmd/kscore-module/`.
- `CAP-PLUG-023` — FEATURES.md declares no source path; the capability is evidenced by `cmd/kscore-module/`.

Known gaps and limitations:

- `CAP-PLUG-003` — Module test framework: injectable record/replay http/exec/secrets test hosts.
- `CAP-PLUG-013` — Per-capability-call context propagation in the Starlark SDK.
- `CAP-PLUG-029` — Referenced code exists, but this backlog item describes work that had not landed: a module signed by the real `cosign` CLI in keyless mode verifies through an extended trust policy with a Rekor inclusion check; `kscore-module sign` can load a cosign-encrypted `c.
- `CAP-PLUG-030` — Referenced code exists, but this backlog item describes work that had not landed: `POST /publish` requires a valid credential (API key or mTLS); an unauthenticated publish is rejected 401/403; a publish under a namespace the credential does not own is rejected;.
- `CAP-PLUG-031` — Referenced code exists, but this backlog item describes work that had not landed: a module exceeding `limits.memory` is terminated with a typed error before exhausting host memory; verified by a high-allocation test module.

### Multi-environment support

| ID | Capability | Status | Scope | Source |
|---|---|---|---|---|
| `CAP-MULTI-001` | Platform detection | implemented | Future | `FEATURES.md L682` |
| `CAP-MULTI-002` | Hardware introspection | planned | Future | `FEATURES.md L683` |
| `CAP-MULTI-003` | Network interface detection | planned | Future | `FEATURES.md L684` |
| `CAP-MULTI-004` | IPv6 dual-stack on all listeners | unknown | Future | `FEATURES.md L685` |
| `CAP-MULTI-005` | Address family preference | unknown | Future | `FEATURES.md L686` |
| `CAP-MULTI-006` | IPv6 bracketing helpers | unknown | Future | `FEATURES.md L687` |
| `CAP-MULTI-007` | Cloud metadata stub | unknown | Future | `FEATURES.md L688` |
| `CAP-MULTI-008` | Windows agent | planned | Future | `FEATURES.md L692` |
| `CAP-MULTI-009` | macOS agent | planned | Future | `FEATURES.md L693` |
| `CAP-MULTI-010` | Container runtime detection | planned | Future | `FEATURES.md L694` |
| `CAP-MULTI-011` | Kubernetes operator | planned | Future | `FEATURES.md L698` |
| `CAP-MULTI-012` | k8s_* stdlib modules | planned | Future | `FEATURES.md L699` |
| `CAP-MULTI-013` | Full cloud metadata extraction | planned | Future | `FEATURES.md L703` |
| `CAP-MULTI-014` | Container metadata extraction | planned | Future | `FEATURES.md L704` |
| `CAP-MULTI-015` | Service mesh integration | planned | Future | `FEATURES.md L708` |
| `CAP-MULTI-016` | Edge agent mode | planned | Future | `FEATURES.md L709` |
| `CAP-MULTI-017` | DNS provider management | planned | Future | `FEATURES.md L710` |
| `CAP-MULTI-018` | Advanced networking | planned | Future | `FEATURES.md L711` |
| `CAP-MULTI-019` | Domain overview and architecture notes | reference | Future | `PROJECT-DETAILS.md L1335` |

Scope and status notes:

- `CAP-MULTI-001` — FEATURES.md declares a source path that no longer resolves, but the identifiers it quotes are present in the tree: `/etc/os-release`.
- `CAP-MULTI-002` — FEATURES.md declares source path(s) that are absent from the final tree: internal/hardware/
- `CAP-MULTI-003` — FEATURES.md declares source path(s) that are absent from the final tree: internal/netutil/
- `CAP-MULTI-011` — FEATURES.md declares source path(s) that are absent from the final tree: internal/k8s/

### Specialized and extension domains

| ID | Capability | Status | Scope | Source |
|---|---|---|---|---|
| `CAP-SPEC-001` | File distribution (basic) | partial | Future | `FEATURES.md L719` |
| `CAP-SPEC-002` | Self-management (basic) | partial | Future | `FEATURES.md L720` |
| `CAP-SPEC-003` | Basic rate limiting | implemented | Future | `FEATURES.md L721` |
| `CAP-SPEC-004` | File distribution: NATS Object Store + Git backend + mirror groups (geographic redundancy with read | planned | Future | `FEATURES.md L725` |
| `CAP-SPEC-005` | Self-management: automated scheduled backups + rolling upgrades + drift detection on self-config | planned | Future | `FEATURES.md L726` |
| `CAP-SPEC-006` | Quota system | planned | Future | `FEATURES.md L727` |
| `CAP-SPEC-007` | kscore-loadtest benchmarking | planned | Future | `FEATURES.md L728` |
| `CAP-SPEC-008` | Proxy agents | planned | Future | `FEATURES.md L732` |
| `CAP-SPEC-009` | Air-gapped deployments | planned | Future | `FEATURES.md L735` |
| `CAP-SPEC-010` | Federation | planned | Future | `FEATURES.md L736` |
| `CAP-SPEC-011` | MCP server | planned | Future | `FEATURES.md L737` |
| `CAP-SPEC-012` | Saga checkpoint resume | planned | Future | `FEATURES.md L738` |
| `CAP-SPEC-013` | Web UI / Management Console | planned | Future | `FEATURES.md L742` |
| `CAP-SPEC-014` | Blueprint marketplace | planned | Future | `FEATURES.md L743` |
| `CAP-SPEC-015` | Multi-cloud test matrix | planned | Future | `FEATURES.md L744` |
| `CAP-SPEC-016` | Cross-platform expanded test matrix | planned | Future | `FEATURES.md L745` |
| `CAP-SPEC-017` | UDP data diode | planned | Future | `FEATURES.md L746` |
| `CAP-SPEC-018` | Per-agent NATS credentials with subject permissions | planned | v0.6 | `docs/project/ROADMAP.md L321` |
| `CAP-SPEC-019` | Reactor engine + event lifecycle tracking | planned | Future | `docs/project/ROADMAP.md L444` |
| `CAP-SPEC-020` | Backup orchestration features | planned | Future | `docs/project/ROADMAP.md L544` |
| `CAP-SPEC-021` | Bootstrap phase handlers + durable checkpointer | planned | Future | `docs/project/ROADMAP.md L552` |
| `CAP-SPEC-022` | Batch dispatcher: no orphan-job recovery | planned | Future | `docs/project/ROADMAP.md L576` |
| `CAP-SPEC-023` | API key issuance: non-transactional | planned | Future | `docs/project/ROADMAP.md L584` |
| `CAP-SPEC-024` | Dependency posture re-audit | planned | Future | `docs/project/ROADMAP.md L636` |
| `CAP-SPEC-025` | RunbookGRPCServer not registered at boot | planned | Future | `docs/project/ROADMAP.md L660` |
| `CAP-SPEC-026` | Blueprint publish: no path into the server catalog | planned | Future | `docs/project/ROADMAP.md L714` |
| `CAP-SPEC-027` | Agent secret grants are config-only | planned | Future | `docs/project/ROADMAP.md L728` |
| `CAP-SPEC-028` | Changie configuration: replacements for reference-link maintenance | planned | Future | `docs/project/ROADMAP.md L736` |
| `CAP-SPEC-029` | Glob matching: no ** (double-star) | planned | Future | `docs/project/ROADMAP.md L991` |
| `CAP-SPEC-030` | Migration journal: no per-table checkpoint resume | planned | Future | `docs/project/ROADMAP.md L998` |
| `CAP-SPEC-031` | Rename source/v1x-backlog label to drop the version pin | planned | Future | `docs/project/ROADMAP.md L1005` |
| `CAP-SPEC-032` | PROJECT-DETAILS.md lint cleanup | planned | Future | `docs/project/ROADMAP.md L1013` |
| `CAP-SPEC-033` | Operations runbooks accuracy sweep | planned | Future | `docs/project/ROADMAP.md L1021` |
| `CAP-SPEC-034` | Backup destinations: Backblaze B2 documentation + smoke test | planned | Future | `docs/project/ROADMAP.md L1069` |
| `CAP-SPEC-035` | Phase E1: required signed commits on main branch protection | partial | Future | `docs/project/ROADMAP.md L1093` |
| `CAP-SPEC-036` | Promo video pipeline — remaining shots and polish | planned | Future | `docs/project/ROADMAP.md L1117` |
| `CAP-SPEC-037` | Logging: context-aware threading of deep helpers | planned | Future | `docs/project/ROADMAP.md L1155` |
| `CAP-SPEC-038` | Rate-limit: Retry-After HTTP-date format alternative | planned | Future | `docs/project/ROADMAP.md L1174` |
| `CAP-SPEC-039` | Backup destinations: SFTP + GCS + Azure Blob + advanced S3 auth | planned | Future | `docs/project/ROADMAP.md L1206` |
| `CAP-SPEC-040` | Backup encryption: AWS KMS + Vault key providers | planned | Future | `docs/project/ROADMAP.md L1214` |
| `CAP-SPEC-041` | Weighted endpoint load distribution + K8s endpoint discovery | planned | Future | `docs/project/ROADMAP.md L1439` |
| `CAP-SPEC-042` | Air-gap baseline | planned | Future | `docs/project/ROADMAP.md L1479` |
| `CAP-SPEC-043` | Config: no per-endpoint TLS overrides | planned | Future | `docs/project/ROADMAP.md L1593` |
| `CAP-SPEC-044` | macOS agent | planned | Future | `docs/project/ROADMAP.md L1624` |
| `CAP-SPEC-045` | kscore-backup verify confirms integrity | implemented | Future | `epics/18-self-mgmt-files-ratelimit.md L121` |
| `CAP-SPEC-046` | Restore over populated cluster requires --force | implemented | Future | `epics/18-self-mgmt-files-ratelimit.md L123` |
| `CAP-SPEC-047` | Rejected requests counted in metric | implemented | Future | `epics/18-self-mgmt-files-ratelimit.md L137` |
| `CAP-SPEC-048` | Domain overview and architecture notes | reference | Future | `PROJECT-DETAILS.md L1386` |
| `CAP-SPEC-049` | CI cache and compression steps can no longer fail a gate | implemented | Future | `.changes/unreleased/20260903-ci-cache-non-blocking.yaml` |
| `CAP-SPEC-050` | make clean and make clean-check derive their stray lists from the tree | implemented | Future | `.changes/unreleased/20260903-clean-check-and-tape-guard.yaml` |
| `CAP-SPEC-051` | Dependency upgrades clearing 10 govulncheck findings | implemented | Future | `.changes/unreleased/20260903-dep-cve-upgrades.yaml` |
| `CAP-SPEC-052` | E2E JetStream budget no longer depends on the host's free disk | implemented | Future | `.changes/unreleased/20260903-e2e-jetstream-budget.yaml` |
| `CAP-SPEC-053` | Go toolchain bumped to 1.27.1 | implemented | Future | `.changes/unreleased/20260903-go-1-27-1.yaml` |
| `CAP-SPEC-054` | make install-tools rebuilds analysers whose build toolchain is stale | implemented | Future | `.changes/unreleased/20260903-install-tools-stale.yaml` |
| `CAP-SPEC-055` | Three per-feature docs clips | implemented | Future | `.changes/unreleased/20260903-promo-docs-clips.yaml` |
| `CAP-SPEC-056` | Promo pipeline renders multiple reels | implemented | Future | `.changes/unreleased/20260903-promo-reels.yaml` |
| `CAP-SPEC-057` | Promo render pipeline works end to end | implemented | Future | `.changes/unreleased/20260903-promo-render-fixes.yaml` |
| `CAP-SPEC-058` | Promo scenario shots corrected against real CLI output | implemented | Future | `.changes/unreleased/20260903-promo-scenario-fixes.yaml` |
| `CAP-SPEC-059` | promogen tapes verifies every promo tape command still resolves | implemented | Future | `.changes/unreleased/20260903-promo-tape-guard.yaml` |
| `CAP-SPEC-060` | promogen's template-injection suppression uses the #nosec form | implemented | Future | `.changes/unreleased/20260903-promogen-nosec-form.yaml` |
| `CAP-SPEC-061` | An agent can now prove which agent it is | implemented | Future | `.changes/unreleased/20260904-agent-svid-signing.yaml` |
| `CAP-SPEC-062` | ApplyState dispatches to agents when the request carries a target | implemented | Future | `.changes/unreleased/20260904-applystate-remote.yaml` |
| `CAP-SPEC-063` | Stacked pull requests get CI | implemented | Future | `.changes/unreleased/20260904-ci-pr-any-base.yaml` |
| `CAP-SPEC-064` | The E2E topology now enrolls agents the way an operator does, and proves the secret path end to end | implemented | Future | `.changes/unreleased/20260904-e2e-agent-secret.yaml` |
| `CAP-SPEC-065` | The fleet docs clip runs a package-version query instead of hostname | implemented | Future | `.changes/unreleased/20260904-fleet-clip-package-query.yaml` |
| `CAP-SPEC-066` | Dropped the permissions: blocks from the Forgejo workflows | implemented | Future | `.changes/unreleased/20260904-forgejo-permissions-warning.yaml` |
| `CAP-SPEC-067` | Docs clips are now one continuous session per task, not one per command | implemented | Future | `.changes/unreleased/20260904-promo-task-clips.yaml` |
| `CAP-SPEC-068` | The secrets docs clip shows the credential being used, not printed | implemented | Future | `.changes/unreleased/20260904-secrets-clip-realistic.yaml` |
| `CAP-SPEC-069` | kscorectl secrets list rejects a positional prefix instead of silently ignoring it | implemented | Future | `.changes/unreleased/20260904-secrets-list-noargs.yaml` |
| `CAP-SPEC-070` | A state file can reference a secret | implemented | Future | `.changes/unreleased/20260904-state-secret-function.yaml` |
| `CAP-SPEC-071` | ApplyBlueprint accepts a target | implemented | Future | `.changes/unreleased/20260905-blueprint-remote-grpc.yaml` |
| `CAP-SPEC-072` | A blueprint can be applied to a fleet | implemented | Future | `.changes/unreleased/20260905-blueprint-remote-runner.yaml` |
| `CAP-SPEC-073` | The tree is gofmt-clean under Go 1.27, and stays that way | implemented | Future | `.changes/unreleased/20260905-gofmt-1-27.yaml` |
| `CAP-SPEC-074` | Tracked the agent-secret follow-ups that were only named in passing | implemented | Future | `.changes/unreleased/20260905-secret-followup-entries.yaml` |
| `CAP-SPEC-075` | statemgmt.Marshal renders a StateFile back to YAML | implemented | Future | `.changes/unreleased/20260905-statemgmt-marshal.yaml` |
| `CAP-SPEC-076` | A failing test job now says which test failed | implemented | Future | `.changes/unreleased/20260905-test-failure-diagnostics.yaml` |

Scope and status notes:

- `CAP-SPEC-008` — FEATURES.md declares source path(s) that are absent from the final tree: internal/proxy/, internal/protocols/, internal/vendors/
- `CAP-SPEC-011` — FEATURES.md declares source path(s) that are absent from the final tree: internal/mcp/, cmd/kscore-mcp/
- `CAP-SPEC-018` — Status established by hand audit of the v0.6 scope set: The roadmap entry states no per-subject permissions are configured anywhere.
- `CAP-SPEC-049` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-050` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-051` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-052` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-053` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-054` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-055` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-056` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-057` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-058` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-059` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-060` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-061` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-062` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-063` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-064` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-065` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-066` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-067` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-068` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-069` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-070` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-071` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-072` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-073` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-074` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-075` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.
- `CAP-SPEC-076` — An unreleased changelog fragment records a change that landed; the fragment is the evidence.

Known gaps and limitations:

- `CAP-SPEC-001` — File distribution: PUT-side resume.
- `CAP-SPEC-002` — Backup + restore component adapters (storage/JetStream/etcd/config/secrets/cluster).
- `CAP-SPEC-004` — File distribution: NATS Object Store + Git backends, mirror groups, conflict resolution.
- `CAP-SPEC-018` — Referenced code exists, but this backlog item describes work that had not landed: Per-agent NATS credentials with subject permissions.
- `CAP-SPEC-019` — The backlog records this as outstanding work; the entry names no reference paths, so the archive shows what was wanted rather than what was built.
- `CAP-SPEC-020` — The backlog records this as outstanding work; the entry names no reference paths, so the archive shows what was wanted rather than what was built.
- `CAP-SPEC-021` — Referenced code exists, but this backlog item describes work that had not landed: `kscore-bootstrap --seed dev-seed.yaml` runs end-to-end on a fresh Linux host and lands at `StateVerified`; SIGKILL mid-Configuring then restart resumes at `StateConfiguring`.
- `CAP-SPEC-022` — The backlog item's own text states this had not been built: Batch dispatcher: no orphan-job recovery.
- `CAP-SPEC-023` — The backlog item's own text states this had not been built: API key issuance: non-transactional.
- `CAP-SPEC-024` — Referenced code exists, but this backlog item describes work that had not landed: A timestamped re-audit lands in `docs/project/SECURITY-GOVERNANCE.md` "Dependency posture" section with the new module count + license distribution + main.
- `CAP-SPEC-025` — Referenced code exists, but this backlog item describes work that had not landed: `cmd/kscore-server/main.go` constructs a `runbook.Engine` from config + registers `RunbookGRPCServer`. Integration test verifies `RunbookService.ListRunbooks` returns 200 against a.
- `CAP-SPEC-026` — Referenced code exists, but this backlog item describes work that had not landed: Blueprint publish: no path into the server catalog.
- `CAP-SPEC-027` — Referenced code exists, but this backlog item describes work that had not landed: Agent secret grants are config-only.
- `CAP-SPEC-028` — Referenced code exists, but this backlog item describes work that had not landed: `.changie.yaml` has a `replacements:` block that, when `make changelog-batch VERSION=v0.2.0` runs against a CHANGELOG.md currently pointing at `[Unreleased]: …compare/v0.1.0...HEAD.
- `CAP-SPEC-029` — The backlog item's own text states this had not been built: Glob matching: no `**` (double-star).
- `CAP-SPEC-030` — The backlog item's own text states this had not been built: Migration journal: no per-table checkpoint resume.
- `CAP-SPEC-031` — Referenced code exists, but this backlog item describes work that had not landed: `source/v1x-backlog` is gone from `tools/trackerctl/config/labels.yaml`, `issues.go::labelNamesFor`, both test files, `ISSUE-TRACKING.md` §`source/v1x-backlog` table row, and the R.
- `CAP-SPEC-032` — Referenced code exists, but this backlog item describes work that had not landed: `markdownlint-cli2 PROJECT-DETAILS.md` returns 0 errors. The carve-out is removed from `.markdownlint-cli2.yaml`'s `ignores` block. `make docs-lint` (with the file no longer carved.
- `CAP-SPEC-033` — The backlog item's own text states this had not been built: Each runbook step is verified against a fresh `make e2e-up` topology; commands run as documented; outputs match.
- `CAP-SPEC-034` — Referenced code exists, but this backlog item describes work that had not landed: B2 documented in `internal/backup/dest/dest.go` package comment + README; a smoke test exercises a real B2 bucket via the `s3://` path + B2 endpoint.
- `CAP-SPEC-035` — contributor-key onboarding flow documented (probably in `CONTRIBUTING.md` or a new `docs/project/SIGNING-KEYS.md`); the `main` branch protection rule on Codeberg has `require_signe.
- `CAP-SPEC-036` — Referenced code exists, but this backlog item describes work that had not landed: Scenario runs against an agent host with a `package` + `service` declaration once remote apply lands; `make promo` produces `dist/promo/keystone-30s.mp4` with crossfades; a stale t.
- `CAP-SPEC-037` — Referenced code exists, but this backlog item describes work that had not landed: `tools/logaudit` (or `grep` audit) reports zero non-context `slog.*` calls outside an explicit allowList; allowList entries each name why the site can't take a ctx (e.g., process-w.
- `CAP-SPEC-038` — Referenced code exists, but this backlog item describes work that had not landed: An operator flag selects between formats; tests cover both round-trips through a representative client.
- `CAP-SPEC-038` — Rate-limit: configurable gRPC `retry-after-ms` trailer key.
- `CAP-SPEC-039` — Referenced code exists, but this backlog item describes work that had not landed: `kscore-backup create --dest sftp://host/path/foo.tar` succeeds; same for `gs://` and `azblob://`; `AWS_WEB_IDENTITY_TOKEN_FILE` produces a valid S3 client on EKS.
- `CAP-SPEC-040` — Referenced code exists, but this backlog item describes work that had not landed: `kscore-backup create --key-provider=aws-kms:arn:...` writes an artifact whose age recipients are wrapped under the KMS key; restore unwraps via the same KMS. Same for `vault:trans.
- `CAP-SPEC-041` — The backlog item's own text states this had not been built: Load measurably distributes proportionally to weights; `nats.urls = ["k8s://service-name"]` resolves through discovery.
- `CAP-SPEC-042` — The backlog records this as outstanding work; the entry names no reference paths, so the archive shows what was wanted rather than what was built.
- `CAP-SPEC-043` — The backlog item's own text states this had not been built: Config: no per-endpoint TLS overrides.
- `CAP-SPEC-044` — The backlog records this as outstanding work; the entry names no reference paths, so the archive shows what was wanted rather than what was built.

### Operational runbooks

| ID | Capability | Status | Scope | Source |
|---|---|---|---|---|
| `CAP-OPS-001` | Runbook: README | implemented | Future | `docs/runbooks/README.md L19` |
| `CAP-OPS-002` | Runbook: backup restore | implemented | Future | `docs/runbooks/backup-restore.md L3` |
| `CAP-OPS-003` | Runbook: bootstrap new cluster | implemented | Future | `docs/runbooks/bootstrap-new-cluster.md L3` |
| `CAP-OPS-004` | Runbook: capacity scaling | implemented | Future | `docs/runbooks/capacity-scaling.md L3` |
| `CAP-OPS-005` | Runbook: certificate rotation | implemented | Future | `docs/runbooks/certificate-rotation.md L3` |
| `CAP-OPS-006` | Runbook: disaster recovery | implemented | Future | `docs/runbooks/disaster-recovery.md L3` |
| `CAP-OPS-007` | Runbook: emergency rollback | implemented | Future | `docs/runbooks/emergency-rollback.md L3` |
| `CAP-OPS-008` | Runbook: performance degradation | implemented | Future | `docs/runbooks/performance-degradation.md L3` |
| `CAP-OPS-009` | Runbook: scheduled maintenance | implemented | Future | `docs/runbooks/scheduled-maintenance.md L3` |
| `CAP-OPS-010` | Runbook: security incident | implemented | Future | `docs/runbooks/security-incident.md L3` |
| `CAP-OPS-011` | Runbook: troubleshooting | implemented | Future | `docs/runbooks/troubleshooting.md L3` |
| `CAP-OPS-012` | Runbook: upgrade cluster | implemented | Future | `docs/runbooks/upgrade-cluster.md L3` |

### Release, packaging and distribution

| ID | Capability | Status | Scope | Source |
|---|---|---|---|---|
| `CAP-REL-001` | Expanded getting-started guides (per-domain tutorials) | implemented | Future | `docs/project/ROADMAP.md L240` |
| `CAP-REL-002` | Soak-test infrastructure for fd / connection / goroutine leaks | planned | Future | `docs/project/ROADMAP.md L606` |
| `CAP-REL-003` | Sustained-load profiling baseline | planned | Future | `docs/project/ROADMAP.md L614` |
| `CAP-REL-004` | Security baseline expansion | planned | Future | `docs/project/ROADMAP.md L622` |
| `CAP-REL-005` | Error-message docs URLs | planned | Future | `docs/project/ROADMAP.md L1029` |
| `CAP-REL-006` | Release dry-run expansion | planned | Future | `docs/project/ROADMAP.md L1163` |
| `CAP-REL-007` | All HA resilience tests (Epic 13) pass | implemented | Future | `epics/19-test-harden-release.md L111` |
| `CAP-REL-008` | All performance SLOs met in CI | implemented | Future | `epics/19-test-harden-release.md L112` |
| `CAP-REL-009` | Security baseline clean: 0 secrets, 0 known CVEs, 0 gosec high/critical | implemented | v0.6 | `epics/19-test-harden-release.md L113` |
| `CAP-REL-010` | Coverage gates pass: critical >70%, CLI >40% | implemented | Future | `epics/19-test-harden-release.md L114` |
| `CAP-REL-011` | SHA-256 checksum file generated | implemented | v0.6 | `epics/19-test-harden-release.md L117` |
| `CAP-REL-012` | kscorectl --help lists all subcommands; each has help text | implemented | Future | `epics/19-test-harden-release.md L118` |
| `CAP-REL-013` | CLI reference docs cover every command | implemented | Future | `epics/19-test-harden-release.md L119` |
| `CAP-REL-014` | Quick-start guide can be followed end-to-end on a fresh Ubuntu VM in &lt;30 minutes | implemented | Future | `epics/19-test-harden-release.md L122` |
| `CAP-REL-015` | RELEASE-PLAYBOOK.md, CHANGELOG.md (v0.1.0 entry), SECURITY.md, COMPATIBILITY.md complete and reviewed | implemented | Future | `epics/19-test-harden-release.md L124` |
| `CAP-REL-016` | v1.0.0 tag pushed; goreleaser produces release artifacts | planned | Future | `epics/19-test-harden-release.md L125` |
| `CAP-REL-017` | Domain overview and architecture notes | reference | Future | `PROJECT-DETAILS.md L1484` |

Known gaps and limitations:

- `CAP-REL-002` — Referenced code exists, but this backlog item describes work that had not landed: `make soak` runs the docker-compose topology under sustained load for ≥1 hour, asserts: (a) `runtime.NumGoroutine` flat ±5%; (b) per-process `lsof` count flat ±5%; (c) per-process.
- `CAP-REL-003` — Referenced code exists, but this backlog item describes work that had not landed: `make profile-sustained` (or equivalent) runs the harness; per-domain profile files land next to `docs/project/PROFILING-BASELINE.md`'s baseline numbers; the doc gains a per-domain.
- `CAP-REL-004` — Referenced code exists, but this backlog item describes work that had not landed: Each sub-bullet adds its own Make target + CI step + brief docs entry under `docs/project/SECURITY-GOVERNANCE.md` "Security Baseline Pipeline." `make security` (or equivalent umbre.
- `CAP-REL-005` — The backlog item's own text states this had not been built: A `pkg/api/apierror` (or similar) helper produces "&lt;message>. See &lt;docs-url>" strings; the docs site has the matching slug pages; key user-facing errors (config validation, secrets.
- `CAP-REL-006` — Referenced code exists, but this backlog item describes work that had not landed: Each sub-bullet adds its own check function in `scripts/release-smoke.sh` (or the container companion) and a row in `docs/project/SECURITY-GOVERNANCE.md` "Release Dry-Run Smoke.".

### Generation 1 planning artifacts

| ID | Capability | Status | Scope | Source |
|---|---|---|---|---|
| `CAP-REL-018` | 6.3 Why v0.1 / v0.5 / v1.0 Are What They Are — MVP Reasoning Recap | reference | Future | `PROJECT-DETAILS.md L1510` |
| `CAP-FOUND-034` | 3.6 Project Layout (target for rebuild) | reference | Future | `PROJECT-DETAILS.md L150` |
| `CAP-FOUND-035` | 5.4 Build, Release, & Supply Chain | reference | Future | `PROJECT-DETAILS.md L1458` |
| `CAP-FOUND-036` | Phase A — Foundations (weeks 1-2) | reference | Future | `PROJECT-DETAILS.md L1533` |
| `CAP-FOUND-037` | Phase B — API & wire format (weeks 3-4) | reference | Future | `PROJECT-DETAILS.md L1540` |
| `CAP-FOUND-038` | Phase C — Messaging (weeks 4-5, parallel with B) | reference | Future | `PROJECT-DETAILS.md L1545` |
| `CAP-FOUND-039` | Phase D — Agent runtime (weeks 5-6) | reference | Future | `PROJECT-DETAILS.md L1550` |
| `CAP-FOUND-040` | Phase E — Control plane services (weeks 7-9) | reference | Future | `PROJECT-DETAILS.md L1554` |
| `CAP-FOUND-041` | Phase F — Identity & secrets (weeks 9-11) | reference | Future | `PROJECT-DETAILS.md L1560` |
| `CAP-FOUND-042` | Phase G — Events, audit, policy infra (weeks 11-12) | reference | Future | `PROJECT-DETAILS.md L1565` |
| `CAP-FOUND-043` | Phase H — Clustering & HA (weeks 13-15) [the v1 differentiator] | reference | Future | `PROJECT-DETAILS.md L1570` |
| `CAP-FOUND-044` | Phase I — Plugin / module system (weeks 16-18) | reference | Future | `PROJECT-DETAILS.md L1578` |
| `CAP-FOUND-045` | Phase J — Blueprints & runbooks (weeks 19-20) | reference | Future | `PROJECT-DETAILS.md L1583` |
| `CAP-FOUND-046` | Phase K — GitOps & webhooks (weeks 20-21) | reference | Future | `PROJECT-DETAILS.md L1588` |
| `CAP-FOUND-047` | Phase L — Observability & ops (weeks 22-23) | reference | Future | `PROJECT-DETAILS.md L1594` |
| `CAP-FOUND-048` | Phase M — File distribution & misc (weeks 23-24) | reference | Future | `PROJECT-DETAILS.md L1601` |
| `CAP-FOUND-049` | Phase N — Test, harden, release (weeks 25-26) | reference | Future | `PROJECT-DETAILS.md L1604` |

### Generation 2 transition

| ID | Capability | Status | Scope | Source |
|---|---|---|---|---|
| `CAP-TRANS-001` | Generation 2 reboot transition record | implemented | v0.6 | `.changes/unreleased/20260909-generation-1-freeze.yaml` |

## Known limitations of this catalog

Two things a reader should not assume, both established by the independent
verification this task required rather than discovered later.

**The check proves form, not truth.** `make capability-catalog-check` confirms
the catalog is well-formed, completely covered, and internally consistent. It
cannot confirm that any individual status is *correct*. Reconciling the totals
table catches a single status changed to a falsehood, because the counts stop
adding up — but a compensating swap between two entries balances and passes, and
a renamed capability passes. The statuses rest on the audit recorded in this
task's commit history; CI protects them from drift, not from error.

**`unknown` is a deliberate answer, not an unfinished one.** Where `FEATURES.md`
declares no source path and quotes no identifier that resolves, the entry says
`unknown` rather than guessing. A looser matching rule was tried and rejected
during the audit: matching any single significant word of a capability name to a
Go filename paired "Windows agent" and "macOS agent" with the same
`internal/agent/agent.go`, and "Multi-region replication" with
`internal/audit/multi.go`. It produced 182 matches, most of them wrong. A false
`implemented` is a claim the archive cannot support; an `unknown` is an
admission it cannot be settled from the sources. The remaining `unknown` entries
are the second kind.

## Coverage

Every heading, checklist item, capability bullet, roadmap entry, and changelog
fragment in the Generation 1 sources maps to exactly one entry above.
[`../transition/capability-coverage.json`](../transition/capability-coverage.json)
records that mapping item by item, with the relation each item bears to its
entry — `primary`, `refinement`, `duplicate`, or `domain-overview`. It is the
artifact to check against, rather than this file's prose, when verifying that
nothing was dropped.

Sources reconciled: `FEATURES.md`, `PROJECT-DETAILS.md`,
[`ROADMAP.md`](ROADMAP.md), [`STATE-SUPPORT-MATRIX.md`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/tag/archive-2026-09-pre-v0.6-reboot/docs/project/STATE-SUPPORT-MATRIX.md),
`epics/00`–`epics/19`, `docs/runbooks/`, and the unreleased changelog fragments.

## Not planned by default

The catalog does not imply that Keystone should become a general-purpose IaC
engine, Kubernetes replacement, remote desktop, endpoint-detection product, or
unbounded shell gateway. Those directions require a new product-level RFC, not
ordinary promotion from Future.
