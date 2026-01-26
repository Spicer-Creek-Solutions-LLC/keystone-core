package events

import (
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/logging"
	"github.com/shawnbutts/keystone-core/internal/metrics"
)

func TestDefaultTelemetryConfig(t *testing.T) {
	cfg := DefaultTelemetryConfig()

	if cfg.NATSURL != "nats://localhost:4222" {
		t.Errorf("expected NATSURL nats://localhost:4222, got %s", cfg.NATSURL)
	}

	if cfg.LogSubject != "kscore.logs.>" {
		t.Errorf("expected log subject kscore.logs.>, got %s", cfg.LogSubject)
	}

	if cfg.MetricSubject != "kscore.metrics.>" {
		t.Errorf("expected metric subject kscore.metrics.>, got %s", cfg.MetricSubject)
	}

	if cfg.TraceSubject != "kscore.traces.>" {
		t.Errorf("expected trace subject kscore.traces.>, got %s", cfg.TraceSubject)
	}

	if cfg.AuditSubject != "kscore.audit.>" {
		t.Errorf("expected audit subject kscore.audit.>, got %s", cfg.AuditSubject)
	}
}

func TestLogBuffer(t *testing.T) {
	buf := NewLogBuffer(10)

	if buf.Count() != 0 {
		t.Errorf("expected count 0, got %d", buf.Count())
	}

	// Add some logs
	buf.Add(&logging.NATSLogMessage{Level: "info", Message: "msg1"})
	buf.Add(&logging.NATSLogMessage{Level: "error", Message: "msg2"})
	buf.Add(&logging.NATSLogMessage{Level: "info", Message: "msg3"})

	if buf.Count() != 3 {
		t.Errorf("expected count 3, got %d", buf.Count())
	}

	// Test All()
	all := buf.All()
	if len(all) != 3 {
		t.Errorf("expected 3 logs, got %d", len(all))
	}

	// Test Last()
	last := buf.Last(2)
	if len(last) != 2 {
		t.Errorf("expected 2 logs, got %d", len(last))
	}
	if last[0].Message != "msg2" {
		t.Errorf("expected msg2, got %s", last[0].Message)
	}
}

func TestLogBufferFilterByLevel(t *testing.T) {
	buf := NewLogBuffer(100)

	buf.Add(&logging.NATSLogMessage{Level: "info", Message: "info1"})
	buf.Add(&logging.NATSLogMessage{Level: "error", Message: "error1"})
	buf.Add(&logging.NATSLogMessage{Level: "info", Message: "info2"})

	filtered := buf.FilterByLevel("info")
	if len(filtered) != 2 {
		t.Errorf("expected 2 info logs, got %d", len(filtered))
	}

	filtered = buf.FilterByLevel("error")
	if len(filtered) != 1 {
		t.Errorf("expected 1 error log, got %d", len(filtered))
	}
}

func TestLogBufferClear(t *testing.T) {
	buf := NewLogBuffer(100)

	buf.Add(&logging.NATSLogMessage{Level: "info", Message: "msg1"})
	buf.Add(&logging.NATSLogMessage{Level: "info", Message: "msg2"})

	if buf.Count() != 2 {
		t.Errorf("expected count 2, got %d", buf.Count())
	}

	buf.Clear()

	if buf.Count() != 0 {
		t.Errorf("expected count 0 after clear, got %d", buf.Count())
	}
}

func TestLogBufferMaxSize(t *testing.T) {
	buf := NewLogBuffer(3)

	buf.Add(&logging.NATSLogMessage{Level: "info", Message: "msg1"})
	buf.Add(&logging.NATSLogMessage{Level: "info", Message: "msg2"})
	buf.Add(&logging.NATSLogMessage{Level: "info", Message: "msg3"})
	buf.Add(&logging.NATSLogMessage{Level: "info", Message: "msg4"})

	if buf.Count() != 3 {
		t.Errorf("expected count 3 (max size), got %d", buf.Count())
	}

	logs := buf.All()
	// msg1 should be evicted
	if logs[0].Message != "msg2" {
		t.Errorf("expected first message to be msg2, got %s", logs[0].Message)
	}
}

func TestLogBufferDefaultSize(t *testing.T) {
	buf := NewLogBuffer(0)
	if buf.maxSize != 1000 {
		t.Errorf("expected default max size 1000, got %d", buf.maxSize)
	}
}

func TestLogBufferLastEdgeCases(t *testing.T) {
	buf := NewLogBuffer(10)
	buf.Add(&logging.NATSLogMessage{Message: "msg1"})
	buf.Add(&logging.NATSLogMessage{Message: "msg2"})

	// Request more than available
	last := buf.Last(10)
	if len(last) != 2 {
		t.Errorf("expected 2 logs, got %d", len(last))
	}

	// Request 0 or negative
	last = buf.Last(0)
	if len(last) != 2 {
		t.Errorf("expected 2 logs for 0 request, got %d", len(last))
	}
}

