package policy

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// AuditStore defines the interface for persistent policy audit storage
type AuditStore interface {
	// Store stores a single audit entry
	Store(ctx context.Context, entry *AuditEntry) error

	// StoreBatch stores multiple audit entries
	StoreBatch(ctx context.Context, entries []*AuditEntry) error

	// Query retrieves audit entries matching the filter
	Query(ctx context.Context, filter *AuditFilter) ([]*AuditEntry, error)

	// GetSummary returns summary statistics for entries matching the filter
	GetSummary(ctx context.Context, filter *AuditFilter) (*AuditSummary, error)

	// ApplyRetention applies retention policy, deleting old entries
	ApplyRetention(ctx context.Context, policy *AuditRetentionPolicy) (int64, error)

	// Close closes the store
	Close() error
}

// AuditRetentionPolicy defines retention rules for audit entries
type AuditRetentionPolicy struct {
	// MaxAge is the maximum age for audit entries (0 = no limit)
	MaxAge time.Duration `json:"max_age,omitempty" yaml:"max_age,omitempty"`

	// MaxCount is the maximum number of entries to keep (0 = no limit)
	MaxCount int `json:"max_count,omitempty" yaml:"max_count,omitempty"`

	// MinSeverity keeps only entries with violations at or above this severity
	// Entries with no violations are always kept unless MaxAge/MaxCount applies
	MinSeverity *Severity `json:"min_severity,omitempty" yaml:"min_severity,omitempty"`

	// RetentionInterval is how often to run retention automatically
	RetentionInterval time.Duration `json:"retention_interval,omitempty" yaml:"retention_interval,omitempty"`
}

// DefaultAuditRetentionPolicy returns a sensible default retention policy
func DefaultAuditRetentionPolicy() *AuditRetentionPolicy {
	return &AuditRetentionPolicy{
		MaxAge:            90 * 24 * time.Hour, // 90 days
		MaxCount:          100000,              // 100k entries
		RetentionInterval: 1 * time.Hour,       // Run hourly
	}
}

// AuditRedactionConfig defines which fields should be redacted in audit logs
type AuditRedactionConfig struct {
	// RedactMetadataKeys lists metadata keys whose values should be redacted
	RedactMetadataKeys []string `json:"redact_metadata_keys,omitempty" yaml:"redact_metadata_keys,omitempty"`

	// RedactPatterns lists regex patterns for values to redact anywhere
	RedactPatterns []string `json:"redact_patterns,omitempty" yaml:"redact_patterns,omitempty"`

	// RedactUser replaces user identifiers with a hash
	RedactUser bool `json:"redact_user,omitempty" yaml:"redact_user,omitempty"`

	// compiled patterns (internal)
	compiledPatterns []*regexp.Regexp
}

// DefaultAuditRedactionConfig returns a default redaction config for sensitive data
func DefaultAuditRedactionConfig() *AuditRedactionConfig {
	return &AuditRedactionConfig{
		RedactMetadataKeys: []string{
			"password", "secret", "token", "key", "credential",
			"api_key", "apikey", "auth", "authorization",
		},
		RedactPatterns: []string{
			// AWS credentials
			`AKIA[0-9A-Z]{16}`,
			// JWT tokens
			`eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`,
			// Generic secrets (base64-like strings > 32 chars)
			// Note: commented out as too aggressive - enable if needed
			// `[A-Za-z0-9+/]{32,}={0,2}`,
		},
	}
}

// Compile compiles the redaction patterns
func (c *AuditRedactionConfig) Compile() error {
	c.compiledPatterns = make([]*regexp.Regexp, 0, len(c.RedactPatterns))
	for _, pattern := range c.RedactPatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("invalid redaction pattern %q: %w", pattern, err)
		}
		c.compiledPatterns = append(c.compiledPatterns, re)
	}
	return nil
}

// Redact redacts sensitive values from an audit entry
func (c *AuditRedactionConfig) Redact(entry *AuditEntry) *AuditEntry {
	if c == nil {
		return entry
	}

	// Create a copy to avoid modifying the original
	redacted := *entry
	redacted.Metadata = make(map[string]interface{})
	for k, v := range entry.Metadata {
		redacted.Metadata[k] = v
	}

	// Redact metadata keys
	for _, key := range c.RedactMetadataKeys {
		keyLower := strings.ToLower(key)
		for k := range redacted.Metadata {
			if strings.Contains(strings.ToLower(k), keyLower) {
				redacted.Metadata[k] = "[REDACTED]"
			}
		}
	}

	// Redact pattern matches in metadata values
	for k, v := range redacted.Metadata {
		if strVal, ok := v.(string); ok {
			for _, re := range c.compiledPatterns {
				strVal = re.ReplaceAllString(strVal, "[REDACTED]")
			}
			redacted.Metadata[k] = strVal
		}
	}

	// Optionally redact user
	if c.RedactUser && redacted.User != "" {
		// Keep first 2 chars for debugging, hash the rest
		if len(redacted.User) > 2 {
			redacted.User = redacted.User[:2] + "***"
		} else {
			redacted.User = "***"
		}
	}

	return &redacted
}

