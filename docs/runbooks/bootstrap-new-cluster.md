# Runbook: Bootstrap a New Keystone Core Deployment

## Overview

Install procedure for the v0.x line. Keystone Core v0.1 is
**single-node** — the server runs on one host with embedded NATS +
JetStream and a local SQLite store. The "bootstrap" path is the OS
package manager: install the three `.deb` (or `.rpm`) packages and
the postinstall scriptlets handle user creation, directory layout,
config rendering, and systemd integration.

> **v0.x scope note**: This runbook covers single-node first-install.
> Multi-node HA, region federation, Kubernetes operators, Helm charts,
> and the `kscore-bootstrap` SeedConfig FSM with real phase handlers
> are gate-v1.0 work tracked in
> [`docs/project/ROADMAP.md`](../project/ROADMAP.md). The
> `kscore-bootstrap` binary that ships in v0.1 is the FSM shell with
> logging no-op handlers — useful for wiring tests, not for production
> install.

For the narrative operator walkthrough (with `kscorectl` examples and
expected output) see
[`docs/project/GETTING-STARTED.md`](../project/GETTING-STARTED.md).

## Prerequisites

### Host

- One Linux host. v0.1 cross-distro CI exercises:
  - Ubuntu 22.04 LTS, Ubuntu 24.04 LTS
  - Debian 12
  - Rocky Linux 8, 9, 10
- systemd present (the postinst skips systemd integration in chroots
  / minimal containers but the runbook assumes a real host).
- Root or sudo access.
- Base utilities: `openssl`, `curl`, `tar`, `sed`, `install`. These
  are in the base install on every supported distro.

### Resources

Single-node v0.1 has modest needs — adequate for a trial install on
any laptop or small VM:

| Resource | Minimum | Recommended for sustained use |
|---|---|---|
| CPU | 1 core | 2+ cores |
| Memory | 1 GB | 2+ GB |
| Disk (`/var/lib/kscore`) | 5 GB | 20+ GB (JetStream grows) |

Production sizing (TLS-enabled identity provider, external Postgres,
large agent fleet) is gate-v1.0; this v0.1 runbook covers the trial
install.

### Network

The server binds to localhost by default. Adjust for your topology
before exposing it to remote agents.

| Port | Bind (default) | Component | Required |
|---|---|---|---|
| 5397 | 127.0.0.1 | kscore-server gRPC (control plane API) | Yes |
| 8080 | 127.0.0.1 | kscore-server HTTP (`/health/ready`, `/metrics`) | Yes |
| 4222 | 127.0.0.1 | Embedded NATS (agent transport) | Yes |
| 8081 | 127.0.0.1 | GitOps webhook receiver | Only if `gitops.webhook.enabled: true` |
| 6060 | 127.0.0.1 | pprof endpoint | Only if `profiling.enabled: true` |

Outbound: none required for a single-host trial. Production deployments
that ship metrics to Prometheus, traces to OTLP, or backups to S3 will
need outbound to those destinations.

For remote agents, the server's gRPC + NATS ports must be reachable
from each managed host. Re-bind `server.host` to `0.0.0.0` (or a
specific interface) and adjust `nats.embedded.host` accordingly.

## Trigger conditions

- New host needs Keystone Core installed.
- Test environment / sandbox setup.
- Disaster-recovery cluster provisioning (after the data-plane
  restore step in
  [`disaster-recovery.md`](disaster-recovery.md)).

## Procedure: trial install

This is the exact path Phase D's cross-distro VM matrix exercises.
All commands run as root or via `sudo`.

### Step 1 — Acquire the packages

Pre-v0.1.0 snapshots are operator-distributed (no public package
repository yet — a public repo arrives with the v1.0 release
ceremony). Copy the three packages for your distro family onto the
target host:

```bash
# Debian / Ubuntu
ls kscore-server_*_linux_amd64.deb \
   kscorectl_*_linux_amd64.deb \
   kscore-agent_*_linux_amd64.deb

# Rocky / RHEL
ls kscore-server-*.x86_64.rpm \
   kscorectl-*.x86_64.rpm \
   kscore-agent-*.x86_64.rpm
```

### Step 2 — Install

