package promotion

import (
	"context"
	"testing"
	"time"
)

// MockDeployer for testing
type MockDeployer struct {
	deployErr       error
	currentRevision string
	setWeightErr    error
	deployCalls     int
	setWeightCalls  int
	lastWeight      int
}

// MockRemediator for testing automatic remediation
type MockRemediator struct {
	rollbackErr        error
	scaleDownErr       error
	shiftTrafficErr    error
	executeWorkflowErr error
	rollbackCalls      int
	scaleDownCalls     int
	shiftTrafficCalls  int
	workflowCalls      int
	lastTargetRevision string
	lastReplicas       int
	lastTrafficWeight  int
	lastWorkflow       string
	collectDiagnostics bool
}

func (m *MockRemediator) Rollback(ctx context.Context, env *Environment, toRevision string) error {
	m.rollbackCalls++
	m.lastTargetRevision = toRevision
	return m.rollbackErr
}

func (m *MockRemediator) ScaleDown(ctx context.Context, env *Environment, replicas int) error {
	m.scaleDownCalls++
	m.lastReplicas = replicas
	return m.scaleDownErr
}

func (m *MockRemediator) ShiftTraffic(ctx context.Context, env *Environment, weight int) error {
	m.shiftTrafficCalls++
	m.lastTrafficWeight = weight
	return m.shiftTrafficErr
}

func (m *MockRemediator) ExecuteWorkflow(ctx context.Context, env *Environment, workflow string, params map[string]string) error {
	m.workflowCalls++
	m.lastWorkflow = workflow
	return m.executeWorkflowErr
}

func (m *MockRemediator) CollectDiagnostics(ctx context.Context, env *Environment) (*DiagnosticInfo, error) {
	m.collectDiagnostics = true
	return &DiagnosticInfo{
		CollectedAt: time.Now(),
		Metrics:     map[string]float64{"error_rate": 5.5},
		Events:      []string{"pod unhealthy", "liveness probe failed"},
	}, nil
}

// MockNotifier for testing notifications
type MockNotifier struct {
	notifyCalls int
	lastChannel string
	lastMessage string
}

func (m *MockNotifier) Notify(ctx context.Context, channel string, message string, details map[string]interface{}) error {
	m.notifyCalls++
	m.lastChannel = channel
	m.lastMessage = message
	return nil
}

func (m *MockDeployer) Deploy(ctx context.Context, env *Environment, revision string) error {
	m.deployCalls++
	if m.deployErr != nil {
		return m.deployErr
	}
	m.currentRevision = revision
	return nil
}

func (m *MockDeployer) GetCurrentRevision(ctx context.Context, env *Environment) (string, error) {
	return m.currentRevision, nil
}

func (m *MockDeployer) SetTrafficWeight(ctx context.Context, env *Environment, weight int) error {
	m.setWeightCalls++
	m.lastWeight = weight
	if m.setWeightErr != nil {
		return m.setWeightErr
	}
	return nil
}

func TestEngineRegisterPipeline(t *testing.T) {
	deployer := &MockDeployer{}
	engine := NewEngine(deployer)

	pipeline := &Pipeline{
		Name:        "test-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{Name: "dev"},
			{Name: "prod"},
		},
		Strategy: StrategyImmediate,
	}

	err := engine.RegisterPipeline(pipeline)
	if err != nil {
		t.Fatalf("RegisterPipeline failed: %v", err)
	}

	retrieved, ok := engine.GetPipeline("test-pipeline")
	if !ok {
		t.Error("Pipeline not found")
	}

	if retrieved.Name != "test-pipeline" {
		t.Errorf("Pipeline name = %s, want test-pipeline", retrieved.Name)
	}
}

