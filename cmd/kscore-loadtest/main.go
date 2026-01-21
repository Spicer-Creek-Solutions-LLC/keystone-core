package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/shawnbutts/keystone-core/internal/loadtest"
	"github.com/shawnbutts/keystone-core/pkg/cli/auditutil"
	"github.com/shawnbutts/keystone-core/pkg/cli/output"
	"github.com/shawnbutts/keystone-core/pkg/version"
)

var (
	outputFormat string
	verbose      bool
	auditLevel   string
	auditOutput  string
)

func main() {
	rootCmd := newRootCmd()
	auditHandler := auditutil.Attach(rootCmd, "kscore-loadtest", &auditLevel, &auditOutput)
	if err := rootCmd.Execute(); err != nil {
		auditHandler.LogFailure(err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-loadtest",
		Short: "Keystone Core load testing tool",
		Long: `kscore-loadtest is a CLI tool for running load tests against Keystone Core.

This tool provides commands for:
  - Running load test scenarios with configurable agent counts
  - Testing registration, heartbeat, and command execution performance
  - Generating performance reports

Usage via kscorectl:
  kscorectl loadtest run --agents 100 --scenario registration
  kscorectl loadtest run --agents 50 --scenario commands --duration 5m
  kscorectl loadtest report --file results.json`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, json, yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend")

	rootCmd.AddCommand(
		newRunCmd(),
		newScenariosCmd(),
		newReportCmd(),
		newVersionCmd(),
	)

	return rootCmd
}

func newRunCmd() *cobra.Command {
	var (
		agentCount         int
		scenario           string
		duration           string
		rampUp             string
		heartbeatInterval  string
		commandsPerAgent   int
		concurrentCommands int
		reportDir          string
		natsPort           int
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a load test scenario",
		Long: `Run a load test scenario with configurable parameters.

Available scenarios:
  - registration: Test agent registration performance
  - heartbeat: Test sustained heartbeat throughput
  - commands: Test command execution performance
  - rampup: Test gradual agent ramp-up
  - sustained: Test sustained load with commands and heartbeats

Examples:
  # Run registration test with 100 agents
  kscorectl loadtest run --agents 100 --scenario registration

  # Run command test with 50 agents, 10 commands each
  kscorectl loadtest run --agents 50 --scenario commands --commands-per-agent 10

  # Run sustained load test for 5 minutes
  kscorectl loadtest run --agents 100 --scenario sustained --duration 5m

  # Run with custom ramp-up time
  kscorectl loadtest run --agents 200 --scenario heartbeat --ramp-up 30s --duration 2m`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadtest.DefaultConfig()
			cfg.AgentCount = agentCount
			cfg.NATSPort = natsPort

			if d, err := time.ParseDuration(duration); err == nil {
				cfg.TestDuration = d
			}
			if d, err := time.ParseDuration(rampUp); err == nil {
				cfg.RampUpDuration = d
			}
			if d, err := time.ParseDuration(heartbeatInterval); err == nil {
				cfg.HeartbeatInterval = d
			}
			if commandsPerAgent > 0 {
				cfg.CommandsPerAgent = commandsPerAgent
			}
			if concurrentCommands > 0 {
				cfg.ConcurrentCommands = concurrentCommands
			}
			if reportDir != "" {
				cfg.ReportDir = reportDir
			}

			return runScenario(cfg, scenario)
		},
	}

	cmd.Flags().IntVarP(&agentCount, "agents", "a", 10, "Number of simulated agents")
	cmd.Flags().StringVarP(&scenario, "scenario", "s", "registration", "Scenario to run (registration, heartbeat, commands, rampup, sustained)")
	cmd.Flags().StringVarP(&duration, "duration", "d", "60s", "Test duration")
	cmd.Flags().StringVar(&rampUp, "ramp-up", "10s", "Ramp-up duration for gradual agent start")
	cmd.Flags().StringVar(&heartbeatInterval, "heartbeat-interval", "5s", "Heartbeat interval")
	cmd.Flags().IntVar(&commandsPerAgent, "commands-per-agent", 10, "Commands per agent for command tests")
	cmd.Flags().IntVar(&concurrentCommands, "concurrent-commands", 50, "Max concurrent commands")
	cmd.Flags().StringVar(&reportDir, "report-dir", "reports/loadtest", "Directory for saving reports")
	cmd.Flags().IntVar(&natsPort, "nats-port", 14222, "Port for embedded NATS server")

	return cmd
}

func runScenario(cfg *loadtest.Config, scenario string) error {
	if verbose {
		fmt.Printf("Starting load test scenario: %s\n", scenario)
		fmt.Printf("  Agents: %d\n", cfg.AgentCount)
		fmt.Printf("  Duration: %v\n", cfg.TestDuration)
		fmt.Printf("  Ramp-up: %v\n", cfg.RampUpDuration)
		fmt.Printf("  Heartbeat Interval: %v\n", cfg.HeartbeatInterval)
		fmt.Println()
	}

	harness := loadtest.NewTestHarness(cfg)
	if err := harness.Start(); err != nil {
		return fmt.Errorf("failed to start test harness: %w", err)
	}
	defer harness.Stop()

	result := &loadtest.LoadTestResult{
		TestName:  scenario,
		StartTime: time.Now(),
		Config: loadtest.ResultConfig{
			AgentCount:         cfg.AgentCount,
			HeartbeatInterval:  cfg.HeartbeatInterval,
			TestDuration:       cfg.TestDuration,
			CommandsPerAgent:   cfg.CommandsPerAgent,
			ConcurrentCommands: cfg.ConcurrentCommands,
		},
	}

	pool, err := harness.CreateAgentPool()
	if err != nil {
		return fmt.Errorf("failed to create agent pool: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.TestDuration+5*time.Minute)
	defer cancel()

	switch scenario {
	case "registration":
		err = runRegistration(ctx, cfg, pool, result)
	case "heartbeat":
		err = runHeartbeat(ctx, cfg, pool, harness, result)
	case "commands":
		err = runCommands(ctx, cfg, pool, harness, result)
	case "rampup":
		err = runRampUp(ctx, cfg, pool, harness, result)
	case "sustained":
		err = runSustained(ctx, cfg, pool, harness, result)
	default:
		return fmt.Errorf("unknown scenario: %s", scenario)
	}

	pool.StopAll()

	if err != nil {
		result.Errors = append(result.Errors, err.Error())
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	if cfg.ReportDir != "" {
		if saveErr := loadtest.SaveResult(result, cfg.ReportDir); saveErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save result: %v\n", saveErr)
		} else if verbose {
			fmt.Printf("Report saved to: %s/%s_%d.json\n", cfg.ReportDir, result.TestName, result.StartTime.Unix())
		}
	}

	return outputResult(outputFormat, result, func() {
		printResult(result)
	})
}

func runRegistration(ctx context.Context, cfg *loadtest.Config, pool *loadtest.AgentPool, result *loadtest.LoadTestResult) error {
	if verbose {
		fmt.Println("Running registration test...")
	}

	regStart := time.Now()
	if err := pool.StartAll(ctx); err != nil {
		return err
	}
	regDuration := time.Since(regStart)

	metrics := pool.AggregateMetrics()
	metrics.RegistrationTime = regDuration
	metrics.AgentsRegistered = pool.AgentCount()

	result.Metrics = metrics
	result.Success = metrics.AgentsRegistered == cfg.AgentCount

	return nil
}

func runHeartbeat(ctx context.Context, cfg *loadtest.Config, pool *loadtest.AgentPool, harness *loadtest.TestHarness, result *loadtest.LoadTestResult) error {
	if verbose {
		fmt.Println("Running heartbeat test...")
	}

	if err := pool.StartAll(ctx); err != nil {
		return err
	}

	if verbose {
		fmt.Printf("Running for %v...\n", cfg.TestDuration)
	}
	time.Sleep(cfg.TestDuration)

	metrics := pool.AggregateMetrics()
	cpMetrics := harness.ControlPlane().Metrics()

	metrics.HeartbeatsSent = cpMetrics.Heartbeats
	metrics.HeartbeatsReceived = cpMetrics.Heartbeats
	metrics.AgentsRegistered = pool.AgentCount()

	result.Metrics = metrics
	result.Success = metrics.HeartbeatsSent > 0

	return nil
}

func runCommands(ctx context.Context, cfg *loadtest.Config, pool *loadtest.AgentPool, harness *loadtest.TestHarness, result *loadtest.LoadTestResult) error {
	if verbose {
		fmt.Println("Running command test...")
	}

	if err := pool.StartAll(ctx); err != nil {
		return err
	}

	time.Sleep(500 * time.Millisecond)

	agentIDs := harness.ControlPlane().RegisteredAgentIDs()
	if len(agentIDs) == 0 {
		return fmt.Errorf("no agents registered")
	}

	var commands []string
	for i := 0; i < cfg.CommandsPerAgent; i++ {
		commands = append(commands, agentIDs...)
	}

	if verbose {
		fmt.Printf("Sending %d commands with concurrency %d...\n", len(commands), cfg.ConcurrentCommands)
	}

	cmdStart := time.Now()
	success, failed, latencyCollector := harness.ControlPlane().BroadcastCommand(
		ctx, commands, "echo", []string{"test"}, cfg.CommandTimeout, cfg.ConcurrentCommands)
	cmdDuration := time.Since(cmdStart)

	min, max, avg, p50, p95, p99 := latencyCollector.Calculate()

	metrics := loadtest.Metrics{
		TotalOps:          int64(success + failed),
		SuccessfulOps:     int64(success),
		FailedOps:         int64(failed),
		OpsPerSecond:      float64(success+failed) / cmdDuration.Seconds(),
		AvgLatency:        avg,
		MinLatency:        min,
		MaxLatency:        max,
		P50Latency:        p50,
		P95Latency:        p95,
		P99Latency:        p99,
		CommandsSent:      int64(success + failed),
		CommandsCompleted: int64(success),
		CommandsFailed:    int64(failed),
		AgentsRegistered:  len(agentIDs),
	}

	if metrics.TotalOps > 0 {
		metrics.ErrorRate = float64(failed) / float64(success+failed) * 100
	}

	result.Metrics = metrics
	result.Success = failed == 0 && success > 0

	return nil
}

func runRampUp(ctx context.Context, cfg *loadtest.Config, pool *loadtest.AgentPool, harness *loadtest.TestHarness, result *loadtest.LoadTestResult) error {
	if verbose {
		fmt.Println("Running ramp-up test...")
	}

	rampStart := time.Now()
	if err := pool.StartAll(ctx); err != nil {
		return err
	}
	rampDuration := time.Since(rampStart)

	metrics := pool.AggregateMetrics()
	metrics.AgentsRegistered = pool.AgentCount()
	metrics.RegistrationTime = rampDuration

	result.Metrics = metrics
	result.Success = metrics.AgentsRegistered == cfg.AgentCount

	if verbose {
		fmt.Printf("Ramp-up completed: %d agents in %v (%.2f agents/sec)\n",
			metrics.AgentsRegistered, rampDuration,
			float64(metrics.AgentsRegistered)/rampDuration.Seconds())
	}

	return nil
}

func runSustained(ctx context.Context, cfg *loadtest.Config, pool *loadtest.AgentPool, harness *loadtest.TestHarness, result *loadtest.LoadTestResult) error {
	if verbose {
		fmt.Println("Running sustained load test...")
	}

	if err := pool.StartAll(ctx); err != nil {
		return err
	}

	time.Sleep(500 * time.Millisecond)

	agentIDs := harness.ControlPlane().RegisteredAgentIDs()
	if len(agentIDs) == 0 {
		return fmt.Errorf("no agents registered")
	}

	cmdInterval := cfg.TestDuration / time.Duration(cfg.CommandsPerAgent)
	if cmdInterval < 100*time.Millisecond {
		cmdInterval = 100 * time.Millisecond
	}

	if verbose {
		fmt.Printf("Running sustained load for %v...\n", cfg.TestDuration)
	}

	testStart := time.Now()
	var totalSuccess, totalFailed int
	latencyCollector := loadtest.NewLatencyCollector()

	ticker := time.NewTicker(cmdInterval)
	defer ticker.Stop()

	endTime := time.Now().Add(cfg.TestDuration)
	for time.Now().Before(endTime) {
		select {
		case <-ticker.C:
			success, failed, lc := harness.ControlPlane().BroadcastCommand(
				ctx, agentIDs, "echo", []string{"test"}, cfg.CommandTimeout, cfg.ConcurrentCommands)
			totalSuccess += success
			totalFailed += failed
			latencyCollector.AddBatch(lc.Latencies())
		case <-ctx.Done():
			break
		}
	}
	testDuration := time.Since(testStart)

	cpMetrics := harness.ControlPlane().Metrics()
	min, max, avg, p50, p95, p99 := latencyCollector.Calculate()

	metrics := loadtest.Metrics{
		TotalOps:           int64(totalSuccess + totalFailed),
		SuccessfulOps:      int64(totalSuccess),
		FailedOps:          int64(totalFailed),
		OpsPerSecond:       float64(totalSuccess+totalFailed) / testDuration.Seconds(),
		AvgLatency:         avg,
		MinLatency:         min,
		MaxLatency:         max,
		P50Latency:         p50,
		P95Latency:         p95,
		P99Latency:         p99,
		HeartbeatsSent:     cpMetrics.Heartbeats,
		HeartbeatsReceived: cpMetrics.Heartbeats,
		CommandsSent:       int64(totalSuccess + totalFailed),
		CommandsCompleted:  int64(totalSuccess),
		CommandsFailed:     int64(totalFailed),
		AgentsRegistered:   len(agentIDs),
	}

	if metrics.TotalOps > 0 {
		metrics.ErrorRate = float64(totalFailed) / float64(totalSuccess+totalFailed) * 100
	}

	result.Metrics = metrics
	result.Success = totalFailed == 0 && totalSuccess > 0

	return nil
}

func newScenariosCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scenarios",
		Short: "List available load test scenarios",
		Run: func(cmd *cobra.Command, args []string) {
			scenarios := []struct {
				Name        string
				Description string
			}{
				{"registration", "Test agent registration performance"},
				{"heartbeat", "Test sustained heartbeat throughput"},
				{"commands", "Test command execution performance"},
				{"rampup", "Test gradual agent ramp-up"},
				{"sustained", "Test sustained load with commands and heartbeats"},
			}

			table := &output.Table{
				Headers: []string{"SCENARIO", "DESCRIPTION"},
			}

			for _, s := range scenarios {
				table.Rows = append(table.Rows, []string{s.Name, s.Description})
			}

			output.WriteTable(os.Stdout, table)
		},
	}
}

func newReportCmd() *cobra.Command {
	var file string

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Display a load test report",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := loadtest.LoadResult(file)
			if err != nil {
				return fmt.Errorf("failed to load report: %w", err)
			}

			return outputResult(outputFormat, result, func() {
				printResult(result)
			})
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "Report file to display")
	cmd.MarkFlagRequired("file")

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

func printResult(result *loadtest.LoadTestResult) {
	fmt.Printf("Load Test Results\n")
	fmt.Printf("=================\n\n")
	fmt.Printf("Scenario:  %s\n", result.TestName)
	fmt.Printf("Duration:  %v\n", result.Duration)
	fmt.Printf("Success:   %v\n", result.Success)
	fmt.Printf("\n")

	fmt.Printf("Configuration:\n")
	fmt.Printf("  Agents:           %d\n", result.Config.AgentCount)
	fmt.Printf("  Heartbeat:        %v\n", result.Config.HeartbeatInterval)
	if result.Config.TestDuration > 0 {
		fmt.Printf("  Test Duration:    %v\n", result.Config.TestDuration)
	}
	if result.Config.CommandsPerAgent > 0 {
		fmt.Printf("  Commands/Agent:   %d\n", result.Config.CommandsPerAgent)
	}
	fmt.Printf("\n")

	fmt.Printf("Metrics:\n")
	fmt.Printf("  Agents Registered: %d\n", result.Metrics.AgentsRegistered)
	if result.Metrics.RegistrationTime > 0 {
		fmt.Printf("  Registration Time: %v\n", result.Metrics.RegistrationTime)
	}
	if result.Metrics.HeartbeatsSent > 0 {
		fmt.Printf("  Heartbeats Sent:   %d\n", result.Metrics.HeartbeatsSent)
	}
	if result.Metrics.TotalOps > 0 {
		fmt.Printf("  Total Operations:  %d\n", result.Metrics.TotalOps)
		fmt.Printf("  Successful:        %d\n", result.Metrics.SuccessfulOps)
		fmt.Printf("  Failed:            %d\n", result.Metrics.FailedOps)
		fmt.Printf("  Throughput:        %.2f ops/sec\n", result.Metrics.OpsPerSecond)
		fmt.Printf("  Error Rate:        %.2f%%\n", result.Metrics.ErrorRate)
	}
	fmt.Printf("\n")

	if result.Metrics.AvgLatency > 0 {
		fmt.Printf("Latency:\n")
		fmt.Printf("  Min:  %v\n", result.Metrics.MinLatency)
		fmt.Printf("  Avg:  %v\n", result.Metrics.AvgLatency)
		fmt.Printf("  Max:  %v\n", result.Metrics.MaxLatency)
		fmt.Printf("  P50:  %v\n", result.Metrics.P50Latency)
		fmt.Printf("  P95:  %v\n", result.Metrics.P95Latency)
		fmt.Printf("  P99:  %v\n", result.Metrics.P99Latency)
		fmt.Printf("\n")
	}

	if len(result.Errors) > 0 {
		fmt.Printf("Errors:\n")
		for _, e := range result.Errors {
			fmt.Printf("  - %s\n", e)
		}
		fmt.Printf("\n")
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
