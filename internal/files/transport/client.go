// SPDX-License-Identifier: Apache-2.0

package transport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"

	"go.keystone-core.io/keystone-core/internal/files"
	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

// Client is the client-side file-transport handle. Method semantics
// mirror [backend.Store] so callers see a remote-store-shaped API.
type Client struct {
	conn      *nats.Conn
	subjects  files.Subjects
	principal *auth.Principal

	// Timeout is the wait budget for each request's complete-
	// response cycle. Zero means use the package default.
	Timeout time.Duration
}

// ClientOption configures a [Client] at construction.
type ClientOption func(*Client)

// WithPrincipal sets the default principal the client attaches to
// every outbound request (via Kscore-Principal-Id and -Role
// headers). The server-side ACL gates the request against this
// identity. A nil principal — or omitting the option — sends
// requests with no identity headers; the ACL sees a nil principal
// and applies its default policy.
func WithPrincipal(p *auth.Principal) ClientOption {
	return func(c *Client) { c.principal = p }
}

// defaultClientTimeout is the per-call wait budget for the client's
// response subject. Operators tune via [Client.Timeout].
const defaultClientTimeout = 60 * time.Second

// NewClient returns a Client bound to conn + subjects. Optional
// [ClientOption] values configure the default principal etc.
func NewClient(conn *nats.Conn, subjects files.Subjects, opts ...ClientOption) (*Client, error) {
	if conn == nil {
		return nil, errors.New("transport: nats conn must not be nil")
	}
	if subjects == nil {
		return nil, errors.New("transport: subjects must not be nil")
	}
	c := &Client{conn: conn, subjects: subjects}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// GetOptions controls a [Client.Get] call.
type GetOptions struct {
	// FromChunk resumes a partial download — the service starts
	// streaming from this chunk index instead of 0. Use after a
	// previous Get returned a partial-receive error.
	FromChunk int
}

// Get fetches the file at path. On success it returns the
// authoritative metadata and the assembled body. Per-chunk and
// overall SHA-256 are verified — a mismatch returns an error.
func (c *Client) Get(ctx context.Context, path string, opts GetOptions) (files.FileMetadata, []byte, error) {
	if opts.FromChunk < 0 {
		return files.FileMetadata{}, nil, fmt.Errorf("get: from_chunk must be >= 0, got %d", opts.FromChunk)
	}

	reqID := uuid.NewString()
	transferID := reqID
	respSubj := c.subjects.FilesResponse(reqID)
	chunkSubj := c.subjects.FilesChunk(transferID)

	respCh := make(chan FileResponse, 1)
	respSub, err := c.subscribeResponse(respSubj, respCh)
	if err != nil {
		return files.FileMetadata{}, nil, err
	}
	defer func() { _ = respSub.Unsubscribe() }()

	chunkCh := make(chan *files.FileChunk, 64)
	chunkSub, err := c.subscribeChunks(chunkSubj, chunkCh)
	if err != nil {
		return files.FileMetadata{}, nil, err
	}
	defer func() { _ = chunkSub.Unsubscribe() }()

	if err := c.publishRequest(reqID, transferID, files.FileRequest{
		Operation: files.FileOpGet,
		Path:      path,
		FromChunk: opts.FromChunk,
	}); err != nil {
		return files.FileMetadata{}, nil, err
	}

	resp, err := c.waitForResponse(ctx, respCh)
	if err != nil {
		return files.FileMetadata{}, nil, err
	}
	if resp.Error != "" {
		return files.FileMetadata{}, nil, errors.New(resp.Error)
	}
	if resp.Metadata == nil {
		return files.FileMetadata{}, nil, errors.New("get: response missing metadata")
	}
	meta := *resp.Metadata
	body, err := c.collectGetChunks(ctx, chunkCh, resp.Total, opts.FromChunk, meta)
	if err != nil {
		return meta, nil, err
	}
	return meta, body, nil
}

// Stat returns the latest metadata for path without transferring
// the body. Implemented as a get + "discard chunks" path would
// waste bandwidth, so Stat uses a list-with-prefix-equal-to-path
// shortcut on the wire — the service implementation maps this to
// store.Stat. v1.0 carries Stat over the list operation; v1.x may
// add a dedicated stat operation if the abuse rate justifies it.
func (c *Client) Stat(ctx context.Context, path string) (files.FileMetadata, error) {
	list, err := c.List(ctx, path)
	if err != nil {
		return files.FileMetadata{}, err
	}
	for _, m := range list {
		if m.Path == path {
			return m, nil
		}
	}
	return files.FileMetadata{}, fmt.Errorf("stat: %q not found", path)
}

// Put uploads body at path with the supplied metadata. Returns the
// server-assigned final metadata (with Version, Hash, Size, and
// CreatedAt filled in).
func (c *Client) Put(ctx context.Context, meta files.FileMetadata, body []byte) (files.FileMetadata, error) {
	if err := meta.Validate(); err != nil {
		return files.FileMetadata{}, err
	}
	// Compute size + hash up front so the put-request can declare
	// Total (lets the service size its chunk channel and detect
	// missing chunks via timeout).
	meta.Size = int64(len(body))
	meta.Hash = hashOf(body)

	reqID := uuid.NewString()
	transferID := reqID
	respSubj := c.subjects.FilesResponse(reqID)
	chunkSubj := c.subjects.FilesChunk(transferID)

	respCh := make(chan FileResponse, 2)
	respSub, err := c.subscribeResponse(respSubj, respCh)
	if err != nil {
		return files.FileMetadata{}, err
	}
	defer func() { _ = respSub.Unsubscribe() }()

	if err := c.publishRequest(reqID, transferID, files.FileRequest{
		Operation: files.FileOpPut,
		Path:      meta.Path,
		Metadata:  &meta,
	}); err != nil {
		return files.FileMetadata{}, err
	}

	ready, err := c.waitForResponse(ctx, respCh)
	if err != nil {
		return files.FileMetadata{}, err
	}
	if ready.Error != "" {
		return files.FileMetadata{}, errors.New(ready.Error)
	}
	if ready.Status != StatusReady {
		return files.FileMetadata{}, fmt.Errorf("put: expected ready, got %q", ready.Status)
	}

	if err := c.publishChunks(meta.Path, transferID, chunkSubj, body, ready.Total); err != nil {
		return files.FileMetadata{}, err
	}

	final, err := c.waitForResponse(ctx, respCh)
	if err != nil {
		return files.FileMetadata{}, err
	}
	if final.Error != "" {
		return files.FileMetadata{}, errors.New(final.Error)
	}
	if final.Metadata == nil {
		return files.FileMetadata{}, errors.New("put: final response missing metadata")
	}
	return *final.Metadata, nil
}

// List returns metadata for every file with the given prefix.
func (c *Client) List(ctx context.Context, prefix string) ([]files.FileMetadata, error) {
	reqID := uuid.NewString()
	respSubj := c.subjects.FilesResponse(reqID)

	respCh := make(chan FileResponse, 1)
	respSub, err := c.subscribeResponse(respSubj, respCh)
	if err != nil {
		return nil, err
	}
	defer func() { _ = respSub.Unsubscribe() }()

	if err := c.publishRequest(reqID, "", files.FileRequest{
		Operation: files.FileOpList,
		Path:      prefix,
	}); err != nil {
		return nil, err
	}

	resp, err := c.waitForResponse(ctx, respCh)
	if err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}
	return resp.List, nil
}

