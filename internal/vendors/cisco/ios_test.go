package cisco

import (
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

func TestDefaultIOSConfig(t *testing.T) {
	cfg := DefaultIOSConfig()

	if cfg.VendorConfig == nil {
		t.Error("VendorConfig should not be nil")
	}
	if cfg.VendorConfig.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", cfg.VendorConfig.Timeout)
	}
}

func TestNewIOSAdapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewIOSAdapter(nil)
		if adapter == nil {
			t.Fatal("adapter should not be nil")
		}
		if adapter.sshAdapter == nil {
			t.Error("sshAdapter should not be nil")
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &IOSConfig{
			VendorConfig: &vendors.VendorConfig{
				Timeout:        30 * time.Second,
				EnablePassword: "secret123",
			},
			Secret: "enable_secret",
		}
		adapter := NewIOSAdapter(cfg)
		if adapter.Config.Timeout != 30*time.Second {
			t.Errorf("Timeout = %v, want 30s", adapter.Config.Timeout)
		}
		if adapter.Config.EnablePassword != "secret123" {
			t.Errorf("EnablePassword = %v", adapter.Config.EnablePassword)
		}
	})

	t.Run("nil VendorConfig", func(t *testing.T) {
		cfg := &IOSConfig{}
		adapter := NewIOSAdapter(cfg)
		if adapter.Config == nil {
			t.Error("Config should be set to default")
		}
	})
}

func TestIOSAdapterVendor(t *testing.T) {
	adapter := NewIOSAdapter(nil)
	if adapter.Vendor() != vendors.VendorCiscoIOS {
		t.Errorf("Vendor() = %v, want VendorCiscoIOS", adapter.Vendor())
	}
}

func TestIOSAdapterType(t *testing.T) {
	adapter := NewIOSAdapter(nil)
	if adapter.Type() != protocols.ProtocolSSH {
		t.Errorf("Type() = %v, want ProtocolSSH", adapter.Type())
	}
}

func TestIOSAdapterIsConnected(t *testing.T) {
	adapter := NewIOSAdapter(nil)

	// New adapter should not be connected
	if adapter.IsConnected() {
		t.Error("new adapter should not be connected")
	}
}

func TestIOSAdapterMetrics(t *testing.T) {
	adapter := NewIOSAdapter(nil)
	metrics := adapter.Metrics()
	if metrics == nil {
		t.Fatal("Metrics() should not return nil")
	}
}

func TestParseVersionFacts(t *testing.T) {
	adapter := NewIOSAdapter(nil)
	facts := &vendors.DeviceFacts{}

	versionOutput := `Cisco IOS Software, C2960 Software (C2960-LANBASEK9-M), Version 15.2(2)E9, RELEASE SOFTWARE
Technical Support: http://www.cisco.com/techsupport
Copyright (c) 1986-2018 by Cisco Systems, Inc.
Compiled Mon 16-Jul-18 14:33 by prod_rel_team

ROM: Bootstrap program is C2960 boot loader
BOOTLDR: C2960 Boot Loader (C2960-HBOOT-M) Version 12.2(53r)SEY4

Switch uptime is 1 year, 2 weeks, 3 days, 4 hours, 5 minutes
System returned to ROM by power-on

cisco WS-C2960-24TT-L (PowerPC405) processor (revision K0) with 65536K bytes of memory.
Processor board ID FOC1234ABCD
Last reset from power-on
1 Virtual Ethernet interface
24 FastEthernet interfaces
2 Gigabit Ethernet interfaces`

	adapter.parseVersionFacts(versionOutput, facts)

	if facts.OSVersion != "15.2(2)E9," {
		t.Errorf("OSVersion = %v", facts.OSVersion)
	}
	if facts.Model != "WS-C2960-24TT-L" {
		t.Errorf("Model = %v", facts.Model)
	}
	if facts.SerialNumber != "FOC1234ABCD" {
		t.Errorf("SerialNumber = %v", facts.SerialNumber)
	}
	if facts.Uptime <= 0 {
		t.Error("Uptime should be parsed")
	}
}

func TestParseUptime(t *testing.T) {
	adapter := NewIOSAdapter(nil)

	tests := []struct {
		name   string
		input  string
		minVal time.Duration
	}{
		{
			name:   "full format",
			input:  "uptime is 1 year, 2 weeks, 3 days, 4 hours, 5 minutes",
			minVal: 365*24*time.Hour + 14*24*time.Hour + 3*24*time.Hour + 4*time.Hour + 5*time.Minute,
		},
		{
			name:   "days only",
			input:  "uptime is 10 days, 5 hours",
			minVal: 10*24*time.Hour + 5*time.Hour,
		},
		{
			name:   "hours only",
			input:  "uptime is 5 hours, 30 minutes",
			minVal: 5*time.Hour + 30*time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uptime := adapter.parseUptime(tt.input)
			if uptime < tt.minVal {
				t.Errorf("parseUptime(%q) = %v, want >= %v", tt.input, uptime, tt.minVal)
			}
		})
	}
}

