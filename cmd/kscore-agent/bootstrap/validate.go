package bootstrap

import (
	"context"
	"fmt"
	"strings"
)

var validNodeRoles = map[string]bool{
	"control-plane": true,
	"agent":         true,
	"both":          true,
}

var validStorageBackends = map[string]bool{
	"sqlite":   true,
	"postgres": true,
}

var validNATSMode = map[string]bool{
	"embedded": true,
	"cluster":  true,
	"external": true,
	"leaf":     true,
}

var validPostgresSSLModes = map[string]bool{
	"disable":     true,
	"allow":       true,
	"prefer":      true,
	"require":     true,
	"verify-ca":   true,
	"verify-full": true,
}

func validatePhase(ctx context.Context, state *State) error {
	if state.BootstrapConfig == nil {
		return nil
	}
	if err := validateBootstrapConfig(state.BootstrapConfig); err != nil {
		return err
	}
	if state.Verbose || state.DryRun {
		fmt.Fprintln(state.Output, "bootstrap configuration validated")
	}
	return nil
}

func validateBootstrapConfig(cfg *BootstrapConfig) error {
	if cfg.Mode == "" {
		return fmt.Errorf("mode is required")
	}

	if cfg.ClusterName == "" {
		return fmt.Errorf("cluster name is required")
	}

	if cfg.NodeRole != "" && !isValidNodeRole(cfg.NodeRole) {
		return fmt.Errorf("invalid node role %q", cfg.NodeRole)
	}
	if err := validateNodeLabels(cfg.NodeLabels); err != nil {
		return err
	}

	if cfg.Join != "" && cfg.JoinToken == "" {
		return fmt.Errorf("join token is required when join endpoint is set")
	}

	if cfg.Storage != "" && !validStorageBackends[strings.ToLower(cfg.Storage)] {
		return fmt.Errorf("invalid storage backend %q", cfg.Storage)
	}

	if cfg.NATSMode != "" && !validNATSMode[strings.ToLower(cfg.NATSMode)] {
		return fmt.Errorf("invalid NATS mode %q", cfg.NATSMode)
	}

	// Storage validation only applies to control-plane or both roles.
	// Agent-only nodes don't run a database - they connect to the control plane via NATS.
	needsStorage := strings.EqualFold(cfg.NodeRole, "control-plane") || strings.EqualFold(cfg.NodeRole, "both")
	if needsStorage && strings.EqualFold(cfg.Storage, "postgres") {
		if cfg.PostgresHost == "" || cfg.PostgresDatabase == "" || cfg.PostgresUser == "" {
			return fmt.Errorf("postgres host, database, and user are required")
		}
		if cfg.PostgresPort == 0 {
			return fmt.Errorf("postgres port is required")
		}
		if cfg.PostgresSSLMode != "" && !validPostgresSSLModes[strings.ToLower(cfg.PostgresSSLMode)] {
			return fmt.Errorf("invalid postgres ssl mode %q", cfg.PostgresSSLMode)
		}
	}

	if (strings.EqualFold(cfg.NATSMode, "external") || strings.EqualFold(cfg.NATSMode, "leaf")) && len(cfg.NATSURLs) == 0 {
		return fmt.Errorf("nats urls are required for external or leaf mode")
	}

	if cfg.NATSCredsFile != "" && (cfg.NATSUser != "" || cfg.NATSPassword != "") {
		return fmt.Errorf("nats creds file cannot be combined with nats user/password")
	}

	if (cfg.NATSUser != "") != (cfg.NATSPassword != "") {
		return fmt.Errorf("nats user and password must both be provided")
	}

	if len(cfg.ApplyBlueprints) > 0 && cfg.BlueprintsDir == "" {
		return fmt.Errorf("blueprints dir is required when apply-blueprint is set")
	}
	if (len(cfg.BlueprintParams) > 0 || len(cfg.BlueprintFeatures) > 0 || len(cfg.BlueprintEntrypoints) > 0) && cfg.BlueprintsDir == "" {
		return fmt.Errorf("blueprints dir is required when blueprint overrides are set")
	}
	if cfg.ExportStatesDir != "" && len(cfg.ApplyBlueprints) == 0 {
		return fmt.Errorf("apply-blueprint is required when export states dir is set")
	}

	if !cfg.GenerateCerts {
		if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
			return fmt.Errorf("tls cert and key must both be provided")
		}
	}
	if cfg.GenerateCerts && cfg.TLSCSRFile != "" {
		return fmt.Errorf("tls csr file cannot be set when generating certificates")
	}
	if cfg.TLSCSRFile != "" && cfg.TLSRenewalCommand == "" && cfg.TLSRenewalScriptPath != "" {
		return fmt.Errorf("tls renewal script path requires renewal command")
	}
	if cfg.MigrateFromSQLite != "" && !strings.EqualFold(cfg.Storage, "postgres") {
		return fmt.Errorf("sqlite migration requires postgres storage backend")
	}

	return nil
}

func isValidNodeRole(role string) bool {
	return validNodeRoles[strings.ToLower(role)]
}

func validateNodeLabels(labels map[string]string) error {
	for key, value := range labels {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("node label key cannot be empty")
		}
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("node label %q has empty value", key)
		}
	}
	return nil
}
