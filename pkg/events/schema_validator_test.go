package events

import (
	"strings"
	"testing"
	"time"
)

func TestNewEventSchemaValidator(t *testing.T) {
	validator := NewEventSchemaValidator(nil)
	if validator == nil {
		t.Fatal("Expected validator to be created")
	}
	if validator.config == nil {
		t.Error("Expected default config to be set")
	}
}

func TestEventSchemaValidator_NilEvent(t *testing.T) {
	validator := NewEventSchemaValidator(nil)
	result := validator.Validate(nil)

	if result.Valid {
		t.Error("Expected validation to fail for nil event")
	}
	if len(result.Errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(result.Errors))
	}
	if result.Errors[0].Code != "event_nil" {
		t.Errorf("Expected error code 'event_nil', got '%s'", result.Errors[0].Code)
	}
}

func TestEventSchemaValidator_ValidEvent(t *testing.T) {
	validator := NewEventSchemaValidator(nil)

	event := NewEvent(EventTypeAgentConnect).
		Source("/test/agent").
		Severity(SeverityInfo).
		Tag("env", "test").
		DataMap(map[string]interface{}{"key": "value"}).
		Build()

	result := validator.Validate(event)

	if !result.Valid {
		t.Errorf("Expected validation to pass, got errors: %s", result.Error())
	}
}

func TestEventSchemaValidator_RequiredFields(t *testing.T) {
	config := DefaultSchemaValidatorConfig()
	config.AllowEmptyID = false
	validator := NewEventSchemaValidator(config)

	// Event missing ID
	event := &Event{
		Type:     EventTypeAgentConnect,
		Source:   "/test",
		Time:     time.Now(),
		Severity: SeverityInfo,
	}

	result := validator.Validate(event)

	if result.Valid {
		t.Error("Expected validation to fail for missing ID")
	}

	// Check that the error mentions ID
	hasIDError := false
	for _, err := range result.Errors {
		if err.Field == "id" {
			hasIDError = true
			break
		}
	}
	if !hasIDError {
		t.Error("Expected error for missing ID")
	}
}

func TestEventSchemaValidator_TypeValidation(t *testing.T) {
	config := DefaultSchemaValidatorConfig()
	config.AllowUnknownEventTypes = false
	validator := NewEventSchemaValidator(config)

	// Event with unknown type
	event := NewEvent(EventType("unknown.event.type")).
		Source("/test").
		Severity(SeverityInfo).
		Build()

	result := validator.Validate(event)

	if result.Valid {
		t.Error("Expected validation to fail for unknown event type")
	}

	// Check error code
	found := false
	for _, err := range result.Errors {
		if err.Code == "type_unknown" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error code 'type_unknown'")
	}
}

func TestEventSchemaValidator_TypeTooLong(t *testing.T) {
	validator := NewEventSchemaValidator(nil)

	// Event with very long type
	longType := EventType(strings.Repeat("a", 200))
	event := NewEvent(longType).
		Source("/test").
		Severity(SeverityInfo).
		Build()

	result := validator.Validate(event)

	if result.Valid {
		t.Error("Expected validation to fail for too long type")
	}

	found := false
	for _, err := range result.Errors {
		if err.Code == "type_too_long" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error code 'type_too_long'")
	}
}

func TestEventSchemaValidator_BlockedType(t *testing.T) {
	config := DefaultSchemaValidatorConfig()
	config.BlockedEventTypes = []EventType{EventTypeAgentHeartbeat}
	validator := NewEventSchemaValidator(config)

	event := NewEvent(EventTypeAgentHeartbeat).
		Source("/test").
		Severity(SeverityInfo).
		Build()

	result := validator.Validate(event)

	if result.Valid {
		t.Error("Expected validation to fail for blocked type")
	}

	found := false
	for _, err := range result.Errors {
		if err.Code == "type_blocked" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error code 'type_blocked'")
	}
}

func TestEventSchemaValidator_AllowedTypes(t *testing.T) {
	config := DefaultSchemaValidatorConfig()
	config.AllowedEventTypes = []EventType{EventTypeAgentConnect}
	validator := NewEventSchemaValidator(config)

	// Allowed type should pass
	allowedEvent := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Build()

	result := validator.Validate(allowedEvent)
	if !result.Valid {
		t.Errorf("Expected allowed event to pass: %s", result.Error())
	}

	// Non-allowed type should fail
	notAllowedEvent := NewEvent(EventTypeAgentDisconnect).
		Source("/test").
		Severity(SeverityInfo).
		Build()

	result = validator.Validate(notAllowedEvent)
	if result.Valid {
		t.Error("Expected non-allowed event to fail")
	}
}

