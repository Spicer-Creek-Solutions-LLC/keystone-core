// SPDX-License-Identifier: Apache-2.0

package cluster_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	cli "go.keystone-core.io/keystone-core/internal/cli/cluster"
	icluster "go.keystone-core.io/keystone-core/internal/cluster"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// fakeServer is a minimal in-process ClusterService for exercising
// the CLI client + formatting deterministically (no controlplane
// internals). AddMember is Unimplemented by contract.
type fakeServer struct {
	v1.UnimplementedClusterServiceServer
	removed                  string
	transferred              bool
	restoreForce, restoreDry bool
}

func member(id, name string, role v1.ClusterMemberRole, st v1.ClusterMemberStatus) *v1.ClusterMember {
	return &v1.ClusterMember{Id: id, Name: name, Address: id + ":7000", Role: role, Status: st, Version: "0.x"}
}

func (s *fakeServer) GetClusterStatus(context.Context, *v1.GetClusterStatusRequest) (*v1.GetClusterStatusResponse, error) {
	return &v1.GetClusterStatusResponse{
		ClusterId: "c1", LeaderId: "m1", MemberCount: 2, HealthyCount: 1, Quorum: true,
	}, nil
}

func (s *fakeServer) ListMembers(_ context.Context, r *v1.ListMembersRequest) (*v1.ListMembersResponse, error) {
	all := []*v1.ClusterMember{
		member("m1", "n1", v1.ClusterMemberRole_CLUSTER_MEMBER_ROLE_LEADER, v1.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_HEALTHY),
		member("m2", "n2", v1.ClusterMemberRole_CLUSTER_MEMBER_ROLE_FOLLOWER, v1.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_DEGRADED),
	}
	if r.GetStatus() == v1.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_HEALTHY {
		all = all[:1]
	}
	return &v1.ListMembersResponse{Members: all, TotalCount: int32(len(all))}, nil
}

func (s *fakeServer) GetMember(_ context.Context, r *v1.GetMemberRequest) (*v1.GetMemberResponse, error) {
	if r.GetMemberId() != "m1" {
		return nil, status.Errorf(codes.NotFound, "member %s not found", r.GetMemberId())
	}
	return &v1.GetMemberResponse{Member: member("m1", "n1",
		v1.ClusterMemberRole_CLUSTER_MEMBER_ROLE_LEADER, v1.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_HEALTHY)}, nil
}

func (s *fakeServer) AddMember(context.Context, *v1.AddMemberRequest) (*v1.AddMemberResponse, error) {
	return nil, status.Error(codes.Unimplemented, "members self-register on start")
}

func (s *fakeServer) RemoveMember(_ context.Context, r *v1.RemoveMemberRequest) (*v1.RemoveMemberResponse, error) {
	s.removed = r.GetMemberId()
	return &v1.RemoveMemberResponse{}, nil
}

func (s *fakeServer) GetLeader(context.Context, *v1.GetLeaderRequest) (*v1.GetLeaderResponse, error) {
	return &v1.GetLeaderResponse{Leader: member("m1", "n1",
		v1.ClusterMemberRole_CLUSTER_MEMBER_ROLE_LEADER, v1.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_HEALTHY), Term: 7}, nil
}

func (s *fakeServer) TransferLeader(context.Context, *v1.TransferLeaderRequest) (*v1.TransferLeaderResponse, error) {
	s.transferred = true
	return &v1.TransferLeaderResponse{NewLeader: member("m2", "n2",
		v1.ClusterMemberRole_CLUSTER_MEMBER_ROLE_LEADER, v1.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_HEALTHY)}, nil
}

func (s *fakeServer) Rebalance(context.Context, *v1.RebalanceRequest) (*v1.RebalanceResponse, error) {
	return &v1.RebalanceResponse{ReassignedAgents: 3, Detail: "ok"}, nil
}

func (s *fakeServer) CreateBackup(context.Context, *v1.CreateBackupRequest) (*v1.CreateBackupResponse, error) {
	blob, err := icluster.MarshalSnapshot(icluster.BuildSnapshot("c1", "m1", nil,
		[]icluster.ShardAssignment{{AgentID: "ag1", MemberID: "m1"}}, nil))
	if err != nil {
		return nil, err
	}
	return &v1.CreateBackupResponse{Snapshot: blob, SizeBytes: int64(len(blob))}, nil
}

