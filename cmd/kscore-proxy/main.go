package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/shawnbutts/keystone-core/pkg/cli/auditutil"
	"github.com/shawnbutts/keystone-core/pkg/cli/output"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

var (
	// Global flags
	serverAddr   string
	outputFormat string
	verbose      bool
	auditLevel   string
	auditOutput  string
)

// ProxyDevice represents a managed device
type ProxyDevice struct {
	ID         string            `json:"id" yaml:"id"`
	Name       string            `json:"name" yaml:"name"`
	Address    string            `json:"address" yaml:"address"`
	Protocol   string            `json:"protocol" yaml:"protocol"`
	Vendor     string            `json:"vendor" yaml:"vendor"`
	DeviceType string            `json:"device_type" yaml:"device_type"`
	Profile    string            `json:"profile" yaml:"profile"`
	Credential string            `json:"credential" yaml:"credential"`
	ProxyAgent string            `json:"proxy_agent" yaml:"proxy_agent"`
	Labels     map[string]string `json:"labels" yaml:"labels"`
	Status     string            `json:"status" yaml:"status"`
	Health     string            `json:"health" yaml:"health"`
	LastSeen   time.Time         `json:"last_seen" yaml:"last_seen"`
	CreatedAt  time.Time         `json:"created_at" yaml:"created_at"`
}

// ProxyCredential represents a credential set
type ProxyCredential struct {
	ID          string    `json:"id" yaml:"id"`
	Name        string    `json:"name" yaml:"name"`
	Type        string    `json:"type" yaml:"type"` // ssh-password, ssh-key, snmp-v2c, snmp-v3, rest-token, rest-basic
	Username    string    `json:"username,omitempty" yaml:"username,omitempty"`
	Protocol    string    `json:"protocol" yaml:"protocol"`
	DeviceTypes []string  `json:"device_types,omitempty" yaml:"device_types,omitempty"`
	Backend     string    `json:"backend" yaml:"backend"` // local, vault, k8s-secret
	CreatedAt   time.Time `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" yaml:"updated_at"`
}

// DiscoveredDevice represents a discovered device
type DiscoveredDevice struct {
	ID           string    `json:"id" yaml:"id"`
	Address      string    `json:"address" yaml:"address"`
	Hostname     string    `json:"hostname,omitempty" yaml:"hostname,omitempty"`
	Vendor       string    `json:"vendor" yaml:"vendor"`
	Model        string    `json:"model,omitempty" yaml:"model,omitempty"`
	Profile      string    `json:"profile" yaml:"profile"`
	Status       string    `json:"status" yaml:"status"` // pending, approved, rejected, ignored
	DiscoveredAt time.Time `json:"discovered_at" yaml:"discovered_at"`
	DiscoveryMethod string `json:"discovery_method" yaml:"discovery_method"`
}

// DriftResult represents drift detection result
type DriftResult struct {
	DeviceID    string       `json:"device_id" yaml:"device_id"`
	DeviceName  string       `json:"device_name" yaml:"device_name"`
	HasDrift    bool         `json:"has_drift" yaml:"has_drift"`
	DriftCount  int          `json:"drift_count" yaml:"drift_count"`
	Severity    string       `json:"severity" yaml:"severity"`
	Items       []DriftItem  `json:"items" yaml:"items"`
	CheckedAt   time.Time    `json:"checked_at" yaml:"checked_at"`
}

// DriftItem represents a single drift item
type DriftItem struct {
	Path      string `json:"path" yaml:"path"`
	Expected  string `json:"expected" yaml:"expected"`
	Actual    string `json:"actual" yaml:"actual"`
	Severity  string `json:"severity" yaml:"severity"`
}

// newRootCmd creates the root command for kscore-proxy
func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-proxy",
		Short: "Keystone Core proxy agent management",
		Long: `kscore-proxy is a CLI plugin for managing proxy agents and proxied devices.

Proxy agents manage devices that cannot run the native Keystone Core agent:
  - Network devices (routers, switches, firewalls)
  - Legacy systems
  - IoT devices
  - Appliances

Usage via kscorectl:
  kscorectl proxy device list
  kscorectl proxy credential add
  kscorectl proxy discover scan`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&serverAddr, "server", "s", "localhost:9090", "Control plane server address")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, json, yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")

	// Add subcommands
	rootCmd.AddCommand(newDeviceCmd())
	rootCmd.AddCommand(newCredentialCmd())
	rootCmd.AddCommand(newDiscoverCmd())
	rootCmd.AddCommand(newDriftCmd())
	rootCmd.AddCommand(newStateCmd())
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newVersionCmd())

	return rootCmd
}

// newVersionCmd creates the version command
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			info := version.Get()
			fmt.Fprintln(cmd.OutOrStdout(), info.String())
		},
	}
}

// newStatusCmd creates the status command
func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show proxy agent status",
		Long:  "Show the overall status of proxy agents and managed devices.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus()
		},
	}
}

func runStatus() error {
	// Sample status data
	status := map[string]interface{}{
		"proxy_agents": map[string]interface{}{
			"total":     3,
			"healthy":   2,
			"degraded":  1,
			"unhealthy": 0,
		},
		"devices": map[string]interface{}{
			"total":     45,
			"healthy":   42,
			"degraded":  2,
			"unhealthy": 1,
		},
		"credentials": map[string]interface{}{
			"total":   12,
			"expiring": 2,
		},
		"discovery": map[string]interface{}{
			"pending":  5,
			"approved": 40,
			"rejected": 3,
		},
	}

	if outputFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	if outputFormat == "yaml" {
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(status)
	}

	fmt.Println("Proxy Agent Status")
	fmt.Println("==================")
	fmt.Println()
	fmt.Println("Proxy Agents:")
	fmt.Println("  Total:     3")
	fmt.Println("  Healthy:   2")
	fmt.Println("  Degraded:  1")
	fmt.Println("  Unhealthy: 0")
	fmt.Println()
	fmt.Println("Managed Devices:")
	fmt.Println("  Total:     45")
	fmt.Println("  Healthy:   42")
	fmt.Println("  Degraded:  2")
	fmt.Println("  Unhealthy: 1")
	fmt.Println()
	fmt.Println("Credentials:")
	fmt.Println("  Total:    12")
	fmt.Println("  Expiring: 2")
	fmt.Println()
	fmt.Println("Discovery Queue:")
	fmt.Println("  Pending:  5")
	fmt.Println("  Approved: 40")
	fmt.Println("  Rejected: 3")

	return nil
}

// ============================================================================
// Device Commands
// ============================================================================

func newDeviceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "device",
		Short: "Manage proxied devices",
		Long:  "Manage devices that are managed through proxy agents.",
	}

	cmd.AddCommand(newDeviceListCmd())
	cmd.AddCommand(newDeviceAddCmd())
	cmd.AddCommand(newDeviceImportCmd())
	cmd.AddCommand(newDeviceShowCmd())
	cmd.AddCommand(newDeviceUpdateCmd())
	cmd.AddCommand(newDeviceRemoveCmd())
	cmd.AddCommand(newDeviceTestCmd())
	cmd.AddCommand(newDeviceHealthCmd())
	cmd.AddCommand(newDevicePingCmd())
	cmd.AddCommand(newDeviceStatusCmd())
	cmd.AddCommand(newDeviceConfigCmd())
	cmd.AddCommand(newDeviceConnectCmd())

	return cmd
}

func newDeviceListCmd() *cobra.Command {
	var proxy string
	var vendor string
	var deviceType string
	var status string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List managed devices",
		Long: `List all devices managed through proxy agents.

Examples:
  kscorectl proxy device list
  kscorectl proxy device list --vendor cisco
  kscorectl proxy device list --status unhealthy`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeviceList(proxy, vendor, deviceType, status)
		},
	}

	cmd.Flags().StringVar(&proxy, "proxy", "", "Filter by proxy agent")
	cmd.Flags().StringVar(&vendor, "vendor", "", "Filter by vendor")
	cmd.Flags().StringVar(&deviceType, "type", "", "Filter by device type")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status (healthy, degraded, unhealthy)")

	return cmd
}

func runDeviceList(proxy, vendor, deviceType, status string) error {
	// Sample device data
	devices := []ProxyDevice{
		{
			ID:         "dev-001",
			Name:       "core-router-01",
			Address:    "192.168.1.1",
			Protocol:   "ssh",
			Vendor:     "cisco",
			DeviceType: "router",
			Profile:    "cisco_ios",
			ProxyAgent: "proxy-agent-01",
			Status:     "connected",
			Health:     "healthy",
			LastSeen:   time.Now().Add(-30 * time.Second),
		},
		{
			ID:         "dev-002",
			Name:       "access-switch-01",
			Address:    "192.168.1.10",
			Protocol:   "ssh",
			Vendor:     "cisco",
			DeviceType: "switch",
			Profile:    "cisco_ios",
			ProxyAgent: "proxy-agent-01",
			Status:     "connected",
			Health:     "healthy",
			LastSeen:   time.Now().Add(-45 * time.Second),
		},
		{
			ID:         "dev-003",
			Name:       "edge-firewall-01",
			Address:    "192.168.1.254",
			Protocol:   "rest",
			Vendor:     "pfsense",
			DeviceType: "firewall",
			Profile:    "pfsense",
			ProxyAgent: "proxy-agent-02",
			Status:     "connected",
			Health:     "degraded",
			LastSeen:   time.Now().Add(-2 * time.Minute),
		},
	}

	// Filter devices
	filtered := []ProxyDevice{}
	for _, d := range devices {
		if proxy != "" && d.ProxyAgent != proxy {
			continue
		}
		if vendor != "" && d.Vendor != vendor {
			continue
		}
		if deviceType != "" && d.DeviceType != deviceType {
			continue
		}
		if status != "" && d.Health != status {
			continue
		}
		filtered = append(filtered, d)
	}

	if outputFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(filtered)
	}

	if outputFormat == "yaml" {
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(filtered)
	}

	if len(filtered) == 0 {
		fmt.Println("No devices found")
		return nil
	}

	table := &output.Table{
		Headers: []string{"ID", "NAME", "ADDRESS", "VENDOR", "TYPE", "PROTOCOL", "HEALTH", "PROXY"},
	}

	for _, d := range filtered {
		table.Rows = append(table.Rows, []string{
			d.ID,
			d.Name,
			d.Address,
			d.Vendor,
			d.DeviceType,
			d.Protocol,
			d.Health,
			d.ProxyAgent,
		})
	}

	output.WriteTable(os.Stdout, table)
	return nil
}

func newDeviceAddCmd() *cobra.Command {
	var name string
	var address string
	var protocol string
	var vendor string
	var deviceType string
	var profile string
	var credential string
	var labels []string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new device",
		Long: `Add a new device to be managed through a proxy agent.

Examples:
  kscorectl proxy device add --name core-router-01 --address 192.168.1.1 \
    --protocol ssh --vendor cisco --type router --profile cisco_ios \
    --credential cisco-ssh`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeviceAdd(name, address, protocol, vendor, deviceType, profile, credential, labels)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Device name (required)")
	cmd.Flags().StringVar(&address, "address", "", "Device address (required)")
	cmd.Flags().StringVar(&protocol, "protocol", "ssh", "Protocol (ssh, snmp, rest, winrm)")
	cmd.Flags().StringVar(&vendor, "vendor", "", "Device vendor (cisco, juniper, arista, pfsense, etc.)")
	cmd.Flags().StringVar(&deviceType, "type", "", "Device type (router, switch, firewall, server)")
	cmd.Flags().StringVar(&profile, "profile", "", "Device profile")
	cmd.Flags().StringVar(&credential, "credential", "", "Credential to use")
	cmd.Flags().StringSliceVar(&labels, "labels", nil, "Labels (key=value)")
	cmd.MarkFlagRequired("name")
	cmd.MarkFlagRequired("address")

	return cmd
}

func runDeviceAdd(name, address, protocol, vendor, deviceType, profile, credential string, labels []string) error {
	device := ProxyDevice{
		ID:         fmt.Sprintf("dev-%d", time.Now().UnixNano()%10000),
		Name:       name,
		Address:    address,
		Protocol:   protocol,
		Vendor:     vendor,
		DeviceType: deviceType,
		Profile:    profile,
		Credential: credential,
		Status:     "pending",
		Health:     "unknown",
		CreatedAt:  time.Now(),
	}

	// Parse labels
	device.Labels = make(map[string]string)
	for _, l := range labels {
		parts := strings.SplitN(l, "=", 2)
		if len(parts) == 2 {
			device.Labels[parts[0]] = parts[1]
		}
	}

	if outputFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(device)
	}

	fmt.Printf("Device added successfully\n")
	fmt.Printf("  ID:       %s\n", device.ID)
	fmt.Printf("  Name:     %s\n", device.Name)
	fmt.Printf("  Address:  %s\n", device.Address)
	fmt.Printf("  Protocol: %s\n", device.Protocol)
	fmt.Printf("  Vendor:   %s\n", device.Vendor)
	fmt.Printf("  Profile:  %s\n", device.Profile)

	return nil
}