func TestEventSchemaValidator_SourceValidation(t *testing.T) {
	validator := NewEventSchemaValidator(nil)

	// Event missing source
	event := &Event{
		ID:       "test-id",
		Type:     EventTypeAgentConnect,
		Time:     time.Now(),
		Severity: SeverityInfo,
	}

	result := validator.Validate(event)

	if result.Valid {
		t.Error("Expected validation to fail for missing source")
	}

	found := false
	for _, err := range result.Errors {
		if err.Code == "source_required" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error code 'source_required'")
	}
}

func TestEventSchemaValidator_SourceTooLong(t *testing.T) {
	config := DefaultSchemaValidatorConfig()
	config.MaxSourceLength = 50
	validator := NewEventSchemaValidator(config)

	event := NewEvent(EventTypeAgentConnect).
		Source(strings.Repeat("a", 100)).
		Severity(SeverityInfo).
		Build()

	result := validator.Validate(event)

	if result.Valid {
		t.Error("Expected validation to fail for too long source")
	}

	found := false
	for _, err := range result.Errors {
		if err.Code == "source_too_long" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error code 'source_too_long'")
	}
}

func TestEventSchemaValidator_TimeValidation(t *testing.T) {
	validator := NewEventSchemaValidator(nil)

	// Event with zero time
	event := &Event{
		ID:       "test-id",
		Type:     EventTypeAgentConnect,
		Source:   "/test",
		Severity: SeverityInfo,
	}

	result := validator.Validate(event)

	if result.Valid {
		t.Error("Expected validation to fail for zero time")
	}

	found := false
	for _, err := range result.Errors {
		if err.Code == "time_required" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error code 'time_required'")
	}
}

func TestEventSchemaValidator_FutureTimestamp(t *testing.T) {
	config := DefaultSchemaValidatorConfig()
	config.AllowFutureTimestamps = false
	config.MaxTimestampSkew = 1 * time.Minute
	validator := NewEventSchemaValidator(config)

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Build()
	event.Time = time.Now().Add(10 * time.Minute) // 10 minutes in the future

	result := validator.Validate(event)

	if result.Valid {
		t.Error("Expected validation to fail for future timestamp")
	}

	found := false
	for _, err := range result.Errors {
		if err.Code == "time_in_future" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error code 'time_in_future'")
	}
}

func TestEventSchemaValidator_OldTimestamp(t *testing.T) {
	validator := NewEventSchemaValidator(nil)

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Build()
	event.Time = time.Now().Add(-48 * time.Hour) // 48 hours in the past

	result := validator.Validate(event)

	if result.Valid {
		t.Error("Expected validation to fail for old timestamp")
	}

	found := false
	for _, err := range result.Errors {
		if err.Code == "time_too_old" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error code 'time_too_old'")
	}
}

func TestEventSchemaValidator_SeverityValidation(t *testing.T) {
	validator := NewEventSchemaValidator(nil)

	// Event with invalid severity
	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(Severity("invalid")).
		Build()

	result := validator.Validate(event)

	if result.Valid {
		t.Error("Expected validation to fail for invalid severity")
	}

	found := false
	for _, err := range result.Errors {
		if err.Code == "severity_invalid" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error code 'severity_invalid'")
	}
}

func TestEventSchemaValidator_CorrelationIDRequired(t *testing.T) {
	config := DefaultSchemaValidatorConfig()
	config.RequireCorrelationID = true
	validator := NewEventSchemaValidator(config)

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Build()

	result := validator.Validate(event)

	if result.Valid {
		t.Error("Expected validation to fail for missing correlation ID")
	}

	found := false
	for _, err := range result.Errors {
		if err.Code == "correlation_id_required" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error code 'correlation_id_required'")
	}
}

func TestEventSchemaValidator_TagValidation(t *testing.T) {
	config := DefaultSchemaValidatorConfig()
	config.MaxTagCount = 2
	config.MaxTagKeyLength = 10
	config.MaxTagValueLength = 20
	validator := NewEventSchemaValidator(config)

	// Too many tags
	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Tag("key1", "value1").
		Tag("key2", "value2").
		Tag("key3", "value3").
		Build()

	result := validator.Validate(event)

	if result.Valid {
		t.Error("Expected validation to fail for too many tags")
	}

	found := false
	for _, err := range result.Errors {
		if err.Code == "tags_too_many" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error code 'tags_too_many'")
	}
}

