package juniper

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

func TestDefaultJUNOSConfig(t *testing.T) {
	cfg := DefaultJUNOSConfig()

	if cfg.VendorConfig == nil {
		t.Error("VendorConfig should not be nil")
	}
	if cfg.EnablePrompt != ">" {
		t.Errorf("EnablePrompt = %v, want '>'", cfg.EnablePrompt)
	}
	if cfg.ConfigPrompt != "#" {
		t.Errorf("ConfigPrompt = %v, want '#'", cfg.ConfigPrompt)
	}
	if cfg.UseXML != false {
		t.Error("UseXML should be false by default")
	}
}

func TestNewJUNOSAdapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewJUNOSAdapter(nil)
		if adapter == nil {
			t.Fatal("adapter should not be nil")
		}
		if adapter.sshAdapter == nil {
			t.Error("sshAdapter should not be nil")
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &JUNOSConfig{
			VendorConfig: &vendors.VendorConfig{
				Timeout: 30 * time.Second,
			},
			UseXML: true,
		}
		adapter := NewJUNOSAdapter(cfg)
		if adapter.Config.Timeout != 30*time.Second {
			t.Errorf("Timeout = %v, want 30s", adapter.Config.Timeout)
		}
		if !adapter.useXML {
			t.Error("useXML should be true")
		}
	})

	t.Run("nil VendorConfig", func(t *testing.T) {
		cfg := &JUNOSConfig{}
		adapter := NewJUNOSAdapter(cfg)
		if adapter.Config == nil {
			t.Error("Config should be set to default")
		}
	})
}

func TestJUNOSAdapterVendor(t *testing.T) {
	adapter := NewJUNOSAdapter(nil)
	if adapter.Vendor() != vendors.VendorJuniperJUNOS {
		t.Errorf("Vendor() = %v, want VendorJuniperJUNOS", adapter.Vendor())
	}
}

func TestJUNOSAdapterType(t *testing.T) {
	adapter := NewJUNOSAdapter(nil)
	if adapter.Type() != protocols.ProtocolSSH {
		t.Errorf("Type() = %v, want ProtocolSSH", adapter.Type())
	}
}

func TestJUNOSAdapterIsConnected(t *testing.T) {
	adapter := NewJUNOSAdapter(nil)

	if adapter.IsConnected() {
		t.Error("new adapter should not be connected")
	}
}

func TestJUNOSAdapterMetrics(t *testing.T) {
	adapter := NewJUNOSAdapter(nil)
	metrics := adapter.Metrics()
	if metrics == nil {
		t.Fatal("Metrics() should not return nil")
	}
}

func TestJUNOSParseVersionFacts(t *testing.T) {
	adapter := NewJUNOSAdapter(nil)
	facts := &vendors.DeviceFacts{}

	versionOutput := `Hostname: test-router
Model: mx240
Junos: 21.4R3-S2.3
JUNOS OS Kernel 64-bit  [20220915.x]

Chassis                FOC12345678   serial number`

	adapter.parseVersionFacts(versionOutput, facts)

	if facts.Hostname != "test-router" {
		t.Errorf("Hostname = %v", facts.Hostname)
	}
	if facts.Model != "mx240" {
		t.Errorf("Model = %v", facts.Model)
	}
	if facts.OSVersion == "" {
		t.Error("OSVersion should be parsed")
	}
}

func TestJUNOSParseUptime(t *testing.T) {
	adapter := NewJUNOSAdapter(nil)

	tests := []struct {
		name   string
		input  string
		minVal time.Duration
	}{
		{
			name:   "full format",
			input:  "System booted 100 days, 5 hours, 30 minutes, 45 seconds ago",
			minVal: 100*24*time.Hour + 5*time.Hour + 30*time.Minute + 45*time.Second,
		},
		{
			name:   "days and hours",
			input:  "uptime 10 days 5 hours",
			minVal: 10*24*time.Hour + 5*time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uptime := adapter.parseUptime(tt.input)
			if uptime < tt.minVal {
				t.Errorf("parseUptime(%q) = %v, want >= %v", tt.input, uptime, tt.minVal)
			}
		})
	}
}

