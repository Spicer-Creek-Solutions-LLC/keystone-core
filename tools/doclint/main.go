// doclint asserts that conclusions this project has reached have not been
// contradicted since.
//
// Usage: doclint [-root DIR] [-tasks-complete TASK,TASK,...]
//
// It reads the tracked Markdown files, applies every rule in rules.go, and
// reports each hit with a disposition. NOTHING IS FILTERED: a rule that quietly
// drops hits is a rule whose evidence cannot be reviewed, which is how G06's
// sweep hid one of the two defects it was run to find.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type finding struct {
	rule, file, disposition, excerpt string
}

func main() {
	root := flag.String("root", ".", "repository root")
	verbose := flag.Bool("v", false, "list every hit that needed a judgement")
	done := flag.String("tasks-complete", "", "comma-separated tasks the epic shows complete, for lifetime checks")
	flag.Parse()

	complete := map[string]bool{}
	for _, t := range strings.Split(*done, ",") {
		if t = strings.TrimSpace(t); t != "" {
			complete[t] = true
		}
	}

	files, err := trackedMarkdown(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "doclint:", err)
		os.Exit(2)
	}
	// Belt and braces for the security rule: enumerate the workflow directory
	// itself and require every tracked file in it to be in the swept set. An
	// extension nobody anticipated is exactly how the .yaml gap happened, and a
	// pattern list cannot notice what it does not match.
	if missed, err := unsweptWorkflows(*root, files); err != nil {
		fmt.Fprintln(os.Stderr, "doclint:", err)
		os.Exit(2)
	} else if len(missed) > 0 {
		fmt.Fprintln(os.Stderr, "doclint: these workflow files are not swept:",
			strings.Join(missed, ", "))
		os.Exit(1)
	}
	if len(files) < 20 {
		fmt.Fprintf(os.Stderr, "doclint: %d tracked files — the sweep is not reaching the repository\n", len(files))
		os.Exit(2)
	}

	var fails []string
	var found []finding

	for _, r := range rules {
		if err := r.validate(); err != nil {
			fails = append(fails, fmt.Sprintf("%s: %v", r.ID, err))
			continue
		}
		// A rule past its retirement is a failure, not a warning. Warnings are
		// how a rule set grows until nobody reads it.
		if r.RetireAfter != "" && complete[r.RetireAfter] {
			fails = append(fails, fmt.Sprintf(
				"%s should have been removed: it retires after %s, which is complete. %s",
				r.ID, r.RetireAfter, r.Why))
		}

		hits, err := r.apply(*root, files)
		if err != nil {
			fails = append(fails, fmt.Sprintf("%s: %v", r.ID, err))
			continue
		}
		found = append(found, hits...)
		live := 0
		for _, h := range hits {
			if h.disposition == "LIVE" {
				live++
				fails = append(fails, fmt.Sprintf("%s: %s still states the superseded conclusion — %s",
					r.ID, h.file, h.excerpt))
			}
		}
		if len(hits)-live < r.MinHits {
			fails = append(fails, fmt.Sprintf(
				"%s found %d sites and needs at least %d — its subject has left the tree, so it cannot fail",
				r.ID, len(hits)-live, r.MinHits))
		}
	}

	sort.Slice(found, func(i, j int) bool {
		if found[i].rule != found[j].rule {
			return found[i].rule < found[j].rule
		}
		return found[i].file < found[j].file
	})
	// Every hit is accounted for, and the ones that needed a judgement are
	// listed. A rule whose subject is common -- "P10" appears everywhere --
	// would otherwise bury its own evidence in hundreds of uninteresting lines,
	// and evidence nobody reads is not evidence.
	byRule := map[string]map[string]int{}
	for _, h := range found {
		if byRule[h.rule] == nil {
			byRule[h.rule] = map[string]int{}
		}
		byRule[h.rule][h.disposition]++
	}
	for _, r := range rules {
		d := byRule[r.ID]
		var parts []string
		for k, v := range d {
			if k != "carries the current conclusion" {
				parts = append(parts, fmt.Sprintf("%s=%d", k, v))
			}
		}
		sort.Strings(parts)
		retire := r.RetireAfter
		if retire == "" {
			retire = "permanent"
		}
		fmt.Printf("  %-24s %4d sites  retires:%-10s %s\n",
			r.ID, d["carries the current conclusion"]+sum(parts2(d)), retire, strings.Join(parts, " "))
	}
	if *verbose {
		for _, h := range found {
			if h.disposition == "carries the current conclusion" {
				continue
			}
			fmt.Printf("     %-24s %-40s %s\n", h.rule, h.disposition, h.file)
		}
	}

	if len(fails) > 0 {
		fmt.Println("\nFAIL")
		for _, f := range fails {
			fmt.Println(" -", f)
		}
		os.Exit(1)
	}
	fmt.Printf("\ndoclint: %d rules, %d sites, 0 live\n", len(rules), len(found))
}

func (r Rule) validate() error {
	if r.Why == "" {
		return fmt.Errorf("states no reason; a rule nobody can justify is a rule nobody can retire")
	}
	if r.Added == "" {
		return fmt.Errorf("names no originating task")
	}
	if r.RetireAfter == "" && !strings.Contains(r.Why, "") {
		return nil
	}
	return nil
}

