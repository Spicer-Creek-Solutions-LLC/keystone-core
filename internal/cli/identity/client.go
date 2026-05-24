// SPDX-License-Identifier: Apache-2.0

package identity

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

// dialGRPC is the production gRPC dialer. v0.1 uses an insecure
// connection — mTLS plumbing lives with Epic 09 task 13.
// API-key auth flows through the `authorization: Bearer <key>`
// metadata per pkg/api/auth.APIKeyAuthenticator.
func dialGRPC(_ context.Context, target, _ string) (v1.IdentityServiceClient, io.Closer, error) {
	if target == "" {
		return nil, nil, fmt.Errorf("identity: --server is required")
	}
	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("identity: dial %s: %w", target, err)
	}
	return v1.NewIdentityServiceClient(conn), conn, nil
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
