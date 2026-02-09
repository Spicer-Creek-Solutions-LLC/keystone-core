package dell

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

func TestDefaultOS9Config(t *testing.T) {
	cfg := DefaultOS9Config()

	if cfg.VendorConfig == nil {
		t.Error("VendorConfig should not be nil")
	}
	if cfg.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", cfg.Timeout)
	}
}

func TestNewOS9Adapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewOS9Adapter(nil)
		if adapter == nil {
			t.Fatal("adapter should not be nil")
		}
		if adapter.sshAdapter == nil {
			t.Error("sshAdapter should not be nil")
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &OS9Config{
			VendorConfig: &vendors.VendorConfig{
				Timeout: 45 * time.Second,
			},
		}
		adapter := NewOS9Adapter(cfg)
		if adapter.Config.Timeout != 45*time.Second {
			t.Errorf("Timeout = %v, want 45s", adapter.Config.Timeout)
		}
	})

	t.Run("nil VendorConfig", func(t *testing.T) {
		cfg := &OS9Config{}
		adapter := NewOS9Adapter(cfg)
		if adapter.Config == nil {
			t.Error("Config should be set to default")
		}
	})
}

func TestOS9AdapterVendor(t *testing.T) {
	adapter := NewOS9Adapter(nil)
	if adapter.Vendor() != vendors.VendorDellOS9 {
		t.Errorf("Vendor() = %v, want VendorDellOS9", adapter.Vendor())
	}
}

func TestOS9AdapterType(t *testing.T) {
	adapter := NewOS9Adapter(nil)
	if adapter.Type() != protocols.ProtocolSSH {
		t.Errorf("Type() = %v, want ProtocolSSH", adapter.Type())
	}
}

func TestOS9AdapterIsConnected(t *testing.T) {
	adapter := NewOS9Adapter(nil)
	if adapter.IsConnected() {
		t.Error("new adapter should not be connected")
	}
}

func TestOS9AdapterMetrics(t *testing.T) {
	adapter := NewOS9Adapter(nil)
	metrics := adapter.Metrics()
	if metrics == nil {
		t.Fatal("Metrics() should not return nil")
	}
}

func TestOS9AdapterDisconnect(t *testing.T) {
	adapter := NewOS9Adapter(nil)
	err := adapter.Disconnect(context.Background())
	if err != nil {
		t.Errorf("Disconnect() error = %v, want nil", err)
	}
}

func TestOS9AdapterHealthCheckNotConnected(t *testing.T) {
	adapter := NewOS9Adapter(nil)
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

func TestOS9AdapterRunCommandNilShell(t *testing.T) {
	adapter := NewOS9Adapter(nil)
	_, err := adapter.runCommand(context.Background(), "show version")
	if err == nil {
		t.Error("expected error when shell is nil")
	}
	if err.Error() != "shell not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOS9ParseVersionFacts(t *testing.T) {
	adapter := NewOS9Adapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `Dell Force10 Networks Real Time Operating System Software
Dell Force10 Operating System FTOS: 9.14(2.9)
System Type  : S4048-ON
Serial Number : FTOS123ABC
System uptime is 60 days, 12 hours, 30 minutes
`

	adapter.parseVersionFacts(output, facts)

	if facts.OSVersion != "9.14(2.9)" {
		t.Errorf("OSVersion = %v, want '9.14(2.9)'", facts.OSVersion)
	}
	if facts.Model != "S4048-ON" {
		t.Errorf("Model = %v, want 'S4048-ON'", facts.Model)
	}
	if facts.SerialNumber != "FTOS123ABC" {
		t.Errorf("SerialNumber = %v, want 'FTOS123ABC'", facts.SerialNumber)
	}
	if facts.Uptime <= 0 {
		t.Error("Uptime should be parsed")
	}
}

func TestOS9ParseInterfaces(t *testing.T) {
	adapter := NewOS9Adapter(nil)

	output := `
Interface        Speed  Admin  Link
---------        -----  -----  ----
Te 0/0           10G    up     up
Te 0/1           10G    up     down
Fo 0/48          40G    down   down
Po 1             -      up     up
`

	interfaces := adapter.parseInterfaces(output)
	if len(interfaces) != 4 {
		t.Fatalf("expected 4 interfaces, got %d", len(interfaces))
	}

	if interfaces[0].Name != "Te" {
		t.Errorf("interfaces[0].Name = %v", interfaces[0].Name)
	}
}

func TestNewOS9AdapterFactory(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		factory := NewOS9AdapterFactory(nil)
		adapter, err := factory(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if adapter.Vendor() != vendors.VendorDellOS9 {
			t.Errorf("Vendor() = %v, want VendorDellOS9", adapter.Vendor())
		}
	})

	t.Run("custom vendor config", func(t *testing.T) {
		factory := NewOS9AdapterFactory(nil)
		vendorConfig := &vendors.VendorConfig{Timeout: 90 * time.Second}
		adapter, err := factory(vendorConfig)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		os9Adapter := adapter.(*OS9Adapter)
		if os9Adapter.Config.Timeout != 90*time.Second {
			t.Errorf("Timeout = %v, want 90s", os9Adapter.Config.Timeout)
		}
	})
}

// Verify interface compliance.
var _ vendors.VendorAdapter = (*OS9Adapter)(nil)
