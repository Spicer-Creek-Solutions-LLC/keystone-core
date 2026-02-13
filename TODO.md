# TODO.md

This is a TODO list of work that still needs to be done outside any current epic.

## Resolution Tags

Each TODO item includes a `Resolution:` line to indicate how it should be addressed:

- `doc` — update documentation to match current code behavior.
- `code` — update code to add the documented behavior and update documents to new behavior.
- `both` — update both docs and code.
- `decide` — needs triage to choose a direction.

---

## Open Items

### E2E Test Infrastructure Gaps

The following E2E tests are skipped because the container test environment lacks the necessary configuration. These represent real feature coverage gaps in the E2E suite.

**Scenario tests — missing service configuration in E2E containers:**

- [ ] `TestEvent_ReactorTrigger` — Reactor trigger test requires reactor configuration in E2E containers. Resolution: `code`
- [ ] `TestGitOps_WebhookAuthHMAC` — Webhook HMAC auth test requires webhook receiver in E2E containers. Resolution: `code`
- [ ] `TestGitOps_WebhookAuthBearer` — Webhook Bearer auth test requires webhook receiver in E2E containers. Resolution: `code`
- [ ] `TestGitOps_VerificationTrigger` — Verification workflow test requires webhook configuration. Resolution: `code`
- [ ] `TestGitOps_RollbackTrigger` — Rollback automation test requires GitOps configuration. Resolution: `code`
- [ ] `TestGitOps_RollbackApproval` — Rollback approval test requires GitOps configuration. Resolution: `code`
- [ ] `TestGitOps_WebhookEventEmission` — Event emission test requires webhook configuration. Resolution: `code`
- [ ] `TestPolicy_EnforcementModeAudit` — Audit mode test requires policy configuration. Resolution: `code`
- [ ] `TestPolicy_EnforcementModeWarn` — Warn mode test requires policy configuration. Resolution: `code`
- [ ] `TestPolicy_ViolationBlocking` — Violation blocking test requires policy configuration. Resolution: `code`
- [ ] `TestPolicy_AuditLogging` — Audit logging test requires policy configuration. Resolution: `code`
- [ ] `TestPolicy_ComplianceReporting` — Compliance reporting test requires policy API endpoint. Resolution: `code`

**HA cluster tests — missing infrastructure control:**

- [ ] `TestHACluster_NATSFailure` — Requires NATS-specific container control (separate NATS container). Resolution: `code`
- [ ] `TestHACluster_EtcdFailure` — Requires etcd-specific container control (separate etcd container). Resolution: `code`
- [ ] `TestHACluster_DatabaseFailover` — Requires PostgreSQL replica configuration. Resolution: `code`
- [ ] `TestHACluster_NetworkPartition` — Requires special network configuration (iptables/tc). Resolution: `code`
- [ ] `TestHACluster_SplitBrain` — Requires network partition capability. Resolution: `code`

---

Items moved to epics:

- Secrets API implementation — Epic 43 (`epics/43-secrets-api-implementation.md`)
- REST API handler wiring — Epic 49 (`epics/49-rest-api-handler-wiring.md`)
- Outbound webhook subscriptions — Epic 50 (`epics/50-outbound-webhooks.md`)

---

## Notes

- Test coverage targets: >70% for critical packages, >40% for CLI
- Performance benchmarks should be tracked in CI/CD with regression alerting
- All new features should include comprehensive documentation and tests
- Security considerations should be reviewed for all changes
- Documentation should be updated alongside code changes

---

## Summary Statistics

| Category | Count |
|----------|-------|
| Open — E2E scenario gaps | 12 |
| Open — HA infrastructure gaps | 5 |
| **Total Open** | **17** |
| Previously Resolved | 210 |
| Moved to Epics | 3 |
