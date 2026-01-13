package files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"

	"github.com/shawnbutts/keystone-core/pkg/files/backend"
)

// Server handles file distribution over NATS.
type Server struct {
	config     *ServerConfig
	nc         *nats.Conn
	backends   []backend.Backend
	subs       []*nats.Subscription
	workerPool chan struct{}
	metrics    *ServerMetrics
	mu         sync.RWMutex
	running    bool
	ctx        context.Context
	cancel     context.CancelFunc
}

// ServerConfig configures the file server.
type ServerConfig struct {
	// ClusterID identifies the cluster for NATS subjects.
	ClusterID string `yaml:"cluster_id"`

	// InstanceID identifies this server instance.
	InstanceID string `yaml:"instance_id"`

	// Workers is the number of concurrent transfer handlers.
	Workers int `yaml:"workers"`

	// MaxChunkSize is the maximum chunk size in bytes.
	MaxChunkSize int `yaml:"max_chunk_size"`

	// MaxFileSize is the maximum file size in bytes.
	MaxFileSize int64 `yaml:"max_file_size"`

	// RequestTimeout is the timeout for processing requests.
	RequestTimeout time.Duration `yaml:"request_timeout"`

	// RateLimit configures rate limiting.
	RateLimit *RateLimitConfig `yaml:"rate_limit,omitempty"`
}

// RateLimitConfig configures rate limiting.
type RateLimitConfig struct {
	// PerAgent limit (bytes per second per agent).
	PerAgent int64 `yaml:"per_agent"`

	// Global limit (bytes per second total).
	Global int64 `yaml:"global"`

	// ConcurrentTransfers maximum.
	ConcurrentTransfers int `yaml:"concurrent_transfers"`
}

// ServerMetrics tracks server metrics.
type ServerMetrics struct {
	RequestsTotal      atomic.Int64
	RequestsSucceeded  atomic.Int64
	RequestsFailed     atomic.Int64
	BytesTransferred   atomic.Int64
	ActiveTransfers    atomic.Int32
	ChunksTransferred  atomic.Int64
}

// NewServer creates a new file server.
func NewServer(config *ServerConfig) (*Server, error) {
	if config.ClusterID == "" {
		return nil, fmt.Errorf("cluster_id is required")
	}
	if config.InstanceID == "" {
		config.InstanceID = uuid.New().String()[:8]
	}
	if config.Workers <= 0 {
		config.Workers = 10
	}
	if config.MaxChunkSize <= 0 {
		config.MaxChunkSize = DefaultChunkSize
	}
	if config.MaxChunkSize > MaxChunkSize {
		config.MaxChunkSize = MaxChunkSize
	}
	if config.MaxFileSize <= 0 {
		config.MaxFileSize = DefaultMaxFileSize
	}
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = 5 * time.Minute
	}

	return &Server{
		config:     config,
		workerPool: make(chan struct{}, config.Workers),
		metrics:    &ServerMetrics{},
	}, nil
}

// AddBackend adds a storage backend.
func (s *Server) AddBackend(b backend.Backend) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backends = append(s.backends, b)
}

// Start starts the file server with the given NATS connection.
func (s *Server) Start(nc *nats.Conn) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("server already running")
	}

	if len(s.backends) == 0 {
		return fmt.Errorf("no backends configured")
	}

	s.nc = nc
	s.ctx, s.cancel = context.WithCancel(context.Background())

	// Subscribe to file request subjects
	// Use queue group for load balancing across instances
	queueGroup := "kscore-files"

	// Subscribe to all namespaces
	subject := fmt.Sprintf(SubjectFileRequest, s.config.ClusterID, "*")
	sub, err := nc.QueueSubscribe(subject, queueGroup, s.handleFileRequest)
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", subject, err)
	}
	s.subs = append(s.subs, sub)

	// Subscribe to metadata requests
	subject = fmt.Sprintf(SubjectFileMetadata, s.config.ClusterID, "*")
	sub, err = nc.QueueSubscribe(subject, queueGroup, s.handleMetadataRequest)
	if err != nil {
		s.cleanup()
		return fmt.Errorf("failed to subscribe to %s: %w", subject, err)
	}
	s.subs = append(s.subs, sub)

	s.running = true
	log.Printf("[files] Server started: instance=%s workers=%d", s.config.InstanceID, s.config.Workers)

	return nil
}

// Stop stops the file server.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.cancel()
	s.cleanup()
	s.running = false

	log.Printf("[files] Server stopped: instance=%s", s.config.InstanceID)
	return nil
}

// cleanup unsubscribes from all subjects.
func (s *Server) cleanup() {
	for _, sub := range s.subs {
		sub.Unsubscribe()
	}
	s.subs = nil
}

