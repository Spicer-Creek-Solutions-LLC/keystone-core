package cisco

import (
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/protocols"
	"github.com/shawnbutts/keystone-core/pkg/vendors"
)

func TestDefaultNXOSConfig(t *testing.T) {
	cfg := DefaultNXOSConfig()

	if cfg.VendorConfig == nil {
		t.Error("VendorConfig should not be nil")
	}
	if !cfg.UseJSON {
		t.Error("UseJSON should be true by default")
	}
}

func TestNewNXOSAdapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewNXOSAdapter(nil)
		if adapter == nil {
			t.Fatal("adapter should not be nil")
		}
		if adapter.sshAdapter == nil {
			t.Error("sshAdapter should not be nil")
		}
		if !adapter.useJSON {
			t.Error("useJSON should be true by default")
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &NXOSConfig{
			VendorConfig: &vendors.VendorConfig{
				Timeout: 30 * time.Second,
			},
			UseJSON: false,
		}
		adapter := NewNXOSAdapter(cfg)
		if adapter.Config.Timeout != 30*time.Second {
			t.Errorf("Timeout = %v, want 30s", adapter.Config.Timeout)
		}
		if adapter.useJSON {
			t.Error("useJSON should be false")
		}
	})

	t.Run("nil VendorConfig", func(t *testing.T) {
		cfg := &NXOSConfig{}
		adapter := NewNXOSAdapter(cfg)
		if adapter.Config == nil {
			t.Error("Config should be set to default")
		}
	})
}

func TestNXOSAdapterVendor(t *testing.T) {
	adapter := NewNXOSAdapter(nil)
	if adapter.Vendor() != vendors.VendorCiscoNXOS {
		t.Errorf("Vendor() = %v, want VendorCiscoNXOS", adapter.Vendor())
	}
}

func TestNXOSAdapterType(t *testing.T) {
	adapter := NewNXOSAdapter(nil)
	if adapter.Type() != protocols.ProtocolSSH {
		t.Errorf("Type() = %v, want ProtocolSSH", adapter.Type())
	}
}

func TestNXOSAdapterIsConnected(t *testing.T) {
	adapter := NewNXOSAdapter(nil)

	// New adapter should not be connected
	if adapter.IsConnected() {
		t.Error("new adapter should not be connected")
	}
}

func TestNXOSAdapterMetrics(t *testing.T) {
	adapter := NewNXOSAdapter(nil)
	metrics := adapter.Metrics()
	if metrics == nil {
		t.Fatal("Metrics() should not return nil")
	}
}

func TestNXOSParseVersionFacts(t *testing.T) {
	adapter := NewNXOSAdapter(nil)
	facts := &vendors.DeviceFacts{}

	versionOutput := `Cisco Nexus Operating System (NX-OS) Software
TAC support: http://www.cisco.com/tac
Copyright (c) 2002-2021 by Cisco Systems, Inc.
Documents: http://www.cisco.com/en/US/products/ps9372/tsd_products_support_series_home.html
Software
  BIOS: version 2.0.0
  NXOS: version 9.3(8)
  BIOS compile time:  05/29/2018
  NXOS image file is: bootflash:///nxos.9.3.8.bin
  NXOS compile time:  6/17/2021 12:00:00 [06/17/2021 16:27:47]

Hardware
  cisco Nexus9000 C9332PQ Chassis
  Intel(R) Xeon(R) CPU @ 2.00GHz with 16401472 kB of memory.
  Processor Board ID FDO12345678

  Device name: NXOS-SWITCH
  bootflash:   53298520 kB
Kernel uptime is 100 day(s), 5 hour(s), 30 minute(s), 45 second(s)`

	adapter.parseVersionFacts(versionOutput, facts)

	if facts.Model != "Nexus9000" {
		t.Errorf("Model = %v", facts.Model)
	}
	if facts.SerialNumber != "FDO12345678" {
		t.Errorf("SerialNumber = %v", facts.SerialNumber)
	}
	if facts.Uptime <= 0 {
		t.Error("Uptime should be parsed")
	}
}

