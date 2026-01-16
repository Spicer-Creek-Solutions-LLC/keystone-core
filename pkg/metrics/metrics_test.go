package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/shawnbutts/keystone-core/pkg/testing/helpers"
)

func TestPrometheusCollectorCounter(t *testing.T) {
	collector := NewPrometheusCollector()

	// Register a counter
	err := collector.RegisterMetric(MetricDefinition{
		Name:   "test_counter_total",
		Type:   MetricTypeCounter,
		Help:   "Test counter",
		Labels: []string{"label1", "label2"},
	})
	if err != nil {
		t.Fatalf("Failed to register counter: %v", err)
	}

	// Increment counter
	collector.IncCounter("test_counter_total", map[string]string{
		"label1": "value1",
		"label2": "value2",
	})

	// Add to counter
	collector.AddCounter("test_counter_total", 5.0, map[string]string{
		"label1": "value1",
		"label2": "value2",
	})

	// Verify counter value
	expected := `
		# HELP test_counter_total Test counter
		# TYPE test_counter_total counter
		test_counter_total{label1="value1",label2="value2"} 6
	`
	if err := testutil.CollectAndCompare(collector.registry, strings.NewReader(expected), "test_counter_total"); err != nil {
		t.Errorf("Counter mismatch: %v", err)
	}
}

func TestPrometheusCollectorGauge(t *testing.T) {
	collector := NewPrometheusCollector()

	// Register a gauge
	err := collector.RegisterMetric(MetricDefinition{
		Name:   "test_gauge",
		Type:   MetricTypeGauge,
		Help:   "Test gauge",
		Labels: []string{"label"},
	})
	if err != nil {
		t.Fatalf("Failed to register gauge: %v", err)
	}

	// Set gauge
	collector.SetGauge("test_gauge", 10.5, map[string]string{"label": "value"})

	// Increment gauge
	collector.IncGauge("test_gauge", map[string]string{"label": "value"})

	// Decrement gauge
	collector.DecGauge("test_gauge", map[string]string{"label": "value"})

	// Verify gauge value (10.5 + 1 - 1 = 10.5)
	expected := `
		# HELP test_gauge Test gauge
		# TYPE test_gauge gauge
		test_gauge{label="value"} 10.5
	`
	if err := testutil.CollectAndCompare(collector.registry, strings.NewReader(expected), "test_gauge"); err != nil {
		t.Errorf("Gauge mismatch: %v", err)
	}
}

func TestPrometheusCollectorHistogram(t *testing.T) {
	collector := NewPrometheusCollector()

	// Register a histogram
	err := collector.RegisterMetric(MetricDefinition{
		Name:    "test_histogram_seconds",
		Type:    MetricTypeHistogram,
		Help:    "Test histogram",
		Labels:  []string{"status"},
		Buckets: []float64{0.1, 0.5, 1.0, 5.0},
	})
	if err != nil {
		t.Fatalf("Failed to register histogram: %v", err)
	}

	// Observe values
	collector.ObserveHistogram("test_histogram_seconds", 0.05, map[string]string{"status": "success"})
	collector.ObserveHistogram("test_histogram_seconds", 0.3, map[string]string{"status": "success"})
	collector.ObserveHistogram("test_histogram_seconds", 0.7, map[string]string{"status": "success"})

	// Count buckets
	count := testutil.CollectAndCount(collector.registry, "test_histogram_seconds")
	if count == 0 {
		t.Error("Histogram not collected")
	}
}

func TestPrometheusCollectorDuration(t *testing.T) {
	collector := NewPrometheusCollector()

	// Register a histogram for duration
	err := collector.RegisterMetric(MetricDefinition{
		Name:   "test_duration_seconds",
		Type:   MetricTypeHistogram,
		Help:   "Test duration",
		Labels: []string{"operation"},
	})
	if err != nil {
		t.Fatalf("Failed to register histogram: %v", err)
	}

	// Record duration
	duration := 250 * time.Millisecond
	collector.RecordDuration("test_duration_seconds", duration, map[string]string{
		"operation": "test",
	})

	// Verify histogram was recorded
	count := testutil.CollectAndCount(collector.registry, "test_duration_seconds")
	if count == 0 {
		t.Error("Duration not recorded")
	}
}

func TestTimer(t *testing.T) {
	collector := NewPrometheusCollector()

	// Register metric
	err := collector.RegisterMetric(MetricDefinition{
		Name:   "test_operation_seconds",
		Type:   MetricTypeHistogram,
		Help:   "Test operation duration",
		Labels: []string{"status"},
	})
	if err != nil {
		t.Fatalf("Failed to register metric: %v", err)
	}

	// Create and use timer
	timer := NewTimer(collector, "test_operation_seconds", map[string]string{})
	start := time.Now()
	if err := helpers.WaitForTimeout(2*time.Second, 1*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= 10*time.Millisecond, nil
	}); err != nil {
		t.Fatalf("Timer delay did not elapse: %v", err)
	}
	timer.ObserveDurationWithLabels(map[string]string{"status": "success"})

	// Verify timer recorded
	count := testutil.CollectAndCount(collector.registry, "test_operation_seconds")
	if count == 0 {
		t.Error("Timer duration not recorded")
	}
}