// handleFileRequest handles incoming file requests.
func (s *Server) handleFileRequest(msg *nats.Msg) {
	s.metrics.RequestsTotal.Add(1)

	// Acquire worker slot
	select {
	case s.workerPool <- struct{}{}:
		defer func() { <-s.workerPool }()
	default:
		// All workers busy, respond with retry
		s.respondError(msg, "", ErrCodeTimeout, "server busy, retry later")
		s.metrics.RequestsFailed.Add(1)
		return
	}

	// Parse request
	var req FileRequest
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		s.respondError(msg, "", ErrCodeInvalidRequest, "invalid request: "+err.Error())
		s.metrics.RequestsFailed.Add(1)
		return
	}

	if req.RequestID == "" {
		req.RequestID = uuid.New().String()
	}

	// Validate path
	if err := ValidatePath(req.Path); err != nil {
		s.respondError(msg, req.RequestID, ErrCodeInvalidRequest, err.Error())
		s.metrics.RequestsFailed.Add(1)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(s.ctx, s.config.RequestTimeout)
	defer cancel()

	// Find backend for path
	b := s.findBackend(req.Path)
	if b == nil {
		s.respondError(msg, req.RequestID, ErrCodeNotFound, "no backend for path")
		s.metrics.RequestsFailed.Add(1)
		return
	}

	// Get file
	opts := &backend.GetOptions{
		Version:     req.Version,
		IfNoneMatch: req.Checksum,
	}
	if req.Range != nil {
		opts.Range = &backend.ByteRange{
			Start: req.Range.Start,
			End:   req.Range.End,
		}
	}

	result, err := b.Get(ctx, req.Path, opts)
	if err != nil {
		if backend.IsNotFound(err) {
			s.respondError(msg, req.RequestID, ErrCodeNotFound, "file not found")
		} else {
			s.respondError(msg, req.RequestID, ErrCodeBackendError, err.Error())
		}
		s.metrics.RequestsFailed.Add(1)
		return
	}
	defer result.Reader.Close()

	// Handle conditional GET (not modified)
	if result.NotModified {
		metadata := FileMetadata{
			RequestID:   req.RequestID,
			Path:        req.Path,
			NotModified: true,
			Checksum:    result.Info.Checksum,
		}
		s.respond(msg, metadata)
		s.metrics.RequestsSucceeded.Add(1)
		return
	}

	// Check file size
	if result.Info.Size > s.config.MaxFileSize {
		s.respondError(msg, req.RequestID, ErrCodeFileTooLarge, fmt.Sprintf("file size %d exceeds max %d", result.Info.Size, s.config.MaxFileSize))
		s.metrics.RequestsFailed.Add(1)
		return
	}

	// Determine chunk size
	chunkSize := s.config.MaxChunkSize
	if req.ChunkSize > 0 && req.ChunkSize < chunkSize {
		chunkSize = req.ChunkSize
	}

	// Calculate chunk count
	chunkCount := int((result.Info.Size + int64(chunkSize) - 1) / int64(chunkSize))
	if chunkCount == 0 {
		chunkCount = 1 // Empty files still get one chunk
	}

	// Send metadata first
	metadata := FileMetadata{
		RequestID:    req.RequestID,
		Path:         req.Path,
		Version:      result.Info.Version,
		Size:         result.Info.Size,
		Checksum:     result.Info.Checksum,
		ContentType:  result.Info.ContentType,
		ModifiedTime: result.Info.ModifiedTime,
		ChunkCount:   chunkCount,
		ChunkSize:    chunkSize,
		Tags:         result.Info.Tags,
	}
	s.respond(msg, metadata)

	// Track active transfer
	s.metrics.ActiveTransfers.Add(1)
	defer s.metrics.ActiveTransfers.Add(-1)

	// Stream chunks
	buf := make([]byte, chunkSize)
	for i := 0; i < chunkCount; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Read chunk
		n, err := io.ReadFull(result.Reader, buf)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			log.Printf("[files] Read error for %s chunk %d: %v", req.Path, i, err)
			return
		}

		// Calculate chunk checksum
		hash := sha256.Sum256(buf[:n])
		chunkChecksum := hex.EncodeToString(hash[:])

		// Send chunk
		chunk := FileChunk{
			RequestID:  req.RequestID,
			Index:      i,
			TotalCount: chunkCount,
			Data:       buf[:n],
			Checksum:   chunkChecksum,
			Final:      i == chunkCount-1,
		}

		chunkData, _ := json.Marshal(chunk)
		if err := s.nc.Publish(msg.Reply, chunkData); err != nil {
			log.Printf("[files] Failed to send chunk %d for %s: %v", i, req.Path, err)
			return
		}

		s.metrics.BytesTransferred.Add(int64(n))
		s.metrics.ChunksTransferred.Add(1)
	}

	s.metrics.RequestsSucceeded.Add(1)
}

