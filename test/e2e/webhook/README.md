# Outbound Webhook E2E Suite

Epic 16 task 18 — the build-tagged integration suite that wires the
entire outbound-webhooks pipeline as kscore-server boot will compose
it and proves the §4.14 contract end-to-end against a real
`httptest.Server` receiver.

## What it exercises

- Task 11 — `outbound.SQLiteStore(":memory:")` (the durable schema
  path; not the in-memory test double).
- Task 12 — `outbound.Manager` (push-driven `Handle`, glob filter,
  bounded fan-out, in-Manager retry loop).
- Task 13 — `outbound.HTTPDispatcher` (HTTP POST + custom headers
  + HMAC-signed via task 17 `Sign`).
- Task 14 — `RetryPolicy` + exponential backoff under the §4.14
  "max retries default 3" contract.
- Task 15 — `outbound.CircuitBreaker` (closed → open after 5
  consecutive failures; fast-fails the 6th attempt).
- Task 17 — `outbound.Verify` on the receiver side. The receiver
  IS the validator the epic asks for.

## Scenarios

1. **Happy path** — event emitted → matched → POST'd → receiver
   `Verify`s the X-Keystone-Signature → DeliveryRecord persisted
   `success/200/Attempt=1`.
2. **Retry exhaustion (acceptance line 115)** — receiver always
   returns 502; Manager retries `1 + MaxRetries == 3` times; every
   attempt is signed (receiver `Verify`s 3 times); final record
   `failed/Attempt=3/502` **retained** in the store.
3. **Circuit breaker** — wrap HTTPDispatcher with `CircuitBreaker`;
   after 5 failed events the breaker opens; the 6th event fast-
   fails without touching the receiver; the DeliveryRecord's error
   names the breaker.

## How to run

```
make test-integration
```

That's the same target Epic 13's `test/e2e/ha/` and Epic 15's
`test/e2e/blueprint/` suites run under (`-tags=integration`).

## Why it's build-tagged

The integration tag keeps the standard `make test` fast and
hermetic — these scenarios spin real `httptest.Server`s on real
loopback ports, exercise the SQLite schema, and serialize fan-out
through real `sync.Mutex` / `sync.WaitGroup` primitives. None of
that is unsafe, but it is meaningfully slower than the unit suite
and shouldn't run on every `go test ./...` invocation.
