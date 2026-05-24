// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"context"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// fakeClient is a minimal v1.ControlPlaneServiceClient implementation
// for testing the kscorectl exec subcommands. Each test sets the
// scripted return values directly on the struct.
type fakeClient struct {
	v1.ControlPlaneServiceClient // embedded so we satisfy the interface;
	//                                unset methods panic if called.

	GetBatchJobFn            func(ctx context.Context, in *v1.GetBatchJobRequest) (*v1.GetBatchJobResponse, error)
	ListBatchJobsFn          func(ctx context.Context, in *v1.ListBatchJobsRequest) (*v1.ListBatchJobsResponse, error)
	CancelBatchJobFn         func(ctx context.Context, in *v1.CancelBatchJobRequest) (*v1.CancelBatchJobResponse, error)
	ListBatchAgentResultsFn  func(ctx context.Context, in *v1.ListBatchAgentResultsRequest) (*v1.ListBatchAgentResultsResponse, error)
	GetBatchAgentResultFn    func(ctx context.Context, in *v1.GetBatchAgentResultRequest) (*v1.GetBatchAgentResultResponse, error)
	BatchExecuteCommandFn    func(ctx context.Context, in *v1.BatchExecuteCommandRequest) (v1.ControlPlaneService_BatchExecuteCommandClient, error)
}

func (f *fakeClient) GetBatchJob(ctx context.Context, in *v1.GetBatchJobRequest, _ ...grpc.CallOption) (*v1.GetBatchJobResponse, error) {
	return f.GetBatchJobFn(ctx, in)
}
func (f *fakeClient) ListBatchJobs(ctx context.Context, in *v1.ListBatchJobsRequest, _ ...grpc.CallOption) (*v1.ListBatchJobsResponse, error) {
	return f.ListBatchJobsFn(ctx, in)
}
func (f *fakeClient) CancelBatchJob(ctx context.Context, in *v1.CancelBatchJobRequest, _ ...grpc.CallOption) (*v1.CancelBatchJobResponse, error) {
	return f.CancelBatchJobFn(ctx, in)
}
func (f *fakeClient) ListBatchAgentResults(ctx context.Context, in *v1.ListBatchAgentResultsRequest, _ ...grpc.CallOption) (*v1.ListBatchAgentResultsResponse, error) {
	return f.ListBatchAgentResultsFn(ctx, in)
}
func (f *fakeClient) GetBatchAgentResult(ctx context.Context, in *v1.GetBatchAgentResultRequest, _ ...grpc.CallOption) (*v1.GetBatchAgentResultResponse, error) {
	return f.GetBatchAgentResultFn(ctx, in)
}
func (f *fakeClient) BatchExecuteCommand(ctx context.Context, in *v1.BatchExecuteCommandRequest, _ ...grpc.CallOption) (v1.ControlPlaneService_BatchExecuteCommandClient, error) {
	return f.BatchExecuteCommandFn(ctx, in)
}

// fakeStream replays a scripted set of responses for the BatchExecute
// stream interface.
type fakeBatchStream struct {
	v1.ControlPlaneService_BatchExecuteCommandClient
	events []*v1.BatchExecuteCommandResponse
	idx    int
	ctx    context.Context
}

func (s *fakeBatchStream) Recv() (*v1.BatchExecuteCommandResponse, error) {
	if s.idx >= len(s.events) {
		return nil, io.EOF
	}
	ev := s.events[s.idx]
	s.idx++
	return ev, nil
}
func (s *fakeBatchStream) Context() context.Context  { return s.ctx }
func (s *fakeBatchStream) Header() (metadata.MD, error) { return nil, nil }
func (s *fakeBatchStream) Trailer() metadata.MD       { return nil }
func (s *fakeBatchStream) CloseSend() error           { return nil }
func (s *fakeBatchStream) SendMsg(_ any) error        { return nil }
func (s *fakeBatchStream) RecvMsg(_ any) error        { return nil }

// fakeDial wraps a fakeClient in the Deps.Dial shape.
func fakeDial(client v1.ControlPlaneServiceClient) func(ctx context.Context, target, apiKey string) (v1.ControlPlaneServiceClient, io.Closer, error) {
	return func(_ context.Context, _, _ string) (v1.ControlPlaneServiceClient, io.Closer, error) {
		return client, noopCloser{}, nil
	}
}

type noopCloser struct{}

func (noopCloser) Close() error { return nil }
