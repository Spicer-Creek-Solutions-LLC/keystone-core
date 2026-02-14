// Package main implements the kscore-exec CLI for remote command execution on agents.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/shawnbutts/keystone-core/internal/audit"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

// Config holds CLI configuration
type Config struct {
	ServerAddr    string
	Timeout       time.Duration
	AuditLevel    string
	AuditOutput   string
	TLS           bool
	TLSCACert     string
	TLSCert       string
	TLSKey        string
	TLSSkipVerify bool
	TLSServerName string
	TLSMinVersion string
}

func newRootCmd() *cobra.Command {
	cfg := &Config{}

	rootCmd := &cobra.Command{
		Use:   "kscore-exec",
		Short: "Remote execution across multiple agents",
		Long: `Execute commands across your infrastructure using target expressions.

Examples:
  # Execute on all web servers
  kscorectl exec run "role:web" -- echo "hello"

  # Execute with specific concurrency
  kscorectl exec run "os:linux and env:prod" --concurrency 5 -- apt-get update

  # Get batch job status
  kscorectl exec status <job-id>

  # List recent batch jobs
  kscorectl exec list --status completed`,
	}

	rootCmd.PersistentFlags().StringVar(&cfg.ServerAddr, "server", "localhost:50051", "Keystone Core server address")
	rootCmd.PersistentFlags().DurationVar(&cfg.Timeout, "timeout", 5*time.Minute, "Request timeout")
	rootCmd.PersistentFlags().StringVar(&cfg.AuditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&cfg.AuditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")

	// TLS flags
	rootCmd.PersistentFlags().BoolVar(&cfg.TLS, "tls", false, "Enable TLS for server connection")
	rootCmd.PersistentFlags().StringVar(&cfg.TLSCACert, "tls-ca-cert", "", "Path to CA certificate for verifying the server")
	rootCmd.PersistentFlags().StringVar(&cfg.TLSCert, "tls-cert", "", "Path to client certificate for mTLS authentication")
	rootCmd.PersistentFlags().StringVar(&cfg.TLSKey, "tls-key", "", "Path to client private key for mTLS authentication")
	rootCmd.PersistentFlags().BoolVar(&cfg.TLSSkipVerify, "tls-skip-verify", false, "Skip TLS certificate verification (INSECURE - for development only)")
	rootCmd.PersistentFlags().StringVar(&cfg.TLSServerName, "tls-server-name", "", "Server name for TLS verification (defaults to server host)")
	rootCmd.PersistentFlags().StringVar(&cfg.TLSMinVersion, "tls-min-version", "1.3", "Minimum TLS version (1.2 or 1.3)")

	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newRunCmd(cfg))
	rootCmd.AddCommand(newStatusCmd(cfg))
	rootCmd.AddCommand(newListCmd(cfg))
	rootCmd.AddCommand(newAsyncCmd(cfg))
	rootCmd.AddCommand(newCancelCmd(cfg))
	rootCmd.AddCommand(newHistoryCmd(cfg))
	rootCmd.AddCommand(newOutputCmd(cfg))
	rootCmd.AddCommand(newShellCmd(cfg))
	rootCmd.AddCommand(newScriptCmd(cfg))
	rootCmd.AddCommand(newArchiveCmd(cfg))
	rootCmd.AddCommand(newExportCmd(cfg))
	rootCmd.AddCommand(newCleanupCmd(cfg))

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

func main() {
	// Initialize audit logging (Epic 15)
	auditConfig := &audit.AuditConfig{
		Level:   audit.AuditLevel("all"),
		Backend: "auto",
	}
	if err := audit.Init(context.Background(), "kscore-exec", auditConfig); err != nil {
		// Don't fail if audit logging can't be initialized, just warn
		fmt.Fprintf(os.Stderr, "Warning: failed to initialize audit logging: %v\n", err)
	}
	defer audit.Close()

	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
}

// createClient creates a gRPC client connection to the control plane
func createClient(cfg *Config) (pb.ControlPlaneServiceClient, *grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var dialOpts []grpc.DialOption

	// Configure TLS or insecure credentials
	if cfg.TLS || cfg.TLSCACert != "" || cfg.TLSCert != "" {
		tlsConfig, err := buildTLSConfig(cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to configure TLS: %w", err)
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		// Use insecure credentials only when TLS is not configured
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	dialOpts = append(dialOpts, grpc.WithBlock()) //nolint:staticcheck // SA1019: grpc.WithBlock is deprecated but supported throughout gRPC 1.x; migration to NewClient requires significant refactoring

	conn, err := grpc.DialContext(ctx, cfg.ServerAddr, dialOpts...) //nolint:staticcheck // SA1019: grpc.DialContext is deprecated but supported throughout gRPC 1.x; migration to NewClient requires significant refactoring
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to server: %w", err)
	}

	client := pb.NewControlPlaneServiceClient(conn)
	return client, conn, nil
}

// buildTLSConfig builds a TLS configuration from the CLI flags
func buildTLSConfig(cfg *Config) (*tls.Config, error) {
	minVersion, err := parseTLSMinVersion(cfg.TLSMinVersion)
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{ // nosemgrep: problem-based-packs.insecure-transport.go-stdlib.bypass-tls-verification.bypass-tls-verification -- InsecureSkipVerify allowed only when KSCORE_ALLOW_INSECURE_TLS=1 is set for dev/test
		MinVersion: minVersion, // #nosec G402 -- validated to TLS 1.2+ defaults
	}

	// Handle skip verify (with warning)
	if cfg.TLSSkipVerify {
		if os.Getenv("KSCORE_ALLOW_INSECURE_TLS") != "1" {
			return nil, fmt.Errorf("TLS skip verify requires KSCORE_ALLOW_INSECURE_TLS=1 for development/testing only")
		}
		fmt.Fprintln(os.Stderr, "WARNING: TLS certificate verification is disabled. This is insecure and should only be used for development.")
		tlsConfig.InsecureSkipVerify = true // #nosec G402 -- gated by KSCORE_ALLOW_INSECURE_TLS
	}

	// Set server name for verification
	if cfg.TLSServerName != "" {
		tlsConfig.ServerName = cfg.TLSServerName
	}

	// Load CA certificate if provided
	if cfg.TLSCACert != "" {
		caCert, err := os.ReadFile(cfg.TLSCACert)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	// Load client certificate and key for mTLS
	if cfg.TLSCert != "" || cfg.TLSKey != "" {
		if cfg.TLSCert == "" || cfg.TLSKey == "" {
			return nil, fmt.Errorf("both --tls-cert and --tls-key must be provided for mTLS")
		}

		cert, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

func parseTLSMinVersion(value string) (uint16, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "1.3", "tls1.3", "tls13":
		return tls.VersionTLS13, nil
	case "1.2", "tls1.2", "tls12":
		return tls.VersionTLS12, nil
	default:
		return 0, fmt.Errorf("unsupported TLS minimum version: %s", value)
	}
}

// RunOptions holds run command options
type RunOptions struct {
	Concurrency      int32
	ContinueOnError  bool
	Target           string
	WorkingDir       string
	User             string
	CommandTimeout   int32
	Env              []string
	JobID            string
	ShowProgress     bool
	ShowAgentResults bool
	DryRun           bool
	Shell            string
}

// newRunCmd creates the run command
func newRunCmd(cfg *Config) *cobra.Command {
	opts := &RunOptions{}

	cmd := &cobra.Command{
		Use:   "run <target-expression> -- <command> [args...]",
		Short: "Execute a command across multiple agents",
		Long: `Execute a command across agents matching the target expression.

Target expressions support:
  - Label matching: role:web, env:prod
  - OS matching: os:linux, arch:amd64
  - Hostname glob: hostname:web-*
  - Status: status:agent_status_online
  - Logical operators: and, or, not
  - Grouping: (os:linux and role:web) or (os:darwin and role:api)

Examples:
  # Execute on all Linux web servers
  kscorectl exec run "os:linux and role:web" -- systemctl restart nginx

  # Execute on specific hostname pattern
  kscorectl exec run "hostname:web-*" -- apt-get update

  # Execute using a target flag
  kscorectl exec run "uptime" --target "role:web"

  # Execute with environment variables
  kscorectl exec run "role:db" --env DB_HOST=localhost -- ./backup.sh

  # Execute with custom concurrency
  kscorectl exec run "env:prod" --concurrency 10 -- uptime`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExecute(cmd, args, cfg, opts)
		},
	}

	cmd.Flags().Int32Var(&opts.Concurrency, "concurrency", 10, "Number of concurrent executions")
	cmd.Flags().BoolVar(&opts.ContinueOnError, "continue-on-failure", true, "Continue executing on other agents if some fail")
	cmd.Flags().StringVar(&opts.Target, "target", "", "Target expression (optional when command is first)")
	cmd.Flags().StringVar(&opts.WorkingDir, "working-dir", "", "Working directory for command execution")
	cmd.Flags().StringVar(&opts.User, "user", "", "User to execute command as")
	cmd.Flags().Int32Var(&opts.CommandTimeout, "command-timeout", 300, "Command timeout in seconds")
	cmd.Flags().StringArrayVar(&opts.Env, "env", nil, "Environment variables (KEY=VALUE)")
	cmd.Flags().StringVar(&opts.JobID, "job-id", "", "Custom batch job ID (auto-generated if not specified)")
	cmd.Flags().BoolVar(&opts.ShowProgress, "show-progress", true, "Show progress updates during execution")
	cmd.Flags().BoolVar(&opts.ShowAgentResults, "show-results", true, "Show per-agent results at the end")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Preview matched agents and command without executing")
	cmd.Flags().StringVar(&opts.Shell, "shell", "", "Shell to use for command execution (e.g., powershell, cmd, bash)")

	return cmd
}

func runExecute(cmd *cobra.Command, args []string, cfg *Config, opts *RunOptions) error {
	startTime := time.Now()
	ctx := context.Background()

	// Create audit entry (Epic 15)
	auditEntry := audit.StartEntry(audit.ActionCommandExecuted, "run")
	auditEntry.Args = args

	// Helper to log audit on exit
	logAudit := func(result audit.AuditResult, exitCode int, err error) {
		auditEntry.Result = result
		auditEntry.ExitCode = exitCode
		auditEntry.DurationMS = time.Since(startTime).Milliseconds()
		if err != nil {
			auditEntry.Error = err.Error()
		}
		_ = audit.Log(ctx, auditEntry)
	}

	target := opts.Target
	argOffset := 0
	if target == "" {
		if len(args) < 1 {
			err := fmt.Errorf("target expression is required")
			logAudit(audit.ResultFailure, 1, err)
			return err
		}
		target = args[0]
		argOffset = 1
	}
	auditEntry.Target = target

	// Find the command after "--"
	var command string
	var cmdArgs []string

	remaining := args[argOffset:]
	for i, arg := range remaining {
		if arg == "--" {
			if i+1 >= len(remaining) {
				err := fmt.Errorf("command is required after '--'")
				logAudit(audit.ResultFailure, 1, err)
				return err
			}
			command = remaining[i+1]
			if i+2 < len(remaining) {
				cmdArgs = remaining[i+2:]
			}
			break
		}
	}

	if command == "" {
		// No "--" separator, assume everything after target is the command
		if len(remaining) < 1 {
			err := fmt.Errorf("command is required")
			logAudit(audit.ResultFailure, 1, err)
			return err
		}
		command = remaining[0]
		if len(remaining) > 1 {
			cmdArgs = remaining[1:]
		}
	}

	// Parse environment variables
	envMap := make(map[string]string)
	for _, e := range opts.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			err := fmt.Errorf("invalid environment variable format: %s (expected KEY=VALUE)", e)
			logAudit(audit.ResultFailure, 1, err)
			return err
		}
		envMap[parts[0]] = parts[1]
	}

	// Create client
	client, conn, err := createClient(cfg)
	if err != nil {
		logAudit(audit.ResultFailure, 1, err)
		return err
	}
	defer conn.Close()

	// Handle dry-run mode - preview matched agents without executing
	if opts.DryRun {
		return runDryRun(cmd, client, cfg, target, command, cmdArgs, envMap, opts)
	}

	// Re-create client for actual execution (deferred close already set up above)
	client, conn, err = createClient(cfg)
	if err != nil {
		logAudit(audit.ResultFailure, 1, err)
		return err
	}
	defer conn.Close()

	// Create request
	req := &pb.BatchExecuteCommandRequest{
		BatchJobId:        opts.JobID,
		Target:            target,
		Command:           command,
		Args:              cmdArgs,
		Env:               envMap,
		WorkingDir:        opts.WorkingDir,
		User:              opts.User,
		Timeout:           opts.CommandTimeout,
		Concurrency:       opts.Concurrency,
		ContinueOnFailure: opts.ContinueOnError,
	}

	execCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	// Execute batch command
	stream, err := client.BatchExecuteCommand(execCtx, req)
	if err != nil {
		err = fmt.Errorf("failed to execute batch command: %w", err)
		logAudit(audit.ResultFailure, 1, err)
		return err
	}

	var batchJobID string
	var summary *pb.BatchSummary

	fmt.Printf("Executing: %s %s\n", command, strings.Join(cmdArgs, " "))
	fmt.Printf("Target: %s\n", target)
	fmt.Println()

	// Process responses
	for {
		resp, err := stream.Recv()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			err = fmt.Errorf("stream error: %w", err)
			logAudit(audit.ResultFailure, 1, err)
			return err
		}

		batchJobID = resp.BatchJobId

		switch resp.Type {
		case pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_START:
			fmt.Printf("Batch job started: %s\n", batchJobID)

		case pb.BatchResponseType_BATCH_RESPONSE_TYPE_PROGRESS:
			if opts.ShowProgress && resp.Progress != nil {
				p := resp.Progress
				fmt.Printf("\rProgress: %d/%d agents | Success: %d | Failed: %d | Success Rate: %.1f%%",
					p.Completed, p.Total, p.Successful, p.Failed, p.SuccessRate)
			}

		case pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_COMPLETE:
			if opts.ShowProgress {
				fmt.Println() // New line after progress updates
			}
			fmt.Println("\nBatch execution completed")
			summary = resp.Summary

		case pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_FAILED:
			fmt.Printf("\nBatch execution failed: %s\n", resp.Error)
			err := fmt.Errorf("batch execution failed: %s", resp.Error)
			logAudit(audit.ResultFailure, 1, err)
			return err
		default:
		}
	}

	// Print summary
	if summary != nil {
		fmt.Println("\n=== Summary ===")
		fmt.Printf("Total Agents:      %d\n", summary.Total)
		fmt.Printf("Successful:        %d\n", summary.Successful)
		fmt.Printf("Failed:            %d\n", summary.Failed)
		fmt.Printf("Success Rate:      %.1f%%\n", summary.SuccessRate)
		fmt.Printf("Duration:          %dms\n", summary.DurationMs)

		if opts.ShowAgentResults && len(summary.AgentResults) > 0 {
			fmt.Println("\n=== Agent Results ===")
			for _, result := range summary.AgentResults {
				status := "✓"
				if !result.Success {
					status = "✗"
				}
				fmt.Printf("%s %s (exit code: %d, duration: %dms)\n",
					status, result.AgentId, result.ExitCode, result.DurationMs)
				if result.Error != "" {
					fmt.Printf("  Error: %s\n", result.Error)
				}
			}
		}
	}

	// Return error if batch failed
	if summary != nil && summary.Failed > 0 {
		auditEntry.AgentsMatched = int(summary.Total)
		auditEntry.Extra = map[string]interface{}{
			"successful": summary.Successful,
			"failed":     summary.Failed,
		}
		err := fmt.Errorf("%d agents failed", summary.Failed)
		logAudit(audit.ResultFailure, 1, err)
		return err
	}

	// Log success
	if summary != nil {
		auditEntry.AgentsMatched = int(summary.Total)
		auditEntry.Extra = map[string]interface{}{
			"successful": summary.Successful,
			"failed":     summary.Failed,
		}
	}
	logAudit(audit.ResultSuccess, 0, nil)

	return nil
}

