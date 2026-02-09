// Package main implements the kscore-files CLI for file server operations.
// This is a kscorectl plugin that provides file distribution management.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/shawnbutts/keystone-core/internal/cli/auditutil"
	"github.com/shawnbutts/keystone-core/internal/cli/deprecation"
	"github.com/shawnbutts/keystone-core/internal/files"
	"github.com/shawnbutts/keystone-core/internal/files/backend"
)

var (
	cfgFile     string
	natsURL     string
	clusterID   string
	instanceID  string
	auditLevel  string
	auditOutput string
)

func main() {
	rootCmd := newRootCmd()
	auditHandler := auditutil.Attach(rootCmd, "kscore-files", &auditLevel, &auditOutput)
	if err := rootCmd.Execute(); err != nil {
		auditHandler.LogFailure(err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-files",
		Short: "File distribution server for Keystone Core",
		Long: `kscore-files provides file distribution over NATS for Keystone Core.

It enables agents to retrieve packages, configurations, binaries, and other
files without requiring direct HTTP/S3 access from agents.`,
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path")
	rootCmd.PersistentFlags().StringVar(&natsURL, "nats-url", "nats://localhost:4222", "NATS server URL")
	rootCmd.PersistentFlags().StringVar(&clusterID, "cluster-id", "", "cluster ID")
	rootCmd.PersistentFlags().StringVar(&instanceID, "instance-id", "", "server instance ID")
	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")

	rootCmd.AddCommand(newServeCommand())
	rootCmd.AddCommand(newFilesCmd())
	rootCmd.AddCommand(newCacheCmd())
	rootCmd.AddCommand(newNamespaceCmd())
	rootCmd.AddCommand(newVersionCommand())

	// Add deprecated commands (moving to kscore-files-storage)
	backendCmd := newBackendCmd()
	mirrorsCmd := newMirrorsCmd()
	rootCmd.AddCommand(backendCmd)
	rootCmd.AddCommand(mirrorsCmd)

	// Apply deprecation warnings
	storageDeprecations := deprecation.FilesStorageDeprecations()
	deprecation.DeprecateCommand(backendCmd, storageDeprecations["backend"])
	deprecation.DeprecateCommand(mirrorsCmd, storageDeprecations["mirrors"])

	return rootCmd
}

// ServerConfig is the configuration for the file server.
type ServerConfig struct {
	Server struct {
		ClusterID      string        `yaml:"cluster_id"`
		InstanceID     string        `yaml:"instance_id"`
		Workers        int           `yaml:"workers"`
		MaxChunkSize   int           `yaml:"max_chunk_size"`
		MaxFileSize    int64         `yaml:"max_file_size"`
		RequestTimeout time.Duration `yaml:"request_timeout"`
	} `yaml:"server"`

	NATS struct {
		URL         string `yaml:"url"`
		Token       string `yaml:"token,omitempty"`
		Username    string `yaml:"username,omitempty"`
		Password    string `yaml:"password,omitempty"`
		TLSCertFile string `yaml:"tls_cert_file,omitempty"`
		TLSKeyFile  string `yaml:"tls_key_file,omitempty"`
		TLSCAFile   string `yaml:"tls_ca_file,omitempty"`
	} `yaml:"nats"`

	Backends []BackendConfig `yaml:"backends"`
}

// BackendConfig is the configuration for a storage backend.
type BackendConfig struct {
	Name     string   `yaml:"name"`
	Type     string   `yaml:"type"`
	RootPath string   `yaml:"root_path,omitempty"`
	Paths    []string `yaml:"paths,omitempty"`
	ReadOnly bool     `yaml:"read_only,omitempty"`

	// S3 backend options
	Bucket         string `yaml:"bucket,omitempty"`
	Region         string `yaml:"region,omitempty"`
	Endpoint       string `yaml:"endpoint,omitempty"`
	AccessKeyID    string `yaml:"access_key_id,omitempty"`
	SecretAccessKey string `yaml:"secret_access_key,omitempty"`
	Profile        string `yaml:"profile,omitempty"`
	UsePathStyle   bool   `yaml:"use_path_style,omitempty"`
	Prefix         string `yaml:"prefix,omitempty"`

	// GCS backend options
	Project         string `yaml:"project,omitempty"`
	CredentialsFile string `yaml:"credentials_file,omitempty"`

	// Azure backend options
	Container        string `yaml:"container,omitempty"`
	AccountName      string `yaml:"account_name,omitempty"`
	AccountKey       string `yaml:"account_key,omitempty"`
	ConnectionString string `yaml:"connection_string,omitempty"`

	// Git backend options
	URL        string        `yaml:"url,omitempty"`
	Branch     string        `yaml:"branch,omitempty"`
	LocalPath  string        `yaml:"local_path,omitempty"`
	SSHKeyFile string        `yaml:"ssh_key_file,omitempty"`
	Username   string        `yaml:"username,omitempty"`
	Password   string        `yaml:"password,omitempty"`
	AutoPull   bool          `yaml:"auto_pull,omitempty"`
	PullInterval time.Duration `yaml:"pull_interval,omitempty"`

	// NATS object store backend options
	BucketName string `yaml:"bucket_name,omitempty"`
}

func newServeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the file server",
		Long:  `Start the file distribution server that serves files to agents over NATS.`,
		RunE:  runServe,
	}

	return cmd
}

