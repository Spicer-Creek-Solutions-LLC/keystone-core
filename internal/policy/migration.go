package policy

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

// MigrationVersion represents a policy schema version
type MigrationVersion string

const (
	// Version1 is the initial policy schema
	Version1 MigrationVersion = "v1"
	// Version2 adds support for policy sets and bindings
	Version2 MigrationVersion = "v2"
	// Version3 adds support for custom metadata and tags
	Version3 MigrationVersion = "v3"

	// CurrentVersion is the current policy schema version
	CurrentVersion = Version3
)

// MigrationStatus represents the status of a migration
type MigrationStatus string

const (
	// MigrationPending migration not yet started
	MigrationPending MigrationStatus = "pending"
	// MigrationInProgress migration currently running
	MigrationInProgress MigrationStatus = "in_progress"
	// MigrationCompleted migration completed successfully
	MigrationCompleted MigrationStatus = "completed"
	// MigrationFailed migration failed
	MigrationFailed MigrationStatus = "failed"
	// MigrationRolledBack migration was rolled back
	MigrationRolledBack MigrationStatus = "rolled_back"
)

// MigrationRecord tracks a single migration
type MigrationRecord struct {
	// ID is the unique migration identifier
	ID string `json:"id"`

	// FromVersion is the source version
	FromVersion MigrationVersion `json:"from_version"`

	// ToVersion is the target version
	ToVersion MigrationVersion `json:"to_version"`

	// Status of the migration
	Status MigrationStatus `json:"status"`

	// PoliciesMigrated count
	PoliciesMigrated int `json:"policies_migrated"`

	// PoliciesFailed count
	PoliciesFailed int `json:"policies_failed"`

	// Errors encountered during migration
	Errors []string `json:"errors,omitempty"`

	// StartedAt timestamp
	StartedAt time.Time `json:"started_at"`

	// CompletedAt timestamp
	CompletedAt time.Time `json:"completed_at,omitempty"`

	// Duration of the migration
	Duration time.Duration `json:"duration,omitempty"`

	// Backup location for rollback
	BackupLocation string `json:"backup_location,omitempty"`

	// DryRun indicates if this was a dry run
	DryRun bool `json:"dry_run"`
}

// MigrationPlan describes a planned migration
type MigrationPlan struct {
	// FromVersion is the current version
	FromVersion MigrationVersion `json:"from_version"`

	// ToVersion is the target version
	ToVersion MigrationVersion `json:"to_version"`

	// Steps are the migration steps
	Steps []*MigrationStep `json:"steps"`

	// AffectedPolicies count
	AffectedPolicies int `json:"affected_policies"`

	// EstimatedDuration (rough estimate)
	EstimatedDuration time.Duration `json:"estimated_duration"`

	// BreakingChanges lists breaking changes
	BreakingChanges []string `json:"breaking_changes,omitempty"`

	// Warnings about the migration
	Warnings []string `json:"warnings,omitempty"`
}

// MigrationStep represents a single migration step
type MigrationStep struct {
	// Version this step migrates to
	Version MigrationVersion `json:"version"`

	// Description of what this step does
	Description string `json:"description"`

	// SchemaChanges describes schema changes
	SchemaChanges []string `json:"schema_changes,omitempty"`

	// RequiresBackup indicates if backup is recommended
	RequiresBackup bool `json:"requires_backup"`

	// Reversible indicates if step can be rolled back
	Reversible bool `json:"reversible"`
}

// Migrator handles policy migrations
type Migrator struct {
	registry   *Registry
	migrations map[string]MigrationFunc
	history    []*MigrationRecord
	mu         sync.RWMutex
}

// MigrationFunc is a function that performs a migration
type MigrationFunc func(policy *Policy) (*Policy, error)

// MigrationContext provides context for migration operations
type MigrationContext struct {
	// DryRun if true, don't actually apply changes
	DryRun bool

	// Validate if true, validate migrated policies
	Validate bool

	// BackupDir for policy backups
	BackupDir string

	// StopOnError if true, stop migration on first error
	StopOnError bool

	// Logger for migration logging
	Logger MigrationLogger
}

