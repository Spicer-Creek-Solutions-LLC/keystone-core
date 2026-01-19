package federation

import (
	"context"
	"testing"
	"time"
)

func TestClusterState(t *testing.T) {
	states := []ClusterState{
		StateUnknown,
		StatePending,
		StateJoining,
		StateActive,
		StateDegraded,
		StateUnreachable,
		StateLeaving,
		StateLeft,
	}

	for _, s := range states {
		if s == "" {
			t.Error("State should not be empty")
		}
	}
}

func TestClusterRole(t *testing.T) {
	roles := []ClusterRole{
		RoleLeader,
		RoleMember,
		RoleObserver,
	}

	for _, r := range roles {
		if r == "" {
			t.Error("Role should not be empty")
		}
	}
}

func TestCluster(t *testing.T) {
	now := time.Now()
	cluster := &Cluster{
		ID:       "cluster-1",
		Name:     "prod-east-1",
		Endpoint: "https://cluster-1.example.com",
		State:    StateActive,
		Role:     RoleMember,
		Region:   "us-east-1",
		Zone:     "us-east-1a",
		Provider: "aws",
		Version:  "1.28.0",
		Capacity: &ClusterCapacity{
			Nodes:        10,
			CPUMillis:    100000,
			MemoryBytes:  1099511627776, // 1TB
			StorageBytes: 10995116277760, // 10TB
			Pods:         1100,
			GPUs:         8,
		},
		Labels: map[string]string{
			"environment": "production",
			"tier":        "premium",
		},
		JoinedAt:   &now,
		LastSeenAt: now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if cluster.State != StateActive {
		t.Errorf("State = %s, want active", cluster.State)
	}
	if cluster.Capacity.Nodes != 10 {
		t.Errorf("Nodes = %d, want 10", cluster.Capacity.Nodes)
	}
}

func TestClusterCapacity(t *testing.T) {
	capacity := &ClusterCapacity{
		Nodes:        5,
		CPUMillis:    50000,
		MemoryBytes:  549755813888, // 512GB
		StorageBytes: 1099511627776, // 1TB
		Pods:         550,
		GPUs:         4,
	}

	if capacity.Nodes != 5 {
		t.Errorf("Nodes = %d, want 5", capacity.Nodes)
	}
	if capacity.GPUs != 4 {
		t.Errorf("GPUs = %d, want 4", capacity.GPUs)
	}
}

func TestClusterHealth(t *testing.T) {
	health := &ClusterHealth{
		Healthy: true,
		Ready:   true,
		Conditions: []ClusterCondition{
			{
				Type:               "Reachable",
				Status:             "True",
				LastTransitionTime: time.Now(),
			},
			{
				Type:               "Ready",
				Status:             "True",
				LastTransitionTime: time.Now(),
			},
		},
		LastCheckedAt: time.Now(),
	}

	if !health.Healthy {
		t.Error("Healthy should be true")
	}
	if len(health.Conditions) != 2 {
		t.Errorf("Conditions = %d, want 2", len(health.Conditions))
	}
}

func TestDefaultFederationConfig(t *testing.T) {
	config := DefaultFederationConfig()

	if config.HealthInterval != 30*time.Second {
		t.Errorf("HealthInterval = %v, want 30s", config.HealthInterval)
	}
	if config.SyncInterval != time.Minute {
		t.Errorf("SyncInterval = %v, want 1m", config.SyncInterval)
	}
	if config.HeartbeatTimeout != 2*time.Minute {
		t.Errorf("HeartbeatTimeout = %v, want 2m", config.HeartbeatTimeout)
	}
}

func TestNewFederation(t *testing.T) {
	store := NewInMemoryClusterStore()
	fed := NewFederation(nil, store, "local-cluster")

	if fed == nil {
		t.Fatal("Expected non-nil federation")
	}
	if fed.config.HealthInterval != 30*time.Second {
		t.Error("Default config should be applied")
	}
}

func TestFederation_StartStop(t *testing.T) {
	store := NewInMemoryClusterStore()
	fed := NewFederation(nil, store, "local-cluster")

	var started, stopped bool
	fed.AddListener(func(e *FederationEvent) {
		if e.Type == "federation_started" {
			started = true
		}
		if e.Type == "federation_stopped" {
			stopped = true
		}
	})

	ctx := context.Background()
	if err := fed.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Can't start twice
	if err := fed.Start(ctx); err == nil {
		t.Error("Expected error when starting twice")
	}

	fed.Stop()

	if !started {
		t.Error("Expected federation_started event")
	}
	if !stopped {
		t.Error("Expected federation_stopped event")
	}
}

func TestFederation_LeaveCluster(t *testing.T) {
	store := NewInMemoryClusterStore()
	fed := NewFederation(nil, store, "local-cluster")

	ctx := context.Background()

	// Add a cluster
	cluster := &Cluster{
		ID:       "cluster-1",
		Name:     "test-cluster",
		Endpoint: "https://cluster-1.example.com",
		State:    StateActive,
	}
	store.Save(ctx, cluster)

	var events []*FederationEvent
	fed.AddListener(func(e *FederationEvent) {
		events = append(events, e)
	})

	// Leave cluster
	err := fed.LeaveCluster(ctx, "cluster-1")
	if err != nil {
		t.Fatalf("LeaveCluster failed: %v", err)
	}

	// Verify state
	left, _ := store.Get(ctx, "cluster-1")
	if left.State != StateLeft {
		t.Errorf("State = %s, want left", left.State)
	}

	// Check events
	if len(events) != 2 {
		t.Errorf("Events = %d, want 2 (leaving + left)", len(events))
	}
}

func TestFederation_GetCluster(t *testing.T) {
	store := NewInMemoryClusterStore()
	fed := NewFederation(nil, store, "local-cluster")

	ctx := context.Background()

	cluster := &Cluster{
		ID:       "cluster-1",
		Name:     "test-cluster",
		Endpoint: "https://cluster-1.example.com",
	}
	store.Save(ctx, cluster)

	retrieved, err := fed.GetCluster(ctx, "cluster-1")
	if err != nil {
		t.Fatalf("GetCluster failed: %v", err)
	}
	if retrieved.Name != "test-cluster" {
		t.Errorf("Name = %s, want test-cluster", retrieved.Name)
	}
}

func TestFederation_ListClusters(t *testing.T) {
	store := NewInMemoryClusterStore()
	fed := NewFederation(nil, store, "local-cluster")

	ctx := context.Background()

	clusters := []*Cluster{
		{ID: "c1", Name: "cluster-1", State: StateActive},
		{ID: "c2", Name: "cluster-2", State: StateActive},
		{ID: "c3", Name: "cluster-3", State: StateUnreachable},
	}

	for _, c := range clusters {
		store.Save(ctx, c)
	}

	all, err := fed.ListClusters(ctx)
	if err != nil {
		t.Fatalf("ListClusters failed: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ListClusters = %d, want 3", len(all))
	}

	active, err := fed.ListActiveClusters(ctx)
	if err != nil {
		t.Fatalf("ListActiveClusters failed: %v", err)
	}
	if len(active) != 2 {
		t.Errorf("ListActiveClusters = %d, want 2", len(active))
	}
}

func TestFederation_ListClustersByRegion(t *testing.T) {
	store := NewInMemoryClusterStore()
	fed := NewFederation(nil, store, "local-cluster")

	ctx := context.Background()

	clusters := []*Cluster{
		{ID: "c1", Name: "cluster-1", Region: "us-east-1"},
		{ID: "c2", Name: "cluster-2", Region: "us-east-1"},
		{ID: "c3", Name: "cluster-3", Region: "eu-west-1"},
	}

	for _, c := range clusters {
		store.Save(ctx, c)
	}

	usEast, err := fed.ListClustersByRegion(ctx, "us-east-1")
	if err != nil {
		t.Fatalf("ListClustersByRegion failed: %v", err)
	}
	if len(usEast) != 2 {
		t.Errorf("us-east-1 clusters = %d, want 2", len(usEast))
	}
}

func TestFederation_GetFederationStats(t *testing.T) {
	store := NewInMemoryClusterStore()
	fed := NewFederation(nil, store, "local-cluster")

	ctx := context.Background()

	clusters := []*Cluster{
		{
			ID:       "c1",
			State:    StateActive,
			Role:     RoleLeader,
			Region:   "us-east-1",
			Provider: "aws",
			Capacity: &ClusterCapacity{Nodes: 5, CPUMillis: 50000},
		},
		{
			ID:       "c2",
			State:    StateActive,
			Role:     RoleMember,
			Region:   "us-east-1",
			Provider: "aws",
			Capacity: &ClusterCapacity{Nodes: 3, CPUMillis: 30000},
		},
		{
			ID:       "c3",
			State:    StateUnreachable,
			Role:     RoleMember,
			Region:   "eu-west-1",
			Provider: "gcp",
			Capacity: &ClusterCapacity{Nodes: 4, CPUMillis: 40000},
		},
	}

	for _, c := range clusters {
		store.Save(ctx, c)
	}

	stats, err := fed.GetFederationStats(ctx)
	if err != nil {
		t.Fatalf("GetFederationStats failed: %v", err)
	}

	if stats.TotalClusters != 3 {
		t.Errorf("TotalClusters = %d, want 3", stats.TotalClusters)
	}
	if stats.ActiveClusters != 2 {
		t.Errorf("ActiveClusters = %d, want 2", stats.ActiveClusters)
	}
	if stats.ByState[StateActive] != 2 {
		t.Errorf("ByState[active] = %d, want 2", stats.ByState[StateActive])
	}
	if stats.ByRole[RoleMember] != 2 {
		t.Errorf("ByRole[member] = %d, want 2", stats.ByRole[RoleMember])
	}
	if stats.TotalCapacity.Nodes != 12 {
		t.Errorf("TotalCapacity.Nodes = %d, want 12", stats.TotalCapacity.Nodes)
	}
}

func TestInMemoryClusterStore(t *testing.T) {
	store := NewInMemoryClusterStore()
	ctx := context.Background()

	cluster := &Cluster{
		ID:       "cluster-1",
		Name:     "test-cluster",
		State:    StateActive,
		Region:   "us-east-1",
		Endpoint: "https://cluster-1.example.com",
	}

	// Test Save
	err := store.Save(ctx, cluster)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Test Get
	retrieved, err := store.Get(ctx, "cluster-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved.Name != "test-cluster" {
		t.Errorf("Name = %s, want test-cluster", retrieved.Name)
	}

	// Test List
	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List = %d, want 1", len(list))
	}

	// Test ListByState
	active, err := store.ListByState(ctx, StateActive)
	if err != nil {
		t.Fatalf("ListByState failed: %v", err)
	}
	if len(active) != 1 {
		t.Errorf("ListByState = %d, want 1", len(active))
	}

	// Test ListByRegion
	usEast, err := store.ListByRegion(ctx, "us-east-1")
	if err != nil {
		t.Fatalf("ListByRegion failed: %v", err)
	}
	if len(usEast) != 1 {
		t.Errorf("ListByRegion = %d, want 1", len(usEast))
	}

	// Test Delete
	err = store.Delete(ctx, "cluster-1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = store.Get(ctx, "cluster-1")
	if err == nil {
		t.Error("Expected error after delete")
	}
}

func TestPlacementType(t *testing.T) {
	types := []PlacementType{
		PlacementAll,
		PlacementWeighted,
		PlacementSpread,
		PlacementBinpack,
	}

	for _, pt := range types {
		if pt == "" {
			t.Error("Placement type should not be empty")
		}
	}
}

func TestPlacementPolicy(t *testing.T) {
	policy := &PlacementPolicy{
		Name:      "production",
		Type:      PlacementSpread,
		Regions:   []string{"us-east-1", "us-west-2"},
		Providers: []string{"aws"},
		Selector: map[string]string{
			"tier": "premium",
		},
		Weights: map[string]int{
			"cluster-1": 3,
			"cluster-2": 1,
		},
		Constraints: []PlacementConstraint{
			{
				Type:     "required",
				Key:      "environment",
				Operator: "in",
				Values:   []string{"production"},
			},
		},
	}

	if policy.Type != PlacementSpread {
		t.Errorf("Type = %s, want spread", policy.Type)
	}
	if len(policy.Regions) != 2 {
		t.Errorf("Regions = %d, want 2", len(policy.Regions))
	}
}

func TestNewScheduler(t *testing.T) {
	store := NewInMemoryClusterStore()
	fed := NewFederation(nil, store, "local-cluster")
	scheduler := NewScheduler(fed)

	if scheduler == nil {
		t.Fatal("Expected non-nil scheduler")
	}
}

func TestScheduler_RegisterPolicy(t *testing.T) {
	store := NewInMemoryClusterStore()
	fed := NewFederation(nil, store, "local-cluster")
	scheduler := NewScheduler(fed)

	policy := &PlacementPolicy{
		Name: "test-policy",
		Type: PlacementAll,
	}

	scheduler.RegisterPolicy(policy)

	retrieved, ok := scheduler.GetPolicy("test-policy")
	if !ok {
		t.Error("Expected to find policy")
	}
	if retrieved.Name != "test-policy" {
		t.Errorf("Name = %s, want test-policy", retrieved.Name)
	}
}

func TestScheduler_Schedule(t *testing.T) {
	store := NewInMemoryClusterStore()
	fed := NewFederation(nil, store, "local-cluster")
	scheduler := NewScheduler(fed)

	ctx := context.Background()

	// Add clusters
	clusters := []*Cluster{
		{
			ID:       "c1",
			Name:     "cluster-1",
			State:    StateActive,
			Region:   "us-east-1",
			Provider: "aws",
			Labels:   map[string]string{"tier": "premium"},
		},
		{
			ID:       "c2",
			Name:     "cluster-2",
			State:    StateActive,
			Region:   "us-east-1",
			Provider: "aws",
			Labels:   map[string]string{"tier": "standard"},
		},
		{
			ID:       "c3",
			Name:     "cluster-3",
			State:    StateActive,
			Region:   "eu-west-1",
			Provider: "gcp",
			Labels:   map[string]string{"tier": "premium"},
		},
	}

	for _, c := range clusters {
		store.Save(ctx, c)
	}

	// Test PlacementAll with region filter
	scheduler.RegisterPolicy(&PlacementPolicy{
		Name:    "us-only",
		Type:    PlacementAll,
		Regions: []string{"us-east-1"},
	})

	result, err := scheduler.Schedule(ctx, "us-only")
	if err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("us-only placement = %d, want 2", len(result))
	}

	// Test with label selector
	scheduler.RegisterPolicy(&PlacementPolicy{
		Name:     "premium-only",
		Type:     PlacementAll,
		Selector: map[string]string{"tier": "premium"},
	})

	result, err = scheduler.Schedule(ctx, "premium-only")
	if err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("premium-only placement = %d, want 2", len(result))
	}

	// Test binpack
	scheduler.RegisterPolicy(&PlacementPolicy{
		Name: "binpack",
		Type: PlacementBinpack,
	})

	result, err = scheduler.Schedule(ctx, "binpack")
	if err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("binpack placement = %d, want 1", len(result))
	}
}

