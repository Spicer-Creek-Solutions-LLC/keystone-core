# Runbook: Emergency Rollback

## Overview

Rollback procedures for the v0.x line. Keystone Core v0.x has no
built-in upgrade orchestrator; "emergency rollback" decomposes into
four scenarios depending on what changed. Each scenario uses tools
already shipped in v0.1 — the OS package manager, `kscorectl state
rollback`, `kscore-cluster-backup`, or a config revert.

> **v0.x scope note**: A unified `kscorectl upgrade rollback` command
> with automatic-rollback semantics and rollback-status metrics is
> tracked in
> [`docs/project/ROADMAP.md`](../project/ROADMAP.md). For v0.1, rollback
> is operator-driven, scenario-specific, and uses the primitives below.

## Trigger conditions

Execute this runbook when one of the following is observed shortly
after an upgrade or change:

- `/health/ready` returns non-`ready` for more than ~60 s after the
  startup grace period
- Error rate spikes in the Prometheus metric stream (the actual
  metric names live in `internal/metrics/metricdefs.go`; the agent-
  fleet dashboard at `deploy/grafana/dashboards/agent-fleet.json`
  surfaces the common ones)
- A material fraction of agents disconnect within ~5 minutes of the
  change going live
- A critical functional path is broken (state apply fails for previously
  working YAML, command exec returns errors, audit log writes fail)
- A new dependency-CVE makes the just-deployed version unsafe to keep
  running

## Severity assessment

| Indicator | Warning | Critical |
|---|---|---|
| `/health/ready` status | `ready=false` past grace | `ready=false` past grace + db/jetstream/nats components unhealthy |
| Error rate (kscore_*_errors_total) | > 2× pre-change baseline | > 5× pre-change baseline |
| Agent disconnection ratio | > 5% of fleet | > 10% of fleet |
| API availability | < 99.5% | < 99% |

Decide between *forward-fix* (reroll, patch, retry) and *rollback*
based on whether the change can be safely fixed in-place within ~15
minutes. If not, rollback.

## Scenario A — Server package upgrade rollback (.deb / .rpm)

Use when a recent `apt install kscore-server` or `dnf upgrade
kscore-server` introduced the regression.

```bash
# 1. Stop the service so packaging-manager state changes don't race
#    against in-flight requests.
sudo systemctl stop kscore-server

# 2. Note the installed version, list available versions.
dpkg -l kscore-server || rpm -q kscore-server
apt-cache madison kscore-server         # Debian/Ubuntu
dnf --showduplicates list kscore-server # Rocky/RHEL

# 3. Downgrade to the previous version.
sudo apt install -y kscore-server=0.0.1~snapshot-<short-commit>   # Debian/Ubuntu
sudo dnf downgrade -y kscore-server-0.0.1~snapshot-<short-commit> # Rocky/RHEL

# 4. Bring the service back.
sudo systemctl start kscore-server

# 5. Verify (post-grace).
sleep 35
curl -fsS http://127.0.0.1:8080/health/ready | jq '.ready, .components'
```

Pre-v0.1.0 snapshots use the format `0.0.1~snapshot-<short-commit>`;
once tagged releases exist, the form is `0.1.0-1`, `0.1.1-1`, etc.

## Scenario B — Bad state apply rollback

Use when a recent `kscorectl state apply` left declarations in a
worse state than before. v0.1 `kscorectl state rollback` re-applies
a prior stored run's declarations.

```bash
# 1. Identify the last-known-good run id.
kscorectl state history --limit 10

# 2. Inspect the run you intend to roll back to.
kscorectl state show <run-id>

# 3. Roll back by re-applying the prior run's declarations to the
#    same agent.
kscorectl state rollback <run-id> --agent <agent-id>

# 4. Verify the rolled-back state held.
kscorectl state drift --agent <agent-id>
```

## Scenario C — Bad config change rollback

Use when an edit to `/etc/kscore/server.yaml` (or `/etc/kscore/agent.yaml`)
introduced the regression. The default `/etc/kscore/server.yaml` is
rendered once by the package postinst from `/usr/share/kscore/server.yaml.template`;
operator edits are not tracked by the package manager.

```bash
# 1. If you have the prior config under version control or in a
#    sibling backup file, restore it:
sudo cp /etc/kscore/server.yaml.bak /etc/kscore/server.yaml
sudo chown root:kscore /etc/kscore/server.yaml
sudo chmod 0640 /etc/kscore/server.yaml

# 2. If you DON'T have a backup, regenerate from the shipped template
#    and re-apply your operator edits selectively. Note: this
#    regenerates the HMAC secret which invalidates every agent's
#    bootstrap PSK and requires re-issuance.
sudo install -o root -g kscore -m 0640 \
    /usr/share/kscore/server.yaml.template /etc/kscore/server.yaml
HMAC=$(openssl rand -hex 32)
sudo sed -i "s|__HMAC_SECRET__|${HMAC}|g" /etc/kscore/server.yaml

# 3. Restart and verify.
sudo systemctl restart kscore-server
sleep 35
curl -fsS http://127.0.0.1:8080/health/ready | jq '.ready'
```