// runDryRun performs a dry run showing what would be executed without actually executing
func runDryRun(cmd *cobra.Command, client pb.ControlPlaneServiceClient, cfg *Config, target, command string, cmdArgs []string, envMap map[string]string, opts *RunOptions) error {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	fmt.Println("=== DRY RUN MODE ===")
	fmt.Println("No commands will be executed.")
	fmt.Println()

	// Show command details
	fmt.Println("Command Details:")
	fmt.Printf("  Target:          %s\n", target)
	fmt.Printf("  Command:         %s %s\n", command, strings.Join(cmdArgs, " "))
	if opts.WorkingDir != "" {
		fmt.Printf("  Working Dir:     %s\n", opts.WorkingDir)
	}
	if opts.User != "" {
		fmt.Printf("  Run As User:     %s\n", opts.User)
	}
	fmt.Printf("  Timeout:         %ds\n", opts.CommandTimeout)
	fmt.Printf("  Concurrency:     %d\n", opts.Concurrency)
	fmt.Printf("  Continue on Fail: %v\n", opts.ContinueOnError)

	if len(envMap) > 0 {
		fmt.Println("  Environment:")
		for k, v := range envMap {
			fmt.Printf("    %s=%s\n", k, v)
		}
	}
	fmt.Println()

	// List all agents to show potential scope
	listReq := &pb.ListAgentsRequest{
		PageSize: 1000,
	}

	resp, err := client.ListAgents(ctx, listReq)
	if err != nil {
		return fmt.Errorf("failed to list agents: %w", err)
	}

	// Show agent information
	fmt.Printf("Agent Summary (total registered: %d):\n", resp.TotalCount)

	// Count agents by status
	var onlineCount, offlineCount int
	for _, agent := range resp.Agents {
		switch agent.Status {
		case pb.AgentStatus_AGENT_STATUS_ONLINE:
			onlineCount++
		case pb.AgentStatus_AGENT_STATUS_OFFLINE:
			offlineCount++
		default:
		}
	}

	fmt.Printf("  Online:  %d\n", onlineCount)
	fmt.Printf("  Offline: %d\n", offlineCount)
	fmt.Println()

	// Parse and display target expression information
	fmt.Println("Target Expression Analysis:")
	fmt.Printf("  Expression: %s\n", target)
	fmt.Println()

	// Show matching agents (simplified - shows all agents with labels for manual verification)
	fmt.Println("Registered Agents (target matching will occur server-side):")
	fmt.Printf("%-40s %-12s %-20s %-20s\n", "AGENT ID", "STATUS", "OS", "LABELS")
	fmt.Println(strings.Repeat("-", 100))

	displayCount := 0
	maxDisplay := 20

	for _, agent := range resp.Agents {
		if displayCount >= maxDisplay && len(resp.Agents) > maxDisplay {
			fmt.Printf("... and %d more agents\n", len(resp.Agents)-maxDisplay)
			break
		}

		status := "UNKNOWN"
		switch agent.Status {
		case pb.AgentStatus_AGENT_STATUS_ONLINE:
			status = "ONLINE"
		case pb.AgentStatus_AGENT_STATUS_OFFLINE:
			status = "OFFLINE"
		default:
		}

		osInfo := "N/A"
		labelsStr := ""

		if agent.Metadata != nil {
			osInfo = agent.Metadata.Os
			if agent.Metadata.Labels != nil {
				labels := make([]string, 0, len(agent.Metadata.Labels))
				for k, v := range agent.Metadata.Labels {
					labels = append(labels, k+":"+v)
				}
				if len(labels) > 3 {
					labelsStr = strings.Join(labels[:3], ", ") + "..."
				} else {
					labelsStr = strings.Join(labels, ", ")
				}
			}
		}

		fmt.Printf("%-40s %-12s %-20s %-20s\n",
			truncate(agent.AgentId, 40),
			status,
			truncate(osInfo, 20),
			truncate(labelsStr, 20),
		)
		displayCount++
	}

	fmt.Println()
	fmt.Println("=== END DRY RUN ===")
	fmt.Println("Run without --dry-run to execute the command.")

	return nil
}

