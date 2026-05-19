# Epic 16: GitOps Integration + Outbound Webhooks

**Phase**: K • **Estimate**: 2 weeks (combined; ~1.25w gitops + ~0.75w webhooks) • **Depends on**: 03, 04, 11, 15 • **Blocks**: nothing critical

## Goal

Combine two related concerns: **inbound GitOps webhooks** with verification + manual rollback (the v1.0 deployment-safety story) and **outbound webhook subscriptions** for integration with Slack/PagerDuty/custom receivers (the v1.0 integration story).

## Scope (in)

### GitOps integration (`internal/gitops/`)

- **Webhook receiver**:
  - HTTP server (default `:8081/webhooks`).
  - `Handler` interface (`Type`, `Parse(request, body) → Event`).
  - Concrete handlers: ArgoCD, Flux, GitHub, GitLab.
  - Source auto-detection by header (`X-GitHub-Event`, `X-Gitlab-Event`, `X-Argo-CD-Webhook`, `X-Flux-Event`).
  - `Authenticator` interface — `HMACAuthenticator` (SHA-256 HMAC; secret per source), `BearerAuthenticator`, `NoneAuthenticator`.
  - Event normalization → unified `webhook.Event{webhookID, provider, application, namespace, revision, status, raw}`. `ToKscoreEvent()` emits on Keystone event bus as `gitops.{argocd|flux|github|gitlab}.*`.

- **Verification engine**:
  - `Verifier` interface (`Type`, `Verify(step) → Result`).
  - v1.0 verifiers: HTTP, gRPC, command/script.
  - `Workflow{Steps, Parallel, Timeout, OnFailure}` — sequential default; parallel via goroutines.
  - `Result{Success, Message, Data, Duration, Error, Retries}`.
  - Optional steps don't fail workflow.

