// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package statemgmt

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// WinPackageModule implements Windows package management
// Supports Chocolatey, winget, MSI, and EXE installers
type WinPackageModule struct {
	*BaseModule
}

// NewWinPackageModule creates a new Windows package module
func NewWinPackageModule() *WinPackageModule {
	return &WinPackageModule{
		BaseModule: NewBaseModule("win_package", []string{
			"installed", "removed", "latest",
		}),
	}
}

// PackageSource represents the package source type
type PackageSource string

const (
	SourceChocolatey PackageSource = "chocolatey"
	SourceWinget     PackageSource = "winget"
	SourceMSI        PackageSource = "msi"
	SourceEXE        PackageSource = "exe"
	SourceAuto       PackageSource = "auto"
)

// WinPackageInfo represents information about a Windows package
type WinPackageInfo struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Source    string `json:"source"`
	Publisher string `json:"publisher"`
	Installed bool   `json:"installed"`
}

// Check checks the current state of a Windows package
func (m *WinPackageModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	pkgName := getStringParameter(decl, "name", decl.ID)
	source := PackageSource(getStringParameter(decl, "source", "auto"))
	version := getStringParameter(decl, "version", "")

	// Determine package source if auto
	if source == SourceAuto {
		source = m.detectPackageSource(ctx, pkgName, decl)
	}

	result.Metadata["source"] = string(source)

	// Check if package is installed based on source
	var pkgInfo *WinPackageInfo
	var err error

	switch source {
	case SourceChocolatey:
		pkgInfo, err = m.checkChocolateyPackage(ctx, pkgName)
	case SourceWinget:
		pkgInfo, err = m.checkWingetPackage(ctx, pkgName)
	case SourceMSI, SourceEXE:
		pkgInfo, err = m.checkInstalledProgram(ctx, pkgName)
	default:
		return nil, fmt.Errorf("unsupported package source: %s", source)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to check package: %w", err)
	}

	if pkgInfo != nil && pkgInfo.Installed {
		result.Present = true
		result.CurrentState = "installed"
		result.Metadata["name"] = pkgInfo.Name
		result.Metadata["version"] = pkgInfo.Version
		result.Metadata["publisher"] = pkgInfo.Publisher
	} else {
		result.Present = false
		result.CurrentState = "absent"
	}

	// Compare with desired state
	switch decl.State {
	case "installed":
		if result.Present {
			// Check version if specified
			if version != "" && pkgInfo.Version != version {
				result.Matches = false
				result.Diff["version"] = map[string]string{
					"current": pkgInfo.Version,
					"desired": version,
				}
			} else {
				result.Matches = true
			}
		} else {
			result.Matches = false
			result.Diff["state"] = map[string]string{"current": "absent", "desired": "installed"}
		}

	case "removed":
		if !result.Present {
			result.Matches = true
		} else {
			result.Matches = false
			result.Diff["state"] = map[string]string{"current": "installed", "desired": "removed"}
		}

	case "latest":
		if !result.Present {
			result.Matches = false
			result.Diff["state"] = map[string]string{"current": "absent", "desired": "latest"}
		} else {
			// Check if an update is available
			hasUpdate, newVersion := m.checkForUpdate(ctx, pkgName, source)
			if hasUpdate {
				result.Matches = false
				result.Diff["version"] = map[string]string{
					"current": pkgInfo.Version,
					"desired": newVersion,
				}
			} else {
				result.Matches = true
			}
		}
	}

	return result, nil
}

