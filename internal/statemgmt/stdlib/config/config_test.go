// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func decl(name, state string, params map[string]any) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     "config:" + name,
		Module: "config",
		State:  state,
		Name:   name,
		Params: params,
	}
}

func writeTmp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "cfg")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// --- params / validate ------------------------------------------------

func TestParse_UnknownKey(t *testing.T) {
	t.Parallel()
	if _, err := parseParams(decl("/c", StatePresent, map[string]any{"keys": "x"})); err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestParse_ValueCoercion(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw  any
		want string
	}{
		{"hi", "hi"},
		{1024, "1024"},
		{int64(7), "7"},
		{true, "true"},
		{false, "false"},
		{float64(3), "3"},     // integral float renders clean
		{float64(2.5), "2.5"}, // genuine float
	}
	for _, c := range cases {
		p, err := parseParams(decl("/c", StatePresent, map[string]any{"key": "k", "value": c.raw}))
		if err != nil {
			t.Errorf("value %v: %v", c.raw, err)
			continue
		}
		if p.Value != c.want {
			t.Errorf("value %v → %q, want %q", c.raw, p.Value, c.want)
		}
	}
	// unsupported type
	if _, err := parseParams(decl("/c", StatePresent, map[string]any{"key": "k", "value": []any{1}})); err == nil {
		t.Error("a list value should be rejected")
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		d       *statemgmt.Declaration
		wantErr bool
	}{
		{"present ok keyvalue", decl("/c", StatePresent, map[string]any{"key": "k", "value": "v"}), false},
		{"present ok ini", decl("/c", StatePresent, map[string]any{"key": "k", "value": "v", "format": "ini", "section": "core"}), false},
		{"present needs value", decl("/c", StatePresent, map[string]any{"key": "k"}), true},
		{"present needs key", decl("/c", StatePresent, map[string]any{"value": "v"}), true},
		{"bad format", decl("/c", StatePresent, map[string]any{"key": "k", "value": "v", "format": "toml"}), true},
		{"key with '='", decl("/c", StatePresent, map[string]any{"key": "a=b", "value": "v"}), true},
		{"key with newline", decl("/c", StatePresent, map[string]any{"key": "a\nb", "value": "v"}), true},
		{"key with leading ws", decl("/c", StatePresent, map[string]any{"key": " k", "value": "v"}), true},
		{"key starts with #", decl("/c", StatePresent, map[string]any{"key": "#k", "value": "v"}), true},
		{"key starts with [", decl("/c", StatePresent, map[string]any{"key": "[k", "value": "v"}), true},
		{"multiline value", decl("/c", StatePresent, map[string]any{"key": "k", "value": "a\nb"}), true},
		{"section without ini", decl("/c", StatePresent, map[string]any{"key": "k", "value": "v", "section": "core"}), true},
		{"absent ok", decl("/c", StateAbsent, map[string]any{"key": "k"}), false},
		{"absent ok ini section", decl("/c", StateAbsent, map[string]any{"key": "k", "format": "ini", "section": "core"}), false},
		{"absent rejects value", decl("/c", StateAbsent, map[string]any{"key": "k", "value": "v"}), true},
		{"absent rejects space_around", decl("/c", StateAbsent, map[string]any{"key": "k", "space_around_separator": true}), true},
		{"bad state", decl("/c", "frob", map[string]any{"key": "k"}), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := parseParams(tc.d)
			if err == nil {
				err = p.validate()
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

// --- format.go (pure) -------------------------------------------------

func TestFormat_ParseKV(t *testing.T) {
	t.Parallel()
	cases := []struct {
		line string
		ok   bool
		kv   parsedKV
	}{
		{"k=v", true, parsedKV{lead: "", key: "k", sep: "=", value: "v"}},
		{"  k = v  ", true, parsedKV{lead: "  ", key: "k", sep: " = ", value: "v"}},
		{"k=a=b", true, parsedKV{lead: "", key: "k", sep: "=", value: "a=b"}},
		{"PATH=/x:/y", true, parsedKV{lead: "", key: "PATH", sep: "=", value: "/x:/y"}},
		{"# comment", false, parsedKV{}},
		{"; comment", false, parsedKV{}},
		{"", false, parsedKV{}},
		{"   ", false, parsedKV{}},
		{"[section]", false, parsedKV{}},
		{"no equals here", false, parsedKV{}},
		{"=novalue", false, parsedKV{}},
	}
	for _, c := range cases {
		got, ok := parseKV(c.line)
		if ok != c.ok {
			t.Errorf("parseKV(%q): ok=%v want %v", c.line, ok, c.ok)
			continue
		}
		if ok && got != c.kv {
			t.Errorf("parseKV(%q) = %+v, want %+v", c.line, got, c.kv)
		}
	}
}

func TestFormat_KeyValue_GetSetDel(t *testing.T) {
	t.Parallel()
	base := "# header comment\nfoo=1\nbar = 2\n\n# trailing comment\n"

	// get
	if v, ok := get(base, false, "", "foo"); !ok || v != "1" {
		t.Errorf("get foo = %q,%v", v, ok)
	}
	if _, ok := get(base, false, "", "nope"); ok {
		t.Error("get of a missing key should be false")
	}

	// set existing — replaces value in place, preserving "bar = " style
	c1, ch := set(base, false, "", "bar", "99", false)
	if !ch {
		t.Fatal("set should report a change")
	}
	if !strings.Contains(c1, "bar = 99\n") {
		t.Errorf("set didn't preserve spacing: %q", c1)
	}
	if !strings.Contains(c1, "foo=1\n") || !strings.Contains(c1, "# header comment") || !strings.Contains(c1, "# trailing comment") {
		t.Errorf("set clobbered other content: %q", c1)
	}
	// set to the same value → no change
	if _, ch := set(c1, false, "", "bar", "99", false); ch {
		t.Error("set to the same value should report no change")
	}

	// set new key — appended at EOF, default no spaces
	c2, ch := set(base, false, "", "baz", "3", false)
	if !ch || !strings.Contains(c2, "baz=3\n") {
		t.Errorf("set new key: ch=%v content=%q", ch, c2)
	}
	// set new key with space_around
	c3, _ := set(base, false, "", "baz", "3", true)
	if !strings.Contains(c3, "baz = 3\n") {
		t.Errorf("space_around new key: %q", c3)
	}

	// del existing — removes all occurrences
	dup := "x=1\nfoo=a\nfoo=b\ny=2\n"
	c4, ch := del(dup, false, "", "foo")
	if !ch {
		t.Fatal("del should report a change")
	}
	if strings.Contains(c4, "foo=") {
		t.Errorf("del left a foo line: %q", c4)
	}
	if !strings.Contains(c4, "x=1") || !strings.Contains(c4, "y=2") {
		t.Errorf("del clobbered other keys: %q", c4)
	}
	// del missing → unchanged
	if _, ch := del(base, false, "", "ghost"); ch {
		t.Error("del of a missing key should report no change")
	}

	// set into an empty string → fresh line, trailing newline
	c5, _ := set("", false, "", "k", "v", false)
	if c5 != "k=v\n" {
		t.Errorf("set into empty: %q", c5)
	}
}

func TestFormat_INI_GetSetDel(t *testing.T) {
	t.Parallel()
	base := "top=0\n[core]\nname=keystone\n[net]\nport=8080\n"

	// get within a section
	if v, ok := get(base, true, "core", "name"); !ok || v != "keystone" {
		t.Errorf("get [core] name = %q,%v", v, ok)
	}
	// the same key name doesn't bleed across sections
	if _, ok := get(base, true, "net", "name"); ok {
		t.Error("'name' should not be found in [net]")
	}
	// implicit top section
	if v, ok := get(base, true, "", "top"); !ok || v != "0" {
		t.Errorf("get top-section key = %q,%v", v, ok)
	}

	// set existing within a section
	c1, ch := set(base, true, "net", "port", "9090", false)
	if !ch || !strings.Contains(c1, "port=9090\n") {
		t.Errorf("set [net] port: ch=%v content=%q", ch, c1)
	}

	// set new key into an existing section — lands inside that section
	c2, ch := set(base, true, "core", "owner", "alice", false)
	if !ch {
		t.Fatal("set new ini key should change")
	}
	// "owner=alice" must appear between "[core]" and "[net]"
	coreIdx := strings.Index(c2, "[core]")
	netIdx := strings.Index(c2, "[net]")
	ownerIdx := strings.Index(c2, "owner=alice")
	if coreIdx >= ownerIdx || ownerIdx >= netIdx {
		t.Errorf("new key not placed inside [core]:\n%s", c2)
	}

	// set new key into a missing section — header created at EOF
	c3, ch := set(base, true, "logging", "level", "debug", false)
	if !ch || !strings.Contains(c3, "[logging]\nlevel=debug\n") {
		t.Errorf("missing-section create: ch=%v content=%q", ch, c3)
	}

	// del within a section leaves the header in place
	c4, ch := del(base, true, "core", "name")
	if !ch || strings.Contains(c4, "name=keystone") {
		t.Errorf("del [core] name: ch=%v content=%q", ch, c4)
	}
	if !strings.Contains(c4, "[core]") {
		t.Error("del should keep the [core] header")
	}

	// set into empty + ini section → header + line
	c5, _ := set("", true, "main", "k", "v", false)
	if c5 != "[main]\nk=v\n" {
		t.Errorf("set into empty ini: %q", c5)
	}
}

// --- Check / Apply (on-disk) ------------------------------------------

func TestCheckApply_KeyValue(t *testing.T) {
	t.Parallel()
	path := writeTmp(t, "# config\nalpha=1\nbeta = old\n")
	m := New()
	d := decl(path, StatePresent, map[string]any{"key": "beta", "value": "new"})

	r, err := m.Check(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if r.Matches {
		t.Error("beta=old vs new should drift")
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed {
		t.Error("apply should change")
	}
	got := read(t, path)
	if !strings.Contains(got, "beta = new\n") || !strings.Contains(got, "alpha=1\n") || !strings.Contains(got, "# config") {
		t.Errorf("file after apply: %q", got)
	}

	// idempotent
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Error("should match after apply")
	}
	sr, _ = m.Apply(context.Background(), d)
	if sr.Changed || sr.Comment != "already converged" {
		t.Errorf("second apply: changed=%v comment=%q", sr.Changed, sr.Comment)
	}

	// add a new key
	d2 := decl(path, StatePresent, map[string]any{"key": "gamma", "value": "3"})
	if _, err := m.Apply(context.Background(), d2); err != nil {
		t.Fatal(err)
	}
	if v, ok := get(read(t, path), false, "", "gamma"); !ok || v != "3" {
		t.Errorf("gamma not added: %q,%v", v, ok)
	}
}

func TestCheckApply_INI(t *testing.T) {
	t.Parallel()
	path := writeTmp(t, "[server]\nport = 80\n[db]\nname = app\n")
	m := New()
	d := decl(path, StatePresent, map[string]any{"key": "port", "value": "443", "format": "ini", "section": "server"})

	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Error("port 80 vs 443 should drift")
	}
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	got := read(t, path)
	if !strings.Contains(got, "port = 443\n") || !strings.Contains(got, "name = app\n") {
		t.Errorf("ini after apply: %q", got)
	}
	r, _ = m.Check(context.Background(), d)
	if !r.Matches {
		t.Error("should match after apply")
	}
}

func TestApply_CreatesMissingFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "new.conf")
	m := New()

	// present with default create:true → file created with our line
	d := decl(path, StatePresent, map[string]any{"key": "enabled", "value": "yes"})
	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Error("missing file should drift for present")
	}
	if _, err := m.Apply(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if read(t, path) != "enabled=yes\n" {
		t.Errorf("created file: %q", read(t, path))
	}

	// present with create:false on a missing file → error
	path2 := filepath.Join(dir, "absent.conf")
	d2 := decl(path2, StatePresent, map[string]any{"key": "k", "value": "v", "create": false})
	if _, err := m.Apply(context.Background(), d2); err == nil {
		t.Error("create:false on a missing file should error")
	}
	if _, statErr := os.Stat(path2); !os.IsNotExist(statErr) {
		t.Error("create:false should not have created the file")
	}

	// ini create → header + line
	path3 := filepath.Join(dir, "ini.conf")
	d3 := decl(path3, StatePresent, map[string]any{"key": "level", "value": "info", "format": "ini", "section": "log"})
	if _, err := m.Apply(context.Background(), d3); err != nil {
		t.Fatal(err)
	}
	if read(t, path3) != "[log]\nlevel=info\n" {
		t.Errorf("created ini: %q", read(t, path3))
	}
}

func TestCheckApply_Absent(t *testing.T) {
	t.Parallel()
	path := writeTmp(t, "keep=1\ndrop=2\ndrop=3\nkeep2=4\n")
	m := New()
	d := decl(path, StateAbsent, map[string]any{"key": "drop"})

	r, _ := m.Check(context.Background(), d)
	if r.Matches {
		t.Error("present key should drift from absent")
	}
	sr, err := m.Apply(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if !sr.Changed {
		t.Error("removal should change")
	}
	got := read(t, path)
	if strings.Contains(got, "drop=") {
		t.Errorf("not all 'drop' lines removed: %q", got)
	}
	if !strings.Contains(got, "keep=1") || !strings.Contains(got, "keep2=4") {
		t.Errorf("removal clobbered other keys: %q", got)
	}

	// already absent → no-op
	sr, _ = m.Apply(context.Background(), d)
	if sr.Changed {
		t.Error("absent on a missing key should be a no-op")
	}

	// absent on a non-existent file → match, no-op
	missing := filepath.Join(t.TempDir(), "nope")
	r, _ = m.Check(context.Background(), decl(missing, StateAbsent, map[string]any{"key": "x"}))
	if !r.Matches {
		t.Error("absent on a missing file should match")
	}
	sr, _ = m.Apply(context.Background(), decl(missing, StateAbsent, map[string]any{"key": "x"}))
	if sr.Changed {
		t.Error("absent apply on a missing file should be a no-op")
	}
	if _, statErr := os.Stat(missing); !os.IsNotExist(statErr) {
		t.Error("absent apply must not create the file")
	}
}

func TestApply_PreservesFileMode(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "cfg")
	if err := os.WriteFile(path, []byte("k=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New().Apply(context.Background(), decl(path, StatePresent, map[string]any{"key": "k", "value": "2"})); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode after apply = %o, want 0600", fi.Mode().Perm())
	}
}

func TestApply_ReadError(t *testing.T) {
	t.Parallel()
	// a directory in place of the config file → ReadFile errors
	// (not a "not exist" error).
	dir := t.TempDir()
	if _, err := New().Check(context.Background(), decl(dir, StatePresent, map[string]any{"key": "k", "value": "v"})); err == nil {
		t.Error("reading a directory as a config file should error")
	}
}

// --- module surface ----------------------------------------------------

func TestModuleSurface(t *testing.T) {
	t.Parallel()
	m := New()
	if m.Name() != "config" {
		t.Errorf("Name=%q", m.Name())
	}
	if got := m.ValidStates(); len(got) != 2 || got[0] != StatePresent || got[1] != StateAbsent {
		t.Errorf("ValidStates=%v", got)
	}
	if _, ok := m.(statemgmt.ValidatableModule); !ok {
		t.Error("config should implement ValidatableModule")
	}
	dsm := m.(statemgmt.DriftSeverityModule)
	if dsm.DriftSeverity(decl("/c", StateAbsent, map[string]any{"key": "k"}), nil) != statemgmt.DriftSeverityHigh {
		t.Error("absent drift → HIGH")
	}
	if dsm.DriftSeverity(decl("/c", StatePresent, map[string]any{"key": "k", "value": "v"}), nil) != statemgmt.DriftSeverityMedium {
		t.Error("present drift → MEDIUM")
	}
	if dsm.DriftSeverity(nil, nil) != statemgmt.DriftSeverityMedium {
		t.Error("nil decl → MEDIUM")
	}
	vm := m.(statemgmt.ValidatableModule)
	if err := vm.Validate(decl("/c", StatePresent, map[string]any{"key": "k", "value": "v"})); err != nil {
		t.Errorf("valid decl rejected: %v", err)
	}
	if err := vm.Validate(decl("/c", StatePresent, map[string]any{"key": "k"})); err == nil {
		t.Error("present-without-value should be rejected")
	}

	// Test() round-trip
	path := writeTmp(t, "k=v\n")
	ok, err := m.Test(context.Background(), decl(path, StatePresent, map[string]any{"key": "k", "value": "v"}))
	if err != nil || !ok {
		t.Errorf("Test on a converged key: ok=%v err=%v", ok, err)
	}
	ok, err = m.Test(context.Background(), decl(path, StatePresent, map[string]any{"key": "k", "value": "other"}))
	if err != nil || ok {
		t.Errorf("Test on a drifted key should be false: ok=%v err=%v", ok, err)
	}
}
