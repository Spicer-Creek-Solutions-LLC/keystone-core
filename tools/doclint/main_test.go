package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestApprovedDocumentsReportsUnapprovedTextButAllowsApprovedEdits(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "not-a-repository"))
	runGit(t, root, "init")
	writeApprovalDocument(t, root, "approved text\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "approved")

	writeApprovalDocument(t, root, "approved text\nG26 edit\nG27 edit\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "approved later edits")
	approvedLater := gitOutput(t, root, "rev-parse", "HEAD")

	writeApprovalDocument(t, root, "approved text\nG26 edit\nG27 edit\nC01 therefore builds the assertion for its own deferral rather than reusing one.\nwhich target asserts it was never this dossier's to fix\n")
	writeManifest(t, root, approvalManifestFor(t, root, approvedLater))

	findings, err := checkApprovedDocuments(root, "approved.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || !containsNormalized(findings[0], "C01 therefore builds the assertion for its own deferral rather than reusing one.") || !containsNormalized(findings[0], "which target asserts it was never this dossier's to fix") {
		t.Fatalf("findings = %q, want both G33 sentences", findings)
	}

	writeApprovalDocument(t, root, "approved text\nG26 edit\nG27 edit\nG34 correction\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "approved correction")
	approvedCorrection := gitOutput(t, root, "rev-parse", "HEAD")
	writeManifest(t, root, approvalManifestFor(t, root, approvedCorrection))
	findings, err = checkApprovedDocuments(root, "approved.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings after approved correction = %q", findings)
	}
}

func TestHistoricalC01ApprovalCase(t *testing.T) {
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "not-a-repository"))
	repo := gitOutput(t, ".", "rev-parse", "--show-toplevel")
	originalText := gitShow(t, repo, "743d32868:docs/dossiers/C01.md")
	g26Text := gitShow(t, repo, "9944e35ea:docs/dossiers/C01.md")
	g27Text := gitShow(t, repo, "88ae5d1bb:docs/dossiers/C01.md")
	g33Text := gitShow(t, repo, "72cd2f9a2:docs/dossiers/C01.md")
	g34Text := gitShow(t, repo, "028019924a359328e11ad19d8d3a1d67f5c30a43:docs/dossiers/C01.md")

	root := t.TempDir()
	runGit(t, root, "init")
	writeApprovalDocument(t, root, originalText)
	runGit(t, root, "add", ".")
	runGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "approved C01 snapshot")

	writeApprovalDocument(t, root, g26Text)
	runGit(t, root, "add", ".")
	runGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "approved G26 snapshot")

	writeApprovalDocument(t, root, g27Text)
	runGit(t, root, "add", ".")
	runGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "approved G27 snapshot")
	approvedCommit := gitOutput(t, root, "rev-parse", "HEAD")
	writeManifest(t, root, approvalManifestFor(t, root, approvedCommit))
	findings, err := checkApprovedDocuments(root, "approved.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings for sanctioned G26/G27 sequence = %q", findings)
	}

	writeApprovalDocument(t, root, g33Text)
	writeManifest(t, root, approvalManifestFor(t, root, approvedCommit))
	findings, err = checkApprovedDocuments(root, "approved.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || !containsNormalized(findings[0], "C01 therefore builds the assertion for its own deferral rather than reusing one.") || !containsNormalized(findings[0], "which target asserts it was never this dossier's to fix") {
		t.Fatalf("historical findings = %q, want both G33 sentences", findings)
	}

	writeApprovalDocument(t, root, g34Text)
	runGit(t, root, "add", ".")
	runGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "approved G34 snapshot")
	approvedG34 := gitOutput(t, root, "rev-parse", "HEAD")
	writeManifest(t, root, approvalManifestFor(t, root, approvedG34))
	findings, err = checkApprovedDocuments(root, "approved.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("historical findings after G34 = %q", findings)
	}
}

