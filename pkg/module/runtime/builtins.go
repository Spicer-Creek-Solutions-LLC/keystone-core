// Package runtime provides capability builtins for Starlark and WASM runtimes
package runtime

import (
	"fmt"
	"os"

	"github.com/shawnbutts/keystone-core/pkg/module/capabilities"
	"github.com/shawnbutts/keystone-core/pkg/module/runtime/starlark"
	starlarklib "go.starlark.net/starlark"
)

// CapabilityBuiltins manages capability bindings for module runtimes
type CapabilityBuiltins struct {
	registry *capabilities.CapabilityRegistry
}

// NewCapabilityBuiltins creates a new CapabilityBuiltins instance
func NewCapabilityBuiltins(registry *capabilities.CapabilityRegistry) *CapabilityBuiltins {
	return &CapabilityBuiltins{
		registry: registry,
	}
}

// createCapabilityContext creates a context for capability invocations
func createCapabilityContext(moduleName, moduleVersion string) *capabilities.CapabilityContext {
	return &capabilities.CapabilityContext{
		ModuleName:    moduleName,
		ModuleVersion: moduleVersion,
		CorrelationID: "", // Could be set from thread local storage
	}
}

// RegisterStarlarkBuiltins registers all capability builtins with a Starlark runtime
// Uses safe type assertions to handle stub capabilities gracefully
func (cb *CapabilityBuiltins) RegisterStarlarkBuiltins(rt *starlark.Runtime) error {
	// Register fs.read capability
	if c, err := cb.registry.Get("fs.read"); err == nil && c != nil {
		if fsCap, ok := c.(*capabilities.FSReadCapability); ok {
			rt.RegisterCapability("fs_read", cb.createFSReadBuiltin(fsCap))
			rt.RegisterCapability("fs_exists", cb.createFSExistsBuiltin(fsCap))
		}
	}

	// Register fs.write capability
	if c, err := cb.registry.Get("fs.write"); err == nil && c != nil {
		if fsCap, ok := c.(*capabilities.FSWriteCapability); ok {
			rt.RegisterCapability("fs_write", cb.createFSWriteBuiltin(fsCap))
			rt.RegisterCapability("fs_delete", cb.createFSDeleteBuiltin(fsCap))
			rt.RegisterCapability("fs_mkdir", cb.createFSMkdirBuiltin(fsCap))
		}
	}

	// Register exec capability
	if c, err := cb.registry.Get("exec"); err == nil && c != nil {
		if execCap, ok := c.(*capabilities.ExecCapability); ok {
			rt.RegisterCapability("exec", cb.createExecBuiltin(execCap))
		}
	}

	// Register http.get capability
	if c, err := cb.registry.Get("http.get"); err == nil && c != nil {
		if httpCap, ok := c.(*capabilities.HTTPGetCapability); ok {
			rt.RegisterCapability("http_get", cb.createHTTPGetBuiltin(httpCap))
		}
	}

	// Register http.post capability
	if c, err := cb.registry.Get("http.post"); err == nil && c != nil {
		if httpCap, ok := c.(*capabilities.HTTPPostCapability); ok {
			rt.RegisterCapability("http_post", cb.createHTTPPostBuiltin(httpCap))
		}
	}

	// Register log capability
	if c, err := cb.registry.Get("log"); err == nil && c != nil {
		if logCap, ok := c.(*capabilities.LogCapability); ok {
			rt.RegisterCapability("log", cb.createLogBuiltin(logCap))
		}
	}

	// Register kv capability
	if c, err := cb.registry.Get("kv"); err == nil && c != nil {
		if kvCap, ok := c.(*capabilities.KVCapability); ok {
			rt.RegisterCapability("kv_get", cb.createKVGetBuiltin(kvCap))
			rt.RegisterCapability("kv_set", cb.createKVSetBuiltin(kvCap))
			rt.RegisterCapability("kv_delete", cb.createKVDeleteBuiltin(kvCap))
		}
	}

	// Register secrets.read capability
	if c, err := cb.registry.Get("secrets.read"); err == nil && c != nil {
		if secretsCap, ok := c.(*capabilities.SecretsReadCapability); ok {
			rt.RegisterCapability("secret_get", cb.createSecretGetBuiltin(secretsCap))
		}
	}

	// Register secrets.write capability
	if c, err := cb.registry.Get("secrets.write"); err == nil && c != nil {
		if secretsCap, ok := c.(*capabilities.SecretsWriteCapability); ok {
			rt.RegisterCapability("secret_set", cb.createSecretSetBuiltin(secretsCap))
		}
	}

	// Register time capability
	if c, err := cb.registry.Get("time"); err == nil && c != nil {
		if timeCap, ok := c.(*capabilities.TimeCapability); ok {
			rt.RegisterCapability("time_now", cb.createTimeNowBuiltin(timeCap))
		}
	}

	return nil
}

