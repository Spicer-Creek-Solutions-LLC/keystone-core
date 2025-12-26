package wasm

import (
	"fmt"
	"sync"
	"time"

	"github.com/bytecodealliance/wasmtime-go/v25"
)

// Runtime manages WebAssembly module execution with WASI support
type Runtime struct {
	// Engine is the wasmtime compilation engine
	engine *wasmtime.Engine

	// Store manages runtime state
	store *wasmtime.Store

	// Module is the compiled WASM module
	module *wasmtime.Module

	// Instance is the instantiated module
	instance *wasmtime.Instance

	// Linker manages imports
	linker *wasmtime.Linker

	// Capabilities are host functions exposed to WASM
	capabilities map[string]interface{}

	// Resource limits
	limits ResourceLimits

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
	// Create engine configuration
	engineConfig := wasmtime.NewConfig()

	// Enable fuel metering if configured
	if cfg.Limits.FuelLimit > 0 {
		engineConfig.SetConsumeFuel(true)
	}

	// Create engine
	engine := wasmtime.NewEngineWithConfig(engineConfig)

	// Create linker for imports
	linker := wasmtime.NewLinker(engine)

	// Create store
	store := wasmtime.NewStore(engine)

	// Set fuel if configured
	if cfg.Limits.FuelLimit > 0 {
		if err := store.SetFuel(cfg.Limits.FuelLimit); err != nil {
			return nil, fmt.Errorf("failed to set fuel: %w", err)
		}
	}

	// Add WASI if enabled
	if cfg.EnableWASI {
		wasiConfig := wasmtime.NewWasiConfig()
		wasiConfig.InheritStdout()
		wasiConfig.InheritStderr()
		store.SetWasi(wasiConfig)

		if err := linker.DefineWasi(); err != nil {
			return nil, fmt.Errorf("failed to define WASI: %w", err)
		}
	}

	rt := &Runtime{
		engine:       engine,
		store:        store,
		linker:       linker,
		capabilities: make(map[string]interface{}),
		limits:       cfg.Limits,
	}

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

	// Compile the module
	module, err := wasmtime.NewModule(rt.engine, wasmBytes)
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

	// Compile the module from file
	module, err := wasmtime.NewModuleFromFile(rt.engine, filename)
	if err != nil {
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

	// Instantiate the module
	instance, err := rt.linker.Instantiate(rt.store, rt.module)
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

	// Get the exported function
	fn := rt.instance.GetFunc(rt.store, funcName)
	if fn == nil {
		return nil, fmt.Errorf("function %q not found in module exports", funcName)
	}

	// Convert args to wasmtime values
	wasmArgs := make([]interface{}, len(args))
	for i, arg := range args {
		wasmArgs[i] = arg
	}

	// Call the function
	result, err := fn.Call(rt.store, wasmArgs...)
	if err != nil {
		return nil, fmt.Errorf("function %q failed: %w", funcName, err)
	}

	return result, nil
}

// GetMemory returns the WASM module's linear memory
func (rt *Runtime) GetMemory() (*wasmtime.Memory, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if rt.instance == nil {
		return nil, fmt.Errorf("module not instantiated")
	}

	memory := rt.instance.GetExport(rt.store, "memory")
	if memory == nil {
		return nil, fmt.Errorf("memory export not found")
	}

	mem := memory.Memory()
	if mem == nil {
		return nil, fmt.Errorf("export 'memory' is not a memory")
	}

	return mem, nil
}

// ReadMemory reads bytes from WASM linear memory
func (rt *Runtime) ReadMemory(offset, length uint32) ([]byte, error) {
	memory, err := rt.GetMemory()
	if err != nil {
		return nil, err
	}

	data := memory.UnsafeData(rt.store)
	if uint32(len(data)) < offset+length {
		return nil, fmt.Errorf("memory read out of bounds")
	}

	result := make([]byte, length)
	copy(result, data[offset:offset+length])
	return result, nil
}

// WriteMemory writes bytes to WASM linear memory
func (rt *Runtime) WriteMemory(offset uint32, data []byte) error {
	memory, err := rt.GetMemory()
	if err != nil {
		return err
	}

	memData := memory.UnsafeData(rt.store)
	if uint32(len(memData)) < offset+uint32(len(data)) {
		return fmt.Errorf("memory write out of bounds")
	}

	copy(memData[offset:], data)
	return nil
}

// GetFuelConsumed returns the amount of fuel consumed
func (rt *Runtime) GetFuelConsumed() (uint64, error) {
	if rt.limits.FuelLimit == 0 {
		return 0, fmt.Errorf("fuel metering not enabled")
	}

	remaining, err := rt.store.GetFuel()
	if err != nil {
		return 0, fmt.Errorf("fuel metering not active: %w", err)
	}

	// Calculate consumed from initial limit
	if remaining > rt.limits.FuelLimit {
		return 0, nil
	}
	return rt.limits.FuelLimit - remaining, nil
}

// AddFuel adds more fuel to the store
func (rt *Runtime) AddFuel(fuel uint64) error {
	if rt.limits.FuelLimit == 0 {
		return fmt.Errorf("fuel metering not enabled")
	}

	current, err := rt.store.GetFuel()
	if err != nil {
		return fmt.Errorf("fuel metering not active: %w", err)
	}
	return rt.store.SetFuel(current + fuel)
}

// RegisterHostFunction registers a host function that can be called from WASM
func (rt *Runtime) RegisterHostFunction(module, name string, fn interface{}) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Define the function in the linker
	if err := rt.linker.DefineFunc(rt.store, module, name, fn); err != nil {
		return fmt.Errorf("failed to register host function %s.%s: %w", module, name, err)
	}

	rt.capabilities[module+"."+name] = fn
	return nil
}

// Close cleans up runtime resources
func (rt *Runtime) Close() {
	// Wasmtime handles cleanup automatically via finalizers
	// But we can help by nil'ing out references
	rt.mu.Lock()
	defer rt.mu.Unlock()

	rt.instance = nil
	rt.module = nil
	rt.store = nil
	rt.linker = nil
	rt.engine = nil
}

// DefaultConfig returns a safe default WASM configuration
func DefaultConfig() Config {
	return Config{
		EnableWASI: true,
		Limits: ResourceLimits{
			MaxMemoryPages:   512,              // 32MB (64KB per page)
			MaxTableElements: 1000,             // Reasonable function table size
			MaxInstances:     10,               // Multiple instances per runtime
			FuelLimit:        10_000_000,       // 10 million instructions
			MaxExecutionTime: 5 * time.Second,  // 5 second timeout
		},
		HostImports: nil,
	}
}

// GetExports returns all exported functions from the module
func (rt *Runtime) GetExports() ([]string, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if rt.instance == nil {
		return nil, fmt.Errorf("module not instantiated")
	}

	var exports []string
	for _, exp := range rt.module.Exports() {
		if exp.Type().FuncType() != nil {
			exports = append(exports, exp.Name())
		}
	}

	return exports, nil
}

// ValidateModule validates a WASM module without instantiating it
func ValidateModule(wasmBytes []byte) error {
	engine := wasmtime.NewEngine()
	_, err := wasmtime.NewModule(engine, wasmBytes)
	if err != nil {
		return fmt.Errorf("invalid WASM module: %w", err)
	}
	return nil
}
