package hardware

import "time"

// Info represents comprehensive hardware information
type Info struct {
	CPU        *CPUInfo
	Memory     *MemoryInfo
	Disks      []DiskInfo
	Network    []NetworkInfo
	System     *SystemInfo
	DetectedAt time.Time
}

// CPUInfo represents CPU information
type CPUInfo struct {
	Model       string
	Vendor      string
	Cores       int
	Threads     int
	MHz         float64
	CacheSize   int64 // bytes
	Flags       []string
	Sockets     int
	PhysicalIDs []int
}

// MemoryInfo represents system memory information
type MemoryInfo struct {
	Total       uint64 // bytes
	Available   uint64 // bytes
	Used        uint64 // bytes
	UsedPercent float64
	SwapTotal   uint64 // bytes
	SwapFree    uint64 // bytes
	SwapUsed    uint64 // bytes
}

// DiskInfo represents disk/storage device information
type DiskInfo struct {
	Device      string
	Model       string
	Size        uint64 // bytes
	Type        string // HDD, SSD, NVMe
	Serial      string
	Vendor      string
	Mountpoint  string
	Filesystem  string
	UsedBytes   uint64
	FreeBytes   uint64
	UsedPercent float64
	Rotational  bool
	ReadOnly    bool
}

// NetworkInfo represents network interface information
type NetworkInfo struct {
	Name         string
	HardwareAddr string // MAC address
	MTU          int
	Flags        []string // up, broadcast, multicast, etc.
	Addresses    []string // IP addresses
	Speed        int64    // Mbps
	Duplex       string   // full, half, unknown
	Driver       string
	DriverVersion string
	FirmwareVersion string
	PCIAddress   string
}

// SystemInfo represents system/motherboard information
type SystemInfo struct {
	Manufacturer string
	ProductName  string
	Version      string
	SerialNumber string
	UUID         string
	SKU          string
	Family       string
	BoardVendor  string
	BoardName    string
	BoardVersion string
	BoardSerial  string
	ChassisType  string
	BIOSVendor   string
	BIOSVersion  string
	BIOSDate     string
}

// BMCInfo represents Baseboard Management Controller information
type BMCInfo struct {
	Present         bool
	IPAddress       string
	MACAddress      string
	FirmwareVersion string
	Manufacturer    string
	ProductID       string
}

// Detector interface for hardware detection
type Detector interface {
	// Detect performs comprehensive hardware detection
	Detect() (*Info, error)

	// DetectCPU detects CPU information
	DetectCPU() (*CPUInfo, error)

	// DetectMemory detects memory information
	DetectMemory() (*MemoryInfo, error)

	// DetectDisks detects disk/storage information
	DetectDisks() ([]DiskInfo, error)

	// DetectNetwork detects network interface information
	DetectNetwork() ([]NetworkInfo, error)

	// DetectSystem detects system/motherboard information
	DetectSystem() (*SystemInfo, error)

	// DetectBMC detects BMC/IPMI information
	DetectBMC() (*BMCInfo, error)
}