// ---- Filesystem Builtins ----

func (cb *CapabilityBuiltins) createFSReadBuiltin(c *capabilities.FSReadCapability) starlark.CapabilityFunc {
	return func(thread *starlarklib.Thread, fn *starlarklib.Builtin, args starlarklib.Tuple, kwargs []starlarklib.Tuple) (starlarklib.Value, error) {
		var path string
		if err := starlarklib.UnpackArgs(fn.Name(), args, kwargs, "path", &path); err != nil {
			return nil, err
		}

		ctx := createCapabilityContext("", "")
		data, err := c.ReadFile(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("fs_read: %w", err)
		}

		return starlarklib.String(string(data)), nil //nolint:nilerr // returning Starlark value with nil error is correct
	}
}

func (cb *CapabilityBuiltins) createFSExistsBuiltin(c *capabilities.FSReadCapability) starlark.CapabilityFunc {
	return func(thread *starlarklib.Thread, fn *starlarklib.Builtin, args starlarklib.Tuple, kwargs []starlarklib.Tuple) (starlarklib.Value, error) {
		var path string
		if err := starlarklib.UnpackArgs(fn.Name(), args, kwargs, "path", &path); err != nil {
			return nil, err
		}

		ctx := createCapabilityContext("", "")
		// Try to open the file to check existence
		f, err := c.OpenFile(ctx, path)
		if err != nil {
			return starlarklib.False, nil //nolint:nilerr // file not found is valid result, not an error
		}
		f.Close()
		return starlarklib.True, nil //nolint:nilerr // returning Starlark value with nil error is correct
	}
}

func (cb *CapabilityBuiltins) createFSWriteBuiltin(c *capabilities.FSWriteCapability) starlark.CapabilityFunc {
	return func(thread *starlarklib.Thread, fn *starlarklib.Builtin, args starlarklib.Tuple, kwargs []starlarklib.Tuple) (starlarklib.Value, error) {
		var path string
		var content string
		if err := starlarklib.UnpackArgs(fn.Name(), args, kwargs, "path", &path, "content", &content); err != nil {
			return nil, err
		}

		ctx := createCapabilityContext("", "")
		if err := c.WriteFile(ctx, path, []byte(content), 0o644); err != nil {
			return nil, fmt.Errorf("fs_write: %w", err)
		}

		return starlarklib.True, nil //nolint:nilerr // returning Starlark value with nil error is correct
	}
}

func (cb *CapabilityBuiltins) createFSDeleteBuiltin(c *capabilities.FSWriteCapability) starlark.CapabilityFunc {
	return func(thread *starlarklib.Thread, fn *starlarklib.Builtin, args starlarklib.Tuple, kwargs []starlarklib.Tuple) (starlarklib.Value, error) {
		var path string
		if err := starlarklib.UnpackArgs(fn.Name(), args, kwargs, "path", &path); err != nil {
			return nil, err
		}

		ctx := createCapabilityContext("", "")
		if err := c.DeleteFile(ctx, path); err != nil {
			return nil, fmt.Errorf("fs_delete: %w", err)
		}

		return starlarklib.True, nil //nolint:nilerr // returning Starlark value with nil error is correct
	}
}

