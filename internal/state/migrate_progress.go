// SPDX-License-Identifier: Apache-2.0

package state

import "time"

// ProgressUpdate is emitted via MigrationOptions.ProgressCallback as
// each batch finishes for a given table. Values are computed lazily;
// callers can format them however they like.
type ProgressUpdate struct {
	Table         string        // table being migrated
	RowsCompleted int           // rows of this table read so far
	RowsTotal     int           // total rows of this table at start
	RowsPerSecond float64       // average rate so far for this table
	ETA           time.Duration // estimated time to finish this table
}

// progressReporter tracks per-table start time + completion counts and
// emits ProgressUpdate via the registered callback. Single-goroutine
// usage from Migrate; no locking.
type progressReporter struct {
	cb         func(ProgressUpdate)
	tableStart map[string]time.Time
	tableTotal map[string]int
}

func newProgressReporter(cb func(ProgressUpdate)) *progressReporter {
	return &progressReporter{
		cb:         cb,
		tableStart: map[string]time.Time{},
		tableTotal: map[string]int{},
	}
}

// Start marks the beginning of a table migration with the known total
// row count. Subsequent Update calls compute ETA against this total.
func (r *progressReporter) Start(table string, total int) {
	r.tableStart[table] = time.Now().UTC()
	r.tableTotal[table] = total
}

// Update emits a ProgressUpdate reflecting `completed` rows of `table`.
// No-op when the callback is nil.
func (r *progressReporter) Update(table string, completed int) {
	if r.cb == nil {
		return
	}
	total := r.tableTotal[table]
	rate, eta := computeRateAndETA(r.tableStart[table], completed, total)
	r.cb(ProgressUpdate{
		Table:         table,
		RowsCompleted: completed,
		RowsTotal:     total,
		RowsPerSecond: rate,
		ETA:           eta,
	})
}

// computeRateAndETA derives rows-per-second and estimated time-to-finish
// from the elapsed window and rows completed. Exposed as a free function
// so it's covered by pure unit tests.
func computeRateAndETA(start time.Time, completed, total int) (rate float64, eta time.Duration) {
	if completed <= 0 {
		return 0, 0
	}
	elapsed := time.Since(start).Seconds()
	if elapsed <= 0 {
		return 0, 0
	}
	rate = float64(completed) / elapsed
	if rate <= 0 || total <= 0 || completed >= total {
		return rate, 0
	}
	remaining := total - completed
	seconds := float64(remaining) / rate
	return rate, time.Duration(seconds * float64(time.Second))
}
