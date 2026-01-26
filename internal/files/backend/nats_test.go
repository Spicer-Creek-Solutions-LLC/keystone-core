package backend

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

// MockNATSObjectStore implements NATSObjectStore for testing.
type MockNATSObjectStore struct {
	objects map[string]*mockNATSStoredObject
}

type mockNATSStoredObject struct {
	data    []byte
	info    *NATSObjectInfo
	deleted bool
}

func NewMockNATSObjectStore() *MockNATSObjectStore {
	return &MockNATSObjectStore{
		objects: make(map[string]*mockNATSStoredObject),
	}
}

func (m *MockNATSObjectStore) Get(ctx context.Context, name string) (*NATSObject, error) {
	obj, ok := m.objects[name]
	if !ok || obj.deleted {
		return nil, fmt.Errorf("object not found: %s", name)
	}

	return &NATSObject{
		Body: io.NopCloser(bytes.NewReader(obj.data)),
		Info: obj.info,
	}, nil
}

func (m *MockNATSObjectStore) Put(ctx context.Context, name string, body io.Reader, opts *NATSPutOptions) (*NATSObjectInfo, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}

	// Generate a digest (simplified for testing)
	digest := fmt.Sprintf("SHA-256=%x", data[:min(8, len(data))])

	headers := make(map[string][]string)
	if opts != nil && opts.Headers != nil {
		for k, v := range opts.Headers {
			headers[k] = []string{v}
		}
	}

	info := &NATSObjectInfo{
		Name:    name,
		Size:    uint64(len(data)),
		Digest:  digest,
		ModTime: time.Now(),
		Headers: headers,
	}

	m.objects[name] = &mockNATSStoredObject{
		data: data,
		info: info,
	}

	return info, nil
}

func (m *MockNATSObjectStore) Delete(ctx context.Context, name string) error {
	obj, ok := m.objects[name]
	if !ok {
		return fmt.Errorf("object not found: %s", name)
	}
	obj.deleted = true
	obj.info.Deleted = true
	return nil
}

func (m *MockNATSObjectStore) GetInfo(ctx context.Context, name string) (*NATSObjectInfo, error) {
	obj, ok := m.objects[name]
	if !ok {
		return nil, fmt.Errorf("object not found: %s", name)
	}
	return obj.info, nil
}

func (m *MockNATSObjectStore) List(ctx context.Context, opts *NATSListOptions) ([]*NATSObjectInfo, error) {
	var result []*NATSObjectInfo
	for _, obj := range m.objects {
		if opts != nil && opts.FilterPrefix != "" {
			if !strings.HasPrefix(obj.info.Name, opts.FilterPrefix) {
				continue
			}
		}
		result = append(result, obj.info)
	}
	return result, nil
}

func (m *MockNATSObjectStore) Status(ctx context.Context) (*NATSBucketStatus, error) {
	var size uint64
	var count uint64
	for _, obj := range m.objects {
		if !obj.deleted {
			size += obj.info.Size
			count++
		}
	}

	return &NATSBucketStatus{
		Bucket:   "test-bucket",
		Size:     size,
		Objects:  count,
		Replicas: 1,
	}, nil
}

func TestNewNATSBackend(t *testing.T) {
	tests := []struct {
		name    string
		config  *NATSConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &NATSConfig{
				BucketName: "test-bucket",
				URL:        "nats://localhost:4222",
			},
			wantErr: false,
		},
		{
			name: "missing bucket name",
			config: &NATSConfig{
				URL: "nats://localhost:4222",
			},
			wantErr: true,
		},
		{
			name: "missing URL",
			config: &NATSConfig{
				BucketName: "test-bucket",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend, err := NewNATSBackend(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if backend == nil {
					t.Error("expected backend, got nil")
				}
			}
		})
	}
}

