# Generation 2 Requirements Traceability

This register prevents architecture requirements from existing only in prose.
The test paths are targets for the clean baseline; they become mandatory as the
corresponding implementation lands.

| Requirement | Design owner | Planned automated evidence | Gate |
|---|---|---|---|
| ARCH-COMM-001 | NATS topology ADR | `test/architecture/nats_only_test.go` | PR |
| ARCH-COMM-002 | NATS topology ADR | `test/e2e/docker/network_isolation_test.go` | PR |
| ARCH-COMM-003 | Test architecture ADR | `test/e2e/docker/journey_test.go` | PR |
| ARCH-NATS-001 | NATS topology ADR | `test/e2e/docker/accounts_test.go` | PR |
| ARCH-NATS-002 | NATS identity ADR | `test/e2e/docker/identity_inventory_test.go` | PR |
| ARCH-NATS-003 | Subject authorization ADR | `test/e2e/docker/permissions_test.go` | PR |
| ARCH-NATS-004 | Enrollment ADR | `test/e2e/docker/enrollment_test.go` | PR |
| ARCH-NATS-005 | Subject authorization ADR | `test/e2e/docker/system_subject_permissions_test.go` | PR |
| ARCH-NATS-006 | Protocol ADR | `test/e2e/docker/envelope_confidentiality_test.go`, `test/e2e/docker/envelope_signature_test.go` | PR |
| ARCH-NATS-007 | NATS topology ADR | `test/e2e/docker/resource_limits_test.go` | Main |
| ARCH-NATS-008 | RFC/ADR process | `tools/archlint` decision-matrix rule | PR |
| ARCH-NATS-009 | Job lifecycle ADR | exact-filter serialized-consumer test | PR |
| ARCH-NATS-010 | Job lifecycle ADR | redelivery exhaustion/advisory test | Main |
| ARCH-JOB-001 | Job lifecycle ADR | protocol conformance tests | PR |
| ARCH-JOB-002 | Job lifecycle ADR | restart boundary suite | Main |
| ARCH-JOB-003 | Job lifecycle ADR | non-idempotent duplicate-delivery test | PR |
| ARCH-JOB-004 | Job lifecycle ADR | crash ambiguity tests | Main |
| ARCH-JOB-005 | Job lifecycle ADR | publish/ack fault matrix | Main |
| ARCH-EXEC-001 | Execution ADR | execution limits black-box suite | PR |
| ARCH-EXEC-002 | Execution ADR | descendant-process cancel test | PR + VM |
| ARCH-OBS-001 | Audit ADR | correlated lifecycle audit test | PR |
| ARCH-TEST-001 | Test architecture ADR | per-feature effect assertions | PR |
| ARCH-TEST-002 | Subject authorization ADR | negative identity matrix | PR |
| ARCH-TEST-003 | Test architecture ADR | `tools/archlint` coverage rule | PR |

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
