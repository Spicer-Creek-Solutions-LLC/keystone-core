package secrets

import (
	"net/http"
	"strconv"
	"time"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

// handleComplianceReports handles POST /api/v1/compliance/reports.
func (h *Handler) handleComplianceReports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, ComplianceReportResponse{
		Message: "compliance report generation is not yet implemented",
	})
}

// handleAuditLogs handles GET /api/v1/audit/logs.
func (h *Handler) handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.auditLogger == nil {
		writeError(w, http.StatusServiceUnavailable, "audit logger not available")
		return
	}

	query := r.URL.Query()
	filter := &secrets.SecretAuditFilter{
		SecretPath: query.Get("path"),
		Action:     query.Get("action"),
		AgentID:    query.Get("agent_id"),
	}

	if v := query.Get("start"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid start time format (use RFC3339)")
			return
		}
		filter.StartTime = t
	}

	if v := query.Get("end"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid end time format (use RFC3339)")
			return
		}
		filter.EndTime = t
	}

	if v := query.Get("limit"); v != "" {
		limit, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid limit parameter")
			return
		}
		filter.Limit = limit
	}

	if v := query.Get("offset"); v != "" {
		offset, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid offset parameter")
			return
		}
		filter.Offset = offset
	}

	events := h.auditLogger.GetEventsFiltered(filter)
	if events == nil {
		events = []*secrets.SecretAccessEvent{}
	}

	respEvents := make([]*AuditEventResponse, len(events))
	for i, e := range events {
		respEvents[i] = &AuditEventResponse{
			SecretPath: e.SecretPath,
			Backend:    e.Backend,
			SecretType: e.SecretType,
			AgentID:    e.AgentID,
			SPIFFEID:   e.SPIFFEID,
			RequestID:  e.RequestID,
			LeaseID:    e.LeaseID,
			Action:     e.Action,
			Timestamp:  e.Timestamp,
			DurationMS: e.Duration.Milliseconds(),
			CacheHit:   e.CacheHit,
			Success:    e.Success,
			Error:      e.Error,
			SourceIP:   e.SourceIP,
			Extra:      e.Extra,
		}
	}

	writeJSON(w, http.StatusOK, AuditLogResponse{
		Events: respEvents,
		Total:  len(respEvents),
	})
}

// handleSecretsHealth handles GET /api/v1/health/secrets.
func (h *Handler) handleSecretsHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.broker == nil {
		writeError(w, http.StatusServiceUnavailable, "secrets broker not available")
		return
	}

	ctx := r.Context()
	healthy := h.broker.Healthy(ctx)
	backendHealth := h.broker.BackendHealth(ctx)
	stats := h.broker.Stats(ctx)

	resp := HealthResponse{
		Healthy:       healthy,
		BackendHealth: backendHealth,
		BackendCount:  stats.BackendCount,
		CacheStats:    stats.CacheStats,
		LeaseStats:    stats.LeaseStats,
	}

	status := http.StatusOK
	if !healthy {
		status = http.StatusServiceUnavailable
	}

	writeJSON(w, status, resp)
}
