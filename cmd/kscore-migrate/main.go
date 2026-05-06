// kscore-migrate copies state from a SQLite source to a PostgreSQL
// target. One-shot tool — does not load a YAML config; takes connection
// info via flags. The migration logic lives in internal/state; this
// binary is the user-facing CLI on top of state.Migrator.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/pkg/version"
)

func main() {
	if err := newCommand().Execute(); err != nil {
		os.Exit(1)
	}
}

// newCommand returns the root cobra command with run/validate/version
// subcommands attached. Exposed for testing.
func newCommand() *cobra.Command {
	info := version.Get()
	root := &cobra.Command{
		Use:           "kscore-migrate",
		Short:         "Migrate Keystone Core state from SQLite to PostgreSQL",
		Version:       info.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetVersionTemplate(versionTemplate(info))
	root.AddCommand(newRunCommand())
	root.AddCommand(newValidateCommand())
	root.AddCommand(newVersionCommand())
	return root
}

func versionTemplate(info version.Info) string {
	return fmt.Sprintf("kscore-migrate %s\ncommit: %s\nbuilt:  %s\n",
		info.Version, info.GitCommit, info.BuildDate)
}

// ---- run subcommand -------------------------------------------------------

type runFlags struct {
	sqlite          string
	postgres        string
	dryRun          bool
	batchSize       int
	continueOnError bool
	skipExisting    bool
	txlog           string
	quiet           bool
}

func newRunCommand() *cobra.Command {
	var f runFlags
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Copy rows from SQLite source to PostgreSQL target",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := signal.NotifyContext(cmd.Context(),
				os.Interrupt, syscall.SIGTERM)
			defer cancel()
			return runMigration(ctx, cmd.OutOrStdout(), &f)
		},
	}
	addRunFlags(cmd, &f)
	return cmd
}

func addRunFlags(cmd *cobra.Command, f *runFlags) {
	cmd.Flags().StringVar(&f.sqlite, "sqlite", "", "path to SQLite source database (required)")
	cmd.Flags().StringVar(&f.postgres, "postgres", "", "PostgreSQL target DSN (required)")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "read source and emit plan; no writes to target")
	cmd.Flags().IntVar(&f.batchSize, "batch-size", 100, "rows read per page")
	cmd.Flags().BoolVar(&f.continueOnError, "continue-on-error", false, "record per-row errors and continue")
	cmd.Flags().BoolVar(&f.skipExisting, "skip-existing", false, "INSERT ... ON CONFLICT DO NOTHING")
	cmd.Flags().StringVar(&f.txlog, "txlog", "", "append-only JSONL audit trail path (optional)")
	cmd.Flags().BoolVar(&f.quiet, "quiet", false, "suppress per-batch progress output")
	_ = cmd.MarkFlagRequired("sqlite")
	_ = cmd.MarkFlagRequired("postgres")
}

func runMigration(ctx context.Context, out io.Writer, f *runFlags) error {
	src, dst, err := openStores(f.sqlite, f.postgres)
	if err != nil {
		return err
	}
	defer func() {
		_ = src.Close()
		_ = dst.Close()
	}()

	opts := state.MigrationOptions{
		DryRun:          f.dryRun,
		BatchSize:       f.batchSize,
		ContinueOnError: f.continueOnError,
		SkipExisting:    f.skipExisting,
		TxLogPath:       f.txlog,
	}
	if !f.quiet {
		opts.ProgressCallback = func(p state.ProgressUpdate) {
			fmt.Fprintln(out, formatProgress(p))
		}
	}

	m := state.NewMigrator(src, dst)
	stats, err := m.Migrate(ctx, opts)
	printStats(out, stats, f.dryRun)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}
	if len(stats.Errors) > 0 && !f.continueOnError {
		return fmt.Errorf("%d row error(s) recorded", len(stats.Errors))
	}
	return nil
}

func formatProgress(p state.ProgressUpdate) string {
	pct := 0.0
	if p.RowsTotal > 0 {
		pct = float64(p.RowsCompleted) / float64(p.RowsTotal) * 100
	}
	eta := "?"
	if p.ETA > 0 {
		eta = p.ETA.Round(time.Second).String()
	}
	return fmt.Sprintf("[%s] %d/%d (%.1f%%) — %.1f rows/s — ETA %s",
		p.Table, p.RowsCompleted, p.RowsTotal, pct, p.RowsPerSecond, eta)
}

