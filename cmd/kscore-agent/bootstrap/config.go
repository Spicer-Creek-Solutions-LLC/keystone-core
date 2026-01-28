package bootstrap

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// BootstrapConfig represents bootstrap configuration serialized to YAML.
type BootstrapConfig struct {
	Mode                   string                            `yaml:"mode"`
	ClusterName            string                            `yaml:"cluster_name,omitempty"`
	NodeRole               string                            `yaml:"node_role,omitempty"`
	NodeName               string                            `yaml:"node_name,omitempty"`
	NodeLabels             map[string]string                 `yaml:"node_labels,omitempty"`
	Regions                []string                          `yaml:"regions,omitempty"`
	HAEnabled              bool                              `yaml:"ha_enabled,omitempty"`
	HAReplicas             int                               `yaml:"ha_replicas,omitempty"`
	ObservabilityBackend   string                            `yaml:"observability_backend,omitempty"`
	ObservabilityEndpoint  string                            `yaml:"observability_endpoint,omitempty"`
	IdentityProvider       string                            `yaml:"identity_provider,omitempty"`
	IdentityEndpoint       string                            `yaml:"identity_endpoint,omitempty"`
	Join                   string                            `yaml:"join_endpoint,omitempty"`
	JoinToken              string                            `yaml:"join_token,omitempty"`
	Storage                string                            `yaml:"storage_backend,omitempty"`
	NATSMode               string                            `yaml:"nats_mode,omitempty"`
	BindAddress            string                            `yaml:"bind_address,omitempty"`
	Advertise              string                            `yaml:"advertise_address,omitempty"`
	PostgresHost           string                            `yaml:"postgres_host,omitempty"`
	PostgresPort           int                               `yaml:"postgres_port,omitempty"`
	PostgresDatabase       string                            `yaml:"postgres_database,omitempty"`
	PostgresUser           string                            `yaml:"postgres_user,omitempty"`
	PostgresPassword       string                            `yaml:"postgres_password,omitempty"`
	PostgresSSLMode        string                            `yaml:"postgres_ssl_mode,omitempty"`
	NATSURLs               []string                          `yaml:"nats_urls,omitempty"`
	GenerateCerts          bool                              `yaml:"generate_certs,omitempty"`
	TLSCertFile            string                            `yaml:"tls_cert_file,omitempty"`
	TLSKeyFile             string                            `yaml:"tls_key_file,omitempty"`
	TLSCAFile              string                            `yaml:"tls_ca_file,omitempty"`
	TLSCSRFile             string                            `yaml:"tls_csr_file,omitempty"`
	TLSRenewalCommand      string                            `yaml:"tls_renewal_command,omitempty"`
	TLSRenewalScriptPath   string                            `yaml:"tls_renewal_script_path,omitempty"`
	NATSCredsFile          string                            `yaml:"nats_creds_file,omitempty"`
	NATSUser               string                            `yaml:"nats_user,omitempty"`
	NATSPassword           string                            `yaml:"nats_password,omitempty"`
	PackageChannel         string                            `yaml:"package_channel,omitempty"`
	PackageVersion         string                            `yaml:"package_version,omitempty"`
	MigrateFromSQLite      string                            `yaml:"migrate_from_sqlite,omitempty"`
	MigrateBatchSize       int                               `yaml:"migrate_batch_size,omitempty"`
	MigrateContinueOnError bool                              `yaml:"migrate_continue_on_error,omitempty"`
	MigrateSkipExisting    bool                              `yaml:"migrate_skip_existing,omitempty"`
	SkipRepoSetup          bool                              `yaml:"skip_repo_setup,omitempty"`
	BlueprintsDir          string                            `yaml:"blueprints_dir,omitempty"`
	ApplyBlueprints        []string                          `yaml:"apply_blueprints,omitempty"`
	BlueprintParams        map[string]map[string]interface{} `yaml:"blueprint_params,omitempty"`
	BlueprintFeatures      map[string]map[string]bool        `yaml:"blueprint_features,omitempty"`
	BlueprintEntrypoints   map[string]string                 `yaml:"blueprint_entrypoints,omitempty"`
	ExportStatesDir        string                            `yaml:"export_states_dir,omitempty"`
}

// LoadBootstrapConfig reads a bootstrap configuration from disk.
func LoadBootstrapConfig(path string) (*BootstrapConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read bootstrap config: %w", err)
	}

	var cfg BootstrapConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse bootstrap config: %w", err)
	}

	return &cfg, nil
}

// WriteBootstrapConfig writes a bootstrap configuration to disk.
func WriteBootstrapConfig(path string, cfg *BootstrapConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal bootstrap config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write bootstrap config: %w", err)
	}

	return nil
}
