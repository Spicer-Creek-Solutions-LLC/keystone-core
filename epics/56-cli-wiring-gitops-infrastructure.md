# Epic 56: CLI Wiring — GitOps & Infrastructure

## Status: COMPLETE ✅

## Overview

Replace hardcoded sample data and placeholder commands in the remaining CLI binaries: `kscore-gitops`, `kscore-webhook`, `kscore-agents`, `kscore-module`, `kscorectl`, `kscore-files`, and `kscore-monitor`. These CLIs had ~25 stub commands that used fake data or printed messages without performing real operations.

## Goal

All remaining CLI commands make real API calls. Fallback-to-sample-data patterns are replaced with clear error messages when the server is unavailable.

## Success Criteria

- [x] `kscore-gitops`: rollback wired to REST API; remaining 9 commands return "not yet available" (no server endpoints exist)
- [x] `kscore-webhook`: inbound stubs (list, show, history, secrets) return "not yet available"; test wired to POST real payload
- [x] `kscore-agents`: removed sample data fallbacks; wired re-enroll and token create to POST /api/v1/cluster/tokens
- [x] `kscore-module`: replaced mockRegistryClient with real registry.HTTPClient; SumDB returns "not yet available"; --coverage returns early error
- [x] `kscorectl`: removed 5 maintenance mock data fallbacks (enable, disable, status, queue, cleanup)
- [x] `kscore-files`: --wait prints stderr warning; failover returns "not yet available"
- [x] `kscore-monitor`: no changes needed — hardcoded 0 metrics are correct until server-side aggregation exists
- [x] No `generateSample*` or `outputSample*` functions remain in modified binaries

## Dependencies

- **Epic 5** (GitOps): GitOps engine and webhook receiver exist
- **Epic 44** (Join Tokens): Token API exists for agent re-enrollment
- **Epic 22** (File Distribution): Mirror sync engine exists
- **Epic 50** (Outbound Webhooks): Outbound webhook commands already wired

## Implementation Summary

### Phase 1: kscore-gitops (COMPLETE)

- Created `cmd/kscore-gitops/rest_client.go` — REST client for rollback API
- Wired `rollback` to POST `/api/v1/gitops/rollback`
- Wired remaining 9 commands (promote, status, repo list/add/remove/sync, deploy list/rollback/approve, git-sync) to return "not yet available" errors
- Removed 8 display types and 8 sample generator functions
- Added `--server` persistent flag

### Phase 2: kscore-webhook + kscore-agents (COMPLETE)

**kscore-webhook:**
- Replaced inbound stubs (list, show, history, secrets list, secrets rotate) with "not yet available" errors
- Wired `test` command to POST real HTTP payload to server webhook endpoint
- Removed WebhookHandler, WebhookDelivery, WebhookSecret types and sample generators

**kscore-agents:**
- Removed `outputSampleAgents()` and `outputSampleAgentDetail()` fallbacks — gRPC failures now return errors
- Wired `re-enroll` and `token create` to POST `/api/v1/cluster/tokens` REST API
- Removed broken `generateRandomToken()` function (used time.Now().UnixNano() with time.Sleep)
- Added `createToken()` helper and `getAPIScheme()` for REST API access

### Phase 3: kscore-module + kscorectl + kscore-monitor (COMPLETE)

**kscore-module:**
- Replaced `mockRegistryClient` with real `registry.HTTPClient` from `pkg/module/registry`
- Added `--registry` flag to resolve command (defaults to $KSCORE_REGISTRY)
- Moved `--coverage` check to fail early with clear error
- Updated SumDB message to "not yet available"

**kscorectl:**
- Removed 5 maintenance command fallbacks that showed mock data on connection failure
- Commands now return proper errors: `failed to enable/disable/get maintenance ...`

**kscore-monitor:**
- No changes needed — hardcoded 0 metrics are correct behavior

### Phase 4: kscore-files + Documentation (COMPLETE)

**kscore-files:**
- `mirror sync --wait` prints warning to stderr and continues
- `mirror failover` returns "not yet available" error

## Definition of Done

- [x] All `generateSample*` and `outputSample*` functions removed from modified binaries
- [x] All commands either make real API calls or show clear "not available" errors
- [x] No commands silently show fake data
- [x] `make test` and `make lint` pass
- [x] CLI reference documentation updated
