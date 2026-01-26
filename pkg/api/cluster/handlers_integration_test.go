package cluster

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	clusterpkg "github.com/shawnbutts/keystone-core/internal/cluster"
	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

func TestClusterStatusAndMembersIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping embedded etcd integration test in short mode")
	}

	tempDir, err := os.MkdirTemp("", "kscore-cluster-integration-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	clientPort := freePort(t)
	peerPort := freePort(t)

	config := clusterpkg.DefaultConfig()
	config.Enabled = true
	config.ClusterName = "test-cluster"
	config.AdvertiseAddress = "127.0.0.1"
	config.Etcd.Mode = clusterpkg.EtcdModeEmbedded
	config.Etcd.Embedded.DataDir = filepath.Join(tempDir, "etcd")
	config.Etcd.Embedded.ClientPort = clientPort
	config.Etcd.Embedded.PeerPort = peerPort

	etcdClient, err := clusterpkg.NewEtcdClient(config.Etcd, "")
	if err != nil {
		t.Fatalf("failed to create etcd client: %v", err)
	}
	defer etcdClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := etcdClient.Connect(ctx); err != nil {
		t.Fatalf("failed to connect etcd: %v", err)
	}

	membership, err := clusterpkg.NewMembershipManager(config, etcdClient)
	if err != nil {
		t.Fatalf("failed to create membership manager: %v", err)
	}
	if err := membership.Start(ctx); err != nil {
		t.Fatalf("failed to start membership manager: %v", err)
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		_ = membership.Stop(stopCtx)
	}()

	leader, err := clusterpkg.NewLeaderElector(config, etcdClient, membership.LocalMember().ID)
	if err != nil {
		t.Fatalf("failed to create leader elector: %v", err)
	}

	health, err := clusterpkg.NewHealthMonitor(config, membership, etcdClient, membership.LocalMember().ID)
	if err != nil {
		t.Fatalf("failed to create health monitor: %v", err)
	}
	if err := health.Start(ctx); err != nil {
		t.Fatalf("failed to start health monitor: %v", err)
	}
	defer health.Stop()

	if err := helpers.WaitForTimeout(5*time.Second, 100*time.Millisecond, func() (bool, error) {
		return membership.HasQuorum() && health.HasQuorum(), nil
	}); err != nil {
		t.Fatalf("cluster did not reach quorum: %v", err)
	}

	handler := NewHandler(membership, leader, nil, health, config)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/v1/cluster/status")
	if err != nil {
		t.Fatalf("GET /cluster/status error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/cluster/status status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	resp.Body.Close()

	resp, err = http.Get(server.URL + "/api/v1/cluster/members")
	if err != nil {
		t.Fatalf("GET /cluster/members error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/cluster/members status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var membersResp []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&membersResp); err != nil {
		t.Fatalf("decode /cluster/members response: %v", err)
	}
	resp.Body.Close()

	if len(membersResp) != 1 {
		t.Fatalf("members count = %d, want 1", len(membersResp))
	}
}

func freePort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port
}
