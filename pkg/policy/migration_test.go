package policy

import (
	"testing"
	"time"
)

func TestMigrationVersionConstants(t *testing.T) {
	if CurrentVersion != Version3 {
		t.Errorf("Expected CurrentVersion to be V3, got %s", CurrentVersion)
	}
}

func TestNewPolicyMigrator(t *testing.T) {
	registry := NewRegistry()
	migrator := NewPolicyMigrator(registry)

	if migrator == nil {
		t.Fatal("Expected migrator to be created")
	}

	if migrator.registry != registry {
		t.Error("Expected registry to be set")
	}

	if len(migrator.migrations) == 0 {
		t.Error("Expected builtin migrations to be registered")
	}
}

func TestPolicyMigrator_RegisterMigration(t *testing.T) {
	registry := NewRegistry()
	migrator := NewPolicyMigrator(registry)

	migrator.RegisterMigration("custom_migration", func(p *Policy) (*Policy, error) {
		return p, nil
	})

	if _, ok := migrator.migrations["custom_migration"]; !ok {
		t.Error("Expected custom migration to be registered")
	}
}

func TestPolicyMigrator_Plan(t *testing.T) {
	registry := NewRegistry()
	migrator := NewPolicyMigrator(registry)

	// Add some policies
	registry.RegisterPolicy(&Policy{
		ID:     "test-policy-1",
		Name:   "Test Policy 1",
		Type:   PolicyTypeCEL,
		Policy: "action == 'read'",
	})

	plan, err := migrator.Plan(Version1, Version3)
	if err != nil {
		t.Fatalf("Failed to create plan: %v", err)
	}

	if plan.FromVersion != Version1 {
		t.Errorf("Expected from version V1, got %s", plan.FromVersion)
	}
	if plan.ToVersion != Version3 {
		t.Errorf("Expected to version V3, got %s", plan.ToVersion)
	}
	if len(plan.Steps) != 2 {
		t.Errorf("Expected 2 steps for V1->V3, got %d", len(plan.Steps))
	}
	if plan.AffectedPolicies != 1 {
		t.Errorf("Expected 1 affected policy, got %d", plan.AffectedPolicies)
	}
}

func TestPolicyMigrator_Plan_SameVersion(t *testing.T) {
	registry := NewRegistry()
	migrator := NewPolicyMigrator(registry)

	_, err := migrator.Plan(Version2, Version2)
	if err == nil {
		t.Error("Expected error for same version migration")
	}
}

func TestPolicyMigrator_Plan_UnknownVersion(t *testing.T) {
	registry := NewRegistry()
	migrator := NewPolicyMigrator(registry)

	_, err := migrator.Plan("v99", Version2)
	if err == nil {
		t.Error("Expected error for unknown from version")
	}

	_, err = migrator.Plan(Version1, "v99")
	if err == nil {
		t.Error("Expected error for unknown to version")
	}
}

func TestPolicyMigrator_Plan_Downgrade(t *testing.T) {
	registry := NewRegistry()
	migrator := NewPolicyMigrator(registry)

	plan, err := migrator.Plan(Version3, Version1)
	if err != nil {
		t.Fatalf("Failed to create downgrade plan: %v", err)
	}

	if len(plan.Steps) != 2 {
		t.Errorf("Expected 2 steps for V3->V1 downgrade, got %d", len(plan.Steps))
	}
	if len(plan.BreakingChanges) == 0 {
		t.Error("Expected breaking changes warning for downgrade")
	}
}

func TestPolicyMigrator_Migrate_DryRun(t *testing.T) {
	registry := NewRegistry()
	migrator := NewPolicyMigrator(registry)

	// Add a policy
	policy := &Policy{
		ID:       "test-policy",
		Name:     "Test Policy",
		Type:     PolicyTypeCEL,
		Policy:   "action == 'read'",
		Category: CategorySecurity,
	}
	registry.RegisterPolicy(policy)

	ctx := &MigrationContext{
		DryRun:   true,
		Validate: true,
		Logger:   &noopMigrationLogger{},
	}

	record, err := migrator.Migrate(ctx, Version1, Version2)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	if record.DryRun != true {
		t.Error("Expected dry run flag to be set")
	}
	if record.PoliciesMigrated != 1 {
		t.Errorf("Expected 1 policy migrated, got %d", record.PoliciesMigrated)
	}

	// Verify policy wasn't actually modified
	p, _ := registry.GetPolicy("test-policy")
	if p.Metadata != nil && p.Metadata["migrated_from"] != "" {
		t.Error("Policy should not have been modified in dry run")
	}
}

