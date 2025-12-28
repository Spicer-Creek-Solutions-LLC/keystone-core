# Keystone Core Rust SDK

The official Rust SDK for building Keystone Core modules that compile to WebAssembly.

## Features

- **Type-safe API**: Fully typed Rust interfaces for all Keystone Core capabilities
- **Zero-copy where possible**: Efficient memory usage between WASM and host
- **Cross-platform**: Works on Linux, macOS, and Windows
- **Capability-based security**: Only access what you're granted
- **Ergonomic macros**: `module_main!` and `export_fn!` for easy module creation

## Installation

Add to your `Cargo.toml`:

```toml
[dependencies]
kscore-module-sdk = "0.1"

[lib]
crate-type = ["cdylib"]
```

## Quick Start

```rust
use kscore_module_sdk::{module_main, Result};
use kscore_module_sdk::host::log;

fn my_module() -> Result<String> {
    log::info("Hello from Keystone Core!");
    Ok("Success".to_string())
}

module_main!(my_module);
```

## Building

Keystone Core modules target `wasm32-wasi`:

```bash
# Install the WASI target (one-time setup)
rustup target add wasm32-wasi

# Build your module
cargo build --target wasm32-wasi --release

# Output will be in target/wasm32-wasi/release/your_module.wasm
```

## Available Capabilities

### Filesystem (`fs`)

```rust
use kscore_module_sdk::host::fs;

// Read file as bytes
let data = fs::read_file("/path/to/file")?;

// Read file as string
let content = fs::read_string("/path/to/file")?;

// Write bytes
fs::write_file("/path/to/file", &data)?;

// Write string
fs::write_string("/path/to/file", "Hello!")?;
```

**Required capability**: `fs.read` or `fs.write`

### HTTP (`http`)

```rust
use kscore_module_sdk::host::http;

// GET request
let response = http::get("https://api.example.com/data")?;
println!("Status: {}", response.status_code);
println!("Body: {:?}", response.body);

// POST request
let body = b"request data";
let response = http::post("https://api.example.com/submit", body)?;
```

**Required capability**: `http.get` or `http.post`

### Command Execution (`exec`)

```rust
use kscore_module_sdk::host::exec;

// Run command
let result = exec::run("ls", &["-la".to_string()])?;
println!("Exit code: {}", result.exit_code);
println!("Output: {}", result.stdout);
println!("Errors: {}", result.stderr);

// Run with stdin
let result = exec::run_with_input(
    "grep",
    &["pattern".to_string()],
    "line1\nline2\npattern\n"
)?;
```

**Required capability**: `exec`

### Logging (`log`)

```rust
use kscore_module_sdk::host::log;

log::debug("Debug message");
log::info("Info message");
log::warn("Warning message");
log::error("Error message");
```

**Required capability**: `log`

### Key-Value Storage (`kv`)

```rust
use kscore_module_sdk::host::kv;

// Set a value
kv::set("my-key", "my-value")?;

// Get a value
if let Some(value) = kv::get("my-key")? {
    println!("Value: {}", value);
}
```

**Required capability**: `kv`

### System Information (`system`)

```rust
use kscore_module_sdk::host::system;

// Get CPU information
let cpu = system::cpu_info()?;
println!("CPU: {}", cpu);
```

**Required capability**: `exec` (uses system commands internally)

### Cryptography (`crypto`)

```rust
use kscore_module_sdk::host::crypto;

// Compute SHA256 hash
let hash = crypto::sha256(b"data to hash")?;
println!("Hash: {}", hash);

// Hash a string
let hash = crypto::sha256_string("my string")?;
```

**Required capability**: `exec` (uses external tools)

## Error Handling

The SDK provides a comprehensive error type:

```rust
use kscore_module_sdk::{Error, Result};

fn my_function() -> Result<String> {
    // Filesystem error
    fs::read_string("/nonexistent")?;

    // HTTP error
    http::get("https://invalid")?;

    // Custom error
    Err(Error::other("Something went wrong"))
}
```

Error types:
- `Error::CapabilityDenied` - Capability not granted
- `Error::FileSystem` - File I/O errors
- `Error::Http` - HTTP request errors
- `Error::Exec` - Command execution errors
- `Error::Serialization` - JSON serialization errors
- `Error::Other` - Generic errors

## Module Manifest

Create a `module.yaml` to describe your module:

```yaml
name: myorg/mymodule
version: 1.0.0
description: My Keystone Core module

type: wasm

capabilities:
  - fs.read
  - http.get
  - log

limits:
  memory: 10MB
  timeout: 30s

entrypoint: mymodule.wasm

build:
  command: cargo build --target wasm32-wasi --release
  output: target/wasm32-wasi/release/mymodule.wasm
```

## Example: Hello World

See `examples/hello-world/` for a complete example that:
- Gets CPU information
- Computes SHA256 hash
- Writes results to a file
- Returns JSON output

```bash
cd examples/hello-world
cargo build --target wasm32-wasi --release
```

## Testing

When testing modules, note that host functions are stubbed in non-WASM environments:

```rust
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_module() {
        // In non-WASM mode, host functions return errors
        // Use integration tests with a real WASM runtime for full testing
    }
}
```

For integration testing, use the Keystone Core test framework with a WASM runtime.

## Optimization

The SDK is configured for small WASM binaries:

```toml
[profile.release]
opt-level = "z"     # Optimize for size
lto = true          # Link-time optimization
codegen-units = 1   # Better optimization
strip = true        # Remove debug symbols
```

Typical module sizes: 100-500 KB

## Security

Modules run in a sandboxed WASM environment with:
- **No ambient authority**: Can only use granted capabilities
- **Memory isolation**: Cannot access host memory
- **Deterministic execution**: Same inputs → same outputs
- **Signature verification**: All modules must be signed

Only request capabilities you actually need.

## API Reference

Full API documentation:
```bash
cargo doc --open
```

## License

MIT
