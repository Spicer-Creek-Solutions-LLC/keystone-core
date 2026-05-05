package config

import "fmt"

// StorageConfig configures the persistence backend.
type StorageConfig struct {
	Driver string `koanf:"driver"`
	DSN    string `koanf:"dsn"`
}

var validStorageDrivers = map[string]bool{"sqlite": true, "postgres": true}

// Validate returns an error if Driver is not recognized or DSN is empty.
func (s StorageConfig) Validate() error {
	if !validStorageDrivers[s.Driver] {
		return fmt.Errorf("driver: %q (must be sqlite or postgres)", s.Driver)
	}
	if s.DSN == "" {
		return fmt.Errorf("dsn: must not be empty")
	}
	return nil
}
