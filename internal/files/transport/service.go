package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"go.keystone-core.io/keystone-core/internal/files"
	"go.keystone-core.io/keystone-core/internal/files/acl"
	"go.keystone-core.io/keystone-core/internal/files/backend"
	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

// Service is the server-side file-transport handler. It subscribes
// to files.request.* on Start and dispatches each request against
// the configured [backend.Store]. One Service per file-service node.
type Service struct {
	conn     *nats.Conn
	subjects files.Subjects
	store    backend.Store
	logger   *slog.Logger
	acl      acl.ACL
	auditor  Auditor

	// PutTimeout bounds how long the service waits for all chunks
	// to arrive after a put-ready reply. Zero means use the
	// package default.
	PutTimeout time.Duration

	mu    sync.Mutex
	subs  []*nats.Subscription
	state struct {
		started bool
	}
}

// Auditor is the optional hook the Service calls when an ACL
// denial occurs. Wire it from boot to bridge into Epic 12's audit
// store; v1.0 ships no in-package implementation so the audit
// trail is up to the operator. A nil Auditor disables the hook.
type Auditor func(principal *auth.Principal, op files.FileOperation, path string, reason error)

// ServiceOption configures a [Service] at construction time.
type ServiceOption func(*Service)

// WithACL attaches an ACL to gate every inbound request. A nil
// ACL is allowed and equivalent to omitting the option (no
// gating) — kept that way so existing callers stay green when
// they have not wired identity yet.
func WithACL(a acl.ACL) ServiceOption {
	return func(s *Service) { s.acl = a }
}

// WithAuditor attaches a deny-audit callback. Called synchronously
// inside the request handler, so the operator-supplied function
// should be fast (write-and-return; offload heavy work).
func WithAuditor(a Auditor) ServiceOption {
	return func(s *Service) { s.auditor = a }
}

// defaultPutTimeout is the per-transfer wait budget for inbound
// chunks after the service has acknowledged the put. It defaults
// generously — operator-tuned values land in Task 14 / 15 config.
const defaultPutTimeout = 60 * time.Second

