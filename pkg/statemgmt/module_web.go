package statemgmt

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ============================================================================
// Nginx Site Module
// ============================================================================

// NginxSiteModule manages Nginx site configurations
type NginxSiteModule struct {
	*BaseModule
}

// NewNginxSiteModule creates a new Nginx site module
func NewNginxSiteModule() *NginxSiteModule {
	return &NginxSiteModule{
		BaseModule: NewBaseModule("nginx_site", []string{"enabled", "disabled", "absent"}),
	}
}

// Check verifies the current state of an Nginx site
func (m *NginxSiteModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("nginx_site module is not supported on Windows")
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	// Get Nginx paths
	paths := m.getNginxPaths()

	result := &ModuleCheckResult{
		Present:  false,
		Matches:  false,
		Metadata: make(map[string]interface{}),
	}

	// Check if site exists in sites-available
	availablePath := filepath.Join(paths.sitesAvailable, name)
	enabledPath := filepath.Join(paths.sitesEnabled, name)

	if _, err := os.Stat(availablePath); err == nil {
		result.Present = true
		result.Metadata["available_path"] = availablePath
	}

	// Check if site is enabled (symlink exists in sites-enabled)
	if _, err := os.Lstat(enabledPath); err == nil {
		result.Metadata["enabled"] = true
		result.Metadata["enabled_path"] = enabledPath
	} else {
		result.Metadata["enabled"] = false
	}

	// Determine current state
	switch decl.State {
	case "enabled":
		enabled, _ := result.Metadata["enabled"].(bool)
		result.Matches = result.Present && enabled
		if enabled {
			result.CurrentState = "enabled"
		} else if result.Present {
			result.CurrentState = "disabled"
		} else {
			result.CurrentState = "absent"
		}
	case "disabled":
		enabled, _ := result.Metadata["enabled"].(bool)
		result.Matches = result.Present && !enabled
		if enabled {
			result.CurrentState = "enabled"
		} else if result.Present {
			result.CurrentState = "disabled"
		} else {
			result.CurrentState = "absent"
		}
	case "absent":
		result.Matches = !result.Present
		if result.Present {
			enabled, _ := result.Metadata["enabled"].(bool)
			if enabled {
				result.CurrentState = "enabled"
			} else {
				result.CurrentState = "disabled"
			}
		} else {
			result.CurrentState = "absent"
		}
	}

	return result, nil
}

// Apply makes changes to reach the desired state
func (m *NginxSiteModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("nginx_site module is not supported on Windows")
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	content := getStringParameter(decl, "content", "")
	source := getStringParameter(decl, "source", "")
	reload := getBoolParameter(decl, "reload", true)

	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return nil, err
	}

	if checkResult.Matches {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: fmt.Sprintf("Nginx site %s is already in state %s", name, decl.State),
		}, nil
	}

	paths := m.getNginxPaths()
	availablePath := filepath.Join(paths.sitesAvailable, name)
	enabledPath := filepath.Join(paths.sitesEnabled, name)

	switch decl.State {
	case "enabled":
		// Create/update site configuration if content or source is provided
		if content != "" {
			if err := os.MkdirAll(paths.sitesAvailable, 0755); err != nil {
				return nil, fmt.Errorf("failed to create sites-available directory: %w", err)
			}
			if err := os.WriteFile(availablePath, []byte(content), 0644); err != nil {
				return nil, fmt.Errorf("failed to write site configuration: %w", err)
			}
		} else if source != "" {
			sourceContent, err := os.ReadFile(source)
			if err != nil {
				return nil, fmt.Errorf("failed to read source file: %w", err)
			}
			if err := os.MkdirAll(paths.sitesAvailable, 0755); err != nil {
				return nil, fmt.Errorf("failed to create sites-available directory: %w", err)
			}
			if err := os.WriteFile(availablePath, sourceContent, 0644); err != nil {
				return nil, fmt.Errorf("failed to write site configuration: %w", err)
			}
		}

		// Validate configuration
		if err := m.validateConfig(); err != nil {
			return nil, fmt.Errorf("nginx configuration validation failed: %w", err)
		}

		// Enable site by creating symlink
		if err := os.MkdirAll(paths.sitesEnabled, 0755); err != nil {
			return nil, fmt.Errorf("failed to create sites-enabled directory: %w", err)
		}
		_ = os.Remove(enabledPath) // Remove existing symlink if any
		if err := os.Symlink(availablePath, enabledPath); err != nil {
			return nil, fmt.Errorf("failed to enable site: %w", err)
		}

		// Reload nginx if requested
		if reload {
			if err := m.reloadNginx(); err != nil {
				return nil, fmt.Errorf("failed to reload nginx: %w", err)
			}
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Nginx site %s enabled", name),
		}, nil

	case "disabled":
		// Remove symlink if enabled
		if _, err := os.Lstat(enabledPath); err == nil {
			if err := os.Remove(enabledPath); err != nil {
				return nil, fmt.Errorf("failed to disable site: %w", err)
			}
		}

		// Reload nginx if requested
		if reload {
			if err := m.reloadNginx(); err != nil {
				return nil, fmt.Errorf("failed to reload nginx: %w", err)
			}
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Nginx site %s disabled", name),
		}, nil

	case "absent":
		// Remove symlink if enabled
		if _, err := os.Lstat(enabledPath); err == nil {
			if err := os.Remove(enabledPath); err != nil {
				return nil, fmt.Errorf("failed to remove site symlink: %w", err)
			}
		}

		// Remove site configuration
		if _, err := os.Stat(availablePath); err == nil {
			if err := os.Remove(availablePath); err != nil {
				return nil, fmt.Errorf("failed to remove site configuration: %w", err)
			}
		}

		// Reload nginx if requested
		if reload {
			if err := m.reloadNginx(); err != nil {
				return nil, fmt.Errorf("failed to reload nginx: %w", err)
			}
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Nginx site %s removed", name),
		}, nil
	}

	return nil, fmt.Errorf("unknown state: %s", decl.State)
}

// Test validates the configuration without making changes
func (m *NginxSiteModule) Test(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return nil, err
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: !checkResult.Matches,
		Comment: fmt.Sprintf("Site would be %s (currently %s)", decl.State, checkResult.CurrentState),
	}, nil
}

type nginxPaths struct {
	configDir      string
	sitesAvailable string
	sitesEnabled   string
}

func (m *NginxSiteModule) getNginxPaths() nginxPaths {
	// Check common Nginx locations
	if runtime.GOOS == "darwin" {
		// macOS with Homebrew
		return nginxPaths{
			configDir:      "/usr/local/etc/nginx",
			sitesAvailable: "/usr/local/etc/nginx/sites-available",
			sitesEnabled:   "/usr/local/etc/nginx/sites-enabled",
		}
	}

	// Linux default
	return nginxPaths{
		configDir:      "/etc/nginx",
		sitesAvailable: "/etc/nginx/sites-available",
		sitesEnabled:   "/etc/nginx/sites-enabled",
	}
}

func (m *NginxSiteModule) validateConfig() error {
	cmd := exec.Command("nginx", "-t")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}

