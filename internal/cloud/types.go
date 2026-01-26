package cloud

import "time"

// Provider represents a cloud provider
type Provider int

const (
	// ProviderUnknown represents an unknown or non-cloud environment
	ProviderUnknown Provider = iota
	// ProviderAWS represents Amazon Web Services
	ProviderAWS
	// ProviderGCP represents Google Cloud Platform
	ProviderGCP
	// ProviderAzure represents Microsoft Azure
	ProviderAzure
)

func (p Provider) String() string {
	switch p {
	case ProviderAWS:
		return "aws"
	case ProviderGCP:
		return "gcp"
	case ProviderAzure:
		return "azure"
	default:
		return "unknown"
	}
}

// EnvironmentType represents the cloud environment type
type EnvironmentType int

const (
	// EnvTypeUnknown represents an unknown environment
	EnvTypeUnknown EnvironmentType = iota
	// EnvTypeVM represents a virtual machine
	EnvTypeVM
	// EnvTypeContainer represents a container (ECS, Cloud Run, Container Instances)
	EnvTypeContainer
	// EnvTypeKubernetes represents a Kubernetes pod
	EnvTypeKubernetes
	// EnvTypeServerless represents a serverless function (Lambda, Cloud Functions, Azure Functions)
	EnvTypeServerless
)

func (e EnvironmentType) String() string {
	switch e {
	case EnvTypeVM:
		return "vm"
	case EnvTypeContainer:
		return "container"
	case EnvTypeKubernetes:
		return "kubernetes"
	case EnvTypeServerless:
		return "serverless"
	default:
		return "unknown"
	}
}

// Metadata represents cloud-specific metadata
type Metadata struct {
	// Provider is the detected cloud provider
	Provider Provider

	// EnvironmentType is the type of cloud environment
	EnvironmentType EnvironmentType

	// Region is the cloud region (e.g., us-east-1, us-central1, eastus)
	Region string

	// AvailabilityZone is the availability zone
	AvailabilityZone string

	// InstanceID is the unique instance identifier
	InstanceID string

	// InstanceType is the instance type/size (e.g., t3.micro, n1-standard-1, Standard_D2s_v3)
	InstanceType string

	// AccountID is the cloud account identifier
	AccountID string

	// ProjectID is the project identifier (GCP)
	ProjectID string

	// SubscriptionID is the subscription identifier (Azure)
	SubscriptionID string

	// VPC/Network information
	VPCID     string
	SubnetID  string
	NetworkID string

	// Private and public IP addresses
	PrivateIP string
	PublicIP  string

	// Tags/Labels from cloud provider
	Tags map[string]string

	// Kubernetes-specific metadata (if running in K8s)
	K8s *K8sMetadata

	// Container-specific metadata (if running in container)
	Container *ContainerMetadata

	// Serverless-specific metadata (if running as function)
	Serverless *ServerlessMetadata

	// DetectedAt is when the metadata was collected
	DetectedAt time.Time
}

// K8sMetadata represents Kubernetes-specific metadata
type K8sMetadata struct {
	// PodName is the name of the pod
	PodName string

	// PodNamespace is the namespace of the pod
	PodNamespace string

	// PodUID is the unique identifier of the pod
	PodUID string

	// NodeName is the name of the node running the pod
	NodeName string

	// ServiceAccountName is the service account used by the pod
	ServiceAccountName string

	// ClusterName is the name of the cluster (if available)
	ClusterName string

	// Labels are the pod labels
	Labels map[string]string

	// Annotations are the pod annotations
	Annotations map[string]string
}

// ContainerMetadata represents container-specific metadata
type ContainerMetadata struct {
	// ContainerID is the unique container identifier
	ContainerID string

	// ContainerName is the name of the container
	ContainerName string

	// ImageName is the container image name
	ImageName string

	// ImageDigest is the image digest (SHA256)
	ImageDigest string

	// TaskARN is the ECS task ARN (AWS)
	TaskARN string

	// TaskDefinition is the ECS task definition (AWS)
	TaskDefinition string

	// ClusterARN is the ECS cluster ARN (AWS)
	ClusterARN string

	// ServiceName is the Cloud Run service name (GCP) or Container Instances group (Azure)
	ServiceName string

	// Revision is the Cloud Run revision (GCP)
	Revision string
}

// ServerlessMetadata represents serverless function metadata
type ServerlessMetadata struct {
	// FunctionName is the name of the function
	FunctionName string

	// FunctionVersion is the version of the function
	FunctionVersion string

	// FunctionARN is the function ARN (AWS Lambda)
	FunctionARN string

	// Handler is the function handler
	Handler string

	// Runtime is the function runtime (e.g., python3.9, nodejs18)
	Runtime string

	// MemorySize is the allocated memory in MB
	MemorySize int

	// Timeout is the function timeout in seconds
	Timeout int

	// RequestID is the current request ID (if invoked)
	RequestID string

	// InvocationID is the invocation ID (GCP Cloud Functions)
	InvocationID string
}

// Detector is the interface for cloud metadata detection
type Detector interface {
	// Detect attempts to detect cloud provider and collect metadata
	Detect() (*Metadata, error)

	// IsCloudEnvironment checks if running in a cloud environment
	IsCloudEnvironment() bool

	// GetProvider returns the detected cloud provider
	GetProvider() Provider
}

// Config holds configuration for cloud detection
type Config struct {
	// Timeout for metadata API requests
	Timeout time.Duration

	// EnableAWS enables AWS detection
	EnableAWS bool

	// EnableGCP enables GCP detection
	EnableGCP bool

	// EnableAzure enables Azure detection
	EnableAzure bool

	// EnableKubernetes enables Kubernetes detection
	EnableKubernetes bool

	// CacheDuration is how long to cache metadata
	CacheDuration time.Duration
}

// DefaultConfig returns the default cloud detection configuration
func DefaultConfig() *Config {
	return &Config{
		Timeout:          5 * time.Second,
		EnableAWS:        true,
		EnableGCP:        true,
		EnableAzure:      true,
		EnableKubernetes: true,
		CacheDuration:    5 * time.Minute,
	}
}
