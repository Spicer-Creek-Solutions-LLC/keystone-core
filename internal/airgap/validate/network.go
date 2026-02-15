package validate

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
)

// NetworkChecker monitors active network connections for external addresses.
// Linux-only via /proc/net/tcp; graceful degradation on other platforms.
type NetworkChecker struct {
	InternalNets []*net.IPNet
}

// Name returns the checker name.
func (c *NetworkChecker) Name() string { return "network-connections" }

// Category returns the check category.
func (c *NetworkChecker) Category() CheckCategory { return CategoryNetwork }

// Check inspects active TCP connections for external addresses.
func (c *NetworkChecker) Check(ctx context.Context) ([]Finding, error) {
	if runtime.GOOS != "linux" {
		return []Finding{{
			Category: CategoryNetwork,
			Check:    "network-connections",
			Severity: SeverityWarn,
			Message:  fmt.Sprintf("network connection check not supported on %s", runtime.GOOS),
			Detail:   "only Linux is supported via /proc/net/tcp",
		}}, nil
	}

	var findings []Finding
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}
		ff, err := c.checkProcNet(path)
		if err != nil {
			findings = append(findings, Finding{
				Category: CategoryNetwork,
				Check:    "network-connections",
				Severity: SeverityWarn,
				Message:  fmt.Sprintf("could not read %s: %v", path, err),
			})
			continue
		}
		findings = append(findings, ff...)
	}

	if len(findings) == 0 {
		findings = append(findings, Finding{
			Category: CategoryNetwork,
			Check:    "network-connections",
			Severity: SeverityPass,
			Message:  "no external network connections detected",
		})
	}
	return findings, nil
}

func (c *NetworkChecker) checkProcNet(path string) ([]Finding, error) {
	f, err := os.Open(path) //#nosec G304 -- fixed system path
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var findings []Finding
	scanner := bufio.NewScanner(f)
	scanner.Scan() // skip header

	for scanner.Scan() {
		line := scanner.Text()
		remoteAddr, err := parseProcNetLine(line)
		if err != nil {
			continue
		}
		if remoteAddr == nil || remoteAddr.IsLoopback() || remoteAddr.IsUnspecified() {
			continue
		}

		internal := false
		for _, cidr := range c.InternalNets {
			if cidr.Contains(remoteAddr) {
				internal = true
				break
			}
		}

		if !internal {
			findings = append(findings, Finding{
				Category:    CategoryNetwork,
				Check:       "network-connections",
				Severity:    SeverityFail,
				Message:     fmt.Sprintf("external connection to %s", remoteAddr.String()),
				Remediation: "Investigate and eliminate external network connections for air-gapped compliance",
			})
		}
	}
	return findings, scanner.Err()
}

// parseProcNetLine extracts the remote IP from a /proc/net/tcp line.
// Format: sl local_address rem_address st tx_queue:rx_queue ...
// Addresses are hex-encoded IP:port pairs.
func parseProcNetLine(line string) (net.IP, error) {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return nil, fmt.Errorf("too few fields")
	}

	remoteField := fields[2]
	return parseHexAddr(remoteField)
}

// ParseHexAddr parses a hex-encoded IP:port from /proc/net/tcp.
// IPv4 format: AABBCCDD:PORT (little-endian bytes)
// IPv6 format: 00000000000000000000000000000000:PORT
func parseHexAddr(s string) (net.IP, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid address format")
	}

	hexIP := parts[0]
	ipBytes, err := hex.DecodeString(hexIP)
	if err != nil {
		return nil, fmt.Errorf("decode hex: %w", err)
	}

	switch len(ipBytes) {
	case 4:
		// IPv4: stored in little-endian order in /proc/net/tcp
		return net.IPv4(ipBytes[3], ipBytes[2], ipBytes[1], ipBytes[0]), nil
	case 16:
		// IPv6: stored as 4 groups of 4 bytes, each group in little-endian
		ip := make(net.IP, 16)
		for i := 0; i < 4; i++ {
			ip[i*4] = ipBytes[i*4+3]
			ip[i*4+1] = ipBytes[i*4+2]
			ip[i*4+2] = ipBytes[i*4+1]
			ip[i*4+3] = ipBytes[i*4]
		}
		return ip, nil
	default:
		return nil, fmt.Errorf("unexpected IP length: %d", len(ipBytes))
	}
}
