package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

// mockBackend is a mock storage backend for testing
type mockBackend struct {
	name      string
	data      map[string][]byte
	mu        sync.RWMutex
	pingErr   error
	putErr    error
	getErr    error
	deleteErr error
	latency   time.Duration
}

func newMockBackend(name string) *mockBackend {
	return &mockBackend{
		name: name,
		data: make(map[string][]byte),
	}
}

func (m *mockBackend) Name() string {
	return m.name
}

func (m *mockBackend) Put(ctx context.Context, key string, data io.Reader, size int64) error {
	m.mu.RLock()
	latency := m.latency
	putErr := m.putErr
	m.mu.RUnlock()
	if latency > 0 {
		timer := time.NewTimer(latency)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	if putErr != nil {
		return putErr
	}

	bytes, err := io.ReadAll(data)
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.data[key] = bytes
	m.mu.Unlock()
	return nil
}

func (m *mockBackend) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	m.mu.RLock()
	latency := m.latency
	getErr := m.getErr
	m.mu.RUnlock()
	if latency > 0 {
		timer := time.NewTimer(latency)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	if getErr != nil {
		return nil, getErr
	}

	m.mu.RLock()
	data, ok := m.data[key]
	m.mu.RUnlock()

	if !ok {
		return nil, errors.New("key not found")
	}

	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *mockBackend) Delete(ctx context.Context, key string) error {
	m.mu.RLock()
	latency := m.latency
	deleteErr := m.deleteErr
	m.mu.RUnlock()
	if latency > 0 {
		timer := time.NewTimer(latency)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	if deleteErr != nil {
		return deleteErr
	}

	m.mu.Lock()
	delete(m.data, key)
	m.mu.Unlock()
	return nil
}

func (m *mockBackend) Exists(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	_, ok := m.data[key]
	m.mu.RUnlock()
	return ok, nil
}

func (m *mockBackend) List(ctx context.Context, prefix string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var keys []string
	for k := range m.data {
		if len(prefix) == 0 || len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (m *mockBackend) Ping(ctx context.Context) error {
	m.mu.RLock()
	latency := m.latency
	pingErr := m.pingErr
	m.mu.RUnlock()
	if latency > 0 {
		timer := time.NewTimer(latency)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return pingErr
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.HealthCheckInterval != 30*time.Second {
		t.Errorf("HealthCheckInterval = %v, want 30s", cfg.HealthCheckInterval)
	}
	if cfg.MaxConsecutiveFailures != 3 {
		t.Errorf("MaxConsecutiveFailures = %d, want 3", cfg.MaxConsecutiveFailures)
	}
	if cfg.QueueSize != 1000 {
		t.Errorf("QueueSize = %d, want 1000", cfg.QueueSize)
	}
	if !cfg.EnableQueue {
		t.Error("EnableQueue should be true by default")
	}
}

func TestNewManager(t *testing.T) {
	// With nil config
	m := NewManager(nil)
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.config.HealthCheckInterval != 30*time.Second {
		t.Error("Should use default config when nil")
	}

	// With custom config
	cfg := &Config{HealthCheckInterval: 1 * time.Minute}
	m = NewManager(cfg)
	if m.config.HealthCheckInterval != 1*time.Minute {
		t.Error("Should use provided config")
	}
}

func TestManager_AddBackend(t *testing.T) {
	m := NewManager(nil)

	backend1 := newMockBackend("backend1")
	backend2 := newMockBackend("backend2")

	m.AddBackend(backend1)
	m.AddBackend(backend2)

	health := m.GetBackendHealth()
	if len(health) != 2 {
		t.Errorf("Should have 2 backends, got %d", len(health))
	}

	if health[0].Backend.Name() != "backend1" {
		t.Errorf("First backend name = %s, want backend1", health[0].Backend.Name())
	}
}

func TestManager_Put_Success(t *testing.T) {
	m := NewManager(nil)
	backend := newMockBackend("test")
	m.AddBackend(backend)

	ctx := context.Background()
	data := []byte("test data")

	err := m.Put(ctx, "key1", bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Verify data was stored
	backend.mu.RLock()
	stored, ok := backend.data["key1"]
	backend.mu.RUnlock()

	if !ok {
		t.Error("Data was not stored")
	}
	if !bytes.Equal(stored, data) {
		t.Error("Stored data doesn't match")
	}
}

func TestManager_Put_Failover(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RetryAttempts = 1
	m := NewManager(cfg)

	backend1 := newMockBackend("backend1")
	backend1.mu.Lock()
	backend1.putErr = errors.New("backend1 error")
	backend1.mu.Unlock()

	backend2 := newMockBackend("backend2")

	m.AddBackend(backend1)
	m.AddBackend(backend2)

	ctx := context.Background()
	data := []byte("test data")

	err := m.Put(ctx, "key1", bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Put should succeed with failover: %v", err)
	}

	// Verify data was stored in backend2
	backend2.mu.RLock()
	_, ok := backend2.data["key1"]
	backend2.mu.RUnlock()

	if !ok {
		t.Error("Data should be stored in backend2")
	}
}

func TestManager_Get_Success(t *testing.T) {
	m := NewManager(nil)
	backend := newMockBackend("test")
	backend.data["key1"] = []byte("test data")
	m.AddBackend(backend)

	ctx := context.Background()

	reader, err := m.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer reader.Close()

	data, _ := io.ReadAll(reader)
	if !bytes.Equal(data, []byte("test data")) {
		t.Error("Retrieved data doesn't match")
	}
}

func TestManager_Get_Failover(t *testing.T) {
	m := NewManager(nil)

	backend1 := newMockBackend("backend1")
	backend1.mu.Lock()
	backend1.getErr = errors.New("backend1 error")
	backend1.mu.Unlock()

	backend2 := newMockBackend("backend2")
	backend2.data["key1"] = []byte("test data")

	m.AddBackend(backend1)
	m.AddBackend(backend2)

	ctx := context.Background()

	reader, err := m.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get should succeed with failover: %v", err)
	}
	defer reader.Close()

	data, _ := io.ReadAll(reader)
	if !bytes.Equal(data, []byte("test data")) {
		t.Error("Retrieved data doesn't match")
	}
}

func TestManager_Delete_Success(t *testing.T) {
	m := NewManager(nil)
	backend := newMockBackend("test")
	backend.data["key1"] = []byte("test data")
	m.AddBackend(backend)

	ctx := context.Background()

	err := m.Delete(ctx, "key1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	backend.mu.RLock()
	_, ok := backend.data["key1"]
	backend.mu.RUnlock()

	if ok {
		t.Error("Data should be deleted")
	}
}

func TestManager_Exists(t *testing.T) {
	m := NewManager(nil)
	backend := newMockBackend("test")
	backend.data["key1"] = []byte("test data")
	m.AddBackend(backend)

	ctx := context.Background()

	exists, err := m.Exists(ctx, "key1")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("Key should exist")
	}

	exists, err = m.Exists(ctx, "key2")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Error("Key should not exist")
	}
}

func TestManager_List(t *testing.T) {
	m := NewManager(nil)
	backend := newMockBackend("test")
	backend.data["prefix/key1"] = []byte("data1")
	backend.data["prefix/key2"] = []byte("data2")
	backend.data["other/key3"] = []byte("data3")
	m.AddBackend(backend)

	ctx := context.Background()

	keys, err := m.List(ctx, "prefix/")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(keys) != 2 {
		t.Errorf("Should have 2 keys with prefix, got %d", len(keys))
	}
}

func TestManager_NoBackends(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableQueue = false
	m := NewManager(cfg)

	ctx := context.Background()

	_, err := m.Get(ctx, "key1")
	if err == nil {
		t.Error("Should fail with no backends")
	}

	err = m.Put(ctx, "key1", bytes.NewReader([]byte("data")), 4)
	if err == nil {
		t.Error("Should fail with no backends")
	}
}

func TestManager_Put_ContextCancelDuringRetry(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableQueue = false
	cfg.RetryAttempts = 2
	cfg.RetryDelay = time.Second
	m := NewManager(cfg)

	backend := newMockBackend("test")
	backend.putErr = errors.New("put failed")
	m.AddBackend(backend)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := m.Put(ctx, "key1", bytes.NewReader([]byte("data")), 4)
	if err == nil {
		t.Fatal("expected error from context timeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
}

func TestManager_HealthCheck(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HealthCheckInterval = 50 * time.Millisecond
	cfg.MaxConsecutiveFailures = 2
	m := NewManager(cfg)

	backend := newMockBackend("test")
	m.AddBackend(backend)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := m.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer m.Stop()

	if err := helpers.WaitForTimeout(500*time.Millisecond, 10*time.Millisecond, func() (bool, error) {
		health := m.GetBackendHealth()
		return len(health) > 0 && health[0].Status == StatusHealthy, nil
	}); err != nil {
		t.Fatalf("expected healthy backend: %v", err)
	}

	health := m.GetBackendHealth()
	if len(health) == 0 {
		t.Fatal("No backend health info")
	}
	if health[0].Status != StatusHealthy {
		t.Errorf("Backend should be healthy, got %s", health[0].Status)
	}

	// Make backend unhealthy
	backend.mu.Lock()
	backend.pingErr = errors.New("ping failed")
	backend.mu.Unlock()

	if err := helpers.WaitForTimeout(500*time.Millisecond, 10*time.Millisecond, func() (bool, error) {
		health = m.GetBackendHealth()
		return len(health) > 0 && health[0].Status != StatusHealthy, nil
	}); err != nil {
		t.Fatalf("expected unhealthy backend: %v", err)
	}

	health = m.GetBackendHealth()
	if health[0].Status == StatusHealthy {
		t.Error("Backend should not be healthy after ping failures")
	}
}

func TestManager_GetPrimaryBackendName(t *testing.T) {
	m := NewManager(nil)

	// No backends
	name := m.GetPrimaryBackendName()
	if name != "" {
		t.Error("Should return empty string with no backends")
	}

	backend := newMockBackend("test-backend")
	m.AddBackend(backend)

	name = m.GetPrimaryBackendName()
	if name != "test-backend" {
		t.Errorf("Primary backend name = %s, want test-backend", name)
	}
}

func TestManager_QueueOperation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableQueue = true
	cfg.QueueSize = 10
	m := NewManager(cfg)

	// No backends, operations should be queued
	ctx := context.Background()
	data := []byte("test data")

	err := m.Put(ctx, "key1", bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Put should succeed (queued): %v", err)
	}

	if m.GetQueueSize() != 1 {
		t.Errorf("Queue size = %d, want 1", m.GetQueueSize())
	}
}

func TestManager_QueueFull(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableQueue = true
	cfg.QueueSize = 1
	m := NewManager(cfg)

	ctx := context.Background()

	// First operation should queue
	err := m.Put(ctx, "key1", bytes.NewReader([]byte("data1")), 5)
	if err != nil {
		t.Fatalf("First put should succeed: %v", err)
	}

	// Second operation should fail (queue full)
	err = m.Put(ctx, "key2", bytes.NewReader([]byte("data2")), 5)
	if err == nil {
		t.Error("Second put should fail with full queue")
	}
}

func TestManager_StartStop(t *testing.T) {
	m := NewManager(nil)
	backend := newMockBackend("test")
	m.AddBackend(backend)

	ctx := context.Background()

	// Start
	err := m.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Double start should fail
	err = m.Start(ctx)
	if err == nil {
		t.Error("Double start should fail")
	}

	// Stop
	m.Stop()

	// Double stop should be safe
	m.Stop()
}

func TestStats_RecordOperation(t *testing.T) {
	s := NewStats()

	s.RecordOperation(OpPut, "backend1", true)
	s.RecordOperation(OpPut, "backend1", false)
	s.RecordOperation(OpGet, "backend2", true)

	if s.TotalOperations != 3 {
		t.Errorf("TotalOperations = %d, want 3", s.TotalOperations)
	}
	if s.SuccessfulOps != 2 {
		t.Errorf("SuccessfulOps = %d, want 2", s.SuccessfulOps)
	}
	if s.FailedOps != 1 {
		t.Errorf("FailedOps = %d, want 1", s.FailedOps)
	}

	backend1Stats := s.ByBackend["backend1"]
	if backend1Stats.Operations != 2 {
		t.Errorf("backend1 operations = %d, want 2", backend1Stats.Operations)
	}

	putStats := s.ByOperation[OpPut]
	if putStats.Total != 2 {
		t.Errorf("Put operations = %d, want 2", putStats.Total)
	}
}

func TestStats_RecordHealthCheck(t *testing.T) {
	s := NewStats()

	s.RecordHealthCheck("backend1", true)
	s.RecordHealthCheck("backend1", false)
	s.RecordHealthCheck("backend2", true)

	if s.HealthChecks != 3 {
		t.Errorf("HealthChecks = %d, want 3", s.HealthChecks)
	}
	if s.HealthCheckFails != 1 {
		t.Errorf("HealthCheckFails = %d, want 1", s.HealthCheckFails)
	}
}

func TestStats_RecordFailover(t *testing.T) {
	s := NewStats()

	s.RecordFailover("backend1", "backend2")
	s.RecordFailover("backend2", "backend1")

	if s.FailoverCount != 2 {
		t.Errorf("FailoverCount = %d, want 2", s.FailoverCount)
	}
}

func TestStats_QueueMetrics(t *testing.T) {
	s := NewStats()

	s.RecordQueued()
	s.RecordQueued()
	s.RecordQueueProcessed()
	s.RecordQueueExpired()

	if s.QueuedOps != 2 {
		t.Errorf("QueuedOps = %d, want 2", s.QueuedOps)
	}
	if s.QueueProcessed != 1 {
		t.Errorf("QueueProcessed = %d, want 1", s.QueueProcessed)
	}
	if s.QueueExpired != 1 {
		t.Errorf("QueueExpired = %d, want 1", s.QueueExpired)
	}
}

func TestStats_Snapshot(t *testing.T) {
	s := NewStats()

	s.RecordOperation(OpPut, "backend1", true)
	s.RecordHealthCheck("backend1", true)

	snapshot := s.Snapshot()

	// Modify original
	s.RecordOperation(OpGet, "backend2", true)

	// Snapshot should be unchanged
	if snapshot.TotalOperations != 1 {
		t.Error("Snapshot should be independent copy")
	}
	if len(snapshot.ByBackend) != 1 {
		t.Error("Snapshot should have 1 backend")
	}
}

func TestBackendHealth_Status(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxConsecutiveFailures = 2
	m := NewManager(cfg)

	backend := newMockBackend("test")
	m.AddBackend(backend)

	// Initial status should be healthy
	health := m.GetBackendHealth()
	if health[0].Status != StatusHealthy {
		t.Errorf("Initial status = %s, want healthy", health[0].Status)
	}
}

func TestBytesReaderAt(t *testing.T) {
	data := []byte("hello world")
	r := &bytesReaderAt{data: data}

	// Read from start
	buf := make([]byte, 5)
	n, err := r.ReadAt(buf, 0)
	if err != nil {
		t.Errorf("ReadAt failed: %v", err)
	}
	if n != 5 {
		t.Errorf("Read %d bytes, want 5", n)
	}
	if string(buf) != "hello" {
		t.Errorf("Got %q, want 'hello'", string(buf))
	}

	// Read from offset
	n, err = r.ReadAt(buf, 6)
	if n != 5 {
		t.Errorf("Read %d bytes, want 5", n)
	}
	if string(buf) != "world" {
		t.Errorf("Got %q, want 'world'", string(buf))
	}

	// Read past end
	_, err = r.ReadAt(buf, 100)
	if err != io.EOF {
		t.Error("Should return EOF when reading past end")
	}
}

func TestManager_ProcessQueue(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableQueue = true
	cfg.HealthCheckInterval = 50 * time.Millisecond
	cfg.RetryAttempts = 2
	m := NewManager(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Queue an operation with no backends
	data := []byte("test data")
	err := m.Put(ctx, "key1", bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Put should succeed (queued): %v", err)
	}

	// Add a backend
	backend := newMockBackend("test")
	m.AddBackend(backend)

	// Start the manager to process queue
	err = m.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if err := helpers.WaitForTimeout(500*time.Millisecond, 10*time.Millisecond, func() (bool, error) {
		backend.mu.RLock()
		_, ok := backend.data["key1"]
		backend.mu.RUnlock()
		return ok, nil
	}); err != nil {
		t.Fatalf("expected queued operation to process: %v", err)
	}

	m.Stop()

	// Check if data was stored
	backend.mu.RLock()
	_, ok := backend.data["key1"]
	backend.mu.RUnlock()

	if !ok {
		t.Error("Queued operation should have been processed")
	}
}
