package template

import (
	"testing"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()

	tmpl := &Template{
		Metadata: TemplateMetadata{
			Name:    "test-template",
			Version: "1.0.0",
		},
		Spec: TemplateSpec{
			Steps: []runbook.Step{
				{Name: "step1", Type: runbook.StepTypeNoop},
			},
		},
	}

	t.Run("register new template", func(t *testing.T) {
		if err := r.Register(tmpl); err != nil {
			t.Errorf("Register() error = %v", err)
		}
	})

	t.Run("duplicate registration", func(t *testing.T) {
		if err := r.Register(tmpl); err == nil {
			t.Error("Expected error for duplicate registration")
		}
	})

	t.Run("missing name", func(t *testing.T) {
		badTmpl := &Template{
			Metadata: TemplateMetadata{Version: "1.0.0"},
		}
		if err := r.Register(badTmpl); err == nil {
			t.Error("Expected error for missing name")
		}
	})

	t.Run("missing version", func(t *testing.T) {
		badTmpl := &Template{
			Metadata: TemplateMetadata{Name: "no-version"},
		}
		if err := r.Register(badTmpl); err == nil {
			t.Error("Expected error for missing version")
		}
	})
}

func TestRegistry_Get(t *testing.T) {
	r := NewRegistry()

	v1 := &Template{
		Metadata: TemplateMetadata{
			Name:    "test-template",
			Version: "1.0.0",
		},
		Spec: TemplateSpec{
			Steps: []runbook.Step{{Name: "step1", Type: runbook.StepTypeNoop}},
		},
	}
	v2 := &Template{
		Metadata: TemplateMetadata{
			Name:    "test-template",
			Version: "2.0.0",
		},
		Spec: TemplateSpec{
			Steps: []runbook.Step{{Name: "step2", Type: runbook.StepTypeNoop}},
		},
	}

	r.Register(v1)
	r.Register(v2)

	t.Run("get specific version", func(t *testing.T) {
		tmpl, ok := r.Get("test-template", "1.0.0")
		if !ok {
			t.Fatal("Template not found")
		}
		if tmpl.Metadata.Version != "1.0.0" {
			t.Errorf("Expected version 1.0.0, got %s", tmpl.Metadata.Version)
		}
	})

	t.Run("get latest version", func(t *testing.T) {
		tmpl, ok := r.Get("test-template", "")
		if !ok {
			t.Fatal("Template not found")
		}
		if tmpl.Metadata.Version != "2.0.0" {
			t.Errorf("Expected version 2.0.0, got %s", tmpl.Metadata.Version)
		}
	})

	t.Run("get nonexistent", func(t *testing.T) {
		_, ok := r.Get("nonexistent", "")
		if ok {
			t.Error("Expected template not found")
		}
	})
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()

	r.Register(&Template{
		Metadata: TemplateMetadata{Name: "tmpl1", Version: "1.0.0"},
		Spec:     TemplateSpec{Steps: []runbook.Step{{Name: "s1", Type: runbook.StepTypeNoop}}},
	})
	r.Register(&Template{
		Metadata: TemplateMetadata{Name: "tmpl2", Version: "1.0.0"},
		Spec:     TemplateSpec{Steps: []runbook.Step{{Name: "s1", Type: runbook.StepTypeNoop}}},
	})

	templates := r.List()
	if len(templates) != 2 {
		t.Errorf("Expected 2 templates, got %d", len(templates))
	}
}

func TestRegistry_ListVersions(t *testing.T) {
	r := NewRegistry()

	r.Register(&Template{
		Metadata: TemplateMetadata{Name: "test", Version: "1.0.0"},
		Spec:     TemplateSpec{Steps: []runbook.Step{{Name: "s1", Type: runbook.StepTypeNoop}}},
	})
	r.Register(&Template{
		Metadata: TemplateMetadata{Name: "test", Version: "2.0.0"},
		Spec:     TemplateSpec{Steps: []runbook.Step{{Name: "s1", Type: runbook.StepTypeNoop}}},
	})

	versions := r.ListVersions("test")
	if len(versions) != 2 {
		t.Errorf("Expected 2 versions, got %d", len(versions))
	}

	// Nonexistent template
	versions = r.ListVersions("nonexistent")
	if versions != nil {
		t.Error("Expected nil for nonexistent template")
	}
}

