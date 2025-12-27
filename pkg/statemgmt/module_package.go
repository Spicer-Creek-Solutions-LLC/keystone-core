package statemgmt

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/platform"
)

// PackageModule implements package management
type PackageModule struct {
	*BaseModule
}

// NewPackageModule creates a new package module
func NewPackageModule() *PackageModule {
	return &PackageModule{
		BaseModule: NewBaseModule("package", []string{"installed", "removed", "latest", "purged"}),
	}
}

// Check checks if a package is installed
func (m *PackageModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	pkgName := decl.ID
	pm, err := m.detectPackageManager()
	if err != nil {
		return nil, err
	}

	// Check if package is installed
	installed, version, err := m.isPackageInstalled(ctx, pm, pkgName)
	if err != nil {
		return nil, fmt.Errorf("failed to check package status: %w", err)
	}

	result.Present = installed
	if installed {
		result.CurrentState = "installed"
		result.Metadata["version"] = version
	} else {
		result.CurrentState = "removed"
	}

	// Determine if state matches
	switch decl.State {
	case "installed", "latest":
		result.Matches = installed
		if !installed {
			result.Diff["state"] = map[string]string{"current": "removed", "desired": decl.State}
		} else if decl.State == "latest" {
			// For "latest", we would need to check if an update is available
			// For now, just mark as matching if installed
			result.Matches = true
		}

	case "removed", "purged":
		result.Matches = !installed
		if installed {
			result.Diff["state"] = map[string]string{"current": "installed", "desired": decl.State}
		}
	}

	// Check version constraint if specified
	if desiredVersion := getStringParameter(decl, "version", ""); desiredVersion != "" && installed {
		if version != desiredVersion {
			result.Matches = false
			result.Diff["version"] = map[string]string{"current": version, "desired": desiredVersion}
		}
	}

	return result, nil
}

