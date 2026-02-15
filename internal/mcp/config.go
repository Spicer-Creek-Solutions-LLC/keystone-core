package mcp

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// MCPConfig is the top-level configuration for the MCP server.
type MCPConfig struct {
	Server   ServerConfig   `yaml:"server"`
	Auth     AuthConfig     `yaml:"auth"`
	Features FeaturesConfig `yaml:"features"`
}

// ServerConfig holds connection settings for the upstream kscore-server.
type ServerConfig struct {
	Address    string `yaml:"address"`
	RESTBaseURL string `yaml:"rest_base_url"`
}

// AuthConfig holds credentials for authenticating to kscore-server.
type AuthConfig struct {
	Method    string `yaml:"method"` // apikey, jwt, mtls
	APIKey    string `yaml:"api_key"`
	JWTToken  string `yaml:"jwt_token"`
	TLSCACert string `yaml:"tls_ca_cert"`
	TLSCert   string `yaml:"tls_cert"`
	TLSKey    string `yaml:"tls_key"`
}

// FeaturesConfig controls tool availability and safety limits.
type FeaturesConfig struct {
	MaxTargetCount int    `yaml:"max_target_count"`
	DefaultProfile string `yaml:"default_profile"`
}

// LoadConfig reads and validates an MCPConfig from a YAML file.
func LoadConfig(path string) (*MCPConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg MCPConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &cfg, nil
}

// Validate checks that required fields are set and values are sane.
func (c *MCPConfig) Validate() error {
	if c.Server.Address == "" {
		return fmt.Errorf("server.address is required")
	}
	switch c.Auth.Method {
	case "apikey":
		if c.Auth.APIKey == "" {
			return fmt.Errorf("auth.api_key is required when method=apikey")
		}
	case "jwt":
		if c.Auth.JWTToken == "" {
			return fmt.Errorf("auth.jwt_token is required when method=jwt")
		}
	case "mtls":
		if c.Auth.TLSCert == "" || c.Auth.TLSKey == "" {
			return fmt.Errorf("auth.tls_cert and auth.tls_key are required when method=mtls")
		}
	case "":
		return fmt.Errorf("auth.method is required (apikey, jwt, or mtls)")
	default:
		return fmt.Errorf("unknown auth.method %q: must be apikey, jwt, or mtls", c.Auth.Method)
	}

	if c.Features.DefaultProfile != "" {
		if _, err := ParseProfile(c.Features.DefaultProfile); err != nil {
			return err
		}
	}

	if c.Features.MaxTargetCount < 0 {
		return fmt.Errorf("features.max_target_count must be non-negative")
	}

	return nil
}

// Profile returns the parsed capability profile, defaulting to ops_safe.
func (c *MCPConfig) Profile() Profile {
	if c.Features.DefaultProfile == "" {
		return ProfileOpsSafe
	}
	p, _ := ParseProfile(c.Features.DefaultProfile)
	return p
}

// BuildTLSConfig constructs a TLS config from the auth settings.
func (c *MCPConfig) BuildTLSConfig() (*tls.Config, error) {
	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS13,
	}

	if c.Auth.TLSCACert != "" {
		caCert, err := os.ReadFile(c.Auth.TLSCACert)
		if err != nil {
			return nil, fmt.Errorf("reading CA cert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA cert")
		}
		tlsCfg.RootCAs = pool
	}

	if c.Auth.TLSCert != "" && c.Auth.TLSKey != "" {
		cert, err := tls.LoadX509KeyPair(c.Auth.TLSCert, c.Auth.TLSKey)
		if err != nil {
			return nil, fmt.Errorf("loading client cert: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	return tlsCfg, nil
}
