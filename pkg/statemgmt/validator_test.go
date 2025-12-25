package statemgmt

import (
	"testing"
)

func TestValidator_ValidState(t *testing.T) {
	validator := NewValidator()

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     "/etc/nginx/nginx.conf",
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"source": "file://nginx.conf",
						"mode":   "0644",
						"user":   "root",
						"group":  "root",
					},
				},
			},
			"package": {
				{
					ID:     "nginx",
					Module: "package",
					State:  "installed",
					Parameters: map[string]interface{}{
						"version": ">=1.20",
					},
				},
			},
			"service": {
				{
					ID:     "nginx",
					Module: "service",
					State:  "running",
					Parameters: map[string]interface{}{
						"enable": true,
					},
					Requisites: Requisites{
						Require: []StateReference{
							{Module: "package", ID: "nginx"},
							{Module: "file", ID: "/etc/nginx/nginx.conf"},
						},
					},
				},
			},
		},
	}

	errors := validator.Validate(stateFile)
	if len(errors) > 0 {
		t.Errorf("Expected no validation errors, got %d errors:", len(errors))
		for _, err := range errors {
			t.Errorf("  - %s", err.Message)
		}
	}
}

func TestValidator_InvalidModuleType(t *testing.T) {
	validator := NewValidator()

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"invalid_module": {
				{
					ID:     "test",
					Module: "invalid_module",
					State:  "present",
				},
			},
		},
	}

	errors := validator.Validate(stateFile)
	if len(errors) == 0 {
		t.Fatal("Expected validation error for invalid module type, got none")
	}

	if errors[0].Module != "invalid_module" {
		t.Errorf("Expected error for module 'invalid_module', got '%s'", errors[0].Module)
	}
}

func TestValidator_InvalidState(t *testing.T) {
	validator := NewValidator()

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     "/etc/test",
					Module: "file",
					State:  "invalid_state",
				},
			},
		},
	}

	errors := validator.Validate(stateFile)
	if len(errors) == 0 {
		t.Fatal("Expected validation error for invalid state, got none")
	}

	if errors[0].Field != "state" {
		t.Errorf("Expected error for field 'state', got '%s'", errors[0].Field)
	}
}

func TestValidator_UnknownParameter(t *testing.T) {
	validator := NewValidator()

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     "/etc/test",
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"unknown_param": "value",
					},
				},
			},
		},
	}

	errors := validator.Validate(stateFile)
	if len(errors) == 0 {
		t.Fatal("Expected validation error for unknown parameter, got none")
	}

	if errors[0].Field != "unknown_param" {
		t.Errorf("Expected error for field 'unknown_param', got '%s'", errors[0].Field)
	}
}

func TestValidator_InvalidFileMode(t *testing.T) {
	validator := NewValidator()

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     "/etc/test",
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"mode": "invalid",
					},
				},
			},
		},
	}

	errors := validator.Validate(stateFile)
	if len(errors) == 0 {
		t.Fatal("Expected validation error for invalid file mode, got none")
	}

	if errors[0].Field != "mode" {
		t.Errorf("Expected error for field 'mode', got '%s'", errors[0].Field)
	}
}

func TestValidator_ValidFileModes(t *testing.T) {
	validator := NewValidator()

	validModes := []string{"0644", "0755", "0600", "644", "755"}

	for _, mode := range validModes {
		stateFile := &StateFile{
			Path: "test.yaml",
			States: map[string][]StateDeclaration{
				"file": {
					{
						ID:     "/etc/test",
						Module: "file",
						State:  "present",
						Parameters: map[string]interface{}{
							"mode": mode,
						},
					},
				},
			},
		}

		errors := validator.Validate(stateFile)
		if len(errors) > 0 {
			t.Errorf("Mode '%s' should be valid, got errors: %v", mode, errors)
		}
	}
}

