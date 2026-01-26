// Package discovery provides network discovery for proxied devices.
package discovery

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// ICMPScanner discovers devices using ICMP ping.
type ICMPScanner struct {
	// timeout is the ping timeout
	timeout time.Duration

	// concurrency is max concurrent pings
	concurrency int
}

// NewICMPScanner creates a new ICMP scanner.
func NewICMPScanner(timeout time.Duration, concurrency int) *ICMPScanner {
	if timeout == 0 {
		timeout = 2 * time.Second
	}
	if concurrency == 0 {
		concurrency = 100
	}
	return &ICMPScanner{
		timeout:     timeout,
		concurrency: concurrency,
	}
}

// Name returns the scanner name.
func (s *ICMPScanner) Name() string {
	return "icmp"
}

// Scan performs ICMP discovery.
func (s *ICMPScanner) Scan(ctx context.Context, targets []string) ([]*DiscoveredDevice, error) {
	var devices []*DiscoveredDevice
	var mu sync.Mutex

	// Use semaphore for concurrency control
	sem := make(chan struct{}, s.concurrency)
	var wg sync.WaitGroup

	for _, target := range targets {
		select {
		case <-ctx.Done():
			return devices, ctx.Err()
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			defer func() { <-sem }()

			// Try TCP connect to common ports as ICMP fallback
			// (raw ICMP requires root privileges)
			if s.isReachable(ctx, ip) {
				device := &DiscoveredDevice{
					ID:            fmt.Sprintf("icmp-%s", ip),
					Address:       ip,
					Protocol:      ProtocolICMP,
					DiscoveryTime: time.Now(),
					LastSeen:      time.Now(),
					Status:        StatusPending,
					Metadata:      make(map[string]string),
				}

				// Try reverse DNS
				if names, err := net.LookupAddr(ip); err == nil && len(names) > 0 {
					device.Hostname = strings.TrimSuffix(names[0], ".")
				}

				mu.Lock()
				devices = append(devices, device)
				mu.Unlock()
			}
		}(target)
	}

	wg.Wait()
	return devices, nil
}

// isReachable checks if a host is reachable via TCP connect.
func (s *ICMPScanner) isReachable(ctx context.Context, ip string) bool {
	// Try common ports
	ports := []int{22, 23, 80, 443, 161}

	for _, port := range ports {
		addr := fmt.Sprintf("%s:%d", ip, port)
		dialer := net.Dialer{Timeout: s.timeout}

		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err == nil {
			conn.Close()
			return true
		}
	}
	return false
}

// SSHScanner discovers devices via SSH.
type SSHScanner struct {
	// port is the SSH port to scan
	port int

	// timeout is the connection timeout
	timeout time.Duration

	// concurrency is max concurrent connections
	concurrency int

	// grabBanner determines if we should grab the SSH banner
	grabBanner bool
}

// NewSSHScanner creates a new SSH scanner.
func NewSSHScanner(port int, timeout time.Duration, concurrency int) *SSHScanner {
	if port == 0 {
		port = 22
	}
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	if concurrency == 0 {
		concurrency = 50
	}
	return &SSHScanner{
		port:        port,
		timeout:     timeout,
		concurrency: concurrency,
		grabBanner:  true,
	}
}

// Name returns the scanner name.
func (s *SSHScanner) Name() string {
	return "ssh"
}

// Scan performs SSH discovery.
func (s *SSHScanner) Scan(ctx context.Context, targets []string) ([]*DiscoveredDevice, error) {
	var devices []*DiscoveredDevice
	var mu sync.Mutex

	sem := make(chan struct{}, s.concurrency)
	var wg sync.WaitGroup

	for _, target := range targets {
		select {
		case <-ctx.Done():
			return devices, ctx.Err()
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			defer func() { <-sem }()

			device := s.scanHost(ctx, ip)
			if device != nil {
				mu.Lock()
				devices = append(devices, device)
				mu.Unlock()
			}
		}(target)
	}

	wg.Wait()
	return devices, nil
}