func (m *NginxSiteModule) reloadNginx() error {
	cmd := exec.Command("nginx", "-s", "reload")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}

// ============================================================================
// Nginx Config Module (for config snippets)
// ============================================================================

// NginxConfigModule manages Nginx configuration snippets
type NginxConfigModule struct {
	*BaseModule
}

// NewNginxConfigModule creates a new Nginx config module
func NewNginxConfigModule() *NginxConfigModule {
	return &NginxConfigModule{
		BaseModule: NewBaseModule("nginx_config", []string{"present", "absent"}),
	}
}

// Check verifies the current state of an Nginx config snippet
func (m *NginxConfigModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("nginx_config module is not supported on Windows")
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	dest := getStringParameter(decl, "dest", "")
	if dest == "" {
		// Default to conf.d directory
		if runtime.GOOS == "darwin" {
			dest = "/usr/local/etc/nginx/conf.d"
		} else {
			dest = "/etc/nginx/conf.d"
		}
	}

	configPath := filepath.Join(dest, name)

	result := &ModuleCheckResult{
		Present:  false,
		Matches:  false,
		Metadata: make(map[string]interface{}),
	}

	if _, err := os.Stat(configPath); err == nil {
		result.Present = true
		result.Metadata["path"] = configPath
		result.CurrentState = "present"
	} else {
		result.CurrentState = "absent"
	}

	switch decl.State {
	case "present":
		result.Matches = result.Present
		// TODO: content comparison for idempotency
	case "absent":
		result.Matches = !result.Present
	}

	return result, nil
}

// Apply makes changes to reach the desired state
func (m *NginxConfigModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("nginx_config module is not supported on Windows")
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	content := getStringParameter(decl, "content", "")
	source := getStringParameter(decl, "source", "")
	dest := getStringParameter(decl, "dest", "")
	reload := getBoolParameter(decl, "reload", true)

	if dest == "" {
		if runtime.GOOS == "darwin" {
			dest = "/usr/local/etc/nginx/conf.d"
		} else {
			dest = "/etc/nginx/conf.d"
		}
	}

	configPath := filepath.Join(dest, name)

	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return nil, err
	}

	if checkResult.Matches {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: fmt.Sprintf("Nginx config %s is already in state %s", name, decl.State),
		}, nil
	}

	switch decl.State {
	case "present":
		if content == "" && source == "" {
			return nil, fmt.Errorf("either content or source parameter is required for present state")
		}

		var configContent []byte
		if content != "" {
			configContent = []byte(content)
		} else {
			var err error
			configContent, err = os.ReadFile(source)
			if err != nil {
				return nil, fmt.Errorf("failed to read source file: %w", err)
			}
		}

		// Create directory if needed
		if err := os.MkdirAll(dest, 0755); err != nil {
			return nil, fmt.Errorf("failed to create config directory: %w", err)
		}

		// Write config file
		if err := os.WriteFile(configPath, configContent, 0644); err != nil {
			return nil, fmt.Errorf("failed to write config file: %w", err)
		}

		// Validate configuration
		cmd := exec.Command("nginx", "-t")
		if output, err := cmd.CombinedOutput(); err != nil {
			// Rollback: remove the config file
			_ = os.Remove(configPath)
			return nil, fmt.Errorf("nginx configuration validation failed: %s", string(output))
		}

		// Reload nginx if requested
		if reload {
			cmd := exec.Command("nginx", "-s", "reload")
			if output, err := cmd.CombinedOutput(); err != nil {
				return nil, fmt.Errorf("failed to reload nginx: %s", string(output))
			}
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Nginx config %s created", name),
		}, nil

	case "absent":
		if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to remove config file: %w", err)
		}

		// Reload nginx if requested
		if reload {
			cmd := exec.Command("nginx", "-s", "reload")
			if output, err := cmd.CombinedOutput(); err != nil {
				return nil, fmt.Errorf("failed to reload nginx: %s", string(output))
			}
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Nginx config %s removed", name),
		}, nil
	}

	return nil, fmt.Errorf("unknown state: %s", decl.State)
}

// Test validates the configuration without making changes
func (m *NginxConfigModule) Test(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return nil, err
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: !checkResult.Matches,
		Comment: fmt.Sprintf("Config would be %s (currently %s)", decl.State, checkResult.CurrentState),
	}, nil
}

// ============================================================================
// Nginx Upstream Module
// ============================================================================

// NginxUpstreamModule manages Nginx upstream configurations for load balancing
type NginxUpstreamModule struct {
	*BaseModule
	upstreamDir string // For testing - overrides default path
}

// NewNginxUpstreamModule creates a new Nginx upstream module
func NewNginxUpstreamModule() *NginxUpstreamModule {
	return &NginxUpstreamModule{
		BaseModule: NewBaseModule("nginx_upstream", []string{"present", "absent"}),
	}
}

// Check verifies the current state of an Nginx upstream configuration
func (m *NginxUpstreamModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("nginx_upstream module is not supported on Windows")
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	configPath := m.getUpstreamPath(name)

	result := &ModuleCheckResult{
		Present:  false,
		Matches:  false,
		Metadata: make(map[string]interface{}),
	}

	if _, err := os.Stat(configPath); err == nil {
		result.Present = true
		result.Metadata["path"] = configPath
		result.CurrentState = "present"
	} else {
		result.CurrentState = "absent"
	}

	switch decl.State {
	case "present":
		result.Matches = result.Present
	case "absent":
		result.Matches = !result.Present
	}

	return result, nil
}

// Apply makes changes to reach the desired state
func (m *NginxUpstreamModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("nginx_upstream module is not supported on Windows")
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	reload := getBoolParameter(decl, "reload", true)

	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return nil, err
	}

	if checkResult.Matches {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: fmt.Sprintf("Nginx upstream %s is already in state %s", name, decl.State),
		}, nil
	}

	configPath := m.getUpstreamPath(name)

	switch decl.State {
	case "present":
		servers := getStringSliceParameter(decl, "servers")
		if len(servers) == 0 {
			return nil, fmt.Errorf("servers parameter is required for present state")
		}

		// Build upstream configuration
		config := m.buildUpstreamConfig(decl, name, servers)

		// Create directory if needed
		dir := filepath.Dir(configPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create config directory: %w", err)
		}

		// Write config file
		if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
			return nil, fmt.Errorf("failed to write upstream config: %w", err)
		}

		// Validate configuration
		if err := m.validateConfig(); err != nil {
			_ = os.Remove(configPath)
			return nil, fmt.Errorf("nginx configuration validation failed: %w", err)
		}

		// Reload nginx if requested
		if reload {
			if err := m.reloadNginx(); err != nil {
				return nil, fmt.Errorf("failed to reload nginx: %w", err)
			}
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Nginx upstream %s created with %d servers", name, len(servers)),
		}, nil

	case "absent":
		if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to remove upstream config: %w", err)
		}

		if reload {
			if err := m.reloadNginx(); err != nil {
				return nil, fmt.Errorf("failed to reload nginx: %w", err)
			}
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Nginx upstream %s removed", name),
		}, nil
	}

	return nil, fmt.Errorf("unknown state: %s", decl.State)
}