func TestEngineRegisterPipelineValidation(t *testing.T) {
	deployer := &MockDeployer{}
	engine := NewEngine(deployer)

	tests := []struct {
		name     string
		pipeline *Pipeline
		wantErr  bool
	}{
		{
			name: "valid pipeline",
			pipeline: &Pipeline{
				Name:         "test",
				Environments: []*Environment{{Name: "dev"}},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			pipeline: &Pipeline{
				Environments: []*Environment{{Name: "dev"}},
			},
			wantErr: true,
		},
		{
			name: "missing environments",
			pipeline: &Pipeline{
				Name:         "test",
				Environments: []*Environment{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := engine.RegisterPipeline(tt.pipeline)
			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterPipeline() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEnginePromoteImmediate(t *testing.T) {
	deployer := &MockDeployer{}
	engine := NewEngine(deployer)

	pipeline := &Pipeline{
		Name:        "test-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{Name: "dev", AutoPromote: true},
		},
		Strategy: StrategyImmediate,
		Timeout:  30 * time.Second,
	}

	err := engine.RegisterPipeline(pipeline)
	if err != nil {
		t.Fatalf("RegisterPipeline failed: %v", err)
	}

	req := &Request{
		Pipeline:      "test-pipeline",
		ToEnvironment: "dev",
		Revision:      "abc123",
		RequestedBy:   "test-user",
		Reason:        "Test deployment",
	}

	ctx := context.Background()
	result, err := engine.Promote(ctx, req)
	if err != nil {
		t.Fatalf("Promote failed: %v", err)
	}

	if result.Status != StatusCompleted {
		t.Errorf("Status = %s, want %s", result.Status, StatusCompleted)
	}

	if deployer.deployCalls != 1 {
		t.Errorf("Deploy calls = %d, want 1", deployer.deployCalls)
	}

	if deployer.currentRevision != "abc123" {
		t.Errorf("Deployed revision = %s, want abc123", deployer.currentRevision)
	}
}

func TestEnginePromoteWithApproval(t *testing.T) {
	deployer := &MockDeployer{}
	engine := NewEngine(deployer)

	pipeline := &Pipeline{
		Name:        "test-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{
				Name:            "prod",
				RequireApproval: true,
				Approvers:       []string{"admin"},
			},
		},
		Strategy: StrategyImmediate,
		Timeout:  30 * time.Second,
	}

	err := engine.RegisterPipeline(pipeline)
	if err != nil {
		t.Fatalf("RegisterPipeline failed: %v", err)
	}

	req := &Request{
		Pipeline:      "test-pipeline",
		ToEnvironment: "prod",
		Revision:      "abc123",
		RequestedBy:   "test-user",
		Reason:        "Test deployment",
	}

	ctx := context.Background()
	result, err := engine.Promote(ctx, req)
	if err != nil {
		t.Fatalf("Promote failed: %v", err)
	}

	if result.Status != StatusPending {
		t.Errorf("Status = %s, want %s", result.Status, StatusPending)
	}

	if result.ApprovalInfo == nil {
		t.Fatal("ApprovalInfo should not be nil")
	}

	if !result.ApprovalInfo.Required {
		t.Error("Approval should be required")
	}

	if deployer.deployCalls != 0 {
		t.Errorf("Deploy should not be called yet, got %d calls", deployer.deployCalls)
	}
}

func TestEngineApprovePromotion(t *testing.T) {
	deployer := &MockDeployer{}
	engine := NewEngine(deployer)

	pipeline := &Pipeline{
		Name:        "test-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{
				Name:            "prod",
				RequireApproval: true,
			},
		},
		Strategy: StrategyImmediate,
		Timeout:  30 * time.Second,
	}

	err := engine.RegisterPipeline(pipeline)
	if err != nil {
		t.Fatalf("RegisterPipeline failed: %v", err)
	}

	ctx := context.Background()
	result, err := engine.Promote(ctx, &Request{
		Pipeline:      "test-pipeline",
		ToEnvironment: "prod",
		Revision:      "abc123",
		RequestedBy:   "test-user",
	})
	if err != nil {
		t.Fatalf("Promote failed: %v", err)
	}

	// Approve promotion
	approvalReq := &ApprovalRequest{
		PromotionID: result.ID,
		Approved:    true,
		ApprovedBy:  "admin",
		Reason:      "Approved for testing",
	}

	err = engine.ApprovePromotion(ctx, approvalReq)
	if err != nil {
		t.Fatalf("ApprovePromotion failed: %v", err)
	}

	// Check result was updated
	updated, ok := engine.GetPromotion(result.ID)
	if !ok {
		t.Fatal("Promotion not found")
	}

	if updated.Status != StatusCompleted {
		t.Errorf("Status = %s, want %s", updated.Status, StatusCompleted)
	}

	if updated.ApprovalInfo.ApprovedBy != "admin" {
		t.Errorf("ApprovedBy = %s, want admin", updated.ApprovalInfo.ApprovedBy)
	}

	if deployer.deployCalls != 1 {
		t.Errorf("Deploy calls = %d, want 1", deployer.deployCalls)
	}
}

func TestEngineRejectPromotion(t *testing.T) {
	deployer := &MockDeployer{}
	engine := NewEngine(deployer)

	pipeline := &Pipeline{
		Name:        "test-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{
				Name:            "prod",
				RequireApproval: true,
			},
		},
		Strategy: StrategyImmediate,
		Timeout:  30 * time.Second,
	}

	err := engine.RegisterPipeline(pipeline)
	if err != nil {
		t.Fatalf("RegisterPipeline failed: %v", err)
	}

	ctx := context.Background()
	result, err := engine.Promote(ctx, &Request{
		Pipeline:      "test-pipeline",
		ToEnvironment: "prod",
		Revision:      "abc123",
		RequestedBy:   "test-user",
	})
	if err != nil {
		t.Fatalf("Promote failed: %v", err)
	}

	// Reject promotion
	approvalReq := &ApprovalRequest{
		PromotionID: result.ID,
		Approved:    false,
		ApprovedBy:  "admin",
		Reason:      "Not ready for production",
	}

	err = engine.ApprovePromotion(ctx, approvalReq)
	if err != nil {
		t.Fatalf("ApprovePromotion failed: %v", err)
	}

	// Check result was updated
	updated, ok := engine.GetPromotion(result.ID)
	if !ok {
		t.Fatal("Promotion not found")
	}

	if updated.Status != StatusRejected {
		t.Errorf("Status = %s, want %s", updated.Status, StatusRejected)
	}

	if updated.ApprovalInfo.RejectedBy != "admin" {
		t.Errorf("RejectedBy = %s, want admin", updated.ApprovalInfo.RejectedBy)
	}

	if deployer.deployCalls != 0 {
		t.Errorf("Deploy should not be called, got %d calls", deployer.deployCalls)
	}
}