func writeApprovalDocument(t *testing.T, root, content string) {
	t.Helper()
	path := filepath.Join(root, "docs/dossiers")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "C01.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeManifest(t *testing.T, root string, manifest approvalManifest) {
	t.Helper()
	b, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "approved.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func approvalManifestFor(t *testing.T, root, commit string) approvalManifest {
	t.Helper()
	b, err := gitAt(root, "show", commit+":docs/dossiers/C01.md").Output()
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(b)
	return approvalManifest{Documents: []approvedDocument{{
		Path:   "docs/dossiers/C01.md",
		Commit: commit,
		Digest: hex.EncodeToString(digest[:]),
	}}}
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := gitAt(root, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func gitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := gitAt(root, args...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func gitShow(t *testing.T, root, object string) string {
	t.Helper()
	cmd := gitAt(root, "show", object)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git show %s: %v", object, err)
	}
	return string(out)
}

func containsNormalized(text, phrase string) bool {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		lines = append(lines, strings.TrimPrefix(strings.TrimPrefix(line, "+"), "-"))
	}
	normalized := strings.Join(strings.Fields(strings.Join(lines, " ")), " ")
	wanted := strings.Join(strings.Fields(phrase), " ")
	return strings.Contains(normalized, wanted)
}

// The classifiers decide whether a hit on a superseded conclusion is reported.
// A classifier that fires too readily disposes of live hits silently, which is
// the failure mode the whole tool exists to avoid -- so each one is tested in
// both directions, and the negative case is the one that matters.
func TestDescribesRequiresWordBoundaries(t *testing.T) {
	for _, c := range []struct {
		text  string
		match bool
		why   string
	}{
		{"it is not, and G22 corrected that claim", true, "the case the classifier exists for"},
		{"It Is Not the same thing", true, "case-insensitive"},
		{"it is notable that the runner queued", false, "review of #339: `not` is a prefix of `notable`"},
		{"an earlier draft said otherwise", true, ""},
		{"an earlier versioned protocol", false, "`version` is a prefix of `versioned`"},
		{"was carried here from P08", true, ""},
		{"was carried today by the agent", false, "`to` is a prefix of `today`"},
	} {
		if got := describes.MatchString(c.text); got != c.match {
			t.Errorf("describes(%q) = %v, want %v  %s", c.text, got, c.match, c.why)
		}
	}
}

func TestNegatedRequiresWordBoundaries(t *testing.T) {
	for _, c := range []struct {
		text  string
		match bool
	}{
		{"the operator no longer", true},
		{"this is not", true},
		{"a job cannot", false}, // `not` is the tail of `cannot`
		{"the run was never", true},
		{"whenever", false}, // `never` is the tail of `whenever`
	} {
		if got := negated.MatchString(c.text); got != c.match {
			t.Errorf("negated(%q) = %v, want %v", c.text, got, c.match)
		}
	}
}

// G28: bold is assertion in this repository, not quotation. Treating it as
// quotation meant the claims most worth sweeping were the ones no rule could
// fire on.
func TestEmphasisedTreatsBoldAsAssertion(t *testing.T) {
	for _, c := range []struct {
		unit string
		hit  string
		want bool
	}{
		{"read *one file per task* and it is wrong", "one file per task", true},
		{"**No runner available can run containers**, so", "No runner", false},
		{"a **bold** phrase then the claim runs containers here", "runs containers", false},
		{"*quoted* then plain then *quoted again*", "then plain then", false},
	} {
		at := strings.Index(c.unit, c.hit)
		if at < 0 {
			t.Fatalf("test is wrong: %q not in %q", c.hit, c.unit)
		}
		if got := emphasised(c.unit, at); got != c.want {
			t.Errorf("emphasised(%q, hit=%q) = %v, want %v", c.unit, c.hit, got, c.want)
		}
	}
}

// A control character inside a regex literal is invisible in every editor and
// silently kills the alternative it lands in. `describes` carried two literal
// backspaces from P11b until G28. `make whitespace-check` guards the tree; this
// guards the patterns the tool actually compiles.
func TestNoControlCharactersInPatterns(t *testing.T) {
	ctrl := regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)
	check := func(name, pat string) {
		if loc := ctrl.FindStringIndex(pat); loc != nil {
			t.Errorf("%s holds a control character at byte %d: %q", name, loc[0], pat)
		}
	}
	check("negated", negated.String())
	check("correction", correction.String())
	check("describes", describes.String())
	for _, r := range rules {
		check(r.ID+".Pattern", r.Pattern)
		check(r.ID+".Stale", r.Stale)
	}
}

