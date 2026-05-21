package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"

	"go.keystone-core.io/keystone-core/internal/files"
	aclpkg "go.keystone-core.io/keystone-core/internal/files/acl"
	"go.keystone-core.io/keystone-core/internal/files/backend"
	natspkg "go.keystone-core.io/keystone-core/internal/nats"
	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

// Local json wrappers to keep the test file self-contained without
// reaching for encoding/json at every call site.
var (
	jsonMarshal   = json.Marshal
	jsonUnmarshal = json.Unmarshal
)

// --- embedded NATS rig -------------------------------------------------------

type rig struct {
	srv     *natsserver.Server
	conn    *nats.Conn
	subj    files.Subjects
	svc     *Service
	client  *Client
	store   backend.Store
	cluster string
}

func newRig(t *testing.T) *rig {
	t.Helper()

	opts := &natsserver.Options{
		Host:       "127.0.0.1",
		Port:       freePort(t),
		NoSigs:     true,
		NoLog:      true,
		MaxPayload: 4 * 1024 * 1024, // headroom for 1 MiB chunks + JSON+base64 envelope
	}
	srv, err := natsserver.NewServer(opts)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatal("embedded NATS not ready")
	}

	conn, err := nats.Connect(srv.ClientURL())
	if err != nil {
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatalf("connect: %v", err)
	}

	const cluster = "test"
	subjects, err := natspkg.NewSubjectBuilder(cluster)
	if err != nil {
		conn.Close()
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatalf("NewSubjectBuilder: %v", err)
	}

	store := backend.NewMemoryStore(nil)
	svc, err := NewService(conn, subjects, store, nil)
	if err != nil {
		conn.Close()
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatalf("NewService: %v", err)
	}
	if err := svc.Start(context.Background()); err != nil {
		conn.Close()
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatalf("svc.Start: %v", err)
	}

	client, err := NewClient(conn, subjects)
	if err != nil {
		_ = svc.Stop()
		conn.Close()
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatalf("NewClient: %v", err)
	}

	r := &rig{
		srv:     srv,
		conn:    conn,
		subj:    subjects,
		svc:     svc,
		client:  client,
		store:   store,
		cluster: cluster,
	}
	t.Cleanup(r.close)
	return r
}

func (r *rig) close() {
	_ = r.svc.Stop()
	if r.conn != nil {
		r.conn.Close()
	}
	if r.srv != nil {
		r.srv.Shutdown()
		r.srv.WaitForShutdown()
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return port
}

// --- roundtrip ---------------------------------------------------------------

func TestPutGetRoundTrip_SingleChunk(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	body := []byte("hello world")

	final, err := r.client.Put(ctx, files.FileMetadata{
		Path:        "configs/app.yaml",
		ContentType: "application/yaml",
		Tags:        map[string]string{"env": "prod"},
	}, body)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if final.Version != 1 {
		t.Errorf("version = %d, want 1", final.Version)
	}
	if final.Size != int64(len(body)) {
		t.Errorf("size = %d, want %d", final.Size, len(body))
	}

	meta, got, err := r.client.Get(ctx, "configs/app.yaml", GetOptions{})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("body = %q, want %q", got, body)
	}
	if meta.Hash != files.HashOf(body) {
		t.Errorf("hash = %q, want %q", meta.Hash, files.HashOf(body))
	}
	if meta.Tags["env"] != "prod" {
		t.Errorf("tags = %+v, want env=prod", meta.Tags)
	}
}

func TestPutGetRoundTrip_MultiChunk(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	// 2.5 MiB body — three chunks (1 MiB + 1 MiB + 0.5 MiB).
	body := make([]byte, files.ChunkSize*2+files.ChunkSize/2)
	for i := range body {
		body[i] = byte(i % 251)
	}

	if _, err := r.client.Put(ctx, files.FileMetadata{Path: "big/blob"}, body); err != nil {
		t.Fatalf("Put: %v", err)
	}

	_, got, err := r.client.Get(ctx, "big/blob", GetOptions{})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("body length = %d, want %d", len(got), len(body))
	}
}

