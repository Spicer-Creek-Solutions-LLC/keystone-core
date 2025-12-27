package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the TUI monitor configuration
type Config struct {
	// Control plane connection
	ControlPlane string `yaml:"control_plane"`
	NATSURL      string `yaml:"nats_url"`

	// UI settings
	Theme           string `yaml:"theme"`
	RefreshInterval int    `yaml:"refresh_interval"` // seconds
	NoColor         bool   `yaml:"no_color"`

	// View settings
	EventBufferSize int  `yaml:"event_buffer_size"`
	LogBufferSize   int  `yaml:"log_buffer_size"`
	AutoScroll      bool `yaml:"auto_scroll"`

	// Filters (persistent across sessions)
	DefaultFilters map[string]string `yaml:"default_filters"`

	// Performance
	MaxFPS          int           `yaml:"max_fps"`
	DebounceDelay   time.Duration `yaml:"debounce_delay"`
	BackgroundFetch bool          `yaml:"background_fetch"`
}

// Default returns the default configuration
func Default() *Config {
	return &Config{
		ControlPlane:    "localhost:50051",
		NATSURL:         "nats://localhost:4222",
		Theme:           "dark",
		RefreshInterval: 2,
		NoColor:         false,
		EventBufferSize: 1000,
		LogBufferSize:   1000,
		AutoScroll:      true,
		DefaultFilters:  make(map[string]string),
		MaxFPS:          60,
		DebounceDelay:   100 * time.Millisecond,
		BackgroundFetch: true,
	}
}

// Load loads configuration from a file
func Load(path string) (*Config, error) {
	if path == "" {
		// Try default location
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(home, ".kscore", "monitor.yaml")
	}

	// If file doesn't exist, return default config
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return Default(), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return cfg, nil
}

// Save saves the configuration to a file
func (c *Config) Save(path string) error {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(home, ".kscore", "monitor.yaml")
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