// Apply applies the Windows package state
func (m *WinPackageModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		Success:   false,
		Changed:   false,
		Changes:   make(map[string]interface{}),
		StartTime: startTime,
	}

	// Check current state
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to check current state: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// If already in desired state, no changes needed
	if checkResult.Matches {
		result.Success = true
		result.Changed = false
		result.Comment = "Already in desired state"
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	pkgName := getStringParameter(decl, "name", decl.ID)
	source := PackageSource(getStringParameter(decl, "source", "auto"))
	version := getStringParameter(decl, "version", "")

	// Determine package source if auto
	if source == SourceAuto {
		source = m.detectPackageSource(ctx, pkgName, decl)
	}

	var applyErr error
	switch decl.State {
	case "installed", "latest":
		applyErr = m.installPackage(ctx, pkgName, version, source, decl, result)
	case "removed":
		applyErr = m.removePackage(ctx, pkgName, source, decl, result)
	default:
		applyErr = fmt.Errorf("unsupported state: %s", decl.State)
	}

	if applyErr != nil {
		result.Error = applyErr
		result.Success = false
		result.Comment = fmt.Sprintf("Failed to apply state: %v", applyErr)
	} else {
		result.Success = true
		result.Changed = true
		result.Changes = checkResult.Diff
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil
}

// Test tests if the package is in the desired state
func (m *WinPackageModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// detectPackageSource determines the best package source for a package
func (m *WinPackageModule) detectPackageSource(ctx context.Context, name string, decl *StateDeclaration) PackageSource {
	// Check for explicit installer path
	if hasParameter(decl, "installer") {
		installer := getStringParameter(decl, "installer", "")
		if strings.HasSuffix(strings.ToLower(installer), ".msi") {
			return SourceMSI
		}
		if strings.HasSuffix(strings.ToLower(installer), ".exe") {
			return SourceEXE
		}
	}

	// Try Chocolatey first if available
	if m.isChocolateyAvailable(ctx) {
		return SourceChocolatey
	}

	// Try winget if available
	if m.isWingetAvailable(ctx) {
		return SourceWinget
	}

	// Default to Chocolatey (will fail if not installed)
	return SourceChocolatey
}

// isChocolateyAvailable checks if Chocolatey is installed
func (m *WinPackageModule) isChocolateyAvailable(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "choco", "--version")
	err := cmd.Run()
	return err == nil
}

// isWingetAvailable checks if winget is installed
func (m *WinPackageModule) isWingetAvailable(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "winget", "--version")
	err := cmd.Run()
	return err == nil
}

// checkChocolateyPackage checks if a Chocolatey package is installed
func (m *WinPackageModule) checkChocolateyPackage(ctx context.Context, name string) (*WinPackageInfo, error) {
	cmd := exec.CommandContext(ctx, "choco", "list", "--local-only", "--exact", name, "-r")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Not installed
		return &WinPackageInfo{Name: name, Installed: false}, nil
	}

	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" {
		return &WinPackageInfo{Name: name, Installed: false}, nil
	}

	// Parse: package|version
	lines := strings.Split(outputStr, "\n")
	for _, line := range lines {
		parts := strings.Split(strings.TrimSpace(line), "|")
		if len(parts) >= 2 && strings.EqualFold(parts[0], name) {
			return &WinPackageInfo{
				Name:      parts[0],
				Version:   parts[1],
				Source:    "chocolatey",
				Installed: true,
			}, nil
		}
	}

	return &WinPackageInfo{Name: name, Installed: false}, nil
}

// checkWingetPackage checks if a winget package is installed
func (m *WinPackageModule) checkWingetPackage(ctx context.Context, name string) (*WinPackageInfo, error) {
	cmd := exec.CommandContext(ctx, "winget", "list", "--id", name, "--exact", "--accept-source-agreements")
	output, err := cmd.CombinedOutput()

	outputStr := string(output)
	if err != nil || strings.Contains(outputStr, "No installed package found") {
		return &WinPackageInfo{Name: name, Installed: false}, nil
	}

	// Parse winget output (table format)
	lines := strings.Split(outputStr, "\n")
	for _, line := range lines {
		if strings.Contains(line, name) {
			// Extract version from the line (format varies)
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return &WinPackageInfo{
					Name:      name,
					Version:   fields[len(fields)-1], // Version is usually last
					Source:    "winget",
					Installed: true,
				}, nil
			}
		}
	}

	return &WinPackageInfo{Name: name, Installed: false}, nil
}

// checkInstalledProgram checks if a program is installed via Windows registry
func (m *WinPackageModule) checkInstalledProgram(ctx context.Context, name string) (*WinPackageInfo, error) {
	// Use PowerShell to query installed programs
	psScript := fmt.Sprintf(`
$apps = @()
$apps += Get-ItemProperty "HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*" -ErrorAction SilentlyContinue
$apps += Get-ItemProperty "HKLM:\Software\Wow6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*" -ErrorAction SilentlyContinue
$apps += Get-ItemProperty "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*" -ErrorAction SilentlyContinue

$found = $apps | Where-Object { $_.DisplayName -like '*%s*' } | Select-Object -First 1

if ($found) {
    @{
        Name = $found.DisplayName
        Version = $found.DisplayVersion
        Publisher = $found.Publisher
        Installed = $true
    } | ConvertTo-Json
} else {
    @{
        Name = '%s'
        Installed = $false
    } | ConvertTo-Json
}
`, escapeForPowerShellQuote(name), escapeForPowerShellQuote(name))

	output, err := m.runPowerShellPackage(ctx, psScript)
	if err != nil {
		return nil, fmt.Errorf("failed to query installed programs: %w", err)
	}

	var info WinPackageInfo
	if err := json.Unmarshal([]byte(output), &info); err != nil {
		return nil, fmt.Errorf("failed to parse installed program info: %w", err)
	}

	return &info, nil
}