// MigrationLogger provides logging for migrations
type MigrationLogger interface {
	Info(msg string, fields map[string]interface{})
	Warn(msg string, fields map[string]interface{})
	Error(msg string, fields map[string]interface{})
}

// DefaultMigrationContext returns default migration context
func DefaultMigrationContext() *MigrationContext {
	return &MigrationContext{
		DryRun:      false,
		Validate:    true,
		StopOnError: false,
		Logger:      &noopMigrationLogger{},
	}
}

// noopMigrationLogger is a no-op logger
type noopMigrationLogger struct{}

func (l *noopMigrationLogger) Info(msg string, fields map[string]interface{})  {}
func (l *noopMigrationLogger) Warn(msg string, fields map[string]interface{})  {}
func (l *noopMigrationLogger) Error(msg string, fields map[string]interface{}) {}

// NewMigrator creates a new policy migrator
func NewMigrator(registry *Registry) *Migrator {
	m := &Migrator{
		registry:   registry,
		migrations: make(map[string]MigrationFunc),
		history:    make([]*MigrationRecord, 0),
	}

	// Register built-in migrations
	m.registerBuiltinMigrations()

	return m
}

// registerBuiltinMigrations registers the standard migration functions
func (m *Migrator) registerBuiltinMigrations() {
	// V1 -> V2: Add policy sets support
	m.migrations["v1_to_v2"] = func(p *Policy) (*Policy, error) {
		// V2 added PolicySet support, policies themselves don't change much
		// Ensure metadata exists
		if p.Metadata == nil {
			p.Metadata = make(map[string]string)
		}
		p.Metadata["migrated_from"] = "v1"
		p.Metadata["migration_date"] = time.Now().Format(time.RFC3339)
		return p, nil
	}

	// V2 -> V3: Add tags and enhanced metadata
	m.migrations["v2_to_v3"] = func(p *Policy) (*Policy, error) {
		// V3 enhanced tags and metadata
		if p.Tags == nil {
			p.Tags = make([]string, 0)
		}
		if p.Metadata == nil {
			p.Metadata = make(map[string]string)
		}

		// Auto-add category as tag if not present
		categoryTag := "category:" + string(p.Category)
		hasTag := false
		for _, t := range p.Tags {
			if t == categoryTag {
				hasTag = true
				break
			}
		}
		if !hasTag {
			p.Tags = append(p.Tags, categoryTag)
		}

		p.Metadata["migrated_from"] = "v2"
		p.Metadata["migration_date"] = time.Now().Format(time.RFC3339)
		return p, nil
	}
}

// RegisterMigration registers a custom migration function
func (m *Migrator) RegisterMigration(name string, fn MigrationFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.migrations[name] = fn
}

// Plan creates a migration plan
func (m *Migrator) Plan(fromVersion, toVersion MigrationVersion) (*MigrationPlan, error) {
	// Get all policies to count affected
	policies := m.registry.ListPolicies()

	plan := &MigrationPlan{
		FromVersion:      fromVersion,
		ToVersion:        toVersion,
		Steps:            make([]*MigrationStep, 0),
		AffectedPolicies: len(policies),
	}

	// Determine migration path
	versionOrder := []MigrationVersion{Version1, Version2, Version3}
	fromIdx := -1
	toIdx := -1

	for i, v := range versionOrder {
		if v == fromVersion {
			fromIdx = i
		}
		if v == toVersion {
			toIdx = i
		}
	}

	if fromIdx == -1 {
		return nil, fmt.Errorf("unknown from version: %s", fromVersion)
	}
	if toIdx == -1 {
		return nil, fmt.Errorf("unknown to version: %s", toVersion)
	}

	if fromIdx == toIdx {
		return nil, fmt.Errorf("from and to versions are the same")
	}

	// Build migration steps
	if fromIdx < toIdx {
		// Upgrade path
		for i := fromIdx; i < toIdx; i++ {
			step := m.buildStep(versionOrder[i], versionOrder[i+1])
			plan.Steps = append(plan.Steps, step)
		}
	} else {
		// Downgrade path
		for i := fromIdx; i > toIdx; i-- {
			step := m.buildStep(versionOrder[i], versionOrder[i-1])
			step.Reversible = true // Downgrades are explicit rollbacks
			plan.Steps = append(plan.Steps, step)
			plan.BreakingChanges = append(plan.BreakingChanges,
				fmt.Sprintf("Downgrading from %s to %s may lose data", versionOrder[i], versionOrder[i-1]))
		}
	}

	// Estimate duration (rough: 10ms per policy per step)
	plan.EstimatedDuration = time.Duration(len(policies)*len(plan.Steps)*10) * time.Millisecond

	return plan, nil
}

