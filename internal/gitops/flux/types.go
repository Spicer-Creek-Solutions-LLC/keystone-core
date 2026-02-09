// Package flux provides Flux CD integration for GitOps deployment verification
// and resource status monitoring.
package flux

import "time"

// Config represents Flux client configuration
type Config struct {
	// Kubeconfig path (optional, uses in-cluster config if empty)
	Kubeconfig string

	// Context to use from kubeconfig (optional)
	Context string

	// Namespace to watch (empty means all namespaces)
	Namespace string
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		Namespace: "flux-system",
	}
}

// ResourceKind represents the type of Flux resource
type ResourceKind string

// KindKustomization constants define the error kinds.
const (
	KindKustomization  ResourceKind = "Kustomization"
	KindHelmRelease    ResourceKind = "HelmRelease"
	KindGitRepository  ResourceKind = "GitRepository"
	KindHelmRepository ResourceKind = "HelmRepository"
)

// ResourceStatus represents the status of a Flux resource
type ResourceStatus struct {
	// Kind is the resource kind
	Kind ResourceKind

	// Name is the resource name
	Name string

	// Namespace is the resource namespace
	Namespace string

	// Ready indicates if the resource is ready
	Ready bool

	// Suspended indicates if reconciliation is suspended
	Suspended bool

	// Revision is the current revision
	Revision string

	// Message contains status message
	Message string

	// Conditions are the resource conditions
	Conditions []Condition

	// LastReconcileTime is when the resource was last reconciled
	LastReconcileTime time.Time

	// ObservedAt is when the status was observed
	ObservedAt time.Time
}

// Condition represents a resource condition
type Condition struct {
	Type    string
	Status  string
	Reason  string
	Message string
}

// SuspendRequest represents a request to suspend reconciliation
type SuspendRequest struct {
	// Kind of resource
	Kind ResourceKind

	// Name of resource
	Name string

	// Namespace (optional)
	Namespace string

	// Suspend flag
	Suspend bool
}

// ReconcileRequest represents a request to trigger reconciliation
type ReconcileRequest struct {
	// Kind of resource
	Kind ResourceKind

	// Name of resource
	Name string

	// Namespace (optional)
	Namespace string
}
