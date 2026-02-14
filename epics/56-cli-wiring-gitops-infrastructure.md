# Epic 56: CLI Wiring — GitOps & Infrastructure

## Overview

Replace hardcoded sample data and placeholder commands in the remaining CLI binaries: `kscore-gitops`, `kscore-webhook`, `kscore-agents`, `kscore-module`, `kscorectl`, `kscore-files`, and `kscore-monitor`. These CLIs have ~25 stub commands that use fake data or print messages without performing real operations.

## Goal

All remaining CLI commands make real API calls. Fallback-to-sample-data patterns are replaced with clear error messages when the server is unavailable.

## Success Criteria

- [ ] `kscore-gitops`: all 10 stub commands wired to real GitOps API
- [ ] `kscore-webhook`: 3 remaining stub commands wired to real webhook API
- [ ] `kscore-agents`: remove sample data fallbacks, wire `re-enroll` to real API
- [ ] `kscore-module`: replace mock registry client, implement SumDB verification, implement `--coverage`
- [ ] `kscorectl`: remove mock maintenance data fallbacks
- [ ] `kscore-files`: implement mirror `--wait` and `failover` commands
- [ ] `kscore-monitor`: wire `EventRate` and `APIRequestRate` to real server metrics
- [ ] No `generateSample*` or `outputSample*` functions remain

## Dependencies

- **Epic 5** (GitOps): GitOps engine and webhook receiver exist
- **Epic 49** (REST Handler Wiring): GitOps, webhook, discovery handlers wired
- **Epic 22** (File Distribution): Mirror sync engine exists
- **Epic 44** (Join Tokens): Token API exists for agent re-enrollment

## Technical Tasks

### Phase 1: GitOps CLI (Week 1-2)

**T1.1: Add GitOps Query REST API (if missing)**
- Verify existing REST endpoints for rollback, promotion, status, repo management, deployments
- Add any missing endpoints needed by the CLI

**T1.2: Wire `gitops rollback` and `gitops promote`**
- Replace mock `rollback.Result` and `promotion.Result` with real API calls
- Connect to control plane GitOps engine

**T1.3: Wire `gitops status`, `gitops repo *`**
- Replace `generateSampleOperationStatuses()` and `generateSampleRepoConfigs()` with real API calls
- `repo add/remove/sync` should make real REST calls instead of printing messages

**T1.4: Wire `gitops deploy *`**
- Replace hardcoded deployment data with real deployment tracking API
- `deploy rollback` and `deploy approve` should trigger real operations

### Phase 2: Webhook, Agents, Module CLIs (Week 3)

**T2.1: Wire `webhook test`**
- Replace `generateSamplePayload()` with actually sending a test webhook to a configured endpoint
- Or generate a real payload from an actual recent event

**T2.2: Wire `webhook history` and `webhook secrets rotate`**
- `history`: Query real delivery history from outbound webhook store (Epic 50)
- `secrets rotate`: Implement actual webhook secret rotation via API

**T2.3: Fix `agents list/show` fallback pattern**
- Remove `outputSampleAgents()` and `outputSampleAgentDetail()` fallbacks
- When gRPC connection fails, show a clear error message instead of fake data

**T2.4: Wire `agents re-enroll`**
- Replace fake token generation with real gRPC call to invalidate credentials and generate new enrollment token

**T2.5: Wire `module resolve` with real registry client**
- Replace `mockRegistryClient` with real `RegistryClient` from `pkg/module/registry/`
- Connect to configured registry URL

**T2.6: Implement SumDB verification**
- File: `cmd/kscore-module/cmd_verify.go` line 143
- Replace "SKIPPED (not yet implemented)" with actual SumDB transparency log query

**T2.7: Implement `module test --coverage`**
- File: `cmd/kscore-module/cmd_startest.go` line 173
- Implement Starlark code coverage tracking during test execution

### Phase 3: Remaining CLIs (Week 4)

**T3.1: Fix `kscorectl maintenance` fallbacks**
- Remove mock data for `maintenance queue` and `maintenance cleanup`
- Show error message when server is unavailable

**T3.2: Implement `kscore-files mirror --wait`**
- File: `cmd/kscore-files/commands_mirrors.go` line 580
- Poll sync status until completion instead of returning immediately

**T3.3: Implement `kscore-files mirror failover`**
- File: `cmd/kscore-files/commands_mirrors.go` line 749
- Implement actual routing update to force traffic to specified mirror

**T3.4: Wire `kscore-monitor` server metrics**
- File: `cmd/kscore-monitor/client/client.go` line 352
- Add server-side `EventRate` and `APIRequestRate` metrics endpoints
- Wire monitor client to fetch real values

## Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| SumDB verification requires transparency log infrastructure | Start with signature verification; SumDB can be a later phase |
| Starlark coverage tracking may be complex | Instrument the Starlark interpreter's statement visitor |
| Mirror failover needs routing infrastructure | May require NATS-based routing updates; design carefully |

## Definition of Done

- [ ] All `generateSample*` and `outputSample*` functions removed
- [ ] All commands either make real API calls or show clear "not available" errors
- [ ] No commands silently show fake data
- [ ] `make test` and `make lint` pass
- [ ] CLI reference documentation updated
