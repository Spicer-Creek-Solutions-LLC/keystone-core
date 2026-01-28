package vm

import (
	"fmt"
	"os"
	"path/filepath"
)

const envVMConfig = "KSCORE_VM_CONFIG"

// LoadProvider loads configuration and returns the requested VM provider.
func LoadProvider(configPath string) (Provider, *Config, error) {
	if configPath == "" {
		configPath = os.Getenv(envVMConfig)
	}
	if configPath == "" {
		configPath = "test/bootstrap/vm/config.yaml"
	}

	// If path is not absolute, resolve relative to project root
	if !filepath.IsAbs(configPath) {
		root, err := findProjectRoot()
		if err != nil {
			return nil, nil, fmt.Errorf("find project root: %w", err)
		}
		configPath = filepath.Join(root, configPath)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		return nil, nil, err
	}

	switch cfg.VMProvider {
	case "", "ssh":
		return NewSSHProvider(&cfg.SSH), cfg, nil
	case "vagrant":
		return nil, cfg, fmt.Errorf("vagrant provider not implemented")
	case "cloud":
		return nil, cfg, fmt.Errorf("cloud provider not implemented")
	default:
		return nil, cfg, fmt.Errorf("unknown vm provider %q", cfg.VMProvider)
	}
}

// findProjectRoot walks up from cwd looking for go.mod to find project root.
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find project root (no go.mod found)")
		}
		dir = parent
	}
}
