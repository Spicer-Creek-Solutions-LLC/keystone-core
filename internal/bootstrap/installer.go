package bootstrap

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// InstallerRegistry holds registered component installers
type InstallerRegistry struct {
	installers map[ComponentType]ComponentInstaller
}

// NewInstallerRegistry creates a new installer registry with default installers
func NewInstallerRegistry() *InstallerRegistry {
	return &InstallerRegistry{
		installers: make(map[ComponentType]ComponentInstaller),
	}
}

// Register adds an installer for a component type
func (r *InstallerRegistry) Register(componentType ComponentType, installer ComponentInstaller) {
	r.installers[componentType] = installer
}

// Get returns the installer for a component type
func (r *InstallerRegistry) Get(componentType ComponentType) (ComponentInstaller, bool) {
	installer, ok := r.installers[componentType]
	return installer, ok
}

// DefaultInstallerRegistry returns a registry with default installers
func DefaultInstallerRegistry() *InstallerRegistry {
	registry := NewInstallerRegistry()
	registry.Register(ComponentServer, NewServerInstaller())
	registry.Register(ComponentAgent, NewAgentInstaller())
	registry.Register(ComponentNATS, NewNATSInstaller())
	return registry
}

// BaseInstaller provides common installer functionality
type BaseInstaller struct {
	componentType ComponentType
	serviceName   string
	binaryName    string
	configDir     string
	dataDir       string
	logger        Logger
}

// SetLogger sets the logger for the installer
func (b *BaseInstaller) SetLogger(logger Logger) {
	b.logger = logger
}

func (b *BaseInstaller) log(level, msg string, args ...any) {
	if b.logger == nil {
		return
	}
	switch level {
	case "debug":
		b.logger.Debug(msg, args...)
	case "info":
		b.logger.Info(msg, args...)
	case "warn":
		b.logger.Warn(msg, args...)
	case "error":
		b.logger.Error(msg, args...)
	}
}

// runCommand runs a command and returns output
func (b *BaseInstaller) runCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("command failed: %s %v: %w (stderr: %s)", name, args, err, stderr.String())
	}

	return stdout.String(), nil
}

// fileExists checks if a file exists
func (b *BaseInstaller) fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ensureDir ensures a directory exists
func (b *BaseInstaller) ensureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// writeFile writes content to a file
func (b *BaseInstaller) writeFile(path string, content []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := b.ensureDir(dir); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	return os.WriteFile(path, content, mode)
}

// detectPackageManager detects the system package manager
func detectPackageManager() string {
	managers := []struct {
		name string
		cmd  string
	}{
		{"apt", "apt-get"},
		{"dnf", "dnf"},
		{"yum", "yum"},
		{"apk", "apk"},
		{"brew", "brew"},
		{"pacman", "pacman"},
	}

	for _, pm := range managers {
		if _, err := exec.LookPath(pm.cmd); err == nil {
			return pm.name
		}
	}

	return ""
}

// detectInitSystem detects the system init system
func detectInitSystem() string {
	// Check for systemd
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		return "systemd"
	}

	// Check for launchd (macOS)
	if runtime.GOOS == "darwin" {
		return "launchd"
	}

	// Check for openrc
	if _, err := exec.LookPath("rc-service"); err == nil {
		return "openrc"
	}

	// Check for sysvinit
	if _, err := os.Stat("/etc/init.d"); err == nil {
		return "sysvinit"
	}

	return "unknown"
}

// ServerInstaller installs kscore-server
type ServerInstaller struct {
	BaseInstaller
}

// NewServerInstaller creates a new server installer
func NewServerInstaller() *ServerInstaller {
	return &ServerInstaller{
		BaseInstaller: BaseInstaller{
			componentType: ComponentServer,
			serviceName:   "kscore-server",
			binaryName:    "kscore-server",
			configDir:     "/etc/kscore",
			dataDir:       "/var/lib/kscore",
		},
	}
}

// Install installs the kscore-server component
func (s *ServerInstaller) Install(ctx context.Context, config ComponentConfig) error {
	s.log("info", "Installing kscore-server", "version", config.Version)

	installerType := config.InstallerType
	if installerType == "" {
		installerType = s.detectInstallerType()
	}

	switch installerType {
	case InstallerPackage:
		return s.installPackage(ctx, config)
	case InstallerBinary:
		return s.installBinary(ctx, config)
	case InstallerContainer:
		return s.installContainer(ctx, config)
	default:
		return fmt.Errorf("unsupported installer type: %s", installerType)
	}
}

