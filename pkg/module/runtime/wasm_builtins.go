// Package runtime provides WASM host function bindings for capabilities
package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/shawnbutts/keystone-core/pkg/module/capabilities"
	"github.com/shawnbutts/keystone-core/pkg/module/runtime/wasm"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// createWasmCapabilityContext creates a capability context for WASM host function calls
func createWasmCapabilityContext(moduleName string) *capabilities.CapabilityContext {
	return capabilities.NewCapabilityContext(context.Background(), moduleName)
}

// WasmHostFunctions manages capability bindings for WASM modules
type WasmHostFunctions struct {
	registry *capabilities.CapabilityRegistry
}

// NewWasmHostFunctions creates a new WasmHostFunctions instance
func NewWasmHostFunctions(registry *capabilities.CapabilityRegistry) *WasmHostFunctions {
	return &WasmHostFunctions{
		registry: registry,
	}
}

// RegisterHostFunctions registers all capability host functions with a WASM runtime
// This must be called before module instantiation
func (whf *WasmHostFunctions) RegisterHostFunctions(ctx context.Context, runtime wazero.Runtime) (wazero.CompiledModule, error) {
	// Create the "kscore" host module with all capability functions
	builder := runtime.NewHostModuleBuilder("kscore")

	// Register fs.read functions
	if c, err := whf.registry.Get("fs.read"); err == nil && c != nil {
		fsCap := c.(*capabilities.FSReadCapability)
		builder.NewFunctionBuilder().
			WithFunc(whf.createFSRead(fsCap)).
			WithParameterNames("path_ptr", "path_len", "result_ptr").
			Export("fs_read")
	}

	// Register fs.write functions
	if c, err := whf.registry.Get("fs.write"); err == nil && c != nil {
		fsCap := c.(*capabilities.FSWriteCapability)
		builder.NewFunctionBuilder().
			WithFunc(whf.createFSWrite(fsCap)).
			WithParameterNames("path_ptr", "path_len", "content_ptr", "content_len").
			Export("fs_write")
	}

	// Register exec function
	if c, err := whf.registry.Get("exec"); err == nil && c != nil {
		execCap := c.(*capabilities.ExecCapability)
		builder.NewFunctionBuilder().
			WithFunc(whf.createExec(execCap)).
			WithParameterNames("cmd_ptr", "cmd_len", "args_ptr", "args_len", "result_ptr").
			Export("exec")
	}

	// Register http.get function
	if c, err := whf.registry.Get("http.get"); err == nil && c != nil {
		httpCap := c.(*capabilities.HTTPGetCapability)
		builder.NewFunctionBuilder().
			WithFunc(whf.createHTTPGet(httpCap)).
			WithParameterNames("url_ptr", "url_len", "result_ptr").
			Export("http_get")
	}

	// Register http.post function
	if c, err := whf.registry.Get("http.post"); err == nil && c != nil {
		httpCap := c.(*capabilities.HTTPPostCapability)
		builder.NewFunctionBuilder().
			WithFunc(whf.createHTTPPost(httpCap)).
			WithParameterNames("url_ptr", "url_len", "body_ptr", "body_len", "result_ptr").
			Export("http_post")
	}

	// Register log function
	if c, err := whf.registry.Get("log"); err == nil && c != nil {
		logCap := c.(*capabilities.LogCapability)
		builder.NewFunctionBuilder().
			WithFunc(whf.createLog(logCap)).
			WithParameterNames("level_ptr", "level_len", "msg_ptr", "msg_len").
			Export("log")
	}

	// Register kv functions
	if c, err := whf.registry.Get("kv"); err == nil && c != nil {
		kvCap := c.(*capabilities.KVCapability)
		builder.NewFunctionBuilder().
			WithFunc(whf.createKVGet(kvCap)).
			WithParameterNames("key_ptr", "key_len", "result_ptr").
			Export("kv_get")
		builder.NewFunctionBuilder().
			WithFunc(whf.createKVSet(kvCap)).
			WithParameterNames("key_ptr", "key_len", "value_ptr", "value_len").
			Export("kv_set")
	}

	// Register secrets functions
	if c, err := whf.registry.Get("secrets.read"); err == nil && c != nil {
		secretsCap := c.(*capabilities.SecretsReadCapability)
		builder.NewFunctionBuilder().
			WithFunc(whf.createSecretGet(secretsCap)).
			WithParameterNames("path_ptr", "path_len", "result_ptr").
			Export("secret_get")
	}

	// Register time function
	if c, err := whf.registry.Get("time"); err == nil && c != nil {
		timeCap := c.(*capabilities.TimeCapability)
		builder.NewFunctionBuilder().
			WithFunc(whf.createTimeNow(timeCap)).
			Export("time_now")
	}

	return builder.Compile(ctx)
}

