// SPDX-License-Identifier: Apache-2.0

package runbook

import (
	"context"
	"time"
)

// ObserverEvent is one status transition: execution-level when Step
// is "", otherwise a step transition. It mirrors a [TrailEntry] plus
// the owning execution's identity so a fan-out observer (events /
// audit) has everything without holding the *Execution.
type ObserverEvent struct {
	ExecutionID string
	Runbook     string
	Step        string // "" = execution-level transition
	From        Status
	To          Status
	Note        string
	At          time.Time
}

// Observer receives every execution/step status transition as the
// engine records it onto the audit Trail. Implementations must be
// non-blocking and must not panic (the engine does not recover). The
// adapters that fan out to the event bus / audit log live in
// internal/runbook/observer.
type Observer interface {
	OnTransition(ctx context.Context, ev ObserverEvent)
}

type noopObserver struct{}

func (noopObserver) OnTransition(context.Context, ObserverEvent) {}

// MultiObserver fans one transition out to several observers in
// order. A nil element is skipped.
type MultiObserver []Observer

// OnTransition implements [Observer].
func (m MultiObserver) OnTransition(ctx context.Context, ev ObserverEvent) {
	for _, o := range m {
		if o != nil {
			o.OnTransition(ctx, ev)
		}
	}
}