// NewService returns a Service that will dispatch requests against
// store using subjects. A nil logger maps to [slog.Default].
// Optional [ServiceOption] values wire an ACL and / or an Auditor.
func NewService(conn *nats.Conn, subjects files.Subjects, store backend.Store, logger *slog.Logger, opts ...ServiceOption) (*Service, error) {
	if conn == nil {
		return nil, errors.New("transport: nats conn must not be nil")
	}
	if subjects == nil {
		return nil, errors.New("transport: subjects must not be nil")
	}
	if store == nil {
		return nil, errors.New("transport: backend store must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	s := &Service{
		conn:     conn,
		subjects: subjects,
		store:    store,
		logger:   logger,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// Start registers subscriptions for every file operation. It is
// idempotent at the "already started" check level — calling Start
// twice on the same Service returns an error.
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.started {
		return errors.New("transport: service already started")
	}

	ops := []struct {
		op      files.FileOperation
		handler func(ctx context.Context, m *nats.Msg)
	}{
		{files.FileOpGet, s.handleGet},
		{files.FileOpPut, s.handlePut},
		{files.FileOpList, s.handleList},
		{files.FileOpDelete, s.handleDelete},
	}
	for _, o := range ops {
		op := o.op
		h := o.handler
		subj := s.subjects.FilesRequest(string(op))
		sub, err := s.conn.Subscribe(subj, func(m *nats.Msg) {
			h(ctx, m)
		})
		if err != nil {
			// Best-effort tear down any subscriptions we already
			// registered before this one failed.
			for _, existing := range s.subs {
				_ = existing.Unsubscribe()
			}
			s.subs = nil
			return fmt.Errorf("transport: subscribe %s: %w", subj, err)
		}
		s.subs = append(s.subs, sub)
	}
	// Flush queued SUB protocol messages to the server before declaring
	// the service ready; the nats client buffers subscriptions and an
	// immediate request after Start can otherwise reach the server before
	// the SUB is processed, producing a "no responders" timeout.
	if err := s.conn.Flush(); err != nil {
		for _, existing := range s.subs {
			_ = existing.Unsubscribe()
		}
		s.subs = nil
		return fmt.Errorf("transport: flush subscriptions: %w", err)
	}
	s.state.started = true
	return nil
}

// Stop unsubscribes the request handlers. It is idempotent — Stop
// on a never-started Service is a no-op.
func (s *Service) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.state.started {
		return nil
	}
	var first error
	for _, sub := range s.subs {
		if err := sub.Unsubscribe(); err != nil && first == nil {
			first = err
		}
	}
	s.subs = nil
	s.state.started = false
	return first
}

// --- request handlers --------------------------------------------------------

func (s *Service) handleGet(ctx context.Context, m *nats.Msg) {
	d, ok := s.decodeRequest(m, files.FileOpGet)
	if !ok {
		return
	}
	if !s.authorize(ctx, d, d.req.Path) {
		return
	}
	respSubj := s.subjects.FilesResponse(d.reqID)

	meta, body, err := s.store.Get(ctx, d.req.Path)
	if err != nil {
		s.publishError(respSubj, err)
		return
	}
	defer func() { _ = body.Close() }()

	total := chunkCount(meta.Size)
	if err := s.publishResponse(respSubj, FileResponse{
		Status:    StatusDone,
		Metadata:  &meta,
		Total:     total,
		ChunkSize: files.ChunkSize,
	}); err != nil {
		s.logger.Warn("transport: publish get response failed", "err", err)
		return
	}

	chunkSubj := s.subjects.FilesChunk(d.transferID)
	if err := s.streamChunks(ctx, body, meta, total, d.req.FromChunk, chunkSubj); err != nil {
		s.logger.Warn("transport: stream chunks failed", "err", err, "path", d.req.Path)
	}
}

func (s *Service) handlePut(ctx context.Context, m *nats.Msg) {
	d, ok := s.decodeRequest(m, files.FileOpPut)
	if !ok {
		return
	}
	if !s.authorize(ctx, d, d.req.Path) {
		return
	}
	respSubj := s.subjects.FilesResponse(d.reqID)

	if d.req.Metadata == nil {
		s.publishError(respSubj, errors.New("put: metadata required"))
		return
	}
	if d.req.Metadata.Size <= 0 {
		s.publishError(respSubj, errors.New("put: metadata.size required and must be > 0"))
		return
	}

	total := chunkCount(d.req.Metadata.Size)
	chunkSubj := s.subjects.FilesChunk(d.transferID)

	// Subscribe to the chunk subject BEFORE replying ready so the
	// client's chunks land on the registered subscription.
	chunks := make(chan *files.FileChunk, total)
	sub, err := s.conn.Subscribe(chunkSubj, func(cm *nats.Msg) {
		var c files.FileChunk
		if jerr := json.Unmarshal(cm.Data, &c); jerr != nil {
			s.logger.Warn("transport: chunk unmarshal", "err", jerr)
			return
		}
		select {
		case chunks <- &c:
		default:
			s.logger.Warn("transport: chunk channel full", "transfer_id", d.transferID)
		}
	})
	if err != nil {
		s.publishError(respSubj, fmt.Errorf("subscribe chunks: %w", err))
		return
	}
	defer func() { _ = sub.Unsubscribe() }()

	if err := s.publishResponse(respSubj, FileResponse{
		Status:    StatusReady,
		Total:     total,
		ChunkSize: files.ChunkSize,
	}); err != nil {
		s.publishError(respSubj, fmt.Errorf("publish ready: %w", err))
		return
	}

	body, err := s.collectChunks(ctx, chunks, total, d.req.Path)
	if err != nil {
		s.publishError(respSubj, err)
		return
	}

	final, err := s.store.Put(ctx, *d.req.Metadata, body)
	if err != nil {
		s.publishError(respSubj, fmt.Errorf("backend put: %w", err))
		return
	}
	_ = s.publishResponse(respSubj, FileResponse{
		Status:   StatusDone,
		Metadata: &final,
	})
}

func (s *Service) handleList(ctx context.Context, m *nats.Msg) {
	d, ok := s.decodeRequest(m, files.FileOpList)
	if !ok {
		return
	}
	if !s.authorize(ctx, d, d.req.Path) {
		return
	}
	respSubj := s.subjects.FilesResponse(d.reqID)

	list, err := s.store.List(ctx, d.req.Path)
	if err != nil {
		s.publishError(respSubj, err)
		return
	}
	_ = s.publishResponse(respSubj, FileResponse{
		Status: StatusDone,
		List:   list,
	})
}

func (s *Service) handleDelete(ctx context.Context, m *nats.Msg) {
	d, ok := s.decodeRequest(m, files.FileOpDelete)
	if !ok {
		return
	}
	if !s.authorize(ctx, d, d.req.Path) {
		return
	}
	respSubj := s.subjects.FilesResponse(d.reqID)

	if err := s.store.Delete(ctx, d.req.Path); err != nil {
		s.publishError(respSubj, err)
		return
	}
	_ = s.publishResponse(respSubj, FileResponse{Status: StatusDone})
}

// --- helpers -----------------------------------------------------------------

// decoded carries the per-request state every handler needs after
// header + payload parsing.
type decoded struct {
	req        files.FileRequest
	reqID      string
	transferID string
	principal  *auth.Principal
}

// decodeRequest parses the inbound message + headers; on any
// validation failure it publishes an error on the response subject
// (if reqID is known) and returns ok=false. The caller short-
// circuits when ok=false.
func (s *Service) decodeRequest(m *nats.Msg, want files.FileOperation) (d decoded, ok bool) {
	d.reqID = m.Header.Get(HeaderRequestID)
	d.transferID = m.Header.Get(HeaderTransferID)
	d.principal = principalFromHeaders(m)

	if err := json.Unmarshal(m.Data, &d.req); err != nil {
		if d.reqID != "" {
			s.publishError(s.subjects.FilesResponse(d.reqID), fmt.Errorf("unmarshal request: %w", err))
		} else {
			s.logger.Warn("transport: unmarshal request + no reqID header", "err", err)
		}
		return d, false
	}
	if err := d.req.Validate(); err != nil {
		s.publishError(s.subjects.FilesResponse(d.reqID), err)
		return d, false
	}
	if d.req.Operation != want {
		s.publishError(s.subjects.FilesResponse(d.reqID),
			fmt.Errorf("operation mismatch: want %q, got %q", want, d.req.Operation))
		return d, false
	}
	if d.reqID == "" {
		s.logger.Warn("transport: missing request-id header", "operation", want)
		return d, false
	}
	// transferID is only required for chunked ops.
	if (want == files.FileOpGet || want == files.FileOpPut) && d.transferID == "" {
		s.publishError(s.subjects.FilesResponse(d.reqID), errors.New("missing transfer-id header"))
		return d, false
	}
	return d, true
}

// authorize runs the configured ACL against the request. Returns
// true on allow / no-ACL; false (after publishing the deny + audit
// hook) on deny.
func (s *Service) authorize(ctx context.Context, d decoded, path string) bool {
	if s.acl == nil {
		return true
	}
	ns := files.Namespace(path)
	if err := s.acl.Authorize(ctx, d.principal, d.req.Operation, ns); err != nil {
		if s.auditor != nil {
			s.auditor(d.principal, d.req.Operation, path, err)
		}
		s.publishError(s.subjects.FilesResponse(d.reqID), err)
		return false
	}
	return true
}

// principalFromHeaders reconstructs a [*auth.Principal] from the
// two header fields the transport client sets. An empty role
// header maps to RoleNone (per [auth.ParseRole]); unrecognised
// roles are silently coerced to RoleNone — the ACL decides what
// that means (typically: deny).
func principalFromHeaders(m *nats.Msg) *auth.Principal {
	id := m.Header.Get(HeaderPrincipalID)
	roleStr := m.Header.Get(HeaderPrincipalRole)
	if id == "" && roleStr == "" {
		return nil
	}
	role, _ := auth.ParseRole(roleStr)
	return &auth.Principal{ID: id, Role: role}
}

func (s *Service) publishResponse(subj string, resp FileResponse) error {
	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return s.conn.Publish(subj, b)
}

func (s *Service) publishError(subj string, err error) {
	if subj == "" {
		return
	}
	if pubErr := s.publishResponse(subj, FileResponse{
		Status: StatusDone,
		Error:  err.Error(),
	}); pubErr != nil {
		s.logger.Warn("transport: publish error response failed", "err", pubErr)
	}
}

// streamChunks reads body, slices it into [files.ChunkSize]-sized
// chunks, and publishes chunks fromChunk..total-1 on chunkSubj.
// Chunks 0..fromChunk-1 are still read+discarded so the offset is
// correct — the backend reader does not guarantee seekability.
func (s *Service) streamChunks(ctx context.Context, body io.Reader, meta files.FileMetadata, total, fromChunk int, chunkSubj string) error {
	if fromChunk < 0 || fromChunk >= total {
		return fmt.Errorf("invalid from_chunk %d for total %d", fromChunk, total)
	}
	buf := make([]byte, files.ChunkSize)
	for i := 0; i < total; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, err := io.ReadFull(body, buf)
		switch {
		case errors.Is(err, io.ErrUnexpectedEOF):
			// final partial chunk — n bytes still valid.
		case errors.Is(err, io.EOF):
			return nil
		case err != nil:
			return fmt.Errorf("read chunk %d: %w", i, err)
		}
		if i < fromChunk {
			continue
		}
		data := make([]byte, n)
		copy(data, buf[:n])
		chunk := files.FileChunk{
			ID:     extractTransferIDFromSubject(chunkSubj),
			FileID: meta.Path,
			Index:  i,
			Total:  total,
			Data:   data,
			Hash:   files.HashOf(data),
		}
		payload, err := json.Marshal(chunk)
		if err != nil {
			return err
		}
		if err := s.conn.Publish(chunkSubj, payload); err != nil {
			return fmt.Errorf("publish chunk %d: %w", i, err)
		}
	}
	return nil
}

