package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/shawnbutts/keystone-core/cmd/kscore-agent/bootstrap/tui"
	"github.com/spf13/cobra"
)

// Options holds bootstrap command options.
type Options struct {
	DryRun                  bool
	Verbose                 bool
	NonInteractive          bool
	SkipRepoSetup           bool
	Mode                    string
	ConfigPath              string
	OutputConfig            string
	ClusterName             string
	NodeRole                string
	NodeName                string
	NodeLabels              map[string]string
	NodeLabelArgs           []string
	JSONOutput              bool
	Regions                 []string
	HAEnabled               bool
	HAReplicas              int
	ObservabilityBackend    string
	ObservabilityEndpoint   string
	IdentityProvider        string
	IdentityEndpoint        string
	JoinEndpoint            string
	JoinToken               string
	StorageBackend          string
	NATSMode                string
	BindAddress             string
	AdvertiseAddr           string
	PostgresHost            string
	PostgresPort            int
	PostgresDatabase        string
	PostgresUser            string
	PostgresPassword        string
	PostgresSSLMode         string
	NATSURLs                []string
	GenerateCerts           bool
	TLSCertFile             string
	TLSKeyFile              string
	TLSCAFile               string
	TLSCSRFile              string
	TLSRenewalCommand       string
	TLSRenewalScriptPath    string
	NATSCredsFile           string
	NATSUser                string
	NATSPassword            string
	PackageChannel          string
	PackageVersion          string
	MigrateFromSQLite       string
	MigrateBatchSize        int
	MigrateContinueOnError  bool
	MigrateSkipExisting     bool
	BlueprintsDir           string
	ApplyBlueprints         []string
	BlueprintParams         map[string]map[string]interface{}
	BlueprintFeatures       map[string]map[string]bool
	BlueprintEntrypoints    map[string]string
	BlueprintParamArgs      []string
	BlueprintFeatureArgs    []string
	BlueprintEntrypointArgs []string
	ExportStatesDir         string
}

