// Copyright 2026 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

package netutil

import (
	"net"
	"testing"
)

func TestNetworkInfo_PreferredAddress(t *testing.T) {
	tests := []struct {
		name     string
		info     *NetworkInfo
		pref     AddressFamilyPreference
		expected string
	}{
		{
			name: "prefer IPv4 with both available",
			info: &NetworkInfo{
				PrimaryIPv4: "192.168.1.1",
				PrimaryIPv6: "2001:db8::1",
			},
			pref:     PreferIPv4,
			expected: "192.168.1.1",
		},
		{
			name: "prefer IPv6 with both available",
			info: &NetworkInfo{
				PrimaryIPv4: "192.168.1.1",
				PrimaryIPv6: "2001:db8::1",
			},
			pref:     PreferIPv6,
			expected: "2001:db8::1",
		},
		{
			name: "IPv4 only mode",
			info: &NetworkInfo{
				PrimaryIPv4: "192.168.1.1",
				PrimaryIPv6: "2001:db8::1",
			},
			pref:     IPv4Only,
			expected: "192.168.1.1",
		},
		{
			name: "IPv6 only mode",
			info: &NetworkInfo{
				PrimaryIPv4: "192.168.1.1",
				PrimaryIPv6: "2001:db8::1",
			},
			pref:     IPv6Only,
			expected: "2001:db8::1",
		},
		{
			name: "prefer IPv4 but only IPv6 available",
			info: &NetworkInfo{
				PrimaryIPv6: "2001:db8::1",
			},
			pref:     PreferIPv4,
			expected: "2001:db8::1",
		},
		{
			name: "prefer IPv6 but only IPv4 available",
			info: &NetworkInfo{
				PrimaryIPv4: "192.168.1.1",
			},
			pref:     PreferIPv6,
			expected: "192.168.1.1",
		},
		{
			name: "IPv4 only but only IPv6 available",
			info: &NetworkInfo{
				PrimaryIPv6: "2001:db8::1",
			},
			pref:     IPv4Only,
			expected: "",
		},
		{
			name: "IPv6 only but only IPv4 available",
			info: &NetworkInfo{
				PrimaryIPv4: "192.168.1.1",
			},
			pref:     IPv6Only,
			expected: "",
		},
		{
			name:     "no addresses available",
			info:     &NetworkInfo{},
			pref:     PreferIPv4,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.info.PreferredAddress(tt.pref)
			if result != tt.expected {
				t.Errorf("PreferredAddress(%v) = %q, want %q", tt.pref, result, tt.expected)
			}
		})
	}
}

func TestNetworkInfo_AllAddresses(t *testing.T) {
	info := &NetworkInfo{
		IPv4Addresses: []string{"192.168.1.1", "10.0.0.1"},
		IPv6Addresses: []string{"2001:db8::1", "2001:db8::2"},
	}

	all := info.AllAddresses()
	if len(all) != 4 {
		t.Errorf("AllAddresses() returned %d addresses, want 4", len(all))
	}

	// Check all addresses are present
	expected := map[string]bool{
		"192.168.1.1": false,
		"10.0.0.1":    false,
		"2001:db8::1": false,
		"2001:db8::2": false,
	}
	for _, addr := range all {
		if _, ok := expected[addr]; ok {
			expected[addr] = true
		}
	}
	for addr, found := range expected {
		if !found {
			t.Errorf("AllAddresses() missing %q", addr)
		}
	}
}

func TestSelectPrimaryIPv4(t *testing.T) {
	tests := []struct {
		name      string
		addresses []string
		expected  string
	}{
		{
			name:      "empty list",
			addresses: []string{},
			expected:  "",
		},
		{
			name:      "single private address",
			addresses: []string{"192.168.1.1"},
			expected:  "192.168.1.1",
		},
		{
			name:      "prefer public over private",
			addresses: []string{"192.168.1.1", "8.8.8.8"},
			expected:  "8.8.8.8",
		},
		{
			name:      "multiple private addresses",
			addresses: []string{"10.0.0.1", "172.16.0.1", "192.168.1.1"},
			expected:  "10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := selectPrimaryIPv4(tt.addresses)
			if result != tt.expected {
				t.Errorf("selectPrimaryIPv4(%v) = %q, want %q", tt.addresses, result, tt.expected)
			}
		})
	}
}