func (cb *CapabilityBuiltins) createFSMkdirBuiltin(c *capabilities.FSWriteCapability) starlark.CapabilityFunc {
	return func(thread *starlarklib.Thread, fn *starlarklib.Builtin, args starlarklib.Tuple, kwargs []starlarklib.Tuple) (starlarklib.Value, error) {
		var path string
		if err := starlarklib.UnpackArgs(fn.Name(), args, kwargs, "path", &path); err != nil {
			return nil, err
		}

		ctx := createCapabilityContext("", "")
		if err := c.MkdirAll(ctx, path, os.ModePerm); err != nil {
			return nil, fmt.Errorf("fs_mkdir: %w", err)
		}

		return starlarklib.True, nil //nolint:nilerr // returning Starlark value with nil error is correct
	}
}

// ---- Exec Builtin ----

func (cb *CapabilityBuiltins) createExecBuiltin(c *capabilities.ExecCapability) starlark.CapabilityFunc {
	return func(thread *starlarklib.Thread, fn *starlarklib.Builtin, args starlarklib.Tuple, kwargs []starlarklib.Tuple) (starlarklib.Value, error) {
		var command string
		var cmdArgs *starlarklib.List
		if err := starlarklib.UnpackArgs(fn.Name(), args, kwargs, "command", &command, "args?", &cmdArgs); err != nil {
			return nil, err
		}

		// Convert Starlark list to Go slice
		var goArgs []string
		if cmdArgs != nil {
			for i := 0; i < cmdArgs.Len(); i++ {
				if s, ok := cmdArgs.Index(i).(starlarklib.String); ok {
					goArgs = append(goArgs, string(s))
				}
			}
		}

		ctx := createCapabilityContext("", "")
		result, err := c.Exec(ctx, command, goArgs...)
		if err != nil {
			return nil, fmt.Errorf("exec: %w", err)
		}

		// Return a dict with stdout, stderr, exit_code
		resultDict := starlarklib.NewDict(3)
		_ = resultDict.SetKey(starlarklib.String("stdout"), starlarklib.String(result.Stdout))        //nolint:errcheck // SetKey doesn't fail for string keys
		_ = resultDict.SetKey(starlarklib.String("stderr"), starlarklib.String(result.Stderr))        //nolint:errcheck // SetKey doesn't fail for string keys
		_ = resultDict.SetKey(starlarklib.String("exit_code"), starlarklib.MakeInt(result.ExitCode)) //nolint:errcheck // SetKey doesn't fail for string keys

		return resultDict, nil //nolint:nilerr // returning Starlark value with nil error is correct
	}
}

// ---- HTTP Builtins ----

func (cb *CapabilityBuiltins) createHTTPGetBuiltin(c *capabilities.HTTPGetCapability) starlark.CapabilityFunc {
	return func(thread *starlarklib.Thread, fn *starlarklib.Builtin, args starlarklib.Tuple, kwargs []starlarklib.Tuple) (starlarklib.Value, error) {
		var url string
		if err := starlarklib.UnpackArgs(fn.Name(), args, kwargs, "url", &url); err != nil {
			return nil, err
		}

		ctx := createCapabilityContext("", "")
		resp, err := c.Get(ctx, url, nil)
		if err != nil {
			return nil, fmt.Errorf("http_get: %w", err)
		}

		// Return a dict with status_code, body, headers
		result := starlarklib.NewDict(3)
		_ = result.SetKey(starlarklib.String("status_code"), starlarklib.MakeInt(resp.StatusCode)) //nolint:errcheck // SetKey doesn't fail for string keys
		_ = result.SetKey(starlarklib.String("body"), starlarklib.String(string(resp.Body)))       //nolint:errcheck // SetKey doesn't fail for string keys

		headers := starlarklib.NewDict(len(resp.Headers))
		for k, vals := range resp.Headers {
			// HTTP headers can have multiple values, join with comma
			if len(vals) > 0 {
				_ = headers.SetKey(starlarklib.String(k), starlarklib.String(vals[0])) //nolint:errcheck // SetKey doesn't fail for string keys
			}
		}
		_ = result.SetKey(starlarklib.String("headers"), headers) //nolint:errcheck // SetKey doesn't fail for string keys

		return result, nil //nolint:nilerr // returning Starlark value with nil error is correct
	}
}