// Test validates the configuration without making changes
func (m *NginxUpstreamModule) Test(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return nil, err
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: !checkResult.Matches,
		Comment: fmt.Sprintf("Upstream would be %s (currently %s)", decl.State, checkResult.CurrentState),
	}, nil
}

func (m *NginxUpstreamModule) getUpstreamPath(name string) string {
	var dir string
	if m.upstreamDir != "" {
		dir = m.upstreamDir
	} else if runtime.GOOS == "darwin" {
		dir = "/usr/local/etc/nginx/conf.d"
	} else {
		dir = "/etc/nginx/conf.d"
	}
	return filepath.Join(dir, "upstream-"+name+".conf")
}

func (m *NginxUpstreamModule) buildUpstreamConfig(decl *StateDeclaration, name string, servers []string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("upstream %s {\n", name))

	// Load balancing method
	method := getStringParameter(decl, "method", "round_robin")
	switch method {
	case "least_conn":
		sb.WriteString("    least_conn;\n")
	case "ip_hash":
		sb.WriteString("    ip_hash;\n")
	case "hash":
		hashKey := getStringParameter(decl, "hash_key", "$request_uri")
		consistent := getBoolParameter(decl, "hash_consistent", false)
		if consistent {
			sb.WriteString(fmt.Sprintf("    hash %s consistent;\n", hashKey))
		} else {
			sb.WriteString(fmt.Sprintf("    hash %s;\n", hashKey))
		}
	case "random":
		sb.WriteString("    random;\n")
	// round_robin is default, no directive needed
	}

	// Keepalive connections
	keepalive := getIntParameter(decl, "keepalive", 0)
	if keepalive > 0 {
		sb.WriteString(fmt.Sprintf("    keepalive %d;\n", keepalive))
	}

	// Add servers
	for _, server := range servers {
		serverLine := fmt.Sprintf("    server %s", server)

		// Check for weight in server string (e.g., "192.168.1.1:8080 weight=5")
		if !strings.Contains(server, "weight=") {
			weight := getIntParameter(decl, "weight", 0)
			if weight > 0 {
				serverLine += fmt.Sprintf(" weight=%d", weight)
			}
		}

		// Check for max_fails
		if !strings.Contains(server, "max_fails=") {
			maxFails := getIntParameter(decl, "max_fails", 0)
			if maxFails > 0 {
				serverLine += fmt.Sprintf(" max_fails=%d", maxFails)
			}
		}

		// Check for fail_timeout
		if !strings.Contains(server, "fail_timeout=") {
			failTimeout := getStringParameter(decl, "fail_timeout", "")
			if failTimeout != "" {
				serverLine += fmt.Sprintf(" fail_timeout=%s", failTimeout)
			}
		}

		sb.WriteString(serverLine + ";\n")
	}

	sb.WriteString("}\n")

	return sb.String()
}

func (m *NginxUpstreamModule) validateConfig() error {
	cmd := exec.Command("nginx", "-t")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}

func (m *NginxUpstreamModule) reloadNginx() error {
	cmd := exec.Command("nginx", "-s", "reload")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}

// ============================================================================
// Nginx Proxy Module
// ============================================================================

// NginxProxyModule manages Nginx reverse proxy configurations
type NginxProxyModule struct {
	*BaseModule
}

// NewNginxProxyModule creates a new Nginx proxy module
func NewNginxProxyModule() *NginxProxyModule {
	return &NginxProxyModule{
		BaseModule: NewBaseModule("nginx_proxy", []string{"present", "absent"}),
	}
}

// Check verifies the current state of an Nginx proxy configuration
func (m *NginxProxyModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("nginx_proxy module is not supported on Windows")
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	configPath := m.getProxyPath(name)

	result := &ModuleCheckResult{
		Present:  false,
		Matches:  false,
		Metadata: make(map[string]interface{}),
	}

	if _, err := os.Stat(configPath); err == nil {
		result.Present = true
		result.Metadata["path"] = configPath
		result.CurrentState = "present"
	} else {
		result.CurrentState = "absent"
	}

	switch decl.State {
	case "present":
		result.Matches = result.Present
	case "absent":
		result.Matches = !result.Present
	}

	return result, nil
}

// Apply makes changes to reach the desired state
func (m *NginxProxyModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("nginx_proxy module is not supported on Windows")
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	reload := getBoolParameter(decl, "reload", true)

	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return nil, err
	}

	if checkResult.Matches {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: fmt.Sprintf("Nginx proxy %s is already in state %s", name, decl.State),
		}, nil
	}

	configPath := m.getProxyPath(name)

	switch decl.State {
	case "present":
		backend := getStringParameter(decl, "backend", "")
		if backend == "" {
			return nil, fmt.Errorf("backend parameter is required for present state")
		}

		// Build proxy configuration
		config := m.buildProxyConfig(decl, name, backend)

		// Create directory if needed
		dir := filepath.Dir(configPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create config directory: %w", err)
		}

		// Write config file
		if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
			return nil, fmt.Errorf("failed to write proxy config: %w", err)
		}

		// Validate configuration
		if err := m.validateConfig(); err != nil {
			_ = os.Remove(configPath)
			return nil, fmt.Errorf("nginx configuration validation failed: %w", err)
		}

		// Reload nginx if requested
		if reload {
			if err := m.reloadNginx(); err != nil {
				return nil, fmt.Errorf("failed to reload nginx: %w", err)
			}
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Nginx proxy %s created for backend %s", name, backend),
		}, nil

	case "absent":
		if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to remove proxy config: %w", err)
		}

		if reload {
			if err := m.reloadNginx(); err != nil {
				return nil, fmt.Errorf("failed to reload nginx: %w", err)
			}
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Nginx proxy %s removed", name),
		}, nil
	}

	return nil, fmt.Errorf("unknown state: %s", decl.State)
}

// Test validates the configuration without making changes
func (m *NginxProxyModule) Test(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return nil, err
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: !checkResult.Matches,
		Comment: fmt.Sprintf("Proxy would be %s (currently %s)", decl.State, checkResult.CurrentState),
	}, nil
}

func (m *NginxProxyModule) getProxyPath(name string) string {
	if runtime.GOOS == "darwin" {
		return filepath.Join("/usr/local/etc/nginx/conf.d", "proxy_"+name+".conf")
	}
	return filepath.Join("/etc/nginx/conf.d", "proxy_"+name+".conf")
}