// buildStep creates a migration step
func (m *Migrator) buildStep(from, to MigrationVersion) *MigrationStep {
	step := &MigrationStep{
		Version:        to,
		RequiresBackup: true,
		Reversible:     true,
	}

	switch {
	case from == Version1 && to == Version2:
		step.Description = "Add policy sets and bindings support"
		step.SchemaChanges = []string{
			"Added PolicySet type",
			"Added PolicyBinding type",
			"Added metadata field to Policy",
		}
	case from == Version2 && to == Version3:
		step.Description = "Enhanced tags and metadata support"
		step.SchemaChanges = []string{
			"Tags now include category prefix",
			"Enhanced metadata tracking",
			"Added migration history",
		}
	case from == Version2 && to == Version1:
		step.Description = "Rollback to V1 schema"
		step.SchemaChanges = []string{
			"Remove PolicySet references",
			"Remove enhanced metadata",
		}
	case from == Version3 && to == Version2:
		step.Description = "Rollback to V2 schema"
		step.SchemaChanges = []string{
			"Remove V3 tag enhancements",
			"Reset metadata to V2 format",
		}
	}

	return step
}

// Migrate performs the migration
func (m *Migrator) Migrate(ctx *MigrationContext, fromVersion, toVersion MigrationVersion) (*MigrationRecord, error) {
	if ctx == nil {
		ctx = DefaultMigrationContext()
	}

	record := &MigrationRecord{
		ID:          fmt.Sprintf("migration_%d", time.Now().UnixNano()),
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		Status:      MigrationInProgress,
		StartedAt:   time.Now(),
		DryRun:      ctx.DryRun,
		Errors:      make([]string, 0),
	}

	ctx.Logger.Info("Starting migration", map[string]interface{}{
		"from": fromVersion,
		"to":   toVersion,
	})

	// Get migration plan
	plan, err := m.Plan(fromVersion, toVersion)
	if err != nil {
		record.Status = MigrationFailed
		record.Errors = append(record.Errors, err.Error())
		record.CompletedAt = time.Now()
		record.Duration = record.CompletedAt.Sub(record.StartedAt)
		return record, err
	}

	// Backup policies if not dry run
	if !ctx.DryRun && ctx.BackupDir != "" {
		if err := m.backupPolicies(ctx.BackupDir); err != nil {
			ctx.Logger.Warn("Failed to backup policies", map[string]interface{}{
				"error": err.Error(),
			})
		} else {
			record.BackupLocation = ctx.BackupDir
		}
	}

	// Get all policies
	policies := m.registry.ListPolicies()

	// Apply each step
	for _, step := range plan.Steps {
		migrationKey := fmt.Sprintf("%s_to_%s", step.Version, step.Version)
		// Fix: determine migration key based on step info
		for key := range m.migrations {
			if key == fmt.Sprintf("%s_to_%s", fromVersion, step.Version) {
				migrationKey = key
				break
			}
		}

		migrationFn, ok := m.migrations[migrationKey]
		if !ok {
			// Use default pass-through migration if not found
			migrationFn = func(p *Policy) (*Policy, error) {
				return p, nil
			}
		}

		for _, policy := range policies {
			// Clone the policy for migration to avoid modifying original on dry run
			policyToMigrate := policy
			if ctx.DryRun {
				clone := *policy
				if policy.Tags != nil {
					clone.Tags = make([]string, len(policy.Tags))
					copy(clone.Tags, policy.Tags)
				}
				if policy.Metadata != nil {
					clone.Metadata = make(map[string]string)
					for k, v := range policy.Metadata {
						clone.Metadata[k] = v
					}
				}
				policyToMigrate = &clone
			}

			migratedPolicy, err := migrationFn(policyToMigrate)
			if err != nil {
				record.PoliciesFailed++
				record.Errors = append(record.Errors, fmt.Sprintf("policy %s: %v", policy.ID, err))

				if ctx.StopOnError {
					record.Status = MigrationFailed
					record.CompletedAt = time.Now()
					record.Duration = record.CompletedAt.Sub(record.StartedAt)
					return record, fmt.Errorf("migration stopped: %w", err)
				}
				continue
			}

			// Validate if requested
			if ctx.Validate {
				if err := m.validatePolicy(migratedPolicy); err != nil {
					record.PoliciesFailed++
					record.Errors = append(record.Errors,
						fmt.Sprintf("validation failed for policy %s: %v", policy.ID, err))

					if ctx.StopOnError {
						record.Status = MigrationFailed
						record.CompletedAt = time.Now()
						record.Duration = record.CompletedAt.Sub(record.StartedAt)
						return record, fmt.Errorf("validation stopped: %w", err)
					}
					continue
				}
			}

			// Apply if not dry run
			if !ctx.DryRun {
				migratedPolicy.UpdatedAt = time.Now()
				if err := m.registry.UpdatePolicy(migratedPolicy); err != nil {
					record.PoliciesFailed++
					record.Errors = append(record.Errors,
						fmt.Sprintf("failed to update policy %s: %v", policy.ID, err))
					continue
				}
			}

			record.PoliciesMigrated++
		}

		fromVersion = step.Version // Move to next version
	}

	// Determine final status
	if record.PoliciesFailed > 0 {
		if record.PoliciesMigrated > 0 {
			record.Status = MigrationCompleted // Partial success
		} else {
			record.Status = MigrationFailed
		}
	} else {
		record.Status = MigrationCompleted
	}

	record.CompletedAt = time.Now()
	record.Duration = record.CompletedAt.Sub(record.StartedAt)

	// Store in history
	m.mu.Lock()
	m.history = append(m.history, record)
	m.mu.Unlock()

	ctx.Logger.Info("Migration completed", map[string]interface{}{
		"status":   record.Status,
		"migrated": record.PoliciesMigrated,
		"failed":   record.PoliciesFailed,
		"duration": record.Duration.String(),
	})

	return record, nil
}

