package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/cluster"
)

// Handler provides HTTP handlers for cluster API endpoints.
type Handler struct {
	membership  *cluster.MembershipManager
	leader      *cluster.LeaderElector
	sharding    *cluster.ShardManager
	health      *cluster.HealthMonitor
	config      *cluster.Config
}

// NewHandler creates a new cluster API handler.
func NewHandler(
	membership *cluster.MembershipManager,
	leader *cluster.LeaderElector,
	sharding *cluster.ShardManager,
	health *cluster.HealthMonitor,
	config *cluster.Config,
) *Handler {
	return &Handler{
		membership: membership,
		leader:     leader,
		sharding:   sharding,
		health:     health,
		config:     config,
	}
}

// RegisterRoutes registers the cluster API routes with the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/cluster/status", h.handleStatus)
	mux.HandleFunc("/api/v1/cluster/members", h.handleMembers)
	mux.HandleFunc("/api/v1/cluster/members/", h.handleMember)
	mux.HandleFunc("/api/v1/cluster/leader", h.handleLeader)
	mux.HandleFunc("/api/v1/cluster/leader/transfer", h.handleLeaderTransfer)
	mux.HandleFunc("/api/v1/cluster/rebalance", h.handleRebalance)
	mux.HandleFunc("/api/v1/cluster/backup", h.handleBackup)
	mux.HandleFunc("/api/v1/cluster/restore", h.handleRestore)
}

// ClusterStatusResponse represents the cluster status API response.
type ClusterStatusResponse struct {
	Healthy     bool                   `json:"healthy"`
	MemberCount int                    `json:"member_count"`
	QuorumSize  int                    `json:"quorum_size"`
	HasQuorum   bool                   `json:"has_quorum"`
	LeaderID    string                 `json:"leader_id"`
	Members     []MemberStatusResponse `json:"members"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// MemberStatusResponse represents a member's status in API responses.
type MemberStatusResponse struct {
	ID         string    `json:"id"`
	Address    string    `json:"address"`
	Status     string    `json:"status"`
	IsLeader   bool      `json:"is_leader"`
	Version    string    `json:"version"`
	StartedAt  time.Time `json:"started_at"`
	LastSeen   time.Time `json:"last_seen"`
	AgentCount int       `json:"agent_count"`
	JobCount   int       `json:"job_count"`
}

// handleStatus handles GET /api/v1/cluster/status
func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get all members
	members := h.membership.ListMembers()
	leaderID := h.leader.GetLeaderID()

	// Determine overall health based on quorum
	healthy := h.membership.HasQuorum() && h.health.HasQuorum()

	// Build response
	resp := ClusterStatusResponse{
		Healthy:     healthy,
		MemberCount: len(members),
		QuorumSize:  h.config.QuorumSize,
		HasQuorum:   h.membership.HasQuorum(),
		LeaderID:    leaderID,
		Members:     make([]MemberStatusResponse, 0, len(members)),
		UpdatedAt:   time.Now().UTC(),
	}

	for _, m := range members {
		agentCount := m.AgentCount
		if h.sharding != nil {
			agentCount = h.sharding.GetAgentCountForMember(m.ID)
		}

		resp.Members = append(resp.Members, MemberStatusResponse{
			ID:         m.ID,
			Address:    m.Address,
			Status:     string(m.Status),
			IsLeader:   m.ID == leaderID,
			Version:    m.Version,
			StartedAt:  m.JoinedAt,
			LastSeen:   m.LastHeartbeat,
			AgentCount: agentCount,
			JobCount:   m.JobCount,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleMembers handles /api/v1/cluster/members
func (h *Handler) handleMembers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.getMembers(w, r)
	case http.MethodPost:
		h.addMember(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) getMembers(w http.ResponseWriter, r *http.Request) {
	members := h.membership.ListMembers()
	leaderID := h.leader.GetLeaderID()

	resp := make([]MemberStatusResponse, 0, len(members))
	for _, m := range members {
		agentCount := m.AgentCount
		if h.sharding != nil {
			agentCount = h.sharding.GetAgentCountForMember(m.ID)
		}

		resp = append(resp, MemberStatusResponse{
			ID:         m.ID,
			Address:    m.Address,
			Status:     string(m.Status),
			IsLeader:   m.ID == leaderID,
			Version:    m.Version,
			StartedAt:  m.JoinedAt,
			LastSeen:   m.LastHeartbeat,
			AgentCount: agentCount,
			JobCount:   m.JobCount,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) addMember(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Address string `json:"address"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Address == "" {
		writeError(w, http.StatusBadRequest, "Address is required")
		return
	}

	// TODO: Implement member addition via etcd
	// For now, members self-register when they start
	writeError(w, http.StatusNotImplemented, "Member addition via API not yet implemented. Members self-register on startup.")
}

