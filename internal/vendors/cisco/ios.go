// Package cisco provides Cisco device adapters for proxy agents.
package cisco

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

// IOSAdapter implements VendorAdapter for Cisco IOS devices.
type IOSAdapter struct {
	vendors.BaseVendorAdapter
	sshAdapter *ssh.Adapter
	shell      *ssh.NetworkDeviceShell
	inEnable   bool
	inConfig   bool
}

// IOSConfig contains Cisco IOS specific configuration.
type IOSConfig struct {
	*vendors.VendorConfig
	// Secret is the enable secret.
	Secret string `json:"secret,omitempty"`
}

// DefaultIOSConfig returns a default IOS configuration.
func DefaultIOSConfig() *IOSConfig {
	return &IOSConfig{
		VendorConfig: vendors.DefaultVendorConfig(),
	}
}

// NewIOSAdapter creates a new Cisco IOS adapter.
func NewIOSAdapter(config *IOSConfig) *IOSAdapter {
	if config == nil {
		config = DefaultIOSConfig()
	}
	if config.VendorConfig == nil {
		config.VendorConfig = vendors.DefaultVendorConfig()
	}

	sshConfig := ssh.DefaultConfig()
	sshConfig.ConnectionConfig = protocols.DefaultConnectionConfig()
	sshConfig.Timeout = config.Timeout

	return &IOSAdapter{
		BaseVendorAdapter: vendors.BaseVendorAdapter{
			Config: config.VendorConfig,
		},
		sshAdapter: ssh.NewAdapter(sshConfig),
	}
}

// Vendor implements VendorAdapter.Vendor.
func (a *IOSAdapter) Vendor() vendors.VendorType {
	return vendors.VendorCiscoIOS
}

// Type implements ProtocolAdapter.Type.
func (a *IOSAdapter) Type() protocols.ProtocolType {
	return protocols.ProtocolSSH
}

// Connect implements ProtocolAdapter.Connect.
func (a *IOSAdapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.Device = device
	a.Credential = cred
	a.Protocol = a.sshAdapter

	// Connect using SSH
	if err := a.sshAdapter.Connect(ctx, device, cred); err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	// Create network device shell
	shellConfig := &ssh.NetworkDeviceConfig{
		Vendor:         "cisco",
		Prompts:        []string{">", "#", "(config", "(config-"},
		EnableCmd:      "enable",
		EnablePassword: a.Config.EnablePassword,
	}
	shell, err := a.sshAdapter.NewNetworkDeviceShell(ctx, shellConfig)
	if err != nil {
		_ = a.sshAdapter.Disconnect(ctx) //nolint:errcheck // best-effort cleanup
		return fmt.Errorf("failed to create shell: %w", err)
	}
	a.shell = shell

	// Disable paging if configured - best-effort, some devices may not support this
	if a.Config.DisablePaging {
		_, _ = a.runCommand(ctx, "terminal length 0")
	}

	// Enter enable mode if needed
	if a.Config.EnablePassword != "" || a.Config.PrivilegeLevel > 0 {
		if err := a.enterEnable(ctx); err != nil {
			return fmt.Errorf("failed to enter enable mode: %w", err)
		}
	}

	a.Connected = true
	return nil
}

