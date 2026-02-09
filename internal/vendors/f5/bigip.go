// Package f5 provides F5 BIG-IP device adapters for proxy agents.
package f5

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/protocols/ssh"
	"github.com/shawnbutts/keystone-core/internal/proxy"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

// BigIPAdapter implements VendorAdapter for F5 BIG-IP devices.
type BigIPAdapter struct {
	vendors.BaseVendorAdapter
	sshAdapter *ssh.Adapter
	shell      *ssh.NetworkDeviceShell
	inTmsh     bool
}

// BigIPConfig contains F5 BIG-IP specific configuration.
type BigIPConfig struct {
	*vendors.VendorConfig
}

// DefaultBigIPConfig returns a default BIG-IP configuration.
func DefaultBigIPConfig() *BigIPConfig {
	cfg := vendors.DefaultVendorConfig()
	cfg.EnablePrompt = "(tmos)#"
	cfg.ConfigPrompt = "(tmos)#"
	cfg.DisablePaging = true
	return &BigIPConfig{VendorConfig: cfg}
}

// NewBigIPAdapter creates a new F5 BIG-IP adapter.
func NewBigIPAdapter(config *BigIPConfig) *BigIPAdapter {
	if config == nil {
		config = DefaultBigIPConfig()
	}
	if config.VendorConfig == nil {
		config.VendorConfig = vendors.DefaultVendorConfig()
	}

	sshConfig := ssh.DefaultConfig()
	sshConfig.ConnectionConfig = protocols.DefaultConnectionConfig()
	sshConfig.Timeout = config.Timeout

	return &BigIPAdapter{
		BaseVendorAdapter: vendors.BaseVendorAdapter{
			Config: config.VendorConfig,
		},
		sshAdapter: ssh.NewAdapter(sshConfig),
	}
}

// Vendor implements VendorAdapter.Vendor.
func (a *BigIPAdapter) Vendor() vendors.VendorType {
	return vendors.VendorF5BigIP
}

// Type implements ProtocolAdapter.Type.
func (a *BigIPAdapter) Type() protocols.ProtocolType {
	return protocols.ProtocolSSH
}

// Connect implements ProtocolAdapter.Connect.
func (a *BigIPAdapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.Device = device
	a.Credential = cred
	a.Protocol = a.sshAdapter

	if err := a.sshAdapter.Connect(ctx, device, cred); err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	shellConfig := &ssh.NetworkDeviceConfig{
		Vendor:  "f5_bigip",
		Prompts: []string{"(tmos)#", "#", "$"},
	}
	shell, err := a.sshAdapter.NewNetworkDeviceShell(ctx, shellConfig)
	if err != nil {
		_ = a.sshAdapter.Disconnect(ctx)
		return fmt.Errorf("failed to create shell: %w", err)
	}
	a.shell = shell

	// Enter tmsh (may already be in tmsh if shell defaults to it)
	_, _ = a.runCommand(ctx, "tmsh")
	a.inTmsh = true

	// Disable paging
	if a.Config.DisablePaging {
		_, _ = a.runCommand(ctx, "modify cli preference pager disabled")
	}

	a.Connected = true
	return nil
}

