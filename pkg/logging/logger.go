package logging

import (
	"context"
	"crypto/rand"
	"math"
	"os"
	"sync"
	"time"
)

// StructuredLogger implements the Logger interface
type StructuredLogger struct {
	config        Config
	baseFields    map[string]interface{}
	correlationID string
	mu            sync.RWMutex
	samplingState *samplingState
}

// samplingState tracks sampling state
type samplingState struct {
	counter uint64
	mu      sync.Mutex
}

// NewLogger creates a new structured logger
func NewLogger(config Config) *StructuredLogger {
	// Set defaults
	if config.Formatter == nil {
		config.Formatter = &JSONFormatter{}
	}
	if len(config.Outputs) == 0 {
		config.Outputs = []Output{NewWriterOutput(os.Stdout)}
	}

	logger := &StructuredLogger{
		config:     config,
		baseFields: make(map[string]interface{}),
	}

	// Initialize sampling state if enabled
	if config.Sampling != nil && config.Sampling.Enabled {
		logger.samplingState = &samplingState{}
	}

	return logger
}

// NewDefaultLogger creates a logger with default configuration
func NewDefaultLogger(name string) *StructuredLogger {
	return NewLogger(Config{
		Name:      name,
		Level:     LevelInfo,
		Formatter: &JSONFormatter{},
		Outputs:   []Output{NewWriterOutput(os.Stdout)},
	})
}

// Debug logs a debug-level message
func (l *StructuredLogger) Debug(msg string, fields ...Field) {
	l.log(LevelDebug, msg, fields...)
}

// Info logs an info-level message
func (l *StructuredLogger) Info(msg string, fields ...Field) {
	l.log(LevelInfo, msg, fields...)
}

// Warn logs a warning-level message
func (l *StructuredLogger) Warn(msg string, fields ...Field) {
	l.log(LevelWarn, msg, fields...)
}

// Error logs an error-level message
func (l *StructuredLogger) Error(msg string, fields ...Field) {
	l.log(LevelError, msg, fields...)
}

// WithFields returns a new logger with additional fields
func (l *StructuredLogger) WithFields(fields ...Field) Logger {
	l.mu.RLock()
	newFields := make(map[string]interface{}, len(l.baseFields)+len(fields))
	for k, v := range l.baseFields {
		newFields[k] = v
	}
	l.mu.RUnlock()

	for _, f := range fields {
		newFields[f.Key] = f.Value
	}

	return &StructuredLogger{
		config:        l.config,
		baseFields:    newFields,
		correlationID: l.correlationID,
		samplingState: l.samplingState,
	}
}

// WithCorrelationID returns a new logger with a correlation ID
func (l *StructuredLogger) WithCorrelationID(id string) Logger {
	l.mu.RLock()
	newFields := make(map[string]interface{}, len(l.baseFields))
	for k, v := range l.baseFields {
		newFields[k] = v
	}
	l.mu.RUnlock()

	return &StructuredLogger{
		config:        l.config,
		baseFields:    newFields,
		correlationID: id,
		samplingState: l.samplingState,
	}
}

// WithContext returns a new logger with context (extracts correlation ID if present)
func (l *StructuredLogger) WithContext(ctx context.Context) Logger {
	if id, ok := CorrelationIDFromContext(ctx); ok {
		return l.WithCorrelationID(id)
	}
	return l
}

// SetLevel sets the minimum log level
func (l *StructuredLogger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.config.Level = level
}

// GetLevel returns the current log level
func (l *StructuredLogger) GetLevel() Level {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.config.Level
}

// log is the internal logging method
func (l *StructuredLogger) log(level Level, msg string, fields ...Field) {
	// Check if this level should be logged
	l.mu.RLock()
	currentLevel := l.config.Level
	l.mu.RUnlock()

	if level < currentLevel {
		return
	}

	// Check sampling
	if !l.shouldSample(level) {
		return
	}

	// Build entry
	entry := &Entry{
		Timestamp:     time.Now(),
		Level:         level,
		Logger:        l.config.Name,
		Message:       msg,
		CorrelationID: l.correlationID,
		Fields:        make(map[string]interface{}),
	}

	// Add base fields
	l.mu.RLock()
	for k, v := range l.baseFields {
		entry.Fields[k] = v
	}
	l.mu.RUnlock()

	// Add entry-specific fields
	for _, f := range fields {
		entry.Fields[f.Key] = f.Value
	}

	// Format entry
	data, err := l.config.Formatter.Format(entry)
	if err != nil {
		// Failed to format, write error to stderr
		os.Stderr.WriteString("Failed to format log entry: " + err.Error() + "\n")
		return
	}

	// Write to all outputs
	for _, output := range l.config.Outputs {
		if err := output.Write(data); err != nil {
			// Failed to write, ignore (logging errors shouldn't crash the app)
			continue
		}
	}
}

// shouldSample determines if a log entry should be sampled
func (l *StructuredLogger) shouldSample(level Level) bool {
	if l.samplingState == nil {
		return true
	}

	l.mu.RLock()
	sampling := l.config.Sampling
	l.mu.RUnlock()

	if sampling == nil || !sampling.Enabled {
		return true
	}

	// Always log messages at or above threshold
	if level >= sampling.Threshold {
		return true
	}

	// Sample based on rate
	if sampling.Rate >= 1.0 {
		return true
	}

	if sampling.Rate <= 0.0 {
		return false
	}

	// Use random sampling
	b := make([]byte, 1)
	rand.Read(b)
	threshold := uint8(sampling.Rate * math.MaxUint8)
	return b[0] < threshold
}

// Close closes all outputs
func (l *StructuredLogger) Close() error {
	var lastErr error
	for _, output := range l.config.Outputs {
		if err := output.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Global default logger
var defaultLogger Logger = NewDefaultLogger("titan")
var defaultLoggerMu sync.RWMutex

// SetDefault sets the default logger
func SetDefault(logger Logger) {
	defaultLoggerMu.Lock()
	defer defaultLoggerMu.Unlock()
	defaultLogger = logger
}

// Default returns the default logger
func Default() Logger {
	defaultLoggerMu.RLock()
	defer defaultLoggerMu.RUnlock()
	return defaultLogger
}

// Package-level logging functions using the default logger

// Debug logs a debug-level message using the default logger
func Debug(msg string, fields ...Field) {
	Default().Debug(msg, fields...)
}

// Info logs an info-level message using the default logger
func Info(msg string, fields ...Field) {
	Default().Info(msg, fields...)
}

// Warn logs a warning-level message using the default logger
func Warn(msg string, fields ...Field) {
	Default().Warn(msg, fields...)
}

// Error logs an error-level message using the default logger
func ErrorLog(msg string, fields ...Field) {
	Default().Error(msg, fields...)
}

// WithFields returns a new logger with additional fields using the default logger
func WithFields(fields ...Field) Logger {
	return Default().WithFields(fields...)
}

// WithCorrelationID returns a new logger with a correlation ID using the default logger
func WithCorrelationID(id string) Logger {
	return Default().WithCorrelationID(id)
}

// WithContext returns a new logger with context using the default logger
func WithContext(ctx context.Context) Logger {
	return Default().WithContext(ctx)
}
