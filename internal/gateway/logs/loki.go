package logs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/wait"
)

// LokiConfig holds configuration for the Loki pusher.
type LokiConfig struct {
	// URL is the Loki push endpoint (e.g., http://loki:3100/loki/api/v1/push)
	URL string

	// TenantID for multi-tenant Loki
	TenantID string

	// BatchSize is the number of entries per batch
	BatchSize int

	// BatchWait is how long to wait before flushing a batch
	BatchWait time.Duration

	// Labels to include from log entries
	Labels []string

	// Timeout for push requests
	Timeout time.Duration

	// MaxRetries is the maximum number of retries
	MaxRetries int

	// RetryBackoff is the initial backoff duration
	RetryBackoff time.Duration

	// BasicAuth credentials
	BasicAuth *BasicAuth

	// BearerToken for authentication
	BearerToken string
}

// BasicAuth holds basic authentication credentials.
type BasicAuth struct {
	Username string
	Password string
}

// DefaultLokiConfig returns a configuration with sensible defaults.
func DefaultLokiConfig() LokiConfig {
	return LokiConfig{
		BatchSize:    100,
		BatchWait:    1 * time.Second,
		Labels:       []string{"agent_id", "level", "source"},
		Timeout:      30 * time.Second,
		MaxRetries:   3,
		RetryBackoff: 1 * time.Second,
	}
}

// LokiPusher pushes logs to Loki.
type LokiPusher struct {
	config LokiConfig
	client *http.Client
	store  *LogsStore

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Track last pushed entry
	mu     sync.Mutex
	lastID string

	// Stats
	statsMu       sync.RWMutex
	entriesSent   int64
	entriesFailed int64
	batchesSent   int64
	batchesFailed int64
	lastError     error
	lastErrorTime time.Time
}

// NewLokiPusher creates a new Loki pusher.
func NewLokiPusher(store *LogsStore, config LokiConfig) *LokiPusher {
	ctx, cancel := context.WithCancel(context.Background())
	return &LokiPusher{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
		store:  store,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start starts the Loki pusher.
func (p *LokiPusher) Start() error {
	if p.config.URL == "" {
		return fmt.Errorf("Loki URL is required")
	}

	p.wg.Add(1)
	go p.pushLoop()

	log.Printf("Loki pusher started for %s", p.config.URL)
	return nil
}

// Stop stops the Loki pusher.
func (p *LokiPusher) Stop() error {
	p.cancel()

	// Flush remaining entries
	p.flush()

	p.wg.Wait()
	return nil
}

// pushLoop periodically pushes logs to Loki.
func (p *LokiPusher) pushLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.config.BatchWait)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.flush()
		}
	}
}

// flush pushes pending entries to Loki.
func (p *LokiPusher) flush() {
	p.mu.Lock()
	lastID := p.lastID
	p.mu.Unlock()

	entries := p.store.GetPending(lastID, p.config.BatchSize)
	if len(entries) == 0 {
		return
	}

	// Convert to Loki format
	streams := p.entriesToStreams(entries)
	if len(streams) == 0 {
		return
	}

	// Push to Loki
	if err := p.push(streams); err != nil {
		p.recordError(err)
		p.recordFailure(len(entries))
		return
	}

	// Update last ID
	p.mu.Lock()
	p.lastID = entries[len(entries)-1].ID
	p.mu.Unlock()

	p.recordSuccess(len(entries))
}

// lokiPushRequest is the Loki push API request format.
type lokiPushRequest struct {
	Streams []lokiStream `json:"streams"`
}

// lokiStream is a stream of log entries for Loki.
type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

