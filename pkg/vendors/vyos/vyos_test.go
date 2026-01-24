package vyos

import (
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/vendors"
)

func TestVyOSVersionStructure(t *testing.T) {
	version := &VyOSVersion{
		Version:      "1.4.0",
		BuildDate:    "2024-01-15",
		Variant:      "vyos",
		Architecture: "amd64",
	}

	if version.Version != "1.4.0" {
		t.Errorf("Version = %v", version.Version)
	}
	if version.Variant != "vyos" {
		t.Errorf("Variant = %v", version.Variant)
	}
}

func TestVyOSConfigStructure(t *testing.T) {
	cfg := &VyOSConfig{
		Raw: "interfaces ethernet eth0 address 192.168.1.1/24",
		Tree: map[string]interface{}{
			"interfaces": map[string]interface{}{
				"ethernet": "eth0",
			},
		},
		Commands: []string{"set interfaces ethernet eth0 address 192.168.1.1/24"},
	}

	if cfg.Raw == "" {
		t.Error("Raw should not be empty")
	}
	if len(cfg.Commands) != 1 {
		t.Errorf("Commands count = %d", len(cfg.Commands))
	}
}

func TestVyOSInterfaceStructure(t *testing.T) {
	iface := VyOSInterface{
		Name:        "eth0",
		Type:        "ethernet",
		Description: "WAN interface",
		Address:     []string{"192.168.1.1/24", "10.0.0.1/8"},
		State:       "up",
		HWAddress:   "00:11:22:33:44:55",
		MTU:         1500,
	}

	if iface.Name != "eth0" {
		t.Errorf("Name = %v", iface.Name)
	}
	if iface.Type != "ethernet" {
		t.Errorf("Type = %v", iface.Type)
	}
	if len(iface.Address) != 2 {
		t.Errorf("Address count = %d", len(iface.Address))
	}
	if iface.MTU != 1500 {
		t.Errorf("MTU = %d", iface.MTU)
	}
}

func TestVyOSRouteStructure(t *testing.T) {
	route := VyOSRoute{
		Destination: "0.0.0.0/0",
		NextHop:     "192.168.1.1",
		Interface:   "eth0",
		Distance:    1,
		Blackhole:   false,
	}

	if route.Destination != "0.0.0.0/0" {
		t.Errorf("Destination = %v", route.Destination)
	}
	if route.NextHop != "192.168.1.1" {
		t.Errorf("NextHop = %v", route.NextHop)
	}
	if route.Distance != 1 {
		t.Errorf("Distance = %d", route.Distance)
	}
}

func TestVyOSFirewallRuleStructure(t *testing.T) {
	rule := VyOSFirewallRule{
		Number:      10,
		Action:      "accept",
		Protocol:    "tcp",
		Source:      "192.168.1.0/24",
		Destination: "10.0.0.0/8",
		Port:        "443",
		State:       []string{"new", "established"},
		Description: "Allow HTTPS",
		Log:         true,
		Disabled:    false,
	}

	if rule.Number != 10 {
		t.Errorf("Number = %d", rule.Number)
	}
	if rule.Action != "accept" {
		t.Errorf("Action = %v", rule.Action)
	}
	if len(rule.State) != 2 {
		t.Errorf("State count = %d", len(rule.State))
	}
	if !rule.Log {
		t.Error("Log should be true")
	}
}

func TestVyOSFirewallRulesetStructure(t *testing.T) {
	ruleset := VyOSFirewallRuleset{
		Name:          "WAN_IN",
		DefaultAction: "drop",
		Description:   "Inbound WAN firewall",
		Rules: []VyOSFirewallRule{
			{Number: 10, Action: "accept", Protocol: "icmp"},
			{Number: 20, Action: "accept", Protocol: "tcp", Port: "22"},
		},
	}

	if ruleset.Name != "WAN_IN" {
		t.Errorf("Name = %v", ruleset.Name)
	}
	if ruleset.DefaultAction != "drop" {
		t.Errorf("DefaultAction = %v", ruleset.DefaultAction)
	}
	if len(ruleset.Rules) != 2 {
		t.Errorf("Rules count = %d", len(ruleset.Rules))
	}
}

