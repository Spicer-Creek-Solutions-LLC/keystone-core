# C02-I Acceptance Evidence

## Result

C02-I implements the persistence half of C02. The server store and agent
ledger are separate SQLite databases with independent migration histories and
APIs. The implementation uses `modernc.org/sqlite`, so it does not require
cgo. The agent crash harness exits with `os.Exit` at the selected boundary and
does not unwind database cleanup.

The checked-in ledger schema is [LEDGER-SCHEMA.md](../project/LEDGER-SCHEMA.md).
It is an independent operator-facing artifact; the AC-10 test compares it
with the schema exposed by the running ledger.

## Acceptance cases

`test/contract/persistence/contract_test.go` now provides live assertions for
all thirteen frozen C02 cases:

| Case | Evidence |
| --- | --- |
| AC-1 | Ordered server migrations record versions 1 and 2. |
| AC-2 | An injected failure before migration 2 leaves migration 1 committed and the store recoverable. |
| AC-3 | Server and agent stores migrate independently and do not contain each other's tables. |
| AC-4 | Receipt and start are separate ledger writes. |
| AC-5 | The crash harness distinguishes before-receipt, after-receipt, and after-start termination. |
| AC-6 | Accepted requests persist their job identifier and command arguments. |
| AC-7 | Terminal state and result are committed together, and an unknown job is rejected. |
| AC-8 | Result publication acknowledgement is a separate write after the terminal result. |
| AC-9 | The ledger can be read directly through SQLite without the server package. |
| AC-10 | The checked-in schema artifact matches the shipped ledger tables. |
| AC-11 | The contract verifies both store file modes required by ADR-0008. |
| AC-12 | Receipt retention removes records older than the supplied floor. |
| AC-13 | A non-SQLite ledger is refused as corrupt. |

The implementation does not create accounts or provision parent directories;
those remain C13 responsibilities. Callers provide the deployment-selected
path and provision its containing directory before opening a store.

## Verification

The focused contract was run with:

```text
go test -tags contract ./test/contract/persistence
go test ./internal/store ./internal/ledger
```

The pending manifest registers `store-directory-ownership` as a deferred C13
gate. C02 does not create or chmod the store directories, and no C02 test claims
to prove which account owns them.

Package tests cover rollback of an unknown server job and rejection of an
agent start without a durable receipt; the contract remains the acceptance
authority for the full behavior.
