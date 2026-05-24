// SPDX-License-Identifier: Apache-2.0

package runbook

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidRunbook wraps every structural validation failure so
// callers can errors.Is against it. Individual reasons are joined.
var ErrInvalidRunbook = errors.New("runbook: invalid runbook")

// Validate checks the runbook for internal consistency. All problems
// are collected and returned joined under ErrInvalidRunbook.
//
// It does NOT check that a step's Type is registered — that is the
// engine's concern at execute time (the registry is supplied then,
// Epic 15 task 8).
func (rb *Runbook) Validate() error {
	var errs []error
	add := func(format string, a ...any) {
		errs = append(errs, fmt.Errorf(format, a...))
	}

	if rb.Metadata.Name == "" {
		add("metadata.name is required")
	}
	if len(rb.Spec.Steps) == 0 {
		add("spec.steps must contain at least one step")
	}
	if rb.Spec.Timeout != "" {
		if _, err := time.ParseDuration(rb.Spec.Timeout); err != nil {
			add("spec.timeout %q is not a valid duration: %v", rb.Spec.Timeout, err)
		}
	}
	if rb.Spec.MaxRetries < 0 {
		add("spec.max_retries must be >= 0")
	}

	names := make(map[string]bool, len(rb.Spec.Steps))
	for i, s := range rb.Spec.Steps {
		switch {
		case s.Name == "":
			add("spec.steps[%d].name is required", i)
		case names[s.Name]:
			add("spec.steps: duplicate step name %q", s.Name)
		}
		if s.Name != "" {
			names[s.Name] = true
		}
		if s.Type == "" {
			add("step %q: type is required", stepLabel(s, i))
		}
		if s.Retries < 0 {
			add("step %q: retries must be >= 0", stepLabel(s, i))
		}
		if s.Timeout != "" {
			if _, err := time.ParseDuration(s.Timeout); err != nil {
				add("step %q: timeout %q is not a valid duration: %v", stepLabel(s, i), s.Timeout, err)
			}
		}
	}

	// Reference closure: DependsOn / OnSuccess / OnFailure must name
	// declared steps. Self-dependency is rejected here; broader cycles
	// are the DAG resolver's job (ErrStepCycle).
	for i, s := range rb.Spec.Steps {
		for _, dep := range s.DependsOn {
			if !names[dep] {
				add("step %q: depends_on unknown step %q", stepLabel(s, i), dep)
			}
			if dep == s.Name {
				add("step %q: depends_on itself", stepLabel(s, i))
			}
		}
	}
	for _, ref := range rb.Spec.OnSuccess {
		if !names[ref] {
			add("spec.on_success references unknown step %q", ref)
		}
	}
	for _, ref := range rb.Spec.OnFailure {
		if !names[ref] {
			add("spec.on_failure references unknown step %q", ref)
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("%w (%s): %w", ErrInvalidRunbook, rb.Metadata.Name, errors.Join(errs...))
}

func stepLabel(s Step, i int) string {
	if s.Name != "" {
		return s.Name
	}
	return fmt.Sprintf("#%d", i)
}
