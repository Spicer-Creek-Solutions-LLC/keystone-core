package container

import (
	"fmt"
	"sync"
	"time"
)

// MultiRuntimeDetector detects container runtime across Docker, containerd, etc.
type MultiRuntimeDetector struct {
	config    *Config
	detectors map[Runtime]Detector
	cache     *Metadata
	cacheTime time.Time
	mu        sync.RWMutex
}

// NewDetector creates a new multi-runtime detector
func NewDetector(config *Config) *MultiRuntimeDetector {
	if config == nil {
		config = DefaultConfig()
	}

	d := &MultiRuntimeDetector{
		config:    config,
		detectors: make(map[Runtime]Detector),
	}

	// Register enabled detectors
	if config.EnableDocker {
		d.detectors[RuntimeDocker] = NewDockerDetector(config)
	}

	if config.EnableContainerd {
		d.detectors[RuntimeContainerd] = NewContainerdDetector(config)
	}

	return d
}

// Detect attempts to detect container runtime and collect metadata
func (d *MultiRuntimeDetector) Detect() (*Metadata, error) {
	// Check cache
	if d.isCacheValid() {
		d.mu.RLock()
		defer d.mu.RUnlock()
		return d.cache, nil
	}

	// Try each detector
	for _, detector := range d.detectors {
		if detector.IsContainer() {
			metadata, err := detector.Detect()
			if err == nil {
				// Cache the result
				d.mu.Lock()
				d.cache = metadata
				d.cacheTime = time.Now()
				d.mu.Unlock()

				return metadata, nil
			}
		}
	}

	return nil, fmt.Errorf("no container runtime detected")
}

// IsContainer checks if running in any container
func (d *MultiRuntimeDetector) IsContainer() bool {
	for _, detector := range d.detectors {
		if detector.IsContainer() {
			return true
		}
	}
	return false
}

// GetRuntime returns the detected container runtime
func (d *MultiRuntimeDetector) GetRuntime() Runtime {
	for runtime, detector := range d.detectors {
		if detector.IsContainer() {
			return runtime
		}
	}
	return RuntimeUnknown
}

// isCacheValid checks if the cache is still valid
func (d *MultiRuntimeDetector) isCacheValid() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.cache == nil {
		return false
	}

	return time.Since(d.cacheTime) < d.config.CacheDuration
}

// ClearCache clears the metadata cache
func (d *MultiRuntimeDetector) ClearCache() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.cache = nil
	d.cacheTime = time.Time{}
}

// Global default detector
var (
	defaultDetector *MultiRuntimeDetector
	detectorOnce    sync.Once
)

// GetDefaultDetector returns the default container detector
func GetDefaultDetector() *MultiRuntimeDetector {
	detectorOnce.Do(func() {
		defaultDetector = NewDetector(DefaultConfig())
	})
	return defaultDetector
}

// Detect is a convenience function using the default detector
func Detect() (*Metadata, error) {
	return GetDefaultDetector().Detect()
}

// IsContainer is a convenience function using the default detector
func IsContainer() bool {
	return GetDefaultDetector().IsContainer()
}

// GetRuntime is a convenience function using the default detector
func GetRuntime() Runtime {
	return GetDefaultDetector().GetRuntime()
}
