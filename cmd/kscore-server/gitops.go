// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/events"
	"go.keystone-core.io/keystone-core/internal/gitops/rollback"
	"go.keystone-core.io/keystone-core/internal/gitops/rollback/gitexec"
	"go.keystone-core.io/keystone-core/internal/gitops/verification"
	gitopswh "go.keystone-core.io/keystone-core/internal/gitops/webhook"
	"go.keystone-core.io/keystone-core/pkg/api/gitops"
)

// gitOpsRuntime carries the live GitOps infrastructure constructed at
// boot — the rollback engine + verification store + webhook receiver.
// Each piece is independently nilable: rollback can be on with the
// receiver off, or vice versa.
type gitOpsRuntime struct {
	// Rollback engine + its durable store + the verification result
	// store. Nil when the operator hasn't opted in (v1.0 keeps
	// rollback off-by-default until a real ROLES path lands).
	Rollback         *rollback.Engine
	RollbackStore    *rollback.SQLiteStore
	Verifications    verification.ResultStore
	VerificationsDB  *verification.SQLiteResultStore

	// Webhook receiver (separate :8081/webhooks listener). Nil when
	// gitops.webhook.enabled is false.
	WebhookReceiver *gitopswh.Receiver
}

func (r *gitOpsRuntime) stop(ctx context.Context, log *slog.Logger) {
	if r == nil {
		return
	}
	if r.WebhookReceiver != nil {
		if err := r.WebhookReceiver.Stop(ctx); err != nil {
			log.LogAttrs(ctx, slog.LevelWarn, "gitops webhook: stop", slog.String("err", err.Error()))
		}
	}
	if r.RollbackStore != nil {
		if err := r.RollbackStore.Close(); err != nil {
			log.LogAttrs(ctx, slog.LevelWarn, "gitops rollback store: close", slog.String("err", err.Error()))
		}
	}
	if r.VerificationsDB != nil {
		if err := r.VerificationsDB.Close(); err != nil {
			log.LogAttrs(ctx, slog.LevelWarn, "gitops verification store: close", slog.String("err", err.Error()))
		}
	}
}

// startGitOps wires the rollback engine + verification store + the
// inbound webhook receiver. The rollback engine is always constructed
// (rollback REST routes are operator-facing and gated behind admin
// API-key auth) but the webhook receiver only starts when
// cfg.GitOps.Webhook.Enabled. The git-revert executor is registered
// unconditionally — the ArgoCD executor + K8s executor stay deferred
// (ArgoCD needs a real server; K8s pulls k8s.io/client-go per the
// gate-v1.0 ROADMAP entry "K8s rollout-undo client-go adapter").
func startGitOps(
	ctx context.Context,
	cfg config.GitOpsConfig,
	publisher events.EventPublisher,
	log *slog.Logger,
) (*gitOpsRuntime, error) {
	// Gate the entire gitops runtime on the same flag the operator
	// uses to opt in to GitOps features. Keeps unit tests + minimal
	// dev-mode boots from creating SQLite files on disk.
	if !cfg.Webhook.Enabled {
		return nil, nil
	}
	rt := &gitOpsRuntime{}

	if err := os.MkdirAll("data", 0o750); err != nil {
		return nil, fmt.Errorf("gitops: data dir: %w", err)
	}

	// Rollback engine + stores.
	rbStore, err := rollback.NewSQLiteStore(filepath.Join("data", "rollback.db"))
	if err != nil {
		return nil, fmt.Errorf("gitops rollback: store: %w", err)
	}
	vStore, err := verification.NewSQLiteResultStore(filepath.Join("data", "verifications.db"))
	if err != nil {
		_ = rbStore.Close()
		return nil, fmt.Errorf("gitops verification: store: %w", err)
	}
	engine := rollback.NewEngine(rbStore)
	// Register only the git-revert executor for v1.0 — ArgoCD needs a
	// real server (deferred to v1.x); K8s needs k8s.io/client-go (the
	// gate-v1.0 ROADMAP item "K8s rollout-undo client-go adapter").
	if err := engine.RegisterExecutor(rollback.GitRevertExecutor{Client: &gitexec.Client{}}); err != nil {
		_ = rbStore.Close()
		_ = vStore.Close()
		return nil, fmt.Errorf("gitops rollback: register git executor: %w", err)
	}
	rt.Rollback = engine
	rt.RollbackStore = rbStore
	rt.VerificationsDB = vStore
	rt.Verifications = vStore

	// Webhook receiver (operator opt-in).
	if cfg.Webhook.Enabled {
		specs, err := authSpecsFromConfig(cfg.Webhook.Sources)
		if err != nil {
			rt.stop(ctx, log)
			return nil, fmt.Errorf("gitops webhook: auth specs: %w", err)
		}
		auths, err := gitopswh.BuildAuthenticators(specs)
		if err != nil {
			rt.stop(ctx, log)
			return nil, fmt.Errorf("gitops webhook: build authenticators: %w", err)
		}
		recv := gitopswh.New(gitopswh.ReceiverConfig{
			Addr:           cfg.Webhook.Addr,
			Path:           cfg.Webhook.Path,
			MaxBodyBytes:   cfg.Webhook.MaxBodyBytes,
			Authenticators: auths,
			Emitter:        publisher,
		}, gitopswh.NewDefaultRegistry(), log)
		if err := recv.Start(ctx); err != nil {
			rt.stop(ctx, log)
			return nil, fmt.Errorf("gitops webhook: start: %w", err)
		}
		rt.WebhookReceiver = recv
		log.Info("gitops webhook receiver started", "addr", recv.Addr(), "path", cfg.Webhook.Path)
	}
	return rt, nil
}

// authSpecsFromConfig translates the koanf-side
// config.GitOpsSourceAuthConfig map onto the package-local
// webhook.AuthSpec the receiver consumes. The mapping is mechanical;
// keeping it inside cmd/kscore-server keeps the gitops/webhook
// package free of internal/config imports.
func authSpecsFromConfig(in map[string]config.GitOpsSourceAuthConfig) (map[gitopswh.Provider]gitopswh.AuthSpec, error) {
	out := make(map[gitopswh.Provider]gitopswh.AuthSpec, len(in))
	for src, a := range in {
		prov, err := providerFromString(src)
		if err != nil {
			return nil, err
		}
		out[prov] = gitopswh.AuthSpec{
			Method:          gitopswh.AuthMethod(a.Method),
			Secret:          a.Secret,
			SignatureHeader: a.SignatureHeader,
			HeaderPrefix:    a.HeaderPrefix,
			RequireScheme:   a.RequireScheme,
		}
	}
	return out, nil
}

func providerFromString(s string) (gitopswh.Provider, error) {
	switch s {
	case "github":
		return gitopswh.ProviderGitHub, nil
	case "gitlab":
		return gitopswh.ProviderGitLab, nil
	case "argocd":
		return gitopswh.ProviderArgoCD, nil
	case "flux":
		return gitopswh.ProviderFlux, nil
	}
	return "", fmt.Errorf("gitops webhook: unknown provider %q", s)
}

// providersFrom builds the REST handler's Providers from a started
// runtime. The rollback engine is always wired when rt != nil; the
// verification result store likewise.
func (r *gitOpsRuntime) providersFrom() gitops.Providers {
	if r == nil {
		return gitops.Providers{}
	}
	return gitops.Providers{
		Rollback:      r.Rollback,
		Verifications: r.Verifications,
	}
}

