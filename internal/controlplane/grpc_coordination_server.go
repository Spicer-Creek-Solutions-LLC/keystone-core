// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.keystone-core.io/keystone-core/internal/cluster"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// Backing components are independently nilable; an RPC needing a
// missing one returns codes.Unavailable — except the recovery /
// NATS-status RPCs, whose whole purpose is to answer when peers are
// degraded, so they return best-effort partial data instead.
type (
	coordHealth interface {
		Status() cluster.MemberStatus
		Quorum() cluster.QuorumState
	}
	coordLeader interface {
		LeaderID(ctx context.Context) (string, error)
		IsLeader() bool
	}
	coordMembers interface {
		LoadMembers(ctx context.Context) ([]cluster.Member, error)
	}
	coordShards interface {
		List(ctx context.Context) ([]cluster.ShardAssignment, error)
	}
	coordNATS interface {
		Connected() bool
		Detail() string
	}
)

// CoordinationGRPCServer implements v1.CoordinationServiceServer
// (Epic 13 task 12) — the server↔server NATS-down recovery channel.
// It is mTLS-only: every RPC rejects callers without a verified
// client certificate (codes.Unauthenticated), per the Epic 13
// acceptance criterion.
//
// Registering this on a dedicated mTLS listener is boot wiring
// (deferred — see the "Cluster gRPC services boot registration"
// ROADMAP entry).
type CoordinationGRPCServer struct {
	v1.UnimplementedCoordinationServiceServer

	Health  coordHealth
	Leader  coordLeader
	Members coordMembers
	Shards  coordShards
	NATS    coordNATS
	// Propagate applies a state blob a peer pushed when NATS is
	// down. nil ⇒ accept + no-op (the apply path is boot/later).
	Propagate func(ctx context.Context, kind string, payload []byte) error

	SelfID      string
	SelfVersion string
}

// requireMTLS rejects any caller that did not present a verified
// client certificate. CoordinationService is server↔server only.
func requireMTLS(ctx context.Context) error {
	p, ok := peer.FromContext(ctx)
	if !ok || p.AuthInfo == nil {
		return status.Error(codes.Unauthenticated, "coordination: mTLS required (no peer auth)")
	}
	ti, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(ti.State.VerifiedChains) == 0 {
		return status.Error(codes.Unauthenticated, "coordination: mTLS client certificate required")
	}
	return nil
}

func (s *CoordinationGRPCServer) leaderID(ctx context.Context) string {
	if s.Leader == nil {
		return ""
	}
	id, err := s.Leader.LeaderID(ctx)
	if err != nil {
		return "" // ErrNoLeader / transient → empty (best-effort)
	}
	return id
}

func memberInfos(ms []cluster.Member) []*v1.MemberInfo {
	out := make([]*v1.MemberInfo, 0, len(ms))
	for _, m := range ms {
		out = append(out, &v1.MemberInfo{
			Id: m.ID, Name: m.Name, Addr: m.Addr, Status: string(m.Status),
		})
	}
	return out
}

func shardInfos(as []cluster.ShardAssignment) []*v1.ShardAssignmentInfo {
	out := make([]*v1.ShardAssignmentInfo, 0, len(as))
	for _, a := range as {
		out = append(out, &v1.ShardAssignmentInfo{
			AgentId: a.AgentID, MemberId: a.MemberID, Version: a.Version,
		})
	}
	return out
}

func (s *CoordinationGRPCServer) ClusterHealth(ctx context.Context, _ *v1.ClusterHealthRequest) (*v1.ClusterHealthResponse, error) {
	if err := requireMTLS(ctx); err != nil {
		return nil, err
	}
	if s.Health == nil {
		return nil, status.Error(codes.Unavailable, "coordination: health monitor unavailable")
	}
	resp := &v1.ClusterHealthResponse{
		NodeId:       s.SelfID,
		NodeVersion:  s.SelfVersion,
		At:           timestamppb.New(time.Now().UTC()),
		MemberStatus: string(s.Health.Status()),
		Quorum:       string(s.Health.Quorum()),
		LeaderId:     s.leaderID(ctx),
		// StorageHealthy is not probed by the coordination channel
		// (ClusterService / health endpoints own that); reported
		// true so peers don't misread an unprobed field as a fault.
		StorageHealthy: true,
		NatsHealthy:    s.NATS != nil && s.NATS.Connected(),
	}
	if s.Members != nil {
		if ms, err := s.Members.LoadMembers(ctx); err == nil {
			resp.MemberCount = int32(len(ms))
		}
	}
	return resp, nil
}