func (m *NginxProxyModule) buildProxyConfig(decl *StateDeclaration, name, backend string) string {
	var sb strings.Builder

	listen := getStringParameter(decl, "listen", "80")
	serverName := getStringParameter(decl, "server_name", "_")
	location := getStringParameter(decl, "location", "/")

	sb.WriteString(fmt.Sprintf("# Reverse proxy configuration for %s\n", name))
	sb.WriteString(fmt.Sprintf("server {\n"))
	sb.WriteString(fmt.Sprintf("    listen %s;\n", listen))
	sb.WriteString(fmt.Sprintf("    server_name %s;\n\n", serverName))

	sb.WriteString(fmt.Sprintf("    location %s {\n", location))
	sb.WriteString(fmt.Sprintf("        proxy_pass %s;\n", backend))

	// Proxy headers
	setHeaders := getBoolParameter(decl, "set_headers", true)
	if setHeaders {
		sb.WriteString("        proxy_set_header Host $host;\n")
		sb.WriteString("        proxy_set_header X-Real-IP $remote_addr;\n")
		sb.WriteString("        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n")
		sb.WriteString("        proxy_set_header X-Forwarded-Proto $scheme;\n")
	}

	// Custom headers
	headers := getStringSliceParameter(decl, "headers")
	for _, header := range headers {
		sb.WriteString(fmt.Sprintf("        proxy_set_header %s;\n", header))
	}

	// Timeouts
	connectTimeout := getStringParameter(decl, "connect_timeout", "")
	if connectTimeout != "" {
		sb.WriteString(fmt.Sprintf("        proxy_connect_timeout %s;\n", connectTimeout))
	}
	sendTimeout := getStringParameter(decl, "send_timeout", "")
	if sendTimeout != "" {
		sb.WriteString(fmt.Sprintf("        proxy_send_timeout %s;\n", sendTimeout))
	}
	readTimeout := getStringParameter(decl, "read_timeout", "")
	if readTimeout != "" {
		sb.WriteString(fmt.Sprintf("        proxy_read_timeout %s;\n", readTimeout))
	}

	// Buffering
	buffering := getStringParameter(decl, "buffering", "")
	if buffering != "" {
		sb.WriteString(fmt.Sprintf("        proxy_buffering %s;\n", buffering))
	}
	bufferSize := getStringParameter(decl, "buffer_size", "")
	if bufferSize != "" {
		sb.WriteString(fmt.Sprintf("        proxy_buffer_size %s;\n", bufferSize))
	}

	// WebSocket support
	websocket := getBoolParameter(decl, "websocket", false)
	if websocket {
		sb.WriteString("        proxy_http_version 1.1;\n")
		sb.WriteString("        proxy_set_header Upgrade $http_upgrade;\n")
		sb.WriteString("        proxy_set_header Connection \"upgrade\";\n")
	}

	sb.WriteString("    }\n")
	sb.WriteString("}\n")

	return sb.String()
}

func (m *NginxProxyModule) validateConfig() error {
	cmd := exec.Command("nginx", "-t")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}

func (m *NginxProxyModule) reloadNginx() error {
	cmd := exec.Command("nginx", "-s", "reload")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}

// ============================================================================
// Nginx SSL Module
// ============================================================================

// NginxSSLModule manages Nginx SSL/TLS configurations
type NginxSSLModule struct {
	*BaseModule
}

// NewNginxSSLModule creates a new Nginx SSL module
func NewNginxSSLModule() *NginxSSLModule {
	return &NginxSSLModule{
		BaseModule: NewBaseModule("nginx_ssl", []string{"present", "absent"}),
	}
}

// Check verifies the current state of an Nginx SSL configuration
func (m *NginxSSLModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("nginx_ssl module is not supported on Windows")
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	configPath := m.getSSLPath(name)

	result := &ModuleCheckResult{
		Present:  false,
		Matches:  false,
		Metadata: make(map[string]interface{}),
	}

	if _, err := os.Stat(configPath); err == nil {
		result.Present = true
		result.Metadata["path"] = configPath
		result.CurrentState = "present"
	} else {
		result.CurrentState = "absent"
	}

	switch decl.State {
	case "present":
		result.Matches = result.Present
	case "absent":
		result.Matches = !result.Present
	}

	return result, nil
}

// Apply makes changes to reach the desired state
func (m *NginxSSLModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("nginx_ssl module is not supported on Windows")
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	reload := getBoolParameter(decl, "reload", true)

	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return nil, err
	}

	if checkResult.Matches {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: fmt.Sprintf("Nginx SSL config %s is already in state %s", name, decl.State),
		}, nil
	}

	configPath := m.getSSLPath(name)

	switch decl.State {
	case "present":
		certificate := getStringParameter(decl, "certificate", "")
		certificateKey := getStringParameter(decl, "certificate_key", "")

		if certificate == "" || certificateKey == "" {
			return nil, fmt.Errorf("certificate and certificate_key parameters are required for present state")
		}

		// Verify certificate files exist
		if _, err := os.Stat(certificate); err != nil {
			return nil, fmt.Errorf("certificate file does not exist: %s", certificate)
		}
		if _, err := os.Stat(certificateKey); err != nil {
			return nil, fmt.Errorf("certificate key file does not exist: %s", certificateKey)
		}

		// Build SSL configuration
		config := m.buildSSLConfig(decl, name, certificate, certificateKey)

		// Create directory if needed
		dir := filepath.Dir(configPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create config directory: %w", err)
		}

		// Write config file
		if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
			return nil, fmt.Errorf("failed to write SSL config: %w", err)
		}

		// Validate configuration
		if err := m.validateConfig(); err != nil {
			_ = os.Remove(configPath)
			return nil, fmt.Errorf("nginx configuration validation failed: %w", err)
		}

		// Reload nginx if requested
		if reload {
			if err := m.reloadNginx(); err != nil {
				return nil, fmt.Errorf("failed to reload nginx: %w", err)
			}
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Nginx SSL config %s created", name),
		}, nil

	case "absent":
		if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to remove SSL config: %w", err)
		}

		if reload {
			if err := m.reloadNginx(); err != nil {
				return nil, fmt.Errorf("failed to reload nginx: %w", err)
			}
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Nginx SSL config %s removed", name),
		}, nil
	}

	return nil, fmt.Errorf("unknown state: %s", decl.State)
}

// Test validates the configuration without making changes
func (m *NginxSSLModule) Test(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return nil, err
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: !checkResult.Matches,
		Comment: fmt.Sprintf("SSL config would be %s (currently %s)", decl.State, checkResult.CurrentState),
	}, nil
}

func (m *NginxSSLModule) getSSLPath(name string) string {
	if runtime.GOOS == "darwin" {
		return filepath.Join("/usr/local/etc/nginx/conf.d", "ssl_"+name+".conf")
	}
	return filepath.Join("/etc/nginx/conf.d", "ssl_"+name+".conf")
}

