package events

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ValidationError represents a single validation error
type ValidationError struct {
	// Field is the name of the field that failed validation
	Field string `json:"field"`

	// Code is a machine-readable error code
	Code string `json:"code"`

	// Message is a human-readable error message
	Message string `json:"message"`

	// Value is the actual value that failed validation (for debugging)
	Value interface{} `json:"value,omitempty"`
}

// Error implements the error interface
func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationResult contains the result of validating an event
type EventValidationResult struct {
	// Valid indicates whether the event passed validation
	Valid bool `json:"valid"`

	// Errors is the list of validation errors (if any)
	Errors []*ValidationError `json:"errors,omitempty"`
}

// Error returns a combined error message from all validation errors
func (r *EventValidationResult) Error() string {
	if len(r.Errors) == 0 {
		return ""
	}

	var msgs []string
	for _, err := range r.Errors {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

// Summary returns a brief summary of validation errors
func (r *EventValidationResult) Summary() string {
	if r.Valid {
		return "validation passed"
	}
	return fmt.Sprintf("%d validation error(s): %s", len(r.Errors), r.Error())
}

// EventSchemaValidator validates events against the schema
type EventSchemaValidator struct {
	// Configuration
	config *SchemaValidatorConfig

	// Compiled patterns
	idPattern     *regexp.Regexp
	sourcePattern *regexp.Regexp
	tagKeyPattern *regexp.Regexp
}

// SchemaValidatorConfig configures the event schema validator
type SchemaValidatorConfig struct {
	// AllowUnknownEventTypes allows event types not in the predefined list
	AllowUnknownEventTypes bool

	// AllowEmptyID allows events without an ID (will be generated)
	AllowEmptyID bool

	// AllowFutureTimestamps allows timestamps in the future
	AllowFutureTimestamps bool

	// MaxTimestampSkew is the maximum allowed timestamp skew from now
	MaxTimestampSkew time.Duration

	// MaxTagCount is the maximum number of tags per event
	MaxTagCount int

	// MaxTagKeyLength is the maximum length of a tag key
	MaxTagKeyLength int

	// MaxTagValueLength is the maximum length of a tag value
	MaxTagValueLength int

	// MaxDataFields is the maximum number of data fields per event
	MaxDataFields int

	// MaxDataValueSize is the maximum size of a data field value (in bytes, for strings)
	MaxDataValueSize int

	// MaxSourceLength is the maximum length of the source field
	MaxSourceLength int

	// RequireCorrelationID requires a correlation ID to be set
	RequireCorrelationID bool

	// AllowedEventTypes restricts events to these types (empty = all allowed)
	AllowedEventTypes []EventType

	// BlockedEventTypes blocks these event types
	BlockedEventTypes []EventType

	// RequiredTags requires these tags to be present
	RequiredTags []string

	// StrictMode enables all strict validations
	StrictMode bool
}

// DefaultSchemaValidatorConfig returns default configuration
func DefaultSchemaValidatorConfig() *SchemaValidatorConfig {
	return &SchemaValidatorConfig{
		AllowUnknownEventTypes: true,
		AllowEmptyID:           true,  // IDs are typically auto-generated
		AllowFutureTimestamps:  false,
		MaxTimestampSkew:       5 * time.Minute,
		MaxTagCount:            50,
		MaxTagKeyLength:        64,
		MaxTagValueLength:      256,
		MaxDataFields:          100,
		MaxDataValueSize:       65536, // 64KB
		MaxSourceLength:        512,
		RequireCorrelationID:   false,
		StrictMode:             false,
	}
}

// StrictSchemaValidatorConfig returns strict configuration for production use
func StrictSchemaValidatorConfig() *SchemaValidatorConfig {
	return &SchemaValidatorConfig{
		AllowUnknownEventTypes: false,
		AllowEmptyID:           false,
		AllowFutureTimestamps:  false,
		MaxTimestampSkew:       1 * time.Minute,
		MaxTagCount:            20,
		MaxTagKeyLength:        32,
		MaxTagValueLength:      128,
		MaxDataFields:          50,
		MaxDataValueSize:       8192, // 8KB
		MaxSourceLength:        256,
		RequireCorrelationID:   true,
		StrictMode:             true,
	}
}

// NewEventSchemaValidator creates a new event schema validator
func NewEventSchemaValidator(config *SchemaValidatorConfig) *EventSchemaValidator {
	if config == nil {
		config = DefaultSchemaValidatorConfig()
	}

	return &EventSchemaValidator{
		config:        config,
		idPattern:     regexp.MustCompile(`^[a-zA-Z0-9_-]+$`),
		sourcePattern: regexp.MustCompile(`^[a-zA-Z0-9/_.-]+$`),
		tagKeyPattern: regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_.-]*$`),
	}
}

// Validate validates an event against the schema
func (v *EventSchemaValidator) Validate(event *Event) *EventValidationResult {
	result := &EventValidationResult{
		Valid:  true,
		Errors: make([]*ValidationError, 0),
	}

	if event == nil {
		result.Valid = false
		result.Errors = append(result.Errors, &ValidationError{
			Field:   "event",
			Code:    "event_nil",
			Message: "event cannot be nil",
		})
		return result
	}

	// Validate ID
	v.validateID(event, result)

	// Validate Type
	v.validateType(event, result)

	// Validate Source
	v.validateSource(event, result)

	// Validate Time
	v.validateTime(event, result)

	// Validate Severity
	v.validateSeverity(event, result)

	// Validate CorrelationID
	v.validateCorrelationID(event, result)

	// Validate Tags
	v.validateTags(event, result)

	// Validate Data
	v.validateData(event, result)

	result.Valid = len(result.Errors) == 0
	return result
}

// validateID validates the event ID
func (v *EventSchemaValidator) validateID(event *Event, result *EventValidationResult) {
	if event.ID == "" {
		if !v.config.AllowEmptyID {
			result.Errors = append(result.Errors, &ValidationError{
				Field:   "id",
				Code:    "id_required",
				Message: "event ID is required",
			})
		}
		return
	}

	if len(event.ID) > 64 {
		result.Errors = append(result.Errors, &ValidationError{
			Field:   "id",
			Code:    "id_too_long",
			Message: fmt.Sprintf("event ID exceeds maximum length of 64 characters (got %d)", len(event.ID)),
			Value:   event.ID,
		})
	}

	if v.config.StrictMode && !v.idPattern.MatchString(event.ID) {
		result.Errors = append(result.Errors, &ValidationError{
			Field:   "id",
			Code:    "id_invalid_format",
			Message: "event ID contains invalid characters (allowed: alphanumeric, underscore, hyphen)",
			Value:   event.ID,
		})
	}
}

// validateType validates the event type
func (v *EventSchemaValidator) validateType(event *Event, result *EventValidationResult) {
	if event.Type == "" {
		result.Errors = append(result.Errors, &ValidationError{
			Field:   "type",
			Code:    "type_required",
			Message: "event type is required",
		})
		return
	}

	// Check if type is blocked
	for _, blocked := range v.config.BlockedEventTypes {
		if event.Type == blocked {
			result.Errors = append(result.Errors, &ValidationError{
				Field:   "type",
				Code:    "type_blocked",
				Message: fmt.Sprintf("event type '%s' is blocked", event.Type),
				Value:   event.Type,
			})
			return
		}
	}

	// Check allowed types if configured
	if len(v.config.AllowedEventTypes) > 0 {
		allowed := false
		for _, t := range v.config.AllowedEventTypes {
			if event.Type == t {
				allowed = true
				break
			}
		}
		if !allowed {
			result.Errors = append(result.Errors, &ValidationError{
				Field:   "type",
				Code:    "type_not_allowed",
				Message: fmt.Sprintf("event type '%s' is not in the allowed types list", event.Type),
				Value:   event.Type,
			})
			return
		}
	}

	// Check against known types
	if !v.config.AllowUnknownEventTypes && !v.isKnownEventType(event.Type) {
		result.Errors = append(result.Errors, &ValidationError{
			Field:   "type",
			Code:    "type_unknown",
			Message: fmt.Sprintf("event type '%s' is not a known event type", event.Type),
			Value:   event.Type,
		})
	}

	// Validate format
	if len(event.Type) > 128 {
		result.Errors = append(result.Errors, &ValidationError{
			Field:   "type",
			Code:    "type_too_long",
			Message: fmt.Sprintf("event type exceeds maximum length of 128 characters (got %d)", len(event.Type)),
			Value:   event.Type,
		})
	}
}

// isKnownEventType checks if an event type is known
func (v *EventSchemaValidator) isKnownEventType(eventType EventType) bool {
	knownTypes := []EventType{
		EventTypeAgentConnect, EventTypeAgentDisconnect, EventTypeAgentHeartbeat, EventTypeAgentError,
		EventTypeJobStart, EventTypeJobComplete, EventTypeJobFail, EventTypeJobOutput,
		EventTypeStateApplyStart, EventTypeStateApplyDone, EventTypeStateApplyFail, EventTypeStateChange, EventTypeStateDrift,
		EventTypeUserLogin, EventTypeUserCommand, EventTypeUserError,
		EventTypeSystemStartup, EventTypeSystemShutdown, EventTypeSystemError,
		EventTypePolicyPass, EventTypePolicyViolation,
	}

	for _, known := range knownTypes {
		if eventType == known {
			return true
		}
	}
	return false
}

// validateSource validates the event source
func (v *EventSchemaValidator) validateSource(event *Event, result *EventValidationResult) {
	if event.Source == "" {
		result.Errors = append(result.Errors, &ValidationError{
			Field:   "source",
			Code:    "source_required",
			Message: "event source is required",
		})
		return
	}

	if len(event.Source) > v.config.MaxSourceLength {
		result.Errors = append(result.Errors, &ValidationError{
			Field:   "source",
			Code:    "source_too_long",
			Message: fmt.Sprintf("event source exceeds maximum length of %d characters (got %d)", v.config.MaxSourceLength, len(event.Source)),
			Value:   event.Source,
		})
	}

	if v.config.StrictMode && !v.sourcePattern.MatchString(event.Source) {
		result.Errors = append(result.Errors, &ValidationError{
			Field:   "source",
			Code:    "source_invalid_format",
			Message: "event source contains invalid characters (allowed: alphanumeric, forward slash, underscore, period, hyphen)",
			Value:   event.Source,
		})
	}
}

// validateTime validates the event timestamp
func (v *EventSchemaValidator) validateTime(event *Event, result *EventValidationResult) {
	if event.Time.IsZero() {
		result.Errors = append(result.Errors, &ValidationError{
			Field:   "time",
			Code:    "time_required",
			Message: "event timestamp is required",
		})
		return
	}

	now := time.Now()

	// Check for future timestamps
	if !v.config.AllowFutureTimestamps && event.Time.After(now.Add(v.config.MaxTimestampSkew)) {
		result.Errors = append(result.Errors, &ValidationError{
			Field:   "time",
			Code:    "time_in_future",
			Message: fmt.Sprintf("event timestamp is too far in the future (max skew: %v)", v.config.MaxTimestampSkew),
			Value:   event.Time,
		})
	}

	// Check for very old timestamps (potential clock issues)
	maxPastAge := 24 * time.Hour // Events older than 24 hours are suspicious
	if event.Time.Before(now.Add(-maxPastAge)) {
		result.Errors = append(result.Errors, &ValidationError{
			Field:   "time",
			Code:    "time_too_old",
			Message: fmt.Sprintf("event timestamp is older than %v", maxPastAge),
			Value:   event.Time,
		})
	}
}

// validateSeverity validates the event severity
func (v *EventSchemaValidator) validateSeverity(event *Event, result *EventValidationResult) {
	if event.Severity == "" {
		result.Errors = append(result.Errors, &ValidationError{
			Field:   "severity",
			Code:    "severity_required",
			Message: "event severity is required",
		})
		return
	}

	validSeverities := map[Severity]bool{
		SeverityDebug:    true,
		SeverityInfo:     true,
		SeverityWarning:  true,
		SeverityError:    true,
		SeverityCritical: true,
	}

	if !validSeverities[event.Severity] {
		result.Errors = append(result.Errors, &ValidationError{
			Field:   "severity",
			Code:    "severity_invalid",
			Message: fmt.Sprintf("invalid severity '%s' (allowed: debug, info, warning, error, critical)", event.Severity),
			Value:   event.Severity,
		})
	}
}

// validateCorrelationID validates the correlation ID
func (v *EventSchemaValidator) validateCorrelationID(event *Event, result *EventValidationResult) {
	if v.config.RequireCorrelationID && event.CorrelationID == "" {
		result.Errors = append(result.Errors, &ValidationError{
			Field:   "correlation_id",
			Code:    "correlation_id_required",
			Message: "correlation ID is required",
		})
		return
	}

	if event.CorrelationID != "" && len(event.CorrelationID) > 128 {
		result.Errors = append(result.Errors, &ValidationError{
			Field:   "correlation_id",
			Code:    "correlation_id_too_long",
			Message: fmt.Sprintf("correlation ID exceeds maximum length of 128 characters (got %d)", len(event.CorrelationID)),
			Value:   event.CorrelationID,
		})
	}
}

// validateTags validates the event tags
func (v *EventSchemaValidator) validateTags(event *Event, result *EventValidationResult) {
	if event.Tags == nil {
		return
	}

	// Check tag count
	if len(event.Tags) > v.config.MaxTagCount {
		result.Errors = append(result.Errors, &ValidationError{
			Field:   "tags",
			Code:    "tags_too_many",
			Message: fmt.Sprintf("event has too many tags (max: %d, got: %d)", v.config.MaxTagCount, len(event.Tags)),
			Value:   len(event.Tags),
		})
	}

	// Check required tags
	for _, required := range v.config.RequiredTags {
		if _, ok := event.Tags[required]; !ok {
			result.Errors = append(result.Errors, &ValidationError{
				Field:   fmt.Sprintf("tags.%s", required),
				Code:    "tag_required",
				Message: fmt.Sprintf("required tag '%s' is missing", required),
			})
		}
	}

	// Validate each tag
	for key, value := range event.Tags {
		// Check key length
		if len(key) > v.config.MaxTagKeyLength {
			result.Errors = append(result.Errors, &ValidationError{
				Field:   fmt.Sprintf("tags.%s", key),
				Code:    "tag_key_too_long",
				Message: fmt.Sprintf("tag key exceeds maximum length of %d characters (got %d)", v.config.MaxTagKeyLength, len(key)),
				Value:   key,
			})
		}

		// Check key format in strict mode
		if v.config.StrictMode && !v.tagKeyPattern.MatchString(key) {
			result.Errors = append(result.Errors, &ValidationError{
				Field:   fmt.Sprintf("tags.%s", key),
				Code:    "tag_key_invalid_format",
				Message: "tag key must start with a letter and contain only alphanumeric, underscore, period, or hyphen",
				Value:   key,
			})
		}

		// Check value length
		if len(value) > v.config.MaxTagValueLength {
			result.Errors = append(result.Errors, &ValidationError{
				Field:   fmt.Sprintf("tags.%s", key),
				Code:    "tag_value_too_long",
				Message: fmt.Sprintf("tag value exceeds maximum length of %d characters (got %d)", v.config.MaxTagValueLength, len(value)),
				Value:   value,
			})
		}
	}
}

// validateData validates the event data payload
func (v *EventSchemaValidator) validateData(event *Event, result *EventValidationResult) {
	if event.Data == nil {
		return
	}

	// Check field count
	if len(event.Data) > v.config.MaxDataFields {
		result.Errors = append(result.Errors, &ValidationError{
			Field:   "data",
			Code:    "data_too_many_fields",
			Message: fmt.Sprintf("event data has too many fields (max: %d, got: %d)", v.config.MaxDataFields, len(event.Data)),
			Value:   len(event.Data),
		})
	}

	// Validate data field values
	for key, value := range event.Data {
		v.validateDataValue(key, value, result)
	}
}

// validateDataValue validates a single data field value
func (v *EventSchemaValidator) validateDataValue(key string, value interface{}, result *EventValidationResult) {
	switch val := value.(type) {
	case string:
		if len(val) > v.config.MaxDataValueSize {
			result.Errors = append(result.Errors, &ValidationError{
				Field:   fmt.Sprintf("data.%s", key),
				Code:    "data_value_too_large",
				Message: fmt.Sprintf("data field value exceeds maximum size of %d bytes (got %d)", v.config.MaxDataValueSize, len(val)),
				Value:   len(val),
			})
		}
	case []interface{}:
		if len(val) > 100 {
			result.Errors = append(result.Errors, &ValidationError{
				Field:   fmt.Sprintf("data.%s", key),
				Code:    "data_array_too_large",
				Message: fmt.Sprintf("data array exceeds maximum length of 100 elements (got %d)", len(val)),
				Value:   len(val),
			})
		}
	case map[string]interface{}:
		if len(val) > v.config.MaxDataFields {
			result.Errors = append(result.Errors, &ValidationError{
				Field:   fmt.Sprintf("data.%s", key),
				Code:    "data_nested_too_large",
				Message: fmt.Sprintf("nested data object exceeds maximum fields (max: %d, got: %d)", v.config.MaxDataFields, len(val)),
				Value:   len(val),
			})
		}
	}
}

// ValidateAndError validates an event and returns an error if validation fails
func (v *EventSchemaValidator) ValidateAndError(event *Event) error {
	result := v.Validate(event)
	if !result.Valid {
		return fmt.Errorf("event validation failed: %s", result.Error())
	}
	return nil
}

// ValidatingPublisher wraps an EventPublisher with validation
type ValidatingPublisher struct {
	publisher EventPublisher
	validator *EventSchemaValidator
}

// NewValidatingPublisher creates a new validating publisher wrapper
func NewValidatingPublisher(publisher EventPublisher, config *SchemaValidatorConfig) *ValidatingPublisher {
	return &ValidatingPublisher{
		publisher: publisher,
		validator: NewEventSchemaValidator(config),
	}
}

// Publish validates and publishes an event
func (p *ValidatingPublisher) Publish(event *Event) error {
	if err := p.validator.ValidateAndError(event); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	return p.publisher.Publish(event)
}

// PublishAsync validates and publishes an event asynchronously
func (p *ValidatingPublisher) PublishAsync(event *Event) error {
	if err := p.validator.ValidateAndError(event); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	return p.publisher.PublishAsync(event)
}

// Close closes the underlying publisher
func (p *ValidatingPublisher) Close() error {
	return p.publisher.Close()
}
