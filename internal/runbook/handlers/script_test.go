package handlers

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

func TestScriptLanguage_IsValid(t *testing.T) {
	tests := []struct {
		name string
		lang ScriptLanguage
		want bool
	}{
		{"bash", ScriptLanguageBash, true},
		{"python", ScriptLanguagePython, true},
		{"powershell", ScriptLanguagePowerShell, true},
		{"shell", ScriptLanguageShell, true},
		{"empty", ScriptLanguage(""), true},
		{"invalid", ScriptLanguage("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.lang.IsValid(); got != tt.want {
				t.Errorf("ScriptLanguage.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScriptHandler_Type(t *testing.T) {
	h := NewScriptHandler()
	if got := h.Type(); got != runbook.StepTypeScript {
		t.Errorf("Type() = %v, want %v", got, runbook.StepTypeScript)
	}
}

func TestScriptHandler_Validate(t *testing.T) {
	h := NewScriptHandler()

	tests := []struct {
		name    string
		config  map[string]interface{}
		wantErr bool
	}{
		{
			name: "valid minimal config",
			config: map[string]interface{}{
				"script": "echo hello",
			},
			wantErr: false,
		},
		{
			name: "valid full config",
			config: map[string]interface{}{
				"script":   "echo $VAR",
				"language": "bash",
				"args":     []interface{}{"arg1", "arg2"},
				"env": map[string]interface{}{
					"VAR": "value",
				},
				"workdir": "/tmp",
			},
			wantErr: false,
		},
		{
			name:    "missing script",
			config:  map[string]interface{}{},
			wantErr: true,
		},
		{
			name: "script not string",
			config: map[string]interface{}{
				"script": 123,
			},
			wantErr: true,
		},
		{
			name: "invalid language",
			config: map[string]interface{}{
				"script":   "echo hello",
				"language": "invalid",
			},
			wantErr: true,
		},
		{
			name: "args not list",
			config: map[string]interface{}{
				"script": "echo hello",
				"args":   "not-a-list",
			},
			wantErr: true,
		},
		{
			name: "env not map",
			config: map[string]interface{}{
				"script": "echo hello",
				"env":    "not-a-map",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &runbook.Step{
				Name:   "test",
				Type:   runbook.StepTypeScript,
				Config: tt.config,
			}

			err := h.Validate(step)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestScriptHandler_Execute(t *testing.T) {
	// Skip on Windows if bash isn't available
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Unix shell tests on Windows")
	}

	t.Run("simple echo", func(t *testing.T) {
		h := NewScriptHandler()
		varCtx := newMockVariableContext()

		step := &runbook.Step{
			Name: "test-script",
			Type: runbook.StepTypeScript,
			Config: map[string]interface{}{
				"script":   "echo hello",
				"language": "shell",
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if !result.Success {
			t.Errorf("Expected success, got failure: %s", result.Message)
		}

		stdout := result.Outputs["stdout"].(string)
		if !strings.Contains(stdout, "hello") {
			t.Errorf("Expected stdout to contain 'hello', got %q", stdout)
		}
	})

	t.Run("with environment variables", func(t *testing.T) {
		h := NewScriptHandler()
		varCtx := newMockVariableContext()

		step := &runbook.Step{
			Name: "test-env",
			Type: runbook.StepTypeScript,
			Config: map[string]interface{}{
				"script":   "echo $MY_VAR",
				"language": "bash",
				"env": map[string]interface{}{
					"MY_VAR": "test-value",
				},
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if !result.Success {
			t.Errorf("Expected success, got failure: %s", result.Message)
		}

		stdout := result.Outputs["stdout"].(string)
		if !strings.Contains(stdout, "test-value") {
			t.Errorf("Expected stdout to contain 'test-value', got %q", stdout)
		}
	})

	t.Run("with arguments", func(t *testing.T) {
		h := NewScriptHandler()
		varCtx := newMockVariableContext()

		step := &runbook.Step{
			Name: "test-args",
			Type: runbook.StepTypeScript,
			Config: map[string]interface{}{
				"script":   "echo $1 $2",
				"language": "bash",
				"args":     []interface{}{"arg1", "arg2"},
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if !result.Success {
			t.Errorf("Expected success, got failure: %s", result.Message)
		}

		stdout := result.Outputs["stdout"].(string)
		if !strings.Contains(stdout, "arg1") || !strings.Contains(stdout, "arg2") {
			t.Errorf("Expected stdout to contain args, got %q", stdout)
		}
	})

	t.Run("script failure", func(t *testing.T) {
		h := NewScriptHandler()
		varCtx := newMockVariableContext()

		step := &runbook.Step{
			Name: "test-fail",
			Type: runbook.StepTypeScript,
			Config: map[string]interface{}{
				"script":   "exit 1",
				"language": "shell",
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if result.Success {
			t.Error("Expected failure, got success")
		}

		exitCode := result.Outputs["exit_code"].(int)
		if exitCode != 1 {
			t.Errorf("Expected exit code 1, got %d", exitCode)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		h := NewScriptHandler()
		varCtx := newMockVariableContext()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		step := &runbook.Step{
			Name: "test-cancel",
			Type: runbook.StepTypeScript,
			Config: map[string]interface{}{
				"script":   "sleep 10",
				"language": "shell",
			},
		}

		_, err := h.Execute(ctx, step, varCtx)
		if err == nil {
			t.Fatal("Expected error for cancelled context")
		}
	})
}

func TestScriptHandler_getInterpreter(t *testing.T) {
	h := &ScriptHandler{}

	tests := []struct {
		name     string
		lang     ScriptLanguage
		wantExt  string
		skipOnOS string
	}{
		{
			name:    "bash",
			lang:    ScriptLanguageBash,
			wantExt: ".sh",
		},
		{
			name:    "python",
			lang:    ScriptLanguagePython,
			wantExt: ".py",
		},
		{
			name:     "powershell on windows",
			lang:     ScriptLanguagePowerShell,
			wantExt:  ".ps1",
			skipOnOS: "linux",
		},
		{
			name:    "shell",
			lang:    ScriptLanguageShell,
			wantExt: ".sh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipOnOS != "" && runtime.GOOS == tt.skipOnOS {
				t.Skip("Skipping test on", runtime.GOOS)
			}

			_, ext := h.getInterpreter(tt.lang)
			if runtime.GOOS == "windows" && tt.lang == ScriptLanguageShell {
				if ext != ".bat" {
					t.Errorf("Expected .bat on Windows, got %s", ext)
				}
			} else if ext != tt.wantExt {
				t.Errorf("getInterpreter() ext = %s, want %s", ext, tt.wantExt)
			}
		})
	}
}

func TestScriptFileHandler_Type(t *testing.T) {
	h := NewScriptFileHandler()
	if got := h.Type(); got != runbook.StepType("script_file") {
		t.Errorf("Type() = %v, want script_file", got)
	}
}

func TestScriptFileHandler_Validate(t *testing.T) {
	h := NewScriptFileHandler()

	tests := []struct {
		name    string
		config  map[string]interface{}
		wantErr bool
	}{
		{
			name: "valid config",
			config: map[string]interface{}{
				"file": "/path/to/script.sh",
			},
			wantErr: false,
		},
		{
			name:    "missing file",
			config:  map[string]interface{}{},
			wantErr: true,
		},
		{
			name: "file not string",
			config: map[string]interface{}{
				"file": 123,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &runbook.Step{
				Name:   "test",
				Type:   runbook.StepType("script_file"),
				Config: tt.config,
			}

			err := h.Validate(step)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestScriptFileHandler_Execute(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Unix shell tests on Windows")
	}

	t.Run("execute script file", func(t *testing.T) {
		// Create a temp script file
		f, err := os.CreateTemp("", "test-script-*.sh")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(f.Name())

		if _, err := f.WriteString("#!/bin/sh\necho \"script executed\""); err != nil {
			t.Fatalf("Failed to write script: %v", err)
		}
		f.Close()
		os.Chmod(f.Name(), 0755)

		h := NewScriptFileHandler()
		varCtx := newMockVariableContext()

		step := &runbook.Step{
			Name: "test-file",
			Type: runbook.StepType("script_file"),
			Config: map[string]interface{}{
				"file": f.Name(),
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if !result.Success {
			t.Errorf("Expected success, got failure: %s", result.Message)
		}

		stdout := result.Outputs["stdout"].(string)
		if !strings.Contains(stdout, "script executed") {
			t.Errorf("Expected stdout to contain 'script executed', got %q", stdout)
		}
	})

	t.Run("file not found", func(t *testing.T) {
		h := NewScriptFileHandler()
		varCtx := newMockVariableContext()

		step := &runbook.Step{
			Name: "test-not-found",
			Type: runbook.StepType("script_file"),
			Config: map[string]interface{}{
				"file": "/nonexistent/script.sh",
			},
		}

		result, err := h.Execute(context.Background(), step, varCtx)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if result.Success {
			t.Error("Expected failure, got success")
		}
	})
}

func TestScriptFileHandler_detectLanguage(t *testing.T) {
	h := &ScriptFileHandler{}

	tests := []struct {
		name     string
		config   map[string]interface{}
		filePath string
		want     ScriptLanguage
	}{
		{
			name:     "detect from .sh extension",
			config:   map[string]interface{}{},
			filePath: "/path/to/script.sh",
			want:     ScriptLanguageBash,
		},
		{
			name:     "detect from .py extension",
			config:   map[string]interface{}{},
			filePath: "/path/to/script.py",
			want:     ScriptLanguagePython,
		},
		{
			name:     "detect from .ps1 extension",
			config:   map[string]interface{}{},
			filePath: "/path/to/script.ps1",
			want:     ScriptLanguagePowerShell,
		},
		{
			name:     "detect from .bat extension",
			config:   map[string]interface{}{},
			filePath: "/path/to/script.bat",
			want:     ScriptLanguageShell,
		},
		{
			name: "use configured language",
			config: map[string]interface{}{
				"language": "python",
			},
			filePath: "/path/to/script.sh",
			want:     ScriptLanguagePython,
		},
		{
			name:     "unknown extension",
			config:   map[string]interface{}{},
			filePath: "/path/to/script.unknown",
			want:     ScriptLanguageShell,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := h.detectLanguage(tt.config, tt.filePath)
			if got != tt.want {
				t.Errorf("detectLanguage() = %v, want %v", got, tt.want)
			}
		})
	}
}