// checkForUpdate checks if an update is available for a package
func (m *WinPackageModule) checkForUpdate(ctx context.Context, name string, source PackageSource) (bool, string) {
	switch source {
	case SourceChocolatey:
		cmd := exec.CommandContext(ctx, "choco", "outdated", "-r")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return false, ""
		}

		// Parse: package|currentVersion|availableVersion|pinned
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			parts := strings.Split(strings.TrimSpace(line), "|")
			if len(parts) >= 3 && strings.EqualFold(parts[0], name) {
				return true, parts[2]
			}
		}

	case SourceWinget:
		cmd := exec.CommandContext(ctx, "winget", "upgrade", "--id", name, "--exact", "--accept-source-agreements")
		output, _ := cmd.CombinedOutput()
		if strings.Contains(string(output), "Available") {
			// Try to extract the new version
			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				if strings.Contains(line, name) {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						return true, fields[len(fields)-1]
					}
				}
			}
			return true, "latest"
		}
	}

	return false, ""
}

// installPackage installs a package using the specified source
func (m *WinPackageModule) installPackage(ctx context.Context, name, version string, source PackageSource, decl *StateDeclaration, result *StateResult) error {
	switch source {
	case SourceChocolatey:
		return m.installChocolateyPackage(ctx, name, version, decl, result)
	case SourceWinget:
		return m.installWingetPackage(ctx, name, version, decl, result)
	case SourceMSI:
		return m.installMSIPackage(ctx, decl, result)
	case SourceEXE:
		return m.installEXEPackage(ctx, decl, result)
	default:
		return fmt.Errorf("unsupported package source: %s", source)
	}
}

// installChocolateyPackage installs a package via Chocolatey
func (m *WinPackageModule) installChocolateyPackage(ctx context.Context, name, version string, decl *StateDeclaration, result *StateResult) error {
	args := []string{"install", name, "-y", "--no-progress"}

	if version != "" {
		args = append(args, "--version", version)
	}

	if getBoolParameter(decl, "force", false) {
		args = append(args, "--force")
	}

	if getBoolParameter(decl, "allow_downgrade", false) {
		args = append(args, "--allow-downgrade")
	}

	if params := getStringParameter(decl, "install_args", ""); params != "" {
		args = append(args, "--install-arguments", fmt.Sprintf(`"%s"`, params))
	}

	if params := getStringParameter(decl, "package_params", ""); params != "" {
		args = append(args, "--package-parameters", fmt.Sprintf(`"%s"`, params))
	}

	cmd := exec.CommandContext(ctx, "choco", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("chocolatey install failed: %w, output: %s", err, output)
	}

	result.Comment = fmt.Sprintf("Package %s installed via Chocolatey", name)
	return nil
}

// installWingetPackage installs a package via winget
func (m *WinPackageModule) installWingetPackage(ctx context.Context, name, version string, decl *StateDeclaration, result *StateResult) error {
	args := []string{"install", "--id", name, "--exact", "--silent", "--accept-package-agreements", "--accept-source-agreements"}

	if version != "" {
		args = append(args, "--version", version)
	}

	if getBoolParameter(decl, "force", false) {
		args = append(args, "--force")
	}

	cmd := exec.CommandContext(ctx, "winget", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("winget install failed: %w, output: %s", err, output)
	}

	result.Comment = fmt.Sprintf("Package %s installed via winget", name)
	return nil
}

// installMSIPackage installs an MSI package
func (m *WinPackageModule) installMSIPackage(ctx context.Context, decl *StateDeclaration, result *StateResult) error {
	installer := getStringParameter(decl, "installer", "")
	if installer == "" {
		return fmt.Errorf("installer path is required for MSI packages")
	}

	// Resolve path
	installerPath, err := filepath.Abs(installer)
	if err != nil {
		return fmt.Errorf("invalid installer path: %w", err)
	}

	args := []string{"/i", installerPath, "/qn", "/norestart"}

	if params := getStringParameter(decl, "install_args", ""); params != "" {
		args = append(args, strings.Fields(params)...)
	}

	if logFile := getStringParameter(decl, "log_file", ""); logFile != "" {
		args = append(args, "/l*v", logFile)
	}

	cmd := exec.CommandContext(ctx, "msiexec.exe", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("MSI install failed: %w, output: %s", err, output)
	}

	result.Comment = fmt.Sprintf("MSI package installed: %s", installer)
	return nil
}

