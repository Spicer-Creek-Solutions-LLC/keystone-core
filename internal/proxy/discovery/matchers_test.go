package discovery

import (
	"testing"
)

func TestNewPatternMatcher(t *testing.T) {
	m := NewPatternMatcher()

	if m == nil {
		t.Fatal("NewPatternMatcher returned nil")
	}

	if m.profiles == nil {
		t.Error("profiles slice is nil")
	}

	if m.compiledSysDescr == nil {
		t.Error("compiledSysDescr map is nil")
	}

	if m.compiledBanner == nil {
		t.Error("compiledBanner map is nil")
	}
}

func TestPatternMatcher_AddProfile(t *testing.T) {
	m := NewPatternMatcher()

	profile := &DeviceProfile{
		Name:             "test-profile",
		Description:      "Test Profile",
		VendorPatterns:   []string{"TestVendor"},
		SysDescrPatterns: []string{`TestOS.*`},
		BannerPatterns:   []string{`SSH-2.0-Test.*`},
	}

	err := m.AddProfile(profile)
	if err != nil {
		t.Errorf("AddProfile failed: %v", err)
	}

	if len(m.profiles) != 1 {
		t.Errorf("expected 1 profile, got %d", len(m.profiles))
	}

	if m.profiles[0].Name != "test-profile" {
		t.Errorf("expected profile name 'test-profile', got %s", m.profiles[0].Name)
	}
}

