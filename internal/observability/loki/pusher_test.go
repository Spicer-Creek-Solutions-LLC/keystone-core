package loki

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.URL == "" {
		t.Error("URL should have a default")
	}
	if config.BatchSize <= 0 {
		t.Error("BatchSize should be positive")
	}
	if config.BatchWait <= 0 {
		t.Error("BatchWait should be positive")
	}
	if config.Timeout <= 0 {
		t.Error("Timeout should be positive")
	}
}

func TestPusher_StartStop(t *testing.T) {
	config := DefaultConfig()
	config.BatchWait = 10 * time.Millisecond

	pusher := NewPusher(config)

	if err := pusher.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Starting again should be no-op
	if err := pusher.Start(); err != nil {
		t.Fatalf("Second Start failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := pusher.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Stopping again should be no-op
	if err := pusher.Stop(ctx); err != nil {
		t.Fatalf("Second Stop failed: %v", err)
	}
}

func TestPusher_Push(t *testing.T) {
	var receivedRequests int32
	var receivedEntries int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&receivedRequests, 1)

		var body io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				t.Errorf("Failed to create gzip reader: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			defer gz.Close()
			body = gz
		}

		var req PushRequest
		if err := json.NewDecoder(body).Decode(&req); err != nil {
			t.Errorf("Failed to decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		for _, stream := range req.Streams {
			atomic.AddInt32(&receivedEntries, int32(len(stream.Values)))
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.URL = server.URL
	config.BatchWait = 50 * time.Millisecond
	config.BatchSize = 10

	pusher := NewPusher(config)
	pusher.Start()

	// Push some entries
	for i := 0; i < 5; i++ {
		err := pusher.Push(&Entry{
			Timestamp: time.Now(),
			Line:      "test message",
			Level:     LogLevelInfo,
		})
		if err != nil {
			t.Fatalf("Push failed: %v", err)
		}
	}

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return atomic.LoadInt32(&receivedEntries) == 5, nil
	}); err != nil {
		t.Fatalf("Timed out waiting for batch flush: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	pusher.Stop(ctx)

	if atomic.LoadInt32(&receivedRequests) == 0 {
		t.Error("Expected at least one request")
	}
	if atomic.LoadInt32(&receivedEntries) != 5 {
		t.Errorf("Expected 5 entries, got %d", atomic.LoadInt32(&receivedEntries))
	}
}

func TestPusher_PushLog(t *testing.T) {
	var received atomic.Value
	receivedCh := make(chan struct{}, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, _ := gzip.NewReader(r.Body)
			defer gz.Close()
			body = gz
		}

		var req PushRequest
		json.NewDecoder(body).Decode(&req)
		received.Store(req)
		receivedCh <- struct{}{}

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.URL = server.URL
	config.BatchWait = 50 * time.Millisecond

	pusher := NewPusher(config)
	pusher.Start()

	labels := map[string]string{"app": "test", "env": "dev"}
	pusher.PushLog(LogLevelError, "test error message", labels)

	select {
	case <-receivedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for push request")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	pusher.Stop(ctx)

	req := received.Load().(PushRequest)
	if len(req.Streams) == 0 {
		t.Fatal("Expected at least one stream")
	}

	stream := req.Streams[0]
	if stream.Labels["level"] != "error" {
		t.Errorf("Level = %v, want error", stream.Labels["level"])
	}
	if stream.Labels["app"] != "test" {
		t.Errorf("app label = %v, want test", stream.Labels["app"])
	}
}

func TestPusher_BatchFlush(t *testing.T) {
	var requestCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.URL = server.URL
	config.BatchWait = time.Hour // Long wait
	config.BatchSize = 5

	pusher := NewPusher(config)
	pusher.Start()

	// Push batch size entries to trigger immediate flush
	for i := 0; i < 5; i++ {
		pusher.Push(&Entry{
			Timestamp: time.Now(),
			Line:      "message",
		})
	}

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return atomic.LoadInt32(&requestCount) > 0, nil
	}); err != nil {
		t.Fatalf("Expected batch to flush: %v", err)
	}

	if atomic.LoadInt32(&requestCount) == 0 {
		t.Error("Expected batch to be flushed when size reached")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	pusher.Stop(ctx)
}

func TestPusher_Retry(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.URL = server.URL
	config.BatchWait = 10 * time.Millisecond
	config.RetryCount = 3
	config.RetryBaseDelay = 10 * time.Millisecond
	config.RetryMaxDelay = 50 * time.Millisecond

	pusher := NewPusher(config)
	pusher.Start()

	pusher.Push(&Entry{
		Timestamp: time.Now(),
		Line:      "message",
	})

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return atomic.LoadInt32(&attempts) >= 3, nil
	}); err != nil {
		t.Fatalf("Expected retries to reach 3 attempts: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	pusher.Stop(ctx)

	if atomic.LoadInt32(&attempts) < 3 {
		t.Errorf("Expected at least 3 attempts, got %d", atomic.LoadInt32(&attempts))
	}
}

func TestPusher_TenantID(t *testing.T) {
	var receivedTenantID string
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedTenantID = r.Header.Get("X-Scope-OrgID")
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.URL = server.URL
	config.TenantID = "test-tenant"
	config.BatchWait = 10 * time.Millisecond

	pusher := NewPusher(config)
	pusher.Start()

	pusher.Push(&Entry{
		Timestamp: time.Now(),
		Line:      "message",
	})

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		return receivedTenantID != "", nil
	}); err != nil {
		t.Fatalf("Expected tenant header to be set: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	pusher.Stop(ctx)

	mu.Lock()
	if receivedTenantID != "test-tenant" {
		t.Errorf("TenantID = %v, want test-tenant", receivedTenantID)
	}
	mu.Unlock()
}

func TestPusher_Authentication(t *testing.T) {
	t.Run("basic auth", func(t *testing.T) {
		var receivedAuth string
		var mu sync.Mutex

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			receivedAuth = r.Header.Get("Authorization")
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()

		config := DefaultConfig()
		config.URL = server.URL
		config.Username = "user"
		config.Password = "pass"
		config.BatchWait = 10 * time.Millisecond

		pusher := NewPusher(config)
		pusher.Start()

		pusher.Push(&Entry{Timestamp: time.Now(), Line: "test"})
		if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
			mu.Lock()
			defer mu.Unlock()
			return receivedAuth != "", nil
		}); err != nil {
			t.Fatalf("Expected Authorization header: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		pusher.Stop(ctx)

		mu.Lock()
		if receivedAuth == "" {
			t.Error("Expected Authorization header")
		}
		mu.Unlock()
	})

	t.Run("bearer token", func(t *testing.T) {
		var receivedAuth string
		var mu sync.Mutex

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			receivedAuth = r.Header.Get("Authorization")
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()

		config := DefaultConfig()
		config.URL = server.URL
		config.BearerToken = "my-token"
		config.BatchWait = 10 * time.Millisecond

		pusher := NewPusher(config)
		pusher.Start()

		pusher.Push(&Entry{Timestamp: time.Now(), Line: "test"})
		if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
			mu.Lock()
			defer mu.Unlock()
			return receivedAuth != "", nil
		}); err != nil {
			t.Fatalf("Expected Authorization header: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		pusher.Stop(ctx)

		mu.Lock()
		if receivedAuth != "Bearer my-token" {
			t.Errorf("Authorization = %v, want Bearer my-token", receivedAuth)
		}
		mu.Unlock()
	})
}

func TestPusher_Events(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.URL = server.URL
	config.BatchWait = 10 * time.Millisecond

	pusher := NewPusher(config)

	var receivedEvent *PushEvent
	var mu sync.Mutex
	pusher.AddListener(func(event *PushEvent) {
		mu.Lock()
		receivedEvent = event
		mu.Unlock()
	})

	pusher.Start()

	pusher.Push(&Entry{Timestamp: time.Now(), Line: "test"})
	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		return receivedEvent != nil, nil
	}); err != nil {
		t.Fatalf("Expected to receive event: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	pusher.Stop(ctx)

	mu.Lock()
	if receivedEvent == nil {
		t.Fatal("Expected to receive event")
	}
	if receivedEvent.Type != "push" {
		t.Errorf("Type = %v, want push", receivedEvent.Type)
	}
	if !receivedEvent.Success {
		t.Error("Expected success")
	}
	mu.Unlock()
}

func TestPusher_Stats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.URL = server.URL
	config.BatchWait = 10 * time.Millisecond

	pusher := NewPusher(config)
	pusher.Start()

	for i := 0; i < 5; i++ {
		pusher.Push(&Entry{Timestamp: time.Now(), Line: "test"})
	}

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		stats := pusher.Stats()
		return stats.EntriesPushed == 5 && stats.PushCount > 0, nil
	}); err != nil {
		t.Fatalf("Stats did not update: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	pusher.Stop(ctx)

	stats := pusher.Stats()
	if stats.EntriesPushed != 5 {
		t.Errorf("EntriesPushed = %d, want 5", stats.EntriesPushed)
	}
	if stats.PushCount == 0 {
		t.Error("PushCount should be > 0")
	}
}

func TestPusher_StoppedError(t *testing.T) {
	config := DefaultConfig()
	pusher := NewPusher(config)

	// Don't start the pusher
	err := pusher.Push(&Entry{Timestamp: time.Now(), Line: "test"})
	if !errors.Is(err, ErrPusherStopped) {
		t.Errorf("Expected ErrPusherStopped, got %v", err)
	}
}

func TestLogWriter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.URL = server.URL
	config.BatchWait = 10 * time.Millisecond

	pusher := NewPusher(config)
	pusher.Start()

	labels := map[string]string{"app": "test"}
	writer := NewLogWriter(pusher, LogLevelInfo, labels)

	n, err := writer.Write([]byte("test message\n"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != 13 {
		t.Errorf("Write returned %d, want 13", n)
	}

	// Empty write should succeed
	_, err = writer.Write([]byte("  \n  "))
	if err != nil {
		t.Fatalf("Empty write failed: %v", err)
	}

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		stats := pusher.Stats()
		return stats.EntriesPushed > 0, nil
	}); err != nil {
		t.Fatalf("Expected log write to flush: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	pusher.Stop(ctx)
}

func TestMultiTenantPusher(t *testing.T) {
	var tenants sync.Map

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.Header.Get("X-Scope-OrgID")
		tenants.Store(tenantID, true)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.URL = server.URL
	config.BatchWait = 10 * time.Millisecond

	mp := NewMultiTenantPusher(config)

	// Push to different tenants
	mp.Push("tenant1", &Entry{Timestamp: time.Now(), Line: "msg1"})
	mp.Push("tenant2", &Entry{Timestamp: time.Now(), Line: "msg2"})
	mp.Push("tenant1", &Entry{Timestamp: time.Now(), Line: "msg3"})

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		_, ok1 := tenants.Load("tenant1")
		_, ok2 := tenants.Load("tenant2")
		return ok1 && ok2, nil
	}); err != nil {
		t.Fatalf("Expected tenants to receive data: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	mp.Stop(ctx)

	// Verify both tenants received data
	if _, ok := tenants.Load("tenant1"); !ok {
		t.Error("tenant1 should have received data")
	}
	if _, ok := tenants.Load("tenant2"); !ok {
		t.Error("tenant2 should have received data")
	}
}

func TestFilteredPusher(t *testing.T) {
	var entryCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, _ := gzip.NewReader(r.Body)
			defer gz.Close()
			body = gz
		}

		var req PushRequest
		json.NewDecoder(body).Decode(&req)
		for _, stream := range req.Streams {
			atomic.AddInt32(&entryCount, int32(len(stream.Values)))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.URL = server.URL
	config.BatchWait = 10 * time.Millisecond

	pusher := NewPusher(config)
	pusher.Start()

	filtered := NewFilteredPusher(pusher, LevelFilter(LogLevelWarn))

	// Push entries at different levels
	filtered.Push(&Entry{Timestamp: time.Now(), Line: "debug", Level: LogLevelDebug})
	filtered.Push(&Entry{Timestamp: time.Now(), Line: "info", Level: LogLevelInfo})
	filtered.Push(&Entry{Timestamp: time.Now(), Line: "warn", Level: LogLevelWarn})
	filtered.Push(&Entry{Timestamp: time.Now(), Line: "error", Level: LogLevelError})

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return atomic.LoadInt32(&entryCount) == 2, nil
	}); err != nil {
		t.Fatalf("Expected filtered entries to flush: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	pusher.Stop(ctx)

	// Only warn and error should be pushed
	if atomic.LoadInt32(&entryCount) != 2 {
		t.Errorf("Expected 2 entries (warn+error), got %d", atomic.LoadInt32(&entryCount))
	}
}

func TestSamplingPusher(t *testing.T) {
	var entryCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, _ := gzip.NewReader(r.Body)
			defer gz.Close()
			body = gz
		}

		var req PushRequest
		json.NewDecoder(body).Decode(&req)
		for _, stream := range req.Streams {
			atomic.AddInt32(&entryCount, int32(len(stream.Values)))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.URL = server.URL
	config.BatchWait = 10 * time.Millisecond

	pusher := NewPusher(config)
	pusher.Start()

	// 50% sample rate
	sampled := NewSamplingPusher(pusher, 0.5)

	// Push many entries
	for i := 0; i < 100; i++ {
		sampled.Push(&Entry{Timestamp: time.Now(), Line: "test"})
	}

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return atomic.LoadInt32(&entryCount) >= 20, nil
	}); err != nil {
		t.Fatalf("Expected sampled entries to flush: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	pusher.Stop(ctx)

	count := atomic.LoadInt32(&entryCount)
	// With 50% sample rate, we expect roughly 50 entries (allow for variance)
	if count < 20 || count > 80 {
		t.Errorf("Expected ~50 entries with 50%% sampling, got %d", count)
	}
}

func TestLabelsKey(t *testing.T) {
	labels1 := map[string]string{"a": "1", "b": "2"}
	labels2 := map[string]string{"b": "2", "a": "1"}

	key1 := labelsKey(labels1)
	key2 := labelsKey(labels2)

	if key1 != key2 {
		t.Errorf("Keys should match: %v != %v", key1, key2)
	}

	expected := "a=1,b=2"
	if key1 != expected {
		t.Errorf("Key = %v, want %v", key1, expected)
	}
}

func TestBatchPusher(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.URL = server.URL
	config.BatchWait = 10 * time.Millisecond
	config.BatchSize = 10

	bp := NewBatchPusher(config)
	bp.Start()

	// Push entries
	for i := 0; i < 5; i++ {
		bp.Push(&Entry{Timestamp: time.Now(), Line: "test"})
	}

	// Manually flush
	bp.Flush()

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		stats := bp.Stats()
		return stats.EntriesPushed == 5, nil
	}); err != nil {
		t.Fatalf("Expected batch to flush: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	bp.Stop(ctx)

	stats := bp.Stats()
	if stats.EntriesPushed != 5 {
		t.Errorf("EntriesPushed = %d, want 5", stats.EntriesPushed)
	}
}
