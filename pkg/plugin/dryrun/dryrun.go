// Package dryrun provides dry-run/preview mode for write operations
package dryrun

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// Mode represents the execution mode
type Mode int

const (
	// ModeExecute performs the actual operation
	ModeExecute Mode = iota

	// ModeDryRun simulates the operation and reports what would happen
	ModeDryRun

	// ModePreview shows a preview of changes without executing
	ModePreview
)

// String returns the string representation of the mode
func (m Mode) String() string {
	switch m {
	case ModeExecute:
		return "execute"
	case ModeDryRun:
		return "dry-run"
	case ModePreview:
		return "preview"
	default:
		return "unknown"
	}
}

// Operation represents a write operation that can be dry-run
type Operation struct {
	// Type is the operation type (create, update, delete, etc.)
	Type OperationType `json:"type"`

	// Resource is the resource type being operated on
	Resource string `json:"resource"`

	// Target is the target of the operation (file path, URL, etc.)
	Target string `json:"target"`

	// Description describes what the operation will do
	Description string `json:"description"`

	// Changes lists the specific changes
	Changes []Change `json:"changes,omitempty"`

	// Before is the state before the operation (for updates)
	Before interface{} `json:"before,omitempty"`

	// After is the expected state after the operation
	After interface{} `json:"after,omitempty"`

	// Metadata contains additional operation metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// Error is set if the operation would fail
	Error string `json:"error,omitempty"`

	// Skipped indicates if the operation would be skipped
	Skipped bool `json:"skipped,omitempty"`

	// SkipReason explains why the operation would be skipped
	SkipReason string `json:"skip_reason,omitempty"`
}

// OperationType represents the type of operation
type OperationType string

// OpCreate constants define the operators.
const (
	OpCreate   OperationType = "create"
	OpUpdate   OperationType = "update"
	OpDelete   OperationType = "delete"
	OpReplace  OperationType = "replace"
	OpAppend   OperationType = "append"
	OpMove     OperationType = "move"
	OpCopy     OperationType = "copy"
	OpChmod    OperationType = "chmod"
	OpChown    OperationType = "chown"
	OpLink     OperationType = "link"
	OpExecute  OperationType = "execute"
	OpDownload OperationType = "download"
	OpUpload   OperationType = "upload"
)

// Change represents a specific change within an operation
type Change struct {
	// Field is the field being changed
	Field string `json:"field,omitempty"`

	// Path is the path within a structure being changed
	Path string `json:"path,omitempty"`

	// OldValue is the old value
	OldValue interface{} `json:"old_value,omitempty"`

	// NewValue is the new value
	NewValue interface{} `json:"new_value,omitempty"`

	// Action is the change action (set, unset, add, remove)
	Action string `json:"action"`
}

// Result contains the result of a dry-run
type Result struct {
	// Mode is the execution mode used
	Mode Mode `json:"mode"`

	// Operations lists all operations that would be performed
	Operations []*Operation `json:"operations"`

	// Summary provides a summary of the dry-run
	Summary *Summary `json:"summary"`

	// Errors lists any errors encountered
	Errors []string `json:"errors,omitempty"`

	// Warnings lists any warnings
	Warnings []string `json:"warnings,omitempty"`
}

// Summary provides statistics about the dry-run
type Summary struct {
	// TotalOperations is the total number of operations
	TotalOperations int `json:"total_operations"`

	// Creates is the number of create operations
	Creates int `json:"creates"`

	// Updates is the number of update operations
	Updates int `json:"updates"`

	// Deletes is the number of delete operations
	Deletes int `json:"deletes"`

	// Skipped is the number of skipped operations
	Skipped int `json:"skipped"`

	// Errors is the number of operations that would fail
	Errors int `json:"errors"`

	// NoChanges indicates if no changes would be made
	NoChanges bool `json:"no_changes"`
}

// Recorder records operations for dry-run mode
type Recorder struct {
	mode       Mode
	operations []*Operation
	errors     []string
	warnings   []string
	mu         sync.Mutex
}

// NewRecorder creates a new recorder
func NewRecorder(mode Mode) *Recorder {
	return &Recorder{
		mode:       mode,
		operations: make([]*Operation, 0),
	}
}

// Mode returns the current mode
func (r *Recorder) Mode() Mode {
	return r.mode
}

