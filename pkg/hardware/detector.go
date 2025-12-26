package hardware

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

// DefaultDetector is the default hardware detector implementation
type DefaultDetector struct {
	cache    *Info
	cacheAge time.Duration
}

// NewDetector creates a new hardware detector
func NewDetector() Detector {
	return &DefaultDetector{
		cacheAge: 5 * time.Minute,
	}
}

// Detect performs comprehensive hardware detection
func (d *DefaultDetector) Detect() (*Info, error) {
	// Check cache
	if d.cache != nil && time.Since(d.cache.DetectedAt) < d.cacheAge {
		return d.cache, nil
	}

	info := &Info{
		DetectedAt: time.Now(),
	}

	// Detect CPU
	cpuInfo, err := d.DetectCPU()
	if err == nil {
		info.CPU = cpuInfo
	}

	// Detect Memory
	memInfo, err := d.DetectMemory()
	if err == nil {
		info.Memory = memInfo
	}

	// Detect Disks
	diskInfo, err := d.DetectDisks()
	if err == nil {
		info.Disks = diskInfo
	}

	// Detect Network
	netInfo, err := d.DetectNetwork()
	if err == nil {
		info.Network = netInfo
	}

	// Detect System
	sysInfo, err := d.DetectSystem()
	if err == nil {
		info.System = sysInfo
	}

	// Cache the result
	d.cache = info

	return info, nil
}

// DetectCPU detects CPU information
func (d *DefaultDetector) DetectCPU() (*CPUInfo, error) {
	cpuInfo := &CPUInfo{
		Cores:   runtime.NumCPU(),
		Threads: runtime.NumCPU(),
	}

	// Get CPU info from gopsutil
	infos, err := cpu.Info()
	if err != nil {
		return nil, fmt.Errorf("failed to get CPU info: %w", err)
	}

	if len(infos) > 0 {
		cpuInfo.Model = infos[0].ModelName
		cpuInfo.Vendor = infos[0].VendorID
		cpuInfo.MHz = infos[0].Mhz
		cpuInfo.CacheSize = int64(infos[0].CacheSize) * 1024 // KB to bytes
		cpuInfo.Flags = infos[0].Flags

		// Count physical CPUs
		physicalIDs := make(map[string]bool)
		for _, info := range infos {
			physicalIDs[info.PhysicalID] = true
		}
		cpuInfo.Sockets = len(physicalIDs)

		// Store physical IDs (convert string to int if possible)
		for id := range physicalIDs {
			// PhysicalID might be a string, try to convert
			var intID int
			fmt.Sscanf(id, "%d", &intID)
			cpuInfo.PhysicalIDs = append(cpuInfo.PhysicalIDs, intID)
		}

		// Calculate cores per socket
		if cpuInfo.Sockets > 0 {
			cpuInfo.Cores = len(infos) / cpuInfo.Sockets
		}
	}

	return cpuInfo, nil
}

// DetectMemory detects memory information
func (d *DefaultDetector) DetectMemory() (*MemoryInfo, error) {
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("failed to get memory info: %w", err)
	}

	swapStat, err := mem.SwapMemory()
	if err != nil {
		// Swap might not be available, continue without it
		swapStat = &mem.SwapMemoryStat{}
	}

	memInfo := &MemoryInfo{
		Total:       vmStat.Total,
		Available:   vmStat.Available,
		Used:        vmStat.Used,
		UsedPercent: vmStat.UsedPercent,
		SwapTotal:   swapStat.Total,
		SwapFree:    swapStat.Free,
		SwapUsed:    swapStat.Used,
	}

	return memInfo, nil
}

