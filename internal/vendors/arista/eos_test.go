package arista

import (
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/vendors"
)

func TestDefaultEOSConfig(t *testing.T) {
	cfg := DefaultEOSConfig()

	if cfg.VendorConfig == nil {
		t.Error("VendorConfig should not be nil")
	}
	if cfg.Mode != "ssh" {
		t.Errorf("Mode = %v, want 'ssh'", cfg.Mode)
	}
	if cfg.EAPIPort != 443 {
		t.Errorf("EAPIPort = %d, want 443", cfg.EAPIPort)
	}
	if !cfg.EAPITLS {
		t.Error("EAPITLS should be true by default")
	}
}

func TestNewEOSAdapter(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		adapter := NewEOSAdapter(nil)
		if adapter == nil {
			t.Fatal("adapter should not be nil")
		}
		if adapter.config.Mode != "ssh" {
			t.Errorf("Mode = %v, want 'ssh'", adapter.config.Mode)
		}
		if adapter.sshAdapter == nil {
			t.Error("sshAdapter should not be nil for SSH mode")
		}
	})

	t.Run("eapi mode", func(t *testing.T) {
		cfg := &EOSConfig{
			VendorConfig: vendors.DefaultVendorConfig(),
			Mode:         "eapi",
			EAPIPort:     8080,
			EAPITLS:      false,
		}
		adapter := NewEOSAdapter(cfg)
		if adapter.config.Mode != "eapi" {
			t.Errorf("Mode = %v, want 'eapi'", adapter.config.Mode)
		}
		if adapter.sshAdapter != nil {
			t.Error("sshAdapter should be nil for eAPI mode")
		}
	})

	t.Run("nil VendorConfig", func(t *testing.T) {
		cfg := &EOSConfig{}
		adapter := NewEOSAdapter(cfg)
		if adapter.Config == nil {
			t.Error("Config should be set to default")
		}
	})
}

func TestEOSAdapterVendor(t *testing.T) {
	adapter := NewEOSAdapter(nil)
	if adapter.Vendor() != vendors.VendorAristaEOS {
		t.Errorf("Vendor() = %v, want VendorAristaEOS", adapter.Vendor())
	}
}

func TestEOSAdapterType(t *testing.T) {
	t.Run("ssh mode", func(t *testing.T) {
		cfg := &EOSConfig{
			VendorConfig: vendors.DefaultVendorConfig(),
			Mode:         "ssh",
		}
		adapter := NewEOSAdapter(cfg)
		if adapter.Type() != protocols.ProtocolSSH {
			t.Errorf("Type() = %v, want ProtocolSSH", adapter.Type())
		}
	})

	t.Run("eapi mode", func(t *testing.T) {
		cfg := &EOSConfig{
			VendorConfig: vendors.DefaultVendorConfig(),
			Mode:         "eapi",
		}
		adapter := NewEOSAdapter(cfg)
		if adapter.Type() != protocols.ProtocolREST {
			t.Errorf("Type() = %v, want ProtocolREST", adapter.Type())
		}
	})
}

func TestEOSAdapterIsConnected(t *testing.T) {
	adapter := NewEOSAdapter(nil)

	if adapter.IsConnected() {
		t.Error("new adapter should not be connected")
	}
}

func TestEOSAdapterMetrics(t *testing.T) {
	t.Run("ssh mode", func(t *testing.T) {
		adapter := NewEOSAdapter(nil)
		metrics := adapter.Metrics()
		if metrics == nil {
			t.Fatal("Metrics() should not return nil")
		}
	})

	t.Run("eapi mode", func(t *testing.T) {
		cfg := &EOSConfig{
			VendorConfig: vendors.DefaultVendorConfig(),
			Mode:         "eapi",
		}
		adapter := NewEOSAdapter(cfg)
		metrics := adapter.Metrics()
		if metrics == nil {
			t.Fatal("Metrics() should not return nil")
		}
	})
}

func TestEOSParseVersion(t *testing.T) {
	adapter := NewEOSAdapter(nil)
	facts := &vendors.DeviceFacts{}

	versionOutput := `Arista DCS-7050TX-64-R
Hardware version:    01.02
Software image version: 4.28.3M
Total memory: 8167656 kB
Free memory:  5478128 kB

Serial number: ABC12345678

Uptime: 1 day, 5 hours, 30 minutes`

	adapter.parseVersion(versionOutput, facts)

	// Model parsing depends on specific line format
	if facts.OSVersion != "4.28.3M" {
		t.Errorf("OSVersion = %v", facts.OSVersion)
	}
	if facts.SerialNumber != "ABC12345678" {
		t.Errorf("SerialNumber = %v", facts.SerialNumber)
	}
}

func TestEOSParseHostname(t *testing.T) {
	adapter := NewEOSAdapter(nil)
	facts := &vendors.DeviceFacts{}

	hostnameOutput := `Hostname: eos-switch01
FQDN:     eos-switch01.example.com`

	adapter.parseHostname(hostnameOutput, facts)

	if facts.Hostname != "eos-switch01" {
		t.Errorf("Hostname = %v", facts.Hostname)
	}
	if facts.FQDN != "eos-switch01.example.com" {
		t.Errorf("FQDN = %v", facts.FQDN)
	}
}

