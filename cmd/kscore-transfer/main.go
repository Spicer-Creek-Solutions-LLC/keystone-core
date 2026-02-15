// Package main implements the kscore-transfer CLI for air-gapped data transfer operations.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/shawnbutts/keystone-core/internal/airgap/diode"
	"github.com/shawnbutts/keystone-core/internal/airgap/transfer"
	"github.com/shawnbutts/keystone-core/internal/cli/auditutil"
	"github.com/shawnbutts/keystone-core/internal/events"
	"github.com/shawnbutts/keystone-core/internal/statemgmt/history"
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
	auditHandler := auditutil.Attach(rootCmd, "kscore-transfer", &auditLevel, &auditOutput)
	if err := rootCmd.Execute(); err != nil {
		auditHandler.LogFailure(err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "kscore-transfer",
		Short: "Keystone Core air-gapped data transfer plugin",
		Long: `kscore-transfer exports and imports operational data across air-gapped boundaries.

Export packages are signed, optionally encrypted archives containing audit logs,
events, state history, and other data suitable for transfer via USB, sneakernet,
or data diode.

Usage via kscorectl:
  kscorectl transfer export --type audit --since 24h
  kscorectl transfer import package.tar.gz --output-dir ./data
  kscorectl transfer verify package.tar.gz --verify-key release.pub`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, json, yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().StringVar(&auditLevel, "audit-level", "all", "Audit logging level (all, errors, none)")
	rootCmd.PersistentFlags().StringVar(&auditOutput, "audit-output", "auto", "Audit output backend (auto, syslog, journald, stderr, none)")

	rootCmd.AddCommand(
		newExportCmd(),
		newImportCmd(),
		newVerifyCmd(),
		newSyncCmd(),
		newDiodeCmd(),
		newVersionCmd(),
	)

	return rootCmd
}

func newExportCmd() *cobra.Command {
	var (
		exportType string
		since      string
		until      string
		output     string
		signKey    string
		encryptTo  string
		createdBy  string
		eventsDB   string
		stateDB    string
	)

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export operational data for air-gapped transfer",
		Long: `Export audit logs, events, and state history as a signed, optionally encrypted archive.

The exported package can be transferred via USB, sneakernet, or data diode to
another system for analysis, SIEM ingestion, or disaster recovery.

Examples:
  # Export audit logs from the last 24 hours
  kscorectl transfer export --type audit --since 24h -o audit-export.tar.gz

  # Export everything, signed and encrypted
  kscorectl transfer export --type full --sign-key key.pem \
    --encrypt-to age1recipient -o full-export.tar.gz

  # Export events with a specific time range
  kscorectl transfer export --type events --since 2024-01-15T00:00:00Z \
    --until 2024-01-16T00:00:00Z`,
		RunE: func(cmd *cobra.Command, args []string) error {
			timeRange, err := parseTimeRange(since, until)
			if err != nil {
				return err
			}

			if output == "" {
				output = fmt.Sprintf("kscore-%s-export-%s.tar.gz",
					exportType, time.Now().UTC().Format("20060102-150405"))
			}

			var signingKey []byte
			if signKey != "" {
				signingKey, err = os.ReadFile(signKey) //nolint:gosec // G304: user-provided key path
				if err != nil {
					return fmt.Errorf("reading signing key: %w", err)
				}
			}

			cfg := transfer.ExporterConfig{
				ExportType:   transfer.ExportType(exportType),
				OutputPath:   output,
				CreatedBy:    createdBy,
				SigningKey:    signingKey,
				Encrypt:      encryptTo != "",
				AgeRecipient: encryptTo,
			}
			if timeRange != nil {
				cfg.TimeRange = timeRange
			}

			exp, err := transfer.NewExporter(cfg)
			if err != nil {
				return err
			}

			if err := registerCollectors(exp, exportType, eventsDB, stateDB); err != nil {
				return err
			}

			manifest, err := exp.Export(cmd.Context())
			if err != nil {
				return err
			}

			finalPath := output
			if encryptTo != "" {
				finalPath += ".age"
			}

			return outputResult(outputFormat, manifest, func() {
				fmt.Fprintf(cmd.OutOrStdout(), "Export Complete\n")
				fmt.Fprintf(cmd.OutOrStdout(), "  Type:      %s\n", manifest.ExportType)
				fmt.Fprintf(cmd.OutOrStdout(), "  Created:   %s\n", manifest.Created.Format(time.RFC3339))
				if manifest.CreatedBy != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "  Author:    %s\n", manifest.CreatedBy)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  Datasets:  %d\n", len(manifest.DataSets))
				for _, ds := range manifest.DataSets {
					fmt.Fprintf(cmd.OutOrStdout(), "    - %s: %d records\n", ds.Name, ds.Count)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  Signed:    %v\n", manifest.RequiresVerification)
				fmt.Fprintf(cmd.OutOrStdout(), "  Encrypted: %v\n", manifest.Encrypted)
				fmt.Fprintf(cmd.OutOrStdout(), "  Output:    %s\n", finalPath)
			})
		},
	}

	cmd.Flags().StringVar(&exportType, "type", "", "Export type: audit, events, state, full (required)")
	cmd.Flags().StringVar(&since, "since", "", "Start of time range (duration like 24h or RFC3339 timestamp)")
	cmd.Flags().StringVar(&until, "until", "", "End of time range (RFC3339 timestamp; default: now)")
	cmd.Flags().StringVarP(&output, "output", "O", "", "Output archive path")
	cmd.Flags().StringVar(&signKey, "sign-key", "", "PEM private key path for signing")
	cmd.Flags().StringVar(&encryptTo, "encrypt-to", "", "age public key for encryption")
	cmd.Flags().StringVar(&createdBy, "created-by", "", "Creator identifier")
	cmd.Flags().StringVar(&eventsDB, "events-db", "events.db", "Path to events SQLite database")
	cmd.Flags().StringVar(&stateDB, "state-db", "", "Path to state history SQLite database")
	_ = cmd.MarkFlagRequired("type")

	return cmd
}

