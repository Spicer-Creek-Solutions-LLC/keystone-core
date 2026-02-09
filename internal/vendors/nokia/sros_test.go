package nokia

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

func TestDefaultSROSConfig(t *testing.T) {
	cfg := DefaultSROSConfig()
	if cfg.VendorConfig == nil {
		t.Error("VendorConfig should not be nil")
	}
	if cfg.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", cfg.Timeout)
	}
}

func TestNewSROSAdapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewSROSAdapter(nil)
		if adapter == nil {
			t.Fatal("adapter should not be nil")
		}
	})
	t.Run("custom config", func(t *testing.T) {
		cfg := &SROSConfig{VendorConfig: &vendors.VendorConfig{Timeout: 30 * time.Second}}
		adapter := NewSROSAdapter(cfg)
		if adapter.Config.Timeout != 30*time.Second {
			t.Errorf("Timeout = %v, want 30s", adapter.Config.Timeout)
		}
	})
	t.Run("nil VendorConfig", func(t *testing.T) {
		adapter := NewSROSAdapter(&SROSConfig{})
		if adapter.Config == nil {
			t.Error("Config should be set to default")
		}
	})
}

func TestSROSAdapterVendor(t *testing.T) {
	adapter := NewSROSAdapter(nil)
	if adapter.Vendor() != vendors.VendorNokiaSROS {
		t.Errorf("Vendor() = %v, want VendorNokiaSROS", adapter.Vendor())
	}
}

func TestSROSAdapterType(t *testing.T) {
	adapter := NewSROSAdapter(nil)
	if adapter.Type() != protocols.ProtocolSSH {
		t.Errorf("Type() = %v, want ProtocolSSH", adapter.Type())
	}
}

func TestSROSAdapterIsConnected(t *testing.T) {
	adapter := NewSROSAdapter(nil)
	if adapter.IsConnected() {
		t.Error("new adapter should not be connected")
	}
}

func TestSROSAdapterMetrics(t *testing.T) {
	if NewSROSAdapter(nil).Metrics() == nil {
		t.Fatal("Metrics() should not return nil")
	}
}

func TestSROSAdapterDisconnect(t *testing.T) {
	adapter := NewSROSAdapter(nil)
	if err := adapter.Disconnect(context.Background()); err != nil {
		t.Errorf("Disconnect() error = %v", err)
	}
}

func TestSROSAdapterHealthCheckNotConnected(t *testing.T) {
	adapter := NewSROSAdapter(nil)
	result, err := adapter.HealthCheck(context.Background())
	if err != nil {
		t.Errorf("HealthCheck() error = %v", err)
	}
	if result.Healthy {
		t.Error("should not be healthy when not connected")
	}
}

func TestSROSAdapterRunCommandNilShell(t *testing.T) {
	_, err := NewSROSAdapter(nil).runCommand(context.Background(), "show version")
	if err == nil || err.Error() != "shell not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSROSParseSystemInfo(t *testing.T) {
	adapter := NewSROSAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `System Name            : nokia-pe-01
System Up Time         : 30d 12h 45m 10s
`
	adapter.parseSystemInfo(output, facts)
	if facts.Hostname != "nokia-pe-01" {
		t.Errorf("Hostname = %v, want 'nokia-pe-01'", facts.Hostname)
	}
}

func TestSROSParseVersion(t *testing.T) {
	adapter := NewSROSAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `TiMOS-B-22.10.R3 both/x86_64 Nokia 7750 SR Copyright (c) 2000-2023 Nokia.
`
	adapter.parseVersion(output, facts)
	if facts.OSVersion != "22.10.R3" {
		t.Errorf("OSVersion = %v, want '22.10.R3'", facts.OSVersion)
	}
}

func TestSROSParseChassis(t *testing.T) {
	adapter := NewSROSAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `Type                   : 7750 SR-12
Serial number          : NS1234A5678
`
	adapter.parseChassis(output, facts)
	if facts.Model != "7750 SR-12" {
		t.Errorf("Model = %v, want '7750 SR-12'", facts.Model)
	}
	if facts.SerialNumber != "NS1234A5678" {
		t.Errorf("SerialNumber = %v, want 'NS1234A5678'", facts.SerialNumber)
	}
}

func TestParseSROSUptime(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"30d 12h 45m 10s", 30*24*time.Hour + 12*time.Hour + 45*time.Minute + 10*time.Second},
		{"1d 5h", 1*24*time.Hour + 5*time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if result := parseSROSUptime(tt.input); result != tt.expected {
				t.Errorf("parseSROSUptime(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNewSROSAdapterFactory(t *testing.T) {
	adapter, err := NewSROSAdapterFactory(nil)(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter.Vendor() != vendors.VendorNokiaSROS {
		t.Errorf("Vendor() = %v", adapter.Vendor())
	}
}

var _ vendors.VendorAdapter = (*SROSAdapter)(nil)
