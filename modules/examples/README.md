# Example modules

Reference Keystone Core Starlark modules, spanning the v1.0
capability set and the common authoring shapes. Each is a valid,
signed-publishable, unit-tested module; `examples_test.go` drives
every one through validate → test → execute (and resolve, for
`opsbundle`).

| Module | Capabilities | Shows |
|--------|--------------|-------|
| [`hello`](hello/) | _none_ | minimal deterministic module |
| [`kvcache`](kvcache/) | `kv`, `log` | stateful in-process state + logging |
| [`fsreport`](fsreport/) | `fs.read`, `fs.write` | path scoping; fail-closed + audit |
| [`httpfetch`](httpfetch/) | `http.get` | domain/size-scoped outbound HTTP |
| [`cmdrun`](cmdrun/) | `exec` | command allowlisting |
| [`secretsync`](secretsync/) | `secrets.read`, `secrets.write`, `log` | independent read/write secret scopes |
| [`opsbundle`](opsbundle/) | `kv`, `log` (+ deps) | dependency resolution / lockfile pinning |

## The author UX

```sh
kscore-module init    keystone/mymod          # scaffold
kscore-module validate keystone-mymod         # check the manifest
kscore-module test     keystone-mymod         # run *_test.star
kscore-module build    keystone-mymod -o m.zip
kscore-module sign     m.zip --key local.key  # detached Cosign-compatible sig
kscore-module publish  m.zip --registry http://localhost:8181
kscore-module install  keystone/mymod@1.0.0   # resolve + verify + lockfile
```

`http.get`, `exec`, and `secrets.*` have no host wired under
`kscore-module test` (deterministic unit tests must not perform real
network/process/secret effects — record/replay test hosts are a
post-v1.0 ROADMAP item). Modules using them assert fail-closed +
audited behaviour in `*_test.star`; their live host-backed paths are
exercised in `examples_test.go` with injected fakes.
