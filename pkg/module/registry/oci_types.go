// Package registry provides module registry clients for both HTTP and OCI protocols
package registry

import (
	"time"
)

// OCI Distribution Specification media types
const (
	// OCIManifestMediaType is the media type for OCI image manifests
	OCIManifestMediaType = "application/vnd.oci.image.manifest.v1+json"
	// OCIConfigMediaType is the media type for OCI image configs
	OCIConfigMediaType = "application/vnd.oci.image.config.v1+json"
	// OCILayerMediaType is the media type for OCI layer blobs
	OCILayerMediaType = "application/vnd.oci.image.layer.v1.tar+gzip"
	// KscoreModuleMediaType is the media type for Keystone Core module ZIPs
	KscoreModuleMediaType = "application/vnd.kscore.module.v1+zip"
	// KscoreManifestMediaType is the media type for Keystone Core module manifests
	KscoreManifestMediaType = "application/vnd.kscore.module.manifest.v1+yaml"
	// KscoreSignatureMediaType is the media type for Keystone Core module signatures
	KscoreSignatureMediaType = "application/vnd.kscore.module.signature.v1"
)

// OCIManifest represents an OCI image manifest
type OCIManifest struct {
	// SchemaVersion is the image manifest schema version (always 2)
	SchemaVersion int `json:"schemaVersion"`
	// MediaType is the manifest media type
	MediaType string `json:"mediaType,omitempty"`
	// ArtifactType is the artifact type for OCI artifacts
	ArtifactType string `json:"artifactType,omitempty"`
	// Config is the config descriptor
	Config OCIDescriptor `json:"config"`
	// Layers are the layer descriptors
	Layers []OCIDescriptor `json:"layers"`
	// Annotations are optional manifest annotations
	Annotations map[string]string `json:"annotations,omitempty"`
}

// OCIDescriptor describes a content-addressable blob
type OCIDescriptor struct {
	// MediaType is the media type of the referenced content
	MediaType string `json:"mediaType"`
	// Digest is the digest of the content (sha256:...)
	Digest string `json:"digest"`
	// Size is the size in bytes of the content
	Size int64 `json:"size"`
	// URLs are optional URLs for downloading the content
	URLs []string `json:"urls,omitempty"`
	// Annotations are optional annotations
	Annotations map[string]string `json:"annotations,omitempty"`
}

// OCIConfig represents the OCI image config for a Keystone Core module
type OCIConfig struct {
	// Created is when the module was created
	Created time.Time `json:"created,omitempty"`
	// Author is the module author
	Author string `json:"author,omitempty"`
	// Architecture (always "any" for Keystone modules)
	Architecture string `json:"architecture,omitempty"`
	// OS (always "any" for Keystone modules)
	OS string `json:"os,omitempty"`
	// Config contains module-specific config
	Config OCIModuleConfig `json:"config,omitempty"`
}

// OCIModuleConfig contains Keystone Core module metadata
type OCIModuleConfig struct {
	// Name is the module name
	Name string `json:"name"`
	// Version is the module version
	Version string `json:"version"`
	// Description is the module description
	Description string `json:"description,omitempty"`
	// Capabilities are the required capabilities
	Capabilities []string `json:"capabilities,omitempty"`
	// Dependencies are the module dependencies
	Dependencies map[string]string `json:"dependencies,omitempty"`
}

// OCITagsList is the response from /v2/<name>/tags/list
type OCITagsList struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// OCIRegistryConfig holds OCI registry configuration
type OCIRegistryConfig struct {
	// Registry is the registry hostname (e.g., "ghcr.io", "docker.io")
	Registry string
	// Namespace is the namespace/organization (e.g., "myorg")
	Namespace string
	// Auth is optional authentication configuration
	Auth *AuthConfig
	// Timeout is the HTTP timeout
	Timeout time.Duration
	// InsecureSkipVerify skips TLS verification
	InsecureSkipVerify bool
	// PlainHTTP uses HTTP instead of HTTPS
	PlainHTTP bool
}

// DefaultOCIRegistryConfig returns a config with sensible defaults
func DefaultOCIRegistryConfig(registry, namespace string) *OCIRegistryConfig {
	return &OCIRegistryConfig{
		Registry:  registry,
		Namespace: namespace,
		Timeout:   60 * time.Second,
	}
}

// OCIPushResult contains the result of pushing a module
type OCIPushResult struct {
	// Reference is the full OCI reference (registry/namespace/name:tag)
	Reference string
	// Digest is the manifest digest
	Digest string
	// ModuleName is the module name
	ModuleName string
	// Version is the module version
	Version string
	// Size is the total size in bytes
	Size int64
	// PushedAt is when the module was pushed
	PushedAt time.Time
}

// OCIPullResult contains the result of pulling a module
type OCIPullResult struct {
	// Reference is the full OCI reference
	Reference string
	// Digest is the manifest digest
	Digest string
	// ModulePath is the path to the downloaded module ZIP
	ModulePath string
	// ManifestPath is the path to the downloaded manifest YAML
	ManifestPath string
	// SignaturePath is the path to the signature (if present)
	SignaturePath string
	// Size is the total size in bytes
	Size int64
	// PulledAt is when the module was pulled
	PulledAt time.Time
}

// OCIRegistry is the interface for OCI registry operations
type OCIRegistry interface {
	// Ping checks if the registry is accessible
	Ping() error
	// ListTags lists all tags for a module
	ListTags(moduleName string) ([]string, error)
	// Push pushes a module to the registry
	Push(req *OCIPushRequest) (*OCIPushResult, error)
	// Pull pulls a module from the registry
	Pull(moduleName, version, destDir string) (*OCIPullResult, error)
	// Delete deletes a module version from the registry
	Delete(moduleName, version string) error
	// GetManifest retrieves the OCI manifest for a module
	GetManifest(moduleName, reference string) (*OCIManifest, error)
}

// OCIPushRequest contains data for pushing a module
type OCIPushRequest struct {
	// ModulePath is the path to the module ZIP file
	ModulePath string
	// ManifestPath is the path to the module.yaml manifest
	ManifestPath string
	// SignaturePath is the optional path to the signature file
	SignaturePath string
	// ModuleName is the module name (vendor/name)
	ModuleName string
	// Version is the module version (used as tag)
	Version string
	// Annotations are optional OCI annotations
	Annotations map[string]string
}
