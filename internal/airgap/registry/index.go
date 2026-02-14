package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const indexSchemaVersion = "1.0"

// Index is a pre-built registry index for fast search and listing.
// It is stored as index.json at the registry root.
type Index struct {
	SchemaVersion string           `json:"schema_version"`
	Generated     time.Time        `json:"generated"`
	Modules       []ModuleEntry    `json:"modules"`
	Blueprints    []BlueprintEntry `json:"blueprints,omitempty"`
}

// ModuleEntry summarizes a module in the index.
type ModuleEntry struct {
	Name          string   `json:"name"`
	LatestVersion string   `json:"latest_version"`
	Description   string   `json:"description,omitempty"`
	Versions      []string `json:"versions"`
	Tags          []string `json:"tags,omitempty"`
}

// BlueprintEntry summarizes a blueprint in the index.
type BlueprintEntry struct {
	Name          string   `json:"name"`
	LatestVersion string   `json:"latest_version,omitempty"`
	Description   string   `json:"description,omitempty"`
	Versions      []string `json:"versions,omitempty"`
}

// Generate builds an index by walking the registry filesystem.
// rootDir is the registry root containing modules/ and blueprints/ subdirectories.
func Generate(rootDir string) (*Index, error) {
	idx := &Index{
		SchemaVersion: indexSchemaVersion,
		Generated:     time.Now().UTC(),
	}

	modulesDir := filepath.Join(rootDir, "modules")
	if info, err := os.Stat(modulesDir); err == nil && info.IsDir() {
		modules, err := discoverModulesInDir(modulesDir, modulesDir, "")
		if err != nil {
			return nil, fmt.Errorf("discover modules: %w", err)
		}

		for _, moduleName := range modules {
			entry, err := buildModuleEntry(modulesDir, moduleName)
			if err != nil {
				continue
			}
			idx.Modules = append(idx.Modules, *entry)
		}
	}

	blueprintsDir := filepath.Join(rootDir, "blueprints")
	if info, err := os.Stat(blueprintsDir); err == nil && info.IsDir() {
		entries, err := os.ReadDir(blueprintsDir)
		if err == nil {
			for _, e := range entries {
				if e.IsDir() {
					idx.Blueprints = append(idx.Blueprints, BlueprintEntry{
						Name: e.Name(),
					})
				}
			}
		}
	}

	sort.Slice(idx.Modules, func(i, j int) bool {
		return idx.Modules[i].Name < idx.Modules[j].Name
	})
	sort.Slice(idx.Blueprints, func(i, j int) bool {
		return idx.Blueprints[i].Name < idx.Blueprints[j].Name
	})

	return idx, nil
}

// buildModuleEntry builds a ModuleEntry by reading versions from the filesystem.
func buildModuleEntry(modulesDir, moduleName string) (*ModuleEntry, error) {
	moduleDir := filepath.Join(modulesDir, moduleName)
	dirEntries, err := os.ReadDir(moduleDir)
	if err != nil {
		return nil, err
	}

	var versions []string
	for _, de := range dirEntries {
		if !de.IsDir() {
			continue
		}
		zipPath := filepath.Join(moduleDir, de.Name(), "module.zip")
		if _, err := os.Stat(zipPath); err == nil {
			versions = append(versions, de.Name())
		}
	}

	if len(versions) == 0 {
		return nil, fmt.Errorf("no versions found")
	}

	sort.Sort(sort.Reverse(sort.StringSlice(versions)))

	entry := &ModuleEntry{
		Name:          moduleName,
		LatestVersion: versions[0],
		Versions:      versions,
	}

	// Try to read description and tags from the latest version's info
	infoPath := filepath.Join(moduleDir, versions[0], "module.info")
	//nolint:gosec // G304: infoPath is constructed from trusted registry directory
	if data, err := os.ReadFile(infoPath); err == nil {
		var info struct {
			Description string   `json:"description"`
			Tags        []string `json:"tags"`
		}
		if json.Unmarshal(data, &info) == nil {
			entry.Description = info.Description
			entry.Tags = info.Tags
		}
	}

	return entry, nil
}

// discoverModulesInDir walks a directory tree to find module names.
// A module is identified by having at least one version subdirectory containing module.zip.
// This mirrors the logic in internal/registry/storage/migrate.go.
func discoverModulesInDir(root, current, rel string) ([]string, error) {
	entries, err := os.ReadDir(current)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var modules []string

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		childRel := entry.Name()
		if rel != "" {
			childRel = rel + "/" + entry.Name()
		}
		childPath := filepath.Join(current, entry.Name())

		// Check if this directory is a version dir (contains module.zip)
		zipPath := filepath.Join(childPath, "module.zip")
		if _, err := os.Stat(zipPath); err == nil {
			if rel != "" && !seen[rel] {
				seen[rel] = true
				modules = append(modules, rel)
			}
			continue
		}

		// Recurse into subdirectories
		sub, err := discoverModulesInDir(root, childPath, childRel)
		if err != nil {
			return nil, err
		}
		for _, m := range sub {
			if !seen[m] {
				seen[m] = true
				modules = append(modules, m)
			}
		}
	}

	return modules, nil
}

// LoadIndex reads and parses an index from a JSON file.
func LoadIndex(path string) (*Index, error) {
	//nolint:gosec // G304: path is the caller-provided index file path
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parse index: %w", err)
	}
	return &idx, nil
}

// Save writes the index to a JSON file.
func (idx *Index) Save(path string) error {
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}
	//nolint:gosec // G306: index file needs to be readable
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write index: %w", err)
	}
	return nil
}

// Search returns modules whose name, description, or tags match the query.
// The query is matched case-insensitively as a substring.
func (idx *Index) Search(query string) []ModuleEntry {
	if query == "" {
		return idx.Modules
	}
	q := strings.ToLower(query)
	var results []ModuleEntry
	for _, m := range idx.Modules {
		if strings.Contains(strings.ToLower(m.Name), q) ||
			strings.Contains(strings.ToLower(m.Description), q) ||
			matchTags(m.Tags, q) {
			results = append(results, m)
		}
	}
	return results
}

// SearchBlueprints returns blueprints whose name or description matches the query.
func (idx *Index) SearchBlueprints(query string) []BlueprintEntry {
	if query == "" {
		return idx.Blueprints
	}
	q := strings.ToLower(query)
	var results []BlueprintEntry
	for _, b := range idx.Blueprints {
		if strings.Contains(strings.ToLower(b.Name), q) ||
			strings.Contains(strings.ToLower(b.Description), q) {
			results = append(results, b)
		}
	}
	return results
}

func matchTags(tags []string, query string) bool {
	for _, t := range tags {
		if strings.Contains(strings.ToLower(t), query) {
			return true
		}
	}
	return false
}

