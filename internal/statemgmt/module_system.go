package statemgmt

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// TimezoneModule manages system timezone
type TimezoneModule struct {
	*BaseModule
}

// NewTimezoneModule creates a new timezone module
func NewTimezoneModule() *TimezoneModule {
	return &TimezoneModule{
		BaseModule: NewBaseModule("timezone", []string{"present"}),
	}
}

// Check checks the current timezone
func (m *TimezoneModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	currentTZ, err := m.getCurrentTimezone(ctx)
	if err != nil {
		return nil, err
	}

	result := &ModuleCheckResult{
		Present:      true,
		CurrentState: currentTZ,
		Matches:      currentTZ == name,
		Diff:         make(map[string]interface{}),
		Metadata: map[string]interface{}{
			"name":     name,
			"current":  currentTZ,
			"platform": runtime.GOOS,
		},
	}

	if !result.Matches {
		result.Diff["timezone"] = map[string]interface{}{
			"old": currentTZ,
			"new": name,
		}
	}

	return result, nil
}

// Apply sets the timezone
func (m *TimezoneModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
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
			Comment: "Timezone already set correctly",
		}, nil
	}

	name := getStringParameter(decl, "name", "")

	if err := m.setTimezone(ctx, name); err != nil {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: false,
			Changed: false,
			Comment: fmt.Sprintf("Failed to set timezone: %v", err),
		}, nil
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: true,
		Comment: fmt.Sprintf("Timezone set to %s", name),
	}, nil
}

// Test verifies the timezone
func (m *TimezoneModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

func (m *TimezoneModule) getCurrentTimezone(ctx context.Context) (string, error) {
	switch runtime.GOOS {
	case "linux":
		// Try timedatectl first
		cmd := exec.CommandContext(ctx,"timedatectl", "show", "-p", "Timezone", "--value")
		output, err := cmd.Output()
		if err == nil {
			return strings.TrimSpace(string(output)), nil
		}

		// Fallback to /etc/timezone
		if data, err := os.ReadFile("/etc/timezone"); err == nil {
			return strings.TrimSpace(string(data)), nil
		}

		// Fallback to /etc/localtime symlink
		target, err := os.Readlink("/etc/localtime")
		if err == nil {
			// Extract timezone from path like /usr/share/zoneinfo/America/New_York
			if idx := strings.Index(target, "zoneinfo/"); idx != -1 {
				return target[idx+9:], nil
			}
		}

		return "", fmt.Errorf("could not determine current timezone")

	case "darwin":
		cmd := exec.CommandContext(ctx,"systemsetup", "-gettimezone")
		output, err := cmd.Output()
		if err != nil {
			return "", err
		}
		// Output: "Time Zone: America/New_York"
		parts := strings.SplitN(string(output), ":", 2)
		if len(parts) == 2 {
			return strings.TrimSpace(parts[1]), nil
		}
		return "", fmt.Errorf("could not parse timezone output")

	case "windows":
		cmd := exec.CommandContext(ctx,"tzutil", "/g")
		output, err := cmd.Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(output)), nil

	default:
		return "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func (m *TimezoneModule) setTimezone(ctx context.Context, name string) error {
	switch runtime.GOOS {
	case "linux":
		// Try timedatectl first
		cmd := exec.CommandContext(ctx, "timedatectl", "set-timezone", name)
		if err := cmd.Run(); err == nil {
			return nil
		}

		// Fallback: symlink /etc/localtime
		zonePath := filepath.Join("/usr", "share", "zoneinfo", name)
		if _, err := os.Stat(zonePath); os.IsNotExist(err) {
			return fmt.Errorf("timezone %s not found", name)
		}

		os.Remove("/etc/localtime")
		if err := os.Symlink(zonePath, "/etc/localtime"); err != nil {
			return err
		}

		// Also update /etc/timezone
		//nolint:gosec // G306: timezone file needs to be readable by the system
		return os.WriteFile("/etc/timezone", []byte(name+"\n"), 0o644)

	case "darwin":
		cmd := exec.CommandContext(ctx, "systemsetup", "-settimezone", name)
		return cmd.Run()

	case "windows":
		cmd := exec.CommandContext(ctx, "tzutil", "/s", name)
		return cmd.Run()

	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// LocaleModule manages system locale settings
type LocaleModule struct {
	*BaseModule
}

// NewLocaleModule creates a new locale module
func NewLocaleModule() *LocaleModule {
	return &LocaleModule{
		BaseModule: NewBaseModule("locale", []string{"present"}),
	}
}

// Check checks the current locale
func (m *LocaleModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("locale module not supported on Windows")
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required (e.g., en_US.UTF-8)")
	}

	currentLocale, err := m.getCurrentLocale(ctx)
	if err != nil {
		return nil, err
	}

	result := &ModuleCheckResult{
		Present:      true,
		CurrentState: currentLocale,
		Matches:      currentLocale == name,
		Diff:         make(map[string]interface{}),
		Metadata: map[string]interface{}{
			"name":    name,
			"current": currentLocale,
		},
	}

	if !result.Matches {
		result.Diff["locale"] = map[string]interface{}{
			"old": currentLocale,
			"new": name,
		}
	}

	return result, nil
}

// Apply sets the locale
func (m *LocaleModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
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
			Comment: "Locale already set correctly",
		}, nil
	}

	name := getStringParameter(decl, "name", "")

	if err := m.setLocale(ctx, name); err != nil {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: false,
			Changed: false,
			Comment: fmt.Sprintf("Failed to set locale: %v", err),
		}, nil
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: true,
		Comment: fmt.Sprintf("Locale set to %s", name),
	}, nil
}