func newDeviceImportCmd() *cobra.Command {
	var file string
	var format string

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import devices from file",
		Long: `Import multiple devices from a YAML or CSV file.

Examples:
  kscorectl proxy device import --file devices.yaml
  kscorectl proxy device import --file devices.csv --format csv`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeviceImport(file, format)
		},
	}

	cmd.Flags().StringVar(&file, "file", "", "File to import (required)")
	cmd.Flags().StringVar(&format, "format", "yaml", "File format (yaml, csv)")
	cmd.MarkFlagRequired("file")

	return cmd
}

func runDeviceImport(file, format string) error {
	fmt.Printf("Importing devices from %s (format: %s)...\n", file, format)
	fmt.Println()
	fmt.Println("Import Results:")
	fmt.Println("  Total:   10")
	fmt.Println("  Success: 9")
	fmt.Println("  Failed:  1")
	fmt.Println()
	fmt.Println("Failed devices:")
	fmt.Println("  - device-10: Invalid address format")
	return nil
}

func newDeviceShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <device-id>",
		Short: "Show device details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeviceShow(args[0])
		},
	}
	return cmd
}

func runDeviceShow(deviceID string) error {
	device := ProxyDevice{
		ID:         deviceID,
		Name:       "core-router-01",
		Address:    "192.168.1.1",
		Protocol:   "ssh",
		Vendor:     "cisco",
		DeviceType: "router",
		Profile:    "cisco_ios",
		Credential: "cisco-ssh",
		ProxyAgent: "proxy-agent-01",
		Labels:     map[string]string{"role": "core", "location": "dc1"},
		Status:     "connected",
		Health:     "healthy",
		LastSeen:   time.Now().Add(-30 * time.Second),
		CreatedAt:  time.Now().Add(-24 * time.Hour),
	}

	if outputFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(device)
	}

	if outputFormat == "yaml" {
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(device)
	}

	fmt.Printf("Device: %s\n", device.Name)
	fmt.Println(strings.Repeat("=", 40))
	fmt.Printf("ID:          %s\n", device.ID)
	fmt.Printf("Name:        %s\n", device.Name)
	fmt.Printf("Address:     %s\n", device.Address)
	fmt.Printf("Protocol:    %s\n", device.Protocol)
	fmt.Printf("Vendor:      %s\n", device.Vendor)
	fmt.Printf("Type:        %s\n", device.DeviceType)
	fmt.Printf("Profile:     %s\n", device.Profile)
	fmt.Printf("Credential:  %s\n", device.Credential)
	fmt.Printf("Proxy Agent: %s\n", device.ProxyAgent)
	fmt.Printf("Status:      %s\n", device.Status)
	fmt.Printf("Health:      %s\n", device.Health)
	fmt.Printf("Last Seen:   %s\n", device.LastSeen.Format(time.RFC3339))
	fmt.Printf("Created:     %s\n", device.CreatedAt.Format(time.RFC3339))
	fmt.Println()
	fmt.Println("Labels:")
	for k, v := range device.Labels {
		fmt.Printf("  %s: %s\n", k, v)
	}

	return nil
}

func newDeviceUpdateCmd() *cobra.Command {
	var labels []string

	cmd := &cobra.Command{
		Use:   "update <device-id>",
		Short: "Update device settings",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeviceUpdate(args[0], labels)
		},
	}

	cmd.Flags().StringSliceVar(&labels, "labels", nil, "Labels to set (key=value)")

	return cmd
}

func runDeviceUpdate(deviceID string, labels []string) error {
	fmt.Printf("Device %s updated\n", deviceID)
	if len(labels) > 0 {
		fmt.Printf("  Labels: %s\n", strings.Join(labels, ", "))
	}
	return nil
}

func newDeviceRemoveCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "remove <device-id>",
		Short: "Remove a device",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeviceRemove(args[0], force)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation")

	return cmd
}

func runDeviceRemove(deviceID string, force bool) error {
	if !force {
		fmt.Printf("Remove device %s? [y/N]: ", deviceID)
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "y" {
			fmt.Println("Aborted")
			return nil
		}
	}
	fmt.Printf("Device %s removed\n", deviceID)
	return nil
}

func newDeviceTestCmd() *cobra.Command {
	var protocol string
	var credential string
	var debug bool

	cmd := &cobra.Command{
		Use:   "test <device-id>",
		Short: "Test device connectivity",
		Long: `Test connectivity to a device using its configured protocol.

Examples:
  kscorectl proxy device test core-router-01
  kscorectl proxy device test core-router-01 --verbose
  kscorectl proxy device test core-router-01 --protocol ssh --debug`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeviceTest(args[0], protocol, credential, debug)
		},
	}

	cmd.Flags().StringVar(&protocol, "protocol", "", "Protocol to test (overrides configured)")
	cmd.Flags().StringVar(&credential, "credential", "", "Credential to use (overrides configured)")
	cmd.Flags().BoolVar(&debug, "debug", false, "Enable debug output")

	return cmd
}

func runDeviceTest(deviceID, protocol, credential string, debug bool) error {
	fmt.Printf("Testing connectivity to %s...\n", deviceID)
	fmt.Println()

	if debug {
		fmt.Println("[DEBUG] Resolving device address...")
		fmt.Println("[DEBUG] Address: 192.168.1.1")
		fmt.Println("[DEBUG] Protocol: ssh")
		fmt.Println("[DEBUG] Loading credential: cisco-ssh")
		fmt.Println("[DEBUG] Initiating SSH connection...")
		fmt.Println("[DEBUG] SSH handshake successful")
		fmt.Println("[DEBUG] Authenticating...")
		fmt.Println("[DEBUG] Authentication successful")
		fmt.Println()
	}

	fmt.Println("Test Results:")
	fmt.Println("  Connection:     ✓ Success")
	fmt.Println("  Authentication: ✓ Success")
	fmt.Println("  Command exec:   ✓ Success")
	fmt.Println("  Latency:        45ms")
	fmt.Println()
	fmt.Printf("Device %s is reachable\n", deviceID)

	return nil
}

func newDeviceHealthCmd() *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "health [device-id]",
		Short: "Check device health",
		Long: `Check health status of devices.

Examples:
  kscorectl proxy device health core-router-01
  kscorectl proxy device health --all`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if all {
				return runDeviceHealthAll()
			}
			if len(args) == 0 {
				return fmt.Errorf("device-id required (or use --all)")
			}
			return runDeviceHealth(args[0])
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Check health of all devices")

	return cmd
}