// scanHost scans a single host for SSH.
func (s *SSHScanner) scanHost(ctx context.Context, ip string) *DiscoveredDevice {
	addr := fmt.Sprintf("%s:%d", ip, s.port)
	dialer := net.Dialer{Timeout: s.timeout}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil
	}
	defer conn.Close()

	device := &DiscoveredDevice{
		ID:            fmt.Sprintf("ssh-%s-%d", ip, s.port),
		Address:       ip,
		Port:          s.port,
		Protocol:      ProtocolSSH,
		DiscoveryTime: time.Now(),
		LastSeen:      time.Now(),
		Status:        StatusPending,
		Metadata:      make(map[string]string),
	}

	// Try reverse DNS
	if names, err := net.LookupAddr(ip); err == nil && len(names) > 0 {
		device.Hostname = strings.TrimSuffix(names[0], ".")
	}

	// Grab SSH banner
	if s.grabBanner {
		conn.SetReadDeadline(time.Now().Add(s.timeout))
		banner := make([]byte, 256)
		n, err := conn.Read(banner)
		if err == nil && n > 0 {
			device.Banner = strings.TrimSpace(string(banner[:n]))
			device.Metadata["ssh_banner"] = device.Banner

			// Parse banner for vendor hints
			s.parseBanner(device)
		}
	}

	return device
}

// parseBanner extracts vendor information from SSH banner.
func (s *SSHScanner) parseBanner(device *DiscoveredDevice) {
	banner := strings.ToLower(device.Banner)

	switch {
	case strings.Contains(banner, "cisco"):
		device.Vendor = "Cisco"
		device.Type = DeviceTypeRouter
	case strings.Contains(banner, "juniper") || strings.Contains(banner, "junos"):
		device.Vendor = "Juniper"
		device.Type = DeviceTypeRouter
	case strings.Contains(banner, "arista"):
		device.Vendor = "Arista"
		device.Type = DeviceTypeSwitch
	case strings.Contains(banner, "vyos") || strings.Contains(banner, "vyatta"):
		device.Vendor = "VyOS"
		device.Type = DeviceTypeRouter
	case strings.Contains(banner, "pfsense"):
		device.Vendor = "pfSense"
		device.Type = DeviceTypeFirewall
	case strings.Contains(banner, "opnsense"):
		device.Vendor = "OPNsense"
		device.Type = DeviceTypeFirewall
	case strings.Contains(banner, "fortinet") || strings.Contains(banner, "fortigate"):
		device.Vendor = "Fortinet"
		device.Type = DeviceTypeFirewall
	case strings.Contains(banner, "palo alto") || strings.Contains(banner, "panos"):
		device.Vendor = "Palo Alto"
		device.Type = DeviceTypeFirewall
	case strings.Contains(banner, "ubuntu"):
		device.Vendor = "Ubuntu"
		device.Type = DeviceTypeServer
	case strings.Contains(banner, "debian"):
		device.Vendor = "Debian"
		device.Type = DeviceTypeServer
	case strings.Contains(banner, "centos") || strings.Contains(banner, "red hat"):
		device.Vendor = "Red Hat"
		device.Type = DeviceTypeServer
	case strings.Contains(banner, "openssh"):
		device.Type = DeviceTypeServer
	}
}

// SNMPScanner discovers devices via SNMP.
type SNMPScanner struct {
	// port is the SNMP port to scan
	port int

	// community is the SNMP community string
	community string

	// version is the SNMP version (2c or 3)
	version string

	// timeout is the request timeout
	timeout time.Duration

	// concurrency is max concurrent requests
	concurrency int

	// v3Config is SNMPv3 configuration
	v3Config *SNMPv3Config
}

// SNMPv3Config holds SNMPv3 authentication settings.
type SNMPv3Config struct {
	Username     string
	AuthProtocol string // MD5, SHA, SHA256, SHA512
	AuthPassword string
	PrivProtocol string // DES, AES, AES192, AES256
	PrivPassword string
	SecurityLevel string // noAuthNoPriv, authNoPriv, authPriv
}

// NewSNMPScanner creates a new SNMP scanner.
func NewSNMPScanner(port int, community string, timeout time.Duration, concurrency int) *SNMPScanner {
	if port == 0 {
		port = 161
	}
	if community == "" {
		community = "public"
	}
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	if concurrency == 0 {
		concurrency = 50
	}
	return &SNMPScanner{
		port:        port,
		community:   community,
		version:     "2c",
		timeout:     timeout,
		concurrency: concurrency,
	}
}

// Name returns the scanner name.
func (s *SNMPScanner) Name() string {
	return "snmp"
}

// SetV3Config sets SNMPv3 configuration.
func (s *SNMPScanner) SetV3Config(config *SNMPv3Config) {
	s.v3Config = config
	s.version = "3"
}