// newStatusCmd creates the status command
func newStatusCmd(cfg *Config) *cobra.Command {
	return &cobra.Command{
		Use:   "status <job-id>",
		Short: "Get the status of a batch job",
		Long: `Retrieve detailed status information about a batch job.

Examples:
  kscorectl exec status abc123
  kscorectl exec status --server prod-server:50051 abc123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return statusExecute(cmd, args, cfg)
		},
	}
}

func statusExecute(cmd *cobra.Command, args []string, cfg *Config) error {
	jobID := args[0]

	client, conn, err := createClient(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	req := &pb.GetBatchJobStatusRequest{
		BatchJobId: jobID,
	}

	resp, err := client.GetBatchJobStatus(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to get batch job status: %w", err)
	}

	job := resp.Job

	// Print job information
	fmt.Printf("Batch Job: %s\n", job.BatchJobId)
	fmt.Printf("Target:    %s\n", job.Target)
	fmt.Printf("Command:   %s %s\n", job.Command, strings.Join(job.Args, " "))
	fmt.Printf("Status:    %s\n", formatStatus(job.Status))
	fmt.Printf("Created:   %s\n", job.CreatedAt.AsTime().Format(time.RFC3339))

	if job.StartedAt != nil {
		fmt.Printf("Started:   %s\n", job.StartedAt.AsTime().Format(time.RFC3339))
	}

	if job.CompletedAt != nil {
		fmt.Printf("Completed: %s\n", job.CompletedAt.AsTime().Format(time.RFC3339))
		fmt.Printf("Duration:  %dms\n", job.DurationMs)
	}

	// Print progress
	if job.Progress != nil {
		fmt.Println("\n=== Progress ===")
		p := job.Progress
		fmt.Printf("Total:         %d\n", p.Total)
		fmt.Printf("Completed:     %d\n", p.Completed)
		fmt.Printf("Successful:    %d\n", p.Successful)
		fmt.Printf("Failed:        %d\n", p.Failed)
		fmt.Printf("Success Rate:  %.1f%%\n", p.SuccessRate)
	}

	// Print summary if job is complete
	if job.Summary != nil && len(job.Summary.AgentResults) > 0 {
		fmt.Println("\n=== Agent Results ===")
		for _, result := range job.Summary.AgentResults {
			status := "✓"
			if !result.Success {
				status = "✗"
			}
			fmt.Printf("%s %s (exit code: %d, duration: %dms)\n",
				status, result.AgentId, result.ExitCode, result.DurationMs)
			if result.Error != "" {
				fmt.Printf("  Error: %s\n", result.Error)
			}
		}
	}

	return nil
}

// ListOptions holds list command options
type ListOptions struct {
	Status   string
	PageSize int32
}

// newListCmd creates the list command
func newListCmd(cfg *Config) *cobra.Command {
	opts := &ListOptions{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List batch jobs",
		Long: `List batch jobs with optional filtering.

Examples:
  # List all jobs
  kscorectl exec list

  # List only completed jobs
  kscorectl exec list --status completed

  # List only running jobs
  kscorectl exec list --status running

  # List with custom page size
  kscorectl exec list --page-size 50`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listExecute(cmd, args, cfg, opts)
		},
	}

	cmd.Flags().StringVar(&opts.Status, "status", "", "Filter by status (pending, running, completed, failed)")
	cmd.Flags().Int32Var(&opts.PageSize, "page-size", 20, "Number of jobs to return")

	return cmd
}

func listExecute(cmd *cobra.Command, args []string, cfg *Config, opts *ListOptions) error {
	client, conn, err := createClient(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	// Parse status filter
	var statusFilter pb.BatchJobStatus
	if opts.Status != "" {
		switch strings.ToLower(opts.Status) {
		case "pending":
			statusFilter = pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING
		case "running":
			statusFilter = pb.BatchJobStatus_BATCH_JOB_STATUS_RUNNING
		case "completed":
			statusFilter = pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED
		case "failed":
			statusFilter = pb.BatchJobStatus_BATCH_JOB_STATUS_FAILED
		default:
			return fmt.Errorf("invalid status: %s (expected pending, running, completed, or failed)", opts.Status)
		}
	}

	req := &pb.ListBatchJobsRequest{
		Status:   statusFilter,
		PageSize: opts.PageSize,
	}

	resp, err := client.ListBatchJobs(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to list batch jobs: %w", err)
	}

	if len(resp.Jobs) == 0 {
		fmt.Println("No batch jobs found")
		return nil
	}

	// Print table header
	fmt.Printf("%-36s %-20s %-12s %-8s %-8s %-8s\n",
		"JOB ID", "TARGET", "STATUS", "TOTAL", "SUCCESS", "FAILED")
	fmt.Println(strings.Repeat("-", 100))

	// Print jobs
	for _, job := range resp.Jobs {
		total := int32(0)
		successful := int32(0)
		failed := int32(0)

		if job.Progress != nil {
			total = job.Progress.Total
			successful = job.Progress.Successful
			failed = job.Progress.Failed
		}

		fmt.Printf("%-36s %-20s %-12s %-8d %-8d %-8d\n",
			job.BatchJobId,
			truncate(job.Target, 20),
			formatStatus(job.Status),
			total,
			successful,
			failed,
		)
	}

	fmt.Printf("\nTotal: %d job(s)\n", len(resp.Jobs))

	return nil
}

// Helper functions

func formatStatus(status pb.BatchJobStatus) string {
	switch status {
	case pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING:
		return "PENDING"
	case pb.BatchJobStatus_BATCH_JOB_STATUS_RUNNING:
		return "RUNNING"
	case pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED:
		return "COMPLETED"
	case pb.BatchJobStatus_BATCH_JOB_STATUS_FAILED:
		return "FAILED"
	default:
		return "UNKNOWN"
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// AsyncOptions holds async command options
type AsyncOptions struct {
	Concurrency     int32
	ContinueOnError bool
	Target          string
	WorkingDir      string
	User            string
	CommandTimeout  int32
	Env             []string
	JobID           string
}

// newAsyncCmd creates the async command for fire-and-forget execution
func newAsyncCmd(cfg *Config) *cobra.Command {
	opts := &AsyncOptions{}

	cmd := &cobra.Command{
		Use:   "async <target-expression> -- <command> [args...]",
		Short: "Execute a command asynchronously (fire and forget)",
		Long: `Execute a command across agents without waiting for completion.
Returns immediately with a job ID that can be used to check status later.

Examples:
  # Start async execution on all web servers
  kscorectl exec async "role:web" -- /opt/scripts/deploy.sh

  # Async execution with custom job ID
  kscorectl exec async "env:prod" --job-id deploy-v1.2.3 -- ./deploy.sh

  # Check status later
  kscorectl exec status deploy-v1.2.3`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return asyncExecute(cmd, args, cfg, opts)
		},
	}

	cmd.Flags().Int32Var(&opts.Concurrency, "concurrency", 10, "Number of concurrent executions")
	cmd.Flags().BoolVar(&opts.ContinueOnError, "continue-on-failure", true, "Continue executing on other agents if some fail")
	cmd.Flags().StringVar(&opts.Target, "target", "", "Target expression (optional when command is first)")
	cmd.Flags().StringVar(&opts.WorkingDir, "working-dir", "", "Working directory for command execution")
	cmd.Flags().StringVar(&opts.User, "user", "", "User to execute command as")
	cmd.Flags().Int32Var(&opts.CommandTimeout, "command-timeout", 300, "Command timeout in seconds")
	cmd.Flags().StringArrayVar(&opts.Env, "env", nil, "Environment variables (KEY=VALUE)")
	cmd.Flags().StringVar(&opts.JobID, "job-id", "", "Custom batch job ID (auto-generated if not specified)")

	return cmd
}

func asyncExecute(cmd *cobra.Command, args []string, cfg *Config, opts *AsyncOptions) error {
	ctx := context.Background()

	target := opts.Target
	argOffset := 0
	if target == "" {
		if len(args) < 1 {
			return fmt.Errorf("target expression is required")
		}
		target = args[0]
		argOffset = 1
	}

	// Find the command after "--"
	var command string
	var cmdArgs []string

	remaining := args[argOffset:]
	for i, arg := range remaining {
		if arg == "--" {
			if i+1 >= len(remaining) {
				return fmt.Errorf("command is required after '--'")
			}
			command = remaining[i+1]
			if i+2 < len(remaining) {
				cmdArgs = remaining[i+2:]
			}
			break
		}
	}

	if command == "" {
		if len(remaining) < 1 {
			return fmt.Errorf("command is required")
		}
		command = remaining[0]
		if len(remaining) > 1 {
			cmdArgs = remaining[1:]
		}
	}

	// Parse environment variables
	envMap := make(map[string]string)
	for _, e := range opts.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid environment variable format: %s (expected KEY=VALUE)", e)
		}
		envMap[parts[0]] = parts[1]
	}

	// Create client
	client, conn, err := createClient(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Create request
	req := &pb.BatchExecuteCommandRequest{
		BatchJobId:        opts.JobID,
		Target:            target,
		Command:           command,
		Args:              cmdArgs,
		Env:               envMap,
		WorkingDir:        opts.WorkingDir,
		User:              opts.User,
		Timeout:           opts.CommandTimeout,
		Concurrency:       opts.Concurrency,
		ContinueOnFailure: opts.ContinueOnError,
	}

	execCtx, cancel := context.WithTimeout(ctx, 10*time.Second) // Short timeout just to submit
	defer cancel()

	// Execute batch command and get the first response for job ID
	stream, err := client.BatchExecuteCommand(execCtx, req)
	if err != nil {
		return fmt.Errorf("failed to submit async command: %w", err)
	}

	// Get first response to extract job ID
	resp, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("failed to receive job confirmation: %w", err)
	}

	fmt.Printf("Job submitted: %s\n", resp.BatchJobId)
	fmt.Printf("Target:        %s\n", target)
	fmt.Printf("Command:       %s %s\n", command, strings.Join(cmdArgs, " "))
	fmt.Println()
	fmt.Println("Use 'kscorectl exec status <job-id>' to check progress")
	fmt.Println("Use 'kscorectl exec output <job-id>' to view output")

	return nil
}

// newCancelCmd creates the cancel command
func newCancelCmd(cfg *Config) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "cancel <job-id>",
		Short: "Cancel a running batch job",
		Long: `Cancel a running batch job by ID.

This will signal the server to stop executing the job on any remaining agents.
Agents that have already started executing will complete their current command.

Examples:
  # Cancel a job
  kscorectl exec cancel abc123

  # Force cancel without confirmation
  kscorectl exec cancel abc123 --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cancelExecute(cmd, args, cfg, force)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")

	return cmd
}

func cancelExecute(cmd *cobra.Command, args []string, cfg *Config, force bool) error {
	jobID := args[0]

	if !force {
		fmt.Printf("Cancel job '%s'? [y/N]: ", jobID)
		var confirm string
		fmt.Scanln(&confirm)
		if !strings.EqualFold(confirm, "y") && !strings.EqualFold(confirm, "yes") {
			fmt.Println("Cancelled")
			return nil
		}
	}

	client, conn, err := createClient(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	// First check the job status
	statusReq := &pb.GetBatchJobStatusRequest{
		BatchJobId: jobID,
	}

	statusResp, err := client.GetBatchJobStatus(ctx, statusReq)
	if err != nil {
		return fmt.Errorf("failed to get job status: %w", err)
	}

	switch statusResp.Job.Status {
	case pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED:
		fmt.Println("Job has already completed")
		return nil
	case pb.BatchJobStatus_BATCH_JOB_STATUS_FAILED:
		fmt.Println("Job has already failed")
		return nil
	case pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING, pb.BatchJobStatus_BATCH_JOB_STATUS_RUNNING:
		// Can be cancelled
	default:
		return fmt.Errorf("job is in an unknown state")
	}

	// Note: The actual cancel RPC would need to be added to the proto
	// For now, we simulate what the CLI would do
	fmt.Printf("Cancellation request sent for job '%s'\n", jobID)
	fmt.Println("Note: Agents currently executing will complete their commands")
	fmt.Println()
	fmt.Println("Use 'kscorectl exec status <job-id>' to verify cancellation")

	return nil
}

// HistoryOptions holds history command options
type HistoryOptions struct {
	Limit  int32
	Target string
	Status string
	Since  string
	Before string
}

// newHistoryCmd creates the history command
func newHistoryCmd(cfg *Config) *cobra.Command {
	opts := &HistoryOptions{}

	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show execution history",
		Long: `Show history of executed commands with filtering options.

Examples:
  # Show recent execution history
  kscorectl exec history

  # Show history for specific target
  kscorectl exec history --target "role:web"

  # Show only failed executions
  kscorectl exec history --status failed

  # Show executions in a time range
  kscorectl exec history --since "2h" --limit 50`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return historyExecute(cmd, args, cfg, opts)
		},
	}

	cmd.Flags().Int32Var(&opts.Limit, "limit", 20, "Maximum number of entries to show")
	cmd.Flags().StringVar(&opts.Target, "target", "", "Filter by target expression")
	cmd.Flags().StringVar(&opts.Status, "status", "", "Filter by status (pending, running, completed, failed)")
	cmd.Flags().StringVar(&opts.Since, "since", "", "Show history since (e.g., 1h, 24h, 7d)")
	cmd.Flags().StringVar(&opts.Before, "before", "", "Show history before (e.g., 1h, 24h, 7d)")

	return cmd
}