// Test verifies the locale
func (m *LocaleModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

func (m *LocaleModule) getCurrentLocale(ctx context.Context) (string, error) {
	switch runtime.GOOS {
	case "linux":
		// Try localectl
		cmd := exec.CommandContext(ctx,"localectl", "status")
		output, err := cmd.Output()
		if err == nil {
			for _, line := range strings.Split(string(output), "\n") {
				if strings.Contains(line, "System Locale:") {
					parts := strings.SplitN(line, "=", 2)
					if len(parts) == 2 {
						return strings.TrimSpace(parts[1]), nil
					}
				}
			}
		}

		// Fallback to LANG environment variable
		if lang := os.Getenv("LANG"); lang != "" {
			return lang, nil
		}

		// Read from /etc/locale.conf or /etc/default/locale
		for _, path := range []string{"/etc/locale.conf", "/etc/default/locale"} {
			if data, err := os.ReadFile(path); err == nil {
				for _, line := range strings.Split(string(data), "\n") {
					if strings.HasPrefix(line, "LANG=") {
						return strings.TrimPrefix(line, "LANG="), nil
					}
				}
			}
		}

		return "C", nil

	case "darwin":
		// macOS uses different locale system
		cmd := exec.CommandContext(ctx,"defaults", "read", "-g", "AppleLocale")
		output, err := cmd.Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(output)), nil

	default:
		return "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func (m *LocaleModule) setLocale(ctx context.Context, name string) error {
	switch runtime.GOOS {
	case "linux":
		// Generate locale if needed
		exec.CommandContext(ctx, "locale-gen", name).Run()

		// Try localectl first
		cmd := exec.CommandContext(ctx, "localectl", "set-locale", fmt.Sprintf("LANG=%s", name))
		if err := cmd.Run(); err == nil {
			return nil
		}

		// Fallback: write to locale files
		content := fmt.Sprintf("LANG=%s\n", name)

		// Try /etc/locale.conf (systemd)
		//nolint:gosec // G306: locale config files need to be readable by the system
		if err := os.WriteFile("/etc/locale.conf", []byte(content), 0o644); err == nil {
			return nil
		}

		// Try /etc/default/locale (Debian)
		//nolint:gosec // G306: locale config files need to be readable by the system
		return os.WriteFile("/etc/default/locale", []byte(content), 0o644)

	case "darwin":
		cmd := exec.CommandContext(ctx, "defaults", "write", "-g", "AppleLocale", name)
		return cmd.Run()

	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// HostnameModule manages system hostname
type HostnameModule struct {
	*BaseModule
}

// NewHostnameModule creates a new hostname module
func NewHostnameModule() *HostnameModule {
	return &HostnameModule{
		BaseModule: NewBaseModule("hostname", []string{"present"}),
	}
}

// Check checks the current hostname
func (m *HostnameModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	currentHostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	result := &ModuleCheckResult{
		Present:      true,
		CurrentState: currentHostname,
		Matches:      currentHostname == name,
		Diff:         make(map[string]interface{}),
		Metadata: map[string]interface{}{
			"name":    name,
			"current": currentHostname,
		},
	}

	if !result.Matches {
		result.Diff["hostname"] = map[string]interface{}{
			"old": currentHostname,
			"new": name,
		}
	}

	return result, nil
}

// Apply sets the hostname
func (m *HostnameModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
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
			Comment: "Hostname already set correctly",
		}, nil
	}

	name := getStringParameter(decl, "name", "")
	persistent := getBoolParameter(decl, "persistent", true)

	if err := m.setHostname(ctx, name, persistent); err != nil {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: false,
			Changed: false,
			Comment: fmt.Sprintf("Failed to set hostname: %v", err),
		}, nil
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: true,
		Comment: fmt.Sprintf("Hostname set to %s", name),
	}, nil
}

