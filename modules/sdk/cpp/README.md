# Keystone Core C++ SDK

The official C++ SDK for building Keystone Core modules that compile to WebAssembly.

## Features

- **Header-only library**: No compilation required, just include headers
- **Modern C++17**: Uses standard library features
- **Type-safe API**: RAII and exception-based error handling
- **Cross-platform**: Works on Linux, macOS, and Windows
- **Capability-based security**: Only access what you're granted
- **Small binaries**: Optimized for size with LTO and dead code elimination

## Installation

The SDK is header-only. Just copy the `include/kscore` directory to your project:

```bash
# Copy headers to your project
cp -r include/kscore /your/project/include/
```

Or use as a CMake subdirectory:

```cmake
add_subdirectory(path/to/kscore-module-sdk-cpp)
target_link_libraries(your-module PRIVATE kscore-module-sdk)
```

## Requirements

- **C++17 compiler**
- **WASI SDK** or **Emscripten** for WebAssembly compilation

### Installing WASI SDK

```bash
# Download WASI SDK
wget https://github.com/WebAssembly/wasi-sdk/releases/download/wasi-sdk-21/wasi-sdk-21.0-linux.tar.gz
tar xf wasi-sdk-21.0-linux.tar.gz
export WASI_SDK_PATH=/path/to/wasi-sdk-21.0
```

### Installing Emscripten

```bash
# Clone and install Emscripten
git clone https://github.com/emscripten-core/emsdk.git
cd emsdk
./emsdk install latest
./emsdk activate latest
source ./emsdk_env.sh
```

## Quick Start

```cpp
#include <kscore/kscore.h>

int main() {
    kscore::log::info("Hello from Keystone Core!");
    return 0;
}
```

## Building

### With WASI SDK

```bash
# Create build directory
mkdir build && cd build

# Configure with WASI SDK
cmake -DCMAKE_TOOLCHAIN_FILE=$WASI_SDK_PATH/share/cmake/wasi-sdk.cmake ..

# Build
cmake --build .

# Output: module.wasm
```

### With Emscripten

```bash
# Create build directory
mkdir build && cd build

# Configure with Emscripten
emcmake cmake ..

# Build
cmake --build .

# Output: module.wasm
```

## Available Capabilities

### Filesystem (`kscore::fs`)

```cpp
#include <kscore/kscore.h>

// Read file as bytes
auto data = kscore::fs::read("/path/to/file");

// Read file as string
auto content = kscore::fs::read_string("/path/to/file");

// Write bytes
kscore::fs::write("/path/to/file", data);

// Write string
kscore::fs::write_string("/path/to/file", "Hello!");
```

**Required capability**: `fs.read` or `fs.write`

### HTTP (`kscore::http`)

```cpp
// GET request
auto response = kscore::http::get("https://api.example.com/data");
std::cout << "Status: " << response.status_code << std::endl;

// POST request
std::vector<uint8_t> body = {'d', 'a', 't', 'a'};
auto response = kscore::http::post("https://api.example.com/submit", body);
```

**Required capability**: `http.get` or `http.post`

### Command Execution (`kscore::exec`)

```cpp
// Run command
auto result = kscore::exec::run("ls", {"-la"});
std::cout << "Exit code: " << result.exit_code << std::endl;
std::cout << "Output: " << result.stdout_data << std::endl;

// Run with stdin
auto result = kscore::exec::run_with_input("grep", "pattern", {"pattern"});
```

**Required capability**: `exec`

### Logging (`kscore::log`)

```cpp
kscore::log::debug("Debug message");
kscore::log::info("Info message");
kscore::log::warn("Warning message");
kscore::log::error("Error message");
```

**Required capability**: `log`

### Key-Value Storage (`kscore::kv`)

```cpp
// Set a value
kscore::kv::set("my-key", "my-value");

// Get a value
auto value = kscore::kv::get("my-key");
if (value) {
    std::cout << "Value: " << *value << std::endl;
}
```

**Required capability**: `kv`

### System Information (`kscore::system`)

```cpp
// Get CPU information
auto cpu = kscore::system::get_cpu_info();
std::cout << "CPU: " << cpu << std::endl;
```

**Required capability**: `exec` (uses system commands internally)

### Cryptography (`kscore::crypto`)

```cpp
// Compute SHA256 hash
std::vector<uint8_t> data = {'d', 'a', 't', 'a'};
auto hash = kscore::crypto::sha256(data);
std::cout << "Hash: " << hash << std::endl;

// Hash a string
auto hash = kscore::crypto::sha256_string("my string");
```

**Required capability**: `exec` (uses external tools)

## Error Handling

The SDK uses exceptions for error handling:

```cpp
#include <kscore/kscore.h>

int main() {
    try {
        auto data = kscore::fs::read_string("/nonexistent");
    } catch (const kscore::FileSystemError& e) {
        kscore::log::error(e.what());
        return 1;
    } catch (const kscore::Error& e) {
        kscore::log::error(e.what());
        return 1;
    }
    return 0;
}
```

Error types:
- `kscore::CapabilityDeniedError` - Capability not granted
- `kscore::FileSystemError` - File I/O errors
- `kscore::HttpError` - HTTP request errors
- `kscore::ExecError` - Command execution errors
- `kscore::SerializationError` - JSON serialization errors
- `kscore::Error` - Base class for all errors

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
  command: |
    mkdir -p build && cd build
    cmake -DCMAKE_TOOLCHAIN_FILE=$WASI_SDK_PATH/share/cmake/wasi-sdk.cmake ..
    cmake --build .
  output: build/mymodule.wasm
```

## Example: Hello World

See `examples/hello-world/` for a complete example that:
- Gets CPU information
- Computes SHA256 hash
- Writes results to a file
- Returns JSON output

```bash
cd examples/hello-world
mkdir build && cd build
cmake -DCMAKE_TOOLCHAIN_FILE=$WASI_SDK_PATH/share/cmake/wasi-sdk.cmake ..
cmake --build .
```

## Optimization

The CMakeLists.txt is configured for small WASM binaries:

- `-Os`: Optimize for size
- `-flto`: Link-time optimization
- `-fno-exceptions`: Disable exceptions (optional, smaller binary)
- `-fno-rtti`: Disable RTTI (optional, smaller binary)
- `-Wl,--strip-all`: Strip all symbols
- `-Wl,--gc-sections`: Remove unused code

Typical module sizes: 100-300 KB

## Security

Modules run in a sandboxed WASM environment with:
- **No ambient authority**: Can only use granted capabilities
- **Memory isolation**: Cannot access host memory
- **Deterministic execution**: Same inputs → same outputs
- **Signature verification**: All modules must be signed

Only request capabilities you actually need.

## Platform Detection

The SDK automatically detects WASM compilation:

```cpp
#if defined(__wasm__) || defined(__wasm32__) || defined(__EMSCRIPTEN__)
    // WASM-specific code
#else
    // Non-WASM code (for testing)
#endif
```

## Dependencies

The SDK is self-contained and has no external dependencies. It includes:
- Minimal JSON parser/builder for API communication
- Platform detection macros
- Type-safe wrappers around host functions

## License

MIT