// Disconnect implements ProtocolAdapter.Disconnect.
func (a *IOSAdapter) Disconnect(ctx context.Context) error {
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
func (a *IOSAdapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
	start := time.Now()
	result := &protocols.ExecuteResult{
		StartTime: start,
	}

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
func (a *IOSAdapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
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
	output, err := a.runCommand(ctx, "show version | include uptime")
	result.Latency = time.Since(start)

	if err != nil {
		result.Healthy = false
		result.Status = fmt.Sprintf("health check failed: %v", err)
		return result, nil
	}

	result.Healthy = true
	result.Status = "connected"
	result.Details["uptime"] = strings.TrimSpace(output)

	return result, nil
}

// IsConnected implements ProtocolAdapter.IsConnected.
func (a *IOSAdapter) IsConnected() bool {
	return a.Connected && a.sshAdapter.IsConnected()
}

// Metrics implements ProtocolAdapter.Metrics.
func (a *IOSAdapter) Metrics() *protocols.AdapterMetrics {
	return a.sshAdapter.Metrics()
}

// GetConfig implements VendorAdapter.GetConfig.
func (a *IOSAdapter) GetConfig(ctx context.Context, section string) (string, error) {
	var cmd string
	switch section {
	case "":
		cmd = "show running-config"
	case "startup":
		cmd = "show startup-config"
	case "interface", "interfaces":
		cmd = "show running-config | section interface"
	case "routing":
		cmd = "show running-config | section router"
	case "acl":
		cmd = "show running-config | section access-list"
	default:
		cmd = fmt.Sprintf("show running-config | section %s", section)
	}

	return a.runCommand(ctx, cmd)
}

// SetConfig implements VendorAdapter.SetConfig.
func (a *IOSAdapter) SetConfig(ctx context.Context, commands []string) error {
	// Enter config mode if not already
	if !a.inConfig {
		if err := a.enterConfig(ctx); err != nil {
			return err
		}
		defer func() { _ = a.exitConfig(ctx) }() //nolint:errcheck // best-effort cleanup
	}

	// Execute each command
	for _, cmd := range commands {
		if _, err := a.runCommand(ctx, cmd); err != nil {
			return fmt.Errorf("command failed '%s': %w", cmd, err)
		}
	}

	return nil
}

// GetFacts implements VendorAdapter.GetFacts.
func (a *IOSAdapter) GetFacts(ctx context.Context) (*vendors.DeviceFacts, error) {
	facts := &vendors.DeviceFacts{
		Vendor: "Cisco",
		OSType: "IOS",
		Raw:    make(map[string]string),
	}

	// Get version info
	version, err := a.runCommand(ctx, "show version")
	if err != nil {
		return nil, fmt.Errorf("failed to get version: %w", err)
	}
	facts.Raw["show version"] = version

	// Parse version output
	a.parseVersionFacts(version, facts)

	// Get hostname
	hostname, err := a.runCommand(ctx, "show running-config | include hostname")
	if err == nil {
		if match := regexp.MustCompile(`hostname\s+(\S+)`).FindStringSubmatch(hostname); len(match) > 1 {
			facts.Hostname = match[1]
		}
	}

	// Get interfaces
	intfs, err := a.runCommand(ctx, "show ip interface brief")
	if err == nil {
		facts.Raw["show ip interface brief"] = intfs
		facts.Interfaces = a.parseInterfaces(intfs)
	}

	return facts, nil
}

// SaveConfig implements VendorAdapter.SaveConfig.
func (a *IOSAdapter) SaveConfig(ctx context.Context) error {
	// Exit config mode if in it
	if a.inConfig {
		if err := a.exitConfig(ctx); err != nil {
			return err
		}
	}

	_, err := a.runCommand(ctx, "write memory")
	if err != nil {
		// Try alternative command
		_, err = a.runCommand(ctx, "copy running-config startup-config")
	}
	return err
}

// enterEnable enters enable mode.
func (a *IOSAdapter) enterEnable(ctx context.Context) error {
	if a.inEnable {
		return nil
	}

	output, err := a.runCommand(ctx, "enable")
	if err != nil {
		return err
	}

	// Check if password is needed
	if strings.Contains(strings.ToLower(output), "password") {
		password := a.Config.EnablePassword
		if password == "" {
			// Try to get from credential
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

// enterConfig enters configuration mode.
func (a *IOSAdapter) enterConfig(ctx context.Context) error {
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

// exitConfig exits configuration mode.
func (a *IOSAdapter) exitConfig(ctx context.Context) error {
	if !a.inConfig {
		return nil
	}

	if _, err := a.runCommand(ctx, "end"); err != nil {
		return err
	}

	a.inConfig = false
	return nil
}

// runCommand runs a command and returns the output.
func (a *IOSAdapter) runCommand(ctx context.Context, command string) (string, error) {
	if a.shell == nil {
		return "", fmt.Errorf("shell not initialized")
	}

	result, err := a.shell.Execute(ctx, command)
	if err != nil {
		return "", err
	}

	// Check for common error patterns
	output := result.Output
	lowerOutput := strings.ToLower(output)
	if strings.Contains(lowerOutput, "% invalid input") ||
		strings.Contains(lowerOutput, "% ambiguous command") ||
		strings.Contains(lowerOutput, "% incomplete command") {
		return output, fmt.Errorf("command error: %s", output)
	}

	return output, nil
}

// parseVersionFacts parses show version output into facts.
func (a *IOSAdapter) parseVersionFacts(output string, facts *vendors.DeviceFacts) {
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Parse IOS version
		if strings.Contains(line, "IOS Software") || strings.Contains(line, "Cisco IOS") {
			if match := regexp.MustCompile(`Version\s+(\S+)`).FindStringSubmatch(line); len(match) > 1 {
				facts.OSVersion = match[1]
			}
		}

		// Parse model
		if strings.HasPrefix(line, "cisco") || strings.HasPrefix(line, "Cisco") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				facts.Model = parts[1]
			}
		}

		// Parse serial number
		if strings.Contains(line, "Processor board ID") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				facts.SerialNumber = parts[3]
			}
		}

		// Parse uptime
		if strings.Contains(line, "uptime is") {
			uptime := a.parseUptime(line)
			if uptime > 0 {
				facts.Uptime = uptime
			}
		}

		// Parse memory
		if strings.Contains(line, "bytes of memory") {
			if match := regexp.MustCompile(`(\d+)K/(\d+)K bytes`).FindStringSubmatch(line); len(match) > 2 {
				used, _ := strconv.ParseInt(match[1], 10, 64)
				free, _ := strconv.ParseInt(match[2], 10, 64)
				facts.MemoryTotal = (used + free) * 1024
				facts.MemoryFree = free * 1024
			}
		}
	}
}

// parseUptime parses uptime string into duration.
func (a *IOSAdapter) parseUptime(line string) time.Duration {
	var total time.Duration

	// Match patterns like "1 year, 2 weeks, 3 days, 4 hours, 5 minutes"
	yearMatch := regexp.MustCompile(`(\d+)\s+year`).FindStringSubmatch(line)
	weekMatch := regexp.MustCompile(`(\d+)\s+week`).FindStringSubmatch(line)
	dayMatch := regexp.MustCompile(`(\d+)\s+day`).FindStringSubmatch(line)
	hourMatch := regexp.MustCompile(`(\d+)\s+hour`).FindStringSubmatch(line)
	minuteMatch := regexp.MustCompile(`(\d+)\s+minute`).FindStringSubmatch(line)

	if len(yearMatch) > 1 {
		years, _ := strconv.Atoi(yearMatch[1])
		total += time.Duration(years) * 365 * 24 * time.Hour
	}
	if len(weekMatch) > 1 {
		weeks, _ := strconv.Atoi(weekMatch[1])
		total += time.Duration(weeks) * 7 * 24 * time.Hour
	}
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

// parseInterfaces parses show ip interface brief output.
func (a *IOSAdapter) parseInterfaces(output string) []vendors.InterfaceFact {
	var interfaces []vendors.InterfaceFact
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Interface") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}

		intf := vendors.InterfaceFact{
			Name:        fields[0],
			AdminStatus: strings.ToLower(fields[4]),
			OperStatus:  strings.ToLower(fields[5]),
		}

		// Parse IP address if present
		if fields[1] != "unassigned" {
			intf.IPAddresses = []string{fields[1]}
		}

		interfaces = append(interfaces, intf)
	}

	return interfaces
}

// GetInterface retrieves interface details.
func (a *IOSAdapter) GetInterface(ctx context.Context, name string) (*vendors.InterfaceFact, error) {
	output, err := a.runCommand(ctx, fmt.Sprintf("show interface %s", name))
	if err != nil {
		return nil, err
	}

	intf := &vendors.InterfaceFact{
		Name: name,
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Parse admin/oper status
		if strings.Contains(line, "line protocol") {
			switch {
			case strings.Contains(line, "administratively down"):
				intf.AdminStatus = "down"
			case strings.Contains(line, "is up"):
				intf.AdminStatus = "up"
			case strings.Contains(line, "is down"):
				intf.AdminStatus = "down"
			}

			if strings.Contains(line, "line protocol is up") {
				intf.OperStatus = "up"
			} else {
				intf.OperStatus = "down"
			}
		}

		// Parse description
		if strings.HasPrefix(line, "Description:") {
			intf.Description = strings.TrimPrefix(line, "Description:")
			intf.Description = strings.TrimSpace(intf.Description)
		}

		// Parse MAC address
		if strings.Contains(line, "Hardware is") && strings.Contains(line, "address is") {
			if match := regexp.MustCompile(`address is\s+(\S+)`).FindStringSubmatch(line); len(match) > 1 {
				intf.MacAddress = match[1]
			}
		}

		// Parse MTU
		if strings.Contains(line, "MTU") {
			if match := regexp.MustCompile(`MTU\s+(\d+)`).FindStringSubmatch(line); len(match) > 1 {
				intf.MTU, _ = strconv.Atoi(match[1])
			}
		}

		// Parse speed
		if strings.Contains(line, "BW") {
			if match := regexp.MustCompile(`BW\s+(\d+)\s+Kbit`).FindStringSubmatch(line); len(match) > 1 {
				bw, _ := strconv.Atoi(match[1])
				intf.Speed = bw / 1000 // Convert to Mbps
			}
		}

		// Parse duplex
		if strings.Contains(line, "Duplex") {
			switch {
			case strings.Contains(strings.ToLower(line), "full"):
				intf.Duplex = "full"
			case strings.Contains(strings.ToLower(line), "half"):
				intf.Duplex = "half"
			default:
				intf.Duplex = "auto"
			}
		}
	}

	return intf, nil
}

// ConfigureInterface configures an interface.
func (a *IOSAdapter) ConfigureInterface(ctx context.Context, config *vendors.InterfaceConfig) error {
	commands := []string{
		fmt.Sprintf("interface %s", config.Name),
	}

	if config.Description != "" {
		commands = append(commands, fmt.Sprintf("description %s", config.Description))
	}

	if config.IPAddress != "" {
		// Split IP and mask
		parts := strings.Split(config.IPAddress, "/")
		if len(parts) == 2 {
			mask := cidrToMask(parts[1])
			commands = append(commands, fmt.Sprintf("ip address %s %s", parts[0], mask))
		}
	}

	if config.MTU > 0 {
		commands = append(commands, fmt.Sprintf("mtu %d", config.MTU))
	}

	if config.Enabled {
		commands = append(commands, "no shutdown")
	} else {
		commands = append(commands, "shutdown")
	}

	return a.SetConfig(ctx, commands)
}

// GetVLANs retrieves VLAN information.
func (a *IOSAdapter) GetVLANs(ctx context.Context) ([]vendors.VLANConfig, error) {
	output, err := a.runCommand(ctx, "show vlan brief")
	if err != nil {
		return nil, err
	}

	var vlans []vendors.VLANConfig
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "VLAN") || strings.HasPrefix(line, "----") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		id, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}

		vlans = append(vlans, vendors.VLANConfig{
			ID:    id,
			Name:  fields[1],
			State: strings.ToLower(fields[2]),
		})
	}

	return vlans, nil
}

