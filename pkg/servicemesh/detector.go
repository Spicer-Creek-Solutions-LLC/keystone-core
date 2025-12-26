package servicemesh

import (
	"fmt"
	"sync"
	"time"
)

// MultiMeshDetector detects service mesh across Istio, Linkerd, Consul, etc.
type MultiMeshDetector struct {
	config    *Config
	detectors map[MeshType]Detector
	cache     *Metadata
	cacheTime time.Time
	mu        sync.RWMutex
}

// NewDetector creates a new multi-mesh detector
func NewDetector(config *Config) *MultiMeshDetector {
	if config == nil {
		config = DefaultConfig()
	}

	d := &MultiMeshDetector{
		config:    config,
		detectors: make(map[MeshType]Detector),
	}

	// Register enabled detectors
	if config.EnableIstio {
		d.detectors[MeshTypeIstio] = NewIstioDetector(config)
	}

	if config.EnableLinkerd {
		d.detectors[MeshTypeLinkerd] = NewLinkerdDetector(config)
	}

	if config.EnableConsul {
		d.detectors[MeshTypeConsul] = NewConsulDetector(config)
	}

	return d
}

// Detect attempts to detect service mesh and collect metadata
func (d *MultiMeshDetector) Detect() (*Metadata, error) {
	// Check cache
	if d.isCacheValid() {
		d.mu.RLock()
		defer d.mu.RUnlock()
		return d.cache, nil
	}

	// Try each detector
	for _, detector := range d.detectors {
		if detector.IsServiceMesh() {
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

	return nil, fmt.Errorf("no service mesh detected")
}

// IsServiceMesh checks if running in any service mesh
func (d *MultiMeshDetector) IsServiceMesh() bool {
	for _, detector := range d.detectors {
		if detector.IsServiceMesh() {
			return true
		}
	}
	return false
}

// GetMeshType returns the detected mesh type
func (d *MultiMeshDetector) GetMeshType() MeshType {
	for meshType, detector := range d.detectors {
		if detector.IsServiceMesh() {
			return meshType
		}
	}
	return MeshTypeUnknown
}

// isCacheValid checks if the cache is still valid
func (d *MultiMeshDetector) isCacheValid() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.cache == nil {
		return false
	}

	return time.Since(d.cacheTime) < d.config.CacheDuration
}

// ClearCache clears the metadata cache
func (d *MultiMeshDetector) ClearCache() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.cache = nil
	d.cacheTime = time.Time{}
}

// Global default detector
var (
	defaultDetector *MultiMeshDetector
	detectorOnce    sync.Once
)

// GetDefaultDetector returns the default service mesh detector
func GetDefaultDetector() *MultiMeshDetector {
	detectorOnce.Do(func() {
		defaultDetector = NewDetector(DefaultConfig())
	})
	return defaultDetector
}

// Detect is a convenience function using the default detector
func Detect() (*Metadata, error) {
	return GetDefaultDetector().Detect()
}

// IsServiceMesh is a convenience function using the default detector
func IsServiceMesh() bool {
	return GetDefaultDetector().IsServiceMesh()
}

// GetMeshType is a convenience function using the default detector
func GetMeshType() MeshType {
	return GetDefaultDetector().GetMeshType()
}
