// Package schedule provides HTTP handlers for schedule REST API endpoints.
package schedule

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/schedule"
	"github.com/shawnbutts/keystone-core/pkg/api/apierror"
)

// Manager defines the schedule management interface used by handlers.
type Manager interface {
	Create(ctx context.Context, s *schedule.Schedule) error
	Get(ctx context.Context, id string) (*schedule.Schedule, error)
	Update(ctx context.Context, s *schedule.Schedule) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, filter *schedule.Filter) ([]*schedule.Schedule, error)
	Pause(ctx context.Context, id, by string) error
	Resume(ctx context.Context, id, by string) error
	Enable(ctx context.Context, id, by string) error
	Disable(ctx context.Context, id, by string) error
	TriggerNow(ctx context.Context, id, triggeredBy string) (*schedule.Execution, error)
	ListExecutions(ctx context.Context, filter *schedule.ExecutionFilter) ([]*schedule.Execution, error)
}

// MaintenanceManager defines the maintenance window management interface used by handlers.
type MaintenanceManager interface {
	Create(ctx context.Context, window *schedule.MaintenanceWindow) error
	Get(ctx context.Context, id string) (*schedule.MaintenanceWindow, error)
	Update(ctx context.Context, window *schedule.MaintenanceWindow) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, filter *schedule.MaintenanceWindowFilter) ([]*schedule.MaintenanceWindow, error)
	Start(ctx context.Context, id string) error
	End(ctx context.Context, id string) error
	Cancel(ctx context.Context, id, cancelledBy, reason string) error
	Extend(ctx context.Context, id string, newEndTime time.Time, extendedBy string) error
	GetActiveWindows(ctx context.Context) ([]*schedule.MaintenanceWindow, error)
	GetUpcomingWindows(ctx context.Context, within time.Duration) ([]*schedule.MaintenanceWindow, error)
	GetConflicts(ctx context.Context, window *schedule.MaintenanceWindow) ([]*schedule.MaintenanceConflict, error)
}

// Handler provides HTTP handlers for schedule API endpoints.
type Handler struct {
	schedMgr Manager
	maintMgr MaintenanceManager
}

// NewHandler creates a new schedule API handler.
func NewHandler(schedMgr Manager, maintMgr MaintenanceManager) *Handler {
	return &Handler{schedMgr: schedMgr, maintMgr: maintMgr}
}

// RegisterRoutes registers the schedule API routes with the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/schedules", h.handleSchedules)
	mux.HandleFunc("/api/v1/schedules/", h.handleSchedule)
	mux.HandleFunc("/api/v1/maintenance/windows", h.handleWindows)
	mux.HandleFunc("/api/v1/maintenance/windows/", h.handleWindow)
}

// Schedule response types

// Response represents a schedule in API responses.
type Response struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Description     string            `json:"description,omitempty"`
	Type            string            `json:"type"`
	Status          string            `json:"status"`
	Cron            string            `json:"cron,omitempty"`
	Interval        string            `json:"interval,omitempty"`
	Timezone        string            `json:"timezone,omitempty"`
	Priority        int               `json:"priority"`
	Timeout         string            `json:"timeout,omitempty"`
	NextRun         *time.Time        `json:"next_run,omitempty"`
	LastRun         *time.Time        `json:"last_run,omitempty"`
	RunCount        int64             `json:"run_count"`
	SuccessCount    int64             `json:"success_count"`
	FailureCount    int64             `json:"failure_count"`
	RequireApproval bool              `json:"require_approval"`
	Labels          map[string]string `json:"labels,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	CreatedBy       string            `json:"created_by,omitempty"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// ListResponse represents the list schedules API response.
type ListResponse struct {
	Schedules []Response `json:"schedules"`
	Total     int                `json:"total"`
}

