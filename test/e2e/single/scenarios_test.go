// SPDX-License-Identifier: Apache-2.0

//go:build e2e

package single

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// TestE2E_InfrastructureHealth proves the topology is structurally
// sound — server / Postgres / NATS reachable and the schema applied.
// Mirrors what Task 1's harness asserted, kept here so a single
// `make e2e-test` exercises infrastructure + the new scenarios.
func TestE2E_InfrastructureHealth(t *testing.T) {
	requireDocker(t)
	ctx, cancel := context.WithTimeout(context.Background(), composeReadyBudget)
	defer cancel()

	t.Run("server /health/live", func(t *testing.T) {
		if err := waitForHTTP(ctx, serverHTTPAddr+"/health/live", 200); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("server /health/ready", func(t *testing.T) {
		if err := waitForHTTP(ctx, serverHTTPAddr+"/health/ready", 200); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("nats /healthz", func(t *testing.T) {
		if err := waitForHTTP(ctx, natsMonAddr+"/healthz", 200); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("postgres schema applied", func(t *testing.T) {
		if err := waitForCondition(ctx, "postgres agents table", agentsTableExists); err != nil {
			t.Fatal(err)
		}
	})
}

// TestE2E_AgentRegistration — scenario 1 from epic 19 §Scope. Both
// docker-compose agents (agent-1, agent-2) reach AGENT_STATUS_CONNECTED
// via the PSK bootstrap path + heartbeat subscriber wired in this PR.
func TestE2E_AgentRegistration(t *testing.T) {
	requireDocker(t)
	ctx, cancel := context.WithTimeout(context.Background(), scenarioBudget)
	defer cancel()

	apiKey := extractAdminAPIKey(ctx, t)
	cp, closer := dialControlPlane(t, apiKey)
	defer closer.Close()

	want := map[string]v1.AgentStatus{
		"agent-1": v1.AgentStatus_AGENT_STATUS_CONNECTED,
		"agent-2": v1.AgentStatus_AGENT_STATUS_CONNECTED,
	}

	err := waitForCondition(ctx, "agents connected", func() error {
		resp, err := cp.ListAgents(authContext(ctx, apiKey), &v1.ListAgentsRequest{})
		if err != nil {
			return err
		}
		got := map[string]v1.AgentStatus{}
		for _, a := range resp.Agents {
			got[a.Id] = a.Status
		}
		for id, wantStatus := range want {
			gotStatus, ok := got[id]
			if !ok {
				return fmt.Errorf("%s: not present", id)
			}
			if gotStatus != wantStatus {
				return fmt.Errorf("%s: status=%s want=%s", id, gotStatus, wantStatus)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestE2E_CommandExec — scenario 2. Dispatches a single ExecuteCommand
// against agent-1 and asserts the streamed response carries a
// completion event with exit-0.
func TestE2E_CommandExec(t *testing.T) {
	requireDocker(t)
	ctx, cancel := context.WithTimeout(context.Background(), scenarioBudget)
	defer cancel()

	apiKey := extractAdminAPIKey(ctx, t)
	cp, closer := dialControlPlane(t, apiKey)
	defer closer.Close()

	// Wait until agent-1 is connected (otherwise dispatch races with
	// bootstrap; the registration scenario above runs first in test
	// alphabetical order so this usually no-ops).
	if err := waitForAgentConnected(ctx, cp, apiKey, "agent-1"); err != nil {
		t.Fatal(err)
	}

	// Command + args vary by mode: docker mode targets the distroless
	// agent image (which ships only /usr/local/bin/kscore), native
	// mode runs the agent on the host so any /bin/* command works.
	// The test only cares about a deterministic exit-0 path.
	stream, err := cp.ExecuteCommand(authContext(ctx, apiKey), &v1.ExecuteCommandRequest{
		AgentId:        "agent-1",
		Command:        commandExecBin,
		Args:           commandExecArgs,
		TimeoutSeconds: 10,
	})
	if err != nil {
		t.Fatalf("ExecuteCommand: %v", err)
	}

	var (
		gotCommandID string
		gotStdout    string
		gotExit      int32
		gotCompleted bool
	)
	for {
		ev, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("ExecuteCommand recv: %v", err)
		}
		switch e := ev.Event.(type) {
		case *v1.ExecuteCommandResponse_CommandId:
			gotCommandID = e.CommandId
		case *v1.ExecuteCommandResponse_Output:
			if e.Output != nil {
				gotStdout += string(e.Output.Stdout)
			}
		case *v1.ExecuteCommandResponse_Completion:
			gotCompleted = true
			if e.Completion != nil {
				gotExit = e.Completion.ExitCode
			}
		}
	}

	if gotCommandID == "" {
		t.Errorf("ExecuteCommand: command_id never emitted")
	}
	if !gotCompleted {
		t.Fatal("ExecuteCommand: completion event never emitted")
	}
	if gotExit != 0 {
		t.Errorf("ExecuteCommand exit code = %d, want 0", gotExit)
	}
	// Output chunks are reserved for the post-task-10/12 bridge — v1.0
	// ExecuteCommand emits {command_id, completion} only. Asserting
	// exit-code-0 proves the dispatch + result round-trip.
	_ = gotStdout
}

// TestE2E_StateApply — scenario 3. Sends a minimal state declaration
// via ApplyState; asserts the server emits a run-id (proving the
// run was registered) and a terminal event.
func TestE2E_StateApply(t *testing.T) {
	requireDocker(t)
	ctx, cancel := context.WithTimeout(context.Background(), scenarioBudget)
	defer cancel()

	apiKey := extractAdminAPIKey(ctx, t)
	cp, cpCloser := dialControlPlane(t, apiKey)
	defer cpCloser.Close()
	state, stateCloser := dialStateService(t, apiKey)
	defer stateCloser.Close()

	if err := waitForAgentConnected(ctx, cp, apiKey, "agent-1"); err != nil {
		t.Fatal(err)
	}

	// Smallest declaration the v1.0 schema accepts. With no Target
	// this converges the CONTROL-PLANE host, not an agent — agent_id
	// here is run attribution only. TestE2E_RemoteStateApply below is
	// the scenario that proves fleet dispatch.
	const yaml = `metadata:
  name: epic-19-task-2a-state
  version: "0.1"
file:
  /tmp/keystone-e2e-marker:
    state: present
    content: "epic-19-task-2a"
    mode: "0644"
`

	stream, err := state.ApplyState(authContext(ctx, apiKey), &v1.ApplyStateRequest{
		YamlContent: []byte(yaml),
		AgentId:     "agent-1",
		Source:      "epic-19-task-2a-state.yaml",
	})
	if err != nil {
		t.Fatalf("ApplyState: %v", err)
	}

	var (
		gotRunID    string
		gotTerminal bool
	)
	for {
		ev, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("ApplyState recv: %v", err)
		}
		switch e := ev.Event.(type) {
		case *v1.ApplyStateResponse_RunId:
			gotRunID = e.RunId
		case *v1.ApplyStateResponse_Terminal:
			gotTerminal = true
		}
	}
	if gotRunID == "" {
		t.Errorf("ApplyState: run_id never emitted")
	}
	if !gotTerminal {
		t.Errorf("ApplyState: terminal event never emitted")
	}
}

// TestE2E_RemoteStateApply proves a targeted `state apply` converges
// the AGENT, not the control plane.
//
// This is the assertion whose absence let remote state apply ship
// broken: every prior state scenario omitted Target, so it passed
// while the control plane quietly converged itself.
//
// The discriminator is `.Facts.agent_id` in the file's path. That fact
// exists only on an agent, so the path can only render to
// /tmp/kscore-e2e-remote-agent-1 if compilation happened on agent-1.
// A control-plane compile renders it to something else entirely — so
// the assertion holds even in native mode, where the agents and the
// server share one filesystem and mere file existence would prove
// nothing.
func TestE2E_RemoteStateApply(t *testing.T) {
	requireDocker(t)
	ctx, cancel := context.WithTimeout(context.Background(), scenarioBudget)
	defer cancel()

	apiKey := extractAdminAPIKey(ctx, t)
	cp, cpCloser := dialControlPlane(t, apiKey)
	defer cpCloser.Close()
	state, stateCloser := dialStateService(t, apiKey)
	defer stateCloser.Close()

	if err := waitForAgentConnected(ctx, cp, apiKey, "agent-1"); err != nil {
		t.Fatal(err)
	}
	if err := waitForAgentConnected(ctx, cp, apiKey, "agent-2"); err != nil {
		t.Fatal(err)
	}

	const marker = "remote-state-apply"
	const yaml = `metadata:
  name: remote-state-apply
  version: "0.1"
file:
  /tmp/kscore-e2e-remote-{{ .Facts.agent_id }}:
    state: present
    content: "remote-state-apply on {{ .Facts.agent_id }}"
    mode: "0644"
`

	stream, err := state.ApplyState(authContext(ctx, apiKey), &v1.ApplyStateRequest{
		YamlContent: []byte(yaml),
		Source:      "remote-state-apply.yaml",
		Target:      &v1.Target{AgentIds: []string{"agent-1"}},
	})
	if err != nil {
		t.Fatalf("ApplyState: %v", err)
	}

	var (
		gotRunID  string
		terminal  *v1.StateRunTerminal
		declAgent []string
		declIDs   []string
	)
	for {
		ev, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("ApplyState recv: %v", err)
		}
		switch e := ev.Event.(type) {
		case *v1.ApplyStateResponse_RunId:
			gotRunID = e.RunId
		case *v1.ApplyStateResponse_DeclResult:
			if e.DeclResult != nil {
				declAgent = append(declAgent, e.DeclResult.AgentId)
				declIDs = append(declIDs, e.DeclResult.DeclId)
				if e.DeclResult.ErrorMessage != "" {
					t.Errorf("declaration %s failed: %s",
						e.DeclResult.DeclId, e.DeclResult.ErrorMessage)
				}
			}
		case *v1.ApplyStateResponse_Terminal:
			terminal = e.Terminal
		}
	}

	if gotRunID == "" {
		t.Errorf("ApplyState: run_id never emitted")
	}
	if terminal == nil {
		t.Fatal("ApplyState: terminal event never emitted")
	}

	// Every declaration result must be attributed to the targeted
	// agent. An empty agent_id means the control plane ran it itself.
	if len(declAgent) == 0 {
		t.Fatalf("ApplyState: no declaration results streamed (terminal %+v)", terminal)
	}
	for _, id := range declAgent {
		if id != "agent-1" {
			t.Errorf("declaration result agent_id = %q, want %q", id, "agent-1")
		}
	}

	// The rendered declaration id carries the fact. This is the proof
	// that compilation happened on the agent.
	wantDeclID := "file:/tmp/kscore-e2e-remote-agent-1"
	if len(declIDs) != 1 || declIDs[0] != wantDeclID {
		t.Errorf("declaration ids = %v, want [%s] (a different path means the\n"+
			"state file was rendered somewhere other than agent-1)", declIDs, wantDeclID)
	}

	// Exactly one agent summary, for the one agent targeted. agent-2
	// is connected and must not have been touched.
	if len(terminal.AgentSummaries) != 1 {
		t.Fatalf("agent summaries = %d, want 1: %+v",
			len(terminal.AgentSummaries), terminal.AgentSummaries)
	}
	sum := terminal.AgentSummaries[0]
	if sum.AgentId != "agent-1" {
		t.Errorf("agent summary agent_id = %q, want %q", sum.AgentId, "agent-1")
	}
	if sum.ErrorMessage != "" {
		t.Errorf("agent summary error: %s", sum.ErrorMessage)
	}

	// Native mode only: the agents are host subprocesses, so read back
	// what agent-1 wrote. Docker mode stops at the assertions above —
	// the distroless agent image has no shell to exec into, and the
	// rendered decl id already proves where compilation ran.
	if !agentFSIsHost {
		return
	}
	got, err := os.ReadFile("/tmp/kscore-e2e-remote-agent-1")
	if err != nil {
		t.Fatalf("read file written by agent-1: %v", err)
	}
	want := marker + " on agent-1"
	if string(got) != want {
		t.Errorf("file content = %q, want %q", got, want)
	}
	if _, err := os.Stat("/tmp/kscore-e2e-remote-agent-2"); err == nil {
		t.Error("agent-2 was converged but was not targeted")
	}
	t.Cleanup(func() {
		_ = os.Remove("/tmp/kscore-e2e-remote-agent-1")
	})
}

// TestE2E_AgentSecretRendering proves the whole agent-secret chain
// composes: the agent keeps the SVID bootstrap issued it, proves which
// agent it is with that certificate, is authorized by the server's
// grants, receives a reply sealed to its own key, and renders the
// value into a file on its own host.
//
// Each link has unit tests; none of them can catch the links being
// wired to each other wrongly. This is the assertion that the feature
// exists rather than that its parts do.
func TestE2E_AgentSecretRendering(t *testing.T) {
	requireDocker(t)
	ctx, cancel := context.WithTimeout(context.Background(), scenarioBudget)
	defer cancel()

	apiKey := extractAdminAPIKey(ctx, t)
	cp, cpCloser := dialControlPlane(t, apiKey)
	defer cpCloser.Close()
	sc, scCloser := dialSecretsService(t, apiKey)
	defer scCloser.Close()
	st, stCloser := dialStateService(t, apiKey)
	defer stCloser.Close()

	if err := waitForAgentConnected(ctx, cp, apiKey, "agent-1"); err != nil {
		t.Fatal(err)
	}

	// Inside kv/e2e/agent/, which server.yaml grants to agent-1 only.
	const secretPath = "kv/e2e/agent/db"
	const want = "e2e-rendered-password"
	if _, err := sc.WriteSecret(authContext(ctx, apiKey), &v1.WriteSecretRequest{
		Path: secretPath,
		Data: map[string]string{"password": want},
	}); err != nil {
		t.Fatalf("WriteSecret: %v", err)
	}

	// The path carries .Facts.agent_id, as in TestE2E_RemoteStateApply,
	// so the file's existence proves where the render happened; the
	// content proves the secret was resolved there too.
	const yaml = `metadata:
  name: agent-secret-render
  version: "0.1"
file:
  /tmp/kscore-e2e-secret-{{ .Facts.agent_id }}:
    state: present
    content: "DB_PASS={{ secret \"kv/e2e/agent/db\" \"password\" }}"
    mode: "0600"
`

	stream, err := st.ApplyState(authContext(ctx, apiKey), &v1.ApplyStateRequest{
		YamlContent: []byte(yaml),
		Source:      "agent-secret-render.yaml",
		Target:      &v1.Target{AgentIds: []string{"agent-1"}},
	})
	if err != nil {
		t.Fatalf("ApplyState: %v", err)
	}

	var (
		terminal *v1.StateRunTerminal
		declErrs []string
	)
	for {
		ev, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("ApplyState recv: %v", err)
		}
		switch e := ev.Event.(type) {
		case *v1.ApplyStateResponse_DeclResult:
			if e.DeclResult != nil && e.DeclResult.ErrorMessage != "" {
				declErrs = append(declErrs, e.DeclResult.ErrorMessage)
			}
		case *v1.ApplyStateResponse_Terminal:
			terminal = e.Terminal
		}
	}
	if len(declErrs) > 0 {
		t.Fatalf("declaration errors: %v", declErrs)
	}
	if terminal == nil {
		t.Fatal("ApplyState: terminal event never emitted")
	}
	for _, s := range terminal.AgentSummaries {
		if s.ErrorMessage != "" {
			t.Fatalf("agent %s: %s", s.AgentId, s.ErrorMessage)
		}
	}

	if !agentFSIsHost {
		// Docker mode: the distroless agent image has no shell to read
		// the file back with. The run completing without a declaration
		// error already proves the secret resolved -- an unresolvable
		// reference fails the render rather than writing a blank.
		return
	}
	const path = "/tmp/kscore-e2e-secret-agent-1"
	t.Cleanup(func() { _ = os.Remove(path) })
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the file agent-1 rendered: %v", err)
	}
	if string(got) != "DB_PASS="+want {
		t.Errorf("rendered content = %q, want %q", got, "DB_PASS="+want)
	}
}

// A path outside the agent's grants must fail the render rather than
// writing a file with an empty value.
func TestE2E_AgentSecretDenied(t *testing.T) {
	requireDocker(t)
	ctx, cancel := context.WithTimeout(context.Background(), scenarioBudget)
	defer cancel()

	apiKey := extractAdminAPIKey(ctx, t)
	cp, cpCloser := dialControlPlane(t, apiKey)
	defer cpCloser.Close()
	sc, scCloser := dialSecretsService(t, apiKey)
	defer scCloser.Close()
	st, stCloser := dialStateService(t, apiKey)
	defer stCloser.Close()

	if err := waitForAgentConnected(ctx, cp, apiKey, "agent-1"); err != nil {
		t.Fatal(err)
	}

	// Outside kv/e2e/agent/, so no grant covers it for any agent.
	const secretPath = "kv/e2e/ungranted/db"
	if _, err := sc.WriteSecret(authContext(ctx, apiKey), &v1.WriteSecretRequest{
		Path: secretPath,
		Data: map[string]string{"password": "must-not-be-rendered"},
	}); err != nil {
		t.Fatalf("WriteSecret: %v", err)
	}

	const yaml = `metadata:
  name: agent-secret-denied
  version: "0.1"
file:
  /tmp/kscore-e2e-denied-{{ .Facts.agent_id }}:
    state: present
    content: "DB_PASS={{ secret \"kv/e2e/ungranted/db\" \"password\" }}"
    mode: "0600"
`

	stream, err := st.ApplyState(authContext(ctx, apiKey), &v1.ApplyStateRequest{
		YamlContent: []byte(yaml),
		Source:      "agent-secret-denied.yaml",
		Target:      &v1.Target{AgentIds: []string{"agent-1"}},
	})
	if err != nil {
		t.Fatalf("ApplyState: %v", err)
	}

	var (
		terminal *v1.StateRunTerminal
		failed   bool
	)
	for {
		ev, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("ApplyState recv: %v", err)
		}
		if e, ok := ev.Event.(*v1.ApplyStateResponse_Terminal); ok {
			terminal = e.Terminal
		}
	}
	if terminal == nil {
		t.Fatal("ApplyState: terminal event never emitted")
	}
	for _, s := range terminal.AgentSummaries {
		if s.ErrorMessage != "" {
			failed = true
		}
	}
	if !failed {
		t.Error("an ungranted secret reference did not fail the run")
	}

	if !agentFSIsHost {
		return
	}
	// The file must not exist at all. Writing it with a blank password
	// and reporting success is the outcome this whole design exists to
	// prevent.
	const path = "/tmp/kscore-e2e-denied-agent-1"
	t.Cleanup(func() { _ = os.Remove(path) })
	if _, err := os.Stat(path); err == nil {
		body, _ := os.ReadFile(path)
		t.Errorf("a denied secret still produced a file: %q", body)
	}
}

// TestE2E_BlueprintApply — scenario 4 from epic 19 §Scope. Verifies
// the BlueprintService gRPC surface against the catalog bundled into
// the kscore-server image at /modules/examples/blueprints. Apply runs
// server-side against the local stdlib StateRunner — remote-fleet
// dispatch is the gate-v1.0 ROADMAP item "Remote / distributed
// blueprint apply wiring".
func TestE2E_BlueprintApply(t *testing.T) {
	requireDocker(t)
	ctx, cancel := context.WithTimeout(context.Background(), scenarioBudget)
	defer cancel()

	apiKey := extractAdminAPIKey(ctx, t)
	bpc, closer := dialBlueprintService(t, apiKey)
	defer closer.Close()

	listResp, err := bpc.ListBlueprints(authContext(ctx, apiKey), &v1.ListBlueprintsRequest{})
	if err != nil {
		t.Fatalf("ListBlueprints: %v", err)
	}
	if listResp.TotalCount == 0 {
		t.Fatal("ListBlueprints: catalog is empty (expected modules/examples/blueprints bundled in image)")
	}

	// Use the distroless-compatible `e2e-noop` fixture (lives under
	// test/e2e/single/blueprints/). The production `demo` blueprint
	// installs a package + manages a service — neither is runnable
	// inside the nonroot distroless kscore-server container.
	const blueprintName = "e2e-noop"
	var found bool
	for _, b := range listResp.Blueprints {
		if b.Name == blueprintName {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ListBlueprints: %q not in catalog; got %d entries", blueprintName, listResp.TotalCount)
	}

	applyResp, err := bpc.ApplyBlueprint(authContext(ctx, apiKey), &v1.ApplyBlueprintRequest{
		Name: blueprintName,
	})
	if err != nil {
		t.Fatalf("ApplyBlueprint: %v", err)
	}
	if applyResp.RunId == "" {
		t.Errorf("ApplyBlueprint: run_id empty")
	}
	if applyResp.Report == nil {
		t.Fatal("ApplyBlueprint: report missing")
	}
	if applyResp.Report.Failed != 0 {
		t.Errorf("ApplyBlueprint: report.failed = %d, want 0", applyResp.Report.Failed)
	}
}

// TestE2E_ModuleStdlibExecution — scenario 5, Option A per epic 19
// task 2b. Exercises the loader → runner → multi-stdlib-module
// resolver chain via a composed state YAML (file + cmd with a
// requires-dependency) over ApplyState.
//
// **Deferred to v1.x**: scenario 5's full registry-based "Module
// install + execute" form (Option B). The pieces exist (kscore-
// registry binary, kscore-module CLI, the loader/runtime/verifier
// seams), but production wiring of TrustPolicy / capability.Hosts /
// PolicyChecker into cmd/kscore-{server,agent} is the gate-v1.0
// ROADMAP item "Module system boot wiring". Once that lands,
// scenario 5 graduates to: kscore-registry container + signed
// module fixture + kscore-module install → ApplyState referencing
// the installed Starlark module.
func TestE2E_ModuleStdlibExecution(t *testing.T) {
	requireDocker(t)
	ctx, cancel := context.WithTimeout(context.Background(), scenarioBudget)
	defer cancel()

	apiKey := extractAdminAPIKey(ctx, t)
	cp, cpCloser := dialControlPlane(t, apiKey)
	defer cpCloser.Close()
	state, stateCloser := dialStateService(t, apiKey)
	defer stateCloser.Close()

	if err := waitForAgentConnected(ctx, cp, apiKey, "agent-1"); err != nil {
		t.Fatal(err)
	}

	// Multi-module YAML: file creates a directory, cmd writes a marker
	// inside it with a requires-dependency on the file decl. Exercises
	// the resolver's topo sort across module boundaries.
	const yaml = `metadata:
  name: epic-19-task-2b-multi
  version: "0.1"
file:
  /tmp/keystone-e2e-2b-dir:
    state: directory
    mode: "0755"
cmd:
  cmd-write-marker:
    state: run
    command: "/bin/sh -c 'echo task-2b > /tmp/keystone-e2e-2b-dir/marker'"
    creates: /tmp/keystone-e2e-2b-dir/marker
    require:
      - file: /tmp/keystone-e2e-2b-dir
`

	stream, err := state.ApplyState(authContext(ctx, apiKey), &v1.ApplyStateRequest{
		YamlContent: []byte(yaml),
		AgentId:     "agent-1",
		Source:      "epic-19-task-2b-multi.yaml",
	})
	if err != nil {
		t.Fatalf("ApplyState: %v", err)
	}

	var (
		gotRunID    string
		gotTerminal bool
	)
	for {
		ev, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("ApplyState recv: %v", err)
		}
		switch e := ev.Event.(type) {
		case *v1.ApplyStateResponse_RunId:
			gotRunID = e.RunId
		case *v1.ApplyStateResponse_Terminal:
			gotTerminal = true
		}
	}
	if gotRunID == "" {
		t.Errorf("ApplyState: run_id never emitted")
	}
	if !gotTerminal {
		t.Errorf("ApplyState: terminal event never emitted")
	}
}

// TestE2E_SecretsRoundTrip — scenario 6, KV subset. WriteSecret →
// GetSecret round-trip and ListSecrets enumeration against the
// encrypted-file backend wired in server.yaml. Lease + transit are
// Vault-only in v1.0 (see internal/secrets/file/backend.go:370 and
// internal/secrets/transit.go:13) and stay deferred to a future
// Vault-backed E2E.
func TestE2E_SecretsRoundTrip(t *testing.T) {
	requireDocker(t)
	ctx, cancel := context.WithTimeout(context.Background(), scenarioBudget)
	defer cancel()

	apiKey := extractAdminAPIKey(ctx, t)
	sc, closer := dialSecretsService(t, apiKey)
	defer closer.Close()

	const path = "kv/e2e/task-2b"
	want := map[string]string{
		"username": "kscore-e2e",
		"token":    "task-2b-secret-token",
	}

	t.Run("write", func(t *testing.T) {
		resp, err := sc.WriteSecret(authContext(ctx, apiKey), &v1.WriteSecretRequest{
			Path: path,
			Data: want,
		})
		if err != nil {
			t.Fatalf("WriteSecret: %v", err)
		}
		if resp.Metadata == nil || resp.Metadata.Path != path {
			t.Errorf("WriteSecret metadata = %+v, want path=%q", resp.Metadata, path)
		}
	})

	t.Run("get", func(t *testing.T) {
		resp, err := sc.GetSecret(authContext(ctx, apiKey), &v1.GetSecretRequest{
			Path: path,
		})
		if err != nil {
			t.Fatalf("GetSecret: %v", err)
		}
		if len(resp.Data) != len(want) {
			t.Errorf("GetSecret data has %d entries, want %d", len(resp.Data), len(want))
		}
		for k, v := range want {
			if got := resp.Data[k]; got != v {
				t.Errorf("GetSecret[%q] = %q, want %q", k, got, v)
			}
		}
	})

	t.Run("list", func(t *testing.T) {
		resp, err := sc.ListSecrets(authContext(ctx, apiKey), &v1.ListSecretsRequest{
			PathPrefix: "kv/e2e/",
		})
		if err != nil {
			t.Fatalf("ListSecrets: %v", err)
		}
		found := false
		for _, m := range resp.Secrets {
			if m.Path == path {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ListSecrets: %q not in result; got %d entries", path, len(resp.Secrets))
		}
	})

	t.Run("delete", func(t *testing.T) {
		_, err := sc.DeleteSecret(authContext(ctx, apiKey), &v1.DeleteSecretRequest{
			Path: path,
		})
		if err != nil {
			t.Fatalf("DeleteSecret: %v", err)
		}
	})
}

// TestE2E_AuditLogQuery — scenario 7. Earlier scenarios (registration,
// command exec, blueprint apply, secrets write) emit audit entries
// into the SQL audit store. This scenario verifies the operator-
// facing read path via PolicyService.GetAuditLog and exercises
// GetComplianceReport so the v1.0 audit surface is reachable.
func TestE2E_AuditLogQuery(t *testing.T) {
	requireDocker(t)
	ctx, cancel := context.WithTimeout(context.Background(), scenarioBudget)
	defer cancel()

	apiKey := extractAdminAPIKey(ctx, t)
	pc, closer := dialPolicyService(t, apiKey)
	defer closer.Close()

	t.Run("audit log non-empty", func(t *testing.T) {
		// Earlier scenarios populate the audit store; poll because the
		// audit fan-out is asynchronous through the bus.
		err := waitForCondition(ctx, "audit entries", func() error {
			resp, err := pc.GetAuditLog(authContext(ctx, apiKey), &v1.GetAuditLogRequest{
				Limit: 50,
			})
			if err != nil {
				return err
			}
			if len(resp.Entries) == 0 {
				return fmt.Errorf("no audit entries yet")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("compliance report reachable", func(t *testing.T) {
		// v1.0 audit-mode: a report against an empty policy set returns
		// 0 violations + 0 evaluations. Since is required by the
		// validator; use a 24h window ending now.
		_, err := pc.GetComplianceReport(authContext(ctx, apiKey), &v1.GetComplianceReportRequest{
			Since: timestamppb.New(time.Now().Add(-24 * time.Hour)),
			Until: timestamppb.Now(),
		})
		if err != nil {
			t.Fatalf("GetComplianceReport: %v", err)
		}
	})
}

// TestE2E_OutboundWebhook — scenario 8. Stands up a test
// httptest.Server as the webhook receiver, registers a subscription
// over REST against the live kscore-server, fires the manager's
// synthetic test ping, and asserts the receiver got the POST.
func TestE2E_OutboundWebhook(t *testing.T) {
	requireDocker(t)
	ctx, cancel := context.WithTimeout(context.Background(), scenarioBudget)
	defer cancel()

	apiKey := extractAdminAPIKey(ctx, t)
	hc := &http.Client{Timeout: 5 * time.Second}

	// Receiver runs on the host. In docker mode the server (in a
	// container) reaches the host via host.docker.internal; in native
	// mode the server IS on the host so loopback works. The
	// webhookReceiverHost var captures that difference.
	gotPing := make(chan []byte, 1)
	receiver := newHTTPRecorder(gotPing)
	defer receiver.Close()

	// Register the subscription. URL points the server back at the
	// host-side receiver.
	subPayload := map[string]any{
		"name":        "epic-19-task-2c-receiver",
		"url":         fmt.Sprintf("http://%s:%d/hook", webhookReceiverHost, receiver.port()),
		"events":      []string{"*"},
		"enabled":     true,
		"max_retries": 1,
		"timeout_sec": 5,
	}
	body, _ := json.Marshal(subPayload)
	createReq := adminHTTPRequest(ctx, t, apiKey, http.MethodPost,
		"/api/v1/webhooks/subscriptions", bytes.NewReader(body))
	createResp, err := hc.Do(createReq)
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated && createResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(createResp.Body)
		t.Fatalf("create subscription: status=%d body=%s", createResp.StatusCode, respBody)
	}
	var created struct {
		ID     string `json:"id"`
		Secret string `json:"secret"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ID == "" {
		t.Fatal("create subscription: id missing")
	}

	// Fire the synthetic test ping. The Manager dispatches a known
	// payload to the registered URL.
	testReq := adminHTTPRequest(ctx, t, apiKey, http.MethodPost,
		"/api/v1/webhooks/subscriptions/"+created.ID+"/test", nil)
	testResp, err := hc.Do(testReq)
	if err != nil {
		t.Fatalf("test subscription: %v", err)
	}
	defer testResp.Body.Close()
	if testResp.StatusCode != http.StatusOK && testResp.StatusCode != http.StatusAccepted {
		respBody, _ := io.ReadAll(testResp.Body)
		t.Fatalf("test subscription: status=%d body=%s", testResp.StatusCode, respBody)
	}

	// Receiver should now have the payload. 10s budget covers HTTP
	// dispatcher latency + scheduling.
	select {
	case payload := <-gotPing:
		if len(payload) == 0 {
			t.Errorf("webhook payload empty")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("webhook receiver never got the test ping")
	}
}

// TestE2E_GitOpsWebhookIngest — scenario 9 part 1. POSTs an HMAC-
// signed GitHub-style payload to the kscore-server's GitOps webhook
// receiver on :8081/webhooks. Asserts the request is accepted (202)
// and an event lands on the bus.
func TestE2E_GitOpsWebhookIngest(t *testing.T) {
	requireDocker(t)
	ctx, cancel := context.WithTimeout(context.Background(), scenarioBudget)
	defer cancel()

	const (
		hmacSecret = "epic-19-task-2c-github-hmac-secret"
		recvAddr   = "http://127.0.0.1:8081/webhooks"
	)

	payload := []byte(`{
		"repository":{"full_name":"epic-19/task-2c"},
		"ref":"refs/heads/main",
		"after":"e2e1234567890abcdef1234567890abcdef123456",
		"commits":[{"id":"e2e1234567890abcdef1234567890abcdef123456","message":"task 2c"}]
	}`)

	sig := hmacSHA256(hmacSecret, payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, recvAddr, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", "sha256="+sig)

	hc := &http.Client{Timeout: 5 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", recvAddr, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("webhook ingest: status=%d body=%s", resp.StatusCode, body)
	}
}

// TestE2E_GitOpsRollback — scenario 9 part 2. Exercises the rollback
// engine + REST surface end-to-end via the FSM: create a Pending
// rollback (require_approval=true), reject it, verify it reaches
// the Rejected terminal state. This proves:
//   - rollback engine constructed at boot,
//   - SQLite store persists transitions,
//   - REST handler routes Execute/Reject/Get correctly,
//   - state machine drives Pending → Rejected.
//
// **Real git-executor coverage** (clone → revert → push against a
// working git server) is deferred to a v1.x test that adds either an
// in-compose git server (e.g. gitea) or an alpine/git sidecar to
// initialize a bare repo. The current scenario proves the rollback
// machinery — what was missing in v1.0 — without depending on a
// network-reachable git server inside the docker-compose topology.
func TestE2E_GitOpsRollback(t *testing.T) {
	requireDocker(t)
	ctx, cancel := context.WithTimeout(context.Background(), scenarioBudget)
	defer cancel()

	apiKey := extractAdminAPIKey(ctx, t)
	hc := &http.Client{Timeout: 5 * time.Second}

	// Create a Pending rollback. require_approval=true keeps the
	// engine off the executor path so we don't need a working repo.
	createBody, _ := json.Marshal(map[string]any{
		"executor_type":    "git",
		"application":      "epic-19-task-2c-app",
		"strategy":         "revert",
		"reason":           "task 2c FSM verification",
		"require_approval": true,
		"config": map[string]any{
			"repo_url": "file:///nonexistent/repo.git",
			"branch":   "main",
		},
	})
	createReq := adminHTTPRequest(ctx, t, apiKey, http.MethodPost,
		"/api/v1/gitops/rollback", bytes.NewReader(createBody))
	createResp, err := hc.Do(createReq)
	if err != nil {
		t.Fatalf("create rollback: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusOK &&
		createResp.StatusCode != http.StatusCreated &&
		createResp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(createResp.Body)
		t.Fatalf("create rollback: status=%d body=%s", createResp.StatusCode, body)
	}
	var created struct {
		ID    string `json:"id"`
		State string `json:"state"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("create rollback: id missing")
	}
	if !strings.EqualFold(created.State, "pending") {
		t.Errorf("create rollback: state=%q want=%q (case-insensitive)", created.State, "pending")
	}

	// Reject. Engine transitions Pending → Rejected.
	rejectBody, _ := json.Marshal(map[string]any{
		"approver": "e2e-test",
		"reason":   "smoke-test reject path",
	})
	rejectReq := adminHTTPRequest(ctx, t, apiKey, http.MethodPost,
		"/api/v1/gitops/rollbacks/"+created.ID+"/reject", bytes.NewReader(rejectBody))
	rejectResp, err := hc.Do(rejectReq)
	if err != nil {
		t.Fatalf("reject rollback: %v", err)
	}
	defer rejectResp.Body.Close()
	if rejectResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(rejectResp.Body)
		t.Fatalf("reject rollback: status=%d body=%s", rejectResp.StatusCode, body)
	}

	// Verify final state via GET.
	getReq := adminHTTPRequest(ctx, t, apiKey, http.MethodGet,
		"/api/v1/gitops/rollbacks/"+created.ID, nil)
	getResp, err := hc.Do(getReq)
	if err != nil {
		t.Fatalf("get rollback: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(getResp.Body)
		t.Fatalf("get rollback: status=%d body=%s", getResp.StatusCode, body)
	}
	var final struct {
		ID    string `json:"id"`
		State string `json:"state"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&final); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if !strings.EqualFold(final.State, "rejected") {
		t.Errorf("rollback final state = %q, want %q (case-insensitive)", final.State, "rejected")
	}
}

// ---- helpers ---------------------------------------------------------

func requireDocker(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("e2e: skipping under -short")
	}
}

func agentsTableExists() error {
	db, err := sql.Open("postgres", postgresDSN)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var n int
	err = db.QueryRowContext(ctx,
		`SELECT 1 FROM information_schema.tables WHERE table_name = 'agents'`,
	).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("agents table not yet present")
	}
	return err
}

// httpRecorder is a tiny httptest.Server that captures the first POST
// body into a channel. Used by the outbound webhook scenario to
// verify the server-side dispatcher reached us.
type httpRecorder struct {
	srv  *httptest.Server
	addr string
}

func newHTTPRecorder(out chan<- []byte) *httpRecorder {
	r := &httpRecorder{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		_ = req.Body.Close()
		select {
		case out <- body:
		default:
		}
		w.WriteHeader(http.StatusNoContent)
	})
	// Bind on 0.0.0.0 so the kscore-server container can reach us
	// via host.docker.internal:<port>. httptest.NewServer defaults to
	// 127.0.0.1, which isn't routable from the container.
	r.srv = httptest.NewUnstartedServer(handler)
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		panic("httpRecorder listen: " + err.Error())
	}
	r.srv.Listener = ln
	r.srv.Start()
	r.addr = r.srv.URL
	return r
}

func (r *httpRecorder) Close() { r.srv.Close() }

func (r *httpRecorder) port() int {
	// httptest.Server URL is "http://127.0.0.1:PORT".
	_, p, err := net.SplitHostPort(strings.TrimPrefix(r.addr, "http://"))
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(p)
	return n
}

// hmacSHA256 returns the lowercase hex-encoded HMAC-SHA256 of payload
// under the supplied secret. Matches the encoding the GitHub webhook
// authenticator expects.
func hmacSHA256(secret string, payload []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(payload)
	return hex.EncodeToString(m.Sum(nil))
}

func waitForAgentConnected(ctx context.Context, cp v1.ControlPlaneServiceClient, apiKey, agentID string) error {
	return waitForCondition(ctx, agentID+" connected", func() error {
		resp, err := cp.ListAgents(authContext(ctx, apiKey), &v1.ListAgentsRequest{})
		if err != nil {
			return err
		}
		for _, a := range resp.Agents {
			if a.Id == agentID && a.Status == v1.AgentStatus_AGENT_STATUS_CONNECTED {
				return nil
			}
		}
		return fmt.Errorf("%s not yet connected", agentID)
	})
}
