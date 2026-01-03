package statemgmt

import (
	"context"
	"runtime"
	"testing"
)

func TestNewNetworkModule(t *testing.T) {
	m := NewNetworkModule()
	if m == nil {
		t.Fatal("NewNetworkModule returned nil")
	}
	if m.Name() != "network" {
		t.Errorf("expected name 'network', got '%s'", m.Name())
	}

	states := m.ValidStates()
	stateMap := make(map[string]bool)
	for _, s := range states {
		stateMap[s] = true
	}

	if !stateMap["configured"] {
		t.Error("module should support 'configured' state")
	}
	if !stateMap["absent"] {
		t.Error("module should support 'absent' state")
	}
	if !stateMap["dhcp"] {
		t.Error("module should support 'dhcp' state")
	}
}

func TestNetworkModule_ParseConfig(t *testing.T) {
	m := NewNetworkModule()

	tests := []struct {
		name       string
		decl       *StateDeclaration
		wantIface  string
		wantAddr   string
		wantGW     string
		wantDNS    []string
		wantMTU    int
		wantDHCP   bool
		wantErr    bool
	}{
		{
			name: "basic static config",
			decl: &StateDeclaration{
				ID:     "eth0",
				State:  "configured",
				Module: "network",
				Parameters: map[string]interface{}{
					"address": "192.168.1.100/24",
					"gateway": "192.168.1.1",
				},
			},
			wantIface: "eth0",
			wantAddr:  "192.168.1.100/24",
			wantGW:    "192.168.1.1",
		},
		{
			name: "config with DNS",
			decl: &StateDeclaration{
				ID:     "eth0",
				State:  "configured",
				Module: "network",
				Parameters: map[string]interface{}{
					"address": "10.0.0.5/8",
					"gateway": "10.0.0.1",
					"dns":     "8.8.8.8,8.8.4.4",
				},
			},
			wantIface: "eth0",
			wantAddr:  "10.0.0.5/8",
			wantGW:    "10.0.0.1",
			wantDNS:   []string{"8.8.8.8", "8.8.4.4"},
		},
		{
			name: "config with DNS array",
			decl: &StateDeclaration{
				ID:     "eth0",
				State:  "configured",
				Module: "network",
				Parameters: map[string]interface{}{
					"address": "10.0.0.5/8",
					"dns":     []string{"1.1.1.1", "1.0.0.1"},
				},
			},
			wantIface: "eth0",
			wantAddr:  "10.0.0.5/8",
			wantDNS:   []string{"1.1.1.1", "1.0.0.1"},
		},
		{
			name: "DHCP config",
			decl: &StateDeclaration{
				ID:     "eth0",
				State:  "dhcp",
				Module: "network",
				Parameters: map[string]interface{}{
					"dhcp": true,
				},
			},
			wantIface: "eth0",
			wantDHCP:  true,
		},
		{
			name: "config with MTU",
			decl: &StateDeclaration{
				ID:     "eth0",
				State:  "configured",
				Module: "network",
				Parameters: map[string]interface{}{
					"address": "192.168.1.100/24",
					"mtu":     9000,
				},
			},
			wantIface: "eth0",
			wantAddr:  "192.168.1.100/24",
			wantMTU:   9000,
		},
		{
			name: "explicit interface name",
			decl: &StateDeclaration{
				ID:     "primary_network",
				State:  "configured",
				Module: "network",
				Parameters: map[string]interface{}{
					"interface": "enp0s3",
					"address":   "192.168.1.100/24",
				},
			},
			wantIface: "enp0s3",
			wantAddr:  "192.168.1.100/24",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := m.parseNetworkConfig(tt.decl)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseNetworkConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}

			if config.Interface != tt.wantIface {
				t.Errorf("Interface = %s, want %s", config.Interface, tt.wantIface)
			}
			if config.Address != tt.wantAddr {
				t.Errorf("Address = %s, want %s", config.Address, tt.wantAddr)
			}
			if config.Gateway != tt.wantGW {
				t.Errorf("Gateway = %s, want %s", config.Gateway, tt.wantGW)
			}
			if config.MTU != tt.wantMTU {
				t.Errorf("MTU = %d, want %d", config.MTU, tt.wantMTU)
			}
			if config.DHCP != tt.wantDHCP {
				t.Errorf("DHCP = %v, want %v", config.DHCP, tt.wantDHCP)
			}
			if len(tt.wantDNS) > 0 {
				if len(config.DNS) != len(tt.wantDNS) {
					t.Errorf("DNS len = %d, want %d", len(config.DNS), len(tt.wantDNS))
				} else {
					for i, dns := range tt.wantDNS {
						if config.DNS[i] != dns {
							t.Errorf("DNS[%d] = %s, want %s", i, config.DNS[i], dns)
						}
					}
				}
			}
		})
	}
}

