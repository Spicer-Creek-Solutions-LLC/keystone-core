package platform

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// DefaultDetector is the default platform detector implementation
type DefaultDetector struct {
	cache    *Info
	cacheAge time.Duration
}

// NewDetector creates a new platform detector
func NewDetector() Detector {
	return &DefaultDetector{
		cacheAge: 5 * time.Minute,
	}
}

// Detect performs comprehensive platform detection
func (d *DefaultDetector) Detect() (*Info, error) {
	// Check cache
	if d.cache != nil && time.Since(d.cache.DetectedAt) < d.cacheAge {
		return d.cache, nil
	}

	info := &Info{
		DetectedAt: time.Now(),
		Metadata:   make(map[string]interface{}),
	}

	// Detect OS
	osType, err := d.DetectOS()
	if err != nil {
		return nil, fmt.Errorf("failed to detect OS: %w", err)
	}
	info.OS = osType

	// Detect architecture
	arch, err := d.DetectArch()
	if err != nil {
		return nil, fmt.Errorf("failed to detect architecture: %w", err)
	}
	info.Arch = arch

	// Detect distro (Linux only)
	if info.OS == OSLinux {
		distro, version, err := d.DetectDistro()
		if err == nil {
			info.Distro = distro
			info.Version = version
			info.PlatformFamily = getPlatformFamily(distro)
		}
	}

	// Detect package manager
	pkgMgr, err := d.DetectPackageManager()
	if err == nil {
		info.PackageManager = pkgMgr
	}

	// Detect init system
	initSys, err := d.DetectInitSystem()
	if err == nil {
		info.InitSystem = initSys
	}

	// Detect hostname
	hostname, err := os.Hostname()
	if err == nil {
		info.Hostname = hostname
	}

	// Detect kernel version
	info.KernelVersion = detectKernelVersion()

	// Detect virtualization
	isVirtual, virtType, err := d.IsVirtualMachine()
	if err == nil {
		info.IsVirtual = isVirtual
		info.VirtualizationType = virtType
	}

	// Detect containerization
	isContainer, containerType, err := d.IsContainer()
	if err == nil {
		info.IsContainer = isContainer
		info.ContainerType = containerType
	}

	// Cache the result
	d.cache = info

	return info, nil
}

// DetectOS detects the operating system type
func (d *DefaultDetector) DetectOS() (OSType, error) {
	return NormalizeOSType(runtime.GOOS), nil
}

// DetectArch detects the CPU architecture
func (d *DefaultDetector) DetectArch() (ArchType, error) {
	return NormalizeArchType(runtime.GOARCH), nil
}

// DetectDistro detects the Linux distribution
func (d *DefaultDetector) DetectDistro() (DistroType, string, error) {
	if runtime.GOOS != "linux" {
		return DistroUnknown, "", nil
	}

	// Try /etc/os-release first (modern standard)
	distro, version := parseOSRelease("/etc/os-release")
	if distro != DistroUnknown {
		return distro, version, nil
	}

	// Fallback to /etc/lsb-release
	distro, version = parseLSBRelease("/etc/lsb-release")
	if distro != DistroUnknown {
		return distro, version, nil
	}

	// Fallback to specific distro files
	distro, version = detectDistroFromFiles()
	return distro, version, nil
}

// DetectPackageManager detects the system package manager
func (d *DefaultDetector) DetectPackageManager() (PackageManager, error) {
	switch runtime.GOOS {
	case "linux":
		return detectLinuxPackageManager()
	case "darwin":
		if commandExists("brew") {
			return PackageManagerBrew, nil
		}
		return PackageManagerUnknown, nil
	case "windows":
		if commandExists("choco") {
			return PackageManagerChocolatey, nil
		}
		if commandExists("winget") {
			return PackageManagerWinget, nil
		}
		return PackageManagerUnknown, nil
	default:
		return PackageManagerUnknown, nil
	}
}

