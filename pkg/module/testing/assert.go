package moduletest

import (
	"fmt"
	"sort"
	"strings"

	star "go.starlark.net/starlark"
)

// ns is the frozen `assert` namespace value handed to every test
// file. Same shape as the SDK's capability namespace (kept local so
// the SDK internal stays internal); the methods are stateless so a
// single package-level instance is shared.
type ns struct {
	name    string
	methods map[string]*star.Builtin
}

func (n *ns) String() string        { return fmt.Sprintf("<%s>", n.name) }
func (n *ns) Type() string          { return n.name }
func (n *ns) Freeze()               {}
func (n *ns) Truth() star.Bool      { return star.True }
func (n *ns) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: %s", n.name) }

func (n *ns) Attr(name string) (star.Value, error) {
	if b, ok := n.methods[name]; ok {
		return b, nil
	}
	return nil, nil // nil,nil => Starlark AttributeError
}

func (n *ns) AttrNames() []string {
	out := make([]string, 0, len(n.methods))
	for k := range n.methods {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

var _ star.HasAttrs = (*ns)(nil)

// assertNS is the shared `assert` namespace.
var assertNS = &ns{name: "assert", methods: map[string]*star.Builtin{
	"eq":       star.NewBuiltin("assert.eq", assertEq),
	"ne":       star.NewBuiltin("assert.ne", assertNe),
	"true":     star.NewBuiltin("assert.true", assertTrue),
	"false":    star.NewBuiltin("assert.false", assertFalse),
	"contains": star.NewBuiltin("assert.contains", assertContains),
	"fails":    star.NewBuiltin("assert.fails", assertFails),
	"fail":     star.NewBuiltin("assert.fail", assertFail),
}}

func withMsg(base, msg string) error {
	if msg == "" {
		return fmt.Errorf("%s", base)
	}
	return fmt.Errorf("%s: %s", msg, base)
}

func assertEq(_ *star.Thread, _ *star.Builtin, args star.Tuple, kwargs []star.Tuple) (star.Value, error) {
	var want, got star.Value
	var msg string
	if err := star.UnpackArgs("assert.eq", args, kwargs, "want", &want, "got", &got, "msg?", &msg); err != nil {
		return nil, err
	}
	eq, err := star.Equal(want, got)
	if err != nil {
		return nil, err
	}
	if !eq {
		return nil, withMsg(fmt.Sprintf("assert.eq: want %s, got %s", want.String(), got.String()), msg)
	}
	return star.None, nil
}

func assertNe(_ *star.Thread, _ *star.Builtin, args star.Tuple, kwargs []star.Tuple) (star.Value, error) {
	var a, b star.Value
	var msg string
	if err := star.UnpackArgs("assert.ne", args, kwargs, "a", &a, "b", &b, "msg?", &msg); err != nil {
		return nil, err
	}
	eq, err := star.Equal(a, b)
	if err != nil {
		return nil, err
	}
	if eq {
		return nil, withMsg(fmt.Sprintf("assert.ne: both %s", a.String()), msg)
	}
	return star.None, nil
}

func assertTrue(_ *star.Thread, _ *star.Builtin, args star.Tuple, kwargs []star.Tuple) (star.Value, error) {
	var v star.Value
	var msg string
	if err := star.UnpackArgs("assert.true", args, kwargs, "v", &v, "msg?", &msg); err != nil {
		return nil, err
	}
	if !bool(v.Truth()) {
		return nil, withMsg(fmt.Sprintf("assert.true: %s is falsy", v.String()), msg)
	}
	return star.None, nil
}

func assertFalse(_ *star.Thread, _ *star.Builtin, args star.Tuple, kwargs []star.Tuple) (star.Value, error) {
	var v star.Value
	var msg string
	if err := star.UnpackArgs("assert.false", args, kwargs, "v", &v, "msg?", &msg); err != nil {
		return nil, err
	}
	if bool(v.Truth()) {
		return nil, withMsg(fmt.Sprintf("assert.false: %s is truthy", v.String()), msg)
	}
	return star.None, nil
}

func assertContains(_ *star.Thread, _ *star.Builtin, args star.Tuple, kwargs []star.Tuple) (star.Value, error) {
	var container, item star.Value
	var msg string
	if err := star.UnpackArgs("assert.contains", args, kwargs,
		"container", &container, "item", &item, "msg?", &msg); err != nil {
		return nil, err
	}
	if s, ok := container.(star.String); ok {
		sub, ok := item.(star.String)
		if !ok {
			return nil, withMsg("assert.contains: string container needs a string item", msg)
		}
		if !strings.Contains(string(s), string(sub)) {
			return nil, withMsg(fmt.Sprintf("assert.contains: %q not in %q", string(sub), string(s)), msg)
		}
		return star.None, nil
	}
	it := star.Iterate(container)
	if it == nil {
		return nil, withMsg(fmt.Sprintf("assert.contains: %s is not iterable", container.Type()), msg)
	}
	defer it.Done()
	var x star.Value
	for it.Next(&x) {
		eq, err := star.Equal(x, item)
		if err != nil {
			return nil, err
		}
		if eq {
			return star.None, nil
		}
	}
	return nil, withMsg(fmt.Sprintf("assert.contains: %s not found in %s", item.String(), container.String()), msg)
}

func assertFails(thread *star.Thread, _ *star.Builtin, args star.Tuple, kwargs []star.Tuple) (star.Value, error) {
	var fn star.Value
	var msg string
	if err := star.UnpackArgs("assert.fails", args, kwargs, "fn", &fn, "msg?", &msg); err != nil {
		return nil, err
	}
	if _, ok := fn.(star.Callable); !ok {
		return nil, withMsg("assert.fails: argument is not callable", msg)
	}
	if _, err := star.Call(thread, fn, nil, nil); err == nil {
		return nil, withMsg("assert.fails: expected the call to fail, but it succeeded", msg)
	}
	return star.None, nil
}

func assertFail(_ *star.Thread, _ *star.Builtin, args star.Tuple, kwargs []star.Tuple) (star.Value, error) {
	var msg string
	if err := star.UnpackArgs("assert.fail", args, kwargs, "msg?", &msg); err != nil {
		return nil, err
	}
	if msg == "" {
		msg = "explicit failure"
	}
	return nil, fmt.Errorf("assert.fail: %s", msg)
}
