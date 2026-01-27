package dns

import (
	"context"
	"fmt"
	"testing"
)

func TestSyncer_Sync_DryRun(t *testing.T) {
	ctx := context.Background()
	provider := NewMockProvider()
	provider.SetRecords("example.com", []Record{
		{Type: RecordTypeA, Name: "existing", Value: "192.0.2.1", TTL: 300, ID: "rec1"},
	})

	syncer := NewSyncer(provider, "example.com", SyncOptions{DryRun: true})

	desired := []Record{
		{Type: RecordTypeA, Name: "existing", Value: "192.0.2.1", TTL: 300}, // unchanged
		{Type: RecordTypeA, Name: "new", Value: "192.0.2.2", TTL: 300},      // create
	}

	result, err := syncer.Sync(ctx, desired)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	// Dry run should not call provider methods
	if provider.CreateCalled != 0 {
		t.Error("DryRun should not create records")
	}

	// But should report what would happen
	if result.Created != 1 {
		t.Errorf("DryRun Created = %d, want 1", result.Created)
	}
	if result.Unchanged != 1 {
		t.Errorf("DryRun Unchanged = %d, want 1", result.Unchanged)
	}
}

func TestSyncer_Sync_Create(t *testing.T) {
	ctx := context.Background()
	provider := NewMockProvider()
	// Empty zone - no records

	syncer := NewSyncer(provider, "example.com", SyncOptions{})

	desired := []Record{
		{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300},
		{Type: RecordTypeA, Name: "api", Value: "192.0.2.2", TTL: 300},
	}

	result, err := syncer.Sync(ctx, desired)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	if result.Created != 2 {
		t.Errorf("Created = %d, want 2", result.Created)
	}
	if provider.CreateCalled != 2 {
		t.Errorf("CreateCalled = %d, want 2", provider.CreateCalled)
	}
}

func TestSyncer_Sync_Update(t *testing.T) {
	ctx := context.Background()
	provider := NewMockProvider()
	provider.SetRecords("example.com", []Record{
		{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, ID: "rec1"},
	})

	syncer := NewSyncer(provider, "example.com", SyncOptions{})

	desired := []Record{
		{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 600}, // TTL change
	}

	result, err := syncer.Sync(ctx, desired)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	if result.Updated != 1 {
		t.Errorf("Updated = %d, want 1", result.Updated)
	}
	if provider.UpdateCalled != 1 {
		t.Errorf("UpdateCalled = %d, want 1", provider.UpdateCalled)
	}
}

func TestSyncer_Sync_Delete(t *testing.T) {
	ctx := context.Background()
	provider := NewMockProvider()
	provider.SetRecords("example.com", []Record{
		{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, ID: "rec1"},
		{Type: RecordTypeA, Name: "old", Value: "192.0.2.2", TTL: 300, ID: "rec2"},
	})

	syncer := NewSyncer(provider, "example.com", SyncOptions{DeleteExisting: true})

	desired := []Record{
		{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300},
		// 'old' is not in desired - should be deleted
	}

	result, err := syncer.Sync(ctx, desired)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	if result.Deleted != 1 {
		t.Errorf("Deleted = %d, want 1", result.Deleted)
	}
	if provider.DeleteCalled != 1 {
		t.Errorf("DeleteCalled = %d, want 1", provider.DeleteCalled)
	}
}

func TestSyncer_Sync_NoDeleteWithoutFlag(t *testing.T) {
	ctx := context.Background()
	provider := NewMockProvider()
	provider.SetRecords("example.com", []Record{
		{Type: RecordTypeA, Name: "old", Value: "192.0.2.1", TTL: 300, ID: "rec1"},
	})

	// DeleteExisting is false (default)
	syncer := NewSyncer(provider, "example.com", SyncOptions{})

	result, err := syncer.Sync(ctx, []Record{})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	// Should not delete without DeleteExisting flag
	if provider.DeleteCalled != 0 {
		t.Errorf("DeleteCalled = %d, want 0 (DeleteExisting not set)", provider.DeleteCalled)
	}
	if result.Deleted != 0 {
		t.Errorf("Deleted = %d, want 0", result.Deleted)
	}
}

func TestSyncer_Sync_WithErrors(t *testing.T) {
	ctx := context.Background()
	provider := NewMockProvider()
	provider.CreateErr = fmt.Errorf("API error")

	syncer := NewSyncer(provider, "example.com", SyncOptions{})

	desired := []Record{
		{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300},
	}

	result, err := syncer.Sync(ctx, desired)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	if !result.HasErrors() {
		t.Error("Result should have errors")
	}
	if result.Created != 0 {
		t.Errorf("Created = %d, want 0 (should fail)", result.Created)
	}
}

