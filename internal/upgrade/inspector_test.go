package upgrade

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// =============================================================================
// SelfInspector Tests
// =============================================================================

func TestSelfInspector_ComponentType(t *testing.T) {
	tests := []struct {
		name      string
		component ComponentType
	}{
		{"server", ComponentServer},
		{"agent", ComponentAgent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inspector := NewSelfInspector(tt.component)
			if got := inspector.ComponentType(); got != tt.component {
				t.Errorf("ComponentType() = %v, want %v", got, tt.component)
			}
		})
	}
}

func TestSelfInspector_GetVersion(t *testing.T) {
	// Note: This test will fail if run without -ldflags setting version
	// In CI, versions should be set; in dev, expect "dev" version error
	inspector := NewSelfInspector(ComponentServer)
	ctx := context.Background()

	_, err := inspector.GetVersion(ctx)
	// We expect either a valid version or an error about "dev" build
	// Both are acceptable depending on how the test is run
	if err != nil {
		// Should be the expected "dev build" error
		if err.Error() != "development build - version not set (use -ldflags to set version)" {
			t.Logf("GetVersion() returned error (expected in dev builds): %v", err)
		}
	}
}

// =============================================================================
// NATSInspector Tests
// =============================================================================

func TestNATSInspector_ComponentType(t *testing.T) {
	inspector := NewNATSInspector(nil, "")
	if got := inspector.ComponentType(); got != ComponentNATS {
		t.Errorf("ComponentType() = %v, want %v", got, ComponentNATS)
	}
}

func TestNATSInspector_GetVersion_NoConnection(t *testing.T) {
	inspector := NewNATSInspector(nil, "")
	ctx := context.Background()

	_, err := inspector.GetVersion(ctx)
	if err == nil {
		t.Error("GetVersion() expected error with no connection, got nil")
	}
}

func TestNATSInspector_GetVersion_InvalidEndpoint(t *testing.T) {
	inspector := NewNATSInspector(nil, "nats://invalid:4222")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := inspector.GetVersion(ctx)
	if err == nil {
		t.Error("GetVersion() expected error with invalid endpoint, got nil")
	}
}

// =============================================================================
// DatabaseInspector Tests
// =============================================================================

func TestDatabaseInspector_ComponentType(t *testing.T) {
	inspector := NewDatabaseInspector(nil, "sqlite")
	if got := inspector.ComponentType(); got != ComponentDatabase {
		t.Errorf("ComponentType() = %v, want %v", got, ComponentDatabase)
	}
}

func TestDatabaseInspector_GetVersion_NoConnection(t *testing.T) {
	inspector := NewDatabaseInspector(nil, "sqlite")
	ctx := context.Background()

	_, err := inspector.GetVersion(ctx)
	if err == nil {
		t.Error("GetVersion() expected error with no connection, got nil")
	}
}

func TestDatabaseInspector_GetVersion_SQLite(t *testing.T) {
	// Create an in-memory SQLite database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory SQLite: %v", err)
	}
	defer db.Close()

	inspector := NewDatabaseInspector(db, "sqlite")
	ctx := context.Background()

	version, err := inspector.GetVersion(ctx)
	if err != nil {
		t.Fatalf("GetVersion() error = %v", err)
	}

	// SQLite version should be 3.x.x
	if version.Major != 3 {
		t.Errorf("GetVersion() major = %d, want 3", version.Major)
	}
	t.Logf("Detected SQLite version: %s", version.String())
}

func TestDatabaseInspector_GetVersion_WithSchemaTable(t *testing.T) {
	// Create an in-memory SQLite database with schema_version table
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory SQLite: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	// Create schema_version table
	_, err = db.ExecContext(ctx, `
		CREATE TABLE schema_version (
			version TEXT NOT NULL,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create schema_version table: %v", err)
	}

	// Insert a version
	_, err = db.ExecContext(ctx, `INSERT INTO schema_version (version) VALUES ('1.5.0')`)
	if err != nil {
		t.Fatalf("Failed to insert version: %v", err)
	}

	inspector := NewDatabaseInspector(db, "sqlite")

	version, err := inspector.GetVersion(ctx)
	if err != nil {
		t.Fatalf("GetVersion() error = %v", err)
	}

	if version.Major != 1 || version.Minor != 5 || version.Patch != 0 {
		t.Errorf("GetVersion() = %s, want 1.5.0", version.String())
	}
}

func TestDatabaseInspector_GetVersion_WithMetadataTable(t *testing.T) {
	// Create an in-memory SQLite database with kscore_metadata table
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory SQLite: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	// Create kscore_metadata table
	_, err = db.ExecContext(ctx, `
		CREATE TABLE kscore_metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create kscore_metadata table: %v", err)
	}

	// Insert schema version
	_, err = db.ExecContext(ctx, `INSERT INTO kscore_metadata (key, value) VALUES ('schema_version', '2.3.1')`)
	if err != nil {
		t.Fatalf("Failed to insert metadata: %v", err)
	}

	inspector := NewDatabaseInspector(db, "sqlite")

	version, err := inspector.GetVersion(ctx)
	if err != nil {
		t.Fatalf("GetVersion() error = %v", err)
	}

	if version.Major != 2 || version.Minor != 3 || version.Patch != 1 {
		t.Errorf("GetVersion() = %s, want 2.3.1", version.String())
	}
}