// ConfigureVLAN configures a VLAN.
func (a *IOSAdapter) ConfigureVLAN(ctx context.Context, config *vendors.VLANConfig) error {
	commands := []string{
		fmt.Sprintf("vlan %d", config.ID),
	}

	if config.Name != "" {
		commands = append(commands, fmt.Sprintf("name %s", config.Name))
	}

	if config.State != "" {
		commands = append(commands, fmt.Sprintf("state %s", config.State))
	}

	return a.SetConfig(ctx, commands)
}

// cidrToMask converts CIDR prefix length to subnet mask.
func cidrToMask(cidr string) string {
	prefix, err := strconv.Atoi(cidr)
	if err != nil {
		return "255.255.255.0"
	}

	mask := uint32(0xFFFFFFFF << (32 - prefix))
	return fmt.Sprintf("%d.%d.%d.%d",
		byte(mask>>24),
		byte(mask>>16),
		byte(mask>>8),
		byte(mask))
}

// NewIOSAdapterFactory creates an adapter factory for Cisco IOS.
func NewIOSAdapterFactory(config *IOSConfig) vendors.VendorAdapterFactory {
	return func(vendorConfig *vendors.VendorConfig) (vendors.VendorAdapter, error) {
		if config == nil {
			config = DefaultIOSConfig()
		}
		if vendorConfig != nil {
			config.VendorConfig = vendorConfig
		}
		return NewIOSAdapter(config), nil
	}
}

// init registers the Cisco IOS adapter with the default registry.
func init() {
	vendors.Register(vendors.VendorCiscoIOS, NewIOSAdapterFactory(nil))
}
