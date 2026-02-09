package bootstrap

import (
	"context"
	"fmt"
	"io"
	"strings"
)

func runDatabaseMigration(ctx context.Context, cfg *Config, output io.Writer, verbose bool) error {
	if cfg == nil || cfg.MigrateFromSQLite == "" {
		return nil
	}
	if !strings.EqualFold(cfg.Storage, "postgres") {
		return fmt.Errorf("sqlite migration requires postgres storage backend")
	}
	dsn := buildPostgresDSN(cfg)
	cmd := CommandPlan{
		Name: "kscore-migrate",
		Args: []string{"run", "--sqlite", cfg.MigrateFromSQLite, "--postgres", dsn},
	}
	if cfg.MigrateBatchSize > 0 {
		cmd.Args = append(cmd.Args, "--batch-size", fmt.Sprintf("%d", cfg.MigrateBatchSize))
	}
	if cfg.MigrateContinueOnError {
		cmd.Args = append(cmd.Args, "--continue-on-error")
	}
	if cfg.MigrateSkipExisting {
		cmd.Args = append(cmd.Args, "--skip-existing")
	}
	if verbose {
		fmt.Fprintln(output, "running database migration")
	}
	return runCommands(ctx, []CommandPlan{cmd}, output, verbose)
}
