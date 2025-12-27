// Package runtime provides module runtime interfaces and types
package runtime

import (
	"context"
	"time"
)

// Runtime is the interface that all module runtimes must implement
type Runtime interface {
	// Close releases resources associated with the runtime
	Close() error
}

// StarlarkRuntimeOptions configures the Starlark runtime
type StarlarkRuntimeOptions struct {
	// MaxExecutionTime limits how long a script can run
	MaxExecutionTime time.Duration

	// MaxSteps limits the number of bytecode instructions
	MaxSteps uint64
}

// WasmRuntimeOptions configures the WASM runtime
type WasmRuntimeOptions struct {
	// MaxMemory limits the heap size in bytes
	MaxMemory uint64

	// FuelLimit limits the number of instructions executed
	FuelLimit uint64
}

// StarlarkRuntime extends Runtime with Starlark-specific methods
type StarlarkRuntime interface {
	Runtime

	// ExecuteFile executes a Starlark file and returns the result
	ExecuteFile(ctx context.Context, path string, input map[string]interface{}) (interface{}, error)
}

// WasmRuntime extends Runtime with WASM-specific methods
type WasmRuntime interface {
	Runtime

	// LoadModule loads a WASM module from bytes
	LoadModule(bytes []byte) error

	// ExecuteFunction executes a WASM function and returns the result
	ExecuteFunction(ctx context.Context, funcName string, input map[string]interface{}) (interface{}, error)
}
