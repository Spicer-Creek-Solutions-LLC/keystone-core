package statemgmt

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// SELinuxModule manages SELinux state and booleans
type SELinuxModule struct {
	*BaseModule
}

// NewSELinuxModule creates a new SELinux module
func NewSELinuxModule() *SELinuxModule {
	return &SELinuxModule{
		BaseModule: NewBaseModule("selinux", []string{"enforcing", "permissive", "disabled"}),
	}
}

// Check checks the current SELinux state
func (m *SELinuxModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("SELinux is only available on Linux")
	}

	// Check if SELinux is available
	if _, err := exec.LookPath("getenforce"); err != nil {
		return nil, fmt.Errorf("SELinux tools not found")
	}

	currentMode, err := m.getCurrentMode(ctx)
	if err != nil {
		return nil, err
	}

	result := &ModuleCheckResult{
		Present:      currentMode != "disabled",
		CurrentState: currentMode,
		Matches:      currentMode == decl.State,
		Diff:         make(map[string]interface{}),
		Metadata: map[string]interface{}{
			"current_mode": currentMode,
		},
	}

	if !result.Matches {
		result.Diff["mode"] = map[string]interface{}{
			"old": currentMode,
			"new": decl.State,
		}
	}

	return result, nil
}

// Apply sets the SELinux mode
func (m *SELinuxModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: false,
			Changed: false,
			Comment: fmt.Sprintf("Check failed: %v", err),
		}, nil
	}

	if checkResult.Matches {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: fmt.Sprintf("SELinux already in %s mode", decl.State),
		}, nil
	}

	currentMode := checkResult.CurrentState

	// Handle runtime mode change (enforcing <-> permissive)
	if decl.State != "disabled" && currentMode != "disabled" {
		var mode int
		if decl.State == "enforcing" {
			mode = 1
		} else {
			mode = 0
		}

		cmd := exec.CommandContext(ctx, "setenforce", fmt.Sprintf("%d", mode))
		if err := cmd.Run(); err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Changed: false,
				Comment: fmt.Sprintf("Failed to set SELinux mode: %v", err),
			}, nil
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("SELinux mode changed to %s", decl.State),
		}, nil
	}

	// For disabled state or enabling from disabled, need to modify /etc/selinux/config
	persistent := getBoolParameter(decl, "persistent", true)
	if persistent {
		if err := m.setPersistentMode(decl.State); err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Changed: false,
				Comment: fmt.Sprintf("Failed to set persistent mode: %v", err),
			}, nil
		}
	}

	// Note: Disabling/enabling SELinux requires a reboot
	comment := fmt.Sprintf("SELinux set to %s (reboot required)", decl.State)
	if decl.State != "disabled" && currentMode != "disabled" {
		comment = fmt.Sprintf("SELinux mode changed to %s", decl.State)
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: true,
		Comment: comment,
	}, nil
}

// Test verifies the SELinux mode
func (m *SELinuxModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

func (m *SELinuxModule) getCurrentMode(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "getenforce")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("getenforce failed: %w", err)
	}

	mode := strings.ToLower(strings.TrimSpace(string(output)))
	return mode, nil
}

func (m *SELinuxModule) setPersistentMode(mode string) error {
	configPath := "/etc/selinux/config"

	content, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", configPath, err)
	}

	// Replace SELINUX= line
	pattern := regexp.MustCompile(`(?m)^SELINUX=.*$`)
	newContent := pattern.ReplaceAllString(string(content), fmt.Sprintf("SELINUX=%s", mode))

	//nolint:gosec // G306: SELinux config needs to be readable by the system
	if err := os.WriteFile(configPath, []byte(newContent), 0o644); err != nil {
		return fmt.Errorf("failed to write %s: %w", configPath, err)
	}

	return nil
}

// SELinuxBooleanModule manages SELinux booleans
type SELinuxBooleanModule struct {
	*BaseModule
}