// Test verifies the hostname
func (m *HostnameModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

func (m *HostnameModule) setHostname(ctx context.Context, name string, persistent bool) error {
	switch runtime.GOOS {
	case "linux":
		// Try hostnamectl first
		cmd := exec.CommandContext(ctx, "hostnamectl", "set-hostname", name)
		if err := cmd.Run(); err == nil {
			return nil
		}

		// Fallback: hostname command + /etc/hostname
		cmd = exec.CommandContext(ctx, "hostname", name)
		if err := cmd.Run(); err != nil {
			return err
		}

		if persistent {
			//nolint:gosec // G306: hostname file needs to be readable by the system
			return os.WriteFile("/etc/hostname", []byte(name+"\n"), 0o644)
		}
		return nil

	case "darwin":
		// macOS uses scutil
		for _, setType := range []string{"ComputerName", "HostName", "LocalHostName"} {
			cmd := exec.CommandContext(ctx, "scutil", "--set", setType, name)
			if err := cmd.Run(); err != nil {
				return err
			}
		}
		return nil

	case "windows":
		cmd := exec.CommandContext(ctx, "wmic", "computersystem", "where", "name='%computername%'", "call", "rename", "name="+name)
		return cmd.Run()

	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// HostsModule manages /etc/hosts entries
type HostsModule struct {
	*BaseModule
}

// NewHostsModule creates a new hosts module
func NewHostsModule() *HostsModule {
	return &HostsModule{
		BaseModule: NewBaseModule("hosts", []string{"present", "absent"}),
	}
}

// Check checks if the hosts entry exists
func (m *HostsModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	ip := getStringParameter(decl, "ip", "")
	names := getStringSliceParameter(decl, "names")
	name := getStringParameter(decl, "name", "")

	if ip == "" {
		return nil, fmt.Errorf("ip parameter is required")
	}
	if len(names) == 0 && name == "" {
		return nil, fmt.Errorf("names or name parameter is required")
	}

	// Combine name into names if provided
	if name != "" && len(names) == 0 {
		names = []string{name}
	}

	hostsPath := m.getHostsPath()
	exists, currentNames, err := m.entryExists(hostsPath, ip)
	if err != nil {
		return nil, err
	}

	result := &ModuleCheckResult{
		Present:      exists,
		CurrentState: "absent",
		Matches:      false,
		Diff:         make(map[string]interface{}),
		Metadata: map[string]interface{}{
			"ip":            ip,
			"names":         names,
			"current_names": currentNames,
			"path":          hostsPath,
		},
	}

	if exists {
		result.CurrentState = "present"
	}

	switch decl.State {
	case "present":
		// Check if all names are present
		namesMatch := m.namesMatch(names, currentNames)
		result.Matches = exists && namesMatch
		if !result.Matches {
			result.Diff["entry"] = map[string]interface{}{
				"old": strings.Join(currentNames, " "),
				"new": strings.Join(names, " "),
			}
		}
	case "absent":
		result.Matches = !exists
		if exists {
			result.Diff["entry"] = map[string]interface{}{
				"old": fmt.Sprintf("%s %s", ip, strings.Join(currentNames, " ")),
				"new": nil,
			}
		}
	}

	return result, nil
}

// Apply adds or removes the hosts entry
func (m *HostsModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
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
			Comment: fmt.Sprintf("Hosts entry already %s", decl.State),
		}, nil
	}

	ip := getStringParameter(decl, "ip", "")
	names := getStringSliceParameter(decl, "names")
	name := getStringParameter(decl, "name", "")
	if name != "" && len(names) == 0 {
		names = []string{name}
	}
	hostsPath := m.getHostsPath()

	switch decl.State {
	case "present":
		if err := m.addEntry(hostsPath, ip, names); err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Changed: false,
				Comment: fmt.Sprintf("Failed to add entry: %v", err),
			}, nil
		}
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Added hosts entry: %s %s", ip, strings.Join(names, " ")),
		}, nil

	case "absent":
		if err := m.removeEntry(hostsPath, ip); err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Changed: false,
				Comment: fmt.Sprintf("Failed to remove entry: %v", err),
			}, nil
		}
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Removed hosts entry for %s", ip),
		}, nil
	}

	return nil, fmt.Errorf("unknown state: %s", decl.State)
}