- **Manual rollback engine**:
  - `Executor` interface (`Type`, `Execute(ctx, config, req) → Result`, `GetPreviousRevision()`, `GetLastKnownGood()`).
  - v1.0 executors: Git revert, ArgoCD sync-to-revision, K8s rollout undo.
  - `Engine` (`RegisterExecutor`, `Execute`, `ApproveRollback`, `GetRollback`, `ListRollbacks`).
  - Optional approval gates (`RequireApproval` flag).
  - State machine (Epic 15's `pkg/statemachine`): `Pending → (Approved|Rejected) → InProgress → (Completed|Failed) → (Verifying → Verified|VerificationFailed)`.

- REST: `/api/v1/gitops/verifications` (GET list, get), `/api/v1/gitops/rollback` (POST), `/api/v1/gitops/rollbacks` (GET list, get), `/api/v1/gitops/rollbacks/{id}/approve` (POST).
- `cmd/kscore-gitops` CLI: `verify <workflow-file>`, `rollback --app X --strategy previous|specific|last-known-good --revision Y --reason Z`.

### Outbound webhooks (`internal/webhook/outbound/`)

- `SubscriptionStore` — SQLite-backed CRUD; survives restart.
- `Subscription{ID, Name, URL, Secret, Events []string, Enabled bool, Headers map[string]string, MaxRetries int, TimeoutSec int, CreatedAt, UpdatedAt}`.
- `DeliveryRecord{ID, SubscriptionID, EventType, EventID, Status (pending|success|failed|retrying), StatusCode, Attempt, Error, DeliveredAt}`.
- `Manager` — subscribes to NATS event bus on `>` (cluster-prefixed); pattern-matches each event against each enabled subscription's filter list (glob); fans out async.
- `Dispatcher` — `Deliver(ctx, sub, payload, deliveryID) → (statusCode, error)`. HTTP POST with custom headers, HMAC-SHA256 signature header (`X-Keystone-Signature: sha256=<hex>`), per-subscription timeout.
- `RetryQueue` — exponential backoff with jitter; max retries default 3; on exhaustion → delivery `failed` (history retained).
- Per-endpoint **circuit breaker** (`closed → open` after 5 failures → `half-open` after 30s → `closed` after 2 successes).
- Concurrency: `sync.WaitGroup` tracks in-flight; bounded goroutines for back-pressure; `Stop()` waits for drain.
- Secret masking in API responses (`***`); cleartext returned only on creation.
- REST: `GET/POST /api/v1/webhooks/subscriptions`, `GET/PATCH/DELETE /api/v1/webhooks/subscriptions/{id}`, `POST {id}/test`, `GET {id}/deliveries`.
- CLI: `kscore-webhook outbound list|create|show|delete|history|test`.

## Scope (out / non-goals)

- Multi-env promotion pipelines (sequential dev → staging → prod with approvals + state machine) — v1.1.
- Basic remediation strategies (rollback action) — v1.1.
- Canary deployments — v1.2.
- Threshold evaluation per canary step — v1.2.
- Advanced remediation (scale-down, traffic shift, custom workflows) — v1.3.
- Diagnostic collection on remediation — v1.3.
- Git sync orchestration + multi-repo — v1.4.
- Helm/Kustomize-native integration — v1.5.
- Deployment dependency graph — v1.5.
- Webhook timestamp validation + nonce dedup — v1.1.
- Inbound webhooks for non-GitOps event sources — v1.1.
- Webhook body templating — v1.1.
- Per-destination rate limiting — v1.1.

## Design summary

See `PROJECT-DETAILS.md §4.13` (GitOps), §4.14 (Outbound webhooks).

## Tasks

### GitOps

1. **Webhook receiver HTTP server** + `Handler` interface + 4 concrete handlers (Argo, Flux, GitHub, GitLab).
2. **Source auto-detection** by headers.
3. **`Authenticator` interface** + HMACAuthenticator + BearerAuthenticator + NoneAuthenticator.
4. **Event normalization** + `ToKscoreEvent` emission on event bus.
5. **`Verifier` interface** + 3 v1.0 verifiers.
6. **`Workflow` execution engine** — sequential + parallel; per-step retries + timeout.
7. **Rollback `Executor` interface** + 3 v1.0 executors (Git via `go-git`, ArgoCD via gRPC client, K8s via `client-go`).
8. **`Engine`** — registers executors, manages rollback state machine + approval gates.
9. **Verification result + rollback persistence** in DB.
10. **REST handlers** + `cmd/kscore-gitops` CLI.

### Outbound webhooks

11. **`SubscriptionStore` SQLite impl** (extends `internal/state.Store`).
12. **`Manager`** — subscribes to event bus; pattern-matches; async fan-out with WaitGroup.
13. **`Dispatcher`** — HTTP POST + custom headers + HMAC signing + timeout.
14. **`RetryQueue`** — exp backoff + jitter.
15. **Circuit breaker** per endpoint with state transitions.
16. **REST handlers** + `kscore-webhook outbound` CLI.
17. **Sign + Verify** helpers (`Sign(secret, payload) → "sha256=<hex>"`, `Verify(secret, signature, payload) → bool`).
18. **Integration test**: end-to-end (event emitted → matching subscription delivered with valid HMAC sig → record stored).

## Acceptance criteria

### GitOps

- [x] ArgoCD webhook with valid HMAC ingested → emits `gitops.argocd.sync_succeeded` event. _(tasks 1-4: receiver→HMAC auth→parse→`ToKscoreEvent`; verified end-to-end via httptest receiver + fake emitter asserting the exact type. Live-server form is boot-gated — see the gate-v1.0 "GitOps webhook receiver boot registration" ROADMAP entry.)_
- [x] GitHub webhook with valid HMAC + GitHub event header parsed correctly. _(task 1 GitHub handler + task 3 HMAC; receiver httptest-verified.)_
- [ ] `kscorectl gitops verify webhook-verify.yaml --parallel --timeout 2m` runs HTTP + gRPC + cmd verifiers.
- [ ] `kscorectl gitops rollback --app web --strategy previous --reason "hotfix"` triggers rollback.
- [ ] Rollback with `RequireApproval=true` waits at Pending; `kscorectl gitops rollback approve <id>` proceeds.
- [x] Git-revert executor commits revert and pushes (in test env). _(task 7: `rollback/gitexec` go-git v5; `TestClient_Revert_CommitsAndPushes` seeds a bare remote, reverts to a prior commit, pushes, re-clones and asserts the revert commit's parent + tree + restored file content.)_

### Outbound webhooks

- [ ] `kscore-webhook outbound create --name slack --url https://hooks.slack.com/... --events 'state.drift,policy.violation' --secret xxx` succeeds.
- [ ] Test endpoint `POST /api/v1/webhooks/subscriptions/{id}/test` delivers a synthetic payload.
- [ ] HMAC signature validates on receiver side.
- [ ] Circuit breaker opens after 5 consecutive failures; recovers via half-open.
- [ ] Retries with exp backoff; eventual `failed` after 3 attempts retained in history.
- [ ] `GET /api/v1/webhooks/subscriptions/{id}/deliveries` lists delivery history.
- [ ] Coverage >75% on both `internal/gitops` and `internal/webhook/outbound`.

## Risks

- **Webhook signed-replay** in v1.0 — HMAC only. Capture+replay possible. Document; v1.1 adds timestamp window + nonce dedup.
- **Secret rotation** — single secret per auth method in v1.0; rotation requires restart. Document.
- **Rollback storms** — no cooldown in v1.0; rely on approval gates and operator judgment.
- **Slow receivers** — circuit breaker mitigates; per-delivery timeout caps.
- **Filter perf at high event volume** — glob patterns evaluated per event per subscription. Cache compiled glob in v1.1 if hot.
- **Delivery history growth** — `DeleteOldDeliveries` exists; auto-invocation in v0.1.x dot release.

## References

- PROJECT-DETAILS §4.13, §4.14.
