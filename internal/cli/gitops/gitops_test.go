package gitops

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/gitops/rollback"
	"go.keystone-core.io/keystone-core/internal/gitops/verification"
)

// runCmd invokes the root with args and captures stdout/stderr.
func runCmd(t *testing.T, d Deps, args ...string) (string, string, error) {
	t.Helper()
	cmd := NewCommand(d)
	var out, errb bytes.Buffer
	cmd.SetArgs(args)
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	err := cmd.Execute()
	return out.String(), errb.String(), err
}

func TestVerify_HTTPWorkflow_Success(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)

	path := filepath.Join(t.TempDir(), "wf.yaml")
	if err := os.WriteFile(path, []byte(`name: smoke
parallel: true
timeout: 2m
steps:
  - name: api
    type: http
    config:
      url: `+srv.URL+`
      expect_status: 200
`), 0o644); err != nil {
		t.Fatal(err)
	}

	d := Deps{HTTPClient: srv.Client()}
	out, _, err := runCmd(t, d, "verify", path)
	if err != nil {
		t.Fatalf("verify: %v\n%s", err, out)
	}
	if !strings.Contains(out, "verdict=PASS") {
		t.Errorf("expected PASS in output, got %q", out)
	}
}

func TestVerify_AllThreeVerifierTypes_FlagOverrides(t *testing.T) {
	t.Parallel()
	// Acceptance line 104: kscorectl gitops verify file --parallel
	// --timeout 2m runs HTTP + gRPC + cmd verifiers.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	path := filepath.Join(t.TempDir(), "wf.yaml")
	yaml := `name: post-deploy
steps:
  - name: http-ok
    type: http
    config:
      url: ` + srv.URL + `
  - name: grpc-up
    type: grpc
    config:
      target: ignored
  - name: smoke
    type: command
    config:
      command: "true"
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	d := Deps{
		HTTPClient: srv.Client(),
		HealthCheck: func(context.Context, string, string, bool) (string, error) {
			return "SERVING", nil
		},
		CmdRunner: stubRunner{res: verification.CommandResult{ExitCode: 0}},
	}
	out, _, err := runCmd(t, d, "verify", path, "--parallel", "--timeout", "2m")
	if err != nil {
		t.Fatalf("verify: %v\n%s", err, out)
	}
	for _, s := range []string{"http-ok", "grpc-up", "smoke", "verdict=PASS"} {
		if !strings.Contains(out, s) {
			t.Errorf("missing %q in output:\n%s", s, out)
		}
	}
}

func TestVerify_FailingStepExitsNonZero(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "wf.yaml")
	_ = os.WriteFile(path, []byte(`name: f
steps:
  - name: dead
    type: http
    config:
      url: http://127.0.0.1:1/nope
`), 0o644)
	_, _, err := runCmd(t, Deps{HTTPClient: &http.Client{}}, "verify", path)
	if err == nil {
		t.Error("verify of unreachable URL = nil error, want failure")
	}
}

func TestRollback_ExecuteAndApprove_PersistsConfig(t *testing.T) {
	t.Parallel()
	// Acceptance 105 + 106 + the T8/T9 Config-persistence fix: a
	// RequireApproval=true rollback waits at Pending; a later
	// `rollback approve <id>` (separate invocation, same --store)
	// resumes and drives the rollback to completion using the
	// originally-supplied Config.
	store := filepath.Join(t.TempDir(), "rb.db")
	rec := &recordingGit{}
	d := Deps{GitClient: rec}

	out1, _, err := runCmd(t, d,
		"rollback",
		"--app", "web",
		"--executor", "git",
		"--strategy", "specific",
		"--revision", "abc",
		"--reason", "hotfix",
		"--repo-url", "https://example.com/r.git",
		"--branch", "main",
		"--require-approval",
		"--store", store,
	)
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out1)
	}
	if !strings.Contains(out1, "state=pending") {
		t.Fatalf("expected pending state in execute output:\n%s", out1)
	}
	id := extractID(t, out1)

	out2, _, err := runCmd(t, d, "rollback", "approve", id, "--store", store, "--approver", "alice")
	if err != nil {
		t.Fatalf("approve: %v\n%s", err, out2)
	}
	if !strings.Contains(out2, "state=completed") {
		t.Errorf("expected completed after approve:\n%s", out2)
	}
	if rec.got.RepoURL != "https://example.com/r.git" || rec.got.Branch != "main" || rec.got.ToRevision != "abc" {
		t.Errorf("git client got %+v, want repo+branch+revision from persisted Config", rec.got)
	}
}

func TestRollback_StrategyWithoutSubcommand(t *testing.T) {
	t.Parallel()
	// Acceptance 105 verbatim: `rollback --app web --strategy previous --reason ...`
	rec := &recordingGit{prev: "prevsha"}
	d := Deps{GitClient: rec}
	out, _, err := runCmd(t, d,
		"rollback",
		"--app", "web",
		"--strategy", "previous",
		"--reason", "hotfix",
		"--repo-url", "https://example.com/r.git",
		"--store", ":memory:",
	)
	if err != nil {
		t.Fatalf("rollback: %v\n%s", err, out)
	}
	if !strings.Contains(out, "state=completed") {
		t.Errorf("expected completed:\n%s", out)
	}
	if rec.got.ToRevision != "prevsha" {
		t.Errorf("strategy=previous didn't resolve via GitClient.PreviousRevision; got %+v", rec.got)
	}
}

func TestRollback_ListAndGet(t *testing.T) {
	t.Parallel()
	store := filepath.Join(t.TempDir(), "rb.db")
	rec := &recordingGit{}
	d := Deps{GitClient: rec}
	out, _, err := runCmd(t, d,
		"rollback", "--app", "web", "--strategy", "specific", "--revision", "c1",
		"--executor", "git", "--repo-url", "https://x", "--store", store)
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	id := extractID(t, out)

	listOut, _, err := runCmd(t, d, "rollback", "list", "--store", store)
	if err != nil || !strings.Contains(listOut, id) {
		t.Errorf("list: err=%v out=%s", err, listOut)
	}
	getOut, _, err := runCmd(t, d, "rollback", "get", id, "--store", store)
	if err != nil || !strings.Contains(getOut, "id="+id) {
		t.Errorf("get: err=%v out=%s", err, getOut)
	}
	_, _, err = runCmd(t, d, "rollback", "get", "missing", "--store", store)
	if err == nil {
		t.Error("get missing = nil error, want not-found")
	}
}

func TestRollback_RejectTerminal(t *testing.T) {
	t.Parallel()
	store := filepath.Join(t.TempDir(), "rb.db")
	rec := &recordingGit{}
	d := Deps{GitClient: rec}
	out, _, err := runCmd(t, d,
		"rollback", "--app", "web", "--strategy", "specific", "--revision", "c1",
		"--repo-url", "https://x", "--require-approval", "--store", store)
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	id := extractID(t, out)
	out, _, err = runCmd(t, d, "rollback", "reject", id, "--store", store, "--approver", "bob", "--reason", "no")
	if err != nil {
		t.Fatalf("reject: %v\n%s", err, out)
	}
	if !strings.Contains(out, "state=rejected") {
		t.Errorf("expected rejected state:\n%s", out)
	}
}

func TestExecCommandRunner_ZeroExit(t *testing.T) {
	t.Parallel()
	res, err := ExecCommandRunner{}.Run(context.Background(), verification.CommandRequest{Command: "true"})
	if err != nil || res.ExitCode != 0 {
		t.Errorf("true: exit=%d err=%v", res.ExitCode, err)
	}
}

func TestExecCommandRunner_NonZeroExit(t *testing.T) {
	t.Parallel()
	res, err := ExecCommandRunner{}.Run(context.Background(), verification.CommandRequest{Command: "false"})
	if err != nil {
		t.Fatalf("false: err=%v", err)
	}
	if res.ExitCode == 0 {
		t.Errorf("`false` should be non-zero, got %d", res.ExitCode)
	}
}

func TestExecCommandRunner_LaunchError(t *testing.T) {
	t.Parallel()
	_, err := ExecCommandRunner{}.Run(context.Background(),
		verification.CommandRequest{Command: "/no/such/binary/__kscore_test__"})
	if err == nil {
		t.Error("missing binary = nil error, want launch failure")
	}
}

// --- helpers ----------------------------------------------------------------

// stubRunner is the test [verification.CommandRunner].
type stubRunner struct {
	res verification.CommandResult
	err error
}

func (s stubRunner) Run(context.Context, verification.CommandRequest) (verification.CommandResult, error) {
	return s.res, s.err
}

// recordingGit captures the request the git client receives so tests
// can assert that the persisted Config crossed the approval gate.
type recordingGit struct {
	got  rollback.GitRevertRequest
	prev string
}

func (r *recordingGit) Revert(_ context.Context, req rollback.GitRevertRequest) (rollback.GitRevertResult, error) {
	r.got = req
	return rollback.GitRevertResult{FromRevision: "from", ToRevision: req.ToRevision, NewCommit: "new"}, nil
}
func (r *recordingGit) PreviousRevision(context.Context, string, string, string) (string, error) {
	if r.prev == "" {
		return "prevsha", nil
	}
	return r.prev, nil
}
func (r *recordingGit) LastKnownGood(context.Context, string, string, string) (string, error) {
	return "lkg", nil
}

func extractID(t *testing.T, out string) string {
	t.Helper()
	// Output format: `id=<uuid> state=...`. Pull the first token.
	idx := strings.Index(out, "id=")
	if idx < 0 {
		t.Fatalf("no id in output: %s", out)
	}
	rest := out[idx+len("id="):]
	end := strings.IndexAny(rest, " \t\n")
	if end < 0 {
		end = len(rest)
	}
	return rest[:end]
}