func TestPatternMatcher_AddProfile_InvalidRegex(t *testing.T) {
	m := NewPatternMatcher()

	// Add profile with invalid regex - should return error
	profile := &DeviceProfile{
		Name:             "invalid-profile",
		SysDescrPatterns: []string{`[invalid regex`},
	}

	err := m.AddProfile(profile)
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestPatternMatcher_Match_SysDescr(t *testing.T) {
	m := NewPatternMatcher()

	profile := &DeviceProfile{
		Name:             "cisco-ios",
		VendorPatterns:   []string{"Cisco"},
		SysDescrPatterns: []string{`Cisco IOS Software.*`},
	}
	err := m.AddProfile(profile)
	if err != nil {
		t.Fatalf("AddProfile failed: %v", err)
	}

	// Test matching device
	device := &DiscoveredDevice{
		ID:       "test-1",
		Address:  "192.168.1.1",
		Protocol: ProtocolSNMP,
		Vendor:   "Cisco",
		SysDescr: "Cisco IOS Software, C2960 Software (C2960-LANBASEK9-M), Version 15.0(2)SE",
	}

	matchedProfile := m.Match(device)
	if matchedProfile != "cisco-ios" {
		t.Errorf("expected profile 'cisco-ios', got '%s'", matchedProfile)
	}

	// Test non-matching device
	device2 := &DiscoveredDevice{
		ID:       "test-2",
		Address:  "192.168.1.2",
		Protocol: ProtocolSNMP,
		Vendor:   "Linux",
		SysDescr: "Linux server 5.4.0-generic",
	}

	matchedProfile2 := m.Match(device2)
	if matchedProfile2 != "" {
		t.Errorf("expected no match, got '%s'", matchedProfile2)
	}
}

func TestPatternMatcher_Match_Banner(t *testing.T) {
	m := NewPatternMatcher()

	profile := &DeviceProfile{
		Name:           "juniper-junos",
		VendorPatterns: []string{"Juniper"},
		BannerPatterns: []string{`SSH-2.0-OpenSSH.*JUNOS.*`},
	}
	err := m.AddProfile(profile)
	if err != nil {
		t.Fatalf("AddProfile failed: %v", err)
	}

	device := &DiscoveredDevice{
		ID:       "test-1",
		Address:  "192.168.1.1",
		Protocol: ProtocolSSH,
		Vendor:   "Juniper",
		Banner:   "SSH-2.0-OpenSSH_7.5 JUNOS-22.2R1.9",
	}

	matchedProfile := m.Match(device)
	if matchedProfile != "juniper-junos" {
		t.Errorf("expected profile 'juniper-junos', got '%s'", matchedProfile)
	}
}

func TestPatternMatcher_GetProfile(t *testing.T) {
	m := NewPatternMatcher()

	profile := &DeviceProfile{
		Name:        "test-profile",
		Description: "Test Profile Description",
	}
	err := m.AddProfile(profile)
	if err != nil {
		t.Fatalf("AddProfile failed: %v", err)
	}

	// Test getting existing profile
	found := m.GetProfile("test-profile")
	if found == nil {
		t.Fatal("GetProfile returned nil for existing profile")
	}
	if found.Description != "Test Profile Description" {
		t.Errorf("expected description 'Test Profile Description', got '%s'", found.Description)
	}

	// Test getting non-existent profile
	notFound := m.GetProfile("non-existent")
	if notFound != nil {
		t.Error("expected nil for non-existent profile")
	}
}

func TestDefaultProfiles(t *testing.T) {
	profiles := DefaultProfiles()

	if len(profiles) == 0 {
		t.Fatal("DefaultProfiles returned empty slice")
	}

	// Check for some expected profiles
	expectedProfiles := []string{
		"cisco-ios",
		"cisco-nxos",
		"juniper-junos",
		"arista-eos",
		"vyos",
		"pfsense",
		"opnsense",
		"linux-server",
	}

	profileNames := make(map[string]bool)
	for _, p := range profiles {
		profileNames[p.Name] = true
	}

	for _, expected := range expectedProfiles {
		if !profileNames[expected] {
			t.Errorf("expected profile '%s' not found in default profiles", expected)
		}
	}
}

func TestDefaultMatcher(t *testing.T) {
	matcher := DefaultMatcher()

	if matcher == nil {
		t.Fatal("DefaultMatcher returned nil")
	}

	// Should have default profiles loaded
	if len(matcher.profiles) == 0 {
		t.Error("expected default profiles to be loaded")
	}
}

func TestCompositeMatcher(t *testing.T) {
	m1 := NewPatternMatcher()
	err := m1.AddProfile(&DeviceProfile{
		Name:             "profile-1",
		SysDescrPatterns: []string{`Pattern1.*`},
	})
	if err != nil {
		t.Fatalf("AddProfile failed: %v", err)
	}

	m2 := NewPatternMatcher()
	err = m2.AddProfile(&DeviceProfile{
		Name:             "profile-2",
		SysDescrPatterns: []string{`Pattern2.*`},
	})
	if err != nil {
		t.Fatalf("AddProfile failed: %v", err)
	}

	composite := NewCompositeMatcher(m1, m2)

	// Test matching first matcher
	device1 := &DiscoveredDevice{
		ID:       "test-1",
		SysDescr: "Pattern1 test device",
	}
	match1 := composite.Match(device1)
	if match1 != "profile-1" {
		t.Errorf("expected 'profile-1', got '%s'", match1)
	}

	// Test matching second matcher
	device2 := &DiscoveredDevice{
		ID:       "test-2",
		SysDescr: "Pattern2 test device",
	}
	match2 := composite.Match(device2)
	if match2 != "profile-2" {
		t.Errorf("expected 'profile-2', got '%s'", match2)
	}

	// Test no match
	device3 := &DiscoveredDevice{
		ID:       "test-3",
		SysDescr: "Unknown device",
	}
	match3 := composite.Match(device3)
	if match3 != "" {
		t.Errorf("expected empty string, got '%s'", match3)
	}
}

func TestCompositeMatcher_AddMatcher(t *testing.T) {
	composite := NewCompositeMatcher()

	m := NewPatternMatcher()
	err := m.AddProfile(&DeviceProfile{
		Name:             "test-profile",
		SysDescrPatterns: []string{`Test.*`},
	})
	if err != nil {
		t.Fatalf("AddProfile failed: %v", err)
	}

	composite.AddMatcher(m)

	if len(composite.matchers) != 1 {
		t.Errorf("expected 1 matcher, got %d", len(composite.matchers))
	}
}

func TestDeviceProfile_Fields(t *testing.T) {
	profile := &DeviceProfile{
		Name:                 "test-profile",
		Description:          "Test Profile",
		VendorPatterns:       []string{"TestVendor"},
		SysDescrPatterns:     []string{`Test.*`},
		BannerPatterns:       []string{`SSH-2.0-Test.*`},
		DeviceTypes:          []DeviceType{DeviceTypeRouter},
		Protocols:            []Protocol{ProtocolSSH},
		Ports:                []int{22},
		Priority:             100,
		SuggestedCredentials: []string{"ssh"},
		AdapterType:          "ssh",
	}

	if profile.Name != "test-profile" {
		t.Errorf("Name mismatch")
	}
	if profile.Description != "Test Profile" {
		t.Errorf("Description mismatch")
	}
	if len(profile.VendorPatterns) != 1 || profile.VendorPatterns[0] != "TestVendor" {
		t.Errorf("VendorPatterns mismatch")
	}
	if profile.Priority != 100 {
		t.Errorf("Priority mismatch")
	}
	if profile.AdapterType != "ssh" {
		t.Errorf("AdapterType mismatch")
	}
}

func TestPatternMatcher_PriorityOrdering(t *testing.T) {
	m := NewPatternMatcher()

	// Add profiles with different priorities
	err := m.AddProfile(&DeviceProfile{
		Name:     "low-priority",
		Priority: 10,
	})
	if err != nil {
		t.Fatalf("AddProfile failed: %v", err)
	}

	err = m.AddProfile(&DeviceProfile{
		Name:     "high-priority",
		Priority: 100,
	})
	if err != nil {
		t.Fatalf("AddProfile failed: %v", err)
	}

	err = m.AddProfile(&DeviceProfile{
		Name:     "medium-priority",
		Priority: 50,
	})
	if err != nil {
		t.Fatalf("AddProfile failed: %v", err)
	}

	// Profiles should be ordered by priority (highest first)
	if len(m.profiles) != 3 {
		t.Fatalf("expected 3 profiles, got %d", len(m.profiles))
	}

	if m.profiles[0].Name != "high-priority" {
		t.Errorf("expected first profile to be high-priority, got %s", m.profiles[0].Name)
	}
	if m.profiles[1].Name != "medium-priority" {
		t.Errorf("expected second profile to be medium-priority, got %s", m.profiles[1].Name)
	}
	if m.profiles[2].Name != "low-priority" {
		t.Errorf("expected third profile to be low-priority, got %s", m.profiles[2].Name)
	}
}

func TestNewLLDPNeighborMatcher(t *testing.T) {
	m := NewLLDPNeighborMatcher()

	if m == nil {
		t.Fatal("NewLLDPNeighborMatcher returned nil")
	}

	if m.knownDevices == nil {
		t.Error("knownDevices map is nil")
	}
}

func TestLLDPNeighborMatcher_AddKnownDevice(t *testing.T) {
	m := NewLLDPNeighborMatcher()

	m.AddKnownDevice("switch-01", "cisco-ios")
	m.AddKnownDevice("Router-01", "juniper-junos")

	// Should be case insensitive
	if _, ok := m.knownDevices["switch-01"]; !ok {
		t.Error("expected switch-01 to be added")
	}
	if _, ok := m.knownDevices["router-01"]; !ok {
		t.Error("expected router-01 to be added (lowercase)")
	}
}

func TestLLDPNeighborMatcher_Match(t *testing.T) {
	m := NewLLDPNeighborMatcher()
	m.AddKnownDevice("switch-01", "cisco-ios")

	// Device with matching neighbor
	device := &DiscoveredDevice{
		ID:      "test-1",
		Address: "192.168.1.1",
		Neighbors: []Neighbor{
			{RemoteDevice: "switch-01", RemotePort: "Gi0/1"},
		},
	}

	match := m.Match(device)
	if match != "cisco-ios" {
		t.Errorf("expected 'cisco-ios', got '%s'", match)
	}

	// Device without matching neighbor
	device2 := &DiscoveredDevice{
		ID:        "test-2",
		Address:   "192.168.1.2",
		Neighbors: []Neighbor{},
	}

	match2 := m.Match(device2)
	if match2 != "" {
		t.Errorf("expected empty string, got '%s'", match2)
	}
}

func TestProfileMatcherInterface(t *testing.T) {
	// Verify all matchers implement ProfileMatcher interface
	var _ ProfileMatcher = (*PatternMatcher)(nil)
	var _ ProfileMatcher = (*CompositeMatcher)(nil)
	var _ ProfileMatcher = (*LLDPNeighborMatcher)(nil)
}