// collectChunks blocks until total chunks have arrived (or the
// timeout elapses) and returns the reassembled body via an io.Reader.
// Per-chunk hash and index bounds are verified; out-of-order chunks
// are tolerated since each carries its own Index.
func (s *Service) collectChunks(ctx context.Context, chunks <-chan *files.FileChunk, total int, path string) (io.Reader, error) {
	timeout := s.PutTimeout
	if timeout == 0 {
		timeout = defaultPutTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	received := make([][]byte, total)
	count := 0
	for count < total {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			return nil, fmt.Errorf("put: chunk-collection timeout after %s (have %d of %d)", timeout, count, total)
		case c := <-chunks:
			if c.FileID != path {
				return nil, fmt.Errorf("chunk file_id mismatch: want %q, got %q", path, c.FileID)
			}
			if c.Total != total {
				return nil, fmt.Errorf("chunk total mismatch: want %d, got %d", total, c.Total)
			}
			if c.Index < 0 || c.Index >= total {
				return nil, fmt.Errorf("chunk index %d out of range [0,%d)", c.Index, total)
			}
			if err := c.Validate(); err != nil {
				return nil, fmt.Errorf("chunk %d invalid: %w", c.Index, err)
			}
			if received[c.Index] != nil {
				// duplicate; ignore (put-side resume v1.x will need
				// proper dedupe semantics, but for now skip)
				continue
			}
			received[c.Index] = c.Data
			count++
		}
	}
	// Concatenate in order.
	var size int
	for _, b := range received {
		size += len(b)
	}
	out := make([]byte, 0, size)
	for _, b := range received {
		out = append(out, b...)
	}
	return bytes.NewReader(out), nil
}

// chunkCount returns how many [files.ChunkSize]-sized chunks cover
// size bytes. Zero-byte files use one zero-length chunk so the
// transfer always has at least one element.
func chunkCount(size int64) int {
	if size <= 0 {
		return 1
	}
	n := size / int64(files.ChunkSize)
	if size%int64(files.ChunkSize) != 0 {
		n++
	}
	return int(n)
}

// extractTransferIDFromSubject pulls the last dot-separated token
// off the subject — the chunk subject is `...files.chunk.<id>`.
// Used only to populate FileChunk.ID for the wire-format invariant
// (subscribers don't actually parse it).
func extractTransferIDFromSubject(subj string) string {
	for i := len(subj) - 1; i >= 0; i-- {
		if subj[i] == '.' {
			return subj[i+1:]
		}
	}
	return subj
}
