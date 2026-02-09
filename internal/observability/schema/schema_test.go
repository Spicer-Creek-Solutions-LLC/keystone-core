package schema

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewSchemaRegistry(t *testing.T) {
	r := NewSchemaRegistry()

	// Check that built-in schemas are registered
	for typ := range CurrentVersions {
		schema, ok := r.GetCurrentSchema(typ)
		if !ok {
			t.Errorf("Missing current schema for type %s", typ)
		}
		if schema == nil {
			t.Errorf("Nil schema for type %s", typ)
		}
	}
}

func TestSchemaRegistry_GetSchema(t *testing.T) {
	r := NewSchemaRegistry()

	// Test getting existing schema
	schema, ok := r.GetSchema(SchemaTypeLog, 1)
	if !ok {
		t.Error("Expected to find log schema v1")
	}
	if schema.Version != 1 {
		t.Errorf("Expected version 1, got %d", schema.Version)
	}

	// Test getting non-existent schema
	_, ok = r.GetSchema(SchemaTypeLog, 999)
	if ok {
		t.Error("Should not find non-existent schema version")
	}
}

func TestSchemaRegistry_GetCurrentSchema(t *testing.T) {
	r := NewSchemaRegistry()

	for typ, expectedVersion := range CurrentVersions {
		schema, ok := r.GetCurrentSchema(typ)
		if !ok {
			t.Errorf("Missing current schema for %s", typ)
			continue
		}
		if schema.Version != expectedVersion {
			t.Errorf("For %s, expected version %d, got %d", typ, expectedVersion, schema.Version)
		}
	}
}

func TestSchemaRegistry_ListSchemaVersions(t *testing.T) {
	r := NewSchemaRegistry()

	versions := r.ListSchemaVersions(SchemaTypeLog)
	if len(versions) < 2 {
		t.Errorf("Expected at least 2 log schema versions, got %d", len(versions))
	}
}

func TestSchemaRegistry_Migrate_Log(t *testing.T) {
	r := NewSchemaRegistry()

	// V1 data
	v1Data := map[string]interface{}{
		"timestamp": "2024-01-01T00:00:00Z",
		"level":     "info",
		"message":   "test message",
		"logger":    "test",
	}

	// Migrate to V2
	v2Data, err := r.Migrate(SchemaTypeLog, v1Data, 1, 2)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Check new fields exist
	if _, ok := v2Data["correlation_id"]; !ok {
		t.Error("Missing correlation_id field")
	}
	if _, ok := v2Data["metadata"]; !ok {
		t.Error("Missing metadata field")
	}

	// Original fields should be preserved
	if v2Data["message"] != "test message" {
		t.Error("Original message field not preserved")
	}
}

func TestSchemaRegistry_Migrate_Metric(t *testing.T) {
	r := NewSchemaRegistry()

	// V1 histogram data
	v1Data := map[string]interface{}{
		"name":      "request_duration",
		"type":      "histogram",
		"value":     0.5,
		"timestamp": "2024-01-01T00:00:00Z",
		"labels":    map[string]interface{}{"method": "GET"},
	}

	// Migrate to V2
	v2Data, err := r.Migrate(SchemaTypeMetric, v1Data, 1, 2)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Check new fields
	if _, ok := v2Data["help"]; !ok {
		t.Error("Missing help field")
	}
	if _, ok := v2Data["unit"]; !ok {
		t.Error("Missing unit field")
	}
	if _, ok := v2Data["histogram"]; !ok {
		t.Error("Missing histogram field for histogram type")
	}
}

func TestSchemaRegistry_Migrate_Trace(t *testing.T) {
	r := NewSchemaRegistry()

	// V1 data with tags
	v1Data := map[string]interface{}{
		"trace_id":       "abc123",
		"span_id":        "def456",
		"operation_name": "test_op",
		"start_time":     time.Now().Format(time.RFC3339),
		"duration":       1000, // 1000 microseconds
		"tags":           map[string]interface{}{"http.method": "GET"},
	}

	// Migrate to V2
	v2Data, err := r.Migrate(SchemaTypeTrace, v1Data, 1, 2)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Check renamed field
	if _, ok := v2Data["attributes"]; !ok {
		t.Error("tags should be renamed to attributes")
	}
	if _, ok := v2Data["tags"]; ok {
		t.Error("tags field should be removed")
	}

	// Check new fields
	if _, ok := v2Data["status"]; !ok {
		t.Error("Missing status field")
	}
	if _, ok := v2Data["events"]; !ok {
		t.Error("Missing events field")
	}
	if _, ok := v2Data["links"]; !ok {
		t.Error("Missing links field")
	}
}

