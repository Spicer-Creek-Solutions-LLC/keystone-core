// Package mirror implements mirror groups and geographic routing for file distribution.
package mirror

import (
	"fmt"
	"sync"
)

// Registry manages multiple mirror groups.
type Registry struct {
	groups   map[string]*MirrorGroup
	byPath   map[string]*MirrorGroup // Path prefix -> group
	byNS     map[string]*MirrorGroup // Namespace -> group
	defaultG *MirrorGroup
	mu       sync.RWMutex
}

// NewRegistry creates a new mirror registry.
func NewRegistry() *Registry {
	return &Registry{
		groups: make(map[string]*MirrorGroup),
		byPath: make(map[string]*MirrorGroup),
		byNS:   make(map[string]*MirrorGroup),
	}
}

// Register adds a mirror group to the registry.
func (r *Registry) Register(group *MirrorGroup) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.groups[group.ID()]; exists {
		return fmt.Errorf("mirror group %s already registered", group.ID())
	}

	r.groups[group.ID()] = group

	// Index by path prefix
	for _, prefix := range group.config.PathPrefixes {
		if existing, ok := r.byPath[prefix]; ok {
			return fmt.Errorf("path prefix %s already handled by group %s", prefix, existing.ID())
		}
		r.byPath[prefix] = group
	}

	// Index by namespace
	for _, ns := range group.config.Namespaces {
		if existing, ok := r.byNS[ns]; ok {
			return fmt.Errorf("namespace %s already handled by group %s", ns, existing.ID())
		}
		r.byNS[ns] = group
	}

	// Set as default if no paths/namespaces specified
	if len(group.config.PathPrefixes) == 0 && len(group.config.Namespaces) == 0 {
		if r.defaultG != nil {
			return fmt.Errorf("default mirror group already set to %s", r.defaultG.ID())
		}
		r.defaultG = group
	}

	return nil
}

// Unregister removes a mirror group from the registry.
func (r *Registry) Unregister(groupID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	group, ok := r.groups[groupID]
	if !ok {
		return fmt.Errorf("mirror group %s not found", groupID)
	}

	// Remove from path index
	for _, prefix := range group.config.PathPrefixes {
		delete(r.byPath, prefix)
	}

	// Remove from namespace index
	for _, ns := range group.config.Namespaces {
		delete(r.byNS, ns)
	}

	// Clear default if this was it
	if r.defaultG == group {
		r.defaultG = nil
	}

	delete(r.groups, groupID)
	return nil
}

// Get returns a mirror group by ID.
func (r *Registry) Get(groupID string) (*MirrorGroup, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.groups[groupID]
	return g, ok
}

// GetForPath returns the mirror group handling a specific path.
func (r *Registry) GetForPath(path string) *MirrorGroup {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Find longest matching prefix
	var best *MirrorGroup
	var bestLen int

	for prefix, group := range r.byPath {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			if len(prefix) > bestLen {
				best = group
				bestLen = len(prefix)
			}
		}
	}

	if best != nil {
		return best
	}

	return r.defaultG
}

// GetForNamespace returns the mirror group handling a specific namespace.
func (r *Registry) GetForNamespace(namespace string) *MirrorGroup {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Exact match
	if group, ok := r.byNS[namespace]; ok {
		return group
	}

	// Wildcard match
	if group, ok := r.byNS["*"]; ok {
		return group
	}

	return r.defaultG
}

// GetForRequest returns the best mirror group for a file request.
func (r *Registry) GetForRequest(path, namespace string) *MirrorGroup {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Try namespace-specific group first
	if group, ok := r.byNS[namespace]; ok {
		return group
	}

	// Try path-specific group
	var best *MirrorGroup
	var bestLen int
	for prefix, group := range r.byPath {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			if len(prefix) > bestLen {
				best = group
				bestLen = len(prefix)
			}
		}
	}
	if best != nil {
		return best
	}

	return r.defaultG
}

// List returns all registered mirror groups.
func (r *Registry) List() []*MirrorGroup {
	r.mu.RLock()
	defer r.mu.RUnlock()

	groups := make([]*MirrorGroup, 0, len(r.groups))
	for _, g := range r.groups {
		groups = append(groups, g)
	}
	return groups
}

// GetDefault returns the default mirror group.
func (r *Registry) GetDefault() *MirrorGroup {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.defaultG
}

// SetDefault sets the default mirror group.
func (r *Registry) SetDefault(groupID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	group, ok := r.groups[groupID]
	if !ok {
		return fmt.Errorf("mirror group %s not found", groupID)
	}

	r.defaultG = group
	return nil
}

// GetAllHealth returns health status for all mirrors in all groups.
func (r *Registry) GetAllHealth() map[string]map[string]*MirrorHealth {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]map[string]*MirrorHealth)
	for groupID, group := range r.groups {
		groupHealth := make(map[string]*MirrorHealth)
		for mirrorID := range group.mirrors {
			if h, ok := group.health[mirrorID]; ok {
				groupHealth[mirrorID] = h
			}
		}
		result[groupID] = groupHealth
	}
	return result
}

// GetAllStats returns statistics for all mirrors in all groups.
func (r *Registry) GetAllStats() map[string]map[string]*MirrorStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]map[string]*MirrorStats)
	for groupID, group := range r.groups {
		groupStats := make(map[string]*MirrorStats)
		for mirrorID := range group.mirrors {
			if s, ok := group.stats[mirrorID]; ok {
				groupStats[mirrorID] = s
			}
		}
		result[groupID] = groupStats
	}
	return result
}
