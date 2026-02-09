// Package output provides output formatting utilities for CLI commands.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	clierrors "github.com/shawnbutts/keystone-core/internal/cli/errors"
	"gopkg.in/yaml.v3"
)

// Format represents supported output formats.
type Format string

// FormatText constants define the output formats.
const (
	FormatText  Format = "text"
	FormatJSON  Format = "json"
	FormatYAML  Format = "yaml"
	FormatTable Format = "table"
)

// Table represents tabular output data.
type Table struct {
	Headers []string
	Rows    [][]string
}

// ParseFormat validates and normalizes a format string.
func ParseFormat(format string) (Format, error) {
	switch Format(strings.ToLower(format)) {
	case FormatText, FormatJSON, FormatYAML, FormatTable:
		return Format(strings.ToLower(format)), nil
	default:
		return "", clierrors.New(clierrors.KindInvalidArgument, fmt.Sprintf("unknown output format: %s", format))
	}
}

// Write renders output based on the provided format.
func Write(writer io.Writer, format Format, value any, table *Table, textRenderer func(io.Writer) error) error {
	switch format {
	case FormatJSON:
		return WriteJSON(writer, value)
	case FormatYAML:
		return WriteYAML(writer, value)
	case FormatTable:
		if table == nil {
			return fmt.Errorf("table output requires table data")
		}
		return WriteTable(writer, table)
	case FormatText:
		if textRenderer == nil {
			return clierrors.New(clierrors.KindInvalidArgument, "text output requires renderer")
		}
		return textRenderer(writer)
	default:
		return clierrors.New(clierrors.KindInvalidArgument, fmt.Sprintf("unsupported output format: %s", format))
	}
}

// WriteJSON writes JSON output with indentation.
func WriteJSON(writer io.Writer, value any) error {
	enc := json.NewEncoder(writer)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

// WriteYAML writes YAML output.
func WriteYAML(writer io.Writer, value any) error {
	data, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

// WriteTable renders a table using tabwriter.
func WriteTable(writer io.Writer, table *Table) error {
	tw := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)

	if len(table.Headers) > 0 {
		_, _ = fmt.Fprintln(tw, strings.Join(table.Headers, "\t"))
	}
	for _, row := range table.Rows {
		_, _ = fmt.Fprintln(tw, strings.Join(row, "\t"))
	}

	return tw.Flush()
}
