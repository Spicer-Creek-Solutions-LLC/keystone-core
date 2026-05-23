# Modules

Keystone Core ships two flavors of "module":

- **State modules** — the 35 built-in modules that ship with
  `kscore-server` for managing files, packages, services, networks,
  etc. Authored in Go; not user-extensible. They live at
  [`internal/statemgmt/stdlib/`](../internal/statemgmt/stdlib/) and
  are documented in
  [`docs/project/CONFIGURATION-REFERENCE.md`](../docs/project/CONFIGURATION-REFERENCE.md).

- **Extensibility modules** (this directory) — Cosign-signed
  Starlark modules with capability-based sandboxing. Authored in
  Starlark; resolved / verified / installed via the `kscore-module`
  CLI. This is how operators add their own state-managing logic
  without recompiling the server.

## Subdirectories

- [`examples/`](examples/) — reference modules covering the v1.0
  capability set + the common authoring shapes (minimal /
  stateful / file-scoped / HTTP-scoped / command-allowlisted /
  secrets-scoped / dependency-resolved). Each is unit-tested via
  `examples_test.go`. See [`examples/README.md`](examples/README.md)
  for the per-module capability matrix + author UX walkthrough.
- [`sdk/`](sdk/) — module-author SDK. Currently
  [`starlark/`](sdk/starlark/) (the Starlark runtime helpers + test
  scaffolding). Additional language SDKs land post-v1.0.

## Author workflow

The full lifecycle is in [`examples/README.md`](examples/README.md):

```
kscore-module init     keystone/mymod          # scaffold
kscore-module validate keystone-mymod          # manifest check
kscore-module test     keystone-mymod          # *_test.star
kscore-module build    keystone-mymod -o m.zip
kscore-module sign     m.zip --key local.key   # Cosign-compatible
kscore-module publish  m.zip --registry ...
kscore-module install  keystone/mymod@1.0.0    # resolve + verify
```

See [`docs/project/CLI-REFERENCE.md`](../docs/project/CLI-REFERENCE.md)
for the full subcommand reference.
