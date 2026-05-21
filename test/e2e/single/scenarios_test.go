//go:build e2e

package single

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	_ "github.com/lib/pq"

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

	// Distroless agent image ships only /usr/local/bin/kscore — use
	// its own --version so we have a real binary to exec with a
	// deterministic exit-0 path.
	stream, err := cp.ExecuteCommand(authContext(ctx, apiKey), &v1.ExecuteCommandRequest{
		AgentId:        "agent-1",
		Command:        "/usr/local/bin/kscore",
		Args:           []string{"--version"},
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

	// Smallest declaration the v1.0 schema accepts. Server-side
	// validation drives a real state.Runner against the agent;
	// docker container has /tmp writable by the nonroot UID.
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
