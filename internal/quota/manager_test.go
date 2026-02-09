package quota

import (
	"context"
	"testing"
	"time"
)

func TestResourceType(t *testing.T) {
	types := []ResourceType{
		ResourceCPU,
		ResourceMemory,
		ResourceStorage,
		ResourcePods,
		ResourceSecrets,
		ResourceConfigMaps,
		ResourceServices,
		ResourcePVCs,
		ResourceGPU,
		ResourceLoadBalancers,
		ResourceNodePorts,
		ResourceCustom,
	}

	for _, rt := range types {
		if rt == "" {
			t.Error("Resource type should not be empty")
		}
	}
}

func TestQuantity_String(t *testing.T) {
	tests := []struct {
		quantity Quantity
		expected string
	}{
		{Quantity{Value: 100}, "100"},
		{Quantity{Value: 100, Unit: "m"}, "100m"},
		{Quantity{Value: 1024, Unit: "Mi"}, "1024Mi"},
		{Quantity{Value: 4, Unit: "Gi"}, "4Gi"},
	}

	for _, tt := range tests {
		result := tt.quantity.String()
		if result != tt.expected {
			t.Errorf("String() = %s, want %s", result, tt.expected)
		}
	}
}

func TestQuota_Available(t *testing.T) {
	quota := &Quota{
		Hard: map[ResourceType]Quantity{
			ResourceCPU:    {Value: 1000, Unit: "m"},
			ResourceMemory: {Value: 4096, Unit: "Mi"},
		},
		Used: map[ResourceType]Quantity{
			ResourceCPU:    {Value: 500, Unit: "m"},
			ResourceMemory: {Value: 2048, Unit: "Mi"},
		},
	}

	available := quota.Available()

	if available[ResourceCPU].Value != 500 {
		t.Errorf("Available CPU = %d, want 500", available[ResourceCPU].Value)
	}
	if available[ResourceMemory].Value != 2048 {
		t.Errorf("Available Memory = %d, want 2048", available[ResourceMemory].Value)
	}
}

func TestQuota_UsagePercentage(t *testing.T) {
	quota := &Quota{
		Hard: map[ResourceType]Quantity{
			ResourceCPU:    {Value: 1000},
			ResourceMemory: {Value: 4000},
		},
		Used: map[ResourceType]Quantity{
			ResourceCPU:    {Value: 500},
			ResourceMemory: {Value: 3000},
		},
	}

	percentages := quota.UsagePercentage()

	if percentages[ResourceCPU] != 50 {
		t.Errorf("CPU percentage = %f, want 50", percentages[ResourceCPU])
	}
	if percentages[ResourceMemory] != 75 {
		t.Errorf("Memory percentage = %f, want 75", percentages[ResourceMemory])
	}
}

func TestQuota_IsExceeded(t *testing.T) {
	tests := []struct {
		name     string
		hard     map[ResourceType]Quantity
		used     map[ResourceType]Quantity
		exceeded bool
	}{
		{
			name:     "not exceeded",
			hard:     map[ResourceType]Quantity{ResourceCPU: {Value: 1000}},
			used:     map[ResourceType]Quantity{ResourceCPU: {Value: 500}},
			exceeded: false,
		},
		{
			name:     "exceeded",
			hard:     map[ResourceType]Quantity{ResourceCPU: {Value: 1000}},
			used:     map[ResourceType]Quantity{ResourceCPU: {Value: 1500}},
			exceeded: true,
		},
		{
			name:     "at limit",
			hard:     map[ResourceType]Quantity{ResourceCPU: {Value: 1000}},
			used:     map[ResourceType]Quantity{ResourceCPU: {Value: 1000}},
			exceeded: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quota := &Quota{Hard: tt.hard, Used: tt.used}
			if quota.IsExceeded() != tt.exceeded {
				t.Errorf("IsExceeded() = %v, want %v", quota.IsExceeded(), tt.exceeded)
			}
		})
	}
}

