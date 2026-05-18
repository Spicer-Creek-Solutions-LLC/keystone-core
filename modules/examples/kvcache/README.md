# keystone/kvcache

A stateful in-process cache + counter.

| | |
|---|---|
| Capabilities | `kv`, `log` |
| Demonstrates | stateful capability use + structured logging, both fully testable under `kscore-module test` |

`kv` is an in-process store and `log` is a discardable sink, so this
module's behaviour is exercised completely by its `*_test.star` unit
tests — no host wiring required.

## Run

```sh
kscore-module test modules/examples/kvcache
```

`main({"op": "incr", "key": "hits"})` → `{"value": 1}` (then `2`, …).