func TestValidator_NonExistentRequisite(t *testing.T) {
	validator := NewValidator()

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"service": {
				{
					ID:     "nginx",
					Module: "service",
					State:  "running",
					Requisites: Requisites{
						Require: []StateReference{
							{Module: "package", ID: "nginx"}, // This doesn't exist
						},
					},
				},
			},
		},
	}

	errors := validator.Validate(stateFile)
	if len(errors) == 0 {
		t.Fatal("Expected validation error for non-existent requisite, got none")
	}

	found := false
	for _, err := range errors {
		if err.StateID == "nginx" && err.Module == "service" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected error for service.nginx referencing non-existent package.nginx")
	}
}

func TestValidator_AllRequisiteTypes(t *testing.T) {
	validator := NewValidator()

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"package": {
				{
					ID:     "nginx",
					Module: "package",
					State:  "installed",
				},
			},
			"file": {
				{
					ID:     "/etc/nginx/nginx.conf",
					Module: "file",
					State:  "present",
					Requisites: Requisites{
						Require: []StateReference{
							{Module: "package", ID: "nginx"},
						},
					},
				},
			},
			"service": {
				{
					ID:     "nginx",
					Module: "service",
					State:  "running",
					Requisites: Requisites{
						Require: []StateReference{
							{Module: "file", ID: "/etc/nginx/nginx.conf"},
						},
						Watch: []StateReference{
							{Module: "file", ID: "/etc/nginx/nginx.conf"},
						},
						Prereq: []StateReference{
							{Module: "package", ID: "nginx"},
						},
						Onchanges: []StateReference{
							{Module: "file", ID: "/etc/nginx/nginx.conf"},
						},
					},
				},
			},
		},
	}

	errors := validator.Validate(stateFile)
	if len(errors) > 0 {
		t.Errorf("Expected no validation errors with all requisite types, got %d errors:", len(errors))
		for _, err := range errors {
			t.Errorf("  - %s", err.Message)
		}
	}
}

func TestValidator_ValidateStateID(t *testing.T) {
	tests := []struct {
		id      string
		wantErr bool
	}{
		{"valid-id", false},
		{"/etc/nginx/nginx.conf", false},
		{"nginx", false},
		{"", true}, // Empty ID
		{"test\nid", true}, // Newline
		{"test\tid", true}, // Tab
		{"test\rid", true}, // Carriage return
	}

	for _, tt := range tests {
		err := ValidateStateID(tt.id)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateStateID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
		}
	}
}

func TestValidator_PackageStates(t *testing.T) {
	validator := NewValidator()

	validStates := []string{"installed", "removed", "latest", "purged"}
	invalidStates := []string{"present", "absent", "running"}

	for _, state := range validStates {
		stateFile := &StateFile{
			Path: "test.yaml",
			States: map[string][]StateDeclaration{
				"package": {
					{
						ID:     "nginx",
						Module: "package",
						State:  state,
					},
				},
			},
		}

		errors := validator.Validate(stateFile)
		if len(errors) > 0 {
			t.Errorf("Package state '%s' should be valid, got errors: %v", state, errors)
		}
	}

	for _, state := range invalidStates {
		stateFile := &StateFile{
			Path: "test.yaml",
			States: map[string][]StateDeclaration{
				"package": {
					{
						ID:     "nginx",
						Module: "package",
						State:  state,
					},
				},
			},
		}

		errors := validator.Validate(stateFile)
		if len(errors) == 0 {
			t.Errorf("Package state '%s' should be invalid, but validation passed", state)
		}
	}
}

func TestValidator_ServiceStates(t *testing.T) {
	validator := NewValidator()

	validStates := []string{"running", "stopped", "enabled", "disabled", "dead"}
	invalidStates := []string{"present", "absent", "installed"}

	for _, state := range validStates {
		stateFile := &StateFile{
			Path: "test.yaml",
			States: map[string][]StateDeclaration{
				"service": {
					{
						ID:     "nginx",
						Module: "service",
						State:  state,
					},
				},
			},
		}

		errors := validator.Validate(stateFile)
		if len(errors) > 0 {
			t.Errorf("Service state '%s' should be valid, got errors: %v", state, errors)
		}
	}

	for _, state := range invalidStates {
		stateFile := &StateFile{
			Path: "test.yaml",
			States: map[string][]StateDeclaration{
				"service": {
					{
						ID:     "nginx",
						Module: "service",
						State:  state,
					},
				},
			},
		}

		errors := validator.Validate(stateFile)
		if len(errors) == 0 {
			t.Errorf("Service state '%s' should be invalid, but validation passed", state)
		}
	}
}