func printStats(out io.Writer, s *state.MigrationStats, dryRun bool) {
	if s == nil {
		return
	}
	header := "Migration"
	if dryRun {
		header = "Migration plan (dry-run)"
	}
	fmt.Fprintf(out, "\n%s completed in %s\n\n", header, s.Duration.Round(time.Millisecond))

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TABLE\tREAD\tWRITTEN\tSKIPPED\tERRORED")
	for _, table := range []string{"agents", "commands", "batch_jobs", "batch_agent_results"} {
		ts := s.Tables[table]
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\n",
			table, ts.Read, ts.Written, ts.Skipped, ts.Errored)
	}
	_ = w.Flush()

	if len(s.Errors) > 0 {
		fmt.Fprintf(out, "\n%d error(s):\n", len(s.Errors))
		for _, e := range s.Errors {
			fmt.Fprintf(out, "  %s/%s: %v\n", e.Table, e.ID, e.Err)
		}
	}
}

// ---- validate subcommand --------------------------------------------------

type validateFlags struct {
	sqlite   string
	postgres string
}

func newValidateCommand() *cobra.Command {
	var f validateFlags
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Compare row counts between SQLite source and PostgreSQL target",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runValidate(cmd.Context(), cmd.OutOrStdout(), &f)
		},
	}
	cmd.Flags().StringVar(&f.sqlite, "sqlite", "", "path to SQLite source database (required)")
	cmd.Flags().StringVar(&f.postgres, "postgres", "", "PostgreSQL target DSN (required)")
	_ = cmd.MarkFlagRequired("sqlite")
	_ = cmd.MarkFlagRequired("postgres")
	return cmd
}

func runValidate(ctx context.Context, out io.Writer, f *validateFlags) error {
	src, dst, err := openStores(f.sqlite, f.postgres)
	if err != nil {
		return err
	}
	defer func() {
		_ = src.Close()
		_ = dst.Close()
	}()

	m := state.NewMigrator(src, dst)
	vr, err := m.ValidateMigration(ctx)
	if err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	fmt.Fprintln(out, "Validation:")
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, table := range []string{"agents", "commands", "batch_jobs", "batch_agent_results"} {
		t := vr.Tables[table]
		status := "OK"
		if !t.Match {
			status = "MISMATCH"
		}
		fmt.Fprintf(w, "  %s\tsource=%d\ttarget=%d\t%s\n",
			table, t.SourceCount, t.TargetCount, status)
	}
	_ = w.Flush()

	if !vr.Match {
		var bad []string
		for table, t := range vr.Tables {
			if !t.Match {
				bad = append(bad, table)
			}
		}
		return fmt.Errorf("FAIL: %d table(s) mismatched: %s",
			len(bad), strings.Join(bad, ", "))
	}
	fmt.Fprintln(out, "\nPASS")
	return nil
}

// ---- version subcommand ---------------------------------------------------

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version metadata",
		Run: func(cmd *cobra.Command, _ []string) {
			info := version.Get()
			fmt.Fprint(cmd.OutOrStdout(), versionTemplate(info))
		},
	}
}

// ---- helpers --------------------------------------------------------------

// openStores opens the SQLite source and Postgres target through the
// public state.NewStore factory and type-asserts each to its concrete
// type for use with state.NewMigrator.
func openStores(sqlitePath, postgresDSN string) (*state.SQLiteStore, *state.PostgreSQLStore, error) {
	srcStore, err := state.NewStore(&state.Config{
		Backend: state.BackendSQLite,
		SQLite:  state.SQLiteConfig{Path: sqlitePath},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("open sqlite source %q: %w", sqlitePath, err)
	}
	src, ok := srcStore.(*state.SQLiteStore)
	if !ok {
		_ = srcStore.Close()
		return nil, nil, fmt.Errorf("sqlite source: unexpected store type %T", srcStore)
	}

	dstStore, err := state.NewStore(&state.Config{
		Backend:    state.BackendPostgreSQL,
		PostgreSQL: state.PostgreSQLConfig{DSN: postgresDSN},
	})
	if err != nil {
		_ = src.Close()
		return nil, nil, fmt.Errorf("open postgres target: %w", err)
	}
	dst, ok := dstStore.(*state.PostgreSQLStore)
	if !ok {
		_ = src.Close()
		_ = dstStore.Close()
		return nil, nil, fmt.Errorf("postgres target: unexpected store type %T", dstStore)
	}
	return src, dst, nil
}
