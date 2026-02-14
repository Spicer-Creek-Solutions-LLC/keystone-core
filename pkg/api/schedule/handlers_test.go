package schedule

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/schedule"
)

type mockManager struct {
	schedules  []*schedule.Schedule
	executions []*schedule.Execution
	createErr  error
	getErr     error
	listErr    error
	deleteErr  error
	pauseErr   error
	resumeErr  error
	enableErr  error
	disableErr error
	triggerErr error
	updateErr  error
}

func (m *mockManager) Create(_ context.Context, s *schedule.Schedule) error {
	if m.createErr != nil {
		return m.createErr
	}
	if s.ID == "" {
		s.ID = "test-id"
	}
	s.CreatedAt = time.Now().UTC()
	s.UpdatedAt = s.CreatedAt
	m.schedules = append(m.schedules, s)
	return nil
}

func (m *mockManager) Get(_ context.Context, id string) (*schedule.Schedule, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	for _, s := range m.schedules {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, schedule.ErrScheduleNotFound
}

func (m *mockManager) Update(_ context.Context, s *schedule.Schedule) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	return nil
}

func (m *mockManager) Delete(_ context.Context, id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	return nil
}

func (m *mockManager) List(_ context.Context, _ *schedule.Filter) ([]*schedule.Schedule, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.schedules, nil
}

func (m *mockManager) Pause(_ context.Context, _, _ string) error { return m.pauseErr }
func (m *mockManager) Resume(_ context.Context, _, _ string) error { return m.resumeErr }
func (m *mockManager) Enable(_ context.Context, _, _ string) error { return m.enableErr }
func (m *mockManager) Disable(_ context.Context, _, _ string) error { return m.disableErr }

func (m *mockManager) TriggerNow(_ context.Context, id, by string) (*schedule.Execution, error) {
	if m.triggerErr != nil {
		return nil, m.triggerErr
	}
	exec := &schedule.Execution{
		ID:          "exec-1",
		ScheduleID:  id,
		Status:      schedule.ExecutionStatusPending,
		TriggerType: schedule.TriggerTypeManual,
		TriggeredBy: by,
		CreatedAt:   time.Now().UTC(),
	}
	return exec, nil
}

func (m *mockManager) ListExecutions(_ context.Context, _ *schedule.ExecutionFilter) ([]*schedule.Execution, error) {
	return m.executions, nil
}

type mockMaintenanceManager struct {
	windows      []*schedule.MaintenanceWindow
	conflicts    []*schedule.MaintenanceConflict
	createErr    error
	getErr       error
	listErr      error
	deleteErr    error
	startErr     error
	endErr       error
	cancelErr    error
	extendErr    error
}

func (m *mockMaintenanceManager) Create(_ context.Context, w *schedule.MaintenanceWindow) error {
	if m.createErr != nil {
		return m.createErr
	}
	if w.ID == "" {
		w.ID = "test-window-id"
	}
	w.CreatedAt = time.Now().UTC()
	w.UpdatedAt = w.CreatedAt
	m.windows = append(m.windows, w)
	return nil
}

func (m *mockMaintenanceManager) Get(_ context.Context, id string) (*schedule.MaintenanceWindow, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	for _, w := range m.windows {
		if w.ID == id {
			return w, nil
		}
	}
	return nil, schedule.ErrMaintenanceWindowNotFound
}

func (m *mockMaintenanceManager) Update(_ context.Context, _ *schedule.MaintenanceWindow) error {
	return nil
}

func (m *mockMaintenanceManager) Delete(_ context.Context, _ string) error { return m.deleteErr }
func (m *mockMaintenanceManager) Start(_ context.Context, _ string) error  { return m.startErr }
func (m *mockMaintenanceManager) End(_ context.Context, _ string) error    { return m.endErr }

func (m *mockMaintenanceManager) Cancel(_ context.Context, _, _, _ string) error { return m.cancelErr }

func (m *mockMaintenanceManager) Extend(_ context.Context, _ string, _ time.Time, _ string) error {
	return m.extendErr
}

func (m *mockMaintenanceManager) List(_ context.Context, _ *schedule.MaintenanceWindowFilter) ([]*schedule.MaintenanceWindow, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.windows, nil
}