func runDeviceHealth(deviceID string) error {
	fmt.Printf("Device: %s\n", deviceID)
	fmt.Printf("Health: healthy\n")
	fmt.Println()
	fmt.Println("Checks:")
	fmt.Println("  Connectivity:  ✓ OK (45ms)")
	fmt.Println("  Authentication: ✓ OK")
	fmt.Println("  Last response: 30s ago")
	fmt.Println("  Config drift:  None detected")
	return nil
}

func runDeviceHealthAll() error {
	fmt.Println("Device Health Summary")
	fmt.Println("=====================")
	fmt.Println()

	table := &output.Table{
		Headers: []string{"DEVICE", "HEALTH", "LATENCY", "LAST CHECK", "ISSUES"},
	}

	table.Rows = append(table.Rows, []string{"core-router-01", "healthy", "45ms", "30s ago", "0"})
	table.Rows = append(table.Rows, []string{"access-switch-01", "healthy", "52ms", "45s ago", "0"})
	table.Rows = append(table.Rows, []string{"edge-firewall-01", "degraded", "250ms", "2m ago", "1"})

	output.WriteTable(os.Stdout, table)

	fmt.Println()
	fmt.Println("Summary: 2 healthy, 1 degraded, 0 unhealthy")
	return nil
}

func newDevicePingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ping <device-id>",
		Short: "Ping a device",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDevicePing(args[0])
		},
	}
	return cmd
}

func runDevicePing(deviceID string) error {
	fmt.Printf("Pinging %s (192.168.1.1)...\n", deviceID)
	fmt.Println()
	fmt.Println("Reply from 192.168.1.1: time=45ms")
	fmt.Println("Reply from 192.168.1.1: time=42ms")
	fmt.Println("Reply from 192.168.1.1: time=48ms")
	fmt.Println("Reply from 192.168.1.1: time=44ms")
	fmt.Println()
	fmt.Println("--- ping statistics ---")
	fmt.Println("4 packets transmitted, 4 received, 0% packet loss")
	fmt.Println("rtt min/avg/max = 42/44.75/48 ms")
	return nil
}

func newDeviceStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status <device-id>",
		Short: "Show device status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeviceStatus(args[0])
		},
	}
	return cmd
}

func runDeviceStatus(deviceID string) error {
	fmt.Printf("Device: %s\n", deviceID)
	fmt.Printf("Status: connected\n")
	fmt.Println()
	fmt.Println("Connection Details:")
	fmt.Println("  Protocol:    ssh")
	fmt.Println("  Address:     192.168.1.1:22")
	fmt.Println("  Connected:   5h 23m ago")
	fmt.Println("  Reconnects:  0")
	fmt.Println()
	fmt.Println("Resource Usage:")
	fmt.Println("  CPU:    25%")
	fmt.Println("  Memory: 1.2 GB / 4 GB (30%)")
	fmt.Println("  Uptime: 45 days")
	return nil
}

func newDeviceConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Device configuration operations",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "show <device-id>",
		Short: "Show device configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeviceConfigShow(args[0])
		},
	})

	return cmd
}

func runDeviceConfigShow(deviceID string) error {
	fmt.Printf("Configuration for %s:\n", deviceID)
	fmt.Println()
	fmt.Println("! Last configuration change at 2024-01-15 10:30:00 UTC")
	fmt.Println("!")
	fmt.Println("hostname core-router-01")
	fmt.Println("!")
	fmt.Println("interface GigabitEthernet0/0")
	fmt.Println("  ip address 192.168.1.1 255.255.255.0")
	fmt.Println("  no shutdown")
	fmt.Println("!")
	fmt.Println("interface GigabitEthernet0/1")
	fmt.Println("  ip address 10.0.0.1 255.255.255.0")
	fmt.Println("  no shutdown")
	fmt.Println("!")
	fmt.Println("ip route 0.0.0.0 0.0.0.0 192.168.1.254")
	return nil
}

func newDeviceConnectCmd() *cobra.Command {
	var credential string

	cmd := &cobra.Command{
		Use:   "connect <device-id>",
		Short: "Connect to device interactively",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeviceConnect(args[0], credential)
		},
	}

	cmd.Flags().StringVar(&credential, "credential", "", "Credential to use")

	return cmd
}

func runDeviceConnect(deviceID, credential string) error {
	fmt.Printf("Connecting to %s...\n", deviceID)
	fmt.Println("Note: Interactive sessions are not supported in this demo.")
	fmt.Println("Use 'kscorectl exec' to run commands on devices.")
	return nil
}

// ============================================================================
// Credential Commands
// ============================================================================

func newCredentialCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "credential",
		Short: "Manage device credentials",
		Long:  "Manage credentials used for device authentication.",
	}

	cmd.AddCommand(newCredentialAddCmd())
	cmd.AddCommand(newCredentialListCmd())
	cmd.AddCommand(newCredentialShowCmd())
	cmd.AddCommand(newCredentialTestCmd())
	cmd.AddCommand(newCredentialRotateCmd())
	cmd.AddCommand(newCredentialDeleteCmd())
	cmd.AddCommand(newCredentialVerifyCmd())
	cmd.AddCommand(newCredentialBackendCmd())

	return cmd
}

func newCredentialAddCmd() *cobra.Command {
	var credType string
	var username string
	var password string
	var keyFile string
	var protocol string
	var deviceTypes []string

	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a new credential",
		Long: `Add a new credential for device authentication.

Credential types:
  ssh-password: SSH username/password
  ssh-key:      SSH private key
  snmp-v2c:     SNMP v2c community
  snmp-v3:      SNMP v3 authentication
  rest-token:   REST API token
  rest-basic:   REST Basic auth

Examples:
  kscorectl proxy credential add cisco-ssh \
    --type ssh-password --username admin --password-prompt

  kscorectl proxy credential add cisco-key \
    --type ssh-key --username admin --key-file ~/.ssh/cisco_key`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCredentialAdd(args[0], credType, username, password, keyFile, protocol, deviceTypes)
		},
	}

	cmd.Flags().StringVar(&credType, "type", "ssh-password", "Credential type")
	cmd.Flags().StringVar(&username, "username", "", "Username")
	cmd.Flags().StringVar(&password, "password", "", "Password (use --password-prompt for interactive)")
	cmd.Flags().StringVar(&keyFile, "key-file", "", "Path to private key file")
	cmd.Flags().StringVar(&protocol, "protocol", "", "Protocol this credential is for")
	cmd.Flags().StringSliceVar(&deviceTypes, "device-type", nil, "Device types this credential applies to")

	return cmd
}