func (m *NginxSSLModule) buildSSLConfig(decl *StateDeclaration, name, certificate, certificateKey string) string {
	var sb strings.Builder

	listen := getStringParameter(decl, "listen", "443 ssl")
	serverName := getStringParameter(decl, "server_name", "_")

	sb.WriteString(fmt.Sprintf("# SSL configuration for %s\n", name))
	sb.WriteString("server {\n")
	sb.WriteString(fmt.Sprintf("    listen %s;\n", listen))
	sb.WriteString(fmt.Sprintf("    server_name %s;\n\n", serverName))

	// SSL certificates
	sb.WriteString(fmt.Sprintf("    ssl_certificate %s;\n", certificate))
	sb.WriteString(fmt.Sprintf("    ssl_certificate_key %s;\n\n", certificateKey))

	// SSL protocols
	protocols := getStringParameter(decl, "protocols", "TLSv1.2 TLSv1.3")
	sb.WriteString(fmt.Sprintf("    ssl_protocols %s;\n", protocols))

	// Cipher suites
	ciphers := getStringParameter(decl, "ciphers", "")
	if ciphers != "" {
		sb.WriteString(fmt.Sprintf("    ssl_ciphers %s;\n", ciphers))
		sb.WriteString("    ssl_prefer_server_ciphers on;\n")
	}

	// Session settings
	sessionTimeout := getStringParameter(decl, "session_timeout", "1d")
	sb.WriteString(fmt.Sprintf("    ssl_session_timeout %s;\n", sessionTimeout))

	sessionCache := getStringParameter(decl, "session_cache", "shared:SSL:10m")
	sb.WriteString(fmt.Sprintf("    ssl_session_cache %s;\n", sessionCache))

	sessionTickets := getBoolParameter(decl, "session_tickets", false)
	if !sessionTickets {
		sb.WriteString("    ssl_session_tickets off;\n")
	}

	// OCSP stapling
	ocspStapling := getBoolParameter(decl, "ocsp_stapling", false)
	if ocspStapling {
		sb.WriteString("\n    ssl_stapling on;\n")
		sb.WriteString("    ssl_stapling_verify on;\n")
		trustedCert := getStringParameter(decl, "trusted_certificate", "")
		if trustedCert != "" {
			sb.WriteString(fmt.Sprintf("    ssl_trusted_certificate %s;\n", trustedCert))
		}
	}

	// HSTS
	hsts := getBoolParameter(decl, "hsts", false)
	if hsts {
		hstsMaxAge := getIntParameter(decl, "hsts_max_age", 31536000)
		hstsSubdomains := getBoolParameter(decl, "hsts_include_subdomains", true)
		hstsPreload := getBoolParameter(decl, "hsts_preload", false)
		hstsValue := fmt.Sprintf("max-age=%d", hstsMaxAge)
		if hstsSubdomains {
			hstsValue += "; includeSubDomains"
		}
		if hstsPreload {
			hstsValue += "; preload"
		}
		sb.WriteString(fmt.Sprintf("\n    add_header Strict-Transport-Security \"%s\" always;\n", hstsValue))
	}

	// Root and location
	root := getStringParameter(decl, "root", "")
	if root != "" {
		sb.WriteString(fmt.Sprintf("\n    root %s;\n", root))
	}

	sb.WriteString("\n    location / {\n")
	sb.WriteString("        try_files $uri $uri/ =404;\n")
	sb.WriteString("    }\n")

	sb.WriteString("}\n")

	return sb.String()
}

func (m *NginxSSLModule) validateConfig() error {
	cmd := exec.Command("nginx", "-t")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}

func (m *NginxSSLModule) reloadNginx() error {
	cmd := exec.Command("nginx", "-s", "reload")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}

// ============================================================================
// Nginx Location Module
// ============================================================================

// NginxLocationModule manages Nginx location block configurations
type NginxLocationModule struct {
	*BaseModule
}

// NewNginxLocationModule creates a new Nginx location module
func NewNginxLocationModule() *NginxLocationModule {
	return &NginxLocationModule{
		BaseModule: NewBaseModule("nginx_location", []string{"present", "absent"}),
	}
}

// Check verifies the current state of an Nginx location configuration
func (m *NginxLocationModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("nginx_location module is not supported on Windows")
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	configPath := m.getLocationPath(name)

	result := &ModuleCheckResult{
		Present:  false,
		Matches:  false,
		Metadata: make(map[string]interface{}),
	}

	if _, err := os.Stat(configPath); err == nil {
		result.Present = true
		result.Metadata["path"] = configPath
		result.CurrentState = "present"
	} else {
		result.CurrentState = "absent"
	}

	switch decl.State {
	case "present":
		result.Matches = result.Present
	case "absent":
		result.Matches = !result.Present
	}

	return result, nil
}

// Apply makes changes to reach the desired state
func (m *NginxLocationModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("nginx_location module is not supported on Windows")
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	reload := getBoolParameter(decl, "reload", true)

	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return nil, err
	}

	if checkResult.Matches {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: fmt.Sprintf("Nginx location %s is already in state %s", name, decl.State),
		}, nil
	}

	configPath := m.getLocationPath(name)

	switch decl.State {
	case "present":
		path := getStringParameter(decl, "path", "")
		if path == "" {
			return nil, fmt.Errorf("path parameter is required for present state")
		}

		// Build location configuration
		config := m.buildLocationConfig(decl, name, path)

		// Create directory if needed
		dir := filepath.Dir(configPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create config directory: %w", err)
		}

		// Write config file
		if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
			return nil, fmt.Errorf("failed to write location config: %w", err)
		}

		// Validate configuration
		if err := m.validateConfig(); err != nil {
			_ = os.Remove(configPath)
			return nil, fmt.Errorf("nginx configuration validation failed: %w", err)
		}

		// Reload nginx if requested
		if reload {
			if err := m.reloadNginx(); err != nil {
				return nil, fmt.Errorf("failed to reload nginx: %w", err)
			}
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Nginx location %s created for path %s", name, path),
		}, nil

	case "absent":
		if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to remove location config: %w", err)
		}

		if reload {
			if err := m.reloadNginx(); err != nil {
				return nil, fmt.Errorf("failed to reload nginx: %w", err)
			}
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Nginx location %s removed", name),
		}, nil
	}

	return nil, fmt.Errorf("unknown state: %s", decl.State)
}

// Test validates the configuration without making changes
func (m *NginxLocationModule) Test(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return nil, err
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: !checkResult.Matches,
		Comment: fmt.Sprintf("Location would be %s (currently %s)", decl.State, checkResult.CurrentState),
	}, nil
}

func (m *NginxLocationModule) getLocationPath(name string) string {
	if runtime.GOOS == "darwin" {
		return filepath.Join("/usr/local/etc/nginx/conf.d", "location_"+name+".conf")
	}
	return filepath.Join("/etc/nginx/conf.d", "location_"+name+".conf")
}

