package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	bp "go.keystone-core.io/keystone-core/internal/blueprint"
	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/controlplane"
	"go.keystone-core.io/keystone-core/internal/runbook"
	"go.keystone-core.io/keystone-core/internal/runbook/steps"
	"go.keystone-core.io/keystone-core/internal/statemgmt"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib"
)

// fsBlueprintCatalog walks a directory of blueprint subdirectories
// and serves their manifests. v1.0 baseline — the directory is
// scanned eagerly at construction time; later additions require a
// kscore-server restart. A reload endpoint is post-v1.0 work.
type fsBlueprintCatalog struct {
	manifests map[string]*bp.Manifest
}

func newFSBlueprintCatalog(root string, log *slog.Logger) (*fsBlueprintCatalog, error) {
	c := &fsBlueprintCatalog{manifests: map[string]*bp.Manifest{}}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("blueprint catalog %q: %w", root, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		m, err := bp.Load(dir)
		switch {
		case errors.Is(err, bp.ErrNotFound):
			// Sub-directory without a blueprint.yaml — skip.
			continue
		case err != nil:
			log.Warn("blueprint catalog: skip invalid",
				"dir", dir, "err", err.Error())
			continue
		}
		c.manifests[m.Metadata.Name] = m
	}
	return c, nil
}

func (c *fsBlueprintCatalog) List(_ context.Context) ([]*bp.Manifest, error) {
	out := make([]*bp.Manifest, 0, len(c.manifests))
	keys := make([]string, 0, len(c.manifests))
	for k := range c.manifests {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, c.manifests[k])
	}
	return out, nil
}

func (c *fsBlueprintCatalog) Get(_ context.Context, name string) (*bp.Manifest, error) {
	m, ok := c.manifests[name]
	if !ok {
		return nil, fmt.Errorf("blueprint %q not found", name)
	}
	return m, nil
}

// localBlueprintApplier runs blueprints against a server-local stdlib
// StateRunner. Same convergence path the kscore-blueprint CLI uses
// today — see docs/project/ROADMAP.md "Remote / distributed blueprint
// apply wiring" for the gate-v1.0 deferral on agent-fleet dispatch.
type localBlueprintApplier struct {
	catalog *fsBlueprintCatalog
	exec    *bp.Executor
}

func newLocalBlueprintApplier(c *fsBlueprintCatalog) (*localBlueprintApplier, error) {
	stReg := statemgmt.NewRegistry()
	if err := stdlib.RegisterAll(stReg); err != nil {
		return nil, fmt.Errorf("blueprint applier: stdlib register: %w", err)
	}
	rbReg := runbook.NewRegistry()
	if err := steps.RegisterAll(rbReg, steps.Deps{}); err != nil {
		return nil, fmt.Errorf("blueprint applier: runbook steps: %w", err)
	}
	return &localBlueprintApplier{
		catalog: c,
		exec: &bp.Executor{
			StateRunner: statemgmt.NewRunner(stReg, nil),
			Hooks:       bp.NewRunbookHookRunner(&runbook.Executor{Registry: rbReg}),
			Store:       bp.NewMemoryAppliedStore(),
		},
	}, nil
}

func (a *localBlueprintApplier) Apply(ctx context.Context, name string, opts bp.ApplyOptions) (*bp.ApplyResult, error) {
	m, err := a.catalog.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	return a.exec.Apply(ctx, m, opts)
}

// maybeWireBlueprintService constructs the catalog + applier from
// cfg.Blueprints.CatalogPath. Returns nil + nil when the path is
// empty or the directory is missing — the BlueprintService stays
// unregistered and clients reach Unimplemented. Errors only when a
// non-empty path is set but the catalog walk fails.
func maybeWireBlueprintService(cfg config.BlueprintsConfig, log *slog.Logger) (*controlplane.BlueprintGRPCServer, error) {
	if strings.TrimSpace(cfg.CatalogPath) == "" {
		return nil, nil
	}
	if _, err := os.Stat(cfg.CatalogPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Warn("blueprints.catalogpath does not exist; skipping BlueprintService",
				"path", cfg.CatalogPath)
			return nil, nil
		}
		return nil, fmt.Errorf("blueprints catalog stat: %w", err)
	}
	cat, err := newFSBlueprintCatalog(cfg.CatalogPath, log)
	if err != nil {
		return nil, err
	}
	applier, err := newLocalBlueprintApplier(cat)
	if err != nil {
		return nil, err
	}
	return &controlplane.BlueprintGRPCServer{
		Catalog: cat,
		Applier: applier,
	}, nil
}

