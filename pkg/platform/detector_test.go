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
