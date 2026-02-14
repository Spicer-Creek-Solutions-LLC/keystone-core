package registry

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNewTrustStore_RequiresDir(t *testing.T) {
	_, err := NewTrustStore(TrustConfig{})
	if err == nil {
		t.Fatal("expected error for empty trust dir")
	}
}

func TestTrustStore_Init(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "trust")
	ts, err := NewTrustStore(TrustConfig{TrustDir: dir})
	if err != nil {
		t.Fatalf("NewTrustStore: %v", err)
	}

	if err := ts.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	roots := ts.ListRoots()
	if len(roots) != 0 {
		t.Errorf("expected 0 roots after init, got %d", len(roots))
	}
}

func TestTrustStore_AddRemoveList(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "trust")
	ts, err := NewTrustStore(TrustConfig{TrustDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := ts.Init(); err != nil {
		t.Fatal(err)
	}

	root := TrustRoot{
		Name:      "test-key",
		PublicKey: []byte("-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE...\n-----END PUBLIC KEY-----"),
		Algorithm: "ecdsa",
	}

	if err := ts.AddRoot(root); err != nil {
		t.Fatalf("AddRoot: %v", err)
	}

	roots := ts.ListRoots()
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	if roots[0].Name != "test-key" {
		t.Errorf("root name = %q, want test-key", roots[0].Name)
	}
	if roots[0].AddedAt.IsZero() {
		t.Error("AddedAt should be set")
	}

	// Remove
	if err := ts.RemoveRoot("test-key"); err != nil {
		t.Fatalf("RemoveRoot: %v", err)
	}
	if len(ts.ListRoots()) != 0 {
		t.Error("expected 0 roots after remove")
	}
}

func TestTrustStore_AddDuplicate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "trust")
	ts, err := NewTrustStore(TrustConfig{TrustDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := ts.Init(); err != nil {
		t.Fatal(err)
	}

	root := TrustRoot{Name: "key1", PublicKey: []byte("key-data"), Algorithm: "ed25519"}
	if err := ts.AddRoot(root); err != nil {
		t.Fatal(err)
	}

	err = ts.AddRoot(root)
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
}

func TestTrustStore_AddValidation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "trust")
	ts, err := NewTrustStore(TrustConfig{TrustDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := ts.Init(); err != nil {
		t.Fatal(err)
	}

	// Missing name
	err = ts.AddRoot(TrustRoot{PublicKey: []byte("key")})
	if err == nil {
		t.Error("expected error for empty name")
	}

	// Missing key
	err = ts.AddRoot(TrustRoot{Name: "key1"})
	if err == nil {
		t.Error("expected error for empty public key")
	}
}

func TestTrustStore_RemoveNotFound(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "trust")
	ts, err := NewTrustStore(TrustConfig{TrustDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := ts.Init(); err != nil {
		t.Fatal(err)
	}

	err = ts.RemoveRoot("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent root")
	}
}

func TestTrustStore_Persistence(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "trust")
	ts1, err := NewTrustStore(TrustConfig{TrustDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := ts1.Init(); err != nil {
		t.Fatal(err)
	}

	if err := ts1.AddRoot(TrustRoot{Name: "persisted", PublicKey: []byte("key"), Algorithm: "ed25519"}); err != nil {
		t.Fatal(err)
	}

	// Create a new TrustStore pointing to the same dir — should load existing roots
	ts2, err := NewTrustStore(TrustConfig{TrustDir: dir})
	if err != nil {
		t.Fatal(err)
	}

	roots := ts2.ListRoots()
	if len(roots) != 1 || roots[0].Name != "persisted" {
		t.Errorf("expected persisted root, got %v", roots)
	}
}

func TestTrustStore_ActiveRoots(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "trust")
	ts, err := NewTrustStore(TrustConfig{TrustDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := ts.Init(); err != nil {
		t.Fatal(err)
	}

	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	ts.AddRoot(TrustRoot{Name: "expired", PublicKey: []byte("k"), Algorithm: "ed25519", ExpiresAt: &past})
	ts.AddRoot(TrustRoot{Name: "valid", PublicKey: []byte("k"), Algorithm: "ed25519", ExpiresAt: &future})
	ts.AddRoot(TrustRoot{Name: "no-expiry", PublicKey: []byte("k"), Algorithm: "ed25519"})

	active := ts.ActiveRoots()
	if len(active) != 2 {
		t.Errorf("expected 2 active roots, got %d", len(active))
	}

	names := map[string]bool{}
	for _, r := range active {
		names[r.Name] = true
	}
	if !names["valid"] || !names["no-expiry"] {
		t.Errorf("unexpected active roots: %v", active)
	}
}

func TestTrustStore_RequireSignatures(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "trust")

	ts1, _ := NewTrustStore(TrustConfig{TrustDir: dir, RequireSignatures: true})
	if !ts1.RequireSignatures() {
		t.Error("expected RequireSignatures=true")
	}

	ts2, _ := NewTrustStore(TrustConfig{TrustDir: dir, RequireSignatures: false})
	if ts2.RequireSignatures() {
		t.Error("expected RequireSignatures=false")
	}
}