// G30: doclint enumerates the files it sweeps with `git ls-files`. With GIT_DIR
// set it listed the environment's repository instead of its root -- so the rules
// would sweep the wrong tree and report `0 live` while checking nothing.
func TestTrackedMarkdownIgnoresAmbientGitDir(t *testing.T) {
	t.Setenv("GIT_DIR", realGitDir(t))
	if _, err := trackedMarkdown(t.TempDir()); err == nil {
		t.Fatal("GIT_DIR reached git: doclint enumerated the environment's repository, not its root")
	}
}

// realGitDir returns this checkout's git directory, or skips. It is what makes
// the GIT_DIR tests meaningful: pointing GIT_DIR at a non-repository would make
// git fail for the wrong reason and the test would pass without the fix.
func realGitDir(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--absolute-git-dir").Output()
	if err != nil {
		t.Skip("not running inside a git checkout")
	}
	return strings.TrimSpace(string(out))
}

func TestWithoutGitEnvStripsDiscoveryVars(t *testing.T) {
	env := []string{"PATH=/bin", "GIT_DIR=/x", "GIT_WORK_TREE=/y", "HOME=/h", "GITHUB_TOKEN=keep"}
	got := strings.Join(withoutGitEnv(env), " ")
	for _, bad := range []string{"GIT_DIR=", "GIT_WORK_TREE="} {
		if strings.Contains(got, bad) {
			t.Errorf("withoutGitEnv kept %q: %s", bad, got)
		}
	}
	for _, keep := range []string{"PATH=/bin", "HOME=/h", "GITHUB_TOKEN=keep"} {
		if !strings.Contains(got, keep) {
			t.Errorf("withoutGitEnv dropped %q: %s", keep, got)
		}
	}
	// A variable whose name merely starts with one of the stripped names must
	// survive: prefix matching without the `=` would take GIT_DIRECTORY too.
	if kept := withoutGitEnv([]string{"GIT_DIRECTORY=/z"}); len(kept) != 1 {
		t.Errorf("withoutGitEnv stripped GIT_DIRECTORY, which is a different variable")
	}
}

// G37 review: `primitives-are-c01s` shipped with its Pattern and Stale halves
// naming different vocabularies, twice and in opposite directions. The first
// draft's subject list was narrower, so `cipher`, `curve` and `signature scheme`
// claims matched no subject. Widening Pattern introduced the mirror: Stale still
// listed only the generic words, so a claim naming a CONCRETE primitive -- the
// most direct form of the claim -- passed.
//
// Planting in a shell caught the first and was not checked in, which is why the
// second survived to review. These cases are the demonstration, in the tree.
func TestPrimitiveOwnershipRuleCatchesConcreteNames(t *testing.T) {
	var r Rule
	for _, candidate := range rules {
		if candidate.ID == "primitives-are-c01s" {
			r = candidate
		}
	}
	if r.ID == "" {
		t.Fatal("primitives-are-c01s is not in rules")
	}
	subject := regexp.MustCompile(r.Pattern)
	stale := regexp.MustCompile(r.Stale)

	mustFire := []string{
		// concrete names -- the class review found missing
		"C01 chooses ML-KEM-768.",
		"Ed25519 is C01's choice.",
		"C01 picks X25519 for the KEM.",
		"ML-DSA-65 is C01 to settle.",
		"The AES-256-GCM construction is C01 to decide.",
		// generic nouns -- the class the first draft missed
		"C01 chooses the signature algorithm and the encryption primitives.",
		"The signature scheme is C01 to settle, and the cipher is C01 as well.",
		"The primitives are to be decided by C01 at implementation time.",
		"C01 picks the curve and the signature scheme.",
	}
	for _, text := range mustFire {
		if !subject.MatchString(text) {
			t.Errorf("subject did not match, so the rule never looks: %q", text)
			continue
		}
		if !stale.MatchString(text) {
			t.Errorf("stale conclusion not caught: %q", text)
		}
	}

	// The corrected text, and ordinary correct statements, must not fire.
	mustNotFire := []string{
		"C01 implements Ed25519 and ML-KEM-768; the bytes are this ADR's.",
		"the canonical encoder's implementation; the signature and encryption " +
			"operations as code and not as choices, since sections 4 and 5 make " +
			"the choices; fuzzing; and the computed vectors (C01).",
		"ADR-0005 section 4 chooses Ed25519 and ML-DSA-65.",
	}
	for _, text := range mustNotFire {
		if stale.MatchString(text) {
			t.Errorf("fired on correct text: %q", text)
		}
	}
}
