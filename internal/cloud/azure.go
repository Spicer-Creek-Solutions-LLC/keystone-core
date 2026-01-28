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
	// Azure Instance Metadata Service endpoint
	azureMetadataBaseURL = "http://169.254.169.254/metadata"
	azureMetadataVersion = "2021-02-01"

	// Azure Functions environment variables
	azureFunctionNameEnv    = "AZURE_FUNCTIONS_ENVIRONMENT"
	azureFunctionAppNameEnv = "WEBSITE_SITE_NAME"
	azureFunctionRuntimeEnv = "FUNCTIONS_WORKER_RUNTIME"
)

// AzureDetector detects Azure environments
type AzureDetector struct {
	config     *Config
	httpClient *http.Client
}

// NewAzureDetector creates a new Azure detector
func NewAzureDetector(config *Config) *AzureDetector {
	if config == nil {
		config = DefaultConfig()
	}

	return &AzureDetector{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// Detect attempts to detect Azure environment and collect metadata
func (d *AzureDetector) Detect() (*Metadata, error) {
	// Check environment type
	envType, err := d.detectEnvironmentType()
	if err != nil {
		return nil, err
	}

	metadata := &Metadata{
		Provider:        ProviderAzure,
		EnvironmentType: envType,
		DetectedAt:      time.Now(),
		Tags:            make(map[string]string),
	}

	// Collect metadata based on environment type
	switch envType {
	case EnvTypeServerless:
		if err := d.collectAzureFunctionsMetadata(metadata); err != nil {
			return nil, fmt.Errorf("failed to collect Azure Functions metadata: %w", err)
		}
	case EnvTypeKubernetes:
		if err := d.collectAKSMetadata(metadata); err != nil {
			return nil, fmt.Errorf("failed to collect AKS metadata: %w", err)
		}
	case EnvTypeContainer:
		if err := d.collectContainerInstancesMetadata(metadata); err != nil {
			return nil, fmt.Errorf("failed to collect Container Instances metadata: %w", err)
		}
	case EnvTypeVM:
		if err := d.collectVMMetadata(metadata); err != nil {
			return nil, fmt.Errorf("failed to collect VM metadata: %w", err)
		}
	}

	return metadata, nil
}

// IsCloudEnvironment checks if running in Azure
func (d *AzureDetector) IsCloudEnvironment() bool {
	_, err := d.detectEnvironmentType()
	return err == nil
}

// GetProvider returns Azure as the provider
func (d *AzureDetector) GetProvider() Provider {
	return ProviderAzure
}

// detectEnvironmentType determines the Azure environment type
func (d *AzureDetector) detectEnvironmentType() (EnvironmentType, error) {
	// Check for Azure Functions first
	if d.isAzureFunctions() {
		return EnvTypeServerless, nil
	}

	// Check for AKS
	if d.isAKS() {
		return EnvTypeKubernetes, nil
	}

	// Check for Container Instances
	if d.isContainerInstances() {
		return EnvTypeContainer, nil
	}

	// Check for Azure VM
	if d.isAzureVM() {
		return EnvTypeVM, nil
	}

	return EnvTypeUnknown, fmt.Errorf("not running in Azure")
}

// isAzureFunctions checks if running in Azure Functions
func (d *AzureDetector) isAzureFunctions() bool {
	return os.Getenv(azureFunctionNameEnv) != "" ||
		os.Getenv(azureFunctionRuntimeEnv) != ""
}

// isAKS checks if running in AKS
func (d *AzureDetector) isAKS() bool {
	// Check for Kubernetes service account
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token"); err == nil {
		// Also verify we can access Azure metadata
		return d.isAzureVM()
	}
	return false
}

// isContainerInstances checks if running in Azure Container Instances
func (d *AzureDetector) isContainerInstances() bool {
	// Container Instances have a specific environment variable
	return os.Getenv("CONTAINER_GROUP_NAME") != ""
}

// isAzureVM checks if running on Azure VM
func (d *AzureDetector) isAzureVM() bool {
	req, err := http.NewRequest("GET", azureMetadataBaseURL+"/instance?api-version="+azureMetadataVersion, nil) // nosemgrep: problem-based-packs.insecure-transport.go-stdlib.http-customized-request.http-customized-request -- Azure IMDS uses link-local HTTP
	if err != nil {
		return false
	}

	// Azure requires this header
	req.Header.Set("Metadata", "true")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// collectVMMetadata collects Azure VM metadata
func (d *AzureDetector) collectVMMetadata(metadata *Metadata) error {
	instanceMetadata, err := d.getInstanceMetadata()
	if err != nil {
		return err
	}

	// Compute metadata
	if compute := instanceMetadata.Compute; compute != nil {
		metadata.Region = compute.Location
		metadata.AvailabilityZone = compute.Zone
		metadata.InstanceID = compute.VMID
		metadata.InstanceType = compute.VMSize
		metadata.SubscriptionID = compute.SubscriptionID

		// Tags
		if compute.Tags != "" {
			metadata.Tags = parseAzureTags(compute.Tags)
		}
	}

	// Network metadata
	if network := instanceMetadata.Network; network != nil && len(network.Interface) > 0 {
		iface := network.Interface[0]

		if len(iface.IPv4.IPAddress) > 0 {
			metadata.PrivateIP = iface.IPv4.IPAddress[0].PrivateIPAddress
			metadata.PublicIP = iface.IPv4.IPAddress[0].PublicIPAddress
		}

		if len(iface.IPv4.Subnet) > 0 {
			metadata.SubnetID = iface.IPv4.Subnet[0].Address
		}
	}

	return nil
}

// collectAKSMetadata collects AKS cluster metadata
func (d *AzureDetector) collectAKSMetadata(metadata *Metadata) error {
	// First collect VM metadata
	if err := d.collectVMMetadata(metadata); err != nil {
		return err
	}

	// Collect Kubernetes metadata from environment
	if k8sMetadata := collectK8sMetadataFromEnv(); k8sMetadata != nil {
		metadata.K8s = k8sMetadata
	}

	// Try to get cluster name from tags or compute metadata
	instanceMetadata, err := d.getInstanceMetadata()
	if err == nil && instanceMetadata.Compute != nil {
		if clusterName := instanceMetadata.Compute.ResourceGroupName; clusterName != "" {
			if metadata.K8s == nil {
				metadata.K8s = &K8sMetadata{}
			}
			// AKS resource group often contains cluster name
			metadata.K8s.ClusterName = clusterName
		}
	}

	return nil
}

// collectContainerInstancesMetadata collects Container Instances metadata
func (d *AzureDetector) collectContainerInstancesMetadata(metadata *Metadata) error {
	// Container Instances also expose IMDS
	if err := d.collectVMMetadata(metadata); err != nil {
		return err
	}

	containerGroupName := os.Getenv("CONTAINER_GROUP_NAME")
	containerName := os.Getenv("HOSTNAME")

	metadata.Container = &ContainerMetadata{
		ContainerName: containerName,
		ServiceName:   containerGroupName,
	}

	return nil
}

// collectAzureFunctionsMetadata collects Azure Functions metadata
func (d *AzureDetector) collectAzureFunctionsMetadata(metadata *Metadata) error {
	functionAppName := os.Getenv(azureFunctionAppNameEnv)
	functionName := os.Getenv("FUNCTION_NAME")
	runtime := os.Getenv(azureFunctionRuntimeEnv)

	metadata.Serverless = &ServerlessMetadata{
		FunctionName: functionName,
		Runtime:      runtime,
	}

	// Try to get region from website environment variables
	if region := os.Getenv("REGION_NAME"); region != "" {
		metadata.Region = region
	}

	// Function app name
	if functionAppName != "" {
		if metadata.Serverless.FunctionName == "" {
			metadata.Serverless.FunctionName = functionAppName
		}
	}

	return nil
}

// getInstanceMetadata gets Azure instance metadata
func (d *AzureDetector) getInstanceMetadata() (*azureInstanceMetadata, error) {
	req, err := http.NewRequest("GET", azureMetadataBaseURL+"/instance?api-version="+azureMetadataVersion, nil) // nosemgrep: problem-based-packs.insecure-transport.go-stdlib.http-customized-request.http-customized-request -- Azure IMDS uses link-local HTTP
	if err != nil {
		return nil, err
	}

	req.Header.Set("Metadata", "true")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metadata request failed: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var metadata azureInstanceMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	return &metadata, nil
}

// Helper types for Azure metadata

type azureInstanceMetadata struct {
	Compute *azureComputeMetadata `json:"compute,omitempty"`
	Network *azureNetworkMetadata `json:"network,omitempty"`
}

type azureComputeMetadata struct {
	Location          string `json:"location"`
	Zone              string `json:"zone"`
	VMID              string `json:"vmId"`
	VMSize            string `json:"vmSize"`
	SubscriptionID    string `json:"subscriptionId"`
	ResourceGroupName string `json:"resourceGroupName"`
	Name              string `json:"name"`
	Tags              string `json:"tags"`
	OSType            string `json:"osType"`
}

type azureNetworkMetadata struct {
	Interface []azureNetworkInterface `json:"interface"`
}

type azureNetworkInterface struct {
	IPv4 azureIPv4 `json:"ipv4"`
	IPv6 azureIPv6 `json:"ipv6"`
}

type azureIPv4 struct {
	IPAddress []azureIPAddress `json:"ipAddress"`
	Subnet    []azureSubnet    `json:"subnet"`
}

type azureIPv6 struct {
	IPAddress []azureIPAddress `json:"ipAddress"`
}

type azureIPAddress struct {
	PrivateIPAddress string `json:"privateIpAddress"`
	PublicIPAddress  string `json:"publicIpAddress"`
}

type azureSubnet struct {
	Address string `json:"address"`
	Prefix  string `json:"prefix"`
}

// Helper functions

func parseAzureTags(tagsStr string) map[string]string {
	tags := make(map[string]string)

	// Azure tags come as semicolon-separated key:value pairs
	// Example: "Environment:Production;Application:WebServer"
	pairs := strings.Split(tagsStr, ";")
	for _, pair := range pairs {
		kv := strings.SplitN(pair, ":", 2)
		if len(kv) == 2 {
			tags[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}

	return tags
}
