// Package state provides HTTP handlers for state management REST API endpoints.
package state

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/shawnbutts/keystone-core/internal/statemgmt"
	"github.com/shawnbutts/keystone-core/pkg/api/apierror"
)

// Handler provides HTTP handlers for state API endpoints.
type Handler struct {
	executor *statemgmt.Executor
	differ   *statemgmt.StateDiffer
}

// NewHandler creates a new state API handler.
func NewHandler() *Handler {
	executor := statemgmt.NewExecutor()
	differ := statemgmt.NewStateDiffer()

	return &Handler{
		executor: executor,
		differ:   differ,
	}
}

// RegisterRoutes registers the state API routes with the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/state/apply", h.handleApply)
	mux.HandleFunc("/api/v1/state/check", h.handleCheck)
	mux.HandleFunc("/api/v1/state/drift", h.handleDrift)
}

// Request represents the request body for state operations.
type Request struct {
	// State file content (YAML)
	Content string `json:"content,omitempty"`

	// State file path (local file on server)
	Path string `json:"path,omitempty"`

	// Variables to pass to templates
	Vars map[string]interface{} `json:"vars,omitempty"`

	// DryRun mode - check without applying
	DryRun bool `json:"dry_run,omitempty"`

	// Target specific states by ID
	Targets []string `json:"targets,omitempty"`
}

// ApplyResponse represents the response for POST /api/v1/state/apply.
type ApplyResponse struct {
	RunID     string    `json:"run_id"`
	Status    string    `json:"status"`
	Summary   *Summary  `json:"summary"`
	Results   []Result  `json:"results"`
	StartedAt time.Time `json:"started_at"`
	Duration  string    `json:"duration"`
	DryRun    bool      `json:"dry_run"`
}

// Summary provides summary statistics for a state run.
type Summary struct {
	Total     int `json:"total"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Changed   int `json:"changed"`
	Unchanged int `json:"unchanged"`
	Skipped   int `json:"skipped"`
}

// Result represents the result of a single state application.
type Result struct {
	ID       string `json:"id"`
	Module   string `json:"module"`
	Status   string `json:"status"`
	Changed  bool   `json:"changed"`
	Comment  string `json:"comment,omitempty"`
	Error    string `json:"error,omitempty"`
	Duration string `json:"duration,omitempty"`
}

// CheckResponse represents the response for POST /api/v1/state/check.
type CheckResponse struct {
	Valid    bool              `json:"valid"`
	Errors   []ValidationError `json:"errors,omitempty"`
	Warnings []ValidationError `json:"warnings,omitempty"`
	States   int               `json:"states"`
	Modules  []string          `json:"modules"`
}

// ValidationError represents a validation error or warning.
type ValidationError struct {
	StateID string `json:"state_id,omitempty"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
	Line    int    `json:"line,omitempty"`
}

// DriftResponse represents the response for POST /api/v1/state/drift.
type DriftResponse struct {
	RunID     string        `json:"run_id"`
	HasDrift  bool          `json:"has_drift"`
	Summary   *DriftSummary `json:"summary"`
	States    []DriftState  `json:"states"`
	CheckedAt time.Time     `json:"checked_at"`
	Duration  string        `json:"duration"`
}

// DriftSummary provides summary statistics for drift detection.
type DriftSummary struct {
	Total    int `json:"total"`
	NoDrift  int `json:"no_drift"`
	Low      int `json:"low"`
	Medium   int `json:"medium"`
	High     int `json:"high"`
	Critical int `json:"critical"`
}

// DriftState represents drift status for a single state.
type DriftState struct {
	StateID     string      `json:"state_id"`
	Module      string      `json:"module"`
	HasDrift    bool        `json:"has_drift"`
	Severity    string      `json:"severity"`
	Differences []DriftDiff `json:"differences,omitempty"`
}

// DriftDiff represents a single drift difference.
type DriftDiff struct {
	Path     string      `json:"path"`
	Expected interface{} `json:"expected"`
	Actual   interface{} `json:"actual"`
	Severity string      `json:"severity"`
	Message  string      `json:"message,omitempty"`
}

