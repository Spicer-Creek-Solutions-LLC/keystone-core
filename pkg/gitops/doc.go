// Package gitops provides GitOps integration for Keystone Core, supporting deployment
// verification, automatic rollback, promotion pipelines, and webhook handling.
//
// The gitops package bridges GitOps tools (ArgoCD, Flux) with Keystone's runtime
// infrastructure control, enabling declarative deployments with runtime verification
// and automatic recovery capabilities.
//
// # Subpackages
//
// The gitops package is organized by functionality:
//
// Verification and Recovery:
//   - verification: Deployment verification engine with pluggable verifiers
//   - rollback: Automatic rollback engine with approval workflows
//   - remediation: Automated failure recovery workflows
//
// Promotion and Orchestration:
//   - promotion: Multi-environment promotion pipelines with quality gates
//   - deployment: Dependency ordering and sequencing
//   - orchestration: Multi-repository coordination
//
// GitOps Providers:
//   - argocd: ArgoCD client and integration
//   - flux: Flux client and integration
//   - github: GitHub API client and webhook parsing
//   - gitlab: GitLab API client and webhook parsing
//   - gitsync: Git repository synchronization utilities
//
// Webhook Handling:
//   - webhook: Webhook handler registry with provider auto-detection
//
// # Verification Engine
//
// The verification subpackage orchestrates deployment verification workflows:
//
//	import "github.com/your-org/keystone-core/pkg/gitops/verification"
//
//	engine := verification.NewEngine(cfg)
//	result, err := engine.Execute(ctx, workflow)
//
// Verification workflows support parallel and sequential step execution,
// optional steps, continue-on-failure behavior, and pluggable verifiers
// (HTTP health checks, Kubernetes readiness, custom commands).
//
// # Rollback Engine
//
// The rollback subpackage handles automatic and manual rollback operations:
//
//	import "github.com/your-org/keystone-core/pkg/gitops/rollback"
//
//	engine := rollback.NewEngine(executor, approvalWorkflow)
//	result, err := engine.Rollback(ctx, request)
//
// Rollback supports approval gates, ArgoCD and Git-based executors, and
// automatic triggering on verification failures.
//
// # Promotion Pipelines
//
// The promotion subpackage manages multi-stage deployments:
//
//	import "github.com/your-org/keystone-core/pkg/gitops/promotion"
//
//	engine := promotion.NewEngine(deployer, remediator, notifier, thresholds)
//	result, err := engine.Promote(ctx, pipeline)
//
// Pipelines support canary deployments, traffic weighting, quality thresholds,
// and automatic remediation (rollback, scale adjustments, traffic shifting).
//
// # Webhook Handling
//
// The webhook subpackage provides unified handling for multiple Git providers:
//
//	import "github.com/your-org/keystone-core/pkg/gitops/webhook"
//
//	registry := webhook.NewHandlerRegistry()
//	registry.Register("github", githubHandler)
//	event, err := registry.Parse(r)  // Auto-detects provider
package gitops
