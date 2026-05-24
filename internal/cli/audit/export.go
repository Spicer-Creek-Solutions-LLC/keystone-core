// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	intaudit "go.keystone-core.io/keystone-core/internal/audit"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func exportCmd(g *globals) *cobra.Command {
	var (
		format       string
		outputFile   string
		policyID     string
		user         string
		resourceType string
		action       string
		minSeverity  string
		since        string
		until        string
		redactKeys   []string
		redactPats   []string
		redactUser   bool
		redactRepl   string
	)
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Stream the audit log as JSON / JSONL / CSV (redaction applied on export)",
		Long: "Export the filtered audit log in a chosen wire format.\n\n" +
			"--format json|jsonl|csv (default jsonl). Streams page-by-page " +
			"from the server so a large audit table never buffers in memory. " +
			"Redaction is applied at the export boundary (PROJECT-DETAILS " +
			"§4.12): --redact-key drops metadata keys, --redact-pattern " +
			"regex-replaces metadata values + violation messages, " +
			"--redact-user blanks the user field.\n\n" +
			"The persistent -o/--output flag does not apply to export; use " +
			"--format.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runExport(cmd.Context(), cmd.OutOrStdout(), g, exportOpts{
				format, outputFile, policyID, user, resourceType, action,
				minSeverity, since, until, redactKeys, redactPats, redactUser, redactRepl,
			})
		},
	}
	cmd.Flags().StringVar(&format, "format", "jsonl", "export format: json | jsonl | csv")
	cmd.Flags().StringVar(&outputFile, "output-file", "", "write to this file instead of stdout")
	cmd.Flags().StringVar(&policyID, "policy-id", "", "filter by policy ID")
	cmd.Flags().StringVar(&user, "user", "", "filter by user")
	cmd.Flags().StringVar(&resourceType, "resource-type", "", "filter by resource type")
	cmd.Flags().StringVar(&action, "action", "", "filter by action")
	cmd.Flags().StringVar(&minSeverity, "min-severity", "", "minimum severity (low|medium|high|critical)")
	cmd.Flags().StringVar(&since, "since", "", "window start (RFC3339, 1h/5m, or 7d)")
	cmd.Flags().StringVar(&until, "until", "", "window end (RFC3339)")
	cmd.Flags().StringSliceVar(&redactKeys, "redact-key", nil, "metadata key to drop (repeatable)")
	cmd.Flags().StringSliceVar(&redactPats, "redact-pattern", nil, "regex redacted from metadata values + violation messages (repeatable)")
	cmd.Flags().BoolVar(&redactUser, "redact-user", false, "blank the user field on export")
	cmd.Flags().StringVar(&redactRepl, "redact-replacement", "", "replacement string for matched patterns (default ***)")
	return cmd
}

type exportOpts struct {
	format, outputFile                                string
	policyID, user, resourceType, action, minSeverity string
	since, until                                      string
	redactKeys, redactPats                            []string
	redactUser                                        bool
	redactRepl                                        string
}

func runExport(ctx context.Context, stdout io.Writer, g *globals, o exportOpts) error {
	format, err := intaudit.ParseFormat(o.format)
	if err != nil {
		return err
	}
	// The persistent -o/--output is meaningless for export; reject a
	// non-default value so a confused invocation fails loudly rather
	// than silently ignoring it.
	if g.Output != FormatTable {
		return fmt.Errorf("audit: `export` uses --format, not -o/--output (got -o %q)", g.Output)
	}

	var redaction *intaudit.RedactionConfig
	if len(o.redactKeys) > 0 || len(o.redactPats) > 0 || o.redactUser || o.redactRepl != "" {
		redaction, err = intaudit.NewRedactionConfig(intaudit.RedactionConfigInput{
			RedactMetadataKeys: o.redactKeys,
			RedactPatterns:     o.redactPats,
			RedactUser:         o.redactUser,
			Replacement:        o.redactRepl,
		})
		if err != nil {
			return fmt.Errorf("redaction config: %w", err)
		}
	}

	now := time.Now().UTC()
	sinceT, err := parseSince(o.since, now)
	if err != nil {
		return fmt.Errorf("--since: %w", err)
	}
	untilT, err := parseSince(o.until, now)
	if err != nil {
		return fmt.Errorf("--until: %w", err)
	}

	dst := stdout
	if o.outputFile != "" {
		f, ferr := os.Create(o.outputFile) //nolint:gosec // operator-supplied path
		if ferr != nil {
			return fmt.Errorf("create %s: %w", o.outputFile, ferr)
		}
		defer func() { _ = f.Close() }()
		dst = f
	}

	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()
	ctx = authContext(ctx, g.APIKey)

	exp, err := intaudit.NewExporter(dst, format, redaction)
	if err != nil {
		return err
	}
	if err := exp.Begin(); err != nil {
		return err
	}

	cursor := ""
	for {
		req := &v1.GetAuditLogRequest{
			PolicyId:     o.policyID,
			User:         o.user,
			ResourceType: o.resourceType,
			Action:       o.action,
			MinSeverity:  o.minSeverity,
			Since:        tsOrNil(sinceT),
			Until:        tsOrNil(untilT),
			Cursor:       cursor,
		}
		resp, qerr := client.GetAuditLog(ctx, req)
		if qerr != nil {
			return fmt.Errorf("GetAuditLog: %w", qerr)
		}
		for _, pe := range resp.GetEntries() {
			if werr := exp.WriteEntry(protoToAuditEntry(pe)); werr != nil {
				return werr
			}
		}
		cursor = resp.GetNextCursor()
		if cursor == "" {
			break
		}
	}
	return exp.End()
}

// protoToAuditEntry converts a wire v1.AuditEntry to the domain
// audit.AuditEntry so the existing task-2 RedactionConfig + the
// task-15 formatters operate on one type (reverse of the task-12
// auditEntryToProto). Unknown enum strings fall back to the zero
// value — export is best-effort display, not validation.
func protoToAuditEntry(p *v1.AuditEntry) intaudit.AuditEntry {
	sev, _ := intaudit.ParseSeverity(p.GetSeverity())
	mode, _ := intaudit.ParseEnforcementMode(p.GetEnforcementMode())
	ptype, _ := intaudit.ParsePolicyType(p.GetPolicyType())
	e := intaudit.AuditEntry{
		ID:              p.GetId(),
		PolicyID:        p.GetPolicyId(),
		PolicyName:      p.GetPolicyName(),
		PolicyType:      ptype,
		ResourceType:    p.GetResourceType(),
		Allowed:         p.GetAllowed(),
		Duration:        time.Duration(p.GetDurationNs()),
		EnforcementMode: mode,
		Severity:        sev,
		User:            p.GetUser(),
		Action:          p.GetAction(),
		Metadata:        p.GetMetadata(),
	}
	if ts := p.GetTimestamp(); ts != nil {
		e.Timestamp = ts.AsTime()
	}
	for _, pv := range p.GetViolations() {
		vsev, _ := intaudit.ParseSeverity(pv.GetSeverity())
		e.Violations = append(e.Violations, intaudit.Violation{
			Rule:        pv.GetRule(),
			Message:     pv.GetMessage(),
			Severity:    vsev,
			Path:        pv.GetPath(),
			Expected:    pv.GetExpected(),
			Actual:      pv.GetActual(),
			Remediation: pv.GetRemediation(),
		})
	}
	return e
}
