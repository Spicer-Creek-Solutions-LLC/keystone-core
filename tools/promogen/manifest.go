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

// Resolution is a reel's output frame size.
type Resolution struct {
	Width  int `yaml:"width"`
	Height int `yaml:"height"`
}

// Defaults are inherited by every reel that does not override them.
// Only genuinely shared properties belong here: a reel's duration is
// editorial and is always stated per reel.
type Defaults struct {
	Tolerance  float64    `yaml:"tolerance"`
	Resolution Resolution `yaml:"resolution"`
}

// Manifest is assets/promo/manifest.yaml — every reel's shot list as
// data. A reel renders to one video; shots concatenate in file order,
// so the list is the edit decision list and nothing else encodes shot
// ordering.
type Manifest struct {
	Defaults Defaults `yaml:"defaults"`
	Reels    []Reel   `yaml:"reels"`

	// dir is the manifest's own directory, used to resolve the
	// relative tape paths. Not a YAML field.
	dir string
}

// Reel is one finished video: the 30-second promo, or a short
// per-feature clip embedded in a docs page.
type Reel struct {
	ID    string `yaml:"id"`
	Title string `yaml:"title"`
	// Output is the basename written under dist/promo/, without the
	// extension. Must be unique across reels.
	Output string `yaml:"output"`
	// TargetDuration is asserted, not advisory. A clip that drifts past
	// its budget stops doing its job, so adding a shot has to take the
	// time back from another one.
	TargetDuration float64 `yaml:"target_duration"`
	// Tolerance and Resolution fall back to the manifest defaults when
	// left at their zero value.
	Tolerance  float64    `yaml:"tolerance"`
	Resolution Resolution `yaml:"resolution"`
	// SquareCut additionally renders a 1:1 crop for social. Worth it
	// for the promo, noise for a docs clip.
	SquareCut bool `yaml:"square_cut"`
	// DocsPage is the Hugo page this clip is embedded in, if any.
	// Recorded so a reel and its destination cannot drift apart
	// silently.
	DocsPage string `yaml:"docs_page"`
	Shots    []Shot `yaml:"shots"`
}

// EffectiveTolerance resolves the reel's tolerance against defaults.
func (r Reel) EffectiveTolerance(d Defaults) float64 {
	if r.Tolerance > 0 {
		return r.Tolerance
	}
	return d.Tolerance
}

// EffectiveResolution resolves the reel's frame size against defaults.
func (r Reel) EffectiveResolution(d Defaults) Resolution {
	res := r.Resolution
	if res.Width <= 0 {
		res.Width = d.Resolution.Width
	}
	if res.Height <= 0 {
		res.Height = d.Resolution.Height
	}
	return res
}

// TotalDuration sums the reel's shot durations.
func (r Reel) TotalDuration() float64 {
	var total float64
	for _, s := range r.Shots {
		total += s.Duration
	}
	return total
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

// Reel returns the named reel, or nil when absent.
func (m *Manifest) Reel(id string) *Reel {
	for i := range m.Reels {
		if m.Reels[i].ID == id {
			return &m.Reels[i]
		}
	}
	return nil
}

// AllShots flattens every reel's shots in manifest order. Used by the
// checks that care about tapes rather than about any one video.
func (m *Manifest) AllShots() []Shot {
	var out []Shot
	for _, r := range m.Reels {
		out = append(out, r.Shots...)
	}
	return out
}

// TotalDuration sums every reel's runtime. Reported, not budgeted —
// budgets are per reel.
func (m *Manifest) TotalDuration() float64 {
	var total float64
	for _, r := range m.Reels {
		total += r.TotalDuration()
	}
	return total
}

// Validate returns every problem it finds rather than the first, so a
// single run reports the full set of edits needed.
//
// The per-reel duration budget is the load-bearing rule: a 30-second
// promo that silently grows to 40 stops being a promo, and a docs clip
// that grows past ~20s stops getting watched. Adding a shot must force
// removing time somewhere else in the SAME reel, so each sum is
// asserted, not advisory.
func (m *Manifest) Validate() []string {
	var problems []string

	if m.Defaults.Tolerance < 0 {
		problems = append(problems, "defaults.tolerance must be >= 0")
	}
	if len(m.Reels) == 0 {
		problems = append(problems, "manifest declares no reels")
	}

	// IDs and outputs are unique across the whole manifest: two reels
	// writing the same output file would silently clobber each other,
	// and duplicate ids make `--reel` ambiguous.
	seenReel := make(map[string]bool, len(m.Reels))
	seenOutput := make(map[string]bool, len(m.Reels))

	for i := range m.Reels {
		r := &m.Reels[i]
		where := fmt.Sprintf("reel %d", i+1)
		if r.ID != "" {
			where = fmt.Sprintf("reel %q", r.ID)
		}

		switch {
		case r.ID == "":
			problems = append(problems, where+": id is required")
		case seenReel[r.ID]:
			problems = append(problems, where+": duplicate reel id")
		default:
			seenReel[r.ID] = true
		}

		switch {
		case r.Output == "":
			problems = append(problems, where+": output is required")
		case seenOutput[r.Output]:
			problems = append(problems, fmt.Sprintf(
				"%s: duplicate output %q — two reels would write the same file", where, r.Output))
		default:
			seenOutput[r.Output] = true
		}

		if r.TargetDuration <= 0 {
			problems = append(problems, where+": target_duration must be > 0")
		}
		if r.Tolerance < 0 {
			problems = append(problems, where+": tolerance must be >= 0")
		}
		res := r.EffectiveResolution(m.Defaults)
		if res.Width <= 0 || res.Height <= 0 {
			problems = append(problems, where+
				": resolution width and height must both be > 0 (set on the reel or in defaults)")
		}
		if len(r.Shots) == 0 {
			problems = append(problems, where+": declares no shots")
		}

		problems = append(problems, m.validateShots(where, r)...)

		tol := r.EffectiveTolerance(m.Defaults)
		if total := r.TotalDuration(); r.TargetDuration > 0 {
			if diff := total - r.TargetDuration; diff > tol || diff < -tol {
				problems = append(problems, fmt.Sprintf(
					"%s: shot durations total %.2fs, want %.2fs ±%.2fs (over/under by %+.2fs) — "+
						"add or remove time from an existing shot rather than growing the runtime",
					where, total, r.TargetDuration, tol, diff))
			}
		}
	}

	return problems
}

// validateShots checks one reel's shots. Shot ids are unique within a
// reel rather than globally: two reels legitimately both open on a
// shot called "hook".
func (m *Manifest) validateShots(reelWhere string, r *Reel) []string {
	var problems []string
	seen := make(map[string]bool, len(r.Shots))

	for i, s := range r.Shots {
		where := fmt.Sprintf("%s shot %d", reelWhere, i+1)
		if s.ID != "" {
			where = fmt.Sprintf("%s shot %q", reelWhere, s.ID)
		}

		switch {
		case s.ID == "":
			problems = append(problems, where+": id is required")
		case seen[s.ID]:
			problems = append(problems, where+": duplicate id within the reel")
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
		// telling the viewer what they are looking at. Text-only:
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
	return problems
}

// Features returns the sorted, de-duplicated set of non-empty shot
// feature tags.
func (m *Manifest) Features() []string {
	set := make(map[string]bool)
	for _, s := range m.AllShots() {
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