// Helper to read string from WASM memory
func readString(mod api.Module, ptr, length uint32) (string, error) {
	mem := mod.Memory()
	if mem == nil {
		return "", fmt.Errorf("no memory exported")
	}
	data, ok := mem.Read(ptr, length)
	if !ok {
		return "", fmt.Errorf("memory read out of bounds")
	}
	return string(data), nil
}

// Helper to write string to WASM memory
func writeString(mod api.Module, ptr uint32, data string) error {
	mem := mod.Memory()
	if mem == nil {
		return fmt.Errorf("no memory exported")
	}
	if !mem.Write(ptr, []byte(data)) {
		return fmt.Errorf("memory write out of bounds")
	}
	return nil
}

// Helper to write result to WASM memory as JSON
func writeResult(mod api.Module, ptr uint32, result interface{}) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return writeString(mod, ptr, string(data))
}

// ---- Filesystem Host Functions ----

func (whf *WasmHostFunctions) createFSRead(c *capabilities.FSReadCapability) func(context.Context, api.Module, uint32, uint32, uint32) uint32 {
	return func(ctx context.Context, mod api.Module, pathPtr, pathLen, resultPtr uint32) uint32 {
		path, err := readString(mod, pathPtr, pathLen)
		if err != nil {
			return 1
		}

		capCtx := createWasmCapabilityContext("") //nolint:contextcheck // WASM capability context created inline
		data, err := c.ReadFile(capCtx, path)
		if err != nil {
			if wErr := writeResult(mod, resultPtr, map[string]interface{}{"error": err.Error()}); wErr != nil {
				return 1
			}
			return 1
		}

		if wErr := writeResult(mod, resultPtr, map[string]interface{}{"data": string(data)}); wErr != nil {
			return 1
		}
		return 0
	}
}

func (whf *WasmHostFunctions) createFSWrite(c *capabilities.FSWriteCapability) func(context.Context, api.Module, uint32, uint32, uint32, uint32) uint32 {
	return func(ctx context.Context, mod api.Module, pathPtr, pathLen, contentPtr, contentLen uint32) uint32 {
		path, err := readString(mod, pathPtr, pathLen)
		if err != nil {
			return 1
		}

		content, err := readString(mod, contentPtr, contentLen)
		if err != nil {
			return 1
		}

		capCtx := createWasmCapabilityContext("") //nolint:contextcheck // WASM capability context created inline
		if err := c.WriteFile(capCtx, path, []byte(content), os.FileMode(0o644)); err != nil {
			return 1
		}

		return 0
	}
}

// ---- Exec Host Function ----

func (whf *WasmHostFunctions) createExec(c *capabilities.ExecCapability) func(context.Context, api.Module, uint32, uint32, uint32, uint32, uint32) uint32 {
	return func(ctx context.Context, mod api.Module, cmdPtr, cmdLen, argsPtr, argsLen, resultPtr uint32) uint32 {
		cmd, err := readString(mod, cmdPtr, cmdLen)
		if err != nil {
			return 1
		}

		var args []string
		if argsLen > 0 {
			argsJSON, err := readString(mod, argsPtr, argsLen)
			if err != nil {
				return 1
			}
			if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
				return 1
			}
		}

		capCtx := createWasmCapabilityContext("") //nolint:contextcheck // WASM capability context created inline
		result, err := c.Exec(capCtx, cmd, args...)        //nolint:contextcheck // Exec uses capability context
		if err != nil {
			if wErr := writeResult(mod, resultPtr, map[string]interface{}{"error": err.Error()}); wErr != nil {
				return 1
			}
			return 1
		}

		if wErr := writeResult(mod, resultPtr, map[string]interface{}{
			"stdout":    result.Stdout,
			"stderr":    result.Stderr,
			"exit_code": result.ExitCode,
		}); wErr != nil {
			return 1
		}
		return 0
	}
}

// ---- HTTP Host Functions ----

