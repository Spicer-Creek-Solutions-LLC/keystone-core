package baremetal

import (
	"testing"
)

func TestNewHardwareProfileMatcher(t *testing.T) {
	matcher := NewHardwareProfileMatcher()
	if matcher == nil {
		t.Fatal("expected non-nil matcher")
	}
	if len(matcher.profiles) != 0 {
		t.Errorf("expected empty profiles, got %d", len(matcher.profiles))
	}
}

func TestHardwareProfileMatcher_AddProfile(t *testing.T) {
	matcher := NewHardwareProfileMatcher()

	// Add profiles out of priority order
	profile1 := &HardwareProfile{Name: "low", Priority: 10}
	profile2 := &HardwareProfile{Name: "high", Priority: 100}
	profile3 := &HardwareProfile{Name: "medium", Priority: 50}

	matcher.AddProfile(profile1)
	matcher.AddProfile(profile2)
	matcher.AddProfile(profile3)

	// Verify priority ordering
	profiles := matcher.Profiles()
	if len(profiles) != 3 {
		t.Fatalf("expected 3 profiles, got %d", len(profiles))
	}

	if profiles[0].Name != "high" {
		t.Errorf("expected first profile 'high', got '%s'", profiles[0].Name)
	}
	if profiles[1].Name != "medium" {
		t.Errorf("expected second profile 'medium', got '%s'", profiles[1].Name)
	}
	if profiles[2].Name != "low" {
		t.Errorf("expected third profile 'low', got '%s'", profiles[2].Name)
	}
}

func TestHardwareProfileMatcher_GetProfile(t *testing.T) {
	matcher := NewHardwareProfileMatcher()
	matcher.AddProfile(&HardwareProfile{Name: "test-profile", Priority: 10})

	// Found
	profile := matcher.GetProfile("test-profile")
	if profile == nil {
		t.Fatal("expected to find profile")
	}
	if profile.Name != "test-profile" {
		t.Errorf("expected name 'test-profile', got '%s'", profile.Name)
	}

	// Not found
	profile = matcher.GetProfile("nonexistent")
	if profile != nil {
		t.Error("expected nil for nonexistent profile")
	}
}

func TestHardwareProfileMatcher_Match_CPUCriteria(t *testing.T) {
	tests := []struct {
		name       string
		server     *Server
		criteria   HardwareProfileCriteria
		shouldMatch bool
	}{
		{
			name: "min cores - match",
			server: &Server{
				Hardware: HardwareInfo{CPU: CPUInfo{Cores: 16}},
			},
			criteria:    HardwareProfileCriteria{MinCPUCores: 8},
			shouldMatch: true,
		},
		{
			name: "min cores - no match",
			server: &Server{
				Hardware: HardwareInfo{CPU: CPUInfo{Cores: 4}},
			},
			criteria:    HardwareProfileCriteria{MinCPUCores: 8},
			shouldMatch: false,
		},
		{
			name: "max cores - match",
			server: &Server{
				Hardware: HardwareInfo{CPU: CPUInfo{Cores: 4}},
			},
			criteria:    HardwareProfileCriteria{MaxCPUCores: 8},
			shouldMatch: true,
		},
		{
			name: "max cores - no match",
			server: &Server{
				Hardware: HardwareInfo{CPU: CPUInfo{Cores: 16}},
			},
			criteria:    HardwareProfileCriteria{MaxCPUCores: 8},
			shouldMatch: false,
		},
		{
			name: "architecture - match",
			server: &Server{
				Hardware: HardwareInfo{CPU: CPUInfo{Architecture: "x86_64"}},
			},
			criteria:    HardwareProfileCriteria{CPUArchitecture: []string{"x86_64", "arm64"}},
			shouldMatch: true,
		},
		{
			name: "architecture - case insensitive match",
			server: &Server{
				Hardware: HardwareInfo{CPU: CPUInfo{Architecture: "X86_64"}},
			},
			criteria:    HardwareProfileCriteria{CPUArchitecture: []string{"x86_64"}},
			shouldMatch: true,
		},
		{
			name: "architecture - no match",
			server: &Server{
				Hardware: HardwareInfo{CPU: CPUInfo{Architecture: "arm64"}},
			},
			criteria:    HardwareProfileCriteria{CPUArchitecture: []string{"x86_64"}},
			shouldMatch: false,
		},
		{
			name: "vendor - match",
			server: &Server{
				Hardware: HardwareInfo{CPU: CPUInfo{Vendor: "Intel"}},
			},
			criteria:    HardwareProfileCriteria{CPUVendors: []string{"Intel", "AMD"}},
			shouldMatch: true,
		},
		{
			name: "features - match all",
			server: &Server{
				Hardware: HardwareInfo{CPU: CPUInfo{Features: []string{"avx2", "aes", "sse4"}}},
			},
			criteria:    HardwareProfileCriteria{CPUFeatures: []string{"avx2", "aes"}},
			shouldMatch: true,
		},
		{
			name: "features - missing one",
			server: &Server{
				Hardware: HardwareInfo{CPU: CPUInfo{Features: []string{"avx2", "sse4"}}},
			},
			criteria:    HardwareProfileCriteria{CPUFeatures: []string{"avx2", "aes"}},
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewHardwareProfileMatcher()
			matcher.AddProfile(&HardwareProfile{
				Name:     "test",
				Priority: 100,
				Criteria: tt.criteria,
			})

			result := matcher.Match(tt.server)
			matched := result != nil

			if matched != tt.shouldMatch {
				t.Errorf("expected match=%v, got match=%v", tt.shouldMatch, matched)
			}
		})
	}
}

