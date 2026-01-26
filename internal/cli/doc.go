// Package cli provides reusable utilities for Keystone Core command-line interfaces.
//
// The cli package contains shared functionality used across all Keystone CLI tools
// (kscorectl, kscore-module, kscore-state, kscore-exec, etc.) to ensure consistent
// user experience, error handling, and output formatting.
//
// # Subpackages
//
//   - auditutil: Audit logging coordination for CLI command execution
//   - deprecation: Deprecation warning system with migration guidance
//   - errors: Categorized error types with context and kind classification
//   - output: Structured output formatting (text, JSON, YAML, table)
//
// # Audit Logging
//
// The auditutil subpackage integrates CLI commands with the audit system:
//
//	import "github.com/your-org/keystone-core/pkg/cli/auditutil"
//
//	audit := auditutil.CommandAudit{}
//	cleanup := audit.Init(cfg)
//	defer cleanup()
//	audit.Attach(rootCmd)
//
// # Error Handling
//
// The errors subpackage provides categorized errors for consistent error reporting:
//
//	import clierrors "github.com/your-org/keystone-core/pkg/cli/errors"
//
//	err := clierrors.New(clierrors.KindNotFound, "agent %s not found", agentID)
//	err := clierrors.Wrap(clierrors.KindUnavailable, originalErr, "server unreachable")
//
// Error kinds include: KindInvalidArgument, KindNotFound, KindConflict,
// KindUnavailable, and KindInternal.
//
// # Output Formatting
//
// The output subpackage handles structured output in multiple formats:
//
//	import "github.com/your-org/keystone-core/pkg/cli/output"
//
//	format, _ := output.ParseFormat("json")
//	output.Write(os.Stdout, format, data)
//
// Supported formats: text, json, yaml, table.
//
// # Deprecation Warnings
//
// The deprecation subpackage tracks and displays deprecation warnings:
//
//	import "github.com/your-org/keystone-core/pkg/cli/deprecation"
//
//	info := deprecation.Info{
//	    DeprecatedIn: "0.9.0",
//	    RemoveIn:     "1.0.0",
//	    Replacement:  "kscorectl agents list",
//	}
//	deprecation.DefaultRegistry.Warn("old-command", info)
package cli