func TestEngineCanaryDeployment(t *testing.T) {
	deployer := &MockDeployer{}
	engine := NewEngine(deployer)

	pipeline := &Pipeline{
		Name:        "test-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{Name: "prod"},
		},
		Strategy: StrategyCanary,
		CanarySteps: []CanaryStep{
			{Weight: 25, Duration: 100 * time.Millisecond},
			{Weight: 50, Duration: 100 * time.Millisecond},
			{Weight: 75, Duration: 100 * time.Millisecond},
		},
		Timeout: 10 * time.Second,
	}

	err := engine.RegisterPipeline(pipeline)
	if err != nil {
		t.Fatalf("RegisterPipeline failed: %v", err)
	}

	ctx := context.Background()
	result, err := engine.Promote(ctx, &Request{
		Pipeline:      "test-pipeline",
		ToEnvironment: "prod",
		Revision:      "abc123",
		RequestedBy:   "test-user",
	})
	if err != nil {
		t.Fatalf("Promote failed: %v", err)
	}

	if result.Status != StatusCompleted {
		t.Errorf("Status = %s, want %s", result.Status, StatusCompleted)
	}

	// Should have 4 weight calls (25%, 50%, 75%, 100%)
	if deployer.setWeightCalls != 4 {
		t.Errorf("SetTrafficWeight calls = %d, want 4", deployer.setWeightCalls)
	}

	// Final weight should be 100%
	if deployer.lastWeight != 100 {
		t.Errorf("Last weight = %d, want 100", deployer.lastWeight)
	}

	// Check canary progress
	if len(result.Stages) != 1 {
		t.Fatalf("Stages count = %d, want 1", len(result.Stages))
	}

	stage := result.Stages[0]
	if len(stage.CanaryProgress) != 3 {
		t.Errorf("Canary progress count = %d, want 3", len(stage.CanaryProgress))
	}
}

func TestEngineListPromotions(t *testing.T) {
	deployer := &MockDeployer{}
	engine := NewEngine(deployer)

	pipeline := &Pipeline{
		Name:        "test-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{Name: "dev"},
		},
		Strategy: StrategyImmediate,
	}

	err := engine.RegisterPipeline(pipeline)
	if err != nil {
		t.Fatalf("RegisterPipeline failed: %v", err)
	}

	ctx := context.Background()

	// Create multiple promotions
	for i := 0; i < 3; i++ {
		_, err := engine.Promote(ctx, &Request{
			Pipeline:      "test-pipeline",
			ToEnvironment: "dev",
			Revision:      "abc123",
			RequestedBy:   "test-user",
		})
		if err != nil {
			t.Fatalf("Promote failed: %v", err)
		}
	}

	promotions := engine.ListPromotions()
	if len(promotions) != 3 {
		t.Errorf("Promotions count = %d, want 3", len(promotions))
	}
}

func TestEngineListPendingPromotions(t *testing.T) {
	deployer := &MockDeployer{}
	engine := NewEngine(deployer)

	// Create pipeline with approval
	approvalPipeline := &Pipeline{
		Name:        "approval-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{Name: "prod", RequireApproval: true},
		},
		Strategy: StrategyImmediate,
	}

	// Create pipeline without approval
	immediatePipeline := &Pipeline{
		Name:        "immediate-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{Name: "dev"},
		},
		Strategy: StrategyImmediate,
	}

	engine.RegisterPipeline(approvalPipeline)
	engine.RegisterPipeline(immediatePipeline)

	ctx := context.Background()

	// Create pending promotion
	_, err := engine.Promote(ctx, &Request{
		Pipeline:      "approval-pipeline",
		ToEnvironment: "prod",
		Revision:      "abc123",
		RequestedBy:   "test-user",
	})
	if err != nil {
		t.Fatalf("Promote failed: %v", err)
	}

	// Create completed promotion
	_, err = engine.Promote(ctx, &Request{
		Pipeline:      "immediate-pipeline",
		ToEnvironment: "dev",
		Revision:      "abc123",
		RequestedBy:   "test-user",
	})
	if err != nil {
		t.Fatalf("Promote failed: %v", err)
	}

	pending := engine.ListPendingPromotions()
	if len(pending) != 1 {
		t.Errorf("Pending promotions count = %d, want 1", len(pending))
	}

	if pending[0].Pipeline.Name != "approval-pipeline" {
		t.Errorf("Pending promotion pipeline = %s, want approval-pipeline", pending[0].Pipeline.Name)
	}
}