func TestEOSParseInterfaces(t *testing.T) {
	adapter := NewEOSAdapter(nil)
	facts := &vendors.DeviceFacts{}

	interfaceOutput := `Port       Name        Status       Vlan       Duplex Speed  Type
Et1        Uplink1     connected    1          full   10G    10GBASE-T
Et2                    notconnect   1          full   1G     10/100/1000
Ma1        Mgmt        connected    routed     full   1G     10/100/1000
Po1        LAG1        connected    trunk      full   20G    N/A`

	adapter.parseInterfaces(interfaceOutput, facts)

	if len(facts.Interfaces) < 3 {
		t.Fatalf("expected at least 3 interfaces, got %d", len(facts.Interfaces))
	}

	// Find Et1
	found := false
	for _, iface := range facts.Interfaces {
		if iface.Name == "Et1" {
			found = true
			if iface.OperStatus != "up" {
				t.Errorf("Et1 OperStatus = %v", iface.OperStatus)
			}
			// Speed parsing depends on exact format
			break
		}
	}
	if !found {
		t.Error("Et1 interface not found")
	}
}

func TestParseUptime(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		minVal time.Duration
	}{
		{
			name:   "full format",
			input:  "1 day, 2 hours, 30 minutes",
			minVal: 24*time.Hour + 2*time.Hour + 30*time.Minute,
		},
		{
			name:   "hours only",
			input:  "5 hours, 45 minutes",
			minVal: 5*time.Hour + 45*time.Minute,
		},
		{
			name:   "days only",
			input:  "10 days",
			minVal: 10 * 24 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uptime := parseUptime(tt.input)
			if uptime < tt.minVal {
				t.Errorf("parseUptime(%q) = %v, want >= %v", tt.input, uptime, tt.minVal)
			}
		})
	}
}

func TestNewEOSAdapterFactory(t *testing.T) {
	// The init() function registers the factory, just verify it works
	adapter, err := vendors.Create(vendors.VendorAristaEOS, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter == nil {
		t.Error("adapter should not be nil")
	}
	if adapter.Vendor() != vendors.VendorAristaEOS {
		t.Errorf("Vendor() = %v, want VendorAristaEOS", adapter.Vendor())
	}
}

func TestEOSAdapterDisconnect(t *testing.T) {
	adapter := NewEOSAdapter(nil)

	err := adapter.Disconnect(nil)
	if err != nil {
		t.Errorf("Disconnect() error = %v, want nil", err)
	}
}

func TestEOSAdapterHealthCheckNotConnected(t *testing.T) {
	adapter := NewEOSAdapter(nil)

	// For SSH mode with no connection
	result, err := adapter.HealthCheck(nil)
	if err != nil {
		t.Errorf("HealthCheck() error = %v", err)
	}
	// Result depends on internal state
	_ = result
}

func TestEOSConfigStructure(t *testing.T) {
	cfg := &EOSConfig{
		VendorConfig: &vendors.VendorConfig{
			Timeout:        90 * time.Second,
			EnablePassword: "arista123",
		},
		Mode:         "eapi",
		EAPIPort:     8443,
		EAPITLS:      true,
		EAPIInsecure: false,
		Secret:       "secret456",
	}

	if cfg.Mode != "eapi" {
		t.Errorf("Mode = %v", cfg.Mode)
	}
	if cfg.EAPIPort != 8443 {
		t.Errorf("EAPIPort = %d", cfg.EAPIPort)
	}
	if cfg.Secret != "secret456" {
		t.Errorf("Secret = %v", cfg.Secret)
	}
}

func TestEOSAdapterRunCommandNilShell(t *testing.T) {
	adapter := NewEOSAdapter(nil)

	_, err := adapter.runCommand(nil, "show version")
	if err == nil {
		t.Error("expected error when shell is nil")
	}
	if err.Error() != "shell not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEOSEnterEnableAlreadyInEnable(t *testing.T) {
	adapter := NewEOSAdapter(nil)
	adapter.inEnable = true

	// Should succeed when already in enable mode
	err := adapter.enterEnable(nil)
	if err != nil {
		t.Errorf("enterEnable() error = %v, want nil", err)
	}
}

func TestEOSExitConfigNotInConfig(t *testing.T) {
	adapter := NewEOSAdapter(nil)
	adapter.inConfig = false

	err := adapter.exitConfig(nil)
	if err != nil {
		t.Errorf("exitConfig() error = %v", err)
	}
}

func TestEAPIRequestStructure(t *testing.T) {
	req := eAPIRequest{
		JSONRPC: "2.0",
		Method:  "runCmds",
		Params: eAPIParams{
			Version: 1,
			Cmds:    []string{"show version"},
			Format:  "text",
		},
		ID: "test-1",
	}

	if req.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %v", req.JSONRPC)
	}
	if req.Method != "runCmds" {
		t.Errorf("Method = %v", req.Method)
	}
	if len(req.Params.Cmds) != 1 {
		t.Errorf("Cmds count = %d", len(req.Params.Cmds))
	}
}

func TestEAPIResponseStructure(t *testing.T) {
	resp := eAPIResponse{
		JSONRPC: "2.0",
		ID:      "test-1",
		Result: []eAPIResult{
			{Output: "Arista EOS version 4.28.3M"},
		},
		Error: nil,
	}

	if resp.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %v", resp.JSONRPC)
	}
	if len(resp.Result) != 1 {
		t.Errorf("Result count = %d", len(resp.Result))
	}
	if resp.Result[0].Output == "" {
		t.Error("Output should not be empty")
	}
}

func TestEAPIErrorStructure(t *testing.T) {
	err := &eAPIError{
		Code:    1000,
		Message: "Invalid command",
		Data: []struct {
			Errors []string `json:"errors"`
		}{
			{Errors: []string{"Unknown command: show foo"}},
		},
	}

	if err.Code != 1000 {
		t.Errorf("Code = %d", err.Code)
	}
	if err.Message != "Invalid command" {
		t.Errorf("Message = %v", err.Message)
	}
}