func TestSchemaRegistry_Migrate_Audit(t *testing.T) {
	r := NewSchemaRegistry()

	// V1 data with string actor/resource
	v1Data := map[string]interface{}{
		"timestamp": "2024-01-01T00:00:00Z",
		"action":    "create",
		"actor":     "user123",
		"resource":  "resource456",
		"outcome":   "success",
	}

	// Migrate to V2
	v2Data, err := r.Migrate(SchemaTypeAudit, v1Data, 1, 2)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Check event_id was added
	if _, ok := v2Data["event_id"]; !ok {
		t.Error("Missing event_id field")
	}

	// Check actor converted to object
	actor, ok := v2Data["actor"].(map[string]interface{})
	if !ok {
		t.Error("actor should be an object")
	}
	if actor["id"] != "user123" {
		t.Error("actor.id should be original actor value")
	}

	// Check resource converted to object
	resource, ok := v2Data["resource"].(map[string]interface{})
	if !ok {
		t.Error("resource should be an object")
	}
	if resource["id"] != "resource456" {
		t.Error("resource.id should be original resource value")
	}
}

func TestSchemaRegistry_Migrate_Event(t *testing.T) {
	r := NewSchemaRegistry()

	// V1 data
	v1Data := map[string]interface{}{
		"id":        "evt123",
		"type":      "user.created",
		"source":    "/users",
		"timestamp": "2024-01-01T00:00:00Z",
		"data":      map[string]interface{}{"user_id": "123"},
	}

	// Migrate to V2
	v2Data, err := r.Migrate(SchemaTypeEvent, v1Data, 1, 2)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Check specversion added
	if v2Data["specversion"] != "1.0" {
		t.Errorf("Expected specversion 1.0, got %v", v2Data["specversion"])
	}

	// Check timestamp renamed to time
	if _, ok := v2Data["time"]; !ok {
		t.Error("timestamp should be renamed to time")
	}
	if _, ok := v2Data["timestamp"]; ok {
		t.Error("timestamp field should be removed")
	}
}

func TestSchemaRegistry_MigrateToLatest(t *testing.T) {
	r := NewSchemaRegistry()

	// Start with V1 log
	v1Data := map[string]interface{}{
		"timestamp": "2024-01-01T00:00:00Z",
		"level":     "info",
		"message":   "test",
	}

	latestData, err := r.MigrateToLatest(SchemaTypeLog, v1Data, 1)
	if err != nil {
		t.Fatalf("MigrateToLatest failed: %v", err)
	}

	// Should have all V2 fields
	if _, ok := latestData["correlation_id"]; !ok {
		t.Error("Latest data should have correlation_id")
	}
}

