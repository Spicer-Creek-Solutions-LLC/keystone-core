// Package bootstrap provides infrastructure for bootstrapping Keystone Core clusters
// from scratch, including seed configuration parsing, component installation,
// cluster formation, and handoff to self-management.
package bootstrap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DefaultBootstrapper is the default implementation of the Bootstrapper interface
type DefaultBootstrapper struct {
	config         *SeedConfig
	options        BootstrapOptions
	status         *BootstrapStatus
	registry       *InstallerRegistry
	formation      *ClusterFormation
	handoff        *HandoffManager
	logger         Logger
	statusCallback ProgressCallback
	startTime      time.Time
}

// NewBootstrapper creates a new bootstrapper with the given options
func NewBootstrapper(opts BootstrapOptions, logger Logger) (*DefaultBootstrapper, error) {
	if logger == nil {
		logger = &noopLogger{}
	}

	b := &DefaultBootstrapper{
		options: opts,
		status: &BootstrapStatus{
			Phase:   PhaseInitializing,
			Message: "Initializing bootstrap",
		},
		logger: logger,
	}

	return b, nil
}

// SetProgressCallback sets the callback for progress updates
func (b *DefaultBootstrapper) SetProgressCallback(cb ProgressCallback) {
	b.statusCallback = cb
}

// Bootstrap performs the full bootstrap process
func (b *DefaultBootstrapper) Bootstrap(ctx context.Context, opts BootstrapOptions) (*BootstrapResult, error) {
	b.startTime = time.Now()
	b.options = opts

	result := &BootstrapResult{
		Success: false,
	}

	// Update initial status
	b.updateStatus(PhaseInitializing, "Starting bootstrap process", 0)

	// Step 1: Load and validate configuration
	b.updateStatus(PhaseValidating, "Loading configuration", 5)
	config, err := b.loadConfiguration(ctx)
	if err != nil {
		result.Error = err.Error()
		b.updateStatus(PhaseFailed, err.Error(), 0)
		return result, fmt.Errorf("configuration error: %w", err)
	}
	b.config = config

	// Step 2: Validate configuration
	b.updateStatus(PhaseValidating, "Validating configuration", 10)
	if err := ValidateSeedConfig(config); err != nil {
		result.Error = err.Error()
		b.updateStatus(PhaseFailed, err.Error(), 0)
		return result, fmt.Errorf("validation error: %w", err)
	}

	// If dry-run, stop here
	if opts.DryRun {
		b.updateStatus(PhaseComplete, "Dry run complete - configuration is valid", 100)
		result.Success = true
		result.Duration = time.Since(b.startTime)
		return result, nil
	}

	// Step 3: Initialize installer registry
	b.updateStatus(PhaseInstallingDeps, "Initializing installers", 15)
	b.registry = NewInstallerRegistry()

	// Step 4: Install dependencies
	b.updateStatus(PhaseInstallingDeps, "Installing dependencies", 20)
	if err := b.installDependencies(ctx); err != nil {
		result.Error = err.Error()
		b.updateStatus(PhaseFailed, err.Error(), 0)
		return result, fmt.Errorf("dependency installation failed: %w", err)
	}

	// Step 5: Install server
	b.updateStatus(PhaseInstallingServer, "Installing Keystone Core server", 30)
	if err := b.installServer(ctx); err != nil {
		result.Error = err.Error()
		b.updateStatus(PhaseFailed, err.Error(), 0)
		return result, fmt.Errorf("server installation failed: %w", err)
	}

	// Step 6: Configure server
	b.updateStatus(PhaseConfiguringServer, "Configuring server", 40)
	if err := b.configureServer(ctx); err != nil {
		result.Error = err.Error()
		b.updateStatus(PhaseFailed, err.Error(), 0)
		return result, fmt.Errorf("server configuration failed: %w", err)
	}

	// Step 7: Start server
	b.updateStatus(PhaseStartingServer, "Starting server", 50)
	if err := b.startServer(ctx); err != nil {
		result.Error = err.Error()
		b.updateStatus(PhaseFailed, err.Error(), 0)
		return result, fmt.Errorf("server start failed: %w", err)
	}

	// Step 8: Form cluster
	b.updateStatus(PhaseFormingCluster, "Forming cluster", 60)
	b.formation = NewClusterFormation(config, b.registry, b.logger)
	if err := b.formation.FormCluster(ctx); err != nil {
		result.Error = err.Error()
		b.updateStatus(PhaseFailed, err.Error(), 0)
		return result, fmt.Errorf("cluster formation failed: %w", err)
	}

	// Step 9: Install initial agents if configured
	if len(config.InitialAgents) > 0 {
		b.updateStatus(PhaseInstallingAgents, "Installing agents", 70)
		if err := b.installAgents(ctx); err != nil {
			result.Error = err.Error()
			b.updateStatus(PhaseFailed, err.Error(), 0)
			return result, fmt.Errorf("agent installation failed: %w", err)
		}
	}

	// Build result
	result.ClusterID = b.generateClusterID()
	result.APIEndpoint = b.getAPIEndpoint()
	result.AdminToken = b.generateAdminToken()
	result.CAFingerprint = b.getCAFingerprint()
	result.Components = b.getComponentStatuses(ctx)

	// Step 10: Apply initial states if configured
	if config.PostBootstrap.ApplyStates {
		b.updateStatus(PhaseApplyingStates, "Applying initial states", 80)
		// States will be applied during handoff
	}

	// Step 11: Verify deployment
	if !opts.SkipVerification {
		b.updateStatus(PhaseVerifying, "Verifying deployment", 85)
		if err := b.Verify(ctx); err != nil {
			result.Error = err.Error()
			b.updateStatus(PhaseFailed, err.Error(), 0)
			return result, fmt.Errorf("verification failed: %w", err)
		}
	}

	// Step 12: Handoff to self-management
	b.updateStatus(PhaseHandoff, "Handing off to self-management", 90)
	b.handoff = NewHandoffManager(config, result, b.logger)
	if err := b.handoff.Handoff(ctx); err != nil {
		result.Error = err.Error()
		b.updateStatus(PhaseFailed, err.Error(), 0)
		return result, fmt.Errorf("handoff failed: %w", err)
	}

	// Complete
	result.Success = true
	result.Duration = time.Since(b.startTime)
	b.updateStatus(PhaseComplete, "Bootstrap complete", 100)

	return result, nil
}

