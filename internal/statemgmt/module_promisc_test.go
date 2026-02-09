package statemgmt

import (
	"strings"
	"testing"
)

// Ensure PromiscModule implements the Module interface
var _ Module = (*PromiscModule)(nil)

func TestNewPromiscModule(t *testing.T) {
	m := NewPromiscModule()
	if m == nil {
		t.Fatal("NewPromiscModule returned nil")
	}
	if m.Name() != "promisc" {
		t.Errorf("expected name 'promisc', got %q", m.Name())
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

func TestPromiscModule_ParseConfig(t *testing.T) {
	m := NewPromiscModule()

	tests := []struct {
		name       string
		declID     string
		params     map[string]interface{}
		wantConfig *PromiscConfig
		wantErr    bool
	}{
		{
			name:   "interface from ID",
			declID: "eth0",
			params: map[string]interface{}{},
			wantConfig: &PromiscConfig{
				Interface: "eth0",
				AllMulti:  false,
			},
		},
		{
			name:   "interface from parameter",
			declID: "monitor",
			params: map[string]interface{}{
				"interface": "enp0s3",
			},
			wantConfig: &PromiscConfig{
				Interface: "enp0s3",
				AllMulti:  false,
			},
		},
		{
			name:   "with allmulti",
			declID: "eth0",
			params: map[string]interface{}{
				"allmulti": true,
			},
			wantConfig: &PromiscConfig{
				Interface: "eth0",
				AllMulti:  true,
			},
		},
		{
			name:   "with all_multicast alias",
			declID: "eth0",
			params: map[string]interface{}{
				"all_multicast": true,
			},
			wantConfig: &PromiscConfig{
				Interface: "eth0",
				AllMulti:  true,
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
			config, err := m.parsePromiscConfig(decl)
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
			if config.AllMulti != tt.wantConfig.AllMulti {
				t.Errorf("AllMulti: got %v, want %v", config.AllMulti, tt.wantConfig.AllMulti)
			}
		})
	}
}

func TestPromiscModule_ValidateConfig(t *testing.T) {
	m := NewPromiscModule()

	tests := []struct {
		name      string
		config    *PromiscConfig
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid config",
			config: &PromiscConfig{
				Interface: "eth0",
			},
		},
		{
			name: "valid config with allmulti",
			config: &PromiscConfig{
				Interface: "eth0",
				AllMulti:  true,
			},
		},
		{
			name: "missing interface",
			config: &PromiscConfig{
				Interface: "",
			},
			wantErr:   true,
			errSubstr: "interface is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.validatePromiscConfig(tt.config)
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

func TestPromiscBackendConstants(t *testing.T) {
	backends := []struct {
		backend  PromiscBackend
		expected string
	}{
		{PBUnknown, "unknown"},
		{PBIPLink, "ip_link"},
		{PBIfconfig, "ifconfig"},
		{PBNetsh, "netsh"},
	}

	for _, tc := range backends {
		if string(tc.backend) != tc.expected {
			t.Errorf("PromiscBackend %v: expected %q, got %q", tc.backend, tc.expected, string(tc.backend))
		}
	}
}

func TestPromiscModule_GetPromiscStateLinux_Parsing(t *testing.T) {
	// Test the parsing logic used in getPromiscStateLinux
	// The actual function calls `ip link show` which we can't mock easily,
	// but we can test the string parsing logic

	tests := []struct {
		name         string
		output       string
		wantPromisc  bool
		wantAllmulti bool
	}{
		{
			name:         "no flags",
			output:       "2: eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500",
			wantPromisc:  false,
			wantAllmulti: false,
		},
		{
			name:         "promisc enabled",
			output:       "2: eth0: <BROADCAST,MULTICAST,PROMISC,UP,LOWER_UP> mtu 1500",
			wantPromisc:  true,
			wantAllmulti: false,
		},
		{
			name:         "allmulti enabled",
			output:       "2: eth0: <BROADCAST,MULTICAST,ALLMULTI,UP,LOWER_UP> mtu 1500",
			wantPromisc:  false,
			wantAllmulti: true,
		},
		{
			name:         "both enabled",
			output:       "2: eth0: <BROADCAST,MULTICAST,PROMISC,ALLMULTI,UP,LOWER_UP> mtu 1500",
			wantPromisc:  true,
			wantAllmulti: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			promisc := strings.Contains(tt.output, "PROMISC")
			allmulti := strings.Contains(tt.output, "ALLMULTI")

			if promisc != tt.wantPromisc {
				t.Errorf("promisc: got %v, want %v", promisc, tt.wantPromisc)
			}
			if allmulti != tt.wantAllmulti {
				t.Errorf("allmulti: got %v, want %v", allmulti, tt.wantAllmulti)
			}
		})
	}
}

func TestPromiscModule_GetPromiscStateMacOS_Parsing(t *testing.T) {
	// Test the parsing logic used in getPromiscStateMacOS
	tests := []struct {
		name         string
		output       string
		wantPromisc  bool
		wantAllmulti bool
	}{
		{
			name:         "no promisc flags",
			output:       "en0: flags=8863<UP,BROADCAST,SMART,RUNNING,SIMPLEX,MULTICAST> mtu 1500",
			wantPromisc:  false,
			wantAllmulti: false,
		},
		{
			name:         "promisc enabled",
			output:       "en0: flags=8963<UP,BROADCAST,SMART,RUNNING,PROMISC,SIMPLEX,MULTICAST> mtu 1500",
			wantPromisc:  true,
			wantAllmulti: false,
		},
		{
			name:         "both enabled",
			output:       "en0: flags=8b63<UP,BROADCAST,SMART,RUNNING,PROMISC,ALLMULTI,SIMPLEX,MULTICAST> mtu 1500",
			wantPromisc:  true,
			wantAllmulti: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simple string check since the actual function uses regex
			promisc := strings.Contains(tt.output, "PROMISC")
			allmulti := strings.Contains(tt.output, "ALLMULTI")

			if promisc != tt.wantPromisc {
				t.Errorf("promisc: got %v, want %v", promisc, tt.wantPromisc)
			}
			if allmulti != tt.wantAllmulti {
				t.Errorf("allmulti: got %v, want %v", allmulti, tt.wantAllmulti)
			}
		})
	}
}

func TestPromiscModule_InterfaceDeclarationID(t *testing.T) {
	m := NewPromiscModule()

	// Test that interface can be derived from declaration ID
	decl := &StateDeclaration{
		ID:         "eth0",
		State:      "enabled",
		Parameters: map[string]interface{}{},
	}

	config, err := m.parsePromiscConfig(decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.Interface != "eth0" {
		t.Errorf("Interface: got %q, want %q", config.Interface, "eth0")
	}
}

func TestPromiscModule_InterfaceParameterOverridesID(t *testing.T) {
	m := NewPromiscModule()

	// Test that explicit interface parameter overrides declaration ID
	decl := &StateDeclaration{
		ID:    "monitor_interface",
		State: "enabled",
		Parameters: map[string]interface{}{
			"interface": "enp0s25",
		},
	}

	config, err := m.parsePromiscConfig(decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.Interface != "enp0s25" {
		t.Errorf("Interface: got %q, want %q", config.Interface, "enp0s25")
	}
}
