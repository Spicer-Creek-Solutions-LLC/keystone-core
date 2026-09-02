// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// fragment is the subset of a changie fragment promogen cares about.
// The `Demo` custom field is opt-in: a PR author who just built a
// feature knows whether it is legible in four seconds of terminal
// output, and marks it. Mining that judgement back out of the prose
// body afterwards does not work — changelog bodies describe behaviour,
// not demoability.
type fragment struct {
	Kind   string            `yaml:"kind"`
	Body   string            `yaml:"body"`
	Custom map[string]string `yaml:"custom"`
}

// DemoTag is one changelog entry that asked for screen time.
type DemoTag struct {
	// File is the fragment path, relative to the repo root.
	File string
	// Feature is the `Demo:` value, matched against a shot's feature.
	Feature string
	// Title is the fragment body's leading bold title, for readable
	// reconcile output.
	Title string
}

// ReconcileReport is the result of matching demo-tagged changelog
// entries against the shot list.
type ReconcileReport struct {
	// Unshot are demo-tagged entries with no shot covering them —
	// the actionable half: a feature was flagged worth showing and the
	// video does not show it.
	Unshot []DemoTag
	// Covered are demo-tagged entries a shot already covers.
	Covered []DemoTag
	// Orphaned are shot feature tags matching no unreleased entry.
	// Informational only: a shot legitimately outlives the release
	// that introduced its feature.
	Orphaned []string
}

// LoadDemoTags reads every unreleased changie fragment and returns the
// ones carrying a `Demo:` custom value.
func LoadDemoTags(repoRoot string) ([]DemoTag, error) {
	dir := filepath.Join(repoRoot, ".changes", "unreleased")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("promogen: read %s: %w", dir, err)
	}

	var tags []DemoTag
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || (!strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml")) {
			continue
		}
		path := filepath.Join(dir, name)
		// #nosec G304 G703 -- path is a directory entry under .changes/unreleased
		// in the developer's own checkout.
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("promogen: read %s: %w", path, err)
		}
		var f fragment
		if err := yaml.Unmarshal(raw, &f); err != nil {
			return nil, fmt.Errorf("promogen: parse %s: %w", path, err)
		}
		demo := strings.TrimSpace(f.Custom["Demo"])
		if demo == "" {
			continue
		}
		tags = append(tags, DemoTag{
			File:    filepath.Join(".changes", "unreleased", name),
			Feature: demo,
			Title:   fragmentTitle(f.Body),
		})
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i].File < tags[j].File })
	return tags, nil
}

// fragmentTitle extracts the leading `**Short title.**` from a changie
// body, falling back to the first line. Presentation only.
func fragmentTitle(body string) string {
	line := body
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	line = strings.TrimSpace(line)
	if rest, ok := strings.CutPrefix(line, "**"); ok {
		if i := strings.Index(rest, "**"); i >= 0 {
			return strings.TrimSpace(rest[:i])
		}
	}
	return line
}

// Reconcile matches demo tags against the manifest's shot features.
func Reconcile(m *Manifest, tags []DemoTag) ReconcileReport {
	shotFeatures := make(map[string]bool)
	for _, f := range m.Features() {
		shotFeatures[f] = false // false = not yet claimed by a tag
	}

	var rep ReconcileReport
	for _, t := range tags {
		if _, ok := shotFeatures[t.Feature]; ok {
			shotFeatures[t.Feature] = true
			rep.Covered = append(rep.Covered, t)
			continue
		}
		rep.Unshot = append(rep.Unshot, t)
	}
	for f, claimed := range shotFeatures {
		if !claimed {
			rep.Orphaned = append(rep.Orphaned, f)
		}
	}
	sort.Strings(rep.Orphaned)
	return rep
}