func newImportCmd() *cobra.Command {
	var (
		verifyKey   string
		ageIdentity string
		preview     bool
		outputDir   string
		datasets    string
	)

	cmd := &cobra.Command{
		Use:   "import <package>",
		Short: "Import an exported data package",
		Long: `Import and verify an export package, extracting data files for consumption.

The importer decrypts (if encrypted), verifies signatures and checksums, and
extracts data files. Actual data ingestion (loading into SIEM, etc.) is the
consumer's responsibility.

Examples:
  # Preview package contents
  kscorectl transfer import package.tar.gz --preview

  # Import with verification
  kscorectl transfer import package.tar.gz --verify-key release.pub \
    --output-dir ./imported

  # Import encrypted package
  kscorectl transfer import package.tar.gz.age \
    --age-identity identity.txt --output-dir ./imported

  # Import only specific datasets
  kscorectl transfer import package.tar.gz \
    --output-dir ./imported --datasets audit,events`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			packagePath := args[0]

			var trustedKeys [][]byte
			if verifyKey != "" {
				keyData, err := os.ReadFile(verifyKey) //nolint:gosec // G304: user-provided key path
				if err != nil {
					return fmt.Errorf("reading verify key: %w", err)
				}
				trustedKeys = append(trustedKeys, keyData)
			}

			var dsFilter []string
			if datasets != "" {
				dsFilter = strings.Split(datasets, ",")
			}

			cfg := transfer.ImportConfig{
				PackagePath: packagePath,
				PreviewOnly: preview,
				TrustedKeys: trustedKeys,
				AgeIdentity: ageIdentity,
				DataSets:    dsFilter,
				OutputDir:   outputDir,
			}

			imp, err := transfer.NewImporter(cfg)
			if err != nil {
				return err
			}

			result, err := imp.Import(cmd.Context())
			if err != nil {
				return err
			}

			return outputResult(outputFormat, result, func() {
				if preview {
					fmt.Fprintf(cmd.OutOrStdout(), "Package Preview\n")
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "Import Complete\n")
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  Type:       %s\n", result.Manifest.ExportType)
				fmt.Fprintf(cmd.OutOrStdout(), "  Created:    %s\n", result.Manifest.Created.Format(time.RFC3339))
				if result.Manifest.CreatedBy != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "  Author:     %s\n", result.Manifest.CreatedBy)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  Verified:   %v\n", result.Verified)
				fmt.Fprintf(cmd.OutOrStdout(), "  Signature:  %v\n", result.SignatureValid)
				fmt.Fprintf(cmd.OutOrStdout(), "  Checksums:  %v\n", result.ChecksumsValid)
				fmt.Fprintf(cmd.OutOrStdout(), "  Datasets:\n")
				for _, ds := range result.DataSets {
					if ds.Path != "" {
						fmt.Fprintf(cmd.OutOrStdout(), "    - %s: %d records → %s\n", ds.Name, ds.Count, ds.Path)
					} else {
						fmt.Fprintf(cmd.OutOrStdout(), "    - %s: %d records\n", ds.Name, ds.Count)
					}
				}
				for _, w := range result.Warnings {
					fmt.Fprintf(cmd.ErrOrStderr(), "  Warning: %s\n", w)
				}
			})
		},
	}

	cmd.Flags().StringVar(&verifyKey, "verify-key", "", "PEM public key path for signature verification")
	cmd.Flags().StringVar(&ageIdentity, "age-identity", "", "age identity file path for decryption")
	cmd.Flags().BoolVar(&preview, "preview", false, "Preview contents without extracting")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Extraction destination directory")
	cmd.Flags().StringVar(&datasets, "datasets", "", "Comma-separated dataset filter")

	return cmd
}