func (s *fakeServer) RestoreBackup(_ context.Context, r *v1.RestoreBackupRequest) (*v1.RestoreBackupResponse, error) {
	s.restoreForce, s.restoreDry = r.GetForce(), r.GetDryRun()
	if _, err := icluster.UnmarshalSnapshot(r.GetSnapshot()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return &v1.RestoreBackupResponse{Success: true, Detail: "restored"}, nil
}

func newDeps(t *testing.T) (*fakeServer, cli.Deps) {
	t.Helper()
	fs := &fakeServer{}
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	v1.RegisterClusterServiceServer(gs, fs)
	go func() {
		if err := gs.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Logf("serve: %v", err)
		}
	}()
	t.Cleanup(func() { gs.Stop(); _ = lis.Close() })
	deps := cli.Deps{Dial: func(_ context.Context, _, _ string) (v1.ClusterServiceClient, io.Closer, error) {
		conn, err := grpc.NewClient("passthrough://bufnet",
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
			grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, nil, err
		}
		return v1.NewClusterServiceClient(conn), conn, nil
	}}
	return fs, deps
}

func runCluster(t *testing.T, deps cli.Deps, args ...string) (string, error) {
	t.Helper()
	cmd := cli.NewClusterCommand(deps)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(context.Background())
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func runBackupCLI(t *testing.T, deps cli.Deps, args ...string) (string, error) {
	t.Helper()
	cmd := cli.NewBackupCommand(deps)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(context.Background())
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestStatus(t *testing.T) {
	_, deps := newDeps(t)
	out, err := runCluster(t, deps, "status")
	if err != nil {
		t.Fatalf("status: %v\n%s", err, out)
	}
	for _, w := range []string{"cluster:   c1", "leader:    m1", "members:   2 (healthy 1)", "quorum:    true", "m2"} {
		if !strings.Contains(out, w) {
			t.Errorf("status missing %q\n%s", w, out)
		}
	}
}

func TestMembersAndFilterAndNotFound(t *testing.T) {
	_, deps := newDeps(t)
	out, err := runCluster(t, deps, "members")
	if err != nil || !strings.Contains(out, "m1") || !strings.Contains(out, "m2") || !strings.Contains(out, "total: 2") {
		t.Fatalf("members = %q, %v", out, err)
	}
	out, _ = runCluster(t, deps, "members", "--status", "healthy")
	if !strings.Contains(out, "total: 1") || strings.Contains(out, "m2") {
		t.Errorf("filtered members = %q", out)
	}
	out, _ = runCluster(t, deps, "members", "m1")
	if !strings.Contains(out, "leader") || !strings.Contains(out, "healthy") {
		t.Errorf("get member = %q", out)
	}
	if _, err := runCluster(t, deps, "members", "zzz"); err == nil {
		t.Error("members zzz: want error")
	}
	if _, err := runCluster(t, deps, "members", "--status", "bogus"); err == nil {
		t.Error("bad --status: want error")
	}
}

func TestLeaderTransferRemoveRebalance(t *testing.T) {
	fs, deps := newDeps(t)
	if out, err := runCluster(t, deps, "leader"); err != nil || !strings.Contains(out, "term: 7") {
		t.Fatalf("leader = %q, %v", out, err)
	}
	if out, err := runCluster(t, deps, "transfer-leader", "--target", "m2"); err != nil || !strings.Contains(out, "new leader m2") {
		t.Fatalf("transfer = %q, %v", out, err)
	}
	if !fs.transferred {
		t.Error("TransferLeader not invoked")
	}
	if out, err := runCluster(t, deps, "remove", "m9"); err != nil || !strings.Contains(out, "removed: m9") {
		t.Fatalf("remove = %q, %v", out, err)
	}
	if fs.removed != "m9" {
		t.Errorf("removed = %q", fs.removed)
	}
	if out, err := runCluster(t, deps, "rebalance"); err != nil || !strings.Contains(out, "reassigned agents: 3") {
		t.Fatalf("rebalance = %q, %v", out, err)
	}
}

func TestAddIsContractPassthrough(t *testing.T) {
	_, deps := newDeps(t)
	out, err := runCluster(t, deps, "add", "--name", "x")
	if err == nil {
		t.Fatalf("add: want Unimplemented error, got nil\n%s", out)
	}
	if !strings.Contains(err.Error(), "AddMember") {
		t.Errorf("add error = %v, want it to mention AddMember", err)
	}
}

func TestBackupRestoreRoundTrip(t *testing.T) {
	fs, deps := newDeps(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.kscore")

	if out, err := runBackupCLI(t, deps, "backup", "--output", path); err != nil || !strings.Contains(out, "backup written") {
		t.Fatalf("backup = %q, %v", out, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("snapshot file: %v", err)
	}
	if out, err := runBackupCLI(t, deps, "restore", "--input", path, "--force"); err != nil || !strings.Contains(out, "success: true") {
		t.Fatalf("restore = %q, %v", out, err)
	}
	if !fs.restoreForce {
		t.Error("--force not propagated")
	}
}

func TestBackupToStdout(t *testing.T) {
	_, deps := newDeps(t)
	out, err := runBackupCLI(t, deps, "backup")
	if err != nil || len(out) == 0 {
		t.Fatalf("backup stdout = %d bytes, %v", len(out), err)
	}
}

func TestLocalListAndVerify(t *testing.T) {
	_, deps := newDeps(t)
	dir := t.TempDir()
	good := filepath.Join(dir, "good.kscore")
	blob, _ := icluster.MarshalSnapshot(icluster.BuildSnapshot("c9", "lead", nil, nil, nil))
	if err := os.WriteFile(good, blob, 0o600); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad.kscore")
	if err := os.WriteFile(bad, []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := runBackupCLI(t, deps, "list", dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "good.kscore") || !strings.Contains(out, "c9") ||
		!strings.Contains(out, "bad.kscore") || !strings.Contains(out, "false") {
		t.Errorf("list output = %q", out)
	}

	if out, err := runBackupCLI(t, deps, "verify", "--input", good); err != nil || !strings.Contains(out, "valid:") {
		t.Fatalf("verify good = %q, %v", out, err)
	}
	if _, err := runBackupCLI(t, deps, "verify", "--input", bad); err == nil {
		t.Error("verify bad: want error")
	}
}

func TestJSONOutput(t *testing.T) {
	_, deps := newDeps(t)
	out, err := runCluster(t, deps, "-o", "json", "status")
	if err != nil || !strings.Contains(out, "\"clusterId\"") {
		t.Fatalf("json status = %q, %v", out, err)
	}
	if _, err := runCluster(t, deps, "-o", "yaml", "status"); err == nil {
		t.Error("invalid -o: want error")
	}
}

func TestRootHelp(t *testing.T) {
	// kscore-cluster help
	c := cli.NewClusterCommand(cli.Deps{})
	var b bytes.Buffer
	c.SetOut(&b)
	c.SetErr(&b)
	c.SetArgs([]string{"--help"})
	if err := c.Execute(); err != nil {
		t.Fatalf("cluster --help: %v", err)
	}
	for _, w := range []string{"status", "members", "leader", "add", "remove", "transfer-leader", "rebalance", "backup", "restore", "--server"} {
		if !strings.Contains(b.String(), w) {
			t.Errorf("kscore-cluster --help missing %q", w)
		}
	}
	// kscore-cluster-backup help
	bc := cli.NewBackupCommand(cli.Deps{})
	var bb bytes.Buffer
	bc.SetOut(&bb)
	bc.SetErr(&bb)
	bc.SetArgs([]string{"--help"})
	if err := bc.Execute(); err != nil {
		t.Fatalf("backup --help: %v", err)
	}
	for _, w := range []string{"backup", "restore", "list", "verify"} {
		if !strings.Contains(bb.String(), w) {
			t.Errorf("kscore-cluster-backup --help missing %q", w)
		}
	}
}
