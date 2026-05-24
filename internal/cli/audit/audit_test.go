// SPDX-License-Identifier: Apache-2.0

package audit_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	intaudit "go.keystone-core.io/keystone-core/internal/audit"
	cli "go.keystone-core.io/keystone-core/internal/cli/audit"
	"go.keystone-core.io/keystone-core/internal/controlplane"
	intpolicy "go.keystone-core.io/keystone-core/internal/policy"
	"go.keystone-core.io/keystone-core/internal/state"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

type rig struct {
	listen *bufconn.Listener
	audit  intaudit.AuditStore
}

func newRig(t *testing.T) *rig {
	t.Helper()
	st, err := state.NewStore(&state.Config{
		Backend: state.BackendSQLite,
		SQLite:  state.SQLiteConfig{Path: filepath.Join(t.TempDir(), "a.db")},
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	reg := intpolicy.NewRegistry()
	eng, _ := intpolicy.NewEngine(reg)
	auditLog := intaudit.NewSQLAuditStore(st)
	gen, err := intpolicy.NewReportGenerator(auditLog, intpolicy.NewControlMapping())
	if err != nil {
		t.Fatalf("NewReportGenerator: %v", err)
	}
	srv := controlplane.NewPolicyGRPCServer(eng, gen, auditLog, intaudit.NoopAuditor{})

	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	v1.RegisterPolicyServiceServer(gs, srv)
	go func() {
		if err := gs.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Logf("grpc serve: %v", err)
		}
	}()
	t.Cleanup(func() { gs.Stop(); _ = lis.Close() })

	return &rig{listen: lis, audit: auditLog}
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

func (r *rig) seed(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := r.audit.Store(ctx, intaudit.MustNewAuditEntry(intaudit.AuditEntryInput{
			PolicyID: "p", Action: "policy.evaluate", User: "alice",
			Allowed: i != 0, Severity: intaudit.SeverityHigh,
		})); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

func TestLog(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.seed(t)
	out, err := run(t, r.deps(), "log", "--since", "1h", "--user", "alice")
	if err != nil {
		t.Fatalf("log: %v\n%s", err, out)
	}
	if strings.Count(out, "policy.evaluate") != 3 {
		t.Errorf("log should list 3 entries:\n%s", out)
	}
	jout, err := run(t, r.deps(), "log", "-o", "json")
	if err != nil || !strings.Contains(jout, `"entries"`) {
		t.Errorf("log json: %v\n%s", err, jout)
	}
}

func TestReport(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.seed(t)
	out, err := run(t, r.deps(), "report", "--since", "7d")
	if err != nil {
		t.Fatalf("report: %v\n%s", err, out)
	}
	if !strings.Contains(out, "evaluations: 3") || !strings.Contains(out, "rate:") {
		t.Errorf("report output:\n%s", out)
	}
}

func TestStats(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.seed(t)
	out, err := run(t, r.deps(), "stats", "--since", "30d")
	if err != nil {
		t.Fatalf("stats: %v\n%s", err, out)
	}
	if !strings.Contains(out, "evaluations: 3") {
		t.Errorf("stats output:\n%s", out)
	}
	// stats omits the top-violations table / trend.
	if strings.Contains(out, "TOP POLICY") {
		t.Errorf("stats should not render the top-violations table:\n%s", out)
	}
}

func TestBadSince(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	if _, err := run(t, r.deps(), "log", "--since", "not-a-time"); err == nil {
		t.Errorf("bad --since should error")
	}
}

func TestBadOutput(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	if _, err := run(t, r.deps(), "log", "-o", "csv"); err == nil {
		t.Errorf("invalid --output should error")
	}
}