// DetectInitSystem detects the system init system
func (d *DefaultDetector) DetectInitSystem() (InitSystem, error) {
	switch runtime.GOOS {
	case "linux":
		return detectLinuxInitSystem()
	case "darwin":
		return InitLaunchd, nil
	case "windows":
		return InitWindowsService, nil
	default:
		return InitUnknown, nil
	}
}

// IsVirtualMachine checks if running in a virtual machine
func (d *DefaultDetector) IsVirtualMachine() (bool, string, error) {
	switch runtime.GOOS {
	case "linux":
		return detectLinuxVirtualization()
	case "darwin":
		// Check for common VM indicators on macOS
		if checkFileContains("/proc/cpuinfo", "hypervisor") {
			return true, "unknown", nil
		}
		return false, "", nil
	case "windows":
		// Windows VM detection would use WMI
		return false, "", nil
	default:
		return false, "", nil
	}
}

// IsContainer checks if running in a container
func (d *DefaultDetector) IsContainer() (bool, string, error) {
	switch runtime.GOOS {
	case "linux":
		return detectLinuxContainer()
	default:
		return false, "", nil
	}
}

// Helper functions

func parseOSRelease(path string) (DistroType, string) {
	file, err := os.Open(path)
	if err != nil {
		return DistroUnknown, ""
	}
	defer file.Close()

	var id, version string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ID=") {
			id = strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
		}
		if strings.HasPrefix(line, "VERSION_ID=") {
			version = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
		}
	}

	return normalizeDistroID(id), version
}

func parseLSBRelease(path string) (DistroType, string) {
	file, err := os.Open(path)
	if err != nil {
		return DistroUnknown, ""
	}
	defer file.Close()

	var id, version string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "DISTRIB_ID=") {
			id = strings.ToLower(strings.TrimPrefix(line, "DISTRIB_ID="))
		}
		if strings.HasPrefix(line, "DISTRIB_RELEASE=") {
			version = strings.TrimPrefix(line, "DISTRIB_RELEASE=")
		}
	}

	return normalizeDistroID(id), version
}

func detectDistroFromFiles() (DistroType, string) {
	// Check for specific distro release files
	distroFiles := map[string]DistroType{
		"/etc/redhat-release": DistroCentOS,
		"/etc/centos-release": DistroCentOS,
		"/etc/fedora-release": DistroFedora,
		"/etc/debian_version":  DistroDebian,
		"/etc/alpine-release":  DistroAlpine,
		"/etc/arch-release":    DistroArch,
	}

	for file, distro := range distroFiles {
		if _, err := os.Stat(file); err == nil {
			version := readFirstLine(file)
			return distro, version
		}
	}

	return DistroUnknown, ""
}

func normalizeDistroID(id string) DistroType {
	id = strings.ToLower(id)
	switch {
	case strings.Contains(id, "ubuntu"):
		return DistroUbuntu
	case strings.Contains(id, "debian"):
		return DistroDebian
	case strings.Contains(id, "centos"):
		return DistroCentOS
	case strings.Contains(id, "rhel") || strings.Contains(id, "redhat"):
		return DistroRHEL
	case strings.Contains(id, "fedora"):
		return DistroFedora
	case strings.Contains(id, "alpine"):
		return DistroAlpine
	case strings.Contains(id, "arch"):
		return DistroArch
	case strings.Contains(id, "opensuse") || strings.Contains(id, "suse"):
		return DistroOpenSUSE
	case strings.Contains(id, "amzn") || strings.Contains(id, "amazon"):
		return DistroAmazonLinux
	default:
		return DistroUnknown
	}
}

func getPlatformFamily(distro DistroType) string {
	switch distro {
	case DistroUbuntu, DistroDebian:
		return "debian"
	case DistroCentOS, DistroRHEL, DistroFedora, DistroAmazonLinux:
		return "rhel"
	case DistroOpenSUSE:
		return "suse"
	case DistroArch:
		return "arch"
	case DistroAlpine:
		return "alpine"
	default:
		return "unknown"
	}
}

