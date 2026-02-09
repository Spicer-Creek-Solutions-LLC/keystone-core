package ubiquiti

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

func TestDefaultEdgeOSConfig(t *testing.T) {
	cfg := DefaultEdgeOSConfig()

	if cfg.VendorConfig == nil {
		t.Error("VendorConfig should not be nil")
	}
	if cfg.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", cfg.Timeout)
	}
	if cfg.EnablePrompt != "$" {
		t.Errorf("EnablePrompt = %v, want '$'", cfg.EnablePrompt)
	}
	if cfg.ConfigPrompt != "#" {
		t.Errorf("ConfigPrompt = %v, want '#'", cfg.ConfigPrompt)
	}
}

func TestNewEdgeOSAdapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewEdgeOSAdapter(nil)
		if adapter == nil {
			t.Fatal("adapter should not be nil")
		}
		if adapter.sshAdapter == nil {
			t.Error("sshAdapter should not be nil")
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &EdgeOSConfig{
			VendorConfig: &vendors.VendorConfig{Timeout: 30 * time.Second},
		}
		adapter := NewEdgeOSAdapter(cfg)
		if adapter.Config.Timeout != 30*time.Second {
			t.Errorf("Timeout = %v, want 30s", adapter.Config.Timeout)
		}
	})

	t.Run("nil VendorConfig", func(t *testing.T) {
		cfg := &EdgeOSConfig{}
		adapter := NewEdgeOSAdapter(cfg)
		if adapter.Config == nil {
			t.Error("Config should be set to default")
		}
	})
}

func TestEdgeOSAdapterVendor(t *testing.T) {
	adapter := NewEdgeOSAdapter(nil)
	if adapter.Vendor() != vendors.VendorUbiquitiEdgeOS {
		t.Errorf("Vendor() = %v, want VendorUbiquitiEdgeOS", adapter.Vendor())
	}
}

func TestEdgeOSAdapterType(t *testing.T) {
	adapter := NewEdgeOSAdapter(nil)
	if adapter.Type() != protocols.ProtocolSSH {
		t.Errorf("Type() = %v, want ProtocolSSH", adapter.Type())
	}
}

func TestEdgeOSAdapterIsConnected(t *testing.T) {
	adapter := NewEdgeOSAdapter(nil)
	if adapter.IsConnected() {
		t.Error("new adapter should not be connected")
	}
}

func TestEdgeOSAdapterMetrics(t *testing.T) {
	adapter := NewEdgeOSAdapter(nil)
	metrics := adapter.Metrics()
	if metrics == nil {
		t.Fatal("Metrics() should not return nil")
	}
}

func TestEdgeOSAdapterDisconnect(t *testing.T) {
	adapter := NewEdgeOSAdapter(nil)
	err := adapter.Disconnect(context.Background())
	if err != nil {
		t.Errorf("Disconnect() error = %v, want nil", err)
	}
	if adapter.inConfigMode {
		t.Error("inConfigMode should be false after disconnect")
	}
}

func TestEdgeOSAdapterHealthCheckNotConnected(t *testing.T) {
	adapter := NewEdgeOSAdapter(nil)
	result, err := adapter.HealthCheck(context.Background())
	if err != nil {
		t.Errorf("HealthCheck() error = %v", err)
	}
	if result.Healthy {
		t.Error("should not be healthy when not connected")
	}
}

func TestEdgeOSAdapterRunCommandNilShell(t *testing.T) {
	adapter := NewEdgeOSAdapter(nil)
	_, err := adapter.runCommand(context.Background(), "show version")
	if err == nil {
		t.Error("expected error when shell is nil")
	}
	if err.Error() != "shell not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEdgeOSParseVersion(t *testing.T) {
	adapter := NewEdgeOSAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `Version:      v2.0.9
HW model:     EdgeRouter X 5-Port
Uptime:       30 days, 12:45
`

	adapter.parseVersion(output, facts)

	if facts.OSVersion != "v2.0.9" {
		t.Errorf("OSVersion = %v, want 'v2.0.9'", facts.OSVersion)
	}
}

func TestEdgeOSParseHardware(t *testing.T) {
	adapter := NewEdgeOSAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `Model: EdgeRouter X 5-Port
Serial #: UBNT-ER-X-ABC123
`

	adapter.parseHardware(output, facts)

	if facts.Model != "EdgeRouter X 5-Port" {
		t.Errorf("Model = %v, want 'EdgeRouter X 5-Port'", facts.Model)
	}
	if facts.SerialNumber != "UBNT-ER-X-ABC123" {
		t.Errorf("SerialNumber = %v, want 'UBNT-ER-X-ABC123'", facts.SerialNumber)
	}
}

func TestEdgeOSParseInterfaces(t *testing.T) {
	adapter := NewEdgeOSAdapter(nil)

	output := `Interface    IP Address       S/L  Description
---------    ----------       ---  -----------
eth0         192.168.1.1/24   u/u  WAN
eth1         10.0.0.1/24      u/u  LAN
eth2         -                u/d  unused
eth3         -                d/d  disabled
`

	interfaces := adapter.parseInterfaces(output)
	if len(interfaces) != 4 {
		t.Fatalf("expected 4 interfaces, got %d", len(interfaces))
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

	if interfaces[2].OperStatus != "down" {
		t.Errorf("interfaces[2].OperStatus = %v, want 'down'", interfaces[2].OperStatus)
	}

	if interfaces[3].AdminStatus != "down" {
		t.Errorf("interfaces[3].AdminStatus = %v, want 'down'", interfaces[3].AdminStatus)
	}
}

func TestParseEdgeOSUptime(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"30 days, 12:45", 30*24*time.Hour + 12*time.Hour + 45*time.Minute},
		{"1 day, 5:30", 1*24*time.Hour + 5*time.Hour + 30*time.Minute},
		{"0:15", 15 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseEdgeOSUptime(tt.input)
			if result != tt.expected {
				t.Errorf("parseEdgeOSUptime(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNewEdgeOSAdapterFactory(t *testing.T) {
	factory := NewEdgeOSAdapterFactory(nil)
	adapter, err := factory(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter.Vendor() != vendors.VendorUbiquitiEdgeOS {
		t.Errorf("Vendor() = %v, want VendorUbiquitiEdgeOS", adapter.Vendor())
	}
}

var _ vendors.VendorAdapter = (*EdgeOSAdapter)(nil)
