package ordering

import (
	"context"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Mode != ModePerPartition {
		t.Errorf("expected per_partition mode, got %s", cfg.Mode)
	}
	if cfg.WindowSize != 1 {
		t.Errorf("expected window size 1, got %d", cfg.WindowSize)
	}
	if cfg.AckTimeout != 5*time.Second {
		t.Errorf("expected 5s ack timeout, got %v", cfg.AckTimeout)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				Mode:         ModePerSubject,
				WindowSize:   1,
				AckTimeout:   time.Second,
				PartitionKey: PartitionBySubject,
			},
			wantErr: false,
		},
		{
			name: "missing partition key for per-partition mode",
			config: Config{
				Mode:       ModePerPartition,
				WindowSize: 1,
				AckTimeout: time.Second,
			},
			wantErr: true,
		},
		{
			name: "zero window size",
			config: Config{
				Mode:         ModePerSubject,
				WindowSize:   0,
				AckTimeout:   time.Second,
				PartitionKey: PartitionBySubject,
			},
			wantErr: true,
		},
		{
			name: "negative ack timeout",
			config: Config{
				Mode:         ModePerSubject,
				WindowSize:   1,
				AckTimeout:   -time.Second,
				PartitionKey: PartitionBySubject,
			},
			wantErr: true,
		},
		{
			name: "no ordering mode accepts nil partition key",
			config: Config{
				Mode:       ModeNone,
				WindowSize: 1,
				AckTimeout: time.Second,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPartitionKeyFunctions(t *testing.T) {
	t.Run("PartitionBySubject", func(t *testing.T) {
		key := PartitionBySubject("events.agent.status", nil, nil)
		if key != "events.agent.status" {
			t.Errorf("expected subject as key, got %s", key)
		}
	})

	t.Run("PartitionByAgentID", func(t *testing.T) {
		headers := map[string]string{"agent-id": "agent-123"}
		key := PartitionByAgentID("", nil, headers)
		if key != "agent-123" {
			t.Errorf("expected agent-123, got %s", key)
		}

		// Test default when no agent-id
		key = PartitionByAgentID("", nil, nil)
		if key != "default" {
			t.Errorf("expected default, got %s", key)
		}
	})

	t.Run("PartitionByCorrelationID", func(t *testing.T) {
		headers := map[string]string{"correlation-id": "req-456"}
		key := PartitionByCorrelationID("", nil, headers)
		if key != "req-456" {
			t.Errorf("expected req-456, got %s", key)
		}

		// Test default when no correlation-id
		key = PartitionByCorrelationID("", nil, nil)
		if key != "default" {
			t.Errorf("expected default, got %s", key)
		}
	})
}

func TestOrderedPublisher(t *testing.T) {
	t.Run("create with valid config", func(t *testing.T) {
		cfg := Config{
			Mode:         ModePerSubject,
			PartitionKey: PartitionBySubject,
			WindowSize:   1,
			AckTimeout:   time.Second,
		}

		pub, err := NewOrderedPublisher(nil, cfg)
		if err != nil {
			t.Fatalf("NewOrderedPublisher failed: %v", err)
		}
		defer pub.Close()

		if !pub.running.Load() {
			t.Error("publisher should be running")
		}
	})

	t.Run("create with invalid config", func(t *testing.T) {
		cfg := Config{
			Mode:       ModePerPartition,
			WindowSize: 1,
			AckTimeout: time.Second,
			// Missing PartitionKey
		}

		_, err := NewOrderedPublisher(nil, cfg)
		if err == nil {
			t.Error("expected error for invalid config")
		}
	})

	t.Run("publish without connection returns error", func(t *testing.T) {
		cfg := Config{
			Mode:         ModeNone,
			WindowSize:   1,
			AckTimeout:   time.Second,
			PartitionKey: PartitionBySubject,
		}

		pub, err := NewOrderedPublisher(nil, cfg)
		if err != nil {
			t.Fatalf("NewOrderedPublisher failed: %v", err)
		}
		defer pub.Close()

		ctx := context.Background()
		err = pub.Publish(ctx, "test.subject", []byte("data"), nil)
		if err == nil {
			t.Error("expected error when publishing without connection")
		}
	})

	t.Run("close stops publisher", func(t *testing.T) {
		cfg := Config{
			Mode:         ModePerSubject,
			PartitionKey: PartitionBySubject,
			WindowSize:   1,
			AckTimeout:   time.Second,
		}

		pub, err := NewOrderedPublisher(nil, cfg)
		if err != nil {
			t.Fatalf("NewOrderedPublisher failed: %v", err)
		}

		err = pub.Close()
		if err != nil {
			t.Errorf("Close failed: %v", err)
		}

		if pub.running.Load() {
			t.Error("publisher should not be running after close")
		}
	})
}

func TestOrderedPublisherPartitioning(t *testing.T) {
	cfg := Config{
		Mode:         ModePerPartition,
		PartitionKey: PartitionByAgentID,
		WindowSize:   10,
		AckTimeout:   time.Second,
	}

	pub, err := NewOrderedPublisher(nil, cfg)
	if err != nil {
		t.Fatalf("NewOrderedPublisher failed: %v", err)
	}
	defer pub.Close()

	// Create partitions
	state1 := pub.getOrCreatePartition("agent-1")
	state2 := pub.getOrCreatePartition("agent-2")
	state1Again := pub.getOrCreatePartition("agent-1")

	if state1 != state1Again {
		t.Error("same partition key should return same state")
	}
	if state1 == state2 {
		t.Error("different partition keys should return different states")
	}

	// Check partition count
	if pub.stats.PartitionCount.Load() != 2 {
		t.Errorf("expected 2 partitions, got %d", pub.stats.PartitionCount.Load())
	}
}

func TestSequenceTracker(t *testing.T) {
	t.Run("track sequential", func(t *testing.T) {
		tracker := NewSequenceTracker(10)

		gaps := tracker.Track("p1", 1)
		if len(gaps) != 0 {
			t.Errorf("expected no gaps, got %v", gaps)
		}

		gaps = tracker.Track("p1", 2)
		if len(gaps) != 0 {
			t.Errorf("expected no gaps, got %v", gaps)
		}

		gaps = tracker.Track("p1", 3)
		if len(gaps) != 0 {
			t.Errorf("expected no gaps, got %v", gaps)
		}
	})

	t.Run("detect gaps", func(t *testing.T) {
		tracker := NewSequenceTracker(10)

		tracker.Track("p1", 1)
		gaps := tracker.Track("p1", 5)

		if len(gaps) != 3 {
			t.Errorf("expected 3 gaps (2,3,4), got %v", gaps)
		}

		// Verify gap values
		expected := []uint64{2, 3, 4}
		for i, g := range gaps {
			if g != expected[i] {
				t.Errorf("expected gap %d, got %d", expected[i], g)
			}
		}
	})

	t.Run("max gap limit", func(t *testing.T) {
		tracker := NewSequenceTracker(2)

		tracker.Track("p1", 1)
		gaps := tracker.Track("p1", 100)

		// Should only report maxGap gaps
		if len(gaps) != 2 {
			t.Errorf("expected 2 gaps (maxGap limit), got %d", len(gaps))
		}
	})

	t.Run("get last sequence", func(t *testing.T) {
		tracker := NewSequenceTracker(10)

		tracker.Track("p1", 5)
		tracker.Track("p1", 10)

		last := tracker.GetLastSequence("p1")
		if last != 10 {
			t.Errorf("expected last sequence 10, got %d", last)
		}

		// Unknown partition
		last = tracker.GetLastSequence("unknown")
		if last != 0 {
			t.Errorf("expected 0 for unknown partition, got %d", last)
		}
	})

	t.Run("get gaps", func(t *testing.T) {
		tracker := NewSequenceTracker(10)

		tracker.Track("p1", 1)
		tracker.Track("p1", 5)

		gaps := tracker.GetGaps("p1")
		if len(gaps) != 3 {
			t.Errorf("expected 3 gaps, got %v", gaps)
		}

		// Unknown partition
		gaps = tracker.GetGaps("unknown")
		if gaps != nil {
			t.Errorf("expected nil for unknown partition, got %v", gaps)
		}
	})

	t.Run("clear gap", func(t *testing.T) {
		tracker := NewSequenceTracker(10)

		tracker.Track("p1", 1)
		tracker.Track("p1", 5) // Creates gaps 2, 3, 4

		tracker.ClearGap("p1", 3)

		gaps := tracker.GetGaps("p1")
		if len(gaps) != 2 {
			t.Errorf("expected 2 gaps after clearing 3, got %v", gaps)
		}

		// Verify 3 is not in gaps
		for _, g := range gaps {
			if g == 3 {
				t.Error("gap 3 should have been cleared")
			}
		}
	})

	t.Run("multiple partitions", func(t *testing.T) {
		tracker := NewSequenceTracker(10)

		tracker.Track("p1", 1)
		tracker.Track("p2", 1)
		tracker.Track("p1", 5)
		tracker.Track("p2", 2)

		gaps1 := tracker.GetGaps("p1")
		gaps2 := tracker.GetGaps("p2")

		if len(gaps1) != 3 {
			t.Errorf("p1 expected 3 gaps, got %v", gaps1)
		}
		if len(gaps2) != 0 {
			t.Errorf("p2 expected 0 gaps, got %v", gaps2)
		}
	})
}

func TestDefaultConsumerConfig(t *testing.T) {
	cfg := DefaultConsumerConfig()

	if cfg.MaxAckPending != 1 {
		t.Errorf("expected max ack pending 1, got %d", cfg.MaxAckPending)
	}
	if cfg.AckWait != 30*time.Second {
		t.Errorf("expected 30s ack wait, got %v", cfg.AckWait)
	}
	if cfg.MaxDeliver != 5 {
		t.Errorf("expected max deliver 5, got %d", cfg.MaxDeliver)
	}
	if !cfg.SequenceValidation {
		t.Error("expected sequence validation enabled")
	}
}

func TestConsumerConfigValidation(t *testing.T) {
	handler := func(ctx context.Context, msg *OrderedMessage) error {
		return nil
	}

	tests := []struct {
		name    string
		config  ConsumerConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: ConsumerConfig{
				Stream:        "TEST",
				MaxAckPending: 1,
				Handler:       handler,
			},
			wantErr: false,
		},
		{
			name: "missing stream",
			config: ConsumerConfig{
				MaxAckPending: 1,
				Handler:       handler,
			},
			wantErr: true,
		},
		{
			name: "zero max ack pending",
			config: ConsumerConfig{
				Stream:        "TEST",
				MaxAckPending: 0,
				Handler:       handler,
			},
			wantErr: true,
		},
		{
			name: "missing handler",
			config: ConsumerConfig{
				Stream:        "TEST",
				MaxAckPending: 1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestOrderedMessage(t *testing.T) {
	msg := &OrderedMessage{
		Subject:      "events.agent.status",
		Data:         []byte("test data"),
		Headers:      map[string]string{"agent-id": "agent-123"},
		PartitionKey: "agent-123",
		Sequence:     42,
		Timestamp:    time.Now(),
	}

	if msg.Subject != "events.agent.status" {
		t.Errorf("unexpected subject: %s", msg.Subject)
	}
	if string(msg.Data) != "test data" {
		t.Errorf("unexpected data: %s", msg.Data)
	}
	if msg.PartitionKey != "agent-123" {
		t.Errorf("unexpected partition key: %s", msg.PartitionKey)
	}
	if msg.Sequence != 42 {
		t.Errorf("unexpected sequence: %d", msg.Sequence)
	}
}

func TestModeConstants(t *testing.T) {
	modes := []Mode{
		ModeNone,
		ModePerSubject,
		ModePerPartition,
		ModeGlobal,
	}

	expected := []string{"none", "per_subject", "per_partition", "global"}

	for i, mode := range modes {
		if string(mode) != expected[i] {
			t.Errorf("expected %s, got %s", expected[i], mode)
		}
	}
}

func TestReplayPolicyConstants(t *testing.T) {
	if string(ReplayInstant) != "instant" {
		t.Errorf("expected instant, got %s", ReplayInstant)
	}
	if string(ReplayOriginal) != "original" {
		t.Errorf("expected original, got %s", ReplayOriginal)
	}
}

func TestPublisherStats(t *testing.T) {
	cfg := Config{
		Mode:         ModePerSubject,
		PartitionKey: PartitionBySubject,
		WindowSize:   1,
		AckTimeout:   time.Second,
	}

	pub, err := NewOrderedPublisher(nil, cfg)
	if err != nil {
		t.Fatalf("NewOrderedPublisher failed: %v", err)
	}
	defer pub.Close()

	// Stats should start at zero
	stats := pub.GetStats()
	if stats.TotalPublished.Load() != 0 {
		t.Errorf("expected 0 published, got %d", stats.TotalPublished.Load())
	}
	if stats.TotalFailed.Load() != 0 {
		t.Errorf("expected 0 failed, got %d", stats.TotalFailed.Load())
	}
}

func TestConsumerStats(t *testing.T) {
	var stats ConsumerStats

	// Stats should start at zero
	if stats.TotalReceived.Load() != 0 {
		t.Errorf("expected 0 received, got %d", stats.TotalReceived.Load())
	}

	// Increment stats
	stats.TotalReceived.Add(1)
	stats.TotalProcessed.Add(1)

	if stats.TotalReceived.Load() != 1 {
		t.Errorf("expected 1 received, got %d", stats.TotalReceived.Load())
	}
	if stats.TotalProcessed.Load() != 1 {
		t.Errorf("expected 1 processed, got %d", stats.TotalProcessed.Load())
	}
}
