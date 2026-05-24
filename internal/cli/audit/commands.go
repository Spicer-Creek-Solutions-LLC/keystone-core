// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// ---- log ------------------------------------------------------------------

func logCmd(g *globals) *cobra.Command {
	var (
		policyID     string
		user         string
		resourceType string
		action       string
		minSeverity  string
		since        string
		until        string
		limit        int
		cursor       string
	)
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Paginated audit-log query",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLog(cmd.Context(), cmd.OutOrStdout(), g, logOpts{
				policyID, user, resourceType, action, minSeverity, since, until, limit, cursor,
			})
		},
	}
	cmd.Flags().StringVar(&policyID, "policy-id", "", "filter by policy ID")
	cmd.Flags().StringVar(&user, "user", "", "filter by user")
	cmd.Flags().StringVar(&resourceType, "resource-type", "", "filter by resource type")
	cmd.Flags().StringVar(&action, "action", "", "filter by action")
	cmd.Flags().StringVar(&minSeverity, "min-severity", "", "minimum severity (low|medium|high|critical)")
	cmd.Flags().StringVar(&since, "since", "", "window start (RFC3339, 1h/5m, or 7d)")
	cmd.Flags().StringVar(&until, "until", "", "window end (RFC3339)")
	cmd.Flags().IntVar(&limit, "limit", 50, "page size")
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor from a prior page's next_cursor")
	return cmd
}

type logOpts struct {
	policyID, user, resourceType, action, minSeverity, since, until string
	limit                                                           int
	cursor                                                          string
}

func runLog(ctx context.Context, out io.Writer, g *globals, o logOpts) error {
	if err := validateOutput(g.Output); err != nil {
		return err
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
	req := &v1.GetAuditLogRequest{
		PolicyId:     o.policyID,
		User:         o.user,
		ResourceType: o.resourceType,
		Action:       o.action,
		MinSeverity:  o.minSeverity,
		Since:        tsOrNil(sinceT),
		Until:        tsOrNil(untilT),
		Limit:        int32(o.limit), //nolint:gosec // operator-supplied
		Cursor:       o.cursor,
	}
	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.GetAuditLog(authContext(ctx, g.APIKey), req)
	if err != nil {
		return fmt.Errorf("GetAuditLog: %w", err)
	}
	if g.Output == FormatJSON {
		return writeJSON(out, resp)
	}
	t := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(t, strings.Join([]string{"TIME", "POLICY", "ALLOWED", "SEVERITY", "USER", "ACTION", "RESOURCE"}, "\t"))
	for _, e := range resp.GetEntries() {
		fmt.Fprintln(t, strings.Join([]string{
			fmtTS(e.GetTimestamp()), e.GetPolicyId(), fmt.Sprintf("%t", e.GetAllowed()),
			e.GetSeverity(), e.GetUser(), e.GetAction(), e.GetResourceType(),
		}, "\t"))
	}
	if err := t.Flush(); err != nil {
		return err
	}
	if c := resp.GetNextCursor(); c != "" {
		fmt.Fprintf(out, "next cursor: %s\n", c)
	}
	return nil
}

// ---- report ---------------------------------------------------------------

func reportCmd(g *globals) *cobra.Command {
	var framework, since, until, bucket string
	var topN int
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Compliance report (rate, top violations, trend)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReport(cmd.Context(), cmd.OutOrStdout(), g, framework, since, until, bucket, topN, false)
		},
	}
	cmd.Flags().StringVar(&framework, "framework", "", "framework filter (cis|soc2|nist-800-53|hipaa|pci-dss|gdpr|iso-27001|custom)")
	cmd.Flags().StringVar(&since, "since", "", "window start (RFC3339, 1h/5m, or 30d)")
	cmd.Flags().StringVar(&until, "until", "", "window end (RFC3339, default now)")
	cmd.Flags().StringVar(&bucket, "bucket", "", "trend bucket interval (Go duration, e.g. 24h)")
	cmd.Flags().IntVar(&topN, "top-n", 0, "max top-violation entries (default 10)")
	return cmd
}

// ---- stats ----------------------------------------------------------------

func statsCmd(g *globals) *cobra.Command {
	var since, until string
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Headline audit counts over a window",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReport(cmd.Context(), cmd.OutOrStdout(), g, "", since, until, "", 0, true)
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "window start (RFC3339, 1h/5m, or 7d)")
	cmd.Flags().StringVar(&until, "until", "", "window end (RFC3339, default now)")
	return cmd
}

// runReport backs both `report` and `stats` (stats = the headline
// counts only, no top-violations table / trend).
func runReport(ctx context.Context, out io.Writer, g *globals, framework, since, until, bucket string, topN int, statsOnly bool) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}
	now := time.Now().UTC()
	sinceT, err := parseSince(since, now)
	if err != nil {
		return fmt.Errorf("--since: %w", err)
	}
	untilT, err := parseSince(until, now)
	if err != nil {
		return fmt.Errorf("--until: %w", err)
	}
	req := &v1.GetComplianceReportRequest{
		Framework: framework,
		Since:     tsOrNil(sinceT),
		Until:     tsOrNil(untilT),
		TopN:      int32(topN), //nolint:gosec // operator-supplied
	}
	if bucket != "" {
		d, derr := time.ParseDuration(bucket)
		if derr != nil {
			return fmt.Errorf("--bucket: %w", derr)
		}
		req.BucketIntervalNs = d.Nanoseconds()
	}
	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.GetComplianceReport(authContext(ctx, g.APIKey), req)
	if err != nil {
		return fmt.Errorf("GetComplianceReport: %w", err)
	}
	if g.Output == FormatJSON {
		return writeJSON(out, resp)
	}
	fmt.Fprintf(out, "period:      %s .. %s\n", fmtTS(resp.GetPeriodStart()), fmtTS(resp.GetPeriodEnd()))
	fmt.Fprintf(out, "evaluations: %d (compliant %d / non-compliant %d)\n",
		resp.GetTotalEvaluations(), resp.GetCompliantEvaluations(), resp.GetNonCompliantEvaluations())
	fmt.Fprintf(out, "rate:        %.4f\n", resp.GetComplianceRate())
	if bySev := resp.GetViolationsBySeverity(); len(bySev) > 0 {
		fmt.Fprintf(out, "by-severity: ")
		for _, s := range []string{"low", "medium", "high", "critical"} {
			if n, ok := bySev[s]; ok {
				fmt.Fprintf(out, "%s=%d ", s, n)
			}
		}
		fmt.Fprintln(out)
	}
	if statsOnly {
		return nil
	}
	if tv := resp.GetTopViolations(); len(tv) > 0 {
		t := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(t, "TOP POLICY\tDENIALS")
		for _, v := range tv {
			fmt.Fprintf(t, "%s\t%d\n", v.GetPolicyId(), v.GetCount())
		}
		if err := t.Flush(); err != nil {
			return err
		}
	}
	for _, tp := range resp.GetTrend() {
		fmt.Fprintf(out, "trend %s..%s  rate=%.4f (%d evals)\n",
			fmtTS(tp.GetStart()), fmtTS(tp.GetEnd()), tp.GetComplianceRate(), tp.GetTotalEvaluations())
	}
	return nil
}
