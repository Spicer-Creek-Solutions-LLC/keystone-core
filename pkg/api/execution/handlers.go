// Package execution provides HTTP handlers for execution REST API endpoints.
package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"github.com/shawnbutts/keystone-core/internal/controlplane"
	"github.com/shawnbutts/keystone-core/internal/state"
)

// Handler provides HTTP handlers for execution API endpoints.
type Handler struct {
	dispatcher *controlplane.CommandDispatcher
	connMgr    *controlplane.ConnectionManager
}

// NewHandler creates a new execution API handler.
func NewHandler(dispatcher *controlplane.CommandDispatcher, connMgr *controlplane.ConnectionManager) *Handler {
	return &Handler{
		dispatcher: dispatcher,
		connMgr:    connMgr,
	}
}

// RegisterRoutes registers the execution API routes with the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/exec", h.handleExec)
	mux.HandleFunc("/api/v1/jobs", h.handleJobs)
	mux.HandleFunc("/api/v1/jobs/", h.handleJob)
}

// ExecuteRequest represents the request body for POST /api/v1/exec
type ExecuteRequest struct {
	// Target agent IDs or targeting expression
	Targets []string `json:"targets,omitempty"`
	Target  string   `json:"target,omitempty"` // Single target or expression

	// Command to execute
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`

	// Execution options
	Env        map[string]string `json:"env,omitempty"`
	WorkingDir string            `json:"working_dir,omitempty"`
	User       string            `json:"user,omitempty"`
	Shell      string            `json:"shell,omitempty"`
	Timeout    int32             `json:"timeout,omitempty"` // seconds

	// Async execution
	Async bool `json:"async,omitempty"`
}

// ExecuteResponse represents the response for POST /api/v1/exec
type ExecuteResponse struct {
	JobID     string            `json:"job_id"`
	Status    string            `json:"status"`
	Targets   []string          `json:"targets"`
	Results   map[string]Result `json:"results,omitempty"` // agent_id -> result (for sync)
	CreatedAt time.Time         `json:"created_at"`
}

// Result represents a command execution result
type Result struct {
	AgentID     string    `json:"agent_id"`
	Status      string    `json:"status"`
	ExitCode    int32     `json:"exit_code"`
	Stdout      string    `json:"stdout,omitempty"`
	Stderr      string    `json:"stderr,omitempty"`
	Error       string    `json:"error,omitempty"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	DurationMs  int64     `json:"duration_ms,omitempty"`
}

