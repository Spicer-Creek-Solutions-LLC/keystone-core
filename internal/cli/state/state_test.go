package state

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// ---- fake client + stream -------------------------------------------

type fakeClient struct {
	v1.StateServiceClient
	applyStream  *fakeApplyStream
	applyErr     error
	checkResp    *v1.CheckStateResponse
	checkErr     error
	driftResp    *v1.DetectDriftResponse
	driftErr     error
	applyReqs    []*v1.ApplyStateRequest
	checkReqs    []*v1.CheckStateRequest
	driftReqs    []*v1.DetectDriftRequest
}

func (c *fakeClient) ApplyState(_ context.Context, req *v1.ApplyStateRequest, _ ...grpc.CallOption) (v1.StateService_ApplyStateClient, error) {
	c.applyReqs = append(c.applyReqs, req)
	if c.applyErr != nil {
		return nil, c.applyErr
	}
	return c.applyStream, nil
}

func (c *fakeClient) CheckState(_ context.Context, req *v1.CheckStateRequest, _ ...grpc.CallOption) (*v1.CheckStateResponse, error) {
	c.checkReqs = append(c.checkReqs, req)
	return c.checkResp, c.checkErr
}

func (c *fakeClient) DetectDrift(_ context.Context, req *v1.DetectDriftRequest, _ ...grpc.CallOption) (*v1.DetectDriftResponse, error) {
	c.driftReqs = append(c.driftReqs, req)
	return c.driftResp, c.driftErr
}

// fakeApplyStream serves a fixed slice of events then io.EOF.
type fakeApplyStream struct {
	grpc.ClientStream
	events []*v1.ApplyStateResponse
	idx    int
}

func (s *fakeApplyStream) Recv() (*v1.ApplyStateResponse, error) {
	if s.idx >= len(s.events) {
		return nil, io.EOF
	}
	e := s.events[s.idx]
	s.idx++
	return e, nil
}

// dialFor returns a Deps backed by client.
func dialFor(client v1.StateServiceClient) Deps {
	return Deps{
		Dial: func(context.Context, string, string) (v1.StateServiceClient, io.Closer, error) {
			return client, io.NopCloser(nil), nil
		},
	}
}

// writeYAML writes a small file under t.TempDir() and returns its
// path so subcommand tests can pass it as the positional argument.
func writeYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write yaml: %v", err)
	}
	return path
}

// runCmd executes a subcommand with the args and returns
// (stdout, err). stderr is captured but unused — cobra writes
// errors via the returned error anyway.
func runCmd(t *testing.T, root *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stdout)
	root.SetArgs(args)
	err := root.ExecuteContext(t.Context())
	return stdout.String(), err
}

// ---- apply --------------------------------------------------------

func TestApply_StreamsAllEvents(t *testing.T) {
	t.Parallel()
	stream := &fakeApplyStream{events: []*v1.ApplyStateResponse{
		{Event: &v1.ApplyStateResponse_RunId{RunId: "r-1"}},
		{Event: &v1.ApplyStateResponse_DeclResult{DeclResult: &v1.StateDeclarationResult{
			DeclId: "file:/etc/hosts", Outcome: v1.StateRunOutcome_STATE_RUN_OUTCOME_CHANGED,
		}}},
		{Event: &v1.ApplyStateResponse_Terminal{Terminal: &v1.StateRunTerminal{
			RunId: "r-1", Status: v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED,
			Aggregates: &v1.StateRunAggregates{Total: 1, Changed: 1},
		}}},
	}}
	client := &fakeClient{applyStream: stream}
	root := NewCommand(dialFor(client))

	yaml := writeYAML(t, "file:\n  /etc/hosts:\n    state: present\n")
	out, err := runCmd(t, root, "apply", yaml, "--agent", "web-1")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !strings.Contains(out, "run-id: r-1") {
		t.Errorf("missing run-id; got:\n%s", out)
	}
	if !strings.Contains(out, "[change]") {
		t.Errorf("missing change badge; got:\n%s", out)
	}
	if !strings.Contains(out, "status=completed") {
		t.Errorf("missing terminal; got:\n%s", out)
	}
	if len(client.applyReqs) != 1 {
		t.Fatalf("ApplyState called %d times, want 1", len(client.applyReqs))
	}
	if client.applyReqs[0].AgentId != "web-1" {
		t.Errorf("AgentId = %q, want web-1", client.applyReqs[0].AgentId)
	}
	if client.applyReqs[0].DryRun {
		t.Error("DryRun should be false without --dry-run")
	}
	if client.applyReqs[0].Source != "state.yaml" {
		t.Errorf("Source = %q, want state.yaml (basename default)", client.applyReqs[0].Source)
	}
}