func TestSyncer_Sync_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	provider := NewMockProvider()

	syncer := NewSyncer(provider, "example.com", SyncOptions{})

	desired := []Record{
		{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300},
	}

	result, err := syncer.Sync(ctx, desired)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	if !result.HasErrors() {
		t.Error("Result should have context error")
	}
}

func TestSyncer_Sync_GetRecordsError(t *testing.T) {
	ctx := context.Background()
	provider := NewMockProvider()
	provider.GetErr = fmt.Errorf("API error")

	syncer := NewSyncer(provider, "example.com", SyncOptions{})

	result, err := syncer.Sync(ctx, []Record{})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	if !result.HasErrors() {
		t.Error("Result should have errors from GetRecords")
	}
}

func TestSyncer_Check(t *testing.T) {
	ctx := context.Background()
	provider := NewMockProvider()
	provider.SetRecords("example.com", []Record{
		{Type: RecordTypeA, Name: "existing", Value: "192.0.2.1", TTL: 300, ID: "rec1"},
	})

	syncer := NewSyncer(provider, "example.com", SyncOptions{})

	desired := []Record{
		{Type: RecordTypeA, Name: "existing", Value: "192.0.2.1", TTL: 600}, // TTL change
		{Type: RecordTypeA, Name: "new", Value: "192.0.2.2", TTL: 300},      // create
	}

	plan, err := syncer.Check(ctx, desired)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	summary := plan.Summary()
	if summary.Create != 1 {
		t.Errorf("Create = %d, want 1", summary.Create)
	}
	if summary.Update != 1 {
		t.Errorf("Update = %d, want 1", summary.Update)
	}

	// Check should not modify anything
	if provider.CreateCalled != 0 || provider.UpdateCalled != 0 {
		t.Error("Check() should not modify records")
	}
}

func TestSyncer_Check_Error(t *testing.T) {
	ctx := context.Background()
	provider := NewMockProvider()
	provider.GetErr = fmt.Errorf("API error")

	syncer := NewSyncer(provider, "example.com", SyncOptions{})

	_, err := syncer.Check(ctx, []Record{})
	if err == nil {
		t.Error("Check() should return error from GetRecords")
	}
}

func TestSyncer_DryRun(t *testing.T) {
	ctx := context.Background()
	provider := NewMockProvider()
	provider.SetRecords("example.com", []Record{
		{Type: RecordTypeA, Name: "existing", Value: "192.0.2.1", TTL: 300, ID: "rec1"},
	})

	syncer := NewSyncer(provider, "example.com", SyncOptions{})

	desired := []Record{
		{Type: RecordTypeA, Name: "new", Value: "192.0.2.2", TTL: 300},
	}

	result, err := syncer.DryRun(ctx, desired)
	if err != nil {
		t.Fatalf("DryRun() error = %v", err)
	}

	if result.Zone != "example.com" {
		t.Errorf("Zone = %v, want example.com", result.Zone)
	}

	if len(result.Operations) != 2 { // 1 create + 1 delete
		t.Errorf("Operations = %d, want 2", len(result.Operations))
	}

	if result.Summary.Create != 1 {
		t.Errorf("Summary.Create = %d, want 1", result.Summary.Create)
	}
}

func TestPlanOperations(t *testing.T) {
	plan := &Plan{
		Zone: "example.com",
		Changes: []RecordChange{
			{
				Type:   ChangeTypeCreate,
				Record: &Record{Type: RecordTypeA, Name: "new", Value: "192.0.2.1"},
			},
			{
				Type:    ChangeTypeUpdate,
				Record:  &Record{Type: RecordTypeA, Name: "existing", Value: "192.0.2.2", TTL: 600},
				Current: &Record{Type: RecordTypeA, Name: "existing", Value: "192.0.2.2", TTL: 300},
				Diff:    map[string]FieldDiff{"ttl": {Old: 300, New: 600}},
			},
			{
				Type:    ChangeTypeDelete,
				Record:  &Record{Type: RecordTypeA, Name: "old", Value: "192.0.2.3"},
				Current: &Record{Type: RecordTypeA, Name: "old", Value: "192.0.2.3"},
			},
		},
	}

	ops := PlanOperations(plan)

	if len(ops) != 3 {
		t.Fatalf("PlanOperations() returned %d ops, want 3", len(ops))
	}

	// Check create operation
	if ops[0].Action != ChangeTypeCreate {
		t.Errorf("ops[0].Action = %v, want create", ops[0].Action)
	}
	if ops[0].Zone != "example.com" {
		t.Errorf("ops[0].Zone = %v, want example.com", ops[0].Zone)
	}

	// Check update operation has changes
	if ops[1].Action != ChangeTypeUpdate {
		t.Errorf("ops[1].Action = %v, want update", ops[1].Action)
	}
	if ops[1].Before == nil {
		t.Error("ops[1].Before should not be nil for update")
	}
	if ops[1].Changes == nil {
		t.Error("ops[1].Changes should not be nil for update")
	}

	// Check delete operation has before
	if ops[2].Action != ChangeTypeDelete {
		t.Errorf("ops[2].Action = %v, want delete", ops[2].Action)
	}
	if ops[2].Before == nil {
		t.Error("ops[2].Before should not be nil for delete")
	}
}

