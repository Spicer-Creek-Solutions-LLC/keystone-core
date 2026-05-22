# Getting started

A 30-minute walkthrough from a fresh Ubuntu VM to a running
Keystone Core topology with one applied state. Covers the v1.0
baseline path; the docker-compose harness from epic 19 task 1 does
the heavy lifting so you don't need to provision NATS or Postgres
by hand.

## Prerequisites

Ubuntu 22.04+ or any Linux distro with:

- Go 1.26.3+ (`go.mod` toolchain pin).
- Docker 24+ with Compose v2 (the `docker compose` subcommand, not
  the legacy `docker-compose` binary).
- Git.
- Make.

On a fresh Ubuntu VM, the one-liner:

```bash
sudo apt-get update && sudo apt-get install -y golang-1.26 docker.io docker-compose-v2 git make
# Or use the official Go release for the latest patch:
# wget https://go.dev/dl/go1.26.3.linux-amd64.tar.gz && sudo tar -C /usr/local -xzf go1.26.3.linux-amd64.tar.gz && export PATH=$PATH:/usr/local/go/bin
```

Add your user to the `docker` group (`sudo usermod -aG docker $USER`,
then log out + back in) so `docker compose` works without `sudo`.

## 1. Clone + verify the toolchain

```bash
git clone https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core.git
cd keystone-core
go version    # 1.26.3 or newer
docker compose version
make help     # lists every available target
```

## 2. Bring up the topology

```bash
make e2e-up
```

What this does:

- Builds the kscore-server and kscore-agent images.
- Starts a 5-container compose stack: 1× kscore-server, 2× kscore-agent, Postgres 16, NATS 2.10.
- Waits for every container's healthcheck (~30–90 s on first run; most of that is image pull + Go build).

Ports exposed on `127.0.0.1`:

| Port | Service | Use |
|------|---------|-----|
| `8080` | server HTTP | `/health/*`, `/metrics`, REST API |
| `9090` | server gRPC | operator-facing gRPC |
| `8081` | server GitOps webhook | inbound webhook receiver |
| `5432` | postgres | DB access for queries |
| `8222` | nats monitoring | broker introspection |

Quick check:

```bash
curl http://127.0.0.1:8080/health/ready
# {"ready":true,"components":{...}}
```

## 3. Grab the dev admin API key

The server logs the cleartext exactly once at boot (see
[`pkg/api/apikeys/dev.go`](../../pkg/api/apikeys/dev.go)). Extract it:

```bash
DEV_KEY=$(docker logs kscore-e2e-server 2>&1 \
  | grep "DEV API KEY GENERATED" \
  | python3 -c 'import sys, json; print(json.loads(sys.stdin.read().strip().split("\n")[-1])["key"])')
echo "Admin API key: $DEV_KEY"
```

(In production, you'd use `kscorectl apikey create` instead.)

## 4. Verify both agents registered

```bash
grpcurl -plaintext -H "authorization: Bearer $DEV_KEY" \
    127.0.0.1:9090 keystone.core.v1.ControlPlaneService/ListAgents | jq
```

Expect two entries (`agent-1`, `agent-2`), both with `status: AGENT_STATUS_CONNECTED`.

If you don't have `grpcurl` handy, the REST equivalent:

```bash
curl -s -H "Authorization: Bearer $DEV_KEY" http://127.0.0.1:8080/api/v1/agents | jq
```

## 5. Run a command on agent-1

The agents in the docker-compose harness are distroless (no shell),
so commands must reference the `/usr/local/bin/kscore` binary that
ships in the image — pick `--version` for a clean exit-0:

```bash
grpcurl -plaintext -H "authorization: Bearer $DEV_KEY" \
    -d '{
          "agent_id":"agent-1",
          "command":"/usr/local/bin/kscore",
          "args":["--version"],
          "timeout_seconds":10
        }' \
    127.0.0.1:9090 keystone.core.v1.ControlPlaneService/ExecuteCommand
```

You get a stream of events: a `command_id` first, then a
`completion` with `exit_code: 0`. (v1.0 doesn't yet stream stdout
chunks; the bridge from agent response to BatchAgentResult lands in
a v1.x ROADMAP item.)

## 6. Apply state

Write a tiny declaration to a file:

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

Apply via gRPC:

```bash
grpcurl -plaintext -H "authorization: Bearer $DEV_KEY" \
    -d @ \
    127.0.0.1:9090 keystone.core.v1.StateService/ApplyState <<JSON
{
  "yaml_content": "$(base64 -w0 /tmp/hello.yaml)",
  "agent_id": "agent-1",
  "source": "getting-started.yaml"
}
JSON
```

The stream emits a `run_id`, per-declaration `decl_result` events,
and a terminal `Completed` status. `/tmp/keystone-hello` now exists
inside the agent container.

## 7. Browse the audit log

Every command, every state run, every secret access flows through
the audit store. Query it via PolicyService:

```bash
grpcurl -plaintext -H "authorization: Bearer $DEV_KEY" \
    -d '{"limit":10}' \
    127.0.0.1:9090 keystone.core.v1.PolicyService/GetAuditLog | jq
```

## 8. Tear down

```bash
make e2e-down
```

Removes containers, volumes, and the docker network.

## Next steps

- **The CLI reference**: [`CLI-REFERENCE.md`](CLI-REFERENCE.md) lists every `kscore-*` binary and every subcommand.
- **The API reference**: [`API-REFERENCE.md`](API-REFERENCE.md) indexes every gRPC RPC + REST endpoint with links to the canonical proto / openapi sources.
- **The configuration reference**: [`CONFIGURATION-REFERENCE.md`](CONFIGURATION-REFERENCE.md) catalogs every config key.
- **Architecture**: [`DESIGN.md`](DESIGN.md) for the high-level design, [`../../PROJECT-DETAILS.md`](../../PROJECT-DETAILS.md) for per-domain implementation detail.
- **Security**: [`SECURITY-GOVERNANCE.md`](SECURITY-GOVERNANCE.md) covers the v1.0 four-scan baseline + the disclosure process.

## Troubleshooting

### `make e2e-up` hangs at "Container kscore-e2e-server Healthy"

The first run downloads Go modules + builds the binary inside the
container. On a cold cache this can take 60–90 s. Run
`docker logs -f kscore-e2e-server` in another shell to see progress.

### "Authorization: Bearer" returns 401

The dev API key is cleartext-once per server-process. If you
restarted the server (`make e2e-down && make e2e-up`), regenerate
the variable via step 3.

### `grpcurl` not installed

```bash
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
```

The gRPC server doesn't have reflection enabled in dev mode; pass
`-import-path api/proto -proto api/proto/keystone/core/v1/controlplane.proto`
(or pre-compiled descriptors) for grpcurl to discover services.
Alternative: drive everything via REST at `:8080`.

### State apply fails inside the distroless agent

`/tmp` is writable in the distroless image, but parent directories
under `/etc`, `/var`, etc. are read-only. Stick to `/tmp/...` for
quick experiments; real deployments use a full Linux host.

### Tests want Postgres

`make test-integration` is gated on `KSCORE_TEST_POSTGRES_DSN`. If
unset, the Postgres-dependent tests skip. Set it to a reachable
Postgres (the e2e topology exposes one at `127.0.0.1:5432`):

```bash
export KSCORE_TEST_POSTGRES_DSN="postgres://kscore:kscore@127.0.0.1:5432/kscore?sslmode=disable"
make test-integration
```
