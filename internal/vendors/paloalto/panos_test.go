package paloalto

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

func TestDefaultPANOSConfig(t *testing.T) {
	cfg := DefaultPANOSConfig()

	if cfg.VendorConfig == nil {
		t.Error("VendorConfig should not be nil")
	}
	if cfg.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", cfg.Timeout)
	}
	if cfg.EnablePrompt != ">" {
		t.Errorf("EnablePrompt = %v, want '>'", cfg.EnablePrompt)
	}
	if cfg.ConfigPrompt != "#" {
		t.Errorf("ConfigPrompt = %v, want '#'", cfg.ConfigPrompt)
	}
}

func TestNewPANOSAdapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewPANOSAdapter(nil)
		if adapter == nil {
			t.Fatal("adapter should not be nil")
		}
		if adapter.sshAdapter == nil {
			t.Error("sshAdapter should not be nil")
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &PANOSConfig{
			VendorConfig: &vendors.VendorConfig{
				Timeout: 30 * time.Second,
			},
		}
		adapter := NewPANOSAdapter(cfg)
		if adapter.Config.Timeout != 30*time.Second {
			t.Errorf("Timeout = %v, want 30s", adapter.Config.Timeout)
		}
	})

	t.Run("nil VendorConfig", func(t *testing.T) {
		cfg := &PANOSConfig{}
		adapter := NewPANOSAdapter(cfg)
		if adapter.Config == nil {
			t.Error("Config should be set to default")
		}
	})
}

func TestPANOSAdapterVendor(t *testing.T) {
	adapter := NewPANOSAdapter(nil)
	if adapter.Vendor() != vendors.VendorPANOS {
		t.Errorf("Vendor() = %v, want VendorPANOS", adapter.Vendor())
	}
}

func TestPANOSAdapterType(t *testing.T) {
	adapter := NewPANOSAdapter(nil)
	if adapter.Type() != protocols.ProtocolSSH {
		t.Errorf("Type() = %v, want ProtocolSSH", adapter.Type())
	}
}

func TestPANOSAdapterIsConnected(t *testing.T) {
	adapter := NewPANOSAdapter(nil)
	if adapter.IsConnected() {
		t.Error("new adapter should not be connected")
	}
}

func TestPANOSAdapterMetrics(t *testing.T) {
	adapter := NewPANOSAdapter(nil)
	metrics := adapter.Metrics()
	if metrics == nil {
		t.Fatal("Metrics() should not return nil")
	}
}

func TestPANOSAdapterDisconnect(t *testing.T) {
	adapter := NewPANOSAdapter(nil)
	err := adapter.Disconnect(context.Background())
	if err != nil {
		t.Errorf("Disconnect() error = %v, want nil", err)
	}
}

func TestPANOSAdapterHealthCheckNotConnected(t *testing.T) {
	adapter := NewPANOSAdapter(nil)
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

func TestPANOSAdapterRunCommandNilShell(t *testing.T) {
	adapter := NewPANOSAdapter(nil)
	_, err := adapter.runCommand(context.Background(), "show system info")
	if err == nil {
		t.Error("expected error when shell is nil")
	}
	if err.Error() != "shell not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPANOSParseSystemInfo(t *testing.T) {
	adapter := NewPANOSAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `hostname: pa-fw-01
ip-address: 10.0.0.1
netmask: 255.255.255.0
default-gateway: 10.0.0.254
model: PA-850
serial: 012345678901
sw-version: 10.2.3
uptime: 30 days, 5:12:45
`

	adapter.parseSystemInfo(output, facts)

	if facts.Hostname != "pa-fw-01" {
		t.Errorf("Hostname = %v, want 'pa-fw-01'", facts.Hostname)
	}
	if facts.Model != "PA-850" {
		t.Errorf("Model = %v, want 'PA-850'", facts.Model)
	}
	if facts.SerialNumber != "012345678901" {
		t.Errorf("SerialNumber = %v, want '012345678901'", facts.SerialNumber)
	}
	if facts.OSVersion != "10.2.3" {
		t.Errorf("OSVersion = %v, want '10.2.3'", facts.OSVersion)
	}
	if facts.Uptime <= 0 {
		t.Error("Uptime should be parsed")
	}
}

func TestParsePANOSUptime(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		minVal time.Duration
	}{
		{
			name:   "days and time",
			input:  "30 days, 5:12:45",
			minVal: 30*24*time.Hour + 5*time.Hour + 12*time.Minute + 45*time.Second,
		},
		{
			name:   "time only",
			input:  "0 days, 12:30:00",
			minVal: 12*time.Hour + 30*time.Minute,
		},
		{
			name:   "days only",
			input:  "100 days, 0:00:00",
			minVal: 100 * 24 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uptime := parsePANOSUptime(tt.input)
			if uptime < tt.minVal {
				t.Errorf("parsePANOSUptime(%q) = %v, want >= %v", tt.input, uptime, tt.minVal)
			}
		})
	}
}

func TestNewPANOSAdapterFactory(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		factory := NewPANOSAdapterFactory(nil)
		adapter, err := factory(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if adapter.Vendor() != vendors.VendorPANOS {
			t.Errorf("Vendor() = %v, want VendorPANOS", adapter.Vendor())
		}
	})

	t.Run("custom vendor config", func(t *testing.T) {
		factory := NewPANOSAdapterFactory(nil)
		vendorConfig := &vendors.VendorConfig{Timeout: 120 * time.Second}
		adapter, err := factory(vendorConfig)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		panosAdapter := adapter.(*PANOSAdapter)
		if panosAdapter.Config.Timeout != 120*time.Second {
			t.Errorf("Timeout = %v, want 120s", panosAdapter.Config.Timeout)
		}
	})
}

// Verify interface compliance.
var _ vendors.VendorAdapter = (*PANOSAdapter)(nil)
