package handlers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// ScriptLanguage represents a supported script language.
type ScriptLanguage string

const (
	// ScriptLanguageBash is the bash shell scripting language.
	ScriptLanguageBash ScriptLanguage = "bash"

	// ScriptLanguagePython is the Python scripting language.
	ScriptLanguagePython ScriptLanguage = "python"

	// ScriptLanguagePowerShell is the PowerShell scripting language (Windows).
	ScriptLanguagePowerShell ScriptLanguage = "powershell"

	// ScriptLanguageShell is the default shell (sh on Unix, cmd on Windows).
	ScriptLanguageShell ScriptLanguage = "shell"
)

// IsValid returns true if the script language is valid.
func (l ScriptLanguage) IsValid() bool {
	switch l {
	case ScriptLanguageBash, ScriptLanguagePython, ScriptLanguagePowerShell, ScriptLanguageShell, "":
		return true
	default:
		return false
	}
}

// ScriptHandler handles script step execution.
// Configuration:
//   - script: Inline script content (required)
//   - language: Script language (bash, python, powershell, shell) (default: shell)
//   - args: Arguments to pass to the script (optional)
//   - env: Environment variables (optional)
//   - workdir: Working directory (optional)
//   - timeout: Execution timeout (optional)
type ScriptHandler struct{}

// NewScriptHandler creates a new script step handler.
func NewScriptHandler() *ScriptHandler {
	return &ScriptHandler{}
}

// Type returns the step type.
func (h *ScriptHandler) Type() runbook.StepType {
	return runbook.StepTypeScript
}

// Validate validates the step configuration.
func (h *ScriptHandler) Validate(step *runbook.Step) error {
	config := step.Config

	// Script is required
	script, ok := config["script"]
	if !ok {
		return fmt.Errorf("script step requires 'script' configuration")
	}

	if _, ok := script.(string); !ok {
		return fmt.Errorf("script must be a string")
	}

	// Validate language if provided
	if lang, ok := config["language"].(string); ok {
		if !ScriptLanguage(lang).IsValid() {
			return fmt.Errorf("invalid script language %q (supported: bash, python, powershell, shell)", lang)
		}
	}

	// Validate args if provided
	if args, ok := config["args"]; ok {
		switch args.(type) {
		case []interface{}, []string:
			// Valid
		default:
			return fmt.Errorf("args must be a list of strings")
		}
	}

	// Validate env if provided
	if env, ok := config["env"]; ok {
		if _, ok := env.(map[string]interface{}); !ok {
			return fmt.Errorf("env must be a map of string to string")
		}
	}

	return nil
}

// Execute executes the script step.
func (h *ScriptHandler) Execute(ctx context.Context, step *runbook.Step, varCtx VariableContext) (*runbook.StepResult, error) {
	startTime := time.Now()
	config := step.Config

	// Get script content
	script, err := h.getScript(config, varCtx)
	if err != nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("failed to get script: %v", err),
			Duration: time.Since(startTime),
		}, err
	}

	// Get language
	language := h.getLanguage(config)

	// Get interpreter and extension
	interpreter, ext := h.getInterpreter(language)

	// Create temp script file
	scriptFile, err := h.createScriptFile(script, ext)
	if err != nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("failed to create script file: %v", err),
			Duration: time.Since(startTime),
		}, err
	}
	defer os.Remove(scriptFile)

	// Build command
	args := h.getArgs(config, varCtx)
	cmdArgs := append(interpreter, scriptFile)
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)

	// Set environment
	cmd.Env = os.Environ()
	env := h.getEnv(config, varCtx)
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Set working directory
	if workdir, ok := config["workdir"].(string); ok {
		resolved, err := varCtx.Resolve(workdir)
		if err == nil {
			cmd.Dir = resolved
		}
	}

	// Capture output
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute
	err = cmd.Run()

	outputs := map[string]interface{}{
		"stdout":    stdout.String(),
		"stderr":    stderr.String(),
		"exit_code": 0,
	}

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			outputs["exit_code"] = exitError.ExitCode()
		}

		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("script execution failed: %v", err),
			Duration: time.Since(startTime),
			Outputs:  outputs,
		}, err
	}

	return &runbook.StepResult{
		Success:  true,
		Message:  "script executed successfully",
		Duration: time.Since(startTime),
		Outputs:  outputs,
	}, nil
}

