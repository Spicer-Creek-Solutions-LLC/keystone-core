package controlplane

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	bp "go.keystone-core.io/keystone-core/internal/blueprint"
	rb "go.keystone-core.io/keystone-core/internal/runbook"
	"go.keystone-core.io/keystone-core/internal/statemgmt"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// ---- fakes ----------------------------------------------------------------

type fakeBPCatalog struct {
	list []*bp.Manifest
	get  *bp.Manifest
}

func (f fakeBPCatalog) List(context.Context) ([]*bp.Manifest, error) { return f.list, nil }
func (f fakeBPCatalog) Get(_ context.Context, name string) (*bp.Manifest, error) {
	if f.get == nil || f.get.Metadata.Name != name {
		return nil, errors.New("not found: " + name)
	}
	return f.get, nil
}

type fakeBPApplier struct {
	res *bp.ApplyResult
	err error
}

func (f fakeBPApplier) Apply(context.Context, string, bp.ApplyOptions) (*bp.ApplyResult, error) {
	return f.res, f.err
}

type fakeRBCatalog struct {
	list []*rb.Runbook
	get  *rb.Runbook
}

func (f fakeRBCatalog) List(context.Context) ([]*rb.Runbook, error) { return f.list, nil }
func (f fakeRBCatalog) Get(_ context.Context, id string) (*rb.Runbook, error) {
	if f.get == nil || f.get.Metadata.Name != id {
		return nil, errors.New("not found: " + id)
	}
	return f.get, nil
}

type fakeRBRunner struct {
	exec *rb.Execution
	err  error
}

func (f fakeRBRunner) Execute(context.Context, string, map[string]any) (*rb.Execution, error) {
	return f.exec, f.err
}

type fakeRBStore struct{ e *rb.Execution }

func (f fakeRBStore) Get(_ context.Context, id string) (*rb.Execution, error) {
	if f.e == nil || f.e.ID != id {
		return nil, errors.New("execution not found: " + id)
	}
	return f.e, nil
}

// ---- rigs -----------------------------------------------------------------

func bpRig(t *testing.T, srv *BlueprintGRPCServer) v1.BlueprintServiceClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	v1.RegisterBlueprintServiceServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return v1.NewBlueprintServiceClient(conn)
}

func rbRig(t *testing.T, srv *RunbookGRPCServer) v1.RunbookServiceClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	v1.RegisterRunbookServiceServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return v1.NewRunbookServiceClient(conn)
}

func code(err error) codes.Code { return status.Code(err) }

// ---- blueprint ------------------------------------------------------------

func TestBlueprintService_Unwired(t *testing.T) {
	cl := bpRig(t, &BlueprintGRPCServer{})
	ctx := context.Background()
	if _, err := cl.ListBlueprints(ctx, &v1.ListBlueprintsRequest{}); code(err) != codes.Unavailable {
		t.Fatalf("list code=%v", code(err))
	}
	if _, err := cl.GetBlueprint(ctx, &v1.GetBlueprintRequest{Name: "x"}); code(err) != codes.Unavailable {
		t.Fatalf("get code=%v", code(err))
	}
	if _, err := cl.ApplyBlueprint(ctx, &v1.ApplyBlueprintRequest{Name: "x"}); code(err) != codes.Unavailable {
		t.Fatalf("apply code=%v", code(err))
	}
}

func TestBlueprintService_QueryApply(t *testing.T) {
	m := &bp.Manifest{
		Metadata:    bp.Metadata{Name: "demo", Version: "1.0.0", Description: "d"},
		Parameters:  map[string]bp.ParamSpec{"pw": {Type: "string", Sensitive: true, Source: bp.SourceSecret}},
		Entrypoints: bp.Entrypoints{Default: "apply.yaml"},
	}
	srv := &BlueprintGRPCServer{
		Catalog: fakeBPCatalog{list: []*bp.Manifest{m}, get: m},
		Applier: fakeBPApplier{res: &bp.ApplyResult{
			RunID: "r1", Status: "succeeded",
			Report:  &statemgmt.RunReport{Total: 2, Changed: 1},
			Outputs: map[string]any{"summary": "ok"},
		}},
	}
	cl := bpRig(t, srv)
	ctx := context.Background()

	lr, err := cl.ListBlueprints(ctx, &v1.ListBlueprintsRequest{})
	if err != nil || lr.TotalCount != 1 || lr.Blueprints[0].Name != "demo" {
		t.Fatalf("list=%+v err=%v", lr, err)
	}
	if len(lr.Blueprints[0].Parameters) != 1 || lr.Blueprints[0].Parameters[0] != "pw" {
		t.Fatalf("expected param NAME only, got %+v", lr.Blueprints[0].Parameters)
	}

	gr, err := cl.GetBlueprint(ctx, &v1.GetBlueprintRequest{Name: "demo"})
	if err != nil || gr.Blueprint.Version != "1.0.0" {
		t.Fatalf("get=%+v err=%v", gr, err)
	}
	if _, err := cl.GetBlueprint(ctx, &v1.GetBlueprintRequest{}); code(err) != codes.InvalidArgument {
		t.Fatalf("empty name code=%v", code(err))
	}
	if _, err := cl.GetBlueprint(ctx, &v1.GetBlueprintRequest{Name: "missing"}); code(err) != codes.NotFound {
		t.Fatalf("missing code=%v", code(err))
	}

	ar, err := cl.ApplyBlueprint(ctx, &v1.ApplyBlueprintRequest{
		Name: "demo", Params: map[string]string{"pw": "secret://kv/db"},
	})
	if err != nil || ar.RunId != "r1" || ar.Status != "succeeded" || ar.Report.Total != 2 {
		t.Fatalf("apply=%+v err=%v", ar, err)
	}
	// The secret input must not be echoed anywhere in the response.
	if strings.Contains(ar.String(), "secret://kv/db") {
		t.Fatalf("apply response leaked secret input: %s", ar.String())
	}
}

