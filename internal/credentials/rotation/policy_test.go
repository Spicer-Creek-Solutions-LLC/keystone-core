package rotation

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
)

func TestNewPolicyEngine(t *testing.T) {
	pe := NewPolicyEngine()
	if pe == nil {
		t.Fatal("expected non-nil policy engine")
	}
	if len(pe.policies) != 0 {
		t.Errorf("expected 0 policies, got %d", len(pe.policies))
	}
}

func TestPolicyEngine_AddPolicy(t *testing.T) {
	pe := NewPolicyEngine()

	policy := &RotationPolicy{
		ID:              "p1",
		Name:            "SSH 90-day",
		CredentialTypes: []credentials.CredentialType{credentials.CredentialTypeSSHPassword},
		MaxAge:          90 * 24 * time.Hour,
		WarningAge:      75 * 24 * time.Hour,
		Enabled:         true,
	}

	err := pe.AddPolicy(policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, ok := pe.GetPolicy("p1")
	if !ok {
		t.Fatal("policy not found")
	}
	if got.Name != "SSH 90-day" {
		t.Errorf("expected name 'SSH 90-day', got %s", got.Name)
	}
}

func TestPolicyEngine_AddPolicyValidation(t *testing.T) {
	pe := NewPolicyEngine()

	tests := []struct {
		name   string
		policy *RotationPolicy
	}{
		{"empty ID", &RotationPolicy{MaxAge: time.Hour, CredentialTypes: []credentials.CredentialType{"x"}}},
		{"zero max age", &RotationPolicy{ID: "p1", CredentialTypes: []credentials.CredentialType{"x"}}},
		{"no credential types", &RotationPolicy{ID: "p1", MaxAge: time.Hour}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pe.AddPolicy(tt.policy)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestPolicyEngine_AddPolicyDuplicate(t *testing.T) {
	pe := NewPolicyEngine()

	policy := &RotationPolicy{
		ID:              "p1",
		CredentialTypes: []credentials.CredentialType{credentials.CredentialTypeSSHPassword},
		MaxAge:          time.Hour,
	}

	_ = pe.AddPolicy(policy)
	err := pe.AddPolicy(policy)
	if err == nil {
		t.Error("expected duplicate error")
	}
}

func TestPolicyEngine_UpdatePolicy(t *testing.T) {
	pe := NewPolicyEngine()

	_ = pe.AddPolicy(&RotationPolicy{
		ID:              "p1",
		Name:            "old name",
		CredentialTypes: []credentials.CredentialType{credentials.CredentialTypeSSHPassword},
		MaxAge:          time.Hour,
	})

	err := pe.UpdatePolicy(&RotationPolicy{
		ID:              "p1",
		Name:            "new name",
		CredentialTypes: []credentials.CredentialType{credentials.CredentialTypeSSHPassword},
		MaxAge:          2 * time.Hour,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := pe.GetPolicy("p1")
	if got.Name != "new name" {
		t.Errorf("expected 'new name', got %s", got.Name)
	}
}

func TestPolicyEngine_UpdatePolicyNotFound(t *testing.T) {
	pe := NewPolicyEngine()
	err := pe.UpdatePolicy(&RotationPolicy{ID: "nonexistent"})
	if err == nil {
		t.Error("expected error")
	}
}

func TestPolicyEngine_RemovePolicy(t *testing.T) {
	pe := NewPolicyEngine()

	_ = pe.AddPolicy(&RotationPolicy{
		ID:              "p1",
		CredentialTypes: []credentials.CredentialType{credentials.CredentialTypeSSHPassword},
		MaxAge:          time.Hour,
	})

	err := pe.RemovePolicy("p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, ok := pe.GetPolicy("p1")
	if ok {
		t.Error("policy should be removed")
	}
}

func TestPolicyEngine_RemovePolicyNotFound(t *testing.T) {
	pe := NewPolicyEngine()
	err := pe.RemovePolicy("nonexistent")
	if err == nil {
		t.Error("expected error")
	}
}

func TestPolicyEngine_ListPolicies(t *testing.T) {
	pe := NewPolicyEngine()

	_ = pe.AddPolicy(&RotationPolicy{
		ID: "p1", CredentialTypes: []credentials.CredentialType{credentials.CredentialTypeSSHPassword}, MaxAge: time.Hour,
	})
	_ = pe.AddPolicy(&RotationPolicy{
		ID: "p2", CredentialTypes: []credentials.CredentialType{credentials.CredentialTypeSNMPv2c}, MaxAge: time.Hour,
	})

	policies := pe.ListPolicies()
	if len(policies) != 2 {
		t.Errorf("expected 2 policies, got %d", len(policies))
	}
}

func TestPolicyEngine_EvaluateOK(t *testing.T) {
	pe := NewPolicyEngine()

	_ = pe.AddPolicy(&RotationPolicy{
		ID:              "p1",
		CredentialTypes: []credentials.CredentialType{credentials.CredentialTypeSSHPassword},
		MaxAge:          90 * 24 * time.Hour,
		WarningAge:      75 * 24 * time.Hour,
		Enabled:         true,
	})

	now := time.Now()
	cred := &credentials.SSHPasswordCredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "cred-1",
			CredentialType: credentials.CredentialTypeSSHPassword,
			CreatedAt:      now.Add(-10 * 24 * time.Hour), // 10 days old
		},
		Username: "admin",
		Password: "pass",
	}

	result := pe.Evaluate(cred, now)
	if result.Action != PolicyActionOK {
		t.Errorf("expected OK, got %s: %s", result.Action, result.Message)
	}
}

func TestPolicyEngine_EvaluateWarning(t *testing.T) {
	pe := NewPolicyEngine()

	_ = pe.AddPolicy(&RotationPolicy{
		ID:              "p1",
		CredentialTypes: []credentials.CredentialType{credentials.CredentialTypeSSHPassword},
		MaxAge:          90 * 24 * time.Hour,
		WarningAge:      75 * 24 * time.Hour,
		Enabled:         true,
	})

	now := time.Now()
	cred := &credentials.SSHPasswordCredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "cred-1",
			CredentialType: credentials.CredentialTypeSSHPassword,
			CreatedAt:      now.Add(-80 * 24 * time.Hour), // 80 days old
		},
		Username: "admin",
		Password: "pass",
	}

	result := pe.Evaluate(cred, now)
	if result.Action != PolicyActionWarning {
		t.Errorf("expected Warning, got %s: %s", result.Action, result.Message)
	}
}

func TestPolicyEngine_EvaluateRotate(t *testing.T) {
	pe := NewPolicyEngine()

	_ = pe.AddPolicy(&RotationPolicy{
		ID:              "p1",
		CredentialTypes: []credentials.CredentialType{credentials.CredentialTypeSSHPassword},
		MaxAge:          90 * 24 * time.Hour,
		Enabled:         true,
	})

	now := time.Now()
	cred := &credentials.SSHPasswordCredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "cred-1",
			CredentialType: credentials.CredentialTypeSSHPassword,
			CreatedAt:      now.Add(-100 * 24 * time.Hour), // 100 days old
		},
		Username: "admin",
		Password: "pass",
	}

	result := pe.Evaluate(cred, now)
	if result.Action != PolicyActionRotate {
		t.Errorf("expected Rotate, got %s: %s", result.Action, result.Message)
	}
}

func TestPolicyEngine_EvaluateExpired(t *testing.T) {
	pe := NewPolicyEngine()

	_ = pe.AddPolicy(&RotationPolicy{
		ID:              "p1",
		CredentialTypes: []credentials.CredentialType{credentials.CredentialTypeSSHPassword},
		MaxAge:          90 * 24 * time.Hour,
		Enabled:         true,
	})

	now := time.Now()
	cred := &credentials.SSHPasswordCredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "cred-1",
			CredentialType: credentials.CredentialTypeSSHPassword,
			CreatedAt:      now.Add(-100 * 24 * time.Hour),
			Expires:        now.Add(-1 * time.Hour), // Already expired
		},
		Username: "admin",
		Password: "pass",
	}

	result := pe.Evaluate(cred, now)
	if result.Action != PolicyActionExpired {
		t.Errorf("expected Expired, got %s: %s", result.Action, result.Message)
	}
}

