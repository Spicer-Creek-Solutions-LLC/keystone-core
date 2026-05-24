// SPDX-License-Identifier: Apache-2.0

// Package starlarksdk is the v1.0 Starlark host-capability SDK
// (Epic 14 task 12, PROJECT-DETAILS §4.18): Go shims that expose
// the task-3 capability backends to a Starlark module as namespaced
// builtins (fs / http / exec / secrets / kv / log), with every call
// routed through the task-2 capability.Invoker (grant-gate + audit).
//
// It fills the task-11 starlark.BuiltinProvider seam. Per-module
// Invoker construction (granted Registry + auditor) is the
// kscore-module run / server boot wiring (task 14 + the gate-v1.0
// "Module system boot wiring" ROADMAP entry); this package is the
// bindings, tested with the Invoker wired directly. Example modules
// are task 16.
//
// Capability calls use context.Background(): the task-11 thread
// watchdog (thread.Cancel on timeout/ctx-cancel) already aborts the
// whole Starlark execution mid-call, so per-call ctx is sufficient
// for v1.0 (see the "Per-capability-call context propagation"
// ROADMAP entry). No new dependency.
package starlarksdk

import (
	"context"
	"fmt"
	"sort"

	star "go.starlark.net/starlark"

	"go.keystone-core.io/keystone-core/pkg/module/capability"
	"go.keystone-core.io/keystone-core/pkg/module/manifest"
	srt "go.keystone-core.io/keystone-core/pkg/module/runtime/starlark"
)

// nsValue is a capability namespace exposed to Starlark (e.g. `fs`,
// `kv`) — a frozen value whose attributes are method builtins.
type nsValue struct {
	name    string
	methods map[string]*star.Builtin
}

func newNS(name string, methods map[string]*star.Builtin) *nsValue {
	return &nsValue{name: name, methods: methods}
}

func (n *nsValue) String() string        { return fmt.Sprintf("<capability %q>", n.name) }
func (n *nsValue) Type() string          { return "capability." + n.name }
func (n *nsValue) Freeze()               {}
func (n *nsValue) Truth() star.Bool      { return star.True }
func (n *nsValue) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: capability.%s", n.name) }

func (n *nsValue) Attr(name string) (star.Value, error) {
	if b, ok := n.methods[name]; ok {
		return b, nil
	}
	return nil, nil // nil,nil ⇒ "no such attribute" (Starlark AttributeError)
}