// getScript extracts and resolves the script content.
func (h *ScriptHandler) getScript(config map[string]interface{}, varCtx VariableContext) (string, error) {
	script, ok := config["script"].(string)
	if !ok {
		return "", fmt.Errorf("script is required")
	}

	resolved, err := varCtx.Resolve(script)
	if err != nil {
		return "", fmt.Errorf("resolve script: %w", err)
	}

	return resolved, nil
}

// getLanguage extracts the script language.
func (h *ScriptHandler) getLanguage(config map[string]interface{}) ScriptLanguage {
	if lang, ok := config["language"].(string); ok {
		return ScriptLanguage(lang)
	}
	return ScriptLanguageShell
}

// getInterpreter returns the interpreter command and file extension for the language.
func (h *ScriptHandler) getInterpreter(lang ScriptLanguage) ([]string, string) {
	switch lang {
	case ScriptLanguageBash:
		return []string{"bash"}, ".sh"
	case ScriptLanguagePython:
		// Try python3 first, fall back to python
		if _, err := exec.LookPath("python3"); err == nil {
			return []string{"python3"}, ".py"
		}
		return []string{"python"}, ".py"
	case ScriptLanguagePowerShell:
		if runtime.GOOS == "windows" {
			return []string{"powershell", "-ExecutionPolicy", "Bypass", "-File"}, ".ps1"
		}
		// pwsh is PowerShell Core on Unix
		return []string{"pwsh", "-ExecutionPolicy", "Bypass", "-File"}, ".ps1"
	default: // ScriptLanguageShell
		if runtime.GOOS == "windows" {
			return []string{"cmd", "/c"}, ".bat"
		}
		return []string{"sh"}, ".sh"
	}
}

// createScriptFile creates a temporary file with the script content.
func (h *ScriptHandler) createScriptFile(script, ext string) (string, error) {
	f, err := os.CreateTemp("", "runbook-script-*"+ext)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := f.WriteString(script); err != nil {
		os.Remove(f.Name())
		return "", err
	}

	// Make executable on Unix
	if runtime.GOOS != "windows" {
		if err := os.Chmod(f.Name(), 0700); err != nil {
			os.Remove(f.Name())
			return "", err
		}
	}

	return f.Name(), nil
}

// getArgs extracts script arguments.
func (h *ScriptHandler) getArgs(config map[string]interface{}, varCtx VariableContext) []string {
	args, ok := config["args"]
	if !ok {
		return nil
	}

	var result []string

	switch a := args.(type) {
	case []interface{}:
		for _, arg := range a {
			if argStr, ok := arg.(string); ok {
				resolved, err := varCtx.Resolve(argStr)
				if err == nil {
					result = append(result, resolved)
				} else {
					result = append(result, argStr)
				}
			}
		}
	case []string:
		for _, arg := range a {
			resolved, err := varCtx.Resolve(arg)
			if err == nil {
				result = append(result, resolved)
			} else {
				result = append(result, arg)
			}
		}
	}

	return result
}

// getEnv extracts environment variables.
func (h *ScriptHandler) getEnv(config map[string]interface{}, varCtx VariableContext) map[string]string {
	env, ok := config["env"].(map[string]interface{})
	if !ok {
		return nil
	}

	result := make(map[string]string)
	for k, v := range env {
		if vStr, ok := v.(string); ok {
			resolved, err := varCtx.Resolve(vStr)
			if err == nil {
				result[k] = resolved
			} else {
				result[k] = vStr
			}
		}
	}

	return result
}

