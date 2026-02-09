package dryrun

import (
	"errors"
	"strings"
	"testing"
)

func TestMode_String(t *testing.T) {
	tests := []struct {
		mode     Mode
		expected string
	}{
		{ModeExecute, "execute"},
		{ModeDryRun, "dry-run"},
		{ModePreview, "preview"},
		{Mode(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.mode.String(); got != tt.expected {
			t.Errorf("Mode(%d).String() = %s, want %s", tt.mode, got, tt.expected)
		}
	}
}

func TestNewRecorder(t *testing.T) {
	r := NewRecorder(ModeDryRun)
	if r.Mode() != ModeDryRun {
		t.Errorf("Mode() = %v, want %v", r.Mode(), ModeDryRun)
	}
	if !r.IsDryRun() {
		t.Error("IsDryRun() should be true for ModeDryRun")
	}
}

func TestRecorder_IsDryRun(t *testing.T) {
	tests := []struct {
		mode     Mode
		expected bool
	}{
		{ModeExecute, false},
		{ModeDryRun, true},
		{ModePreview, true},
	}

	for _, tt := range tests {
		r := NewRecorder(tt.mode)
		if got := r.IsDryRun(); got != tt.expected {
			t.Errorf("Mode %v: IsDryRun() = %v, want %v", tt.mode, got, tt.expected)
		}
	}
}

func TestRecorder_RecordCreate(t *testing.T) {
	r := NewRecorder(ModeDryRun)

	content := map[string]string{"key": "value"}
	op := r.RecordCreate("file", "/path/to/file", content)

	if op.Type != OpCreate {
		t.Errorf("Type = %v, want %v", op.Type, OpCreate)
	}
	if op.Resource != "file" {
		t.Errorf("Resource = %s, want file", op.Resource)
	}
	if op.Target != "/path/to/file" {
		t.Errorf("Target = %s, want /path/to/file", op.Target)
	}

	result := r.Result()
	if len(result.Operations) != 1 {
		t.Errorf("Operations count = %d, want 1", len(result.Operations))
	}
	if result.Summary.Creates != 1 {
		t.Errorf("Summary.Creates = %d, want 1", result.Summary.Creates)
	}
}

func TestRecorder_RecordUpdate(t *testing.T) {
	r := NewRecorder(ModeDryRun)

	changes := []Change{
		{Field: "name", OldValue: "old", NewValue: "new", Action: "set"},
	}
	op := r.RecordUpdate("config", "/etc/config", "old content", "new content", changes)

	if op.Type != OpUpdate {
		t.Errorf("Type = %v, want %v", op.Type, OpUpdate)
	}
	if len(op.Changes) != 1 {
		t.Errorf("Changes count = %d, want 1", len(op.Changes))
	}

	result := r.Result()
	if result.Summary.Updates != 1 {
		t.Errorf("Summary.Updates = %d, want 1", result.Summary.Updates)
	}
}

func TestRecorder_RecordDelete(t *testing.T) {
	r := NewRecorder(ModeDryRun)

	op := r.RecordDelete("file", "/path/to/file", "content")

	if op.Type != OpDelete {
		t.Errorf("Type = %v, want %v", op.Type, OpDelete)
	}

	result := r.Result()
	if result.Summary.Deletes != 1 {
		t.Errorf("Summary.Deletes = %d, want 1", result.Summary.Deletes)
	}
}

func TestRecorder_RecordSkip(t *testing.T) {
	r := NewRecorder(ModeDryRun)

	op := r.RecordSkip("file", "/path/to/file", "already exists")

	if !op.Skipped {
		t.Error("Skipped should be true")
	}
	if op.SkipReason != "already exists" {
		t.Errorf("SkipReason = %s, want 'already exists'", op.SkipReason)
	}

	result := r.Result()
	if result.Summary.Skipped != 1 {
		t.Errorf("Summary.Skipped = %d, want 1", result.Summary.Skipped)
	}
}

func TestRecorder_RecordError(t *testing.T) {
	r := NewRecorder(ModeDryRun)

	err := errors.New("permission denied")
	op := r.RecordError("file", "/path/to/file", OpCreate, err)

	if op.Error != "permission denied" {
		t.Errorf("Error = %s, want 'permission denied'", op.Error)
	}

	result := r.Result()
	if result.Summary.Errors != 1 {
		t.Errorf("Summary.Errors = %d, want 1", result.Summary.Errors)
	}
}

func TestRecorder_AddWarning(t *testing.T) {
	r := NewRecorder(ModeDryRun)

	r.AddWarning("This might take a while")

	result := r.Result()
	if len(result.Warnings) != 1 {
		t.Errorf("Warnings count = %d, want 1", len(result.Warnings))
	}
}

func TestRecorder_Clear(t *testing.T) {
	r := NewRecorder(ModeDryRun)
	r.RecordCreate("file", "/path", nil)
	r.AddWarning("warning")

	r.Clear()

	result := r.Result()
	if len(result.Operations) != 0 {
		t.Error("Operations should be empty after Clear")
	}
	if len(result.Warnings) != 0 {
		t.Error("Warnings should be empty after Clear")
	}
}

func TestRecorder_Summary(t *testing.T) {
	r := NewRecorder(ModeDryRun)

	r.RecordCreate("file", "/path1", nil)
	r.RecordCreate("file", "/path2", nil)
	r.RecordUpdate("config", "/etc/config", nil, nil, nil)
	r.RecordDelete("file", "/path3", nil)
	r.RecordSkip("file", "/path4", "unchanged")
	r.RecordError("file", "/path5", OpCreate, errors.New("error"))

	result := r.Result()
	summary := result.Summary

	if summary.TotalOperations != 6 {
		t.Errorf("TotalOperations = %d, want 6", summary.TotalOperations)
	}
	if summary.Creates != 2 {
		t.Errorf("Creates = %d, want 2", summary.Creates)
	}
	if summary.Updates != 1 {
		t.Errorf("Updates = %d, want 1", summary.Updates)
	}
	if summary.Deletes != 1 {
		t.Errorf("Deletes = %d, want 1", summary.Deletes)
	}
	if summary.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", summary.Skipped)
	}
	if summary.Errors != 1 {
		t.Errorf("Errors = %d, want 1", summary.Errors)
	}
	if summary.NoChanges {
		t.Error("NoChanges should be false")
	}
}

func TestRecorder_NoChanges(t *testing.T) {
	r := NewRecorder(ModeDryRun)
	r.RecordSkip("file", "/path", "unchanged")

	result := r.Result()
	if !result.Summary.NoChanges {
		t.Error("NoChanges should be true when only skipped operations")
	}
}

func TestResult_Format(t *testing.T) {
	r := NewRecorder(ModeDryRun)
	r.RecordCreate("file", "/path/new.txt", "content")
	r.RecordUpdate("config", "/etc/app.conf", "old", "new", []Change{
		{Field: "setting", OldValue: "a", NewValue: "b", Action: "set"},
	})
	r.RecordDelete("file", "/path/old.txt", nil)
	r.RecordSkip("file", "/path/same.txt", "unchanged")
	r.AddWarning("Test warning")

	result := r.Result()
	output := result.Format(true)

	if !strings.Contains(output, "Would CREATE 1") {
		t.Error("Output should contain create info")
	}
	if !strings.Contains(output, "Would UPDATE 1") {
		t.Error("Output should contain update info")
	}
	if !strings.Contains(output, "Would DELETE 1") {
		t.Error("Output should contain delete info")
	}
	if !strings.Contains(output, "Would SKIP 1") {
		t.Error("Output should contain skip info")
	}
	if !strings.Contains(output, "Test warning") {
		t.Error("Output should contain warnings")
	}
}

func TestResult_Format_Empty(t *testing.T) {
	r := NewRecorder(ModeDryRun)
	result := r.Result()
	output := result.Format(false)

	if !strings.Contains(output, "No operations") {
		t.Error("Output should indicate no operations")
	}
}

func TestResult_JSON(t *testing.T) {
	r := NewRecorder(ModeDryRun)
	r.RecordCreate("file", "/path", nil)

	result := r.Result()
	data, err := result.JSON()
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}

	if !strings.Contains(string(data), "operations") {
		t.Error("JSON should contain operations")
	}
	if !strings.Contains(string(data), "summary") {
		t.Error("JSON should contain summary")
	}
}

