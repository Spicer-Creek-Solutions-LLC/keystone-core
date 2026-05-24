// SPDX-License-Identifier: Apache-2.0

// Package acl is the namespace-based access-control layer for
// file distribution. It sits between the inbound transport
// request handler and the [backend.Store]: every Get / Put /
// List / Delete is gated by an [ACL.Authorize] call that takes
// the authenticated [auth.Principal], the operation, and the
// path's namespace (leading path segment, see [files.Namespace]).
//
// v1.0 ships one concrete impl: [RoleACL] — per-(namespace,
// operation) minimum-role rules with an explicit default policy
// for unlisted namespaces. The role hierarchy is the Epic 03 v1.0
// 3-role set (admin > operator > readonly).
//
// Threat model and trust boundary:
//
//	The transport layer extracts the caller's identity from NATS
//	message headers (Kscore-Principal-Id, Kscore-Principal-Role).
//	Those headers are trusted because the NATS connection itself
//	is authenticated — operators bring NATS user auth or mTLS
//	(Epic 03). Header-supplied identity is NOT a substitute for
//	connection-layer auth; it is the layer above it. An untrusted
//	or anonymous NATS connection sees a zero-value Principal and
//	is subject to the ACL's default policy (typically deny).
//
// post-v1.0:
//
//	Path-pattern rules (configs/* glob matching), per-principal
//	bindings (CRUD), allowlist/denylist mode flips, and Vault-
//	style policy templating land alongside the broader Epic-03
//	post-v1.0 RBAC expansion.
package acl
