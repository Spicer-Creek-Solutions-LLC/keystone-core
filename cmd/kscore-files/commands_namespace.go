// Package main implements namespace management commands for kscore-files.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/cli/output"
)

// newNamespaceCmd creates the namespace command group.
func newNamespaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "namespace",
		Aliases: []string{"ns"},
		Short:   "Manage namespaces",
		Long:    `Commands for managing file namespaces (create, list, delete, quota).`,
	}

	cmd.AddCommand(newNamespaceListCmd())
	cmd.AddCommand(newNamespaceCreateCmd())
	cmd.AddCommand(newNamespaceDeleteCmd())
	cmd.AddCommand(newNamespaceInfoCmd())
	cmd.AddCommand(newNamespaceQuotaCmd())
	cmd.AddCommand(newNamespaceAccessCmd())

	return cmd
}

// NamespaceInfo contains information about a namespace.
type NamespaceInfo struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Backend     string            `json:"backend"`
	Path        string            `json:"path"`
	Created     time.Time         `json:"created"`
	Modified    time.Time         `json:"modified"`
	FileCount   int64             `json:"file_count"`
	TotalSize   int64             `json:"total_size"`
	Quota       *NamespaceQuota   `json:"quota,omitempty"`
	Access      *NamespaceAccess  `json:"access,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// NamespaceQuota contains quota information.
type NamespaceQuota struct {
	MaxSize      int64   `json:"max_size,omitempty"`
	MaxFiles     int64   `json:"max_files,omitempty"`
	MaxFileSize  int64   `json:"max_file_size,omitempty"`
	UsedSize     int64   `json:"used_size"`
	UsedFiles    int64   `json:"used_files"`
	SizePercent  float64 `json:"size_percent"`
	FilesPercent float64 `json:"files_percent"`
}

// NamespaceAccess contains access control information.
type NamespaceAccess struct {
	ReadOnly     bool     `json:"read_only"`
	AllowedIPs   []string `json:"allowed_ips,omitempty"`
	AllowedUsers []string `json:"allowed_users,omitempty"`
	DeniedUsers  []string `json:"denied_users,omitempty"`
}

// newNamespaceListCmd creates the list command.
func newNamespaceListCmd() *cobra.Command {
	var outputFmt string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List namespaces",
		Long: `List all configured namespaces.

Examples:
  kscore-files namespace list
  kscore-files ns list --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutAdmin)
			defer cancel()

			namespaces, err := listNamespaces(ctx, client)
			if err != nil {
				return err
			}

			format, err := output.ParseFormat(outputFmt)
			if err != nil {
				return err
			}

			switch format {
			case output.FormatJSON:
				return output.WriteJSON(os.Stdout, namespaces)
			case output.FormatYAML:
				return output.WriteYAML(os.Stdout, namespaces)
			case output.FormatTable, output.FormatText:
				if len(namespaces) == 0 {
					fmt.Println("No namespaces configured")
					return nil
				}

				rows := make([][]string, 0, len(namespaces))
				for i := range namespaces {
					ns := &namespaces[i]
					desc := ns.Description
					if len(desc) > 25 {
						desc = desc[:22] + "..."
					}
					rows = append(rows, []string{
						ns.Name,
						ns.Backend,
						fmt.Sprintf("%d", ns.FileCount),
						formatSize(ns.TotalSize),
						desc,
					})
				}
				table := &output.Table{
					Headers: []string{"NAME", "BACKEND", "FILES", "SIZE", "DESCRIPTION"},
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

// newNamespaceCreateCmd creates the create command.
func newNamespaceCreateCmd() *cobra.Command {
	var (
		backend     string
		path        string
		description string
		maxSize     string
		maxFiles    int64
		readOnly    bool
		dryRun      bool
	)

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a namespace",
		Long: `Create a new file namespace.

Examples:
  kscore-files namespace create packages --backend local --path /data/packages
  kscore-files namespace create configs --backend s3 --max-size 10GB
  kscore-files ns create temp --read-only`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			// Parse max size
			var maxSizeBytes int64
			if maxSize != "" {
				var err error
				maxSizeBytes, err = parseSize(maxSize)
				if err != nil {
					return fmt.Errorf("invalid max-size: %w", err)
				}
			}

			req := &CreateNamespaceRequest{
				Name:        name,
				Backend:     backend,
				Path:        path,
				Description: description,
				MaxSize:     maxSizeBytes,
				MaxFiles:    maxFiles,
				ReadOnly:    readOnly,
			}

			if dryRun {
				fmt.Printf("Dry run: would create namespace %s\n", name)
				fmt.Printf("  Backend: %s\n", backend)
				if path != "" {
					fmt.Printf("  Path: %s\n", path)
				}
				if description != "" {
					fmt.Printf("  Description: %s\n", description)
				}
				if maxSizeBytes > 0 {
					fmt.Printf("  Max Size: %s\n", formatSize(maxSizeBytes))
				}
				if maxFiles > 0 {
					fmt.Printf("  Max Files: %d\n", maxFiles)
				}
				fmt.Printf("  Read Only: %t\n", readOnly)
				return nil
			}

			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutAdmin)
			defer cancel()

			if err := createNamespace(ctx, client, req); err != nil {
				return err
			}

			fmt.Printf("Namespace %s created\n", name)
			return nil
		},
	}

	cmd.Flags().StringVar(&backend, "backend", "", "Backend name (required)")
	cmd.Flags().StringVar(&path, "path", "", "Path prefix in backend")
	cmd.Flags().StringVar(&description, "description", "", "Description")
	cmd.Flags().StringVar(&maxSize, "max-size", "", "Maximum total size (e.g., 10GB, 500MB)")
	cmd.Flags().Int64Var(&maxFiles, "max-files", 0, "Maximum number of files")
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "Make namespace read-only")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be created without creating")

	cmd.MarkFlagRequired("backend")

	return cmd
}

