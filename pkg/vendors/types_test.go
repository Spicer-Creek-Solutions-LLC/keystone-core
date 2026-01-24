package vendors

import (
	"context"
	"testing"
	"time"
)

func TestVendorTypeConstants(t *testing.T) {
	tests := []struct {
		vendorType VendorType
		expected   string
	}{
		{VendorCiscoIOS, "cisco_ios"},
		{VendorCiscoNXOS, "cisco_nxos"},
		{VendorJuniperJUNOS, "juniper_junos"},
		{VendorAristaEOS, "arista_eos"},
		{VendorPfSense, "pfsense"},
		{VendorOPNsense, "opnsense"},
		{VendorVyOS, "vyos"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.vendorType) != tt.expected {
				t.Errorf("VendorType = %v, want %v", tt.vendorType, tt.expected)
			}
		})
	}
}

func TestDefaultVendorConfig(t *testing.T) {
	cfg := DefaultVendorConfig()

	if cfg.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", cfg.Timeout)
	}
	if cfg.EnablePrompt != "#" {
		t.Errorf("EnablePrompt = %v, want '#'", cfg.EnablePrompt)
	}
	if cfg.ConfigPrompt != "(config" {
		t.Errorf("ConfigPrompt = %v, want '(config'", cfg.ConfigPrompt)
	}
	if cfg.DisablePaging != true {
		t.Errorf("DisablePaging = %v, want true", cfg.DisablePaging)
	}
	if cfg.PrivilegeLevel != 15 {
		t.Errorf("PrivilegeLevel = %d, want 15", cfg.PrivilegeLevel)
	}
}

func TestVendorConfigStructure(t *testing.T) {
	cfg := &VendorConfig{
		Timeout:        120 * time.Second,
		EnablePrompt:   ">",
		ConfigPrompt:   "(config)",
		EnablePassword: "secret123",
		PrivilegeLevel: 7,
		DisablePaging:  false,
	}

	if cfg.Timeout != 120*time.Second {
		t.Errorf("Timeout = %v", cfg.Timeout)
	}
	if cfg.EnablePrompt != ">" {
		t.Errorf("EnablePrompt = %v", cfg.EnablePrompt)
	}
	if cfg.EnablePassword != "secret123" {
		t.Errorf("EnablePassword = %v", cfg.EnablePassword)
	}
}

func TestNewBaseVendorAdapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter, err := NewBaseVendorAdapter(nil, VendorCiscoIOS)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if adapter == nil {
			t.Fatal("adapter should not be nil")
		}
		if adapter.Config == nil {
			t.Error("Config should be set to default")
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &VendorConfig{
			Timeout: 30 * time.Second,
		}
		adapter, err := NewBaseVendorAdapter(cfg, VendorJuniperJUNOS)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if adapter.Config.Timeout != 30*time.Second {
			t.Errorf("Timeout = %v, want 30s", adapter.Config.Timeout)
		}
	})
}

func TestBaseVendorAdapterType(t *testing.T) {
	t.Run("nil protocol", func(t *testing.T) {
		adapter := &BaseVendorAdapter{}
		if adapter.Type() != "vendor" {
			t.Errorf("Type() = %v, want 'vendor'", adapter.Type())
		}
	})
}

func TestBaseVendorAdapterConnect(t *testing.T) {
	adapter := &BaseVendorAdapter{}

	// Connect with nil protocol should succeed
	err := adapter.Connect(context.Background(), nil, nil)
	if err != nil {
		t.Errorf("Connect() error = %v, want nil", err)
	}
}

func TestBaseVendorAdapterDisconnect(t *testing.T) {
	adapter := &BaseVendorAdapter{Connected: true}

	err := adapter.Disconnect(context.Background())
	if err != nil {
		t.Errorf("Disconnect() error = %v, want nil", err)
	}
	if adapter.Connected {
		t.Error("Connected should be false after disconnect")
	}
}

func TestBaseVendorAdapterExecute(t *testing.T) {
	adapter := &BaseVendorAdapter{}

	result, err := adapter.Execute(context.Background(), nil)
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}
	// With nil protocol, result should be nil
	if result != nil {
		t.Errorf("result should be nil with nil protocol")
	}
}

func TestBaseVendorAdapterHealthCheck(t *testing.T) {
	t.Run("nil protocol", func(t *testing.T) {
		adapter := &BaseVendorAdapter{Connected: true}

		result, err := adapter.HealthCheck(context.Background())
		if err != nil {
			t.Errorf("HealthCheck() error = %v", err)
		}
		if result == nil {
			t.Fatal("result should not be nil")
		}
		if result.Status != "unknown" {
			t.Errorf("Status = %v, want 'unknown'", result.Status)
		}
		if !result.Healthy {
			t.Error("Healthy should be true when Connected is true")
		}
	})

	t.Run("disconnected", func(t *testing.T) {
		adapter := &BaseVendorAdapter{Connected: false}

		result, err := adapter.HealthCheck(context.Background())
		if err != nil {
			t.Errorf("HealthCheck() error = %v", err)
		}
		if result.Healthy {
			t.Error("Healthy should be false when Connected is false")
		}
	})
}