```bash
# Debian / Ubuntu
sudo apt install -y ./kscore-server_*_linux_amd64.deb \
                    ./kscorectl_*_linux_amd64.deb \
                    ./kscore-agent_*_linux_amd64.deb

# Rocky / RHEL
sudo dnf install -y ./kscore-server-*.x86_64.rpm \
                    ./kscorectl-*.x86_64.rpm \
                    ./kscore-agent-*.x86_64.rpm
```

The `kscore-server` postinst is idempotent and performs the following
on first install (see
[`deploy/packaging/kscore-server.postinst`](../../deploy/packaging/kscore-server.postinst)):

1. Creates the `kscore` system user + group (server runs unprivileged).
2. Creates `/etc/kscore` (`root:kscore` `0750`), `/var/lib/kscore`,
   `/var/log/kscore`, `/run/kscore` (each `kscore:kscore` `0750`).
3. Renders `/etc/kscore/server.yaml` from
   `/usr/share/kscore/server.yaml.template`, substituting
   `__HMAC_SECRET__` with a freshly generated `openssl rand -hex 32`
   value unique to this host.
4. `systemctl daemon-reload`, `enable`, and `start` (or `restart` on
   upgrade) of `kscore-server.service`.

The `kscore-agent` postinst (see
[`deploy/packaging/kscore-agent.postinst`](../../deploy/packaging/kscore-agent.postinst)):

1. Creates `/etc/kscore` and the agent's home + log directories
   (root-owned because the agent runs as root to manage host state).
2. Copies `/usr/share/kscore/agent.yaml.template` to
   `/etc/kscore/agent.yaml` (with placeholder `agent.id` and
   `nats.urls`).
3. `daemon-reload` + `enable` of `kscore-agent.service`.
4. **Deliberately does NOT start the agent** — operator edits to
   `agent.id` and `nats.urls` are required first.

### Step 3 — Verify the server is up

```bash
# Wait past the health.startupgraceperiod (30s default) before
# expecting a positive readiness signal.
sleep 35
curl -fsS http://127.0.0.1:8080/health/ready | jq '.ready, .components'

# Service status.
sudo systemctl status kscore-server --no-pager | head -10

# The recent journal will show the banner line:
#   "kscore-server dev (commit ...) gRPC: 127.0.0.1:5397 HTTP: 127.0.0.1:8080 ..."
sudo journalctl -u kscore-server -n 50 --no-pager | grep "kscore-server"

# CLI smoke (--version / --help should both work without server contact).
/usr/bin/kscore-server --version
kscorectl --version
kscorectl --help | head -10
```

