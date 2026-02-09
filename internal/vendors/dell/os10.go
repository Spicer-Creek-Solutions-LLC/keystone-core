// Package dell provides Dell network device adapters for proxy agents.
package dell

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

// OS10Adapter implements VendorAdapter for Dell OS10 (Networking OS10) devices.
type OS10Adapter struct {
	vendors.BaseVendorAdapter
	sshAdapter *ssh.Adapter
	shell      *ssh.NetworkDeviceShell
	inEnable   bool
	inConfig   bool
}

// OS10Config contains Dell OS10 specific configuration.
type OS10Config struct {
	*vendors.VendorConfig
}

// DefaultOS10Config returns a default OS10 configuration.
func DefaultOS10Config() *OS10Config {
	cfg := vendors.DefaultVendorConfig()
	cfg.EnablePrompt = "#"
	cfg.ConfigPrompt = "(conf"
	return &OS10Config{VendorConfig: cfg}
}

// NewOS10Adapter creates a new Dell OS10 adapter.
func NewOS10Adapter(config *OS10Config) *OS10Adapter {
	if config == nil {
		config = DefaultOS10Config()
	}
	if config.VendorConfig == nil {
		config.VendorConfig = vendors.DefaultVendorConfig()
	}

	sshConfig := ssh.DefaultConfig()
	sshConfig.ConnectionConfig = protocols.DefaultConnectionConfig()
	sshConfig.Timeout = config.Timeout

	return &OS10Adapter{
		BaseVendorAdapter: vendors.BaseVendorAdapter{
			Config: config.VendorConfig,
		},
		sshAdapter: ssh.NewAdapter(sshConfig),
	}
}

// Vendor implements VendorAdapter.Vendor.
func (a *OS10Adapter) Vendor() vendors.VendorType {
	return vendors.VendorDellOS10
}

// Type implements ProtocolAdapter.Type.
func (a *OS10Adapter) Type() protocols.ProtocolType {
	return protocols.ProtocolSSH
}

// Connect implements ProtocolAdapter.Connect.
func (a *OS10Adapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.Device = device
	a.Credential = cred
	a.Protocol = a.sshAdapter

	if err := a.sshAdapter.Connect(ctx, device, cred); err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	shellConfig := &ssh.NetworkDeviceConfig{
		Vendor:         "dell_os10",
		Prompts:        []string{">", "#", "(conf", "(conf-"},
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
func (a *OS10Adapter) Disconnect(ctx context.Context) error {
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
func (a *OS10Adapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
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
func (a *OS10Adapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
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
func (a *OS10Adapter) IsConnected() bool {
	return a.Connected && a.sshAdapter.IsConnected()
}

// Metrics implements ProtocolAdapter.Metrics.
func (a *OS10Adapter) Metrics() *protocols.AdapterMetrics {
	return a.sshAdapter.Metrics()
}

// GetConfig implements VendorAdapter.GetConfig.
func (a *OS10Adapter) GetConfig(ctx context.Context, section string) (string, error) {
	var cmd string
	switch section {
	case "":
		cmd = "show running-configuration"
	case "startup":
		cmd = "show startup-configuration"
	case "interface", "interfaces":
		cmd = "show interface status"
	case "routing":
		cmd = "show running-configuration | grep router"
	default:
		cmd = fmt.Sprintf("show running-configuration | grep %s", section)
	}
	return a.runCommand(ctx, cmd)
}

// SetConfig implements VendorAdapter.SetConfig.
func (a *OS10Adapter) SetConfig(ctx context.Context, commands []string) error {
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
func (a *OS10Adapter) GetFacts(ctx context.Context) (*vendors.DeviceFacts, error) {
	facts := &vendors.DeviceFacts{
		Vendor: "Dell",
		OSType: "OS10",
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
		a.parseSystemFacts(system, facts)
	}

	intfs, err := a.runCommand(ctx, "show interface status")
	if err == nil {
		facts.Raw["show interface status"] = intfs
		facts.Interfaces = a.parseInterfaces(intfs)
	}

	return facts, nil
}

// SaveConfig implements VendorAdapter.SaveConfig.
func (a *OS10Adapter) SaveConfig(ctx context.Context) error {
	if a.inConfig {
		if err := a.exitConfig(ctx); err != nil {
			return err
		}
	}
	_, err := a.runCommand(ctx, "write memory")
	if err != nil {
		_, err = a.runCommand(ctx, "copy running-configuration startup-configuration")
	}
	return err
}

func (a *OS10Adapter) enterEnable(ctx context.Context) error {
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

func (a *OS10Adapter) enterConfig(ctx context.Context) error {
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

func (a *OS10Adapter) exitConfig(ctx context.Context) error {
	if !a.inConfig {
		return nil
	}
	if _, err := a.runCommand(ctx, "end"); err != nil {
		return err
	}
	a.inConfig = false
	return nil
}

func (a *OS10Adapter) runCommand(ctx context.Context, command string) (string, error) {
	if a.shell == nil {
		return "", fmt.Errorf("shell not initialized")
	}

	result, err := a.shell.Execute(ctx, command)
	if err != nil {
		return "", err
	}

	output := result.Output
	lowerOutput := strings.ToLower(output)
	if strings.Contains(lowerOutput, "% error") ||
		strings.Contains(lowerOutput, "% invalid input") ||
		strings.Contains(lowerOutput, "% incomplete command") {
		return output, fmt.Errorf("command error: %s", output)
	}
	return output, nil
}

func (a *OS10Adapter) parseVersionFacts(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "OS Version") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.OSVersion = strings.TrimSpace(parts[1])
			}
		}

		if strings.Contains(line, "System Type") || strings.Contains(line, "Platform") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Model = strings.TrimSpace(parts[1])
			}
		}

		if strings.Contains(line, "Serial Number") || strings.Contains(line, "Service Tag") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.SerialNumber = strings.TrimSpace(parts[1])
			}
		}

		if strings.Contains(line, "Up Time") || strings.Contains(line, "uptime") {
			uptime := parseUptime(line)
			if uptime > 0 {
				facts.Uptime = uptime
			}
		}
	}
}

