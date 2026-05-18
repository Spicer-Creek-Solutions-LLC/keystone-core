package cluster_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	icluster "go.keystone-core.io/keystone-core/internal/cluster"
	"go.keystone-core.io/keystone-core/pkg/api/cluster"
)

type fakeStatus struct{ leader string }

func (fakeStatus) ClusterName() string { return "c1" }
func (fakeStatus) Quorate() bool       { return true }
func (f fakeStatus) LeaderID() string  { return f.leader }
func (fakeStatus) Members() ([]icluster.Member, error) {
	return []icluster.Member{{ID: "m1", Status: icluster.MemberHealthy}, {ID: "m2", Status: icluster.MemberDegraded}}, nil
}

type fakeLeader struct {
	id          string
	self        bool
	transferErr error
	transferred bool
}

func (f fakeLeader) LeaderID() string { return f.id }
func (f fakeLeader) IsLeader() bool   { return f.self }
func (f *fakeLeader) TransferLeadership() error {
	f.transferred = true
	return f.transferErr
}

type fakeMembers struct{ evicted string }

func (fakeMembers) List() ([]icluster.Member, error) {
	return []icluster.Member{{ID: "m1", Name: "n1", Addr: "a1", Status: icluster.MemberHealthy}}, nil
}
func (fakeMembers) Get(id string) (icluster.Member, error) {
	if id == "m1" {
		return icluster.Member{ID: "m1", Status: icluster.MemberHealthy}, nil
	}
	return icluster.Member{}, icluster.ErrMemberNotFound
}
func (f *fakeMembers) Evict(id string) error { f.evicted = id; return nil }

type fakeRebal struct{}

func (fakeRebal) Rebalance() (int, error) { return 4, nil }

type fakeBackup struct{}

func (fakeBackup) CreateBackup() ([]byte, error) {
	return icluster.MarshalSnapshot(icluster.BuildSnapshot("c1", "m1", nil,
		[]icluster.ShardAssignment{{AgentID: "ag1", MemberID: "m1"}}, nil))
}
func (fakeBackup) RestoreBackup(b []byte, _ bool) (int, error) {
	if _, err := icluster.UnmarshalSnapshot(b); err != nil {
		return 0, err
	}
	return 1, nil
}

func newSrv(t *testing.T, p cluster.ClusterProviders) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	cluster.NewHandler(p).Register(mux)
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func do(t *testing.T, s *httptest.Server, method, path string, body []byte) (int, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, _ := http.NewRequest(method, s.URL+path, rdr)
	resp, err := s.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode, b
}

func TestCluster_NilProviders503(t *testing.T) {
	s := newSrv(t, cluster.ClusterProviders{})
	for _, p := range []string{"/api/v1/cluster/status", "/api/v1/cluster/leader", "/api/v1/cluster/members", "/api/v1/cluster/members/m1"} {
		if code, _ := do(t, s, "GET", p, nil); code != http.StatusServiceUnavailable {
			t.Errorf("GET %s = %d, want 503", p, code)
		}
	}
	if code, _ := do(t, s, "POST", "/api/v1/cluster/rebalance", nil); code != http.StatusServiceUnavailable {
		t.Errorf("rebalance nil = %d, want 503", code)
	}
}

func TestCluster_StatusMembersLeader(t *testing.T) {
	s := newSrv(t, cluster.ClusterProviders{
		Status:  fakeStatus{leader: "m1"},
		Leader:  &fakeLeader{id: "m1", self: true},
		Members: &fakeMembers{},
	})
	code, b := do(t, s, "GET", "/api/v1/cluster/status", nil)
	if code != 200 {
		t.Fatalf("status = %d", code)
	}
	var st map[string]any
	_ = json.Unmarshal(b, &st)
	if st["cluster_id"] != "c1" || st["leader_id"] != "m1" || st["healthy_count"].(float64) != 1 || st["quorum"] != true {
		t.Fatalf("status body = %s", b)
	}

	code, b = do(t, s, "GET", "/api/v1/cluster/members/m1", nil)
	if code != 200 {
		t.Fatalf("getMember = %d (%s)", code, b)
	}
	if code, _ := do(t, s, "GET", "/api/v1/cluster/members/zzz", nil); code != http.StatusNotFound {
		t.Fatalf("getMember(zzz) = %d, want 404", code)
	}
	if code, _ := do(t, s, "POST", "/api/v1/cluster/members", nil); code != http.StatusNotImplemented {
		t.Fatalf("POST members = %d, want 501", code)
	}
}

func TestCluster_TransferGuardedByLeadership(t *testing.T) {
	// Not leader → 409.
	s := newSrv(t, cluster.ClusterProviders{Leader: &fakeLeader{self: false}})
	if code, _ := do(t, s, "POST", "/api/v1/cluster/leader/transfer", nil); code != http.StatusConflict {
		t.Fatalf("transfer non-leader = %d, want 409", code)
	}
	// Leader → 200.
	lead := &fakeLeader{self: true}
	s2 := newSrv(t, cluster.ClusterProviders{Leader: lead})
	if code, _ := do(t, s2, "POST", "/api/v1/cluster/leader/transfer", nil); code != 200 {
		t.Fatalf("transfer leader = %d, want 200", code)
	}
	if !lead.transferred {
		t.Fatal("TransferLeadership not invoked")
	}
}

func TestCluster_RemoveAndRebalance(t *testing.T) {
	fm := &fakeMembers{}
	s := newSrv(t, cluster.ClusterProviders{Members: fm, Rebalance: fakeRebal{}})
	if code, _ := do(t, s, "DELETE", "/api/v1/cluster/members/m9", nil); code != http.StatusNoContent {
		t.Fatalf("DELETE member = %d, want 204", code)
	}
	if fm.evicted != "m9" {
		t.Fatalf("evicted = %q", fm.evicted)
	}
	code, b := do(t, s, "POST", "/api/v1/cluster/rebalance", nil)
	if code != 200 {
		t.Fatalf("rebalance = %d", code)
	}
	var rb map[string]any
	_ = json.Unmarshal(b, &rb)
	if rb["reassigned_agents"].(float64) != 4 {
		t.Fatalf("rebalance body = %s", b)
	}
}

func TestCluster_BackupRestoreRoundTrip(t *testing.T) {
	s := newSrv(t, cluster.ClusterProviders{Backup: fakeBackup{}})
	code, blob := do(t, s, "POST", "/api/v1/cluster/backup", nil)
	if code != 200 || len(blob) == 0 {
		t.Fatalf("backup = %d len=%d", code, len(blob))
	}
	body, _ := json.Marshal(map[string]any{"snapshot": blob, "force": true})
	code, rb := do(t, s, "POST", "/api/v1/cluster/restore", body)
	if code != 200 {
		t.Fatalf("restore = %d (%s)", code, rb)
	}
	var rr map[string]any
	_ = json.Unmarshal(rb, &rr)
	if rr["success"] != true || rr["applied"].(float64) != 1 {
		t.Fatalf("restore body = %s", rb)
	}
	// Invalid snapshot → 400.
	bad, _ := json.Marshal(map[string]any{"snapshot": []byte("garbage")})
	if code, _ := do(t, s, "POST", "/api/v1/cluster/restore", bad); code != http.StatusBadRequest {
		t.Fatalf("restore(garbage) = %d, want 400", code)
	}
}