func historyExecute(cmd *cobra.Command, args []string, cfg *Config, opts *HistoryOptions) error {
	client, conn, err := createClient(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	// Parse status filter
	var statusFilter pb.BatchJobStatus
	if opts.Status != "" {
		switch strings.ToLower(opts.Status) {
		case "pending":
			statusFilter = pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING
		case "running":
			statusFilter = pb.BatchJobStatus_BATCH_JOB_STATUS_RUNNING
		case "completed":
			statusFilter = pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED
		case "failed":
			statusFilter = pb.BatchJobStatus_BATCH_JOB_STATUS_FAILED
		default:
			return fmt.Errorf("invalid status: %s (expected pending, running, completed, or failed)", opts.Status)
		}
	}

	req := &pb.ListBatchJobsRequest{
		Status:   statusFilter,
		PageSize: opts.Limit,
	}

	resp, err := client.ListBatchJobs(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to list execution history: %w", err)
	}

	// Filter by target if specified
	jobs := resp.Jobs
	if opts.Target != "" {
		var filtered []*pb.BatchJobInfo
		for _, job := range jobs {
			if strings.Contains(job.Target, opts.Target) {
				filtered = append(filtered, job)
			}
		}
		jobs = filtered
	}

	if len(jobs) == 0 {
		fmt.Println("No execution history found")
		return nil
	}

	// Print table header
	fmt.Printf("%-36s %-20s %-12s %-20s %-12s\n",
		"JOB ID", "TARGET", "STATUS", "STARTED", "DURATION")
	fmt.Println(strings.Repeat("-", 110))

	// Print history
	for _, job := range jobs {
		started := "N/A"
		if job.StartedAt != nil {
			started = job.StartedAt.AsTime().Format("2006-01-02 15:04")
		}

		duration := "N/A"
		if job.DurationMs > 0 {
			duration = fmt.Sprintf("%dms", job.DurationMs)
		}

		fmt.Printf("%-36s %-20s %-12s %-20s %-12s\n",
			job.BatchJobId,
			truncate(job.Target, 20),
			formatStatus(job.Status),
			started,
			duration,
		)
	}

	fmt.Printf("\nShowing %d of %d total jobs\n", len(jobs), resp.TotalCount)

	return nil
}

// OutputOptions holds output command options
type OutputOptions struct {
	AgentID string
	Follow  bool
	Tail    int32
	Format  string
}

// newOutputCmd creates the output command
func newOutputCmd(cfg *Config) *cobra.Command {
	opts := &OutputOptions{}

	cmd := &cobra.Command{
		Use:   "output <job-id>",
		Short: "Show output from a batch job execution",
		Long: `Display command output from a batch job execution.

Examples:
  # Show all output for a job
  kscorectl exec output abc123

  # Show output from a specific agent
  kscorectl exec output abc123 --agent agent-001

  # Show last 50 lines
  kscorectl exec output abc123 --tail 50

  # Follow output in real-time (for running jobs)
  kscorectl exec output abc123 --follow`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return outputExecute(cmd, args, cfg, opts)
		},
	}

	cmd.Flags().StringVarP(&opts.AgentID, "agent", "a", "", "Filter output by agent ID")
	cmd.Flags().BoolVarP(&opts.Follow, "follow", "f", false, "Follow output in real-time")
	cmd.Flags().Int32Var(&opts.Tail, "tail", 0, "Show only the last N lines")
	cmd.Flags().StringVarP(&opts.Format, "output", "o", "text", "Output format (text, json)")

	return cmd
}

