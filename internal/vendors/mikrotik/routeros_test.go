package mikrotik

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

func TestDefaultRouterOSConfig(t *testing.T) {
	cfg := DefaultRouterOSConfig()

	if cfg.VendorConfig == nil {
		t.Error("VendorConfig should not be nil")
	}
	if cfg.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", cfg.Timeout)
	}
	if !cfg.DisablePaging {
		t.Error("DisablePaging should be true by default")
	}
}

func TestNewRouterOSAdapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewRouterOSAdapter(nil)
		if adapter == nil {
			t.Fatal("adapter should not be nil")
		}
		if adapter.sshAdapter == nil {
			t.Error("sshAdapter should not be nil")
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &RouterOSConfig{
			VendorConfig: &vendors.VendorConfig{Timeout: 30 * time.Second},
		}
		adapter := NewRouterOSAdapter(cfg)
		if adapter.Config.Timeout != 30*time.Second {
			t.Errorf("Timeout = %v, want 30s", adapter.Config.Timeout)
		}
	})

	t.Run("nil VendorConfig", func(t *testing.T) {
		cfg := &RouterOSConfig{}
		adapter := NewRouterOSAdapter(cfg)
		if adapter.Config == nil {
			t.Error("Config should be set to default")
		}
	})
}

func TestRouterOSAdapterVendor(t *testing.T) {
	adapter := NewRouterOSAdapter(nil)
	if adapter.Vendor() != vendors.VendorMikroTikRouterOS {
		t.Errorf("Vendor() = %v, want VendorMikroTikRouterOS", adapter.Vendor())
	}
}

func TestRouterOSAdapterType(t *testing.T) {
	adapter := NewRouterOSAdapter(nil)
	if adapter.Type() != protocols.ProtocolSSH {
		t.Errorf("Type() = %v, want ProtocolSSH", adapter.Type())
	}
}

func TestRouterOSAdapterIsConnected(t *testing.T) {
	adapter := NewRouterOSAdapter(nil)
	if adapter.IsConnected() {
		t.Error("new adapter should not be connected")
	}
}

func TestRouterOSAdapterMetrics(t *testing.T) {
	adapter := NewRouterOSAdapter(nil)
	metrics := adapter.Metrics()
	if metrics == nil {
		t.Fatal("Metrics() should not return nil")
	}
}

func TestRouterOSAdapterDisconnect(t *testing.T) {
	adapter := NewRouterOSAdapter(nil)
	err := adapter.Disconnect(context.Background())
	if err != nil {
		t.Errorf("Disconnect() error = %v, want nil", err)
	}
}

func TestRouterOSAdapterHealthCheckNotConnected(t *testing.T) {
	adapter := NewRouterOSAdapter(nil)
	result, err := adapter.HealthCheck(context.Background())
	if err != nil {
		t.Errorf("HealthCheck() error = %v", err)
	}
	if result.Healthy {
		t.Error("should not be healthy when not connected")
	}
}

func TestRouterOSAdapterRunCommandNilShell(t *testing.T) {
	adapter := NewRouterOSAdapter(nil)
	_, err := adapter.runCommand(context.Background(), "/system identity print")
	if err == nil {
		t.Error("expected error when shell is nil")
	}
	if err.Error() != "shell not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRouterOSAdapterSaveConfig(t *testing.T) {
	adapter := NewRouterOSAdapter(nil)
	err := adapter.SaveConfig(context.Background())
	if err != nil {
		t.Errorf("SaveConfig() error = %v, want nil (auto-save)", err)
	}
}

func TestRouterOSParseIdentity(t *testing.T) {
	adapter := NewRouterOSAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `  name: MikroTik-Router-01
`

	adapter.parseIdentity(output, facts)

	if facts.Hostname != "MikroTik-Router-01" {
		t.Errorf("Hostname = %v, want 'MikroTik-Router-01'", facts.Hostname)
	}
}

func TestRouterOSParseResource(t *testing.T) {
	adapter := NewRouterOSAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `                   uptime: 30d12h45m10s
                  version: 7.12.1 (stable)
               board-name: hAP ac3
                 cpu-load: 15%
`

	adapter.parseResource(output, facts)

	if facts.OSVersion != "7.12.1 (stable)" {
		t.Errorf("OSVersion = %v, want '7.12.1 (stable)'", facts.OSVersion)
	}
	if facts.Model != "hAP ac3" {
		t.Errorf("Model = %v, want 'hAP ac3'", facts.Model)
	}
	if facts.CPUUsage != 15 {
		t.Errorf("CPUUsage = %v, want 15", facts.CPUUsage)
	}
}

func TestRouterOSParseRouterboard(t *testing.T) {
	adapter := NewRouterOSAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `       routerboard: yes
             model: RouterBOARD 4011iGS+
     serial-number: HEF309ABCDEF
`

	adapter.parseRouterboard(output, facts)

	if facts.SerialNumber != "HEF309ABCDEF" {
		t.Errorf("SerialNumber = %v, want 'HEF309ABCDEF'", facts.SerialNumber)
	}
	if facts.Model != "RouterBOARD 4011iGS+" {
		t.Errorf("Model = %v, want 'RouterBOARD 4011iGS+'", facts.Model)
	}
}

func TestRouterOSParseInterfaces(t *testing.T) {
	adapter := NewRouterOSAdapter(nil)

	output := `Flags: D - dynamic, X - disabled, R - running, S - slave
 #     NAME            TYPE       MTU
 0  R  ether1          ether     1500
 1  RS ether2          ether     1500
 2  X  wlan1           wlan      1500
`

	interfaces := adapter.parseInterfaces(output)
	if len(interfaces) < 2 {
		t.Fatalf("expected at least 2 interfaces, got %d", len(interfaces))
	}
}

func TestParseRouterOSUptime(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"30d12h45m10s", 30*24*time.Hour + 12*time.Hour + 45*time.Minute + 10*time.Second},
		{"1w2d3h", 9*24*time.Hour + 3*time.Hour},
		{"5h30m", 5*time.Hour + 30*time.Minute},
		{"45s", 45 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseRouterOSUptime(tt.input)
			if result != tt.expected {
				t.Errorf("parseRouterOSUptime(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNewRouterOSAdapterFactory(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		factory := NewRouterOSAdapterFactory(nil)
		adapter, err := factory(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if adapter.Vendor() != vendors.VendorMikroTikRouterOS {
			t.Errorf("Vendor() = %v, want VendorMikroTikRouterOS", adapter.Vendor())
		}
	})
}

var _ vendors.VendorAdapter = (*RouterOSAdapter)(nil)