func (s *ServerInstaller) detectInstallerType() InstallerType {
	// Prefer package manager if available
	if pm := detectPackageManager(); pm != "" {
		return InstallerPackage
	}
	return InstallerBinary
}

func (s *ServerInstaller) installPackage(ctx context.Context, config ComponentConfig) error {
	pm := detectPackageManager()
	if pm == "" {
		return fmt.Errorf("no package manager found")
	}

	version := config.Version
	if version == "" {
		version = "latest"
	}

	var cmd string
	var args []string

	switch pm {
	case "apt":
		// First, add the repository if not present
		cmd = "apt-get"
		args = []string{"install", "-y", s.binaryName}
		if version != "latest" {
			args = []string{"install", "-y", fmt.Sprintf("%s=%s", s.binaryName, version)}
		}
	case "dnf", "yum":
		cmd = pm
		args = []string{"install", "-y", s.binaryName}
		if version != "latest" {
			args = []string{"install", "-y", fmt.Sprintf("%s-%s", s.binaryName, version)}
		}
	case "apk":
		cmd = "apk"
		args = []string{"add", s.binaryName}
	case "brew":
		cmd = "brew"
		args = []string{"install", s.binaryName}
	default:
		return fmt.Errorf("unsupported package manager: %s", pm)
	}

	_, err := s.runCommand(ctx, cmd, args...)
	return err
}

func (s *ServerInstaller) installBinary(ctx context.Context, config ComponentConfig) error {
	version := config.Version
	if version == "" {
		version = "latest"
	}

	// Determine download URL
	arch := runtime.GOARCH
	osName := runtime.GOOS

	url := fmt.Sprintf("https://releases.kscore.io/kscore-server/%s/%s_%s_%s",
		version, s.binaryName, osName, arch)

	// Download binary
	s.log("info", "Downloading kscore-server", "url", url)

	binPath := "/usr/local/bin/" + s.binaryName
	if err := s.downloadFile(ctx, url, binPath); err != nil {
		return fmt.Errorf("failed to download binary: %w", err)
	}

	// Make executable
	if err := os.Chmod(binPath, 0755); err != nil {
		return fmt.Errorf("failed to make binary executable: %w", err)
	}

	// Create systemd service file
	if detectInitSystem() == "systemd" {
		if err := s.createSystemdService(); err != nil {
			return fmt.Errorf("failed to create systemd service: %w", err)
		}
	}

	return nil
}

func (s *ServerInstaller) installContainer(ctx context.Context, config ComponentConfig) error {
	version := config.Version
	if version == "" {
		version = "latest"
	}

	image := fmt.Sprintf("ghcr.io/keystone-core/%s:%s", s.binaryName, version)

	// Check for docker or podman
	runtime := "docker"
	if _, err := exec.LookPath("docker"); err != nil {
		if _, err := exec.LookPath("podman"); err != nil {
			return fmt.Errorf("neither docker nor podman found")
		}
		runtime = "podman"
	}

	// Pull image
	s.log("info", "Pulling container image", "image", image)
	if _, err := s.runCommand(ctx, runtime, "pull", image); err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	return nil
}

func (s *ServerInstaller) downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func (s *ServerInstaller) createSystemdService() error {
	serviceContent := `[Unit]
Description=Keystone Core Server
Documentation=https://docs.kscore.io
After=network-online.target
Wants=network-online.target

[Service]
Type=notify
ExecStart=/usr/local/bin/kscore-server --config /etc/kscore/server.yaml
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5
LimitNOFILE=65536
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
`
	return s.writeFile("/etc/systemd/system/kscore-server.service", []byte(serviceContent), 0644)
}

// Uninstall removes the kscore-server component
func (s *ServerInstaller) Uninstall(ctx context.Context) error {
	s.log("info", "Uninstalling kscore-server")

	// Stop service first
	_ = s.Stop(ctx)

	// Remove binary
	os.Remove("/usr/local/bin/" + s.binaryName)

	// Remove systemd service
	os.Remove("/etc/systemd/system/kscore-server.service")

	// Reload systemd
	if detectInitSystem() == "systemd" {
		s.runCommand(ctx, "systemctl", "daemon-reload")
	}

	return nil
}

