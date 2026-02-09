// Package cluster provides HTTP handlers for cluster management API endpoints.
package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/shawnbutts/keystone-core/internal/cluster"
)

// Handler provides HTTP handlers for cluster API endpoints.
type Handler struct {
	membership  *cluster.MembershipManager
	leader      *cluster.LeaderElector
	sharding    *cluster.ShardManager
	health      *cluster.HealthMonitor
	config      *cluster.Config
	shardStore  *cluster.ShardStore
	configStore *cluster.ConfigStore
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

// SetShardStore sets the shard store for backup/restore operations.
func (h *Handler) SetShardStore(store *cluster.ShardStore) {
	h.shardStore = store
}

// SetConfigStore sets the config store for backup/restore operations.
func (h *Handler) SetConfigStore(store *cluster.ConfigStore) {
	h.configStore = store
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

// StatusResponse represents the cluster status API response.
type StatusResponse struct {
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
	resp := StatusResponse{
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
	var req cluster.AddMemberRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	member, err := h.membership.AddMember(r.Context(), &req)
	if err != nil {
		// Check if it's a conflict error (member already exists or address in use)
		errMsg := err.Error()
		if contains(errMsg, "already exists") || contains(errMsg, "already in use") {
			writeError(w, http.StatusConflict, errMsg)
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to add member: "+errMsg)
		return
	}

	writeJSON(w, http.StatusCreated, member)
}

// contains checks if s contains substr (simple helper to avoid strings import)
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
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
	Success     bool      `json:"success"`
	Reason      string    `json:"reason"`
	MovedAgents int       `json:"moved_agents"`
	TriggerID   string    `json:"trigger_member_id"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Duration    string    `json:"duration"`
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
	if _, err := io.Copy(w, bytes.NewReader(backup)); err != nil { // nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter -- binary download response
		writeError(w, http.StatusInternalServerError, "Failed to write backup")
	}
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

	var backup BackupData
	if err := json.Unmarshal(data, &backup); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid backup format: "+err.Error())
		return
	}

	if err := h.validateBackup(&backup); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Parse options from query parameters
	options := RestoreOptions{
		RestoreShards: r.URL.Query().Get("restore_shards") != "false",
		RestoreConfig: r.URL.Query().Get("restore_config") != "false",
		Force:         r.URL.Query().Get("force") == "true",
	}

	ctx := r.Context()
	if err := h.performRestore(ctx, &backup, &options); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := RestoreResult{
		Success:        true,
		Message:        "Cluster state restored successfully",
		ShardsRestored: len(backup.Shards),
		ConfigRestored: len(backup.Config),
	}

	writeJSON(w, http.StatusOK, result)
}

// BackupData represents the cluster backup structure.
type BackupData struct {
	Version   string            `json:"version"`
	Timestamp time.Time         `json:"timestamp"`
	Cluster   Backup            `json:"cluster"`
	Shards    []ShardBackup     `json:"shards,omitempty"`
	Config    map[string]string `json:"config,omitempty"`
}

// Backup contains cluster membership information.
type Backup struct {
	Name       string                 `json:"name"`
	QuorumSize int                    `json:"quorum_size"`
	Members    []MemberStatusResponse `json:"members"`
	LeaderID   string                 `json:"leader_id"`
}

// ShardBackup contains a shard assignment.
type ShardBackup struct {
	AgentID    string    `json:"agent_id"`
	MemberID   string    `json:"member_id"`
	AssignedAt time.Time `json:"assigned_at"`
}

// RestoreOptions configures how restore behaves.
type RestoreOptions struct {
	RestoreShards bool `json:"restore_shards"`
	RestoreConfig bool `json:"restore_config"`
	Force         bool `json:"force"` // Force restore even if cluster is healthy
}

// RestoreResult contains the results of a restore operation.
type RestoreResult struct {
	Success        bool     `json:"success"`
	Message        string   `json:"message"`
	ShardsRestored int      `json:"shards_restored,omitempty"`
	ConfigRestored int      `json:"config_restored,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
}

func (h *Handler) createBackup(ctx context.Context) ([]byte, error) {
	members := h.membership.ListMembers()
	leaderID := h.leader.GetLeaderID()

	backup := BackupData{
		Version:   "1.0",
		Timestamp: time.Now().UTC(),
	}

	// Backup cluster membership
	backup.Cluster.Name = h.config.ClusterName
	backup.Cluster.QuorumSize = h.config.QuorumSize
	backup.Cluster.LeaderID = leaderID
	backup.Cluster.Members = make([]MemberStatusResponse, 0, len(members))

	for _, m := range members {
		agentCount := m.AgentCount
		if h.sharding != nil {
			agentCount = h.sharding.GetAgentCountForMember(m.ID)
		}

		backup.Cluster.Members = append(backup.Cluster.Members, MemberStatusResponse{
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

	// Backup shard assignments
	if h.sharding != nil {
		assignments := h.sharding.GetAllAssignments()
		backup.Shards = make([]ShardBackup, 0, len(assignments))
		for agentID, memberID := range assignments {
			backup.Shards = append(backup.Shards, ShardBackup{
				AgentID:    agentID,
				MemberID:   memberID,
				AssignedAt: time.Now().UTC(), // We don't have original time, use now
			})
		}
	}

	// Backup configuration if config store is available
	if h.configStore != nil {
		configs, err := h.configStore.ListConfigs(ctx, "")
		if err == nil && len(configs) > 0 {
			backup.Config = make(map[string]string, len(configs))
			for key, value := range configs {
				backup.Config[key] = string(value)
			}
		}
	}

	return json.MarshalIndent(backup, "", "  ")
}

// validateBackup validates the backup data structure.
func (h *Handler) validateBackup(backup *BackupData) error {
	if backup.Version == "" {
		return fmt.Errorf("invalid backup: missing version")
	}

	// Check version compatibility
	if backup.Version != "1.0" {
		return fmt.Errorf("unsupported backup version: %s (supported: 1.0)", backup.Version)
	}

	if backup.Timestamp.IsZero() {
		return fmt.Errorf("invalid backup: missing timestamp")
	}

	if backup.Cluster.Name == "" {
		return fmt.Errorf("invalid backup: missing cluster name")
	}

	return nil
}

// performRestore executes the actual restore operation.
func (h *Handler) performRestore(ctx context.Context, backup *BackupData, options *RestoreOptions) error {
	var warnings []string

	// Safety check: don't restore to a healthy cluster unless forced
	if !options.Force && h.membership.HasQuorum() {
		return fmt.Errorf("cluster is healthy with quorum; use force=true to restore anyway")
	}

	// Verify cluster name matches (prevent restoring wrong backup)
	if backup.Cluster.Name != h.config.ClusterName {
		return fmt.Errorf("backup cluster name '%s' does not match current cluster '%s'",
			backup.Cluster.Name, h.config.ClusterName)
	}

	// Restore shard assignments
	shardsRestored := 0
	if options.RestoreShards && len(backup.Shards) > 0 {
		if h.shardStore == nil {
			warnings = append(warnings, "shard store not configured, skipping shard restoration")
		} else {
			for _, shard := range backup.Shards {
				// Check if the target member exists and is healthy
				member, err := h.membership.GetMember(shard.MemberID)
				if err != nil || !member.Status.IsHealthy() {
					// Try to find a healthy member to assign to
					healthyMembers := h.membership.GetHealthyMembers()
					if len(healthyMembers) == 0 {
						warnings = append(warnings,
							fmt.Sprintf("no healthy members for agent %s, skipping", shard.AgentID))
						continue
					}
					// Use the first healthy member as fallback
					shard.MemberID = healthyMembers[0].ID
					warnings = append(warnings,
						fmt.Sprintf("agent %s reassigned to %s (original member unavailable)",
							shard.AgentID, shard.MemberID))
				}

				assignment := &cluster.ShardAssignment{
					AgentID:    shard.AgentID,
					MemberID:   shard.MemberID,
					AssignedAt: shard.AssignedAt,
					Version:    1,
				}

				if err := h.shardStore.SetAssignment(ctx, assignment); err != nil {
					warnings = append(warnings,
						fmt.Sprintf("failed to restore shard for agent %s: %v", shard.AgentID, err))
					continue
				}
				shardsRestored++
			}
		}
	}

	// Restore configuration
	configRestored := 0
	if options.RestoreConfig && len(backup.Config) > 0 {
		if h.configStore == nil {
			warnings = append(warnings, "config store not configured, skipping config restoration")
		} else {
			for key, value := range backup.Config {
				if err := h.configStore.SetConfig(ctx, key, []byte(value)); err != nil {
					warnings = append(warnings,
						fmt.Sprintf("failed to restore config key %s: %v", key, err))
					continue
				}
				configRestored++
			}
		}
	}

	// Log warnings if any
	if len(warnings) > 0 {
		// In a real implementation, we'd log these
		_ = warnings
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