func TestPolicyMigrator_Migrate(t *testing.T) {
	registry := NewRegistry()
	migrator := NewPolicyMigrator(registry)

	// Add a policy
	policy := &Policy{
		ID:       "test-policy",
		Name:     "Test Policy",
		Type:     PolicyTypeCEL,
		Policy:   "action == 'read'",
		Category: CategorySecurity,
	}
	registry.RegisterPolicy(policy)

	ctx := DefaultMigrationContext()
	record, err := migrator.Migrate(ctx, Version1, Version2)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	if record.Status != MigrationCompleted {
		t.Errorf("Expected status completed, got %s", record.Status)
	}
	if record.PoliciesMigrated != 1 {
		t.Errorf("Expected 1 policy migrated, got %d", record.PoliciesMigrated)
	}
	if record.Duration == 0 {
		t.Error("Expected duration to be set")
	}

	// Verify policy was modified
	p, _ := registry.GetPolicy("test-policy")
	if p.Metadata == nil || p.Metadata["migrated_from"] != "v1" {
		t.Error("Policy should have migration metadata")
	}
}

func TestPolicyMigrator_Migrate_MultiStep(t *testing.T) {
	registry := NewRegistry()
	migrator := NewPolicyMigrator(registry)

	policy := &Policy{
		ID:       "test-policy",
		Name:     "Test Policy",
		Type:     PolicyTypeCEL,
		Policy:   "action == 'read'",
		Category: CategorySecurity,
	}
	registry.RegisterPolicy(policy)

	ctx := DefaultMigrationContext()
	record, err := migrator.Migrate(ctx, Version1, Version3)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	if record.Status != MigrationCompleted {
		t.Errorf("Expected status completed, got %s", record.Status)
	}

	// Verify policy has V3 enhancements
	p, _ := registry.GetPolicy("test-policy")
	if p.Tags == nil || len(p.Tags) == 0 {
		t.Error("Expected tags to be set for V3")
	}

	// Should have category tag
	hasCategory := false
	for _, tag := range p.Tags {
		if tag == "category:security" {
			hasCategory = true
			break
		}
	}
	if !hasCategory {
		t.Error("Expected category tag to be added")
	}
}

func TestPolicyMigrator_GetHistory(t *testing.T) {
	registry := NewRegistry()
	migrator := NewPolicyMigrator(registry)

	registry.RegisterPolicy(&Policy{
		ID:     "test",
		Name:   "Test",
		Type:   PolicyTypeCEL,
		Policy: "true",
	})

	// Initially empty
	history := migrator.GetHistory()
	if len(history) != 0 {
		t.Error("Expected empty history initially")
	}

	// After migration
	ctx := DefaultMigrationContext()
	migrator.Migrate(ctx, Version1, Version2)

	history = migrator.GetHistory()
	if len(history) != 1 {
		t.Errorf("Expected 1 history record, got %d", len(history))
	}
}

func TestPolicyMigrator_GetLatestMigration(t *testing.T) {
	registry := NewRegistry()
	migrator := NewPolicyMigrator(registry)

	registry.RegisterPolicy(&Policy{
		ID:     "test",
		Name:   "Test",
		Type:   PolicyTypeCEL,
		Policy: "true",
	})

	// Initially nil
	if migrator.GetLatestMigration() != nil {
		t.Error("Expected nil for no migrations")
	}

	ctx := DefaultMigrationContext()
	migrator.Migrate(ctx, Version1, Version2)

	latest := migrator.GetLatestMigration()
	if latest == nil {
		t.Fatal("Expected latest migration")
	}
	if latest.FromVersion != Version1 {
		t.Errorf("Expected from V1, got %s", latest.FromVersion)
	}
}