// Configure configures the kscore-server component
func (s *ServerInstaller) Configure(ctx context.Context, config ComponentConfig) error {
	s.log("info", "Configuring kscore-server")

	if err := s.ensureDir(s.configDir); err != nil {
		return err
	}

	if err := s.ensureDir(s.dataDir); err != nil {
		return err
	}

	// Write config file if settings provided
	if len(config.Settings) > 0 {
		configPath := filepath.Join(s.configDir, "server.yaml")
		// Generate config from settings - simplified for now
		configContent := "# Keystone Core Server Configuration\n"
		for k, v := range config.Settings {
			configContent += fmt.Sprintf("%s: %v\n", k, v)
		}
		if err := s.writeFile(configPath, []byte(configContent), 0644); err != nil {
			return fmt.Errorf("failed to write config: %w", err)
		}
	}

	return nil
}

// Start starts the kscore-server service
func (s *ServerInstaller) Start(ctx context.Context) error {
	s.log("info", "Starting kscore-server")

	initSystem := detectInitSystem()
	switch initSystem {
	case "systemd":
		if _, err := s.runCommand(ctx, "systemctl", "daemon-reload"); err != nil {
			return err
		}
		if _, err := s.runCommand(ctx, "systemctl", "enable", s.serviceName); err != nil {
			return err
		}
		_, err := s.runCommand(ctx, "systemctl", "start", s.serviceName)
		return err
	case "launchd":
		_, err := s.runCommand(ctx, "launchctl", "load", fmt.Sprintf("/Library/LaunchDaemons/io.kscore.%s.plist", s.serviceName))
		return err
	default:
		return fmt.Errorf("unsupported init system: %s", initSystem)
	}
}

// Stop stops the kscore-server service
func (s *ServerInstaller) Stop(ctx context.Context) error {
	s.log("info", "Stopping kscore-server")

	initSystem := detectInitSystem()
	switch initSystem {
	case "systemd":
		_, err := s.runCommand(ctx, "systemctl", "stop", s.serviceName)
		return err
	case "launchd":
		_, err := s.runCommand(ctx, "launchctl", "unload", fmt.Sprintf("/Library/LaunchDaemons/io.kscore.%s.plist", s.serviceName))
		return err
	default:
		return fmt.Errorf("unsupported init system: %s", initSystem)
	}
}

// Status returns the status of kscore-server
func (s *ServerInstaller) Status(ctx context.Context) (*ComponentStatus, error) {
	status := &ComponentStatus{
		Type: s.componentType,
	}

	// Check if binary exists
	binPath := "/usr/local/bin/" + s.binaryName
	status.Installed = s.fileExists(binPath)

	// Get version
	if status.Installed {
		version, err := s.Version(ctx)
		if err == nil {
			status.Version = version
		}
	}

	// Check if running
	initSystem := detectInitSystem()
	switch initSystem {
	case "systemd":
		output, err := s.runCommand(ctx, "systemctl", "is-active", s.serviceName)
		status.Running = err == nil && strings.TrimSpace(output) == "active"
	case "launchd":
		output, err := s.runCommand(ctx, "launchctl", "list")
		status.Running = err == nil && strings.Contains(output, s.serviceName)
	}

	// Check health
	if status.Running {
		status.Healthy = s.checkHealth(ctx)
	}

	return status, nil
}

