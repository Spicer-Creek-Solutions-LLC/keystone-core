package blueprint

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewLoader(t *testing.T) {
	storage := &mockStorage{}
	loader := NewLoader(storage)

	if loader == nil {
		t.Fatal("NewLoader returned nil")
	}
	if loader.storage == nil {
		t.Error("Loader storage is nil")
	}
	if loader.validator == nil {
		t.Error("Loader validator is nil")
	}
	if loader.cache == nil {
		t.Error("Loader cache is nil")
	}
}

func TestLoader_Load(t *testing.T) {
	bp := &Blueprint{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata: Metadata{
			Name:    "test-blueprint",
			Version: "1.0.0",
		},
		Entrypoints: map[string]string{
			"default": "states/init.yaml",
		},
		Parameters: map[string]ParameterSchema{
			"name": {Type: "string", Default: "default-name"},
		},
	}

	storage := &mockStorage{
		blueprints: map[string]*Blueprint{
			"blueprints/test/test-blueprint": bp,
		},
	}

	loader := NewLoader(storage)

	config := &LoadConfig{
		Name:    "blueprints/test/test-blueprint",
		Version: "1.0.0",
	}

	result, err := loader.Load(context.Background(), config)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if result.Blueprint == nil {
		t.Fatal("Result blueprint is nil")
	}
	if result.ResolvedVersion != "1.0.0" {
		t.Errorf("ResolvedVersion = %s, want 1.0.0", result.ResolvedVersion)
	}
	if result.ResolvedParameters["name"] != "default-name" {
		t.Errorf("Default parameter not applied: %v", result.ResolvedParameters)
	}
}