func TestPolicyEngine_EvaluateNoMatchingPolicy(t *testing.T) {
	pe := NewPolicyEngine()

	_ = pe.AddPolicy(&RotationPolicy{
		ID:              "p1",
		CredentialTypes: []credentials.CredentialType{credentials.CredentialTypeSNMPv2c},
		MaxAge:          time.Hour,
		Enabled:         true,
	})

	cred := &credentials.SSHPasswordCredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "cred-1",
			CredentialType: credentials.CredentialTypeSSHPassword,
		},
		Username: "admin",
		Password: "pass",
	}

	result := pe.Evaluate(cred, time.Now())
	if result.Action != PolicyActionOK {
		t.Errorf("expected OK (no matching policy), got %s", result.Action)
	}
}

func TestPolicyEngine_EvaluateDisabledPolicy(t *testing.T) {
	pe := NewPolicyEngine()

	_ = pe.AddPolicy(&RotationPolicy{
		ID:              "p1",
		CredentialTypes: []credentials.CredentialType{credentials.CredentialTypeSSHPassword},
		MaxAge:          1 * time.Hour,
		Enabled:         false, // Disabled
	})

	now := time.Now()
	cred := &credentials.SSHPasswordCredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "cred-1",
			CredentialType: credentials.CredentialTypeSSHPassword,
			CreatedAt:      now.Add(-100 * 24 * time.Hour),
		},
		Username: "admin",
		Password: "pass",
	}

	result := pe.Evaluate(cred, now)
	if result.Action != PolicyActionOK {
		t.Errorf("expected OK (disabled policy), got %s", result.Action)
	}
}