// Scan performs SNMP discovery.
func (s *SNMPScanner) Scan(ctx context.Context, targets []string) ([]*DiscoveredDevice, error) {
	var devices []*DiscoveredDevice
	var mu sync.Mutex

	sem := make(chan struct{}, s.concurrency)
	var wg sync.WaitGroup

	for _, target := range targets {
		select {
		case <-ctx.Done():
			return devices, ctx.Err()
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			defer func() { <-sem }()

			device := s.scanHost(ctx, ip)
			if device != nil {
				mu.Lock()
				devices = append(devices, device)
				mu.Unlock()
			}
		}(target)
	}

	wg.Wait()
	return devices, nil
}

// scanHost scans a single host for SNMP.
func (s *SNMPScanner) scanHost(ctx context.Context, ip string) *DiscoveredDevice {
	// Build SNMP GET request for sysDescr, sysObjectID, sysName
	// OIDs: 1.3.6.1.2.1.1.1.0 (sysDescr), 1.3.6.1.2.1.1.2.0 (sysObjectID), 1.3.6.1.2.1.1.5.0 (sysName)

	addr := fmt.Sprintf("%s:%d", ip, s.port)

	// Create UDP connection
	conn, err := net.DialTimeout("udp", addr, s.timeout)
	if err != nil {
		return nil
	}
	defer conn.Close()

	// Build SNMPv2c GET request
	request := s.buildGetRequest([]string{
		"1.3.6.1.2.1.1.1.0", // sysDescr
		"1.3.6.1.2.1.1.5.0", // sysName
	})

	conn.SetDeadline(time.Now().Add(s.timeout))
	_, err = conn.Write(request)
	if err != nil {
		return nil
	}

	// Read response
	response := make([]byte, 4096)
	n, err := conn.Read(response)
	if err != nil {
		return nil
	}

	// Parse response
	sysDescr, sysName := s.parseResponse(response[:n])
	if sysDescr == "" {
		return nil
	}

	device := &DiscoveredDevice{
		ID:            fmt.Sprintf("snmp-%s-%d", ip, s.port),
		Address:       ip,
		Port:          s.port,
		Protocol:      ProtocolSNMP,
		SysDescr:      sysDescr,
		Hostname:      sysName,
		DiscoveryTime: time.Now(),
		LastSeen:      time.Now(),
		Status:        StatusPending,
		Metadata:      make(map[string]string),
	}

	device.Metadata["snmp_sysdescr"] = sysDescr
	if sysName != "" {
		device.Metadata["snmp_sysname"] = sysName
	}

	// Parse sysDescr for vendor/model/version
	s.parseSysDescr(device)

	return device
}

// buildGetRequest builds an SNMPv2c GET request.
func (s *SNMPScanner) buildGetRequest(oids []string) []byte {
	// Simplified SNMP packet builder
	// In production, use gosnmp library

	// This is a basic SNMPv2c GET request structure
	// Version: 1 (SNMPv2c)
	// Community: from config
	// PDU: GetRequest

	var packet []byte

	// Build OID bindings
	var bindings []byte
	for _, oid := range oids {
		binding := s.encodeOIDBinding(oid)
		bindings = append(bindings, binding...)
	}

	// Build PDU
	requestID := []byte{0x02, 0x04, 0x00, 0x00, 0x00, 0x01} // Integer: 1
	errorStatus := []byte{0x02, 0x01, 0x00}                 // Integer: 0
	errorIndex := []byte{0x02, 0x01, 0x00}                  // Integer: 0
	varbindList := append([]byte{0x30, byte(len(bindings))}, bindings...)

	pduContent := append(requestID, errorStatus...)
	pduContent = append(pduContent, errorIndex...)
	pduContent = append(pduContent, varbindList...)
	pdu := append([]byte{0xa0, byte(len(pduContent))}, pduContent...) // GetRequest PDU

	// Build message
	version := []byte{0x02, 0x01, 0x01}                              // Integer: 1 (SNMPv2c)
	community := append([]byte{0x04, byte(len(s.community))}, []byte(s.community)...)

	messageContent := append(version, community...)
	messageContent = append(messageContent, pdu...)
	packet = append([]byte{0x30, byte(len(messageContent))}, messageContent...)

	return packet
}

// encodeOIDBinding encodes a single OID binding.
func (s *SNMPScanner) encodeOIDBinding(oid string) []byte {
	// Encode OID and NULL value
	oidBytes := s.encodeOID(oid)
	nullValue := []byte{0x05, 0x00} // NULL

	content := append(oidBytes, nullValue...)
	return append([]byte{0x30, byte(len(content))}, content...)
}