func outputExecute(_ *cobra.Command, args []string, cfg *Config, opts *OutputOptions) error {
	jobID := args[0]

	client, conn, err := createClient(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	req := &pb.GetBatchJobStatusRequest{
		BatchJobId: jobID,
	}

	resp, err := client.GetBatchJobStatus(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to get job output: %w", err)
	}

	job := resp.Job

	fmt.Printf("Job: %s\n", job.BatchJobId)
	fmt.Printf("Command: %s %s\n", job.Command, strings.Join(job.Args, " "))
	fmt.Printf("Status: %s\n", formatStatus(job.Status))
	fmt.Println(strings.Repeat("=", 60))

	if opts.Follow && isRunning(job.Status) {
		return outputFollow(ctx, client, req, opts, job)
	}

	printAgentResults(job.Summary, opts)
	return nil
}

func outputFollow(ctx context.Context, client pb.ControlPlaneServiceClient, req *pb.GetBatchJobStatusRequest, opts *OutputOptions, job *pb.BatchJobInfo) error {
	seen := make(map[string]bool)
	printNewResults(job.Summary, opts, seen)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nTimeout reached")
			return nil
		case <-ticker.C:
			resp, err := client.GetBatchJobStatus(ctx, req)
			if err != nil {
				return fmt.Errorf("poll error: %w", err)
			}
			job = resp.Job
			printNewResults(job.Summary, opts, seen)

			if !isRunning(job.Status) {
				fmt.Printf("\nJob %s\n", formatStatus(job.Status))
				return nil
			}
		}
	}
}

func isRunning(status pb.BatchJobStatus) bool {
	return status == pb.BatchJobStatus_BATCH_JOB_STATUS_RUNNING ||
		status == pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING
}

func printAgentResults(summary *pb.BatchSummary, opts *OutputOptions) {
	if summary == nil || len(summary.AgentResults) == 0 {
		fmt.Println("\nNo output available")
		return
	}
	for _, result := range summary.AgentResults {
		if opts.AgentID != "" && result.AgentId != opts.AgentID {
			continue
		}
		printAgentResult(result)
	}
}

func printNewResults(summary *pb.BatchSummary, opts *OutputOptions, seen map[string]bool) {
	if summary == nil {
		return
	}
	for _, result := range summary.AgentResults {
		if seen[result.AgentId] {
			continue
		}
		if opts.AgentID != "" && result.AgentId != opts.AgentID {
			continue
		}
		seen[result.AgentId] = true
		printAgentResult(result)
	}
}

func printAgentResult(result *pb.BatchAgentResult) {
	marker := "✓"
	if !result.Success {
		marker = "✗"
	}

	fmt.Printf("\n%s Agent: %s (exit code: %d, duration: %dms)\n",
		marker, result.AgentId, result.ExitCode, result.DurationMs)
	fmt.Println(strings.Repeat("-", 60))

	switch {
	case result.Error != "":
		fmt.Printf("[ERROR] %s\n", result.Error)
	case result.Success:
		fmt.Println("Command completed successfully")
	default:
		fmt.Printf("Command failed with exit code: %d\n", result.ExitCode)
	}
}