func TestApply_FailingTerminalReturnsError(t *testing.T) {
	t.Parallel()
	stream := &fakeApplyStream{events: []*v1.ApplyStateResponse{
		{Event: &v1.ApplyStateResponse_RunId{RunId: "r-1"}},
		{Event: &v1.ApplyStateResponse_Terminal{Terminal: &v1.StateRunTerminal{
			RunId: "r-1", Status: v1.StateRunStatus_STATE_RUN_STATUS_FAILED,
			Aggregates: &v1.StateRunAggregates{},
		}}},
	}}
	client := &fakeClient{applyStream: stream}
	root := NewCommand(dialFor(client))
	yaml := writeYAML(t, "file:\n  /a:\n    state: present\n")
	_, err := runCmd(t, root, "apply", yaml)
	if err == nil || !strings.Contains(err.Error(), "failed") {
		t.Errorf("want failure error, got %v", err)
	}
}

func TestApply_DryRunFlagWired(t *testing.T) {
	t.Parallel()
	stream := &fakeApplyStream{events: []*v1.ApplyStateResponse{
		{Event: &v1.ApplyStateResponse_Terminal{Terminal: &v1.StateRunTerminal{
			Status: v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED, Aggregates: &v1.StateRunAggregates{},
		}}},
	}}
	client := &fakeClient{applyStream: stream}
	root := NewCommand(dialFor(client))
	yaml := writeYAML(t, "file:\n  /a:\n    state: present\n")
	if _, err := runCmd(t, root, "apply", yaml, "--dry-run"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !client.applyReqs[0].DryRun {
		t.Error("DryRun should be true with --dry-run")
	}
}

func TestApply_VariablesAndFactsWired(t *testing.T) {
	t.Parallel()
	stream := &fakeApplyStream{events: []*v1.ApplyStateResponse{
		{Event: &v1.ApplyStateResponse_Terminal{Terminal: &v1.StateRunTerminal{
			Status: v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED, Aggregates: &v1.StateRunAggregates{},
		}}},
	}}
	client := &fakeClient{applyStream: stream}
	root := NewCommand(dialFor(client))
	yaml := writeYAML(t, "file:\n  /a:\n    state: present\n")
	_, err := runCmd(t, root, "apply", yaml,
		"--variable", "port=8080",
		"--variable", "user=www",
		"--fact", "os=linux",
	)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	req := client.applyReqs[0]
	if req.VariableOverrides["port"] != "8080" || req.VariableOverrides["user"] != "www" {
		t.Errorf("VariableOverrides lost: %+v", req.VariableOverrides)
	}
	if req.Facts["os"] != "linux" {
		t.Errorf("Facts lost: %+v", req.Facts)
	}
}

func TestApply_StreamErrorBubblesUp(t *testing.T) {
	t.Parallel()
	client := &fakeClient{applyErr: errors.New("server unavailable")}
	root := NewCommand(dialFor(client))
	yaml := writeYAML(t, "file:\n  /a:\n    state: present\n")
	_, err := runCmd(t, root, "apply", yaml)
	if err == nil || !strings.Contains(err.Error(), "server unavailable") {
		t.Errorf("want underlying error surfaced, got %v", err)
	}
}

// ---- check --------------------------------------------------------

func TestCheck_RendersDeclarations(t *testing.T) {
	t.Parallel()
	client := &fakeClient{checkResp: &v1.CheckStateResponse{
		RunId:  "r-2",
		Status: v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED,
		Aggregates: &v1.StateRunAggregates{Total: 2, Drifted: 1, Unchanged: 1},
		Declarations: []*v1.StateDeclarationResult{
			{DeclId: "file:/a", Outcome: v1.StateRunOutcome_STATE_RUN_OUTCOME_DRIFT_DETECTED},
			{DeclId: "file:/b", Outcome: v1.StateRunOutcome_STATE_RUN_OUTCOME_UNCHANGED},
		},
	}}
	root := NewCommand(dialFor(client))
	yaml := writeYAML(t, "file:\n  /a:\n    state: present\n")
	out, err := runCmd(t, root, "check", yaml)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !strings.Contains(out, "file:/a") || !strings.Contains(out, "file:/b") {
		t.Errorf("missing decl rows; got:\n%s", out)
	}
	if !strings.Contains(out, "drifted=1") {
		t.Errorf("missing summary; got:\n%s", out)
	}
}

// ---- drift --------------------------------------------------------

func TestDrift_PrintsAggregateSeverity(t *testing.T) {
	t.Parallel()
	client := &fakeClient{driftResp: &v1.DetectDriftResponse{
		RunId:             "r-3",
		AggregateSeverity: v1.DriftSeverity_DRIFT_SEVERITY_HIGH,
		Aggregates:        &v1.StateRunAggregates{Drifted: 1},
		Statuses: []*v1.DriftDeclaration{
			{DeclId: "file:/x", Severity: v1.DriftSeverity_DRIFT_SEVERITY_HIGH, State: v1.DriftState_DRIFT_STATE_DRIFTED},
		},
	}}
	root := NewCommand(dialFor(client))
	yaml := writeYAML(t, "file:\n  /x:\n    state: present\n")
	out, err := runCmd(t, root, "drift", yaml)
	if err != nil {
		t.Fatalf("drift: %v", err)
	}
	if !strings.Contains(out, "aggregate severity: high") {
		t.Errorf("missing aggregate; got:\n%s", out)
	}
}

func TestDrift_FixReappliesWhenDrifted(t *testing.T) {
	t.Parallel()
	stream := &fakeApplyStream{events: []*v1.ApplyStateResponse{
		{Event: &v1.ApplyStateResponse_Terminal{Terminal: &v1.StateRunTerminal{
			Status: v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED, Aggregates: &v1.StateRunAggregates{},
		}}},
	}}
	client := &fakeClient{
		driftResp: &v1.DetectDriftResponse{
			AggregateSeverity: v1.DriftSeverity_DRIFT_SEVERITY_HIGH,
			Aggregates:        &v1.StateRunAggregates{Drifted: 1},
			Statuses: []*v1.DriftDeclaration{
				{DeclId: "file:/x", State: v1.DriftState_DRIFT_STATE_DRIFTED, Severity: v1.DriftSeverity_DRIFT_SEVERITY_HIGH},
			},
		},
		applyStream: stream,
	}
	root := NewCommand(dialFor(client))
	yaml := writeYAML(t, "file:\n  /x:\n    state: present\n")
	out, err := runCmd(t, root, "drift", yaml, "--fix")
	if err != nil {
		t.Fatalf("drift --fix: %v", err)
	}
	if !strings.Contains(out, "--- fix ---") {
		t.Errorf("missing fix divider; got:\n%s", out)
	}
	if len(client.applyReqs) != 1 {
		t.Errorf("ApplyState called %d times, want 1 (after detection)", len(client.applyReqs))
	}
}

func TestDrift_FixSkipsWhenNoDrift(t *testing.T) {
	t.Parallel()
	client := &fakeClient{driftResp: &v1.DetectDriftResponse{
		AggregateSeverity: v1.DriftSeverity_DRIFT_SEVERITY_NONE,
		Aggregates:        &v1.StateRunAggregates{},
	}}
	root := NewCommand(dialFor(client))
	yaml := writeYAML(t, "file:\n  /x:\n    state: present\n")
	out, err := runCmd(t, root, "drift", yaml, "--fix")
	if err != nil {
		t.Fatalf("drift --fix: %v", err)
	}
	if !strings.Contains(out, "no drift to fix") {
		t.Errorf("missing no-drift hint; got:\n%s", out)
	}
	if len(client.applyReqs) != 0 {
		t.Errorf("ApplyState should NOT be called when no drift; got %d", len(client.applyReqs))
	}
}

// ---- compile + vars (local, no client) ----------------------------

func TestCompile_PrintsOrderedDeclarations(t *testing.T) {
	t.Parallel()
	root := NewCommand(Deps{})
	yaml := writeYAML(t, strings.Join([]string{
		"package:",
		"  nginx:",
		"    state: installed",
		"file:",
		"  /etc/nginx.conf:",
		"    state: present",
		"    require: [{ package: nginx }]",
	}, "\n")+"\n")
	out, err := runCmd(t, root, "compile", yaml)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	// Require chain: package:nginx must come before file:/etc/nginx.conf.
	pkgIdx := strings.Index(out, "package:nginx")
	fileIdx := strings.Index(out, "file:/etc/nginx.conf")
	if pkgIdx < 0 || fileIdx < 0 || pkgIdx > fileIdx {
		t.Errorf("topo order broken; got:\n%s", out)
	}
	if !strings.Contains(out, "require=") {
		t.Errorf("missing require summary; got:\n%s", out)
	}
}

func TestCompile_RejectsIncludes(t *testing.T) {
	t.Parallel()
	root := NewCommand(Deps{})
	yaml := writeYAML(t, "includes:\n  - other.yaml\n")
	_, err := runCmd(t, root, "compile", yaml)
	if err == nil || !strings.Contains(err.Error(), "includes") {
		t.Errorf("expected includes-rejected error, got %v", err)
	}
}

func TestVarsGet_All(t *testing.T) {
	t.Parallel()
	root := NewCommand(Deps{})
	yaml := writeYAML(t, "variables:\n  port: 8080\n  user: www\n")
	out, err := runCmd(t, root, "vars", "get", yaml)
	if err != nil {
		t.Fatalf("vars get: %v", err)
	}
	if !strings.Contains(out, "port=8080") || !strings.Contains(out, "user=www") {
		t.Errorf("missing vars; got:\n%s", out)
	}
}

func TestVarsGet_OneKey(t *testing.T) {
	t.Parallel()
	root := NewCommand(Deps{})
	yaml := writeYAML(t, "variables:\n  port: 8080\n")
	out, err := runCmd(t, root, "vars", "get", yaml, "port")
	if err != nil {
		t.Fatalf("vars get: %v", err)
	}
	if strings.TrimSpace(out) != "8080" {
		t.Errorf("single-key output should be just the value; got %q", out)
	}
}

func TestVarsGet_VariableOverride(t *testing.T) {
	t.Parallel()
	root := NewCommand(Deps{})
	yaml := writeYAML(t, "variables:\n  port: 80\n")
	out, err := runCmd(t, root, "vars", "get", yaml, "port", "--variable", "port=8443")
	if err != nil {
		t.Fatalf("vars get: %v", err)
	}
	if strings.TrimSpace(out) != "8443" {
		t.Errorf("override should win; got %q", out)
	}
}

func TestVarsGet_MissingKey(t *testing.T) {
	t.Parallel()
	root := NewCommand(Deps{})
	yaml := writeYAML(t, "variables:\n  port: 80\n")
	_, err := runCmd(t, root, "vars", "get", yaml, "absent")
	if err == nil {
		t.Fatal("expected missing-key error")
	}
}

// ---- flag parsing -------------------------------------------------

func TestParseKeyValues(t *testing.T) {
	t.Parallel()
	out, err := parseKeyValues([]string{"a=1", "b=two", "c="})
	if err != nil {
		t.Fatalf("parseKeyValues: %v", err)
	}
	if out["a"] != "1" || out["b"] != "two" || out["c"] != "" {
		t.Errorf("parseKeyValues = %v", out)
	}
	if _, err := parseKeyValues([]string{"no-equals"}); err == nil {
		t.Error("expected error on missing =")
	}
	if _, err := parseKeyValues([]string{"=value"}); err == nil {
		t.Error("expected error on empty key")
	}
	if out, err := parseKeyValues(nil); err != nil || out != nil {
		t.Errorf("nil input: got %v err=%v, want nil/nil", out, err)
	}
}

func TestResolveSource(t *testing.T) {
	t.Parallel()
	if got := resolveSource("explicit", "fallback.yaml"); got != "explicit" {
		t.Errorf("got %q, want explicit", got)
	}
	if got := resolveSource("", "fallback.yaml"); got != "fallback.yaml" {
		t.Errorf("got %q, want fallback.yaml", got)
	}
}

func TestReadInputYAML_MissingFile(t *testing.T) {
	t.Parallel()
	_, _, err := readInputYAML([]string{"/no/such/file.yaml"})
	if err == nil {
		t.Error("expected read error")
	}
}

func TestReadInputYAML_RequiresArg(t *testing.T) {
	t.Parallel()
	_, _, err := readInputYAML(nil)
	if err == nil {
		t.Error("expected required-arg error")
	}
}
