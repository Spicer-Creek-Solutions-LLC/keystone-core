package k8s

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ExecutionMode defines how commands are executed in Kubernetes
type ExecutionMode string

const (
	// ExecModePod executes commands in a pod
	ExecModePod ExecutionMode = "pod"
	// ExecModeJob creates a Kubernetes Job
	ExecModeJob ExecutionMode = "job"
	// ExecModeNode executes on the node (requires node agent)
	ExecModeNode ExecutionMode = "node"
)

// ClusterConfig holds Kubernetes cluster configuration
type ClusterConfig struct {
	// Name is the cluster identifier
	Name string
	// Kubeconfig is the path to kubeconfig file (optional, uses in-cluster config if empty)
	Kubeconfig string
	// Context is the kubeconfig context to use (optional, uses current context if empty)
	Context string
	// APIServer is the Kubernetes API server URL (optional, from kubeconfig)
	APIServer string
	// Token for authentication (optional, from kubeconfig)
	Token string
	// CertificateAuthority is the CA certificate path (optional, from kubeconfig)
	CertificateAuthority string
	// Namespace is the default namespace for operations
	Namespace string
	// Timeout for API operations
	Timeout time.Duration
}

// PodExecOptions defines options for pod command execution
type PodExecOptions struct {
	// Namespace of the pod
	Namespace string
	// PodName is the name of the pod
	PodName string
	// Container is the container name (optional, uses first container if empty)
	Container string
	// Command to execute
	Command []string
	// Stdin enables stdin
	Stdin bool
	// Stdout enables stdout capture
	Stdout bool
	// Stderr enables stderr capture
	Stderr bool
	// TTY allocates a terminal
	TTY bool
	// Timeout for execution
	Timeout time.Duration
}

// PodExecResult contains the result of pod command execution
type PodExecResult struct {
	// ExitCode of the command
	ExitCode int
	// Stdout output
	Stdout string
	// Stderr output
	Stderr string
	// Error if execution failed
	Error error
	// Duration of execution
	Duration time.Duration
}

// PodSelector defines how to select pods for execution
type PodSelector struct {
	// Namespace to search in (empty = all namespaces)
	Namespace string
	// LabelSelector in Kubernetes label selector format (e.g., "app=nginx,tier=frontend")
	LabelSelector string
	// FieldSelector in Kubernetes field selector format (e.g., "status.phase=Running")
	FieldSelector string
	// Names is a list of specific pod names
	Names []string
	// Container to target (empty = first container)
	Container string
	// MaxPods limits the number of pods to execute on (0 = no limit)
	MaxPods int
}

// ResourceInfo holds Kubernetes resource information
type ResourceInfo struct {
	// Kind is the resource kind (e.g., "Pod", "Deployment")
	Kind string
	// Namespace of the resource
	Namespace string
	// Name of the resource
	Name string
	// Labels attached to the resource
	Labels map[string]string
	// Annotations attached to the resource
	Annotations map[string]string
	// Status of the resource
	Status ResourceStatus
	// CreationTimestamp when the resource was created
	CreationTimestamp time.Time
	// Metadata contains additional resource metadata
	Metadata map[string]interface{}
}

// ResourceStatus represents the status of a Kubernetes resource
type ResourceStatus string

const (
	// StatusRunning indicates the resource is running
	StatusRunning ResourceStatus = "Running"
	// StatusPending indicates the resource is pending
	StatusPending ResourceStatus = "Pending"
	// StatusSucceeded indicates the resource succeeded
	StatusSucceeded ResourceStatus = "Succeeded"
	// StatusFailed indicates the resource failed
	StatusFailed ResourceStatus = "Failed"
	// StatusUnknown indicates unknown status
	StatusUnknown ResourceStatus = "Unknown"
)

// DeploymentInfo holds deployment-specific information
type DeploymentInfo struct {
	ResourceInfo
	// Replicas is the desired number of replicas
	Replicas int32
	// AvailableReplicas is the number of available replicas
	AvailableReplicas int32
	// ReadyReplicas is the number of ready replicas
	ReadyReplicas int32
	// UpdatedReplicas is the number of updated replicas
	UpdatedReplicas int32
}

// ServiceInfo holds service-specific information
type ServiceInfo struct {
	ResourceInfo
	// Type is the service type (ClusterIP, NodePort, LoadBalancer)
	Type string
	// ClusterIP is the service cluster IP
	ClusterIP string
	// ExternalIPs are external IPs
	ExternalIPs []string
	// Ports are service ports
	Ports []ServicePort
}

// ServicePort represents a service port
type ServicePort struct {
	// Name of the port
	Name string
	// Protocol (TCP, UDP, SCTP)
	Protocol string
	// Port number
	Port int32
	// TargetPort on the pod
	TargetPort int32
	// NodePort for NodePort/LoadBalancer services
	NodePort int32
}

// WatchEvent represents a Kubernetes resource watch event
type WatchEvent struct {
	// Type is the event type (ADDED, MODIFIED, DELETED)
	Type string
	// Resource is the affected resource
	Resource ResourceInfo
	// Timestamp when the event occurred
	Timestamp time.Time
}

