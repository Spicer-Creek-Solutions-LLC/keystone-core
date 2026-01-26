package policy

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestBuiltinEvaluator_RequireLabels(t *testing.T) {
	evaluator := NewBuiltinEvaluator()
	ctx := context.Background()

	tests := []struct {
		name     string
		config   RequireLabelsConfig
		resource interface{}
		want     bool
	}{
		{
			name: "all labels present",
			config: RequireLabelsConfig{
				Labels: []string{"env", "team"},
			},
			resource: map[string]interface{}{
				"labels": map[string]interface{}{
					"env":  "production",
					"team": "platform",
				},
			},
			want: true,
		},
		{
			name: "missing label",
			config: RequireLabelsConfig{
				Labels: []string{"env", "team", "owner"},
			},
			resource: map[string]interface{}{
				"labels": map[string]interface{}{
					"env":  "production",
					"team": "platform",
				},
			},
			want: false,
		},
		{
			name: "empty value when require values",
			config: RequireLabelsConfig{
				Labels:        []string{"env"},
				RequireValues: true,
			},
			resource: map[string]interface{}{
				"labels": map[string]interface{}{
					"env": "",
				},
			},
			want: false,
		},
		{
			name: "label pattern match",
			config: RequireLabelsConfig{
				LabelPatterns: []string{"^app\\..*"},
			},
			resource: map[string]interface{}{
				"labels": map[string]interface{}{
					"app.version": "1.0",
				},
			},
			want: true,
		},
		{
			name: "label pattern no match",
			config: RequireLabelsConfig{
				LabelPatterns: []string{"^app\\..*"},
			},
			resource: map[string]interface{}{
				"labels": map[string]interface{}{
					"env": "production",
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configBytes, _ := json.Marshal(tt.config)
			policyConfig := BuiltinPolicyConfig{
				Name:   BuiltinRequireLabels,
				Config: configBytes,
			}
			policyBytes, _ := json.Marshal(policyConfig)

			policy := &Policy{
				ID:       "test-require-labels",
				Name:     "Test Require Labels",
				Type:     PolicyTypeBuiltin,
				Policy:   string(policyBytes),
				Severity: SeverityMedium,
				Enabled:  true,
			}

			input := &EvaluationInput{
				Resource: tt.resource,
				Action:   "create",
			}

			result, err := evaluator.Evaluate(ctx, policy, input)
			if err != nil {
				t.Fatalf("Evaluate error: %v", err)
			}

			if result.Allowed != tt.want {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.want)
			}
		})
	}
}

func TestBuiltinEvaluator_RequireOwner(t *testing.T) {
	evaluator := NewBuiltinEvaluator()
	ctx := context.Background()

	tests := []struct {
		name     string
		config   RequireOwnerConfig
		resource interface{}
		want     bool
	}{
		{
			name: "owner present",
			config: RequireOwnerConfig{
				OwnerLabel: "owner",
			},
			resource: map[string]interface{}{
				"labels": map[string]interface{}{
					"owner": "john@example.com",
				},
			},
			want: true,
		},
		{
			name: "owner missing",
			config: RequireOwnerConfig{
				OwnerLabel: "owner",
			},
			resource: map[string]interface{}{
				"labels": map[string]interface{}{
					"env": "production",
				},
			},
			want: false,
		},
		{
			name: "invalid owner",
			config: RequireOwnerConfig{
				OwnerLabel:  "owner",
				ValidOwners: []string{"alice@example.com", "bob@example.com"},
			},
			resource: map[string]interface{}{
				"labels": map[string]interface{}{
					"owner": "charlie@example.com",
				},
			},
			want: false,
		},
		{
			name: "valid owner from list",
			config: RequireOwnerConfig{
				OwnerLabel:  "owner",
				ValidOwners: []string{"alice@example.com", "bob@example.com"},
			},
			resource: map[string]interface{}{
				"labels": map[string]interface{}{
					"owner": "alice@example.com",
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configBytes, _ := json.Marshal(tt.config)
			policyConfig := BuiltinPolicyConfig{
				Name:   BuiltinRequireOwner,
				Config: configBytes,
			}
			policyBytes, _ := json.Marshal(policyConfig)

			policy := &Policy{
				ID:       "test-require-owner",
				Name:     "Test Require Owner",
				Type:     PolicyTypeBuiltin,
				Policy:   string(policyBytes),
				Severity: SeverityMedium,
				Enabled:  true,
			}

			input := &EvaluationInput{
				Resource: tt.resource,
				Action:   "create",
			}

			result, err := evaluator.Evaluate(ctx, policy, input)
			if err != nil {
				t.Fatalf("Evaluate error: %v", err)
			}

			if result.Allowed != tt.want {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.want)
			}
		})
	}
}