func TestQuota_ExceededResources(t *testing.T) {
	quota := &Quota{
		Hard: map[ResourceType]Quantity{
			ResourceCPU:    {Value: 1000},
			ResourceMemory: {Value: 4000},
			ResourcePods:   {Value: 100},
		},
		Used: map[ResourceType]Quantity{
			ResourceCPU:    {Value: 1500}, // exceeded
			ResourceMemory: {Value: 3000}, // not exceeded
			ResourcePods:   {Value: 150},  // exceeded
		},
	}

	exceeded := quota.ExceededResources()

	if len(exceeded) != 2 {
		t.Errorf("Exceeded resources = %d, want 2", len(exceeded))
	}
}

func TestNewManager(t *testing.T) {
	store := NewInMemoryStore()
	manager := NewManager(store)

	if manager == nil {
		t.Fatal("Expected non-nil manager")
	}
}

func TestManager_Create(t *testing.T) {
	store := NewInMemoryStore()
	manager := NewManager(store)

	var events []*Event
	manager.AddListener(func(e *Event) {
		events = append(events, e)
	})

	ctx := context.Background()
	quota := &Quota{
		ID:   "quota-1",
		Name: "test-quota",
		Scope: &Scope{
			Type: "namespace",
			Name: "default",
		},
		Hard: map[ResourceType]Quantity{
			ResourceCPU:    {Value: 4000, Unit: "m"},
			ResourceMemory: {Value: 8192, Unit: "Mi"},
		},
	}

	err := manager.Create(ctx, quota)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify it was stored
	retrieved, err := manager.Get(ctx, "quota-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved.Name != "test-quota" {
		t.Errorf("Name = %s, want test-quota", retrieved.Name)
	}

	// Check event
	if len(events) != 1 || events[0].Type != "created" {
		t.Error("Expected created event")
	}
}

func TestManager_SetDefault(t *testing.T) {
	store := NewInMemoryStore()
	manager := NewManager(store)

	defaultQuota := &Quota{
		ID:   "default-namespace",
		Name: "default-namespace-quota",
		Hard: map[ResourceType]Quantity{
			ResourceCPU: {Value: 2000},
		},
	}

	manager.SetDefault("namespace", defaultQuota)

	retrieved, ok := manager.GetDefault("namespace")
	if !ok {
		t.Error("Expected to find default quota")
	}
	if retrieved.Name != "default-namespace-quota" {
		t.Errorf("Name = %s, want default-namespace-quota", retrieved.Name)
	}
}

func TestManager_CheckAllocation(t *testing.T) {
	store := NewInMemoryStore()
	manager := NewManager(store)

	ctx := context.Background()
	quota := &Quota{
		ID:   "quota-1",
		Name: "test-quota",
		Scope: &Scope{
			Type: "namespace",
			Name: "default",
		},
		Hard: map[ResourceType]Quantity{
			ResourceCPU:    {Value: 4000},
			ResourceMemory: {Value: 8192},
		},
		Used: map[ResourceType]Quantity{
			ResourceCPU:    {Value: 3000},
			ResourceMemory: {Value: 4096},
		},
	}
	manager.Create(ctx, quota)

	tests := []struct {
		name    string
		request *Request
		allowed bool
	}{
		{
			name: "allowed",
			request: &Request{
				Resources: map[ResourceType]Quantity{
					ResourceCPU: {Value: 500},
				},
			},
			allowed: true,
		},
		{
			name: "not allowed - insufficient",
			request: &Request{
				Resources: map[ResourceType]Quantity{
					ResourceCPU: {Value: 2000},
				},
			},
			allowed: false,
		},
	}

	scope := &Scope{Type: "namespace", Name: "default"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := manager.CheckAllocation(ctx, scope, tt.request)
			if err != nil {
				t.Fatalf("CheckAllocation failed: %v", err)
			}
			if result.Allowed != tt.allowed {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.allowed)
			}
		})
	}
}

