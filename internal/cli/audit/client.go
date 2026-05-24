// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// dialGRPC is the production gRPC dialer (insecure; CLI mTLS is a
// shared v0.x ROADMAP carry-over). API-key auth via metadata.
func dialGRPC(_ context.Context, target, _ string) (v1.PolicyServiceClient, io.Closer, error) {
	if target == "" {
		return nil, nil, fmt.Errorf("audit: --server is required")
	}
	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("audit: dial %s: %w", target, err)
	}
	return v1.NewPolicyServiceClient(conn), conn, nil
}

func authContext(ctx context.Context, apiKey string) context.Context {
	if apiKey == "" {
		apiKey = os.Getenv("KSCORE_API_KEY")
	}
	if apiKey == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+apiKey)
}

func validateOutput(format string) error {
	switch format {
	case FormatTable, FormatJSON:
		return nil
	}
	return fmt.Errorf("audit: invalid --output %q (want %q or %q)", format, FormatTable, FormatJSON)
}

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

// parseSince: RFC3339 | Go duration | `<n>d` days. Empty → zero.
func parseSince(value string, now time.Time) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	if strings.HasSuffix(value, "d") {
		if n, err := strconv.Atoi(strings.TrimSuffix(value, "d")); err == nil && n >= 0 {
			return now.Add(-time.Duration(n) * 24 * time.Hour), nil
		}
	}
	if d, err := time.ParseDuration(value); err == nil {
		return now.Add(-d), nil
	}
	return time.Time{}, fmt.Errorf("invalid time %q (want RFC3339, a Go duration like 1h/5m, or a day count like 7d)", value)
}

func tsOrNil(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

func fmtTS(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return "-"
	}
	return ts.AsTime().UTC().Format(time.RFC3339)
}
