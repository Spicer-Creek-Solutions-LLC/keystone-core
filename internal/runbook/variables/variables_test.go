package variables

import (
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

func TestContext_Basic(t *testing.T) {
	inputs := map[string]interface{}{
		"target": "localhost",
		"port":   8080,
	}

	ctx := NewContext("exec-123", "test-runbook", "1.0.0", inputs)

	if ctx.ExecutionID() != "exec-123" {
		t.Errorf("ExecutionID() = %v, want %v", ctx.ExecutionID(), "exec-123")
	}

	if ctx.RunbookName() != "test-runbook" {
		t.Errorf("RunbookName() = %v, want %v", ctx.RunbookName(), "test-runbook")
	}

	if ctx.RunbookVersion() != "1.0.0" {
		t.Errorf("RunbookVersion() = %v, want %v", ctx.RunbookVersion(), "1.0.0")
	}

	if ctx.StartTime().IsZero() {
		t.Error("StartTime() should not be zero")
	}
}

func TestContext_Inputs(t *testing.T) {
	inputs := map[string]interface{}{
		"target": "localhost",
		"port":   8080,
	}

	ctx := NewContext("exec-123", "test-runbook", "1.0.0", inputs)

	// Get existing input
	if v, ok := ctx.GetInput("target"); !ok || v != "localhost" {
		t.Errorf("GetInput(target) = %v, %v; want localhost, true", v, ok)
	}

	// Get non-existent input
	if _, ok := ctx.GetInput("nonexistent"); ok {
		t.Error("GetInput(nonexistent) should return false")
	}

	// Set new input
	ctx.SetInput("newInput", "newValue")
	if v, ok := ctx.GetInput("newInput"); !ok || v != "newValue" {
		t.Errorf("GetInput(newInput) = %v, %v; want newValue, true", v, ok)
	}
}

func TestContext_StepOutputs(t *testing.T) {
	ctx := NewContext("exec-123", "test-runbook", "1.0.0", nil)

	// Set step output
	ctx.SetStepOutput("step1", "result", "success")
	ctx.SetStepOutput("step1", "count", 42)

	// Get step output
	if v, ok := ctx.GetStepOutput("step1", "result"); !ok || v != "success" {
		t.Errorf("GetStepOutput(step1, result) = %v, %v; want success, true", v, ok)
	}

	if v, ok := ctx.GetStepOutput("step1", "count"); !ok || v != 42 {
		t.Errorf("GetStepOutput(step1, count) = %v, %v; want 42, true", v, ok)
	}

	// Get non-existent step output
	if _, ok := ctx.GetStepOutput("step1", "nonexistent"); ok {
		t.Error("GetStepOutput(step1, nonexistent) should return false")
	}

	if _, ok := ctx.GetStepOutput("nonexistent", "result"); ok {
		t.Error("GetStepOutput(nonexistent, result) should return false")
	}

	// Set all outputs at once
	ctx.SetStepOutputs("step2", map[string]interface{}{
		"a": 1,
		"b": 2,
	})

	if outputs, ok := ctx.GetStepOutputs("step2"); !ok || len(outputs) != 2 {
		t.Errorf("GetStepOutputs(step2) = %v, %v; want 2 outputs", outputs, ok)
	}
}

func TestContext_Resolve(t *testing.T) {
	inputs := map[string]interface{}{
		"name": "world",
	}

	ctx := NewContext("exec-123", "test-runbook", "1.0.0", inputs)
	ctx.SetStepOutput("greet", "message", "hello")

	tests := []struct {
		name     string
		template string
		want     string
		wantErr  bool
	}{
		{
			name:     "no template",
			template: "plain text",
			want:     "plain text",
		},
		{
			name:     "input reference",
			template: "Hello {{ .inputs.name }}!",
			want:     "Hello world!",
		},
		{
			name:     "step output reference",
			template: "{{ .steps.greet.message }}",
			want:     "hello",
		},
		{
			name:     "runbook metadata",
			template: "{{ .runbook.name }}",
			want:     "test-runbook",
		},
		{
			name:     "execution metadata",
			template: "{{ .execution.id }}",
			want:     "exec-123",
		},
		{
			name:     "string function",
			template: "{{ upper .inputs.name }}",
			want:     "WORLD",
		},
		{
			name:     "default value",
			template: `{{ default "fallback" .inputs.missing }}`,
			want:     "fallback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ctx.Resolve(tt.template)
			if (err != nil) != tt.wantErr {
				t.Errorf("Resolve() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Resolve() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContext_ResolveMap(t *testing.T) {
	inputs := map[string]interface{}{
		"target": "localhost",
		"port":   8080,
	}

	ctx := NewContext("exec-123", "test-runbook", "1.0.0", inputs)

	m := map[string]interface{}{
		"host":   "{{ .inputs.target }}",
		"port":   "{{ .inputs.port }}",
		"static": "value",
		"number": 42,
		"nested": map[string]interface{}{
			"inner": "{{ .inputs.target }}",
		},
	}

	resolved, err := ctx.ResolveMap(m)
	if err != nil {
		t.Fatalf("ResolveMap() error = %v", err)
	}

	if resolved["host"] != "localhost" {
		t.Errorf("resolved[host] = %v, want localhost", resolved["host"])
	}

	if resolved["static"] != "value" {
		t.Errorf("resolved[static] = %v, want value", resolved["static"])
	}

	if resolved["number"] != 42 {
		t.Errorf("resolved[number] = %v, want 42", resolved["number"])
	}

	if nested, ok := resolved["nested"].(map[string]interface{}); ok {
		if nested["inner"] != "localhost" {
			t.Errorf("resolved[nested][inner] = %v, want localhost", nested["inner"])
		}
	} else {
		t.Error("resolved[nested] should be a map")
	}
}

func TestTemplateEngine_Functions(t *testing.T) {
	ctx := NewContext("exec-123", "test-runbook", "1.0.0", nil)

	tests := []struct {
		name     string
		template string
		want     string
	}{
		{"upper", "{{ upper \"hello\" }}", "HELLO"},
		{"lower", "{{ lower \"HELLO\" }}", "hello"},
		{"trim", "{{ trim \"  hello  \" }}", "hello"},
		{"trimPrefix", `{{ trimPrefix "pre_value" "pre_" }}`, "value"},
		{"trimSuffix", `{{ trimSuffix "value_suf" "_suf" }}`, "value"},
		{"replace", `{{ replace "old-old-old" "old" "new" }}`, "new-new-new"},
		{"contains", "{{ contains \"hello\" \"ell\" }}", "true"},
		{"hasPrefix", "{{ hasPrefix \"hello\" \"hel\" }}", "true"},
		{"hasSuffix", "{{ hasSuffix \"hello\" \"llo\" }}", "true"},
		{"toString", "{{ toString 42 }}", "42"},
		{"toInt", "{{ toInt \"42\" }}", "42"},
		{"toFloat", "{{ toFloat \"3.14\" }}", "3.14"},
		{"toBool", "{{ toBool \"true\" }}", "true"},
		{"toJSON", `{{ toJSON "hello" }}`, `"hello"`},
		{"b64enc", "{{ b64enc \"hello\" }}", "aGVsbG8="},
		{"b64dec", "{{ b64dec \"aGVsbG8=\" }}", "hello"},
		{"regexMatch", `{{ regexMatch "h.*o" "hello" }}`, "true"},
		{"regexFind", `{{ regexFind "[0-9]+" "abc123def" }}`, "123"},
		{"len", "{{ len \"hello\" }}", "5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ctx.Resolve(tt.template)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Resolve() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasTemplateSyntax(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"plain text", false},
		{"{{ .value }}", true},
		{"Hello {{ .name }}", true},
		{"{ not template }", false},
		{"{{}}", true},
		{"", false},
	}

	for _, tt := range tests {
		got := hasTemplateSyntax(tt.input)
		if got != tt.want {
			t.Errorf("hasTemplateSyntax(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestOutputParser_JSON(t *testing.T) {
	parser := NewOutputParser()

	data := map[string]interface{}{
		"body": `{"name": "test", "count": 42, "items": [1, 2, 3]}`,
	}

	tests := []struct {
		name    string
		output  runbook.OutputDef
		want    interface{}
		wantErr bool
	}{
		{
			name: "parse entire JSON",
			output: runbook.OutputDef{
				Name:   "result",
				Source: runbook.OutputSourceBody,
				Parser: runbook.OutputParserJSON,
			},
			want: map[string]interface{}{
				"name":  "test",
				"count": float64(42), // JSON numbers are float64
				"items": []interface{}{float64(1), float64(2), float64(3)},
			},
		},
		{
			name: "extract field",
			output: runbook.OutputDef{
				Name:   "name",
				Source: runbook.OutputSourceBody,
				Parser: runbook.OutputParserJSON,
				Path:   ".name",
			},
			want: "test",
		},
		{
			name: "extract array element",
			output: runbook.OutputDef{
				Name:   "first",
				Source: runbook.OutputSourceBody,
				Parser: runbook.OutputParserJSON,
				Path:   ".items[0]",
			},
			want: float64(1),
		},
		{
			name: "non-existent field",
			output: runbook.OutputDef{
				Name:   "missing",
				Source: runbook.OutputSourceBody,
				Parser: runbook.OutputParserJSON,
				Path:   ".nonexistent",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.Parse(data, &tt.output)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !deepEqual(got, tt.want) {
				t.Errorf("Parse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOutputParser_Regex(t *testing.T) {
	parser := NewOutputParser()

	data := map[string]interface{}{
		"stdout": "Version: 1.2.3\nBuild: 456",
	}

	tests := []struct {
		name    string
		output  runbook.OutputDef
		want    interface{}
		wantErr bool
	}{
		{
			name: "capture group",
			output: runbook.OutputDef{
				Name:   "version",
				Source: runbook.OutputSourceStdout,
				Parser: runbook.OutputParserRegex,
				Path:   `Version: (\d+\.\d+\.\d+)`,
			},
			want: "1.2.3",
		},
		{
			name: "named groups",
			output: runbook.OutputDef{
				Name:   "info",
				Source: runbook.OutputSourceStdout,
				Parser: runbook.OutputParserRegex,
				Path:   `Version: (?P<version>\d+\.\d+\.\d+)`,
			},
			want: map[string]string{
				"version": "1.2.3",
			},
		},
		{
			name: "no match",
			output: runbook.OutputDef{
				Name:   "nomatch",
				Source: runbook.OutputSourceStdout,
				Parser: runbook.OutputParserRegex,
				Path:   `NotFound: (.*)`,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.Parse(data, &tt.output)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !deepEqual(got, tt.want) {
				t.Errorf("Parse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOutputParser_Line(t *testing.T) {
	parser := NewOutputParser()

	data := map[string]interface{}{
		"stdout": "line1\nline2\nline3",
	}

	tests := []struct {
		name    string
		output  runbook.OutputDef
		want    interface{}
		wantErr bool
	}{
		{
			name: "all lines",
			output: runbook.OutputDef{
				Name:   "lines",
				Source: runbook.OutputSourceStdout,
				Parser: runbook.OutputParserLine,
				Path:   "all",
			},
			want: []string{"line1", "line2", "line3"},
		},
		{
			name: "first line",
			output: runbook.OutputDef{
				Name:   "first",
				Source: runbook.OutputSourceStdout,
				Parser: runbook.OutputParserLine,
				Path:   "first",
			},
			want: "line1",
		},
		{
			name: "last line",
			output: runbook.OutputDef{
				Name:   "last",
				Source: runbook.OutputSourceStdout,
				Parser: runbook.OutputParserLine,
				Path:   "last",
			},
			want: "line3",
		},
		{
			name: "line by number",
			output: runbook.OutputDef{
				Name:   "second",
				Source: runbook.OutputSourceStdout,
				Parser: runbook.OutputParserLine,
				Path:   "2",
			},
			want: "line2",
		},
		{
			name: "line count",
			output: runbook.OutputDef{
				Name:   "count",
				Source: runbook.OutputSourceStdout,
				Parser: runbook.OutputParserLine,
				Path:   "count",
			},
			want: 3,
		},
		{
			name: "line out of range",
			output: runbook.OutputDef{
				Name:   "outofrange",
				Source: runbook.OutputSourceStdout,
				Parser: runbook.OutputParserLine,
				Path:   "10",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.Parse(data, &tt.output)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !deepEqual(got, tt.want) {
				t.Errorf("Parse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOutputParser_Sources(t *testing.T) {
	parser := NewOutputParser()

	data := map[string]interface{}{
		"stdout":      "stdout content",
		"stderr":      "stderr content",
		"exit_code":   0,
		"status_code": 200,
		"body":        "response body",
		"headers": map[string]interface{}{
			"Content-Type": "application/json",
		},
	}

	tests := []struct {
		name   string
		source runbook.OutputSource
		want   string
	}{
		{"stdout", runbook.OutputSourceStdout, "stdout content"},
		{"stderr", runbook.OutputSourceStderr, "stderr content"},
		{"body", runbook.OutputSourceBody, "response body"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := &runbook.OutputDef{
				Name:   "test",
				Source: tt.source,
			}
			got, err := parser.Parse(data, output)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Parse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContext_ToData(t *testing.T) {
	inputs := map[string]interface{}{
		"key": "value",
	}

	ctx := NewContext("exec-123", "test-runbook", "1.0.0", inputs)
	ctx.SetStepOutput("step1", "out", "val")

	data := ctx.ToData()

	// Check structure
	if _, ok := data["inputs"]; !ok {
		t.Error("data should have 'inputs' key")
	}
	if _, ok := data["steps"]; !ok {
		t.Error("data should have 'steps' key")
	}
	if _, ok := data["runbook"]; !ok {
		t.Error("data should have 'runbook' key")
	}
	if _, ok := data["execution"]; !ok {
		t.Error("data should have 'execution' key")
	}
	if _, ok := data["now"]; !ok {
		t.Error("data should have 'now' key")
	}

	// Check values
	rb := data["runbook"].(map[string]interface{})
	if rb["name"] != "test-runbook" {
		t.Errorf("runbook.name = %v, want test-runbook", rb["name"])
	}

	exec := data["execution"].(map[string]interface{})
	if exec["id"] != "exec-123" {
		t.Errorf("execution.id = %v, want exec-123", exec["id"])
	}

	// now should be recent
	if now, ok := data["now"].(time.Time); ok {
		if time.Since(now) > time.Second {
			t.Error("now should be recent")
		}
	}
}

// deepEqual is a helper for comparing test values
func deepEqual(a, b interface{}) bool {
	// Handle map comparison
	if ma, ok := a.(map[string]interface{}); ok {
		if mb, ok := b.(map[string]interface{}); ok {
			if len(ma) != len(mb) {
				return false
			}
			for k, va := range ma {
				if vb, ok := mb[k]; !ok || !deepEqual(va, vb) {
					return false
				}
			}
			return true
		}
	}

	// Handle string map comparison (for regex named groups)
	if ma, ok := a.(map[string]string); ok {
		if mb, ok := b.(map[string]string); ok {
			if len(ma) != len(mb) {
				return false
			}
			for k, va := range ma {
				if vb, ok := mb[k]; !ok || va != vb {
					return false
				}
			}
			return true
		}
	}

	// Handle slice comparison
	if sa, ok := a.([]interface{}); ok {
		if sb, ok := b.([]interface{}); ok {
			if len(sa) != len(sb) {
				return false
			}
			for i := range sa {
				if !deepEqual(sa[i], sb[i]) {
					return false
				}
			}
			return true
		}
	}

	// Handle string slice comparison
	if sa, ok := a.([]string); ok {
		if sb, ok := b.([]string); ok {
			if len(sa) != len(sb) {
				return false
			}
			for i := range sa {
				if sa[i] != sb[i] {
					return false
				}
			}
			return true
		}
	}

	return a == b
}
