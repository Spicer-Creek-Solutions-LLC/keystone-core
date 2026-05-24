// SPDX-License-Identifier: Apache-2.0

// Package grafana hosts the build-time validation gate for the v1.0
// Grafana dashboards shipped under deploy/grafana/dashboards.
//
// The test in this package is the "promtool or equivalent" the Epic 17
// task-8 spec asks for. It does not require a running Prometheus or
// Grafana — it cross-checks the dashboard JSON against the kscore
// metric set declared in internal/metrics/metricdefs.go plus the
// expected_metrics.txt diff target.
package grafana

import (
	"bufio"
	"bytes"
	"embed"
	"encoding/json"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/metrics"
)

//go:embed dashboards/*.json
var dashboardsFS embed.FS

//go:embed expected_metrics.txt
var expectedMetricsRaw []byte

// dashboard is the minimal Grafana schema we validate against. We
// intentionally accept extra fields without erroring — Grafana's full
// schema is huge and most of it is irrelevant to "does this dashboard
// reference only metrics we ship?".
type dashboard struct {
	Title         string       `json:"title"`
	UID           string       `json:"uid"`
	SchemaVersion int          `json:"schemaVersion"`
	Tags          []string     `json:"tags"`
	Panels        []panel      `json:"panels"`
	Templating    templating   `json:"templating"`
	Time          timeRange    `json:"time"`
	Refresh       interface{}  `json:"refresh"`
}

type panel struct {
	ID      int      `json:"id"`
	Title   string   `json:"title"`
	Type    string   `json:"type"`
	Targets []target `json:"targets"`
	Panels  []panel  `json:"panels"` // Grafana row panels can nest
}

type target struct {
	Expr  string `json:"expr"`
	RefID string `json:"refId"`
}

type templating struct {
	List []templateVar `json:"list"`
}

type templateVar struct {
	Name  string      `json:"name"`
	Type  string      `json:"type"`
	Query interface{} `json:"query"` // sometimes string, sometimes object
}

type timeRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// metricNameRE captures the long-form names we care about. The token
// boundary on either side prevents matching kscore_agents_total inside
// kscore_agents_total_bucket (which is a histogram-bucket suffix that
// Prom synthesizes, not a name to validate).
var metricNameRE = regexp.MustCompile(`\b(kscore_[a-z0-9_]+|go_[a-z0-9_]+|process_[a-z0-9_]+)\b`)

func loadExpectedMetrics(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(expectedMetricsRaw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out[line] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read expected_metrics.txt: %v", err)
	}
	return out
}

