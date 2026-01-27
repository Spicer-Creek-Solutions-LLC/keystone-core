package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/cli/auditutil"
	"github.com/shawnbutts/keystone-core/internal/cli/output"
	"github.com/shawnbutts/keystone-core/internal/runbook/approval"
	"github.com/shawnbutts/keystone-core/internal/runbook/intervention"
	"github.com/shawnbutts/keystone-core/pkg/version"

	_ "modernc.org/sqlite"
)

var (
	serverAddr   string
	outputFormat string
	auditLevel   string
	auditOutput  string
	dbPath       string
)

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-runbook",
		Short: "Runbook management for Keystone Core",
		Long: `kscore-runbook provides runbook execution and management capabilities.

This command provides:
  - Runbook listing and execution
  - Approval management (list, approve, reject)
  - Intervention management (list, respond)
  - Execution history and status

Commands:
  approvals    - Manage pending approvals
  approve      - Approve a pending request
  reject       - Reject a pending request
  delegate     - Delegate an approval to another user
  interventions - Manage pending interventions
  respond      - Respond to an intervention request

Examples:
  # List pending approvals
  kscore-runbook approvals

  # Approve a request
  kscore-runbook approve req-123 --reason "Verified prerequisites"

  # Reject a request
  kscore-runbook reject req-123 --reason "Not ready for production"

  # List pending interventions
  kscore-runbook interventions

  # Respond to an intervention
  kscore-runbook respond int-456 --value version=1.0.0 --confirmed`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().StringVarP(&serverAddr, "server", "s", "localhost:9090", "Control plane server address")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "format", "o", "table", "Output format (table, text, json, yaml)")
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "", "Path to runbook database (for local testing)")
	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend")

	rootCmd.AddCommand(
		newVersionCmd(),
		newApprovalsCmd(),
		newApproveCmd(),
		newRejectCmd(),
		newDelegateCmd(),
		newInterventionsCmd(),
		newRespondCmd(),
	)

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
	rootCmd := newRootCmd()
	auditHandler := auditutil.Attach(rootCmd, "kscore-runbook", &auditLevel, &auditOutput)
	if err := rootCmd.Execute(); err != nil {
		auditHandler.LogFailure(err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// ============================================================================
// Approval Commands
// ============================================================================

var (
	approvalsMine      bool
	approvalsState     string
	approvalsExecID    string
	approvalsLimit     int
)

func newApprovalsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "approvals",
		Short: "List pending approval requests",
		Long: `List approval requests that are pending or in other states.

Examples:
  # List all pending approvals
  kscore-runbook approvals

  # List only your approvals
  kscore-runbook approvals --mine

  # Filter by state
  kscore-runbook approvals --state approved

  # Filter by execution
  kscore-runbook approvals --execution exec-123`,
		RunE: runApprovals,
	}

	cmd.Flags().BoolVar(&approvalsMine, "mine", false, "Show only approvals assigned to me")
	cmd.Flags().StringVar(&approvalsState, "state", "", "Filter by state (pending, approved, rejected, expired, cancelled)")
	cmd.Flags().StringVar(&approvalsExecID, "execution", "", "Filter by execution ID")
	cmd.Flags().IntVar(&approvalsLimit, "limit", 50, "Maximum number of results")

	return cmd
}

