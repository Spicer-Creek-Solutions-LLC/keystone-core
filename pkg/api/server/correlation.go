package server

import (
	"context"
	"net/http"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"go.keystone-core.io/keystone-core/internal/logging"
)

// maxCorrelationIDLen bounds the inbound header / metadata value. The
// Epic 01 generator returns 32 hex chars; doubling that gives ops
// callers room to use friendlier IDs (UUIDs, trace IDs) without
// accepting unbounded strings that could blow up log lines.
const maxCorrelationIDLen = 128

// HTTPCorrelationMiddleware sits OUTERMOST in the chain — before CORS,
// before auth, before metrics — so every inbound request (health
// probes, 401-rejected calls, anything) carries a correlation ID into
// the log stream. The middleware:
//
//   - Reads the inbound logging.HTTPHeader value.
//   - If present and acceptable (printable ASCII, length-bounded),
//     uses it as-is so client-supplied IDs propagate through.
//   - Otherwise generates a fresh ID via logging.NewCorrelationID.
//   - Stamps the ID on ctx with logging.WithCorrelationID.
//   - Echoes the ID on the response so the caller can correlate from
//     their side without parsing the body.
func HTTPCorrelationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(logging.HTTPHeader)
		if !isAcceptableCorrelationID(id) {
			id = logging.NewCorrelationID()
		}
		// Echo BEFORE serving so any panic recovery / streaming handler
		// still surfaces the ID to the caller.
		w.Header().Set(logging.HTTPHeader, id)
		ctx := logging.WithCorrelationID(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// UnaryCorrelationInterceptor is the gRPC unary mirror of
// HTTPCorrelationMiddleware. Reads logging.GRPCMetadataKey from
// inbound metadata, generates a fresh ID when absent/malformed, and
// sets the same key on the response trailer so clients can correlate.
func UnaryCorrelationInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		id := extractGRPCCorrelationID(ctx)
		if !isAcceptableCorrelationID(id) {
			id = logging.NewCorrelationID()
		}
		// Trailer echoes the ID to clients without forcing the handler
		// to opt in. Failure to set a trailer is non-fatal — that path
		// happens on a malformed context that we can't do much about
		// from here.
		_ = grpc.SetTrailer(ctx, metadata.Pairs(logging.GRPCMetadataKey, id))
		ctx = logging.WithCorrelationID(ctx, id)
		return handler(ctx, req)
	}
}

// StreamCorrelationInterceptor is the gRPC streaming mirror. Same
// extract-or-generate logic as the unary form; the ID lives on a
// wrapped ServerStream's context so handlers see it.
func StreamCorrelationInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		id := extractGRPCCorrelationID(ctx)
		if !isAcceptableCorrelationID(id) {
			id = logging.NewCorrelationID()
		}
		ss.SetTrailer(metadata.Pairs(logging.GRPCMetadataKey, id))
		return handler(srv, &correlatedStream{ServerStream: ss, ctx: logging.WithCorrelationID(ctx, id)})
	}
}

// correlatedStream overrides Context() so the wrapped handler observes
// the correlation-stamped ctx without us touching every method on the
// embedded ServerStream.
type correlatedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *correlatedStream) Context() context.Context { return s.ctx }

// extractGRPCCorrelationID returns the first value at GRPCMetadataKey
// from inbound metadata, or "" when absent.
func extractGRPCCorrelationID(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	v := md.Get(logging.GRPCMetadataKey)
	if len(v) == 0 {
		return ""
	}
	return v[0]
}

// isAcceptableCorrelationID rejects empty, too-long, or non-printable
// IDs. We want a generous regex (so UUIDs / trace IDs / etc. all pass)
// but tight enough that a hostile client can't smuggle newlines into
// the log stream.
func isAcceptableCorrelationID(id string) bool {
	if id == "" || len(id) > maxCorrelationIDLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if c < 0x20 || c == 0x7f {
			return false
		}
	}
	// Quick reject for any leading/trailing whitespace — clients
	// sometimes copy-paste with a stray newline.
	if strings.TrimSpace(id) != id {
		return false
	}
	return true
}
