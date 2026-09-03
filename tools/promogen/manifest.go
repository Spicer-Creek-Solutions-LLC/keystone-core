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

// shotKind constrains the two rendering paths. A card is a static
// frame of styled text; a terminal shot is a real command run against
// the live promo topology.
const (
	kindCard     = "card"
	kindTerminal = "terminal"
)

// Manifest is assets/promo/manifest.yaml — the shot list as data.
// The rendered video is a concatenation of shots in file order, so the
// list is the edit decision list; nothing else encodes shot ordering.
type Manifest struct {
	TargetDuration float64 `yaml:"target_duration"`
	Tolerance      float64 `yaml:"tolerance"`
	Resolution     struct {
		Width  int `yaml:"width"`
		Height int `yaml:"height"`
	} `yaml:"resolution"`
	Shots []Shot `yaml:"shots"`

	// dir is the manifest's own directory, used to resolve the
	// relative tape paths. Not a YAML field.
	dir string
}

// Shot is one cut of the finished video.
type Shot struct {
	ID       string  `yaml:"id"`
	Kind     string  `yaml:"kind"`
	Duration float64 `yaml:"duration"`
	// Caption is the lower-third overlaid on a terminal shot. Cards
	// carry their copy inside the tape itself, so their caption is
	// empty by design.
	Caption string `yaml:"caption"`
	Tape    string `yaml:"tape"`
	// Feature ties a shot to a changelog entry's `Demo:` value so
	// reconcile can report a demoable feature that has no shot.
	Feature string `yaml:"feature"`
}

// LoadManifest reads and parses a manifest without validating it.
func LoadManifest(path string) (*Manifest, error) {
	// #nosec G304 G703 -- promogen is a developer tool run from a checkout; the
	// manifest path derives from its own --repo-root/--promo-dir flags.
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("promogen: read manifest: %w", err)
	}
	var m Manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("promogen: parse manifest: %w", err)
	}
	m.dir = filepath.Dir(path)
	return &m, nil
}

// TotalDuration sums the shot durations.
func (m *Manifest) TotalDuration() float64 {
	var total float64
	for _, s := range m.Shots {
		total += s.Duration
	}
	return total
}

// Validate returns every problem it finds rather than the first, so a
// single run reports the full set of edits needed.
//
// The duration budget is the load-bearing rule: a 30-second promo that
// silently grows to 40 stops being a promo. Adding a shot must force
// removing time somewhere else, so the sum is asserted, not advisory.
func (m *Manifest) Validate() []string {
	var problems []string

	if m.TargetDuration <= 0 {
		problems = append(problems, "target_duration must be > 0")
	}
	if m.Tolerance < 0 {
		problems = append(problems, "tolerance must be >= 0")
	}
	if m.Resolution.Width <= 0 || m.Resolution.Height <= 0 {
		problems = append(problems, "resolution.width and resolution.height must both be > 0")
	}
	if len(m.Shots) == 0 {
		problems = append(problems, "manifest declares no shots")
	}

	seen := make(map[string]bool, len(m.Shots))
	for i, s := range m.Shots {
		where := fmt.Sprintf("shot %d", i+1)
		if s.ID != "" {
			where = fmt.Sprintf("shot %q", s.ID)
		}

		switch {
		case s.ID == "":
			problems = append(problems, where+": id is required")
		case seen[s.ID]:
			problems = append(problems, where+": duplicate id")
		default:
			seen[s.ID] = true
		}

		if s.Kind != kindCard && s.Kind != kindTerminal {
			problems = append(problems, fmt.Sprintf("%s: kind must be %q or %q, got %q",
				where, kindCard, kindTerminal, s.Kind))
		}
		if s.Duration <= 0 {
			problems = append(problems, where+": duration must be > 0")
		}
		// A terminal shot with no caption is a wall of output with no
		// telling the viewer what they are looking at. Text-only promo:
		// the caption is the whole narration.
		if s.Kind == kindTerminal && strings.TrimSpace(s.Caption) == "" {
			problems = append(problems, where+": terminal shots require a caption")
		}
		if s.Tape == "" {
			problems = append(problems, where+": tape is required")
			continue
		}
		// Tape paths must stay inside the promo directory. A manifest is
		// repo source rather than untrusted input, but a path that
		// escapes the tree is a mistake worth naming either way, and it
		// keeps the render from reaching outside assets/promo/.
		tapePath, ok := m.resolveTape(s.Tape)
		if !ok {
			problems = append(problems, fmt.Sprintf(
				"%s: tape %s escapes the promo directory", where, s.Tape))
			continue
		}
		// #nosec G304 G703 -- resolveTape confines the path to the promo directory.
		if _, err := os.Stat(tapePath); err != nil {
			problems = append(problems, fmt.Sprintf("%s: tape %s not found", where, s.Tape))
		}
	}

	if total := m.TotalDuration(); m.TargetDuration > 0 {
		if diff := total - m.TargetDuration; diff > m.Tolerance || diff < -m.Tolerance {
			problems = append(problems, fmt.Sprintf(
				"shot durations total %.2fs, want %.2fs ±%.2fs (over/under by %+.2fs) — "+
					"add or remove time from an existing shot rather than growing the runtime",
				total, m.TargetDuration, m.Tolerance, diff))
		}
	}

	return problems
}

// Features returns the sorted, de-duplicated set of non-empty shot
// feature tags.
func (m *Manifest) Features() []string {
	set := make(map[string]bool)
	for _, s := range m.Shots {
		if s.Feature != "" {
			set[s.Feature] = true
		}
	}
	out := make([]string, 0, len(set))
	for f := range set {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// resolveTape joins a manifest-relative tape path onto the promo
// directory, rejecting anything that escapes it.
func (m *Manifest) resolveTape(tape string) (string, bool) {
	if filepath.IsAbs(tape) {
		return "", false
	}
	base := filepath.Clean(m.dir)
	full := filepath.Clean(filepath.Join(base, tape))
	if full != base && !strings.HasPrefix(full, base+string(filepath.Separator)) {
		return "", false
	}
	return full, true
}