func TestRegistry_Delete(t *testing.T) {
	r := NewRegistry()

	r.Register(&Template{
		Metadata: TemplateMetadata{Name: "test", Version: "1.0.0"},
		Spec:     TemplateSpec{Steps: []runbook.Step{{Name: "s1", Type: runbook.StepTypeNoop}}},
	})
	r.Register(&Template{
		Metadata: TemplateMetadata{Name: "test", Version: "2.0.0"},
		Spec:     TemplateSpec{Steps: []runbook.Step{{Name: "s1", Type: runbook.StepTypeNoop}}},
	})

	t.Run("delete specific version", func(t *testing.T) {
		if err := r.Delete("test", "1.0.0"); err != nil {
			t.Errorf("Delete() error = %v", err)
		}

		_, ok := r.Get("test", "1.0.0")
		if ok {
			t.Error("Deleted version still exists")
		}

		// Latest should now be 2.0.0
		latest, _ := r.GetLatestVersion("test")
		if latest != "2.0.0" {
			t.Errorf("Expected latest 2.0.0, got %s", latest)
		}
	})

	t.Run("delete nonexistent", func(t *testing.T) {
		if err := r.Delete("nonexistent", "1.0.0"); err == nil {
			t.Error("Expected error for nonexistent template")
		}
	})
}