func (m *NginxLocationModule) buildLocationConfig(decl *StateDeclaration, name, path string) string {
	var sb strings.Builder

	// Location modifier
	modifier := getStringParameter(decl, "modifier", "")
	locationDirective := ""
	if modifier != "" {
		locationDirective = fmt.Sprintf("location %s %s", modifier, path)
	} else {
		locationDirective = fmt.Sprintf("location %s", path)
	}

	sb.WriteString(fmt.Sprintf("# Location snippet for %s\n", name))
	sb.WriteString(fmt.Sprintf("# Include in server block: include conf.d/location_%s.conf;\n", name))
	sb.WriteString(fmt.Sprintf("%s {\n", locationDirective))

	// Root or alias
	root := getStringParameter(decl, "root", "")
	if root != "" {
		sb.WriteString(fmt.Sprintf("    root %s;\n", root))
	}
	alias := getStringParameter(decl, "alias", "")
	if alias != "" {
		sb.WriteString(fmt.Sprintf("    alias %s;\n", alias))
	}

	// Try files
	tryFiles := getStringParameter(decl, "try_files", "")
	if tryFiles != "" {
		sb.WriteString(fmt.Sprintf("    try_files %s;\n", tryFiles))
	}

	// Index
	index := getStringParameter(decl, "index", "")
	if index != "" {
		sb.WriteString(fmt.Sprintf("    index %s;\n", index))
	}

	// Autoindex
	autoindex := getBoolParameter(decl, "autoindex", false)
	if autoindex {
		sb.WriteString("    autoindex on;\n")
	}

	// Proxy pass
	proxyPass := getStringParameter(decl, "proxy_pass", "")
	if proxyPass != "" {
		sb.WriteString(fmt.Sprintf("    proxy_pass %s;\n", proxyPass))
	}

	// FastCGI
	fastcgiPass := getStringParameter(decl, "fastcgi_pass", "")
	if fastcgiPass != "" {
		sb.WriteString(fmt.Sprintf("    fastcgi_pass %s;\n", fastcgiPass))
		sb.WriteString("    include fastcgi_params;\n")
		fastcgiParam := getStringParameter(decl, "fastcgi_script", "")
		if fastcgiParam != "" {
			sb.WriteString(fmt.Sprintf("    fastcgi_param SCRIPT_FILENAME %s;\n", fastcgiParam))
		}
	}

	// Return
	returnCode := getIntParameter(decl, "return_code", 0)
	if returnCode > 0 {
		returnURL := getStringParameter(decl, "return_url", "")
		if returnURL != "" {
			sb.WriteString(fmt.Sprintf("    return %d %s;\n", returnCode, returnURL))
		} else {
			sb.WriteString(fmt.Sprintf("    return %d;\n", returnCode))
		}
	}

	// Rewrite
	rewrite := getStringParameter(decl, "rewrite", "")
	if rewrite != "" {
		sb.WriteString(fmt.Sprintf("    rewrite %s;\n", rewrite))
	}

	// Access control
	allow := getStringSliceParameter(decl, "allow")
	for _, a := range allow {
		sb.WriteString(fmt.Sprintf("    allow %s;\n", a))
	}
	deny := getStringSliceParameter(decl, "deny")
	for _, d := range deny {
		sb.WriteString(fmt.Sprintf("    deny %s;\n", d))
	}

	// Custom directives
	directives := getStringSliceParameter(decl, "directives")
	for _, d := range directives {
		sb.WriteString(fmt.Sprintf("    %s;\n", d))
	}

	sb.WriteString("}\n")

	return sb.String()
}

func (m *NginxLocationModule) validateConfig() error {
	cmd := exec.Command("nginx", "-t")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}

func (m *NginxLocationModule) reloadNginx() error {
	cmd := exec.Command("nginx", "-s", "reload")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}

// ============================================================================
// Nginx Rate Limit Module
// ============================================================================

// NginxRateLimitModule manages Nginx rate limiting configurations
type NginxRateLimitModule struct {
	*BaseModule
}

// NewNginxRateLimitModule creates a new Nginx rate limit module
func NewNginxRateLimitModule() *NginxRateLimitModule {
	return &NginxRateLimitModule{
		BaseModule: NewBaseModule("nginx_rate_limit", []string{"present", "absent"}),
	}
}

// Check verifies the current state of an Nginx rate limit configuration
func (m *NginxRateLimitModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("nginx_rate_limit module is not supported on Windows")
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	configPath := m.getRateLimitPath(name)

	result := &ModuleCheckResult{
		Present:  false,
		Matches:  false,
		Metadata: make(map[string]interface{}),
	}

	if _, err := os.Stat(configPath); err == nil {
		result.Present = true
		result.Metadata["path"] = configPath
		result.CurrentState = "present"
	} else {
		result.CurrentState = "absent"
	}

	switch decl.State {
	case "present":
		result.Matches = result.Present
	case "absent":
		result.Matches = !result.Present
	}

	return result, nil
}

// Apply makes changes to reach the desired state
func (m *NginxRateLimitModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("nginx_rate_limit module is not supported on Windows")
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	reload := getBoolParameter(decl, "reload", true)

	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return nil, err
	}

	if checkResult.Matches {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: fmt.Sprintf("Nginx rate limit %s is already in state %s", name, decl.State),
		}, nil
	}

	configPath := m.getRateLimitPath(name)

	switch decl.State {
	case "present":
		zone := getStringParameter(decl, "zone", "")
		if zone == "" {
			zone = name
		}
		rate := getStringParameter(decl, "rate", "")
		if rate == "" {
			return nil, fmt.Errorf("rate parameter is required for present state")
		}

		// Build rate limit configuration
		config := m.buildRateLimitConfig(decl, name, zone, rate)

		// Create directory if needed
		dir := filepath.Dir(configPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create config directory: %w", err)
		}

		// Write config file
		if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
			return nil, fmt.Errorf("failed to write rate limit config: %w", err)
		}

		// Validate configuration
		if err := m.validateConfig(); err != nil {
			_ = os.Remove(configPath)
			return nil, fmt.Errorf("nginx configuration validation failed: %w", err)
		}

		// Reload nginx if requested
		if reload {
			if err := m.reloadNginx(); err != nil {
				return nil, fmt.Errorf("failed to reload nginx: %w", err)
			}
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Nginx rate limit zone %s created with rate %s", zone, rate),
		}, nil

	case "absent":
		if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to remove rate limit config: %w", err)
		}

		if reload {
			if err := m.reloadNginx(); err != nil {
				return nil, fmt.Errorf("failed to reload nginx: %w", err)
			}
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Nginx rate limit %s removed", name),
		}, nil
	}

	return nil, fmt.Errorf("unknown state: %s", decl.State)
}

// Test validates the configuration without making changes
func (m *NginxRateLimitModule) Test(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return nil, err
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: !checkResult.Matches,
		Comment: fmt.Sprintf("Rate limit would be %s (currently %s)", decl.State, checkResult.CurrentState),
	}, nil
}

func (m *NginxRateLimitModule) getRateLimitPath(name string) string {
	if runtime.GOOS == "darwin" {
		return filepath.Join("/usr/local/etc/nginx/conf.d", "ratelimit_"+name+".conf")
	}
	return filepath.Join("/etc/nginx/conf.d", "ratelimit_"+name+".conf")
}

