package bootstrap

import (
	"fmt"
	"os"
	"strings"
)

// DeploymentMode defines the bootstrap deployment mode.
type DeploymentMode string

const (
	DeploymentModeDemo       DeploymentMode = "demo"
	DeploymentModeProduction DeploymentMode = "production"
	DeploymentModeFullScale  DeploymentMode = "fullscale"
	DeploymentModeCustom     DeploymentMode = "custom"
)

// DeploymentConfig defines default configuration for a deployment mode.
type DeploymentConfig struct {
	Mode           DeploymentMode
	Description    string
	StorageBackend string
	NATSMode       string
	EtcdMode       string
	HA             bool
}

type modeRequirement struct {
	CPUs     int
	MemoryMB uint64
	DiskGB   uint64
}

var defaultInstallPaths = []string{
	"/etc/kscore/server.yaml",
	"/etc/kscore/agent.yaml",
	"/var/lib/kscore",
}

var defaultDeploymentConfigs = map[DeploymentMode]DeploymentConfig{
	DeploymentModeDemo: {
		Mode:           DeploymentModeDemo,
		Description:    "Single-node demo with embedded dependencies",
		StorageBackend: "sqlite",
		NATSMode:       "embedded",
		EtcdMode:       "embedded",
		HA:             false,
	},
	DeploymentModeProduction: {
		Mode:           DeploymentModeProduction,
		Description:    "Production-ready cluster defaults",
		StorageBackend: "postgres",
		NATSMode:       "cluster",
		EtcdMode:       "external",
		HA:             true,
	},
	DeploymentModeFullScale: {
		Mode:           DeploymentModeFullScale,
		Description:    "Full-scale multi-node deployment",
		StorageBackend: "postgres",
		NATSMode:       "cluster",
		EtcdMode:       "external",
		HA:             true,
	},
	DeploymentModeCustom: {
		Mode:        DeploymentModeCustom,
		Description: "Custom deployment configuration",
	},
}

var modeRequirements = map[DeploymentMode]modeRequirement{
	DeploymentModeDemo: {
		CPUs:     1,
		MemoryMB: 2048,
		DiskGB:   10,
	},
	DeploymentModeProduction: {
		CPUs:     4,
		MemoryMB: 8192,
		DiskGB:   50,
	},
	DeploymentModeFullScale: {
		CPUs:     8,
		MemoryMB: 16384,
		DiskGB:   100,
	},
}

// ParseDeploymentMode converts a string into a deployment mode.
func ParseDeploymentMode(raw string) (DeploymentMode, error) {
	if raw == "" {
		return "", nil
	}

	switch DeploymentMode(strings.ToLower(raw)) {
	case DeploymentModeDemo:
		return DeploymentModeDemo, nil
	case DeploymentModeProduction:
		return DeploymentModeProduction, nil
	case DeploymentModeFullScale:
		return DeploymentModeFullScale, nil
	case DeploymentModeCustom:
		return DeploymentModeCustom, nil
	default:
		return "", fmt.Errorf("unsupported deployment mode: %s", raw)
	}
}

// DefaultDeploymentConfig returns the default config for a mode.
func DefaultDeploymentConfig(mode DeploymentMode) (DeploymentConfig, bool) {
	cfg, ok := defaultDeploymentConfigs[mode]
	return cfg, ok
}

// DetectMode returns a deployment mode based on simple heuristics.
func DetectMode() DeploymentMode {
	return DetectModeFromPaths(defaultInstallPaths)
}

// DetectModeFromPaths checks installation paths to infer the deployment mode.
func DetectModeFromPaths(paths []string) DeploymentMode {
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return DeploymentModeProduction
		}
	}
	return DeploymentModeDemo
}

// RecommendMode suggests a deployment mode based on detected system resources.
func RecommendMode(system *SystemInfo) DeploymentMode {
	if system == nil {
		return DeploymentModeDemo
	}
	if system.ExistingInstall {
		return DeploymentModeProduction
	}
	if meetsRequirement(system.Resources, modeRequirements[DeploymentModeFullScale]) {
		return DeploymentModeFullScale
	}
	if meetsRequirement(system.Resources, modeRequirements[DeploymentModeProduction]) {
		return DeploymentModeProduction
	}
	return DeploymentModeDemo
}

// ModeHints returns human-friendly requirement hints for each mode.
func ModeHints(system *SystemInfo) map[DeploymentMode]string {
	hints := make(map[DeploymentMode]string, len(modeRequirements))
	var detected string
	if system != nil {
		detected = formatDetected(system.Resources)
	}
	for mode, req := range modeRequirements {
		hints[mode] = formatRequirement(req, detected)
	}
	return hints
}

// ResourceSummary returns a short summary of detected resources.
func ResourceSummary(system *SystemInfo) string {
	if system == nil {
		return ""
	}
	return formatDetected(system.Resources)
}

func meetsRequirement(resources ResourceInfo, req modeRequirement) bool {
	if resources.CPUCount < req.CPUs {
		return false
	}
	if resources.MemoryTotalMB < req.MemoryMB {
		return false
	}
	if resources.DiskFreeGB < req.DiskGB {
		return false
	}
	return true
}

func formatRequirement(req modeRequirement, detected string) string {
	requirement := fmt.Sprintf("Req: %d CPU / %dGB RAM / %dGB disk", req.CPUs, req.MemoryMB/1024, req.DiskGB)
	if detected == "" {
		return requirement
	}
	return fmt.Sprintf("%s. %s", requirement, detected)
}

func formatDetected(resources ResourceInfo) string {
	return fmt.Sprintf("Detected: %d CPU / %dGB RAM / %dGB free", resources.CPUCount, resources.MemoryTotalMB/1024, resources.DiskFreeGB)
}
