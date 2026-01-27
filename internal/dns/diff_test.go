package dns

import (
	"strings"
	"testing"
)

func TestDiffer_Diff(t *testing.T) {
	tests := []struct {
		name           string
		zone           string
		desired        []Record
		current        []Record
		expectedCreate int
		expectedUpdate int
		expectedDelete int
		expectedNoop   int
	}{
		{
			name: "create new records",
			zone: "example.com",
			desired: []Record{
				{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300},
				{Type: RecordTypeA, Name: "api", Value: "192.0.2.2", TTL: 300},
			},
			current:        []Record{},
			expectedCreate: 2,
			expectedUpdate: 0,
			expectedDelete: 0,
			expectedNoop:   0,
		},
		{
			name: "delete records",
			zone: "example.com",
			desired: []Record{},
			current: []Record{
				{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, ID: "rec1"},
				{Type: RecordTypeA, Name: "api", Value: "192.0.2.2", TTL: 300, ID: "rec2"},
			},
			expectedCreate: 0,
			expectedUpdate: 0,
			expectedDelete: 2,
			expectedNoop:   0,
		},
		{
			name: "update TTL",
			zone: "example.com",
			desired: []Record{
				{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 600},
			},
			current: []Record{
				{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, ID: "rec1"},
			},
			expectedCreate: 0,
			expectedUpdate: 1,
			expectedDelete: 0,
			expectedNoop:   0,
		},
		{
			name: "no changes",
			zone: "example.com",
			desired: []Record{
				{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300},
			},
			current: []Record{
				{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, ID: "rec1"},
			},
			expectedCreate: 0,
			expectedUpdate: 0,
			expectedDelete: 0,
			expectedNoop:   1,
		},
		{
			name: "mixed changes",
			zone: "example.com",
			desired: []Record{
				{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300},  // unchanged
				{Type: RecordTypeA, Name: "api", Value: "192.0.2.2", TTL: 600},  // update TTL
				{Type: RecordTypeA, Name: "new", Value: "192.0.2.3", TTL: 300},  // create
			},
			current: []Record{
				{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, ID: "rec1"},
				{Type: RecordTypeA, Name: "api", Value: "192.0.2.2", TTL: 300, ID: "rec2"},
				{Type: RecordTypeA, Name: "old", Value: "192.0.2.4", TTL: 300, ID: "rec3"}, // delete
			},
			expectedCreate: 1,
			expectedUpdate: 1,
			expectedDelete: 1,
			expectedNoop:   1,
		},
		{
			name: "update priority for MX",
			zone: "example.com",
			desired: []Record{
				{Type: RecordTypeMX, Name: "@", Value: "mail.example.com.", TTL: 300, Priority: 20},
			},
			current: []Record{
				{Type: RecordTypeMX, Name: "@", Value: "mail.example.com.", TTL: 300, Priority: 10, ID: "rec1"},
			},
			expectedCreate: 0,
			expectedUpdate: 1,
			expectedDelete: 0,
			expectedNoop:   0,
		},
		{
			name: "update SRV fields",
			zone: "example.com",
			desired: []Record{
				{Type: RecordTypeSRV, Name: "_http._tcp", Value: "server.example.com.", TTL: 300, Priority: 10, Weight: 10, Port: 8080},
			},
			current: []Record{
				{Type: RecordTypeSRV, Name: "_http._tcp", Value: "server.example.com.", TTL: 300, Priority: 10, Weight: 5, Port: 80, ID: "rec1"},
			},
			expectedCreate: 0,
			expectedUpdate: 1,
			expectedDelete: 0,
			expectedNoop:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			differ := NewDiffer(tt.zone)
			plan := differ.Diff(tt.desired, tt.current)

			summary := plan.Summary()
			if summary.Create != tt.expectedCreate {
				t.Errorf("Create = %v, want %v", summary.Create, tt.expectedCreate)
			}
			if summary.Update != tt.expectedUpdate {
				t.Errorf("Update = %v, want %v", summary.Update, tt.expectedUpdate)
			}
			if summary.Delete != tt.expectedDelete {
				t.Errorf("Delete = %v, want %v", summary.Delete, tt.expectedDelete)
			}
			if summary.Noop != tt.expectedNoop {
				t.Errorf("Noop = %v, want %v", summary.Noop, tt.expectedNoop)
			}
		})
	}
}

