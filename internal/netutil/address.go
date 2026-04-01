// Copyright 2024 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

// Package netutil provides network address parsing utilities with full IPv6 support.
package netutil

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// AddressFamily represents the IP address family.
type AddressFamily int

const (
	// FamilyUnknown indicates an unknown address family.
	FamilyUnknown AddressFamily = iota
	// FamilyIPv4 indicates IPv4 only.
	FamilyIPv4
	// FamilyIPv6 indicates IPv6 only.
	FamilyIPv6
	// FamilyDualStack indicates both IPv4 and IPv6.
	FamilyDualStack
)

// String returns the string representation of the address family.
func (f AddressFamily) String() string {
	switch f {
	case FamilyIPv4:
		return "ipv4"
	case FamilyIPv6:
		return "ipv6"
	case FamilyDualStack:
		return "dual-stack"
	default:
		return "unknown"
	}
}

// ParseAddressFamily parses a string into an AddressFamily.
func ParseAddressFamily(s string) AddressFamily {
	switch strings.ToLower(s) {
	case "ipv4", "ip4", "4":
		return FamilyIPv4
	case "ipv6", "ip6", "6":
		return FamilyIPv6
	case "dual-stack", "dualstack", "dual", "both":
		return FamilyDualStack
	default:
		return FamilyUnknown
	}
}

// AddressFamilyPreference represents the preference for address family selection.
type AddressFamilyPreference int

const (
	// PreferIPv4 prefers IPv4 but allows IPv6 fallback.
	PreferIPv4 AddressFamilyPreference = iota
	// PreferIPv6 prefers IPv6 but allows IPv4 fallback.
	PreferIPv6
	// IPv4Only uses only IPv4 addresses.
	IPv4Only
	// IPv6Only uses only IPv6 addresses.
	IPv6Only
)

// String returns the string representation of the preference.
func (p AddressFamilyPreference) String() string {
	switch p {
	case PreferIPv4:
		return "prefer_ipv4"
	case PreferIPv6:
		return "prefer_ipv6"
	case IPv4Only:
		return "ipv4_only"
	case IPv6Only:
		return "ipv6_only"
	default:
		return "unknown"
	}
}

// ParseAddressFamilyPreference parses a string into an AddressFamilyPreference.
func ParseAddressFamilyPreference(s string) (AddressFamilyPreference, error) {
	switch strings.ToLower(strings.ReplaceAll(s, "-", "_")) {
	case "prefer_ipv4", "preferipv4":
		return PreferIPv4, nil
	case "prefer_ipv6", "preferipv6":
		return PreferIPv6, nil
	case "ipv4_only", "ipv4only", "ipv4":
		return IPv4Only, nil
	case "ipv6_only", "ipv6only", "ipv6":
		return IPv6Only, nil
	default:
		return PreferIPv4, fmt.Errorf("unknown address family preference: %s", s)
	}
}

// Address represents a parsed network address with IP and port.
type Address struct {
	// Host is the IP address or hostname.
	Host string
	// Port is the port number (0 if not specified).
	Port int
	// IP is the parsed IP address (nil if Host is a hostname).
	IP net.IP
	// Family is the detected address family.
	Family AddressFamily
	// Zone is the IPv6 zone ID (e.g., "eth0" in fe80::1%eth0).
	Zone string
	// Original is the original input string.
	Original string
}

// IsIPv4 returns true if this is an IPv4 address.
func (a *Address) IsIPv4() bool {
	return a.Family == FamilyIPv4
}

// IsIPv6 returns true if this is an IPv6 address.
func (a *Address) IsIPv6() bool {
	return a.Family == FamilyIPv6
}

// IsLoopback returns true if this is a loopback address.
func (a *Address) IsLoopback() bool {
	if a.IP != nil {
		return a.IP.IsLoopback()
	}
	return a.Host == "localhost" || a.Host == "127.0.0.1" || a.Host == "::1"
}

// IsUnspecified returns true if this is an unspecified (all interfaces) address.
func (a *Address) IsUnspecified() bool {
	if a.IP != nil {
		return a.IP.IsUnspecified()
	}
	return a.Host == "0.0.0.0" || a.Host == "::" || a.Host == ""
}

