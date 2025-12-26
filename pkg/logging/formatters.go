package logging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// JSONFormatter formats log entries as JSON
type JSONFormatter struct {
	// PrettyPrint enables pretty-printed JSON
	PrettyPrint bool
}

// Format formats an entry as JSON
func (f *JSONFormatter) Format(entry *Entry) ([]byte, error) {
	// Create output map
	output := make(map[string]interface{})
	output["timestamp"] = entry.Timestamp.Format(time.RFC3339Nano)
	output["level"] = entry.Level.String()
	output["logger"] = entry.Logger
	output["message"] = entry.Message

	if entry.CorrelationID != "" {
		output["correlation_id"] = entry.CorrelationID
	}

	// Add fields
	for k, v := range entry.Fields {
		output[k] = v
	}

	var data []byte
	var err error

	if f.PrettyPrint {
		data, err = json.MarshalIndent(output, "", "  ")
	} else {
		data, err = json.Marshal(output)
	}

	if err != nil {
		return nil, err
	}

	// Add newline
	data = append(data, '\n')
	return data, nil
}

// LogfmtFormatter formats log entries as logfmt (key=value pairs)
type LogfmtFormatter struct{}

// Format formats an entry as logfmt
func (f *LogfmtFormatter) Format(entry *Entry) ([]byte, error) {
	var buf bytes.Buffer

	// Standard fields
	writeKeyValue(&buf, "timestamp", entry.Timestamp.Format(time.RFC3339Nano))
	writeKeyValue(&buf, "level", entry.Level.String())
	writeKeyValue(&buf, "logger", entry.Logger)
	writeKeyValue(&buf, "message", entry.Message)

	if entry.CorrelationID != "" {
		writeKeyValue(&buf, "correlation_id", entry.CorrelationID)
	}

	// Additional fields (sorted for consistency)
	if len(entry.Fields) > 0 {
		keys := make([]string, 0, len(entry.Fields))
		for k := range entry.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			writeKeyValue(&buf, k, entry.Fields[k])
		}
	}

	// Remove trailing space and add newline
	data := buf.Bytes()
	if len(data) > 0 && data[len(data)-1] == ' ' {
		data = data[:len(data)-1]
	}
	data = append(data, '\n')

	return data, nil
}

// writeKeyValue writes a key-value pair in logfmt format
func writeKeyValue(buf *bytes.Buffer, key string, value interface{}) {
	buf.WriteString(key)
	buf.WriteByte('=')

	// Format value
	var valStr string
	switch v := value.(type) {
	case string:
		// Quote if contains spaces or special characters
		if needsQuoting(v) {
			valStr = fmt.Sprintf("%q", v)
		} else {
			valStr = v
		}
	case time.Time:
		valStr = v.Format(time.RFC3339Nano)
	case time.Duration:
		valStr = v.String()
	case fmt.Stringer:
		valStr = v.String()
	default:
		valStr = fmt.Sprintf("%v", v)
	}

	buf.WriteString(valStr)
	buf.WriteByte(' ')
}

// needsQuoting returns true if a string needs quoting in logfmt
func needsQuoting(s string) bool {
	if s == "" {
		return true
	}
	return strings.ContainsAny(s, " \t\n\r\"=")
}

// TextFormatter formats log entries as human-readable text
type TextFormatter struct {
	// DisableColors disables colored output
	DisableColors bool

	// DisableTimestamp disables timestamp in output
	DisableTimestamp bool
}

// Format formats an entry as text
func (f *TextFormatter) Format(entry *Entry) ([]byte, error) {
	var buf bytes.Buffer

	// Timestamp
	if !f.DisableTimestamp {
		buf.WriteString(entry.Timestamp.Format("2006-01-02 15:04:05.000"))
		buf.WriteByte(' ')
	}

	// Level (with color)
	levelStr := strings.ToUpper(entry.Level.String())
	if !f.DisableColors {
		levelStr = colorize(entry.Level, levelStr)
	}
	buf.WriteString(fmt.Sprintf("[%-5s]", levelStr))
	buf.WriteByte(' ')

	// Logger
	if entry.Logger != "" {
		buf.WriteString(fmt.Sprintf("[%s]", entry.Logger))
		buf.WriteByte(' ')
	}

	// Message
	buf.WriteString(entry.Message)

	// Correlation ID
	if entry.CorrelationID != "" {
		buf.WriteString(fmt.Sprintf(" correlation_id=%s", entry.CorrelationID))
	}

	// Fields
	if len(entry.Fields) > 0 {
		// Sort keys for consistency
		keys := make([]string, 0, len(entry.Fields))
		for k := range entry.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			buf.WriteString(fmt.Sprintf(" %s=%v", k, entry.Fields[k]))
		}
	}

	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorGray   = "\033[37m"
)

// colorize adds color to a string based on log level
func colorize(level Level, s string) string {
	switch level {
	case LevelDebug:
		return colorGray + s + colorReset
	case LevelInfo:
		return colorBlue + s + colorReset
	case LevelWarn:
		return colorYellow + s + colorReset
	case LevelError:
		return colorRed + s + colorReset
	default:
		return s
	}
}
