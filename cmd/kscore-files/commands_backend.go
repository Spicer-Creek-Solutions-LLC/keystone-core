// Package main implements backend management commands for kscore-files.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/pkg/cli/output"
	"github.com/shawnbutts/keystone-core/pkg/files"
)

// newBackendCmd creates the backend command group.
func newBackendCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backend",
		Short: "Manage storage backends",
		Long:  `Commands for managing storage backends (status, sync, enable/disable).`,
	}

	cmd.AddCommand(newBackendListCmd())
	cmd.AddCommand(newBackendStatusCmd())
	cmd.AddCommand(newBackendSyncCmd())
	cmd.AddCommand(newBackendEnableCmd())
	cmd.AddCommand(newBackendDisableCmd())
	cmd.AddCommand(newBackendHealthCmd())

	return cmd
}

// newBackendListCmd creates the list command.
func newBackendListCmd() *cobra.Command {
	var outputFmt string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List configured backends",
		Long: `List all configured storage backends and their status.

Examples:
  kscore-files backend list
  kscore-files backend list --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutAdmin)
			defer cancel()

			backends, err := client.ListBackends(ctx)
			if err != nil {
				return fmt.Errorf("failed to list backends: %w", err)
			}

			format, err := output.ParseFormat(outputFmt)
			if err != nil {
				return err
			}

			switch format {
			case output.FormatJSON:
				return output.WriteJSON(os.Stdout, backends)
			case output.FormatYAML:
				return output.WriteYAML(os.Stdout, backends)
			case output.FormatTable, output.FormatText:
				if len(backends) == 0 {
					fmt.Println("No backends configured")
					return nil
				}

				rows := make([][]string, 0, len(backends))
				for _, b := range backends {
					status := "enabled"
					if !b.Enabled {
						status = "disabled"
					}
					readOnly := "no"
					if b.ReadOnly {
						readOnly = "yes"
					}
					paths := strings.Join(b.Paths, ", ")
					if len(paths) > 30 {
						paths = paths[:27] + "..."
					}
					rows = append(rows, []string{b.Name, b.Type, status, readOnly, paths})
				}

				table := &output.Table{
					Headers: []string{"NAME", "TYPE", "STATUS", "READONLY", "PATHS"},
					Rows:    rows,
				}
				return output.WriteTable(os.Stdout, table)
			default:
				return fmt.Errorf("unsupported output format: %s", outputFmt)
			}
		},
	}

	cmd.Flags().StringVarP(&outputFmt, "output", "o", "table", "Output format (table, text, json, yaml)")

	return cmd
}

// newBackendStatusCmd creates the status command.
func newBackendStatusCmd() *cobra.Command {
	var outputFmt string

	cmd := &cobra.Command{
		Use:   "status <name>",
		Short: "Get backend status",
		Long: `Get detailed status information for a storage backend.

Examples:
  kscore-files backend status local
  kscore-files backend status s3-primary --output json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutAdmin)
			defer cancel()

			status, err := client.GetBackendStatus(ctx, name)
			if err != nil {
				return fmt.Errorf("failed to get backend status: %w", err)
			}

			format, err := output.ParseFormat(outputFmt)
			if err != nil {
				return err
			}

			switch format {
			case output.FormatJSON:
				return output.WriteJSON(os.Stdout, status)
			case output.FormatYAML:
				return output.WriteYAML(os.Stdout, status)
			case output.FormatTable:
				table := buildKeyValueTable([][2]string{
					{"BACKEND", status.Name},
					{"TYPE", status.Type},
					{"STATUS", formatStatus(status.Healthy)},
					{"ENABLED", fmt.Sprintf("%t", status.Enabled)},
					{"READ-ONLY", fmt.Sprintf("%t", status.ReadOnly)},
					{"LAST ERROR", status.LastError},
					{"LAST ERROR TIME", formatTime(status.LastErrorTime)},
					{"LAST SYNC", formatTime(status.LastSync)},
				})
				if err := output.WriteTable(os.Stdout, table); err != nil {
					return err
				}
				stats := buildKeyValueTable([][2]string{
					{"FILES", fmt.Sprintf("%d", status.Stats.FileCount)},
					{"TOTAL SIZE", formatSize(status.Stats.TotalSize)},
					{"READS", fmt.Sprintf("%d (%s)", status.Stats.ReadCount, formatSize(status.Stats.BytesRead))},
					{"WRITES", fmt.Sprintf("%d (%s)", status.Stats.WriteCount, formatSize(status.Stats.BytesWritten))},
					{"ERRORS", fmt.Sprintf("%d", status.Stats.ErrorCount)},
				})
				fmt.Println("\nStatistics:")
				return output.WriteTable(os.Stdout, stats)
			case output.FormatText:
				fmt.Printf("Backend: %s\n", status.Name)
				fmt.Printf("Type: %s\n", status.Type)
				fmt.Printf("Status: %s\n", formatStatus(status.Healthy))
				fmt.Printf("Enabled: %v\n", status.Enabled)
				fmt.Printf("Read-Only: %v\n", status.ReadOnly)
				fmt.Println()
				fmt.Printf("Statistics:\n")
				fmt.Printf("  Files: %d\n", status.Stats.FileCount)
				fmt.Printf("  Total Size: %s\n", formatSize(status.Stats.TotalSize))
				fmt.Printf("  Reads: %d (%s)\n", status.Stats.ReadCount, formatSize(status.Stats.BytesRead))
				fmt.Printf("  Writes: %d (%s)\n", status.Stats.WriteCount, formatSize(status.Stats.BytesWritten))
				fmt.Printf("  Errors: %d\n", status.Stats.ErrorCount)
				fmt.Println()
				if status.LastError != "" {
					fmt.Printf("Last Error: %s (at %s)\n", status.LastError, status.LastErrorTime.Format(time.RFC3339))
				}
				if !status.LastSync.IsZero() {
					fmt.Printf("Last Sync: %s\n", status.LastSync.Format(time.RFC3339))
				}
				return nil
			default:
				return fmt.Errorf("unsupported output format: %s", outputFmt)
			}
		},
	}

	cmd.Flags().StringVarP(&outputFmt, "output", "o", "text", "Output format (text, json, yaml, table)")

	return cmd
}

