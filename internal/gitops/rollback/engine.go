package rollback

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"go.keystone-core.io/keystone-core/pkg/statemachine"
)

// ErrRollbackNotFound is returned by engine ops on an unknown id.
var ErrRollbackNotFound = errors.New("rollback: not found")

// TransitionRecord is one audit entry of a rollback's lifecycle.
type TransitionRecord struct {
	From  RollbackState `json:"from"`
	To    RollbackState `json:"to"`
	Event RollbackEvent `json:"event"`
	At    time.Time     `json:"at"`
}

// Rollback is the persisted record of one rollback through its
// lifecycle. State is the source of truth across process boundaries;
// the FSM is rebuilt from it for each operation.
type Rollback struct {
	ID              string             `json:"id"`
	Application     string             `json:"application"`
	ExecutorType    string             `json:"executor_type"`
	Strategy        Strategy           `json:"strategy"`
	Revision        string             `json:"revision,omitempty"`
	Reason          string             `json:"reason,omitempty"`
	RequireApproval bool               `json:"require_approval"`
	State           RollbackState      `json:"state"`
	FromRevision    string             `json:"from_revision,omitempty"`
	ToRevision      string             `json:"to_revision,omitempty"`
	Result          *Result            `json:"result,omitempty"`
	Approver        string             `json:"approver,omitempty"`
	Error           string             `json:"error,omitempty"`
	Transitions     []TransitionRecord `json:"transitions,omitempty"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

// RollbackSpec is the input to [Engine.Execute].
type RollbackSpec struct {
	ExecutorType    string // "git" | "argocd" | "k8s"
	Config          Config
	Request         Request
	RequireApproval bool
}

// RollbackStore persists [Rollback] records. The v1.0 in-memory
// [MemoryStore] is process-scoped; the durable SQLite store is
// Epic 16 task 9.
type RollbackStore interface {
	Save(ctx context.Context, rb *Rollback) error
	Get(ctx context.Context, id string) (*Rollback, bool, error)
	List(ctx context.Context) ([]*Rollback, error)
}

// PostVerifier optionally verifies a rollback after it completes. A
// nil verifier (engine default) makes Completed a terminal state; a
// configured verifier drives Completed → Verifying → Verified |
// VerificationFailed. Narrow seam so this package needs no
// verification-engine import.
type PostVerifier interface {
	Verify(ctx context.Context, rb *Rollback) (bool, error)
}

// MemoryStore is the v1.0 in-memory [RollbackStore]. Records are
// deep-ish copied on the way in/out so callers cannot mutate stored
// state by holding the pointer.
type MemoryStore struct {
	mu sync.RWMutex
	m  map[string]Rollback
}

// NewMemoryStore returns an empty store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{m: make(map[string]Rollback)}
}

// Save implements [RollbackStore].
func (s *MemoryStore) Save(_ context.Context, rb *Rollback) error {
	if rb == nil || rb.ID == "" {
		return errors.New("rollback: store save: nil record or empty id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *rb
	cp.Transitions = append([]TransitionRecord(nil), rb.Transitions...)
	s.m[rb.ID] = cp
	return nil
}

// Get implements [RollbackStore].
func (s *MemoryStore) Get(_ context.Context, id string) (*Rollback, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rb, ok := s.m[id]
	if !ok {
		return nil, false, nil
	}
	rb.Transitions = append([]TransitionRecord(nil), rb.Transitions...)
	return &rb, true, nil
}

// List implements [RollbackStore]; newest CreatedAt last is not
// guaranteed — callers sort if they need order.
func (s *MemoryStore) List(_ context.Context) ([]*Rollback, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Rollback, 0, len(s.m))
	for _, rb := range s.m {
		c := rb
		c.Transitions = append([]TransitionRecord(nil), rb.Transitions...)
		out = append(out, &c)
	}
	return out, nil
}

// Engine orchestrates rollbacks: it picks an [Executor] from its
// registry, drives the lifecycle state machine (with optional
// approval gate), runs optional post-verification, and persists every
// transition to the store.
type Engine struct {
	reg      *Registry
	store    RollbackStore
	verifier PostVerifier
	now      func() time.Time
	idGen    func() string
}

// Option customises an [Engine].
type Option func(*Engine)

// WithPostVerifier attaches a post-completion verifier.
func WithPostVerifier(v PostVerifier) Option { return func(e *Engine) { e.verifier = v } }

// WithClock injects a deterministic time source (tests).
func WithClock(fn func() time.Time) Option { return func(e *Engine) { e.now = fn } }

// WithIDGenerator injects a deterministic id source (tests).
func WithIDGenerator(fn func() string) Option { return func(e *Engine) { e.idGen = fn } }

// NewEngine returns an Engine backed by store. Executors are added
// via [Engine.RegisterExecutor].
func NewEngine(store RollbackStore, opts ...Option) *Engine {
	e := &Engine{
		reg:   NewRegistry(),
		store: store,
		now:   time.Now,
		idGen: uuid.NewString,
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// RegisterExecutor adds an executor to the engine's registry.
func (e *Engine) RegisterExecutor(x Executor) error { return e.reg.Register(x) }

// Execute starts a rollback. When spec.RequireApproval is set the
// rollback is persisted Pending and returned for a later
// [Engine.ApproveRollback]; otherwise it is driven to a terminal
// state synchronously and the finished record returned.
func (e *Engine) Execute(ctx context.Context, spec RollbackSpec) (*Rollback, error) {
	if _, ok := e.reg.Lookup(spec.ExecutorType); !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownExecutor, spec.ExecutorType)
	}
	ts := e.now()
	rb := &Rollback{
		ID:              e.idGen(),
		Application:     spec.Request.Application,
		ExecutorType:    spec.ExecutorType,
		Strategy:        spec.Request.Strategy,
		Revision:        spec.Request.Revision,
		Reason:          spec.Request.Reason,
		RequireApproval: spec.RequireApproval,
		State:           StatePending,
		CreatedAt:       ts,
		UpdatedAt:       ts,
	}
	if err := e.store.Save(ctx, rb); err != nil {
		return nil, err
	}
	if spec.RequireApproval {
		return rb, nil
	}
	if err := e.fire(ctx, rb, EventApprove); err != nil {
		return rb, err
	}
	return e.drive(ctx, rb, spec), nil
}

// ApproveRollback approves a Pending rollback and drives it to a
// terminal state. The original spec is reconstructed from the record.
func (e *Engine) ApproveRollback(ctx context.Context, id, approver string) (*Rollback, error) {
	rb, err := e.load(ctx, id)
	if err != nil {
		return nil, err
	}
	rb.Approver = approver
	if ferr := e.fire(ctx, rb, EventApprove); ferr != nil {
		return rb, ferr
	}
	spec := RollbackSpec{
		ExecutorType: rb.ExecutorType,
		Request: Request{
			Application: rb.Application,
			Strategy:    rb.Strategy,
			Revision:    rb.Revision,
			Reason:      rb.Reason,
		},
	}
	return e.drive(ctx, rb, spec), nil
}

// RejectRollback rejects a Pending rollback (terminal). It is the
// other half of the Pending fork the epic state machine specifies;
// the epic API list names only ApproveRollback but Pending→Rejected
// is unreachable without it.
func (e *Engine) RejectRollback(ctx context.Context, id, approver, reason string) (*Rollback, error) {
	rb, err := e.load(ctx, id)
	if err != nil {
		return nil, err
	}
	rb.Approver = approver
	if reason != "" {
		rb.Error = "rejected: " + reason
	}
	if ferr := e.fire(ctx, rb, EventReject); ferr != nil {
		return rb, ferr
	}
	return rb, nil
}

// GetRollback returns the stored record.
func (e *Engine) GetRollback(ctx context.Context, id string) (*Rollback, bool, error) {
	return e.store.Get(ctx, id)
}

// ListRollbacks returns all stored records.
func (e *Engine) ListRollbacks(ctx context.Context) ([]*Rollback, error) {
	return e.store.List(ctx)
}

// drive runs Approved → InProgress → (Completed|Failed) → optional
// (Verifying → Verified|VerificationFailed). It assumes rb is already
// Approved. Errors from individual transitions are recorded on the
// record (rb.Error); drive returns the record, not an error, so the
// caller always sees the terminal state.
func (e *Engine) drive(ctx context.Context, rb *Rollback, spec RollbackSpec) *Rollback {
	if err := e.fire(ctx, rb, EventStart); err != nil {
		return rb
	}
	x, ok := e.reg.Lookup(rb.ExecutorType)
	if !ok {
		rb.Error = fmt.Sprintf("%v: %q", ErrUnknownExecutor, rb.ExecutorType)
		_ = e.fire(ctx, rb, EventFail)
		return rb
	}

	res := x.Execute(ctx, spec.Config, spec.Request)
	rb.Result = &res
	rb.FromRevision, rb.ToRevision = res.FromRevision, res.ToRevision
	if !res.Success {
		if res.Error != nil {
			rb.Error = res.Error.Error()
		} else if res.Message != "" {
			rb.Error = res.Message
		}
		_ = e.fire(ctx, rb, EventFail)
		return rb
	}
	if err := e.fire(ctx, rb, EventComplete); err != nil {
		return rb
	}

	if e.verifier == nil {
		return rb // Completed is terminal without a verifier
	}
	if err := e.fire(ctx, rb, EventVerify); err != nil {
		return rb
	}
	ok, verr := e.verifier.Verify(ctx, rb)
	if verr != nil {
		rb.Error = "verification error: " + verr.Error()
		_ = e.fire(ctx, rb, EventVerifyFail)
		return rb
	}
	if ok {
		_ = e.fire(ctx, rb, EventVerifyOK)
	} else {
		_ = e.fire(ctx, rb, EventVerifyFail)
	}
	return rb
}

// fire rebuilds the FSM at rb.State, applies event, records the
// transition + timestamp on the record, and persists. A rejected
// transition is wrapped as ErrInvalidTransition and the record is
// left unchanged (not persisted).
func (e *Engine) fire(ctx context.Context, rb *Rollback, event RollbackEvent) error {
	m, err := newMachine(rb.State)
	if err != nil {
		return err
	}
	from := rb.State
	if ferr := m.Fire(ctx, event); ferr != nil {
		if errors.Is(ferr, statemachine.ErrNoTransition) || errors.Is(ferr, statemachine.ErrUnknownEvent) {
			return fmt.Errorf("%w: state=%s event=%s", ErrInvalidTransition, from, event)
		}
		return ferr
	}
	to := m.Current()
	now := e.now()
	rb.State = to
	rb.UpdatedAt = now
	rb.Transitions = append(rb.Transitions, TransitionRecord{From: from, To: to, Event: event, At: now})
	return e.store.Save(ctx, rb)
}

func (e *Engine) load(ctx context.Context, id string) (*Rollback, error) {
	rb, ok, err := e.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrRollbackNotFound, id)
	}
	return rb, nil
}
