package audit

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Format is an audit-export wire format per PROJECT-DETAILS §4.12.
type Format string

const (
	// FormatJSON emits a single JSON array of entries.
	FormatJSON Format = "json"
	// FormatJSONL emits one JSON object per line (jq / fluent
	// friendly; the streaming default).
	FormatJSONL Format = "jsonl"
	// FormatCSV emits a header row + one flat row per entry.
	FormatCSV Format = "csv"
)

// ParseFormat accepts the canonical lowercase names (whitespace
// trimmed, case-folded).
func ParseFormat(s string) (Format, error) {
	switch f := Format(strings.ToLower(strings.TrimSpace(s))); f {
	case FormatJSON, FormatJSONL, FormatCSV:
		return f, nil
	default:
		return "", fmt.Errorf("audit: invalid export format %q (want json | jsonl | csv)", s)
	}
}

// csvColumns is the stable CSV header. Scalars are flat columns;
// violations collapse to a count and metadata to one compact-JSON
// cell (operators picking CSV want spreadsheet-friendly scalars —
// the full nested detail is in the json / jsonl formats).
var csvColumns = []string{
	"id", "timestamp", "policy_id", "policy_name", "policy_type",
	"resource_type", "allowed", "duration_ns", "enforcement_mode",
	"severity", "user", "action", "violations_count", "metadata",
}

// Exporter streams audit entries to an io.Writer in a chosen
// Format. Lifecycle: Begin → WriteEntry* → End. Streaming (no
// whole-log buffering) so a large audit table can be exported
// page-by-page from a cursor-paginated source.
//
// Redaction is intrinsic to export per §4.12 ("Applied on
// export"): a non-nil, non-noop RedactionConfig is applied to every
// entry BEFORE formatting, so the export boundary is the guaranteed
// redaction point regardless of caller.
type Exporter struct {
	w         io.Writer
	format    Format
	redaction *RedactionConfig

	csv   *csv.Writer
	count int // entries written (JSON comma handling)
	begun bool
	ended bool
}

// NewExporter validates format and returns an Exporter writing to
// w. redaction may be nil (no redaction).
func NewExporter(w io.Writer, format Format, redaction *RedactionConfig) (*Exporter, error) {
	if _, err := ParseFormat(string(format)); err != nil {
		return nil, err
	}
	e := &Exporter{w: w, format: format, redaction: redaction}
	if format == FormatCSV {
		e.csv = csv.NewWriter(w)
	}
	return e, nil
}

// Begin writes any format preamble (JSON `[`, CSV header row).
// Idempotent-safe to call once before the first WriteEntry.
func (e *Exporter) Begin() error {
	if e.begun {
		return nil
	}
	e.begun = true
	switch e.format {
	case FormatJSON:
		_, err := io.WriteString(e.w, "[")
		return err
	case FormatCSV:
		return e.csv.Write(csvColumns)
	}
	return nil
}

// WriteEntry redacts (if configured) then formats one entry.
func (e *Exporter) WriteEntry(entry AuditEntry) error {
	if !e.begun {
		if err := e.Begin(); err != nil {
			return err
		}
	}
	if e.redaction != nil {
		entry = e.redaction.Apply(entry)
	}
	switch e.format {
	case FormatJSON:
		if e.count > 0 {
			if _, err := io.WriteString(e.w, ","); err != nil {
				return err
			}
		}
		b, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("audit: export marshal entry %s: %w", entry.ID, err)
		}
		if _, err := e.w.Write(b); err != nil {
			return err
		}
	case FormatJSONL:
		b, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("audit: export marshal entry %s: %w", entry.ID, err)
		}
		if _, err := e.w.Write(b); err != nil {
			return err
		}
		if _, err := io.WriteString(e.w, "\n"); err != nil {
			return err
		}
	case FormatCSV:
		if err := e.csv.Write(e.csvRow(entry)); err != nil {
			return err
		}
	}
	e.count++
	return nil
}

// End writes any format epilogue (JSON `]`, CSV flush).
func (e *Exporter) End() error {
	if e.ended {
		return nil
	}
	e.ended = true
	if !e.begun {
		if err := e.Begin(); err != nil {
			return err
		}
	}
	switch e.format {
	case FormatJSON:
		_, err := io.WriteString(e.w, "]\n")
		return err
	case FormatCSV:
		e.csv.Flush()
		return e.csv.Error()
	}
	return nil
}

// csvRow flattens an entry to the stable column set. metadata is a
// compact deterministic JSON object (encoding/json sorts map keys);
// violations collapse to a count.
func (e *Exporter) csvRow(entry AuditEntry) []string {
	meta := "{}"
	if len(entry.Metadata) > 0 {
		if b, err := json.Marshal(entry.Metadata); err == nil {
			meta = string(b)
		}
	}
	return []string{
		entry.ID,
		entry.Timestamp.UTC().Format(time.RFC3339Nano),
		entry.PolicyID,
		entry.PolicyName,
		entry.PolicyType.String(),
		entry.ResourceType,
		strconv.FormatBool(entry.Allowed),
		strconv.FormatInt(entry.Duration.Nanoseconds(), 10),
		entry.EnforcementMode.String(),
		entry.Severity.String(),
		entry.User,
		entry.Action,
		strconv.Itoa(len(entry.Violations)),
		meta,
	}
}

// Export is the batch convenience: Begin, WriteEntry for each, End.
// Callers exporting from a paginated source should drive the
// Exporter lifecycle directly so memory stays bounded.
func Export(w io.Writer, format Format, redaction *RedactionConfig, entries []AuditEntry) error {
	exp, err := NewExporter(w, format, redaction)
	if err != nil {
		return err
	}
	if err := exp.Begin(); err != nil {
		return err
	}
	for _, en := range entries {
		if err := exp.WriteEntry(en); err != nil {
			return err
		}
	}
	return exp.End()
}