// Apply applies the package state
func (m *PackageModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
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

	pm, err := m.detectPackageManager()
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to detect package manager: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Apply changes
	var applyErr error
	switch decl.State {
	case "installed":
		applyErr = m.installPackage(ctx, pm, decl, result)
	case "latest":
		applyErr = m.installOrUpgradePackage(ctx, pm, decl, result)
	case "removed":
		applyErr = m.removePackage(ctx, pm, decl, result, false)
	case "purged":
		applyErr = m.removePackage(ctx, pm, decl, result, true)
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
func (m *PackageModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// PackageManager represents different package managers
type PackageManager string

const (
	PMUnknown   PackageManager = "unknown"
	PMApt       PackageManager = "apt"
	PMYum       PackageManager = "yum"
	PMDNF       PackageManager = "dnf"
	PMApk       PackageManager = "apk"
	PMBrew      PackageManager = "brew"
	PMPacman    PackageManager = "pacman"
	PMZypper    PackageManager = "zypper"
	PMChoco     PackageManager = "chocolatey"
	PMWinget    PackageManager = "winget"
)

// detectPackageManager detects the available package manager using platform detection
func (m *PackageModule) detectPackageManager() (PackageManager, error) {
	// Use platform detection for more accurate detection
	platformPM, err := platform.DetectPackageManager()
	if err == nil && platformPM != platform.PackageManagerUnknown {
		return convertPlatformPM(platformPM), nil
	}

	// Fallback to manual detection
	managers := []struct {
		name    PackageManager
		command string
	}{
		{PMApt, "apt-get"},
		{PMDNF, "dnf"},
		{PMYum, "yum"},
		{PMApk, "apk"},
		{PMPacman, "pacman"},
		{PMZypper, "zypper"},
		{PMBrew, "brew"},
		{PMChoco, "choco"},
		{PMWinget, "winget"},
	}

	for _, mgr := range managers {
		if _, err := exec.LookPath(mgr.command); err == nil {
			return mgr.name, nil
		}
	}

	return PMUnknown, fmt.Errorf("no supported package manager found on %s", runtime.GOOS)
}

// convertPlatformPM converts platform.PackageManager to statemgmt.PackageManager
func convertPlatformPM(pm platform.PackageManager) PackageManager {
	switch pm {
	case platform.PackageManagerAPT:
		return PMApt
	case platform.PackageManagerYum:
		return PMYum
	case platform.PackageManagerDNF:
		return PMDNF
	case platform.PackageManagerZypper:
		return PMZypper
	case platform.PackageManagerPacman:
		return PMPacman
	case platform.PackageManagerAPK:
		return PMApk
	case platform.PackageManagerBrew:
		return PMBrew
	case platform.PackageManagerChocolatey:
		return PMChoco
	case platform.PackageManagerWinget:
		return PMWinget
	default:
		return PMUnknown
	}
}

// isPackageInstalled checks if a package is installed
func (m *PackageModule) isPackageInstalled(ctx context.Context, pm PackageManager, pkgName string) (bool, string, error) {
	var cmd *exec.Cmd

	switch pm {
	case PMApt:
		cmd = exec.CommandContext(ctx, "dpkg-query", "-W", "-f=${Status} ${Version}", pkgName)
	case PMYum, PMDNF:
		cmd = exec.CommandContext(ctx, "rpm", "-q", pkgName)
	case PMApk:
		cmd = exec.CommandContext(ctx, "apk", "info", "-e", pkgName)
	case PMBrew:
		cmd = exec.CommandContext(ctx, "brew", "list", "--versions", pkgName)
	case PMPacman:
		cmd = exec.CommandContext(ctx, "pacman", "-Q", pkgName)
	case PMZypper:
		cmd = exec.CommandContext(ctx, "rpm", "-q", pkgName)
	default:
		return false, "", fmt.Errorf("unsupported package manager: %s", pm)
	}

	output, err := cmd.Output()
	if err != nil {
		// Package not installed
		return false, "", nil
	}

	outputStr := strings.TrimSpace(string(output))

	// Parse version based on package manager
	var version string
	switch pm {
	case PMApt:
		// Output: "install ok installed <version>"
		if strings.Contains(outputStr, "install ok installed") {
			parts := strings.Fields(outputStr)
			if len(parts) > 3 {
				version = parts[3]
			}
			return true, version, nil
		}
		return false, "", nil

	case PMYum, PMDNF, PMZypper:
		// Output: "package-name-version-release.arch"
		version = outputStr
		return true, version, nil

	case PMApk:
		// Output: "package-version" if installed
		if outputStr != "" {
			version = strings.TrimPrefix(outputStr, pkgName+"-")
			return true, version, nil
		}
		return false, "", nil

	case PMBrew:
		// Output: "package version"
		parts := strings.Fields(outputStr)
		if len(parts) > 1 {
			version = parts[1]
		}
		return true, version, nil

	case PMPacman:
		// Output: "package version"
		parts := strings.Fields(outputStr)
		if len(parts) > 1 {
			version = parts[1]
		}
		return true, version, nil
	}

	return false, "", nil
}

// installPackage installs a package
func (m *PackageModule) installPackage(ctx context.Context, pm PackageManager, decl *StateDeclaration, result *StateResult) error {
	pkgName := decl.ID
	version := getStringParameter(decl, "version", "")

	var cmd *exec.Cmd
	switch pm {
	case PMApt:
		if version != "" {
			pkgName = fmt.Sprintf("%s=%s", pkgName, version)
		}
		cmd = exec.CommandContext(ctx, "apt-get", "install", "-y", pkgName)

	case PMDNF:
		if version != "" {
			pkgName = fmt.Sprintf("%s-%s", pkgName, version)
		}
		cmd = exec.CommandContext(ctx, "dnf", "install", "-y", pkgName)

	case PMYum:
		if version != "" {
			pkgName = fmt.Sprintf("%s-%s", pkgName, version)
		}
		cmd = exec.CommandContext(ctx, "yum", "install", "-y", pkgName)

	case PMApk:
		if version != "" {
			pkgName = fmt.Sprintf("%s=%s", pkgName, version)
		}
		cmd = exec.CommandContext(ctx, "apk", "add", pkgName)

	case PMBrew:
		cmd = exec.CommandContext(ctx, "brew", "install", pkgName)

	case PMPacman:
		cmd = exec.CommandContext(ctx, "pacman", "-S", "--noconfirm", pkgName)

	case PMZypper:
		cmd = exec.CommandContext(ctx, "zypper", "--non-interactive", "install", pkgName)

	default:
		return fmt.Errorf("unsupported package manager: %s", pm)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install package: %w (output: %s)", err, string(output))
	}

	result.Comment = fmt.Sprintf("Package %s installed", pkgName)
	return nil
}

// installOrUpgradePackage installs or upgrades a package to the latest version
func (m *PackageModule) installOrUpgradePackage(ctx context.Context, pm PackageManager, decl *StateDeclaration, result *StateResult) error {
	// For now, just install (upgrade support could be added later)
	return m.installPackage(ctx, pm, decl, result)
}

// removePackage removes a package
func (m *PackageModule) removePackage(ctx context.Context, pm PackageManager, decl *StateDeclaration, result *StateResult, purge bool) error {
	pkgName := decl.ID

	var cmd *exec.Cmd
	switch pm {
	case PMApt:
		if purge {
			cmd = exec.CommandContext(ctx, "apt-get", "purge", "-y", pkgName)
		} else {
			cmd = exec.CommandContext(ctx, "apt-get", "remove", "-y", pkgName)
		}

	case PMDNF:
		cmd = exec.CommandContext(ctx, "dnf", "remove", "-y", pkgName)

	case PMYum:
		cmd = exec.CommandContext(ctx, "yum", "remove", "-y", pkgName)

	case PMApk:
		if purge {
			cmd = exec.CommandContext(ctx, "apk", "del", "--purge", pkgName)
		} else {
			cmd = exec.CommandContext(ctx, "apk", "del", pkgName)
		}

	case PMBrew:
		cmd = exec.CommandContext(ctx, "brew", "uninstall", pkgName)

	case PMPacman:
		cmd = exec.CommandContext(ctx, "pacman", "-R", "--noconfirm", pkgName)

	case PMZypper:
		cmd = exec.CommandContext(ctx, "zypper", "--non-interactive", "remove", pkgName)

	default:
		return fmt.Errorf("unsupported package manager: %s", pm)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove package: %w (output: %s)", err, string(output))
	}

	if purge {
		result.Comment = fmt.Sprintf("Package %s purged", pkgName)
	} else {
		result.Comment = fmt.Sprintf("Package %s removed", pkgName)
	}
	return nil
}

func init() {
	RegisterModule(NewPackageModule())
}