func TestNetworkModule_DetectNetworkManager(t *testing.T) {
	m := NewNetworkModule()

	nm, err := m.detectNetworkManager()
	if err != nil {
		// Not an error if no network manager is available in test environment
		t.Logf("No network manager detected: %v", err)
		return
	}

	switch runtime.GOOS {
	case "darwin":
		if nm != NMNetworkSetup {
			t.Errorf("expected NMNetworkSetup on macOS, got %s", nm)
		}
	case "windows":
		if nm != NMNetsh {
			t.Errorf("expected NMNetsh on Windows, got %s", nm)
		}
	case "linux":
		// Could be any of the Linux network managers
		if nm != NMNetworkManager && nm != NMNetplan && nm != NMSystemdNetworkd && nm != NMIfupdown {
			t.Errorf("unexpected network manager on Linux: %s", nm)
		}
	}
}

func TestNetworkModule_NormalizeAddress(t *testing.T) {
	tests := []struct {
		addr    string
		netmask string
		want    string
	}{
		{"192.168.1.100", "255.255.255.0", "192.168.1.100/24"},
		{"10.0.0.1", "255.0.0.0", "10.0.0.1/8"},
		{"192.168.1.100/24", "", "192.168.1.100/24"},
		{"", "", ""},
		{"172.16.0.1", "255.255.0.0", "172.16.0.1/16"},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			got := normalizeAddress(tt.addr, tt.netmask)
			if got != tt.want {
				t.Errorf("normalizeAddress(%s, %s) = %s, want %s", tt.addr, tt.netmask, got, tt.want)
			}
		})
	}
}

func TestNetworkModule_CIDRToNetmask(t *testing.T) {
	tests := []struct {
		prefix string
		want   string
	}{
		{"24", "255.255.255.0"},
		{"8", "255.0.0.0"},
		{"16", "255.255.0.0"},
		{"32", "255.255.255.255"},
		{"0", "0.0.0.0"},
		{"28", "255.255.255.240"},
	}

	for _, tt := range tests {
		t.Run(tt.prefix, func(t *testing.T) {
			got := cidrToNetmask(tt.prefix)
			if got != tt.want {
				t.Errorf("cidrToNetmask(%s) = %s, want %s", tt.prefix, got, tt.want)
			}
		})
	}
}

func TestNetworkModule_StringSlicesEqual(t *testing.T) {
	// Note: stringSlicesEqual uses a map, so order doesn't matter
	tests := []struct {
		name string
		a    []string
		b    []string
		want bool
	}{
		{"both empty", nil, nil, true},
		{"both same", []string{"a", "b"}, []string{"a", "b"}, true},
		{"same elements different order", []string{"a", "b"}, []string{"b", "a"}, true}, // order ignored
		{"different length", []string{"a"}, []string{"a", "b"}, false},
		{"different content", []string{"a"}, []string{"b"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stringSlicesEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("stringSlicesEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNetworkModule_Check_NonexistentInterface(t *testing.T) {
	m := NewNetworkModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "nonexistent_interface_12345",
		State:  "configured",
		Module: "network",
		Parameters: map[string]interface{}{
			"address": "192.168.1.100/24",
		},
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if result.Present {
		t.Error("expected Present=false for nonexistent interface")
	}
	if result.CurrentState != "absent" {
		t.Errorf("expected CurrentState='absent', got '%s'", result.CurrentState)
	}
}

func TestNetworkModule_Check_AbsentState(t *testing.T) {
	m := NewNetworkModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "nonexistent_interface_12345",
		State:  "absent",
		Module: "network",
	}

	result, err := m.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if result.Present {
		t.Error("expected Present=false for nonexistent interface")
	}
	if !result.Matches {
		t.Error("expected Matches=true when interface is absent and state is 'absent'")
	}
}

func TestNetworkModule_Test(t *testing.T) {
	m := NewNetworkModule()
	ctx := context.Background()

	// Test with nonexistent interface wanting absent state
	decl := &StateDeclaration{
		ID:     "nonexistent_interface_12345",
		State:  "absent",
		Module: "network",
	}

	matches, err := m.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test() error = %v", err)
	}

	if !matches {
		t.Error("expected Test() to return true for absent interface with 'absent' state")
	}
}

func TestNetworkManagerConstants(t *testing.T) {
	// Verify network manager constants are defined correctly
	managers := []NetworkManager{
		NMUnknown,
		NMNetworkManager,
		NMNetplan,
		NMIfupdown,
		NMSystemdNetworkd,
		NMNetworkSetup,
		NMNetsh,
	}

	expected := []string{
		"unknown",
		"networkmanager",
		"netplan",
		"ifupdown",
		"systemd-networkd",
		"networksetup",
		"netsh",
	}

	for i, nm := range managers {
		if string(nm) != expected[i] {
			t.Errorf("NetworkManager constant %d = %s, want %s", i, string(nm), expected[i])
		}
	}
}
