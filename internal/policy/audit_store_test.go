package policy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLitePolicyAuditStore_StoreAndQuery(t *testing.T) {
	// Create temp database
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "audit_test.db")

	config := &SQLitePolicyAuditStoreConfig{
		Path:          dbPath,
		MaxOpenConns:  1,
		MaxIdleConns:  1,
		AutoRetention: false,
	}

	store, err := NewSQLitePolicyAuditStore(config)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Store an entry
	entry := &AuditEntry{
		ID:              "test-1",
		Timestamp:       time.Now(),
		PolicyID:        "policy-1",
		PolicyName:      "Test Policy",
		PolicyType:      PolicyTypeOPA,
		ResourceType:    "deployment",
		Allowed:         false,
		Duration:        100 * time.Millisecond,
		EnforcementMode: ModeEnforce,
		User:            "admin",
		Action:          "create",
		Violations: []Violation{
			{
				Rule:     "rule-1",
				Message:  "Resource missing required label",
				Severity: SeverityHigh,
			},
		},
		Metadata: map[string]interface{}{
			"namespace": "default",
		},
	}

	if err := store.Store(ctx, entry); err != nil {
		t.Fatalf("failed to store entry: %v", err)
	}

	// Query all entries
	entries, err := store.Query(ctx, nil)
	if err != nil {
		t.Fatalf("failed to query entries: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	if entries[0].PolicyID != "policy-1" {
		t.Errorf("expected policy-1, got %s", entries[0].PolicyID)
	}

	if entries[0].Allowed {
		t.Error("expected allowed=false")
	}

	if len(entries[0].Violations) != 1 {
		t.Errorf("expected 1 violation, got %d", len(entries[0].Violations))
	}
}

func TestSQLitePolicyAuditStore_QueryWithFilter(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "audit_test.db")

	config := DefaultSQLitePolicyAuditStoreConfig(dbPath)
	config.AutoRetention = false

	store, err := NewSQLitePolicyAuditStore(config)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Store multiple entries
	now := time.Now()
	entries := []*AuditEntry{
		{ID: "e1", Timestamp: now.Add(-2 * time.Hour), PolicyID: "p1", Allowed: true, User: "user1"},
		{ID: "e2", Timestamp: now.Add(-1 * time.Hour), PolicyID: "p2", Allowed: false, User: "user2"},
		{ID: "e3", Timestamp: now, PolicyID: "p1", Allowed: true, User: "user1"},
	}

	if err := store.StoreBatch(ctx, entries); err != nil {
		t.Fatalf("failed to store batch: %v", err)
	}

	// Filter by policy ID
	filter := &AuditFilter{PolicyID: "p1"}
	results, err := store.Query(ctx, filter)
	if err != nil {
		t.Fatalf("failed to query with filter: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results for policy p1, got %d", len(results))
	}

	// Filter by user
	filter = &AuditFilter{User: "user2"}
	results, err = store.Query(ctx, filter)
	if err != nil {
		t.Fatalf("failed to query with filter: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result for user2, got %d", len(results))
	}

	// Filter by allowed status
	allowed := false
	filter = &AuditFilter{Allowed: &allowed}
	results, err = store.Query(ctx, filter)
	if err != nil {
		t.Fatalf("failed to query with filter: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 denied result, got %d", len(results))
	}

	// Filter by time range
	filter = &AuditFilter{
		StartTime: now.Add(-90 * time.Minute),
		EndTime:   now.Add(time.Minute),
	}
	results, err = store.Query(ctx, filter)
	if err != nil {
		t.Fatalf("failed to query with filter: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results in time range, got %d", len(results))
	}

	// Filter with limit
	filter = &AuditFilter{Limit: 1}
	results, err = store.Query(ctx, filter)
	if err != nil {
		t.Fatalf("failed to query with limit: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result with limit, got %d", len(results))
	}
}

func TestSQLitePolicyAuditStore_Retention(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "audit_test.db")

	config := &SQLitePolicyAuditStoreConfig{
		Path:          dbPath,
		AutoRetention: false,
	}

	store, err := NewSQLitePolicyAuditStore(config)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Store entries with different ages
	now := time.Now()
	entries := []*AuditEntry{
		{ID: "old1", Timestamp: now.Add(-48 * time.Hour), PolicyID: "p1"},
		{ID: "old2", Timestamp: now.Add(-36 * time.Hour), PolicyID: "p1"},
		{ID: "new1", Timestamp: now.Add(-12 * time.Hour), PolicyID: "p1"},
		{ID: "new2", Timestamp: now, PolicyID: "p1"},
	}

	if err := store.StoreBatch(ctx, entries); err != nil {
		t.Fatalf("failed to store batch: %v", err)
	}

	// Apply retention - delete entries older than 24 hours
	policy := &AuditRetentionPolicy{
		MaxAge: 24 * time.Hour,
	}

	deleted, err := store.ApplyRetention(ctx, policy)
	if err != nil {
		t.Fatalf("failed to apply retention: %v", err)
	}

	if deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", deleted)
	}

	// Verify remaining entries
	count, err := store.Count(ctx)
	if err != nil {
		t.Fatalf("failed to count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 remaining entries, got %d", count)
	}
}