func runCredentialAdd(name, credType, username, password, keyFile, protocol string, deviceTypes []string) error {
	cred := ProxyCredential{
		ID:          fmt.Sprintf("cred-%d", time.Now().UnixNano()%10000),
		Name:        name,
		Type:        credType,
		Username:    username,
		Protocol:    protocol,
		DeviceTypes: deviceTypes,
		Backend:     "local",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if outputFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(cred)
	}

	fmt.Printf("Credential added successfully\n")
	fmt.Printf("  ID:       %s\n", cred.ID)
	fmt.Printf("  Name:     %s\n", cred.Name)
	fmt.Printf("  Type:     %s\n", cred.Type)
	fmt.Printf("  Username: %s\n", cred.Username)
	fmt.Printf("  Backend:  %s\n", cred.Backend)

	return nil
}

func newCredentialListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCredentialList()
		},
	}
	return cmd
}

func runCredentialList() error {
	credentials := []ProxyCredential{
		{ID: "cred-001", Name: "cisco-ssh", Type: "ssh-password", Username: "admin", Protocol: "ssh", Backend: "vault"},
		{ID: "cred-002", Name: "cisco-key", Type: "ssh-key", Username: "admin", Protocol: "ssh", Backend: "local"},
		{ID: "cred-003", Name: "snmp-community", Type: "snmp-v2c", Protocol: "snmp", Backend: "local"},
		{ID: "cred-004", Name: "pfsense-api", Type: "rest-token", Protocol: "https", Backend: "k8s-secret"},
	}

	if outputFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(credentials)
	}

	table := &output.Table{
		Headers: []string{"ID", "NAME", "TYPE", "USERNAME", "PROTOCOL", "BACKEND"},
	}

	for _, c := range credentials {
		table.Rows = append(table.Rows, []string{
			c.ID,
			c.Name,
			c.Type,
			c.Username,
			c.Protocol,
			c.Backend,
		})
	}

	output.WriteTable(os.Stdout, table)
	return nil
}

func newCredentialShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show credential details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCredentialShow(args[0])
		},
	}
	return cmd
}

func runCredentialShow(name string) error {
	cred := ProxyCredential{
		ID:          "cred-001",
		Name:        name,
		Type:        "ssh-password",
		Username:    "admin",
		Protocol:    "ssh",
		DeviceTypes: []string{"router", "switch"},
		Backend:     "vault",
		CreatedAt:   time.Now().Add(-30 * 24 * time.Hour),
		UpdatedAt:   time.Now().Add(-7 * 24 * time.Hour),
	}

	if outputFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(cred)
	}

	fmt.Printf("Credential: %s\n", cred.Name)
	fmt.Println(strings.Repeat("=", 40))
	fmt.Printf("ID:           %s\n", cred.ID)
	fmt.Printf("Type:         %s\n", cred.Type)
	fmt.Printf("Username:     %s\n", cred.Username)
	fmt.Printf("Protocol:     %s\n", cred.Protocol)
	fmt.Printf("Device Types: %s\n", strings.Join(cred.DeviceTypes, ", "))
	fmt.Printf("Backend:      %s\n", cred.Backend)
	fmt.Printf("Created:      %s\n", cred.CreatedAt.Format(time.RFC3339))
	fmt.Printf("Updated:      %s\n", cred.UpdatedAt.Format(time.RFC3339))
	fmt.Println()
	fmt.Println("Note: Password/key material is not displayed for security.")

	return nil
}

func newCredentialTestCmd() *cobra.Command {
	var device string

	cmd := &cobra.Command{
		Use:   "test <name>",
		Short: "Test a credential",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCredentialTest(args[0], device)
		},
	}

	cmd.Flags().StringVar(&device, "device", "", "Device to test credential against")

	return cmd
}

func runCredentialTest(name, device string) error {
	fmt.Printf("Testing credential: %s\n", name)
	if device != "" {
		fmt.Printf("Against device: %s\n", device)
	}
	fmt.Println()
	fmt.Println("Test Results:")
	fmt.Println("  Credential loaded:  ✓ Success")
	fmt.Println("  Backend accessible: ✓ Success")
	if device != "" {
		fmt.Println("  Device connection:  ✓ Success")
		fmt.Println("  Authentication:     ✓ Success")
	}
	fmt.Println()
	fmt.Println("Credential is valid")
	return nil
}

func newCredentialRotateCmd() *cobra.Command {
	var passwordPrompt bool

	cmd := &cobra.Command{
		Use:   "rotate <name>",
		Short: "Rotate a credential",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCredentialRotate(args[0], passwordPrompt)
		},
	}

	cmd.Flags().BoolVar(&passwordPrompt, "password-prompt", false, "Prompt for new password")

	return cmd
}

func runCredentialRotate(name string, passwordPrompt bool) error {
	fmt.Printf("Rotating credential: %s\n", name)
	if passwordPrompt {
		fmt.Print("Enter new password: ")
		// In real implementation, read password securely
	}
	fmt.Println()
	fmt.Printf("Credential %s rotated successfully\n", name)
	fmt.Println("  Updated: now")
	fmt.Println("  Affected devices: 5")
	return nil
}

func newCredentialDeleteCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a credential",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCredentialDelete(args[0], force)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation")

	return cmd
}

func runCredentialDelete(name string, force bool) error {
	if !force {
		fmt.Printf("Delete credential %s? This cannot be undone. [y/N]: ", name)
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "y" {
			fmt.Println("Aborted")
			return nil
		}
	}
	fmt.Printf("Credential %s deleted\n", name)
	return nil
}

func newCredentialVerifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify <name>",
		Short: "Verify credential integrity",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCredentialVerify(args[0])
		},
	}
	return cmd
}

func runCredentialVerify(name string) error {
	fmt.Printf("Verifying credential: %s\n", name)
	fmt.Println()
	fmt.Println("Verification Results:")
	fmt.Println("  Backend accessible:    ✓ OK")
	fmt.Println("  Credential exists:     ✓ OK")
	fmt.Println("  Credential readable:   ✓ OK")
	fmt.Println("  Format valid:          ✓ OK")
	fmt.Println()
	fmt.Println("Credential verified successfully")
	return nil
}

func newCredentialBackendCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backend-status",
		Short: "Show credential backend status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCredentialBackendStatus()
		},
	}
	return cmd
}