// NewSELinuxBooleanModule creates a new SELinux boolean module
func NewSELinuxBooleanModule() *SELinuxBooleanModule {
	return &SELinuxBooleanModule{
		BaseModule: NewBaseModule("selinux_boolean", []string{"on", "off"}),
	}
}

// Check checks the current SELinux boolean value
func (m *SELinuxBooleanModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("SELinux is only available on Linux")
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	currentValue, err := m.getBooleanValue(ctx, name)
	if err != nil {
		return nil, err
	}

	currentState := "off"
	if currentValue {
		currentState = "on"
	}

	result := &ModuleCheckResult{
		Present:      true,
		CurrentState: currentState,
		Matches:      currentState == decl.State,
		Diff:         make(map[string]interface{}),
		Metadata: map[string]interface{}{
			"name":  name,
			"value": currentValue,
		},
	}

	if !result.Matches {
		result.Diff[name] = map[string]interface{}{
			"old": currentState,
			"new": decl.State,
		}
	}

	return result, nil
}

// Apply sets the SELinux boolean
func (m *SELinuxBooleanModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: false,
			Changed: false,
			Comment: fmt.Sprintf("Check failed: %v", err),
		}, nil
	}

	if checkResult.Matches {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: fmt.Sprintf("SELinux boolean already %s", decl.State),
		}, nil
	}

	name := getStringParameter(decl, "name", "")
	persistent := getBoolParameter(decl, "persistent", true)

	value := "0"
	if decl.State == "on" {
		value = "1"
	}

	args := []string{name, value}
	if persistent {
		args = append([]string{"-P"}, args...)
	}

	cmd := exec.CommandContext(ctx, "setsebool", args...)
	if err := cmd.Run(); err != nil {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: false,
			Changed: false,
			Comment: fmt.Sprintf("Failed to set boolean: %v", err),
		}, nil
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: true,
		Comment: fmt.Sprintf("SELinux boolean %s set to %s", name, decl.State),
	}, nil
}

// Test verifies the SELinux boolean
func (m *SELinuxBooleanModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

func (m *SELinuxBooleanModule) getBooleanValue(ctx context.Context, name string) (bool, error) {
	cmd := exec.CommandContext(ctx, "getsebool", name)
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("getsebool failed: %w", err)
	}

	// Output format: "boolean_name --> on" or "boolean_name --> off"
	parts := strings.Split(string(output), "-->")
	if len(parts) < 2 {
		return false, fmt.Errorf("unexpected getsebool output: %s", output)
	}

	value := strings.TrimSpace(parts[1])
	return value == "on", nil
}

// AppArmorModule manages AppArmor profile state
type AppArmorModule struct {
	*BaseModule
}

// NewAppArmorModule creates a new AppArmor module
func NewAppArmorModule() *AppArmorModule {
	return &AppArmorModule{
		BaseModule: NewBaseModule("apparmor", []string{"enforce", "complain", "disabled"}),
	}
}

// Check checks the current AppArmor profile state
func (m *AppArmorModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("AppArmor is only available on Linux")
	}

	// Check if AppArmor is available
	if _, err := os.Stat("/sys/kernel/security/apparmor"); os.IsNotExist(err) {
		return nil, fmt.Errorf("AppArmor is not enabled on this system")
	}

	profile := getStringParameter(decl, "profile", "")
	if profile == "" {
		return nil, fmt.Errorf("profile parameter is required")
	}

	currentMode, exists, err := m.getProfileMode(profile)
	if err != nil {
		return nil, err
	}

	result := &ModuleCheckResult{
		Present:      exists,
		CurrentState: currentMode,
		Matches:      false,
		Diff:         make(map[string]interface{}),
		Metadata: map[string]interface{}{
			"profile":      profile,
			"current_mode": currentMode,
		},
	}

	switch decl.State {
	case "enforce":
		result.Matches = exists && currentMode == "enforce"
	case "complain":
		result.Matches = exists && currentMode == "complain"
	case "disabled":
		result.Matches = !exists || currentMode == "disabled"
	}

	if !result.Matches {
		result.Diff["mode"] = map[string]interface{}{
			"old": currentMode,
			"new": decl.State,
		}
	}

	return result, nil
}