// IsDryRun returns true if in dry-run mode
func (r *Recorder) IsDryRun() bool {
	return r.mode == ModeDryRun || r.mode == ModePreview
}

// Record records an operation
func (r *Recorder) Record(op *Operation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.operations = append(r.operations, op)
}

// RecordCreate records a create operation
func (r *Recorder) RecordCreate(resource, target string, content interface{}) *Operation {
	op := &Operation{
		Type:        OpCreate,
		Resource:    resource,
		Target:      target,
		Description: fmt.Sprintf("Create %s: %s", resource, target),
		After:       content,
	}
	r.Record(op)
	return op
}

// RecordUpdate records an update operation
func (r *Recorder) RecordUpdate(resource, target string, before, after interface{}, changes []Change) *Operation {
	op := &Operation{
		Type:        OpUpdate,
		Resource:    resource,
		Target:      target,
		Description: fmt.Sprintf("Update %s: %s", resource, target),
		Before:      before,
		After:       after,
		Changes:     changes,
	}
	r.Record(op)
	return op
}

// RecordDelete records a delete operation
func (r *Recorder) RecordDelete(resource, target string, before interface{}) *Operation {
	op := &Operation{
		Type:        OpDelete,
		Resource:    resource,
		Target:      target,
		Description: fmt.Sprintf("Delete %s: %s", resource, target),
		Before:      before,
	}
	r.Record(op)
	return op
}

// RecordSkip records a skipped operation
func (r *Recorder) RecordSkip(resource, target, reason string) *Operation {
	op := &Operation{
		Type:        OpUpdate,
		Resource:    resource,
		Target:      target,
		Description: fmt.Sprintf("Skip %s: %s", resource, target),
		Skipped:     true,
		SkipReason:  reason,
	}
	r.Record(op)
	return op
}

// RecordError records an operation that would fail
func (r *Recorder) RecordError(resource, target string, opType OperationType, err error) *Operation {
	op := &Operation{
		Type:        opType,
		Resource:    resource,
		Target:      target,
		Description: fmt.Sprintf("%s %s: %s would fail", opType, resource, target),
		Error:       err.Error(),
	}
	r.Record(op)
	return op
}

// AddError adds a general error
func (r *Recorder) AddError(err string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errors = append(r.errors, err)
}

// AddWarning adds a warning
func (r *Recorder) AddWarning(warning string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.warnings = append(r.warnings, warning)
}

// Result returns the dry-run result
func (r *Recorder) Result() *Result {
	r.mu.Lock()
	defer r.mu.Unlock()

	result := &Result{
		Mode:       r.mode,
		Operations: r.operations,
		Errors:     r.errors,
		Warnings:   r.warnings,
		Summary:    r.computeSummary(),
	}

	return result
}

func (r *Recorder) computeSummary() *Summary {
	summary := &Summary{
		TotalOperations: len(r.operations),
	}

	for _, op := range r.operations {
		if op.Skipped {
			summary.Skipped++
			continue
		}
		if op.Error != "" {
			summary.Errors++
			continue
		}

		switch op.Type {
		case OpCreate:
			summary.Creates++
		case OpUpdate, OpReplace, OpAppend, OpChmod, OpChown:
			summary.Updates++
		case OpDelete:
			summary.Deletes++
		default:
		}
	}

	summary.NoChanges = summary.Creates == 0 && summary.Updates == 0 && summary.Deletes == 0

	return summary
}

// Clear clears all recorded operations
func (r *Recorder) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.operations = make([]*Operation, 0)
	r.errors = nil
	r.warnings = nil
}

