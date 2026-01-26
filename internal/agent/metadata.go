package agent

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"

	"github.com/shawnbutts/keystone-core/internal/hardware"
	"github.com/shawnbutts/keystone-core/internal/platform"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

// NetworkInfo contains detailed network addressing information
type NetworkInfo struct {
	IPv4Addresses []string // All IPv4 addresses (excluding link-local)
	IPv6Addresses []string // All IPv6 addresses (excluding link-local)
	AllAddresses  []string // Combined list for backward compatibility
	IsDualStack   bool     // True if both IPv4 and IPv6 addresses are available
}

// Metadata represents agent system metadata
type Metadata struct {
	Hostname        string
	OS              string
	Architecture    string
	IPAddresses     []string // Deprecated: use NetworkInfo.AllAddresses
	IPv4Addresses   []string // All IPv4 addresses (excluding link-local)
	IPv6Addresses   []string // All IPv6 addresses (excluding link-local)
	IsDualStack     bool     // True if both address families available
	PlatformVersion string
	AgentVersion    string
	Labels          map[string]string
	// Platform detection information
	Distro             string
	DistroVersion      string
	PackageManager     string
	InitSystem         string
	KernelVersion      string
	IsVirtual          bool
	VirtualizationType string
	IsContainer        bool
	ContainerType      string
	// Hardware information
	CPUModel        string
	CPUCores        int
	CPUThreads      int
	MemoryTotal     uint64 // bytes
	MemoryAvailable uint64 // bytes
	DiskCount       int
	NetworkCount    int
	SystemVendor    string
	SystemProduct   string
	SystemUUID      string
}

// CollectMetadata gathers system information about the agent
func CollectMetadata() (*Metadata, error) {
	metadata := &Metadata{
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		AgentVersion: version.Get().Version,
		Labels:       make(map[string]string),
	}

	// Get hostname
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %w", err)
	}
	metadata.Hostname = hostname

	// Get platform version (simplified - could be more detailed per OS)
	metadata.PlatformVersion = runtime.Version()

	// Get IP addresses with IPv4/IPv6 separation
	networkInfo, err := CollectNetworkInfo()
	if err != nil {
		// Don't fail, just log warning
		fmt.Printf("Warning: failed to get IP addresses: %v\n", err)
		networkInfo = &NetworkInfo{}
	}
	metadata.IPAddresses = networkInfo.AllAddresses // Backward compatibility
	metadata.IPv4Addresses = networkInfo.IPv4Addresses
	metadata.IPv6Addresses = networkInfo.IPv6Addresses
	metadata.IsDualStack = networkInfo.IsDualStack

	// Collect platform information using platform detection
	platformInfo, err := platform.Detect()
	if err != nil {
		// Don't fail, just log warning
		fmt.Printf("Warning: failed to detect platform: %v\n", err)
	} else {
		metadata.Distro = platformInfo.Distro.String()
		metadata.DistroVersion = platformInfo.Version
		metadata.PackageManager = platformInfo.PackageManager.String()
		metadata.InitSystem = platformInfo.InitSystem.String()
		metadata.KernelVersion = platformInfo.KernelVersion
		metadata.IsVirtual = platformInfo.IsVirtual
		metadata.VirtualizationType = platformInfo.VirtualizationType
		metadata.IsContainer = platformInfo.IsContainer
		metadata.ContainerType = platformInfo.ContainerType
	}

	// Collect hardware information using hardware detection
	hwInfo, err := hardware.Detect()
	if err != nil {
		// Don't fail, just log warning
		fmt.Printf("Warning: failed to detect hardware: %v\n", err)
	} else {
		if hwInfo.CPU != nil {
			metadata.CPUModel = hwInfo.CPU.Model
			metadata.CPUCores = hwInfo.CPU.Cores
			metadata.CPUThreads = hwInfo.CPU.Threads
		}
		if hwInfo.Memory != nil {
			metadata.MemoryTotal = hwInfo.Memory.Total
			metadata.MemoryAvailable = hwInfo.Memory.Available
		}
		metadata.DiskCount = len(hwInfo.Disks)
		metadata.NetworkCount = len(hwInfo.Network)
		if hwInfo.System != nil {
			metadata.SystemVendor = hwInfo.System.Manufacturer
			metadata.SystemProduct = hwInfo.System.ProductName
			metadata.SystemUUID = hwInfo.System.UUID
		}
	}

	return metadata, nil
}