// Test verifies the hosts entry
func (m *HostsModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

func (m *HostsModule) getHostsPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("SystemRoot"), "System32", "drivers", "etc", "hosts")
	}
	return "/etc/hosts"
}

func (m *HostsModule) entryExists(path, ip string) (exists bool, hostnames []string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return false, nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == ip {
			return true, fields[1:], nil
		}
	}

	return false, nil, scanner.Err()
}

func (m *HostsModule) namesMatch(expected, current []string) bool {
	if len(expected) != len(current) {
		return false
	}
	expectedSet := make(map[string]bool)
	for _, n := range expected {
		expectedSet[n] = true
	}
	for _, n := range current {
		if !expectedSet[n] {
			return false
		}
	}
	return true
}

func (m *HostsModule) addEntry(path, ip string, names []string) error {
	// Read current file
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Remove existing entry for this IP
	var lines []string
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			lines = append(lines, line)
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) >= 1 && fields[0] == ip {
			continue // Skip this entry
		}
		lines = append(lines, line)
	}

	// Add new entry
	newEntry := fmt.Sprintf("%s\t%s", ip, strings.Join(names, " "))
	lines = append(lines, newEntry)

	//nolint:gosec // G306: hosts file needs to be readable by the system
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func (m *HostsModule) removeEntry(path, ip string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var lines []string
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			lines = append(lines, line)
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) >= 1 && fields[0] == ip {
			continue // Skip this entry
		}
		lines = append(lines, line)
	}

	//nolint:gosec // G306: hosts file needs to be readable by the system
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

// SysctlModule manages Linux sysctl settings
type SysctlModule struct {
	*BaseModule
}

