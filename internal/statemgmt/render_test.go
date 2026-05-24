// SPDX-License-Identifier: Apache-2.0

package statemgmt

import (
	"strings"
	"testing"
)

func TestAsciiTitle(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"":               "",
		"hello":          "Hello",
		"hello world":    "Hello World",
		"  spaced  out ": "  Spaced  Out ",
		"already Title":  "Already Title",
		"123 numeric":    "123 Numeric",
		"UPPER lower":    "UPPER Lower",
	}
	for in, want := range cases {
		if got := asciiTitle(in); got != want {
			t.Errorf("asciiTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJoinFunc(t *testing.T) {
	t.Parallel()
	cases := []struct {
		sep   string
		items []any
		want  string
	}{
		{",", []any{"a", "b", "c"}, "a,b,c"},
		{"-", []any{1, 2, 3}, "1-2-3"},
		{" ", []any{"one", 2, true}, "one 2 true"},
		{",", []any{}, ""},
		{"", []any{"a", "b"}, "ab"},
	}
	for _, c := range cases {
		if got := joinFunc(c.sep, c.items); got != c.want {
			t.Errorf("joinFunc(%q, %v) = %q, want %q", c.sep, c.items, got, c.want)
		}
	}
}

func TestSplitFunc(t *testing.T) {
	t.Parallel()
	got := splitFunc(",", "a,b,c")
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("splitFunc len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("splitFunc[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDefaultFunc(t *testing.T) {
	t.Parallel()
	cases := []struct {
		def, v any
		want   any
	}{
		{"fallback", nil, "fallback"},
		{"fallback", "", "fallback"},
		{"fallback", "actual", "actual"},
		{"fallback", 0, 0},        // numeric zero is NOT "empty" for v1.0
		{"fallback", false, false}, // boolean false is NOT "empty"
		{42, nil, 42},
	}
	for _, c := range cases {
		if got := defaultFunc(c.def, c.v); got != c.want {
			t.Errorf("defaultFunc(%v, %v) = %v, want %v", c.def, c.v, got, c.want)
		}
	}
}

func TestRenderString_Identity(t *testing.T) {
	t.Parallel()
	r := NewRenderer()
	got, err := r.RenderString("no directives here", nil)
	if err != nil {
		t.Fatalf("RenderString: %v", err)
	}
	if got != "no directives here" {
		t.Errorf("RenderString = %q, want identity", got)
	}
}

func TestRenderString_Substitution(t *testing.T) {
	t.Parallel()
	r := NewRenderer()
	ctx := renderContext{Vars: map[string]any{"user": "www-data"}}
	got, err := r.RenderString("hello {{ .Vars.user }}", ctx)
	if err != nil {
		t.Fatalf("RenderString: %v", err)
	}
	if got != "hello www-data" {
		t.Errorf("RenderString = %q, want \"hello www-data\"", got)
	}
}

func TestRenderString_MissingKey_IsError(t *testing.T) {
	t.Parallel()
	r := NewRenderer()
	ctx := renderContext{Vars: map[string]any{}}
	_, err := r.RenderString("hello {{ .Vars.absent }}", ctx)
	if err == nil {
		t.Fatal("missing variable must error (missingkey=error)")
	}
	if !strings.Contains(err.Error(), "statemgmt: render") {
		t.Errorf("err = %v, want wrapped statemgmt prefix", err)
	}
}

func TestRenderString_ParseError(t *testing.T) {
	t.Parallel()
	r := NewRenderer()
	_, err := r.RenderString("{{ unterminated", nil)
	if err == nil {
		t.Fatal("malformed template must error")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("err = %v, want \"parse\" in message", err)
	}
}

func TestRenderString_CustomFuncs_Wired(t *testing.T) {
	t.Parallel()
	r := NewRenderer()
	ctx := renderContext{
		Vars: map[string]any{
			"name":  "  nginx  ",
			"parts": []any{"a", "b", "c"},
			"empty": "",
		},
	}
	cases := map[string]string{
		`{{ upper .Vars.name | trim }}`:                "NGINX",
		`{{ lower "HELLO" }}`:                          "hello",
		`{{ title "two words" }}`:                      "Two Words",
		`{{ join "," .Vars.parts }}`:                   "a,b,c",
		`{{ index (split "," "x,y,z") 1 }}`:            "y",
		`{{ default "fallback" .Vars.empty }}`:         "fallback",
		`{{ default "fallback" .Vars.name | trim }}`:   "nginx",
	}
	for tpl, want := range cases {
		got, err := r.RenderString(tpl, ctx)
		if err != nil {
			t.Errorf("RenderString(%q): %v", tpl, err)
			continue
		}
		if got != want {
			t.Errorf("RenderString(%q) = %q, want %q", tpl, got, want)
		}
	}
}

func TestRenderStateFile_RendersStateNameParams(t *testing.T) {
	t.Parallel()
	r := NewRenderer()
	sf := &StateFile{
		Variables: map[string]any{
			"app":   "nginx",
			"state": "running",
			"user":  "www-data",
			"env":   "prod",
		},
		Declarations: []*Declaration{{
			ID:     "service:placeholder",
			Module: "service",
			Name:   "{{ .Vars.app }}",
			State:  "{{ .Vars.state }}",
			Params: map[string]any{
				"owner": "{{ .Vars.user }}",
				"tags":  []any{"{{ .Vars.env }}", "literal"},
				"nested": map[string]any{
					"mode": "0644",
					"path": "/etc/{{ .Vars.app }}.conf",
				},
				"count": 3,
				"on":    true,
			},
		}},
	}
	out, err := r.RenderStateFile(sf, nil)
	if err != nil {
		t.Fatalf("RenderStateFile: %v", err)
	}
	d := out.Declarations[0]
	if d.Name != "nginx" {
		t.Errorf("Name = %q, want nginx", d.Name)
	}
	if d.State != "running" {
		t.Errorf("State = %q, want running", d.State)
	}
	if d.ID != "service:nginx" {
		t.Errorf("ID = %q, want service:nginx (recomputed after Name renders)", d.ID)
	}
	if d.Params["owner"] != "www-data" {
		t.Errorf("Params[owner] = %v, want www-data", d.Params["owner"])
	}
	tags, ok := d.Params["tags"].([]any)
	if !ok || len(tags) != 2 {
		t.Fatalf("Params[tags] = %v, want []any of len 2", d.Params["tags"])
	}
	if tags[0] != "prod" || tags[1] != "literal" {
		t.Errorf("Params[tags] = %v, want [prod literal]", tags)
	}
	nested, _ := d.Params["nested"].(map[string]any)
	if nested["path"] != "/etc/nginx.conf" {
		t.Errorf("Params.nested.path = %v, want /etc/nginx.conf", nested["path"])
	}
	if nested["mode"] != "0644" {
		t.Errorf("Params.nested.mode = %v, want 0644 (unchanged)", nested["mode"])
	}
	if d.Params["count"] != 3 {
		t.Errorf("Params[count] = %v (%T), want int 3 passed through", d.Params["count"], d.Params["count"])
	}
	if d.Params["on"] != true {
		t.Errorf("Params[on] = %v, want true passed through", d.Params["on"])
	}
}

func TestRenderStateFile_FactsInScope(t *testing.T) {
	t.Parallel()
	r := NewRenderer()
	sf := &StateFile{
		Variables: map[string]any{},
		Declarations: []*Declaration{{
			ID:     "package:apt",
			Module: "package",
			Name:   "apt",
			State:  "{{ if eq .Facts.os \"debian\" }}installed{{ else }}absent{{ end }}",
			Params: map[string]any{},
		}},
	}
	out, err := r.RenderStateFile(sf, map[string]any{"os": "debian"})
	if err != nil {
		t.Fatalf("RenderStateFile: %v", err)
	}
	if out.Declarations[0].State != "installed" {
		t.Errorf("State = %q, want installed", out.Declarations[0].State)
	}
}

func TestRenderStateFile_ModuleNotRendered(t *testing.T) {
	t.Parallel()
	// A literal "{{ template }}" string in Module survives — it is
	// NOT rendered. This guards against the silent-mis-dispatch
	// failure mode where a typo'd module-name template routes a
	// declaration to the wrong stdlib module.
	r := NewRenderer()
	sf := &StateFile{
		Variables: map[string]any{"m": "file"},
		Declarations: []*Declaration{{
			ID:     "{{ .Vars.m }}:/etc/hosts",
			Module: "{{ .Vars.m }}",
			Name:   "/etc/hosts",
			State:  "present",
		}},
	}
	out, err := r.RenderStateFile(sf, nil)
	if err != nil {
		t.Fatalf("RenderStateFile: %v", err)
	}
	if out.Declarations[0].Module != "{{ .Vars.m }}" {
		t.Errorf("Module = %q, want literal template string preserved", out.Declarations[0].Module)
	}
	// ID is recomputed from the (un-rendered) Module + (rendered) Name.
	if out.Declarations[0].ID != "{{ .Vars.m }}:/etc/hosts" {
		t.Errorf("ID = %q, want literal template + name", out.Declarations[0].ID)
	}
}

func TestRenderStateFile_VariablesNotRendered(t *testing.T) {
	t.Parallel()
	// A variable value containing template syntax substitutes as a
	// literal string into rendered Params; it is NOT re-evaluated.
	// This is the v1.0 sandboxing posture against agent-supplied
	// attack payloads.
	r := NewRenderer()
	sf := &StateFile{
		Variables: map[string]any{
			"injection": "{{ .Vars.secret }}",
			"secret":    "never-leaked",
		},
		Declarations: []*Declaration{{
			Module: "file",
			Name:   "/tmp/x",
			State:  "present",
			Params: map[string]any{
				"content": "{{ .Vars.injection }}",
			},
		}},
	}
	out, err := r.RenderStateFile(sf, nil)
	if err != nil {
		t.Fatalf("RenderStateFile: %v", err)
	}
	got := out.Declarations[0].Params["content"]
	if got != "{{ .Vars.secret }}" {
		t.Errorf("content = %q, want literal template string (no recursive expansion)", got)
	}
	// And the surviving Variables map must still hold the literal
	// template syntax — the renderer does not mutate the source.
	if sf.Variables["injection"] != "{{ .Vars.secret }}" {
		t.Errorf("source Variables mutated; got %v", sf.Variables["injection"])
	}
}

func TestRenderStateFile_MetadataAndIncludesNotRendered(t *testing.T) {
	t.Parallel()
	r := NewRenderer()
	sf := &StateFile{
		Metadata: Metadata{Name: "{{ .Vars.name }}", Version: "1.0"},
		Includes: []string{"{{ .Vars.file }}"},
		Variables: map[string]any{
			"name": "would-be-rendered",
			"file": "would-be-rendered.yaml",
		},
	}
	out, err := r.RenderStateFile(sf, nil)
	if err != nil {
		t.Fatalf("RenderStateFile: %v", err)
	}
	if out.Metadata.Name != "{{ .Vars.name }}" {
		t.Errorf("Metadata.Name was rendered; got %q", out.Metadata.Name)
	}
	if len(out.Includes) != 1 || out.Includes[0] != "{{ .Vars.file }}" {
		t.Errorf("Includes was rendered; got %v", out.Includes)
	}
}

func TestRenderStateFile_MissingVarErrorCitesDecl(t *testing.T) {
	t.Parallel()
	r := NewRenderer()
	sf := &StateFile{
		Variables: map[string]any{},
		Declarations: []*Declaration{{
			ID:     "file:/etc/hosts",
			Module: "file",
			Name:   "/etc/hosts",
			State:  "{{ .Vars.absent }}",
		}},
	}
	_, err := r.RenderStateFile(sf, nil)
	if err == nil {
		t.Fatal("expected error for missing variable")
	}
	if !strings.Contains(err.Error(), `"file:/etc/hosts"`) {
		t.Errorf("err = %v, want declaration ID cited", err)
	}
	if !strings.Contains(err.Error(), "State") {
		t.Errorf("err = %v, want field name cited", err)
	}
}

func TestRenderStateFile_NoOpOnEmptyFields(t *testing.T) {
	t.Parallel()
	r := NewRenderer()
	sf := &StateFile{
		Declarations: []*Declaration{{
			Module: "file",
			Name:   "/etc/hosts",
		}},
	}
	out, err := r.RenderStateFile(sf, nil)
	if err != nil {
		t.Fatalf("RenderStateFile: %v", err)
	}
	if out.Declarations[0].State != "" {
		t.Errorf("State = %q, want empty pass-through", out.Declarations[0].State)
	}
	if out.Declarations[0].Params != nil {
		t.Errorf("Params = %v, want nil pass-through", out.Declarations[0].Params)
	}
}

func TestRenderStateFile_OrderPreserved(t *testing.T) {
	t.Parallel()
	r := NewRenderer()
	sf := &StateFile{
		Variables: map[string]any{},
		Declarations: []*Declaration{
			{Module: "package", Name: "first", State: "installed"},
			{Module: "file", Name: "second", State: "present"},
			{Module: "service", Name: "third", State: "running"},
		},
	}
	out, err := r.RenderStateFile(sf, nil)
	if err != nil {
		t.Fatalf("RenderStateFile: %v", err)
	}
	wantIDs := []string{"package:first", "file:second", "service:third"}
	for i, want := range wantIDs {
		if out.Declarations[i].ID != want {
			t.Errorf("Declarations[%d].ID = %q, want %q", i, out.Declarations[i].ID, want)
		}
	}
}

func TestRenderStateFile_NilInput(t *testing.T) {
	t.Parallel()
	r := NewRenderer()
	out, err := r.RenderStateFile(nil, nil)
	if err != nil {
		t.Fatalf("RenderStateFile(nil): %v", err)
	}
	if out != nil {
		t.Errorf("RenderStateFile(nil) = %v, want nil", out)
	}
}

func TestRenderer_ReusableAcrossCalls(t *testing.T) {
	t.Parallel()
	r := NewRenderer()
	for i := 0; i < 3; i++ {
		out, err := r.RenderString("{{ upper .Vars.x }}", renderContext{Vars: map[string]any{"x": "abc"}})
		if err != nil {
			t.Fatalf("RenderString iteration %d: %v", i, err)
		}
		if out != "ABC" {
			t.Errorf("iteration %d: got %q, want ABC", i, out)
		}
	}
}