func TestBuiltinEvaluator_AllowedEnvironments(t *testing.T) {
	evaluator := NewBuiltinEvaluator()
	ctx := context.Background()

	tests := []struct {
		name    string
		config  AllowedEnvironmentsConfig
		context map[string]interface{}
		want    bool
	}{
		{
			name: "allowed environment",
			config: AllowedEnvironmentsConfig{
				Environments: []string{"dev", "staging", "production"},
			},
			context: map[string]interface{}{
				"environment": "production",
			},
			want: true,
		},
		{
			name: "disallowed environment",
			config: AllowedEnvironmentsConfig{
				Environments: []string{"dev", "staging"},
			},
			context: map[string]interface{}{
				"environment": "production",
			},
			want: false,
		},
		{
			name: "custom environment key",
			config: AllowedEnvironmentsConfig{
				Environments:   []string{"prod"},
				EnvironmentKey: "env",
			},
			context: map[string]interface{}{
				"env": "prod",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configBytes, _ := json.Marshal(tt.config)
			policyConfig := BuiltinPolicyConfig{
				Name:   BuiltinAllowedEnvironments,
				Config: configBytes,
			}
			policyBytes, _ := json.Marshal(policyConfig)

			policy := &Policy{
				ID:       "test-allowed-environments",
				Name:     "Test Allowed Environments",
				Type:     PolicyTypeBuiltin,
				Policy:   string(policyBytes),
				Severity: SeverityMedium,
				Enabled:  true,
			}

			input := &EvaluationInput{
				Action:  "deploy",
				Context: tt.context,
			}

			result, err := evaluator.Evaluate(ctx, policy, input)
			if err != nil {
				t.Fatalf("Evaluate error: %v", err)
			}

			if result.Allowed != tt.want {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.want)
			}
		})
	}
}

func TestBuiltinEvaluator_AllowedActions(t *testing.T) {
	evaluator := NewBuiltinEvaluator()
	ctx := context.Background()

	tests := []struct {
		name   string
		config AllowedActionsConfig
		action string
		want   bool
	}{
		{
			name: "allowed action",
			config: AllowedActionsConfig{
				Actions: []string{"read", "list", "get"},
			},
			action: "read",
			want:   true,
		},
		{
			name: "disallowed action",
			config: AllowedActionsConfig{
				Actions: []string{"read", "list"},
			},
			action: "delete",
			want:   false,
		},
		{
			name: "action matches pattern",
			config: AllowedActionsConfig{
				ActionPatterns: []string{"^get.*", "^list.*"},
			},
			action: "getUser",
			want:   true,
		},
		{
			name: "action no pattern match",
			config: AllowedActionsConfig{
				ActionPatterns: []string{"^get.*", "^list.*"},
			},
			action: "deleteUser",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configBytes, _ := json.Marshal(tt.config)
			policyConfig := BuiltinPolicyConfig{
				Name:   BuiltinAllowedActions,
				Config: configBytes,
			}
			policyBytes, _ := json.Marshal(policyConfig)

			policy := &Policy{
				ID:       "test-allowed-actions",
				Name:     "Test Allowed Actions",
				Type:     PolicyTypeBuiltin,
				Policy:   string(policyBytes),
				Severity: SeverityMedium,
				Enabled:  true,
			}

			input := &EvaluationInput{
				Action: tt.action,
			}

			result, err := evaluator.Evaluate(ctx, policy, input)
			if err != nil {
				t.Fatalf("Evaluate error: %v", err)
			}

			if result.Allowed != tt.want {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.want)
			}
		})
	}
}