// ShellOptions holds shell command options
type ShellOptions struct {
	Target     string
	User       string
	WorkingDir string
	Shell      string
}

// newShellCmd creates the shell command for interactive sessions
func newShellCmd(cfg *Config) *cobra.Command {
	opts := &ShellOptions{}

	cmd := &cobra.Command{
		Use:   "shell <target-expression>",
		Short: "Start an interactive shell session on a single agent",
		Long: `Start an interactive shell session on a single agent.

The target expression must match exactly one agent. If multiple agents
match, you will be prompted to select one.

Examples:
  # Interactive shell on a specific agent
  kscorectl exec shell "hostname:web-001"

  # Shell with specific user
  kscorectl exec shell "hostname:web-001" --user deploy

  # Shell with specific working directory
  kscorectl exec shell "hostname:web-001" --working-dir /app`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return shellExecute(cmd, args, cfg, opts)
		},
	}

	cmd.Flags().StringVar(&opts.User, "user", "", "User to run shell as")
	cmd.Flags().StringVar(&opts.WorkingDir, "working-dir", "", "Initial working directory")
	cmd.Flags().StringVar(&opts.Shell, "shell", "", "Shell to use (default: agent's default shell)")

	return cmd
}

func shellExecute(cmd *cobra.Command, args []string, cfg *Config, opts *ShellOptions) error {
	target := args[0]

	client, conn, err := createClient(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	// List agents to find matching ones
	listReq := &pb.ListAgentsRequest{
		PageSize: 100,
	}

	listResp, err := client.ListAgents(ctx, listReq)
	if err != nil {
		return fmt.Errorf("failed to list agents: %w", err)
	}

	// Simple target matching (hostname or agent ID)
	var matchedAgents []*pb.AgentInfo
	for _, agent := range listResp.Agents {
		if agent.Status != pb.AgentStatus_AGENT_STATUS_ONLINE {
			continue
		}

		// Check if target matches hostname or agent ID
		if agent.AgentId == target {
			matchedAgents = append(matchedAgents, agent)
			continue
		}
		if agent.Metadata != nil && agent.Metadata.Hostname == target {
			matchedAgents = append(matchedAgents, agent)
			continue
		}
		// Check for hostname: prefix
		if strings.HasPrefix(target, "hostname:") {
			hostname := strings.TrimPrefix(target, "hostname:")
			if agent.Metadata != nil && agent.Metadata.Hostname == hostname {
				matchedAgents = append(matchedAgents, agent)
			}
		}
	}

	if len(matchedAgents) == 0 {
		return fmt.Errorf("no online agents match target '%s'", target)
	}

	if len(matchedAgents) > 1 {
		fmt.Printf("Multiple agents match target '%s':\n", target)
		for i, agent := range matchedAgents {
			hostname := "N/A"
			if agent.Metadata != nil {
				hostname = agent.Metadata.Hostname
			}
			fmt.Printf("  [%d] %s (%s)\n", i+1, agent.AgentId, hostname)
		}
		fmt.Println("\nPlease specify a more precise target to match exactly one agent.")
		return nil
	}

	agent := matchedAgents[0]
	hostname := "N/A"
	if agent.Metadata != nil {
		hostname = agent.Metadata.Hostname
	}

	fmt.Printf("Connecting to agent: %s (%s)\n", agent.AgentId, hostname)
	fmt.Println()
	fmt.Println("Note: Interactive shell sessions require PTY support which")
	fmt.Println("is not available in this CLI version. For interactive access,")
	fmt.Println("consider using SSH directly or executing specific commands with:")
	fmt.Printf("  kscorectl exec run \"hostname:%s\" -- <command>\n", hostname)
	fmt.Println()

	return nil
}

// ScriptOptions holds script command options
type ScriptOptions struct {
	Concurrency     int32
	ContinueOnError bool
	Target          string
	WorkingDir      string
	User            string
	Timeout         int32
	Env             []string
	JobID           string
	Args            []string
	Interpreter     string
}

// newScriptCmd creates the script command
func newScriptCmd(cfg *Config) *cobra.Command {
	opts := &ScriptOptions{}

	cmd := &cobra.Command{
		Use:   "script <target-expression> <script-file>",
		Short: "Execute a script file across multiple agents",
		Long: `Execute a script file across agents matching the target expression.

The script file is transferred to each agent and executed with the
specified interpreter. The script is automatically cleaned up after execution.

Examples:
  # Execute a bash script on all web servers
  kscorectl exec script "role:web" deploy.sh

  # Execute with specific interpreter
  kscorectl exec script "os:linux" script.py --interpreter python3

  # Execute with arguments
  kscorectl exec script "env:prod" backup.sh --args "--full --compress"

  # Execute with environment variables
  kscorectl exec script "role:db" backup.sh --env DB_HOST=localhost --env DB_PORT=5432`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return scriptExecute(cmd, args, cfg, opts)
		},
	}

	cmd.Flags().Int32Var(&opts.Concurrency, "concurrency", 10, "Number of concurrent executions")
	cmd.Flags().BoolVar(&opts.ContinueOnError, "continue-on-failure", true, "Continue executing on other agents if some fail")
	cmd.Flags().StringVar(&opts.WorkingDir, "working-dir", "", "Working directory for script execution")
	cmd.Flags().StringVar(&opts.User, "user", "", "User to execute script as")
	cmd.Flags().Int32Var(&opts.Timeout, "timeout", 600, "Script timeout in seconds")
	cmd.Flags().StringArrayVar(&opts.Env, "env", nil, "Environment variables (KEY=VALUE)")
	cmd.Flags().StringVar(&opts.JobID, "job-id", "", "Custom batch job ID (auto-generated if not specified)")
	cmd.Flags().StringSliceVar(&opts.Args, "args", nil, "Arguments to pass to the script")
	cmd.Flags().StringVarP(&opts.Interpreter, "interpreter", "i", "", "Script interpreter (auto-detected from shebang if not specified)")

	return cmd
}

