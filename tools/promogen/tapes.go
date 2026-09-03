// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// typeLine matches a VHS `Type` directive's payload in either quoting
// style. Double quotes carry the commands the viewer sees; backticks
// carry hidden setup (and are still worth checking, since a broken
// setup line breaks the shot just as thoroughly).
//
// Deliberately NOT anchored at end-of-line: a VHS Type directive is
// normally followed by `Enter`, so anchoring matched nothing at all.
var typeLine = regexp.MustCompile("^\\s*Type\\s+(?:\"([^\"]*)\"|`([^`]*)`)")

// TapeCommand is one project-binary invocation found in a tape.
type TapeCommand struct {
	// Tape is the path relative to the promo directory.
	Tape string
	// Line is the 1-indexed line the command appears on.
	Line int
	// Raw is the full command text as typed.
	Raw string
	// Binary is the cmd/ binary being invoked.
	Binary string
	// Path is the subcommand path (e.g. ["audit", "stats"]).
	Path []string
}

// String renders the command path for reporting.
func (c TapeCommand) String() string {
	return strings.TrimSpace(c.Binary + " " + strings.Join(c.Path, " "))
}

// subcommandToken reports whether tok continues a subcommand path.
//
// Paths stop at the first flag or at anything that looks like a file
// argument, so `kscorectl state apply state/web.yaml` yields
// ["state","apply"] and `kscorectl audit stats --since 30m` yields
// ["audit","stats"].
func subcommandToken(tok string) bool {
	if tok == "" || strings.HasPrefix(tok, "-") {
		return false
	}
	if strings.ContainsAny(tok, "/.=$'\"") {
		return false
	}
	return true
}

// ExtractCommands scans a tape for invocations of any binary in bins.
//
// A tape line may chain several commands with `;` or `&&`; each segment
// is considered, so a hidden `cd x; kscorectl ...` setup line is still
// checked.
func ExtractCommands(promoDir, tape string, bins map[string]bool) ([]TapeCommand, error) {
	full := filepath.Join(promoDir, tape)
	// #nosec G304 G703 -- path comes from the validated manifest.
	raw, err := os.ReadFile(full)
	if err != nil {
		return nil, fmt.Errorf("promogen: read tape %s: %w", tape, err)
	}

	var out []TapeCommand
	for i, line := range strings.Split(string(raw), "\n") {
		m := typeLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		payload := m[1]
		if payload == "" {
			payload = m[2]
		}
		for _, seg := range splitSegments(payload) {
			fields := strings.Fields(seg)
			if len(fields) == 0 || !bins[fields[0]] {
				continue
			}
			cmd := TapeCommand{
				Tape:   tape,
				Line:   i + 1,
				Raw:    strings.TrimSpace(seg),
				Binary: fields[0],
			}
			for _, tok := range fields[1:] {
				if !subcommandToken(tok) {
					break
				}
				cmd.Path = append(cmd.Path, tok)
			}
			out = append(out, cmd)
		}
	}
	return out, nil
}

// splitSegments breaks a shell line on `;`, `&&` and `|` so each
// command in a chain is examined on its own.
var segmentSplit = regexp.MustCompile(`;|&&|\|\||\|`)

func splitSegments(line string) []string {
	return segmentSplit.Split(line, -1)
}

