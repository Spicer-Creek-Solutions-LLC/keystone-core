package logging

import (
	"context"
	"io"
	"time"
)

// Level represents the severity level of a log entry
type Level int

const (
	// LevelDebug is for debug-level messages
	LevelDebug Level = iota
	// LevelInfo is for informational messages
	LevelInfo
	// LevelWarn is for warning messages
	LevelWarn
	// LevelError is for error messages
	LevelError
)

// String returns the string representation of the log level
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return "unknown"
	}
}

// ParseLevel parses a string into a Level
func ParseLevel(s string) (Level, bool) {
	switch s {
	case "debug":
		return LevelDebug, true
	case "info":
		return LevelInfo, true
	case "warn":
		return LevelWarn, true
	case "error":
		return LevelError, true
	default:
		return LevelInfo, false
	}
}

// Entry represents a single log entry
type Entry struct {
	Timestamp     time.Time              `json:"timestamp"`
	Level         Level                  `json:"level"`
	Logger        string                 `json:"logger"`
	Message       string                 `json:"message"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	Fields        map[string]interface{} `json:"fields,omitempty"`
}

// Logger defines the interface for structured logging
type Logger interface {
	// Debug logs a debug-level message
	Debug(msg string, fields ...Field)

	// Info logs an info-level message
	Info(msg string, fields ...Field)

	// Warn logs a warning-level message
	Warn(msg string, fields ...Field)

	// Error logs an error-level message
	Error(msg string, fields ...Field)

	// WithFields returns a new logger with additional fields
	WithFields(fields ...Field) Logger

	// WithCorrelationID returns a new logger with a correlation ID
	WithCorrelationID(id string) Logger

	// WithContext returns a new logger with context (extracts correlation ID if present)
	WithContext(ctx context.Context) Logger

	// SetLevel sets the minimum log level
	SetLevel(level Level)

	// GetLevel returns the current log level
	GetLevel() Level
}

// Field represents a structured log field
type Field struct {
	Key   string
	Value interface{}
}

// Fields creates multiple fields from key-value pairs
func Fields(kvPairs ...interface{}) []Field {
	if len(kvPairs)%2 != 0 {
		// Odd number of arguments, append nil
		kvPairs = append(kvPairs, nil)
	}

	fields := make([]Field, 0, len(kvPairs)/2)
	for i := 0; i < len(kvPairs); i += 2 {
		key, ok := kvPairs[i].(string)
		if !ok {
			continue
		}
		fields = append(fields, Field{Key: key, Value: kvPairs[i+1]})
	}
	return fields
}

// Common field constructors
func String(key, value string) Field {
	return Field{Key: key, Value: value}
}

func Int(key string, value int) Field {
	return Field{Key: key, Value: value}
}

func Int64(key string, value int64) Field {
	return Field{Key: key, Value: value}
}

func Float64(key string, value float64) Field {
	return Field{Key: key, Value: value}
}

func Bool(key string, value bool) Field {
	return Field{Key: key, Value: value}
}

func Duration(key string, value time.Duration) Field {
	return Field{Key: key, Value: value}
}

func Time(key string, value time.Time) Field {
	return Field{Key: key, Value: value}
}

func Error(err error) Field {
	if err == nil {
		return Field{Key: "error", Value: nil}
	}
	return Field{Key: "error", Value: err.Error()}
}

func Any(key string, value interface{}) Field {
	return Field{Key: key, Value: value}
}

// Formatter formats log entries for output
type Formatter interface {
	// Format formats a log entry into bytes
	Format(entry *Entry) ([]byte, error)
}

// Output writes formatted log entries
type Output interface {
	// Write writes formatted log data
	Write(data []byte) error

	// Close closes the output
	Close() error
}

// WriterOutput wraps an io.Writer as an Output
type WriterOutput struct {
	writer io.Writer
}

// NewWriterOutput creates a new WriterOutput
func NewWriterOutput(w io.Writer) *WriterOutput {
	return &WriterOutput{writer: w}
}

// Write writes data to the underlying writer
func (w *WriterOutput) Write(data []byte) error {
	_, err := w.writer.Write(data)
	return err
}

// Close is a no-op for WriterOutput
func (w *WriterOutput) Close() error {
	return nil
}

// SamplingConfig configures log sampling
type SamplingConfig struct {
	// Enabled enables sampling
	Enabled bool

	// Rate is the sampling rate (0.0-1.0)
	Rate float64

	// Threshold is the minimum level to sample
	Threshold Level
}

// Config represents logger configuration
type Config struct {
	// Level is the minimum log level
	Level Level

	// Name is the logger name
	Name string

	// Formatter is the log formatter
	Formatter Formatter

	// Outputs are the log outputs
	Outputs []Output

	// Sampling configures log sampling
	Sampling *SamplingConfig

	// IncludeCaller includes caller information in logs
	IncludeCaller bool
}