// Status returns the current bootstrap status
func (b *DefaultBootstrapper) Status() *BootstrapStatus {
	return b.status
}

// Verify verifies the bootstrap result
func (b *DefaultBootstrapper) Verify(ctx context.Context) error {
	// Check all components are running
	for _, componentType := range []ComponentType{ComponentServer, ComponentNATS} {
		installer, ok := b.registry.Get(componentType)
		if !ok {
			continue // Component might not be installed (e.g., external NATS)
		}

		status, err := installer.Status(ctx)
		if err != nil {
			return fmt.Errorf("failed to get status for %s: %w", componentType, err)
		}

		if !status.Running {
			return fmt.Errorf("component %s is not running", componentType)
		}

		if !status.Healthy {
			return fmt.Errorf("component %s is not healthy", componentType)
		}
	}

	return nil
}

// Cleanup removes partial bootstrap artifacts on failure
func (b *DefaultBootstrapper) Cleanup(ctx context.Context) error {
	b.logger.Info("cleaning up bootstrap artifacts")

	// Stop and uninstall components in reverse order
	componentOrder := []ComponentType{ComponentAgent, ComponentServer, ComponentNATS}

	for _, ct := range componentOrder {
		installer, ok := b.registry.Get(ct)
		if !ok {
			continue
		}

		// Try to stop first
		if err := installer.Stop(ctx); err != nil {
			b.logger.Warn("failed to stop component", "component", ct, "error", err)
		}

		// Then uninstall
		if err := installer.Uninstall(ctx); err != nil {
			b.logger.Warn("failed to uninstall component", "component", ct, "error", err)
		}
	}

	// Clean up directories
	cleanupDirs := []string{
		"/etc/kscore",
		"/var/lib/kscore",
		"/var/log/kscore",
	}

	if b.options.Force {
		for _, dir := range cleanupDirs {
			if err := os.RemoveAll(dir); err != nil {
				b.logger.Warn("failed to remove directory", "dir", dir, "error", err)
			}
		}
	}

	return nil
}

