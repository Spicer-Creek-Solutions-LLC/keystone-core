# Getting started

A guided ~30-minute walkthrough from a fresh Ubuntu or Rocky VM to a
running Keystone Core single-node deployment with one applied state.
Uses the operator install path — `apt install` / `dnf install` of the
v0.1.x packages — and drives the system through `kscorectl`, the
operator CLI.

For the dense reference walkthrough (full package layout, postinst
behavior, filesystem map, port table, verification matrix), see
[`../runbooks/bootstrap-new-cluster.md`](../runbooks/bootstrap-new-cluster.md).
This guide is the friendlier first-time tour; the runbook is what you
reach for when you have to do this without thinking about it.

> **v0.1.x scope note.** Until the v1.0 release ceremony, `.deb` /
> `.rpm` packages are operator-distributed rather than published to a
> public repo. Get the snapshot you've been sent onto the target host
> before you start.
>
> Single-node trial: this guide co-locates the server and a single
> agent on the same VM, with the agent talking to the embedded NATS
> inside the server. Real multi-host topologies put agents on the
> hosts they manage — covered in
> [`../runbooks/bootstrap-new-cluster.md`](../runbooks/bootstrap-new-cluster.md).

## Prerequisites

- Fresh Ubuntu 22.04+ / 24.04, Debian 12, or Rocky 8 / 9 / 10 VM
  (full validated matrix in
  [`E2E-VM-TESTING.md`](E2E-VM-TESTING.md)).
- Root or sudo access on the VM.
- `curl` and `jq` for the smoke checks:
  - Debian/Ubuntu: `sudo apt install -y curl jq`
  - Rocky/RHEL: `sudo dnf install -y curl jq`
- The Keystone Core package snapshot for your distro family, copied
  onto the host (three packages: `kscore-server`, `kscore-cli`,
  `kscore-agent`).

## 1. Install

Pick the section for your distro family.

### Debian / Ubuntu

```bash
sudo apt install -y ./kscore-server_*_linux_amd64.deb \
                    ./kscore-cli_*_linux_amd64.deb \
                    ./kscore-agent_*_linux_amd64.deb
```

### Rocky / RHEL

```bash
sudo dnf install -y ./kscore-server-*.x86_64.rpm \
                    ./kscore-cli-*.x86_64.rpm \
                    ./kscore-agent-*.x86_64.rpm
```

What the `kscore-server` postinst does on a first install:

1. Creates the `kscore` system user + group (server runs unprivileged).
2. Lays down `/etc/kscore/` (`root:kscore` `0750`), `/var/lib/kscore/`,
   `/var/log/kscore/`, `/run/kscore/` (each `kscore:kscore` `0750`).
3. Renders `/etc/kscore/server.yaml` with a freshly generated HMAC
   secret unique to this host.
4. `daemon-reload`, `enable`, and `start` of `kscore-server.service`.

The `kscore-agent` postinst is **deliberately quieter**: it lays down
files and enables the unit but does **not** start the agent. You'll
edit `/etc/kscore/agent.yaml` and start it manually in step 3.

## 2. Verify the server is up

```bash
sleep 35    # past the 30s startup grace period
curl -fsS http://127.0.0.1:8080/health/ready | jq '.ready, .components'
```

You should see `true` and a `components` object with every dependency
in `READY`. If you see `ready=false` past 35 s, the server is still
warming up or there's a config issue — jump to
[`../runbooks/troubleshooting.md`](../runbooks/troubleshooting.md).