func TestDiffer_IgnoreTTL(t *testing.T) {
	differ := NewDiffer("example.com")
	differ.IgnoreTTL = true

	desired := []Record{
		{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 600},
	}
	current := []Record{
		{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, ID: "rec1"},
	}

	plan := differ.Diff(desired, current)
	summary := plan.Summary()

	if summary.Update != 0 {
		t.Errorf("With IgnoreTTL=true, Update = %v, want 0", summary.Update)
	}
	if summary.Noop != 1 {
		t.Errorf("With IgnoreTTL=true, Noop = %v, want 1", summary.Noop)
	}
}

func TestDiffer_IgnoreProxied(t *testing.T) {
	differ := NewDiffer("example.com")
	differ.IgnoreProxied = true

	trueVal := true
	falseVal := false

	desired := []Record{
		{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, Proxied: &trueVal},
	}
	current := []Record{
		{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, Proxied: &falseVal, ID: "rec1"},
	}

	plan := differ.Diff(desired, current)
	summary := plan.Summary()

	if summary.Update != 0 {
		t.Errorf("With IgnoreProxied=true, Update = %v, want 0", summary.Update)
	}
	if summary.Noop != 1 {
		t.Errorf("With IgnoreProxied=true, Noop = %v, want 1", summary.Noop)
	}
}

func TestDiffer_ProxiedChange(t *testing.T) {
	differ := NewDiffer("example.com")

	trueVal := true
	falseVal := false

	desired := []Record{
		{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, Proxied: &trueVal},
	}
	current := []Record{
		{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, Proxied: &falseVal, ID: "rec1"},
	}

	plan := differ.Diff(desired, current)
	summary := plan.Summary()

	if summary.Update != 1 {
		t.Errorf("Proxied change should trigger Update = 1, got %v", summary.Update)
	}

	// Check diff details
	for _, change := range plan.Changes {
		if change.Type == ChangeTypeUpdate {
			if _, ok := change.Diff["proxied"]; !ok {
				t.Error("Expected diff to contain 'proxied' field")
			}
		}
	}
}

