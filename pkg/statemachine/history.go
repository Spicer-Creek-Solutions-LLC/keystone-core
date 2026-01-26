package statemachine

import (
	"sync"
	"time"
)

// HistoryRecord represents a single state transition in the history.
type HistoryRecord[S, E comparable] struct {
	// Timestamp when the transition occurred.
	Timestamp time.Time `json:"timestamp"`

	// From is the state before the transition.
	From S `json:"from"`

	// To is the state after the transition.
	To S `json:"to"`

	// Event that triggered the transition.
	Event E `json:"event"`

	// Duration is how long the machine was in the From state.
	Duration time.Duration `json:"duration"`

	// Metadata contains optional additional context.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// History tracks state transition history with a configurable maximum size.
// It operates as a circular buffer, dropping the oldest entries when full.
type History[S, E comparable] struct {
	mu       sync.RWMutex
	records  []HistoryRecord[S, E]
	maxSize  int
	position int // Current write position in circular buffer
	count    int // Total records written (used to calculate actual count)
}

// NewHistory creates a new History with the specified maximum size.
// If maxSize is 0 or negative, history tracking is disabled.
func NewHistory[S, E comparable](maxSize int) *History[S, E] {
	if maxSize <= 0 {
		return nil
	}
	return &History[S, E]{
		records: make([]HistoryRecord[S, E], maxSize),
		maxSize: maxSize,
	}
}

// Record adds a new transition to the history.
func (h *History[S, E]) Record(from, to S, event E, duration time.Duration, metadata map[string]any) {
	if h == nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	var metadataCopy map[string]any
	if metadata != nil {
		metadataCopy = make(map[string]any, len(metadata))
		for key, value := range metadata {
			metadataCopy[key] = value
		}
	}

	h.records[h.position] = HistoryRecord[S, E]{
		Timestamp: time.Now(),
		From:      from,
		To:        to,
		Event:     event,
		Duration:  duration,
		Metadata:  metadataCopy,
	}

	h.position = (h.position + 1) % h.maxSize
	h.count++
}

// All returns all history records in chronological order (oldest first).
func (h *History[S, E]) All() []HistoryRecord[S, E] {
	if h == nil {
		return nil
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	actualCount := h.actualCount()
	if actualCount == 0 {
		return nil
	}

	result := make([]HistoryRecord[S, E], actualCount)

	if h.count < h.maxSize {
		// Buffer not yet full, records are in order from 0
		copy(result, h.records[:actualCount])
	} else {
		// Buffer is full, oldest is at position, wrap around
		firstPart := h.records[h.position:]
		secondPart := h.records[:h.position]
		copy(result, firstPart)
		copy(result[len(firstPart):], secondPart)
	}

	return result
}

// Last returns the most recent n records in chronological order (oldest first).
func (h *History[S, E]) Last(n int) []HistoryRecord[S, E] {
	if h == nil || n <= 0 {
		return nil
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	actualCount := h.actualCount()
	if actualCount == 0 {
		return nil
	}

	if n > actualCount {
		n = actualCount
	}

	result := make([]HistoryRecord[S, E], n)

	// Calculate where to start reading
	if h.count < h.maxSize {
		// Buffer not yet full
		start := actualCount - n
		copy(result, h.records[start:actualCount])
	} else {
		// Buffer is full, need to handle wrap-around
		// Most recent is at (position - 1 + maxSize) % maxSize
		// Start reading from (position - n + maxSize) % maxSize
		start := (h.position - n + h.maxSize) % h.maxSize

		if start < h.position {
			// No wrap needed
			copy(result, h.records[start:h.position])
		} else {
			// Wrap around
			firstPart := h.records[start:]
			secondPart := h.records[:h.position]
			copy(result, firstPart)
			copy(result[len(firstPart):], secondPart)
		}
	}

	return result
}

// Latest returns the most recent history record, or nil if history is empty.
func (h *History[S, E]) Latest() *HistoryRecord[S, E] {
	if h == nil {
		return nil
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.count == 0 {
		return nil
	}

	idx := (h.position - 1 + h.maxSize) % h.maxSize
	record := h.records[idx]
	return &record
}

// Count returns the total number of transitions recorded.
// This may exceed the history size if old records have been dropped.
func (h *History[S, E]) Count() int {
	if h == nil {
		return 0
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.count
}

// Size returns the number of records currently in history.
func (h *History[S, E]) Size() int {
	if h == nil {
		return 0
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.actualCount()
}

// MaxSize returns the maximum history size.
func (h *History[S, E]) MaxSize() int {
	if h == nil {
		return 0
	}
	return h.maxSize
}

// Clear removes all history records.
func (h *History[S, E]) Clear() {
	if h == nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	h.records = make([]HistoryRecord[S, E], h.maxSize)
	h.position = 0
	h.count = 0
}

// actualCount returns the actual number of records in the buffer.
// Must be called with lock held.
func (h *History[S, E]) actualCount() int {
	if h.count < h.maxSize {
		return h.count
	}
	return h.maxSize
}

// Filter returns records matching the given predicate.
func (h *History[S, E]) Filter(predicate func(HistoryRecord[S, E]) bool) []HistoryRecord[S, E] {
	if h == nil {
		return nil
	}

	all := h.All()
	var result []HistoryRecord[S, E]
	for _, record := range all {
		if predicate(record) {
			result = append(result, record)
		}
	}
	return result
}

// TransitionsFrom returns all records where the transition started from the given state.
func (h *History[S, E]) TransitionsFrom(state S) []HistoryRecord[S, E] {
	return h.Filter(func(r HistoryRecord[S, E]) bool {
		return r.From == state
	})
}

// TransitionsTo returns all records where the transition ended at the given state.
func (h *History[S, E]) TransitionsTo(state S) []HistoryRecord[S, E] {
	return h.Filter(func(r HistoryRecord[S, E]) bool {
		return r.To == state
	})
}

// TransitionsByEvent returns all records triggered by the given event.
func (h *History[S, E]) TransitionsByEvent(event E) []HistoryRecord[S, E] {
	return h.Filter(func(r HistoryRecord[S, E]) bool {
		return r.Event == event
	})
}