// VerifyCommands builds each referenced binary and asserts every
// subcommand path resolves.
//
// This is the guard for the failure mode that actually bites: a tape
// whose command has been renamed or removed still records perfectly
// happily, with the error text sitting in frame. Checking it costs a
// couple of seconds; noticing it after the fact costs a re-render, or
// worse, ships.
//
// Verification runs `<bin> <path...> --help`, which cobra resolves
// without executing anything, so no server or topology is needed.
//
// Plugin dispatch has to be modelled or this reports false failures for
// most of the CLI. kscorectl resolves an unregistered `kscorectl <name>`
// by exec'ing a `kscore-<name>` binary from $PATH, git/kubectl style
// (cmd/kscorectl/main.go). So the sibling is built alongside and the
// probe runs with the build directory on PATH — otherwise every
// delegated subcommand (audit, module, secrets, events, ...) looks
// broken.
func VerifyCommands(repoRoot string, cmds []TapeCommand, bins map[string]bool) ([]string, error) {
	if len(cmds) == 0 {
		return nil, nil
	}

	tmp, err := os.MkdirTemp("", "promogen-tapes-")
	if err != nil {
		return nil, fmt.Errorf("promogen: tempdir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	built := make(map[string]bool)
	build := func(bin string) error {
		if built[bin] {
			return nil
		}
		// #nosec G204 -- bin is a directory name under cmd/, read from
		// the repo's own tree.
		cmd := exec.Command("go", "build", "-o", filepath.Join(tmp, bin), "./"+filepath.Join("cmd", bin))
		cmd.Dir = repoRoot
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("promogen: build %s: %w\n%s", bin, err, out)
		}
		built[bin] = true
		return nil
	}

	// The probe sees only the freshly built binaries, so a plugin that
	// happens to be installed on the developer's machine cannot mask a
	// tape referencing a subcommand this tree does not provide.
	probeEnv := append(os.Environ(), "PATH="+tmp)

	var problems []string
	for _, c := range cmds {
		if err := build(c.Binary); err != nil {
			return nil, err
		}
		// Build the plugin sibling too when the tape's first path
		// element names one. Membership in bins (the set of directories
		// under cmd/) is the test, rather than stat'ing a path built
		// from tape text — the name never becomes a filesystem path
		// unless it is already a known binary.
		if len(c.Path) > 0 {
			if sibling := "kscore-" + c.Path[0]; bins[sibling] {
				if err := build(sibling); err != nil {
					return nil, err
				}
			}
		}

		args := append(append([]string{}, c.Path...), "--help")
		// #nosec G204 -- the binary was just built from cmd/, and args
		// are subcommand tokens extracted from a tape.
		probe := exec.Command(filepath.Join(tmp, c.Binary), args...)
		probe.Env = probeEnv
		out, err := probe.CombinedOutput()
		if err != nil {
			problems = append(problems, fmt.Sprintf(
				"%s:%d: `%s` does not resolve — %s",
				c.Tape, c.Line, c.String(), firstUsefulLine(string(out))))
			continue
		}
		// Exit status alone is not enough. Given an unrecognised
		// trailing token, cobra prints the PARENT's help and exits 0
		// — `kscorectl state aply --help` happily renders the help for
		// `state`. So confirm the rendered Usage actually names the
		// command that was asked for.
		if len(c.Path) > 0 {
			want := c.Path[len(c.Path)-1]
			if !usageMentions(string(out), want) {
				problems = append(problems, fmt.Sprintf(
					"%s:%d: `%s` resolved to a different command — %q is absent from "+
						"its usage line, so cobra fell back to the parent's help",
					c.Tape, c.Line, c.String(), want))
			}
		}
	}
	return problems, nil
}

// firstUsefulLine picks the cobra error line out of a help dump.
func firstUsefulLine(out string) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Error:") || strings.HasPrefix(line, "error:") {
			return line
		}
	}
	if line, _, ok := strings.Cut(strings.TrimSpace(out), "\n"); ok {
		return line
	}
	return strings.TrimSpace(out)
}

// ProjectBinaries returns the set of cmd/* binary names.
func ProjectBinaries(repoRoot string) (map[string]bool, error) {
	entries, err := os.ReadDir(filepath.Join(repoRoot, "cmd"))
	if err != nil {
		return nil, fmt.Errorf("promogen: read cmd/: %w", err)
	}
	bins := make(map[string]bool, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			bins[e.Name()] = true
		}
	}
	return bins, nil
}

// SortedNames renders a binary set deterministically, for reporting.
func SortedNames(bins map[string]bool) []string {
	out := make([]string, 0, len(bins))
	for b := range bins {
		out = append(out, b)
	}
	sort.Strings(out)
	return out
}

// usageMentions reports whether a --help dump's Usage block names want.
//
// cobra renders `Usage:` followed by one or more indented usage lines;
// only that block is considered, since the long description or an
// Available Commands list may mention a sibling command by name and
// would otherwise mask the fallback this is looking for.
func usageMentions(help, want string) bool {
	lines := strings.Split(help, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "Usage:" {
			continue
		}
		for _, u := range lines[i+1:] {
			if strings.TrimSpace(u) == "" {
				return false
			}
			for _, tok := range strings.Fields(u) {
				if tok == want {
					return true
				}
			}
		}
	}
	return false
}