// encodeOID encodes an OID string to BER.
func (s *SNMPScanner) encodeOID(oid string) []byte {
	parts := strings.Split(oid, ".")
	if len(parts) < 2 {
		return nil
	}

	// First two components combined: 40*X + Y
	var first, second int
	fmt.Sscanf(parts[0], "%d", &first)
	fmt.Sscanf(parts[1], "%d", &second)

	encoded := []byte{byte(40*first + second)}

	// Encode remaining components
	for i := 2; i < len(parts); i++ {
		var val int
		fmt.Sscanf(parts[i], "%d", &val)
		encoded = append(encoded, s.encodeOIDComponent(val)...)
	}

	return append([]byte{0x06, byte(len(encoded))}, encoded...)
}

// encodeOIDComponent encodes a single OID component.
func (s *SNMPScanner) encodeOIDComponent(val int) []byte {
	if val < 128 {
		return []byte{byte(val)}
	}

	// Multi-byte encoding
	var bytes []byte
	for val > 0 {
		b := byte(val & 0x7f)
		val >>= 7
		if len(bytes) > 0 {
			b |= 0x80
		}
		bytes = append([]byte{b}, bytes...)
	}
	return bytes
}

// parseResponse parses SNMP response to extract sysDescr and sysName.
func (s *SNMPScanner) parseResponse(data []byte) (sysDescr, sysName string) {
	// Basic SNMP response parsing
	// In production, use gosnmp library

	if len(data) < 10 {
		return "", ""
	}

	// Find string values in the response
	// Look for octet string tags (0x04) followed by length and data
	for i := 0; i < len(data)-2; i++ {
		if data[i] == 0x04 { // Octet string
			length := int(data[i+1])
			if i+2+length <= len(data) {
				str := string(data[i+2 : i+2+length])
				if sysDescr == "" && len(str) > 10 {
					sysDescr = str
				} else if sysName == "" && len(str) > 0 && len(str) < 64 {
					sysName = str
				}
			}
		}
	}

	return sysDescr, sysName
}

// parseSysDescr extracts vendor information from sysDescr.
func (s *SNMPScanner) parseSysDescr(device *DiscoveredDevice) {
	descr := strings.ToLower(device.SysDescr)

	switch {
	case strings.Contains(descr, "cisco ios"):
		device.Vendor = "Cisco"
		device.Type = DeviceTypeRouter
		if strings.Contains(descr, "switch") || strings.Contains(descr, "catalyst") {
			device.Type = DeviceTypeSwitch
		}
		// Extract version
		if idx := strings.Index(device.SysDescr, "Version"); idx >= 0 {
			parts := strings.Fields(device.SysDescr[idx:])
			if len(parts) >= 2 {
				device.Version = strings.TrimSuffix(parts[1], ",")
			}
		}

	case strings.Contains(descr, "cisco nx-os") || strings.Contains(descr, "nexus"):
		device.Vendor = "Cisco"
		device.Type = DeviceTypeSwitch
		device.Model = "Nexus"

	case strings.Contains(descr, "juniper") || strings.Contains(descr, "junos"):
		device.Vendor = "Juniper"
		device.Type = DeviceTypeRouter
		if strings.Contains(descr, "ex") {
			device.Type = DeviceTypeSwitch
		}
		// Extract JUNOS version
		if idx := strings.Index(device.SysDescr, "JUNOS"); idx >= 0 {
			parts := strings.Fields(device.SysDescr[idx:])
			if len(parts) >= 2 {
				device.Version = parts[1]
			}
		}

	case strings.Contains(descr, "arista"):
		device.Vendor = "Arista"
		device.Type = DeviceTypeSwitch
		// Extract EOS version
		if idx := strings.Index(device.SysDescr, "EOS"); idx >= 0 {
			parts := strings.Fields(device.SysDescr[idx:])
			if len(parts) >= 2 {
				device.Version = parts[1]
			}
		}

	case strings.Contains(descr, "pfsense"):
		device.Vendor = "pfSense"
		device.Type = DeviceTypeFirewall

	case strings.Contains(descr, "opnsense"):
		device.Vendor = "OPNsense"
		device.Type = DeviceTypeFirewall

	case strings.Contains(descr, "vyos") || strings.Contains(descr, "vyatta"):
		device.Vendor = "VyOS"
		device.Type = DeviceTypeRouter

	case strings.Contains(descr, "fortinet") || strings.Contains(descr, "fortigate"):
		device.Vendor = "Fortinet"
		device.Type = DeviceTypeFirewall

	case strings.Contains(descr, "palo alto"):
		device.Vendor = "Palo Alto"
		device.Type = DeviceTypeFirewall

	case strings.Contains(descr, "hp") || strings.Contains(descr, "procurve") || strings.Contains(descr, "aruba"):
		device.Vendor = "HPE/Aruba"
		device.Type = DeviceTypeSwitch

	case strings.Contains(descr, "dell"):
		device.Vendor = "Dell"
		device.Type = DeviceTypeSwitch

	case strings.Contains(descr, "linux"):
		device.Type = DeviceTypeServer
		if strings.Contains(descr, "ubuntu") {
			device.Vendor = "Ubuntu"
		} else if strings.Contains(descr, "debian") {
			device.Vendor = "Debian"
		} else if strings.Contains(descr, "centos") {
			device.Vendor = "CentOS"
		} else if strings.Contains(descr, "red hat") {
			device.Vendor = "Red Hat"
		}

	case strings.Contains(descr, "windows"):
		device.Vendor = "Microsoft"
		device.Type = DeviceTypeServer

	case strings.Contains(descr, "printer") || strings.Contains(descr, "print"):
		device.Type = DeviceTypePrinter

	case strings.Contains(descr, "access point") || strings.Contains(descr, "wireless"):
		device.Type = DeviceTypeAccessPoint
	}
}