func detectLinuxPackageManager() (PackageManager, error) {
	// Check in order of preference
	if commandExists("apt-get") {
		return PackageManagerAPT, nil
	}
	if commandExists("dnf") {
		return PackageManagerDNF, nil
	}
	if commandExists("yum") {
		return PackageManagerYum, nil
	}
	if commandExists("zypper") {
		return PackageManagerZypper, nil
	}
	if commandExists("pacman") {
		return PackageManagerPacman, nil
	}
	if commandExists("apk") {
		return PackageManagerAPK, nil
	}

	return PackageManagerUnknown, nil
}

func detectLinuxInitSystem() (InitSystem, error) {
	// Check for systemd
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		return InitSystemd, nil
	}
	if commandExists("systemctl") {
		return InitSystemd, nil
	}

	// Check for upstart
	if commandExists("initctl") && checkFileContains("/proc/1/comm", "init") {
		return InitUpstart, nil
	}

	// Check for OpenRC
	if commandExists("rc-service") {
		return InitOpenRC, nil
	}

	// Fallback to SysV
	if _, err := os.Stat("/etc/init.d"); err == nil {
		return InitSysV, nil
	}

	return InitUnknown, nil
}

func detectLinuxVirtualization() (bool, string, error) {
	// Check /proc/cpuinfo for hypervisor flag
	if checkFileContains("/proc/cpuinfo", "hypervisor") {
		// Try to determine the type
		if checkFileContains("/sys/class/dmi/id/product_name", "VMware") {
			return true, "vmware", nil
		}
		if checkFileContains("/sys/class/dmi/id/product_name", "VirtualBox") {
			return true, "virtualbox", nil
		}
		if checkFileContains("/sys/class/dmi/id/product_name", "KVM") {
			return true, "kvm", nil
		}
		if checkFileContains("/sys/class/dmi/id/sys_vendor", "QEMU") {
			return true, "qemu", nil
		}
		if checkFileContains("/sys/class/dmi/id/sys_vendor", "Xen") {
			return true, "xen", nil
		}
		if checkFileContains("/sys/class/dmi/id/product_name", "HVM domU") {
			return true, "xen", nil
		}
		return true, "unknown", nil
	}

	return false, "", nil
}

func detectLinuxContainer() (bool, string, error) {
	// Check for Docker
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true, "docker", nil
	}

	// Check cgroup for docker/lxc
	if checkFileContains("/proc/1/cgroup", "docker") {
		return true, "docker", nil
	}
	if checkFileContains("/proc/1/cgroup", "lxc") {
		return true, "lxc", nil
	}

	// Check for Kubernetes
	if _, err := os.Stat("/var/run/secrets/kubernetes.io"); err == nil {
		return true, "kubernetes", nil
	}

	return false, "", nil
}

func detectKernelVersion() string {
	if runtime.GOOS == "linux" {
		output, err := exec.Command("uname", "-r").Output()
		if err == nil {
			return strings.TrimSpace(string(output))
		}
	}
	return ""
}

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func checkFileContains(path, substring string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), substring)
}

func readFirstLine(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		return scanner.Text()
	}
	return ""
}

// Global default detector instance
var defaultDetector Detector = NewDetector()

// Detect performs platform detection using the default detector
func Detect() (*Info, error) {
	return defaultDetector.Detect()
}

// DetectOS detects the operating system type using the default detector
func DetectOS() (OSType, error) {
	return defaultDetector.DetectOS()
}

// DetectDistro detects the Linux distribution using the default detector
func DetectDistro() (DistroType, string, error) {
	return defaultDetector.DetectDistro()
}

// DetectArch detects the CPU architecture using the default detector
func DetectArch() (ArchType, error) {
	return defaultDetector.DetectArch()
}

// DetectPackageManager detects the package manager using the default detector
func DetectPackageManager() (PackageManager, error) {
	return defaultDetector.DetectPackageManager()
}

// DetectInitSystem detects the init system using the default detector
func DetectInitSystem() (InitSystem, error) {
	return defaultDetector.DetectInitSystem()
}