func (m *NginxRateLimitModule) buildRateLimitConfig(decl *StateDeclaration, name, zone, rate string) string {
	var sb strings.Builder

	key := getStringParameter(decl, "key", "$binary_remote_addr")
	size := getStringParameter(decl, "size", "10m")

	sb.WriteString(fmt.Sprintf("# Rate limit configuration for %s\n", name))
	sb.WriteString(fmt.Sprintf("limit_req_zone %s zone=%s:%s rate=%s;\n\n", key, zone, size, rate))

	// Generate location snippet for applying rate limit
	sb.WriteString(fmt.Sprintf("# Apply in location block:\n"))
	sb.WriteString(fmt.Sprintf("# limit_req zone=%s", zone))

	burst := getIntParameter(decl, "burst", 0)
	if burst > 0 {
		sb.WriteString(fmt.Sprintf(" burst=%d", burst))
	}

	nodelay := getBoolParameter(decl, "nodelay", false)
	if nodelay {
		sb.WriteString(" nodelay")
	}

	delay := getIntParameter(decl, "delay", 0)
	if delay > 0 && !nodelay {
		sb.WriteString(fmt.Sprintf(" delay=%d", delay))
	}

	sb.WriteString(";\n\n")

	// Error handling
	statusCode := getIntParameter(decl, "status_code", 429)
	if statusCode != 429 {
		sb.WriteString(fmt.Sprintf("limit_req_status %d;\n", statusCode))
	}

	logLevel := getStringParameter(decl, "log_level", "")
	if logLevel != "" {
		sb.WriteString(fmt.Sprintf("limit_req_log_level %s;\n", logLevel))
	}

	// Connection limiting (optional)
	connLimit := getIntParameter(decl, "conn_limit", 0)
	if connLimit > 0 {
		connZone := getStringParameter(decl, "conn_zone", zone+"_conn")
		connSize := getStringParameter(decl, "conn_size", "10m")
		sb.WriteString(fmt.Sprintf("\nlimit_conn_zone %s zone=%s:%s;\n", key, connZone, connSize))
		sb.WriteString(fmt.Sprintf("# Apply in location block: limit_conn %s %d;\n", connZone, connLimit))
	}

	return sb.String()
}

func (m *NginxRateLimitModule) validateConfig() error {
	cmd := exec.Command("nginx", "-t")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}

func (m *NginxRateLimitModule) reloadNginx() error {
	cmd := exec.Command("nginx", "-s", "reload")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}

// ============================================================================
// Apache Site Module
// ============================================================================

// ApacheSiteModule manages Apache site configurations
type ApacheSiteModule struct {
	*BaseModule
}

// NewApacheSiteModule creates a new Apache site module
func NewApacheSiteModule() *ApacheSiteModule {
	return &ApacheSiteModule{
		BaseModule: NewBaseModule("apache_site", []string{"enabled", "disabled", "absent"}),
	}
}

// Check verifies the current state of an Apache site
func (m *ApacheSiteModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("apache_site module is not supported on Windows")
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	paths := m.getApachePaths()

	result := &ModuleCheckResult{
		Present:  false,
		Matches:  false,
		Metadata: make(map[string]interface{}),
	}

	// Check if site exists in sites-available
	availablePath := filepath.Join(paths.sitesAvailable, name+".conf")
	enabledPath := filepath.Join(paths.sitesEnabled, name+".conf")

	if _, err := os.Stat(availablePath); err == nil {
		result.Present = true
		result.Metadata["available_path"] = availablePath
	}

	// Check if site is enabled
	if _, err := os.Lstat(enabledPath); err == nil {
		result.Metadata["enabled"] = true
		result.Metadata["enabled_path"] = enabledPath
	} else {
		result.Metadata["enabled"] = false
	}

	// Determine current state
	enabled, _ := result.Metadata["enabled"].(bool)
	switch decl.State {
	case "enabled":
		result.Matches = result.Present && enabled
		if enabled {
			result.CurrentState = "enabled"
		} else if result.Present {
			result.CurrentState = "disabled"
		} else {
			result.CurrentState = "absent"
		}
	case "disabled":
		result.Matches = result.Present && !enabled
		if enabled {
			result.CurrentState = "enabled"
		} else if result.Present {
			result.CurrentState = "disabled"
		} else {
			result.CurrentState = "absent"
		}
	case "absent":
		result.Matches = !result.Present
		if result.Present {
			if enabled {
				result.CurrentState = "enabled"
			} else {
				result.CurrentState = "disabled"
			}
		} else {
			result.CurrentState = "absent"
		}
	}

	return result, nil
}

// Apply makes changes to reach the desired state
func (m *ApacheSiteModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("apache_site module is not supported on Windows")
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	content := getStringParameter(decl, "content", "")
	source := getStringParameter(decl, "source", "")
	reload := getBoolParameter(decl, "reload", true)

	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return nil, err
	}

	if checkResult.Matches {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: fmt.Sprintf("Apache site %s is already in state %s", name, decl.State),
		}, nil
	}

	paths := m.getApachePaths()
	availablePath := filepath.Join(paths.sitesAvailable, name+".conf")

	switch decl.State {
	case "enabled":
		// Create/update site configuration if content or source is provided
		if content != "" || source != "" {
			var configContent []byte
			if content != "" {
				configContent = []byte(content)
			} else {
				var err error
				configContent, err = os.ReadFile(source)
				if err != nil {
					return nil, fmt.Errorf("failed to read source file: %w", err)
				}
			}
			if err := os.MkdirAll(paths.sitesAvailable, 0755); err != nil {
				return nil, fmt.Errorf("failed to create sites-available directory: %w", err)
			}
			if err := os.WriteFile(availablePath, configContent, 0644); err != nil {
				return nil, fmt.Errorf("failed to write site configuration: %w", err)
			}
		}

		// Enable site using a2ensite
		if err := m.runA2ensite(name); err != nil {
			return nil, fmt.Errorf("failed to enable site: %w", err)
		}

		// Reload apache if requested
		if reload {
			if err := m.reloadApache(); err != nil {
				return nil, fmt.Errorf("failed to reload apache: %w", err)
			}
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Apache site %s enabled", name),
		}, nil

	case "disabled":
		// Disable site using a2dissite
		if err := m.runA2dissite(name); err != nil {
			return nil, fmt.Errorf("failed to disable site: %w", err)
		}

		// Reload apache if requested
		if reload {
			if err := m.reloadApache(); err != nil {
				return nil, fmt.Errorf("failed to reload apache: %w", err)
			}
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Apache site %s disabled", name),
		}, nil

	case "absent":
		// Disable site first
		_ = m.runA2dissite(name) // Ignore error if already disabled

		// Remove site configuration
		if err := os.Remove(availablePath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to remove site configuration: %w", err)
		}

		// Reload apache if requested
		if reload {
			if err := m.reloadApache(); err != nil {
				return nil, fmt.Errorf("failed to reload apache: %w", err)
			}
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Apache site %s removed", name),
		}, nil
	}

	return nil, fmt.Errorf("unknown state: %s", decl.State)
}

// Test validates the configuration without making changes
func (m *ApacheSiteModule) Test(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return nil, err
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: !checkResult.Matches,
		Comment: fmt.Sprintf("Site would be %s (currently %s)", decl.State, checkResult.CurrentState),
	}, nil
}

type apachePaths struct {
	configDir      string
	sitesAvailable string
	sitesEnabled   string
	modsAvailable  string
	modsEnabled    string
}

