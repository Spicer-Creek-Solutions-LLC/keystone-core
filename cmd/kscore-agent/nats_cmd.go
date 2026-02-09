package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/config"
	natsmgr "github.com/shawnbutts/keystone-core/internal/nats"
)

func newNATSCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "nats",
		Short: "NATS connection diagnostics",
		Long:  `Commands for diagnosing and testing the agent's NATS connection.`,
	}

	cmd.AddCommand(newNATSPingCmd())
	cmd.AddCommand(newNATSStatusCmd())
	cmd.AddCommand(newNATSTestCmd())

	return cmd
}

func newNATSPingCmd() *cobra.Command {
	var (
		count   int
		timeout string
	)

	cmd := &cobra.Command{
		Use:   "ping",
		Short: "Ping the NATS server and measure round-trip time",
		Long: `Connects to the NATS server and measures round-trip latency
by sending ping requests. Reports individual RTT values and a summary.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNATSPing(cmd, count, timeout)
		},
		SilenceUsage: true,
	}

	cmd.Flags().IntVar(&count, "count", 3, "Number of pings to send")
	cmd.Flags().StringVar(&timeout, "timeout", "5s", "Connection timeout")

	return cmd
}

func newNATSStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show NATS connection status",
		Long: `Connects to the NATS server and reports connection information
including state, server URL, mode, JetStream status, and traffic statistics.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNATSStatus(cmd)
		},
		SilenceUsage: true,
	}

	return cmd
}

func newNATSTestCmd() *cobra.Command {
	var verbose bool

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run NATS connectivity test suite",
		Long: `Runs a comprehensive connectivity test suite against the NATS server:
  1. Connection test - verifies ability to connect
  2. Publish/subscribe test - verifies message delivery
  3. JetStream test - verifies JetStream functionality (if enabled)
  4. Latency test - measures round-trip time`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNATSTest(cmd, verbose)
		},
		SilenceUsage: true,
	}

	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show detailed output for each test")

	return cmd
}

func connectNATSManager() (*natsmgr.Manager, error) {
	cfg, err := config.LoadConfig(cfgFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	manager, err := natsmgr.NewManager(&cfg.NATS)
	if err != nil {
		return nil, fmt.Errorf("failed to create NATS manager: %w", err)
	}

	if err := manager.Start(); err != nil {
		return nil, fmt.Errorf("failed to start NATS manager: %w", err)
	}

	return manager, nil
}

func natsModeName(manager *natsmgr.Manager, cfg *config.NATSConfig) string {
	switch cfg.Mode {
	case config.NATSModeEmbedded:
		return "embedded"
	case config.NATSModeExternal:
		return "external"
	case config.NATSModeLeaf:
		return "leaf"
	default:
		if manager.IsEmbedded() {
			return "embedded"
		}
		return "external"
	}
}

func runNATSPing(cmd *cobra.Command, count int, timeout string) error {
	timeoutDur, err := time.ParseDuration(timeout)
	if err != nil {
		return fmt.Errorf("invalid timeout %q: %w", timeout, err)
	}

	_ = timeoutDur

	manager, err := connectNATSManager()
	if err != nil {
		return err
	}
	defer manager.Shutdown()

	conn := manager.Conn()
	if conn == nil {
		return fmt.Errorf("NATS connection is not available")
	}

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "PING %s\n", conn.ConnectedUrl())

	var minRTT, maxRTT, totalRTT time.Duration
	successful := 0

	for i := 0; i < count; i++ {
		rtt, err := conn.RTT()
		if err != nil {
			fmt.Fprintf(w, "ping %d: error - %v\n", i+1, err)
			continue
		}

		successful++
		totalRTT += rtt

		if successful == 1 || rtt < minRTT {
			minRTT = rtt
		}
		if rtt > maxRTT {
			maxRTT = rtt
		}

		fmt.Fprintf(w, "ping %d: rtt=%s\n", i+1, rtt)
	}

	fmt.Fprintf(w, "\n--- ping statistics ---\n")
	fmt.Fprintf(w, "%d pings sent, %d successful, %d failed\n", count, successful, count-successful)
	if successful > 0 {
		avgRTT := totalRTT / time.Duration(successful)
		fmt.Fprintf(w, "rtt min/avg/max = %s/%s/%s\n", minRTT, avgRTT, maxRTT)
	}

	return nil
}

