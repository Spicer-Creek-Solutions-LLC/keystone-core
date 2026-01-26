package statemgmt

import (
	"strings"
	"testing"
)

// Ensure Dot1xModule implements the Module interface
var _ Module = (*Dot1xModule)(nil)

func TestNewDot1xModule(t *testing.T) {
	m := NewDot1xModule()
	if m == nil {
		t.Fatal("NewDot1xModule returned nil")
	}
	if m.Name() != "dot1x" {
		t.Errorf("expected name 'dot1x', got %q", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"enabled", "disabled"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
	for i, s := range expected {
		if states[i] != s {
			t.Errorf("expected state %q at position %d, got %q", s, i, states[i])
		}
	}
}

func TestDot1xModule_ParseConfig(t *testing.T) {
	m := NewDot1xModule()

	tests := []struct {
		name       string
		params     map[string]interface{}
		wantConfig *Dot1xConfig
		wantErr    bool
	}{
		{
			name: "EAP-TLS config",
			params: map[string]interface{}{
				"interface":   "eth0",
				"eap_method":  "tls",
				"identity":    "user@example.com",
				"client_cert": "/etc/pki/client.crt",
				"client_key":  "/etc/pki/client.key",
				"ca_cert":     "/etc/pki/ca.crt",
			},
			wantConfig: &Dot1xConfig{
				Name:       "dot1x-eth0",
				Interface:  "eth0",
				EAPMethod:  "tls",
				Identity:   "user@example.com",
				ClientCert: "/etc/pki/client.crt",
				ClientKey:  "/etc/pki/client.key",
				CACert:     "/etc/pki/ca.crt",
			},
		},
		{
			name: "EAP-PEAP config",
			params: map[string]interface{}{
				"interface":  "eth0",
				"eap_method": "peap",
				"identity":   "user",
				"password":   "secret",
				"phase2":     "mschapv2",
			},
			wantConfig: &Dot1xConfig{
				Name:      "dot1x-eth0",
				Interface: "eth0",
				EAPMethod: "peap",
				Identity:  "user",
				Password:  "secret",
				Phase2:    "mschapv2",
			},
		},
		{
			name: "EAP-TTLS config with anonymous identity",
			params: map[string]interface{}{
				"interface":          "eth1",
				"eap":                "ttls", // Test alternate parameter name
				"identity":           "user@domain.com",
				"password":           "pass123",
				"inner_auth":         "pap", // Test alternate parameter name
				"anonymous_identity": "anon@domain.com",
				"ca_cert":            "/etc/ssl/ca.pem",
			},
			wantConfig: &Dot1xConfig{
				Name:      "dot1x-eth1",
				Interface: "eth1",
				EAPMethod: "ttls",
				Identity:  "user@domain.com",
				Password:  "pass123",
				Phase2:    "pap",
				Anonymous: "anon@domain.com",
				CACert:    "/etc/ssl/ca.pem",
			},
		},
		{
			name: "custom name",
			params: map[string]interface{}{
				"interface":  "eth0",
				"name":       "corporate-auth",
				"eap_method": "peap",
				"identity":   "user",
				"password":   "secret",
			},
			wantConfig: &Dot1xConfig{
				Name:      "corporate-auth",
				Interface: "eth0",
				EAPMethod: "peap",
				Identity:  "user",
				Password:  "secret",
			},
		},
		{
			name:    "missing interface",
			params:  map[string]interface{}{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decl := &StateDeclaration{
				Parameters: tt.params,
			}
			config, err := m.parseDot1xConfig(decl)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if config.Name != tt.wantConfig.Name {
				t.Errorf("Name: got %q, want %q", config.Name, tt.wantConfig.Name)
			}
			if config.Interface != tt.wantConfig.Interface {
				t.Errorf("Interface: got %q, want %q", config.Interface, tt.wantConfig.Interface)
			}
			if config.EAPMethod != tt.wantConfig.EAPMethod {
				t.Errorf("EAPMethod: got %q, want %q", config.EAPMethod, tt.wantConfig.EAPMethod)
			}
			if config.Identity != tt.wantConfig.Identity {
				t.Errorf("Identity: got %q, want %q", config.Identity, tt.wantConfig.Identity)
			}
			if config.Password != tt.wantConfig.Password {
				t.Errorf("Password: got %q, want %q", config.Password, tt.wantConfig.Password)
			}
			if config.ClientCert != tt.wantConfig.ClientCert {
				t.Errorf("ClientCert: got %q, want %q", config.ClientCert, tt.wantConfig.ClientCert)
			}
			if config.ClientKey != tt.wantConfig.ClientKey {
				t.Errorf("ClientKey: got %q, want %q", config.ClientKey, tt.wantConfig.ClientKey)
			}
			if config.CACert != tt.wantConfig.CACert {
				t.Errorf("CACert: got %q, want %q", config.CACert, tt.wantConfig.CACert)
			}
			if config.Phase2 != tt.wantConfig.Phase2 {
				t.Errorf("Phase2: got %q, want %q", config.Phase2, tt.wantConfig.Phase2)
			}
			if config.Anonymous != tt.wantConfig.Anonymous {
				t.Errorf("Anonymous: got %q, want %q", config.Anonymous, tt.wantConfig.Anonymous)
			}
		})
	}
}

func TestDot1xModule_ValidateConfig(t *testing.T) {
	m := NewDot1xModule()

	tests := []struct {
		name      string
		config    *Dot1xConfig
		state     string
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid EAP-TLS config",
			config: &Dot1xConfig{
				Interface:  "eth0",
				EAPMethod:  "tls",
				Identity:   "user@example.com",
				ClientCert: "/etc/pki/client.crt",
				ClientKey:  "/etc/pki/client.key",
			},
			state: "enabled",
		},
		{
			name: "valid EAP-PEAP config",
			config: &Dot1xConfig{
				Interface: "eth0",
				EAPMethod: "peap",
				Identity:  "user",
				Password:  "secret",
			},
			state: "enabled",
		},
		{
			name: "valid EAP-TTLS config",
			config: &Dot1xConfig{
				Interface: "eth0",
				EAPMethod: "ttls",
				Identity:  "user",
				Password:  "secret",
			},
			state: "enabled",
		},
		{
			name: "disabled state only needs interface",
			config: &Dot1xConfig{
				Interface: "eth0",
			},
			state: "disabled",
		},
		{
			name: "missing interface",
			config: &Dot1xConfig{
				EAPMethod: "peap",
				Identity:  "user",
				Password:  "secret",
			},
			state:     "enabled",
			wantErr:   true,
			errSubstr: "interface is required",
		},
		{
			name: "missing eap_method",
			config: &Dot1xConfig{
				Interface: "eth0",
				Identity:  "user",
			},
			state:     "enabled",
			wantErr:   true,
			errSubstr: "eap_method is required",
		},
		{
			name: "invalid eap_method",
			config: &Dot1xConfig{
				Interface: "eth0",
				EAPMethod: "invalid",
				Identity:  "user",
			},
			state:     "enabled",
			wantErr:   true,
			errSubstr: "invalid EAP method",
		},
		{
			name: "missing identity",
			config: &Dot1xConfig{
				Interface: "eth0",
				EAPMethod: "peap",
			},
			state:     "enabled",
			wantErr:   true,
			errSubstr: "identity is required",
		},
		{
			name: "EAP-TLS missing client_cert",
			config: &Dot1xConfig{
				Interface: "eth0",
				EAPMethod: "tls",
				Identity:  "user",
				ClientKey: "/etc/pki/client.key",
			},
			state:     "enabled",
			wantErr:   true,
			errSubstr: "client_cert is required",
		},
		{
			name: "EAP-TLS missing client_key",
			config: &Dot1xConfig{
				Interface:  "eth0",
				EAPMethod:  "tls",
				Identity:   "user",
				ClientCert: "/etc/pki/client.crt",
			},
			state:     "enabled",
			wantErr:   true,
			errSubstr: "client_key is required",
		},
		{
			name: "EAP-PEAP missing password",
			config: &Dot1xConfig{
				Interface: "eth0",
				EAPMethod: "peap",
				Identity:  "user",
			},
			state:     "enabled",
			wantErr:   true,
			errSubstr: "password is required",
		},
		{
			name: "EAP-TTLS missing password",
			config: &Dot1xConfig{
				Interface: "eth0",
				EAPMethod: "ttls",
				Identity:  "user",
			},
			state:     "enabled",
			wantErr:   true,
			errSubstr: "password is required",
		},
		{
			name: "invalid phase2 method",
			config: &Dot1xConfig{
				Interface: "eth0",
				EAPMethod: "peap",
				Identity:  "user",
				Password:  "secret",
				Phase2:    "invalid",
			},
			state:     "enabled",
			wantErr:   true,
			errSubstr: "invalid phase2 method",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.validateDot1xConfig(tt.config, tt.state)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
					return
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidEAPMethods(t *testing.T) {
	validMethods := []string{"tls", "ttls", "peap"}
	invalidMethods := []string{"md5", "leap", "fast", "invalid", ""}

	for _, method := range validMethods {
		if !validEAPMethods[method] {
			t.Errorf("expected %q to be a valid EAP method", method)
		}
	}

	for _, method := range invalidMethods {
		if validEAPMethods[method] {
			t.Errorf("expected %q to be an invalid EAP method", method)
		}
	}
}

func TestValidPhase2Methods(t *testing.T) {
	validMethods := []string{"mschapv2", "pap", "chap", "md5", "gtc", ""}
	invalidMethods := []string{"invalid", "eap-md5", "none"}

	for _, method := range validMethods {
		if !validPhase2Methods[method] {
			t.Errorf("expected %q to be a valid phase2 method", method)
		}
	}

	for _, method := range invalidMethods {
		if validPhase2Methods[method] {
			t.Errorf("expected %q to be an invalid phase2 method", method)
		}
	}
}

func TestDot1xModule_GenerateWpaSupplicantConfig(t *testing.T) {
	m := NewDot1xModule()

	tests := []struct {
		name     string
		config   *Dot1xConfig
		contains []string
	}{
		{
			name: "EAP-TLS",
			config: &Dot1xConfig{
				Interface:  "eth0",
				EAPMethod:  "tls",
				Identity:   "user@example.com",
				ClientCert: "/etc/pki/client.crt",
				ClientKey:  "/etc/pki/client.key",
				CACert:     "/etc/pki/ca.crt",
			},
			contains: []string{
				"key_mgmt=IEEE8021X",
				"eap=TLS",
				`identity="user@example.com"`,
				`client_cert="/etc/pki/client.crt"`,
				`private_key="/etc/pki/client.key"`,
				`ca_cert="/etc/pki/ca.crt"`,
			},
		},
		{
			name: "EAP-PEAP",
			config: &Dot1xConfig{
				Interface: "eth0",
				EAPMethod: "peap",
				Identity:  "user",
				Password:  "secret",
				Phase2:    "mschapv2",
				CACert:    "/etc/ssl/ca.pem",
			},
			contains: []string{
				"key_mgmt=IEEE8021X",
				"eap=PEAP",
				`identity="user"`,
				`password="secret"`,
				`phase2="auth=MSCHAPV2"`,
				`ca_cert="/etc/ssl/ca.pem"`,
			},
		},
		{
			name: "EAP-TTLS with anonymous identity",
			config: &Dot1xConfig{
				Interface: "eth0",
				EAPMethod: "ttls",
				Identity:  "user@domain.com",
				Password:  "pass123",
				Phase2:    "pap",
				Anonymous: "anon@domain.com",
			},
			contains: []string{
				"key_mgmt=IEEE8021X",
				"eap=TTLS",
				`identity="user@domain.com"`,
				`password="pass123"`,
				`phase2="auth=PAP"`,
				`anonymous_identity="anon@domain.com"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := m.generateWpaSupplicantDot1xConfig(tt.config)

			for _, substr := range tt.contains {
				if !strings.Contains(output, substr) {
					t.Errorf("output missing expected substring %q\nOutput:\n%s", substr, output)
				}
			}
		})
	}
}

func TestDot1xModule_GenerateWindowsProfileXML(t *testing.T) {
	m := NewDot1xModule()

	tests := []struct {
		name     string
		config   *Dot1xConfig
		contains []string
	}{
		{
			name: "EAP-TLS",
			config: &Dot1xConfig{
				Interface:  "eth0",
				EAPMethod:  "tls",
				Identity:   "user@example.com",
				ClientCert: "/etc/pki/client.crt",
				ClientKey:  "/etc/pki/client.key",
			},
			contains: []string{
				"<OneXEnabled>true</OneXEnabled>",
				">13</Type>", // EAP-TLS type
				"EapTlsConnectionPropertiesV1",
			},
		},
		{
			name: "EAP-PEAP",
			config: &Dot1xConfig{
				Interface: "eth0",
				EAPMethod: "peap",
				Identity:  "user",
				Password:  "secret",
				Phase2:    "mschapv2",
			},
			contains: []string{
				"<OneXEnabled>true</OneXEnabled>",
				">25</Type>", // EAP-PEAP type
				"MsPeapConnectionPropertiesV1",
				"MsChapV2ConnectionPropertiesV1",
			},
		},
		{
			name: "EAP-TTLS",
			config: &Dot1xConfig{
				Interface: "eth0",
				EAPMethod: "ttls",
				Identity:  "user",
				Password:  "secret",
			},
			contains: []string{
				"<OneXEnabled>true</OneXEnabled>",
				">21</Type>", // EAP-TTLS type
				"EapTtlsConnectionPropertiesV1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := m.generateWindowsDot1xProfileXML(tt.config)

			for _, substr := range tt.contains {
				if !strings.Contains(output, substr) {
					t.Errorf("output missing expected substring %q\nOutput:\n%s", substr, output)
				}
			}
		})
	}
}

func TestDot1xModule_GenerateMacOSProfile(t *testing.T) {
	m := NewDot1xModule()

	tests := []struct {
		name     string
		config   *Dot1xConfig
		contains []string
	}{
		{
			name: "EAP-TLS",
			config: &Dot1xConfig{
				Name:      "corporate",
				Interface: "en0",
				EAPMethod: "tls",
				Identity:  "user@example.com",
			},
			contains: []string{
				"com.apple.firstactiveethernet.managed",
				"<integer>13</integer>", // EAP-TLS type
				"<string>user@example.com</string>",
				"com.keystone.dot1x.corporate",
			},
		},
		{
			name: "EAP-PEAP with password",
			config: &Dot1xConfig{
				Name:      "office",
				Interface: "en0",
				EAPMethod: "peap",
				Identity:  "user",
				Password:  "secret",
			},
			contains: []string{
				"com.apple.firstactiveethernet.managed",
				"<integer>25</integer>", // EAP-PEAP type
				"<key>UserPassword</key>",
				"<string>secret</string>",
			},
		},
		{
			name: "EAP-TTLS",
			config: &Dot1xConfig{
				Name:      "wifi-auth",
				Interface: "en0",
				EAPMethod: "ttls",
				Identity:  "user",
				Password:  "pass",
			},
			contains: []string{
				"<integer>21</integer>", // EAP-TTLS type
				"<key>UserPassword</key>",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := m.generateMacOSDot1xProfile(tt.config)

			for _, substr := range tt.contains {
				if !strings.Contains(output, substr) {
					t.Errorf("output missing expected substring %q\nOutput:\n%s", substr, output)
				}
			}
		})
	}
}

func TestDot1xBackendConstants(t *testing.T) {
	backends := []struct {
		backend  Dot1xBackend
		expected string
	}{
		{D1XUnknown, "unknown"},
		{D1XWpaSupplicant, "wpa_supplicant"},
		{D1XNetworkManager, "networkmanager"},
		{D1XDot3svc, "dot3svc"},
		{D1XProfiles, "profiles"},
	}

	for _, tc := range backends {
		if string(tc.backend) != tc.expected {
			t.Errorf("Dot1xBackend %v: expected %q, got %q", tc.backend, tc.expected, string(tc.backend))
		}
	}
}

func TestDot1xModule_MapPhase2Method(t *testing.T) {
	m := NewDot1xModule()

	tests := []struct {
		input    string
		expected string
	}{
		{"mschapv2", "MSCHAPV2"},
		{"pap", "PAP"},
		{"chap", "CHAP"},
		{"md5", "MD5"},
		{"gtc", "GTC"},
		{"", "MSCHAPV2"},      // default
		{"unknown", "MSCHAPV2"}, // default for unknown
	}

	for _, tt := range tests {
		result := m.mapPhase2Method(tt.input)
		if result != tt.expected {
			t.Errorf("mapPhase2Method(%q): got %q, want %q", tt.input, result, tt.expected)
		}
	}
}
