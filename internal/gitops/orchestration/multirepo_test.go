package orchestration

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

// mockDeployer is a mock deployer for testing
type mockDeployer struct {
	deployFunc     func(ctx context.Context, repo *Repository, revision string) (*RepoResult, error)
	getCurrentFunc func(ctx context.Context, repo *Repository) (string, error)
	rollbackFunc   func(ctx context.Context, repo *Repository, revision string) error
	deployerType   string
}

func (m *mockDeployer) Deploy(ctx context.Context, repo *Repository, revision string) (*RepoResult, error) {
	if m.deployFunc != nil {
		return m.deployFunc(ctx, repo, revision)
	}
	return &RepoResult{
		RepoName:  repo.Name,
		Revision:  revision,
		Status:    StatusCompleted,
		StartTime: time.Now(),
		EndTime:   time.Now(),
		Message:   "Deployed successfully",
	}, nil
}

func (m *mockDeployer) GetCurrentRevision(ctx context.Context, repo *Repository) (string, error) {
	if m.getCurrentFunc != nil {
		return m.getCurrentFunc(ctx, repo)
	}
	return "abc123", nil
}

func (m *mockDeployer) Rollback(ctx context.Context, repo *Repository, revision string) error {
	if m.rollbackFunc != nil {
		return m.rollbackFunc(ctx, repo, revision)
	}
	return nil
}

func (m *mockDeployer) Type() string {
	if m.deployerType != "" {
		return m.deployerType
	}
	return "default"
}

// mockVerifier is a mock verifier for testing
type mockVerifier struct {
	verifyFunc   func(ctx context.Context, config *VerificationConfig) (*VerificationResult, error)
	verifierType string
}

func (m *mockVerifier) Verify(ctx context.Context, config *VerificationConfig) (*VerificationResult, error) {
	if m.verifyFunc != nil {
		return m.verifyFunc(ctx, config)
	}
	return &VerificationResult{
		Passed:   true,
		Message:  "Verification passed",
		Duration: 100 * time.Millisecond,
		Attempts: 1,
	}, nil
}

func (m *mockVerifier) Type() string {
	if m.verifierType != "" {
		return m.verifierType
	}
	return "default"
}

func TestNewOrchestrator(t *testing.T) {
	orch := NewOrchestrator()
	if orch == nil {
		t.Fatal("expected non-nil orchestrator")
	}
	if orch.deployers == nil {
		t.Error("expected deployers map to be initialized")
	}
	if orch.verifiers == nil {
		t.Error("expected verifiers map to be initialized")
	}
	if orch.plans == nil {
		t.Error("expected plans map to be initialized")
	}
}

func TestOrchestrator_RegisterDeployer(t *testing.T) {
	orch := NewOrchestrator()
	deployer := &mockDeployer{deployerType: "test"}

	orch.RegisterDeployer(deployer)

	// Verify registered
	orch.mu.RLock()
	_, ok := orch.deployers["test"]
	orch.mu.RUnlock()

	if !ok {
		t.Error("expected deployer to be registered")
	}
}

func TestOrchestrator_RegisterVerifier(t *testing.T) {
	orch := NewOrchestrator()
	verifier := &mockVerifier{verifierType: "test"}

	orch.RegisterVerifier(verifier)

	// Verify registered
	orch.mu.RLock()
	_, ok := orch.verifiers["test"]
	orch.mu.RUnlock()

	if !ok {
		t.Error("expected verifier to be registered")
	}
}

