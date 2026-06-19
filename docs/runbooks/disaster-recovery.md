# Runbook: Disaster Recovery

## Overview

Restore procedures for the v0.x line when the Keystone Core deployment
is down and needs to be rebuilt from a prior snapshot or from scratch.
Companion to [`backup-restore.md`](backup-restore.md) (which owns
backup *creation*) and [`emergency-rollback.md`](emergency-rollback.md)
(which owns rollback of a live-but-misbehaving deployment).

This runbook is specifically about: "the cluster is down, restore it."

> **v0.x scope note**: Keystone Core v0.1 is **single-node**. Multi-
> region failover, cross-cluster replication, and split-brain recovery
> are gate-v1.0 features tracked in
> [`docs/project/ROADMAP.md`](../project/ROADMAP.md). The v0.1 DR
> primitives are:
>
> - `kscore-cluster-backup list <dir>` — list snapshots (local file
>   inspection; no server contact required)
> - `kscore-cluster-backup verify --input <snapshot>` — verify a
>   snapshot's integrity (local file inspection; no server contact
>   required)
> - `kscore-cluster-backup backup` / `restore` — the snapshot
>   backup/restore subcommands talk to the server's
>   `ClusterGRPCServer`. That gRPC service is **not yet wired at boot**
>   in v0.1 (tracked under gate-v1.0); the runnable v0.1 backup
>   path today is `kscore-backup` (see `cmd/kscore-backup`, documented
>   in [`backup-restore.md`](backup-restore.md)).
>
> The procedures below therefore treat snapshots as **tar archives of
> `/var/lib/kscore/` plus `/etc/kscore/`** that were produced by the
> `kscore-backup` tool, by `kscore-cluster-backup backup` (where the
> service is wired), or by an out-of-band filesystem snapshot. Postgres
> dumps, where applicable, come from the operator's existing `pg_dump`
> / `pg_basebackup` tooling — kscore-core does not own Postgres
> backup orchestration.

## RTO and RPO expectations on the v0.x line

| Scenario                                  | RTO target | RPO target           | Notes |
|-------------------------------------------|------------|----------------------|-------|
| SQLite corruption, snapshot on hand       | 15-30 min  | last snapshot        | filesystem restore + restart |
| Postgres corruption, recent dump on hand  | 30-60 min  | last `pg_dump` / WAL | Postgres-side restore from operator tooling |
| JetStream store corruption                | 15-30 min  | last snapshot        | loses in-flight events between backup + restore |
| Identity / CA data loss                   | 15-30 min  | last snapshot        | identity is opt-in; default config has it off |
| Config destruction (no operator backup)   | 30-60 min  | last operator change | regenerate from template, rotates HMAC, re-issue agent PSKs |
| Total host loss, snapshot on hand         | 1-3 hours  | last snapshot        | OS reinstall + package install + filesystem restore |
| Total host loss, no snapshot              | not recoverable | n/a              | rebuild from scratch per `bootstrap-new-cluster.md`; agent state is lost |

These are **modest** targets relative to the v1.0 HA story. v0.1 is a
single-node deployment; an RTO of "minutes to a few hours" is the
shape you should plan around, with RPO bounded by your snapshot
cadence. The fastest path to a smaller RPO on v0.1 is a more frequent
backup schedule, not a runtime feature.

## Pre-DR checklist

Before starting, confirm:

- [ ] You have a usable snapshot. Verify it without trusting the
      filename:

```bash
kscore-cluster-backup list /var/backups/kscore/
kscore-cluster-backup verify --input /var/backups/kscore/<snapshot>.tar
```

- [ ] You know which storage driver the failed deployment used
      (SQLite vs Postgres). Check the snapshot's
      `/etc/kscore/server.yaml` for `storage.driver`.
- [ ] If Postgres: you have credentials and a recent dump or PITR
      window from your Postgres tooling.
- [ ] You have root on the recovery host.
- [ ] The recovery host has the same major-version
      `kscore-server` / `kscorectl` packages installed as the
      snapshot was taken on. Cross-version restore is not
      supported on the v0.x line.
- [ ] Stakeholders have been notified per
      [`docs/project/INCIDENT-RESPONSE.md`](../project/INCIDENT-RESPONSE.md).

---

