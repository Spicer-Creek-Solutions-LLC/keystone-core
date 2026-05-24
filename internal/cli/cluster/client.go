// SPDX-License-Identifier: Apache-2.0

package cluster

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
// kscore-policy / kscore-identity / kscore-secrets / kscore-events.
// API-key auth flows through `authorization: Bearer <key>` metadata.
func dialGRPC(_ context.Context, target, _ string) (v1.ClusterServiceClient, io.Closer, error) {
	if target == "" {
		return nil, nil, fmt.Errorf("cluster: --server is required")
	}
	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("cluster: dial %s: %w", target, err)
	}
	return v1.NewClusterServiceClient(conn), conn, nil
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

// connect dials + attaches auth in one step; callers defer the
// returned closer.
func (g *globals) connect(ctx context.Context) (v1.ClusterServiceClient, io.Closer, context.Context, error) {
	if err := validateOutput(g.Output); err != nil {
		return nil, nil, nil, err
	}
	client, closer, err := g.Deps.Dial(ctx, g.Server, g.APIKey)
	if err != nil {
		return nil, nil, nil, err
	}
	return client, closer, authContext(ctx, g.APIKey), nil
}
