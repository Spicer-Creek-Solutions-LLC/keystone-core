// Package config provides configuration management for the TUI monitor.
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
	InitialView     int    `yaml:"initial_view"` // 1-13, 0 means default (dashboard)

	// View settings
	EventBufferSize  int            `yaml:"event_buffer_size"`
	LogBufferSize    int            `yaml:"log_buffer_size"`
	AutoScroll       bool           `yaml:"auto_scroll"`
	ViewRefreshRates map[string]int `yaml:"view_refresh_rates"` // per-view refresh in seconds; 0 = realtime (no polling)

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
		InitialView:     0,
		EventBufferSize: 1000,
		LogBufferSize:   1000,
		AutoScroll:      true,
		DefaultFilters: make(map[string]string),
		ViewRefreshRates: map[string]int{
			"dashboard": 2,
			"agents":    5,
			"events":    0,
			"drift":     10,
			"policy":    10,
			"jobs":      5,
			"logs":      0,
			"metrics":   5,
			"cluster":   10,
			"secrets":   15,
			"schedules": 15,
			"runbooks":  10,
			"webhooks":  15,
		},
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
	//nolint:gosec // G301: config directory needs to be accessible by admin users
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	//nolint:gosec // G306: config file needs to be readable by the user
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
