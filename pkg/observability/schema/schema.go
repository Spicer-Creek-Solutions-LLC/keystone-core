// Package schema provides versioned schemas for observability data with migration support
package schema

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// SchemaType represents the type of observability schema
type SchemaType string

const (
	SchemaTypeLog     SchemaType = "log"
	SchemaTypeMetric  SchemaType = "metric"
	SchemaTypeTrace   SchemaType = "trace"
	SchemaTypeAudit   SchemaType = "audit"
	SchemaTypeEvent   SchemaType = "event"
	SchemaTypeAlert   SchemaType = "alert"
)

// CurrentVersions holds the current schema versions for each type
var CurrentVersions = map[SchemaType]int{
	SchemaTypeLog:    2,
	SchemaTypeMetric: 2,
	SchemaTypeTrace:  2,
	SchemaTypeAudit:  2,
	SchemaTypeEvent:  2,
	SchemaTypeAlert:  1,
}

// SchemaVersion represents a versioned schema
type SchemaVersion struct {
	// Type is the schema type
	Type SchemaType `json:"type"`

	// Version is the schema version number
	Version int `json:"version"`

	// Name is a human-readable name for the schema
	Name string `json:"name"`

	// Description describes the schema
	Description string `json:"description,omitempty"`

	// Fields describes the schema fields
	Fields []FieldDefinition `json:"fields"`

	// DeprecatedAt indicates when this version was deprecated
	DeprecatedAt *time.Time `json:"deprecated_at,omitempty"`

	// SunsetAt indicates when this version will be removed
	SunsetAt *time.Time `json:"sunset_at,omitempty"`

	// Changes lists changes from the previous version
	Changes []string `json:"changes,omitempty"`
}

// FieldDefinition describes a field in a schema
type FieldDefinition struct {
	// Name is the field name
	Name string `json:"name"`

	// Type is the field type (string, int, float, bool, object, array, timestamp)
	Type string `json:"type"`

	// Required indicates if the field is required
	Required bool `json:"required"`

	// Description describes the field
	Description string `json:"description,omitempty"`

	// Deprecated indicates if the field is deprecated
	Deprecated bool `json:"deprecated,omitempty"`

	// Default is the default value
	Default interface{} `json:"default,omitempty"`

	// NestedFields for object types
	NestedFields []FieldDefinition `json:"nested_fields,omitempty"`

	// AddedInVersion is when this field was added
	AddedInVersion int `json:"added_in_version,omitempty"`

	// RemovedInVersion is when this field was removed
	RemovedInVersion int `json:"removed_in_version,omitempty"`

	// RenamedFrom is the previous field name if renamed
	RenamedFrom string `json:"renamed_from,omitempty"`
}

// SchemaRegistry manages schema versions
type SchemaRegistry struct {
	schemas   map[SchemaType]map[int]*SchemaVersion
	migrators map[string]Migrator
	mu        sync.RWMutex
}

// Migrator migrates data between schema versions
type Migrator interface {
	// FromVersion returns the source version
	FromVersion() int

	// ToVersion returns the target version
	ToVersion() int

	// Migrate migrates a record from one version to another
	Migrate(data map[string]interface{}) (map[string]interface{}, error)

	// Describe returns a description of changes made
	Describe() []string
}

// NewSchemaRegistry creates a new schema registry
func NewSchemaRegistry() *SchemaRegistry {
	r := &SchemaRegistry{
		schemas:   make(map[SchemaType]map[int]*SchemaVersion),
		migrators: make(map[string]Migrator),
	}

	// Register built-in schemas
	r.registerBuiltinSchemas()
	r.registerBuiltinMigrators()

	return r
}

// RegisterSchema registers a schema version
func (r *SchemaRegistry) RegisterSchema(schema *SchemaVersion) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.schemas[schema.Type]; !ok {
		r.schemas[schema.Type] = make(map[int]*SchemaVersion)
	}

	r.schemas[schema.Type][schema.Version] = schema
	return nil
}