func TestEventSchemaValidator_TagKeyTooLong(t *testing.T) {
	config := DefaultSchemaValidatorConfig()
	config.MaxTagKeyLength = 5
	validator := NewEventSchemaValidator(config)

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Tag("this_key_is_too_long", "value").
		Build()

	result := validator.Validate(event)

	if result.Valid {
		t.Error("Expected validation to fail for tag key too long")
	}

	found := false
	for _, err := range result.Errors {
		if err.Code == "tag_key_too_long" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error code 'tag_key_too_long'")
	}
}

func TestEventSchemaValidator_TagValueTooLong(t *testing.T) {
	config := DefaultSchemaValidatorConfig()
	config.MaxTagValueLength = 10
	validator := NewEventSchemaValidator(config)

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Tag("key", strings.Repeat("a", 100)).
		Build()

	result := validator.Validate(event)

	if result.Valid {
		t.Error("Expected validation to fail for tag value too long")
	}

	found := false
	for _, err := range result.Errors {
		if err.Code == "tag_value_too_long" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error code 'tag_value_too_long'")
	}
}

func TestEventSchemaValidator_RequiredTags(t *testing.T) {
	config := DefaultSchemaValidatorConfig()
	config.RequiredTags = []string{"env", "region"}
	validator := NewEventSchemaValidator(config)

	// Missing required tag
	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Tag("env", "prod").
		Build()

	result := validator.Validate(event)

	if result.Valid {
		t.Error("Expected validation to fail for missing required tag")
	}

	found := false
	for _, err := range result.Errors {
		if err.Code == "tag_required" && strings.Contains(err.Message, "region") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error for missing 'region' tag")
	}
}

func TestEventSchemaValidator_DataFieldCount(t *testing.T) {
	config := DefaultSchemaValidatorConfig()
	config.MaxDataFields = 3
	validator := NewEventSchemaValidator(config)

	data := map[string]interface{}{}
	for i := 0; i < 10; i++ {
		data[strings.Repeat("k", i+1)] = "value"
	}

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		DataMap(data).
		Build()

	result := validator.Validate(event)

	if result.Valid {
		t.Error("Expected validation to fail for too many data fields")
	}

	found := false
	for _, err := range result.Errors {
		if err.Code == "data_too_many_fields" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error code 'data_too_many_fields'")
	}
}

func TestEventSchemaValidator_DataValueTooLarge(t *testing.T) {
	config := DefaultSchemaValidatorConfig()
	config.MaxDataValueSize = 100
	validator := NewEventSchemaValidator(config)

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		DataMap(map[string]interface{}{
			"large_field": strings.Repeat("a", 1000),
		}).
		Build()

	result := validator.Validate(event)

	if result.Valid {
		t.Error("Expected validation to fail for data value too large")
	}

	found := false
	for _, err := range result.Errors {
		if err.Code == "data_value_too_large" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error code 'data_value_too_large'")
	}
}

func TestEventSchemaValidator_StrictMode(t *testing.T) {
	config := StrictSchemaValidatorConfig()
	validator := NewEventSchemaValidator(config)

	// Event with invalid ID format
	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		CorrelationID("test-correlation").
		Build()
	event.ID = "invalid id with spaces!"

	result := validator.Validate(event)

	if result.Valid {
		t.Error("Expected strict validation to fail for invalid ID format")
	}

	found := false
	for _, err := range result.Errors {
		if err.Code == "id_invalid_format" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error code 'id_invalid_format'")
	}
}

func TestEventSchemaValidator_StrictModeTagKeyFormat(t *testing.T) {
	config := StrictSchemaValidatorConfig()
	validator := NewEventSchemaValidator(config)

	event := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		CorrelationID("test-correlation").
		Tag("123invalid", "value"). // Starts with number
		Build()

	result := validator.Validate(event)

	if result.Valid {
		t.Error("Expected strict validation to fail for invalid tag key format")
	}

	found := false
	for _, err := range result.Errors {
		if err.Code == "tag_key_invalid_format" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error code 'tag_key_invalid_format'")
	}
}