// IsLinkLocal returns true if this is a link-local address.
func (a *Address) IsLinkLocal() bool {
	if a.IP != nil {
		return a.IP.IsLinkLocalUnicast() || a.IP.IsLinkLocalMulticast()
	}
	return false
}

// String returns the address as a string suitable for use in URLs.
// IPv6 addresses are returned with brackets.
func (a *Address) String() string {
	if a.Port == 0 {
		return a.HostString()
	}
	return net.JoinHostPort(a.HostString(), strconv.Itoa(a.Port))
}

// HostString returns the host portion of the address.
// IPv6 addresses are NOT bracketed (use String() for URL-safe format).
func (a *Address) HostString() string {
	if a.Zone != "" && a.IP != nil {
		return a.IP.String() + "%" + a.Zone
	}
	return a.Host
}

// HostPort returns host:port format suitable for net.Dial.
func (a *Address) HostPort() string {
	if a.Port == 0 {
		return a.Host
	}
	return net.JoinHostPort(a.Host, strconv.Itoa(a.Port))
}

// Network returns the network type ("tcp4", "tcp6", or "tcp").
func (a *Address) Network() string {
	switch a.Family {
	case FamilyIPv4:
		return "tcp4"
	case FamilyIPv6:
		return "tcp6"
	default:
		return "tcp"
	}
}

// ParseAddress parses an address string into an Address struct.
// Supports formats:
//   - "host:port" (IPv4 or hostname)
//   - "[host]:port" (IPv6)
//   - "host" (no port)
//   - "[host]" (IPv6 no port)
//   - "::1" (IPv6 loopback)
//   - "::" (IPv6 all interfaces)
//   - "0.0.0.0" (IPv4 all interfaces)
func ParseAddress(addr string) (*Address, error) {
	if addr == "" {
		return nil, fmt.Errorf("empty address")
	}

	// Validate bracket matching for IPv6 addresses
	openBracket := strings.Count(addr, "[")
	closeBracket := strings.Count(addr, "]")
	if openBracket != closeBracket {
		return nil, fmt.Errorf("mismatched brackets in address: %s", addr)
	}
	if openBracket > 1 || closeBracket > 1 {
		return nil, fmt.Errorf("invalid bracket usage in address: %s", addr)
	}
	// If there's an open bracket, it must be first and close bracket must be after it
	if openBracket == 1 {
		openIdx := strings.Index(addr, "[")
		closeIdx := strings.Index(addr, "]")
		if openIdx != 0 || closeIdx < openIdx {
			return nil, fmt.Errorf("invalid bracket position in address: %s", addr)
		}
	}

	result := &Address{
		Original: addr,
	}

	// Try to parse as host:port first
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		// No port specified, treat entire string as host
		host = addr
		portStr = ""

		// Handle IPv6 addresses with brackets but no port
		if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
			host = host[1 : len(host)-1]
		}
	}

	// Parse port
	if portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("invalid port: %s", portStr)
		}
		if port < 0 || port > 65535 {
			return nil, fmt.Errorf("port out of range: %d", port)
		}
		result.Port = port
	}

	// Check for zone ID (IPv6 link-local)
	if idx := strings.Index(host, "%"); idx != -1 {
		result.Zone = host[idx+1:]
		host = host[:idx]
	}

	result.Host = host

	// Try to parse as IP address
	if ip := net.ParseIP(host); ip != nil {
		result.IP = ip
		if ip.To4() != nil {
			result.Family = FamilyIPv4
		} else {
			result.Family = FamilyIPv6
		}
	} else {
		// It's a hostname - family is unknown until resolved
		result.Family = FamilyUnknown
	}

	return result, nil
}