// Apply sets the AppArmor profile mode
func (m *AppArmorModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: false,
			Changed: false,
			Comment: fmt.Sprintf("Check failed: %v", err),
		}, nil
	}

	if checkResult.Matches {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: fmt.Sprintf("AppArmor profile already in %s mode", decl.State),
		}, nil
	}

	profile := getStringParameter(decl, "profile", "")

	switch decl.State {
	case "enforce":
		if err := m.setProfileMode(ctx, profile, "enforce"); err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Changed: false,
				Comment: fmt.Sprintf("Failed to set enforce mode: %v", err),
			}, nil
		}
	case "complain":
		if err := m.setProfileMode(ctx, profile, "complain"); err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Changed: false,
				Comment: fmt.Sprintf("Failed to set complain mode: %v", err),
			}, nil
		}
	case "disabled":
		if err := m.disableProfile(ctx, profile); err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Changed: false,
				Comment: fmt.Sprintf("Failed to disable profile: %v", err),
			}, nil
		}
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: true,
		Comment: fmt.Sprintf("AppArmor profile %s set to %s", profile, decl.State),
	}, nil
}

// Test verifies the AppArmor profile mode
func (m *AppArmorModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

func (m *AppArmorModule) getProfileMode(profile string) (mode string, exists bool, err error) {
	// Read /sys/kernel/security/apparmor/profiles
	file, err := os.Open("/sys/kernel/security/apparmor/profiles")
	if err != nil {
		return "", false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// Format: "profile_name (mode)"
		if strings.HasPrefix(line, profile+" ") {
			if strings.Contains(line, "(enforce)") {
				return "enforce", true, nil
			} else if strings.Contains(line, "(complain)") {
				return "complain", true, nil
			}
			return "unknown", true, nil
		}
	}

	return "disabled", false, scanner.Err()
}

func (m *AppArmorModule) setProfileMode(ctx context.Context, profile, mode string) error {
	var cmd *exec.Cmd
	if mode == "enforce" {
		cmd = exec.CommandContext(ctx, "aa-enforce", profile)
	} else {
		cmd = exec.CommandContext(ctx, "aa-complain", profile)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set profile mode: %w", err)
	}

	return nil
}

func (m *AppArmorModule) disableProfile(ctx context.Context, profile string) error {
	// First try aa-disable
	cmd := exec.CommandContext(ctx, "aa-disable", profile)
	if err := cmd.Run(); err != nil {
		// Try creating a symlink in /etc/apparmor.d/disable
		disableDir := "/etc/apparmor.d/disable"
		//nolint:gosec // G301: apparmor disable directory needs system access
		if err := os.MkdirAll(disableDir, 0o755); err != nil {
			return fmt.Errorf("failed to create disable directory: %w", err)
		}

		profilePath := filepath.Join("/etc", "apparmor.d", profile)
		disablePath := filepath.Join(disableDir, profile)

		// Create symlink
		if err := os.Symlink(profilePath, disablePath); err != nil && !os.IsExist(err) {
			return fmt.Errorf("failed to create disable symlink: %w", err)
		}

		// Reload the profile
		reloadCmd := exec.CommandContext(ctx, "apparmor_parser", "-R", profilePath)
		reloadCmd.Run() // Ignore error if profile wasn't loaded
	}

	return nil
}

// AppArmorProfileModule manages AppArmor profile installation
type AppArmorProfileModule struct {
	*BaseModule
}

// NewAppArmorProfileModule creates a new AppArmor profile module
func NewAppArmorProfileModule() *AppArmorProfileModule {
	return &AppArmorProfileModule{
		BaseModule: NewBaseModule("apparmor_profile", []string{"present", "absent"}),
	}
}

// Check checks if the AppArmor profile exists
func (m *AppArmorProfileModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("AppArmor is only available on Linux")
	}

	name := getStringParameter(decl, "name", "")
	source := getStringParameter(decl, "source", "")
	content := getStringParameter(decl, "content", "")

	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	profilePath := filepath.Join("/etc", "apparmor.d", name)
	exists := false
	var currentContent []byte

	if _, err := os.Stat(profilePath); err == nil {
		exists = true
		currentContent, _ = os.ReadFile(profilePath)
	}

	result := &ModuleCheckResult{
		Present:      exists,
		CurrentState: "absent",
		Matches:      false,
		Diff:         make(map[string]interface{}),
		Metadata: map[string]interface{}{
			"name": name,
			"path": profilePath,
		},
	}

	if exists {
		result.CurrentState = "present"
	}

	switch decl.State {
	case "present":
		if source != "" || content != "" {
			var desiredContent []byte
			if source != "" {
				var err error
				desiredContent, err = os.ReadFile(source)
				if err != nil {
					return nil, fmt.Errorf("failed to read source: %w", err)
				}
			} else {
				desiredContent = []byte(content)
			}
			result.Matches = exists && bytes.Equal(currentContent, desiredContent)
			if !result.Matches {
				result.Diff["content"] = "differs"
			}
		} else {
			result.Matches = exists
		}
	case "absent":
		result.Matches = !exists
		if exists {
			result.Diff["profile"] = map[string]interface{}{
				"old": "present",
				"new": "absent",
			}
		}
	}

	return result, nil
}

