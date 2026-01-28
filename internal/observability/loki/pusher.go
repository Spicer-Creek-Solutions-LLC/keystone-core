// Package loki provides a built-in Loki log pusher for Keystone.
package loki

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand" // nosemgrep: go.lang.security.audit.crypto.math_random.math-random-used -- jitter/sampling do not require crypto randomness
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/wait"
)

// Common errors.
var (
	ErrPusherStopped = errors.New("pusher is stopped")
	ErrBatchFull     = errors.New("batch is full")
	ErrPushFailed    = errors.New("push failed")
)

// LogLevel represents a log level.
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// Entry represents a log entry.
type Entry struct {
	Timestamp time.Time         `json:"timestamp"`
	Line      string            `json:"line"`
	Level     LogLevel          `json:"level,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// Stream represents a Loki log stream.
type Stream struct {
	Labels map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

// PushRequest represents a Loki push request.
type PushRequest struct {
	Streams []Stream `json:"streams"`
}

// Config configures the Loki pusher.
type Config struct {
	URL            string            `json:"url"`
	TenantID       string            `json:"tenantId,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
	BatchSize      int               `json:"batchSize"`
	BatchWait      time.Duration     `json:"batchWait"`
	Timeout        time.Duration     `json:"timeout"`
	RetryCount     int               `json:"retryCount"`
	RetryBaseDelay time.Duration     `json:"retryBaseDelay"`
	RetryMaxDelay  time.Duration     `json:"retryMaxDelay"`
	Compression    bool              `json:"compression"`
	Username       string            `json:"username,omitempty"`
	Password       string            `json:"password,omitempty"`
	BearerToken    string            `json:"bearerToken,omitempty"`
}

// DefaultConfig returns a default configuration.
func DefaultConfig() *Config {
	return &Config{
		URL:            "http://localhost:3100/loki/api/v1/push",
		BatchSize:      1000,
		BatchWait:      1 * time.Second,
		Timeout:        10 * time.Second,
		RetryCount:     3,
		RetryBaseDelay: 500 * time.Millisecond,
		RetryMaxDelay:  30 * time.Second,
		Compression:    true,
	}
}

// Pusher pushes logs to Loki.
type Pusher struct {
	config    *Config
	client    *http.Client
	entries   chan *Entry
	batch     []*Entry
	batchMu   sync.Mutex
	stopCh    chan struct{}
	doneCh    chan struct{}
	running   bool
	mu        sync.RWMutex
	listeners []PushEventListener
	stats     *Stats
}

// Stats contains pusher statistics.
type Stats struct {
	EntriesPushed     int64     `json:"entriesPushed"`
	EntriesDropped    int64     `json:"entriesDropped"`
	BytesPushed       int64     `json:"bytesPushed"`
	PushCount         int64     `json:"pushCount"`
	PushErrors        int64     `json:"pushErrors"`
	LastPushTime      time.Time `json:"lastPushTime"`
	LastPushError     string    `json:"lastPushError,omitempty"`
	LastPushErrorTime time.Time `json:"lastPushErrorTime,omitempty"`
	mu                sync.RWMutex
}

// PushEvent represents a push event.
type PushEvent struct {
	Type       string        `json:"type"`
	EntryCount int           `json:"entryCount"`
	ByteCount  int           `json:"byteCount"`
	Success    bool          `json:"success"`
	Error      string        `json:"error,omitempty"`
	Timestamp  time.Time     `json:"timestamp"`
	Duration   time.Duration `json:"duration"`
}

// PushEventListener is called when push events occur.
type PushEventListener func(*PushEvent)

// NewPusher creates a new Loki pusher.
func NewPusher(config *Config) *Pusher {
	if config.BatchSize <= 0 {
		config.BatchSize = 1000
	}
	if config.BatchWait <= 0 {
		config.BatchWait = time.Second
	}
	if config.Timeout <= 0 {
		config.Timeout = 10 * time.Second
	}
	if config.RetryCount <= 0 {
		config.RetryCount = 3
	}
	if config.RetryBaseDelay <= 0 {
		config.RetryBaseDelay = 500 * time.Millisecond
	}
	if config.RetryMaxDelay <= 0 {
		config.RetryMaxDelay = 30 * time.Second
	}

	return &Pusher{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
		entries: make(chan *Entry, config.BatchSize*2),
		batch:   make([]*Entry, 0, config.BatchSize),
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
		stats:   &Stats{},
	}
}