// HTTPScanner discovers devices via HTTP/HTTPS.
type HTTPScanner struct {
	// ports are the HTTP ports to scan
	ports []int

	// timeout is the connection timeout
	timeout time.Duration

	// concurrency is max concurrent connections
	concurrency int

	// checkHTTPS determines if we should try HTTPS
	checkHTTPS bool
}

// NewHTTPScanner creates a new HTTP scanner.
func NewHTTPScanner(ports []int, timeout time.Duration, concurrency int) *HTTPScanner {
	if len(ports) == 0 {
		ports = []int{80, 443, 8080, 8443}
	}
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	if concurrency == 0 {
		concurrency = 50
	}
	return &HTTPScanner{
		ports:       ports,
		timeout:     timeout,
		concurrency: concurrency,
		checkHTTPS:  true,
	}
}

// Name returns the scanner name.
func (s *HTTPScanner) Name() string {
	return "http"
}

// Scan performs HTTP discovery.
func (s *HTTPScanner) Scan(ctx context.Context, targets []string) ([]*DiscoveredDevice, error) {
	var devices []*DiscoveredDevice
	var mu sync.Mutex

	sem := make(chan struct{}, s.concurrency)
	var wg sync.WaitGroup

	for _, target := range targets {
		for _, port := range s.ports {
			select {
			case <-ctx.Done():
				return devices, ctx.Err()
			case sem <- struct{}{}:
			}

			wg.Add(1)
			go func(ip string, port int) {
				defer wg.Done()
				defer func() { <-sem }()

				device := s.scanHost(ctx, ip, port)
				if device != nil {
					mu.Lock()
					devices = append(devices, device)
					mu.Unlock()
				}
			}(target, port)
		}
	}

	wg.Wait()
	return devices, nil
}

// scanHost scans a single host for HTTP.
func (s *HTTPScanner) scanHost(ctx context.Context, ip string, port int) *DiscoveredDevice {
	addr := fmt.Sprintf("%s:%d", ip, port)
	dialer := net.Dialer{Timeout: s.timeout}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil
	}
	defer conn.Close()

	protocol := ProtocolHTTP
	if port == 443 || port == 8443 {
		protocol = ProtocolHTTPS
	}

	device := &DiscoveredDevice{
		ID:            fmt.Sprintf("http-%s-%d", ip, port),
		Address:       ip,
		Port:          port,
		Protocol:      protocol,
		DiscoveryTime: time.Now(),
		LastSeen:      time.Now(),
		Status:        StatusPending,
		Metadata:      make(map[string]string),
	}

	// Try reverse DNS
	if names, err := net.LookupAddr(ip); err == nil && len(names) > 0 {
		device.Hostname = strings.TrimSuffix(names[0], ".")
	}

	// Send HTTP request to get server header
	conn.SetDeadline(time.Now().Add(s.timeout))
	request := fmt.Sprintf("GET / HTTP/1.0\r\nHost: %s\r\n\r\n", ip)
	conn.Write([]byte(request))

	response := make([]byte, 4096)
	n, err := conn.Read(response)
	if err == nil && n > 0 {
		s.parseHTTPResponse(device, string(response[:n]))
	}

	return device
}

