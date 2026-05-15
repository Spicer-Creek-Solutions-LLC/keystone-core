package audit

import (
	"context"
	"errors"
	"fmt"
)

// DefaultIterateMaxPages caps [IterateAll] so a corrupted cursor
// can't drive an infinite loop. Override with [WithIterateMaxPages].
const DefaultIterateMaxPages = 10_000

// ErrIterateMaxPages is returned when [IterateAll] exhausts its
// configured page budget without seeing an empty NextCursor.
var ErrIterateMaxPages = errors.New("audit: IterateAll exceeded MaxPages")

// IterateOption configures [IterateAll].
type IterateOption func(*iterateConfig)

type iterateConfig struct {
	maxPages int
}

// WithIterateMaxPages caps the number of Query calls IterateAll will
// make. Non-positive values fall back to [DefaultIterateMaxPages].
func WithIterateMaxPages(n int) IterateOption {
	return func(c *iterateConfig) {
		if n > 0 {
			c.maxPages = n
		}
	}
}

// IterateAll walks the full result set for query by repeatedly
// calling store.Query with the previous page's NextCursor, until
// the store reports the page is the last (empty NextCursor).
//
// fn is called for each entry in order. Return a non-nil error from
// fn to abort iteration; that error is returned to the caller and
// no further pages are fetched. A canceled ctx between pages is
// detected before the next Query and returned as ctx.Err().
//
// Pagination state — Limit and Descending — comes from query
// directly; Cursor is overwritten between pages. A non-empty
// initial Cursor resumes pagination from that point.
//
// MaxPages caps the number of Query calls (default
// [DefaultIterateMaxPages]); exceeding it returns
// [ErrIterateMaxPages] to surface a likely-corrupted cursor loop.
func IterateAll(
	ctx context.Context,
	store AuditStore,
	query AuditQuery,
	fn func(AuditEntry) error,
	opts ...IterateOption,
) error {
	if store == nil {
		return errors.New("audit: IterateAll: store is required")
	}
	if fn == nil {
		return errors.New("audit: IterateAll: fn is required")
	}
	cfg := iterateConfig{maxPages: DefaultIterateMaxPages}
	for _, opt := range opts {
		opt(&cfg)
	}

	for page := 0; page < cfg.maxPages; page++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		got, err := store.Query(ctx, query)
		if err != nil {
			return fmt.Errorf("audit: IterateAll page %d: %w", page, err)
		}
		for _, e := range got.Entries {
			if err := fn(e); err != nil {
				return err
			}
		}
		if got.NextCursor == "" {
			return nil
		}
		query.Cursor = got.NextCursor
	}
	return ErrIterateMaxPages
}
