package platform

import (
	"runtime"
	"time"
)

// OSType represents the operating system type
type OSType string

const (
	// OSLinux represents Linux operating systems
	OSLinux OSType = "linux"
	// OSWindows represents Windows operating systems
	OSWindows OSType = "windows"
	// OSMacOS represents macOS operating systems
	OSMacOS OSType = "darwin"
	// OSBSD represents BSD operating systems
	OSBSD OSType = "bsd"
	// OSUnknown represents unknown operating systems
	OSUnknown OSType = "unknown"
)

// DistroType represents Linux distribution types
type DistroType string

const (
	// DistroUbuntu represents Ubuntu
	DistroUbuntu DistroType = "ubuntu"
	// DistroDebian represents Debian
	DistroDebian DistroType = "debian"
	// DistroCentOS represents CentOS
	DistroCentOS DistroType = "centos"
	// DistroRHEL represents Red Hat Enterprise Linux
	DistroRHEL DistroType = "rhel"
	// DistroFedora represents Fedora
	DistroFedora DistroType = "fedora"
	// DistroAlpine represents Alpine Linux
	DistroAlpine DistroType = "alpine"
	// DistroArch represents Arch Linux
	DistroArch DistroType = "arch"
	// DistroOpenSUSE represents openSUSE
	DistroOpenSUSE DistroType = "opensuse"
	// DistroAmazonLinux represents Amazon Linux
	DistroAmazonLinux DistroType = "amzn"
	// DistroUnknown represents unknown distributions
	DistroUnknown DistroType = "unknown"
)

// ArchType represents CPU architecture
type ArchType string

const (
	// ArchAMD64 represents x86_64/amd64 architecture
	ArchAMD64 ArchType = "amd64"
	// ArchARM64 represents ARM64/aarch64 architecture
	ArchARM64 ArchType = "arm64"
	// ArchARM represents 32-bit ARM architecture
	ArchARM ArchType = "arm"
	// Arch386 represents 32-bit x86 architecture
	Arch386 ArchType = "386"
	// ArchUnknown represents unknown architecture
	ArchUnknown ArchType = "unknown"
)

// PackageManager represents the system package manager
type PackageManager string

const (
	// PackageManagerAPT represents apt (Debian/Ubuntu)
	PackageManagerAPT PackageManager = "apt"
	// PackageManagerYum represents yum (CentOS/RHEL 7)
	PackageManagerYum PackageManager = "yum"
	// PackageManagerDNF represents dnf (CentOS/RHEL 8+, Fedora)
	PackageManagerDNF PackageManager = "dnf"
	// PackageManagerZypper represents zypper (openSUSE)
	PackageManagerZypper PackageManager = "zypper"
	// PackageManagerPacman represents pacman (Arch)
	PackageManagerPacman PackageManager = "pacman"
	// PackageManagerAPK represents apk (Alpine)
	PackageManagerAPK PackageManager = "apk"
	// PackageManagerBrew represents Homebrew (macOS)
	PackageManagerBrew PackageManager = "brew"
	// PackageManagerChocolatey represents Chocolatey (Windows)
	PackageManagerChocolatey PackageManager = "chocolatey"
	// PackageManagerWinget represents winget (Windows)
	PackageManagerWinget PackageManager = "winget"
	// PackageManagerUnknown represents unknown package manager
	PackageManagerUnknown PackageManager = "unknown"
)

// InitSystem represents the system init system
type InitSystem string

const (
	// InitSystemd represents systemd
	InitSystemd InitSystem = "systemd"
	// InitUpstart represents upstart
	InitUpstart InitSystem = "upstart"
	// InitSysV represents SysV init
	InitSysV InitSystem = "sysv"
	// InitOpenRC represents OpenRC
	InitOpenRC InitSystem = "openrc"
	// InitLaunchd represents launchd (macOS)
	InitLaunchd InitSystem = "launchd"
	// InitWindowsService represents Windows Service Manager
	InitWindowsService InitSystem = "windows_service"
	// InitUnknown represents unknown init system
	InitUnknown InitSystem = "unknown"
)