func TestGet_NotFound(t *testing.T) {
	r := newRig(t)
	_, _, err := r.client.Get(context.Background(), "missing/path", GetOptions{})
	if err == nil {
		t.Fatal("want error for missing path")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %v, want substring 'not found'", err)
	}
}

func TestDelete(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	if _, err := r.client.Put(ctx, files.FileMetadata{Path: "tmp"}, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := r.client.Delete(ctx, "tmp"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, err := r.client.Get(ctx, "tmp", GetOptions{}); err == nil {
		t.Error("Get after delete should fail")
	}
}

func TestDelete_NotFound(t *testing.T) {
	r := newRig(t)
	err := r.client.Delete(context.Background(), "never/existed")
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %v, want 'not found'", err)
	}
}

func TestList(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	for _, p := range []string{"a/x", "a/y", "b/z"} {
		if _, err := r.client.Put(ctx, files.FileMetadata{Path: p}, []byte("z")); err != nil {
			t.Fatal(err)
		}
	}
	all, err := r.client.List(ctx, "")
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("len = %d, want 3", len(all))
	}

	aOnly, err := r.client.List(ctx, "a/")
	if err != nil {
		t.Fatalf("List prefix: %v", err)
	}
	if len(aOnly) != 2 {
		t.Errorf("prefix len = %d, want 2", len(aOnly))
	}
}

func TestStat(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	if _, err := r.client.Put(ctx, files.FileMetadata{Path: "x/y.txt"}, []byte("v")); err != nil {
		t.Fatal(err)
	}
	m, err := r.client.Stat(ctx, "x/y.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if m.Path != "x/y.txt" || m.Size != 1 {
		t.Errorf("Stat result = %+v", m)
	}
}

func TestGet_Resume_FromMidChunk(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	// 3-chunk body.
	body := make([]byte, files.ChunkSize*2+128)
	for i := range body {
		body[i] = byte(i % 200)
	}
	if _, err := r.client.Put(ctx, files.FileMetadata{Path: "resume/blob"}, body); err != nil {
		t.Fatal(err)
	}

	// Resume from chunk 2 (the trailing 128-byte partial chunk).
	_, got, err := r.client.Get(ctx, "resume/blob", GetOptions{FromChunk: 2})
	if err != nil {
		t.Fatalf("Get from chunk 2: %v", err)
	}
	tail := body[files.ChunkSize*2:]
	if !bytes.Equal(got, tail) {
		t.Errorf("resume body len=%d, want %d", len(got), len(tail))
	}
}

func TestGet_Resume_OutOfRange(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	body := make([]byte, 100)
	if _, err := r.client.Put(ctx, files.FileMetadata{Path: "small"}, body); err != nil {
		t.Fatal(err)
	}
	_, _, err := r.client.Get(ctx, "small", GetOptions{FromChunk: 1})
	if err == nil {
		t.Fatal("want error for from_chunk >= total")
	}
}

func TestGet_Resume_Negative(t *testing.T) {
	r := newRig(t)
	_, _, err := r.client.Get(context.Background(), "x", GetOptions{FromChunk: -1})
	if err == nil {
		t.Fatal("want error for negative from_chunk")
	}
}

func TestPut_HashRoundTrip(t *testing.T) {
	// The server-side store recomputes hash; we verify the client
	// computed hash matches what the server recomputed.
	r := newRig(t)
	ctx := context.Background()
	body := []byte("the brown fox")
	final, err := r.client.Put(ctx, files.FileMetadata{Path: "p"}, body)
	if err != nil {
		t.Fatal(err)
	}
	if final.Hash != files.HashOf(body) {
		t.Errorf("hash = %s, want %s", final.Hash, files.HashOf(body))
	}
}

func TestPut_ValidatesMetadata(t *testing.T) {
	r := newRig(t)
	_, err := r.client.Put(context.Background(), files.FileMetadata{Path: "/bad"}, []byte("x"))
	if err == nil {
		t.Fatal("want validation error")
	}
}

func TestServiceStart_Idempotent(t *testing.T) {
	r := newRig(t)
	if err := r.svc.Start(context.Background()); err == nil {
		t.Fatal("second Start should error")
	}
}

