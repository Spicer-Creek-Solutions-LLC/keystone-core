package policy

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
// connection — CLI mTLS is a v0.x ROADMAP carry-over shared with
// kscore-identity / kscore-secrets / kscore-events. API-key auth
// flows through `authorization: Bearer <key>` metadata.
func dialGRPC(_ context.Context, target, _ string) (v1.PolicyServiceClient, io.Closer, error) {
	if target == "" {
		return nil, nil, fmt.Errorf("policy: --server is required")
	}
	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("policy: dial %s: %w", target, err)
	}
	return v1.NewPolicyServiceClient(conn), conn, nil
}

// authContext attaches the API key (from --api-key or
// KSCORE_API_KEY) to outbound gRPC metadata. Empty key leaves the
// context untouched.
func authContext(ctx context.Context, apiKey string) context.Context {
	if apiKey == "" {
		apiKey = os.Getenv("KSCORE_API_KEY")
	}
	if apiKey == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+apiKey)
}