func TestExecutor_DryRun(t *testing.T) {
	exec := NewExecutor(ModeDryRun)

	executed := false
	op := &Operation{
		Type:     OpCreate,
		Resource: "file",
		Target:   "/path",
	}

	err := exec.Execute(op, func() error {
		executed = true
		return nil
	})

	if err != nil {
		t.Errorf("Execute() error: %v", err)
	}
	if executed {
		t.Error("Function should not be executed in dry-run mode")
	}

	result := exec.Result()
	if len(result.Operations) != 1 {
		t.Error("Operation should be recorded")
	}
}

func TestExecutor_Execute(t *testing.T) {
	exec := NewExecutor(ModeExecute)

	executed := false
	op := &Operation{
		Type:     OpCreate,
		Resource: "file",
		Target:   "/path",
	}

	err := exec.Execute(op, func() error {
		executed = true
		return nil
	})

	if err != nil {
		t.Errorf("Execute() error: %v", err)
	}
	if !executed {
		t.Error("Function should be executed in execute mode")
	}
}

func TestExecutor_ExecuteError(t *testing.T) {
	exec := NewExecutor(ModeExecute)

	expectedErr := errors.New("test error")
	op := &Operation{
		Type:     OpCreate,
		Resource: "file",
		Target:   "/path",
	}

	err := exec.Execute(op, func() error {
		return expectedErr
	})

	if !errors.Is(err, expectedErr) {
		t.Errorf("Execute() should return the function error")
	}
	if op.Error != "test error" {
		t.Error("Operation should have error recorded")
	}
}

func TestOperationType(t *testing.T) {
	types := []OperationType{
		OpCreate, OpUpdate, OpDelete, OpReplace,
		OpAppend, OpMove, OpCopy, OpChmod, OpChown,
		OpLink, OpExecute, OpDownload, OpUpload,
	}

	for _, opType := range types {
		if opType == "" {
			t.Errorf("OperationType should not be empty")
		}
	}
}
