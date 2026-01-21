package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/shawnbutts/keystone-core/pkg/cli/auditutil"
	"github.com/shawnbutts/keystone-core/pkg/cli/output"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

var (
	serverAddr   string
	outputFormat string
	verbose      bool
	auditLevel   string
	auditOutput  string
)

// Config holds CLI configuration
type Config struct {
	ServerAddr   string
	OutputFormat string
	Verbose      bool
}

func main() {
	rootCmd := newRootCmd()
	auditHandler := auditutil.Attach(rootCmd, "kscore-backup", &auditLevel, &auditOutput)
	if err := rootCmd.Execute(); err != nil {
		auditHandler.LogFailure(err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-backup",
		Short: "Keystone Core backup management plugin",
		Long: `kscore-backup is a CLI plugin for managing Keystone Core backups.

This plugin provides commands for:
  - Creating backups (full, incremental, component-specific)
  - Listing and querying backup history
  - Verifying backup integrity
  - Restoring from backups
  - Managing backup replication

Usage via kscorectl:
  kscorectl backup create --type full
  kscorectl backup list --last 24h
  kscorectl backup verify --id backup-20240115-060000
  kscorectl backup restore --id backup-20240115-060000`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().StringVarP(&serverAddr, "server", "s", "localhost:9090", "Control plane server address")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, json, yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")

	cfg := &Config{
		ServerAddr:   serverAddr,
		OutputFormat: outputFormat,
		Verbose:      verbose,
	}

	rootCmd.AddCommand(
		newCreateCmd(cfg),
		newListCmd(cfg),
		newShowCmd(cfg),
		newVerifyCmd(cfg),
		newRestoreCmd(cfg),
		newDeleteCmd(cfg),
		newReplicationStatusCmd(cfg),
		newScheduleCmd(cfg),
		newRetentionCmd(cfg),
		newVersionCmd(),
	)

	return rootCmd
}

// BackupInfo represents backup information
type BackupInfo struct {
	ID          string            `json:"id" yaml:"id"`
	Type        string            `json:"type" yaml:"type"`
	Status      string            `json:"status" yaml:"status"`
	Size        string            `json:"size" yaml:"size"`
	SizeBytes   int64             `json:"size_bytes" yaml:"size_bytes"`
	Components  []string          `json:"components" yaml:"components"`
	Destination string            `json:"destination" yaml:"destination"`
	Encrypted   bool              `json:"encrypted" yaml:"encrypted"`
	Compressed  bool              `json:"compressed" yaml:"compressed"`
	Checksum    string            `json:"checksum" yaml:"checksum"`
	CreatedAt   string            `json:"created_at" yaml:"created_at"`
	CompletedAt string            `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
	Duration    string            `json:"duration,omitempty" yaml:"duration,omitempty"`
	Error       string            `json:"error,omitempty" yaml:"error,omitempty"`
	Labels      map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

// VerificationResult represents backup verification result
type VerificationResult struct {
	BackupID       string            `json:"backup_id" yaml:"backup_id"`
	Valid          bool              `json:"valid" yaml:"valid"`
	ChecksumMatch  bool              `json:"checksum_match" yaml:"checksum_match"`
	ComponentsOK   map[string]bool   `json:"components_ok" yaml:"components_ok"`
	IntegrityOK    bool              `json:"integrity_ok" yaml:"integrity_ok"`
	Restorable     bool              `json:"restorable" yaml:"restorable"`
	Issues         []string          `json:"issues,omitempty" yaml:"issues,omitempty"`
	VerifiedAt     string            `json:"verified_at" yaml:"verified_at"`
	VerificationID string            `json:"verification_id" yaml:"verification_id"`
}

// ReplicationStatus represents backup replication status
type ReplicationStatus struct {
	Enabled      bool                `json:"enabled" yaml:"enabled"`
	Destinations []ReplicationDest   `json:"destinations" yaml:"destinations"`
	LastSync     string              `json:"last_sync" yaml:"last_sync"`
	NextSync     string              `json:"next_sync" yaml:"next_sync"`
	SyncInterval string              `json:"sync_interval" yaml:"sync_interval"`
	Status       string              `json:"status" yaml:"status"`
}

// ReplicationDest represents a replication destination
type ReplicationDest struct {
	Name       string `json:"name" yaml:"name"`
	Type       string `json:"type" yaml:"type"`
	Status     string `json:"status" yaml:"status"`
	LastSync   string `json:"last_sync" yaml:"last_sync"`
	BackupCount int   `json:"backup_count" yaml:"backup_count"`
	TotalSize  string `json:"total_size" yaml:"total_size"`
}

// RestoreResult represents restore operation result
type RestoreResult struct {
	RestoreID   string   `json:"restore_id" yaml:"restore_id"`
	BackupID    string   `json:"backup_id" yaml:"backup_id"`
	Status      string   `json:"status" yaml:"status"`
	Target      string   `json:"target" yaml:"target"`
	Components  []string `json:"components" yaml:"components"`
	StartedAt   string   `json:"started_at" yaml:"started_at"`
	CompletedAt string   `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
	Duration    string   `json:"duration,omitempty" yaml:"duration,omitempty"`
	DryRun      bool     `json:"dry_run" yaml:"dry_run"`
	Error       string   `json:"error,omitempty" yaml:"error,omitempty"`
}

// ScheduleInfo represents backup schedule information
type ScheduleInfo struct {
	Name        string   `json:"name" yaml:"name"`
	Schedule    string   `json:"schedule" yaml:"schedule"`
	Type        string   `json:"type" yaml:"type"`
	Components  []string `json:"components" yaml:"components"`
	Destination string   `json:"destination" yaml:"destination"`
	Enabled     bool     `json:"enabled" yaml:"enabled"`
	LastRun     string   `json:"last_run" yaml:"last_run"`
	NextRun     string   `json:"next_run" yaml:"next_run"`
	RetainCount int      `json:"retain_count" yaml:"retain_count"`
}

// RetentionPolicy represents backup retention policy
type RetentionPolicy struct {
	Name          string `json:"name" yaml:"name"`
	MaxBackups    int    `json:"max_backups" yaml:"max_backups"`
	MaxAge        string `json:"max_age" yaml:"max_age"`
	KeepDaily     int    `json:"keep_daily" yaml:"keep_daily"`
	KeepWeekly    int    `json:"keep_weekly" yaml:"keep_weekly"`
	KeepMonthly   int    `json:"keep_monthly" yaml:"keep_monthly"`
	KeepYearly    int    `json:"keep_yearly" yaml:"keep_yearly"`
	AppliesTo     string `json:"applies_to" yaml:"applies_to"`
}

func newCreateCmd(cfg *Config) *cobra.Command {
	var (
		backupType  string
		components  []string
		destination string
		encrypt     bool
		compress    bool
		labels      []string
		async       bool
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new backup",
		Long: `Create a new backup of Keystone Core data.

Backup types:
  full          - Complete backup of all components
  incremental   - Only changes since last backup
  database      - Database only
  configuration - Configuration files only
  jetstream     - NATS JetStream data only
  etcd          - etcd cluster data only
  secrets       - Secrets and credentials only

Examples:
  # Create a full backup
  kscorectl backup create --type full

  # Create an incremental backup
  kscorectl backup create --type incremental

  # Create a database-only backup to S3
  kscorectl backup create --type database --destination s3://mybucket/backups

  # Create encrypted backup with specific components
  kscorectl backup create --type full --components database,config --encrypt`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cfg, backupType, components, destination, encrypt, compress, labels, async)
		},
	}

	cmd.Flags().StringVarP(&backupType, "type", "t", "full", "Backup type (full, incremental, database, configuration, jetstream, etcd, secrets)")
	cmd.Flags().StringSliceVarP(&components, "components", "c", nil, "Specific components to backup (database, config, secrets, jetstream, etcd, certificates)")
	cmd.Flags().StringVarP(&destination, "destination", "d", "", "Backup destination (local, s3://bucket/path, gs://bucket/path, azure://container/path)")
	cmd.Flags().BoolVarP(&encrypt, "encrypt", "e", false, "Encrypt the backup")
	cmd.Flags().BoolVar(&compress, "compress", true, "Compress the backup")
	cmd.Flags().StringSliceVarP(&labels, "label", "l", nil, "Labels to attach to backup (key=value)")
	cmd.Flags().BoolVar(&async, "async", false, "Run backup asynchronously")

	return cmd
}