func TestControlPlaneCollector(t *testing.T) {
	promCollector := NewPrometheusCollector()
	if err := InitializeStandardMetrics(promCollector); err != nil {
		t.Fatalf("Failed to initialize metrics: %v", err)
	}

	cpCollector := NewControlPlaneCollector(promCollector)

	// Record API request
	cpCollector.RecordAPIRequest("GET", "/api/v1/agents", "200", 50*time.Millisecond)

	// Set agents connected
	cpCollector.SetAgentsConnected("us-east-1", "web", 10)

	// Record agent disconnect
	cpCollector.RecordAgentDisconnect()

	// Record command execution
	cpCollector.RecordCommandExecution("success", 2*time.Second)

	// Record state application
	cpCollector.RecordStateApplication("success", 5*time.Second)

	// Record policy evaluation
	cpCollector.RecordPolicyEvaluation("security-policy", "allowed")

	// Record events
	cpCollector.RecordEventPublished("agent.connect")
	cpCollector.RecordEventProcessed("agent.connect")

	// Verify metrics were recorded
	count := testutil.CollectAndCount(promCollector.registry)
	if count == 0 {
		t.Error("No metrics collected")
	}
}

func TestAgentCollector(t *testing.T) {
	promCollector := NewPrometheusCollector()
	if err := InitializeStandardMetrics(promCollector); err != nil {
		t.Fatalf("Failed to initialize metrics: %v", err)
	}

	agentCollector := NewAgentCollector(promCollector)

	agentID := "agent-001"

	// Record heartbeat
	agentCollector.RecordHeartbeat(agentID)

	// Record resource usage
	agentCollector.RecordCPUUsage(agentID, 45.5)
	agentCollector.RecordMemoryUsage(agentID, 1024*1024*512)   // 512 MB
	agentCollector.RecordDiskUsage(agentID, 1024*1024*1024*10) // 10 GB

	// Record operations
	agentCollector.RecordCommandExecuted(agentID, "success")
	agentCollector.RecordStateApplied(agentID, "success")

	// Verify metrics
	count := testutil.CollectAndCount(promCollector.registry)
	if count == 0 {
		t.Error("No agent metrics collected")
	}
}

func TestStateCollector(t *testing.T) {
	promCollector := NewPrometheusCollector()
	if err := InitializeStandardMetrics(promCollector); err != nil {
		t.Fatalf("Failed to initialize metrics: %v", err)
	}

	stateCollector := NewStateCollector(promCollector)

	// Set resource count
	stateCollector.SetResourceCount("file", "applied", 50)

	// Record drift detection
	stateCollector.RecordDriftDetection("nginx-config")

	// Record state change
	stateCollector.RecordStateChange("file")

	// Verify metrics
	count := testutil.CollectAndCount(promCollector.registry)
	if count == 0 {
		t.Error("No state metrics collected")
	}
}

func TestGitOpsCollector(t *testing.T) {
	promCollector := NewPrometheusCollector()
	if err := InitializeStandardMetrics(promCollector); err != nil {
		t.Fatalf("Failed to initialize metrics: %v", err)
	}

	gitopsCollector := NewGitOpsCollector(promCollector)

	// Record webhook
	gitopsCollector.RecordWebhookReceived("argocd")

	// Record verification
	gitopsCollector.RecordDeploymentVerified("success")

	// Record rollback
	gitopsCollector.RecordRollbackTriggered()

	// Verify metrics
	count := testutil.CollectAndCount(promCollector.registry)
	if count == 0 {
		t.Error("No GitOps metrics collected")
	}
}

func TestPolicyCollector(t *testing.T) {
	promCollector := NewPrometheusCollector()
	if err := InitializeStandardMetrics(promCollector); err != nil {
		t.Fatalf("Failed to initialize metrics: %v", err)
	}

	policyCollector := NewPolicyCollector(promCollector)

	// Record violation
	policyCollector.RecordViolation("security-001", "high")

	// Record remediation
	policyCollector.RecordRemediation("security-001", "success")

	// Set compliance score
	policyCollector.SetComplianceScore("PCI-DSS", 95.5)

	// Verify metrics
	count := testutil.CollectAndCount(promCollector.registry)
	if count == 0 {
		t.Error("No policy metrics collected")
	}
}

func TestDuplicateMetricRegistration(t *testing.T) {
	collector := NewPrometheusCollector()

	def := MetricDefinition{
		Name:   "test_duplicate",
		Type:   MetricTypeCounter,
		Help:   "Test duplicate",
		Labels: []string{},
	}

	// Register once - should succeed
	err := collector.RegisterMetric(def)
	if err != nil {
		t.Fatalf("First registration failed: %v", err)
	}

	// Register again - should fail
	err = collector.RegisterMetric(def)
	if err == nil {
		t.Error("Expected error on duplicate registration")
	}
}

