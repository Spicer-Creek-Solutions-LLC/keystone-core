package statemgmt

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
)

func TestLoader_Load_SingleFile(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"root.yaml": &fstest.MapFile{Data: []byte(`
metadata: {name: root, version: "1.0"}
variables: {a: 1}
package:
  nginx: {state: installed}
`)},
	}
	sf, err := NewLoader(fsys).Load("root.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if sf.Metadata.Name != "root" {
		t.Errorf("Metadata.Name = %q, want root", sf.Metadata.Name)
	}
	if len(sf.Declarations) != 1 || sf.Declarations[0].ID != "package:nginx" {
		t.Errorf("Declarations = %+v, want one package:nginx", sf.Declarations)
	}
	if sf.Includes != nil {
		t.Errorf("Includes should be zeroed after expansion, got %v", sf.Includes)
	}
	if sf.Variables["a"] != 1 {
		t.Errorf("Variables[a] = %v, want 1", sf.Variables["a"])
	}
}

func TestLoader_Load_LinearChain(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"root.yaml": &fstest.MapFile{Data: []byte(`
includes: [a.yaml]
package: {root-pkg: {state: installed}}
`)},
		"a.yaml": &fstest.MapFile{Data: []byte(`
includes: [b.yaml]
package: {a-pkg: {state: installed}}
`)},
		"b.yaml": &fstest.MapFile{Data: []byte(`
package: {b-pkg: {state: installed}}
`)},
	}
	sf, err := NewLoader(fsys).Load("root.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Preorder includes-first: b → a → root.
	wantIDs := []string{"package:b-pkg", "package:a-pkg", "package:root-pkg"}
	if len(sf.Declarations) != len(wantIDs) {
		t.Fatalf("Declarations len = %d, want %d", len(sf.Declarations), len(wantIDs))
	}
	for i, want := range wantIDs {
		if sf.Declarations[i].ID != want {
			t.Errorf("Declarations[%d].ID = %q, want %q", i, sf.Declarations[i].ID, want)
		}
	}
}

func TestLoader_Load_Diamond_ParsesEachLeafOnce(t *testing.T) {
	t.Parallel()
	// root → {a, b} → c. c must be parsed once; declarations from
	// c appear once in the merged tree even though a and b each
	// claim them as their first include.
	files := map[string]string{
		"root.yaml": `
includes: [a.yaml, b.yaml]
package: {root-pkg: {state: installed}}
`,
		"a.yaml": `
includes: [c.yaml]
package: {a-pkg: {state: installed}}
`,
		"b.yaml": `
includes: [c.yaml]
package: {b-pkg: {state: installed}}
`,
		"c.yaml": `
variables: {shared: true}
`,
	}
	counter := newCountingFS(files)
	_, err := NewLoader(counter).Load("root.yaml")
	// Diamond loads with duplicate declarations cause an error;
	// here c contributes no declarations so the load should
	// succeed and c should be parsed exactly once.
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := counter.opens("c.yaml"); got != 1 {
		t.Errorf("c.yaml opened %d times, want 1 (diamond should hit cache)", got)
	}
}

func TestLoader_Load_DiamondWithSharedDecls_Rejected(t *testing.T) {
	t.Parallel()
	// Diamond where the shared leaf c provides a declaration. Both
	// a and b would flatten c's declaration into themselves, so the
	// merge at root sees a duplicate. v1.0 has no extend/override —
	// reject.
	fsys := fstest.MapFS{
		"root.yaml": &fstest.MapFile{Data: []byte(`includes: [a.yaml, b.yaml]`)},
		"a.yaml":    &fstest.MapFile{Data: []byte(`includes: [c.yaml]`)},
		"b.yaml":    &fstest.MapFile{Data: []byte(`includes: [c.yaml]`)},
		"c.yaml":    &fstest.MapFile{Data: []byte(`package: {shared: {state: installed}}`)},
	}
	_, err := NewLoader(fsys).Load("root.yaml")
	if err == nil {
		t.Fatal("expected duplicate-ID error from diamond with shared declaration")
	}
	if !strings.Contains(err.Error(), `"package:shared"`) {
		t.Errorf("err = %v, want mention of declaration ID", err)
	}
	if !strings.Contains(err.Error(), "c.yaml") {
		t.Errorf("err = %v, want mention of c.yaml as the source", err)
	}
}

