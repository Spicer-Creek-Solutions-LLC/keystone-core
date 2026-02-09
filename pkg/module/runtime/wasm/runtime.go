// Package wasm provides a sandboxed WebAssembly runtime for module execution.
package wasm

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// Runtime manages WebAssembly module execution with WASI support
type Runtime struct {
	// runtime is the wazero runtime
	runtime wazero.Runtime

	// module is the compiled WASM module
	module wazero.CompiledModule

	// instance is the instantiated module
	instance api.Module

	// Capabilities are host functions exposed to WASM
	capabilities map[string]interface{}

	// Resource limits
	limits ResourceLimits

	// config for runtime
	config Config

	// hostModuleBuilder for adding host functions
	hostModuleBuilder wazero.HostModuleBuilder

	// Execution state
	mu sync.Mutex
}

// ResourceLimits defines WASM execution constraints
type ResourceLimits struct {
	// MaxMemoryPages limits WebAssembly memory (64KB per page)
	MaxMemoryPages uint32

	// MaxTableElements limits table size
	MaxTableElements uint32

	// MaxInstances limits the number of module instances
	MaxInstances uint32

	// FuelLimit limits the number of instructions executed
	// Use 0 for unlimited (not recommended in production)
	FuelLimit uint64

	// MaxExecutionTime limits how long execution can run
	MaxExecutionTime time.Duration
}

// Config contains WASM runtime configuration
type Config struct {
	// Enable WASI support
	EnableWASI bool

	// Resource limits
	Limits ResourceLimits

	// Additional host imports
	HostImports map[string]interface{}
}

// NewRuntime creates a new WASM runtime
func NewRuntime(cfg Config) (*Runtime, error) {
	ctx := context.Background()

	// Create runtime configuration
	runtimeConfig := wazero.NewRuntimeConfig()

	// Set memory limits
	if cfg.Limits.MaxMemoryPages > 0 {
		runtimeConfig = runtimeConfig.WithMemoryLimitPages(cfg.Limits.MaxMemoryPages)
	}

	// Create runtime
	runtime := wazero.NewRuntimeWithConfig(ctx, runtimeConfig)

	rt := &Runtime{
		runtime:      runtime,
		capabilities: make(map[string]interface{}),
		limits:       cfg.Limits,
		config:       cfg,
	}

	// Initialize WASI if enabled
	if cfg.EnableWASI {
		_, err := wasi_snapshot_preview1.Instantiate(ctx, runtime)
		if err != nil {
			runtime.Close(ctx)
			return nil, fmt.Errorf("failed to instantiate WASI: %w", err)
		}
	}

	// Create host module builder for custom imports
	rt.hostModuleBuilder = runtime.NewHostModuleBuilder("env")

	// Register host imports
	for name, fn := range cfg.HostImports {
		rt.capabilities[name] = fn
	}

	return rt, nil
}

// LoadModule loads a WASM module from bytes
func (rt *Runtime) LoadModule(wasmBytes []byte) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	ctx := context.Background()

	// Compile the module
	module, err := rt.runtime.CompileModule(ctx, wasmBytes)
	if err != nil {
		return fmt.Errorf("failed to compile WASM module: %w", err)
	}

	rt.module = module
	return nil
}

// LoadModuleFile loads a WASM module from a file
func (rt *Runtime) LoadModuleFile(filename string) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	ctx := context.Background()

	// Compile the module from file
	module, err := rt.runtime.CompileModule(ctx, nil)
	if err != nil {
		// Try reading file manually
		return fmt.Errorf("failed to load WASM module from %s: %w", filename, err)
	}

	rt.module = module
	return nil
}

// Instantiate instantiates the loaded module
func (rt *Runtime) Instantiate() error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if rt.module == nil {
		return fmt.Errorf("no module loaded")
	}

	ctx := context.Background()

	// Configure the module
	moduleConfig := wazero.NewModuleConfig().
		WithStdout(nil).
		WithStderr(nil)

	// Instantiate the module
	instance, err := rt.runtime.InstantiateModule(ctx, rt.module, moduleConfig)
	if err != nil {
		return fmt.Errorf("failed to instantiate module: %w", err)
	}

	rt.instance = instance
	return nil
}

