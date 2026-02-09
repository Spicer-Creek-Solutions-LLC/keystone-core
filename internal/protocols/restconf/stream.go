package restconf

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/shawnbutts/keystone-core/internal/protocols/rest"
)

// StreamEvent represents a Server-Sent Event received from a RESTCONF stream.
type StreamEvent struct {
	ID    string
	Event string
	Data  []byte
}

// StreamSubscription represents an active SSE event stream subscription.
type StreamSubscription struct {
	stream string
	events chan StreamEvent
	errs   chan error
	cancel context.CancelFunc
}

// Events returns the channel for receiving stream events.
func (s *StreamSubscription) Events() <-chan StreamEvent {
	return s.events
}

// Errors returns the channel for receiving stream errors.
func (s *StreamSubscription) Errors() <-chan error {
	return s.errs
}

// Close terminates the subscription.
func (s *StreamSubscription) Close() {
	s.cancel()
}

// Subscribe opens an SSE connection to a RESTCONF notification stream (RFC 8040 Section 6).
func (a *Adapter) Subscribe(ctx context.Context, stream string) (*StreamSubscription, error) {
	a.mu.RLock()
	if !a.connected {
		a.mu.RUnlock()
		return nil, fmt.Errorf("not connected")
	}
	rootPath := a.rootPath
	client := a.client
	auth := a.auth
	a.mu.RUnlock()

	streamCtx, cancel := context.WithCancel(ctx)

	url := rootPath + "/streams/" + strings.TrimPrefix(stream, "/")
	req, err := client.NewRequest(streamCtx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create stream request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	if auth != nil {
		if err := auth.Authenticate(req); err != nil {
			cancel()
			return nil, fmt.Errorf("authenticate stream: %w", err)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open stream: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		r, _ := rest.NewResponse(resp)
		cancel()
		if r != nil {
			return nil, fmt.Errorf("stream error: HTTP %d: %s", resp.StatusCode, r.Body())
		}
		return nil, fmt.Errorf("stream error: HTTP %d", resp.StatusCode)
	}

	sub := &StreamSubscription{
		stream: stream,
		events: make(chan StreamEvent, 64),
		errs:   make(chan error, 1),
		cancel: cancel,
	}

	go readSSE(streamCtx, resp, sub)

	return sub, nil
}

// readSSE reads Server-Sent Events from an HTTP response body.
func readSSE(ctx context.Context, resp *http.Response, sub *StreamSubscription) {
	defer resp.Body.Close()
	defer close(sub.events)
	defer close(sub.errs)

	scanner := bufio.NewScanner(resp.Body)
	var event StreamEvent
	var dataLines []string

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Text()

		if line == "" {
			// Empty line = event boundary
			if len(dataLines) > 0 {
				event.Data = []byte(strings.Join(dataLines, "\n"))
				select {
				case sub.events <- event:
				case <-ctx.Done():
					return
				}
				event = StreamEvent{}
				dataLines = nil
			}
			continue
		}

		if strings.HasPrefix(line, ":") {
			// Comment, skip
			continue
		}

		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")

		switch field {
		case "id":
			event.ID = value
		case "event":
			event.Event = value
		case "data":
			dataLines = append(dataLines, value)
		}
	}

	if err := scanner.Err(); err != nil {
		select {
		case sub.errs <- err:
		case <-ctx.Done():
		}
	}
}