// handleMetadataRequest handles metadata-only requests.
func (s *Server) handleMetadataRequest(msg *nats.Msg) {
	s.metrics.RequestsTotal.Add(1)

	var req FileRequest
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		s.respondError(msg, "", ErrCodeInvalidRequest, "invalid request: "+err.Error())
		s.metrics.RequestsFailed.Add(1)
		return
	}

	if req.RequestID == "" {
		req.RequestID = uuid.New().String()
	}

	if err := ValidatePath(req.Path); err != nil {
		s.respondError(msg, req.RequestID, ErrCodeInvalidRequest, err.Error())
		s.metrics.RequestsFailed.Add(1)
		return
	}

	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	b := s.findBackend(req.Path)
	if b == nil {
		s.respondError(msg, req.RequestID, ErrCodeNotFound, "no backend for path")
		s.metrics.RequestsFailed.Add(1)
		return
	}

	info, err := b.Stat(ctx, req.Path)
	if err != nil {
		if backend.IsNotFound(err) {
			s.respondError(msg, req.RequestID, ErrCodeNotFound, "file not found")
		} else {
			s.respondError(msg, req.RequestID, ErrCodeBackendError, err.Error())
		}
		s.metrics.RequestsFailed.Add(1)
		return
	}

	chunkSize := s.config.MaxChunkSize
	chunkCount := int((info.Size + int64(chunkSize) - 1) / int64(chunkSize))
	if chunkCount == 0 {
		chunkCount = 1
	}

	metadata := FileMetadata{
		RequestID:    req.RequestID,
		Path:         req.Path,
		Version:      info.Version,
		Size:         info.Size,
		Checksum:     info.Checksum,
		ContentType:  info.ContentType,
		ModifiedTime: info.ModifiedTime,
		ChunkCount:   chunkCount,
		ChunkSize:    chunkSize,
		Tags:         info.Tags,
	}

	s.respond(msg, metadata)
	s.metrics.RequestsSucceeded.Add(1)
}

// findBackend finds the appropriate backend for a path.
// It uses path matching from backend config and returns the highest priority
// (lowest priority number) healthy backend that matches the path.
func (s *Server) findBackend(path string) backend.Backend {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Find backends that match the path, sorted by priority
	type match struct {
		backend  backend.Backend
		priority int
	}
	var matches []match

	for _, b := range s.backends {
		cfg := b.BaseConfig()

		// Check if backend matches this path
		// If no paths configured, backend matches all paths (fallback)
		pathMatches := len(cfg.Paths) == 0 || cfg.MatchesPath(path)

		if pathMatches {
			// Only include healthy backends
			if err := b.Health(context.Background()); err == nil {
				matches = append(matches, match{backend: b, priority: cfg.Priority})
			}
		}
	}

	if len(matches) == 0 {
		return nil
	}

	// Sort by priority (lower number = higher priority)
	for i := 0; i < len(matches)-1; i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[j].priority < matches[i].priority {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}

	return matches[0].backend
}

// respond sends a JSON response.
func (s *Server) respond(msg *nats.Msg, data interface{}) {
	if msg.Reply == "" {
		return
	}
	response, _ := json.Marshal(data)
	s.nc.Publish(msg.Reply, response)
}

// respondError sends an error response.
func (s *Server) respondError(msg *nats.Msg, requestID string, code FileErrorCode, message string) {
	if msg.Reply == "" {
		return
	}
	errResp := FileError{
		RequestID: requestID,
		Code:      code,
		Message:   message,
	}
	response, _ := json.Marshal(errResp)
	s.nc.Publish(msg.Reply, response)
}

// Metrics returns current server metrics.
func (s *Server) Metrics() ServerMetrics {
	return ServerMetrics{
		RequestsTotal:     atomic.Int64{},
		RequestsSucceeded: atomic.Int64{},
		RequestsFailed:    atomic.Int64{},
		BytesTransferred:  atomic.Int64{},
		ActiveTransfers:   atomic.Int32{},
		ChunksTransferred: atomic.Int64{},
	}
}

// GetMetrics returns a copy of the current metrics.
func (s *Server) GetMetrics() *ServerMetrics {
	m := &ServerMetrics{}
	m.RequestsTotal.Store(s.metrics.RequestsTotal.Load())
	m.RequestsSucceeded.Store(s.metrics.RequestsSucceeded.Load())
	m.RequestsFailed.Store(s.metrics.RequestsFailed.Load())
	m.BytesTransferred.Store(s.metrics.BytesTransferred.Load())
	m.ActiveTransfers.Store(s.metrics.ActiveTransfers.Load())
	m.ChunksTransferred.Store(s.metrics.ChunksTransferred.Load())
	return m
}
