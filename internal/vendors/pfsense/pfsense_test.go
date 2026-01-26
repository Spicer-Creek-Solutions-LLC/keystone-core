package pfsense

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
	if cfg.APIVersion != "v1" {
		t.Errorf("APIVersion = %v, want 'v1'", cfg.APIVersion)
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
		if adapter.config.APIVersion != "v1" {
			t.Errorf("APIVersion = %v, want 'v1'", adapter.config.APIVersion)
		}
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &Config{
			VendorConfig:       vendors.DefaultVendorConfig(),
			Port:               8443,
			TLS:                true,
			InsecureSkipVerify: true,
			APIVersion:         "v2",
		}
		adapter := NewAdapter(cfg)
		if adapter.config.Port != 8443 {
			t.Errorf("Port = %d, want 8443", adapter.config.Port)
		}
		if adapter.config.APIVersion != "v2" {
			t.Errorf("APIVersion = %v, want 'v2'", adapter.config.APIVersion)
		}
	})

	t.Run("nil VendorConfig", func(t *testing.T) {
		cfg := &Config{}
		adapter := NewAdapter(cfg)
		if adapter.Config == nil {
			t.Error("Config should be set to default")
		}
		if adapter.config.APIVersion != "v1" {
			t.Errorf("APIVersion should default to 'v1'")
		}
	})

	t.Run("empty APIVersion", func(t *testing.T) {
		cfg := &Config{
			VendorConfig: vendors.DefaultVendorConfig(),
			APIVersion:   "",
		}
		adapter := NewAdapter(cfg)
		if adapter.config.APIVersion != "v1" {
			t.Errorf("APIVersion should default to 'v1', got %v", adapter.config.APIVersion)
		}
	})
}

func TestAdapterVendor(t *testing.T) {
	adapter := NewAdapter(nil)
	if adapter.Vendor() != vendors.VendorPfSense {
		t.Errorf("Vendor() = %v, want VendorPfSense", adapter.Vendor())
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
	adapter.token = "test-token"
	adapter.Connected = true

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
	if adapter.token != "" {
		t.Error("token should be empty")
	}
}

func TestAPIResponseStructure(t *testing.T) {
	resp := APIResponse{
		Code:    200,
		Status:  "ok",
		Return:  0,
		Message: "Success",
		Data:    map[string]interface{}{"hostname": "pfsense"},
	}

	if resp.Code != 200 {
		t.Errorf("Code = %d", resp.Code)
	}
	if resp.Status != "ok" {
		t.Errorf("Status = %v", resp.Status)
	}
	if resp.Data == nil {
		t.Error("Data should not be nil")
	}
}

func TestAliasStructure(t *testing.T) {
	alias := Alias{
		ID:          1,
		Name:        "internal_networks",
		Type:        "network",
		Address:     []string{"192.168.0.0/16", "10.0.0.0/8"},
		Description: "Internal networks",
		Detail:      []string{"RFC1918", "RFC1918"},
	}

	if alias.Name != "internal_networks" {
		t.Errorf("Name = %v", alias.Name)
	}
	if alias.Type != "network" {
		t.Errorf("Type = %v", alias.Type)
	}
	if len(alias.Address) != 2 {
		t.Errorf("Address count = %d", len(alias.Address))
	}
}

func TestRuleStructure(t *testing.T) {
	rule := Rule{
		Tracker:     1234567890,
		Type:        "pass",
		Interface:   "wan",
		IPProtocol:  "inet",
		Protocol:    "tcp",
		Source:      "any",
		SrcPort:     "",
		Destination: "192.168.1.0/24",
		DstPort:     "443",
		Description: "Allow HTTPS to LAN",
		Disabled:    false,
		Top:         false,
	}

	if rule.Interface != "wan" {
		t.Errorf("Interface = %v", rule.Interface)
	}
	if rule.Protocol != "tcp" {
		t.Errorf("Protocol = %v", rule.Protocol)
	}
	if rule.DstPort != "443" {
		t.Errorf("DstPort = %v", rule.DstPort)
	}
	if rule.Disabled {
		t.Error("Disabled should be false")
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
		APIVersion:         "v2",
	}

	if cfg.Port != 8443 {
		t.Errorf("Port = %d", cfg.Port)
	}
	if !cfg.TLS {
		t.Error("TLS should be true")
	}
	if cfg.APIVersion != "v2" {
		t.Errorf("APIVersion = %v", cfg.APIVersion)
	}
}

func TestAdapterParseInterfaces(t *testing.T) {
	adapter := NewAdapter(nil)
	facts := &vendors.DeviceFacts{}

	// Simulated API response
	data := []byte(`{
		"code": 200,
		"status": "ok",
		"data": {
			"wan": {
				"status": "up",
				"ipaddr": "192.168.1.1",
				"macaddr": "00:11:22:33:44:55",
				"mtu": 1500,
				"media": "1000baseT"
			},
			"lan": {
				"status": "up",
				"ipaddr": "10.0.0.1",
				"macaddr": "00:11:22:33:44:56",
				"mtu": 1500
			}
		}
	}`)

	adapter.parseInterfaces(data, facts)

	if len(facts.Interfaces) != 2 {
		t.Fatalf("expected 2 interfaces, got %d", len(facts.Interfaces))
	}
}

func TestSaveConfig(t *testing.T) {
	adapter := NewAdapter(nil)

	// SaveConfig is a no-op for pfSense
	err := adapter.SaveConfig(nil)
	if err != nil {
		t.Errorf("SaveConfig() error = %v", err)
	}
}

func TestServiceControl(t *testing.T) {
	adapter := NewAdapter(nil)

	// Test invalid action
	err := adapter.ServiceControl(nil, "dhcpd", "invalid")
	if err == nil {
		t.Error("expected error for invalid action")
	}
	if err.Error() != "unsupported action: invalid" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFactoryRegistration(t *testing.T) {
	// The init() function registers the factory
	adapter, err := vendors.Create(vendors.VendorPfSense, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter == nil {
		t.Error("adapter should not be nil")
	}
	if adapter.Vendor() != vendors.VendorPfSense {
		t.Errorf("Vendor() = %v, want VendorPfSense", adapter.Vendor())
	}
}
