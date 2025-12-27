# TitanAnvil Go SDK

The official Go SDK for building TitanAnvil modules that compile to WebAssembly using TinyGo.

## Features

- **TinyGo-optimized**: Designed for small WASM binaries
- **Type-safe API**: Fully typed Go interfaces for all TitanAnvil capabilities
- **Cross-platform**: Works on Linux, macOS, and Windows
- **Capability-based security**: Only access what you're granted
- **Simple API**: Idiomatic Go code

## Installation

```bash
go get github.com/titananvil/titan-module-sdk-go
```

## Requirements

- **TinyGo 0.30+**: Required for wasm32-wasi compilation
- **Go 1.21+**: For development and testing

Install TinyGo:
```bash
# macOS
brew install tinygo

# Linux
wget https://github.com/tinygo-org/tinygo/releases/download/v0.30.0/tinygo_0.30.0_amd64.deb
sudo dpkg -i tinygo_0.30.0_amd64.deb

# Windows
choco install tinygo
```

## Quick Start

```go
package main

import (
    titansdk "github.com/titananvil/titan-module-sdk-go"
)

func main() {
    titansdk.LogInfo("Hello from TitanAnvil!")
}
```

## Building

TitanAnvil modules target `wasm32-wasi`:

```bash
# Build your module
tinygo build -o module.wasm -target wasm32-wasi .

# Optimize for size
tinygo build -o module.wasm -target wasm32-wasi -opt=z .
```

## Available Capabilities

### Filesystem

```go
import titansdk "github.com/titananvil/titan-module-sdk-go"

// Read file as bytes
data, err := titansdk.ReadFile("/path/to/file")

// Read file as string
content, err := titansdk.ReadString("/path/to/file")

// Write bytes
err := titansdk.WriteFile("/path/to/file", data)

// Write string
err := titansdk.WriteString("/path/to/file", "Hello!")
```

**Required capability**: `fs.read` or `fs.write`

### HTTP

```go
// GET request
response, err := titansdk.HTTPGet("https://api.example.com/data")
if err == nil {
    fmt.Printf("Status: %d\n", response.StatusCode)
    fmt.Printf("Body: %s\n", string(response.Body))
}

// POST request
body := []byte("request data")
response, err := titansdk.HTTPPost("https://api.example.com/submit", body)
```

**Required capability**: `http.get` or `http.post`

### Command Execution

```go
// Run command
result, err := titansdk.Exec("ls", "-la")
if err == nil {
    fmt.Printf("Exit code: %d\n", result.ExitCode)
    fmt.Printf("Output: %s\n", result.Stdout)
}

// Run with stdin
result, err := titansdk.ExecWithInput("grep", "pattern", "line1\nline2\npattern\n")
```

**Required capability**: `exec`

### Logging

```go
titansdk.LogDebug("Debug message")
titansdk.LogInfo("Info message")
titansdk.LogWarn("Warning message")
titansdk.LogError("Error message")
```

**Required capability**: `log`

### Key-Value Storage

```go
// Set a value
err := titansdk.KvSet("my-key", "my-value")

// Get a value
value, exists, err := titansdk.KvGet("my-key")
if exists {
    fmt.Printf("Value: %s\n", value)
}
```

**Required capability**: `kv`

### System Information

```go
// Get CPU information
cpu, err := titansdk.GetCPUInfo()
if err == nil {
    fmt.Printf("CPU: %s\n", cpu)
}
```

**Required capability**: `exec` (uses system commands internally)

### Cryptography

```go
// Compute SHA256 hash
hash, err := titansdk.SHA256([]byte("data to hash"))
if err == nil {
    fmt.Printf("Hash: %s\n", hash)
}

// Hash a string
hash, err := titansdk.SHA256String("my string")
```

**Required capability**: `exec` (uses external tools)

## Error Handling

```go
import titansdk "github.com/titananvil/titan-module-sdk-go"

func myFunction() error {
    // Filesystem error
    _, err := titansdk.ReadString("/nonexistent")
    if err != nil {
        return err
    }

    // Custom error
    return titansdk.NewError("Something went wrong")
}
```

Error types:
- `ErrorTypeCapabilityDenied` - Capability not granted
- `ErrorTypeFileSystem` - File I/O errors
- `ErrorTypeHTTP` - HTTP request errors
- `ErrorTypeExec` - Command execution errors
- `ErrorTypeSerialization` - JSON serialization errors
- `ErrorTypeOther` - Generic errors

## Module Manifest

Create a `module.yaml` to describe your module:

```yaml
name: myorg/mymodule
version: 1.0.0
description: My TitanAnvil module

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
  command: tinygo build -o mymodule.wasm -target wasm32-wasi -opt=z .
  output: mymodule.wasm
```

## Example: Hello World

See `examples/hello-world/` for a complete example that:
- Gets CPU information
- Computes SHA256 hash
- Writes results to a file
- Returns JSON output

```bash
cd examples/hello-world
tinygo build -o hello-world.wasm -target wasm32-wasi -opt=z .
```

## Testing

Host functions are stubbed in non-WASM builds:

```go
package main

import (
    "testing"
    titansdk "github.com/titananvil/titan-module-sdk-go"
)

func TestModule(t *testing.T) {
    // In non-WASM mode, host functions return errors
    _, err := titansdk.ReadFile("/test")
    if err == nil {
        t.Error("Expected error in non-WASM environment")
    }
}
```

For integration testing, use the TitanAnvil test framework with a WASM runtime.

## Optimization

TinyGo produces small WASM binaries. Use these flags for best results:

```bash
tinygo build -o module.wasm -target wasm32-wasi \
    -opt=z \           # Optimize for size
    -no-debug \        # Remove debug info
    -panic=trap        # Smaller panic handling
    .
```

Typical module sizes: 50-200 KB

## Security

Modules run in a sandboxed WASM environment with:
- **No ambient authority**: Can only use granted capabilities
- **Memory isolation**: Cannot access host memory
- **Deterministic execution**: Same inputs → same outputs
- **Signature verification**: All modules must be signed

Only request capabilities you actually need.

## License

MIT
