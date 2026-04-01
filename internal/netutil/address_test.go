// Copyright 2026 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

package netutil

import (
	"net"
	"testing"
)

func TestAddressFamilyString(t *testing.T) {
	tests := []struct {
		family   AddressFamily
		expected string
	}{
		{FamilyUnknown, "unknown"},
		{FamilyIPv4, "ipv4"},
		{FamilyIPv6, "ipv6"},
		{FamilyDualStack, "dual-stack"},
	}

	for _, tt := range tests {
		got := tt.family.String()
		if got != tt.expected {
			t.Errorf("AddressFamily(%d).String() = %s, want %s", tt.family, got, tt.expected)
		}
	}
}

func TestParseAddressFamily(t *testing.T) {
	tests := []struct {
		input    string
		expected AddressFamily
	}{
		{"ipv4", FamilyIPv4},
		{"IPv4", FamilyIPv4},
		{"ip4", FamilyIPv4},
		{"4", FamilyIPv4},
		{"ipv6", FamilyIPv6},
		{"IPv6", FamilyIPv6},
		{"ip6", FamilyIPv6},
		{"6", FamilyIPv6},
		{"dual-stack", FamilyDualStack},
		{"dualstack", FamilyDualStack},
		{"dual", FamilyDualStack},
		{"both", FamilyDualStack},
		{"unknown", FamilyUnknown},
		{"invalid", FamilyUnknown},
	}

	for _, tt := range tests {
		got := ParseAddressFamily(tt.input)
		if got != tt.expected {
			t.Errorf("ParseAddressFamily(%s) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestParseAddressFamilyPreference(t *testing.T) {
	tests := []struct {
		input       string
		expected    AddressFamilyPreference
		shouldError bool
	}{
		{"prefer_ipv4", PreferIPv4, false},
		{"prefer-ipv4", PreferIPv4, false},
		{"preferipv4", PreferIPv4, false},
		{"prefer_ipv6", PreferIPv6, false},
		{"prefer-ipv6", PreferIPv6, false},
		{"ipv4_only", IPv4Only, false},
		{"ipv4-only", IPv4Only, false},
		{"ipv4only", IPv4Only, false},
		{"ipv4", IPv4Only, false},
		{"ipv6_only", IPv6Only, false},
		{"ipv6-only", IPv6Only, false},
		{"ipv6only", IPv6Only, false},
		{"ipv6", IPv6Only, false},
		{"invalid", PreferIPv4, true},
	}

	for _, tt := range tests {
		got, err := ParseAddressFamilyPreference(tt.input)
		if tt.shouldError {
			if err == nil {
				t.Errorf("ParseAddressFamilyPreference(%s) expected error, got nil", tt.input)
			}
		} else {
			if err != nil {
				t.Errorf("ParseAddressFamilyPreference(%s) unexpected error: %v", tt.input, err)
			}
			if got != tt.expected {
				t.Errorf("ParseAddressFamilyPreference(%s) = %v, want %v", tt.input, got, tt.expected)
			}
		}
	}
}

func TestParseAddressIPv4(t *testing.T) {
	tests := []struct {
		input    string
		host     string
		port     int
		family   AddressFamily
		hasError bool
	}{
		{"192.168.1.1:8080", "192.168.1.1", 8080, FamilyIPv4, false},
		{"127.0.0.1:443", "127.0.0.1", 443, FamilyIPv4, false},
		{"0.0.0.0:80", "0.0.0.0", 80, FamilyIPv4, false},
		{"10.0.1.5", "10.0.1.5", 0, FamilyIPv4, false},
		{"localhost:8080", "localhost", 8080, FamilyUnknown, false},
		{"example.com:443", "example.com", 443, FamilyUnknown, false},
		{"", "", 0, FamilyUnknown, true},
		{"192.168.1.1:99999", "", 0, FamilyUnknown, true}, // Port out of range
		{"192.168.1.1:-1", "", 0, FamilyUnknown, true},    // Negative port
		{"192.168.1.1:abc", "", 0, FamilyUnknown, true},   // Invalid port
	}

	for _, tt := range tests {
		addr, err := ParseAddress(tt.input)
		if tt.hasError {
			if err == nil {
				t.Errorf("ParseAddress(%s) expected error, got nil", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseAddress(%s) unexpected error: %v", tt.input, err)
			continue
		}
		if addr.Host != tt.host {
			t.Errorf("ParseAddress(%s).Host = %s, want %s", tt.input, addr.Host, tt.host)
		}
		if addr.Port != tt.port {
			t.Errorf("ParseAddress(%s).Port = %d, want %d", tt.input, addr.Port, tt.port)
		}
		if addr.Family != tt.family {
			t.Errorf("ParseAddress(%s).Family = %v, want %v", tt.input, addr.Family, tt.family)
		}
	}
}

func TestParseAddressIPv6(t *testing.T) {
	tests := []struct {
		input    string
		host     string
		port     int
		family   AddressFamily
		hasError bool
	}{
		{"[::1]:8080", "::1", 8080, FamilyIPv6, false},
		{"[::]:80", "::", 80, FamilyIPv6, false},
		{"[2001:db8::1]:443", "2001:db8::1", 443, FamilyIPv6, false},
		{"[2001:db8:85a3::8a2e:370:7334]:8080", "2001:db8:85a3::8a2e:370:7334", 8080, FamilyIPv6, false},
		{"[::1]", "::1", 0, FamilyIPv6, false},
		{"::1", "::1", 0, FamilyIPv6, false},
		{"::", "::", 0, FamilyIPv6, false},
		// Full notation
		{"[2001:0db8:85a3:0000:0000:8a2e:0370:7334]:8080", "2001:0db8:85a3:0000:0000:8a2e:0370:7334", 8080, FamilyIPv6, false},
	}

	for _, tt := range tests {
		addr, err := ParseAddress(tt.input)
		if tt.hasError {
			if err == nil {
				t.Errorf("ParseAddress(%s) expected error, got nil", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseAddress(%s) unexpected error: %v", tt.input, err)
			continue
		}
		if addr.Host != tt.host {
			t.Errorf("ParseAddress(%s).Host = %s, want %s", tt.input, addr.Host, tt.host)
		}
		if addr.Port != tt.port {
			t.Errorf("ParseAddress(%s).Port = %d, want %d", tt.input, addr.Port, tt.port)
		}
		if addr.Family != tt.family {
			t.Errorf("ParseAddress(%s).Family = %v, want %v", tt.input, addr.Family, tt.family)
		}
	}
}

func TestParseAddressWithZoneID(t *testing.T) {
	tests := []struct {
		input string
		host  string
		zone  string
	}{
		{"[fe80::1%eth0]:8080", "fe80::1", "eth0"},
		{"fe80::1%eth0", "fe80::1", "eth0"},
		{"[fe80::1%en0]:443", "fe80::1", "en0"},
	}

	for _, tt := range tests {
		addr, err := ParseAddress(tt.input)
		if err != nil {
			t.Errorf("ParseAddress(%s) unexpected error: %v", tt.input, err)
			continue
		}
		if addr.Host != tt.host {
			t.Errorf("ParseAddress(%s).Host = %s, want %s", tt.input, addr.Host, tt.host)
		}
		if addr.Zone != tt.zone {
			t.Errorf("ParseAddress(%s).Zone = %s, want %s", tt.input, addr.Zone, tt.zone)
		}
	}
}

func TestParseURL(t *testing.T) {
	tests := []struct {
		input    string
		host     string
		port     int
		family   AddressFamily
		hasError bool
	}{
		{"nats://192.168.1.1:4222", "192.168.1.1", 4222, FamilyIPv4, false},
		{"nats://[::1]:4222", "::1", 4222, FamilyIPv6, false},
		{"nats://[2001:db8::1]:4222", "2001:db8::1", 4222, FamilyIPv6, false},
		{"grpc://localhost:8080", "localhost", 8080, FamilyUnknown, false},
		{"https://[2001:db8::1]:443/api", "2001:db8::1", 443, FamilyIPv6, false},
		{"http://example.com:80/path", "example.com", 80, FamilyUnknown, false},
		{"nats://server:4222", "server", 4222, FamilyUnknown, false},
		{"", "", 0, FamilyUnknown, true},
	}

	for _, tt := range tests {
		addr, err := ParseURL(tt.input)
		if tt.hasError {
			if err == nil {
				t.Errorf("ParseURL(%s) expected error, got nil", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseURL(%s) unexpected error: %v", tt.input, err)
			continue
		}
		if addr.Host != tt.host {
			t.Errorf("ParseURL(%s).Host = %s, want %s", tt.input, addr.Host, tt.host)
		}
		if addr.Port != tt.port {
			t.Errorf("ParseURL(%s).Port = %d, want %d", tt.input, addr.Port, tt.port)
		}
		if addr.Family != tt.family {
			t.Errorf("ParseURL(%s).Family = %v, want %v", tt.input, addr.Family, tt.family)
		}
	}
}

func TestAddressString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"192.168.1.1:8080", "192.168.1.1:8080"},
		{"[::1]:8080", "[::1]:8080"},
		{"localhost:80", "localhost:80"},
		{"192.168.1.1", "192.168.1.1"},
		{"[::1]", "::1"}, // No port, no brackets in output
	}

	for _, tt := range tests {
		addr, err := ParseAddress(tt.input)
		if err != nil {
			t.Errorf("ParseAddress(%s) error: %v", tt.input, err)
			continue
		}
		got := addr.String()
		if got != tt.expected {
			t.Errorf("ParseAddress(%s).String() = %s, want %s", tt.input, got, tt.expected)
		}
	}
}

func TestAddressMethods(t *testing.T) {
	// Test IsIPv4
	ipv4, _ := ParseAddress("192.168.1.1:8080")
	if !ipv4.IsIPv4() {
		t.Error("192.168.1.1:8080 should be IPv4")
	}
	if ipv4.IsIPv6() {
		t.Error("192.168.1.1:8080 should not be IPv6")
	}

	// Test IsIPv6
	ipv6, _ := ParseAddress("[::1]:8080")
	if !ipv6.IsIPv6() {
		t.Error("[::1]:8080 should be IPv6")
	}
	if ipv6.IsIPv4() {
		t.Error("[::1]:8080 should not be IPv4")
	}

	// Test IsLoopback
	loopback4, _ := ParseAddress("127.0.0.1:8080")
	if !loopback4.IsLoopback() {
		t.Error("127.0.0.1 should be loopback")
	}

	loopback6, _ := ParseAddress("[::1]:8080")
	if !loopback6.IsLoopback() {
		t.Error("::1 should be loopback")
	}

	// Test IsUnspecified
	unspec4, _ := ParseAddress("0.0.0.0:8080")
	if !unspec4.IsUnspecified() {
		t.Error("0.0.0.0 should be unspecified")
	}

	unspec6, _ := ParseAddress("[::]:8080")
	if !unspec6.IsUnspecified() {
		t.Error(":: should be unspecified")
	}

	// Test Network
	if ipv4.Network() != "tcp4" {
		t.Errorf("IPv4 Network() = %s, want tcp4", ipv4.Network())
	}
	if ipv6.Network() != "tcp6" {
		t.Errorf("IPv6 Network() = %s, want tcp6", ipv6.Network())
	}
}

func TestFormatAddress(t *testing.T) {
	tests := []struct {
		host     string
		port     int
		expected string
	}{
		{"192.168.1.1", 8080, "192.168.1.1:8080"},
		{"::1", 8080, "[::1]:8080"},
		{"2001:db8::1", 443, "[2001:db8::1]:443"},
		{"localhost", 80, "localhost:80"},
		{"192.168.1.1", 0, "192.168.1.1"},
		{"::1", 0, "[::1]"},
	}

	for _, tt := range tests {
		got := FormatAddress(tt.host, tt.port)
		if got != tt.expected {
			t.Errorf("FormatAddress(%s, %d) = %s, want %s", tt.host, tt.port, got, tt.expected)
		}
	}
}

func TestFormatURL(t *testing.T) {
	tests := []struct {
		scheme   string
		host     string
		port     int
		path     string
		expected string
	}{
		{"nats", "192.168.1.1", 4222, "", "nats://192.168.1.1:4222"},
		{"nats", "::1", 4222, "", "nats://[::1]:4222"},
		{"https", "2001:db8::1", 443, "/api", "https://[2001:db8::1]:443/api"},
		{"grpc", "localhost", 8080, "service", "grpc://localhost:8080/service"},
	}

	for _, tt := range tests {
		got := FormatURL(tt.scheme, tt.host, tt.port, tt.path)
		if got != tt.expected {
			t.Errorf("FormatURL(%s, %s, %d, %s) = %s, want %s",
				tt.scheme, tt.host, tt.port, tt.path, got, tt.expected)
		}
	}
}

func TestParseCIDR(t *testing.T) {
	tests := []struct {
		input    string
		family   AddressFamily
		hasError bool
	}{
		{"192.168.1.0/24", FamilyIPv4, false},
		{"10.0.0.0/8", FamilyIPv4, false},
		{"2001:db8::/32", FamilyIPv6, false},
		{"::1/128", FamilyIPv6, false},
		{"invalid", FamilyUnknown, true},
		{"", FamilyUnknown, true},
		{"192.168.1.0/33", FamilyUnknown, true}, // Invalid prefix length
	}

	for _, tt := range tests {
		cidr, err := ParseCIDR(tt.input)
		if tt.hasError {
			if err == nil {
				t.Errorf("ParseCIDR(%s) expected error, got nil", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseCIDR(%s) unexpected error: %v", tt.input, err)
			continue
		}
		if cidr.Family != tt.family {
			t.Errorf("ParseCIDR(%s).Family = %v, want %v", tt.input, cidr.Family, tt.family)
		}
	}
}

func TestCIDRContains(t *testing.T) {
	tests := []struct {
		cidr     string
		ip       string
		contains bool
	}{
		{"192.168.1.0/24", "192.168.1.100", true},
		{"192.168.1.0/24", "192.168.2.1", false},
		{"10.0.0.0/8", "10.255.255.255", true},
		{"10.0.0.0/8", "11.0.0.1", false},
		{"2001:db8::/32", "2001:db8::1", true},
		{"2001:db8::/32", "2001:db9::1", false},
	}

	for _, tt := range tests {
		cidr, err := ParseCIDR(tt.cidr)
		if err != nil {
			t.Errorf("ParseCIDR(%s) error: %v", tt.cidr, err)
			continue
		}
		ip := net.ParseIP(tt.ip)
		got := cidr.Contains(ip)
		if got != tt.contains {
			t.Errorf("CIDR(%s).Contains(%s) = %v, want %v", tt.cidr, tt.ip, got, tt.contains)
		}
	}
}

func TestValidateAddress(t *testing.T) {
	tests := []struct {
		input    string
		hasError bool
	}{
		{"192.168.1.1", false},
		{"::1", false},
		{"2001:db8::1", false},
		{"[::1]", false},
		{"fe80::1%eth0", false},
		{"invalid", true},
		{"256.256.256.256", true},
	}

	for _, tt := range tests {
		err := ValidateAddress(tt.input)
		if tt.hasError && err == nil {
			t.Errorf("ValidateAddress(%s) expected error, got nil", tt.input)
		}
		if !tt.hasError && err != nil {
			t.Errorf("ValidateAddress(%s) unexpected error: %v", tt.input, err)
		}
	}
}

func TestValidateIPv6Address(t *testing.T) {
	tests := []struct {
		input    string
		hasError bool
	}{
		{"::1", false},
		{"2001:db8::1", false},
		{"[2001:db8::1]", false},
		{"fe80::1%eth0", false},
		{"192.168.1.1", true}, // Not IPv6
		{"invalid", true},
	}

	for _, tt := range tests {
		err := ValidateIPv6Address(tt.input)
		if tt.hasError && err == nil {
			t.Errorf("ValidateIPv6Address(%s) expected error, got nil", tt.input)
		}
		if !tt.hasError && err != nil {
			t.Errorf("ValidateIPv6Address(%s) unexpected error: %v", tt.input, err)
		}
	}
}

func TestIsIPv4Address(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"192.168.1.1", true},
		{"127.0.0.1", true},
		{"0.0.0.0", true},
		{"::1", false},
		{"2001:db8::1", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		got := IsIPv4Address(tt.input)
		if got != tt.expected {
			t.Errorf("IsIPv4Address(%s) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestIsIPv6Address(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"::1", true},
		{"2001:db8::1", true},
		{"[2001:db8::1]", true},
		{"::", true},
		{"192.168.1.1", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		got := IsIPv6Address(tt.input)
		if got != tt.expected {
			t.Errorf("IsIPv6Address(%s) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestNormalizeIPv6(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"2001:0db8:0000:0000:0000:0000:0000:0001", "2001:db8::1"},
		{"[2001:0db8::1]", "[2001:db8::1]"},
		{"::1", "::1"},
		{"::", "::"},
		{"fe80::1%eth0", "fe80::1%eth0"},
		{"invalid", "invalid"},
	}

	for _, tt := range tests {
		got := NormalizeIPv6(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizeIPv6(%s) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}

func TestExpandIPv6(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"::1", "0000:0000:0000:0000:0000:0000:0000:0001"},
		{"::", "0000:0000:0000:0000:0000:0000:0000:0000"},
		{"2001:db8::1", "2001:0db8:0000:0000:0000:0000:0000:0001"},
		{"[::1]", "0000:0000:0000:0000:0000:0000:0000:0001"},
		{"192.168.1.1", "192.168.1.1"}, // IPv4 unchanged
		{"invalid", "invalid"},
	}

	for _, tt := range tests {
		got := ExpandIPv6(tt.input)
		if got != tt.expected {
			t.Errorf("ExpandIPv6(%s) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}

func TestMustFunctions(t *testing.T) {
	// Test MustParseAddress
	defer func() {
		if r := recover(); r != nil {
			t.Error("MustParseAddress should not panic for valid address")
		}
	}()
	addr := MustParseAddress("192.168.1.1:8080")
	if addr.Host != "192.168.1.1" {
		t.Error("MustParseAddress returned wrong host")
	}

	// Test MustParseURL
	url := MustParseURL("nats://localhost:4222")
	if url.Port != 4222 {
		t.Error("MustParseURL returned wrong port")
	}

	// Test MustParseCIDR
	cidr := MustParseCIDR("192.168.0.0/16")
	if cidr.Family != FamilyIPv4 {
		t.Error("MustParseCIDR returned wrong family")
	}
}

func TestMustFunctionsPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustParseAddress should panic for invalid address")
		}
	}()
	MustParseAddress("") // Should panic
}
