package opnsense

import (
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.VendorConfig == nil {
		t.Error("VendorConfig should not be nil")
	}
	if cfg.Port != 443 {
		t.Errorf("Port = %d, want 443", cfg.Port)
	}
	if !cfg.TLS {
		t.Error("TLS should be true by default")
	}
}

func TestNewAdapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewAdapter(nil)
		if adapter == nil {
			t.Fatal("adapter should not be nil")
		}
		if adapter.config.Port != 443 {
			t.Errorf("Port = %d, want 443", adapter.config.Port)
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &Config{
			VendorConfig:       vendors.DefaultVendorConfig(),
			Port:               8443,
			TLS:                true,
			InsecureSkipVerify: true,
		}
		adapter := NewAdapter(cfg)
		if adapter.config.Port != 8443 {
			t.Errorf("Port = %d, want 8443", adapter.config.Port)
		}
		if !adapter.config.InsecureSkipVerify {
			t.Error("InsecureSkipVerify should be true")
		}
	})

	t.Run("nil VendorConfig", func(t *testing.T) {
		cfg := &Config{}
		adapter := NewAdapter(cfg)
		if adapter.Config == nil {
			t.Error("Config should be set to default")
		}
	})
}

func TestAdapterVendor(t *testing.T) {
	adapter := NewAdapter(nil)
	if adapter.Vendor() != vendors.VendorOPNsense {
		t.Errorf("Vendor() = %v, want VendorOPNsense", adapter.Vendor())
	}
}

func TestAdapterType(t *testing.T) {
	adapter := NewAdapter(nil)
	if adapter.Type() != protocols.ProtocolREST {
		t.Errorf("Type() = %v, want ProtocolREST", adapter.Type())
	}
}

func TestAdapterIsConnected(t *testing.T) {
	adapter := NewAdapter(nil)

	if adapter.IsConnected() {
		t.Error("new adapter should not be connected")
	}
}

func TestAdapterMetrics(t *testing.T) {
	adapter := NewAdapter(nil)
	metrics := adapter.Metrics()
	if metrics == nil {
		t.Fatal("Metrics() should not return nil")
	}
}

func TestAdapterDisconnect(t *testing.T) {
	adapter := NewAdapter(nil)

	err := adapter.Disconnect(nil)
	if err != nil {
		t.Errorf("Disconnect() error = %v, want nil", err)
	}
	if adapter.Connected {
		t.Error("Connected should be false")
	}
	if adapter.httpClient != nil {
		t.Error("httpClient should be nil")
	}
}

func TestSystemStatusStructure(t *testing.T) {
	status := SystemStatus{
		Name:      "opnsense",
		Uptime:    86400,
		DateTime:  "2024-01-15 10:30:00",
		Kernel:    "13.2-RELEASE-p4",
		CPU:       "Intel Xeon",
		CPUUsage:  "15%",
		MemTotal:  "8GB",
		MemUsed:   "4GB",
		DiskUsage: "25%",
	}

	if status.Name != "opnsense" {
		t.Errorf("Name = %v", status.Name)
	}
	if status.Uptime != 86400 {
		t.Errorf("Uptime = %d", status.Uptime)
	}
}

func TestFirmwareStatusStructure(t *testing.T) {
	firmware := FirmwareStatus{
		ProductVersion:    "24.1.1",
		ProductName:       "OPNsense",
		ProductArch:       "amd64",
		ProductNickname:   "Savvy Shark",
		ProductHash:       "abc123",
		ProductMirror:     "https://pkg.opnsense.org",
		ProductRepos:      "OPNsense",
		ProductTime:       "2024-01-15T10:00:00Z",
		LastCheck:         "2024-01-15T11:00:00Z",
		OSVersion:         "13.2-RELEASE-p4",
		NeedsReboot:       "0",
		UpgradeNeedReboot: "0",
	}

	if firmware.ProductVersion != "24.1.1" {
		t.Errorf("ProductVersion = %v", firmware.ProductVersion)
	}
	if firmware.ProductName != "OPNsense" {
		t.Errorf("ProductName = %v", firmware.ProductName)
	}
}