func TestLoader_Load_VariableLayering_RootWins(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"root.yaml": &fstest.MapFile{Data: []byte(`
includes: [a.yaml]
variables: {port: 443, owner: root}
`)},
		"a.yaml": &fstest.MapFile{Data: []byte(`
variables: {port: 80, only-in-a: true}
`)},
	}
	sf, err := NewLoader(fsys).Load("root.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if sf.Variables["port"] != 443 {
		t.Errorf("Variables[port] = %v, want 443 (root wins)", sf.Variables["port"])
	}
	if sf.Variables["owner"] != "root" {
		t.Errorf("Variables[owner] = %v, want root", sf.Variables["owner"])
	}
	if sf.Variables["only-in-a"] != true {
		t.Errorf("Variables[only-in-a] = %v, want true", sf.Variables["only-in-a"])
	}
}

func TestLoader_Load_VariableLayering_LaterSiblingWins(t *testing.T) {
	t.Parallel()
	// root includes [a, b]; both define `x`; b is later in the
	// list so b wins.
	fsys := fstest.MapFS{
		"root.yaml": &fstest.MapFile{Data: []byte(`includes: [a.yaml, b.yaml]`)},
		"a.yaml":    &fstest.MapFile{Data: []byte(`variables: {x: from-a}`)},
		"b.yaml":    &fstest.MapFile{Data: []byte(`variables: {x: from-b}`)},
	}
	sf, err := NewLoader(fsys).Load("root.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if sf.Variables["x"] != "from-b" {
		t.Errorf("Variables[x] = %v, want from-b (later sibling wins)", sf.Variables["x"])
	}
}

func TestLoader_Load_DuplicateID_RootAndInclude(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"root.yaml": &fstest.MapFile{Data: []byte(`
includes: [a.yaml]
package: {nginx: {state: installed}}
`)},
		"a.yaml": &fstest.MapFile{Data: []byte(`
package: {nginx: {state: latest}}
`)},
	}
	_, err := NewLoader(fsys).Load("root.yaml")
	if err == nil {
		t.Fatal("expected duplicate-ID error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "a.yaml") || !strings.Contains(msg, "root.yaml") {
		t.Errorf("err = %v, want both file paths cited", err)
	}
	if !strings.Contains(msg, `"package:nginx"`) {
		t.Errorf("err = %v, want declaration ID cited", err)
	}
}

func TestLoader_Load_Cycle_Direct(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"root.yaml": &fstest.MapFile{Data: []byte(`includes: [root.yaml]`)},
	}
	_, err := NewLoader(fsys).Load("root.yaml")
	if err == nil {
		t.Fatal("expected include cycle error")
	}
	if !strings.Contains(err.Error(), "include cycle") {
		t.Errorf("err = %v, want \"include cycle\" prefix", err)
	}
	if !strings.Contains(err.Error(), "root.yaml → root.yaml") {
		t.Errorf("err = %v, want \"root.yaml → root.yaml\" in cycle path", err)
	}
}

func TestLoader_Load_Cycle_Indirect(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"root.yaml": &fstest.MapFile{Data: []byte(`includes: [a.yaml]`)},
		"a.yaml":    &fstest.MapFile{Data: []byte(`includes: [b.yaml]`)},
		"b.yaml":    &fstest.MapFile{Data: []byte(`includes: [a.yaml]`)},
	}
	_, err := NewLoader(fsys).Load("root.yaml")
	if err == nil {
		t.Fatal("expected include cycle error")
	}
	if !strings.Contains(err.Error(), "a.yaml → b.yaml → a.yaml") {
		t.Errorf("err = %v, want full cycle chain a.yaml → b.yaml → a.yaml", err)
	}
}

func TestLoader_Load_MissingInclude(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"root.yaml": &fstest.MapFile{Data: []byte(`includes: [ghost.yaml]`)},
	}
	_, err := NewLoader(fsys).Load("root.yaml")
	if err == nil {
		t.Fatal("expected missing-file error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "root.yaml") {
		t.Errorf("err = %v, want referencing file cited", err)
	}
	if !strings.Contains(msg, "ghost.yaml") {
		t.Errorf("err = %v, want missing path cited", err)
	}
}

func TestLoader_Load_RelativePathResolution(t *testing.T) {
	t.Parallel()
	// sub/x.yaml includes "helper.yaml" — should resolve to
	// sub/helper.yaml, not the bare top-level "helper.yaml".
	fsys := fstest.MapFS{
		"sub/x.yaml":      &fstest.MapFile{Data: []byte(`includes: [helper.yaml]`)},
		"sub/helper.yaml": &fstest.MapFile{Data: []byte(`variables: {from: sub}`)},
		"helper.yaml":     &fstest.MapFile{Data: []byte(`variables: {from: top}`)},
	}
	sf, err := NewLoader(fsys).Load("sub/x.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if sf.Variables["from"] != "sub" {
		t.Errorf("Variables[from] = %v, want sub (relative resolution)", sf.Variables["from"])
	}
}

