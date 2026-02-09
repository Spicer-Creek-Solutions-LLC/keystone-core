package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
)

// generateTestKey creates a test SSH public key.
func generateTestKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("failed to create SSH public key: %v", err)
	}
	return publicKey
}

func TestNewHostKeyVerifier(t *testing.T) {
	tests := []struct {
		mode HostKeyCheckMode
	}{
		{HostKeyCheckStrict},
		{HostKeyCheckTOFU},
		{HostKeyCheckAcceptNew},
		{HostKeyCheckNo},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			v := NewHostKeyVerifier(tt.mode)
			if v == nil {
				t.Fatal("NewHostKeyVerifier returned nil")
			}
			if v.Mode != tt.mode {
				t.Errorf("Mode = %s, want %s", v.Mode, tt.mode)
			}
			if v.learnedKeys == nil {
				t.Error("learnedKeys map not initialized")
			}
		})
	}
}

func TestHostKeyVerifier_NoMode(t *testing.T) {
	v := NewHostKeyVerifier(HostKeyCheckNo)
	key := generateTestKey(t)
	addr := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 22}

	// Should accept any key
	err := v.Verify("example.com:22", addr, key)
	if err != nil {
		t.Errorf("HostKeyCheckNo should accept any key, got error: %v", err)
	}

	// Should accept even a different key for same host
	key2 := generateTestKey(t)
	err = v.Verify("example.com:22", addr, key2)
	if err != nil {
		t.Errorf("HostKeyCheckNo should accept different key, got error: %v", err)
	}
}

func TestHostKeyVerifier_StrictMode_UnknownHost(t *testing.T) {
	v := NewHostKeyVerifier(HostKeyCheckStrict)
	v.KnownHostsPath = "/nonexistent/path" // Ensure no known_hosts file
	key := generateTestKey(t)
	addr := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 22}

	err := v.Verify("unknown.example.com:22", addr, key)
	if err == nil {
		t.Error("HostKeyCheckStrict should reject unknown host")
	}

	var unknownErr *UnknownHostError
	if !errors.As(err, &unknownErr) {
		t.Errorf("Expected UnknownHostError, got %T", err)
	}
}

func TestHostKeyVerifier_TOFU_LearnKey(t *testing.T) {
	v := NewHostKeyVerifier(HostKeyCheckTOFU)
	v.StoreLearnedKeys = false // Don't try to persist
	key := generateTestKey(t)
	addr := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 22}

	// Track if OnNewKey was called
	var newKeyHost string
	v.OnNewKey = func(hostname string, k ssh.PublicKey) {
		newKeyHost = hostname
	}

	// First connection should learn the key
	err := v.Verify("tofu.example.com:22", addr, key)
	if err != nil {
		t.Errorf("TOFU mode should accept new key on first use, got error: %v", err)
	}

	if newKeyHost != "tofu.example.com" {
		t.Errorf("OnNewKey was not called or hostname wrong, got %s", newKeyHost)
	}

	// Same key should be accepted
	err = v.Verify("tofu.example.com:22", addr, key)
	if err != nil {
		t.Errorf("TOFU mode should accept same key, got error: %v", err)
	}

	// Different key should be rejected
	key2 := generateTestKey(t)
	err = v.Verify("tofu.example.com:22", addr, key2)
	if err == nil {
		t.Error("TOFU mode should reject different key for known host")
	}

	var mismatchErr *HostKeyMismatchError
	if !errors.As(err, &mismatchErr) {
		t.Errorf("Expected HostKeyMismatchError, got %T", err)
	}
}

func TestHostKeyVerifier_AcceptNew(t *testing.T) {
	v := NewHostKeyVerifier(HostKeyCheckAcceptNew)
	v.StoreLearnedKeys = false
	key := generateTestKey(t)
	addr := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 22}

	// Should accept new host
	err := v.Verify("new.example.com:22", addr, key)
	if err != nil {
		t.Errorf("AcceptNew mode should accept new key, got error: %v", err)
	}

	// Should reject changed key
	key2 := generateTestKey(t)
	err = v.Verify("new.example.com:22", addr, key2)
	if err == nil {
		t.Error("AcceptNew mode should reject changed key")
	}
}

func TestHostKeyVerifier_AddKnownHost(t *testing.T) {
	v := NewHostKeyVerifier(HostKeyCheckStrict)
	key := generateTestKey(t)
	addr := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 22}

	// Manually add a known host
	v.AddKnownHost("manual.example.com", key)

	// Should now accept that host
	err := v.Verify("manual.example.com:22", addr, key)
	if err != nil {
		t.Errorf("Should accept manually added host, got error: %v", err)
	}
}

func TestHostKeyVerifier_RemoveKnownHost(t *testing.T) {
	v := NewHostKeyVerifier(HostKeyCheckTOFU)
	v.StoreLearnedKeys = false
	key := generateTestKey(t)
	addr := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 22}

	// Learn a key
	err := v.Verify("remove.example.com:22", addr, key)
	if err != nil {
		t.Fatalf("Failed to learn key: %v", err)
	}

	// Remove it
	v.RemoveKnownHost("remove.example.com")

	// Now a different key should be accepted (re-learned)
	key2 := generateTestKey(t)
	err = v.Verify("remove.example.com:22", addr, key2)
	if err != nil {
		t.Errorf("After removal, should accept new key, got error: %v", err)
	}
}