func TestEngineConfigurableThresholds(t *testing.T) {
	deployer := &MockDeployer{}
	engine := NewEngine(deployer)

	// Test pipeline with custom thresholds
	customThresholds := &ThresholdConfig{
		Name:        "custom-config",
		Description: "Custom thresholds for this pipeline",
		Thresholds: []Threshold{
			{Metric: MetricErrorRate, Operator: OperatorLessThan, Value: 2.0},
			{Metric: MetricLatencyP95, Operator: OperatorLessThan, Value: 1000},
		},
		FailurePolicy:       FailurePolicyPause,
		EvaluationInterval:  45 * time.Second,
		WarmupPeriod:        90 * time.Second,
		ConsecutiveFailures: 4,
	}

	pipeline := &Pipeline{
		Name:        "custom-threshold-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{Name: "dev"},
			{
				Name: "staging",
				Thresholds: &ThresholdConfig{
					Name: "staging-config",
					Thresholds: []Threshold{
						{Metric: MetricErrorRate, Operator: OperatorLessThan, Value: 5.0},
					},
					FailurePolicy: FailurePolicyIgnore,
				},
			},
			{
				Name:              "prod",
				ThresholdPreset:   "strict",
				InheritThresholds: false,
			},
		},
		Strategy:   StrategyCanary,
		Thresholds: customThresholds,
	}

	err := engine.RegisterPipeline(pipeline)
	if err != nil {
		t.Fatalf("RegisterPipeline failed: %v", err)
	}

	// Test getEffectiveThresholds for different environments
	tests := []struct {
		envName      string
		expectPreset bool
		presetName   string
		expectCustom bool
		expectPolicy FailurePolicy
	}{
		{
			envName:      "dev",
			expectCustom: true,
			expectPolicy: FailurePolicyPause,
		},
		{
			envName:      "staging",
			expectCustom: true,
			expectPolicy: FailurePolicyIgnore,
		},
		{
			envName:      "prod",
			expectPreset: true,
			presetName:   "strict",
			expectPolicy: FailurePolicyRollback,
		},
	}

	for _, tt := range tests {
		t.Run(tt.envName, func(t *testing.T) {
			var env *Environment
			for _, e := range pipeline.Environments {
				if e.Name == tt.envName {
					env = e
					break
				}
			}

			thresholds := engine.getEffectiveThresholds(pipeline, env)
			if thresholds == nil {
				t.Fatal("getEffectiveThresholds returned nil")
			}

			if thresholds.FailurePolicy != tt.expectPolicy {
				t.Errorf("FailurePolicy = %s, want %s", thresholds.FailurePolicy, tt.expectPolicy)
			}

			if tt.expectPreset {
				preset, _ := GetPreset(tt.presetName)
				if len(thresholds.Thresholds) != len(preset.Thresholds) {
					t.Errorf("Threshold count = %d, want %d (from preset %s)",
						len(thresholds.Thresholds), len(preset.Thresholds), tt.presetName)
				}
			}
		})
	}
}

func TestEngineThresholdPresets(t *testing.T) {
	deployer := &MockDeployer{}
	engine := NewEngine(deployer)

	// Test pipeline using preset
	pipeline := &Pipeline{
		Name:        "preset-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{Name: "dev"},
		},
		Strategy:        StrategyCanary,
		ThresholdPreset: "latency-sensitive",
	}

	err := engine.RegisterPipeline(pipeline)
	if err != nil {
		t.Fatalf("RegisterPipeline failed: %v", err)
	}

	env := pipeline.Environments[0]
	thresholds := engine.getEffectiveThresholds(pipeline, env)

	if thresholds == nil {
		t.Fatal("getEffectiveThresholds returned nil")
	}

	preset, _ := GetPreset("latency-sensitive")
	if thresholds.Name != preset.Name {
		t.Errorf("Threshold config name = %s, want %s", thresholds.Name, preset.Name)
	}

	// latency-sensitive preset should have latency thresholds
	hasLatencyP50 := false
	for _, th := range thresholds.Thresholds {
		if th.Metric == MetricLatencyP50 {
			hasLatencyP50 = true
			break
		}
	}
	if !hasLatencyP50 {
		t.Error("latency-sensitive preset should include latency_p50 threshold")
	}
}

