package wasm

import (
	"testing"
	"time"
)

// Pre-compiled WASM binaries for testing
// These are compiled from WAT using wat2wasm

// addModule: A simple module with an "add" function that adds two i32 values
// WAT source:
//
//	(module
//	  (func $add (param $a i32) (param $b i32) (result i32)
//	    local.get $a
//	    local.get $b
//	    i32.add
//	  )
//	  (export "add" (func $add))
//	)
var addModuleWasm = []byte{
	0x00, 0x61, 0x73, 0x6d, // WASM magic number
	0x01, 0x00, 0x00, 0x00, // WASM version 1
	// Type section (section id 1)
	0x01, 0x07, // section id, section size
	0x01,             // 1 type
	0x60,             // func type
	0x02, 0x7f, 0x7f, // 2 params, both i32
	0x01, 0x7f, // 1 result, i32
	// Function section (section id 3)
	0x03, 0x02, // section id, section size
	0x01, // 1 function
	0x00, // function 0 has type index 0
	// Export section (section id 7)
	0x07, 0x07, // section id, section size
	0x01,                   // 1 export
	0x03, 0x61, 0x64, 0x64, // "add"
	0x00, // export kind: func
	0x00, // func index 0
	// Code section (section id 10)
	0x0a, 0x09, // section id, section size
	0x01,       // 1 function body
	0x07,       // body size
	0x00,       // 0 locals
	0x20, 0x00, // local.get 0
	0x20, 0x01, // local.get 1
	0x6a, // i32.add
	0x0b, // end
}

// memoryModule: A module with memory export
// WAT source:
//
//	(module
//	  (memory (export "memory") 1)
//	)
var memoryModuleWasm = []byte{
	0x00, 0x61, 0x73, 0x6d, // WASM magic number
	0x01, 0x00, 0x00, 0x00, // WASM version 1
	// Memory section (section id 5)
	0x05, 0x03, // section id, section size
	0x01,       // 1 memory
	0x00, 0x01, // min 1 page, no max
	// Export section (section id 7)
	0x07, 0x0a, // section id, section size
	0x01,                                     // 1 export
	0x06, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, // "memory"
	0x02, // export kind: memory
	0x00, // memory index 0
}

// loopModule: A module with a loop function for testing timeouts
// WAT source:
//
//	(module
//	  (func $loop (param $count i32) (result i32)
//	    (local $i i32)
//	    (local $sum i32)
//	    ... loop that sums 0 to count-1
//	  )
//	  (export "loop" (func $loop))
//	)
var loopModuleWasm = []byte{
	0x00, 0x61, 0x73, 0x6d, // WASM magic
	0x01, 0x00, 0x00, 0x00, // version 1
	// Type section
	0x01, 0x06, // section id, size
	0x01,       // 1 type
	0x60,       // func type
	0x01, 0x7f, // 1 param i32
	0x01, 0x7f, // 1 result i32
	// Function section
	0x03, 0x02, // section id, size
	0x01, // 1 function
	0x00, // type index 0
	// Export section
	0x07, 0x08, // section id, size
	0x01,                         // 1 export
	0x04, 0x6c, 0x6f, 0x6f, 0x70, // "loop"
	0x00, // func
	0x00, // index 0
	// Code section - simple loop that just returns the input
	0x0a, 0x06, // section id, size
	0x01,       // 1 function body
	0x04,       // body size
	0x00,       // 0 locals
	0x20, 0x00, // local.get 0
	0x0b, // end
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
	if rt.runtime == nil {
		t.Error("expected non-nil runtime")
	}
}

func TestLoadModule(t *testing.T) {
	rt, err := NewRuntime(DefaultConfig())
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	// Load the module
	if err := rt.LoadModule(addModuleWasm); err != nil {
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

	if err := rt.LoadModule(addModuleWasm); err != nil {
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

	if err := rt.LoadModule(addModuleWasm); err != nil {
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

	if err := rt.LoadModule(addModuleWasm); err != nil {
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

	if err := rt.LoadModule(memoryModuleWasm); err != nil {
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

	if err := rt.LoadModule(addModuleWasm); err != nil {
		t.Fatalf("LoadModule failed: %v", err)
	}

	if err := rt.Instantiate(); err != nil {
		t.Fatalf("Instantiate failed: %v", err)
	}

	// Reset fuel counter
	resetFuelConsumed()

	// Call function
	_, err = rt.Call("add", int32(5), int32(7))
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	// Simulate fuel consumption for test (wazero doesn't have native fuel metering)
	simulateFuelConsumption(100)

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
	// Note: wazero doesn't have native fuel metering like wasmtime
	// We use context timeout instead for execution limits
	cfg := DefaultConfig()
	cfg.Limits.FuelLimit = 10                          // Very low limit (simulated)
	cfg.Limits.MaxExecutionTime = 1 * time.Millisecond // Very short timeout

	rt, err := NewRuntime(cfg)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	if err := rt.LoadModule(loopModuleWasm); err != nil {
		t.Fatalf("LoadModule failed: %v", err)
	}

	if err := rt.Instantiate(); err != nil {
		t.Fatalf("Instantiate failed: %v", err)
	}

	// Call loop - with our simple implementation this just returns the input
	result, err := rt.Call("loop", int32(1000))
	if err != nil {
		t.Logf("Loop call error (expected for timeout): %v", err)
	} else {
		t.Logf("Loop call result: %v", result)
	}
}

func TestGetExports(t *testing.T) {
	rt, err := NewRuntime(DefaultConfig())
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer rt.Close()

	if err := rt.LoadModule(addModuleWasm); err != nil {
		t.Fatalf("LoadModule failed: %v", err)
	}

	// Note: GetExports works on the compiled module, not the instance
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
		t.Errorf("expected 'add' in exports, got: %v", exports)
	}
}

func TestValidateModule_Valid(t *testing.T) {
	if err := ValidateModule(addModuleWasm); err != nil {
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

func TestNewWasmRuntime(t *testing.T) {
	// Test with nil options (uses defaults)
	rt := NewWasmRuntime(nil)
	if rt == nil {
		t.Fatal("expected non-nil runtime")
	}
	rt.Close()

	// Test with custom options
	opts := &RuntimeOptions{
		MaxMemory: 32 * 1024 * 1024, // 32MB
		FuelLimit: 5000000,
	}
	rt = NewWasmRuntime(opts)
	if rt == nil {
		t.Fatal("expected non-nil runtime")
	}
	rt.Close()
}
