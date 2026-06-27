---
title: Authoring & Publishing a Module
weight: 3
---

A **module** is a unit of custom logic written in
[Starlark](https://github.com/bazelbuild/starlark) (a small, deterministic
Python dialect). Modules have a single entrypoint — `main(input)` — that
transforms an input dict into an output dict, and a manifest that declares
their identity, version, and **capabilities** (what side effects they are
allowed to perform). A module that requests no capabilities can have no
side effects: the safest possible default.

This guide writes a module, tests it, runs it locally, then builds, signs,
publishes, and installs it. It uses the `kscore-module` and
`kscore-registry` CLIs.

## Prerequisites

- Keystone Core installed (`kscore-module version` works).
- `openssl` (for generating a signing key in the publish step).

## 1. Scaffold a module

```sh
kscore-module init keystone/greeter --dir greeter
```

That writes:

```text
greeter/
├── manifest.yaml   # identity, version, type, entrypoint, capabilities
└── main.star       # the module: def main(input)
```

The manifest:

```yaml
name: keystone/greeter
version: 0.1.0
type: starlark
entrypoint: main.star
description: A Keystone Core module
license: Apache-2.0
```

## 2. Write the logic

Edit `greeter/main.star`. `main(input)` is the entrypoint; factor out
pure helpers so they are easy to test:

```python
def greet(name):
    return "hello, %s!" % (name or "world")

def main(input):
    return {"message": greet(input.get("name", ""))}
```

## 3. Add unit tests

Tests live in `*_test.star` next to the module. Every top-level
`test_*` function is a test; the module's functions are in scope
directly. Create `greeter/main_test.star`:

```python
def test_default():
    assert.eq("hello, world!", main({})["message"])

def test_named():
    assert.eq("hello, ada!", main({"name": "ada"})["message"])
```

Tests must be deterministic — they run without any capabilities, so they
can't touch the network or filesystem.

## 4. Validate, test, run

```sh
kscore-module validate greeter   # manifest is well-formed
kscore-module test     greeter   # run the *_test.star files
kscore-module run      greeter '{"name":"ada"}' --skip-verification
```

Expected output:

```text
ok: keystone/greeter@0.1.0
tests: 2 passed, 0 failed
{
  "message": "hello, ada!"
}
```

`run` verifies a module's signature by default; `--skip-verification` is
the local-development escape hatch (you sign for real in step 6).

## 5. Capabilities

The scaffold requests **no** capabilities, so `greeter` is a pure
transform. A module that needs side effects declares them in the
manifest, and the runtime denies anything not granted. For example, a
module that fetches a URL declares the `http` capability:

```yaml
capabilities:
  http:
    allow_hosts: ["example.com"]
```

Browse the
[example modules](https://codeberg.org/Spicer-Creek-Solutions-LLC/keystone-core/src/branch/main/modules/examples)
for the capability shapes — `httpfetch` (http), `cmdrun` (exec),
`secretsync` (secrets), `fsreport` (filesystem).

## 6. Build & sign

Package the module as a ZIP, then sign it. Signatures are detached and
Cosign-compatible.

```sh
kscore-module build greeter -o greeter-0.1.0.zip

# A signing key (ECDSA P-256 is fine; SEC1 / PKCS8 / PKCS1 are accepted)
openssl ecparam -name prime256v1 -genkey -noout -out signing.key

kscore-module sign greeter-0.1.0.zip --key signing.key
# → greeter-0.1.0.zip.sig
```

Verify the artifact against the matching public key — what a consumer
does before trusting it:

```sh
openssl ec -in signing.key -pubout -out signing.pub
kscore-module verify greeter-0.1.0.zip --key signing.pub
```

## 7. Publish

Run a registry, then publish the signed artifact to it. In one terminal:

```sh
kscore-registry serve --addr :8181 --dir ./registry-data
```

In another:

```sh
kscore-module publish greeter-0.1.0.zip --registry http://localhost:8181
```

The publish uploads the ZIP and its `.sig` sidecar.

## 8. Install

Resolve, download, verify, and lock the module (and any dependencies)
from the registry:

```sh
kscore-module install keystone/greeter@0.1.0 --registry http://localhost:8181
```

Install writes a `module.lock` pinning the resolved versions + digests, so
re-installs are reproducible and tamper-evident.

## Next steps

- [Authoring a Blueprint](../blueprint-authoring/) — package a whole
  deployment.
- The [CLI reference](/docs/reference/cli-reference/) for every
  `kscore-module` and `kscore-registry` flag.
