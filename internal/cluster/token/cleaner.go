package token

import (
	"context"
	"log"
	"time"
)

// DefaultCleanupInterval is the default interval between cleanup runs.
const DefaultCleanupInterval = 1 * time.Hour

// Cleaner periodically removes expired and revoked tokens from a Store.
type Cleaner struct {
	store    Store
	interval time.Duration
	stopChan chan struct{}
	doneChan chan struct{}
}

// NewCleaner creates a new token cleaner.
// If interval is zero, DefaultCleanupInterval is used.
func NewCleaner(store Store, interval time.Duration) *Cleaner {
	if interval <= 0 {
		interval = DefaultCleanupInterval
	}
	return &Cleaner{
		store:    store,
		interval: interval,
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
}

// Start begins periodic cleanup. It blocks until Stop is called or the
// context is cancelled. Call in a goroutine.
func (c *Cleaner) Start(ctx context.Context) {
	defer close(c.doneChan)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopChan:
			return
		case <-ticker.C:
			n, err := c.store.DeleteExpired(ctx)
			if err != nil {
				log.Printf("[WARN] token cleaner: failed to delete expired tokens: %v", err)
				continue
			}
			if n > 0 {
				log.Printf("[INFO] token cleaner: deleted %d expired/revoked tokens", n)
			}
		}
	}
}

// Stop signals the cleaner to stop and waits for it to finish.
func (c *Cleaner) Stop() {
	close(c.stopChan)
	<-c.doneChan
}

// Done returns a channel that is closed when the cleaner has stopped.
func (c *Cleaner) Done() <-chan struct{} {
	return c.doneChan
}
