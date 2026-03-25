// Package checkpoint provides persistent state checkpointing for state machines.
//
// The Adapter hooks into a state machine's OnTransition callback to persist the
// current state after each transition. On restart, the last checkpointed state
// can be restored and used as the initial state for a new machine instance.
//
// Usage:
//
//	adapter := checkpoint.New[State, Event](store, "bootstrap-abc", "bootstrap")
//	restored, ok := adapter.Restore(ctx)
//	initial := PhaseInitializing
//	if ok { initial = restored }
//	builder := statemachine.New[State, Event](initial).
//	    AddTransition(...)
//	adapter.Wrap(builder)
//	machine := builder.MustBuild()
package checkpoint

import (
	"context"
	"encoding/json"
	"time"

	"github.com/shawnbutts/keystone-core/internal/logging"
	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

// Record represents a persisted machine checkpoint.
type Record struct {
	MachineID   string         `json:"machine_id"`
	MachineName string         `json:"machine_name"`
	State       string         `json:"state"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// Store persists and retrieves machine checkpoints.
type Store interface {
	Save(ctx context.Context, r *Record) error
	Load(ctx context.Context, machineID string) (*Record, error)
	Delete(ctx context.Context, machineID string) error
	List(ctx context.Context, machineName string) ([]*Record, error)
}

// SaveErrorFunc is called when a checkpoint save fails. The transition has
// already completed — this is a notification hook for alerting or metrics.
type SaveErrorFunc func(machineID string, state string, err error)

// Adapter connects a state machine to a checkpoint store.
// The S and E type parameters must be ~string so that states can be
// serialized to/from the string-typed Record.State field.
type Adapter[S ~string, E ~string] struct {
	store       Store
	machineID   string
	machineName string
	logger      logging.Logger
	onSaveError SaveErrorFunc
}

// New creates a checkpoint adapter for the given machine instance.
func New[S ~string, E ~string](store Store, machineID, machineName string) *Adapter[S, E] {
	return &Adapter[S, E]{
		store:       store,
		machineID:   machineID,
		machineName: machineName,
		logger:      logging.WithFields(logging.Field{Key: "component", Value: "checkpoint"}, logging.Field{Key: "machine", Value: machineName}),
	}
}

// WithOnSaveError registers a callback that fires when a checkpoint save
// fails. The transition still proceeds — this is for alerting or metrics.
func (a *Adapter[S, E]) WithOnSaveError(fn SaveErrorFunc) *Adapter[S, E] {
	a.onSaveError = fn
	return a
}

// Wrap adds an OnTransition callback to the builder that persists state
// after each transition. Persistence is best-effort: errors are logged
// but do not block the transition.
func (a *Adapter[S, E]) Wrap(builder *statemachine.Builder[S, E]) {
	builder.OnTransition(func(ctx context.Context, from, to S, event E) {
		r := &Record{
			MachineID:   a.machineID,
			MachineName: a.machineName,
			State:       string(to),
			UpdatedAt:   time.Now(),
		}
		if err := a.store.Save(ctx, r); err != nil {
			a.logger.WithContext(ctx).Warn("checkpoint save failed",
				logging.Field{Key: "machine_id", Value: a.machineID},
				logging.Field{Key: "state", Value: string(to)},
				logging.Field{Key: "error", Value: err},
			)
			if a.onSaveError != nil {
				a.onSaveError(a.machineID, string(to), err)
			}
		}
	})
}

// Restore loads the last checkpointed state. Returns the state and true
// if a checkpoint was found, or the zero value and false otherwise.
func (a *Adapter[S, E]) Restore(ctx context.Context) (S, bool) {
	r, err := a.store.Load(ctx, a.machineID)
	if err != nil {
		a.logger.WithContext(ctx).Warn("checkpoint load failed",
			logging.Field{Key: "machine_id", Value: a.machineID},
			logging.Field{Key: "error", Value: err},
		)
		var zero S
		return zero, false
	}
	if r == nil {
		var zero S
		return zero, false
	}
	return S(r.State), true
}

// SaveWithMetadata persists a checkpoint with additional metadata.
func (a *Adapter[S, E]) SaveWithMetadata(ctx context.Context, state S, metadata map[string]any) error {
	metaCopy := make(map[string]any, len(metadata))
	for k, v := range metadata {
		metaCopy[k] = v
	}

	r := &Record{
		MachineID:   a.machineID,
		MachineName: a.machineName,
		State:       string(state),
		Metadata:    metaCopy,
		UpdatedAt:   time.Now(),
	}
	return a.store.Save(ctx, r)
}

// Delete removes the checkpoint for this machine instance.
func (a *Adapter[S, E]) Delete(ctx context.Context) error {
	return a.store.Delete(ctx, a.machineID)
}

// MarshalMetadata is a helper to serialize metadata to JSON for storage.
func MarshalMetadata(m map[string]any) (string, error) {
	if m == nil {
		return "", nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// UnmarshalMetadata is a helper to deserialize metadata from JSON.
func UnmarshalMetadata(s string) (map[string]any, error) {
	if s == "" {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, err
	}
	return m, nil
}
