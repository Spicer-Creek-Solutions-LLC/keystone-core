// Package baremetal provides automated discovery and management of bare metal servers.
package baremetal

import (
	"slices"
	"strings"
)

// HardwareProfile defines criteria for classifying bare metal servers.
type HardwareProfile struct {
	// Name is the profile identifier
	Name string `json:"name"`

	// Description describes the profile
	Description string `json:"description,omitempty"`

	// Priority for matching (higher = checked first)
	Priority int `json:"priority"`

	// Criteria defines hardware matching rules
	Criteria HardwareProfileCriteria `json:"criteria"`

	// Labels to apply when matched
	Labels map[string]string `json:"labels,omitempty"`

	// Pool to assign when matched
	Pool string `json:"pool,omitempty"`
}

// HardwareProfileCriteria defines hardware matching rules.
type HardwareProfileCriteria struct {
	// CPU requirements
	MinCPUCores     int      `json:"min_cpu_cores,omitempty"`
	MaxCPUCores     int      `json:"max_cpu_cores,omitempty"`
	CPUArchitecture []string `json:"cpu_architecture,omitempty"` // x86_64, arm64
	CPUVendors      []string `json:"cpu_vendors,omitempty"`      // Intel, AMD, ARM
	CPUFeatures     []string `json:"cpu_features,omitempty"`     // avx2, aes, etc.

	// Memory requirements
	MinMemoryMB int64    `json:"min_memory_mb,omitempty"`
	MaxMemoryMB int64    `json:"max_memory_mb,omitempty"`
	RequireECC  *bool    `json:"require_ecc,omitempty"`
	MemoryTypes []string `json:"memory_types,omitempty"` // DDR4, DDR5

	// Storage requirements
	MinStorageMB int64    `json:"min_storage_mb,omitempty"`
	MaxStorageMB int64    `json:"max_storage_mb,omitempty"`
	StorageTypes []string `json:"storage_types,omitempty"` // nvme, ssd, hdd
	RequireRAID  *bool    `json:"require_raid,omitempty"`

	// Network requirements
	MinNetworkSpeed int   `json:"min_network_speed,omitempty"` // Mbps
	RequireSRIOV    *bool `json:"require_sriov,omitempty"`

	// GPU requirements
	RequireGPU     *bool    `json:"require_gpu,omitempty"`
	GPUVendors     []string `json:"gpu_vendors,omitempty"` // NVIDIA, AMD
	MinGPUMemoryMB int64    `json:"min_gpu_memory_mb,omitempty"`

	// BMC requirements
	BMCTypes []string `json:"bmc_types,omitempty"` // ipmi, redfish, ilo, idrac
}

// HardwareProfileMatcher matches servers to profiles based on hardware characteristics.
type HardwareProfileMatcher struct {
	profiles []*HardwareProfile
}

// NewHardwareProfileMatcher creates a new hardware profile matcher.
func NewHardwareProfileMatcher() *HardwareProfileMatcher {
	return &HardwareProfileMatcher{
		profiles: make([]*HardwareProfile, 0),
	}
}

// AddProfile adds a profile to the matcher.
// Profiles are kept sorted by priority (higher priority first).
func (m *HardwareProfileMatcher) AddProfile(profile *HardwareProfile) {
	inserted := false
	for i, p := range m.profiles {
		if profile.Priority > p.Priority {
			m.profiles = slices.Insert(m.profiles, i, profile)
			inserted = true
			break
		}
	}
	if !inserted {
		m.profiles = append(m.profiles, profile)
	}
}

// Match finds the first matching profile for a server.
// Returns nil if no profile matches.
func (m *HardwareProfileMatcher) Match(server *Server) *HardwareProfile {
	for _, profile := range m.profiles {
		if m.matchesCriteria(server, &profile.Criteria) {
			return profile
		}
	}
	return nil
}

// GetProfile returns a profile by name.
func (m *HardwareProfileMatcher) GetProfile(name string) *HardwareProfile {
	for _, profile := range m.profiles {
		if profile.Name == name {
			return profile
		}
	}
	return nil
}

// Profiles returns all registered profiles.
func (m *HardwareProfileMatcher) Profiles() []*HardwareProfile {
	result := make([]*HardwareProfile, len(m.profiles))
	copy(result, m.profiles)
	return result
}