// parseHTTPResponse parses HTTP response for server info.
func (s *HTTPScanner) parseHTTPResponse(device *DiscoveredDevice, response string) {
	lines := strings.Split(response, "\r\n")

	for _, line := range lines {
		lower := strings.ToLower(line)

		if strings.HasPrefix(lower, "server:") {
			server := strings.TrimSpace(line[7:])
			device.Metadata["http_server"] = server
			device.Banner = server

			// Parse server for vendor hints
			serverLower := strings.ToLower(server)
			switch {
			case strings.Contains(serverLower, "cisco"):
				device.Vendor = "Cisco"
			case strings.Contains(serverLower, "juniper"):
				device.Vendor = "Juniper"
			case strings.Contains(serverLower, "arista"):
				device.Vendor = "Arista"
			case strings.Contains(serverLower, "nginx"):
				device.Metadata["webserver"] = "nginx"
			case strings.Contains(serverLower, "apache"):
				device.Metadata["webserver"] = "apache"
			case strings.Contains(serverLower, "lighttpd"):
				device.Metadata["webserver"] = "lighttpd"
			}
		}

		if strings.HasPrefix(lower, "x-powered-by:") {
			device.Metadata["powered_by"] = strings.TrimSpace(line[13:])
		}
	}
}

// WinRMScanner discovers devices via WinRM.
type WinRMScanner struct {
	// ports are the WinRM ports to scan
	ports []int

	// timeout is the connection timeout
	timeout time.Duration

	// concurrency is max concurrent connections
	concurrency int
}

// NewWinRMScanner creates a new WinRM scanner.
func NewWinRMScanner(ports []int, timeout time.Duration, concurrency int) *WinRMScanner {
	if len(ports) == 0 {
		ports = []int{5985, 5986} // HTTP and HTTPS WinRM
	}
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	if concurrency == 0 {
		concurrency = 50
	}
	return &WinRMScanner{
		ports:       ports,
		timeout:     timeout,
		concurrency: concurrency,
	}
}

// Name returns the scanner name.
func (s *WinRMScanner) Name() string {
	return "winrm"
}

// Scan performs WinRM discovery.
func (s *WinRMScanner) Scan(ctx context.Context, targets []string) ([]*DiscoveredDevice, error) {
	var devices []*DiscoveredDevice
	var mu sync.Mutex

	sem := make(chan struct{}, s.concurrency)
	var wg sync.WaitGroup

	for _, target := range targets {
		for _, port := range s.ports {
			select {
			case <-ctx.Done():
				return devices, ctx.Err()
			case sem <- struct{}{}:
			}

			wg.Add(1)
			go func(ip string, port int) {
				defer wg.Done()
				defer func() { <-sem }()

				device := s.scanHost(ctx, ip, port)
				if device != nil {
					mu.Lock()
					devices = append(devices, device)
					mu.Unlock()
				}
			}(target, port)
		}
	}

	wg.Wait()
	return devices, nil
}

// scanHost scans a single host for WinRM.
func (s *WinRMScanner) scanHost(ctx context.Context, ip string, port int) *DiscoveredDevice {
	addr := fmt.Sprintf("%s:%d", ip, port)
	dialer := net.Dialer{Timeout: s.timeout}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil
	}
	defer conn.Close()

	device := &DiscoveredDevice{
		ID:            fmt.Sprintf("winrm-%s-%d", ip, port),
		Address:       ip,
		Port:          port,
		Protocol:      ProtocolWinRM,
		Vendor:        "Microsoft",
		Type:          DeviceTypeServer,
		DiscoveryTime: time.Now(),
		LastSeen:      time.Now(),
		Status:        StatusPending,
		Metadata:      make(map[string]string),
	}

	// Try reverse DNS
	if names, err := net.LookupAddr(ip); err == nil && len(names) > 0 {
		device.Hostname = strings.TrimSuffix(names[0], ".")
	}

	device.Metadata["winrm_port"] = fmt.Sprintf("%d", port)
	if port == 5986 {
		device.Metadata["winrm_https"] = "true"
	}

	return device
}