func (a *OS10Adapter) parseSystemFacts(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Hostname") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Hostname = strings.TrimSpace(parts[1])
			}
		} else if facts.Hostname == "" && strings.HasPrefix(line, "Node Name") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Hostname = strings.TrimSpace(parts[1])
			}
		}
	}
}

func (a *OS10Adapter) parseInterfaces(output string) []vendors.InterfaceFact {
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

		for _, f := range fields[1 : len(fields)-2] {
			if strings.Contains(f, "/") || strings.Contains(f, ".") {
				intf.IPAddresses = append(intf.IPAddresses, f)
			}
			if speed, err := strconv.Atoi(strings.TrimSuffix(f, "G")); err == nil && speed > 0 {
				intf.Speed = speed * 1000
			}
		}

		interfaces = append(interfaces, intf)
	}
	return interfaces
}

// parseUptime parses Dell-style uptime strings.
func parseUptime(line string) time.Duration {
	var total time.Duration

	dayMatch := regexp.MustCompile(`(\d+)\s*day`).FindStringSubmatch(line)
	hourMatch := regexp.MustCompile(`(\d+)\s*hour`).FindStringSubmatch(line)
	minuteMatch := regexp.MustCompile(`(\d+)\s*min`).FindStringSubmatch(line)

	if len(dayMatch) > 1 {
		days, _ := strconv.Atoi(dayMatch[1])
		total += time.Duration(days) * 24 * time.Hour
	}
	if len(hourMatch) > 1 {
		hours, _ := strconv.Atoi(hourMatch[1])
		total += time.Duration(hours) * time.Hour
	}
	if len(minuteMatch) > 1 {
		minutes, _ := strconv.Atoi(minuteMatch[1])
		total += time.Duration(minutes) * time.Minute
	}

	return total
}

// NewOS10AdapterFactory creates an adapter factory for Dell OS10.
func NewOS10AdapterFactory(config *OS10Config) vendors.VendorAdapterFactory {
	return func(vendorConfig *vendors.VendorConfig) (vendors.VendorAdapter, error) {
		if config == nil {
			config = DefaultOS10Config()
		}
		if vendorConfig != nil {
			config.VendorConfig = vendorConfig
		}
		return NewOS10Adapter(config), nil
	}
}

func init() {
	vendors.Register(vendors.VendorDellOS10, NewOS10AdapterFactory(nil))
}