func TestBuiltinEvaluator_DenyPrivileged(t *testing.T) {
	evaluator := NewBuiltinEvaluator()
	ctx := context.Background()

	tests := []struct {
		name    string
		context map[string]interface{}
		want    bool
	}{
		{
			name:    "no privileged flag",
			context: map[string]interface{}{},
			want:    true,
		},
		{
			name: "privileged true",
			context: map[string]interface{}{
				"privileged": true,
			},
			want: false,
		},
		{
			name: "privileged false",
			context: map[string]interface{}{
				"privileged": false,
			},
			want: true,
		},
		{
			name: "run as root",
			context: map[string]interface{}{
				"run_as_user": "root",
			},
			want: false,
		},
		{
			name: "run as uid 0",
			context: map[string]interface{}{
				"run_as_user": "0",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policyConfig := BuiltinPolicyConfig{
				Name:   BuiltinDenyPrivileged,
				Config: json.RawMessage("{}"),
			}
			policyBytes, _ := json.Marshal(policyConfig)

			policy := &Policy{
				ID:       "test-deny-privileged",
				Name:     "Test Deny Privileged",
				Type:     PolicyTypeBuiltin,
				Policy:   string(policyBytes),
				Severity: SeverityCritical,
				Enabled:  true,
			}

			input := &EvaluationInput{
				Action:  "execute",
				Context: tt.context,
			}

			result, err := evaluator.Evaluate(ctx, policy, input)
			if err != nil {
				t.Fatalf("Evaluate error: %v", err)
			}

			if result.Allowed != tt.want {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.want)
			}
		})
	}
}

func TestBuiltinEvaluator_AllowedUsers(t *testing.T) {
	evaluator := NewBuiltinEvaluator()
	ctx := context.Background()

	tests := []struct {
		name    string
		config  AllowedUsersConfig
		user    string
		context map[string]interface{}
		want    bool
	}{
		{
			name: "allowed user",
			config: AllowedUsersConfig{
				Users: []string{"alice", "bob"},
			},
			user: "alice",
			want: true,
		},
		{
			name: "disallowed user",
			config: AllowedUsersConfig{
				Users: []string{"alice", "bob"},
			},
			user: "charlie",
			want: false,
		},
		{
			name: "user matches pattern",
			config: AllowedUsersConfig{
				UserPatterns: []string{"^admin-.*"},
			},
			user: "admin-john",
			want: true,
		},
		{
			name: "user in allowed group",
			config: AllowedUsersConfig{
				Groups: []string{"admins", "operators"},
			},
			user: "charlie",
			context: map[string]interface{}{
				"user_groups": []string{"operators"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configBytes, _ := json.Marshal(tt.config)
			policyConfig := BuiltinPolicyConfig{
				Name:   BuiltinAllowedUsers,
				Config: configBytes,
			}
			policyBytes, _ := json.Marshal(policyConfig)

			policy := &Policy{
				ID:       "test-allowed-users",
				Name:     "Test Allowed Users",
				Type:     PolicyTypeBuiltin,
				Policy:   string(policyBytes),
				Severity: SeverityMedium,
				Enabled:  true,
			}

			input := &EvaluationInput{
				Action:  "access",
				User:    tt.user,
				Context: tt.context,
			}

			result, err := evaluator.Evaluate(ctx, policy, input)
			if err != nil {
				t.Fatalf("Evaluate error: %v", err)
			}

			if result.Allowed != tt.want {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.want)
			}
		})
	}
}

func TestBuiltinEvaluator_DeniedUsers(t *testing.T) {
	evaluator := NewBuiltinEvaluator()
	ctx := context.Background()

	tests := []struct {
		name   string
		config AllowedUsersConfig
		user   string
		want   bool
	}{
		{
			name: "denied user",
			config: AllowedUsersConfig{
				Users: []string{"malicious", "banned"},
			},
			user: "malicious",
			want: false,
		},
		{
			name: "not denied user",
			config: AllowedUsersConfig{
				Users: []string{"malicious", "banned"},
			},
			user: "alice",
			want: true,
		},
		{
			name: "denied pattern match",
			config: AllowedUsersConfig{
				UserPatterns: []string{"^bot-.*"},
			},
			user: "bot-spam",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configBytes, _ := json.Marshal(tt.config)
			policyConfig := BuiltinPolicyConfig{
				Name:   BuiltinDeniedUsers,
				Config: configBytes,
			}
			policyBytes, _ := json.Marshal(policyConfig)

			policy := &Policy{
				ID:       "test-denied-users",
				Name:     "Test Denied Users",
				Type:     PolicyTypeBuiltin,
				Policy:   string(policyBytes),
				Severity: SeverityHigh,
				Enabled:  true,
			}

			input := &EvaluationInput{
				Action: "access",
				User:   tt.user,
			}

			result, err := evaluator.Evaluate(ctx, policy, input)
			if err != nil {
				t.Fatalf("Evaluate error: %v", err)
			}

			if result.Allowed != tt.want {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.want)
			}
		})
	}
}