func TestLoader_Load_ParseErrorInInclude(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"root.yaml": &fstest.MapFile{Data: []byte(`includes: [broken.yaml]`)},
		"broken.yaml": &fstest.MapFile{Data: []byte(`
package:
  nginx:
    state: 42
`)},
	}
	_, err := NewLoader(fsys).Load("root.yaml")
	if err == nil {
		t.Fatal("expected parse error to propagate")
	}
	if !strings.Contains(err.Error(), "broken.yaml") {
		t.Errorf("err = %v, want broken.yaml cited", err)
	}
}

func TestLoader_Load_MetadataOnlyFromRoot(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"root.yaml": &fstest.MapFile{Data: []byte(`
metadata: {name: root, version: "9.0"}
includes: [a.yaml]
`)},
		"a.yaml": &fstest.MapFile{Data: []byte(`
metadata: {name: included, version: "1.0"}
`)},
	}
	sf, err := NewLoader(fsys).Load("root.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if sf.Metadata.Name != "root" || sf.Metadata.Version != "9.0" {
		t.Errorf("Metadata = %+v, want {root 9.0} (included metadata must be dropped)", sf.Metadata)
	}
}

func TestLoader_Load_EmptyInclude(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"root.yaml":  &fstest.MapFile{Data: []byte(`includes: [empty.yaml]`)},
		"empty.yaml": &fstest.MapFile{Data: []byte(``)},
	}
	sf, err := NewLoader(fsys).Load("root.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(sf.Declarations) != 0 {
		t.Errorf("Declarations = %v, want none", sf.Declarations)
	}
}

func TestLoader_Load_RejectsPathTraversal(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"sub/x.yaml": &fstest.MapFile{Data: []byte(`includes: ["../escape.yaml"]`)},
	}
	_, err := NewLoader(fsys).Load("sub/x.yaml")
	if err == nil {
		t.Fatal("expected error for path-traversal include")
	}
}

func TestLoader_Load_NilFS(t *testing.T) {
	t.Parallel()
	_, err := (&Loader{}).Load("root.yaml")
	if err == nil {
		t.Fatal("expected error when FS is nil")
	}
	if !strings.Contains(err.Error(), "Loader.FS is nil") {
		t.Errorf("err = %v, want mention of nil FS", err)
	}
}

func TestLoader_Load_FromOSDirFS(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWrite(t, dir, "root.yaml", `
includes: [helper.yaml]
package: {root-pkg: {state: installed}}
`)
	mustWrite(t, dir, "helper.yaml", `
variables: {greeting: hi}
`)
	sf, err := NewLoader(os.DirFS(dir)).Load("root.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if sf.Variables["greeting"] != "hi" {
		t.Errorf("Variables[greeting] = %v, want hi", sf.Variables["greeting"])
	}
	if len(sf.Declarations) != 1 {
		t.Errorf("Declarations len = %d, want 1", len(sf.Declarations))
	}
}

// countingFS wraps an fs.FS and counts how many times each file is
// opened. Used to assert diamond-load caching.
type countingFS struct {
	inner  fs.FS
	counts map[string]*atomic.Int64
}

func newCountingFS(files map[string]string) *countingFS {
	m := fstest.MapFS{}
	c := &countingFS{counts: map[string]*atomic.Int64{}}
	for name, body := range files {
		m[name] = &fstest.MapFile{Data: []byte(body)}
		c.counts[name] = new(atomic.Int64)
	}
	c.inner = m
	return c
}

func (c *countingFS) Open(name string) (fs.File, error) {
	if counter, ok := c.counts[name]; ok {
		counter.Add(1)
	}
	return c.inner.Open(name)
}

func (c *countingFS) opens(name string) int64 {
	if counter, ok := c.counts[name]; ok {
		return counter.Load()
	}
	return 0
}

func mustWrite(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(strings.TrimSpace(body)+"\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// Smoke: cycle errors should not be misclassified as fs.ErrNotExist
// by the wrap chain (otherwise we'd produce a misleading "missing
// include" error).
func TestLoader_Load_CycleNotMistakenForNotExist(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"a.yaml": &fstest.MapFile{Data: []byte(`includes: [a.yaml]`)},
	}
	_, err := NewLoader(fsys).Load("a.yaml")
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, fs.ErrNotExist) {
		t.Errorf("cycle error must not match fs.ErrNotExist; got %v", err)
	}
}