func TestManager_Allocate(t *testing.T) {
	store := NewInMemoryStore()
	manager := NewManager(store)

	ctx := context.Background()
	quota := &Quota{
		ID:   "quota-1",
		Name: "test-quota",
		Scope: &Scope{
			Type: "namespace",
			Name: "default",
		},
		Hard: map[ResourceType]Quantity{
			ResourceCPU: {Value: 4000},
		},
		Used: map[ResourceType]Quantity{
			ResourceCPU: {Value: 1000},
		},
	}
	manager.Create(ctx, quota)

	scope := &Scope{Type: "namespace", Name: "default"}
	request := &Request{
		Resources: map[ResourceType]Quantity{
			ResourceCPU: {Value: 500},
		},
	}

	result, err := manager.Allocate(ctx, scope, request)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}
	if !result.Allowed {
		t.Error("Allocation should be allowed")
	}

	// Check usage was updated
	updated, _ := manager.Get(ctx, "quota-1")
	if updated.Used[ResourceCPU].Value != 1500 {
		t.Errorf("Used CPU = %d, want 1500", updated.Used[ResourceCPU].Value)
	}
}

func TestManager_Release(t *testing.T) {
	store := NewInMemoryStore()
	manager := NewManager(store)

	ctx := context.Background()
	quota := &Quota{
		ID:   "quota-1",
		Name: "test-quota",
		Scope: &Scope{
			Type: "namespace",
			Name: "default",
		},
		Hard: map[ResourceType]Quantity{
			ResourceCPU: {Value: 4000},
		},
		Used: map[ResourceType]Quantity{
			ResourceCPU: {Value: 2000},
		},
	}
	manager.Create(ctx, quota)

	scope := &Scope{Type: "namespace", Name: "default"}
	request := &Request{
		Resources: map[ResourceType]Quantity{
			ResourceCPU: {Value: 500},
		},
	}

	err := manager.Release(ctx, scope, request)
	if err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	// Check usage was updated
	updated, _ := manager.Get(ctx, "quota-1")
	if updated.Used[ResourceCPU].Value != 1500 {
		t.Errorf("Used CPU = %d, want 1500", updated.Used[ResourceCPU].Value)
	}
}

func TestManager_GetUsageReport(t *testing.T) {
	store := NewInMemoryStore()
	manager := NewManager(store)

	ctx := context.Background()
	quota := &Quota{
		ID:   "quota-1",
		Name: "test-quota",
		Scope: &Scope{
			Type: "namespace",
			Name: "default",
		},
		Hard: map[ResourceType]Quantity{
			ResourceCPU:    {Value: 4000, Unit: "m"},
			ResourceMemory: {Value: 8192, Unit: "Mi"},
		},
		Used: map[ResourceType]Quantity{
			ResourceCPU:    {Value: 2000, Unit: "m"},
			ResourceMemory: {Value: 4096, Unit: "Mi"},
		},
	}
	manager.Create(ctx, quota)

	report, err := manager.GetUsageReport(ctx, "quota-1")
	if err != nil {
		t.Fatalf("GetUsageReport failed: %v", err)
	}

	if report.QuotaID != "quota-1" {
		t.Errorf("QuotaID = %s, want quota-1", report.QuotaID)
	}
	if len(report.Resources) != 2 {
		t.Errorf("Resources = %d, want 2", len(report.Resources))
	}
}