func (whf *WasmHostFunctions) createHTTPGet(c *capabilities.HTTPGetCapability) func(context.Context, api.Module, uint32, uint32, uint32) uint32 {
	return func(ctx context.Context, mod api.Module, urlPtr, urlLen, resultPtr uint32) uint32 {
		url, err := readString(mod, urlPtr, urlLen)
		if err != nil {
			return 1
		}

		capCtx := createWasmCapabilityContext("") //nolint:contextcheck // WASM capability context created inline
		resp, err := c.Get(capCtx, url, nil)
		if err != nil {
			if wErr := writeResult(mod, resultPtr, map[string]interface{}{"error": err.Error()}); wErr != nil {
				return 1
			}
			return 1
		}

		// Convert http.Header to simple map for JSON serialization
		headers := make(map[string]string)
		for k, vals := range resp.Headers {
			if len(vals) > 0 {
				headers[k] = vals[0]
			}
		}

		if wErr := writeResult(mod, resultPtr, map[string]interface{}{
			"status_code": resp.StatusCode,
			"body":        string(resp.Body),
			"headers":     headers,
		}); wErr != nil {
			return 1
		}
		return 0
	}
}

func (whf *WasmHostFunctions) createHTTPPost(c *capabilities.HTTPPostCapability) func(context.Context, api.Module, uint32, uint32, uint32, uint32, uint32) uint32 {
	return func(ctx context.Context, mod api.Module, urlPtr, urlLen, bodyPtr, bodyLen, resultPtr uint32) uint32 {
		url, err := readString(mod, urlPtr, urlLen)
		if err != nil {
			return 1
		}

		body, err := readString(mod, bodyPtr, bodyLen)
		if err != nil {
			return 1
		}

		capCtx := createWasmCapabilityContext("") //nolint:contextcheck // WASM capability context created inline
		resp, err := c.Post(capCtx, url, []byte(body), map[string]string{"Content-Type": "application/json"})
		if err != nil {
			if wErr := writeResult(mod, resultPtr, map[string]interface{}{"error": err.Error()}); wErr != nil {
				return 1
			}
			return 1
		}

		// Convert http.Header to simple map for JSON serialization
		headers := make(map[string]string)
		for k, vals := range resp.Headers {
			if len(vals) > 0 {
				headers[k] = vals[0]
			}
		}

		if wErr := writeResult(mod, resultPtr, map[string]interface{}{
			"status_code": resp.StatusCode,
			"body":        string(resp.Body),
			"headers":     headers,
		}); wErr != nil {
			return 1
		}
		return 0
	}
}

// ---- Log Host Function ----

func (whf *WasmHostFunctions) createLog(c *capabilities.LogCapability) func(context.Context, api.Module, uint32, uint32, uint32, uint32) {
	return func(ctx context.Context, mod api.Module, levelPtr, levelLen, msgPtr, msgLen uint32) {
		level, err := readString(mod, levelPtr, levelLen)
		if err != nil {
			return
		}

		msg, err := readString(mod, msgPtr, msgLen)
		if err != nil {
			return
		}

		capCtx := createWasmCapabilityContext("") //nolint:contextcheck // WASM capability context created inline
		_ = c.Log(capCtx, level, msg, nil) //nolint:errcheck // best-effort logging
	}
}

// ---- KV Host Functions ----

func (whf *WasmHostFunctions) createKVGet(c *capabilities.KVCapability) func(context.Context, api.Module, uint32, uint32, uint32) uint32 {
	return func(ctx context.Context, mod api.Module, keyPtr, keyLen, resultPtr uint32) uint32 {
		key, err := readString(mod, keyPtr, keyLen)
		if err != nil {
			return 1
		}

		capCtx := createWasmCapabilityContext("") //nolint:contextcheck // WASM capability context created inline
		value, err := c.Get(capCtx, key)
		if err != nil {
			if wErr := writeResult(mod, resultPtr, map[string]interface{}{"error": err.Error()}); wErr != nil {
				return 1
			}
			return 1
		}

		if wErr := writeResult(mod, resultPtr, map[string]interface{}{
			"value":  value,
			"exists": value != "",
		}); wErr != nil {
			return 1
		}
		return 0
	}
}

func (whf *WasmHostFunctions) createKVSet(c *capabilities.KVCapability) func(context.Context, api.Module, uint32, uint32, uint32, uint32) uint32 {
	return func(ctx context.Context, mod api.Module, keyPtr, keyLen, valuePtr, valueLen uint32) uint32 {
		key, err := readString(mod, keyPtr, keyLen)
		if err != nil {
			return 1
		}

		value, err := readString(mod, valuePtr, valueLen)
		if err != nil {
			return 1
		}

		capCtx := createWasmCapabilityContext("") //nolint:contextcheck // WASM capability context created inline
		if err := c.Set(capCtx, key, value); err != nil {
			return 1
		}

		return 0
	}
}