// GetSchema returns a specific schema version
func (r *SchemaRegistry) GetSchema(typ SchemaType, version int) (*SchemaVersion, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if versions, ok := r.schemas[typ]; ok {
		if schema, ok := versions[version]; ok {
			return schema, true
		}
	}
	return nil, false
}

// GetCurrentSchema returns the current schema version for a type
func (r *SchemaRegistry) GetCurrentSchema(typ SchemaType) (*SchemaVersion, bool) {
	currentVersion, ok := CurrentVersions[typ]
	if !ok {
		return nil, false
	}
	return r.GetSchema(typ, currentVersion)
}

// ListSchemaVersions returns all versions for a schema type
func (r *SchemaRegistry) ListSchemaVersions(typ SchemaType) []*SchemaVersion {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var schemas []*SchemaVersion
	if versions, ok := r.schemas[typ]; ok {
		for _, schema := range versions {
			schemas = append(schemas, schema)
		}
	}
	return schemas
}

// RegisterMigrator registers a schema migrator
func (r *SchemaRegistry) RegisterMigrator(typ SchemaType, migrator Migrator) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := migratorKey(typ, migrator.FromVersion(), migrator.ToVersion())
	r.migrators[key] = migrator
}

// GetMigrator returns a migrator for a version transition
func (r *SchemaRegistry) GetMigrator(typ SchemaType, fromVersion, toVersion int) (Migrator, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := migratorKey(typ, fromVersion, toVersion)
	migrator, ok := r.migrators[key]
	return migrator, ok
}

// Migrate migrates data from one schema version to another
func (r *SchemaRegistry) Migrate(typ SchemaType, data map[string]interface{}, fromVersion, toVersion int) (map[string]interface{}, error) {
	if fromVersion == toVersion {
		return data, nil
	}

	result := data
	step := 1
	if toVersion < fromVersion {
		step = -1
	}

	for v := fromVersion; v != toVersion; v += step {
		nextV := v + step
		migrator, ok := r.GetMigrator(typ, v, nextV)
		if !ok {
			return nil, fmt.Errorf("no migrator for %s schema v%d to v%d", typ, v, nextV)
		}

		var err error
		result, err = migrator.Migrate(result)
		if err != nil {
			return nil, fmt.Errorf("migration from v%d to v%d failed: %w", v, nextV, err)
		}
	}

	return result, nil
}

// MigrateToLatest migrates data to the latest schema version
func (r *SchemaRegistry) MigrateToLatest(typ SchemaType, data map[string]interface{}, fromVersion int) (map[string]interface{}, error) {
	latestVersion, ok := CurrentVersions[typ]
	if !ok {
		return nil, fmt.Errorf("unknown schema type: %s", typ)
	}
	return r.Migrate(typ, data, fromVersion, latestVersion)
}

// ValidateData validates data against a schema
func (r *SchemaRegistry) ValidateData(typ SchemaType, version int, data map[string]interface{}) []ValidationError {
	schema, ok := r.GetSchema(typ, version)
	if !ok {
		return []ValidationError{{
			Field:   "",
			Message: fmt.Sprintf("unknown schema version: %s v%d", typ, version),
		}}
	}

	return validateFields(schema.Fields, data, "")
}

func validateFields(fields []FieldDefinition, data map[string]interface{}, prefix string) []ValidationError {
	var errors []ValidationError

	for _, field := range fields {
		fieldPath := field.Name
		if prefix != "" {
			fieldPath = prefix + "." + field.Name
		}

		value, exists := data[field.Name]

		// Check required fields
		if field.Required && !exists {
			errors = append(errors, ValidationError{
				Field:   fieldPath,
				Message: "required field is missing",
			})
			continue
		}

		if !exists {
			continue
		}

		// Type validation
		if err := validateType(field, value); err != nil {
			errors = append(errors, ValidationError{
				Field:   fieldPath,
				Message: err.Error(),
			})
		}

		// Nested validation for objects
		if field.Type == "object" && len(field.NestedFields) > 0 {
			if nested, ok := value.(map[string]interface{}); ok {
				errors = append(errors, validateFields(field.NestedFields, nested, fieldPath)...)
			}
		}
	}

	return errors
}

