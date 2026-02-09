package statemgmt

import (
	"context"
	"os/exec"
	"testing"
)

// ============================================================================
// PostgreSQL Database Module Tests
// ============================================================================

func TestNewPostgresDatabaseModule(t *testing.T) {
	m := NewPostgresDatabaseModule()

	if m.Name() != "postgres_database" {
		t.Errorf("expected name 'postgres_database', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
	for i, s := range expected {
		if states[i] != s {
			t.Errorf("expected state[%d] = '%s', got '%s'", i, s, states[i])
		}
	}
}

func TestPostgresDatabaseModule_Check_MissingName(t *testing.T) {
	m := NewPostgresDatabaseModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "postgres_database",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil || err.Error() != "name parameter is required" {
		t.Errorf("expected name required error, got: %v", err)
	}
}

func TestPostgresDatabaseModule_Check_NoPsql(t *testing.T) {
	// Skip if psql is actually available
	if _, err := exec.LookPath("psql"); err == nil {
		t.Skip("psql is available, skipping")
	}

	m := NewPostgresDatabaseModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "postgres_database",
		State:  "present",
		Parameters: map[string]interface{}{
			"name": "testdb",
		},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil {
		t.Error("expected psql not available error")
	}
}

// ============================================================================
// PostgreSQL User Module Tests
// ============================================================================

func TestNewPostgresUserModule(t *testing.T) {
	m := NewPostgresUserModule()

	if m.Name() != "postgres_user" {
		t.Errorf("expected name 'postgres_user', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestPostgresUserModule_Check_MissingName(t *testing.T) {
	m := NewPostgresUserModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "postgres_user",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil || err.Error() != "name parameter is required" {
		t.Errorf("expected name required error, got: %v", err)
	}
}

func TestPostgresUserModule_Check_NoPsql(t *testing.T) {
	if _, err := exec.LookPath("psql"); err == nil {
		t.Skip("psql is available, skipping")
	}

	m := NewPostgresUserModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "postgres_user",
		State:  "present",
		Parameters: map[string]interface{}{
			"name": "testuser",
		},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil {
		t.Error("expected psql not available error")
	}
}

// ============================================================================
// PostgreSQL Extension Module Tests
// ============================================================================

func TestNewPostgresExtensionModule(t *testing.T) {
	m := NewPostgresExtensionModule()

	if m.Name() != "postgres_extension" {
		t.Errorf("expected name 'postgres_extension', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestPostgresExtensionModule_Check_MissingName(t *testing.T) {
	m := NewPostgresExtensionModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "postgres_extension",
		State:  "present",
		Parameters: map[string]interface{}{
			"database": "testdb",
		},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil || err.Error() != "name parameter is required" {
		t.Errorf("expected name required error, got: %v", err)
	}
}

func TestPostgresExtensionModule_Check_MissingDatabase(t *testing.T) {
	m := NewPostgresExtensionModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "postgres_extension",
		State:  "present",
		Parameters: map[string]interface{}{
			"name": "pg_stat_statements",
		},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil || err.Error() != "database parameter is required" {
		t.Errorf("expected database required error, got: %v", err)
	}
}

// ============================================================================
// MySQL Database Module Tests
// ============================================================================

func TestNewMySQLDatabaseModule(t *testing.T) {
	m := NewMySQLDatabaseModule()

	if m.Name() != "mysql_database" {
		t.Errorf("expected name 'mysql_database', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestMySQLDatabaseModule_Check_MissingName(t *testing.T) {
	m := NewMySQLDatabaseModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "mysql_database",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil || err.Error() != "name parameter is required" {
		t.Errorf("expected name required error, got: %v", err)
	}
}

func TestMySQLDatabaseModule_Check_NoMysql(t *testing.T) {
	if _, err := exec.LookPath("mysql"); err == nil {
		t.Skip("mysql is available, skipping")
	}

	m := NewMySQLDatabaseModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "mysql_database",
		State:  "present",
		Parameters: map[string]interface{}{
			"name": "testdb",
		},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil {
		t.Error("expected mysql not available error")
	}
}

// ============================================================================
// MySQL User Module Tests
// ============================================================================

func TestNewMySQLUserModule(t *testing.T) {
	m := NewMySQLUserModule()

	if m.Name() != "mysql_user" {
		t.Errorf("expected name 'mysql_user', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestMySQLUserModule_Check_MissingName(t *testing.T) {
	m := NewMySQLUserModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "mysql_user",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil || err.Error() != "name parameter is required" {
		t.Errorf("expected name required error, got: %v", err)
	}
}

func TestMySQLUserModule_BuildGrantSQL(t *testing.T) {
	m := NewMySQLUserModule()

	tests := []struct {
		name     string
		user     string
		host     string
		priv     string
		expected string
	}{
		{
			name:     "all privileges",
			user:     "testuser",
			host:     "localhost",
			priv:     "testdb.*:ALL",
			expected: "GRANT ALL ON testdb.* TO 'testuser'@'localhost'",
		},
		{
			name:     "select only",
			user:     "readonly",
			host:     "%",
			priv:     "mydb.mytable:SELECT",
			expected: "GRANT SELECT ON mydb.mytable TO 'readonly'@'%'",
		},
		{
			name:     "multiple privileges",
			user:     "appuser",
			host:     "192.168.1.%",
			priv:     "appdb.*:SELECT,INSERT,UPDATE",
			expected: "GRANT SELECT,INSERT,UPDATE ON appdb.* TO 'appuser'@'192.168.1.%'",
		},
		{
			name:     "invalid format",
			user:     "user",
			host:     "host",
			priv:     "invalid",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := m.buildGrantSQL(tt.user, tt.host, tt.priv)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// ============================================================================
// Redis Module Tests
// ============================================================================

func TestNewRedisModule(t *testing.T) {
	m := NewRedisModule()

	if m.Name() != "redis" {
		t.Errorf("expected name 'redis', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestRedisModule_Check_MissingName(t *testing.T) {
	m := NewRedisModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "redis",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil || err.Error() != "name parameter is required (config key or user)" {
		t.Errorf("expected name required error, got: %v", err)
	}
}

func TestRedisModule_Check_NoRedisCli(t *testing.T) {
	if _, err := exec.LookPath("redis-cli"); err == nil {
		t.Skip("redis-cli is available, skipping")
	}

	m := NewRedisModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "redis",
		State:  "present",
		Parameters: map[string]interface{}{
			"name": "maxmemory",
			"type": "config",
		},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil {
		t.Error("expected redis-cli not available error")
	}
}

// ============================================================================
// Helper Function Tests
// ============================================================================

func TestEscapePostgresString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with'quote", "with''quote"},
		{"multiple'single'quotes", "multiple''single''quotes"},
		{"", ""},
	}

	for _, tt := range tests {
		result := escapePostgresString(tt.input)
		if result != tt.expected {
			t.Errorf("escapePostgresString(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestQuotePostgresIdentifier(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple_name", "simple_name"},
		{"CamelCase", "CamelCase"},
		{"with space", `"with space"`},
		{"with-dash", `"with-dash"`},
		{"with.dot", `"with.dot"`},
		{`with"quote`, `"with""quote"`},
	}

	for _, tt := range tests {
		result := quotePostgresIdentifier(tt.input)
		if result != tt.expected {
			t.Errorf("quotePostgresIdentifier(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestEscapeMySQLString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with'quote", `with\'quote`},
		{`with\backslash`, `with\\backslash`},
		{`mixed\and'both`, `mixed\\and\'both`},
		{"", ""},
	}

	for _, tt := range tests {
		result := escapeMySQLString(tt.input)
		if result != tt.expected {
			t.Errorf("escapeMySQLString(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestEscapeMySQLIdentifier(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple_name", "simple_name"},
		{"with`backtick", "with``backtick"},
		{"multiple`back`ticks", "multiple``back``ticks"},
		{"", ""},
	}

	for _, tt := range tests {
		result := escapeMySQLIdentifier(tt.input)
		if result != tt.expected {
			t.Errorf("escapeMySQLIdentifier(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

// ============================================================================
// Integration Tests (require databases)
// ============================================================================

func TestPostgresDatabaseModule_Integration(t *testing.T) {
	// Check if psql is available
	cmd := exec.CommandContext(context.Background(),"psql", "--version")
	if err := cmd.Run(); err != nil {
		t.Skip("psql is not available")
	}

	// This would require a running PostgreSQL server
	// For now, just test that the module can be instantiated
	m := NewPostgresDatabaseModule()
	if m == nil {
		t.Error("expected module to be created")
	}
}

func TestMySQLDatabaseModule_Integration(t *testing.T) {
	// Check if mysql is available
	cmd := exec.CommandContext(context.Background(),"mysql", "--version")
	if err := cmd.Run(); err != nil {
		t.Skip("mysql is not available")
	}

	// This would require a running MySQL server
	// For now, just test that the module can be instantiated
	m := NewMySQLDatabaseModule()
	if m == nil {
		t.Error("expected module to be created")
	}
}

func TestRedisModule_Integration(t *testing.T) {
	// Check if redis-cli is available
	cmd := exec.CommandContext(context.Background(),"redis-cli", "--version")
	if err := cmd.Run(); err != nil {
		t.Skip("redis-cli is not available")
	}

	// This would require a running Redis server
	// For now, just test that the module can be instantiated
	m := NewRedisModule()
	if m == nil {
		t.Error("expected module to be created")
	}
}

// ============================================================================
// Connection Args Tests
// ============================================================================

func TestPostgresDatabaseModule_BuildConnArgs(t *testing.T) {
	m := NewPostgresDatabaseModule()

	args := m.buildConnArgs("myhost", 5433, "myuser", "", "mydb")
	expected := []string{"-h", "myhost", "-p", "5433", "-U", "myuser", "-d", "mydb"}

	if len(args) != len(expected) {
		t.Errorf("expected %d args, got %d", len(expected), len(args))
	}
	for i, e := range expected {
		if i < len(args) && args[i] != e {
			t.Errorf("arg[%d] = %s, expected %s", i, args[i], e)
		}
	}
}

func TestMySQLDatabaseModule_BuildConnArgs(t *testing.T) {
	m := NewMySQLDatabaseModule()

	// Test TCP connection with password
	decl := &StateDeclaration{
		Parameters: map[string]interface{}{
			"host":     "myhost",
			"port":     3307,
			"user":     "myuser",
			"password": "secret",
		},
	}

	args := m.buildConnArgs(decl)
	// -h myhost -P 3307 -u myuser -psecret = 7 args
	if len(args) != 7 {
		t.Errorf("expected 7 args for TCP with password, got %d: %v", len(args), args)
	}

	// Test socket connection with password
	decl.Parameters["socket"] = "/var/run/mysqld/mysqld.sock"
	args = m.buildConnArgs(decl)
	// -S socket -u myuser -psecret = 5 args
	if len(args) != 5 {
		t.Errorf("expected 5 args for socket with password, got %d: %v", len(args), args)
	}
	if args[0] != "-S" {
		t.Errorf("expected socket flag -S, got %s", args[0])
	}

	// Test TCP without password
	decl2 := &StateDeclaration{
		Parameters: map[string]interface{}{
			"host": "myhost",
			"port": 3307,
			"user": "myuser",
		},
	}
	args = m.buildConnArgs(decl2)
	// -h myhost -P 3307 -u myuser = 6 args
	if len(args) != 6 {
		t.Errorf("expected 6 args for TCP without password, got %d: %v", len(args), args)
	}
}

func TestRedisModule_BuildConnArgs(t *testing.T) {
	m := NewRedisModule()

	// Test TCP connection
	decl := &StateDeclaration{
		Parameters: map[string]interface{}{
			"host":     "myhost",
			"port":     6380,
			"password": "secret",
		},
	}

	args := m.buildConnArgs(decl)
	if len(args) != 6 {
		t.Errorf("expected 6 args for TCP with password, got %d: %v", len(args), args)
	}

	// Test socket connection
	decl.Parameters["socket"] = "/var/run/redis/redis.sock"
	args = m.buildConnArgs(decl)
	if len(args) != 4 {
		t.Errorf("expected 4 args for socket with password, got %d: %v", len(args), args)
	}
	if args[0] != "-s" {
		t.Errorf("expected socket flag -s, got %s", args[0])
	}
}