func TestBuiltinEvaluator_TimeWindow(t *testing.T) {
	evaluator := NewBuiltinEvaluator()
	ctx := context.Background()

	// Use UTC since the policy defaults to UTC when no timezone is specified
	now := time.Now().UTC()
	currentDay := int(now.Weekday())
	currentHour := now.Hour()

	tests := []struct {
		name   string
		config TimeWindowConfig
		want   bool
	}{
		{
			name: "all day allowed",
			config: TimeWindowConfig{
				AllowedHoursStart: 0,
				AllowedHoursEnd:   24,
			},
			want: true,
		},
		{
			name: "within allowed day",
			config: TimeWindowConfig{
				AllowedDays: []int{currentDay},
			},
			want: true,
		},
		{
			name: "outside allowed day",
			config: TimeWindowConfig{
				AllowedDays: []int{(currentDay + 1) % 7},
			},
			want: false,
		},
		{
			name: "blocked date",
			config: TimeWindowConfig{
				BlockedDates: []string{now.Format("2006-01-02")},
			},
			want: false,
		},
		{
			name: "current hour in range",
			config: TimeWindowConfig{
				AllowedHoursStart: currentHour,
				AllowedHoursEnd:   currentHour + 2,
			},
			want: true,
		},
		{
			name: "current hour outside range",
			config: TimeWindowConfig{
				// Pick a 2-hour window that doesn't include current hour
				// If it's before noon, block afternoon; if afternoon, block morning
				AllowedHoursStart: func() int {
					if currentHour < 12 {
						return 14
					}
					return 2
				}(),
				AllowedHoursEnd: func() int {
					if currentHour < 12 {
						return 16
					}
					return 4
				}(),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configBytes, _ := json.Marshal(tt.config)
			policyConfig := BuiltinPolicyConfig{
				Name:   BuiltinTimeWindow,
				Config: configBytes,
			}
			policyBytes, _ := json.Marshal(policyConfig)

			policy := &Policy{
				ID:       "test-time-window",
				Name:     "Test Time Window",
				Type:     PolicyTypeBuiltin,
				Policy:   string(policyBytes),
				Severity: SeverityMedium,
				Enabled:  true,
			}

			input := &EvaluationInput{
				Action: "deploy",
			}

			result, err := evaluator.Evaluate(ctx, policy, input)
			if err != nil {
				t.Fatalf("Evaluate error: %v", err)
			}

			if result.Allowed != tt.want {
				t.Errorf("Allowed = %v, want %v, violations: %v", result.Allowed, tt.want, result.Violations)
			}
		})
	}
}

func TestBuiltinEvaluator_NoRootExecution(t *testing.T) {
	evaluator := NewBuiltinEvaluator()
	ctx := context.Background()

	tests := []struct {
		name    string
		config  NoRootExecutionConfig
		user    string
		action  string
		context map[string]interface{}
		want    bool
	}{
		{
			name:   "not running as root",
			config: NoRootExecutionConfig{},
			user:   "alice",
			action: "execute",
			want:   true,
		},
		{
			name:   "running as root blocked",
			config: NoRootExecutionConfig{},
			user:   "alice",
			action: "execute",
			context: map[string]interface{}{
				"run_as_user": "root",
			},
			want: false,
		},
		{
			name: "allowed user can run as root",
			config: NoRootExecutionConfig{
				AllowedUsers: []string{"admin"},
			},
			user:   "admin",
			action: "execute",
			context: map[string]interface{}{
				"run_as_user": "root",
			},
			want: true,
		},
		{
			name: "allowed action can run as root",
			config: NoRootExecutionConfig{
				AllowedActions: []string{"system-maintenance"},
			},
			user:   "alice",
			action: "system-maintenance",
			context: map[string]interface{}{
				"run_as_user": "root",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configBytes, _ := json.Marshal(tt.config)
			policyConfig := BuiltinPolicyConfig{
				Name:   BuiltinNoRootExecution,
				Config: configBytes,
			}
			policyBytes, _ := json.Marshal(policyConfig)

			policy := &Policy{
				ID:       "test-no-root",
				Name:     "Test No Root Execution",
				Type:     PolicyTypeBuiltin,
				Policy:   string(policyBytes),
				Severity: SeverityCritical,
				Enabled:  true,
			}

			input := &EvaluationInput{
				Action:  tt.action,
				User:    tt.user,
				Context: tt.context,
			}

			result, err := evaluator.Evaluate(ctx, policy, input)
			if err != nil {
				t.Fatalf("Evaluate error: %v", err)
			}

			if result.Allowed != tt.want {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.want)
			}
		})
	}
}