// Info contains comprehensive platform information
type Info struct {
	// OS is the operating system type
	OS OSType

	// Distro is the Linux distribution (Linux only)
	Distro DistroType

	// Version is the OS/distro version
	Version string

	// Arch is the CPU architecture
	Arch ArchType

	// PackageManager is the detected package manager
	PackageManager PackageManager

	// InitSystem is the detected init system
	InitSystem InitSystem

	// Hostname is the system hostname
	Hostname string

	// KernelVersion is the kernel version
	KernelVersion string

	// PlatformFamily groups related platforms (e.g., "debian" for Ubuntu/Debian)
	PlatformFamily string

	// IsVirtual indicates if running in a virtual machine
	IsVirtual bool

	// VirtualizationType indicates the virtualization type (e.g., "kvm", "vmware")
	VirtualizationType string

	// IsContainer indicates if running in a container
	IsContainer bool

	// ContainerType indicates the container type (e.g., "docker", "lxc")
	ContainerType string

	// DetectedAt is when this information was collected
	DetectedAt time.Time

	// Metadata contains additional platform-specific metadata
	Metadata map[string]interface{}
}

// Detector is the interface for platform detection
type Detector interface {
	// Detect performs platform detection and returns comprehensive info
	Detect() (*Info, error)

	// DetectOS detects the operating system type
	DetectOS() (OSType, error)

	// DetectDistro detects the Linux distribution
	DetectDistro() (DistroType, string, error)

	// DetectArch detects the CPU architecture
	DetectArch() (ArchType, error)

	// DetectPackageManager detects the package manager
	DetectPackageManager() (PackageManager, error)

	// DetectInitSystem detects the init system
	DetectInitSystem() (InitSystem, error)

	// IsVirtualMachine checks if running in a VM
	IsVirtualMachine() (bool, string, error)

	// IsContainer checks if running in a container
	IsContainer() (bool, string, error)
}

// String returns the string representation of OSType
func (o OSType) String() string {
	return string(o)
}

// String returns the string representation of DistroType
func (d DistroType) String() string {
	return string(d)
}

// String returns the string representation of ArchType
func (a ArchType) String() string {
	return string(a)
}

// String returns the string representation of PackageManager
func (p PackageManager) String() string {
	return string(p)
}

// String returns the string representation of InitSystem
func (i InitSystem) String() string {
	return string(i)
}

// IsLinux returns true if the OS is Linux
func (i *Info) IsLinux() bool {
	return i.OS == OSLinux
}

// IsWindows returns true if the OS is Windows
func (i *Info) IsWindows() bool {
	return i.OS == OSWindows
}

// IsMacOS returns true if the OS is macOS
func (i *Info) IsMacOS() bool {
	return i.OS == OSMacOS
}

// IsBSD returns true if the OS is BSD
func (i *Info) IsBSD() bool {
	return i.OS == OSBSD
}

// IsDebianBased returns true if the distro is Debian-based
func (i *Info) IsDebianBased() bool {
	return i.Distro == DistroUbuntu || i.Distro == DistroDebian
}

// IsRHELBased returns true if the distro is RHEL-based
func (i *Info) IsRHELBased() bool {
	return i.Distro == DistroCentOS || i.Distro == DistroRHEL || i.Distro == DistroFedora || i.Distro == DistroAmazonLinux
}

// UsesAPT returns true if the system uses apt
func (i *Info) UsesAPT() bool {
	return i.PackageManager == PackageManagerAPT
}

// UsesYum returns true if the system uses yum
func (i *Info) UsesYum() bool {
	return i.PackageManager == PackageManagerYum
}

// UsesDNF returns true if the system uses dnf
func (i *Info) UsesDNF() bool {
	return i.PackageManager == PackageManagerDNF
}

// UsesSystemd returns true if the system uses systemd
func (i *Info) UsesSystemd() bool {
	return i.InitSystem == InitSystemd
}

// GetRuntimeOS returns the runtime GOOS value
func GetRuntimeOS() string {
	return runtime.GOOS
}

// GetRuntimeArch returns the runtime GOARCH value
func GetRuntimeArch() string {
	return runtime.GOARCH
}

// NormalizeOSType converts runtime.GOOS to OSType
func NormalizeOSType(goos string) OSType {
	switch goos {
	case "linux":
		return OSLinux
	case "windows":
		return OSWindows
	case "darwin":
		return OSMacOS
	case "freebsd", "openbsd", "netbsd":
		return OSBSD
	default:
		return OSUnknown
	}
}

// NormalizeArchType converts runtime.GOARCH to ArchType
func NormalizeArchType(goarch string) ArchType {
	switch goarch {
	case "amd64":
		return ArchAMD64
	case "arm64":
		return ArchARM64
	case "arm":
		return ArchARM
	case "386":
		return Arch386
	default:
		return ArchUnknown
	}
}