func TestServiceStop_Idempotent(t *testing.T) {
	r := newRig(t)
	if err := r.svc.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Stop(); err != nil {
		t.Fatalf("second Stop should be a no-op, got %v", err)
	}
}

func TestNewService_NilGuards(t *testing.T) {
	if _, err := NewService(nil, nil, nil, nil); err == nil {
		t.Fatal("want nil-conn error")
	}
}

func TestNewClient_NilGuards(t *testing.T) {
	if _, err := NewClient(nil, nil); err == nil {
		t.Fatal("want nil-conn error")
	}
}

func TestPut_VersionBumpsAcrossPuts(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	v1, err := r.client.Put(ctx, files.FileMetadata{Path: "p"}, []byte("a"))
	if err != nil {
		t.Fatal(err)
	}
	if v1.Version != 1 {
		t.Errorf("v1 = %d, want 1", v1.Version)
	}
	v2, err := r.client.Put(ctx, files.FileMetadata{Path: "p"}, []byte("ab"))
	if err != nil {
		t.Fatal(err)
	}
	if v2.Version != 2 {
		t.Errorf("v2 = %d, want 2", v2.Version)
	}
}

// TestService_BadRequestPayload covers the decodeRequest error
// branches: unparsable JSON, missing reqID header, mismatched
// operation, missing transferID.
func TestService_BadRequestPayload(t *testing.T) {
	r := newRig(t)

	t.Run("unparseable JSON", func(t *testing.T) {
		// Publish raw garbage to the put subject; expect a
		// response containing an unmarshal error if reqID header
		// is present, else service logs a warning.
		subj := r.subj.FilesRequest("put")
		respSub, ch := mustSubscribeResponse(t, r.conn, r.subj.FilesResponse("rq-1"))
		defer func() { _ = respSub.Unsubscribe() }()

		msg := nats.NewMsg(subj)
		msg.Data = []byte("not json")
		msg.Header.Set(HeaderRequestID, "rq-1")
		msg.Header.Set(HeaderTransferID, "tx-1")
		if err := r.conn.PublishMsg(msg); err != nil {
			t.Fatal(err)
		}
		resp := waitResp(t, ch)
		if resp.Error == "" {
			t.Errorf("want error, got %+v", resp)
		}
	})

	t.Run("operation mismatch", func(t *testing.T) {
		subj := r.subj.FilesRequest("put")
		respSub, ch := mustSubscribeResponse(t, r.conn, r.subj.FilesResponse("rq-2"))
		defer func() { _ = respSub.Unsubscribe() }()

		req := files.FileRequest{Operation: files.FileOpGet, Path: "x"}
		body, _ := jsonMarshal(req)
		msg := nats.NewMsg(subj)
		msg.Data = body
		msg.Header.Set(HeaderRequestID, "rq-2")
		msg.Header.Set(HeaderTransferID, "tx-2")
		if err := r.conn.PublishMsg(msg); err != nil {
			t.Fatal(err)
		}
		resp := waitResp(t, ch)
		if resp.Error == "" || !strings.Contains(resp.Error, "operation mismatch") {
			t.Errorf("want operation mismatch, got %+v", resp)
		}
	})

	t.Run("missing transfer id on put", func(t *testing.T) {
		subj := r.subj.FilesRequest("put")
		respSub, ch := mustSubscribeResponse(t, r.conn, r.subj.FilesResponse("rq-3"))
		defer func() { _ = respSub.Unsubscribe() }()

		req := files.FileRequest{
			Operation: files.FileOpPut,
			Path:      "x",
			Metadata:  &files.FileMetadata{Path: "x", Size: 1},
		}
		body, _ := jsonMarshal(req)
		msg := nats.NewMsg(subj)
		msg.Data = body
		msg.Header.Set(HeaderRequestID, "rq-3")
		// no transfer-id header
		if err := r.conn.PublishMsg(msg); err != nil {
			t.Fatal(err)
		}
		resp := waitResp(t, ch)
		if resp.Error == "" || !strings.Contains(resp.Error, "transfer-id") {
			t.Errorf("want transfer-id error, got %+v", resp)
		}
	})

	t.Run("put with nil metadata", func(t *testing.T) {
		subj := r.subj.FilesRequest("put")
		respSub, ch := mustSubscribeResponse(t, r.conn, r.subj.FilesResponse("rq-4"))
		defer func() { _ = respSub.Unsubscribe() }()

		req := files.FileRequest{Operation: files.FileOpPut, Path: "x"}
		body, _ := jsonMarshal(req)
		msg := nats.NewMsg(subj)
		msg.Data = body
		msg.Header.Set(HeaderRequestID, "rq-4")
		msg.Header.Set(HeaderTransferID, "tx-4")
		if err := r.conn.PublishMsg(msg); err != nil {
			t.Fatal(err)
		}
		resp := waitResp(t, ch)
		if resp.Error == "" || !strings.Contains(resp.Error, "metadata required") {
			t.Errorf("want metadata-required error, got %+v", resp)
		}
	})
}