// ---- Secrets Host Function ----

func (whf *WasmHostFunctions) createSecretGet(c *capabilities.SecretsReadCapability) func(context.Context, api.Module, uint32, uint32, uint32) uint32 {
	return func(ctx context.Context, mod api.Module, pathPtr, pathLen, resultPtr uint32) uint32 {
		path, err := readString(mod, pathPtr, pathLen)
		if err != nil {
			return 1
		}

		capCtx := createWasmCapabilityContext("") //nolint:contextcheck // WASM capability context created inline
		value, err := c.ReadSecret(capCtx, path)
		if err != nil {
			if wErr := writeResult(mod, resultPtr, map[string]interface{}{"error": err.Error()}); wErr != nil {
				return 1
			}
			return 1
		}

		if wErr := writeResult(mod, resultPtr, map[string]interface{}{"value": value}); wErr != nil {
			return 1
		}
		return 0
	}
}

// ---- Time Host Function ----

func (whf *WasmHostFunctions) createTimeNow(c *capabilities.TimeCapability) func(context.Context, api.Module) uint64 {
	return func(ctx context.Context, mod api.Module) uint64 {
		capCtx := createWasmCapabilityContext("") //nolint:contextcheck // WASM capability context created inline
		return uint64(c.Now(capCtx).Unix())       //nolint:gosec // G115: Unix timestamp fits in uint64
	}
}

// RegisterWithWasmRuntime is a convenience method that registers host functions with an existing wasm.Runtime
// Note: This is a simplified version - in practice, host functions need to be registered before module instantiation
// Uses safe type assertions to handle stub capabilities gracefully
func (whf *WasmHostFunctions) RegisterWithWasmRuntime(rt *wasm.Runtime) error {
	var errs []error

	registerFn := func(module, name string, fn interface{}) {
		if err := rt.RegisterHostFunction(module, name, fn); err != nil {
			errs = append(errs, fmt.Errorf("register %s.%s: %w", module, name, err))
		}
	}

	if c, err := whf.registry.Get("fs.read"); err == nil && c != nil {
		if fsCap, ok := c.(*capabilities.FSReadCapability); ok {
			registerFn("kscore", "fs_read", whf.createFSRead(fsCap))
		}
	}
	if c, err := whf.registry.Get("fs.write"); err == nil && c != nil {
		if fsCap, ok := c.(*capabilities.FSWriteCapability); ok {
			registerFn("kscore", "fs_write", whf.createFSWrite(fsCap))
		}
	}
	if c, err := whf.registry.Get("exec"); err == nil && c != nil {
		if execCap, ok := c.(*capabilities.ExecCapability); ok {
			registerFn("kscore", "exec", whf.createExec(execCap))
		}
	}
	if c, err := whf.registry.Get("http.get"); err == nil && c != nil {
		if httpCap, ok := c.(*capabilities.HTTPGetCapability); ok {
			registerFn("kscore", "http_get", whf.createHTTPGet(httpCap))
		}
	}
	if c, err := whf.registry.Get("http.post"); err == nil && c != nil {
		if httpCap, ok := c.(*capabilities.HTTPPostCapability); ok {
			registerFn("kscore", "http_post", whf.createHTTPPost(httpCap))
		}
	}
	if c, err := whf.registry.Get("log"); err == nil && c != nil {
		if logCap, ok := c.(*capabilities.LogCapability); ok {
			registerFn("kscore", "log", whf.createLog(logCap))
		}
	}
	if c, err := whf.registry.Get("kv"); err == nil && c != nil {
		if kvCap, ok := c.(*capabilities.KVCapability); ok {
			registerFn("kscore", "kv_get", whf.createKVGet(kvCap))
			registerFn("kscore", "kv_set", whf.createKVSet(kvCap))
		}
	}
	if c, err := whf.registry.Get("secrets.read"); err == nil && c != nil {
		if secretsCap, ok := c.(*capabilities.SecretsReadCapability); ok {
			registerFn("kscore", "secret_get", whf.createSecretGet(secretsCap))
		}
	}
	if c, err := whf.registry.Get("time"); err == nil && c != nil {
		if timeCap, ok := c.(*capabilities.TimeCapability); ok {
			registerFn("kscore", "time_now", whf.createTimeNow(timeCap))
		}
	}

	return errors.Join(errs...)
}