func TestHardwareProfileMatcher_Match_MemoryCriteria(t *testing.T) {
	trueBool := true

	tests := []struct {
		name        string
		server      *Server
		criteria    HardwareProfileCriteria
		shouldMatch bool
	}{
		{
			name: "min memory - match",
			server: &Server{
				Hardware: HardwareInfo{Memory: MemoryInfo{TotalMB: 65536}},
			},
			criteria:    HardwareProfileCriteria{MinMemoryMB: 32768},
			shouldMatch: true,
		},
		{
			name: "min memory - no match",
			server: &Server{
				Hardware: HardwareInfo{Memory: MemoryInfo{TotalMB: 16384}},
			},
			criteria:    HardwareProfileCriteria{MinMemoryMB: 32768},
			shouldMatch: false,
		},
		{
			name: "max memory - match",
			server: &Server{
				Hardware: HardwareInfo{Memory: MemoryInfo{TotalMB: 16384}},
			},
			criteria:    HardwareProfileCriteria{MaxMemoryMB: 32768},
			shouldMatch: true,
		},
		{
			name: "max memory - no match",
			server: &Server{
				Hardware: HardwareInfo{Memory: MemoryInfo{TotalMB: 65536}},
			},
			criteria:    HardwareProfileCriteria{MaxMemoryMB: 32768},
			shouldMatch: false,
		},
		{
			name: "require ECC - match",
			server: &Server{
				Hardware: HardwareInfo{Memory: MemoryInfo{ECC: true}},
			},
			criteria:    HardwareProfileCriteria{RequireECC: &trueBool},
			shouldMatch: true,
		},
		{
			name: "require ECC - no match",
			server: &Server{
				Hardware: HardwareInfo{Memory: MemoryInfo{ECC: false}},
			},
			criteria:    HardwareProfileCriteria{RequireECC: &trueBool},
			shouldMatch: false,
		},
		{
			name: "memory type - match",
			server: &Server{
				Hardware: HardwareInfo{Memory: MemoryInfo{Type: "DDR5"}},
			},
			criteria:    HardwareProfileCriteria{MemoryTypes: []string{"DDR4", "DDR5"}},
			shouldMatch: true,
		},
		{
			name: "memory type - no match",
			server: &Server{
				Hardware: HardwareInfo{Memory: MemoryInfo{Type: "DDR3"}},
			},
			criteria:    HardwareProfileCriteria{MemoryTypes: []string{"DDR4", "DDR5"}},
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewHardwareProfileMatcher()
			matcher.AddProfile(&HardwareProfile{
				Name:     "test",
				Priority: 100,
				Criteria: tt.criteria,
			})

			result := matcher.Match(tt.server)
			matched := result != nil

			if matched != tt.shouldMatch {
				t.Errorf("expected match=%v, got match=%v", tt.shouldMatch, matched)
			}
		})
	}
}

