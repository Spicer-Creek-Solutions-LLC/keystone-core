// Package auditutil provides audit logging utilities for CLI commands.
package auditutil

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/shawnbutts/keystone-core/internal/audit"
	"github.com/spf13/cobra"
)

// CommandAudit coordinates audit logging for a CLI command execution.
type CommandAudit struct {
	entry       *audit.AuditEntry
	start       time.Time
	cleanup     func()
	initialized bool
}

// Attach installs audit hooks on the root command and returns a handler for failures.
func Attach(rootCmd *cobra.Command, tool string, level, backend *string) *CommandAudit {
	handler := &CommandAudit{}

	prevPreRunE := rootCmd.PersistentPreRunE
	prevPostRun := rootCmd.PersistentPostRun

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if prevPreRunE != nil {
			if err := prevPreRunE(cmd, args); err != nil {
				return err
			}
		}
		if !handler.initialized {
			handler.cleanup = Init(cmd.Context(), tool, *level, *backend)
			handler.initialized = true
		}

		handler.entry = audit.StartEntry(audit.ActionCommandExecuted, cmd.CommandPath())
		handler.entry.Args = os.Args[1:]
		handler.entry.Target = cmd.CommandPath()
		handler.start = time.Now()
		return nil
	}

	rootCmd.PersistentPostRun = func(cmd *cobra.Command, args []string) {
		if prevPostRun != nil {
			prevPostRun(cmd, args)
		}
		handler.log(cmd.Context(), audit.ResultSuccess, 0, nil)
		if handler.cleanup != nil {
			handler.cleanup()
		}
	}

	return handler
}

// LogFailure logs a failed audit entry if one was created.
func (c *CommandAudit) LogFailure(err error) {
	c.log(context.Background(), audit.ResultFailure, 1, err)
	if c.cleanup != nil {
		c.cleanup()
	}
}

func (c *CommandAudit) log(ctx context.Context, result audit.AuditResult, exitCode int, err error) {
	if c.entry == nil {
		return
	}

	c.entry.Result = result
	c.entry.ExitCode = exitCode
	c.entry.DurationMS = time.Since(c.start).Milliseconds()
	if err != nil {
		c.entry.Error = err.Error()
	}
	_ = audit.Log(ctx, c.entry)
}

// Init configures the global auditor and returns a cleanup function.
func Init(ctx context.Context, tool, level, backend string) func() {
	config := &audit.AuditConfig{
		Level:   audit.AuditLevel(level),
		Backend: backend,
	}
	if err := audit.Init(ctx, tool, config); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to initialize audit logging: %v\n", err)
	}
	return func() {
		_ = audit.Close()
	}
}
