package statemgmt

import (
	"strings"
	"testing"
)

// Ensure LinkModule implements the Module interface
var _ Module = (*LinkModule)(nil)

func TestNewLinkModule(t *testing.T) {
	m := NewLinkModule()
	if m == nil {
		t.Fatal("NewLinkModule returned nil")
	}
	if m.Name() != "link" {
		t.Errorf("expected name 'link', got %q", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"configured", "default"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
	for i, s := range expected {
		if states[i] != s {
			t.Errorf("expected state %q at position %d, got %q", s, i, states[i])
		}
	}
}

func TestLinkModule_ParseConfig(t *testing.T) {
	m := NewLinkModule()

	tests := []struct {
		name       string
		declID     string
		params     map[string]interface{}
		wantConfig *LinkConfig
		wantErr    bool
	}{
		{
			name:   "interface from ID",
			declID: "eth0",
			params: map[string]interface{}{
				"speed":  1000,
				"duplex": "full",
			},
			wantConfig: &LinkConfig{
				Interface: "eth0",
				Speed:     1000,
				Duplex:    "full",
			},
		},
		{
			name:   "interface from parameter overrides ID",
			declID: "default",
			params: map[string]interface{}{
				"interface": "enp0s3",
				"speed":     100,
				"duplex":    "half",
			},
			wantConfig: &LinkConfig{
				Interface: "enp0s3",
				Speed:     100,
				Duplex:    "half",
			},
		},
		{
			name:   "auto-negotiation on",
			declID: "eth0",
			params: map[string]interface{}{
				"autoneg": true,
			},
			wantConfig: &LinkConfig{
				Interface: "eth0",
				Autoneg:   boolPtr(true),
			},
		},
		{
			name:   "auto-negotiation off with speed",
			declID: "eth0",
			params: map[string]interface{}{
				"speed":            1000,
				"duplex":           "full",
				"auto_negotiation": false,
			},
			wantConfig: &LinkConfig{
				Interface: "eth0",
				Speed:     1000,
				Duplex:    "full",
				Autoneg:   boolPtr(false),
			},
		},
		{
			name:   "MTU configuration",
			declID: "eth0",
			params: map[string]interface{}{
				"mtu": 9000,
			},
			wantConfig: &LinkConfig{
				Interface: "eth0",
				MTU:       9000,
			},
		},
		{
			name:   "Wake-on-LAN magic",
			declID: "eth0",
			params: map[string]interface{}{
				"wol": "magic",
			},
			wantConfig: &LinkConfig{
				Interface: "eth0",
				WOL:       "magic",
			},
		},
		{
			name:   "wake_on_lan alias",
			declID: "eth0",
			params: map[string]interface{}{
				"wake_on_lan": "disabled",
			},
			wantConfig: &LinkConfig{
				Interface: "eth0",
				WOL:       "disabled",
			},
		},
		{
			name:   "speed as float64 (YAML parsing)",
			declID: "eth0",
			params: map[string]interface{}{
				"speed": float64(10000),
				"mtu":   float64(1500),
			},
			wantConfig: &LinkConfig{
				Interface: "eth0",
				Speed:     10000,
				MTU:       1500,
			},
		},
		{
			name:    "missing interface",
			declID:  "",
			params:  map[string]interface{}{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decl := &StateDeclaration{
				ID:         tt.declID,
				Parameters: tt.params,
			}
			config, err := m.parseLinkConfig(decl)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if config.Interface != tt.wantConfig.Interface {
				t.Errorf("Interface: got %q, want %q", config.Interface, tt.wantConfig.Interface)
			}
			if config.Speed != tt.wantConfig.Speed {
				t.Errorf("Speed: got %d, want %d", config.Speed, tt.wantConfig.Speed)
			}
			if config.Duplex != tt.wantConfig.Duplex {
				t.Errorf("Duplex: got %q, want %q", config.Duplex, tt.wantConfig.Duplex)
			}
			if config.MTU != tt.wantConfig.MTU {
				t.Errorf("MTU: got %d, want %d", config.MTU, tt.wantConfig.MTU)
			}
			if config.WOL != tt.wantConfig.WOL {
				t.Errorf("WOL: got %q, want %q", config.WOL, tt.wantConfig.WOL)
			}

			// Check autoneg pointer
			if tt.wantConfig.Autoneg != nil {
				if config.Autoneg == nil {
					t.Error("Autoneg: expected non-nil, got nil")
				} else if *config.Autoneg != *tt.wantConfig.Autoneg {
					t.Errorf("Autoneg: got %v, want %v", *config.Autoneg, *tt.wantConfig.Autoneg)
				}
			} else if config.Autoneg != nil {
				t.Errorf("Autoneg: expected nil, got %v", *config.Autoneg)
			}
		})
	}
}

func TestLinkModule_ValidateConfig(t *testing.T) {
	m := NewLinkModule()

	tests := []struct {
		name      string
		config    *LinkConfig
		state     string
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid 1000 Mbps full duplex",
			config: &LinkConfig{
				Interface: "eth0",
				Speed:     1000,
				Duplex:    "full",
			},
			state: "configured",
		},
		{
			name: "valid 100 Mbps half duplex",
			config: &LinkConfig{
				Interface: "eth0",
				Speed:     100,
				Duplex:    "half",
			},
			state: "configured",
		},
		{
			name: "valid 10G speed",
			config: &LinkConfig{
				Interface: "eth0",
				Speed:     10000,
				Duplex:    "full",
			},
			state: "configured",
		},
		{
			name: "valid MTU jumbo frame",
			config: &LinkConfig{
				Interface: "eth0",
				MTU:       9000,
			},
			state: "configured",
		},
		{
			name: "valid WOL magic",
			config: &LinkConfig{
				Interface: "eth0",
				WOL:       "magic",
			},
			state: "configured",
		},
		{
			name: "default state only needs interface",
			config: &LinkConfig{
				Interface: "eth0",
			},
			state: "default",
		},
		{
			name: "missing interface",
			config: &LinkConfig{
				Speed: 1000,
			},
			state:     "configured",
			wantErr:   true,
			errSubstr: "interface is required",
		},
		{
			name: "invalid speed",
			config: &LinkConfig{
				Interface: "eth0",
				Speed:     999,
			},
			state:     "configured",
			wantErr:   true,
			errSubstr: "invalid speed",
		},
		{
			name: "invalid duplex",
			config: &LinkConfig{
				Interface: "eth0",
				Duplex:    "quarter",
			},
			state:     "configured",
			wantErr:   true,
			errSubstr: "invalid duplex",
		},
		{
			name: "MTU too small",
			config: &LinkConfig{
				Interface: "eth0",
				MTU:       50,
			},
			state:     "configured",
			wantErr:   true,
			errSubstr: "invalid MTU",
		},
		{
			name: "MTU too large",
			config: &LinkConfig{
				Interface: "eth0",
				MTU:       70000,
			},
			state:     "configured",
			wantErr:   true,
			errSubstr: "invalid MTU",
		},
		{
			name: "invalid WOL mode",
			config: &LinkConfig{
				Interface: "eth0",
				WOL:       "invalid",
			},
			state:     "configured",
			wantErr:   true,
			errSubstr: "invalid WOL mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.validateLinkConfig(tt.config, tt.state)
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

func TestValidLinkSpeeds(t *testing.T) {
	validSpeeds := []int{10, 100, 1000, 2500, 5000, 10000, 25000, 40000, 100000}
	invalidSpeeds := []int{0, 50, 500, 999, 1001, 9999, 15000}

	for _, speed := range validSpeeds {
		if !validLinkSpeeds[speed] {
			t.Errorf("expected %d to be a valid link speed", speed)
		}
	}

	for _, speed := range invalidSpeeds {
		if validLinkSpeeds[speed] {
			t.Errorf("expected %d to be an invalid link speed", speed)
		}
	}
}

func TestValidDuplexModes(t *testing.T) {
	validModes := []string{"full", "half", ""}
	invalidModes := []string{"quarter", "double", "auto"}

	for _, mode := range validModes {
		if !validDuplexModes[mode] {
			t.Errorf("expected %q to be a valid duplex mode", mode)
		}
	}

	for _, mode := range invalidModes {
		if validDuplexModes[mode] {
			t.Errorf("expected %q to be an invalid duplex mode", mode)
		}
	}
}

func TestValidWOLModes(t *testing.T) {
	validModes := []string{"disabled", "magic", "unicast", "multicast", "broadcast", "arp", ""}
	invalidModes := []string{"on", "off", "enabled", "all"}

	for _, mode := range validModes {
		if !validWOLModes[mode] {
			t.Errorf("expected %q to be a valid WOL mode", mode)
		}
	}

	for _, mode := range invalidModes {
		if validWOLModes[mode] {
			t.Errorf("expected %q to be an invalid WOL mode", mode)
		}
	}
}

func TestLinkModule_WOLModeToEthtool(t *testing.T) {
	m := NewLinkModule()

	tests := []struct {
		mode     string
		expected string
	}{
		{"disabled", "d"},
		{"magic", "g"},
		{"unicast", "u"},
		{"multicast", "m"},
		{"broadcast", "b"},
		{"arp", "a"},
		{"unknown", "d"}, // default
	}

	for _, tt := range tests {
		result := m.wolModeToEthtool(tt.mode)
		if result != tt.expected {
			t.Errorf("wolModeToEthtool(%q): got %q, want %q", tt.mode, result, tt.expected)
		}
	}
}

func TestLinkModule_BuildMacOSMediaString(t *testing.T) {
	m := NewLinkModule()

	tests := []struct {
		name     string
		config   *LinkConfig
		expected string
	}{
		{
			name: "1000 Mbps full duplex",
			config: &LinkConfig{
				Speed:  1000,
				Duplex: "full",
			},
			expected: "1000baseT mediaopt full-duplex",
		},
		{
			name: "100 Mbps half duplex",
			config: &LinkConfig{
				Speed:  100,
				Duplex: "half",
			},
			expected: "100baseT mediaopt half-duplex",
		},
		{
			name: "speed only",
			config: &LinkConfig{
				Speed: 1000,
			},
			expected: "1000baseT",
		},
		{
			name: "no speed",
			config: &LinkConfig{
				Duplex: "full",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := m.buildMacOSMediaString(tt.config)
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestLinkModule_BuildWindowsSpeedDuplex(t *testing.T) {
	m := NewLinkModule()

	tests := []struct {
		name     string
		config   *LinkConfig
		expected string
	}{
		{
			name: "auto-negotiation",
			config: &LinkConfig{
				Autoneg: boolPtr(true),
			},
			expected: "0",
		},
		{
			name: "10 Mbps half",
			config: &LinkConfig{
				Speed:  10,
				Duplex: "half",
			},
			expected: "1",
		},
		{
			name: "10 Mbps full",
			config: &LinkConfig{
				Speed:  10,
				Duplex: "full",
			},
			expected: "2",
		},
		{
			name: "100 Mbps half",
			config: &LinkConfig{
				Speed:  100,
				Duplex: "half",
			},
			expected: "3",
		},
		{
			name: "100 Mbps full",
			config: &LinkConfig{
				Speed:  100,
				Duplex: "full",
			},
			expected: "4",
		},
		{
			name: "1000 Mbps full",
			config: &LinkConfig{
				Speed:  1000,
				Duplex: "full",
			},
			expected: "5",
		},
		{
			name: "1000 Mbps default duplex",
			config: &LinkConfig{
				Speed: 1000,
			},
			expected: "5",
		},
		{
			name: "unknown speed defaults to auto",
			config: &LinkConfig{
				Speed: 2500,
			},
			expected: "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := m.buildWindowsSpeedDuplex(tt.config)
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestLinkBackendConstants(t *testing.T) {
	backends := []struct {
		backend  LinkBackend
		expected string
	}{
		{LBUnknown, "unknown"},
		{LBEthtool, "ethtool"},
		{LBNetworkSetup, "networksetup"},
		{LBNetsh, "netsh"},
	}

	for _, tc := range backends {
		if string(tc.backend) != tc.expected {
			t.Errorf("LinkBackend %v: expected %q, got %q", tc.backend, tc.expected, string(tc.backend))
		}
	}
}
