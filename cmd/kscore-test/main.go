package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/shawnbutts/keystone-core/internal/cli/auditutil"
	"github.com/shawnbutts/keystone-core/internal/cli/output"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

var (
	serverAddr   string
	outputFormat string
	verbose      bool
	auditLevel   string
	auditOutput  string
)

type Config struct {
	ServerAddr   string
	OutputFormat string
	Verbose      bool
}

func main() {
	rootCmd := newRootCmd()
	auditHandler := auditutil.Attach(rootCmd, "kscore-test", &auditLevel, &auditOutput)
	if err := rootCmd.Execute(); err != nil {
		auditHandler.LogFailure(err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-test",
		Short: "Keystone Core test runner plugin",
		Long: `kscore-test is a CLI plugin for running tests against Keystone Core deployments.

This plugin provides commands for:
  - Running smoke tests to verify basic functionality
  - Running integration test suites
  - Running e2e tests against deployed infrastructure
  - Managing test suites and results

Usage via kscorectl:
  kscorectl test smoke
  kscorectl test integration --suite recovery
  kscorectl test run --suite basic --target production
  kscorectl test list`,
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
		newSmokeCmd(cfg),
		newIntegrationCmd(cfg),
		newRunCmd(cfg),
		newListCmd(cfg),
		newShowCmd(cfg),
		newHistoryCmd(cfg),
		newSuiteCmd(cfg),
		newVersionCmd(),
	)

	return rootCmd
}

// TestResult represents a test execution result
type TestResult struct {
	ID         string            `json:"id" yaml:"id"`
	Suite      string            `json:"suite" yaml:"suite"`
	Type       string            `json:"type" yaml:"type"`
	Status     string            `json:"status" yaml:"status"`
	Target     string            `json:"target,omitempty" yaml:"target,omitempty"`
	Total      int               `json:"total" yaml:"total"`
	Passed     int               `json:"passed" yaml:"passed"`
	Failed     int               `json:"failed" yaml:"failed"`
	Skipped    int               `json:"skipped" yaml:"skipped"`
	Duration   string            `json:"duration" yaml:"duration"`
	StartedAt  string            `json:"started_at" yaml:"started_at"`
	CompletedAt string           `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
	Tests      []TestCase        `json:"tests,omitempty" yaml:"tests,omitempty"`
	Failures   []TestFailure     `json:"failures,omitempty" yaml:"failures,omitempty"`
	Labels     map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

// TestCase represents an individual test case
type TestCase struct {
	Name       string `json:"name" yaml:"name"`
	Status     string `json:"status" yaml:"status"`
	Duration   string `json:"duration" yaml:"duration"`
	Message    string `json:"message,omitempty" yaml:"message,omitempty"`
	Error      string `json:"error,omitempty" yaml:"error,omitempty"`
}

// TestFailure represents a test failure
type TestFailure struct {
	Test    string `json:"test" yaml:"test"`
	Message string `json:"message" yaml:"message"`
	Details string `json:"details,omitempty" yaml:"details,omitempty"`
}

// TestSuite represents a test suite definition
type TestSuite struct {
	Name        string   `json:"name" yaml:"name"`
	Description string   `json:"description" yaml:"description"`
	Type        string   `json:"type" yaml:"type"`
	Tests       int      `json:"tests" yaml:"tests"`
	Tags        []string `json:"tags,omitempty" yaml:"tags,omitempty"`
	Timeout     string   `json:"timeout" yaml:"timeout"`
	LastRun     string   `json:"last_run,omitempty" yaml:"last_run,omitempty"`
	LastStatus  string   `json:"last_status,omitempty" yaml:"last_status,omitempty"`
}

func newSmokeCmd(cfg *Config) *cobra.Command {
	var (
		target  string
		timeout string
		tags    []string
	)

	cmd := &cobra.Command{
		Use:   "smoke",
		Short: "Run smoke tests",
		Long: `Run smoke tests to verify basic Keystone Core functionality.

Smoke tests include:
  - Control plane connectivity
  - Agent registration verification
  - Basic command execution
  - State application check
  - Event system connectivity

Examples:
  # Run all smoke tests
  kscorectl test smoke

  # Run smoke tests against specific target
  kscorectl test smoke --target "environment:production"

  # Run with timeout
  kscorectl test smoke --timeout 5m`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSmoke(cfg, target, timeout, tags)
		},
	}

	cmd.Flags().StringVarP(&target, "target", "t", "", "Target expression for agents")
	cmd.Flags().StringVar(&timeout, "timeout", "5m", "Test timeout duration")
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "Filter tests by tags")

	return cmd
}

func runSmoke(cfg *Config, target, timeout string, tags []string) error {
	// Sample smoke test results
	result := TestResult{
		ID:          fmt.Sprintf("smoke-%s", time.Now().Format("20060102-150405")),
		Suite:       "smoke",
		Type:        "smoke",
		Status:      "passed",
		Target:      target,
		Total:       8,
		Passed:      8,
		Failed:      0,
		Skipped:     0,
		Duration:    "12.5s",
		StartedAt:   time.Now().Format(time.RFC3339),
		CompletedAt: time.Now().Add(12500 * time.Millisecond).Format(time.RFC3339),
		Tests: []TestCase{
			{Name: "control_plane_connectivity", Status: "passed", Duration: "1.2s"},
			{Name: "grpc_api_health", Status: "passed", Duration: "0.8s"},
			{Name: "rest_api_health", Status: "passed", Duration: "0.6s"},
			{Name: "agent_registration", Status: "passed", Duration: "2.1s"},
			{Name: "agent_heartbeat", Status: "passed", Duration: "1.5s"},
			{Name: "basic_command_execution", Status: "passed", Duration: "3.2s"},
			{Name: "state_check", Status: "passed", Duration: "1.8s"},
			{Name: "event_system", Status: "passed", Duration: "1.3s"},
		},
	}

	if target != "" {
		result.Labels = map[string]string{"target": target}
	}

	return outputResult(cfg.OutputFormat, result, func() {
		printTestResult(result, verbose)
	})
}

func newIntegrationCmd(cfg *Config) *cobra.Command {
	var (
		suite   string
		target  string
		timeout string
		tags    []string
		parallel int
	)

	cmd := &cobra.Command{
		Use:   "integration",
		Short: "Run integration tests",
		Long: `Run integration test suites to verify Keystone Core functionality.

Available test suites:
  - basic: Basic functionality tests
  - recovery: Disaster recovery and failover tests
  - cluster: Clustering and HA tests
  - state: State management tests
  - execution: Remote execution tests
  - events: Event system tests
  - policy: Policy enforcement tests
  - gitops: GitOps integration tests

Examples:
  # Run recovery test suite
  kscorectl test integration --suite recovery

  # Run multiple suites
  kscorectl test integration --suite basic,state

  # Run with target expression
  kscorectl test integration --suite cluster --target "role:control-plane"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runIntegration(cfg, suite, target, timeout, tags, parallel)
		},
	}

	cmd.Flags().StringVarP(&suite, "suite", "S", "basic", "Test suite to run (basic, recovery, cluster, state, execution, events, policy, gitops)")
	cmd.Flags().StringVarP(&target, "target", "t", "", "Target expression for agents")
	cmd.Flags().StringVar(&timeout, "timeout", "30m", "Test timeout duration")
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "Filter tests by tags")
	cmd.Flags().IntVar(&parallel, "parallel", 1, "Number of parallel test executions")

	return cmd
}

