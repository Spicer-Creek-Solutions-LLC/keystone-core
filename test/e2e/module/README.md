# Module system end-to-end suite (Epic 14 task 17)

In-process, end-to-end integration tests for the Epic 14 module
system, driving the **real `kscore-module` CLI** against a **fake
registry server** (the task-9 HTTP handler over a filesystem-backed
storage, served by `httptest`).

## Scope — honest

This is the canonical Epic-14 e2e. The CLI is constructed exactly as
`cmd/kscore-module` constructs it (registry HTTP client + the
task-15 `pkg/module/testing` runner injected), so the behaviour
under test is the real binary's, over the real Go-mod wire protocol
on a real `net/http` server.

It proves, end-to-end:

- **Full author lifecycle** — `init → validate → test → build →
  sign → verify → publish → install --key` (signature + SHA-256
  hash verified, lockfile written) → **reproducible** re-install
  (byte-identical `module.lock`) → **execute the distributed
  artifact** (fetched from the registry, unzipped, run through the
  task-11 Starlark runtime + task-12 SDK).
- **Dependency graph over HTTP** — a root declaring `dependencies:`
  on two published leaves; `resolve` / `tree` / `install` against
  the HTTP fake registry; lockfile pins all and is reproducible.
- **Integrity failures** — wrong `--key` (signature mismatch),
  `--key` against an unsigned module, and a signed module whose
  stored ZIP is tampered after publish: each fails and **writes no
  lockfile**.
- **Registry Go-mod protocol conformance** — `.info` exposes only
  `{Version, Time}` (Hash stays internal, §4.18); `.sig` is `200`
  for a signed module and `404` for an unsigned one; re-publishing
  an existing version is a conflict.

It deliberately does **not** cover (post-v1.0, already ROADMAP-logged):

- a real network / OCI / NATS-Object-Store / SumDB registry backend
  (v1.0 is the filesystem registry);
- live `http` / `exec` / `secrets` capability hosts under
  `kscore-module test` (record/replay test hosts — the task-15
  `v1.x` ROADMAP entry); modules using them assert fail-closed in
  their `*_test.star` and exercise host-backed paths in
  `modules/examples/examples_test.go` with injected fakes.

The unsigned-tamper case is intentionally absent: a registry that
serves self-consistent (tampered) bytes + hash cannot be caught by
the hash gate alone — which is exactly why signing exists, and is
why the tamper case here uses a **signed** module.

## Run

```sh
make test-integration            # -race -tags=integration -p=1
# or just this package:
CGO_ENABLED=1 go test -race -tags=integration ./test/e2e/module/...
```

Build-tagged `integration`, so it is excluded from the default
`go test ./...` / `make test`.
