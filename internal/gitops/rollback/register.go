// SPDX-License-Identifier: Apache-2.0

package rollback

// Deps carries the client seams the executors need. A nil seam is
// valid — the corresponding executor registers but fails with
// [ErrNotConfigured] at Execute time (the Kubernetes client-go
// adapter is deferred to boot, so K8s is commonly nil in v1.0).
type Deps struct {
	Git  GitClient
	Argo ArgoClient
	K8s  K8sRolloutClient
}

// RegisterAll binds the three v1.0 executors (git, argocd, k8s) onto
// reg using deps. Single wiring point; mirrors the verification /
// webhook RegisterAll pattern so the set is enumerated in one place.
func RegisterAll(reg *Registry, deps Deps) error {
	executors := []Executor{
		GitRevertExecutor{Client: deps.Git},
		ArgoCDExecutor{Client: deps.Argo},
		K8sRolloutExecutor{Client: deps.K8s},
	}
	for _, e := range executors {
		if err := reg.Register(e); err != nil {
			return err
		}
	}
	return nil
}

// NewDefaultRegistry returns a [Registry] with the three v1.0
// executors registered using deps. Panics only on a programming
// error (an executor reporting an empty type), which the built-in
// set cannot hit — a guard for future additions.
func NewDefaultRegistry(deps Deps) *Registry {
	reg := NewRegistry()
	if err := RegisterAll(reg, deps); err != nil {
		panic("rollback: built-in executor registration failed: " + err.Error())
	}
	return reg
}