// entriesToStreams converts log entries to Loki streams.
func (p *LokiPusher) entriesToStreams(entries []LogEntry) []lokiStream {
	// Group entries by label set
	streamMap := make(map[string]*lokiStream)

	for _, entry := range entries {
		// Build label set
		labels := make(map[string]string)
		for _, labelKey := range p.config.Labels {
			switch labelKey {
			case "agent_id":
				labels["agent_id"] = entry.AgentID
			case "level":
				labels["level"] = entry.Level.String()
			case "source":
				if entry.Source != "" {
					labels["source"] = entry.Source
				}
			default:
				if val, ok := entry.Labels[labelKey]; ok {
					labels[labelKey] = val
				}
			}
		}

		// Create stream key
		streamKey := labelsToKey(labels)

		// Get or create stream
		stream, exists := streamMap[streamKey]
		if !exists {
			stream = &lokiStream{
				Stream: labels,
				Values: make([][]string, 0),
			}
			streamMap[streamKey] = stream
		}

		// Add entry to stream
		// Loki format: [timestamp_ns, message]
		ts := strconv.FormatInt(entry.Timestamp.UnixNano(), 10)

		// Build message with fields
		message := entry.Message
		if len(entry.Fields) > 0 {
			fieldsJSON, err := json.Marshal(entry.Fields)
			if err == nil {
				message = message + " " + string(fieldsJSON)
			}
		}

		stream.Values = append(stream.Values, []string{ts, message})
	}

	// Convert map to slice
	streams := make([]lokiStream, 0, len(streamMap))
	for _, stream := range streamMap {
		streams = append(streams, *stream)
	}

	return streams
}

// labelsToKey creates a consistent key from labels.
func labelsToKey(labels map[string]string) string {
	// Sort keys for consistent ordering
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	for i, k := range keys {
		if i > 0 {
			buf.WriteString(",")
		}
		buf.WriteString(k)
		buf.WriteString("=")
		buf.WriteString(labels[k])
	}
	return buf.String()
}

// push sends streams to Loki.
func (p *LokiPusher) push(streams []lokiStream) error {
	request := lokiPushRequest{Streams: streams}

	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Retry loop
	backoff := p.config.RetryBackoff
	for attempt := 0; attempt <= p.config.MaxRetries; attempt++ {
		if attempt > 0 {
			if err := wait.ForContext(p.ctx, backoff); err != nil {
				return err
			}
			backoff *= 2
		}

		err = p.doPush(data)
		if err == nil {
			return nil
		}
	}

	return fmt.Errorf("failed after %d attempts: %w", p.config.MaxRetries+1, err)
}

// doPush performs the HTTP request to Loki.
func (p *LokiPusher) doPush(data []byte) error {
	req, err := http.NewRequestWithContext(p.ctx, http.MethodPost, p.config.URL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Add tenant ID if configured
	if p.config.TenantID != "" {
		req.Header.Set("X-Scope-OrgID", p.config.TenantID)
	}

	// Add authentication
	if p.config.BasicAuth != nil {
		req.SetBasicAuth(p.config.BasicAuth.Username, p.config.BasicAuth.Password)
	} else if p.config.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.config.BearerToken)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("Loki push failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// recordSuccess records a successful batch push.
func (p *LokiPusher) recordSuccess(count int) {
	p.statsMu.Lock()
	defer p.statsMu.Unlock()
	p.entriesSent += int64(count)
	p.batchesSent++
}

// recordFailure records a failed batch push.
func (p *LokiPusher) recordFailure(count int) {
	p.statsMu.Lock()
	defer p.statsMu.Unlock()
	p.entriesFailed += int64(count)
	p.batchesFailed++
}

// recordError records an error.
func (p *LokiPusher) recordError(err error) {
	p.statsMu.Lock()
	defer p.statsMu.Unlock()
	p.lastError = err
	p.lastErrorTime = time.Now()
	log.Printf("Loki push error: %v", err)
}

// LokiPusherStats holds pusher statistics.
type LokiPusherStats struct {
	EntriesSent   int64
	EntriesFailed int64
	BatchesSent   int64
	BatchesFailed int64
	LastError     error
	LastErrorTime time.Time
}

// Stats returns pusher statistics.
func (p *LokiPusher) Stats() LokiPusherStats {
	p.statsMu.RLock()
	defer p.statsMu.RUnlock()
	return LokiPusherStats{
		EntriesSent:   p.entriesSent,
		EntriesFailed: p.entriesFailed,
		BatchesSent:   p.batchesSent,
		BatchesFailed: p.batchesFailed,
		LastError:     p.lastError,
		LastErrorTime: p.lastErrorTime,
	}
}
