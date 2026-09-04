// SPDX-License-Identifier: Apache-2.0

// promogen maintains the generated half of the promo-video pipeline in
// assets/promo/ (see assets/promo/README.md for the script and shot
// list, and docs/project/ROADMAP.md for why the pipeline exists).
//
// The split it enforces: the *narrative* — which shots, in what order,
// saying what — is hand-authored in manifest.yaml and reviewed like any
// other source. Everything *mechanical* — the version on the end card,
// the module count, the runtime budget, whether a demo-tagged changelog
// entry has a shot — is derived from the repository, so the video
// cannot quietly drift from what the project ships.
//
// Subcommands:
//
//	validate   assert the manifest is well-formed and inside its
//	           runtime budget. Reads only.
//	sync       render assets/promo/tapes/*.tape.tmpl from repo facts.
//	           -check reports drift without writing (CI gate).
//	reconcile  match `Demo:`-tagged changelog fragments against the
//	           shot list. -strict makes an unshot entry an error.
//	plan       emit the shot list as TSV for pipeline/build.sh, so the
//	           ffmpeg assembler never re-parses the manifest itself.
//	facts      print the derived facts (debugging).
//
// Usage:
//
//	go run ./tools/promogen validate
//	go run ./tools/promogen sync [-check]
//	go run ./tools/promogen reconcile [-strict]
//	go run ./tools/promogen tapes
//	go run ./tools/promogen reels
//	go run ./tools/promogen plan [-reel <id>]
//
// Exit codes: 0 on pass, 1 on any validation failure, sync drift under
// -check, or unshot demo entry under -strict.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

const (
	defaultPromoDir = "assets/promo"
	manifestName    = "manifest.yaml"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("promogen: expected a subcommand " +
			"(validate | sync | reconcile | tapes | reels | plan | facts)")
	}

	cmd, rest := args[0], args[1:]
	fs := flag.NewFlagSet("promogen "+cmd, flag.ContinueOnError)
	repoRoot := fs.String("repo-root", ".", "repository root")
	promoDir := fs.String("promo-dir", defaultPromoDir, "promo asset directory, relative to -repo-root")
	check := fs.Bool("check", false, "sync: report drift without writing")
	strict := fs.Bool("strict", false, "reconcile: exit non-zero when a demo-tagged entry has no shot")
	reel := fs.String("reel", "", "plan: emit only this reel's shots (default: every reel)")
	if err := fs.Parse(rest); err != nil {
		return err
	}

	dir := filepath.Join(*repoRoot, *promoDir)

	switch cmd {
	case "validate":
		return cmdValidate(*repoRoot, dir)
	case "sync":
		return cmdSync(*repoRoot, dir, *check)
	case "reconcile":
		return cmdReconcile(*repoRoot, dir, *strict)
	case "tapes":
		return cmdTapes(*repoRoot, dir)
	case "plan":
		return cmdPlan(dir, *reel)
	case "reels":
		return cmdReels(dir)
	case "facts":
		return cmdFacts(*repoRoot)
	default:
		return fmt.Errorf("promogen: unknown subcommand %q "+
			"(want validate | sync | reconcile | tapes | reels | plan | facts)", cmd)
	}
}

func cmdValidate(repoRoot, promoDir string) error {
	m, err := LoadManifest(filepath.Join(promoDir, manifestName))
	if err != nil {
		return err
	}
	problems := m.Validate()
	problems = append(problems, m.ValidateDocsPages(repoRoot)...)
	if len(problems) > 0 {
		fmt.Fprintf(os.Stderr, "FAIL %s: %d problem(s)\n", manifestName, len(problems))
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "  - %s\n", p)
		}
		return fmt.Errorf("promogen: manifest validation failed")
	}
	for _, r := range m.Reels {
		fmt.Printf("OK   reel %-18s %d shots, %.2fs (target %.2fs ±%.2fs)  -> %s\n",
			r.ID, len(r.Shots), r.TotalDuration(), r.TargetDuration,
			r.EffectiveTolerance(m.Defaults), r.Output+".mp4")
	}
	fmt.Printf("OK   %d reel(s), %.2fs of finished video\n", len(m.Reels), m.TotalDuration())
	return nil
}

func cmdSync(repoRoot, promoDir string, check bool) error {
	facts, err := DeriveFacts(repoRoot)
	if err != nil {
		return err
	}
	res, err := SyncTapes(promoDir, facts, check)
	if err != nil {
		return err
	}
	for _, f := range res.Unchanged {
		fmt.Printf("OK   %s\n", f)
	}
	for _, f := range res.Written {
		verb := "WROTE"
		if check {
			verb = "DRIFT"
		}
		fmt.Printf("%s %s\n", verb, f)
	}
	if check && len(res.Written) > 0 {
		return fmt.Errorf("promogen: %d generated tape(s) out of date; run `make update-promo`",
			len(res.Written))
	}
	return nil
}

