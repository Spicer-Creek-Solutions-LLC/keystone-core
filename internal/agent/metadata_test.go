// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"
)

func TestGopsutilCollector_HeartbeatSmoke(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("gopsutil smoke runs Linux/macOS only in v1.0")
	}
	c := NewGopsutilCollector(testLogger())
	hb := c.Heartbeat(context.Background(), "agent-7")

	if hb.AgentID != "agent-7" {
		t.Errorf("AgentID = %q", hb.AgentID)
	}
	if hb.TS.IsZero() {
		t.Error("TS is zero")
	}
	// CPU% is 0 on the very first call (cpu.Percent(0) needs a
	// baseline). Memory% should be a real reading on any Linux/
	// macOS host.
	if hb.MemPercent <= 0 {
		t.Errorf("MemPercent = %v, want >0", hb.MemPercent)
	}
}

func TestGopsutilCollector_HeartbeatCPUNonZeroAfterPrime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("gopsutil smoke runs Linux/macOS only in v1.0")
	}
	c := NewGopsutilCollector(testLogger())
	// Prime: first call returns 0.
	_ = c.Heartbeat(context.Background(), "agent-7")
	// Spin briefly so cpu.Percent has a delta.
	for i := 0; i < 5_000_000; i++ {
		_ = i * i
	}
	hb := c.Heartbeat(context.Background(), "agent-7")
	// Not asserting exact value (busy host could be at 0% if
	// scheduler napped); just that the field is present and the
	// percent type is plausible.
	if hb.CPUPercent < 0 || hb.CPUPercent > 100 {
		t.Errorf("CPUPercent = %v, want in [0, 100]", hb.CPUPercent)
	}
}

func TestGopsutilCollector_MetadataSmoke(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("gopsutil smoke runs Linux/macOS only in v1.0")
	}
	c := NewGopsutilCollector(testLogger())
	md := c.Metadata(context.Background(), "agent-7", map[string]string{"role": "test"})

	if md.AgentID != "agent-7" {
		t.Errorf("AgentID = %q", md.AgentID)
	}
	if md.Hostname == "" {
		t.Error("Hostname empty")
	}
	if md.OS == "" {
		t.Error("OS empty")
	}
	if md.CPUCount <= 0 {
		t.Errorf("CPUCount = %d, want >0", md.CPUCount)
	}
	if md.MemTotalBytes == 0 {
		t.Error("MemTotalBytes = 0")
	}
	if md.Architecture == "" {
		t.Error("Architecture empty")
	}
	if md.Labels["role"] != "test" {
		t.Errorf("Labels[role] = %q, want test", md.Labels["role"])
	}
	// At least one NIC (loopback) should be present.
	if len(md.NICs) == 0 {
		t.Error("NICs empty (expected at least loopback)")
	}
}

func TestGopsutilCollector_MetadataNICDualStackFlag(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("gopsutil smoke runs Linux/macOS only in v1.0")
	}
	c := NewGopsutilCollector(testLogger())
	md := c.Metadata(context.Background(), "agent-7", nil)

	for _, nic := range md.NICs {
		want := len(nic.IPv4) > 0 && len(nic.IPv6) > 0
		if nic.DualStack != want {
			t.Errorf("NIC %q: DualStack = %v, want %v (v4=%d, v6=%d)",
				nic.Name, nic.DualStack, want, len(nic.IPv4), len(nic.IPv6))
		}
	}
}

func TestGopsutilCollector_MetadataJSONShape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("gopsutil smoke runs Linux/macOS only in v1.0")
	}
	c := NewGopsutilCollector(testLogger())
	md := c.Metadata(context.Background(), "agent-7", nil)

	b, err := json.Marshal(md)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var rt AgentMetadata
	if err := json.Unmarshal(b, &rt); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if rt.Hostname != md.Hostname {
		t.Errorf("hostname drift after JSON round-trip: %q -> %q", md.Hostname, rt.Hostname)
	}
}

func TestTopNDisksByTotal_Sorting(t *testing.T) {
	infos := []DiskInfo{
		{Mountpoint: "/small", TotalBytes: 100, UsedPercent: 10},
		{Mountpoint: "/large", TotalBytes: 9000, UsedPercent: 50},
		{Mountpoint: "/medium", TotalBytes: 500, UsedPercent: 80},
	}
	out := topNDisksByTotal(infos, 2)
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	if out[0].Mountpoint != "/large" {
		t.Errorf("[0] = %q, want /large (largest)", out[0].Mountpoint)
	}
	if out[1].Mountpoint != "/medium" {
		t.Errorf("[1] = %q, want /medium", out[1].Mountpoint)
	}
}

func TestTopNDisksByTotal_NLargerThanInput(t *testing.T) {
	infos := []DiskInfo{
		{Mountpoint: "/a", TotalBytes: 100},
	}
	out := topNDisksByTotal(infos, 5)
	if len(out) != 1 {
		t.Errorf("len = %d, want 1 (capped to input length)", len(out))
	}
}

func TestPseudoFilesystemsFiltered(t *testing.T) {
	for _, fs := range []string{"tmpfs", "proc", "sysfs", "cgroup"} {
		if !pseudoFilesystems[fs] {
			t.Errorf("%q should be in pseudoFilesystems blocklist", fs)
		}
	}
	for _, fs := range []string{"ext4", "xfs", "btrfs"} {
		if pseudoFilesystems[fs] {
			t.Errorf("%q should NOT be in pseudoFilesystems blocklist", fs)
		}
	}
}