func TestBuiltinEvaluator_RequireApproval(t *testing.T) {
	evaluator := NewBuiltinEvaluator()
	ctx := context.Background()

	tests := []struct {
		name    string
		config  RequireApprovalConfig
		action  string
		context map[string]interface{}
		want    bool
	}{
		{
			name: "action does not require approval",
			config: RequireApprovalConfig{
				Actions: []string{"delete", "terminate"},
			},
			action: "list",
			want:   true,
		},
		{
			name: "action requires approval - not approved",
			config: RequireApprovalConfig{
				Actions: []string{"delete", "terminate"},
			},
			action: "delete",
			want:   false,
		},
		{
			name: "action requires approval - approved",
			config: RequireApprovalConfig{
				Actions: []string{"delete", "terminate"},
			},
			action: "delete",
			context: map[string]interface{}{
				"approved": true,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configBytes, _ := json.Marshal(tt.config)
			policyConfig := BuiltinPolicyConfig{
				Name:   BuiltinRequireApproval,
				Config: configBytes,
			}
			policyBytes, _ := json.Marshal(policyConfig)

			policy := &Policy{
				ID:       "test-require-approval",
				Name:     "Test Require Approval",
				Type:     PolicyTypeBuiltin,
				Policy:   string(policyBytes),
				Severity: SeverityHigh,
				Enabled:  true,
			}

			input := &EvaluationInput{
				Action:  tt.action,
				Context: tt.context,
			}

			result, err := evaluator.Evaluate(ctx, policy, input)
			if err != nil {
				t.Fatalf("Evaluate error: %v", err)
			}

			if result.Allowed != tt.want {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.want)
			}
		})
	}
}

func TestBuiltinEvaluator_MaxConcurrent(t *testing.T) {
	evaluator := NewBuiltinEvaluator()
	ctx := context.Background()

	tests := []struct {
		name    string
		config  MaxConcurrentConfig
		context map[string]interface{}
		want    bool
	}{
		{
			name: "under limit",
			config: MaxConcurrentConfig{
				MaxConcurrent: 10,
			},
			context: map[string]interface{}{
				"concurrent_count": 5,
			},
			want: true,
		},
		{
			name: "at limit",
			config: MaxConcurrentConfig{
				MaxConcurrent: 10,
			},
			context: map[string]interface{}{
				"concurrent_count": 10,
			},
			want: false,
		},
		{
			name: "over limit",
			config: MaxConcurrentConfig{
				MaxConcurrent: 10,
			},
			context: map[string]interface{}{
				"concurrent_count": 15,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configBytes, _ := json.Marshal(tt.config)
			policyConfig := BuiltinPolicyConfig{
				Name:   BuiltinMaxConcurrent,
				Config: configBytes,
			}
			policyBytes, _ := json.Marshal(policyConfig)

			policy := &Policy{
				ID:       "test-max-concurrent",
				Name:     "Test Max Concurrent",
				Type:     PolicyTypeBuiltin,
				Policy:   string(policyBytes),
				Severity: SeverityMedium,
				Enabled:  true,
			}

			input := &EvaluationInput{
				Action:  "execute",
				Context: tt.context,
			}

			result, err := evaluator.Evaluate(ctx, policy, input)
			if err != nil {
				t.Fatalf("Evaluate error: %v", err)
			}

			if result.Allowed != tt.want {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.want)
			}
		})
	}
}

