package server_test

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/shawnbutts/keystone-core/internal/config"
	"github.com/shawnbutts/keystone-core/pkg/api/auth"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"

	"github.com/shawnbutts/keystone-core/pkg/api/server"
)

const testAPIKey = "test-integration-key"

// startTestServer starts a gRPC server with all 7 services registered and an
// auth interceptor. It returns a client connection and cleanup function.
func startTestServer(t *testing.T) (*grpc.ClientConn, func()) {
	t.Helper()

	// Set up API key auth
	authCfg := config.APIKeyAuthConfig{
		MetadataKey: "x-api-key",
		Keys: map[string]config.APIKeyConfig{
			testAPIKey: {Name: "test", Role: "admin", Enabled: true},
		},
	}
	authenticator, err := auth.NewAPIKeyAuthenticator(authCfg)
	if err != nil {
		t.Fatalf("create authenticator: %v", err)
	}
	interceptorCfg := &auth.InterceptorConfig{
		Authenticators: []auth.Authenticator{authenticator},
		MetadataKey:    "x-api-key",
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(auth.UnaryServerInterceptor(interceptorCfg)),
		grpc.StreamInterceptor(auth.StreamServerInterceptor(interceptorCfg)),
	)

	// Register all 7 services with nil deps
	pb.RegisterControlPlaneServiceServer(grpcServer, server.NewControlPlaneServer(nil, nil, nil, nil))
	pb.RegisterSecretsServiceServer(grpcServer, server.NewSecretsServer(nil, nil, nil))
	pb.RegisterAgentServiceServer(grpcServer, server.NewAgentServer(nil))
	pb.RegisterStateServiceServer(grpcServer, server.NewStateServer(nil, nil))
	pb.RegisterEventServiceServer(grpcServer, server.NewEventServer(nil, nil, nil))
	pb.RegisterPolicyServiceServer(grpcServer, server.NewPolicyServer(nil, nil, nil, nil))
	pb.RegisterClusterServiceServer(grpcServer, server.NewClusterServer(nil, nil, nil))

	var lc net.ListenConfig
	lis, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go grpcServer.Serve(lis)

	conn, err := grpc.NewClient(
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		grpcServer.Stop()
		t.Fatalf("dial: %v", err)
	}

	cleanup := func() {
		conn.Close()
		grpcServer.Stop()
	}
	return conn, cleanup
}

func authedCtx() context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), "x-api-key", testAPIKey)
}

// TestIntegration_AllServicesRegistered verifies that all 7 gRPC services are
// reachable on a single server. Services with nil-dep guards return Unavailable.
// ControlPlaneService predates the nil-dep pattern so we verify it via auth only.
func TestIntegration_AllServicesRegistered(t *testing.T) {
	conn, cleanup := startTestServer(t)
	defer cleanup()

	ctx := authedCtx()

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "SecretsService/GetSecret",
			call: func() error {
				_, err := pb.NewSecretsServiceClient(conn).GetSecret(ctx, &pb.GetSecretRequest{Path: "secret/test"})
				return err
			},
		},
		{
			name: "AgentService/GetAgentInfo",
			call: func() error {
				_, err := pb.NewAgentServiceClient(conn).GetAgentInfo(ctx, &pb.GetAgentInfoRequest{AgentId: "a1"})
				return err
			},
		},
		{
			name: "StateService/DetectDrift",
			call: func() error {
				_, err := pb.NewStateServiceClient(conn).DetectDrift(ctx, &pb.DetectDriftRequest{Target: "agent-1"})
				return err
			},
		},
		{
			name: "EventService/EmitEvent",
			call: func() error {
				_, err := pb.NewEventServiceClient(conn).EmitEvent(ctx, &pb.EmitEventRequest{Type: "test"})
				return err
			},
		},
		{
			name: "PolicyService/ListPolicies",
			call: func() error {
				_, err := pb.NewPolicyServiceClient(conn).ListPolicies(ctx, &pb.ListPoliciesRequest{})
				return err
			},
		},
		{
			name: "ClusterService/GetClusterStatus",
			call: func() error {
				_, err := pb.NewClusterServiceClient(conn).GetClusterStatus(ctx, &pb.GetClusterStatusRequest{})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("expected gRPC status error, got: %v", err)
			}
			if st.Code() != codes.Unavailable {
				t.Errorf("got code %v, want Unavailable (msg: %s)", st.Code(), st.Message())
			}
		})
	}
}