func TestSQLitePolicyAuditStore_RetentionMaxCount(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "audit_test.db")

	config := &SQLitePolicyAuditStoreConfig{
		Path:          dbPath,
		AutoRetention: false,
	}

	store, err := NewSQLitePolicyAuditStore(config)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Store 10 entries
	entries := make([]*AuditEntry, 10)
	for i := 0; i < 10; i++ {
		entries[i] = &AuditEntry{
			ID:        generateAuditID(),
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
			PolicyID:  "p1",
		}
	}

	if err := store.StoreBatch(ctx, entries); err != nil {
		t.Fatalf("failed to store batch: %v", err)
	}

	// Apply retention - keep only 5
	policy := &AuditRetentionPolicy{
		MaxCount: 5,
	}

	deleted, err := store.ApplyRetention(ctx, policy)
	if err != nil {
		t.Fatalf("failed to apply retention: %v", err)
	}

	if deleted != 5 {
		t.Errorf("expected 5 deleted, got %d", deleted)
	}

	count, err := store.Count(ctx)
	if err != nil {
		t.Fatalf("failed to count: %v", err)
	}
	if count != 5 {
		t.Errorf("expected 5 remaining entries, got %d", count)
	}
}

func TestAuditRedactionConfig_Redact(t *testing.T) {
	config := &AuditRedactionConfig{
		RedactMetadataKeys: []string{"password", "secret", "token"},
		RedactPatterns:     []string{`AKIA[0-9A-Z]{16}`},
		RedactUser:         true,
	}

	if err := config.Compile(); err != nil {
		t.Fatalf("failed to compile: %v", err)
	}

	entry := &AuditEntry{
		ID:   "test-1",
		User: "administrator",
		Metadata: map[string]interface{}{
			"password":     "secret123",
			"api_token":    "abc123",
			"aws_key":      "AKIAIOSFODNN7EXAMPLE",
			"normal_field": "safe value",
		},
	}

	redacted := config.Redact(entry)

	// Check user was redacted
	if redacted.User != "ad***" {
		t.Errorf("expected user 'ad***', got %s", redacted.User)
	}

	// Check password was redacted
	if redacted.Metadata["password"] != "[REDACTED]" {
		t.Errorf("expected password redacted, got %v", redacted.Metadata["password"])
	}

	// Check api_token was redacted (contains 'token')
	if redacted.Metadata["api_token"] != "[REDACTED]" {
		t.Errorf("expected api_token redacted, got %v", redacted.Metadata["api_token"])
	}

	// Check AWS key pattern was redacted
	if redacted.Metadata["aws_key"] != "[REDACTED]" {
		t.Errorf("expected aws_key pattern redacted, got %v", redacted.Metadata["aws_key"])
	}

	// Check normal field was not redacted
	if redacted.Metadata["normal_field"] != "safe value" {
		t.Errorf("expected normal_field unchanged, got %v", redacted.Metadata["normal_field"])
	}

	// Verify original was not modified
	if entry.User != "administrator" {
		t.Error("original entry should not be modified")
	}
}

func TestAuditRedactionConfig_ShortUser(t *testing.T) {
	config := &AuditRedactionConfig{
		RedactUser: true,
	}

	entry := &AuditEntry{User: "a"}
	redacted := config.Redact(entry)
	if redacted.User != "***" {
		t.Errorf("expected '***' for short user, got %s", redacted.User)
	}

	entry = &AuditEntry{User: "ab"}
	redacted = config.Redact(entry)
	if redacted.User != "***" {
		t.Errorf("expected '***' for 2-char user, got %s", redacted.User)
	}

	entry = &AuditEntry{User: "abc"}
	redacted = config.Redact(entry)
	if redacted.User != "ab***" {
		t.Errorf("expected 'ab***' for 3-char user, got %s", redacted.User)
	}
}

