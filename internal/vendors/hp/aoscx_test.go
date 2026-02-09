package hp

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

func TestDefaultAOSCXConfig(t *testing.T) {
	cfg := DefaultAOSCXConfig()

	if cfg.VendorConfig == nil {
		t.Error("VendorConfig should not be nil")
	}
	if cfg.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", cfg.Timeout)
	}
}

func TestNewAOSCXAdapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewAOSCXAdapter(nil)
		if adapter == nil {
			t.Fatal("adapter should not be nil")
		}
		if adapter.sshAdapter == nil {
			t.Error("sshAdapter should not be nil")
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &AOSCXConfig{
			VendorConfig: &vendors.VendorConfig{
				Timeout: 45 * time.Second,
			},
		}
		adapter := NewAOSCXAdapter(cfg)
		if adapter.Config.Timeout != 45*time.Second {
			t.Errorf("Timeout = %v, want 45s", adapter.Config.Timeout)
		}
	})

	t.Run("nil VendorConfig", func(t *testing.T) {
		cfg := &AOSCXConfig{}
		adapter := NewAOSCXAdapter(cfg)
		if adapter.Config == nil {
			t.Error("Config should be set to default")
		}
	})
}

func TestAOSCXAdapterVendor(t *testing.T) {
	adapter := NewAOSCXAdapter(nil)
	if adapter.Vendor() != vendors.VendorHPAOSCX {
		t.Errorf("Vendor() = %v, want VendorHPAOSCX", adapter.Vendor())
	}
}

func TestAOSCXAdapterType(t *testing.T) {
	adapter := NewAOSCXAdapter(nil)
	if adapter.Type() != protocols.ProtocolSSH {
		t.Errorf("Type() = %v, want ProtocolSSH", adapter.Type())
	}
}

func TestAOSCXAdapterIsConnected(t *testing.T) {
	adapter := NewAOSCXAdapter(nil)
	if adapter.IsConnected() {
		t.Error("new adapter should not be connected")
	}
}

func TestAOSCXAdapterMetrics(t *testing.T) {
	adapter := NewAOSCXAdapter(nil)
	metrics := adapter.Metrics()
	if metrics == nil {
		t.Fatal("Metrics() should not return nil")
	}
}

func TestAOSCXAdapterDisconnect(t *testing.T) {
	adapter := NewAOSCXAdapter(nil)
	err := adapter.Disconnect(context.Background())
	if err != nil {
		t.Errorf("Disconnect() error = %v, want nil", err)
	}
}

func TestAOSCXAdapterHealthCheckNotConnected(t *testing.T) {
	adapter := NewAOSCXAdapter(nil)
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

func TestAOSCXAdapterRunCommandNilShell(t *testing.T) {
	adapter := NewAOSCXAdapter(nil)
	_, err := adapter.runCommand(context.Background(), "show version")
	if err == nil {
		t.Error("expected error when shell is nil")
	}
	if err.Error() != "shell not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAOSCXParseVersionFacts(t *testing.T) {
	adapter := NewAOSCXAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `AOS-CX 10.06.0010
Product Name    : Aruba 6300M
Serial Number   : SG456DEF789
Hostname        : cx-switch-01
`

	adapter.parseVersionFacts(output, facts)

	if facts.OSVersion != "10.06.0010" {
		t.Errorf("OSVersion = %v, want '10.06.0010'", facts.OSVersion)
	}
	if facts.Model != "Aruba 6300M" {
		t.Errorf("Model = %v, want 'Aruba 6300M'", facts.Model)
	}
	if facts.SerialNumber != "SG456DEF789" {
		t.Errorf("SerialNumber = %v, want 'SG456DEF789'", facts.SerialNumber)
	}
	if facts.Hostname != "cx-switch-01" {
		t.Errorf("Hostname = %v, want 'cx-switch-01'", facts.Hostname)
	}
}

func TestAOSCXParseInterfaces(t *testing.T) {
	adapter := NewAOSCXAdapter(nil)

	output := `
Port          Admin  Link
----------    -----  ----
1/1/1         up     up
1/1/2         up     down
1/1/3         down   down
vlan10        up     up
`

	interfaces := adapter.parseInterfaces(output)
	if len(interfaces) != 4 {
		t.Fatalf("expected 4 interfaces, got %d", len(interfaces))
	}

	if interfaces[0].Name != "1/1/1" {
		t.Errorf("interfaces[0].Name = %v, want '1/1/1'", interfaces[0].Name)
	}
	if interfaces[0].AdminStatus != "up" {
		t.Errorf("interfaces[0].AdminStatus = %v, want 'up'", interfaces[0].AdminStatus)
	}
	if interfaces[1].OperStatus != "down" {
		t.Errorf("interfaces[1].OperStatus = %v, want 'down'", interfaces[1].OperStatus)
	}
}

func TestNewAOSCXAdapterFactory(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		factory := NewAOSCXAdapterFactory(nil)
		adapter, err := factory(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if adapter.Vendor() != vendors.VendorHPAOSCX {
			t.Errorf("Vendor() = %v, want VendorHPAOSCX", adapter.Vendor())
		}
	})

	t.Run("custom vendor config", func(t *testing.T) {
		factory := NewAOSCXAdapterFactory(nil)
		vendorConfig := &vendors.VendorConfig{Timeout: 90 * time.Second}
		adapter, err := factory(vendorConfig)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		cxAdapter := adapter.(*AOSCXAdapter)
		if cxAdapter.Config.Timeout != 90*time.Second {
			t.Errorf("Timeout = %v, want 90s", cxAdapter.Config.Timeout)
		}
	})
}

// Verify interface compliance.
var _ vendors.VendorAdapter = (*AOSCXAdapter)(nil)
