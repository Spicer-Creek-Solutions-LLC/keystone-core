package bootstrap

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/shawnbutts/keystone-core/pkg/netutil"
	"github.com/shawnbutts/keystone-core/pkg/platform"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

// ResourceInfo captures basic system resource capacity information.
type ResourceInfo struct {
	CPUCount      int
	MemoryTotalMB uint64
	DiskTotalGB   uint64
	DiskFreeGB    uint64
}

// SystemInfo captures detected system and network information for bootstrap.
type SystemInfo struct {
	Platform        *platform.Info
	Network         *netutil.NetworkInfo
	Resources       ResourceInfo
	ExistingInstall bool
}

// DetectSystem performs platform, network, and resource detection.
func DetectSystem() (*SystemInfo, error) {
	return DetectSystemWithPaths(defaultInstallPaths)
}

// DetectSystemWithPaths performs detection with a custom set of install paths.
func DetectSystemWithPaths(paths []string) (*SystemInfo, error) {
	platformInfo, err := platform.Detect()
	if err != nil {
		return nil, fmt.Errorf("detect platform: %w", err)
	}

	networkInfo, err := netutil.GetNetworkInfo()
	if err != nil {
		return nil, fmt.Errorf("detect network: %w", err)
	}

	resources, err := detectResources()
	if err != nil {
		return nil, fmt.Errorf("detect resources: %w", err)
	}

	return &SystemInfo{
		Platform:        platformInfo,
		Network:         networkInfo,
		Resources:       resources,
		ExistingInstall: DetectExistingInstall(paths),
	}, nil
}

// DetectExistingInstall checks for known Keystone Core artifacts.
func DetectExistingInstall(paths []string) bool {
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

func detectPhase(ctx context.Context, state *State) error {
	info, err := DetectSystem()
	if err != nil {
		return err
	}
	state.System = info

	if state.DryRun || state.Verbose {
		fmt.Fprintf(state.Output, "platform: %s/%s\n", info.Platform.OS, info.Platform.Arch)
		if info.Platform.Distro != platform.DistroUnknown {
			fmt.Fprintf(state.Output, "distro: %s %s\n", info.Platform.Distro, info.Platform.Version)
		}
		fmt.Fprintf(state.Output, "package manager: %s\n", info.Platform.PackageManager)
		fmt.Fprintf(state.Output, "init system: %s\n", info.Platform.InitSystem)
		fmt.Fprintf(state.Output, "cpu: %d, memory: %d MB, disk free: %d GB\n",
			info.Resources.CPUCount, info.Resources.MemoryTotalMB, info.Resources.DiskFreeGB)
		if info.Network != nil {
			fmt.Fprintf(state.Output, "primary ipv4: %s, primary ipv6: %s\n",
				info.Network.PrimaryIPv4, info.Network.PrimaryIPv6)
		}
		if info.ExistingInstall {
			fmt.Fprintln(state.Output, "existing install: detected")
		}
	}

	return nil
}

func detectResources() (ResourceInfo, error) {
	info := ResourceInfo{
		CPUCount: runtime.NumCPU(),
	}

	memInfo, err := mem.VirtualMemory()
	if err != nil {
		return info, err
	}
	info.MemoryTotalMB = memInfo.Total / (1024 * 1024)

	diskInfo, err := disk.Usage("/")
	if err != nil {
		return info, err
	}
	info.DiskTotalGB = diskInfo.Total / (1024 * 1024 * 1024)
	info.DiskFreeGB = diskInfo.Free / (1024 * 1024 * 1024)

	return info, nil
}
