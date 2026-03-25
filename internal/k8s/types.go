package k8s

import (
	"context"
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

// StatefulSetInfo holds statefulset-specific information
type StatefulSetInfo struct {
	ResourceInfo
	// Replicas is the desired number of replicas
	Replicas int32
	// ReadyReplicas is the number of ready replicas
	ReadyReplicas int32
	// CurrentReplicas is the number of current replicas
	CurrentReplicas int32
	// UpdatedReplicas is the number of updated replicas
	UpdatedReplicas int32
	// CurrentRevision is the current revision
	CurrentRevision string
	// UpdateRevision is the update revision
	UpdateRevision string
	// ServiceName is the headless service name
	ServiceName string
	// PodManagementPolicy is the pod management policy (OrderedReady or Parallel)
	PodManagementPolicy string
	// UpdateStrategy is the update strategy (RollingUpdate or OnDelete)
	UpdateStrategy string
}

// DaemonSetInfo holds daemonset-specific information
type DaemonSetInfo struct {
	ResourceInfo
	// DesiredNumberScheduled is the desired number of pods
	DesiredNumberScheduled int32
	// CurrentNumberScheduled is the current number of pods
	CurrentNumberScheduled int32
	// NumberReady is the number of ready pods
	NumberReady int32
	// NumberAvailable is the number of available pods
	NumberAvailable int32
	// NumberMisscheduled is the number of misscheduled pods
	NumberMisscheduled int32
	// UpdatedNumberScheduled is the number of updated pods
	UpdatedNumberScheduled int32
	// UpdateStrategy is the update strategy (RollingUpdate or OnDelete)
	UpdateStrategy string
}

// JobInfo holds job-specific information
type JobInfo struct {
	ResourceInfo
	// Active is the number of actively running pods
	Active int32
	// Succeeded is the number of pods that completed successfully
	Succeeded int32
	// Failed is the number of pods that failed
	Failed int32
	// Completions is the desired number of completions
	Completions int32
	// Parallelism is the max number of pods running at once
	Parallelism int32
	// BackoffLimit is the number of retries before marking as failed
	BackoffLimit int32
	// StartTime is when the job started
	StartTime *time.Time
	// CompletionTime is when the job completed
	CompletionTime *time.Time
}

// CronJobInfo holds cronjob-specific information
type CronJobInfo struct {
	ResourceInfo
	// Schedule is the cron schedule string
	Schedule string
	// Suspend indicates if the cronjob is suspended
	Suspend bool
	// ConcurrencyPolicy is Allow, Forbid, or Replace
	ConcurrencyPolicy string
	// LastScheduleTime is when the job was last scheduled
	LastScheduleTime *time.Time
	// LastSuccessfulTime is when the job last succeeded
	LastSuccessfulTime *time.Time
	// ActiveJobs is the number of currently running jobs
	ActiveJobs int32
}

// PVCInfo holds persistent volume claim information
type PVCInfo struct {
	ResourceInfo
	// Phase is the PVC phase (Pending, Bound, Lost)
	Phase string
	// StorageClassName is the storage class
	StorageClassName string
	// VolumeName is the bound volume name
	VolumeName string
	// AccessModes are the access modes
	AccessModes []string
	// RequestedStorage is the requested storage size
	RequestedStorage string
	// AllocatedStorage is the actual allocated storage
	AllocatedStorage string
}

// HPAInfo holds horizontal pod autoscaler information
type HPAInfo struct {
	ResourceInfo
	// MinReplicas is the minimum number of replicas
	MinReplicas int32
	// MaxReplicas is the maximum number of replicas
	MaxReplicas int32
	// CurrentReplicas is the current number of replicas
	CurrentReplicas int32
	// DesiredReplicas is the desired number of replicas
	DesiredReplicas int32
	// TargetKind is the kind of the scale target (Deployment, StatefulSet, etc.)
	TargetKind string
	// TargetName is the name of the scale target
	TargetName string
	// CurrentCPUUtilization is the current CPU utilization percentage
	CurrentCPUUtilization *int32
	// TargetCPUUtilization is the target CPU utilization percentage
	TargetCPUUtilization *int32
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
	ExecInPod(ctx context.Context, opts PodExecOptions) (*PodExecResult, error)

	// ExecInPods executes a command in multiple pods matching selector
	ExecInPods(ctx context.Context, selector PodSelector, command []string) ([]PodExecResult, error)

	// GetPod retrieves pod information
	GetPod(ctx context.Context, namespace, name string) (*ResourceInfo, error)

	// ListPods lists pods matching selector
	ListPods(ctx context.Context, selector PodSelector) ([]ResourceInfo, error)

	// GetDeployment retrieves deployment information
	GetDeployment(ctx context.Context, namespace, name string) (*DeploymentInfo, error)

	// GetService retrieves service information
	GetService(ctx context.Context, namespace, name string) (*ServiceInfo, error)

	// WatchPods watches for pod events
	WatchPods(ctx context.Context, selector PodSelector) (<-chan WatchEvent, error)

	// CreateResource creates a Kubernetes resource
	CreateResource(namespace string, manifest []byte) error

	// UpdateResource updates a Kubernetes resource
	UpdateResource(namespace string, manifest []byte) error

	// DeleteResource deletes a Kubernetes resource
	DeleteResource(namespace, kind, name string) error

	// GetClusterInfo returns information about the cluster
	GetClusterInfo(ctx context.Context) (*ClusterInfo, error)

	// GetNamespace retrieves namespace information
	GetNamespace(ctx context.Context, name string) (*NamespaceInfo, error)

	// ListNamespaces lists all namespaces
	ListNamespaces(ctx context.Context) ([]NamespaceInfo, error)

	// CreateNamespace creates a new namespace
	CreateNamespace(ctx context.Context, spec NamespaceSpec) error

	// UpdateNamespace updates a namespace's labels and annotations
	UpdateNamespace(ctx context.Context, spec NamespaceSpec) error

	// DeleteNamespace deletes a namespace
	DeleteNamespace(ctx context.Context, name string) error

	// CreateDeployment creates a new deployment
	CreateDeployment(ctx context.Context, namespace string, spec DeploymentSpec) error

	// UpdateDeployment updates a deployment
	UpdateDeployment(ctx context.Context, namespace string, spec DeploymentSpec) error

	// DeleteDeployment deletes a deployment
	DeleteDeployment(ctx context.Context, namespace, name string) error

	// ScaleDeployment scales a deployment to specified replicas
	ScaleDeployment(ctx context.Context, namespace, name string, replicas int32) error

	// CreateService creates a new service
	CreateService(ctx context.Context, namespace string, spec ServiceSpec) error

	// UpdateService updates a service
	UpdateService(ctx context.Context, namespace string, spec ServiceSpec) error

	// DeleteService deletes a service
	DeleteService(ctx context.Context, namespace, name string) error

	// GetConfigMap retrieves configmap information
	GetConfigMap(ctx context.Context, namespace, name string) (*ConfigMapInfo, error)

	// CreateConfigMap creates a new configmap
	CreateConfigMap(ctx context.Context, namespace string, spec ConfigMapSpec) error

	// UpdateConfigMap updates a configmap
	UpdateConfigMap(ctx context.Context, namespace string, spec ConfigMapSpec) error

	// DeleteConfigMap deletes a configmap
	DeleteConfigMap(ctx context.Context, namespace, name string) error

	// GetSecret retrieves secret information
	GetSecret(ctx context.Context, namespace, name string) (*SecretInfo, error)

	// CreateSecret creates a new secret
	CreateSecret(ctx context.Context, namespace string, spec SecretSpec) error

	// UpdateSecret updates a secret
	UpdateSecret(ctx context.Context, namespace string, spec SecretSpec) error

	// DeleteSecret deletes a secret
	DeleteSecret(ctx context.Context, namespace, name string) error

	// GetIngress retrieves ingress information
	GetIngress(ctx context.Context, namespace, name string) (*IngressInfo, error)

	// CreateIngress creates a new ingress
	CreateIngress(ctx context.Context, namespace string, spec IngressSpec) error

	// UpdateIngress updates an ingress
	UpdateIngress(ctx context.Context, namespace string, spec IngressSpec) error

	// DeleteIngress deletes an ingress
	DeleteIngress(ctx context.Context, namespace, name string) error

	// GetStatefulSet retrieves statefulset information
	GetStatefulSet(ctx context.Context, namespace, name string) (*StatefulSetInfo, error)

	// CreateStatefulSet creates a new statefulset
	CreateStatefulSet(ctx context.Context, namespace string, spec StatefulSetSpec) error

	// UpdateStatefulSet updates a statefulset
	UpdateStatefulSet(ctx context.Context, namespace string, spec StatefulSetSpec) error

	// DeleteStatefulSet deletes a statefulset
	DeleteStatefulSet(ctx context.Context, namespace, name string) error

	// ScaleStatefulSet scales a statefulset to specified replicas
	ScaleStatefulSet(ctx context.Context, namespace, name string, replicas int32) error

	// GetDaemonSet retrieves daemonset information
	GetDaemonSet(ctx context.Context, namespace, name string) (*DaemonSetInfo, error)

	// CreateDaemonSet creates a new daemonset
	CreateDaemonSet(ctx context.Context, namespace string, spec DaemonSetSpec) error

	// UpdateDaemonSet updates a daemonset
	UpdateDaemonSet(ctx context.Context, namespace string, spec DaemonSetSpec) error

	// DeleteDaemonSet deletes a daemonset
	DeleteDaemonSet(ctx context.Context, namespace, name string) error

	// GetJob retrieves job information
	GetJob(ctx context.Context, namespace, name string) (*JobInfo, error)

	// CreateJob creates a new job
	CreateJob(ctx context.Context, namespace string, spec JobSpec) error

	// DeleteJob deletes a job
	DeleteJob(ctx context.Context, namespace, name string) error

	// GetCronJob retrieves cronjob information
	GetCronJob(ctx context.Context, namespace, name string) (*CronJobInfo, error)

	// CreateCronJob creates a new cronjob
	CreateCronJob(ctx context.Context, namespace string, spec CronJobSpec) error

	// UpdateCronJob updates a cronjob
	UpdateCronJob(ctx context.Context, namespace string, spec CronJobSpec) error

	// DeleteCronJob deletes a cronjob
	DeleteCronJob(ctx context.Context, namespace, name string) error

	// GetPVC retrieves persistent volume claim information
	GetPVC(ctx context.Context, namespace, name string) (*PVCInfo, error)

	// CreatePVC creates a new persistent volume claim
	CreatePVC(ctx context.Context, namespace string, spec PVCSpec) error

	// UpdatePVC updates a persistent volume claim
	UpdatePVC(ctx context.Context, namespace string, spec PVCSpec) error

	// DeletePVC deletes a persistent volume claim
	DeletePVC(ctx context.Context, namespace, name string) error

	// GetHPA retrieves horizontal pod autoscaler information
	GetHPA(ctx context.Context, namespace, name string) (*HPAInfo, error)

	// CreateHPA creates a new horizontal pod autoscaler
	CreateHPA(ctx context.Context, namespace string, spec HPASpec) error

	// UpdateHPA updates a horizontal pod autoscaler
	UpdateHPA(ctx context.Context, namespace string, spec HPASpec) error

	// DeleteHPA deletes a horizontal pod autoscaler
	DeleteHPA(ctx context.Context, namespace, name string) error
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

// NamespaceInfo holds namespace-specific information
type NamespaceInfo struct {
	ResourceInfo
	// Phase is the namespace phase (Active, Terminating)
	Phase string
	// Finalizers are namespace finalizers
	Finalizers []string
}

// NamespaceSpec defines namespace creation parameters
type NamespaceSpec struct {
	// Name of the namespace
	Name string
	// Labels to apply to the namespace
	Labels map[string]string
	// Annotations to apply to the namespace
	Annotations map[string]string
}

// DeploymentSpec defines deployment creation/update parameters
type DeploymentSpec struct {
	// Name of the deployment
	Name string
	// Replicas is the desired number of replicas
	Replicas int32
	// Labels to apply to the deployment
	Labels map[string]string
	// Annotations to apply to the deployment
	Annotations map[string]string
	// Image is the container image (required for create)
	Image string
	// ContainerPort is the container port to expose
	ContainerPort int32
	// Selector labels for pod selection
	Selector map[string]string
}

// StatefulSetSpec defines statefulset creation/update parameters
type StatefulSetSpec struct {
	// Name of the statefulset
	Name string
	// Replicas is the desired number of replicas
	Replicas int32
	// Labels to apply to the statefulset
	Labels map[string]string
	// Annotations to apply to the statefulset
	Annotations map[string]string
	// Image is the container image (required for create)
	Image string
	// ContainerPort is the container port to expose
	ContainerPort int32
	// Selector labels for pod selection
	Selector map[string]string
	// ServiceName is the headless service name (required for create)
	ServiceName string
	// PodManagementPolicy is the pod management policy (OrderedReady or Parallel)
	// Default: OrderedReady
	PodManagementPolicy string
	// UpdateStrategy is the update strategy (RollingUpdate or OnDelete)
	// Default: RollingUpdate
	UpdateStrategy string
	// VolumeClaimTemplates defines persistent volume claims for stateful pods
	VolumeClaimTemplates []VolumeClaimTemplate
}

// VolumeClaimTemplate defines a persistent volume claim template for StatefulSet
type VolumeClaimTemplate struct {
	// Name of the volume claim
	Name string
	// StorageClassName is the storage class for the volume
	StorageClassName string
	// AccessModes are the access modes for the volume (ReadWriteOnce, ReadOnlyMany, ReadWriteMany)
	AccessModes []string
	// StorageSize is the storage size (e.g., "10Gi")
	StorageSize string
}

// DaemonSetSpec defines daemonset creation/update parameters
type DaemonSetSpec struct {
	// Name of the daemonset
	Name string
	// Labels to apply to the daemonset
	Labels map[string]string
	// Annotations to apply to the daemonset
	Annotations map[string]string
	// Image is the container image (required for create)
	Image string
	// ContainerPort is the container port to expose
	ContainerPort int32
	// Selector labels for pod selection
	Selector map[string]string
	// UpdateStrategy is the update strategy (RollingUpdate or OnDelete)
	// Default: RollingUpdate
	UpdateStrategy string
	// NodeSelector for scheduling pods on specific nodes
	NodeSelector map[string]string
}

// JobSpec defines job creation/update parameters
type JobSpec struct {
	// Name of the job
	Name string
	// Labels to apply to the job
	Labels map[string]string
	// Annotations to apply to the job
	Annotations map[string]string
	// Image is the container image (required for create)
	Image string
	// Command is the command to run
	Command []string
	// Args are the command arguments
	Args []string
	// Completions is the desired number of completions (default: 1)
	Completions int32
	// Parallelism is the max number of pods running at once (default: 1)
	Parallelism int32
	// BackoffLimit is the number of retries before marking as failed (default: 6)
	BackoffLimit int32
	// ActiveDeadlineSeconds is the max duration the job can run
	ActiveDeadlineSeconds *int64
	// TTLSecondsAfterFinished is how long to keep the job after completion
	TTLSecondsAfterFinished *int32
	// RestartPolicy is the restart policy (Never or OnFailure)
	RestartPolicy string
}

// CronJobSpec defines cronjob creation/update parameters
type CronJobSpec struct {
	// Name of the cronjob
	Name string
	// Labels to apply to the cronjob
	Labels map[string]string
	// Annotations to apply to the cronjob
	Annotations map[string]string
	// Schedule is the cron schedule string (required)
	Schedule string
	// Image is the container image (required for create)
	Image string
	// Command is the command to run
	Command []string
	// Args are the command arguments
	Args []string
	// Suspend indicates if the cronjob should be suspended
	Suspend bool
	// ConcurrencyPolicy is Allow, Forbid, or Replace (default: Allow)
	ConcurrencyPolicy string
	// SuccessfulJobsHistoryLimit is how many successful jobs to keep
	SuccessfulJobsHistoryLimit *int32
	// FailedJobsHistoryLimit is how many failed jobs to keep
	FailedJobsHistoryLimit *int32
	// StartingDeadlineSeconds is the deadline for starting the job
	StartingDeadlineSeconds *int64
	// RestartPolicy is the restart policy (Never or OnFailure)
	RestartPolicy string
}

// PVCSpec defines persistent volume claim creation/update parameters
type PVCSpec struct {
	// Name of the PVC
	Name string
	// Labels to apply to the PVC
	Labels map[string]string
	// Annotations to apply to the PVC
	Annotations map[string]string
	// StorageClassName is the storage class
	StorageClassName string
	// AccessModes are the access modes (ReadWriteOnce, ReadOnlyMany, ReadWriteMany)
	AccessModes []string
	// StorageSize is the requested storage size (e.g., "10Gi")
	StorageSize string
	// VolumeName is the name of a specific PV to bind to
	VolumeName string
	// VolumeMode is Filesystem or Block (default: Filesystem)
	VolumeMode string
}

// HPASpec defines horizontal pod autoscaler creation/update parameters
type HPASpec struct {
	// Name of the HPA
	Name string
	// Labels to apply to the HPA
	Labels map[string]string
	// Annotations to apply to the HPA
	Annotations map[string]string
	// TargetKind is the kind of the scale target (Deployment, StatefulSet, etc.)
	TargetKind string
	// TargetName is the name of the scale target
	TargetName string
	// MinReplicas is the minimum number of replicas (default: 1)
	MinReplicas int32
	// MaxReplicas is the maximum number of replicas (required)
	MaxReplicas int32
	// TargetCPUUtilization is the target CPU utilization percentage
	TargetCPUUtilization *int32
	// TargetMemoryUtilization is the target memory utilization percentage
	TargetMemoryUtilization *int32
}

// ServiceSpec defines service creation/update parameters
type ServiceSpec struct {
	// Name of the service
	Name string
	// Type is the service type (ClusterIP, NodePort, LoadBalancer, ExternalName)
	Type string
	// Selector labels for pod selection
	Selector map[string]string
	// Ports is the list of service ports
	Ports []ServicePortSpec
	// Labels to apply to the service
	Labels map[string]string
	// Annotations to apply to the service
	Annotations map[string]string
	// ClusterIP is the cluster IP (optional, "None" for headless)
	ClusterIP string
	// ExternalName is the external name for ExternalName type services
	ExternalName string
}

// ServicePortSpec defines a service port for creation/update
type ServicePortSpec struct {
	// Name of the port (required if multiple ports)
	Name string
	// Protocol (TCP, UDP, SCTP) - defaults to TCP
	Protocol string
	// Port is the service port number
	Port int32
	// TargetPort is the target port on the pod (defaults to Port)
	TargetPort int32
	// NodePort for NodePort/LoadBalancer services (optional, auto-assigned if 0)
	NodePort int32
}

// ConfigMapInfo holds configmap-specific information
type ConfigMapInfo struct {
	ResourceInfo
	// Data holds the key-value data pairs
	Data map[string]string
	// BinaryData holds the binary data (base64 encoded in YAML)
	BinaryData map[string][]byte
}

// ConfigMapSpec defines configmap creation/update parameters
type ConfigMapSpec struct {
	// Name of the configmap
	Name string
	// Labels to apply to the configmap
	Labels map[string]string
	// Annotations to apply to the configmap
	Annotations map[string]string
	// Data holds the key-value data pairs
	Data map[string]string
	// BinaryData holds the binary data
	BinaryData map[string][]byte
}

// SecretInfo holds secret-specific information
type SecretInfo struct {
	ResourceInfo
	// Type is the secret type (Opaque, kubernetes.io/tls, etc.)
	Type string
	// Data holds the secret data (base64 decoded)
	Data map[string][]byte
}

// SecretSpec defines secret creation/update parameters
type SecretSpec struct {
	// Name of the secret
	Name string
	// Type is the secret type (Opaque, kubernetes.io/tls, kubernetes.io/dockerconfigjson, etc.)
	// Default: "Opaque"
	Type string
	// Labels to apply to the secret
	Labels map[string]string
	// Annotations to apply to the secret
	Annotations map[string]string
	// Data holds the secret data (already base64 encoded or raw bytes)
	Data map[string][]byte
	// StringData holds secret data as strings (converted to Data on creation)
	StringData map[string]string
}

// IngressInfo holds ingress-specific information
type IngressInfo struct {
	ResourceInfo
	// IngressClassName is the name of the IngressClass
	IngressClassName string
	// Rules contains the list of host rules
	Rules []IngressRule
	// TLS contains TLS configuration
	TLS []IngressTLS
	// DefaultBackend is the default backend for the ingress
	DefaultBackend *IngressBackend
	// LoadBalancer contains the current status of the load-balancer
	LoadBalancerIngress []string
}

// IngressRule represents an ingress rule
type IngressRule struct {
	// Host is the fully qualified domain name of a network host
	Host string
	// Paths contains the list of paths for the host
	Paths []IngressPath
}

// IngressPath represents a path and backend for an ingress rule
type IngressPath struct {
	// Path is the path to match
	Path string
	// PathType is the type of path matching (Exact, Prefix, ImplementationSpecific)
	PathType string
	// Backend is the backend to route traffic to
	Backend IngressBackend
}

// IngressBackend represents a backend for an ingress
type IngressBackend struct {
	// ServiceName is the name of the service
	ServiceName string
	// ServicePort is the port of the service
	ServicePort int32
}

// IngressTLS represents TLS configuration for an ingress
type IngressTLS struct {
	// Hosts are the hosts included in the TLS certificate
	Hosts []string
	// SecretName is the name of the secret containing the TLS certificate
	SecretName string
}

// IngressSpec defines ingress creation/update parameters
type IngressSpec struct {
	// Name of the ingress
	Name string
	// IngressClassName is the name of the IngressClass to use
	IngressClassName string
	// Labels to apply to the ingress
	Labels map[string]string
	// Annotations to apply to the ingress
	Annotations map[string]string
	// Rules contains the list of host rules
	Rules []IngressRule
	// TLS contains TLS configuration
	TLS []IngressTLS
	// DefaultBackend is the default backend for unmatched requests
	DefaultBackend *IngressBackend
}