// DetectDisks detects disk/storage information
func (d *DefaultDetector) DetectDisks() ([]DiskInfo, error) {
	var disks []DiskInfo

	// Get partition information
	partitions, err := disk.Partitions(false)
	if err != nil {
		return nil, fmt.Errorf("failed to get disk partitions: %w", err)
	}

	for _, partition := range partitions {
		// Get usage stats for this partition
		usage, err := disk.Usage(partition.Mountpoint)
		if err != nil {
			// Skip partitions we can't get usage for
			continue
		}

		diskInfo := DiskInfo{
			Device:      partition.Device,
			Mountpoint:  partition.Mountpoint,
			Filesystem:  partition.Fstype,
			UsedBytes:   usage.Used,
			FreeBytes:   usage.Free,
			UsedPercent: usage.UsedPercent,
		}

		// Try to get more detailed disk info
		if ioCounters, err := disk.IOCounters(partition.Device); err == nil {
			if counter, ok := ioCounters[partition.Device]; ok {
				diskInfo.Serial = counter.SerialNumber
			}
		}

		disks = append(disks, diskInfo)
	}

	return disks, nil
}

// DetectNetwork detects network interface information
func (d *DefaultDetector) DetectNetwork() ([]NetworkInfo, error) {
	var networks []NetworkInfo

	// Get network interfaces
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
	}

	for _, iface := range interfaces {
		// Skip loopback
		if strings.Contains(strings.ToLower(iface.Name), "lo") {
			continue
		}

		netInfo := NetworkInfo{
			Name:         iface.Name,
			HardwareAddr: iface.HardwareAddr,
			MTU:          iface.MTU,
			Flags:        iface.Flags,
		}

		// Get IP addresses
		for _, addr := range iface.Addrs {
			netInfo.Addresses = append(netInfo.Addresses, addr.Addr)
		}

		networks = append(networks, netInfo)
	}

	return networks, nil
}

// DetectSystem detects system/motherboard information
func (d *DefaultDetector) DetectSystem() (*SystemInfo, error) {
	// Get host information
	hostInfo, err := host.Info()
	if err != nil {
		return nil, fmt.Errorf("failed to get host info: %w", err)
	}

	sysInfo := &SystemInfo{
		Manufacturer: hostInfo.Platform,
		ProductName:  hostInfo.PlatformFamily,
		Version:      hostInfo.PlatformVersion,
		UUID:         hostInfo.HostID,
	}

	return sysInfo, nil
}

// DetectBMC detects BMC/IPMI information
func (d *DefaultDetector) DetectBMC() (*BMCInfo, error) {
	// BMC detection requires IPMI tools or specialized libraries
	// For now, return a placeholder indicating BMC detection is not yet implemented
	bmcInfo := &BMCInfo{
		Present: false,
	}

	// TODO: Implement IPMI detection using ipmitool or go-ipmi library
	// This would require:
	// 1. Check if IPMI is available (ipmitool presence)
	// 2. Query BMC information (ipmitool bmc info)
	// 3. Get network configuration (ipmitool lan print)
	// 4. Get firmware version

	return bmcInfo, nil
}

// Global default detector instance
var defaultDetector Detector = NewDetector()

// Detect performs hardware detection using the default detector
func Detect() (*Info, error) {
	return defaultDetector.Detect()
}

// DetectCPU detects CPU information using the default detector
func DetectCPU() (*CPUInfo, error) {
	return defaultDetector.DetectCPU()
}

// DetectMemory detects memory information using the default detector
func DetectMemory() (*MemoryInfo, error) {
	return defaultDetector.DetectMemory()
}

// DetectDisks detects disk information using the default detector
func DetectDisks() ([]DiskInfo, error) {
	return defaultDetector.DetectDisks()
}

// DetectNetwork detects network information using the default detector
func DetectNetwork() ([]NetworkInfo, error) {
	return defaultDetector.DetectNetwork()
}

// DetectSystem detects system information using the default detector
func DetectSystem() (*SystemInfo, error) {
	return defaultDetector.DetectSystem()
}

// DetectBMC detects BMC information using the default detector
func DetectBMC() (*BMCInfo, error) {
	return defaultDetector.DetectBMC()
}