// Disconnect implements ProtocolAdapter.Disconnect.
func (a *BigIPAdapter) Disconnect(ctx context.Context) error {
	a.Connected = false
	a.inTmsh = false

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
func (a *BigIPAdapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
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
func (a *BigIPAdapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
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
	output, err := a.runCommand(ctx, "show sys version")
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
func (a *BigIPAdapter) IsConnected() bool {
	return a.Connected && a.sshAdapter.IsConnected()
}

// Metrics implements ProtocolAdapter.Metrics.
func (a *BigIPAdapter) Metrics() *protocols.AdapterMetrics {
	return a.sshAdapter.Metrics()
}

// GetConfig implements VendorAdapter.GetConfig.
func (a *BigIPAdapter) GetConfig(ctx context.Context, section string) (string, error) {
	var cmd string
	switch section {
	case "":
		cmd = "list sys global-settings"
	case "ltm", "virtual":
		cmd = "list ltm virtual"
	case "pool", "pools":
		cmd = "list ltm pool"
	case "node", "nodes":
		cmd = "list ltm node"
	case "interface", "interfaces":
		cmd = "list net interface"
	case "vlan", "vlans":
		cmd = "list net vlan"
	case "self", "self-ip":
		cmd = "list net self"
	default:
		cmd = fmt.Sprintf("list %s", section)
	}
	return a.runCommand(ctx, cmd)
}

// SetConfig implements VendorAdapter.SetConfig.
// BIG-IP tmsh uses create/modify/delete commands directly.
func (a *BigIPAdapter) SetConfig(ctx context.Context, commands []string) error {
	for _, cmd := range commands {
		if _, err := a.runCommand(ctx, cmd); err != nil {
			return fmt.Errorf("command failed '%s': %w", cmd, err)
		}
	}
	return nil
}

// GetFacts implements VendorAdapter.GetFacts.
func (a *BigIPAdapter) GetFacts(ctx context.Context) (*vendors.DeviceFacts, error) {
	facts := &vendors.DeviceFacts{
		Vendor: "F5",
		OSType: "BIG-IP",
		Raw:    make(map[string]string),
	}

	version, err := a.runCommand(ctx, "show sys version")
	if err != nil {
		return nil, fmt.Errorf("failed to get version: %w", err)
	}
	facts.Raw["show sys version"] = version
	a.parseVersion(version, facts)

	hardware, err := a.runCommand(ctx, "show sys hardware")
	if err == nil {
		facts.Raw["show sys hardware"] = hardware
		a.parseHardware(hardware, facts)
	}

	globalSettings, err := a.runCommand(ctx, "list sys global-settings hostname")
	if err == nil {
		a.parseGlobalSettings(globalSettings, facts)
	}

	intfs, err := a.runCommand(ctx, "list net interface")
	if err == nil {
		facts.Raw["list net interface"] = intfs
		facts.Interfaces = a.parseInterfaces(intfs)
	}

	return facts, nil
}

// SaveConfig implements VendorAdapter.SaveConfig.
func (a *BigIPAdapter) SaveConfig(ctx context.Context) error {
	_, err := a.runCommand(ctx, "save sys config")
	return err
}

// ListVirtuals lists all virtual servers.
func (a *BigIPAdapter) ListVirtuals(ctx context.Context) (string, error) {
	return a.runCommand(ctx, "list ltm virtual")
}

// ListPools lists all pools.
func (a *BigIPAdapter) ListPools(ctx context.Context) (string, error) {
	return a.runCommand(ctx, "list ltm pool")
}

func (a *BigIPAdapter) runCommand(ctx context.Context, command string) (string, error) {
	if a.shell == nil {
		return "", fmt.Errorf("shell not initialized")
	}

	result, err := a.shell.Execute(ctx, command)
	if err != nil {
		return "", err
	}

	output := result.Output
	lowerOutput := strings.ToLower(output)
	if strings.Contains(lowerOutput, "syntax error") ||
		strings.Contains(lowerOutput, "unexpected argument") ||
		strings.Contains(lowerOutput, "01070734") { // BIG-IP error code for invalid command
		return output, fmt.Errorf("command error: %s", output)
	}
	return output, nil
}

func (a *BigIPAdapter) parseVersion(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "Version") && !strings.Contains(line, "Build") {
			if match := regexp.MustCompile(`(\d+\.\d+\.\d+(?:\.\d+)?)`).FindStringSubmatch(line); len(match) > 1 {
				facts.OSVersion = match[1]
			}
		}
	}
}

func (a *BigIPAdapter) parseHardware(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "Platform") || strings.Contains(line, "Name") {
			if parts := strings.SplitN(line, " ", 2); len(parts) == 2 {
				model := strings.TrimSpace(parts[1])
				if model != "" && facts.Model == "" {
					facts.Model = model
				}
			}
		}

		if strings.Contains(line, "Serial") {
			if match := regexp.MustCompile(`(\S{10,})`).FindStringSubmatch(line); len(match) > 1 {
				facts.SerialNumber = match[1]
			}
		}

		if strings.Contains(line, "Memory") && strings.Contains(line, "Total") {
			if match := regexp.MustCompile(`(\d+)`).FindStringSubmatch(line); len(match) > 1 {
				mem, _ := strconv.ParseInt(match[1], 10, 64)
				facts.MemoryTotal = mem * 1024 * 1024
			}
		}
	}
}

func (a *BigIPAdapter) parseGlobalSettings(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "hostname") {
			if match := regexp.MustCompile(`hostname\s+(\S+)`).FindStringSubmatch(line); len(match) > 1 {
				facts.Hostname = match[1]
			}
		}
	}
}

func (a *BigIPAdapter) parseInterfaces(output string) []vendors.InterfaceFact {
	var interfaces []vendors.InterfaceFact
	var current *vendors.InterfaceFact

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "net interface") {
			if current != nil {
				interfaces = append(interfaces, *current)
			}
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				current = &vendors.InterfaceFact{
					Name: fields[2],
				}
			}
		}

		if current == nil {
			continue
		}

		if strings.HasPrefix(line, "media-active") {
			if strings.Contains(line, "none") {
				current.OperStatus = "down"
			} else {
				current.OperStatus = "up"
			}
		}

		if strings.HasPrefix(line, "enabled") {
			current.AdminStatus = "up"
		}
		if strings.HasPrefix(line, "disabled") {
			current.AdminStatus = "down"
		}

		if strings.HasPrefix(line, "mac-address") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				current.MacAddress = parts[1]
			}
		}
	}

	if current != nil {
		interfaces = append(interfaces, *current)
	}

	return interfaces
}

// NewBigIPAdapterFactory creates an adapter factory for F5 BIG-IP.
func NewBigIPAdapterFactory(config *BigIPConfig) vendors.VendorAdapterFactory {
	return func(vendorConfig *vendors.VendorConfig) (vendors.VendorAdapter, error) {
		if config == nil {
			config = DefaultBigIPConfig()
		}
		if vendorConfig != nil {
			config.VendorConfig = vendorConfig
		}
		return NewBigIPAdapter(config), nil
	}
}

func init() {
	vendors.Register(vendors.VendorF5BigIP, NewBigIPAdapterFactory(nil))
}