func TestValidator_UserModule(t *testing.T) {
	validator := NewValidator()

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"user": {
				{
					ID:     "nginx",
					Module: "user",
					State:  "present",
					Parameters: map[string]interface{}{
						"uid":        1001,
						"gid":        1001,
						"home":       "/var/lib/nginx",
						"shell":      "/bin/false",
						"createhome": false,
						"system":     true,
					},
				},
			},
		},
	}

	errors := validator.Validate(stateFile)
	if len(errors) > 0 {
		t.Errorf("Expected no validation errors for user module, got %d errors:", len(errors))
		for _, err := range errors {
			t.Errorf("  - %s", err.Message)
		}
	}
}

func TestValidator_GroupModule(t *testing.T) {
	validator := NewValidator()

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"group": {
				{
					ID:     "nginx",
					Module: "group",
					State:  "present",
					Parameters: map[string]interface{}{
						"gid":     1001,
						"system":  true,
						"members": []string{"www-data", "nginx"},
					},
				},
			},
		},
	}

	errors := validator.Validate(stateFile)
	if len(errors) > 0 {
		t.Errorf("Expected no validation errors for group module, got %d errors:", len(errors))
		for _, err := range errors {
			t.Errorf("  - %s", err.Message)
		}
	}
}

func TestValidator_CmdModule(t *testing.T) {
	validator := NewValidator()

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"cmd": {
				{
					ID:     "reload-nginx",
					Module: "cmd",
					State:  "run",
					Parameters: map[string]interface{}{
						"creates": "/var/run/nginx.pid",
						"cwd":     "/etc/nginx",
						"env": map[string]string{
							"PATH": "/usr/local/bin:/usr/bin:/bin",
						},
						"timeout": 30,
						"runas":   "root",
						"shell":   "/bin/bash",
					},
				},
			},
		},
	}

	errors := validator.Validate(stateFile)
	if len(errors) > 0 {
		t.Errorf("Expected no validation errors for cmd module, got %d errors:", len(errors))
		for _, err := range errors {
			t.Errorf("  - %s", err.Message)
		}
	}
}

func TestValidator_ComplexStateDependencies(t *testing.T) {
	validator := NewValidator()

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"user": {
				{
					ID:     "nginx",
					Module: "user",
					State:  "present",
				},
			},
			"group": {
				{
					ID:     "nginx",
					Module: "group",
					State:  "present",
				},
			},
			"package": {
				{
					ID:     "nginx",
					Module: "package",
					State:  "installed",
					Requisites: Requisites{
						Require: []StateReference{
							{Module: "user", ID: "nginx"},
							{Module: "group", ID: "nginx"},
						},
					},
				},
			},
			"file": {
				{
					ID:     "/etc/nginx/nginx.conf",
					Module: "file",
					State:  "present",
					Requisites: Requisites{
						Require: []StateReference{
							{Module: "package", ID: "nginx"},
						},
					},
				},
				{
					ID:     "/var/log/nginx",
					Module: "file",
					State:  "directory",
					Requisites: Requisites{
						Require: []StateReference{
							{Module: "user", ID: "nginx"},
							{Module: "group", ID: "nginx"},
						},
					},
				},
			},
			"service": {
				{
					ID:     "nginx",
					Module: "service",
					State:  "running",
					Requisites: Requisites{
						Require: []StateReference{
							{Module: "file", ID: "/etc/nginx/nginx.conf"},
							{Module: "file", ID: "/var/log/nginx"},
						},
						Watch: []StateReference{
							{Module: "file", ID: "/etc/nginx/nginx.conf"},
						},
					},
				},
			},
		},
	}

	errors := validator.Validate(stateFile)
	if len(errors) > 0 {
		t.Errorf("Expected no validation errors for complex dependencies, got %d errors:", len(errors))
		for _, err := range errors {
			t.Errorf("  - %s", err.Message)
		}
	}
}
