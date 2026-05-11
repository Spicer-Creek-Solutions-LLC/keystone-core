package statemgmt

import (
	"fmt"
	"sort"
	"sync"
)

// Registry maps a module name to the Factory that constructs it. It
// is safe for concurrent use; the only writer is Register and the
// hot path (Get / Has / List) takes the read lock.
//
// Factories are invoked outside the lock so a factory that reaches
// back into the registry (uncommon, but legal) cannot deadlock.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// NewRegistry returns an empty Registry. Tests should use this
// rather than DefaultRegistry so global state does not leak between
// test cases.
func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]Factory)}
}

// Register associates name with factory. It returns
// ErrInvalidModuleName if name is empty, ErrNilFactory if factory is
// nil, and ErrDuplicateModule (wrapped with the offending name) if
// name is already registered.
func (r *Registry) Register(name string, factory Factory) error {
	if name == "" {
		return ErrInvalidModuleName
	}
	if factory == nil {
		return fmt.Errorf("%w: %q", ErrNilFactory, name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("%w: %q", ErrDuplicateModule, name)
	}
	r.factories[name] = factory
	return nil
}

// Get returns a fresh Module built by the registered factory. It
// returns ErrModuleNotFound (wrapped with the requested name) if no
// factory is registered under that name. The factory is invoked
// without holding the lock.
func (r *Registry) Get(name string) (Module, error) {
	r.mu.RLock()
	factory, ok := r.factories[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrModuleNotFound, name)
	}
	return factory(), nil
}

// Has reports whether a factory is registered under name.
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.factories[name]
	return ok
}

// List returns the registered module names in sorted order.
func (r *Registry) List() []string {
	r.mu.RLock()
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	r.mu.RUnlock()
	sort.Strings(names)
	return names
}

// DefaultRegistry is the process-wide Registry that stdlib modules
// register themselves into via package-level init() calls. The
// runner consults DefaultRegistry by default; tests that need
// isolation should construct their own Registry with NewRegistry.
var DefaultRegistry = NewRegistry()

// RegisterModule registers factory under name in DefaultRegistry. It
// is the convenience entry point stdlib modules call from init().
func RegisterModule(name string, factory Factory) error {
	return DefaultRegistry.Register(name, factory)
}