func runApprovals(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	storage, cleanup, err := getApprovalStorage()
	if err != nil {
		return err
	}
	defer cleanup()

	opts := approval.ListOptions{
		Limit: approvalsLimit,
	}

	if approvalsExecID != "" {
		opts.ExecutionID = approvalsExecID
	}

	if approvalsState != "" {
		opts.State = approval.RequestState(approvalsState)
	}

	requests, err := storage.ListRequests(ctx, opts)
	if err != nil {
		return fmt.Errorf("list approvals: %w", err)
	}

	format, err := output.ParseFormat(outputFormat)
	if err != nil {
		return err
	}

	if len(requests) == 0 {
		switch format {
		case output.FormatJSON:
			return output.WriteJSON(os.Stdout, requests)
		case output.FormatYAML:
			return output.WriteYAML(os.Stdout, requests)
		default:
			fmt.Println("No approval requests found.")
			return nil
		}
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(os.Stdout, requests)
	case output.FormatYAML:
		return output.WriteYAML(os.Stdout, requests)
	case output.FormatTable, output.FormatText:
		fmt.Printf("%-12s %-30s %-12s %-10s %-20s\n", "ID", "TITLE", "STATE", "RESPONSES", "CREATED")
		fmt.Println(strings.Repeat("-", 90))
		for _, req := range requests {
			fmt.Printf("%-12s %-30s %-12s %-10d %-20s\n",
				truncate(req.ID, 12),
				truncate(req.Title, 30),
				req.State,
				len(req.Responses),
				req.CreatedAt.Format("2006-01-02 15:04"),
			)
		}
		fmt.Printf("\nTotal: %d approval requests\n", len(requests))
		return nil
	default:
		return fmt.Errorf("unsupported output format: %s", outputFormat)
	}
}

var (
	approveReason string
)

func newApproveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "approve <request-id>",
		Short: "Approve a pending approval request",
		Long: `Approve a pending approval request.

Examples:
  # Approve a request
  kscore-runbook approve req-123 --reason "Verified prerequisites"

  # Approve without reason (if allowed)
  kscore-runbook approve req-123`,
		Args: cobra.ExactArgs(1),
		RunE: runApprove,
	}

	cmd.Flags().StringVar(&approveReason, "reason", "", "Reason for approval")

	return cmd
}

func runApprove(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	requestID := args[0]

	storage, cleanup, err := getApprovalStorage()
	if err != nil {
		return err
	}
	defer cleanup()

	manager := approval.NewManager(storage)

	// Get current user (in production, from auth context)
	user := getCurrentUser()

	req, err := manager.Respond(ctx, requestID, user, approval.DecisionApproved, approveReason)
	if err != nil {
		return fmt.Errorf("approve: %w", err)
	}

	fmt.Printf("Approved request %s\n", requestID)
	fmt.Printf("  Title:  %s\n", req.Title)
	fmt.Printf("  State:  %s\n", req.State)
	fmt.Printf("  By:     %s\n", user)

	return nil
}

var (
	rejectReason string
)

func newRejectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reject <request-id>",
		Short: "Reject a pending approval request",
		Long: `Reject a pending approval request.

Examples:
  # Reject a request with reason
  kscore-runbook reject req-123 --reason "Replication lag too high"`,
		Args: cobra.ExactArgs(1),
		RunE: runReject,
	}

	cmd.Flags().StringVar(&rejectReason, "reason", "", "Reason for rejection (required)")
	cmd.MarkFlagRequired("reason")

	return cmd
}

func runReject(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	requestID := args[0]

	storage, cleanup, err := getApprovalStorage()
	if err != nil {
		return err
	}
	defer cleanup()

	manager := approval.NewManager(storage)

	user := getCurrentUser()

	req, err := manager.Respond(ctx, requestID, user, approval.DecisionRejected, rejectReason)
	if err != nil {
		return fmt.Errorf("reject: %w", err)
	}

	fmt.Printf("Rejected request %s\n", requestID)
	fmt.Printf("  Title:  %s\n", req.Title)
	fmt.Printf("  State:  %s\n", req.State)
	fmt.Printf("  Reason: %s\n", rejectReason)

	return nil
}

var (
	delegateTo string
)

func newDelegateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delegate <request-id>",
		Short: "Delegate an approval request to another user",
		Long: `Delegate an approval request to another user or group.

Examples:
  # Delegate to another user
  kscore-runbook delegate req-123 --to @another-approver

  # Delegate to a group
  kscore-runbook delegate req-123 --to @platform-team`,
		Args: cobra.ExactArgs(1),
		RunE: runDelegate,
	}

	cmd.Flags().StringVar(&delegateTo, "to", "", "User or group to delegate to (required)")
	cmd.MarkFlagRequired("to")

	return cmd
}