// Format formats the result for display
func (r *Result) Format(verbose bool) string {
	var buf bytes.Buffer

	buf.WriteString(fmt.Sprintf("Dry-run mode: %s\n\n", r.Mode))

	if len(r.Operations) == 0 {
		buf.WriteString("No operations would be performed.\n")
		return buf.String()
	}

	// Group operations by type
	creates := filterOps(r.Operations, OpCreate)
	updates := filterOps(r.Operations, OpUpdate)
	deletes := filterOps(r.Operations, OpDelete)
	skipped := filterSkipped(r.Operations)
	errors := filterErrors(r.Operations)

	if len(creates) > 0 {
		buf.WriteString(fmt.Sprintf("Would CREATE %d resource(s):\n", len(creates)))
		for _, op := range creates {
			buf.WriteString(fmt.Sprintf("  + %s: %s\n", op.Resource, op.Target))
			if verbose && op.After != nil {
				buf.WriteString(formatContent(op.After, "    "))
			}
		}
		buf.WriteString("\n")
	}

	if len(updates) > 0 {
		buf.WriteString(fmt.Sprintf("Would UPDATE %d resource(s):\n", len(updates)))
		for _, op := range updates {
			buf.WriteString(fmt.Sprintf("  ~ %s: %s\n", op.Resource, op.Target))
			if verbose && len(op.Changes) > 0 {
				for _, c := range op.Changes {
					buf.WriteString(fmt.Sprintf("    %s %s: %v -> %v\n", c.Action, c.Field, c.OldValue, c.NewValue))
				}
			}
		}
		buf.WriteString("\n")
	}

	if len(deletes) > 0 {
		buf.WriteString(fmt.Sprintf("Would DELETE %d resource(s):\n", len(deletes)))
		for _, op := range deletes {
			buf.WriteString(fmt.Sprintf("  - %s: %s\n", op.Resource, op.Target))
		}
		buf.WriteString("\n")
	}

	if len(skipped) > 0 {
		buf.WriteString(fmt.Sprintf("Would SKIP %d resource(s):\n", len(skipped)))
		for _, op := range skipped {
			buf.WriteString(fmt.Sprintf("  ○ %s: %s (%s)\n", op.Resource, op.Target, op.SkipReason))
		}
		buf.WriteString("\n")
	}

	if len(errors) > 0 {
		buf.WriteString(fmt.Sprintf("Would FAIL %d operation(s):\n", len(errors)))
		for _, op := range errors {
			buf.WriteString(fmt.Sprintf("  ✗ %s: %s - %s\n", op.Resource, op.Target, op.Error))
		}
		buf.WriteString("\n")
	}

	// Summary
	buf.WriteString("Summary:\n")
	buf.WriteString(fmt.Sprintf("  Total: %d, Create: %d, Update: %d, Delete: %d, Skip: %d, Errors: %d\n",
		r.Summary.TotalOperations, r.Summary.Creates, r.Summary.Updates,
		r.Summary.Deletes, r.Summary.Skipped, r.Summary.Errors))

	if len(r.Warnings) > 0 {
		buf.WriteString("\nWarnings:\n")
		for _, w := range r.Warnings {
			buf.WriteString(fmt.Sprintf("  ⚠ %s\n", w))
		}
	}

	return buf.String()
}

// JSON returns the result as JSON
func (r *Result) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

func filterOps(ops []*Operation, opType OperationType) []*Operation {
	var result []*Operation
	for _, op := range ops {
		if op.Type == opType && !op.Skipped && op.Error == "" {
			result = append(result, op)
		}
	}
	return result
}

func filterSkipped(ops []*Operation) []*Operation {
	var result []*Operation
	for _, op := range ops {
		if op.Skipped {
			result = append(result, op)
		}
	}
	return result
}

func filterErrors(ops []*Operation) []*Operation {
	var result []*Operation
	for _, op := range ops {
		if op.Error != "" {
			result = append(result, op)
		}
	}
	return result
}

func formatContent(content interface{}, indent string) string {
	data, err := json.MarshalIndent(content, indent, "  ")
	if err != nil {
		return indent + fmt.Sprintf("%v\n", content)
	}
	lines := strings.Split(string(data), "\n")
	var result strings.Builder
	for _, line := range lines {
		result.WriteString(indent + line + "\n")
	}
	return result.String()
}

// Executor wraps operations with dry-run support
type Executor struct {
	recorder *Recorder
}

// NewExecutor creates a new executor
func NewExecutor(mode Mode) *Executor {
	return &Executor{
		recorder: NewRecorder(mode),
	}
}

// IsDryRun returns true if in dry-run mode
func (e *Executor) IsDryRun() bool {
	return e.recorder.IsDryRun()
}

// Recorder returns the underlying recorder
func (e *Executor) Recorder() *Recorder {
	return e.recorder
}

// Execute executes an operation, recording it if in dry-run mode
func (e *Executor) Execute(op *Operation, fn func() error) error {
	e.recorder.Record(op)

	if e.IsDryRun() {
		return nil
	}

	err := fn()
	if err != nil {
		op.Error = err.Error()
	}
	return err
}

// Result returns the execution result
func (e *Executor) Result() *Result {
	return e.recorder.Result()
}
