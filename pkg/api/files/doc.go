// Package files exposes REST routes for the file-distribution
// domain (Epic 18 task 14). Routes:
//
//	GET    /api/v1/files                       list (?prefix=...)
//	GET    /api/v1/files/metadata/{path...}    metadata only (Stat)
//	GET    /api/v1/files/{path...}             body bytes
//	PUT    /api/v1/files/{path...}             upload body
//	DELETE /api/v1/files/{path...}             delete
//
// The handler talks directly to the [backend.Store] passed at
// construction — no NATS hop, no transport.Client wrapper. The
// equivalent surface over NATS is internal/files/transport (Task
// 11); both share the same backend so REST and bus-based access
// converge on a single source of truth.
//
// Authentication is upstream (auth interceptor / middleware sets
// the [*auth.Principal] on the request context); this package
// reads it via [auth.PrincipalFromContext] and feeds it into the
// optional [acl.ACL] for per-namespace authorization. A nil ACL
// is allowed and means "no gating" — operator deployments wire
// an ACL at boot.
//
// Disabled state: when the backing store is nil, every route
// returns 503 Service Unavailable. This matches the
// pkg/api/secrets handler convention.
//
// v1.x:
//
//	HTTP Range support on GET, pagination on LIST, gRPC stubs.
//	Not in scope for v1.0; v1.x adds them as operator demand
//	justifies the wire-format / API expansion.
package files