func TestPlan_HasChanges(t *testing.T) {
	tests := []struct {
		name     string
		changes  []RecordChange
		expected bool
	}{
		{
			name:     "no changes",
			changes:  []RecordChange{},
			expected: false,
		},
		{
			name: "only noop",
			changes: []RecordChange{
				{Type: ChangeTypeNoop, Record: &Record{}},
			},
			expected: false,
		},
		{
			name: "create",
			changes: []RecordChange{
				{Type: ChangeTypeCreate, Record: &Record{}},
			},
			expected: true,
		},
		{
			name: "update",
			changes: []RecordChange{
				{Type: ChangeTypeUpdate, Record: &Record{}},
			},
			expected: true,
		},
		{
			name: "delete",
			changes: []RecordChange{
				{Type: ChangeTypeDelete, Record: &Record{}},
			},
			expected: true,
		},
		{
			name: "mixed with noop",
			changes: []RecordChange{
				{Type: ChangeTypeNoop, Record: &Record{}},
				{Type: ChangeTypeCreate, Record: &Record{}},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := &Plan{Zone: "example.com", Changes: tt.changes}
			if got := plan.HasChanges(); got != tt.expected {
				t.Errorf("Plan.HasChanges() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestPlan_Summary(t *testing.T) {
	plan := &Plan{
		Zone: "example.com",
		Changes: []RecordChange{
			{Type: ChangeTypeCreate, Record: &Record{}},
			{Type: ChangeTypeCreate, Record: &Record{}},
			{Type: ChangeTypeUpdate, Record: &Record{}},
			{Type: ChangeTypeDelete, Record: &Record{}},
			{Type: ChangeTypeNoop, Record: &Record{}},
			{Type: ChangeTypeNoop, Record: &Record{}},
			{Type: ChangeTypeNoop, Record: &Record{}},
		},
	}

	summary := plan.Summary()

	if summary.Zone != "example.com" {
		t.Errorf("Summary.Zone = %v, want example.com", summary.Zone)
	}
	if summary.Create != 2 {
		t.Errorf("Summary.Create = %v, want 2", summary.Create)
	}
	if summary.Update != 1 {
		t.Errorf("Summary.Update = %v, want 1", summary.Update)
	}
	if summary.Delete != 1 {
		t.Errorf("Summary.Delete = %v, want 1", summary.Delete)
	}
	if summary.Noop != 3 {
		t.Errorf("Summary.Noop = %v, want 3", summary.Noop)
	}
}

func TestPlanSummary_String(t *testing.T) {
	summary := PlanSummary{
		Zone:   "example.com",
		Create: 2,
		Update: 1,
		Delete: 1,
		Noop:   3,
	}

	str := summary.String()

	if !strings.Contains(str, "example.com") {
		t.Error("Summary.String() should contain zone name")
	}
	if !strings.Contains(str, "2 create") {
		t.Error("Summary.String() should contain create count")
	}
	if !strings.Contains(str, "1 update") {
		t.Error("Summary.String() should contain update count")
	}
	if !strings.Contains(str, "1 delete") {
		t.Error("Summary.String() should contain delete count")
	}
	if !strings.Contains(str, "3 unchanged") {
		t.Error("Summary.String() should contain noop count")
	}
}

func TestFormatPlan(t *testing.T) {
	plan := &Plan{
		Zone: "example.com",
		Changes: []RecordChange{
			{
				Type:   ChangeTypeCreate,
				Record: &Record{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300},
			},
			{
				Type:    ChangeTypeUpdate,
				Record:  &Record{Type: RecordTypeA, Name: "api", Value: "192.0.2.2", TTL: 600},
				Current: &Record{Type: RecordTypeA, Name: "api", Value: "192.0.2.2", TTL: 300},
				Diff:    map[string]FieldDiff{"ttl": {Old: 300, New: 600}},
			},
			{
				Type:    ChangeTypeDelete,
				Record:  &Record{Type: RecordTypeA, Name: "old", Value: "192.0.2.3", TTL: 300},
				Current: &Record{Type: RecordTypeA, Name: "old", Value: "192.0.2.3", TTL: 300},
			},
			{
				Type:    ChangeTypeNoop,
				Record:  &Record{Type: RecordTypeA, Name: "unchanged", Value: "192.0.2.4", TTL: 300},
				Current: &Record{Type: RecordTypeA, Name: "unchanged", Value: "192.0.2.4", TTL: 300},
			},
		},
	}

	output := FormatPlan(plan)

	// Should contain create marker
	if !strings.Contains(output, "+ A www") {
		t.Error("FormatPlan should contain create marker for www")
	}

	// Should contain update marker
	if !strings.Contains(output, "~ A api") {
		t.Error("FormatPlan should contain update marker for api")
	}

	// Should contain delete marker
	if !strings.Contains(output, "- A old") {
		t.Error("FormatPlan should contain delete marker for old")
	}

	// Should NOT contain unchanged records by default (check for record line, not summary)
	if strings.Contains(output, "A unchanged 192.0.2.4") {
		t.Error("FormatPlan should not contain noop records by default")
	}

	// Should contain TTL diff
	if !strings.Contains(output, "ttl:") {
		t.Error("FormatPlan should contain TTL diff details")
	}
}

func TestFormatPlanVerbose(t *testing.T) {
	plan := &Plan{
		Zone: "example.com",
		Changes: []RecordChange{
			{
				Type:   ChangeTypeCreate,
				Record: &Record{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300},
			},
			{
				Type:    ChangeTypeNoop,
				Record:  &Record{Type: RecordTypeA, Name: "unchanged", Value: "192.0.2.4", TTL: 300},
				Current: &Record{Type: RecordTypeA, Name: "unchanged", Value: "192.0.2.4", TTL: 300},
			},
		},
	}

	output := FormatPlanVerbose(plan)

	// Should contain unchanged records in verbose mode
	if !strings.Contains(output, "unchanged") {
		t.Error("FormatPlanVerbose should contain noop records")
	}
}

func TestDiffer_SortChanges(t *testing.T) {
	differ := NewDiffer("example.com")

	// Create changes in random order
	changes := []RecordChange{
		{Type: ChangeTypeNoop, Record: &Record{Type: RecordTypeA, Name: "z-record"}},
		{Type: ChangeTypeDelete, Record: &Record{Type: RecordTypeA, Name: "a-record"}},
		{Type: ChangeTypeCreate, Record: &Record{Type: RecordTypeCNAME, Name: "b-record"}},
		{Type: ChangeTypeCreate, Record: &Record{Type: RecordTypeA, Name: "c-record"}},
		{Type: ChangeTypeUpdate, Record: &Record{Type: RecordTypeA, Name: "d-record"}},
	}

	differ.sortChanges(changes)

	// Verify order: create, update, noop, delete
	expectedOrder := []ChangeType{
		ChangeTypeCreate, // A c-record (A before CNAME alphabetically)
		ChangeTypeCreate, // CNAME b-record
		ChangeTypeUpdate, // A d-record
		ChangeTypeNoop,   // A z-record
		ChangeTypeDelete, // A a-record
	}

	for i, change := range changes {
		if change.Type != expectedOrder[i] {
			t.Errorf("changes[%d].Type = %v, want %v", i, change.Type, expectedOrder[i])
		}
	}
}

func TestDiffer_PreservesRecordID(t *testing.T) {
	differ := NewDiffer("example.com")

	desired := []Record{
		{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 600}, // TTL change
	}
	current := []Record{
		{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, ID: "existing-id"},
	}

	plan := differ.Diff(desired, current)

	// Find the update change
	for _, change := range plan.Changes {
		if change.Type == ChangeTypeUpdate {
			if change.Record.ID != "existing-id" {
				t.Errorf("Update change should preserve ID, got %v", change.Record.ID)
			}
			return
		}
	}
	t.Error("Expected to find an update change")
}

func TestDiffer_CompareRecords_AllFields(t *testing.T) {
	trueVal := true
	falseVal := false

	differ := NewDiffer("example.com")

	tests := []struct {
		name        string
		desired     *Record
		current     *Record
		expectedDif []string
	}{
		{
			name:        "TTL difference",
			desired:     &Record{TTL: 600},
			current:     &Record{TTL: 300},
			expectedDif: []string{"ttl"},
		},
		{
			name:        "priority difference",
			desired:     &Record{Priority: 20},
			current:     &Record{Priority: 10},
			expectedDif: []string{"priority"},
		},
		{
			name:        "weight difference",
			desired:     &Record{Weight: 10},
			current:     &Record{Weight: 5},
			expectedDif: []string{"weight"},
		},
		{
			name:        "port difference",
			desired:     &Record{Port: 8080},
			current:     &Record{Port: 80},
			expectedDif: []string{"port"},
		},
		{
			name:        "proxied difference",
			desired:     &Record{Proxied: &trueVal},
			current:     &Record{Proxied: &falseVal},
			expectedDif: []string{"proxied"},
		},
		{
			name:        "multiple differences",
			desired:     &Record{TTL: 600, Priority: 20},
			current:     &Record{TTL: 300, Priority: 10},
			expectedDif: []string{"ttl", "priority"},
		},
		{
			name:        "no difference",
			desired:     &Record{TTL: 300, Priority: 10},
			current:     &Record{TTL: 300, Priority: 10},
			expectedDif: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff := differ.compareRecords(tt.desired, tt.current)

			if len(diff) != len(tt.expectedDif) {
				t.Errorf("compareRecords() returned %d diffs, want %d", len(diff), len(tt.expectedDif))
				return
			}

			for _, field := range tt.expectedDif {
				if _, ok := diff[field]; !ok {
					t.Errorf("compareRecords() missing expected diff for field %q", field)
				}
			}
		})
	}
}

func TestDiffer_Normalization(t *testing.T) {
	differ := NewDiffer("example.com")

	// Test that records with different formats but same meaning are matched
	desired := []Record{
		{Type: RecordTypeA, Name: "www.example.com", Value: "192.0.2.1", TTL: 300},
	}
	current := []Record{
		{Type: RecordTypeA, Name: "www", Value: "192.0.2.1", TTL: 300, ID: "rec1"},
	}

	plan := differ.Diff(desired, current)
	summary := plan.Summary()

	// Should be treated as the same record after normalization
	if summary.Noop != 1 {
		t.Errorf("Normalized records should match, got Create=%d, Update=%d, Noop=%d",
			summary.Create, summary.Update, summary.Noop)
	}
}
