package starlark

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"
)

func TestNewRuntime(t *testing.T) {
	rt := NewRuntime(DefaultConfig())
	if rt == nil {
		t.Fatal("expected non-nil runtime")
	}
	if !rt.Deterministic {
		t.Error("expected deterministic runtime by default")
	}
}

func TestLoadSource_SimpleScript(t *testing.T) {
	rt := NewRuntime(DefaultConfig())

	source := `
result = 2 + 2
message = "hello"
`

	globals, err := rt.LoadSource("test.star", source)
	if err != nil {
		t.Fatalf("LoadSource failed: %v", err)
	}

	// Check result
	result, ok := globals["result"]
	if !ok {
		t.Fatal("result not found in globals")
	}
	if resultInt, ok := result.(starlark.Int); ok {
		if val, _ := resultInt.Int64(); val != 4 {
			t.Errorf("expected result=4, got %d", val)
		}
	} else {
		t.Errorf("expected Int, got %T", result)
	}

	// Check message
	message, ok := globals["message"]
	if !ok {
		t.Fatal("message not found in globals")
	}
	if messageStr, ok := message.(starlark.String); ok {
		if string(messageStr) != "hello" {
			t.Errorf("expected message='hello', got %s", messageStr)
		}
	} else {
		t.Errorf("expected String, got %T", message)
	}
}

func TestLoadSource_WithFunction(t *testing.T) {
	rt := NewRuntime(DefaultConfig())

	source := `
def greet(name):
    return "Hello, " + name + "!"

result = greet("Alice")
`

	globals, err := rt.LoadSource("test.star", source)
	if err != nil {
		t.Fatalf("LoadSource failed: %v", err)
	}

	// Check result
	result, ok := globals["result"]
	if !ok {
		t.Fatal("result not found in globals")
	}
	if resultStr, ok := result.(starlark.String); ok {
		if string(resultStr) != "Hello, Alice!" {
			t.Errorf("expected 'Hello, Alice!', got %s", resultStr)
		}
	} else {
		t.Errorf("expected String, got %T", result)
	}
}

func TestCall_Function(t *testing.T) {
	rt := NewRuntime(DefaultConfig())

	source := `
def add(a, b):
    return a + b
`

	globals, err := rt.LoadSource("test.star", source)
	if err != nil {
		t.Fatalf("LoadSource failed: %v", err)
	}

	// Call the function
	result, err := rt.Call(globals, "add", starlark.MakeInt(10), starlark.MakeInt(32))
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	if resultInt, ok := result.(starlark.Int); ok {
		if val, _ := resultInt.Int64(); val != 42 {
			t.Errorf("expected 42, got %d", val)
		}
	} else {
		t.Errorf("expected Int, got %T", result)
	}
}

func TestCall_NonExistentFunction(t *testing.T) {
	rt := NewRuntime(DefaultConfig())

	source := `x = 1`
	globals, err := rt.LoadSource("test.star", source)
	if err != nil {
		t.Fatalf("LoadSource failed: %v", err)
	}

	_, err = rt.Call(globals, "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent function")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestCall_NotCallable(t *testing.T) {
	rt := NewRuntime(DefaultConfig())

	source := `x = 42`
	globals, err := rt.LoadSource("test.star", source)
	if err != nil {
		t.Fatalf("LoadSource failed: %v", err)
	}

	_, err = rt.Call(globals, "x")
	if err == nil {
		t.Error("expected error for non-callable value")
	}
	if !strings.Contains(err.Error(), "not callable") {
		t.Errorf("expected 'not callable' error, got: %v", err)
	}
}

func TestExecutionTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Limits.MaxExecutionTime = 100 * time.Millisecond
	rt := NewRuntime(cfg)

	// Infinite recursion script (will timeout)
	source := `
def infinite():
    return infinite()

infinite()
`

	_, err := rt.LoadSource("test.star", source)
	if err == nil {
		t.Error("expected timeout or stack overflow error")
	}
	// Could be timeout or stack overflow, both are acceptable
}

func TestMaxSteps(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Limits.MaxSteps = 1000 // Very low limit
	rt := NewRuntime(cfg)

	// Script that requires many steps
	source := `
total = 0
for i in range(1000):
    total += i
`

	_, err := rt.LoadSource("test.star", source)
	if err == nil {
		t.Error("expected max steps error")
	}
}

func TestRegisterCapability(t *testing.T) {
	rt := NewRuntime(DefaultConfig())

	// Register a custom capability
	rt.RegisterCapability("test_add", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var a, b int64
		if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "a", &a, "b", &b); err != nil {
			return nil, err
		}
		return starlark.MakeInt64(a + b), nil
	})

	source := `result = test_add(10, 32)`
	globals, err := rt.LoadSource("test.star", source)
	if err != nil {
		t.Fatalf("LoadSource failed: %v", err)
	}

	result, ok := globals["result"]
	if !ok {
		t.Fatal("result not found")
	}
	if resultInt, ok := result.(starlark.Int); ok {
		if val, _ := resultInt.Int64(); val != 42 {
			t.Errorf("expected 42, got %d", val)
		}
	}
}

