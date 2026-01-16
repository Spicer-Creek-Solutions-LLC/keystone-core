---
title: "SDK Reference"
weight: 13
description: >
  Language SDKs for building Keystone Core modules
---

## Overview

Keystone Core provides language SDKs for writing modules that compile to WASM and run in the module runtime.

Available SDKs:
- Go (TinyGo)
- Rust
- C++

All SDKs ship with a `hello-world` example under `modules/sdk/*/examples/hello-world`.

## Go SDK

Path: `modules/sdk/go`

Key notes:
- Build with TinyGo to target `wasm32-wasi`.
- Implement the `Run` entry point and use the SDK host APIs.

Example:

```bash
cd modules/sdk/go/examples/hello-world
tinygo build -o hello-world-go.wasm -target wasm32-wasi -opt=z .
```

## Rust SDK

Path: `modules/sdk/rust`

Key notes:
- Build with `wasm32-wasi`.
- Use the SDK crate to access host functions and types.

Example:

```bash
cd modules/sdk/rust/examples/hello-world
cargo build --target wasm32-wasi --release
```

## C++ SDK

Path: `modules/sdk/cpp`

Key notes:
- Build with the WASI SDK toolchain.
- Include `kscore` headers from the SDK.

Example:

```bash
cd modules/sdk/cpp/examples/hello-world
cmake -B build -DCMAKE_TOOLCHAIN_FILE=$WASI_SDK_PATH/share/cmake/wasi-sdk.cmake .
cmake --build build
```

## Module Packaging

Each SDK example includes a `module.yaml` manifest. Use `kscorectl module build` and
`kscorectl module sign` for packaging and signing.

## Benchmarks

The Go SDK includes micro-benchmarks for core helper functions. Run them from
`modules/sdk/go`:

```bash
go test -bench .
```

Sample results (Linux x86_64, Go 1.25):

```
BenchmarkLogLevelString-8    1000000000    0.1946 ns/op
BenchmarkErrorString-8       16122675     75.79 ns/op
BenchmarkLogInfoStub-8       1000000000    0.1983 ns/op
```