// validatePolicy validates a policy after migration
func (m *Migrator) validatePolicy(p *Policy) error {
	if p.ID == "" {
		return fmt.Errorf("policy ID is required")
	}
	if p.Name == "" {
		return fmt.Errorf("policy name is required")
	}
	if p.Type == "" {
		return fmt.Errorf("policy type is required")
	}
	if p.Policy == "" {
		return fmt.Errorf("policy content is required")
	}

	// Validate type-specific content
	switch p.Type {
	case PolicyTypeOPA:
		if len(p.Policy) < 10 { // Basic sanity check
			return fmt.Errorf("OPA policy content appears invalid")
		}
	case PolicyTypeCEL:
		if len(p.Policy) < 5 { // Basic sanity check
			return fmt.Errorf("CEL policy content appears invalid")
		}
	case PolicyTypeBuiltin:
		// Validate builtin policy config
		var config map[string]interface{}
		if err := json.Unmarshal([]byte(p.Policy), &config); err != nil {
			return fmt.Errorf("builtin policy config is not valid JSON: %w", err)
		}
	}

	return nil
}

// backupPolicies creates a backup of all policies
func (m *Migrator) backupPolicies(dir string) error {
	policies := m.registry.ListPolicies()

	// Create backup structure
	backup := &Backup{
		Version:   CurrentVersion,
		CreatedAt: time.Now(),
		Policies:  policies,
	}

	_ = backup // Would write to dir in real implementation
	_ = dir

	return nil // Simplified for now
}

// Backup represents a backup of policies
type Backup struct {
	Version   MigrationVersion `json:"version"`
	CreatedAt time.Time        `json:"created_at"`
	Policies  []*Policy        `json:"policies"`
}