func (cb *CapabilityBuiltins) createHTTPPostBuiltin(c *capabilities.HTTPPostCapability) starlark.CapabilityFunc {
	return func(thread *starlarklib.Thread, fn *starlarklib.Builtin, args starlarklib.Tuple, kwargs []starlarklib.Tuple) (starlarklib.Value, error) {
		var url string
		var body string
		var contentType = "application/json"
		if err := starlarklib.UnpackArgs(fn.Name(), args, kwargs, "url", &url, "body", &body, "content_type?", &contentType); err != nil {
			return nil, err
		}

		ctx := createCapabilityContext("", "")
		headers := map[string]string{"Content-Type": contentType}
		resp, err := c.Post(ctx, url, []byte(body), headers)
		if err != nil {
			return nil, fmt.Errorf("http_post: %w", err)
		}

		// Return a dict with status_code, body, headers
		result := starlarklib.NewDict(3)
		_ = result.SetKey(starlarklib.String("status_code"), starlarklib.MakeInt(resp.StatusCode)) //nolint:errcheck // SetKey doesn't fail for string keys
		_ = result.SetKey(starlarklib.String("body"), starlarklib.String(string(resp.Body)))       //nolint:errcheck // SetKey doesn't fail for string keys

		respHeaders := starlarklib.NewDict(len(resp.Headers))
		for k, vals := range resp.Headers {
			if len(vals) > 0 {
				_ = respHeaders.SetKey(starlarklib.String(k), starlarklib.String(vals[0])) //nolint:errcheck // SetKey doesn't fail for string keys
			}
		}
		_ = result.SetKey(starlarklib.String("headers"), respHeaders) //nolint:errcheck // SetKey doesn't fail for string keys

		return result, nil //nolint:nilerr // returning Starlark value with nil error is correct
	}
}

// ---- Log Builtin ----

func (cb *CapabilityBuiltins) createLogBuiltin(c *capabilities.LogCapability) starlark.CapabilityFunc {
	return func(thread *starlarklib.Thread, fn *starlarklib.Builtin, args starlarklib.Tuple, kwargs []starlarklib.Tuple) (starlarklib.Value, error) {
		var level string
		var message string
		if err := starlarklib.UnpackArgs(fn.Name(), args, kwargs, "level", &level, "message", &message); err != nil {
			return nil, err
		}

		ctx := createCapabilityContext("", "")
		if err := c.Log(ctx, level, message, nil); err != nil {
			return nil, fmt.Errorf("log: %w", err)
		}

		return starlarklib.None, nil //nolint:nilerr // returning Starlark value with nil error is correct
	}
}

// ---- KV Builtins ----

func (cb *CapabilityBuiltins) createKVGetBuiltin(c *capabilities.KVCapability) starlark.CapabilityFunc {
	return func(thread *starlarklib.Thread, fn *starlarklib.Builtin, args starlarklib.Tuple, kwargs []starlarklib.Tuple) (starlarklib.Value, error) {
		var key string
		if err := starlarklib.UnpackArgs(fn.Name(), args, kwargs, "key", &key); err != nil {
			return nil, err
		}

		ctx := createCapabilityContext("", "")
		value, err := c.Get(ctx, key)
		if err != nil {
			// Key not found returns None
			return starlarklib.None, nil //nolint:nilerr // key not found is valid result, not an error
		}

		return starlarklib.String(value), nil //nolint:nilerr // returning Starlark value with nil error is correct
	}
}

