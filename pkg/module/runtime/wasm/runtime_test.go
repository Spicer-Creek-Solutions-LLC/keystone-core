package wasm

import (
	"testing"

	"github.com/bytecodealliance/wasmtime-go/v25"
)

// Simple WASM module in WAT (WebAssembly Text Format)
// This module exports an "add" function that adds two i32 integers
const addModuleWAT = `
(module
  (func $add (param $a i32) (param $b i32) (result i32)
    local.get $a
    local.get $b
    i32.add
  )
  (export "add" (func $add))
)
`

// WASM module with memory export
const memoryModuleWAT = `
(module
  (memory (export "memory") 1)
  (func $get_value (param $offset i32) (result i32)
    local.get $offset
    i32.load
  )
  (export "get_value" (func $get_value))
)
`

// WASM module that loops (for fuel testing)
const loopModuleWAT = `
(module
  (func $loop (param $count i32) (result i32)
    (local $i i32)
    (local $sum i32)
    (local.set $i (i32.const 0))
    (local.set $sum (i32.const 0))
    (block $break
      (loop $continue
        (br_if $break (i32.ge_u (local.get $i) (local.get $count)))
        (local.set $sum (i32.add (local.get $sum) (local.get $i)))
        (local.set $i (i32.add (local.get $i) (i32.const 1)))
        (br $continue)
      )
    )
    (local.get $sum)
  )
  (export "loop" (func $loop))
)
`

// Helper to compile WAT to WASM
func watToWasm(wat string) ([]byte, error) {
	return wasmtime.Wat2Wasm(wat)
}

func TestNewRuntime(t *testing.T) {
	rt, err := NewRuntime(DefaultConfig())
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	if rt == nil {
		t.Fatal("expected non-nil runtime")
	}
	if rt.engine == nil {
		t.Error("expected non-nil engine")
	}
	if rt.store == nil {
		t.Error("expected non-nil store")
	}
}

func TestLoadModule(t *testing.T) {
	rt, err := NewRuntime(DefaultConfig())
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	// Compile WAT to WASM
	wasmBytes, err := watToWasm(addModuleWAT)
	if err != nil {
		t.Fatalf("Failed to compile WAT: %v", err)
	}

	// Load the module
	if err := rt.LoadModule(wasmBytes); err != nil {
		t.Fatalf("LoadModule failed: %v", err)
	}

	if rt.module == nil {
		t.Error("expected module to be loaded")
	}
}

func TestInstantiate(t *testing.T) {
	rt, err := NewRuntime(DefaultConfig())
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	wasmBytes, err := watToWasm(addModuleWAT)
	if err != nil {
		t.Fatalf("Failed to compile WAT: %v", err)
	}

	if err := rt.LoadModule(wasmBytes); err != nil {
		t.Fatalf("LoadModule failed: %v", err)
	}

	if err := rt.Instantiate(); err != nil {
		t.Fatalf("Instantiate failed: %v", err)
	}

	if rt.instance == nil {
		t.Error("expected instance to be created")
	}
}

func TestCall_AddFunction(t *testing.T) {
	rt, err := NewRuntime(DefaultConfig())
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	wasmBytes, err := watToWasm(addModuleWAT)
	if err != nil {
		t.Fatalf("Failed to compile WAT: %v", err)
	}

	if err := rt.LoadModule(wasmBytes); err != nil {
		t.Fatalf("LoadModule failed: %v", err)
	}

	if err := rt.Instantiate(); err != nil {
		t.Fatalf("Instantiate failed: %v", err)
	}

	// Call the add function
	result, err := rt.Call("add", int32(10), int32(32))
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	if result.(int32) != 42 {
		t.Errorf("expected 42, got %v", result)
	}
}

func TestCall_NonExistentFunction(t *testing.T) {
	rt, err := NewRuntime(DefaultConfig())
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	wasmBytes, err := watToWasm(addModuleWAT)
	if err != nil {
		t.Fatalf("Failed to compile WAT: %v", err)
	}

	if err := rt.LoadModule(wasmBytes); err != nil {
		t.Fatalf("LoadModule failed: %v", err)
	}

	if err := rt.Instantiate(); err != nil {
		t.Fatalf("Instantiate failed: %v", err)
	}

	_, err = rt.Call("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent function")
	}
}

func TestMemoryOperations(t *testing.T) {
	rt, err := NewRuntime(DefaultConfig())
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	wasmBytes, err := watToWasm(memoryModuleWAT)
	if err != nil {
		t.Fatalf("Failed to compile WAT: %v", err)
	}

	if err := rt.LoadModule(wasmBytes); err != nil {
		t.Fatalf("LoadModule failed: %v", err)
	}

	if err := rt.Instantiate(); err != nil {
		t.Fatalf("Instantiate failed: %v", err)
	}

	// Write data to memory
	testData := []byte{1, 2, 3, 4}
	if err := rt.WriteMemory(0, testData); err != nil {
		t.Fatalf("WriteMemory failed: %v", err)
	}

	// Read data back
	readData, err := rt.ReadMemory(0, 4)
	if err != nil {
		t.Fatalf("ReadMemory failed: %v", err)
	}

	for i, b := range testData {
		if readData[i] != b {
			t.Errorf("byte %d: expected %d, got %d", i, b, readData[i])
		}
	}
}

func TestFuelMeteringEnabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Limits.FuelLimit = 100000

	rt, err := NewRuntime(cfg)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	wasmBytes, err := watToWasm(addModuleWAT)
	if err != nil {
		t.Fatalf("Failed to compile WAT: %v", err)
	}

	if err := rt.LoadModule(wasmBytes); err != nil {
		t.Fatalf("LoadModule failed: %v", err)
	}

	if err := rt.Instantiate(); err != nil {
		t.Fatalf("Instantiate failed: %v", err)
	}

	// Call function
	_, err = rt.Call("add", int32(5), int32(7))
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	// Check fuel consumed
	consumed, err := rt.GetFuelConsumed()
	if err != nil {
		t.Fatalf("GetFuelConsumed failed: %v", err)
	}

	if consumed == 0 {
		t.Error("expected fuel to be consumed")
	}

	t.Logf("Fuel consumed: %d", consumed)
}

func TestFuelExhaustion(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Limits.FuelLimit = 10 // Very low limit

	rt, err := NewRuntime(cfg)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	wasmBytes, err := watToWasm(loopModuleWAT)
	if err != nil {
		t.Fatalf("Failed to compile WAT: %v", err)
	}

	if err := rt.LoadModule(wasmBytes); err != nil {
		t.Fatalf("LoadModule failed: %v", err)
	}

	if err := rt.Instantiate(); err != nil {
		t.Fatalf("Instantiate failed: %v", err)
	}

	// Try to call loop with high count (should run out of fuel)
	_, err = rt.Call("loop", int32(1000))
	if err == nil {
		t.Error("expected fuel exhaustion error")
	}
}

func TestGetExports(t *testing.T) {
	rt, err := NewRuntime(DefaultConfig())
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	wasmBytes, err := watToWasm(addModuleWAT)
	if err != nil {
		t.Fatalf("Failed to compile WAT: %v", err)
	}

	if err := rt.LoadModule(wasmBytes); err != nil {
		t.Fatalf("LoadModule failed: %v", err)
	}

	if err := rt.Instantiate(); err != nil {
		t.Fatalf("Instantiate failed: %v", err)
	}

	exports, err := rt.GetExports()
	if err != nil {
		t.Fatalf("GetExports failed: %v", err)
	}

	if len(exports) == 0 {
		t.Error("expected at least one export")
	}

	// Should have "add" function
	found := false
	for _, name := range exports {
		if name == "add" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'add' in exports")
	}
}

func TestValidateModule_Valid(t *testing.T) {
	wasmBytes, err := watToWasm(addModuleWAT)
	if err != nil {
		t.Fatalf("Failed to compile WAT: %v", err)
	}

	if err := ValidateModule(wasmBytes); err != nil {
		t.Errorf("ValidateModule failed for valid module: %v", err)
	}
}

func TestValidateModule_Invalid(t *testing.T) {
	invalidWASM := []byte{0x00, 0x01, 0x02, 0x03} // Not valid WASM

	if err := ValidateModule(invalidWASM); err == nil {
		t.Error("expected validation error for invalid WASM")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.EnableWASI {
		t.Error("expected WASI to be enabled by default")
	}
	if cfg.Limits.MaxMemoryPages == 0 {
		t.Error("expected non-zero max memory pages")
	}
	if cfg.Limits.FuelLimit == 0 {
		t.Error("expected non-zero fuel limit")
	}
	if cfg.Limits.MaxExecutionTime == 0 {
		t.Error("expected non-zero execution timeout")
	}
}

func TestRegisterHostFunction(t *testing.T) {
	rt, err := NewRuntime(DefaultConfig())
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	// Register a host function
	err = rt.RegisterHostFunction("env", "test_multiply", func(a, b int32) int32 {
		return a * b
	})
	if err != nil {
		t.Fatalf("RegisterHostFunction failed: %v", err)
	}

	// Verify it was registered
	if _, ok := rt.capabilities["env.test_multiply"]; !ok {
		t.Error("expected capability to be registered")
	}
}

func TestClose(t *testing.T) {
	rt, err := NewRuntime(DefaultConfig())
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}

	// Close should not panic
	rt.Close()

	// Verify cleanup
	if rt.instance != nil {
		t.Error("expected instance to be nil after close")
	}
	if rt.module != nil {
		t.Error("expected module to be nil after close")
	}
}
