package capabilities

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// Mock implementations

type mockSecretsStore struct {
	data map[string]string
}

func newMockSecretsStore() *mockSecretsStore {
	return &mockSecretsStore{
		data: make(map[string]string),
	}
}

func (m *mockSecretsStore) Get(path string) (string, error) {
	value, exists := m.data[path]
	if !exists {
		return "", fmt.Errorf("secret not found")
	}
	return value, nil
}

func (m *mockSecretsStore) Set(path, value string) error {
	m.data[path] = value
	return nil
}

func (m *mockSecretsStore) Delete(path string) error {
	delete(m.data, path)
	return nil
}

type mockLogger struct {
	entries []logEntry
}

type logEntry struct {
	level   string
	message string
	fields  map[string]interface{}
}

func (m *mockLogger) Log(level, message string, fields map[string]interface{}) {
	m.entries = append(m.entries, logEntry{
		level:   level,
		message: message,
		fields:  fields,
	})
}

type mockKVStore struct {
	data map[string]string
}

func newMockKVStore() *mockKVStore {
	return &mockKVStore{
		data: make(map[string]string),
	}
}

func (m *mockKVStore) Get(key string) (string, error) {
	value, exists := m.data[key]
	if !exists {
		return "", fmt.Errorf("key not found")
	}
	return value, nil
}

func (m *mockKVStore) Set(key, value string) error {
	m.data[key] = value
	return nil
}

func (m *mockKVStore) Delete(key string) error {
	delete(m.data, key)
	return nil
}