// ParseURL parses a URL string and extracts the address.
// Supports formats:
//   - "nats://host:port"
//   - "nats://[ipv6]:port"
//   - "grpc://host:port"
//   - "https://[ipv6]:port/path"
func ParseURL(urlStr string) (*Address, error) {
	if urlStr == "" {
		return nil, fmt.Errorf("empty URL")
	}

	// Handle URLs without scheme
	if !strings.Contains(urlStr, "://") {
		return ParseAddress(urlStr)
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	result := &Address{
		Original: urlStr,
		Host:     u.Hostname(),
	}

	// Parse port
	if portStr := u.Port(); portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("invalid port: %s", portStr)
		}
		result.Port = port
	}

	// Check for zone ID
	host := result.Host
	if idx := strings.Index(host, "%"); idx != -1 {
		result.Zone = host[idx+1:]
		host = host[:idx]
		result.Host = host
	}

	// Try to parse as IP
	if ip := net.ParseIP(host); ip != nil {
		result.IP = ip
		if ip.To4() != nil {
			result.Family = FamilyIPv4
		} else {
			result.Family = FamilyIPv6
		}
	}

	return result, nil
}

// MustParseAddress parses an address or panics.
func MustParseAddress(addr string) *Address {
	a, err := ParseAddress(addr)
	if err != nil {
		panic(err)
	}
	return a
}

// MustParseURL parses a URL or panics.
func MustParseURL(urlStr string) *Address {
	a, err := ParseURL(urlStr)
	if err != nil {
		panic(err)
	}
	return a
}

// FormatAddress formats an address for use in a URL.
// IPv6 addresses are bracketed, IPv4 addresses are not.
func FormatAddress(host string, port int) string {
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		// IPv6 - needs brackets
		if port > 0 {
			return fmt.Sprintf("[%s]:%d", host, port)
		}
		return fmt.Sprintf("[%s]", host)
	}
	// IPv4 or hostname
	if port > 0 {
		return fmt.Sprintf("%s:%d", host, port)
	}
	return host
}

// FormatURL formats an address as a URL with the given scheme.
func FormatURL(scheme, host string, port int, path string) string {
	addr := FormatAddress(host, port)
	if path != "" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return fmt.Sprintf("%s://%s%s", scheme, addr, path)
}

// CIDR represents a network CIDR (e.g., 192.168.1.0/24 or 2001:db8::/32).
type CIDR struct {
	// Network is the parsed network.
	Network *net.IPNet
	// Family is the address family.
	Family AddressFamily
	// Original is the original input string.
	Original string
}

// Contains returns true if the CIDR contains the given IP address.
func (c *CIDR) Contains(ip net.IP) bool {
	if c.Network == nil {
		return false
	}
	return c.Network.Contains(ip)
}

// ContainsAddress returns true if the CIDR contains the given Address.
func (c *CIDR) ContainsAddress(addr *Address) bool {
	if addr.IP == nil {
		return false
	}
	return c.Contains(addr.IP)
}

// String returns the CIDR in standard notation.
func (c *CIDR) String() string {
	if c.Network == nil {
		return c.Original
	}
	return c.Network.String()
}

// ParseCIDR parses a CIDR notation string.
func ParseCIDR(cidr string) (*CIDR, error) {
	if cidr == "" {
		return nil, fmt.Errorf("empty CIDR")
	}

	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR: %w", err)
	}

	result := &CIDR{
		Network:  network,
		Original: cidr,
	}

	// Determine family
	if network.IP.To4() != nil {
		result.Family = FamilyIPv4
	} else {
		result.Family = FamilyIPv6
	}

	return result, nil
}

// MustParseCIDR parses a CIDR or panics.
func MustParseCIDR(cidr string) *CIDR {
	c, err := ParseCIDR(cidr)
	if err != nil {
		panic(err)
	}
	return c
}

// ValidateIPv6Address validates that a string is a valid IPv6 address.
func ValidateIPv6Address(addr string) error {
	// Remove brackets if present
	if strings.HasPrefix(addr, "[") && strings.HasSuffix(addr, "]") {
		addr = addr[1 : len(addr)-1]
	}

	// Remove zone ID if present
	if idx := strings.Index(addr, "%"); idx != -1 {
		addr = addr[:idx]
	}

	ip := net.ParseIP(addr)
	if ip == nil {
		return fmt.Errorf("invalid IP address: %s", addr)
	}

	if ip.To4() != nil {
		return fmt.Errorf("not an IPv6 address: %s", addr)
	}

	return nil
}