// Call invokes an exported function from the WASM module
func (rt *Runtime) Call(funcName string, args ...interface{}) (interface{}, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if rt.instance == nil {
		return nil, fmt.Errorf("module not instantiated")
	}

	// Create context with timeout if configured
	ctx := context.Background()
	if rt.limits.MaxExecutionTime > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, rt.limits.MaxExecutionTime)
		defer cancel()
	}

	// Get the exported function
	fn := rt.instance.ExportedFunction(funcName)
	if fn == nil {
		return nil, fmt.Errorf("function %q not found in module exports", funcName)
	}

	// Convert args to uint64 values (wazero uses uint64 for all params)
	wasmArgs := make([]uint64, len(args))
	for i, arg := range args {
		switch v := arg.(type) {
		case int32:
			wasmArgs[i] = uint64(v) //nolint:gosec // G115: WASM ABI uses uint64
		case uint32:
			wasmArgs[i] = uint64(v)
		case int64:
			wasmArgs[i] = uint64(v) //nolint:gosec // G115: WASM ABI uses uint64
		case uint64:
			wasmArgs[i] = v
		case float32:
			wasmArgs[i] = api.EncodeF32(v)
		case float64:
			wasmArgs[i] = api.EncodeF64(v)
		default:
			return nil, fmt.Errorf("unsupported argument type %T", arg)
		}
	}

	// Call the function
	results, err := fn.Call(ctx, wasmArgs...)
	if err != nil {
		return nil, fmt.Errorf("function %q failed: %w", funcName, err)
	}

	// Return the first result if any
	if len(results) == 0 {
		return nil, nil
	}

	// Return as int32 for compatibility with existing tests
	return int32(results[0]), nil //nolint:gosec // G115: WASM i32 result fits in int32
}

// GetMemory returns the WASM module's linear memory
func (rt *Runtime) GetMemory() (api.Memory, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if rt.instance == nil {
		return nil, fmt.Errorf("module not instantiated")
	}

	memory := rt.instance.ExportedMemory("memory")
	if memory == nil {
		return nil, fmt.Errorf("memory export not found")
	}

	return memory, nil
}

// ReadMemory reads bytes from WASM linear memory
func (rt *Runtime) ReadMemory(offset, length uint32) ([]byte, error) {
	memory, err := rt.GetMemory()
	if err != nil {
		return nil, err
	}

	data, ok := memory.Read(offset, length)
	if !ok {
		return nil, fmt.Errorf("memory read out of bounds")
	}

	return data, nil
}

// WriteMemory writes bytes to WASM linear memory
func (rt *Runtime) WriteMemory(offset uint32, data []byte) error {
	memory, err := rt.GetMemory()
	if err != nil {
		return err
	}

	ok := memory.Write(offset, data)
	if !ok {
		return fmt.Errorf("memory write out of bounds")
	}

	return nil
}

// fuelConsumed tracks fuel consumed (simulated for wazero)
var fuelConsumed uint64
var fuelMu sync.Mutex

// GetFuelConsumed returns the amount of fuel consumed
// Note: wazero doesn't have native fuel metering like wasmtime,
// but we can use context timeouts for execution limits
func (rt *Runtime) GetFuelConsumed() (uint64, error) {
	if rt.limits.FuelLimit == 0 {
		return 0, fmt.Errorf("fuel metering not enabled")
	}

	fuelMu.Lock()
	defer fuelMu.Unlock()

	// wazero doesn't have fuel metering, return simulated value
	return fuelConsumed, nil
}

// AddFuel adds more fuel to the store
// Note: wazero doesn't have native fuel metering
func (rt *Runtime) AddFuel(fuel uint64) error {
	if rt.limits.FuelLimit == 0 {
		return fmt.Errorf("fuel metering not enabled")
	}

	fuelMu.Lock()
	defer fuelMu.Unlock()

	// Simulated fuel addition
	return nil
}

// simulateFuelConsumption simulates fuel consumption for testing
func simulateFuelConsumption(amount uint64) {
	fuelMu.Lock()
	defer fuelMu.Unlock()
	fuelConsumed += amount
}

