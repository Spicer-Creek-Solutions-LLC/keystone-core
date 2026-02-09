package dns

import (
	"context"
	"fmt"
	"time"
)

// SyncOptions configures sync behavior.
type SyncOptions struct {
	// DryRun performs a diff without making changes
	DryRun bool

	// DeleteExisting allows deletion of records not in desired state
	DeleteExisting bool

	// IgnoreTTL ignores TTL differences when diffing
	IgnoreTTL bool

	// IgnoreProxied ignores proxied field differences
	IgnoreProxied bool

	// BatchSize limits concurrent operations (0 = unlimited)
	BatchSize int
}

// Syncer synchronizes DNS records with a provider.
type Syncer struct {
	provider Provider
	zone     string
	options  SyncOptions
}

// NewSyncer creates a new syncer for a zone.
func NewSyncer(provider Provider, zone string, options SyncOptions) *Syncer {
	return &Syncer{
		provider: provider,
		zone:     zone,
		options:  options,
	}
}

// Sync synchronizes DNS records to match the desired state.
func (s *Syncer) Sync(ctx context.Context, desired []Record) (*SyncResult, error) {
	start := time.Now()
	result := &SyncResult{Zone: s.zone}

	// Get current records from provider
	current, err := s.provider.GetRecords(ctx, s.zone)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("failed to get current records: %w", err))
		result.Duration = time.Since(start)
		return result, nil
	}

	// Compute diff
	differ := NewDiffer(s.zone)
	differ.IgnoreTTL = s.options.IgnoreTTL
	differ.IgnoreProxied = s.options.IgnoreProxied
	plan := differ.Diff(desired, current)

	result.Changes = plan.Changes

	// If dry-run, return the plan without making changes
	if s.options.DryRun {
		summary := plan.Summary()
		result.Created = summary.Create
		result.Updated = summary.Update
		result.Deleted = summary.Delete
		result.Unchanged = summary.Noop
		result.Duration = time.Since(start)
		return result, nil
	}

	// Apply changes
	for _, change := range plan.Changes {
		if ctx.Err() != nil {
			result.Errors = append(result.Errors, ctx.Err())
			break
		}

		switch change.Type {
		case ChangeTypeCreate:
			_, err := s.provider.CreateRecord(ctx, s.zone, *change.Record)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("create %s %s: %w",
					change.Record.Type, change.Record.Name, err))
			} else {
				result.Created++
			}

		case ChangeTypeUpdate:
			_, err := s.provider.UpdateRecord(ctx, s.zone, *change.Record)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("update %s %s: %w",
					change.Record.Type, change.Record.Name, err))
			} else {
				result.Updated++
			}

		case ChangeTypeDelete:
			if !s.options.DeleteExisting {
				continue
			}
			err := s.provider.DeleteRecord(ctx, s.zone, *change.Record)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("delete %s %s: %w",
					change.Record.Type, change.Record.Name, err))
			} else {
				result.Deleted++
			}

		case ChangeTypeNoop:
			result.Unchanged++
		}
	}

	result.Duration = time.Since(start)
	return result, nil
}

// Check performs a diff and returns the plan without making changes.
func (s *Syncer) Check(ctx context.Context, desired []Record) (*Plan, error) {
	current, err := s.provider.GetRecords(ctx, s.zone)
	if err != nil {
		return nil, fmt.Errorf("failed to get current records: %w", err)
	}

	differ := NewDiffer(s.zone)
	differ.IgnoreTTL = s.options.IgnoreTTL
	differ.IgnoreProxied = s.options.IgnoreProxied

	return differ.Diff(desired, current), nil
}

// Operation represents a structured DNS operation for dry-run output.
type Operation struct {
	// Action is the operation type (create, update, delete, noop)
	Action ChangeType `json:"action"`

	// Zone is the DNS zone
	Zone string `json:"zone"`

	// Record is the target record state
	Record Record `json:"record"`

	// Before is the current record state (for updates/deletes)
	Before *Record `json:"before,omitempty"`

	// Changes describes field-level changes (for updates)
	Changes map[string]FieldDiff `json:"changes,omitempty"`
}

// PlanOperations converts a plan to structured operations for dry-run output.
func PlanOperations(plan *Plan) []Operation {
	ops := make([]Operation, 0, len(plan.Changes))
	for _, change := range plan.Changes {
		op := Operation{
			Action: change.Type,
			Zone:   plan.Zone,
			Record: *change.Record,
		}

		if change.Current != nil {
			op.Before = change.Current
		}

		if change.Diff != nil {
			op.Changes = change.Diff
		}

		ops = append(ops, op)
	}
	return ops
}

// FilterOperations returns only operations matching the given action types.
func FilterOperations(ops []Operation, actions ...ChangeType) []Operation {
	if len(actions) == 0 {
		return ops
	}

	actionSet := make(map[ChangeType]bool)
	for _, a := range actions {
		actionSet[a] = true
	}

	filtered := make([]Operation, 0)
	for i := range ops {
		if actionSet[ops[i].Action] {
			filtered = append(filtered, ops[i])
		}
	}
	return filtered
}

// DryRunResult contains structured dry-run output.
type DryRunResult struct {
	Zone       string      `json:"zone"`
	Operations []Operation `json:"operations"`
	Summary    PlanSummary `json:"summary"`
}

// DryRun performs a check and returns structured dry-run output.
func (s *Syncer) DryRun(ctx context.Context, desired []Record) (*DryRunResult, error) {
	plan, err := s.Check(ctx, desired)
	if err != nil {
		return nil, err
	}

	return &DryRunResult{
		Zone:       s.zone,
		Operations: PlanOperations(plan),
		Summary:    plan.Summary(),
	}, nil
}