// Start starts the pusher.
func (p *Pusher) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return nil
	}

	p.running = true
	go p.run()

	return nil
}

// Stop stops the pusher.
func (p *Pusher) Stop(ctx context.Context) error {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return nil
	}
	p.running = false
	close(p.stopCh)
	p.mu.Unlock()

	// Wait for the pusher to finish or context to timeout
	select {
	case <-p.doneCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Push pushes a log entry.
func (p *Pusher) Push(entry *Entry) error {
	p.mu.RLock()
	if !p.running {
		p.mu.RUnlock()
		return ErrPusherStopped
	}
	p.mu.RUnlock()

	select {
	case p.entries <- entry:
		return nil
	default:
		p.stats.mu.Lock()
		p.stats.EntriesDropped++
		p.stats.mu.Unlock()
		return ErrBatchFull
	}
}

// PushLog pushes a simple log message.
func (p *Pusher) PushLog(level LogLevel, message string, labels map[string]string) error {
	return p.Push(&Entry{
		Timestamp: time.Now(),
		Line:      message,
		Level:     level,
		Labels:    labels,
	})
}

// AddListener adds an event listener.
func (p *Pusher) AddListener(listener PushEventListener) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.listeners = append(p.listeners, listener)
}

// Stats returns the pusher statistics.
func (p *Pusher) Stats() Stats {
	p.stats.mu.RLock()
	defer p.stats.mu.RUnlock()
	return *p.stats
}

func (p *Pusher) run() {
	defer close(p.doneCh)

	ticker := time.NewTicker(p.config.BatchWait)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			// Flush remaining entries
			p.flushBatch()
			return

		case entry := <-p.entries:
			p.addToBatch(entry)

			if len(p.batch) >= p.config.BatchSize {
				p.flushBatch()
			}

		case <-ticker.C:
			if len(p.batch) > 0 {
				p.flushBatch()
			}
		}
	}
}

func (p *Pusher) addToBatch(entry *Entry) {
	p.batchMu.Lock()
	defer p.batchMu.Unlock()
	p.batch = append(p.batch, entry)
}

func (p *Pusher) flushBatch() {
	p.batchMu.Lock()
	if len(p.batch) == 0 {
		p.batchMu.Unlock()
		return
	}

	entries := p.batch
	p.batch = make([]*Entry, 0, p.config.BatchSize)
	p.batchMu.Unlock()

	start := time.Now()
	err := p.pushEntries(entries)
	duration := time.Since(start)

	event := &PushEvent{
		Type:       "push",
		EntryCount: len(entries),
		Success:    err == nil,
		Timestamp:  time.Now(),
		Duration:   duration,
	}

	if err != nil {
		event.Error = err.Error()
		p.stats.mu.Lock()
		p.stats.PushErrors++
		p.stats.LastPushError = err.Error()
		p.stats.LastPushErrorTime = time.Now()
		p.stats.mu.Unlock()
	} else {
		p.stats.mu.Lock()
		p.stats.EntriesPushed += int64(len(entries))
		p.stats.PushCount++
		p.stats.LastPushTime = time.Now()
		p.stats.mu.Unlock()
	}

	p.emit(event)
}

func (p *Pusher) pushEntries(entries []*Entry) error {
	req := p.buildPushRequest(entries)

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	var body io.Reader = bytes.NewReader(data)
	contentType := "application/json"

	// Compress if enabled
	if p.config.Compression {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		if _, err := gz.Write(data); err != nil {
			return fmt.Errorf("compress: %w", err)
		}
		if err := gz.Close(); err != nil {
			return fmt.Errorf("compress close: %w", err)
		}
		body = &buf
		data = buf.Bytes()
	}

	// Retry with exponential backoff
	var lastErr error
	for attempt := 0; attempt <= p.config.RetryCount; attempt++ {
		if attempt > 0 {
			delay := p.calculateBackoff(attempt)
			wait.ForDuration(delay)
		}

		if err := p.doPush(body, data, contentType); err != nil {
			lastErr = err
			// Reset body for retry
			body = bytes.NewReader(data)
			continue
		}

		// Update stats
		p.stats.mu.Lock()
		p.stats.BytesPushed += int64(len(data))
		p.stats.mu.Unlock()

		return nil
	}

	return fmt.Errorf("%w: %v", ErrPushFailed, lastErr)
}

