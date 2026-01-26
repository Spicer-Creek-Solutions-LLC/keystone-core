package statemgmt

import (
	"strings"
	"testing"
)

func TestNewWiFiModule(t *testing.T) {
	m := NewWiFiModule()
	if m == nil {
		t.Fatal("NewWiFiModule returned nil")
	}
	if m.Name() != "wifi" {
		t.Errorf("expected name 'wifi', got %q", m.Name())
	}

	states := m.ValidStates()
	expectedStates := []string{"connected", "configured", "absent"}
	if len(states) != len(expectedStates) {
		t.Errorf("expected %d states, got %d", len(expectedStates), len(states))
	}
	for i, s := range expectedStates {
		if states[i] != s {
			t.Errorf("expected state %d to be %q, got %q", i, s, states[i])
		}
	}
}

func TestWiFiModule_ParseConfig(t *testing.T) {
	m := NewWiFiModule()

	tests := []struct {
		name        string
		decl        *StateDeclaration
		expectError bool
		validate    func(*WiFiConfig) error
	}{
		{
			name: "basic WPA2 config",
			decl: &StateDeclaration{
				ID:     "office_wifi",
				State:  "connected",
				Module: "wifi",
				Parameters: map[string]interface{}{
					"ssid":     "Office WiFi",
					"security": "wpa2-psk",
					"password": "secretpass123",
				},
			},
			expectError: false,
			validate: func(c *WiFiConfig) error {
				if c.SSID != "Office WiFi" {
					t.Errorf("expected SSID 'Office WiFi', got %q", c.SSID)
				}
				if c.Security != "wpa2-psk" {
					t.Errorf("expected security 'wpa2-psk', got %q", c.Security)
				}
				if c.Password != "secretpass123" {
					t.Errorf("expected password 'secretpass123', got %q", c.Password)
				}
				if c.Name != "Office WiFi" {
					t.Errorf("expected name to default to SSID, got %q", c.Name)
				}
				return nil
			},
		},
		{
			name: "SSID from ID when not specified",
			decl: &StateDeclaration{
				ID:     "my_network",
				State:  "connected",
				Module: "wifi",
				Parameters: map[string]interface{}{
					"security": "open",
				},
			},
			expectError: false,
			validate: func(c *WiFiConfig) error {
				if c.SSID != "my_network" {
					t.Errorf("expected SSID to default to ID 'my_network', got %q", c.SSID)
				}
				return nil
			},
		},
		{
			name: "security mode normalization - wpa2 alias",
			decl: &StateDeclaration{
				ID:     "test",
				State:  "connected",
				Module: "wifi",
				Parameters: map[string]interface{}{
					"ssid":     "Test",
					"security": "wpa2",
					"password": "password123",
				},
			},
			expectError: false,
			validate: func(c *WiFiConfig) error {
				if c.Security != "wpa2-psk" {
					t.Errorf("expected security 'wpa2-psk' (normalized from wpa2), got %q", c.Security)
				}
				return nil
			},
		},
		{
			name: "security mode normalization - none alias",
			decl: &StateDeclaration{
				ID:     "test",
				State:  "connected",
				Module: "wifi",
				Parameters: map[string]interface{}{
					"ssid":     "Test",
					"security": "none",
				},
			},
			expectError: false,
			validate: func(c *WiFiConfig) error {
				if c.Security != "open" {
					t.Errorf("expected security 'open' (normalized from none), got %q", c.Security)
				}
				return nil
			},
		},
		{
			name: "hidden network with priority",
			decl: &StateDeclaration{
				ID:     "hidden_net",
				State:  "connected",
				Module: "wifi",
				Parameters: map[string]interface{}{
					"ssid":     "HiddenSSID",
					"security": "wpa3",
					"password": "supersecret",
					"hidden":   true,
					"priority": 50,
				},
			},
			expectError: false,
			validate: func(c *WiFiConfig) error {
				if !c.Hidden {
					t.Error("expected hidden to be true")
				}
				if c.Priority != 50 {
					t.Errorf("expected priority 50, got %d", c.Priority)
				}
				return nil
			},
		},
		{
			name: "custom interface and name",
			decl: &StateDeclaration{
				ID:     "custom",
				State:  "connected",
				Module: "wifi",
				Parameters: map[string]interface{}{
					"name":      "My Custom Connection",
					"ssid":      "ActualSSID",
					"security":  "wpa2-psk",
					"password":  "password",
					"interface": "wlan1",
				},
			},
			expectError: false,
			validate: func(c *WiFiConfig) error {
				if c.Name != "My Custom Connection" {
					t.Errorf("expected name 'My Custom Connection', got %q", c.Name)
				}
				if c.SSID != "ActualSSID" {
					t.Errorf("expected SSID 'ActualSSID', got %q", c.SSID)
				}
				if c.Interface != "wlan1" {
					t.Errorf("expected interface 'wlan1', got %q", c.Interface)
				}
				return nil
			},
		},
		{
			name: "auto_connect disabled",
			decl: &StateDeclaration{
				ID:     "no_auto",
				State:  "configured",
				Module: "wifi",
				Parameters: map[string]interface{}{
					"ssid":        "NoAutoNet",
					"security":    "open",
					"auto_connect": false,
				},
			},
			expectError: false,
			validate: func(c *WiFiConfig) error {
				if c.AutoConnect {
					t.Error("expected auto_connect to be false")
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := m.parseWiFiConfig(tt.decl)
			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if tt.validate != nil {
				tt.validate(config)
			}
		})
	}
}

func TestWiFiModule_ValidateConfig(t *testing.T) {
	m := NewWiFiModule()

	tests := []struct {
		name        string
		config      *WiFiConfig
		state       string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid WPA2 config",
			config: &WiFiConfig{
				SSID:     "TestNet",
				Security: "wpa2-psk",
				Password: "password123",
			},
			state:       "connected",
			expectError: false,
		},
		{
			name: "valid open network",
			config: &WiFiConfig{
				SSID:     "OpenNet",
				Security: "open",
			},
			state:       "connected",
			expectError: false,
		},
		{
			name: "missing SSID",
			config: &WiFiConfig{
				SSID:     "",
				Security: "wpa2-psk",
				Password: "password",
			},
			state:       "connected",
			expectError: true,
			errorMsg:    "ssid is required",
		},
		{
			name: "SSID too long",
			config: &WiFiConfig{
				SSID:     "This SSID is way too long and exceeds the 32 character limit",
				Security: "open",
			},
			state:       "connected",
			expectError: true,
			errorMsg:    "ssid must be 32 characters or less",
		},
		{
			name: "invalid security mode",
			config: &WiFiConfig{
				SSID:     "TestNet",
				Security: "wpa4",
				Password: "password",
			},
			state:       "connected",
			expectError: true,
			errorMsg:    "invalid security mode",
		},
		{
			name: "missing password for WPA2",
			config: &WiFiConfig{
				SSID:     "TestNet",
				Security: "wpa2-psk",
				Password: "",
			},
			state:       "connected",
			expectError: true,
			errorMsg:    "password is required",
		},
		{
			name: "WPA password too short",
			config: &WiFiConfig{
				SSID:     "TestNet",
				Security: "wpa2-psk",
				Password: "short",
			},
			state:       "connected",
			expectError: true,
			errorMsg:    "WPA password must be 8-63 characters",
		},
		{
			name: "WPA password too long",
			config: &WiFiConfig{
				SSID:     "TestNet",
				Security: "wpa2-psk",
				Password: strings.Repeat("a", 64),
			},
			state:       "connected",
			expectError: true,
			errorMsg:    "WPA password must be 8-63 characters",
		},
		{
			name: "WPA password at minimum length",
			config: &WiFiConfig{
				SSID:     "TestNet",
				Security: "wpa2-psk",
				Password: "12345678",
			},
			state:       "connected",
			expectError: false,
		},
		{
			name: "WPA password at maximum length",
			config: &WiFiConfig{
				SSID:     "TestNet",
				Security: "wpa2-psk",
				Password: strings.Repeat("a", 63),
			},
			state:       "connected",
			expectError: false,
		},
		{
			name: "priority out of range (negative)",
			config: &WiFiConfig{
				SSID:     "TestNet",
				Security: "open",
				Priority: -1,
			},
			state:       "connected",
			expectError: true,
			errorMsg:    "priority must be 0-100",
		},
		{
			name: "priority out of range (too high)",
			config: &WiFiConfig{
				SSID:     "TestNet",
				Security: "open",
				Priority: 101,
			},
			state:       "connected",
			expectError: true,
			errorMsg:    "priority must be 0-100",
		},
		{
			name: "absent state only needs SSID",
			config: &WiFiConfig{
				SSID: "ToDelete",
			},
			state:       "absent",
			expectError: false,
		},
		{
			name: "valid WEP key (5 chars)",
			config: &WiFiConfig{
				SSID:     "WEPNet",
				Security: "wep",
				Password: "abcde",
			},
			state:       "connected",
			expectError: false,
		},
		{
			name: "valid WEP key (13 chars)",
			config: &WiFiConfig{
				SSID:     "WEPNet",
				Security: "wep",
				Password: "abcdefghijklm",
			},
			state:       "connected",
			expectError: false,
		},
		{
			name: "invalid WEP key length",
			config: &WiFiConfig{
				SSID:     "WEPNet",
				Security: "wep",
				Password: "abc",
			},
			state:       "connected",
			expectError: true,
			errorMsg:    "WEP key must be 5 or 13 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.validateWiFiConfig(tt.config, tt.state)
			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
					return
				}
				if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidWiFiSecurityModes(t *testing.T) {
	validModes := []string{"wpa2-psk", "wpa2", "wpa3", "wpa3-personal", "wep", "open", "none"}
	for _, mode := range validModes {
		if !validWiFiSecurityModes[mode] {
			t.Errorf("expected %q to be a valid security mode", mode)
		}
	}

	invalidModes := []string{"wpa4", "wpa1", "invalid", ""}
	for _, mode := range invalidModes {
		if validWiFiSecurityModes[mode] {
			t.Errorf("expected %q to be an invalid security mode", mode)
		}
	}
}

func TestWiFiModule_GenerateWpaSupplicantNetworkBlock(t *testing.T) {
	m := NewWiFiModule()

	tests := []struct {
		name     string
		config   *WiFiConfig
		contains []string
	}{
		{
			name: "WPA2 network",
			config: &WiFiConfig{
				SSID:     "TestNet",
				Security: "wpa2-psk",
				Password: "mypassword",
			},
			contains: []string{
				`ssid="TestNet"`,
				`key_mgmt=WPA-PSK`,
				`psk="mypassword"`,
			},
		},
		{
			name: "open network",
			config: &WiFiConfig{
				SSID:     "OpenNet",
				Security: "open",
			},
			contains: []string{
				`ssid="OpenNet"`,
				`key_mgmt=NONE`,
			},
		},
		{
			name: "hidden network with priority",
			config: &WiFiConfig{
				SSID:     "HiddenNet",
				Security: "wpa2-psk",
				Password: "secret",
				Hidden:   true,
				Priority: 10,
			},
			contains: []string{
				`ssid="HiddenNet"`,
				`scan_ssid=1`,
				`priority=10`,
			},
		},
		{
			name: "WEP network",
			config: &WiFiConfig{
				SSID:     "WEPNet",
				Security: "wep",
				Password: "wepkey",
			},
			contains: []string{
				`ssid="WEPNet"`,
				`key_mgmt=NONE`,
				`wep_key0="wepkey"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := m.generateWpaSupplicantNetworkBlock(tt.config)

			if !strings.HasPrefix(output, "network={") {
				t.Error("expected output to start with 'network={'")
			}
			if !strings.HasSuffix(strings.TrimSpace(output), "}") {
				t.Error("expected output to end with '}'")
			}

			for _, expected := range tt.contains {
				if !strings.Contains(output, expected) {
					t.Errorf("expected output to contain %q\nGot:\n%s", expected, output)
				}
			}
		})
	}
}

func TestWiFiModule_GenerateWindowsProfileXML(t *testing.T) {
	m := NewWiFiModule()

	tests := []struct {
		name     string
		config   *WiFiConfig
		contains []string
	}{
		{
			name: "WPA2 network",
			config: &WiFiConfig{
				Name:        "TestConnection",
				SSID:        "TestNet",
				Security:    "wpa2-psk",
				Password:    "mypassword",
				AutoConnect: true,
			},
			contains: []string{
				`<name>TestConnection</name>`,
				`<name>TestNet</name>`,
				`<authentication>WPA2PSK</authentication>`,
				`<encryption>AES</encryption>`,
				`<keyMaterial>mypassword</keyMaterial>`,
				`<connectionMode>auto</connectionMode>`,
			},
		},
		{
			name: "open network",
			config: &WiFiConfig{
				Name:        "OpenConn",
				SSID:        "OpenNet",
				Security:    "open",
				AutoConnect: false,
			},
			contains: []string{
				`<authentication>open</authentication>`,
				`<encryption>none</encryption>`,
				`<connectionMode>manual</connectionMode>`,
			},
		},
		{
			name: "hidden network",
			config: &WiFiConfig{
				Name:     "HiddenConn",
				SSID:     "HiddenNet",
				Security: "wpa2-psk",
				Password: "secret",
				Hidden:   true,
			},
			contains: []string{
				`<nonBroadcast>true</nonBroadcast>`,
			},
		},
		{
			name: "WPA3 network",
			config: &WiFiConfig{
				Name:     "WPA3Conn",
				SSID:     "WPA3Net",
				Security: "wpa3",
				Password: "wpa3password",
			},
			contains: []string{
				`<authentication>WPA3SAE</authentication>`,
				`<encryption>AES</encryption>`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := m.generateWindowsProfileXML(tt.config)

			if !strings.Contains(output, `<?xml version="1.0"?>`) {
				t.Error("expected XML declaration")
			}
			if !strings.Contains(output, `<WLANProfile`) {
				t.Error("expected WLANProfile element")
			}

			for _, expected := range tt.contains {
				if !strings.Contains(output, expected) {
					t.Errorf("expected output to contain %q\nGot:\n%s", expected, output)
				}
			}
		})
	}
}

func TestWiFiBackendConstants(t *testing.T) {
	// Verify backend constants are distinct
	backends := []WiFiBackend{
		WBUnknown,
		WBNetworkManager,
		WBWpaSupplicant,
		WBNetworkSetup,
		WBNetshWlan,
	}

	seen := make(map[WiFiBackend]bool)
	for _, b := range backends {
		if seen[b] {
			t.Errorf("duplicate backend constant: %s", b)
		}
		seen[b] = true
	}
}

// Interface compliance test
var _ Module = (*WiFiModule)(nil)
