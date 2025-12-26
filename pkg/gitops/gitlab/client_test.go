package gitlab

import (
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
		ProjectID:          "titanorg/titan-states",
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
		ProjectID:   "titanorg/titan-states",
		Ref:         "abc123",
		State:       "success",
		TargetURL:   "https://titan.example.com/verification/123",
		Description: "All verification checks passed",
		Name:        "titan/deployment-verification",
	}

	if req.State != "success" {
		t.Errorf("State = %v, want success", req.State)
	}

	if req.Name != "titan/deployment-verification" {
		t.Errorf("Name = %v", req.Name)
	}
}

func TestCommentRequest(t *testing.T) {
	req := &CommentRequest{
		ProjectID: "titanorg/titan-states",
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
		WebURL:       "https://gitlab.com/titanorg/titan-states/-/merge_requests/42",
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
		ProjectID: "titanorg/titan-states",
		// Token is empty
	}

	_, err := NewClient(config)
	if err == nil {
		t.Error("NewClient() expected error for missing token, got nil")
	}
}