func TestToGoValue(t *testing.T) {
	tests := []struct {
		name     string
		input    starlark.Value
		expected interface{}
	}{
		{"None", starlark.None, nil},
		{"Bool", starlark.Bool(true), true},
		{"Int", starlark.MakeInt(42), int64(42)},
		{"Float", starlark.Float(3.14), float64(3.14)},
		{"String", starlark.String("hello"), "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ToGoValue(tt.input)
			if err != nil {
				t.Fatalf("ToGoValue failed: %v", err)
			}
			if fmt.Sprintf("%v", result) != fmt.Sprintf("%v", tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestToGoValue_List(t *testing.T) {
	list := starlark.NewList([]starlark.Value{
		starlark.MakeInt(1),
		starlark.MakeInt(2),
		starlark.MakeInt(3),
	})

	result, err := ToGoValue(list)
	if err != nil {
		t.Fatalf("ToGoValue failed: %v", err)
	}

	slice, ok := result.([]interface{})
	if !ok {
		t.Fatalf("expected slice, got %T", result)
	}

	if len(slice) != 3 {
		t.Errorf("expected length 3, got %d", len(slice))
	}

	if slice[0] != int64(1) || slice[1] != int64(2) || slice[2] != int64(3) {
		t.Errorf("unexpected slice values: %v", slice)
	}
}

func TestToGoValue_Dict(t *testing.T) {
	dict := starlark.NewDict(2)
	dict.SetKey(starlark.String("name"), starlark.String("Alice"))
	dict.SetKey(starlark.String("age"), starlark.MakeInt(30))

	result, err := ToGoValue(dict)
	if err != nil {
		t.Fatalf("ToGoValue failed: %v", err)
	}

	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}

	if m["name"] != "Alice" {
		t.Errorf("expected name=Alice, got %v", m["name"])
	}
	if m["age"] != int64(30) {
		t.Errorf("expected age=30, got %v", m["age"])
	}
}

func TestFromGoValue(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string // Type name
	}{
		{"nil", nil, "NoneType"},
		{"bool", true, "bool"},
		{"int", 42, "int"},
		{"int64", int64(42), "int"},
		{"float64", 3.14, "float"},
		{"string", "hello", "string"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FromGoValue(tt.input)
			if err != nil {
				t.Fatalf("FromGoValue failed: %v", err)
			}
			if result.Type() != tt.expected {
				t.Errorf("expected type %s, got %s", tt.expected, result.Type())
			}
		})
	}
}

func TestFromGoValue_Slice(t *testing.T) {
	input := []interface{}{1, 2, 3}

	result, err := FromGoValue(input)
	if err != nil {
		t.Fatalf("FromGoValue failed: %v", err)
	}

	list, ok := result.(*starlark.List)
	if !ok {
		t.Fatalf("expected *starlark.List, got %T", result)
	}

	if list.Len() != 3 {
		t.Errorf("expected length 3, got %d", list.Len())
	}
}

func TestFromGoValue_Map(t *testing.T) {
	input := map[string]interface{}{
		"name": "Alice",
		"age":  30,
	}

	result, err := FromGoValue(input)
	if err != nil {
		t.Fatalf("FromGoValue failed: %v", err)
	}

	dict, ok := result.(*starlark.Dict)
	if !ok {
		t.Fatalf("expected *starlark.Dict, got %T", result)
	}

	if dict.Len() != 2 {
		t.Errorf("expected length 2, got %d", dict.Len())
	}
}

func TestReset(t *testing.T) {
	rt := NewRuntime(DefaultConfig())

	// Load and execute a script
	source := `x = 42`
	_, err := rt.LoadSource("test.star", source)
	if err != nil {
		t.Fatalf("LoadSource failed: %v", err)
	}

	// Reset the runtime
	rt.Reset()

	// The thread should be reset (this is mainly checking no panic occurs)
	source2 := `y = 100`
	globals, err := rt.LoadSource("test2.star", source2)
	if err != nil {
		t.Fatalf("LoadSource after reset failed: %v", err)
	}

	if _, ok := globals["y"]; !ok {
		t.Error("expected y in globals after reset")
	}
}

func TestDeterministicConfig(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Deterministic {
		t.Error("default config should be deterministic")
	}
	if cfg.Limits.MaxExecutionTime <= 0 {
		t.Error("default config should have execution timeout")
	}
	if cfg.Limits.MaxSteps <= 0 {
		t.Error("default config should have max steps limit")
	}
}