func (s *ServerInstaller) checkHealth(ctx context.Context) bool {
	// Try to hit the health endpoint
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://localhost:8080/health/live")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Version returns the installed version
func (s *ServerInstaller) Version(ctx context.Context) (string, error) {
	output, err := s.runCommand(ctx, s.binaryName, "--version")
	if err != nil {
		return "", err
	}
	// Parse version from output
	parts := strings.Fields(output)
	if len(parts) >= 2 {
		return parts[1], nil
	}
	return strings.TrimSpace(output), nil
}

// AgentInstaller installs kscore-agent
type AgentInstaller struct {
	BaseInstaller
}

// NewAgentInstaller creates a new agent installer
func NewAgentInstaller() *AgentInstaller {
	return &AgentInstaller{
		BaseInstaller: BaseInstaller{
			componentType: ComponentAgent,
			serviceName:   "kscore-agent",
			binaryName:    "kscore-agent",
			configDir:     "/etc/kscore",
			dataDir:       "/var/lib/kscore",
		},
	}
}

// Install installs the kscore-agent component
func (a *AgentInstaller) Install(ctx context.Context, config ComponentConfig) error {
	a.log("info", "Installing kscore-agent", "version", config.Version)

	installerType := config.InstallerType
	if installerType == "" {
		if pm := detectPackageManager(); pm != "" {
			installerType = InstallerPackage
		} else {
			installerType = InstallerBinary
		}
	}

	switch installerType {
	case InstallerPackage:
		return a.installPackage(ctx, config)
	case InstallerBinary:
		return a.installBinary(ctx, config)
	default:
		return fmt.Errorf("unsupported installer type: %s", installerType)
	}
}

func (a *AgentInstaller) installPackage(ctx context.Context, config ComponentConfig) error {
	pm := detectPackageManager()
	if pm == "" {
		return fmt.Errorf("no package manager found")
	}

	var cmd string
	var args []string

	switch pm {
	case "apt":
		cmd = "apt-get"
		args = []string{"install", "-y", a.binaryName}
	case "dnf", "yum":
		cmd = pm
		args = []string{"install", "-y", a.binaryName}
	default:
		return fmt.Errorf("unsupported package manager: %s", pm)
	}

	_, err := a.runCommand(ctx, cmd, args...)
	return err
}

func (a *AgentInstaller) installBinary(ctx context.Context, config ComponentConfig) error {
	version := config.Version
	if version == "" {
		version = "latest"
	}

	arch := runtime.GOARCH
	osName := runtime.GOOS

	url := fmt.Sprintf("https://releases.kscore.io/kscore-agent/%s/%s_%s_%s",
		version, a.binaryName, osName, arch)

	binPath := "/usr/local/bin/" + a.binaryName
	if err := a.downloadFile(ctx, url, binPath); err != nil {
		return fmt.Errorf("failed to download binary: %w", err)
	}

	if err := os.Chmod(binPath, 0755); err != nil {
		return fmt.Errorf("failed to make binary executable: %w", err)
	}

	if detectInitSystem() == "systemd" {
		if err := a.createSystemdService(); err != nil {
			return fmt.Errorf("failed to create systemd service: %w", err)
		}
	}

	return nil
}

func (a *AgentInstaller) downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func (a *AgentInstaller) createSystemdService() error {
	serviceContent := `[Unit]
Description=Keystone Core Agent
Documentation=https://docs.kscore.io
After=network-online.target
Wants=network-online.target

[Service]
Type=notify
ExecStart=/usr/local/bin/kscore-agent --config /etc/kscore/agent.yaml
ExecReload=/bin/kill -HUP $MAINPID
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
`
	return a.writeFile("/etc/systemd/system/kscore-agent.service", []byte(serviceContent), 0644)
}

// Uninstall removes the kscore-agent component
func (a *AgentInstaller) Uninstall(ctx context.Context) error {
	a.log("info", "Uninstalling kscore-agent")
	_ = a.Stop(ctx)
	os.Remove("/usr/local/bin/" + a.binaryName)
	os.Remove("/etc/systemd/system/kscore-agent.service")
	if detectInitSystem() == "systemd" {
		a.runCommand(ctx, "systemctl", "daemon-reload")
	}
	return nil
}

// Configure configures the kscore-agent component
func (a *AgentInstaller) Configure(ctx context.Context, config ComponentConfig) error {
	a.log("info", "Configuring kscore-agent")

	if err := a.ensureDir(a.configDir); err != nil {
		return err
	}

	if len(config.Settings) > 0 {
		configPath := filepath.Join(a.configDir, "agent.yaml")
		configContent := "# Keystone Core Agent Configuration\n"
		for k, v := range config.Settings {
			configContent += fmt.Sprintf("%s: %v\n", k, v)
		}
		if err := a.writeFile(configPath, []byte(configContent), 0644); err != nil {
			return fmt.Errorf("failed to write config: %w", err)
		}
	}

	return nil
}

// Start starts the kscore-agent service
func (a *AgentInstaller) Start(ctx context.Context) error {
	a.log("info", "Starting kscore-agent")

	initSystem := detectInitSystem()
	switch initSystem {
	case "systemd":
		if _, err := a.runCommand(ctx, "systemctl", "daemon-reload"); err != nil {
			return err
		}
		if _, err := a.runCommand(ctx, "systemctl", "enable", a.serviceName); err != nil {
			return err
		}
		_, err := a.runCommand(ctx, "systemctl", "start", a.serviceName)
		return err
	default:
		return fmt.Errorf("unsupported init system: %s", initSystem)
	}
}

// Stop stops the kscore-agent service
func (a *AgentInstaller) Stop(ctx context.Context) error {
	a.log("info", "Stopping kscore-agent")

	initSystem := detectInitSystem()
	switch initSystem {
	case "systemd":
		_, err := a.runCommand(ctx, "systemctl", "stop", a.serviceName)
		return err
	default:
		return fmt.Errorf("unsupported init system: %s", initSystem)
	}
}

// Status returns the status of kscore-agent
func (a *AgentInstaller) Status(ctx context.Context) (*ComponentStatus, error) {
	status := &ComponentStatus{
		Type: a.componentType,
	}

	binPath := "/usr/local/bin/" + a.binaryName
	status.Installed = a.fileExists(binPath)

	if status.Installed {
		version, err := a.Version(ctx)
		if err == nil {
			status.Version = version
		}
	}

	initSystem := detectInitSystem()
	switch initSystem {
	case "systemd":
		output, err := a.runCommand(ctx, "systemctl", "is-active", a.serviceName)
		status.Running = err == nil && strings.TrimSpace(output) == "active"
	}

	status.Healthy = status.Running

	return status, nil
}

// Version returns the installed version
func (a *AgentInstaller) Version(ctx context.Context) (string, error) {
	output, err := a.runCommand(ctx, a.binaryName, "--version")
	if err != nil {
		return "", err
	}
	parts := strings.Fields(output)
	if len(parts) >= 2 {
		return parts[1], nil
	}
	return strings.TrimSpace(output), nil
}

// NATSInstaller installs NATS server
type NATSInstaller struct {
	BaseInstaller
}

// NewNATSInstaller creates a new NATS installer
func NewNATSInstaller() *NATSInstaller {
	return &NATSInstaller{
		BaseInstaller: BaseInstaller{
			componentType: ComponentNATS,
			serviceName:   "nats-server",
			binaryName:    "nats-server",
			configDir:     "/etc/nats",
			dataDir:       "/var/lib/nats",
		},
	}
}

// Install installs the NATS server component
func (n *NATSInstaller) Install(ctx context.Context, config ComponentConfig) error {
	n.log("info", "Installing nats-server", "version", config.Version)

	installerType := config.InstallerType
	if installerType == "" {
		if pm := detectPackageManager(); pm != "" {
			installerType = InstallerPackage
		} else {
			installerType = InstallerBinary
		}
	}

	switch installerType {
	case InstallerPackage:
		return n.installPackage(ctx, config)
	case InstallerBinary:
		return n.installBinary(ctx, config)
	default:
		return fmt.Errorf("unsupported installer type: %s", installerType)
	}
}

func (n *NATSInstaller) installPackage(ctx context.Context, config ComponentConfig) error {
	pm := detectPackageManager()

	switch pm {
	case "apt":
		// NATS not typically in apt, use binary
		return n.installBinary(ctx, config)
	case "brew":
		_, err := n.runCommand(ctx, "brew", "install", "nats-server")
		return err
	default:
		return n.installBinary(ctx, config)
	}
}

func (n *NATSInstaller) installBinary(ctx context.Context, config ComponentConfig) error {
	version := config.Version
	if version == "" {
		version = "2.10.0"
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

	url := fmt.Sprintf("https://github.com/nats-io/nats-server/releases/download/v%s/nats-server-v%s-%s-%s.tar.gz",
		version, version, osName, archName)

	n.log("info", "Downloading nats-server", "url", url)

	// Download and extract
	tmpDir, err := os.MkdirTemp("", "nats-install")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	tarPath := filepath.Join(tmpDir, "nats.tar.gz")
	if err := n.downloadFile(ctx, url, tarPath); err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}

	// Extract
	if _, err := n.runCommand(ctx, "tar", "-xzf", tarPath, "-C", tmpDir); err != nil {
		return fmt.Errorf("failed to extract: %w", err)
	}

	// Find and copy binary
	extractDir := filepath.Join(tmpDir, fmt.Sprintf("nats-server-v%s-%s-%s", version, osName, archName))
	srcBin := filepath.Join(extractDir, "nats-server")
	dstBin := "/usr/local/bin/nats-server"

	data, err := os.ReadFile(srcBin)
	if err != nil {
		return fmt.Errorf("failed to read binary: %w", err)
	}

	if err := os.WriteFile(dstBin, data, 0755); err != nil {
		return fmt.Errorf("failed to install binary: %w", err)
	}

	if detectInitSystem() == "systemd" {
		if err := n.createSystemdService(); err != nil {
			return fmt.Errorf("failed to create systemd service: %w", err)
		}
	}

	return nil
}

func (n *NATSInstaller) downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func (n *NATSInstaller) createSystemdService() error {
	serviceContent := `[Unit]
Description=NATS Server
Documentation=https://docs.nats.io
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/nats-server -c /etc/nats/nats.conf
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5
LimitNOFILE=800000

[Install]
WantedBy=multi-user.target
`
	return n.writeFile("/etc/systemd/system/nats-server.service", []byte(serviceContent), 0644)
}

// Uninstall removes the NATS server component
func (n *NATSInstaller) Uninstall(ctx context.Context) error {
	n.log("info", "Uninstalling nats-server")
	_ = n.Stop(ctx)
	os.Remove("/usr/local/bin/nats-server")
	os.Remove("/etc/systemd/system/nats-server.service")
	if detectInitSystem() == "systemd" {
		n.runCommand(ctx, "systemctl", "daemon-reload")
	}
	return nil
}

// Configure configures the NATS server component
func (n *NATSInstaller) Configure(ctx context.Context, config ComponentConfig) error {
	n.log("info", "Configuring nats-server")

	if err := n.ensureDir(n.configDir); err != nil {
		return err
	}

	if err := n.ensureDir(n.dataDir); err != nil {
		return err
	}

	// Default NATS configuration
	natsConfig := `# NATS Server Configuration
port: 4222
http_port: 8222

jetstream {
  store_dir: "/var/lib/nats/jetstream"
  max_memory_store: 1GB
  max_file_store: 10GB
}
`

	configPath := filepath.Join(n.configDir, "nats.conf")
	return n.writeFile(configPath, []byte(natsConfig), 0644)
}

// Start starts the NATS server service
func (n *NATSInstaller) Start(ctx context.Context) error {
	n.log("info", "Starting nats-server")

	initSystem := detectInitSystem()
	switch initSystem {
	case "systemd":
		if _, err := n.runCommand(ctx, "systemctl", "daemon-reload"); err != nil {
			return err
		}
		if _, err := n.runCommand(ctx, "systemctl", "enable", n.serviceName); err != nil {
			return err
		}
		_, err := n.runCommand(ctx, "systemctl", "start", n.serviceName)
		return err
	default:
		return fmt.Errorf("unsupported init system: %s", initSystem)
	}
}

// Stop stops the NATS server service
func (n *NATSInstaller) Stop(ctx context.Context) error {
	n.log("info", "Stopping nats-server")

	initSystem := detectInitSystem()
	switch initSystem {
	case "systemd":
		_, err := n.runCommand(ctx, "systemctl", "stop", n.serviceName)
		return err
	default:
		return fmt.Errorf("unsupported init system: %s", initSystem)
	}
}

// Status returns the status of NATS server
func (n *NATSInstaller) Status(ctx context.Context) (*ComponentStatus, error) {
	status := &ComponentStatus{
		Type: n.componentType,
	}

	binPath := "/usr/local/bin/" + n.binaryName
	status.Installed = n.fileExists(binPath)

	if status.Installed {
		version, err := n.Version(ctx)
		if err == nil {
			status.Version = version
		}
	}

	initSystem := detectInitSystem()
	switch initSystem {
	case "systemd":
		output, err := n.runCommand(ctx, "systemctl", "is-active", n.serviceName)
		status.Running = err == nil && strings.TrimSpace(output) == "active"
	}

	if status.Running {
		status.Healthy = n.checkHealth(ctx)
	}

	return status, nil
}

func (n *NATSInstaller) checkHealth(ctx context.Context) bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://localhost:8222/healthz")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Version returns the installed version
func (n *NATSInstaller) Version(ctx context.Context) (string, error) {
	output, err := n.runCommand(ctx, n.binaryName, "--version")
	if err != nil {
		return "", err
	}
	// NATS output: "nats-server: v2.10.0"
	parts := strings.Fields(output)
	for _, p := range parts {
		if strings.HasPrefix(p, "v") {
			return strings.TrimPrefix(p, "v"), nil
		}
	}
	return strings.TrimSpace(output), nil
}