// matchesCriteria checks if a server matches the given criteria.
func (m *HardwareProfileMatcher) matchesCriteria(server *Server, criteria *HardwareProfileCriteria) bool {
	hw := &server.Hardware

	// CPU checks
	if criteria.MinCPUCores > 0 && hw.CPU.Cores < criteria.MinCPUCores {
		return false
	}
	if criteria.MaxCPUCores > 0 && hw.CPU.Cores > criteria.MaxCPUCores {
		return false
	}
	if len(criteria.CPUArchitecture) > 0 && !containsIgnoreCase(criteria.CPUArchitecture, hw.CPU.Architecture) {
		return false
	}
	if len(criteria.CPUVendors) > 0 && !containsIgnoreCase(criteria.CPUVendors, hw.CPU.Vendor) {
		return false
	}
	if len(criteria.CPUFeatures) > 0 && !hasAllFeatures(hw.CPU.Features, criteria.CPUFeatures) {
		return false
	}

	// Memory checks
	if criteria.MinMemoryMB > 0 && hw.Memory.TotalMB < criteria.MinMemoryMB {
		return false
	}
	if criteria.MaxMemoryMB > 0 && hw.Memory.TotalMB > criteria.MaxMemoryMB {
		return false
	}
	if criteria.RequireECC != nil && *criteria.RequireECC && !hw.Memory.ECC {
		return false
	}
	if len(criteria.MemoryTypes) > 0 && !containsIgnoreCase(criteria.MemoryTypes, hw.Memory.Type) {
		return false
	}

	// Storage checks
	if criteria.MinStorageMB > 0 && hw.Storage.TotalSizeMB < criteria.MinStorageMB {
		return false
	}
	if criteria.MaxStorageMB > 0 && hw.Storage.TotalSizeMB > criteria.MaxStorageMB {
		return false
	}
	if len(criteria.StorageTypes) > 0 && !hasAnyStorageType(hw.Storage.Devices, criteria.StorageTypes) {
		return false
	}
	if criteria.RequireRAID != nil && *criteria.RequireRAID && !hasRAIDController(hw.Storage.Controllers) {
		return false
	}

	// Network checks
	if criteria.MinNetworkSpeed > 0 && !hasNetworkSpeed(hw.Network.Interfaces, criteria.MinNetworkSpeed) {
		return false
	}
	if criteria.RequireSRIOV != nil && *criteria.RequireSRIOV && !hasSRIOVCapable(hw.Network.Interfaces) {
		return false
	}

	// GPU checks
	if criteria.RequireGPU != nil {
		hasGPU := len(hw.GPUs) > 0
		if *criteria.RequireGPU && !hasGPU {
			return false
		}
		if !*criteria.RequireGPU && hasGPU {
			return false
		}
	}
	if len(criteria.GPUVendors) > 0 && !hasGPUVendor(hw.GPUs, criteria.GPUVendors) {
		return false
	}
	if criteria.MinGPUMemoryMB > 0 && !hasGPUMemory(hw.GPUs, criteria.MinGPUMemoryMB) {
		return false
	}

	// BMC checks
	if len(criteria.BMCTypes) > 0 {
		if hw.BMC == nil {
			return false
		}
		if !containsIgnoreCase(criteria.BMCTypes, hw.BMC.Type) {
			return false
		}
	}

	return true
}

// Helper functions for matching

// containsIgnoreCase checks if slice contains value (case-insensitive).
func containsIgnoreCase(slice []string, value string) bool {
	valueLower := strings.ToLower(value)
	for _, s := range slice {
		if strings.ToLower(s) == valueLower {
			return true
		}
	}
	return false
}

// hasAllFeatures checks if all required features are present.
func hasAllFeatures(available, required []string) bool {
	availableSet := make(map[string]bool)
	for _, f := range available {
		availableSet[strings.ToLower(f)] = true
	}
	for _, r := range required {
		if !availableSet[strings.ToLower(r)] {
			return false
		}
	}
	return true
}

// hasAnyStorageType checks if any storage device matches required types.
func hasAnyStorageType(devices []StorageDevice, types []string) bool {
	for _, device := range devices {
		if containsIgnoreCase(types, device.Type) {
			return true
		}
	}
	return false
}

// hasRAIDController checks if a RAID controller is present.
func hasRAIDController(controllers []StorageController) bool {
	for _, c := range controllers {
		if strings.EqualFold(c.Type, "raid") {
			return true
		}
	}
	return false
}