// JobResponse represents a job in API responses
type JobResponse struct {
	ID          string    `json:"id"`
	AgentID     string    `json:"agent_id"`
	Command     string    `json:"command"`
	Args        []string  `json:"args,omitempty"`
	Status      string    `json:"status"`
	ExitCode    int32     `json:"exit_code,omitempty"`
	Stdout      string    `json:"stdout,omitempty"`
	Stderr      string    `json:"stderr,omitempty"`
	Error       string    `json:"error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	DurationMs  int64     `json:"duration_ms,omitempty"`
}

// JobListResponse represents the response for GET /api/v1/jobs
type JobListResponse struct {
	Jobs        []JobResponse `json:"jobs"`
	Total       int           `json:"total"`
	Limit       int           `json:"limit"`
	Offset      int           `json:"offset"`
	RetrievedAt time.Time     `json:"retrieved_at"`
}

// handleExec handles POST /api/v1/exec
func (h *Handler) handleExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	// Validate request
	if req.Command == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}

	// Determine targets
	targets := req.Targets
	if len(targets) == 0 && req.Target != "" {
		targets = []string{req.Target}
	}
	if len(targets) == 0 {
		writeError(w, http.StatusBadRequest, "at least one target is required")
		return
	}

	// Set default timeout
	timeout := req.Timeout
	if timeout == 0 {
		timeout = 60 // 60 seconds default
	}

	// Build protobuf request
	pbReq := &pb.ExecuteCommandRequest{
		Command:    req.Command,
		Args:       req.Args,
		Env:        req.Env,
		WorkingDir: req.WorkingDir,
		User:       req.User,
		Timeout:    timeout,
	}

	// Execute on each target
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeout)*time.Second)
	defer cancel()

	results := make(map[string]Result)
	var jobID string
	var overallStatus string = "pending"

	for _, target := range targets {
		// Verify target agent exists
		agent, err := h.connMgr.GetAgent(target)
		if err != nil {
			results[target] = Result{
				AgentID: target,
				Status:  "error",
				Error:   fmt.Sprintf("agent not found: %s", target),
			}
			continue
		}

		// Set agent ID for the request
		pbReq.AgentId = agent.ID

		// For async, just dispatch and return
		if req.Async {
			respChan, err := h.dispatcher.ExecuteCommand(ctx, pbReq)
			if err != nil {
				results[target] = Result{
					AgentID: target,
					Status:  "error",
					Error:   err.Error(),
				}
				continue
			}

			// Get first response for job ID
			select {
			case resp := <-respChan:
				jobID = resp.CommandId
				results[target] = Result{
					AgentID: target,
					Status:  "dispatched",
				}
				// Drain remaining responses in background
				go func() {
					for range respChan {
					}
				}()
			case <-ctx.Done():
				results[target] = Result{
					AgentID: target,
					Status:  "timeout",
					Error:   "timed out waiting for response",
				}
			}
			overallStatus = "dispatched"
		} else {
			// Synchronous execution - wait for completion
			respChan, err := h.dispatcher.ExecuteCommand(ctx, pbReq)
			if err != nil {
				results[target] = Result{
					AgentID: target,
					Status:  "error",
					Error:   err.Error(),
				}
				continue
			}

			// Collect all responses - aggregate stdout/stderr
			var stdout, stderr strings.Builder
			var finalStatus string = "running"
			var exitCode int32
			var errMsg string
			var completedAt time.Time

			for resp := range respChan {
				jobID = resp.CommandId

				switch resp.Type {
				case pb.CommandResponseType_COMMAND_RESPONSE_TYPE_STDOUT:
					stdout.Write(resp.Data)
				case pb.CommandResponseType_COMMAND_RESPONSE_TYPE_STDERR:
					stderr.Write(resp.Data)
				case pb.CommandResponseType_COMMAND_RESPONSE_TYPE_COMPLETED:
					finalStatus = "success"
					exitCode = resp.ExitCode
					if resp.Timestamp != nil {
						completedAt = resp.Timestamp.AsTime()
					}
				case pb.CommandResponseType_COMMAND_RESPONSE_TYPE_FAILED:
					finalStatus = "failed"
					exitCode = resp.ExitCode
					errMsg = resp.Error
					if resp.Timestamp != nil {
						completedAt = resp.Timestamp.AsTime()
					}
				case pb.CommandResponseType_COMMAND_RESPONSE_TYPE_TIMEOUT:
					finalStatus = "timeout"
					errMsg = "command timed out"
					if resp.Timestamp != nil {
						completedAt = resp.Timestamp.AsTime()
					}
				}
			}

			results[target] = Result{
				AgentID:     target,
				Status:      finalStatus,
				ExitCode:    exitCode,
				Stdout:      stdout.String(),
				Stderr:      stderr.String(),
				Error:       errMsg,
				CompletedAt: completedAt,
			}

			if finalStatus == "success" {
				overallStatus = "completed"
			} else if finalStatus == "failed" || finalStatus == "timeout" {
				overallStatus = "failed"
			}
		}
	}

	resp := ExecuteResponse{
		JobID:     jobID,
		Status:    overallStatus,
		Targets:   targets,
		Results:   results,
		CreatedAt: time.Now().UTC(),
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleJobs handles GET /api/v1/jobs
func (h *Handler) handleJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	agentID := query.Get("agent_id")
	statusStr := query.Get("status")
	limitStr := query.Get("limit")
	offsetStr := query.Get("offset")
	sortBy := query.Get("sort")
	sortOrder := query.Get("order")

	// Parse limit and offset
	limit := 50 // default
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	offset := 0
	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// Build filter
	filter := &state.CommandFilter{
		AgentID:   agentID,
		Limit:     limit,
		Offset:    offset,
		SortBy:    sortBy,
		SortOrder: sortOrder,
	}

	// Parse status filter
	if statusStr != "" {
		status := stringToCommandStatus(statusStr)
		filter.Status = &status
	}

	// Get commands
	ctx := r.Context()
	commands, err := h.dispatcher.ListCommands(ctx, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list jobs: "+err.Error())
		return
	}

	// Convert to response
	jobs := make([]JobResponse, 0, len(commands))
	for _, cmd := range commands {
		job := commandToJobResponse(cmd)
		jobs = append(jobs, job)
	}

	resp := JobListResponse{
		Jobs:        jobs,
		Total:       len(jobs),
		Limit:       limit,
		Offset:      offset,
		RetrievedAt: time.Now().UTC(),
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleJob handles GET /api/v1/jobs/{id}
func (h *Handler) handleJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract job ID from path
	jobID := strings.TrimPrefix(r.URL.Path, "/api/v1/jobs/")
	if jobID == "" {
		writeError(w, http.StatusBadRequest, "Job ID required")
		return
	}

	// Get job
	ctx := r.Context()
	cmd, err := h.dispatcher.GetCommand(ctx, jobID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Job not found")
		return
	}

	resp := commandToJobResponse(cmd)
	writeJSON(w, http.StatusOK, resp)
}

// Helper functions

func commandToJobResponse(cmd *state.CommandRecord) JobResponse {
	job := JobResponse{
		ID:         cmd.ID,
		AgentID:    cmd.AgentID,
		Command:    cmd.Command,
		Args:       cmd.Args,
		Status:     commandStatusToString(cmd.Status),
		ExitCode:   cmd.ExitCode,
		Stdout:     cmd.Stdout,
		Stderr:     cmd.Stderr,
		Error:      cmd.Error,
		CreatedAt:  cmd.CreatedAt,
		DurationMs: cmd.DurationMs,
	}

	if cmd.StartedAt != nil {
		job.StartedAt = *cmd.StartedAt
	}
	if cmd.CompletedAt != nil {
		job.CompletedAt = *cmd.CompletedAt
	}

	return job
}

func commandStatusToString(status pb.CommandStatus) string {
	switch status {
	case pb.CommandStatus_COMMAND_STATUS_PENDING:
		return "pending"
	case pb.CommandStatus_COMMAND_STATUS_RUNNING:
		return "running"
	case pb.CommandStatus_COMMAND_STATUS_COMPLETED:
		return "success"
	case pb.CommandStatus_COMMAND_STATUS_FAILED:
		return "failed"
	case pb.CommandStatus_COMMAND_STATUS_TIMEOUT:
		return "timeout"
	case pb.CommandStatus_COMMAND_STATUS_CANCELLED:
		return "cancelled"
	default:
		return "unknown"
	}
}

func stringToCommandStatus(s string) pb.CommandStatus {
	switch strings.ToLower(s) {
	case "pending":
		return pb.CommandStatus_COMMAND_STATUS_PENDING
	case "running":
		return pb.CommandStatus_COMMAND_STATUS_RUNNING
	case "success", "completed":
		return pb.CommandStatus_COMMAND_STATUS_COMPLETED
	case "failed":
		return pb.CommandStatus_COMMAND_STATUS_FAILED
	case "timeout":
		return pb.CommandStatus_COMMAND_STATUS_TIMEOUT
	case "cancelled":
		return pb.CommandStatus_COMMAND_STATUS_CANCELLED
	default:
		return pb.CommandStatus_COMMAND_STATUS_PENDING
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
