// SPDX-License-Identifier: Apache-2.0

package statemgmt

import (
	"reflect"
	"strings"
	"testing"
)

// roundTrip is the contract: Marshal then Parse yields an equivalent
// StateFile. Not a byte-identical document -- comments and key order
// are explicitly not preserved.
func roundTrip(t *testing.T, src string) (*StateFile, *StateFile) {
	t.Helper()
	original, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse(source): %v", err)
	}
	out, err := Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	reparsed, err := Parse(out)
	if err != nil {
		t.Fatalf("Parse(marshalled): %v\n--- marshalled ---\n%s", err, out)
	}
	return original, reparsed
}

func declByID(sf *StateFile) map[string]*Declaration {
	out := map[string]*Declaration{}
	for _, d := range sf.Declarations {
		out[d.Module+":"+d.Name] = d
	}
	return out
}

func TestMarshal_RoundTrip(t *testing.T) {
	const src = `metadata:
  name: app-stack
  version: "1.2"

variables:
  port: 8080
  name: web

file:
  /etc/app.env:
    state: present
    mode: "0644"
    content: |
      PORT=8080
  /etc/other.conf:
    state: absent

service:
  nginx:
    state: running
    enabled: true
`
	original, reparsed := roundTrip(t, src)

	if reparsed.Metadata != original.Metadata {
		t.Errorf("metadata = %+v, want %+v", reparsed.Metadata, original.Metadata)
	}
	if !reflect.DeepEqual(reparsed.Variables, original.Variables) {
		t.Errorf("variables = %#v, want %#v", reparsed.Variables, original.Variables)
	}
	if len(reparsed.Declarations) != len(original.Declarations) {
		t.Fatalf("declarations = %d, want %d", len(reparsed.Declarations), len(original.Declarations))
	}

	got, want := declByID(reparsed), declByID(original)
	for id, w := range want {
		g, ok := got[id]
		if !ok {
			t.Errorf("declaration %q lost in the round trip", id)
			continue
		}
		if g.Module != w.Module || g.Name != w.Name || g.State != w.State {
			t.Errorf("%s: got module=%q name=%q state=%q, want %q/%q/%q",
				id, g.Module, g.Name, g.State, w.Module, w.Name, w.State)
		}
		if !reflect.DeepEqual(g.Params, w.Params) {
			t.Errorf("%s params:\n got %#v\nwant %#v", id, g.Params, w.Params)
		}
	}
}

// The failure this guards against is subtle and would be found in
// production, not here: "0644" unquoted parses back as the integer
// 420, and a file mode silently becomes wrong.
func TestMarshal_PreservesStringyScalars(t *testing.T) {
	const src = `file:
  /etc/app.conf:
    state: present
    mode: "0644"
    version: "1.0"
    enabled: "true"
    count: "42"
    empty: ""
`
	_, reparsed := roundTrip(t, src)
	if len(reparsed.Declarations) != 1 {
		t.Fatalf("declarations = %d, want 1", len(reparsed.Declarations))
	}
	params := reparsed.Declarations[0].Params
	for key, want := range map[string]string{
		"mode": "0644", "version": "1.0", "enabled": "true", "count": "42", "empty": "",
	} {
		got, ok := params[key].(string)
		if !ok {
			t.Errorf("param %q = %#v (%T), want the string %q", key, params[key], params[key], want)
			continue
		}
		if got != want {
			t.Errorf("param %q = %q, want %q", key, got, want)
		}
	}
}

// Real types must stay their real types -- quoting everything would be
// as wrong as quoting nothing.
func TestMarshal_PreservesNonStringScalars(t *testing.T) {
	const src = `service:
  nginx:
    state: running
    enabled: true
    workers: 4
    ratio: 0.5
    absent_value: null
`
	_, reparsed := roundTrip(t, src)
	params := reparsed.Declarations[0].Params
	if v, ok := params["enabled"].(bool); !ok || !v {
		t.Errorf("enabled = %#v (%T), want bool true", params["enabled"], params["enabled"])
	}
	if v, ok := params["workers"].(int); !ok || v != 4 {
		t.Errorf("workers = %#v (%T), want int 4", params["workers"], params["workers"])
	}
	if v, ok := params["ratio"].(float64); !ok || v != 0.5 {
		t.Errorf("ratio = %#v (%T), want float64 0.5", params["ratio"], params["ratio"])
	}
	if params["absent_value"] != nil {
		t.Errorf("absent_value = %#v, want nil", params["absent_value"])
	}
}

