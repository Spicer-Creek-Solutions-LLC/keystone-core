// SPDX-License-Identifier: Apache-2.0

package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
)

func TestSnapshot_MarshalUnmarshalRoundTrip(t *testing.T) {
	snap := BuildSnapshot("c1", "leader-9",
		[]Member{{ID: "m1", Name: "n1", Addr: "a1", Status: MemberHealthy}},
		[]ShardAssignment{{AgentID: "ag1", MemberID: "m1", Version: 3}},
		json.RawMessage(`{"k":"v"}`),
	)
	blob, err := MarshalSnapshot(snap)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := UnmarshalSnapshot(blob)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Meta.ClusterName != "c1" || got.Meta.LeaderID != "leader-9" {
		t.Fatalf("meta = %+v", got.Meta)
	}
	if len(got.Members) != 1 || got.Members[0].ID != "m1" {
		t.Fatalf("members = %+v", got.Members)
	}
	if len(got.Shards) != 1 || got.Shards[0].Version != 3 {
		t.Fatalf("shards = %+v", got.Shards)
	}
	if string(got.ConfigJSON) != `{"k":"v"}` {
		t.Fatalf("config = %s", got.ConfigJSON)
	}
}

func TestSnapshot_EnvelopeValidation(t *testing.T) {
	good, _ := MarshalSnapshot(BuildSnapshot("c", "", nil, nil, nil))

	if _, err := UnmarshalSnapshot([]byte("short")); !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("short = %v, want ErrInvalidSnapshot", err)
	}
	badMagic := append([]byte(nil), good...)
	badMagic[0] = 'X'
	if _, err := UnmarshalSnapshot(badMagic); !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("bad magic = %v, want ErrInvalidSnapshot", err)
	}
	badVer := append([]byte(nil), good...)
	badVer[16] = 99
	if _, err := UnmarshalSnapshot(badVer); !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("bad version = %v, want ErrInvalidSnapshot", err)
	}
	badJSON := append(append([]byte(nil), good[:17]...), []byte("not json")...)
	if _, err := UnmarshalSnapshot(badJSON); !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("bad body = %v, want ErrInvalidSnapshot", err)
	}
}

type fakeRestoreStore struct {
	mu       sync.Mutex
	assigned map[string]string
	existing map[string]bool // agents that already have an assignment
}

func newFakeRestoreStore(existing ...string) *fakeRestoreStore {
	e := map[string]bool{}
	for _, a := range existing {
		e[a] = true
	}
	return &fakeRestoreStore{assigned: map[string]string{}, existing: e}
}

func (f *fakeRestoreStore) Assign(_ context.Context, agentID, memberID string) (ShardAssignment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.assigned[agentID] = memberID
	return ShardAssignment{AgentID: agentID, MemberID: memberID, Version: 1}, nil
}

func (f *fakeRestoreStore) AssignIf(_ context.Context, agentID, memberID string, _ int64) (ShardAssignment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.existing[agentID] {
		return ShardAssignment{}, ErrVersionConflict // create-only: present → conflict
	}
	f.assigned[agentID] = memberID
	return ShardAssignment{AgentID: agentID, MemberID: memberID, Version: 1}, nil
}

func TestRestoreShards_ForceOverwritesAll(t *testing.T) {
	st := newFakeRestoreStore("ag1") // ag1 already exists
	snap := &ClusterSnapshot{Shards: []ShardAssignment{
		{AgentID: "ag1", MemberID: "m2"}, {AgentID: "ag2", MemberID: "m3"},
	}}
	n, err := RestoreShards(context.Background(), st, snap, true)
	if err != nil || n != 2 {
		t.Fatalf("force restore = %d, %v; want 2, nil", n, err)
	}
	if st.assigned["ag1"] != "m2" || st.assigned["ag2"] != "m3" {
		t.Fatalf("assigned = %v", st.assigned)
	}
}

func TestRestoreShards_NoForcePreservesExisting(t *testing.T) {
	st := newFakeRestoreStore("ag1") // ag1 already assigned → must be preserved
	snap := &ClusterSnapshot{Shards: []ShardAssignment{
		{AgentID: "ag1", MemberID: "m2"}, {AgentID: "ag2", MemberID: "m3"},
	}}
	n, err := RestoreShards(context.Background(), st, snap, false)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if n != 1 { // only ag2 created; ag1 conflict skipped
		t.Fatalf("no-force restore applied %d, want 1", n)
	}
	if _, ok := st.assigned["ag1"]; ok {
		t.Fatalf("ag1 must be preserved (not overwritten): %v", st.assigned)
	}
	if st.assigned["ag2"] != "m3" {
		t.Fatalf("ag2 not restored: %v", st.assigned)
	}
}