func TestHardwareProfileMatcher_Match_StorageCriteria(t *testing.T) {
	trueBool := true

	tests := []struct {
		name        string
		server      *Server
		criteria    HardwareProfileCriteria
		shouldMatch bool
	}{
		{
			name: "min storage - match",
			server: &Server{
				Hardware: HardwareInfo{Storage: StorageInfo{TotalSizeMB: 1048576}}, // 1 TB
			},
			criteria:    HardwareProfileCriteria{MinStorageMB: 524288}, // 512 GB
			shouldMatch: true,
		},
		{
			name: "min storage - no match",
			server: &Server{
				Hardware: HardwareInfo{Storage: StorageInfo{TotalSizeMB: 262144}}, // 256 GB
			},
			criteria:    HardwareProfileCriteria{MinStorageMB: 524288},
			shouldMatch: false,
		},
		{
			name: "storage type - match nvme",
			server: &Server{
				Hardware: HardwareInfo{
					Storage: StorageInfo{
						Devices: []StorageDevice{{Type: "nvme"}, {Type: "ssd"}},
					},
				},
			},
			criteria:    HardwareProfileCriteria{StorageTypes: []string{"nvme"}},
			shouldMatch: true,
		},
		{
			name: "storage type - no match",
			server: &Server{
				Hardware: HardwareInfo{
					Storage: StorageInfo{
						Devices: []StorageDevice{{Type: "hdd"}},
					},
				},
			},
			criteria:    HardwareProfileCriteria{StorageTypes: []string{"nvme", "ssd"}},
			shouldMatch: false,
		},
		{
			name: "require RAID - match",
			server: &Server{
				Hardware: HardwareInfo{
					Storage: StorageInfo{
						Controllers: []StorageController{{Type: "raid"}},
					},
				},
			},
			criteria:    HardwareProfileCriteria{RequireRAID: &trueBool},
			shouldMatch: true,
		},
		{
			name: "require RAID - no match",
			server: &Server{
				Hardware: HardwareInfo{
					Storage: StorageInfo{
						Controllers: []StorageController{{Type: "hba"}},
					},
				},
			},
			criteria:    HardwareProfileCriteria{RequireRAID: &trueBool},
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewHardwareProfileMatcher()
			matcher.AddProfile(&HardwareProfile{
				Name:     "test",
				Priority: 100,
				Criteria: tt.criteria,
			})

			result := matcher.Match(tt.server)
			matched := result != nil

			if matched != tt.shouldMatch {
				t.Errorf("expected match=%v, got match=%v", tt.shouldMatch, matched)
			}
		})
	}
}