func TestInterfaceStatsStructure(t *testing.T) {
	stats := InterfaceStats{
		Name:        "em0",
		Description: "WAN",
		MacAddress:  "00:11:22:33:44:55",
		Status:      "up",
		MTU:         1500,
		IPAddresses: []string{"192.168.1.1"},
		Media:       "1000baseT",
		MediaRaw:    "1000baseT full-duplex",
		BytesIn:     1000000,
		BytesOut:    2000000,
		PacketsIn:   10000,
		PacketsOut:  20000,
		ErrorsIn:    0,
		ErrorsOut:   0,
	}

	if stats.Name != "em0" {
		t.Errorf("Name = %v", stats.Name)
	}
	if stats.Status != "up" {
		t.Errorf("Status = %v", stats.Status)
	}
	if stats.MTU != 1500 {
		t.Errorf("MTU = %d", stats.MTU)
	}
}

func TestAliasStructure(t *testing.T) {
	alias := Alias{
		UUID:        "abc-123-def",
		Name:        "internal_networks",
		Type:        "network",
		Content:     "192.168.0.0/16\n10.0.0.0/8",
		Description: "Internal networks",
		Enabled:     "1",
	}

	if alias.Name != "internal_networks" {
		t.Errorf("Name = %v", alias.Name)
	}
	if alias.Type != "network" {
		t.Errorf("Type = %v", alias.Type)
	}
}

func TestRuleStructure(t *testing.T) {
	rule := Rule{
		UUID:        "rule-123",
		Sequence:    10,
		Interface:   "wan",
		Direction:   "in",
		Action:      "pass",
		Protocol:    "tcp",
		Source:      "any",
		SourcePort:  "",
		Destination: "192.168.1.0/24",
		DestPort:    "443",
		Description: "Allow HTTPS to LAN",
		Enabled:     "1",
	}

	if rule.Interface != "wan" {
		t.Errorf("Interface = %v", rule.Interface)
	}
	if rule.Action != "pass" {
		t.Errorf("Action = %v", rule.Action)
	}
	if rule.DestPort != "443" {
		t.Errorf("DestPort = %v", rule.DestPort)
	}
}

func TestConfigStructure(t *testing.T) {
	cfg := &Config{
		VendorConfig: &vendors.VendorConfig{
			Timeout: 90 * time.Second,
		},
		Port:               8443,
		TLS:                true,
		InsecureSkipVerify: true,
	}

	if cfg.Port != 8443 {
		t.Errorf("Port = %d", cfg.Port)
	}
	if !cfg.TLS {
		t.Error("TLS should be true")
	}
	if !cfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true")
	}
}

func TestAdapterParseInterfaces(t *testing.T) {
	adapter := NewAdapter(nil)
	facts := &vendors.DeviceFacts{}

	// Simulated JSON response
	data := []byte(`{
		"em0": {
			"name": "em0",
			"descr": "WAN",
			"macaddr": "00:11:22:33:44:55",
			"status": "up",
			"mtu": 1500,
			"ipaddr": ["192.168.1.1"]
		},
		"em1": {
			"name": "em1",
			"descr": "LAN",
			"macaddr": "00:11:22:33:44:56",
			"status": "down",
			"mtu": 1500
		}
	}`)

	adapter.parseInterfaces(data, facts)

	if len(facts.Interfaces) != 2 {
		t.Fatalf("expected 2 interfaces, got %d", len(facts.Interfaces))
	}
}

func TestSaveConfig(t *testing.T) {
	adapter := NewAdapter(nil)

	// SaveConfig is a no-op for OPNsense
	err := adapter.SaveConfig(nil)
	if err != nil {
		t.Errorf("SaveConfig() error = %v", err)
	}
}

func TestFactoryRegistration(t *testing.T) {
	// The init() function registers the factory
	adapter, err := vendors.Create(vendors.VendorOPNsense, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter == nil {
		t.Error("adapter should not be nil")
	}
	if adapter.Vendor() != vendors.VendorOPNsense {
		t.Errorf("Vendor() = %v, want VendorOPNsense", adapter.Vendor())
	}
}