func newVerifyCmd() *cobra.Command {
	var verifyKey string

	cmd := &cobra.Command{
		Use:   "verify <package>",
		Short: "Verify an export package's integrity",
		Long: `Verify an export package's signatures, checksums, and manifest.

Examples:
  kscorectl transfer verify package.tar.gz --verify-key release.pub`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			packagePath := args[0]

			var trustedKeys [][]byte
			if verifyKey != "" {
				keyData, err := os.ReadFile(verifyKey) //nolint:gosec // G304: user-provided key path
				if err != nil {
					return fmt.Errorf("reading verify key: %w", err)
				}
				trustedKeys = append(trustedKeys, keyData)
			}

			cfg := transfer.ImportConfig{
				PackagePath: packagePath,
				PreviewOnly: true,
				TrustedKeys: trustedKeys,
			}

			imp, err := transfer.NewImporter(cfg)
			if err != nil {
				return err
			}

			result, err := imp.Import(cmd.Context())
			if err != nil {
				return err
			}

			return outputResult(outputFormat, result, func() {
				fmt.Fprintf(cmd.OutOrStdout(), "Package Verification\n")
				fmt.Fprintf(cmd.OutOrStdout(), "  File:       %s\n", packagePath)
				fmt.Fprintf(cmd.OutOrStdout(), "  Verified:   %v\n", result.Verified)
				fmt.Fprintf(cmd.OutOrStdout(), "  Signature:  %v\n", result.SignatureValid)
				fmt.Fprintf(cmd.OutOrStdout(), "  Checksums:  %v\n", result.ChecksumsValid)
				fmt.Fprintf(cmd.OutOrStdout(), "  Type:       %s\n", result.Manifest.ExportType)
				fmt.Fprintf(cmd.OutOrStdout(), "  Datasets:   %d\n", len(result.DataSets))
				for _, ds := range result.DataSets {
					fmt.Fprintf(cmd.OutOrStdout(), "    - %s: %d records\n", ds.Name, ds.Count)
				}
				for _, w := range result.Warnings {
					fmt.Fprintf(cmd.ErrOrStderr(), "  Warning: %s\n", w)
				}
			})
		},
	}

	cmd.Flags().StringVar(&verifyKey, "verify-key", "", "PEM public key path for signature verification")

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