func TestHardwareProfileMatcher_Match_NetworkCriteria(t *testing.T) {
	trueBool := true

	tests := []struct {
		name        string
		server      *Server
		criteria    HardwareProfileCriteria
		shouldMatch bool
	}{
		{
			name: "min network speed - match",
			server: &Server{
				Hardware: HardwareInfo{
					Network: NetworkInfo{
						Interfaces: []NetworkInterface{{SpeedMbps: 25000}},
					},
				},
			},
			criteria:    HardwareProfileCriteria{MinNetworkSpeed: 10000},
			shouldMatch: true,
		},
		{
			name: "min network speed - no match",
			server: &Server{
				Hardware: HardwareInfo{
					Network: NetworkInfo{
						Interfaces: []NetworkInterface{{SpeedMbps: 1000}},
					},
				},
			},
			criteria:    HardwareProfileCriteria{MinNetworkSpeed: 10000},
			shouldMatch: false,
		},
		{
			name: "require SR-IOV - match",
			server: &Server{
				Hardware: HardwareInfo{
					Network: NetworkInfo{
						Interfaces: []NetworkInterface{{SRIOVCapable: true}},
					},
				},
			},
			criteria:    HardwareProfileCriteria{RequireSRIOV: &trueBool},
			shouldMatch: true,
		},
		{
			name: "require SR-IOV - no match",
			server: &Server{
				Hardware: HardwareInfo{
					Network: NetworkInfo{
						Interfaces: []NetworkInterface{{SRIOVCapable: false}},
					},
				},
			},
			criteria:    HardwareProfileCriteria{RequireSRIOV: &trueBool},
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewHardwareProfileMatcher()
			matcher.AddProfile(&HardwareProfile{
				Name:     "test",
				Priority: 100,
				Criteria: tt.criteria,
			})

			result := matcher.Match(tt.server)
			matched := result != nil

			if matched != tt.shouldMatch {
				t.Errorf("expected match=%v, got match=%v", tt.shouldMatch, matched)
			}
		})
	}
}

func TestHardwareProfileMatcher_Match_GPUCriteria(t *testing.T) {
	trueBool := true
	falseBool := false

	tests := []struct {
		name        string
		server      *Server
		criteria    HardwareProfileCriteria
		shouldMatch bool
	}{
		{
			name: "require GPU - match",
			server: &Server{
				Hardware: HardwareInfo{
					GPUs: []GPUInfo{{Vendor: "NVIDIA", MemoryMB: 16384}},
				},
			},
			criteria:    HardwareProfileCriteria{RequireGPU: &trueBool},
			shouldMatch: true,
		},
		{
			name: "require GPU - no match",
			server: &Server{
				Hardware: HardwareInfo{GPUs: nil},
			},
			criteria:    HardwareProfileCriteria{RequireGPU: &trueBool},
			shouldMatch: false,
		},
		{
			name: "require no GPU - match",
			server: &Server{
				Hardware: HardwareInfo{GPUs: nil},
			},
			criteria:    HardwareProfileCriteria{RequireGPU: &falseBool},
			shouldMatch: true,
		},
		{
			name: "require no GPU - no match",
			server: &Server{
				Hardware: HardwareInfo{
					GPUs: []GPUInfo{{Vendor: "NVIDIA"}},
				},
			},
			criteria:    HardwareProfileCriteria{RequireGPU: &falseBool},
			shouldMatch: false,
		},
		{
			name: "GPU vendor - match",
			server: &Server{
				Hardware: HardwareInfo{
					GPUs: []GPUInfo{{Vendor: "NVIDIA"}},
				},
			},
			criteria:    HardwareProfileCriteria{GPUVendors: []string{"NVIDIA", "AMD"}},
			shouldMatch: true,
		},
		{
			name: "GPU vendor - no match",
			server: &Server{
				Hardware: HardwareInfo{
					GPUs: []GPUInfo{{Vendor: "Intel"}},
				},
			},
			criteria:    HardwareProfileCriteria{GPUVendors: []string{"NVIDIA", "AMD"}},
			shouldMatch: false,
		},
		{
			name: "min GPU memory - match",
			server: &Server{
				Hardware: HardwareInfo{
					GPUs: []GPUInfo{{MemoryMB: 32768}}, // 32 GB
				},
			},
			criteria:    HardwareProfileCriteria{MinGPUMemoryMB: 16384}, // 16 GB
			shouldMatch: true,
		},
		{
			name: "min GPU memory - no match",
			server: &Server{
				Hardware: HardwareInfo{
					GPUs: []GPUInfo{{MemoryMB: 8192}}, // 8 GB
				},
			},
			criteria:    HardwareProfileCriteria{MinGPUMemoryMB: 16384},
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewHardwareProfileMatcher()
			matcher.AddProfile(&HardwareProfile{
				Name:     "test",
				Priority: 100,
				Criteria: tt.criteria,
			})

			result := matcher.Match(tt.server)
			matched := result != nil

			if matched != tt.shouldMatch {
				t.Errorf("expected match=%v, got match=%v", tt.shouldMatch, matched)
			}
		})
	}
}

