// Package api provides gRPC and REST API endpoints for the Keystone Core control plane.
// NOTE: API/ABI is not finalized and may change without notice.
//
// The api package implements handlers for agent management, remote command execution,
// state management, policy enforcement, events, GitOps operations, and webhook handling.
// It serves as the primary interface between clients (CLI tools, external systems) and
// the control plane backend.
//
// # Subpackages
//
// The api package is organized into subpackages by domain:
//
//   - v1: Protocol Buffer definitions for gRPC services and messages
//   - server: gRPC server implementation (ControlPlaneServer)
//   - agents: Agent REST API handlers (list, show, delete, tags)
//   - execution: Remote command execution REST API
//   - state: State management REST API (apply, check, diff)
//   - policy: Policy enforcement REST API
//   - events: Event streaming REST API
//   - gitops: GitOps integration REST API (verification, rollback)
//   - webhooks: Webhook ingestion handlers
//   - auth: Authentication and authorization (JWT, mTLS, rate limiting)
//   - cluster: High-availability cluster coordination
//   - versioning: API version management utilities
//
// # Usage
//
// API handlers are typically registered with a router during server startup:
//
//	import "github.com/your-org/keystone-core/pkg/api/agents"
//
//	handler := agents.NewHandler(agentStore, logger)
//	handler.RegisterRoutes(mux)
//
// # Authentication
//
// The auth subpackage provides middleware for:
//   - JWT token validation
//   - mTLS certificate authentication
//   - Rate limiting per client
//   - Authorization interceptors for gRPC
//
// # Protocol Buffers
//
// The v1 subpackage contains generated Protocol Buffer types for gRPC services.
// Key services include ControlPlaneService for agent management and command execution,
// and CoordinationService for cluster operations.
package api