## Scenario D — Catastrophic recovery (server won't start)

Use when the server fails to start or can't be diagnosed in-place
(corrupted SQLite, broken JetStream, mangled config). Restores from
a prior cluster backup snapshot. The state plane (SQLite or
Postgres), JetStream state, identity material, and operator configs
all roll back to the snapshot point in time.

```bash
# 1. List available snapshots in your configured backup destination.
kscore-cluster-backup list /var/backups/kscore/

# 2. Verify the snapshot's integrity before restoring (no server
#    contact needed; this is local file inspection).
kscore-cluster-backup verify --input /var/backups/kscore/<snapshot>.tar

# 3. Stop the running server (if it's running).
sudo systemctl stop kscore-server

# 4. Restore. Refuses to overwrite a populated cluster without --force.
sudo kscore-cluster-backup restore \
    --input /var/backups/kscore/<snapshot>.tar \
    --force

# 5. Restart and verify.
sudo systemctl start kscore-server
sleep 35
curl -fsS http://127.0.0.1:8080/health/ready | jq '.ready, .components'
```

The actual backup-creation runbook is
[`docs/runbooks/backup-restore.md`](backup-restore.md); without a
prior backup, scenario D is not available and the only path is
forward-fix.

## Verification checklist (after any scenario)

- [ ] `curl /health/ready` returns `ready=true` with every component
      reporting `status=ok`
- [ ] Agent fleet reconnects (use `kscore-events list --type
      agent_connected --since 5m` to confirm — the events stream is
      authoritative; the Prometheus metric is a lagging indicator)
- [ ] State apply works against a representative agent (try a
      previously-working YAML; should land idempotent with zero
      changes)
- [ ] Audit log accepts new writes (a `kscore-audit log --limit 5`
      after the verification commands above should show those very
      commands as new entries)
- [ ] Error-rate metrics return to the pre-incident baseline within
      ~10 minutes

## Post-procedure

### Immediate (within 1 hour)

1. [ ] Update the incident ticket / status page with rollback timing
       and the failing version + the version restored.
2. [ ] Notify stakeholders that rollback is complete.
3. [ ] Collect diagnostic snapshot for the post-mortem:

```bash
D=/tmp/diagnostics-$(date -u +%Y%m%dT%H%M%S)
mkdir -p "$D"
sudo journalctl -u kscore-server --since "2 hours ago" > "$D/server.log"
sudo journalctl -u kscore-agent --since "2 hours ago" > "$D/agent.log" 2>/dev/null || true
sudo cp /etc/kscore/server.yaml "$D/server.yaml.at-rollback"
sudo cp /var/lib/kscore/keystone.db "$D/keystone.db.at-rollback" 2>/dev/null || true
sudo dpkg -l kscore-server kscorectl kscore-agent > "$D/installed-packages.txt" 2>/dev/null || \
    sudo rpm -q kscore-server kscorectl kscore-agent > "$D/installed-packages.txt"
sudo tar czf "${D}.tar.gz" -C /tmp "$(basename $D)"
echo "Diagnostic bundle: ${D}.tar.gz"
```

### Within 24 hours

1. [ ] Create an incident report covering the change, the symptoms,
       the rollback scenario used, and the resulting metrics.
2. [ ] Schedule the post-mortem.
3. [ ] File a bug report against the failing version, attaching the
       diagnostic bundle.

### Before retrying the change

1. [ ] Root cause identified and fixed.
2. [ ] Fix validated on the same OS / kernel / dependency set as
       production (the v0.5 cross-distro CI matrix gates this).
3. [ ] Decision: same rollback scenario should still be available
       if the retry also fails.
4. [ ] Stakeholders notified of retry plan.

## Appendix: Useful Prometheus queries

Metric names follow the `kscore_<domain>_<event>_<unit>` convention
documented in `internal/metrics/metricdefs.go`; the exhaustive list
is in `deploy/grafana/expected_metrics.txt`. The most common
rollback-relevant queries:

```promql
# Per-domain error rate (5-minute window)
sum by (domain) (rate(kscore_api_errors_total[5m]))

# Request latency P99 (control-plane gRPC)
histogram_quantile(0.99,
    sum(rate(kscore_api_request_duration_seconds_bucket[5m]))
    by (le, method))

# Connected agents (real-time gauge)
kscore_agents_connected

# Audit-log write rate (proxy for "is the audit pipeline healthy")
rate(kscore_audit_writes_total[5m])
```

Forge an alert on `kscore_health_ready{component=...} == 0` for
fast-fail signaling.

## Escalation

If multiple rollback scenarios fail and the server still won't reach
`ready=true`, escalate per
[`docs/project/INCIDENT-RESPONSE.md`](../project/INCIDENT-RESPONSE.md).
Maintainer contact and reporting channels are documented in
[`OWNERSHIP.md`](../../OWNERSHIP.md) and
[`SECURITY.md`](../../SECURITY.md).
