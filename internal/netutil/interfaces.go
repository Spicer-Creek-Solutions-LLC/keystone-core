// Copyright 2024 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

package netutil

import (
	"context"
	"net"
	"sort"
	"strings"
)

// NetworkInfo contains information about available network addresses.
type NetworkInfo struct {
	// IPv4Addresses contains all non-loopback IPv4 addresses.
	IPv4Addresses []string
	// IPv6Addresses contains all non-loopback, non-link-local IPv6 addresses.
	IPv6Addresses []string
	// LinkLocalIPv6 contains link-local IPv6 addresses (fe80::/10).
	LinkLocalIPv6 []string
	// LoopbackIPv4 is the loopback IPv4 address if present.
	LoopbackIPv4 string
	// LoopbackIPv6 is the loopback IPv6 address if present.
	LoopbackIPv6 string
	// PrimaryIPv4 is the best IPv4 address for external communication.
	PrimaryIPv4 string
	// PrimaryIPv6 is the best IPv6 address for external communication.
	PrimaryIPv6 string
	// Hostname is the system hostname.
	Hostname string
	// HasIPv4 indicates if IPv4 is available (non-loopback).
	HasIPv4 bool
	// HasIPv6 indicates if IPv6 is available (non-loopback, non-link-local).
	HasIPv6 bool
}

// PreferredAddress returns the preferred address based on the preference.
func (n *NetworkInfo) PreferredAddress(pref AddressFamilyPreference) string {
	switch pref {
	case IPv6Only:
		if n.PrimaryIPv6 != "" {
			return n.PrimaryIPv6
		}
		return ""
	case IPv4Only:
		if n.PrimaryIPv4 != "" {
			return n.PrimaryIPv4
		}
		return ""
	case PreferIPv6:
		if n.PrimaryIPv6 != "" {
			return n.PrimaryIPv6
		}
		if n.PrimaryIPv4 != "" {
			return n.PrimaryIPv4
		}
		return ""
	default: // includes PreferIPv4
		if n.PrimaryIPv4 != "" {
			return n.PrimaryIPv4
		}
		if n.PrimaryIPv6 != "" {
			return n.PrimaryIPv6
		}
		return ""
	}
}

// AllAddresses returns all available addresses (IPv4 and IPv6).
func (n *NetworkInfo) AllAddresses() []string {
	all := make([]string, 0, len(n.IPv4Addresses)+len(n.IPv6Addresses))
	all = append(all, n.IPv4Addresses...)
	all = append(all, n.IPv6Addresses...)
	return all
}

// GetNetworkInfo detects all network interfaces and their addresses.
func GetNetworkInfo() (*NetworkInfo, error) {
	info := &NetworkInfo{
		IPv4Addresses: make([]string, 0),
		IPv6Addresses: make([]string, 0),
		LinkLocalIPv6: make([]string, 0),
	}

	// Get hostname
	hostname, _ := getHostname()
	info.Hostname = hostname

	// Get all network interfaces
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		// Skip down interfaces
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			default:
				continue
			}

			if ip == nil {
				continue
			}

			ipStr := ip.String()

			// Handle IPv4
			if ip4 := ip.To4(); ip4 != nil {
				if ip.IsLoopback() {
					info.LoopbackIPv4 = ipStr
				} else {
					info.IPv4Addresses = append(info.IPv4Addresses, ipStr)
				}
				continue
			}

			// Handle IPv6
			switch {
			case ip.IsLoopback():
				info.LoopbackIPv6 = ipStr
			case ip.IsLinkLocalUnicast():
				// Include zone ID for link-local addresses
				if iface.Name != "" {
					info.LinkLocalIPv6 = append(info.LinkLocalIPv6, ipStr+"%"+iface.Name)
				} else {
					info.LinkLocalIPv6 = append(info.LinkLocalIPv6, ipStr)
				}
			default:
				info.IPv6Addresses = append(info.IPv6Addresses, ipStr)
			}
		}
	}

	// Sort addresses for consistent ordering
	sort.Strings(info.IPv4Addresses)
	sort.Strings(info.IPv6Addresses)
	sort.Strings(info.LinkLocalIPv6)

	// Set primary addresses
	info.PrimaryIPv4 = selectPrimaryIPv4(info.IPv4Addresses)
	info.PrimaryIPv6 = selectPrimaryIPv6(info.IPv6Addresses)

	// Set availability flags
	info.HasIPv4 = len(info.IPv4Addresses) > 0
	info.HasIPv6 = len(info.IPv6Addresses) > 0

	return info, nil
}

// getHostname returns the system hostname.
func getHostname() (string, error) {
	// This is a simple wrapper that can be mocked in tests
	return getHostnameImpl()
}

// getHostnameImpl is the actual implementation.
var getHostnameImpl = func() (string, error) {
	return "", nil // Will be replaced with os.Hostname() when needed
}

// selectPrimaryIPv4 selects the best IPv4 address for external communication.
// Prefers non-RFC1918 (public) addresses, then private addresses.
func selectPrimaryIPv4(addresses []string) string {
	if len(addresses) == 0 {
		return ""
	}

	// Try to find a public (non-RFC1918) address first
	for _, addr := range addresses {
		ip := net.ParseIP(addr)
		if ip != nil && !isPrivateIPv4(ip) {
			return addr
		}
	}

	// Fall back to first private address
	return addresses[0]
}