func TestSQLitePolicyAuditStore_GetSummary(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "audit_test.db")

	config := &SQLitePolicyAuditStoreConfig{
		Path:          dbPath,
		AutoRetention: false,
	}

	store, err := NewSQLitePolicyAuditStore(config)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Store entries with violations
	entries := []*AuditEntry{
		{
			ID:           "e1",
			Timestamp:    time.Now(),
			PolicyID:     "p1",
			Allowed:      true,
			Duration:     100 * time.Millisecond,
			ResourceType: "deployment",
		},
		{
			ID:           "e2",
			Timestamp:    time.Now(),
			PolicyID:     "p2",
			Allowed:      false,
			Duration:     200 * time.Millisecond,
			ResourceType: "deployment",
			Violations: []Violation{
				{Rule: "r1", Severity: SeverityHigh},
				{Rule: "r2", Severity: SeverityMedium},
			},
		},
		{
			ID:           "e3",
			Timestamp:    time.Now(),
			PolicyID:     "p1",
			Allowed:      false,
			Duration:     150 * time.Millisecond,
			ResourceType: "service",
			Violations: []Violation{
				{Rule: "r3", Severity: SeverityHigh},
			},
		},
	}

	if err := store.StoreBatch(ctx, entries); err != nil {
		t.Fatalf("failed to store batch: %v", err)
	}

	summary, err := store.GetSummary(ctx, nil)
	if err != nil {
		t.Fatalf("failed to get summary: %v", err)
	}

	if summary.TotalEvaluations != 3 {
		t.Errorf("expected 3 total evaluations, got %d", summary.TotalEvaluations)
	}

	if summary.AllowedEvaluations != 1 {
		t.Errorf("expected 1 allowed, got %d", summary.AllowedEvaluations)
	}

	if summary.DeniedEvaluations != 2 {
		t.Errorf("expected 2 denied, got %d", summary.DeniedEvaluations)
	}

	if summary.TotalViolations != 3 {
		t.Errorf("expected 3 total violations, got %d", summary.TotalViolations)
	}

	if summary.ViolationsBySeverity[SeverityHigh] != 2 {
		t.Errorf("expected 2 high severity violations, got %d", summary.ViolationsBySeverity[SeverityHigh])
	}

	if summary.EvaluationsByResource["deployment"] != 2 {
		t.Errorf("expected 2 deployment evaluations, got %d", summary.EvaluationsByResource["deployment"])
	}
}

func TestSQLitePolicyAuditStore_ConfigValidation(t *testing.T) {
	// Nil config
	_, err := NewSQLitePolicyAuditStore(nil)
	if err == nil {
		t.Error("expected error for nil config")
	}

	// Empty path
	_, err = NewSQLitePolicyAuditStore(&SQLitePolicyAuditStoreConfig{})
	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestDefaultAuditRetentionPolicy(t *testing.T) {
	policy := DefaultAuditRetentionPolicy()

	if policy.MaxAge != 90*24*time.Hour {
		t.Errorf("expected 90 days max age, got %v", policy.MaxAge)
	}

	if policy.MaxCount != 100000 {
		t.Errorf("expected 100k max count, got %d", policy.MaxCount)
	}

	if policy.RetentionInterval != 1*time.Hour {
		t.Errorf("expected 1 hour retention interval, got %v", policy.RetentionInterval)
	}
}

func TestDefaultAuditRedactionConfig(t *testing.T) {
	config := DefaultAuditRedactionConfig()

	if len(config.RedactMetadataKeys) == 0 {
		t.Error("expected default redaction keys")
	}

	if len(config.RedactPatterns) == 0 {
		t.Error("expected default redaction patterns")
	}

	// Should compile without error
	if err := config.Compile(); err != nil {
		t.Errorf("failed to compile default config: %v", err)
	}
}

func TestAuditRedactionConfig_InvalidPattern(t *testing.T) {
	config := &AuditRedactionConfig{
		RedactPatterns: []string{"[invalid"},
	}

	if err := config.Compile(); err == nil {
		t.Error("expected error for invalid regex pattern")
	}
}

// Test cleanup
func TestSQLitePolicyAuditStore_Close(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "audit_test.db")

	config := &SQLitePolicyAuditStoreConfig{
		Path:          dbPath,
		AutoRetention: true,
		RetentionPolicy: &AuditRetentionPolicy{
			MaxAge:            24 * time.Hour,
			RetentionInterval: 100 * time.Millisecond,
		},
	}

	store, err := NewSQLitePolicyAuditStore(config)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	// Close should stop auto-retention goroutine
	if err := store.Close(); err != nil {
		t.Errorf("failed to close store: %v", err)
	}

	// Verify database file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file should exist after close")
	}
}
