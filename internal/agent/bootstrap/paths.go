// SPDX-License-Identifier: Apache-2.0

package bootstrap

// Canonical on-disk locations for the agent, kept in one place so the
// bootstrap FSM, the TUI configurer, and the kscore-agent CLI defaults
// can never drift apart again (PROJECT-DETAILS §4.6 on-disk layout).
//
// These deliberately use the short `kscore` naming that the rest of the
// tree standardised on — the server config (/etc/kscore/server.yaml),
// the shipped systemd units (deploy/systemd/*.service), and the agent's
// own unit generator (internal/agent/systemd) all use it.
//
//   - DefaultAgentConfigPath MUST equal systemd.DefaultConfigPath: the
//     unit generator bakes `ExecStart … --config <that path>`, so the
//     path bootstrap renders to and the path the unit reads from have to
//     match. The two packages stay decoupled (no import edge); the
//     invariant is locked by paths_invariant_test.go instead.
//   - DefaultStatePath lives under the agent's own state directory
//     (/var/lib/kscore-agent — the systemd unit's StateDirectory /
//     ReadWritePaths), not the server's /var/lib/kscore.
const (
	DefaultAgentConfigPath = "/etc/kscore/agent.yaml"
	DefaultStatePath       = "/var/lib/kscore-agent/bootstrap.json"
)
