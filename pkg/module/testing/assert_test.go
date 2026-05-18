package moduletest

import (
	"strings"
	"testing"

	star "go.starlark.net/starlark"
)

// callStar execs body (with `assert` predeclared) and calls fn().
func callStar(t *testing.T, body, fn string) error {
	t.Helper()
	g, err := star.ExecFileOptions(strictOptions, &star.Thread{Name: "t"},
		"t.star", body, star.StringDict{"assert": assertNS})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	_, cerr := star.Call(&star.Thread{Name: "t"}, g[fn], nil, nil)
	return cerr
}

func TestAssertHelpers(t *testing.T) {
	cases := []struct {
		name, body, fn string
		wantErr        string // "" => must succeed
	}{
		{"eq ok", "def f():\n    assert.eq(1, 1)\n", "f", ""},
		{"ne ok", "def f():\n    assert.ne(1, 2)\n", "f", ""},
		{"ne fail", "def f():\n    assert.ne(1, 1)\n", "f", "both"},
		{"false ok", "def f():\n    assert.false(0)\n", "f", ""},
		{"true fail", "def f():\n    assert.true(0, 'msg')\n", "f", "msg"},
		{"false fail", "def f():\n    assert.false(1)\n", "f", "truthy"},
		{"contains str ok", "def f():\n    assert.contains('abc', 'b')\n", "f", ""},
		{"contains str miss", "def f():\n    assert.contains('abc', 'z')\n", "f", "not in"},
		{"contains str bad item", "def f():\n    assert.contains('abc', 1)\n", "f", "needs a string"},
		{"contains list ok", "def f():\n    assert.contains([1, 2, 3], 2)\n", "f", ""},
		{"contains list miss", "def f():\n    assert.contains([1, 2], 9)\n", "f", "not found"},
		{"contains not iterable", "def f():\n    assert.contains(5, 1)\n", "f", "not iterable"},
		{"fails ok", "def f():\n    assert.fails(lambda: fail('boom'))\n", "f", ""},
		{"fails but succeeds", "def f():\n    assert.fails(lambda: 1)\n", "f", "expected the call to fail"},
		{"fails not callable", "def f():\n    assert.fails(5)\n", "f", "not callable"},
		{"fail default msg", "def f():\n    assert.fail()\n", "f", "explicit failure"},
		{"fail custom", "def f():\n    assert.fail('nope')\n", "f", "nope"},
		{"eq unpack error", "def f():\n    assert.eq(1)\n", "f", "missing argument"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := callStar(t, c.body, c.fn)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("want success, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("err = %v, want substring %q", err, c.wantErr)
			}
		})
	}
}

func TestAssertNamespaceValue(t *testing.T) {
	if assertNS.Type() != "assert" || !strings.Contains(assertNS.String(), "assert") {
		t.Fatalf("Type/String: %s / %s", assertNS.Type(), assertNS.String())
	}
	if !bool(assertNS.Truth()) {
		t.Fatal("Truth should be true")
	}
	if _, err := assertNS.Hash(); err == nil {
		t.Fatal("namespace must be unhashable")
	}
	names := assertNS.AttrNames()
	if len(names) != 7 || names[0] != "contains" {
		t.Fatalf("AttrNames = %v", names)
	}
	a, err := assertNS.Attr("eq")
	if err != nil || a == nil {
		t.Fatalf("Attr(eq) = %v, %v", a, err)
	}
	miss, err := assertNS.Attr("nope")
	if err != nil || miss != nil {
		t.Fatalf("Attr(nope) = %v, %v; want nil,nil", miss, err)
	}
}
