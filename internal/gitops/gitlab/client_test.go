package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.BaseURL != "https://gitlab.com" {
		t.Errorf("BaseURL = %v, want https://gitlab.com", config.BaseURL)
	}
}

func TestMergeRequestRequest(t *testing.T) {
	req := &MergeRequestRequest{
		ProjectID:          "kscoreorg/kscore-states",
		Title:              "Fix: Revert broken deployment",
		Description:        "Reverting deployment due to verification failure",
		SourceBranch:       "revert-123",
		TargetBranch:       "main",
		RemoveSourceBranch: true,
		AllowCollaboration: true,
	}

	if req.Title != "Fix: Revert broken deployment" {
		t.Errorf("Title = %v", req.Title)
	}

	if !req.RemoveSourceBranch {
		t.Error("RemoveSourceBranch = false, want true")
	}
}

func TestCommitStatusRequest(t *testing.T) {
	req := &CommitStatusRequest{
		ProjectID:   "kscoreorg/kscore-states",
		Ref:         "abc123",
		State:       "success",
		TargetURL:   "https://kscore.example.com/verification/123",
		Description: "All verification checks passed",
		Name:        "kscore/deployment-verification",
	}

	if req.State != "success" {
		t.Errorf("State = %v, want success", req.State)
	}

	if req.Name != "kscore/deployment-verification" {
		t.Errorf("Name = %v", req.Name)
	}
}

func TestCommentRequest(t *testing.T) {
	req := &CommentRequest{
		ProjectID: "kscoreorg/kscore-states",
		MRIID:     42,
		Body:      "✅ Deployment verification passed\n\n**Results:**\n- Health check: OK\n- Smoke tests: PASSED",
	}

	if req.MRIID != 42 {
		t.Errorf("MRIID = %d, want 42", req.MRIID)
	}

	if req.Body == "" {
		t.Error("Body is empty")
	}
}

func TestMergeRequestInfo(t *testing.T) {
	info := &MergeRequestInfo{
		IID:          42,
		Title:        "Fix: Revert deployment",
		State:        "opened",
		SourceBranch: "revert-123",
		TargetBranch: "main",
		MergedAt:     "",
		Draft:        false,
		WebURL:       "https://gitlab.com/kscoreorg/kscore-states/-/merge_requests/42",
	}

	if info.IID != 42 {
		t.Errorf("IID = %d, want 42", info.IID)
	}

	if info.State != "opened" {
		t.Errorf("State = %v, want opened", info.State)
	}

	if info.MergedAt != "" {
		t.Errorf("MergedAt = %v, want empty", info.MergedAt)
	}
}

func TestNewClientRequiresToken(t *testing.T) {
	config := &Config{
		BaseURL:   "https://gitlab.com",
		ProjectID: "kscoreorg/kscore-states",
		// Token is empty
	}

	_, err := NewClient(config)
	if err == nil {
		t.Error("NewClient() expected error for missing token, got nil")
	}
}

func TestNewClientWithToken(t *testing.T) {
	config := &Config{
		BaseURL:   "https://gitlab.com",
		ProjectID: "kscoreorg/kscore-states",
		Token:     "test-token",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Errorf("NewClient() unexpected error: %v", err)
	}
	if client == nil {
		t.Error("NewClient() returned nil client")
	}
}

func TestNewClientWithNilConfig(t *testing.T) {
	// Should use DefaultConfig which has no token
	_, err := NewClient(nil)
	if err == nil {
		t.Error("NewClient(nil) expected error for missing token, got nil")
	}
}