// isPrivateIPv4 checks if an IPv4 address is in RFC1918 private ranges.
func isPrivateIPv4(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}

	// 10.0.0.0/8
	if ip4[0] == 10 {
		return true
	}
	// 172.16.0.0/12
	if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
		return true
	}
	// 192.168.0.0/16
	if ip4[0] == 192 && ip4[1] == 168 {
		return true
	}
	// 169.254.0.0/16 (link-local)
	if ip4[0] == 169 && ip4[1] == 254 {
		return true
	}

	return false
}

// selectPrimaryIPv6 selects the best IPv6 address for external communication.
// Prefers global unicast addresses, then unique local addresses.
func selectPrimaryIPv6(addresses []string) string {
	if len(addresses) == 0 {
		return ""
	}

	// Try to find a global unicast address (2000::/3)
	for _, addr := range addresses {
		ip := net.ParseIP(addr)
		if ip != nil && isGlobalUnicastIPv6(ip) {
			return addr
		}
	}

	// Fall back to first address (might be unique local fc00::/7)
	return addresses[0]
}

// isGlobalUnicastIPv6 checks if an IPv6 address is a global unicast address.
func isGlobalUnicastIPv6(ip net.IP) bool {
	if ip == nil || ip.To4() != nil {
		return false
	}
	// Global unicast: 2000::/3 (first byte 001x xxxx = 0x20-0x3f)
	return len(ip) == 16 && ip[0] >= 0x20 && ip[0] <= 0x3f
}

// InterfaceAddresses represents addresses for a single network interface.
type InterfaceAddresses struct {
	// Name is the interface name (e.g., "eth0", "en0").
	Name string
	// Index is the interface index.
	Index int
	// HardwareAddr is the MAC address.
	HardwareAddr string
	// Flags are the interface flags.
	Flags net.Flags
	// IPv4 contains IPv4 addresses with CIDR notation.
	IPv4 []string
	// IPv6 contains IPv6 addresses with CIDR notation.
	IPv6 []string
}

// GetInterfaceAddresses returns detailed address information for all interfaces.
func GetInterfaceAddresses() ([]InterfaceAddresses, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var result []InterfaceAddresses

	for _, iface := range ifaces {
		ia := InterfaceAddresses{
			Name:         iface.Name,
			Index:        iface.Index,
			HardwareAddr: iface.HardwareAddr.String(),
			Flags:        iface.Flags,
			IPv4:         make([]string, 0),
			IPv6:         make([]string, 0),
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			// We want the CIDR notation for interface listing
			addrStr := addr.String()

			// Determine if IPv4 or IPv6
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil {
				continue
			}

			if ip.To4() != nil {
				ia.IPv4 = append(ia.IPv4, addrStr)
			} else {
				// For IPv6 link-local, add interface name as zone
				if ip.IsLinkLocalUnicast() && !strings.Contains(addrStr, "%") {
					addrStr = strings.Replace(addrStr, ip.String(), ip.String()+"%"+iface.Name, 1)
				}
				ia.IPv6 = append(ia.IPv6, addrStr)
			}
		}

		result = append(result, ia)
	}

	return result, nil
}

// ResolveHostAddresses resolves a hostname to both IPv4 and IPv6 addresses.
func ResolveHostAddresses(host string) (*NetworkInfo, error) {
	info := &NetworkInfo{
		Hostname:      host,
		IPv4Addresses: make([]string, 0),
		IPv6Addresses: make([]string, 0),
	}

	// Try to resolve the hostname
	resolver := &net.Resolver{}
	ipAddrs, err := resolver.LookupIPAddr(context.Background(), host)
	if err != nil {
		return nil, err
	}

	for _, ipAddr := range ipAddrs {
		ipStr := ipAddr.IP.String()
		addr := ipAddr.IP
		if addr.To4() != nil {
			info.IPv4Addresses = append(info.IPv4Addresses, ipStr)
		} else {
			info.IPv6Addresses = append(info.IPv6Addresses, ipStr)
		}
	}

	// Sort for consistent ordering
	sort.Strings(info.IPv4Addresses)
	sort.Strings(info.IPv6Addresses)

	// Set primary addresses
	if len(info.IPv4Addresses) > 0 {
		info.PrimaryIPv4 = info.IPv4Addresses[0]
		info.HasIPv4 = true
	}
	if len(info.IPv6Addresses) > 0 {
		info.PrimaryIPv6 = info.IPv6Addresses[0]
		info.HasIPv6 = true
	}

	return info, nil
}

// SelectAddressForConnection selects the best local address for connecting to a remote address.
func SelectAddressForConnection(remoteAddr *Address, localInfo *NetworkInfo, pref AddressFamilyPreference) string {
	// If remote is IPv6, prefer IPv6 local address
	if remoteAddr.IsIPv6() {
		switch pref {
		case IPv4Only:
			return "" // Can't connect to IPv6 with IPv4 only
		default:
			if localInfo.PrimaryIPv6 != "" {
				return localInfo.PrimaryIPv6
			}
		}
	}

	// If remote is IPv4, prefer IPv4 local address
	if remoteAddr.IsIPv4() {
		switch pref {
		case IPv6Only:
			return "" // Can't connect to IPv4 with IPv6 only
		default:
			if localInfo.PrimaryIPv4 != "" {
				return localInfo.PrimaryIPv4
			}
		}
	}

	// For hostname (unknown family), use preference
	return localInfo.PreferredAddress(pref)
}