func TestNATSBackend_BasicOperations(t *testing.T) {
	config := &NATSConfig{
		Config: Config{
			Name: "test-nats",
			Type: BackendTypeNATSObject,
		},
		BucketName: "test-bucket",
		URL:        "nats://localhost:4222",
	}

	backend, err := NewNATSBackend(config)
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}

	store := NewMockNATSObjectStore()
	backend.SetStore(store)

	ctx := context.Background()

	// Test Name and Type
	if backend.Name() != "test-nats" {
		t.Errorf("expected name 'test-nats', got '%s'", backend.Name())
	}
	if backend.Type() != BackendTypeNATSObject {
		t.Errorf("expected type %s, got %s", BackendTypeNATSObject, backend.Type())
	}

	// Test Put
	content := []byte("Hello, NATS Object Store!")
	result, err := backend.Put(ctx, "/test/file.txt", bytes.NewReader(content), &PutOptions{
		ContentType: "text/plain",
	})
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if result.Size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), result.Size)
	}
	if result.Checksum == "" {
		t.Error("expected checksum, got empty")
	}

	// Test Exists
	exists, err := backend.Exists(ctx, "/test/file.txt")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("expected file to exist")
	}

	// Test Get
	getResult, err := backend.Get(ctx, "/test/file.txt", nil)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer getResult.Reader.Close()

	data, _ := io.ReadAll(getResult.Reader)
	if !bytes.Equal(data, content) {
		t.Errorf("expected content %q, got %q", content, data)
	}

	// Test Stat
	info, err := backend.Stat(ctx, "/test/file.txt")
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), info.Size)
	}
	if info.Name != "file.txt" {
		t.Errorf("expected name 'file.txt', got '%s'", info.Name)
	}

	// Test List
	listResult, err := backend.List(ctx, "/test", nil)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(listResult.Files) != 1 {
		t.Errorf("expected 1 file, got %d", len(listResult.Files))
	}

	// Test Delete
	err = backend.Delete(ctx, "/test/file.txt")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Test file no longer exists
	exists, err = backend.Exists(ctx, "/test/file.txt")
	if err != nil {
		t.Fatalf("Exists after delete failed: %v", err)
	}
	if exists {
		t.Error("expected file to not exist after delete")
	}
}

func TestNATSBackend_NotFound(t *testing.T) {
	config := &NATSConfig{
		BucketName: "test-bucket",
		URL:        "nats://localhost:4222",
	}

	backend, _ := NewNATSBackend(config)
	store := NewMockNATSObjectStore()
	backend.SetStore(store)

	ctx := context.Background()

	// Test Get not found
	_, err := backend.Get(ctx, "/nonexistent/file.txt", nil)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Test Stat not found
	_, err = backend.Stat(ctx, "/nonexistent/file.txt")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Test Exists returns false for nonexistent
	exists, err := backend.Exists(ctx, "/nonexistent/file.txt")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Error("expected file to not exist")
	}

	// Test Delete is idempotent for nonexistent
	err = backend.Delete(ctx, "/nonexistent/file.txt")
	if err != nil {
		t.Errorf("Delete should be idempotent, got: %v", err)
	}
}

func TestNATSBackend_ReadOnly(t *testing.T) {
	config := &NATSConfig{
		Config: Config{
			ReadOnly: true,
		},
		BucketName: "test-bucket",
		URL:        "nats://localhost:4222",
	}

	backend, _ := NewNATSBackend(config)
	store := NewMockNATSObjectStore()
	backend.SetStore(store)

	ctx := context.Background()

	// Test Put fails in read-only mode
	_, err := backend.Put(ctx, "/test/file.txt", bytes.NewReader([]byte("test")), nil)
	if err == nil {
		t.Error("expected error for Put in read-only mode")
	}

	// Test Delete fails in read-only mode
	err = backend.Delete(ctx, "/test/file.txt")
	if err == nil {
		t.Error("expected error for Delete in read-only mode")
	}
}