func (cb *CapabilityBuiltins) createKVSetBuiltin(c *capabilities.KVCapability) starlark.CapabilityFunc {
	return func(thread *starlarklib.Thread, fn *starlarklib.Builtin, args starlarklib.Tuple, kwargs []starlarklib.Tuple) (starlarklib.Value, error) {
		var key string
		var value string
		if err := starlarklib.UnpackArgs(fn.Name(), args, kwargs, "key", &key, "value", &value); err != nil {
			return nil, err
		}

		ctx := createCapabilityContext("", "")
		if err := c.Set(ctx, key, value); err != nil {
			return nil, fmt.Errorf("kv_set: %w", err)
		}

		return starlarklib.True, nil //nolint:nilerr // returning Starlark value with nil error is correct
	}
}

func (cb *CapabilityBuiltins) createKVDeleteBuiltin(c *capabilities.KVCapability) starlark.CapabilityFunc {
	return func(thread *starlarklib.Thread, fn *starlarklib.Builtin, args starlarklib.Tuple, kwargs []starlarklib.Tuple) (starlarklib.Value, error) {
		var key string
		if err := starlarklib.UnpackArgs(fn.Name(), args, kwargs, "key", &key); err != nil {
			return nil, err
		}

		ctx := createCapabilityContext("", "")
		if err := c.Delete(ctx, key); err != nil {
			return nil, fmt.Errorf("kv_delete: %w", err)
		}

		return starlarklib.True, nil //nolint:nilerr // returning Starlark value with nil error is correct
	}
}

// ---- Secrets Builtins ----

func (cb *CapabilityBuiltins) createSecretGetBuiltin(c *capabilities.SecretsReadCapability) starlark.CapabilityFunc {
	return func(thread *starlarklib.Thread, fn *starlarklib.Builtin, args starlarklib.Tuple, kwargs []starlarklib.Tuple) (starlarklib.Value, error) {
		var path string
		if err := starlarklib.UnpackArgs(fn.Name(), args, kwargs, "path", &path); err != nil {
			return nil, err
		}

		ctx := createCapabilityContext("", "")
		value, err := c.ReadSecret(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("secret_get: %w", err)
		}

		return starlarklib.String(value), nil //nolint:nilerr // returning Starlark value with nil error is correct
	}
}

func (cb *CapabilityBuiltins) createSecretSetBuiltin(c *capabilities.SecretsWriteCapability) starlark.CapabilityFunc {
	return func(thread *starlarklib.Thread, fn *starlarklib.Builtin, args starlarklib.Tuple, kwargs []starlarklib.Tuple) (starlarklib.Value, error) {
		var path string
		var value string
		if err := starlarklib.UnpackArgs(fn.Name(), args, kwargs, "path", &path, "value", &value); err != nil {
			return nil, err
		}

		ctx := createCapabilityContext("", "")
		if err := c.WriteSecret(ctx, path, value); err != nil {
			return nil, fmt.Errorf("secret_set: %w", err)
		}

		return starlarklib.True, nil //nolint:nilerr // returning Starlark value with nil error is correct
	}
}

// ---- Time Builtin ----

func (cb *CapabilityBuiltins) createTimeNowBuiltin(c *capabilities.TimeCapability) starlark.CapabilityFunc {
	return func(thread *starlarklib.Thread, fn *starlarklib.Builtin, args starlarklib.Tuple, kwargs []starlarklib.Tuple) (starlarklib.Value, error) {
		ctx := createCapabilityContext("", "")
		t := c.Now(ctx)
		return starlarklib.MakeInt64(t.Unix()), nil //nolint:nilerr // returning Starlark value with nil error is correct
	}
}