func TestSchemaRegistry_ValidateData(t *testing.T) {
	r := NewSchemaRegistry()

	t.Run("valid data", func(t *testing.T) {
		data := map[string]interface{}{
			"timestamp": "2024-01-01T00:00:00Z",
			"level":     "info",
			"message":   "test",
		}

		errors := r.ValidateData(SchemaTypeLog, 1, data)
		if len(errors) > 0 {
			t.Errorf("Expected no errors, got %v", errors)
		}
	})

	t.Run("missing required field", func(t *testing.T) {
		data := map[string]interface{}{
			"timestamp": "2024-01-01T00:00:00Z",
			// Missing level and message
		}

		errors := r.ValidateData(SchemaTypeLog, 1, data)
		if len(errors) == 0 {
			t.Error("Expected validation errors for missing required fields")
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		data := map[string]interface{}{
			"timestamp": "2024-01-01T00:00:00Z",
			"level":     123, // Should be string
			"message":   "test",
		}

		errors := r.ValidateData(SchemaTypeLog, 1, data)
		if len(errors) == 0 {
			t.Error("Expected validation error for wrong type")
		}
	})
}

func TestVersionedRecord_MarshalJSON(t *testing.T) {
	record := NewVersionedRecord(SchemaTypeLog, map[string]interface{}{
		"timestamp": "2024-01-01T00:00:00Z",
		"level":     "info",
		"message":   "test",
	})

	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Verify structure
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if raw["_schema_type"] != string(SchemaTypeLog) {
		t.Errorf("Expected _schema_type=%s, got %v", SchemaTypeLog, raw["_schema_type"])
	}
	if raw["_schema_version"].(float64) != float64(CurrentVersions[SchemaTypeLog]) {
		t.Errorf("Expected _schema_version=%d, got %v", CurrentVersions[SchemaTypeLog], raw["_schema_version"])
	}
	if raw["message"] != "test" {
		t.Error("Data fields should be flattened into record")
	}
}

func TestVersionedRecord_UnmarshalJSON(t *testing.T) {
	jsonData := `{
		"_schema_type": "log",
		"_schema_version": 2,
		"timestamp": "2024-01-01T00:00:00Z",
		"level": "info",
		"message": "test"
	}`

	var record VersionedRecord
	if err := json.Unmarshal([]byte(jsonData), &record); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if record.Type != SchemaTypeLog {
		t.Errorf("Expected Type=%s, got %s", SchemaTypeLog, record.Type)
	}
	if record.Version != 2 {
		t.Errorf("Expected Version=2, got %d", record.Version)
	}
	if record.Data["message"] != "test" {
		t.Error("Data should contain message field")
	}
}

func TestMigratorKey(t *testing.T) {
	key := migratorKey(SchemaTypeLog, 1, 2)
	expected := "log:1->2"
	if key != expected {
		t.Errorf("Expected key %s, got %s", expected, key)
	}
}

func TestSchemaVersion_Fields(t *testing.T) {
	r := NewSchemaRegistry()

	schema, ok := r.GetSchema(SchemaTypeLog, 2)
	if !ok {
		t.Fatal("Log schema V2 not found")
	}

	// Check that correlation_id was added in V2
	found := false
	for _, field := range schema.Fields {
		if field.Name == "correlation_id" {
			found = true
			if field.AddedInVersion != 2 {
				t.Errorf("correlation_id should have AddedInVersion=2, got %d", field.AddedInVersion)
			}
		}
	}
	if !found {
		t.Error("correlation_id field not found in V2 schema")
	}
}

func TestSchemaVersion_Changes(t *testing.T) {
	r := NewSchemaRegistry()

	schema, ok := r.GetSchema(SchemaTypeLog, 2)
	if !ok {
		t.Fatal("Log schema V2 not found")
	}

	if len(schema.Changes) == 0 {
		t.Error("V2 schema should have documented changes")
	}
}

func TestMigrator_Describe(t *testing.T) {
	r := NewSchemaRegistry()

	migrator, ok := r.GetMigrator(SchemaTypeLog, 1, 2)
	if !ok {
		t.Fatal("Log migrator 1->2 not found")
	}

	changes := migrator.Describe()
	if len(changes) == 0 {
		t.Error("Migrator should describe its changes")
	}
}

func TestSchemaRegistry_NoMigrationNeeded(t *testing.T) {
	r := NewSchemaRegistry()

	data := map[string]interface{}{
		"message": "test",
	}

	// Same version should return data unchanged
	result, err := r.Migrate(SchemaTypeLog, data, 2, 2)
	if err != nil {
		t.Fatalf("Same version migration should not error: %v", err)
	}
	if result["message"] != "test" {
		t.Error("Data should be unchanged")
	}
}

func TestSchemaRegistry_MissingMigrator(t *testing.T) {
	r := NewSchemaRegistry()

	data := map[string]interface{}{}

	// Try to migrate to a non-existent version
	_, err := r.Migrate(SchemaTypeLog, data, 1, 999)
	if err == nil {
		t.Error("Should error when no migrator exists")
	}
}

func TestValidateType(t *testing.T) {
	tests := []struct {
		name      string
		field     FieldDefinition
		value     interface{}
		expectErr bool
	}{
		{
			name:      "valid string",
			field:     FieldDefinition{Type: "string", Required: true},
			value:     "test",
			expectErr: false,
		},
		{
			name:      "invalid string",
			field:     FieldDefinition{Type: "string", Required: true},
			value:     123,
			expectErr: true,
		},
		{
			name:      "valid int",
			field:     FieldDefinition{Type: "int", Required: true},
			value:     123,
			expectErr: false,
		},
		{
			name:      "valid int from float64",
			field:     FieldDefinition{Type: "int", Required: true},
			value:     float64(123),
			expectErr: false,
		},
		{
			name:      "valid bool",
			field:     FieldDefinition{Type: "bool", Required: true},
			value:     true,
			expectErr: false,
		},
		{
			name:      "nil on required",
			field:     FieldDefinition{Type: "string", Required: true},
			value:     nil,
			expectErr: true,
		},
		{
			name:      "nil on optional",
			field:     FieldDefinition{Type: "string", Required: false},
			value:     nil,
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateType(tt.field, tt.value)
			if tt.expectErr && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}