// ExecutionResponse represents an execution in API responses.
type ExecutionResponse struct {
	ID           string     `json:"id"`
	ScheduleID   string     `json:"schedule_id"`
	ScheduleName string     `json:"schedule_name"`
	Status       string     `json:"status"`
	TriggerType  string     `json:"trigger_type"`
	TriggeredBy  string     `json:"triggered_by,omitempty"`
	StartTime    *time.Time `json:"start_time,omitempty"`
	EndTime      *time.Time `json:"end_time,omitempty"`
	Duration     string     `json:"duration,omitempty"`
	SuccessCount int        `json:"success_count"`
	FailureCount int        `json:"failure_count"`
	CreatedAt    time.Time  `json:"created_at"`
}

// ExecutionListResponse represents the list executions API response.
type ExecutionListResponse struct {
	Executions []ExecutionResponse `json:"executions"`
	Total      int                 `json:"total"`
}

// WindowResponse represents a maintenance window in API responses.
type WindowResponse struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Description     string            `json:"description,omitempty"`
	Type            string            `json:"type"`
	Status          string            `json:"status"`
	StartTime       time.Time         `json:"start_time"`
	EndTime         time.Time         `json:"end_time"`
	Timezone        string            `json:"timezone,omitempty"`
	RequireApproval bool              `json:"require_approval"`
	Labels          map[string]string `json:"labels,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	CreatedBy       string            `json:"created_by,omitempty"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// WindowListResponse represents the list windows API response.
type WindowListResponse struct {
	Windows []WindowResponse `json:"windows"`
	Total   int              `json:"total"`
}

// ConflictResponse represents a maintenance conflict in API responses.
type ConflictResponse struct {
	WindowID       string `json:"window_id"`
	WindowName     string `json:"window_name"`
	ConflictType   string `json:"conflict_type"`
	ConflictingID  string `json:"conflicting_id"`
	Severity       string `json:"severity"`
	Message        string `json:"message"`
}

// CreateScheduleRequest represents the request body for creating a schedule.
type CreateScheduleRequest struct {
	Name              string            `json:"name"`
	Description       string            `json:"description,omitempty"`
	Type              string            `json:"type"`
	Cron              string            `json:"cron,omitempty"`
	Interval          string            `json:"interval,omitempty"`
	Timezone          string            `json:"timezone,omitempty"`
	Priority          int               `json:"priority,omitempty"`
	Timeout           string            `json:"timeout,omitempty"`
	RequireApproval   bool              `json:"require_approval,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Payload           json.RawMessage   `json:"payload,omitempty"`
	CreatedBy         string            `json:"created_by,omitempty"`
}

// TriggerRequest represents the request body for triggering a schedule.
type TriggerRequest struct {
	TriggeredBy string `json:"triggered_by,omitempty"`
}

// CreateWindowRequest represents the request body for creating a maintenance window.
type CreateWindowRequest struct {
	Name            string            `json:"name"`
	Description     string            `json:"description,omitempty"`
	Type            string            `json:"type"`
	StartTime       string            `json:"start_time"`
	EndTime         string            `json:"end_time"`
	Timezone        string            `json:"timezone,omitempty"`
	RequireApproval bool              `json:"require_approval,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	CreatedBy       string            `json:"created_by,omitempty"`
}