func TestBuiltinEvaluator_ResourceQuota(t *testing.T) {
	evaluator := NewBuiltinEvaluator()
	ctx := context.Background()

	tests := []struct {
		name    string
		config  ResourceQuotaConfig
		context map[string]interface{}
		want    bool
	}{
		{
			name: "under total quota",
			config: ResourceQuotaConfig{
				MaxResources: 100,
			},
			context: map[string]interface{}{
				"total_resources": 50,
			},
			want: true,
		},
		{
			name: "at total quota",
			config: ResourceQuotaConfig{
				MaxResources: 100,
			},
			context: map[string]interface{}{
				"total_resources": 100,
			},
			want: false,
		},
		{
			name: "under user quota",
			config: ResourceQuotaConfig{
				MaxPerUser: 10,
			},
			context: map[string]interface{}{
				"user_resources": 5,
			},
			want: true,
		},
		{
			name: "over user quota",
			config: ResourceQuotaConfig{
				MaxPerUser: 10,
			},
			context: map[string]interface{}{
				"user_resources": 15,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configBytes, _ := json.Marshal(tt.config)
			policyConfig := BuiltinPolicyConfig{
				Name:   BuiltinResourceQuota,
				Config: configBytes,
			}
			policyBytes, _ := json.Marshal(policyConfig)

			policy := &Policy{
				ID:       "test-resource-quota",
				Name:     "Test Resource Quota",
				Type:     PolicyTypeBuiltin,
				Policy:   string(policyBytes),
				Severity: SeverityMedium,
				Enabled:  true,
			}

			input := &EvaluationInput{
				Action:  "create",
				Context: tt.context,
			}

			result, err := evaluator.Evaluate(ctx, policy, input)
			if err != nil {
				t.Fatalf("Evaluate error: %v", err)
			}

			if result.Allowed != tt.want {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.want)
			}
		})
	}
}

func TestBuiltinEvaluator_PatternDeny(t *testing.T) {
	evaluator := NewBuiltinEvaluator()
	ctx := context.Background()

	tests := []struct {
		name     string
		config   PatternConfig
		resource interface{}
		want     bool
	}{
		{
			name: "name does not match denied pattern",
			config: PatternConfig{
				Patterns: []string{"^test-.*", "^dev-.*"},
			},
			resource: map[string]interface{}{
				"name": "prod-app",
			},
			want: true,
		},
		{
			name: "name matches denied pattern",
			config: PatternConfig{
				Patterns: []string{"^test-.*", "^dev-.*"},
			},
			resource: map[string]interface{}{
				"name": "test-app",
			},
			want: false,
		},
		{
			name: "custom field match",
			config: PatternConfig{
				Patterns: []string{".*-deprecated$"},
				Field:    "metadata.tag",
			},
			resource: map[string]interface{}{
				"metadata": map[string]interface{}{
					"tag": "v1-deprecated",
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configBytes, _ := json.Marshal(tt.config)
			policyConfig := BuiltinPolicyConfig{
				Name:   BuiltinPatternDeny,
				Config: configBytes,
			}
			policyBytes, _ := json.Marshal(policyConfig)

			policy := &Policy{
				ID:       "test-pattern-deny",
				Name:     "Test Pattern Deny",
				Type:     PolicyTypeBuiltin,
				Policy:   string(policyBytes),
				Severity: SeverityMedium,
				Enabled:  true,
			}

			input := &EvaluationInput{
				Resource: tt.resource,
				Action:   "create",
			}

			result, err := evaluator.Evaluate(ctx, policy, input)
			if err != nil {
				t.Fatalf("Evaluate error: %v", err)
			}

			if result.Allowed != tt.want {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.want)
			}
		})
	}
}

func TestBuiltinEvaluator_PatternAllow(t *testing.T) {
	evaluator := NewBuiltinEvaluator()
	ctx := context.Background()

	tests := []struct {
		name     string
		config   PatternConfig
		resource interface{}
		want     bool
	}{
		{
			name: "name matches allowed pattern",
			config: PatternConfig{
				Patterns: []string{"^prod-.*", "^staging-.*"},
			},
			resource: map[string]interface{}{
				"name": "prod-app",
			},
			want: true,
		},
		{
			name: "name does not match allowed pattern",
			config: PatternConfig{
				Patterns: []string{"^prod-.*", "^staging-.*"},
			},
			resource: map[string]interface{}{
				"name": "test-app",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configBytes, _ := json.Marshal(tt.config)
			policyConfig := BuiltinPolicyConfig{
				Name:   BuiltinPatternAllow,
				Config: configBytes,
			}
			policyBytes, _ := json.Marshal(policyConfig)

			policy := &Policy{
				ID:       "test-pattern-allow",
				Name:     "Test Pattern Allow",
				Type:     PolicyTypeBuiltin,
				Policy:   string(policyBytes),
				Severity: SeverityMedium,
				Enabled:  true,
			}

			input := &EvaluationInput{
				Resource: tt.resource,
				Action:   "create",
			}

			result, err := evaluator.Evaluate(ctx, policy, input)
			if err != nil {
				t.Fatalf("Evaluate error: %v", err)
			}

			if result.Allowed != tt.want {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.want)
			}
		})
	}
}

