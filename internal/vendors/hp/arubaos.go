package hp

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/protocols/ssh"
	"github.com/shawnbutts/keystone-core/internal/proxy"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

// ArubaOSAdapter implements VendorAdapter for ArubaOS wireless controllers.
type ArubaOSAdapter struct {
	vendors.BaseVendorAdapter
	sshAdapter *ssh.Adapter
	shell      *ssh.NetworkDeviceShell
	inEnable   bool
	inConfig   bool
}

// ArubaOSConfig contains ArubaOS specific configuration.
type ArubaOSConfig struct {
	*vendors.VendorConfig
}

// DefaultArubaOSConfig returns a default ArubaOS configuration.
func DefaultArubaOSConfig() *ArubaOSConfig {
	cfg := vendors.DefaultVendorConfig()
	cfg.EnablePrompt = "#"
	cfg.ConfigPrompt = "(config)"
	return &ArubaOSConfig{VendorConfig: cfg}
}

// NewArubaOSAdapter creates a new ArubaOS adapter.
func NewArubaOSAdapter(config *ArubaOSConfig) *ArubaOSAdapter {
	if config == nil {
		config = DefaultArubaOSConfig()
	}
	if config.VendorConfig == nil {
		config.VendorConfig = vendors.DefaultVendorConfig()
	}

	sshConfig := ssh.DefaultConfig()
	sshConfig.ConnectionConfig = protocols.DefaultConnectionConfig()
	sshConfig.Timeout = config.Timeout

	return &ArubaOSAdapter{
		BaseVendorAdapter: vendors.BaseVendorAdapter{
			Config: config.VendorConfig,
		},
		sshAdapter: ssh.NewAdapter(sshConfig),
	}
}

// Vendor implements VendorAdapter.Vendor.
func (a *ArubaOSAdapter) Vendor() vendors.VendorType {
	return vendors.VendorHPArubaOS
}

// Type implements ProtocolAdapter.Type.
func (a *ArubaOSAdapter) Type() protocols.ProtocolType {
	return protocols.ProtocolSSH
}

// Connect implements ProtocolAdapter.Connect.
func (a *ArubaOSAdapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.Device = device
	a.Credential = cred
	a.Protocol = a.sshAdapter

	if err := a.sshAdapter.Connect(ctx, device, cred); err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	shellConfig := &ssh.NetworkDeviceConfig{
		Vendor:         "hp_arubaos",
		Prompts:        []string{">", "#", "(config)"},
		EnableCmd:      "enable",
		EnablePassword: a.Config.EnablePassword,
	}
	shell, err := a.sshAdapter.NewNetworkDeviceShell(ctx, shellConfig)
	if err != nil {
		_ = a.sshAdapter.Disconnect(ctx)
		return fmt.Errorf("failed to create shell: %w", err)
	}
	a.shell = shell

	if a.Config.DisablePaging {
		_, _ = a.runCommand(ctx, "no paging")
	}

	if a.Config.EnablePassword != "" || a.Config.PrivilegeLevel > 0 {
		if err := a.enterEnable(ctx); err != nil {
			return fmt.Errorf("failed to enter enable mode: %w", err)
		}
	}

	a.Connected = true
	return nil
}

// Disconnect implements ProtocolAdapter.Disconnect.
func (a *ArubaOSAdapter) Disconnect(ctx context.Context) error {
	a.Connected = false
	a.inEnable = false
	a.inConfig = false

	if a.shell != nil {
		a.shell.Close()
		a.shell = nil
	}

	if a.sshAdapter != nil {
		return a.sshAdapter.Disconnect(ctx)
	}
	return nil
}

// Execute implements ProtocolAdapter.Execute.
func (a *ArubaOSAdapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
	start := time.Now()
	result := &protocols.ExecuteResult{StartTime: start}

	output, err := a.runCommand(ctx, req.Command)
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(start)

	if err != nil {
		result.Error = err.Error()
		result.ExitCode = 1
		return result, err
	}

	result.Stdout = []byte(output)
	result.ExitCode = 0
	return result, nil
}

