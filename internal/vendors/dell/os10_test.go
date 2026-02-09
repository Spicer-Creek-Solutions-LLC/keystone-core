package dell

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

func TestDefaultOS10Config(t *testing.T) {
	cfg := DefaultOS10Config()

	if cfg.VendorConfig == nil {
		t.Error("VendorConfig should not be nil")
	}
	if cfg.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", cfg.Timeout)
	}
	if cfg.ConfigPrompt != "(conf" {
		t.Errorf("ConfigPrompt = %v, want '(conf'", cfg.ConfigPrompt)
	}
}

func TestNewOS10Adapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewOS10Adapter(nil)
		if adapter == nil {
			t.Fatal("adapter should not be nil")
		}
		if adapter.sshAdapter == nil {
			t.Error("sshAdapter should not be nil")
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &OS10Config{
			VendorConfig: &vendors.VendorConfig{
				Timeout: 30 * time.Second,
			},
		}
		adapter := NewOS10Adapter(cfg)
		if adapter.Config.Timeout != 30*time.Second {
			t.Errorf("Timeout = %v, want 30s", adapter.Config.Timeout)
		}
	})

	t.Run("nil VendorConfig", func(t *testing.T) {
		cfg := &OS10Config{}
		adapter := NewOS10Adapter(cfg)
		if adapter.Config == nil {
			t.Error("Config should be set to default")
		}
	})
}

func TestOS10AdapterVendor(t *testing.T) {
	adapter := NewOS10Adapter(nil)
	if adapter.Vendor() != vendors.VendorDellOS10 {
		t.Errorf("Vendor() = %v, want VendorDellOS10", adapter.Vendor())
	}
}

func TestOS10AdapterType(t *testing.T) {
	adapter := NewOS10Adapter(nil)
	if adapter.Type() != protocols.ProtocolSSH {
		t.Errorf("Type() = %v, want ProtocolSSH", adapter.Type())
	}
}

func TestOS10AdapterIsConnected(t *testing.T) {
	adapter := NewOS10Adapter(nil)
	if adapter.IsConnected() {
		t.Error("new adapter should not be connected")
	}
}

func TestOS10AdapterMetrics(t *testing.T) {
	adapter := NewOS10Adapter(nil)
	metrics := adapter.Metrics()
	if metrics == nil {
		t.Fatal("Metrics() should not return nil")
	}
}

func TestOS10AdapterDisconnect(t *testing.T) {
	adapter := NewOS10Adapter(nil)
	err := adapter.Disconnect(context.Background())
	if err != nil {
		t.Errorf("Disconnect() error = %v, want nil", err)
	}
}

func TestOS10AdapterHealthCheckNotConnected(t *testing.T) {
	adapter := NewOS10Adapter(nil)
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

func TestOS10AdapterRunCommandNilShell(t *testing.T) {
	adapter := NewOS10Adapter(nil)
	_, err := adapter.runCommand(context.Background(), "show version")
	if err == nil {
		t.Error("expected error when shell is nil")
	}
	if err.Error() != "shell not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOS10ParseVersionFacts(t *testing.T) {
	adapter := NewOS10Adapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `Dell EMC Networking OS10 Enterprise
OS Version : 10.5.1.4
Platform   : S5248F-ON
Serial Number : ABC123XYZ
Up Time    : 30 days, 5 hours, 10 minutes
`

	adapter.parseVersionFacts(output, facts)

	if facts.OSVersion != "10.5.1.4" {
		t.Errorf("OSVersion = %v, want '10.5.1.4'", facts.OSVersion)
	}
	if facts.Model != "S5248F-ON" {
		t.Errorf("Model = %v, want 'S5248F-ON'", facts.Model)
	}
	if facts.SerialNumber != "ABC123XYZ" {
		t.Errorf("SerialNumber = %v, want 'ABC123XYZ'", facts.SerialNumber)
	}
	if facts.Uptime <= 0 {
		t.Error("Uptime should be parsed")
	}
}

func TestOS10ParseSystemFacts(t *testing.T) {
	adapter := NewOS10Adapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `Hostname    : os10-switch-01
Node Name   : primary
`

	adapter.parseSystemFacts(output, facts)

	if facts.Hostname != "os10-switch-01" {
		t.Errorf("Hostname = %v, want 'os10-switch-01'", facts.Hostname)
	}
}

func TestOS10ParseInterfaces(t *testing.T) {
	adapter := NewOS10Adapter(nil)

	output := `
Interface        Status  Speed
---------        ------  -----
ethernet1/1/1    up      up
ethernet1/1/2    up      down
ethernet1/1/3    down    down
port-channel1    up      up
`

	interfaces := adapter.parseInterfaces(output)
	if len(interfaces) != 4 {
		t.Fatalf("expected 4 interfaces, got %d", len(interfaces))
	}

	if interfaces[0].Name != "ethernet1/1/1" {
		t.Errorf("interfaces[0].Name = %v", interfaces[0].Name)
	}
}

func TestParseUptime(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		minVal time.Duration
	}{
		{
			name:   "days hours minutes",
			input:  "Up Time: 30 days, 5 hours, 10 minutes",
			minVal: 30*24*time.Hour + 5*time.Hour + 10*time.Minute,
		},
		{
			name:   "hours only",
			input:  "uptime is 5 hours, 30 minutes",
			minVal: 5*time.Hour + 30*time.Minute,
		},
		{
			name:   "days only",
			input:  "uptime is 100 days",
			minVal: 100 * 24 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uptime := parseUptime(tt.input)
			if uptime < tt.minVal {
				t.Errorf("parseUptime(%q) = %v, want >= %v", tt.input, uptime, tt.minVal)
			}
		})
	}
}

func TestNewOS10AdapterFactory(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		factory := NewOS10AdapterFactory(nil)
		adapter, err := factory(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if adapter.Vendor() != vendors.VendorDellOS10 {
			t.Errorf("Vendor() = %v, want VendorDellOS10", adapter.Vendor())
		}
	})

	t.Run("custom vendor config", func(t *testing.T) {
		factory := NewOS10AdapterFactory(nil)
		vendorConfig := &vendors.VendorConfig{Timeout: 120 * time.Second}
		adapter, err := factory(vendorConfig)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		os10Adapter := adapter.(*OS10Adapter)
		if os10Adapter.Config.Timeout != 120*time.Second {
			t.Errorf("Timeout = %v, want 120s", os10Adapter.Config.Timeout)
		}
	})
}

// Verify interface compliance.
var _ vendors.VendorAdapter = (*OS10Adapter)(nil)
