// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTape(t *testing.T, body string) (promoDir, name string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "tapes"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	name = filepath.Join("tapes", "x.tape")
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write tape: %v", err)
	}
	return dir, name
}

var testBins = map[string]bool{"kscorectl": true, "kscore-audit": true}

func TestExtractCommands(t *testing.T) {
	// Mirrors the real tapes: a hidden backtick setup line, a visible
	// double-quoted command, a chained line, and noise that must not
	// be picked up.
	body := "# a comment mentioning kscorectl state apply\n" +
		"Output build/promo/clips/x.mp4\n" +
		"Hide\n" +
		"Type `cd assets/promo/scenario; PS1=\"$ \"; clear` Enter\n" +
		"Show\n" +
		"Type \"kscorectl state apply state/web.yaml\" Enter\n" +
		"Type \"kscorectl audit stats --since 30m\" Enter\n" +
		"Type `bash drift.sh >/dev/null` Enter\n" +
		"Sleep 3s\n"

	dir, name := writeTape(t, body)
	got, err := ExtractCommands(dir, name, testBins)
	if err != nil {
		t.Fatalf("ExtractCommands: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d commands, want 2: %+v", len(got), got)
	}
	if got[0].String() != "kscorectl state apply" {
		t.Errorf("[0] = %q, want \"kscorectl state apply\"", got[0].String())
	}
	if got[0].Line != 6 {
		t.Errorf("[0].Line = %d, want 6", got[0].Line)
	}
	// --since is a flag, so the path must stop before it.
	if got[1].String() != "kscorectl audit stats" {
		t.Errorf("[1] = %q, want \"kscorectl audit stats\"", got[1].String())
	}
}

// A VHS Type directive is normally followed by `Enter`. Anchoring the
// regex at end-of-line matched nothing at all, which the "no commands
// found" guard in cmdTapes exists to catch.
func TestExtractCommands_TrailingEnter(t *testing.T) {
	dir, name := writeTape(t, "Type \"kscorectl state drift f.yaml\" Enter\n")
	got, err := ExtractCommands(dir, name, testBins)
	if err != nil {
		t.Fatalf("ExtractCommands: %v", err)
	}
	if len(got) != 1 || got[0].String() != "kscorectl state drift" {
		t.Fatalf("got %+v, want one kscorectl state drift", got)
	}
}

func TestExtractCommands_ChainedSegments(t *testing.T) {
	dir, name := writeTape(t, "Type `cd x && kscorectl agent list` Enter\n")
	got, err := ExtractCommands(dir, name, testBins)
	if err != nil {
		t.Fatalf("ExtractCommands: %v", err)
	}
	if len(got) != 1 || got[0].String() != "kscorectl agent list" {
		t.Fatalf("got %+v, want the chained kscorectl command", got)
	}
}

func TestExtractCommands_MissingTape(t *testing.T) {
	if _, err := ExtractCommands(t.TempDir(), "tapes/absent.tape", testBins); err == nil {
		t.Error("ExtractCommands(missing) = nil error, want error")
	}
}

func TestSubcommandToken(t *testing.T) {
	tests := []struct {
		tok  string
		want bool
	}{
		{"apply", true},
		{"drift", true},
		{"", false},
		{"--fix", false},
		{"-o", false},
		{"state/web.yaml", false},
		{"web.yaml", false},
		{"KEY=value", false},
		{"$HOME", false},
		{`"quoted"`, false},
	}
	for _, tt := range tests {
		if got := subcommandToken(tt.tok); got != tt.want {
			t.Errorf("subcommandToken(%q) = %v, want %v", tt.tok, got, tt.want)
		}
	}
}

// Exit status alone does not catch a typo: given an unrecognised
// trailing token cobra prints the PARENT's help and exits 0. The usage
// line is what distinguishes "resolved" from "fell back".
func TestUsageMentions(t *testing.T) {
	applyHelp := "Apply a YAML state file\n\nUsage:\n  kscorectl state apply <file> [flags]\n\nFlags:\n  -h, --help\n"
	parentHelp := "Declarative state management\n\nUsage:\n  kscorectl state [command]\n\nAvailable Commands:\n  apply       Apply a YAML state file\n"

	if !usageMentions(applyHelp, "apply") {
		t.Error("usageMentions(apply help, \"apply\") = false, want true")
	}
	// "apply" appears in the parent's Available Commands list but not
	// in its Usage block; counting that would defeat the check.
	if usageMentions(parentHelp, "apply") {
		t.Error("usageMentions(parent help, \"apply\") = true, want false — " +
			"only the Usage block counts")
	}
	if usageMentions(parentHelp, "aply") {
		t.Error("usageMentions(parent help, \"aply\") = true, want false")
	}
	if usageMentions("no usage block here", "apply") {
		t.Error("usageMentions(no usage block) = true, want false")
	}
}

func TestFirstUsefulLine(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Usage:\n  x\nError: unknown command \"list\"\n", `Error: unknown command "list"`},
		{"error: bad thing\n", "error: bad thing"},
		{"just one line", "just one line"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := firstUsefulLine(tt.in); got != tt.want {
			t.Errorf("firstUsefulLine(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestProjectBinaries(t *testing.T) {
	bins, err := ProjectBinaries(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("ProjectBinaries: %v", err)
	}
	for _, want := range []string{"kscorectl", "kscore-audit", "kscore-server"} {
		if !bins[want] {
			t.Errorf("ProjectBinaries() missing %q", want)
		}
	}
	if names := SortedNames(bins); len(names) < 2 || !sortedAscending(names) {
		t.Errorf("SortedNames() = %v, want sorted", names)
	}
}

func sortedAscending(s []string) bool {
	for i := 1; i < len(s); i++ {
		if s[i-1] > s[i] {
			return false
		}
	}
	return true
}

// The repo's own tapes must always yield extractable commands. This is
// the parsing half only, deliberately.
//
// Resolving them means building cmd/ binaries, and invoking the Go
// toolchain from inside `go test -race ./...` makes the unit suite
// depend on having a toolchain and spare memory while ~100 packages
// compile in parallel. That guarantee is already enforced where it
// belongs: `make promo-check` runs `promogen tapes` in CI, and
// `make promo` runs it again before spending three minutes rendering.
// Duplicating it here buys nothing and costs the whole suite.
func TestRepoTapesYieldCommands(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	promoDir := filepath.Join(repoRoot, defaultPromoDir)

	m, err := LoadManifest(filepath.Join(promoDir, manifestName))
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	bins, err := ProjectBinaries(repoRoot)
	if err != nil {
		t.Fatalf("ProjectBinaries: %v", err)
	}

	var all []TapeCommand
	for _, shot := range m.Shots {
		cmds, err := ExtractCommands(promoDir, shot.Tape, bins)
		if err != nil {
			t.Fatalf("ExtractCommands(%s): %v", shot.Tape, err)
		}
		all = append(all, cmds...)
	}
	if len(all) == 0 {
		t.Fatal("no project-binary commands extracted from the repo's tapes — " +
			"the Type-directive parser has probably broken, which would make " +
			"`promogen tapes` silently vacuous")
	}
	for _, c := range all {
		if c.Binary == "" || len(c.Path) == 0 {
			t.Errorf("%s:%d extracted with an empty binary or path: %+v", c.Tape, c.Line, c)
		}
	}
}
