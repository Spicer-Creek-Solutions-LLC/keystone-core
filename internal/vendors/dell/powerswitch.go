package dell

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

// PowerSwitchAdapter implements VendorAdapter for Dell PowerSwitch (N-series) devices.
type PowerSwitchAdapter struct {
	vendors.BaseVendorAdapter
	sshAdapter *ssh.Adapter
	shell      *ssh.NetworkDeviceShell
	inEnable   bool
	inConfig   bool
}

// PowerSwitchConfig contains Dell PowerSwitch specific configuration.
type PowerSwitchConfig struct {
	*vendors.VendorConfig
}

// DefaultPowerSwitchConfig returns a default PowerSwitch configuration.
func DefaultPowerSwitchConfig() *PowerSwitchConfig {
	cfg := vendors.DefaultVendorConfig()
	cfg.EnablePrompt = "#"
	cfg.ConfigPrompt = "(config"
	return &PowerSwitchConfig{VendorConfig: cfg}
}

// NewPowerSwitchAdapter creates a new Dell PowerSwitch adapter.
func NewPowerSwitchAdapter(config *PowerSwitchConfig) *PowerSwitchAdapter {
	if config == nil {
		config = DefaultPowerSwitchConfig()
	}
	if config.VendorConfig == nil {
		config.VendorConfig = vendors.DefaultVendorConfig()
	}

	sshConfig := ssh.DefaultConfig()
	sshConfig.ConnectionConfig = protocols.DefaultConnectionConfig()
	sshConfig.Timeout = config.Timeout

	return &PowerSwitchAdapter{
		BaseVendorAdapter: vendors.BaseVendorAdapter{
			Config: config.VendorConfig,
		},
		sshAdapter: ssh.NewAdapter(sshConfig),
	}
}

// Vendor implements VendorAdapter.Vendor.
func (a *PowerSwitchAdapter) Vendor() vendors.VendorType {
	return vendors.VendorDellPowerSwitch
}

// Type implements ProtocolAdapter.Type.
func (a *PowerSwitchAdapter) Type() protocols.ProtocolType {
	return protocols.ProtocolSSH
}

