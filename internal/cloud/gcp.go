package cloud

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// GCP metadata service endpoint
	gcpMetadataBaseURL = "http://metadata.google.internal/computeMetadata/v1"

	// Cloud Functions environment variables
	gcpFunctionTargetEnv = "FUNCTION_TARGET"
	gcpFunctionNameEnv   = "K_SERVICE"       // Cloud Run uses K_SERVICE
	gcpFunctionRevisionEnv = "K_REVISION"    // Cloud Run revision
	gcpFunctionConfigEnv = "K_CONFIGURATION" // Cloud Run configuration
)

// GCPDetector detects GCP environments
type GCPDetector struct {
	config     *Config
	httpClient *http.Client
}

// NewGCPDetector creates a new GCP detector
func NewGCPDetector(config *Config) *GCPDetector {
	if config == nil {
		config = DefaultConfig()
	}

	return &GCPDetector{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// Detect attempts to detect GCP environment and collect metadata
func (d *GCPDetector) Detect() (*Metadata, error) {
	// Check environment type
	envType, err := d.detectEnvironmentType()
	if err != nil {
		return nil, err
	}

	metadata := &Metadata{
		Provider:        ProviderGCP,
		EnvironmentType: envType,
		DetectedAt:      time.Now(),
		Tags:            make(map[string]string),
	}

	// Collect metadata based on environment type
	switch envType {
	case EnvTypeServerless:
		if err := d.collectCloudFunctionsMetadata(metadata); err != nil {
			return nil, fmt.Errorf("failed to collect Cloud Functions metadata: %w", err)
		}
	case EnvTypeKubernetes:
		if err := d.collectGKEMetadata(metadata); err != nil {
			return nil, fmt.Errorf("failed to collect GKE metadata: %w", err)
		}
	case EnvTypeContainer:
		if err := d.collectCloudRunMetadata(metadata); err != nil {
			return nil, fmt.Errorf("failed to collect Cloud Run metadata: %w", err)
		}
	case EnvTypeVM:
		if err := d.collectComputeEngineMetadata(metadata); err != nil {
			return nil, fmt.Errorf("failed to collect Compute Engine metadata: %w", err)
		}
	}

	return metadata, nil
}

// IsCloudEnvironment checks if running in GCP
func (d *GCPDetector) IsCloudEnvironment() bool {
	_, err := d.detectEnvironmentType()
	return err == nil
}

// GetProvider returns GCP as the provider
func (d *GCPDetector) GetProvider() Provider {
	return ProviderGCP
}

// detectEnvironmentType determines the GCP environment type
func (d *GCPDetector) detectEnvironmentType() (EnvironmentType, error) {
	// Check for Cloud Functions/Cloud Run first
	if d.isCloudFunctions() {
		return EnvTypeServerless, nil
	}

	if d.isCloudRun() {
		return EnvTypeContainer, nil
	}

	// Check for GKE
	if d.isGKE() {
		return EnvTypeKubernetes, nil
	}

	// Check for Compute Engine
	if d.isComputeEngine() {
		return EnvTypeVM, nil
	}

	return EnvTypeUnknown, fmt.Errorf("not running in GCP")
}

// isCloudFunctions checks if running in Cloud Functions
func (d *GCPDetector) isCloudFunctions() bool {
	return os.Getenv(gcpFunctionTargetEnv) != ""
}

// isCloudRun checks if running in Cloud Run
func (d *GCPDetector) isCloudRun() bool {
	// Cloud Run sets K_SERVICE but not FUNCTION_TARGET
	return os.Getenv(gcpFunctionNameEnv) != "" && os.Getenv(gcpFunctionTargetEnv) == ""
}

// isGKE checks if running in GKE
func (d *GCPDetector) isGKE() bool {
	// Check for Kubernetes service account
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token"); err == nil {
		// Also verify we can access GCP metadata
		return d.isComputeEngine()
	}
	return false
}

// isComputeEngine checks if running on GCP Compute Engine
func (d *GCPDetector) isComputeEngine() bool {
	req, err := http.NewRequest("GET", gcpMetadataBaseURL, nil)
	if err != nil {
		return false
	}

	// GCP requires this header
	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// Check if response has the Google metadata flavor header
	return resp.Header.Get("Metadata-Flavor") == "Google"
}

// collectComputeEngineMetadata collects Compute Engine instance metadata
func (d *GCPDetector) collectComputeEngineMetadata(metadata *Metadata) error {
	// Get project ID
	if projectID, err := d.getMetadata("/project/project-id"); err == nil {
		metadata.ProjectID = projectID
	}

	// Get instance details
	if instanceID, err := d.getMetadata("/instance/id"); err == nil {
		metadata.InstanceID = instanceID
	}

	if instanceType, err := d.getMetadata("/instance/machine-type"); err == nil {
		// Extract just the machine type name from the full path
		parts := strings.Split(instanceType, "/")
		if len(parts) > 0 {
			metadata.InstanceType = parts[len(parts)-1]
		}
	}

	if zone, err := d.getMetadata("/instance/zone"); err == nil {
		// Extract zone from full path (projects/PROJECT_ID/zones/ZONE)
		parts := strings.Split(zone, "/")
		if len(parts) > 0 {
			metadata.AvailabilityZone = parts[len(parts)-1]
			// Extract region from zone (e.g., us-central1-a -> us-central1)
			zoneParts := strings.Split(metadata.AvailabilityZone, "-")
			if len(zoneParts) >= 2 {
				metadata.Region = strings.Join(zoneParts[:len(zoneParts)-1], "-")
			}
		}
	}

	// Get network information
	if networkName, err := d.getMetadata("/instance/network-interfaces/0/network"); err == nil {
		parts := strings.Split(networkName, "/")
		if len(parts) > 0 {
			metadata.NetworkID = parts[len(parts)-1]
		}
	}

	if subnetName, err := d.getMetadata("/instance/network-interfaces/0/subnetwork"); err == nil {
		parts := strings.Split(subnetName, "/")
		if len(parts) > 0 {
			metadata.SubnetID = parts[len(parts)-1]
		}
	}

	if privateIP, err := d.getMetadata("/instance/network-interfaces/0/ip"); err == nil {
		metadata.PrivateIP = privateIP
	}

	if publicIP, err := d.getMetadata("/instance/network-interfaces/0/access-configs/0/external-ip"); err == nil {
		metadata.PublicIP = publicIP
	}

	// Get instance attributes (labels)
	if attributes, err := d.getInstanceAttributes(); err == nil {
		for k, v := range attributes {
			metadata.Tags[k] = v
		}
	}

	return nil
}

// collectGKEMetadata collects GKE cluster metadata
func (d *GCPDetector) collectGKEMetadata(metadata *Metadata) error {
	// First collect Compute Engine metadata
	if err := d.collectComputeEngineMetadata(metadata); err != nil {
		return err
	}

	// Get cluster name from instance attributes
	if clusterName, err := d.getMetadata("/instance/attributes/cluster-name"); err == nil {
		metadata.K8s = &K8sMetadata{
			ClusterName: clusterName,
		}
	}

	// Collect Kubernetes metadata from environment
	if k8sMetadata := collectK8sMetadataFromEnv(); k8sMetadata != nil {
		if metadata.K8s == nil {
			metadata.K8s = k8sMetadata
		} else {
			// Merge
			metadata.K8s.PodName = k8sMetadata.PodName
			metadata.K8s.PodNamespace = k8sMetadata.PodNamespace
			metadata.K8s.PodUID = k8sMetadata.PodUID
			metadata.K8s.NodeName = k8sMetadata.NodeName
			metadata.K8s.ServiceAccountName = k8sMetadata.ServiceAccountName
		}
	}

	return nil
}

// collectCloudRunMetadata collects Cloud Run service metadata
func (d *GCPDetector) collectCloudRunMetadata(metadata *Metadata) error {
	// Get project ID from metadata service
	if projectID, err := d.getMetadata("/project/project-id"); err == nil {
		metadata.ProjectID = projectID
	}

	// Get region from metadata service
	if region, err := d.getMetadata("/instance/region"); err == nil {
		parts := strings.Split(region, "/")
		if len(parts) > 0 {
			metadata.Region = parts[len(parts)-1]
		}
	}

	serviceName := os.Getenv(gcpFunctionNameEnv)
	revision := os.Getenv(gcpFunctionRevisionEnv)

	metadata.Container = &ContainerMetadata{
		ServiceName: serviceName,
		Revision:    revision,
	}

	return nil
}

// collectCloudFunctionsMetadata collects Cloud Functions metadata
func (d *GCPDetector) collectCloudFunctionsMetadata(metadata *Metadata) error {
	// Get project ID from metadata service
	if projectID, err := d.getMetadata("/project/project-id"); err == nil {
		metadata.ProjectID = projectID
	}

	// Get region from metadata service
	if region, err := d.getMetadata("/instance/region"); err == nil {
		parts := strings.Split(region, "/")
		if len(parts) > 0 {
			metadata.Region = parts[len(parts)-1]
		}
	}

	functionName := os.Getenv(gcpFunctionNameEnv)
	if functionName == "" {
		functionName = os.Getenv("FUNCTION_NAME")
	}

	metadata.Serverless = &ServerlessMetadata{
		FunctionName: functionName,
		Handler:      os.Getenv(gcpFunctionTargetEnv),
		Runtime:      os.Getenv("FUNCTION_RUNTIME"),
		InvocationID: os.Getenv("X_GOOGLE_FUNCTION_EXECUTION_ID"),
	}

	// Parse memory size
	if memStr := os.Getenv("FUNCTION_MEMORY_MB"); memStr != "" {
		var mem int
		fmt.Sscanf(memStr, "%d", &mem)
		metadata.Serverless.MemorySize = mem
	}

	// Parse timeout
	if timeoutStr := os.Getenv("FUNCTION_TIMEOUT_SEC"); timeoutStr != "" {
		var timeout int
		fmt.Sscanf(timeoutStr, "%d", &timeout)
		metadata.Serverless.Timeout = timeout
	}

	return nil
}

// getMetadata gets a metadata value from GCP metadata service
func (d *GCPDetector) getMetadata(path string) (string, error) {
	req, err := http.NewRequest("GET", gcpMetadataBaseURL+path, nil)
	if err != nil {
		return "", err
	}

	// GCP requires this header
	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata request failed: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(data)), nil
}

// getInstanceAttributes gets instance attributes (labels)
func (d *GCPDetector) getInstanceAttributes() (map[string]string, error) {
	req, err := http.NewRequest("GET", gcpMetadataBaseURL+"/instance/attributes/?recursive=true", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("attributes request failed: %d", resp.StatusCode)
	}

	var attributes map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&attributes); err != nil {
		return nil, err
	}

	return attributes, nil
}