func (s *CoordinationGRPCServer) LookupLeader(ctx context.Context, _ *v1.LookupLeaderRequest) (*v1.LookupLeaderResponse, error) {
	if err := requireMTLS(ctx); err != nil {
		return nil, err
	}
	if s.Leader == nil {
		return nil, status.Error(codes.Unavailable, "coordination: leader elector unavailable")
	}
	id, err := s.Leader.LeaderID(ctx)
	if err != nil && !errors.Is(err, cluster.ErrNoLeader) {
		return nil, status.Errorf(codes.Internal, "lookup leader: %v", err)
	}
	return &v1.LookupLeaderResponse{LeaderId: id, IsSelf: s.Leader.IsLeader()}, nil
}

func (s *CoordinationGRPCServer) NATSStatus(ctx context.Context, _ *v1.NATSStatusRequest) (*v1.NATSStatusResponse, error) {
	if err := requireMTLS(ctx); err != nil {
		return nil, err
	}
	// "NATS is unavailable" is itself a valid answer on the
	// recovery channel — never return Unavailable here.
	if s.NATS == nil {
		return &v1.NATSStatusResponse{Connected: false, Detail: "unknown"}, nil
	}
	return &v1.NATSStatusResponse{Connected: s.NATS.Connected(), Detail: s.NATS.Detail()}, nil
}

func (s *CoordinationGRPCServer) RecoveryCoordinate(ctx context.Context, req *v1.RecoveryCoordinateRequest) (*v1.RecoveryCoordinateResponse, error) {
	if err := requireMTLS(ctx); err != nil {
		return nil, err
	}
	// Best-effort snapshot: a recovering peer wants whatever the
	// healthy peer can give, even if some component is absent.
	resp := &v1.RecoveryCoordinateResponse{Acknowledged: true, LeaderId: s.leaderID(ctx)}
	if s.Members != nil {
		if ms, err := s.Members.LoadMembers(ctx); err == nil {
			resp.Members = memberInfos(ms)
		}
	}
	if s.Shards != nil {
		if as, err := s.Shards.List(ctx); err == nil {
			resp.ShardAssignments = shardInfos(as)
		}
	}
	return resp, nil
}

func (s *CoordinationGRPCServer) NodeHeartbeat(ctx context.Context, _ *v1.NodeHeartbeatRequest) (*v1.NodeHeartbeatResponse, error) {
	if err := requireMTLS(ctx); err != nil {
		return nil, err
	}
	resp := &v1.NodeHeartbeatResponse{
		ServerTime: timestamppb.New(time.Now().UTC()),
		MemberId:   s.SelfID,
	}
	if s.Health != nil {
		resp.Status = string(s.Health.Status())
	}
	return resp, nil
}

func (s *CoordinationGRPCServer) PropagateState(ctx context.Context, req *v1.PropagateStateRequest) (*v1.PropagateStateResponse, error) {
	if err := requireMTLS(ctx); err != nil {
		return nil, err
	}
	if s.Propagate == nil {
		// No apply hook wired yet — accept so the sender's
		// NATS-down fan-out does not stall; real apply is boot/later.
		return &v1.PropagateStateResponse{Accepted: true}, nil
	}
	if err := s.Propagate(ctx, req.GetKind(), req.GetSnapshot()); err != nil {
		return nil, status.Errorf(codes.Internal, "propagate state: %v", err)
	}
	return &v1.PropagateStateResponse{Accepted: true}, nil
}