func TestService_ListBackendError(t *testing.T) {
	// Path validation failure surfaces as response error.
	r := newRig(t)
	subj := r.subj.FilesRequest("list")
	respSub, ch := mustSubscribeResponse(t, r.conn, r.subj.FilesResponse("rq-list-bad"))
	defer func() { _ = respSub.Unsubscribe() }()

	req := files.FileRequest{Operation: files.FileOpList, Path: "../escape"}
	body, _ := jsonMarshal(req)
	msg := nats.NewMsg(subj)
	msg.Data = body
	msg.Header.Set(HeaderRequestID, "rq-list-bad")
	if err := r.conn.PublishMsg(msg); err != nil {
		t.Fatal(err)
	}
	resp := waitResp(t, ch)
	if resp.Error == "" {
		t.Errorf("want validation error, got %+v", resp)
	}
}

func TestPublishError_NoSubject(t *testing.T) {
	// Service.publishError with empty subject is a no-op — exercise
	// the early-return branch via the Service struct directly.
	r := newRig(t)
	r.svc.publishError("", errors.New("boom"))
}

func TestChunkCount_Boundary(t *testing.T) {
	cases := []struct {
		size int64
		want int
	}{
		{0, 1},
		{1, 1},
		{files.ChunkSize - 1, 1},
		{files.ChunkSize, 1},
		{files.ChunkSize + 1, 2},
		{2 * files.ChunkSize, 2},
		{2*files.ChunkSize + 1, 3},
	}
	for _, tc := range cases {
		if got := chunkCount(tc.size); got != tc.want {
			t.Errorf("chunkCount(%d) = %d, want %d", tc.size, got, tc.want)
		}
	}
}

func TestExtractTransferIDFromSubject(t *testing.T) {
	if got := extractTransferIDFromSubject("kscore.test.files.chunk.abc-123"); got != "abc-123" {
		t.Errorf("got %q", got)
	}
	if got := extractTransferIDFromSubject("nodots"); got != "nodots" {
		t.Errorf("got %q", got)
	}
}

func mustSubscribeResponse(t *testing.T, conn *nats.Conn, subj string) (*nats.Subscription, chan FileResponse) {
	t.Helper()
	ch := make(chan FileResponse, 4)
	sub, err := conn.Subscribe(subj, func(m *nats.Msg) {
		var resp FileResponse
		_ = jsonUnmarshal(m.Data, &resp)
		ch <- resp
	})
	if err != nil {
		t.Fatalf("subscribe %s: %v", subj, err)
	}
	return sub, ch
}

func waitResp(t *testing.T, ch <-chan FileResponse) FileResponse {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(2 * time.Second):
		t.Fatal("response timeout")
	}
	return FileResponse{}
}

// --- ACL wiring --------------------------------------------------------------

// rigWithACL extends newRig with an ACL + auditor + a client that
// carries a chosen principal. Keeps the integration boilerplate
// out of every ACL test.
type rigWithACL struct {
	*rig
	auditMu      sync.Mutex
	auditCalls   []auditCall
	clientByRole map[auth.Role]*Client
}

type auditCall struct {
	id     string
	role   auth.Role
	op     files.FileOperation
	path   string
	reason string
}