func TestHardwareProfileMatcher_Match_BMCCriteria(t *testing.T) {
	tests := []struct {
		name        string
		server      *Server
		criteria    HardwareProfileCriteria
		shouldMatch bool
	}{
		{
			name: "BMC type - match",
			server: &Server{
				Hardware: HardwareInfo{
					BMC: &BMCInfo{Type: "redfish"},
				},
			},
			criteria:    HardwareProfileCriteria{BMCTypes: []string{"redfish", "ipmi"}},
			shouldMatch: true,
		},
		{
			name: "BMC type - no match",
			server: &Server{
				Hardware: HardwareInfo{
					BMC: &BMCInfo{Type: "ilo"},
				},
			},
			criteria:    HardwareProfileCriteria{BMCTypes: []string{"redfish", "ipmi"}},
			shouldMatch: false,
		},
		{
			name: "BMC required but missing",
			server: &Server{
				Hardware: HardwareInfo{BMC: nil},
			},
			criteria:    HardwareProfileCriteria{BMCTypes: []string{"redfish"}},
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewHardwareProfileMatcher()
			matcher.AddProfile(&HardwareProfile{
				Name:     "test",
				Priority: 100,
				Criteria: tt.criteria,
			})

			result := matcher.Match(tt.server)
			matched := result != nil

			if matched != tt.shouldMatch {
				t.Errorf("expected match=%v, got match=%v", tt.shouldMatch, matched)
			}
		})
	}
}

func TestHardwareProfileMatcher_Match_Priority(t *testing.T) {
	trueBool := true

	matcher := NewHardwareProfileMatcher()

	// Add profiles - GPU profile has higher priority
	matcher.AddProfile(&HardwareProfile{
		Name:     "compute-standard",
		Priority: 100,
		Criteria: HardwareProfileCriteria{
			MinCPUCores: 16,
			MinMemoryMB: 65536,
		},
	})
	matcher.AddProfile(&HardwareProfile{
		Name:     "compute-gpu",
		Priority: 200,
		Criteria: HardwareProfileCriteria{
			RequireGPU:  &trueBool,
			MinCPUCores: 16,
		},
	})

	// Server with GPU should match compute-gpu (higher priority)
	gpuServer := &Server{
		Hardware: HardwareInfo{
			CPU:    CPUInfo{Cores: 32},
			Memory: MemoryInfo{TotalMB: 131072},
			GPUs:   []GPUInfo{{Vendor: "NVIDIA"}},
		},
	}

	profile := matcher.Match(gpuServer)
	if profile == nil {
		t.Fatal("expected a matching profile")
	}
	if profile.Name != "compute-gpu" {
		t.Errorf("expected 'compute-gpu', got '%s'", profile.Name)
	}

	// Server without GPU should match compute-standard
	nonGPUServer := &Server{
		Hardware: HardwareInfo{
			CPU:    CPUInfo{Cores: 32},
			Memory: MemoryInfo{TotalMB: 131072},
		},
	}

	profile = matcher.Match(nonGPUServer)
	if profile == nil {
		t.Fatal("expected a matching profile")
	}
	if profile.Name != "compute-standard" {
		t.Errorf("expected 'compute-standard', got '%s'", profile.Name)
	}
}

func TestHardwareProfileMatcher_Match_NoMatch(t *testing.T) {
	matcher := NewHardwareProfileMatcher()
	matcher.AddProfile(&HardwareProfile{
		Name:     "high-spec",
		Priority: 100,
		Criteria: HardwareProfileCriteria{
			MinCPUCores: 64,
			MinMemoryMB: 262144,
		},
	})

	// Low-spec server doesn't match
	server := &Server{
		Hardware: HardwareInfo{
			CPU:    CPUInfo{Cores: 4},
			Memory: MemoryInfo{TotalMB: 8192},
		},
	}

	profile := matcher.Match(server)
	if profile != nil {
		t.Errorf("expected no match, got '%s'", profile.Name)
	}
}

