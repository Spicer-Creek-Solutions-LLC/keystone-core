// Package state provides WinRM state modules for Windows systems.
package state

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// =============================================================================
// WinRM Modules
// =============================================================================

// WinRMFileModule manages files via WinRM.
type WinRMFileModule struct {
	BaseProxyModule
}

// NewWinRMFileModule creates a new WinRM file module.
func NewWinRMFileModule() *WinRMFileModule {
	return &WinRMFileModule{BaseProxyModule{name: "winrm_file"}}
}

// Execute runs the file module.
func (m *WinRMFileModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	path, _ := m.GetString(mctx.Parameters, "path")
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	state, _ := m.GetString(mctx.Parameters, "state")
	if state == "" {
		state = "present"
	}

	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	switch state {
	case "present", "file":
		return m.ensureFile(ctx, mctx, path, result)
	case "absent":
		return m.removeFile(ctx, mctx, path, result)
	case "directory":
		return m.ensureDirectory(ctx, mctx, path, result)
	default:
		return nil, fmt.Errorf("unknown state: %s", state)
	}
}

// Check performs a dry-run check.
func (m *WinRMFileModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

func (m *WinRMFileModule) ensureFile(ctx context.Context, mctx ModuleContext, path string, result *ModuleResult) (*ModuleResult, error) {
	// Check if file exists using PowerShell
	checkResult, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("Test-Path -Path '%s' -PathType Leaf", path))
	if err != nil {
		return nil, err
	}

	exists := strings.TrimSpace(string(checkResult.Stdout)) == "True"

	content, hasContent := m.GetString(mctx.Parameters, "content")
	source, hasSource := m.GetString(mctx.Parameters, "source")

	if !exists {
		result.Changed = true
		result.Comment = fmt.Sprintf("File %s will be created", path)

		if mctx.DryRun {
			return result, nil
		}

		switch {
		case hasContent:
			// Create file with content using PowerShell
			// Escape single quotes in content
			escapedContent := strings.ReplaceAll(content, "'", "''")
			cmd := fmt.Sprintf("Set-Content -Path '%s' -Value '%s'", path, escapedContent)
			if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
				return nil, fmt.Errorf("failed to create file: %w", err)
			}
		case hasSource:
			// Copy from source
			cmd := fmt.Sprintf("Copy-Item -Path '%s' -Destination '%s'", source, path)
			if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
				return nil, fmt.Errorf("failed to copy file: %w", err)
			}
		default:
			// Create empty file
			cmd := fmt.Sprintf("New-Item -Path '%s' -ItemType File -Force", path)
			if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
				return nil, fmt.Errorf("failed to create file: %w", err)
			}
		}

		result.Comment = fmt.Sprintf("File %s created", path)
	} else {
		result.Comment = fmt.Sprintf("File %s already exists", path)

		// Check content if provided
		if hasContent {
			catResult, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("Get-Content -Path '%s' -Raw", path))
			if err == nil && strings.TrimSpace(string(catResult.Stdout)) != strings.TrimSpace(content) {
				result.Changed = true
				result.Comment = fmt.Sprintf("File %s will be updated", path)

				if !mctx.DryRun {
					escapedContent := strings.ReplaceAll(content, "'", "''")
					cmd := fmt.Sprintf("Set-Content -Path '%s' -Value '%s'", path, escapedContent)
					if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
						return nil, fmt.Errorf("failed to update file: %w", err)
					}
					result.Comment = fmt.Sprintf("File %s updated", path)
				}
			}
		}
	}

	return result, nil
}

func (m *WinRMFileModule) removeFile(ctx context.Context, mctx ModuleContext, path string, result *ModuleResult) (*ModuleResult, error) {
	// Check if file/directory exists
	checkResult, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("Test-Path -Path '%s'", path))
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(string(checkResult.Stdout)) == "True" {
		result.Changed = true
		result.Comment = fmt.Sprintf("File %s will be removed", path)

		if !mctx.DryRun {
			cmd := fmt.Sprintf("Remove-Item -Path '%s' -Recurse -Force", path)
			if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
				return nil, fmt.Errorf("failed to remove: %w", err)
			}
			result.Comment = fmt.Sprintf("File %s removed", path)
		}
	} else {
		result.Comment = fmt.Sprintf("File %s already absent", path)
	}

	return result, nil
}