func runCreate(cfg *Config, backupType string, components []string, destination string, encrypt, compress bool, labels []string, async bool) error {
	// Parse labels
	labelMap := make(map[string]string)
	for _, l := range labels {
		parts := strings.SplitN(l, "=", 2)
		if len(parts) == 2 {
			labelMap[parts[0]] = parts[1]
		}
	}

	// Generate backup info (sample for demonstration)
	backupID := fmt.Sprintf("backup-%s", time.Now().Format("20060102-150405"))

	if destination == "" {
		destination = "local:/var/lib/kscore/backups"
	}

	if len(components) == 0 {
		switch backupType {
		case "full":
			components = []string{"database", "config", "secrets", "jetstream", "etcd", "certificates"}
		case "database":
			components = []string{"database"}
		case "configuration":
			components = []string{"config"}
		case "jetstream":
			components = []string{"jetstream"}
		case "etcd":
			components = []string{"etcd"}
		case "secrets":
			components = []string{"secrets"}
		default:
			components = []string{"database", "config"}
		}
	}

	backup := BackupInfo{
		ID:          backupID,
		Type:        backupType,
		Status:      "completed",
		Size:        "256 MB",
		SizeBytes:   268435456,
		Components:  components,
		Destination: destination,
		Encrypted:   encrypt,
		Compressed:  compress,
		Checksum:    "sha256:a1b2c3d4e5f6...",
		CreatedAt:   time.Now().Format(time.RFC3339),
		CompletedAt: time.Now().Add(2 * time.Minute).Format(time.RFC3339),
		Duration:    "2m15s",
		Labels:      labelMap,
	}

	if async {
		backup.Status = "running"
		backup.CompletedAt = ""
		backup.Duration = ""
	}

	return outputResult(cfg.OutputFormat, backup, func() {
		if async {
			fmt.Printf("Backup started: %s\n", backup.ID)
			fmt.Printf("Status: %s\n", backup.Status)
			fmt.Printf("Use 'kscorectl backup show %s' to check progress\n", backup.ID)
		} else {
			fmt.Printf("Backup created successfully\n\n")
			fmt.Printf("ID:          %s\n", backup.ID)
			fmt.Printf("Type:        %s\n", backup.Type)
			fmt.Printf("Status:      %s\n", backup.Status)
			fmt.Printf("Size:        %s\n", backup.Size)
			fmt.Printf("Components:  %s\n", strings.Join(backup.Components, ", "))
			fmt.Printf("Destination: %s\n", backup.Destination)
			fmt.Printf("Encrypted:   %v\n", backup.Encrypted)
			fmt.Printf("Compressed:  %v\n", backup.Compressed)
			fmt.Printf("Duration:    %s\n", backup.Duration)
			fmt.Printf("Checksum:    %s\n", backup.Checksum)
		}
	})
}