// newBackendSyncCmd creates the sync command.
func newBackendSyncCmd() *cobra.Command {
	var (
		dryRun bool
		force  bool
	)

	cmd := &cobra.Command{
		Use:   "sync <source> <destination>",
		Short: "Synchronize backends",
		Long: `Synchronize files between two storage backends.

Examples:
  kscore-files backend sync local s3-backup
  kscore-files backend sync s3-primary local --dry-run
  kscore-files backend sync local s3-archive --force`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			source := args[0]
			dest := args[1]

			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutAdminSync)
			defer cancel()

			opts := &files.BackendSyncOptions{
				DryRun: dryRun,
				Force:  force,
			}

			if dryRun {
				fmt.Println("Calculating backend sync plan...")
			} else {
				fmt.Println("Syncing backends...")
			}

			result, err := client.SyncBackends(ctx, source, dest, opts)
			if err != nil {
				return fmt.Errorf("failed to sync backends: %w", err)
			}

			if dryRun {
				fmt.Println("Dry run - would perform the following:")
			}

			fmt.Printf("Files to copy: %d (%s)\n", result.FilesCopied, formatSize(result.BytesCopied))
			fmt.Printf("Files to delete: %d\n", result.FilesDeleted)
			fmt.Printf("Files unchanged: %d\n", result.FilesUnchanged)

			if result.Errors > 0 {
				fmt.Printf("Errors: %d\n", result.Errors)
			}

			if !dryRun {
				fmt.Printf("\nSync completed in %s\n", result.Duration)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done")
	cmd.Flags().BoolVar(&force, "force", false, "Force sync even if destination has newer files")

	return cmd
}

// newBackendEnableCmd creates the enable command.
func newBackendEnableCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "enable <name>",
		Short: "Enable a backend",
		Long: `Enable a storage backend for serving files.

Examples:
  kscore-files backend enable s3-backup`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if dryRun {
				fmt.Printf("Dry run: would enable backend %s\n", name)
				return nil
			}

			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutAdmin)
			defer cancel()

			if err := client.SetBackendEnabled(ctx, name, true); err != nil {
				return fmt.Errorf("failed to enable backend: %w", err)
			}

			fmt.Printf("Backend %s enabled\n", name)
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without enabling")

	return cmd
}

