// Package runtime provides WASM host function bindings for capabilities
package runtime

import (
	"context"
	"encoding/json"
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
	if cap, err := whf.registry.Get("fs.read"); err == nil && cap != nil {
		fsCap := cap.(*capabilities.FSReadCapability)
		builder.NewFunctionBuilder().
			WithFunc(whf.createFSRead(fsCap)).
			WithParameterNames("path_ptr", "path_len", "result_ptr").
			Export("fs_read")
	}

	// Register fs.write functions
	if cap, err := whf.registry.Get("fs.write"); err == nil && cap != nil {
		fsCap := cap.(*capabilities.FSWriteCapability)
		builder.NewFunctionBuilder().
			WithFunc(whf.createFSWrite(fsCap)).
			WithParameterNames("path_ptr", "path_len", "content_ptr", "content_len").
			Export("fs_write")
	}

	// Register exec function
	if cap, err := whf.registry.Get("exec"); err == nil && cap != nil {
		execCap := cap.(*capabilities.ExecCapability)
		builder.NewFunctionBuilder().
			WithFunc(whf.createExec(execCap)).
			WithParameterNames("cmd_ptr", "cmd_len", "args_ptr", "args_len", "result_ptr").
			Export("exec")
	}

	// Register http.get function
	if cap, err := whf.registry.Get("http.get"); err == nil && cap != nil {
		httpCap := cap.(*capabilities.HTTPGetCapability)
		builder.NewFunctionBuilder().
			WithFunc(whf.createHTTPGet(httpCap)).
			WithParameterNames("url_ptr", "url_len", "result_ptr").
			Export("http_get")
	}

	// Register http.post function
	if cap, err := whf.registry.Get("http.post"); err == nil && cap != nil {
		httpCap := cap.(*capabilities.HTTPPostCapability)
		builder.NewFunctionBuilder().
			WithFunc(whf.createHTTPPost(httpCap)).
			WithParameterNames("url_ptr", "url_len", "body_ptr", "body_len", "result_ptr").
			Export("http_post")
	}

	// Register log function
	if cap, err := whf.registry.Get("log"); err == nil && cap != nil {
		logCap := cap.(*capabilities.LogCapability)
		builder.NewFunctionBuilder().
			WithFunc(whf.createLog(logCap)).
			WithParameterNames("level_ptr", "level_len", "msg_ptr", "msg_len").
			Export("log")
	}

	// Register kv functions
	if cap, err := whf.registry.Get("kv"); err == nil && cap != nil {
		kvCap := cap.(*capabilities.KVCapability)
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
	if cap, err := whf.registry.Get("secrets.read"); err == nil && cap != nil {
		secretsCap := cap.(*capabilities.SecretsReadCapability)
		builder.NewFunctionBuilder().
			WithFunc(whf.createSecretGet(secretsCap)).
			WithParameterNames("path_ptr", "path_len", "result_ptr").
			Export("secret_get")
	}

	// Register time function
	if cap, err := whf.registry.Get("time"); err == nil && cap != nil {
		timeCap := cap.(*capabilities.TimeCapability)
		builder.NewFunctionBuilder().
			WithFunc(whf.createTimeNow(timeCap)).
			Export("time_now")
	}

	return builder.Compile(ctx)
}

// Helper to read string from WASM memory
func readString(mod api.Module, ptr, len uint32) (string, error) {
	mem := mod.Memory()
	if mem == nil {
		return "", fmt.Errorf("no memory exported")
	}
	data, ok := mem.Read(ptr, len)
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

func (whf *WasmHostFunctions) createFSRead(cap *capabilities.FSReadCapability) func(context.Context, api.Module, uint32, uint32, uint32) uint32 {
	return func(ctx context.Context, mod api.Module, pathPtr, pathLen, resultPtr uint32) uint32 {
		path, err := readString(mod, pathPtr, pathLen)
		if err != nil {
			return 1
		}

		capCtx := createWasmCapabilityContext("")
		data, err := cap.ReadFile(capCtx, path)
		if err != nil {
			writeResult(mod, resultPtr, map[string]interface{}{"error": err.Error()})
			return 1
		}

		writeResult(mod, resultPtr, map[string]interface{}{"data": string(data)})
		return 0
	}
}

func (whf *WasmHostFunctions) createFSWrite(cap *capabilities.FSWriteCapability) func(context.Context, api.Module, uint32, uint32, uint32, uint32) uint32 {
	return func(ctx context.Context, mod api.Module, pathPtr, pathLen, contentPtr, contentLen uint32) uint32 {
		path, err := readString(mod, pathPtr, pathLen)
		if err != nil {
			return 1
		}

		content, err := readString(mod, contentPtr, contentLen)
		if err != nil {
			return 1
		}

		capCtx := createWasmCapabilityContext("")
		if err := cap.WriteFile(capCtx, path, []byte(content), os.FileMode(0644)); err != nil {
			return 1
		}

		return 0
	}
}

// ---- Exec Host Function ----

func (whf *WasmHostFunctions) createExec(cap *capabilities.ExecCapability) func(context.Context, api.Module, uint32, uint32, uint32, uint32, uint32) uint32 {
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

		capCtx := createWasmCapabilityContext("")
		result, err := cap.Exec(capCtx, cmd, args...)
		if err != nil {
			writeResult(mod, resultPtr, map[string]interface{}{"error": err.Error()})
			return 1
		}

		writeResult(mod, resultPtr, map[string]interface{}{
			"stdout":    result.Stdout,
			"stderr":    result.Stderr,
			"exit_code": result.ExitCode,
		})
		return 0
	}
}

// ---- HTTP Host Functions ----

func (whf *WasmHostFunctions) createHTTPGet(cap *capabilities.HTTPGetCapability) func(context.Context, api.Module, uint32, uint32, uint32) uint32 {
	return func(ctx context.Context, mod api.Module, urlPtr, urlLen, resultPtr uint32) uint32 {
		url, err := readString(mod, urlPtr, urlLen)
		if err != nil {
			return 1
		}

		capCtx := createWasmCapabilityContext("")
		resp, err := cap.Get(capCtx, url, nil)
		if err != nil {
			writeResult(mod, resultPtr, map[string]interface{}{"error": err.Error()})
			return 1
		}

		// Convert http.Header to simple map for JSON serialization
		headers := make(map[string]string)
		for k, vals := range resp.Headers {
			if len(vals) > 0 {
				headers[k] = vals[0]
			}
		}

		writeResult(mod, resultPtr, map[string]interface{}{
			"status_code": resp.StatusCode,
			"body":        string(resp.Body),
			"headers":     headers,
		})
		return 0
	}
}

func (whf *WasmHostFunctions) createHTTPPost(cap *capabilities.HTTPPostCapability) func(context.Context, api.Module, uint32, uint32, uint32, uint32, uint32) uint32 {
	return func(ctx context.Context, mod api.Module, urlPtr, urlLen, bodyPtr, bodyLen, resultPtr uint32) uint32 {
		url, err := readString(mod, urlPtr, urlLen)
		if err != nil {
			return 1
		}

		body, err := readString(mod, bodyPtr, bodyLen)
		if err != nil {
			return 1
		}

		capCtx := createWasmCapabilityContext("")
		resp, err := cap.Post(capCtx, url, []byte(body), map[string]string{"Content-Type": "application/json"})
		if err != nil {
			writeResult(mod, resultPtr, map[string]interface{}{"error": err.Error()})
			return 1
		}

		// Convert http.Header to simple map for JSON serialization
		headers := make(map[string]string)
		for k, vals := range resp.Headers {
			if len(vals) > 0 {
				headers[k] = vals[0]
			}
		}

		writeResult(mod, resultPtr, map[string]interface{}{
			"status_code": resp.StatusCode,
			"body":        string(resp.Body),
			"headers":     headers,
		})
		return 0
	}
}

// ---- Log Host Function ----

func (whf *WasmHostFunctions) createLog(cap *capabilities.LogCapability) func(context.Context, api.Module, uint32, uint32, uint32, uint32) {
	return func(ctx context.Context, mod api.Module, levelPtr, levelLen, msgPtr, msgLen uint32) {
		level, err := readString(mod, levelPtr, levelLen)
		if err != nil {
			return
		}

		msg, err := readString(mod, msgPtr, msgLen)
		if err != nil {
			return
		}

		capCtx := createWasmCapabilityContext("")
		cap.Log(capCtx, level, msg, nil)
	}
}

// ---- KV Host Functions ----

func (whf *WasmHostFunctions) createKVGet(cap *capabilities.KVCapability) func(context.Context, api.Module, uint32, uint32, uint32) uint32 {
	return func(ctx context.Context, mod api.Module, keyPtr, keyLen, resultPtr uint32) uint32 {
		key, err := readString(mod, keyPtr, keyLen)
		if err != nil {
			return 1
		}

		capCtx := createWasmCapabilityContext("")
		value, err := cap.Get(capCtx, key)
		if err != nil {
			writeResult(mod, resultPtr, map[string]interface{}{"error": err.Error()})
			return 1
		}

		writeResult(mod, resultPtr, map[string]interface{}{
			"value":  value,
			"exists": value != "",
		})
		return 0
	}
}

func (whf *WasmHostFunctions) createKVSet(cap *capabilities.KVCapability) func(context.Context, api.Module, uint32, uint32, uint32, uint32) uint32 {
	return func(ctx context.Context, mod api.Module, keyPtr, keyLen, valuePtr, valueLen uint32) uint32 {
		key, err := readString(mod, keyPtr, keyLen)
		if err != nil {
			return 1
		}

		value, err := readString(mod, valuePtr, valueLen)
		if err != nil {
			return 1
		}

		capCtx := createWasmCapabilityContext("")
		if err := cap.Set(capCtx, key, value); err != nil {
			return 1
		}

		return 0
	}
}

// ---- Secrets Host Function ----

func (whf *WasmHostFunctions) createSecretGet(cap *capabilities.SecretsReadCapability) func(context.Context, api.Module, uint32, uint32, uint32) uint32 {
	return func(ctx context.Context, mod api.Module, pathPtr, pathLen, resultPtr uint32) uint32 {
		path, err := readString(mod, pathPtr, pathLen)
		if err != nil {
			return 1
		}

		capCtx := createWasmCapabilityContext("")
		value, err := cap.ReadSecret(capCtx, path)
		if err != nil {
			writeResult(mod, resultPtr, map[string]interface{}{"error": err.Error()})
			return 1
		}

		writeResult(mod, resultPtr, map[string]interface{}{"value": value})
		return 0
	}
}

// ---- Time Host Function ----

func (whf *WasmHostFunctions) createTimeNow(cap *capabilities.TimeCapability) func(context.Context, api.Module) uint64 {
	return func(ctx context.Context, mod api.Module) uint64 {
		capCtx := createWasmCapabilityContext("")
		return uint64(cap.Now(capCtx).Unix())
	}
}

// RegisterWithWasmRuntime is a convenience method that registers host functions with an existing wasm.Runtime
// Note: This is a simplified version - in practice, host functions need to be registered before module instantiation
// Uses safe type assertions to handle stub capabilities gracefully
func (whf *WasmHostFunctions) RegisterWithWasmRuntime(rt *wasm.Runtime) error {
	// Register each capability as a host function
	if cap, err := whf.registry.Get("fs.read"); err == nil && cap != nil {
		if fsCap, ok := cap.(*capabilities.FSReadCapability); ok {
			rt.RegisterHostFunction("kscore", "fs_read", whf.createFSRead(fsCap))
		}
	}
	if cap, err := whf.registry.Get("fs.write"); err == nil && cap != nil {
		if fsCap, ok := cap.(*capabilities.FSWriteCapability); ok {
			rt.RegisterHostFunction("kscore", "fs_write", whf.createFSWrite(fsCap))
		}
	}
	if cap, err := whf.registry.Get("exec"); err == nil && cap != nil {
		if execCap, ok := cap.(*capabilities.ExecCapability); ok {
			rt.RegisterHostFunction("kscore", "exec", whf.createExec(execCap))
		}
	}
	if cap, err := whf.registry.Get("http.get"); err == nil && cap != nil {
		if httpCap, ok := cap.(*capabilities.HTTPGetCapability); ok {
			rt.RegisterHostFunction("kscore", "http_get", whf.createHTTPGet(httpCap))
		}
	}
	if cap, err := whf.registry.Get("http.post"); err == nil && cap != nil {
		if httpCap, ok := cap.(*capabilities.HTTPPostCapability); ok {
			rt.RegisterHostFunction("kscore", "http_post", whf.createHTTPPost(httpCap))
		}
	}
	if cap, err := whf.registry.Get("log"); err == nil && cap != nil {
		if logCap, ok := cap.(*capabilities.LogCapability); ok {
			rt.RegisterHostFunction("kscore", "log", whf.createLog(logCap))
		}
	}
	if cap, err := whf.registry.Get("kv"); err == nil && cap != nil {
		if kvCap, ok := cap.(*capabilities.KVCapability); ok {
			rt.RegisterHostFunction("kscore", "kv_get", whf.createKVGet(kvCap))
			rt.RegisterHostFunction("kscore", "kv_set", whf.createKVSet(kvCap))
		}
	}
	if cap, err := whf.registry.Get("secrets.read"); err == nil && cap != nil {
		if secretsCap, ok := cap.(*capabilities.SecretsReadCapability); ok {
			rt.RegisterHostFunction("kscore", "secret_get", whf.createSecretGet(secretsCap))
		}
	}
	if cap, err := whf.registry.Get("time"); err == nil && cap != nil {
		if timeCap, ok := cap.(*capabilities.TimeCapability); ok {
			rt.RegisterHostFunction("kscore", "time_now", whf.createTimeNow(timeCap))
		}
	}
	return nil
}