func TestEngineInheritThresholds(t *testing.T) {
	deployer := &MockDeployer{}
	engine := NewEngine(deployer)

	pipelineThresholds := &ThresholdConfig{
		Name: "pipeline-config",
		Thresholds: []Threshold{
			{Metric: MetricErrorRate, Operator: OperatorLessThan, Value: 1.0},
			{Metric: MetricLatencyP95, Operator: OperatorLessThan, Value: 500},
		},
		FailurePolicy:      FailurePolicyRollback,
		EvaluationInterval: 30 * time.Second,
	}

	envThresholds := &ThresholdConfig{
		Name: "env-config",
		Thresholds: []Threshold{
			// Override error rate threshold
			{Metric: MetricErrorRate, Operator: OperatorLessThan, Value: 5.0},
			// Add new success rate threshold
			{Metric: MetricSuccessRate, Operator: OperatorGreaterOrEqual, Value: 95.0},
		},
		FailurePolicy: FailurePolicyPause,
	}

	pipeline := &Pipeline{
		Name:        "inherit-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{
				Name:              "staging",
				Thresholds:        envThresholds,
				InheritThresholds: true,
			},
		},
		Strategy:   StrategyCanary,
		Thresholds: pipelineThresholds,
	}

	err := engine.RegisterPipeline(pipeline)
	if err != nil {
		t.Fatalf("RegisterPipeline failed: %v", err)
	}

	env := pipeline.Environments[0]
	thresholds := engine.getEffectiveThresholds(pipeline, env)

	if thresholds == nil {
		t.Fatal("getEffectiveThresholds returned nil")
	}

	// Should have merged thresholds: error_rate (from env), latency_p95 (from pipeline), success_rate (from env)
	thresholdMap := make(map[ThresholdMetric]Threshold)
	for _, th := range thresholds.Thresholds {
		thresholdMap[th.Metric] = th
	}

	// Error rate should be from env (5.0)
	if errorRate, ok := thresholdMap[MetricErrorRate]; ok {
		if errorRate.Value != 5.0 {
			t.Errorf("ErrorRate threshold = %.1f, want 5.0 (from env)", errorRate.Value)
		}
	} else {
		t.Error("ErrorRate threshold not found")
	}

	// Latency should be from pipeline (500)
	if latency, ok := thresholdMap[MetricLatencyP95]; ok {
		if latency.Value != 500 {
			t.Errorf("LatencyP95 threshold = %.0f, want 500 (from pipeline)", latency.Value)
		}
	} else {
		t.Error("LatencyP95 threshold not found")
	}

	// Success rate should be from env (95.0)
	if successRate, ok := thresholdMap[MetricSuccessRate]; ok {
		if successRate.Value != 95.0 {
			t.Errorf("SuccessRate threshold = %.1f, want 95.0 (from env)", successRate.Value)
		}
	} else {
		t.Error("SuccessRate threshold not found")
	}

	// Failure policy should be from env (pause)
	if thresholds.FailurePolicy != FailurePolicyPause {
		t.Errorf("FailurePolicy = %s, want %s", thresholds.FailurePolicy, FailurePolicyPause)
	}

	// Evaluation interval should be from pipeline (30s) since env didn't specify
	if thresholds.EvaluationInterval != 30*time.Second {
		t.Errorf("EvaluationInterval = %v, want 30s", thresholds.EvaluationInterval)
	}
}

func TestEngineCanaryStepThresholds(t *testing.T) {
	deployer := &MockDeployer{}
	engine := NewEngine(deployer)

	// Pipeline with per-step thresholds
	pipeline := &Pipeline{
		Name:        "step-threshold-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{Name: "prod"},
		},
		Strategy: StrategyCanary,
		CanarySteps: []CanaryStep{
			{
				Weight:   25,
				Duration: 100 * time.Millisecond,
				// Use stricter thresholds at low traffic
				Thresholds: &ThresholdConfig{
					Name: "step-1-strict",
					Thresholds: []Threshold{
						{Metric: MetricErrorRate, Operator: OperatorLessThan, Value: 0.1},
					},
				},
			},
			{
				Weight:           50,
				Duration:         100 * time.Millisecond,
				SkipVerification: true, // Skip verification at this step
			},
			{
				Weight:   75,
				Duration: 100 * time.Millisecond,
				// Use relaxed thresholds at high traffic
				Thresholds: &ThresholdConfig{
					Name: "step-3-relaxed",
					Thresholds: []Threshold{
						{Metric: MetricErrorRate, Operator: OperatorLessThan, Value: 2.0},
					},
				},
			},
		},
		Thresholds: &ThresholdConfig{
			Name: "default-config",
			Thresholds: []Threshold{
				{Metric: MetricErrorRate, Operator: OperatorLessThan, Value: 1.0},
			},
		},
	}

	err := engine.RegisterPipeline(pipeline)
	if err != nil {
		t.Fatalf("RegisterPipeline failed: %v", err)
	}

	// Verify step thresholds are properly set
	if pipeline.CanarySteps[0].Thresholds == nil {
		t.Error("Step 1 should have custom thresholds")
	} else if pipeline.CanarySteps[0].Thresholds.Thresholds[0].Value != 0.1 {
		t.Errorf("Step 1 error rate threshold = %.1f, want 0.1",
			pipeline.CanarySteps[0].Thresholds.Thresholds[0].Value)
	}

	if !pipeline.CanarySteps[1].SkipVerification {
		t.Error("Step 2 should skip verification")
	}

	if pipeline.CanarySteps[2].Thresholds == nil {
		t.Error("Step 3 should have custom thresholds")
	} else if pipeline.CanarySteps[2].Thresholds.Thresholds[0].Value != 2.0 {
		t.Errorf("Step 3 error rate threshold = %.1f, want 2.0",
			pipeline.CanarySteps[2].Thresholds.Thresholds[0].Value)
	}
}

