# keystone/cmdrun

Runs an allowlisted command — the canonical day-2 ops automation
shape ("GitOps deploys it. We keep it running.").

| | |
|---|---|
| Capabilities | `exec` (`commands` allowlist, `working_dir`, `timeout`) |
| Demonstrates | command allowlisting; fail-closed on a non-allowlisted command |

Like `httpfetch`, `kscore-module test` wires no exec host, so the
unit tests assert fail-closed behaviour. The Go example test injects
an exec host and shows both the allowed (`echo`) and denied (`rm`)
paths with their audit entries.

## Run

```sh
kscore-module test modules/examples/cmdrun
```
