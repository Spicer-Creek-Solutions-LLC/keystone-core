# keystone/httpfetch

Fetches an HTTP resource and returns its status + size.

| | |
|---|---|
| Capabilities | `http.get` (`domains` allowlist, `max_response_size`, `timeout`) |
| Demonstrates | outbound-HTTP scoping; the post-v1.0 record/replay test-host gap |

`kscore-module test` does not wire an HTTP host (deterministic unit
tests must not hit the network — record/replay fixtures are a
post-v1.0 ROADMAP item), so the capability call **fails closed and
is audited**. The unit tests assert the pure response-shaping logic
and that fail-closed behaviour; the live request, domain-allow, and
domain-deny paths are exercised in the Go example test with an
injected HTTP host.

## Run

```sh
kscore-module test modules/examples/httpfetch
```
