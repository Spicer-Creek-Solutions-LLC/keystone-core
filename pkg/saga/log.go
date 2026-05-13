package saga

import (
	"context"
	"errors"
)

// ErrNotFound is returned by [Log.GetExecution] when no execution
// matches the requested ID. Tests rely on this for not-found
// semantics.
var ErrNotFound = errors.New("saga: execution not found")

// Log persists [Execution] records.
//
// v0.1 ships [NewInMemoryLog] only. A SQLite implementation
// (`pkg/saga/log_sqlite`) is in the v0.x roadmap and is the
// prerequisite for the v1.x checkpoint-resume feature — that
// implementation will record per-step transitions so a crashed
// saga can be re-loaded and continued. v0.1's [Coordinator.Run]
// records the initial Pending→Running state once and the terminal
// state once.
type Log interface {
	// SaveExecution writes the [Execution] under its ID. Successive
	// calls with the same ID overwrite. Returns an error only on
	// backend failure; the in-memory log never returns an error.
	SaveExecution(ctx context.Context, e *Execution) error
	// GetExecution returns the most recently saved [Execution] for
	// id. Returns [ErrNotFound] when no record exists.
	GetExecution(ctx context.Context, id string) (*Execution, error)
	// ListExecutions returns every saved [Execution] in insertion
	// order (oldest first). Returned slice may be modified by the
	// caller without affecting the log's internal state.
	ListExecutions(ctx context.Context) ([]*Execution, error)
}
