# keystone/fsreport

Reads a text file and writes a one-line summary — the capability
**security** showcase.

| | |
|---|---|
| Capabilities | `fs.read`, `fs.write` (scoped `paths`, `denied_paths`, `max_file_size`) |
| Demonstrates | path scoping enforced by the runtime, not the module; fail-closed + audited on an out-of-scope path |

The manifest scopes both capabilities to `**/sandbox/**`. The module
code has no path checks of its own — the capability layer denies and
audits anything outside the allowed glob. The unit tests assert the
pure summary logic and that out-of-scope reads/writes fail closed;
the real scoped read+write round-trip is shown in the Go example
test against a temporary `sandbox/` directory.

## Run

```sh
kscore-module test modules/examples/fsreport
```