// loadConfiguration loads the seed configuration
func (b *DefaultBootstrapper) loadConfiguration(ctx context.Context) (*SeedConfig, error) {
	switch b.options.Mode {
	case BootstrapModeSeed:
		return b.loadSeedConfig()
	case BootstrapModeRestore:
		return b.loadRestoreConfig()
	case BootstrapModeImport:
		return b.loadImportConfig()
	default:
		return b.loadSeedConfig()
	}
}

// loadSeedConfig loads configuration for seed mode
func (b *DefaultBootstrapper) loadSeedConfig() (*SeedConfig, error) {
	loader := NewConfigLoader()

	if b.options.SeedConfigPath != "" {
		config, err := loader.LoadSeedConfig(b.options.SeedConfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load seed config from %s: %w", b.options.SeedConfigPath, err)
		}
		return config, nil
	}

	// Use default configuration
	return DefaultSeedConfig(), nil
}

// loadRestoreConfig loads configuration for restore mode
func (b *DefaultBootstrapper) loadRestoreConfig() (*SeedConfig, error) {
	if b.options.BackupPath == "" {
		return nil, fmt.Errorf("backup path required for restore mode")
	}

	// In restore mode, we load config from the backup
	// For now, return default config - full restore implementation in Phase 2
	return DefaultSeedConfig(), nil
}

// loadImportConfig loads configuration for import mode
func (b *DefaultBootstrapper) loadImportConfig() (*SeedConfig, error) {
	// Import mode discovers existing installation
	// For now, return default config - full import implementation later
	return DefaultSeedConfig(), nil
}

// installDependencies installs required dependencies
func (b *DefaultBootstrapper) installDependencies(ctx context.Context) error {
	// Install NATS if using embedded mode
	if b.config.NATS.Mode == NATSModeEmbedded {
		// NATS is embedded, no separate installation needed
		b.logger.Info("using embedded NATS, no separate installation needed")
		return nil
	}

	// For standalone/cluster mode, we might need to install NATS
	// This depends on the installation method specified
	return nil
}

// installServer installs the Keystone Core server
func (b *DefaultBootstrapper) installServer(ctx context.Context) error {
	installer := &ServerInstaller{
		BaseInstaller: BaseInstaller{logger: b.logger},
	}
	b.registry.Register(ComponentServer, installer)

	componentConfig := ComponentConfig{
		Type:       ComponentServer,
		Version:    "latest", // Would be specific version in real implementation
		ConfigPath: "/etc/kscore/server.yaml",
		DataDir:    "/var/lib/kscore",
	}

	return installer.Install(ctx, componentConfig)
}

// configureServer configures the Keystone Core server
func (b *DefaultBootstrapper) configureServer(ctx context.Context) error {
	installer, ok := b.registry.Get(ComponentServer)
	if !ok {
		return fmt.Errorf("server installer not registered")
	}

	componentConfig := ComponentConfig{
		Type:       ComponentServer,
		ConfigPath: "/etc/kscore/server.yaml",
		DataDir:    "/var/lib/kscore",
		Settings: map[string]any{
			"cluster_name":    b.config.Cluster.Name,
			"api_listen":      b.config.ControlPlane.API.Listen,
			"nats_mode":       b.config.NATS.Mode,
			"database_type":   b.config.Database.Type,
			"database_path":   b.config.Database.Path,
			"etcd_mode":       b.config.Etcd.Mode,
			"tls_enabled":     b.config.ControlPlane.API.TLS.Enabled,
			"tls_auto":        b.config.ControlPlane.API.TLS.AutoGenerate,
		},
	}

	return installer.Configure(ctx, componentConfig)
}