func TestEngineRemediationConfig(t *testing.T) {
	deployer := &MockDeployer{}
	engine := NewEngine(deployer)

	// Test pipeline with custom remediation config
	pipeline := &Pipeline{
		Name:        "remediation-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{
				Name: "staging",
				Remediation: &RemediationConfig{
					Enabled:              true,
					Strategy:             RemediationScaleDown,
					MaxAttempts:          2,
					RetryDelay:           5 * time.Second,
					NotifyOnRemediation:  true,
					NotificationChannels: []string{"slack"},
				},
			},
			{
				Name: "prod",
				// Uses pipeline-level remediation
			},
		},
		Strategy: StrategyCanary,
		Remediation: &RemediationConfig{
			Enabled:             true,
			Strategy:            RemediationRollback,
			MaxAttempts:         3,
			TimeoutPerAttempt:   1 * time.Minute,
			NotifyOnRemediation: true,
			CollectDiagnostics:  true,
		},
	}

	err := engine.RegisterPipeline(pipeline)
	if err != nil {
		t.Fatalf("RegisterPipeline failed: %v", err)
	}

	// Test getEffectiveRemediation for staging (should use env-specific config)
	stagingEnv := pipeline.Environments[0]
	stagingRemediation := engine.getEffectiveRemediation(pipeline, stagingEnv)
	if stagingRemediation == nil {
		t.Fatal("getEffectiveRemediation returned nil for staging")
	}
	if stagingRemediation.Strategy != RemediationScaleDown {
		t.Errorf("Staging remediation strategy = %s, want %s", stagingRemediation.Strategy, RemediationScaleDown)
	}
	if stagingRemediation.MaxAttempts != 2 {
		t.Errorf("Staging remediation max attempts = %d, want 2", stagingRemediation.MaxAttempts)
	}

	// Test getEffectiveRemediation for prod (should use pipeline config)
	prodEnv := pipeline.Environments[1]
	prodRemediation := engine.getEffectiveRemediation(pipeline, prodEnv)
	if prodRemediation == nil {
		t.Fatal("getEffectiveRemediation returned nil for prod")
	}
	if prodRemediation.Strategy != RemediationRollback {
		t.Errorf("Prod remediation strategy = %s, want %s", prodRemediation.Strategy, RemediationRollback)
	}
	if prodRemediation.MaxAttempts != 3 {
		t.Errorf("Prod remediation max attempts = %d, want 3", prodRemediation.MaxAttempts)
	}
}

func TestEngineRemediationBackwardsCompatibility(t *testing.T) {
	deployer := &MockDeployer{}
	engine := NewEngine(deployer)

	// Test pipeline using old RollbackOnFailure flag
	pipeline := &Pipeline{
		Name:        "old-style-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{Name: "prod"},
		},
		Strategy:          StrategyCanary,
		RollbackOnFailure: true, // Old style config
	}

	err := engine.RegisterPipeline(pipeline)
	if err != nil {
		t.Fatalf("RegisterPipeline failed: %v", err)
	}

	// Should get default remediation config
	env := pipeline.Environments[0]
	remediation := engine.getEffectiveRemediation(pipeline, env)
	if remediation == nil {
		t.Fatal("getEffectiveRemediation returned nil with RollbackOnFailure=true")
	}

	// Default config should use rollback strategy
	if remediation.Strategy != RemediationRollback {
		t.Errorf("Default remediation strategy = %s, want %s", remediation.Strategy, RemediationRollback)
	}

	if !remediation.Enabled {
		t.Error("Default remediation should be enabled")
	}
}

func TestEngineExecuteRemediation(t *testing.T) {
	deployer := &MockDeployer{currentRevision: "v1.0.0"}
	remediator := &MockRemediator{}
	notifier := &MockNotifier{}

	engine := NewEngine(deployer,
		WithRemediator(remediator),
		WithNotifier(notifier),
	)

	pipeline := &Pipeline{
		Name:        "test-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{Name: "prod"},
		},
		Strategy: StrategyCanary,
		Remediation: &RemediationConfig{
			Enabled:              true,
			Strategy:             RemediationRollback,
			MaxAttempts:          2,
			TimeoutPerAttempt:    10 * time.Second,
			NotifyOnRemediation:  true,
			NotificationChannels: []string{"slack", "pagerduty"},
			CollectDiagnostics:   true,
		},
	}

	err := engine.RegisterPipeline(pipeline)
	if err != nil {
		t.Fatalf("RegisterPipeline failed: %v", err)
	}

	// Execute remediation
	ctx := context.Background()
	env := pipeline.Environments[0]
	trigger := RemediationTrigger{
		Type:   "threshold_failure",
		Reason: "Error rate exceeded 5%",
	}
	stage := &StageResult{Environment: env.Name}

	result := engine.executeRemediation(ctx, pipeline, env, trigger, "v0.9.0", stage)

	// Verify remediation succeeded
	if result.Status != RemediationStatusSucceeded {
		t.Errorf("Remediation status = %s, want %s", result.Status, RemediationStatusSucceeded)
	}

	// Verify rollback was called
	if remediator.rollbackCalls != 1 {
		t.Errorf("Rollback calls = %d, want 1", remediator.rollbackCalls)
	}

	if remediator.lastTargetRevision != "v0.9.0" {
		t.Errorf("Rollback target revision = %s, want v0.9.0", remediator.lastTargetRevision)
	}

	// Verify diagnostics were collected
	if !remediator.collectDiagnostics {
		t.Error("Diagnostics should have been collected")
	}

	if result.Diagnostics == nil {
		t.Error("Result should contain diagnostics")
	}

	// Verify notifications were sent
	if notifier.notifyCalls != 2 { // slack + pagerduty
		t.Errorf("Notify calls = %d, want 2", notifier.notifyCalls)
	}

	// Verify attempt details
	if len(result.AttemptDetails) != 1 {
		t.Errorf("Attempt details count = %d, want 1", len(result.AttemptDetails))
	}

	if result.AttemptDetails[0].Status != RemediationStatusSucceeded {
		t.Errorf("Attempt status = %s, want %s", result.AttemptDetails[0].Status, RemediationStatusSucceeded)
	}
}

