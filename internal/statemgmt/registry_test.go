// SPDX-License-Identifier: Apache-2.0

package statemgmt

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
)

// noopModule is the simplest possible Module satisfying the
// interface. It exists only for tests; real modules live in
// internal/statemgmt/modules/<name>/ once Task 11 lands.
type noopModule struct {
	name string
}

func (m *noopModule) Name() string                                           { return m.name }
func (m *noopModule) ValidStates() []string                                  { return []string{"present"} }
func (m *noopModule) Check(context.Context, *Declaration) (*ModuleCheckResult, error) {
	return &ModuleCheckResult{Matches: true}, nil
}
func (m *noopModule) Apply(context.Context, *Declaration) (*StateResult, error) {
	return &StateResult{Success: true}, nil
}
func (m *noopModule) Test(context.Context, *Declaration) (bool, error) { return true, nil }

func noopFactory(name string) Factory {
	return func() Module { return &noopModule{name: name} }
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	if err := r.Register("file", noopFactory("file")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	m, err := r.Get("file")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if m.Name() != "file" {
		t.Errorf("Module.Name() = %q, want %q", m.Name(), "file")
	}
}

func TestRegistry_Get_ReturnsFreshInstance(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	_ = r.Register("file", noopFactory("file"))
	a, _ := r.Get("file")
	b, _ := r.Get("file")
	if a == b {
		t.Error("Get returned the same instance twice; factory should produce a fresh Module per call")
	}
}

func TestRegistry_Register_DuplicateRejected(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	if err := r.Register("file", noopFactory("file")); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	err := r.Register("file", noopFactory("file"))
	if !errors.Is(err, ErrDuplicateModule) {
		t.Fatalf("second Register err = %v, want wrapping ErrDuplicateModule", err)
	}
}

func TestRegistry_Register_EmptyName(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	err := r.Register("", noopFactory("x"))
	if !errors.Is(err, ErrInvalidModuleName) {
		t.Fatalf("err = %v, want wrapping ErrInvalidModuleName", err)
	}
}

func TestRegistry_Register_NilFactory(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	err := r.Register("file", nil)
	if !errors.Is(err, ErrNilFactory) {
		t.Fatalf("err = %v, want wrapping ErrNilFactory", err)
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	m, err := r.Get("ghost")
	if m != nil {
		t.Errorf("Get returned non-nil Module for unknown name: %#v", m)
	}
	if !errors.Is(err, ErrModuleNotFound) {
		t.Fatalf("err = %v, want wrapping ErrModuleNotFound", err)
	}
}

func TestRegistry_Has(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	if r.Has("file") {
		t.Error("Has on empty registry returned true")
	}
	_ = r.Register("file", noopFactory("file"))
	if !r.Has("file") {
		t.Error("Has on registered name returned false")
	}
	if r.Has("ghost") {
		t.Error("Has on unregistered name returned true")
	}
}

func TestRegistry_List_Sorted(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	if got := r.List(); len(got) != 0 {
		t.Fatalf("empty List = %v, want empty", got)
	}
	names := []string{"service", "file", "package", "user"}
	for _, n := range names {
		if err := r.Register(n, noopFactory(n)); err != nil {
			t.Fatalf("Register %q: %v", n, err)
		}
	}
	got := r.List()
	want := append([]string(nil), names...)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("List len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("List()[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

func TestDefaultRegistry_RegisterModule(t *testing.T) {
	// Not parallel: mutates the global DefaultRegistry.
	name := fmt.Sprintf("test-default-%d", testCounter.next())
	t.Cleanup(func() {
		DefaultRegistry.mu.Lock()
		delete(DefaultRegistry.factories, name)
		DefaultRegistry.mu.Unlock()
	})
	if err := RegisterModule(name, noopFactory(name)); err != nil {
		t.Fatalf("RegisterModule: %v", err)
	}
	if !DefaultRegistry.Has(name) {
		t.Fatalf("DefaultRegistry.Has(%q) = false after RegisterModule", name)
	}
	m, err := DefaultRegistry.Get(name)
	if err != nil {
		t.Fatalf("DefaultRegistry.Get: %v", err)
	}
	if m.Name() != name {
		t.Errorf("Module.Name() = %q, want %q", m.Name(), name)
	}
}

func TestRegistry_ConcurrentRegisterAndGet(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	const N = 64
	var wg sync.WaitGroup
	wg.Add(N * 2)
	for i := 0; i < N; i++ {
		name := fmt.Sprintf("m%d", i)
		go func(name string) {
			defer wg.Done()
			if err := r.Register(name, noopFactory(name)); err != nil {
				t.Errorf("Register %q: %v", name, err)
			}
		}(name)
		go func(name string) {
			defer wg.Done()
			// Get may or may not find the module — we just need
			// to confirm the race detector stays quiet and either
			// outcome is well-formed.
			if m, err := r.Get(name); err == nil && m.Name() != name {
				t.Errorf("Get(%q) returned module named %q", name, m.Name())
			} else if err != nil && !errors.Is(err, ErrModuleNotFound) {
				t.Errorf("Get(%q) err = %v, want nil or ErrModuleNotFound", name, err)
			}
		}(name)
	}
	wg.Wait()
	if got := len(r.List()); got != N {
		t.Errorf("final List len = %d, want %d", got, N)
	}
}

// testCounter hands out unique suffixes for tests that mutate
// DefaultRegistry so parallel test binaries do not collide on names.
var testCounter counter

type counter struct {
	mu sync.Mutex
	n  int
}

func (c *counter) next() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
	return c.n
}