// Connect implements ProtocolAdapter.Connect.
func (a *PowerSwitchAdapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.Device = device
	a.Credential = cred
	a.Protocol = a.sshAdapter

	if err := a.sshAdapter.Connect(ctx, device, cred); err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	shellConfig := &ssh.NetworkDeviceConfig{
		Vendor:         "dell_powerswitch",
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
		_, _ = a.runCommand(ctx, "terminal length 0")
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
func (a *PowerSwitchAdapter) Disconnect(ctx context.Context) error {
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
func (a *PowerSwitchAdapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
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
func (a *PowerSwitchAdapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
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
func (a *PowerSwitchAdapter) IsConnected() bool {
	return a.Connected && a.sshAdapter.IsConnected()
}

// Metrics implements ProtocolAdapter.Metrics.
func (a *PowerSwitchAdapter) Metrics() *protocols.AdapterMetrics {
	return a.sshAdapter.Metrics()
}

// GetConfig implements VendorAdapter.GetConfig.
func (a *PowerSwitchAdapter) GetConfig(ctx context.Context, section string) (string, error) {
	var cmd string
	switch section {
	case "":
		cmd = "show running-config"
	case "startup":
		cmd = "show startup-config"
	case "interface", "interfaces":
		cmd = "show interfaces status"
	case "vlan", "vlans":
		cmd = "show vlan"
	default:
		cmd = fmt.Sprintf("show running-config %s", section)
	}
	return a.runCommand(ctx, cmd)
}

// SetConfig implements VendorAdapter.SetConfig.
func (a *PowerSwitchAdapter) SetConfig(ctx context.Context, commands []string) error {
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
func (a *PowerSwitchAdapter) GetFacts(ctx context.Context) (*vendors.DeviceFacts, error) {
	facts := &vendors.DeviceFacts{
		Vendor: "Dell",
		OSType: "PowerSwitch",
		Raw:    make(map[string]string),
	}

	version, err := a.runCommand(ctx, "show version")
	if err != nil {
		return nil, fmt.Errorf("failed to get version: %w", err)
	}
	facts.Raw["show version"] = version
	a.parseVersionFacts(version, facts)

	hostname, err := a.runCommand(ctx, "show running-config | include hostname")
	if err == nil {
		if match := regexp.MustCompile(`hostname\s+(\S+)`).FindStringSubmatch(hostname); len(match) > 1 {
			facts.Hostname = match[1]
		}
	}

	intfs, err := a.runCommand(ctx, "show interfaces status")
	if err == nil {
		facts.Raw["show interfaces status"] = intfs
		facts.Interfaces = a.parseInterfaces(intfs)
	}

	return facts, nil
}

// SaveConfig implements VendorAdapter.SaveConfig.
func (a *PowerSwitchAdapter) SaveConfig(ctx context.Context) error {
	if a.inConfig {
		if err := a.exitConfig(ctx); err != nil {
			return err
		}
	}
	_, err := a.runCommand(ctx, "copy running-config startup-config")
	return err
}

func (a *PowerSwitchAdapter) enterEnable(ctx context.Context) error {
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

func (a *PowerSwitchAdapter) enterConfig(ctx context.Context) error {
	if !a.inEnable {
		if err := a.enterEnable(ctx); err != nil {
			return err
		}
	}

	if _, err := a.runCommand(ctx, "configure"); err != nil {
		return fmt.Errorf("failed to enter config mode: %w", err)
	}
	a.inConfig = true
	return nil
}

func (a *PowerSwitchAdapter) exitConfig(ctx context.Context) error {
	if !a.inConfig {
		return nil
	}
	if _, err := a.runCommand(ctx, "exit"); err != nil {
		return err
	}
	a.inConfig = false
	return nil
}

func (a *PowerSwitchAdapter) runCommand(ctx context.Context, command string) (string, error) {
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
		strings.Contains(lowerOutput, "% error") ||
		strings.Contains(lowerOutput, "command not found") {
		return output, fmt.Errorf("command error: %s", output)
	}
	return output, nil
}

func (a *PowerSwitchAdapter) parseVersionFacts(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "SW Version") || strings.Contains(line, "Software Version") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.OSVersion = strings.TrimSpace(parts[1])
			}
		}

		if strings.Contains(line, "Machine Type") || strings.Contains(line, "System Model") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Model = strings.TrimSpace(parts[1])
			}
		}

		if strings.Contains(line, "Serial Number") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.SerialNumber = strings.TrimSpace(parts[1])
			}
		}

		if strings.Contains(line, "uptime") || strings.Contains(line, "Up Time") {
			uptime := parseUptime(line)
			if uptime > 0 {
				facts.Uptime = uptime
			}
		}
	}
}

func (a *PowerSwitchAdapter) parseInterfaces(output string) []vendors.InterfaceFact {
	var interfaces []vendors.InterfaceFact
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Port") || strings.HasPrefix(line, "----") || strings.HasPrefix(line, "Interface") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		intf := vendors.InterfaceFact{
			Name:        fields[0],
			AdminStatus: strings.ToLower(fields[len(fields)-2]),
			OperStatus:  strings.ToLower(fields[len(fields)-1]),
		}
		interfaces = append(interfaces, intf)
	}
	return interfaces
}

// NewPowerSwitchAdapterFactory creates an adapter factory for Dell PowerSwitch.
func NewPowerSwitchAdapterFactory(config *PowerSwitchConfig) vendors.VendorAdapterFactory {
	return func(vendorConfig *vendors.VendorConfig) (vendors.VendorAdapter, error) {
		if config == nil {
			config = DefaultPowerSwitchConfig()
		}
		if vendorConfig != nil {
			config.VendorConfig = vendorConfig
		}
		return NewPowerSwitchAdapter(config), nil
	}
}

func init() {
	vendors.Register(vendors.VendorDellPowerSwitch, NewPowerSwitchAdapterFactory(nil))
}
