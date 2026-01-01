package nats

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestMessageBufferState_String(t *testing.T) {
	tests := []struct {
		state    MessageBufferState
		expected string
	}{
		{BufferStateIdle, "idle"},
		{BufferStateBuffering, "buffering"},
		{BufferStateFlushing, "flushing"},
		{BufferStateDraining, "draining"},
		{MessageBufferState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("MessageBufferState.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDefaultBufferConfig(t *testing.T) {
	cfg := DefaultBufferConfig()

	if !cfg.Enabled {
		t.Error("Enabled = false, want true")
	}
	if cfg.MaxSize != 64*1024*1024 {
		t.Errorf("MaxSize = %d, want 67108864 (64MB)", cfg.MaxSize)
	}
	if cfg.MaxMessages != 10000 {
		t.Errorf("MaxMessages = %d, want 10000", cfg.MaxMessages)
	}
	if cfg.MaxAge != 1*time.Hour {
		t.Errorf("MaxAge = %v, want 1h", cfg.MaxAge)
	}
	if cfg.PersistToDisk {
		t.Error("PersistToDisk = true, want false")
	}
	if cfg.StreamName != "KSCORE_BUFFER" {
		t.Errorf("StreamName = %s, want KSCORE_BUFFER", cfg.StreamName)
	}
	if cfg.FlushInterval != 5*time.Second {
		t.Errorf("FlushInterval = %v, want 5s", cfg.FlushInterval)
	}
	if cfg.FlushBatchSize != 100 {
		t.Errorf("FlushBatchSize = %d, want 100", cfg.FlushBatchSize)
	}
	if cfg.DeduplicationWindow != 2*time.Minute {
		t.Errorf("DeduplicationWindow = %v, want 2m", cfg.DeduplicationWindow)
	}
}

func TestNewMessageBuffer(t *testing.T) {
	tests := []struct {
		name    string
		config  *BufferConfig
		wantErr bool
	}{
		{
			name:    "nil config uses defaults",
			config:  nil,
			wantErr: false,
		},
		{
			name: "valid config",
			config: &BufferConfig{
				Enabled:     true,
				MaxSize:     1024 * 1024,
				MaxMessages: 1000,
			},
			wantErr: false,
		},
		{
			name: "invalid max size",
			config: &BufferConfig{
				MaxSize:     0,
				MaxMessages: 1000,
			},
			wantErr: true,
		},
		{
			name: "invalid max messages",
			config: &BufferConfig{
				MaxSize:     1024,
				MaxMessages: 0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffer, err := NewMessageBuffer(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewMessageBuffer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && buffer == nil {
				t.Error("NewMessageBuffer() returned nil without error")
			}
		})
	}
}

func TestMessageBuffer_Lifecycle(t *testing.T) {
	buffer, err := NewMessageBuffer(nil)
	if err != nil {
		t.Fatalf("NewMessageBuffer() error = %v", err)
	}

	ctx := context.Background()

	// Start
	if err := buffer.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Should be idle initially
	if buffer.State() != BufferStateIdle {
		t.Errorf("State() = %v, want idle", buffer.State())
	}

	// Stop
	if err := buffer.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	if buffer.State() != BufferStateIdle {
		t.Errorf("State() after stop = %v, want idle", buffer.State())
	}
}

func TestMessageBuffer_Buffer(t *testing.T) {
	config := &BufferConfig{
		Enabled:     true,
		MaxSize:     1024,
		MaxMessages: 10,
	}

	buffer, err := NewMessageBuffer(config)
	if err != nil {
		t.Fatalf("NewMessageBuffer() error = %v", err)
	}

	ctx := context.Background()
	if err := buffer.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer buffer.Stop()

	// Buffer a message
	msg := &BufferedMessage{
		ID:      "test-1",
		Subject: "test.subject",
		Data:    []byte("test data"),
	}

	if err := buffer.Buffer(msg); err != nil {
		t.Errorf("Buffer() error = %v", err)
	}

	if buffer.Len() != 1 {
		t.Errorf("Len() = %d, want 1", buffer.Len())
	}

	if buffer.Size() != int64(len(msg.Data)) {
		t.Errorf("Size() = %d, want %d", buffer.Size(), len(msg.Data))
	}

	if !buffer.IsBuffering() {
		t.Error("IsBuffering() = false, want true")
	}
}

func TestMessageBuffer_BufferNilMessage(t *testing.T) {
	buffer, err := NewMessageBuffer(nil)
	if err != nil {
		t.Fatalf("NewMessageBuffer() error = %v", err)
	}

	if err := buffer.Buffer(nil); err == nil {
		t.Error("Buffer(nil) should return error")
	}
}

func TestMessageBuffer_BufferDisabled(t *testing.T) {
	config := &BufferConfig{
		Enabled:     false,
		MaxSize:     1024,
		MaxMessages: 10,
	}

	buffer, err := NewMessageBuffer(config)
	if err != nil {
		t.Fatalf("NewMessageBuffer() error = %v", err)
	}

	msg := &BufferedMessage{
		Subject: "test",
		Data:    []byte("data"),
	}

	if err := buffer.Buffer(msg); err == nil {
		t.Error("Buffer() should error when disabled")
	}
}

func TestMessageBuffer_SizeLimit(t *testing.T) {
	config := &BufferConfig{
		Enabled:     true,
		MaxSize:     100, // Very small
		MaxMessages: 100,
	}

	buffer, err := NewMessageBuffer(config)
	if err != nil {
		t.Fatalf("NewMessageBuffer() error = %v", err)
	}

	ctx := context.Background()
	if err := buffer.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer buffer.Stop()

	// Buffer messages until we hit size limit
	for i := 0; i < 5; i++ {
		msg := &BufferedMessage{
			ID:      "test-" + string(rune('a'+i)),
			Subject: "test",
			Data:    make([]byte, 30), // 30 bytes each
		}
		_ = buffer.Buffer(msg)
	}

	// Should have dropped old messages to stay under limit
	if buffer.Size() > config.MaxSize {
		t.Errorf("Size() = %d, exceeds MaxSize %d", buffer.Size(), config.MaxSize)
	}
}

func TestMessageBuffer_MessageLimit(t *testing.T) {
	config := &BufferConfig{
		Enabled:     true,
		MaxSize:     1024 * 1024,
		MaxMessages: 3, // Only allow 3 messages
	}

	buffer, err := NewMessageBuffer(config)
	if err != nil {
		t.Fatalf("NewMessageBuffer() error = %v", err)
	}

	ctx := context.Background()
	if err := buffer.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer buffer.Stop()

	// Buffer more messages than allowed
	for i := 0; i < 5; i++ {
		msg := &BufferedMessage{
			ID:      "test-" + string(rune('a'+i)),
			Subject: "test",
			Data:    []byte("data"),
		}
		_ = buffer.Buffer(msg)
	}

	// Should only have 3 messages
	if buffer.Len() != 3 {
		t.Errorf("Len() = %d, want 3", buffer.Len())
	}
}

func TestMessageBuffer_Deduplication(t *testing.T) {
	config := &BufferConfig{
		Enabled:             true,
		MaxSize:             1024,
		MaxMessages:         100,
		DeduplicationWindow: 1 * time.Minute,
	}

	buffer, err := NewMessageBuffer(config)
	if err != nil {
		t.Fatalf("NewMessageBuffer() error = %v", err)
	}

	ctx := context.Background()
	if err := buffer.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer buffer.Stop()

	// Buffer same message twice
	msg1 := &BufferedMessage{
		ID:      "duplicate-id",
		Subject: "test",
		Data:    []byte("data1"),
	}

	msg2 := &BufferedMessage{
		ID:      "duplicate-id", // Same ID
		Subject: "test",
		Data:    []byte("data2"),
	}

	_ = buffer.Buffer(msg1)
	_ = buffer.Buffer(msg2)

	// Should only have one message due to deduplication
	if buffer.Len() != 1 {
		t.Errorf("Len() = %d, want 1 (deduplication)", buffer.Len())
	}
}

func TestMessageBuffer_Clear(t *testing.T) {
	buffer, err := NewMessageBuffer(nil)
	if err != nil {
		t.Fatalf("NewMessageBuffer() error = %v", err)
	}

	ctx := context.Background()
	if err := buffer.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer buffer.Stop()

	// Add some messages
	for i := 0; i < 5; i++ {
		msg := &BufferedMessage{
			Subject: "test",
			Data:    []byte("data"),
		}
		_ = buffer.Buffer(msg)
	}

	if buffer.Len() != 5 {
		t.Fatalf("Len() = %d, want 5", buffer.Len())
	}

	// Clear
	buffer.Clear()

	if buffer.Len() != 0 {
		t.Errorf("Len() after Clear() = %d, want 0", buffer.Len())
	}
	if buffer.Size() != 0 {
		t.Errorf("Size() after Clear() = %d, want 0", buffer.Size())
	}
	if buffer.State() != BufferStateIdle {
		t.Errorf("State() after Clear() = %v, want idle", buffer.State())
	}
}

func TestMessageBuffer_GetStats(t *testing.T) {
	config := &BufferConfig{
		Enabled:     true,
		MaxSize:     1024,
		MaxMessages: 100,
	}

	buffer, err := NewMessageBuffer(config)
	if err != nil {
		t.Fatalf("NewMessageBuffer() error = %v", err)
	}

	ctx := context.Background()
	if err := buffer.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer buffer.Stop()

	// Add some messages
	for i := 0; i < 3; i++ {
		msg := &BufferedMessage{
			Subject: "test",
			Data:    []byte("test data"),
		}
		_ = buffer.Buffer(msg)
	}

	stats := buffer.GetStats()
	if stats == nil {
		t.Fatal("GetStats() = nil")
	}

	if stats.MessageCount != 3 {
		t.Errorf("stats.MessageCount = %d, want 3", stats.MessageCount)
	}
	if stats.TotalAdded != 3 {
		t.Errorf("stats.TotalAdded = %d, want 3", stats.TotalAdded)
	}
	if stats.MaxSize != config.MaxSize {
		t.Errorf("stats.MaxSize = %d, want %d", stats.MaxSize, config.MaxSize)
	}
}

func TestMessageBuffer_Callbacks(t *testing.T) {
	buffer, err := NewMessageBuffer(nil)
	if err != nil {
		t.Fatalf("NewMessageBuffer() error = %v", err)
	}

	var bufferFullCalls atomic.Int32
	var messageSentCalls atomic.Int32
	var messageErrorCalls atomic.Int32

	buffer.SetBufferFullCallback(func(subject string, size int64) {
		bufferFullCalls.Add(1)
	})

	buffer.SetMessageSentCallback(func(msg *BufferedMessage) {
		messageSentCalls.Add(1)
	})

	buffer.SetMessageErrorCallback(func(msg *BufferedMessage, err error) {
		messageErrorCalls.Add(1)
	})

	// Callbacks should be set (we can't easily test they're called without a connection)
	// Just verify no panic when setting
}

func TestMessageBuffer_OversizedMessage(t *testing.T) {
	config := &BufferConfig{
		Enabled:     true,
		MaxSize:     100, // Very small max size
		MaxMessages: 100,
	}

	buffer, err := NewMessageBuffer(config)
	if err != nil {
		t.Fatalf("NewMessageBuffer() error = %v", err)
	}

	msg := &BufferedMessage{
		Subject: "test",
		Data:    make([]byte, 200), // Larger than MaxSize
	}

	if err := buffer.Buffer(msg); err == nil {
		t.Error("Buffer() should error for oversized message")
	}
}

func TestBufferedMessage(t *testing.T) {
	msg := BufferedMessage{
		ID:        "test-id",
		Subject:   "test.subject",
		Data:      []byte("test data"),
		Timestamp: time.Now(),
		Header: map[string][]string{
			"X-Custom": {"value1", "value2"},
		},
		RetryCount: 3,
	}

	if msg.ID != "test-id" {
		t.Errorf("ID = %s, want test-id", msg.ID)
	}
	if msg.Subject != "test.subject" {
		t.Errorf("Subject = %s, want test.subject", msg.Subject)
	}
	if string(msg.Data) != "test data" {
		t.Errorf("Data = %s, want 'test data'", string(msg.Data))
	}
	if msg.RetryCount != 3 {
		t.Errorf("RetryCount = %d, want 3", msg.RetryCount)
	}
	if len(msg.Header["X-Custom"]) != 2 {
		t.Errorf("Header[X-Custom] length = %d, want 2", len(msg.Header["X-Custom"]))
	}
}

func TestBufferStats(t *testing.T) {
	stats := BufferStats{
		State:        BufferStateBuffering,
		MessageCount: 10,
		CurrentSize:  1024,
		MaxSize:      65536,
		TotalAdded:   100,
		TotalSent:    80,
		TotalFailed:  5,
		DroppedOld:   10,
		DroppedSize:  5,
	}

	if stats.State != BufferStateBuffering {
		t.Errorf("State = %v, want buffering", stats.State)
	}
	if stats.MessageCount != 10 {
		t.Errorf("MessageCount = %d, want 10", stats.MessageCount)
	}
	if stats.TotalSent != 80 {
		t.Errorf("TotalSent = %d, want 80", stats.TotalSent)
	}
	if stats.TotalFailed != 5 {
		t.Errorf("TotalFailed = %d, want 5", stats.TotalFailed)
	}
}

func TestLeafBufferManager_Creation(t *testing.T) {
	leafConfig := &LeafNodeConfig{
		Role: LeafNodeRoleHub,
		Port: 7422,
	}

	bufferConfig := DefaultBufferConfig()

	manager, err := NewLeafBufferManager(leafConfig, bufferConfig)
	if err != nil {
		t.Fatalf("NewLeafBufferManager() error = %v", err)
	}

	if manager.buffer == nil {
		t.Error("buffer is nil")
	}

	if !manager.autoFlush {
		t.Error("autoFlush = false, want true")
	}
}

func TestLeafBufferManager_SetAutoFlush(t *testing.T) {
	leafConfig := &LeafNodeConfig{
		Role: LeafNodeRoleHub,
		Port: 7422,
	}

	manager, err := NewLeafBufferManager(leafConfig, nil)
	if err != nil {
		t.Fatalf("NewLeafBufferManager() error = %v", err)
	}

	// Should be true by default
	if !manager.autoFlush {
		t.Error("initial autoFlush = false, want true")
	}

	manager.SetAutoFlush(false)
	if manager.autoFlush {
		t.Error("autoFlush after SetAutoFlush(false) = true, want false")
	}

	manager.SetAutoFlush(true)
	if !manager.autoFlush {
		t.Error("autoFlush after SetAutoFlush(true) = false, want true")
	}
}

func TestLeafBufferManager_GetBuffer(t *testing.T) {
	leafConfig := &LeafNodeConfig{
		Role: LeafNodeRoleHub,
		Port: 7422,
	}

	manager, err := NewLeafBufferManager(leafConfig, nil)
	if err != nil {
		t.Fatalf("NewLeafBufferManager() error = %v", err)
	}

	buffer := manager.GetBuffer()
	if buffer == nil {
		t.Error("GetBuffer() = nil")
	}
}

func TestLeafBufferManager_Lifecycle(t *testing.T) {
	leafConfig := &LeafNodeConfig{
		Role: LeafNodeRoleHub,
		Port: 7422,
	}

	manager, err := NewLeafBufferManager(leafConfig, nil)
	if err != nil {
		t.Fatalf("NewLeafBufferManager() error = %v", err)
	}

	ctx := context.Background()

	// Start
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Verify both leaf manager and buffer are running
	if !manager.IsRunning() {
		t.Error("IsRunning() = false, want true")
	}

	// Stop
	if err := manager.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	if manager.IsRunning() {
		t.Error("IsRunning() after Stop() = true, want false")
	}
}

func TestLeafBufferManager_Publish_Connected(t *testing.T) {
	leafConfig := &LeafNodeConfig{
		Role: LeafNodeRoleHub,
		Port: 7422,
	}

	manager, err := NewLeafBufferManager(leafConfig, nil)
	if err != nil {
		t.Fatalf("NewLeafBufferManager() error = %v", err)
	}

	ctx := context.Background()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer manager.Stop()

	// Publish when connected - should go directly
	err = manager.Publish("test.subject", []byte("test data"))
	if err != nil {
		t.Errorf("Publish() error = %v", err)
	}

	// Buffer should be empty since we're connected
	if manager.GetBuffer().Len() != 0 {
		t.Errorf("Buffer.Len() = %d, want 0 (message should go directly)", manager.GetBuffer().Len())
	}
}

func TestLeafBufferManager_Publish_Disconnected(t *testing.T) {
	leafConfig := &LeafNodeConfig{
		Role: LeafNodeRoleLeaf,
		Remotes: []LeafRemoteConfig{
			{URLs: []string{"nats://nonexistent:7422"}},
		},
	}

	bufferConfig := DefaultBufferConfig()

	manager, err := NewLeafBufferManager(leafConfig, bufferConfig)
	if err != nil {
		t.Fatalf("NewLeafBufferManager() error = %v", err)
	}

	// Start the buffer but not the leaf manager (simulating disconnected state)
	ctx := context.Background()
	if err := manager.GetBuffer().Start(ctx); err != nil {
		t.Fatalf("buffer.Start() error = %v", err)
	}
	defer manager.GetBuffer().Stop()

	// Publish when disconnected - should buffer
	err = manager.Publish("test.subject", []byte("test data"))
	if err != nil {
		t.Errorf("Publish() error = %v", err)
	}

	// Message should be buffered
	if manager.GetBuffer().Len() != 1 {
		t.Errorf("Buffer.Len() = %d, want 1 (message should be buffered)", manager.GetBuffer().Len())
	}
}

func TestGenerateBufferMessageID(t *testing.T) {
	id1 := generateBufferMessageID()
	id2 := generateBufferMessageID()

	if id1 == "" {
		t.Error("generateBufferMessageID() returned empty string")
	}

	if id1 == id2 {
		t.Error("generateBufferMessageID() returned same ID twice")
	}
}

func TestMessageBuffer_FlushNoConnection(t *testing.T) {
	buffer, err := NewMessageBuffer(nil)
	if err != nil {
		t.Fatalf("NewMessageBuffer() error = %v", err)
	}

	ctx := context.Background()
	if err := buffer.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer buffer.Stop()

	// Try to flush with nil connection
	err = buffer.Flush(nil)
	if err == nil {
		t.Error("Flush(nil) should return error")
	}
}

func TestMessageBuffer_FlushBatchNoConnection(t *testing.T) {
	buffer, err := NewMessageBuffer(nil)
	if err != nil {
		t.Fatalf("NewMessageBuffer() error = %v", err)
	}

	// Try to flush batch with nil connection
	_, err = buffer.FlushBatch(nil, 10)
	if err == nil {
		t.Error("FlushBatch(nil) should return error")
	}
}

func TestMessageBuffer_FlushBatchEmpty(t *testing.T) {
	buffer, err := NewMessageBuffer(nil)
	if err != nil {
		t.Fatalf("NewMessageBuffer() error = %v", err)
	}

	ctx := context.Background()
	if err := buffer.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer buffer.Stop()

	// Create a real NATS connection for testing
	leafConfig := &LeafNodeConfig{
		Role: LeafNodeRoleHub,
		Port: 7422,
	}

	manager, err := NewLeafNodeManager(leafConfig)
	if err != nil {
		t.Fatalf("NewLeafNodeManager() error = %v", err)
	}

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("manager.Start() error = %v", err)
	}
	defer manager.Stop()

	client := manager.GetClient()
	if client == nil {
		t.Fatal("GetClient() = nil")
	}

	// Flush empty buffer
	n, err := buffer.FlushBatch(client, 10)
	if err != nil {
		t.Errorf("FlushBatch() error = %v", err)
	}
	if n != 0 {
		t.Errorf("FlushBatch() = %d, want 0", n)
	}
}

func TestBufferConfig(t *testing.T) {
	config := BufferConfig{
		Enabled:             true,
		MaxSize:             1024 * 1024,
		MaxMessages:         5000,
		MaxAge:              2 * time.Hour,
		PersistToDisk:       true,
		StreamName:          "MY_BUFFER",
		FlushInterval:       10 * time.Second,
		FlushBatchSize:      50,
		RetryDelay:          2 * time.Second,
		DeduplicationWindow: 5 * time.Minute,
	}

	if !config.Enabled {
		t.Error("Enabled = false, want true")
	}
	if config.MaxSize != 1024*1024 {
		t.Errorf("MaxSize = %d, want 1048576", config.MaxSize)
	}
	if config.MaxMessages != 5000 {
		t.Errorf("MaxMessages = %d, want 5000", config.MaxMessages)
	}
	if config.MaxAge != 2*time.Hour {
		t.Errorf("MaxAge = %v, want 2h", config.MaxAge)
	}
	if !config.PersistToDisk {
		t.Error("PersistToDisk = false, want true")
	}
	if config.StreamName != "MY_BUFFER" {
		t.Errorf("StreamName = %s, want MY_BUFFER", config.StreamName)
	}
}
