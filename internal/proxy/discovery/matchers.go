// Package discovery provides network discovery for proxied devices.
package discovery

import (
	"regexp"
	"strings"
)

// DeviceProfile defines a device profile for matching.
type DeviceProfile struct {
	// Name is the profile name
	Name string `json:"name"`

	// Description describes the profile
	Description string `json:"description,omitempty"`

	// Vendor patterns to match
	VendorPatterns []string `json:"vendor_patterns,omitempty"`

	// SysDescr patterns to match (regex)
	SysDescrPatterns []string `json:"sysdescr_patterns,omitempty"`

	// Banner patterns to match (regex)
	BannerPatterns []string `json:"banner_patterns,omitempty"`

	// DeviceTypes to match
	DeviceTypes []DeviceType `json:"device_types,omitempty"`

	// Protocols to match
	Protocols []DiscoveryProtocol `json:"protocols,omitempty"`

	// Ports to match
	Ports []int `json:"ports,omitempty"`

	// Priority for matching (higher = checked first)
	Priority int `json:"priority"`

	// SuggestedCredentials are credential types to try
	SuggestedCredentials []string `json:"suggested_credentials,omitempty"`

	// AdapterType is the recommended adapter type
	AdapterType string `json:"adapter_type,omitempty"`
}

// PatternMatcher matches devices to profiles using patterns.
type PatternMatcher struct {
	profiles []*DeviceProfile

	// compiledSysDescr holds compiled regex patterns
	compiledSysDescr map[string][]*regexp.Regexp

	// compiledBanner holds compiled banner patterns
	compiledBanner map[string][]*regexp.Regexp
}

// NewPatternMatcher creates a new pattern matcher.
func NewPatternMatcher() *PatternMatcher {
	return &PatternMatcher{
		profiles:         make([]*DeviceProfile, 0),
		compiledSysDescr: make(map[string][]*regexp.Regexp),
		compiledBanner:   make(map[string][]*regexp.Regexp),
	}
}

// AddProfile adds a profile to the matcher.
func (m *PatternMatcher) AddProfile(profile *DeviceProfile) error {
	// Compile regex patterns
	var sysDescrPatterns []*regexp.Regexp
	for _, pattern := range profile.SysDescrPatterns {
		re, err := regexp.Compile("(?i)" + pattern) // Case insensitive
		if err != nil {
			return err
		}
		sysDescrPatterns = append(sysDescrPatterns, re)
	}
	m.compiledSysDescr[profile.Name] = sysDescrPatterns

	var bannerPatterns []*regexp.Regexp
	for _, pattern := range profile.BannerPatterns {
		re, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			return err
		}
		bannerPatterns = append(bannerPatterns, re)
	}
	m.compiledBanner[profile.Name] = bannerPatterns

	// Insert profile in priority order
	inserted := false
	for i, p := range m.profiles {
		if profile.Priority > p.Priority {
			m.profiles = append(m.profiles[:i], append([]*DeviceProfile{profile}, m.profiles[i:]...)...)
			inserted = true
			break
		}
	}
	if !inserted {
		m.profiles = append(m.profiles, profile)
	}

	return nil
}

// Match returns the matching profile name for a device.
func (m *PatternMatcher) Match(device *DiscoveredDevice) string {
	for _, profile := range m.profiles {
		if m.matchProfile(device, profile) {
			// Copy suggested credentials
			device.SuggestedCredentials = profile.SuggestedCredentials
			return profile.Name
		}
	}
	return ""
}

