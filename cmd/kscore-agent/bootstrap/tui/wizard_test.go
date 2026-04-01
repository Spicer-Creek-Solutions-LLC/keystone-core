// Copyright 2024 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"testing"
)

func TestParsePort(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"valid port", "5432", 5432},
		{"port with whitespace", "  5432  ", 5432},
		{"empty string", "", 0},
		{"invalid port", "abc", 0},
		{"negative", "-1", -1},
		{"zero", "0", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parsePort(tt.input)
			if result != tt.expected {
				t.Errorf("parsePort(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"single value", "foo", []string{"foo"}},
		{"multiple values", "foo,bar,baz", []string{"foo", "bar", "baz"}},
		{"with whitespace", " foo , bar , baz ", []string{"foo", "bar", "baz"}},
		{"empty string", "", []string{}},
		{"empty values", "foo,,bar", []string{"foo", "bar"}},
		{"trailing comma", "foo,bar,", []string{"foo", "bar"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitCSV(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("splitCSV(%q) = %v, want %v", tt.input, result, tt.expected)
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("splitCSV(%q)[%d] = %q, want %q", tt.input, i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestFormatNodeLabels(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected string
	}{
		{
			name:     "empty labels",
			input:    map[string]string{},
			expected: "",
		},
		{
			name:     "single label",
			input:    map[string]string{"env": "prod"},
			expected: "env=prod",
		},
		{
			name:     "multiple labels sorted",
			input:    map[string]string{"env": "prod", "app": "web", "tier": "frontend"},
			expected: "app=web,env=prod,tier=frontend",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatNodeLabels(tt.input)
			if result != tt.expected {
				t.Errorf("formatNodeLabels(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatBlueprintParams(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]map[string]interface{}
		expected []string
	}{
		{
			name:     "empty params",
			input:    map[string]map[string]interface{}{},
			expected: nil,
		},
		{
			name: "single param",
			input: map[string]map[string]interface{}{
				"demo": {"name": "test"},
			},
			expected: []string{"demo:name=test"},
		},
		{
			name: "multiple params sorted",
			input: map[string]map[string]interface{}{
				"demo":     {"port": 8080, "name": "app"},
				"standard": {"replicas": 3},
			},
			expected: []string{"demo:name=app", "demo:port=8080", "standard:replicas=3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatBlueprintParams(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("formatBlueprintParams(%v) = %v, want %v", tt.input, result, tt.expected)
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("formatBlueprintParams(%v)[%d] = %q, want %q", tt.input, i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestFormatBlueprintFeatures(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]map[string]bool
		expected []string
	}{
		{
			name:     "empty features",
			input:    map[string]map[string]bool{},
			expected: nil,
		},
		{
			name: "single feature",
			input: map[string]map[string]bool{
				"demo": {"metrics": true},
			},
			expected: []string{"demo:metrics=true"},
		},
		{
			name: "multiple features sorted",
			input: map[string]map[string]bool{
				"demo":     {"metrics": true, "logs": false},
				"standard": {"ha": true},
			},
			expected: []string{"demo:logs=false", "demo:metrics=true", "standard:ha=true"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatBlueprintFeatures(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("formatBlueprintFeatures(%v) = %v, want %v", tt.input, result, tt.expected)
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("formatBlueprintFeatures(%v)[%d] = %q, want %q", tt.input, i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestFormatBlueprintEntrypoints(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected []string
	}{
		{
			name:     "empty entrypoints",
			input:    map[string]string{},
			expected: nil,
		},
		{
			name: "single entrypoint",
			input: map[string]string{
				"demo": "main",
			},
			expected: []string{"demo:main"},
		},
		{
			name: "multiple entrypoints sorted",
			input: map[string]string{
				"demo":     "main",
				"standard": "default",
			},
			expected: []string{"demo:main", "standard:default"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatBlueprintEntrypoints(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("formatBlueprintEntrypoints(%v) = %v, want %v", tt.input, result, tt.expected)
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("formatBlueprintEntrypoints(%v)[%d] = %q, want %q", tt.input, i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestIsFullscaleMode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"fullscale lowercase", "fullscale", true},
		{"fullscale uppercase", "FULLSCALE", true},
		{"fullscale mixed case", "FullScale", true},
		{"fullscale with whitespace", "  fullscale  ", true},
		{"demo mode", "demo", false},
		{"production mode", "production", false},
		{"custom mode", "custom", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isFullscaleMode(tt.input)
			if result != tt.expected {
				t.Errorf("isFullscaleMode(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestModeDescription(t *testing.T) {
	tests := []struct {
		name     string
		initial  WizardConfig
		mode     string
		base     string
		expected string
	}{
		{
			name:     "no hints or recommendation",
			initial:  WizardConfig{},
			mode:     "demo",
			base:     "Single-node demo",
			expected: "Single-node demo",
		},
		{
			name: "with hint",
			initial: WizardConfig{
				ModeHints: map[string]string{"demo": "Good for testing"},
			},
			mode:     "demo",
			base:     "Single-node demo",
			expected: "Single-node demo. Good for testing",
		},
		{
			name: "with recommendation",
			initial: WizardConfig{
				RecommendedMode: "demo",
			},
			mode:     "demo",
			base:     "Single-node demo",
			expected: "Single-node demo (recommended)",
		},
		{
			name: "with hint and recommendation",
			initial: WizardConfig{
				ModeHints:       map[string]string{"demo": "Good for testing"},
				RecommendedMode: "demo",
			},
			mode:     "demo",
			base:     "Single-node demo",
			expected: "Single-node demo. Good for testing (recommended)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := modeDescription(tt.initial, tt.mode, tt.base)
			if result != tt.expected {
				t.Errorf("modeDescription() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestModeListTitle(t *testing.T) {
	tests := []struct {
		name     string
		initial  WizardConfig
		expected string
	}{
		{
			name:     "no resource summary",
			initial:  WizardConfig{},
			expected: "Select deployment mode",
		},
		{
			name: "with resource summary",
			initial: WizardConfig{
				ResourceSummary: "4 CPU, 8GB RAM",
			},
			expected: "Select deployment mode (4 CPU, 8GB RAM)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := modeListTitle(tt.initial)
			if result != tt.expected {
				t.Errorf("modeListTitle() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestWizardConfig(t *testing.T) {
	config := WizardConfig{
		Mode:          "demo",
		ClusterName:   "test-cluster",
		NodeRole:      "both",
		NodeName:      "node-1",
		Storage:       "sqlite",
		NATSMode:      "embedded",
		HAEnabled:     false,
		GenerateCerts: true,
	}

	if config.Mode != "demo" {
		t.Errorf("expected Mode to be 'demo', got %s", config.Mode)
	}
	if config.ClusterName != "test-cluster" {
		t.Errorf("expected ClusterName to be 'test-cluster', got %s", config.ClusterName)
	}
	if config.NodeRole != "both" {
		t.Errorf("expected NodeRole to be 'both', got %s", config.NodeRole)
	}
	if config.HAEnabled {
		t.Error("expected HAEnabled to be false")
	}
	if !config.GenerateCerts {
		t.Error("expected GenerateCerts to be true")
	}
}

func TestModeItem(t *testing.T) {
	item := modeItem{
		mode:        "demo",
		title:       "Demo",
		description: "Single-node demo",
	}

	if item.Title() != "Demo" {
		t.Errorf("expected Title() to return 'Demo', got %s", item.Title())
	}
	if item.Description() != "Single-node demo" {
		t.Errorf("expected Description() to return 'Single-node demo', got %s", item.Description())
	}
	if item.FilterValue() != "Demo" {
		t.Errorf("expected FilterValue() to return 'Demo', got %s", item.FilterValue())
	}
}

func TestRoleItem(t *testing.T) {
	item := roleItem{
		role:        "control-plane",
		description: "Control plane services only",
	}

	if item.Title() != "control-plane" {
		t.Errorf("expected Title() to return 'control-plane', got %s", item.Title())
	}
	if item.Description() != "Control plane services only" {
		t.Errorf("expected Description() to return 'Control plane services only', got %s", item.Description())
	}
	if item.FilterValue() != "control-plane" {
		t.Errorf("expected FilterValue() to return 'control-plane', got %s", item.FilterValue())
	}
}

func TestNewModel(t *testing.T) {
	initial := WizardConfig{
		Mode:        "demo",
		ClusterName: "test-cluster",
	}

	model := newModel(initial)

	if model.step != stepMode {
		t.Errorf("expected initial step to be stepMode, got %v", model.step)
	}
	if model.done {
		t.Error("expected done to be false")
	}
	if model.err != nil {
		t.Errorf("expected err to be nil, got %v", model.err)
	}
	// The config is copied from initial, so Mode is preserved
	if model.config.Mode != "demo" {
		t.Errorf("expected config.Mode to be 'demo', got %s", model.config.Mode)
	}
	if model.config.ClusterName != "test-cluster" {
		t.Errorf("expected config.ClusterName to be 'test-cluster', got %s", model.config.ClusterName)
	}
}

func TestNewModelWithInitialValues(t *testing.T) {
	initial := WizardConfig{
		ClusterName:     "my-cluster",
		NodeRole:        "agent",
		Storage:         "postgres",
		PostgresHost:    "db.example.com",
		PostgresPort:    5432,
		HAEnabled:       true,
		HAReplicas:      3,
		NATSURLs:        []string{"nats://localhost:4222"},
		RecommendedMode: "production",
	}

	model := newModel(initial)

	// Check that initial values are set in text inputs
	if model.clusterInput.Value() != "my-cluster" {
		t.Errorf("expected clusterInput to have 'my-cluster', got %s", model.clusterInput.Value())
	}
	if model.postgresHostInput.Value() != "db.example.com" {
		t.Errorf("expected postgresHostInput to have 'db.example.com', got %s", model.postgresHostInput.Value())
	}
	if model.postgresPortInput.Value() != "5432" {
		t.Errorf("expected postgresPortInput to have '5432', got %s", model.postgresPortInput.Value())
	}
}

func TestWizardModelInit(t *testing.T) {
	model := newModel(WizardConfig{})
	cmd := model.Init()

	// Init() should return nil
	if cmd != nil {
		t.Error("expected Init() to return nil")
	}
}
