package hardware

import (
	"testing"
	"time"
)

func TestDetectCPU(t *testing.T) {
	detector := NewDetector()
	cpuInfo, err := detector.DetectCPU()
	if err != nil {
		t.Fatalf("DetectCPU failed: %v", err)
	}

	if cpuInfo == nil {
		t.Fatal("expected non-nil CPU info")
	}

	// Check basic CPU info
	if cpuInfo.Cores <= 0 {
		t.Error("expected positive core count")
	}

	if cpuInfo.Threads <= 0 {
		t.Error("expected positive thread count")
	}

	t.Logf("CPU: %s (%s)", cpuInfo.Model, cpuInfo.Vendor)
	t.Logf("Cores: %d, Threads: %d", cpuInfo.Cores, cpuInfo.Threads)
	t.Logf("Sockets: %d", cpuInfo.Sockets)
	t.Logf("MHz: %.2f", cpuInfo.MHz)
}

func TestDetectMemory(t *testing.T) {
	detector := NewDetector()
	memInfo, err := detector.DetectMemory()
	if err != nil {
		t.Fatalf("DetectMemory failed: %v", err)
	}

	if memInfo == nil {
		t.Fatal("expected non-nil memory info")
	}

	// Check basic memory info
	if memInfo.Total == 0 {
		t.Error("expected non-zero total memory")
	}

	if memInfo.UsedPercent < 0 || memInfo.UsedPercent > 100 {
		t.Errorf("invalid memory usage percent: %.2f", memInfo.UsedPercent)
	}

	t.Logf("Memory Total: %d GB", memInfo.Total/(1024*1024*1024))
	t.Logf("Memory Available: %d GB", memInfo.Available/(1024*1024*1024))
	t.Logf("Memory Used: %.2f%%", memInfo.UsedPercent)
}

func TestDetectDisks(t *testing.T) {
	detector := NewDetector()
	disks, err := detector.DetectDisks()
	if err != nil {
		t.Fatalf("DetectDisks failed: %v", err)
	}

	if len(disks) == 0 {
		t.Error("expected at least one disk")
	}

	for i, disk := range disks {
		t.Logf("Disk %d: %s", i, disk.Device)
		t.Logf("  Mountpoint: %s", disk.Mountpoint)
		t.Logf("  Filesystem: %s", disk.Filesystem)
		t.Logf("  Used: %.2f%%", disk.UsedPercent)

		// Basic validation
		if disk.Device == "" {
			t.Error("expected non-empty device name")
		}

		if disk.UsedPercent < 0 || disk.UsedPercent > 100 {
			t.Errorf("invalid disk usage percent: %.2f", disk.UsedPercent)
		}
	}
}

func TestDetectNetwork(t *testing.T) {
	detector := NewDetector()
	networks, err := detector.DetectNetwork()
	if err != nil {
		t.Fatalf("DetectNetwork failed: %v", err)
	}

	// May have no non-loopback interfaces in some environments
	if len(networks) == 0 {
		t.Log("No non-loopback network interfaces found")
		return
	}

	for i, net := range networks {
		t.Logf("Network %d: %s", i, net.Name)
		t.Logf("  MAC: %s", net.HardwareAddr)
		t.Logf("  MTU: %d", net.MTU)
		t.Logf("  Addresses: %v", net.Addresses)

		// Basic validation
		if net.Name == "" {
			t.Error("expected non-empty interface name")
		}

		if net.MTU <= 0 {
			t.Error("expected positive MTU")
		}
	}
}

func TestDetectSystem(t *testing.T) {
	detector := NewDetector()
	sysInfo, err := detector.DetectSystem()
	if err != nil {
		t.Fatalf("DetectSystem failed: %v", err)
	}

	if sysInfo == nil {
		t.Fatal("expected non-nil system info")
	}

	t.Logf("System: %s", sysInfo.Manufacturer)
	t.Logf("Product: %s", sysInfo.ProductName)
	t.Logf("Version: %s", sysInfo.Version)
	t.Logf("UUID: %s", sysInfo.UUID)
}

func TestDetectBMC(t *testing.T) {
	detector := NewDetector()
	bmcInfo, err := detector.DetectBMC()
	if err != nil {
		t.Fatalf("DetectBMC failed: %v", err)
	}

	if bmcInfo == nil {
		t.Fatal("expected non-nil BMC info")
	}

	// BMC is typically not present in most environments
	t.Logf("BMC Present: %v", bmcInfo.Present)
	if bmcInfo.Present {
		t.Logf("BMC IP: %s", bmcInfo.IPAddress)
		t.Logf("BMC MAC: %s", bmcInfo.MACAddress)
		t.Logf("Firmware: %s", bmcInfo.FirmwareVersion)
	}
}

func TestDetect(t *testing.T) {
	detector := NewDetector()
	info, err := detector.Detect()
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}

	if info == nil {
		t.Fatal("expected non-nil hardware info")
	}

	// Check that at least some components were detected
	if info.CPU == nil {
		t.Error("expected CPU info to be detected")
	}

	if info.Memory == nil {
		t.Error("expected memory info to be detected")
	}

	if len(info.Disks) == 0 {
		t.Log("Warning: no disks detected")
	}

	// Check that DetectedAt is set
	if info.DetectedAt.IsZero() {
		t.Error("expected DetectedAt to be set")
	}

	t.Logf("Hardware detection completed at: %s", info.DetectedAt.Format(time.RFC3339))
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

func TestGlobalDetectors(t *testing.T) {
	// Test global Detect function
	info, err := Detect()
	if err != nil {
		t.Fatalf("global Detect failed: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil info from global Detect")
	}

	// Test global DetectCPU function
	cpuInfo, err := DetectCPU()
	if err != nil {
		t.Fatalf("global DetectCPU failed: %v", err)
	}
	if cpuInfo == nil {
		t.Error("expected CPU info from global DetectCPU")
	}

	// Test global DetectMemory function
	memInfo, err := DetectMemory()
	if err != nil {
		t.Fatalf("global DetectMemory failed: %v", err)
	}
	if memInfo == nil {
		t.Error("expected memory info from global DetectMemory")
	}

	// Test global DetectDisks function
	disks, err := DetectDisks()
	if err != nil {
		t.Fatalf("global DetectDisks failed: %v", err)
	}
	if len(disks) == 0 {
		t.Log("Warning: no disks from global DetectDisks")
	}

	// Test global DetectNetwork function
	networks, err := DetectNetwork()
	if err != nil {
		t.Fatalf("global DetectNetwork failed: %v", err)
	}
	_ = networks // May be empty in some environments

	// Test global DetectSystem function
	sysInfo, err := DetectSystem()
	if err != nil {
		t.Fatalf("global DetectSystem failed: %v", err)
	}
	if sysInfo == nil {
		t.Error("expected system info from global DetectSystem")
	}

	// Test global DetectBMC function
	bmcInfo, err := DetectBMC()
	if err != nil {
		t.Fatalf("global DetectBMC failed: %v", err)
	}
	if bmcInfo == nil {
		t.Error("expected BMC info from global DetectBMC")
	}
}
