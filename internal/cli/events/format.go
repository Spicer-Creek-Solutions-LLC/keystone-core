// SPDX-License-Identifier: Apache-2.0

package events

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// validateOutput rejects unknown --output values up-front so each
// subcommand can fail fast. Allows table or json for non-streaming
// subcommands; streaming subcommands accept jsonlines additionally.
func validateOutput(format string, allowJSONLines bool) error {
	switch format {
	case FormatTable, FormatJSON:
		return nil
	case FormatJSONLines:
		if allowJSONLines {
			return nil
		}
	}
	want := fmt.Sprintf("%q or %q", FormatTable, FormatJSON)
	if allowJSONLines {
		want = fmt.Sprintf("%q, %q, or %q", FormatTable, FormatJSON, FormatJSONLines)
	}
	return fmt.Errorf("events: invalid --output %q (want %s)", format, want)
}

// writeJSON renders a protobuf message as indented JSON. Uses
// protojson so timestamps + maps + enums round-trip per the proto
// spec.
func writeJSON(w io.Writer, msg proto.Message) error {
	opts := protojson.MarshalOptions{Multiline: true, Indent: "  ", EmitUnpopulated: false}
	data, err := opts.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	_, err = io.WriteString(w, "\n")
	return err
}

// writeEventJSONLine emits a single event as one JSON object plus a
// trailing newline — the JSON-lines format jq / awk / fluent pipelines
// expect.
func writeEventJSONLine(w io.Writer, e *v1.Event) error {
	opts := protojson.MarshalOptions{EmitUnpopulated: false}
	data, err := opts.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	_, err = io.WriteString(w, "\n")
	return err
}

// writeEventCompactLine emits one fixed-column line per event for
// the `watch` subcommand's tail-friendly output. Columns: time |
// severity | type | source | tags-summary. Newline terminated.
func writeEventCompactLine(w io.Writer, e *v1.Event) error {
	tags := formatTagsCompact(e.GetTags())
	_, err := fmt.Fprintf(w, "%s  %-8s  %-30s  %-20s  %s\n",
		formatProtoTimestamp(e.GetTime()),
		severityNameFromEnum(e.GetSeverity()),
		e.GetType(),
		e.GetSource(),
		tags,
	)
	return err
}

// table is a minimal tabwriter helper, identical to the kscore-secrets
// version so output style stays consistent across the binary suite.
type table struct {
	w      *tabwriter.Writer
	closed bool
}

func newTable(w io.Writer) *table {
	return &table{w: tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)}
}

func (t *table) header(cols ...string) {
	_, _ = fmt.Fprintln(t.w, strings.Join(cols, "\t"))
}

func (t *table) row(cells ...string) {
	_, _ = fmt.Fprintln(t.w, strings.Join(cells, "\t"))
}

func (t *table) flush() error {
	if t.closed {
		return nil
	}
	t.closed = true
	return t.w.Flush()
}

// formatProtoTimestamp prints a protobuf timestamp in RFC3339 form;
// nil renders as "—" so columns stay aligned.
func formatProtoTimestamp(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return "—"
	}
	t := ts.AsTime()
	if t.IsZero() {
		return "—"
	}
	return t.Format(time.RFC3339)
}

// formatTagsCompact renders a tag map as `k=v,k=v` in sort-stable
// order. Empty maps render as "—".
func formatTagsCompact(m map[string]string) string {
	if len(m) == 0 {
		return "—"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+m[k])
	}
	return strings.Join(parts, ",")
}

// severityNameFromEnum converts the proto enum back to the canonical
// lowercase name. Mirrors `events.Severity.String()` for the table /
// compact paths. Unknown values render as "unknown" so column width
// stays predictable.
func severityNameFromEnum(s v1.EventSeverity) string {
	switch s {
	case v1.EventSeverity_EVENT_SEVERITY_DEBUG:
		return "debug"
	case v1.EventSeverity_EVENT_SEVERITY_INFO:
		return "info"
	case v1.EventSeverity_EVENT_SEVERITY_WARN:
		return "warn"
	case v1.EventSeverity_EVENT_SEVERITY_ERROR:
		return "error"
	case v1.EventSeverity_EVENT_SEVERITY_CRITICAL:
		return "critical"
	default:
		return "unknown"
	}
}

// severityEnumFromName converts a canonical lowercase severity name
// to the proto enum. Aliases (`warning` → warn, `fatal` → critical)
// match `events.ParseSeverity`. Returns UNSPECIFIED + error for
// unknown names.
func severityEnumFromName(name string) (v1.EventSeverity, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "":
		return v1.EventSeverity_EVENT_SEVERITY_UNSPECIFIED, nil
	case "debug":
		return v1.EventSeverity_EVENT_SEVERITY_DEBUG, nil
	case "info":
		return v1.EventSeverity_EVENT_SEVERITY_INFO, nil
	case "warn", "warning":
		return v1.EventSeverity_EVENT_SEVERITY_WARN, nil
	case "error":
		return v1.EventSeverity_EVENT_SEVERITY_ERROR, nil
	case "critical", "fatal":
		return v1.EventSeverity_EVENT_SEVERITY_CRITICAL, nil
	}
	return v1.EventSeverity_EVENT_SEVERITY_UNSPECIFIED, fmt.Errorf("invalid severity %q (want debug|info|warn|error|critical)", name)
}

// parseTagFlag splits a `--tag k=v` entry. Empty key is rejected;
// empty value is allowed (operators may legitimately want a "tag
// present with empty value" predicate).
func parseTagFlag(entry string) (string, string, error) {
	idx := strings.IndexByte(entry, '=')
	if idx <= 0 {
		return "", "", fmt.Errorf("invalid --tag entry %q (want key=value)", entry)
	}
	return entry[:idx], entry[idx+1:], nil
}

// parseTagFlags converts a slice of `--tag k=v` entries to the
// `map[string]string` filter shape. Returns an error on the first
// malformed entry.
func parseTagFlags(entries []string) (map[string]string, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		k, v, err := parseTagFlag(e)
		if err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}

// parseRelativeDuration accepts either an RFC3339 timestamp ("since"
// = absolute) OR a Go duration string ("1h", "5m"). Empty input
// returns the zero time. The relative form subtracts the duration
// from `now` (`now` injected for test determinism).
func parseRelativeDuration(value string, now time.Time) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	if d, err := time.ParseDuration(value); err == nil {
		return now.Add(-d), nil
	}
	return time.Time{}, fmt.Errorf("invalid time %q (want RFC3339 timestamp or duration like 1h / 5m)", value)
}

// parseDataFlag parses --data <json-or-@file>. `@file.json` reads
// the file; everything else is treated as inline JSON. Returns the
// raw decoded `map[string]any` for translation into proto Struct.
func parseDataFlag(value string, readFile func(string) ([]byte, error)) (map[string]any, error) {
	if value == "" {
		return nil, nil
	}
	var raw []byte
	if strings.HasPrefix(value, "@") {
		path := strings.TrimPrefix(value, "@")
		b, err := readFile(path)
		if err != nil {
			return nil, fmt.Errorf("read --data file %q: %w", path, err)
		}
		raw = b
	} else {
		raw = []byte(value)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse --data JSON: %w", err)
	}
	return out, nil
}