// TestIntegration_AuthInterceptor_RejectsUnauthenticated verifies the auth
// interceptor enforces authentication on all services. Calls without an API
// key should be rejected with Unauthenticated.
func TestIntegration_AuthInterceptor_RejectsUnauthenticated(t *testing.T) {
	conn, cleanup := startTestServer(t)
	defer cleanup()

	ctx := context.Background() // no auth metadata

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "ControlPlaneService",
			call: func() error {
				_, err := pb.NewControlPlaneServiceClient(conn).ListAgents(ctx, &pb.ListAgentsRequest{})
				return err
			},
		},
		{
			name: "SecretsService",
			call: func() error {
				_, err := pb.NewSecretsServiceClient(conn).GetSecret(ctx, &pb.GetSecretRequest{Path: "x"})
				return err
			},
		},
		{
			name: "AgentService",
			call: func() error {
				_, err := pb.NewAgentServiceClient(conn).GetAgentInfo(ctx, &pb.GetAgentInfoRequest{AgentId: "a1"})
				return err
			},
		},
		{
			name: "StateService",
			call: func() error {
				_, err := pb.NewStateServiceClient(conn).DetectDrift(ctx, &pb.DetectDriftRequest{Target: "agent-1"})
				return err
			},
		},
		{
			name: "EventService",
			call: func() error {
				_, err := pb.NewEventServiceClient(conn).EmitEvent(ctx, &pb.EmitEventRequest{Type: "test"})
				return err
			},
		},
		{
			name: "PolicyService",
			call: func() error {
				_, err := pb.NewPolicyServiceClient(conn).ListPolicies(ctx, &pb.ListPoliciesRequest{})
				return err
			},
		},
		{
			name: "ClusterService",
			call: func() error {
				_, err := pb.NewClusterServiceClient(conn).GetClusterStatus(ctx, &pb.GetClusterStatusRequest{})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("expected gRPC status error, got: %v", err)
			}
			if st.Code() != codes.Unauthenticated {
				t.Errorf("got code %v, want Unauthenticated", st.Code())
			}
		})
	}
}

// TestIntegration_AuthInterceptor_InvalidKey verifies that an invalid API key
// is rejected by the auth interceptor.
func TestIntegration_AuthInterceptor_InvalidKey(t *testing.T) {
	conn, cleanup := startTestServer(t)
	defer cleanup()

	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-api-key", "wrong-key")
	_, err := pb.NewControlPlaneServiceClient(conn).GetServerStatus(ctx, &pb.GetServerStatusRequest{})
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got: %v", err)
	}
	if st.Code() != codes.Unauthenticated {
		t.Errorf("got code %v, want Unauthenticated", st.Code())
	}
}

// TestIntegration_AuthInterceptor_StreamingRPC verifies the auth interceptor
// covers streaming RPCs (not just unary).
func TestIntegration_AuthInterceptor_StreamingRPC(t *testing.T) {
	conn, cleanup := startTestServer(t)
	defer cleanup()

	// Unauthenticated streaming call
	ctx := context.Background()
	stream, err := pb.NewEventServiceClient(conn).SubscribeEvents(ctx, &pb.SubscribeEventsRequest{})
	if err != nil {
		// Some gRPC versions return error on creation, others on Recv
		st, ok := status.FromError(err)
		if ok && st.Code() == codes.Unauthenticated {
			return // expected
		}
	}
	if stream != nil {
		_, err = stream.Recv()
		st, ok := status.FromError(err)
		if !ok {
			t.Fatalf("expected gRPC status error, got: %v", err)
		}
		if st.Code() != codes.Unauthenticated {
			t.Errorf("got code %v, want Unauthenticated", st.Code())
		}
	}
}

