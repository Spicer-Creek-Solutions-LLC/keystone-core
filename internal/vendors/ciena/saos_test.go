package ciena

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

func TestDefaultSAOSConfig(t *testing.T) {
	cfg := DefaultSAOSConfig()
	if cfg.VendorConfig == nil {
		t.Error("VendorConfig should not be nil")
	}
	if cfg.EnablePrompt != ">" {
		t.Errorf("EnablePrompt = %v, want '>'", cfg.EnablePrompt)
	}
}

func TestNewSAOSAdapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewSAOSAdapter(nil)
		if adapter == nil {
			t.Fatal("adapter should not be nil")
		}
	})
	t.Run("custom config", func(t *testing.T) {
		cfg := &SAOSConfig{VendorConfig: &vendors.VendorConfig{Timeout: 30 * time.Second}}
		adapter := NewSAOSAdapter(cfg)
		if adapter.Config.Timeout != 30*time.Second {
			t.Errorf("Timeout = %v, want 30s", adapter.Config.Timeout)
		}
	})
	t.Run("nil VendorConfig", func(t *testing.T) {
		adapter := NewSAOSAdapter(&SAOSConfig{})
		if adapter.Config == nil {
			t.Error("Config should be set to default")
		}
	})
}

func TestSAOSAdapterVendor(t *testing.T) {
	if NewSAOSAdapter(nil).Vendor() != vendors.VendorCienaSAOS {
		t.Error("wrong vendor type")
	}
}

func TestSAOSAdapterType(t *testing.T) {
	if NewSAOSAdapter(nil).Type() != protocols.ProtocolSSH {
		t.Error("wrong protocol type")
	}
}

func TestSAOSAdapterIsConnected(t *testing.T) {
	if NewSAOSAdapter(nil).IsConnected() {
		t.Error("new adapter should not be connected")
	}
}

func TestSAOSAdapterMetrics(t *testing.T) {
	if NewSAOSAdapter(nil).Metrics() == nil {
		t.Fatal("Metrics() should not return nil")
	}
}

func TestSAOSAdapterDisconnect(t *testing.T) {
	if err := NewSAOSAdapter(nil).Disconnect(context.Background()); err != nil {
		t.Errorf("Disconnect() error = %v", err)
	}
}

func TestSAOSAdapterHealthCheckNotConnected(t *testing.T) {
	result, err := NewSAOSAdapter(nil).HealthCheck(context.Background())
	if err != nil {
		t.Errorf("HealthCheck() error = %v", err)
	}
	if result.Healthy {
		t.Error("should not be healthy")
	}
}

func TestSAOSAdapterRunCommandNilShell(t *testing.T) {
	_, err := NewSAOSAdapter(nil).runCommand(context.Background(), "software show")
	if err == nil || err.Error() != "shell not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSAOSParseSoftware(t *testing.T) {
	adapter := NewSAOSAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `Running Package : SAOS 10.7.1
Installed Package : SAOS 10.7.1
`
	adapter.parseSoftware(output, facts)

	if facts.OSVersion != "10.7.1" {
		t.Errorf("OSVersion = %v, want '10.7.1'", facts.OSVersion)
	}
}

func TestSAOSParseChassis(t *testing.T) {
	adapter := NewSAOSAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `Type           : 5171
Serial         : CIENA12345678
Name           : ciena-switch-01
Up Time        : 30 days, 12 hours, 45 minutes
`
	adapter.parseChassis(output, facts)

	if facts.Model != "5171" {
		t.Errorf("Model = %v, want '5171'", facts.Model)
	}
	if facts.SerialNumber != "CIENA12345678" {
		t.Errorf("SerialNumber = %v, want 'CIENA12345678'", facts.SerialNumber)
	}
	if facts.Hostname != "ciena-switch-01" {
		t.Errorf("Hostname = %v, want 'ciena-switch-01'", facts.Hostname)
	}
}

func TestSAOSParsePorts(t *testing.T) {
	adapter := NewSAOSAdapter(nil)

	output := `Port   Admin   Oper   Speed
1/1    enabled up     10G
1/2    enabled down   10G
1/3    disabled down  10G
`

	ports := adapter.parsePorts(output)
	if len(ports) != 3 {
		t.Fatalf("expected 3 ports, got %d", len(ports))
	}

	if ports[0].Name != "1/1" {
		t.Errorf("ports[0].Name = %v, want '1/1'", ports[0].Name)
	}
	if ports[0].AdminStatus != "up" {
		t.Errorf("ports[0].AdminStatus = %v, want 'up'", ports[0].AdminStatus)
	}
	if ports[2].AdminStatus != "down" {
		t.Errorf("ports[2].AdminStatus = %v, want 'down'", ports[2].AdminStatus)
	}
}

func TestParseSAOSUptime(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"30 days, 12 hours, 45 minutes", 30*24*time.Hour + 12*time.Hour + 45*time.Minute},
		{"1 day, 5 hours, 30 minutes, 10 seconds", 1*24*time.Hour + 5*time.Hour + 30*time.Minute + 10*time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if result := parseSAOSUptime(tt.input); result != tt.expected {
				t.Errorf("parseSAOSUptime(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNewSAOSAdapterFactory(t *testing.T) {
	adapter, err := NewSAOSAdapterFactory(nil)(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter.Vendor() != vendors.VendorCienaSAOS {
		t.Errorf("Vendor() = %v", adapter.Vendor())
	}
}

var _ vendors.VendorAdapter = (*SAOSAdapter)(nil)