func loadDashboards(t *testing.T) map[string]dashboard {
	t.Helper()
	entries, err := dashboardsFS.ReadDir("dashboards")
	if err != nil {
		t.Fatalf("read dashboards dir: %v", err)
	}
	out := map[string]dashboard{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := fs.ReadFile(dashboardsFS, path.Join("dashboards", e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		var d dashboard
		if err := json.Unmarshal(raw, &d); err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		out[e.Name()] = d
	}
	return out
}

func TestDashboards_Count(t *testing.T) {
	got := loadDashboards(t)
	if len(got) != 12 {
		t.Fatalf("dashboard count = %d, want 12 (epic 17 line 54)", len(got))
	}
}

func TestDashboards_RequiredFields(t *testing.T) {
	for name, d := range loadDashboards(t) {
		t.Run(name, func(t *testing.T) {
			if d.Title == "" {
				t.Errorf("title is empty")
			}
			if d.UID == "" {
				t.Errorf("uid is empty")
			}
			if d.SchemaVersion < 16 {
				t.Errorf("schemaVersion = %d, want >= 16 (modern Grafana)", d.SchemaVersion)
			}
			if len(d.Panels) == 0 {
				t.Errorf("panels is empty")
			}
			if len(d.Templating.List) == 0 {
				t.Errorf("templating.list is empty (datasource var required)")
			}
			if !hasTag(d.Tags, "kscore") {
				t.Errorf("missing kscore tag in %v", d.Tags)
			}
		})
	}
}

func TestDashboards_UIDsUnique(t *testing.T) {
	got := loadDashboards(t)
	seen := map[string]string{}
	for name, d := range got {
		if prev, ok := seen[d.UID]; ok {
			t.Errorf("uid %q used by both %s and %s", d.UID, prev, name)
		}
		seen[d.UID] = name
		if !strings.HasPrefix(d.UID, "kscore-") {
			t.Errorf("%s: uid %q lacks kscore- prefix", name, d.UID)
		}
	}
}

func TestDashboards_TemplatingHasDatasource(t *testing.T) {
	for name, d := range loadDashboards(t) {
		t.Run(name, func(t *testing.T) {
			if !hasTemplateVar(d.Templating.List, "datasource", "datasource") {
				t.Errorf("missing datasource templating variable")
			}
		})
	}
}

func TestDashboards_AtLeastOneQuery(t *testing.T) {
	for name, d := range loadDashboards(t) {
		t.Run(name, func(t *testing.T) {
			if extractAllQueries(d) == 0 {
				t.Errorf("no panel queries")
			}
		})
	}
}

func TestDashboards_ReferencedMetricsAreExpected(t *testing.T) {
	expected := loadExpectedMetrics(t)
	for name, d := range loadDashboards(t) {
		t.Run(name, func(t *testing.T) {
			for _, ref := range collectMetricNames(d) {
				if !expected[ref] {
					t.Errorf("references metric %q not in expected_metrics.txt", ref)
				}
			}
		})
	}
}

func TestExpectedMetrics_CoversMetricDefs(t *testing.T) {
	expected := loadExpectedMetrics(t)
	defs := allKscoreMetricDefs()
	for _, name := range defs {
		if !expected[name] {
			t.Errorf("metricdefs.go defines %q but expected_metrics.txt does not list it", name)
		}
	}
}

func TestExpectedMetrics_NoUnknownKscoreEntries(t *testing.T) {
	// Every kscore_* line in expected_metrics.txt must be backed by a
	// definition in metricdefs.go. Other prefixes (go_, process_) are
	// runtime collectors and not validated here.
	expected := loadExpectedMetrics(t)
	defs := map[string]bool{}
	for _, name := range allKscoreMetricDefs() {
		defs[name] = true
	}
	for name := range expected {
		if !strings.HasPrefix(name, "kscore_") {
			continue
		}
		if !defs[name] {
			t.Errorf("expected_metrics.txt lists kscore metric %q not in metricdefs.go", name)
		}
	}
}

// allKscoreMetricDefs returns every kscore_* metric the binary
// registers — the source of truth for the dashboard-vs-code diff.
func allKscoreMetricDefs() []string {
	defs := []metrics.MetricDef{
		metrics.DefAgentsTotal,
		metrics.DefCommandsExecutedTotal,
		metrics.DefCommandDurationSeconds,
		metrics.DefStateApplyTotal,
		metrics.DefStateDriftDetectedTotal,
		metrics.DefEventsEmittedTotal,
		metrics.DefSecretsAccessTotal,
		metrics.DefAuditEntriesTotal,
		metrics.DefClusterMembersTotal,
		metrics.DefClusterQuorum,
		metrics.DefClusterFailoverTotal,
		metrics.DefGRPCRequestDurationSeconds,
		metrics.DefHTTPRequestDurationSeconds,
		metrics.DefFilesCacheHitsTotal,
		metrics.DefFilesCacheMissesTotal,
		metrics.DefRatelimitRejectedTotal,
	}
	out := make([]string, 0, len(defs)+1)
	for _, d := range defs {
		out = append(out, d.Name)
	}
	// The cardinality self-metric is registered by NewRegistry, not as
	// a MetricDef constant.
	out = append(out, metrics.CardinalityMetricName)
	sort.Strings(out)
	return out
}

// extractAllQueries returns the count of non-empty panel queries
// across the dashboard, recursing through row panels.
func extractAllQueries(d dashboard) int {
	return countQueriesInPanels(d.Panels)
}

func countQueriesInPanels(panels []panel) int {
	n := 0
	for _, p := range panels {
		for _, t := range p.Targets {
			if strings.TrimSpace(t.Expr) != "" {
				n++
			}
		}
		n += countQueriesInPanels(p.Panels)
	}
	return n
}

// collectMetricNames extracts every kscore_/go_/process_ metric name
// referenced anywhere in the dashboard (panel queries + template var
// queries). Histogram and summary suffixes (_bucket / _count / _sum)
// are normalized back to the base metric name so expected_metrics.txt
// only needs to list base names — Prom synthesizes the suffix variants
// at scrape time.
func collectMetricNames(d dashboard) []string {
	seen := map[string]bool{}
	collectFromPanels(d.Panels, seen)
	for _, v := range d.Templating.List {
		if s, ok := v.Query.(string); ok {
			for _, m := range metricNameRE.FindAllString(s, -1) {
				seen[normalizeMetric(m)] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func collectFromPanels(panels []panel, seen map[string]bool) {
	for _, p := range panels {
		for _, t := range p.Targets {
			for _, m := range metricNameRE.FindAllString(t.Expr, -1) {
				seen[normalizeMetric(m)] = true
			}
		}
		collectFromPanels(p.Panels, seen)
	}
}

// normalizeMetric strips histogram / summary auto-suffixes so the
// validation gate matches kscore_http_request_duration_seconds_bucket
// against the base kscore_http_request_duration_seconds line in
// expected_metrics.txt.
func normalizeMetric(name string) string {
	for _, suffix := range []string{"_bucket", "_count", "_sum"} {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(name, suffix)
		}
	}
	return name
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

func hasTemplateVar(vars []templateVar, name, typ string) bool {
	for _, v := range vars {
		if v.Name == name && v.Type == typ {
			return true
		}
	}
	return false
}