// matchProfile checks if a device matches a profile.
func (m *PatternMatcher) matchProfile(device *DiscoveredDevice, profile *DeviceProfile) bool {
	// Check vendor patterns
	if len(profile.VendorPatterns) > 0 {
		matched := false
		for _, pattern := range profile.VendorPatterns {
			if strings.EqualFold(device.Vendor, pattern) ||
				strings.Contains(strings.ToLower(device.Vendor), strings.ToLower(pattern)) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check device types
	if len(profile.DeviceTypes) > 0 {
		matched := false
		for _, dt := range profile.DeviceTypes {
			if device.Type == dt {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check protocols
	if len(profile.Protocols) > 0 {
		matched := false
		for _, proto := range profile.Protocols {
			if device.Protocol == proto {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check ports
	if len(profile.Ports) > 0 {
		matched := false
		for _, port := range profile.Ports {
			if device.Port == port {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check sysDescr patterns (regex)
	if patterns, ok := m.compiledSysDescr[profile.Name]; ok && len(patterns) > 0 {
		if device.SysDescr == "" {
			return false
		}
		matched := false
		for _, re := range patterns {
			if re.MatchString(device.SysDescr) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check banner patterns (regex)
	if patterns, ok := m.compiledBanner[profile.Name]; ok && len(patterns) > 0 {
		if device.Banner == "" {
			return false
		}
		matched := false
		for _, re := range patterns {
			if re.MatchString(device.Banner) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	return true
}

// GetProfile returns a profile by name.
func (m *PatternMatcher) GetProfile(name string) *DeviceProfile {
	for _, profile := range m.profiles {
		if profile.Name == name {
			return profile
		}
	}
	return nil
}

// DefaultProfiles returns the default device profiles.
func DefaultProfiles() []*DeviceProfile {
	return []*DeviceProfile{
		// Cisco IOS
		{
			Name:             "cisco-ios",
			Description:      "Cisco IOS routers and switches",
			VendorPatterns:   []string{"Cisco"},
			SysDescrPatterns: []string{"Cisco IOS", "IOS Software"},
			DeviceTypes:      []DeviceType{DeviceTypeRouter, DeviceTypeSwitch},
			Priority:         100,
			SuggestedCredentials: []string{"ssh", "snmp"},
			AdapterType:      "cisco-ios",
		},
		// Cisco NX-OS
		{
			Name:             "cisco-nxos",
			Description:      "Cisco Nexus switches (NX-OS)",
			VendorPatterns:   []string{"Cisco"},
			SysDescrPatterns: []string{"NX-OS", "Nexus"},
			DeviceTypes:      []DeviceType{DeviceTypeSwitch},
			Priority:         110,
			SuggestedCredentials: []string{"ssh", "snmp"},
			AdapterType:      "cisco-nxos",
		},
		// Juniper JUNOS
		{
			Name:             "juniper-junos",
			Description:      "Juniper routers and switches (JUNOS)",
			VendorPatterns:   []string{"Juniper"},
			SysDescrPatterns: []string{"JUNOS", "Juniper Networks"},
			DeviceTypes:      []DeviceType{DeviceTypeRouter, DeviceTypeSwitch},
			Priority:         100,
			SuggestedCredentials: []string{"ssh", "snmp", "netconf"},
			AdapterType:      "juniper-junos",
		},
		// Arista EOS
		{
			Name:             "arista-eos",
			Description:      "Arista switches (EOS)",
			VendorPatterns:   []string{"Arista"},
			SysDescrPatterns: []string{"Arista", "EOS"},
			DeviceTypes:      []DeviceType{DeviceTypeSwitch},
			Priority:         100,
			SuggestedCredentials: []string{"ssh", "snmp", "eapi"},
			AdapterType:      "arista-eos",
		},
		// VyOS
		{
			Name:             "vyos",
			Description:      "VyOS/EdgeOS routers",
			VendorPatterns:   []string{"VyOS", "Vyatta", "EdgeOS"},
			SysDescrPatterns: []string{"VyOS", "Vyatta", "EdgeOS"},
			BannerPatterns:   []string{"vyos", "vyatta", "edgeos"},
			DeviceTypes:      []DeviceType{DeviceTypeRouter, DeviceTypeFirewall},
			Priority:         100,
			SuggestedCredentials: []string{"ssh"},
			AdapterType:      "vyos",
		},
		// pfSense
		{
			Name:             "pfsense",
			Description:      "pfSense firewalls",
			VendorPatterns:   []string{"pfSense"},
			SysDescrPatterns: []string{"pfSense", "pfsense"},
			BannerPatterns:   []string{"pfsense"},
			DeviceTypes:      []DeviceType{DeviceTypeFirewall},
			Priority:         100,
			SuggestedCredentials: []string{"ssh", "api"},
			AdapterType:      "pfsense",
		},
		// OPNsense
		{
			Name:             "opnsense",
			Description:      "OPNsense firewalls",
			VendorPatterns:   []string{"OPNsense"},
			SysDescrPatterns: []string{"OPNsense", "opnsense"},
			BannerPatterns:   []string{"opnsense"},
			DeviceTypes:      []DeviceType{DeviceTypeFirewall},
			Priority:         100,
			SuggestedCredentials: []string{"ssh", "api"},
			AdapterType:      "opnsense",
		},
		// Fortinet FortiGate
		{
			Name:             "fortinet-fortigate",
			Description:      "Fortinet FortiGate firewalls",
			VendorPatterns:   []string{"Fortinet"},
			SysDescrPatterns: []string{"FortiGate", "Fortinet"},
			DeviceTypes:      []DeviceType{DeviceTypeFirewall},
			Priority:         100,
			SuggestedCredentials: []string{"ssh", "api"},
			AdapterType:      "fortinet-fortigate",
		},
		// Palo Alto
		{
			Name:             "paloalto-panos",
			Description:      "Palo Alto firewalls (PAN-OS)",
			VendorPatterns:   []string{"Palo Alto"},
			SysDescrPatterns: []string{"Palo Alto", "PAN-OS"},
			DeviceTypes:      []DeviceType{DeviceTypeFirewall},
			Priority:         100,
			SuggestedCredentials: []string{"ssh", "api"},
			AdapterType:      "paloalto-panos",
		},
		// HPE/Aruba
		{
			Name:             "hpe-aruba",
			Description:      "HPE/Aruba switches",
			VendorPatterns:   []string{"HPE", "HP", "Aruba", "ProCurve"},
			SysDescrPatterns: []string{"ProCurve", "Aruba", "HP "},
			DeviceTypes:      []DeviceType{DeviceTypeSwitch},
			Priority:         90,
			SuggestedCredentials: []string{"ssh", "snmp"},
			AdapterType:      "hpe-aruba",
		},
		// Dell Networking
		{
			Name:             "dell-networking",
			Description:      "Dell networking switches",
			VendorPatterns:   []string{"Dell"},
			SysDescrPatterns: []string{"Dell Networking", "Force10", "Dell EMC"},
			DeviceTypes:      []DeviceType{DeviceTypeSwitch},
			Priority:         90,
			SuggestedCredentials: []string{"ssh", "snmp"},
			AdapterType:      "dell-networking",
		},
		// Linux Server
		{
			Name:             "linux-server",
			Description:      "Linux servers",
			VendorPatterns:   []string{"Ubuntu", "Debian", "Red Hat", "CentOS"},
			SysDescrPatterns: []string{"Linux"},
			BannerPatterns:   []string{"OpenSSH", "Ubuntu", "Debian"},
			DeviceTypes:      []DeviceType{DeviceTypeServer},
			Protocols:        []DiscoveryProtocol{ProtocolSSH},
			Priority:         50,
			SuggestedCredentials: []string{"ssh"},
			AdapterType:      "ssh",
		},
		// Windows Server
		{
			Name:             "windows-server",
			Description:      "Windows servers",
			VendorPatterns:   []string{"Microsoft"},
			SysDescrPatterns: []string{"Windows", "Microsoft"},
			DeviceTypes:      []DeviceType{DeviceTypeServer},
			Protocols:        []DiscoveryProtocol{ProtocolWinRM},
			Priority:         50,
			SuggestedCredentials: []string{"winrm"},
			AdapterType:      "winrm",
		},
		// Generic Router
		{
			Name:             "generic-router",
			Description:      "Generic router (SSH)",
			DeviceTypes:      []DeviceType{DeviceTypeRouter},
			Protocols:        []DiscoveryProtocol{ProtocolSSH, ProtocolSNMP},
			Priority:         10,
			SuggestedCredentials: []string{"ssh", "snmp"},
			AdapterType:      "ssh",
		},
		// Generic Switch
		{
			Name:             "generic-switch",
			Description:      "Generic switch (SSH)",
			DeviceTypes:      []DeviceType{DeviceTypeSwitch},
			Protocols:        []DiscoveryProtocol{ProtocolSSH, ProtocolSNMP},
			Priority:         10,
			SuggestedCredentials: []string{"ssh", "snmp"},
			AdapterType:      "ssh",
		},
		// Generic Firewall
		{
			Name:             "generic-firewall",
			Description:      "Generic firewall (SSH)",
			DeviceTypes:      []DeviceType{DeviceTypeFirewall},
			Protocols:        []DiscoveryProtocol{ProtocolSSH, ProtocolHTTPS},
			Priority:         10,
			SuggestedCredentials: []string{"ssh", "api"},
			AdapterType:      "ssh",
		},
		// Generic Server
		{
			Name:             "generic-server",
			Description:      "Generic server (SSH)",
			DeviceTypes:      []DeviceType{DeviceTypeServer},
			Protocols:        []DiscoveryProtocol{ProtocolSSH},
			Priority:         5,
			SuggestedCredentials: []string{"ssh"},
			AdapterType:      "ssh",
		},
	}
}

// LLDPNeighborMatcher matches devices based on LLDP/CDP neighbor information.
type LLDPNeighborMatcher struct {
	// knownDevices maps device names to profiles
	knownDevices map[string]string
}

// NewLLDPNeighborMatcher creates a new LLDP neighbor matcher.
func NewLLDPNeighborMatcher() *LLDPNeighborMatcher {
	return &LLDPNeighborMatcher{
		knownDevices: make(map[string]string),
	}
}

// AddKnownDevice adds a known device to the matcher.
func (m *LLDPNeighborMatcher) AddKnownDevice(deviceName, profile string) {
	m.knownDevices[strings.ToLower(deviceName)] = profile
}

// Match returns a profile based on LLDP neighbor information.
func (m *LLDPNeighborMatcher) Match(device *DiscoveredDevice) string {
	// Check if any neighbor is a known device
	for _, neighbor := range device.Neighbors {
		if profile, ok := m.knownDevices[strings.ToLower(neighbor.RemoteDevice)]; ok {
			// If connected to a known device, suggest similar profile
			return profile
		}
	}
	return ""
}

// CompositeMatcher combines multiple matchers.
type CompositeMatcher struct {
	matchers []ProfileMatcher
}

// NewCompositeMatcher creates a new composite matcher.
func NewCompositeMatcher(matchers ...ProfileMatcher) *CompositeMatcher {
	return &CompositeMatcher{
		matchers: matchers,
	}
}

// AddMatcher adds a matcher to the composite.
func (m *CompositeMatcher) AddMatcher(matcher ProfileMatcher) {
	m.matchers = append(m.matchers, matcher)
}

// Match returns the first matching profile from any matcher.
func (m *CompositeMatcher) Match(device *DiscoveredDevice) string {
	for _, matcher := range m.matchers {
		if profile := matcher.Match(device); profile != "" {
			return profile
		}
	}
	return ""
}

// DefaultMatcher creates a matcher with default profiles.
func DefaultMatcher() *PatternMatcher {
	matcher := NewPatternMatcher()
	for _, profile := range DefaultProfiles() {
		matcher.AddProfile(profile)
	}
	return matcher
}
