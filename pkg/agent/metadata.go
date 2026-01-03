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

	"github.com/shawnbutts/keystone-core/pkg/hardware"
	"github.com/shawnbutts/keystone-core/pkg/platform"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

// Metadata represents agent system metadata
type Metadata struct {
	Hostname        string
	OS              string
	Architecture    string
	IPAddresses     []string
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
	CPUModel       string
	CPUCores       int
	CPUThreads     int
	MemoryTotal    uint64 // bytes
	MemoryAvailable uint64 // bytes
	DiskCount      int
	NetworkCount   int
	SystemVendor   string
	SystemProduct  string
	SystemUUID     string
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

	// Get IP addresses
	ipAddresses, err := getIPAddresses()
	if err != nil {
		// Don't fail, just log warning
		fmt.Printf("Warning: failed to get IP addresses: %v\n", err)
		ipAddresses = []string{}
	}
	metadata.IPAddresses = ipAddresses

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

// getIPAddresses retrieves all non-loopback IP addresses
func getIPAddresses() ([]string, error) {
	var ips []string

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

			// Prefer IPv4, but include IPv6
			ips = append(ips, ip.String())
		}
	}

	return ips, nil
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
