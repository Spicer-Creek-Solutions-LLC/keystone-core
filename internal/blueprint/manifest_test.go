package blueprint

import (
	"testing"
)

func TestParseManifest_Valid(t *testing.T) {
	data := []byte(`
apiVersion: blueprints.kscore.io/v1
kind: Blueprint

metadata:
  name: web-app-stack
  version: 1.2.0
  description: A web application stack
  maintainers:
    - name: Test Team
      email: test@example.com
  license: Apache-2.0
  keywords:
    - web
    - nginx
  categories:
    - application-stack

compatibility:
  kscore: ">=1.5.0"
  modules:
    - modules/std/files@^1.0
  platforms:
    - os: linux
      family: debian
      versions: ["11", "12"]

dependencies:
  requires:
    - blueprints/community/base-system@^1.0
  requires_before:
    - blueprints/community/ssl-certificates@^2.0

features:
  ssl:
    description: Enable SSL
    default: true
    enables:
      - states/nginx/ssl.yaml
  monitoring:
    description: Enable monitoring
    default: false
    parameters:
      - monitoring.*

entrypoints:
  default: states/init.yaml
  rollback: states/rollback.yaml
  nginx_only: states/nginx/init.yaml

parameters:
  app_name:
    type: string
    required: true
    description: Application name
    pattern: "^[a-z][a-z0-9-]{2,30}$"
  port:
    type: integer
    default: 3000
    minimum: 1024
    maximum: 65535
  domains:
    type: array
    items:
      type: string
    default: []
  nginx:
    type: object
    properties:
      worker_processes:
        type: integer
        default: 4

outputs:
  app_url:
    description: Application URL
    value: "https://{{ domain }}"
  config_path:
    description: Config path
    value: "/etc/app/{{ app_name }}"
    sensitive: false

hooks:
  pre_apply:
    - states/hooks/pre-apply.yaml
  post_apply:
    - states/hooks/post-apply.yaml
`)

	bp, err := ParseManifest(data)
	if err != nil {
		t.Fatalf("ParseManifest failed: %v", err)
	}

	// Verify basic fields
	if bp.APIVersion != APIVersion {
		t.Errorf("APIVersion = %s, want %s", bp.APIVersion, APIVersion)
	}
	if bp.Kind != Kind {
		t.Errorf("Kind = %s, want %s", bp.Kind, Kind)
	}

	// Verify metadata
	if bp.Metadata.Name != "web-app-stack" {
		t.Errorf("Metadata.Name = %s, want web-app-stack", bp.Metadata.Name)
	}
	if bp.Metadata.Version != "1.2.0" {
		t.Errorf("Metadata.Version = %s, want 1.2.0", bp.Metadata.Version)
	}
	if len(bp.Metadata.Maintainers) != 1 {
		t.Errorf("Metadata.Maintainers count = %d, want 1", len(bp.Metadata.Maintainers))
	}
	if len(bp.Metadata.Keywords) != 2 {
		t.Errorf("Metadata.Keywords count = %d, want 2", len(bp.Metadata.Keywords))
	}

	// Verify compatibility
	if bp.Compatibility == nil {
		t.Fatal("Compatibility is nil")
	}
	if bp.Compatibility.Kscore != ">=1.5.0" {
		t.Errorf("Compatibility.Kscore = %s, want >=1.5.0", bp.Compatibility.Kscore)
	}
	if len(bp.Compatibility.Modules) != 1 {
		t.Errorf("Compatibility.Modules count = %d, want 1", len(bp.Compatibility.Modules))
	}
	if len(bp.Compatibility.Platforms) != 1 {
		t.Errorf("Compatibility.Platforms count = %d, want 1", len(bp.Compatibility.Platforms))
	}

	// Verify dependencies
	if bp.Dependencies == nil {
		t.Fatal("Dependencies is nil")
	}
	if len(bp.Dependencies.Requires) != 1 {
		t.Errorf("Dependencies.Requires count = %d, want 1", len(bp.Dependencies.Requires))
	}
	if len(bp.Dependencies.RequiresBefore) != 1 {
		t.Errorf("Dependencies.RequiresBefore count = %d, want 1", len(bp.Dependencies.RequiresBefore))
	}

	// Verify features
	if len(bp.Features) != 2 {
		t.Errorf("Features count = %d, want 2", len(bp.Features))
	}
	if !bp.HasFeature("ssl") {
		t.Error("HasFeature(ssl) = false, want true")
	}
	if !bp.IsFeatureEnabled("ssl") {
		t.Error("IsFeatureEnabled(ssl) = false, want true")
	}
	if bp.IsFeatureEnabled("monitoring") {
		t.Error("IsFeatureEnabled(monitoring) = true, want false")
	}

	// Verify entrypoints
	if len(bp.Entrypoints) != 3 {
		t.Errorf("Entrypoints count = %d, want 3", len(bp.Entrypoints))
	}
	if bp.DefaultEntrypoint() != "states/init.yaml" {
		t.Errorf("DefaultEntrypoint() = %s, want states/init.yaml", bp.DefaultEntrypoint())
	}
	if bp.GetEntrypoint("rollback") != "states/rollback.yaml" {
		t.Errorf("GetEntrypoint(rollback) = %s, want states/rollback.yaml", bp.GetEntrypoint("rollback"))
	}
	if bp.GetEntrypoint("nonexistent") != "" {
		t.Errorf("GetEntrypoint(nonexistent) = %s, want empty", bp.GetEntrypoint("nonexistent"))
	}

	// Verify parameters
	if len(bp.Parameters) != 4 {
		t.Errorf("Parameters count = %d, want 4", len(bp.Parameters))
	}

	appNameParam, ok := bp.GetParameter("app_name")
	if !ok {
		t.Fatal("GetParameter(app_name) failed")
	}
	if appNameParam.Type != "string" {
		t.Errorf("app_name.Type = %s, want string", appNameParam.Type)
	}
	if !appNameParam.Required {
		t.Error("app_name.Required = false, want true")
	}

	// Test nested parameter access
	workerParam, ok := bp.GetParameter("nginx.worker_processes")
	if !ok {
		t.Fatal("GetParameter(nginx.worker_processes) failed")
	}
	if workerParam.Type != "integer" {
		t.Errorf("nginx.worker_processes.Type = %s, want integer", workerParam.Type)
	}

	// Verify required parameters
	required := bp.RequiredParameters()
	if len(required) != 1 || required[0] != "app_name" {
		t.Errorf("RequiredParameters() = %v, want [app_name]", required)
	}

	// Verify outputs
	if len(bp.Outputs) != 2 {
		t.Errorf("Outputs count = %d, want 2", len(bp.Outputs))
	}

	// Verify hooks
	if bp.Hooks == nil {
		t.Fatal("Hooks is nil")
	}
	if len(bp.Hooks.PreApply) != 1 {
		t.Errorf("Hooks.PreApply count = %d, want 1", len(bp.Hooks.PreApply))
	}
}

