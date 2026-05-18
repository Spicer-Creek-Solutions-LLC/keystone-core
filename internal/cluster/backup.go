package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Cluster backup format: a small binary envelope wrapping a JSON
// body — magic (16B) + format version (1B) + JSON(ClusterSnapshot)
// — mirroring the internal/secrets/file envelope precedent so the
// blob is self-describing, version-gated and tamper-evident on the
// header.
var snapshotMagic = [16]byte{'K', 'S', 'C', 'O', 'R', 'E', '-', 'C', 'L', 'U', 'S', 'T', 'E', 'R', 0, 0}

const snapshotFormatV1 byte = 1

// ClusterSnapshotMeta is the backup header metadata.
type ClusterSnapshotMeta struct {
	ClusterName string    `json:"cluster_name"`
	TakenAt     time.Time `json:"taken_at"`
	LeaderID    string    `json:"leader_id"`
}

// ClusterSnapshot is the backed-up cluster state: metadata, the
// observed member set, the shard assignments, and an opaque
// operator-config blob. Members are informational only (they are
// ephemeral / self-registered and are NOT restored).
type ClusterSnapshot struct {
	Meta       ClusterSnapshotMeta `json:"meta"`
	Members    []Member            `json:"members"`
	Shards     []ShardAssignment   `json:"shards"`
	ConfigJSON json.RawMessage     `json:"config_json,omitempty"`
}

// MarshalSnapshot encodes s as a versioned binary envelope.
func MarshalSnapshot(s *ClusterSnapshot) ([]byte, error) {
	body, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot: %w", err)
	}
	out := make([]byte, 0, len(snapshotMagic)+1+len(body))
	out = append(out, snapshotMagic[:]...)
	out = append(out, snapshotFormatV1)
	out = append(out, body...)
	return out, nil
}

// UnmarshalSnapshot validates the envelope and decodes the body.
func UnmarshalSnapshot(b []byte) (*ClusterSnapshot, error) {
	const hdr = len(snapshotMagic) + 1
	if len(b) < hdr {
		return nil, fmt.Errorf("%w: too short", ErrInvalidSnapshot)
	}
	if [16]byte(b[:16]) != snapshotMagic {
		return nil, fmt.Errorf("%w: bad magic", ErrInvalidSnapshot)
	}
	if b[16] != snapshotFormatV1 {
		return nil, fmt.Errorf("%w: unknown format version %d", ErrInvalidSnapshot, b[16])
	}
	var s ClusterSnapshot
	if err := json.Unmarshal(b[hdr:], &s); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSnapshot, err)
	}
	return &s, nil
}

// BuildSnapshot assembles a ClusterSnapshot from the current
// cluster state.
func BuildSnapshot(clusterName, leaderID string, members []Member, shards []ShardAssignment, configJSON []byte) *ClusterSnapshot {
	return &ClusterSnapshot{
		Meta: ClusterSnapshotMeta{
			ClusterName: clusterName,
			TakenAt:     time.Now().UTC(),
			LeaderID:    leaderID,
		},
		Members:    members,
		Shards:     shards,
		ConfigJSON: configJSON,
	}
}

// shardRestorer is the slice of ShardStore restore needs.
// *ShardStore satisfies it.
type shardRestorer interface {
	Assign(ctx context.Context, agentID, memberID string) (ShardAssignment, error)
	AssignIf(ctx context.Context, agentID, memberID string, expectedVersion int64) (ShardAssignment, error)
}

// RestoreShards re-applies the snapshot's shard map. With force the
// assignments overwrite unconditionally; without force only
// absent agents are created (existing assignments are preserved —
// version conflicts are skipped). Members/config are not restored
// (ephemeral / operator concern). Returns the number applied.
func RestoreShards(ctx context.Context, store shardRestorer, snap *ClusterSnapshot, force bool) (int, error) {
	applied := 0
	for _, a := range snap.Shards {
		if force {
			if _, err := store.Assign(ctx, a.AgentID, a.MemberID); err != nil {
				return applied, fmt.Errorf("restore %q: %w", a.AgentID, err)
			}
			applied++
			continue
		}
		if _, err := store.AssignIf(ctx, a.AgentID, a.MemberID, 0); err != nil {
			if errors.Is(err, ErrVersionConflict) {
				continue // already assigned; preserve without force
			}
			return applied, fmt.Errorf("restore %q: %w", a.AgentID, err)
		}
		applied++
	}
	return applied, nil
}