// NewSysctlModule creates a new sysctl module
func NewSysctlModule() *SysctlModule {
	return &SysctlModule{
		BaseModule: NewBaseModule("sysctl", []string{"present", "absent"}),
	}
}

// Check checks the sysctl value
func (m *SysctlModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("sysctl module is only available on Linux")
	}

	name := getStringParameter(decl, "name", "")
	value := getStringParameter(decl, "value", "")

	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	currentValue, exists, err := m.getValue(name)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	result := &ModuleCheckResult{
		Present:      exists,
		CurrentState: currentValue,
		Matches:      false,
		Diff:         make(map[string]interface{}),
		Metadata: map[string]interface{}{
			"name":    name,
			"value":   value,
			"current": currentValue,
		},
	}

	switch decl.State {
	case "present":
		result.Matches = exists && currentValue == value
		if !result.Matches {
			result.Diff[name] = map[string]interface{}{
				"old": currentValue,
				"new": value,
			}
		}
	case "absent":
		// For sysctl, absent typically means removing from sysctl.conf
		// The kernel parameter still exists with its default value
		result.Matches = !m.isInSysctlConf(name)
	}

	return result, nil
}

// Apply sets or removes the sysctl value
func (m *SysctlModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
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
			Comment: "Sysctl already configured correctly",
		}, nil
	}

	name := getStringParameter(decl, "name", "")
	value := getStringParameter(decl, "value", "")
	persistent := getBoolParameter(decl, "persistent", true)

	switch decl.State {
	case "present":
		if err := m.setValue(ctx, name, value, persistent); err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Changed: false,
				Comment: fmt.Sprintf("Failed to set sysctl: %v", err),
			}, nil
		}
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Set %s = %s", name, value),
		}, nil

	case "absent":
		if err := m.removeFromSysctlConf(name); err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Changed: false,
				Comment: fmt.Sprintf("Failed to remove from sysctl.conf: %v", err),
			}, nil
		}
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Removed %s from sysctl.conf", name),
		}, nil
	}

	return nil, fmt.Errorf("unknown state: %s", decl.State)
}

// Test verifies the sysctl value
func (m *SysctlModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

func (m *SysctlModule) getValue(name string) (value string, exists bool, err error) {
	// Convert to path format (net.ipv4.ip_forward -> /proc/sys/net/ipv4/ip_forward)
	path := filepath.Join("/proc", "sys", strings.ReplaceAll(name, ".", "/"))

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}

	return strings.TrimSpace(string(data)), true, nil
}

func (m *SysctlModule) setValue(ctx context.Context, name, value string, persistent bool) error {
	// Set immediately
	cmd := exec.CommandContext(ctx, "sysctl", "-w", fmt.Sprintf("%s=%s", name, value))
	if err := cmd.Run(); err != nil {
		return err
	}

	// Make persistent if requested
	if persistent {
		return m.addToSysctlConf(name, value)
	}
	return nil
}