// setupMockServer creates a test server that mocks GitLab API
func setupMockServer(t *testing.T, handler http.Handler) (*httptest.Server, *Client) {
	server := httptest.NewServer(handler)

	config := &Config{
		BaseURL:   server.URL,
		Token:     "test-token",
		ProjectID: "testproject",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	return server, client
}

func TestCreateMergeRequest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/testproject/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		resp := map[string]interface{}{
			"iid":           42,
			"title":         "Test MR",
			"state":         "opened",
			"source_branch": "feature-branch",
			"target_branch": "main",
			"draft":         false,
			"web_url":       "https://gitlab.com/testproject/-/merge_requests/42",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server, client := setupMockServer(t, mux)
	defer server.Close()

	mr, err := client.CreateMergeRequest(context.Background(), &MergeRequestRequest{
		Title:        "Test MR",
		Description:  "Test description",
		SourceBranch: "feature-branch",
		TargetBranch: "main",
	})

	if err != nil {
		t.Fatalf("CreateMergeRequest() error: %v", err)
	}

	if mr.IID != 42 {
		t.Errorf("MR IID = %d, want 42", mr.IID)
	}

	if mr.Title != "Test MR" {
		t.Errorf("MR title = %s, want 'Test MR'", mr.Title)
	}

	if mr.State != "opened" {
		t.Errorf("MR state = %s, want 'opened'", mr.State)
	}
}

func TestCreateMergeRequest_MissingProjectID(t *testing.T) {
	config := &Config{
		BaseURL: "https://gitlab.com",
		Token:   "test-token",
		// ProjectID not set
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	_, err = client.CreateMergeRequest(context.Background(), &MergeRequestRequest{
		Title:        "Test MR",
		SourceBranch: "feature",
		TargetBranch: "main",
	})

	if err == nil {
		t.Error("Expected error for missing project ID, got nil")
	}

	if !strings.Contains(err.Error(), "project ID must be specified") {
		t.Errorf("Expected 'project ID' error, got: %v", err)
	}
}

func TestGetMergeRequest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/testproject/merge_requests/42", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}

		resp := map[string]interface{}{
			"iid":           42,
			"title":         "Test MR",
			"state":         "merged",
			"source_branch": "feature-branch",
			"target_branch": "main",
			"draft":         false,
			"web_url":       "https://gitlab.com/testproject/-/merge_requests/42",
			"merged_at":     "2024-01-15T10:30:00Z",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server, client := setupMockServer(t, mux)
	defer server.Close()

	mr, err := client.GetMergeRequest(context.Background(), "", 42)
	if err != nil {
		t.Fatalf("GetMergeRequest() error: %v", err)
	}

	if mr.IID != 42 {
		t.Errorf("MR IID = %d, want 42", mr.IID)
	}

	if mr.State != "merged" {
		t.Errorf("MR state = %s, want 'merged'", mr.State)
	}
}

func TestListMergeRequests(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/testproject/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}

		state := r.URL.Query().Get("state")
		if state != "opened" {
			t.Errorf("Expected state=opened, got %s", state)
		}

		resp := []map[string]interface{}{
			{
				"iid":           1,
				"title":         "MR 1",
				"state":         "opened",
				"source_branch": "branch-1",
				"target_branch": "main",
				"web_url":       "https://gitlab.com/testproject/-/merge_requests/1",
			},
			{
				"iid":           2,
				"title":         "MR 2",
				"state":         "opened",
				"source_branch": "branch-2",
				"target_branch": "main",
				"web_url":       "https://gitlab.com/testproject/-/merge_requests/2",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server, client := setupMockServer(t, mux)
	defer server.Close()

	mrs, err := client.ListMergeRequests(context.Background(), "", "opened")
	if err != nil {
		t.Fatalf("ListMergeRequests() error: %v", err)
	}

	if len(mrs) != 2 {
		t.Errorf("Expected 2 MRs, got %d", len(mrs))
	}

	if mrs[0].Title != "MR 1" {
		t.Errorf("First MR title = %s, want 'MR 1'", mrs[0].Title)
	}
}

func TestUpdateCommitStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/testproject/statuses/abc123", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		resp := map[string]interface{}{
			"id":          1,
			"sha":         "abc123",
			"status":      "success",
			"name":        "kscore/verification",
			"description": "Tests passed",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server, client := setupMockServer(t, mux)
	defer server.Close()

	err := client.UpdateCommitStatus(context.Background(), &CommitStatusRequest{
		Ref:         "abc123",
		State:       "success",
		Description: "Tests passed",
		Name:        "kscore/verification",
		TargetURL:   "https://example.com/status",
	})

	if err != nil {
		t.Fatalf("UpdateCommitStatus() error: %v", err)
	}
}

func TestUpdateCommitStatus_MissingProjectID(t *testing.T) {
	config := &Config{
		BaseURL: "https://gitlab.com",
		Token:   "test-token",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	err = client.UpdateCommitStatus(context.Background(), &CommitStatusRequest{
		Ref:   "abc123",
		State: "success",
	})

	if err == nil {
		t.Error("Expected error for missing project ID, got nil")
	}
}

func TestCommentOnMR(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/testproject/merge_requests/42/notes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		resp := map[string]interface{}{
			"id":   1,
			"body": "Test comment",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server, client := setupMockServer(t, mux)
	defer server.Close()

	err := client.CommentOnMR(context.Background(), &CommentRequest{
		MRIID: 42,
		Body:  "Test comment",
	})

	if err != nil {
		t.Fatalf("CommentOnMR() error: %v", err)
	}
}

func TestCommentOnMR_MissingProjectID(t *testing.T) {
	config := &Config{
		BaseURL: "https://gitlab.com",
		Token:   "test-token",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	err = client.CommentOnMR(context.Background(), &CommentRequest{
		MRIID: 42,
		Body:  "Test",
	})

	if err == nil {
		t.Error("Expected error for missing project ID, got nil")
	}
}

func TestMergeMergeRequest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/testproject/merge_requests/42/merge", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("Expected PUT, got %s", r.Method)
		}

		resp := map[string]interface{}{
			"iid":       42,
			"state":     "merged",
			"merged_at": "2024-01-15T10:30:00Z",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server, client := setupMockServer(t, mux)
	defer server.Close()

	err := client.MergeMergeRequest(context.Background(), "", 42, "Merge commit message")
	if err != nil {
		t.Fatalf("MergeMergeRequest() error: %v", err)
	}
}

func TestMergeMergeRequest_MissingProjectID(t *testing.T) {
	config := &Config{
		BaseURL: "https://gitlab.com",
		Token:   "test-token",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	err = client.MergeMergeRequest(context.Background(), "", 42, "message")
	if err == nil {
		t.Error("Expected error for missing project ID, got nil")
	}
}

func TestCreateMergeRequest_WithExplicitProjectID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/projects/explicit-project/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"iid":           1,
			"title":         "Test",
			"state":         "opened",
			"source_branch": "feature",
			"target_branch": "main",
			"web_url":       "https://gitlab.com/explicit-project/-/merge_requests/1",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server, client := setupMockServer(t, mux)
	defer server.Close()

	mr, err := client.CreateMergeRequest(context.Background(), &MergeRequestRequest{
		ProjectID:    "explicit-project",
		Title:        "Test",
		SourceBranch: "feature",
		TargetBranch: "main",
	})

	if err != nil {
		t.Fatalf("CreateMergeRequest() error: %v", err)
	}

	if mr.IID != 1 {
		t.Errorf("MR IID = %d, want 1", mr.IID)
	}
}

func TestGetMergeRequest_MissingProjectID(t *testing.T) {
	config := &Config{
		BaseURL: "https://gitlab.com",
		Token:   "test-token",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	_, err = client.GetMergeRequest(context.Background(), "", 42)
	if err == nil {
		t.Error("Expected error for missing project ID, got nil")
	}
}

func TestListMergeRequests_MissingProjectID(t *testing.T) {
	config := &Config{
		BaseURL: "https://gitlab.com",
		Token:   "test-token",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	_, err = client.ListMergeRequests(context.Background(), "", "opened")
	if err == nil {
		t.Error("Expected error for missing project ID, got nil")
	}
}
