package mirror

import (
	"testing"
	"time"
)

func TestNewMirrorGroup(t *testing.T) {
	tests := []struct {
		name    string
		config  *MirrorGroupConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &MirrorGroupConfig{
				ID:   "test-group",
				Name: "Test Group",
				Mirrors: []*Mirror{
					{ID: "mirror-1", ClusterID: "cluster-1", Enabled: true},
					{ID: "mirror-2", ClusterID: "cluster-2", Enabled: true},
				},
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			config: &MirrorGroupConfig{
				Name: "Test Group",
				Mirrors: []*Mirror{
					{ID: "mirror-1", ClusterID: "cluster-1"},
				},
			},
			wantErr: true,
		},
		{
			name: "no mirrors",
			config: &MirrorGroupConfig{
				ID:      "test-group",
				Mirrors: []*Mirror{},
			},
			wantErr: true,
		},
		{
			name: "duplicate mirror ID",
			config: &MirrorGroupConfig{
				ID: "test-group",
				Mirrors: []*Mirror{
					{ID: "mirror-1", ClusterID: "cluster-1"},
					{ID: "mirror-1", ClusterID: "cluster-2"},
				},
			},
			wantErr: true,
		},
		{
			name: "mirror missing cluster ID",
			config: &MirrorGroupConfig{
				ID: "test-group",
				Mirrors: []*Mirror{
					{ID: "mirror-1"},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid read strategy",
			config: &MirrorGroupConfig{
				ID: "test-group",
				Mirrors: []*Mirror{
					{ID: "mirror-1", ClusterID: "cluster-1"},
				},
				ReadStrategy: "invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid write policy",
			config: &MirrorGroupConfig{
				ID: "test-group",
				Mirrors: []*Mirror{
					{ID: "mirror-1", ClusterID: "cluster-1"},
				},
				WritePolicy: "invalid",
			},
			wantErr: true,
		},
		{
			name: "quorum too large",
			config: &MirrorGroupConfig{
				ID: "test-group",
				Mirrors: []*Mirror{
					{ID: "mirror-1", ClusterID: "cluster-1"},
				},
				WritePolicy: WritePolicyQuorum,
				QuorumSize:  5,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group, err := NewMirrorGroup(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewMirrorGroup() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && group == nil {
				t.Error("NewMirrorGroup() returned nil group")
			}
		})
	}
}

func TestMirrorGroup_GetHealthyMirrors(t *testing.T) {
	group, err := NewMirrorGroup(&MirrorGroupConfig{
		ID: "test-group",
		Mirrors: []*Mirror{
			{ID: "m1", ClusterID: "c1", Enabled: true},
			{ID: "m2", ClusterID: "c2", Enabled: true},
			{ID: "m3", ClusterID: "c3", Enabled: false},
		},
	})
	if err != nil {
		t.Fatalf("Failed to create group: %v", err)
	}

	// Initially all enabled mirrors should be returned (state is unknown)
	healthy := group.GetHealthyMirrors()
	if len(healthy) != 2 {
		t.Errorf("Expected 2 healthy mirrors, got %d", len(healthy))
	}

	// Mark one as unhealthy
	group.UpdateHealth("m1", MirrorStateUnhealthy, 100*time.Millisecond, nil)
	healthy = group.GetHealthyMirrors()
	if len(healthy) != 1 {
		t.Errorf("Expected 1 healthy mirror, got %d", len(healthy))
	}
}

func TestMirrorGroup_UpdateHealth(t *testing.T) {
	group, err := NewMirrorGroup(&MirrorGroupConfig{
		ID: "test-group",
		Mirrors: []*Mirror{
			{ID: "m1", ClusterID: "c1", Enabled: true},
		},
	})
	if err != nil {
		t.Fatalf("Failed to create group: %v", err)
	}

	// Update with success
	group.UpdateHealth("m1", MirrorStateHealthy, 50*time.Millisecond, nil)
	health, ok := group.GetHealth("m1")
	if !ok {
		t.Fatal("Health not found")
	}
	if health.State != MirrorStateHealthy {
		t.Errorf("Expected healthy state, got %s", health.State)
	}
	if health.AvgLatency != 50*time.Millisecond {
		t.Errorf("Expected 50ms latency, got %s", health.AvgLatency)
	}

	// Update with more samples
	group.UpdateHealth("m1", MirrorStateHealthy, 100*time.Millisecond, nil)
	health, _ = group.GetHealth("m1")
	// EMA should be closer to 100ms now
	if health.AvgLatency <= 50*time.Millisecond {
		t.Errorf("Expected latency > 50ms, got %s", health.AvgLatency)
	}
}

func TestMirrorGroup_MatchesPath(t *testing.T) {
	group, _ := NewMirrorGroup(&MirrorGroupConfig{
		ID: "test-group",
		Mirrors: []*Mirror{
			{ID: "m1", ClusterID: "c1"},
		},
		PathPrefixes: []string{"/packages/", "/configs/"},
	})

	tests := []struct {
		path    string
		matches bool
	}{
		{"/packages/nginx.tar.gz", true},
		{"/packages/sub/dir/file.txt", true},
		{"/configs/app.yaml", true},
		{"/other/file.txt", false},
		{"/pack", false},
	}

	for _, tt := range tests {
		if got := group.MatchesPath(tt.path); got != tt.matches {
			t.Errorf("MatchesPath(%s) = %v, want %v", tt.path, got, tt.matches)
		}
	}

	// Empty path prefixes should match all
	groupAll, _ := NewMirrorGroup(&MirrorGroupConfig{
		ID: "test-group-all",
		Mirrors: []*Mirror{
			{ID: "m1", ClusterID: "c1"},
		},
	})
	if !groupAll.MatchesPath("/any/path") {
		t.Error("Empty path prefixes should match all paths")
	}
}

func TestMirrorGroup_MatchesNamespace(t *testing.T) {
	group, _ := NewMirrorGroup(&MirrorGroupConfig{
		ID: "test-group",
		Mirrors: []*Mirror{
			{ID: "m1", ClusterID: "c1"},
		},
		Namespaces: []string{"prod", "staging"},
	})

	tests := []struct {
		namespace string
		matches   bool
	}{
		{"prod", true},
		{"staging", true},
		{"dev", false},
		{"production", false},
	}

	for _, tt := range tests {
		if got := group.MatchesNamespace(tt.namespace); got != tt.matches {
			t.Errorf("MatchesNamespace(%s) = %v, want %v", tt.namespace, got, tt.matches)
		}
	}
}

func TestRegistry(t *testing.T) {
	registry := NewRegistry()

	group1, _ := NewMirrorGroup(&MirrorGroupConfig{
		ID:           "group-1",
		PathPrefixes: []string{"/packages/"},
		Mirrors: []*Mirror{
			{ID: "m1", ClusterID: "c1"},
		},
	})

	group2, _ := NewMirrorGroup(&MirrorGroupConfig{
		ID:         "group-2",
		Namespaces: []string{"prod"},
		Mirrors: []*Mirror{
			{ID: "m2", ClusterID: "c2"},
		},
	})

	defaultGroup, _ := NewMirrorGroup(&MirrorGroupConfig{
		ID: "default",
		Mirrors: []*Mirror{
			{ID: "m3", ClusterID: "c3"},
		},
	})

	// Register groups
	if err := registry.Register(group1); err != nil {
		t.Fatalf("Failed to register group1: %v", err)
	}
	if err := registry.Register(group2); err != nil {
		t.Fatalf("Failed to register group2: %v", err)
	}
	if err := registry.Register(defaultGroup); err != nil {
		t.Fatalf("Failed to register default group: %v", err)
	}

	// Test GetForPath
	if g := registry.GetForPath("/packages/nginx.tar.gz"); g != group1 {
		t.Error("Expected group1 for /packages/ path")
	}
	if g := registry.GetForPath("/other/file.txt"); g != defaultGroup {
		t.Error("Expected default group for unmatched path")
	}

	// Test GetForNamespace
	if g := registry.GetForNamespace("prod"); g != group2 {
		t.Error("Expected group2 for prod namespace")
	}
	if g := registry.GetForNamespace("dev"); g != defaultGroup {
		t.Error("Expected default group for unmatched namespace")
	}

	// Test GetForRequest
	if g := registry.GetForRequest("/any/path", "prod"); g != group2 {
		t.Error("Expected group2 for prod namespace request")
	}
	if g := registry.GetForRequest("/packages/file.txt", "dev"); g != group1 {
		t.Error("Expected group1 for /packages/ path")
	}

	// Test duplicate registration
	if err := registry.Register(group1); err == nil {
		t.Error("Expected error for duplicate registration")
	}

	// Test unregister
	if err := registry.Unregister("group-1"); err != nil {
		t.Fatalf("Failed to unregister: %v", err)
	}
	if g := registry.GetForPath("/packages/file.txt"); g != defaultGroup {
		t.Error("Expected default after unregister")
	}
}

func TestNearestRouter(t *testing.T) {
	router := NewNearestRouter()

	group, _ := NewMirrorGroup(&MirrorGroupConfig{
		ID: "test-group",
		Mirrors: []*Mirror{
			{ID: "m1", ClusterID: "c1", Enabled: true, Priority: 1},
			{ID: "m2", ClusterID: "c2", Enabled: true, Priority: 2},
			{ID: "m3", ClusterID: "c3", Enabled: true, Priority: 3},
		},
		ReadStrategy: ReadStrategyNearest,
	})

	// Set latencies
	router.UpdateLatency("m1", 100*time.Millisecond)
	router.UpdateLatency("m2", 50*time.Millisecond)
	router.UpdateLatency("m3", 200*time.Millisecond)

	mirrors := router.SelectForRead(group, nil)
	if len(mirrors) != 3 {
		t.Fatalf("Expected 3 mirrors, got %d", len(mirrors))
	}

	// Should be sorted by latency
	if mirrors[0].ID != "m2" {
		t.Errorf("Expected m2 first (lowest latency), got %s", mirrors[0].ID)
	}
	if mirrors[1].ID != "m1" {
		t.Errorf("Expected m1 second, got %s", mirrors[1].ID)
	}
	if mirrors[2].ID != "m3" {
		t.Errorf("Expected m3 last, got %s", mirrors[2].ID)
	}
}

func TestRoundRobinRouter(t *testing.T) {
	router := NewRoundRobinRouter()

	group, _ := NewMirrorGroup(&MirrorGroupConfig{
		ID: "test-group",
		Mirrors: []*Mirror{
			{ID: "m1", ClusterID: "c1", Enabled: true, Weight: 1},
			{ID: "m2", ClusterID: "c2", Enabled: true, Weight: 1},
		},
		ReadStrategy: ReadStrategyRoundRobin,
	})

	// Call multiple times and track first selections
	selections := make(map[string]int)
	for i := 0; i < 100; i++ {
		mirrors := router.SelectForRead(group, nil)
		if len(mirrors) > 0 {
			selections[mirrors[0].ID]++
		}
	}

	// Should be roughly even distribution
	if selections["m1"] < 40 || selections["m1"] > 60 {
		t.Errorf("Expected ~50 selections for m1, got %d", selections["m1"])
	}
	if selections["m2"] < 40 || selections["m2"] > 60 {
		t.Errorf("Expected ~50 selections for m2, got %d", selections["m2"])
	}
}

func TestFailoverRouter(t *testing.T) {
	router := NewFailoverRouter()

	group, _ := NewMirrorGroup(&MirrorGroupConfig{
		ID: "test-group",
		Mirrors: []*Mirror{
			{ID: "m1", ClusterID: "c1", Enabled: true, Priority: 3},
			{ID: "m2", ClusterID: "c2", Enabled: true, Priority: 1},
			{ID: "m3", ClusterID: "c3", Enabled: true, Priority: 2},
		},
		ReadStrategy: ReadStrategyFailover,
	})

	mirrors := router.SelectForRead(group, nil)
	if len(mirrors) != 3 {
		t.Fatalf("Expected 3 mirrors, got %d", len(mirrors))
	}

	// Should be sorted by priority
	if mirrors[0].ID != "m2" {
		t.Errorf("Expected m2 first (priority 1), got %s", mirrors[0].ID)
	}
	if mirrors[1].ID != "m3" {
		t.Errorf("Expected m3 second (priority 2), got %s", mirrors[1].ID)
	}
	if mirrors[2].ID != "m1" {
		t.Errorf("Expected m1 last (priority 3), got %s", mirrors[2].ID)
	}
}

func TestWriteRouters(t *testing.T) {
	group, _ := NewMirrorGroup(&MirrorGroupConfig{
		ID: "test-group",
		Mirrors: []*Mirror{
			{ID: "m1", ClusterID: "c1", Enabled: true, Priority: 1, ReadOnly: false},
			{ID: "m2", ClusterID: "c2", Enabled: true, Priority: 2, ReadOnly: false},
			{ID: "m3", ClusterID: "c3", Enabled: true, Priority: 3, ReadOnly: true},
		},
	})

	t.Run("AllWriteRouter", func(t *testing.T) {
		router := NewAllWriteRouter()
		mirrors, err := router.SelectForWrite(group)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if len(mirrors) != 2 {
			t.Errorf("Expected 2 writable mirrors, got %d", len(mirrors))
		}
	})

	t.Run("PrimaryOnlyWriteRouter", func(t *testing.T) {
		router := NewPrimaryOnlyWriteRouter()
		mirrors, err := router.SelectForWrite(group)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if len(mirrors) != 1 {
			t.Errorf("Expected 1 mirror, got %d", len(mirrors))
		}
		if mirrors[0].ID != "m1" {
			t.Errorf("Expected m1 (primary), got %s", mirrors[0].ID)
		}
	})

	t.Run("PrimarySecondaryWriteRouter", func(t *testing.T) {
		router := NewPrimarySecondaryWriteRouter()
		mirrors, err := router.SelectForWrite(group)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if len(mirrors) != 2 {
			t.Errorf("Expected 2 mirrors, got %d", len(mirrors))
		}
	})

	t.Run("QuorumWriteRouter", func(t *testing.T) {
		router := NewQuorumWriteRouter(2)
		mirrors, err := router.SelectForWrite(group)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if len(mirrors) < 2 {
			t.Errorf("Expected at least 2 mirrors, got %d", len(mirrors))
		}
	})
}

func TestCircuitBreaker(t *testing.T) {
	cb := NewCircuitBreaker("m1", 3, 100*time.Millisecond)

	// Initially closed
	if cb.State() != CircuitClosed {
		t.Error("Expected closed state initially")
	}
	if !cb.Allow() {
		t.Error("Should allow requests when closed")
	}

	// Record failures
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitClosed {
		t.Error("Should still be closed after 2 failures")
	}

	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Error("Should be open after 3 failures")
	}
	if cb.Allow() {
		t.Error("Should not allow requests when open")
	}

	// Wait for reset timeout
	time.Sleep(150 * time.Millisecond)
	if !cb.Allow() {
		t.Error("Should allow request after reset timeout (half-open)")
	}
	if cb.State() != CircuitHalfOpen {
		t.Error("Expected half-open state")
	}

	// Success should close circuit
	cb.RecordSuccess()
	cb.RecordSuccess()
	if cb.State() != CircuitClosed {
		t.Error("Should be closed after successes in half-open")
	}
}

func TestParseLocation(t *testing.T) {
	tests := []struct {
		input    string
		expected *Location
	}{
		{
			input:    "us-east",
			expected: &Location{Region: "us-east"},
		},
		{
			input:    "us-east/us-east-1a",
			expected: &Location{Region: "us-east", Zone: "us-east-1a"},
		},
		{
			input:    "us-east/us-east-1a/dc1",
			expected: &Location{Region: "us-east", Zone: "us-east-1a", Datacenter: "dc1"},
		},
		{
			input:    "37.7749,-122.4194",
			expected: &Location{Latitude: 37.7749, Longitude: -122.4194},
		},
		{
			input:    "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		got := ParseLocation(tt.input)
		if tt.expected == nil {
			if got != nil {
				t.Errorf("ParseLocation(%s) = %v, want nil", tt.input, got)
			}
			continue
		}
		if got == nil {
			t.Errorf("ParseLocation(%s) = nil, want %v", tt.input, tt.expected)
			continue
		}
		if got.Region != tt.expected.Region {
			t.Errorf("ParseLocation(%s).Region = %s, want %s", tt.input, got.Region, tt.expected.Region)
		}
		if got.Zone != tt.expected.Zone {
			t.Errorf("ParseLocation(%s).Zone = %s, want %s", tt.input, got.Zone, tt.expected.Zone)
		}
		if got.Datacenter != tt.expected.Datacenter {
			t.Errorf("ParseLocation(%s).Datacenter = %s, want %s", tt.input, got.Datacenter, tt.expected.Datacenter)
		}
	}
}

func TestHaversineDistance(t *testing.T) {
	// San Francisco to New York is about 4130 km
	sf := &Location{Latitude: 37.7749, Longitude: -122.4194}
	ny := &Location{Latitude: 40.7128, Longitude: -74.0060}

	dist := DistanceKm(sf, ny)
	if dist < 4000 || dist > 4200 {
		t.Errorf("Expected ~4130 km, got %.2f km", dist)
	}

	// Same location should be 0
	dist = DistanceKm(sf, sf)
	if dist > 0.1 {
		t.Errorf("Expected ~0 km for same location, got %.2f km", dist)
	}

	// Nil locations
	if dist := DistanceKm(nil, ny); dist != -1 {
		t.Errorf("Expected -1 for nil location, got %.2f", dist)
	}
}

func TestGeoRouter(t *testing.T) {
	router := NewGeoRouter(NewFailoverRouter())

	group, _ := NewMirrorGroup(&MirrorGroupConfig{
		ID: "test-group",
		Mirrors: []*Mirror{
			{ID: "m1", ClusterID: "c1", Enabled: true, Location: &Location{Region: "us-east"}},
			{ID: "m2", ClusterID: "c2", Enabled: true, Location: &Location{Region: "us-west"}},
			{ID: "m3", ClusterID: "c3", Enabled: true, Location: &Location{Region: "eu-west"}},
		},
	})

	// Agent in us-east should prefer us-east mirror
	agentLocation := &Location{Region: "us-east"}
	mirrors := router.SelectForRead(group, agentLocation)

	if len(mirrors) == 0 {
		t.Fatal("Expected mirrors to be returned")
	}
	if mirrors[0].ID != "m1" {
		t.Errorf("Expected m1 (us-east) first for us-east agent, got %s", mirrors[0].ID)
	}

	// No location should fall back to failover router
	mirrors = router.SelectForRead(group, nil)
	if len(mirrors) == 0 {
		t.Fatal("Expected mirrors with nil location")
	}
}
