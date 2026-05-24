# Keystone Core Operational Runbooks

This directory contains operational runbooks for common Keystone Core
procedures. Runbooks are written for **operators on production
hosts** — if you're looking for a first-time install tutorial, use
[`../project/GETTING-STARTED.md`](../project/GETTING-STARTED.md)
instead; if you're hacking on the source, see
[`../project/DEVELOPMENT.md`](../project/DEVELOPMENT.md).

> **v0.1.x scope note.** Every runbook in this directory was
> rewritten during Phase E of the public-launch checklist to match
> v0.1 reality — the `apt install` / `dnf install` package path,
> `systemd`-managed services, and `kscorectl` for CLI interaction.
> Each runbook carries an inline `**v0.x scope note:**` callout
> wherever v0.1 narrows the full v1.0 surface (e.g. multi-node
> cluster operations that depend on capabilities not yet wired at
> boot).

## I'm here because…

**…I need to install Keystone Core on a new host.**
→ [`bootstrap-new-cluster.md`](bootstrap-new-cluster.md). The dense
reference walkthrough — package install, postinst behavior, filesystem
layout, port table, verification matrix.

**…I'm upgrading or scheduling maintenance.**
→ [`upgrade-cluster.md`](upgrade-cluster.md) for the single-node
upgrade path. [`scheduled-maintenance.md`](scheduled-maintenance.md)
for planned-window procedures.

**…something broke; I need to roll back fast.**
→ [`emergency-rollback.md`](emergency-rollback.md). Four v0.1
scenarios (package downgrade, state rollback, config revert,
catastrophic restore).

**…I need to back up or restore.**
→ [`backup-restore.md`](backup-restore.md) for routine procedures;
[`disaster-recovery.md`](disaster-recovery.md) for the six concrete DR
scenarios (SQLite / Postgres / JetStream / Identity / Config /
Total-host-loss) + the RTO/RPO drill procedure.

**…there's a security incident.**
→ [`security-incident.md`](security-incident.md). Five-phase response
(immediate / investigation / containment / verification /
post-incident) with real `kscorectl audit log` /
`kscorectl events stream` / `kscorectl identity *` commands.

**…something is slow or timing out.**
→ [`performance-degradation.md`](performance-degradation.md) for the
diagnostic tree; [`capacity-scaling.md`](capacity-scaling.md) for
"is it time to scale?" decisions.

**…I need to rotate TLS certificates.**
→ [`certificate-rotation.md`](certificate-rotation.md).

**…something's wrong but I'm not sure what.**
→ [`troubleshooting.md`](troubleshooting.md). The general diagnostic
tree — server-won't-start, agent-won't-register, journal walkthroughs,
diagnostic-bundle script.

## Available Runbooks

| Runbook | Description | When to Use |
|---------|-------------|-------------|
| [bootstrap-new-cluster.md](bootstrap-new-cluster.md) | Bootstrap a new Keystone Core cluster | Initial deployment |
| [scheduled-maintenance.md](scheduled-maintenance.md) | Perform scheduled maintenance | Planned maintenance windows |
| [emergency-rollback.md](emergency-rollback.md) | Emergency rollback procedure | Failed upgrade or critical issues |
| [disaster-recovery.md](disaster-recovery.md) | Disaster recovery procedures | Complete cluster loss |
| [certificate-rotation.md](certificate-rotation.md) | Rotate TLS certificates | Certificate expiry or compromise |
| [backup-restore.md](backup-restore.md) | Backup and restore procedures | Data protection and recovery |
| [upgrade-cluster.md](upgrade-cluster.md) | Upgrade Keystone Core cluster | Version upgrades |
| [troubleshooting.md](troubleshooting.md) | Common troubleshooting procedures | Diagnosing issues |
| [performance-degradation.md](performance-degradation.md) | Diagnose and remediate performance issues | API latency, timeouts, slowness |
| [security-incident.md](security-incident.md) | Security incident response procedures | Breaches, credential compromise, attacks |
| [capacity-scaling.md](capacity-scaling.md) | Scale infrastructure for increased load | Growth, resource constraints |

## Runbook Format

Each runbook follows a consistent format:

1. **Overview** - What the runbook covers
2. **Prerequisites** - Required access, tools, information
3. **Trigger Conditions** - When to execute this runbook
4. **Procedure** - Step-by-step instructions
5. **Verification** - How to verify success
6. **Rollback** - How to undo if needed
7. **Post-Procedure** - Cleanup and documentation

## Using Runbooks

1. Read the entire runbook before starting
2. Gather all prerequisites
3. Notify stakeholders if required
4. Execute steps in order
5. Document any deviations
6. Complete verification steps
7. Update incident/change ticket

## Contributing

When creating or updating runbooks:

- Use clear, unambiguous language
- Include exact commands (copy-pasteable)
- Add verification steps after each major action
- Include rollback procedures
- Test procedures in staging first
- Keep runbooks up to date with product changes
