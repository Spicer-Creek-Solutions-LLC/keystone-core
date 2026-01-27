// Package k8s provides Kubernetes integration for secret injection.
// It includes sidecar injector, init container support, secret synchronization,
// and a mutating admission webhook for automatic secret injection.
package k8s

import (
	"time"
)

// InjectionMode determines how secrets are injected into pods.
type InjectionMode string

const (
	// InjectionModeSidecar injects a sidecar container that continuously syncs secrets.
	InjectionModeSidecar InjectionMode = "sidecar"

	// InjectionModeInit injects an init container that fetches secrets before pod start.
	InjectionModeInit InjectionMode = "init"

	// InjectionModeBoth injects both init and sidecar containers.
	InjectionModeBoth InjectionMode = "both"
)

// SecretType represents the type of secret to inject.
type SecretType string

const (
	// SecretTypeFile writes secrets to files.
	SecretTypeFile SecretType = "file"

	// SecretTypeEnv injects secrets as environment variables.
	SecretTypeEnv SecretType = "env"

	// SecretTypeVolume mounts secrets as a volume.
	SecretTypeVolume SecretType = "volume"
)

// InjectorConfig configures the secret injector.
type InjectorConfig struct {
	// Mode is the injection mode (sidecar, init, or both).
	Mode InjectionMode `json:"mode" yaml:"mode"`

	// Image is the injector container image.
	Image string `json:"image" yaml:"image"`

	// ImagePullPolicy is the image pull policy.
	ImagePullPolicy string `json:"image_pull_policy" yaml:"image_pull_policy"`

	// ServiceAccountName is the service account for the injector.
	ServiceAccountName string `json:"service_account_name" yaml:"service_account_name"`

	// BrokerAddress is the address of the secrets broker.
	BrokerAddress string `json:"broker_address" yaml:"broker_address"`

	// RefreshInterval is how often to refresh secrets (for sidecar mode).
	RefreshInterval time.Duration `json:"refresh_interval" yaml:"refresh_interval"`

	// SecretVolumePath is the path where secrets are mounted.
	SecretVolumePath string `json:"secret_volume_path" yaml:"secret_volume_path"`

	// Resources specifies container resource requirements.
	Resources *ResourceRequirements `json:"resources,omitempty" yaml:"resources,omitempty"`

	// Annotations to add to injected pods.
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`

	// Labels to add to injected containers.
	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

// ResourceRequirements defines container resource limits and requests.
type ResourceRequirements struct {
	Limits   ResourceList `json:"limits,omitempty" yaml:"limits,omitempty"`
	Requests ResourceList `json:"requests,omitempty" yaml:"requests,omitempty"`
}

// ResourceList is a map of resource name to quantity.
type ResourceList map[string]string

// DefaultInjectorConfig returns a default injector configuration.
func DefaultInjectorConfig() *InjectorConfig {
	return &InjectorConfig{
		Mode:             InjectionModeSidecar,
		Image:            "keystone-core/secret-injector:latest",
		ImagePullPolicy:  "IfNotPresent",
		RefreshInterval:  30 * time.Second,
		SecretVolumePath: "/secrets",
		Resources: &ResourceRequirements{
			Limits: ResourceList{
				"cpu":    "100m",
				"memory": "64Mi",
			},
			Requests: ResourceList{
				"cpu":    "10m",
				"memory": "16Mi",
			},
		},
	}
}

// SecretInjection defines a secret to be injected into a pod.
type SecretInjection struct {
	// Name is a unique name for this injection.
	Name string `json:"name" yaml:"name"`

	// SecretPath is the path to the secret in the backend.
	SecretPath string `json:"secret_path" yaml:"secret_path"`

	// SecretKey is the specific key within the secret (optional).
	SecretKey string `json:"secret_key,omitempty" yaml:"secret_key,omitempty"`

	// Type is how the secret should be injected.
	Type SecretType `json:"type" yaml:"type"`

	// FilePath is the file path for file-type injection.
	FilePath string `json:"file_path,omitempty" yaml:"file_path,omitempty"`

	// FileMode is the file permissions for file-type injection.
	FileMode string `json:"file_mode,omitempty" yaml:"file_mode,omitempty"`

	// EnvVar is the environment variable name for env-type injection.
	EnvVar string `json:"env_var,omitempty" yaml:"env_var,omitempty"`

	// Template is an optional template for rendering the secret.
	Template string `json:"template,omitempty" yaml:"template,omitempty"`
}

// PodInjectionSpec defines the injection specification for a pod.
type PodInjectionSpec struct {
	// Enabled indicates if injection is enabled for this pod.
	Enabled bool `json:"enabled" yaml:"enabled"`

	// Mode overrides the default injection mode.
	Mode InjectionMode `json:"mode,omitempty" yaml:"mode,omitempty"`

	// Secrets is the list of secrets to inject.
	Secrets []SecretInjection `json:"secrets" yaml:"secrets"`

	// ServiceAccountAuth uses the pod's service account for authentication.
	ServiceAccountAuth bool `json:"service_account_auth" yaml:"service_account_auth"`

	// InitFirst ensures init container runs before other init containers.
	InitFirst bool `json:"init_first" yaml:"init_first"`

	// ShareProcessNamespace shares process namespace with sidecar.
	ShareProcessNamespace bool `json:"share_process_namespace" yaml:"share_process_namespace"`
}

// WebhookConfig configures the mutating admission webhook.
type WebhookConfig struct {
	// Enabled indicates if the webhook is enabled.
	Enabled bool `json:"enabled" yaml:"enabled"`

	// Port is the webhook server port.
	Port int `json:"port" yaml:"port"`

	// CertFile is the path to the TLS certificate.
	CertFile string `json:"cert_file" yaml:"cert_file"`

	// KeyFile is the path to the TLS private key.
	KeyFile string `json:"key_file" yaml:"key_file"`

	// CABundle is the CA bundle for webhook validation.
	CABundle []byte `json:"ca_bundle,omitempty" yaml:"ca_bundle,omitempty"`

	// NamespaceSelector selects namespaces for injection.
	NamespaceSelector *LabelSelector `json:"namespace_selector,omitempty" yaml:"namespace_selector,omitempty"`

	// ObjectSelector selects pods for injection.
	ObjectSelector *LabelSelector `json:"object_selector,omitempty" yaml:"object_selector,omitempty"`

	// FailurePolicy determines behavior when webhook fails.
	FailurePolicy string `json:"failure_policy" yaml:"failure_policy"`

	// InjectorConfig is the default injector configuration.
	InjectorConfig *InjectorConfig `json:"injector_config" yaml:"injector_config"`
}

// LabelSelector selects resources by labels.
type LabelSelector struct {
	MatchLabels      map[string]string          `json:"match_labels,omitempty" yaml:"match_labels,omitempty"`
	MatchExpressions []LabelSelectorRequirement `json:"match_expressions,omitempty" yaml:"match_expressions,omitempty"`
}

// LabelSelectorRequirement is a selector requirement.
type LabelSelectorRequirement struct {
	Key      string   `json:"key" yaml:"key"`
	Operator string   `json:"operator" yaml:"operator"`
	Values   []string `json:"values,omitempty" yaml:"values,omitempty"`
}

// DefaultWebhookConfig returns a default webhook configuration.
func DefaultWebhookConfig() *WebhookConfig {
	return &WebhookConfig{
		Enabled:        true,
		Port:           8443,
		FailurePolicy:  "Ignore",
		InjectorConfig: DefaultInjectorConfig(),
	}
}

// SyncConfig configures Kubernetes secret synchronization.
type SyncConfig struct {
	// Enabled indicates if sync is enabled.
	Enabled bool `json:"enabled" yaml:"enabled"`

	// SyncInterval is how often to sync secrets.
	SyncInterval time.Duration `json:"sync_interval" yaml:"sync_interval"`

	// Namespace is the namespace to sync secrets to (empty for all namespaces).
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`

	// Secrets is the list of secrets to sync.
	Secrets []SyncSecretSpec `json:"secrets" yaml:"secrets"`

	// DeleteOrphans removes K8s secrets that are no longer in the source.
	DeleteOrphans bool `json:"delete_orphans" yaml:"delete_orphans"`
}