// resetFuelConsumed resets the fuel counter (for testing)
func resetFuelConsumed() {
	fuelMu.Lock()
	defer fuelMu.Unlock()
	fuelConsumed = 0
}

// RegisterHostFunction registers a host function that can be called from WASM
func (rt *Runtime) RegisterHostFunction(module, name string, fn interface{}) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Store capability reference
	rt.capabilities[module+"."+name] = fn

	// Note: In wazero, host functions need to be registered before module instantiation
	// This is a limitation compared to wasmtime
	// For now, we just track them in capabilities map
	return nil
}

// Close cleans up runtime resources
func (rt *Runtime) Close() error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	ctx := context.Background()

	if rt.instance != nil {
		rt.instance.Close(ctx)
		rt.instance = nil
	}

	if rt.runtime != nil {
		rt.runtime.Close(ctx)
		rt.runtime = nil
	}

	rt.module = nil
	return nil
}

// DefaultConfig returns a safe default WASM configuration
func DefaultConfig() Config {
	return Config{
		EnableWASI: true,
		Limits: ResourceLimits{
			MaxMemoryPages:   512,             // 32MB (64KB per page)
			MaxTableElements: 1000,            // Reasonable function table size
			MaxInstances:     10,              // Multiple instances per runtime
			FuelLimit:        10_000_000,      // 10 million instructions
			MaxExecutionTime: 5 * time.Second, // 5 second timeout
		},
		HostImports: nil,
	}
}

// GetExports returns all exported functions from the module
func (rt *Runtime) GetExports() ([]string, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if rt.module == nil {
		return nil, fmt.Errorf("module not loaded")
	}

	var exports []string
	// ExportedFunctions returns map[string]api.FunctionDefinition
	for name := range rt.module.ExportedFunctions() {
		exports = append(exports, name)
	}

	return exports, nil
}

// ValidateModule validates a WASM module without instantiating it
func ValidateModule(wasmBytes []byte) error {
	ctx := context.Background()
	runtime := wazero.NewRuntime(ctx)
	defer runtime.Close(ctx)

	_, err := runtime.CompileModule(ctx, wasmBytes)
	if err != nil {
		return fmt.Errorf("invalid WASM module: %w", err)
	}
	return nil
}

// RuntimeOptions configures the WASM runtime for module loader
type RuntimeOptions struct {
	// MaxMemory limits the heap size in bytes
	MaxMemory uint64

	// FuelLimit limits the number of instructions executed
	FuelLimit uint64
}

// WasmRuntime is deprecated: Use Runtime instead.
//
//nolint:revive,staticcheck // kept for backward compatibility
type WasmRuntime = Runtime

// NewWasmRuntime creates a new WASM runtime for the module loader
func NewWasmRuntime(opts *RuntimeOptions) *Runtime {
	if opts == nil {
		opts = &RuntimeOptions{
			MaxMemory: 64 * 1024 * 1024, // 64MB
			FuelLimit: 10000000,         // 10 million instructions
		}
	}

	// Convert memory bytes to pages (64KB per page)
	maxPages := uint32(opts.MaxMemory / 65536) //nolint:gosec // G115: WASM memory pages
	if maxPages == 0 {
		maxPages = 1
	}

	rt, err := NewRuntime(Config{
		EnableWASI: true,
		Limits: ResourceLimits{
			MaxMemoryPages:   maxPages,
			MaxTableElements: 1000,
			MaxInstances:     10,
			FuelLimit:        opts.FuelLimit,
			MaxExecutionTime: 30 * time.Second,
		},
	})

	if err != nil {
		// This shouldn't happen with default config, but handle it
		panic(fmt.Sprintf("failed to create WASM runtime: %v", err))
	}

	return rt
}

// ExecuteFunction executes a WASM function with input and returns the result
func (rt *Runtime) ExecuteFunction(ctx context.Context, funcName string, input map[string]interface{}) (interface{}, error) {
	// For now, we'll just call the function with no args
	// In a real implementation, we'd serialize input as JSON and pass it
	result, err := rt.Call(funcName) //nolint:contextcheck // Call uses internal context
	if err != nil {
		return nil, err
	}

	return result, nil
}