func TestDefaultMigrationContext(t *testing.T) {
	ctx := DefaultMigrationContext()

	if ctx.DryRun != false {
		t.Error("Expected DryRun to be false by default")
	}
	if ctx.Validate != true {
		t.Error("Expected Validate to be true by default")
	}
	if ctx.StopOnError != false {
		t.Error("Expected StopOnError to be false by default")
	}
	if ctx.Logger == nil {
		t.Error("Expected Logger to be set")
	}
}

func TestNewPolicyConverter(t *testing.T) {
	converter := NewPolicyConverter()

	if converter == nil {
		t.Fatal("Expected converter to be created")
	}

	if len(converter.converters) == 0 {
		t.Error("Expected builtin converters to be registered")
	}
}

func TestPolicyConverter_CanConvert(t *testing.T) {
	converter := NewPolicyConverter()

	// Same type - always possible
	if !converter.CanConvert(PolicyTypeCEL, PolicyTypeCEL) {
		t.Error("Expected same-type conversion to be possible")
	}

	// Builtin converters
	if !converter.CanConvert(PolicyTypeCEL, PolicyTypeOPA) {
		t.Error("Expected CEL to OPA conversion to be available")
	}
	if !converter.CanConvert(PolicyTypeOPA, PolicyTypeCEL) {
		t.Error("Expected OPA to CEL conversion to be available")
	}

	// Unavailable conversion
	if converter.CanConvert(PolicyTypeBuiltin, PolicyTypeOPA) {
		t.Error("Expected builtin to OPA conversion to not be available")
	}
}

func TestPolicyConverter_Convert_SameType(t *testing.T) {
	converter := NewPolicyConverter()

	policy := &Policy{
		ID:     "test",
		Name:   "Test Policy",
		Type:   PolicyTypeCEL,
		Policy: "action == 'read'",
	}

	converted, err := converter.Convert(policy, PolicyTypeCEL)
	if err != nil {
		t.Fatalf("Conversion failed: %v", err)
	}

	if converted.Type != PolicyTypeCEL {
		t.Errorf("Expected type CEL, got %s", converted.Type)
	}
	if converted.Policy != policy.Policy {
		t.Error("Expected policy content to be unchanged")
	}
}

func TestPolicyConverter_Convert_CELToOPA(t *testing.T) {
	converter := NewPolicyConverter()

	policy := &Policy{
		ID:       "test",
		Name:     "Test Policy",
		Type:     PolicyTypeCEL,
		Policy:   "action == 'read'",
		Metadata: make(map[string]string),
	}

	converted, err := converter.Convert(policy, PolicyTypeOPA)
	if err != nil {
		t.Fatalf("Conversion failed: %v", err)
	}

	if converted.Type != PolicyTypeOPA {
		t.Errorf("Expected type OPA, got %s", converted.Type)
	}
	if converted.Metadata["converted_from"] != "cel" {
		t.Error("Expected conversion metadata")
	}
	if converted.Metadata["requires_review"] != "true" {
		t.Error("Expected review flag")
	}
}

func TestPolicyConverter_Convert_OPAToCEL(t *testing.T) {
	converter := NewPolicyConverter()

	policy := &Policy{
		ID:   "test",
		Name: "Test Policy",
		Type: PolicyTypeOPA,
		Policy: `package keystone.test
import rego.v1
default allow := false
`,
		Metadata: make(map[string]string),
	}

	converted, err := converter.Convert(policy, PolicyTypeCEL)
	if err != nil {
		t.Fatalf("Conversion failed: %v", err)
	}

	if converted.Type != PolicyTypeCEL {
		t.Errorf("Expected type CEL, got %s", converted.Type)
	}
	if converted.Metadata["converted_from"] != "opa" {
		t.Error("Expected conversion metadata")
	}
}

func TestPolicyConverter_Convert_UnavailableConversion(t *testing.T) {
	converter := NewPolicyConverter()

	policy := &Policy{
		ID:     "test",
		Name:   "Test Policy",
		Type:   PolicyTypeBuiltin,
		Policy: `{"name": "test"}`,
	}

	_, err := converter.Convert(policy, PolicyTypeOPA)
	if err == nil {
		t.Error("Expected error for unavailable conversion")
	}
}