func TestPolicyEngine_EvaluateMostUrgentWins(t *testing.T) {
	pe := NewPolicyEngine()

	// Lenient policy: 365-day max age (would be OK)
	_ = pe.AddPolicy(&RotationPolicy{
		ID:              "lenient",
		CredentialTypes: []credentials.CredentialType{credentials.CredentialTypeSSHPassword},
		MaxAge:          365 * 24 * time.Hour,
		WarningAge:      300 * 24 * time.Hour,
		Enabled:         true,
	})

	// Strict policy: 30-day max age (would trigger rotate)
	_ = pe.AddPolicy(&RotationPolicy{
		ID:              "strict",
		CredentialTypes: []credentials.CredentialType{credentials.CredentialTypeSSHPassword},
		MaxAge:          30 * 24 * time.Hour,
		Enabled:         true,
	})

	now := time.Now()
	cred := &credentials.SSHPasswordCredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "cred-1",
			CredentialType: credentials.CredentialTypeSSHPassword,
			CreatedAt:      now.Add(-60 * 24 * time.Hour), // 60 days old
		},
		Username: "admin",
		Password: "pass",
	}

	result := pe.Evaluate(cred, now)
	if result.Action != PolicyActionRotate {
		t.Errorf("expected Rotate (strict policy), got %s: %s", result.Action, result.Message)
	}
	if result.PolicyID != "strict" {
		t.Errorf("expected strict policy, got %s", result.PolicyID)
	}
}

func TestPolicyEngine_EvaluateNoCreationTime(t *testing.T) {
	pe := NewPolicyEngine()

	_ = pe.AddPolicy(&RotationPolicy{
		ID:              "p1",
		CredentialTypes: []credentials.CredentialType{credentials.CredentialTypeSSHPassword},
		MaxAge:          time.Hour,
		Enabled:         true,
	})

	cred := &credentials.SSHPasswordCredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "cred-1",
			CredentialType: credentials.CredentialTypeSSHPassword,
			// No CreatedAt set
		},
		Username: "admin",
		Password: "pass",
	}

	result := pe.Evaluate(cred, time.Now())
	if result.Action != PolicyActionOK {
		t.Errorf("expected OK (no creation time), got %s", result.Action)
	}
}

func TestPolicyEngine_FindDueCredentials(t *testing.T) {
	pe := NewPolicyEngine()

	_ = pe.AddPolicy(&RotationPolicy{
		ID:              "p1",
		CredentialTypes: []credentials.CredentialType{credentials.CredentialTypeSSHPassword},
		MaxAge:          30 * 24 * time.Hour,
		Enabled:         true,
		RollbackOnFail:  true,
	})

	store := credentials.NewInMemoryCredentialStore()
	ctx := context.Background()
	now := time.Now()

	// Old credential (should be flagged)
	_ = store.Store(ctx, &credentials.SSHPasswordCredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "old-cred",
			CredentialType: credentials.CredentialTypeSSHPassword,
			CreatedAt:      now.Add(-60 * 24 * time.Hour),
		},
		Username: "admin",
		Password: "pass1",
	})

	// Fresh credential (should be OK)
	_ = store.Store(ctx, &credentials.SSHPasswordCredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "fresh-cred",
			CredentialType: credentials.CredentialTypeSSHPassword,
			CreatedAt:      now.Add(-5 * 24 * time.Hour),
		},
		Username: "admin",
		Password: "pass2",
	})

	// SNMP credential (no matching policy)
	_ = store.Store(ctx, &credentials.SNMPv2cCredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "snmp-cred",
			CredentialType: credentials.CredentialTypeSNMPv2c,
			CreatedAt:      now.Add(-60 * 24 * time.Hour),
		},
		Community: "public",
	})

	jobs, err := pe.FindDueCredentials(ctx, store, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(jobs) != 1 {
		t.Fatalf("expected 1 due credential, got %d", len(jobs))
	}
	if jobs[0].CredentialID != "old-cred" {
		t.Errorf("expected old-cred, got %s", jobs[0].CredentialID)
	}
	if !jobs[0].RollbackOnFail {
		t.Error("expected RollbackOnFail from policy")
	}
}

