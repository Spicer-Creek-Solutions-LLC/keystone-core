// SPDX-License-Identifier: Apache-2.0

package rollback

import (
	"context"
	"time"
)

// ArgoHistoryEntry is one ArgoCD deployment-history row.
type ArgoHistoryEntry struct {
	ID       int64
	Revision string
}

// ArgoApp is the slice of an ArgoCD application this package needs.
type ArgoApp struct {
	Name string
	// SyncRevision is the currently-synced revision.
	SyncRevision string
	// History is oldest→newest deployment history.
	History []ArgoHistoryEntry
}

// ArgoClient is the seam the ArgoCD executor needs. The concrete
// implementation (argoexec, stdlib REST) lives in the argoexec
// subpackage — no argo-cd/v3 dependency.
type ArgoClient interface {
	// GetApplication returns the app's current revision + history.
	GetApplication(ctx context.Context, name string) (ArgoApp, error)
	// SyncToRevision syncs the app to revision.
	SyncToRevision(ctx context.Context, name, revision string) error
}

// ArgoCDExecutor rolls back by syncing an ArgoCD application to a
// prior revision. Config: app (optional; defaults to
// [Request.Application]).
type ArgoCDExecutor struct {
	Client ArgoClient
}

// Type implements [Executor].
func (ArgoCDExecutor) Type() string { return "argocd" }

func (e ArgoCDExecutor) appName(cfg Config, req Request) string {
	return cfgStringOpt(cfg, "app", req.Application)
}

// Execute implements [Executor].
func (e ArgoCDExecutor) Execute(ctx context.Context, cfg Config, req Request) Result {
	start := time.Now()
	if e.Client == nil {
		return failf(start, ErrNotConfigured, "argocd: no client configured")
	}
	name := e.appName(cfg, req)
	if name == "" {
		return failf(start, ErrConfig, "argocd: application name is required")
	}
	app, err := e.Client.GetApplication(ctx, name)
	if err != nil {
		return failf(start, err, "argocd: get application: %v", err)
	}
	target, err := resolveTarget(ctx, e, cfg, req)
	if err != nil {
		return failf(start, err, "argocd: resolve target: %v", err)
	}
	if err := e.Client.SyncToRevision(ctx, name, target); err != nil {
		return failf(start, err, "argocd: sync to %s failed: %v", target, err)
	}
	return Result{
		Success:      true,
		Message:      "argocd: synced " + name + " to " + target,
		FromRevision: app.SyncRevision,
		ToRevision:   target,
		Data:         map[string]any{"application": name},
		Duration:     time.Since(start),
	}
}

// GetPreviousRevision implements [Executor]: the revision of the
// second-newest history entry (the one before the current deploy).
func (e ArgoCDExecutor) GetPreviousRevision(ctx context.Context, cfg Config, req Request) (string, error) {
	if e.Client == nil {
		return "", ErrNotConfigured
	}
	app, err := e.Client.GetApplication(ctx, e.appName(cfg, req))
	if err != nil {
		return "", err
	}
	if len(app.History) < 2 {
		return "", ErrConfig
	}
	return app.History[len(app.History)-2].Revision, nil
}

// GetLastKnownGood implements [Executor]. v1.0 best-effort: the same
// as the previous revision (provider history has no verification
// signal yet — that lands with task-9 persistence).
func (e ArgoCDExecutor) GetLastKnownGood(ctx context.Context, cfg Config, req Request) (string, error) {
	return e.GetPreviousRevision(ctx, cfg, req)
}
