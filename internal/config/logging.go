package config

import "fmt"

// LoggingConfig configures the structured logger.
type LoggingConfig struct {
	Level  string `koanf:"level"`
	Format string `koanf:"format"`
}

var (
	validLogLevels  = map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	validLogFormats = map[string]bool{"json": true, "logfmt": true, "text": true}
)

// Validate returns an error if Level or Format is not a recognized value.
func (l LoggingConfig) Validate() error {
	if !validLogLevels[l.Level] {
		return fmt.Errorf("level: %q (must be debug, info, warn, or error)", l.Level)
	}
	if !validLogFormats[l.Format] {
		return fmt.Errorf("format: %q (must be json, logfmt, or text)", l.Format)
	}
	return nil
}