// ScriptFileHandler handles script file step execution.
// This is similar to ScriptHandler but loads script from a file.
// Configuration:
//   - file: Path to script file (required)
//   - language: Script language (auto-detected from extension if not specified)
//   - args: Arguments to pass to the script (optional)
//   - env: Environment variables (optional)
//   - workdir: Working directory (optional)
type ScriptFileHandler struct{}

// NewScriptFileHandler creates a new script file step handler.
func NewScriptFileHandler() *ScriptFileHandler {
	return &ScriptFileHandler{}
}

// Type returns the step type.
func (h *ScriptFileHandler) Type() runbook.StepType {
	return runbook.StepType("script_file")
}

// Validate validates the step configuration.
func (h *ScriptFileHandler) Validate(step *runbook.Step) error {
	config := step.Config

	// File is required
	file, ok := config["file"]
	if !ok {
		return fmt.Errorf("script_file step requires 'file' configuration")
	}

	if _, ok := file.(string); !ok {
		return fmt.Errorf("file must be a string")
	}

	return nil
}

// Execute executes the script file step.
func (h *ScriptFileHandler) Execute(ctx context.Context, step *runbook.Step, varCtx VariableContext) (*runbook.StepResult, error) {
	startTime := time.Now()
	config := step.Config

	// Get file path
	filePath, err := h.getFilePath(config, varCtx)
	if err != nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("failed to get file path: %v", err),
			Duration: time.Since(startTime),
		}, err
	}

	// Verify file exists
	if _, err := os.Stat(filePath); err != nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("script file not found: %v", err),
			Duration: time.Since(startTime),
		}, err
	}

	// Detect language from extension if not specified
	language := h.detectLanguage(config, filePath)

	// Get interpreter
	interpreter, _ := (&ScriptHandler{}).getInterpreter(language)

	// Build command
	scriptHandler := &ScriptHandler{}
	args := scriptHandler.getArgs(config, varCtx)
	cmdArgs := append(interpreter, filePath)
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)

	// Set environment
	cmd.Env = os.Environ()
	env := scriptHandler.getEnv(config, varCtx)
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Set working directory
	if workdir, ok := config["workdir"].(string); ok {
		resolved, err := varCtx.Resolve(workdir)
		if err == nil {
			cmd.Dir = resolved
		}
	}

	// Capture output
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute
	err = cmd.Run()

	outputs := map[string]interface{}{
		"stdout":    stdout.String(),
		"stderr":    stderr.String(),
		"exit_code": 0,
		"file":      filePath,
	}

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			outputs["exit_code"] = exitError.ExitCode()
		}

		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("script execution failed: %v", err),
			Duration: time.Since(startTime),
			Outputs:  outputs,
		}, err
	}

	return &runbook.StepResult{
		Success:  true,
		Message:  "script executed successfully",
		Duration: time.Since(startTime),
		Outputs:  outputs,
	}, nil
}

// getFilePath extracts and resolves the file path.
func (h *ScriptFileHandler) getFilePath(config map[string]interface{}, varCtx VariableContext) (string, error) {
	file, ok := config["file"].(string)
	if !ok {
		return "", fmt.Errorf("file is required")
	}

	resolved, err := varCtx.Resolve(file)
	if err != nil {
		return "", fmt.Errorf("resolve file path: %w", err)
	}

	return resolved, nil
}

// detectLanguage detects the script language from file extension or config.
func (h *ScriptFileHandler) detectLanguage(config map[string]interface{}, filePath string) ScriptLanguage {
	// Use configured language if provided
	if lang, ok := config["language"].(string); ok {
		return ScriptLanguage(lang)
	}

	// Detect from file extension
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".sh":
		return ScriptLanguageBash
	case ".py":
		return ScriptLanguagePython
	case ".ps1":
		return ScriptLanguagePowerShell
	case ".bat", ".cmd":
		return ScriptLanguageShell
	default:
		return ScriptLanguageShell
	}
}
