package identity

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	// Pure Go SQLite driver
	_ "modernc.org/sqlite"
)

// SQLiteTokenStoreConfig configures the SQLite token store.
type SQLiteTokenStoreConfig struct {
	// Path is the path to the SQLite database file.
	// Required.
	Path string

	// WALMode enables Write-Ahead Logging for better concurrent access.
	// Recommended for production use.
	WALMode bool

	// BusyTimeout is the timeout in milliseconds to wait when the database
	// is locked by another process. Default is 5000ms.
	BusyTimeout int
}

// SQLiteTokenStore is a SQLite-backed implementation of JoinTokenStore.
// Unlike InMemoryTokenStore, this provides persistence across restarts.
//
// Limitations:
//   - Slower than in-memory for high-frequency operations due to disk I/O
//   - Requires filesystem access
//   - Database file should be on local storage for best performance
//
// Suitable for:
//   - Production deployments requiring token persistence
//   - Scenarios where tokens must survive server restarts
//   - Audit/compliance requirements for token usage tracking
type SQLiteTokenStore struct {
	db   *sql.DB
	path string
	mu   sync.Mutex
}

// NewSQLiteTokenStore creates a new SQLite-backed token store.
func NewSQLiteTokenStore(config *SQLiteTokenStoreConfig) (*SQLiteTokenStore, error) {
	if config == nil {
		return nil, fmt.Errorf("config required")
	}
	if config.Path == "" {
		return nil, fmt.Errorf("database path required")
	}

	busyTimeout := config.BusyTimeout
	if busyTimeout == 0 {
		busyTimeout = 5000
	}

	dsn := fmt.Sprintf("%s?_busy_timeout=%d", config.Path, busyTimeout)
	if config.WALMode {
		dsn += "&_journal_mode=WAL"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	store := &SQLiteTokenStore{
		db:   db,
		path: config.Path,
	}

	// Initialize schema
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

// initSchema creates the required tables and indexes.
func (s *SQLiteTokenStore) initSchema() error {
	schema := `
		CREATE TABLE IF NOT EXISTS join_tokens (
			token_hash TEXT PRIMARY KEY,
			salt TEXT NOT NULL,
			token_prefix TEXT NOT NULL,
			agent_id TEXT,
			expires_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL,
			used INTEGER NOT NULL DEFAULT 0,
			used_at DATETIME,
			used_by TEXT,
			metadata TEXT
		);

		CREATE INDEX IF NOT EXISTS idx_join_tokens_expires_at ON join_tokens(expires_at);
		CREATE INDEX IF NOT EXISTS idx_join_tokens_used ON join_tokens(used);
		CREATE INDEX IF NOT EXISTS idx_join_tokens_agent_id ON join_tokens(agent_id);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Close closes the database connection.
func (s *SQLiteTokenStore) Close() error {
	return s.db.Close()
}

// Create creates a new join token. The token must have the Token field set
// with the plaintext value. This function will generate a salt, compute the
// hash, and store only the hash. The Token field is cleared before storage.
// The caller should save the returned token's Token field (from the input)
// as it will not be available after this call.
func (s *SQLiteTokenStore) Create(ctx context.Context, token *JoinToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if token.Token == "" {
		return fmt.Errorf("token value required")
	}

	// Generate salt for this token
	salt, err := generateSalt()
	if err != nil {
		return fmt.Errorf("failed to generate salt: %w", err)
	}

	// Compute the salted hash
	tokenHash := hashToken(token.Token, salt)

	// Check if this hash already exists (extremely unlikely with random salt)
	var exists int
	err = s.db.QueryRowContext(ctx, "SELECT 1 FROM join_tokens WHERE token_hash = ?", tokenHash).Scan(&exists)
	if err == nil {
		return fmt.Errorf("token already exists")
	} else if err != sql.ErrNoRows {
		return fmt.Errorf("failed to check for existing token: %w", err)
	}

	// Store the token prefix for identification
	prefix := tokenPrefix(token.Token)

	// Set created time if not set
	if token.CreatedAt.IsZero() {
		token.CreatedAt = time.Now()
	}

	// Serialize metadata
	var metadataJSON []byte
	if token.Metadata != nil {
		metadataJSON, err = json.Marshal(token.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	// Insert the token
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO join_tokens (
			token_hash, salt, token_prefix, agent_id, expires_at, created_at,
			used, used_at, used_by, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, tokenHash, salt, prefix, token.AgentID, token.ExpiresAt, token.CreatedAt,
		boolToInt(token.Used), nullTime(token.UsedAt), nullString(token.UsedBy),
		nullBytes(metadataJSON))
	if err != nil {
		return fmt.Errorf("failed to insert token: %w", err)
	}

	// Update token fields with generated values
	token.TokenHash = tokenHash
	token.Salt = salt
	token.TokenPrefix = prefix

	// Clear the raw token value - it should never be persisted
	token.Token = ""

	return nil
}

// Get retrieves a join token by its plaintext value. The store will hash
// the provided value with each token's salt to find a match.
func (s *SQLiteTokenStore) Get(ctx context.Context, tokenValue string) (*JoinToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// We need to iterate through all tokens and try hashing with each salt
	// This is O(n) but necessary with per-token salts
	rows, err := s.db.QueryContext(ctx, `
		SELECT token_hash, salt, token_prefix, agent_id, expires_at, created_at,
			   used, used_at, used_by, metadata
		FROM join_tokens
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query tokens: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		token, err := scanToken(rows)
		if err != nil {
			return nil, err
		}

		if hashToken(tokenValue, token.Salt) == token.TokenHash {
			return token, nil
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tokens: %w", err)
	}

	return nil, fmt.Errorf("token not found")
}

// MarkUsed marks a token as used. The tokenValue should be the plaintext token.
func (s *SQLiteTokenStore) MarkUsed(ctx context.Context, tokenValue, usedBy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find the token first to get its hash
	foundHash, err := s.findTokenHash(ctx, tokenValue)
	if err != nil {
		return err
	}

	if foundHash == "" {
		return fmt.Errorf("token not found")
	}

	// Update the token
	now := time.Now()
	_, err = s.db.ExecContext(ctx, `
		UPDATE join_tokens SET used = 1, used_at = ?, used_by = ?
		WHERE token_hash = ?
	`, now, usedBy, foundHash)
	if err != nil {
		return fmt.Errorf("failed to update token: %w", err)
	}

	return nil
}

// Delete deletes a token by its plaintext value.
func (s *SQLiteTokenStore) Delete(ctx context.Context, tokenValue string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find the token first to get its hash
	foundHash, err := s.findTokenHash(ctx, tokenValue)
	if err != nil {
		return err
	}

	if foundHash == "" {
		return fmt.Errorf("token not found")
	}

	// Delete the token
	_, err = s.db.ExecContext(ctx, "DELETE FROM join_tokens WHERE token_hash = ?", foundHash)
	if err != nil {
		return fmt.Errorf("failed to delete token: %w", err)
	}

	return nil
}

// findTokenHash finds the hash of a token by iterating through all tokens
// and checking the hash with each token's salt. This must be called without
// holding the mutex as it needs to query the database.
func (s *SQLiteTokenStore) findTokenHash(ctx context.Context, tokenValue string) (string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT token_hash, salt FROM join_tokens")
	if err != nil {
		return "", fmt.Errorf("failed to query tokens: %w", err)
	}
	defer rows.Close()

	var foundHash string
	for rows.Next() {
		var hash, salt string
		if err := rows.Scan(&hash, &salt); err != nil {
			return "", fmt.Errorf("failed to scan token: %w", err)
		}
		if hashToken(tokenValue, salt) == hash {
			foundHash = hash
			break
		}
	}

	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("error iterating tokens: %w", err)
	}

	return foundHash, nil
}

// List lists all tokens. The Token field will be empty (redacted) for security.
// Only the TokenPrefix is available for identification.
func (s *SQLiteTokenStore) List(ctx context.Context) ([]*JoinToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.QueryContext(ctx, `
		SELECT token_hash, salt, token_prefix, agent_id, expires_at, created_at,
			   used, used_at, used_by, metadata
		FROM join_tokens
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query tokens: %w", err)
	}
	defer rows.Close()

	var tokens []*JoinToken
	for rows.Next() {
		token, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tokens: %w", err)
	}

	return tokens, nil
}

// Cleanup removes expired and used tokens.
func (s *SQLiteTokenStore) Cleanup(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.ExecContext(ctx, `
		DELETE FROM join_tokens
		WHERE used = 1 OR expires_at < ?
	`, time.Now())
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup tokens: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get affected rows: %w", err)
	}

	return int(count), nil
}

// Count returns the total number of tokens.
func (s *SQLiteTokenStore) Count(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM join_tokens").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count tokens: %w", err)
	}
	return count, nil
}

// GetStats returns statistics about tokens.
func (s *SQLiteTokenStore) GetStats(ctx context.Context) (*TokenStoreStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stats := &TokenStoreStats{}

	// Count total
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM join_tokens").Scan(&stats.Total)
	if err != nil {
		return nil, fmt.Errorf("failed to count total: %w", err)
	}

	// Count used
	err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM join_tokens WHERE used = 1").Scan(&stats.Used)
	if err != nil {
		return nil, fmt.Errorf("failed to count used: %w", err)
	}

	// Count expired (but not used)
	err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM join_tokens WHERE used = 0 AND expires_at < ?", time.Now()).Scan(&stats.Expired)
	if err != nil {
		return nil, fmt.Errorf("failed to count expired: %w", err)
	}

	// Count valid (not used and not expired)
	stats.Valid = stats.Total - stats.Used - stats.Expired

	return stats, nil
}

// TokenStoreStats contains statistics about the token store.
type TokenStoreStats struct {
	Total   int
	Valid   int
	Used    int
	Expired int
}

// Helper functions

func scanToken(rows *sql.Rows) (*JoinToken, error) {
	var token JoinToken
	var agentID, usedBy sql.NullString
	var usedAt sql.NullTime
	var metadata []byte
	var usedInt int

	err := rows.Scan(
		&token.TokenHash, &token.Salt, &token.TokenPrefix, &agentID,
		&token.ExpiresAt, &token.CreatedAt, &usedInt, &usedAt, &usedBy, &metadata,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan token: %w", err)
	}

	token.AgentID = agentID.String
	token.Used = usedInt != 0
	if usedAt.Valid {
		token.UsedAt = usedAt.Time
	}
	token.UsedBy = usedBy.String

	if len(metadata) > 0 {
		if err := json.Unmarshal(metadata, &token.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &token, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t, Valid: true}
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullBytes(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return b
}

// Ensure SQLiteTokenStore implements JoinTokenStore
var _ JoinTokenStore = (*SQLiteTokenStore)(nil)