// NewCommand returns the kscore-agent bootstrap command.
func NewCommand() *cobra.Command {
	opts := &Options{}

	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Bootstrap a Keystone Core deployment",
		Long: `Bootstrap a Keystone Core deployment using a guided flow.

This command is the entry point for Epic 27's single-binary bootstrap.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := contextWithSignal(cmd.Context())
			defer cancel()

			if opts.ConfigPath != "" {
				loaded, err := LoadBootstrapConfig(opts.ConfigPath)
				if err != nil {
					return err
				}
				if opts.Mode == "" {
					opts.Mode = loaded.Mode
				}
				if opts.ClusterName == "" {
					opts.ClusterName = loaded.ClusterName
				}
				if opts.NodeRole == "" {
					opts.NodeRole = loaded.NodeRole
				}
				if opts.NodeName == "" {
					opts.NodeName = loaded.NodeName
				}
				if len(opts.NodeLabels) == 0 {
					opts.NodeLabels = loaded.NodeLabels
				}
				if len(opts.Regions) == 0 {
					opts.Regions = loaded.Regions
				}
				if !opts.HAEnabled {
					opts.HAEnabled = loaded.HAEnabled
				}
				if opts.HAReplicas == 0 && loaded.HAReplicas != 0 {
					opts.HAReplicas = loaded.HAReplicas
				}
				if opts.ObservabilityBackend == "" {
					opts.ObservabilityBackend = loaded.ObservabilityBackend
				}
				if opts.ObservabilityEndpoint == "" {
					opts.ObservabilityEndpoint = loaded.ObservabilityEndpoint
				}
				if opts.IdentityProvider == "" {
					opts.IdentityProvider = loaded.IdentityProvider
				}
				if opts.IdentityEndpoint == "" {
					opts.IdentityEndpoint = loaded.IdentityEndpoint
				}
				if opts.JoinEndpoint == "" {
					opts.JoinEndpoint = loaded.Join
				}
				if opts.JoinToken == "" {
					opts.JoinToken = loaded.JoinToken
				}
				if opts.StorageBackend == "" {
					opts.StorageBackend = loaded.Storage
				}
				if opts.NATSMode == "" {
					opts.NATSMode = loaded.NATSMode
				}
				if opts.BindAddress == "" {
					opts.BindAddress = loaded.BindAddress
				}
				if opts.AdvertiseAddr == "" {
					opts.AdvertiseAddr = loaded.Advertise
				}
				if opts.PostgresHost == "" {
					opts.PostgresHost = loaded.PostgresHost
				}
				if opts.PostgresPort == 0 && loaded.PostgresPort != 0 {
					opts.PostgresPort = loaded.PostgresPort
				}
				if opts.PostgresDatabase == "" {
					opts.PostgresDatabase = loaded.PostgresDatabase
				}
				if opts.PostgresUser == "" {
					opts.PostgresUser = loaded.PostgresUser
				}
				if opts.PostgresPassword == "" {
					opts.PostgresPassword = loaded.PostgresPassword
				}
				if opts.PostgresSSLMode == "" {
					opts.PostgresSSLMode = loaded.PostgresSSLMode
				}
				if len(opts.NATSURLs) == 0 {
					opts.NATSURLs = loaded.NATSURLs
				}
				if !opts.GenerateCerts {
					opts.GenerateCerts = loaded.GenerateCerts
				}
				if opts.TLSCertFile == "" {
					opts.TLSCertFile = loaded.TLSCertFile
				}
				if opts.TLSKeyFile == "" {
					opts.TLSKeyFile = loaded.TLSKeyFile
				}
				if opts.TLSCAFile == "" {
					opts.TLSCAFile = loaded.TLSCAFile
				}
				if opts.TLSCSRFile == "" {
					opts.TLSCSRFile = loaded.TLSCSRFile
				}
				if opts.TLSRenewalCommand == "" {
					opts.TLSRenewalCommand = loaded.TLSRenewalCommand
				}
				if opts.TLSRenewalScriptPath == "" {
					opts.TLSRenewalScriptPath = loaded.TLSRenewalScriptPath
				}
				if opts.NATSCredsFile == "" {
					opts.NATSCredsFile = loaded.NATSCredsFile
				}
				if opts.NATSUser == "" {
					opts.NATSUser = loaded.NATSUser
				}
				if opts.NATSPassword == "" {
					opts.NATSPassword = loaded.NATSPassword
				}
				if opts.PackageChannel == "" {
					opts.PackageChannel = loaded.PackageChannel
				}
				if opts.PackageVersion == "" {
					opts.PackageVersion = loaded.PackageVersion
				}
				if opts.MigrateFromSQLite == "" {
					opts.MigrateFromSQLite = loaded.MigrateFromSQLite
				}
				if opts.MigrateBatchSize == 0 && loaded.MigrateBatchSize != 0 {
					opts.MigrateBatchSize = loaded.MigrateBatchSize
				}
				if !opts.MigrateContinueOnError {
					opts.MigrateContinueOnError = loaded.MigrateContinueOnError
				}
				if !opts.MigrateSkipExisting {
					opts.MigrateSkipExisting = loaded.MigrateSkipExisting
				}
				if opts.BlueprintsDir == "" {
					opts.BlueprintsDir = loaded.BlueprintsDir
				}
				if len(opts.ApplyBlueprints) == 0 {
					opts.ApplyBlueprints = loaded.ApplyBlueprints
				}
				if len(opts.BlueprintParams) == 0 {
					opts.BlueprintParams = loaded.BlueprintParams
				}
				if len(opts.BlueprintFeatures) == 0 {
					opts.BlueprintFeatures = loaded.BlueprintFeatures
				}
				if len(opts.BlueprintEntrypoints) == 0 {
					opts.BlueprintEntrypoints = loaded.BlueprintEntrypoints
				}
				if opts.ExportStatesDir == "" {
					opts.ExportStatesDir = loaded.ExportStatesDir
				}
			}

			applyEnvOverrides(opts)
			if err := applyNodeLabelOverrides(opts); err != nil {
				return err
			}
			if err := applyBlueprintOverrides(opts); err != nil {
				return err
			}

			mode, err := ParseDeploymentMode(opts.Mode)
			if err != nil {
				return err
			}

			cfg := &BootstrapConfig{
				Mode:                   string(mode),
				ClusterName:            opts.ClusterName,
				NodeRole:               opts.NodeRole,
				NodeName:               opts.NodeName,
				NodeLabels:             opts.NodeLabels,
				Regions:                opts.Regions,
				HAEnabled:              opts.HAEnabled,
				HAReplicas:             opts.HAReplicas,
				ObservabilityBackend:   opts.ObservabilityBackend,
				ObservabilityEndpoint:  opts.ObservabilityEndpoint,
				IdentityProvider:       opts.IdentityProvider,
				IdentityEndpoint:       opts.IdentityEndpoint,
				Join:                   opts.JoinEndpoint,
				JoinToken:              opts.JoinToken,
				Storage:                opts.StorageBackend,
				NATSMode:               opts.NATSMode,
				BindAddress:            opts.BindAddress,
				Advertise:              opts.AdvertiseAddr,
				PostgresHost:           opts.PostgresHost,
				PostgresPort:           opts.PostgresPort,
				PostgresDatabase:       opts.PostgresDatabase,
				PostgresUser:           opts.PostgresUser,
				PostgresPassword:       opts.PostgresPassword,
				PostgresSSLMode:        opts.PostgresSSLMode,
				NATSURLs:               opts.NATSURLs,
				GenerateCerts:          opts.GenerateCerts,
				TLSCertFile:            opts.TLSCertFile,
				TLSKeyFile:             opts.TLSKeyFile,
				TLSCAFile:              opts.TLSCAFile,
				TLSCSRFile:             opts.TLSCSRFile,
				TLSRenewalCommand:      opts.TLSRenewalCommand,
				TLSRenewalScriptPath:   opts.TLSRenewalScriptPath,
				NATSCredsFile:          opts.NATSCredsFile,
				NATSUser:               opts.NATSUser,
				NATSPassword:           opts.NATSPassword,
				PackageChannel:         opts.PackageChannel,
				PackageVersion:         opts.PackageVersion,
				MigrateFromSQLite:      opts.MigrateFromSQLite,
				MigrateBatchSize:       opts.MigrateBatchSize,
				MigrateContinueOnError: opts.MigrateContinueOnError,
				MigrateSkipExisting:    opts.MigrateSkipExisting,
				SkipRepoSetup:          opts.SkipRepoSetup,
				BlueprintsDir:          opts.BlueprintsDir,
				ApplyBlueprints:        opts.ApplyBlueprints,
				BlueprintParams:        opts.BlueprintParams,
				BlueprintFeatures:      opts.BlueprintFeatures,
				BlueprintEntrypoints:   opts.BlueprintEntrypoints,
				ExportStatesDir:        opts.ExportStatesDir,
			}

			if !opts.NonInteractive {
				systemInfo, _ := DetectSystem()
				recommended := RecommendMode(systemInfo)
				hints := ModeHints(systemInfo)
				resourceSummary := ResourceSummary(systemInfo)
				selected, err := tui.RunWizard(tui.WizardConfig{
					Mode:            cfg.Mode,
					RecommendedMode: string(recommended),
					ResourceSummary: resourceSummary,
					ModeHints: map[string]string{
						string(DeploymentModeDemo):       hints[DeploymentModeDemo],
						string(DeploymentModeProduction): hints[DeploymentModeProduction],
						string(DeploymentModeFullScale):  hints[DeploymentModeFullScale],
					},
					ClusterName:             cfg.ClusterName,
					NodeRole:                cfg.NodeRole,
					NodeName:                cfg.NodeName,
					NodeLabels:              cfg.NodeLabels,
					NodeLabelArgs:           opts.NodeLabelArgs,
					Regions:                 cfg.Regions,
					HAEnabled:               cfg.HAEnabled,
					HAReplicas:              cfg.HAReplicas,
					ObservabilityBackend:    cfg.ObservabilityBackend,
					ObservabilityEndpoint:   cfg.ObservabilityEndpoint,
					IdentityProvider:        cfg.IdentityProvider,
					IdentityEndpoint:        cfg.IdentityEndpoint,
					Join:                    cfg.Join,
					JoinToken:               cfg.JoinToken,
					Storage:                 cfg.Storage,
					NATSMode:                cfg.NATSMode,
					BindAddress:             cfg.BindAddress,
					Advertise:               cfg.Advertise,
					PostgresHost:            cfg.PostgresHost,
					PostgresPort:            cfg.PostgresPort,
					PostgresDatabase:        cfg.PostgresDatabase,
					PostgresUser:            cfg.PostgresUser,
					PostgresPassword:        cfg.PostgresPassword,
					PostgresSSLMode:         cfg.PostgresSSLMode,
					NATSURLs:                cfg.NATSURLs,
					GenerateCerts:           cfg.GenerateCerts,
					TLSCertFile:             cfg.TLSCertFile,
					TLSKeyFile:              cfg.TLSKeyFile,
					TLSCAFile:               cfg.TLSCAFile,
					TLSCSRFile:              cfg.TLSCSRFile,
					TLSRenewalCommand:       cfg.TLSRenewalCommand,
					TLSRenewalScriptPath:    cfg.TLSRenewalScriptPath,
					NATSCredsFile:           cfg.NATSCredsFile,
					NATSUser:                cfg.NATSUser,
					NATSPassword:            cfg.NATSPassword,
					PackageChannel:          cfg.PackageChannel,
					PackageVersion:          cfg.PackageVersion,
					MigrateFromSQLite:       cfg.MigrateFromSQLite,
					MigrateBatchSize:        cfg.MigrateBatchSize,
					MigrateContinueOnError:  cfg.MigrateContinueOnError,
					MigrateSkipExisting:     cfg.MigrateSkipExisting,
					BlueprintsDir:           cfg.BlueprintsDir,
					ApplyBlueprints:         cfg.ApplyBlueprints,
					BlueprintParams:         cfg.BlueprintParams,
					BlueprintFeatures:       cfg.BlueprintFeatures,
					BlueprintEntrypoints:    cfg.BlueprintEntrypoints,
					BlueprintParamArgs:      opts.BlueprintParamArgs,
					BlueprintFeatureArgs:    opts.BlueprintFeatureArgs,
					BlueprintEntrypointArgs: opts.BlueprintEntrypointArgs,
					ExportStatesDir:         cfg.ExportStatesDir,
				})
				if err != nil {
					return err
				}
				cfg = &BootstrapConfig{
					Mode:                   selected.Mode,
					ClusterName:            selected.ClusterName,
					NodeRole:               selected.NodeRole,
					NodeName:               selected.NodeName,
					NodeLabels:             selected.NodeLabels,
					Regions:                selected.Regions,
					HAEnabled:              selected.HAEnabled,
					HAReplicas:             selected.HAReplicas,
					ObservabilityBackend:   selected.ObservabilityBackend,
					ObservabilityEndpoint:  selected.ObservabilityEndpoint,
					IdentityProvider:       selected.IdentityProvider,
					IdentityEndpoint:       selected.IdentityEndpoint,
					Join:                   selected.Join,
					JoinToken:              selected.JoinToken,
					Storage:                selected.Storage,
					NATSMode:               selected.NATSMode,
					BindAddress:            selected.BindAddress,
					Advertise:              selected.Advertise,
					PostgresHost:           selected.PostgresHost,
					PostgresPort:           selected.PostgresPort,
					PostgresDatabase:       selected.PostgresDatabase,
					PostgresUser:           selected.PostgresUser,
					PostgresPassword:       selected.PostgresPassword,
					PostgresSSLMode:        selected.PostgresSSLMode,
					NATSURLs:               selected.NATSURLs,
					GenerateCerts:          selected.GenerateCerts,
					TLSCertFile:            selected.TLSCertFile,
					TLSKeyFile:             selected.TLSKeyFile,
					TLSCAFile:              selected.TLSCAFile,
					TLSCSRFile:             selected.TLSCSRFile,
					TLSRenewalCommand:      selected.TLSRenewalCommand,
					TLSRenewalScriptPath:   selected.TLSRenewalScriptPath,
					NATSCredsFile:          selected.NATSCredsFile,
					NATSUser:               selected.NATSUser,
					NATSPassword:           selected.NATSPassword,
					PackageChannel:         selected.PackageChannel,
					PackageVersion:         selected.PackageVersion,
					MigrateFromSQLite:      selected.MigrateFromSQLite,
					MigrateBatchSize:       selected.MigrateBatchSize,
					MigrateContinueOnError: selected.MigrateContinueOnError,
					MigrateSkipExisting:    selected.MigrateSkipExisting,
					SkipRepoSetup:          opts.SkipRepoSetup,
					BlueprintsDir:          selected.BlueprintsDir,
					ApplyBlueprints:        selected.ApplyBlueprints,
					BlueprintParams:        selected.BlueprintParams,
					BlueprintFeatures:      selected.BlueprintFeatures,
					BlueprintEntrypoints:   selected.BlueprintEntrypoints,
					ExportStatesDir:        selected.ExportStatesDir,
				}
				opts.Mode = cfg.Mode
				opts.ClusterName = cfg.ClusterName
				opts.NodeRole = cfg.NodeRole
				opts.NodeName = cfg.NodeName
				opts.NodeLabels = cfg.NodeLabels
				opts.Regions = cfg.Regions
				opts.HAEnabled = cfg.HAEnabled
				opts.HAReplicas = cfg.HAReplicas
				opts.ObservabilityBackend = cfg.ObservabilityBackend
				opts.ObservabilityEndpoint = cfg.ObservabilityEndpoint
				opts.IdentityProvider = cfg.IdentityProvider
				opts.IdentityEndpoint = cfg.IdentityEndpoint
				opts.JoinEndpoint = cfg.Join
				opts.JoinToken = cfg.JoinToken
				opts.StorageBackend = cfg.Storage
				opts.NATSMode = cfg.NATSMode
				opts.BindAddress = cfg.BindAddress
				opts.AdvertiseAddr = cfg.Advertise
				opts.PostgresHost = cfg.PostgresHost
				opts.PostgresPort = cfg.PostgresPort
				opts.PostgresDatabase = cfg.PostgresDatabase
				opts.PostgresUser = cfg.PostgresUser
				opts.PostgresPassword = cfg.PostgresPassword
				opts.PostgresSSLMode = cfg.PostgresSSLMode
				opts.NATSURLs = cfg.NATSURLs
				opts.GenerateCerts = cfg.GenerateCerts
				opts.TLSCertFile = cfg.TLSCertFile
				opts.TLSKeyFile = cfg.TLSKeyFile
				opts.TLSCAFile = cfg.TLSCAFile
				opts.TLSCSRFile = cfg.TLSCSRFile
				opts.TLSRenewalCommand = cfg.TLSRenewalCommand
				opts.TLSRenewalScriptPath = cfg.TLSRenewalScriptPath
				opts.NATSCredsFile = cfg.NATSCredsFile
				opts.NATSUser = cfg.NATSUser
				opts.NATSPassword = cfg.NATSPassword
				opts.PackageChannel = cfg.PackageChannel
				opts.PackageVersion = cfg.PackageVersion
				opts.MigrateFromSQLite = cfg.MigrateFromSQLite
				opts.MigrateBatchSize = cfg.MigrateBatchSize
				opts.MigrateContinueOnError = cfg.MigrateContinueOnError
				opts.MigrateSkipExisting = cfg.MigrateSkipExisting
				opts.BlueprintsDir = cfg.BlueprintsDir
				opts.ApplyBlueprints = cfg.ApplyBlueprints
				opts.ExportStatesDir = cfg.ExportStatesDir
				if len(selected.NodeLabelArgs) > 0 {
					opts.NodeLabelArgs = selected.NodeLabelArgs
				}
				if err := applyNodeLabelOverrides(opts); err != nil {
					return err
				}
				if len(selected.BlueprintParamArgs) > 0 {
					opts.BlueprintParamArgs = selected.BlueprintParamArgs
				}
				if len(selected.BlueprintFeatureArgs) > 0 {
					opts.BlueprintFeatureArgs = selected.BlueprintFeatureArgs
				}
				if len(selected.BlueprintEntrypointArgs) > 0 {
					opts.BlueprintEntrypointArgs = selected.BlueprintEntrypointArgs
				}
				if err := applyBlueprintOverrides(opts); err != nil {
					return err
				}
			}

			mode, err = ParseDeploymentMode(opts.Mode)
			if err != nil {
				return err
			}

			applyOptionDefaults(opts, mode)

			cfg.Mode = opts.Mode
			cfg.ClusterName = opts.ClusterName
			cfg.NodeRole = opts.NodeRole
			cfg.NodeName = opts.NodeName
			cfg.NodeLabels = opts.NodeLabels
			cfg.Regions = opts.Regions
			cfg.HAEnabled = opts.HAEnabled
			cfg.HAReplicas = opts.HAReplicas
			cfg.ObservabilityBackend = opts.ObservabilityBackend
			cfg.ObservabilityEndpoint = opts.ObservabilityEndpoint
			cfg.IdentityProvider = opts.IdentityProvider
			cfg.IdentityEndpoint = opts.IdentityEndpoint
			cfg.Join = opts.JoinEndpoint
			cfg.JoinToken = opts.JoinToken
			cfg.Storage = opts.StorageBackend
			cfg.NATSMode = opts.NATSMode
			cfg.BindAddress = opts.BindAddress
			cfg.Advertise = opts.AdvertiseAddr
			cfg.PostgresHost = opts.PostgresHost
			cfg.PostgresPort = opts.PostgresPort
			cfg.PostgresDatabase = opts.PostgresDatabase
			cfg.PostgresUser = opts.PostgresUser
			cfg.PostgresPassword = opts.PostgresPassword
			cfg.PostgresSSLMode = opts.PostgresSSLMode
			cfg.NATSURLs = opts.NATSURLs
			cfg.GenerateCerts = opts.GenerateCerts
			cfg.TLSCertFile = opts.TLSCertFile
			cfg.TLSKeyFile = opts.TLSKeyFile
			cfg.TLSCAFile = opts.TLSCAFile
			cfg.TLSCSRFile = opts.TLSCSRFile
			cfg.TLSRenewalCommand = opts.TLSRenewalCommand
			cfg.TLSRenewalScriptPath = opts.TLSRenewalScriptPath
			cfg.NATSCredsFile = opts.NATSCredsFile
			cfg.NATSUser = opts.NATSUser
			cfg.NATSPassword = opts.NATSPassword
			cfg.PackageChannel = opts.PackageChannel
			cfg.PackageVersion = opts.PackageVersion
			cfg.MigrateFromSQLite = opts.MigrateFromSQLite
			cfg.MigrateBatchSize = opts.MigrateBatchSize
			cfg.MigrateContinueOnError = opts.MigrateContinueOnError
			cfg.MigrateSkipExisting = opts.MigrateSkipExisting
			cfg.BlueprintsDir = opts.BlueprintsDir
			cfg.ApplyBlueprints = opts.ApplyBlueprints
			cfg.BlueprintParams = opts.BlueprintParams
			cfg.BlueprintFeatures = opts.BlueprintFeatures
			cfg.BlueprintEntrypoints = opts.BlueprintEntrypoints
			cfg.ExportStatesDir = opts.ExportStatesDir

			runner := NewRunner(cmd.OutOrStdout())
			runner.SetMode(mode)
			runner.SetBootstrapConfig(cfg)
			runner.SetVerbose(opts.Verbose)
			runner.SetDryRun(opts.DryRun)
			runner.SetJSONOutput(opts.JSONOutput)
			if opts.OutputConfig != "" {
				if err := WriteBootstrapConfig(opts.OutputConfig, cfg); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "bootstrap config written to %s\n", opts.OutputConfig)
				return nil
			}
			if !opts.NonInteractive {
				return tui.RunProgress(ctx, func(w io.Writer) error {
					runner.SetOutput(w)
					runner.SetJSONOutput(true)
					return runner.Run(ctx)
				})
			}

			return runner.Run(ctx)
		},
	}

	cmd.Flags().StringVar(&opts.Mode, "mode", "", "Deployment mode: demo, production, fullscale, custom")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Show planned actions without making changes")
	cmd.Flags().BoolVar(&opts.Verbose, "verbose", false, "Enable verbose output")
	cmd.Flags().BoolVar(&opts.NonInteractive, "non-interactive", false, "Run without interactive prompts")
	cmd.Flags().BoolVar(&opts.SkipRepoSetup, "skip-repo-setup", false, "Skip package repository configuration (use when packages are pre-installed)")
	cmd.Flags().StringVar(&opts.ConfigPath, "config", "", "Path to bootstrap configuration file")
	cmd.Flags().StringVar(&opts.OutputConfig, "config-file", "", "Write bootstrap configuration to file")
	cmd.Flags().StringVar(&opts.ClusterName, "cluster-name", "", "Cluster name")
	cmd.Flags().StringVar(&opts.NodeRole, "node-role", "", "Node role: control-plane, agent, both")
	cmd.Flags().StringVar(&opts.NodeName, "node-name", "", "Node name (defaults to hostname)")
	cmd.Flags().StringArrayVar(&opts.NodeLabelArgs, "node-label", nil, "Node labels (format: key=value)")
	cmd.Flags().BoolVar(&opts.JSONOutput, "json", false, "Output progress as JSON")
	cmd.Flags().StringVar(&opts.JoinEndpoint, "join", "", "Cluster endpoint to join")
	cmd.Flags().StringVar(&opts.JoinToken, "join-token", "", "Join token for authentication")
	cmd.Flags().StringVar(&opts.StorageBackend, "storage-backend", "", "Storage backend: sqlite, postgres")
	cmd.Flags().StringVar(&opts.NATSMode, "nats-mode", "", "NATS mode: embedded, cluster, external, leaf")
	cmd.Flags().StringVar(&opts.BindAddress, "bind-address", "", "Address to bind services")
	cmd.Flags().StringVar(&opts.AdvertiseAddr, "advertise-address", "", "Address to advertise to cluster")
	cmd.Flags().StringVar(&opts.PostgresHost, "postgres-host", "", "PostgreSQL host")
	cmd.Flags().IntVar(&opts.PostgresPort, "postgres-port", 0, "PostgreSQL port")
	cmd.Flags().StringVar(&opts.PostgresDatabase, "postgres-database", "", "PostgreSQL database")
	cmd.Flags().StringVar(&opts.PostgresUser, "postgres-user", "", "PostgreSQL user")
	cmd.Flags().StringVar(&opts.PostgresPassword, "postgres-password", "", "PostgreSQL password")
	cmd.Flags().StringVar(&opts.PostgresSSLMode, "postgres-sslmode", "", "PostgreSQL SSL mode")
	cmd.Flags().StringSliceVar(&opts.NATSURLs, "nats-urls", nil, "External NATS URLs")
	cmd.Flags().BoolVar(&opts.GenerateCerts, "generate-certs", false, "Generate self-signed certificates")
	cmd.Flags().StringVar(&opts.TLSCertFile, "tls-cert-file", "", "TLS certificate file")
	cmd.Flags().StringVar(&opts.TLSKeyFile, "tls-key-file", "", "TLS key file")
	cmd.Flags().StringVar(&opts.TLSCAFile, "tls-ca-file", "", "TLS CA file")
	cmd.Flags().StringVar(&opts.TLSCSRFile, "tls-csr-file", "", "TLS certificate signing request output file")
	cmd.Flags().StringVar(&opts.TLSRenewalCommand, "tls-renewal-command", "", "Command to renew TLS certificates")
	cmd.Flags().StringVar(&opts.TLSRenewalScriptPath, "tls-renewal-script", "", "Path to write the TLS renewal script")
	cmd.Flags().StringVar(&opts.NATSCredsFile, "nats-creds-file", "", "NATS creds file")
	cmd.Flags().StringVar(&opts.NATSUser, "nats-user", "", "NATS username")
	cmd.Flags().StringVar(&opts.NATSPassword, "nats-password", "", "NATS password")
	cmd.Flags().StringVar(&opts.PackageChannel, "package-channel", "", "Package channel to install from")
	cmd.Flags().StringVar(&opts.PackageVersion, "package-version", "", "Package version to install (pin)")
	cmd.Flags().StringVar(&opts.MigrateFromSQLite, "migrate-from-sqlite", "", "Path to SQLite database to migrate into PostgreSQL")
	cmd.Flags().IntVar(&opts.MigrateBatchSize, "migrate-batch-size", 100, "Batch size for migration")
	cmd.Flags().BoolVar(&opts.MigrateContinueOnError, "migrate-continue-on-error", false, "Continue migration if some records fail")
	cmd.Flags().BoolVar(&opts.MigrateSkipExisting, "migrate-skip-existing", true, "Skip records that already exist in target")
	cmd.Flags().StringVar(&opts.BlueprintsDir, "blueprints-dir", "", "Directory containing blueprints")
	cmd.Flags().StringSliceVar(&opts.ApplyBlueprints, "apply-blueprint", nil, "Blueprints to apply after bootstrap")
	cmd.Flags().StringArrayVar(&opts.BlueprintParamArgs, "blueprint-param", nil, "Blueprint parameter override (format: blueprint:KEY=VALUE)")
	cmd.Flags().StringArrayVar(&opts.BlueprintFeatureArgs, "blueprint-feature", nil, "Blueprint feature toggle (format: blueprint:feature=true)")
	cmd.Flags().StringArrayVar(&opts.BlueprintEntrypointArgs, "blueprint-entrypoint", nil, "Blueprint entrypoint override (format: blueprint:entrypoint)")
	cmd.Flags().StringVar(&opts.ExportStatesDir, "export-states-dir", "", "Write rendered blueprint states to a directory instead of applying them")

	return cmd
}

func contextWithSignal(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		select {
		case <-ctx.Done():
		case <-sigChan:
			cancel()
		}
	}()

	return ctx, cancel
}

// formatPhase renders a phase label for CLI output.
func formatPhase(w io.Writer, step Progress, jsonOutput bool) {
	if jsonOutput {
		payload := map[string]interface{}{
			"event":     "phase",
			"phase":     step.Phase,
			"completed": step.Completed,
			"total":     step.Total,
		}
		if encoded, err := json.Marshal(payload); err == nil {
			fmt.Fprintln(w, string(encoded))
		}
		return
	}
	fmt.Fprintf(w, "==> [%d/%d] %s\n", step.Completed, step.Total, step.Phase)
}

func applyEnvOverrides(opts *Options) {
	if opts.Mode == "" {
		opts.Mode = os.Getenv("KSCORE_BOOTSTRAP_MODE")
	}
	if opts.ClusterName == "" {
		opts.ClusterName = os.Getenv("KSCORE_CLUSTER_NAME")
	}
	if opts.NodeRole == "" {
		opts.NodeRole = os.Getenv("KSCORE_NODE_ROLE")
	}
	if opts.NodeName == "" {
		opts.NodeName = os.Getenv("KSCORE_NODE_NAME")
	}
	if opts.JoinEndpoint == "" {
		opts.JoinEndpoint = os.Getenv("KSCORE_JOIN_ENDPOINT")
	}
	if opts.JoinToken == "" {
		opts.JoinToken = os.Getenv("KSCORE_JOIN_TOKEN")
	}
	if opts.StorageBackend == "" {
		opts.StorageBackend = os.Getenv("KSCORE_STORAGE_BACKEND")
	}
	if opts.NATSMode == "" {
		opts.NATSMode = os.Getenv("KSCORE_NATS_MODE")
	}
	if opts.BindAddress == "" {
		opts.BindAddress = os.Getenv("KSCORE_BIND_ADDRESS")
	}
	if opts.AdvertiseAddr == "" {
		opts.AdvertiseAddr = os.Getenv("KSCORE_ADVERTISE_ADDRESS")
	}
	if opts.PostgresHost == "" {
		opts.PostgresHost = os.Getenv("KSCORE_POSTGRES_HOST")
	}
	if opts.PostgresPort == 0 {
		opts.PostgresPort = getenvInt("KSCORE_POSTGRES_PORT")
	}
	if opts.PostgresDatabase == "" {
		opts.PostgresDatabase = os.Getenv("KSCORE_POSTGRES_DATABASE")
	}
	if opts.PostgresUser == "" {
		opts.PostgresUser = os.Getenv("KSCORE_POSTGRES_USER")
	}
	if opts.PostgresPassword == "" {
		opts.PostgresPassword = os.Getenv("KSCORE_POSTGRES_PASSWORD")
	}
	if opts.PostgresSSLMode == "" {
		opts.PostgresSSLMode = os.Getenv("KSCORE_POSTGRES_SSLMODE")
	}
	if len(opts.NATSURLs) == 0 {
		if value := os.Getenv("KSCORE_NATS_URLS"); value != "" {
			opts.NATSURLs = splitCSV(value)
		}
	}
	if len(opts.NodeLabelArgs) == 0 && len(opts.NodeLabels) == 0 {
		if value := os.Getenv("KSCORE_NODE_LABELS"); value != "" {
			opts.NodeLabelArgs = splitCSV(value)
		}
	}
	if !opts.GenerateCerts {
		if value := os.Getenv("KSCORE_GENERATE_CERTS"); value == "1" || value == "true" {
			opts.GenerateCerts = true
		}
	}
	if opts.TLSCertFile == "" {
		opts.TLSCertFile = os.Getenv("KSCORE_TLS_CERT_FILE")
	}
	if opts.TLSKeyFile == "" {
		opts.TLSKeyFile = os.Getenv("KSCORE_TLS_KEY_FILE")
	}
	if opts.TLSCAFile == "" {
		opts.TLSCAFile = os.Getenv("KSCORE_TLS_CA_FILE")
	}
	if opts.TLSCSRFile == "" {
		opts.TLSCSRFile = os.Getenv("KSCORE_TLS_CSR_FILE")
	}
	if opts.TLSRenewalCommand == "" {
		opts.TLSRenewalCommand = os.Getenv("KSCORE_TLS_RENEWAL_COMMAND")
	}
	if opts.TLSRenewalScriptPath == "" {
		opts.TLSRenewalScriptPath = os.Getenv("KSCORE_TLS_RENEWAL_SCRIPT")
	}
	if opts.NATSCredsFile == "" {
		opts.NATSCredsFile = os.Getenv("KSCORE_NATS_CREDS_FILE")
	}
	if opts.NATSUser == "" {
		opts.NATSUser = os.Getenv("KSCORE_NATS_USER")
	}
	if opts.NATSPassword == "" {
		opts.NATSPassword = os.Getenv("KSCORE_NATS_PASSWORD")
	}
	if opts.PackageChannel == "" {
		opts.PackageChannel = os.Getenv("KSCORE_PACKAGE_CHANNEL")
	}
	if opts.PackageVersion == "" {
		opts.PackageVersion = os.Getenv("KSCORE_PACKAGE_VERSION")
	}
	if opts.MigrateFromSQLite == "" {
		opts.MigrateFromSQLite = os.Getenv("KSCORE_MIGRATE_FROM_SQLITE")
	}
	if value := os.Getenv("KSCORE_MIGRATE_BATCH_SIZE"); value != "" {
		opts.MigrateBatchSize = getenvInt("KSCORE_MIGRATE_BATCH_SIZE")
	}
	if value := os.Getenv("KSCORE_MIGRATE_CONTINUE_ON_ERROR"); value != "" {
		opts.MigrateContinueOnError = value == "1" || value == "true"
	}
	if value := os.Getenv("KSCORE_MIGRATE_SKIP_EXISTING"); value != "" {
		opts.MigrateSkipExisting = value == "1" || value == "true"
	}
	if opts.BlueprintsDir == "" {
		opts.BlueprintsDir = os.Getenv("KSCORE_BLUEPRINTS_DIR")
	}
	if len(opts.ApplyBlueprints) == 0 {
		if value := os.Getenv("KSCORE_APPLY_BLUEPRINTS"); value != "" {
			opts.ApplyBlueprints = splitCSV(value)
		}
	}
	if opts.ExportStatesDir == "" {
		opts.ExportStatesDir = os.Getenv("KSCORE_EXPORT_STATES_DIR")
	}
	if len(opts.BlueprintParamArgs) == 0 && len(opts.BlueprintParams) == 0 {
		if value := os.Getenv("KSCORE_BLUEPRINT_PARAMS"); value != "" {
			opts.BlueprintParamArgs = splitCSV(value)
		}
	}
	if len(opts.BlueprintFeatureArgs) == 0 && len(opts.BlueprintFeatures) == 0 {
		if value := os.Getenv("KSCORE_BLUEPRINT_FEATURES"); value != "" {
			opts.BlueprintFeatureArgs = splitCSV(value)
		}
	}
	if len(opts.BlueprintEntrypointArgs) == 0 && len(opts.BlueprintEntrypoints) == 0 {
		if value := os.Getenv("KSCORE_BLUEPRINT_ENTRYPOINTS"); value != "" {
			opts.BlueprintEntrypointArgs = splitCSV(value)
		}
	}
	if !opts.NonInteractive {
		if value := os.Getenv("KSCORE_BOOTSTRAP_NON_INTERACTIVE"); value == "1" || value == "true" {
			opts.NonInteractive = true
		}
	}
}

func applyOptionDefaults(opts *Options, mode DeploymentMode) {
	if opts.ClusterName == "" {
		opts.ClusterName = "keystone"
	}

	if opts.NodeRole == "" {
		if opts.JoinEndpoint != "" {
			opts.NodeRole = "agent"
		} else if mode == DeploymentModeDemo {
			opts.NodeRole = "both"
		} else {
			opts.NodeRole = "control-plane"
		}
	}

	if opts.NodeName == "" {
		if hostname, err := os.Hostname(); err == nil {
			opts.NodeName = hostname
		}
	}

	if opts.StorageBackend == "" {
		if mode == DeploymentModeDemo {
			opts.StorageBackend = "sqlite"
		} else {
			opts.StorageBackend = "postgres"
		}
	}

	if opts.NATSMode == "" {
		if mode == DeploymentModeDemo {
			opts.NATSMode = "embedded"
		} else {
			opts.NATSMode = "cluster"
		}
	}

	if opts.PostgresPort == 0 && opts.StorageBackend == "postgres" {
		opts.PostgresPort = 5432
	}

	if opts.PostgresSSLMode == "" && opts.StorageBackend == "postgres" {
		opts.PostgresSSLMode = "prefer"
	}

	if opts.PackageChannel == "" {
		opts.PackageChannel = "stable"
	}

	if opts.GenerateCerts {
		if opts.TLSCertFile == "" {
			opts.TLSCertFile = defaultTLSCertPath
		}
		if opts.TLSKeyFile == "" {
			opts.TLSKeyFile = defaultTLSKeyPath
		}
		if opts.TLSCAFile == "" {
			opts.TLSCAFile = defaultTLSCAPath
		}
	}

	if opts.BlueprintsDir == "" && (len(opts.ApplyBlueprints) > 0 || len(opts.BlueprintParams) > 0 || len(opts.BlueprintFeatures) > 0 || len(opts.BlueprintEntrypoints) > 0) {
		opts.BlueprintsDir = defaultBlueprintsDir
	}
}

func getenvInt(key string) int {
	value := os.Getenv(key)
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