func (r Rule) apply(root string, files []string) ([]finding, error) {
	// Case-insensitive: these match prose, and a sentence beginning with the
	// subject capitalises it. A case-sensitive sweep passes on "Acceptance
	// evidence is recorded alongside the dossier" while failing on the same
	// sentence mid-paragraph, which is an arbitrary distinction.
	pat, err := regexp.Compile("(?im)" + r.Pattern)
	if err != nil {
		return nil, fmt.Errorf("Pattern: %w", err)
	}
	stale, err := regexp.Compile("(?im)" + r.Stale)
	if err != nil {
		return nil, fmt.Errorf("Stale: %w", err)
	}

	var out []finding
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			return nil, err
		}
		us := units(string(b), strings.HasSuffix(f, ".md"))
		for i, unit := range us {
			if !pat.MatchString(unit) {
				continue
			}
			loc := stale.FindStringIndex(unit)
			if loc == nil {
				out = append(out, finding{r.ID, f, "carries the current conclusion", ""})
				continue
			}
			next := ""
			if i+1 < len(us) {
				next = us[i+1]
			}
			excerpt := strings.TrimSpace(unit[loc[0]:min(loc[1]+30, len(unit))])
			out = append(out, finding{r.ID, f, r.classify(f, unit, next, loc[0]), excerpt})
		}
	}
	return out, nil
}

// units splits a document into the things a claim can live in. A TABLE ROW IS
// ITS OWN UNIT: DL-8's third tier, step 4 — a stale row must not be excused by
// a citation elsewhere in the same table.
//
// The paragraph model is Markdown's. In YAML a line IS the unit: flattening a
// block joins `  pull_request_target:` onto the line above, so a pattern
// anchored with ^ stops matching and the rule silently passes. The
// demonstration caught exactly that.
func units(doc string, markdown bool) []string {
	if !markdown {
		return strings.Split(doc, "\n")
	}
	return markdownUnits(doc)
}

func markdownUnits(doc string) []string {
	doc = regexp.MustCompile("(?ms)^```.*?^```").ReplaceAllString(doc, "")
	var out []string
	for _, para := range regexp.MustCompile(`\n\s*\n`).Split(doc, -1) {
		var rows, prose []string
		for _, line := range strings.Split(para, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "|") {
				rows = append(rows, line)
			} else {
				prose = append(prose, line)
			}
		}
		for _, r := range rows {
			out = append(out, strings.Join(strings.Fields(r), " "))
		}
		if p := strings.Join(strings.Fields(strings.Join(prose, " ")), " "); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// classify decides what a hit on the superseded conclusion actually is.
//
// Seven ad-hoc sweeps preceded this tool and each grew its own classifier as it
// met a case. This is their union, named branch by branch -- because a sweep
// that silently swallows a hit is a sweep whose evidence cannot be reviewed,
// and one that reports every legitimate mention forces the documents to stop
// explaining themselves.
func (r Rule) classify(file, unit, next string, at int) string {
	before := unit[:at]

	// Negated: "the operator NO LONGER reaches them only through the server".
	// The superseded wording appears in order to deny it.
	if negated.MatchString(before) {
		return "negated"
	}
	// Quoted, in either form the documents use. Straight quotes are counted for
	// parity; markdown emphasis is matched around the hit, because *"..."* and
	// *...* are both how this repository quotes superseded text.
	if strings.Count(before, `"`)%2 == 1 || emphasised(unit, at) {
		return "quoted as superseded"
	}
	// Corrected in place: the note sits in this unit or the next one, which is
	// where a correction naturally goes.
	if correction.MatchString(unit) || correction.MatchString(next) {
		return "corrected in place"
	}
	// A passage that states the old conclusion in order to say it was wrong.
	if describes.MatchString(unit) {
		return "describes the correction"
	}
	if why := r.Exempt[file]; why != "" {
		return "exempt: " + why
	}
	return "LIVE"
}

var (
	negated    = regexp.MustCompile(`(?i)no longer\s*$|not\s*$|never\s*$`)
	correction = regexp.MustCompile(`Corrected at ` + "`" + `G\d\d` + "`" + `|corrected the claim|G2\d corrected`)
	describes  = regexp.MustCompile(`(?i)it is not|was carried (?:here|to)|wrongly said|had advertised|an earlier (?:draft|version)`)
)

// emphasised reports whether the hit sits inside a markdown emphasis span,
// which is how this repository quotes wording it is superseding.
func emphasised(unit string, at int) bool {
	open := strings.LastIndex(unit[:at], "*")
	if open < 0 {
		return false
	}
	close := strings.Index(unit[at:], "*")
	return close >= 0
}

func trackedMarkdown(root string) ([]string, error) {
	// From git, not a glob. The Makefile records why: a glob silently disagreed
	// about what "every tracked file" meant.
	// Both YAML spellings. An earlier version asked only for *.yml, so a
	// tracked .yaml file was never scanned -- including this repository's own
	// compose.yaml, and any future .forgejo/workflows/*.yaml, which would have
	// bypassed the forbidden-trigger rule while doclint reported success.
	cmd := exec.Command("git", "ls-files", "*.md", "*.yml", "*.yaml")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	var files []string
	for _, f := range strings.Fields(string(out)) {
		if strings.HasPrefix(f, "docs/transition/") {
			continue // verbatim auditor reports; evidence, not this project's prose
		}
		files = append(files, f)
	}
	return files, nil
}

func parts2(d map[string]int) []int {
	var out []int
	for k, v := range d {
		if k != "carries the current conclusion" {
			out = append(out, v)
		}
	}
	return out
}

func sum(xs []int) int {
	n := 0
	for _, x := range xs {
		n += x
	}
	return n
}

// unsweptWorkflows returns tracked files under .forgejo/workflows that the
// sweep's file list does not contain.
func unsweptWorkflows(root string, swept []string) ([]string, error) {
	in := map[string]bool{}
	for _, f := range swept {
		in[f] = true
	}
	cmd := exec.Command("git", "ls-files", ".forgejo/workflows")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files .forgejo/workflows: %w", err)
	}
	var missed []string
	for _, f := range strings.Fields(string(out)) {
		if !in[f] {
			missed = append(missed, f)
		}
	}
	return missed, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