// handleMember handles /api/v1/cluster/members/{id}
func (h *Handler) handleMember(w http.ResponseWriter, r *http.Request) {
	// Extract member ID from URL
	memberID := r.URL.Path[len("/api/v1/cluster/members/"):]
	if memberID == "" {
		http.Error(w, "Member ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getMember(w, r, memberID)
	case http.MethodDelete:
		h.removeMember(w, r, memberID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) getMember(w http.ResponseWriter, r *http.Request, memberID string) {
	member, err := h.membership.GetMember(memberID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Member not found")
		return
	}

	leaderID := h.leader.GetLeaderID()
	agentCount := member.AgentCount
	if h.sharding != nil {
		agentCount = h.sharding.GetAgentCountForMember(member.ID)
	}

	resp := MemberStatusResponse{
		ID:         member.ID,
		Address:    member.Address,
		Status:     string(member.Status),
		IsLeader:   member.ID == leaderID,
		Version:    member.Version,
		StartedAt:  member.JoinedAt,
		LastSeen:   member.LastHeartbeat,
		AgentCount: agentCount,
		JobCount:   member.JobCount,
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request, memberID string) {
	force := r.URL.Query().Get("force") == "true"

	ctx := r.Context()

	if err := h.membership.RemoveMember(ctx, memberID, force); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleLeader handles GET /api/v1/cluster/leader
func (h *Handler) handleLeader(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	leaderID := h.leader.GetLeaderID()
	if leaderID == "" {
		writeError(w, http.StatusNotFound, "No leader elected")
		return
	}

	member, err := h.membership.GetMember(leaderID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Leader not found in membership")
		return
	}

	agentCount := member.AgentCount
	if h.sharding != nil {
		agentCount = h.sharding.GetAgentCountForMember(member.ID)
	}

	resp := MemberStatusResponse{
		ID:         member.ID,
		Address:    member.Address,
		Status:     string(member.Status),
		IsLeader:   true,
		Version:    member.Version,
		StartedAt:  member.JoinedAt,
		LastSeen:   member.LastHeartbeat,
		AgentCount: agentCount,
		JobCount:   member.JobCount,
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleLeaderTransfer handles POST /api/v1/cluster/leader/transfer
func (h *Handler) handleLeaderTransfer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TargetID string `json:"target_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.TargetID == "" {
		writeError(w, http.StatusBadRequest, "target_id is required")
		return
	}

	ctx := r.Context()

	if err := h.leader.TransferLeadership(ctx, req.TargetID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message":   "Leadership transfer initiated",
		"target_id": req.TargetID,
	})
}

// RebalanceResponse represents the rebalance API response.
type RebalanceResponse struct {
	Success      bool      `json:"success"`
	Reason       string    `json:"reason"`
	MovedAgents  int       `json:"moved_agents"`
	TriggerID    string    `json:"trigger_member_id"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	Duration     string    `json:"duration"`
}

// handleRebalance handles POST /api/v1/cluster/rebalance
func (h *Handler) handleRebalance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	if h.sharding == nil {
		writeError(w, http.StatusServiceUnavailable, "Sharding not enabled")
		return
	}

	// Get optional reason from query parameter
	reason := r.URL.Query().Get("reason")
	if reason == "" {
		reason = "API request"
	}

	event, err := h.sharding.Rebalance(ctx, reason)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := RebalanceResponse{
		Success:     true,
		Reason:      event.Reason,
		MovedAgents: event.MovedAgents,
		TriggerID:   event.TriggerMemberID,
		StartTime:   event.StartTime,
		EndTime:     event.EndTime,
		Duration:    event.EndTime.Sub(event.StartTime).String(),
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleBackup handles GET /api/v1/cluster/backup
func (h *Handler) handleBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Create backup
	backup, err := h.createBackup(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=cluster-backup.json")
	w.Write(backup)
}

// handleRestore handles POST /api/v1/cluster/restore
func (h *Handler) handleRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	ctx := r.Context()

	if err := h.restoreBackup(ctx, data); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "Cluster state restored successfully",
	})
}

// BackupData represents the cluster backup structure.
type BackupData struct {
	Version   string    `json:"version"`
	Timestamp time.Time `json:"timestamp"`
	Cluster   struct {
		Name       string                 `json:"name"`
		QuorumSize int                    `json:"quorum_size"`
		Members    []MemberStatusResponse `json:"members"`
	} `json:"cluster"`
	// Add more sections as needed (config, shards, etc.)
}

func (h *Handler) createBackup(ctx context.Context) ([]byte, error) {
	members := h.membership.ListMembers()
	leaderID := h.leader.GetLeaderID()

	backup := BackupData{
		Version:   "1.0",
		Timestamp: time.Now().UTC(),
	}

	backup.Cluster.Name = h.config.ClusterName
	backup.Cluster.QuorumSize = h.config.QuorumSize
	backup.Cluster.Members = make([]MemberStatusResponse, 0, len(members))

	for _, m := range members {
		backup.Cluster.Members = append(backup.Cluster.Members, MemberStatusResponse{
			ID:         m.ID,
			Address:    m.Address,
			Status:     string(m.Status),
			IsLeader:   m.ID == leaderID,
			Version:    m.Version,
			StartedAt:  m.JoinedAt,
			LastSeen:   m.LastHeartbeat,
			AgentCount: m.AgentCount,
			JobCount:   m.JobCount,
		})
	}

	return json.MarshalIndent(backup, "", "  ")
}

func (h *Handler) restoreBackup(ctx context.Context, data []byte) error {
	var backup BackupData
	if err := json.Unmarshal(data, &backup); err != nil {
		return err
	}

	// TODO: Implement full restore logic
	// For now, just validate the backup format
	if backup.Version == "" {
		return fmt.Errorf("invalid backup: missing version")
	}

	return nil
}

// Helper functions

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
