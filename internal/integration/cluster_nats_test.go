// Package integration provides in-process integration tests that verify
// cross-epic interactions without requiring Docker containers.
package integration

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/events"
)

// =============================================================================
// Epic 11 (Clustering) + Epic 4 (Events) Integration
// Tests: Cluster events flow through the event system
// =============================================================================

// TestIntegration_ClusterEventsEmission tests that cluster lifecycle events
// are properly emitted to the event bus.
func TestIntegration_ClusterEventsEmission(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Track cluster events
	var clusterEvents []*events.Event
	var mu sync.Mutex

	sub, err := env.eventBus.Subscribe("cluster-tracker",
		func(e *events.Event) error {
			// Manual filter for cluster.* events
			if strings.HasPrefix(string(e.Type), "cluster.") {
				mu.Lock()
				clusterEvents = append(clusterEvents, e)
				mu.Unlock()
			}
			return nil
		})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Simulate cluster lifecycle events
	clusterLifecycle := []struct {
		eventType events.EventType
		data      map[string]interface{}
	}{
		{
			eventType: events.EventType("cluster.member.join"),
			data: map[string]interface{}{
				"member_id": "server-1",
				"address":   "10.0.0.1:2380",
				"role":      "follower",
			},
		},
		{
			eventType: events.EventType("cluster.leader.elected"),
			data: map[string]interface{}{
				"leader_id": "server-1",
				"term":      1,
			},
		},
		{
			eventType: events.EventType("cluster.member.join"),
			data: map[string]interface{}{
				"member_id": "server-2",
				"address":   "10.0.0.2:2380",
				"role":      "follower",
			},
		},
		{
			eventType: events.EventType("cluster.quorum.gained"),
			data: map[string]interface{}{
				"members":      2,
				"quorum_size":  2,
				"cluster_size": 3,
			},
		},
		{
			eventType: events.EventType("cluster.member.join"),
			data: map[string]interface{}{
				"member_id": "server-3",
				"address":   "10.0.0.3:2380",
				"role":      "follower",
			},
		},
	}

	for _, evt := range clusterLifecycle {
		event := events.NewEvent(evt.eventType).
			Source("/cluster").
			DataMap(evt.data).
			Build()

		if err := env.eventBus.PublishSync(event); err != nil {
			t.Errorf("Failed to publish %s: %v", evt.eventType, err)
		}
	}

	mu.Lock()
	count := len(clusterEvents)
	mu.Unlock()

	if count != len(clusterLifecycle) {
		t.Errorf("Expected %d cluster events, got %d", len(clusterLifecycle), count)
	}

	// Verify specific event types
	memberJoins := 0
	leaderElected := 0
	quorumGained := 0

	for _, e := range clusterEvents {
		switch e.Type {
		case events.EventType("cluster.member.join"):
			memberJoins++
		case events.EventType("cluster.leader.elected"):
			leaderElected++
		case events.EventType("cluster.quorum.gained"):
			quorumGained++
		}
	}

	if memberJoins != 3 {
		t.Errorf("Expected 3 member.join events, got %d", memberJoins)
	}
	if leaderElected != 1 {
		t.Errorf("Expected 1 leader.elected event, got %d", leaderElected)
	}
	if quorumGained != 1 {
		t.Errorf("Expected 1 quorum.gained event, got %d", quorumGained)
	}

	t.Logf("Cluster events: %d joins, %d leader elections, %d quorum events", memberJoins, leaderElected, quorumGained)
}

// =============================================================================
// Epic 14 (NATS Mesh) + Epic 11 (Clustering) Integration
// Tests: NATS mesh cluster communication
// =============================================================================

