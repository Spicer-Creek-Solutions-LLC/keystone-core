package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"

	"go.keystone-core.io/keystone-core/internal/files"
	"go.keystone-core.io/keystone-core/internal/files/backend"
	natspkg "go.keystone-core.io/keystone-core/internal/nats"
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