// hasNetworkSpeed checks if any interface meets minimum speed.
func hasNetworkSpeed(interfaces []NetworkInterface, minSpeed int) bool {
	for _, iface := range interfaces {
		if iface.SpeedMbps >= minSpeed {
			return true
		}
	}
	return false
}

// hasSRIOVCapable checks if any interface supports SR-IOV.
func hasSRIOVCapable(interfaces []NetworkInterface) bool {
	for _, iface := range interfaces {
		if iface.SRIOVCapable {
			return true
		}
	}
	return false
}

// hasGPUVendor checks if any GPU matches required vendors.
func hasGPUVendor(gpus []GPUInfo, vendors []string) bool {
	for _, gpu := range gpus {
		if containsIgnoreCase(vendors, gpu.Vendor) {
			return true
		}
	}
	return false
}

// hasGPUMemory checks if any GPU has minimum memory.
func hasGPUMemory(gpus []GPUInfo, minMemoryMB int64) bool {
	for _, gpu := range gpus {
		if gpu.MemoryMB >= minMemoryMB {
			return true
		}
	}
	return false
}

// DefaultHardwareProfiles returns the default hardware profiles.
func DefaultHardwareProfiles() []*HardwareProfile {
	trueBool := true

	return []*HardwareProfile{
		// GPU compute nodes - highest priority for GPU workloads
		{
			Name:        "compute-gpu",
			Description: "GPU compute nodes for ML/AI workloads",
			Priority:    200,
			Criteria: HardwareProfileCriteria{
				RequireGPU:  &trueBool,
				MinCPUCores: 32,
				MinMemoryMB: 131072, // 128 GB
			},
			Labels: map[string]string{
				"role":     "compute",
				"workload": "gpu",
			},
			Pool: "gpu-pool",
		},
		// High-performance compute
		{
			Name:        "compute-high",
			Description: "High-performance compute nodes",
			Priority:    150,
			Criteria: HardwareProfileCriteria{
				MinCPUCores: 64,
				MinMemoryMB: 262144, // 256 GB
				StorageTypes: []string{"nvme"},
			},
			Labels: map[string]string{
				"role":        "compute",
				"performance": "high",
			},
			Pool: "compute-pool",
		},
		// Hypervisor nodes
		{
			Name:        "hypervisor",
			Description: "Virtualization host nodes",
			Priority:    140,
			Criteria: HardwareProfileCriteria{
				MinCPUCores:  32,
				MinMemoryMB:  262144, // 256 GB
				RequireSRIOV: &trueBool,
			},
			Labels: map[string]string{
				"role": "hypervisor",
			},
			Pool: "hypervisor-pool",
		},
		// Database servers
		{
			Name:        "database",
			Description: "Database server nodes",
			Priority:    130,
			Criteria: HardwareProfileCriteria{
				MinCPUCores:  16,
				MinMemoryMB:  131072, // 128 GB
				RequireECC:   &trueBool,
				StorageTypes: []string{"nvme"},
			},
			Labels: map[string]string{
				"role": "database",
			},
			Pool: "database-pool",
		},
		// Storage-dense nodes
		{
			Name:        "storage-dense",
			Description: "Storage-optimized nodes",
			Priority:    120,
			Criteria: HardwareProfileCriteria{
				MinStorageMB: 10485760, // 10 TB
				RequireRAID:  &trueBool,
			},
			Labels: map[string]string{
				"role": "storage",
			},
			Pool: "storage-pool",
		},
		// Standard compute
		{
			Name:        "compute-standard",
			Description: "Standard compute nodes",
			Priority:    100,
			Criteria: HardwareProfileCriteria{
				MinCPUCores: 16,
				MinMemoryMB: 65536, // 64 GB
			},
			Labels: map[string]string{
				"role":        "compute",
				"performance": "standard",
			},
			Pool: "compute-pool",
		},
		// Edge nodes
		{
			Name:        "edge",
			Description: "Edge/IoT nodes",
			Priority:    50,
			Criteria: HardwareProfileCriteria{
				MaxCPUCores: 8,
				MaxMemoryMB: 32768, // 32 GB
			},
			Labels: map[string]string{
				"role": "edge",
			},
			Pool: "edge-pool",
		},
	}
}

// DefaultHardwareProfileMatcher creates a matcher with default profiles.
func DefaultHardwareProfileMatcher() *HardwareProfileMatcher {
	matcher := NewHardwareProfileMatcher()
	for _, profile := range DefaultHardwareProfiles() {
		matcher.AddProfile(profile)
	}
	return matcher
}