func (p *Pusher) doPush(body io.Reader, data []byte, contentType string) error {
	req, err := http.NewRequest("POST", p.config.URL, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", contentType)
	if p.config.Compression {
		req.Header.Set("Content-Encoding", "gzip")
	}
	if p.config.TenantID != "" {
		req.Header.Set("X-Scope-OrgID", p.config.TenantID)
	}

	// Add authentication
	if p.config.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.config.BearerToken)
	} else if p.config.Username != "" {
		req.SetBasicAuth(p.config.Username, p.config.Password)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (p *Pusher) buildPushRequest(entries []*Entry) *PushRequest {
	// Group entries by labels
	streamMap := make(map[string]*Stream)

	for _, entry := range entries {
		// Merge global and entry labels
		labels := make(map[string]string)
		for k, v := range p.config.Labels {
			labels[k] = v
		}
		for k, v := range entry.Labels {
			labels[k] = v
		}
		if entry.Level != "" {
			labels["level"] = string(entry.Level)
		}

		// Create a key from labels
		key := labelsKey(labels)

		stream, ok := streamMap[key]
		if !ok {
			stream = &Stream{
				Labels: labels,
				Values: make([][2]string, 0),
			}
			streamMap[key] = stream
		}

		// Loki expects nanoseconds as string
		ts := strconv.FormatInt(entry.Timestamp.UnixNano(), 10)
		stream.Values = append(stream.Values, [2]string{ts, entry.Line})
	}

	// Convert map to slice
	streams := make([]Stream, 0, len(streamMap))
	for _, stream := range streamMap {
		streams = append(streams, *stream)
	}

	return &PushRequest{Streams: streams}
}

func (p *Pusher) calculateBackoff(attempt int) time.Duration {
	// Exponential backoff with jitter
	delay := p.config.RetryBaseDelay * (1 << uint(attempt))
	if delay > p.config.RetryMaxDelay {
		delay = p.config.RetryMaxDelay
	}

	// Add jitter (0-25%)
	jitter := time.Duration(rand.Int63n(int64(delay / 4))) // nosemgrep: go.lang.security.audit.crypto.math_random.math-random-used -- jitter does not require crypto randomness
	return delay + jitter
}

func (p *Pusher) emit(event *PushEvent) {
	p.mu.RLock()
	listeners := p.listeners
	p.mu.RUnlock()

	for _, listener := range listeners {
		listener(event)
	}
}

func labelsKey(labels map[string]string) string {
	// Sort keys for consistent ordering
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(labels[k])
	}
	return b.String()
}

// BatchPusher wraps Pusher with batch-aware features.
type BatchPusher struct {
	pusher *Pusher
	buffer []*Entry
	bufMu  sync.Mutex
}

// NewBatchPusher creates a new batch pusher.
func NewBatchPusher(config *Config) *BatchPusher {
	return &BatchPusher{
		pusher: NewPusher(config),
		buffer: make([]*Entry, 0, config.BatchSize),
	}
}

// Start starts the batch pusher.
func (bp *BatchPusher) Start() error {
	return bp.pusher.Start()
}

// Stop stops the batch pusher.
func (bp *BatchPusher) Stop(ctx context.Context) error {
	bp.Flush()
	return bp.pusher.Stop(ctx)
}

// Push adds an entry to the buffer.
func (bp *BatchPusher) Push(entry *Entry) error {
	bp.bufMu.Lock()
	bp.buffer = append(bp.buffer, entry)
	shouldFlush := len(bp.buffer) >= bp.pusher.config.BatchSize
	bp.bufMu.Unlock()

	if shouldFlush {
		bp.Flush()
	}
	return nil
}

// Flush flushes the buffer.
func (bp *BatchPusher) Flush() {
	bp.bufMu.Lock()
	entries := bp.buffer
	bp.buffer = make([]*Entry, 0, bp.pusher.config.BatchSize)
	bp.bufMu.Unlock()

	for _, entry := range entries {
		bp.pusher.Push(entry)
	}
}

// AddListener adds an event listener.
func (bp *BatchPusher) AddListener(listener PushEventListener) {
	bp.pusher.AddListener(listener)
}

// Stats returns statistics.
func (bp *BatchPusher) Stats() Stats {
	return bp.pusher.Stats()
}

// LogWriter implements io.Writer for log output.
type LogWriter struct {
	pusher *Pusher
	level  LogLevel
	labels map[string]string
}