func TestBaseVendorAdapterIsConnected(t *testing.T) {
	t.Run("nil protocol", func(t *testing.T) {
		adapter := &BaseVendorAdapter{Connected: true}
		if !adapter.IsConnected() {
			t.Error("IsConnected() should return true")
		}
	})

	t.Run("not connected", func(t *testing.T) {
		adapter := &BaseVendorAdapter{Connected: false}
		if adapter.IsConnected() {
			t.Error("IsConnected() should return false")
		}
	})
}

func TestBaseVendorAdapterMetrics(t *testing.T) {
	adapter := &BaseVendorAdapter{}
	metrics := adapter.Metrics()
	if metrics == nil {
		t.Fatal("Metrics() should not return nil")
	}
}

func TestBaseVendorAdapterSetConnected(t *testing.T) {
	adapter := &BaseVendorAdapter{}

	adapter.SetConnected(true)
	if !adapter.Connected {
		t.Error("Connected should be true")
	}

	adapter.SetConnected(false)
	if adapter.Connected {
		t.Error("Connected should be false")
	}
}

func TestNewVendorRegistry(t *testing.T) {
	registry := NewVendorRegistry()
	if registry == nil {
		t.Fatal("registry should not be nil")
	}
	if registry.factories == nil {
		t.Error("factories map should be initialized")
	}
}

func TestVendorRegistryRegister(t *testing.T) {
	registry := NewVendorRegistry()

	factory := func(config *VendorConfig) (VendorAdapter, error) {
		return nil, nil
	}

	registry.Register(VendorCiscoIOS, factory)

	vendors := registry.ListVendors()
	if len(vendors) != 1 {
		t.Errorf("ListVendors() returned %d vendors, want 1", len(vendors))
	}
}