func runServe(cmd *cobra.Command, args []string) error {
	// Load configuration
	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Override with flags
	if natsURL != "" && config.NATS.URL == "" {
		config.NATS.URL = natsURL
	}
	if clusterID != "" {
		config.Server.ClusterID = clusterID
	}
	if instanceID != "" {
		config.Server.InstanceID = instanceID
	}

	// Validate required fields
	if config.Server.ClusterID == "" {
		return fmt.Errorf("cluster_id is required (use --cluster-id or config file)")
	}
	if config.NATS.URL == "" {
		return fmt.Errorf("NATS URL is required (use --nats-url or config file)")
	}

	// Create server
	serverConfig := &files.ServerConfig{
		ClusterID:      config.Server.ClusterID,
		InstanceID:     config.Server.InstanceID,
		Workers:        config.Server.Workers,
		MaxChunkSize:   config.Server.MaxChunkSize,
		MaxFileSize:    config.Server.MaxFileSize,
		RequestTimeout: config.Server.RequestTimeout,
	}

	server, err := files.NewServer(serverConfig)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	// Add backends
	for i := range config.Backends {
		b, err := createBackend(config.Backends[i])
		if err != nil {
			return fmt.Errorf("failed to create backend %s: %w", config.Backends[i].Name, err)
		}
		server.AddBackend(b)
	}

	// Connect to NATS
	natsOpts := []nats.Option{
		nats.Name("kscore-files"),
	}
	if config.NATS.Token != "" {
		natsOpts = append(natsOpts, nats.Token(config.NATS.Token))
	}
	if config.NATS.Username != "" && config.NATS.Password != "" {
		natsOpts = append(natsOpts, nats.UserInfo(config.NATS.Username, config.NATS.Password))
	}

	nc, err := nats.Connect(config.NATS.URL, natsOpts...)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}
	defer nc.Close()

	// Start server
	if err := server.Start(nc); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	log.Printf("[files] Server started: cluster=%s instance=%s", config.Server.ClusterID, serverConfig.InstanceID)

	// Wait for shutdown signal
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()

	log.Println("[files] Shutting down...")
	if err := server.Stop(); err != nil {
		log.Printf("[files] Error stopping server: %v", err)
	}

	return nil
}

func loadConfig() (*ServerConfig, error) {
	config := &ServerConfig{}

	// Set defaults
	config.NATS.URL = "nats://localhost:4222"
	config.Server.Workers = 10
	config.Server.MaxChunkSize = files.DefaultChunkSize
	config.Server.MaxFileSize = files.DefaultMaxFileSize
	config.Server.RequestTimeout = 5 * time.Minute

	// Load from file if specified
	if cfgFile != "" {
		data, err := os.ReadFile(cfgFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		if err := yaml.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	return config, nil
}

func createBackend(bc BackendConfig) (backend.Backend, error) {
	base := backend.Config{
		Name:     bc.Name,
		Type:     backend.Type(bc.Type),
		Paths:    bc.Paths,
		ReadOnly: bc.ReadOnly,
	}

	switch bc.Type {
	case "filesystem", "local":
		base.Type = backend.TypeFilesystem
		return backend.NewFilesystemBackend(&backend.FilesystemConfig{
			Config:     base,
			Root:       bc.RootPath,
			CreateDirs: true,
		})

	case "s3":
		return backend.NewS3Backend(&backend.S3Config{
			Config:         base,
			Bucket:         bc.Bucket,
			Region:         bc.Region,
			Endpoint:       bc.Endpoint,
			AccessKeyID:    bc.AccessKeyID,
			SecretAccessKey: bc.SecretAccessKey,
			Profile:        bc.Profile,
			UsePathStyle:   bc.UsePathStyle,
			Prefix:         bc.Prefix,
		})

	case "gcs":
		return backend.NewGCSBackend(&backend.GCSConfig{
			Config:          base,
			Bucket:          bc.Bucket,
			Project:         bc.Project,
			CredentialsFile: bc.CredentialsFile,
			Prefix:          bc.Prefix,
		})

	case "azure":
		return backend.NewAzureBackend(&backend.AzureConfig{
			Config:           base,
			Container:        bc.Container,
			AccountName:      bc.AccountName,
			AccountKey:       bc.AccountKey,
			ConnectionString: bc.ConnectionString,
			Prefix:           bc.Prefix,
		})

	case "git":
		return backend.NewGitBackend(&backend.GitConfig{
			Config:       base,
			URL:          bc.URL,
			Branch:       bc.Branch,
			LocalPath:    bc.LocalPath,
			SSHKeyFile:   bc.SSHKeyFile,
			Username:     bc.Username,
			Password:     bc.Password,
			AutoPull:     bc.AutoPull,
			PullInterval: bc.PullInterval,
		})

	case "nats", "nats-object-store":
		base.Type = backend.TypeNATSObject
		return backend.NewNATSBackend(&backend.NATSConfig{
			Config:     base,
			BucketName: bc.BucketName,
			URL:        bc.Endpoint,
		})

	default:
		return nil, fmt.Errorf("unsupported backend type: %s (supported: filesystem, s3, gcs, azure, git, nats)", bc.Type)
	}
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("kscore-files version 0.1.0")
		},
	}
}
