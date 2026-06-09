// SPDX-License-Identifier: Apache-2.0

package statemgmt

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse_Empty(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"zero bytes":         "",
		"whitespace only":    "   \n\n  ",
		"explicit empty doc": "---\n",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			sf, err := Parse([]byte(input))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if sf == nil {
				t.Fatal("Parse returned nil StateFile")
				return
			}
			if sf.Metadata.Name != "" || sf.Metadata.Version != "" {
				t.Errorf("Metadata = %+v, want zero", sf.Metadata)
			}
			if len(sf.Includes) != 0 || len(sf.Variables) != 0 || len(sf.Declarations) != 0 {
				t.Errorf("expected zero-value StateFile, got %+v", sf)
			}
		})
	}
}

func TestParse_MetadataOnly(t *testing.T) {
	t.Parallel()
	sf, err := Parse([]byte("metadata:\n  name: foo\n  version: \"1.2\"\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if sf.Metadata.Name != "foo" {
		t.Errorf("Name = %q, want foo", sf.Metadata.Name)
	}
	if sf.Metadata.Version != "1.2" {
		t.Errorf("Version = %q, want 1.2", sf.Metadata.Version)
	}
	if len(sf.Declarations) != 0 {
		t.Errorf("expected no declarations, got %d", len(sf.Declarations))
	}
}

func TestParse_VariablesOnly(t *testing.T) {
	t.Parallel()
	in := `
variables:
  user: www-data
  port: 8080
  enabled: true
`
	sf, err := Parse([]byte(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if sf.Variables["user"] != "www-data" {
		t.Errorf("Variables[user] = %v, want www-data", sf.Variables["user"])
	}
	if sf.Variables["port"] != 8080 {
		t.Errorf("Variables[port] = %v (%T), want int 8080", sf.Variables["port"], sf.Variables["port"])
	}
	if sf.Variables["enabled"] != true {
		t.Errorf("Variables[enabled] = %v, want true", sf.Variables["enabled"])
	}
}

func TestParse_IncludesOnly(t *testing.T) {
	t.Parallel()
	in := "includes:\n  - common/base.yaml\n  - common/security.yaml\n"
	sf, err := Parse([]byte(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []string{"common/base.yaml", "common/security.yaml"}
	if len(sf.Includes) != len(want) {
		t.Fatalf("Includes len = %d, want %d", len(sf.Includes), len(want))
	}
	for i, w := range want {
		if sf.Includes[i] != w {
			t.Errorf("Includes[%d] = %q, want %q", i, sf.Includes[i], w)
		}
	}
}

func TestParse_SingleResource(t *testing.T) {
	t.Parallel()
	in := `
file:
  /etc/hosts:
    state: present
    user: root
    mode: "0644"
`
	sf, err := Parse([]byte(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(sf.Declarations) != 1 {
		t.Fatalf("Declarations len = %d, want 1", len(sf.Declarations))
	}
	d := sf.Declarations[0]
	if d.ID != "file:/etc/hosts" {
		t.Errorf("ID = %q, want file:/etc/hosts", d.ID)
	}
	if d.Module != "file" {
		t.Errorf("Module = %q, want file", d.Module)
	}
	if d.Name != "/etc/hosts" {
		t.Errorf("Name = %q, want /etc/hosts", d.Name)
	}
	if d.State != "present" {
		t.Errorf("State = %q, want present", d.State)
	}
	if _, lingers := d.Params["state"]; lingers {
		t.Error("state key should be promoted out of Params")
	}
	if d.Params["user"] != "root" {
		t.Errorf("Params[user] = %v, want root", d.Params["user"])
	}
	if d.Params["mode"] != "0644" {
		t.Errorf("Params[mode] = %v, want \"0644\"", d.Params["mode"])
	}
}

func TestParse_WebserverFixture(t *testing.T) {
	t.Parallel()
	data := readFixture(t, "webserver.yaml")
	sf, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if sf.Metadata.Name != "webserver-setup" {
		t.Errorf("Metadata.Name = %q, want webserver-setup", sf.Metadata.Name)
	}
	if len(sf.Includes) != 2 {
		t.Errorf("Includes len = %d, want 2", len(sf.Includes))
	}
	if len(sf.Variables) != 2 {
		t.Errorf("Variables len = %d, want 2", len(sf.Variables))
	}
	if len(sf.Declarations) != 3 {
		t.Fatalf("Declarations len = %d, want 3", len(sf.Declarations))
	}
	wantIDs := []string{
		"package:nginx",
		"file:/etc/nginx/nginx.conf",
		"service:nginx",
	}
	for i, want := range wantIDs {
		if sf.Declarations[i].ID != want {
			t.Errorf("Declarations[%d].ID = %q, want %q", i, sf.Declarations[i].ID, want)
		}
	}
	// Requisites must round-trip opaquely — the parser does not
	// interpret them; the resolver (Task 5) does. Just verify they
	// landed in Params untouched.
	fileDecl := sf.Declarations[1]
	if _, ok := fileDecl.Params["require"]; !ok {
		t.Error("file declaration: require should be preserved in Params")
	}
	if _, ok := fileDecl.Params["watch"]; !ok {
		t.Error("file declaration: watch should be preserved in Params")
	}
}

func TestParse_PreservesSourceOrder(t *testing.T) {
	t.Parallel()
	data := readFixture(t, "order.yaml")
	sf, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	wantIDs := []string{
		// service section first
		"service:zeta",
		"service:alpha",
		// package section second
		"package:middle",
		// file section third
		"file:/tmp/last",
		"file:/tmp/first",
	}
	if len(sf.Declarations) != len(wantIDs) {
		t.Fatalf("Declarations len = %d, want %d", len(sf.Declarations), len(wantIDs))
	}
	for i, want := range wantIDs {
		if sf.Declarations[i].ID != want {
			t.Errorf("Declarations[%d].ID = %q, want %q", i, sf.Declarations[i].ID, want)
		}
	}
}

func TestParse_MissingStateLeavesEmpty(t *testing.T) {
	t.Parallel()
	in := "package:\n  nginx:\n    version: \"1.20\"\n"
	sf, err := Parse([]byte(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(sf.Declarations) != 1 {
		t.Fatalf("Declarations len = %d, want 1", len(sf.Declarations))
	}
	if sf.Declarations[0].State != "" {
		t.Errorf("State = %q, want empty (validator's problem, not parser's)", sf.Declarations[0].State)
	}
}

func TestParse_StateMustBeString(t *testing.T) {
	t.Parallel()
	in := "package:\n  nginx:\n    state: 42\n"
	_, err := Parse([]byte(in))
	if err == nil {
		t.Fatal("Parse should reject non-string state")
	}
	if !strings.Contains(err.Error(), "state must be a string") {
		t.Errorf("err = %v, want mention of \"state must be a string\"", err)
	}
}

func TestParse_RejectsNonMappingTopLevel(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"sequence": "- foo\n- bar\n",
		"scalar":   "hello\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(in))
			if err == nil {
				t.Fatalf("Parse should reject top-level %s", name)
			}
		})
	}
}

func TestParse_TopLevelNullIsEmpty(t *testing.T) {
	t.Parallel()
	// YAML "null" at the top level is a valid empty document — we
	// treat it the same as zero bytes rather than rejecting it, so
	// templates that render to nothing still parse.
	sf, err := Parse([]byte("null\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(sf.Declarations) != 0 {
		t.Errorf("expected empty StateFile, got %+v", sf)
	}
}

func TestParse_RejectsMalformedSections(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"metadata not a map":         "metadata: \"foo\"\n",
		"variables not a map":        "variables:\n  - one\n  - two\n",
		"includes not a sequence":    "includes: nope\n",
		"includes entry not string":  "includes:\n  - {nested: map}\n",
		"module section not a map":   "file: \"hello\"\n",
		"resource body not a map":    "file:\n  /etc/hosts: just-a-string\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(in))
			if err == nil {
				t.Fatalf("Parse should reject %s", name)
			}
		})
	}
}

func TestParse_MalformedYAML(t *testing.T) {
	t.Parallel()
	_, err := Parse([]byte("file:\n  /etc/hosts: {state: present\n"))
	if err == nil {
		t.Fatal("Parse should reject malformed YAML")
	}
	// The error must surface as a parse error of some shape; we do
	// not pin the exact text since it comes from upstream yaml.v3.
	if !strings.Contains(err.Error(), "statemgmt: parse") {
		t.Errorf("err = %v, want wrapped statemgmt: parse: prefix", err)
	}
}

func TestParse_ErrorIncludesLineNumber(t *testing.T) {
	t.Parallel()
	// Resource body on line 3 is a scalar, not a mapping.
	in := "file:\n  /etc/hosts:\n    just-a-string\n"
	_, err := Parse([]byte(in))
	if err == nil {
		t.Fatal("Parse should reject scalar resource body")
	}
	if !strings.Contains(err.Error(), "line ") {
		t.Errorf("err = %v, want \"line N\" prefix", err)
	}
}

// Defensive: errors must wrap nothing fancy from the yaml lib that
// the caller would have to type-assert; statemgmt: parse: prefix is
// the contract.
func TestParse_ErrorWrapping(t *testing.T) {
	t.Parallel()
	_, err := Parse([]byte("metadata: 1\n"))
	if err == nil {
		t.Fatal("Parse should fail")
	}
	if !strings.HasPrefix(err.Error(), "statemgmt: parse:") {
		t.Errorf("err = %v, want \"statemgmt: parse:\" prefix", err)
	}
	if errors.Is(err, ErrModuleNotFound) {
		t.Error("parse errors must not collide with registry sentinels")
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return data
}
