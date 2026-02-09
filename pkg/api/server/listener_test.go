// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"testing"

	"github.com/shawnbutts/keystone-core/internal/netutil"
)

func TestFormatListenAddress(t *testing.T) {
	tests := []struct {
		name        string
		addr        string
		defaultPort int
		expected    string
	}{
		{
			name:        "IPv4 without port",
			addr:        "0.0.0.0",
			defaultPort: 8080,
			expected:    "0.0.0.0:8080",
		},
		{
			name:        "IPv4 with port",
			addr:        "0.0.0.0:9090",
			defaultPort: 8080,
			expected:    "0.0.0.0:9090",
		},
		{
			name:        "IPv6 without port or brackets",
			addr:        "::",
			defaultPort: 8080,
			expected:    "[::]:8080",
		},
		{
			name:        "IPv6 with brackets without port",
			addr:        "[::]",
			defaultPort: 8080,
			expected:    "[::]:8080",
		},
		{
			name:        "IPv6 with brackets and port",
			addr:        "[::]:9090",
			defaultPort: 8080,
			expected:    "[::]:9090",
		},
		{
			name:        "IPv6 localhost without port",
			addr:        "::1",
			defaultPort: 8080,
			expected:    "[::1]:8080",
		},
		{
			name:        "IPv6 full address without port",
			addr:        "2001:db8::1",
			defaultPort: 8080,
			expected:    "[2001:db8::1]:8080",
		},
		{
			name:        "IPv6 full address with brackets and port",
			addr:        "[2001:db8::1]:9090",
			defaultPort: 8080,
			expected:    "[2001:db8::1]:9090",
		},
		{
			name:        "localhost without port",
			addr:        "127.0.0.1",
			defaultPort: 8080,
			expected:    "127.0.0.1:8080",
		},
		{
			name:        "whitespace trimming",
			addr:        "  0.0.0.0  ",
			defaultPort: 8080,
			expected:    "0.0.0.0:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatListenAddress(tt.addr, tt.defaultPort)
			if result != tt.expected {
				t.Errorf("formatListenAddress(%q, %d) = %q, want %q",
					tt.addr, tt.defaultPort, result, tt.expected)
			}
		})
	}
}

func TestHasPort(t *testing.T) {
	tests := []struct {
		addr     string
		expected bool
	}{
		{"0.0.0.0", false},
		{"0.0.0.0:8080", true},
		{"::", false},
		{"[::]", false},
		{"[::]:8080", true},
		{"::1", false},
		{"[::1]:8080", true},
		{"2001:db8::1", false},
		{"[2001:db8::1]:8080", true},
		{"localhost", false},
		{"localhost:8080", true},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			result := hasPort(tt.addr)
			if result != tt.expected {
				t.Errorf("hasPort(%q) = %v, want %v", tt.addr, result, tt.expected)
			}
		})
	}
}

func TestIsIPv6Address(t *testing.T) {
	tests := []struct {
		addr     string
		expected bool
	}{
		{"0.0.0.0", false},
		{"127.0.0.1", false},
		{"192.168.1.1", false},
		{"::", true},
		{"[::]", true},
		{"::1", true},
		{"[::1]", true},
		{"2001:db8::1", true},
		{"[2001:db8::1]", true},
		{"fe80::1%eth0", true},
		{"localhost", false},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			result := isIPv6Address(tt.addr)
			if result != tt.expected {
				t.Errorf("isIPv6Address(%q) = %v, want %v", tt.addr, result, tt.expected)
			}
		})
	}
}

func TestSelectPreferredAddress(t *testing.T) {
	tests := []struct {
		name      string
		addresses []string
		pref      netutil.AddressFamilyPreference
		expected  string
	}{
		{
			name:      "empty addresses",
			addresses: []string{},
			pref:      netutil.PreferIPv4,
			expected:  "",
		},
		{
			name:      "prefer IPv4 with both",
			addresses: []string{"192.168.1.1", "2001:db8::1"},
			pref:      netutil.PreferIPv4,
			expected:  "192.168.1.1",
		},
		{
			name:      "prefer IPv6 with both",
			addresses: []string{"192.168.1.1", "2001:db8::1"},
			pref:      netutil.PreferIPv6,
			expected:  "2001:db8::1",
		},
		{
			name:      "IPv4 only with both",
			addresses: []string{"192.168.1.1", "2001:db8::1"},
			pref:      netutil.IPv4Only,
			expected:  "192.168.1.1",
		},
		{
			name:      "IPv6 only with both",
			addresses: []string{"192.168.1.1", "2001:db8::1"},
			pref:      netutil.IPv6Only,
			expected:  "2001:db8::1",
		},
		{
			name:      "prefer IPv4 but only IPv6 available",
			addresses: []string{"2001:db8::1"},
			pref:      netutil.PreferIPv4,
			expected:  "2001:db8::1",
		},
		{
			name:      "prefer IPv6 but only IPv4 available",
			addresses: []string{"192.168.1.1"},
			pref:      netutil.PreferIPv6,
			expected:  "192.168.1.1",
		},
		{
			name:      "IPv4 only but only IPv6 available",
			addresses: []string{"2001:db8::1"},
			pref:      netutil.IPv4Only,
			expected:  "",
		},
		{
			name:      "IPv6 only but only IPv4 available",
			addresses: []string{"192.168.1.1"},
			pref:      netutil.IPv6Only,
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SelectPreferredAddress(tt.addresses, tt.pref)
			if result != tt.expected {
				t.Errorf("SelectPreferredAddress(%v, %v) = %q, want %q",
					tt.addresses, tt.pref, result, tt.expected)
			}
		})
	}
}

