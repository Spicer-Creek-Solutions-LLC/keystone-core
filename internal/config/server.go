package config

import "fmt"

// ServerConfig configures the HTTP and gRPC listeners.
type ServerConfig struct {
	Host     string     `koanf:"host"`
	GRPCPort int        `koanf:"grpcport"`
	HTTPPort int        `koanf:"httpport"`
	TLS      TLSConfig  `koanf:"tls"`
	CORS     CORSConfig `koanf:"cors"`
}

// TLSConfig configures TLS termination for the listeners.
type TLSConfig struct {
	Enabled  bool   `koanf:"enabled"`
	CertFile string `koanf:"certfile"`
	KeyFile  string `koanf:"keyfile"`
}

// CORSConfig configures the HTTP CORS middleware. CORS lives outside
// the auth chain so OPTIONS preflight returns 204 before rate-limit
// is consulted (PROJECT-DETAILS §4.4 acceptance criterion).
type CORSConfig struct {
	Enabled        bool     `koanf:"enabled"`
	AllowedOrigins []string `koanf:"allowedorigins"`
	AllowedMethods []string `koanf:"allowedmethods"`
	AllowedHeaders []string `koanf:"allowedheaders"`
}

// Validate returns an error if any server-level field is invalid.
func (s ServerConfig) Validate() error {
	if s.Host == "" {
		return fmt.Errorf("host: must not be empty")
	}
	if s.GRPCPort < 1 || s.GRPCPort > 65535 {
		return fmt.Errorf("grpcport: %d out of range [1,65535]", s.GRPCPort)
	}
	if s.HTTPPort < 1 || s.HTTPPort > 65535 {
		return fmt.Errorf("httpport: %d out of range [1,65535]", s.HTTPPort)
	}
	if err := s.TLS.Validate(); err != nil {
		return fmt.Errorf("tls: %w", err)
	}
	if err := s.CORS.Validate(); err != nil {
		return fmt.Errorf("cors: %w", err)
	}
	return nil
}

// Validate ensures CORSConfig is internally consistent. Disabled CORS
// is always valid; enabled CORS must declare at least one origin.
func (c CORSConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if len(c.AllowedOrigins) == 0 {
		return fmt.Errorf("allowedorigins: required when enabled")
	}
	return nil
}

// Validate returns an error if TLS is enabled but cert/key paths are unset.
func (t TLSConfig) Validate() error {
	if !t.Enabled {
		return nil
	}
	if t.CertFile == "" {
		return fmt.Errorf("certfile: required when tls is enabled")
	}
	if t.KeyFile == "" {
		return fmt.Errorf("keyfile: required when tls is enabled")
	}
	return nil
}