func TestEventSchemaValidator_ValidateAndError(t *testing.T) {
	validator := NewEventSchemaValidator(nil)

	// Valid event
	validEvent := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Build()

	err := validator.ValidateAndError(validEvent)
	if err != nil {
		t.Errorf("Expected no error for valid event, got: %v", err)
	}

	// Invalid event
	invalidEvent := &Event{}
	err = validator.ValidateAndError(invalidEvent)
	if err == nil {
		t.Error("Expected error for invalid event")
	}
}

func TestValidationResult_Summary(t *testing.T) {
	result := &EventValidationResult{
		Valid: true,
	}
	if result.Summary() != "validation passed" {
		t.Errorf("Expected 'validation passed', got '%s'", result.Summary())
	}

	result = &EventValidationResult{
		Valid: false,
		Errors: []*ValidationError{
			{Field: "id", Code: "id_required", Message: "event ID is required"},
			{Field: "type", Code: "type_required", Message: "event type is required"},
		},
	}
	summary := result.Summary()
	if !strings.Contains(summary, "2 validation error") {
		t.Errorf("Expected summary to mention 2 errors, got '%s'", summary)
	}
}

func TestValidationError_Error(t *testing.T) {
	err := &ValidationError{
		Field:   "test_field",
		Code:    "test_code",
		Message: "test message",
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "test_field") {
		t.Error("Expected error string to contain field name")
	}
	if !strings.Contains(errStr, "test message") {
		t.Error("Expected error string to contain message")
	}
}

func TestValidatingPublisher(t *testing.T) {
	mockPublisher := &mockEventPublisher{}
	validatingPub := NewValidatingPublisher(mockPublisher, nil)

	// Valid event should be published
	validEvent := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Build()

	err := validatingPub.Publish(validEvent)
	if err != nil {
		t.Errorf("Expected valid event to be published: %v", err)
	}
	if mockPublisher.publishCount != 1 {
		t.Errorf("Expected 1 publish, got %d", mockPublisher.publishCount)
	}

	// Invalid event should not be published
	invalidEvent := &Event{} // Missing required fields
	err = validatingPub.Publish(invalidEvent)
	if err == nil {
		t.Error("Expected validation error for invalid event")
	}
	if mockPublisher.publishCount != 1 {
		t.Error("Expected invalid event to not be published")
	}
}

func TestValidatingPublisher_PublishAsync(t *testing.T) {
	mockPublisher := &mockEventPublisher{}
	validatingPub := NewValidatingPublisher(mockPublisher, nil)

	// Valid event should be published
	validEvent := NewEvent(EventTypeAgentConnect).
		Source("/test").
		Severity(SeverityInfo).
		Build()

	err := validatingPub.PublishAsync(validEvent)
	if err != nil {
		t.Errorf("Expected valid event to be published async: %v", err)
	}
	if mockPublisher.asyncPublishCount != 1 {
		t.Errorf("Expected 1 async publish, got %d", mockPublisher.asyncPublishCount)
	}
}

// Mock publisher for testing
type mockEventPublisher struct {
	publishCount      int
	asyncPublishCount int
}

func (p *mockEventPublisher) Publish(event *Event) error {
	p.publishCount++
	return nil
}

func (p *mockEventPublisher) PublishAsync(event *Event) error {
	p.asyncPublishCount++
	return nil
}

func (p *mockEventPublisher) Close() error {
	return nil
}

func TestDefaultSchemaValidatorConfig(t *testing.T) {
	config := DefaultSchemaValidatorConfig()

	if config.MaxTagCount <= 0 {
		t.Error("Expected MaxTagCount to be positive")
	}
	if config.MaxDataFields <= 0 {
		t.Error("Expected MaxDataFields to be positive")
	}
	if config.MaxTimestampSkew <= 0 {
		t.Error("Expected MaxTimestampSkew to be positive")
	}
}

func TestStrictSchemaValidatorConfig(t *testing.T) {
	config := StrictSchemaValidatorConfig()

	if config.AllowUnknownEventTypes {
		t.Error("Expected strict config to not allow unknown event types")
	}
	if config.AllowEmptyID {
		t.Error("Expected strict config to not allow empty ID")
	}
	if !config.RequireCorrelationID {
		t.Error("Expected strict config to require correlation ID")
	}
	if !config.StrictMode {
		t.Error("Expected strict config to have StrictMode enabled")
	}
}