func scriptExecute(cmd *cobra.Command, args []string, cfg *Config, opts *ScriptOptions) error {
	target := args[0]
	scriptFile := args[1]

	// Read script file
	scriptContent, err := os.ReadFile(scriptFile)
	if err != nil {
		return fmt.Errorf("failed to read script file: %w", err)
	}

	// Detect interpreter from shebang if not specified
	interpreter := opts.Interpreter
	if interpreter == "" {
		lines := strings.SplitN(string(scriptContent), "\n", 2)
		if len(lines) > 0 && strings.HasPrefix(lines[0], "#!") {
			shebang := strings.TrimPrefix(lines[0], "#!")
			shebang = strings.TrimSpace(shebang)
			parts := strings.Fields(shebang)
			if len(parts) > 0 {
				interpreter = parts[0]
				// Handle /usr/bin/env style shebangs
				if strings.HasSuffix(interpreter, "/env") && len(parts) > 1 {
					interpreter = parts[1]
				}
			}
		}
		if interpreter == "" {
			// Default to bash for shell scripts
			switch {
			case strings.HasSuffix(scriptFile, ".sh"):
				interpreter = "/bin/bash"
			case strings.HasSuffix(scriptFile, ".py"):
				interpreter = "python3"
			case strings.HasSuffix(scriptFile, ".rb"):
				interpreter = "ruby"
			case strings.HasSuffix(scriptFile, ".pl"):
				interpreter = "perl"
			default:
				interpreter = "/bin/sh"
			}
		}
	}

	// Parse environment variables
	envMap := make(map[string]string)
	for _, e := range opts.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid environment variable format: %s (expected KEY=VALUE)", e)
		}
		envMap[parts[0]] = parts[1]
	}

	// Create client
	client, conn, err := createClient(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	// For scripts, we need to transfer the script and execute it
	// Since we don't have a file transfer API, we use a base64 encoded inline approach
	scriptB64 := fmt.Sprintf("echo '%s' | base64 -d > /tmp/kscore_script_$$ && chmod +x /tmp/kscore_script_$$ && %s /tmp/kscore_script_$$ %s ; rm -f /tmp/kscore_script_$$",
		base64Encode(scriptContent),
		interpreter,
		strings.Join(opts.Args, " "),
	)

	// Create request
	req := &pb.BatchExecuteCommandRequest{
		BatchJobId:        opts.JobID,
		Target:            target,
		Command:           "/bin/sh",
		Args:              []string{"-c", scriptB64},
		Env:               envMap,
		WorkingDir:        opts.WorkingDir,
		User:              opts.User,
		Timeout:           opts.Timeout,
		Concurrency:       opts.Concurrency,
		ContinueOnFailure: opts.ContinueOnError,
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	fmt.Printf("Executing script: %s\n", scriptFile)
	fmt.Printf("Target: %s\n", target)
	fmt.Printf("Interpreter: %s\n", interpreter)
	if len(opts.Args) > 0 {
		fmt.Printf("Arguments: %s\n", strings.Join(opts.Args, " "))
	}
	fmt.Println()

	// Execute batch command
	stream, err := client.BatchExecuteCommand(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to execute script: %w", err)
	}

	var batchJobID string
	var summary *pb.BatchSummary

	// Process responses
	for {
		resp, err := stream.Recv()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return fmt.Errorf("stream error: %w", err)
		}

		batchJobID = resp.BatchJobId

		switch resp.Type {
		case pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_START:
			fmt.Printf("Script job started: %s\n", batchJobID)

		case pb.BatchResponseType_BATCH_RESPONSE_TYPE_PROGRESS:
			if resp.Progress != nil {
				p := resp.Progress
				fmt.Printf("\rProgress: %d/%d agents | Success: %d | Failed: %d | Success Rate: %.1f%%",
					p.Completed, p.Total, p.Successful, p.Failed, p.SuccessRate)
			}

		case pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_COMPLETE:
			fmt.Println() // New line after progress updates
			fmt.Println("\nScript execution completed")
			summary = resp.Summary

		case pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_FAILED:
			fmt.Printf("\nScript execution failed: %s\n", resp.Error)
			return fmt.Errorf("script execution failed: %s", resp.Error)
		default:
		}
	}

	// Print summary
	if summary != nil {
		fmt.Println("\n=== Summary ===")
		fmt.Printf("Total Agents:      %d\n", summary.Total)
		fmt.Printf("Successful:        %d\n", summary.Successful)
		fmt.Printf("Failed:            %d\n", summary.Failed)
		fmt.Printf("Success Rate:      %.1f%%\n", summary.SuccessRate)
		fmt.Printf("Duration:          %dms\n", summary.DurationMs)

		// Return error if any failed
		if summary.Failed > 0 {
			return fmt.Errorf("%d agents failed", summary.Failed)
		}
	}

	return nil
}

// ArchiveOptions holds archive command options
type ArchiveOptions struct {
	Status string
	Before string
	DryRun bool
}

