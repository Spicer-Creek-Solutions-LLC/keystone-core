# keystone/opsbundle

A composite day-2 ops module that **pins companion modules** as
dependencies.

| | |
|---|---|
| Capabilities | `kv`, `log` |
| Dependencies | `keystone/httpfetch >=1.0.0`, `keystone/cmdrun >=1.0.0` |
| Demonstrates | the dependency-resolution UX: `resolve`, `tree`, lockfile pinning, reproducible installs |

`dependencies:` is a **distribution** relationship — `kscore-module
resolve` / `tree` / `install` fetch the pinned companions and lock
their exact versions (Minimum Version Selection) for a reproducible
supply chain. Runtime `load()` of another module's code is a
post-v1.0 item (see the project ROADMAP); this entrypoint is
standalone.

## Run

```sh
# publish the companions first, then:
kscore-module resolve modules/examples/opsbundle --registry http://localhost:8181
kscore-module tree    modules/examples/opsbundle --registry http://localhost:8181
```

The Go example test publishes the companions to an in-process
registry and asserts the resolved lockfile pins all three modules.
