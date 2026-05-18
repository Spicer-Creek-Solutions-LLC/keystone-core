package cluster

import (
	"context"
	"errors"

	"go.etcd.io/etcd/api/v3/v3rpc/rpctypes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Sentinel errors for the cluster package. The set is intentionally
// small at Task 1 — it grows as later tasks add membership /
// election / fencing failure modes. Callers compare with
// errors.Is.
var (
	// ErrInvalidConfig is returned by NewEtcdClient when the
	// supplied EtcdConfig is structurally invalid.
	ErrInvalidConfig = errors.New("cluster: invalid etcd config")

	// ErrNotStarted is returned by operations that require a
	// started client (Start not called, or already Stopped).
	ErrNotStarted = errors.New("cluster: etcd client not started")

	// ErrAlreadyStarted is returned by Start when the client is
	// already running.
	ErrAlreadyStarted = errors.New("cluster: etcd client already started")

	// ErrStopped is returned when Start is called after Stop;
	// an EtcdClient is single-use.
	ErrStopped = errors.New("cluster: etcd client already stopped")

	// ErrEtcdUnavailable is returned when the embedded server
	// fails to become ready, or an external cluster cannot be
	// reached within the dial/start timeout.
	ErrEtcdUnavailable = errors.New("cluster: etcd unavailable")

	// ErrLeaseNotFound is returned when a lease ID is unknown to
	// etcd (expired or never granted).
	ErrLeaseNotFound = errors.New("cluster: lease not found")

	// ErrMemberNotFound is returned by MembershipManager.GetMember
	// when no member record exists for the requested ID.
	ErrMemberNotFound = errors.New("cluster: member not found")

	// ErrNotRegistered is returned by membership operations that
	// require Register to have been called first.
	ErrNotRegistered = errors.New("cluster: not registered")

	// ErrInvalidTransition is returned when a member status change
	// is not a permitted edge of the lifecycle state machine.
	ErrInvalidTransition = errors.New("cluster: invalid member status transition")

	// ErrNoLeader is returned by LeaderElector.LeaderID when the
	// election currently has no leader.
	ErrNoLeader = errors.New("cluster: no leader")

	// ErrShardNotFound is returned by ShardStore.Get when no
	// assignment exists for the requested agent.
	ErrShardNotFound = errors.New("cluster: shard assignment not found")

	// ErrVersionConflict is returned by the ShardStore optimistic
	// CAS operations (AssignIf/DeleteIf) when the stored version
	// no longer matches the expected one.
	ErrVersionConflict = errors.New("cluster: shard version conflict")

	// ErrNoEligibleMembers is returned by ShardManager when the
	// ring is empty (no HEALTHY/DEGRADED members), so an agent
	// cannot be assigned an owner.
	ErrNoEligibleMembers = errors.New("cluster: no eligible members")

	// ErrFenced is returned by FencingManager.Guard when an
	// operation is blocked because this node is fenced (in a
	// minority partition or running under a stale epoch).
	ErrFenced = errors.New("cluster: operation fenced (split-brain protection)")
)

// isLeaseNotFound reports whether err is etcd's "lease not found".
// rpctypes.ErrLeaseNotFound is an EtcdError without an Is method, so
// errors.Is against it is unreliable across the gRPC boundary; the
// gRPC NotFound status code is the robust signal for the lease ops
// Task 1 exercises (Revoke is the only NotFound-bearing call here).
func isLeaseNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, rpctypes.ErrLeaseNotFound) {
		return true
	}
	return status.Code(err) == codes.NotFound
}

// translateError maps a raw etcd/gRPC error onto the package's
// sentinel family where a stable classification exists, and passes
// context cancellation through unwrapped so callers' select loops
// keep working. Unclassified errors are returned wrapped so the
// original cause is still inspectable. The mapping is deliberately
// thin at Task 1 and expands with the membership/fencing tasks.
func translateError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case isLeaseNotFound(err):
		return ErrLeaseNotFound
	default:
		return err
	}
}
