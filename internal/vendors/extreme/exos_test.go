package extreme

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

func TestDefaultEXOSConfig(t *testing.T) {
	cfg := DefaultEXOSConfig()

	if cfg.VendorConfig == nil {
		t.Error("VendorConfig should not be nil")
	}
	if cfg.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", cfg.Timeout)
	}
	if cfg.EnablePrompt != "#" {
		t.Errorf("EnablePrompt = %v, want '#'", cfg.EnablePrompt)
	}
}

func TestNewEXOSAdapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewEXOSAdapter(nil)
		if adapter == nil {
			t.Fatal("adapter should not be nil")
		}
		if adapter.sshAdapter == nil {
			t.Error("sshAdapter should not be nil")
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &EXOSConfig{
			VendorConfig: &vendors.VendorConfig{Timeout: 30 * time.Second},
		}
		adapter := NewEXOSAdapter(cfg)
		if adapter.Config.Timeout != 30*time.Second {
			t.Errorf("Timeout = %v, want 30s", adapter.Config.Timeout)
		}
	})

	t.Run("nil VendorConfig", func(t *testing.T) {
		cfg := &EXOSConfig{}
		adapter := NewEXOSAdapter(cfg)
		if adapter.Config == nil {
			t.Error("Config should be set to default")
		}
	})
}

func TestEXOSAdapterVendor(t *testing.T) {
	adapter := NewEXOSAdapter(nil)
	if adapter.Vendor() != vendors.VendorExtremeEXOS {
		t.Errorf("Vendor() = %v, want VendorExtremeEXOS", adapter.Vendor())
	}
}

func TestEXOSAdapterType(t *testing.T) {
	adapter := NewEXOSAdapter(nil)
	if adapter.Type() != protocols.ProtocolSSH {
		t.Errorf("Type() = %v, want ProtocolSSH", adapter.Type())
	}
}

func TestEXOSAdapterIsConnected(t *testing.T) {
	adapter := NewEXOSAdapter(nil)
	if adapter.IsConnected() {
		t.Error("new adapter should not be connected")
	}
}

func TestEXOSAdapterMetrics(t *testing.T) {
	adapter := NewEXOSAdapter(nil)
	metrics := adapter.Metrics()
	if metrics == nil {
		t.Fatal("Metrics() should not return nil")
	}
}

func TestEXOSAdapterDisconnect(t *testing.T) {
	adapter := NewEXOSAdapter(nil)
	err := adapter.Disconnect(context.Background())
	if err != nil {
		t.Errorf("Disconnect() error = %v, want nil", err)
	}
}

func TestEXOSAdapterHealthCheckNotConnected(t *testing.T) {
	adapter := NewEXOSAdapter(nil)
	result, err := adapter.HealthCheck(context.Background())
	if err != nil {
		t.Errorf("HealthCheck() error = %v", err)
	}
	if result.Healthy {
		t.Error("should not be healthy when not connected")
	}
}

func TestEXOSAdapterRunCommandNilShell(t *testing.T) {
	adapter := NewEXOSAdapter(nil)
	_, err := adapter.runCommand(context.Background(), "show version")
	if err == nil {
		t.Error("expected error when shell is nil")
	}
	if err.Error() != "shell not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEXOSParseVersion(t *testing.T) {
	adapter := NewEXOSAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `Switch    : 800606-00-06 1234567890 Rev 6.0 BootROM: 2.0.1.1
IMG:      : ExtremeXOS version 31.7.1.4 by release-manager
`

	adapter.parseVersion(output, facts)

	if facts.OSVersion != "31.7.1.4" {
		t.Errorf("OSVersion = %v, want '31.7.1.4'", facts.OSVersion)
	}
}

func TestEXOSParseSwitchInfo(t *testing.T) {
	adapter := NewEXOSAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `SysName:          exos-switch-01
SysLocation:      Datacenter A
System Type:      X460-G2-48p-10GE4
System UpTime:    30 days 12 hrs 45 min 10 secs
`

	adapter.parseSwitchInfo(output, facts)

	if facts.Hostname != "exos-switch-01" {
		t.Errorf("Hostname = %v, want 'exos-switch-01'", facts.Hostname)
	}
	if facts.Model != "X460-G2-48p-10GE4" {
		t.Errorf("Model = %v, want 'X460-G2-48p-10GE4'", facts.Model)
	}
}

func TestEXOSParsePorts(t *testing.T) {
	adapter := NewEXOSAdapter(nil)

	output := `Port  State   Speed  Duplex
====  =====   =====  ======
1:1   E       1G     Full
1:2   E       1G     Full
1:3   D       -      -
2:1   E       10G    Full
`

	ports := adapter.parsePorts(output)
	if len(ports) != 4 {
		t.Fatalf("expected 4 ports, got %d", len(ports))
	}

	if ports[0].Name != "1:1" {
		t.Errorf("ports[0].Name = %v, want '1:1'", ports[0].Name)
	}
	if ports[0].AdminStatus != "up" {
		t.Errorf("ports[0].AdminStatus = %v, want 'up'", ports[0].AdminStatus)
	}

	if ports[2].AdminStatus != "down" {
		t.Errorf("ports[2].AdminStatus = %v, want 'down'", ports[2].AdminStatus)
	}
}

func TestParseEXOSUptime(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"30 days 12 hrs 45 min 10 secs", 30*24*time.Hour + 12*time.Hour + 45*time.Minute + 10*time.Second},
		{"1 day 5 hrs", 1*24*time.Hour + 5*time.Hour},
		{"45 min 30 secs", 45*time.Minute + 30*time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseEXOSUptime(tt.input)
			if result != tt.expected {
				t.Errorf("parseEXOSUptime(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNewEXOSAdapterFactory(t *testing.T) {
	factory := NewEXOSAdapterFactory(nil)
	adapter, err := factory(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter.Vendor() != vendors.VendorExtremeEXOS {
		t.Errorf("Vendor() = %v, want VendorExtremeEXOS", adapter.Vendor())
	}
}

var _ vendors.VendorAdapter = (*EXOSAdapter)(nil)