// Delete removes the file at path.
func (c *Client) Delete(ctx context.Context, path string) error {
	reqID := uuid.NewString()
	respSubj := c.subjects.FilesResponse(reqID)

	respCh := make(chan FileResponse, 1)
	respSub, err := c.subscribeResponse(respSubj, respCh)
	if err != nil {
		return err
	}
	defer func() { _ = respSub.Unsubscribe() }()

	if err := c.publishRequest(reqID, "", files.FileRequest{
		Operation: files.FileOpDelete,
		Path:      path,
	}); err != nil {
		return err
	}

	resp, err := c.waitForResponse(ctx, respCh)
	if err != nil {
		return err
	}
	if resp.Error != "" {
		return errors.New(resp.Error)
	}
	return nil
}

// --- helpers -----------------------------------------------------------------

func (c *Client) subscribeResponse(subj string, ch chan<- FileResponse) (*nats.Subscription, error) {
	return c.conn.Subscribe(subj, func(m *nats.Msg) {
		var resp FileResponse
		if err := json.Unmarshal(m.Data, &resp); err != nil {
			resp = FileResponse{Error: fmt.Sprintf("unmarshal response: %v", err)}
		}
		select {
		case ch <- resp:
		default:
		}
	})
}

func (c *Client) subscribeChunks(subj string, ch chan<- *files.FileChunk) (*nats.Subscription, error) {
	return c.conn.Subscribe(subj, func(m *nats.Msg) {
		var chunk files.FileChunk
		if err := json.Unmarshal(m.Data, &chunk); err != nil {
			return
		}
		select {
		case ch <- &chunk:
		default:
		}
	})
}

func (c *Client) publishRequest(reqID, transferID string, req files.FileRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	msg := nats.NewMsg(c.subjects.FilesRequest(string(req.Operation)))
	msg.Data = data
	msg.Header.Set(HeaderRequestID, reqID)
	if transferID != "" {
		msg.Header.Set(HeaderTransferID, transferID)
	}
	if c.principal != nil {
		if c.principal.ID != "" {
			msg.Header.Set(HeaderPrincipalID, c.principal.ID)
		}
		msg.Header.Set(HeaderPrincipalRole, c.principal.Role.String())
	}
	return c.conn.PublishMsg(msg)
}