func TestValidator_Validate(t *testing.T) {
	tests := []struct {
		name    string
		tmpl    *Template
		wantErr bool
	}{
		{
			name: "valid template",
			tmpl: &Template{
				APIVersion: "runbook.keystone.io/v1",
				Kind:       "StepTemplate",
				Metadata: TemplateMetadata{
					Name:    "test-template",
					Version: "1.0.0",
				},
				Spec: TemplateSpec{
					Steps: []runbook.Step{
						{Name: "step1", Type: runbook.StepTypeNoop},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing apiVersion",
			tmpl: &Template{
				Kind: "StepTemplate",
				Metadata: TemplateMetadata{
					Name:    "test",
					Version: "1.0.0",
				},
				Spec: TemplateSpec{
					Steps: []runbook.Step{{Name: "s1", Type: runbook.StepTypeNoop}},
				},
			},
			wantErr: true,
		},
		{
			name: "wrong kind",
			tmpl: &Template{
				APIVersion: "runbook.keystone.io/v1",
				Kind:       "WrongKind",
				Metadata: TemplateMetadata{
					Name:    "test",
					Version: "1.0.0",
				},
				Spec: TemplateSpec{
					Steps: []runbook.Step{{Name: "s1", Type: runbook.StepTypeNoop}},
				},
			},
			wantErr: true,
		},
		{
			name: "missing name",
			tmpl: &Template{
				APIVersion: "runbook.keystone.io/v1",
				Kind:       "StepTemplate",
				Metadata: TemplateMetadata{
					Version: "1.0.0",
				},
				Spec: TemplateSpec{
					Steps: []runbook.Step{{Name: "s1", Type: runbook.StepTypeNoop}},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid version",
			tmpl: &Template{
				APIVersion: "runbook.keystone.io/v1",
				Kind:       "StepTemplate",
				Metadata: TemplateMetadata{
					Name:    "test",
					Version: "invalid",
				},
				Spec: TemplateSpec{
					Steps: []runbook.Step{{Name: "s1", Type: runbook.StepTypeNoop}},
				},
			},
			wantErr: true,
		},
		{
			name: "no steps",
			tmpl: &Template{
				APIVersion: "runbook.keystone.io/v1",
				Kind:       "StepTemplate",
				Metadata: TemplateMetadata{
					Name:    "test",
					Version: "1.0.0",
				},
				Spec: TemplateSpec{
					Steps: []runbook.Step{},
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate step names",
			tmpl: &Template{
				APIVersion: "runbook.keystone.io/v1",
				Kind:       "StepTemplate",
				Metadata: TemplateMetadata{
					Name:    "test",
					Version: "1.0.0",
				},
				Spec: TemplateSpec{
					Steps: []runbook.Step{
						{Name: "step1", Type: runbook.StepTypeNoop},
						{Name: "step1", Type: runbook.StepTypeNoop},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate parameter names",
			tmpl: &Template{
				APIVersion: "runbook.keystone.io/v1",
				Kind:       "StepTemplate",
				Metadata: TemplateMetadata{
					Name:    "test",
					Version: "1.0.0",
				},
				Spec: TemplateSpec{
					Parameters: []TemplateParameter{
						{Name: "param1", Type: "string"},
						{Name: "param1", Type: "int"},
					},
					Steps: []runbook.Step{{Name: "s1", Type: runbook.StepTypeNoop}},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.tmpl)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParser_Parse(t *testing.T) {
	parser := NewParser()

	t.Run("valid YAML", func(t *testing.T) {
		yaml := `
apiVersion: runbook.keystone.io/v1
kind: StepTemplate
metadata:
  name: test-template
  version: 1.0.0
spec:
  steps:
    - name: step1
      type: noop
`
		tmpl, err := parser.Parse([]byte(yaml))
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		if tmpl.Metadata.Name != "test-template" {
			t.Errorf("Expected name 'test-template', got %s", tmpl.Metadata.Name)
		}
	})

	t.Run("invalid YAML", func(t *testing.T) {
		yaml := `invalid: yaml: content:`
		_, err := parser.Parse([]byte(yaml))
		if err == nil {
			t.Error("Expected error for invalid YAML")
		}
	})

	t.Run("validation failure", func(t *testing.T) {
		yaml := `
apiVersion: runbook.keystone.io/v1
kind: StepTemplate
metadata:
  name: test
spec:
  steps: []
`
		_, err := parser.Parse([]byte(yaml))
		if err == nil {
			t.Error("Expected validation error")
		}
	})
}

func TestExpander_Expand(t *testing.T) {
	registry := NewRegistry()

	// Register a simple template
	tmpl := &Template{
		Metadata: TemplateMetadata{
			Name:    "notify-deploy",
			Version: "1.0.0",
		},
		Spec: TemplateSpec{
			Parameters: []TemplateParameter{
				{Name: "environment", Type: "string", Required: true},
				{Name: "channel", Type: "string", Default: "ops"},
			},
			Steps: []runbook.Step{
				{
					Name: "notify-start",
					Type: runbook.StepTypeNotification,
					Config: map[string]interface{}{
						"channel": "{{ .params.channel }}",
						"message": "Deploying to {{ .params.environment }}",
					},
				},
				{
					Name:      "notify-end",
					Type:      runbook.StepTypeNotification,
					DependsOn: []string{"notify-start"},
					Config: map[string]interface{}{
						"channel": "{{ .params.channel }}",
						"message": "Deployment to {{ .params.environment }} complete",
					},
				},
			},
		},
	}
	registry.Register(tmpl)

	expander := NewExpander(registry)

	t.Run("expand template reference", func(t *testing.T) {
		rb := &runbook.Runbook{
			APIVersion: "runbook.keystone.io/v1",
			Kind:       "Runbook",
			Metadata:   runbook.Metadata{Name: "test-runbook"},
			Spec: runbook.RunbookSpec{
				Steps: []runbook.Step{
					{
						Name: "deploy-notifications",
						Type: runbook.StepType("template"),
						Config: map[string]interface{}{
							"template": "notify-deploy",
							"parameters": map[string]interface{}{
								"environment": "production",
							},
						},
					},
				},
			},
		}

		expanded, err := expander.Expand(rb)
		if err != nil {
			t.Fatalf("Expand() error = %v", err)
		}

		// Should have 2 steps from the template
		if len(expanded.Spec.Steps) != 2 {
			t.Errorf("Expected 2 steps, got %d", len(expanded.Spec.Steps))
		}

		// Check parameter substitution
		if expanded.Spec.Steps[0].Config["message"] != "Deploying to production" {
			t.Errorf("Expected substituted message, got %v", expanded.Spec.Steps[0].Config["message"])
		}

		// Check default parameter
		if expanded.Spec.Steps[0].Config["channel"] != "ops" {
			t.Errorf("Expected default channel 'ops', got %v", expanded.Spec.Steps[0].Config["channel"])
		}
	})

	t.Run("missing required parameter", func(t *testing.T) {
		rb := &runbook.Runbook{
			Spec: runbook.RunbookSpec{
				Steps: []runbook.Step{
					{
						Name: "deploy",
						Type: runbook.StepType("template"),
						Config: map[string]interface{}{
							"template":   "notify-deploy",
							"parameters": map[string]interface{}{},
						},
					},
				},
			},
		}

		_, err := expander.Expand(rb)
		if err == nil {
			t.Error("Expected error for missing required parameter")
		}
	})

	t.Run("template not found", func(t *testing.T) {
		rb := &runbook.Runbook{
			Spec: runbook.RunbookSpec{
				Steps: []runbook.Step{
					{
						Name: "unknown",
						Type: runbook.StepType("template"),
						Config: map[string]interface{}{
							"template": "nonexistent",
						},
					},
				},
			},
		}

		_, err := expander.Expand(rb)
		if err == nil {
			t.Error("Expected error for nonexistent template")
		}
	})
}

func TestSubstituteString(t *testing.T) {
	params := map[string]interface{}{
		"env":     "production",
		"count":   5,
		"enabled": true,
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"{{ .params.env }}", "production"},
		{"{{.params.env}}", "production"},
		{"Deploy to {{ .params.env }}", "Deploy to production"},
		{"Count: {{ .params.count }}", "Count: 5"},
		{"Enabled: {{ .params.enabled }}", "Enabled: true"},
		{"No substitution", "No substitution"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := substituteString(tt.input, params)
			if result != tt.expected {
				t.Errorf("substituteString() = %q, want %q", result, tt.expected)
			}
		})
	}
}