func TestBlueprintService_ApplyFailedVsSetupError(t *testing.T) {
	ctx := context.Background()

	// ran-but-failed → response with status, NOT a gRPC error.
	fail := bpRig(t, &BlueprintGRPCServer{Applier: fakeBPApplier{
		res: &bp.ApplyResult{RunID: "r2", Status: "failed", Report: &statemgmt.RunReport{Failed: 1}},
		err: bp.ErrApplyFailed,
	}})
	ar, err := fail.ApplyBlueprint(ctx, &v1.ApplyBlueprintRequest{Name: "demo"})
	if err != nil || ar.Status != "failed" || ar.Report.Failed != 1 {
		t.Fatalf("failed-apply ar=%+v err=%v", ar, err)
	}

	// setup error, no result → Internal.
	se := bpRig(t, &BlueprintGRPCServer{Applier: fakeBPApplier{err: errors.New("boom")}})
	if _, err := se.ApplyBlueprint(ctx, &v1.ApplyBlueprintRequest{Name: "demo"}); code(err) != codes.Internal {
		t.Fatalf("setup-err code=%v", code(err))
	}
}

// ---- runbook --------------------------------------------------------------

func TestRunbookService_Unwired(t *testing.T) {
	cl := rbRig(t, &RunbookGRPCServer{})
	ctx := context.Background()
	if _, err := cl.ListRunbooks(ctx, &v1.ListRunbooksRequest{}); code(err) != codes.Unavailable {
		t.Fatalf("list code=%v", code(err))
	}
	if _, err := cl.ExecuteRunbook(ctx, &v1.ExecuteRunbookRequest{Runbook: "x"}); code(err) != codes.Unavailable {
		t.Fatalf("execute code=%v", code(err))
	}
	if _, err := cl.GetRunbookExecution(ctx, &v1.GetRunbookExecutionRequest{Id: "x"}); code(err) != codes.Unavailable {
		t.Fatalf("getexec code=%v", code(err))
	}
}

func TestRunbookService_QueryExecuteStatus(t *testing.T) {
	r := &rb.Runbook{Metadata: rb.Metadata{Name: "db-restart", Version: "1.0.0"},
		Spec: rb.Spec{Steps: []rb.Step{{Type: "noop", Name: "stop"}}}}
	okExec := &rb.Execution{ID: "e1", Runbook: "db-restart", Status: rb.StatusSucceeded}
	srv := &RunbookGRPCServer{
		Catalog: fakeRBCatalog{list: []*rb.Runbook{r}, get: r},
		Runner:  fakeRBRunner{exec: okExec},
		Store:   fakeRBStore{e: okExec},
	}
	cl := rbRig(t, srv)
	ctx := context.Background()

	lr, err := cl.ListRunbooks(ctx, &v1.ListRunbooksRequest{})
	if err != nil || lr.TotalCount != 1 || lr.Runbooks[0].Name != "db-restart" {
		t.Fatalf("list=%+v err=%v", lr, err)
	}
	gr, err := cl.GetRunbook(ctx, &v1.GetRunbookRequest{Id: "db-restart"})
	if err != nil || len(gr.Runbook.Steps) != 1 {
		t.Fatalf("get=%+v err=%v", gr, err)
	}
	if _, err := cl.GetRunbook(ctx, &v1.GetRunbookRequest{}); code(err) != codes.InvalidArgument {
		t.Fatalf("empty id code=%v", code(err))
	}

	er, err := cl.ExecuteRunbook(ctx, &v1.ExecuteRunbookRequest{Runbook: "db-restart", Inputs: map[string]string{"agent_id": "a1"}})
	if err != nil || er.Execution.Status != "succeeded" {
		t.Fatalf("execute=%+v err=%v", er, err)
	}
	xr, err := cl.GetRunbookExecution(ctx, &v1.GetRunbookExecutionRequest{Id: "e1"})
	if err != nil || xr.Execution.Id != "e1" {
		t.Fatalf("getexec=%+v err=%v", xr, err)
	}
	if _, err := cl.GetRunbookExecution(ctx, &v1.GetRunbookExecutionRequest{Id: "nope"}); code(err) == codes.OK {
		t.Fatal("missing execution should error")
	}
}

func TestRunbookService_ExecuteFailedVsSetupError(t *testing.T) {
	ctx := context.Background()

	failExec := &rb.Execution{ID: "e2", Status: rb.StatusFailed}
	fr := rbRig(t, &RunbookGRPCServer{Runner: fakeRBRunner{exec: failExec, err: rb.ErrExecutionFailed}})
	er, err := fr.ExecuteRunbook(ctx, &v1.ExecuteRunbookRequest{Runbook: "x"})
	if err != nil || er.Execution.Status != "failed" {
		t.Fatalf("failed-exec er=%+v err=%v", er, err)
	}

	se := rbRig(t, &RunbookGRPCServer{Runner: fakeRBRunner{err: errors.New("boom")}})
	if _, err := se.ExecuteRunbook(ctx, &v1.ExecuteRunbookRequest{Runbook: "x"}); code(err) != codes.Internal {
		t.Fatalf("setup-err code=%v", code(err))
	}
}