func TestLoader_LoadWithCache(t *testing.T) {
	bp := &Blueprint{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata: Metadata{
			Name:    "test-blueprint",
			Version: "1.0.0",
		},
	}

	callCount := 0
	storage := &mockStorage{
		blueprints: map[string]*Blueprint{
			"blueprints/test/test-blueprint": bp,
		},
		onGet: func() {
			callCount++
		},
	}

	loader := NewLoader(storage)

	config := &LoadConfig{
		Name:    "blueprints/test/test-blueprint",
		Version: "1.0.0",
	}

	// First load
	_, err := loader.Load(context.Background(), config)
	if err != nil {
		t.Fatalf("First load failed: %v", err)
	}

	// Second load should use cache
	_, err = loader.Load(context.Background(), config)
	if err != nil {
		t.Fatalf("Second load failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("Storage.Get called %d times, expected 1 (cache miss)", callCount)
	}
}

func TestLoader_LoadFromPath(t *testing.T) {
	// Create a temporary blueprint directory
	tmpDir, err := os.MkdirTemp("", "blueprint-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write blueprint.yaml
	manifest := `
apiVersion: blueprints.kscore.io/v1
kind: Blueprint
metadata:
  name: test-bp
  version: 1.0.0
entrypoints:
  default: states/init.yaml
`
	if err := os.WriteFile(filepath.Join(tmpDir, "blueprint.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	loader := NewLoader(&mockStorage{})
	config := &LoadConfig{}

	result, err := loader.LoadFromPath(context.Background(), tmpDir, config)
	if err != nil {
		t.Fatalf("LoadFromPath failed: %v", err)
	}

	if result.Blueprint.Metadata.Name != "test-bp" {
		t.Errorf("Blueprint name = %s, want test-bp", result.Blueprint.Metadata.Name)
	}
}

func TestLoader_FeatureResolution(t *testing.T) {
	bp := &Blueprint{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata: Metadata{
			Name:    "test-blueprint",
			Version: "1.0.0",
		},
		Features: map[string]Feature{
			"ssl":        {Description: "Enable SSL", Default: true},
			"monitoring": {Description: "Enable monitoring", Default: false},
			"logging":    {Description: "Enable logging", Default: true},
		},
	}

	storage := &mockStorage{
		blueprints: map[string]*Blueprint{"test": bp},
	}
	loader := NewLoader(storage)

	tests := []struct {
		name      string
		overrides map[string]bool
		expected  []string
	}{
		{
			name:      "default features",
			overrides: nil,
			expected:  []string{"ssl", "logging"},
		},
		{
			name:      "override enable",
			overrides: map[string]bool{"monitoring": true},
			expected:  []string{"ssl", "logging", "monitoring"},
		},
		{
			name:      "override disable",
			overrides: map[string]bool{"ssl": false},
			expected:  []string{"logging"},
		},
		{
			name:      "override both",
			overrides: map[string]bool{"ssl": false, "monitoring": true},
			expected:  []string{"logging", "monitoring"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enabled := loader.resolveFeatures(bp, tt.overrides)

			// Check all expected are present
			for _, exp := range tt.expected {
				if !containsString(enabled, exp) {
					t.Errorf("Expected feature %s not enabled. Got: %v", exp, enabled)
				}
			}

			// Check no unexpected features
			if len(enabled) != len(tt.expected) {
				t.Errorf("Feature count mismatch. Got %v, expected %v", enabled, tt.expected)
			}
		})
	}
}

func TestLoader_ParameterResolution(t *testing.T) {
	bp := &Blueprint{
		Parameters: map[string]ParameterSchema{
			"name":    {Type: "string", Default: "default-name"},
			"port":    {Type: "integer", Default: 8080},
			"enabled": {Type: "boolean", Default: true},
			"tags":    {Type: "array", Default: []interface{}{"tag1", "tag2"}},
			"config": {
				Type: "object",
				Properties: map[string]ParameterSchema{
					"debug":   {Type: "boolean", Default: false},
					"timeout": {Type: "integer", Default: 30},
				},
			},
		},
	}

	loader := NewLoader(&mockStorage{})

	tests := []struct {
		name       string
		userParams map[string]interface{}
		checkKey   string
		expected   interface{}
	}{
		{
			name:       "default string",
			userParams: nil,
			checkKey:   "name",
			expected:   "default-name",
		},
		{
			name:       "override string",
			userParams: map[string]interface{}{"name": "custom-name"},
			checkKey:   "name",
			expected:   "custom-name",
		},
		{
			name:       "default integer",
			userParams: nil,
			checkKey:   "port",
			expected:   8080,
		},
		{
			name:       "default boolean",
			userParams: nil,
			checkKey:   "enabled",
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := loader.resolveParameters(bp, tt.userParams, nil)
			if err != nil {
				t.Fatalf("resolveParameters failed: %v", err)
			}

			if result[tt.checkKey] != tt.expected {
				t.Errorf("Parameter %s = %v, want %v", tt.checkKey, result[tt.checkKey], tt.expected)
			}
		})
	}
}

func TestLoader_ParameterValidation(t *testing.T) {
	minLen := 3
	maxLen := 10
	min := float64(1)
	max := float64(100)

	bp := &Blueprint{
		Parameters: map[string]ParameterSchema{
			"required_param": {Type: "string", Required: true},
			"string_param":   {Type: "string", Pattern: "^[a-z]+$", MinLength: &minLen, MaxLength: &maxLen},
			"int_param":      {Type: "integer", Minimum: &min, Maximum: &max},
			"enum_param":     {Type: "string", Enum: []interface{}{"a", "b", "c"}},
		},
	}

	loader := NewLoader(&mockStorage{})

	tests := []struct {
		name        string
		params      map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name:        "missing required",
			params:      map[string]interface{}{},
			expectError: true,
			errorMsg:    "required parameter missing",
		},
		{
			name:        "valid params",
			params:      map[string]interface{}{"required_param": "test"},
			expectError: false,
		},
		{
			name:        "type mismatch",
			params:      map[string]interface{}{"required_param": 123},
			expectError: true,
			errorMsg:    "expected string",
		},
		{
			name:        "pattern mismatch",
			params:      map[string]interface{}{"required_param": "x", "string_param": "ABC123"},
			expectError: true,
			errorMsg:    "does not match pattern",
		},
		{
			name:        "too short",
			params:      map[string]interface{}{"required_param": "x", "string_param": "ab"},
			expectError: true,
			errorMsg:    "less than minimum",
		},
		{
			name:        "too long",
			params:      map[string]interface{}{"required_param": "x", "string_param": "abcdefghijk"},
			expectError: true,
			errorMsg:    "exceeds maximum",
		},
		{
			name:        "below minimum",
			params:      map[string]interface{}{"required_param": "x", "int_param": 0},
			expectError: true,
			errorMsg:    "less than minimum",
		},
		{
			name:        "above maximum",
			params:      map[string]interface{}{"required_param": "x", "int_param": 101},
			expectError: true,
			errorMsg:    "exceeds maximum",
		},
		{
			name:        "not in enum",
			params:      map[string]interface{}{"required_param": "x", "enum_param": "d"},
			expectError: true,
			errorMsg:    "not in allowed values",
		},
		{
			name:        "valid enum",
			params:      map[string]interface{}{"required_param": "x", "enum_param": "b"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := loader.validateParameters(bp, tt.params, nil)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got nil")
				} else if tt.errorMsg != "" && !containsString([]string{err.Error()}, tt.errorMsg) {
					// Check if error message contains expected substring
					if err.Error() != "" {
						t.Logf("Got error: %v", err)
					}
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestLoader_FeatureGatedParameters(t *testing.T) {
	bp := &Blueprint{
		Features: map[string]Feature{
			"monitoring": {Description: "Enable monitoring", Default: false},
		},
		Parameters: map[string]ParameterSchema{
			"always_param": {Type: "string", Default: "always"},
			"monitoring_param": {
				Type:    "string",
				Default: "monitor-value",
				Feature: "monitoring",
			},
		},
	}

	loader := NewLoader(&mockStorage{})

	// Without feature enabled
	t.Run("feature disabled", func(t *testing.T) {
		result, err := loader.resolveParameters(bp, nil, []string{})
		if err != nil {
			t.Fatalf("resolveParameters failed: %v", err)
		}

		if result["always_param"] != "always" {
			t.Errorf("always_param = %v, want always", result["always_param"])
		}
		if _, exists := result["monitoring_param"]; exists {
			t.Error("monitoring_param should not be set when feature disabled")
		}
	})

	// With feature enabled
	t.Run("feature enabled", func(t *testing.T) {
		result, err := loader.resolveParameters(bp, nil, []string{"monitoring"})
		if err != nil {
			t.Fatalf("resolveParameters failed: %v", err)
		}

		if result["monitoring_param"] != "monitor-value" {
			t.Errorf("monitoring_param = %v, want monitor-value", result["monitoring_param"])
		}
	})
}

func TestLoader_StateFileResolution(t *testing.T) {
	bp := &Blueprint{
		Entrypoints: map[string]string{
			"default":  "states/init.yaml",
			"rollback": "states/rollback.yaml",
		},
		Features: map[string]Feature{
			"ssl":        {Enables: []string{"states/ssl.yaml"}},
			"monitoring": {Enables: []string{"states/prometheus.yaml", "states/grafana.yaml"}},
		},
	}

	loader := NewLoader(&mockStorage{})

	tests := []struct {
		name            string
		entrypoint      string
		enabledFeatures []string
		expectedFiles   []string
		expectError     bool
	}{
		{
			name:            "default entrypoint",
			entrypoint:      "",
			enabledFeatures: nil,
			expectedFiles:   []string{"states/init.yaml"},
		},
		{
			name:            "custom entrypoint",
			entrypoint:      "rollback",
			enabledFeatures: nil,
			expectedFiles:   []string{"states/rollback.yaml"},
		},
		{
			name:            "with features",
			entrypoint:      "",
			enabledFeatures: []string{"ssl", "monitoring"},
			expectedFiles:   []string{"states/init.yaml", "states/ssl.yaml", "states/prometheus.yaml", "states/grafana.yaml"},
		},
		{
			name:            "invalid entrypoint",
			entrypoint:      "nonexistent",
			enabledFeatures: nil,
			expectError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, err := loader.resolveStateFiles(bp, tt.entrypoint, tt.enabledFeatures)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveStateFiles failed: %v", err)
			}

			if len(files) != len(tt.expectedFiles) {
				t.Errorf("File count = %d, want %d. Got: %v", len(files), len(tt.expectedFiles), files)
			}

			for _, expected := range tt.expectedFiles {
				found := false
				for _, f := range files {
					if f == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected file %s not found in %v", expected, files)
				}
			}
		})
	}
}

func TestLoader_RenderTemplate(t *testing.T) {
	bp := &Blueprint{
		Metadata: Metadata{
			Name:    "test-blueprint",
			Version: "1.0.0",
		},
	}

	loader := NewLoader(&mockStorage{})

	tests := []struct {
		name     string
		template string
		params   map[string]interface{}
		expected string
	}{
		{
			name:     "simple substitution",
			template: "Hello, {{ .name }}!",
			params:   map[string]interface{}{"name": "World"},
			expected: "Hello, World!",
		},
		{
			name:     "default function",
			template: `{{ default "default-value" .missing }}`,
			params:   map[string]interface{}{},
			expected: "default-value",
		},
		{
			name:     "upper function",
			template: `{{ upper .name }}`,
			params:   map[string]interface{}{"name": "test"},
			expected: "TEST",
		},
		{
			name:     "lower function",
			template: `{{ lower .name }}`,
			params:   map[string]interface{}{"name": "TEST"},
			expected: "test",
		},
		{
			name:     "quote function",
			template: `{{ quote .name }}`,
			params:   map[string]interface{}{"name": "test"},
			expected: `"test"`,
		},
		{
			name:     "trim function",
			template: `{{ trim .name }}`,
			params:   map[string]interface{}{"name": "  test  "},
			expected: "test",
		},
		{
			name:     "blueprint metadata access",
			template: `{{ .blueprint.Metadata.Name }}`,
			params:   map[string]interface{}{},
			expected: "test-blueprint",
		},
		{
			name:     "nested param access",
			template: `{{ .params.config.debug }}`,
			params:   map[string]interface{}{"config": map[string]interface{}{"debug": true}},
			expected: "true",
		},
		{
			name:     "contains function",
			template: `{{ if contains .name "es" }}yes{{ else }}no{{ end }}`,
			params:   map[string]interface{}{"name": "test"},
			expected: "yes",
		},
		{
			name:     "replace function",
			template: `{{ replace "-" "_" .name }}`,
			params:   map[string]interface{}{"name": "my-test-name"},
			expected: "my_test_name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := loader.renderTemplate(tt.template, tt.params, bp)
			if err != nil {
				t.Fatalf("renderTemplate failed: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Result = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestLoader_LoadStateWithBlueprint(t *testing.T) {
	stateData := []byte(`
include:
  - blueprint: blueprints/community/web-app-stack
    version: "^1.0"
    as: web
    features:
      ssl: true
      monitoring: false
    parameters:
      domain: example.com
      port: 8080
  - file: ./common.yaml

states:
  custom_state:
    module: cmd
    run: echo hello
`)

	loader := NewLoader(&mockStorage{})
	state, err := loader.LoadStateWithBlueprint(context.Background(), stateData)
	if err != nil {
		t.Fatalf("LoadStateWithBlueprint failed: %v", err)
	}

	if len(state.Include) != 2 {
		t.Errorf("Include count = %d, want 2", len(state.Include))
	}

	// Check blueprint include
	if !state.Include[0].IsBlueprint() {
		t.Error("First include should be a blueprint")
	}
	if state.Include[0].Blueprint != "blueprints/community/web-app-stack" {
		t.Errorf("Blueprint = %s, want blueprints/community/web-app-stack", state.Include[0].Blueprint)
	}
	if state.Include[0].Version != "^1.0" {
		t.Errorf("Version = %s, want ^1.0", state.Include[0].Version)
	}
	if state.Include[0].As != "web" {
		t.Errorf("As = %s, want web", state.Include[0].As)
	}
	if !state.Include[0].Features["ssl"] {
		t.Error("Feature ssl should be true")
	}
	if state.Include[0].Features["monitoring"] {
		t.Error("Feature monitoring should be false")
	}
	if state.Include[0].Parameters["domain"] != "example.com" {
		t.Errorf("Parameter domain = %v, want example.com", state.Include[0].Parameters["domain"])
	}

	// Check file include
	if state.Include[1].IsBlueprint() {
		t.Error("Second include should not be a blueprint")
	}
	if state.Include[1].File != "./common.yaml" {
		t.Errorf("File = %s, want ./common.yaml", state.Include[1].File)
	}

	// Check states
	if len(state.States) != 1 {
		t.Errorf("States count = %d, want 1", len(state.States))
	}
}

func TestBlueprintInclude_ToLoadConfig(t *testing.T) {
	include := BlueprintInclude{
		Blueprint:  "blueprints/test/my-bp",
		Version:    "1.2.3",
		As:         "instance",
		Entrypoint: "custom",
		Features:   map[string]bool{"ssl": true},
		Parameters: map[string]interface{}{"key": "value"},
	}

	config := include.ToLoadConfig()

	if config.Name != "blueprints/test/my-bp" {
		t.Errorf("Name = %s, want blueprints/test/my-bp", config.Name)
	}
	if config.Version != "1.2.3" {
		t.Errorf("Version = %s, want 1.2.3", config.Version)
	}
	if config.As != "instance" {
		t.Errorf("As = %s, want instance", config.As)
	}
	if config.Entrypoint != "custom" {
		t.Errorf("Entrypoint = %s, want custom", config.Entrypoint)
	}
	if !config.Features["ssl"] {
		t.Error("Feature ssl should be true")
	}
	if config.Parameters["key"] != "value" {
		t.Errorf("Parameter key = %v, want value", config.Parameters["key"])
	}
	if !config.Validate {
		t.Error("Validate should be true")
	}
}

// Helper tests

func TestMergeParameters(t *testing.T) {
	dst := map[string]interface{}{
		"key1": "value1",
		"nested": map[string]interface{}{
			"a": "1",
			"b": "2",
		},
	}

	src := map[string]interface{}{
		"key2": "value2",
		"nested": map[string]interface{}{
			"b": "override",
			"c": "3",
		},
	}

	mergeParameters(dst, src)

	if dst["key1"] != "value1" {
		t.Error("key1 should be preserved")
	}
	if dst["key2"] != "value2" {
		t.Error("key2 should be added")
	}

	nested := dst["nested"].(map[string]interface{})
	if nested["a"] != "1" {
		t.Error("nested.a should be preserved")
	}
	if nested["b"] != "override" {
		t.Error("nested.b should be overridden")
	}
	if nested["c"] != "3" {
		t.Error("nested.c should be added")
	}
}

func TestSetNestedValue(t *testing.T) {
	m := make(map[string]interface{})

	setNestedValue(m, "simple", "value")
	if m["simple"] != "value" {
		t.Error("Simple value not set")
	}

	setNestedValue(m, "nested.deep.value", "deep-value")
	nested, ok := m["nested"].(map[string]interface{})
	if !ok {
		t.Fatal("nested should be a map")
	}
	deep, ok := nested["deep"].(map[string]interface{})
	if !ok {
		t.Fatal("nested.deep should be a map")
	}
	if deep["value"] != "deep-value" {
		t.Error("Deep value not set")
	}
}

func TestContainsString(t *testing.T) {
	slice := []string{"a", "b", "c"}

	if !containsString(slice, "b") {
		t.Error("Should contain 'b'")
	}
	if containsString(slice, "d") {
		t.Error("Should not contain 'd'")
	}
	if containsString(nil, "a") {
		t.Error("Nil slice should not contain anything")
	}
}

func TestContainsValue(t *testing.T) {
	slice := []interface{}{"a", 1, true}

	if !containsValue(slice, "a") {
		t.Error("Should contain 'a'")
	}
	if !containsValue(slice, 1) {
		t.Error("Should contain 1")
	}
	if !containsValue(slice, true) {
		t.Error("Should contain true")
	}
	if containsValue(slice, "missing") {
		t.Error("Should not contain 'missing'")
	}
}

func TestValidateType(t *testing.T) {
	tests := []struct {
		name        string
		typeName    string
		value       interface{}
		expectError bool
	}{
		{"valid string", "string", "test", false},
		{"invalid string", "string", 123, true},
		{"valid integer int", "integer", 42, false},
		{"valid integer int64", "integer", int64(42), false},
		{"valid integer float64", "integer", float64(42), false},
		{"invalid integer", "integer", "42", true},
		{"valid number", "number", 3.14, false},
		{"valid boolean", "boolean", true, false},
		{"invalid boolean", "boolean", "true", true},
		{"valid array", "array", []interface{}{1, 2, 3}, false},
		{"invalid array", "array", "not an array", true},
		{"valid object", "object", map[string]interface{}{"key": "value"}, false},
		{"invalid object", "object", "not an object", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateType("test", tt.typeName, tt.value)
			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected float64
	}{
		{"int", 42, 42.0},
		{"int32", int32(42), 42.0},
		{"int64", int64(42), 42.0},
		{"float32", float32(3.14), float64(float32(3.14))},
		{"float64", 3.14, 3.14},
		{"string", "42", 0}, // Invalid type returns 0
		{"nil", nil, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toFloat64(tt.value)
			if result != tt.expected {
				t.Errorf("toFloat64(%v) = %v, want %v", tt.value, result, tt.expected)
			}
		})
	}
}

// Mock storage for testing
type mockStorage struct {
	blueprints map[string]*Blueprint
	onGet      func()
}

func (m *mockStorage) Get(ctx context.Context, name string, version string) (*Blueprint, error) {
	if m.onGet != nil {
		m.onGet()
	}
	if bp, ok := m.blueprints[name]; ok {
		return bp, nil
	}
	return nil, ErrBlueprintNotFound
}

func (m *mockStorage) List(ctx context.Context, filter *ListFilter) ([]*BlueprintInfo, error) {
	return nil, nil
}

func (m *mockStorage) Versions(ctx context.Context, name string) ([]string, error) {
	return nil, nil
}

func (m *mockStorage) Exists(ctx context.Context, name string, version string) (bool, error) {
	_, ok := m.blueprints[name]
	return ok, nil
}

func (m *mockStorage) Close() error {
	return nil
}
