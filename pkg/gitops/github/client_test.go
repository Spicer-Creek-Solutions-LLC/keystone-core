package github

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

	if config.BaseURL != "https://api.github.com" {
		t.Errorf("BaseURL = %v, want https://api.github.com", config.BaseURL)
	}
}

func TestPullRequestRequest(t *testing.T) {
	req := &PullRequestRequest{
		Owner:               "kscoreorg",
		Repo:                "kscore-states",
		Title:               "Fix: Revert broken deployment",
		Body:                "Reverting deployment due to verification failure",
		Head:                "revert-123",
		Base:                "main",
		Draft:               false,
		MaintainerCanModify: true,
	}

	if req.Title != "Fix: Revert broken deployment" {
		t.Errorf("Title = %v", req.Title)
	}

	if !req.MaintainerCanModify {
		t.Error("MaintainerCanModify = false, want true")
	}
}

func TestCommitStatusRequest(t *testing.T) {
	req := &CommitStatusRequest{
		Owner:       "kscoreorg",
		Repo:        "kscore-states",
		Ref:         "abc123",
		State:       "success",
		TargetURL:   "https://kscore.example.com/verification/123",
		Description: "All verification checks passed",
		Context:     "kscore/deployment-verification",
	}

	if req.State != "success" {
		t.Errorf("State = %v, want success", req.State)
	}

	if req.Context != "kscore/deployment-verification" {
		t.Errorf("Context = %v", req.Context)
	}
}

func TestCommentRequest(t *testing.T) {
	req := &CommentRequest{
		Owner:    "kscoreorg",
		Repo:     "kscore-states",
		PRNumber: 42,
		Body:     "✅ Deployment verification passed\n\n**Results:**\n- Health check: OK\n- Smoke tests: PASSED",
	}

	if req.PRNumber != 42 {
		t.Errorf("PRNumber = %d, want 42", req.PRNumber)
	}

	if req.Body == "" {
		t.Error("Body is empty")
	}
}

func TestPullRequestInfo(t *testing.T) {
	info := &PullRequestInfo{
		Number: 42,
		Title:  "Fix: Revert deployment",
		State:  "open",
		Head:   "revert-123",
		Base:   "main",
		Merged: false,
		Draft:  false,
		URL:    "https://github.com/kscoreorg/kscore-states/pull/42",
	}

	if info.Number != 42 {
		t.Errorf("Number = %d, want 42", info.Number)
	}

	if info.State != "open" {
		t.Errorf("State = %v, want open", info.State)
	}

	if info.Merged {
		t.Error("Merged = true, want false")
	}
}

func TestNewClientRequiresToken(t *testing.T) {
	config := &Config{
		BaseURL: "https://api.github.com",
		Owner:   "kscoreorg",
		Repo:    "kscore-states",
		// Token is empty
	}

	_, err := NewClient(config)
	if err == nil {
		t.Error("NewClient() expected error for missing token, got nil")
	}
}

