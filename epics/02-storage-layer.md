# Epic 02: Storage Layer

**Phase**: A • **Estimate**: 1 week • **Depends on**: 01 • **Blocks**: 04, 06, 07, 08, 11, 12, 13, 14, 15

## Goal

Persistent state of record. Both SQLite (single-server / dev) and PostgreSQL (cluster / prod) backends behind a single `Store` interface. Auto-DDL on first start. SQLite → PostgreSQL migration tool with dry-run, batch sizing, txlog, and validation.

## Scope (in)

- `Store` root interface composing sub-interfaces (`AgentStore`, `CommandStore`, `BatchJobStore`, `HealthStore`). Other domains (events, secrets metadata, audit, policy, cluster) extend with their own sub-interfaces in their respective epics.
- `SQLiteStore` and `PostgreSQLStore` implementations with direct parametrized SQL (no ORM). Allowlisted sort columns.
- Connection pool defaults: SQLite max-open=1 (single writer), Postgres max-open=25 max-idle=5 conn-max-life=30m, all configurable.
- Auto-schema initialization (`CREATE TABLE IF NOT EXISTS`) on `NewStore` for the v1.0 baseline schema (`agents`, `commands`, `batch_jobs`, `batch_agent_results`).
- IPv6-safe Postgres DSN builder (brackets only for IPv6 literals).
- JSON-encoded complex columns (labels, env, IPs) — surface unmarshal errors loudly, do **not** silently swallow.
- `cmd/kscore-migrate` — SQLite → PostgreSQL migrator with `Migrator{Migrate, ValidateMigration}`, `MigrationOptions{DryRun, BatchSize default 100, ContinueOnError, SkipExisting, ProgressCallback}`, `MigrationStats`, `TransactionLog` (audit trail of operations with checkpoints), `ProgressReporter` (per-table rate + ETA).
- Migration order: agents → commands → batch_jobs → batch_agent_results (FK order). `INSERT ... ON CONFLICT DO NOTHING` when SkipExisting.

## Scope (out / non-goals)

- Schema versioning / `golang-migrate` — v1.1.
- Encryption at rest (`KeyProvider`) — v1.5; KeyProvider scaffolding only if it costs nothing.
- Multi-table transaction wrapper (`Tx` type) — v1.2.
- Backup/restore as Store API methods — `kscore-backup` (Epic 18) handles ops-level backup.
- Cloud KMS integration — v2.0.
- Loki / Prometheus / Jaeger query backends — v1.4.

## Design summary

See `PROJECT-DETAILS.md §4.3` (Storage Layer).

## Tasks

1. **`Store` + sub-interfaces** in `internal/state/store.go`. Define `Config{Backend (sqlite|postgresql), SQLite{Path, WAL, BusyTimeout}, PostgreSQL (DSN or struct), MaxOpenConns, MaxIdleConns, ConnMaxLife}`. `NewStore(config) (Store, error)` factory.
2. **Schema definitions** in `internal/state/schema.go` (or per-table files). DDL embedded in `initSchema()`. Indexes on status, timestamps, agent_id.
3. **`SQLiteStore`** implementation + tests (in `t.TempDir()`). Verify WAL on, busy_timeout, FK enforced.
4. **`PostgreSQLStore`** implementation + tests (skipped if no `KSCORE_TEST_POSTGRES_DSN`; CI uses docker-compose).
5. **JSON column handling** with explicit error surfacing on unmarshal failure (this is a fix vs the existing code — do not regress).
6. **IPv6-safe DSN builder** with helper + unit tests.
7. **`Migrator`** + `MigrationOptions` + `MigrationStats` + `TransactionLog` (txlog persisted to disk for recovery) + `ProgressReporter`.
8. **`cmd/kscore-migrate`** CLI (`run`, `validate`, `version`).
9. **End-to-end migration test** in CI: SQLite source → Postgres target → ValidateMigration passes.

## Acceptance criteria

- [ ] `Store` interface fully wired for both backends; all sub-interfaces stub-or-real.
- [ ] Auto-DDL on first run; second run is a no-op (no errors).
- [ ] Connection pools configured per backend.
- [x] IPv6 DSN literals work (`postgres://user:pw@[::1]:5432/db`).
- [ ] JSON unmarshal errors return errors, not empty maps.
- [ ] `kscore-migrate run --dry-run --sqlite ./test.db --postgres "postgres://..." --batch-size 100` produces accurate migration plan.
- [ ] Real migration with 10k agents + 100k commands completes; `validate` reports identical row counts.
- [ ] `--continue-on-error` records errors in txlog and continues.
- [ ] Coverage >80% for `internal/state` and migrator.

## Risks

- **SQLite single-writer constraint**: tests must not assume concurrent writes work; race tests on writes will deadlock if not constrained.
- **Postgres docker-compose flakiness in CI**: gate Postgres tests behind env var; ensure embedded NATS smoke runs without Postgres.
- **JSON schema drift**: when later epics add sub-interfaces (events, secrets metadata, audit), schema-init order matters — document and enforce with tests.

## References

- PROJECT-DETAILS §4.3, §3.2 (deps).
