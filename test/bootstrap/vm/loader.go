package vm

import (
	"fmt"
	"os"
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