// newNamespaceDeleteCmd creates the delete command.
func newNamespaceDeleteCmd() *cobra.Command {
	var (
		force  bool
		dryRun bool
	)

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a namespace",
		Long: `Delete a namespace. Files in the namespace are not deleted by default.

Examples:
  kscore-files namespace delete temp
  kscore-files ns delete old-packages --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if dryRun {
				fmt.Printf("Dry run: would delete namespace %s\n", name)
				return nil
			}

			if !force {
				fmt.Printf("Delete namespace %s? [y/N]: ", name)
				var confirm string
				fmt.Scanln(&confirm)
				if !strings.EqualFold(confirm, "y") && !strings.EqualFold(confirm, "yes") {
					fmt.Println("Cancelled")
					return nil
				}
			}

			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutAdmin)
			defer cancel()

			if err := deleteNamespace(ctx, client, name); err != nil {
				return err
			}

			fmt.Printf("Namespace %s deleted\n", name)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Don't prompt for confirmation")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be deleted without deleting")

	return cmd
}

// newNamespaceInfoCmd creates the info command.
func newNamespaceInfoCmd() *cobra.Command {
	var outputFmt string

	cmd := &cobra.Command{
		Use:   "info <name>",
		Short: "Get namespace information",
		Long: `Get detailed information about a namespace.

Examples:
  kscore-files namespace info packages
  kscore-files ns info configs --output json`,
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

			info, err := getNamespaceInfo(ctx, client, name)
			if err != nil {
				return err
			}

			format, err := output.ParseFormat(outputFmt)
			if err != nil {
				return err
			}

			switch format {
			case output.FormatJSON:
				return output.WriteJSON(os.Stdout, info)
			case output.FormatYAML:
				return output.WriteYAML(os.Stdout, info)
			case output.FormatTable:
				table := buildKeyValueTable([][2]string{
					{"NAMESPACE", info.Name},
					{"DESCRIPTION", info.Description},
					{"BACKEND", info.Backend},
					{"PATH", info.Path},
					{"CREATED", info.Created.Format(time.RFC3339)},
					{"MODIFIED", info.Modified.Format(time.RFC3339)},
					{"FILES", fmt.Sprintf("%d", info.FileCount)},
					{"TOTAL SIZE", formatSize(info.TotalSize)},
				})
				if err := output.WriteTable(os.Stdout, table); err != nil {
					return err
				}
				if info.Quota != nil {
					fmt.Println("\nQuota:")
					quota := buildKeyValueTable([][2]string{
						{"SIZE", quotaLine(info.Quota)},
						{"FILES", quotaFilesLine(info.Quota)},
						{"MAX FILE SIZE", quotaMaxFileSize(info.Quota)},
					})
					if err := output.WriteTable(os.Stdout, quota); err != nil {
						return err
					}
				}
				if info.Access != nil {
					fmt.Println("\nAccess:")
					access := buildKeyValueTable([][2]string{
						{"READ-ONLY", fmt.Sprintf("%t", info.Access.ReadOnly)},
						{"ALLOWED IPS", strings.Join(info.Access.AllowedIPs, ", ")},
						{"ALLOWED USERS", strings.Join(info.Access.AllowedUsers, ", ")},
						{"DENIED USERS", strings.Join(info.Access.DeniedUsers, ", ")},
					})
					if err := output.WriteTable(os.Stdout, access); err != nil {
						return err
					}
				}
				if len(info.Metadata) > 0 {
					rows := make([][2]string, 0, len(info.Metadata))
					for k, v := range info.Metadata {
						rows = append(rows, [2]string{k, v})
					}
					fmt.Println("\nMetadata:")
					return output.WriteTable(os.Stdout, buildKeyValueTable(rows))
				}
				return nil
			case output.FormatText:
				fmt.Printf("Namespace: %s\n", info.Name)
				if info.Description != "" {
					fmt.Printf("Description: %s\n", info.Description)
				}
				fmt.Printf("Backend: %s\n", info.Backend)
				fmt.Printf("Path: %s\n", info.Path)
				fmt.Printf("Created: %s\n", info.Created.Format(time.RFC3339))
				fmt.Printf("Modified: %s\n", info.Modified.Format(time.RFC3339))
				fmt.Printf("Files: %d\n", info.FileCount)
				fmt.Printf("Total Size: %s\n", formatSize(info.TotalSize))

				if info.Quota != nil {
					fmt.Println("\nQuota:")
					if info.Quota.MaxSize > 0 {
						fmt.Printf("  Size: %s / %s (%.1f%%)\n",
							formatSize(info.Quota.UsedSize),
							formatSize(info.Quota.MaxSize),
							info.Quota.SizePercent)
					}
					if info.Quota.MaxFiles > 0 {
						fmt.Printf("  Files: %d / %d (%.1f%%)\n",
							info.Quota.UsedFiles,
							info.Quota.MaxFiles,
							info.Quota.FilesPercent)
					}
					if info.Quota.MaxFileSize > 0 {
						fmt.Printf("  Max File Size: %s\n", formatSize(info.Quota.MaxFileSize))
					}
				}

				if info.Access != nil {
					fmt.Println("\nAccess:")
					fmt.Printf("  Read-Only: %v\n", info.Access.ReadOnly)
					if len(info.Access.AllowedIPs) > 0 {
						fmt.Printf("  Allowed IPs: %s\n", strings.Join(info.Access.AllowedIPs, ", "))
					}
					if len(info.Access.AllowedUsers) > 0 {
						fmt.Printf("  Allowed Users: %s\n", strings.Join(info.Access.AllowedUsers, ", "))
					}
				}

				if len(info.Metadata) > 0 {
					fmt.Println("\nMetadata:")
					for k, v := range info.Metadata {
						fmt.Printf("  %s: %s\n", k, v)
					}
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

// newNamespaceQuotaCmd creates the quota command.
func newNamespaceQuotaCmd() *cobra.Command {
	var (
		maxSize     string
		maxFiles    int64
		maxFileSize string
		clearQuota  bool
		dryRun      bool
	)

	cmd := &cobra.Command{
		Use:   "quota <name>",
		Short: "Set namespace quota",
		Long: `Set or clear quota limits for a namespace.

Examples:
  kscore-files namespace quota packages --max-size 100GB
  kscore-files namespace quota configs --max-files 1000 --max-file-size 10MB
  kscore-files ns quota temp --clear`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if clearQuota {
				if dryRun {
					fmt.Printf("Dry run: would clear quota for namespace %s\n", name)
					return nil
				}

				client, cleanup, err := createAdminClient()
				if err != nil {
					return err
				}
				defer cleanup()

				ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutAdmin)
				defer cancel()

				if err := setNamespaceQuota(ctx, client, name, nil); err != nil {
					return err
				}
				fmt.Printf("Quota cleared for namespace %s\n", name)
				return nil
			}

			quota := &NamespaceQuota{}

			if maxSize != "" {
				size, err := parseSize(maxSize)
				if err != nil {
					return fmt.Errorf("invalid max-size: %w", err)
				}
				quota.MaxSize = size
			}

			if maxFiles > 0 {
				quota.MaxFiles = maxFiles
			}

			if maxFileSize != "" {
				size, err := parseSize(maxFileSize)
				if err != nil {
					return fmt.Errorf("invalid max-file-size: %w", err)
				}
				quota.MaxFileSize = size
			}

			if dryRun {
				fmt.Printf("Dry run: would update quota for namespace %s\n", name)
				if quota.MaxSize > 0 {
					fmt.Printf("  Max Size: %s\n", formatSize(quota.MaxSize))
				}
				if quota.MaxFiles > 0 {
					fmt.Printf("  Max Files: %d\n", quota.MaxFiles)
				}
				if quota.MaxFileSize > 0 {
					fmt.Printf("  Max File Size: %s\n", formatSize(quota.MaxFileSize))
				}
				return nil
			}

			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutAdmin)
			defer cancel()

			if err := setNamespaceQuota(ctx, client, name, quota); err != nil {
				return err
			}

			fmt.Printf("Quota updated for namespace %s\n", name)
			return nil
		},
	}

	cmd.Flags().StringVar(&maxSize, "max-size", "", "Maximum total size")
	cmd.Flags().Int64Var(&maxFiles, "max-files", 0, "Maximum number of files")
	cmd.Flags().StringVar(&maxFileSize, "max-file-size", "", "Maximum file size")
	cmd.Flags().BoolVar(&clearQuota, "clear", false, "Clear all quotas")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be changed without updating quota")

	return cmd
}

// newNamespaceAccessCmd creates the access command.
func newNamespaceAccessCmd() *cobra.Command {
	var (
		readOnly    bool
		readWrite   bool
		allowIP     []string
		denyIP      []string
		allowUser   []string
		denyUser    []string
		clearAccess bool
		dryRun      bool
	)

	cmd := &cobra.Command{
		Use:   "access <name>",
		Short: "Set namespace access controls",
		Long: `Set access controls for a namespace.

Examples:
  kscore-files namespace access packages --read-only
  kscore-files namespace access configs --allow-ip 10.0.0.0/8
  kscore-files ns access secure --allow-user admin --allow-user ops
  kscore-files ns access temp --clear`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if clearAccess {
				if dryRun {
					fmt.Printf("Dry run: would clear access controls for namespace %s\n", name)
					return nil
				}

				client, cleanup, err := createAdminClient()
				if err != nil {
					return err
				}
				defer cleanup()

				ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutAdmin)
				defer cancel()

				if err := setNamespaceAccess(ctx, client, name, nil); err != nil {
					return err
				}
				fmt.Printf("Access controls cleared for namespace %s\n", name)
				return nil
			}

			access := &NamespaceAccess{}

			if readOnly {
				access.ReadOnly = true
			} else if readWrite {
				access.ReadOnly = false
			}

			access.AllowedIPs = allowIP
			access.AllowedUsers = allowUser
			access.DeniedUsers = denyUser

			// Handle deny IPs by removing from allowed (simplified)
			if len(denyIP) > 0 && len(access.AllowedIPs) == 0 {
				// This is a simplified model - in practice, denied IPs would be handled separately
				fmt.Println("Warning: --deny-ip requires existing --allow-ip rules")
			}

			if dryRun {
				fmt.Printf("Dry run: would update access controls for namespace %s\n", name)
				fmt.Printf("  Read Only: %t\n", access.ReadOnly)
				if len(access.AllowedIPs) > 0 {
					fmt.Printf("  Allowed IPs: %s\n", strings.Join(access.AllowedIPs, ", "))
				}
				if len(allowUser) > 0 {
					fmt.Printf("  Allowed Users: %s\n", strings.Join(allowUser, ", "))
				}
				if len(denyUser) > 0 {
					fmt.Printf("  Denied Users: %s\n", strings.Join(denyUser, ", "))
				}
				return nil
			}

			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutAdmin)
			defer cancel()

			if err := setNamespaceAccess(ctx, client, name, access); err != nil {
				return err
			}

			fmt.Printf("Access controls updated for namespace %s\n", name)
			return nil
		},
	}

	cmd.Flags().BoolVar(&readOnly, "read-only", false, "Make namespace read-only")
	cmd.Flags().BoolVar(&readWrite, "read-write", false, "Make namespace read-write")
	cmd.Flags().StringSliceVar(&allowIP, "allow-ip", nil, "Allow IP/CIDR")
	cmd.Flags().StringSliceVar(&denyIP, "deny-ip", nil, "Deny IP/CIDR")
	cmd.Flags().StringSliceVar(&allowUser, "allow-user", nil, "Allow user")
	cmd.Flags().StringSliceVar(&denyUser, "deny-user", nil, "Deny user")
	cmd.Flags().BoolVar(&clearAccess, "clear", false, "Clear all access controls")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be changed without updating access controls")

	return cmd
}

// Helper functions

type CreateNamespaceRequest struct {
	Name        string `json:"name"`
	Backend     string `json:"backend"`
	Path        string `json:"path,omitempty"`
	Description string `json:"description,omitempty"`
	MaxSize     int64  `json:"max_size,omitempty"`
	MaxFiles    int64  `json:"max_files,omitempty"`
	ReadOnly    bool   `json:"read_only,omitempty"`
}

func listNamespaces(ctx context.Context, client *AdminClient) ([]NamespaceInfo, error) {
	subject := fmt.Sprintf("kscore.%s.files.admin.namespace.list", client.clusterID)

	msg, err := client.nc.RequestWithContext(ctx, subject, nil)
	if err != nil {
		return nil, err
	}

	var namespaces []NamespaceInfo
	if err := json.Unmarshal(msg.Data, &namespaces); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return namespaces, nil
}

func createNamespace(ctx context.Context, client *AdminClient, req *CreateNamespaceRequest) error {
	subject := fmt.Sprintf("kscore.%s.files.admin.namespace.create", client.clusterID)

	reqData, _ := json.Marshal(req)

	_, err := client.nc.RequestWithContext(ctx, subject, reqData)
	return err
}

func deleteNamespace(ctx context.Context, client *AdminClient, name string) error {
	subject := fmt.Sprintf("kscore.%s.files.admin.namespace.delete", client.clusterID)

	req := map[string]string{"name": name}
	reqData, _ := json.Marshal(req)

	_, err := client.nc.RequestWithContext(ctx, subject, reqData)
	return err
}

func getNamespaceInfo(ctx context.Context, client *AdminClient, name string) (*NamespaceInfo, error) {
	subject := fmt.Sprintf("kscore.%s.files.admin.namespace.info", client.clusterID)

	req := map[string]string{"name": name}
	reqData, _ := json.Marshal(req)

	msg, err := client.nc.RequestWithContext(ctx, subject, reqData)
	if err != nil {
		return nil, err
	}

	var info NamespaceInfo
	if err := json.Unmarshal(msg.Data, &info); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &info, nil
}

func setNamespaceQuota(ctx context.Context, client *AdminClient, name string, quota *NamespaceQuota) error {
	subject := fmt.Sprintf("kscore.%s.files.admin.namespace.quota", client.clusterID)

	req := map[string]interface{}{
		"name":  name,
		"quota": quota,
	}
	reqData, _ := json.Marshal(req)

	_, err := client.nc.RequestWithContext(ctx, subject, reqData)
	return err
}

func setNamespaceAccess(ctx context.Context, client *AdminClient, name string, access *NamespaceAccess) error {
	subject := fmt.Sprintf("kscore.%s.files.admin.namespace.access", client.clusterID)

	req := map[string]interface{}{
		"name":   name,
		"access": access,
	}
	reqData, _ := json.Marshal(req)

	_, err := client.nc.RequestWithContext(ctx, subject, reqData)
	return err
}

func quotaLine(quota *NamespaceQuota) string {
	if quota == nil || quota.MaxSize == 0 {
		return ""
	}
	return fmt.Sprintf("%s / %s (%.1f%%)", formatSize(quota.UsedSize), formatSize(quota.MaxSize), quota.SizePercent)
}

func quotaFilesLine(quota *NamespaceQuota) string {
	if quota == nil || quota.MaxFiles == 0 {
		return ""
	}
	return fmt.Sprintf("%d / %d (%.1f%%)", quota.UsedFiles, quota.MaxFiles, quota.FilesPercent)
}

func quotaMaxFileSize(quota *NamespaceQuota) string {
	if quota == nil || quota.MaxFileSize == 0 {
		return ""
	}
	return formatSize(quota.MaxFileSize)
}

func parseSize(s string) (int64, error) {
	s = strings.ToUpper(strings.TrimSpace(s))

	multipliers := map[string]int64{
		"B":  1,
		"KB": 1024,
		"MB": 1024 * 1024,
		"GB": 1024 * 1024 * 1024,
		"TB": 1024 * 1024 * 1024 * 1024,
		"K":  1024,
		"M":  1024 * 1024,
		"G":  1024 * 1024 * 1024,
		"T":  1024 * 1024 * 1024 * 1024,
	}

	for suffix, mult := range multipliers {
		if strings.HasSuffix(s, suffix) {
			numStr := strings.TrimSuffix(s, suffix)
			var num float64
			if _, err := fmt.Sscanf(numStr, "%f", &num); err != nil {
				return 0, err
			}
			return int64(num * float64(mult)), nil
		}
	}

	// No suffix - assume bytes
	var num int64
	if _, err := fmt.Sscanf(s, "%d", &num); err != nil {
		return 0, err
	}
	return num, nil
}