// SQLitePolicyAuditStoreConfig configures the SQLite audit store
type SQLitePolicyAuditStoreConfig struct {
	// Path is the SQLite database file path
	Path string `json:"path" yaml:"path"`

	// MaxOpenConns is the maximum number of open connections
	MaxOpenConns int `json:"max_open_conns,omitempty" yaml:"max_open_conns,omitempty"`

	// MaxIdleConns is the maximum number of idle connections
	MaxIdleConns int `json:"max_idle_conns,omitempty" yaml:"max_idle_conns,omitempty"`

	// RetentionPolicy configures automatic retention
	RetentionPolicy *AuditRetentionPolicy `json:"retention_policy,omitempty" yaml:"retention_policy,omitempty"`

	// RedactionConfig configures data redaction
	RedactionConfig *AuditRedactionConfig `json:"redaction,omitempty" yaml:"redaction,omitempty"`

	// AutoRetention enables automatic retention cleanup
	AutoRetention bool `json:"auto_retention,omitempty" yaml:"auto_retention,omitempty"`
}

// DefaultSQLitePolicyAuditStoreConfig returns a default config
func DefaultSQLitePolicyAuditStoreConfig(path string) *SQLitePolicyAuditStoreConfig {
	return &SQLitePolicyAuditStoreConfig{
		Path:            path,
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		RetentionPolicy: DefaultAuditRetentionPolicy(),
		RedactionConfig: DefaultAuditRedactionConfig(),
		AutoRetention:   true,
	}
}

// SQLitePolicyAuditStore implements AuditStore using SQLite
type SQLitePolicyAuditStore struct {
	db       *sql.DB
	config   *SQLitePolicyAuditStoreConfig
	mu       sync.RWMutex
	redactor *AuditRedactionConfig

	// Auto-retention
	retentionCancel context.CancelFunc
	retentionWg     sync.WaitGroup
}

// NewSQLitePolicyAuditStore creates a new SQLite policy audit store
func NewSQLitePolicyAuditStore(config *SQLitePolicyAuditStoreConfig) (*SQLitePolicyAuditStore, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if config.Path == "" {
		return nil, fmt.Errorf("database path is required")
	}

	// Open database
	db, err := sql.Open("sqlite", config.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	if config.MaxOpenConns > 0 {
		db.SetMaxOpenConns(config.MaxOpenConns)
	}
	if config.MaxIdleConns > 0 {
		db.SetMaxIdleConns(config.MaxIdleConns)
	}

	store := &SQLitePolicyAuditStore{
		db:     db,
		config: config,
	}

	// Compile redaction patterns
	if config.RedactionConfig != nil {
		if err := config.RedactionConfig.Compile(); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to compile redaction patterns: %w", err)
		}
		store.redactor = config.RedactionConfig
	}

	// Initialize schema
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Start auto-retention if enabled
	if config.AutoRetention && config.RetentionPolicy != nil {
		store.startAutoRetention()
	}

	return store, nil
}

