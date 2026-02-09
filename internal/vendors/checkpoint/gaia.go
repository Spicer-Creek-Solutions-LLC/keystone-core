// Package checkpoint provides Check Point Gaia device adapters for proxy agents.
package checkpoint

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

// GaiaAdapter implements VendorAdapter for Check Point Gaia devices.
type GaiaAdapter struct {
	vendors.BaseVendorAdapter
	sshAdapter *ssh.Adapter
	shell      *ssh.NetworkDeviceShell
}

// GaiaConfig contains Check Point Gaia specific configuration.
type GaiaConfig struct {
	*vendors.VendorConfig
}

// DefaultGaiaConfig returns a default Gaia configuration.
func DefaultGaiaConfig() *GaiaConfig {
	cfg := vendors.DefaultVendorConfig()
	cfg.EnablePrompt = ">"
	cfg.ConfigPrompt = ">"
	cfg.DisablePaging = true
	return &GaiaConfig{VendorConfig: cfg}
}

// NewGaiaAdapter creates a new Check Point Gaia adapter.
func NewGaiaAdapter(config *GaiaConfig) *GaiaAdapter {
	if config == nil {
		config = DefaultGaiaConfig()
	}
	if config.VendorConfig == nil {
		config.VendorConfig = vendors.DefaultVendorConfig()
	}

	sshConfig := ssh.DefaultConfig()
	sshConfig.ConnectionConfig = protocols.DefaultConnectionConfig()
	sshConfig.Timeout = config.Timeout

	return &GaiaAdapter{
		BaseVendorAdapter: vendors.BaseVendorAdapter{
			Config: config.VendorConfig,
		},
		sshAdapter: ssh.NewAdapter(sshConfig),
	}
}

// Vendor implements VendorAdapter.Vendor.
func (a *GaiaAdapter) Vendor() vendors.VendorType {
	return vendors.VendorCheckpointGaia
}

// Type implements ProtocolAdapter.Type.
func (a *GaiaAdapter) Type() protocols.ProtocolType {
	return protocols.ProtocolSSH
}

// Connect implements ProtocolAdapter.Connect.
func (a *GaiaAdapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.Device = device
	a.Credential = cred
	a.Protocol = a.sshAdapter

	if err := a.sshAdapter.Connect(ctx, device, cred); err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	shellConfig := &ssh.NetworkDeviceConfig{
		Vendor:  "checkpoint_gaia",
		Prompts: []string{">", "#"},
	}
	shell, err := a.sshAdapter.NewNetworkDeviceShell(ctx, shellConfig)
	if err != nil {
		_ = a.sshAdapter.Disconnect(ctx)
		return fmt.Errorf("failed to create shell: %w", err)
	}
	a.shell = shell

	if a.Config.DisablePaging {
		_, _ = a.runCommand(ctx, "set clienv rows 0")
	}

	a.Connected = true
	return nil
}