func runDelegate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	requestID := args[0]

	storage, cleanup, err := getApprovalStorage()
	if err != nil {
		return err
	}
	defer cleanup()

	// Get the request
	req, err := storage.GetRequest(ctx, requestID)
	if err != nil {
		return fmt.Errorf("get request: %w", err)
	}
	if req == nil {
		return fmt.Errorf("request not found: %s", requestID)
	}

	if req.State != approval.RequestStatePending {
		return fmt.Errorf("can only delegate pending requests (current state: %s)", req.State)
	}

	// Add delegate to approvers
	user := getCurrentUser()
	req.Approvers = append(req.Approvers, delegateTo)
	req.UpdatedAt = time.Now()

	// Record delegation in metadata
	if req.Metadata == nil {
		req.Metadata = make(map[string]interface{})
	}
	delegations, _ := req.Metadata["delegations"].([]interface{})
	delegations = append(delegations, map[string]interface{}{
		"from":         user,
		"to":           delegateTo,
		"delegated_at": time.Now().Format(time.RFC3339),
	})
	req.Metadata["delegations"] = delegations

	if err := storage.SaveRequest(ctx, req); err != nil {
		return fmt.Errorf("save request: %w", err)
	}

	fmt.Printf("Delegated request %s to %s\n", requestID, delegateTo)
	fmt.Printf("  Title:     %s\n", req.Title)
	fmt.Printf("  Approvers: %s\n", strings.Join(req.Approvers, ", "))

	return nil
}

// ============================================================================
// Intervention Commands
// ============================================================================

var (
	interventionsState  string
	interventionsExecID string
	interventionsLimit  int
)

func newInterventionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "interventions",
		Short: "List pending intervention requests",
		Long: `List intervention requests (prompts, confirmations, manual waits).

Examples:
  # List all pending interventions
  kscore-runbook interventions

  # Filter by state
  kscore-runbook interventions --state pending

  # Filter by execution
  kscore-runbook interventions --execution exec-123`,
		RunE: runInterventions,
	}

	cmd.Flags().StringVar(&interventionsState, "state", "", "Filter by state (pending, completed, expired, cancelled)")
	cmd.Flags().StringVar(&interventionsExecID, "execution", "", "Filter by execution ID")
	cmd.Flags().IntVar(&interventionsLimit, "limit", 50, "Maximum number of results")

	return cmd
}

func runInterventions(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	storage, cleanup, err := getInterventionStorage()
	if err != nil {
		return err
	}
	defer cleanup()

	opts := intervention.ListOptions{
		Limit: interventionsLimit,
	}

	if interventionsExecID != "" {
		opts.ExecutionID = interventionsExecID
	}

	if interventionsState != "" {
		opts.State = intervention.InterventionState(interventionsState)
	}

	requests, err := storage.ListRequests(ctx, opts)
	if err != nil {
		return fmt.Errorf("list interventions: %w", err)
	}

	format, err := output.ParseFormat(outputFormat)
	if err != nil {
		return err
	}

	if len(requests) == 0 {
		switch format {
		case output.FormatJSON:
			return output.WriteJSON(os.Stdout, requests)
		case output.FormatYAML:
			return output.WriteYAML(os.Stdout, requests)
		default:
			fmt.Println("No intervention requests found.")
			return nil
		}
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(os.Stdout, requests)
	case output.FormatYAML:
		return output.WriteYAML(os.Stdout, requests)
	case output.FormatTable, output.FormatText:
		fmt.Printf("%-12s %-12s %-30s %-12s %-20s\n", "ID", "TYPE", "TITLE", "STATE", "CREATED")
		fmt.Println(strings.Repeat("-", 90))
		for _, req := range requests {
			fmt.Printf("%-12s %-12s %-30s %-12s %-20s\n",
				truncate(req.ID, 12),
				req.Type,
				truncate(req.Title, 30),
				req.State,
				req.CreatedAt.Format("2006-01-02 15:04"),
			)
		}
		fmt.Printf("\nTotal: %d intervention requests\n", len(requests))
		return nil
	default:
		return fmt.Errorf("unsupported output format: %s", outputFormat)
	}
}

