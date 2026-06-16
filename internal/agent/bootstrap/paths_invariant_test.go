package bootstrap_test

import (
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/agent/bootstrap"
	"go.keystone-core.io/keystone-core/internal/agent/systemd"
)

// TestAgentConfigPathMatchesSystemdUnit locks the invariant that the
// path bootstrap renders the agent config to is the same path the
// generated systemd unit reads from (ExecStart … --config <path>).
//
// The two packages are intentionally decoupled (no import edge), so
// nothing at compile time forces the constants to agree — this test is
// the enforcement. They drifted once (bootstrap rendered to
// /etc/keystone-core/keystone-core-agent.yaml while the unit pointed at
// /etc/kscore/agent.yaml), which silently broke `kscore-agent bootstrap`
// followed by `kscore-agent service install`.
func TestAgentConfigPathMatchesSystemdUnit(t *testing.T) {
	if bootstrap.DefaultAgentConfigPath != systemd.DefaultConfigPath {
		t.Fatalf("agent config path drift: bootstrap renders to %q but the systemd unit reads %q — a default bootstrap+service-install would not line up",
			bootstrap.DefaultAgentConfigPath, systemd.DefaultConfigPath)
	}
}

// TestStatePathUnderAgentStateDir guards the second half of the fix:
// the bootstrap FSM state file must live under the agent's own state
// directory (the systemd unit's StateDirectory / ReadWritePaths), not
// the server's /var/lib/kscore — otherwise the hardened agent unit
// (ProtectSystem=strict) could not write it.
func TestStatePathUnderAgentStateDir(t *testing.T) {
	const agentStateDir = "/var/lib/kscore-agent/"
	if !strings.HasPrefix(bootstrap.DefaultStatePath, agentStateDir) {
		t.Fatalf("bootstrap state path %q is not under the agent state dir %q",
			bootstrap.DefaultStatePath, agentStateDir)
	}
}