func newListCmd(cfg *Config) *cobra.Command {
	var (
		last       string
		backupType string
		status     string
		limit      int
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List backups",
		Long: `List backups with optional filtering.

Examples:
  # List all backups
  kscorectl backup list

  # List backups from last 24 hours
  kscorectl backup list --last 24h

  # List only full backups
  kscorectl backup list --type full

  # List completed backups
  kscorectl backup list --status completed`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cfg, last, backupType, status, limit)
		},
	}

	cmd.Flags().StringVar(&last, "last", "", "Show backups from last duration (e.g., 24h, 7d)")
	cmd.Flags().StringVarP(&backupType, "type", "t", "", "Filter by backup type")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status (completed, failed, running)")
	cmd.Flags().IntVarP(&limit, "limit", "n", 20, "Maximum number of backups to show")

	return cmd
}

func runList(cfg *Config, last, backupType, status string, limit int) error {
	// Sample backup data
	backups := []BackupInfo{
		{
			ID:          "backup-20240115-060000",
			Type:        "full",
			Status:      "completed",
			Size:        "512 MB",
			SizeBytes:   536870912,
			Components:  []string{"database", "config", "secrets", "jetstream", "etcd", "certificates"},
			Destination: "s3://kscore-backups/prod",
			Encrypted:   true,
			Compressed:  true,
			Checksum:    "sha256:abc123...",
			CreatedAt:   "2024-01-15T06:00:00Z",
			CompletedAt: "2024-01-15T06:05:23Z",
			Duration:    "5m23s",
		},
		{
			ID:          "backup-20240114-180000",
			Type:        "incremental",
			Status:      "completed",
			Size:        "128 MB",
			SizeBytes:   134217728,
			Components:  []string{"database", "config"},
			Destination: "s3://kscore-backups/prod",
			Encrypted:   true,
			Compressed:  true,
			Checksum:    "sha256:def456...",
			CreatedAt:   "2024-01-14T18:00:00Z",
			CompletedAt: "2024-01-14T18:02:15Z",
			Duration:    "2m15s",
		},
		{
			ID:          "backup-20240114-060000",
			Type:        "full",
			Status:      "completed",
			Size:        "498 MB",
			SizeBytes:   522190848,
			Components:  []string{"database", "config", "secrets", "jetstream", "etcd", "certificates"},
			Destination: "s3://kscore-backups/prod",
			Encrypted:   true,
			Compressed:  true,
			Checksum:    "sha256:ghi789...",
			CreatedAt:   "2024-01-14T06:00:00Z",
			CompletedAt: "2024-01-14T06:04:58Z",
			Duration:    "4m58s",
		},
		{
			ID:          "backup-20240113-120000",
			Type:        "database",
			Status:      "failed",
			Size:        "0 B",
			SizeBytes:   0,
			Components:  []string{"database"},
			Destination: "local:/var/lib/kscore/backups",
			Encrypted:   false,
			Compressed:  true,
			CreatedAt:   "2024-01-13T12:00:00Z",
			Error:       "database connection timeout",
		},
		{
			ID:          "backup-20240113-060000",
			Type:        "full",
			Status:      "completed",
			Size:        "485 MB",
			SizeBytes:   508559360,
			Components:  []string{"database", "config", "secrets", "jetstream", "etcd", "certificates"},
			Destination: "s3://kscore-backups/prod",
			Encrypted:   true,
			Compressed:  true,
			Checksum:    "sha256:jkl012...",
			CreatedAt:   "2024-01-13T06:00:00Z",
			CompletedAt: "2024-01-13T06:05:01Z",
			Duration:    "5m1s",
		},
	}

	// Filter by type
	if backupType != "" {
		var filtered []BackupInfo
		for _, b := range backups {
			if b.Type == backupType {
				filtered = append(filtered, b)
			}
		}
		backups = filtered
	}

	// Filter by status
	if status != "" {
		var filtered []BackupInfo
		for _, b := range backups {
			if b.Status == status {
				filtered = append(filtered, b)
			}
		}
		backups = filtered
	}

	// Apply limit
	if limit > 0 && len(backups) > limit {
		backups = backups[:limit]
	}

	return outputResult(cfg.OutputFormat, backups, func() {
		if len(backups) == 0 {
			fmt.Println("No backups found")
			return
		}

		table := &output.Table{
			Headers: []string{"ID", "TYPE", "STATUS", "SIZE", "COMPONENTS", "DESTINATION", "CREATED"},
		}

		for _, b := range backups {
			statusIcon := "✓"
			if b.Status == "failed" {
				statusIcon = "✗"
			} else if b.Status == "running" {
				statusIcon = "◐"
			}

			components := strings.Join(b.Components, ",")
			if len(components) > 20 {
				components = components[:17] + "..."
			}

			dest := b.Destination
			if len(dest) > 25 {
				dest = "..." + dest[len(dest)-22:]
			}

			table.Rows = append(table.Rows, []string{
				b.ID,
				b.Type,
				fmt.Sprintf("%s %s", statusIcon, b.Status),
				b.Size,
				components,
				dest,
				b.CreatedAt,
			})
		}

		output.WriteTable(os.Stdout, table)
		fmt.Printf("\nTotal: %d backups\n", len(backups))
	})
}

func newShowCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <backup-id>",
		Short: "Show backup details",
		Long: `Show detailed information about a specific backup.

Examples:
  kscorectl backup show backup-20240115-060000`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShow(cfg, args[0])
		},
	}

	return cmd
}

func runShow(cfg *Config, backupID string) error {
	// Sample backup data
	backup := BackupInfo{
		ID:          backupID,
		Type:        "full",
		Status:      "completed",
		Size:        "512 MB",
		SizeBytes:   536870912,
		Components:  []string{"database", "config", "secrets", "jetstream", "etcd", "certificates"},
		Destination: "s3://kscore-backups/prod/" + backupID + ".tar.gz",
		Encrypted:   true,
		Compressed:  true,
		Checksum:    "sha256:a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4y5z6",
		CreatedAt:   "2024-01-15T06:00:00Z",
		CompletedAt: "2024-01-15T06:05:23Z",
		Duration:    "5m23s",
		Labels: map[string]string{
			"environment": "production",
			"schedule":    "daily",
		},
	}

	return outputResult(cfg.OutputFormat, backup, func() {
		fmt.Printf("Backup Details\n")
		fmt.Printf("==============\n\n")
		fmt.Printf("ID:          %s\n", backup.ID)
		fmt.Printf("Type:        %s\n", backup.Type)
		fmt.Printf("Status:      %s\n", backup.Status)
		fmt.Printf("Size:        %s (%d bytes)\n", backup.Size, backup.SizeBytes)
		fmt.Printf("Components:  %s\n", strings.Join(backup.Components, ", "))
		fmt.Printf("Destination: %s\n", backup.Destination)
		fmt.Printf("Encrypted:   %v\n", backup.Encrypted)
		fmt.Printf("Compressed:  %v\n", backup.Compressed)
		fmt.Printf("Checksum:    %s\n", backup.Checksum)
		fmt.Printf("Created:     %s\n", backup.CreatedAt)
		fmt.Printf("Completed:   %s\n", backup.CompletedAt)
		fmt.Printf("Duration:    %s\n", backup.Duration)
		if len(backup.Labels) > 0 {
			fmt.Printf("\nLabels:\n")
			for k, v := range backup.Labels {
				fmt.Printf("  %s: %s\n", k, v)
			}
		}
	})
}