// Disconnect implements ProtocolAdapter.Disconnect.
func (a *GaiaAdapter) Disconnect(ctx context.Context) error {
	a.Connected = false

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
func (a *GaiaAdapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
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
func (a *GaiaAdapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
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
	output, err := a.runCommand(ctx, "show hostname")
	result.Latency = time.Since(start)

	if err != nil {
		result.Healthy = false
		result.Status = fmt.Sprintf("health check failed: %v", err)
		return result, nil
	}

	result.Healthy = true
	result.Status = "connected"
	result.Details["hostname"] = strings.TrimSpace(output)
	return result, nil
}

// IsConnected implements ProtocolAdapter.IsConnected.
func (a *GaiaAdapter) IsConnected() bool {
	return a.Connected && a.sshAdapter.IsConnected()
}

// Metrics implements ProtocolAdapter.Metrics.
func (a *GaiaAdapter) Metrics() *protocols.AdapterMetrics {
	return a.sshAdapter.Metrics()
}

// GetConfig implements VendorAdapter.GetConfig.
func (a *GaiaAdapter) GetConfig(ctx context.Context, section string) (string, error) {
	if section == "" {
		return a.runCommand(ctx, "show configuration")
	}
	return a.runCommand(ctx, fmt.Sprintf("show configuration %s", section))
}

// SetConfig implements VendorAdapter.SetConfig.
func (a *GaiaAdapter) SetConfig(ctx context.Context, commands []string) error {
	for _, cmd := range commands {
		if _, err := a.runCommand(ctx, cmd); err != nil {
			return fmt.Errorf("command failed '%s': %w", cmd, err)
		}
	}
	return nil
}

// GetFacts implements VendorAdapter.GetFacts.
func (a *GaiaAdapter) GetFacts(ctx context.Context) (*vendors.DeviceFacts, error) {
	facts := &vendors.DeviceFacts{
		Vendor: "Check Point",
		OSType: "Gaia",
		Raw:    make(map[string]string),
	}

	version, err := a.runCommand(ctx, "show version all")
	if err != nil {
		return nil, fmt.Errorf("failed to get version: %w", err)
	}
	facts.Raw["show version all"] = version
	a.parseVersion(version, facts)

	hostname, err := a.runCommand(ctx, "show hostname")
	if err == nil {
		facts.Hostname = strings.TrimSpace(hostname)
	}

	asset, err := a.runCommand(ctx, "show asset all")
	if err == nil {
		facts.Raw["show asset all"] = asset
		a.parseAsset(asset, facts)
	}

	intfs, err := a.runCommand(ctx, "show interfaces all")
	if err == nil {
		facts.Raw["show interfaces all"] = intfs
		facts.Interfaces = a.parseInterfaces(intfs)
	}

	return facts, nil
}

// SaveConfig implements VendorAdapter.SaveConfig.
func (a *GaiaAdapter) SaveConfig(ctx context.Context) error {
	_, err := a.runCommand(ctx, "save config")
	return err
}

func (a *GaiaAdapter) runCommand(ctx context.Context, command string) (string, error) {
	if a.shell == nil {
		return "", fmt.Errorf("shell not initialized")
	}

	result, err := a.shell.Execute(ctx, command)
	if err != nil {
		return "", err
	}

	output := result.Output
	lowerOutput := strings.ToLower(output)
	if strings.Contains(lowerOutput, "invalid command") ||
		strings.Contains(lowerOutput, "wrong parameter") ||
		strings.Contains(lowerOutput, "cpmi error") {
		return output, fmt.Errorf("command error: %s", output)
	}
	return output, nil
}

func (a *GaiaAdapter) parseVersion(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "Product version") {
			if match := regexp.MustCompile(`R(\d+(?:\.\d+)*)`).FindStringSubmatch(line); len(match) > 1 {
				facts.OSVersion = "R" + match[1]
			}
		}
	}
}

func (a *GaiaAdapter) parseAsset(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "Platform") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Model = strings.TrimSpace(parts[1])
			}
		}

		if strings.Contains(line, "Serial Number") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.SerialNumber = strings.TrimSpace(parts[1])
			}
		}
	}
}

func (a *GaiaAdapter) parseInterfaces(output string) []vendors.InterfaceFact {
	var interfaces []vendors.InterfaceFact

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "Interface") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		iface := vendors.InterfaceFact{
			Name: fields[0],
		}

		for _, f := range fields[1:] {
			switch strings.ToLower(f) {
			case "up":
				if iface.AdminStatus == "" {
					iface.AdminStatus = "up"
				} else {
					iface.OperStatus = "up"
				}
			case "down":
				if iface.AdminStatus == "" {
					iface.AdminStatus = "down"
				} else {
					iface.OperStatus = "down"
				}
			default:
				if strings.Contains(f, ":") && len(f) == 17 {
					iface.MacAddress = f
				}
			}
		}

		if iface.AdminStatus != "" {
			interfaces = append(interfaces, iface)
		}
	}

	return interfaces
}

// NewGaiaAdapterFactory creates an adapter factory for Check Point Gaia.
func NewGaiaAdapterFactory(config *GaiaConfig) vendors.VendorAdapterFactory {
	return func(vendorConfig *vendors.VendorConfig) (vendors.VendorAdapter, error) {
		if config == nil {
			config = DefaultGaiaConfig()
		}
		if vendorConfig != nil {
			config.VendorConfig = vendorConfig
		}
		return NewGaiaAdapter(config), nil
	}
}

func init() {
	vendors.Register(vendors.VendorCheckpointGaia, NewGaiaAdapterFactory(nil))
}