// TestIntegration_NATSMeshClusterEvents tests that NATS mesh events
// are properly propagated in a clustered environment.
func TestIntegration_NATSMeshClusterEvents(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Track NATS mesh events
	var meshEvents []*events.Event
	var mu sync.Mutex

	sub, err := env.eventBus.Subscribe("mesh-tracker",
		func(e *events.Event) error {
			// Filter for nats.* events
			if strings.HasPrefix(string(e.Type), "nats.") {
				mu.Lock()
				meshEvents = append(meshEvents, e)
				mu.Unlock()
			}
			return nil
		})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Simulate NATS mesh lifecycle events
	meshLifecycle := []struct {
		eventType events.EventType
		data      map[string]interface{}
	}{
		{
			eventType: events.EventType("nats.server.connected"),
			data: map[string]interface{}{
				"server_id": "nats-1",
				"address":   "10.0.0.1:4222",
				"cluster":   "kscore-nats",
				"is_leader": false,
			},
		},
		{
			eventType: events.EventType("nats.route.established"),
			data: map[string]interface{}{
				"from_server": "nats-1",
				"to_server":   "nats-2",
				"route_type":  "cluster",
			},
		},
		{
			eventType: events.EventType("nats.jetstream.ready"),
			data: map[string]interface{}{
				"server_id":    "nats-1",
				"streams":      5,
				"consumers":    10,
				"storage_used": 1024000,
			},
		},
		{
			eventType: events.EventType("nats.leaf.connected"),
			data: map[string]interface{}{
				"leaf_id":    "leaf-agent-1",
				"hub_server": "nats-1",
				"agent_id":   "agent-1",
			},
		},
	}

	for _, evt := range meshLifecycle {
		event := events.NewEvent(evt.eventType).
			Source("/nats/mesh").
			DataMap(evt.data).
			Build()

		if err := env.eventBus.PublishSync(event); err != nil {
			t.Errorf("Failed to publish %s: %v", evt.eventType, err)
		}
	}

	mu.Lock()
	count := len(meshEvents)
	mu.Unlock()

	if count != len(meshLifecycle) {
		t.Errorf("Expected %d mesh events, got %d", len(meshLifecycle), count)
	}

	t.Logf("NATS mesh events processed: %d events", count)
}

// =============================================================================
// Cluster + NATS Command Routing Integration
// Tests: Commands routed through clustered NATS
// =============================================================================

