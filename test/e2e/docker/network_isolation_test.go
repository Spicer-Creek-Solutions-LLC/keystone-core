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
	"os/exec"
	"strings"
	"testing"
)

// What this file does NOT establish, stated rather than implied: that the
// product works across this topology. ARCH-COMM-002 requires the server and
// agents to FUNCTION without a direct route, and nothing here runs a journey --
// no NATS connection exists until C06, and no journey until C05. This proves the
// arrangement. ADR-0010 § 1 records the distinction.

func docker(t *testing.T, args ...string) (string, error) {
	t.Helper()
	out, err := exec.Command("docker", args...).CombinedOutput()
	return string(out), err
}

func mustDocker(t *testing.T, args ...string) string {
	t.Helper()
	out, err := docker(t, args...)
	if err != nil {
		t.Fatalf("docker %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func requireDocker(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not on PATH")
	}
	if out, err := docker(t, "info", "--format", "{{.ServerVersion}}"); err != nil {
		t.Skipf("no docker daemon: %v\n%s", err, out)
	}
}

// reachable runs the probe: can a container on netA reach one on netB by name?
// It is the single piece of logic both directions below exercise, so that the
// negative case tests the probe rather than a paraphrase of it.
func reachable(t *testing.T, prefix string, joined bool) bool {
	t.Helper()
	netA, netB := prefix+"-a", prefix+"-b"
	c1, c2 := prefix+"-1", prefix+"-2"

	t.Cleanup(func() {
		_, _ = docker(t, "rm", "-f", c1, c2)
		_, _ = docker(t, "network", "rm", netA, netB)
	})

	mustDocker(t, "network", "create", netA)
	if !joined {
		mustDocker(t, "network", "create", netB)
	}
	second := netA
	if !joined {
		second = netB
	}
	mustDocker(t, "run", "-d", "--name", c1, "--network", netA, "alpine:3", "sleep", "60")
	mustDocker(t, "run", "-d", "--name", c2, "--network", second, "alpine:3", "sleep", "60")

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