// handleApply handles POST /api/v1/state/apply
func (h *Handler) handleApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	// Load state file
	stateFile, err := h.loadStateFile(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Failed to load state: "+err.Error())
		return
	}

	// Set dry-run mode directly on executor
	h.executor.DryRun = req.DryRun

	// Execute state
	ctx := r.Context()
	run, err := h.executor.ExecuteState(ctx, stateFile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to execute state: "+err.Error())
		return
	}

	// Build response
	resp := ApplyResponse{
		RunID:     run.RunID,
		Status:    stateRunStatusToString(run),
		Summary:   convertSummary(run.Summary),
		Results:   convertResults(run.Results),
		StartedAt: run.StartTime,
		Duration:  run.Summary.Duration.String(),
		DryRun:    req.DryRun,
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleCheck handles POST /api/v1/state/check
func (h *Handler) handleCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	// Load state file
	stateFile, err := h.loadStateFile(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Failed to load state: "+err.Error())
		return
	}

	// Validate state file
	validator := statemgmt.NewValidator()
	result := validator.Validate(stateFile)

	// Collect unique modules
	moduleSet := make(map[string]bool)
	for module := range stateFile.States {
		moduleSet[module] = true
	}
	modules := make([]string, 0, len(moduleSet))
	for m := range moduleSet {
		modules = append(modules, m)
	}

	// Convert validation issues to errors/warnings
	var errors, warnings []ValidationError
	for _, issue := range result.Issues {
		ve := ValidationError{
			StateID: issue.StateID,
			Field:   issue.Field,
			Message: issue.Message,
			Line:    issue.Line,
		}
		switch issue.Level {
		case statemgmt.ValidationLevelError:
			errors = append(errors, ve)
		case statemgmt.ValidationLevelWarning:
			warnings = append(warnings, ve)
		default:
		}
	}

	// Build response
	resp := CheckResponse{
		Valid:    result.Valid,
		Errors:   errors,
		Warnings: warnings,
		States:   len(stateFile.States),
		Modules:  modules,
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleDrift handles POST /api/v1/state/drift
func (h *Handler) handleDrift(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	// Load state file
	stateFile, err := h.loadStateFile(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Failed to load state: "+err.Error())
		return
	}

	// Check drift
	report, err := h.differ.CheckDrift(stateFile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to check drift: "+err.Error())
		return
	}

	// Build response
	resp := DriftResponse{
		RunID:     report.RunID,
		HasDrift:  report.Summary.Total > report.Summary.NoDrift,
		Summary:   convertDriftSummary(report.Summary),
		States:    convertDriftStates(report.States),
		CheckedAt: report.CheckedAt,
		Duration:  report.Duration.String(),
	}

	writeJSON(w, http.StatusOK, resp)
}

// Helper functions

func (h *Handler) loadStateFile(req Request) (*statemgmt.StateFile, error) {
	parser := statemgmt.NewParser(".")

	if req.Content != "" {
		// Parse from content by writing to a temp file
		tmpFile, err := os.CreateTemp("", "kscore-state-*.yaml")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp file: %w", err)
		}
		defer os.Remove(tmpFile.Name())
		defer tmpFile.Close()

		if _, err := tmpFile.WriteString(req.Content); err != nil {
			return nil, fmt.Errorf("failed to write temp file: %w", err)
		}
		tmpFile.Close()

		return parser.ParseFile(tmpFile.Name())
	} else if req.Path != "" {
		// Load from file
		return parser.ParseFile(req.Path)
	}
	return nil, fmt.Errorf("either content or path is required")
}

func stateRunStatusToString(run *statemgmt.StateRun) string {
	if run.Summary == nil {
		return "unknown"
	}
	if run.Summary.Failed > 0 {
		return "failed"
	}
	if run.Summary.Changed > 0 {
		return "changed"
	}
	return "success"
}

func convertSummary(s *statemgmt.RunSummary) *Summary {
	if s == nil {
		return nil
	}
	return &Summary{
		Total:     s.Total,
		Succeeded: s.Succeeded,
		Failed:    s.Failed,
		Changed:   s.Changed,
		Unchanged: s.Unchanged,
		Skipped:   0, // RunSummary doesn't have Skipped
	}
}

func convertResults(results []*statemgmt.StateResult) []Result {
	out := make([]Result, 0, len(results))
	for _, r := range results {
		errStr := ""
		if r.Error != nil {
			errStr = r.Error.Error()
		}
		status := "success"
		if !r.Success {
			status = "failed"
		}
		out = append(out, Result{
			ID:       r.StateID,
			Module:   r.Module,
			Status:   status,
			Changed:  r.Changed,
			Comment:  r.Comment,
			Error:    errStr,
			Duration: r.Duration.String(),
		})
	}
	return out
}

func convertDriftSummary(s *statemgmt.DriftSummary) *DriftSummary {
	if s == nil {
		return nil
	}
	return &DriftSummary{
		Total:    s.Total,
		NoDrift:  s.NoDrift,
		Low:      s.LowDrift,
		Medium:   s.MediumDrift,
		High:     s.HighDrift,
		Critical: s.CriticalDrift,
	}
}

func convertDriftStates(states []*statemgmt.DriftStatus) []DriftState {
	out := make([]DriftState, 0, len(states))
	for _, s := range states {
		out = append(out, DriftState{
			StateID:     s.StateID,
			Module:      s.Module,
			HasDrift:    s.HasDrift,
			Severity:    string(s.Severity),
			Differences: convertDifferences(s.Differences),
		})
	}
	return out
}

func convertDifferences(diffs []statemgmt.Difference) []DriftDiff {
	out := make([]DriftDiff, 0, len(diffs))
	for _, d := range diffs {
		out = append(out, DriftDiff{
			Path:     d.Path,
			Expected: d.Desired,
			Actual:   d.Actual,
			Severity: string(d.Severity),
			Message:  d.Message,
		})
	}
	return out
}

func parseStateID(id string) []string {
	// State IDs are typically "module.name" format
	for i := 0; i < len(id); i++ {
		if id[i] == '.' {
			return []string{id[:i], id[i+1:]}
		}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	apierror.Write(w, status, message)
}