func (m *SysctlModule) addToSysctlConf(name, value string) error {
	confPath := "/etc/sysctl.d/99-keystone.conf"

	// Read current entries
	entries := make(map[string]string)
	if data, err := os.ReadFile(confPath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				entries[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
	}

	// Update entry
	entries[name] = value

	// Write back
	lines := make([]string, 0, 1+len(entries))
	lines = append(lines, "# Managed by Keystone Core")
	for k, v := range entries {
		lines = append(lines, fmt.Sprintf("%s = %s", k, v))
	}

	//nolint:gosec // G301: sysctl.d directory needs system access
	if err := os.MkdirAll("/etc/sysctl.d", 0o755); err != nil {
		return err
	}
	//nolint:gosec // G306: sysctl config files need to be readable by the kernel
	return os.WriteFile(confPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func (m *SysctlModule) isInSysctlConf(name string) bool {
	confPath := "/etc/sysctl.d/99-keystone.conf"
	data, err := os.ReadFile(confPath)
	if err != nil {
		return false
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, name+" ") || strings.HasPrefix(line, name+"=") {
			return true
		}
	}
	return false
}

func (m *SysctlModule) removeFromSysctlConf(name string) error {
	confPath := "/etc/sysctl.d/99-keystone.conf"
	data, err := os.ReadFile(confPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, name+" ") || strings.HasPrefix(trimmed, name+"=") {
			continue
		}
		lines = append(lines, line)
	}

	//nolint:gosec // G306: sysctl config files need to be readable by the kernel
	return os.WriteFile(confPath, []byte(strings.Join(lines, "\n")), 0o644)
}

// KernelModuleModule manages Linux kernel modules
type KernelModuleModule struct {
	*BaseModule
}

// NewKernelModuleModule creates a new kernel module module
func NewKernelModuleModule() *KernelModuleModule {
	return &KernelModuleModule{
		BaseModule: NewBaseModule("kernel_module", []string{"loaded", "unloaded", "blacklisted"}),
	}
}

// Check checks if the kernel module is loaded
func (m *KernelModuleModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("kernel_module is only available on Linux")
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	loaded, err := m.isLoaded(name)
	if err != nil {
		return nil, err
	}

	blacklisted := m.isBlacklisted(name)

	result := &ModuleCheckResult{
		Present:      loaded,
		CurrentState: "unloaded",
		Matches:      false,
		Diff:         make(map[string]interface{}),
		Metadata: map[string]interface{}{
			"name":        name,
			"loaded":      loaded,
			"blacklisted": blacklisted,
		},
	}

	if loaded {
		result.CurrentState = "loaded"
	}
	if blacklisted {
		result.CurrentState = "blacklisted"
	}

	switch decl.State {
	case "loaded":
		result.Matches = loaded && !blacklisted
	case "unloaded":
		result.Matches = !loaded
	case "blacklisted":
		result.Matches = blacklisted && !loaded
	}

	if !result.Matches {
		result.Diff["state"] = map[string]interface{}{
			"old": result.CurrentState,
			"new": decl.State,
		}
	}

	return result, nil
}

// Apply loads, unloads, or blacklists the kernel module
func (m *KernelModuleModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
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
			Comment: fmt.Sprintf("Kernel module already %s", decl.State),
		}, nil
	}

	name := getStringParameter(decl, "name", "")
	params := getStringParameter(decl, "params", "")
	persistent := getBoolParameter(decl, "persistent", false)

	switch decl.State {
	case "loaded":
		// Remove from blacklist if needed
		if m.isBlacklisted(name) {
			_ = m.removeFromBlacklist(name) //nolint:errcheck // best-effort blacklist removal
		}

		if err := m.loadModule(ctx, name, params); err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Changed: false,
				Comment: fmt.Sprintf("Failed to load module: %v", err),
			}, nil
		}

		if persistent {
			_ = m.addToModulesLoad(name) //nolint:errcheck // best-effort persistent config
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Loaded kernel module %s", name),
		}, nil

	case "unloaded":
		if err := m.unloadModule(ctx, name); err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Changed: false,
				Comment: fmt.Sprintf("Failed to unload module: %v", err),
			}, nil
		}

		if persistent {
			_ = m.removeFromModulesLoad(name) //nolint:errcheck // best-effort persistent config
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Unloaded kernel module %s", name),
		}, nil

	case "blacklisted":
		// Unload if loaded
		if checkResult.Present {
			_ = m.unloadModule(ctx, name) //nolint:errcheck // best-effort unload before blacklist
		}

		if err := m.addToBlacklist(name); err != nil {
			return &StateResult{
				StateID: decl.ID,
				Module:  m.Name(),
				Success: false,
				Changed: false,
				Comment: fmt.Sprintf("Failed to blacklist module: %v", err),
			}, nil
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Blacklisted kernel module %s", name),
		}, nil
	}

	return nil, fmt.Errorf("unknown state: %s", decl.State)
}