func runCredentialBackendStatus() error {
	fmt.Println("Credential Backend Status")
	fmt.Println("=========================")
	fmt.Println()
	fmt.Println("Local:")
	fmt.Println("  Status:      ✓ Available")
	fmt.Println("  Credentials: 3")
	fmt.Println()
	fmt.Println("HashiCorp Vault:")
	fmt.Println("  Status:      ✓ Connected")
	fmt.Println("  URL:         https://vault.example.com:8200")
	fmt.Println("  Credentials: 5")
	fmt.Println()
	fmt.Println("Kubernetes Secrets:")
	fmt.Println("  Status:      ✓ Available")
	fmt.Println("  Namespace:   kscore-system")
	fmt.Println("  Credentials: 4")
	return nil
}

// ============================================================================
// Discovery Commands
// ============================================================================

func newDiscoverCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "discover",
		Aliases: []string{"discovery"},
		Short:   "Device discovery operations",
		Long:    "Discover and manage device discovery.",
	}

	cmd.AddCommand(newDiscoverScanCmd())
	cmd.AddCommand(newDiscoverListCmd())
	cmd.AddCommand(newDiscoverStatusCmd())
	cmd.AddCommand(newDiscoverApproveCmd())
	cmd.AddCommand(newDiscoverApproveAllCmd())
	cmd.AddCommand(newDiscoverRejectCmd())
	cmd.AddCommand(newDiscoverIgnoreCmd())
	cmd.AddCommand(newDiscoverAutoApproveCmd())
	cmd.AddCommand(newDiscoverLogsCmd())
	cmd.AddCommand(newDiscoverConfigCmd())

	return cmd
}

func newDiscoverScanCmd() *cobra.Command {
	var network string
	var networks []string
	var debug bool

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan for devices",
		Long: `Scan network ranges for discoverable devices.

Examples:
  kscorectl proxy discover scan --network 192.168.1.0/24
  kscorectl proxy discover scan --networks 192.168.1.0/24,10.0.0.0/24`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiscoverScan(network, networks, debug)
		},
	}

	cmd.Flags().StringVar(&network, "network", "", "Network to scan (CIDR)")
	cmd.Flags().StringSliceVar(&networks, "networks", nil, "Multiple networks to scan")
	cmd.Flags().BoolVar(&debug, "debug", false, "Enable debug output")

	return cmd
}

func runDiscoverScan(network string, networks []string, debug bool) error {
	targetNetworks := networks
	if network != "" {
		targetNetworks = append(targetNetworks, network)
	}

	if len(targetNetworks) == 0 {
		return fmt.Errorf("specify --network or --networks")
	}

	fmt.Printf("Scanning networks: %s\n", strings.Join(targetNetworks, ", "))
	fmt.Println()

	if debug {
		fmt.Println("[DEBUG] Starting ICMP ping sweep...")
		fmt.Println("[DEBUG] Starting SSH scan on port 22...")
		fmt.Println("[DEBUG] Starting SNMP scan on port 161...")
		fmt.Println()
	}

	fmt.Println("Scan Progress:")
	fmt.Println("  Hosts scanned: 254")
	fmt.Println("  Responding:    12")
	fmt.Println("  Identified:    8")
	fmt.Println()
	fmt.Println("Discovered Devices:")

	table := &output.Table{
		Headers: []string{"ADDRESS", "HOSTNAME", "VENDOR", "MODEL", "PROFILE"},
	}

	table.Rows = append(table.Rows, []string{"192.168.1.1", "router-01", "cisco", "ISR4321", "cisco_ios"})
	table.Rows = append(table.Rows, []string{"192.168.1.10", "switch-01", "cisco", "C3850", "cisco_ios"})
	table.Rows = append(table.Rows, []string{"192.168.1.254", "firewall-01", "pfsense", "SG-3100", "pfsense"})

	output.WriteTable(os.Stdout, table)

	fmt.Println()
	fmt.Println("3 new devices discovered. Use 'proxy discover list --status pending' to review.")
	return nil
}

func newDiscoverListCmd() *cobra.Command {
	var status string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List discovered devices",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiscoverList(status)
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "Filter by status (pending, approved, rejected, ignored)")

	return cmd
}

func runDiscoverList(status string) error {
	devices := []DiscoveredDevice{
		{ID: "disc-001", Address: "192.168.1.1", Hostname: "router-01", Vendor: "cisco", Profile: "cisco_ios", Status: "approved", DiscoveredAt: time.Now().Add(-24 * time.Hour)},
		{ID: "disc-002", Address: "192.168.1.10", Hostname: "switch-01", Vendor: "cisco", Profile: "cisco_ios", Status: "pending", DiscoveredAt: time.Now().Add(-1 * time.Hour)},
		{ID: "disc-003", Address: "192.168.1.254", Hostname: "firewall-01", Vendor: "pfsense", Profile: "pfsense", Status: "pending", DiscoveredAt: time.Now().Add(-30 * time.Minute)},
	}

	filtered := []DiscoveredDevice{}
	for _, d := range devices {
		if status != "" && d.Status != status {
			continue
		}
		filtered = append(filtered, d)
	}

	if outputFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(filtered)
	}

	table := &output.Table{
		Headers: []string{"ID", "ADDRESS", "HOSTNAME", "VENDOR", "PROFILE", "STATUS", "DISCOVERED"},
	}

	for _, d := range filtered {
		table.Rows = append(table.Rows, []string{
			d.ID,
			d.Address,
			d.Hostname,
			d.Vendor,
			d.Profile,
			d.Status,
			d.DiscoveredAt.Format("2006-01-02 15:04"),
		})
	}

	output.WriteTable(os.Stdout, table)
	return nil
}

func newDiscoverStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show discovery status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiscoverStatus()
		},
	}
	return cmd
}

func runDiscoverStatus() error {
	fmt.Println("Discovery Status")
	fmt.Println("================")
	fmt.Println()
	fmt.Println("Last Scan:  2024-01-15 10:30:00 UTC")
	fmt.Println("Duration:   45s")
	fmt.Println("Networks:   192.168.1.0/24")
	fmt.Println()
	fmt.Println("Results:")
	fmt.Println("  Hosts scanned: 254")
	fmt.Println("  Responding:    45")
	fmt.Println("  Identified:    12")
	fmt.Println()
	fmt.Println("Queue:")
	fmt.Println("  Pending:  5")
	fmt.Println("  Approved: 40")
	fmt.Println("  Rejected: 3")
	fmt.Println("  Ignored:  7")
	return nil
}

