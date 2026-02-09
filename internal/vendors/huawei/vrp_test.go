package huawei

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

func TestDefaultVRPConfig(t *testing.T) {
	cfg := DefaultVRPConfig()
	if cfg.VendorConfig == nil {
		t.Error("VendorConfig should not be nil")
	}
	if cfg.EnablePrompt != ">" {
		t.Errorf("EnablePrompt = %v, want '>'", cfg.EnablePrompt)
	}
	if cfg.ConfigPrompt != "]" {
		t.Errorf("ConfigPrompt = %v, want ']'", cfg.ConfigPrompt)
	}
}

func TestNewVRPAdapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewVRPAdapter(nil)
		if adapter == nil {
			t.Fatal("adapter should not be nil")
		}
	})
	t.Run("custom config", func(t *testing.T) {
		cfg := &VRPConfig{VendorConfig: &vendors.VendorConfig{Timeout: 30 * time.Second}}
		adapter := NewVRPAdapter(cfg)
		if adapter.Config.Timeout != 30*time.Second {
			t.Errorf("Timeout = %v, want 30s", adapter.Config.Timeout)
		}
	})
	t.Run("nil VendorConfig", func(t *testing.T) {
		adapter := NewVRPAdapter(&VRPConfig{})
		if adapter.Config == nil {
			t.Error("Config should be set to default")
		}
	})
}

func TestVRPAdapterVendor(t *testing.T) {
	if NewVRPAdapter(nil).Vendor() != vendors.VendorHuaweiVRP {
		t.Error("wrong vendor type")
	}
}

func TestVRPAdapterType(t *testing.T) {
	if NewVRPAdapter(nil).Type() != protocols.ProtocolSSH {
		t.Error("wrong protocol type")
	}
}

func TestVRPAdapterIsConnected(t *testing.T) {
	if NewVRPAdapter(nil).IsConnected() {
		t.Error("new adapter should not be connected")
	}
}

func TestVRPAdapterMetrics(t *testing.T) {
	if NewVRPAdapter(nil).Metrics() == nil {
		t.Fatal("Metrics() should not return nil")
	}
}

func TestVRPAdapterDisconnect(t *testing.T) {
	if err := NewVRPAdapter(nil).Disconnect(context.Background()); err != nil {
		t.Errorf("Disconnect() error = %v", err)
	}
}

func TestVRPAdapterHealthCheckNotConnected(t *testing.T) {
	result, err := NewVRPAdapter(nil).HealthCheck(context.Background())
	if err != nil {
		t.Errorf("HealthCheck() error = %v", err)
	}
	if result.Healthy {
		t.Error("should not be healthy")
	}
}

func TestVRPAdapterRunCommandNilShell(t *testing.T) {
	_, err := NewVRPAdapter(nil).runCommand(context.Background(), "display version")
	if err == nil || err.Error() != "shell not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestVRPParseVersion(t *testing.T) {
	adapter := NewVRPAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `Huawei Versatile Routing Platform Software
VRP (R) software, Version 8.180 (CE6870EI V200R005C20SPC800)
HUAWEI CE6870-48S6CQ-EI uptime is 30 days, 12 hours, 45 minutes
`
	adapter.parseVersion(output, facts)

	if facts.OSVersion != "8.180 (CE6870EI V200R005C20SPC800)" {
		t.Errorf("OSVersion = %v", facts.OSVersion)
	}
	if facts.Model != "CE6870-48S6CQ-EI" {
		t.Errorf("Model = %v, want 'CE6870-48S6CQ-EI'", facts.Model)
	}
}

func TestVRPParseHostname(t *testing.T) {
	adapter := NewVRPAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := ` sysname huawei-switch-01
`
	adapter.parseHostname(output, facts)

	if facts.Hostname != "huawei-switch-01" {
		t.Errorf("Hostname = %v, want 'huawei-switch-01'", facts.Hostname)
	}
}

func TestVRPParseInterfaces(t *testing.T) {
	adapter := NewVRPAdapter(nil)

	output := `Interface            PHY   Protocol
GE0/0/1              up    up
GE0/0/2              up    down
GE0/0/3              *down down
`

	interfaces := adapter.parseInterfaces(output)
	if len(interfaces) != 3 {
		t.Fatalf("expected 3 interfaces, got %d", len(interfaces))
	}

	if interfaces[0].Name != "GE0/0/1" {
		t.Errorf("interfaces[0].Name = %v", interfaces[0].Name)
	}
	if interfaces[0].AdminStatus != "up" {
		t.Errorf("interfaces[0].AdminStatus = %v, want 'up'", interfaces[0].AdminStatus)
	}
	if interfaces[0].OperStatus != "up" {
		t.Errorf("interfaces[0].OperStatus = %v, want 'up'", interfaces[0].OperStatus)
	}
	if interfaces[2].AdminStatus != "down" {
		t.Errorf("interfaces[2].AdminStatus = %v, want 'down'", interfaces[2].AdminStatus)
	}
}

func TestParseVRPUptime(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"30 days, 12 hours, 45 minutes", 30*24*time.Hour + 12*time.Hour + 45*time.Minute},
		{"1 day, 5 hours, 30 minutes, 10 seconds", 1*24*time.Hour + 5*time.Hour + 30*time.Minute + 10*time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if result := parseVRPUptime(tt.input); result != tt.expected {
				t.Errorf("parseVRPUptime(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNewVRPAdapterFactory(t *testing.T) {
	adapter, err := NewVRPAdapterFactory(nil)(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter.Vendor() != vendors.VendorHuaweiVRP {
		t.Errorf("Vendor() = %v", adapter.Vendor())
	}
}

var _ vendors.VendorAdapter = (*VRPAdapter)(nil)
