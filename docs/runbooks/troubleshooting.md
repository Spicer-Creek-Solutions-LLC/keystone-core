# Runbook: Troubleshooting Guide

## Overview

Common Keystone Core problems and how to diagnose them in v0.1. Each
section follows a `Symptoms → Diagnosis → Resolution` shape; commands
are shown for the v0.1 single-node default install.

> **v0.x scope note**: This runbook covers the v0.1 single-node
> deployment. Multi-node cluster topology, leader-election failures,
> split-brain detection, and orchestrated cluster diagnostics are
> tracked under the gate-v1.0 HA work in
> [`docs/project/ROADMAP.md`](../project/ROADMAP.md).

## Quick diagnostics

```bash
# Server reachable?
curl -fsS http://127.0.0.1:8080/health/ready | jq

# Service status?
sudo systemctl status kscore-server --no-pager | head -15
sudo systemctl status kscore-agent  --no-pager | head -15

# Recent server log lines (the cli.go RootCommand silences cobra
# errors via SilenceErrors: true, so the most useful signal on a
# crashloop is the journal -- not stderr).
sudo journalctl -u kscore-server -n 200 --no-pager

# Audit log accepting writes? (proxy for "is the pipeline healthy")
kscorectl audit log --limit 5
```

## Issue categories

1. [Server won't start (silent exit / 217 USER)](#server-wont-start)
2. [Agent issues](#agent-issues)
3. [NATS issues](#nats-issues)
4. [Database (SQLite / Postgres) issues](#database-issues)
5. [State application issues](#state-application-issues)
6. [Performance issues](#performance-issues)
7. [Collecting diagnostics for a bug report](#collecting-diagnostics-for-a-bug-report)

---

## Server won't start

### Symptoms

- `sudo systemctl start kscore-server` succeeds but `is-active`
  reports `activating (auto-restart)` indefinitely
- `journalctl -u kscore-server` shows the routine `"starting"` log
  but no banner line (no `"kscore-server dev (commit ...) gRPC:
  127.0.0.1:5397 HTTP: 127.0.0.1:8080 ..."`)
- Then a `"stopped"` log + systemd reports
  `Main process exited, code=exited, status=1/FAILURE`
- No stderr error message anywhere

This is the silent-exit pattern. The cli.go RootCommand uses
cobra's `SilenceErrors: true` so the wrapped error from
`run()` is swallowed. A v0.x ROADMAP entry tracks the long-term
fix; in the meantime, diagnose by reading the journal closely and
running the binary outside systemd to surface the real error.

### Diagnosis

```bash
# 1. Try running the server manually outside systemd to see
#    stderr. Run as the kscore user so file permissions match
#    what systemd would use.
sudo -u kscore /usr/bin/kscore-server --config /etc/kscore/server.yaml

# 2. Check if a port the server needs is already bound.
ss -tlnp | grep -E ':(5397|8080|4222|8081) '

# 3. Check if the SQLite database file is locked or unreadable.
sudo -u kscore sqlite3 /var/lib/kscore/keystone.db "PRAGMA integrity_check"

# 4. Check the user / dirs the postinst created.
id kscore
ls -la /etc/kscore /var/lib/kscore /var/log/kscore /run/kscore
```

### Resolution by failure cause

**Port conflict on gRPC 5397** (most common after a Rocky 10 install
where Cockpit owns 9090 on older defaults, or any host with a prior
service holding 5397):

```bash
# Identify the holder.
ss -tlnp | grep :5397

# Either: stop the conflicting service, or change the kscore-server
# port in /etc/kscore/server.yaml:
sudo $EDITOR /etc/kscore/server.yaml  # set server.grpcport to a free value
sudo systemctl restart kscore-server
```

**Invalid config** (operator-edited `/etc/kscore/server.yaml` no
longer parses):

```bash
# Run manually to see the validation error.
sudo -u kscore /usr/bin/kscore-server --config /etc/kscore/server.yaml 2>&1 \
    | tail -5

# If unrecoverable, regenerate from the shipped template (rotates
# HMAC secret -- invalidates every agent's bootstrap PSK).
sudo install -o root -g kscore -m 0640 \
    /usr/share/kscore/server.yaml.template /etc/kscore/server.yaml
HMAC=$(openssl rand -hex 32)
sudo sed -i "s|__HMAC_SECRET__|${HMAC}|g" /etc/kscore/server.yaml
sudo systemctl restart kscore-server
```

**SQLite locked** (a previous server process holds the WAL):

```bash
# Check for open handles.
sudo lsof /var/lib/kscore/keystone.db
# Kill stray holders, then retry start.
```

**`status=217/USER`** (the kscore user doesn't exist — postinst
didn't run):

```bash
# Re-run the postinst manually.
sudo dpkg-reconfigure kscore-server   # Debian/Ubuntu
sudo rpm --restore kscore-server      # RPM (best-effort)
# If that doesn't work, reinstall the package.
```

---

## Agent issues

### Agent not connecting

**Symptoms**: `kscore-agent` service is running but the control
plane never sees the agent — no entry in
`kscorectl events list --type agent_connected`.

**Diagnosis**:

```bash
# On the agent host:
sudo systemctl status kscore-agent --no-pager
sudo journalctl -u kscore-agent -n 200 --no-pager

# Confirm the configured nats.urls value.
sudo grep -A1 'nats:' /etc/kscore/agent.yaml

# Test NATS reachability from the agent host.
nc -zv <server-host> 4222     # NATS client port
# (If your deployment uses external NATS in `nats.mode: external`,
# the URL + port come from `nats.urls` in /etc/kscore/agent.yaml.
# Default v0.1 single-node setup runs embedded NATS on the same
# host as the server, bound to 127.0.0.1:4222.)
```

**Resolution**:

- **`nats.urls` unset or empty**: the v0.1 default `agent.yaml.template`
  ships placeholder values. Set the URL and an agent.id. The
  packaged `kscore-agent.postinst` deliberately does not start the
  service for this reason.

  ```bash
  sudo $EDITOR /etc/kscore/agent.yaml
  # set agent.id and confirm nats.urls
  sudo systemctl start kscore-agent
  ```

- **HMAC secret mismatch**: if the server's HMAC was rotated (see
  the server-rebuild path above), every agent's stored secret is
  now invalid. Re-issue per the bootstrap chain.

- **Network ACL blocking NATS**: confirm port 4222 reachability
  with `nc` / `mtr`. If using `nats.mode: external`, also check
  the external broker's logs.

### Agent heartbeat appears stale

**Symptoms**: agent shows in event stream but heartbeats lag —
`metadata.last_heartbeat_at` falls behind wall-clock by minutes.

**Diagnosis**:

```bash
# Server-side: how fresh are heartbeats?
kscorectl events list --type agent_heartbeat --since 5m

# Agent-side: heartbeat-publish errors in the journal?
sudo journalctl -u kscore-agent | grep -i heartbeat | tail -10
```

**Resolution**:

- If the host is under sustained high CPU, the agent may not be
  scheduling its heartbeat goroutine on time. Lower the system
  load or raise `agent.heartbeatinterval` in `/etc/kscore/agent.yaml`
  (default 30s).
- If NATS reconnect storms are happening, raise
  `nats.reconnectWait` (default 2s); a noisy network with brief
  drops can otherwise keep the agent reconnecting faster than
  it heartbeats.

---

## NATS issues

The v0.1 default is **embedded NATS** — the server runs an
in-process JetStream-enabled NATS that listens on 127.0.0.1:4222.
External-NATS deployments configure `nats.mode: external` + an
external broker.

### Symptoms: "NATS connection refused" / events not flowing

**Diagnosis**:

```bash
# Is anything listening on 4222?
ss -tlnp | grep :4222

# Server log for the embedded-NATS bootstrap line.
sudo journalctl -u kscore-server | grep "nats embedded server started" | tail -5

# JetStream working? The audit log + events stream both depend on
# JetStream; if they accept writes, JetStream is healthy.
kscorectl audit log --limit 1
kscorectl events list --since 5m --limit 3
```

**Resolution (embedded mode)**:

- If `:4222` is bound by something else (rare), change the embedded
  port in `/etc/kscore/server.yaml` under
  `nats.embedded.port` and restart.
- If the embedded JetStream store directory is unwritable, fix
  ownership of `/var/lib/kscore/jetstream`.

**Resolution (external mode)**:

- Confirm the external broker is healthy with `nats-server --help`
  or via the broker's own monitoring endpoint (default `:8222` on
  many deployments).
- Confirm credentials / TLS material match what the kscore-server
  config expects.

### JetStream store fills the disk

**Diagnosis**:

```bash
# How big is the JetStream store?
sudo du -sh /var/lib/kscore/jetstream/
df -h /var/lib/kscore/

# What streams are biggest?
sudo du -sh /var/lib/kscore/jetstream/*/
```

**Resolution**:

- Per-event-type retention is set in
  `internal/events.applyRetention` and is operator-configurable.
  Reduce `events.retention.<type>.max_age` or `max_bytes` and
  restart the server.
- The `nats.jetstream.storedir` value can be moved to a larger
  partition. Stop the server before moving the directory.

---

## Database issues

The v0.1 default storage is **SQLite at `/var/lib/kscore/keystone.db`**.
Postgres is supported (`storage.driver: postgres`, with a libpq
DSN) but requires the operator to run their own Postgres instance.

### Symptoms: state queries fail / "database is locked"

**Diagnosis**:

```bash
# SQLite: integrity check.
sudo -u kscore sqlite3 /var/lib/kscore/keystone.db "PRAGMA integrity_check"
sudo -u kscore sqlite3 /var/lib/kscore/keystone.db "PRAGMA quick_check"

# Who has the DB open?
sudo lsof /var/lib/kscore/keystone.db

# Postgres: connection count.
PGPASSWORD=<pwd> psql -h <host> -U kscore kscore \
    -c "SELECT count(*) FROM pg_stat_activity WHERE datname='kscore'"
```

**Resolution**:

- **SQLite locked**: a stray process holds the WAL. Kill the holder,
  then restart kscore-server. If no holder is visible, the WAL may
  have been left in a partial-checkpoint state — stop the server,
  delete the `.db-shm` and `.db-wal` sidecar files (the WAL is
  recoverable from the journal), and restart.

- **Postgres connection limit**: raise `max_connections` on the
  Postgres side, or reduce `storage.poolsize` in the kscore-server
  config.

### Symptoms: SQLite database corruption

**Resolution**: restore from a recent backup. The recovery flow is
documented in Scenario D of
[`emergency-rollback.md`](emergency-rollback.md) (catastrophic
recovery via `kscore-cluster-backup restore`). The SQLite native
`.recover` flow is a best-effort fallback:

```bash
sudo systemctl stop kscore-server
sudo -u kscore sqlite3 /var/lib/kscore/keystone.db ".recover" \
    | sudo -u kscore sqlite3 /var/lib/kscore/keystone-recovered.db
# Inspect /var/lib/kscore/keystone-recovered.db for completeness
# before swapping it in.
```

---

## State application issues

### Symptoms: `kscorectl state apply` returns errors

**Diagnosis**:

```bash
# Try compile-only first (no server contact) -- isolates parse /
# render / validate errors from runtime-apply errors.
kscorectl state compile ./my-state.yaml

# If compile passes, run check (server-side validation without
# Apply phase) to isolate apply-time errors.
kscorectl state check ./my-state.yaml --agent <agent-id>

# Inspect a prior run's outcome.
kscorectl state history --limit 10
kscorectl state show <run-id>
```

**Resolution**:

- **Compile errors**: fix the YAML; the error message names the
  failing module + key.
- **Module-not-found**: confirm the named module is in the stdlib
  (35 modules ship; see `internal/statemgmt/stdlib/`). External
  modules require the operator to install + sign their bundle.
- **Apply-time errors**: per-declaration outcome is in
  `kscorectl state show <run-id>`; the declResult includes the
  agent-side error verbatim.

### Symptoms: state drift detected

**Diagnosis**:

```bash
# Show drifted declarations.
kscorectl state drift --agent <agent-id>
```

**Resolution**:

- Re-apply the YAML to remediate, or use `--fix` to do both in one
  step:

  ```bash
  kscorectl state drift --agent <agent-id> --fix ./my-state.yaml
  ```

- If drift recurs on the same declarations, an out-of-band process
  is mutating the host. Find it (file-monitoring, audit-log
  correlation), then either bring that process into the state
  YAML or exclude the path from kscore management.

---

## Performance issues

### High CPU on kscore-server

**Diagnosis**:

```bash
# Process-level.
top -bn1 -p $(pgrep -d, kscore-server)

# Enable the pprof endpoint by setting profiling.enabled: true in
# /etc/kscore/server.yaml. Defaults to off because pprof leaks
# heap state. Listens on 127.0.0.1:6060 (localhost-only by
# default for the same reason).
sudo $EDITOR /etc/kscore/server.yaml   # set profiling.enabled: true
sudo systemctl restart kscore-server

# Sample CPU.
curl -fsS "http://127.0.0.1:6060/debug/pprof/profile?seconds=30" > /tmp/cpu.pprof
go tool pprof /tmp/cpu.pprof
```

**Resolution**: investigate the profile. Common hotspots:

- Embedded NATS JetStream snapshot/compaction loops — raise
  `nats.jetstream.maxstorage` or move to external NATS.
- High audit-log write rate — sample the audit query API
  (`kscorectl audit log --limit 100`) to see what's noisy.
- gRPC reflection isn't enabled in dev mode by default; if you
  turned it on, the introspection load is observable.

### High memory on kscore-server

**Diagnosis**:

```bash
# RSS / VSZ.
ps -o pid,user,%mem,rss,vsz,comm -p $(pgrep -d, kscore-server)

# Heap profile (requires profiling.enabled: true).
curl -fsS http://127.0.0.1:6060/debug/pprof/heap > /tmp/heap.pprof
go tool pprof /tmp/heap.pprof
```

**Resolution**: typical leak shapes:

- Long-lived gRPC streams not being cleaned up — confirm streaming
  consumers (`kscorectl events subscribe`, etc.) close their context
  on exit.
- A misconfigured JetStream consumer pulling without ack'ing.

### Slow API responses

**Diagnosis**:

```bash
# Latency to /health/ready (should be < 5ms once past grace).
curl -w '%{time_total}\n' -o /dev/null -s http://127.0.0.1:8080/health/ready

# gRPC latency via metrics (replace <PROM_URL>).
curl -s "http://<PROM_URL>/api/v1/query?query=histogram_quantile(0.99,sum(rate(kscore_api_request_duration_seconds_bucket[5m]))%20by%20(le,method))"

# Database query times (SQLite — slow log not native; correlate
# with audit-log timing).
kscorectl audit log --limit 50 -o json | jq -r '.[]|[.timestamp,.elapsed_ms]|@tsv'
```

**Resolution**:

- **SQLite slow**: run `VACUUM` periodically; the SQLite WAL
  growth can slow lookups on large stores. Schedule via cron or
  consider migrating to Postgres for HA / scale.
- **Network latency between server and external NATS**: confirm
  the broker is on the same LAN segment / region.

---

## Collecting diagnostics for a bug report

When opening a bug report, attach:

```bash
D=/tmp/kscore-diag-$(date -u +%Y%m%dT%H%M%S)
mkdir -p "$D"

# Service journals (last 24h).
sudo journalctl -u kscore-server --since "24 hours ago" --no-pager > "$D/server.log"
sudo journalctl -u kscore-agent  --since "24 hours ago" --no-pager > "$D/agent.log" 2>/dev/null || true

# Config (redacted).
sudo cp /etc/kscore/server.yaml "$D/server.yaml.redacted"
sudo sed -i 's/^\(\s*hmacsecret:\s*"\).*"/\1REDACTED"/' "$D/server.yaml.redacted"
sudo sed -i 's/^\(\s*master_key:\s*\).*$/\1REDACTED/'    "$D/server.yaml.redacted"
sudo cp /etc/kscore/agent.yaml "$D/agent.yaml.redacted" 2>/dev/null || true

# Versions.
sudo dpkg -l kscore-server kscore-cli kscore-agent > "$D/installed-packages.txt" 2>/dev/null || \
    sudo rpm -q kscore-server kscore-cli kscore-agent > "$D/installed-packages.txt"
/usr/bin/kscore-server --version >> "$D/installed-packages.txt"

# System info.
uname -a > "$D/system-info.txt"
free -h >> "$D/system-info.txt"
df -h  >> "$D/system-info.txt"
ss -tlnp >> "$D/system-info.txt"

# Health snapshot.
curl -fsS http://127.0.0.1:8080/health/ready > "$D/health-ready.json" 2>/dev/null || true

# Audit log tail (last 100 entries).
kscorectl audit log --limit 100 -o json > "$D/audit-tail.json" 2>/dev/null || true

# Bundle.
sudo tar czf "${D}.tar.gz" -C /tmp "$(basename $D)"
echo "Diagnostic bundle: ${D}.tar.gz"
```

Attach `${D}.tar.gz` to the bug report. **Inspect the bundle before
sharing** — the redaction is best-effort and may miss
operator-added secrets in the YAML.

## Getting help

- **Documentation**: [`docs/project/`](../project/) — design docs,
  policy, security, runbooks. Auto-generated CLI / configuration /
  API references are at
  [`CLI-REFERENCE.md`](../project/CLI-REFERENCE.md) /
  [`CONFIGURATION-REFERENCE.md`](../project/CONFIGURATION-REFERENCE.md) /
  [`API-REFERENCE.md`](../project/API-REFERENCE.md).
- **Issues**: [`codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/issues`](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/issues)
  (primary). GitHub mirror exists for discoverability but issues
  are tracked on Codeberg.
- **Security reports**: see [`SECURITY.md`](../../SECURITY.md) for
  the disclosure flow (do NOT open a public issue for security
  bugs).
- **Project ownership + maintainer contact**: see
  [`OWNERSHIP.md`](../../OWNERSHIP.md).
- **Incident response procedures** (operator-side): see
  [`docs/project/INCIDENT-RESPONSE.md`](../project/INCIDENT-RESPONSE.md).