func (n *nsValue) AttrNames() []string {
	names := make([]string, 0, len(n.methods))
	for k := range n.methods {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

var _ star.HasAttrs = (*nsValue)(nil)

// guard runs fn through the Invoker (audit + grant-gate) and turns a
// capability error into a Starlark error (aborts the module — the
// intended security behavior; the failure is audited regardless).
func guard(inv *capability.Invoker, capName, op string, fn func(context.Context) error) error {
	return inv.Invoke(context.Background(), capName, op, fn)
}

// ---- builtin constructors -----------------------------------------------

func fsReadFn(rd *capability.FSRead, inv *capability.Invoker) *star.Builtin {
	return star.NewBuiltin("read", func(_ *star.Thread, b *star.Builtin, args star.Tuple, kw []star.Tuple) (star.Value, error) {
		var path string
		if err := star.UnpackArgs(b.Name(), args, kw, "path", &path); err != nil {
			return nil, err
		}
		var data []byte
		if err := guard(inv, manifest.CapFSRead, "read", func(context.Context) error {
			var e error
			data, e = rd.Read(path)
			return e
		}); err != nil {
			return nil, err
		}
		return star.String(data), nil
	})
}

func fsWriteFn(wr *capability.FSWrite, inv *capability.Invoker) *star.Builtin {
	return star.NewBuiltin("write", func(_ *star.Thread, b *star.Builtin, args star.Tuple, kw []star.Tuple) (star.Value, error) {
		var path, data string
		if err := star.UnpackArgs(b.Name(), args, kw, "path", &path, "data", &data); err != nil {
			return nil, err
		}
		if err := guard(inv, manifest.CapFSWrite, "write", func(context.Context) error {
			return wr.Write(path, []byte(data), 0o644)
		}); err != nil {
			return nil, err
		}
		return star.None, nil
	})
}

func httpCallFn(method, capName string, h *capability.HTTPCap, inv *capability.Invoker) *star.Builtin {
	return star.NewBuiltin(method, func(_ *star.Thread, b *star.Builtin, args star.Tuple, kw []star.Tuple) (star.Value, error) {
		var url, body string
		if err := star.UnpackArgs(b.Name(), args, kw, "url", &url, "body?", &body); err != nil {
			return nil, err
		}
		var (
			respBody []byte
			status   int
		)
		if err := guard(inv, capName, method, func(ctx context.Context) error {
			var e error
			respBody, status, e = h.Call(ctx, url, []byte(body))
			return e
		}); err != nil {
			return nil, err
		}
		d := star.NewDict(2)
		_ = d.SetKey(star.String("status"), star.MakeInt(status))
		_ = d.SetKey(star.String("body"), star.String(respBody))
		return d, nil
	})
}

func execRunFn(ex *capability.Exec, inv *capability.Invoker) *star.Builtin {
	return star.NewBuiltin("run", func(_ *star.Thread, b *star.Builtin, args star.Tuple, kw []star.Tuple) (star.Value, error) {
		var name string
		var argList *star.List
		if err := star.UnpackArgs(b.Name(), args, kw, "cmd", &name, "args?", &argList); err != nil {
			return nil, err
		}
		cmdArgs, err := listToStrings(argList)
		if err != nil {
			return nil, err
		}
		var stdout, stderr []byte
		if err := guard(inv, manifest.CapExec, "run", func(ctx context.Context) error {
			var e error
			stdout, stderr, e = ex.Run(ctx, name, cmdArgs)
			return e
		}); err != nil {
			return nil, err
		}
		d := star.NewDict(2)
		_ = d.SetKey(star.String("stdout"), star.String(stdout))
		_ = d.SetKey(star.String("stderr"), star.String(stderr))
		return d, nil
	})
}

func secretsReadFn(rd *capability.SecretsRead, inv *capability.Invoker) *star.Builtin {
	return star.NewBuiltin("read", func(_ *star.Thread, b *star.Builtin, args star.Tuple, kw []star.Tuple) (star.Value, error) {
		var path string
		if err := star.UnpackArgs(b.Name(), args, kw, "path", &path); err != nil {
			return nil, err
		}
		var secret map[string]any
		if err := guard(inv, manifest.CapSecretsRead, "read", func(ctx context.Context) error {
			var e error
			secret, e = rd.Get(ctx, path)
			return e
		}); err != nil {
			return nil, err
		}
		return srt.ToValue(secret)
	})
}

func secretsWriteFn(wr *capability.SecretsWrite, inv *capability.Invoker) *star.Builtin {
	return star.NewBuiltin("write", func(_ *star.Thread, b *star.Builtin, args star.Tuple, kw []star.Tuple) (star.Value, error) {
		var path string
		var data *star.Dict
		if err := star.UnpackArgs(b.Name(), args, kw, "path", &path, "data", &data); err != nil {
			return nil, err
		}
		gv, err := srt.FromValue(data)
		if err != nil {
			return nil, err
		}
		m, ok := gv.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("secrets.write: data must be a dict")
		}
		if err := guard(inv, manifest.CapSecretsWrite, "write", func(ctx context.Context) error {
			return wr.Set(ctx, path, m)
		}); err != nil {
			return nil, err
		}
		return star.None, nil
	})
}

func kvNamespace(kv *capability.KV, inv *capability.Invoker) *nsValue {
	get := star.NewBuiltin("get", func(_ *star.Thread, b *star.Builtin, args star.Tuple, kw []star.Tuple) (star.Value, error) {
		var key string
		if err := star.UnpackArgs(b.Name(), args, kw, "key", &key); err != nil {
			return nil, err
		}
		var v string
		var found bool
		_ = guard(inv, manifest.CapKV, "get", func(context.Context) error {
			v, found = kv.Get(key)
			return nil
		})
		if !found {
			return star.None, nil
		}
		return star.String(v), nil
	})
	set := star.NewBuiltin("set", func(_ *star.Thread, b *star.Builtin, args star.Tuple, kw []star.Tuple) (star.Value, error) {
		var key, val string
		if err := star.UnpackArgs(b.Name(), args, kw, "key", &key, "value", &val); err != nil {
			return nil, err
		}
		if err := guard(inv, manifest.CapKV, "set", func(context.Context) error {
			return kv.Set(key, val)
		}); err != nil {
			return nil, err
		}
		return star.None, nil
	})
	del := star.NewBuiltin("delete", func(_ *star.Thread, b *star.Builtin, args star.Tuple, kw []star.Tuple) (star.Value, error) {
		var key string
		if err := star.UnpackArgs(b.Name(), args, kw, "key", &key); err != nil {
			return nil, err
		}
		_ = guard(inv, manifest.CapKV, "delete", func(context.Context) error {
			kv.Delete(key)
			return nil
		})
		return star.None, nil
	})
	return newNS("kv", map[string]*star.Builtin{"get": get, "set": set, "delete": del})
}

func logNamespace(lg *capability.Log, inv *capability.Invoker) *nsValue {
	mk := func(level string) *star.Builtin {
		return star.NewBuiltin(level, func(_ *star.Thread, b *star.Builtin, args star.Tuple, kw []star.Tuple) (star.Value, error) {
			var msg string
			if err := star.UnpackArgs(b.Name(), star.Tuple{firstOrEmpty(args)}, nil, "msg", &msg); err != nil {
				return nil, err
			}
			fields := map[string]string{}
			for _, pair := range kw {
				k, _ := star.AsString(pair[0])
				fields[k] = pair[1].String()
			}
			if err := guard(inv, manifest.CapLog, level, func(context.Context) error {
				return lg.Emit(level, msg, fields)
			}); err != nil {
				return nil, err
			}
			return star.None, nil
		})
	}
	return newNS("log", map[string]*star.Builtin{
		"debug": mk("debug"), "info": mk("info"), "warn": mk("warn"), "error": mk("error"),
	})
}

// ---- assembly -----------------------------------------------------------

// BuildStringDict turns the granted capability backends (from
// capability.BuildCapabilities) into the Starlark predeclared
// namespace globals, every call routed through inv.
func BuildStringDict(caps map[string]any, inv *capability.Invoker) (star.StringDict, error) {
	if inv == nil {
		return nil, fmt.Errorf("starlarksdk: nil invoker")
	}
	fsMethods := map[string]*star.Builtin{}
	httpMethods := map[string]*star.Builtin{}
	secretsMethods := map[string]*star.Builtin{}
	dict := star.StringDict{}

	names := make([]string, 0, len(caps))
	for n := range caps {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		switch c := caps[name].(type) {
		case *capability.FSRead:
			fsMethods["read"] = fsReadFn(c, inv)
		case *capability.FSWrite:
			fsMethods["write"] = fsWriteFn(c, inv)
		case *capability.HTTPCap:
			if name == manifest.CapHTTPGet {
				httpMethods["get"] = httpCallFn("get", manifest.CapHTTPGet, c, inv)
			} else {
				httpMethods["post"] = httpCallFn("post", manifest.CapHTTPPost, c, inv)
			}
		case *capability.Exec:
			dict["exec"] = newNS("exec", map[string]*star.Builtin{"run": execRunFn(c, inv)})
		case *capability.SecretsRead:
			secretsMethods["read"] = secretsReadFn(c, inv)
		case *capability.SecretsWrite:
			secretsMethods["write"] = secretsWriteFn(c, inv)
		case *capability.KV:
			dict["kv"] = kvNamespace(c, inv)
		case *capability.Log:
			dict["log"] = logNamespace(c, inv)
		default:
			return nil, fmt.Errorf("starlarksdk: unknown capability %q (%T)", name, caps[name])
		}
	}
	if len(fsMethods) > 0 {
		dict["fs"] = newNS("fs", fsMethods)
	}
	if len(httpMethods) > 0 {
		dict["http"] = newNS("http", httpMethods)
	}
	if len(secretsMethods) > 0 {
		dict["secrets"] = newNS("secrets", secretsMethods)
	}
	return dict, nil
}

// Provider adapts BuildStringDict to the task-11
// starlark.BuiltinProvider seam, binding the per-module Invoker.
func Provider(inv *capability.Invoker) srt.BuiltinProvider {
	return func(caps map[string]any) (star.StringDict, error) {
		return BuildStringDict(caps, inv)
	}
}

func listToStrings(l *star.List) ([]string, error) {
	if l == nil {
		return nil, nil
	}
	out := make([]string, 0, l.Len())
	for i := 0; i < l.Len(); i++ {
		s, ok := star.AsString(l.Index(i))
		if !ok {
			return nil, fmt.Errorf("exec args[%d] is not a string", i)
		}
		out = append(out, s)
	}
	return out, nil
}

func firstOrEmpty(args star.Tuple) star.Value {
	if len(args) > 0 {
		return args[0]
	}
	return star.String("")
}