// installEXEPackage installs an EXE package
func (m *WinPackageModule) installEXEPackage(ctx context.Context, decl *StateDeclaration, result *StateResult) error {
	installer := getStringParameter(decl, "installer", "")
	if installer == "" {
		return fmt.Errorf("installer path is required for EXE packages")
	}

	// Resolve path
	installerPath, err := filepath.Abs(installer)
	if err != nil {
		return fmt.Errorf("invalid installer path: %w", err)
	}

	// Default silent install arguments for common installers
	silentArgs := getStringParameter(decl, "install_args", "/S /SILENT /VERYSILENT /quiet")

	args := strings.Fields(silentArgs)

	cmd := exec.CommandContext(ctx, installerPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("EXE install failed: %w, output: %s", err, output)
	}

	result.Comment = fmt.Sprintf("EXE package installed: %s", installer)
	return nil
}

// removePackage removes a package using the specified source
func (m *WinPackageModule) removePackage(ctx context.Context, name string, source PackageSource, decl *StateDeclaration, result *StateResult) error {
	switch source {
	case SourceChocolatey:
		return m.removeChocolateyPackage(ctx, name, decl, result)
	case SourceWinget:
		return m.removeWingetPackage(ctx, name, decl, result)
	case SourceMSI, SourceEXE:
		return m.removeInstalledProgram(ctx, name, decl, result)
	default:
		return fmt.Errorf("unsupported package source: %s", source)
	}
}

// removeChocolateyPackage removes a package via Chocolatey
func (m *WinPackageModule) removeChocolateyPackage(ctx context.Context, name string, decl *StateDeclaration, result *StateResult) error {
	args := []string{"uninstall", name, "-y", "--no-progress"}

	if getBoolParameter(decl, "remove_dependencies", false) {
		args = append(args, "--remove-dependencies")
	}

	cmd := exec.CommandContext(ctx, "choco", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("chocolatey uninstall failed: %w, output: %s", err, output)
	}

	result.Comment = fmt.Sprintf("Package %s removed via Chocolatey", name)
	return nil
}

// removeWingetPackage removes a package via winget
func (m *WinPackageModule) removeWingetPackage(ctx context.Context, name string, decl *StateDeclaration, result *StateResult) error {
	args := []string{"uninstall", "--id", name, "--exact", "--silent", "--accept-source-agreements"}

	cmd := exec.CommandContext(ctx, "winget", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("winget uninstall failed: %w, output: %s", err, output)
	}

	result.Comment = fmt.Sprintf("Package %s removed via winget", name)
	return nil
}

// removeInstalledProgram removes a program installed via MSI/EXE
func (m *WinPackageModule) removeInstalledProgram(ctx context.Context, name string, decl *StateDeclaration, result *StateResult) error {
	// Try to find the uninstall command from registry
	psScript := fmt.Sprintf(`
$apps = @()
$apps += Get-ItemProperty "HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*" -ErrorAction SilentlyContinue
$apps += Get-ItemProperty "HKLM:\Software\Wow6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*" -ErrorAction SilentlyContinue
$apps += Get-ItemProperty "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*" -ErrorAction SilentlyContinue

$found = $apps | Where-Object { $_.DisplayName -like '*%s*' } | Select-Object -First 1

if ($found -and $found.UninstallString) {
    $found.UninstallString
} else {
    ''
}
`, escapeForPowerShellQuote(name))

	uninstallString, err := m.runPowerShellPackage(ctx, psScript)
	if err != nil {
		return fmt.Errorf("failed to find uninstall command: %w", err)
	}

	uninstallString = strings.TrimSpace(uninstallString)
	if uninstallString == "" {
		return fmt.Errorf("no uninstall command found for %s", name)
	}

	// Execute the uninstall command with silent flags
	var cmd *exec.Cmd
	if strings.Contains(strings.ToLower(uninstallString), "msiexec") {
		// MSI uninstaller
		args := strings.Fields(uninstallString)
		args = append(args, "/qn", "/norestart")
		cmd = exec.CommandContext(ctx, args[0], args[1:]...)
	} else {
		// EXE uninstaller - try common silent flags
		silentArgs := getStringParameter(decl, "uninstall_args", "/S /SILENT /VERYSILENT /quiet")
		cmd = exec.CommandContext(ctx, "cmd", "/c", uninstallString+" "+silentArgs)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("uninstall failed: %w, output: %s", err, output)
	}

	result.Comment = fmt.Sprintf("Program %s removed", name)
	return nil
}

// runPowerShellPackage runs a PowerShell script and returns the output
func (m *WinPackageModule) runPowerShellPackage(ctx context.Context, script string) (string, error) {
	cmd := exec.CommandContext(ctx, "powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy", "Bypass",
		"-Command", script)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("PowerShell command failed: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

func init() {
	RegisterModule(NewWinPackageModule())
}
