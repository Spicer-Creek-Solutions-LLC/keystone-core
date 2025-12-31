package statemgmt

import (
	"strings"
	"testing"
)

func TestTemplateRenderer_BasicRender(t *testing.T) {
	renderer := NewTemplateRenderer()
	ctx := &TemplateContext{
		Vars: map[string]interface{}{
			"name": "Keystone Core",
			"port": 8080,
		},
		Facts: map[string]interface{}{
			"environment": "production",
		},
	}

	template := "Application {{.vars.name}} on port {{.vars.port}} in {{.facts.environment}}"
	result, err := renderer.Render(template, ctx)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	expected := "Application Keystone Core on port 8080 in production"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestTemplateRenderer_NestedData(t *testing.T) {
	renderer := NewTemplateRenderer()
	ctx := &TemplateContext{
		Vars: map[string]interface{}{
			"database": map[string]interface{}{
				"host": "db.example.com",
				"port": 5432,
			},
		},
		Facts: map[string]interface{}{},
	}

	template := "host: {{.vars.database.host}}:{{.vars.database.port}}"
	result, err := renderer.Render(template, ctx)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	expected := "host: db.example.com:5432"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestTemplateRenderer_CustomFunctions(t *testing.T) {
	renderer := NewTemplateRenderer()
	ctx := &TemplateContext{
		Vars: map[string]interface{}{
			"name": "keystoneCore",
		},
		Facts: map[string]interface{}{},
	}

	tests := []struct {
		name     string
		template string
		expected string
	}{
		{"upper", "{{.vars.name | upper}}", "KEYSTONECORE"},
		{"lower", "{{.vars.name | lower}}", "keystonecore"},
		{"title", "{{.vars.name | title}}", "KeystoneCore"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := renderer.Render(tt.template, ctx)
			if err != nil {
				t.Fatalf("Render failed: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestTemplateRenderer_DefaultFunction(t *testing.T) {
	renderer := NewTemplateRenderer()
	ctx := &TemplateContext{
		Vars: map[string]interface{}{
			"replicas": nil,
		},
		Facts: map[string]interface{}{},
	}

	template := "replicas: {{.vars.replicas | default 3}}"
	result, err := renderer.Render(template, ctx)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	expected := "replicas: 3"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestTemplateRenderer_Conditional(t *testing.T) {
	renderer := NewTemplateRenderer()
	ctx := &TemplateContext{
		Vars: map[string]interface{}{
			"is_production": true,
		},
		Facts: map[string]interface{}{},
	}

	template := "{{if .vars.is_production}}production{{else}}development{{end}}"
	result, err := renderer.Render(template, ctx)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	expected := "production"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestVars_SetGet(t *testing.T) {
	vars := NewVars()
	vars.Set("key", "value")

	val, ok := vars.Get("key")
	if !ok {
		t.Fatal("Expected key to exist")
	}

	if val != "value" {
		t.Errorf("Expected 'value', got '%v'", val)
	}

	_, ok = vars.Get("nonexistent")
	if ok {
		t.Error("Expected key to not exist")
	}
}

func TestVars_Merge(t *testing.T) {
	vars1 := NewVars()
	vars1.Set("a", 1)
	vars1.Set("b", 2)

	vars2 := NewVars()
	vars2.Set("b", 3)
	vars2.Set("c", 4)

	vars1.Merge(vars2)

	// Check that b was overwritten
	val, _ := vars1.Get("b")
	if val != 3 {
		t.Errorf("Expected b=3, got b=%v", val)
	}

	// Check that c was added
	val, ok := vars1.Get("c")
	if !ok {
		t.Fatal("Expected c to exist")
	}
	if val != 4 {
		t.Errorf("Expected c=4, got c=%v", val)
	}

	// Check that a is still there
	val, _ = vars1.Get("a")
	if val != 1 {
		t.Errorf("Expected a=1, got a=%v", val)
	}
}

func TestFacts_SystemFacts(t *testing.T) {
	facts := NewFacts()

	// Check that system facts are collected
	os, ok := facts.Get("os")
	if !ok {
		t.Fatal("Expected 'os' fact to exist")
	}
	if os == "" {
		t.Error("Expected 'os' fact to have a value")
	}

	arch, ok := facts.Get("arch")
	if !ok {
		t.Fatal("Expected 'arch' fact to exist")
	}
	if arch == "" {
		t.Error("Expected 'arch' fact to have a value")
	}

	numCPU, ok := facts.Get("num_cpu")
	if !ok {
		t.Fatal("Expected 'num_cpu' fact to exist")
	}
	if numCPU.(int) <= 0 {
		t.Error("Expected 'num_cpu' to be > 0")
	}
}

func TestFacts_CustomFacts(t *testing.T) {
	facts := NewFacts()
	facts.Set("environment", "production")
	facts.Set("datacenter", "us-east-1")

	env := facts.GetString("environment")
	if env != "production" {
		t.Errorf("Expected 'production', got '%s'", env)
	}

	dc := facts.GetString("datacenter")
	if dc != "us-east-1" {
		t.Errorf("Expected 'us-east-1', got '%s'", dc)
	}
}

func TestLoadVarsFromYAML(t *testing.T) {
	data := map[string]interface{}{
		"db_host": "localhost",
		"db_port": 5432,
		"nested": map[string]interface{}{
			"key": "value",
		},
	}

	vars := LoadVarsFromYAML(data)

	host, ok := vars.Get("db_host")
	if !ok {
		t.Fatal("Expected 'db_host' to exist")
	}
	if host != "localhost" {
		t.Errorf("Expected 'localhost', got '%v'", host)
	}
}

func TestRenderStateFile(t *testing.T) {
	stateFile := &StateFile{
		Path: "test.yaml",
		Metadata: StateMetadata{
			Description: "Deploy {{.vars.app_name}} to {{.facts.environment}}",
		},
		States: map[string][]StateDeclaration{
			"file": {
				{
					Module: "file",
					ID:     "/etc/app/config.yml",
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "db_host: {{.vars.db_host}}\nenv: {{.facts.environment}}",
					},
				},
			},
		},
	}

	vars := NewVars()
	vars.Set("app_name", "MyApp")
	vars.Set("db_host", "db.example.com")

	facts := NewFacts()
	facts.Set("environment", "production")

	err := RenderStateFile(stateFile, vars, facts)
	if err != nil {
		t.Fatalf("RenderStateFile failed: %v", err)
	}

	// Check metadata was rendered
	if !strings.Contains(stateFile.Metadata.Description, "MyApp") {
		t.Error("Expected metadata to contain 'MyApp'")
	}
	if !strings.Contains(stateFile.Metadata.Description, "production") {
		t.Error("Expected metadata to contain 'production'")
	}

	// Check file contents were rendered
	contents := stateFile.States["file"][0].Parameters["contents"].(string)
	if !strings.Contains(contents, "db.example.com") {
		t.Error("Expected contents to contain 'db.example.com'")
	}
	if !strings.Contains(contents, "production") {
		t.Error("Expected contents to contain 'production'")
	}
}

func TestTemplateRenderer_ErrorHandling(t *testing.T) {
	renderer := NewTemplateRenderer()
	ctx := &TemplateContext{
		Vars:  map[string]interface{}{},
		Facts: map[string]interface{}{},
	}

	// Invalid template syntax
	template := "{{.vars.name"
	_, err := renderer.Render(template, ctx)
	if err == nil {
		t.Error("Expected error for invalid template syntax")
	}

	// Missing variable (should not error, just output empty)
	template = "{{.vars.nonexistent}}"
	result, err := renderer.Render(template, ctx)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "<no value>" {
		t.Logf("Got result: '%s'", result)
	}
}