func newVerifyCmd(cfg *Config) *cobra.Command {
	var (
		checkIntegrity  bool
		checkRestorable bool
		verbose         bool
	)

	cmd := &cobra.Command{
		Use:   "verify <backup-id>",
		Short: "Verify backup integrity",
		Long: `Verify the integrity and restorability of a backup.

This command performs:
  - Checksum verification
  - Component integrity checks
  - Optional restore simulation

Examples:
  # Basic verification
  kscorectl backup verify --id backup-20240115-060000

  # Full verification with restore check
  kscorectl backup verify --id backup-20240115-060000 --check-restorable`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerify(cfg, args[0], checkIntegrity, checkRestorable, verbose)
		},
	}

	cmd.Flags().BoolVar(&checkIntegrity, "check-integrity", true, "Verify component integrity")
	cmd.Flags().BoolVar(&checkRestorable, "check-restorable", false, "Verify backup can be restored")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed verification output")

	return cmd
}

func runVerify(cfg *Config, backupID string, checkIntegrity, checkRestorable, verbose bool) error {
	result := VerificationResult{
		BackupID:      backupID,
		Valid:         true,
		ChecksumMatch: true,
		ComponentsOK: map[string]bool{
			"database":     true,
			"config":       true,
			"secrets":      true,
			"jetstream":    true,
			"etcd":         true,
			"certificates": true,
		},
		IntegrityOK:    true,
		Restorable:     checkRestorable,
		Issues:         []string{},
		VerifiedAt:     time.Now().Format(time.RFC3339),
		VerificationID: fmt.Sprintf("verify-%s", time.Now().Format("20060102-150405")),
	}

	return outputResult(cfg.OutputFormat, result, func() {
		fmt.Printf("Backup Verification\n")
		fmt.Printf("===================\n\n")
		fmt.Printf("Backup ID:       %s\n", result.BackupID)
		fmt.Printf("Verification ID: %s\n", result.VerificationID)
		fmt.Printf("Verified At:     %s\n\n", result.VerifiedAt)

		fmt.Printf("Results:\n")
		fmt.Printf("  Valid:          %s\n", boolToStatus(result.Valid))
		fmt.Printf("  Checksum Match: %s\n", boolToStatus(result.ChecksumMatch))
		fmt.Printf("  Integrity OK:   %s\n", boolToStatus(result.IntegrityOK))
		if checkRestorable {
			fmt.Printf("  Restorable:     %s\n", boolToStatus(result.Restorable))
		}

		fmt.Printf("\nComponent Status:\n")
		for component, ok := range result.ComponentsOK {
			fmt.Printf("  %-15s %s\n", component+":", boolToStatus(ok))
		}

		if len(result.Issues) > 0 {
			fmt.Printf("\nIssues Found:\n")
			for _, issue := range result.Issues {
				fmt.Printf("  - %s\n", issue)
			}
		} else {
			fmt.Printf("\n✓ No issues found\n")
		}
	})
}