func TestInMemoryStore(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	quota := &Quota{
		ID:   "quota-1",
		Name: "test-quota",
		Scope: &Scope{
			Type: "namespace",
			Name: "default",
		},
		Hard: map[ResourceType]Quantity{
			ResourceCPU: {Value: 4000},
		},
	}

	// Test Save
	err := store.Save(ctx, quota)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Test Get
	retrieved, err := store.Get(ctx, "quota-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved.Name != "test-quota" {
		t.Errorf("Name = %s, want test-quota", retrieved.Name)
	}

	// Test GetByScope
	byScope, err := store.GetByScope(ctx, &Scope{Type: "namespace", Name: "default"})
	if err != nil {
		t.Fatalf("GetByScope failed: %v", err)
	}
	if byScope.ID != "quota-1" {
		t.Errorf("ID = %s, want quota-1", byScope.ID)
	}

	// Test List
	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List count = %d, want 1", len(list))
	}

	// Test ListByType
	byType, err := store.ListByType(ctx, "namespace")
	if err != nil {
		t.Fatalf("ListByType failed: %v", err)
	}
	if len(byType) != 1 {
		t.Errorf("ListByType count = %d, want 1", len(byType))
	}

	// Test Delete
	err = store.Delete(ctx, "quota-1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = store.Get(ctx, "quota-1")
	if err == nil {
		t.Error("Expected error after delete")
	}
}

func TestQuotaEnforcer(t *testing.T) {
	store := NewInMemoryStore()
	manager := NewManager(store)
	enforcer := NewEnforcer(manager)

	ctx := context.Background()
	quota := &Quota{
		ID:    "quota-1",
		Scope: &Scope{Type: "namespace", Name: "default"},
		Hard: map[ResourceType]Quantity{
			ResourceCPU: {Value: 1000},
		},
		Used: map[ResourceType]Quantity{
			ResourceCPU: {Value: 800},
		},
	}
	manager.Create(ctx, quota)

	// Test allowed
	err := enforcer.Enforce(ctx, &Scope{Type: "namespace", Name: "default"}, &Request{
		Resources: map[ResourceType]Quantity{
			ResourceCPU: {Value: 100},
		},
	})
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Test exceeded
	err = enforcer.Enforce(ctx, &Scope{Type: "namespace", Name: "default"}, &Request{
		Resources: map[ResourceType]Quantity{
			ResourceCPU: {Value: 500},
		},
	})
	if err == nil {
		t.Error("Expected quota exceeded error")
	}

	// Test disabled
	enforcer.Disable()
	if enforcer.IsEnabled() {
		t.Error("Enforcer should be disabled")
	}

	err = enforcer.Enforce(ctx, &Scope{Type: "namespace", Name: "default"}, &Request{
		Resources: map[ResourceType]Quantity{
			ResourceCPU: {Value: 5000},
		},
	})
	if err != nil {
		t.Error("Disabled enforcer should allow all")
	}
}

func TestQuotaAggregator(t *testing.T) {
	store := NewInMemoryStore()
	manager := NewManager(store)
	aggregator := NewAggregator(manager)

	ctx := context.Background()

	// Create multiple quotas
	quotas := []*Quota{
		{
			ID:    "q1",
			Scope: &Scope{Type: "namespace", Name: "ns1"},
			Hard:  map[ResourceType]Quantity{ResourceCPU: {Value: 1000}},
			Used:  map[ResourceType]Quantity{ResourceCPU: {Value: 500}},
		},
		{
			ID:    "q2",
			Scope: &Scope{Type: "namespace", Name: "ns2"},
			Hard:  map[ResourceType]Quantity{ResourceCPU: {Value: 2000}},
			Used:  map[ResourceType]Quantity{ResourceCPU: {Value: 1000}},
		},
	}

	for _, q := range quotas {
		manager.Create(ctx, q)
	}

	agg, err := aggregator.Aggregate(ctx, "namespace")
	if err != nil {
		t.Fatalf("Aggregate failed: %v", err)
	}

	if agg.QuotaCount != 2 {
		t.Errorf("QuotaCount = %d, want 2", agg.QuotaCount)
	}
	if agg.TotalHard[ResourceCPU].Value != 3000 {
		t.Errorf("TotalHard CPU = %d, want 3000", agg.TotalHard[ResourceCPU].Value)
	}
	if agg.TotalUsed[ResourceCPU].Value != 1500 {
		t.Errorf("TotalUsed CPU = %d, want 1500", agg.TotalUsed[ResourceCPU].Value)
	}
	if agg.Utilization[ResourceCPU] != 50 {
		t.Errorf("Utilization CPU = %f, want 50", agg.Utilization[ResourceCPU])
	}
}