Sanity-check the CLI works (these don't contact the server):

```bash
/usr/bin/kscore-server --version
kscorectl --version
kscorectl --help | head -10
```

## 3. Bring the agent online

Edit `/etc/kscore/agent.yaml`. Two values matter for a single-node
trial:

- `agent.id` — any operator-meaningful name. Using the host's short
  hostname is a reasonable default:

  ```bash
  sudo sed -i "s|^  id:.*|  id: $(hostname -s)|" /etc/kscore/agent.yaml
  ```

- `nats.urls` — for the single-node trial, point at the embedded
  NATS server inside `kscore-server` on this same host:

  ```bash
  sudo sed -i 's|^  urls:.*|  urls: ["nats://127.0.0.1:4222"]|' /etc/kscore/agent.yaml
  ```

(Open the file with your editor of choice if you'd rather see the
full context.)

Then start the agent and verify the unit is running:

```bash
sudo systemctl start kscore-agent
sudo systemctl status kscore-agent --no-pager | head -10
```

Confirm the agent registered with the server:

```bash
curl -fsS http://127.0.0.1:8080/api/v1/agents | jq
```

> **v0.1.x scope note.** A `kscorectl agent list` subcommand is on
> the v0.x backlog (tracked in [`ROADMAP.md`](ROADMAP.md)); until it
> lands, querying registered agents goes through the REST endpoint
> above or through `kscore-server`'s journal:
>
> ```bash
> sudo journalctl -u kscore-server -n 50 --no-pager | grep -i agent
> ```

## 4. Run a command

Run a no-op command on the freshly registered agent — print its FQDN:

```bash
kscorectl exec --agent "$(hostname -s)" -- hostname -f
```

You should see the agent's FQDN echoed back, with a clean exit. If the
command hangs, the agent isn't reachable — check step 3.

## 5. Apply state

Write a small declaration:

```bash
cat >/tmp/hello.yaml <<'YAML'
metadata:
  name: getting-started
  version: "0.1"
file:
  /tmp/keystone-hello:
    state: present
    content: "hello from keystone-core\n"
    mode: "0644"
YAML
```

Apply it via `kscorectl`:

```bash
kscorectl state apply /tmp/hello.yaml --agent "$(hostname -s)"
```

Verify the file landed on the host:

```bash
cat /tmp/keystone-hello
# hello from keystone-core
```

## 6. Browse the audit log

Every command and every state run flows through the audit store.
Query recent entries:

```bash
kscorectl audit log --limit 10
```

You'll see your `exec` from step 4 and the `state apply` from step 5
recorded, with timestamps and actor IDs. The audit stream is the
canonical answer to "what just happened on this control plane?"

## 7. Tear down (optional)

To remove Keystone Core entirely:

```bash
# Debian / Ubuntu
sudo apt purge -y kscore-server kscore-agent kscore-cli

# Rocky / RHEL
sudo dnf remove -y kscore-server kscore-agent kscore-cli
```

`purge` (apt) and `remove` (dnf) leave `/etc/kscore/` and
`/var/lib/kscore/` in place by default — useful if you want to
reinstall and pick up where you left off. Remove them manually for
a fully clean state:

```bash
sudo rm -rf /etc/kscore /var/lib/kscore /var/log/kscore
```

## Next steps

- **The CLI reference**: [`CLI-REFERENCE.md`](CLI-REFERENCE.md) lists
  every `kscore-*` binary and every subcommand. Each operator binary
  is also reachable as `kscorectl <name>` via plugin dispatch.
- **The configuration reference**:
  [`CONFIGURATION-REFERENCE.md`](CONFIGURATION-REFERENCE.md) catalogs
  every key in `server.yaml` and `agent.yaml`.
- **The API reference**: [`API-REFERENCE.md`](API-REFERENCE.md) indexes
  every gRPC RPC + REST endpoint with links to the canonical proto /
  openapi sources.
- **Architecture**: [`DESIGN.md`](DESIGN.md) for the high-level design;
  [`../../PROJECT-DETAILS.md`](../../PROJECT-DETAILS.md) for per-domain
  implementation detail.
- **Operations**: [`../runbooks/`](../runbooks/) covers
  bootstrap-new-cluster, disaster-recovery, security-incident,
  troubleshooting, upgrade-cluster, and more.
- **Security**: [`SECURITY-GOVERNANCE.md`](SECURITY-GOVERNANCE.md)
  covers the v1.0 four-scan baseline + the vulnerability disclosure
  process; [`../../SECURITY.md`](../../SECURITY.md) is the reporting
  entry point.
- **Developing on the source?** See [`DEVELOPMENT.md`](DEVELOPMENT.md)
  § Local Dev Topology — the contributor docker-compose harness for
  ad-hoc iteration on server / agent code.

## Troubleshooting

### Server health stays `ready=false`

Past the 35-second grace period, this means either the server hasn't
finished initializing all of its components (NATS, store, identity, …)
or one of them failed. Check the journal:

```bash
sudo journalctl -u kscore-server -n 100 --no-pager
```

Common causes: a syntax error in `server.yaml`, a port conflict on
`5397` or `8080`, or filesystem permissions on `/var/lib/kscore`. The
runbook [`../runbooks/troubleshooting.md`](../runbooks/troubleshooting.md)
walks the full diagnostic tree.

### Agent doesn't register

```bash
sudo journalctl -u kscore-agent -n 50 --no-pager
```

The most common cause is `nats.urls` in `/etc/kscore/agent.yaml` not
pointing at a reachable NATS endpoint. For a single-node trial it
should be exactly `["nats://127.0.0.1:4222"]` — the embedded NATS
inside `kscore-server` on this same host.

### Port already in use

The defaults are `8080` (HTTP), `5397` (gRPC), `4222` (embedded NATS).
If any collide on your host — Cockpit historically used `9090`, which
is why the server default moved from `9090` to `5397` — edit the
matching block in `/etc/kscore/server.yaml` and restart:

```bash
sudo systemctl restart kscore-server
```

### `kscorectl state apply` reports "agent not found"

The agent identifier passed via `--agent` must match the `agent.id`
in `/etc/kscore/agent.yaml`. Confirm what the agent registered as:

```bash
curl -fsS http://127.0.0.1:8080/api/v1/agents | jq '.[].id'
```

then re-issue `kscorectl state apply --agent <that-id>`.
