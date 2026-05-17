package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// ShardAssignment is one agent→member mapping. Version is etcd's
// ModRevision for the key — the optimistic-concurrency token used
// by AssignIf/DeleteIf. It is etcd-owned, not part of the stored
// value.
type ShardAssignment struct {
	AgentID   string
	MemberID  string
	Version   int64
	UpdatedAt time.Time
}

// shardRecord is the JSON value stored at <prefix>/shards/<agentID>.
type shardRecord struct {
	MemberID  string    `json:"member_id"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ShardEventType classifies a shard-map change.
type ShardEventType string

const (
	ShardSet     ShardEventType = "set"
	ShardDeleted ShardEventType = "deleted"
)

// ShardEvent is delivered by Watch. For ShardDeleted, MemberID is
// the departing owner (reconstructed from etcd's prev-kv).
type ShardEvent struct {
	Type     ShardEventType
	AgentID  string
	MemberID string
	Version  int64
}

// ShardStoreConfig configures a ShardStore.
type ShardStoreConfig struct {
	Etcd      *EtcdClient
	KeyPrefix string
	Logger    *slog.Logger
}

// ShardStore persists the agent→member shard map in etcd with
// version-based optimistic concurrency. It is a stateless wrapper
// over a started EtcdClient (no own lifecycle/goroutines; methods
// propagate ErrNotStarted). It is the persistence half of
// sharding; the rebalance algorithm and the ShardManager that
// composes it with HashRing + the membership watch is Task 6.
type ShardStore struct {
	etcd   *EtcdClient
	prefix string
	log    *slog.Logger
}

// NewShardStore validates cfg and returns a store.
func NewShardStore(cfg ShardStoreConfig) (*ShardStore, error) {
	if cfg.Etcd == nil {
		return nil, fmt.Errorf("%w: Etcd client is required", ErrInvalidConfig)
	}
	if cfg.KeyPrefix == "" {
		return nil, fmt.Errorf("%w: KeyPrefix is required", ErrInvalidConfig)
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &ShardStore{etcd: cfg.Etcd, prefix: cfg.KeyPrefix, log: log}, nil
}

func (s *ShardStore) shardsPrefix() string {
	return strings.TrimRight(s.prefix, "/") + "/shards/"
}

func (s *ShardStore) shardKey(agentID string) string {
	return s.shardsPrefix() + agentID
}

func (s *ShardStore) agentFromKey(key string) string {
	return strings.TrimPrefix(key, s.shardsPrefix())
}

func marshalShard(memberID string) (string, time.Time, error) {
	now := time.Now().UTC()
	b, err := json.Marshal(shardRecord{MemberID: memberID, UpdatedAt: now})
	return string(b), now, err
}

// Assign unconditionally upserts agentID→memberID and returns the
// new assignment (Version = the post-write store revision).
func (s *ShardStore) Assign(ctx context.Context, agentID, memberID string) (ShardAssignment, error) {
	val, now, err := marshalShard(memberID)
	if err != nil {
		return ShardAssignment{}, err
	}
	txn, err := s.etcd.Txn(ctx)
	if err != nil {
		return ShardAssignment{}, err
	}
	resp, err := txn.Then(clientv3.OpPut(s.shardKey(agentID), val)).Commit()
	if err != nil {
		return ShardAssignment{}, translateError(err)
	}
	return ShardAssignment{
		AgentID:   agentID,
		MemberID:  memberID,
		Version:   resp.Header.Revision,
		UpdatedAt: now,
	}, nil
}

// AssignIf is the optimistic-CAS upsert. expectedVersion == 0 means
// create-only (the key must not exist); a positive value requires
// the current ModRevision to match. On mismatch it returns
// ErrVersionConflict (the current version is discoverable via Get).
func (s *ShardStore) AssignIf(ctx context.Context, agentID, memberID string, expectedVersion int64) (ShardAssignment, error) {
	val, now, err := marshalShard(memberID)
	if err != nil {
		return ShardAssignment{}, err
	}
	key := s.shardKey(agentID)
	var cmp clientv3.Cmp
	if expectedVersion == 0 {
		cmp = clientv3.Compare(clientv3.CreateRevision(key), "=", 0)
	} else {
		cmp = clientv3.Compare(clientv3.ModRevision(key), "=", expectedVersion)
	}
	txn, err := s.etcd.Txn(ctx)
	if err != nil {
		return ShardAssignment{}, err
	}
	resp, err := txn.If(cmp).
		Then(clientv3.OpPut(key, val)).
		Else(clientv3.OpGet(key)).
		Commit()
	if err != nil {
		return ShardAssignment{}, translateError(err)
	}
	if !resp.Succeeded {
		return ShardAssignment{}, fmt.Errorf("%w: agent %q expected version %d",
			ErrVersionConflict, agentID, expectedVersion)
	}
	return ShardAssignment{
		AgentID:   agentID,
		MemberID:  memberID,
		Version:   resp.Header.Revision,
		UpdatedAt: now,
	}, nil
}

// Get returns the assignment for agentID, or ErrShardNotFound.
func (s *ShardStore) Get(ctx context.Context, agentID string) (ShardAssignment, error) {
	resp, err := s.etcd.Get(ctx, s.shardKey(agentID))
	if err != nil {
		return ShardAssignment{}, err
	}
	if len(resp.Kvs) == 0 {
		return ShardAssignment{}, ErrShardNotFound
	}
	kv := resp.Kvs[0]
	var rec shardRecord
	if err := json.Unmarshal(kv.Value, &rec); err != nil {
		return ShardAssignment{}, fmt.Errorf("decode shard %q: %w", agentID, err)
	}
	return ShardAssignment{
		AgentID:   agentID,
		MemberID:  rec.MemberID,
		Version:   kv.ModRevision,
		UpdatedAt: rec.UpdatedAt,
	}, nil
}

// List returns the full shard map, sorted by agent ID. Malformed
// records are skipped + logged (mirroring LoadMembers).
func (s *ShardStore) List(ctx context.Context) ([]ShardAssignment, error) {
	resp, err := s.etcd.Get(ctx, s.shardsPrefix(), clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	out := make([]ShardAssignment, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var rec shardRecord
		if err := json.Unmarshal(kv.Value, &rec); err != nil {
			s.log.Warn("skipping malformed shard record", "key", string(kv.Key), "err", err)
			continue
		}
		out = append(out, ShardAssignment{
			AgentID:   s.agentFromKey(string(kv.Key)),
			MemberID:  rec.MemberID,
			Version:   kv.ModRevision,
			UpdatedAt: rec.UpdatedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AgentID < out[j].AgentID })
	return out, nil
}

// Delete removes agentID's assignment. Idempotent: deleting an
// absent assignment is a no-op returning nil.
func (s *ShardStore) Delete(ctx context.Context, agentID string) error {
	if _, err := s.etcd.Delete(ctx, s.shardKey(agentID)); err != nil {
		return translateError(err)
	}
	return nil
}

// DeleteIf is the optimistic-CAS delete. It succeeds (nil) if the
// version matches and the key is removed, OR if the key is already
// gone (the goal — absence — is met). It returns ErrVersionConflict
// only when the key exists at a different version.
func (s *ShardStore) DeleteIf(ctx context.Context, agentID string, expectedVersion int64) error {
	key := s.shardKey(agentID)
	txn, err := s.etcd.Txn(ctx)
	if err != nil {
		return err
	}
	resp, err := txn.
		If(clientv3.Compare(clientv3.ModRevision(key), "=", expectedVersion)).
		Then(clientv3.OpDelete(key)).
		Else(clientv3.OpGet(key)).
		Commit()
	if err != nil {
		return translateError(err)
	}
	if resp.Succeeded {
		return nil
	}
	// Compare failed: either the key is gone (already absent →
	// goal met) or it exists at a different version (conflict).
	getResp := resp.Responses[0].GetResponseRange()
	if getResp == nil || len(getResp.Kvs) == 0 {
		return nil
	}
	return fmt.Errorf("%w: agent %q expected version %d, have %d",
		ErrVersionConflict, agentID, expectedVersion, getResp.Kvs[0].ModRevision)
}

// Watch streams shard-map changes for agentID assignments. The
// channel closes when ctx is cancelled (or the underlying watch
// ends). It is the change feed the ShardManager (Task 6) consumes.
func (s *ShardStore) Watch(ctx context.Context) (<-chan ShardEvent, error) {
	wch, err := s.etcd.Watch(ctx, s.shardsPrefix(),
		clientv3.WithPrefix(), clientv3.WithPrevKV())
	if err != nil {
		return nil, err
	}
	out := make(chan ShardEvent, 64)
	go func() {
		defer close(out)
		for resp := range wch {
			if resp.Canceled {
				return
			}
			for _, ev := range resp.Events {
				se, ok := s.toShardEvent(ev)
				if !ok {
					continue
				}
				select {
				case out <- se:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

func (s *ShardStore) toShardEvent(ev *clientv3.Event) (ShardEvent, bool) {
	agentID := s.agentFromKey(string(ev.Kv.Key))
	if ev.Type == clientv3.EventTypeDelete {
		mem := ""
		if ev.PrevKv != nil {
			var rec shardRecord
			if json.Unmarshal(ev.PrevKv.Value, &rec) == nil {
				mem = rec.MemberID
			}
		}
		return ShardEvent{
			Type:     ShardDeleted,
			AgentID:  agentID,
			MemberID: mem,
			Version:  ev.Kv.ModRevision,
		}, true
	}
	var rec shardRecord
	if json.Unmarshal(ev.Kv.Value, &rec) != nil {
		return ShardEvent{}, false
	}
	return ShardEvent{
		Type:     ShardSet,
		AgentID:  agentID,
		MemberID: rec.MemberID,
		Version:  ev.Kv.ModRevision,
	}, true
}