func (m *mockMaintenanceManager) GetActiveWindows(_ context.Context) ([]*schedule.MaintenanceWindow, error) {
	var active []*schedule.MaintenanceWindow
	for _, w := range m.windows {
		if w.Status == schedule.MaintenanceWindowStatusActive {
			active = append(active, w)
		}
	}
	return active, nil
}

func (m *mockMaintenanceManager) GetUpcomingWindows(_ context.Context, _ time.Duration) ([]*schedule.MaintenanceWindow, error) {
	return m.windows, nil
}

func (m *mockMaintenanceManager) GetConflicts(_ context.Context, _ *schedule.MaintenanceWindow) ([]*schedule.MaintenanceConflict, error) {
	return m.conflicts, nil
}

func TestHandler_ListSchedules(t *testing.T) {
	mgr := &mockManager{
		schedules: []*schedule.Schedule{
			{ID: "s1", Name: "daily-backup", Type: schedule.TypeCommand, Status: schedule.StatusActive},
			{ID: "s2", Name: "hourly-sync", Type: schedule.TypeState, Status: schedule.StatusPaused},
		},
	}
	h := NewHandler(mgr, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/schedules", nil)
	w := httptest.NewRecorder()
	h.handleSchedules(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp ListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2", resp.Total)
	}
	if resp.Schedules[0].Name != "daily-backup" {
		t.Errorf("name = %s, want daily-backup", resp.Schedules[0].Name)
	}
}

func TestHandler_CreateSchedule(t *testing.T) {
	mgr := &mockManager{}
	h := NewHandler(mgr, nil)

	body := `{"name":"test-sched","type":"command","cron":"0 * * * *"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/schedules", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.handleSchedules(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Name != "test-sched" {
		t.Errorf("name = %s, want test-sched", resp.Name)
	}
}

func TestHandler_CreateSchedule_MissingName(t *testing.T) {
	mgr := &mockManager{}
	h := NewHandler(mgr, nil)

	body := `{"type":"command"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/schedules", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.handleSchedules(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_GetSchedule(t *testing.T) {
	mgr := &mockManager{
		schedules: []*schedule.Schedule{
			{ID: "s1", Name: "test", Type: schedule.TypeCommand, Status: schedule.StatusActive},
		},
	}
	h := NewHandler(mgr, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/schedules/s1", nil)
	w := httptest.NewRecorder()
	h.handleSchedule(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandler_GetSchedule_NotFound(t *testing.T) {
	mgr := &mockManager{}
	h := NewHandler(mgr, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/schedules/missing", nil)
	w := httptest.NewRecorder()
	h.handleSchedule(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandler_DeleteSchedule(t *testing.T) {
	mgr := &mockManager{}
	h := NewHandler(mgr, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/schedules/s1", nil)
	w := httptest.NewRecorder()
	h.handleSchedule(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
}

func TestHandler_TriggerSchedule(t *testing.T) {
	mgr := &mockManager{
		schedules: []*schedule.Schedule{
			{ID: "s1", Name: "test", Status: schedule.StatusActive},
		},
	}
	h := NewHandler(mgr, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/schedules/s1/trigger", nil)
	req.ContentLength = 0
	w := httptest.NewRecorder()
	h.handleSchedule(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp ExecutionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.ScheduleID != "s1" {
		t.Errorf("schedule_id = %s, want s1", resp.ScheduleID)
	}
}

func TestHandler_PauseSchedule(t *testing.T) {
	mgr := &mockManager{}
	h := NewHandler(mgr, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/schedules/s1/pause", nil)
	w := httptest.NewRecorder()
	h.handleSchedule(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandler_ResumeSchedule(t *testing.T) {
	mgr := &mockManager{}
	h := NewHandler(mgr, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/schedules/s1/resume", nil)
	w := httptest.NewRecorder()
	h.handleSchedule(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandler_GetHistory(t *testing.T) {
	now := time.Now().UTC()
	mgr := &mockManager{
		executions: []*schedule.Execution{
			{ID: "e1", ScheduleID: "s1", Status: schedule.ExecutionStatusCompleted, CreatedAt: now},
		},
	}
	h := NewHandler(mgr, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/schedules/s1/history", nil)
	w := httptest.NewRecorder()
	h.handleSchedule(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp ExecutionListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Total != 1 {
		t.Errorf("total = %d, want 1", resp.Total)
	}
}

func TestHandler_NilScheduleManager(t *testing.T) {
	h := NewHandler(nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/schedules", nil)
	w := httptest.NewRecorder()
	h.handleSchedules(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestHandler_ListWindows(t *testing.T) {
	maintMgr := &mockMaintenanceManager{
		windows: []*schedule.MaintenanceWindow{
			{ID: "w1", Name: "weekly-patching", Type: schedule.MaintenanceWindowTypePlanned, Status: schedule.MaintenanceWindowStatusScheduled,
				StartTime: time.Now().Add(24 * time.Hour), EndTime: time.Now().Add(28 * time.Hour)},
		},
	}
	h := NewHandler(nil, maintMgr)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/maintenance/windows", nil)
	w := httptest.NewRecorder()
	h.handleWindows(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp WindowListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Total != 1 {
		t.Errorf("total = %d, want 1", resp.Total)
	}
}

func TestHandler_CreateWindow(t *testing.T) {
	maintMgr := &mockMaintenanceManager{}
	h := NewHandler(nil, maintMgr)

	body := `{"name":"test-window","type":"planned","start_time":"2025-01-15T02:00:00Z","end_time":"2025-01-15T06:00:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/maintenance/windows", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.handleWindows(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusCreated, w.Body.String())
	}
}

func TestHandler_CreateWindow_MissingName(t *testing.T) {
	maintMgr := &mockMaintenanceManager{}
	h := NewHandler(nil, maintMgr)

	body := `{"type":"planned","start_time":"2025-01-15T02:00:00Z","end_time":"2025-01-15T06:00:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/maintenance/windows", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.handleWindows(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_GetWindow(t *testing.T) {
	maintMgr := &mockMaintenanceManager{
		windows: []*schedule.MaintenanceWindow{
			{ID: "w1", Name: "test", StartTime: time.Now(), EndTime: time.Now().Add(time.Hour)},
		},
	}
	h := NewHandler(nil, maintMgr)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/maintenance/windows/w1", nil)
	w := httptest.NewRecorder()
	h.handleWindow(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandler_GetWindow_NotFound(t *testing.T) {
	maintMgr := &mockMaintenanceManager{}
	h := NewHandler(nil, maintMgr)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/maintenance/windows/missing", nil)
	w := httptest.NewRecorder()
	h.handleWindow(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandler_DeleteWindow(t *testing.T) {
	maintMgr := &mockMaintenanceManager{}
	h := NewHandler(nil, maintMgr)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/maintenance/windows/w1", nil)
	w := httptest.NewRecorder()
	h.handleWindow(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
}

func TestHandler_StartWindow(t *testing.T) {
	maintMgr := &mockMaintenanceManager{}
	h := NewHandler(nil, maintMgr)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/maintenance/windows/w1/start", nil)
	w := httptest.NewRecorder()
	h.handleWindow(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandler_EndWindow(t *testing.T) {
	maintMgr := &mockMaintenanceManager{}
	h := NewHandler(nil, maintMgr)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/maintenance/windows/w1/end", nil)
	w := httptest.NewRecorder()
	h.handleWindow(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandler_CancelWindow(t *testing.T) {
	maintMgr := &mockMaintenanceManager{}
	h := NewHandler(nil, maintMgr)

	body := `{"reason":"no longer needed"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/maintenance/windows/w1/cancel", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.handleWindow(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandler_ExtendWindow(t *testing.T) {
	maintMgr := &mockMaintenanceManager{
		windows: []*schedule.MaintenanceWindow{
			{ID: "w1", Name: "test", EndTime: time.Now().Add(time.Hour)},
		},
	}
	h := NewHandler(nil, maintMgr)

	body := `{"duration":"2h"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/maintenance/windows/w1/extend", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.handleWindow(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestHandler_ActiveWindows(t *testing.T) {
	maintMgr := &mockMaintenanceManager{
		windows: []*schedule.MaintenanceWindow{
			{ID: "w1", Name: "active-window", Status: schedule.MaintenanceWindowStatusActive,
				StartTime: time.Now().Add(-time.Hour), EndTime: time.Now().Add(time.Hour)},
		},
	}
	h := NewHandler(nil, maintMgr)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/maintenance/windows/active", nil)
	w := httptest.NewRecorder()
	h.handleWindow(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandler_Conflicts(t *testing.T) {
	maintMgr := &mockMaintenanceManager{
		windows: []*schedule.MaintenanceWindow{
			{ID: "w1", Name: "test", StartTime: time.Now(), EndTime: time.Now().Add(time.Hour)},
		},
		conflicts: []*schedule.MaintenanceConflict{},
	}
	h := NewHandler(nil, maintMgr)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/maintenance/windows/w1/conflicts", nil)
	w := httptest.NewRecorder()
	h.handleWindow(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandler_NilMaintenanceManager(t *testing.T) {
	h := NewHandler(nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/maintenance/windows", nil)
	w := httptest.NewRecorder()
	h.handleWindows(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestMapScheduleError(t *testing.T) {
	tests := []struct {
		err    error
		status int
	}{
		{schedule.ErrScheduleNotFound, http.StatusNotFound},
		{schedule.ErrExecutionNotFound, http.StatusNotFound},
		{schedule.ErrScheduleExists, http.StatusConflict},
		{schedule.ErrScheduleDisabled, http.StatusConflict},
		{schedule.ErrInvalidSchedule, http.StatusBadRequest},
		{schedule.ErrInvalidCron, http.StatusBadRequest},
		{schedule.ErrStoreClosed, http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		if got := mapScheduleError(tt.err); got != tt.status {
			t.Errorf("mapScheduleError(%v) = %d, want %d", tt.err, got, tt.status)
		}
	}
}

func TestMapMaintenanceError(t *testing.T) {
	tests := []struct {
		err    error
		status int
	}{
		{schedule.ErrMaintenanceWindowNotFound, http.StatusNotFound},
		{schedule.ErrMaintenanceWindowExists, http.StatusConflict},
		{schedule.ErrMaintenanceActive, http.StatusConflict},
		{schedule.ErrMaintenanceConflict, http.StatusConflict},
		{schedule.ErrStoreClosed, http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		if got := mapMaintenanceError(tt.err); got != tt.status {
			t.Errorf("mapMaintenanceError(%v) = %d, want %d", tt.err, got, tt.status)
		}
	}
}

func TestRegisterRoutes(t *testing.T) {
	h := NewHandler(nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	paths := []string{
		"/api/v1/schedules",
		"/api/v1/schedules/",
		"/api/v1/maintenance/windows",
		"/api/v1/maintenance/windows/",
	}
	for _, p := range paths {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code == http.StatusNotFound {
			t.Errorf("route %s not registered (got 404)", p)
		}
	}
}

func TestConvertSchedule(t *testing.T) {
	now := time.Now().UTC()
	s := &schedule.Schedule{
		ID:       "s1",
		Name:     "test",
		Type:     schedule.TypeCommand,
		Status:   schedule.StatusActive,
		Interval: 30 * time.Minute,
		Timeout:  time.Hour,
		NextRun:  &now,
		Labels:   map[string]string{"env": "prod"},
	}

	resp := convertSchedule(s)
	if resp.ID != "s1" {
		t.Errorf("ID = %s, want s1", resp.ID)
	}
	if resp.Interval != "30m0s" {
		t.Errorf("Interval = %s, want 30m0s", resp.Interval)
	}
	if resp.Timeout != "1h0m0s" {
		t.Errorf("Timeout = %s, want 1h0m0s", resp.Timeout)
	}
}

func TestConvertExecution(t *testing.T) {
	now := time.Now().UTC()
	e := &schedule.Execution{
		ID:          "e1",
		ScheduleID:  "s1",
		Status:      schedule.ExecutionStatusCompleted,
		TriggerType: schedule.TriggerTypeScheduled,
		StartTime:   &now,
		Duration:    5 * time.Minute,
		CreatedAt:   now,
	}

	resp := convertExecution(e)
	if resp.ID != "e1" {
		t.Errorf("ID = %s, want e1", resp.ID)
	}
	if resp.Duration != "5m0s" {
		t.Errorf("Duration = %s, want 5m0s", resp.Duration)
	}
}

func TestConvertWindow(t *testing.T) {
	now := time.Now().UTC()
	win := &schedule.MaintenanceWindow{
		ID:        "w1",
		Name:      "test",
		Type:      schedule.MaintenanceWindowTypePlanned,
		Status:    schedule.MaintenanceWindowStatusScheduled,
		StartTime: now,
		EndTime:   now.Add(4 * time.Hour),
	}

	resp := convertWindow(win)
	if resp.ID != "w1" {
		t.Errorf("ID = %s, want w1", resp.ID)
	}
	if resp.Type != "planned" {
		t.Errorf("Type = %s, want planned", resp.Type)
	}
}