// CollectNetworkInfo gathers network information with separate IPv4/IPv6 addresses
func CollectNetworkInfo() (*NetworkInfo, error) {
	info := &NetworkInfo{
		IPv4Addresses: []string{},
		IPv6Addresses: []string{},
		AllAddresses:  []string{},
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
	}

	for _, iface := range interfaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
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
			}

			if ip == nil || ip.IsLoopback() {
				continue
			}

			// Check for IPv4
			if ip4 := ip.To4(); ip4 != nil {
				// Skip IPv4 link-local (169.254.x.x)
				if isIPv4LinkLocal(ip4) {
					continue
				}
				info.IPv4Addresses = append(info.IPv4Addresses, ip4.String())
				info.AllAddresses = append(info.AllAddresses, ip4.String())
			} else if ip6 := ip.To16(); ip6 != nil {
				// Skip IPv6 link-local (fe80::/10)
				if isIPv6LinkLocal(ip6) {
					continue
				}
				info.IPv6Addresses = append(info.IPv6Addresses, ip6.String())
				info.AllAddresses = append(info.AllAddresses, ip6.String())
			}
		}
	}

	// Determine dual-stack status
	info.IsDualStack = len(info.IPv4Addresses) > 0 && len(info.IPv6Addresses) > 0

	return info, nil
}

// isIPv4LinkLocal returns true if the IP is a link-local address (169.254.0.0/16)
func isIPv4LinkLocal(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		return ip4[0] == 169 && ip4[1] == 254
	}
	return false
}

// isIPv6LinkLocal returns true if the IP is a link-local address (fe80::/10)
func isIPv6LinkLocal(ip net.IP) bool {
	// Link-local IPv6 addresses start with fe80::/10
	// This means first 10 bits are 1111111010, so first byte is 0xfe
	// and second byte has upper 2 bits as 10 (0x80-0xbf range)
	if len(ip) != net.IPv6len {
		return false
	}
	return ip[0] == 0xfe && (ip[1]&0xc0) == 0x80
}

// getIPAddresses is deprecated, use CollectNetworkInfo instead
func getIPAddresses() ([]string, error) {
	info, err := CollectNetworkInfo()
	if err != nil {
		return nil, err
	}
	return info.AllAddresses, nil
}

// SystemMetrics represents current system resource usage
type SystemMetrics struct {
	CPUPercent    float32
	MemoryPercent float32
	DiskPercent   float32
	LoadAverage   []float32
	NumGoroutines int
	MemoryUsage   uint64
}

// CollectMetrics gathers current system metrics using gopsutil
func CollectMetrics() (*SystemMetrics, error) {
	metrics := &SystemMetrics{
		NumGoroutines: runtime.NumGoroutine(),
	}

	// Get Go runtime memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	metrics.MemoryUsage = memStats.Alloc

	// Get CPU usage (percentage over 200ms interval)
	cpuPercents, err := cpu.Percent(200*time.Millisecond, false)
	if err == nil && len(cpuPercents) > 0 {
		metrics.CPUPercent = float32(cpuPercents[0])
	}

	// Get memory usage
	memInfo, err := mem.VirtualMemory()
	if err == nil {
		metrics.MemoryPercent = float32(memInfo.UsedPercent)
	}

	// Get disk usage (root filesystem)
	diskInfo, err := disk.Usage("/")
	if err == nil {
		metrics.DiskPercent = float32(diskInfo.UsedPercent)
	}

	// Get load average (Unix systems only, returns 0 on Windows)
	loadInfo, err := load.Avg()
	if err == nil && loadInfo != nil {
		metrics.LoadAverage = []float32{
			float32(loadInfo.Load1),
			float32(loadInfo.Load5),
			float32(loadInfo.Load15),
		}
	} else {
		metrics.LoadAverage = []float32{0.0, 0.0, 0.0}
	}

	return metrics, nil
}

// CollectMetricsNonBlocking gathers system metrics without blocking for CPU measurement.
// Use this for frequent polling where the 200ms CPU measurement delay is not acceptable.
func CollectMetricsNonBlocking() (*SystemMetrics, error) {
	metrics := &SystemMetrics{
		NumGoroutines: runtime.NumGoroutine(),
	}

	// Get Go runtime memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	metrics.MemoryUsage = memStats.Alloc

	// Get CPU usage (instantaneous - less accurate but non-blocking)
	cpuPercents, err := cpu.Percent(0, false)
	if err == nil && len(cpuPercents) > 0 {
		metrics.CPUPercent = float32(cpuPercents[0])
	}

	// Get memory usage
	memInfo, err := mem.VirtualMemory()
	if err == nil {
		metrics.MemoryPercent = float32(memInfo.UsedPercent)
	}

	// Get disk usage (root filesystem)
	diskInfo, err := disk.Usage("/")
	if err == nil {
		metrics.DiskPercent = float32(diskInfo.UsedPercent)
	}

	// Get load average
	loadInfo, err := load.Avg()
	if err == nil && loadInfo != nil {
		metrics.LoadAverage = []float32{
			float32(loadInfo.Load1),
			float32(loadInfo.Load5),
			float32(loadInfo.Load15),
		}
	} else {
		metrics.LoadAverage = []float32{0.0, 0.0, 0.0}
	}

	return metrics, nil
}
