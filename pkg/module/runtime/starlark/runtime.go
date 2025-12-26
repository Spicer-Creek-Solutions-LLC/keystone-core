package starlark

import (
	"fmt"
	"sync"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// Runtime manages Starlark script execution with sandboxing
type Runtime struct {
	// Deterministic mode disables non-deterministic builtins (random, time)
	Deterministic bool

	// Thread contains the execution environment
	thread *starlark.Thread

	// Predeclared contains built-in symbols available to scripts
	predeclared starlark.StringDict

	// Capabilities tracks granted host capabilities
	capabilities map[string]CapabilityFunc

	// Resource limits
	limits ResourceLimits

	// Execution state
	mu sync.Mutex
}

// ResourceLimits defines execution constraints
type ResourceLimits struct {
	// MaxExecutionTime limits how long a script can run
	MaxExecutionTime time.Duration

	// MaxSteps limits the number of bytecode instructions
	// Use 0 for unlimited (not recommended in production)
	MaxSteps uint64

	// MaxStackDepth limits recursion depth
	MaxStackDepth int

	// MaxMemory limits the heap size (not enforced yet in Starlark)
	MaxMemory uint64
}

// CapabilityFunc is a function exposed to Starlark with capability grants
type CapabilityFunc func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error)

// Config contains runtime configuration
type Config struct {
	// Deterministic mode (recommended: true)
	Deterministic bool

	// Resource limits
	Limits ResourceLimits

	// Additional predeclared symbols
	Predeclared starlark.StringDict
}

// NewRuntime creates a new Starlark runtime with the given configuration
func NewRuntime(cfg Config) *Runtime {
	rt := &Runtime{
		Deterministic: cfg.Deterministic,
		predeclared:   make(starlark.StringDict),
		capabilities:  make(map[string]CapabilityFunc),
		limits:        cfg.Limits,
	}

	// Add default deterministic builtins
	rt.predeclared["struct"] = starlark.NewBuiltin("struct", starlarkstruct.Make)

	// Add user-provided predeclared symbols
	for k, v := range cfg.Predeclared {
		rt.predeclared[k] = v
	}

	// Create thread with limits
	rt.thread = &starlark.Thread{
		Name: "titan-module",
	}

	// Set max steps if configured
	if rt.limits.MaxSteps > 0 {
		rt.thread.SetMaxExecutionSteps(rt.limits.MaxSteps)
	}

	return rt
}

// LoadFile loads and executes a Starlark file
func (rt *Runtime) LoadFile(filename string) (starlark.StringDict, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Apply execution timeout if configured
	if rt.limits.MaxExecutionTime > 0 {
		timer := time.AfterFunc(rt.limits.MaxExecutionTime, func() {
			rt.thread.Cancel("execution timeout exceeded")
		})
		defer timer.Stop()
	}

	// Execute the file
	globals, err := starlark.ExecFile(rt.thread, filename, nil, rt.predeclared)
	if err != nil {
		return nil, fmt.Errorf("failed to execute %s: %w", filename, err)
	}

	return globals, nil
}

// LoadSource loads and executes Starlark source code
func (rt *Runtime) LoadSource(filename, source string) (starlark.StringDict, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Apply execution timeout if configured
	if rt.limits.MaxExecutionTime > 0 {
		timer := time.AfterFunc(rt.limits.MaxExecutionTime, func() {
			rt.thread.Cancel("execution timeout exceeded")
		})
		defer timer.Stop()
	}

	// Execute the source
	globals, err := starlark.ExecFile(rt.thread, filename, []byte(source), rt.predeclared)
	if err != nil {
		return nil, fmt.Errorf("failed to execute source: %w", err)
	}

	return globals, nil
}

// Call invokes a Starlark function by name
func (rt *Runtime) Call(globals starlark.StringDict, fnName string, args ...starlark.Value) (starlark.Value, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Look up the function
	fn, ok := globals[fnName]
	if !ok {
		return nil, fmt.Errorf("function %q not found", fnName)
	}

	// Ensure it's callable
	callable, ok := fn.(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("%q is not callable", fnName)
	}

	// Apply execution timeout if configured
	if rt.limits.MaxExecutionTime > 0 {
		timer := time.AfterFunc(rt.limits.MaxExecutionTime, func() {
			rt.thread.Cancel("execution timeout exceeded")
		})
		defer timer.Stop()
	}

	// Call the function
	result, err := starlark.Call(rt.thread, callable, starlark.Tuple(args), nil)
	if err != nil {
		return nil, fmt.Errorf("function %q failed: %w", fnName, err)
	}

	return result, nil
}