func (m *WinRMFileModule) ensureDirectory(ctx context.Context, mctx ModuleContext, path string, result *ModuleResult) (*ModuleResult, error) {
	// Check if directory exists
	checkResult, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("Test-Path -Path '%s' -PathType Container", path))
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(string(checkResult.Stdout)) != "True" {
		result.Changed = true
		result.Comment = fmt.Sprintf("Directory %s will be created", path)

		if !mctx.DryRun {
			cmd := fmt.Sprintf("New-Item -Path '%s' -ItemType Directory -Force", path)
			if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
				return nil, fmt.Errorf("failed to create directory: %w", err)
			}
			result.Comment = fmt.Sprintf("Directory %s created", path)
		}
	} else {
		result.Comment = fmt.Sprintf("Directory %s already exists", path)
	}

	return result, nil
}

// WinRMServiceModule manages Windows services via WinRM.
type WinRMServiceModule struct {
	BaseProxyModule
}

// NewWinRMServiceModule creates a new WinRM service module.
func NewWinRMServiceModule() *WinRMServiceModule {
	return &WinRMServiceModule{BaseProxyModule{name: "winrm_service"}}
}

// Execute runs the service module.
func (m *WinRMServiceModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	name, _ := m.GetString(mctx.Parameters, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	state, _ := m.GetString(mctx.Parameters, "state")
	startMode, hasStartMode := m.GetString(mctx.Parameters, "start_mode")

	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	// Get current service state
	statusResult, _ := mctx.ExecuteCommand(ctx, fmt.Sprintf("Get-Service -Name '%s' | Select-Object -Property Status,StartType | ConvertTo-Json", name))

	var serviceInfo struct {
		Status    string `json:"Status"`
		StartType string `json:"StartType"`
	}
	json.Unmarshal(statusResult.Stdout, &serviceInfo)

	isRunning := serviceInfo.Status == "Running"

	// Handle state
	switch state {
	case "running", "started":
		if !isRunning {
			result.Changed = true
			if mctx.DryRun {
				result.Comment = fmt.Sprintf("Service %s would be started", name)
			} else {
				_, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("Start-Service -Name '%s'", name))
				if err != nil {
					return nil, fmt.Errorf("failed to start service: %w", err)
				}
				result.Comment = fmt.Sprintf("Service %s started", name)
			}
		} else {
			result.Comment = fmt.Sprintf("Service %s is already running", name)
		}

	case "stopped":
		if isRunning {
			result.Changed = true
			if mctx.DryRun {
				result.Comment = fmt.Sprintf("Service %s would be stopped", name)
			} else {
				_, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("Stop-Service -Name '%s' -Force", name))
				if err != nil {
					return nil, fmt.Errorf("failed to stop service: %w", err)
				}
				result.Comment = fmt.Sprintf("Service %s stopped", name)
			}
		} else {
			result.Comment = fmt.Sprintf("Service %s is already stopped", name)
		}

	case "restarted":
		result.Changed = true
		if mctx.DryRun {
			result.Comment = fmt.Sprintf("Service %s would be restarted", name)
		} else {
			_, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("Restart-Service -Name '%s' -Force", name))
			if err != nil {
				return nil, fmt.Errorf("failed to restart service: %w", err)
			}
			result.Comment = fmt.Sprintf("Service %s restarted", name)
		}
	}

	// Handle start mode (startup type)
	if hasStartMode {
		var desiredStartType string
		switch strings.ToLower(startMode) {
		case "auto", "automatic":
			desiredStartType = "Automatic"
		case "manual":
			desiredStartType = "Manual"
		case "disabled":
			desiredStartType = "Disabled"
		}

		if serviceInfo.StartType != desiredStartType {
			result.Changed = true
			if !mctx.DryRun {
				_, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("Set-Service -Name '%s' -StartupType %s", name, desiredStartType))
				if err != nil {
					return nil, fmt.Errorf("failed to set startup type: %w", err)
				}
			}
		}
	}

	return result, nil
}