func (m *ApacheSiteModule) getApachePaths() apachePaths {
	if runtime.GOOS == "darwin" {
		// macOS with Homebrew
		return apachePaths{
			configDir:      "/usr/local/etc/httpd",
			sitesAvailable: "/usr/local/etc/httpd/sites-available",
			sitesEnabled:   "/usr/local/etc/httpd/sites-enabled",
			modsAvailable:  "/usr/local/etc/httpd/mods-available",
			modsEnabled:    "/usr/local/etc/httpd/mods-enabled",
		}
	}

	// Linux default (Debian/Ubuntu style)
	return apachePaths{
		configDir:      "/etc/apache2",
		sitesAvailable: "/etc/apache2/sites-available",
		sitesEnabled:   "/etc/apache2/sites-enabled",
		modsAvailable:  "/etc/apache2/mods-available",
		modsEnabled:    "/etc/apache2/mods-enabled",
	}
}

func (m *ApacheSiteModule) runA2ensite(name string) error {
	// Try a2ensite first (Debian/Ubuntu)
	cmd := exec.Command("a2ensite", name)
	if output, err := cmd.CombinedOutput(); err == nil {
		return nil
	} else if !strings.Contains(string(output), "not found") {
		return fmt.Errorf("%s", string(output))
	}

	// Fallback: create symlink manually
	paths := m.getApachePaths()
	availablePath := filepath.Join(paths.sitesAvailable, name+".conf")
	enabledPath := filepath.Join(paths.sitesEnabled, name+".conf")

	if err := os.MkdirAll(paths.sitesEnabled, 0755); err != nil {
		return fmt.Errorf("failed to create sites-enabled directory: %w", err)
	}
	_ = os.Remove(enabledPath)
	return os.Symlink(availablePath, enabledPath)
}

func (m *ApacheSiteModule) runA2dissite(name string) error {
	// Try a2dissite first (Debian/Ubuntu)
	cmd := exec.Command("a2dissite", name)
	if output, err := cmd.CombinedOutput(); err == nil {
		return nil
	} else if !strings.Contains(string(output), "not found") {
		return fmt.Errorf("%s", string(output))
	}

	// Fallback: remove symlink manually
	paths := m.getApachePaths()
	enabledPath := filepath.Join(paths.sitesEnabled, name+".conf")
	return os.Remove(enabledPath)
}

func (m *ApacheSiteModule) reloadApache() error {
	// Try different service names
	serviceNames := []string{"apache2", "httpd"}
	for _, svc := range serviceNames {
		cmd := exec.Command("systemctl", "reload", svc)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	// Try apachectl
	cmd := exec.Command("apachectl", "graceful")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to reload apache: %s", string(output))
	}
	return nil
}

// ============================================================================
// Apache Module Management
// ============================================================================

// ApacheModuleModule manages Apache module configurations
type ApacheModuleModule struct {
	*BaseModule
}

// NewApacheModuleModule creates a new Apache module management module
func NewApacheModuleModule() *ApacheModuleModule {
	return &ApacheModuleModule{
		BaseModule: NewBaseModule("apache_module", []string{"enabled", "disabled"}),
	}
}

// Check verifies the current state of an Apache module
func (m *ApacheModuleModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("apache_module is not supported on Windows")
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	result := &ModuleCheckResult{
		Present:  false,
		Matches:  false,
		Metadata: make(map[string]interface{}),
	}

	// Check if module is enabled
	enabled := m.isModuleEnabled(name)
	result.Metadata["enabled"] = enabled

	if enabled {
		result.Present = true
		result.CurrentState = "enabled"
	} else {
		// Check if module is available
		available := m.isModuleAvailable(name)
		result.Present = available
		result.CurrentState = "disabled"
	}

	switch decl.State {
	case "enabled":
		result.Matches = enabled
	case "disabled":
		result.Matches = !enabled
	}

	return result, nil
}

// Apply makes changes to reach the desired state
func (m *ApacheModuleModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("apache_module is not supported on Windows")
	}

	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	reload := getBoolParameter(decl, "reload", true)

	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return nil, err
	}

	if checkResult.Matches {
		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: false,
			Comment: fmt.Sprintf("Apache module %s is already %s", name, decl.State),
		}, nil
	}

	switch decl.State {
	case "enabled":
		if err := m.runA2enmod(name); err != nil {
			return nil, fmt.Errorf("failed to enable module: %w", err)
		}

		if reload {
			if err := m.reloadApache(); err != nil {
				return nil, fmt.Errorf("failed to reload apache: %w", err)
			}
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Apache module %s enabled", name),
		}, nil

	case "disabled":
		if err := m.runA2dismod(name); err != nil {
			return nil, fmt.Errorf("failed to disable module: %w", err)
		}

		if reload {
			if err := m.reloadApache(); err != nil {
				return nil, fmt.Errorf("failed to reload apache: %w", err)
			}
		}

		return &StateResult{
			StateID: decl.ID,
			Module:  m.Name(),
			Success: true,
			Changed: true,
			Comment: fmt.Sprintf("Apache module %s disabled", name),
		}, nil
	}

	return nil, fmt.Errorf("unknown state: %s", decl.State)
}

// Test validates the configuration without making changes
func (m *ApacheModuleModule) Test(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return nil, err
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: !checkResult.Matches,
		Comment: fmt.Sprintf("Module would be %s (currently %s)", decl.State, checkResult.CurrentState),
	}, nil
}

func (m *ApacheModuleModule) isModuleEnabled(name string) bool {
	// Check using apache2ctl -M
	cmd := exec.Command("apache2ctl", "-M")
	output, err := cmd.Output()
	if err != nil {
		// Try apachectl
		cmd = exec.Command("apachectl", "-M")
		output, err = cmd.Output()
		if err != nil {
			return false
		}
	}

	// Module names in output are like "rewrite_module" or "ssl_module"
	moduleName := name + "_module"
	return strings.Contains(string(output), moduleName)
}

func (m *ApacheModuleModule) isModuleAvailable(name string) bool {
	paths := apachePaths{
		modsAvailable: "/etc/apache2/mods-available",
	}
	if runtime.GOOS == "darwin" {
		paths.modsAvailable = "/usr/local/etc/httpd/mods-available"
	}

	// Check for .load file
	loadPath := filepath.Join(paths.modsAvailable, name+".load")
	if _, err := os.Stat(loadPath); err == nil {
		return true
	}

	// Check for .conf file
	confPath := filepath.Join(paths.modsAvailable, name+".conf")
	if _, err := os.Stat(confPath); err == nil {
		return true
	}

	return false
}

func (m *ApacheModuleModule) runA2enmod(name string) error {
	cmd := exec.Command("a2enmod", name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s", string(output))
	}
	return nil
}

func (m *ApacheModuleModule) runA2dismod(name string) error {
	cmd := exec.Command("a2dismod", name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s", string(output))
	}
	return nil
}

func (m *ApacheModuleModule) reloadApache() error {
	// Try different service names
	serviceNames := []string{"apache2", "httpd"}
	for _, svc := range serviceNames {
		cmd := exec.Command("systemctl", "reload", svc)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	// Try apachectl
	cmd := exec.Command("apachectl", "graceful")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to reload apache: %s", string(output))
	}
	return nil
}