// RegisterCapability registers a host capability function
// Capabilities are the bridge between Starlark and host functionality
func (rt *Runtime) RegisterCapability(name string, fn CapabilityFunc) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	rt.capabilities[name] = fn

	// Wrap in a Starlark builtin
	builtin := starlark.NewBuiltin(name, fn)
	rt.predeclared[name] = builtin
}

// GetThread returns the Starlark thread (for advanced use)
func (rt *Runtime) GetThread() *starlark.Thread {
	return rt.thread
}

// Reset resets the runtime state
func (rt *Runtime) Reset() {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Create a new thread
	rt.thread = &starlark.Thread{
		Name: "titan-module",
	}

	if rt.limits.MaxSteps > 0 {
		rt.thread.SetMaxExecutionSteps(rt.limits.MaxSteps)
	}
}

// DefaultConfig returns a safe default configuration
func DefaultConfig() Config {
	return Config{
		Deterministic: true, // Always deterministic by default
		Limits: ResourceLimits{
			MaxExecutionTime: 5 * time.Second,
			MaxSteps:         1_000_000, // 1 million bytecode instructions
			MaxStackDepth:    1000,
			MaxMemory:        50 * 1024 * 1024, // 50MB (not enforced yet)
		},
		Predeclared: nil,
	}
}

// ToGoValue converts a Starlark value to a Go value
// Supports: None, Bool, Int, Float, String, List, Dict, Tuple
func ToGoValue(v starlark.Value) (interface{}, error) {
	switch v := v.(type) {
	case starlark.NoneType:
		return nil, nil
	case starlark.Bool:
		return bool(v), nil
	case starlark.Int:
		i, ok := v.Int64()
		if !ok {
			return nil, fmt.Errorf("integer too large")
		}
		return i, nil
	case starlark.Float:
		return float64(v), nil
	case starlark.String:
		return string(v), nil
	case *starlark.List:
		result := make([]interface{}, v.Len())
		for i := 0; i < v.Len(); i++ {
			elem, err := ToGoValue(v.Index(i))
			if err != nil {
				return nil, err
			}
			result[i] = elem
		}
		return result, nil
	case *starlark.Dict:
		result := make(map[string]interface{})
		for _, item := range v.Items() {
			key, ok := item[0].(starlark.String)
			if !ok {
				return nil, fmt.Errorf("dict keys must be strings")
			}
			val, err := ToGoValue(item[1])
			if err != nil {
				return nil, err
			}
			result[string(key)] = val
		}
		return result, nil
	case starlark.Tuple:
		result := make([]interface{}, v.Len())
		for i := 0; i < v.Len(); i++ {
			elem, err := ToGoValue(v.Index(i))
			if err != nil {
				return nil, err
			}
			result[i] = elem
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported Starlark type: %s", v.Type())
	}
}

// FromGoValue converts a Go value to a Starlark value
// Supports: nil, bool, int64, float64, string, slices, maps
func FromGoValue(v interface{}) (starlark.Value, error) {
	switch v := v.(type) {
	case nil:
		return starlark.None, nil
	case bool:
		return starlark.Bool(v), nil
	case int:
		return starlark.MakeInt(v), nil
	case int64:
		return starlark.MakeInt64(v), nil
	case float64:
		return starlark.Float(v), nil
	case string:
		return starlark.String(v), nil
	case []interface{}:
		elems := make([]starlark.Value, len(v))
		for i, elem := range v {
			val, err := FromGoValue(elem)
			if err != nil {
				return nil, err
			}
			elems[i] = val
		}
		return starlark.NewList(elems), nil
	case map[string]interface{}:
		dict := starlark.NewDict(len(v))
		for k, val := range v {
			starVal, err := FromGoValue(val)
			if err != nil {
				return nil, err
			}
			if err := dict.SetKey(starlark.String(k), starVal); err != nil {
				return nil, err
			}
		}
		return dict, nil
	default:
		return nil, fmt.Errorf("unsupported Go type: %T", v)
	}
}
