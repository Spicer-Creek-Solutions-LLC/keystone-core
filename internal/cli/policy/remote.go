package policy

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// ---- list -----------------------------------------------------------------

func listCmd(g *globals) *cobra.Command {
	var (
		policySets bool
		pageSize   int
		pageToken  string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List registered policies (or policy sets with --policy-sets)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd.Context(), cmd.OutOrStdout(), g, policySets, pageSize, pageToken)
		},
	}
	cmd.Flags().BoolVar(&policySets, "policy-sets", false, "list policy sets instead of policies")
	cmd.Flags().IntVar(&pageSize, "page-size", 50, "page size")
	cmd.Flags().StringVar(&pageToken, "page-token", "", "pagination token from a prior page")
	return cmd
}

func runList(ctx context.Context, out io.Writer, g *globals, sets bool, size int, token string) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}
	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()
	ctx = authContext(ctx, g.APIKey)

	if sets {
		resp, err := client.ListPolicySets(ctx, &v1.ListPolicySetsRequest{
			PageSize: int32(size), PageToken: token, //nolint:gosec // operator-supplied
		})
		if err != nil {
			return fmt.Errorf("ListPolicySets: %w", err)
		}
		if g.Output == FormatJSON {
			return writeJSON(out, resp)
		}
		t := newTable(out)
		t.header("ID", "NAME", "POLICIES", "ENABLED")
		for _, ps := range resp.GetPolicySets() {
			t.row(ps.GetId(), ps.GetName(),
				fmt.Sprintf("%d", len(ps.GetPolicyIds())), fmt.Sprintf("%t", ps.GetEnabled()))
		}
		fmt.Fprintf(out, "total: %d  next: %q\n", resp.GetTotalCount(), resp.GetNextPageToken())
		return t.flush()
	}

	resp, err := client.ListPolicies(ctx, &v1.ListPoliciesRequest{
		PageSize: int32(size), PageToken: token, //nolint:gosec // operator-supplied
	})
	if err != nil {
		return fmt.Errorf("ListPolicies: %w", err)
	}
	if g.Output == FormatJSON {
		return writeJSON(out, resp)
	}
	t := newTable(out)
	t.header("ID", "NAME", "TYPE", "CATEGORY", "SEVERITY", "MODE", "ENABLED")
	for _, p := range resp.GetPolicies() {
		t.row(p.GetId(), p.GetName(), p.GetType(), p.GetCategory(),
			p.GetSeverity(), p.GetEnforcementMode(), fmt.Sprintf("%t", p.GetEnabled()))
	}
	if err := t.flush(); err != nil {
		return err
	}
	fmt.Fprintf(out, "total: %d  next: %q\n", resp.GetTotalCount(), resp.GetNextPageToken())
	return nil
}

// ---- show -----------------------------------------------------------------

func showCmd(g *globals) *cobra.Command {
	var policySet bool
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show a policy (or a policy set with --policy-set) by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShow(cmd.Context(), cmd.OutOrStdout(), g, args[0], policySet)
		},
	}
	cmd.Flags().BoolVar(&policySet, "policy-set", false, "treat <id> as a policy-set ID")
	return cmd
}

func runShow(ctx context.Context, out io.Writer, g *globals, id string, set bool) error {
	if err := validateOutput(g.Output); err != nil {
		return err
	}
	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()
	ctx = authContext(ctx, g.APIKey)

	if set {
		resp, err := client.GetPolicySet(ctx, &v1.GetPolicySetRequest{Id: id})
		if err != nil {
			return fmt.Errorf("GetPolicySet: %w", err)
		}
		return writeJSON(out, resp)
	}
	resp, err := client.GetPolicy(ctx, &v1.GetPolicyRequest{Id: id})
	if err != nil {
		return fmt.Errorf("GetPolicy: %w", err)
	}
	return writeJSON(out, resp)
}

// ---- compliance -----------------------------------------------------------

func complianceCmd(g *globals) *cobra.Command {
	var (
		framework string
		since     string
		until     string
		bucket    string
		topN      int
	)
	cmd := &cobra.Command{
		Use:   "compliance",
		Short: "Compliance report over an audit window",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCompliance(cmd.Context(), cmd.OutOrStdout(), g, framework, since, until, bucket, topN)
		},
	}
	cmd.Flags().StringVar(&framework, "framework", "", "framework filter (cis|soc2|nist-800-53|hipaa|pci-dss|gdpr|iso-27001|custom)")
	cmd.Flags().StringVar(&since, "since", "", "window start (RFC3339, 1h/5m, or 30d)")
	cmd.Flags().StringVar(&until, "until", "", "window end (RFC3339, default now)")
	cmd.Flags().StringVar(&bucket, "bucket", "", "trend bucket interval (Go duration, e.g. 24h)")
	cmd.Flags().IntVar(&topN, "top-n", 0, "max top-violation entries (default 10)")
	return cmd
}