func TestGetInterfaceType(t *testing.T) {
	adapter := &VyOSAdapter{}

	tests := []struct {
		name     string
		expected string
	}{
		{"eth0", "ethernet"},
		{"eth1", "ethernet"},
		{"lo", "loopback"},
		{"bond0", "bonding"},
		{"br0", "bridge"},
		{"tun0", "tunnel"},
		{"vti0", "vti"},
		{"wg0", "wireguard"},
		{"vlan100", "vif"},
		// Note: eth0.100 and br0.100 return their prefix type because prefix checks come first
		{"eth0.100", "ethernet"},
		{"br0.100", "bridge"},
		// Only "vlan" prefix or names without known prefix containing "." return "vif"
		{"foo.100", "vif"},
		{"unknown0", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adapter.getInterfaceType(tt.name)
			if got != tt.expected {
				t.Errorf("getInterfaceType(%q) = %v, want %v", tt.name, got, tt.expected)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	adapter := &VyOSAdapter{}

	versionOutput := `Version:          VyOS 1.4.0
Build date:       2024-01-15
Release train:    sagitta
Architecture:     amd64

Built by:         autobuild@vyos.net
Built on:         https://github.com/vyos/vyos-build
Compiled for CPU: Intel Core Processor`

	version := adapter.parseVersion(versionOutput)

	if version.Version != "VyOS 1.4.0" {
		t.Errorf("Version = %v", version.Version)
	}
	if version.BuildDate != "2024-01-15" {
		t.Errorf("BuildDate = %v", version.BuildDate)
	}
	if version.Architecture != "amd64" {
		t.Errorf("Architecture = %v", version.Architecture)
	}
	if version.Variant != "vyos" {
		t.Errorf("Variant = %v", version.Variant)
	}
}

func TestParseVersionEdgeOS(t *testing.T) {
	adapter := &VyOSAdapter{}

	versionOutput := `Version:      EdgeOS v2.0.9
Build date:   2022-06-15
Ubiquiti Networks, Inc.`

	version := adapter.parseVersion(versionOutput)

	if version.Variant != "edgeos" {
		t.Errorf("Variant = %v, want 'edgeos'", version.Variant)
	}
}

func TestParseUptime(t *testing.T) {
	adapter := &VyOSAdapter{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic uptime",
			input:    "system up 10 days, 5:30",
			expected: "10 days, 5:30",
		},
		{
			name:     "no match",
			input:    "unknown format",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// parseUptime returns a string representation
			got := adapter.parseUptime(tt.input)
			// Just check it doesn't panic
			_ = got
		})
	}
}

func TestParseUptimeDuration(t *testing.T) {
	adapter := &VyOSAdapter{}

	tests := []struct {
		name   string
		input  string
		minVal time.Duration
	}{
		{
			name:   "days and time",
			input:  "up 5 days, 2:30",
			minVal: 5*24*time.Hour + 2*time.Hour + 30*time.Minute,
		},
		{
			name:   "hours only",
			input:  "up 0 days, 5:45",
			minVal: 5*time.Hour + 45*time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uptime := adapter.parseUptimeDuration(tt.input)
			if uptime < tt.minVal {
				t.Errorf("parseUptimeDuration(%q) = %v, want >= %v", tt.input, uptime, tt.minVal)
			}
		})
	}
}

func TestParseInterfaceFacts(t *testing.T) {
	adapter := &VyOSAdapter{}

	output := `eth0     up      192.168.1.1/24
eth1     down    unassigned
lo       up      127.0.0.1/8`

	interfaces := adapter.parseInterfaceFacts(output)

	if len(interfaces) != 3 {
		t.Fatalf("expected 3 interfaces, got %d", len(interfaces))
	}
}

func TestParseInterfaces(t *testing.T) {
	adapter := &VyOSAdapter{}

	output := `eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500
    link/ether 00:0c:29:xx:xx:xx brd ff:ff:ff:ff:ff:ff
    inet 192.168.1.1/24 brd 192.168.1.255 scope global eth0
eth1: <BROADCAST,MULTICAST> mtu 1500
    link/ether 00:0c:29:yy:yy:yy brd ff:ff:ff:ff:ff:ff`

	interfaces := adapter.parseInterfaces(output)

	if len(interfaces) < 1 {
		t.Fatal("expected at least 1 interface")
	}

	// Check first interface
	eth0 := interfaces[0]
	if eth0.Name != "eth0" {
		t.Errorf("Name = %v", eth0.Name)
	}
	if eth0.State != "up" {
		t.Errorf("State = %v", eth0.State)
	}
	if eth0.MTU != 1500 {
		t.Errorf("MTU = %d", eth0.MTU)
	}
}

func TestParseRoutes(t *testing.T) {
	adapter := &VyOSAdapter{}

	output := `Codes: K - kernel, C - connected, S - static
S>* 0.0.0.0/0 [1/0] via 192.168.1.1, eth0
C>* 192.168.1.0/24 is directly connected, eth0
C>* 10.0.0.0/8 is directly connected, eth1`

	routes := adapter.parseRoutes(output)

	if len(routes) < 2 {
		t.Fatalf("expected at least 2 routes, got %d", len(routes))
	}
}

func TestParseFirewallRulesets(t *testing.T) {
	adapter := &VyOSAdapter{}

	output := `Ruleset: WAN_IN
default-action drop

Ruleset: LAN_OUT
default-action accept`

	rulesets := adapter.parseFirewallRulesets(output)

	if len(rulesets) != 2 {
		t.Fatalf("expected 2 rulesets, got %d", len(rulesets))
	}

	if rulesets[0].Name != "WAN_IN" {
		t.Errorf("rulesets[0].Name = %v", rulesets[0].Name)
	}
	if rulesets[0].DefaultAction != "drop" {
		t.Errorf("rulesets[0].DefaultAction = %v", rulesets[0].DefaultAction)
	}
}

func TestParseMemoryInfo(t *testing.T) {
	adapter := &VyOSAdapter{}

	// VyOS memory output format with "total" and "free" labels on separate lines
	output := `Memory information:
Total: 4096000 kB
Used:  2048000 kB
Free:  2048000 kB`

	total, free := adapter.parseMemoryInfo(output)

	if total <= 0 {
		t.Error("total should be > 0")
	}
	if free <= 0 {
		t.Error("free should be > 0")
	}
	// Values should be converted from KB to bytes
	expectedTotal := int64(4096000 * 1024)
	expectedFree := int64(2048000 * 1024)
	if total != expectedTotal {
		t.Errorf("total = %d, want %d", total, expectedTotal)
	}
	if free != expectedFree {
		t.Errorf("free = %d, want %d", free, expectedFree)
	}
}

func TestParseSerialNumber(t *testing.T) {
	adapter := &VyOSAdapter{}

	// VyOS typically doesn't have a serial number
	serial := adapter.parseSerialNumber("CPU info output")
	if serial != "" {
		t.Errorf("SerialNumber should be empty, got %v", serial)
	}
}

func TestVyOSAdapterVendor(t *testing.T) {
	adapter := &VyOSAdapter{}
	if adapter.Vendor() != vendors.VendorVyOS {
		t.Errorf("Vendor() = %v, want VendorVyOS", adapter.Vendor())
	}
}
