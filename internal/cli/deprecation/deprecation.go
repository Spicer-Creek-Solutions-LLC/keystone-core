// Package deprecation provides utilities for deprecating CLI commands and flags
// with structured warnings and migration guidance.
package deprecation

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

// Info holds metadata about a deprecated command or subcommand.
type Info struct {
	// DeprecatedIn is the version when the command was deprecated.
	DeprecatedIn string

	// RemoveIn is the version when the command will be removed.
	RemoveIn string

	// Replacement is the new command to use instead.
	// Example: "kscore-blueprint-publish publish" or "kscore-federation list"
	Replacement string

	// MigrationGuide is a URL to documentation about the migration.
	MigrationGuide string

	// Message is an optional custom message to display.
	Message string
}

// Registry tracks deprecated commands and ensures warnings are shown only once per session.
type Registry struct {
	mu       sync.Mutex
	warnings map[string]bool
	silent   bool
}

// DefaultRegistry is the global deprecation registry.
var DefaultRegistry = &Registry{
	warnings: make(map[string]bool),
}

// SetSilent disables deprecation warnings (useful for testing or scripting).
func (r *Registry) SetSilent(silent bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.silent = silent
}

// IsSilent returns whether deprecation warnings are suppressed.
func (r *Registry) IsSilent() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.silent
}

// HasWarned returns true if a warning has already been shown for the given command path.
func (r *Registry) HasWarned(cmdPath string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.warnings[cmdPath]
}

// MarkWarned records that a warning has been shown for the given command path.
func (r *Registry) MarkWarned(cmdPath string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.warnings[cmdPath] = true
}

// Reset clears all warning records (useful for testing).
func (r *Registry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.warnings = make(map[string]bool)
}

// DeprecateCommand marks a command as deprecated and attaches warning hooks.
// The warning is displayed when the command is executed.
func DeprecateCommand(cmd *cobra.Command, info *Info) {
	DeprecateCommandWithRegistry(cmd, info, DefaultRegistry)
}

// DeprecateCommandWithRegistry marks a command as deprecated using a specific registry.
func DeprecateCommandWithRegistry(cmd *cobra.Command, info *Info, registry *Registry) {
	// Store deprecation metadata in annotations
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.Annotations["deprecated"] = "true"
	cmd.Annotations["deprecated_in"] = info.DeprecatedIn
	cmd.Annotations["remove_in"] = info.RemoveIn
	cmd.Annotations["replacement"] = info.Replacement

	// Mark as deprecated in Cobra's native system too (adds to help text)
	cmd.Deprecated = formatShortDeprecation(info)

	// Hook into PreRunE to show detailed warning
	prevPreRunE := cmd.PreRunE
	prevPreRun := cmd.PreRun

	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		showWarning(c, info, registry)

		if prevPreRunE != nil {
			return prevPreRunE(c, args)
		}
		if prevPreRun != nil {
			prevPreRun(c, args)
		}
		return nil
	}
	// Clear PreRun since we've merged it into PreRunE
	cmd.PreRun = nil
}

// DeprecateSubcommand is a convenience function for deprecating a subcommand
// that is being moved to a new top-level command.
func DeprecateSubcommand(parent *cobra.Command, subcommandName string, info *Info) {
	for _, sub := range parent.Commands() {
		if sub.Name() == subcommandName {
			DeprecateCommand(sub, info)
			return
		}
	}
}

// AddAlias creates a deprecated alias command that calls the new command.
// This is useful for providing backward compatibility when a command is moved.
func AddAlias(parent *cobra.Command, oldName, newCmdPath string, info *Info) *cobra.Command {
	alias := &cobra.Command{
		Use:    oldName,
		Short:  fmt.Sprintf("(Deprecated) Use '%s' instead", newCmdPath),
		Hidden: false, // Keep visible so users discover the deprecation
		RunE: func(cmd *cobra.Command, args []string) error {
			showWarning(cmd, info, DefaultRegistry)
			return fmt.Errorf("command '%s' has been moved to '%s'", cmd.CommandPath(), newCmdPath)
		},
	}

	if info.Replacement != "" {
		alias.Deprecated = formatShortDeprecation(info)
	}

	parent.AddCommand(alias)
	return alias
}

// showWarning displays the deprecation warning if not already shown.
func showWarning(cmd *cobra.Command, info *Info, registry *Registry) {
	if registry.IsSilent() {
		return
	}

	cmdPath := cmd.CommandPath()
	if registry.HasWarned(cmdPath) {
		return
	}
	registry.MarkWarned(cmdPath)

	warning := FormatWarning(cmdPath, info)
	fmt.Fprint(os.Stderr, warning)
}