func validateType(field FieldDefinition, value interface{}) error {
	if value == nil {
		if field.Required {
			return fmt.Errorf("value is nil")
		}
		return nil
	}

	switch field.Type {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("expected string, got %T", value)
		}
	case "int":
		switch value.(type) {
		case int, int32, int64, float64:
			// Accept float64 as it's the default JSON number type
		default:
			return fmt.Errorf("expected int, got %T", value)
		}
	case "float":
		switch value.(type) {
		case float32, float64:
		default:
			return fmt.Errorf("expected float, got %T", value)
		}
	case "bool":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("expected bool, got %T", value)
		}
	case "timestamp":
		switch value.(type) {
		case time.Time, string:
		default:
			return fmt.Errorf("expected timestamp, got %T", value)
		}
	case "object":
		if _, ok := value.(map[string]interface{}); !ok {
			return fmt.Errorf("expected object, got %T", value)
		}
	case "array":
		if _, ok := value.([]interface{}); !ok {
			return fmt.Errorf("expected array, got %T", value)
		}
	}

	return nil
}

// ValidationError represents a schema validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return e.Message
}

func migratorKey(typ SchemaType, from, to int) string {
	return fmt.Sprintf("%s:%d->%d", typ, from, to)
}

// VersionedRecord is a record with schema version information
type VersionedRecord struct {
	SchemaType    SchemaType             `json:"_schema_type"`
	SchemaVersion int                    `json:"_schema_version"`
	Data          map[string]interface{} `json:"data"`
}

// NewVersionedRecord creates a new versioned record
func NewVersionedRecord(typ SchemaType, data map[string]interface{}) *VersionedRecord {
	return &VersionedRecord{
		SchemaType:    typ,
		SchemaVersion: CurrentVersions[typ],
		Data:          data,
	}
}

// MarshalJSON implements json.Marshaler
func (r *VersionedRecord) MarshalJSON() ([]byte, error) {
	// Flatten the record with schema info at top level
	output := make(map[string]interface{})
	output["_schema_type"] = r.SchemaType
	output["_schema_version"] = r.SchemaVersion
	for k, v := range r.Data {
		output[k] = v
	}
	return json.Marshal(output)
}

// UnmarshalJSON implements json.Unmarshaler
func (r *VersionedRecord) UnmarshalJSON(data []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if typ, ok := raw["_schema_type"].(string); ok {
		r.SchemaType = SchemaType(typ)
		delete(raw, "_schema_type")
	}

	if ver, ok := raw["_schema_version"].(float64); ok {
		r.SchemaVersion = int(ver)
		delete(raw, "_schema_version")
	}

	r.Data = raw
	return nil
}

