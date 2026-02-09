package alliedtelesis

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

func TestDefaultAWPlusConfig(t *testing.T) {
	cfg := DefaultAWPlusConfig()
	if cfg.VendorConfig == nil {
		t.Error("VendorConfig should not be nil")
	}
	if cfg.EnablePrompt != "#" {
		t.Errorf("EnablePrompt = %v, want '#'", cfg.EnablePrompt)
	}
	if cfg.ConfigPrompt != "(config" {
		t.Errorf("ConfigPrompt = %v, want '(config'", cfg.ConfigPrompt)
	}
}

func TestNewAWPlusAdapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewAWPlusAdapter(nil)
		if adapter == nil {
			t.Fatal("adapter should not be nil")
		}
	})
	t.Run("custom config", func(t *testing.T) {
		cfg := &AWPlusConfig{VendorConfig: &vendors.VendorConfig{Timeout: 30 * time.Second}}
		adapter := NewAWPlusAdapter(cfg)
		if adapter.Config.Timeout != 30*time.Second {
			t.Errorf("Timeout = %v, want 30s", adapter.Config.Timeout)
		}
	})
	t.Run("nil VendorConfig", func(t *testing.T) {
		adapter := NewAWPlusAdapter(&AWPlusConfig{})
		if adapter.Config == nil {
			t.Error("Config should be set to default")
		}
	})
}

func TestAWPlusAdapterVendor(t *testing.T) {
	if NewAWPlusAdapter(nil).Vendor() != vendors.VendorAlliedTelesisAW {
		t.Error("wrong vendor type")
	}
}

func TestAWPlusAdapterType(t *testing.T) {
	if NewAWPlusAdapter(nil).Type() != protocols.ProtocolSSH {
		t.Error("wrong protocol type")
	}
}

func TestAWPlusAdapterIsConnected(t *testing.T) {
	if NewAWPlusAdapter(nil).IsConnected() {
		t.Error("new adapter should not be connected")
	}
}

func TestAWPlusAdapterMetrics(t *testing.T) {
	if NewAWPlusAdapter(nil).Metrics() == nil {
		t.Fatal("Metrics() should not return nil")
	}
}

func TestAWPlusAdapterDisconnect(t *testing.T) {
	if err := NewAWPlusAdapter(nil).Disconnect(context.Background()); err != nil {
		t.Errorf("Disconnect() error = %v", err)
	}
}

func TestAWPlusAdapterHealthCheckNotConnected(t *testing.T) {
	result, err := NewAWPlusAdapter(nil).HealthCheck(context.Background())
	if err != nil {
		t.Errorf("HealthCheck() error = %v", err)
	}
	if result.Healthy {
		t.Error("should not be healthy")
	}
}

func TestAWPlusAdapterRunCommandNilShell(t *testing.T) {
	_, err := NewAWPlusAdapter(nil).runCommand(context.Background(), "show version")
	if err == nil || err.Error() != "shell not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAWPlusParseVersion(t *testing.T) {
	adapter := NewAWPlusAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `AlliedWare Plus (TM) Version 5.5.3-2.1
Build name : x930-5.5.3-2.1.rel
Model Name : AT-x930-28GTX
Uptime     : 30 days, 12 hours, 45 minutes
`
	adapter.parseVersion(output, facts)

	if facts.OSVersion != "5.5.3-2.1" {
		t.Errorf("OSVersion = %v, want '5.5.3-2.1'", facts.OSVersion)
	}
	if facts.Model != "AT-x930-28GTX" {
		t.Errorf("Model = %v, want 'AT-x930-28GTX'", facts.Model)
	}
}

func TestAWPlusParseSystem(t *testing.T) {
	adapter := NewAWPlusAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `Hostname:          at-switch-01
Serial number:     A12345B678
`
	adapter.parseSystem(output, facts)

	if facts.Hostname != "at-switch-01" {
		t.Errorf("Hostname = %v, want 'at-switch-01'", facts.Hostname)
	}
	if facts.SerialNumber != "A12345B678" {
		t.Errorf("SerialNumber = %v, want 'A12345B678'", facts.SerialNumber)
	}
}

func TestParseAWPlusUptime(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"30 days, 12 hours, 45 minutes", 30*24*time.Hour + 12*time.Hour + 45*time.Minute},
		{"1 day, 5 hours, 30 minutes, 10 seconds", 1*24*time.Hour + 5*time.Hour + 30*time.Minute + 10*time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if result := parseAWPlusUptime(tt.input); result != tt.expected {
				t.Errorf("parseAWPlusUptime(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNewAWPlusAdapterFactory(t *testing.T) {
	adapter, err := NewAWPlusAdapterFactory(nil)(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter.Vendor() != vendors.VendorAlliedTelesisAW {
		t.Errorf("Vendor() = %v", adapter.Vendor())
	}
}

var _ vendors.VendorAdapter = (*AWPlusAdapter)(nil)
