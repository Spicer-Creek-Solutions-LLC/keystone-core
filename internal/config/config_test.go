package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeYAML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestLoad_DefaultsWhenNoPath(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\"): %v", err)
	}
	if cfg.Mode != ModeDevelopment {
		t.Errorf("Mode = %q, want development", cfg.Mode)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Server.Host = %q", cfg.Server.Host)
	}
	if cfg.Server.GRPCPort != 5397 {
		t.Errorf("Server.GRPCPort = %d, want 5397", cfg.Server.GRPCPort)
	}
	if cfg.Server.HTTPPort != 8080 {
		t.Errorf("Server.HTTPPort = %d, want 8080", cfg.Server.HTTPPort)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("Logging.Level = %q, want info", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("Logging.Format = %q, want json", cfg.Logging.Format)
	}
	if cfg.Storage.Driver != "sqlite" {
		t.Errorf("Storage.Driver = %q, want sqlite", cfg.Storage.Driver)
	}
	if cfg.Storage.DSN != "./data/keystone.db" {
		t.Errorf("Storage.DSN = %q", cfg.Storage.DSN)
	}
}

func TestLoad_FromYAML(t *testing.T) {
	path := writeYAML(t, `
mode: production
server:
  host: 127.0.0.1
  grpcport: 19090
  httpport: 18080
  tls:
    enabled: true
    certfile: /etc/cert.pem
    keyfile: /etc/key.pem
logging:
  level: debug
  format: text
storage:
  driver: postgres
  dsn: postgres://localhost/keystone
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Mode != ModeProduction {
		t.Errorf("Mode = %q", cfg.Mode)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Server.Host = %q", cfg.Server.Host)
	}
	if cfg.Server.GRPCPort != 19090 {
		t.Errorf("Server.GRPCPort = %d", cfg.Server.GRPCPort)
	}
	if cfg.Server.HTTPPort != 18080 {
		t.Errorf("Server.HTTPPort = %d", cfg.Server.HTTPPort)
	}
	if !cfg.Server.TLS.Enabled {
		t.Error("TLS.Enabled = false")
	}
	if cfg.Server.TLS.CertFile != "/etc/cert.pem" {
		t.Errorf("CertFile = %q", cfg.Server.TLS.CertFile)
	}
	if cfg.Server.TLS.KeyFile != "/etc/key.pem" {
		t.Errorf("KeyFile = %q", cfg.Server.TLS.KeyFile)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Logging.Level = %q", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "text" {
		t.Errorf("Logging.Format = %q", cfg.Logging.Format)
	}
	if cfg.Storage.Driver != "postgres" {
		t.Errorf("Storage.Driver = %q", cfg.Storage.Driver)
	}
	if cfg.Storage.DSN != "postgres://localhost/keystone" {
		t.Errorf("Storage.DSN = %q", cfg.Storage.DSN)
	}
}

func TestLoad_EnvOverridesYAML(t *testing.T) {
	path := writeYAML(t, `
mode: development
server:
  grpcport: 9090
`)
	t.Setenv("KSCORE_SERVER_GRPCPORT", "11111")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.GRPCPort != 11111 {
		t.Errorf("GRPCPort = %d, want 11111 (env should override YAML)", cfg.Server.GRPCPort)
	}
}

func TestLoad_EnvOverridesDefault(t *testing.T) {
	t.Setenv("KSCORE_LOGGING_LEVEL", "debug")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Logging.Level = %q, want debug (env should override default)", cfg.Logging.Level)
	}
}

func TestLoad_BadPath(t *testing.T) {
	if _, err := Load("/nonexistent-config-file-7421/x.yaml"); err == nil {
		t.Error("Load with bad path: expected error, got nil")
	}
}

func TestLoad_BadYAML(t *testing.T) {
	path := writeYAML(t, "this is: : not: valid: yaml::")
	if _, err := Load(path); err == nil {
		t.Error("Load with bad YAML: expected error, got nil")
	}
}

func TestLoad_FailsValidation(t *testing.T) {
	path := writeYAML(t, `
mode: development
server:
  grpcport: 99999
`)
	if _, err := Load(path); err == nil {
		t.Error("Load with out-of-range port: expected error, got nil")
	}
}

func TestMode_Validate(t *testing.T) {
	if err := ModeDevelopment.Validate(); err != nil {
		t.Errorf("development: %v", err)
	}
	if err := ModeProduction.Validate(); err != nil {
		t.Errorf("production: %v", err)
	}
	if err := Mode("staging").Validate(); err == nil {
		t.Error("staging should be rejected")
	}
}

func TestProductionWarnings_DevModeNone(t *testing.T) {
	cfg := defaultConfig()
	cfg.Mode = ModeDevelopment
	if w := cfg.ProductionWarnings(); len(w) != 0 {
		t.Errorf("warnings in dev = %v, want none", w)
	}
}

// safeCORS narrows the default-config wildcard origins so a single
// production-warning test only asserts on its own subject. CORS-
// specific cases use the wildcard explicitly.
func safeCORS() CORSConfig {
	return CORSConfig{
		Enabled:        true,
		AllowedOrigins: []string{"https://app.example.com"},
		AllowedMethods: []string{"GET", "POST"},
		AllowedHeaders: []string{"Authorization"},
	}
}

func TestProductionWarnings_TLSDisabled(t *testing.T) {
	cfg := defaultConfig()
	cfg.Mode = ModeProduction
	cfg.Storage.Driver = "postgres"
	cfg.Server.TLS.Enabled = false
	cfg.Server.CORS = safeCORS()
	cfg.Security.HMACSecret = "test-prod-hmac-key"
	w := warningsExcludingHMACStatic(cfg)
	if len(w) != 1 {
		t.Fatalf("warnings = %v, want 1 (TLS disabled)", w)
	}
	if w[0] != "TLS is disabled in production" {
		t.Errorf("warning = %q", w[0])
	}
}

func TestProductionWarnings_SQLiteInProd(t *testing.T) {
	cfg := defaultConfig()
	cfg.Mode = ModeProduction
	cfg.Server.TLS.Enabled = true
	cfg.Server.TLS.CertFile = "/c"
	cfg.Server.TLS.KeyFile = "/k"
	cfg.Storage.Driver = "sqlite"
	cfg.Server.CORS = safeCORS()
	cfg.Security.HMACSecret = "test-prod-hmac-key"
	w := warningsExcludingHMACStatic(cfg)
	if len(w) != 1 {
		t.Fatalf("warnings = %v, want 1 (SQLite)", w)
	}
}

func TestProductionWarnings_CORSWildcardInProd(t *testing.T) {
	cfg := defaultConfig()
	cfg.Mode = ModeProduction
	cfg.Server.TLS.Enabled = true
	cfg.Server.TLS.CertFile = "/c"
	cfg.Server.TLS.KeyFile = "/k"
	cfg.Storage.Driver = "postgres"
	cfg.Storage.DSN = "postgres://x"
	cfg.Security.HMACSecret = "test-prod-hmac-key"
	// CORS=* is the default — leave it.

	w := warningsExcludingHMACStatic(cfg)
	if len(w) != 1 {
		t.Fatalf("warnings = %v, want 1 (CORS *)", w)
	}
	if w[0] != "CORS allows all origins (*) in production" {
		t.Errorf("warning = %q", w[0])
	}
}

func TestProductionWarnings_CORSExplicitOriginsOK(t *testing.T) {
	cfg := defaultConfig()
	cfg.Mode = ModeProduction
	cfg.Server.TLS.Enabled = true
	cfg.Server.TLS.CertFile = "/c"
	cfg.Server.TLS.KeyFile = "/k"
	cfg.Storage.Driver = "postgres"
	cfg.Storage.DSN = "postgres://x"
	cfg.Server.CORS = safeCORS()
	cfg.Security.HMACSecret = "test-prod-hmac-key"

	if w := warningsExcludingHMACStatic(cfg); len(w) != 0 {
		t.Errorf("warnings = %v, want none", w)
	}
}

func TestProductionWarnings_CORSDisabledOK(t *testing.T) {
	cfg := defaultConfig()
	cfg.Mode = ModeProduction
	cfg.Server.TLS.Enabled = true
	cfg.Server.TLS.CertFile = "/c"
	cfg.Server.TLS.KeyFile = "/k"
	cfg.Storage.Driver = "postgres"
	cfg.Storage.DSN = "postgres://x"
	cfg.Server.CORS = CORSConfig{Enabled: false, AllowedOrigins: []string{"*"}}
	cfg.Security.HMACSecret = "test-prod-hmac-key"

	if w := warningsExcludingHMACStatic(cfg); len(w) != 0 {
		t.Errorf("warnings = %v, want none (CORS disabled)", w)
	}
}

// warningsExcludingHMACStatic returns ProductionWarnings filtered to
// drop the "security.hmacsecret is set to a static value" line.
// Every test in this file sets HMACSecret to a non-empty value to
// avoid the more-severe C1 empty-secret warning, which produces the
// static-value notice as a side-effect; tests that care about other
// warnings filter the static notice out.
func warningsExcludingHMACStatic(cfg *Config) []string {
	var out []string
	for _, w := range cfg.ProductionWarnings() {
		if strings.Contains(w, "security.hmacsecret is set to a static value") {
			continue
		}
		out = append(out, w)
	}
	return out
}

func TestProductionWarnings_AllSafe(t *testing.T) {
	cfg := defaultConfig()
	cfg.Mode = ModeProduction
	cfg.Server.TLS.Enabled = true
	cfg.Server.TLS.CertFile = "/c"
	cfg.Server.TLS.KeyFile = "/k"
	cfg.Storage.Driver = "postgres"
	cfg.Storage.DSN = "postgres://x"
	cfg.Server.CORS = safeCORS()
	cfg.Security.HMACSecret = "test-prod-hmac-key"
	if w := warningsExcludingHMACStatic(cfg); len(w) != 0 {
		t.Errorf("warnings = %v, want none (besides documented static-HMAC notice) in fully safe prod", w)
	}
}

// Epic 19 task 8 hardening pass — verify the three new dev-mode
// knob warnings fire in production.

func TestProductionWarnings_HMACSecretSet(t *testing.T) {
	cfg := defaultConfig()
	cfg.Mode = ModeProduction
	cfg.Server.TLS.Enabled = true
	cfg.Server.TLS.CertFile = "/c"
	cfg.Server.TLS.KeyFile = "/k"
	cfg.Storage.Driver = "postgres"
	cfg.Storage.DSN = "postgres://x"
	cfg.Server.CORS = safeCORS()
	cfg.Security.HMACSecret = "deadbeef"
	w := cfg.ProductionWarnings()
	if !containsSubstr(w, "security.hmacsecret is set to a static value") {
		t.Errorf("want static-HMAC-secret warning, got %v", w)
	}
}

// Phase B5 finding C1: empty HMACSecret in production is more
// dangerous than a static one (agent command-signature verification
// silently disabled). ProductionWarnings() must call this out.
func TestProductionWarnings_HMACSecretEmpty(t *testing.T) {
	cfg := defaultConfig()
	cfg.Mode = ModeProduction
	cfg.Server.TLS.Enabled = true
	cfg.Server.TLS.CertFile = "/c"
	cfg.Server.TLS.KeyFile = "/k"
	cfg.Storage.Driver = "postgres"
	cfg.Storage.DSN = "postgres://x"
	cfg.Server.CORS = safeCORS()
	cfg.Security.HMACSecret = ""
	w := cfg.ProductionWarnings()
	if !containsSubstr(w, "EMPTY") {
		t.Errorf("want empty-HMAC-secret warning, got %v", w)
	}
}

func TestProductionWarnings_SecretsInlineMasterKey(t *testing.T) {
	cfg := defaultConfig()
	cfg.Mode = ModeProduction
	cfg.Server.TLS.Enabled = true
	cfg.Server.TLS.CertFile = "/c"
	cfg.Server.TLS.KeyFile = "/k"
	cfg.Storage.Driver = "postgres"
	cfg.Storage.DSN = "postgres://x"
	cfg.Server.CORS = safeCORS()
	cfg.Secrets.Enabled = true
	cfg.Secrets.Backends = []SecretsBackendConfig{{
		Name: "file", Type: SecretsBackendTypeFile,
		File: &SecretsFileBackendConfig{Path: "/var/lib/kscore/secrets", MasterKey: "inline:deadbeef"},
	}}
	w := cfg.ProductionWarnings()
	if !containsSubstr(w, "inline:") {
		t.Errorf("want inline master_key warning, got %v", w)
	}
}

func TestProductionWarnings_BootstrapPSKInProd(t *testing.T) {
	cfg := defaultConfig()
	cfg.Mode = ModeProduction
	cfg.Server.TLS.Enabled = true
	cfg.Server.TLS.CertFile = "/c"
	cfg.Server.TLS.KeyFile = "/k"
	cfg.Storage.Driver = "postgres"
	cfg.Storage.DSN = "postgres://x"
	cfg.Server.CORS = safeCORS()
	cfg.NATS.Bootstrap.Enabled = true
	w := cfg.ProductionWarnings()
	if !containsSubstr(w, "nats.bootstrap.enabled") {
		t.Errorf("want bootstrap-PSK warning, got %v", w)
	}
}