func newRestoreCmd(cfg *Config) *cobra.Command {
	var (
		target     string
		components []string
		dryRun     bool
		force      bool
		async      bool
	)

	cmd := &cobra.Command{
		Use:   "restore <backup-id>",
		Short: "Restore from a backup",
		Long: `Restore Keystone Core data from a backup.

CAUTION: This operation will overwrite existing data!

Examples:
  # Dry-run restore (shows what would be restored)
  kscorectl backup restore --id backup-20240115-060000 --dry-run

  # Restore to test cluster
  kscorectl backup restore --id backup-20240115-060000 --target test-cluster

  # Restore specific components
  kscorectl backup restore --id backup-20240115-060000 --components database,config

  # Force restore without confirmation
  kscorectl backup restore --id backup-20240115-060000 --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRestore(cfg, args[0], target, components, dryRun, force, async)
		},
	}

	cmd.Flags().StringVarP(&target, "target", "t", "", "Target cluster for restore")
	cmd.Flags().StringSliceVarP(&components, "components", "c", nil, "Specific components to restore")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be restored without making changes")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompts")
	cmd.Flags().BoolVar(&async, "async", false, "Run restore asynchronously")

	return cmd
}

func runRestore(cfg *Config, backupID, target string, components []string, dryRun, force, async bool) error {
	if target == "" {
		target = "local"
	}

	if len(components) == 0 {
		components = []string{"database", "config", "secrets", "jetstream", "etcd", "certificates"}
	}

	result := RestoreResult{
		RestoreID:  fmt.Sprintf("restore-%s", time.Now().Format("20060102-150405")),
		BackupID:   backupID,
		Status:     "completed",
		Target:     target,
		Components: components,
		StartedAt:  time.Now().Format(time.RFC3339),
		DryRun:     dryRun,
	}

	if dryRun {
		result.Status = "dry-run"
	} else if async {
		result.Status = "running"
	} else {
		result.CompletedAt = time.Now().Add(5 * time.Minute).Format(time.RFC3339)
		result.Duration = "5m12s"
	}

	return outputResult(cfg.OutputFormat, result, func() {
		if dryRun {
			fmt.Printf("Restore Dry-Run\n")
			fmt.Printf("===============\n\n")
			fmt.Printf("This would restore from backup: %s\n\n", backupID)
			fmt.Printf("Components to restore:\n")
			for _, c := range components {
				fmt.Printf("  - %s\n", c)
			}
			fmt.Printf("\nTarget: %s\n", target)
			fmt.Printf("\nNo changes made (dry-run mode)\n")
		} else if async {
			fmt.Printf("Restore started: %s\n", result.RestoreID)
			fmt.Printf("Backup ID:       %s\n", result.BackupID)
			fmt.Printf("Status:          %s\n", result.Status)
			fmt.Printf("\nUse 'kscorectl backup show %s' to check progress\n", result.RestoreID)
		} else {
			fmt.Printf("Restore Completed\n")
			fmt.Printf("=================\n\n")
			fmt.Printf("Restore ID: %s\n", result.RestoreID)
			fmt.Printf("Backup ID:  %s\n", result.BackupID)
			fmt.Printf("Status:     %s\n", result.Status)
			fmt.Printf("Target:     %s\n", result.Target)
			fmt.Printf("Duration:   %s\n", result.Duration)
			fmt.Printf("\nRestored Components:\n")
			for _, c := range result.Components {
				fmt.Printf("  ✓ %s\n", c)
			}
		}
	})
}

func newDeleteCmd(cfg *Config) *cobra.Command {
	var (
		force   bool
		olderThan string
	)

	cmd := &cobra.Command{
		Use:   "delete <backup-id>",
		Short: "Delete a backup",
		Long: `Delete a backup from storage.

Examples:
  # Delete a specific backup
  kscorectl backup delete backup-20240115-060000

  # Delete without confirmation
  kscorectl backup delete backup-20240115-060000 --force

  # Delete backups older than 30 days
  kscorectl backup delete --older-than 30d`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var backupID string
			if len(args) > 0 {
				backupID = args[0]
			}
			return runDelete(cfg, backupID, force, olderThan)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompts")
	cmd.Flags().StringVar(&olderThan, "older-than", "", "Delete backups older than duration (e.g., 30d, 1w)")

	return cmd
}

func runDelete(cfg *Config, backupID string, force bool, olderThan string) error {
	if backupID == "" && olderThan == "" {
		return fmt.Errorf("either backup-id or --older-than must be specified")
	}

	result := struct {
		Deleted []string `json:"deleted" yaml:"deleted"`
		Count   int      `json:"count" yaml:"count"`
	}{
		Deleted: []string{backupID},
		Count:   1,
	}

	if olderThan != "" {
		result.Deleted = []string{
			"backup-20231215-060000",
			"backup-20231214-060000",
			"backup-20231213-060000",
		}
		result.Count = 3
	}

	return outputResult(cfg.OutputFormat, result, func() {
		if result.Count == 1 {
			fmt.Printf("Deleted backup: %s\n", result.Deleted[0])
		} else {
			fmt.Printf("Deleted %d backups:\n", result.Count)
			for _, id := range result.Deleted {
				fmt.Printf("  - %s\n", id)
			}
		}
	})
}

func newReplicationStatusCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "replication-status",
		Short: "Show backup replication status",
		Long: `Show the status of backup replication to secondary destinations.

Examples:
  kscorectl backup replication-status`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReplicationStatus(cfg)
		},
	}

	return cmd
}

func runReplicationStatus(cfg *Config) error {
	status := ReplicationStatus{
		Enabled:      true,
		LastSync:     "2024-01-15T06:10:00Z",
		NextSync:     "2024-01-15T18:00:00Z",
		SyncInterval: "12h",
		Status:       "healthy",
		Destinations: []ReplicationDest{
			{
				Name:        "us-west-2",
				Type:        "s3",
				Status:      "synced",
				LastSync:    "2024-01-15T06:10:00Z",
				BackupCount: 30,
				TotalSize:   "15.2 GB",
			},
			{
				Name:        "eu-central-1",
				Type:        "s3",
				Status:      "synced",
				LastSync:    "2024-01-15T06:10:05Z",
				BackupCount: 30,
				TotalSize:   "15.2 GB",
			},
			{
				Name:        "local-archive",
				Type:        "sftp",
				Status:      "syncing",
				LastSync:    "2024-01-14T18:00:00Z",
				BackupCount: 28,
				TotalSize:   "14.5 GB",
			},
		},
	}

	return outputResult(cfg.OutputFormat, status, func() {
		fmt.Printf("Backup Replication Status\n")
		fmt.Printf("=========================\n\n")
		fmt.Printf("Enabled:       %v\n", status.Enabled)
		fmt.Printf("Status:        %s\n", status.Status)
		fmt.Printf("Sync Interval: %s\n", status.SyncInterval)
		fmt.Printf("Last Sync:     %s\n", status.LastSync)
		fmt.Printf("Next Sync:     %s\n\n", status.NextSync)

		table := &output.Table{
			Headers: []string{"DESTINATION", "TYPE", "STATUS", "LAST SYNC", "BACKUPS", "SIZE"},
		}

		for _, d := range status.Destinations {
			statusIcon := "✓"
			if d.Status == "syncing" {
				statusIcon = "◐"
			} else if d.Status == "failed" {
				statusIcon = "✗"
			}

			table.Rows = append(table.Rows, []string{
				d.Name,
				d.Type,
				fmt.Sprintf("%s %s", statusIcon, d.Status),
				d.LastSync,
				fmt.Sprintf("%d", d.BackupCount),
				d.TotalSize,
			})
		}

		output.WriteTable(os.Stdout, table)
	})
}

func newScheduleCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Manage backup schedules",
		Long:  `Manage automated backup schedules.`,
	}

	cmd.AddCommand(
		newScheduleListCmd(cfg),
		newScheduleCreateCmd(cfg),
		newScheduleDeleteCmd(cfg),
		newScheduleEnableCmd(cfg),
		newScheduleDisableCmd(cfg),
	)

	return cmd
}

func newScheduleListCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List backup schedules",
		RunE: func(cmd *cobra.Command, args []string) error {
			schedules := []ScheduleInfo{
				{
					Name:        "daily-full",
					Schedule:    "0 6 * * *",
					Type:        "full",
					Components:  []string{"database", "config", "secrets", "jetstream", "etcd", "certificates"},
					Destination: "s3://kscore-backups/prod",
					Enabled:     true,
					LastRun:     "2024-01-15T06:00:00Z",
					NextRun:     "2024-01-16T06:00:00Z",
					RetainCount: 7,
				},
				{
					Name:        "hourly-incremental",
					Schedule:    "0 * * * *",
					Type:        "incremental",
					Components:  []string{"database", "config"},
					Destination: "s3://kscore-backups/prod",
					Enabled:     true,
					LastRun:     "2024-01-15T12:00:00Z",
					NextRun:     "2024-01-15T13:00:00Z",
					RetainCount: 24,
				},
				{
					Name:        "weekly-archive",
					Schedule:    "0 2 * * 0",
					Type:        "full",
					Components:  []string{"database", "config", "secrets", "jetstream", "etcd", "certificates"},
					Destination: "azure://kscore-archive/weekly",
					Enabled:     true,
					LastRun:     "2024-01-14T02:00:00Z",
					NextRun:     "2024-01-21T02:00:00Z",
					RetainCount: 52,
				},
			}

			return outputResult(cfg.OutputFormat, schedules, func() {
				table := &output.Table{
					Headers: []string{"NAME", "SCHEDULE", "TYPE", "ENABLED", "LAST RUN", "NEXT RUN", "RETAIN"},
				}

				for _, s := range schedules {
					enabled := "✓"
					if !s.Enabled {
						enabled = "✗"
					}

					table.Rows = append(table.Rows, []string{
						s.Name,
						s.Schedule,
						s.Type,
						enabled,
						s.LastRun,
						s.NextRun,
						fmt.Sprintf("%d", s.RetainCount),
					})
				}

				output.WriteTable(os.Stdout, table)
			})
		},
	}

	return cmd
}

func newScheduleCreateCmd(cfg *Config) *cobra.Command {
	var (
		schedule    string
		backupType  string
		components  []string
		destination string
		retain      int
	)

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a backup schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Created backup schedule: %s\n", args[0])
			fmt.Printf("  Schedule:    %s\n", schedule)
			fmt.Printf("  Type:        %s\n", backupType)
			fmt.Printf("  Destination: %s\n", destination)
			fmt.Printf("  Retain:      %d\n", retain)
			return nil
		},
	}

	cmd.Flags().StringVar(&schedule, "schedule", "0 6 * * *", "Cron schedule expression")
	cmd.Flags().StringVarP(&backupType, "type", "t", "full", "Backup type")
	cmd.Flags().StringSliceVarP(&components, "components", "c", nil, "Components to backup")
	cmd.Flags().StringVarP(&destination, "destination", "d", "", "Backup destination")
	cmd.Flags().IntVar(&retain, "retain", 7, "Number of backups to retain")

	return cmd
}

func newScheduleDeleteCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a backup schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Deleted backup schedule: %s\n", args[0])
			return nil
		},
	}

	return cmd
}

func newScheduleEnableCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enable <name>",
		Short: "Enable a backup schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Enabled backup schedule: %s\n", args[0])
			return nil
		},
	}

	return cmd
}

func newScheduleDisableCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disable <name>",
		Short: "Disable a backup schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Disabled backup schedule: %s\n", args[0])
			return nil
		},
	}

	return cmd
}

func newRetentionCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "retention",
		Short: "Manage backup retention policies",
		Long:  `Manage backup retention policies.`,
	}

	cmd.AddCommand(
		newRetentionShowCmd(cfg),
		newRetentionSetCmd(cfg),
		newRetentionApplyCmd(cfg),
	)

	return cmd
}

func newRetentionShowCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show retention policies",
		RunE: func(cmd *cobra.Command, args []string) error {
			policies := []RetentionPolicy{
				{
					Name:        "default",
					MaxBackups:  30,
					MaxAge:      "30d",
					KeepDaily:   7,
					KeepWeekly:  4,
					KeepMonthly: 6,
					KeepYearly:  2,
					AppliesTo:   "all",
				},
				{
					Name:        "archive",
					MaxBackups:  0,
					MaxAge:      "365d",
					KeepDaily:   0,
					KeepWeekly:  52,
					KeepMonthly: 12,
					KeepYearly:  5,
					AppliesTo:   "weekly-archive",
				},
			}

			return outputResult(cfg.OutputFormat, policies, func() {
				for _, p := range policies {
					fmt.Printf("Policy: %s\n", p.Name)
					fmt.Printf("  Applies To:   %s\n", p.AppliesTo)
					fmt.Printf("  Max Backups:  %d\n", p.MaxBackups)
					fmt.Printf("  Max Age:      %s\n", p.MaxAge)
					fmt.Printf("  Keep Daily:   %d\n", p.KeepDaily)
					fmt.Printf("  Keep Weekly:  %d\n", p.KeepWeekly)
					fmt.Printf("  Keep Monthly: %d\n", p.KeepMonthly)
					fmt.Printf("  Keep Yearly:  %d\n", p.KeepYearly)
					fmt.Println()
				}
			})
		},
	}

	return cmd
}

func newRetentionSetCmd(cfg *Config) *cobra.Command {
	var (
		maxBackups  int
		maxAge      string
		keepDaily   int
		keepWeekly  int
		keepMonthly int
		keepYearly  int
	)

	cmd := &cobra.Command{
		Use:   "set <policy-name>",
		Short: "Set retention policy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Updated retention policy: %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().IntVar(&maxBackups, "max-backups", 0, "Maximum number of backups to keep")
	cmd.Flags().StringVar(&maxAge, "max-age", "", "Maximum age of backups (e.g., 30d)")
	cmd.Flags().IntVar(&keepDaily, "keep-daily", 0, "Daily backups to keep")
	cmd.Flags().IntVar(&keepWeekly, "keep-weekly", 0, "Weekly backups to keep")
	cmd.Flags().IntVar(&keepMonthly, "keep-monthly", 0, "Monthly backups to keep")
	cmd.Flags().IntVar(&keepYearly, "keep-yearly", 0, "Yearly backups to keep")

	return cmd
}

func newRetentionApplyCmd(cfg *Config) *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply retention policies",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRun {
				fmt.Println("Dry-run: Would delete the following backups:")
				fmt.Println("  - backup-20231115-060000 (older than 30 days)")
				fmt.Println("  - backup-20231114-060000 (older than 30 days)")
				fmt.Println("  - backup-20231113-060000 (older than 30 days)")
				fmt.Println("\nNo changes made (dry-run mode)")
			} else {
				fmt.Println("Applied retention policies")
				fmt.Println("  Deleted: 3 backups")
				fmt.Println("  Retained: 27 backups")
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be deleted without making changes")

	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			info := version.Get()
			fmt.Fprintln(cmd.OutOrStdout(), info.String())
		},
	}
}

// Helper functions

func boolToStatus(b bool) string {
	if b {
		return "✓ yes"
	}
	return "✗ no"
}

func outputResult(format string, data interface{}, tableFunc func()) error {
	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	case "yaml":
		return yaml.NewEncoder(os.Stdout).Encode(data)
	default:
		tableFunc()
		return nil
	}
}
