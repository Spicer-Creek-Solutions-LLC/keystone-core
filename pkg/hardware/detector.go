package hardware

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

// fileExists checks if a file exists
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// dirExists checks if a directory exists
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// execLookPath looks for an executable in PATH
func execLookPath(name string) (string, error) {
	return exec.LookPath(name)
}

// execCommand runs a command and returns its output
func execCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

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
	bmcInfo := &BMCInfo{
		Present: false,
	}

	// Try multiple detection methods in order of reliability

	// Method 1: Check for IPMI device files (Linux)
	if runtime.GOOS == "linux" {
		if d.detectIPMIDevice() {
			bmcInfo.Present = true
		}
	}

	// Method 2: Try ipmitool if available
	if d.hasIPMITool() {
		info, err := d.detectBMCViaIPMITool()
		if err == nil && info != nil {
			return info, nil
		}
		// If ipmitool exists but failed, BMC might still be present
		// but not accessible (permissions, etc.)
		if bmcInfo.Present {
			return bmcInfo, nil
		}
	}

	// Method 3: Check DMI/SMBIOS for IPMI interface (Linux)
	if runtime.GOOS == "linux" {
		if d.detectIPMIViaDMI() {
			bmcInfo.Present = true
		}
	}

	return bmcInfo, nil
}

// detectIPMIDevice checks for IPMI device files on Linux
func (d *DefaultDetector) detectIPMIDevice() bool {
	// Common IPMI device paths
	ipmiDevices := []string{
		"/dev/ipmi0",
		"/dev/ipmi/0",
		"/dev/ipmidev/0",
	}

	for _, device := range ipmiDevices {
		if fileExists(device) {
			return true
		}
	}

	// Check sysfs for IPMI devices
	if dirExists("/sys/class/ipmi") {
		return true
	}

	return false
}

// detectIPMIViaDMI checks DMI/SMBIOS for IPMI interface information
func (d *DefaultDetector) detectIPMIViaDMI() bool {
	// Check DMI type 38 (IPMI Device Information)
	dmiPath := "/sys/class/dmi/id/product_name"
	if !fileExists(dmiPath) {
		return false
	}

	// Check for common BMC-related files in sysfs
	bmcIndicators := []string{
		"/sys/devices/platform/ipmi_si.0",
		"/sys/devices/platform/ipmi_ssif.0",
		"/sys/module/ipmi_si",
		"/sys/module/ipmi_ssif",
		"/sys/module/ipmi_devintf",
	}

	for _, indicator := range bmcIndicators {
		if fileExists(indicator) || dirExists(indicator) {
			return true
		}
	}

	return false
}

// hasIPMITool checks if ipmitool is available in PATH
func (d *DefaultDetector) hasIPMITool() bool {
	_, err := execLookPath("ipmitool")
	return err == nil
}

// detectBMCViaIPMITool uses ipmitool to get detailed BMC information
func (d *DefaultDetector) detectBMCViaIPMITool() (*BMCInfo, error) {
	bmcInfo := &BMCInfo{
		Present: true,
	}

	// Get BMC info
	bmcOutput, err := execCommand("ipmitool", "bmc", "info")
	if err != nil {
		return nil, fmt.Errorf("failed to run ipmitool bmc info: %w", err)
	}

	// Parse BMC info output
	parseBMCInfo(bmcOutput, bmcInfo)

	// Get LAN configuration (channel 1 is most common)
	lanOutput, err := execCommand("ipmitool", "lan", "print", "1")
	if err == nil {
		parseLANInfo(lanOutput, bmcInfo)
	}

	return bmcInfo, nil
}

// parseBMCInfo parses the output of 'ipmitool bmc info'
func parseBMCInfo(output string, info *BMCInfo) {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "Firmware Revision":
			info.FirmwareVersion = value
		case "Manufacturer ID":
			info.Manufacturer = value
		case "Manufacturer Name":
			// Prefer name over ID
			if info.Manufacturer == "" || !strings.Contains(info.Manufacturer, "(") {
				info.Manufacturer = value
			}
		case "Product ID":
			info.ProductID = value
		case "Product Name":
			// Append to ProductID if available
			if info.ProductID != "" {
				info.ProductID = info.ProductID + " (" + value + ")"
			} else {
				info.ProductID = value
			}
		}
	}
}

// parseLANInfo parses the output of 'ipmitool lan print'
func parseLANInfo(output string, info *BMCInfo) {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "IP Address":
			info.IPAddress = value
		case "MAC Address":
			info.MACAddress = value
		}
	}
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
