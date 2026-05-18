# keystone/secretsync

Reads a credential from one scoped secret path, rotates it, and
writes the copy to a different scoped path.

| | |
|---|---|
| Capabilities | `secrets.read` (`app/source/*`), `secrets.write` (`app/dest/*`), `log` |
| Demonstrates | per-path secret scoping; read and write scopes are independent |

`secrets.read` and `secrets.write` are scoped to **different** path
prefixes — a real least-privilege pattern (read source, write
destination, never the reverse). `kscore-module test` wires no
secrets host so the unit tests assert fail-closed; the Go example
test injects an in-memory secrets host and shows the round-trip plus
the cross-scope denials (reading `app/dest/*` or writing
`app/source/*` is rejected and audited).

## Run

```sh
kscore-module test modules/examples/secretsync
```
