# Keystone Core Example Modules

This directory contains example modules demonstrating Keystone Core module development in all supported languages.

## Hello World Examples

All hello world examples perform the same operations to demonstrate language equivalence:

1. Get the CPU make and model
2. Compute the SHA256 hash of the CPU information
3. Write the results to a file in the temp directory (`hello-from-kscore-{language}.txt`)
4. Return the results as JSON

Results should be identical across all languages (except for the file name).

Note: SDK-focused hello world examples live under `modules/sdk/*/examples/hello-world`.
Those are minimal SDK usage samples, while this directory focuses on full module
packages that mirror real-world layouts.

### Starlark

```bash
cd hello-world-starlark
# Run with Keystone Core runtime
kscorectl module run .
```

**Language**: Starlark (Python-like)
**Module Type**: Starlark
**Binary Size**: N/A (interpreted)
**Build Time**: N/A (interpreted)

### Rust

```bash
cd hello-world-rust
cargo build --target wasm32-wasi --release
# Output: target/wasm32-wasi/release/hello_world_rust.wasm
```

**Language**: Rust
**Module Type**: WASM (wasm32-wasi)
**Binary Size**: ~200-300 KB (optimized)
**Build Time**: ~5-10 seconds

### Go

```bash
cd hello-world-go
tinygo build -o hello-world-go.wasm -target wasm32-wasi -opt=z .
# Output: hello-world-go.wasm
```

**Language**: Go (TinyGo)
**Module Type**: WASM (wasm32-wasi)
**Binary Size**: ~100-150 KB (optimized)
**Build Time**: ~3-5 seconds

### C++

```bash
cd hello-world-cpp
mkdir build && cd build
cmake -DCMAKE_TOOLCHAIN_FILE=$WASI_SDK_PATH/share/cmake/wasi-sdk.cmake ..
cmake --build .
# Output: build/hello-world-cpp.wasm
```

**Language**: C++17
**Module Type**: WASM (wasm32-wasi)
**Binary Size**: ~150-200 KB (optimized)
**Build Time**: ~2-4 seconds

## Testing Examples

All examples can be tested with the Keystone Core test framework:

```bash
# Test all examples
kscorectl module test examples/

# Test specific example
kscorectl module test examples/hello-world-rust
```

## Capabilities Required

All hello world examples require the same capabilities:

- `exec` - For CPU detection via system commands (sysctl, wmic, /proc/cpuinfo)
- `fs.write` - For writing the output file
- `log` - For logging progress

## Expected Output

All examples produce identical output (except for file path):

```json
{
  "cpu_info": "Intel(R) Core(TM) i9-9900K CPU @ 3.60GHz",
  "hash": "a1b2c3d4e5f6...",
  "file_path": "/tmp/hello-from-kscore-{language}.txt"
}
```

And create a file containing:

```
CPU: Intel(R) Core(TM) i9-9900K CPU @ 3.60GHz
SHA256: a1b2c3d4e5f6...
```

## Performance Comparison

Approximate execution times (may vary by system):

| Language  | Startup | Execution | Total |
|-----------|---------|-----------|-------|
| Starlark  | ~1ms    | ~50ms     | ~51ms |
| Rust      | ~2ms    | ~40ms     | ~42ms |
| Go        | ~2ms    | ~45ms     | ~47ms |
| C++       | ~2ms    | ~40ms     | ~42ms |

WASM modules have slightly higher startup overhead but similar execution performance.

## Language Choice Guide

### Choose Starlark when:
- Rapid prototyping and iteration
- Simple scripting tasks
- No compilation required
- Python-like syntax preferred
- Smaller deployments

### Choose Rust when:
- Maximum performance critical
- Complex data structures
- Strong type safety required
- Large-scale production deployments

### Choose Go when:
- Balance of simplicity and performance
- Familiar with Go ecosystem
- Quick compilation times important
- Moderate binary size acceptable

### Choose C++ when:
- Existing C++ codebase
- Header-only library preferred
- Fine-grained control needed
- Legacy system integration

## Next Steps

- Explore stdlib modules in `modules/stdlib/`
- Review SDK documentation in `modules/sdk/{language}/`
- Compare SDK hello world samples in `modules/sdk/*/examples/hello-world`
- Build your own modules with `kscorectl module init`