func TestParseInterfaces(t *testing.T) {
	adapter := NewIOSAdapter(nil)

	interfaceOutput := `Interface              IP-Address      OK? Method Status                Protocol
FastEthernet0/1        192.168.1.1     YES NVRAM  up                    up
FastEthernet0/2        unassigned      YES unset  administratively down down
GigabitEthernet0/1     10.0.0.1        YES NVRAM  up                    up
Vlan1                  unassigned      YES unset  up                    down    `

	interfaces := adapter.parseInterfaces(interfaceOutput)

	if len(interfaces) != 4 {
		t.Fatalf("expected 4 interfaces, got %d", len(interfaces))
	}

	// Check first interface
	if interfaces[0].Name != "FastEthernet0/1" {
		t.Errorf("interfaces[0].Name = %v", interfaces[0].Name)
	}
	if len(interfaces[0].IPAddresses) != 1 || interfaces[0].IPAddresses[0] != "192.168.1.1" {
		t.Errorf("interfaces[0].IPAddresses = %v", interfaces[0].IPAddresses)
	}
	if interfaces[0].AdminStatus != "up" {
		t.Errorf("interfaces[0].AdminStatus = %v", interfaces[0].AdminStatus)
	}
	if interfaces[0].OperStatus != "up" {
		t.Errorf("interfaces[0].OperStatus = %v", interfaces[0].OperStatus)
	}

	// Check unassigned interface
	if len(interfaces[1].IPAddresses) != 0 {
		t.Errorf("interfaces[1] should not have IP addresses")
	}
}

func TestCidrToMask(t *testing.T) {
	tests := []struct {
		cidr string
		want string
	}{
		{"24", "255.255.255.0"},
		{"16", "255.255.0.0"},
		{"8", "255.0.0.0"},
		{"32", "255.255.255.255"},
		{"30", "255.255.255.252"},
		{"invalid", "255.255.255.0"},
	}

	for _, tt := range tests {
		t.Run(tt.cidr, func(t *testing.T) {
			got := cidrToMask(tt.cidr)
			if got != tt.want {
				t.Errorf("cidrToMask(%q) = %v, want %v", tt.cidr, got, tt.want)
			}
		})
	}
}

func TestNewIOSAdapterFactory(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		factory := NewIOSAdapterFactory(nil)
		if factory == nil {
			t.Fatal("factory should not be nil")
		}

		adapter, err := factory(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if adapter == nil {
			t.Error("adapter should not be nil")
		}
		if adapter.Vendor() != vendors.VendorCiscoIOS {
			t.Errorf("Vendor() = %v, want VendorCiscoIOS", adapter.Vendor())
		}
	})

	t.Run("custom vendor config", func(t *testing.T) {
		factory := NewIOSAdapterFactory(nil)

		vendorConfig := &vendors.VendorConfig{
			Timeout: 120 * time.Second,
		}
		adapter, err := factory(vendorConfig)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		iosAdapter, ok := adapter.(*IOSAdapter)
		if !ok {
			t.Fatal("expected *IOSAdapter")
		}
		if iosAdapter.Config.Timeout != 120*time.Second {
			t.Errorf("Timeout = %v, want 120s", iosAdapter.Config.Timeout)
		}
	})
}

func TestIOSAdapterDisconnect(t *testing.T) {
	adapter := NewIOSAdapter(nil)

	// Disconnect on unconnected adapter should succeed
	err := adapter.Disconnect(nil)
	if err != nil {
		t.Errorf("Disconnect() error = %v, want nil", err)
	}
}

func TestIOSAdapterHealthCheckNotConnected(t *testing.T) {
	adapter := NewIOSAdapter(nil)

	result, err := adapter.HealthCheck(nil)
	if err != nil {
		t.Errorf("HealthCheck() error = %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Healthy {
		t.Error("should not be healthy when not connected")
	}
	if result.Status != "not connected" {
		t.Errorf("Status = %v, want 'not connected'", result.Status)
	}
}

func TestIOSConfigStructure(t *testing.T) {
	cfg := &IOSConfig{
		VendorConfig: &vendors.VendorConfig{
			Timeout:        90 * time.Second,
			EnablePassword: "enable123",
			PrivilegeLevel: 15,
		},
		Secret: "secret456",
	}

	if cfg.Secret != "secret456" {
		t.Errorf("Secret = %v", cfg.Secret)
	}
	if cfg.VendorConfig.EnablePassword != "enable123" {
		t.Errorf("EnablePassword = %v", cfg.VendorConfig.EnablePassword)
	}
}

func TestIOSAdapterRunCommandNilShell(t *testing.T) {
	adapter := NewIOSAdapter(nil)

	_, err := adapter.runCommand(nil, "show version")
	if err == nil {
		t.Error("expected error when shell is nil")
	}
	if err.Error() != "shell not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}
