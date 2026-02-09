package f5

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

func TestDefaultBigIPConfig(t *testing.T) {
	cfg := DefaultBigIPConfig()

	if cfg.VendorConfig == nil {
		t.Error("VendorConfig should not be nil")
	}
	if cfg.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", cfg.Timeout)
	}
	if cfg.EnablePrompt != "(tmos)#" {
		t.Errorf("EnablePrompt = %v, want '(tmos)#'", cfg.EnablePrompt)
	}
	if !cfg.DisablePaging {
		t.Error("DisablePaging should be true by default")
	}
}

func TestNewBigIPAdapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewBigIPAdapter(nil)
		if adapter == nil {
			t.Fatal("adapter should not be nil")
		}
		if adapter.sshAdapter == nil {
			t.Error("sshAdapter should not be nil")
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &BigIPConfig{
			VendorConfig: &vendors.VendorConfig{
				Timeout: 30 * time.Second,
			},
		}
		adapter := NewBigIPAdapter(cfg)
		if adapter.Config.Timeout != 30*time.Second {
			t.Errorf("Timeout = %v, want 30s", adapter.Config.Timeout)
		}
	})

	t.Run("nil VendorConfig", func(t *testing.T) {
		cfg := &BigIPConfig{}
		adapter := NewBigIPAdapter(cfg)
		if adapter.Config == nil {
			t.Error("Config should be set to default")
		}
	})
}

func TestBigIPAdapterVendor(t *testing.T) {
	adapter := NewBigIPAdapter(nil)
	if adapter.Vendor() != vendors.VendorF5BigIP {
		t.Errorf("Vendor() = %v, want VendorF5BigIP", adapter.Vendor())
	}
}

func TestBigIPAdapterType(t *testing.T) {
	adapter := NewBigIPAdapter(nil)
	if adapter.Type() != protocols.ProtocolSSH {
		t.Errorf("Type() = %v, want ProtocolSSH", adapter.Type())
	}
}

func TestBigIPAdapterIsConnected(t *testing.T) {
	adapter := NewBigIPAdapter(nil)
	if adapter.IsConnected() {
		t.Error("new adapter should not be connected")
	}
}

func TestBigIPAdapterMetrics(t *testing.T) {
	adapter := NewBigIPAdapter(nil)
	metrics := adapter.Metrics()
	if metrics == nil {
		t.Fatal("Metrics() should not return nil")
	}
}

func TestBigIPAdapterDisconnect(t *testing.T) {
	adapter := NewBigIPAdapter(nil)
	err := adapter.Disconnect(context.Background())
	if err != nil {
		t.Errorf("Disconnect() error = %v, want nil", err)
	}
}

func TestBigIPAdapterHealthCheckNotConnected(t *testing.T) {
	adapter := NewBigIPAdapter(nil)
	result, err := adapter.HealthCheck(context.Background())
	if err != nil {
		t.Errorf("HealthCheck() error = %v", err)
	}
	if result.Healthy {
		t.Error("should not be healthy when not connected")
	}
	if result.Status != "not connected" {
		t.Errorf("Status = %v, want 'not connected'", result.Status)
	}
}

func TestBigIPAdapterRunCommandNilShell(t *testing.T) {
	adapter := NewBigIPAdapter(nil)
	_, err := adapter.runCommand(context.Background(), "show sys version")
	if err == nil {
		t.Error("expected error when shell is nil")
	}
	if err.Error() != "shell not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBigIPParseVersion(t *testing.T) {
	adapter := NewBigIPAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `Sys::Version
Main Package
  Product   BIG-IP
  Version   16.1.3.1
  Build     0.0.11
  Edition   Point Release 1
  Date      Fri Aug 12 13:16:53 PDT 2022
`

	adapter.parseVersion(output, facts)

	if facts.OSVersion != "16.1.3.1" {
		t.Errorf("OSVersion = %v, want '16.1.3.1'", facts.OSVersion)
	}
}

func TestBigIPParseHardware(t *testing.T) {
	adapter := NewBigIPAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `Sys::Hardware
Platform BIG-IP-i5800
Name     i5800
Serial   f5-ABCD-12345678
Memory (bytes)
  Total 32768
`

	adapter.parseHardware(output, facts)

	if facts.Model != "BIG-IP-i5800" {
		t.Errorf("Model = %v, want 'BIG-IP-i5800'", facts.Model)
	}
	if facts.SerialNumber != "f5-ABCD-12345678" {
		t.Errorf("SerialNumber = %v, want 'f5-ABCD-12345678'", facts.SerialNumber)
	}
}

func TestBigIPParseGlobalSettings(t *testing.T) {
	adapter := NewBigIPAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `sys global-settings {
    hostname bigip-01.example.com
    gui-setup disabled
}
`

	adapter.parseGlobalSettings(output, facts)

	if facts.Hostname != "bigip-01.example.com" {
		t.Errorf("Hostname = %v, want 'bigip-01.example.com'", facts.Hostname)
	}
}

func TestBigIPParseInterfaces(t *testing.T) {
	adapter := NewBigIPAdapter(nil)

	output := `net interface 1.1 {
    enabled
    mac-address 00:23:e9:f5:01:01
    media-active 10000T-FD
}
net interface 1.2 {
    enabled
    mac-address 00:23:e9:f5:01:02
    media-active none
}
net interface mgmt {
    disabled
    mac-address 00:23:e9:f5:00:00
    media-active 1000T-FD
}
`

	interfaces := adapter.parseInterfaces(output)
	if len(interfaces) != 3 {
		t.Fatalf("expected 3 interfaces, got %d", len(interfaces))
	}

	if interfaces[0].Name != "1.1" {
		t.Errorf("interfaces[0].Name = %v, want '1.1'", interfaces[0].Name)
	}
	if interfaces[0].AdminStatus != "up" {
		t.Errorf("interfaces[0].AdminStatus = %v, want 'up'", interfaces[0].AdminStatus)
	}
	if interfaces[0].OperStatus != "up" {
		t.Errorf("interfaces[0].OperStatus = %v, want 'up'", interfaces[0].OperStatus)
	}
	if interfaces[0].MacAddress != "00:23:e9:f5:01:01" {
		t.Errorf("interfaces[0].MacAddress = %v", interfaces[0].MacAddress)
	}

	// Interface 1.2 has media-active none -> oper down
	if interfaces[1].OperStatus != "down" {
		t.Errorf("interfaces[1].OperStatus = %v, want 'down'", interfaces[1].OperStatus)
	}

	// mgmt interface is disabled
	if interfaces[2].AdminStatus != "down" {
		t.Errorf("interfaces[2].AdminStatus = %v, want 'down'", interfaces[2].AdminStatus)
	}
}

func TestNewBigIPAdapterFactory(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		factory := NewBigIPAdapterFactory(nil)
		adapter, err := factory(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if adapter.Vendor() != vendors.VendorF5BigIP {
			t.Errorf("Vendor() = %v, want VendorF5BigIP", adapter.Vendor())
		}
	})

	t.Run("custom vendor config", func(t *testing.T) {
		factory := NewBigIPAdapterFactory(nil)
		vendorConfig := &vendors.VendorConfig{Timeout: 120 * time.Second}
		adapter, err := factory(vendorConfig)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		bigipAdapter := adapter.(*BigIPAdapter)
		if bigipAdapter.Config.Timeout != 120*time.Second {
			t.Errorf("Timeout = %v, want 120s", bigipAdapter.Config.Timeout)
		}
	})
}

// Verify interface compliance.
var _ vendors.VendorAdapter = (*BigIPAdapter)(nil)