If `/health/ready` returns `ready=false` past the grace period, or
the service is in `activating (auto-restart)`, jump to
[`troubleshooting.md`](troubleshooting.md#server-wont-start).

## What just landed on the filesystem

| Path | Purpose | Created by |
|---|---|---|
| `/usr/bin/kscore-server` | Server binary | server package |
| `/usr/bin/kscore-agent` | Agent binary | agent package |
| `/usr/bin/kscorectl` | Operator CLI | cli package |
| `/usr/bin/kscore-bootstrap` | Bootstrap CLI (FSM shell, v0.1) | server package |
| `/usr/bin/kscore-*` | Dispatched sub-binaries (audit, secrets, …) | cli package |
| `/usr/share/kscore/server.yaml.template` | Server config template | server package |
| `/usr/share/kscore/agent.yaml.template` | Agent config template | agent package |
| `/etc/kscore/server.yaml` | Rendered server config | server postinst |
| `/etc/kscore/agent.yaml` | Rendered agent config (placeholders) | agent postinst |
| `/var/lib/kscore/keystone.db` | SQLite state (created on first server start) | kscore-server |
| `/var/lib/kscore/jetstream/` | Embedded JetStream store | kscore-server |
| `/var/log/kscore/` | Log destination if file logging configured | server postinst |
| `/run/kscore/` | Runtime sockets | server postinst |
| `/etc/systemd/system/multi-user.target.wants/kscore-server.service` | systemd enable link | server postinst |

## Configuration walkthrough

The rendered `/etc/kscore/server.yaml` is shown below. This is the
trial-mode default — single-node, embedded NATS, SQLite, TLS off,
identity off — and it boots without further edits.

```yaml
mode: development

server:
  host: 127.0.0.1
  grpcport: 5397
  httpport: 8080
  tls:
    enabled: false

logging:
  level: info
  format: json

storage:
  driver: sqlite
  dsn: /var/lib/kscore/keystone.db

nats:
  mode: embedded
  jetstream:
    enabled: true
    storedir: /var/lib/kscore/jetstream

identity:
  enabled: false

security:
  hmacsecret: "<32-byte hex generated on first install>"

metrics:
  enabled: true
  path: /metrics

health:
  startupgraceperiod: 30s
  checktimeout: 2s
```

The koanf tags are flat lowercase — `grpcport`, `httpport`,
`storedir`, `hmacsecret`, `startupgraceperiod`, `heartbeatinterval`.
There are no snake_case (`grpc_port`, `store_dir`) or kebab-case keys
anywhere in v0.1; the canonical schema lives in
[`internal/config/`](../../internal/config/) and is auto-rendered into
[`CONFIGURATION-REFERENCE.md`](../project/CONFIGURATION-REFERENCE.md).

### When to edit `/etc/kscore/server.yaml`

| Reason | Section to touch | See |
|---|---|---|
| Expose to remote agents | `server.host`, `nats.embedded.host` | [`CONFIGURATION-REFERENCE.md`](../project/CONFIGURATION-REFERENCE.md) |
| Enable TLS | `server.tls.{enabled,certfile,keyfile}` | hardening checklist below |
| Switch to Postgres | `storage.{driver,dsn}` | hardening checklist below |
| Switch to external NATS | `nats.{mode,urls}` | hardening checklist below |
| Enable identity provider | `identity.enabled: true` + `trustdomain` | [`CONFIGURATION-REFERENCE.md`](../project/CONFIGURATION-REFERENCE.md) |
| Enable agent bootstrap PSKs | `nats.bootstrap.{enabled,psks}` | "Bootstrap an agent" below |
| Promote to production mode | `mode: production` | every WARN in [`SECURITY-GOVERNANCE.md`](../project/SECURITY-GOVERNANCE.md) must be addressed |

After any edit:

```bash
sudo systemctl restart kscore-server
sleep 35
curl -fsS http://127.0.0.1:8080/health/ready | jq '.ready, .components'
```

The rendered config is **not** a packaging conffile — upgrades will
not overwrite operator edits (see
[`upgrade-cluster.md`](upgrade-cluster.md)).

## Bootstrap an agent

The agent package leaves `agent.yaml` with placeholder values and the
unit disabled-from-start by design. To bring up the agent on the same
host as the server (the simplest trial topology):

### Step 1 — Edit `/etc/kscore/agent.yaml`

```bash
sudo $EDITOR /etc/kscore/agent.yaml
```

At minimum, set:

```yaml
agent:
  id: "trial-host-01"          # operator-chosen, any non-empty string

nats:
  urls:
    - "nats://127.0.0.1:4222"   # embedded NATS on the same host
```

The template's other defaults (`heartbeatinterval: 30s`,
`metadatainterval: 60s`) are fine for a trial.

### Step 2 — Decide on agent registration

v0.1 supports two registration shapes:

- **PSK-based bootstrap** (server-side `nats.bootstrap.psks` lists
  per-agent hex secrets; agent presents `agent.bootstrappsk` at
  boot). The PSK record is in-memory on the server — a server
  restart wipes consumption, so PSKs are intended as
  rotation-friendly one-shots.
- **Pre-provisioned agent rows** (operator inserts the agent
  identity out-of-band before the agent starts). This is the
  trial-install shortcut; no server-side edits needed.

For the trial, leave both `nats.bootstrap.enabled: false` on the
server and `agent.bootstrappsk` unset on the agent. The control plane
will accept heartbeats from the agent.id straight off; production
deployments must turn on either PSK-based bootstrap or
identity-issued join tokens (gate-v1.0 — see
[`ROADMAP.md`](../project/ROADMAP.md)).

### Step 3 — Start and verify

```bash
sudo systemctl start kscore-agent
sudo systemctl status kscore-agent --no-pager | head -10

# Server-side: confirm the agent registered.
kscore-events list --type agent_connected --since 5m

# Optional: tail the agent journal to see reconnect / heartbeat logs.
sudo journalctl -u kscore-agent -f
```

