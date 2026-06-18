// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"io"

	"google.golang.org/grpc"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// fakeClient is a minimal v1.ControlPlaneServiceClient implementation
// for testing. Each test sets the scripted ListAgents return value
// directly on the struct.
type fakeClient struct {
	v1.ControlPlaneServiceClient // embedded so we satisfy the interface;
	//                                unset methods panic if called.

	ListAgentsFn   func(ctx context.Context, in *v1.ListAgentsRequest) (*v1.ListAgentsResponse, error)
	QuarantineFn   func(ctx context.Context, in *v1.QuarantineAgentRequest) (*v1.QuarantineAgentResponse, error)
	UnquarantineFn func(ctx context.Context, in *v1.UnquarantineAgentRequest) (*v1.UnquarantineAgentResponse, error)
	VerifyFn       func(ctx context.Context, in *v1.VerifyAgentRequest) (*v1.VerifyAgentResponse, error)
}

func (f *fakeClient) VerifyAgent(ctx context.Context, in *v1.VerifyAgentRequest, _ ...grpc.CallOption) (*v1.VerifyAgentResponse, error) {
	return f.VerifyFn(ctx, in)
}

func (f *fakeClient) ListAgents(ctx context.Context, in *v1.ListAgentsRequest, _ ...grpc.CallOption) (*v1.ListAgentsResponse, error) {
	return f.ListAgentsFn(ctx, in)
}

// fakeDial wraps a fakeClient in the Deps.Dial shape.
func fakeDial(client v1.ControlPlaneServiceClient) func(ctx context.Context, target, apiKey string) (v1.ControlPlaneServiceClient, io.Closer, error) {
	return func(_ context.Context, _, _ string) (v1.ControlPlaneServiceClient, io.Closer, error) {
		return client, noopCloser{}, nil
	}
}

type noopCloser struct{}

func (noopCloser) Close() error { return nil }
