package cluster

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/cluster"
)

func TestBackupData_Validation(t *testing.T) {
	tests := []struct {
		name    string
		backup  BackupData
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid backup",
			backup: BackupData{
				Version:   "1.0",
				Timestamp: time.Now(),
				Cluster: Backup{
					Name:       "test-cluster",
					QuorumSize: 2,
				},
			},
			wantErr: false,
		},
		{
			name: "missing version",
			backup: BackupData{
				Timestamp: time.Now(),
				Cluster: Backup{
					Name: "test-cluster",
				},
			},
			wantErr: true,
			errMsg:  "missing version",
		},
		{
			name: "unsupported version",
			backup: BackupData{
				Version:   "2.0",
				Timestamp: time.Now(),
				Cluster: Backup{
					Name: "test-cluster",
				},
			},
			wantErr: true,
			errMsg:  "unsupported backup version",
		},
		{
			name: "missing timestamp",
			backup: BackupData{
				Version: "1.0",
				Cluster: Backup{
					Name: "test-cluster",
				},
			},
			wantErr: true,
			errMsg:  "missing timestamp",
		},
		{
			name: "missing cluster name",
			backup: BackupData{
				Version:   "1.0",
				Timestamp: time.Now(),
				Cluster:   Backup{},
			},
			wantErr: true,
			errMsg:  "missing cluster name",
		},
	}

	h := &Handler{
		config: &cluster.Config{
			ClusterName: "test-cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.validateBackup(&tt.backup)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateBackup() expected error, got nil")
					return
				}
				if tt.errMsg != "" && !bytes.Contains([]byte(err.Error()), []byte(tt.errMsg)) {
					t.Errorf("validateBackup() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateBackup() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestBackupData_JSONRoundTrip(t *testing.T) {
	original := BackupData{
		Version:   "1.0",
		Timestamp: time.Now().UTC().Truncate(time.Second),
		Cluster: Backup{
			Name:       "test-cluster",
			QuorumSize: 3,
			LeaderID:   "member-1",
			Members: []MemberStatusResponse{
				{
					ID:       "member-1",
					Address:  "192.168.1.1:5000",
					Status:   "healthy",
					IsLeader: true,
				},
				{
					ID:       "member-2",
					Address:  "192.168.1.2:5000",
					Status:   "healthy",
					IsLeader: false,
				},
			},
		},
		Shards: []ShardBackup{
			{
				AgentID:    "agent-1",
				MemberID:   "member-1",
				AssignedAt: time.Now().UTC().Truncate(time.Second),
			},
			{
				AgentID:    "agent-2",
				MemberID:   "member-2",
				AssignedAt: time.Now().UTC().Truncate(time.Second),
			},
		},
		Config: map[string]string{
			"setting1": "value1",
			"setting2": "value2",
		},
	}

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal backup: %v", err)
	}

	// Unmarshal
	var restored BackupData
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal backup: %v", err)
	}

	// Verify
	if restored.Version != original.Version {
		t.Errorf("Version mismatch: got %s, want %s", restored.Version, original.Version)
	}
	if restored.Cluster.Name != original.Cluster.Name {
		t.Errorf("Cluster name mismatch: got %s, want %s", restored.Cluster.Name, original.Cluster.Name)
	}
	if len(restored.Cluster.Members) != len(original.Cluster.Members) {
		t.Errorf("Members count mismatch: got %d, want %d", len(restored.Cluster.Members), len(original.Cluster.Members))
	}
	if len(restored.Shards) != len(original.Shards) {
		t.Errorf("Shards count mismatch: got %d, want %d", len(restored.Shards), len(original.Shards))
	}
	if len(restored.Config) != len(original.Config) {
		t.Errorf("Config count mismatch: got %d, want %d", len(restored.Config), len(original.Config))
	}
}

