package resolver

import (
	"time"

	"github.com/shawnbutts/keystone-core/pkg/module/manifest"
)

// ModuleReference represents a reference to a module
type ModuleReference struct {
	// Name is the module name (e.g., "vendor/package")
	Name string

	// Version is the requested version or constraint
	Version string

	// Resolved is the actual resolved version (after constraint solving)
	Resolved string

	// Hash is the module hash (from SumDB or lockfile)
	Hash string
}

// DependencyNode represents a node in the dependency graph
type DependencyNode struct {
	// Module is the module reference
	Module ModuleReference

	// Dependencies are the direct dependencies of this module
	Dependencies []*DependencyNode

	// Parent is the module that depends on this one (nil for root)
	Parent *DependencyNode

	// Depth is the depth in the dependency tree (0 for root)
	Depth int
}

// ResolutionRequest represents a request to resolve dependencies
type ResolutionRequest struct {
	// RootModule is the module whose dependencies we're resolving
	RootModule string

	// RootVersion is the version of the root module
	RootVersion string

	// Manifest is the parsed module manifest
	Manifest *manifest.Manifest

	// LockFile is an optional existing lock file
	LockFile *manifest.LockFile

	// UseLockFile indicates whether to use the lock file if present
	UseLockFile bool

	// AllowPrerelease indicates whether to allow prerelease versions
	AllowPrerelease bool

	// MaxDepth is the maximum dependency depth (0 = unlimited)
	MaxDepth int
}

// ResolutionResult represents the result of dependency resolution
type ResolutionResult struct {
	// RootModule is the root module
	RootModule ModuleReference

	// Resolved is the complete resolved dependency graph
	Resolved []*DependencyNode

	// LockFile is the generated lock file
	LockFile *manifest.LockFile

	// Duration is how long resolution took
	Duration time.Duration

	// Errors are any errors encountered during resolution
	Errors []error

	// Warnings are any warnings generated during resolution
	Warnings []string
}

// RegistryClient defines the interface for querying module registries
type RegistryClient interface {
	// ListVersions returns all available versions for a module
	ListVersions(moduleName string) ([]string, error)

	// GetModuleInfo returns metadata for a specific version
	GetModuleInfo(moduleName, version string) (*ModuleInfo, error)

	// GetModuleManifest returns the manifest for a specific version
	GetModuleManifest(moduleName, version string) (*manifest.Manifest, error)

	// DownloadModule downloads a module to the specified path
	DownloadModule(moduleName, version, destPath string) error
}

// ModuleInfo represents metadata about a module version
type ModuleInfo struct {
	// Name is the module name
	Name string

	// Version is the module version
	Version string

	// Hash is the module hash (from SumDB)
	Hash string

	// PublishedAt is when the module was published
	PublishedAt time.Time

	// Description is the module description
	Description string

	// Dependencies are the module's dependencies (name -> version constraint)
	Dependencies map[string]string

	// Size is the module size in bytes
	Size int64
}

// CacheConfig defines configuration for the module cache
type CacheConfig struct {
	// Dir is the cache directory path
	Dir string

	// MaxSize is the maximum cache size in bytes (0 = unlimited)
	MaxSize int64

	// MaxAge is the maximum age for cached modules (0 = unlimited)
	MaxAge time.Duration

	// Readonly indicates if the cache is read-only
	Readonly bool
}

// CacheEntry represents a cached module
type CacheEntry struct {
	// Module is the module reference
	Module ModuleReference

	// Path is the local path to the cached module
	Path string

	// CachedAt is when the module was cached
	CachedAt time.Time

	// Size is the module size in bytes
	Size int64

	// Verified indicates if the module has been verified
	Verified bool
}

// Resolver defines the interface for module dependency resolution
type Resolver interface {
	// Resolve resolves dependencies for a module
	Resolve(req *ResolutionRequest) (*ResolutionResult, error)

	// ResolveFromManifest resolves dependencies from a manifest
	ResolveFromManifest(manifest *manifest.Manifest) (*ResolutionResult, error)

	// Update updates dependencies to the latest compatible versions
	Update(req *ResolutionRequest) (*ResolutionResult, error)

	// ValidateLockFile validates that a lock file is still valid
	ValidateLockFile(lockFile *manifest.LockFile) error
}

// VersionConstraint represents a version constraint
type VersionConstraint interface {
	// Matches returns true if the version satisfies the constraint
	Matches(version string) bool

	// String returns the constraint as a string
	String() string

	// IsExact returns true if this is an exact version (not a range)
	IsExact() bool
}

// ConstraintParser parses version constraints
type ConstraintParser interface {
	// Parse parses a constraint string
	Parse(constraint string) (VersionConstraint, error)

	// ParseMultiple parses multiple constraints (AND'd together)
	ParseMultiple(constraints []string) (VersionConstraint, error)
}

// VersionSelector selects the best version from available versions
type VersionSelector interface {
	// Select selects the best version matching the constraint
	Select(constraint VersionConstraint, available []string) (string, error)

	// SelectHighest selects the highest available version
	SelectHighest(available []string) (string, error)

	// SelectLowest selects the lowest available version
	SelectLowest(available []string) (string, error)
}

// ConflictResolver resolves version conflicts using a strategy
type ConflictResolver interface {
	// Resolve resolves conflicts between multiple version constraints
	Resolve(moduleName string, constraints []VersionConstraint) (string, error)

	// Strategy returns the conflict resolution strategy name
	Strategy() string
}

// DependencyGraph represents a module dependency graph
type DependencyGraph interface {
	// AddNode adds a node to the graph
	AddNode(node *DependencyNode) error

	// GetNode returns a node by module name
	GetNode(moduleName string) (*DependencyNode, error)

	// GetAllNodes returns all nodes in the graph
	GetAllNodes() []*DependencyNode

	// HasCycle detects if the graph has a cycle
	HasCycle() (bool, []string)

	// TopologicalSort returns a topologically sorted list of modules
	TopologicalSort() ([]*DependencyNode, error)

	// Flatten returns a flattened list of all dependencies
	Flatten() []ModuleReference
}