func TestEngineRemediationRetry(t *testing.T) {
	deployer := &MockDeployer{currentRevision: "v1.0.0"}
	remediator := &MockRemediator{}
	failCount := 0

	// Create a remediator that fails first attempt then succeeds
	customRemediator := &MockRemediator{
		rollbackErr: nil,
	}

	// Override Rollback to fail first time
	engine := NewEngine(deployer,
		WithRemediator(customRemediator),
	)

	// Make rollback fail first time
	customRemediator.rollbackErr = context.DeadlineExceeded

	pipeline := &Pipeline{
		Name:        "retry-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{Name: "prod"},
		},
		Strategy: StrategyCanary,
		Remediation: &RemediationConfig{
			Enabled:           true,
			Strategy:          RemediationRollback,
			MaxAttempts:       3,
			RetryDelay:        10 * time.Millisecond,
			TimeoutPerAttempt: 100 * time.Millisecond,
		},
	}

	engine.RegisterPipeline(pipeline)

	ctx := context.Background()
	env := pipeline.Environments[0]
	trigger := RemediationTrigger{Type: "test"}
	stage := &StageResult{Environment: env.Name}

	result := engine.executeRemediation(ctx, pipeline, env, trigger, "v0.9.0", stage)

	// Since our mock always fails, remediation should fail
	if result.Status != RemediationStatusFailed {
		t.Errorf("Remediation status = %s, want %s", result.Status, RemediationStatusFailed)
	}

	// Should have tried 3 times
	if customRemediator.rollbackCalls != 3 {
		t.Errorf("Rollback calls = %d, want 3", customRemediator.rollbackCalls)
	}

	if result.Attempts != 3 {
		t.Errorf("Attempts = %d, want 3", result.Attempts)
	}

	// Should have 3 attempt details
	if len(result.AttemptDetails) != 3 {
		t.Errorf("AttemptDetails count = %d, want 3", len(result.AttemptDetails))
	}

	// Silence unused variable warning
	_ = failCount
	_ = remediator
}

func TestEngineRemediationStrategies(t *testing.T) {
	tests := []struct {
		name           string
		strategy       RemediationStrategy
		expectRollback int
		expectScale    int
		expectShift    int
		expectWorkflow int
	}{
		{
			name:           "rollback strategy",
			strategy:       RemediationRollback,
			expectRollback: 1,
		},
		{
			name:        "scale_down strategy",
			strategy:    RemediationScaleDown,
			expectScale: 1,
		},
		{
			name:        "traffic_shift strategy",
			strategy:    RemediationTrafficShift,
			expectShift: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployer := &MockDeployer{currentRevision: "v1.0.0"}
			remediator := &MockRemediator{}

			engine := NewEngine(deployer, WithRemediator(remediator))

			pipeline := &Pipeline{
				Name:        "strategy-test-pipeline",
				Application: "test-app",
				Environments: []*Environment{
					{Name: "prod"},
				},
				Remediation: &RemediationConfig{
					Enabled:     true,
					Strategy:    tt.strategy,
					MaxAttempts: 1,
				},
			}

			engine.RegisterPipeline(pipeline)

			ctx := context.Background()
			env := pipeline.Environments[0]
			trigger := RemediationTrigger{Type: "test"}
			stage := &StageResult{Environment: env.Name}

			engine.executeRemediation(ctx, pipeline, env, trigger, "v0.9.0", stage)

			if remediator.rollbackCalls != tt.expectRollback {
				t.Errorf("Rollback calls = %d, want %d", remediator.rollbackCalls, tt.expectRollback)
			}
			if remediator.scaleDownCalls != tt.expectScale {
				t.Errorf("ScaleDown calls = %d, want %d", remediator.scaleDownCalls, tt.expectScale)
			}
			if remediator.shiftTrafficCalls != tt.expectShift {
				t.Errorf("ShiftTraffic calls = %d, want %d", remediator.shiftTrafficCalls, tt.expectShift)
			}
		})
	}
}

func TestEngineRemediationDisabled(t *testing.T) {
	deployer := &MockDeployer{currentRevision: "v1.0.0"}
	remediator := &MockRemediator{}

	engine := NewEngine(deployer, WithRemediator(remediator))

	pipeline := &Pipeline{
		Name:        "disabled-remediation-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{Name: "prod"},
		},
		Remediation: &RemediationConfig{
			Enabled: false, // Disabled
		},
	}

	engine.RegisterPipeline(pipeline)

	ctx := context.Background()
	env := pipeline.Environments[0]
	trigger := RemediationTrigger{Type: "test"}
	stage := &StageResult{Environment: env.Name}

	result := engine.executeRemediation(ctx, pipeline, env, trigger, "v0.9.0", stage)

	// Should be skipped
	if result.Status != RemediationStatusSkipped {
		t.Errorf("Remediation status = %s, want %s", result.Status, RemediationStatusSkipped)
	}

	// No remediation actions should be taken
	if remediator.rollbackCalls != 0 {
		t.Errorf("Rollback calls = %d, want 0", remediator.rollbackCalls)
	}
}