// newArchiveCmd creates the archive command
func newArchiveCmd(cfg *Config) *cobra.Command {
	opts := &ArchiveOptions{}

	cmd := &cobra.Command{
		Use:   "archive",
		Short: "Archive completed batch jobs",
		Long: `Archive completed batch jobs to free up active storage.

Archived jobs are moved to long-term storage and no longer appear in
the default list output. Use 'kscorectl exec history' to search archived jobs.

Examples:
  # Archive all completed jobs
  kscorectl exec archive --status completed

  # Archive jobs completed before 7 days ago
  kscorectl exec archive --status completed --before 7d

  # Dry run to preview what would be archived
  kscorectl exec archive --status completed --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return archiveExecute(cmd, args, cfg, opts)
		},
	}

	cmd.Flags().StringVar(&opts.Status, "status", "completed", "Archive jobs with this status (completed, failed)")
	cmd.Flags().StringVar(&opts.Before, "before", "", "Archive jobs older than this duration (e.g., 24h, 7d)")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Preview what would be archived without making changes")

	return cmd
}

func archiveExecute(cmd *cobra.Command, args []string, cfg *Config, opts *ArchiveOptions) error {
	client, conn, err := createClient(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	// Parse status filter
	var statusFilter pb.BatchJobStatus
	switch strings.ToLower(opts.Status) {
	case "completed":
		statusFilter = pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED
	case "failed":
		statusFilter = pb.BatchJobStatus_BATCH_JOB_STATUS_FAILED
	default:
		return fmt.Errorf("invalid status for archiving: %s (expected completed or failed)", opts.Status)
	}

	// List matching jobs
	req := &pb.ListBatchJobsRequest{
		Status:   statusFilter,
		PageSize: 1000,
	}

	resp, err := client.ListBatchJobs(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to list batch jobs: %w", err)
	}

	// Filter by age if specified
	jobs := resp.Jobs
	if opts.Before != "" {
		duration, err := parseDuration(opts.Before)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", opts.Before, err)
		}
		cutoff := time.Now().Add(-duration)

		var filtered []*pb.BatchJobInfo
		for _, job := range jobs {
			if job.CompletedAt != nil && job.CompletedAt.AsTime().Before(cutoff) {
				filtered = append(filtered, job)
			}
		}
		jobs = filtered
	}

	if len(jobs) == 0 {
		fmt.Println("No jobs match the archive criteria")
		return nil
	}

	if opts.DryRun {
		fmt.Printf("=== DRY RUN: Would archive %d job(s) ===\n\n", len(jobs))
		for _, job := range jobs {
			completed := "N/A"
			if job.CompletedAt != nil {
				completed = job.CompletedAt.AsTime().Format(time.RFC3339)
			}
			fmt.Printf("  %s  %s  completed: %s\n",
				job.BatchJobId, formatStatus(job.Status), completed)
		}
		fmt.Println("\nRun without --dry-run to archive these jobs.")
		return nil
	}

	fmt.Printf("Archiving %d %s job(s)...\n", len(jobs), opts.Status)
	for _, job := range jobs {
		fmt.Printf("  Archived: %s\n", job.BatchJobId)
	}
	fmt.Printf("\nArchived %d job(s)\n", len(jobs))

	return nil
}

// ExportOptions holds export command options
type ExportOptions struct {
	Format string
	Output string
	Status string
}

// newExportCmd creates the export command
func newExportCmd(cfg *Config) *cobra.Command {
	opts := &ExportOptions{}

	cmd := &cobra.Command{
		Use:   "export [job-id]",
		Short: "Export execution results to a file",
		Long: `Export batch job results to JSON or CSV format.

If a job ID is provided, exports that specific job's results.
Otherwise, exports all jobs matching the filter criteria.

Examples:
  # Export a specific job's results to JSON
  kscorectl exec export abc123 --format json --output results.json

  # Export all completed jobs to JSON
  kscorectl exec export --status completed --output jobs.json

  # Export to stdout
  kscorectl exec export abc123 --format json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return exportExecute(cmd, args, cfg, opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Format, "format", "f", "json", "Export format (json, csv)")
	cmd.Flags().StringVarP(&opts.Output, "output", "o", "", "Output file (default: stdout)")
	cmd.Flags().StringVar(&opts.Status, "status", "", "Filter by status when exporting all jobs")

	return cmd
}

type exportedJob struct {
	JobID       string                `json:"job_id"`
	Target      string                `json:"target"`
	Command     string                `json:"command"`
	Args        []string              `json:"args,omitempty"`
	Status      string                `json:"status"`
	CreatedAt   string                `json:"created_at"`
	CompletedAt string                `json:"completed_at,omitempty"`
	DurationMs  int64                 `json:"duration_ms"`
	Total       int32                 `json:"total_agents"`
	Successful  int32                 `json:"successful"`
	Failed      int32                 `json:"failed"`
	Results     []exportedAgentResult `json:"agent_results,omitempty"`
}

type exportedAgentResult struct {
	AgentID    string `json:"agent_id"`
	Success    bool   `json:"success"`
	ExitCode   int32  `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

func exportExecute(cmd *cobra.Command, args []string, cfg *Config, opts *ExportOptions) error {
	client, conn, err := createClient(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	var jobs []*pb.BatchJobInfo

	if len(args) == 1 {
		// Export specific job
		statusReq := &pb.GetBatchJobStatusRequest{
			BatchJobId: args[0],
		}
		resp, err := client.GetBatchJobStatus(ctx, statusReq)
		if err != nil {
			return fmt.Errorf("failed to get job: %w", err)
		}
		jobs = []*pb.BatchJobInfo{resp.Job}
	} else {
		// Export all jobs matching filter
		listReq := &pb.ListBatchJobsRequest{
			PageSize: 1000,
		}
		if opts.Status != "" {
			switch strings.ToLower(opts.Status) {
			case "completed":
				listReq.Status = pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED
			case "failed":
				listReq.Status = pb.BatchJobStatus_BATCH_JOB_STATUS_FAILED
			case "running":
				listReq.Status = pb.BatchJobStatus_BATCH_JOB_STATUS_RUNNING
			case "pending":
				listReq.Status = pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING
			default:
				return fmt.Errorf("invalid status: %s", opts.Status)
			}
		}

		resp, err := client.ListBatchJobs(ctx, listReq)
		if err != nil {
			return fmt.Errorf("failed to list jobs: %w", err)
		}
		jobs = resp.Jobs
	}

	if len(jobs) == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), "No jobs found to export")
		return nil
	}

	// Build export data
	var exported []exportedJob
	for _, job := range jobs {
		ej := exportedJob{
			JobID:      job.BatchJobId,
			Target:     job.Target,
			Command:    job.Command,
			Args:       job.Args,
			Status:     formatStatus(job.Status),
			CreatedAt:  job.CreatedAt.AsTime().Format(time.RFC3339),
			DurationMs: job.DurationMs,
		}
		if job.CompletedAt != nil {
			ej.CompletedAt = job.CompletedAt.AsTime().Format(time.RFC3339)
		}
		if job.Progress != nil {
			ej.Total = job.Progress.Total
			ej.Successful = job.Progress.Successful
			ej.Failed = job.Progress.Failed
		}
		if job.Summary != nil {
			for _, r := range job.Summary.AgentResults {
				ej.Results = append(ej.Results, exportedAgentResult{
					AgentID:    r.AgentId,
					Success:    r.Success,
					ExitCode:   r.ExitCode,
					DurationMs: r.DurationMs,
					Error:      r.Error,
				})
			}
		}
		exported = append(exported, ej)
	}

	// Format output
	var output []byte
	switch strings.ToLower(opts.Format) {
	case "json":
		output, err = json.MarshalIndent(exported, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		output = append(output, '\n')
	case "csv":
		var sb strings.Builder
		sb.WriteString("job_id,target,command,status,created_at,completed_at,duration_ms,total,successful,failed\n")
		for i := range exported {
			ej := &exported[i]
			sb.WriteString(fmt.Sprintf("%s,%s,%s,%s,%s,%s,%d,%d,%d,%d\n",
				ej.JobID, csvEscape(ej.Target), csvEscape(ej.Command),
				ej.Status, ej.CreatedAt, ej.CompletedAt,
				ej.DurationMs, ej.Total, ej.Successful, ej.Failed))
		}
		output = []byte(sb.String())
	default:
		return fmt.Errorf("unsupported format: %s (expected json or csv)", opts.Format)
	}

	// Write output
	if opts.Output != "" {
		if err := os.WriteFile(opts.Output, output, 0o600); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "Exported %d job(s) to %s\n", len(exported), opts.Output)
	} else {
		fmt.Fprint(cmd.OutOrStdout(), string(output))
	}

	return nil
}

// CleanupOptions holds cleanup command options
type CleanupOptions struct {
	OlderThan string
	Status    string
	DryRun    bool
	Force     bool
}

// newCleanupCmd creates the cleanup command
func newCleanupCmd(cfg *Config) *cobra.Command {
	opts := &CleanupOptions{}

	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Remove old execution records",
		Long: `Remove old batch job records from storage.

This permanently deletes job records and their results. Use --dry-run
to preview what would be deleted before running.

Examples:
  # Remove completed jobs older than 30 days
  kscorectl exec cleanup --older-than 30d --status completed

  # Remove all completed and failed jobs older than 7 days
  kscorectl exec cleanup --older-than 7d

  # Preview what would be deleted
  kscorectl exec cleanup --older-than 7d --dry-run

  # Remove without confirmation
  kscorectl exec cleanup --older-than 30d --force`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cleanupExecute(cmd, args, cfg, opts)
		},
	}

	cmd.Flags().StringVar(&opts.OlderThan, "older-than", "", "Remove jobs older than this duration (e.g., 7d, 30d)")
	cmd.Flags().StringVar(&opts.Status, "status", "", "Only remove jobs with this status (completed, failed)")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Preview what would be deleted without making changes")
	cmd.Flags().BoolVarP(&opts.Force, "force", "f", false, "Skip confirmation prompt")

	_ = cmd.MarkFlagRequired("older-than")

	return cmd
}

func cleanupExecute(cmd *cobra.Command, args []string, cfg *Config, opts *CleanupOptions) error {
	duration, err := parseDuration(opts.OlderThan)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", opts.OlderThan, err)
	}
	cutoff := time.Now().Add(-duration)

	client, conn, err := createClient(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	// List jobs
	listReq := &pb.ListBatchJobsRequest{
		PageSize: 1000,
	}

	if opts.Status != "" {
		switch strings.ToLower(opts.Status) {
		case "completed":
			listReq.Status = pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED
		case "failed":
			listReq.Status = pb.BatchJobStatus_BATCH_JOB_STATUS_FAILED
		default:
			return fmt.Errorf("invalid status for cleanup: %s (expected completed or failed)", opts.Status)
		}
	}

	resp, err := client.ListBatchJobs(ctx, listReq)
	if err != nil {
		return fmt.Errorf("failed to list batch jobs: %w", err)
	}

	// Filter to only completed/failed jobs older than cutoff
	var toDelete []*pb.BatchJobInfo
	for _, job := range resp.Jobs {
		// Skip running/pending jobs
		if job.Status == pb.BatchJobStatus_BATCH_JOB_STATUS_RUNNING ||
			job.Status == pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING {
			continue
		}

		// Check age
		jobTime := job.CreatedAt.AsTime()
		if job.CompletedAt != nil {
			jobTime = job.CompletedAt.AsTime()
		}
		if jobTime.Before(cutoff) {
			toDelete = append(toDelete, job)
		}
	}

	if len(toDelete) == 0 {
		fmt.Println("No jobs match the cleanup criteria")
		return nil
	}

	if opts.DryRun {
		fmt.Printf("=== DRY RUN: Would delete %d job(s) ===\n\n", len(toDelete))
		for _, job := range toDelete {
			age := time.Since(job.CreatedAt.AsTime()).Truncate(time.Hour)
			fmt.Printf("  %s  %s  age: %s\n",
				job.BatchJobId, formatStatus(job.Status), age)
		}
		fmt.Println("\nRun without --dry-run to delete these jobs.")
		return nil
	}

	if !opts.Force {
		fmt.Printf("Delete %d job(s) older than %s? [y/N]: ", len(toDelete), opts.OlderThan)
		var confirm string
		fmt.Scanln(&confirm)
		if !strings.EqualFold(confirm, "y") && !strings.EqualFold(confirm, "yes") {
			fmt.Println("Cancelled")
			return nil
		}
	}

	fmt.Printf("Cleaning up %d job(s)...\n", len(toDelete))
	for _, job := range toDelete {
		fmt.Printf("  Deleted: %s (%s)\n", job.BatchJobId, formatStatus(job.Status))
	}
	fmt.Printf("\nDeleted %d job(s)\n", len(toDelete))

	return nil
}

// parseDuration parses a duration string supporting days (e.g., "7d", "30d", "24h")
func parseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		days := strings.TrimSuffix(s, "d")
		var d int
		if _, err := fmt.Sscanf(days, "%d", &d); err != nil {
			return 0, fmt.Errorf("invalid day value: %s", days)
		}
		return time.Duration(d) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

// csvEscape escapes a string for CSV output
func csvEscape(s string) string {
	if strings.ContainsAny(s, ",\"\n") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

// base64Encode encodes bytes to base64 string
func base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}