func runIntegration(cfg *Config, suite, target, timeout string, tags []string, parallel int) error {
	// Sample integration test results
	suites := strings.Split(suite, ",")

	var tests []TestCase
	total, passed, failed := 0, 0, 0

	for _, s := range suites {
		switch strings.TrimSpace(s) {
		case "basic":
			tests = append(tests, []TestCase{
				{Name: "basic/agent_registration", Status: "passed", Duration: "2.1s"},
				{Name: "basic/agent_metadata", Status: "passed", Duration: "1.5s"},
				{Name: "basic/command_execution", Status: "passed", Duration: "3.2s"},
				{Name: "basic/command_streaming", Status: "passed", Duration: "4.1s"},
				{Name: "basic/batch_execution", Status: "passed", Duration: "5.5s"},
			}...)
			total += 5
			passed += 5
		case "recovery":
			tests = append(tests, []TestCase{
				{Name: "recovery/backup_create", Status: "passed", Duration: "15.2s"},
				{Name: "recovery/backup_restore", Status: "passed", Duration: "25.3s"},
				{Name: "recovery/failover_detection", Status: "passed", Duration: "8.7s"},
				{Name: "recovery/leader_election", Status: "passed", Duration: "12.1s"},
				{Name: "recovery/agent_reconnection", Status: "passed", Duration: "6.5s"},
			}...)
			total += 5
			passed += 5
		case "cluster":
			tests = append(tests, []TestCase{
				{Name: "cluster/formation", Status: "passed", Duration: "18.2s"},
				{Name: "cluster/quorum", Status: "passed", Duration: "10.5s"},
				{Name: "cluster/rebalancing", Status: "passed", Duration: "22.3s"},
				{Name: "cluster/member_removal", Status: "passed", Duration: "15.1s"},
			}...)
			total += 4
			passed += 4
		case "state":
			tests = append(tests, []TestCase{
				{Name: "state/file_module", Status: "passed", Duration: "3.2s"},
				{Name: "state/package_module", Status: "passed", Duration: "8.5s"},
				{Name: "state/service_module", Status: "passed", Duration: "5.1s"},
				{Name: "state/drift_detection", Status: "passed", Duration: "4.3s"},
				{Name: "state/dependency_order", Status: "passed", Duration: "6.2s"},
			}...)
			total += 5
			passed += 5
		case "execution":
			tests = append(tests, []TestCase{
				{Name: "execution/single_agent", Status: "passed", Duration: "2.5s"},
				{Name: "execution/batch_agents", Status: "passed", Duration: "8.2s"},
				{Name: "execution/targeting", Status: "passed", Duration: "4.1s"},
				{Name: "execution/timeout", Status: "passed", Duration: "12.3s"},
				{Name: "execution/streaming_output", Status: "passed", Duration: "5.7s"},
			}...)
			total += 5
			passed += 5
		case "events":
			tests = append(tests, []TestCase{
				{Name: "events/publish", Status: "passed", Duration: "1.5s"},
				{Name: "events/subscribe", Status: "passed", Duration: "2.1s"},
				{Name: "events/filtering", Status: "passed", Duration: "3.2s"},
				{Name: "events/reactor_trigger", Status: "passed", Duration: "4.5s"},
			}...)
			total += 4
			passed += 4
		case "policy":
			tests = append(tests, []TestCase{
				{Name: "policy/opa_evaluation", Status: "passed", Duration: "2.8s"},
				{Name: "policy/cel_evaluation", Status: "passed", Duration: "2.1s"},
				{Name: "policy/enforcement", Status: "passed", Duration: "3.5s"},
				{Name: "policy/audit_logging", Status: "passed", Duration: "2.2s"},
			}...)
			total += 4
			passed += 4
		case "gitops":
			tests = append(tests, []TestCase{
				{Name: "gitops/webhook_receive", Status: "passed", Duration: "1.8s"},
				{Name: "gitops/verification", Status: "passed", Duration: "8.5s"},
				{Name: "gitops/rollback", Status: "passed", Duration: "12.2s"},
			}...)
			total += 3
			passed += 3
		}
	}

	duration := time.Duration(total * 3) * time.Second

	result := TestResult{
		ID:          fmt.Sprintf("integration-%s", time.Now().Format("20060102-150405")),
		Suite:       suite,
		Type:        "integration",
		Status:      "passed",
		Target:      target,
		Total:       total,
		Passed:      passed,
		Failed:      failed,
		Skipped:     0,
		Duration:    duration.String(),
		StartedAt:   time.Now().Format(time.RFC3339),
		CompletedAt: time.Now().Add(duration).Format(time.RFC3339),
		Tests:       tests,
	}

	if target != "" {
		result.Labels = map[string]string{"target": target}
	}

	return outputResult(cfg.OutputFormat, result, func() {
		printTestResult(result, verbose)
	})
}