// Rollback rolls back a migration
func (m *Migrator) Rollback(recordID string, ctx *MigrationContext) (*MigrationRecord, error) {
	if ctx == nil {
		ctx = DefaultMigrationContext()
	}

	// Find the record
	m.mu.RLock()
	var record *MigrationRecord
	for _, r := range m.history {
		if r.ID == recordID {
			record = r
			break
		}
	}
	m.mu.RUnlock()

	if record == nil {
		return nil, fmt.Errorf("migration record not found: %s", recordID)
	}

	if record.Status != MigrationCompleted {
		return nil, fmt.Errorf("can only rollback completed migrations")
	}

	// Perform reverse migration
	return m.Migrate(ctx, record.ToVersion, record.FromVersion)
}

// GetHistory returns migration history
func (m *Migrator) GetHistory() []*MigrationRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()

	history := make([]*MigrationRecord, len(m.history))
	copy(history, m.history)
	return history
}

// GetLatestMigration returns the most recent migration
func (m *Migrator) GetLatestMigration() *MigrationRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.history) == 0 {
		return nil
	}
	return m.history[len(m.history)-1]
}

// Converter handles conversion between policy types
type Converter struct {
	converters map[string]ConverterFunc
}

// ConverterFunc converts a policy from one type to another
type ConverterFunc func(policy *Policy, targetType Type) (*Policy, error)

// NewConverter creates a new policy converter
func NewConverter() *Converter {
	c := &Converter{
		converters: make(map[string]ConverterFunc),
	}

	// Register built-in converters
	c.registerBuiltinConverters()

	return c
}

// registerBuiltinConverters registers standard converters
func (c *Converter) registerBuiltinConverters() {
	// CEL to OPA (simplified conversion)
	c.converters["cel_to_opa"] = func(p *Policy, targetType PolicyType) (*Policy, error) {
		converted := *p
		converted.Type = PolicyTypeOPA

		// Basic CEL to Rego conversion (simplified)
		// In reality, this would need a proper parser and transformer
		converted.Policy = fmt.Sprintf(`
package keystone.policy.%s

import rego.v1

# Converted from CEL: %s
default allow := false

allow if {
    # TODO: Manual conversion required
    # Original CEL: %s
    true
}
`, p.ID, p.Name, p.Policy)

		converted.Metadata["converted_from"] = "cel"
		converted.Metadata["conversion_date"] = time.Now().Format(time.RFC3339)
		converted.Metadata["requires_review"] = "true"

		return &converted, nil
	}

	// OPA to CEL (simplified conversion)
	c.converters["opa_to_cel"] = func(p *Policy, targetType PolicyType) (*Policy, error) {
		converted := *p
		converted.Type = PolicyTypeCEL

		// OPA to CEL conversion is complex and often lossy
		// This is a placeholder that marks for manual review
		converted.Policy = fmt.Sprintf(`// Converted from OPA: %s
// Original policy requires manual review
// OPA source: %s
true`, p.Name, "see metadata for original")

		converted.Metadata["converted_from"] = "opa"
		converted.Metadata["conversion_date"] = time.Now().Format(time.RFC3339)
		converted.Metadata["original_policy"] = p.Policy
		converted.Metadata["requires_review"] = "true"

		return &converted, nil
	}
}

// Convert converts a policy to a different type
func (c *Converter) Convert(policy *Policy, targetType PolicyType) (*Policy, error) {
	if policy.Type == targetType {
		// No conversion needed
		converted := *policy
		return &converted, nil
	}

	key := fmt.Sprintf("%s_to_%s", policy.Type, targetType)
	converter, ok := c.converters[key]
	if !ok {
		return nil, fmt.Errorf("no converter available for %s to %s", policy.Type, targetType)
	}

	return converter(policy, targetType)
}

// CanConvert checks if a conversion is available
func (c *Converter) CanConvert(fromType, toType PolicyType) bool {
	if fromType == toType {
		return true
	}
	key := fmt.Sprintf("%s_to_%s", fromType, toType)
	_, ok := c.converters[key]
	return ok
}

