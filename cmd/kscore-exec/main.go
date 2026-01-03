package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"github.com/shawnbutts/keystone-core/pkg/audit"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

var (
	// Global flags
	serverAddr  string
	timeout     time.Duration
	auditLevel  string
	auditOutput string

	rootCmd = &cobra.Command{
		Use:   "exec",
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

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			info := version.Get()
			fmt.Println(info.String())
		},
	}
)

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&serverAddr, "server", "localhost:50051", "Keystone Core server address")
	rootCmd.PersistentFlags().DurationVar(&timeout, "timeout", 5*time.Minute, "Request timeout")
	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")

	// Add subcommands
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(listCmd)
}

func main() {
	// Initialize audit logging (Epic 15)
	auditConfig := &audit.AuditConfig{
		Level:   audit.AuditLevel(auditLevel),
		Backend: auditOutput,
	}
	if err := audit.Init("kscore-exec", auditConfig); err != nil {
		// Don't fail if audit logging can't be initialized, just warn
		fmt.Fprintf(os.Stderr, "Warning: failed to initialize audit logging: %v\n", err)
	}
	defer audit.Close()

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// createClient creates a gRPC client connection to the control plane
func createClient() (pb.ControlPlaneServiceClient, *grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// TODO: Add TLS support
	conn, err := grpc.DialContext(ctx, serverAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to server: %w", err)
	}

	client := pb.NewControlPlaneServiceClient(conn)
	return client, conn, nil
}

// Run command

var (
	runConcurrency      int32
	runContinueOnError  bool
	runWorkingDir       string
	runUser             string
	runCommandTimeout   int32
	runEnv              []string
	runJobID            string
	runShowProgress     bool
	runShowAgentResults bool
)

var runCmd = &cobra.Command{
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

  # Execute with environment variables
  kscorectl exec run "role:db" --env DB_HOST=localhost -- ./backup.sh

  # Execute with custom concurrency
  kscorectl exec run "env:prod" --concurrency 10 -- uptime`,
	Args: cobra.MinimumNArgs(1),
	RunE: runExecute,
}

func init() {
	runCmd.Flags().Int32Var(&runConcurrency, "concurrency", 10, "Number of concurrent executions")
	runCmd.Flags().BoolVar(&runContinueOnError, "continue-on-failure", true, "Continue executing on other agents if some fail")
	runCmd.Flags().StringVar(&runWorkingDir, "working-dir", "", "Working directory for command execution")
	runCmd.Flags().StringVar(&runUser, "user", "", "User to execute command as")
	runCmd.Flags().Int32Var(&runCommandTimeout, "command-timeout", 300, "Command timeout in seconds")
	runCmd.Flags().StringArrayVar(&runEnv, "env", nil, "Environment variables (KEY=VALUE)")
	runCmd.Flags().StringVar(&runJobID, "job-id", "", "Custom batch job ID (auto-generated if not specified)")
	runCmd.Flags().BoolVar(&runShowProgress, "show-progress", true, "Show progress updates during execution")
	runCmd.Flags().BoolVar(&runShowAgentResults, "show-results", true, "Show per-agent results at the end")
}

func runExecute(cmd *cobra.Command, args []string) error {
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

	if len(args) < 1 {
		err := fmt.Errorf("target expression is required")
		logAudit(audit.ResultFailure, 1, err)
		return err
	}

	target := args[0]
	auditEntry.Target = target

	// Find the command after "--"
	var command string
	var cmdArgs []string

	for i, arg := range args {
		if arg == "--" {
			if i+1 >= len(args) {
				err := fmt.Errorf("command is required after '--'")
				logAudit(audit.ResultFailure, 1, err)
				return err
			}
			command = args[i+1]
			if i+2 < len(args) {
				cmdArgs = args[i+2:]
			}
			break
		}
	}

	if command == "" {
		// No "--" separator, assume everything after target is the command
		if len(args) < 2 {
			err := fmt.Errorf("command is required")
			logAudit(audit.ResultFailure, 1, err)
			return err
		}
		command = args[1]
		if len(args) > 2 {
			cmdArgs = args[2:]
		}
	}

	// Parse environment variables
	envMap := make(map[string]string)
	for _, e := range runEnv {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			err := fmt.Errorf("invalid environment variable format: %s (expected KEY=VALUE)", e)
			logAudit(audit.ResultFailure, 1, err)
			return err
		}
		envMap[parts[0]] = parts[1]
	}

	// Create client
	client, conn, err := createClient()
	if err != nil {
		logAudit(audit.ResultFailure, 1, err)
		return err
	}
	defer conn.Close()

	// Create request
	req := &pb.BatchExecuteCommandRequest{
		BatchJobId:        runJobID,
		Target:            target,
		Command:           command,
		Args:              cmdArgs,
		Env:               envMap,
		WorkingDir:        runWorkingDir,
		User:              runUser,
		Timeout:           runCommandTimeout,
		Concurrency:       runConcurrency,
		ContinueOnFailure: runContinueOnError,
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
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
			if runShowProgress && resp.Progress != nil {
				p := resp.Progress
				fmt.Printf("\rProgress: %d/%d agents | Success: %d | Failed: %d | Success Rate: %.1f%%",
					p.Completed, p.Total, p.Successful, p.Failed, p.SuccessRate)
			}

		case pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_COMPLETE:
			if runShowProgress {
				fmt.Println() // New line after progress updates
			}
			fmt.Println("\nBatch execution completed")
			summary = resp.Summary

		case pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_FAILED:
			fmt.Printf("\nBatch execution failed: %s\n", resp.Error)
			err := fmt.Errorf("batch execution failed: %s", resp.Error)
			logAudit(audit.ResultFailure, 1, err)
			return err
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

		if runShowAgentResults && len(summary.AgentResults) > 0 {
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

	// Exit with error if batch failed
	if summary != nil && summary.Failed > 0 {
		auditEntry.AgentsMatched = int(summary.Total)
		auditEntry.Extra = map[string]interface{}{
			"successful": summary.Successful,
			"failed":     summary.Failed,
		}
		logAudit(audit.ResultFailure, 1, fmt.Errorf("%d agents failed", summary.Failed))
		os.Exit(1)
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

// Status command

var statusCmd = &cobra.Command{
	Use:   "status <job-id>",
	Short: "Get the status of a batch job",
	Long: `Retrieve detailed status information about a batch job.

Examples:
  kscorectl exec status abc123
  kscorectl exec status --server prod-server:50051 abc123`,
	Args: cobra.ExactArgs(1),
	RunE: statusExecute,
}

func statusExecute(cmd *cobra.Command, args []string) error {
	jobID := args[0]

	client, conn, err := createClient()
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
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

// List command

var (
	listStatus   string
	listPageSize int32
)

var listCmd = &cobra.Command{
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
	RunE: listExecute,
}

func init() {
	listCmd.Flags().StringVar(&listStatus, "status", "", "Filter by status (pending, running, completed, failed)")
	listCmd.Flags().Int32Var(&listPageSize, "page-size", 20, "Number of jobs to return")
}

func listExecute(cmd *cobra.Command, args []string) error {
	client, conn, err := createClient()
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Parse status filter
	var statusFilter pb.BatchJobStatus
	if listStatus != "" {
		switch strings.ToLower(listStatus) {
		case "pending":
			statusFilter = pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING
		case "running":
			statusFilter = pb.BatchJobStatus_BATCH_JOB_STATUS_RUNNING
		case "completed":
			statusFilter = pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED
		case "failed":
			statusFilter = pb.BatchJobStatus_BATCH_JOB_STATUS_FAILED
		default:
			return fmt.Errorf("invalid status: %s (expected pending, running, completed, or failed)", listStatus)
		}
	}

	req := &pb.ListBatchJobsRequest{
		Status:   statusFilter,
		PageSize: listPageSize,
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
