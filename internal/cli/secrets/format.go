// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// MaskedSecretValue is the literal rendered in place of secret data
// values in table output. Matches the broker-side `secrets.MaskedValue`
// constant so operators see the same `***` consistently across logs +
// audit + CLI output.
const MaskedSecretValue = "***"

// validateOutput rejects unknown --output values up-front so each
// subcommand can fail fast.
func validateOutput(format string) error {
	switch format {
	case FormatTable, FormatJSON:
		return nil
	default:
		return fmt.Errorf("secrets: invalid --output %q (want %q or %q)", format, FormatTable, FormatJSON)
	}
}

// writeJSON renders a protobuf message as indented JSON. Uses
// protojson so timestamps + maps + enums round-trip per the proto
// spec.
func writeJSON(w io.Writer, msg proto.Message) error {
	opts := protojson.MarshalOptions{
		Multiline:       true,
		Indent:          "  ",
		EmitUnpopulated: false,
	}
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

// table is a minimal tabwriter helper.
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

// formatLabels renders a `map[string]string` in stable `k=v,k=v`
// form for table output. Empty maps render as "—".
func formatLabels(m map[string]string) string {
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

// maskedDataLines renders the data map as `k=***` lines (table form),
// hiding cleartext. Output is sorted for deterministic display.
func maskedDataLines(data map[string]string) string {
	if len(data) == 0 {
		return ""
	}
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		lines = append(lines, fmt.Sprintf("%s=%s", k, MaskedSecretValue))
	}
	return strings.Join(lines, "\n")
}

// cleartextDataLines renders the data map verbatim (`k=v` lines)
// for `--show-cleartext` mode.
func cleartextDataLines(data map[string]string) string {
	if len(data) == 0 {
		return ""
	}
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		lines = append(lines, fmt.Sprintf("%s=%s", k, data[k]))
	}
	return strings.Join(lines, "\n")
}

// parseKeyVal parses a `--data k=v` / `--label k=v` flag entry.
// Empty key rejected; empty value allowed (some callers legitimately
// want an empty value).
func parseKeyVal(entry string) (string, string, error) {
	idx := strings.IndexByte(entry, '=')
	if idx <= 0 {
		return "", "", fmt.Errorf("invalid --data / --label entry %q (want key=value)", entry)
	}
	return entry[:idx], entry[idx+1:], nil
}
