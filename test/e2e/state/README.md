# State stdlib cross-distro matrix

End-to-end smoke for the Epic 08 stdlib modules that touch live system
state, run across the supported Linux distro matrix. Each distro boots
in a **privileged container with its real init system** (systemd or
OpenRC); the harness applies a state fixture **twice** and asserts the
second pass is a zero-change no-op (idempotency) — so the cross-distro
`package` (apt/dnf/apk) and `service` (systemd/OpenRC) backends are
exercised against the live system, not mocked.

Hermetic modules (`file`, `link`, `cmd`, `config`) are already covered
by `cmd/kscore-server/state_integration_test.go` and don't need a
container.

## Running

```sh
make test-cross-distro                 # all distros
bash test/e2e/state/run.sh alpine-3-19 # narrow to a subset
```

Requires Docker; the target **skips cleanly** (exit 0) when Docker is
absent, so it's safe in `make check` chains. It needs **privileged**
containers (systemd-as-PID-1 needs cgroup write access), so it is a
manual / Docker-host gate and is **not wired into CI** (the shared
runner pool is unprivileged).

## Distro matrix

| Distro       | Init system | Pkg mgr | Status |
|--------------|-------------|---------|--------|
| Debian 12    | systemd     | apt     | ✓      |
| Ubuntu 22.04 | systemd     | apt     | ✓      |
| Ubuntu 24.04 | systemd     | apt     | ✓      |
| Rocky 9      | systemd     | dnf     | ✓      |
| Alpine 3.19  | OpenRC      | apk     | ✓      |

## Layout

| File                 | Purpose |
|----------------------|---------|
| `run.sh`             | Orchestrator (invoked by `make test-cross-distro`). Builds the static harness, then per distro boots a privileged init container, runs `smoke.sh` via `docker exec`, and aggregates pass/fail. Skips without Docker. |
| `smoke.sh`           | Runs inside each container: refreshes the package index, renders the init-appropriate fixture with this distro's package/service names, runs the harness. |
| `harness/`           | A small static (CGO-free, runs on glibc + musl) Go program that compiles a state file and drives `Runner.Run` twice against the local registry — no kscore-server / NATS / Postgres needed. Has its own hermetic unit test. |
| `smoke.systemd.yaml` | Fixture for the systemd distros — `service` declared `running` + `enable`. |
| `smoke.openrc.yaml`  | Fixture for Alpine — `service` declared `stopped` + `enable` (OpenRC service supervision doesn't daemonise reliably in a container; the runlevel/enable path and status/exists queries do, so the smoke asserts those). |

## Coverage scope

The fixtures currently exercise `file` + `package` + `service` — the
modules whose backends vary across distros and now have cross-distro
implementations. `user`/`group`/`hostname`/… join the fixtures as their
own cross-distro backends land (the `user`/`group` modules currently
shell out to shadow-utils, absent on Alpine, which busybox replaces
with `adduser`/`addgroup` — tracked separately).

## Adding a distro

Append a `name|image|init|pkg_mgr|pkg|svc|boot` row to the `DISTROS`
array in `run.sh` (the `boot` field is the container's PID-1 command —
install + exec the init system), and tick the table above.