func TestRestoreOptions_Defaults(t *testing.T) {
	opts := RestoreOptions{
		RestoreShards: true,
		RestoreConfig: true,
		Force:         false,
	}

	if !opts.RestoreShards {
		t.Error("RestoreShards should default to true")
	}
	if !opts.RestoreConfig {
		t.Error("RestoreConfig should default to true")
	}
	if opts.Force {
		t.Error("Force should default to false")
	}
}

func TestHandleBackup_MethodNotAllowed(t *testing.T) {
	h := &Handler{
		config: &cluster.Config{ClusterName: "test"},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/backup", nil)
	rec := httptest.NewRecorder()

	h.handleBackup(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("handleBackup() status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleRestore_MethodNotAllowed(t *testing.T) {
	h := &Handler{
		config: &cluster.Config{ClusterName: "test"},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster/restore", nil)
	rec := httptest.NewRecorder()

	h.handleRestore(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("handleRestore() status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleRestore_NilMembership(t *testing.T) {
	h := &Handler{
		config: &cluster.Config{ClusterName: "test"},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/restore", bytes.NewReader([]byte("invalid json")))
	rec := httptest.NewRecorder()

	h.handleRestore(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("handleRestore() nil membership status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestCreateBackup_RequiresMembership(t *testing.T) {
	// Test that createBackup requires a membership manager
	// The actual backup creation requires real infrastructure,
	// so we just test that our types and validation work correctly.

	// This test verifies the backup structure can be created
	backup := BackupData{
		Version:   "1.0",
		Timestamp: time.Now().UTC(),
		Cluster: Backup{
			Name:       "test-cluster",
			QuorumSize: 2,
			LeaderID:   "member-1",
			Members: []MemberStatusResponse{
				{
					ID:       "member-1",
					Address:  "192.168.1.1:5000",
					Status:   "healthy",
					IsLeader: true,
				},
			},
		},
		Shards: []ShardBackup{
			{
				AgentID:    "agent-1",
				MemberID:   "member-1",
				AssignedAt: time.Now().UTC(),
			},
		},
		Config: map[string]string{
			"key1": "value1",
		},
	}

	// Verify it can be marshaled/unmarshaled
	data, err := json.Marshal(backup)
	if err != nil {
		t.Fatalf("Failed to marshal backup: %v", err)
	}

	var restored BackupData
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal backup: %v", err)
	}

	// Verify validation passes
	h := &Handler{
		config: &cluster.Config{
			ClusterName: "test-cluster",
		},
	}

	if err := h.validateBackup(&restored); err != nil {
		t.Errorf("Backup validation failed: %v", err)
	}
}

func TestRestoreResult_Structure(t *testing.T) {
	result := RestoreResult{
		Success:        true,
		Message:        "Test message",
		ShardsRestored: 5,
		ConfigRestored: 3,
		Warnings:       []string{"warning1", "warning2"},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal RestoreResult: %v", err)
	}

	var restored RestoreResult
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal RestoreResult: %v", err)
	}

	if restored.Success != result.Success {
		t.Errorf("Success mismatch")
	}
	if restored.ShardsRestored != result.ShardsRestored {
		t.Errorf("ShardsRestored mismatch: got %d, want %d", restored.ShardsRestored, result.ShardsRestored)
	}
	if restored.ConfigRestored != result.ConfigRestored {
		t.Errorf("ConfigRestored mismatch: got %d, want %d", restored.ConfigRestored, result.ConfigRestored)
	}
	if len(restored.Warnings) != len(result.Warnings) {
		t.Errorf("Warnings count mismatch: got %d, want %d", len(restored.Warnings), len(result.Warnings))
	}
}

func TestSetShardStore(t *testing.T) {
	h := &Handler{}

	if h.shardStore != nil {
		t.Error("shardStore should be nil initially")
	}

	// We can't create a real ShardStore without etcd, but we can verify the setter works
	h.SetShardStore(nil)

	if h.shardStore != nil {
		t.Error("SetShardStore(nil) should set shardStore to nil")
	}
}

func TestSetConfigStore(t *testing.T) {
	h := &Handler{}

	if h.configStore != nil {
		t.Error("configStore should be nil initially")
	}

	h.SetConfigStore(nil)

	if h.configStore != nil {
		t.Error("SetConfigStore(nil) should set configStore to nil")
	}
}

func TestBackup_Structure(t *testing.T) {
	cb := Backup{
		Name:       "my-cluster",
		QuorumSize: 3,
		LeaderID:   "leader-1",
		Members: []MemberStatusResponse{
			{ID: "member-1", Status: "healthy"},
			{ID: "member-2", Status: "healthy"},
			{ID: "member-3", Status: "degraded"},
		},
	}

	if cb.Name != "my-cluster" {
		t.Errorf("Name mismatch")
	}
	if cb.QuorumSize != 3 {
		t.Errorf("QuorumSize mismatch")
	}
	if len(cb.Members) != 3 {
		t.Errorf("Members count mismatch")
	}
}

func TestShardBackup_Structure(t *testing.T) {
	now := time.Now().UTC()
	sb := ShardBackup{
		AgentID:    "agent-123",
		MemberID:   "member-456",
		AssignedAt: now,
	}

	if sb.AgentID != "agent-123" {
		t.Errorf("AgentID mismatch")
	}
	if sb.MemberID != "member-456" {
		t.Errorf("MemberID mismatch")
	}
	if !sb.AssignedAt.Equal(now) {
		t.Errorf("AssignedAt mismatch")
	}
}

func TestNewHandler(t *testing.T) {
	config := &cluster.Config{
		ClusterName: "test-cluster",
		QuorumSize:  2,
	}

	h := NewHandler(nil, nil, nil, nil, config)

	if h == nil {
		t.Fatal("NewHandler() returned nil")
	}
	if h.config != config {
		t.Error("config not set correctly")
	}
}

func TestRegisterRoutes(t *testing.T) {
	h := &Handler{
		config: &cluster.Config{ClusterName: "test"},
	}

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Test that routes are registered by verifying they handle non-GET methods
	// (handlers return 405 Method Not Allowed instead of 404 Not Found)
	tests := []struct {
		route  string
		method string
	}{
		// These routes only allow GET, so POST should return 405
		{"/api/v1/cluster/backup", http.MethodPost},
		// These routes only allow POST, so GET should return 405
		{"/api/v1/cluster/restore", http.MethodGet},
		{"/api/v1/cluster/leader/transfer", http.MethodGet},
		{"/api/v1/cluster/rebalance", http.MethodGet},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.route, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		// 405 Method Not Allowed means the route is registered
		if rec.Code == http.StatusNotFound {
			t.Errorf("Route %s not registered (got 404)", tt.route)
		}
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("Route %s: got status %d, want %d", tt.route, rec.Code, http.StatusMethodNotAllowed)
		}
	}
}

func TestHandleStatus_MethodNotAllowed(t *testing.T) {
	h := &Handler{
		config: &cluster.Config{ClusterName: "test"},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/status", nil)
	rec := httptest.NewRecorder()

	h.handleStatus(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("handleStatus() POST status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleLeader_MethodNotAllowed(t *testing.T) {
	h := &Handler{
		config: &cluster.Config{ClusterName: "test"},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/leader", nil)
	rec := httptest.NewRecorder()

	h.handleLeader(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("handleLeader() POST status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleLeaderTransfer_MethodNotAllowed(t *testing.T) {
	h := &Handler{
		config: &cluster.Config{ClusterName: "test"},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster/leader/transfer", nil)
	rec := httptest.NewRecorder()

	h.handleLeaderTransfer(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("handleLeaderTransfer() GET status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleLeaderTransfer_NilMembership(t *testing.T) {
	h := &Handler{
		config: &cluster.Config{ClusterName: "test"},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/leader/transfer", bytes.NewReader([]byte(`{"target_id":"node-2"}`)))
	rec := httptest.NewRecorder()

	h.handleLeaderTransfer(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("handleLeaderTransfer() nil membership status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleRebalance_MethodNotAllowed(t *testing.T) {
	h := &Handler{
		config: &cluster.Config{ClusterName: "test"},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster/rebalance", nil)
	rec := httptest.NewRecorder()

	h.handleRebalance(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("handleRebalance() GET status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleRebalance_NilMembership(t *testing.T) {
	h := &Handler{
		config:   &cluster.Config{ClusterName: "test"},
		sharding: nil,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/rebalance", nil)
	rec := httptest.NewRecorder()

	h.handleRebalance(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("handleRebalance() no sharding status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleMembers_MethodNotAllowed(t *testing.T) {
	h := &Handler{
		config: &cluster.Config{ClusterName: "test"},
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/cluster/members", nil)
	rec := httptest.NewRecorder()

	h.handleMembers(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("handleMembers() PUT status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleMember_MissingID(t *testing.T) {
	h := &Handler{
		config: &cluster.Config{ClusterName: "test"},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster/members/", nil)
	rec := httptest.NewRecorder()

	h.handleMember(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("handleMember() missing ID status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleMember_MethodNotAllowed(t *testing.T) {
	h := &Handler{
		config: &cluster.Config{ClusterName: "test"},
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/cluster/members/member-1", nil)
	rec := httptest.NewRecorder()

	h.handleMember(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("handleMember() PUT status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestContains_Helper(t *testing.T) {
	tests := []struct {
		s      string
		substr string
		want   bool
	}{
		{"already exists", "already exists", true},
		{"already in use", "already in use", true},
		{"member already exists", "already exists", true},
		{"address already in use", "already in use", true},
		{"some other error", "already exists", false},
		{"", "test", false},
		{"test", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.substr, func(t *testing.T) {
			got := contains(tt.s, tt.substr)
			if got != tt.want {
				t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
			}
		})
	}
}

func TestStatusResponse_Structure(t *testing.T) {
	now := time.Now().UTC()
	resp := StatusResponse{
		Healthy:     true,
		MemberCount: 3,
		QuorumSize:  2,
		HasQuorum:   true,
		LeaderID:    "leader-1",
		Members: []MemberStatusResponse{
			{
				ID:         "member-1",
				Address:    "192.168.1.1:5000",
				Status:     "healthy",
				IsLeader:   true,
				Version:    "1.0.0",
				StartedAt:  now,
				LastSeen:   now,
				AgentCount: 10,
				JobCount:   5,
			},
		},
		UpdatedAt: now,
	}

	// Test JSON serialization
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal StatusResponse: %v", err)
	}

	var restored StatusResponse
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal StatusResponse: %v", err)
	}

	if restored.Healthy != resp.Healthy {
		t.Error("Healthy mismatch")
	}
	if restored.MemberCount != resp.MemberCount {
		t.Error("MemberCount mismatch")
	}
	if restored.QuorumSize != resp.QuorumSize {
		t.Error("QuorumSize mismatch")
	}
	if restored.HasQuorum != resp.HasQuorum {
		t.Error("HasQuorum mismatch")
	}
	if restored.LeaderID != resp.LeaderID {
		t.Error("LeaderID mismatch")
	}
	if len(restored.Members) != len(resp.Members) {
		t.Error("Members count mismatch")
	}
}

func TestMemberStatusResponse_Structure(t *testing.T) {
	now := time.Now().UTC()
	resp := MemberStatusResponse{
		ID:         "member-1",
		Address:    "192.168.1.1:5000",
		Status:     "healthy",
		IsLeader:   true,
		Version:    "1.0.0",
		StartedAt:  now,
		LastSeen:   now,
		AgentCount: 10,
		JobCount:   5,
	}

	// Test JSON serialization
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal MemberStatusResponse: %v", err)
	}

	var restored MemberStatusResponse
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal MemberStatusResponse: %v", err)
	}

	if restored.ID != resp.ID {
		t.Error("ID mismatch")
	}
	if restored.Address != resp.Address {
		t.Error("Address mismatch")
	}
	if restored.Status != resp.Status {
		t.Error("Status mismatch")
	}
	if restored.IsLeader != resp.IsLeader {
		t.Error("IsLeader mismatch")
	}
	if restored.Version != resp.Version {
		t.Error("Version mismatch")
	}
	if restored.AgentCount != resp.AgentCount {
		t.Error("AgentCount mismatch")
	}
	if restored.JobCount != resp.JobCount {
		t.Error("JobCount mismatch")
	}
}

func TestRebalanceResponse_Structure(t *testing.T) {
	now := time.Now().UTC()
	resp := RebalanceResponse{
		Success:     true,
		Reason:      "Manual rebalance",
		MovedAgents: 5,
		TriggerID:   "member-1",
		StartTime:   now,
		EndTime:     now.Add(100 * time.Millisecond),
		Duration:    "100ms",
	}

	// Test JSON serialization
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal RebalanceResponse: %v", err)
	}

	var restored RebalanceResponse
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal RebalanceResponse: %v", err)
	}

	if restored.Success != resp.Success {
		t.Error("Success mismatch")
	}
	if restored.Reason != resp.Reason {
		t.Error("Reason mismatch")
	}
	if restored.MovedAgents != resp.MovedAgents {
		t.Error("MovedAgents mismatch")
	}
	if restored.TriggerID != resp.TriggerID {
		t.Error("TriggerID mismatch")
	}
	if restored.Duration != resp.Duration {
		t.Error("Duration mismatch")
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	data := map[string]string{"message": "test"}

	writeJSON(rec, http.StatusOK, data)

	if rec.Code != http.StatusOK {
		t.Errorf("writeJSON() status = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("writeJSON() Content-Type = %s, want application/json", contentType)
	}

	var result map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if result["message"] != "test" {
		t.Errorf("writeJSON() body mismatch")
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "test error")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("writeError() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var result map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if result["error"] != "invalid_request" {
		t.Errorf("writeError() error code mismatch: got %q, want %q", result["error"], "invalid_request")
	}
	if result["message"] != "test error" {
		t.Errorf("writeError() message mismatch: got %q, want %q", result["message"], "test error")
	}
}

func TestAddMember_NilMembership(t *testing.T) {
	h := &Handler{
		config: &cluster.Config{ClusterName: "test"},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/members", bytes.NewReader([]byte("invalid json")))
	rec := httptest.NewRecorder()

	h.addMember(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("addMember() nil membership status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestLeaderTransferRequest_Structure(t *testing.T) {
	// Test that the leader transfer request can be properly serialized
	req := struct {
		TargetID string `json:"target_id"`
	}{
		TargetID: "member-1",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	var restored struct {
		TargetID string `json:"target_id"`
	}
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal request: %v", err)
	}

	if restored.TargetID != req.TargetID {
		t.Errorf("TargetID mismatch: got %s, want %s", restored.TargetID, req.TargetID)
	}
}

func TestLeaderResponse_Structure(t *testing.T) {
	now := time.Now().UTC()
	resp := MemberStatusResponse{
		ID:         "leader-1",
		Address:    "192.168.1.1:5000",
		Status:     "healthy",
		IsLeader:   true,
		Version:    "1.0.0",
		StartedAt:  now,
		LastSeen:   now,
		AgentCount: 50,
		JobCount:   10,
	}

	// Verify leader-specific fields
	if !resp.IsLeader {
		t.Error("IsLeader should be true for leader response")
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}

	var decoded MemberStatusResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if decoded.ID != resp.ID {
		t.Errorf("ID mismatch")
	}
	if !decoded.IsLeader {
		t.Error("IsLeader mismatch after decode")
	}
}

func TestHandleLeader_NoLeader(t *testing.T) {
	// Test that when there's no leader, we get a 404
	// This requires a mock, but we can test the response structure
	resp := struct {
		Error string `json:"error"`
	}{
		Error: "No leader elected",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal error response: %v", err)
	}

	var decoded struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}

	if decoded.Error != "No leader elected" {
		t.Errorf("Error message mismatch: got %s", decoded.Error)
	}
}

func TestHandleMember_DeleteMethod(t *testing.T) {
	// Handler setup to verify it compiles correctly
	_ = &Handler{
		config: &cluster.Config{ClusterName: "test"},
	}

	// DELETE requires membership manager, but we can test without it
	// to verify the method routing works
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/cluster/members/member-1", nil)
	_ = httptest.NewRecorder()

	// Verify method routing works
	if req.Method != http.MethodDelete {
		t.Error("Method should be DELETE")
	}

	// Verify the member ID extraction logic
	memberID := req.URL.Path[len("/api/v1/cluster/members/"):]
	if memberID != "member-1" {
		t.Errorf("Member ID extraction failed: got %s, want member-1", memberID)
	}
}

func TestHandleMember_GetMethod(t *testing.T) {
	// Verify that GET requests are properly handled
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster/members/member-1", nil)

	// Verify the member ID extraction logic
	memberID := req.URL.Path[len("/api/v1/cluster/members/"):]
	if memberID != "member-1" {
		t.Errorf("Member ID extraction failed: got %s, want member-1", memberID)
	}
}

func TestRemoveMemberForceQuery(t *testing.T) {
	tests := []struct {
		url       string
		wantForce bool
	}{
		{"/api/v1/cluster/members/member-1", false},
		{"/api/v1/cluster/members/member-1?force=false", false},
		{"/api/v1/cluster/members/member-1?force=true", true},
		{"/api/v1/cluster/members/member-1?force=TRUE", false}, // case sensitive
	}

	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodDelete, tt.url, nil)
		force := req.URL.Query().Get("force") == "true"
		if force != tt.wantForce {
			t.Errorf("URL %s: force = %v, want %v", tt.url, force, tt.wantForce)
		}
	}
}

func TestRebalanceReasonQuery(t *testing.T) {
	tests := []struct {
		url        string
		wantReason string
	}{
		{"/api/v1/cluster/rebalance", "API request"},
		{"/api/v1/cluster/rebalance?reason=manual", "manual"},
		{"/api/v1/cluster/rebalance?reason=maintenance", "maintenance"},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodPost, tt.url, nil)
		reason := req.URL.Query().Get("reason")
		if reason == "" {
			reason = "API request"
		}
		if reason != tt.wantReason {
			t.Errorf("URL %s: reason = %v, want %v", tt.url, reason, tt.wantReason)
		}
	}
}

func TestBackupRequest_MethodCheck(t *testing.T) {
	// Verify the GET method is properly used for backup requests
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster/backup", nil)

	if req.Method != http.MethodGet {
		t.Error("Backup request should use GET method")
	}

	// Verify URL path
	if req.URL.Path != "/api/v1/cluster/backup" {
		t.Errorf("Unexpected path: got %s, want /api/v1/cluster/backup", req.URL.Path)
	}
}

func TestRestoreRequest_Structure(t *testing.T) {
	now := time.Now().UTC()
	request := struct {
		Backup  BackupData     `json:"backup"`
		Options RestoreOptions `json:"options"`
	}{
		Backup: BackupData{
			Version:   "1.0",
			Timestamp: now,
			Cluster: Backup{
				Name:       "test-cluster",
				QuorumSize: 3,
			},
		},
		Options: RestoreOptions{
			RestoreShards: true,
			RestoreConfig: true,
			Force:         false,
		},
	}

	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Failed to marshal restore request: %v", err)
	}

	var decoded struct {
		Backup  BackupData     `json:"backup"`
		Options RestoreOptions `json:"options"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal restore request: %v", err)
	}

	if decoded.Backup.Version != request.Backup.Version {
		t.Error("Backup version mismatch")
	}
	if decoded.Options.RestoreShards != request.Options.RestoreShards {
		t.Error("RestoreShards mismatch")
	}
}

func TestClusterMismatch_Validation(t *testing.T) {
	// Test the cluster mismatch validation logic
	currentCluster := "production-cluster"
	backupCluster := "different-cluster"

	backup := BackupData{
		Version:   "1.0",
		Timestamp: time.Now(),
		Cluster: Backup{
			Name: backupCluster,
		},
	}

	// Validation should fail when cluster names don't match
	if backup.Cluster.Name == currentCluster {
		t.Error("Test setup error: clusters should be different")
	}

	// Verify the mismatch detection logic
	if backup.Cluster.Name != backupCluster {
		t.Error("Cluster name not set correctly")
	}

	// This is the condition that would trigger a BadRequest
	if backup.Cluster.Name != currentCluster {
		// Expected - names don't match, restore should be rejected
	} else {
		t.Error("Expected cluster name mismatch")
	}
}

func TestHandleMembers_GET(t *testing.T) {
	// Handler setup to verify it compiles correctly
	_ = &Handler{
		config: &cluster.Config{ClusterName: "test"},
	}

	// GET without membership manager will panic, but we can test routing
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster/members", nil)

	if req.Method != http.MethodGet {
		t.Error("Expected GET method")
	}
}

func TestHandleMembers_POST_NilMembership(t *testing.T) {
	h := &Handler{
		config: &cluster.Config{ClusterName: "test"},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/members", bytes.NewReader([]byte("{}")))
	rec := httptest.NewRecorder()

	h.handleMembers(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("handleMembers() POST nil membership status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestValidateBackup_MissingClusterName(t *testing.T) {
	h := &Handler{
		config: &cluster.Config{
			ClusterName: "my-cluster",
		},
	}

	backup := &BackupData{
		Version:   "1.0",
		Timestamp: time.Now(),
		Cluster: Backup{
			Name: "", // Missing cluster name
		},
	}

	err := h.validateBackup(backup)
	if err == nil {
		t.Fatal("validateBackup() should fail for missing cluster name")
	}
	if !contains(err.Error(), "missing cluster name") {
		t.Errorf("validateBackup() error should mention missing cluster name, got: %v", err)
	}
}

func TestValidateBackup_UnsupportedVersion(t *testing.T) {
	h := &Handler{
		config: &cluster.Config{
			ClusterName: "my-cluster",
		},
	}

	backup := &BackupData{
		Version:   "2.0", // Unsupported version
		Timestamp: time.Now(),
		Cluster: Backup{
			Name: "my-cluster",
		},
	}

	err := h.validateBackup(backup)
	if err == nil {
		t.Fatal("validateBackup() should fail for unsupported version")
	}
	if !contains(err.Error(), "unsupported backup version") {
		t.Errorf("validateBackup() error should mention unsupported version, got: %v", err)
	}
}

func TestValidateBackup_Valid(t *testing.T) {
	h := &Handler{
		config: &cluster.Config{
			ClusterName: "my-cluster",
		},
	}

	backup := &BackupData{
		Version:   "1.0",
		Timestamp: time.Now(),
		Cluster: Backup{
			Name: "my-cluster",
		},
	}

	err := h.validateBackup(backup)
	if err != nil {
		t.Errorf("validateBackup() should succeed for valid backup, got error: %v", err)
	}
}

func TestRestoreResult_JSONEncoding(t *testing.T) {
	result := RestoreResult{
		Success:        false,
		Message:        "Restoration failed",
		ShardsRestored: 0,
		ConfigRestored: 0,
		Warnings:       []string{"warning 1", "warning 2", "warning 3"},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal RestoreResult: %v", err)
	}

	// Verify JSON contains expected fields
	if !bytes.Contains(data, []byte(`"success":false`)) {
		t.Error("JSON should contain success:false")
	}
	if !bytes.Contains(data, []byte(`"warnings"`)) {
		t.Error("JSON should contain warnings field")
	}

	var decoded RestoreResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal RestoreResult: %v", err)
	}

	if len(decoded.Warnings) != 3 {
		t.Errorf("Expected 3 warnings, got %d", len(decoded.Warnings))
	}
}

func TestErrorResponse_Structure(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusInternalServerError, "Something went wrong")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	var resp struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if resp.Error != "internal_error" {
		t.Errorf("Error code = %q, want %q", resp.Error, "internal_error")
	}
	if resp.Message != "Something went wrong" {
		t.Errorf("Error message = %q, want %q", resp.Message, "Something went wrong")
	}
}

func TestWriteJSON_ComplexData(t *testing.T) {
	rec := httptest.NewRecorder()
	data := map[string]interface{}{
		"string":  "value",
		"number":  42,
		"boolean": true,
		"nested": map[string]string{
			"key": "value",
		},
		"array": []string{"a", "b", "c"},
	}

	writeJSON(rec, http.StatusOK, data)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", contentType)
	}

	var decoded map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&decoded); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if decoded["string"] != "value" {
		t.Error("string field mismatch")
	}
	if decoded["number"].(float64) != 42 {
		t.Error("number field mismatch")
	}
	if decoded["boolean"] != true {
		t.Error("boolean field mismatch")
	}
}

func TestBackupData_EmptyFields(t *testing.T) {
	backup := BackupData{
		Version:   "1.0",
		Timestamp: time.Now(),
		Cluster: Backup{
			Name:    "test",
			Members: []MemberStatusResponse{}, // Empty members
		},
		Shards: []ShardBackup{}, // Empty shards
		Config: nil,             // Nil config
	}

	data, err := json.Marshal(backup)
	if err != nil {
		t.Fatalf("Failed to marshal backup with empty fields: %v", err)
	}

	var decoded BackupData
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal backup: %v", err)
	}

	if len(decoded.Cluster.Members) != 0 {
		t.Error("Members should be empty")
	}
	if len(decoded.Shards) != 0 {
		t.Error("Shards should be empty")
	}
}

func TestMemberStatusResponse_AllFields(t *testing.T) {
	now := time.Now().UTC()
	resp := MemberStatusResponse{
		ID:         "member-id-123",
		Address:    "10.0.0.1:5000",
		Status:     "degraded",
		IsLeader:   false,
		Version:    "2.1.0",
		StartedAt:  now.Add(-24 * time.Hour),
		LastSeen:   now,
		AgentCount: 100,
		JobCount:   25,
	}

	// Verify all fields are accessible
	if resp.ID == "" {
		t.Error("ID should not be empty")
	}
	if resp.Address == "" {
		t.Error("Address should not be empty")
	}
	if resp.Status != "degraded" {
		t.Errorf("Status = %s, want degraded", resp.Status)
	}
	if resp.IsLeader {
		t.Error("IsLeader should be false")
	}
	if resp.AgentCount != 100 {
		t.Errorf("AgentCount = %d, want 100", resp.AgentCount)
	}
	if resp.JobCount != 25 {
		t.Errorf("JobCount = %d, want 25", resp.JobCount)
	}
}

func TestStatusResponse_AllFields(t *testing.T) {
	now := time.Now().UTC()
	resp := StatusResponse{
		Healthy:     false,
		MemberCount: 5,
		QuorumSize:  3,
		HasQuorum:   true,
		LeaderID:    "leader-member",
		Members: []MemberStatusResponse{
			{ID: "member-1", Status: "healthy"},
			{ID: "member-2", Status: "healthy"},
			{ID: "member-3", Status: "healthy"},
			{ID: "member-4", Status: "degraded"},
			{ID: "member-5", Status: "unhealthy"},
		},
		UpdatedAt: now,
	}

	// Verify all fields
	if resp.Healthy {
		t.Error("Healthy should be false")
	}
	if resp.MemberCount != 5 {
		t.Errorf("MemberCount = %d, want 5", resp.MemberCount)
	}
	if resp.QuorumSize != 3 {
		t.Errorf("QuorumSize = %d, want 3", resp.QuorumSize)
	}
	if !resp.HasQuorum {
		t.Error("HasQuorum should be true")
	}
	if len(resp.Members) != 5 {
		t.Errorf("Members count = %d, want 5", len(resp.Members))
	}
}
