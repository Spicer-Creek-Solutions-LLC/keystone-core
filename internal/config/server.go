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
//
// Three modes:
//
//   - Enabled=false: plain TCP. Insecure; only appropriate for dev
//     loopback or behind a TLS-terminating ingress.
//   - Enabled=true + CertFile/KeyFile both set: file-sourced cert/key.
//     Conventional production setup with externally-provisioned PKI.
//   - Enabled=true + CertFile/KeyFile both empty: identity-provider
//     sourced. The embedded provider (Epic 09) issues a server SVID
//     for `spiffe://<td>/server/control-plane` and refreshes it on
//     each signing-CA rotation. Requires Identity.Enabled=true.
//
// Both modes enforce TLS 1.3 minimum + mTLS request semantics
// (ClientAuth = tls.VerifyClientCertIfGiven so API-key clients can
// still authenticate over TLS without a peer cert).
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

// Validate returns an error if TLS is enabled with a partial
// cert/key pair (exactly one of CertFile/KeyFile set). Both-set
// (file-sourced) and both-empty (identity-provider sourced) are
// both valid configurations.
func (t TLSConfig) Validate() error {
	if !t.Enabled {
		return nil
	}
	if (t.CertFile == "") != (t.KeyFile == "") {
		return fmt.Errorf("certfile/keyfile: both must be set OR both empty (identity-provider mode)")
	}
	return nil
}

// SourcedFromFiles reports whether the operator configured an
// explicit cert/key file pair. False means "derive from identity
// provider."
func (t TLSConfig) SourcedFromFiles() bool {
	return t.Enabled && t.CertFile != "" && t.KeyFile != ""
}