func TestHostKeyVerifier_GetKnownHosts(t *testing.T) {
	v := NewHostKeyVerifier(HostKeyCheckTOFU)
	v.StoreLearnedKeys = false
	key := generateTestKey(t)
	addr := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 22}

	// Learn some keys
	_ = v.Verify("host1.example.com:22", addr, key)

	key2 := generateTestKey(t)
	_ = v.Verify("host2.example.com:22", addr, key2)

	hosts := v.GetKnownHosts()
	if len(hosts) != 2 {
		t.Errorf("GetKnownHosts returned %d hosts, want 2", len(hosts))
	}

	if _, ok := hosts["host1.example.com"]; !ok {
		t.Error("host1.example.com not in known hosts")
	}
	if _, ok := hosts["host2.example.com"]; !ok {
		t.Error("host2.example.com not in known hosts")
	}
}

func TestHostKeyVerifier_OnKeyMismatch(t *testing.T) {
	v := NewHostKeyVerifier(HostKeyCheckTOFU)
	v.StoreLearnedKeys = false
	key := generateTestKey(t)
	addr := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 22}

	// Track mismatch callback
	var mismatchHost string
	v.OnKeyMismatch = func(hostname string, expected, actual ssh.PublicKey) {
		mismatchHost = hostname
	}

	// Learn a key
	_ = v.Verify("mismatch.example.com:22", addr, key)

	// Try different key
	key2 := generateTestKey(t)
	_ = v.Verify("mismatch.example.com:22", addr, key2)

	if mismatchHost != "mismatch.example.com" {
		t.Errorf("OnKeyMismatch not called or wrong hostname, got %s", mismatchHost)
	}
}

func TestHostKeyVerifier_PersistKey(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "hostkey-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	knownHostsPath := filepath.Join(tmpDir, "known_hosts")

	v := NewHostKeyVerifier(HostKeyCheckTOFU)
	v.KnownHostsPath = knownHostsPath
	v.StoreLearnedKeys = true

	key := generateTestKey(t)
	addr := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 22}

	// Learn a key (should persist)
	err = v.Verify("persist.example.com:22", addr, key)
	if err != nil {
		t.Fatalf("Failed to verify/learn key: %v", err)
	}

	// Check file was created
	if _, err := os.Stat(knownHostsPath); os.IsNotExist(err) {
		t.Error("known_hosts file was not created")
	}

	// Read and verify content
	content, err := os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatalf("Failed to read known_hosts: %v", err)
	}

	if len(content) == 0 {
		t.Error("known_hosts file is empty")
	}

	// Create new verifier and verify it can read the persisted key
	v2 := NewHostKeyVerifier(HostKeyCheckStrict)
	v2.KnownHostsPath = knownHostsPath

	err = v2.Verify("persist.example.com:22", addr, key)
	if err != nil {
		t.Errorf("New verifier should accept persisted key, got error: %v", err)
	}
}

func TestNormalizeHostname(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"example.com", "example.com"},
		{"example.com:22", "example.com"},
		{"example.com:2222", "example.com:2222"},
		{"[::1]:22", "::1"},
		{"[::1]:2222", "[::1]:2222"},
		{"192.168.1.1", "192.168.1.1"},
		{"192.168.1.1:22", "192.168.1.1"},
		{"192.168.1.1:2222", "192.168.1.1:2222"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeHostname(tt.input)
			if got != tt.want {
				t.Errorf("normalizeHostname(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFingerprintSHA256(t *testing.T) {
	key := generateTestKey(t)
	fp := FingerprintSHA256(key)

	// Should start with SHA256:
	if len(fp) < 7 || fp[:7] != "SHA256:" {
		t.Errorf("Fingerprint should start with 'SHA256:', got %s", fp)
	}

	// Should be consistent
	fp2 := FingerprintSHA256(key)
	if fp != fp2 {
		t.Errorf("Fingerprint should be consistent, got %s and %s", fp, fp2)
	}

	// Different keys should have different fingerprints
	key2 := generateTestKey(t)
	fp3 := FingerprintSHA256(key2)
	if fp == fp3 {
		t.Error("Different keys should have different fingerprints")
	}
}

func TestHostKeyMismatchError(t *testing.T) {
	key1 := generateTestKey(t)
	key2 := generateTestKey(t)

	err := &HostKeyMismatchError{
		Hostname:    "test.example.com",
		ExpectedKey: key1,
		ActualKey:   key2,
	}

	msg := err.Error()
	if msg == "" {
		t.Error("Error message should not be empty")
	}
	if !contains(msg, "test.example.com") {
		t.Error("Error message should contain hostname")
	}
	if !contains(msg, "MITM") {
		t.Error("Error message should mention MITM attack")
	}
}

func TestUnknownHostError(t *testing.T) {
	key := generateTestKey(t)
	err := &UnknownHostError{
		Hostname:    "unknown.example.com",
		Fingerprint: FingerprintSHA256(key),
		KeyType:     key.Type(),
	}

	msg := err.Error()
	if msg == "" {
		t.Error("Error message should not be empty")
	}
	if !contains(msg, "unknown.example.com") {
		t.Error("Error message should contain hostname")
	}
}

func TestDefaultHostKeyVerifier(t *testing.T) {
	v := DefaultHostKeyVerifier()
	if v == nil {
		t.Fatal("DefaultHostKeyVerifier returned nil")
	}
	if v.Mode != HostKeyCheckTOFU {
		t.Errorf("Default mode should be TOFU, got %s", v.Mode)
	}
}

func TestKeysEqual(t *testing.T) {
	key1 := generateTestKey(t)
	key2 := generateTestKey(t)

	if !keysEqual(key1, key1) {
		t.Error("Same key should be equal to itself")
	}
	if keysEqual(key1, key2) {
		t.Error("Different keys should not be equal")
	}
}

// contains checks if a string contains a substring (helper for tests)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
