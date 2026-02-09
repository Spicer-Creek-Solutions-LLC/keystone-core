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

// AOSCXAdapter implements VendorAdapter for Aruba AOS-CX switches.
type AOSCXAdapter struct {
	vendors.BaseVendorAdapter
	sshAdapter *ssh.Adapter
	shell      *ssh.NetworkDeviceShell
	inConfig   bool
}

// AOSCXConfig contains AOS-CX specific configuration.
type AOSCXConfig struct {
	*vendors.VendorConfig
}

// DefaultAOSCXConfig returns a default AOS-CX configuration.
func DefaultAOSCXConfig() *AOSCXConfig {
	cfg := vendors.DefaultVendorConfig()
	cfg.EnablePrompt = "#"
	cfg.ConfigPrompt = "(config)"
	return &AOSCXConfig{VendorConfig: cfg}
}

// NewAOSCXAdapter creates a new AOS-CX adapter.
func NewAOSCXAdapter(config *AOSCXConfig) *AOSCXAdapter {
	if config == nil {
		config = DefaultAOSCXConfig()
	}
	if config.VendorConfig == nil {
		config.VendorConfig = vendors.DefaultVendorConfig()
	}

	sshConfig := ssh.DefaultConfig()
	sshConfig.ConnectionConfig = protocols.DefaultConnectionConfig()
	sshConfig.Timeout = config.Timeout

	return &AOSCXAdapter{
		BaseVendorAdapter: vendors.BaseVendorAdapter{
			Config: config.VendorConfig,
		},
		sshAdapter: ssh.NewAdapter(sshConfig),
	}
}

// Vendor implements VendorAdapter.Vendor.
func (a *AOSCXAdapter) Vendor() vendors.VendorType {
	return vendors.VendorHPAOSCX
}

// Type implements ProtocolAdapter.Type.
func (a *AOSCXAdapter) Type() protocols.ProtocolType {
	return protocols.ProtocolSSH
}

// Connect implements ProtocolAdapter.Connect.
func (a *AOSCXAdapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.Device = device
	a.Credential = cred
	a.Protocol = a.sshAdapter

	if err := a.sshAdapter.Connect(ctx, device, cred); err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	shellConfig := &ssh.NetworkDeviceConfig{
		Vendor:  "hp_aoscx",
		Prompts: []string{">", "#", "(config)"},
	}
	shell, err := a.sshAdapter.NewNetworkDeviceShell(ctx, shellConfig)
	if err != nil {
		_ = a.sshAdapter.Disconnect(ctx)
		return fmt.Errorf("failed to create shell: %w", err)
	}
	a.shell = shell

	if a.Config.DisablePaging {
		_, _ = a.runCommand(ctx, "no page")
	}

	a.Connected = true
	return nil
}

// Disconnect implements ProtocolAdapter.Disconnect.
func (a *AOSCXAdapter) Disconnect(ctx context.Context) error {
	a.Connected = false
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
func (a *AOSCXAdapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
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
func (a *AOSCXAdapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
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
func (a *AOSCXAdapter) IsConnected() bool {
	return a.Connected && a.sshAdapter.IsConnected()
}

// Metrics implements ProtocolAdapter.Metrics.
func (a *AOSCXAdapter) Metrics() *protocols.AdapterMetrics {
	return a.sshAdapter.Metrics()
}

// GetConfig implements VendorAdapter.GetConfig.
func (a *AOSCXAdapter) GetConfig(ctx context.Context, section string) (string, error) {
	var cmd string
	switch section {
	case "":
		cmd = "show running-config"
	case "startup":
		cmd = "show startup-config"
	case "interface", "interfaces":
		cmd = "show interface brief"
	case "vlan", "vlans":
		cmd = "show vlan"
	default:
		cmd = fmt.Sprintf("show running-config %s", section)
	}
	return a.runCommand(ctx, cmd)
}

// SetConfig implements VendorAdapter.SetConfig.
func (a *AOSCXAdapter) SetConfig(ctx context.Context, commands []string) error {
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
func (a *AOSCXAdapter) GetFacts(ctx context.Context) (*vendors.DeviceFacts, error) {
	facts := &vendors.DeviceFacts{
		Vendor: "Aruba",
		OSType: "AOS-CX",
		Raw:    make(map[string]string),
	}

	version, err := a.runCommand(ctx, "show version")
	if err != nil {
		return nil, fmt.Errorf("failed to get version: %w", err)
	}
	facts.Raw["show version"] = version
	a.parseVersionFacts(version, facts)

	system, err := a.runCommand(ctx, "show system")
	if err == nil {
		facts.Raw["show system"] = system
	}

	intfs, err := a.runCommand(ctx, "show interface brief")
	if err == nil {
		facts.Raw["show interface brief"] = intfs
		facts.Interfaces = a.parseInterfaces(intfs)
	}

	return facts, nil
}

// SaveConfig implements VendorAdapter.SaveConfig.
func (a *AOSCXAdapter) SaveConfig(ctx context.Context) error {
	if a.inConfig {
		if err := a.exitConfig(ctx); err != nil {
			return err
		}
	}
	_, err := a.runCommand(ctx, "copy running-config startup-config")
	return err
}

func (a *AOSCXAdapter) enterConfig(ctx context.Context) error {
	if _, err := a.runCommand(ctx, "configure terminal"); err != nil {
		return fmt.Errorf("failed to enter config mode: %w", err)
	}
	a.inConfig = true
	return nil
}

func (a *AOSCXAdapter) exitConfig(ctx context.Context) error {
	if !a.inConfig {
		return nil
	}
	if _, err := a.runCommand(ctx, "end"); err != nil {
		return err
	}
	a.inConfig = false
	return nil
}

func (a *AOSCXAdapter) runCommand(ctx context.Context, command string) (string, error) {
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
		strings.Contains(lowerOutput, "% unknown command") {
		return output, fmt.Errorf("command error: %s", output)
	}
	return output, nil
}

func (a *AOSCXAdapter) parseVersionFacts(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "AOS-CX") {
			if match := regexp.MustCompile(`Version\s+:\s*(\S+)`).FindStringSubmatch(line); len(match) > 1 {
				facts.OSVersion = match[1]
			} else if match := regexp.MustCompile(`AOS-CX\s+(\S+)`).FindStringSubmatch(line); len(match) > 1 {
				facts.OSVersion = match[1]
			}
		}

		if strings.Contains(line, "Product Name") || strings.Contains(line, "Platform") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Model = strings.TrimSpace(parts[1])
			}
		}

		if strings.Contains(line, "Serial Number") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.SerialNumber = strings.TrimSpace(parts[1])
			}
		}

		if strings.HasPrefix(line, "Hostname") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Hostname = strings.TrimSpace(parts[1])
			}
		}
	}
}

func (a *AOSCXAdapter) parseInterfaces(output string) []vendors.InterfaceFact {
	var interfaces []vendors.InterfaceFact
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Port") || strings.HasPrefix(line, "----") {
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

// NewAOSCXAdapterFactory creates an adapter factory for AOS-CX.
func NewAOSCXAdapterFactory(config *AOSCXConfig) vendors.VendorAdapterFactory {
	return func(vendorConfig *vendors.VendorConfig) (vendors.VendorAdapter, error) {
		if config == nil {
			config = DefaultAOSCXConfig()
		}
		if vendorConfig != nil {
			config.VendorConfig = vendorConfig
		}
		return NewAOSCXAdapter(config), nil
	}
}

func init() {
	vendors.Register(vendors.VendorHPAOSCX, NewAOSCXAdapterFactory(nil))
}
