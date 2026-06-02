// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"net/http"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Fencer authorises a request against the cluster split-brain fencing
// layer (Epic 13 task 11). A node that has lost etcd quorum or been
// deposed as leader self-fences; Guard then rejects writes (and, in
// strict mode, reads) per the configured fence mode.
//
// Guard returns a release func that the caller MUST invoke when the
// operation completes (it tracks in-flight operations so graceful
// shutdown can drain them); on a fenced operation it returns a nil
// release and a non-nil error. nil Fencer ⇒ no fencing (the
// single-node / clustering-disabled path).
//
// pkg/api/server stays free of internal/cluster: cmd/kscore-server
// injects an adapter over the cluster FencingManager that maps the
// write bool onto the cluster OpType.
type Fencer interface {
	Guard(write bool) (release func(), err error)
}

// fencedMessage is the rejection surfaced to callers (gRPC
// Unavailable / HTTP 503) when this node is fenced. It is deliberately
// generic — a fenced node should not leak quorum/topology detail to
// arbitrary callers.
const fencedMessage = "node fenced: cluster quorum lost or leadership superseded (split-brain protection)"

// writeMethods is the set of fully-qualified gRPC methods that mutate
// shared cluster state and are therefore guarded as writes. Methods
// absent from this set are treated as reads, so they continue to be
// served by a read-only-fenced minority node (the §4.15 "reads
// continue" contract); correctness for any unlisted write is still
// backstopped by etcd quorum at the storage layer. CoordinationService
// is intentionally absent — it runs on its own listener (never this
// chain) and is the recovery channel that must work during a
// partition.
var writeMethods = map[string]bool{
	"/keystone.core.v1.StateService/ApplyState":                 true,
	"/keystone.core.v1.ControlPlaneService/ExecuteCommand":      true,
	"/keystone.core.v1.ControlPlaneService/BatchExecuteCommand": true,
	"/keystone.core.v1.EventService/EmitEvent":                  true,
	"/keystone.core.v1.SecretsService/WriteSecret":              true,
	"/keystone.core.v1.SecretsService/DeleteSecret":             true,
	"/keystone.core.v1.SecretsService/RenewLease":               true,
	"/keystone.core.v1.SecretsService/RevokeLease":              true,
	"/keystone.core.v1.PolicyService/EvaluatePolicy":            true,
	"/keystone.core.v1.PolicyService/EvaluatePolicySet":         true,
	"/keystone.core.v1.ClusterService/AddMember":                true,
	"/keystone.core.v1.ClusterService/RemoveMember":             true,
	"/keystone.core.v1.ClusterService/TransferLeader":           true,
	"/keystone.core.v1.ClusterService/Rebalance":                true,
	"/keystone.core.v1.ClusterService/CreateBackup":             true,
	"/keystone.core.v1.ClusterService/RestoreBackup":            true,
}

// isWriteMethod reports whether a fully-qualified gRPC method mutates
// shared state (and so must be fenced as a write).
func isWriteMethod(fullMethod string) bool { return writeMethods[fullMethod] }

// fencingUnaryInterceptor guards every unary RPC with the Fencer,
// rejecting a fenced operation with codes.Unavailable. The guard is
// held for the duration of the handler so graceful-shutdown drain
// sees the in-flight request.
func fencingUnaryInterceptor(f Fencer) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		release, err := f.Guard(isWriteMethod(info.FullMethod))
		if err != nil {
			return nil, status.Error(codes.Unavailable, fencedMessage)
		}
		if release != nil {
			defer release()
		}
		return handler(ctx, req)
	}
}

// fencingStreamInterceptor guards every streaming RPC. Streams are
// classified by method like unary calls (the cluster watch/subscribe
// streams are reads); the guard is held for the stream's lifetime.
func fencingStreamInterceptor(f Fencer) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		release, err := f.Guard(isWriteMethod(info.FullMethod))
		if err != nil {
			return status.Error(codes.Unavailable, fencedMessage)
		}
		if release != nil {
			defer release()
		}
		return handler(srv, ss)
	}
}

// fencingHTTPMiddleware guards the REST API surface: any request whose
// method is not a safe/read verb (GET/HEAD/OPTIONS) is a write and is
// rejected with 503 when this node is fenced. The guard is held for
// the request's duration.
func fencingHTTPMiddleware(f Fencer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			release, err := f.Guard(isHTTPWrite(r.Method))
			if err != nil {
				http.Error(w, fencedMessage, http.StatusServiceUnavailable)
				return
			}
			if release != nil {
				defer release()
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isHTTPWrite reports whether an HTTP method mutates state. GET, HEAD
// and OPTIONS are the safe/read verbs; everything else (POST, PUT,
// PATCH, DELETE, …) is a write.
func isHTTPWrite(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}