func TestVendorRegistryCreate(t *testing.T) {
	registry := NewVendorRegistry()

	t.Run("vendor not found", func(t *testing.T) {
		_, err := registry.Create(VendorCiscoIOS, nil)
		if err == nil {
			t.Error("expected error for unregistered vendor")
		}
		vendorErr, ok := err.(*VendorNotFoundError)
		if !ok {
			t.Errorf("expected VendorNotFoundError, got %T", err)
		} else if vendorErr.Vendor != VendorCiscoIOS {
			t.Errorf("Vendor = %v, want %v", vendorErr.Vendor, VendorCiscoIOS)
		}
	})

	t.Run("vendor found", func(t *testing.T) {
		factory := func(config *VendorConfig) (VendorAdapter, error) {
			adapter, _ := NewBaseVendorAdapter(config, VendorCiscoIOS)
			return &testVendorAdapter{BaseVendorAdapter: *adapter}, nil
		}
		registry.Register(VendorCiscoIOS, factory)

		adapter, err := registry.Create(VendorCiscoIOS, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if adapter == nil {
			t.Error("adapter should not be nil")
		}
	})
}

func TestVendorRegistryListVendors(t *testing.T) {
	registry := NewVendorRegistry()

	factory := func(config *VendorConfig) (VendorAdapter, error) {
		return nil, nil
	}

	registry.Register(VendorCiscoIOS, factory)
	registry.Register(VendorJuniperJUNOS, factory)

	vendors := registry.ListVendors()
	if len(vendors) != 2 {
		t.Errorf("ListVendors() returned %d vendors, want 2", len(vendors))
	}
}

func TestVendorNotFoundError(t *testing.T) {
	err := &VendorNotFoundError{Vendor: VendorCiscoIOS}
	expected := "vendor not found: cisco_ios"
	if err.Error() != expected {
		t.Errorf("Error() = %v, want %v", err.Error(), expected)
	}
}

func TestDeviceFactsStructure(t *testing.T) {
	facts := &DeviceFacts{
		Hostname:     "router1",
		FQDN:         "router1.example.com",
		Model:        "C9300-24P",
		SerialNumber: "FCW1234ABC",
		Vendor:       "Cisco",
		OSType:       "IOS",
		OSVersion:    "17.3.3",
		Uptime:       24 * time.Hour,
		MemoryTotal:  8 * 1024 * 1024 * 1024,
		MemoryFree:   4 * 1024 * 1024 * 1024,
		CPUUsage:     25.5,
		Interfaces: []InterfaceFact{
			{Name: "GigabitEthernet1/0/1", AdminStatus: "up", OperStatus: "up"},
		},
		Raw: map[string]string{"version": "test output"},
	}

	if facts.Hostname != "router1" {
		t.Errorf("Hostname = %v", facts.Hostname)
	}
	if facts.Model != "C9300-24P" {
		t.Errorf("Model = %v", facts.Model)
	}
	if facts.OSVersion != "17.3.3" {
		t.Errorf("OSVersion = %v", facts.OSVersion)
	}
	if len(facts.Interfaces) != 1 {
		t.Errorf("Interfaces count = %d", len(facts.Interfaces))
	}
}

func TestInterfaceFactStructure(t *testing.T) {
	iface := InterfaceFact{
		Name:        "eth0",
		Description: "Management interface",
		MacAddress:  "00:11:22:33:44:55",
		IPAddresses: []string{"192.168.1.1/24", "10.0.0.1/8"},
		Speed:       1000,
		MTU:         1500,
		AdminStatus: "up",
		OperStatus:  "up",
		Duplex:      "full",
	}

	if iface.Name != "eth0" {
		t.Errorf("Name = %v", iface.Name)
	}
	if iface.Speed != 1000 {
		t.Errorf("Speed = %d", iface.Speed)
	}
	if len(iface.IPAddresses) != 2 {
		t.Errorf("IPAddresses count = %d", len(iface.IPAddresses))
	}
}

func TestInterfaceConfigStructure(t *testing.T) {
	config := &InterfaceConfig{
		Name:        "GigabitEthernet0/1",
		Description: "Uplink",
		IPAddress:   "192.168.1.1/24",
		Enabled:     true,
		Speed:       1000,
		MTU:         9000,
		Duplex:      "full",
	}

	if config.Name != "GigabitEthernet0/1" {
		t.Errorf("Name = %v", config.Name)
	}
	if !config.Enabled {
		t.Error("Enabled should be true")
	}
	if config.MTU != 9000 {
		t.Errorf("MTU = %d", config.MTU)
	}
}

func TestVLANConfigStructure(t *testing.T) {
	vlan := &VLANConfig{
		ID:    100,
		Name:  "Management",
		State: "active",
	}

	if vlan.ID != 100 {
		t.Errorf("ID = %d", vlan.ID)
	}
	if vlan.Name != "Management" {
		t.Errorf("Name = %v", vlan.Name)
	}
}

func TestRouteConfigStructure(t *testing.T) {
	route := &RouteConfig{
		Destination: "10.0.0.0/8",
		NextHop:     "192.168.1.1",
		Interface:   "eth0",
		Metric:      100,
		Description: "Corporate network",
	}

	if route.Destination != "10.0.0.0/8" {
		t.Errorf("Destination = %v", route.Destination)
	}
	if route.NextHop != "192.168.1.1" {
		t.Errorf("NextHop = %v", route.NextHop)
	}
}

func TestACLConfigStructure(t *testing.T) {
	acl := &ACLConfig{
		Name: "BLOCK_TELNET",
		Type: "extended",
		Rules: []ACLRule{
			{
				Sequence:        10,
				Action:          "deny",
				Protocol:        "tcp",
				Source:          "any",
				Destination:     "any",
				DestinationPort: "23",
				Log:             true,
			},
			{
				Sequence:    20,
				Action:      "permit",
				Protocol:    "ip",
				Source:      "any",
				Destination: "any",
			},
		},
	}

	if acl.Name != "BLOCK_TELNET" {
		t.Errorf("Name = %v", acl.Name)
	}
	if len(acl.Rules) != 2 {
		t.Errorf("Rules count = %d", len(acl.Rules))
	}
	if acl.Rules[0].DestinationPort != "23" {
		t.Errorf("Rules[0].DestinationPort = %v", acl.Rules[0].DestinationPort)
	}
}

func TestCommandResultStructure(t *testing.T) {
	result := &CommandResult{
		Command:  "show version",
		Output:   "Cisco IOS...",
		Error:    "",
		ExitCode: 0,
		Duration: 100 * time.Millisecond,
	}

	if result.Command != "show version" {
		t.Errorf("Command = %v", result.Command)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d", result.ExitCode)
	}
	if result.Duration != 100*time.Millisecond {
		t.Errorf("Duration = %v", result.Duration)
	}
}

func TestSystemInfoStructure(t *testing.T) {
	info := &SystemInfo{
		Hostname:     "router1",
		Vendor:       "Cisco",
		Model:        "ISR4331",
		Version:      "17.3.3",
		SerialNumber: "FCW1234ABC",
		Uptime:       "10 days, 5 hours",
	}

	if info.Hostname != "router1" {
		t.Errorf("Hostname = %v", info.Hostname)
	}
	if info.Version != "17.3.3" {
		t.Errorf("Version = %v", info.Version)
	}
}

func TestDeviceConfigStructure(t *testing.T) {
	config := &DeviceConfig{
		Running:   "hostname router1\n...",
		Startup:   "hostname router1\n...",
		Timestamp: time.Now(),
	}

	if config.Running == "" {
		t.Error("Running should not be empty")
	}
	if config.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

// testVendorAdapter is a minimal implementation for testing
type testVendorAdapter struct {
	BaseVendorAdapter
}

func (a *testVendorAdapter) Vendor() VendorType {
	return VendorCiscoIOS
}

func (a *testVendorAdapter) GetConfig(ctx context.Context, section string) (string, error) {
	return "", nil
}

func (a *testVendorAdapter) SetConfig(ctx context.Context, commands []string) error {
	return nil
}

func (a *testVendorAdapter) GetFacts(ctx context.Context) (*DeviceFacts, error) {
	return &DeviceFacts{}, nil
}

func (a *testVendorAdapter) SaveConfig(ctx context.Context) error {
	return nil
}