func TestEngineHandleVerificationFailure(t *testing.T) {
	deployer := &MockDeployer{currentRevision: "v1.0.0"}
	remediator := &MockRemediator{}

	engine := NewEngine(deployer, WithRemediator(remediator))

	tests := []struct {
		name           string
		failurePolicy  FailurePolicy
		expectRollback int
		expectStatus   RemediationStatus
		expectError    bool
	}{
		{
			name:           "rollback policy triggers remediation",
			failurePolicy:  FailurePolicyRollback,
			expectRollback: 1,
			expectStatus:   RemediationStatusSucceeded,
			expectError:    true, // Still returns error but remediation succeeded
		},
		{
			name:           "pause policy marks pending",
			failurePolicy:  FailurePolicyPause,
			expectRollback: 0,
			expectStatus:   RemediationStatusPending,
			expectError:    true,
		},
		{
			name:           "ignore policy skips and continues",
			failurePolicy:  FailurePolicyIgnore,
			expectRollback: 0,
			expectStatus:   RemediationStatusSkipped,
			expectError:    false, // No error, continues deployment
		},
		{
			name:           "abort policy returns error without remediation",
			failurePolicy:  FailurePolicyAbort,
			expectRollback: 0,
			expectStatus:   "", // No remediation result
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock
			remediator.rollbackCalls = 0

			pipeline := &Pipeline{
				Name:        "verify-fail-pipeline",
				Application: "test-app",
				Environments: []*Environment{
					{Name: "prod"},
				},
				Thresholds: &ThresholdConfig{
					FailurePolicy: tt.failurePolicy,
				},
				Remediation: &RemediationConfig{
					Enabled:     true,
					Strategy:    RemediationRollback,
					MaxAttempts: 1,
				},
			}

			engine.RegisterPipeline(pipeline)

			ctx := context.Background()
			env := pipeline.Environments[0]
			evalResult := &EvaluationResult{
				Passed:  false,
				Message: "Error rate exceeded threshold",
			}
			stage := &StageResult{Environment: env.Name}

			err := engine.handleVerificationFailure(ctx, pipeline, env, evalResult, 1, 25, "v0.9.0", stage)

			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			if remediator.rollbackCalls != tt.expectRollback {
				t.Errorf("Rollback calls = %d, want %d", remediator.rollbackCalls, tt.expectRollback)
			}

			if tt.expectStatus != "" {
				if stage.RemediationResult == nil {
					t.Fatal("Expected remediation result but got nil")
				}
				if stage.RemediationResult.Status != tt.expectStatus {
					t.Errorf("Remediation status = %s, want %s", stage.RemediationResult.Status, tt.expectStatus)
				}
			}
		})
	}
}

func TestEngineCustomWorkflowRemediation(t *testing.T) {
	deployer := &MockDeployer{currentRevision: "v1.0.0"}
	remediator := &MockRemediator{}

	engine := NewEngine(deployer, WithRemediator(remediator))

	pipeline := &Pipeline{
		Name:        "custom-workflow-pipeline",
		Application: "test-app",
		Environments: []*Environment{
			{Name: "prod"},
		},
		Remediation: &RemediationConfig{
			Enabled:        true,
			Strategy:       RemediationCustom,
			CustomWorkflow: "rollback-with-notification",
			MaxAttempts:    1,
		},
	}

	engine.RegisterPipeline(pipeline)

	ctx := context.Background()
	env := pipeline.Environments[0]
	trigger := RemediationTrigger{Type: "test"}
	stage := &StageResult{Environment: env.Name}

	result := engine.executeRemediation(ctx, pipeline, env, trigger, "v0.9.0", stage)

	if result.Status != RemediationStatusSucceeded {
		t.Errorf("Remediation status = %s, want %s", result.Status, RemediationStatusSucceeded)
	}

	if remediator.workflowCalls != 1 {
		t.Errorf("Workflow calls = %d, want 1", remediator.workflowCalls)
	}

	if remediator.lastWorkflow != "rollback-with-notification" {
		t.Errorf("Workflow = %s, want rollback-with-notification", remediator.lastWorkflow)
	}
}

func TestDefaultRemediationConfig(t *testing.T) {
	config := DefaultRemediationConfig()

	if !config.Enabled {
		t.Error("Default config should be enabled")
	}

	if config.Strategy != RemediationRollback {
		t.Errorf("Default strategy = %s, want %s", config.Strategy, RemediationRollback)
	}

	if config.MaxAttempts != 3 {
		t.Errorf("Default max attempts = %d, want 3", config.MaxAttempts)
	}

	if config.RetryDelay != 10*time.Second {
		t.Errorf("Default retry delay = %v, want 10s", config.RetryDelay)
	}

	if config.TimeoutPerAttempt != 2*time.Minute {
		t.Errorf("Default timeout per attempt = %v, want 2m", config.TimeoutPerAttempt)
	}

	if !config.NotifyOnRemediation {
		t.Error("Default should notify on remediation")
	}

	if !config.CollectDiagnostics {
		t.Error("Default should collect diagnostics")
	}
}