func TestNATSBackend_Prefix(t *testing.T) {
	config := &NATSConfig{
		BucketName: "test-bucket",
		URL:        "nats://localhost:4222",
		Prefix:     "myprefix",
	}

	backend, _ := NewNATSBackend(config)
	store := NewMockNATSObjectStore()
	backend.SetStore(store)

	ctx := context.Background()

	// Put with prefix
	_, err := backend.Put(ctx, "/test/file.txt", bytes.NewReader([]byte("test")), nil)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Verify the object was stored with prefix
	_, ok := store.objects["myprefix/test/file.txt"]
	if !ok {
		t.Error("expected object to be stored with prefix")
	}

	// Get should work with original path
	_, err = backend.Get(ctx, "/test/file.txt", nil)
	if err != nil {
		t.Errorf("Get failed: %v", err)
	}
}

func TestNATSBackend_ConditionalGet(t *testing.T) {
	config := &NATSConfig{
		BucketName: "test-bucket",
		URL:        "nats://localhost:4222",
	}

	backend, _ := NewNATSBackend(config)
	store := NewMockNATSObjectStore()
	backend.SetStore(store)

	ctx := context.Background()

	// Put a file
	_, err := backend.Put(ctx, "/test/file.txt", bytes.NewReader([]byte("test")), nil)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Get info to get the digest
	info, err := backend.Stat(ctx, "/test/file.txt")
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	// Conditional get with matching digest should return NotModified
	result, err := backend.Get(ctx, "/test/file.txt", &GetOptions{
		IfNoneMatch: info.ETag,
	})
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !result.NotModified {
		t.Error("expected NotModified to be true")
	}

	// Conditional get with non-matching digest should return content
	result, err = backend.Get(ctx, "/test/file.txt", &GetOptions{
		IfNoneMatch: "different-digest",
	})
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if result.NotModified {
		t.Error("expected NotModified to be false")
	}
	result.Reader.Close()
}

func TestNATSBackend_NoStore(t *testing.T) {
	config := &NATSConfig{
		BucketName: "test-bucket",
		URL:        "nats://localhost:4222",
	}

	backend, _ := NewNATSBackend(config)
	// Don't set store

	ctx := context.Background()

	// All operations should fail with "not configured" error
	_, err := backend.Get(ctx, "/test/file.txt", nil)
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected 'not configured' error, got: %v", err)
	}

	_, err = backend.Put(ctx, "/test/file.txt", bytes.NewReader([]byte("test")), nil)
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected 'not configured' error, got: %v", err)
	}

	err = backend.Delete(ctx, "/test/file.txt")
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected 'not configured' error, got: %v", err)
	}

	_, err = backend.Exists(ctx, "/test/file.txt")
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected 'not configured' error, got: %v", err)
	}

	_, err = backend.Stat(ctx, "/test/file.txt")
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected 'not configured' error, got: %v", err)
	}

	_, err = backend.List(ctx, "/test", nil)
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected 'not configured' error, got: %v", err)
	}

	err = backend.Health(ctx)
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("expected 'not configured' error, got: %v", err)
	}
}

func TestNATSBackend_Health(t *testing.T) {
	config := &NATSConfig{
		BucketName: "test-bucket",
		URL:        "nats://localhost:4222",
	}

	backend, _ := NewNATSBackend(config)
	store := NewMockNATSObjectStore()
	backend.SetStore(store)

	ctx := context.Background()

	err := backend.Health(ctx)
	if err != nil {
		t.Errorf("Health check failed: %v", err)
	}
}

func TestIsNATSNotFound(t *testing.T) {
	tests := []struct {
		err      error
		expected bool
	}{
		{nil, false},
		{fmt.Errorf("object not found"), true},
		{fmt.Errorf("not found: test"), true},
		{fmt.Errorf("no message found"), true},
		{fmt.Errorf("some other error"), false},
	}

	for _, tt := range tests {
		result := isNATSNotFound(tt.err)
		if result != tt.expected {
			t.Errorf("isNATSNotFound(%v) = %v, expected %v", tt.err, result, tt.expected)
		}
	}
}
