package checkpoint

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

func TestDefaultGaiaConfig(t *testing.T) {
	cfg := DefaultGaiaConfig()

	if cfg.VendorConfig == nil {
		t.Error("VendorConfig should not be nil")
	}
	if cfg.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", cfg.Timeout)
	}
	if cfg.EnablePrompt != ">" {
		t.Errorf("EnablePrompt = %v, want '>'", cfg.EnablePrompt)
	}
	if !cfg.DisablePaging {
		t.Error("DisablePaging should be true by default")
	}
}

func TestNewGaiaAdapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewGaiaAdapter(nil)
		if adapter == nil {
			t.Fatal("adapter should not be nil")
		}
		if adapter.sshAdapter == nil {
			t.Error("sshAdapter should not be nil")
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &GaiaConfig{
			VendorConfig: &vendors.VendorConfig{
				Timeout: 30 * time.Second,
			},
		}
		adapter := NewGaiaAdapter(cfg)
		if adapter.Config.Timeout != 30*time.Second {
			t.Errorf("Timeout = %v, want 30s", adapter.Config.Timeout)
		}
	})

	t.Run("nil VendorConfig", func(t *testing.T) {
		cfg := &GaiaConfig{}
		adapter := NewGaiaAdapter(cfg)
		if adapter.Config == nil {
			t.Error("Config should be set to default")
		}
	})
}

func TestGaiaAdapterVendor(t *testing.T) {
	adapter := NewGaiaAdapter(nil)
	if adapter.Vendor() != vendors.VendorCheckpointGaia {
		t.Errorf("Vendor() = %v, want VendorCheckpointGaia", adapter.Vendor())
	}
}

func TestGaiaAdapterType(t *testing.T) {
	adapter := NewGaiaAdapter(nil)
	if adapter.Type() != protocols.ProtocolSSH {
		t.Errorf("Type() = %v, want ProtocolSSH", adapter.Type())
	}
}

func TestGaiaAdapterIsConnected(t *testing.T) {
	adapter := NewGaiaAdapter(nil)
	if adapter.IsConnected() {
		t.Error("new adapter should not be connected")
	}
}

func TestGaiaAdapterMetrics(t *testing.T) {
	adapter := NewGaiaAdapter(nil)
	metrics := adapter.Metrics()
	if metrics == nil {
		t.Fatal("Metrics() should not return nil")
	}
}

func TestGaiaAdapterDisconnect(t *testing.T) {
	adapter := NewGaiaAdapter(nil)
	err := adapter.Disconnect(context.Background())
	if err != nil {
		t.Errorf("Disconnect() error = %v, want nil", err)
	}
}

func TestGaiaAdapterHealthCheckNotConnected(t *testing.T) {
	adapter := NewGaiaAdapter(nil)
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

func TestGaiaAdapterRunCommandNilShell(t *testing.T) {
	adapter := NewGaiaAdapter(nil)
	_, err := adapter.runCommand(context.Background(), "show hostname")
	if err == nil {
		t.Error("expected error when shell is nil")
	}
	if err.Error() != "shell not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGaiaParseVersion(t *testing.T) {
	adapter := NewGaiaAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `Product version Check Point Gaia R81.20
OS build 335
OS kernel version 3.10.0-957.21.3cpx86_64
`

	adapter.parseVersion(output, facts)

	if facts.OSVersion != "R81.20" {
		t.Errorf("OSVersion = %v, want 'R81.20'", facts.OSVersion)
	}
}

func TestGaiaParseAsset(t *testing.T) {
	adapter := NewGaiaAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `Platform: Check Point 5800
Serial Number: CP1234567890
BIOS version: 2.0
`

	adapter.parseAsset(output, facts)

	if facts.Model != "Check Point 5800" {
		t.Errorf("Model = %v, want 'Check Point 5800'", facts.Model)
	}
	if facts.SerialNumber != "CP1234567890" {
		t.Errorf("SerialNumber = %v, want 'CP1234567890'", facts.SerialNumber)
	}
}

func TestGaiaParseInterfaces(t *testing.T) {
	adapter := NewGaiaAdapter(nil)

	output := `Interface  State  Link
-------  -----  ----
eth0     up     up     00:1c:7f:aa:bb:cc
eth1     up     down   00:1c:7f:aa:bb:dd
eth2     down   down   00:1c:7f:aa:bb:ee
`

	interfaces := adapter.parseInterfaces(output)
	if len(interfaces) != 3 {
		t.Fatalf("expected 3 interfaces, got %d", len(interfaces))
	}

	if interfaces[0].Name != "eth0" {
		t.Errorf("interfaces[0].Name = %v, want 'eth0'", interfaces[0].Name)
	}
	if interfaces[0].AdminStatus != "up" {
		t.Errorf("interfaces[0].AdminStatus = %v, want 'up'", interfaces[0].AdminStatus)
	}
	if interfaces[0].OperStatus != "up" {
		t.Errorf("interfaces[0].OperStatus = %v, want 'up'", interfaces[0].OperStatus)
	}
}

func TestNewGaiaAdapterFactory(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		factory := NewGaiaAdapterFactory(nil)
		adapter, err := factory(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if adapter.Vendor() != vendors.VendorCheckpointGaia {
			t.Errorf("Vendor() = %v, want VendorCheckpointGaia", adapter.Vendor())
		}
	})

	t.Run("custom vendor config", func(t *testing.T) {
		factory := NewGaiaAdapterFactory(nil)
		vendorConfig := &vendors.VendorConfig{Timeout: 120 * time.Second}
		adapter, err := factory(vendorConfig)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		gaiaAdapter := adapter.(*GaiaAdapter)
		if gaiaAdapter.Config.Timeout != 120*time.Second {
			t.Errorf("Timeout = %v, want 120s", gaiaAdapter.Config.Timeout)
		}
	})
}

var _ vendors.VendorAdapter = (*GaiaAdapter)(nil)