// Check performs a dry-run check.
func (m *WinRMServiceModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// WinRMRegistryModule manages Windows registry via WinRM.
type WinRMRegistryModule struct {
	BaseProxyModule
}

// NewWinRMRegistryModule creates a new WinRM registry module.
func NewWinRMRegistryModule() *WinRMRegistryModule {
	return &WinRMRegistryModule{BaseProxyModule{name: "winrm_registry"}}
}

// Execute runs the registry module.
func (m *WinRMRegistryModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	path, _ := m.GetString(mctx.Parameters, "path")
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	name, _ := m.GetString(mctx.Parameters, "name")
	state, _ := m.GetString(mctx.Parameters, "state")
	if state == "" {
		state = "present"
	}

	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	switch state {
	case "present":
		// Check if key/value exists
		var checkCmd string
		if name != "" {
			checkCmd = fmt.Sprintf("(Get-ItemProperty -Path '%s' -Name '%s' -ErrorAction SilentlyContinue) -ne $null", path, name)
		} else {
			checkCmd = fmt.Sprintf("Test-Path -Path '%s'", path)
		}

		checkResult, _ := mctx.ExecuteCommand(ctx, checkCmd)
		exists := strings.TrimSpace(string(checkResult.Stdout)) == "True"

		if name != "" {
			// Set registry value
			value, hasValue := mctx.Parameters["value"]
			valueType, _ := m.GetString(mctx.Parameters, "type")
			if valueType == "" {
				valueType = "String"
			}

			if hasValue {
				if !exists {
					result.Changed = true
				} else {
					// Check current value
					getResult, _ := mctx.ExecuteCommand(ctx, fmt.Sprintf("(Get-ItemProperty -Path '%s' -Name '%s').%s", path, name, name))
					currentValue := strings.TrimSpace(string(getResult.Stdout))
					if currentValue != fmt.Sprintf("%v", value) {
						result.Changed = true
					}
				}

				if result.Changed {
					if mctx.DryRun {
						result.Comment = fmt.Sprintf("Registry value %s\\%s would be set", path, name)
					} else {
						// Ensure key exists
						_, _ = mctx.ExecuteCommand(ctx, fmt.Sprintf("New-Item -Path '%s' -Force | Out-Null", path)) //nolint:errcheck // best-effort create key

						// Set value
						cmd := fmt.Sprintf("Set-ItemProperty -Path '%s' -Name '%s' -Value '%v' -Type %s",
							path, name, value, valueType)
						if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
							return nil, fmt.Errorf("failed to set registry value: %w", err)
						}
						result.Comment = fmt.Sprintf("Registry value %s\\%s set", path, name)
					}
				} else {
					result.Comment = fmt.Sprintf("Registry value %s\\%s already correct", path, name)
				}
			}
		} else {
			// Ensure key exists
			if !exists {
				result.Changed = true
				if mctx.DryRun {
					result.Comment = fmt.Sprintf("Registry key %s would be created", path)
				} else {
					cmd := fmt.Sprintf("New-Item -Path '%s' -Force", path)
					if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
						return nil, fmt.Errorf("failed to create registry key: %w", err)
					}
					result.Comment = fmt.Sprintf("Registry key %s created", path)
				}
			} else {
				result.Comment = fmt.Sprintf("Registry key %s already exists", path)
			}
		}

	case "absent":
		var checkCmd string
		if name != "" {
			checkCmd = fmt.Sprintf("(Get-ItemProperty -Path '%s' -Name '%s' -ErrorAction SilentlyContinue) -ne $null", path, name)
		} else {
			checkCmd = fmt.Sprintf("Test-Path -Path '%s'", path)
		}

		checkResult, _ := mctx.ExecuteCommand(ctx, checkCmd)
		exists := strings.TrimSpace(string(checkResult.Stdout)) == "True"

		if exists {
			result.Changed = true
			if mctx.DryRun {
				if name != "" {
					result.Comment = fmt.Sprintf("Registry value %s\\%s would be removed", path, name)
				} else {
					result.Comment = fmt.Sprintf("Registry key %s would be removed", path)
				}
			} else {
				var cmd string
				if name != "" {
					cmd = fmt.Sprintf("Remove-ItemProperty -Path '%s' -Name '%s' -Force", path, name)
				} else {
					cmd = fmt.Sprintf("Remove-Item -Path '%s' -Recurse -Force", path)
				}
				if _, err := mctx.ExecuteCommand(ctx, cmd); err != nil {
					return nil, fmt.Errorf("failed to remove registry: %w", err)
				}
				if name != "" {
					result.Comment = fmt.Sprintf("Registry value %s\\%s removed", path, name)
				} else {
					result.Comment = fmt.Sprintf("Registry key %s removed", path)
				}
			}
		} else {
			if name != "" {
				result.Comment = fmt.Sprintf("Registry value %s\\%s already absent", path, name)
			} else {
				result.Comment = fmt.Sprintf("Registry key %s already absent", path)
			}
		}
	}

	return result, nil
}

