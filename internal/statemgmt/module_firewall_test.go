package statemgmt

import (
	"context"
	"runtime"
	"testing"
)

func TestNewFirewallModule(t *testing.T) {
	m := NewFirewallModule()
	if m == nil {
		t.Fatal("NewFirewallModule returned nil")
	}
	if m.Name() != "firewall" {
		t.Errorf("expected name 'firewall', got '%s'", m.Name())
	}

	states := m.ValidStates()
	stateMap := make(map[string]bool)
	for _, s := range states {
		stateMap[s] = true
	}

	if !stateMap["present"] {
		t.Error("module should support 'present' state")
	}
	if !stateMap["absent"] {
		t.Error("module should support 'absent' state")
	}
}

func TestFirewallModule_ParseConfig(t *testing.T) {
	m := NewFirewallModule()

	tests := []struct {
		name         string
		decl         *StateDeclaration
		wantPort     int
		wantProtocol string
		wantAction   FirewallAction
		wantDir      FirewallDirection
		wantErr      bool
	}{
		{
			name: "basic TCP rule",
			decl: &StateDeclaration{
				ID:     "allow-ssh",
				State:  "present",
				Module: "firewall",
				Parameters: map[string]interface{}{
					"port":     22,
					"protocol": "tcp",
					"action":   "accept",
				},
			},
			wantPort:     22,
			wantProtocol: "tcp",
			wantAction:   FAAccept,
			wantDir:      FDInput,
		},
		{
			name: "UDP rule with direction",
			decl: &StateDeclaration{
				ID:     "allow-dns",
				State:  "present",
				Module: "firewall",
				Parameters: map[string]interface{}{
					"port":      53,
					"protocol":  "udp",
					"direction": "output",
					"action":    "accept",
				},
			},
			wantPort:     53,
			wantProtocol: "udp",
			wantAction:   FAAccept,
			wantDir:      FDOutput,
		},
		{
			name: "drop rule",
			decl: &StateDeclaration{
				ID:     "block-telnet",
				State:  "present",
				Module: "firewall",
				Parameters: map[string]interface{}{
					"port":   23,
					"action": "drop",
				},
			},
			wantPort:   23,
			wantAction: FADrop,
		},
		{
			name: "reject rule",
			decl: &StateDeclaration{
				ID:     "reject-ftp",
				State:  "present",
				Module: "firewall",
				Parameters: map[string]interface{}{
					"port":   21,
					"action": "reject",
				},
			},
			wantPort:   21,
			wantAction: FAReject,
		},
		{
			name: "rule with source",
			decl: &StateDeclaration{
				ID:     "allow-from-lan",
				State:  "present",
				Module: "firewall",
				Parameters: map[string]interface{}{
					"port":     80,
					"source":   "192.168.1.0/24",
					"action":   "accept",
				},
			},
			wantPort:   80,
			wantAction: FAAccept,
		},
		{
			name: "invalid action",
			decl: &StateDeclaration{
				ID:     "invalid",
				State:  "present",
				Module: "firewall",
				Parameters: map[string]interface{}{
					"action": "invalid_action",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid protocol",
			decl: &StateDeclaration{
				ID:     "invalid-proto",
				State:  "present",
				Module: "firewall",
				Parameters: map[string]interface{}{
					"protocol": "invalid",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := m.parseFirewallConfig(tt.decl)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseFirewallConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			if config.Port != tt.wantPort {
				t.Errorf("Port = %d, want %d", config.Port, tt.wantPort)
			}
			if tt.wantProtocol != "" && config.Protocol != tt.wantProtocol {
				t.Errorf("Protocol = %s, want %s", config.Protocol, tt.wantProtocol)
			}
			if config.Action != tt.wantAction {
				t.Errorf("Action = %v, want %v", config.Action, tt.wantAction)
			}
			if tt.wantDir != "" && config.Direction != tt.wantDir {
				t.Errorf("Direction = %v, want %v", config.Direction, tt.wantDir)
			}
		})
	}
}

func TestFirewallModule_DetectBackend(t *testing.T) {
	m := NewFirewallModule()

	backend, err := m.detectFirewallBackend()

	switch runtime.GOOS {
	case "darwin":
		if err != nil {
			t.Fatalf("detectFirewallBackend() error = %v on macOS", err)
		}
		if backend != FBPF {
			t.Errorf("expected FBPF on macOS, got %s", backend)
		}
	case "windows":
		if err != nil {
			t.Fatalf("detectFirewallBackend() error = %v on Windows", err)
		}
		if backend != FBNetsh {
			t.Errorf("expected FBNetsh on Windows, got %s", backend)
		}
	case "linux":
		// On Linux, it depends on what's installed
		if err != nil {
			t.Logf("detectFirewallBackend() returned error: %v", err)
		} else {
			validBackends := map[FirewallBackend]bool{
				FBIptables: true, FBNftables: true, FBFirewalld: true,
			}
			if !validBackends[backend] {
				t.Errorf("unexpected Linux backend: %s", backend)
			}
		}
	default:
		if err == nil {
			t.Errorf("expected error for unsupported OS %s", runtime.GOOS)
		}
	}
}

func TestFirewallModule_BuildRuleDescription(t *testing.T) {
	m := NewFirewallModule()

	tests := []struct {
		name     string
		config   *FirewallConfig
		expected string
	}{
		{
			name: "simple port rule",
			config: &FirewallConfig{
				Protocol: "tcp",
				Port:     22,
				Action:   FAAccept,
			},
			expected: "accept tcp port 22",
		},
		{
			name: "port range",
			config: &FirewallConfig{
				Protocol:  "tcp",
				PortRange: "8000:8100",
				Action:    FADrop,
			},
			expected: "drop tcp ports 8000:8100",
		},
		{
			name: "with source",
			config: &FirewallConfig{
				Protocol: "tcp",
				Port:     80,
				Source:   "10.0.0.0/8",
				Action:   FAAccept,
			},
			expected: "accept tcp port 80 from 10.0.0.0/8",
		},
		{
			name: "with destination",
			config: &FirewallConfig{
				Protocol:    "udp",
				Port:        53,
				Destination: "8.8.8.8",
				Action:      FAAccept,
			},
			expected: "accept udp port 53 to 8.8.8.8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := m.buildRuleDescription(tt.config)
			if result != tt.expected {
				t.Errorf("buildRuleDescription() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestFirewallBackendConstants(t *testing.T) {
	backends := []FirewallBackend{
		FBUnknown,
		FBIptables,
		FBNftables,
		FBFirewalld,
		FBPF,
		FBNetsh,
	}

	expected := []string{
		"unknown",
		"iptables",
		"nftables",
		"firewalld",
		"pf",
		"netsh",
	}

	for i, fb := range backends {
		if string(fb) != expected[i] {
			t.Errorf("FirewallBackend constant %d = %s, want %s", i, string(fb), expected[i])
		}
	}
}

func TestFirewallActionConstants(t *testing.T) {
	actions := []FirewallAction{
		FAAccept,
		FADrop,
		FAReject,
	}

	expected := []string{"accept", "drop", "reject"}

	for i, fa := range actions {
		if string(fa) != expected[i] {
			t.Errorf("FirewallAction constant %d = %s, want %s", i, string(fa), expected[i])
		}
	}
}

func TestFirewallDirectionConstants(t *testing.T) {
	directions := []FirewallDirection{
		FDInput,
		FDOutput,
		FDForward,
	}

	expected := []string{"input", "output", "forward"}

	for i, fd := range directions {
		if string(fd) != expected[i] {
			t.Errorf("FirewallDirection constant %d = %s, want %s", i, string(fd), expected[i])
		}
	}
}

func TestFirewallModule_Check_NonexistentRule(t *testing.T) {
	m := NewFirewallModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "nonexistent-rule-12345",
		State:  "present",
		Module: "firewall",
		Parameters: map[string]interface{}{
			"port":     99999,
			"protocol": "tcp",
			"action":   "accept",
		},
	}

	// This test may fail depending on platform capabilities
	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Logf("Check() error = %v (expected on some platforms)", err)
		return
	}

	if result.Present {
		t.Error("expected Present=false for nonexistent rule")
	}
	if result.CurrentState != "absent" {
		t.Errorf("expected CurrentState='absent', got '%s'", result.CurrentState)
	}
}

func TestFirewallModule_Check_AbsentState(t *testing.T) {
	m := NewFirewallModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "nonexistent-rule-12345",
		State:  "absent",
		Module: "firewall",
		Parameters: map[string]interface{}{
			"port":     99999,
			"protocol": "tcp",
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Logf("Check() error = %v (expected on some platforms)", err)
		return
	}

	if result.Present {
		t.Error("expected Present=false for nonexistent rule")
	}
	if !result.Matches {
		t.Error("expected Matches=true when rule doesn't exist and state is 'absent'")
	}
}

func TestNewIptablesModule(t *testing.T) {
	m := NewIptablesModule()
	if m == nil {
		t.Fatal("NewIptablesModule returned nil")
	}
	if m.Name() != "iptables" {
		t.Errorf("expected name 'iptables', got '%s'", m.Name())
	}

	states := m.ValidStates()
	stateMap := make(map[string]bool)
	for _, s := range states {
		stateMap[s] = true
	}

	if !stateMap["present"] {
		t.Error("module should support 'present' state")
	}
	if !stateMap["absent"] {
		t.Error("module should support 'absent' state")
	}
	if !stateMap["flush"] {
		t.Error("module should support 'flush' state")
	}
	if !stateMap["policy"] {
		t.Error("module should support 'policy' state")
	}
}

func TestIptablesModule_ParseConfig(t *testing.T) {
	m := NewIptablesModule()

	tests := []struct {
		name      string
		decl      *StateDeclaration
		wantTable string
		wantChain string
		wantJump  string
		wantErr   bool
	}{
		{
			name: "basic rule",
			decl: &StateDeclaration{
				ID:     "test-rule",
				State:  "present",
				Module: "iptables",
				Parameters: map[string]interface{}{
					"table":     "filter",
					"chain":     "INPUT",
					"protocol":  "tcp",
					"dest_port": "22",
					"jump":      "ACCEPT",
				},
			},
			wantTable: "filter",
			wantChain: "INPUT",
			wantJump:  "ACCEPT",
		},
		{
			name: "nat table rule",
			decl: &StateDeclaration{
				ID:     "nat-rule",
				State:  "present",
				Module: "iptables",
				Parameters: map[string]interface{}{
					"table": "nat",
					"chain": "POSTROUTING",
					"jump":  "MASQUERADE",
				},
			},
			wantTable: "nat",
			wantChain: "POSTROUTING",
			wantJump:  "MASQUERADE",
		},
		{
			name: "invalid table",
			decl: &StateDeclaration{
				ID:     "invalid",
				State:  "present",
				Module: "iptables",
				Parameters: map[string]interface{}{
					"table": "invalid_table",
				},
			},
			wantErr: true,
		},
		{
			name: "policy state valid",
			decl: &StateDeclaration{
				ID:     "policy",
				State:  "policy",
				Module: "iptables",
				Parameters: map[string]interface{}{
					"chain":  "INPUT",
					"policy": "DROP",
				},
			},
			wantChain: "INPUT",
		},
		{
			name: "policy state invalid",
			decl: &StateDeclaration{
				ID:     "policy-invalid",
				State:  "policy",
				Module: "iptables",
				Parameters: map[string]interface{}{
					"chain":  "INPUT",
					"policy": "INVALID",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := m.parseIptablesConfig(tt.decl)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseIptablesConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			if tt.wantTable != "" && config.Table != tt.wantTable {
				t.Errorf("Table = %s, want %s", config.Table, tt.wantTable)
			}
			if tt.wantChain != "" && config.Chain != tt.wantChain {
				t.Errorf("Chain = %s, want %s", config.Chain, tt.wantChain)
			}
			if tt.wantJump != "" && config.Jump != tt.wantJump {
				t.Errorf("Jump = %s, want %s", config.Jump, tt.wantJump)
			}
		})
	}
}

func TestIptablesModule_BuildRuleArgs(t *testing.T) {
	m := NewIptablesModule()

	tests := []struct {
		name     string
		config   *IptablesConfig
		contains []string
	}{
		{
			name: "basic SSH rule",
			config: &IptablesConfig{
				Protocol: "tcp",
				DestPort: "22",
				Jump:     "ACCEPT",
			},
			contains: []string{"-p", "tcp", "--dport", "22", "-j", "ACCEPT"},
		},
		{
			name: "rule with source",
			config: &IptablesConfig{
				Source:   "192.168.1.0/24",
				Protocol: "tcp",
				DestPort: "80",
				Jump:     "ACCEPT",
			},
			contains: []string{"-s", "192.168.1.0/24", "--dport", "80"},
		},
		{
			name: "rule with comment",
			config: &IptablesConfig{
				Protocol: "tcp",
				DestPort: "443",
				Comment:  "Allow HTTPS",
				Jump:     "ACCEPT",
			},
			contains: []string{"-m", "comment", "--comment", "Allow HTTPS"},
		},
		{
			name: "rule with state",
			config: &IptablesConfig{
				State: "ESTABLISHED,RELATED",
				Jump:  "ACCEPT",
			},
			contains: []string{"-m", "state", "--state", "ESTABLISHED,RELATED"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := m.buildRuleArgs(tt.config)
			argsStr := ""
			for _, a := range args {
				argsStr += a + " "
			}

			for _, want := range tt.contains {
				found := false
				for _, arg := range args {
					if arg == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("buildRuleArgs() missing %s, got: %v", want, args)
				}
			}
		})
	}
}

func TestNewNftablesModule(t *testing.T) {
	m := NewNftablesModule()
	if m == nil {
		t.Fatal("NewNftablesModule returned nil")
	}
	if m.Name() != "nftables" {
		t.Errorf("expected name 'nftables', got '%s'", m.Name())
	}
}

func TestNftablesModule_ParseConfig(t *testing.T) {
	m := NewNftablesModule()

	tests := []struct {
		name       string
		decl       *StateDeclaration
		wantFamily string
		wantTable  string
		wantChain  string
		wantErr    bool
	}{
		{
			name: "basic rule",
			decl: &StateDeclaration{
				ID:     "test-rule",
				State:  "present",
				Module: "nftables",
				Parameters: map[string]interface{}{
					"family":    "ip",
					"table":     "filter",
					"chain":     "input",
					"dest_port": "22",
					"action":    "accept",
				},
			},
			wantFamily: "ip",
			wantTable:  "filter",
			wantChain:  "input",
		},
		{
			name: "inet family",
			decl: &StateDeclaration{
				ID:     "inet-rule",
				State:  "present",
				Module: "nftables",
				Parameters: map[string]interface{}{
					"family": "inet",
					"table":  "myfilter",
					"chain":  "mychain",
				},
			},
			wantFamily: "inet",
			wantTable:  "myfilter",
			wantChain:  "mychain",
		},
		{
			name: "invalid family",
			decl: &StateDeclaration{
				ID:     "invalid",
				State:  "present",
				Module: "nftables",
				Parameters: map[string]interface{}{
					"family": "invalid_family",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := m.parseNftablesConfig(tt.decl)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseNftablesConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			if config.Family != tt.wantFamily {
				t.Errorf("Family = %s, want %s", config.Family, tt.wantFamily)
			}
			if config.Table != tt.wantTable {
				t.Errorf("Table = %s, want %s", config.Table, tt.wantTable)
			}
			if config.Chain != tt.wantChain {
				t.Errorf("Chain = %s, want %s", config.Chain, tt.wantChain)
			}
		})
	}
}

func TestNftablesModule_BuildRule(t *testing.T) {
	m := NewNftablesModule()

	tests := []struct {
		name     string
		config   *NftablesConfig
		contains []string
	}{
		{
			name: "basic rule",
			config: &NftablesConfig{
				Protocol: "tcp",
				DestPort: "22",
				Action:   "accept",
			},
			contains: []string{"tcp", "dport", "22", "accept"},
		},
		{
			name: "rule with source",
			config: &NftablesConfig{
				Source: "192.168.1.0/24",
				Action: "accept",
			},
			contains: []string{"ip", "saddr", "192.168.1.0/24", "accept"},
		},
		{
			name: "rule with counter",
			config: &NftablesConfig{
				Protocol: "tcp",
				Counter:  true,
				Action:   "drop",
			},
			contains: []string{"counter", "drop"},
		},
		{
			name: "rule with comment",
			config: &NftablesConfig{
				Comment: "Allow SSH",
				Action:  "accept",
			},
			contains: []string{"comment", "\"Allow SSH\"", "accept"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := m.buildRule(tt.config)

			for _, want := range tt.contains {
				if !containsSubstring(rule, want) {
					t.Errorf("buildRule() missing %s, got: %s", want, rule)
				}
			}
		})
	}
}

func TestNewFirewalldModule(t *testing.T) {
	m := NewFirewalldModule()
	if m == nil {
		t.Fatal("NewFirewalldModule returned nil")
	}
	if m.Name() != "firewalld" {
		t.Errorf("expected name 'firewalld', got '%s'", m.Name())
	}
}

func TestFirewalldModule_ParseConfig(t *testing.T) {
	m := NewFirewalldModule()

	tests := []struct {
		name      string
		decl      *StateDeclaration
		wantZone  string
		wantPort  int
		wantProto string
		wantErr   bool
	}{
		{
			name: "service rule",
			decl: &StateDeclaration{
				ID:     "allow-ssh",
				State:  "present",
				Module: "firewalld",
				Parameters: map[string]interface{}{
					"zone":    "public",
					"service": "ssh",
				},
			},
			wantZone: "public",
		},
		{
			name: "port rule",
			decl: &StateDeclaration{
				ID:     "allow-custom",
				State:  "present",
				Module: "firewalld",
				Parameters: map[string]interface{}{
					"zone":     "internal",
					"port":     8080,
					"protocol": "tcp",
				},
			},
			wantZone:  "internal",
			wantPort:  8080,
			wantProto: "tcp",
		},
		{
			name: "invalid protocol",
			decl: &StateDeclaration{
				ID:     "invalid",
				State:  "present",
				Module: "firewalld",
				Parameters: map[string]interface{}{
					"port":     80,
					"protocol": "invalid",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := m.parseFirewalldConfig(tt.decl)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseFirewalldConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			if config.Zone != tt.wantZone {
				t.Errorf("Zone = %s, want %s", config.Zone, tt.wantZone)
			}
			if tt.wantPort > 0 && config.Port != tt.wantPort {
				t.Errorf("Port = %d, want %d", config.Port, tt.wantPort)
			}
			if tt.wantProto != "" && config.Protocol != tt.wantProto {
				t.Errorf("Protocol = %s, want %s", config.Protocol, tt.wantProto)
			}
		})
	}
}

func TestFirewalldModule_BuildRichRule(t *testing.T) {
	m := NewFirewalldModule()

	tests := []struct {
		name     string
		config   *FirewalldConfig
		contains []string
	}{
		{
			name: "basic port rule",
			config: &FirewalldConfig{
				Family:   "ipv4",
				DestPort: 22,
				Protocol: "tcp",
				Action:   "accept",
			},
			contains: []string{"rule", "family=\"ipv4\"", "port port=\"22\" protocol=\"tcp\"", "accept"},
		},
		{
			name: "rule with source",
			config: &FirewalldConfig{
				Family:     "ipv4",
				SourceAddr: "192.168.1.0/24",
				DestPort:   80,
				Protocol:   "tcp",
				Action:     "accept",
			},
			contains: []string{"source address=\"192.168.1.0/24\"", "accept"},
		},
		{
			name: "rule with logging",
			config: &FirewalldConfig{
				DestPort:  22,
				Protocol:  "tcp",
				LogPrefix: "SSH-attempt",
				LogLevel:  "info",
				Action:    "reject",
			},
			contains: []string{"log", "prefix=\"SSH-attempt\"", "level=\"info\"", "reject"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := m.buildRichRule(tt.config)

			for _, want := range tt.contains {
				if !containsSubstring(rule, want) {
					t.Errorf("buildRichRule() missing %s, got: %s", want, rule)
				}
			}
		})
	}
}

// Helper function
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 &&
			findSubstringInFirewall(s, substr)))
}

func findSubstringInFirewall(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