func TestCreateListener(t *testing.T) {
	// Test basic listener creation with IPv4
	cfg := &ListenerConfig{
		Addresses:   []string{"127.0.0.1"},
		DefaultPort: 0, // Use ephemeral port
	}

	result, err := CreateListener(cfg)
	if err != nil {
		t.Fatalf("CreateListener() error = %v", err)
	}
	defer result.Listener.Close()

	if result.Listener == nil {
		t.Error("CreateListener() returned nil listener")
	}
	if result.Address == "" {
		t.Error("CreateListener() returned empty address")
	}
	if result.IsIPv6 {
		t.Error("CreateListener() incorrectly marked IPv4 as IPv6")
	}
}

func TestCreateListenerIPv6(t *testing.T) {
	// Test listener creation with IPv6
	cfg := &ListenerConfig{
		Addresses:   []string{"::1"},
		DefaultPort: 0, // Use ephemeral port
	}

	result, err := CreateListener(cfg)
	if err != nil {
		t.Fatalf("CreateListener() error = %v", err)
	}
	defer result.Listener.Close()

	if result.Listener == nil {
		t.Error("CreateListener() returned nil listener")
	}
	if !result.IsIPv6 {
		t.Error("CreateListener() should mark ::1 as IPv6")
	}
}

func TestCreateListeners(t *testing.T) {
	// Test creating multiple listeners
	cfg := &ListenerConfig{
		Addresses:   []string{"127.0.0.1", "::1"},
		DefaultPort: 0, // Use ephemeral ports
	}

	results, err := CreateListeners(cfg)
	if err != nil {
		t.Fatalf("CreateListeners() error = %v", err)
	}
	defer func() {
		for _, r := range results {
			r.Listener.Close()
		}
	}()

	if len(results) != 2 {
		t.Errorf("CreateListeners() returned %d listeners, want 2", len(results))
	}

	// Check we have one IPv4 and one IPv6
	hasIPv4, hasIPv6 := false, false
	for _, r := range results {
		if r.IsIPv6 {
			hasIPv6 = true
		} else {
			hasIPv4 = true
		}
	}

	if !hasIPv4 {
		t.Error("CreateListeners() should have created an IPv4 listener")
	}
	if !hasIPv6 {
		t.Error("CreateListeners() should have created an IPv6 listener")
	}
}

func TestCreateListenersError(t *testing.T) {
	// Test error handling - empty addresses
	cfg := &ListenerConfig{
		Addresses:   []string{},
		DefaultPort: 8080,
	}

	_, err := CreateListeners(cfg)
	if err == nil {
		t.Error("CreateListeners() should return error for empty addresses")
	}
}

func TestCreateDualStackListeners(t *testing.T) {
	tests := []struct {
		name          string
		family        netutil.AddressFamilyPreference
		expectedCount int
		expectedIPv4  bool
		expectedIPv6  bool
	}{
		{
			name:          "IPv4 only",
			family:        netutil.IPv4Only,
			expectedCount: 1,
			expectedIPv4:  true,
			expectedIPv6:  false,
		},
		{
			name:          "IPv6 only",
			family:        netutil.IPv6Only,
			expectedCount: 1,
			expectedIPv4:  false,
			expectedIPv6:  true,
		},
		{
			name:          "prefer IPv4 (dual-stack)",
			family:        netutil.PreferIPv4,
			expectedCount: 2,
			expectedIPv4:  true,
			expectedIPv6:  true,
		},
		{
			name:          "prefer IPv6 (dual-stack)",
			family:        netutil.PreferIPv6,
			expectedCount: 2,
			expectedIPv4:  true,
			expectedIPv6:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := CreateDualStackListeners(0, tt.family) // Use ephemeral port
			if err != nil {
				t.Fatalf("CreateDualStackListeners() error = %v", err)
			}
			defer func() {
				for _, r := range results {
					r.Listener.Close()
				}
			}()

			if len(results) != tt.expectedCount {
				t.Errorf("CreateDualStackListeners() returned %d listeners, want %d",
					len(results), tt.expectedCount)
			}

			hasIPv4, hasIPv6 := false, false
			for _, r := range results {
				if r.IsIPv6 {
					hasIPv6 = true
				} else {
					hasIPv4 = true
				}
			}

			if hasIPv4 != tt.expectedIPv4 {
				t.Errorf("CreateDualStackListeners() hasIPv4 = %v, want %v", hasIPv4, tt.expectedIPv4)
			}
			if hasIPv6 != tt.expectedIPv6 {
				t.Errorf("CreateDualStackListeners() hasIPv6 = %v, want %v", hasIPv6, tt.expectedIPv6)
			}
		})
	}
}
