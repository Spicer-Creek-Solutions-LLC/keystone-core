package cloud

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// MultiCloudDetector detects cloud provider across AWS, GCP, and Azure
type MultiCloudDetector struct {
	config    *Config
	detectors map[Provider]Detector
	cache     *Metadata
	cacheTime time.Time
	mu        sync.RWMutex
}

// NewDetector creates a new multi-cloud detector
func NewDetector(config *Config) *MultiCloudDetector {
	if config == nil {
		config = DefaultConfig()
	}

	d := &MultiCloudDetector{
		config:    config,
		detectors: make(map[Provider]Detector),
	}

	// Register enabled detectors
	if config.EnableAWS {
		d.detectors[ProviderAWS] = NewAWSDetector(config)
	}

	if config.EnableGCP {
		d.detectors[ProviderGCP] = NewGCPDetector(config)
	}

	if config.EnableAzure {
		d.detectors[ProviderAzure] = NewAzureDetector(config)
	}

	return d
}

// Detect attempts to detect cloud provider and collect metadata
func (d *MultiCloudDetector) Detect() (*Metadata, error) {
	// Check cache
	if d.isCacheValid() {
		d.mu.RLock()
		defer d.mu.RUnlock()
		return d.cache, nil
	}

	// Try each detector
	for _, detector := range d.detectors {
		if detector.IsCloudEnvironment() {
			metadata, err := detector.Detect()
			if err == nil {
				// Cache the result
				d.mu.Lock()
				d.cache = metadata
				d.cacheTime = time.Now()
				d.mu.Unlock()

				// Enrich with Kubernetes metadata if enabled
				if d.config.EnableKubernetes && metadata.EnvironmentType != EnvTypeKubernetes {
					if k8sMetadata := collectK8sMetadataFromEnv(); k8sMetadata != nil {
						metadata.K8s = k8sMetadata
						metadata.EnvironmentType = EnvTypeKubernetes
					}
				}

				return metadata, nil
			}
		}
	}

	return nil, fmt.Errorf("no cloud environment detected")
}

// IsCloudEnvironment checks if running in any cloud environment
func (d *MultiCloudDetector) IsCloudEnvironment() bool {
	for _, detector := range d.detectors {
		if detector.IsCloudEnvironment() {
			return true
		}
	}
	return false
}

// GetProvider returns the detected cloud provider
func (d *MultiCloudDetector) GetProvider() Provider {
	for provider, detector := range d.detectors {
		if detector.IsCloudEnvironment() {
			return provider
		}
	}
	return ProviderUnknown
}

// isCacheValid checks if the cache is still valid
func (d *MultiCloudDetector) isCacheValid() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.cache == nil {
		return false
	}

	return time.Since(d.cacheTime) < d.config.CacheDuration
}

// ClearCache clears the metadata cache
func (d *MultiCloudDetector) ClearCache() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.cache = nil
	d.cacheTime = time.Time{}
}

// Global default detector
var (
	defaultDetector *MultiCloudDetector
	detectorOnce    sync.Once
)

// GetDefaultDetector returns the default cloud detector
func GetDefaultDetector() *MultiCloudDetector {
	detectorOnce.Do(func() {
		defaultDetector = NewDetector(DefaultConfig())
	})
	return defaultDetector
}

// Detect is a convenience function using the default detector
func Detect() (*Metadata, error) {
	return GetDefaultDetector().Detect()
}

// IsCloudEnvironment is a convenience function using the default detector
func IsCloudEnvironment() bool {
	return GetDefaultDetector().IsCloudEnvironment()
}

// GetProvider is a convenience function using the default detector
func GetProvider() Provider {
	return GetDefaultDetector().GetProvider()
}

// collectK8sMetadataFromEnv collects Kubernetes metadata from environment variables
// This is used when running in Kubernetes across any cloud provider
func collectK8sMetadataFromEnv() *K8sMetadata {
	// Check if we're in Kubernetes by looking for service account token
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token"); err != nil {
		return nil
	}

	metadata := &K8sMetadata{
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
	}

	// Collect from downward API environment variables
	// These are commonly set via Kubernetes downward API
	metadata.PodName = os.Getenv("POD_NAME")
	metadata.PodNamespace = os.Getenv("POD_NAMESPACE")
	metadata.PodUID = os.Getenv("POD_UID")
	metadata.NodeName = os.Getenv("NODE_NAME")
	metadata.ServiceAccountName = os.Getenv("SERVICE_ACCOUNT")

	// If POD_NAME is not set, try HOSTNAME (often the pod name)
	if metadata.PodName == "" {
		metadata.PodName = os.Getenv("HOSTNAME")
	}

	// Read service account name from file if not in env
	if metadata.ServiceAccountName == "" {
		if data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
			// Use namespace as fallback if service account not available
			if metadata.PodNamespace == "" {
				metadata.PodNamespace = strings.TrimSpace(string(data))
			}
		}
	}

	// Parse labels from environment (if set via downward API)
	// Labels are often set as LABEL_KEY=value
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "LABEL_") {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimPrefix(parts[0], "LABEL_")
				metadata.Labels[key] = parts[1]
			}
		}
	}

	// Only return if we found at least some K8s metadata
	if metadata.PodName != "" || metadata.PodNamespace != "" {
		return metadata
	}

	return nil
}
