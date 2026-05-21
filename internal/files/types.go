package files

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ChunkSize is the v1.0 default per-chunk byte budget. PROJECT-
// DETAILS §4.20 fixes this at 1 MiB; smaller would inflate the
// per-chunk overhead, larger would butt against JetStream's
// message-size cap.
const ChunkSize = 1 << 20

// hashHexRe matches a lowercase hex SHA-256 string. Uppercase is
// rejected on purpose so the round-trip serialisation is canonical.
var hashHexRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// FileOperation enumerates the four CRUD verbs the file service
// understands. Operation drives which other [FileRequest] fields
// are required (see [FileRequest.Validate]).
type FileOperation string

const (
	FileOpPut    FileOperation = "put"
	FileOpGet    FileOperation = "get"
	FileOpList   FileOperation = "list"
	FileOpDelete FileOperation = "delete"
)

// Valid reports whether op is one of the four canonical operations.
func (op FileOperation) Valid() bool {
	switch op {
	case FileOpPut, FileOpGet, FileOpList, FileOpDelete:
		return true
	default:
		return false
	}
}

// FileMetadata describes a file's properties. Path is the namespaced
// slash-delimited address; Hash is the hex SHA-256 of the assembled
// body; Version is monotonic per Path and assigned server-side on
// put.
type FileMetadata struct {
	Path        string            `json:"path"`
	Size        int64             `json:"size"`
	Hash        string            `json:"hash"`
	ContentType string            `json:"content_type,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	Version     int64             `json:"version"`
	Tags        map[string]string `json:"tags,omitempty"`
}

// Validate enforces FileMetadata invariants. Hash may be empty on a
// put request (the server computes it on assembly); on a get
// response Hash is required and must be hex SHA-256.
func (m *FileMetadata) Validate() error {
	if err := validateFilePath(m.Path); err != nil {
		return fmt.Errorf("metadata.path: %w", err)
	}
	if m.Size < 0 {
		return fmt.Errorf("metadata.size: must not be negative, got %d", m.Size)
	}
	if m.Hash != "" && !hashHexRe.MatchString(m.Hash) {
		return fmt.Errorf("metadata.hash: must be lowercase hex SHA-256 (64 chars), got %q", m.Hash)
	}
	if m.Version < 0 {
		return fmt.Errorf("metadata.version: must not be negative, got %d", m.Version)
	}
	return nil
}

// FileRequest is the operation-agnostic envelope the bus carries for
// put / get / list / delete. Body is the small-file fast path —
// transfers larger than [ChunkSize] use the [FileChunk] streaming
// path instead.
type FileRequest struct {
	Operation FileOperation `json:"operation"`
	Path      string        `json:"path,omitempty"`
	Metadata  *FileMetadata `json:"metadata,omitempty"`
	Body      []byte        `json:"body,omitempty"`
}

// Validate enforces per-operation rules:
//
//	put     Path required; Metadata may be nil (server fills it) but
//	        if supplied must Validate; Body and the chunked path are
//	        the two valid bodies — Validate does not enforce
//	        Body-or-chunks here (it is the transport's job).
//	get     Path required.
//	delete  Path required.
//	list    Path optional — empty means "list all files in scope";
//	        Metadata + Body must be empty.
func (r *FileRequest) Validate() error {
	if !r.Operation.Valid() {
		return fmt.Errorf("request.operation: invalid %q (want put|get|list|delete)", r.Operation)
	}

	switch r.Operation {
	case FileOpPut, FileOpGet, FileOpDelete:
		if err := validateFilePath(r.Path); err != nil {
			return fmt.Errorf("request.path: %w", err)
		}
	case FileOpList:
		if r.Path != "" {
			if err := validateFilePath(r.Path); err != nil {
				return fmt.Errorf("request.path: %w", err)
			}
		}
		if r.Metadata != nil {
			return errors.New("request.metadata: must be nil for list operation")
		}
		if r.Body != nil {
			return errors.New("request.body: must be nil for list operation")
		}
	}

	if r.Metadata != nil {
		if err := r.Metadata.Validate(); err != nil {
			return fmt.Errorf("request.%w", err)
		}
	}
	return nil
}

// FileChunk is one slice of a streaming put or get. Index runs
// 0..Total-1; the receiver verifies Hash against SHA-256(Data) before
// appending, and computes the assembled FileMetadata.Hash across
// every chunk's Data in order.
type FileChunk struct {
	ID     string `json:"id"`      // uuid for the streaming transfer
	FileID string `json:"file_id"` // FileMetadata.Path
	Index  int    `json:"index"`
	Total  int    `json:"total"`
	Data   []byte `json:"data"`
	Hash   string `json:"hash"` // hex SHA-256 of Data
}

// Validate enforces chunk invariants. Hash is recomputed and
// compared — a chunk with a mismatched Hash is rejected before any
// I/O, matching the verify-before-write contract from PROJECT-
// DETAILS §4.20.
func (c *FileChunk) Validate() error {
	if c.ID == "" {
		return errors.New("chunk.id: must not be empty")
	}
	if err := validateFilePath(c.FileID); err != nil {
		return fmt.Errorf("chunk.file_id: %w", err)
	}
	if c.Total <= 0 {
		return fmt.Errorf("chunk.total: must be > 0, got %d", c.Total)
	}
	if c.Index < 0 || c.Index >= c.Total {
		return fmt.Errorf("chunk.index: %d out of range [0,%d)", c.Index, c.Total)
	}
	if c.Data == nil {
		return errors.New("chunk.data: must not be nil")
	}
	if len(c.Data) > ChunkSize {
		return fmt.Errorf("chunk.data: %d bytes exceeds ChunkSize %d", len(c.Data), ChunkSize)
	}
	if !hashHexRe.MatchString(c.Hash) {
		return fmt.Errorf("chunk.hash: must be lowercase hex SHA-256 (64 chars), got %q", c.Hash)
	}
	want := sha256.Sum256(c.Data)
	if hex.EncodeToString(want[:]) != c.Hash {
		return errors.New("chunk.hash: does not match SHA-256(data)")
	}
	return nil
}

// HashOf is a small helper for callers building a FileChunk — it
// returns the canonical hex SHA-256 of b. Returning a string keeps
// the call sites compact; the cost of one hex.EncodeToString per
// chunk is dwarfed by the I/O.
func HashOf(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// validateFilePath enforces the path invariants documented at the
// package level: non-empty; no leading slash (would shadow the
// cluster prefix); no ".." anywhere (path-traversal guard); no dots
// inside tokens (NATS wildcard collision); no whitespace.
func validateFilePath(p string) error {
	if p == "" {
		return errors.New("must not be empty")
	}
	if strings.HasPrefix(p, "/") {
		return fmt.Errorf("must not start with %q", "/")
	}
	if strings.HasSuffix(p, "/") {
		return fmt.Errorf("must not end with %q", "/")
	}
	if strings.Contains(p, "//") {
		return errors.New("must not contain empty segments")
	}
	for _, tok := range strings.Split(p, "/") {
		if tok == ".." {
			return errors.New("must not contain '..' segments")
		}
		if tok == "" {
			return errors.New("must not contain empty segments")
		}
		if strings.ContainsAny(tok, " \t\r\n") {
			return fmt.Errorf("must not contain whitespace in segment %q", tok)
		}
		if strings.ContainsAny(tok, ">*") {
			return fmt.Errorf("must not contain NATS wildcards in segment %q", tok)
		}
	}
	return nil
}
