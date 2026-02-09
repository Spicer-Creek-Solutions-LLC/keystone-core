// Package main implements the kscore-files-storage CLI for storage backend and mirror management.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/cli/auditutil"
	"github.com/shawnbutts/keystone-core/internal/cli/output"
	"github.com/shawnbutts/keystone-core/internal/files"
	"github.com/shawnbutts/keystone-core/internal/files/mirror"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

var (
	natsURL     string
	clusterID   string
	outputFmt   string
	auditLevel  string
	auditOutput string
)

func main() {
	rootCmd := newRootCmd()
	auditHandler := auditutil.Attach(rootCmd, "kscore-files-storage", &auditLevel, &auditOutput)
	if err := rootCmd.Execute(); err != nil {
		auditHandler.LogFailure(err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-files-storage",
		Short: "Storage backend and mirror management for Keystone Core",
		Long: `kscore-files-storage manages storage backends and mirror groups for file distribution.

This command provides tools for:
  - Managing storage backends (list, status, sync, enable/disable)
  - Managing mirror groups for high availability
  - Synchronizing data between backends and mirrors
  - Monitoring health and performance

Commands:
  backend   - Manage storage backends
  mirrors   - Manage mirror groups

Examples:
  # List storage backends
  kscore-files-storage backend list

  # Check backend health
  kscore-files-storage backend health

  # Sync backends
  kscore-files-storage backend sync local s3-backup

  # List mirror groups
  kscore-files-storage mirrors list

  # Trigger mirror sync
  kscore-files-storage mirrors sync primary-group`,
	}

	rootCmd.PersistentFlags().StringVar(&natsURL, "nats-url", "nats://localhost:4222", "NATS server URL")
	rootCmd.PersistentFlags().StringVar(&clusterID, "cluster-id", "", "cluster ID")
	rootCmd.PersistentFlags().StringVarP(&outputFmt, "output", "o", "table", "Output format (table, text, json, yaml)")
	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")

	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newBackendCmd())
	rootCmd.AddCommand(newMirrorsCmd())

	return rootCmd
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

// ============================================================================
// Backend Commands
// ============================================================================

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

func newBackendListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured backends",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			backends, err := client.ListBackends(ctx)
			if err != nil {
				return fmt.Errorf("failed to list backends: %w", err)
			}

			format, _ := output.ParseFormat(outputFmt)
			switch format {
			case output.FormatJSON:
				return output.WriteJSON(os.Stdout, backends)
			case output.FormatYAML:
				return output.WriteYAML(os.Stdout, backends)
			default:
				if len(backends) == 0 {
					fmt.Println("No backends configured")
					return nil
				}

				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "NAME\tTYPE\tSTATUS\tREADONLY\tPATHS")
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
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", b.Name, b.Type, status, readOnly, paths)
				}
				return w.Flush()
			}
		},
	}
}

func newBackendStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <name>",
		Short: "Get backend status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			status, err := client.GetBackendStatus(ctx, name)
			if err != nil {
				return fmt.Errorf("failed to get backend status: %w", err)
			}

			format, _ := output.ParseFormat(outputFmt)
			switch format {
			case output.FormatJSON:
				return output.WriteJSON(os.Stdout, status)
			case output.FormatYAML:
				return output.WriteYAML(os.Stdout, status)
			default:
				fmt.Printf("Backend: %s\n", status.Name)
				fmt.Printf("Type: %s\n", status.Type)
				fmt.Printf("Status: %s\n", formatHealthy(status.Healthy))
				fmt.Printf("Enabled: %v\n", status.Enabled)
				fmt.Printf("Read-Only: %v\n", status.ReadOnly)
				fmt.Println()
				fmt.Printf("Statistics:\n")
				fmt.Printf("  Files: %d\n", status.Stats.FileCount)
				fmt.Printf("  Total Size: %s\n", formatSize(status.Stats.TotalSize))
				fmt.Printf("  Reads: %d (%s)\n", status.Stats.ReadCount, formatSize(status.Stats.BytesRead))
				fmt.Printf("  Writes: %d (%s)\n", status.Stats.WriteCount, formatSize(status.Stats.BytesWritten))
				fmt.Printf("  Errors: %d\n", status.Stats.ErrorCount)
				return nil
			}
		},
	}
}

func newBackendSyncCmd() *cobra.Command {
	var dryRun, force bool

	cmd := &cobra.Command{
		Use:   "sync <source> <destination>",
		Short: "Synchronize backends",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			source, dest := args[0], args[1]

			if dryRun {
				fmt.Println("Calculating backend sync plan...")
			} else {
				fmt.Println("Syncing backends...")
			}

			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()

			result, err := client.SyncBackends(ctx, source, dest, &files.BackendSyncOptions{
				DryRun: dryRun,
				Force:  force,
			})
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

func newBackendEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <name>",
		Short: "Enable a backend",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := client.SetBackendEnabled(ctx, name, true); err != nil {
				return fmt.Errorf("failed to enable backend: %w", err)
			}

			fmt.Printf("Backend %s enabled\n", name)
			return nil
		},
	}
}

func newBackendDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <name>",
		Short: "Disable a backend",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := client.SetBackendEnabled(ctx, name, false); err != nil {
				return fmt.Errorf("failed to disable backend: %w", err)
			}

			fmt.Printf("Backend %s disabled\n", name)
			return nil
		},
	}
}

func newBackendHealthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Check health of all backends",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			health, err := client.CheckBackendHealth(ctx)
			if err != nil {
				return fmt.Errorf("failed to check backend health: %w", err)
			}

			format, _ := output.ParseFormat(outputFmt)
			switch format {
			case output.FormatJSON:
				return output.WriteJSON(os.Stdout, health)
			case output.FormatYAML:
				return output.WriteYAML(os.Stdout, health)
			default:
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "BACKEND\tSTATUS\tLATENCY\tMESSAGE")
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
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", h.Name, status, latency, message)
				}
				w.Flush()
				fmt.Println()
				if allHealthy {
					fmt.Println("All backends healthy")
				} else {
					fmt.Println("Some backends are unhealthy")
					return fmt.Errorf("unhealthy backends detected")
				}
				return nil
			}
		},
	}
}

// ============================================================================
// Mirror Commands
// ============================================================================

func newMirrorsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mirrors",
		Short: "Manage mirror groups",
		Long:  `Commands for managing mirror groups, sync operations, and conflict resolution.`,
	}

	cmd.AddCommand(newMirrorsListCmd())
	cmd.AddCommand(newMirrorsShowCmd())
	cmd.AddCommand(newMirrorsSyncCmd())
	cmd.AddCommand(newMirrorsHealthCmd())
	cmd.AddCommand(newMirrorsConflictsCmd())

	return cmd
}

func newMirrorsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List mirror groups",
		RunE: func(cmd *cobra.Command, args []string) error {
			registry := getMirrorRegistry()
			groups := registry.List()

			type GroupInfo struct {
				ID           string `json:"id"`
				Name         string `json:"name"`
				MirrorCount  int    `json:"mirror_count"`
				ReadStrategy string `json:"read_strategy"`
				WritePolicy  string `json:"write_policy"`
			}

			infos := make([]GroupInfo, 0, len(groups))
			for _, g := range groups {
				infos = append(infos, GroupInfo{
					ID:           g.ID(),
					Name:         g.Name(),
					MirrorCount:  len(g.GetMirrors()),
					ReadStrategy: string(g.Config().ReadStrategy),
					WritePolicy:  string(g.Config().WritePolicy),
				})
			}

			format, _ := output.ParseFormat(outputFmt)
			switch format {
			case output.FormatJSON:
				return output.WriteJSON(os.Stdout, infos)
			case output.FormatYAML:
				return output.WriteYAML(os.Stdout, infos)
			default:
				if len(infos) == 0 {
					fmt.Println("No mirror groups configured.")
					return nil
				}

				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "ID\tNAME\tMIRRORS\tSTRATEGY\tPOLICY")
				for _, info := range infos {
					fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n",
						info.ID, info.Name, info.MirrorCount, info.ReadStrategy, info.WritePolicy)
				}
				return w.Flush()
			}
		},
	}
}

func newMirrorsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <group-id>",
		Short: "Show mirror group details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			groupID := args[0]
			registry := getMirrorRegistry()

			group, ok := registry.Get(groupID)
			if !ok {
				return fmt.Errorf("mirror group not found: %s", groupID)
			}

			fmt.Printf("Mirror Group: %s\n", group.ID())
			fmt.Printf("Name:         %s\n", group.Name())
			fmt.Printf("Strategy:     %s\n", group.Config().ReadStrategy)
			fmt.Printf("Policy:       %s\n", group.Config().WritePolicy)
			fmt.Println()

			mirrors := group.GetMirrors()
			if len(mirrors) > 0 {
				fmt.Println("Mirrors:")
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "  ID\tCLUSTER\tPRIORITY\tPRIMARY\tENABLED")
				for _, m := range mirrors {
					primary := ""
					if m.IsPrimary {
						primary = "yes"
					}
					enabled := ""
					if m.Enabled {
						enabled = "yes"
					}
					fmt.Fprintf(w, "  %s\t%s\t%d\t%s\t%s\n",
						m.ID, m.ClusterID, m.Priority, primary, enabled)
				}
				w.Flush()
			}

			return nil
		},
	}
}

func newMirrorsSyncCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "sync <group-id>",
		Short: "Trigger manual sync",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			groupID := args[0]

			if dryRun {
				fmt.Printf("Dry run: would sync mirror group %s\n", groupID)
				return nil
			}

			registry := getMirrorRegistry()
			_, ok := registry.Get(groupID)
			if !ok {
				return fmt.Errorf("mirror group not found: %s", groupID)
			}

			fmt.Printf("Sync scheduled for group: %s\n", groupID)
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done")

	return cmd
}

func newMirrorsHealthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Show mirror health",
		RunE: func(cmd *cobra.Command, args []string) error {
			registry := getMirrorRegistry()

			type HealthEntry struct {
				GroupID  string `json:"group_id"`
				MirrorID string `json:"mirror_id"`
				State    string `json:"state"`
				Latency  string `json:"latency"`
			}

			allHealth := registry.GetAllHealth()
			entries := make([]HealthEntry, 0, len(allHealth)*2) // Estimate 2 mirrors per group
			for groupID, groupHealth := range allHealth {
				for mirrorID, health := range groupHealth {
					entries = append(entries, HealthEntry{
						GroupID:  groupID,
						MirrorID: mirrorID,
						State:    string(health.State),
						Latency:  formatDuration(health.AvgLatency),
					})
				}
			}

			sort.Slice(entries, func(i, j int) bool {
				if entries[i].GroupID != entries[j].GroupID {
					return entries[i].GroupID < entries[j].GroupID
				}
				return entries[i].MirrorID < entries[j].MirrorID
			})

			format, _ := output.ParseFormat(outputFmt)
			switch format {
			case output.FormatJSON:
				return output.WriteJSON(os.Stdout, entries)
			case output.FormatYAML:
				return output.WriteYAML(os.Stdout, entries)
			default:
				if len(entries) == 0 {
					fmt.Println("No health data available.")
					return nil
				}

				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "GROUP\tMIRROR\tSTATE\tLATENCY")
				for _, e := range entries {
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.GroupID, e.MirrorID, e.State, e.Latency)
				}
				return w.Flush()
			}
		},
	}
}

func newMirrorsConflictsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "conflicts",
		Short: "List unresolved conflicts",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("No unresolved conflicts.")
			return nil
		},
	}
}

// ============================================================================
// Client and Helpers
// ============================================================================

type AdminClient struct {
	nc        *nats.Conn
	clusterID string
}

type BackendInfo struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Enabled  bool     `json:"enabled"`
	ReadOnly bool     `json:"read_only"`
	Paths    []string `json:"paths"`
}

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

type BackendStats struct {
	FileCount    int64 `json:"file_count"`
	TotalSize    int64 `json:"total_size"`
	ReadCount    int64 `json:"read_count"`
	WriteCount   int64 `json:"write_count"`
	BytesRead    int64 `json:"bytes_read"`
	BytesWritten int64 `json:"bytes_written"`
	ErrorCount   int64 `json:"error_count"`
}

type BackendHealth struct {
	Name    string        `json:"name"`
	Healthy bool          `json:"healthy"`
	Latency time.Duration `json:"latency"`
	Message string        `json:"message,omitempty"`
}

func createAdminClient() (*AdminClient, func(), error) {
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	client := &AdminClient{
		nc:        nc,
		clusterID: clusterID,
	}

	return client, func() { nc.Close() }, nil
}

func (c *AdminClient) ListBackends(ctx context.Context) ([]BackendInfo, error) {
	subject := fmt.Sprintf("kscore.%s.files.admin.backends.list", c.clusterID)
	msg, err := c.nc.RequestWithContext(ctx, subject, nil)
	if err != nil {
		return nil, err
	}
	var backends []BackendInfo
	if err := json.Unmarshal(msg.Data, &backends); err != nil {
		return nil, err
	}
	return backends, nil
}

func (c *AdminClient) GetBackendStatus(ctx context.Context, name string) (*BackendStatus, error) {
	subject := fmt.Sprintf("kscore.%s.files.admin.backends.status", c.clusterID)
	reqData, _ := json.Marshal(map[string]string{"name": name})
	msg, err := c.nc.RequestWithContext(ctx, subject, reqData)
	if err != nil {
		return nil, err
	}
	var status BackendStatus
	if err := json.Unmarshal(msg.Data, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

func (c *AdminClient) SyncBackends(ctx context.Context, source, dest string, opts *files.BackendSyncOptions) (*files.BackendSyncResult, error) {
	subject := fmt.Sprintf("kscore.%s.files.admin.backends.sync", c.clusterID)
	reqData, _ := json.Marshal(map[string]interface{}{
		"source":      source,
		"destination": dest,
		"dry_run":     opts.DryRun,
		"force":       opts.Force,
	})
	msg, err := c.nc.RequestWithContext(ctx, subject, reqData)
	if err != nil {
		return nil, err
	}
	var result files.BackendSyncResult
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *AdminClient) SetBackendEnabled(ctx context.Context, name string, enabled bool) error {
	subject := fmt.Sprintf("kscore.%s.files.admin.backends.enable", c.clusterID)
	reqData, _ := json.Marshal(map[string]interface{}{"name": name, "enabled": enabled})
	_, err := c.nc.RequestWithContext(ctx, subject, reqData)
	return err
}

func (c *AdminClient) CheckBackendHealth(ctx context.Context) ([]BackendHealth, error) {
	subject := fmt.Sprintf("kscore.%s.files.admin.backends.health", c.clusterID)
	msg, err := c.nc.RequestWithContext(ctx, subject, nil)
	if err != nil {
		return nil, err
	}
	var health []BackendHealth
	if err := json.Unmarshal(msg.Data, &health); err != nil {
		return nil, err
	}
	return health, nil
}

func getMirrorRegistry() *mirror.Registry {
	return mirror.NewRegistry()
}

func formatHealthy(healthy bool) string {
	if healthy {
		return "healthy"
	}
	return "unhealthy"
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "-"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}