func TestIsPrivateIPv4(t *testing.T) {
	tests := []struct {
		ip       string
		expected bool
	}{
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"172.15.0.1", false},
		{"172.32.0.1", false},
		{"192.168.0.1", true},
		{"192.168.255.255", true},
		{"169.254.0.1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			result := isPrivateIPv4(ip)
			if result != tt.expected {
				t.Errorf("isPrivateIPv4(%s) = %v, want %v", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestSelectPrimaryIPv6(t *testing.T) {
	tests := []struct {
		name      string
		addresses []string
		expected  string
	}{
		{
			name:      "empty list",
			addresses: []string{},
			expected:  "",
		},
		{
			name:      "single global unicast",
			addresses: []string{"2001:db8::1"},
			expected:  "2001:db8::1",
		},
		{
			name:      "prefer global over unique local",
			addresses: []string{"fd00::1", "2001:db8::1"},
			expected:  "2001:db8::1",
		},
		{
			name:      "only unique local",
			addresses: []string{"fd00::1", "fc00::1"},
			expected:  "fd00::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := selectPrimaryIPv6(tt.addresses)
			if result != tt.expected {
				t.Errorf("selectPrimaryIPv6(%v) = %q, want %q", tt.addresses, result, tt.expected)
			}
		})
	}
}

func TestIsGlobalUnicastIPv6(t *testing.T) {
	tests := []struct {
		ip       string
		expected bool
	}{
		{"2001:db8::1", true},
		{"2607:f8b0:4004:800::200e", true}, // Google
		{"2600:1f18:1234:5678::1", true},   // AWS
		{"fd00::1", false},                 // Unique local
		{"fc00::1", false},                 // Unique local
		{"fe80::1", false},                 // Link local
		{"::1", false},                     // Loopback
		{"192.168.1.1", false},             // IPv4
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			result := isGlobalUnicastIPv6(ip)
			if result != tt.expected {
				t.Errorf("isGlobalUnicastIPv6(%s) = %v, want %v", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestGetNetworkInfo(t *testing.T) {
	// This test verifies the function doesn't panic and returns valid data
	info, err := GetNetworkInfo()
	if err != nil {
		t.Fatalf("GetNetworkInfo() error: %v", err)
	}

	if info == nil {
		t.Fatal("GetNetworkInfo() returned nil")
	}

	// At minimum, we should have loopback addresses on most systems
	// But we can't guarantee specific addresses, so just check structure
	t.Logf("IPv4 addresses: %v", info.IPv4Addresses)
	t.Logf("IPv6 addresses: %v", info.IPv6Addresses)
	t.Logf("Link-local IPv6: %v", info.LinkLocalIPv6)
	t.Logf("Loopback IPv4: %s", info.LoopbackIPv4)
	t.Logf("Loopback IPv6: %s", info.LoopbackIPv6)
	t.Logf("Primary IPv4: %s", info.PrimaryIPv4)
	t.Logf("Primary IPv6: %s", info.PrimaryIPv6)
	t.Logf("HasIPv4: %v, HasIPv6: %v", info.HasIPv4, info.HasIPv6)
}

func TestGetInterfaceAddresses(t *testing.T) {
	// This test verifies the function doesn't panic and returns valid data
	ifaces, err := GetInterfaceAddresses()
	if err != nil {
		t.Fatalf("GetInterfaceAddresses() error: %v", err)
	}

	// We should have at least one interface (loopback)
	if len(ifaces) == 0 {
		t.Log("Warning: no interfaces found")
	}

	for _, iface := range ifaces {
		t.Logf("Interface %s (index %d): IPv4=%v, IPv6=%v",
			iface.Name, iface.Index, iface.IPv4, iface.IPv6)
	}
}

func TestResolveHostAddresses(t *testing.T) {
	// Test with localhost which should always resolve
	info, err := ResolveHostAddresses("localhost")
	if err != nil {
		// Some systems may not have localhost in hosts file
		t.Logf("ResolveHostAddresses(localhost) error (may be expected): %v", err)
		return
	}

	if info == nil {
		t.Fatal("ResolveHostAddresses() returned nil")
	}

	t.Logf("localhost resolved to: IPv4=%v, IPv6=%v", info.IPv4Addresses, info.IPv6Addresses)
}

func TestSelectAddressForConnection(t *testing.T) {
	localInfo := &NetworkInfo{
		PrimaryIPv4: "192.168.1.100",
		PrimaryIPv6: "2001:db8::100",
	}

	tests := []struct {
		name       string
		remoteAddr *Address
		pref       AddressFamilyPreference
		expected   string
	}{
		{
			name:       "connect to IPv4 with prefer IPv4",
			remoteAddr: &Address{IP: net.ParseIP("192.168.1.1"), Family: FamilyIPv4},
			pref:       PreferIPv4,
			expected:   "192.168.1.100",
		},
		{
			name:       "connect to IPv6 with prefer IPv6",
			remoteAddr: &Address{IP: net.ParseIP("2001:db8::1"), Family: FamilyIPv6},
			pref:       PreferIPv6,
			expected:   "2001:db8::100",
		},
		{
			name:       "connect to IPv4 with IPv6 only",
			remoteAddr: &Address{IP: net.ParseIP("192.168.1.1"), Family: FamilyIPv4},
			pref:       IPv6Only,
			expected:   "",
		},
		{
			name:       "connect to IPv6 with IPv4 only",
			remoteAddr: &Address{IP: net.ParseIP("2001:db8::1"), Family: FamilyIPv6},
			pref:       IPv4Only,
			expected:   "",
		},
		{
			name:       "connect to hostname with prefer IPv4",
			remoteAddr: &Address{Host: "example.com", Family: FamilyUnknown},
			pref:       PreferIPv4,
			expected:   "192.168.1.100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SelectAddressForConnection(tt.remoteAddr, localInfo, tt.pref)
			if result != tt.expected {
				t.Errorf("SelectAddressForConnection() = %q, want %q", result, tt.expected)
			}
		})
	}
}
