package policy_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"go.keystone-core.io/keystone-core/internal/audit"
	cli "go.keystone-core.io/keystone-core/internal/cli/policy"
	"go.keystone-core.io/keystone-core/internal/controlplane"
	intpolicy "go.keystone-core.io/keystone-core/internal/policy"
	"go.keystone-core.io/keystone-core/internal/state"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

type rig struct {
	listen *bufconn.Listener
	engine *intpolicy.Engine
	audit  audit.AuditStore
}

func newRig(t *testing.T) *rig {
	t.Helper()
	st, err := state.NewStore(&state.Config{
		Backend: state.BackendSQLite,
		SQLite:  state.SQLiteConfig{Path: filepath.Join(t.TempDir(), "p.db")},
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	reg := intpolicy.NewRegistry()
	eng, err := intpolicy.NewEngine(reg,
		intpolicy.WithEvaluator(audit.PolicyTypeBuiltin, intpolicy.NewBuiltinEvaluator()),
	)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	auditLog := audit.NewSQLAuditStore(st)
	gen, err := intpolicy.NewReportGenerator(auditLog, intpolicy.NewControlMapping())
	if err != nil {
		t.Fatalf("NewReportGenerator: %v", err)
	}
	srv := controlplane.NewPolicyGRPCServer(eng, gen, auditLog, audit.NoopAuditor{})

	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	v1.RegisterPolicyServiceServer(gs, srv)
	go func() {
		if err := gs.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Logf("grpc serve: %v", err)
		}
	}()
	t.Cleanup(func() { gs.Stop(); _ = lis.Close() })

	return &rig{listen: lis, engine: eng, audit: auditLog}
}

func (r *rig) deps() cli.Deps {
	return cli.Deps{
		Dial: func(_ context.Context, _, _ string) (v1.PolicyServiceClient, io.Closer, error) {
			conn, err := grpc.NewClient("passthrough://bufnet",
				grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
					return r.listen.DialContext(ctx)
				}),
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			)
			if err != nil {
				return nil, nil, err
			}
			return v1.NewPolicyServiceClient(conn), conn, nil
		},
	}
}

func (r *rig) regBuiltin(t *testing.T, id, code string, enabled bool) {
	t.Helper()
	if err := r.engine.Registry().RegisterPolicy(&intpolicy.Policy{
		ID: id, Name: id, Type: audit.PolicyTypeBuiltin,
		Category: intpolicy.CategorySecurity, Severity: audit.SeverityHigh,
		EnforcementMode: audit.EnforcementModeAudit, Code: code, Enabled: enabled,
	}); err != nil {
		t.Fatalf("RegisterPolicy %s: %v", id, err)
	}
}

func run(t *testing.T, deps cli.Deps, args ...string) (string, error) {
	t.Helper()
	cmd := cli.NewCommand(deps)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	cmd.SetContext(context.Background())
	err := cmd.Execute()
	return buf.String(), err
}

func TestList(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.regBuiltin(t, "p-b", `{"rule":"allowed-actions","allowed":["x"]}`, true)
	r.regBuiltin(t, "p-a", `{"rule":"allowed-actions","allowed":["x"]}`, true)

	out, err := run(t, r.deps(), "list")
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	if !strings.Contains(out, "p-a") || !strings.Contains(out, "p-b") || !strings.Contains(out, "total: 2") {
		t.Errorf("list output:\n%s", out)
	}
	jout, err := run(t, r.deps(), "list", "-o", "json")
	if err != nil || !strings.Contains(jout, `"id"`) {
		t.Errorf("list json: %v\n%s", err, jout)
	}
}

func TestShow(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.regBuiltin(t, "p1", `{"rule":"allowed-actions","allowed":["read"]}`, true)
	out, err := run(t, r.deps(), "show", "p1")
	if err != nil || !strings.Contains(out, `"id"`) || !strings.Contains(out, "p1") {
		t.Errorf("show: %v\n%s", err, out)
	}
	if _, err := run(t, r.deps(), "show", "ghost"); err == nil {
		t.Errorf("show ghost should error (NotFound)")
	}
}