// HealthCheck implements ProtocolAdapter.HealthCheck.
func (a *ArubaOSAdapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
	result := &protocols.HealthCheckResult{
		LastCheck: time.Now(),
		Details:   make(map[string]interface{}),
	}

	if !a.Connected || a.shell == nil {
		result.Healthy = false
		result.Status = "not connected"
		return result, nil
	}

	start := time.Now()
	output, err := a.runCommand(ctx, "show version")
	result.Latency = time.Since(start)

	if err != nil {
		result.Healthy = false
		result.Status = fmt.Sprintf("health check failed: %v", err)
		return result, nil
	}

	result.Healthy = true
	result.Status = "connected"
	result.Details["version"] = strings.TrimSpace(output)
	return result, nil
}

// IsConnected implements ProtocolAdapter.IsConnected.
func (a *ArubaOSAdapter) IsConnected() bool {
	return a.Connected && a.sshAdapter.IsConnected()
}

// Metrics implements ProtocolAdapter.Metrics.
func (a *ArubaOSAdapter) Metrics() *protocols.AdapterMetrics {
	return a.sshAdapter.Metrics()
}

// GetConfig implements VendorAdapter.GetConfig.
func (a *ArubaOSAdapter) GetConfig(ctx context.Context, section string) (string, error) {
	var cmd string
	switch section {
	case "":
		cmd = "show running-config"
	case "startup":
		cmd = "show startup-config"
	case "interface", "interfaces":
		cmd = "show running-config | include interface"
	case "ap":
		cmd = "show ap database"
	default:
		cmd = fmt.Sprintf("show running-config | include %s", section)
	}
	return a.runCommand(ctx, cmd)
}

// SetConfig implements VendorAdapter.SetConfig.
func (a *ArubaOSAdapter) SetConfig(ctx context.Context, commands []string) error {
	if !a.inConfig {
		if err := a.enterConfig(ctx); err != nil {
			return err
		}
		defer func() { _ = a.exitConfig(ctx) }()
	}

	for _, cmd := range commands {
		if _, err := a.runCommand(ctx, cmd); err != nil {
			return fmt.Errorf("command failed '%s': %w", cmd, err)
		}
	}
	return nil
}

// GetFacts implements VendorAdapter.GetFacts.
func (a *ArubaOSAdapter) GetFacts(ctx context.Context) (*vendors.DeviceFacts, error) {
	facts := &vendors.DeviceFacts{
		Vendor: "HP/Aruba",
		OSType: "ArubaOS",
		Raw:    make(map[string]string),
	}

	version, err := a.runCommand(ctx, "show version")
	if err != nil {
		return nil, fmt.Errorf("failed to get version: %w", err)
	}
	facts.Raw["show version"] = version
	a.parseVersionFacts(version, facts)

	switches, err := a.runCommand(ctx, "show switches")
	if err == nil {
		facts.Raw["show switches"] = switches
	}

	return facts, nil
}

// SaveConfig implements VendorAdapter.SaveConfig.
func (a *ArubaOSAdapter) SaveConfig(ctx context.Context) error {
	if a.inConfig {
		if err := a.exitConfig(ctx); err != nil {
			return err
		}
	}
	_, err := a.runCommand(ctx, "write memory")
	return err
}

func (a *ArubaOSAdapter) enterEnable(ctx context.Context) error {
	if a.inEnable {
		return nil
	}

	output, err := a.runCommand(ctx, "enable")
	if err != nil {
		return err
	}

	if strings.Contains(strings.ToLower(output), "password") {
		password := a.Config.EnablePassword
		if password == "" {
			if sshCred, ok := a.Credential.(*credentials.SSHPasswordCredential); ok {
				password = sshCred.Password
			}
		}
		if _, err := a.runCommand(ctx, password); err != nil {
			return fmt.Errorf("enable authentication failed: %w", err)
		}
	}

	a.inEnable = true
	return nil
}

