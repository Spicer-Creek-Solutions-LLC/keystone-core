package docker

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The compose plugin is installed by the CI job, and this is what makes that
// installation load-bearing rather than decorative.
//
// CI-RUNNER.md § Verification step 1 settles R6 with `docker compose version`,
// which proves the plugin resolves and nothing more. A tool installed only to
// print its own version is the shape this repository keeps rejecting elsewhere,
// so the plugin is also USED here: `docker compose config` parses and validates
// the tracked topology.
//
// Nothing in `make check` looked at compose.yaml before G33. It has been in the
// tree since P11b, and a file no gate reads is a file that drifts.

// composeFile is resolved from this source file rather than from the working
// directory. `go test` runs in the package directory, so a bare "compose.yaml"
// works under the gate and fails anywhere else -- a trap for a later harness
// that runs a compiled suite, which ADR-0010 § 1 leaves open.
var composeFile = func() string {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		return "compose.yaml"
	}
	return filepath.Join(filepath.Dir(self), "compose.yaml")
}()

func composeConfig(t *testing.T, args ...string) (string, error) {
	t.Helper()
	full := append([]string{"compose", "-f", composeFile}, args...)
	out, err := exec.Command("docker", full...).CombinedOutput()
	return string(out), err
}

// TestComposeTopologyIsValid parses the topology. `config` resolves the file and
// renders it; it starts nothing, pulls nothing and builds nothing, so this costs
// no containers and needs no teardown.
func TestComposeTopologyIsValid(t *testing.T) {
	requireDocker(t)

	out, err := composeConfig(t, "config")
	if err != nil {
		t.Fatalf("docker compose config rejected %s: %v\n%s", composeFile, err, out)
	}

	// The two properties TESTING.md § "Required test topology" asks of this file
	// and that a parse alone would not notice. Asserted against the RENDERED
	// config rather than the source, so a value reaching the daemon differently
	// than it reads in the file is caught.
	for _, svc := range []string{"broker", "server", "agent-1", "agent-2", "operator"} {
		if !strings.Contains(out, svc+":") {
			t.Errorf("rendered topology is missing the %q service", svc)
		}
	}

	// TESTING.md requires that no agent publishes a host port. `config` renders
	// published ports under a `published:` key, so its absence is the assertion.
	if strings.Contains(out, "published:") {
		t.Errorf("a service publishes a host port; TESTING.md requires none.\n%s", out)
	}
}

// TestComposeConfigDetectsABrokenTopology is the negative direction, for the
// same reason the isolation probe has one: a validator demonstrated only on a
// file that passes would report success on a file it never really read.
func TestComposeConfigDetectsABrokenTopology(t *testing.T) {
	requireDocker(t)

	// A service naming a network the file does not define. Compose resolves
	// network references, so this is rejected at `config` time -- no daemon
	// state is created either way.
	broken := `
name: keystone-acceptance-negative
services:
  lonely:
    image: alpine:3
    networks: [does-not-exist]
networks:
  defined-but-unused:
    internal: true
`
	cmd := exec.Command("docker", "compose", "-f", "-", "config")
	cmd.Stdin = strings.NewReader(broken)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Errorf("docker compose config accepted a service on an undefined network, "+
			"so its acceptance of %s proves nothing\n%s", composeFile, out)
	}
}