// initSchema initializes the database schema
func (s *SQLitePolicyAuditStore) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS policy_audit (
		id TEXT PRIMARY KEY,
		timestamp INTEGER NOT NULL,
		policy_id TEXT NOT NULL,
		policy_name TEXT,
		policy_type TEXT,
		resource_type TEXT,
		allowed INTEGER NOT NULL,
		duration_ns INTEGER NOT NULL,
		violations TEXT,
		enforcement_mode TEXT,
		user TEXT,
		action TEXT,
		metadata TEXT,
		created_at INTEGER NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_policy_audit_timestamp ON policy_audit(timestamp);
	CREATE INDEX IF NOT EXISTS idx_policy_audit_policy_id ON policy_audit(policy_id);
	CREATE INDEX IF NOT EXISTS idx_policy_audit_resource_type ON policy_audit(resource_type);
	CREATE INDEX IF NOT EXISTS idx_policy_audit_allowed ON policy_audit(allowed);
	CREATE INDEX IF NOT EXISTS idx_policy_audit_user ON policy_audit(user);
	CREATE INDEX IF NOT EXISTS idx_policy_audit_created_at ON policy_audit(created_at);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Store stores a single audit entry
func (s *SQLitePolicyAuditStore) Store(ctx context.Context, entry *AuditEntry) error {
	if entry == nil {
		return fmt.Errorf("entry is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Apply redaction
	if s.redactor != nil {
		entry = s.redactor.Redact(entry)
	}

	return s.storeEntry(ctx, entry)
}

// storeEntry stores an entry (caller must hold lock)
func (s *SQLitePolicyAuditStore) storeEntry(ctx context.Context, entry *AuditEntry) error {
	// Serialize violations and metadata
	violationsJSON, err := json.Marshal(entry.Violations)
	if err != nil {
		return fmt.Errorf("failed to marshal violations: %w", err)
	}

	metadataJSON, err := json.Marshal(entry.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		INSERT INTO policy_audit (
			id, timestamp, policy_id, policy_name, policy_type,
			resource_type, allowed, duration_ns, violations,
			enforcement_mode, user, action, metadata, created_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	allowed := 0
	if entry.Allowed {
		allowed = 1
	}

	_, err = s.db.ExecContext(ctx, query,
		entry.ID,
		entry.Timestamp.Unix(),
		entry.PolicyID,
		entry.PolicyName,
		string(entry.PolicyType),
		entry.ResourceType,
		allowed,
		entry.Duration.Nanoseconds(),
		string(violationsJSON),
		string(entry.EnforcementMode),
		entry.User,
		entry.Action,
		string(metadataJSON),
		time.Now().Unix(),
	)

	if err != nil {
		return fmt.Errorf("failed to store audit entry: %w", err)
	}

	return nil
}

// StoreBatch stores multiple audit entries
func (s *SQLitePolicyAuditStore) StoreBatch(ctx context.Context, entries []*AuditEntry) error {
	if len(entries) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO policy_audit (
			id, timestamp, policy_id, policy_name, policy_type,
			resource_type, allowed, duration_ns, violations,
			enforcement_mode, user, action, metadata, created_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now().Unix()

	for _, entry := range entries {
		if entry == nil {
			continue
		}

		// Apply redaction
		if s.redactor != nil {
			entry = s.redactor.Redact(entry)
		}

		violationsJSON, err := json.Marshal(entry.Violations)
		if err != nil {
			return fmt.Errorf("failed to marshal violations: %w", err)
		}

		metadataJSON, err := json.Marshal(entry.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}

		allowed := 0
		if entry.Allowed {
			allowed = 1
		}

		_, err = stmt.ExecContext(ctx,
			entry.ID,
			entry.Timestamp.Unix(),
			entry.PolicyID,
			entry.PolicyName,
			string(entry.PolicyType),
			entry.ResourceType,
			allowed,
			entry.Duration.Nanoseconds(),
			string(violationsJSON),
			string(entry.EnforcementMode),
			entry.User,
			entry.Action,
			string(metadataJSON),
			now,
		)
		if err != nil {
			return fmt.Errorf("failed to store audit entry: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Query retrieves audit entries matching the filter
func (s *SQLitePolicyAuditStore) Query(ctx context.Context, filter *AuditFilter) ([]*AuditEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT id, timestamp, policy_id, policy_name, policy_type,
		resource_type, allowed, duration_ns, violations,
		enforcement_mode, user, action, metadata
		FROM policy_audit WHERE 1=1`
	args := make([]interface{}, 0)

	if filter != nil {
		if filter.PolicyID != "" {
			query += " AND policy_id = ?"
			args = append(args, filter.PolicyID)
		}
		if filter.ResourceType != "" {
			query += " AND resource_type = ?"
			args = append(args, filter.ResourceType)
		}
		if filter.Allowed != nil {
			allowed := 0
			if *filter.Allowed {
				allowed = 1
			}
			query += " AND allowed = ?"
			args = append(args, allowed)
		}
		if !filter.StartTime.IsZero() {
			query += " AND timestamp >= ?"
			args = append(args, filter.StartTime.Unix())
		}
		if !filter.EndTime.IsZero() {
			query += " AND timestamp <= ?"
			args = append(args, filter.EndTime.Unix())
		}
		if filter.User != "" {
			query += " AND user = ?"
			args = append(args, filter.User)
		}
		if filter.Action != "" {
			query += " AND action = ?"
			args = append(args, filter.Action)
		}
	}

	query += " ORDER BY timestamp DESC"

	if filter != nil && filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit entries: %w", err)
	}
	defer rows.Close()

	entries := make([]*AuditEntry, 0)
	for rows.Next() {
		entry, err := s.scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return entries, nil
}

// scanEntry scans a single row into an AuditEntry
func (s *SQLitePolicyAuditStore) scanEntry(rows *sql.Rows) (*AuditEntry, error) {
	var (
		id              string
		timestamp       int64
		policyID        string
		policyName      sql.NullString
		policyType      sql.NullString
		resourceType    sql.NullString
		allowed         int
		durationNs      int64
		violationsJSON  string
		enforcementMode sql.NullString
		user            sql.NullString
		action          sql.NullString
		metadataJSON    string
	)

	err := rows.Scan(
		&id, &timestamp, &policyID, &policyName, &policyType,
		&resourceType, &allowed, &durationNs, &violationsJSON,
		&enforcementMode, &user, &action, &metadataJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan audit entry: %w", err)
	}

	entry := &AuditEntry{
		ID:           id,
		Timestamp:    time.Unix(timestamp, 0),
		PolicyID:     policyID,
		PolicyName:   policyName.String,
		PolicyType:   PolicyType(policyType.String),
		ResourceType: resourceType.String,
		Allowed:      allowed == 1,
		Duration:     time.Duration(durationNs),
		User:         user.String,
		Action:       action.String,
	}

	if enforcementMode.Valid {
		entry.EnforcementMode = EnforcementMode(enforcementMode.String)
	}

	if violationsJSON != "" {
		if err := json.Unmarshal([]byte(violationsJSON), &entry.Violations); err != nil {
			return nil, fmt.Errorf("failed to unmarshal violations: %w", err)
		}
	}

	if metadataJSON != "" {
		entry.Metadata = make(map[string]interface{})
		if err := json.Unmarshal([]byte(metadataJSON), &entry.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return entry, nil
}

// GetSummary returns summary statistics for entries matching the filter
func (s *SQLitePolicyAuditStore) GetSummary(ctx context.Context, filter *AuditFilter) (*AuditSummary, error) {
	entries, err := s.Query(ctx, filter)
	if err != nil {
		return nil, err
	}

	summary := &AuditSummary{
		TotalEvaluations:      len(entries),
		AllowedEvaluations:    0,
		DeniedEvaluations:     0,
		TotalViolations:       0,
		ViolationsBySeverity:  make(map[Severity]int),
		ViolationsByPolicy:    make(map[string]int),
		EvaluationsByResource: make(map[string]int),
		AverageDuration:       0,
	}

	totalDuration := time.Duration(0)

	for _, entry := range entries {
		if entry.Allowed {
			summary.AllowedEvaluations++
		} else {
			summary.DeniedEvaluations++
		}

		summary.TotalViolations += len(entry.Violations)
		totalDuration += entry.Duration

		for _, v := range entry.Violations {
			summary.ViolationsBySeverity[v.Severity]++
			summary.ViolationsByPolicy[entry.PolicyID]++
		}

		if entry.ResourceType != "" {
			summary.EvaluationsByResource[entry.ResourceType]++
		}
	}

	if len(entries) > 0 {
		summary.AverageDuration = totalDuration / time.Duration(len(entries))
	}

	return summary, nil
}

// ApplyRetention applies retention policy, deleting old entries
func (s *SQLitePolicyAuditStore) ApplyRetention(ctx context.Context, policy *AuditRetentionPolicy) (int64, error) {
	if policy == nil {
		return 0, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var totalDeleted int64

	// Delete entries older than MaxAge
	if policy.MaxAge > 0 {
		cutoff := time.Now().Add(-policy.MaxAge).Unix()
		result, err := s.db.ExecContext(ctx, "DELETE FROM policy_audit WHERE timestamp < ?", cutoff)
		if err != nil {
			return totalDeleted, fmt.Errorf("failed to delete old entries: %w", err)
		}
		deleted, _ := result.RowsAffected()
		totalDeleted += deleted
	}

	// Keep only MaxCount most recent entries
	if policy.MaxCount > 0 {
		// Delete entries beyond the limit
		result, err := s.db.ExecContext(ctx, `
			DELETE FROM policy_audit WHERE id NOT IN (
				SELECT id FROM policy_audit ORDER BY timestamp DESC LIMIT ?
			)
		`, policy.MaxCount)
		if err != nil {
			return totalDeleted, fmt.Errorf("failed to enforce max count: %w", err)
		}
		deleted, _ := result.RowsAffected()
		totalDeleted += deleted
	}

	// Note: MinSeverity filtering would require more complex logic
	// to parse violations JSON - skipped for now

	return totalDeleted, nil
}

// startAutoRetention starts the automatic retention goroutine
func (s *SQLitePolicyAuditStore) startAutoRetention() {
	if s.config.RetentionPolicy == nil || s.config.RetentionPolicy.RetentionInterval <= 0 {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.retentionCancel = cancel

	s.retentionWg.Add(1)
	go func() {
		defer s.retentionWg.Done()

		ticker := time.NewTicker(s.config.RetentionPolicy.RetentionInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = s.ApplyRetention(ctx, s.config.RetentionPolicy)
			}
		}
	}()
}

// Close closes the store
func (s *SQLitePolicyAuditStore) Close() error {
	// Stop auto-retention
	if s.retentionCancel != nil {
		s.retentionCancel()
		s.retentionWg.Wait()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Count returns the total number of audit entries
func (s *SQLitePolicyAuditStore) Count(ctx context.Context) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int64
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM policy_audit").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count entries: %w", err)
	}
	return count, nil
}