func TestBuiltinEvaluator_ValidatePolicy(t *testing.T) {
	evaluator := NewBuiltinEvaluator()
	ctx := context.Background()

	tests := []struct {
		name       string
		policyCode string
		wantErr    bool
	}{
		{
			name:       "valid policy",
			policyCode: `{"name": "require-labels", "config": {"labels": ["env"]}}`,
			wantErr:    false,
		},
		{
			name:       "invalid json",
			policyCode: `{"name": "require-labels"`,
			wantErr:    true,
		},
		{
			name:       "missing name",
			policyCode: `{"config": {"labels": ["env"]}}`,
			wantErr:    true,
		},
		{
			name:       "unknown policy name",
			policyCode: `{"name": "unknown-policy", "config": {}}`,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := evaluator.ValidatePolicy(ctx, tt.policyCode)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePolicy() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBuiltinEvaluator_ListBuiltinPolicies(t *testing.T) {
	evaluator := NewBuiltinEvaluator()

	policies := evaluator.ListBuiltinPolicies()
	if len(policies) == 0 {
		t.Error("Expected at least one built-in policy")
	}

	// Check that all expected policies are present
	expected := []BuiltinPolicyName{
		BuiltinRequireLabels,
		BuiltinRequireOwner,
		BuiltinAllowedEnvironments,
		BuiltinAllowedActions,
		BuiltinDenyPrivileged,
		BuiltinAllowedUsers,
		BuiltinDeniedUsers,
		BuiltinTimeWindow,
		BuiltinNoRootExecution,
		BuiltinRequireApproval,
		BuiltinMaxConcurrent,
		BuiltinResourceQuota,
		BuiltinPatternDeny,
		BuiltinPatternAllow,
	}

	policySet := make(map[BuiltinPolicyName]bool)
	for _, p := range policies {
		policySet[p] = true
	}

	for _, e := range expected {
		if !policySet[e] {
			t.Errorf("Expected policy %s not found", e)
		}
	}
}

func TestPolicyEngine_BuiltinIntegration(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)
	ctx := context.Background()

	// Register a built-in policy
	config := RequireLabelsConfig{
		Labels: []string{"env", "team"},
	}
	configBytes, _ := json.Marshal(config)
	policyConfig := BuiltinPolicyConfig{
		Name:   BuiltinRequireLabels,
		Config: configBytes,
	}
	policyBytes, _ := json.Marshal(policyConfig)

	policy := &Policy{
		ID:              "builtin-require-labels",
		Name:            "Require Labels",
		Type:            PolicyTypeBuiltin,
		Category:        CategoryCompliance,
		Severity:        SeverityMedium,
		EnforcementMode: ModeEnforce,
		Policy:          string(policyBytes),
		Enabled:         true,
	}

	if err := registry.RegisterPolicy(policy); err != nil {
		t.Fatalf("RegisterPolicy error: %v", err)
	}

	// Test evaluation
	input := &EvaluationInput{
		Resource: map[string]interface{}{
			"labels": map[string]interface{}{
				"env":  "production",
				"team": "platform",
			},
		},
		Action: "create",
	}

	result, err := engine.Evaluate(ctx, "builtin-require-labels", input)
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}

	if !result.Allowed {
		t.Errorf("Expected allowed, got denied: %v", result.Violations)
	}

	// Test with missing label
	input.Resource = map[string]interface{}{
		"labels": map[string]interface{}{
			"env": "production",
		},
	}

	result, err = engine.Evaluate(ctx, "builtin-require-labels", input)
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}

	if result.Allowed {
		t.Error("Expected denied for missing label")
	}

	if len(result.Violations) != 1 {
		t.Errorf("Expected 1 violation, got %d", len(result.Violations))
	}
}