func TestNewClientWithToken(t *testing.T) {
	config := &Config{
		BaseURL: "https://api.github.com",
		Owner:   "kscoreorg",
		Repo:    "kscore-states",
		Token:   "test-token",
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

// setupMockServer creates a test server that mocks GitHub API
func setupMockServer(t *testing.T, handler http.Handler) (*httptest.Server, *Client) {
	server := httptest.NewServer(handler)

	config := &Config{
		BaseURL: server.URL,
		Token:   "test-token",
		Owner:   "testowner",
		Repo:    "testrepo",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	return server, client
}

func TestCreatePullRequest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/testowner/testrepo/pulls", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		// Verify request body
		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("Failed to decode request body: %v", err)
		}

		if reqBody["title"] != "Test PR" {
			t.Errorf("Expected title 'Test PR', got %v", reqBody["title"])
		}

		// Return mock response
		resp := map[string]interface{}{
			"number":   42,
			"title":    "Test PR",
			"state":    "open",
			"html_url": "https://github.com/testowner/testrepo/pull/42",
			"merged":   false,
			"draft":    false,
			"head": map[string]interface{}{
				"ref": "feature-branch",
			},
			"base": map[string]interface{}{
				"ref": "main",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server, client := setupMockServer(t, mux)
	defer server.Close()

	pr, err := client.CreatePullRequest(context.Background(), &PullRequestRequest{
		Title: "Test PR",
		Body:  "Test body",
		Head:  "feature-branch",
		Base:  "main",
	})

	if err != nil {
		t.Fatalf("CreatePullRequest() error: %v", err)
	}

	if pr.Number != 42 {
		t.Errorf("PR number = %d, want 42", pr.Number)
	}

	if pr.Title != "Test PR" {
		t.Errorf("PR title = %s, want 'Test PR'", pr.Title)
	}

	if pr.State != "open" {
		t.Errorf("PR state = %s, want 'open'", pr.State)
	}
}

func TestCreatePullRequest_MissingOwnerRepo(t *testing.T) {
	config := &Config{
		BaseURL: "https://api.github.com",
		Token:   "test-token",
		// Owner and Repo not set
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	_, err = client.CreatePullRequest(context.Background(), &PullRequestRequest{
		Title: "Test PR",
		Head:  "feature",
		Base:  "main",
	})

	if err == nil {
		t.Error("Expected error for missing owner/repo, got nil")
	}

	if !strings.Contains(err.Error(), "owner and repo must be specified") {
		t.Errorf("Expected 'owner and repo' error, got: %v", err)
	}
}

func TestGetPullRequest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/testowner/testrepo/pulls/42", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}

		resp := map[string]interface{}{
			"number":   42,
			"title":    "Test PR",
			"state":    "open",
			"html_url": "https://github.com/testowner/testrepo/pull/42",
			"merged":   false,
			"draft":    true,
			"head": map[string]interface{}{
				"ref": "feature-branch",
			},
			"base": map[string]interface{}{
				"ref": "main",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server, client := setupMockServer(t, mux)
	defer server.Close()

	pr, err := client.GetPullRequest(context.Background(), "", "", 42)
	if err != nil {
		t.Fatalf("GetPullRequest() error: %v", err)
	}

	if pr.Number != 42 {
		t.Errorf("PR number = %d, want 42", pr.Number)
	}

	if !pr.Draft {
		t.Error("PR draft = false, want true")
	}
}

func TestListPullRequests(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/testowner/testrepo/pulls", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}

		state := r.URL.Query().Get("state")
		if state != "open" {
			t.Errorf("Expected state=open, got %s", state)
		}

		resp := []map[string]interface{}{
			{
				"number":   1,
				"title":    "PR 1",
				"state":    "open",
				"html_url": "https://github.com/testowner/testrepo/pull/1",
				"head":     map[string]interface{}{"ref": "branch-1"},
				"base":     map[string]interface{}{"ref": "main"},
			},
			{
				"number":   2,
				"title":    "PR 2",
				"state":    "open",
				"html_url": "https://github.com/testowner/testrepo/pull/2",
				"head":     map[string]interface{}{"ref": "branch-2"},
				"base":     map[string]interface{}{"ref": "main"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server, client := setupMockServer(t, mux)
	defer server.Close()

	prs, err := client.ListPullRequests(context.Background(), "", "", "open")
	if err != nil {
		t.Fatalf("ListPullRequests() error: %v", err)
	}

	if len(prs) != 2 {
		t.Errorf("Expected 2 PRs, got %d", len(prs))
	}

	if prs[0].Title != "PR 1" {
		t.Errorf("First PR title = %s, want 'PR 1'", prs[0].Title)
	}
}

func TestUpdateCommitStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/testowner/testrepo/statuses/abc123", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("Failed to decode request body: %v", err)
		}

		if reqBody["state"] != "success" {
			t.Errorf("Expected state 'success', got %v", reqBody["state"])
		}

		if reqBody["context"] != "kscore/verification" {
			t.Errorf("Expected context 'kscore/verification', got %v", reqBody["context"])
		}

		resp := map[string]interface{}{
			"id":          1,
			"state":       "success",
			"description": "Tests passed",
			"context":     "kscore/verification",
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
		Context:     "kscore/verification",
		TargetURL:   "https://example.com/status",
	})

	if err != nil {
		t.Fatalf("UpdateCommitStatus() error: %v", err)
	}
}

func TestCommentOnPR(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/testowner/testrepo/issues/42/comments", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("Failed to decode request body: %v", err)
		}

		if reqBody["body"] != "Test comment" {
			t.Errorf("Expected body 'Test comment', got %v", reqBody["body"])
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

	err := client.CommentOnPR(context.Background(), &CommentRequest{
		PRNumber: 42,
		Body:     "Test comment",
	})

	if err != nil {
		t.Fatalf("CommentOnPR() error: %v", err)
	}
}

func TestMergePullRequest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/testowner/testrepo/pulls/42/merge", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("Expected PUT, got %s", r.Method)
		}

		resp := map[string]interface{}{
			"sha":     "abc123",
			"merged":  true,
			"message": "Pull Request successfully merged",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server, client := setupMockServer(t, mux)
	defer server.Close()

	err := client.MergePullRequest(context.Background(), "", "", 42, "Merge commit message")
	if err != nil {
		t.Fatalf("MergePullRequest() error: %v", err)
	}
}

func TestMergePullRequest_MissingOwnerRepo(t *testing.T) {
	config := &Config{
		BaseURL: "https://api.github.com",
		Token:   "test-token",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	err = client.MergePullRequest(context.Background(), "", "", 42, "message")
	if err == nil {
		t.Error("Expected error for missing owner/repo, got nil")
	}
}

func TestCreatePullRequest_WithExplicitOwnerRepo(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/explicit-owner/explicit-repo/pulls", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"number":   1,
			"title":    "Test",
			"state":    "open",
			"html_url": "https://github.com/explicit-owner/explicit-repo/pull/1",
			"head":     map[string]interface{}{"ref": "feature"},
			"base":     map[string]interface{}{"ref": "main"},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server, client := setupMockServer(t, mux)
	defer server.Close()

	pr, err := client.CreatePullRequest(context.Background(), &PullRequestRequest{
		Owner: "explicit-owner",
		Repo:  "explicit-repo",
		Title: "Test",
		Head:  "feature",
		Base:  "main",
	})

	if err != nil {
		t.Fatalf("CreatePullRequest() error: %v", err)
	}

	if pr.Number != 1 {
		t.Errorf("PR number = %d, want 1", pr.Number)
	}
}