func TestScheduler_Schedule_PolicyNotFound(t *testing.T) {
	store := NewInMemoryClusterStore()
	fed := NewFederation(nil, store, "local-cluster")
	scheduler := NewScheduler(fed)

	ctx := context.Background()

	_, err := scheduler.Schedule(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent policy")
	}
}

func TestFederationConfig(t *testing.T) {
	config := &FederationConfig{
		ID:               "fed-1",
		Name:             "production-federation",
		HealthInterval:   15 * time.Second,
		SyncInterval:     30 * time.Second,
		HeartbeatTimeout: time.Minute,
		TLSConfig: &TLSConfig{
			CACert:     "/path/to/ca.crt",
			ClientCert: "/path/to/client.crt",
			ClientKey:  "/path/to/client.key",
		},
	}

	if config.HealthInterval != 15*time.Second {
		t.Errorf("HealthInterval = %v, want 15s", config.HealthInterval)
	}
	if config.TLSConfig.CACert != "/path/to/ca.crt" {
		t.Error("TLSConfig.CACert mismatch")
	}
}

func TestTLSConfig(t *testing.T) {
	config := &TLSConfig{
		CACert:     "ca-cert-data",
		ClientCert: "client-cert-data",
		ClientKey:  "client-key-data",
		SkipVerify: true,
	}

	if !config.SkipVerify {
		t.Error("SkipVerify should be true")
	}
}