// registerBuiltinSchemas registers the built-in schema versions
func (r *SchemaRegistry) registerBuiltinSchemas() {
	// Log Schema V1
	r.RegisterSchema(&SchemaVersion{
		Type:        SchemaTypeLog,
		Version:     1,
		Name:        "Log Entry V1",
		Description: "Original log entry schema",
		Fields: []FieldDefinition{
			{Name: "timestamp", Type: "timestamp", Required: true, Description: "Log timestamp"},
			{Name: "level", Type: "string", Required: true, Description: "Log level"},
			{Name: "message", Type: "string", Required: true, Description: "Log message"},
			{Name: "logger", Type: "string", Required: false, Description: "Logger name"},
			{Name: "fields", Type: "object", Required: false, Description: "Additional fields"},
		},
	})

	// Log Schema V2
	r.RegisterSchema(&SchemaVersion{
		Type:        SchemaTypeLog,
		Version:     2,
		Name:        "Log Entry V2",
		Description: "Enhanced log entry with metadata and correlation",
		Fields: []FieldDefinition{
			{Name: "timestamp", Type: "timestamp", Required: true, Description: "Log timestamp"},
			{Name: "level", Type: "string", Required: true, Description: "Log level"},
			{Name: "message", Type: "string", Required: true, Description: "Log message"},
			{Name: "logger", Type: "string", Required: false, Description: "Logger name"},
			{Name: "correlation_id", Type: "string", Required: false, Description: "Correlation ID for request tracing", AddedInVersion: 2},
			{Name: "fields", Type: "object", Required: false, Description: "Additional fields"},
			{Name: "metadata", Type: "object", Required: false, Description: "System metadata", AddedInVersion: 2, NestedFields: []FieldDefinition{
				{Name: "host", Type: "string", Required: false},
				{Name: "pid", Type: "int", Required: false},
				{Name: "version", Type: "string", Required: false},
				{Name: "service", Type: "string", Required: false},
				{Name: "caller", Type: "string", Required: false},
			}},
		},
		Changes: []string{
			"Added correlation_id field for request tracing",
			"Added metadata block for system information",
		},
	})

	// Metric Schema V1
	r.RegisterSchema(&SchemaVersion{
		Type:        SchemaTypeMetric,
		Version:     1,
		Name:        "Metric V1",
		Description: "Original metric schema",
		Fields: []FieldDefinition{
			{Name: "name", Type: "string", Required: true, Description: "Metric name"},
			{Name: "type", Type: "string", Required: true, Description: "Metric type (counter, gauge, histogram, summary)"},
			{Name: "value", Type: "float", Required: true, Description: "Metric value"},
			{Name: "timestamp", Type: "timestamp", Required: true, Description: "Metric timestamp"},
			{Name: "labels", Type: "object", Required: false, Description: "Metric labels"},
		},
	})

	// Metric Schema V2
	r.RegisterSchema(&SchemaVersion{
		Type:        SchemaTypeMetric,
		Version:     2,
		Name:        "Metric V2",
		Description: "Enhanced metric schema with histogram support",
		Fields: []FieldDefinition{
			{Name: "name", Type: "string", Required: true, Description: "Metric name"},
			{Name: "type", Type: "string", Required: true, Description: "Metric type"},
			{Name: "value", Type: "float", Required: true, Description: "Metric value"},
			{Name: "timestamp", Type: "timestamp", Required: true, Description: "Metric timestamp"},
			{Name: "labels", Type: "object", Required: false, Description: "Metric labels"},
			{Name: "help", Type: "string", Required: false, Description: "Metric help text", AddedInVersion: 2},
			{Name: "unit", Type: "string", Required: false, Description: "Metric unit", AddedInVersion: 2},
			{Name: "histogram", Type: "object", Required: false, Description: "Histogram data", AddedInVersion: 2, NestedFields: []FieldDefinition{
				{Name: "buckets", Type: "array", Required: false},
				{Name: "count", Type: "int", Required: false},
				{Name: "sum", Type: "float", Required: false},
			}},
		},
		Changes: []string{
			"Added help text field",
			"Added unit field for metric units",
			"Added histogram object for bucket data",
		},
	})

	// Trace Schema V1
	r.RegisterSchema(&SchemaVersion{
		Type:        SchemaTypeTrace,
		Version:     1,
		Name:        "Trace Span V1",
		Description: "Original trace span schema",
		Fields: []FieldDefinition{
			{Name: "trace_id", Type: "string", Required: true, Description: "Trace ID"},
			{Name: "span_id", Type: "string", Required: true, Description: "Span ID"},
			{Name: "parent_span_id", Type: "string", Required: false, Description: "Parent span ID"},
			{Name: "operation_name", Type: "string", Required: true, Description: "Operation name"},
			{Name: "start_time", Type: "timestamp", Required: true, Description: "Start timestamp"},
			{Name: "duration", Type: "int", Required: true, Description: "Duration in microseconds"},
			{Name: "tags", Type: "object", Required: false, Description: "Span tags"},
		},
	})

	// Trace Schema V2
	r.RegisterSchema(&SchemaVersion{
		Type:        SchemaTypeTrace,
		Version:     2,
		Name:        "Trace Span V2",
		Description: "Enhanced trace span with status and events",
		Fields: []FieldDefinition{
			{Name: "trace_id", Type: "string", Required: true, Description: "Trace ID"},
			{Name: "span_id", Type: "string", Required: true, Description: "Span ID"},
			{Name: "parent_span_id", Type: "string", Required: false, Description: "Parent span ID"},
			{Name: "operation_name", Type: "string", Required: true, Description: "Operation name"},
			{Name: "start_time", Type: "timestamp", Required: true, Description: "Start timestamp"},
			{Name: "end_time", Type: "timestamp", Required: false, Description: "End timestamp", AddedInVersion: 2},
			{Name: "duration", Type: "int", Required: true, Description: "Duration in microseconds"},
			{Name: "status", Type: "object", Required: false, Description: "Span status", AddedInVersion: 2, NestedFields: []FieldDefinition{
				{Name: "code", Type: "string", Required: true},
				{Name: "message", Type: "string", Required: false},
			}},
			{Name: "attributes", Type: "object", Required: false, Description: "Span attributes (renamed from tags)", RenamedFrom: "tags"},
			{Name: "events", Type: "array", Required: false, Description: "Span events", AddedInVersion: 2},
			{Name: "links", Type: "array", Required: false, Description: "Span links", AddedInVersion: 2},
		},
		Changes: []string{
			"Added end_time field",
			"Added status object with code and message",
			"Renamed tags to attributes (OpenTelemetry alignment)",
			"Added events array for span events",
			"Added links array for span links",
		},
	})

	// Audit Schema V1
	r.RegisterSchema(&SchemaVersion{
		Type:        SchemaTypeAudit,
		Version:     1,
		Name:        "Audit Entry V1",
		Description: "Original audit entry schema",
		Fields: []FieldDefinition{
			{Name: "timestamp", Type: "timestamp", Required: true, Description: "Event timestamp"},
			{Name: "action", Type: "string", Required: true, Description: "Action performed"},
			{Name: "actor", Type: "string", Required: true, Description: "Who performed the action"},
			{Name: "resource", Type: "string", Required: true, Description: "Resource affected"},
			{Name: "outcome", Type: "string", Required: true, Description: "Action outcome (success, failure)"},
		},
	})

	// Audit Schema V2
	r.RegisterSchema(&SchemaVersion{
		Type:        SchemaTypeAudit,
		Version:     2,
		Name:        "Audit Entry V2",
		Description: "Enhanced audit entry with detailed context",
		Fields: []FieldDefinition{
			{Name: "timestamp", Type: "timestamp", Required: true, Description: "Event timestamp"},
			{Name: "event_id", Type: "string", Required: true, Description: "Unique event ID", AddedInVersion: 2},
			{Name: "action", Type: "string", Required: true, Description: "Action performed"},
			{Name: "actor", Type: "object", Required: true, Description: "Actor information", NestedFields: []FieldDefinition{
				{Name: "id", Type: "string", Required: true},
				{Name: "type", Type: "string", Required: true},
				{Name: "name", Type: "string", Required: false},
				{Name: "ip_address", Type: "string", Required: false, AddedInVersion: 2},
			}},
			{Name: "resource", Type: "object", Required: true, Description: "Resource information", NestedFields: []FieldDefinition{
				{Name: "id", Type: "string", Required: true},
				{Name: "type", Type: "string", Required: true},
				{Name: "name", Type: "string", Required: false},
			}},
			{Name: "outcome", Type: "string", Required: true, Description: "Action outcome"},
			{Name: "reason", Type: "string", Required: false, Description: "Failure reason", AddedInVersion: 2},
			{Name: "context", Type: "object", Required: false, Description: "Additional context", AddedInVersion: 2},
			{Name: "correlation_id", Type: "string", Required: false, Description: "Correlation ID", AddedInVersion: 2},
		},
		Changes: []string{
			"Added event_id for unique identification",
			"Expanded actor to include type and IP address",
			"Expanded resource to structured object",
			"Added reason field for failure details",
			"Added context for additional information",
			"Added correlation_id for request tracing",
		},
	})

	// Event Schema V1
	r.RegisterSchema(&SchemaVersion{
		Type:        SchemaTypeEvent,
		Version:     1,
		Name:        "Event V1",
		Description: "Original event schema",
		Fields: []FieldDefinition{
			{Name: "id", Type: "string", Required: true, Description: "Event ID"},
			{Name: "type", Type: "string", Required: true, Description: "Event type"},
			{Name: "source", Type: "string", Required: true, Description: "Event source"},
			{Name: "timestamp", Type: "timestamp", Required: true, Description: "Event timestamp"},
			{Name: "data", Type: "object", Required: false, Description: "Event data"},
		},
	})

	// Event Schema V2 (CloudEvents aligned)
	r.RegisterSchema(&SchemaVersion{
		Type:        SchemaTypeEvent,
		Version:     2,
		Name:        "Event V2 (CloudEvents)",
		Description: "CloudEvents-aligned event schema",
		Fields: []FieldDefinition{
			{Name: "specversion", Type: "string", Required: true, Description: "CloudEvents spec version", AddedInVersion: 2},
			{Name: "id", Type: "string", Required: true, Description: "Event ID"},
			{Name: "type", Type: "string", Required: true, Description: "Event type"},
			{Name: "source", Type: "string", Required: true, Description: "Event source URI"},
			{Name: "time", Type: "timestamp", Required: true, Description: "Event timestamp", RenamedFrom: "timestamp"},
			{Name: "subject", Type: "string", Required: false, Description: "Event subject", AddedInVersion: 2},
			{Name: "datacontenttype", Type: "string", Required: false, Description: "Data content type", AddedInVersion: 2},
			{Name: "dataschema", Type: "string", Required: false, Description: "Data schema URI", AddedInVersion: 2},
			{Name: "data", Type: "object", Required: false, Description: "Event data"},
		},
		Changes: []string{
			"Aligned with CloudEvents specification",
			"Added specversion field",
			"Renamed timestamp to time",
			"Added subject, datacontenttype, dataschema fields",
		},
	})

	// Alert Schema V1
	r.RegisterSchema(&SchemaVersion{
		Type:        SchemaTypeAlert,
		Version:     1,
		Name:        "Alert V1",
		Description: "Alert notification schema",
		Fields: []FieldDefinition{
			{Name: "id", Type: "string", Required: true, Description: "Alert ID"},
			{Name: "name", Type: "string", Required: true, Description: "Alert name"},
			{Name: "severity", Type: "string", Required: true, Description: "Alert severity"},
			{Name: "status", Type: "string", Required: true, Description: "Alert status (firing, resolved)"},
			{Name: "starts_at", Type: "timestamp", Required: true, Description: "Alert start time"},
			{Name: "ends_at", Type: "timestamp", Required: false, Description: "Alert end time"},
			{Name: "summary", Type: "string", Required: false, Description: "Alert summary"},
			{Name: "description", Type: "string", Required: false, Description: "Alert description"},
			{Name: "labels", Type: "object", Required: false, Description: "Alert labels"},
			{Name: "annotations", Type: "object", Required: false, Description: "Alert annotations"},
		},
	})
}

// registerBuiltinMigrators registers the built-in schema migrators
func (r *SchemaRegistry) registerBuiltinMigrators() {
	// Log V1 -> V2
	r.RegisterMigrator(SchemaTypeLog, &logMigratorV1ToV2{})

	// Metric V1 -> V2
	r.RegisterMigrator(SchemaTypeMetric, &metricMigratorV1ToV2{})

	// Trace V1 -> V2
	r.RegisterMigrator(SchemaTypeTrace, &traceMigratorV1ToV2{})

	// Audit V1 -> V2
	r.RegisterMigrator(SchemaTypeAudit, &auditMigratorV1ToV2{})

	// Event V1 -> V2
	r.RegisterMigrator(SchemaTypeEvent, &eventMigratorV1ToV2{})
}
