# Generation 2 Requirements Traceability

This register prevents architecture requirements from existing only in prose.
The test paths are targets for the clean baseline; they become mandatory as the
corresponding implementation lands.

**`Lands at` names the task that owes each row's evidence**, and it is what
makes the register's own rule enforceable. That rule — *"CI must fail when an
implemented invariant lacks its registered evidence"* — has an empty precondition
while nothing is implemented, so it would pass as coverage for the whole of Stage
C. With this column, `tools/archlint` derives liveness from the epic: **when a
task is ticked, its rows must have their evidence.** No dates to maintain, and
the rule becomes live task by task.

**Design owners are identifiers, not prose.** *"NATS topology ADR"* cannot be
checked against anything; `ADR-0002` can. That is how a *"Test architecture ADR"*
sat in this register pointing at a document nobody had written, until `ADR-0010`
became it.

**This is the only invariant-to-evidence map.** [`ADR-0010`](../adr/0010-acceptance-harness.md)
§ 13 designs the harness that produces the evidence and deliberately holds no
table of its own, so a row is updated here rather than in two places. `ADR-0010`
§ 14 states what this register cannot do: a row naming a file that does not exist
is a target, not enforcement, and `ARCH-TEST-003` is not satisfied until one
does.

| Requirement | Design owner | Planned automated evidence | Gate | Lands at |
|---|---|---|---|---|
| ARCH-COMM-001 | `ADR-0002` | `test/architecture/nats_only_test.go` | PR | `C06` |
| ARCH-COMM-002 | `ADR-0002` | `test/e2e/docker/network_isolation_test.go` | PR | `P11` |
| ARCH-COMM-003 | `ADR-0010` | `test/e2e/docker/journey_test.go` | PR | `C05` |
| ARCH-NATS-001 | `ADR-0002` | `test/e2e/docker/accounts_test.go` | PR | `C03` |
| ARCH-NATS-002 | `ADR-0003` | `test/e2e/docker/identity_inventory_test.go` | PR | `C03` |
| ARCH-NATS-003 | `ADR-0004` | `test/e2e/docker/permissions_test.go` | PR | `C03` |
| ARCH-NATS-004 | `ADR-0003` | `test/e2e/docker/enrollment_test.go` | PR | `C05` |
| ARCH-NATS-005 | `ADR-0004` | `test/e2e/docker/system_subject_permissions_test.go` | PR | `C03` |
| ARCH-NATS-006 | `ADR-0005` | `test/e2e/docker/envelope_confidentiality_test.go`, `test/e2e/docker/envelope_signature_test.go` | PR | `C08` |
| ARCH-NATS-007 | `ADR-0002` | `test/e2e/docker/resource_limits_test.go` | Main | `C15` |
| ARCH-NATS-008 | RFC/ADR process | `tools/archlint` decision-matrix rule | PR | `P11` |
| ARCH-NATS-009 | `ADR-0006` | exact-filter serialized-consumer test | PR | `C06` |
| ARCH-NATS-010 | `ADR-0006` | redelivery exhaustion/advisory test | Main | `C06` |
| ARCH-JOB-001 | `ADR-0006` | `test/contract/protocol/contract_test.go` | PR | `C01` |
| ARCH-JOB-002 | `ADR-0006` | restart boundary suite | Main | `C02` |
| ARCH-JOB-003 | `ADR-0006` | non-idempotent duplicate-delivery test | PR | `C08` |
| ARCH-JOB-004 | `ADR-0006` | crash ambiguity tests | Main | `C14` |
| ARCH-JOB-005 | `ADR-0006` | publish/ack fault matrix | Main | `C14` |
| ARCH-EXEC-001 | `ADR-0007` | execution limits black-box suite | PR | `C07` |
| ARCH-EXEC-002 | `ADR-0007` | descendant-process cancel test | PR + VM | `C07` |
| ARCH-OBS-001 | `ADR-0008` | correlated lifecycle audit test | PR | `C12` |
| ARCH-TEST-001 | `ADR-0010` | per-feature effect assertions | PR | `C08` |
| ARCH-TEST-002 | `ADR-0004` | negative identity matrix | PR | `C03` |
| ARCH-TEST-003 | `ADR-0010` | `tools/archlint` coverage rule | PR | `P11` |

## Maintenance rules

- A new normative requirement receives a stable identifier before code lands.
- A requirement cannot be marked implemented without a link to executable
  evidence or an approved manual-review exception.
- Renaming a test requires updating this register in the same pull request.
- **Changing a normative invariant's text requires an RFC, in either
  direction.** Removing or weakening one obviously does; strengthening one
  does too, because a requirement every later ADR is measured against should
  not change under a task approval. This rule previously covered only
  removal and weakening, while
  [`REBOOT-EXECUTION-PLAN.md`](REBOOT-EXECUTION-PLAN.md) already stated the
  broader rule; [RFC 0002](../rfcs/0002-signed-envelope-classes.md) was the
  first amendment either governed, and settled it here.
- CI must fail when an implemented invariant lacks its registered evidence.
