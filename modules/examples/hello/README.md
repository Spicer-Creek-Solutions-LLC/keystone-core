# keystone/hello

The minimal example: a deterministic module that requests **no
capabilities**. With no capabilities granted there is nothing to
deny — this is the safest module shape and the recommended starting
point.

| | |
|---|---|
| Capabilities | _none_ |
| Demonstrates | the input → output contract, the deterministic core |

## Run

```sh
kscore-module validate modules/examples/hello
kscore-module test     modules/examples/hello
kscore-module build    modules/examples/hello -o hello-1.0.0.zip
```

`main({"name": "ops"})` → `{"message": "hello, ops!"}`.
