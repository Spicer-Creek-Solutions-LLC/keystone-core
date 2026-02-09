package mellanox

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

func TestDefaultOnyxConfig(t *testing.T) {
	cfg := DefaultOnyxConfig()
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

func TestNewOnyxAdapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewOnyxAdapter(nil)
		if adapter == nil {
			t.Fatal("adapter should not be nil")
		}
	})
	t.Run("custom config", func(t *testing.T) {
		cfg := &OnyxConfig{VendorConfig: &vendors.VendorConfig{Timeout: 30 * time.Second}}
		adapter := NewOnyxAdapter(cfg)
		if adapter.Config.Timeout != 30*time.Second {
			t.Errorf("Timeout = %v, want 30s", adapter.Config.Timeout)
		}
	})
	t.Run("nil VendorConfig", func(t *testing.T) {
		adapter := NewOnyxAdapter(&OnyxConfig{})
		if adapter.Config == nil {
			t.Error("Config should be set to default")
		}
	})
}

func TestOnyxAdapterVendor(t *testing.T) {
	if NewOnyxAdapter(nil).Vendor() != vendors.VendorMellanoxOnyx {
		t.Error("wrong vendor type")
	}
}

func TestOnyxAdapterType(t *testing.T) {
	if NewOnyxAdapter(nil).Type() != protocols.ProtocolSSH {
		t.Error("wrong protocol type")
	}
}

func TestOnyxAdapterIsConnected(t *testing.T) {
	if NewOnyxAdapter(nil).IsConnected() {
		t.Error("new adapter should not be connected")
	}
}

func TestOnyxAdapterMetrics(t *testing.T) {
	if NewOnyxAdapter(nil).Metrics() == nil {
		t.Fatal("Metrics() should not return nil")
	}
}

func TestOnyxAdapterDisconnect(t *testing.T) {
	if err := NewOnyxAdapter(nil).Disconnect(context.Background()); err != nil {
		t.Errorf("Disconnect() error = %v", err)
	}
}

func TestOnyxAdapterHealthCheckNotConnected(t *testing.T) {
	result, err := NewOnyxAdapter(nil).HealthCheck(context.Background())
	if err != nil {
		t.Errorf("HealthCheck() error = %v", err)
	}
	if result.Healthy {
		t.Error("should not be healthy")
	}
}

func TestOnyxAdapterRunCommandNilShell(t *testing.T) {
	_, err := NewOnyxAdapter(nil).runCommand(context.Background(), "show version")
	if err == nil || err.Error() != "shell not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOnyxParseVersion(t *testing.T) {
	adapter := NewOnyxAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `Product name:      MLNX-OS
Product release:   3.10.4102
Uptime:            30 days, 12 hours, 45 minutes
`
	adapter.parseVersion(output, facts)

	if facts.OSVersion != "3.10.4102" {
		t.Errorf("OSVersion = %v, want '3.10.4102'", facts.OSVersion)
	}
}

func TestOnyxParseSystem(t *testing.T) {
	adapter := NewOnyxAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `Hostname:          mlnx-switch-01
Serial number:     MT1234567890
`
	adapter.parseSystem(output, facts)

	if facts.Hostname != "mlnx-switch-01" {
		t.Errorf("Hostname = %v, want 'mlnx-switch-01'", facts.Hostname)
	}
	if facts.SerialNumber != "MT1234567890" {
		t.Errorf("SerialNumber = %v, want 'MT1234567890'", facts.SerialNumber)
	}
}

func TestOnyxParseInterfaces(t *testing.T) {
	adapter := NewOnyxAdapter(nil)

	output := `Port     Admin  Oper   Speed
-------  -----  -----  -----
Eth1/1   up     up     100G
Eth1/2   up     down   100G
Eth1/3   down   down   100G
`

	interfaces := adapter.parseInterfaces(output)
	if len(interfaces) != 3 {
		t.Fatalf("expected 3 interfaces, got %d", len(interfaces))
	}

	if interfaces[0].Name != "Eth1/1" {
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

func TestParseOnyxUptime(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"30 days, 12 hours, 45 minutes", 30*24*time.Hour + 12*time.Hour + 45*time.Minute},
		{"1 day, 5 hours, 30 minutes, 10 seconds", 1*24*time.Hour + 5*time.Hour + 30*time.Minute + 10*time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if result := parseOnyxUptime(tt.input); result != tt.expected {
				t.Errorf("parseOnyxUptime(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNewOnyxAdapterFactory(t *testing.T) {
	adapter, err := NewOnyxAdapterFactory(nil)(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter.Vendor() != vendors.VendorMellanoxOnyx {
		t.Errorf("Vendor() = %v", adapter.Vendor())
	}
}

var _ vendors.VendorAdapter = (*OnyxAdapter)(nil)
