package dell

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

func TestDefaultPowerSwitchConfig(t *testing.T) {
	cfg := DefaultPowerSwitchConfig()

	if cfg.VendorConfig == nil {
		t.Error("VendorConfig should not be nil")
	}
	if cfg.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", cfg.Timeout)
	}
}

func TestNewPowerSwitchAdapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewPowerSwitchAdapter(nil)
		if adapter == nil {
			t.Fatal("adapter should not be nil")
		}
		if adapter.sshAdapter == nil {
			t.Error("sshAdapter should not be nil")
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &PowerSwitchConfig{
			VendorConfig: &vendors.VendorConfig{
				Timeout: 45 * time.Second,
			},
		}
		adapter := NewPowerSwitchAdapter(cfg)
		if adapter.Config.Timeout != 45*time.Second {
			t.Errorf("Timeout = %v, want 45s", adapter.Config.Timeout)
		}
	})

	t.Run("nil VendorConfig", func(t *testing.T) {
		cfg := &PowerSwitchConfig{}
		adapter := NewPowerSwitchAdapter(cfg)
		if adapter.Config == nil {
			t.Error("Config should be set to default")
		}
	})
}

func TestPowerSwitchAdapterVendor(t *testing.T) {
	adapter := NewPowerSwitchAdapter(nil)
	if adapter.Vendor() != vendors.VendorDellPowerSwitch {
		t.Errorf("Vendor() = %v, want VendorDellPowerSwitch", adapter.Vendor())
	}
}

func TestPowerSwitchAdapterType(t *testing.T) {
	adapter := NewPowerSwitchAdapter(nil)
	if adapter.Type() != protocols.ProtocolSSH {
		t.Errorf("Type() = %v, want ProtocolSSH", adapter.Type())
	}
}

func TestPowerSwitchAdapterIsConnected(t *testing.T) {
	adapter := NewPowerSwitchAdapter(nil)
	if adapter.IsConnected() {
		t.Error("new adapter should not be connected")
	}
}

func TestPowerSwitchAdapterMetrics(t *testing.T) {
	adapter := NewPowerSwitchAdapter(nil)
	metrics := adapter.Metrics()
	if metrics == nil {
		t.Fatal("Metrics() should not return nil")
	}
}

func TestPowerSwitchAdapterDisconnect(t *testing.T) {
	adapter := NewPowerSwitchAdapter(nil)
	err := adapter.Disconnect(context.Background())
	if err != nil {
		t.Errorf("Disconnect() error = %v, want nil", err)
	}
}

func TestPowerSwitchAdapterHealthCheckNotConnected(t *testing.T) {
	adapter := NewPowerSwitchAdapter(nil)
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

func TestPowerSwitchAdapterRunCommandNilShell(t *testing.T) {
	adapter := NewPowerSwitchAdapter(nil)
	_, err := adapter.runCommand(context.Background(), "show version")
	if err == nil {
		t.Error("expected error when shell is nil")
	}
	if err.Error() != "shell not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPowerSwitchParseVersionFacts(t *testing.T) {
	adapter := NewPowerSwitchAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `
SW Version   : 6.5.1.7
System Model : N3048P
Serial Number : DL123456789
System uptime is 15 days, 3 hours, 45 minutes
`

	adapter.parseVersionFacts(output, facts)

	if facts.OSVersion != "6.5.1.7" {
		t.Errorf("OSVersion = %v, want '6.5.1.7'", facts.OSVersion)
	}
	if facts.Model != "N3048P" {
		t.Errorf("Model = %v, want 'N3048P'", facts.Model)
	}
	if facts.SerialNumber != "DL123456789" {
		t.Errorf("SerialNumber = %v, want 'DL123456789'", facts.SerialNumber)
	}
	if facts.Uptime <= 0 {
		t.Error("Uptime should be parsed")
	}
}

func TestPowerSwitchParseInterfaces(t *testing.T) {
	adapter := NewPowerSwitchAdapter(nil)

	output := `
Port      Description  Admin  Link
----      -----------  -----  ----
Gi1/0/1   Server1      up     up
Gi1/0/2   Server2      up     down
Gi1/0/3   -            down   down
Te1/0/1   Uplink       up     up
`

	interfaces := adapter.parseInterfaces(output)
	if len(interfaces) != 4 {
		t.Fatalf("expected 4 interfaces, got %d", len(interfaces))
	}

	if interfaces[0].Name != "Gi1/0/1" {
		t.Errorf("interfaces[0].Name = %v, want 'Gi1/0/1'", interfaces[0].Name)
	}
}

func TestNewPowerSwitchAdapterFactory(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		factory := NewPowerSwitchAdapterFactory(nil)
		adapter, err := factory(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if adapter.Vendor() != vendors.VendorDellPowerSwitch {
			t.Errorf("Vendor() = %v, want VendorDellPowerSwitch", adapter.Vendor())
		}
	})

	t.Run("custom vendor config", func(t *testing.T) {
		factory := NewPowerSwitchAdapterFactory(nil)
		vendorConfig := &vendors.VendorConfig{Timeout: 90 * time.Second}
		adapter, err := factory(vendorConfig)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		psAdapter := adapter.(*PowerSwitchAdapter)
		if psAdapter.Config.Timeout != 90*time.Second {
			t.Errorf("Timeout = %v, want 90s", psAdapter.Config.Timeout)
		}
	})
}

// Verify interface compliance.
var _ vendors.VendorAdapter = (*PowerSwitchAdapter)(nil)
