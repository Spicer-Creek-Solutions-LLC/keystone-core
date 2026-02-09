package fortinet

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

func TestDefaultFortiOSConfig(t *testing.T) {
	cfg := DefaultFortiOSConfig()

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

func TestNewFortiOSAdapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewFortiOSAdapter(nil)
		if adapter == nil {
			t.Fatal("adapter should not be nil")
		}
		if adapter.sshAdapter == nil {
			t.Error("sshAdapter should not be nil")
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &FortiOSConfig{
			VendorConfig: &vendors.VendorConfig{
				Timeout: 30 * time.Second,
			},
			VDOM: "root",
		}
		adapter := NewFortiOSAdapter(cfg)
		if adapter.Config.Timeout != 30*time.Second {
			t.Errorf("Timeout = %v, want 30s", adapter.Config.Timeout)
		}
		if adapter.vdom != "root" {
			t.Errorf("vdom = %v, want 'root'", adapter.vdom)
		}
	})

	t.Run("nil VendorConfig", func(t *testing.T) {
		cfg := &FortiOSConfig{}
		adapter := NewFortiOSAdapter(cfg)
		if adapter.Config == nil {
			t.Error("Config should be set to default")
		}
	})
}

func TestFortiOSAdapterVendor(t *testing.T) {
	adapter := NewFortiOSAdapter(nil)
	if adapter.Vendor() != vendors.VendorFortiOS {
		t.Errorf("Vendor() = %v, want VendorFortiOS", adapter.Vendor())
	}
}

func TestFortiOSAdapterType(t *testing.T) {
	adapter := NewFortiOSAdapter(nil)
	if adapter.Type() != protocols.ProtocolSSH {
		t.Errorf("Type() = %v, want ProtocolSSH", adapter.Type())
	}
}

func TestFortiOSAdapterIsConnected(t *testing.T) {
	adapter := NewFortiOSAdapter(nil)
	if adapter.IsConnected() {
		t.Error("new adapter should not be connected")
	}
}

func TestFortiOSAdapterMetrics(t *testing.T) {
	adapter := NewFortiOSAdapter(nil)
	metrics := adapter.Metrics()
	if metrics == nil {
		t.Fatal("Metrics() should not return nil")
	}
}

func TestFortiOSAdapterDisconnect(t *testing.T) {
	adapter := NewFortiOSAdapter(nil)
	err := adapter.Disconnect(context.Background())
	if err != nil {
		t.Errorf("Disconnect() error = %v, want nil", err)
	}
}

func TestFortiOSAdapterHealthCheckNotConnected(t *testing.T) {
	adapter := NewFortiOSAdapter(nil)
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

func TestFortiOSAdapterRunCommandNilShell(t *testing.T) {
	adapter := NewFortiOSAdapter(nil)
	_, err := adapter.runCommand(context.Background(), "get system status")
	if err == nil {
		t.Error("expected error when shell is nil")
	}
	if err.Error() != "shell not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFortiOSSaveConfig(t *testing.T) {
	adapter := NewFortiOSAdapter(nil)
	err := adapter.SaveConfig(context.Background())
	if err != nil {
		t.Errorf("SaveConfig() should be no-op, got error: %v", err)
	}
}

func TestFortiOSParseStatus(t *testing.T) {
	adapter := NewFortiOSAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `Version: FortiGate-100F v7.2.4,build1396,230131 (GA.F)
Virus-DB: 1.00000(2018-04-09 18:07)
IPS-DB: 6.00741(2015-12-01 02:30)
Serial-Number: FG100FTK12345678
Hostname: fw-edge-01
Platform Full Name: FortiGate-100F
Uptime: 45 days, 12 hours, 30 minutes
`

	adapter.parseStatus(output, facts)

	if facts.OSVersion != "7.2.4" {
		t.Errorf("OSVersion = %v, want '7.2.4'", facts.OSVersion)
	}
	if facts.Hostname != "fw-edge-01" {
		t.Errorf("Hostname = %v, want 'fw-edge-01'", facts.Hostname)
	}
	if facts.SerialNumber != "FG100FTK12345678" {
		t.Errorf("SerialNumber = %v, want 'FG100FTK12345678'", facts.SerialNumber)
	}
	if facts.Model != "FortiGate-100F" {
		t.Errorf("Model = %v, want 'FortiGate-100F'", facts.Model)
	}
	if facts.Uptime <= 0 {
		t.Error("Uptime should be parsed")
	}
}

func TestFortiOSParsePerformance(t *testing.T) {
	adapter := NewFortiOSAdapter(nil)
	facts := &vendors.DeviceFacts{}

	output := `CPU states: 12% user 3% system 0% nice 85% idle
Memory: 8192 4096 (total used in KB)
`

	adapter.parsePerformance(output, facts)

	if facts.CPUUsage != 12 {
		t.Errorf("CPUUsage = %v, want 12", facts.CPUUsage)
	}
	if facts.MemoryTotal != 8192*1024 {
		t.Errorf("MemoryTotal = %v, want %v", facts.MemoryTotal, 8192*1024)
	}
}

func TestParseFortiUptime(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		minVal time.Duration
	}{
		{
			name:   "days hours minutes",
			input:  "Uptime: 45 days, 12 hours, 30 minutes",
			minVal: 45*24*time.Hour + 12*time.Hour + 30*time.Minute,
		},
		{
			name:   "hours only",
			input:  "Uptime: 5 hours, 10 minutes",
			minVal: 5*time.Hour + 10*time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uptime := parseFortiUptime(tt.input)
			if uptime < tt.minVal {
				t.Errorf("parseFortiUptime(%q) = %v, want >= %v", tt.input, uptime, tt.minVal)
			}
		})
	}
}

func TestNewFortiOSAdapterFactory(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		factory := NewFortiOSAdapterFactory(nil)
		adapter, err := factory(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if adapter.Vendor() != vendors.VendorFortiOS {
			t.Errorf("Vendor() = %v, want VendorFortiOS", adapter.Vendor())
		}
	})

	t.Run("custom vendor config", func(t *testing.T) {
		factory := NewFortiOSAdapterFactory(nil)
		vendorConfig := &vendors.VendorConfig{Timeout: 120 * time.Second}
		adapter, err := factory(vendorConfig)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		fortiAdapter := adapter.(*FortiOSAdapter)
		if fortiAdapter.Config.Timeout != 120*time.Second {
			t.Errorf("Timeout = %v, want 120s", fortiAdapter.Config.Timeout)
		}
	})
}

// Verify interface compliance.
var _ vendors.VendorAdapter = (*FortiOSAdapter)(nil)
