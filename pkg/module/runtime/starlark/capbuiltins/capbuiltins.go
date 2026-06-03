// SPDX-License-Identifier: Apache-2.0

// Package capbuiltins is the production starlark.BuiltinProvider
// (Epic 14 task 12): it bridges a module's granted capability backends
// into the Starlark builtins the module calls — fs_read, http_get,
// exec_run, secret_read, kv_*, log, etc.
//
// Each builtin delegates to the scoped capability backend that the
// loader built from the manifest, so every effect is already
// path-/domain-/command-scoped and size/rate/timeout-limited. A
// capability the module did not request is simply absent from the
// Starlark namespace (the backend isn't in the caps map). Host calls
// inherit the execution deadline via
// [starlark.ContextFromThread].
package capbuiltins

import (
	"fmt"

	star "go.starlark.net/starlark"

	"go.keystone-core.io/keystone-core/pkg/module/capability"
	"go.keystone-core.io/keystone-core/pkg/module/manifest"
	srt "go.keystone-core.io/keystone-core/pkg/module/runtime/starlark"
)

// Provider is the starlark.BuiltinProvider. It returns the predeclared
// Starlark globals for the given granted capability backends. Returns
// an error only if a backend has an unexpected concrete type (a loader
// wiring bug), never for a capability the module simply lacks.
func Provider(caps map[string]any) (star.StringDict, error) {
	d := star.StringDict{}
	for name, backend := range caps {
		if err := bind(d, name, backend); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func bind(d star.StringDict, name string, backend any) error {
	switch name {
	case manifest.CapFSRead:
		b, ok := backend.(*capability.FSRead)
		if !ok {
			return typeErr(name, backend)
		}
		d["fs_read"] = star.NewBuiltin("fs_read", fsRead(b))
	case manifest.CapFSWrite:
		b, ok := backend.(*capability.FSWrite)
		if !ok {
			return typeErr(name, backend)
		}
		d["fs_write"] = star.NewBuiltin("fs_write", fsWrite(b))
	case manifest.CapHTTPGet:
		b, ok := backend.(*capability.HTTPCap)
		if !ok {
			return typeErr(name, backend)
		}
		d["http_get"] = star.NewBuiltin("http_get", httpGet(b))
	case manifest.CapHTTPPost:
		b, ok := backend.(*capability.HTTPCap)
		if !ok {
			return typeErr(name, backend)
		}
		d["http_post"] = star.NewBuiltin("http_post", httpPost(b))
	case manifest.CapExec:
		b, ok := backend.(*capability.Exec)
		if !ok {
			return typeErr(name, backend)
		}
		d["exec_run"] = star.NewBuiltin("exec_run", execRun(b))
	case manifest.CapSecretsRead:
		b, ok := backend.(*capability.SecretsRead)
		if !ok {
			return typeErr(name, backend)
		}
		d["secret_read"] = star.NewBuiltin("secret_read", secretRead(b))
	case manifest.CapSecretsWrite:
		b, ok := backend.(*capability.SecretsWrite)
		if !ok {
			return typeErr(name, backend)
		}
		d["secret_write"] = star.NewBuiltin("secret_write", secretWrite(b))
	case manifest.CapKV:
		b, ok := backend.(*capability.KV)
		if !ok {
			return typeErr(name, backend)
		}
		d["kv_get"] = star.NewBuiltin("kv_get", kvGet(b))
		d["kv_set"] = star.NewBuiltin("kv_set", kvSet(b))
		d["kv_delete"] = star.NewBuiltin("kv_delete", kvDelete(b))
	case manifest.CapLog:
		b, ok := backend.(*capability.Log)
		if !ok {
			return typeErr(name, backend)
		}
		d["log"] = star.NewBuiltin("log", logEmit(b))
	default:
		return fmt.Errorf("capbuiltins: unknown capability %q", name)
	}
	return nil
}

func typeErr(name string, backend any) error {
	return fmt.Errorf("capbuiltins: capability %q has unexpected backend type %T", name, backend)
}

// ---- filesystem -------------------------------------------------------

func fsRead(b *capability.FSRead) builtinFn {
	return func(t *star.Thread, _ *star.Builtin, args star.Tuple, kwargs []star.Tuple) (star.Value, error) {
		var path string
		if err := star.UnpackArgs("fs_read", args, kwargs, "path", &path); err != nil {
			return nil, err
		}
		data, err := b.Read(path)
		if err != nil {
			return nil, err
		}
		return star.String(data), nil
	}
}

func fsWrite(b *capability.FSWrite) builtinFn {
	return func(t *star.Thread, _ *star.Builtin, args star.Tuple, kwargs []star.Tuple) (star.Value, error) {
		var (
			path string
			data string
			perm = 0o644
		)
		if err := star.UnpackArgs("fs_write", args, kwargs, "path", &path, "data", &data, "perm?", &perm); err != nil {
			return nil, err
		}
		if err := b.Write(path, []byte(data), uint32(perm)); err != nil {
			return nil, err
		}
		return star.None, nil
	}
}

// ---- http -------------------------------------------------------------

func httpGet(b *capability.HTTPCap) builtinFn {
	return func(t *star.Thread, _ *star.Builtin, args star.Tuple, kwargs []star.Tuple) (star.Value, error) {
		var url string
		if err := star.UnpackArgs("http_get", args, kwargs, "url", &url); err != nil {
			return nil, err
		}
		body, status, err := b.Call(srt.ContextFromThread(t), url, nil)
		if err != nil {
			return nil, err
		}
		return star.Tuple{star.String(body), star.MakeInt(status)}, nil
	}
}

func httpPost(b *capability.HTTPCap) builtinFn {
	return func(t *star.Thread, _ *star.Builtin, args star.Tuple, kwargs []star.Tuple) (star.Value, error) {
		var url, body string
		if err := star.UnpackArgs("http_post", args, kwargs, "url", &url, "body", &body); err != nil {
			return nil, err
		}
		respBody, status, err := b.Call(srt.ContextFromThread(t), url, []byte(body))
		if err != nil {
			return nil, err
		}
		return star.Tuple{star.String(respBody), star.MakeInt(status)}, nil
	}
}

// ---- exec -------------------------------------------------------------

func execRun(b *capability.Exec) builtinFn {
	return func(t *star.Thread, _ *star.Builtin, args star.Tuple, kwargs []star.Tuple) (star.Value, error) {
		var (
			name    string
			argList *star.List
		)
		if err := star.UnpackArgs("exec_run", args, kwargs, "name", &name, "args?", &argList); err != nil {
			return nil, err
		}
		cmdArgs, err := stringSlice(argList)
		if err != nil {
			return nil, fmt.Errorf("exec_run: args: %w", err)
		}
		stdout, stderr, err := b.Run(srt.ContextFromThread(t), name, cmdArgs)
		if err != nil {
			return nil, err
		}
		return star.Tuple{star.String(stdout), star.String(stderr)}, nil
	}
}

// ---- secrets ----------------------------------------------------------

func secretRead(b *capability.SecretsRead) builtinFn {
	return func(t *star.Thread, _ *star.Builtin, args star.Tuple, kwargs []star.Tuple) (star.Value, error) {
		var path string
		if err := star.UnpackArgs("secret_read", args, kwargs, "path", &path); err != nil {
			return nil, err
		}
		data, err := b.Get(srt.ContextFromThread(t), path)
		if err != nil {
			return nil, err
		}
		return srt.ToValue(data)
	}
}

func secretWrite(b *capability.SecretsWrite) builtinFn {
	return func(t *star.Thread, _ *star.Builtin, args star.Tuple, kwargs []star.Tuple) (star.Value, error) {
		var (
			path string
			data *star.Dict
		)
		if err := star.UnpackArgs("secret_write", args, kwargs, "path", &path, "data", &data); err != nil {
			return nil, err
		}
		goVal, err := srt.FromValue(data)
		if err != nil {
			return nil, fmt.Errorf("secret_write: data: %w", err)
		}
		m, ok := goVal.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("secret_write: data must be a dict, got %T", goVal)
		}
		if err := b.Set(srt.ContextFromThread(t), path, m); err != nil {
			return nil, err
		}
		return star.None, nil
	}
}

// ---- kv ---------------------------------------------------------------

func kvGet(b *capability.KV) builtinFn {
	return func(t *star.Thread, _ *star.Builtin, args star.Tuple, kwargs []star.Tuple) (star.Value, error) {
		var key string
		if err := star.UnpackArgs("kv_get", args, kwargs, "key", &key); err != nil {
			return nil, err
		}
		val, ok := b.Get(key)
		return star.Tuple{star.String(val), star.Bool(ok)}, nil
	}
}

func kvSet(b *capability.KV) builtinFn {
	return func(t *star.Thread, _ *star.Builtin, args star.Tuple, kwargs []star.Tuple) (star.Value, error) {
		var key, value string
		if err := star.UnpackArgs("kv_set", args, kwargs, "key", &key, "value", &value); err != nil {
			return nil, err
		}
		if err := b.Set(key, value); err != nil {
			return nil, err
		}
		return star.None, nil
	}
}

func kvDelete(b *capability.KV) builtinFn {
	return func(t *star.Thread, _ *star.Builtin, args star.Tuple, kwargs []star.Tuple) (star.Value, error) {
		var key string
		if err := star.UnpackArgs("kv_delete", args, kwargs, "key", &key); err != nil {
			return nil, err
		}
		b.Delete(key)
		return star.None, nil
	}
}

// ---- log --------------------------------------------------------------

func logEmit(b *capability.Log) builtinFn {
	return func(t *star.Thread, _ *star.Builtin, args star.Tuple, kwargs []star.Tuple) (star.Value, error) {
		var level, msg string
		if err := star.UnpackArgs("log", args, kwargs, "level", &level, "msg", &msg); err != nil {
			return nil, err
		}
		if err := b.Emit(level, msg, nil); err != nil {
			return nil, err
		}
		return star.None, nil
	}
}

// ---- helpers ----------------------------------------------------------

type builtinFn = func(*star.Thread, *star.Builtin, star.Tuple, []star.Tuple) (star.Value, error)

// stringSlice converts an optional Starlark list of strings to []string.
// nil list ⇒ empty slice.
func stringSlice(l *star.List) ([]string, error) {
	if l == nil {
		return nil, nil
	}
	out := make([]string, 0, l.Len())
	for i := 0; i < l.Len(); i++ {
		s, ok := star.AsString(l.Index(i))
		if !ok {
			return nil, fmt.Errorf("element %d is not a string", i)
		}
		out = append(out, s)
	}
	return out, nil
}