// Apply installs or removes the AppArmor profile
func (m *AppArmorProfileModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: false,
			Changed: false,
			Comment: fmt.Sprintf("Check failed: %v", err),
		}, nil
	}

	if checkResult.Matches {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: fmt.Sprintf("AppArmor profile already %s", decl.State),
		}, nil
	}

	name := getStringParameter(decl, "name", "")
	source := getStringParameter(decl, "source", "")
	content := getStringParameter(decl, "content", "")
	mode := getStringParameter(decl, "mode", "enforce")
	profilePath := checkResult.Metadata["path"].(string)

	switch decl.State {
	case "present":
		var profileContent []byte
		switch {
		case source != "":
			var err error
			profileContent, err = os.ReadFile(source)
			if err != nil {
				return &StateResult{
					StateID: decl.ID,
					Module:  m.Name(),
					Success: false,
					Changed: false,
					Comment: fmt.Sprintf("Failed to read source: %v", err),
				}, nil
			}
		case content != "":
			profileContent = []byte(content)
		default:
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Changed: false,
				Comment: "Either source or content parameter is required",
			}, nil
		}

		//nolint:gosec // G306: AppArmor profiles need to be readable by the kernel
		if err := os.WriteFile(profilePath, profileContent, 0o644); err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Changed: false,
				Comment: fmt.Sprintf("Failed to write profile: %v", err),
			}, nil
		}

		// Load the profile
		loadArgs := []string{"-r", profilePath}
		if mode == "complain" {
			loadArgs = []string{"-r", "-C", profilePath}
		}
		cmd := exec.CommandContext(ctx, "apparmor_parser", loadArgs...)
		if err := cmd.Run(); err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Changed: true,
				Comment: fmt.Sprintf("Profile written but failed to load: %v", err),
			}, nil
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("AppArmor profile %s installed and loaded in %s mode", name, mode),
		}, nil

	case "absent":
		// Unload the profile first
		cmd := exec.CommandContext(ctx, "apparmor_parser", "-R", profilePath)
		cmd.Run() // Ignore error

		if err := os.Remove(profilePath); err != nil && !os.IsNotExist(err) {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Changed: false,
				Comment: fmt.Sprintf("Failed to remove profile: %v", err),
			}, nil
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("AppArmor profile %s removed", name),
		}, nil
	}

	return nil, fmt.Errorf("unknown state: %s", decl.State)
}

// Test verifies the profile state
func (m *AppArmorProfileModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

func init() {
	_ = RegisterModule(NewSELinuxModule())
	_ = RegisterModule(NewSELinuxBooleanModule())
	_ = RegisterModule(NewAppArmorModule())
	_ = RegisterModule(NewAppArmorProfileModule())
}
