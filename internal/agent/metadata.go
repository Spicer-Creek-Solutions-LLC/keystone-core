// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"log/slog"
	"net"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
)

// HeartbeatMetrics is the lightweight payload published every
// HeartbeatInterval (default 30s). Cheap to collect — gopsutil's
// CPU%, memory%, and per-mount disk%.
type HeartbeatMetrics struct {
	AgentID    string      `json:"agent_id"`
	TS         time.Time   `json:"ts"`
	CPUPercent float64     `json:"cpu_percent"`
	MemPercent float64     `json:"mem_percent"`
	Disks      []DiskUsage `json:"disks,omitempty"`
}

// DiskUsage carries one mountpoint's percent-used. Heartbeats include
// the top-N (by total size) physical filesystems; full metadata
// includes per-mount detail.
type DiskUsage struct {
	Mountpoint  string  `json:"mountpoint"`
	UsedPercent float64 `json:"used_percent"`
}

// AgentMetadata is the full per-host snapshot published every
// MetadataInterval (default 60s). Expensive — DMI lookups, NIC
// enumeration. PROJECT-DETAILS §4.6 lists the field set.
type AgentMetadata struct {
	AgentID         string            `json:"agent_id"`
	TS              time.Time         `json:"ts"`
	Hostname        string            `json:"hostname"`
	OS              string            `json:"os"`
	Platform        string            `json:"platform"`
	PlatformVersion string            `json:"platform_version"`
	KernelVersion   string            `json:"kernel_version"`
	Architecture    string            `json:"architecture"`
	VirtSystem      string            `json:"virt_system,omitempty"`
	VirtRole        string            `json:"virt_role,omitempty"`
	CPUCount        int               `json:"cpu_count"`
	MemTotalBytes   uint64            `json:"mem_total_bytes"`
	Disks           []DiskInfo        `json:"disks,omitempty"`
	NICs            []NIC             `json:"nics,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
}

// DiskInfo is one mountpoint's full snapshot. AgentMetadata carries
// every physical-filesystem mount.
type DiskInfo struct {
	Mountpoint  string  `json:"mountpoint"`
	Filesystem  string  `json:"filesystem"`
	TotalBytes  uint64  `json:"total_bytes"`
	UsedBytes   uint64  `json:"used_bytes"`
	UsedPercent float64 `json:"used_percent"`
}

// NIC is one network interface. IPv4 / IPv6 addresses are CIDR
// strings; DualStack is true iff both lists are non-empty (the
// §4.6 acceptance: "IPv4 + IPv6 separated, dual-stack flagged").
type NIC struct {
	Name      string   `json:"name"`
	MACAddr   string   `json:"mac_addr,omitempty"`
	IPv4      []string `json:"ipv4,omitempty"`
	IPv6      []string `json:"ipv6,omitempty"`
	DualStack bool     `json:"dual_stack"`
	MTU       int      `json:"mtu"`
	Up        bool     `json:"up"`
}

// MetricsCollector is the narrow surface Agent needs from the host-
// metrics layer. internal/agent/metadata.go's gopsutilCollector is
// the production impl; tests pass a fake. Same pattern as the
// NATSClient / Subjects interfaces — keeps the lifecycle code
// testable without gopsutil dependencies.
type MetricsCollector interface {
	Heartbeat(ctx context.Context, agentID string) HeartbeatMetrics
	Metadata(ctx context.Context, agentID string, labels map[string]string) AgentMetadata
}

// heartbeatDiskTopN caps the number of mounts surfaced in the
// lightweight heartbeat. Operators inspecting `/api/status` see the
// hot mounts; the full per-mount detail is in the metadata payload.
const heartbeatDiskTopN = 5

// NewGopsutilCollector returns the production MetricsCollector.
// Best-effort error handling — gopsutil failures (weird /proc
// layouts, container restrictions) log at Warn and degrade fields
// to zero rather than failing the heartbeat.
//
// CPU% caveat: cpu.Percent(0, false) returns the delta since the
// previous call. The very first heartbeat reports 0%; subsequent
// reports are real. Operators see this only on agent startup; v1.x
// can prime the collector at Agent.Start.
func NewGopsutilCollector(log *slog.Logger) MetricsCollector {
	if log == nil {
		log = slog.Default()
	}
	return &gopsutilCollector{
		log: log,
		now: time.Now,
	}
}

type gopsutilCollector struct {
	log *slog.Logger
	now func() time.Time
}

func (c *gopsutilCollector) Heartbeat(ctx context.Context, agentID string) HeartbeatMetrics {
	out := HeartbeatMetrics{
		AgentID: agentID,
		TS:      c.now().UTC(),
	}

	pcts, err := cpu.PercentWithContext(ctx, 0, false)
	if err != nil {
		c.log.Warn("agent: cpu.Percent", "err", err)
	} else if len(pcts) > 0 {
		out.CPUPercent = pcts[0]
	}

	if vm, err := mem.VirtualMemoryWithContext(ctx); err != nil {
		c.log.Warn("agent: mem.VirtualMemory", "err", err)
	} else if vm != nil {
		out.MemPercent = vm.UsedPercent
	}

	parts, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		c.log.Warn("agent: disk.Partitions", "err", err)
		return out
	}
	infos := collectDiskInfos(ctx, c.log, parts)
	if len(infos) > 0 {
		out.Disks = topNDisksByTotal(infos, heartbeatDiskTopN)
	}
	return out
}

func (c *gopsutilCollector) Metadata(ctx context.Context, agentID string, labels map[string]string) AgentMetadata {
	out := AgentMetadata{
		AgentID:      agentID,
		TS:           c.now().UTC(),
		Architecture: runtime.GOARCH,
		Labels:       labels,
	}

	if hi, err := host.InfoWithContext(ctx); err != nil {
		c.log.Warn("agent: host.Info", "err", err)
	} else if hi != nil {
		out.Hostname = hi.Hostname
		out.OS = hi.OS
		out.Platform = hi.Platform
		out.PlatformVersion = hi.PlatformVersion
		out.KernelVersion = hi.KernelVersion
		if hi.KernelArch != "" {
			out.Architecture = hi.KernelArch
		}
		out.VirtSystem = hi.VirtualizationSystem
		out.VirtRole = hi.VirtualizationRole
	}

	if n, err := cpu.CountsWithContext(ctx, true); err != nil {
		c.log.Warn("agent: cpu.Counts", "err", err)
	} else {
		out.CPUCount = n
	}

	if vm, err := mem.VirtualMemoryWithContext(ctx); err != nil {
		c.log.Warn("agent: mem.VirtualMemory", "err", err)
	} else if vm != nil {
		out.MemTotalBytes = vm.Total
	}

	if parts, err := disk.PartitionsWithContext(ctx, false); err != nil {
		c.log.Warn("agent: disk.Partitions", "err", err)
	} else {
		out.Disks = collectDiskInfos(ctx, c.log, parts)
	}

	out.NICs = collectNICs(c.log)

	return out
}

// pseudoFilesystems is the blocklist of fs types that aren't real
// disks. Mounts on these (proc, sysfs, tmpfs, etc.) are skipped at
// collection time so heartbeats and metadata don't bloat with
// pseudo-fs noise.
var pseudoFilesystems = map[string]bool{
	"autofs": true, "binfmt_misc": true, "bpf": true,
	"cgroup": true, "cgroup2": true, "configfs": true,
	"debugfs": true, "devpts": true, "devtmpfs": true,
	"fuse.lxcfs": true, "fusectl": true, "hugetlbfs": true,
	"mqueue": true, "nsfs": true, "proc": true, "pstore": true,
	"ramfs": true, "rpc_pipefs": true, "securityfs": true,
	"sysfs": true, "tmpfs": true, "tracefs": true,
}

func collectDiskInfos(ctx context.Context, log *slog.Logger, parts []disk.PartitionStat) []DiskInfo {
	out := make([]DiskInfo, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		if pseudoFilesystems[p.Fstype] {
			continue
		}
		if _, dup := seen[p.Mountpoint]; dup {
			continue
		}
		seen[p.Mountpoint] = struct{}{}
		usage, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil {
			log.Warn("agent: disk.Usage", "mountpoint", p.Mountpoint, "err", err)
			continue
		}
		out = append(out, DiskInfo{
			Mountpoint:  p.Mountpoint,
			Filesystem:  p.Fstype,
			TotalBytes:  usage.Total,
			UsedBytes:   usage.Used,
			UsedPercent: usage.UsedPercent,
		})
	}
	return out
}

// topNDisksByTotal returns up to n DiskUsage entries sorted by
// total bytes descending. Used to keep heartbeats lightweight while
// still surfacing the operator-relevant mounts (e.g., a 2 TB data
// drive that's filling up alongside the 50 GB root partition).
func topNDisksByTotal(infos []DiskInfo, n int) []DiskUsage {
	if n > len(infos) {
		n = len(infos)
	}
	cp := append([]DiskInfo(nil), infos...)
	sort.SliceStable(cp, func(i, j int) bool {
		return cp[i].TotalBytes > cp[j].TotalBytes
	})
	out := make([]DiskUsage, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, DiskUsage{
			Mountpoint:  cp[i].Mountpoint,
			UsedPercent: cp[i].UsedPercent,
		})
	}
	return out
}

// collectNICs enumerates network interfaces via stdlib net (avoids
// pulling in gopsutil/net for this — the IPv4/v6 split is the only
// thing we need, and it's trivial with net.IP.To4()).
//
// Loopback is included (consistent enumeration; consumers filter).
func collectNICs(log *slog.Logger) []NIC {
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Warn("agent: net.Interfaces", "err", err)
		return nil
	}
	out := make([]NIC, 0, len(ifaces))
	for _, iface := range ifaces {
		nic := NIC{
			Name:    iface.Name,
			MACAddr: iface.HardwareAddr.String(),
			MTU:     iface.MTU,
			Up:      iface.Flags&net.FlagUp != 0,
		}
		addrs, err := iface.Addrs()
		if err != nil {
			log.Warn("agent: iface.Addrs", "iface", iface.Name, "err", err)
			out = append(out, nic)
			continue
		}
		for _, a := range addrs {
			cidr := a.String()
			ip, _, err := net.ParseCIDR(cidr)
			if err != nil {
				// Some address types (e.g., link-local without prefix)
				// don't parse as CIDR; skip rather than fail the NIC.
				continue
			}
			if ip.To4() != nil {
				nic.IPv4 = append(nic.IPv4, cidr)
			} else if strings.Contains(cidr, ":") {
				nic.IPv6 = append(nic.IPv6, cidr)
			}
		}
		nic.DualStack = len(nic.IPv4) > 0 && len(nic.IPv6) > 0
		out = append(out, nic)
	}
	return out
}
