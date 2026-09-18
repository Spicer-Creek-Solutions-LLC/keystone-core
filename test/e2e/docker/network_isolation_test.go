// Package docker holds the acceptance harness's container-level tests.
//
// This file is the registered evidence for ARCH-COMM-002. ADR-0010 § 2 requires
// the isolation to be PROVED BY A PROBE rather than asserted -- "a topology that
// merely omits a route passes vacuously the day someone adds one" -- so the test
// below does two things, and the second is the one that matters:
//
//  1. containers on separate networks cannot reach each other; and
//  2. the same probe, run against a deliberately joined pair, REPORTS THEM
//     REACHABLE.
//
// Without (2) the probe would pass on a topology with a route and prove nothing.
// A check demonstrated only in its passing direction is the defect this stage
// has spent several tasks finding.
package docker

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// What this file does NOT establish, stated rather than implied: that the
// product works across this topology. ARCH-COMM-002 requires the server and
// agents to FUNCTION without a direct route, and nothing here runs a journey --
// no NATS connection exists until C06, and no journey until C05. This proves the
// arrangement. ADR-0010 § 1 records the distinction.

// label marks everything this suite creates.
//
// Under the shape CI-RUNNER.md § "How a job gets Docker" chose, the job talks to
// the HOST's daemon, so containers and networks made here are siblings of the
// job's own container and outlive it. That document states the consequence
// plainly -- "the suite must tear down what it creates, including when it
// fails" -- and a label is how that is discharged, because it survives what
// t.Cleanup does not: a timeout panic, a SIGKILL, or a run this forge cancels
// because another push arrived.
const label = "keystone-acceptance=1"

// requireDockerEnv makes the gate path refuse to skip.
//
// G33: this used to be an unconditional t.Skip when the client or the daemon was
// missing, and `container-suite` would then have gone green on a CI job whose
// client install had silently failed. That is DL-1 -- a check that cannot fail,
// mistaken for evidence. The Makefile and the workflow both set this; a
// developer invoking `go test ./test/...` by hand still gets the skip.
const requireDockerEnv = "KEYSTONE_REQUIRE_DOCKER"

// dockerCmd is the raw invocation, with no *testing.T. TestMain sweeps before
// any test exists, so the helpers it needs cannot depend on one.
func dockerCmd(args ...string) (string, error) {
	out, err := exec.Command("docker", args...).CombinedOutput()
	return string(out), err
}

func docker(t *testing.T, args ...string) (string, error) {
	t.Helper()
	return dockerCmd(args...)
}

func mustDocker(t *testing.T, args ...string) string {
	t.Helper()
	out, err := docker(t, args...)
	if err != nil {
		t.Fatalf("docker %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

// unavailable reports why Docker cannot be used here, or "" when it can. It is
// one function so that the skip and the failure below cannot diverge in what
// they consider available -- two conditions would be two things to drift.
func unavailable() string {
	if _, err := exec.LookPath("docker"); err != nil {
		return "no docker client on PATH"
	}
	if out, err := dockerCmd("info", "--format", "{{.ServerVersion}}"); err != nil {
		return fmt.Sprintf("no reachable docker daemon: %v\n%s", err, out)
	}
	return ""
}

func requireDocker(t *testing.T) {
	t.Helper()
	why := unavailable()
	if why == "" {
		return
	}
	if os.Getenv(requireDockerEnv) != "" {
		// Naming the variable matters: this failure is read in a CI log by
		// someone deciding whether the install step or the socket is at fault,
		// and "docker not found" alone does not separate them.
		t.Fatalf("%s is set, so this gate must not skip: %s\n"+
			"This is the container suite's gate path (make container-suite). "+
			"Either the pinned Docker client failed to install or the host's "+
			"socket is not reachable from this job; see docs/project/CI-RUNNER.md.",
			requireDockerEnv, why)
	}
	t.Skipf("%s (set %s to make this a failure instead)", why, requireDockerEnv)
}

// sweep removes everything this suite has ever labelled.
//
// Errors are ignored deliberately: there is nothing to remove on a clean host,
// and a sweep that failed the suite for finding nothing would be a gate failing
// on success. It removes by label rather than by name, so it collects residue
// this process never created.
//
// No test here calls t.Parallel(), so a sweep cannot remove another test's
// containers while they are in use. A parallel test added later would have to
// scope the label per test rather than per suite.
func sweep() {
	if out, err := dockerCmd("ps", "-aq", "--filter", "label="+label); err == nil {
		if ids := strings.Fields(out); len(ids) > 0 {
			_, _ = dockerCmd(append([]string{"rm", "-f"}, ids...)...)
		}
	}
	if out, err := dockerCmd("network", "ls", "-q", "--filter", "label="+label); err == nil {
		if ids := strings.Fields(out); len(ids) > 0 {
			_, _ = dockerCmd(append([]string{"network", "rm"}, ids...)...)
		}
	}
}

// TestMain sweeps before any test runs and after all of them have.
//
// The entry sweep is the half t.Cleanup cannot provide: it removes what a run
// killed mid-test left behind, which the next run would otherwise meet as a name
// collision on `docker run --name`. The exit sweep is belt to t.Cleanup's
// braces, and catches a test that failed before registering one.
func TestMain(m *testing.M) {
	if unavailable() == "" {
		sweep()
	}
	code := m.Run()
	if unavailable() == "" {
		sweep()
	}
	os.Exit(code)
}

// reachable runs the probe: can a container on netA reach one on netB by name?
// It is the single piece of logic both directions below exercise, so that the
// negative case tests the probe rather than a paraphrase of it.
//
// Names carry the process id so that two runs on one host -- this forge runs a
// single runner serially, but a developer's machine has no such rule -- do not
// collide on `docker run --name`.
func reachable(t *testing.T, prefix string, joined bool) bool {
	t.Helper()
	run := fmt.Sprintf("%s-%d", prefix, os.Getpid())
	netA, netB := run+"-a", run+"-b"
	c1, c2 := run+"-1", run+"-2"

	t.Cleanup(sweep)

	mustDocker(t, "network", "create", "--label", label, netA)
	if !joined {
		mustDocker(t, "network", "create", "--label", label, netB)
	}
	second := netA
	if !joined {
		second = netB
	}
	mustDocker(t, "run", "-d", "--label", label, "--name", c1, "--network", netA, "alpine:3", "sleep", "60")
	mustDocker(t, "run", "-d", "--label", label, "--name", c2, "--network", second, "alpine:3", "sleep", "60")

	_, err := docker(t, "exec", c1, "ping", "-c1", "-W2", c2)
	return err == nil
}

// The property ARCH-COMM-002's arrangement rests on.
func TestSeparateNetworksAreNotReachable(t *testing.T) {
	requireDocker(t)
	if reachable(t, "ks-iso", false) {
		t.Error("a container reached one on a separate network; the topology is not isolated")
	}
}

// The probe can fail. Run against a pair on ONE network, the same logic must
// report them reachable -- otherwise the test above would pass on any topology,
// including one with a route, and would be evidence of nothing.
func TestTheProbeDetectsAJoinedPair(t *testing.T) {
	requireDocker(t)
	if !reachable(t, "ks-join", true) {
		t.Error("the probe reported a joined pair unreachable; it cannot detect a route, " +
			"so its passing result proves nothing")
	}
}