// newBackendDisableCmd creates the disable command.
func newBackendDisableCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "disable <name>",
		Short: "Disable a backend",
		Long: `Disable a storage backend (stops serving files from it).

Examples:
  kscore-files backend disable s3-archive`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if dryRun {
				fmt.Printf("Dry run: would disable backend %s\n", name)
				return nil
			}

			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutAdmin)
			defer cancel()

			if err := client.SetBackendEnabled(ctx, name, false); err != nil {
				return fmt.Errorf("failed to disable backend: %w", err)
			}

			fmt.Printf("Backend %s disabled\n", name)
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without disabling")

	return cmd
}

// newBackendHealthCmd creates the health command.
func newBackendHealthCmd() *cobra.Command {
	var outputFmt string

	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check health of all backends",
		Long: `Check the health status of all configured storage backends.

Examples:
  kscore-files backend health
  kscore-files backend health --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutAdmin)
			defer cancel()

			health, err := client.CheckBackendHealth(ctx)
			if err != nil {
				return fmt.Errorf("failed to check backend health: %w", err)
			}

			format, err := output.ParseFormat(outputFmt)
			if err != nil {
				return err
			}

			switch format {
			case output.FormatJSON:
				return output.WriteJSON(os.Stdout, health)
			case output.FormatYAML:
				return output.WriteYAML(os.Stdout, health)
			case output.FormatTable, output.FormatText:
				fmt.Printf("%-20s %-10s %-15s %s\n", "BACKEND", "STATUS", "LATENCY", "MESSAGE")
				fmt.Println(strings.Repeat("-", 70))

				allHealthy := true
				for _, h := range health {
					status := "healthy"
					if !h.Healthy {
						status = "unhealthy"
						allHealthy = false
					}
					latency := fmt.Sprintf("%dms", h.Latency.Milliseconds())
					message := h.Message
					if len(message) > 30 {
						message = message[:27] + "..."
					}
					fmt.Printf("%-20s %-10s %-15s %s\n", h.Name, status, latency, message)
				}

				fmt.Println()
				if allHealthy {
					fmt.Println("All backends healthy")
				} else {
					fmt.Println("Some backends are unhealthy")
					return fmt.Errorf("unhealthy backends detected")
				}
				return nil
			default:
				return fmt.Errorf("unsupported output format: %s", outputFmt)
			}
		},
	}

	cmd.Flags().StringVarP(&outputFmt, "output", "o", "table", "Output format (table, text, json, yaml)")

	return cmd
}

// AdminClient provides admin operations for the file server.
type AdminClient struct {
	nc        *nats.Conn
	clusterID string
}

// BackendInfo contains information about a backend.
type BackendInfo struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Enabled  bool     `json:"enabled"`
	ReadOnly bool     `json:"read_only"`
	Paths    []string `json:"paths"`
}

// BackendStatus contains detailed status of a backend.
type BackendStatus struct {
	Name          string       `json:"name"`
	Type          string       `json:"type"`
	Enabled       bool         `json:"enabled"`
	ReadOnly      bool         `json:"read_only"`
	Healthy       bool         `json:"healthy"`
	Stats         BackendStats `json:"stats"`
	LastError     string       `json:"last_error,omitempty"`
	LastErrorTime time.Time    `json:"last_error_time,omitempty"`
	LastSync      time.Time    `json:"last_sync,omitempty"`
}

// BackendStats contains statistics for a backend.
type BackendStats struct {
	FileCount    int64 `json:"file_count"`
	TotalSize    int64 `json:"total_size"`
	ReadCount    int64 `json:"read_count"`
	WriteCount   int64 `json:"write_count"`
	BytesRead    int64 `json:"bytes_read"`
	BytesWritten int64 `json:"bytes_written"`
	ErrorCount   int64 `json:"error_count"`
}

// BackendHealth contains health information for a backend.
type BackendHealth struct {
	Name    string        `json:"name"`
	Healthy bool          `json:"healthy"`
	Latency time.Duration `json:"latency"`
	Message string        `json:"message,omitempty"`
}

func createAdminClient() (*AdminClient, func(), error) {
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to NATS (check --nats-url or server availability): %w", err)
	}

	client := &AdminClient{
		nc:        nc,
		clusterID: clusterID,
	}

	cleanup := func() {
		nc.Close()
	}

	return client, cleanup, nil
}

// ListBackends returns all configured backends.
func (c *AdminClient) ListBackends(ctx context.Context) ([]BackendInfo, error) {
	subject := fmt.Sprintf("kscore.%s.files.admin.backends.list", c.clusterID)

	msg, err := c.nc.RequestWithContext(ctx, subject, nil)
	if err != nil {
		return nil, err
	}

	var backends []BackendInfo
	if err := json.Unmarshal(msg.Data, &backends); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return backends, nil
}

// GetBackendStatus returns the status of a specific backend.
func (c *AdminClient) GetBackendStatus(ctx context.Context, name string) (*BackendStatus, error) {
	subject := fmt.Sprintf("kscore.%s.files.admin.backends.status", c.clusterID)

	req := map[string]string{"name": name}
	reqData, _ := json.Marshal(req)

	msg, err := c.nc.RequestWithContext(ctx, subject, reqData)
	if err != nil {
		return nil, err
	}

	var status BackendStatus
	if err := json.Unmarshal(msg.Data, &status); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &status, nil
}

// SyncBackends synchronizes files between two backends.
func (c *AdminClient) SyncBackends(ctx context.Context, source, dest string, opts *files.BackendSyncOptions) (*files.BackendSyncResult, error) {
	subject := fmt.Sprintf("kscore.%s.files.admin.backends.sync", c.clusterID)

	req := map[string]interface{}{
		"source":      source,
		"destination": dest,
		"dry_run":     opts.DryRun,
		"force":       opts.Force,
	}
	reqData, _ := json.Marshal(req)

	msg, err := c.nc.RequestWithContext(ctx, subject, reqData)
	if err != nil {
		return nil, err
	}

	var result files.BackendSyncResult
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

// SetBackendEnabled enables or disables a backend.
func (c *AdminClient) SetBackendEnabled(ctx context.Context, name string, enabled bool) error {
	subject := fmt.Sprintf("kscore.%s.files.admin.backends.enable", c.clusterID)

	req := map[string]interface{}{
		"name":    name,
		"enabled": enabled,
	}
	reqData, _ := json.Marshal(req)

	_, err := c.nc.RequestWithContext(ctx, subject, reqData)
	return err
}

// CheckBackendHealth checks the health of all backends.
func (c *AdminClient) CheckBackendHealth(ctx context.Context) ([]BackendHealth, error) {
	subject := fmt.Sprintf("kscore.%s.files.admin.backends.health", c.clusterID)

	msg, err := c.nc.RequestWithContext(ctx, subject, nil)
	if err != nil {
		return nil, err
	}

	var health []BackendHealth
	if err := json.Unmarshal(msg.Data, &health); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return health, nil
}

func formatStatus(healthy bool) string {
	if healthy {
		return "healthy"
	}
	return "unhealthy"
}
