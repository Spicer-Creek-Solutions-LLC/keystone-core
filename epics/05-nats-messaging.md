# Epic 05: NATS Messaging

**Phase**: C • **Estimate**: 2 weeks • **Depends on**: 01 • **Blocks**: 04, 06, 11, 13, 14

## Goal

NATS messaging layer with embedded mode (zero-deps single binary) and external mode (HA cluster) — the v1 control-plane↔agent transport. Cluster-prefixed subject hierarchy from day 1 so v2.0 supercluster slides in without refactor. Bootstrap registration with minimal-permission credential exchange.

## Scope (in)

- `internal/nats/` — Manager, ConnectionManager, ConnectionStrategy interface (Direct + TLS only in v1.0), StrategySelector, Endpoint + EndpointState, JetStream stream definitions.
- Embedded NATS server via `nats-server/v2` library (config: port, max_connections, JetStream on by default, max_memory).
- External NATS connect with multi-endpoint failover, exp-backoff reconnect.
- **Subject hierarchy with cluster prefix** (always — even single-cluster v1):
  ```
  kscore.{cluster}.agent.register
  kscore.{cluster}.agent.heartbeat
  kscore.{cluster}.agent.{id}.command|response|state|events
  kscore.{cluster}.server.announce|control
  kscore.{cluster}.bootstrap.{id}.register|response
  kscore.{cluster}.discovery
  ```
- `Envelope{MessageID, CorrelationID, Priority (Low|Normal|High|Critical), TTL, ClusterPrefix}` wrapper for every message.
- `DedupConfig{WindowDuration, MaxEntries, PerSubjectOverrides}` SHA-256-based dedup.
- Per-endpoint `CircuitBreakerConfig{FailureThreshold, SuccessThreshold, OpenDuration, HalfOpenMaxAttempts}` — closed → open → half-open → closed.
- JetStream enablement (embedded + external).
- Bootstrap registration flow:
  1. Agent obtains bootstrap credential (PSK or short-TTL one-time token; default TTL 5m).
  2. Agent connects with permissions limited to `kscore.{cluster}.bootstrap.{id}.register|response`.
  3. Agent publishes registration with identity proof; server validates, generates full credentials.
  4. Server publishes full credentials on response subject.
  5. Agent reconnects with full credentials (agent-specific subjects).
  6. Bootstrap credential expires.
- `Manager.Health()` + `ConnectionManager.Health()` for /health/ready integration.

## Scope (out / non-goals)

- Leaf nodes — v2.0.
- Supercluster / gateway — v2.0.
- WebSocket / WSS transport — v2.0.
- Auto-discovery (DNS, mDNS, K8s, Consul, etcd) — v1.3 (K8s) / v2.0 (others).
- Reverse-leaf NAT traversal — v2.0.
- Exactly-once delivery — v2.x.

## Design summary

See `PROJECT-DETAILS.md §4.2`.

## Tasks