func TestJUNOSParseInterfaces(t *testing.T) {
	adapter := NewJUNOSAdapter(nil)

	interfaceOutput := `Interface               Admin Link Proto    Local                 Remote
ge-0/0/0                up    up
ge-0/0/0.0              up    up   inet     192.168.1.1/24
ge-0/0/1                down  down
lo0                     up    up
lo0.0                   up    up   inet     10.0.0.1/32`

	interfaces := adapter.parseInterfaces(interfaceOutput)

	if len(interfaces) < 4 {
		t.Fatalf("expected at least 4 interfaces, got %d", len(interfaces))
	}

	// Check first interface
	found := false
	for _, iface := range interfaces {
		if iface.Name == "ge-0/0/0.0" {
			found = true
			if iface.AdminStatus != "up" {
				t.Errorf("AdminStatus = %v", iface.AdminStatus)
			}
			if iface.OperStatus != "up" {
				t.Errorf("OperStatus = %v", iface.OperStatus)
			}
			break
		}
	}
	if !found {
		t.Error("ge-0/0/0.0 interface not found")
	}
}

func TestNewJUNOSAdapterFactory(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		factory := NewJUNOSAdapterFactory(nil)
		if factory == nil {
			t.Fatal("factory should not be nil")
		}

		adapter, err := factory(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if adapter == nil {
			t.Error("adapter should not be nil")
		}
		if adapter.Vendor() != vendors.VendorJuniperJUNOS {
			t.Errorf("Vendor() = %v, want VendorJuniperJUNOS", adapter.Vendor())
		}
	})

	t.Run("custom vendor config", func(t *testing.T) {
		factory := NewJUNOSAdapterFactory(nil)

		vendorConfig := &vendors.VendorConfig{
			Timeout: 120 * time.Second,
		}
		adapter, err := factory(vendorConfig)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		junosAdapter, ok := adapter.(*JUNOSAdapter)
		if !ok {
			t.Fatal("expected *JUNOSAdapter")
		}
		if junosAdapter.Config.Timeout != 120*time.Second {
			t.Errorf("Timeout = %v, want 120s", junosAdapter.Config.Timeout)
		}
	})
}

func TestJUNOSAdapterDisconnect(t *testing.T) {
	adapter := NewJUNOSAdapter(nil)

	err := adapter.Disconnect(context.Background())
	if err != nil {
		t.Errorf("Disconnect() error = %v, want nil", err)
	}
}

func TestJUNOSAdapterHealthCheckNotConnected(t *testing.T) {
	adapter := NewJUNOSAdapter(nil)

	result, err := adapter.HealthCheck(context.Background())
	if err != nil {
		t.Errorf("HealthCheck() error = %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Healthy {
		t.Error("should not be healthy when not connected")
	}
	if result.Status != "not connected" {
		t.Errorf("Status = %v, want 'not connected'", result.Status)
	}
}

func TestJUNOSConfigStructure(t *testing.T) {
	cfg := &JUNOSConfig{
		VendorConfig: &vendors.VendorConfig{
			Timeout:        90 * time.Second,
			EnablePassword: "juniper123",
		},
		UseXML: true,
	}

	if !cfg.UseXML {
		t.Error("UseXML should be true")
	}
	if cfg.Timeout != 90*time.Second {
		t.Errorf("Timeout = %v", cfg.Timeout)
	}
}

func TestJUNOSAdapterRunCommandNilShell(t *testing.T) {
	adapter := NewJUNOSAdapter(nil)

	_, err := adapter.runCommand(context.Background(), "show version")
	if err == nil {
		t.Error("expected error when shell is nil")
	}
	if err.Error() != "shell not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestJUNOSExitConfigNotInConfig(t *testing.T) {
	adapter := NewJUNOSAdapter(nil)
	adapter.inConfig = false

	err := adapter.exitConfig(context.Background())
	if err != nil {
		t.Errorf("exitConfig() error = %v", err)
	}
}

func TestJUNOSCommitNotInConfig(t *testing.T) {
	adapter := NewJUNOSAdapter(nil)
	adapter.inConfig = false

	err := adapter.Commit(context.Background())
	if err == nil {
		t.Error("expected error when not in config mode")
	}
}

func TestJUNOSCommitConfirmNotInConfig(t *testing.T) {
	adapter := NewJUNOSAdapter(nil)
	adapter.inConfig = false

	err := adapter.CommitConfirm(context.Background(), 5)
	if err == nil {
		t.Error("expected error when not in config mode")
	}
}