// CancelWindowRequest represents the request body for cancelling a window.
type CancelWindowRequest struct {
	CancelledBy string `json:"cancelled_by,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

// ExtendWindowRequest represents the request body for extending a window.
type ExtendWindowRequest struct {
	EndTime    string `json:"end_time,omitempty"`
	Duration   string `json:"duration,omitempty"`
	ExtendedBy string `json:"extended_by,omitempty"`
}

// Schedule handlers

func (h *Handler) handleSchedules(w http.ResponseWriter, r *http.Request) {
	if h.schedMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "Schedule service requires cluster mode")
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.listSchedules(w, r)
	case http.MethodPost:
		h.createSchedule(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleSchedule(w http.ResponseWriter, r *http.Request) {
	if h.schedMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "Schedule service requires cluster mode")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/schedules/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusBadRequest, "Schedule ID required")
		return
	}

	id := parts[0]

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			h.getSchedule(w, r, id)
		case http.MethodPut:
			h.updateSchedule(w, r, id)
		case http.MethodDelete:
			h.deleteSchedule(w, r, id)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	action := parts[1]
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	switch action {
	case "trigger":
		h.triggerSchedule(w, r, id)
	case "pause":
		h.pauseSchedule(w, r, id)
	case "resume":
		h.resumeSchedule(w, r, id)
	case "enable":
		h.enableSchedule(w, r, id)
	case "disable":
		h.disableSchedule(w, r, id)
	case "history":
		h.getHistory(w, r, id)
	default:
		writeError(w, http.StatusNotFound, "Unknown action: "+action)
	}
}

func (h *Handler) listSchedules(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()

	filter := &schedule.Filter{}
	if t := query.Get("type"); t != "" {
		filter.Type = []schedule.Type{schedule.Type(t)}
	}
	if s := query.Get("status"); s != "" {
		filter.Status = []schedule.Status{schedule.Status(s)}
	}
	if limit := query.Get("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil && l > 0 {
			filter.Limit = l
		}
	}
	if offset := query.Get("offset"); offset != "" {
		if o, err := strconv.Atoi(offset); err == nil && o >= 0 {
			filter.Offset = o
		}
	}

	schedules, err := h.schedMgr.List(ctx, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list schedules: "+err.Error())
		return
	}

	resp := ListResponse{
		Schedules: make([]Response, 0, len(schedules)),
		Total:     len(schedules),
	}
	for _, s := range schedules {
		resp.Schedules = append(resp.Schedules, convertSchedule(s))
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) createSchedule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreateScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	s := &schedule.Schedule{
		Name:            req.Name,
		Description:     req.Description,
		Type:            schedule.Type(req.Type),
		Cron:            req.Cron,
		Interval:        parseDuration(req.Interval),
		Timezone:        req.Timezone,
		Priority:        req.Priority,
		RequireApproval: req.RequireApproval,
		Labels:          req.Labels,
		Payload:         req.Payload,
		CreatedBy:       req.CreatedBy,
	}
	if req.Timeout != "" {
		if d, err := time.ParseDuration(req.Timeout); err == nil {
			s.Timeout = d
		}
	}

	if err := h.schedMgr.Create(ctx, s); err != nil {
		status := mapScheduleError(err)
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, convertSchedule(s))
}

func (h *Handler) getSchedule(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()

	s, err := h.schedMgr.Get(ctx, id)
	if err != nil {
		status := mapScheduleError(err)
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, convertSchedule(s))
}

func (h *Handler) updateSchedule(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()

	var req CreateScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	s := &schedule.Schedule{
		ID:              id,
		Name:            req.Name,
		Description:     req.Description,
		Type:            schedule.Type(req.Type),
		Cron:            req.Cron,
		Interval:        parseDuration(req.Interval),
		Timezone:        req.Timezone,
		Priority:        req.Priority,
		RequireApproval: req.RequireApproval,
		Labels:          req.Labels,
		Payload:         req.Payload,
	}
	if req.Timeout != "" {
		if d, err := time.ParseDuration(req.Timeout); err == nil {
			s.Timeout = d
		}
	}

	if err := h.schedMgr.Update(ctx, s); err != nil {
		status := mapScheduleError(err)
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, convertSchedule(s))
}

func (h *Handler) deleteSchedule(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()

	if err := h.schedMgr.Delete(ctx, id); err != nil {
		status := mapScheduleError(err)
		writeError(w, status, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) triggerSchedule(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()

	var req TriggerRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
			return
		}
	}
	if req.TriggeredBy == "" {
		req.TriggeredBy = "api"
	}

	exec, err := h.schedMgr.TriggerNow(ctx, id, req.TriggeredBy)
	if err != nil {
		status := mapScheduleError(err)
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, convertExecution(exec))
}

func (h *Handler) pauseSchedule(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.schedMgr.Pause(r.Context(), id, "api"); err != nil {
		status := mapScheduleError(err)
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused", "id": id})
}

func (h *Handler) resumeSchedule(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.schedMgr.Resume(r.Context(), id, "api"); err != nil {
		status := mapScheduleError(err)
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed", "id": id})
}

func (h *Handler) enableSchedule(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.schedMgr.Enable(r.Context(), id, "api"); err != nil {
		status := mapScheduleError(err)
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "enabled", "id": id})
}

func (h *Handler) disableSchedule(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.schedMgr.Disable(r.Context(), id, "api"); err != nil {
		status := mapScheduleError(err)
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "disabled", "id": id})
}

func (h *Handler) getHistory(w http.ResponseWriter, r *http.Request, scheduleID string) {
	ctx := r.Context()
	query := r.URL.Query()

	filter := &schedule.ExecutionFilter{ScheduleID: scheduleID}
	if s := query.Get("status"); s != "" {
		filter.Status = []schedule.ExecutionStatus{schedule.ExecutionStatus(s)}
	}
	if limit := query.Get("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil && l > 0 {
			filter.Limit = l
		}
	}

	executions, err := h.schedMgr.ListExecutions(ctx, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list executions: "+err.Error())
		return
	}

	resp := ExecutionListResponse{
		Executions: make([]ExecutionResponse, 0, len(executions)),
		Total:      len(executions),
	}
	for _, e := range executions {
		resp.Executions = append(resp.Executions, convertExecution(e))
	}

	writeJSON(w, http.StatusOK, resp)
}

// Maintenance window handlers

func (h *Handler) handleWindows(w http.ResponseWriter, r *http.Request) {
	if h.maintMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "Maintenance service requires cluster mode")
		return
	}

	// Check for /active suffix on the base path
	if strings.HasSuffix(r.URL.Path, "/active") {
		h.listActiveWindows(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.listWindows(w, r)
	case http.MethodPost:
		h.createWindow(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleWindow(w http.ResponseWriter, r *http.Request) {
	if h.maintMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "Maintenance service requires cluster mode")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/maintenance/windows/")

	// Handle /active path
	if path == "active" || strings.HasPrefix(path, "active/") {
		h.listActiveWindows(w, r)
		return
	}

	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusBadRequest, "Window ID required")
		return
	}

	id := parts[0]

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			h.getWindow(w, r, id)
		case http.MethodDelete:
			h.deleteWindow(w, r, id)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	action := parts[1]

	switch action {
	case "start":
		h.startWindow(w, r, id)
	case "end":
		h.endWindow(w, r, id)
	case "cancel":
		h.cancelWindow(w, r, id)
	case "extend":
		h.extendWindow(w, r, id)
	case "conflicts":
		h.getConflicts(w, r, id)
	default:
		writeError(w, http.StatusNotFound, "Unknown action: "+action)
	}
}

func (h *Handler) listWindows(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()

	filter := &schedule.MaintenanceWindowFilter{}
	if s := query.Get("status"); s != "" {
		filter.Status = []schedule.MaintenanceWindowStatus{schedule.MaintenanceWindowStatus(s)}
	}
	if t := query.Get("type"); t != "" {
		filter.Type = []schedule.MaintenanceWindowType{schedule.MaintenanceWindowType(t)}
	}
	if limit := query.Get("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil && l > 0 {
			filter.Limit = l
		}
	}

	windows, err := h.maintMgr.List(ctx, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list windows: "+err.Error())
		return
	}

	resp := WindowListResponse{
		Windows: make([]WindowResponse, 0, len(windows)),
		Total:   len(windows),
	}
	for _, win := range windows {
		resp.Windows = append(resp.Windows, convertWindow(win))
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) createWindow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreateWindowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid start_time: "+err.Error())
		return
	}
	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid end_time: "+err.Error())
		return
	}

	win := &schedule.MaintenanceWindow{
		Name:            req.Name,
		Description:     req.Description,
		Type:            schedule.MaintenanceWindowType(req.Type),
		StartTime:       startTime,
		EndTime:         endTime,
		Timezone:        req.Timezone,
		RequireApproval: req.RequireApproval,
		Labels:          req.Labels,
		CreatedBy:       req.CreatedBy,
	}

	if err := h.maintMgr.Create(ctx, win); err != nil {
		status := mapMaintenanceError(err)
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, convertWindow(win))
}

func (h *Handler) getWindow(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()

	win, err := h.maintMgr.Get(ctx, id)
	if err != nil {
		status := mapMaintenanceError(err)
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, convertWindow(win))
}

func (h *Handler) deleteWindow(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()

	if err := h.maintMgr.Delete(ctx, id); err != nil {
		status := mapMaintenanceError(err)
		writeError(w, status, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) startWindow(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.maintMgr.Start(r.Context(), id); err != nil {
		status := mapMaintenanceError(err)
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started", "id": id})
}

func (h *Handler) endWindow(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.maintMgr.End(r.Context(), id); err != nil {
		status := mapMaintenanceError(err)
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ended", "id": id})
}

func (h *Handler) cancelWindow(w http.ResponseWriter, r *http.Request, id string) {
	var req CancelWindowRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
			return
		}
	}
	if req.CancelledBy == "" {
		req.CancelledBy = "api"
	}

	if err := h.maintMgr.Cancel(r.Context(), id, req.CancelledBy, req.Reason); err != nil {
		status := mapMaintenanceError(err)
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled", "id": id})
}

func (h *Handler) extendWindow(w http.ResponseWriter, r *http.Request, id string) {
	var req ExtendWindowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	var newEndTime time.Time
	switch {
	case req.EndTime != "":
		t, err := time.Parse(time.RFC3339, req.EndTime)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid end_time: "+err.Error())
			return
		}
		newEndTime = t
	case req.Duration != "":
		d, err := time.ParseDuration(req.Duration)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid duration: "+err.Error())
			return
		}
		win, err := h.maintMgr.Get(r.Context(), id)
		if err != nil {
			status := mapMaintenanceError(err)
			writeError(w, status, err.Error())
			return
		}
		newEndTime = win.EndTime.Add(d)
	default:
		writeError(w, http.StatusBadRequest, "either end_time or duration is required")
		return
	}

	if req.ExtendedBy == "" {
		req.ExtendedBy = "api"
	}

	if err := h.maintMgr.Extend(r.Context(), id, newEndTime, req.ExtendedBy); err != nil {
		status := mapMaintenanceError(err)
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "extended", "id": id})
}

func (h *Handler) listActiveWindows(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	windows, err := h.maintMgr.GetActiveWindows(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list active windows: "+err.Error())
		return
	}

	resp := WindowListResponse{
		Windows: make([]WindowResponse, 0, len(windows)),
		Total:   len(windows),
	}
	for _, win := range windows {
		resp.Windows = append(resp.Windows, convertWindow(win))
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) getConflicts(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()

	win, err := h.maintMgr.Get(ctx, id)
	if err != nil {
		status := mapMaintenanceError(err)
		writeError(w, status, err.Error())
		return
	}

	conflicts, err := h.maintMgr.GetConflicts(ctx, win)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to check conflicts: "+err.Error())
		return
	}

	resp := make([]ConflictResponse, 0, len(conflicts))
	for _, c := range conflicts {
		resp = append(resp, ConflictResponse{
			WindowID:      c.WindowID,
			WindowName:    c.WindowName,
			ConflictType:  string(c.ConflictType),
			ConflictingID: c.ConflictingID,
			Severity:      string(c.Severity),
			Message:       c.Message,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"conflicts": resp, "total": len(resp)})
}

// Conversion helpers

func convertSchedule(s *schedule.Schedule) Response {
	resp := Response{
		ID:              s.ID,
		Name:            s.Name,
		Description:     s.Description,
		Type:            string(s.Type),
		Status:          string(s.Status),
		Cron:            s.Cron,
		Timezone:        s.Timezone,
		Priority:        s.Priority,
		NextRun:         s.NextRun,
		LastRun:         s.LastRun,
		RunCount:        s.RunCount,
		SuccessCount:    s.SuccessCount,
		FailureCount:    s.FailureCount,
		RequireApproval: s.RequireApproval,
		Labels:          s.Labels,
		CreatedAt:       s.CreatedAt,
		CreatedBy:       s.CreatedBy,
		UpdatedAt:       s.UpdatedAt,
	}
	if s.Interval > 0 {
		resp.Interval = s.Interval.String()
	}
	if s.Timeout > 0 {
		resp.Timeout = s.Timeout.String()
	}
	return resp
}

func convertExecution(e *schedule.Execution) ExecutionResponse {
	resp := ExecutionResponse{
		ID:           e.ID,
		ScheduleID:   e.ScheduleID,
		ScheduleName: e.ScheduleName,
		Status:       string(e.Status),
		TriggerType:  string(e.TriggerType),
		TriggeredBy:  e.TriggeredBy,
		SuccessCount: e.SuccessCount,
		FailureCount: e.FailureCount,
		CreatedAt:    e.CreatedAt,
	}
	if e.StartTime != nil {
		resp.StartTime = e.StartTime
	}
	if e.EndTime != nil {
		resp.EndTime = e.EndTime
	}
	if e.Duration > 0 {
		resp.Duration = e.Duration.String()
	}
	return resp
}

func convertWindow(w *schedule.MaintenanceWindow) WindowResponse {
	return WindowResponse{
		ID:              w.ID,
		Name:            w.Name,
		Description:     w.Description,
		Type:            string(w.Type),
		Status:          string(w.Status),
		StartTime:       w.StartTime,
		EndTime:         w.EndTime,
		Timezone:        w.Timezone,
		RequireApproval: w.RequireApproval,
		Labels:          w.Labels,
		CreatedAt:       w.CreatedAt,
		CreatedBy:       w.CreatedBy,
		UpdatedAt:       w.UpdatedAt,
	}
}

// Error mapping

func mapScheduleError(err error) int {
	if errors.Is(err, schedule.ErrScheduleNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, schedule.ErrExecutionNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, schedule.ErrScheduleExists) {
		return http.StatusConflict
	}
	if errors.Is(err, schedule.ErrScheduleDisabled) {
		return http.StatusConflict
	}
	if errors.Is(err, schedule.ErrInvalidSchedule) || errors.Is(err, schedule.ErrInvalidCron) {
		return http.StatusBadRequest
	}
	if errors.Is(err, schedule.ErrStoreClosed) || errors.Is(err, schedule.ErrStoreNotConnected) {
		return http.StatusServiceUnavailable
	}
	return http.StatusInternalServerError
}

func mapMaintenanceError(err error) int {
	if errors.Is(err, schedule.ErrMaintenanceWindowNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, schedule.ErrMaintenanceWindowExists) {
		return http.StatusConflict
	}
	if errors.Is(err, schedule.ErrMaintenanceActive) {
		return http.StatusConflict
	}
	if errors.Is(err, schedule.ErrMaintenanceConflict) {
		return http.StatusConflict
	}
	if errors.Is(err, schedule.ErrStoreClosed) || errors.Is(err, schedule.ErrStoreNotConnected) {
		return http.StatusServiceUnavailable
	}
	return http.StatusInternalServerError
}

func parseDuration(s string) time.Duration {
	if s == "" {
		return 0
	}
	d, _ := time.ParseDuration(s)
	return d
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck // best-effort response encoding
}

func writeError(w http.ResponseWriter, status int, message string) {
	apierror.Write(w, status, message)
}
