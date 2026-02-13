package token

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// SQLiteStoreConfig configures the SQLite token store.
type SQLiteStoreConfig struct {
	Path        string
	WALMode     bool
	BusyTimeout int
}

// SQLiteStore implements Store using SQLite.
type SQLiteStore struct {
	db *sql.DB
	mu sync.Mutex
}

// NewSQLiteStore creates a new SQLite-backed token store.
func NewSQLiteStore(config *SQLiteStoreConfig) (*SQLiteStore, error) {
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

	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	store := &SQLiteStore{db: db}
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

func (s *SQLiteStore) initSchema() error {
	schema := `
		CREATE TABLE IF NOT EXISTS cluster_join_tokens (
			id TEXT PRIMARY KEY,
			token_hash TEXT NOT NULL,
			salt TEXT NOT NULL,
			label TEXT,
			created_at DATETIME NOT NULL,
			expires_at DATETIME NOT NULL,
			max_uses INTEGER NOT NULL DEFAULT 0,
			used_count INTEGER NOT NULL DEFAULT 0,
			created_by TEXT,
			revoked INTEGER NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_cjt_expires ON cluster_join_tokens(expires_at);
		CREATE INDEX IF NOT EXISTS idx_cjt_token_hash ON cluster_join_tokens(token_hash);
	`
	_, err := s.db.ExecContext(context.Background(), schema)
	return err
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Create persists a new join token.
func (s *SQLiteStore) Create(ctx context.Context, token *JoinToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO cluster_join_tokens (
			id, token_hash, salt, label, created_at, expires_at,
			max_uses, used_count, created_by, revoked
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		token.ID, token.TokenHash, token.Salt,
		nullString(token.Label),
		token.CreatedAt, token.ExpiresAt,
		token.MaxUses, token.UsedCount,
		nullString(token.CreatedBy),
		boolToInt(token.Revoked),
	)
	if err != nil {
		return fmt.Errorf("failed to insert token: %w", err)
	}
	return nil
}

// GetByID retrieves a token by its unique ID.
func (s *SQLiteStore) GetByID(ctx context.Context, id string) (*JoinToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, token_hash, salt, label, created_at, expires_at,
			   max_uses, used_count, created_by, revoked
		FROM cluster_join_tokens WHERE id = ?`, id)

	token, err := scanTokenRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTokenNotFound
		}
		return nil, fmt.Errorf("failed to get token: %w", err)
	}
	return token, nil
}

// Lookup finds a token by hashing the raw value against each stored salt.
func (s *SQLiteStore) Lookup(ctx context.Context, rawToken string) (*JoinToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, token_hash, salt, label, created_at, expires_at,
			   max_uses, used_count, created_by, revoked
		FROM cluster_join_tokens
		WHERE revoked = 0 AND expires_at > ?`, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to query tokens: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		token, err := scanTokenRows(rows)
		if err != nil {
			return nil, err
		}
		if HashToken(rawToken, token.Salt) == token.TokenHash {
			return token, nil
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tokens: %w", err)
	}
	return nil, ErrTokenNotFound
}

// List returns all stored tokens.
func (s *SQLiteStore) List(ctx context.Context) ([]*JoinToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, token_hash, salt, label, created_at, expires_at,
			   max_uses, used_count, created_by, revoked
		FROM cluster_join_tokens ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to query tokens: %w", err)
	}
	defer rows.Close()

	var tokens []*JoinToken
	for rows.Next() {
		token, err := scanTokenRows(rows)
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

// Revoke marks a token as revoked.
func (s *SQLiteStore) Revoke(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.ExecContext(ctx,
		"UPDATE cluster_join_tokens SET revoked = 1 WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}
	if n == 0 {
		return ErrTokenNotFound
	}
	return nil
}

// IncrementUses atomically increments UsedCount if the token is still valid.
func (s *SQLiteStore) IncrementUses(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	result, err := s.db.ExecContext(ctx, `
		UPDATE cluster_join_tokens
		SET used_count = used_count + 1
		WHERE id = ? AND revoked = 0 AND expires_at > ? AND (max_uses = 0 OR used_count < max_uses)`,
		id, now)
	if err != nil {
		return fmt.Errorf("failed to increment uses: %w", err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}
	if n > 0 {
		return nil
	}

	// Determine why the update failed.
	row := s.db.QueryRowContext(ctx,
		"SELECT revoked, expires_at, max_uses, used_count FROM cluster_join_tokens WHERE id = ?", id)

	var revoked int
	var expiresAt time.Time
	var maxUses, usedCount int
	if err := row.Scan(&revoked, &expiresAt, &maxUses, &usedCount); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTokenNotFound
		}
		return fmt.Errorf("failed to query token: %w", err)
	}

	if revoked != 0 {
		return ErrTokenRevoked
	}
	if now.After(expiresAt) {
		return ErrTokenExpired
	}
	if maxUses > 0 && usedCount >= maxUses {
		return ErrTokenExhausted
	}
	return fmt.Errorf("failed to increment uses for unknown reason")
}

// DeleteExpired removes expired and revoked tokens.
func (s *SQLiteStore) DeleteExpired(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.ExecContext(ctx, `
		DELETE FROM cluster_join_tokens
		WHERE revoked = 1 OR expires_at < ?`, time.Now())
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired tokens: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get affected rows: %w", err)
	}
	return int(n), nil
}

// scanTokenRow scans a single row from QueryRow.
func scanTokenRow(row *sql.Row) (*JoinToken, error) {
	var t JoinToken
	var label, createdBy sql.NullString
	var revoked int

	err := row.Scan(
		&t.ID, &t.TokenHash, &t.Salt, &label,
		&t.CreatedAt, &t.ExpiresAt,
		&t.MaxUses, &t.UsedCount, &createdBy, &revoked,
	)
	if err != nil {
		return nil, err
	}

	t.Label = label.String
	t.CreatedBy = createdBy.String
	t.Revoked = revoked != 0
	return &t, nil
}

// scanTokenRows scans a row from Query results.
func scanTokenRows(rows *sql.Rows) (*JoinToken, error) {
	var t JoinToken
	var label, createdBy sql.NullString
	var revoked int

	err := rows.Scan(
		&t.ID, &t.TokenHash, &t.Salt, &label,
		&t.CreatedAt, &t.ExpiresAt,
		&t.MaxUses, &t.UsedCount, &createdBy, &revoked,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan token: %w", err)
	}

	t.Label = label.String
	t.CreatedBy = createdBy.String
	t.Revoked = revoked != 0
	return &t, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

var _ Store = (*SQLiteStore)(nil)
