# Epic 03: API Surface (gRPC + REST + Auth)

**Phase**: B • **Estimate**: 2 weeks • **Depends on**: 01 • **Blocks**: 04, 06, 07, 09, 10, 11, 12, 13, 16

## Goal

A single source of truth (proto) for all server↔client and client↔agent contracts, with a compatible REST surface for ad-hoc tooling. Authentication (API key / JWT / mTLS) and basic RBAC interceptors. Versioning registry.

## Scope (in)

- 8 proto files in `api/proto/` package `keystone.core.v1`, Go package `pkg/api/v1`:
  - `agent.proto` — `AgentService`: Register, Heartbeat, ExecuteCommand (stream), GetAgentInfo.
  - `controlplane.proto` — `ControlPlaneService`: ServerStatus, List/GetAgents, ExecuteCommand (stream), BatchExecuteCommand (stream), command status/history.
  - `state.proto` — `StateService`: ApplyState (stream), CheckState, DetectDrift, GetStateHistory, GetStateStatus.
  - `event.proto` — `EventService`: ListEvents, GetEvent, EmitEvent, SubscribeEvents (stream), GetEventTypes, GetEventStats.
  - `policy.proto` — `PolicyService`: EvaluatePolicy, EvaluatePolicySet, ListPolicies/Sets/Bindings, ListViolations, GetComplianceReport, GetAuditLog. (CRUD methods present but server returns Unimplemented in v1.0; v1.8 enables.)
  - `secrets.proto` — `SecretsService`: GetSecret, ListSecrets, WriteSecret, DeleteSecret, GetLease, ListLeases, RenewLease, RevokeLease, Encrypt, Decrypt, Sign, Verify.
  - `cluster.proto` — `ClusterService`: GetClusterStatus, ListMembers, GetMember, AddMember, RemoveMember, GetLeader, TransferLeader, Rebalance, CreateBackup, RestoreBackup, WatchMembership (stream), WatchLeadership (stream).
  - `coordination.proto` — `CoordinationService` (mTLS-only): ClusterHealth, GetLeader, NATSStatus, RecoveryCoordinate, Heartbeat, PropagateState.
- `pkg/api/v1/` populated via `make proto` (Buf generate).
- `pkg/api/auth/` — `Principal{ID, Name, Role, AuthMethod, Metadata, AuthenticatedAt}`, role hierarchy `admin > operator > readonly`, `Authenticator` interface, `Authorizer` interface, concrete `APIKeyAuthenticator`, `JWTAuthenticator`, `MTLSAuthenticator`, `RBACAuthorizer`, `RateLimiter` (exp backoff), `InterceptorConfig` builder.
- `pkg/api/rbac/` (v1.0 minimum) — hardcoded method→required-role map; bypass list (health, registration, coordination); `Handler` REST for `GET /api/v1/rbac/roles` (read-only listing).
- `pkg/api/apikeys/` — `APIKey{ID, Name, KeyHash, Role, CreatedAt, ExpiresAt, LastUsed}`; `Store` (in-memory + SQLite); `Handler` for `POST/GET/DELETE /api/v1/apikeys` (cleartext returned only on creation; never stored).
- `pkg/api/versioning/` — registry tracks `Status{current, supported, deprecated, retired, beta, alpha}` + `ReleasedAt`, `DeprecatedAt`, `SunsetAt`.
- Per-domain client packages in `pkg/api/<domain>/` (agents, cluster, events, execution, gitops, maintenance, policy, runbook, schedule, secrets, state, webhooks). REST handlers live here too — hand-coded, not grpc-gateway-generated.
- OpenAPI spec at `api/openapi/openapi-spec.yaml` — hand-maintained in v1.0.

## Scope (out / non-goals)

- gRPC-gateway annotation-driven REST — v2.0 (v1.0 hand-codes for control + simplicity).
- OpenAPI auto-generation — v2.0.
- Full RBAC role/permission CRUD with per-resource permissions — v1.2.
- MaintenanceService, ScheduleService gRPC — v1.1.
- MirrorService, DiscoveryService — v2.x.

## Design summary

See `PROJECT-DETAILS.md §4.5` (API Surface).

## Tasks

1. **Author all 8 .proto files** with v1.0 RPC sets. Use `keystone.core.v1` package; `go_package = "github.com/<org>/keystone-core/pkg/api/v1"`.
2. **Buf lint + buf breaking config** — STANDARD with documented exclusions; `buf breaking` against `main`.
3. **`make proto`** target verified end-to-end.
4. **`pkg/api/auth/`** — Principal, Authenticator/Authorizer interfaces, three concrete authenticators, RBACAuthorizer with method map, RateLimiter, InterceptorConfig builder, gRPC unary + stream interceptors, HTTP middleware. Tests for each.
5. **`pkg/api/apikeys/`** — types, in-memory + SQLite stores (extend `internal/state.Store` with `APIKeyStore` sub-interface), Handler REST routes. Constant-time hash compare.
6. **`pkg/api/versioning/`** — registry struct + helpers; loaded at server startup; emits deprecation headers.
7. **Per-domain client packages**: stub `Handler` structs in each `pkg/api/<domain>/` with route registration helpers. Concrete handlers ship with their respective epics.
8. **`api/openapi/openapi-spec.yaml`** initial scaffold covering v1.0 endpoints; CI lint via `redocly` or `swagger-cli`.

## Acceptance criteria

- [ ] All 8 protos compile via `make proto`.
- [ ] `buf lint` passes; `buf breaking` against `main` clean.
- [ ] Auth interceptor chain orders correctly: CORS → rate-limit → auth → handler.
- [ ] All three auth methods (API key, JWT, mTLS) round-trip in unit tests.
- [ ] CoordinationService rejects non-mTLS callers.
- [ ] RBACAuthorizer denies operator-level methods to readonly principal; allows admin operations on admin principal.
- [ ] API key generation returns cleartext only on creation; storage holds hash only.
- [ ] Bypass list (health, registration, coordination internal) works without credentials.
- [ ] Versioning registry serves deprecation headers when configured for a deprecated endpoint.
- [ ] Coverage >85% on `pkg/api/auth`; >80% on apikeys.

## Risks

- **gRPC vs REST drift**: enforce in code review — every new RPC ships its REST handler in the same PR.
- **Streaming reconnection**: clients must implement exp backoff; document in client package READMEs.
- **mTLS chicken-and-egg in CI**: tests use in-memory CA from Epic 09 (placeholder helper in v1.0 test util until Epic 09 lands).
- **Hand-maintained OpenAPI drift**: every new gRPC method requires a manual OpenAPI update in same PR. Add CI check that fails if proto changes without OpenAPI changes (best-effort heuristic).

## References

- PROJECT-DETAILS §4.5, §4.10 (auth model).
