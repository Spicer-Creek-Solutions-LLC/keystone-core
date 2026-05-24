// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
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
// subcommand can fail fast.
func validateOutput(format string) error {
	switch format {
	case FormatTable, FormatJSON:
		return nil
	default:
		return fmt.Errorf("identity: invalid --output %q (want %q or %q)", format, FormatTable, FormatJSON)
	}
}

// writeJSON renders a protobuf message as indented JSON. Uses
// protojson so timestamps + maps + enums round-trip per the proto
// spec (vs. encoding/json which would butcher them).
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

// writeJSONAny renders an arbitrary value as indented JSON. Used
// for ad-hoc output that doesn't have a proto shape (e.g. the
// status command's flat snapshot).
func writeJSONAny(w io.Writer, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	_, err = io.WriteString(w, "\n")
	return err
}

// table is a minimal tabwriter helper. Caller provides the header
// + rows; we emit padded columns.
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

// formatTimestamp prints a proto-wrapped timestamp in RFC3339 form;
// nil and zero render as "—" so table columns stay aligned.
func formatTimestamp(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "—"
	}
	return t.Format(time.RFC3339)
}

// formatDuration renders a duration in operator-friendly form
// ("2h30m"), capped at three significant units for readability.
func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	if d > 24*time.Hour {
		days := int(d / (24 * time.Hour))
		rem := d - time.Duration(days)*24*time.Hour
		hours := int(rem / time.Hour)
		return fmt.Sprintf("%dd%dh", days, hours)
	}
	return d.Truncate(time.Minute).String()
}

// certInfoForCLI projects an x509.Certificate into the CACertInfo
// proto shape — same fields the gRPC GetCAInfo returns for the
// root, but computed client-side for the signing cert (which v0.1
// only ships as PEM via ExportCA SIGNING).
func certInfoForCLI(cert *x509.Certificate) *v1.CACertInfo {
	keyType := ""
	switch pk := cert.PublicKey.(type) {
	case *ecdsa.PublicKey:
		keyType = "ECDSA-" + pk.Curve.Params().Name
	case *rsa.PublicKey:
		keyType = fmt.Sprintf("RSA-%d", pk.N.BitLen())
	default:
		keyType = fmt.Sprintf("%T", pk)
	}
	return &v1.CACertInfo{
		Subject:   cert.Subject.String(),
		Serial:    cert.SerialNumber.Text(16),
		NotBefore: timestamppb.New(cert.NotBefore),
		NotAfter:  timestamppb.New(cert.NotAfter),
		KeyType:   keyType,
	}
}

// formatMetadata renders a map[string]string in stable
// "k=v,k=v" form for the table view. Empty maps render as "—".
func formatMetadata(m map[string]string) string {
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
