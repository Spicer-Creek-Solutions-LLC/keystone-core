// Copyright 2026 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/shawnbutts/keystone-core/internal/netutil"
)

// ListenerConfig holds configuration for creating network listeners.
type ListenerConfig struct {
	// Addresses to bind to (can include ports like "0.0.0.0:8080" or "[::]:8080")
	Addresses []string
	// DefaultPort is used when address doesn't include a port
	DefaultPort int
	// AddressFamily preference for selecting addresses
	AddressFamily netutil.AddressFamilyPreference
}

// ListenerResult contains information about a created listener.
type ListenerResult struct {
	// Listener is the network listener
	Listener net.Listener
	// Address is the actual address being listened on
	Address string
	// IsIPv6 indicates if this is an IPv6 listener
	IsIPv6 bool
}

// CreateListeners creates network listeners for the configured addresses.
// For dual-stack support, it can create multiple listeners (one for IPv4, one for IPv6).
// Returns the list of listeners and any error.
func CreateListeners(cfg *ListenerConfig) ([]*ListenerResult, error) {
	if len(cfg.Addresses) == 0 {
		return nil, fmt.Errorf("no addresses configured")
	}

	var results []*ListenerResult

	for _, addr := range cfg.Addresses {
		listenAddr := formatListenAddress(addr, cfg.DefaultPort)

		// Determine network type - use tcp for both IPv4 and IPv6
		// The address format tells Go which protocol to use
		network := "tcp"

		listener, err := (&net.ListenConfig{}).Listen(context.Background(), network, listenAddr)
		if err != nil {
			// Close any already created listeners
			for _, r := range results {
				r.Listener.Close()
			}
			return nil, fmt.Errorf("failed to listen on %s: %w", listenAddr, err)
		}

		// Determine if this is IPv6
		parsedAddr, _ := netutil.ParseAddress(addr)
		isIPv6 := parsedAddr != nil && parsedAddr.IsIPv6()

		results = append(results, &ListenerResult{
			Listener: listener,
			Address:  listener.Addr().String(),
			IsIPv6:   isIPv6,
		})
	}

	return results, nil
}

// CreateListener creates a single network listener for the first address.
// This is a convenience function for simple cases.
func CreateListener(cfg *ListenerConfig) (*ListenerResult, error) {
	results, err := CreateListeners(cfg)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no listeners created")
	}
	return results[0], nil
}

// CreateDualStackListeners creates listeners for both IPv4 and IPv6 if the
// configuration supports dual-stack mode.
func CreateDualStackListeners(port int, family netutil.AddressFamilyPreference) ([]*ListenerResult, error) {
	var addresses []string

	switch family {
	case netutil.IPv4Only:
		addresses = []string{"0.0.0.0"}
	case netutil.IPv6Only:
		addresses = []string{"::"}
	case netutil.PreferIPv4, netutil.PreferIPv6:
		// Dual-stack: bind to both
		addresses = []string{"0.0.0.0", "::"}
	default:
		// Default to IPv4 only for backwards compatibility
		addresses = []string{"0.0.0.0"}
	}

	return CreateListeners(&ListenerConfig{
		Addresses:     addresses,
		DefaultPort:   port,
		AddressFamily: family,
	})
}

// formatListenAddress formats an address for net.Listen.
// It handles various formats:
// - "0.0.0.0" -> "0.0.0.0:port"
// - "0.0.0.0:8080" -> "0.0.0.0:8080"
// - "::" -> "[::]:port"
// - "[::]:8080" -> "[::]:8080"
// - "::1" -> "[::1]:port"
func formatListenAddress(addr string, defaultPort int) string {
	addr = strings.TrimSpace(addr)

	// Check if address already has a port
	if hasPort(addr) {
		// Ensure IPv6 addresses are properly bracketed
		return ensureIPv6Brackets(addr)
	}

	// No port, need to add one
	if isIPv6Address(addr) {
		// IPv6 address needs brackets
		addr = strings.Trim(addr, "[]")
		return fmt.Sprintf("[%s]:%d", addr, defaultPort)
	}

	// IPv4 address or hostname
	return fmt.Sprintf("%s:%d", addr, defaultPort)
}

// hasPort checks if an address already includes a port.
func hasPort(addr string) bool {
	// For IPv6 with brackets, port is after the ]
	if strings.Contains(addr, "]:") {
		return true
	}

	// If it has brackets but no ]:, it's IPv6 without port
	if strings.HasPrefix(addr, "[") {
		return false
	}

	// Count colons to detect IPv6 vs IPv4:port
	colonCount := strings.Count(addr, ":")

	// No colons = no port
	if colonCount == 0 {
		return false
	}

	// Multiple colons without brackets = IPv6 without port (e.g., "::1", "2001:db8::1")
	if colonCount > 1 {
		return false
	}

	// Exactly one colon and no brackets = IPv4:port or hostname:port
	// Make sure there's something after the colon
	lastColon := strings.LastIndex(addr, ":")
	return lastColon > 0 && lastColon < len(addr)-1
}

// isIPv6Address checks if an address is IPv6.
func isIPv6Address(addr string) bool {
	// Remove brackets if present
	addr = strings.Trim(addr, "[]")

	// Check for port suffix and remove it
	if strings.Contains(addr, "]:") {
		addr = strings.Split(addr, "]:")[0]
		addr = strings.TrimPrefix(addr, "[")
	}

	// Contains multiple colons - likely IPv6
	if strings.Count(addr, ":") > 1 {
		return true
	}

	// Try parsing as IP
	ip := net.ParseIP(addr)
	return ip != nil && ip.To4() == nil
}

// ensureIPv6Brackets ensures IPv6 addresses in host:port format have proper brackets.
func ensureIPv6Brackets(addr string) string {
	// Already properly formatted
	if strings.HasPrefix(addr, "[") {
		return addr
	}

	// Count colons to detect IPv6
	colonCount := strings.Count(addr, ":")
	if colonCount <= 1 {
		// IPv4 or hostname with port
		return addr
	}

	// IPv6 address - need to figure out where the port is
	// Find the last : that separates the port
	lastColon := strings.LastIndex(addr, ":")

	// Check if everything before the last colon is a valid IPv6
	possibleIP := addr[:lastColon]
	possiblePort := addr[lastColon+1:]

	ip := net.ParseIP(possibleIP)
	if ip != nil && ip.To4() == nil {
		// It's IPv6, add brackets
		return fmt.Sprintf("[%s]:%s", possibleIP, possiblePort)
	}

	// Not a clean split, might be full IPv6 without port
	return addr
}

// SelectPreferredAddress selects the preferred address based on address family preference.
func SelectPreferredAddress(addresses []string, pref netutil.AddressFamilyPreference) string {
	if len(addresses) == 0 {
		return ""
	}

	var ipv4, ipv6 string

	for _, addr := range addresses {
		parsed, _ := netutil.ParseAddress(addr)
		if parsed == nil {
			continue
		}
		if parsed.IsIPv6() && ipv6 == "" {
			ipv6 = addr
		} else if parsed.IsIPv4() && ipv4 == "" {
			ipv4 = addr
		}
	}

	switch pref {
	case netutil.IPv6Only:
		return ipv6
	case netutil.IPv4Only:
		return ipv4
	case netutil.PreferIPv6:
		if ipv6 != "" {
			return ipv6
		}
		return ipv4
	default: // includes netutil.PreferIPv4
		if ipv4 != "" {
			return ipv4
		}
		return ipv6
	}
}