// TestIntegration_StateHistory_WithStore verifies GetStateHistory returns data
// when a history store is wired (instead of codes.Unavailable).
func TestIntegration_StateHistory_WithStore(t *testing.T) {
	conn, cleanup := startTestServerWithHistoryStore(t)
	defer cleanup()

	ctx := authedCtx()
	client := pb.NewStateServiceClient(conn)

	resp, err := client.GetStateHistory(ctx, &pb.GetStateHistoryRequest{})
	if err != nil {
		t.Fatalf("GetStateHistory: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	// Empty store, so no runs
	if len(resp.Runs) != 0 {
		t.Errorf("expected 0 runs, got %d", len(resp.Runs))
	}
}

// TestIntegration_StateStatus_WithStore verifies GetStateStatus returns data
// when a history store is wired.
func TestIntegration_StateStatus_WithStore(t *testing.T) {
	conn, cleanup := startTestServerWithHistoryStore(t)
	defer cleanup()

	ctx := authedCtx()
	client := pb.NewStateServiceClient(conn)

	resp, err := client.GetStateStatus(ctx, &pb.GetStateStatusRequest{AgentId: "agent-1"})
	if err != nil {
		t.Fatalf("GetStateStatus: %v", err)
	}
	if resp.AgentId != "agent-1" {
		t.Errorf("agent_id = %q, want %q", resp.AgentId, "agent-1")
	}
}

// startTestServerWithHistoryStore starts a gRPC server with StateServer wired
// with a mock history store so GetStateHistory/GetStateStatus work.
func startTestServerWithHistoryStore(t *testing.T) (*grpc.ClientConn, func()) {
	t.Helper()

	authCfg := config.APIKeyAuthConfig{
		MetadataKey: "x-api-key",
		Keys: map[string]config.APIKeyConfig{
			testAPIKey: {Name: "test", Role: "admin", Enabled: true},
		},
	}
	authenticator, err := auth.NewAPIKeyAuthenticator(authCfg)
	if err != nil {
		t.Fatalf("create authenticator: %v", err)
	}
	interceptorCfg := &auth.InterceptorConfig{
		Authenticators: []auth.Authenticator{authenticator},
		MetadataKey:    "x-api-key",
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(auth.UnaryServerInterceptor(interceptorCfg)),
		grpc.StreamInterceptor(auth.StreamServerInterceptor(interceptorCfg)),
	)

	stateServer := server.NewStateServer(nil, nil)
	stateServer.SetHistoryStore(&emptyHistoryStore{})
	pb.RegisterStateServiceServer(grpcServer, stateServer)

	var lc net.ListenConfig
	lis, lisErr := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if lisErr != nil {
		t.Fatalf("listen: %v", lisErr)
	}
	go grpcServer.Serve(lis)

	conn, connErr := grpc.NewClient(
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if connErr != nil {
		grpcServer.Stop()
		t.Fatalf("dial: %v", connErr)
	}

	cleanup := func() {
		conn.Close()
		grpcServer.Stop()
	}
	return conn, cleanup
}

// emptyHistoryStore is a no-op history store for integration tests.
type emptyHistoryStore struct{}

func (s *emptyHistoryStore) SaveRun(_ context.Context, _ *server.StateHistoryRun) error {
	return nil
}
func (s *emptyHistoryStore) ListRuns(_ context.Context, _ *server.StateHistoryFilter) ([]*server.StateHistoryRun, string, error) {
	return nil, "", nil
}
func (s *emptyHistoryStore) GetStatus(_ context.Context, _, _ string) ([]*server.StateStatusEntry, error) {
	return nil, nil
}