var (
	respondValues    []string
	respondConfirmed bool
	respondComment   string
)

func newRespondCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "respond <request-id>",
		Short: "Respond to an intervention request",
		Long: `Respond to an intervention request (prompt, confirm, or wait_manual).

For prompt-type interventions, provide values using --value flags.
For confirm-type interventions, use --confirmed or --declined.
For wait_manual interventions, just acknowledge.

Examples:
  # Respond to a prompt
  kscore-runbook respond int-123 --value version=1.0.0 --value replicas=3

  # Confirm an action
  kscore-runbook respond int-456 --confirmed --comment "Looks good"

  # Decline a confirmation
  kscore-runbook respond int-456 --comment "Not ready"

  # Acknowledge a manual wait
  kscore-runbook respond int-789 --confirmed --comment "Verified manually"`,
		Args: cobra.ExactArgs(1),
		RunE: runRespond,
	}

	cmd.Flags().StringArrayVar(&respondValues, "value", nil, "Set a value (format: name=value)")
	cmd.Flags().BoolVar(&respondConfirmed, "confirmed", false, "Confirm the request")
	cmd.Flags().StringVar(&respondComment, "comment", "", "Optional comment")

	return cmd
}

func runRespond(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	requestID := args[0]

	storage, cleanup, err := getInterventionStorage()
	if err != nil {
		return err
	}
	defer cleanup()

	manager := intervention.NewManager(storage)

	// Parse values
	values := make(map[string]interface{})
	for _, v := range respondValues {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid value format %q (expected name=value)", v)
		}
		values[parts[0]] = parts[1]
	}

	user := getCurrentUser()

	req, err := manager.Respond(ctx, requestID, user, values, respondConfirmed, respondComment)
	if err != nil {
		return fmt.Errorf("respond: %w", err)
	}

	fmt.Printf("Responded to intervention %s\n", requestID)
	fmt.Printf("  Title:     %s\n", req.Title)
	fmt.Printf("  Type:      %s\n", req.Type)
	fmt.Printf("  State:     %s\n", req.State)
	if respondConfirmed {
		fmt.Printf("  Confirmed: yes\n")
	}
	if len(values) > 0 {
		fmt.Printf("  Values:    %v\n", values)
	}

	return nil
}

// ============================================================================
// Helper Functions
// ============================================================================

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func getCurrentUser() string {
	// In production, this would come from the auth context
	// For now, use the OS user or a default
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	return "operator"
}

func getApprovalStorage() (approval.Storage, func(), error) {
	if dbPath == "" {
		// In production, connect to control plane API
		// For testing, use a default path
		dbPath = os.Getenv("KSCORE_RUNBOOK_DB")
		if dbPath == "" {
			return nil, func() {}, fmt.Errorf("database path required (--db or KSCORE_RUNBOOK_DB)")
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, func() {}, fmt.Errorf("open database: %w", err)
	}

	storage, err := approval.NewSQLiteStorage(db)
	if err != nil {
		db.Close()
		return nil, func() {}, fmt.Errorf("init approval storage: %w", err)
	}

	return storage, func() { db.Close() }, nil
}

func getInterventionStorage() (intervention.Storage, func(), error) {
	if dbPath == "" {
		dbPath = os.Getenv("KSCORE_RUNBOOK_DB")
		if dbPath == "" {
			return nil, func() {}, fmt.Errorf("database path required (--db or KSCORE_RUNBOOK_DB)")
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, func() {}, fmt.Errorf("open database: %w", err)
	}

	storage, err := intervention.NewSQLiteStorage(db)
	if err != nil {
		db.Close()
		return nil, func() {}, fmt.Errorf("init intervention storage: %w", err)
	}

	return storage, func() { db.Close() }, nil
}