## Scenario 1 — SQLite corruption (default storage backend)

**Symptoms**:

- `kscore-server` fails to start; `journalctl -u kscore-server`
  shows the silent-exit pattern (see `troubleshooting.md` §
  "Server won't start").
- Manual run surfaces `database disk image is malformed`,
  `file is not a database`, or `PRAGMA integrity_check` returns
  errors:

```bash
sudo -u kscore sqlite3 /var/lib/kscore/keystone.db "PRAGMA integrity_check"
```

- Audit-log writes fail. State queries return errors.

**Impact**: server unavailable. Agents disconnect after their
heartbeat-timeout window.

**Procedure**:

```bash
# 1. Stop the server.
sudo systemctl stop kscore-server

# 2. Preserve the corrupt DB for forensics before overwriting it.
sudo cp -a /var/lib/kscore/keystone.db \
    /var/lib/kscore/keystone.db.corrupt-$(date -u +%Y%m%dT%H%M%S)

# 3. (Optional best-effort) attempt SQLite's native .recover before
#    restoring from snapshot. If this produces a usable DB, RPO is
#    "near zero" instead of "last snapshot."
sudo -u kscore sqlite3 /var/lib/kscore/keystone.db ".recover" \
    | sudo -u kscore sqlite3 /var/lib/kscore/keystone-recovered.db
# Inspect /var/lib/kscore/keystone-recovered.db for completeness.
# If usable, swap it in:
#   sudo -u kscore mv /var/lib/kscore/keystone-recovered.db \
#                     /var/lib/kscore/keystone.db
#   sudo systemctl start kscore-server
#   curl -fsS http://127.0.0.1:8080/health/ready | jq

# 4. If .recover failed or produced an incomplete DB, restore from
#    snapshot. The snapshot tar contains /var/lib/kscore/ in full --
#    extract just the keystone.db file (and its sidecars) to bound
#    the change set.
sudo tar -xf /var/backups/kscore/<snapshot>.tar \
    -C / \
    var/lib/kscore/keystone.db

# Remove stale WAL / SHM that would otherwise confuse the new DB
# file.
sudo rm -f /var/lib/kscore/keystone.db-wal \
           /var/lib/kscore/keystone.db-shm

sudo chown kscore:kscore /var/lib/kscore/keystone.db
sudo chmod 0640          /var/lib/kscore/keystone.db

# 5. Restart and verify.
sudo systemctl start kscore-server
sleep 35
curl -fsS http://127.0.0.1:8080/health/ready | jq '.ready, .components'
```

**Verification**:

- `/health/ready` returns `ready=true` with every component
  `status=ok`.
- `kscore-audit log --limit 5` accepts the new request and the
  call itself appears in the returned tail.
- A `kscorectl state check ./examples/hello.yaml --agent <agent-id>`
  against a known-good YAML succeeds.

**Data loss**: everything written to the DB between the snapshot
time and the failure is lost. Cross-reference the audit log on
the recovered DB against the snapshot's audit log to identify
which state-applies / exec-runs landed in the gap and need to be
manually re-run.

---

## Scenario 2 — Postgres backend corruption / unavailable

**Symptoms**:

- `kscore-server` startup logs surface
  `failed to connect to postgres` / `pq:` errors, or the server
  starts but every state query fails.
- The kscore-side health check fails:

```bash
curl -fsS http://127.0.0.1:8080/health/ready | jq '.components'
```

with the `storage` component reporting `status=fail`.

**Impact**: server unavailable. Postgres being shared with other
applications can broaden the impact.

**Procedure**:

```bash
# 1. Confirm the failure is Postgres-side, not kscore-side.
PGPASSWORD=<pwd> psql -h <pg-host> -U kscore kscore \
    -c "SELECT count(*) FROM pg_stat_activity WHERE datname='kscore'"

# 2. Stop kscore-server so it doesn't keep retrying.
sudo systemctl stop kscore-server

# 3. Restore Postgres using YOUR Postgres tooling. kscore-core does
#    not ship Postgres restore primitives. Typical paths:
#
#    a. From a logical dump:
#       PGPASSWORD=<pwd> psql -h <pg-host> -U postgres -c \
#           "DROP DATABASE kscore; CREATE DATABASE kscore OWNER kscore;"
#       PGPASSWORD=<pwd> pg_restore -h <pg-host> -U kscore -d kscore \
#           /backup/postgres/kscore-<timestamp>.dump
#
#    b. From a base backup + WAL (point-in-time recovery): run the
#       Postgres-side recovery flow your operator documentation
#       covers; kscore-core is unaffected by which mechanism.

# 4. Verify the database is queryable.
PGPASSWORD=<pwd> psql -h <pg-host> -U kscore kscore \
    -c "SELECT count(*) FROM kscore_audit_events"

# 5. Restart kscore-server.
sudo systemctl start kscore-server
sleep 35
curl -fsS http://127.0.0.1:8080/health/ready | jq '.ready, .components'
```