func cmdReconcile(repoRoot, promoDir string, strict bool) error {
	m, err := LoadManifest(filepath.Join(promoDir, manifestName))
	if err != nil {
		return err
	}
	tags, err := LoadDemoTags(repoRoot)
	if err != nil {
		return err
	}
	rep := Reconcile(m, tags)

	for _, t := range rep.Covered {
		fmt.Printf("OK      %s -> shot %q\n", t.File, t.Feature)
	}
	for _, f := range rep.Orphaned {
		fmt.Printf("NOTE    shot feature %q matches no unreleased entry "+
			"(fine for a shot older than the current cycle)\n", f)
	}
	for _, t := range rep.Unshot {
		fmt.Printf("UNSHOT  %s tagged Demo: %s — %s\n", t.File, t.Feature, t.Title)
	}

	if len(rep.Unshot) == 0 {
		fmt.Printf("OK   %d demo-tagged entr(ies), all covered by a shot\n", len(rep.Covered))
		return nil
	}
	fmt.Printf("\n%d demo-tagged changelog entr(ies) have no shot. Either add a shot to "+
		"%s (and take the time back from another shot), or drop the Demo: tag.\n",
		len(rep.Unshot), manifestName)
	if strict {
		return fmt.Errorf("promogen: %d unshot demo-tagged entr(ies)", len(rep.Unshot))
	}
	return nil
}

// cmdTapes verifies every project-binary invocation across every tape
// in the manifest.
func cmdTapes(repoRoot, promoDir string) error {
	m, err := LoadManifest(filepath.Join(promoDir, manifestName))
	if err != nil {
		return err
	}
	bins, err := ProjectBinaries(repoRoot)
	if err != nil {
		return err
	}

	var all []TapeCommand
	for _, s := range m.AllShots() {
		cmds, err := ExtractCommands(promoDir, s.Tape, bins)
		if err != nil {
			return err
		}
		all = append(all, cmds...)
	}
	if len(all) == 0 {
		return fmt.Errorf("promogen: no project-binary commands found in any tape; "+
			"either the tapes stopped invoking the %d cmd/ binaries or the "+
			"Type-directive parser broke", len(bins))
	}

	problems, err := VerifyCommands(repoRoot, all, bins)
	if err != nil {
		return err
	}
	for _, c := range all {
		fmt.Printf("OK   %s:%d  %s\n", c.Tape, c.Line, c.String())
	}
	if len(problems) > 0 {
		fmt.Fprintf(os.Stderr, "\nFAIL %d tape command(s) do not resolve:\n", len(problems))
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "  - %s\n", p)
		}
		return fmt.Errorf("promogen: %d unresolvable tape command(s)", len(problems))
	}
	fmt.Printf("OK   %d tape command(s) resolve\n", len(all))
	return nil
}

// cmdPlan emits one TSV row per shot: id, kind, duration, tape,
// caption. pipeline/build.sh reads this instead of parsing YAML in
// bash, which keeps exactly one parser for the manifest.
func cmdPlan(promoDir, reel string) error {
	m, err := LoadManifest(filepath.Join(promoDir, manifestName))
	if err != nil {
		return err
	}
	if problems := m.Validate(); len(problems) > 0 {
		return fmt.Errorf("promogen: refusing to emit a plan for an invalid manifest; "+
			"run `go run ./tools/promogen validate` (%d problem(s))", len(problems))
	}
	if reel != "" && m.Reel(reel) == nil {
		return fmt.Errorf("promogen: no reel %q in %s", reel, manifestName)
	}
	for _, r := range m.Reels {
		if reel != "" && r.ID != reel {
			continue
		}
		for _, s := range r.Shots {
			fmt.Printf("%s\t%s\t%s\t%.3f\t%s\t%s\n",
				r.ID, s.ID, s.Kind, s.Duration, s.Tape, s.Caption)
		}
	}
	return nil
}

// cmdReels emits one TSV row per reel: id, output, target duration,
// square-cut flag, shot count, title. build.sh reads this to know what
// to render and where to write it, so the manifest still has exactly
// one parser.
func cmdReels(promoDir string) error {
	m, err := LoadManifest(filepath.Join(promoDir, manifestName))
	if err != nil {
		return err
	}
	if problems := m.Validate(); len(problems) > 0 {
		return fmt.Errorf("promogen: refusing to list reels for an invalid manifest; "+
			"run `go run ./tools/promogen validate` (%d problem(s))", len(problems))
	}
	for _, r := range m.Reels {
		res := r.EffectiveResolution(m.Defaults)
		fmt.Printf("%s\t%s\t%.3f\t%t\t%d\t%dx%d\t%s\n",
			r.ID, r.Output, r.TargetDuration, r.SquareCut, len(r.Shots),
			res.Width, res.Height, r.Title)
	}
	return nil
}

func cmdFacts(repoRoot string) error {
	f, err := DeriveFacts(repoRoot)
	if err != nil {
		return err
	}
	fmt.Printf("Version      %s\n", f.Version)
	fmt.Printf("PreRelease   %t\n", f.PreRelease)
	fmt.Printf("ReleaseLabel %s\n", f.ReleaseLabel)
	fmt.Printf("ModuleCount  %d\n", f.ModuleCount)
	fmt.Printf("BinaryCount  %d\n", f.BinaryCount)
	fmt.Printf("DistroCount  %d\n", f.DistroCount)
	fmt.Printf("RepoURL      %s\n", f.RepoURL)
	return nil
}