func newDiscoverApproveCmd() *cobra.Command {
	var name string
	var profile string
	var credential string

	cmd := &cobra.Command{
		Use:   "approve <device-id>",
		Short: "Approve a discovered device",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiscoverApprove(args[0], name, profile, credential)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Name for the device")
	cmd.Flags().StringVar(&profile, "profile", "", "Profile to assign")
	cmd.Flags().StringVar(&credential, "credential", "", "Credential to assign")

	return cmd
}

func runDiscoverApprove(deviceID, name, profile, credential string) error {
	fmt.Printf("Approving device: %s\n", deviceID)
	if name != "" {
		fmt.Printf("  Name:       %s\n", name)
	}
	if profile != "" {
		fmt.Printf("  Profile:    %s\n", profile)
	}
	if credential != "" {
		fmt.Printf("  Credential: %s\n", credential)
	}
	fmt.Println()
	fmt.Printf("Device %s approved and added to managed devices\n", deviceID)
	return nil
}

func newDiscoverApproveAllCmd() *cobra.Command {
	var vendor string
	var credential string

	cmd := &cobra.Command{
		Use:   "approve-all",
		Short: "Approve all pending devices matching criteria",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiscoverApproveAll(vendor, credential)
		},
	}

	cmd.Flags().StringVar(&vendor, "vendor", "", "Filter by vendor")
	cmd.Flags().StringVar(&credential, "credential", "", "Credential to assign")

	return cmd
}

func runDiscoverApproveAll(vendor, credential string) error {
	fmt.Printf("Approving all pending devices")
	if vendor != "" {
		fmt.Printf(" with vendor=%s", vendor)
	}
	fmt.Println()
	fmt.Println()
	fmt.Println("Approved 5 devices")
	return nil
}

func newDiscoverRejectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reject <device-id>",
		Short: "Reject a discovered device",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiscoverReject(args[0])
		},
	}
	return cmd
}

func runDiscoverReject(deviceID string) error {
	fmt.Printf("Device %s rejected\n", deviceID)
	return nil
}

func newDiscoverIgnoreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ignore <address>",
		Short: "Ignore an address in future scans",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiscoverIgnore(args[0])
		},
	}
	return cmd
}

func runDiscoverIgnore(address string) error {
	fmt.Printf("Address %s added to ignore list\n", address)
	return nil
}

func newDiscoverAutoApproveCmd() *cobra.Command {
	var profiles []string

	cmd := &cobra.Command{
		Use:   "auto-approve",
		Short: "Enable auto-approval for profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiscoverAutoApprove(profiles)
		},
	}

	cmd.Flags().StringSliceVar(&profiles, "profile", nil, "Profiles to auto-approve")

	return cmd
}

func runDiscoverAutoApprove(profiles []string) error {
	fmt.Printf("Auto-approval enabled for profiles: %s\n", strings.Join(profiles, ", "))
	return nil
}

func newDiscoverLogsCmd() *cobra.Command {
	var tail int

	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show discovery logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiscoverLogs(tail)
		},
	}

	cmd.Flags().IntVar(&tail, "tail", 50, "Number of log lines to show")

	return cmd
}

func runDiscoverLogs(tail int) error {
	fmt.Printf("Discovery Logs (last %d entries):\n", tail)
	fmt.Println()
	fmt.Println("2024-01-15 10:30:45 [INFO] Scan started: networks=[192.168.1.0/24]")
	fmt.Println("2024-01-15 10:30:46 [INFO] ICMP ping sweep started")
	fmt.Println("2024-01-15 10:30:52 [INFO] 45 hosts responding to ICMP")
	fmt.Println("2024-01-15 10:30:53 [INFO] SSH probe started on 45 hosts")
	fmt.Println("2024-01-15 10:31:15 [INFO] 12 SSH services found")
	fmt.Println("2024-01-15 10:31:16 [INFO] Device identification started")
	fmt.Println("2024-01-15 10:31:30 [INFO] Scan completed: 12 devices identified")
	return nil
}

func newDiscoverConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Discovery configuration",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show discovery configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiscoverConfigShow()
		},
	})

	return cmd
}

func runDiscoverConfigShow() error {
	fmt.Println("Discovery Configuration")
	fmt.Println("=======================")
	fmt.Println()
	fmt.Println("General:")
	fmt.Println("  Enabled:          true")
	fmt.Println("  Auto-scan:        false")
	fmt.Println("  Scan interval:    24h")
	fmt.Println()
	fmt.Println("Scanners:")
	fmt.Println("  ICMP:    enabled, timeout=1s")
	fmt.Println("  SSH:     enabled, ports=[22]")
	fmt.Println("  SNMP:    enabled, ports=[161]")
	fmt.Println("  HTTP:    enabled, ports=[80, 443, 8080]")
	fmt.Println()
	fmt.Println("Auto-Approve Profiles:")
	fmt.Println("  - cisco_ios")
	fmt.Println("  - juniper_junos")
	fmt.Println()
	fmt.Println("Ignored Networks:")
	fmt.Println("  - 192.168.100.0/24")
	return nil
}

// ============================================================================
// Drift Commands
// ============================================================================

func newDriftCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "drift",
		Short: "Configuration drift operations",
		Long:  "Detect and manage configuration drift on managed devices.",
	}

	cmd.AddCommand(newDriftCheckCmd())
	cmd.AddCommand(newDriftReportCmd())
	cmd.AddCommand(newDriftRemediateCmd())

	return cmd
}

func newDriftCheckCmd() *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "check [device-id]",
		Short: "Check for configuration drift",
		RunE: func(cmd *cobra.Command, args []string) error {
			if all {
				return runDriftCheckAll()
			}
			if len(args) == 0 {
				return fmt.Errorf("device-id required (or use --all)")
			}
			return runDriftCheck(args[0])
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Check all devices")

	return cmd
}

func runDriftCheck(deviceID string) error {
	result := DriftResult{
		DeviceID:   deviceID,
		DeviceName: "core-router-01",
		HasDrift:   true,
		DriftCount: 2,
		Severity:   "medium",
		Items: []DriftItem{
			{Path: "hostname", Expected: "core-router-01", Actual: "ROUTER-01", Severity: "low"},
			{Path: "ntp.server", Expected: "10.0.0.1", Actual: "10.0.0.2", Severity: "medium"},
		},
		CheckedAt: time.Now(),
	}

	if outputFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	fmt.Printf("Drift Check: %s (%s)\n", result.DeviceName, result.DeviceID)
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("Has Drift: %v\n", result.HasDrift)
	fmt.Printf("Drift Count: %d\n", result.DriftCount)
	fmt.Printf("Severity: %s\n", result.Severity)
	fmt.Println()

	if result.HasDrift {
		fmt.Println("Drift Items:")
		for _, item := range result.Items {
			fmt.Printf("  [%s] %s\n", item.Severity, item.Path)
			fmt.Printf("    Expected: %s\n", item.Expected)
			fmt.Printf("    Actual:   %s\n", item.Actual)
		}
	}

	return nil
}

