package registry

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// GCConfig configures garbage collection.
type GCConfig struct {
	// KeepVersions keeps the N most recent versions per module.
	// 0 means keep all versions (no version-count pruning).
	KeepVersions int

	// MaxAge removes versions older than this duration.
	// 0 means no age-based pruning.
	MaxAge time.Duration

	// DryRun reports what would be removed without deleting.
	DryRun bool
}

// GCResult summarizes a garbage collection run.
type GCResult struct {
	RemovedModules []string
	ReclaimedBytes int64
}

// GC removes old module versions based on the retention policy.
func (r *Registry) GC(ctx context.Context, cfg GCConfig) (*GCResult, error) {
	if cfg.KeepVersions == 0 && cfg.MaxAge == 0 {
		return &GCResult{}, nil
	}

	modules, err := discoverModulesInDir(r.ModulesDir(), r.ModulesDir(), "")
	if err != nil {
		return nil, fmt.Errorf("discover modules: %w", err)
	}

	result := &GCResult{}

	for _, moduleName := range modules {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}

		versions, err := r.backend.ListVersions(ctx, moduleName)
		if err != nil {
			continue
		}

		toRemove := selectVersionsForRemoval(ctx, r, moduleName, versions, cfg)

		for _, version := range toRemove {
			size := versionDirSize(filepath.Join(r.ModulesDir(), moduleName, version))

			if cfg.DryRun {
				result.RemovedModules = append(result.RemovedModules, fmt.Sprintf("%s@%s", moduleName, version))
				result.ReclaimedBytes += size
				continue
			}

			if err := r.backend.Delete(ctx, moduleName, version); err != nil {
				continue
			}

			result.RemovedModules = append(result.RemovedModules, fmt.Sprintf("%s@%s", moduleName, version))
			result.ReclaimedBytes += size
		}
	}

	if !cfg.DryRun && len(result.RemovedModules) > 0 && r.config.AutoIndex {
		if err := r.Reindex(); err != nil {
			return result, fmt.Errorf("reindex after GC: %w", err)
		}
	}

	return result, nil
}

// selectVersionsForRemoval determines which versions to remove based on retention policy.
func selectVersionsForRemoval(ctx context.Context, r *Registry, moduleName string, versions []string, cfg GCConfig) []string {
	// FilesystemBackend returns versions sorted descending, which is what we want
	// (keep the first N, consider removing the rest)

	var candidates []string

	if cfg.KeepVersions > 0 && len(versions) > cfg.KeepVersions {
		candidates = versions[cfg.KeepVersions:]
	}

	if cfg.MaxAge > 0 {
		cutoff := time.Now().Add(-cfg.MaxAge)
		var aged []string
		for _, v := range versions {
			info, err := r.backend.GetInfo(ctx, moduleName, v)
			if err != nil {
				continue
			}
			if info.PublishedAt.Before(cutoff) {
				aged = append(aged, v)
			}
		}

		if cfg.KeepVersions > 0 {
			// Intersect: only remove versions that are both beyond keep count AND too old
			candidateSet := make(map[string]bool)
			for _, v := range candidates {
				candidateSet[v] = true
			}
			var intersection []string
			for _, v := range aged {
				if candidateSet[v] {
					intersection = append(intersection, v)
				}
			}
			candidates = intersection
		} else {
			candidates = aged
		}
	}

	sort.Strings(candidates)
	return candidates
}

// versionDirSize calculates the total size of files in a version directory.
func versionDirSize(dir string) int64 {
	var total int64
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		total += info.Size()
	}
	return total
}