func newRigWithACL(t *testing.T, acl aclpkg.ACL) *rigWithACL {
	t.Helper()
	r := newRig(t)
	// Stop the rig's default (no-ACL) service and rebuild with ACL +
	// auditor wired.
	if err := r.svc.Stop(); err != nil {
		t.Fatal(err)
	}
	w := &rigWithACL{rig: r, clientByRole: make(map[auth.Role]*Client)}
	auditor := func(p *auth.Principal, op files.FileOperation, path string, reason error) {
		w.auditMu.Lock()
		defer w.auditMu.Unlock()
		w.auditCalls = append(w.auditCalls, auditCall{
			id:     principalID(p),
			role:   principalRole(p),
			op:     op,
			path:   path,
			reason: reason.Error(),
		})
	}
	svc, err := NewService(r.conn, r.subj, r.store, nil,
		WithACL(acl),
		WithAuditor(auditor),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.svc = svc

	for _, role := range []auth.Role{auth.RoleNone, auth.RoleReadonly, auth.RoleOperator, auth.RoleAdmin} {
		client, err := NewClient(r.conn, r.subj, WithPrincipal(&auth.Principal{
			ID:   "p-" + role.String(),
			Role: role,
		}))
		if err != nil {
			t.Fatal(err)
		}
		w.clientByRole[role] = client
	}
	return w
}

func principalID(p *auth.Principal) string {
	if p == nil {
		return ""
	}
	return p.ID
}

func principalRole(p *auth.Principal) auth.Role {
	if p == nil {
		return auth.RoleNone
	}
	return p.Role
}

func TestACL_AllowsAdminAndPerRule_DeniesOthers(t *testing.T) {
	acl := aclpkg.NewRoleACL(
		aclpkg.WithRule("configs", files.FileOpGet, auth.RoleReadonly),
		aclpkg.WithRule("configs", files.FileOpPut, auth.RoleOperator),
	)
	w := newRigWithACL(t, acl)
	ctx := context.Background()

	// Operator can put to configs/.
	if _, err := w.clientByRole[auth.RoleOperator].Put(ctx,
		files.FileMetadata{Path: "configs/app.yaml"},
		[]byte("v1"),
	); err != nil {
		t.Fatalf("operator put configs: %v", err)
	}

	// Readonly can get configs/.
	if _, _, err := w.clientByRole[auth.RoleReadonly].Get(ctx, "configs/app.yaml", GetOptions{}); err != nil {
		t.Fatalf("readonly get configs: %v", err)
	}

	// Readonly cannot put.
	_, err := w.clientByRole[auth.RoleReadonly].Put(ctx,
		files.FileMetadata{Path: "configs/other.yaml"},
		[]byte("v"),
	)
	if err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("readonly put: want forbidden, got %v", err)
	}

	// Admin bypass: put to a namespace with no rule.
	if _, err := w.clientByRole[auth.RoleAdmin].Put(ctx,
		files.FileMetadata{Path: "system/secret.yaml"},
		[]byte("z"),
	); err != nil {
		t.Errorf("admin put system: %v", err)
	}

	// None role: closed-by-default; even Get configs (which only
	// requires Readonly) is denied — but wait, a None role does NOT
	// satisfy Readonly minimum. So this should be forbidden.
	_, _, err = w.clientByRole[auth.RoleNone].Get(ctx, "configs/app.yaml", GetOptions{})
	if err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("none-role get configs: want forbidden, got %v", err)
	}
}

func TestACL_AuditorCalledOnDeny(t *testing.T) {
	acl := aclpkg.NewRoleACL() // closed-by-default
	w := newRigWithACL(t, acl)

	_, _, err := w.clientByRole[auth.RoleReadonly].Get(context.Background(), "any/file", GetOptions{})
	if err == nil {
		t.Fatal("want forbidden")
	}

	// Wait for the auditor to fire (the service publishes the deny
	// asynchronously; the client returns when the response arrives).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		w.auditMu.Lock()
		n := len(w.auditCalls)
		w.auditMu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	w.auditMu.Lock()
	defer w.auditMu.Unlock()
	if len(w.auditCalls) != 1 {
		t.Fatalf("audit calls = %d, want 1", len(w.auditCalls))
	}
	ac := w.auditCalls[0]
	if ac.role != auth.RoleReadonly {
		t.Errorf("audit role = %s, want readonly", ac.role)
	}
	if ac.op != files.FileOpGet {
		t.Errorf("audit op = %s, want get", ac.op)
	}
	if ac.path != "any/file" {
		t.Errorf("audit path = %s, want any/file", ac.path)
	}
}

