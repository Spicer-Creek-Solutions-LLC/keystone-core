// Package logging provides structured logging for Keystone Core.
// Epic 15: Observability Enhancements - stdout-first logging with optional syslog.
package logging

import (
	"log"
	"os"
	"time"

	"github.com/shawnbutts/keystone-core/internal/config"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

// NewLoggerFromConfig creates a structured logger from config.LoggingConfig.
// This is the primary factory function for creating loggers in services.
func NewLoggerFromConfig(cfg *config.LoggingConfig, serviceName string) *StructuredLogger {
	// Parse log level
	level := LevelInfo
	if cfg.Level != "" {
		if parsed, ok := ParseLevel(cfg.Level); ok {
			level = parsed
		}
	}

	// Get hostname for syslog and metadata
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// Create formatter and outputs based on config
	var formatter Formatter
	var outputs []Output

	switch cfg.Output {
	case "syslog":
		// Use syslog output with syslog formatter
		facility := FacilityDaemon
		if cfg.Syslog.Facility != "" {
			if parsed, ok := ParseFacility(cfg.Syslog.Facility); ok {
				facility = parsed
			}
		}

		appName := serviceName
		if cfg.Syslog.AppName != "" {
			appName = cfg.Syslog.AppName
		}

		// Create syslog formatter (RFC 5424)
		formatter = NewSyslogFormatter(facility, appName)

		// Create syslog output
		syslogConfig := &SyslogConfig{
			Network:     cfg.Syslog.Network,
			Address:     cfg.Syslog.Address,
			Facility:    facility,
			AppName:     appName,
			Hostname:    hostname,
			DialTimeout: 5 * time.Second,
		}

		// Set defaults if not configured
		if syslogConfig.Network == "" {
			syslogConfig.Network = "unix"
		}
		if syslogConfig.Address == "" {
			syslogConfig.Address = "/dev/log"
		}

		// Configure TLS if enabled
		if cfg.Syslog.TLS.Enabled {
			syslogConfig.Network = "tcp+tls"
			syslogConfig.TLS = &SyslogTLSConfig{
				Enabled:            true,
				CACert:             cfg.Syslog.TLS.CACert,
				Cert:               cfg.Syslog.TLS.Cert,
				Key:                cfg.Syslog.TLS.Key,
				InsecureSkipVerify: cfg.Syslog.TLS.SkipVerify,
				MinVersion:         cfg.Syslog.TLS.MinVersion,
			}
		}

		syslogOutput, err := NewSyslogOutput(syslogConfig)
		if err != nil {
			// Fallback to stdout if syslog connection fails
			log.Printf("WARNING: Failed to connect to syslog (%v), falling back to stdout", err)
			outputs = []Output{NewWriterOutput(os.Stdout)}
			formatter = &JSONFormatter{}
		} else {
			outputs = []Output{syslogOutput}
		}

	default:
		// Default to stdout with configured format
		outputs = []Output{NewWriterOutput(os.Stdout)}
		switch cfg.Format {
		case "logfmt":
			formatter = &LogfmtFormatter{}
		case "text":
			formatter = &TextFormatter{}
		default:
			formatter = &JSONFormatter{}
		}
	}

	// Get version info
	versionInfo := version.Get()

	// Build metadata
	metadata := &EntryMetadata{
		Host:    hostname,
		PID:     os.Getpid(),
		Version: versionInfo.Version,
		Service: serviceName,
	}

	// Create logger config
	logConfig := Config{
		Level:           level,
		Name:            serviceName,
		Formatter:       formatter,
		Outputs:         outputs,
		IncludeCaller:   cfg.IncludeCaller,
		IncludeMetadata: true, // Always include metadata for structured logging
		Metadata:        metadata,
	}

	return NewLogger(logConfig)
}

// NewServiceLogger creates a logger for a Keystone Core service.
// This is a convenience function that applies sensible defaults.
func NewServiceLogger(serviceName string) *StructuredLogger {
	// Use default config
	cfg := &config.LoggingConfig{
		Level:  config.DefaultLoggingLevel,
		Format: config.DefaultLoggingFormat,
		Output: config.DefaultLoggingOutput,
	}

	// Check environment variables for overrides
	if level := os.Getenv("KSCORE_LOG_LEVEL"); level != "" {
		cfg.Level = level
	}
	if format := os.Getenv("KSCORE_LOG_FORMAT"); format != "" {
		cfg.Format = format
	}

	return NewLoggerFromConfig(cfg, serviceName)
}

// InitDefaultLogger initializes the global default logger for a service.
// This should be called early in main() to set up structured logging.
func InitDefaultLogger(serviceName string) *StructuredLogger {
	logger := NewServiceLogger(serviceName)
	SetDefault(logger)
	return logger
}

// InitDefaultLoggerFromConfig initializes the global default logger from config.
func InitDefaultLoggerFromConfig(cfg *config.LoggingConfig, serviceName string) *StructuredLogger {
	logger := NewLoggerFromConfig(cfg, serviceName)
	SetDefault(logger)
	return logger
}