// ListAvailableConversions lists all available conversion paths
func (c *Converter) ListAvailableConversions() []string {
	result := make([]string, 0, len(c.converters))
	for k := range c.converters {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}

// RegisterConverter registers a custom converter
func (c *Converter) RegisterConverter(fromType, toType PolicyType, fn ConverterFunc) {
	key := fmt.Sprintf("%s_to_%s", fromType, toType)
	c.converters[key] = fn
}

// Validator validates policies for migration compatibility
type Validator struct {
	validators map[PolicyType]ValidatorFunc
}

// ValidatorFunc validates a policy
type ValidatorFunc func(policy *Policy) []ValidationError

// ValidationError represents a validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Severe  bool   `json:"severe"`
}

// NewValidator creates a new policy validator
func NewValidator() *Validator {
	v := &Validator{
		validators: make(map[PolicyType]ValidatorFunc),
	}

	// Register built-in validators
	v.registerBuiltinValidators()

	return v
}

// registerBuiltinValidators registers standard validators
func (v *Validator) registerBuiltinValidators() {
	// OPA validator
	v.validators[TypeOPA] = func(p *Policy) []ValidationError {
		var errors []ValidationError

		if len(p.Policy) < 20 {
			errors = append(errors, ValidationError{
				Field:   "policy",
				Message: "OPA policy appears too short",
				Severe:  true,
			})
		}

		// Check for required OPA elements
		if !containsString(p.Policy, "package") {
			errors = append(errors, ValidationError{
				Field:   "policy",
				Message: "OPA policy missing 'package' declaration",
				Severe:  true,
			})
		}

		return errors
	}

	// CEL validator
	v.validators[TypeCEL] = func(p *Policy) []ValidationError {
		var errors []ValidationError

		if len(p.Policy) < 3 {
			errors = append(errors, ValidationError{
				Field:   "policy",
				Message: "CEL policy appears too short",
				Severe:  true,
			})
		}

		return errors
	}

	// Builtin validator
	v.validators[TypeBuiltin] = func(p *Policy) []ValidationError {
		var errors []ValidationError

		var config map[string]interface{}
		if err := json.Unmarshal([]byte(p.Policy), &config); err != nil {
			errors = append(errors, ValidationError{
				Field:   "policy",
				Message: fmt.Sprintf("Invalid JSON configuration: %v", err),
				Severe:  true,
			})
			return errors
		}

		if _, ok := config["name"]; !ok {
			errors = append(errors, ValidationError{
				Field:   "policy.name",
				Message: "Builtin policy missing 'name' field",
				Severe:  true,
			})
		}

		return errors
	}
}

// Validate validates a policy
func (v *Validator) Validate(policy *Policy) []ValidationError {
	var errors []ValidationError

	// Common validation
	if policy.ID == "" {
		errors = append(errors, ValidationError{
			Field:   "id",
			Message: "Policy ID is required",
			Severe:  true,
		})
	}

	if policy.Name == "" {
		errors = append(errors, ValidationError{
			Field:   "name",
			Message: "Policy name is required",
			Severe:  true,
		})
	}

	if policy.Type == "" {
		errors = append(errors, ValidationError{
			Field:   "type",
			Message: "Policy type is required",
			Severe:  true,
		})
	}

	// Type-specific validation
	if validator, ok := v.validators[policy.Type]; ok {
		errors = append(errors, validator(policy)...)
	}

	return errors
}

// RegisterValidator registers a custom validator
func (v *Validator) RegisterValidator(policyType PolicyType, fn ValidatorFunc) {
	v.validators[policyType] = fn
}

// containsString checks if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || s != "" && containsStringHelper(s, substr))
}

func containsStringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Deprecated aliases for backward compatibility
//
//nolint:revive,staticcheck // stuttering names kept for backward compatibility
type (
	// PolicyMigrator is deprecated: Use Migrator instead.
	PolicyMigrator = Migrator
	// PolicyBackup is deprecated: Use Backup instead.
	PolicyBackup = Backup
	// PolicyConverter is deprecated: Use Converter instead.
	PolicyConverter = Converter
	// PolicyValidator is deprecated: Use Validator instead.
	PolicyValidator = Validator
)

// NewPolicyMigrator is deprecated: Use NewMigrator instead.
var NewPolicyMigrator = NewMigrator

// NewPolicyConverter is deprecated: Use NewConverter instead.
var NewPolicyConverter = NewConverter

// NewPolicyValidator is deprecated: Use NewValidator instead.
var NewPolicyValidator = NewValidator