func runNATSStatus(cmd *cobra.Command) error {
	cfg, err := config.LoadConfig(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	manager, err := natsmgr.NewManager(&cfg.NATS)
	if err != nil {
		return fmt.Errorf("failed to create NATS manager: %w", err)
	}

	if err := manager.Start(); err != nil {
		return fmt.Errorf("failed to start NATS manager: %w", err)
	}
	defer manager.Shutdown()

	conn := manager.Conn()
	if conn == nil {
		return fmt.Errorf("NATS connection is not available")
	}

	w := cmd.OutOrStdout()
	stats := conn.Stats()
	mode := natsModeName(manager, &cfg.NATS)

	fmt.Fprintf(w, "NATS Connection Status\n")
	fmt.Fprintf(w, "  State:          %s\n", conn.Status().String())
	fmt.Fprintf(w, "  Server URL:     %s\n", conn.ConnectedUrl())
	fmt.Fprintf(w, "  Mode:           %s\n", mode)
	fmt.Fprintf(w, "  JetStream:      %v\n", cfg.NATS.JetStream.Enabled)
	fmt.Fprintf(w, "  Subscriptions:  %d\n", conn.NumSubscriptions())
	fmt.Fprintf(w, "  Bytes In:       %d\n", stats.InBytes)
	fmt.Fprintf(w, "  Bytes Out:      %d\n", stats.OutBytes)
	fmt.Fprintf(w, "  Msgs In:        %d\n", stats.InMsgs)
	fmt.Fprintf(w, "  Msgs Out:       %d\n", stats.OutMsgs)
	fmt.Fprintf(w, "  Reconnects:     %d\n", stats.Reconnects)

	return nil
}

func runNATSTest(cmd *cobra.Command, verbose bool) error {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "NATS Connectivity Test Suite\n")
	fmt.Fprintf(w, "============================\n\n")

	cfg, err := config.LoadConfig(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	allPassed := true

	// Test 1: Connection
	fmt.Fprintf(w, "1. Connection Test\n")
	start := time.Now()
	manager, err := natsmgr.NewManager(&cfg.NATS)
	if err != nil {
		fmt.Fprintf(w, "   FAIL: %v (%s)\n\n", err, time.Since(start))
		return nil
	}

	if err := manager.Start(); err != nil {
		fmt.Fprintf(w, "   FAIL: %v (%s)\n\n", err, time.Since(start))
		return nil
	}
	defer manager.Shutdown()

	conn := manager.Conn()
	if conn == nil {
		fmt.Fprintf(w, "   FAIL: connection is nil (%s)\n\n", time.Since(start))
		return nil
	}
	elapsed := time.Since(start)
	fmt.Fprintf(w, "   PASS (%s)\n", elapsed)
	if verbose {
		fmt.Fprintf(w, "   Server: %s\n", conn.ConnectedUrl())
	}
	fmt.Fprintln(w)

	// Test 2: Publish/Subscribe
	fmt.Fprintf(w, "2. Publish/Subscribe Test\n")
	start = time.Now()
	passed, detail := testPubSub(conn, verbose)
	elapsed = time.Since(start)
	if passed {
		fmt.Fprintf(w, "   PASS (%s)\n", elapsed)
	} else {
		fmt.Fprintf(w, "   FAIL: %s (%s)\n", detail, elapsed)
		allPassed = false
	}
	if verbose && detail != "" && passed {
		fmt.Fprintf(w, "   %s\n", detail)
	}
	fmt.Fprintln(w)

	// Test 3: JetStream
	fmt.Fprintf(w, "3. JetStream Test\n")
	if cfg.NATS.JetStream.Enabled {
		start = time.Now()
		passed, detail = testJetStream(conn, verbose)
		elapsed = time.Since(start)
		if passed {
			fmt.Fprintf(w, "   PASS (%s)\n", elapsed)
		} else {
			fmt.Fprintf(w, "   FAIL: %s (%s)\n", detail, elapsed)
			allPassed = false
		}
		if verbose && detail != "" && passed {
			fmt.Fprintf(w, "   %s\n", detail)
		}
	} else {
		fmt.Fprintf(w, "   SKIP (JetStream not enabled)\n")
	}
	fmt.Fprintln(w)

	// Test 4: Latency
	fmt.Fprintf(w, "4. Latency Test\n")
	start = time.Now()
	passed, detail = testLatency(conn, verbose)
	elapsed = time.Since(start)
	if passed {
		fmt.Fprintf(w, "   PASS (%s)\n", elapsed)
	} else {
		fmt.Fprintf(w, "   FAIL: %s (%s)\n", detail, elapsed)
		allPassed = false
	}
	if verbose && detail != "" && passed {
		fmt.Fprintf(w, "   %s\n", detail)
	}
	fmt.Fprintln(w)

	// Summary
	if allPassed {
		fmt.Fprintf(w, "All tests passed.\n")
	} else {
		fmt.Fprintf(w, "Some tests failed.\n")
	}

	return nil
}

func testPubSub(conn *nats.Conn, verbose bool) (bool, string) {
	subject := "_kscore.diag.test." + nats.NewInbox()
	received := make(chan []byte, 1)

	sub, err := conn.Subscribe(subject, func(msg *nats.Msg) {
		received <- msg.Data
	})
	if err != nil {
		return false, fmt.Sprintf("subscribe error: %v", err)
	}
	defer sub.Unsubscribe()

	if err := conn.Flush(); err != nil {
		return false, fmt.Sprintf("flush error: %v", err)
	}

	payload := []byte("keystone-diag-test")
	if err := conn.Publish(subject, payload); err != nil {
		return false, fmt.Sprintf("publish error: %v", err)
	}

	if err := conn.Flush(); err != nil {
		return false, fmt.Sprintf("flush error: %v", err)
	}

	select {
	case msg := <-received:
		if string(msg) != string(payload) {
			return false, fmt.Sprintf("payload mismatch: got %q, want %q", msg, payload)
		}
		if verbose {
			return true, fmt.Sprintf("Published and received %d bytes on %s", len(payload), subject)
		}
		return true, ""
	case <-time.After(5 * time.Second):
		return false, "timed out waiting for message"
	}
}

func testJetStream(conn *nats.Conn, verbose bool) (bool, string) {
	js, err := conn.JetStream()
	if err != nil {
		return false, fmt.Sprintf("JetStream context error: %v", err)
	}

	streamName := "_KSCORE_DIAG_TEST"
	subject := "_kscore.diag.js.test"

	// Create temporary stream
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     streamName,
		Subjects: []string{subject},
		MaxMsgs:  1,
		Storage:  nats.MemoryStorage,
	})
	if err != nil {
		return false, fmt.Sprintf("create stream error: %v", err)
	}

	// Clean up stream on exit
	defer js.DeleteStream(streamName)

	// Publish
	_, err = js.Publish(subject, []byte("jetstream-diag-test"))
	if err != nil {
		return false, fmt.Sprintf("JetStream publish error: %v", err)
	}

	// Subscribe and consume
	sub, err := js.SubscribeSync(subject, nats.Durable("_KSCORE_DIAG"))
	if err != nil {
		return false, fmt.Sprintf("JetStream subscribe error: %v", err)
	}
	defer sub.Unsubscribe()

	msg, err := sub.NextMsg(5 * time.Second)
	if err != nil {
		return false, fmt.Sprintf("JetStream consume error: %v", err)
	}

	if string(msg.Data) != "jetstream-diag-test" {
		return false, fmt.Sprintf("JetStream payload mismatch: got %q", msg.Data)
	}

	if verbose {
		return true, fmt.Sprintf("Created stream %s, published, consumed, and deleted", streamName)
	}
	return true, ""
}

func testLatency(conn *nats.Conn, verbose bool) (bool, string) {
	const pingCount = 5
	var (
		mu                             sync.Mutex
		minRTT, maxRTT, totalRTT       time.Duration
		successful                     int
	)

	for i := 0; i < pingCount; i++ {
		rtt, err := conn.RTT()
		if err != nil {
			continue
		}
		mu.Lock()
		successful++
		totalRTT += rtt
		if successful == 1 || rtt < minRTT {
			minRTT = rtt
		}
		if rtt > maxRTT {
			maxRTT = rtt
		}
		mu.Unlock()
	}

	if successful == 0 {
		return false, "all pings failed"
	}

	avgRTT := totalRTT / time.Duration(successful)

	if verbose {
		return true, fmt.Sprintf("%d/%d pings succeeded, rtt min/avg/max = %s/%s/%s",
			successful, pingCount, minRTT, avgRTT, maxRTT)
	}
	return true, ""
}
