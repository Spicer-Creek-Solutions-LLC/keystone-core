// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func validateOutput(format string) error {
	switch format {
	case FormatTable, FormatJSON:
		return nil
	}
	return fmt.Errorf("policy: invalid --output %q (want %q or %q)", format, FormatTable, FormatJSON)
}

// writeJSON renders a protobuf message as indented JSON via
// protojson so timestamps / maps / enums round-trip per the proto
// spec (matches the kscore-events convention).
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

// parseSince accepts an RFC3339 timestamp, a Go duration (1h, 5m),
// or a `<n>d` day-suffixed duration (30d, 7d) — the §4.12
// acceptance criteria use `--since 30d` / `--since 7d` which Go's
// time.ParseDuration can't handle (max unit is hours). Empty →
// zero time.
func parseSince(value string, now time.Time) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	if d, ok := parseDayDuration(value); ok {
		return now.Add(-d), nil
	}
	if d, err := time.ParseDuration(value); err == nil {
		return now.Add(-d), nil
	}
	return time.Time{}, fmt.Errorf("invalid time %q (want RFC3339, a Go duration like 1h/5m, or a day count like 30d)", value)
}

// parseDayDuration parses a `<n>d` string into a duration of n*24h.
func parseDayDuration(value string) (time.Duration, bool) {
	if !strings.HasSuffix(value, "d") {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSuffix(value, "d"))
	if err != nil || n < 0 {
		return 0, false
	}
	return time.Duration(n) * 24 * time.Hour, true
}

// table is the shared tabwriter helper (identical shape to the
// kscore-events / kscore-secrets table renderer).
type table struct {
	w *tabwriter.Writer
}

func newTable(w io.Writer) *table {
	return &table{w: tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)}
}

func (t *table) header(cols ...string) { fmt.Fprintln(t.w, strings.Join(cols, "\t")) }
func (t *table) row(cells ...string)   { fmt.Fprintln(t.w, strings.Join(cells, "\t")) }
func (t *table) flush() error          { return t.w.Flush() }
