package rollback

import (
	"context"
	"strconv"
	"time"
)

// K8sRevision is one entry of a Deployment's rollout history
// (ReplicaSet revision annotation).
type K8sRevision struct {
	Revision int64
}

// K8sRolloutClient is the seam the Kubernetes executor needs. The
// concrete client-go adapter is intentionally deferred (it would pull
// k8s.io/client-go + api + apimachinery) and wired at kscore-server /
// kscore-gitops boot — see the gate-v1.0 ROADMAP entry. Until then a
// nil client makes the executor fail with [ErrNotConfigured].
type K8sRolloutClient interface {
	// RevisionHistory returns the Deployment's rollout revisions,
	// oldest→newest.
	RevisionHistory(ctx context.Context, namespace, deployment string) ([]K8sRevision, error)
	// RolloutUndo rolls the Deployment back to toRevision (0 = the
	// immediately previous revision, matching `kubectl rollout undo`).
	RolloutUndo(ctx context.Context, namespace, deployment string, toRevision int64) error
}

// K8sRolloutExecutor rolls back a Deployment via rollout-undo.
// Config: namespace (default "default"), deployment (required).
type K8sRolloutExecutor struct {
	Client K8sRolloutClient
}

// Type implements [Executor].
func (K8sRolloutExecutor) Type() string { return "k8s" }

func k8sCfg(cfg Config) (namespace, deployment string, err error) {
	deployment, err = cfgString(cfg, "deployment")
	if err != nil {
		return "", "", err
	}
	return cfgStringOpt(cfg, "namespace", "default"), deployment, nil
}

// Execute implements [Executor]. StrategyPrevious maps to undo-to-0
// (kubectl semantics); StrategySpecific/LastKnownGood resolve a
// concrete revision number first.
func (e K8sRolloutExecutor) Execute(ctx context.Context, cfg Config, req Request) Result {
	start := time.Now()
	if e.Client == nil {
		return failf(start, ErrNotConfigured, "k8s: no client configured (client-go adapter deferred to boot)")
	}
	ns, dep, err := k8sCfg(cfg)
	if err != nil {
		return failf(start, err, "k8s: %v", err)
	}

	var toRev int64 // 0 = previous (kubectl rollout undo default)
	if req.Strategy != StrategyPrevious {
		target, terr := resolveTarget(ctx, e, cfg, req)
		if terr != nil {
			return failf(start, terr, "k8s: resolve target: %v", terr)
		}
		n, perr := strconv.ParseInt(target, 10, 64)
		if perr != nil {
			return failf(start, ErrConfig, "k8s: revision %q is not an integer", target)
		}
		toRev = n
	}

	if err := e.Client.RolloutUndo(ctx, ns, dep, toRev); err != nil {
		return failf(start, err, "k8s: rollout undo failed: %v", err)
	}
	to := "previous"
	if toRev != 0 {
		to = strconv.FormatInt(toRev, 10)
	}
	return Result{
		Success:    true,
		Message:    "k8s: rolled back " + ns + "/" + dep + " to " + to,
		ToRevision: to,
		Data:       map[string]any{"namespace": ns, "deployment": dep},
		Duration:   time.Since(start),
	}
}

// GetPreviousRevision implements [Executor]: the second-newest
// rollout revision number.
func (e K8sRolloutExecutor) GetPreviousRevision(ctx context.Context, cfg Config, _ Request) (string, error) {
	if e.Client == nil {
		return "", ErrNotConfigured
	}
	ns, dep, err := k8sCfg(cfg)
	if err != nil {
		return "", err
	}
	hist, err := e.Client.RevisionHistory(ctx, ns, dep)
	if err != nil {
		return "", err
	}
	if len(hist) < 2 {
		return "", ErrConfig
	}
	return strconv.FormatInt(hist[len(hist)-2].Revision, 10), nil
}

// GetLastKnownGood implements [Executor]. v1.0 best-effort: same as
// previous (no verification signal until task-9 persistence).
func (e K8sRolloutExecutor) GetLastKnownGood(ctx context.Context, cfg Config, req Request) (string, error) {
	return e.GetPreviousRevision(ctx, cfg, req)
}