func (m *mockKVStore) List(prefix string) ([]string, error) {
	var keys []string
	for key := range m.data {
		if prefix == "" || len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

// Tests for SecretsReadCapability

func TestSecretsReadCapability_Validate(t *testing.T) {
	tests := []struct {
		name        string
		cap         *SecretsReadCapability
		expectError bool
	}{
		{
			name: "valid capability",
			cap: &SecretsReadCapability{
				AllowedPaths: []string{"app/*"},
			},
			expectError: false,
		},
		{
			name: "no allowed paths",
			cap: &SecretsReadCapability{
				AllowedPaths: []string{},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cap.Validate()
			if tt.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestSecretsReadCapability_ReadSecret(t *testing.T) {
	store := newMockSecretsStore()
	store.Set("app/db/password", "secret123")

	testCap := &SecretsReadCapability{
		AllowedPaths: []string{"app/*"},
	}
	testCap.SetStore(store)

	ctx := NewCapabilityContext(context.Background(), "test-module")

	// Test reading allowed secret
	value, err := testCap.ReadSecret(ctx, "app/db/password")
	if err != nil {
		t.Fatalf("failed to read secret: %v", err)
	}

	if value != "secret123" {
		t.Errorf("expected value 'secret123', got %q", value)
	}

	// Test reading not allowed secret
	_, err = testCap.ReadSecret(ctx, "other/secret")
	if !errors.Is(err, ErrPathNotAllowed) {
		t.Errorf("expected ErrPathNotAllowed, got %v", err)
	}
}

func TestSecretsWriteCapability_WriteSecret(t *testing.T) {
	store := newMockSecretsStore()

	testCap := &SecretsWriteCapability{
		AllowedPaths: []string{"app/*"},
	}
	testCap.SetStore(store)

	ctx := NewCapabilityContext(context.Background(), "test-module")

	// Test writing allowed secret
	err := testCap.WriteSecret(ctx, "app/api/key", "apikey123")
	if err != nil {
		t.Fatalf("failed to write secret: %v", err)
	}

	// Verify it was written
	value, _ := store.Get("app/api/key")
	if value != "apikey123" {
		t.Errorf("expected value 'apikey123', got %q", value)
	}

	// Test writing not allowed secret
	err = testCap.WriteSecret(ctx, "other/secret", "value")
	if !errors.Is(err, ErrPathNotAllowed) {
		t.Errorf("expected ErrPathNotAllowed, got %v", err)
	}
}

func TestSecretsWriteCapability_DeleteSecret(t *testing.T) {
	store := newMockSecretsStore()
	store.Set("app/temp", "value")

	testCap := &SecretsWriteCapability{
		AllowedPaths: []string{"app/*"},
	}
	testCap.SetStore(store)

	ctx := NewCapabilityContext(context.Background(), "test-module")

	// Test deleting allowed secret
	err := testCap.DeleteSecret(ctx, "app/temp")
	if err != nil {
		t.Fatalf("failed to delete secret: %v", err)
	}

	// Verify it was deleted
	_, err = store.Get("app/temp")
	if err == nil {
		t.Error("secret still exists after deletion")
	}
}

// Tests for LogCapability

func TestLogCapability_Validate(t *testing.T) {
	tests := []struct {
		name        string
		cap         *LogCapability
		expectError bool
	}{
		{
			name:        "valid capability",
			cap:         &LogCapability{},
			expectError: false,
		},
		{
			name: "valid with rate limit",
			cap: &LogCapability{
				RateLimit: &RateLimit{
					Requests: 10,
					Period:   time.Minute,
				},
			},
			expectError: false,
		},
		{
			name: "invalid rate limit",
			cap: &LogCapability{
				RateLimit: &RateLimit{
					Requests: 0,
					Period:   time.Minute,
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cap.Validate()
			if tt.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestLogCapability_Log(t *testing.T) {
	logger := &mockLogger{}

	testCap := &LogCapability{}
	testCap.SetLogger(logger)

	ctx := NewCapabilityContext(context.Background(), "test-module")

	// Test logging
	err := testCap.Log(ctx, "info", "test message", map[string]interface{}{
		"key": "value",
	})
	if err != nil {
		t.Fatalf("failed to log: %v", err)
	}

	// Check log entry
	if len(logger.entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logger.entries))
	}

	entry := logger.entries[0]
	if entry.level != "info" {
		t.Errorf("expected level 'info', got %q", entry.level)
	}
	if entry.message != "test message" {
		t.Errorf("expected message 'test message', got %q", entry.message)
	}
	if entry.fields["module"] != "test-module" {
		t.Errorf("expected module 'test-module', got %v", entry.fields["module"])
	}
}

func TestLogCapability_LogRateLimit(t *testing.T) {
	logger := &mockLogger{}

	testCap := &LogCapability{
		RateLimit: &RateLimit{
			Requests: 2,
			Period:   time.Minute,
		},
	}
	testCap.SetLogger(logger)

	ctx := NewCapabilityContext(context.Background(), "test-module")

	// First two logs should succeed
	err := testCap.Log(ctx, "info", "message 1", nil)
	if err != nil {
		t.Fatalf("first log failed: %v", err)
	}

	err = testCap.Log(ctx, "info", "message 2", nil)
	if err != nil {
		t.Fatalf("second log failed: %v", err)
	}

	// Third log should fail due to rate limit
	err = testCap.Log(ctx, "info", "message 3", nil)
	if !errors.Is(err, ErrRateLimitExceeded) {
		t.Errorf("expected ErrRateLimitExceeded, got %v", err)
	}

	// Check we only got 2 log entries
	if len(logger.entries) != 2 {
		t.Errorf("expected 2 log entries, got %d", len(logger.entries))
	}
}

// Tests for TimeCapability

func TestTimeCapability_Validate(t *testing.T) {
	timeCap := &TimeCapability{}
	if err := timeCap.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTimeCapability_Now(t *testing.T) {
	timeCap := &TimeCapability{}
	ctx := NewCapabilityContext(context.Background(), "test-module")

	before := time.Now()
	now := timeCap.Now(ctx)
	after := time.Now()

	if now.Before(before) || now.After(after) {
		t.Errorf("time out of expected range: %v not between %v and %v", now, before, after)
	}
}

func TestTimeCapability_Unix(t *testing.T) {
	timeCap := &TimeCapability{}
	ctx := NewCapabilityContext(context.Background(), "test-module")

	before := time.Now().Unix()
	unix := timeCap.Unix(ctx)
	after := time.Now().Unix()

	if unix < before || unix > after {
		t.Errorf("unix time out of expected range: %d not between %d and %d", unix, before, after)
	}
}

// Tests for KVCapability

func TestKVCapability_Validate(t *testing.T) {
	tests := []struct {
		name        string
		cap         *KVCapability
		expectError bool
	}{
		{
			name: "valid capability",
			cap: &KVCapability{
				Namespace: "test-module",
			},
			expectError: false,
		},
		{
			name: "empty namespace",
			cap: &KVCapability{
				Namespace: "",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cap.Validate()
			if tt.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestKVCapability_SetGet(t *testing.T) {
	store := newMockKVStore()

	testCap := &KVCapability{
		Namespace: "test-module",
	}
	testCap.SetStore(store)

	ctx := NewCapabilityContext(context.Background(), "test-module")

	// Test set
	err := testCap.Set(ctx, "key1", "value1")
	if err != nil {
		t.Fatalf("failed to set: %v", err)
	}

	// Test get
	value, err := testCap.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}

	if value != "value1" {
		t.Errorf("expected value 'value1', got %q", value)
	}

	// Verify namespace was applied
	namespacedValue, _ := store.Get("test-module/key1")
	if namespacedValue != "value1" {
		t.Errorf("namespace not applied correctly")
	}
}

func TestKVCapability_Delete(t *testing.T) {
	store := newMockKVStore()
	store.Set("test-module/key1", "value1")

	testCap := &KVCapability{
		Namespace: "test-module",
	}
	testCap.SetStore(store)

	ctx := NewCapabilityContext(context.Background(), "test-module")

	// Test delete
	err := testCap.Delete(ctx, "key1")
	if err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	// Verify it was deleted
	_, err = store.Get("test-module/key1")
	if err == nil {
		t.Error("key still exists after deletion")
	}
}

func TestKVCapability_List(t *testing.T) {
	store := newMockKVStore()
	store.Set("test-module/key1", "value1")
	store.Set("test-module/key2", "value2")
	store.Set("other-module/key3", "value3")

	testCap := &KVCapability{
		Namespace: "test-module",
	}
	testCap.SetStore(store)

	ctx := NewCapabilityContext(context.Background(), "test-module")

	// Test list
	keys, err := testCap.List(ctx)
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}

	// Should only get keys from our namespace
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}

	// Check keys (order doesn't matter)
	keyMap := make(map[string]bool)
	for _, key := range keys {
		keyMap[key] = true
	}

	if !keyMap["key1"] || !keyMap["key2"] {
		t.Errorf("unexpected keys: %v", keys)
	}
}

func TestMatchesPath(t *testing.T) {
	tests := []struct {
		pattern  string
		path     string
		expected bool
	}{
		{
			pattern:  "app/db",
			path:     "app/db",
			expected: true,
		},
		{
			pattern:  "app/*",
			path:     "app/db",
			expected: true,
		},
		{
			pattern:  "app/*",
			path:     "app/db/password",
			expected: true,
		},
		{
			pattern:  "app/*",
			path:     "other/secret",
			expected: false,
		},
		{
			pattern:  "app/*/password",
			path:     "app/db/password",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.path, func(t *testing.T) {
			result := matchesPath(tt.pattern, tt.path)
			if result != tt.expected {
				t.Errorf("matchesPath(%q, %q) = %v, expected %v", tt.pattern, tt.path, result, tt.expected)
			}
		})
	}
}