func TestFederationEvent(t *testing.T) {
	event := &FederationEvent{
		Type:       "cluster_joined",
		ClusterID:  "cluster-1",
		Timestamp:  time.Now(),
		Message:    "Cluster joined the federation",
		Details: map[string]interface{}{
			"region": "us-east-1",
		},
	}

	if event.Type != "cluster_joined" {
		t.Errorf("Type = %s, want cluster_joined", event.Type)
	}
}

func TestClusterCondition(t *testing.T) {
	condition := ClusterCondition{
		Type:               "Ready",
		Status:             "True",
		LastTransitionTime: time.Now(),
		Reason:             "AllSystemsOperational",
		Message:            "All components are healthy",
	}

	if condition.Status != "True" {
		t.Errorf("Status = %s, want True", condition.Status)
	}
}

func TestPlacementConstraint(t *testing.T) {
	constraint := PlacementConstraint{
		Type:     "required",
		Key:      "topology.kubernetes.io/zone",
		Operator: "in",
		Values:   []string{"us-east-1a", "us-east-1b"},
	}

	if constraint.Type != "required" {
		t.Errorf("Type = %s, want required", constraint.Type)
	}
	if len(constraint.Values) != 2 {
		t.Errorf("Values = %d, want 2", len(constraint.Values))
	}
}

func TestCopyCluster(t *testing.T) {
	now := time.Now()
	original := &Cluster{
		ID:       "c1",
		Name:     "original",
		State:    StateActive,
		JoinedAt: &now,
		Capacity: &ClusterCapacity{
			Nodes:     5,
			CPUMillis: 50000,
		},
		Labels: map[string]string{
			"env": "prod",
		},
	}

	copy := copyCluster(original)

	// Verify deep copy
	if copy.ID != original.ID {
		t.Error("ID should match")
	}

	// Modify copy and verify original is unchanged
	copy.Name = "modified"
	copy.Labels["env"] = "dev"

	if original.Name == "modified" {
		t.Error("Original should not be modified")
	}
	if original.Labels["env"] == "dev" {
		t.Error("Original labels should not be modified")
	}
}