// Check performs a dry-run check.
func (m *WinRMRegistryModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}

// WinRMPackageModule manages Windows packages via WinRM.
type WinRMPackageModule struct {
	BaseProxyModule
}

// NewWinRMPackageModule creates a new WinRM package module.
func NewWinRMPackageModule() *WinRMPackageModule {
	return &WinRMPackageModule{BaseProxyModule{name: "winrm_package"}}
}

// Execute runs the package module.
func (m *WinRMPackageModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	name, _ := m.GetString(mctx.Parameters, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	state, _ := m.GetString(mctx.Parameters, "state")
	if state == "" {
		state = "present"
	}

	provider, _ := m.GetString(mctx.Parameters, "provider")
	if provider == "" {
		provider = "chocolatey" // Default to Chocolatey
	}

	result := &ModuleResult{
		Details: make(map[string]interface{}),
	}

	switch provider {
	case "chocolatey", "choco":
		return m.executeChocolatey(ctx, mctx, name, state, result)
	case "winget":
		return m.executeWinget(ctx, mctx, name, state, result)
	case "msi":
		return m.executeMSI(ctx, mctx, name, state, result)
	default:
		return nil, fmt.Errorf("unknown package provider: %s", provider)
	}
}

func (m *WinRMPackageModule) executeChocolatey(ctx context.Context, mctx ModuleContext, name, state string, result *ModuleResult) (*ModuleResult, error) {
	// Check if package is installed
	checkResult, _ := mctx.ExecuteCommand(ctx, fmt.Sprintf("choco list --local-only %s --exact --limit-output", name))
	isInstalled := strings.TrimSpace(string(checkResult.Stdout)) != ""

	switch state {
	case "present", "installed":
		if !isInstalled {
			result.Changed = true
			if mctx.DryRun {
				result.Comment = fmt.Sprintf("Package %s would be installed via Chocolatey", name)
			} else {
				_, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("choco install %s -y", name))
				if err != nil {
					return nil, fmt.Errorf("failed to install package: %w", err)
				}
				result.Comment = fmt.Sprintf("Package %s installed via Chocolatey", name)
			}
		} else {
			result.Comment = fmt.Sprintf("Package %s is already installed", name)
		}

	case "absent", "removed":
		if isInstalled {
			result.Changed = true
			if mctx.DryRun {
				result.Comment = fmt.Sprintf("Package %s would be uninstalled via Chocolatey", name)
			} else {
				_, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("choco uninstall %s -y", name))
				if err != nil {
					return nil, fmt.Errorf("failed to uninstall package: %w", err)
				}
				result.Comment = fmt.Sprintf("Package %s uninstalled via Chocolatey", name)
			}
		} else {
			result.Comment = fmt.Sprintf("Package %s is already absent", name)
		}

	case "latest":
		result.Changed = true
		if mctx.DryRun {
			result.Comment = fmt.Sprintf("Package %s would be upgraded via Chocolatey", name)
		} else {
			_, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("choco upgrade %s -y", name))
			if err != nil {
				return nil, fmt.Errorf("failed to upgrade package: %w", err)
			}
			result.Comment = fmt.Sprintf("Package %s upgraded via Chocolatey", name)
		}
	}

	return result, nil
}