**Verification**: same as Scenario 1 — `/health/ready`, audit log
accepting writes, a state-check round-trip.

**Data loss**: bounded by your Postgres backup cadence and the
Postgres-side WAL retention. Coordinate Postgres PITR with the
kscore-side audit log to identify the gap.

---

## Scenario 3 — JetStream store corruption

**Symptoms**:

- `kscore-server` starts but the events stream and audit-log write
  rate drop to zero.
- `journalctl -u kscore-server` shows JetStream-side errors:
  `jetstream stream replay failed`, `consumer not found`,
  `stream file corrupt`.
- `kscore-events list --since 5m` returns no entries even though
  agents are actively heartbeating.

**Impact**: in-flight events between the corruption point and the
restore are lost. State persistence (SQLite / Postgres) is
unaffected — the agent fleet and state history survive.

**Procedure**:

```bash
# 1. Stop the server.
sudo systemctl stop kscore-server

# 2. Preserve the corrupt JetStream directory for forensics.
sudo mv /var/lib/kscore/jetstream \
        /var/lib/kscore/jetstream.corrupt-$(date -u +%Y%m%dT%H%M%S)

# 3. Restore the JetStream subtree from snapshot.
sudo tar -xf /var/backups/kscore/<snapshot>.tar \
    -C / \
    var/lib/kscore/jetstream

sudo chown -R kscore:kscore /var/lib/kscore/jetstream

# 4. Restart.
sudo systemctl start kscore-server
sleep 35

# 5. Confirm JetStream is replaying / accepting writes.
sudo journalctl -u kscore-server -n 100 --no-pager \
    | grep -i jetstream
curl -fsS http://127.0.0.1:8080/health/ready | jq '.components'
kscore-events list --since 1m --limit 5
```

**Verification**:

- `/health/ready` returns `ready=true` with the
  `nats`/`jetstream` component `status=ok`.
- `kscore-audit log --limit 5` accepts new writes — the audit
  pipeline depends on JetStream, so this is the load-bearing
  signal.
- New `kscore-events list` queries return entries within ~10s
  of post-restart wall-clock.

**Data loss**: any event published to JetStream between the
snapshot and the corruption is lost. Outbound effects already
delivered to agents are unaffected; in-flight `kscorectl exec
run` results that hadn't been ACK'd may need to be re-run.

---

## Scenario 4 — Identity / CA data loss

**Symptoms**:

- Identity is opt-in. The default `/etc/kscore/server.yaml.template`
  ships `identity.enabled: false`; **if you haven't enabled
  identity, skip this scenario**.
- With identity enabled (operator-provided CA cert/key or
  identity-provider mode): the server fails to start with TLS
  bootstrap errors, or agents fail mTLS handshakes against the
  control-plane gRPC endpoint.
- `journalctl -u kscore-server` shows `failed to load identity
  material` / `certfile not found` / `failed to derive identity`.

**Impact**: depends on enabled mode. Server may refuse to start
(CertFile/KeyFile mode) or start but reject mTLS clients
(identity-provider mode).

**Procedure**:

```bash
# 1. Stop the server.
sudo systemctl stop kscore-server

# 2. Restore the identity subtree from snapshot. The on-disk layout
#    is under /var/lib/kscore/identity/ (CA material, issued
#    cert/key pairs, identity-provider state).
sudo tar -xf /var/backups/kscore/<snapshot>.tar \
    -C / \
    var/lib/kscore/identity

sudo chown -R kscore:kscore /var/lib/kscore/identity
sudo chmod -R go-rwx        /var/lib/kscore/identity

# 3. If your server.yaml points identity.certfile / identity.keyfile
#    at paths OUTSIDE /var/lib/kscore/ (e.g. /etc/kscore/tls/),
#    restore those paths separately from the snapshot:
sudo tar -xf /var/backups/kscore/<snapshot>.tar -C / \
    etc/kscore/tls/ 2>/dev/null || true

# 4. Restart.
sudo systemctl start kscore-server
sleep 35
curl -fsS http://127.0.0.1:8080/health/ready | jq '.ready, .components'
```

**Verification**:

- `/health/ready` returns `ready=true` with the `identity`
  component `status=ok`.
- A known-good agent reconnects per its mTLS handshake — confirm
  via `kscore-events list --type agent_connected --since 5m`.

**Data loss**: any cert / key material issued after the snapshot
is lost. Agents holding post-snapshot identities need to be
re-issued via the identity bootstrap flow. The certificate-rotation
runbook covers re-issuance.

---

## Scenario 5 — Config destruction (regenerate from template)

**Symptoms**:

- `/etc/kscore/server.yaml` has been deleted, truncated, or
  rendered unparseable, and **no operator-side backup** of the
  prior config exists.
- `kscore-server` startup fails before it reaches the banner line.

**Impact**: server unavailable. Regenerating from the shipped
template **rotates the HMAC secret**, which invalidates every
agent's stored bootstrap PSK; every agent must be re-bootstrapped.

**Procedure**:

This scenario overlaps with Scenario C in
[`emergency-rollback.md`](emergency-rollback.md). Use Scenario C
as the authoritative procedure; the steps below are reproduced for
DR convenience.

```bash
# 1. Stop the server (it isn't running anyway, but make state
#    deterministic).
sudo systemctl stop kscore-server

# 2. Regenerate /etc/kscore/server.yaml from the shipped template.
sudo install -o root -g kscore -m 0640 \
    /usr/share/kscore/server.yaml.template /etc/kscore/server.yaml

HMAC=$(openssl rand -hex 32)
sudo sed -i "s|__HMAC_SECRET__|${HMAC}|g" /etc/kscore/server.yaml

# 3. If you remember your operator edits (storage.driver, listen
#    addresses, identity.enabled, etc.), apply them now BEFORE
#    starting. Otherwise you'll boot with template defaults --
#    SQLite at /var/lib/kscore/keystone.db, gRPC :5397, HTTP :8080,
#    embedded NATS :4222, profiling off, identity off.
sudo $EDITOR /etc/kscore/server.yaml

# 4. Restart.
sudo systemctl start kscore-server
sleep 35
curl -fsS http://127.0.0.1:8080/health/ready | jq '.ready, .components'

# 5. Re-bootstrap each agent. The HMAC rotation invalidated every
#    agent's stored bootstrap PSK. Follow the agent-bootstrap flow
#    in bootstrap-new-cluster.md for each affected host.
```

**Verification**:

- `/health/ready` returns `ready=true`.
- Re-bootstrapped agents appear in
  `kscore-events list --type agent_connected --since 30m`.
- State applies and audit-log writes work end-to-end.

**Data loss**: no in-database data loss (SQLite / Postgres / JetStream
all survive). Operator-side config customisations are lost unless
restored from a sibling backup or version-controlled config.

---

## Scenario 6 — Total host loss (rebuild + restore)

**Symptoms**: the host is gone (hardware failure, accidental VM
deletion, ransomware) and a replacement host is being provisioned
or has already been provisioned. SSH no longer reaches the prior
host.

**Impact**: full outage until rebuild completes. This is the
worst-case v0.1 DR scenario.

**Procedure**:

```bash
# 1. Provision the replacement host with the same OS major version
#    as the failed host (cross-distro DR is not supported on the
#    v0.x line). Network reachability for agents must match the
#    original host (same IP, or a DNS record they'll re-resolve).

# 2. Install the kscore packages. Use the SAME version as the
#    snapshot was taken on -- the snapshot tar will not survive a
#    cross-version restore.
sudo apt install -y \
    ./kscore-server_<version>_linux_amd64.deb \
    ./kscorectl_<version>_linux_amd64.deb   # Debian/Ubuntu
# or
sudo dnf install -y \
    ./kscore-server-<version>.x86_64.rpm \
    ./kscorectl-<version>.x86_64.rpm        # Rocky/RHEL

# 3. Stop the freshly-postinst-started service so the restore
#    doesn't race against a running server.
sudo systemctl stop kscore-server

# 4. Restore state + config from snapshot.
sudo tar -xf /tmp/<snapshot>.tar \
    -C / \
    var/lib/kscore \
    etc/kscore

# Fix ownership (the tar may have been created on a host where
# the kscore UID/GID differs from the replacement host's).
sudo chown -R kscore:kscore /var/lib/kscore
sudo chown -R root:kscore   /etc/kscore
sudo chmod 0640             /etc/kscore/server.yaml

# 5. If using Postgres, point at the restored / live Postgres
#    instance. Verify the connection independently before starting
#    the server:
#       PGPASSWORD=<pwd> psql -h <pg-host> -U kscore kscore -c '\dt'

# 6. Start the server.
sudo systemctl start kscore-server
sleep 35
curl -fsS http://127.0.0.1:8080/health/ready | jq '.ready, .components'

# 7. Update DNS if the replacement host has a different IP.
#    Agents resolve the control plane via their /etc/kscore/agent.yaml
#    nats.urls; if you used hostnames there, DNS update is enough.
#    If you used raw IPs, edit each agent.yaml.
```

**Verification**:

- `/health/ready` returns `ready=true` with every component
  `status=ok`.
- Every agent that survived the host loss reconnects within ~5
  minutes:

```bash
kscore-events list --type agent_connected --since 30m
```

- A spot-check state apply succeeds end-to-end:

```bash
kscorectl state apply ./examples/hello.yaml --agent <agent-id>
kscorectl state drift --agent <agent-id>
```

- Audit log accepts new writes:

```bash
kscore-audit log --limit 5
```

**Data loss**: bounded by snapshot freshness (state + config) and
Postgres backup cadence (if applicable). Agent-side runtime state
(in-flight exec runs, queued state-applies) between the snapshot
and the host loss is gone.

**No snapshot available?** This procedure cannot complete. Treat
the deployment as net-new and follow
[`bootstrap-new-cluster.md`](bootstrap-new-cluster.md). You will
lose the entire agent fleet's stored identities, audit history,
and state-run history.

---

## RTO/RPO drill procedure

Snapshots that aren't periodically restored are not known to work.
Run this drill **quarterly** at minimum, ideally monthly:

```bash
# 1. Spin up an isolated drill host (VM / container) on the same OS
#    + kscore-server version as production.

# 2. Copy the most recent production snapshot to the drill host.
scp prod:/var/backups/kscore/<latest>.tar drill:/tmp/

# 3. On the drill host, walk Scenario 6 (total host rebuild) end-to-end.
ssh drill
sudo systemctl stop kscore-server
sudo tar -xf /tmp/<latest>.tar -C / var/lib/kscore etc/kscore
sudo chown -R kscore:kscore /var/lib/kscore
sudo chown -R root:kscore   /etc/kscore
sudo systemctl start kscore-server
sleep 35

# 4. Run the standard DR verification checklist below.

# 5. Record wall-clock duration -- this is your measured RTO.
#    Compare against the production snapshot timestamp -- this is
#    your measured RPO if the prior incident happened at drill time.

# 6. Tear down the drill host.
```

Drill results should be filed in your incident-response log. A
snapshot you haven't restored within 90 days is a snapshot you
shouldn't assume works.

---

## Cross-references

- **Backup creation**: [`backup-restore.md`](backup-restore.md).
  This file does not document `kscore-backup` / `kscore-cluster-backup
  create` ergonomics; that runbook does.
- **Live-but-bad deployment** (server up, recently changed, behaving
  badly): [`emergency-rollback.md`](emergency-rollback.md). Use that
  runbook instead of this one when the server is running but the
  *change* needs to be rolled back. The four scenarios there are
  package downgrade (A), state rollback (B), config revert (C), and
  catastrophic restore-from-snapshot (D — overlaps with Scenarios 1
  and 6 here).
- **Net-new install** (no prior deployment, no snapshot to restore):
  [`bootstrap-new-cluster.md`](bootstrap-new-cluster.md).