func TestPolicyConverter_ListAvailableConversions(t *testing.T) {
	converter := NewPolicyConverter()

	conversions := converter.ListAvailableConversions()

	if len(conversions) < 2 {
		t.Errorf("Expected at least 2 conversions, got %d", len(conversions))
	}

	// Check for expected conversions
	hasceltoopa := false
	hasoptocel := false
	for _, c := range conversions {
		if c == "cel_to_opa" {
			hasceltoopa = true
		}
		if c == "opa_to_cel" {
			hasoptocel = true
		}
	}

	if !hasceltoopa {
		t.Error("Expected cel_to_opa conversion")
	}
	if !hasoptocel {
		t.Error("Expected opa_to_cel conversion")
	}
}

func TestPolicyConverter_RegisterConverter(t *testing.T) {
	converter := NewPolicyConverter()

	converter.RegisterConverter(PolicyTypeBuiltin, PolicyTypeCEL, func(p *Policy, target PolicyType) (*Policy, error) {
		converted := *p
		converted.Type = target
		converted.Policy = "true"
		return &converted, nil
	})

	if !converter.CanConvert(PolicyTypeBuiltin, PolicyTypeCEL) {
		t.Error("Expected custom conversion to be registered")
	}
}

func TestNewPolicyValidator(t *testing.T) {
	validator := NewPolicyValidator()

	if validator == nil {
		t.Fatal("Expected validator to be created")
	}

	if len(validator.validators) == 0 {
		t.Error("Expected builtin validators to be registered")
	}
}

func TestPolicyValidator_Validate_Common(t *testing.T) {
	validator := NewPolicyValidator()

	// Empty policy
	errors := validator.Validate(&Policy{})

	hasIDError := false
	hasNameError := false
	hasTypeError := false
	for _, e := range errors {
		if e.Field == "id" {
			hasIDError = true
		}
		if e.Field == "name" {
			hasNameError = true
		}
		if e.Field == "type" {
			hasTypeError = true
		}
	}

	if !hasIDError {
		t.Error("Expected ID validation error")
	}
	if !hasNameError {
		t.Error("Expected name validation error")
	}
	if !hasTypeError {
		t.Error("Expected type validation error")
	}
}

func TestPolicyValidator_Validate_OPA(t *testing.T) {
	validator := NewPolicyValidator()

	// Valid OPA policy
	errors := validator.Validate(&Policy{
		ID:   "test",
		Name: "Test",
		Type: PolicyTypeOPA,
		Policy: `package keystone.test
import rego.v1
default allow := false
`,
	})

	for _, e := range errors {
		if e.Severe {
			t.Errorf("Unexpected severe error: %s - %s", e.Field, e.Message)
		}
	}

	// Invalid OPA policy (too short)
	errors = validator.Validate(&Policy{
		ID:     "test",
		Name:   "Test",
		Type:   PolicyTypeOPA,
		Policy: "short",
	})

	hasSevereError := false
	for _, e := range errors {
		if e.Severe {
			hasSevereError = true
			break
		}
	}
	if !hasSevereError {
		t.Error("Expected severe error for short OPA policy")
	}

	// OPA policy missing package
	errors = validator.Validate(&Policy{
		ID:     "test",
		Name:   "Test",
		Type:   PolicyTypeOPA,
		Policy: "this is a long policy with no required declaration",
	})

	hasMissingPackage := false
	for _, e := range errors {
		if e.Message == "OPA policy missing 'package' declaration" {
			hasMissingPackage = true
			break
		}
	}
	if !hasMissingPackage {
		t.Error("Expected error for missing package declaration")
	}
}