func (c *Client) waitForResponse(ctx context.Context, ch <-chan FileResponse) (FileResponse, error) {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = defaultClientTimeout
	}
	select {
	case <-ctx.Done():
		return FileResponse{}, ctx.Err()
	case <-time.After(timeout):
		return FileResponse{}, fmt.Errorf("response timeout after %s", timeout)
	case resp := <-ch:
		return resp, nil
	}
}

// collectGetChunks gathers chunks fromChunk..total-1 and verifies
// per-chunk + assembled SHA-256 against meta.Hash. On partial
// receive (timeout / context cancel), returns the accumulated bytes
// plus an error so callers know how many chunks landed and can
// resume from fromChunk + len(received).
func (c *Client) collectGetChunks(ctx context.Context, chunks <-chan *files.FileChunk, total, fromChunk int, meta files.FileMetadata) ([]byte, error) {
	if fromChunk >= total {
		return nil, fmt.Errorf("get: from_chunk %d >= total %d", fromChunk, total)
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = defaultClientTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	received := make([][]byte, total)
	count := 0
	for count < total-fromChunk {
		select {
		case <-ctx.Done():
			return concat(received, fromChunk), ctx.Err()
		case <-timer.C:
			return concat(received, fromChunk), fmt.Errorf("chunk timeout after %s (got %d of %d expected)", timeout, count, total-fromChunk)
		case ch := <-chunks:
			if ch.FileID != meta.Path {
				return nil, fmt.Errorf("chunk file_id mismatch: want %q, got %q", meta.Path, ch.FileID)
			}
			if ch.Total != total {
				return nil, fmt.Errorf("chunk total mismatch: want %d, got %d", total, ch.Total)
			}
			if ch.Index < fromChunk || ch.Index >= total {
				return nil, fmt.Errorf("chunk index %d outside [%d,%d)", ch.Index, fromChunk, total)
			}
			if received[ch.Index] != nil {
				continue
			}
			if err := ch.Validate(); err != nil {
				return nil, fmt.Errorf("chunk %d invalid: %w", ch.Index, err)
			}
			received[ch.Index] = ch.Data
			count++
		}
	}

	body := concat(received, fromChunk)
	// Verify assembled hash only when starting from offset 0; on
	// resume the client lacks the leading chunks and cannot
	// recompute the full-body hash. The caller is responsible for
	// stitching multiple resumed segments and verifying.
	if fromChunk == 0 {
		if h := hashOf(body); h != meta.Hash {
			return nil, fmt.Errorf("assembled body hash mismatch: want %s, got %s", meta.Hash, h)
		}
	}
	return body, nil
}

// publishChunks slices body into chunks 0..total-1 and publishes
// each on chunkSubj. Each chunk carries its own SHA-256.
func (c *Client) publishChunks(path, transferID, chunkSubj string, body []byte, total int) error {
	if total <= 0 {
		return fmt.Errorf("publishChunks: total must be > 0, got %d", total)
	}
	chunkSize := files.ChunkSize
	for i := 0; i < total; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > len(body) {
			end = len(body)
		}
		data := body[start:end]
		// Copy so the chunk's slice is independent of the caller's
		// buffer; per the wire-format invariant the chunk's Data
		// is treated as immutable.
		dataCopy := make([]byte, len(data))
		copy(dataCopy, data)
		chunk := files.FileChunk{
			ID:     transferID,
			FileID: path,
			Index:  i,
			Total:  total,
			Data:   dataCopy,
			Hash:   files.HashOf(dataCopy),
		}
		payload, err := json.Marshal(chunk)
		if err != nil {
			return err
		}
		if err := c.conn.Publish(chunkSubj, payload); err != nil {
			return fmt.Errorf("publish chunk %d: %w", i, err)
		}
	}
	// Flush so chunks are on the wire before we wait for the
	// service's final response.
	return c.conn.Flush()
}

// concat assembles the received slices in order. Holes (nil
// entries before fromChunk are expected — those bytes were never
// requested) are skipped. Holes at index >= fromChunk indicate a
// missing chunk and produce a zero-byte hole; the caller decides
// what to do.
func concat(received [][]byte, fromChunk int) []byte {
	var size int
	for i := fromChunk; i < len(received); i++ {
		size += len(received[i])
	}
	out := make([]byte, 0, size)
	for i := fromChunk; i < len(received); i++ {
		out = append(out, received[i]...)
	}
	return out
}

// hashOf is a small wrapper so client + service share the canonical
// hex SHA-256 form without depending on files.HashOf in both files
// (files.HashOf already exists; this is just a forward without
// drifting). Inlined to avoid a tiny adapter — keeps the file
// self-contained.
func hashOf(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