// startServer starts the Keystone Core server
func (b *DefaultBootstrapper) startServer(ctx context.Context) error {
	installer, ok := b.registry.Get(ComponentServer)
	if !ok {
		return fmt.Errorf("server installer not registered")
	}

	return installer.Start(ctx)
}

// installAgents installs agents on configured hosts
func (b *DefaultBootstrapper) installAgents(ctx context.Context) error {
	for _, agentConfig := range b.config.InitialAgents {
		b.logger.Info("would install agent on host", "host", agentConfig.Host)
		// In full implementation, this would:
		// 1. SSH to the target host
		// 2. Copy agent binary
		// 3. Configure agent with server connection details
		// 4. Start agent service
	}

	return nil
}

// generateClusterID generates a unique cluster identifier
func (b *DefaultBootstrapper) generateClusterID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return fmt.Sprintf("%s-%s", b.config.Cluster.Name, hex.EncodeToString(bytes))
}

// generateAdminToken generates an admin token for initial access
func (b *DefaultBootstrapper) generateAdminToken() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// getAPIEndpoint returns the API endpoint for the cluster
func (b *DefaultBootstrapper) getAPIEndpoint() string {
	if len(b.config.ControlPlane.Nodes) > 0 {
		node := b.config.ControlPlane.Nodes[0]
		return fmt.Sprintf("%s:%d", node.Host, node.Port)
	}
	return b.config.ControlPlane.API.Listen
}

// getCAFingerprint returns the CA certificate fingerprint
func (b *DefaultBootstrapper) getCAFingerprint() string {
	// In real implementation, compute SHA256 fingerprint of CA cert
	caPath := filepath.Join("/etc/kscore/certs", "ca.crt")
	if _, err := os.Stat(caPath); err != nil {
		return "auto-generated"
	}
	return "sha256:..." // Would compute actual fingerprint
}

// getComponentStatuses returns the status of all installed components
func (b *DefaultBootstrapper) getComponentStatuses(ctx context.Context) []ComponentStatus {
	var statuses []ComponentStatus

	for _, ct := range []ComponentType{ComponentServer, ComponentNATS, ComponentAgent} {
		installer, ok := b.registry.Get(ct)
		if !ok {
			continue
		}

		status, err := installer.Status(ctx)
		if err != nil {
			statuses = append(statuses, ComponentStatus{
				Type:  ct,
				Error: err.Error(),
			})
			continue
		}

		statuses = append(statuses, *status)
	}

	return statuses
}

// updateStatus updates the bootstrap status and calls the callback
func (b *DefaultBootstrapper) updateStatus(phase BootstrapPhase, message string, progress int) {
	b.status = &BootstrapStatus{
		Phase:       phase,
		Message:     message,
		Progress:    progress,
		StartTime:   b.startTime,
		CurrentStep: message,
	}

	if phase == PhaseFailed {
		b.status.Error = message
	}

	if b.statusCallback != nil {
		b.statusCallback(b.status)
	}

	// Log status updates
	if b.logger != nil {
		b.logger.Info("bootstrap status", "phase", phase, "message", message, "progress", progress)
	}
}

// noopLogger is a no-op logger implementation
type noopLogger struct{}

func (l *noopLogger) Debug(msg string, args ...any) {}
func (l *noopLogger) Info(msg string, args ...any)  {}
func (l *noopLogger) Warn(msg string, args ...any)  {}
func (l *noopLogger) Error(msg string, args ...any) {}

// RunBootstrap is a convenience function to run a bootstrap with default settings
func RunBootstrap(ctx context.Context, configPath string, verbose bool) (*BootstrapResult, error) {
	opts := BootstrapOptions{
		Mode:           BootstrapModeSeed,
		SeedConfigPath: configPath,
		Verbose:        verbose,
	}

	bootstrapper, err := NewBootstrapper(opts, nil)
	if err != nil {
		return nil, err
	}

	return bootstrapper.Bootstrap(ctx, opts)
}
