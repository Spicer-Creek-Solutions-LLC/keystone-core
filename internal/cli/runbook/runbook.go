// SPDX-License-Identifier: Apache-2.0

// Package runbook implements the kscore-runbook CLI (Epic 15
// task 10): list, execute, status, list-executions, audit, test.
// Dependency-light; the runbook engine (with its step registry) and
// the execution store are injected via Deps so cmd/kscore-runbook
// wires the real implementations at boot.
package runbook

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/cli"
	rb "go.keystone-core.io/keystone-core/internal/runbook"
)

// ErrEngineNotConfigured is returned by execute when no runbook
// Executor was injected.
var ErrEngineNotConfigured = errors.New("kscore-runbook: execution engine not configured")

// ExecutionStore persists runbook executions for status /
// list-executions / audit. v1.0 ships the in-memory implementation;
// a durable backend is the gate-v1.0 ROADMAP item "Durable runbook
// execution store".
type ExecutionStore interface {
	Save(ctx context.Context, e *rb.Execution) error
	Get(ctx context.Context, id string) (*rb.Execution, error)
	List(ctx context.Context) ([]*rb.Execution, error)
}

// MemoryExecutionStore is the default in-process ExecutionStore.
type MemoryExecutionStore struct {
	mu    sync.Mutex
	byID  map[string]*rb.Execution
	order []string
}

// NewMemoryExecutionStore returns an empty in-memory store.
func NewMemoryExecutionStore() *MemoryExecutionStore {
	return &MemoryExecutionStore{byID: make(map[string]*rb.Execution)}
}

// Save stores e under e.ID (overwriting).
func (s *MemoryExecutionStore) Save(_ context.Context, e *rb.Execution) error {
	if e == nil || e.ID == "" {
		return errors.New("execution has no ID")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[e.ID]; !ok {
		s.order = append(s.order, e.ID)
	}
	s.byID[e.ID] = e
	return nil
}

// Get returns the execution for id, or an error.
func (s *MemoryExecutionStore) Get(_ context.Context, id string) (*rb.Execution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.byID[id]
	if !ok {
		return nil, fmt.Errorf("execution %q not found", id)
	}
	return e, nil
}

// List returns executions in insertion order.
func (s *MemoryExecutionStore) List(_ context.Context) ([]*rb.Execution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*rb.Execution, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, s.byID[id])
	}
	return out, nil
}

// Deps wires the engine + store seams. list/test work with a zero
// Deps; execute needs Executor; status/list-executions/audit use
// Store (a zero Deps gets an in-memory store).
type Deps struct {
	Executor *rb.Executor
	Store    ExecutionStore
}

func (d Deps) store() ExecutionStore {
	if d.Store != nil {
		return d.Store
	}
	return NewMemoryExecutionStore()
}

// NewCommand returns the kscore-runbook root command. A shared
// store instance is bound across subcommands so execute + status
// see the same data within one process.
func NewCommand(d Deps) *cobra.Command {
	if d.Store == nil {
		d.Store = NewMemoryExecutionStore()
	}
	root := &cobra.Command{
		Use:           "kscore-runbook",
		Short:         "Keystone Core runbook CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(listCmd(), executeCmd(d), statusCmd(d), listExecutionsCmd(d), auditCmd(d), testCmd())
	cli.AddVersion(root)
	return root
}

func withContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list [dir]",
		Short: "List runbooks (*.yaml) in a directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				return err
			}
			var names []string
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				ext := filepath.Ext(e.Name())
				if ext != ".yaml" && ext != ".yml" {
					continue
				}
				r, err := rb.Load(filepath.Join(dir, e.Name()))
				if err != nil {
					continue // not a runbook (or invalid) — skip silently in a listing
				}
				names = append(names, fmt.Sprintf("%s\t%s", e.Name(), r.Metadata.Name))
			}
			sort.Strings(names)
			w := cmd.OutOrStdout()
			for _, n := range names {
				fmt.Fprintln(w, n)
			}
			if len(names) == 0 {
				fmt.Fprintln(w, "(no runbooks)")
			}
			return nil
		},
	}
}

func testCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test <runbook.yaml>",
		Short: "Statically validate a runbook (load + validate + DAG cycle check)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := rb.Load(args[0]) // Load runs Validate
			if err != nil {
				return err
			}
			if err := r.CheckDAG(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "ok: %s (%d steps)\n", r.Metadata.Name, len(r.Spec.Steps))
			return nil
		},
	}
}
