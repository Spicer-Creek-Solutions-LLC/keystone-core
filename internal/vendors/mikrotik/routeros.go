// Package mikrotik provides MikroTik RouterOS device adapters for proxy agents.
package mikrotik

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

// RouterOSAdapter implements VendorAdapter for MikroTik RouterOS devices.
type RouterOSAdapter struct {
	vendors.BaseVendorAdapter
	sshAdapter *ssh.Adapter
	shell      *ssh.NetworkDeviceShell
}

// RouterOSConfig contains MikroTik RouterOS specific configuration.
type RouterOSConfig struct {
	*vendors.VendorConfig
}

// DefaultRouterOSConfig returns a default RouterOS configuration.
func DefaultRouterOSConfig() *RouterOSConfig {
	cfg := vendors.DefaultVendorConfig()
	cfg.EnablePrompt = ">"
	cfg.ConfigPrompt = ">"
	cfg.DisablePaging = true
	return &RouterOSConfig{VendorConfig: cfg}
}

// NewRouterOSAdapter creates a new MikroTik RouterOS adapter.
func NewRouterOSAdapter(config *RouterOSConfig) *RouterOSAdapter {
	if config == nil {
		config = DefaultRouterOSConfig()
	}
	if config.VendorConfig == nil {
		config.VendorConfig = vendors.DefaultVendorConfig()
	}

	sshConfig := ssh.DefaultConfig()
	sshConfig.ConnectionConfig = protocols.DefaultConnectionConfig()
	sshConfig.Timeout = config.Timeout

	return &RouterOSAdapter{
		BaseVendorAdapter: vendors.BaseVendorAdapter{
			Config: config.VendorConfig,
		},
		sshAdapter: ssh.NewAdapter(sshConfig),
	}
}

// Vendor implements VendorAdapter.Vendor.
func (a *RouterOSAdapter) Vendor() vendors.VendorType {
	return vendors.VendorMikroTikRouterOS
}

// Type implements ProtocolAdapter.Type.
func (a *RouterOSAdapter) Type() protocols.ProtocolType {
	return protocols.ProtocolSSH
}

// Connect implements ProtocolAdapter.Connect.
func (a *RouterOSAdapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.Device = device
	a.Credential = cred
	a.Protocol = a.sshAdapter

	if err := a.sshAdapter.Connect(ctx, device, cred); err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	shellConfig := &ssh.NetworkDeviceConfig{
		Vendor:  "mikrotik_routeros",
		Prompts: []string{">", "] >"},
	}
	shell, err := a.sshAdapter.NewNetworkDeviceShell(ctx, shellConfig)
	if err != nil {
		_ = a.sshAdapter.Disconnect(ctx)
		return fmt.Errorf("failed to create shell: %w", err)
	}
	a.shell = shell

	a.Connected = true
	return nil
}

// Disconnect implements ProtocolAdapter.Disconnect.
func (a *RouterOSAdapter) Disconnect(ctx context.Context) error {
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
func (a *RouterOSAdapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
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
func (a *RouterOSAdapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
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
	output, err := a.runCommand(ctx, "/system identity print")
	result.Latency = time.Since(start)

	if err != nil {
		result.Healthy = false
		result.Status = fmt.Sprintf("health check failed: %v", err)
		return result, nil
	}

	result.Healthy = true
	result.Status = "connected"
	result.Details["identity"] = strings.TrimSpace(output)
	return result, nil
}

// IsConnected implements ProtocolAdapter.IsConnected.
func (a *RouterOSAdapter) IsConnected() bool {
	return a.Connected && a.sshAdapter.IsConnected()
}

// Metrics implements ProtocolAdapter.Metrics.
func (a *RouterOSAdapter) Metrics() *protocols.AdapterMetrics {
	return a.sshAdapter.Metrics()
}

// GetConfig implements VendorAdapter.GetConfig.
func (a *RouterOSAdapter) GetConfig(ctx context.Context, section string) (string, error) {
	if section == "" {
		return a.runCommand(ctx, "/export")
	}
	return a.runCommand(ctx, fmt.Sprintf("/%s export", section))
}

// SetConfig implements VendorAdapter.SetConfig.
func (a *RouterOSAdapter) SetConfig(ctx context.Context, commands []string) error {
	for _, cmd := range commands {
		if _, err := a.runCommand(ctx, cmd); err != nil {
			return fmt.Errorf("command failed '%s': %w", cmd, err)
		}
	}
	return nil
}

// GetFacts implements VendorAdapter.GetFacts.
func (a *RouterOSAdapter) GetFacts(ctx context.Context) (*vendors.DeviceFacts, error) {
	facts := &vendors.DeviceFacts{
		Vendor: "MikroTik",
		OSType: "RouterOS",
		Raw:    make(map[string]string),
	}

	identity, err := a.runCommand(ctx, "/system identity print")
	if err == nil {
		a.parseIdentity(identity, facts)
	}

	resource, err := a.runCommand(ctx, "/system resource print")
	if err != nil {
		return nil, fmt.Errorf("failed to get resource: %w", err)
	}
	facts.Raw["/system resource print"] = resource
	a.parseResource(resource, facts)

	routerboard, err := a.runCommand(ctx, "/system routerboard print")
	if err == nil {
		facts.Raw["/system routerboard print"] = routerboard
		a.parseRouterboard(routerboard, facts)
	}

	intfs, err := a.runCommand(ctx, "/interface print")
	if err == nil {
		facts.Raw["/interface print"] = intfs
		facts.Interfaces = a.parseInterfaces(intfs)
	}

	return facts, nil
}

// SaveConfig implements VendorAdapter.SaveConfig.
// RouterOS auto-saves all configuration changes.
func (a *RouterOSAdapter) SaveConfig(_ context.Context) error {
	return nil
}

func (a *RouterOSAdapter) runCommand(ctx context.Context, command string) (string, error) {
	if a.shell == nil {
		return "", fmt.Errorf("shell not initialized")
	}

	result, err := a.shell.Execute(ctx, command)
	if err != nil {
		return "", err
	}

	output := result.Output
	lowerOutput := strings.ToLower(output)
	if strings.Contains(lowerOutput, "bad command") ||
		strings.Contains(lowerOutput, "syntax error") ||
		strings.Contains(lowerOutput, "no such item") ||
		strings.Contains(lowerOutput, "expected end of command") {
		return output, fmt.Errorf("command error: %s", output)
	}
	return output, nil
}

func (a *RouterOSAdapter) parseIdentity(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "name:") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Hostname = strings.TrimSpace(parts[1])
			}
		}
	}
}

