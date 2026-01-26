package cluster

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
	"github.com/shawnbutts/keystone-core/internal/testing/mocks"
)

type integrationLeaderElection struct {
	leaderID string
}

func (l *integrationLeaderElection) Campaign(ctx context.Context) error {
	_ = ctx
	return nil
}

func (l *integrationLeaderElection) Resign(ctx context.Context) error {
	_ = ctx
	return nil
}

func (l *integrationLeaderElection) IsLeader() bool {
	return l.leaderID != ""
}

func (l *integrationLeaderElection) GetLeader(ctx context.Context) (string, error) {
	_ = ctx
	return l.leaderID, nil
}

func (l *integrationLeaderElection) TransferLeadership(ctx context.Context, targetID string) error {
	l.leaderID = targetID
	return nil
}

func (l *integrationLeaderElection) AddObserver(observer LeadershipObserver) {}

func (l *integrationLeaderElection) RemoveObserver(observer LeadershipObserver) {}

func TestCoordinationServiceIntegration(t *testing.T) {
	config := &Config{
		ClusterName: "test-cluster",
		QuorumSize:  1,
	}

	member := &Member{
		ID:            "member-1",
		Address:       "127.0.0.1:9090",
		GRPCAddress:   "127.0.0.1:9090",
		Status:        MemberStatusHealthy,
		IsLeader:      true,
		JoinedAt:      time.Now().Add(-1 * time.Minute),
		LastHeartbeat: time.Now(),
	}

	membership := &MembershipManager{
		config:  config,
		members: map[string]*Member{"member-1": member},
	}

	natsStatus := &mocks.NATSStatusProvider{
		Connected:     true,
		URLs:          []string{"nats://127.0.0.1:4222"},
		JetStream:     true,
		LastPublish:   time.Now().Add(-5 * time.Second),
		LastSubscribe: time.Now().Add(-3 * time.Second),
	}

	leader := &integrationLeaderElection{leaderID: "member-1"}

	server, err := NewCoordinationServer(&CoordinationServerConfig{ServerID: "member-1"}, membership, nil, leader, natsStatus)
	if err != nil {
		t.Fatalf("NewCoordinationServer() error = %v", err)
	}

	grpcServer := grpc.NewServer()
	server.Register(grpcServer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			t.Logf("gRPC server error: %v", err)
		}
	}()
	defer grpcServer.GracefulStop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatalf("failed to dial gRPC: %v", err)
	}
	defer conn.Close()

	client := pb.NewCoordinationServiceClient(conn)

	healthResp, err := client.ClusterHealth(ctx, &pb.ClusterHealthRequest{
		RequestId:      "req-1",
		IncludeMembers: true,
		IncludeNats:    true,
	})
	if err != nil {
		t.Fatalf("ClusterHealth() error = %v", err)
	}
	if healthResp.TotalMembers != 1 || healthResp.HealthyMembers != 1 {
		t.Fatalf("ClusterHealth() members = %d/%d, want 1/1", healthResp.HealthyMembers, healthResp.TotalMembers)
	}
	if healthResp.LeaderId != "member-1" {
		t.Fatalf("ClusterHealth() leader = %q, want %q", healthResp.LeaderId, "member-1")
	}
	if healthResp.NatsStatus == nil {
		t.Fatal("ClusterHealth() expected NATS status")
	}
	if healthResp.NatsStatus.Status != pb.NATSHealthStatus_NATS_HEALTH_STATUS_HEALTHY {
		t.Fatalf("ClusterHealth() NATS status = %v, want healthy", healthResp.NatsStatus.Status)
	}
	if healthResp.NatsStatus.ConnectedServers != 1 {
		t.Fatalf("ClusterHealth() connected servers = %d, want 1", healthResp.NatsStatus.ConnectedServers)
	}

	leaderResp, err := client.GetLeader(ctx, &pb.GetLeaderRequest{RequestId: "req-2"})
	if err != nil {
		t.Fatalf("GetLeader() error = %v", err)
	}
	if !leaderResp.HasLeader || leaderResp.LeaderId != "member-1" {
		t.Fatalf("GetLeader() leader = %q, want member-1", leaderResp.LeaderId)
	}

	heartbeatResp, err := client.Heartbeat(ctx, &pb.ServerHeartbeatRequest{
		SenderId:  "member-2",
		Timestamp: timestamppb.New(time.Now()),
		Sequence:  1,
	})
	if err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
	if heartbeatResp.ResponderId != "member-1" {
		t.Fatalf("Heartbeat() responder = %q, want %q", heartbeatResp.ResponderId, "member-1")
	}

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return server.heartbeatCount > 0, nil
	}); err != nil {
		t.Fatalf("heartbeat metrics not updated: %v", err)
	}
}