func TestHardwareProfileMatcher_Match_MultipleCriteria(t *testing.T) {
	trueBool := true

	matcher := NewHardwareProfileMatcher()
	matcher.AddProfile(&HardwareProfile{
		Name:     "database",
		Priority: 100,
		Criteria: HardwareProfileCriteria{
			MinCPUCores:  16,
			MinMemoryMB:  131072,
			RequireECC:   &trueBool,
			StorageTypes: []string{"nvme"},
		},
		Labels: map[string]string{
			"role": "database",
		},
		Pool: "database-pool",
	})

	// Server that matches all criteria
	server := &Server{
		Hardware: HardwareInfo{
			CPU:    CPUInfo{Cores: 32},
			Memory: MemoryInfo{TotalMB: 262144, ECC: true},
			Storage: StorageInfo{
				Devices: []StorageDevice{{Type: "nvme"}},
			},
		},
	}

	profile := matcher.Match(server)
	if profile == nil {
		t.Fatal("expected a matching profile")
	}
	if profile.Name != "database" {
		t.Errorf("expected 'database', got '%s'", profile.Name)
	}
	if profile.Pool != "database-pool" {
		t.Errorf("expected pool 'database-pool', got '%s'", profile.Pool)
	}
	if profile.Labels["role"] != "database" {
		t.Errorf("expected label role='database', got '%s'", profile.Labels["role"])
	}
}

func TestDefaultHardwareProfiles(t *testing.T) {
	profiles := DefaultHardwareProfiles()
	if len(profiles) == 0 {
		t.Fatal("expected default profiles")
	}

	// Verify we have expected profiles
	names := make(map[string]bool)
	for _, p := range profiles {
		names[p.Name] = true
	}

	expectedNames := []string{
		"compute-gpu",
		"compute-high",
		"hypervisor",
		"database",
		"storage-dense",
		"compute-standard",
		"edge",
	}

	for _, name := range expectedNames {
		if !names[name] {
			t.Errorf("expected default profile '%s' not found", name)
		}
	}
}

func TestDefaultHardwareProfileMatcher(t *testing.T) {
	matcher := DefaultHardwareProfileMatcher()
	if matcher == nil {
		t.Fatal("expected non-nil matcher")
	}

	profiles := matcher.Profiles()
	if len(profiles) == 0 {
		t.Fatal("expected profiles to be loaded")
	}

	// Verify priority ordering - compute-gpu should be first (highest priority)
	if profiles[0].Name != "compute-gpu" {
		t.Errorf("expected first profile 'compute-gpu', got '%s'", profiles[0].Name)
	}
}