func TestNXOSParseUptime(t *testing.T) {
	adapter := NewNXOSAdapter(nil)

	tests := []struct {
		name   string
		input  string
		minVal time.Duration
	}{
		{
			name:   "full format",
			input:  "Kernel uptime is 100 day(s), 5 hour(s), 30 minute(s), 45 second(s)",
			minVal: 100*24*time.Hour + 5*time.Hour + 30*time.Minute + 45*time.Second,
		},
		{
			name:   "days and hours",
			input:  "uptime is 10 days, 5 hours",
			minVal: 10*24*time.Hour + 5*time.Hour,
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

func TestNXOSParseInterfaces(t *testing.T) {
	adapter := NewNXOSAdapter(nil)

	interfaceOutput := `IP Interface Status for VRF "default"(1)
Interface            IP Address      Interface Status
Vlan1                192.168.1.1     protocol-up/link-up/admin-up
Vlan100              10.0.0.1        protocol-up/link-up/admin-up
mgmt0                192.168.100.1   protocol-up/link-up/admin-up
Ethernet1/1          --              protocol-down/link-down/admin-down `

	interfaces := adapter.parseInterfaces(interfaceOutput)

	if len(interfaces) < 3 {
		t.Fatalf("expected at least 3 interfaces, got %d", len(interfaces))
	}
}

func TestNewNXOSAdapterFactory(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		factory := NewNXOSAdapterFactory(nil)
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
		if adapter.Vendor() != vendors.VendorCiscoNXOS {
			t.Errorf("Vendor() = %v, want VendorCiscoNXOS", adapter.Vendor())
		}
	})

	t.Run("custom vendor config", func(t *testing.T) {
		factory := NewNXOSAdapterFactory(nil)

		vendorConfig := &vendors.VendorConfig{
			Timeout: 120 * time.Second,
		}
		adapter, err := factory(vendorConfig)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		nxosAdapter, ok := adapter.(*NXOSAdapter)
		if !ok {
			t.Fatal("expected *NXOSAdapter")
		}
		if nxosAdapter.Config.Timeout != 120*time.Second {
			t.Errorf("Timeout = %v, want 120s", nxosAdapter.Config.Timeout)
		}
	})
}

func TestNXOSAdapterDisconnect(t *testing.T) {
	adapter := NewNXOSAdapter(nil)

	// Disconnect on unconnected adapter should succeed
	err := adapter.Disconnect(nil)
	if err != nil {
		t.Errorf("Disconnect() error = %v, want nil", err)
	}
}

func TestNXOSAdapterHealthCheckNotConnected(t *testing.T) {
	adapter := NewNXOSAdapter(nil)

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

func TestNXOSConfigStructure(t *testing.T) {
	cfg := &NXOSConfig{
		VendorConfig: &vendors.VendorConfig{
			Timeout:        90 * time.Second,
			EnablePassword: "enable123",
		},
		UseJSON: true,
	}

	if !cfg.UseJSON {
		t.Error("UseJSON should be true")
	}
	if cfg.VendorConfig.Timeout != 90*time.Second {
		t.Errorf("Timeout = %v", cfg.VendorConfig.Timeout)
	}
}

func TestNXOSAdapterRunCommandNilShell(t *testing.T) {
	adapter := NewNXOSAdapter(nil)

	_, err := adapter.runCommand(nil, "show version")
	if err == nil {
		t.Error("expected error when shell is nil")
	}
	if err.Error() != "shell not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNXOSExitConfigNotInConfig(t *testing.T) {
	adapter := NewNXOSAdapter(nil)
	adapter.inConfig = false

	// Should succeed when not in config mode
	err := adapter.exitConfig(nil)
	if err != nil {
		t.Errorf("exitConfig() error = %v", err)
	}
}