func registerCollectors(exp *transfer.Exporter, exportType, eventsDB, stateDB string) error {
	switch exportType {
	case "audit":
		exp.AddCollector(transfer.NewAuditCollector(noopAuditQuery))
	case "events":
		store, err := openEventStore(eventsDB)
		if err != nil {
			return err
		}
		exp.AddCollector(transfer.NewEventCollector(store))
	case "state":
		store, err := openStateStore(stateDB)
		if err != nil {
			return err
		}
		exp.AddCollector(transfer.NewStateCollector(store))
	case "full":
		exp.AddCollector(transfer.NewAuditCollector(noopAuditQuery))
		if eventsDB != "" {
			store, err := openEventStore(eventsDB)
			if err != nil {
				return err
			}
			exp.AddCollector(transfer.NewEventCollector(store))
		}
		if stateDB != "" {
			store, err := openStateStore(stateDB)
			if err != nil {
				return err
			}
			exp.AddCollector(transfer.NewStateCollector(store))
		}
	default:
		return fmt.Errorf("unsupported export type: %s", exportType)
	}
	return nil
}

func openEventStore(dbPath string) (events.EventStore, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("--events-db path is required for events export")
	}
	store, err := events.NewSQLiteEventStore(&events.EventStoreConfig{Path: dbPath})
	if err != nil {
		return nil, fmt.Errorf("opening events database: %w", err)
	}
	return store, nil
}

func openStateStore(dbPath string) (history.Store, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("--state-db path is required for state export")
	}
	store, err := history.NewSQLiteStore(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening state history database: %w", err)
	}
	return store, nil
}

func noopAuditQuery(_ context.Context, _ *transfer.CollectOptions) ([]*transfer.AuditEntry, error) {
	return nil, nil
}

func parseTimeRange(since, until string) (*transfer.TimeRange, error) {
	if since == "" && until == "" {
		return nil, nil
	}

	var start, end time.Time
	var err error

	if since != "" {
		start, err = parseTimeOrDuration(since)
		if err != nil {
			return nil, fmt.Errorf("invalid --since value: %w", err)
		}
	}

	if until != "" {
		end, err = time.Parse(time.RFC3339, until)
		if err != nil {
			return nil, fmt.Errorf("invalid --until value (expected RFC3339): %w", err)
		}
	} else {
		end = time.Now().UTC()
	}

	return &transfer.TimeRange{Start: start, End: end}, nil
}

func parseTimeOrDuration(s string) (time.Time, error) {
	if d, err := time.ParseDuration(s); err == nil {
		return time.Now().UTC().Add(-d), nil
	}
	return time.Parse(time.RFC3339, s)
}

func newSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Manage sync windows for periodic-connectivity environments",
		Long: `Manage sync windows that allow periodic data synchronization in environments
with intermittent network connectivity.

Sync windows define time-limited windows during which data can be transferred
between air-gapped and connected networks, with bandwidth limiting and
operation prioritization.

Note: Sync window management requires a running sync daemon. These commands
will be fully functional when daemon mode is available.`,
	}

	syncDaemonRequired := func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("not yet available — sync daemon required")
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List configured sync windows",
			RunE:  syncDaemonRequired,
		},
		&cobra.Command{
			Use:   "show <name>",
			Short: "Show sync window details",
			Args:  cobra.ExactArgs(1),
			RunE:  syncDaemonRequired,
		},
		&cobra.Command{
			Use:   "trigger <name>",
			Short: "Trigger a sync window immediately",
			Args:  cobra.ExactArgs(1),
			RunE:  syncDaemonRequired,
		},
		&cobra.Command{
			Use:   "pause <name>",
			Short: "Pause a running sync window",
			Args:  cobra.ExactArgs(1),
			RunE:  syncDaemonRequired,
		},
		&cobra.Command{
			Use:   "resume <name>",
			Short: "Resume a paused sync window",
			Args:  cobra.ExactArgs(1),
			RunE:  syncDaemonRequired,
		},
		&cobra.Command{
			Use:   "cancel <name>",
			Short: "Cancel a running sync window",
			Args:  cobra.ExactArgs(1),
			RunE:  syncDaemonRequired,
		},
		&cobra.Command{
			Use:   "history",
			Short: "Show sync window execution history",
			RunE:  syncDaemonRequired,
		},
	)

	return cmd
}

func newDiodeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diode",
		Short: "Send and receive data via UDP data diode",
		Long: `Transfer data over a unidirectional UDP data diode connection.

Data diodes provide hardware-enforced one-way data transfer, commonly used
in classified and high-security environments. The sender transmits data via
UDP with optional forward error correction (FEC) for packet loss recovery.`,
	}

	cmd.AddCommand(
		newDiodeSendCmd(),
		newDiodeReceiveCmd(),
	)

	return cmd
}

func newDiodeSendCmd() *cobra.Command {
	var (
		address      string
		filePath     string
		packetSize   int
		rateLimit    int64
		fecEnabled   bool
		fecGroupSize int
	)

	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send a file through a data diode",
		Long: `Send a file through a UDP data diode connection.

Examples:
  kscorectl transfer diode send --address 10.0.0.2:9000 --file export.tar.gz
  kscorectl transfer diode send --address 10.0.0.2:9000 --file data.bin --fec
  kscorectl transfer diode send --address 10.0.0.2:9000 --file data.bin --rate-limit 1048576`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := diode.Config{
				Address:      address,
				PacketSize:   packetSize,
				RateLimit:    rateLimit,
				FECEnabled:   fecEnabled,
				FECGroupSize: fecGroupSize,
			}
			cfg.Defaults()

			sender, err := diode.NewSender(cfg)
			if err != nil {
				return fmt.Errorf("creating sender: %w", err)
			}
			defer sender.Close()

			fmt.Fprintf(cmd.OutOrStdout(), "Sending %s to %s...\n", filePath, address)
			if fecEnabled {
				fmt.Fprintf(cmd.OutOrStdout(), "  FEC enabled (group size: %d)\n", cfg.FECGroupSize)
			}
			if rateLimit > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "  Rate limit: %d bytes/sec\n", rateLimit)
			}

			if err := sender.SendFile(cmd.Context(), filePath); err != nil {
				return fmt.Errorf("send failed: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Send complete.")
			return nil
		},
	}

	cmd.Flags().StringVar(&address, "address", "", "Destination address host:port (required)")
	cmd.Flags().StringVar(&filePath, "file", "", "File to send (required)")
	cmd.Flags().IntVar(&packetSize, "packet-size", 0, "UDP packet size in bytes (default: 1400)")
	cmd.Flags().Int64Var(&rateLimit, "rate-limit", 0, "Bandwidth limit in bytes/sec (0 = unlimited)")
	cmd.Flags().BoolVar(&fecEnabled, "fec", false, "Enable forward error correction")
	cmd.Flags().IntVar(&fecGroupSize, "fec-redundancy", 0, "FEC group size (default: 5)")
	_ = cmd.MarkFlagRequired("address")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

func newDiodeReceiveCmd() *cobra.Command {
	var (
		listenAddr string
		outputDir  string
		timeout    time.Duration
		fecEnabled bool
	)

	cmd := &cobra.Command{
		Use:   "receive",
		Short: "Receive a file from a data diode",
		Long: `Listen for incoming data diode transfers over UDP.

Examples:
  kscorectl transfer diode receive --listen :9000 --output-dir ./received
  kscorectl transfer diode receive --listen :9000 --output-dir ./received --fec --timeout 60s`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := diode.Config{
				Address:    listenAddr,
				FECEnabled: fecEnabled,
				Timeout:    timeout,
			}
			cfg.Defaults()

			receiver, err := diode.NewReceiver(cfg)
			if err != nil {
				return fmt.Errorf("creating receiver: %w", err)
			}
			defer receiver.Close()

			fmt.Fprintf(cmd.OutOrStdout(), "Listening on %s for incoming transfers...\n", listenAddr)
			if fecEnabled {
				fmt.Fprintln(cmd.OutOrStdout(), "  FEC recovery enabled")
			}

			outPath, err := receiver.ReceiveToFile(cmd.Context(), outputDir)
			if err != nil {
				return fmt.Errorf("receive failed: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Received file: %s\n", outPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&listenAddr, "listen", ":9000", "Listen address host:port")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Output directory for received files (required)")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Receive timeout")
	cmd.Flags().BoolVar(&fecEnabled, "fec", false, "Enable forward error correction recovery")
	_ = cmd.MarkFlagRequired("output-dir")

	return cmd
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