func runCompliance(ctx context.Context, out io.Writer, g *globals, framework, since, until, bucket string, topN int) error {
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
	req := &v1.GetComplianceReportRequest{Framework: framework, TopN: int32(topN)} //nolint:gosec // operator-supplied
	if !sinceT.IsZero() {
		req.Since = timestamppb.New(sinceT)
	}
	if !untilT.IsZero() {
		req.Until = timestamppb.New(untilT)
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
	fmt.Fprintf(out, "period:        %s .. %s\n",
		fmtTS(resp.GetPeriodStart()), fmtTS(resp.GetPeriodEnd()))
	fmt.Fprintf(out, "evaluations:   %d (compliant %d / non-compliant %d)\n",
		resp.GetTotalEvaluations(), resp.GetCompliantEvaluations(), resp.GetNonCompliantEvaluations())
	fmt.Fprintf(out, "rate:          %.4f\n", resp.GetComplianceRate())
	if len(resp.GetTopViolations()) > 0 {
		t := newTable(out)
		t.header("TOP POLICY", "DENIALS")
		for _, tv := range resp.GetTopViolations() {
			t.row(tv.GetPolicyId(), fmt.Sprintf("%d", tv.GetCount()))
		}
		if err := t.flush(); err != nil {
			return err
		}
	}
	return nil
}

// ---- violations -----------------------------------------------------------

func violationsCmd(g *globals) *cobra.Command {
	var (
		policyID     string
		resourceType string
		user         string
		severity     string
		since        string
		until        string
		limit        int
		cursor       string
	)
	cmd := &cobra.Command{
		Use:   "violations",
		Short: "List denied audit entries (policy violations)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runViolations(cmd.Context(), cmd.OutOrStdout(), g,
				policyID, resourceType, user, severity, since, until, limit, cursor)
		},
	}
	cmd.Flags().StringVar(&policyID, "policy-id", "", "filter by policy ID")
	cmd.Flags().StringVar(&resourceType, "resource-type", "", "filter by resource type")
	cmd.Flags().StringVar(&user, "user", "", "filter by user")
	cmd.Flags().StringVar(&severity, "severity", "", "minimum severity (low|medium|high|critical)")
	cmd.Flags().StringVar(&since, "since", "", "window start (RFC3339, 1h/5m, or 7d)")
	cmd.Flags().StringVar(&until, "until", "", "window end (RFC3339)")
	cmd.Flags().IntVar(&limit, "limit", 50, "page size")
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor")
	return cmd
}

func runViolations(ctx context.Context, out io.Writer, g *globals, policyID, resourceType, user, severity, since, until string, limit int, cursor string) error {
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
	req := &v1.ListViolationsRequest{
		PolicyId:     policyID,
		ResourceType: resourceType,
		User:         user,
		Limit:        int32(limit), //nolint:gosec // operator-supplied
		Cursor:       cursor,
	}
	if !sinceT.IsZero() {
		req.Since = timestamppb.New(sinceT)
	}
	if !untilT.IsZero() {
		req.Until = timestamppb.New(untilT)
	}

	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.ListViolations(authContext(ctx, g.APIKey), req)
	if err != nil {
		return fmt.Errorf("ListViolations: %w", err)
	}
	entries := resp.GetEntries()
	// --severity is a client-side floor (the RPC filters denied-only;
	// min-severity isn't a ListViolations field, so apply it here).
	if severity != "" {
		entries = filterMinSeverity(entries, severity)
	}
	if g.Output == FormatJSON {
		return writeJSON(out, &v1.ListViolationsResponse{Entries: entries, NextCursor: resp.GetNextCursor()})
	}
	t := newTable(out)
	t.header("TIME", "POLICY", "SEVERITY", "USER", "ACTION", "RESOURCE")
	for _, e := range entries {
		t.row(fmtTS(e.GetTimestamp()), e.GetPolicyId(), e.GetSeverity(),
			e.GetUser(), e.GetAction(), e.GetResourceType())
	}
	if err := t.flush(); err != nil {
		return err
	}
	if c := resp.GetNextCursor(); c != "" {
		fmt.Fprintf(out, "next cursor: %s\n", c)
	}
	return nil
}

var severityRank = map[string]int{"low": 1, "medium": 2, "high": 3, "critical": 4}

func filterMinSeverity(entries []*v1.AuditEntry, min string) []*v1.AuditEntry {
	floor := severityRank[min]
	if floor == 0 {
		return entries
	}
	out := make([]*v1.AuditEntry, 0, len(entries))
	for _, e := range entries {
		if severityRank[e.GetSeverity()] >= floor {
			out = append(out, e)
		}
	}
	return out
}

func fmtTS(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return "-"
	}
	return ts.AsTime().UTC().Format(time.RFC3339)
}