// NewLogWriter creates a new log writer.
func NewLogWriter(pusher *Pusher, level LogLevel, labels map[string]string) *LogWriter {
	return &LogWriter{
		pusher: pusher,
		level:  level,
		labels: labels,
	}
}

// Write writes log data.
func (w *LogWriter) Write(p []byte) (n int, err error) {
	line := strings.TrimSpace(string(p))
	if line == "" {
		return len(p), nil
	}

	err = w.pusher.Push(&Entry{
		Timestamp: time.Now(),
		Line:      line,
		Level:     w.level,
		Labels:    w.labels,
	})
	if err != nil {
		return 0, err
	}

	return len(p), nil
}

// MultiTenantPusher manages pushers for multiple tenants.
type MultiTenantPusher struct {
	baseConfig *Config
	pushers    map[string]*Pusher
	mu         sync.RWMutex
}

// NewMultiTenantPusher creates a new multi-tenant pusher.
func NewMultiTenantPusher(baseConfig *Config) *MultiTenantPusher {
	return &MultiTenantPusher{
		baseConfig: baseConfig,
		pushers:    make(map[string]*Pusher),
	}
}

// GetPusher returns a pusher for a tenant.
func (mp *MultiTenantPusher) GetPusher(tenantID string) *Pusher {
	mp.mu.RLock()
	pusher, ok := mp.pushers[tenantID]
	mp.mu.RUnlock()

	if ok {
		return pusher
	}

	mp.mu.Lock()
	defer mp.mu.Unlock()

	// Double check
	if pusher, ok := mp.pushers[tenantID]; ok {
		return pusher
	}

	// Create new pusher for tenant
	config := *mp.baseConfig
	config.TenantID = tenantID
	pusher = NewPusher(&config)
	pusher.Start()
	mp.pushers[tenantID] = pusher

	return pusher
}

// Push pushes an entry for a tenant.
func (mp *MultiTenantPusher) Push(tenantID string, entry *Entry) error {
	return mp.GetPusher(tenantID).Push(entry)
}

// Stop stops all pushers.
func (mp *MultiTenantPusher) Stop(ctx context.Context) error {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	var errs []error
	for _, pusher := range mp.pushers {
		if err := pusher.Stop(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors stopping pushers: %v", errs)
	}
	return nil
}

// FilteredPusher wraps a pusher with filtering.
type FilteredPusher struct {
	pusher *Pusher
	filter func(*Entry) bool
}

// NewFilteredPusher creates a new filtered pusher.
func NewFilteredPusher(pusher *Pusher, filter func(*Entry) bool) *FilteredPusher {
	return &FilteredPusher{
		pusher: pusher,
		filter: filter,
	}
}

// Push pushes an entry if it passes the filter.
func (fp *FilteredPusher) Push(entry *Entry) error {
	if fp.filter != nil && !fp.filter(entry) {
		return nil
	}
	return fp.pusher.Push(entry)
}

// LevelFilter returns a filter for minimum log level.
func LevelFilter(minLevel LogLevel) func(*Entry) bool {
	levels := map[LogLevel]int{
		LogLevelDebug: 0,
		LogLevelInfo:  1,
		LogLevelWarn:  2,
		LogLevelError: 3,
	}

	minLevelInt := levels[minLevel]

	return func(entry *Entry) bool {
		entryLevel, ok := levels[entry.Level]
		if !ok {
			return true // Allow unknown levels
		}
		return entryLevel >= minLevelInt
	}
}

// SamplingPusher wraps a pusher with sampling.
type SamplingPusher struct {
	pusher     *Pusher
	sampleRate float64
}

// NewSamplingPusher creates a new sampling pusher.
func NewSamplingPusher(pusher *Pusher, sampleRate float64) *SamplingPusher {
	if sampleRate <= 0 {
		sampleRate = 1.0
	} else if sampleRate > 1.0 {
		sampleRate = 1.0
	}

	return &SamplingPusher{
		pusher:     pusher,
		sampleRate: sampleRate,
	}
}

// Push pushes an entry based on sample rate.
func (sp *SamplingPusher) Push(entry *Entry) error {
	if rand.Float64() > sp.sampleRate { // nosemgrep: go.lang.security.audit.crypto.math_random.math-random-used -- sampling does not require crypto randomness
		return nil
	}
	return sp.pusher.Push(entry)
}