// Test verifies the kernel module state
func (m *KernelModuleModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

func (m *KernelModuleModule) isLoaded(name string) (bool, error) {
	data, err := os.ReadFile("/proc/modules")
	if err != nil {
		return false, err
	}

	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 1 && fields[0] == name {
			return true, nil
		}
	}
	return false, nil
}

func (m *KernelModuleModule) isBlacklisted(name string) bool {
	blacklistPath := "/etc/modprobe.d/keystone-blacklist.conf"
	data, err := os.ReadFile(blacklistPath)
	if err != nil {
		return false
	}

	pattern := regexp.MustCompile(`^\s*blacklist\s+` + regexp.QuoteMeta(name) + `\s*$`)
	for _, line := range strings.Split(string(data), "\n") {
		if pattern.MatchString(line) {
			return true
		}
	}
	return false
}

func (m *KernelModuleModule) loadModule(ctx context.Context, name, params string) error {
	args := []string{name}
	if params != "" {
		args = append(args, params)
	}
	cmd := exec.CommandContext(ctx, "modprobe", args...)
	return cmd.Run()
}

func (m *KernelModuleModule) unloadModule(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "modprobe", "-r", name)
	return cmd.Run()
}

func (m *KernelModuleModule) addToBlacklist(name string) error {
	blacklistPath := "/etc/modprobe.d/keystone-blacklist.conf"

	var lines []string
	if data, err := os.ReadFile(blacklistPath); err == nil {
		lines = strings.Split(string(data), "\n")
	} else {
		lines = []string{"# Managed by Keystone Core"}
	}

	// Check if already blacklisted
	for _, line := range lines {
		if strings.TrimSpace(line) == fmt.Sprintf("blacklist %s", name) {
			return nil
		}
	}

	lines = append(lines, fmt.Sprintf("blacklist %s", name))
	//nolint:gosec // G306: modprobe.d config files need to be readable by the kernel
	return os.WriteFile(blacklistPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func (m *KernelModuleModule) removeFromBlacklist(name string) error {
	blacklistPath := "/etc/modprobe.d/keystone-blacklist.conf"
	data, err := os.ReadFile(blacklistPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var lines []string
	pattern := regexp.MustCompile(`^\s*blacklist\s+` + regexp.QuoteMeta(name) + `\s*$`)
	for _, line := range strings.Split(string(data), "\n") {
		if !pattern.MatchString(line) {
			lines = append(lines, line)
		}
	}

	//nolint:gosec // G306: modprobe.d config files need to be readable by the kernel
	return os.WriteFile(blacklistPath, []byte(strings.Join(lines, "\n")), 0o644)
}

func (m *KernelModuleModule) addToModulesLoad(name string) error {
	path := "/etc/modules-load.d/keystone.conf"

	var lines []string
	if data, err := os.ReadFile(path); err == nil {
		lines = strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == name {
				return nil // Already present
			}
		}
	} else {
		lines = []string{"# Managed by Keystone Core"}
	}

	lines = append(lines, name)
	//nolint:gosec // G301: modules-load.d directory needs system access
	if err := os.MkdirAll("/etc/modules-load.d", 0o755); err != nil {
		return err
	}
	//nolint:gosec // G306: modules-load.d config files need to be readable by systemd
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func (m *KernelModuleModule) removeFromModulesLoad(name string) error {
	path := "/etc/modules-load.d/keystone.conf"
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) != name {
			lines = append(lines, line)
		}
	}

	//nolint:gosec // G306: modules-load.d config files need to be readable by systemd
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

func init() {
	_ = RegisterModule(NewTimezoneModule())
	_ = RegisterModule(NewLocaleModule())
	_ = RegisterModule(NewHostnameModule())
	_ = RegisterModule(NewHostsModule())
	_ = RegisterModule(NewSysctlModule())
	_ = RegisterModule(NewKernelModuleModule())
}