func TestUnknownMetricType(t *testing.T) {
	collector := NewPrometheusCollector()

	def := MetricDefinition{
		Name:   "test_unknown",
		Type:   MetricType("unknown"),
		Help:   "Test unknown type",
		Labels: []string{},
	}

	err := collector.RegisterMetric(def)
	if err == nil {
		t.Error("Expected error for unknown metric type")
	}
}

func TestCounterIncNonExistent(t *testing.T) {
	collector := NewPrometheusCollector()

	// Try to increment non-existent counter - should not panic
	collector.IncCounter("non_existent", map[string]string{})
}

func TestGaugeSetNonExistent(t *testing.T) {
	collector := NewPrometheusCollector()

	// Try to set non-existent gauge - should not panic
	collector.SetGauge("non_existent", 10.0, map[string]string{})
}

func TestHistogramObserveNonExistent(t *testing.T) {
	collector := NewPrometheusCollector()

	// Try to observe non-existent histogram - should not panic
	collector.ObserveHistogram("non_existent", 1.0, map[string]string{})
}

func TestMetricHandler(t *testing.T) {
	collector := NewPrometheusCollector()

	handler := collector.Handler()
	if handler == nil {
		t.Error("Expected non-nil handler")
	}
}

func TestClusterCollector(t *testing.T) {
	collector := NewPrometheusCollector()
	err := InitializeStandardMetrics(collector)
	if err != nil {
		t.Fatalf("Failed to initialize metrics: %v", err)
	}

	clusterCollector := NewClusterCollector(collector)

	t.Run("SetMemberCount", func(t *testing.T) {
		clusterCollector.SetMemberCount(3.0)
	})

	t.Run("SetHealthyMemberCount", func(t *testing.T) {
		clusterCollector.SetHealthyMemberCount(2.0)
	})

	t.Run("SetHasQuorum", func(t *testing.T) {
		clusterCollector.SetHasQuorum(true)
		clusterCollector.SetHasQuorum(false)
	})

	t.Run("RecordLeaderChange", func(t *testing.T) {
		clusterCollector.RecordLeaderChange("election")
		clusterCollector.RecordLeaderChange("failover")
	})

	t.Run("RecordLeaderElectionDuration", func(t *testing.T) {
		clusterCollector.RecordLeaderElectionDuration(500 * time.Millisecond)
	})

	t.Run("RecordRebalance", func(t *testing.T) {
		clusterCollector.RecordRebalance("manual", 2*time.Second, 10)
	})

	t.Run("RecordHeartbeatLatency", func(t *testing.T) {
		clusterCollector.RecordHeartbeatLatency("member-1", 50*time.Millisecond)
	})

	t.Run("RecordEtcdOperation", func(t *testing.T) {
		clusterCollector.RecordEtcdOperation("put", "success", 10*time.Millisecond)
		clusterCollector.RecordEtcdOperation("get", "error", 100*time.Millisecond)
	})

	t.Run("SetMemberStatus", func(t *testing.T) {
		clusterCollector.SetMemberStatus("member-1", 1.0) // healthy
		clusterCollector.SetMemberStatus("member-2", 0.5) // degraded
		clusterCollector.SetMemberStatus("member-3", 0.0) // unhealthy
	})

	t.Run("SetIsLeader", func(t *testing.T) {
		clusterCollector.SetIsLeader(true)
		clusterCollector.SetIsLeader(false)
	})
}

func TestNetworkCollector(t *testing.T) {
	collector := NewPrometheusCollector()
	err := InitializeStandardMetrics(collector)
	if err != nil {
		t.Fatalf("Failed to initialize metrics: %v", err)
	}

	networkCollector := NewNetworkCollector(collector)

	t.Run("SetActiveListeners", func(t *testing.T) {
		networkCollector.SetActiveListeners("grpc", "ipv4", "8080", 1.0)
		networkCollector.SetActiveListeners("grpc", "ipv6", "8080", 1.0)
		networkCollector.SetActiveListeners("http", "ipv4", "8081", 1.0)
		networkCollector.SetActiveListeners("http", "ipv6", "8081", 1.0)
	})

	t.Run("RecordConnection", func(t *testing.T) {
		networkCollector.RecordConnection("grpc", "ipv4")
		networkCollector.RecordConnection("grpc", "ipv6")
		networkCollector.RecordConnection("http", "ipv4")
		networkCollector.RecordConnection("http", "ipv6")
	})

	t.Run("SetActiveConnections", func(t *testing.T) {
		networkCollector.SetActiveConnections("grpc", "ipv4", 10.0)
		networkCollector.SetActiveConnections("grpc", "ipv6", 5.0)
		networkCollector.SetActiveConnections("http", "ipv4", 20.0)
		networkCollector.SetActiveConnections("http", "ipv6", 15.0)
	})

	t.Run("SetAgentsByIPVersion", func(t *testing.T) {
		networkCollector.SetAgentsByIPVersion("ipv4", 50.0)
		networkCollector.SetAgentsByIPVersion("ipv6", 25.0)
		networkCollector.SetAgentsByIPVersion("dual_stack", 10.0)
	})
}