func (a *ArubaOSAdapter) enterConfig(ctx context.Context) error {
	if !a.inEnable {
		if err := a.enterEnable(ctx); err != nil {
			return err
		}
	}

	if _, err := a.runCommand(ctx, "configure terminal"); err != nil {
		return fmt.Errorf("failed to enter config mode: %w", err)
	}
	a.inConfig = true
	return nil
}

func (a *ArubaOSAdapter) exitConfig(ctx context.Context) error {
	if !a.inConfig {
		return nil
	}
	if _, err := a.runCommand(ctx, "end"); err != nil {
		return err
	}
	a.inConfig = false
	return nil
}

func (a *ArubaOSAdapter) runCommand(ctx context.Context, command string) (string, error) {
	if a.shell == nil {
		return "", fmt.Errorf("shell not initialized")
	}

	result, err := a.shell.Execute(ctx, command)
	if err != nil {
		return "", err
	}

	output := result.Output
	lowerOutput := strings.ToLower(output)
	if strings.Contains(lowerOutput, "% invalid input") ||
		strings.Contains(lowerOutput, "% ambiguous command") ||
		strings.Contains(lowerOutput, "command is not recognized") {
		return output, fmt.Errorf("command error: %s", output)
	}
	return output, nil
}

func (a *ArubaOSAdapter) parseVersionFacts(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "ArubaOS") {
			if match := regexp.MustCompile(`ArubaOS\s+\(MODEL:\s*(\S+)\)`).FindStringSubmatch(line); len(match) > 1 {
				facts.Model = match[1]
			}
			if match := regexp.MustCompile(`Version\s+(\S+)`).FindStringSubmatch(line); len(match) > 1 {
				facts.OSVersion = match[1]
			}
		}

		if strings.Contains(line, "Serial Number") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.SerialNumber = strings.TrimSpace(parts[1])
			}
		}

		if strings.Contains(line, "uptime") {
			uptime := parseUptime(line)
			if uptime > 0 {
				facts.Uptime = uptime
			}
		}

		if strings.HasPrefix(line, "Hostname") || strings.HasPrefix(line, "System Name") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Hostname = strings.TrimSpace(parts[1])
			}
		}
	}
}

// NewArubaOSAdapterFactory creates an adapter factory for ArubaOS.
func NewArubaOSAdapterFactory(config *ArubaOSConfig) vendors.VendorAdapterFactory {
	return func(vendorConfig *vendors.VendorConfig) (vendors.VendorAdapter, error) {
		if config == nil {
			config = DefaultArubaOSConfig()
		}
		if vendorConfig != nil {
			config.VendorConfig = vendorConfig
		}
		return NewArubaOSAdapter(config), nil
	}
}

// GetAPDatabase retrieves the access point database.
func (a *ArubaOSAdapter) GetAPDatabase(ctx context.Context) ([]APInfo, error) {
	output, err := a.runCommand(ctx, "show ap database")
	if err != nil {
		return nil, err
	}
	return a.parseAPDatabase(output), nil
}

// APInfo contains access point information.
type APInfo struct {
	Name   string `json:"name"`
	Group  string `json:"group"`
	Model  string `json:"model"`
	IP     string `json:"ip"`
	Status string `json:"status"`
}

func (a *ArubaOSAdapter) parseAPDatabase(output string) []APInfo {
	var aps []APInfo
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Name") || strings.HasPrefix(line, "----") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		aps = append(aps, APInfo{
			Name:   fields[0],
			Group:  fields[1],
			Model:  fields[2],
			IP:     fields[3],
			Status: fields[4],
		})
	}
	return aps
}

func init() {
	vendors.Register(vendors.VendorHPArubaOS, NewArubaOSAdapterFactory(nil))
}