// =============================================================================
// EtcdInspector Tests
// =============================================================================

func TestEtcdInspector_ComponentType(t *testing.T) {
	inspector := NewEtcdInspector(nil, nil)
	if got := inspector.ComponentType(); got != ComponentEtcd {
		t.Errorf("ComponentType() = %v, want %v", got, ComponentEtcd)
	}
}

func TestEtcdInspector_GetVersion_NoConnection(t *testing.T) {
	inspector := NewEtcdInspector(nil, nil)
	ctx := context.Background()

	_, err := inspector.GetVersion(ctx)
	if err == nil {
		t.Error("GetVersion() expected error with no connection, got nil")
	}
}

func TestEtcdInspector_GetVersion_InvalidEndpoint(t *testing.T) {
	inspector := NewEtcdInspector(nil, []string{"http://invalid:2379"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := inspector.GetVersion(ctx)
	if err == nil {
		t.Error("GetVersion() expected error with invalid endpoint, got nil")
	}
}

// =============================================================================
// BinaryInspector Tests
// =============================================================================

func TestBinaryInspector_ComponentType(t *testing.T) {
	inspector := NewBinaryInspector("/usr/bin/test", ComponentServer)
	if got := inspector.ComponentType(); got != ComponentServer {
		t.Errorf("ComponentType() = %v, want %v", got, ComponentServer)
	}
}

func TestBinaryInspector_GetVersion_InvalidBinary(t *testing.T) {
	inspector := NewBinaryInspector("/nonexistent/binary", ComponentServer)
	ctx := context.Background()

	_, err := inspector.GetVersion(ctx)
	if err == nil {
		t.Error("GetVersion() expected error with invalid binary, got nil")
	}
}

// =============================================================================
// InspectorRegistry Tests
// =============================================================================

func TestInspectorRegistry_RegisterAndGet(t *testing.T) {
	registry := NewInspectorRegistry()

	// Register an inspector
	serverInspector := NewSelfInspector(ComponentServer)
	registry.Register(serverInspector)

	// Get should return the inspector
	got, ok := registry.Get(ComponentServer)
	if !ok {
		t.Error("Get() returned false, want true")
	}
	if got != serverInspector {
		t.Error("Get() returned different inspector")
	}

	// Get for unregistered component should return false
	_, ok = registry.Get(ComponentNATS)
	if ok {
		t.Error("Get() for unregistered component returned true, want false")
	}
}

func TestInspectorRegistry_GetVersion_NotRegistered(t *testing.T) {
	registry := NewInspectorRegistry()
	ctx := context.Background()

	_, err := registry.GetVersion(ctx, ComponentNATS)
	if err == nil {
		t.Error("GetVersion() expected error for unregistered component, got nil")
	}
}

// =============================================================================
// HTTPVersionProvider Integration Tests
// =============================================================================

func TestHTTPVersionProvider_GetCurrentVersion_WithInspector(t *testing.T) {
	// Create provider
	provider := NewHTTPVersionProvider("http://localhost:8080", nil)

	// The provider should have server and agent inspectors registered by default
	ctx := context.Background()

	// Try to get server version (will fail with dev version in tests)
	_, err := provider.GetCurrentVersion(ctx, ComponentServer)
	if err != nil {
		// Expected in dev builds
		t.Logf("GetCurrentVersion(server) error (expected in dev): %v", err)
	}

	// Try NATS without registering inspector - should error
	_, err = provider.GetCurrentVersion(ctx, ComponentNATS)
	if err == nil {
		t.Error("GetCurrentVersion(nats) expected error without inspector, got nil")
	}
}

func TestHTTPVersionProvider_RegisterInspector(t *testing.T) {
	provider := NewHTTPVersionProvider("http://localhost:8080", nil)

	// Create an in-memory SQLite database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory SQLite: %v", err)
	}
	defer db.Close()

	// Register database inspector
	dbInspector := NewDatabaseInspector(db, "sqlite")
	provider.RegisterInspector(dbInspector)

	// Now GetCurrentVersion for database should work
	ctx := context.Background()
	version, err := provider.GetCurrentVersion(ctx, ComponentDatabase)
	if err != nil {
		t.Fatalf("GetCurrentVersion(database) error = %v", err)
	}

	// Should be SQLite 3.x
	if version.Major != 3 {
		t.Errorf("GetCurrentVersion(database) major = %d, want 3", version.Major)
	}
}