func TestFilterOperations(t *testing.T) {
	ops := []Operation{
		{Action: ChangeTypeCreate},
		{Action: ChangeTypeUpdate},
		{Action: ChangeTypeDelete},
		{Action: ChangeTypeNoop},
		{Action: ChangeTypeCreate},
	}

	// Filter creates
	creates := FilterOperations(ops, ChangeTypeCreate)
	if len(creates) != 2 {
		t.Errorf("FilterOperations(create) = %d, want 2", len(creates))
	}

	// Filter updates and deletes
	changes := FilterOperations(ops, ChangeTypeUpdate, ChangeTypeDelete)
	if len(changes) != 2 {
		t.Errorf("FilterOperations(update,delete) = %d, want 2", len(changes))
	}

	// No filter returns all
	all := FilterOperations(ops)
	if len(all) != 5 {
		t.Errorf("FilterOperations() = %d, want 5", len(all))
	}
}

func TestSyncer_IgnoreTTL(t *testing.T) {
	ctx := context.Background()
	provider := NewMockProvider()
	provider.SetRecords("example.com", []Record{
		{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, ID: "rec1"},
	})

	syncer := NewSyncer(provider, "example.com", SyncOptions{IgnoreTTL: true})

	desired := []Record{
		{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 600}, // TTL change
	}

	result, err := syncer.Sync(ctx, desired)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	// With IgnoreTTL, no update should happen
	if result.Updated != 0 {
		t.Errorf("With IgnoreTTL, Updated = %d, want 0", result.Updated)
	}
	if result.Unchanged != 1 {
		t.Errorf("With IgnoreTTL, Unchanged = %d, want 1", result.Unchanged)
	}
}

func TestSyncer_IgnoreProxied(t *testing.T) {
	ctx := context.Background()
	trueVal := true
	falseVal := false

	provider := NewMockProvider()
	provider.SetRecords("example.com", []Record{
		{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, Proxied: &falseVal, ID: "rec1"},
	})

	syncer := NewSyncer(provider, "example.com", SyncOptions{IgnoreProxied: true})

	desired := []Record{
		{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, Proxied: &trueVal},
	}

	result, err := syncer.Sync(ctx, desired)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	// With IgnoreProxied, no update should happen
	if result.Updated != 0 {
		t.Errorf("With IgnoreProxied, Updated = %d, want 0", result.Updated)
	}
}

func TestSyncer_Mixed(t *testing.T) {
	ctx := context.Background()
	provider := NewMockProvider()
	provider.SetRecords("example.com", []Record{
		{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, ID: "rec1"}, // unchanged
		{Type: RecordTypeA, Name: "api", Value: "192.0.2.2", TTL: 300, ID: "rec2"}, // update TTL
		{Type: RecordTypeA, Name: "old", Value: "192.0.2.3", TTL: 300, ID: "rec3"}, // delete
	})

	syncer := NewSyncer(provider, "example.com", SyncOptions{DeleteExisting: true})

	desired := []Record{
		{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300}, // unchanged
		{Type: RecordTypeA, Name: "api", Value: "192.0.2.2", TTL: 600}, // update TTL
		{Type: RecordTypeA, Name: "new", Value: "192.0.2.4", TTL: 300}, // create
	}

	result, err := syncer.Sync(ctx, desired)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	if result.Created != 1 {
		t.Errorf("Created = %d, want 1", result.Created)
	}
	if result.Updated != 1 {
		t.Errorf("Updated = %d, want 1", result.Updated)
	}
	if result.Deleted != 1 {
		t.Errorf("Deleted = %d, want 1", result.Deleted)
	}
	if result.Unchanged != 1 {
		t.Errorf("Unchanged = %d, want 1", result.Unchanged)
	}

	if provider.CreateCalled != 1 {
		t.Errorf("CreateCalled = %d, want 1", provider.CreateCalled)
	}
	if provider.UpdateCalled != 1 {
		t.Errorf("UpdateCalled = %d, want 1", provider.UpdateCalled)
	}
	if provider.DeleteCalled != 1 {
		t.Errorf("DeleteCalled = %d, want 1", provider.DeleteCalled)
	}
}

func TestSyncer_Duration(t *testing.T) {
	ctx := context.Background()
	provider := NewMockProvider()

	syncer := NewSyncer(provider, "example.com", SyncOptions{})

	result, err := syncer.Sync(ctx, []Record{})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	if result.Duration == 0 {
		t.Error("Duration should be set")
	}
}
