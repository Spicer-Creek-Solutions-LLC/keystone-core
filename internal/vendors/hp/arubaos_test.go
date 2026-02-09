package hp

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

func TestDefaultArubaOSConfig(t *testing.T) {
	cfg := DefaultArubaOSConfig()

	if cfg.VendorConfig == nil {
		t.Error("VendorConfig should not be nil")
	}
	if cfg.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", cfg.Timeout)
	}
}

func TestNewArubaOSAdapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewArubaOSAdapter(nil)
		if adapter == nil {
			t.Fatal("adapter should not be nil")
		}
		if adapter.sshAdapter == nil {
			t.Error("sshAdapter should not be nil")
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &ArubaOSConfig{
			VendorConfig: &vendors.VendorConfig{
				Timeout: 30 * time.Second,
			},
		}
		adapter := NewArubaOSAdapter(cfg)
		if adapter.Config.Timeout != 30*time.Second {
			t.Errorf("Timeout = %v, want 30s", adapter.Config.Timeout)
		}
	})

	t.Run("nil VendorConfig", func(t *testing.T) {
		cfg := &ArubaOSConfig{}
		adapter := NewArubaOSAdapter(cfg)
		if adapter.Config == nil {
			t.Error("Config should be set to default")
		}
	})
}

func TestArubaOSAdapterVendor(t *testing.T) {
	adapter := NewArubaOSAdapter(nil)
	if adapter.Vendor() != vendors.VendorHPArubaOS {
		t.Errorf("Vendor() = %v, want VendorHPArubaOS", adapter.Vendor())
	}
}

func TestArubaOSAdapterType(t *testing.T) {
	adapter := NewArubaOSAdapter(nil)
	if adapter.Type() != protocols.ProtocolSSH {
		t.Errorf("Type() = %v, want ProtocolSSH", adapter.Type())
	}
}

func TestArubaOSAdapterIsConnected(t *testing.T) {
	adapter := NewArubaOSAdapter(nil)
	if adapter.IsConnected() {
		t.Error("new adapter should not be connected")
	}
}

func TestArubaOSAdapterMetrics(t *testing.T) {
	adapter := NewArubaOSAdapter(nil)
	metrics := adapter.Metrics()
	if metrics == nil {
		t.Fatal("Metrics() should not return nil")
	}
}

func TestArubaOSAdapterDisconnect(t *testing.T) {
	adapter := NewArubaOSAdapter(nil)
	err := adapter.Disconnect(context.Background())
	if err != nil {
		t.Errorf("Disconnect() error = %v, want nil", err)
	}
}

func TestArubaOSAdapterHealthCheckNotConnected(t *testing.T) {
	adapter := NewArubaOSAdapter(nil)
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

func TestArubaOSAdapterRunCommandNilShell(t *testing.T) {
	adapter := NewArubaOSAdapter(nil)
	_, err := adapter.runCommand(context.Background(), "show version")
	if err == nil {
		t.Error("expected error when shell is nil")
	}
	if err.Error() != "shell not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestArubaOSParseVersionFacts(t *testing.T) {
	adapter := NewArubaOSAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `ArubaOS (MODEL: Aruba7210), Version 8.6.0.4
Website: http://www.arubanetworks.com
Serial Number      : CN12345678
System uptime      : 30d 5h 45m
Hostname           : aruba-controller-01
`

	adapter.parseVersionFacts(output, facts)

	if facts.Model != "Aruba7210" {
		t.Errorf("Model = %v, want 'Aruba7210'", facts.Model)
	}
	if facts.OSVersion != "8.6.0.4" {
		t.Errorf("OSVersion = %v, want '8.6.0.4'", facts.OSVersion)
	}
	if facts.SerialNumber != "CN12345678" {
		t.Errorf("SerialNumber = %v, want 'CN12345678'", facts.SerialNumber)
	}
	if facts.Hostname != "aruba-controller-01" {
		t.Errorf("Hostname = %v, want 'aruba-controller-01'", facts.Hostname)
	}
	if facts.Uptime <= 0 {
		t.Error("Uptime should be parsed")
	}
}

func TestArubaOSParseAPDatabase(t *testing.T) {
	adapter := NewArubaOSAdapter(nil)

	output := `
Name          Group          Model     IP            Status
----          -----          -----     --            ------
AP-101        default        AP-315    10.0.1.101    Up
AP-102        floor2         AP-325    10.0.1.102    Down
AP-103        default        AP-535    10.0.1.103    Up
`

	aps := adapter.parseAPDatabase(output)
	if len(aps) != 3 {
		t.Fatalf("expected 3 APs, got %d", len(aps))
	}
	if aps[0].Name != "AP-101" {
		t.Errorf("aps[0].Name = %v, want 'AP-101'", aps[0].Name)
	}
	if aps[0].Status != "Up" {
		t.Errorf("aps[0].Status = %v, want 'Up'", aps[0].Status)
	}
	if aps[1].Group != "floor2" {
		t.Errorf("aps[1].Group = %v, want 'floor2'", aps[1].Group)
	}
}

func TestNewArubaOSAdapterFactory(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		factory := NewArubaOSAdapterFactory(nil)
		adapter, err := factory(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if adapter.Vendor() != vendors.VendorHPArubaOS {
			t.Errorf("Vendor() = %v, want VendorHPArubaOS", adapter.Vendor())
		}
	})

	t.Run("custom vendor config", func(t *testing.T) {
		factory := NewArubaOSAdapterFactory(nil)
		vendorConfig := &vendors.VendorConfig{Timeout: 120 * time.Second}
		adapter, err := factory(vendorConfig)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		arubaAdapter := adapter.(*ArubaOSAdapter)
		if arubaAdapter.Config.Timeout != 120*time.Second {
			t.Errorf("Timeout = %v, want 120s", arubaAdapter.Config.Timeout)
		}
	})
}

// Verify interface compliance.
var _ vendors.VendorAdapter = (*ArubaOSAdapter)(nil)
