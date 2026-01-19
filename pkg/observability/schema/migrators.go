package schema

import (
	"fmt"
	"time"
)

// logMigratorV1ToV2 migrates log entries from V1 to V2
type logMigratorV1ToV2 struct{}

func (m *logMigratorV1ToV2) FromVersion() int { return 1 }
func (m *logMigratorV1ToV2) ToVersion() int   { return 2 }

func (m *logMigratorV1ToV2) Migrate(data map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// Copy existing fields
	for k, v := range data {
		result[k] = v
	}

	// Add correlation_id if not present
	if _, ok := result["correlation_id"]; !ok {
		result["correlation_id"] = ""
	}

	// Add metadata block if not present
	if _, ok := result["metadata"]; !ok {
		result["metadata"] = map[string]interface{}{
			"host":    "",
			"pid":     0,
			"version": "",
			"service": "",
			"caller":  "",
		}
	}

	return result, nil
}

func (m *logMigratorV1ToV2) Describe() []string {
	return []string{
		"Added correlation_id field (default: empty string)",
		"Added metadata block with host, pid, version, service, caller fields",
	}
}

// metricMigratorV1ToV2 migrates metrics from V1 to V2
type metricMigratorV1ToV2 struct{}

func (m *metricMigratorV1ToV2) FromVersion() int { return 1 }
func (m *metricMigratorV1ToV2) ToVersion() int   { return 2 }

func (m *metricMigratorV1ToV2) Migrate(data map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// Copy existing fields
	for k, v := range data {
		result[k] = v
	}

	// Add help text if not present
	if _, ok := result["help"]; !ok {
		name, _ := result["name"].(string)
		result["help"] = fmt.Sprintf("Metric: %s", name)
	}

	// Add unit if not present
	if _, ok := result["unit"]; !ok {
		result["unit"] = ""
	}

	// Add histogram block for histogram type
	metricType, _ := result["type"].(string)
	if metricType == "histogram" {
		if _, ok := result["histogram"]; !ok {
			result["histogram"] = map[string]interface{}{
				"buckets": []interface{}{},
				"count":   0,
				"sum":     0.0,
			}
		}
	}

	return result, nil
}

func (m *metricMigratorV1ToV2) Describe() []string {
	return []string{
		"Added help field with default metric description",
		"Added unit field (default: empty string)",
		"Added histogram block for histogram-type metrics",
	}
}

// traceMigratorV1ToV2 migrates trace spans from V1 to V2
type traceMigratorV1ToV2 struct{}

func (m *traceMigratorV1ToV2) FromVersion() int { return 1 }
func (m *traceMigratorV1ToV2) ToVersion() int   { return 2 }

func (m *traceMigratorV1ToV2) Migrate(data map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// Copy existing fields, renaming tags to attributes
	for k, v := range data {
		if k == "tags" {
			result["attributes"] = v
		} else {
			result[k] = v
		}
	}

	// Calculate end_time from start_time + duration
	if _, ok := result["end_time"]; !ok {
		if startTime, ok := result["start_time"]; ok {
			if duration, ok := result["duration"]; ok {
				var start time.Time
				switch st := startTime.(type) {
				case time.Time:
					start = st
				case string:
					start, _ = time.Parse(time.RFC3339, st)
				}

				var dur int64
				switch d := duration.(type) {
				case float64:
					dur = int64(d)
				case int:
					dur = int64(d)
				case int64:
					dur = d
				}

				if !start.IsZero() && dur > 0 {
					endTime := start.Add(time.Duration(dur) * time.Microsecond)
					result["end_time"] = endTime.Format(time.RFC3339Nano)
				}
			}
		}
	}

	// Add status if not present
	if _, ok := result["status"]; !ok {
		result["status"] = map[string]interface{}{
			"code":    "OK",
			"message": "",
		}
	}

	// Add events array if not present
	if _, ok := result["events"]; !ok {
		result["events"] = []interface{}{}
	}

	// Add links array if not present
	if _, ok := result["links"]; !ok {
		result["links"] = []interface{}{}
	}

	// Ensure attributes exists (if tags didn't exist)
	if _, ok := result["attributes"]; !ok {
		result["attributes"] = map[string]interface{}{}
	}

	return result, nil
}

func (m *traceMigratorV1ToV2) Describe() []string {
	return []string{
		"Renamed tags field to attributes (OpenTelemetry alignment)",
		"Added end_time calculated from start_time + duration",
		"Added status block with code and message",
		"Added events array for span events",
		"Added links array for span links",
	}
}

// auditMigratorV1ToV2 migrates audit entries from V1 to V2
type auditMigratorV1ToV2 struct{}

func (m *auditMigratorV1ToV2) FromVersion() int { return 1 }
func (m *auditMigratorV1ToV2) ToVersion() int   { return 2 }