func TestExceededError(t *testing.T) {
	err := &ExceededError{
		Scope:        &Scope{Type: "namespace", Name: "default"},
		Insufficient: []ResourceType{ResourceCPU, ResourceMemory},
		Message:      "quota exceeded",
	}

	if err.Error() != "quota exceeded" {
		t.Errorf("Error() = %s, want 'quota exceeded'", err.Error())
	}
}

func TestScope(t *testing.T) {
	scope := &Scope{
		Type:      "namespace",
		Name:      "production",
		Namespace: "production",
		Cluster:   "cluster-1",
	}

	if scope.Type != "namespace" {
		t.Errorf("Type = %s, want namespace", scope.Type)
	}
	if scope.Cluster != "cluster-1" {
		t.Errorf("Cluster = %s, want cluster-1", scope.Cluster)
	}
}

func TestRequest(t *testing.T) {
	request := &Request{
		Resources: map[ResourceType]Quantity{
			ResourceCPU:    {Value: 500, Unit: "m"},
			ResourceMemory: {Value: 1024, Unit: "Mi"},
		},
		Requester: "user@example.com",
		Reason:    "new deployment",
	}

	if len(request.Resources) != 2 {
		t.Errorf("Resources = %d, want 2", len(request.Resources))
	}
}

func TestAllocationResult(t *testing.T) {
	result := &AllocationResult{
		Allowed: false,
		Reason:  "insufficient CPU",
		Requested: map[ResourceType]Quantity{
			ResourceCPU: {Value: 2000},
		},
		Available: map[ResourceType]Quantity{
			ResourceCPU: {Value: 1000},
		},
		Insufficient: []ResourceType{ResourceCPU},
	}

	if result.Allowed {
		t.Error("Allowed should be false")
	}
	if len(result.Insufficient) != 1 {
		t.Errorf("Insufficient = %d, want 1", len(result.Insufficient))
	}
}

func TestUsageReport(t *testing.T) {
	report := &UsageReport{
		QuotaID:     "quota-1",
		QuotaName:   "test-quota",
		Scope:       &Scope{Type: "namespace", Name: "default"},
		GeneratedAt: time.Now(),
		Resources: []ResourceUsage{
			{
				Type:       ResourceCPU,
				Hard:       Quantity{Value: 4000, Unit: "m"},
				Used:       Quantity{Value: 2000, Unit: "m"},
				Available:  Quantity{Value: 2000, Unit: "m"},
				Percentage: 50,
			},
		},
	}

	if report.QuotaID != "quota-1" {
		t.Errorf("QuotaID = %s, want quota-1", report.QuotaID)
	}
	if len(report.Resources) != 1 {
		t.Errorf("Resources = %d, want 1", len(report.Resources))
	}
}

func TestResourceUsage(t *testing.T) {
	usage := ResourceUsage{
		Type:       ResourceMemory,
		Hard:       Quantity{Value: 8192, Unit: "Mi"},
		Used:       Quantity{Value: 6144, Unit: "Mi"},
		Available:  Quantity{Value: 2048, Unit: "Mi"},
		Percentage: 75,
	}

	if usage.Percentage != 75 {
		t.Errorf("Percentage = %f, want 75", usage.Percentage)
	}
}

func TestEvent(t *testing.T) {
	event := &Event{
		Type:      "warning",
		QuotaID:   "quota-1",
		Scope:     &Scope{Type: "namespace", Name: "default"},
		Timestamp: time.Now(),
		Message:   "CPU usage at 90%",
		Details: map[string]interface{}{
			"resource":   ResourceCPU,
			"percentage": 90.0,
		},
	}

	if event.Type != "warning" {
		t.Errorf("Type = %s, want warning", event.Type)
	}
}
