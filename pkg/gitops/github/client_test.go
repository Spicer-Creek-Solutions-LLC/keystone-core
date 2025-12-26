package github

import (
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
		Owner:               "titanorg",
		Repo:                "titan-states",
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
		Owner:       "titanorg",
		Repo:        "titan-states",
		Ref:         "abc123",
		State:       "success",
		TargetURL:   "https://titan.example.com/verification/123",
		Description: "All verification checks passed",
		Context:     "titan/deployment-verification",
	}

	if req.State != "success" {
		t.Errorf("State = %v, want success", req.State)
	}

	if req.Context != "titan/deployment-verification" {
		t.Errorf("Context = %v", req.Context)
	}
}

func TestCommentRequest(t *testing.T) {
	req := &CommentRequest{
		Owner:    "titanorg",
		Repo:     "titan-states",
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
		URL:    "https://github.com/titanorg/titan-states/pull/42",
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
		Owner:   "titanorg",
		Repo:    "titan-states",
		// Token is empty
	}

	_, err := NewClient(config)
	if err == nil {
		t.Error("NewClient() expected error for missing token, got nil")
	}
}
