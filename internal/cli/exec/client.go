package exec

import (
	"context"
	"fmt"
	"io"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// dialGRPC is the production gRPC dialer. v1.0 uses an insecure
// connection — TLS plumbing lives with Epic 09 (identity / auth). API
// key auth flows through the `authorization: Bearer <key>` metadata
// per pkg/api/auth.APIKeyAuthenticator.
func dialGRPC(_ context.Context, target, _ string) (v1.ControlPlaneServiceClient, io.Closer, error) {
	if target == "" {
		return nil, nil, fmt.Errorf("exec: --server is required")
	}
	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("exec: dial %s: %w", target, err)
	}
	return v1.NewControlPlaneServiceClient(conn), conn, nil
}

// authContext attaches the API key (from the --api-key flag or the
// KSCORE_API_KEY env var) to outbound gRPC metadata. Empty key leaves
// the context untouched; the server rejects with Unauthenticated when
// the auth chain is wired.
func authContext(ctx context.Context, apiKey string) context.Context {
	if apiKey == "" {
		apiKey = os.Getenv("KSCORE_API_KEY")
	}
	if apiKey == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+apiKey)
}
