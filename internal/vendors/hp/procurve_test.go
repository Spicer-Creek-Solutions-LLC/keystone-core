package hp

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

func TestDefaultProCurveConfig(t *testing.T) {
	cfg := DefaultProCurveConfig()

	if cfg.VendorConfig == nil {
		t.Error("VendorConfig should not be nil")
	}
	if cfg.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", cfg.Timeout)
	}
	if cfg.EnablePrompt != "#" {
		t.Errorf("EnablePrompt = %v, want '#'", cfg.EnablePrompt)
	}
	if cfg.ConfigPrompt != "(config)" {
		t.Errorf("ConfigPrompt = %v, want '(config)'", cfg.ConfigPrompt)
	}
}

func TestNewProCurveAdapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewProCurveAdapter(nil)
		if adapter == nil {
			t.Fatal("adapter should not be nil")
		}
		if adapter.sshAdapter == nil {
			t.Error("sshAdapter should not be nil")
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &ProCurveConfig{
			VendorConfig: &vendors.VendorConfig{
				Timeout:        30 * time.Second,
				EnablePassword: "secret123",
			},
		}
		adapter := NewProCurveAdapter(cfg)
		if adapter.Config.Timeout != 30*time.Second {
			t.Errorf("Timeout = %v, want 30s", adapter.Config.Timeout)
		}
	})

	t.Run("nil VendorConfig", func(t *testing.T) {
		cfg := &ProCurveConfig{}
		adapter := NewProCurveAdapter(cfg)
		if adapter.Config == nil {
			t.Error("Config should be set to default")
		}
	})
}

func TestProCurveAdapterVendor(t *testing.T) {
	adapter := NewProCurveAdapter(nil)
	if adapter.Vendor() != vendors.VendorHPProCurve {
		t.Errorf("Vendor() = %v, want VendorHPProCurve", adapter.Vendor())
	}
}

func TestProCurveAdapterType(t *testing.T) {
	adapter := NewProCurveAdapter(nil)
	if adapter.Type() != protocols.ProtocolSSH {
		t.Errorf("Type() = %v, want ProtocolSSH", adapter.Type())
	}
}

func TestProCurveAdapterIsConnected(t *testing.T) {
	adapter := NewProCurveAdapter(nil)
	if adapter.IsConnected() {
		t.Error("new adapter should not be connected")
	}
}

func TestProCurveAdapterMetrics(t *testing.T) {
	adapter := NewProCurveAdapter(nil)
	metrics := adapter.Metrics()
	if metrics == nil {
		t.Fatal("Metrics() should not return nil")
	}
}

func TestProCurveAdapterDisconnect(t *testing.T) {
	adapter := NewProCurveAdapter(nil)
	err := adapter.Disconnect(context.Background())
	if err != nil {
		t.Errorf("Disconnect() error = %v, want nil", err)
	}
}

func TestProCurveAdapterHealthCheckNotConnected(t *testing.T) {
	adapter := NewProCurveAdapter(nil)
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

func TestProCurveAdapterRunCommandNilShell(t *testing.T) {
	adapter := NewProCurveAdapter(nil)
	_, err := adapter.runCommand(context.Background(), "show version")
	if err == nil {
		t.Error("expected error when shell is nil")
	}
	if err.Error() != "shell not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProCurveParseSystemInfo(t *testing.T) {
	adapter := NewProCurveAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `
 Status and Counters - General System Information

  System Name        : switch-01
  System Contact     : admin@example.com
  System Location    : DC1-Rack-5
  System Uptime      : 45d 12h 30m
  Serial Number      : SG123ABC456

  Memory   - Total : 262144 Kbytes
`

	adapter.parseSystemInfo(output, facts)

	if facts.Hostname != "switch-01" {
		t.Errorf("Hostname = %v, want 'switch-01'", facts.Hostname)
	}
	if facts.SerialNumber != "SG123ABC456" {
		t.Errorf("SerialNumber = %v, want 'SG123ABC456'", facts.SerialNumber)
	}
	if facts.Uptime <= 0 {
		t.Error("Uptime should be parsed")
	}
}

func TestProCurveParseVersion(t *testing.T) {
	adapter := NewProCurveAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `
  Image stamp:    /ws/swbuildm/rel_ukiah_qaoff/code/build/lakes(swbuildm_rel_ukiah_qaoff_rel_ukiah)
                  Feb 14 2019 07:33:12
  Software revision : WC.16.10.0003
  ROM Version       : WC.17.02.0003
`

	adapter.parseVersion(output, facts)

	if facts.OSVersion != "WC.16.10.0003" {
		t.Errorf("OSVersion = %v, want 'WC.16.10.0003'", facts.OSVersion)
	}
}

func TestProCurveParseInterfaces(t *testing.T) {
	adapter := NewProCurveAdapter(nil)

	output := `
 Port  Type      Enabled  Status
 ----  --------  -------  ------
 1     100/1000T Yes      Up
 2     100/1000T Yes      Down
 3     100/1000T No       Down
 Trk1  Trunk     Yes      Up
`

	interfaces := adapter.parseInterfaces(output)
	if len(interfaces) != 4 {
		t.Fatalf("expected 4 interfaces, got %d", len(interfaces))
	}

	if interfaces[0].Name != "1" {
		t.Errorf("interfaces[0].Name = %v, want '1'", interfaces[0].Name)
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
			input:  "45d 12h 30m",
			minVal: 45*24*time.Hour + 12*time.Hour + 30*time.Minute,
		},
		{
			name:   "hours minutes",
			input:  "5h 30m",
			minVal: 5*time.Hour + 30*time.Minute,
		},
		{
			name:   "days only",
			input:  "100d",
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

func TestNewProCurveAdapterFactory(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		factory := NewProCurveAdapterFactory(nil)
		adapter, err := factory(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if adapter.Vendor() != vendors.VendorHPProCurve {
			t.Errorf("Vendor() = %v, want VendorHPProCurve", adapter.Vendor())
		}
	})

	t.Run("custom vendor config", func(t *testing.T) {
		factory := NewProCurveAdapterFactory(nil)
		vendorConfig := &vendors.VendorConfig{Timeout: 120 * time.Second}
		adapter, err := factory(vendorConfig)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		proCurveAdapter := adapter.(*ProCurveAdapter)
		if proCurveAdapter.Config.Timeout != 120*time.Second {
			t.Errorf("Timeout = %v, want 120s", proCurveAdapter.Config.Timeout)
		}
	})
}

// Verify interface compliance.
var _ vendors.VendorAdapter = (*ProCurveAdapter)(nil)