If the agent never appears in the events list, jump to
[`troubleshooting.md`](troubleshooting.md#agent-issues).

## Post-install verification checklist

- [ ] `curl http://127.0.0.1:8080/health/ready` returns `ready=true`
      with every component reporting `status=ok`
- [ ] `kscore-server --version` reports the expected version
- [ ] `kscorectl --version` reports the expected version
- [ ] `/var/lib/kscore/keystone.db` exists and is owned by
      `kscore:kscore`
- [ ] `/var/lib/kscore/jetstream/` exists and is populated
      (the embedded server creates the commands + events streams on
      first start)
- [ ] At least one entry in
      `kscore-audit log --limit 5` (every CLI invocation against
      the server writes an audit record — proves the audit pipeline
      is healthy end-to-end)
- [ ] If you started the agent: it appears in
      `kscore-events list --type agent_connected --since 5m`
- [ ] A representative state apply works:

```bash
kscorectl state apply ./examples/hello.yaml --agent trial-host-01
kscorectl state drift --agent trial-host-01   # should report zero drift
```

## Cross-references

- Operator walkthrough (narrative, with expected output):
  [`docs/project/GETTING-STARTED.md`](../project/GETTING-STARTED.md)
- Full canonical config schema:
  [`docs/project/CONFIGURATION-REFERENCE.md`](../project/CONFIGURATION-REFERENCE.md)
- Full CLI reference (`kscorectl`, sub-binaries, exit codes):
  [`docs/project/CLI-REFERENCE.md`](../project/CLI-REFERENCE.md)
- When the install doesn't work:
  [`troubleshooting.md`](troubleshooting.md)
- When a deployed change broke things:
  [`emergency-rollback.md`](emergency-rollback.md)
- Routine upgrades:
  [`upgrade-cluster.md`](upgrade-cluster.md)
- Backups and snapshot creation:
  [`backup-restore.md`](backup-restore.md)
- DR-cluster rebuild from a snapshot:
  [`disaster-recovery.md`](disaster-recovery.md)
- Production-mode warning catalog:
  [`docs/project/SECURITY-GOVERNANCE.md`](../project/SECURITY-GOVERNANCE.md)

## Appendix: production hardening checklist

The trial install is intentionally insecure-by-default so it boots
out-of-box on any host. **Before exposing the server to a real agent
fleet**, work through the following. Each item maps to a config
section or external dependency; none of them are automatic.

### TLS

- [ ] Provision a server cert + key (operator-managed PKI, ACME, or
      the identity provider — see next item).
- [ ] Set `server.tls.enabled: true` and either:
  - `server.tls.certfile` + `server.tls.keyfile` (file-sourced PKI), or
  - leave both empty + set `identity.enabled: true` (identity-provider
      sources the server cert and rotates it on each CA roll).
- [ ] Re-bind `server.host` from `127.0.0.1` to the interface that
      remote agents reach.
- [ ] Confirm `mode: production` triggers the "TLS is disabled" prod
      warning if you forget — see
      [`SECURITY-GOVERNANCE.md`](../project/SECURITY-GOVERNANCE.md).

### Identity provider

- [ ] Set `identity.enabled: true`.
- [ ] Pick a trust domain (`identity.trustdomain`) — usually a DNS
      label you own.
- [ ] Allocate persistent storage at `identity.storagepath` (defaults
      to `./data/identity`; relocate to `/var/lib/kscore/identity`).
- [ ] First-start initializes the signing CA. Back up
      `identity.storagepath` immediately after — losing it means
      re-bootstrapping every agent.

### HMAC secret rotation

- [ ] The first-install HMAC was generated by `openssl rand -hex 32`.
      Decide on a rotation cadence (the value lives in
      `security.hmacsecret`).
- [ ] Each rotation invalidates every agent's stored bootstrap PSK;
      plan a coordinated re-bootstrap (or graduate to identity-issued
      join tokens via `identity.enabled: true`).
- [ ] Storing the secret inline in `server.yaml` is a v0.1 footgun
      `ProductionWarnings()` calls out — rotate via an out-of-band
      secret manager. KMS-backed rotation is post-v1.0.

### External NATS

- [ ] If you need cross-host messaging without exposing the embedded
      NATS, switch to external mode:

```yaml
nats:
  mode: external
  urls:
    - "nats://nats-1.internal:4222"
    - "nats://nats-2.internal:4222"
    - "nats://nats-3.internal:4222"
```

- [ ] Validate the external broker has JetStream enabled (the
      commands + events streams require it).
- [ ] Configure NATS-level auth (`token` / `credential`) per your
      external broker's auth model.

### External Postgres

- [ ] Provision a Postgres instance (v0.1 targets PG 14+).
- [ ] Create the database + user; grant DDL privileges (kscore-server
      runs schema migrations on first start).
- [ ] Switch the kscore config:

```yaml
storage:
  driver: postgres
  dsn: "host=pg.internal port=5432 dbname=kscore user=kscore password=… sslmode=require"
```

- [ ] If migrating from SQLite, follow the migration playbook in
      [`disaster-recovery.md`](disaster-recovery.md) (export-and-
      import; in-place migration is gate-v1.0).

### Observability

- [ ] Confirm `/metrics` is reachable from your Prometheus scraper
      (default path `/metrics` on `http://<host>:8080`).
- [ ] Import the agent-fleet dashboard from
      [`deploy/grafana/dashboards/agent-fleet.json`](../../deploy/grafana/dashboards/agent-fleet.json)
      and the rest of the Grafana bundle.
- [ ] Enable OpenTelemetry tracing if you have an OTLP collector:

```yaml
tracing:
  enabled: true
  exporter: otlp
  sampler: probabilistic
  samplerate: 0.1
```

- [ ] Forward `journalctl -u kscore-server -u kscore-agent` to your
      central log store.

### Backups

- [ ] Schedule `kscore-cluster-backup backup` runs (cron / systemd
      timer / your scheduler of choice). See
      [`backup-restore.md`](backup-restore.md) for the create
      cadence and destination guidance.
- [ ] Pick a backup destination outside the kscore host (S3, NFS,
      object store with versioning).
- [ ] Test the restore path end-to-end at least once before going
      live. An un-tested backup is a not-yet-failed backup.

### Security baseline

- [ ] Run a vulnerability scanner against the packaged binaries
      (`syft` for SBOM, `grype` / `trivy` for CVEs). The release
      pipeline produces SBOMs alongside the packages — verify they
      match what's installed.
- [ ] Set `mode: production` to enable the warning catalog. Address
      every WARN that the server prints on startup before going
      live.
- [ ] Set `security.defaultpolicy: deny` (it's the default but worth
      confirming).
- [ ] Lock down the gRPC + HTTP listeners at the network layer
      (firewall / VPC security group) — `server.host: 0.0.0.0` plus
      open ingress is the v0.1 footgun the trial install
      deliberately avoids.

## Appendix: what `kscore-bootstrap` is in v0.1

The `kscore-bootstrap` binary that ships in v0.1 is the FSM shell for
the gate-v1.0 bootstrap orchestrator. It reads a `SeedConfig` YAML
and drives the state machine through phases (detect, configure,
validate, install, blueprints, verify) — but the phase handlers are
logging no-ops that return nil. The binary is useful today for:

- Exercising the FSM wiring in tests.
- Validating a SeedConfig YAML parses.
- Watching the phase transitions log in dev.

It is **not** the supported install path in v0.1. The supported path
is `apt install` / `dnf install` per Step 2 above. Real phase
handlers (host detect, binary install, systemd unit gen, blueprint
engine invocation, `/health` verify) are tracked under the gate-v1.0
ROADMAP entry "Bootstrap phase handlers + durable checkpointer". The
binary lives at `/usr/bin/kscore-bootstrap` after install; running it
without `--seed <path>` exits non-zero with a usage banner.

## Escalation

If the install fails on a supported distro and
[`troubleshooting.md`](troubleshooting.md) doesn't get you unblocked,
file an issue per
[`docs/project/INCIDENT-RESPONSE.md`](../project/INCIDENT-RESPONSE.md).
Attach the diagnostic bundle described in the
[`troubleshooting.md` collection appendix](troubleshooting.md#collecting-diagnostics-for-a-bug-report).