func (m *auditMigratorV1ToV2) Migrate(data map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// Copy and transform fields
	result["timestamp"] = data["timestamp"]
	result["action"] = data["action"]
	result["outcome"] = data["outcome"]

	// Generate event_id if not present
	if eventID, ok := data["event_id"]; ok {
		result["event_id"] = eventID
	} else {
		// Generate a simple ID based on timestamp
		ts, _ := data["timestamp"].(string)
		result["event_id"] = fmt.Sprintf("audit-%s", ts)
	}

	// Convert actor from string to object
	if actor, ok := data["actor"].(string); ok {
		result["actor"] = map[string]interface{}{
			"id":         actor,
			"type":       "user",
			"name":       actor,
			"ip_address": "",
		}
	} else if actorObj, ok := data["actor"].(map[string]interface{}); ok {
		// Already an object, ensure all fields exist
		if _, ok := actorObj["ip_address"]; !ok {
			actorObj["ip_address"] = ""
		}
		result["actor"] = actorObj
	}

	// Convert resource from string to object
	if resource, ok := data["resource"].(string); ok {
		result["resource"] = map[string]interface{}{
			"id":   resource,
			"type": "unknown",
			"name": resource,
		}
	} else if resourceObj, ok := data["resource"].(map[string]interface{}); ok {
		result["resource"] = resourceObj
	}

	// Add new fields with defaults
	if reason, ok := data["reason"]; ok {
		result["reason"] = reason
	} else {
		result["reason"] = ""
	}

	if context, ok := data["context"]; ok {
		result["context"] = context
	} else {
		result["context"] = map[string]interface{}{}
	}

	if corrID, ok := data["correlation_id"]; ok {
		result["correlation_id"] = corrID
	} else {
		result["correlation_id"] = ""
	}

	return result, nil
}

func (m *auditMigratorV1ToV2) Describe() []string {
	return []string{
		"Added event_id field for unique identification",
		"Converted actor from string to structured object with id, type, name, ip_address",
		"Converted resource from string to structured object with id, type, name",
		"Added reason field for failure details",
		"Added context object for additional information",
		"Added correlation_id for request tracing",
	}
}

// eventMigratorV1ToV2 migrates events from V1 to V2 (CloudEvents format)
type eventMigratorV1ToV2 struct{}

func (m *eventMigratorV1ToV2) FromVersion() int { return 1 }
func (m *eventMigratorV1ToV2) ToVersion() int   { return 2 }

func (m *eventMigratorV1ToV2) Migrate(data map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// Add CloudEvents specversion
	result["specversion"] = "1.0"

	// Copy and rename fields
	result["id"] = data["id"]
	result["type"] = data["type"]
	result["source"] = data["source"]

	// Rename timestamp to time
	if ts, ok := data["timestamp"]; ok {
		result["time"] = ts
	} else if t, ok := data["time"]; ok {
		result["time"] = t
	}

	// Copy data
	if d, ok := data["data"]; ok {
		result["data"] = d
	}

	// Add new CloudEvents fields with defaults
	if subject, ok := data["subject"]; ok {
		result["subject"] = subject
	} else {
		result["subject"] = ""
	}

	if datacontenttype, ok := data["datacontenttype"]; ok {
		result["datacontenttype"] = datacontenttype
	} else {
		result["datacontenttype"] = "application/json"
	}

	if dataschema, ok := data["dataschema"]; ok {
		result["dataschema"] = dataschema
	} else {
		result["dataschema"] = ""
	}

	return result, nil
}

func (m *eventMigratorV1ToV2) Describe() []string {
	return []string{
		"Added specversion field set to '1.0' (CloudEvents)",
		"Renamed timestamp to time (CloudEvents alignment)",
		"Added subject field",
		"Added datacontenttype field (default: application/json)",
		"Added dataschema field for data schema URI",
	}
}

// Reverse migrators (for downgrade support)

// logMigratorV2ToV1 migrates log entries from V2 to V1
type logMigratorV2ToV1 struct{}

func (m *logMigratorV2ToV1) FromVersion() int { return 2 }
func (m *logMigratorV2ToV1) ToVersion() int   { return 1 }

func (m *logMigratorV2ToV1) Migrate(data map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// Copy fields, omitting V2-only fields
	for k, v := range data {
		switch k {
		case "correlation_id", "metadata":
			// Skip V2-only fields
		default:
			result[k] = v
		}
	}

	return result, nil
}

func (m *logMigratorV2ToV1) Describe() []string {
	return []string{
		"Removed correlation_id field",
		"Removed metadata block",
	}
}

// traceMigratorV2ToV1 migrates trace spans from V2 to V1
type traceMigratorV2ToV1 struct{}

func (m *traceMigratorV2ToV1) FromVersion() int { return 2 }
func (m *traceMigratorV2ToV1) ToVersion() int   { return 1 }

func (m *traceMigratorV2ToV1) Migrate(data map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// Copy fields, transforming and omitting as needed
	for k, v := range data {
		switch k {
		case "attributes":
			// Rename back to tags
			result["tags"] = v
		case "end_time", "status", "events", "links":
			// Skip V2-only fields
		default:
			result[k] = v
		}
	}

	return result, nil
}

func (m *traceMigratorV2ToV1) Describe() []string {
	return []string{
		"Renamed attributes back to tags",
		"Removed end_time, status, events, links fields",
	}
}

// eventMigratorV2ToV1 migrates events from V2 to V1
type eventMigratorV2ToV1 struct{}

func (m *eventMigratorV2ToV1) FromVersion() int { return 2 }
func (m *eventMigratorV2ToV1) ToVersion() int   { return 1 }

func (m *eventMigratorV2ToV1) Migrate(data map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	result["id"] = data["id"]
	result["type"] = data["type"]
	result["source"] = data["source"]
	result["data"] = data["data"]

	// Rename time back to timestamp
	if t, ok := data["time"]; ok {
		result["timestamp"] = t
	}

	return result, nil
}

func (m *eventMigratorV2ToV1) Describe() []string {
	return []string{
		"Removed specversion field",
		"Renamed time back to timestamp",
		"Removed subject, datacontenttype, dataschema fields",
	}
}