// SyncSecretSpec defines a secret to sync to Kubernetes.
type SyncSecretSpec struct {
	// SourcePath is the path in the secrets backend.
	SourcePath string `json:"source_path" yaml:"source_path"`

	// DestName is the Kubernetes secret name.
	DestName string `json:"dest_name" yaml:"dest_name"`

	// DestNamespace is the Kubernetes namespace (empty uses default).
	DestNamespace string `json:"dest_namespace,omitempty" yaml:"dest_namespace,omitempty"`

	// Type is the Kubernetes secret type (default: Opaque).
	Type string `json:"type,omitempty" yaml:"type,omitempty"`

	// Labels to add to the synced secret.
	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`

	// Annotations to add to the synced secret.
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`

	// KeyMapping maps source keys to destination keys.
	KeyMapping map[string]string `json:"key_mapping,omitempty" yaml:"key_mapping,omitempty"`
}

// DefaultSyncConfig returns a default sync configuration.
func DefaultSyncConfig() *SyncConfig {
	return &SyncConfig{
		Enabled:       true,
		SyncInterval:  60 * time.Second,
		DeleteOrphans: false,
	}
}

// Annotations used for injection configuration.
const (
	// AnnotationInject enables/disables injection.
	AnnotationInject = "secrets.keystone.io/inject"

	// AnnotationMode sets the injection mode.
	AnnotationMode = "secrets.keystone.io/mode"

	// AnnotationSecrets specifies secrets to inject (JSON).
	AnnotationSecrets = "secrets.keystone.io/secrets"

	// AnnotationStatus indicates injection status.
	AnnotationStatus = "secrets.keystone.io/status"

	// AnnotationServiceAccountAuth enables service account auth.
	AnnotationServiceAccountAuth = "secrets.keystone.io/service-account-auth"

	// LabelInjected indicates a pod has been injected.
	LabelInjected = "secrets.keystone.io/injected"
)
