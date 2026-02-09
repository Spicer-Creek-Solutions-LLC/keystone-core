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

// AgentModule manages kscore-agent installation and configuration
type AgentModule struct {
	logger Logger
}

// NewAgentModule creates a new agent module
func NewAgentModule(logger Logger) *AgentModule {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &AgentModule{logger: logger}
}

// Name returns the module name
func (m *AgentModule) Name() string {
	return "kscore_agent"
}

// ComponentType returns the component type
func (m *AgentModule) ComponentType() ComponentType {
	return ComponentAgent
}

// ValidStates returns valid states
func (m *AgentModule) ValidStates() []ComponentState {
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

// Check checks the current state of kscore-agent
func (m *AgentModule) Check(ctx context.Context, config interface{}) (*CheckResult, error) {
	cfg, ok := config.(*AgentConfig)
	if !ok {
		return nil, fmt.Errorf("invalid config type: expected *AgentConfig")
	}

	result := &CheckResult{
		Component:    ComponentAgent,
		DesiredState: cfg.State,
		Diff:         make(map[string]interface{}),
		Metadata:     make(map[string]interface{}),
	}

	// Check if binary exists
	binaryPath := cfg.BinaryPath
	if binaryPath == "" {
		binaryPath = GetDefaultBinaryPath(ComponentAgent)
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
		configPath = GetDefaultConfigPath(ComponentAgent)
	}
	result.ConfigPath = configPath
	result.ConfigValid = FileExists(configPath)
	result.Metadata["config_path"] = configPath

	// Check labels
	if len(cfg.Labels) > 0 {
		result.Metadata["labels"] = cfg.Labels
	}

	// Determine current state
	switch {
	case !result.Present:
		result.CurrentState = StateUninstalled
	case result.Running:
		result.CurrentState = StateRunning
	default:
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
func (m *AgentModule) Apply(ctx context.Context, config interface{}, dryRun bool) (*ApplyResult, error) {
	cfg, ok := config.(*AgentConfig)
	if !ok {
		return nil, fmt.Errorf("invalid config type: expected *AgentConfig")
	}

	startTime := time.Now()
	result := &ApplyResult{
		Component: ComponentAgent,
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
func (m *AgentModule) Validate(config interface{}) error {
	cfg, ok := config.(*AgentConfig)
	if !ok {
		return fmt.Errorf("invalid config type: expected *AgentConfig")
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

	// Validate server connection
	if cfg.ServerURL == "" && len(cfg.NATSURLs) == 0 {
		errors.Add("connection", "either server_url or nats_urls must be specified")
	}

	if errors.HasErrors() {
		return errors
	}
	return nil
}

// install installs kscore-agent
func (m *AgentModule) install(ctx context.Context, cfg *AgentConfig, result *ApplyResult) error {
	m.logger.Info("Installing kscore-agent", "version", cfg.Version, "method", cfg.InstallMethod)

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

func (m *AgentModule) installViaPackage(ctx context.Context, cfg *AgentConfig, result *ApplyResult) error {
	pm := DetectPackageManager()
	packageName := "kscore-agent"

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

func (m *AgentModule) installViaBinary(ctx context.Context, cfg *AgentConfig, result *ApplyResult) error {
	version := cfg.Version
	if version == "" {
		version = "latest"
	}

	arch := runtime.GOARCH
	osName := runtime.GOOS

	downloadURL := fmt.Sprintf("https://github.com/shawnbutts/keystone-core/releases/download/%s/kscore-agent-%s-%s", version, osName, arch)
	if osName == "windows" {
		downloadURL += ".exe"
	}

	binaryPath := cfg.BinaryPath
	if binaryPath == "" {
		binaryPath = GetDefaultBinaryPath(ComponentAgent)
	}

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

	if runtime.GOOS != "windows" {
		if err := os.Chmod(binaryPath, 0o755); err != nil { //nolint:gosec // G302: Binary must be executable by all users
			return fmt.Errorf("failed to make binary executable: %w", err)
		}
	}

	if runtime.GOOS == "linux" && DetectInitSystem() == "systemd" {
		if err := m.createSystemdService(cfg); err != nil { //nolint:contextcheck // createSystemdService doesn't take context
			return fmt.Errorf("failed to create systemd service: %w", err)
		}
	}

	result.Changes["installed"] = true
	result.Changes["install_method"] = "binary"
	result.Changes["binary_path"] = binaryPath
	return nil
}

func (m *AgentModule) installViaDocker(ctx context.Context, cfg *AgentConfig, result *ApplyResult) error {
	version := cfg.Version
	if version == "" {
		version = "latest"
	}

	image := fmt.Sprintf("ghcr.io/shawnbutts/keystone-core/kscore-agent:%s", version)

	output, err := RunCommand(ctx, "docker", "pull", image)
	if err != nil {
		return fmt.Errorf("docker pull failed: %w\nOutput: %s", err, output)
	}

	result.Changes["installed"] = true
	result.Changes["install_method"] = "docker"
	result.Changes["image"] = image
	return nil
}

// uninstall uninstalls kscore-agent
func (m *AgentModule) uninstall(ctx context.Context, cfg *AgentConfig, result *ApplyResult) error {
	m.logger.Info("Uninstalling kscore-agent")

	_ = m.stop(ctx, cfg, result)
	_ = m.disable(ctx, cfg, result)

	pm := DetectPackageManager()
	packageName := "kscore-agent"

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
		binaryPath := cfg.BinaryPath
		if binaryPath == "" {
			binaryPath = GetDefaultBinaryPath(ComponentAgent)
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
func (m *AgentModule) start(ctx context.Context, cfg *AgentConfig, result *ApplyResult) error {
	m.logger.Info("Starting kscore-agent")

	initSystem := DetectInitSystem()

	var output string
	var err error

	switch initSystem {
	case "systemd":
		output, err = RunCommand(ctx, "systemctl", "start", "kscore-agent")
	case "launchd":
		output, err = RunCommand(ctx, "launchctl", "load", "-w", "/Library/LaunchDaemons/com.keystone.kscore-agent.plist")
	case "openrc":
		output, err = RunCommand(ctx, "rc-service", "kscore-agent", "start")
	case "windows":
		output, err = RunCommand(ctx, "sc", "start", "kscore-agent")
	default:
		output, err = RunCommand(ctx, "service", "kscore-agent", "start")
	}

	if err != nil {
		return fmt.Errorf("failed to start service: %w\nOutput: %s", err, output)
	}

	result.Changes["started"] = true
	result.RestartedServices = append(result.RestartedServices, "kscore-agent")
	return nil
}

// stop stops the service
func (m *AgentModule) stop(ctx context.Context, cfg *AgentConfig, result *ApplyResult) error {
	m.logger.Info("Stopping kscore-agent")

	initSystem := DetectInitSystem()

	var output string
	var err error

	switch initSystem {
	case "systemd":
		output, err = RunCommand(ctx, "systemctl", "stop", "kscore-agent")
	case "launchd":
		output, err = RunCommand(ctx, "launchctl", "unload", "/Library/LaunchDaemons/com.keystone.kscore-agent.plist")
	case "openrc":
		output, err = RunCommand(ctx, "rc-service", "kscore-agent", "stop")
	case "windows":
		output, err = RunCommand(ctx, "sc", "stop", "kscore-agent")
	default:
		output, err = RunCommand(ctx, "service", "kscore-agent", "stop")
	}

	if err != nil {
		return fmt.Errorf("failed to stop service: %w\nOutput: %s", err, output)
	}

	result.Changes["stopped"] = true
	return nil
}

// enable enables the service at boot
func (m *AgentModule) enable(ctx context.Context, cfg *AgentConfig, result *ApplyResult) error {
	m.logger.Info("Enabling kscore-agent")

	initSystem := DetectInitSystem()

	var output string
	var err error

	switch initSystem {
	case "systemd":
		output, err = RunCommand(ctx, "systemctl", "enable", "kscore-agent")
	case "openrc":
		output, err = RunCommand(ctx, "rc-update", "add", "kscore-agent", "default")
	case "windows":
		output, err = RunCommand(ctx, "sc", "config", "kscore-agent", "start=", "auto")
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
func (m *AgentModule) disable(ctx context.Context, cfg *AgentConfig, result *ApplyResult) error {
	m.logger.Info("Disabling kscore-agent")

	initSystem := DetectInitSystem()

	var output string
	var err error

	switch initSystem {
	case "systemd":
		output, err = RunCommand(ctx, "systemctl", "disable", "kscore-agent")
	case "openrc":
		output, err = RunCommand(ctx, "rc-update", "del", "kscore-agent", "default")
	case "windows":
		output, err = RunCommand(ctx, "sc", "config", "kscore-agent", "start=", "disabled")
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
func (m *AgentModule) configure(ctx context.Context, cfg *AgentConfig, result *ApplyResult) error {
	m.logger.Info("Configuring kscore-agent")

	configPath := cfg.ConfigPath
	if configPath == "" {
		configPath = GetDefaultConfigPath(ComponentAgent)
	}

	configData := m.buildConfig(cfg)

	data, err := yaml.Marshal(configData)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := WriteFile(configPath, data, 0o640); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	result.Changes["configured"] = true
	result.Changes["config_path"] = configPath
	result.RequiresRestart = true
	return nil
}

// buildConfig builds the agent configuration
func (m *AgentModule) buildConfig(cfg *AgentConfig) map[string]interface{} {
	config := make(map[string]interface{})

	if cfg.AgentID != "" {
		config["agent_id"] = cfg.AgentID
	}

	// Server connection
	if cfg.ServerURL != "" {
		config["server_url"] = cfg.ServerURL
	}

	// NATS configuration
	if len(cfg.NATSURLs) > 0 {
		config["nats_urls"] = cfg.NATSURLs
	}

	// Labels
	if len(cfg.Labels) > 0 {
		config["labels"] = cfg.Labels
	}

	// Heartbeat
	if cfg.HeartbeatInterval > 0 {
		config["heartbeat_interval"] = cfg.HeartbeatInterval.String()
	}

	// TLS
	if cfg.TLSEnabled {
		config["tls"] = map[string]interface{}{
			"enabled":   true,
			"cert_file": cfg.TLSCertFile,
			"key_file":  cfg.TLSKeyFile,
			"ca_file":   cfg.TLSCAFile,
		}
	}

	// Embedded NATS
	if cfg.EmbeddedNATS {
		config["embedded_nats"] = true
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
func (m *AgentModule) getInstalledVersion(ctx context.Context, binaryPath string) (string, error) {
	output, err := RunCommand(ctx, binaryPath, "version")
	if err != nil {
		return "", err
	}

	parts := strings.Fields(output)
	for i, part := range parts {
		if part == "version" && i+1 < len(parts) {
			return parts[i+1], nil
		}
	}

	for _, part := range parts {
		if strings.HasPrefix(part, "v") || strings.Contains(part, ".") {
			return strings.TrimPrefix(part, "v"), nil
		}
	}

	return output, nil
}

// isServiceRunning checks if the service is running
func (m *AgentModule) isServiceRunning(ctx context.Context, initSystem string) (bool, error) {
	var output string
	var err error

	switch initSystem {
	case "systemd":
		output, err = RunCommand(ctx, "systemctl", "is-active", "kscore-agent")
		return output == "active", err
	case "launchd":
		_, err = RunCommand(ctx, "launchctl", "list", "com.keystone.kscore-agent")
		return err == nil, nil
	case "openrc":
		output, err = RunCommand(ctx, "rc-service", "kscore-agent", "status")
		return strings.Contains(output, "started"), err
	case "windows":
		output, err = RunCommand(ctx, "sc", "query", "kscore-agent")
		return strings.Contains(output, "RUNNING"), err
	default:
		output, err = RunCommand(ctx, "service", "kscore-agent", "status")
		return strings.Contains(strings.ToLower(output), "running"), err
	}
}

// isServiceEnabled checks if the service is enabled at boot
func (m *AgentModule) isServiceEnabled(ctx context.Context, initSystem string) (bool, error) {
	var output string
	var err error

	switch initSystem {
	case "systemd":
		output, err = RunCommand(ctx, "systemctl", "is-enabled", "kscore-agent")
		return output == "enabled", err
	case "openrc":
		output, err = RunCommand(ctx, "rc-update", "show", "default")
		return strings.Contains(output, "kscore-agent"), err
	case "windows":
		output, err = RunCommand(ctx, "sc", "qc", "kscore-agent")
		return strings.Contains(output, "AUTO_START"), err
	default:
		return true, nil
	}
}

// createSystemdService creates a systemd service file
func (m *AgentModule) createSystemdService(cfg *AgentConfig) error {
	binaryPath := cfg.BinaryPath
	if binaryPath == "" {
		binaryPath = GetDefaultBinaryPath(ComponentAgent)
	}

	configPath := cfg.ConfigPath
	if configPath == "" {
		configPath = GetDefaultConfigPath(ComponentAgent)
	}

	user := cfg.User
	if user == "" {
		user = "root" // Agent often needs root for state management
	}

	group := cfg.Group
	if group == "" {
		group = "root"
	}

	tmpl := `[Unit]
Description=Keystone Core Agent
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

# Note: Agent needs more permissions for state management
NoNewPrivileges=false
ProtectSystem=false
ProtectHome=false

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
	})
	if err != nil {
		return err
	}

	servicePath := "/etc/systemd/system/kscore-agent.service"
	if err := WriteFile(servicePath, []byte(buf.String()), 0o644); err != nil {
		return err
	}

	_, err = RunCommand(context.Background(), "systemctl", "daemon-reload")
	return err
}

// DefaultAgentConfig returns a default agent configuration
func DefaultAgentConfig() *AgentConfig {
	return &AgentConfig{
		BaseConfig: BaseConfig{
			State:         StateRunning,
			Channel:       "stable",
			InstallMethod: InstallMethodPackage,
			ConfigPath:    GetDefaultConfigPath(ComponentAgent),
			BinaryPath:    GetDefaultBinaryPath(ComponentAgent),
			DataDir:       GetDefaultDataDir(ComponentAgent),
			LogDir:        filepath.Join(GetDefaultDataDir(ComponentAgent), "..", "log"),
			User:          "root",
			Group:         "root",
		},
		HeartbeatInterval: 30 * time.Second,
	}
}
