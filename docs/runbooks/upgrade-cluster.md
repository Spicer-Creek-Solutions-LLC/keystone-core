# Runbook: Upgrade a Keystone Core deployment

## Overview

Upgrade procedures for the v0.x line. Keystone Core v0.1 is **single-
node** — the server runs on one host and is upgraded as a single
unit using the OS package manager. Rolling / canary / blue-green
upgrade strategies and the orchestrated `kscorectl upgrade` command
family arrive with the HA work tracked in
[`docs/project/ROADMAP.md`](../project/ROADMAP.md).

> **v0.x scope note**: This runbook covers the v0.1 single-node
> upgrade flow. Multi-node orchestration, canary deploys, and
> automatic agent fleet upgrades are gate-v1.0 features tracked in
> the roadmap.

## Prerequisites

- [ ] Target version validated in a non-production environment
- [ ] Backup created within the last 24 hours
      (see [`backup-restore.md`](backup-restore.md))
- [ ] Release notes reviewed
      (see [`CHANGELOG.md`](../../CHANGELOG.md))
- [ ] Maintenance window scheduled
- [ ] Rollback plan reviewed
      (see [`emergency-rollback.md`](emergency-rollback.md))

## When to upgrade

- A new version ships with a feature you need
- A security patch is required (see
  [`SECURITY.md`](../../SECURITY.md) for disclosure timelines)
- A bug fix in your dependency path
- A breaking change in the v0.x line is documented in
  [`CHANGELOG.md`](../../CHANGELOG.md) requiring proactive migration

## Pre-upgrade

```bash
# 1. Confirm current versions.
sudo systemctl status kscore-server --no-pager | head -5
dpkg -l kscore-server kscorectl kscore-agent 2>/dev/null || \
    rpm -q kscore-server kscorectl kscore-agent

# 2. Confirm cluster (server) is healthy before the change.
curl -fsS http://127.0.0.1:8080/health/ready | jq '.ready, .components'

# 3. Confirm agents are connected (the events stream is the
#    authoritative signal; Prometheus is a lagging indicator).
kscore-events list --type agent_connected --since 1h

# 4. Sample recent error baseline so post-upgrade comparison is
#    meaningful. Replace <PROM_URL> with your scrape target.
curl -s "http://<PROM_URL>/api/v1/query?query=sum(rate(kscore_api_errors_total[5m]))" \
    > /tmp/upgrade-baseline-errors.json

# 5. Take a snapshot. Skip only if you have a recent backup AND a
#    documented restore drill in the last 30 days.
sudo kscore-cluster-backup backup \
    --output /var/backups/kscore/pre-upgrade-$(date -u +%Y%m%dT%H%M%S).tar \
    --description "pre-upgrade snapshot"

# 6. Read the release notes for breaking changes. Honor any
#    documented config migration before stopping the service.
$EDITOR CHANGELOG.md  # or open in your forge UI
```

## Server upgrade

```bash
# 1. Stop the server. The systemd unit drains in-flight gRPC
#    streams cleanly within TimeoutStopSec=30s.
sudo systemctl stop kscore-server

# 2. Upgrade the package via the OS package manager. apt /
#    dnf will pull the new version from whichever channel is
#    configured (the v0.x snapshot artifacts ship via
#    operator-managed repos in v0.1; a public package repo
#    arrives with the v1.0 release ceremony).
sudo apt install -y ./kscore-server_<version>_linux_amd64.deb   # Debian/Ubuntu
sudo dnf install -y ./kscore-server-<version>.x86_64.rpm        # Rocky/RHEL

# 3. The postinst re-runs daemon-reload + enable + (re)start. If
#    your config has operator edits the postinst preserves them
#    (the rendered /etc/kscore/server.yaml is NOT a conffile;
#    upgrades never overwrite it).
sudo systemctl status kscore-server --no-pager | head -10

# 4. Wait for the 30s grace period then verify health.
sleep 35
curl -fsS http://127.0.0.1:8080/health/ready | jq '.ready, .components'

# 5. Verify the running version matches expectations.
/usr/bin/kscore-server --version
```

## CLI upgrade

The `kscorectl` package bundles the `kscorectl` CLI and the companion
`kscore-*` tools (`kscore-audit`, `kscore-secrets`, …). Upgrade it in
lockstep with the server.

```bash
sudo apt install -y ./kscorectl_<version>_linux_amd64.deb   # Debian/Ubuntu
sudo dnf install -y ./kscorectl-<version>.x86_64.rpm        # Rocky/RHEL

# Sanity-check the new CLI.
kscorectl --version
kscorectl --help | head -10
```

## Agent upgrade

Agents are upgraded per-host. The server keeps running while
individual agents are bounced; agents handle reconnect via the
control plane's bootstrap chain.

For each managed host:

```bash
# 1. Stop the agent locally on the host (or via a remote command
#    if the agent connection is healthy).
sudo systemctl stop kscore-agent

# 2. Upgrade the package.
sudo apt install -y ./kscore-agent_<version>_linux_amd64.deb   # Debian/Ubuntu
sudo dnf install -y ./kscore-agent-<version>.x86_64.rpm        # Rocky/RHEL

# 3. Restart. The agent re-registers with the control plane on
#    the next NATS reconnect (60s default, see
#    nats.reconnectWait in /etc/kscore/agent.yaml).
sudo systemctl start kscore-agent

# 4. Verify the agent reconnected by watching the control-plane
#    events stream from the operator workstation.
kscore-events list --type agent_connected --since 5m \
    --filter "metadata.agent_id == '<host-id>'"
```

If you have many hosts, automate this with the configuration-
management tool you already use to push packages
(`ansible`, `chef`, `puppet`, `salt`, etc.); v0.1 does not ship a
built-in agent-fleet upgrade orchestrator.

## Verification

After the server + CLI + (relevant) agent upgrades land:

- [ ] `/health/ready` returns `ready=true` with every component
      reporting `status=ok`
- [ ] `kscore-server --version` and `kscorectl --version` both
      report the target version
- [ ] Every upgraded agent reappears in
      `kscore-events list --type agent_connected --since 30m`
- [ ] A representative state apply still works:

```bash
kscorectl state apply ./examples/hello.yaml --agent <agent-id>
kscorectl state drift --agent <agent-id>
```

- [ ] Error-rate metrics return to (or improve on) the pre-upgrade
      baseline within ~10 minutes — compare against the
      `/tmp/upgrade-baseline-errors.json` captured pre-upgrade
- [ ] Audit log accepts new writes:

```bash
kscore-audit log --limit 5
```

- [ ] No new alerts firing in your Prometheus / observability stack

## If something goes wrong

Follow [`emergency-rollback.md`](emergency-rollback.md). The
relevant scenario depends on what changed:

- Server package broke things → Scenario A (package downgrade)
- A state apply done during the upgrade window left bad state →
  Scenario B (`kscorectl state rollback`)
- Operator config edit during the window caused the regression →
  Scenario C (config revert from backup)
- Server can't start at all → Scenario D (restore from the
  pre-upgrade snapshot you took in step 5 above)

## Post-procedure

### Immediate

1. [ ] Update the change-management ticket with the actual versions
       deployed and the wall-clock duration.
2. [ ] Notify stakeholders the upgrade completed.

### Within 24 hours

1. [ ] Review post-upgrade metrics for any sustained regression.
2. [ ] Address any new warnings in the audit log.
3. [ ] Update local documentation if any operator step changed.
4. [ ] Clean up old version artifacts you no longer need
       (old `.deb` / `.rpm` files in `/tmp/` etc.).

### If a rollback was needed

1. [ ] Collect diagnostics (see `emergency-rollback.md` post-procedure).
2. [ ] File a bug report against the failing version.
3. [ ] Schedule the post-mortem.
4. [ ] Plan the retry after the root cause is fixed.

## Appendix: version-compatibility expectations on the v0.x line

The v0.x line **allows breaking changes between minor versions**.
The `Breaking changes` section in
[`CHANGELOG.md`](../../CHANGELOG.md) is the authoritative source
for each release. Read it before upgrading.

The v1.0 SemVer stability commitment (no breaking changes in
patches, breaking changes only at major-version boundaries)
begins at v1.0.0, not v0.1.0. See
[`docs/project/VERSIONING.md`](../project/VERSIONING.md).

## Appendix: troubleshooting an upgrade

### `systemctl start kscore-server` fails with exit 1

The cli.go RootCommand silences cobra errors by default
(`SilenceErrors: true`); a fatal-init failure surfaces only as the
routine `"stopped"` log line + `status=1/FAILURE` in journalctl.
Common causes:

- **Port conflict**: another process holds gRPC `:5397` (the
  default) or HTTP `:8080`. Diagnose with `ss -tlnp | grep -E
  ':5397|:8080'`.
- **Invalid config**: an operator-edited
  `/etc/kscore/server.yaml` no longer parses against the new
  binary's config schema. Diagnose by running
  `/usr/bin/kscore-server --config /etc/kscore/server.yaml`
  manually outside systemd and reading stderr.
- **Storage migration error**: a schema migration in the new
  binary failed against the existing SQLite or Postgres state.
  Diagnose by running the binary manually and reading the
  migration error.

A v0.x ROADMAP entry ("kscore-server: surface fatal-init errors
to stderr instead of silently exiting") tracks the long-term fix
for the silent-exit pattern.

### Agent stays disconnected after upgrade

- Check `journalctl -u kscore-agent -n 100` for the agent's last
  reconnect-attempt log line.
- Confirm the agent.yaml `nats.urls` still points at the server's
  NATS port.
- Confirm the agent identity wasn't invalidated. If the server-side
  HMAC secret was regenerated during the upgrade (see Scenario C
  in `emergency-rollback.md`), every agent's bootstrap PSK needs
  to be re-issued.

### Server logs warn `secrets: enable production controls`

Pre-existing dev-mode bootstrap UX surface. The
[`SECURITY-GOVERNANCE.md`](../project/SECURITY-GOVERNANCE.md)
"Production warnings" section lists every prod-mode WARN; none
of them are upgrade-introduced.