func TestMetricBuffer(t *testing.T) {
	buf := NewMetricBuffer()

	if buf.Count() != 0 {
		t.Errorf("expected count 0, got %d", buf.Count())
	}

	// Add some metrics
	buf.Add(&metrics.NATSMetricMessage{
		Service:   "service1",
		Name:      "metric1",
		Type:      "counter",
		Value:     100,
		Timestamp: time.Now().Format(time.RFC3339),
	})

	buf.Add(&metrics.NATSMetricMessage{
		Service:   "service1",
		Name:      "metric2",
		Type:      "gauge",
		Value:     50,
		Timestamp: time.Now().Format(time.RFC3339),
	})

	buf.Add(&metrics.NATSMetricMessage{
		Service:   "service2",
		Name:      "metric1",
		Type:      "counter",
		Value:     200,
		Timestamp: time.Now().Format(time.RFC3339),
	})

	if buf.Count() != 3 {
		t.Errorf("expected count 3, got %d", buf.Count())
	}

	// Test Get()
	m := buf.Get("service1", "metric1")
	if m == nil {
		t.Fatal("expected metric, got nil")
	}
	if m.Value != 100 {
		t.Errorf("expected value 100, got %f", m.Value)
	}
}

func TestMetricBufferUpdate(t *testing.T) {
	buf := NewMetricBuffer()

	buf.Add(&metrics.NATSMetricMessage{
		Service: "svc",
		Name:    "m1",
		Value:   10,
	})

	// Update same metric
	buf.Add(&metrics.NATSMetricMessage{
		Service: "svc",
		Name:    "m1",
		Value:   20,
	})

	// Should still have 1 metric (updated)
	if buf.Count() != 1 {
		t.Errorf("expected count 1 after update, got %d", buf.Count())
	}

	m := buf.Get("svc", "m1")
	if m.Value != 20 {
		t.Errorf("expected updated value 20, got %f", m.Value)
	}
}

func TestMetricBufferFilterByService(t *testing.T) {
	buf := NewMetricBuffer()

	buf.Add(&metrics.NATSMetricMessage{Service: "svc1", Name: "m1"})
	buf.Add(&metrics.NATSMetricMessage{Service: "svc1", Name: "m2"})
	buf.Add(&metrics.NATSMetricMessage{Service: "svc2", Name: "m1"})

	filtered := buf.FilterByService("svc1")
	if len(filtered) != 2 {
		t.Errorf("expected 2 metrics for svc1, got %d", len(filtered))
	}

	filtered = buf.FilterByService("svc2")
	if len(filtered) != 1 {
		t.Errorf("expected 1 metric for svc2, got %d", len(filtered))
	}

	filtered = buf.FilterByService("unknown")
	if len(filtered) != 0 {
		t.Errorf("expected 0 metrics for unknown, got %d", len(filtered))
	}
}

func TestMetricBufferFilterByType(t *testing.T) {
	buf := NewMetricBuffer()

	buf.Add(&metrics.NATSMetricMessage{Service: "svc", Name: "m1", Type: "counter"})
	buf.Add(&metrics.NATSMetricMessage{Service: "svc", Name: "m2", Type: "gauge"})
	buf.Add(&metrics.NATSMetricMessage{Service: "svc", Name: "m3", Type: "counter"})

	filtered := buf.FilterByType("counter")
	if len(filtered) != 2 {
		t.Errorf("expected 2 counters, got %d", len(filtered))
	}

	filtered = buf.FilterByType("gauge")
	if len(filtered) != 1 {
		t.Errorf("expected 1 gauge, got %d", len(filtered))
	}
}

func TestMetricBufferClear(t *testing.T) {
	buf := NewMetricBuffer()

	buf.Add(&metrics.NATSMetricMessage{Service: "svc", Name: "m1"})
	buf.Add(&metrics.NATSMetricMessage{Service: "svc", Name: "m2"})

	if buf.Count() != 2 {
		t.Errorf("expected count 2, got %d", buf.Count())
	}

	buf.Clear()

	if buf.Count() != 0 {
		t.Errorf("expected count 0 after clear, got %d", buf.Count())
	}
}

func TestMetricBufferAll(t *testing.T) {
	buf := NewMetricBuffer()

	buf.Add(&metrics.NATSMetricMessage{Service: "svc", Name: "m1"})
	buf.Add(&metrics.NATSMetricMessage{Service: "svc", Name: "m2"})

	all := buf.All()
	if len(all) != 2 {
		t.Errorf("expected 2 metrics, got %d", len(all))
	}
}

func TestMetricBufferGetNonExistent(t *testing.T) {
	buf := NewMetricBuffer()

	m := buf.Get("nonexistent", "metric")
	if m != nil {
		t.Errorf("expected nil for nonexistent metric, got %v", m)
	}
}

func TestLogMsg(t *testing.T) {
	msg := LogMsg{
		Log: &logging.NATSLogMessage{
			Level:   "info",
			Message: "test",
		},
	}

	if msg.Log.Level != "info" {
		t.Errorf("expected level info, got %s", msg.Log.Level)
	}
}

func TestMetricMsg(t *testing.T) {
	msg := MetricMsg{
		Metric: &metrics.NATSMetricMessage{
			Name:  "test",
			Value: 42,
		},
	}

	if msg.Metric.Value != 42 {
		t.Errorf("expected value 42, got %f", msg.Metric.Value)
	}
}