func TestACL_NilACL_AllowsAll(t *testing.T) {
	// Existing rig (newRig) builds the service with nil ACL.
	// Verify by sending a request from a no-principal client —
	// should succeed.
	r := newRig(t)
	ctx := context.Background()
	if _, err := r.client.Put(ctx, files.FileMetadata{Path: "anywhere"}, []byte("z")); err != nil {
		t.Errorf("nil-ACL Put should succeed, got %v", err)
	}
}

func TestACL_DenyOnList(t *testing.T) {
	// List with prefix "secret/" should be denied under closed-by-
	// default ACL for a readonly principal.
	acl := aclpkg.NewRoleACL()
	w := newRigWithACL(t, acl)
	_, err := w.clientByRole[auth.RoleReadonly].List(context.Background(), "secret/foo")
	if err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("readonly list secret/: want forbidden, got %v", err)
	}
}

func TestACL_DenyOnDelete(t *testing.T) {
	acl := aclpkg.NewRoleACL(
		aclpkg.WithRule("tmp", files.FileOpDelete, auth.RoleOperator),
	)
	w := newRigWithACL(t, acl)
	ctx := context.Background()

	// Seed via admin (bypass).
	if _, err := w.clientByRole[auth.RoleAdmin].Put(ctx, files.FileMetadata{Path: "tmp/file"}, []byte("z")); err != nil {
		t.Fatal(err)
	}

	if err := w.clientByRole[auth.RoleReadonly].Delete(ctx, "tmp/file"); err == nil ||
		!strings.Contains(err.Error(), "forbidden") {
		t.Errorf("readonly delete: want forbidden, got %v", err)
	}
	if err := w.clientByRole[auth.RoleOperator].Delete(ctx, "tmp/file"); err != nil {
		t.Errorf("operator delete: %v", err)
	}
}

func TestACL_HeadersPassPrincipal(t *testing.T) {
	// Verify that a Client without WithPrincipal sends no headers,
	// and one with WithPrincipal sets both ID + Role.
	r := newRig(t)
	defaultC, _ := NewClient(r.conn, r.subj)
	if defaultC.principal != nil {
		t.Error("default Client.principal should be nil")
	}
	p := &auth.Principal{ID: "u-7", Role: auth.RoleOperator}
	withP, _ := NewClient(r.conn, r.subj, WithPrincipal(p))
	if withP.principal != p {
		t.Errorf("Client.principal = %+v, want %+v", withP.principal, p)
	}
}

func TestPrincipalFromHeaders_Empty(t *testing.T) {
	m := nats.NewMsg("any")
	if got := principalFromHeaders(m); got != nil {
		t.Errorf("empty headers should return nil, got %+v", got)
	}
}

func TestPrincipalFromHeaders_Populated(t *testing.T) {
	m := nats.NewMsg("any")
	m.Header.Set(HeaderPrincipalID, "abc")
	m.Header.Set(HeaderPrincipalRole, "operator")
	got := principalFromHeaders(m)
	if got == nil || got.ID != "abc" || got.Role != auth.RoleOperator {
		t.Errorf("got = %+v", got)
	}
}

func TestPrincipalFromHeaders_UnknownRoleCoerces(t *testing.T) {
	m := nats.NewMsg("any")
	m.Header.Set(HeaderPrincipalID, "abc")
	m.Header.Set(HeaderPrincipalRole, "wizard")
	got := principalFromHeaders(m)
	if got == nil || got.Role != auth.RoleNone {
		t.Errorf("unknown role should coerce to RoleNone, got %+v", got)
	}
}

func TestClient_ContextCancel(t *testing.T) {
	r := newRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := r.client.Get(ctx, "x", GetOptions{})
	if err == nil {
		t.Fatal("want context-cancel error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