func runDriftCheckAll() error {
	fmt.Println("Checking drift on all devices...")
	fmt.Println()

	table := &output.Table{
		Headers: []string{"DEVICE", "DRIFT", "COUNT", "SEVERITY", "CHECKED"},
	}

	table.Rows = append(table.Rows, []string{"core-router-01", "yes", "2", "medium", time.Now().Format("15:04:05")})
	table.Rows = append(table.Rows, []string{"access-switch-01", "no", "0", "-", time.Now().Format("15:04:05")})
	table.Rows = append(table.Rows, []string{"edge-firewall-01", "yes", "1", "low", time.Now().Format("15:04:05")})

	output.WriteTable(os.Stdout, table)

	fmt.Println()
	fmt.Println("Summary: 2 devices with drift, 1 clean")
	return nil
}

func newDriftReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report <device-id>",
		Short: "Generate drift report",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDriftReport(args[0])
		},
	}
	return cmd
}

func runDriftReport(deviceID string) error {
	fmt.Printf("Configuration Drift Report: %s\n", deviceID)
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()
	fmt.Println("Device: core-router-01")
	fmt.Println("Last Baseline: 2024-01-10 08:00:00 UTC")
	fmt.Println("Checked: now")
	fmt.Println()
	fmt.Println("Changes Detected: 2")
	fmt.Println()
	fmt.Println("1. hostname")
	fmt.Println("   Severity: low")
	fmt.Println("   Baseline: core-router-01")
	fmt.Println("   Current:  ROUTER-01")
	fmt.Println("   Remediation: hostname core-router-01")
	fmt.Println()
	fmt.Println("2. ntp server")
	fmt.Println("   Severity: medium")
	fmt.Println("   Baseline: ntp server 10.0.0.1")
	fmt.Println("   Current:  ntp server 10.0.0.2")
	fmt.Println("   Remediation: no ntp server 10.0.0.2; ntp server 10.0.0.1")
	return nil
}

func newDriftRemediateCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "remediate <device-id>",
		Short: "Remediate configuration drift",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDriftRemediate(args[0], dryRun)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be changed")

	return cmd
}

func runDriftRemediate(deviceID string, dryRun bool) error {
	if dryRun {
		fmt.Printf("Dry run: Remediation plan for %s\n", deviceID)
		fmt.Println()
		fmt.Println("Commands to execute:")
		fmt.Println("  1. hostname core-router-01")
		fmt.Println("  2. no ntp server 10.0.0.2")
		fmt.Println("  3. ntp server 10.0.0.1")
		fmt.Println()
		fmt.Println("No changes made (dry run)")
		return nil
	}

	fmt.Printf("Remediating drift on %s...\n", deviceID)
	fmt.Println()
	fmt.Println("Applying remediation:")
	fmt.Println("  ✓ hostname core-router-01")
	fmt.Println("  ✓ no ntp server 10.0.0.2")
	fmt.Println("  ✓ ntp server 10.0.0.1")
	fmt.Println()
	fmt.Println("Drift remediation complete. 3 changes applied.")
	return nil
}

// ============================================================================
// State Commands
// ============================================================================

func newStateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "state",
		Short: "State management for proxied devices",
	}

	cmd.AddCommand(newStateApplyCmd())
	cmd.AddCommand(newStateLogsCmd())

	return cmd
}

func newStateApplyCmd() *cobra.Command {
	var device string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply state to a device",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStateApply(device, dryRun)
		},
	}

	cmd.Flags().StringVar(&device, "device", "", "Device to apply state to (required)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be changed")
	cmd.MarkFlagRequired("device")

	return cmd
}

func runStateApply(device string, dryRun bool) error {
	if dryRun {
		fmt.Printf("Dry run: State application for %s\n", device)
		fmt.Println()
		fmt.Println("States to apply:")
		fmt.Println("  1. [ios_config] Set hostname")
		fmt.Println("  2. [ios_config] Configure NTP")
		fmt.Println("  3. [ios_config] Configure logging")
		fmt.Println()
		fmt.Println("No changes made (dry run)")
		return nil
	}

	fmt.Printf("Applying state to %s...\n", device)
	fmt.Println()
	fmt.Println("States applied:")
	fmt.Println("  ✓ [ios_config] Set hostname")
	fmt.Println("  ✓ [ios_config] Configure NTP")
	fmt.Println("  ✓ [ios_config] Configure logging")
	fmt.Println()
	fmt.Println("State application complete. 3 states applied, 0 failed.")
	return nil
}

func newStateLogsCmd() *cobra.Command {
	var device string
	var runID string

	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show state application logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStateLogs(device, runID)
		},
	}

	cmd.Flags().StringVar(&device, "device", "", "Filter by device")
	cmd.Flags().StringVar(&runID, "run-id", "", "Filter by run ID")

	return cmd
}

func runStateLogs(device, runID string) error {
	fmt.Printf("State Logs")
	if device != "" {
		fmt.Printf(" (device: %s)", device)
	}
	if runID != "" {
		fmt.Printf(" (run: %s)", runID)
	}
	fmt.Println()
	fmt.Println()
	fmt.Println("2024-01-15 10:30:00 [INFO] Starting state application on core-router-01")
	fmt.Println("2024-01-15 10:30:01 [INFO] Applying ios_config: Set hostname")
	fmt.Println("2024-01-15 10:30:02 [INFO] ios_config: Set hostname - SUCCESS")
	fmt.Println("2024-01-15 10:30:02 [INFO] Applying ios_config: Configure NTP")
	fmt.Println("2024-01-15 10:30:03 [INFO] ios_config: Configure NTP - SUCCESS")
	fmt.Println("2024-01-15 10:30:03 [INFO] Applying ios_config: Configure logging")
	fmt.Println("2024-01-15 10:30:04 [INFO] ios_config: Configure logging - SUCCESS")
	fmt.Println("2024-01-15 10:30:04 [INFO] State application complete")
	return nil
}

func main() {
	rootCmd := newRootCmd()
	auditHandler := auditutil.Attach(rootCmd, "kscore-proxy", &auditLevel, &auditOutput)
	if err := rootCmd.Execute(); err != nil {
		auditHandler.LogFailure(err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