// ValidateIPv4Address validates that a string is a valid IPv4 address.
func ValidateIPv4Address(addr string) error {
	ip := net.ParseIP(addr)
	if ip == nil {
		return fmt.Errorf("invalid IP address: %s", addr)
	}

	if ip.To4() == nil {
		return fmt.Errorf("not an IPv4 address: %s", addr)
	}

	return nil
}

// ValidateAddress validates that a string is a valid IP address (IPv4 or IPv6).
func ValidateAddress(addr string) error {
	// Remove brackets if present
	if strings.HasPrefix(addr, "[") && strings.HasSuffix(addr, "]") {
		addr = addr[1 : len(addr)-1]
	}

	// Remove zone ID if present
	if idx := strings.Index(addr, "%"); idx != -1 {
		addr = addr[:idx]
	}

	if net.ParseIP(addr) == nil {
		return fmt.Errorf("invalid IP address: %s", addr)
	}

	return nil
}

// IsIPv4Address returns true if the string is a valid IPv4 address.
func IsIPv4Address(addr string) bool {
	ip := net.ParseIP(addr)
	return ip != nil && ip.To4() != nil
}

// IsIPv6Address returns true if the string is a valid IPv6 address.
func IsIPv6Address(addr string) bool {
	// Remove brackets if present
	if strings.HasPrefix(addr, "[") && strings.HasSuffix(addr, "]") {
		addr = addr[1 : len(addr)-1]
	}

	// Remove zone ID if present
	if idx := strings.Index(addr, "%"); idx != -1 {
		addr = addr[:idx]
	}

	ip := net.ParseIP(addr)
	return ip != nil && ip.To4() == nil
}

// NormalizeIPv6 normalizes an IPv6 address to standard form.
// Returns the address unchanged if not a valid IPv6 address.
func NormalizeIPv6(addr string) string {
	// Remove brackets if present
	hasBrackets := false
	if strings.HasPrefix(addr, "[") && strings.HasSuffix(addr, "]") {
		addr = addr[1 : len(addr)-1]
		hasBrackets = true
	}

	// Extract zone ID if present
	zone := ""
	if idx := strings.Index(addr, "%"); idx != -1 {
		zone = addr[idx:]
		addr = addr[:idx]
	}

	ip := net.ParseIP(addr)
	if ip == nil {
		return addr + zone
	}

	normalized := ip.String() + zone
	if hasBrackets {
		return "[" + normalized + "]"
	}
	return normalized
}

// ExpandIPv6 expands an IPv6 address to its full form.
// Returns the address unchanged if not a valid IPv6 address.
func ExpandIPv6(addr string) string {
	// Remove brackets if present
	if strings.HasPrefix(addr, "[") && strings.HasSuffix(addr, "]") {
		addr = addr[1 : len(addr)-1]
	}

	// Remove zone ID if present
	zone := ""
	if idx := strings.Index(addr, "%"); idx != -1 {
		zone = addr[idx:]
		addr = addr[:idx]
	}

	ip := net.ParseIP(addr)
	if ip == nil || ip.To4() != nil {
		return addr + zone
	}

	// Expand to full form
	parts := make([]string, 8)
	for i := 0; i < 8; i++ {
		parts[i] = fmt.Sprintf("%02x%02x", ip[i*2], ip[i*2+1])
	}
	return strings.Join(parts, ":") + zone
}

// CompressIPv6 compresses an IPv6 address using :: notation.
// Returns the address unchanged if not a valid IPv6 address.
func CompressIPv6(addr string) string {
	// Remove brackets if present
	if strings.HasPrefix(addr, "[") && strings.HasSuffix(addr, "]") {
		addr = addr[1 : len(addr)-1]
	}

	// Remove zone ID if present
	zone := ""
	if idx := strings.Index(addr, "%"); idx != -1 {
		zone = addr[idx:]
		addr = addr[:idx]
	}

	ip := net.ParseIP(addr)
	if ip == nil || ip.To4() != nil {
		return addr + zone
	}

	// net.IP.String() already compresses
	return ip.String() + zone
}