func TestPolicyEngine_FindDueCredentialsEmpty(t *testing.T) {
	pe := NewPolicyEngine()
	store := credentials.NewInMemoryCredentialStore()

	jobs, err := pe.FindDueCredentials(context.Background(), store, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(jobs))
	}
}

func TestPolicyEngine_EvaluateAllCredentialTypes(t *testing.T) {
	pe := NewPolicyEngine()

	allTypes := []credentials.CredentialType{
		credentials.CredentialTypeSSHPassword,
		credentials.CredentialTypeSSHKey,
		credentials.CredentialTypeSNMPv2c,
		credentials.CredentialTypeSNMPv3,
		credentials.CredentialTypeWinRM,
		credentials.CredentialTypeRESTBasic,
		credentials.CredentialTypeRESTBearer,
		credentials.CredentialTypeRESTAPIKey,
		credentials.CredentialTypeRESTOAuth2,
		credentials.CredentialTypeGNMI,
	}

	_ = pe.AddPolicy(&RotationPolicy{
		ID:              "all-types",
		CredentialTypes: allTypes,
		MaxAge:          30 * 24 * time.Hour,
		Enabled:         true,
	})

	now := time.Now()
	oldTime := now.Add(-60 * 24 * time.Hour)

	creds := []credentials.Credential{
		&credentials.SSHPasswordCredential{
			BaseCredential: credentials.BaseCredential{CredentialID: "ssh-pw", CredentialType: credentials.CredentialTypeSSHPassword, CreatedAt: oldTime},
			Username: "a", Password: "b",
		},
		&credentials.SSHKeyCredential{
			BaseCredential: credentials.BaseCredential{CredentialID: "ssh-key", CredentialType: credentials.CredentialTypeSSHKey, CreatedAt: oldTime},
			Username: "a", PrivateKey: []byte("k"),
		},
		&credentials.SNMPv2cCredential{
			BaseCredential: credentials.BaseCredential{CredentialID: "snmpv2c", CredentialType: credentials.CredentialTypeSNMPv2c, CreatedAt: oldTime},
			Community: "c",
		},
		&credentials.SNMPv3Credential{
			BaseCredential:  credentials.BaseCredential{CredentialID: "snmpv3", CredentialType: credentials.CredentialTypeSNMPv3, CreatedAt: oldTime},
			Username: "u", SecurityLevel: credentials.SNMPv3SecurityNoAuthNoPriv,
		},
		&credentials.WinRMCredential{
			BaseCredential: credentials.BaseCredential{CredentialID: "winrm", CredentialType: credentials.CredentialTypeWinRM, CreatedAt: oldTime},
			Username: "a", Password: "b",
		},
		&credentials.RESTBasicCredential{
			BaseCredential: credentials.BaseCredential{CredentialID: "rest-basic", CredentialType: credentials.CredentialTypeRESTBasic, CreatedAt: oldTime},
			Username: "a", Password: "b",
		},
		&credentials.RESTBearerCredential{
			BaseCredential: credentials.BaseCredential{CredentialID: "rest-bearer", CredentialType: credentials.CredentialTypeRESTBearer, CreatedAt: oldTime},
			Token: "t",
		},
		&credentials.RESTAPIKeyCredential{
			BaseCredential: credentials.BaseCredential{CredentialID: "rest-apikey", CredentialType: credentials.CredentialTypeRESTAPIKey, CreatedAt: oldTime},
			APIKey: "k",
		},
		&credentials.RESTOAuth2Credential{
			BaseCredential: credentials.BaseCredential{CredentialID: "rest-oauth2", CredentialType: credentials.CredentialTypeRESTOAuth2, CreatedAt: oldTime},
			ClientID: "c", ClientSecret: "s", TokenURL: "https://example.com/token",
		},
		&credentials.GNMICredential{
			BaseCredential: credentials.BaseCredential{CredentialID: "gnmi", CredentialType: credentials.CredentialTypeGNMI, CreatedAt: oldTime},
		},
	}

	for _, cred := range creds {
		t.Run(string(cred.Type()), func(t *testing.T) {
			result := pe.Evaluate(cred, now)
			if result.Action != PolicyActionRotate {
				t.Errorf("expected Rotate for %s, got %s: %s", cred.Type(), result.Action, result.Message)
			}
		})
	}
}

func TestActionPriority(t *testing.T) {
	if actionPriority(PolicyActionExpired) <= actionPriority(PolicyActionRotate) {
		t.Error("expired should be higher priority than rotate")
	}
	if actionPriority(PolicyActionRotate) <= actionPriority(PolicyActionWarning) {
		t.Error("rotate should be higher priority than warning")
	}
	if actionPriority(PolicyActionWarning) <= actionPriority(PolicyActionOK) {
		t.Error("warning should be higher priority than OK")
	}
}
