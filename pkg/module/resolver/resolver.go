package resolver

import (
	"fmt"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/module/manifest"
)

// ModuleResolver implements the Resolver interface
type ModuleResolver struct {
	// Registry is the registry client for fetching module info
	Registry RegistryClient

	// Cache is the module cache
	Cache *ModuleCache

	// ConstraintParser parses version constraints
	ConstraintParser ConstraintParser

	// VersionSelector selects the best version
	VersionSelector VersionSelector

	// ConflictResolver resolves version conflicts
	ConflictResolver ConflictResolver

	// MaxDepth is the maximum dependency depth (0 = unlimited)
	MaxDepth int

	// AllowPrerelease indicates whether to allow prerelease versions
	AllowPrerelease bool
}

// NewModuleResolver creates a new module resolver
func NewModuleResolver(registry RegistryClient, cacheDir string) (*ModuleResolver, error) {
	cacheConfig := CacheConfig{
		Dir:      cacheDir,
		MaxSize:  0, // unlimited
		MaxAge:   0, // unlimited
		Readonly: false,
	}

	cache, err := NewModuleCache(cacheConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache: %w", err)
	}

	return &ModuleResolver{
		Registry:         registry,
		Cache:            cache,
		ConstraintParser: &DefaultConstraintParser{},
		VersionSelector:  &DefaultVersionSelector{},
		ConflictResolver: NewMVSConflictResolver(),
		MaxDepth:         0,
		AllowPrerelease:  false,
	}, nil
}

// Resolve resolves dependencies for a module
func (r *ModuleResolver) Resolve(req *ResolutionRequest) (*ResolutionResult, error) {
	start := time.Now()
	result := &ResolutionResult{
		RootModule: ModuleReference{
			Name:    req.RootModule,
			Version: req.RootVersion,
		},
		Errors:   []error{},
		Warnings: []string{},
	}

	// Use lock file if requested and present
	if req.UseLockFile && req.LockFile != nil {
		// Validate lock file first
		if err := r.ValidateLockFile(req.LockFile); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Lock file validation failed: %v", err))
			req.UseLockFile = false // Fall back to fresh resolution
		}
	}

	// If using lock file, construct result from it
	if req.UseLockFile && req.LockFile != nil {
		nodes, err := r.buildGraphFromLockFile(req.LockFile)
		if err != nil {
			return nil, fmt.Errorf("failed to build graph from lock file: %w", err)
		}
		result.Resolved = nodes
		result.LockFile = req.LockFile
		result.Duration = time.Since(start)
		return result, nil
	}

	// Fresh resolution: build dependency graph
	graph := NewDependencyGraph()
	visited := make(map[string]bool)

	// Determine max depth
	maxDepth := req.MaxDepth
	if maxDepth == 0 {
		maxDepth = r.MaxDepth
	}

	// Resolve dependencies recursively
	rootNode := &DependencyNode{
		Module: result.RootModule,
		Depth:  0,
	}

	if err := r.resolveDependencies(req.Manifest, rootNode, graph, visited, maxDepth); err != nil {
		result.Errors = append(result.Errors, err)
		result.Duration = time.Since(start)
		return result, err
	}

	// Check for cycles
	hasCycle, cyclePath := graph.HasCycle()
	if hasCycle {
		err := &CircularDependencyError{
			Cycle: cyclePath,
		}
		result.Errors = append(result.Errors, err)
		result.Duration = time.Since(start)
		return result, err
	}

	// Topologically sort the graph
	sorted, err := graph.TopologicalSort()
	if err != nil {
		result.Errors = append(result.Errors, err)
		result.Duration = time.Since(start)
		return result, err
	}

	result.Resolved = sorted

	// Generate lock file
	lockFile, err := r.generateLockFile(result.RootModule, sorted)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to generate lock file: %v", err))
	} else {
		result.LockFile = lockFile
	}

	result.Duration = time.Since(start)
	return result, nil
}

// ResolveFromManifest resolves dependencies from a manifest
func (r *ModuleResolver) ResolveFromManifest(m *manifest.Manifest) (*ResolutionResult, error) {
	req := &ResolutionRequest{
		RootModule:      m.Name,
		RootVersion:     m.Version,
		Manifest:        m,
		UseLockFile:     false,
		AllowPrerelease: r.AllowPrerelease,
		MaxDepth:        r.MaxDepth,
	}
	return r.Resolve(req)
}

// Update updates dependencies to the latest compatible versions
func (r *ModuleResolver) Update(req *ResolutionRequest) (*ResolutionResult, error) {
	// Force fresh resolution (ignore lock file)
	req.UseLockFile = false

	// Resolve with latest versions
	return r.Resolve(req)
}