1. **`Manager`** — embedded server lifecycle (Start, Shutdown) + external connection lifecycle via `nats.Connect()`. _(landed: `internal/nats.Manager`, embedded uses `nats.InProcessServer`; cmd/kscore-server now constructs the real manager from `cfg.NATS`.)_
2. **`ConnectionManager`** — multi-endpoint with health check, failover, circuit breaker. Tests with synthetic endpoints. _(landed combined with task 3: external mode delegates to `ConnectionManager`; v1.0 leans on nats.go native multi-URL failover with per-endpoint observability layered via callbacks. `Breaker` interface in place; real state machine arrives in task 7.)_
3. **`Endpoint` + `EndpointState`** types; per-endpoint health tracking (state, latency P50/P99, failure count). _(landed with task 2: `internal/nats/endpoint.go`; latency P50/P99 from a 64-sample ring buffer fed by 5s RTT probes.)_
4. **`SubjectBuilder`** with mandatory cluster prefix. _(landed: `internal/nats/subject.go` owns the v1.0 hierarchy via typed constructors; `Manager.Publish` calls `Validate(subject)` and rejects anything not under `kscore.{cluster}.…` or that contains a wildcard / whitespace; `controlplane.CommandDispatcher` now consumes a narrow `Subjects` interface threaded through `server.Options.Subjects`.)_
5. **`Envelope`** wrapper + JSON codec. _(landed: `pkg/envelope.Envelope{MessageID, CorrelationID, Priority, TTLMillis, ClusterPrefix, Payload}` with JSON codec; `Manager.PublishEnvelope` is the only publish path — byte-level Publish retired; `CommandDispatcher` now stamps `CorrelationID = command.ID` so responses can match without a separate lookup. `MessageID` is the dedup key consumed by Task 6.)_
6. **Dedup** — SHA-256 keyed sliding window in memory; cleanup loop. _(landed: producer-side dedup in `internal/nats.Dedup` with length-prefixed `sha256(subject || messageID)` keys (defense against separator-byte hash-collision suppression attacks); list+map storage; longest-prefix `PerSubjectOverrides`; cleanup goroutine purges expired. `Manager.PublishEnvelope` checks before publish, records on success — failed publishes leave no phantom entry. `envelope.ErrDuplicate` is the sentinel; `SubjectBuilder.Validate` and `envelope.Validate` reject non-printable ASCII as defense-in-depth.)_
7. **Circuit breaker** state machine + tests. _(landed: real `circuitBreaker` replacing the noop stub from task 2; closed→open→half-open→closed with injectable clock for deterministic tests; `OnFailure`/`OnSuccess` driven from existing connect callbacks; `ConnectionManager.Health` returns error when every endpoint's breaker is OPEN. Active dial-time eviction (skipping OPEN endpoints when nats.go picks the next reconnect target) is **deferred to v1.x** — would require replacing nats.go's native multi-URL failover with a per-endpoint dial loop, marginal v1.0 benefit.)_
8. **JetStream stream definitions** for events + commands (defaults: 7d, 10GB, 1M msgs, DiscardNew). _(landed: `internal/nats/jetstream.go` defines `KSCORE_COMMANDS_<cluster>` and `KSCORE_EVENTS_<cluster>` streams; `Manager.ensureStreams` upserts on Start (idempotent across restarts; hard-fails on stream-creation error). `JetStreamConfig` extended with `StreamMaxAge`/`StreamMaxBytes`/`StreamMaxMsgs`/`StreamReplicas`. The redundant `Embedded.EnableJetStream` flag was retired — `JetStream.Enabled` is the single switch.)_
9. **Bootstrap registration server-side handler** — listens on `kscore.{cluster}.bootstrap.>`; validates evidence (PSK in v1.0; pluggable for future SPIFFE). _(landed: `internal/controlplane.BootstrapHandler` subscribes on `kscore.{cluster}.bootstrap.*.register`, validates via `BootstrapValidator` (v1.0 default `PSKValidator`: constant-time compare, single-use consume, in-memory; v1.x persists), issues credentials via `CredentialIssuer` (v1.0 default `APIKeyIssuer` returning a `RoleOperator` API key), persists agent record, publishes `AgentCredentials` on the response subject with `CorrelationID` = inbound `MessageID`. Defaults to disabled — operator opt-in via `cfg.NATS.Bootstrap.{Enabled,PSKs}`. NATS subject-permission scoping is a v1.0 trust assumption tightening with Epic 09.)_
10. **Bootstrap registration client-side flow** in `kscore-agent` integration (Epic 06).
11. **`Manager.Health()` + `ConnectionManager.Health()`** integration.
12. **IPv6 address bracket handling** in URL formatting (`[::]:4222` not `:4222`).
13. **Integration test**: full embedded-NATS round-trip (publish + subscribe + JetStream consume).

## Acceptance criteria

- [ ] Embedded mode: `kscore-server` boots with NATS in-process; agents connect via `nats://localhost:4222`.
- [x] External mode: multi-endpoint config with failover; killing one endpoint causes circuit breaker to open then half-open recovery. _(task 7 — state machine transitions covered by `TestCircuitBreaker_*`; `TestConnectionManager_HealthDegradesWhenAllBreakersOpen` covers the integration. Multi-endpoint failover at the nats.go layer covered by Task 2's `TestConnectionManager_FailoverWhenPrimaryDownAtStart`.)_
- [x] All published subjects use `kscore.{cluster}.…` prefix; verified by interceptor in tests. _(task 4 — `SubjectBuilder.Validate` is invoked from `Manager.Publish`; `TestManager_PublishRejectsUnprefixed` asserts wildcard/whitespace/wrong-cluster rejection plus a positive-control publish via the typed constructor.)_
- [x] Envelope MessageID dedup discards duplicates within window. _(task 6 — `Manager.PublishEnvelope` returns `envelope.ErrDuplicate` for repeats inside `WindowDuration`; `TestManager_PublishEnvelopeRejectsDuplicate` covers the round-trip plus per-subject overrides.)_
- [ ] Bootstrap registration end-to-end: agent with bootstrap credential → server validates → full credentials issued → agent reconnects → publishes on agent-specific subject. _(server side closed in task 9 — `TestBootstrapHandler_FullRoundTrip` covers `register → validate → AgentCredentials response`. Agent-side reconnect + publish is task 10 in Epic 06.)_
- [ ] Health() reports unhealthy when NATS down; recovers when up.
- [ ] IPv6 endpoints work (`nats://[::1]:4222`).
- [ ] Coverage >80% on `internal/nats`.

## Risks

- **Subscription leaks**: every subscription needs unsub on disconnect. Wrap in connection-scoped manager.
- **Bootstrap credential timing**: credential exchange not atomic at NATS level; agent briefly has old creds during switch. Document; tests must tolerate.
- **Dedup window vs RTT**: window must exceed max network RTT — too short → false duplicates. Default 5m; document.
- **JetStream storage on embedded**: data dir must be writable; missing dir = silent JetStream-disabled state. Validate on startup.

## References

- PROJECT-DETAILS §4.2, §4.6 (agent integration).