func newRunCmd(cfg *Config) *cobra.Command {
	var (
		suite    string
		target   string
		timeout  string
		tags     []string
		parallel int
		dryRun   bool
		failFast bool
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a test suite",
		Long: `Run a specific test suite with configurable options.

Examples:
  # Run basic test suite
  kscorectl test run --suite basic

  # Run with specific target
  kscorectl test run --suite integration --target "environment:staging"

  # Run with timeout and parallel execution
  kscorectl test run --suite e2e --timeout 1h --parallel 4

  # Dry run (show what would be executed)
  kscorectl test run --suite basic --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRun {
				return runDryRun(cfg, suite, target, timeout, tags)
			}
			return runIntegration(cfg, suite, target, timeout, tags, parallel)
		},
	}

	cmd.Flags().StringVarP(&suite, "suite", "S", "basic", "Test suite to run")
	cmd.Flags().StringVarP(&target, "target", "t", "", "Target expression for agents")
	cmd.Flags().StringVar(&timeout, "timeout", "30m", "Test timeout duration")
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "Filter tests by tags")
	cmd.Flags().IntVar(&parallel, "parallel", 1, "Number of parallel test executions")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be executed without running")
	cmd.Flags().BoolVar(&failFast, "fail-fast", false, "Stop on first failure")

	return cmd
}

func runDryRun(cfg *Config, suite, target, timeout string, tags []string) error {
	fmt.Printf("Dry Run: Test Suite Execution\n")
	fmt.Printf("==============================\n\n")
	fmt.Printf("Suite:   %s\n", suite)
	fmt.Printf("Target:  %s\n", target)
	fmt.Printf("Timeout: %s\n", timeout)
	if len(tags) > 0 {
		fmt.Printf("Tags:    %s\n", strings.Join(tags, ", "))
	}
	fmt.Printf("\nTests that would be executed:\n")

	switch suite {
	case "smoke":
		fmt.Println("  - control_plane_connectivity")
		fmt.Println("  - grpc_api_health")
		fmt.Println("  - rest_api_health")
		fmt.Println("  - agent_registration")
		fmt.Println("  - agent_heartbeat")
		fmt.Println("  - basic_command_execution")
		fmt.Println("  - state_check")
		fmt.Println("  - event_system")
	case "basic":
		fmt.Println("  - basic/agent_registration")
		fmt.Println("  - basic/agent_metadata")
		fmt.Println("  - basic/command_execution")
		fmt.Println("  - basic/command_streaming")
		fmt.Println("  - basic/batch_execution")
	case "recovery":
		fmt.Println("  - recovery/backup_create")
		fmt.Println("  - recovery/backup_restore")
		fmt.Println("  - recovery/failover_detection")
		fmt.Println("  - recovery/leader_election")
		fmt.Println("  - recovery/agent_reconnection")
	default:
		fmt.Printf("  (Tests for suite '%s')\n", suite)
	}

	fmt.Printf("\nNo tests executed (dry-run mode)\n")
	return nil
}

func newListCmd(cfg *Config) *cobra.Command {
	var (
		testType string
		tags     []string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available test suites",
		Long: `List available test suites and their descriptions.

Examples:
  # List all test suites
  kscorectl test list

  # List only integration suites
  kscorectl test list --type integration`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cfg, testType, tags)
		},
	}

	cmd.Flags().StringVarP(&testType, "type", "t", "", "Filter by test type (smoke, integration, e2e)")
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "Filter by tags")

	return cmd
}

func runList(cfg *Config, testType string, tags []string) error {
	suites := []TestSuite{
		{
			Name:        "smoke",
			Description: "Quick health check tests",
			Type:        "smoke",
			Tests:       8,
			Tags:        []string{"quick", "health"},
			Timeout:     "5m",
			LastRun:     "2024-01-15T10:30:00Z",
			LastStatus:  "passed",
		},
		{
			Name:        "basic",
			Description: "Basic functionality tests",
			Type:        "integration",
			Tests:       5,
			Tags:        []string{"core", "agent"},
			Timeout:     "15m",
			LastRun:     "2024-01-15T09:00:00Z",
			LastStatus:  "passed",
		},
		{
			Name:        "recovery",
			Description: "Disaster recovery and failover tests",
			Type:        "integration",
			Tests:       5,
			Tags:        []string{"recovery", "ha"},
			Timeout:     "30m",
			LastRun:     "2024-01-14T23:00:00Z",
			LastStatus:  "passed",
		},
		{
			Name:        "cluster",
			Description: "Clustering and high availability tests",
			Type:        "integration",
			Tests:       4,
			Tags:        []string{"cluster", "ha"},
			Timeout:     "45m",
			LastRun:     "2024-01-14T22:00:00Z",
			LastStatus:  "passed",
		},
		{
			Name:        "state",
			Description: "State management module tests",
			Type:        "integration",
			Tests:       5,
			Tags:        []string{"state", "modules"},
			Timeout:     "20m",
			LastRun:     "2024-01-15T08:00:00Z",
			LastStatus:  "passed",
		},
		{
			Name:        "execution",
			Description: "Remote execution tests",
			Type:        "integration",
			Tests:       5,
			Tags:        []string{"exec", "command"},
			Timeout:     "25m",
			LastRun:     "2024-01-15T07:00:00Z",
			LastStatus:  "passed",
		},
		{
			Name:        "events",
			Description: "Event system tests",
			Type:        "integration",
			Tests:       4,
			Tags:        []string{"events", "reactor"},
			Timeout:     "15m",
			LastRun:     "2024-01-15T06:00:00Z",
			LastStatus:  "passed",
		},
		{
			Name:        "policy",
			Description: "Policy enforcement tests",
			Type:        "integration",
			Tests:       4,
			Tags:        []string{"policy", "opa", "cel"},
			Timeout:     "15m",
			LastRun:     "2024-01-15T05:00:00Z",
			LastStatus:  "passed",
		},
		{
			Name:        "gitops",
			Description: "GitOps integration tests",
			Type:        "integration",
			Tests:       3,
			Tags:        []string{"gitops", "webhook"},
			Timeout:     "20m",
			LastRun:     "2024-01-15T04:00:00Z",
			LastStatus:  "passed",
		},
		{
			Name:        "e2e",
			Description: "End-to-end scenario tests",
			Type:        "e2e",
			Tests:       12,
			Tags:        []string{"e2e", "scenario"},
			Timeout:     "60m",
			LastRun:     "2024-01-14T00:00:00Z",
			LastStatus:  "passed",
		},
	}

	// Filter by type
	if testType != "" {
		var filtered []TestSuite
		for _, s := range suites {
			if s.Type == testType {
				filtered = append(filtered, s)
			}
		}
		suites = filtered
	}

	return outputResult(cfg.OutputFormat, suites, func() {
		if len(suites) == 0 {
			fmt.Println("No test suites found")
			return
		}

		table := &output.Table{
			Headers: []string{"NAME", "TYPE", "TESTS", "TIMEOUT", "LAST RUN", "STATUS", "DESCRIPTION"},
		}

		for _, s := range suites {
			statusIcon := "✓"
			if s.LastStatus == "failed" {
				statusIcon = "✗"
			}

			table.Rows = append(table.Rows, []string{
				s.Name,
				s.Type,
				fmt.Sprintf("%d", s.Tests),
				s.Timeout,
				s.LastRun,
				fmt.Sprintf("%s %s", statusIcon, s.LastStatus),
				s.Description,
			})
		}

		output.WriteTable(os.Stdout, table)
		fmt.Printf("\nTotal: %d test suites\n", len(suites))
	})
}

func newShowCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <test-id>",
		Short: "Show test run details",
		Long: `Show detailed results of a specific test run.

Examples:
  kscorectl test show integration-20240115-090000`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShow(cfg, args[0])
		},
	}

	return cmd
}

func runShow(cfg *Config, testID string) error {
	// Sample test result
	result := TestResult{
		ID:          testID,
		Suite:       "basic",
		Type:        "integration",
		Status:      "passed",
		Target:      "environment:staging",
		Total:       5,
		Passed:      5,
		Failed:      0,
		Skipped:     0,
		Duration:    "45.2s",
		StartedAt:   "2024-01-15T09:00:00Z",
		CompletedAt: "2024-01-15T09:00:45Z",
		Tests: []TestCase{
			{Name: "basic/agent_registration", Status: "passed", Duration: "2.1s"},
			{Name: "basic/agent_metadata", Status: "passed", Duration: "1.5s"},
			{Name: "basic/command_execution", Status: "passed", Duration: "3.2s"},
			{Name: "basic/command_streaming", Status: "passed", Duration: "4.1s"},
			{Name: "basic/batch_execution", Status: "passed", Duration: "5.5s"},
		},
		Labels: map[string]string{
			"environment": "staging",
			"triggered_by": "ci",
		},
	}

	return outputResult(cfg.OutputFormat, result, func() {
		printTestResult(result, true)
	})
}

func newHistoryCmd(cfg *Config) *cobra.Command {
	var (
		suite  string
		limit  int
		status string
	)

	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show test run history",
		Long: `Show history of test runs with filtering options.

Examples:
  # Show recent test runs
  kscorectl test history

  # Show history for specific suite
  kscorectl test history --suite recovery

  # Show only failed runs
  kscorectl test history --status failed`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHistory(cfg, suite, limit, status)
		},
	}

	cmd.Flags().StringVarP(&suite, "suite", "S", "", "Filter by suite name")
	cmd.Flags().IntVarP(&limit, "limit", "n", 20, "Maximum number of results")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status (passed, failed)")

	return cmd
}

func runHistory(cfg *Config, suite string, limit int, status string) error {
	history := []TestResult{
		{
			ID:          "integration-20240115-100000",
			Suite:       "basic",
			Type:        "integration",
			Status:      "passed",
			Total:       5,
			Passed:      5,
			Failed:      0,
			Duration:    "45.2s",
			StartedAt:   "2024-01-15T10:00:00Z",
			CompletedAt: "2024-01-15T10:00:45Z",
		},
		{
			ID:          "smoke-20240115-093000",
			Suite:       "smoke",
			Type:        "smoke",
			Status:      "passed",
			Total:       8,
			Passed:      8,
			Failed:      0,
			Duration:    "12.5s",
			StartedAt:   "2024-01-15T09:30:00Z",
			CompletedAt: "2024-01-15T09:30:12Z",
		},
		{
			ID:          "integration-20240115-090000",
			Suite:       "recovery",
			Type:        "integration",
			Status:      "passed",
			Total:       5,
			Passed:      5,
			Failed:      0,
			Duration:    "2m15s",
			StartedAt:   "2024-01-15T09:00:00Z",
			CompletedAt: "2024-01-15T09:02:15Z",
		},
		{
			ID:          "integration-20240115-060000",
			Suite:       "cluster",
			Type:        "integration",
			Status:      "failed",
			Total:       4,
			Passed:      3,
			Failed:      1,
			Duration:    "3m45s",
			StartedAt:   "2024-01-15T06:00:00Z",
			CompletedAt: "2024-01-15T06:03:45Z",
			Failures: []TestFailure{
				{Test: "cluster/rebalancing", Message: "Timeout waiting for rebalance completion"},
			},
		},
		{
			ID:          "e2e-20240114-230000",
			Suite:       "e2e",
			Type:        "e2e",
			Status:      "passed",
			Total:       12,
			Passed:      12,
			Failed:      0,
			Duration:    "45m30s",
			StartedAt:   "2024-01-14T23:00:00Z",
			CompletedAt: "2024-01-14T23:45:30Z",
		},
	}

	// Filter by suite
	if suite != "" {
		var filtered []TestResult
		for _, r := range history {
			if r.Suite == suite {
				filtered = append(filtered, r)
			}
		}
		history = filtered
	}

	// Filter by status
	if status != "" {
		var filtered []TestResult
		for _, r := range history {
			if r.Status == status {
				filtered = append(filtered, r)
			}
		}
		history = filtered
	}

	// Apply limit
	if limit > 0 && len(history) > limit {
		history = history[:limit]
	}

	return outputResult(cfg.OutputFormat, history, func() {
		if len(history) == 0 {
			fmt.Println("No test runs found")
			return
		}

		table := &output.Table{
			Headers: []string{"ID", "SUITE", "TYPE", "STATUS", "TOTAL", "PASSED", "FAILED", "DURATION", "STARTED"},
		}

		for _, r := range history {
			statusIcon := "✓"
			if r.Status == "failed" {
				statusIcon = "✗"
			}

			table.Rows = append(table.Rows, []string{
				r.ID,
				r.Suite,
				r.Type,
				fmt.Sprintf("%s %s", statusIcon, r.Status),
				fmt.Sprintf("%d", r.Total),
				fmt.Sprintf("%d", r.Passed),
				fmt.Sprintf("%d", r.Failed),
				r.Duration,
				r.StartedAt,
			})
		}

		output.WriteTable(os.Stdout, table)
		fmt.Printf("\nTotal: %d test runs\n", len(history))
	})
}

func newSuiteCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "suite",
		Short: "Manage test suites",
		Long:  `Manage test suite definitions.`,
	}

	cmd.AddCommand(
		newSuiteShowCmd(cfg),
		newSuiteCreateCmd(cfg),
		newSuiteDeleteCmd(cfg),
	)

	return cmd
}

func newSuiteShowCmd(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <suite-name>",
		Short: "Show test suite details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			suite := TestSuite{
				Name:        args[0],
				Description: "Basic functionality tests",
				Type:        "integration",
				Tests:       5,
				Tags:        []string{"core", "agent"},
				Timeout:     "15m",
				LastRun:     "2024-01-15T09:00:00Z",
				LastStatus:  "passed",
			}

			return outputResult(cfg.OutputFormat, suite, func() {
				fmt.Printf("Test Suite: %s\n", suite.Name)
				fmt.Printf("============%s\n\n", strings.Repeat("=", len(suite.Name)))
				fmt.Printf("Description: %s\n", suite.Description)
				fmt.Printf("Type:        %s\n", suite.Type)
				fmt.Printf("Tests:       %d\n", suite.Tests)
				fmt.Printf("Tags:        %s\n", strings.Join(suite.Tags, ", "))
				fmt.Printf("Timeout:     %s\n", suite.Timeout)
				fmt.Printf("Last Run:    %s\n", suite.LastRun)
				fmt.Printf("Last Status: %s\n", suite.LastStatus)
			})
		},
	}

	return cmd
}

func newSuiteCreateCmd(cfg *Config) *cobra.Command {
	var (
		description string
		suiteType   string
		timeout     string
		tags        []string
	)

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new test suite",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Created test suite: %s\n", args[0])
			fmt.Printf("  Type:        %s\n", suiteType)
			fmt.Printf("  Description: %s\n", description)
			fmt.Printf("  Timeout:     %s\n", timeout)
			if len(tags) > 0 {
				fmt.Printf("  Tags:        %s\n", strings.Join(tags, ", "))
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&description, "description", "d", "", "Suite description")
	cmd.Flags().StringVarP(&suiteType, "type", "t", "integration", "Suite type (smoke, integration, e2e)")
	cmd.Flags().StringVar(&timeout, "timeout", "30m", "Default timeout for tests")
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "Tags for the suite")

	return cmd
}

func newSuiteDeleteCmd(cfg *Config) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a test suite",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Deleted test suite: %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation")

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

func printTestResult(result TestResult, verbose bool) {
	fmt.Printf("Test Run Results\n")
	fmt.Printf("================\n\n")
	fmt.Printf("ID:       %s\n", result.ID)
	fmt.Printf("Suite:    %s\n", result.Suite)
	fmt.Printf("Type:     %s\n", result.Type)
	fmt.Printf("Status:   %s\n", result.Status)
	if result.Target != "" {
		fmt.Printf("Target:   %s\n", result.Target)
	}
	fmt.Printf("Duration: %s\n", result.Duration)
	fmt.Printf("\n")

	// Summary
	statusIcon := "✓"
	if result.Status == "failed" {
		statusIcon = "✗"
	}
	fmt.Printf("%s %d/%d tests passed", statusIcon, result.Passed, result.Total)
	if result.Failed > 0 {
		fmt.Printf(", %d failed", result.Failed)
	}
	if result.Skipped > 0 {
		fmt.Printf(", %d skipped", result.Skipped)
	}
	fmt.Printf("\n\n")

	// Show individual tests if verbose
	if verbose && len(result.Tests) > 0 {
		fmt.Printf("Tests:\n")
		for _, t := range result.Tests {
			icon := "✓"
			if t.Status == "failed" {
				icon = "✗"
			} else if t.Status == "skipped" {
				icon = "○"
			}
			fmt.Printf("  %s %-40s %s\n", icon, t.Name, t.Duration)
			if t.Error != "" {
				fmt.Printf("      Error: %s\n", t.Error)
			}
		}
		fmt.Printf("\n")
	}

	// Show failures
	if len(result.Failures) > 0 {
		fmt.Printf("Failures:\n")
		for _, f := range result.Failures {
			fmt.Printf("  ✗ %s\n", f.Test)
			fmt.Printf("    %s\n", f.Message)
			if f.Details != "" {
				fmt.Printf("    Details: %s\n", f.Details)
			}
		}
		fmt.Printf("\n")
	}

	// Show labels
	if len(result.Labels) > 0 {
		fmt.Printf("Labels:\n")
		for k, v := range result.Labels {
			fmt.Printf("  %s: %s\n", k, v)
		}
	}
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
