package cli

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// P11a lands no command-execution behaviour (P11 § 5.2, AC-9). That is a claim
// about the whole module, so it is checked against the whole module rather than
// asserted in prose.
//
// It is a point-in-time check on a boundary that erodes by increments, which
// P11's § 5.3 records. tools/archlint is where it becomes a standing rule.

// Imports that would mean a binary can reach the broker, the operator socket, or
// a child process. Each is a capability a C task introduces deliberately.
var forbidden = map[string]string{
	"github.com/nats-io/nats.go": "a NATS connection is C06's",
	"github.com/nats-io/jsm.go":  "JetStream management is C06's",
	"os/exec":                    "process execution is C07's",
	"golang.org/x/sys/unix":      "SO_PEERCRED is confined to C04's operator substrate; identity changes are C07's",
}

func moduleGoFiles(t *testing.T) []string {
	t.Helper()
	root := filepath.Join("..", "..")
	var out []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// tools/ are separate modules with their own lifetimes, and the
			// archive is not ours. Skipping them is stated, not silent.
			switch d.Name() {
			case ".git", "docs", "tools", "epics":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ".go") {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module: %v", err)
	}
	if len(out) < 5 {
		t.Fatalf("found %d Go files; the walk is not reaching the module", len(out))
	}
	return out
}

func TestNoBinaryCanReachTheBrokerTheSocketOrAProcess(t *testing.T) {
	fset := token.NewFileSet()
	for _, p := range moduleGoFiles(t) {
		f, err := parser.ParseFile(fset, p, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parsing %s: %v", p, err)
		}
		// The boundary is about what the PRODUCT can do. A test may shell out
		// -- version_test.go runs `git rev-parse` because AC-8 requires the
		// version to be derived rather than typed, and forbidding that would
		// push the test toward the literal it exists to reject. The loosening
		// is scoped to _test.go and stated rather than silent.
		isTest := strings.HasSuffix(p, "_test.go")
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			operatorFile := strings.Contains(filepath.ToSlash(p), "/internal/operator/")
			if why, bad := forbidden[path]; bad && !isTest && !(operatorFile && path == "golang.org/x/sys/unix") {
				t.Errorf("%s imports %q: %s", p, path, why)
			}
			if path == "net" && !isTest && !operatorFile {
				t.Errorf("%s imports net: Unix networking is confined to C04's operator substrate", p)
			}
		}
	}
}

// No journey verb is wired before the task that builds it. The charter's seven
// journeys are the surface C-stage tasks build; a stub would have to misuse an
// exit code to refuse one.
func TestNoJourneyVerbIsWired(t *testing.T) {
	// C05 wires `enroll` and nothing else (D-C05-7).
	verbs := []string{`"run"`, `"status"`, `"output"`, `"cancel"`, `"audit"`, `"agents"`}
	for _, p := range moduleGoFiles(t) {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		for _, v := range verbs {
			// A verb is wired when it appears as a string literal being
			// dispatched on. Prose in a comment is not a command.
			for _, line := range strings.Split(src, "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "//") {
					continue
				}
				if strings.Contains(line, v) {
					t.Errorf("%s appears to wire the journey verb %s: %q", p, v, trimmed)
				}
			}
		}
	}
}