func TestOrchestrator_RegisterPlan(t *testing.T) {
	orch := NewOrchestrator()

	plan := &OrchestrationPlan{
		Name: "test-plan",
		Groups: []*DeploymentGroup{
			{
				Name: "group1",
				Repositories: []*Repository{
					{Name: "repo1", URL: "https://github.com/org/repo1"},
				},
			},
		},
	}

	err := orch.RegisterPlan(plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify registered
	retrieved, ok := orch.GetPlan("test-plan")
	if !ok {
		t.Error("expected plan to be registered")
	}
	if retrieved.Name != "test-plan" {
		t.Errorf("expected plan name 'test-plan', got '%s'", retrieved.Name)
	}
}

func TestOrchestrator_RegisterPlan_Errors(t *testing.T) {
	orch := NewOrchestrator()

	// Missing name
	err := orch.RegisterPlan(&OrchestrationPlan{
		Groups: []*DeploymentGroup{
			{Name: "g1", Repositories: []*Repository{{Name: "r1"}}},
		},
	})
	if err == nil {
		t.Error("expected error for missing name")
	}

	// No groups
	err = orch.RegisterPlan(&OrchestrationPlan{
		Name:   "test",
		Groups: []*DeploymentGroup{},
	})
	if err == nil {
		t.Error("expected error for no groups")
	}

	// Empty group
	err = orch.RegisterPlan(&OrchestrationPlan{
		Name: "test",
		Groups: []*DeploymentGroup{
			{Name: "g1", Repositories: []*Repository{}},
		},
	})
	if err == nil {
		t.Error("expected error for empty group")
	}

	// Duplicate group names
	err = orch.RegisterPlan(&OrchestrationPlan{
		Name: "test",
		Groups: []*DeploymentGroup{
			{Name: "g1", Repositories: []*Repository{{Name: "r1"}}},
			{Name: "g1", Repositories: []*Repository{{Name: "r2"}}},
		},
	})
	if err == nil {
		t.Error("expected error for duplicate group names")
	}
}

func TestOrchestrator_CircularDependency(t *testing.T) {
	orch := NewOrchestrator()

	// Create circular dependency: g1 -> g2 -> g3 -> g1
	err := orch.RegisterPlan(&OrchestrationPlan{
		Name: "test",
		Groups: []*DeploymentGroup{
			{Name: "g1", Dependencies: []string{"g3"}, Repositories: []*Repository{{Name: "r1"}}},
			{Name: "g2", Dependencies: []string{"g1"}, Repositories: []*Repository{{Name: "r2"}}},
			{Name: "g3", Dependencies: []string{"g2"}, Repositories: []*Repository{{Name: "r3"}}},
		},
	})
	if err == nil {
		t.Error("expected error for circular dependency")
	}
}

func TestOrchestrator_Execute_Simple(t *testing.T) {
	orch := NewOrchestrator()
	orch.RegisterDeployer(&mockDeployer{})

	plan := &OrchestrationPlan{
		Name: "simple",
		Groups: []*DeploymentGroup{
			{
				Name: "group1",
				Repositories: []*Repository{
					{Name: "repo1"},
				},
			},
		},
	}
	orch.RegisterPlan(plan)

	ctx := context.Background()
	result, err := orch.Execute(ctx, &OrchestrationRequest{
		PlanName:    "simple",
		RequestedBy: "user1",
		Reason:      "Test deployment",
		Revisions:   map[string]string{"repo1": "v1.0.0"},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != StatusCompleted {
		t.Errorf("expected status completed, got %s", result.Status)
	}
	if len(result.GroupResults) != 1 {
		t.Errorf("expected 1 group result, got %d", len(result.GroupResults))
	}
}

func TestOrchestrator_Execute_WithDependencies(t *testing.T) {
	orch := NewOrchestrator()
	orch.RegisterDeployer(&mockDeployer{})

	plan := &OrchestrationPlan{
		Name: "with-deps",
		Groups: []*DeploymentGroup{
			{
				Name: "database",
				Repositories: []*Repository{
					{Name: "db-migrations"},
				},
			},
			{
				Name:         "backend",
				Dependencies: []string{"database"},
				Repositories: []*Repository{
					{Name: "api-server"},
				},
			},
			{
				Name:         "frontend",
				Dependencies: []string{"backend"},
				Repositories: []*Repository{
					{Name: "web-ui"},
				},
			},
		},
	}
	orch.RegisterPlan(plan)

	ctx := context.Background()
	result, err := orch.Execute(ctx, &OrchestrationRequest{
		PlanName:    "with-deps",
		RequestedBy: "user1",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != StatusCompleted {
		t.Errorf("expected status completed, got %s", result.Status)
	}

	// Verify execution order respects dependencies
	if len(result.ExecutionOrder) != 3 {
		t.Fatalf("expected 3 groups in execution order, got %d", len(result.ExecutionOrder))
	}

	dbIdx := -1
	backendIdx := -1
	frontendIdx := -1
	for i, name := range result.ExecutionOrder {
		switch name {
		case "database":
			dbIdx = i
		case "backend":
			backendIdx = i
		case "frontend":
			frontendIdx = i
		}
	}

	if dbIdx > backendIdx {
		t.Error("database should be deployed before backend")
	}
	if backendIdx > frontendIdx {
		t.Error("backend should be deployed before frontend")
	}
}

func TestOrchestrator_Execute_Parallel(t *testing.T) {
	orch := NewOrchestrator()

	var deployCount atomic.Int64
	orch.RegisterDeployer(&mockDeployer{
		deployFunc: func(ctx context.Context, repo *Repository, revision string) (*RepoResult, error) {
			deployCount.Add(1)
			return &RepoResult{
				RepoName: repo.Name,
				Status:   StatusCompleted,
			}, nil
		},
	})

	plan := &OrchestrationPlan{
		Name: "parallel",
		Groups: []*DeploymentGroup{
			{
				Name:     "services",
				Parallel: true,
				Repositories: []*Repository{
					{Name: "service1"},
					{Name: "service2"},
					{Name: "service3"},
				},
			},
		},
	}
	orch.RegisterPlan(plan)

	ctx := context.Background()
	result, _ := orch.Execute(ctx, &OrchestrationRequest{
		PlanName: "parallel",
	})

	if deployCount.Load() != 3 {
		t.Errorf("expected 3 deployments, got %d", deployCount.Load())
	}
	if result.Status != StatusCompleted {
		t.Errorf("expected status completed, got %s", result.Status)
	}
}

func TestOrchestrator_Execute_StopOnFailure(t *testing.T) {
	orch := NewOrchestrator()

	orch.RegisterDeployer(&mockDeployer{
		deployFunc: func(ctx context.Context, repo *Repository, revision string) (*RepoResult, error) {
			if repo.Name == "failing-repo" {
				return nil, fmt.Errorf("deployment failed")
			}
			return &RepoResult{
				RepoName: repo.Name,
				Status:   StatusCompleted,
			}, nil
		},
	})

	plan := &OrchestrationPlan{
		Name: "stop-on-fail",
		Groups: []*DeploymentGroup{
			{
				Name:          "failing-group",
				StopOnFailure: true,
				Repositories: []*Repository{
					{Name: "failing-repo"},
				},
			},
			{
				Name: "second-group",
				Repositories: []*Repository{
					{Name: "should-not-deploy"},
				},
			},
		},
	}
	orch.RegisterPlan(plan)

	ctx := context.Background()
	result, _ := orch.Execute(ctx, &OrchestrationRequest{
		PlanName: "stop-on-fail",
	})

	if result.Status != StatusFailed {
		t.Errorf("expected status failed, got %s", result.Status)
	}
	if len(result.ExecutionOrder) != 1 {
		t.Errorf("expected only 1 group executed, got %d", len(result.ExecutionOrder))
	}
}

func TestOrchestrator_Execute_DryRun(t *testing.T) {
	orch := NewOrchestrator()

	deployCalled := false
	orch.RegisterDeployer(&mockDeployer{
		deployFunc: func(ctx context.Context, repo *Repository, revision string) (*RepoResult, error) {
			deployCalled = true
			return &RepoResult{Status: StatusCompleted}, nil
		},
	})

	plan := &OrchestrationPlan{
		Name: "dry-run",
		Groups: []*DeploymentGroup{
			{
				Name: "group1",
				Repositories: []*Repository{
					{Name: "repo1"},
				},
			},
		},
	}
	orch.RegisterPlan(plan)

	ctx := context.Background()
	result, _ := orch.Execute(ctx, &OrchestrationRequest{
		PlanName: "dry-run",
		DryRun:   true,
	})

	if deployCalled {
		t.Error("deployer should not be called during dry run")
	}
	if result.Status != StatusCompleted {
		t.Errorf("expected status completed, got %s", result.Status)
	}
}

func TestOrchestrator_Execute_WithApproval(t *testing.T) {
	orch := NewOrchestrator()
	orch.RegisterDeployer(&mockDeployer{})

	plan := &OrchestrationPlan{
		Name:            "needs-approval",
		RequireApproval: true,
		Groups: []*DeploymentGroup{
			{
				Name: "group1",
				Repositories: []*Repository{
					{Name: "repo1"},
				},
			},
		},
	}
	orch.RegisterPlan(plan)

	ctx := context.Background()
	result, _ := orch.Execute(ctx, &OrchestrationRequest{
		PlanName: "needs-approval",
	})

	if result.Status != StatusPending {
		t.Errorf("expected status pending, got %s", result.Status)
	}
	if result.ApprovalInfo == nil {
		t.Error("expected approval info to be set")
	}
	if !result.ApprovalInfo.Required {
		t.Error("expected approval to be required")
	}
}

func TestOrchestrator_Execute_ForceBypassApproval(t *testing.T) {
	orch := NewOrchestrator()
	orch.RegisterDeployer(&mockDeployer{})

	plan := &OrchestrationPlan{
		Name:            "needs-approval",
		RequireApproval: true,
		Groups: []*DeploymentGroup{
			{
				Name: "group1",
				Repositories: []*Repository{
					{Name: "repo1"},
				},
			},
		},
	}
	orch.RegisterPlan(plan)

	ctx := context.Background()
	result, _ := orch.Execute(ctx, &OrchestrationRequest{
		PlanName: "needs-approval",
		Force:    true,
	})

	if result.Status != StatusCompleted {
		t.Errorf("expected status completed (force bypass), got %s", result.Status)
	}
}

func TestOrchestrator_Execute_SkipGroups(t *testing.T) {
	orch := NewOrchestrator()

	deployCounts := make(map[string]int)
	orch.RegisterDeployer(&mockDeployer{
		deployFunc: func(ctx context.Context, repo *Repository, revision string) (*RepoResult, error) {
			deployCounts[repo.Name]++
			return &RepoResult{Status: StatusCompleted}, nil
		},
	})

	plan := &OrchestrationPlan{
		Name: "skip-groups",
		Groups: []*DeploymentGroup{
			{
				Name: "group1",
				Repositories: []*Repository{
					{Name: "repo1"},
				},
			},
			{
				Name: "group2",
				Repositories: []*Repository{
					{Name: "repo2"},
				},
			},
		},
	}
	orch.RegisterPlan(plan)

	ctx := context.Background()
	result, _ := orch.Execute(ctx, &OrchestrationRequest{
		PlanName:     "skip-groups",
		GroupsToSkip: []string{"group1"},
	})

	if deployCounts["repo1"] > 0 {
		t.Error("repo1 should have been skipped")
	}
	if deployCounts["repo2"] != 1 {
		t.Errorf("expected repo2 to be deployed once, got %d", deployCounts["repo2"])
	}
	if result.Status != StatusCompleted {
		t.Errorf("expected status completed, got %s", result.Status)
	}
}

func TestOrchestrator_Callbacks(t *testing.T) {
	orch := NewOrchestrator()
	orch.RegisterDeployer(&mockDeployer{})

	groupStarted := false
	groupCompleted := false
	orchestrationCompleted := false

	orch.OnGroupStart(func(orchID, groupName string) {
		groupStarted = true
	})

	orch.OnGroupComplete(func(orchID, groupName string, result *GroupResult) {
		groupCompleted = true
	})

	orch.OnComplete(func(result *OrchestrationResult) {
		orchestrationCompleted = true
	})

	plan := &OrchestrationPlan{
		Name: "callbacks",
		Groups: []*DeploymentGroup{
			{
				Name: "group1",
				Repositories: []*Repository{
					{Name: "repo1"},
				},
			},
		},
	}
	orch.RegisterPlan(plan)

	ctx := context.Background()
	orch.Execute(ctx, &OrchestrationRequest{
		PlanName: "callbacks",
	})

	if err := helpers.WaitForTimeout(500*time.Millisecond, 10*time.Millisecond, func() (bool, error) {
		return groupStarted && groupCompleted && orchestrationCompleted, nil
	}); err != nil {
		t.Fatalf("expected callbacks to execute: %v", err)
	}

	if !groupStarted {
		t.Error("expected group start callback to be called")
	}
	if !groupCompleted {
		t.Error("expected group complete callback to be called")
	}
	if !orchestrationCompleted {
		t.Error("expected orchestration complete callback to be called")
	}
}

func TestOrchestrator_GetOrchestration(t *testing.T) {
	orch := NewOrchestrator()
	orch.RegisterDeployer(&mockDeployer{})

	plan := &OrchestrationPlan{
		Name: "test",
		Groups: []*DeploymentGroup{
			{
				Name: "group1",
				Repositories: []*Repository{
					{Name: "repo1"},
				},
			},
		},
	}
	orch.RegisterPlan(plan)

	ctx := context.Background()
	result, _ := orch.Execute(ctx, &OrchestrationRequest{
		PlanName: "test",
	})

	retrieved, ok := orch.GetOrchestration(result.ID)
	if !ok {
		t.Error("expected orchestration to be found")
	}
	if retrieved.ID != result.ID {
		t.Errorf("expected ID '%s', got '%s'", result.ID, retrieved.ID)
	}
}

func TestOrchestrator_ListOrchestrations(t *testing.T) {
	orch := NewOrchestrator()
	orch.RegisterDeployer(&mockDeployer{})

	plan := &OrchestrationPlan{
		Name: "test",
		Groups: []*DeploymentGroup{
			{
				Name: "group1",
				Repositories: []*Repository{
					{Name: "repo1"},
				},
			},
		},
	}
	orch.RegisterPlan(plan)

	ctx := context.Background()
	orch.Execute(ctx, &OrchestrationRequest{PlanName: "test"})
	orch.Execute(ctx, &OrchestrationRequest{PlanName: "test"})

	results := orch.ListOrchestrations()
	if len(results) != 2 {
		t.Errorf("expected 2 orchestrations, got %d", len(results))
	}
}

func TestOrchestrator_CancelOrchestration(t *testing.T) {
	orch := NewOrchestrator()
	orch.RegisterDeployer(&mockDeployer{})

	plan := &OrchestrationPlan{
		Name:            "cancel-test",
		RequireApproval: true,
		Groups: []*DeploymentGroup{
			{
				Name: "group1",
				Repositories: []*Repository{
					{Name: "repo1"},
				},
			},
		},
	}
	orch.RegisterPlan(plan)

	ctx := context.Background()
	result, _ := orch.Execute(ctx, &OrchestrationRequest{
		PlanName: "cancel-test",
	})

	err := orch.CancelOrchestration(result.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	retrieved, _ := orch.GetOrchestration(result.ID)
	if retrieved.Status != StatusCancelled {
		t.Errorf("expected status cancelled, got %s", retrieved.Status)
	}
}

func TestOrchestrator_WithVerification(t *testing.T) {
	orch := NewOrchestrator()
	orch.RegisterDeployer(&mockDeployer{})
	orch.RegisterVerifier(&mockVerifier{verifierType: "http"})

	plan := &OrchestrationPlan{
		Name: "with-verify",
		Groups: []*DeploymentGroup{
			{
				Name: "group1",
				Repositories: []*Repository{
					{Name: "repo1"},
				},
				Verification: &VerificationConfig{
					Type:     "http",
					Endpoint: "http://localhost/health",
					Timeout:  5 * time.Second,
				},
			},
		},
	}
	orch.RegisterPlan(plan)

	ctx := context.Background()
	result, _ := orch.Execute(ctx, &OrchestrationRequest{
		PlanName: "with-verify",
	})

	if result.Status != StatusCompleted {
		t.Errorf("expected status completed, got %s", result.Status)
	}
	if len(result.GroupResults) > 0 && result.GroupResults[0].VerificationResult == nil {
		t.Error("expected verification result to be present")
	}
}

func TestOrchestrator_VerificationFailure(t *testing.T) {
	orch := NewOrchestrator()
	orch.RegisterDeployer(&mockDeployer{})
	orch.RegisterVerifier(&mockVerifier{
		verifierType: "http",
		verifyFunc: func(ctx context.Context, config *VerificationConfig) (*VerificationResult, error) {
			return &VerificationResult{
				Passed:  false,
				Message: "Health check failed",
			}, nil
		},
	})

	plan := &OrchestrationPlan{
		Name: "verify-fail",
		Groups: []*DeploymentGroup{
			{
				Name: "group1",
				Repositories: []*Repository{
					{Name: "repo1"},
				},
				Verification: &VerificationConfig{
					Type: "http",
				},
			},
		},
	}
	orch.RegisterPlan(plan)

	ctx := context.Background()
	result, _ := orch.Execute(ctx, &OrchestrationRequest{
		PlanName: "verify-fail",
	})

	if result.Status != StatusPartialFail {
		t.Errorf("expected partial failure status, got %s", result.Status)
	}
}

func TestRepository_Fields(t *testing.T) {
	repo := &Repository{
		Name:   "test-repo",
		URL:    "https://github.com/org/repo",
		Branch: "main",
		Path:   "services/api",
		DeploymentConfig: &DeploymentConfig{
			Type:        "argocd",
			Application: "my-app",
			Namespace:   "production",
		},
		Metadata: map[string]string{"env": "prod"},
	}

	if repo.Name != "test-repo" {
		t.Error("expected name to be preserved")
	}
	if repo.DeploymentConfig.Type != "argocd" {
		t.Error("expected deployment config type")
	}
}

func TestDeploymentGroup_Fields(t *testing.T) {
	group := &DeploymentGroup{
		Name:              "backend",
		Description:       "Backend services",
		Parallel:          true,
		StopOnFailure:     true,
		RollbackOnFailure: true,
		Dependencies:      []string{"database"},
		Repositories: []*Repository{
			{Name: "api"},
		},
	}

	if !group.Parallel {
		t.Error("expected parallel to be true")
	}
	if !group.StopOnFailure {
		t.Error("expected stop on failure")
	}
	if len(group.Dependencies) != 1 {
		t.Error("expected 1 dependency")
	}
}

func TestOrchestrationStatus_Values(t *testing.T) {
	statuses := []OrchestrationStatus{
		StatusPending,
		StatusApproved,
		StatusInProgress,
		StatusCompleted,
		StatusFailed,
		StatusRollingBack,
		StatusRolledBack,
		StatusCancelled,
		StatusPartialFail,
	}

	for _, s := range statuses {
		if s == "" {
			t.Error("expected non-empty status value")
		}
	}
}
