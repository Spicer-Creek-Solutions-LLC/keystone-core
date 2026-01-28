package selfmgmt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/template" // nosemgrep: go.lang.security.audit.xss.import-text-template.import-text-template -- templates render service configs, not HTML responses
	"time"

	"gopkg.in/yaml.v3"
)

// ServerModule manages kscore-server installation and configuration
type ServerModule struct {
	logger Logger
}

// NewServerModule creates a new server module
func NewServerModule(logger Logger) *ServerModule {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &ServerModule{logger: logger}
}

// Name returns the module name
func (m *ServerModule) Name() string {
	return "kscore_server"
}

// ComponentType returns the component type
func (m *ServerModule) ComponentType() ComponentType {
	return ComponentServer
}

// ValidStates returns valid states
func (m *ServerModule) ValidStates() []ComponentState {
	return []ComponentState{
		StateInstalled,
		StateUninstalled,
		StateRunning,
		StateStopped,
		StateConfigured,
		StateEnabled,
		StateDisabled,
	}
}

// Check checks the current state of kscore-server
func (m *ServerModule) Check(ctx context.Context, config interface{}) (*CheckResult, error) {
	cfg, ok := config.(*ServerConfig)
	if !ok {
		return nil, fmt.Errorf("invalid config type: expected *ServerConfig")
	}

	result := &CheckResult{
		Component:    ComponentServer,
		DesiredState: cfg.State,
		Diff:         make(map[string]interface{}),
		Metadata:     make(map[string]interface{}),
	}

	// Check if binary exists
	binaryPath := cfg.BinaryPath
	if binaryPath == "" {
		binaryPath = GetDefaultBinaryPath(ComponentServer)
	}
	result.Present = FileExists(binaryPath)
	result.Metadata["binary_path"] = binaryPath

	// Get installed version
	if result.Present {
		version, err := m.getInstalledVersion(ctx, binaryPath)
		if err == nil {
			result.InstalledVersion = version
			result.Metadata["installed_version"] = version
		}
	}
	result.DesiredVersion = cfg.Version

	// Check service status
	initSystem := DetectInitSystem()
	result.Metadata["init_system"] = initSystem

	running, err := m.isServiceRunning(ctx, initSystem)
	if err == nil {
		result.Running = running
		result.Metadata["running"] = running
	}

	enabled, err := m.isServiceEnabled(ctx, initSystem)
	if err == nil {
		result.Enabled = enabled
		result.Metadata["enabled"] = enabled
	}

	// Check configuration
	configPath := cfg.ConfigPath
	if configPath == "" {
		configPath = GetDefaultConfigPath(ComponentServer)
	}
	result.ConfigPath = configPath
	result.ConfigValid = FileExists(configPath)
	result.Metadata["config_path"] = configPath

	// Determine current state
	if !result.Present {
		result.CurrentState = StateUninstalled
	} else if result.Running {
		result.CurrentState = StateRunning
	} else {
		result.CurrentState = StateStopped
	}

	// Check if state matches
	switch cfg.State {
	case StateInstalled:
		result.Matches = result.Present
		if !result.Present {
			result.Diff["state"] = map[string]string{"current": "uninstalled", "desired": "installed"}
		}
		if cfg.Version != "" && result.InstalledVersion != cfg.Version {
			result.Matches = false
			result.Diff["version"] = map[string]string{"current": result.InstalledVersion, "desired": cfg.Version}
		}

	case StateUninstalled:
		result.Matches = !result.Present
		if result.Present {
			result.Diff["state"] = map[string]string{"current": "installed", "desired": "uninstalled"}
		}

	case StateRunning:
		result.Matches = result.Present && result.Running
		if !result.Present {
			result.Diff["state"] = map[string]string{"current": "uninstalled", "desired": "running"}
		} else if !result.Running {
			result.Diff["state"] = map[string]string{"current": "stopped", "desired": "running"}
		}

	case StateStopped:
		result.Matches = result.Present && !result.Running
		if !result.Present {
			result.Diff["state"] = map[string]string{"current": "uninstalled", "desired": "stopped"}
		} else if result.Running {
			result.Diff["state"] = map[string]string{"current": "running", "desired": "stopped"}
		}

	case StateConfigured:
		result.Matches = result.ConfigValid
		if !result.ConfigValid {
			result.Diff["config"] = map[string]string{"current": "missing", "desired": "configured"}
		}

	case StateEnabled:
		result.Matches = result.Enabled
		if !result.Enabled {
			result.Diff["enabled"] = map[string]bool{"current": false, "desired": true}
		}

	case StateDisabled:
		result.Matches = !result.Enabled
		if result.Enabled {
			result.Diff["enabled"] = map[string]bool{"current": true, "desired": false}
		}
	}

	return result, nil
}

