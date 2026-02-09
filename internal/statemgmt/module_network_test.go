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
		name      string
		decl      *StateDeclaration
		wantIface string
		wantAddr  string
		wantGW    string
		wantDNS   []string
		wantMTU   int
		wantDHCP  bool
		wantErr   bool
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
			if tt.wantAddr != "" {
				if len(config.Addresses) == 0 || config.Addresses[0] != tt.wantAddr {
					gotAddr := ""
					if len(config.Addresses) > 0 {
						gotAddr = config.Addresses[0]
					}
					t.Errorf("Address = %s, want %s", gotAddr, tt.wantAddr)
				}
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
	ctx := context.Background()

	nm, err := m.detectNetworkManager(ctx)
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

func TestNetworkModule_UpdateIfupdownConfig_NewInterface(t *testing.T) {
	m := NewNetworkModule()

	config := &NetworkConfig{
		Interface: "eth0",
		Addresses: []string{"192.168.1.100"},
		Gateway:   "192.168.1.1",
		DNS:       []string{"8.8.8.8", "8.8.4.4"},
	}

	// Empty file - should add new interface
	result := m.updateIfupdownConfig("", config, "192.168.1.100", "255.255.255.0", false)

	if !networkTestContains(result, "auto eth0") {
		t.Error("expected 'auto eth0' in result")
	}
	if !networkTestContains(result, "iface eth0 inet static") {
		t.Error("expected 'iface eth0 inet static' in result")
	}
	if !networkTestContains(result, "address 192.168.1.100") {
		t.Error("expected 'address 192.168.1.100' in result")
	}
	if !networkTestContains(result, "netmask 255.255.255.0") {
		t.Error("expected 'netmask 255.255.255.0' in result")
	}
	if !networkTestContains(result, "gateway 192.168.1.1") {
		t.Error("expected 'gateway 192.168.1.1' in result")
	}
	if !networkTestContains(result, "dns-nameservers 8.8.8.8 8.8.4.4") {
		t.Error("expected 'dns-nameservers 8.8.8.8 8.8.4.4' in result")
	}
}

func TestNetworkModule_UpdateIfupdownConfig_DHCP(t *testing.T) {
	m := NewNetworkModule()

	config := &NetworkConfig{
		Interface: "eth0",
	}

	result := m.updateIfupdownConfig("", config, "", "", true)

	if !networkTestContains(result, "auto eth0") {
		t.Error("expected 'auto eth0' in result")
	}
	if !networkTestContains(result, "iface eth0 inet dhcp") {
		t.Error("expected 'iface eth0 inet dhcp' in result")
	}
}

func TestNetworkModule_UpdateIfupdownConfig_ReplaceExisting(t *testing.T) {
	m := NewNetworkModule()

	existingContent := `# Interfaces file
auto lo
iface lo inet loopback

auto eth0
iface eth0 inet static
    address 10.0.0.5
    netmask 255.0.0.0
    gateway 10.0.0.1

auto eth1
iface eth1 inet dhcp
`

	config := &NetworkConfig{
		Interface: "eth0",
		Addresses: []string{"192.168.1.100"},
		Gateway:   "192.168.1.1",
	}

	result := m.updateIfupdownConfig(existingContent, config, "192.168.1.100", "255.255.255.0", false)

	// Should preserve lo and eth1
	if !networkTestContains(result, "auto lo") {
		t.Error("expected 'auto lo' to be preserved")
	}
	if !networkTestContains(result, "iface lo inet loopback") {
		t.Error("expected 'iface lo inet loopback' to be preserved")
	}
	if !networkTestContains(result, "auto eth1") {
		t.Error("expected 'auto eth1' to be preserved")
	}
	if !networkTestContains(result, "iface eth1 inet dhcp") {
		t.Error("expected 'iface eth1 inet dhcp' to be preserved")
	}

	// Should have updated eth0 config
	if !networkTestContains(result, "iface eth0 inet static") {
		t.Error("expected 'iface eth0 inet static' in result")
	}
	if !networkTestContains(result, "address 192.168.1.100") {
		t.Error("expected new 'address 192.168.1.100' in result")
	}
	if !networkTestContains(result, "gateway 192.168.1.1") {
		t.Error("expected new 'gateway 192.168.1.1' in result")
	}

	// Should NOT have old config
	if networkTestContains(result, "address 10.0.0.5") {
		t.Error("old address should be replaced")
	}
	if networkTestContains(result, "gateway 10.0.0.1") {
		t.Error("old gateway should be replaced")
	}
}

func TestNetworkModule_UpdateIfupdownConfig_WithMTUAndMetric(t *testing.T) {
	m := NewNetworkModule()

	config := &NetworkConfig{
		Interface:     "eth0",
		Addresses:     []string{"192.168.1.100"},
		MTU:           9000,
		Metric:        100,
		SearchDomains: []string{"example.com", "local"},
	}

	result := m.updateIfupdownConfig("", config, "192.168.1.100", "255.255.255.0", false)

	if !networkTestContains(result, "mtu 9000") {
		t.Error("expected 'mtu 9000' in result")
	}
	if !networkTestContains(result, "metric 100") {
		t.Error("expected 'metric 100' in result")
	}
	if !networkTestContains(result, "dns-search example.com local") {
		t.Error("expected 'dns-search example.com local' in result")
	}
}

// helper function for string contains check
func networkTestContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestNetworkModule_ParseIPv6Config(t *testing.T) {
	m := NewNetworkModule()

	tests := []struct {
		name           string
		decl           *StateDeclaration
		wantAddr6      string
		wantGW6        string
		wantDHCP6      bool
		wantIPv6       bool
		wantIPv6Priv   bool
		wantAcceptRA   *bool
		wantSearchDoms []string
	}{
		{
			name: "basic IPv6 static config",
			decl: &StateDeclaration{
				ID:     "eth0",
				State:  "configured",
				Module: "network",
				Parameters: map[string]interface{}{
					"address":      "192.168.1.100/24",
					"address6":     "2001:db8::1/64",
					"gateway6":     "2001:db8::ffff",
					"ipv6_enabled": true,
				},
			},
			wantAddr6: "2001:db8::1/64",
			wantGW6:   "2001:db8::ffff",
			wantIPv6:  true,
		},
		{
			name: "DHCPv6 config",
			decl: &StateDeclaration{
				ID:     "eth0",
				State:  "dhcp",
				Module: "network",
				Parameters: map[string]interface{}{
					"dhcp":  true,
					"dhcp6": true,
				},
			},
			wantDHCP6: true,
			wantIPv6:  true, // Auto-enabled when dhcp6 is true
		},
		{
			name: "IPv6 with privacy extensions",
			decl: &StateDeclaration{
				ID:     "eth0",
				State:  "configured",
				Module: "network",
				Parameters: map[string]interface{}{
					"address6":     "2001:db8::1/64",
					"ipv6_privacy": true,
					"ipv6_enabled": true,
				},
			},
			wantAddr6:    "2001:db8::1/64",
			wantIPv6:     true,
			wantIPv6Priv: true,
		},
		{
			name: "IPv6 with accept_ra disabled",
			decl: &StateDeclaration{
				ID:     "eth0",
				State:  "configured",
				Module: "network",
				Parameters: map[string]interface{}{
					"address6":     "2001:db8::1/64",
					"accept_ra":    false,
					"ipv6_enabled": true,
				},
			},
			wantAddr6:    "2001:db8::1/64",
			wantIPv6:     true,
			wantAcceptRA: boolPtr(false),
		},
		{
			name: "config with search domains",
			decl: &StateDeclaration{
				ID:     "eth0",
				State:  "configured",
				Module: "network",
				Parameters: map[string]interface{}{
					"address":        "192.168.1.100/24",
					"search_domains": "example.com,local.lan",
				},
			},
			wantSearchDoms: []string{"example.com", "local.lan"},
		},
		{
			name: "config with search domains array",
			decl: &StateDeclaration{
				ID:     "eth0",
				State:  "configured",
				Module: "network",
				Parameters: map[string]interface{}{
					"address":        "192.168.1.100/24",
					"search_domains": []string{"corp.example.com", "internal"},
				},
			},
			wantSearchDoms: []string{"corp.example.com", "internal"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := m.parseNetworkConfig(tt.decl)
			if err != nil {
				t.Fatalf("parseNetworkConfig() error = %v", err)
			}

			if tt.wantAddr6 != "" {
				if len(config.Addresses6) == 0 || config.Addresses6[0] != tt.wantAddr6 {
					gotAddr6 := ""
					if len(config.Addresses6) > 0 {
						gotAddr6 = config.Addresses6[0]
					}
					t.Errorf("Address6 = %s, want %s", gotAddr6, tt.wantAddr6)
				}
			}
			if config.Gateway6 != tt.wantGW6 {
				t.Errorf("Gateway6 = %s, want %s", config.Gateway6, tt.wantGW6)
			}
			if config.DHCP6 != tt.wantDHCP6 {
				t.Errorf("DHCP6 = %v, want %v", config.DHCP6, tt.wantDHCP6)
			}
			if config.IPv6Enabled != tt.wantIPv6 {
				t.Errorf("IPv6Enabled = %v, want %v", config.IPv6Enabled, tt.wantIPv6)
			}
			if config.IPv6Privacy != tt.wantIPv6Priv {
				t.Errorf("IPv6Privacy = %v, want %v", config.IPv6Privacy, tt.wantIPv6Priv)
			}
			if tt.wantAcceptRA != nil {
				if config.AcceptRA == nil {
					t.Errorf("AcceptRA = nil, want %v", *tt.wantAcceptRA)
				} else if *config.AcceptRA != *tt.wantAcceptRA {
					t.Errorf("AcceptRA = %v, want %v", *config.AcceptRA, *tt.wantAcceptRA)
				}
			}
			if len(tt.wantSearchDoms) > 0 {
				if len(config.SearchDomains) != len(tt.wantSearchDoms) {
					t.Errorf("SearchDomains len = %d, want %d", len(config.SearchDomains), len(tt.wantSearchDoms))
				} else {
					for i, dom := range tt.wantSearchDoms {
						if config.SearchDomains[i] != dom {
							t.Errorf("SearchDomains[%d] = %s, want %s", i, config.SearchDomains[i], dom)
						}
					}
				}
			}
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func TestNetworkModule_UpdateIfupdownConfig_IPv6Static(t *testing.T) {
	m := NewNetworkModule()

	raEnabled := true
	config := &NetworkConfig{
		Interface:   "eth0",
		Addresses:   []string{"192.168.1.100"},
		Gateway:     "192.168.1.1",
		DNS:         []string{"8.8.8.8", "2001:4860:4860::8888"},
		Addresses6:  []string{"2001:db8::1/64"},
		Gateway6:    "2001:db8::ffff",
		IPv6Enabled: true,
		AcceptRA:    &raEnabled,
	}

	result := m.updateIfupdownConfig("", config, "192.168.1.100", "255.255.255.0", false)

	// IPv4 stanza
	if !networkTestContains(result, "iface eth0 inet static") {
		t.Error("expected 'iface eth0 inet static' in result")
	}
	if !networkTestContains(result, "address 192.168.1.100") {
		t.Error("expected 'address 192.168.1.100' in result")
	}
	if !networkTestContains(result, "dns-nameservers 8.8.8.8") {
		t.Error("expected IPv4 DNS in inet stanza")
	}

	// IPv6 stanza
	if !networkTestContains(result, "iface eth0 inet6 static") {
		t.Error("expected 'iface eth0 inet6 static' in result")
	}
	if !networkTestContains(result, "address 2001:db8::1") {
		t.Error("expected IPv6 address in inet6 stanza")
	}
	if !networkTestContains(result, "netmask 64") {
		t.Error("expected IPv6 prefix length as netmask")
	}
	if !networkTestContains(result, "gateway 2001:db8::ffff") {
		t.Error("expected IPv6 gateway in inet6 stanza")
	}
	if !networkTestContains(result, "accept_ra 1") {
		t.Error("expected accept_ra setting in inet6 stanza")
	}
	if !networkTestContains(result, "dns-nameservers 2001:4860:4860::8888") {
		t.Error("expected IPv6 DNS in inet6 stanza")
	}
}

func TestNetworkModule_UpdateIfupdownConfig_DHCPv6(t *testing.T) {
	m := NewNetworkModule()

	config := &NetworkConfig{
		Interface:   "eth0",
		DHCP:        true,
		DHCP6:       true,
		IPv6Enabled: true,
	}

	result := m.updateIfupdownConfig("", config, "", "", true)

	// IPv4 DHCP stanza
	if !networkTestContains(result, "iface eth0 inet dhcp") {
		t.Error("expected 'iface eth0 inet dhcp' in result")
	}

	// IPv6 DHCPv6 stanza
	if !networkTestContains(result, "iface eth0 inet6 dhcp") {
		t.Error("expected 'iface eth0 inet6 dhcp' in result")
	}
}

func TestNetworkModule_UpdateIfupdownConfig_IPv6SLAAC(t *testing.T) {
	m := NewNetworkModule()

	config := &NetworkConfig{
		Interface:   "eth0",
		Addresses:   []string{"192.168.1.100"},
		IPv6Enabled: true,
		IPv6Privacy: true,
		// No Addresses6 or DHCP6, so SLAAC mode
	}

	result := m.updateIfupdownConfig("", config, "192.168.1.100", "255.255.255.0", false)

	// IPv6 SLAAC (auto) stanza
	if !networkTestContains(result, "iface eth0 inet6 auto") {
		t.Error("expected 'iface eth0 inet6 auto' in result")
	}
	if !networkTestContains(result, "privext 2") {
		t.Error("expected 'privext 2' for privacy extensions")
	}
}

func TestIsValidMAC(t *testing.T) {
	tests := []struct {
		mac  string
		want bool
	}{
		{"00:11:22:33:44:55", true},
		{"00:11:22:33:44:55", true},
		{"00-11-22-33-44-55", true},
		{"0011.2233.4455", true},
		{"aa:bb:cc:dd:ee:ff", true},
		{"AA:BB:CC:DD:EE:FF", true},
		{"", false},
		{"not-a-mac", false},
		{"00:11:22:33:44", false},       // too short
		{"00:11:22:33:44:55:66", false}, // too long
		{"gg:hh:ii:jj:kk:ll", false},    // invalid hex
	}

	for _, tt := range tests {
		t.Run(tt.mac, func(t *testing.T) {
			got := isValidMAC(tt.mac)
			if got != tt.want {
				t.Errorf("isValidMAC(%q) = %v, want %v", tt.mac, got, tt.want)
			}
		})
	}
}

func TestIsValidWoLMode(t *testing.T) {
	tests := []struct {
		mode string
		want bool
	}{
		{"magic", true},
		{"unicast", true},
		{"multicast", true},
		{"broadcast", true},
		{"arp", true},
		{"off", true},
		{"g", true},
		{"u", true},
		{"m", true},
		{"b", true},
		{"a", true},
		{"d", true},
		{"MAGIC", true}, // case insensitive
		{"Magic", true},
		{"G", true},
		{"", false},
		{"invalid", false},
		{"on", false},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			got := isValidWoLMode(tt.mode)
			if got != tt.want {
				t.Errorf("isValidWoLMode(%q) = %v, want %v", tt.mode, got, tt.want)
			}
		})
	}
}

func TestWolModeToEthtool(t *testing.T) {
	tests := []struct {
		mode string
		want string
	}{
		{"magic", "g"},
		{"unicast", "u"},
		{"multicast", "m"},
		{"broadcast", "b"},
		{"arp", "a"},
		{"off", "d"},
		{"MAGIC", "g"}, // case insensitive
		{"g", "g"},     // already shorthand
		{"d", "d"},
		{"unknown", "unknown"}, // passthrough
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			got := wolModeToEthtool(tt.mode)
			if got != tt.want {
				t.Errorf("wolModeToEthtool(%q) = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

func TestNetworkModule_ParseMACAndWoL(t *testing.T) {
	m := NewNetworkModule()

	tests := []struct {
		name    string
		decl    *StateDeclaration
		wantMAC string
		wantWoL string
	}{
		{
			name: "config with MAC address",
			decl: &StateDeclaration{
				ID:     "eth0",
				State:  "configured",
				Module: "network",
				Parameters: map[string]interface{}{
					"address":     "192.168.1.100/24",
					"mac_address": "00:11:22:33:44:55",
				},
			},
			wantMAC: "00:11:22:33:44:55",
		},
		{
			name: "config with Wake-on-LAN (wol)",
			decl: &StateDeclaration{
				ID:     "eth0",
				State:  "configured",
				Module: "network",
				Parameters: map[string]interface{}{
					"address": "192.168.1.100/24",
					"wol":     "magic",
				},
			},
			wantWoL: "magic",
		},
		{
			name: "config with Wake-on-LAN (wake_on_lan)",
			decl: &StateDeclaration{
				ID:     "eth0",
				State:  "configured",
				Module: "network",
				Parameters: map[string]interface{}{
					"address":     "192.168.1.100/24",
					"wake_on_lan": "g",
				},
			},
			wantWoL: "g",
		},
		{
			name: "config with both MAC and WoL",
			decl: &StateDeclaration{
				ID:     "eth0",
				State:  "configured",
				Module: "network",
				Parameters: map[string]interface{}{
					"address":     "192.168.1.100/24",
					"mac_address": "aa:bb:cc:dd:ee:ff",
					"wol":         "off",
				},
			},
			wantMAC: "aa:bb:cc:dd:ee:ff",
			wantWoL: "off",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := m.parseNetworkConfig(tt.decl)
			if err != nil {
				t.Fatalf("parseNetworkConfig() error = %v", err)
			}

			if config.MACAddress != tt.wantMAC {
				t.Errorf("MACAddress = %q, want %q", config.MACAddress, tt.wantMAC)
			}
			if config.WakeOnLAN != tt.wantWoL {
				t.Errorf("WakeOnLAN = %q, want %q", config.WakeOnLAN, tt.wantWoL)
			}
		})
	}
}

func TestNetworkModule_UpdateIfupdownConfig_WithMACAndWoL(t *testing.T) {
	m := NewNetworkModule()

	config := &NetworkConfig{
		Interface:  "eth0",
		Addresses:  []string{"192.168.1.100"},
		Gateway:    "192.168.1.1",
		MACAddress: "00:11:22:33:44:55",
		WakeOnLAN:  "magic",
	}

	result := m.updateIfupdownConfig("", config, "192.168.1.100", "255.255.255.0", false)

	if !networkTestContains(result, "hwaddress ether 00:11:22:33:44:55") {
		t.Error("expected 'hwaddress ether 00:11:22:33:44:55' in result")
	}
	if !networkTestContains(result, "post-up ethtool -s eth0 wol g") {
		t.Error("expected 'post-up ethtool -s eth0 wol g' in result")
	}
}

func TestNetworkModule_UpdateIfupdownConfig_WoLOff(t *testing.T) {
	m := NewNetworkModule()

	config := &NetworkConfig{
		Interface: "eth0",
		Addresses: []string{"192.168.1.100"},
		WakeOnLAN: "off",
	}

	result := m.updateIfupdownConfig("", config, "192.168.1.100", "255.255.255.0", false)

	if !networkTestContains(result, "post-up ethtool -s eth0 wol d") {
		t.Error("expected 'post-up ethtool -s eth0 wol d' for WoL off")
	}
}