func TestParseManifest_Minimal(t *testing.T) {
	data := []byte(`
apiVersion: blueprints.kscore.io/v1
kind: Blueprint
metadata:
  name: minimal
  version: 0.1.0
`)

	bp, err := ParseManifest(data)
	if err != nil {
		t.Fatalf("ParseManifest failed: %v", err)
	}

	if bp.Metadata.Name != "minimal" {
		t.Errorf("Metadata.Name = %s, want minimal", bp.Metadata.Name)
	}

	// Verify default entrypoint
	if bp.DefaultEntrypoint() != "states/init.yaml" {
		t.Errorf("DefaultEntrypoint() = %s, want states/init.yaml", bp.DefaultEntrypoint())
	}

	// Verify no features
	if len(bp.Features) != 0 {
		t.Errorf("Features count = %d, want 0", len(bp.Features))
	}
}

func TestParseManifest_Invalid(t *testing.T) {
	data := []byte(`not valid yaml: [`)

	_, err := ParseManifest(data)
	if err == nil {
		t.Fatal("ParseManifest should fail on invalid YAML")
	}
}

func TestBlueprint_FullName(t *testing.T) {
	bp := &Blueprint{
		Metadata: Metadata{
			Name: "web-app-stack",
		},
	}

	// Without source path
	if got := bp.FullName(); got != "blueprints/web-app-stack" {
		t.Errorf("FullName() = %s, want blueprints/web-app-stack", got)
	}

	// With source path
	bp.SourcePath = "/path/to/blueprints/community/web-app-stack"
	if got := bp.FullName(); got != "blueprints/community/web-app-stack" {
		t.Errorf("FullName() = %s, want blueprints/community/web-app-stack", got)
	}
}

func TestBlueprint_SensitiveParameters(t *testing.T) {
	bp := &Blueprint{
		Parameters: map[string]ParameterSchema{
			"username": {Type: "string"},
			"password": {Type: "string", Sensitive: true},
			"config": {
				Type: "object",
				Properties: map[string]ParameterSchema{
					"api_key": {Type: "string", Sensitive: true},
					"url":     {Type: "string"},
				},
			},
		},
	}

	sensitive := bp.SensitiveParameters()
	if len(sensitive) != 2 {
		t.Errorf("SensitiveParameters count = %d, want 2", len(sensitive))
	}

	// Check that password and config.api_key are marked sensitive
	found := make(map[string]bool)
	for _, s := range sensitive {
		found[s] = true
	}
	if !found["password"] {
		t.Error("password not in sensitive parameters")
	}
	if !found["config.api_key"] {
		t.Error("config.api_key not in sensitive parameters")
	}
}

func TestBlueprint_Marshal(t *testing.T) {
	bp := &Blueprint{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata: Metadata{
			Name:    "test-blueprint",
			Version: "1.0.0",
		},
		Parameters: map[string]ParameterSchema{
			"name": {Type: "string", Required: true},
		},
	}

	data, err := bp.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Parse it back
	parsed, err := ParseManifest(data)
	if err != nil {
		t.Fatalf("ParseManifest of marshaled data failed: %v", err)
	}

	if parsed.Metadata.Name != bp.Metadata.Name {
		t.Errorf("Round-trip Metadata.Name = %s, want %s", parsed.Metadata.Name, bp.Metadata.Name)
	}
}
