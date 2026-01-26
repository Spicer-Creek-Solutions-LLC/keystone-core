package logging

import (
	"testing"

	"github.com/shawnbutts/keystone-core/internal/config"
)

func TestNewLoggerFromConfig(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *config.LoggingConfig
		serviceName string
		wantLevel   Level
	}{
		{
			name: "default config",
			cfg: &config.LoggingConfig{
				Level:  "info",
				Format: "json",
			},
			serviceName: "test-service",
			wantLevel:   LevelInfo,
		},
		{
			name: "debug level",
			cfg: &config.LoggingConfig{
				Level:  "debug",
				Format: "json",
			},
			serviceName: "test-service",
			wantLevel:   LevelDebug,
		},
		{
			name: "error level",
			cfg: &config.LoggingConfig{
				Level:  "error",
				Format: "json",
			},
			serviceName: "test-service",
			wantLevel:   LevelError,
		},
		{
			name: "invalid level defaults to info",
			cfg: &config.LoggingConfig{
				Level:  "invalid",
				Format: "json",
			},
			serviceName: "test-service",
			wantLevel:   LevelInfo,
		},
		{
			name: "logfmt format",
			cfg: &config.LoggingConfig{
				Level:  "info",
				Format: "logfmt",
			},
			serviceName: "test-service",
			wantLevel:   LevelInfo,
		},
		{
			name: "text format",
			cfg: &config.LoggingConfig{
				Level:  "info",
				Format: "text",
			},
			serviceName: "test-service",
			wantLevel:   LevelInfo,
		},
		{
			name: "empty format defaults to json",
			cfg: &config.LoggingConfig{
				Level:  "info",
				Format: "",
			},
			serviceName: "test-service",
			wantLevel:   LevelInfo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewLoggerFromConfig(tt.cfg, tt.serviceName)
			if logger == nil {
				t.Fatal("expected non-nil logger")
			}
			if logger.GetLevel() != tt.wantLevel {
				t.Errorf("got level %v, want %v", logger.GetLevel(), tt.wantLevel)
			}
			if logger.config.Name != tt.serviceName {
				t.Errorf("got service name %s, want %s", logger.config.Name, tt.serviceName)
			}
		})
	}
}

func TestNewLoggerFromConfigWithMetadata(t *testing.T) {
	cfg := &config.LoggingConfig{
		Level:         "info",
		Format:        "json",
		IncludeCaller: true,
	}

	logger := NewLoggerFromConfig(cfg, "test-service")

	// Check metadata was set
	if logger.config.Metadata == nil {
		t.Fatal("expected metadata to be set")
	}

	// Host should be set
	if logger.config.Metadata.Host == "" {
		t.Error("expected host to be set")
	}

	// PID should be set
	if logger.config.Metadata.PID == 0 {
		t.Error("expected PID to be set")
	}

	// Service should match
	if logger.config.Metadata.Service != "test-service" {
		t.Errorf("got service %s, want test-service", logger.config.Metadata.Service)
	}

	// IncludeCaller should be respected
	if !logger.config.IncludeCaller {
		t.Error("expected IncludeCaller to be true")
	}
}

func TestNewServiceLogger(t *testing.T) {
	// Clear any environment variables for consistent testing
	logger := NewServiceLogger("my-service")

	if logger == nil {
		t.Fatal("expected non-nil logger")
	}

	// Default level should be info
	if logger.GetLevel() != LevelInfo {
		t.Errorf("got level %v, want %v", logger.GetLevel(), LevelInfo)
	}

	// Service name should be set
	if logger.config.Name != "my-service" {
		t.Errorf("got service name %s, want my-service", logger.config.Name)
	}
}

func TestInitDefaultLogger(t *testing.T) {
	// Save original default logger
	original := Default()
	defer SetDefault(original)

	logger := InitDefaultLogger("init-test")

	if logger == nil {
		t.Fatal("expected non-nil logger")
	}

	// Check default was set
	if Default() != logger {
		t.Error("expected default logger to be set")
	}

	if logger.config.Name != "init-test" {
		t.Errorf("got service name %s, want init-test", logger.config.Name)
	}
}

func TestInitDefaultLoggerFromConfig(t *testing.T) {
	// Save original default logger
	original := Default()
	defer SetDefault(original)

	cfg := &config.LoggingConfig{
		Level:  "debug",
		Format: "json",
	}

	logger := InitDefaultLoggerFromConfig(cfg, "config-test")

	if logger == nil {
		t.Fatal("expected non-nil logger")
	}

	// Check default was set
	if Default() != logger {
		t.Error("expected default logger to be set")
	}

	// Check level was applied
	if logger.GetLevel() != LevelDebug {
		t.Errorf("got level %v, want %v", logger.GetLevel(), LevelDebug)
	}
}

func TestNewLoggerFromConfigFormatterSelection(t *testing.T) {
	tests := []struct {
		format       string
		wantTypeName string
	}{
		{"json", "*logging.JSONFormatter"},
		{"logfmt", "*logging.LogfmtFormatter"},
		{"text", "*logging.TextFormatter"},
		{"unknown", "*logging.JSONFormatter"}, // defaults to JSON
		{"", "*logging.JSONFormatter"},        // defaults to JSON
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			cfg := &config.LoggingConfig{
				Level:  "info",
				Format: tt.format,
			}

			logger := NewLoggerFromConfig(cfg, "test")

			// Get formatter type name
			typeName := getFormatterTypeName(logger.config.Formatter)
			if typeName != tt.wantTypeName {
				t.Errorf("got formatter %s, want %s", typeName, tt.wantTypeName)
			}
		})
	}
}

func getFormatterTypeName(f Formatter) string {
	switch f.(type) {
	case *JSONFormatter:
		return "*logging.JSONFormatter"
	case *LogfmtFormatter:
		return "*logging.LogfmtFormatter"
	case *TextFormatter:
		return "*logging.TextFormatter"
	default:
		return "unknown"
	}
}