func TestPolicyValidator_Validate_CEL(t *testing.T) {
	validator := NewPolicyValidator()

	// Valid CEL policy
	errors := validator.Validate(&Policy{
		ID:     "test",
		Name:   "Test",
		Type:   PolicyTypeCEL,
		Policy: "action == 'read'",
	})

	for _, e := range errors {
		if e.Severe {
			t.Errorf("Unexpected severe error: %s - %s", e.Field, e.Message)
		}
	}

	// Invalid CEL policy (too short)
	errors = validator.Validate(&Policy{
		ID:     "test",
		Name:   "Test",
		Type:   PolicyTypeCEL,
		Policy: "x",
	})

	hasSevereError := false
	for _, e := range errors {
		if e.Severe {
			hasSevereError = true
			break
		}
	}
	if !hasSevereError {
		t.Error("Expected severe error for short CEL policy")
	}
}

func TestPolicyValidator_Validate_Builtin(t *testing.T) {
	validator := NewPolicyValidator()

	// Valid builtin policy
	errors := validator.Validate(&Policy{
		ID:     "test",
		Name:   "Test",
		Type:   PolicyTypeBuiltin,
		Policy: `{"name": "require-labels", "config": {"labels": ["env"]}}`,
	})

	for _, e := range errors {
		if e.Severe {
			t.Errorf("Unexpected severe error: %s - %s", e.Field, e.Message)
		}
	}

	// Invalid JSON
	errors = validator.Validate(&Policy{
		ID:     "test",
		Name:   "Test",
		Type:   PolicyTypeBuiltin,
		Policy: "not json",
	})

	hasJSONError := false
	for _, e := range errors {
		if e.Field == "policy" && e.Severe {
			hasJSONError = true
			break
		}
	}
	if !hasJSONError {
		t.Error("Expected error for invalid JSON")
	}

	// Missing name field
	errors = validator.Validate(&Policy{
		ID:     "test",
		Name:   "Test",
		Type:   PolicyTypeBuiltin,
		Policy: `{"config": {}}`,
	})

	hasMissingName := false
	for _, e := range errors {
		if e.Field == "policy.name" {
			hasMissingName = true
			break
		}
	}
	if !hasMissingName {
		t.Error("Expected error for missing name field")
	}
}

func TestPolicyValidator_RegisterValidator(t *testing.T) {
	validator := NewPolicyValidator()

	validator.RegisterValidator("custom", func(p *Policy) []ValidationError {
		return []ValidationError{{
			Field:   "custom",
			Message: "Custom validation",
			Severe:  false,
		}}
	})

	errors := validator.Validate(&Policy{
		ID:     "test",
		Name:   "Test",
		Type:   "custom",
		Policy: "anything",
	})

	hasCustomError := false
	for _, e := range errors {
		if e.Field == "custom" {
			hasCustomError = true
			break
		}
	}
	if !hasCustomError {
		t.Error("Expected custom validation error")
	}
}

func TestMigrationRecord(t *testing.T) {
	record := &MigrationRecord{
		ID:               "test-migration",
		FromVersion:      Version1,
		ToVersion:        Version2,
		Status:           MigrationCompleted,
		PoliciesMigrated: 10,
		PoliciesFailed:   0,
		StartedAt:        time.Now().Add(-time.Minute),
		CompletedAt:      time.Now(),
	}

	record.Duration = record.CompletedAt.Sub(record.StartedAt)

	if record.Duration < time.Minute {
		t.Error("Expected duration to be at least 1 minute")
	}
}

func TestMigrationPlan(t *testing.T) {
	plan := &MigrationPlan{
		FromVersion:      Version1,
		ToVersion:        Version3,
		AffectedPolicies: 100,
		Steps: []*MigrationStep{
			{Version: Version2, Description: "Step 1"},
			{Version: Version3, Description: "Step 2"},
		},
	}

	if len(plan.Steps) != 2 {
		t.Errorf("Expected 2 steps, got %d", len(plan.Steps))
	}
}

func TestContainsString(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"hello world", "world", true},
		{"hello world", "hello", true},
		{"hello world", "foo", false},
		{"", "foo", false},
		{"foo", "", true},
		{"package test", "package", true},
	}

	for _, tt := range tests {
		result := containsString(tt.s, tt.substr)
		if result != tt.expected {
			t.Errorf("containsString(%q, %q) = %v, want %v",
				tt.s, tt.substr, result, tt.expected)
		}
	}
}
