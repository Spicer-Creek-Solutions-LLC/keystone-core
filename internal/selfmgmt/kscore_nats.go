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
)

// NATSModule manages NATS server installation and configuration
type NATSModule struct {
	logger Logger
}

// NewNATSModule creates a new NATS module
func NewNATSModule(logger Logger) *NATSModule {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &NATSModule{logger: logger}
}

// Name returns the module name
func (m *NATSModule) Name() string {
	return "kscore_nats"
}

// ComponentType returns the component type
func (m *NATSModule) ComponentType() ComponentType {
	return ComponentNATS
}

// ValidStates returns valid states
func (m *NATSModule) ValidStates() []ComponentState {
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

// Check checks the current state of NATS
func (m *NATSModule) Check(ctx context.Context, config interface{}) (*CheckResult, error) {
	cfg, ok := config.(*NATSConfig)
	if !ok {
		return nil, fmt.Errorf("invalid config type: expected *NATSConfig")
	}

	result := &CheckResult{
		Component:    ComponentNATS,
		DesiredState: cfg.State,
		Diff:         make(map[string]interface{}),
		Metadata:     make(map[string]interface{}),
	}

	// Check if binary exists
	binaryPath := cfg.BinaryPath
	if binaryPath == "" {
		binaryPath = GetDefaultBinaryPath(ComponentNATS)
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
		configPath = GetDefaultConfigPath(ComponentNATS)
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
func (m *NATSModule) Apply(ctx context.Context, config interface{}, dryRun bool) (*ApplyResult, error) {
	cfg, ok := config.(*NATSConfig)
	if !ok {
		return nil, fmt.Errorf("invalid config type: expected *NATSConfig")
	}

	startTime := time.Now()
	result := &ApplyResult{
		Component: ComponentNATS,
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
func (m *NATSModule) Validate(config interface{}) error {
	cfg, ok := config.(*NATSConfig)
	if !ok {
		return fmt.Errorf("invalid config type: expected *NATSConfig")
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

	// Validate mode
	if cfg.Mode != "" {
		switch cfg.Mode {
		case "standalone", "cluster", "leaf":
			// valid
		default:
			errors.Add("mode", fmt.Sprintf("invalid mode: %s", cfg.Mode))
		}
	}

	// Validate ports
	if cfg.ClientPort < 0 || cfg.ClientPort > 65535 {
		errors.Add("client_port", "must be between 0 and 65535")
	}
	if cfg.ClusterPort < 0 || cfg.ClusterPort > 65535 {
		errors.Add("cluster_port", "must be between 0 and 65535")
	}
	if cfg.HTTPPort < 0 || cfg.HTTPPort > 65535 {
		errors.Add("http_port", "must be between 0 and 65535")
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

	// Validate cluster configuration
	if cfg.Mode == "cluster" {
		if cfg.ClusterName == "" {
			errors.Add("cluster_name", "required for cluster mode")
		}
		if len(cfg.Routes) == 0 {
			errors.Add("routes", "at least one route required for cluster mode")
		}
	}

	if errors.HasErrors() {
		return errors
	}
	return nil
}

// install installs NATS
func (m *NATSModule) install(ctx context.Context, cfg *NATSConfig, result *ApplyResult) error {
	m.logger.Info("Installing NATS server", "version", cfg.Version, "method", cfg.InstallMethod)

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

func (m *NATSModule) installViaPackage(ctx context.Context, cfg *NATSConfig, result *ApplyResult) error {
	pm := DetectPackageManager()
	packageName := "nats-server"

	var cmd string
	var args []string

	switch pm {
	case "apt":
		// NATS may need a custom repo for apt
		cmd = "apt-get"
		args = []string{"install", "-y", packageName}
	case "dnf", "yum":
		cmd = pm
		args = []string{"install", "-y", packageName}
	case "apk":
		cmd = "apk"
		args = []string{"add", packageName}
	case "brew":
		cmd = "brew"
		args = []string{"install", "nats-server"}
	case "chocolatey":
		cmd = "choco"
		args = []string{"install", "-y", packageName}
	default:
		// Fall back to binary install
		return m.installViaBinary(ctx, cfg, result)
	}

	output, err := RunCommand(ctx, cmd, args...)
	if err != nil {
		// Try binary install as fallback
		m.logger.Warn("Package install failed, trying binary install", "error", err)
		return m.installViaBinary(ctx, cfg, result)
	}

	result.Changes["installed"] = true
	result.Changes["install_method"] = "package"
	_ = output
	return nil
}

func (m *NATSModule) installViaBinary(ctx context.Context, cfg *NATSConfig, result *ApplyResult) error {
	version := cfg.Version
	if version == "" {
		version = "latest"
	}

	arch := runtime.GOARCH
	osName := runtime.GOOS

	// NATS uses different naming convention
	archName := arch
	if arch == "amd64" {
		archName = "amd64"
	} else if arch == "arm64" {
		archName = "arm64"
	}

	downloadURL := fmt.Sprintf("https://github.com/nats-io/nats-server/releases/download/v%s/nats-server-v%s-%s-%s.zip", version, version, osName, archName)

	// Download to temp location
	tempDir := os.TempDir()
	zipPath := filepath.Join(tempDir, "nats-server.zip")

	var output string
	var err error
	if _, lookErr := RunCommand(ctx, "which", "curl"); lookErr == nil {
		output, err = RunCommand(ctx, "curl", "-L", "-o", zipPath, downloadURL)
	} else {
		output, err = RunCommand(ctx, "wget", "-O", zipPath, downloadURL)
	}
	if err != nil {
		return fmt.Errorf("binary download failed: %w\nOutput: %s", err, output)
	}

	// Extract
	extractDir := filepath.Join(tempDir, "nats-server")
	output, err = RunCommand(ctx, "unzip", "-o", zipPath, "-d", extractDir)
	if err != nil {
		return fmt.Errorf("failed to extract: %w\nOutput: %s", err, output)
	}

	// Find the binary
	binaryPath := cfg.BinaryPath
	if binaryPath == "" {
		binaryPath = GetDefaultBinaryPath(ComponentNATS)
	}

	// Copy binary to destination
	srcBinary := filepath.Join(extractDir, fmt.Sprintf("nats-server-v%s-%s-%s", version, osName, archName), "nats-server")
	if runtime.GOOS == "windows" {
		srcBinary += ".exe"
	}

	if err := CopyFile(srcBinary, binaryPath); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(binaryPath, 0755); err != nil {
			return fmt.Errorf("failed to make binary executable: %w", err)
		}
	}

	// Cleanup
	os.RemoveAll(extractDir)
	os.Remove(zipPath)

	// Create service file
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

func (m *NATSModule) installViaDocker(ctx context.Context, cfg *NATSConfig, result *ApplyResult) error {
	version := cfg.Version
	if version == "" {
		version = "latest"
	}

	image := fmt.Sprintf("nats:%s", version)

	output, err := RunCommand(ctx, "docker", "pull", image)
	if err != nil {
		return fmt.Errorf("docker pull failed: %w\nOutput: %s", err, output)
	}

	result.Changes["installed"] = true
	result.Changes["install_method"] = "docker"
	result.Changes["image"] = image
	return nil
}

// uninstall uninstalls NATS
func (m *NATSModule) uninstall(ctx context.Context, cfg *NATSConfig, result *ApplyResult) error {
	m.logger.Info("Uninstalling NATS server")

	_ = m.stop(ctx, cfg, result)
	_ = m.disable(ctx, cfg, result)

	pm := DetectPackageManager()

	var cmd string
	var args []string

	switch pm {
	case "apt":
		cmd = "apt-get"
		args = []string{"remove", "-y", "nats-server"}
	case "dnf", "yum":
		cmd = pm
		args = []string{"remove", "-y", "nats-server"}
	case "apk":
		cmd = "apk"
		args = []string{"del", "nats-server"}
	case "brew":
		cmd = "brew"
		args = []string{"uninstall", "nats-server"}
	case "chocolatey":
		cmd = "choco"
		args = []string{"uninstall", "-y", "nats-server"}
	default:
		binaryPath := cfg.BinaryPath
		if binaryPath == "" {
			binaryPath = GetDefaultBinaryPath(ComponentNATS)
		}
		if err := os.Remove(binaryPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove binary: %w", err)
		}
		result.Changes["uninstalled"] = true
		return nil
	}

	output, err := RunCommand(ctx, cmd, args...)
	if err != nil {
		// If package removal fails, try removing binary
		binaryPath := cfg.BinaryPath
		if binaryPath == "" {
			binaryPath = GetDefaultBinaryPath(ComponentNATS)
		}
		if rmErr := os.Remove(binaryPath); rmErr != nil && !os.IsNotExist(rmErr) {
			return fmt.Errorf("package removal failed: %w\nOutput: %s", err, output)
		}
	}

	result.Changes["uninstalled"] = true
	return nil
}

// start starts the service
func (m *NATSModule) start(ctx context.Context, cfg *NATSConfig, result *ApplyResult) error {
	m.logger.Info("Starting NATS server")

	initSystem := DetectInitSystem()

	var output string
	var err error

	switch initSystem {
	case "systemd":
		output, err = RunCommand(ctx, "systemctl", "start", "nats-server")
	case "launchd":
		output, err = RunCommand(ctx, "launchctl", "load", "-w", "/Library/LaunchDaemons/io.nats.nats-server.plist")
	case "openrc":
		output, err = RunCommand(ctx, "rc-service", "nats-server", "start")
	case "windows":
		output, err = RunCommand(ctx, "sc", "start", "nats-server")
	default:
		output, err = RunCommand(ctx, "service", "nats-server", "start")
	}

	if err != nil {
		return fmt.Errorf("failed to start service: %w\nOutput: %s", err, output)
	}

	result.Changes["started"] = true
	result.RestartedServices = append(result.RestartedServices, "nats-server")
	return nil
}

// stop stops the service
func (m *NATSModule) stop(ctx context.Context, cfg *NATSConfig, result *ApplyResult) error {
	m.logger.Info("Stopping NATS server")

	initSystem := DetectInitSystem()

	var output string
	var err error

	switch initSystem {
	case "systemd":
		output, err = RunCommand(ctx, "systemctl", "stop", "nats-server")
	case "launchd":
		output, err = RunCommand(ctx, "launchctl", "unload", "/Library/LaunchDaemons/io.nats.nats-server.plist")
	case "openrc":
		output, err = RunCommand(ctx, "rc-service", "nats-server", "stop")
	case "windows":
		output, err = RunCommand(ctx, "sc", "stop", "nats-server")
	default:
		output, err = RunCommand(ctx, "service", "nats-server", "stop")
	}

	if err != nil {
		return fmt.Errorf("failed to stop service: %w\nOutput: %s", err, output)
	}

	result.Changes["stopped"] = true
	return nil
}

// enable enables the service at boot
func (m *NATSModule) enable(ctx context.Context, cfg *NATSConfig, result *ApplyResult) error {
	m.logger.Info("Enabling NATS server")

	initSystem := DetectInitSystem()

	var output string
	var err error

	switch initSystem {
	case "systemd":
		output, err = RunCommand(ctx, "systemctl", "enable", "nats-server")
	case "openrc":
		output, err = RunCommand(ctx, "rc-update", "add", "nats-server", "default")
	case "windows":
		output, err = RunCommand(ctx, "sc", "config", "nats-server", "start=", "auto")
	default:
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to enable service: %w\nOutput: %s", err, output)
	}

	result.Changes["enabled"] = true
	return nil
}

// disable disables the service at boot
func (m *NATSModule) disable(ctx context.Context, cfg *NATSConfig, result *ApplyResult) error {
	m.logger.Info("Disabling NATS server")

	initSystem := DetectInitSystem()

	var output string
	var err error

	switch initSystem {
	case "systemd":
		output, err = RunCommand(ctx, "systemctl", "disable", "nats-server")
	case "openrc":
		output, err = RunCommand(ctx, "rc-update", "del", "nats-server", "default")
	case "windows":
		output, err = RunCommand(ctx, "sc", "config", "nats-server", "start=", "disabled")
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
func (m *NATSModule) configure(ctx context.Context, cfg *NATSConfig, result *ApplyResult) error {
	m.logger.Info("Configuring NATS server")

	configPath := cfg.ConfigPath
	if configPath == "" {
		configPath = GetDefaultConfigPath(ComponentNATS)
	}

	configContent := m.buildConfig(cfg)

	if err := WriteFile(configPath, []byte(configContent), 0640); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	result.Changes["configured"] = true
	result.Changes["config_path"] = configPath
	result.RequiresRestart = true
	return nil
}

// buildConfig builds the NATS configuration file content
func (m *NATSModule) buildConfig(cfg *NATSConfig) string {
	var sb strings.Builder

	// Server name
	sb.WriteString("# NATS Server Configuration\n")
	sb.WriteString("# Generated by Keystone Core Self-Management\n\n")

	// Client port
	clientPort := cfg.ClientPort
	if clientPort == 0 {
		clientPort = 4222
	}
	sb.WriteString(fmt.Sprintf("port: %d\n", clientPort))

	// HTTP monitoring port
	if cfg.HTTPPort > 0 {
		sb.WriteString(fmt.Sprintf("http_port: %d\n", cfg.HTTPPort))
	}

	// Data directory
	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = GetDefaultDataDir(ComponentNATS)
	}

	// JetStream
	if cfg.JetStreamEnabled {
		sb.WriteString("\njetstream {\n")
		storePath := cfg.JetStreamStorePath
		if storePath == "" {
			storePath = filepath.Join(dataDir, "jetstream")
		}
		sb.WriteString(fmt.Sprintf("  store_dir: %s\n", storePath))
		if cfg.JetStreamMaxMemory > 0 {
			sb.WriteString(fmt.Sprintf("  max_memory_store: %d\n", cfg.JetStreamMaxMemory))
		}
		if cfg.JetStreamMaxFile > 0 {
			sb.WriteString(fmt.Sprintf("  max_file_store: %d\n", cfg.JetStreamMaxFile))
		}
		sb.WriteString("}\n")
	}

	// Cluster configuration
	if cfg.Mode == "cluster" && cfg.ClusterName != "" {
		clusterPort := cfg.ClusterPort
		if clusterPort == 0 {
			clusterPort = 6222
		}

		sb.WriteString("\ncluster {\n")
		sb.WriteString(fmt.Sprintf("  name: %s\n", cfg.ClusterName))
		sb.WriteString(fmt.Sprintf("  port: %d\n", clusterPort))

		if len(cfg.Routes) > 0 {
			sb.WriteString("  routes: [\n")
			for _, route := range cfg.Routes {
				sb.WriteString(fmt.Sprintf("    %s\n", route))
			}
			sb.WriteString("  ]\n")
		}

		sb.WriteString("}\n")
	}

	// TLS configuration
	if cfg.TLSEnabled {
		sb.WriteString("\ntls {\n")
		sb.WriteString(fmt.Sprintf("  cert_file: %s\n", cfg.TLSCertFile))
		sb.WriteString(fmt.Sprintf("  key_file: %s\n", cfg.TLSKeyFile))
		if cfg.TLSCAFile != "" {
			sb.WriteString(fmt.Sprintf("  ca_file: %s\n", cfg.TLSCAFile))
		}
		sb.WriteString("}\n")
	}

	// Authorization
	if cfg.Authorization != nil {
		sb.WriteString("\nauthorization {\n")
		if cfg.Authorization.Token != "" {
			sb.WriteString(fmt.Sprintf("  token: %s\n", cfg.Authorization.Token))
		}
		if len(cfg.Authorization.Users) > 0 {
			sb.WriteString("  users: [\n")
			for _, user := range cfg.Authorization.Users {
				sb.WriteString("    {\n")
				sb.WriteString(fmt.Sprintf("      user: %s\n", user.User))
				sb.WriteString(fmt.Sprintf("      password: %s\n", user.Password))
				if len(user.Publish) > 0 {
					sb.WriteString(fmt.Sprintf("      publish: [%s]\n", strings.Join(user.Publish, ", ")))
				}
				if len(user.Subscribe) > 0 {
					sb.WriteString(fmt.Sprintf("      subscribe: [%s]\n", strings.Join(user.Subscribe, ", ")))
				}
				sb.WriteString("    }\n")
			}
			sb.WriteString("  ]\n")
		}
		sb.WriteString("}\n")
	}

	// Logging
	sb.WriteString("\n# Logging\n")
	sb.WriteString("debug: false\n")
	sb.WriteString("trace: false\n")
	logDir := cfg.LogDir
	if logDir == "" {
		logDir = filepath.Join(dataDir, "..", "log")
	}
	sb.WriteString(fmt.Sprintf("logfile: %s/nats-server.log\n", logDir))

	return sb.String()
}

// getInstalledVersion gets the installed version
func (m *NATSModule) getInstalledVersion(ctx context.Context, binaryPath string) (string, error) {
	output, err := RunCommand(ctx, binaryPath, "--version")
	if err != nil {
		return "", err
	}

	// Parse "nats-server: v2.10.0"
	parts := strings.Fields(output)
	for _, part := range parts {
		if strings.HasPrefix(part, "v") {
			return strings.TrimPrefix(part, "v"), nil
		}
	}

	return output, nil
}

// isServiceRunning checks if the service is running
func (m *NATSModule) isServiceRunning(ctx context.Context, initSystem string) (bool, error) {
	var output string
	var err error

	switch initSystem {
	case "systemd":
		output, err = RunCommand(ctx, "systemctl", "is-active", "nats-server")
		return output == "active", err
	case "launchd":
		output, err = RunCommand(ctx, "launchctl", "list", "io.nats.nats-server")
		return err == nil, nil
	case "openrc":
		output, err = RunCommand(ctx, "rc-service", "nats-server", "status")
		return strings.Contains(output, "started"), err
	case "windows":
		output, err = RunCommand(ctx, "sc", "query", "nats-server")
		return strings.Contains(output, "RUNNING"), err
	default:
		output, err = RunCommand(ctx, "service", "nats-server", "status")
		return strings.Contains(strings.ToLower(output), "running"), err
	}
}

// isServiceEnabled checks if the service is enabled at boot
func (m *NATSModule) isServiceEnabled(ctx context.Context, initSystem string) (bool, error) {
	var output string
	var err error

	switch initSystem {
	case "systemd":
		output, err = RunCommand(ctx, "systemctl", "is-enabled", "nats-server")
		return output == "enabled", err
	case "openrc":
		output, err = RunCommand(ctx, "rc-update", "show", "default")
		return strings.Contains(output, "nats-server"), err
	case "windows":
		output, err = RunCommand(ctx, "sc", "qc", "nats-server")
		return strings.Contains(output, "AUTO_START"), err
	default:
		return true, nil
	}
}

// createSystemdService creates a systemd service file
func (m *NATSModule) createSystemdService(cfg *NATSConfig) error {
	binaryPath := cfg.BinaryPath
	if binaryPath == "" {
		binaryPath = GetDefaultBinaryPath(ComponentNATS)
	}

	configPath := cfg.ConfigPath
	if configPath == "" {
		configPath = GetDefaultConfigPath(ComponentNATS)
	}

	user := cfg.User
	if user == "" {
		user = "nats"
	}

	group := cfg.Group
	if group == "" {
		group = "nats"
	}

	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = GetDefaultDataDir(ComponentNATS)
	}

	logDir := cfg.LogDir
	if logDir == "" {
		logDir = "/var/log/nats"
	}

	tmpl := `[Unit]
Description=NATS Server
Documentation=https://docs.nats.io
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User={{.User}}
Group={{.Group}}
ExecStart={{.BinaryPath}} -c {{.ConfigPath}}
ExecReload=/bin/kill -s HUP $MAINPID
ExecStop=/bin/kill -s SIGINT $MAINPID
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

	servicePath := "/etc/systemd/system/nats-server.service"
	if err := WriteFile(servicePath, []byte(buf.String()), 0644); err != nil {
		return err
	}

	_, err = RunCommand(context.Background(), "systemctl", "daemon-reload")
	return err
}

// DefaultNATSConfig returns a default NATS configuration
func DefaultNATSConfig() *NATSConfig {
	return &NATSConfig{
		BaseConfig: BaseConfig{
			State:         StateRunning,
			Channel:       "stable",
			InstallMethod: InstallMethodPackage,
			ConfigPath:    GetDefaultConfigPath(ComponentNATS),
			BinaryPath:    GetDefaultBinaryPath(ComponentNATS),
			DataDir:       GetDefaultDataDir(ComponentNATS),
			LogDir:        "/var/log/nats",
			User:          "nats",
			Group:         "nats",
		},
		Mode:             "standalone",
		ClientPort:       4222,
		HTTPPort:         8222,
		JetStreamEnabled: true,
	}
}