// OperatorConfig holds operator configuration
type OperatorConfig struct {
	// Namespace where the operator runs
	Namespace string
	// LeaderElection enables leader election
	LeaderElection bool
	// LeaderElectionID is the lease name for leader election
	LeaderElectionID string
	// MetricsAddr is the address for metrics endpoint
	MetricsAddr string
	// ProbeAddr is the address for health probes
	ProbeAddr string
	// ReconcileInterval is how often to reconcile resources
	ReconcileInterval time.Duration
	// MaxConcurrentReconciles is the max number of concurrent reconciles
	MaxConcurrentReconciles int
}

// RemoteExecutionSpec defines the desired state of RemoteExecution CRD
type RemoteExecutionSpec struct {
	// Target defines pod selection criteria
	Target PodSelector
	// Command to execute
	Command []string
	// Schedule in cron format (optional, for scheduled execution)
	Schedule string
	// Timeout for execution
	Timeout metav1.Duration
	// Mode is the execution mode (pod, job, node)
	Mode ExecutionMode
}

// RemoteExecutionStatus defines the observed state of RemoteExecution
type RemoteExecutionStatus struct {
	// Phase is the current phase (Pending, Running, Succeeded, Failed)
	Phase string
	// StartTime when execution started
	StartTime *metav1.Time
	// CompletionTime when execution completed
	CompletionTime *metav1.Time
	// PodsExecuted is the number of pods where command was executed
	PodsExecuted int
	// PodsSucceeded is the number of pods where command succeeded
	PodsSucceeded int
	// PodsFailed is the number of pods where command failed
	PodsFailed int
	// Message provides additional information
	Message string
	// Results contains per-pod execution results
	Results []PodExecutionResult
}

// PodExecutionResult holds the result for a single pod
type PodExecutionResult struct {
	// PodName is the name of the pod
	PodName string
	// Namespace of the pod
	Namespace string
	// ExitCode of the command
	ExitCode int
	// Output from the command
	Output string
	// Error message if execution failed
	Error string
	// Duration of execution
	Duration metav1.Duration
}

// StateConfigSpec defines the desired state of StateConfig CRD
type StateConfigSpec struct {
	// Target defines pod selection criteria
	Target PodSelector
	// States contains state declarations
	States []StateDeclaration
	// Vars contains variables for template rendering
	Vars map[string]interface{}
	// Schedule in cron format (optional, for scheduled application)
	Schedule string
}

// StateDeclaration defines a single state
type StateDeclaration struct {
	// Name of the state
	Name string
	// Module type (file, package, service, etc.)
	Module string
	// Parameters for the module
	Parameters map[string]interface{}
	// Requisites for dependency ordering
	Requisites map[string][]string
}

// StateConfigStatus defines the observed state of StateConfig
type StateConfigStatus struct {
	// Phase is the current phase (Pending, Applying, Applied, Failed)
	Phase string
	// LastApplied when the state was last applied
	LastApplied *metav1.Time
	// PodsApplied is the number of pods where state was applied
	PodsApplied int
	// PodsSucceeded is the number of pods where state application succeeded
	PodsSucceeded int
	// PodsFailed is the number of pods where state application failed
	PodsFailed int
	// Message provides additional information
	Message string
	// DriftDetected indicates if drift was detected
	DriftDetected bool
}

// ClientInterface defines the interface for Kubernetes client operations
type ClientInterface interface {
	// ExecInPod executes a command in a specific pod
	ExecInPod(opts PodExecOptions) (*PodExecResult, error)

	// ExecInPods executes a command in multiple pods matching selector
	ExecInPods(selector PodSelector, command []string) ([]PodExecResult, error)

	// GetPod retrieves pod information
	GetPod(namespace, name string) (*ResourceInfo, error)

	// ListPods lists pods matching selector
	ListPods(selector PodSelector) ([]ResourceInfo, error)

	// GetDeployment retrieves deployment information
	GetDeployment(namespace, name string) (*DeploymentInfo, error)

	// GetService retrieves service information
	GetService(namespace, name string) (*ServiceInfo, error)

	// WatchPods watches for pod events
	WatchPods(selector PodSelector) (<-chan WatchEvent, error)

	// CreateResource creates a Kubernetes resource
	CreateResource(namespace string, manifest []byte) error

	// UpdateResource updates a Kubernetes resource
	UpdateResource(namespace string, manifest []byte) error

	// DeleteResource deletes a Kubernetes resource
	DeleteResource(namespace, kind, name string) error

	// GetClusterInfo returns information about the cluster
	GetClusterInfo() (*ClusterInfo, error)
}

// ClusterInfo holds information about a Kubernetes cluster
type ClusterInfo struct {
	// Version is the Kubernetes version
	Version string
	// Nodes is the number of nodes
	Nodes int
	// Namespaces is the number of namespaces
	Namespaces int
	// APIServer is the API server endpoint
	APIServer string
}
