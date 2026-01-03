package platform

import (
	"runtime"
	"testing"
	"time"
)

func TestDetectOS(t *testing.T) {
	detector := NewDetector()
	osType, err := detector.DetectOS()
	if err != nil {
		t.Fatalf("DetectOS failed: %v", err)
	}

	expected := NormalizeOSType(runtime.GOOS)
	if osType != expected {
		t.Errorf("expected OS %s, got %s", expected, osType)
	}
}

func TestDetectArch(t *testing.T) {
	detector := NewDetector()
	arch, err := detector.DetectArch()
	if err != nil {
		t.Fatalf("DetectArch failed: %v", err)
	}

	expected := NormalizeArchType(runtime.GOARCH)
	if arch != expected {
		t.Errorf("expected arch %s, got %s", expected, arch)
	}
}

func TestDetect(t *testing.T) {
	detector := NewDetector()
	info, err := detector.Detect()
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	if info == nil {
		t.Fatal("expected non-nil info")
	}

	// Check that OS is detected
	if info.OS == OSUnknown {
		t.Error("expected OS to be detected")
	}

	// Check that Arch is detected
	if info.Arch == ArchUnknown {
		t.Error("expected Arch to be detected")
	}

	// Check that DetectedAt is set
	if info.DetectedAt.IsZero() {
		t.Error("expected DetectedAt to be set")
	}

	// Check that Metadata is initialized
	if info.Metadata == nil {
		t.Error("expected Metadata to be initialized")
	}
}

func TestDetectCaching(t *testing.T) {
	detector := NewDetector().(*DefaultDetector)

	// First detection
	info1, err := detector.Detect()
	if err != nil {
		t.Fatalf("first Detect failed: %v", err)
	}

	// Second detection (should be cached)
	info2, err := detector.Detect()
	if err != nil {
		t.Fatalf("second Detect failed: %v", err)
	}

	// Should return same instance due to caching
	if info1.DetectedAt != info2.DetectedAt {
		t.Error("expected cached result")
	}

	// Wait for cache to expire
	detector.cacheAge = 1 * time.Millisecond
	time.Sleep(2 * time.Millisecond)

	// Third detection (cache expired)
	info3, err := detector.Detect()
	if err != nil {
		t.Fatalf("third Detect failed: %v", err)
	}

	// Should be a new detection
	if info1.DetectedAt == info3.DetectedAt {
		t.Error("expected cache to expire")
	}
}

func TestOSType(t *testing.T) {
	tests := []struct {
		osType OSType
		str    string
	}{
		{OSLinux, "linux"},
		{OSWindows, "windows"},
		{OSMacOS, "darwin"},
		{OSBSD, "bsd"},
		{OSUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.str, func(t *testing.T) {
			if tt.osType.String() != tt.str {
				t.Errorf("expected %s, got %s", tt.str, tt.osType.String())
			}
		})
	}
}

func TestArchType(t *testing.T) {
	tests := []struct {
		arch ArchType
		str  string
	}{
		{ArchAMD64, "amd64"},
		{ArchARM64, "arm64"},
		{ArchARM, "arm"},
		{Arch386, "386"},
		{ArchUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.str, func(t *testing.T) {
			if tt.arch.String() != tt.str {
				t.Errorf("expected %s, got %s", tt.str, tt.arch.String())
			}
		})
	}
}