func (a *RouterOSAdapter) parseResource(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "version:") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.OSVersion = strings.TrimSpace(parts[1])
			}
		}

		if strings.HasPrefix(line, "board-name:") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Model = strings.TrimSpace(parts[1])
			}
		}

		if strings.HasPrefix(line, "uptime:") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Uptime = parseRouterOSUptime(strings.TrimSpace(parts[1]))
			}
		}

		if strings.HasPrefix(line, "cpu-load:") {
			if match := regexp.MustCompile(`(\d+)`).FindStringSubmatch(line); len(match) > 1 {
				var cpu float64
				fmt.Sscanf(match[1], "%f", &cpu)
				facts.CPUUsage = cpu
			}
		}
	}
}

func (a *RouterOSAdapter) parseRouterboard(output string, facts *vendors.DeviceFacts) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "serial-number:") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.SerialNumber = strings.TrimSpace(parts[1])
			}
		}

		if strings.HasPrefix(line, "model:") && facts.Model == "" {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				facts.Model = strings.TrimSpace(parts[1])
			}
		}
	}
}

func (a *RouterOSAdapter) parseInterfaces(output string) []vendors.InterfaceFact {
	var interfaces []vendors.InterfaceFact

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Flags") || strings.HasPrefix(line, "#") ||
			strings.HasPrefix(line, "Columns") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		// RouterOS interface print: flags name type mtu ...
		var name string
		for _, f := range fields {
			if !strings.ContainsAny(f, "RSXDI") || len(f) > 10 {
				name = f
				break
			}
		}
		if name == "" || strings.ContainsAny(name, "=;") {
			continue
		}

		iface := vendors.InterfaceFact{
			Name:        name,
			AdminStatus: "up",
			OperStatus:  "up",
		}

		if len(fields) > 0 {
			flags := fields[0]
			if strings.Contains(flags, "X") {
				iface.AdminStatus = "down"
				iface.OperStatus = "down"
			}
			if strings.Contains(flags, "D") {
				iface.OperStatus = "down"
			}
		}

		interfaces = append(interfaces, iface)
	}

	return interfaces
}

func parseRouterOSUptime(s string) time.Duration {
	var days, hours, minutes, seconds int
	s = strings.TrimSpace(s)

	if match := regexp.MustCompile(`(\d+)w`).FindStringSubmatch(s); len(match) > 1 {
		fmt.Sscanf(match[1], "%d", &days)
		days *= 7
	}
	if match := regexp.MustCompile(`(\d+)d`).FindStringSubmatch(s); len(match) > 1 {
		var d int
		fmt.Sscanf(match[1], "%d", &d)
		days += d
	}
	if match := regexp.MustCompile(`(\d+)h`).FindStringSubmatch(s); len(match) > 1 {
		fmt.Sscanf(match[1], "%d", &hours)
	}
	if match := regexp.MustCompile(`(\d+)m`).FindStringSubmatch(s); len(match) > 1 {
		fmt.Sscanf(match[1], "%d", &minutes)
	}
	if match := regexp.MustCompile(`(\d+)s`).FindStringSubmatch(s); len(match) > 1 {
		fmt.Sscanf(match[1], "%d", &seconds)
	}

	return time.Duration(days)*24*time.Hour +
		time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second
}

// NewRouterOSAdapterFactory creates an adapter factory for MikroTik RouterOS.
func NewRouterOSAdapterFactory(config *RouterOSConfig) vendors.VendorAdapterFactory {
	return func(vendorConfig *vendors.VendorConfig) (vendors.VendorAdapter, error) {
		if config == nil {
			config = DefaultRouterOSConfig()
		}
		if vendorConfig != nil {
			config.VendorConfig = vendorConfig
		}
		return NewRouterOSAdapter(config), nil
	}
}

func init() {
	vendors.Register(vendors.VendorMikroTikRouterOS, NewRouterOSAdapterFactory(nil))
}