// ValidateLockFile validates that a lock file is still valid
func (r *ModuleResolver) ValidateLockFile(lockFile *manifest.LockFile) error {
	if lockFile == nil {
		return fmt.Errorf("lock file is nil")
	}

	// Check version (must be 1)
	if lockFile.SchemaVersion != 1 {
		return fmt.Errorf("unsupported lock file version: %d", lockFile.SchemaVersion)
	}

	// Validate each locked module
	for name, locked := range lockFile.Modules {
		// Check if module still exists in registry
		versions, err := r.Registry.ListVersions(name)
		if err != nil {
			return fmt.Errorf("failed to list versions for %s: %w", name, err)
		}

		// Check if the locked version is still available
		found := false
		for _, v := range versions {
			if v == locked.Version {
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("locked version %s@%s not found in registry", name, locked.Version)
		}

		// Get module info and verify hash
		info, err := r.Registry.GetModuleInfo(name, locked.Version)
		if err != nil {
			return fmt.Errorf("failed to get info for %s@%s: %w", name, locked.Version, err)
		}

		if info.Hash != locked.Hash {
			return fmt.Errorf("hash mismatch for %s@%s: expected %s, got %s",
				name, locked.Version, locked.Hash, info.Hash)
		}
	}

	return nil
}

// resolveDependencies recursively resolves dependencies
func (r *ModuleResolver) resolveDependencies(
	m *manifest.Manifest,
	node *DependencyNode,
	graph DependencyGraph,
	visited map[string]bool,
	maxDepth int,
) error {
	// Check depth limit
	if maxDepth > 0 && node.Depth >= maxDepth {
		return nil
	}

	// Mark as visited
	visited[node.Module.Name] = true

	// Add node to graph
	if err := graph.AddNode(node); err != nil {
		return err
	}

	// Process dependencies (map[string]string format in manifest)
	for depName, depVersion := range m.Dependencies {
		depKey := fmt.Sprintf("%s@%s", depName, depVersion)

		// Skip if already visited
		if visited[depKey] {
			continue
		}

		// Parse constraint
		constraint, err := r.ConstraintParser.Parse(depVersion)
		if err != nil {
			return fmt.Errorf("invalid constraint %s for %s: %w", depVersion, depName, err)
		}

		// Get available versions
		versions, err := r.Registry.ListVersions(depName)
		if err != nil {
			return fmt.Errorf("failed to list versions for %s: %w", depName, err)
		}

		// Filter out prereleases if not allowed
		if !r.AllowPrerelease {
			versions = filterPrereleases(versions)
		}

		// Select best version
		selectedVersion, err := r.VersionSelector.Select(constraint, versions)
		if err != nil {
			return fmt.Errorf("failed to select version for %s: %w", depName, err)
		}

		// Get manifest for selected version
		depManifest, err := r.Registry.GetModuleManifest(depName, selectedVersion)
		if err != nil {
			return fmt.Errorf("failed to get manifest for %s@%s: %w", depName, selectedVersion, err)
		}

		// Create dependency node
		depNode := &DependencyNode{
			Module: ModuleReference{
				Name:     depName,
				Version:  depVersion,
				Resolved: selectedVersion,
			},
			Parent: node,
			Depth:  node.Depth + 1,
		}

		// Add to parent's dependencies
		node.Dependencies = append(node.Dependencies, depNode)

		// Recursively resolve
		if err := r.resolveDependencies(depManifest, depNode, graph, visited, maxDepth); err != nil {
			return err
		}
	}

	return nil
}

// generateLockFile generates a lock file from resolved dependencies
func (r *ModuleResolver) generateLockFile(root ModuleReference, resolved []*DependencyNode) (*manifest.LockFile, error) {
	lockFile := &manifest.LockFile{
		SchemaVersion: 1,
		Modules:       make(map[string]manifest.LockedModule),
	}

	// Add all resolved modules to lock file (skip root module)
	for _, node := range resolved {
		// Skip the root module (it's not a dependency from the registry)
		if node.Module.Name == root.Name {
			continue
		}

		// Get module info for hash
		info, err := r.Registry.GetModuleInfo(node.Module.Name, node.Module.Resolved)
		if err != nil {
			return nil, fmt.Errorf("failed to get info for %s@%s: %w",
				node.Module.Name, node.Module.Resolved, err)
		}

		locked := manifest.LockedModule{
			Version: node.Module.Resolved,
			Hash:    info.Hash,
		}

		lockFile.Modules[node.Module.Name] = locked
	}

	return lockFile, nil
}

// buildGraphFromLockFile builds a dependency graph from a lock file
func (r *ModuleResolver) buildGraphFromLockFile(lockFile *manifest.LockFile) ([]*DependencyNode, error) {
	nodes := []*DependencyNode{}

	// Create nodes for all locked modules
	for name, locked := range lockFile.Modules {
		node := &DependencyNode{
			Module: ModuleReference{
				Name:     name,
				Version:  locked.Version,
				Resolved: locked.Version,
				Hash:     locked.Hash,
			},
			Depth: 0, // Will be updated when building tree
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// filterPrereleases filters out prerelease versions
func filterPrereleases(versions []string) []string {
	filtered := []string{}
	for _, v := range versions {
		ver, err := ParseVersion(v)
		if err != nil {
			continue
		}
		if ver.Prerelease == "" {
			filtered = append(filtered, v)
		}
	}
	return filtered
}
