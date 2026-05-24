// SPDX-License-Identifier: Apache-2.0

package runbook

import (
	"errors"
	"strings"
	"testing"
)

func okRunbook() *Runbook {
	return &Runbook{
		Metadata: Metadata{Name: "rb"},
		Spec: Spec{
			Steps: []Step{
				{Type: "noop", Name: "a"},
				{Type: "noop", Name: "b", DependsOn: []string{"a"}},
			},
		},
	}
}

func TestValidate_OK(t *testing.T) {
	rb := okRunbook()
	rb.Spec.Timeout = "30s"
	rb.Spec.OnSuccess = []string{"b"}
	rb.Spec.OnFailure = []string{"a"}
	if err := rb.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidate_Errors(t *testing.T) {
	tests := []struct {
		name string
		mut  func(*Runbook)
		want string
	}{
		{"no name", func(r *Runbook) { r.Metadata.Name = "" }, "metadata.name is required"},
		{"no steps", func(r *Runbook) { r.Spec.Steps = nil }, "at least one step"},
		{"bad spec timeout", func(r *Runbook) { r.Spec.Timeout = "nope" }, "spec.timeout"},
		{"neg max retries", func(r *Runbook) { r.Spec.MaxRetries = -1 }, "max_retries must be >= 0"},
		{"empty step name", func(r *Runbook) { r.Spec.Steps[1].Name = "" }, "name is required"},
		{"dup step name", func(r *Runbook) { r.Spec.Steps[1].Name = "a" }, "duplicate step name"},
		{"empty type", func(r *Runbook) { r.Spec.Steps[0].Type = "" }, "type is required"},
		{"neg retries", func(r *Runbook) { r.Spec.Steps[0].Retries = -2 }, "retries must be >= 0"},
		{"bad step timeout", func(r *Runbook) { r.Spec.Steps[0].Timeout = "x" }, "is not a valid duration"},
		{"unknown depends_on", func(r *Runbook) { r.Spec.Steps[0].DependsOn = []string{"ghost"} }, "depends_on unknown step"},
		{"self depends_on", func(r *Runbook) { r.Spec.Steps[0].DependsOn = []string{"a"} }, "depends_on itself"},
		{"bad on_success", func(r *Runbook) { r.Spec.OnSuccess = []string{"ghost"} }, "on_success references unknown"},
		{"bad on_failure", func(r *Runbook) { r.Spec.OnFailure = []string{"ghost"} }, "on_failure references unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rb := okRunbook()
			tc.mut(rb)
			err := rb.Validate()
			if !errors.Is(err, ErrInvalidRunbook) {
				t.Fatalf("err=%v want ErrInvalidRunbook", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%q want substring %q", err, tc.want)
			}
		})
	}
}