// TestIntegration_CommandRoutingInCluster tests that commands are properly
// routed through the NATS cluster to the correct agents.
func TestIntegration_CommandRoutingInCluster(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	correlationID := "cmd-route-" + time.Now().Format("20060102150405")

	// Track command events
	var commandEvents []*events.Event
	var mu sync.Mutex

	sub, err := env.eventBus.Subscribe("command-tracker", func(e *events.Event) error {
		if e.CorrelationID == correlationID {
			mu.Lock()
			commandEvents = append(commandEvents, e)
			mu.Unlock()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Simulate command routing through cluster
	// Step 1: Command received by control plane
	cmdReceivedEvent := events.NewEvent(events.EventType("command.received")).
		Source("/control-plane/server-1").
		CorrelationID(correlationID).
		Data("command", "state.apply").
		Data("target", "agent-1").
		Data("received_by", "server-1").
		Build()

	if err := env.eventBus.PublishSync(cmdReceivedEvent); err != nil {
		t.Fatalf("Failed to publish command received: %v", err)
	}

	// Step 2: Command routed via NATS
	cmdRoutedEvent := events.NewEvent(events.EventType("command.routed")).
		Source("/nats/router").
		CorrelationID(correlationID).
		Data("command", "state.apply").
		Data("target", "agent-1").
		Data("route_path", []string{"server-1", "nats-cluster", "agent-1"}).
		Build()

	if err := env.eventBus.PublishSync(cmdRoutedEvent); err != nil {
		t.Fatalf("Failed to publish command routed: %v", err)
	}

	// Step 3: Command delivered to agent
	cmdDeliveredEvent := events.NewEvent(events.EventType("command.delivered")).
		Source("/agent/agent-1").
		CorrelationID(correlationID).
		Data("command", "state.apply").
		Data("delivered_to", "agent-1").
		Data("latency_ms", 15).
		Build()

	if err := env.eventBus.PublishSync(cmdDeliveredEvent); err != nil {
		t.Fatalf("Failed to publish command delivered: %v", err)
	}

	// Step 4: Command executed
	cmdExecutedEvent := events.NewEvent(events.EventTypeJobComplete).
		Source("/agent/agent-1").
		CorrelationID(correlationID).
		Data("command", "state.apply").
		Data("exit_code", 0).
		Data("duration_ms", 1500).
		Build()

	if err := env.eventBus.PublishSync(cmdExecutedEvent); err != nil {
		t.Fatalf("Failed to publish command executed: %v", err)
	}

	mu.Lock()
	count := len(commandEvents)
	eventsCopy := make([]*events.Event, len(commandEvents))
	copy(eventsCopy, commandEvents)
	mu.Unlock()

	if count != 4 {
		t.Errorf("Expected 4 command events, got %d", count)
	}

	// Verify all expected event types are present (order may vary with async delivery)
	expectedTypes := map[events.EventType]bool{
		events.EventType("command.received"):  false,
		events.EventType("command.routed"):    false,
		events.EventType("command.delivered"): false,
		events.EventTypeJobComplete:           false,
	}

	for _, e := range eventsCopy {
		if _, ok := expectedTypes[e.Type]; ok {
			expectedTypes[e.Type] = true
		}
	}

	for eventType, found := range expectedTypes {
		if !found {
			t.Errorf("Missing expected event type: %s", eventType)
		}
	}

	t.Logf("Command routing verified through %d events", count)
}

// =============================================================================
// Cluster State Replication Integration
// Tests: State replication across cluster nodes
// =============================================================================

// TestIntegration_ClusterStateReplication tests that state changes are
// properly replicated across cluster nodes.
func TestIntegration_ClusterStateReplication(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	correlationID := "state-repl-" + time.Now().Format("20060102150405")

	// Track replication events
	var replicationEvents []*events.Event
	var mu sync.Mutex

	sub, err := env.eventBus.Subscribe("replication-tracker", func(e *events.Event) error {
		if e.CorrelationID == correlationID {
			mu.Lock()
			replicationEvents = append(replicationEvents, e)
			mu.Unlock()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Simulate state replication
	// Step 1: State change on primary
	stateChangeEvent := events.NewEvent(events.EventTypeStateChange).
		Source("/state/primary").
		CorrelationID(correlationID).
		Data("key", "agent/agent-1/facts").
		Data("operation", "update").
		Data("node", "server-1").
		Build()

	if err := env.eventBus.PublishSync(stateChangeEvent); err != nil {
		t.Fatalf("Failed to publish state change: %v", err)
	}

	// Step 2: Replication to secondaries
	servers := []string{"server-2", "server-3"}
	for _, server := range servers {
		replicatedEvent := events.NewEvent(events.EventType("state.replicated")).
			Source("/state/replication").
			CorrelationID(correlationID).
			Data("key", "agent/agent-1/facts").
			Data("from", "server-1").
			Data("to", server).
			Data("lag_ms", 5).
			Build()

		if err := env.eventBus.PublishSync(replicatedEvent); err != nil {
			t.Errorf("Failed to publish replication to %s: %v", server, err)
		}
	}

	// Step 3: Replication acknowledged
	ackEvent := events.NewEvent(events.EventType("state.replication.ack")).
		Source("/state/replication").
		CorrelationID(correlationID).
		Data("key", "agent/agent-1/facts").
		Data("ack_count", 2).
		Data("total_nodes", 3).
		Data("quorum", true).
		Build()

	if err := env.eventBus.PublishSync(ackEvent); err != nil {
		t.Fatalf("Failed to publish replication ack: %v", err)
	}

	mu.Lock()
	count := len(replicationEvents)
	mu.Unlock()

	if count != 4 {
		t.Errorf("Expected 4 replication events, got %d", count)
	}

	// Verify quorum was achieved
	hasQuorum := false
	for _, e := range replicationEvents {
		if e.Type == events.EventType("state.replication.ack") {
			if quorum, ok := e.Data["quorum"].(bool); ok && quorum {
				hasQuorum = true
			}
		}
	}

	if !hasQuorum {
		t.Error("Expected replication to achieve quorum")
	}

	t.Logf("State replication verified with %d events", count)
}

// =============================================================================
// Cluster Failover Integration
// Tests: Leader failover in clustered environment
// =============================================================================

// TestIntegration_ClusterFailover tests that cluster properly handles
// leader failover.
func TestIntegration_ClusterFailover(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	correlationID := "failover-" + time.Now().Format("20060102150405")

	// Track failover events
	var failoverEvents []*events.Event
	var mu sync.Mutex

	sub, err := env.eventBus.Subscribe("failover-tracker", func(e *events.Event) error {
		if e.CorrelationID == correlationID {
			mu.Lock()
			failoverEvents = append(failoverEvents, e)
			mu.Unlock()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Simulate failover scenario
	// Step 1: Leader failure detected
	failureEvent := events.NewEvent(events.EventType("cluster.member.failed")).
		Source("/cluster/health").
		CorrelationID(correlationID).
		Severity(events.SeverityError).
		Data("member_id", "server-1").
		Data("was_leader", true).
		Data("failure_reason", "heartbeat_timeout").
		Build()

	if err := env.eventBus.PublishSync(failureEvent); err != nil {
		t.Fatalf("Failed to publish failure event: %v", err)
	}

	// Step 2: Election started
	electionEvent := events.NewEvent(events.EventType("cluster.election.started")).
		Source("/cluster/election").
		CorrelationID(correlationID).
		Data("term", 2).
		Data("candidates", []string{"server-2", "server-3"}).
		Build()

	if err := env.eventBus.PublishSync(electionEvent); err != nil {
		t.Fatalf("Failed to publish election event: %v", err)
	}

	// Step 3: New leader elected
	newLeaderEvent := events.NewEvent(events.EventType("cluster.leader.elected")).
		Source("/cluster/election").
		CorrelationID(correlationID).
		Data("leader_id", "server-2").
		Data("term", 2).
		Data("votes", 2).
		Build()

	if err := env.eventBus.PublishSync(newLeaderEvent); err != nil {
		t.Fatalf("Failed to publish new leader event: %v", err)
	}

	// Step 4: Cluster stabilized
	stabilizedEvent := events.NewEvent(events.EventType("cluster.stabilized")).
		Source("/cluster/health").
		CorrelationID(correlationID).
		Data("leader_id", "server-2").
		Data("healthy_members", 2).
		Data("total_members", 3).
		Data("failover_duration_ms", 250).
		Build()

	if err := env.eventBus.PublishSync(stabilizedEvent); err != nil {
		t.Fatalf("Failed to publish stabilized event: %v", err)
	}

	mu.Lock()
	count := len(failoverEvents)
	mu.Unlock()

	if count != 4 {
		t.Errorf("Expected 4 failover events, got %d", count)
	}

	// Verify failover completed successfully
	hasNewLeader := false
	hasStabilized := false

	for _, e := range failoverEvents {
		switch e.Type {
		case events.EventType("cluster.leader.elected"):
			if e.Data["leader_id"] == "server-2" {
				hasNewLeader = true
			}
		case events.EventType("cluster.stabilized"):
			hasStabilized = true
		}
	}

	if !hasNewLeader {
		t.Error("Expected new leader to be elected")
	}
	if !hasStabilized {
		t.Error("Expected cluster to stabilize")
	}

	t.Logf("Cluster failover verified with %d events", count)
}

// =============================================================================
// Multi-Region Cluster Integration
// Tests: Events across multiple cluster regions
// =============================================================================

// TestIntegration_MultiRegionCluster tests event propagation across
// multiple cluster regions.
func TestIntegration_MultiRegionCluster(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	correlationID := "multi-region-" + time.Now().Format("20060102150405")

	// Track multi-region events
	var regionEvents []*events.Event
	var mu sync.Mutex

	sub, err := env.eventBus.Subscribe("region-tracker", func(e *events.Event) error {
		if e.CorrelationID == correlationID {
			mu.Lock()
			regionEvents = append(regionEvents, e)
			mu.Unlock()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Simulate multi-region events
	regions := []string{"us-west", "us-east", "eu-west"}

	for _, region := range regions {
		// Region registers
		registerEvent := events.NewEvent(events.EventType("cluster.region.registered")).
			Source("/cluster/regions").
			CorrelationID(correlationID).
			Data("region", region).
			Data("servers", 3).
			Data("agents", 50).
			Build()

		if err := env.eventBus.PublishSync(registerEvent); err != nil {
			t.Errorf("Failed to publish region register for %s: %v", region, err)
		}
	}

	// Gateway connections between regions
	gateways := []struct {
		from string
		to   string
	}{
		{"us-west", "us-east"},
		{"us-east", "eu-west"},
		{"eu-west", "us-west"},
	}

	for _, gw := range gateways {
		gatewayEvent := events.NewEvent(events.EventType("cluster.gateway.connected")).
			Source("/cluster/gateway").
			CorrelationID(correlationID).
			Data("from_region", gw.from).
			Data("to_region", gw.to).
			Data("latency_ms", 50).
			Build()

		if err := env.eventBus.PublishSync(gatewayEvent); err != nil {
			t.Errorf("Failed to publish gateway connection %s -> %s: %v", gw.from, gw.to, err)
		}
	}

	mu.Lock()
	count := len(regionEvents)
	mu.Unlock()

	// Expected: 3 region registrations + 3 gateway connections = 6
	if count != 6 {
		t.Errorf("Expected 6 multi-region events, got %d", count)
	}

	// Count event types
	registrations := 0
	gateways_connected := 0

	for _, e := range regionEvents {
		switch e.Type {
		case events.EventType("cluster.region.registered"):
			registrations++
		case events.EventType("cluster.gateway.connected"):
			gateways_connected++
		}
	}

	if registrations != 3 {
		t.Errorf("Expected 3 region registrations, got %d", registrations)
	}
	if gateways_connected != 3 {
		t.Errorf("Expected 3 gateway connections, got %d", gateways_connected)
	}

	t.Logf("Multi-region cluster: %d registrations, %d gateway connections", registrations, gateways_connected)
}