func TestDefaultHardwareProfileMatcher_MatchRealServer(t *testing.T) {
	matcher := DefaultHardwareProfileMatcher()

	tests := []struct {
		name           string
		server         *Server
		expectedProfile string
	}{
		{
			name: "GPU server matches compute-gpu",
			server: &Server{
				Hardware: HardwareInfo{
					CPU:    CPUInfo{Cores: 64},
					Memory: MemoryInfo{TotalMB: 262144},
					GPUs:   []GPUInfo{{Vendor: "NVIDIA", MemoryMB: 40960}},
				},
			},
			expectedProfile: "compute-gpu",
		},
		{
			name: "High-spec server matches compute-high",
			server: &Server{
				Hardware: HardwareInfo{
					CPU:    CPUInfo{Cores: 128},
					Memory: MemoryInfo{TotalMB: 524288},
					Storage: StorageInfo{
						Devices: []StorageDevice{{Type: "nvme"}},
					},
				},
			},
			expectedProfile: "compute-high",
		},
		{
			name: "Hypervisor server matches hypervisor",
			server: &Server{
				Hardware: HardwareInfo{
					CPU:    CPUInfo{Cores: 48},
					Memory: MemoryInfo{TotalMB: 393216},
					Network: NetworkInfo{
						Interfaces: []NetworkInterface{{SRIOVCapable: true}},
					},
				},
			},
			expectedProfile: "hypervisor",
		},
		{
			name: "Database server matches database",
			server: &Server{
				Hardware: HardwareInfo{
					CPU:    CPUInfo{Cores: 32},
					Memory: MemoryInfo{TotalMB: 262144, ECC: true},
					Storage: StorageInfo{
						Devices: []StorageDevice{{Type: "nvme"}},
					},
				},
			},
			expectedProfile: "database",
		},
		{
			name: "Storage server matches storage-dense",
			server: &Server{
				Hardware: HardwareInfo{
					CPU:    CPUInfo{Cores: 16},
					Memory: MemoryInfo{TotalMB: 65536},
					Storage: StorageInfo{
						TotalSizeMB: 20971520, // 20 TB
						Controllers: []StorageController{{Type: "raid"}},
					},
				},
			},
			expectedProfile: "storage-dense",
		},
		{
			name: "Standard server matches compute-standard",
			server: &Server{
				Hardware: HardwareInfo{
					CPU:    CPUInfo{Cores: 24},
					Memory: MemoryInfo{TotalMB: 98304},
				},
			},
			expectedProfile: "compute-standard",
		},
		{
			name: "Edge device matches edge",
			server: &Server{
				Hardware: HardwareInfo{
					CPU:    CPUInfo{Cores: 4},
					Memory: MemoryInfo{TotalMB: 16384},
				},
			},
			expectedProfile: "edge",
		},
		{
			name: "Minimal server no match",
			server: &Server{
				Hardware: HardwareInfo{
					CPU:    CPUInfo{Cores: 12},
					Memory: MemoryInfo{TotalMB: 49152}, // 48 GB - between edge max and standard min
				},
			},
			expectedProfile: "", // No match
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := matcher.Match(tt.server)

			if tt.expectedProfile == "" {
				if profile != nil {
					t.Errorf("expected no match, got '%s'", profile.Name)
				}
			} else {
				if profile == nil {
					t.Fatalf("expected profile '%s', got nil", tt.expectedProfile)
				}
				if profile.Name != tt.expectedProfile {
					t.Errorf("expected profile '%s', got '%s'", tt.expectedProfile, profile.Name)
				}
			}
		})
	}
}

// Helper function tests

func TestContainsIgnoreCase(t *testing.T) {
	tests := []struct {
		slice    []string
		value    string
		expected bool
	}{
		{[]string{"Intel", "AMD"}, "intel", true},
		{[]string{"Intel", "AMD"}, "INTEL", true},
		{[]string{"Intel", "AMD"}, "ARM", false},
		{[]string{}, "Intel", false},
	}

	for _, tt := range tests {
		result := containsIgnoreCase(tt.slice, tt.value)
		if result != tt.expected {
			t.Errorf("containsIgnoreCase(%v, %q) = %v, expected %v", tt.slice, tt.value, result, tt.expected)
		}
	}
}

func TestHasAllFeatures(t *testing.T) {
	tests := []struct {
		available []string
		required  []string
		expected  bool
	}{
		{[]string{"avx2", "aes", "sse4"}, []string{"avx2", "aes"}, true},
		{[]string{"avx2", "sse4"}, []string{"avx2", "aes"}, false},
		{[]string{"AVX2", "AES"}, []string{"avx2", "aes"}, true}, // case insensitive
		{[]string{}, []string{"avx2"}, false},
		{[]string{"avx2"}, []string{}, true},
	}

	for _, tt := range tests {
		result := hasAllFeatures(tt.available, tt.required)
		if result != tt.expected {
			t.Errorf("hasAllFeatures(%v, %v) = %v, expected %v", tt.available, tt.required, result, tt.expected)
		}
	}
}
