package framework

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config defines bootstrap test configuration.
type Config struct {
	Test struct {
		Timeout     string `yaml:"timeout"`
		Parallel    int    `yaml:"parallel"`
		RetryFailed int    `yaml:"retry_failed"`
	} `yaml:"test"`
	Docker struct {
		Registry  string `yaml:"registry"`
		CacheFrom bool   `yaml:"cache_from"`
	} `yaml:"docker"`
	Platforms []Platform `yaml:"platforms"`
	Scenarios []Scenario `yaml:"scenarios"`
}

// Platform describes a platform target for bootstrap tests.
type Platform struct {
	Name    string            `yaml:"name"`
	Image   string            `yaml:"image"`
	Args    map[string]string `yaml:"args"`
	Enabled bool              `yaml:"enabled"`
}

// Scenario describes a bootstrap scenario.
type Scenario struct {
	Name    string `yaml:"name"`
	Enabled bool   `yaml:"enabled"`
	Timeout string `yaml:"timeout"`
	Nodes   int    `yaml:"nodes"`
}

// LoadConfig reads a bootstrap test config from disk.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// EnabledPlatforms returns enabled platforms, optionally filtered by name list.
func (c *Config) EnabledPlatforms(filter string) ([]Platform, error) {
	return filterPlatforms(c.Platforms, filter)
}

// EnabledScenarios returns enabled scenarios, optionally filtered by name list.
func (c *Config) EnabledScenarios(filter string) ([]Scenario, error) {
	return filterScenarios(c.Scenarios, filter)
}

func filterPlatforms(platforms []Platform, filter string) ([]Platform, error) {
	names := parseFilter(filter)
	if len(names) == 0 {
		var enabled []Platform
		for _, platform := range platforms {
			if platform.Enabled {
				enabled = append(enabled, platform)
			}
		}
		if len(enabled) == 0 {
			return nil, errors.New("no enabled platforms configured")
		}
		return enabled, nil
	}

	var selected []Platform
	for _, name := range names {
		found := false
		for _, platform := range platforms {
			if platform.Name != name {
				continue
			}
			found = true
			if !platform.Enabled {
				return nil, fmt.Errorf("platform %q is disabled", name)
			}
			selected = append(selected, platform)
		}
		if !found {
			return nil, fmt.Errorf("platform %q not found", name)
		}
	}
	return selected, nil
}

func filterScenarios(scenarios []Scenario, filter string) ([]Scenario, error) {
	names := parseFilter(filter)
	if len(names) == 0 {
		var enabled []Scenario
		for _, scenario := range scenarios {
			if scenario.Enabled {
				enabled = append(enabled, scenario)
			}
		}
		if len(enabled) == 0 {
			return nil, errors.New("no enabled scenarios configured")
		}
		return enabled, nil
	}

	var selected []Scenario
	for _, name := range names {
		found := false
		for _, scenario := range scenarios {
			if scenario.Name != name {
				continue
			}
			found = true
			if !scenario.Enabled {
				return nil, fmt.Errorf("scenario %q is disabled", name)
			}
			selected = append(selected, scenario)
		}
		if !found {
			return nil, fmt.Errorf("scenario %q not found", name)
		}
	}
	return selected, nil
}

func parseFilter(filter string) []string {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return nil
	}
	parts := strings.Split(filter, ",")
	var names []string
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	return names
}