func TestNormalizeOSType(t *testing.T) {
	tests := []struct {
		goos     string
		expected OSType
	}{
		{"linux", OSLinux},
		{"windows", OSWindows},
		{"darwin", OSMacOS},
		{"freebsd", OSBSD},
		{"openbsd", OSBSD},
		{"netbsd", OSBSD},
		{"unknown", OSUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			result := NormalizeOSType(tt.goos)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestNormalizeArchType(t *testing.T) {
	tests := []struct {
		goarch   string
		expected ArchType
	}{
		{"amd64", ArchAMD64},
		{"arm64", ArchARM64},
		{"arm", ArchARM},
		{"386", Arch386},
		{"unknown", ArchUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.goarch, func(t *testing.T) {
			result := NormalizeArchType(tt.goarch)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestInfoHelpers(t *testing.T) {
	info := &Info{
		OS:             OSLinux,
		Distro:         DistroUbuntu,
		PackageManager: PackageManagerAPT,
		InitSystem:     InitSystemd,
	}

	if !info.IsLinux() {
		t.Error("expected IsLinux to be true")
	}
	if info.IsWindows() {
		t.Error("expected IsWindows to be false")
	}
	if !info.IsDebianBased() {
		t.Error("expected IsDebianBased to be true")
	}
	if info.IsRHELBased() {
		t.Error("expected IsRHELBased to be false")
	}
	if !info.UsesAPT() {
		t.Error("expected UsesAPT to be true")
	}
	if !info.UsesSystemd() {
		t.Error("expected UsesSystemd to be true")
	}
}

func TestGetPlatformFamily(t *testing.T) {
	tests := []struct {
		distro   DistroType
		expected string
	}{
		{DistroUbuntu, "debian"},
		{DistroDebian, "debian"},
		{DistroCentOS, "rhel"},
		{DistroRHEL, "rhel"},
		{DistroFedora, "rhel"},
		{DistroAmazonLinux, "rhel"},
		{DistroOpenSUSE, "suse"},
		{DistroArch, "arch"},
		{DistroAlpine, "alpine"},
		{DistroUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.distro), func(t *testing.T) {
			result := getPlatformFamily(tt.distro)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestNormalizeDistroID(t *testing.T) {
	tests := []struct {
		id       string
		expected DistroType
	}{
		{"ubuntu", DistroUbuntu},
		{"Ubuntu", DistroUbuntu},
		{"debian", DistroDebian},
		{"centos", DistroCentOS},
		{"rhel", DistroRHEL},
		{"redhat", DistroRHEL},
		{"fedora", DistroFedora},
		{"alpine", DistroAlpine},
		{"arch", DistroArch},
		{"opensuse", DistroOpenSUSE},
		{"suse", DistroOpenSUSE},
		{"amzn", DistroAmazonLinux},
		{"amazon", DistroAmazonLinux},
		{"unknown-distro", DistroUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			result := normalizeDistroID(tt.id)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestDetectPackageManager(t *testing.T) {
	detector := NewDetector()
	pkgMgr, err := detector.DetectPackageManager()
	if err != nil {
		t.Fatalf("DetectPackageManager failed: %v", err)
	}

	// Should detect something on most systems
	t.Logf("Detected package manager: %s", pkgMgr)
}

func TestDetectInitSystem(t *testing.T) {
	detector := NewDetector()
	initSys, err := detector.DetectInitSystem()
	if err != nil {
		t.Fatalf("DetectInitSystem failed: %v", err)
	}

	// Should detect something on most systems
	t.Logf("Detected init system: %s", initSys)
}

func TestGlobalDetectors(t *testing.T) {
	// Test global Detect function
	info, err := Detect()
	if err != nil {
		t.Fatalf("global Detect failed: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil info from global Detect")
	}

	// Test global DetectOS function
	osType, err := DetectOS()
	if err != nil {
		t.Fatalf("global DetectOS failed: %v", err)
	}
	if osType == OSUnknown {
		t.Error("expected OS to be detected by global DetectOS")
	}

	// Test global DetectArch function
	arch, err := DetectArch()
	if err != nil {
		t.Fatalf("global DetectArch failed: %v", err)
	}
	if arch == ArchUnknown {
		t.Error("expected arch to be detected by global DetectArch")
	}
}

func TestGetRuntimeOS(t *testing.T) {
	runtimeOS := GetRuntimeOS()
	if runtimeOS == "" {
		t.Error("expected non-empty runtime OS")
	}
	if runtimeOS != runtime.GOOS {
		t.Errorf("expected %s, got %s", runtime.GOOS, runtimeOS)
	}
}

func TestGetRuntimeArch(t *testing.T) {
	runtimeArch := GetRuntimeArch()
	if runtimeArch == "" {
		t.Error("expected non-empty runtime arch")
	}
	if runtimeArch != runtime.GOARCH {
		t.Errorf("expected %s, got %s", runtime.GOARCH, runtimeArch)
	}
}

func TestPackageManagerString(t *testing.T) {
	tests := []PackageManager{
		PackageManagerAPT,
		PackageManagerYum,
		PackageManagerDNF,
		PackageManagerZypper,
		PackageManagerPacman,
		PackageManagerAPK,
		PackageManagerBrew,
		PackageManagerChocolatey,
		PackageManagerWinget,
		PackageManagerUnknown,
	}

	for _, pm := range tests {
		if pm.String() == "" {
			t.Errorf("expected non-empty string for %v", pm)
		}
	}
}

func TestInitSystemString(t *testing.T) {
	tests := []InitSystem{
		InitSystemd,
		InitUpstart,
		InitSysV,
		InitOpenRC,
		InitLaunchd,
		InitWindowsService,
		InitUnknown,
	}

	for _, is := range tests {
		if is.String() == "" {
			t.Errorf("expected non-empty string for %v", is)
		}
	}
}

func TestDistroTypeString(t *testing.T) {
	tests := []struct {
		distro   DistroType
		expected string
	}{
		{DistroUbuntu, "ubuntu"},
		{DistroDebian, "debian"},
		{DistroCentOS, "centos"},
		{DistroRHEL, "rhel"},
		{DistroFedora, "fedora"},
		{DistroAlpine, "alpine"},
		{DistroArch, "arch"},
		{DistroOpenSUSE, "opensuse"},
		{DistroAmazonLinux, "amzn"},
		{DistroUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.distro.String() != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, tt.distro.String())
			}
		})
	}
}

func TestInfoIsMacOS(t *testing.T) {
	tests := []struct {
		name   string
		os     OSType
		expect bool
	}{
		{"macos", OSMacOS, true},
		{"linux", OSLinux, false},
		{"windows", OSWindows, false},
		{"bsd", OSBSD, false},
		{"unknown", OSUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &Info{OS: tt.os}
			if info.IsMacOS() != tt.expect {
				t.Errorf("IsMacOS() = %v, want %v", info.IsMacOS(), tt.expect)
			}
		})
	}
}

func TestInfoIsBSD(t *testing.T) {
	tests := []struct {
		name   string
		os     OSType
		expect bool
	}{
		{"bsd", OSBSD, true},
		{"linux", OSLinux, false},
		{"windows", OSWindows, false},
		{"macos", OSMacOS, false},
		{"unknown", OSUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &Info{OS: tt.os}
			if info.IsBSD() != tt.expect {
				t.Errorf("IsBSD() = %v, want %v", info.IsBSD(), tt.expect)
			}
		})
	}
}

func TestInfoUsesYum(t *testing.T) {
	tests := []struct {
		name   string
		pm     PackageManager
		expect bool
	}{
		{"yum", PackageManagerYum, true},
		{"apt", PackageManagerAPT, false},
		{"dnf", PackageManagerDNF, false},
		{"unknown", PackageManagerUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &Info{PackageManager: tt.pm}
			if info.UsesYum() != tt.expect {
				t.Errorf("UsesYum() = %v, want %v", info.UsesYum(), tt.expect)
			}
		})
	}
}

func TestInfoUsesDNF(t *testing.T) {
	tests := []struct {
		name   string
		pm     PackageManager
		expect bool
	}{
		{"dnf", PackageManagerDNF, true},
		{"apt", PackageManagerAPT, false},
		{"yum", PackageManagerYum, false},
		{"unknown", PackageManagerUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &Info{PackageManager: tt.pm}
			if info.UsesDNF() != tt.expect {
				t.Errorf("UsesDNF() = %v, want %v", info.UsesDNF(), tt.expect)
			}
		})
	}
}

func TestInfoRHELBased_AllDistros(t *testing.T) {
	tests := []struct {
		distro DistroType
		expect bool
	}{
		{DistroCentOS, true},
		{DistroRHEL, true},
		{DistroFedora, true},
		{DistroAmazonLinux, true},
		{DistroUbuntu, false},
		{DistroDebian, false},
		{DistroAlpine, false},
		{DistroArch, false},
		{DistroOpenSUSE, false},
		{DistroUnknown, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.distro), func(t *testing.T) {
			info := &Info{Distro: tt.distro}
			if info.IsRHELBased() != tt.expect {
				t.Errorf("IsRHELBased() for %s = %v, want %v", tt.distro, info.IsRHELBased(), tt.expect)
			}
		})
	}
}

func TestInfoDebianBased_AllDistros(t *testing.T) {
	tests := []struct {
		distro DistroType
		expect bool
	}{
		{DistroUbuntu, true},
		{DistroDebian, true},
		{DistroCentOS, false},
		{DistroRHEL, false},
		{DistroFedora, false},
		{DistroAlpine, false},
		{DistroArch, false},
		{DistroOpenSUSE, false},
		{DistroUnknown, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.distro), func(t *testing.T) {
			info := &Info{Distro: tt.distro}
			if info.IsDebianBased() != tt.expect {
				t.Errorf("IsDebianBased() for %s = %v, want %v", tt.distro, info.IsDebianBased(), tt.expect)
			}
		})
	}
}

func TestInfoMetadata(t *testing.T) {
	info := &Info{
		OS:       OSLinux,
		Distro:   DistroUbuntu,
		Version:  "22.04",
		Arch:     ArchAMD64,
		Metadata: map[string]interface{}{"kernel": "5.15.0"},
	}

	if info.Metadata["kernel"] != "5.15.0" {
		t.Errorf("Metadata[kernel] = %v, want 5.15.0", info.Metadata["kernel"])
	}

	if info.Version != "22.04" {
		t.Errorf("Version = %s, want 22.04", info.Version)
	}
}

func TestInfoVirtualization(t *testing.T) {
	info := &Info{
		IsVirtual:          true,
		VirtualizationType: "kvm",
		IsContainer:        false,
		ContainerType:      "",
	}

	if !info.IsVirtual {
		t.Error("IsVirtual = false, want true")
	}
	if info.VirtualizationType != "kvm" {
		t.Errorf("VirtualizationType = %s, want kvm", info.VirtualizationType)
	}
	if info.IsContainer {
		t.Error("IsContainer = true, want false")
	}
}

func TestInfoContainer(t *testing.T) {
	info := &Info{
		IsVirtual:          false,
		VirtualizationType: "",
		IsContainer:        true,
		ContainerType:      "docker",
	}

	if info.IsVirtual {
		t.Error("IsVirtual = true, want false")
	}
	if !info.IsContainer {
		t.Error("IsContainer = false, want true")
	}
	if info.ContainerType != "docker" {
		t.Errorf("ContainerType = %s, want docker", info.ContainerType)
	}
}