// FormatWarning creates a formatted deprecation warning message.
func FormatWarning(cmdPath string, info *Info) string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(strings.Repeat("=", 70))
	b.WriteString("\n")
	b.WriteString("  DEPRECATION WARNING\n")
	b.WriteString(strings.Repeat("=", 70))
	b.WriteString("\n\n")

	b.WriteString(fmt.Sprintf("  Command: %s\n", cmdPath))

	if info.DeprecatedIn != "" {
		b.WriteString(fmt.Sprintf("  Deprecated in: v%s\n", strings.TrimPrefix(info.DeprecatedIn, "v")))
	}

	if info.RemoveIn != "" {
		b.WriteString(fmt.Sprintf("  Will be removed in: v%s\n", strings.TrimPrefix(info.RemoveIn, "v")))
	}

	b.WriteString("\n")

	if info.Message != "" {
		b.WriteString(fmt.Sprintf("  %s\n\n", info.Message))
	}

	if info.Replacement != "" {
		b.WriteString(fmt.Sprintf("  Use instead: %s\n", info.Replacement))
	}

	if info.MigrationGuide != "" {
		b.WriteString(fmt.Sprintf("  Migration guide: %s\n", info.MigrationGuide))
	}

	b.WriteString("\n")
	b.WriteString(strings.Repeat("=", 70))
	b.WriteString("\n\n")

	return b.String()
}

// formatShortDeprecation creates a short deprecation message for Cobra's help text.
func formatShortDeprecation(info *Info) string {
	if info.Replacement != "" {
		return fmt.Sprintf("use '%s' instead", info.Replacement)
	}
	if info.RemoveIn != "" {
		return fmt.Sprintf("will be removed in v%s", strings.TrimPrefix(info.RemoveIn, "v"))
	}
	return "this command is deprecated"
}

// IsDeprecated checks if a command is marked as deprecated.
func IsDeprecated(cmd *cobra.Command) bool {
	if cmd.Annotations == nil {
		return false
	}
	return cmd.Annotations["deprecated"] == "true"
}

// GetDeprecationInfo extracts deprecation info from command annotations.
func GetDeprecationInfo(cmd *cobra.Command) *Info {
	if !IsDeprecated(cmd) {
		return nil
	}

	return &Info{
		DeprecatedIn: cmd.Annotations["deprecated_in"],
		RemoveIn:     cmd.Annotations["remove_in"],
		Replacement:  cmd.Annotations["replacement"],
	}
}

// CheckRemovalDate returns true if the removal date has passed or is approaching.
// This can be used to escalate warnings or fail commands near removal.
func CheckRemovalDate(removalDate time.Time) (approaching bool, daysUntil int) {
	if removalDate.IsZero() {
		return false, -1
	}

	daysUntil = int(time.Until(removalDate).Hours() / 24)
	return daysUntil <= 30, daysUntil
}

// WarnIfApproachingRemoval prints an additional urgent warning if removal is near.
func WarnIfApproachingRemoval(removalDate time.Time) {
	approaching, days := CheckRemovalDate(removalDate)
	if !approaching {
		return
	}

	var msg string
	switch {
	case days < 0:
		msg = "  URGENT: This command's removal date has PASSED!\n"
	case days == 0:
		msg = "  URGENT: This command will be removed TODAY!\n"
	default:
		msg = fmt.Sprintf("  URGENT: This command will be removed in %d days!\n", days)
	}

	fmt.Fprintf(os.Stderr, "\n%s\n", msg)
}

// Migrations defines the mapping of deprecated commands to their replacements.
// This is used for documentation and automated migration tooling.
type Migrations struct {
	mu       sync.RWMutex
	mappings map[string]string
}

// DefaultMigrations is the global migrations registry.
var DefaultMigrations = &Migrations{
	mappings: make(map[string]string),
}

// Register adds a migration mapping from old command path to new command path.
func (m *Migrations) Register(oldPath, newPath string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mappings[oldPath] = newPath
}

// GetReplacement returns the replacement command for a deprecated command.
func (m *Migrations) GetReplacement(oldPath string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	newPath, ok := m.mappings[oldPath]
	return newPath, ok
}

// All returns all registered migrations.
func (m *Migrations) All() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]string, len(m.mappings))
	for k, v := range m.mappings {
		result[k] = v
	}
	return result
}