func TestViolations(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	_ = r.audit.Store(context.Background(), audit.MustNewAuditEntry(audit.AuditEntryInput{
		PolicyID: "p", Action: "policy.evaluate", Allowed: false, Severity: audit.SeverityHigh,
		Violations: []audit.Violation{{Rule: "r", Message: "no", Severity: audit.SeverityHigh}},
	}))
	_ = r.audit.Store(context.Background(), audit.MustNewAuditEntry(audit.AuditEntryInput{
		PolicyID: "p", Action: "policy.evaluate", Allowed: true, Severity: audit.SeverityLow,
	}))
	out, err := run(t, r.deps(), "violations")
	if err != nil {
		t.Fatalf("violations: %v\n%s", err, out)
	}
	if !strings.Contains(out, "high") || strings.Count(out, "policy.evaluate") != 1 {
		t.Errorf("violations should show only the 1 denied entry:\n%s", out)
	}
	// --severity critical filters the high one out client-side.
	out2, _ := run(t, r.deps(), "violations", "--severity", "critical")
	if strings.Contains(out2, "policy.evaluate") {
		t.Errorf("--severity critical should exclude the high entry:\n%s", out2)
	}
}

func TestCompliance(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	for i := 0; i < 3; i++ {
		_ = r.audit.Store(context.Background(), audit.MustNewAuditEntry(audit.AuditEntryInput{
			PolicyID: "p", Action: "policy.evaluate", Allowed: i != 0, Severity: audit.SeverityHigh,
		}))
	}
	out, err := run(t, r.deps(), "compliance", "--since", "1h")
	if err != nil {
		t.Fatalf("compliance: %v\n%s", err, out)
	}
	if !strings.Contains(out, "evaluations:   3") || !strings.Contains(out, "rate:") {
		t.Errorf("compliance output:\n%s", out)
	}
}

func TestEvalLocal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bi := filepath.Join(dir, "p.json")
	if err := os.WriteFile(bi, []byte(`{"rule":"allowed-actions","allowed":["read"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	inp := filepath.Join(dir, "in.json")
	if err := os.WriteFile(inp, []byte(`{"action":"read","user":"alice"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// No deps needed — eval is local/in-process.
	out, err := run(t, cli.Deps{}, "eval", bi, "--input", inp)
	if err != nil {
		t.Fatalf("eval: %v\n%s", err, out)
	}
	if !strings.Contains(out, "ALLOW") {
		t.Errorf("eval allow expected:\n%s", out)
	}
	// Deny path.
	inDeny := filepath.Join(dir, "deny.json")
	_ = os.WriteFile(inDeny, []byte(`{"action":"delete"}`), 0o600)
	out2, _ := run(t, cli.Deps{}, "eval", bi, "--input", inDeny)
	if !strings.Contains(out2, "DENY") {
		t.Errorf("eval deny expected:\n%s", out2)
	}
}

func TestEvalLocal_OPAandCEL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rego := filepath.Join(dir, "p.rego")
	_ = os.WriteFile(rego, []byte("package keystone.policy\n\nallow := true\n"), 0o600)
	if out, err := run(t, cli.Deps{}, "eval", rego); err != nil || !strings.Contains(out, "ALLOW") {
		t.Errorf("opa eval: %v\n%s", err, out)
	}
	cel := filepath.Join(dir, "p.cel")
	_ = os.WriteFile(cel, []byte(`action == "read"`), 0o600)
	inp := filepath.Join(dir, "in.json")
	_ = os.WriteFile(inp, []byte(`{"action":"read"}`), 0o600)
	if out, err := run(t, cli.Deps{}, "eval", cel, "--input", inp); err != nil || !strings.Contains(out, "ALLOW") {
		t.Errorf("cel eval: %v\n%s", err, out)
	}
}

func TestValidateLocal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	good := filepath.Join(dir, "good.rego")
	_ = os.WriteFile(good, []byte("package keystone.policy\n\nallow := true\n"), 0o600)
	if out, err := run(t, cli.Deps{}, "validate", good); err != nil || !strings.Contains(out, "valid") {
		t.Errorf("validate good: %v\n%s", err, out)
	}
	bad := filepath.Join(dir, "bad.rego")
	_ = os.WriteFile(bad, []byte("this is not (((rego"), 0o600)
	if _, err := run(t, cli.Deps{}, "validate", bad); err == nil {
		t.Errorf("validate bad should error")
	}
	// Unknown extension without --type.
	mystery := filepath.Join(dir, "p.txt")
	_ = os.WriteFile(mystery, []byte("x"), 0o600)
	if _, err := run(t, cli.Deps{}, "validate", mystery); err == nil {
		t.Errorf("unknown extension should require --type")
	}
}

func TestBadOutputFlag(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	if _, err := run(t, r.deps(), "list", "-o", "yaml"); err == nil {
		t.Errorf("invalid --output should error")
	}
}