func TestMarshal_NestedStructures(t *testing.T) {
	const src = `file:
  /etc/app.yaml:
    state: present
    require:
      - service: nginx
      - file: /etc/base.conf
    labels:
      env: "prod"
      tier: "web"
    ports:
      - 80
      - 443
`
	original, reparsed := roundTrip(t, src)
	if !reflect.DeepEqual(reparsed.Declarations[0].Params, original.Declarations[0].Params) {
		t.Errorf("nested params did not round-trip:\n got %#v\nwant %#v",
			reparsed.Declarations[0].Params, original.Declarations[0].Params)
	}
}

func TestMarshal_MultilineContent(t *testing.T) {
	const src = `file:
  /etc/app.env:
    state: present
    content: |
      LINE_ONE=1
      LINE_TWO=2
`
	_, reparsed := roundTrip(t, src)
	got, _ := reparsed.Declarations[0].Params["content"].(string)
	if got != "LINE_ONE=1\nLINE_TWO=2\n" {
		t.Errorf("content = %q, want the two lines with a trailing newline", got)
	}
}

func TestMarshal_IncludesAndVariables(t *testing.T) {
	const src = `includes:
  - base.yaml
  - overlay.yaml

variables:
  a: 1
  b: "two"
`
	original, reparsed := roundTrip(t, src)
	if !reflect.DeepEqual(reparsed.Includes, original.Includes) {
		t.Errorf("includes = %v, want %v", reparsed.Includes, original.Includes)
	}
	if !reflect.DeepEqual(reparsed.Variables, original.Variables) {
		t.Errorf("variables = %#v, want %#v", reparsed.Variables, original.Variables)
	}
}

// Output must not depend on Go map iteration order, or two marshals of
// the same file would produce different bytes and different run
// history.
func TestMarshal_IsDeterministic(t *testing.T) {
	const src = `file:
  /z.conf: {state: present, mode: "0600", owner: "root"}
  /a.conf: {state: present, mode: "0644", owner: "app"}
service:
  nginx: {state: running}
  redis: {state: running}
`
	sf, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	first, err := Marshal(sf)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for i := 0; i < 20; i++ {
		again, err := Marshal(sf)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if string(again) != string(first) {
			t.Fatalf("marshal %d differs from the first:\n%s\n---\n%s", i, first, again)
		}
	}
	// Sorted, so /a.conf precedes /z.conf and file precedes service.
	out := string(first)
	if strings.Index(out, "/a.conf") > strings.Index(out, "/z.conf") {
		t.Error("resources are not sorted within a module section")
	}
	if strings.Index(out, "file:") > strings.Index(out, "service:") {
		t.Error("module sections are not sorted")
	}
}

func TestMarshal_Empty(t *testing.T) {
	t.Run("empty state file", func(t *testing.T) {
		out, err := Marshal(&StateFile{})
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		sf, err := Parse(out)
		if err != nil {
			t.Fatalf("Parse(marshalled empty): %v (output %q)", err, out)
		}
		if len(sf.Declarations) != 0 {
			t.Errorf("declarations = %d, want 0", len(sf.Declarations))
		}
	})

	t.Run("nil", func(t *testing.T) {
		if _, err := Marshal(nil); err == nil {
			t.Error("Marshal(nil) error = nil, want an error")
		}
	})

	t.Run("declaration with no module", func(t *testing.T) {
		sf := &StateFile{Declarations: []*Declaration{{Name: "x", State: "present"}}}
		if _, err := Marshal(sf); err == nil {
			t.Error("Marshal() error = nil for a declaration with no module")
		}
	})

	t.Run("nil declaration is skipped", func(t *testing.T) {
		sf := &StateFile{Declarations: []*Declaration{nil}}
		if _, err := Marshal(sf); err != nil {
			t.Errorf("Marshal: %v", err)
		}
	})
}

// The point of the whole exercise: a file that has been marshalled
// must still compile through the pipeline an agent runs.
func TestMarshal_OutputStillCompiles(t *testing.T) {
	const src = `metadata:
  name: compiles
  version: "1.0"

variables:
  greeting: hello

file:
  /tmp/x.env:
    state: present
    mode: "0600"
    content: "{{ .Vars.greeting }} {{ .Facts.os }}"
`
	sf, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, err := Marshal(sf)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	reparsed, err := Parse(out)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	rendered, err := NewRenderer().RenderStateFile(reparsed, map[string]any{"os": "linux"})
	if err != nil {
		t.Fatalf("RenderStateFile after round trip: %v", err)
	}
	got, _ := rendered.Declarations[0].Params["content"].(string)
	if got != "hello linux" {
		t.Errorf("rendered content = %q, want %q", got, "hello linux")
	}
}