func (m *WinRMPackageModule) executeWinget(ctx context.Context, mctx ModuleContext, name, state string, result *ModuleResult) (*ModuleResult, error) {
	// Check if package is installed
	checkResult, _ := mctx.ExecuteCommand(ctx, fmt.Sprintf("winget list --id %s --exact", name))
	isInstalled := !strings.Contains(string(checkResult.Stdout), "No installed package")

	switch state {
	case "present", "installed":
		if !isInstalled {
			result.Changed = true
			if mctx.DryRun {
				result.Comment = fmt.Sprintf("Package %s would be installed via winget", name)
			} else {
				_, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("winget install --id %s --exact --silent --accept-package-agreements --accept-source-agreements", name))
				if err != nil {
					return nil, fmt.Errorf("failed to install package: %w", err)
				}
				result.Comment = fmt.Sprintf("Package %s installed via winget", name)
			}
		} else {
			result.Comment = fmt.Sprintf("Package %s is already installed", name)
		}

	case "absent", "removed":
		if isInstalled {
			result.Changed = true
			if mctx.DryRun {
				result.Comment = fmt.Sprintf("Package %s would be uninstalled via winget", name)
			} else {
				_, err := mctx.ExecuteCommand(ctx, fmt.Sprintf("winget uninstall --id %s --exact --silent", name))
				if err != nil {
					return nil, fmt.Errorf("failed to uninstall package: %w", err)
				}
				result.Comment = fmt.Sprintf("Package %s uninstalled via winget", name)
			}
		} else {
			result.Comment = fmt.Sprintf("Package %s is already absent", name)
		}
	}

	return result, nil
}

func (m *WinRMPackageModule) executeMSI(ctx context.Context, mctx ModuleContext, name, state string, result *ModuleResult) (*ModuleResult, error) {
	source, hasSource := m.GetString(mctx.Parameters, "source")
	productID, hasProductID := m.GetString(mctx.Parameters, "product_id")

	switch state {
	case "present", "installed":
		if !hasSource {
			return nil, fmt.Errorf("source is required for MSI installation")
		}

		result.Changed = true
		if mctx.DryRun {
			result.Comment = fmt.Sprintf("MSI %s would be installed", source)
		} else {
			args, _ := m.GetString(mctx.Parameters, "arguments")
			if args == "" {
				args = "/qn /norestart"
			}
			cmd := fmt.Sprintf("Start-Process msiexec.exe -ArgumentList '/i %q %s' -Wait -NoNewWindow", source, args)
			_, err := mctx.ExecuteCommand(ctx, cmd)
			if err != nil {
				return nil, fmt.Errorf("failed to install MSI: %w", err)
			}
			result.Comment = fmt.Sprintf("MSI %s installed", source)
		}

	case "absent", "removed":
		if !hasProductID {
			return nil, fmt.Errorf("product_id is required for MSI uninstallation")
		}

		result.Changed = true
		if mctx.DryRun {
			result.Comment = fmt.Sprintf("MSI %s would be uninstalled", productID)
		} else {
			cmd := fmt.Sprintf("Start-Process msiexec.exe -ArgumentList '/x %s /qn /norestart' -Wait -NoNewWindow", productID)
			_, err := mctx.ExecuteCommand(ctx, cmd)
			if err != nil {
				return nil, fmt.Errorf("failed to uninstall MSI: %w", err)
			}
			result.Comment = fmt.Sprintf("MSI %s uninstalled", productID)
		}
	}

	return result, nil
}

// Check performs a dry-run check.
func (m *WinRMPackageModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	mctx.DryRun = true
	return m.Execute(ctx, mctx)
}