// Apply applies the desired state
func (m *ServerModule) Apply(ctx context.Context, config interface{}, dryRun bool) (*ApplyResult, error) {
	cfg, ok := config.(*ServerConfig)
	if !ok {
		return nil, fmt.Errorf("invalid config type: expected *ServerConfig")
	}

	startTime := time.Now()
	result := &ApplyResult{
		Component: ComponentServer,
		StartTime: startTime,
		Changes:   make(map[string]interface{}),
	}

	// Check current state
	checkResult, err := m.Check(ctx, config)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to check current state: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	result.PreviousState = checkResult.CurrentState

	// Already in desired state
	if checkResult.Matches {
		result.Success = true
		result.Changed = false
		result.NewState = checkResult.CurrentState
		result.Comment = "Already in desired state"
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	if dryRun {
		result.Success = true
		result.Changed = true
		result.Comment = "Would apply changes (dry-run)"
		result.Changes = checkResult.Diff
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Apply changes based on desired state
	var applyErr error
	switch cfg.State {
	case StateInstalled:
		applyErr = m.install(ctx, cfg, result)
	case StateUninstalled:
		applyErr = m.uninstall(ctx, cfg, result)
	case StateRunning:
		if !checkResult.Present {
			applyErr = m.install(ctx, cfg, result)
			if applyErr == nil {
				applyErr = m.start(ctx, cfg, result)
			}
		} else {
			applyErr = m.start(ctx, cfg, result)
		}
	case StateStopped:
		applyErr = m.stop(ctx, cfg, result)
	case StateConfigured:
		applyErr = m.configure(ctx, cfg, result)
	case StateEnabled:
		applyErr = m.enable(ctx, cfg, result)
	case StateDisabled:
		applyErr = m.disable(ctx, cfg, result)
	default:
		applyErr = fmt.Errorf("unsupported state: %s", cfg.State)
	}

	if applyErr != nil {
		result.Error = applyErr
		result.Comment = fmt.Sprintf("Failed to apply state: %v", applyErr)
	} else {
		result.Success = true
		result.Changed = true
		result.NewState = cfg.State
		result.Comment = fmt.Sprintf("Successfully applied state: %s", cfg.State)
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil
}

// Validate validates the configuration
func (m *ServerModule) Validate(config interface{}) error {
	cfg, ok := config.(*ServerConfig)
	if !ok {
		return fmt.Errorf("invalid config type: expected *ServerConfig")
	}

	var errors ValidationErrors

	// Validate state
	validState := false
	for _, s := range m.ValidStates() {
		if s == cfg.State {
			validState = true
			break
		}
	}
	if !validState {
		errors.Add("state", fmt.Sprintf("invalid state: %s", cfg.State))
	}

	// Validate install method
	if cfg.InstallMethod != "" {
		switch cfg.InstallMethod {
		case InstallMethodPackage, InstallMethodBinary, InstallMethodDocker, InstallMethodHelm:
			// valid
		default:
			errors.Add("install_method", fmt.Sprintf("invalid install method: %s", cfg.InstallMethod))
		}
	}

	// Validate TLS configuration
	if cfg.TLSEnabled {
		if cfg.TLSCertFile == "" {
			errors.Add("tls_cert_file", "required when TLS is enabled")
		}
		if cfg.TLSKeyFile == "" {
			errors.Add("tls_key_file", "required when TLS is enabled")
		}
	}

	// Validate database configuration
	if cfg.DatabaseType != "" {
		switch cfg.DatabaseType {
		case "sqlite", "postgresql":
			// valid
		default:
			errors.Add("database_type", fmt.Sprintf("invalid database type: %s", cfg.DatabaseType))
		}
	}

	// Validate NATS mode
	if cfg.NATSMode != "" {
		switch cfg.NATSMode {
		case "embedded", "external", "leaf":
			// valid
		default:
			errors.Add("nats_mode", fmt.Sprintf("invalid NATS mode: %s", cfg.NATSMode))
		}
	}

	if errors.HasErrors() {
		return errors
	}
	return nil
}

// install installs kscore-server
func (m *ServerModule) install(ctx context.Context, cfg *ServerConfig, result *ApplyResult) error {
	m.logger.Info("Installing kscore-server", "version", cfg.Version, "method", cfg.InstallMethod)

	method := cfg.InstallMethod
	if method == "" {
		method = InstallMethodPackage
	}

	switch method {
	case InstallMethodPackage:
		return m.installViaPackage(ctx, cfg, result)
	case InstallMethodBinary:
		return m.installViaBinary(ctx, cfg, result)
	case InstallMethodDocker:
		return m.installViaDocker(ctx, cfg, result)
	default:
		return fmt.Errorf("unsupported install method: %s", method)
	}
}

func (m *ServerModule) installViaPackage(ctx context.Context, cfg *ServerConfig, result *ApplyResult) error {
	pm := DetectPackageManager()
	packageName := "kscore-server"

	var cmd string
	var args []string

	switch pm {
	case "apt":
		cmd = "apt-get"
		args = []string{"install", "-y", packageName}
		if cfg.Version != "" {
			args[2] = fmt.Sprintf("%s=%s", packageName, cfg.Version)
		}
	case "dnf", "yum":
		cmd = pm
		args = []string{"install", "-y", packageName}
		if cfg.Version != "" {
			args[2] = fmt.Sprintf("%s-%s", packageName, cfg.Version)
		}
	case "apk":
		cmd = "apk"
		args = []string{"add", packageName}
		if cfg.Version != "" {
			args[1] = fmt.Sprintf("%s=%s", packageName, cfg.Version)
		}
	case "brew":
		cmd = "brew"
		args = []string{"install", packageName}
		if cfg.Version != "" {
			args = []string{"install", fmt.Sprintf("%s@%s", packageName, cfg.Version)}
		}
	case "chocolatey":
		cmd = "choco"
		args = []string{"install", "-y", packageName}
		if cfg.Version != "" {
			args = append(args, "--version", cfg.Version)
		}
	default:
		return fmt.Errorf("unsupported package manager: %s", pm)
	}

	output, err := RunCommand(ctx, cmd, args...)
	if err != nil {
		return fmt.Errorf("package install failed: %w\nOutput: %s", err, output)
	}

	result.Changes["installed"] = true
	result.Changes["install_method"] = "package"
	return nil
}

func (m *ServerModule) installViaBinary(ctx context.Context, cfg *ServerConfig, result *ApplyResult) error {
	// Determine download URL
	version := cfg.Version
	if version == "" {
		version = "latest"
	}

	arch := runtime.GOARCH
	osName := runtime.GOOS

	// Download binary
	downloadURL := fmt.Sprintf("https://github.com/shawnbutts/keystone-core/releases/download/%s/kscore-server-%s-%s", version, osName, arch)
	if osName == "windows" {
		downloadURL += ".exe"
	}

	binaryPath := cfg.BinaryPath
	if binaryPath == "" {
		binaryPath = GetDefaultBinaryPath(ComponentServer)
	}

	// Use curl or wget to download
	var output string
	var err error
	if _, lookErr := RunCommand(ctx, "which", "curl"); lookErr == nil {
		output, err = RunCommand(ctx, "curl", "-L", "-o", binaryPath, downloadURL)
	} else {
		output, err = RunCommand(ctx, "wget", "-O", binaryPath, downloadURL)
	}
	if err != nil {
		return fmt.Errorf("binary download failed: %w\nOutput: %s", err, output)
	}

	// Make executable
	if runtime.GOOS != "windows" {
		if err := os.Chmod(binaryPath, 0755); err != nil {
			return fmt.Errorf("failed to make binary executable: %w", err)
		}
	}

	// Create systemd service file if on Linux
	if runtime.GOOS == "linux" && DetectInitSystem() == "systemd" {
		if err := m.createSystemdService(cfg); err != nil {
			return fmt.Errorf("failed to create systemd service: %w", err)
		}
	}

	result.Changes["installed"] = true
	result.Changes["install_method"] = "binary"
	result.Changes["binary_path"] = binaryPath
	return nil
}

func (m *ServerModule) installViaDocker(ctx context.Context, cfg *ServerConfig, result *ApplyResult) error {
	version := cfg.Version
	if version == "" {
		version = "latest"
	}

	image := fmt.Sprintf("ghcr.io/shawnbutts/keystone-core/kscore-server:%s", version)

	output, err := RunCommand(ctx, "docker", "pull", image)
	if err != nil {
		return fmt.Errorf("docker pull failed: %w\nOutput: %s", err, output)
	}

	result.Changes["installed"] = true
	result.Changes["install_method"] = "docker"
	result.Changes["image"] = image
	return nil
}

// uninstall uninstalls kscore-server
func (m *ServerModule) uninstall(ctx context.Context, cfg *ServerConfig, result *ApplyResult) error {
	m.logger.Info("Uninstalling kscore-server")

	// Stop service first
	_ = m.stop(ctx, cfg, result)

	// Disable service
	_ = m.disable(ctx, cfg, result)

	pm := DetectPackageManager()
	packageName := "kscore-server"

	var cmd string
	var args []string

	switch pm {
	case "apt":
		cmd = "apt-get"
		args = []string{"remove", "-y", packageName}
	case "dnf", "yum":
		cmd = pm
		args = []string{"remove", "-y", packageName}
	case "apk":
		cmd = "apk"
		args = []string{"del", packageName}
	case "brew":
		cmd = "brew"
		args = []string{"uninstall", packageName}
	case "chocolatey":
		cmd = "choco"
		args = []string{"uninstall", "-y", packageName}
	default:
		// Try to remove binary directly
		binaryPath := cfg.BinaryPath
		if binaryPath == "" {
			binaryPath = GetDefaultBinaryPath(ComponentServer)
		}
		if err := os.Remove(binaryPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove binary: %w", err)
		}
		result.Changes["uninstalled"] = true
		return nil
	}

	output, err := RunCommand(ctx, cmd, args...)
	if err != nil {
		return fmt.Errorf("package removal failed: %w\nOutput: %s", err, output)
	}

	result.Changes["uninstalled"] = true
	return nil
}

// start starts the service
func (m *ServerModule) start(ctx context.Context, cfg *ServerConfig, result *ApplyResult) error {
	m.logger.Info("Starting kscore-server")

	initSystem := DetectInitSystem()

	var output string
	var err error

	switch initSystem {
	case "systemd":
		output, err = RunCommand(ctx, "systemctl", "start", "kscore-server")
	case "launchd":
		output, err = RunCommand(ctx, "launchctl", "load", "-w", "/Library/LaunchDaemons/com.keystone.kscore-server.plist")
	case "openrc":
		output, err = RunCommand(ctx, "rc-service", "kscore-server", "start")
	case "windows":
		output, err = RunCommand(ctx, "sc", "start", "kscore-server")
	default:
		output, err = RunCommand(ctx, "service", "kscore-server", "start")
	}

	if err != nil {
		return fmt.Errorf("failed to start service: %w\nOutput: %s", err, output)
	}

	result.Changes["started"] = true
	result.RestartedServices = append(result.RestartedServices, "kscore-server")
	return nil
}

// stop stops the service
func (m *ServerModule) stop(ctx context.Context, cfg *ServerConfig, result *ApplyResult) error {
	m.logger.Info("Stopping kscore-server")

	initSystem := DetectInitSystem()

	var output string
	var err error

	switch initSystem {
	case "systemd":
		output, err = RunCommand(ctx, "systemctl", "stop", "kscore-server")
	case "launchd":
		output, err = RunCommand(ctx, "launchctl", "unload", "/Library/LaunchDaemons/com.keystone.kscore-server.plist")
	case "openrc":
		output, err = RunCommand(ctx, "rc-service", "kscore-server", "stop")
	case "windows":
		output, err = RunCommand(ctx, "sc", "stop", "kscore-server")
	default:
		output, err = RunCommand(ctx, "service", "kscore-server", "stop")
	}

	if err != nil {
		return fmt.Errorf("failed to stop service: %w\nOutput: %s", err, output)
	}

	result.Changes["stopped"] = true
	return nil
}

// enable enables the service at boot
func (m *ServerModule) enable(ctx context.Context, cfg *ServerConfig, result *ApplyResult) error {
	m.logger.Info("Enabling kscore-server")

	initSystem := DetectInitSystem()

	var output string
	var err error

	switch initSystem {
	case "systemd":
		output, err = RunCommand(ctx, "systemctl", "enable", "kscore-server")
	case "openrc":
		output, err = RunCommand(ctx, "rc-update", "add", "kscore-server", "default")
	case "windows":
		output, err = RunCommand(ctx, "sc", "config", "kscore-server", "start=", "auto")
	default:
		// launchd services are enabled by default when loaded
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to enable service: %w\nOutput: %s", err, output)
	}

	result.Changes["enabled"] = true
	return nil
}

// disable disables the service at boot
func (m *ServerModule) disable(ctx context.Context, cfg *ServerConfig, result *ApplyResult) error {
	m.logger.Info("Disabling kscore-server")

	initSystem := DetectInitSystem()

	var output string
	var err error

	switch initSystem {
	case "systemd":
		output, err = RunCommand(ctx, "systemctl", "disable", "kscore-server")
	case "openrc":
		output, err = RunCommand(ctx, "rc-update", "del", "kscore-server", "default")
	case "windows":
		output, err = RunCommand(ctx, "sc", "config", "kscore-server", "start=", "disabled")
	default:
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to disable service: %w\nOutput: %s", err, output)
	}

	result.Changes["disabled"] = true
	return nil
}

// configure applies configuration
func (m *ServerModule) configure(ctx context.Context, cfg *ServerConfig, result *ApplyResult) error {
	m.logger.Info("Configuring kscore-server")

	configPath := cfg.ConfigPath
	if configPath == "" {
		configPath = GetDefaultConfigPath(ComponentServer)
	}

	// Build configuration
	configData := m.buildConfig(cfg)

	// Write configuration
	data, err := yaml.Marshal(configData)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := WriteFile(configPath, data, 0640); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	result.Changes["configured"] = true
	result.Changes["config_path"] = configPath
	result.RequiresRestart = true
	return nil
}

// buildConfig builds the server configuration
func (m *ServerModule) buildConfig(cfg *ServerConfig) map[string]interface{} {
	config := make(map[string]interface{})

	if cfg.ClusterID != "" {
		config["cluster_id"] = cfg.ClusterID
	}

	// API configuration
	api := make(map[string]interface{})
	if cfg.ListenAddress != "" {
		api["listen"] = cfg.ListenAddress
	}
	if cfg.GRPCAddress != "" {
		api["grpc_listen"] = cfg.GRPCAddress
	}
	if cfg.TLSEnabled {
		api["tls"] = map[string]interface{}{
			"enabled":   true,
			"cert_file": cfg.TLSCertFile,
			"key_file":  cfg.TLSKeyFile,
			"ca_file":   cfg.TLSCAFile,
		}
	}
	if len(api) > 0 {
		config["api"] = api
	}

	// NATS configuration
	nats := make(map[string]interface{})
	if cfg.NATSMode != "" {
		nats["mode"] = cfg.NATSMode
	}
	if len(cfg.NATSURLs) > 0 {
		nats["urls"] = cfg.NATSURLs
	}
	if len(nats) > 0 {
		config["nats"] = nats
	}

	// Database configuration
	db := make(map[string]interface{})
	if cfg.DatabaseType != "" {
		db["type"] = cfg.DatabaseType
	}
	if cfg.DatabaseURL != "" {
		db["url"] = cfg.DatabaseURL
	}
	if len(db) > 0 {
		config["database"] = db
	}

	// Clustering configuration
	if cfg.EnableClustering {
		cluster := map[string]interface{}{
			"enabled": true,
		}
		if len(cfg.EtcdEndpoints) > 0 {
			cluster["etcd_endpoints"] = cfg.EtcdEndpoints
		}
		config["cluster"] = cluster
	}

	// Directories
	if cfg.DataDir != "" {
		config["data_dir"] = cfg.DataDir
	}
	if cfg.LogDir != "" {
		config["log_dir"] = cfg.LogDir
	}

	return config
}

// getInstalledVersion gets the installed version
func (m *ServerModule) getInstalledVersion(ctx context.Context, binaryPath string) (string, error) {
	output, err := RunCommand(ctx, binaryPath, "version")
	if err != nil {
		return "", err
	}

	// Parse version from output (e.g., "kscore-server version 1.0.0")
	parts := strings.Fields(output)
	for i, part := range parts {
		if part == "version" && i+1 < len(parts) {
			return parts[i+1], nil
		}
	}

	// Try to find semver pattern
	for _, part := range parts {
		if strings.HasPrefix(part, "v") || strings.Contains(part, ".") {
			return strings.TrimPrefix(part, "v"), nil
		}
	}

	return output, nil
}

// isServiceRunning checks if the service is running
func (m *ServerModule) isServiceRunning(ctx context.Context, initSystem string) (bool, error) {
	var output string
	var err error

	switch initSystem {
	case "systemd":
		output, err = RunCommand(ctx, "systemctl", "is-active", "kscore-server")
		return output == "active", err
	case "launchd":
		output, err = RunCommand(ctx, "launchctl", "list", "com.keystone.kscore-server")
		return err == nil, nil
	case "openrc":
		output, err = RunCommand(ctx, "rc-service", "kscore-server", "status")
		return strings.Contains(output, "started"), err
	case "windows":
		output, err = RunCommand(ctx, "sc", "query", "kscore-server")
		return strings.Contains(output, "RUNNING"), err
	default:
		output, err = RunCommand(ctx, "service", "kscore-server", "status")
		return strings.Contains(strings.ToLower(output), "running"), err
	}
}

// isServiceEnabled checks if the service is enabled at boot
func (m *ServerModule) isServiceEnabled(ctx context.Context, initSystem string) (bool, error) {
	var output string
	var err error

	switch initSystem {
	case "systemd":
		output, err = RunCommand(ctx, "systemctl", "is-enabled", "kscore-server")
		return output == "enabled", err
	case "openrc":
		output, err = RunCommand(ctx, "rc-update", "show", "default")
		return strings.Contains(output, "kscore-server"), err
	case "windows":
		output, err = RunCommand(ctx, "sc", "qc", "kscore-server")
		return strings.Contains(output, "AUTO_START"), err
	default:
		// launchd services are enabled when loaded
		return true, nil
	}
}

// createSystemdService creates a systemd service file
func (m *ServerModule) createSystemdService(cfg *ServerConfig) error {
	binaryPath := cfg.BinaryPath
	if binaryPath == "" {
		binaryPath = GetDefaultBinaryPath(ComponentServer)
	}

	configPath := cfg.ConfigPath
	if configPath == "" {
		configPath = GetDefaultConfigPath(ComponentServer)
	}

	user := cfg.User
	if user == "" {
		user = "kscore"
	}

	group := cfg.Group
	if group == "" {
		group = "kscore"
	}

	tmpl := `[Unit]
Description=Keystone Core Server
Documentation=https://github.com/shawnbutts/keystone-core
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User={{.User}}
Group={{.Group}}
ExecStart={{.BinaryPath}} --config {{.ConfigPath}}
Restart=on-failure
RestartSec=5
LimitNOFILE=65536

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths={{.DataDir}} {{.LogDir}}
PrivateTmp=true

[Install]
WantedBy=multi-user.target
`

	t, err := template.New("systemd").Parse(tmpl)
	if err != nil {
		return err
	}

	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = GetDefaultDataDir(ComponentServer)
	}

	logDir := cfg.LogDir
	if logDir == "" {
		logDir = "/var/log/kscore"
	}

	var buf strings.Builder
	err = t.Execute(&buf, map[string]string{
		"BinaryPath": binaryPath,
		"ConfigPath": configPath,
		"User":       user,
		"Group":      group,
		"DataDir":    dataDir,
		"LogDir":     logDir,
	})
	if err != nil {
		return err
	}

	servicePath := "/etc/systemd/system/kscore-server.service"
	if err := WriteFile(servicePath, []byte(buf.String()), 0644); err != nil {
		return err
	}

	// Reload systemd
	_, err = RunCommand(context.Background(), "systemctl", "daemon-reload")
	return err
}

// DefaultServerConfig returns a default server configuration
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		BaseConfig: BaseConfig{
			State:         StateRunning,
			Channel:       "stable",
			InstallMethod: InstallMethodPackage,
			ConfigPath:    GetDefaultConfigPath(ComponentServer),
			BinaryPath:    GetDefaultBinaryPath(ComponentServer),
			DataDir:       GetDefaultDataDir(ComponentServer),
			LogDir:        filepath.Join(GetDefaultDataDir(ComponentServer), "..", "log"),
			User:          "kscore",
			Group:         "kscore",
		},
		ListenAddress: ":8080",
		GRPCAddress:   ":9090",
		NATSMode:      "embedded",
		DatabaseType:  "sqlite",
	}
}