- **Server-side troubleshooting** (silent exit, port conflict,
  storage / NATS issues): [`troubleshooting.md`](troubleshooting.md).
- **Certificate / identity rotation**:
  [`certificate-rotation.md`](certificate-rotation.md).
- **Incident response process** (notifications, post-mortem flow):
  [`docs/project/INCIDENT-RESPONSE.md`](../project/INCIDENT-RESPONSE.md).

---

## Post-DR checklist

After completing any DR scenario:

- [ ] `curl /health/ready` returns `ready=true` with every component
      reporting `status=ok`.
- [ ] All agents that should have reconnected are visible via
      `kscore-events list --type agent_connected --since 30m`.
- [ ] Audit log accepts new writes (`kscore-audit log --limit 5`
      shows recent entries including the verification calls
      themselves).
- [ ] A representative state apply / drift round-trip succeeds:

```bash
kscorectl state apply ./examples/hello.yaml --agent <agent-id>
kscorectl state drift --agent <agent-id>
```

- [ ] Prometheus error-rate metrics return to a sane baseline within
      ~10 minutes of restart.
- [ ] If HMAC was rotated (Scenario 5), every agent has been
      re-bootstrapped.
- [ ] If identity material was restored (Scenario 4), any agents
      issued certs after the snapshot have been re-issued.
- [ ] The corrupt/old data preserved at the start of each scenario
      (`*.corrupt-<timestamp>`) is either copied off to forensic
      storage or deliberately discarded.

## Post-DR diagnostic bundle

Collect this bundle for the post-mortem regardless of which scenario
ran:

```bash
D=/tmp/kscore-dr-$(date -u +%Y%m%dT%H%M%S)
mkdir -p "$D"

# Service journals around the incident window.
sudo journalctl -u kscore-server --since "6 hours ago" --no-pager > "$D/server.log"
sudo journalctl -u kscore-agent  --since "6 hours ago" --no-pager > "$D/agent.log" 2>/dev/null || true

# Config (redacted) at recovery time.
sudo cp /etc/kscore/server.yaml "$D/server.yaml.at-recovery"
sudo sed -i 's/^\(\s*hmacsecret:\s*"\).*"/\1REDACTED"/' "$D/server.yaml.at-recovery"
sudo sed -i 's/^\(\s*master_key:\s*\).*$/\1REDACTED/'    "$D/server.yaml.at-recovery"

# Snapshot manifest (which snapshot was used).
echo "snapshot restored: <snapshot>.tar" > "$D/restore-manifest.txt"
echo "wall-clock start: <start-time-UTC>" >> "$D/restore-manifest.txt"
echo "wall-clock end:   <end-time-UTC>"   >> "$D/restore-manifest.txt"
echo "scenario:         <1-6>"            >> "$D/restore-manifest.txt"

# Versions.
sudo dpkg -l kscore-server kscorectl kscore-agent > "$D/installed-packages.txt" 2>/dev/null || \
    sudo rpm -q kscore-server kscorectl kscore-agent > "$D/installed-packages.txt"
/usr/bin/kscore-server --version >> "$D/installed-packages.txt"

# Health snapshot.
curl -fsS http://127.0.0.1:8080/health/ready > "$D/health-ready.json" 2>/dev/null || true

# Audit log tail (last 200 entries -- captures restore-time activity).
kscore-audit log --limit 200 -o json > "$D/audit-tail.json" 2>/dev/null || true

# Storage sizes (sanity-check the restore actually populated the data plane).
sudo du -sh /var/lib/kscore /var/lib/kscore/* 2>/dev/null > "$D/storage-sizes.txt"

# Bundle.
sudo tar czf "${D}.tar.gz" -C /tmp "$(basename $D)"
echo "DR diagnostic bundle: ${D}.tar.gz"
```

Attach the bundle to the incident ticket. **Inspect before sharing
externally** — the redaction is best-effort and may miss
operator-added secrets in the YAML.

## Escalation

If a chosen scenario fails to bring the server back to
`ready=true`, escalate per
[`docs/project/INCIDENT-RESPONSE.md`](../project/INCIDENT-RESPONSE.md).
Maintainer contact channels are documented in
[`OWNERSHIP.md`](../../OWNERSHIP.md) and
[`SECURITY.md`](../../SECURITY.md).
