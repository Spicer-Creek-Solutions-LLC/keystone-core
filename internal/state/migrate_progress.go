package state

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// ProgressReporter tracks and reports migration progress
type ProgressReporter struct {
	mu sync.RWMutex

	// Overall progress
	startTime time.Time
	tables    []string
	current   map[string]*TableProgress

	// Callbacks
	onProgress func(*ProgressSnapshot)
	onComplete func(*ProgressSnapshot)
	onError    func(string, string, error)

	// Output
	writer    io.Writer
	format    ProgressFormat
	lastPrint time.Time

	// Settings
	printInterval time.Duration
	showETA       bool
	showRate      bool
}

// TableProgress tracks progress for a single table
type TableProgress struct {
	Name             string
	TotalRecords     int
	ProcessedRecords int
	SkippedRecords   int
	FailedRecords    int
	StartTime        time.Time
	EndTime          time.Time
	Status           TableStatus
	CurrentRate      float64 // records per second
	rateWindow       []ratePoint
}

type ratePoint struct {
	timestamp time.Time
	count     int
}

// TableStatus represents the status of a table migration
type TableStatus string

// TableStatus constants define the possible statuses.
const (
	TableStatusPending    TableStatus = "pending"
	TableStatusInProgress TableStatus = "in_progress"
	TableStatusCompleted  TableStatus = "completed"
	TableStatusFailed     TableStatus = "failed"
)

// ProgressFormat defines how progress is displayed
type ProgressFormat string

// FormatText constants define the output formats.
const (
	FormatText     ProgressFormat = "text"
	FormatJSON     ProgressFormat = "json"
	FormatCompact  ProgressFormat = "compact"
	FormatProgress ProgressFormat = "progress" // Progress bar style
)

// ProgressSnapshot represents a point-in-time view of migration progress
type ProgressSnapshot struct {
	Timestamp          time.Time
	Elapsed            time.Duration
	EstimatedRemaining time.Duration
	OverallPercent     float64
	Tables             map[string]*TableSnapshot
	CurrentTable       string
	RecordsPerSecond   float64
	TotalProcessed     int
	TotalRecords       int
	TotalSkipped       int
	TotalFailed        int
}

// TableSnapshot represents a point-in-time view of table progress
type TableSnapshot struct {
	Name      string
	Status    TableStatus
	Percent   float64
	Processed int
	Total     int
	Skipped   int
	Failed    int
	Elapsed   time.Duration
	Rate      float64
}

// ProgressReporterConfig configures the progress reporter
type ProgressReporterConfig struct {
	Writer        io.Writer
	Format        ProgressFormat
	PrintInterval time.Duration
	ShowETA       bool
	ShowRate      bool
	OnProgress    func(*ProgressSnapshot)
	OnComplete    func(*ProgressSnapshot)
	OnError       func(string, string, error)
}

// DefaultProgressReporterConfig returns sensible defaults
func DefaultProgressReporterConfig() *ProgressReporterConfig {
	return &ProgressReporterConfig{
		Writer:        os.Stdout,
		Format:        FormatText,
		PrintInterval: 1 * time.Second,
		ShowETA:       true,
		ShowRate:      true,
	}
}

// NewProgressReporter creates a new progress reporter
func NewProgressReporter(config *ProgressReporterConfig) *ProgressReporter {
	if config == nil {
		config = DefaultProgressReporterConfig()
	}

	writer := config.Writer
	if writer == nil {
		writer = os.Stdout
	}

	return &ProgressReporter{
		current:       make(map[string]*TableProgress),
		writer:        writer,
		format:        config.Format,
		printInterval: config.PrintInterval,
		showETA:       config.ShowETA,
		showRate:      config.ShowRate,
		onProgress:    config.OnProgress,
		onComplete:    config.OnComplete,
		onError:       config.OnError,
	}
}

// Start begins tracking migration progress
func (pr *ProgressReporter) Start(tables []string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	pr.startTime = time.Now()
	pr.tables = tables

	for _, table := range tables {
		pr.current[table] = &TableProgress{
			Name:       table,
			Status:     TableStatusPending,
			rateWindow: make([]ratePoint, 0, 100),
		}
	}
}

// StartTable marks a table as starting migration
func (pr *ProgressReporter) StartTable(table string, totalRecords int) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	if tp, ok := pr.current[table]; ok {
		tp.Status = TableStatusInProgress
		tp.StartTime = time.Now()
		tp.TotalRecords = totalRecords
		tp.ProcessedRecords = 0
		tp.rateWindow = append(tp.rateWindow, ratePoint{
			timestamp: time.Now(),
			count:     0,
		})
	}
}

// UpdateProgress updates the progress for a table
func (pr *ProgressReporter) UpdateProgress(table string, processed, skipped, failed int) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	if tp, ok := pr.current[table]; ok {
		tp.ProcessedRecords = processed
		tp.SkippedRecords = skipped
		tp.FailedRecords = failed

		// Update rate window
		now := time.Now()
		tp.rateWindow = append(tp.rateWindow, ratePoint{
			timestamp: now,
			count:     processed,
		})

		// Keep only recent points (last 30 seconds)
		cutoff := now.Add(-30 * time.Second)
		filtered := tp.rateWindow[:0]
		for _, p := range tp.rateWindow {
			if p.timestamp.After(cutoff) {
				filtered = append(filtered, p)
			}
		}
		tp.rateWindow = filtered

		// Calculate rate
		if len(tp.rateWindow) >= 2 {
			first := tp.rateWindow[0]
			last := tp.rateWindow[len(tp.rateWindow)-1]
			duration := last.timestamp.Sub(first.timestamp).Seconds()
			if duration > 0 {
				tp.CurrentRate = float64(last.count-first.count) / duration
			}
		}
	}

	// Print progress if enough time has passed
	if time.Since(pr.lastPrint) >= pr.printInterval {
		pr.printProgressLocked()
		pr.lastPrint = time.Now()
	}
}

// CompleteTable marks a table as completed
func (pr *ProgressReporter) CompleteTable(table string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	if tp, ok := pr.current[table]; ok {
		tp.Status = TableStatusCompleted
		tp.EndTime = time.Now()
	}
}

// FailTable marks a table as failed
func (pr *ProgressReporter) FailTable(table string, err error) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	if tp, ok := pr.current[table]; ok {
		tp.Status = TableStatusFailed
		tp.EndTime = time.Now()
	}

	if pr.onError != nil {
		pr.onError(table, "", err)
	}
}

// RecordError records an error for a specific record
func (pr *ProgressReporter) RecordError(table, recordID string, err error) {
	if pr.onError != nil {
		pr.onError(table, recordID, err)
	}
}

// GetSnapshot returns a snapshot of current progress
func (pr *ProgressReporter) GetSnapshot() *ProgressSnapshot {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	return pr.getSnapshotLocked()
}

func (pr *ProgressReporter) getSnapshotLocked() *ProgressSnapshot {
	snap := &ProgressSnapshot{
		Timestamp: time.Now(),
		Elapsed:   time.Since(pr.startTime),
		Tables:    make(map[string]*TableSnapshot),
	}

	var totalProcessed, totalRecords, totalSkipped, totalFailed int
	var overallRate float64
	var currentTable string

	for name, tp := range pr.current {
		tableSnap := &TableSnapshot{
			Name:      name,
			Status:    tp.Status,
			Processed: tp.ProcessedRecords,
			Total:     tp.TotalRecords,
			Skipped:   tp.SkippedRecords,
			Failed:    tp.FailedRecords,
			Rate:      tp.CurrentRate,
		}

		if tp.TotalRecords > 0 {
			tableSnap.Percent = float64(tp.ProcessedRecords) / float64(tp.TotalRecords) * 100
		}

		if tp.Status == TableStatusInProgress || tp.Status == TableStatusCompleted {
			if tp.EndTime.IsZero() {
				tableSnap.Elapsed = time.Since(tp.StartTime)
			} else {
				tableSnap.Elapsed = tp.EndTime.Sub(tp.StartTime)
			}
		}

		snap.Tables[name] = tableSnap

		totalProcessed += tp.ProcessedRecords
		totalRecords += tp.TotalRecords
		totalSkipped += tp.SkippedRecords
		totalFailed += tp.FailedRecords
		overallRate += tp.CurrentRate

		if tp.Status == TableStatusInProgress {
			currentTable = name
		}
	}

	snap.TotalProcessed = totalProcessed
	snap.TotalRecords = totalRecords
	snap.TotalSkipped = totalSkipped
	snap.TotalFailed = totalFailed
	snap.RecordsPerSecond = overallRate
	snap.CurrentTable = currentTable

	if totalRecords > 0 {
		snap.OverallPercent = float64(totalProcessed) / float64(totalRecords) * 100
	}

	// Estimate remaining time
	if overallRate > 0 && totalRecords > totalProcessed {
		remaining := totalRecords - totalProcessed
		snap.EstimatedRemaining = time.Duration(float64(remaining)/overallRate) * time.Second
	}

	return snap
}

// printProgressLocked prints progress (caller must hold lock)
func (pr *ProgressReporter) printProgressLocked() {
	snap := pr.getSnapshotLocked()

	switch pr.format {
	case FormatJSON:
		pr.printJSON(snap)
	case FormatCompact:
		pr.printCompact(snap)
	case FormatProgress:
		pr.printProgressBar(snap)
	default:
		pr.printText(snap)
	}

	if pr.onProgress != nil {
		pr.onProgress(snap)
	}
}

func (pr *ProgressReporter) printText(snap *ProgressSnapshot) {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\n=== Migration Progress (%.1f%%) ===\n", snap.OverallPercent))
	sb.WriteString(fmt.Sprintf("Elapsed: %s", formatDuration(snap.Elapsed)))

	if pr.showETA && snap.EstimatedRemaining > 0 {
		sb.WriteString(fmt.Sprintf("  |  ETA: %s", formatDuration(snap.EstimatedRemaining)))
	}
	if pr.showRate {
		sb.WriteString(fmt.Sprintf("  |  Rate: %.1f rec/s", snap.RecordsPerSecond))
	}
	sb.WriteString("\n\n")

	for _, table := range pr.tables {
		ts := snap.Tables[table]
		if ts == nil {
			continue
		}

		status := string(ts.Status)
		switch ts.Status {
		case TableStatusCompleted:
			status = "DONE"
		case TableStatusInProgress:
			status = "RUNNING"
		case TableStatusFailed:
			status = "FAILED"
		case TableStatusPending:
			status = "PENDING"
		}

		if ts.Total > 0 {
			sb.WriteString(fmt.Sprintf("  %-25s [%s] %d/%d (%.1f%%)  skipped=%d failed=%d\n",
				ts.Name, status, ts.Processed, ts.Total, ts.Percent,
				ts.Skipped, ts.Failed))
		} else {
			sb.WriteString(fmt.Sprintf("  %-25s [%s]\n", ts.Name, status))
		}
	}

	sb.WriteString("\n")
	fmt.Fprint(pr.writer, sb.String())
}

func (pr *ProgressReporter) printCompact(snap *ProgressSnapshot) {
	var parts []string
	for _, table := range pr.tables {
		ts := snap.Tables[table]
		if ts == nil {
			continue
		}

		status := "?"
		switch ts.Status {
		case TableStatusCompleted:
			status = "✓"
		case TableStatusInProgress:
			status = fmt.Sprintf("%.0f%%", ts.Percent)
		case TableStatusFailed:
			status = "✗"
		case TableStatusPending:
			status = "-"
		}
		parts = append(parts, fmt.Sprintf("%s:%s", ts.Name, status))
	}

	fmt.Fprintf(pr.writer, "[%.1f%%] %s | %.1f rec/s\n",
		snap.OverallPercent,
		strings.Join(parts, " "),
		snap.RecordsPerSecond)
}

func (pr *ProgressReporter) printProgressBar(snap *ProgressSnapshot) {
	barWidth := 40
	filled := int(snap.OverallPercent / 100 * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}

	bar := strings.Repeat("=", filled) + strings.Repeat("-", barWidth-filled)

	var eta string
	if pr.showETA && snap.EstimatedRemaining > 0 {
		eta = fmt.Sprintf(" ETA: %s", formatDuration(snap.EstimatedRemaining))
	}

	// Use carriage return to overwrite the line
	fmt.Fprintf(pr.writer, "\r[%s] %.1f%% (%d/%d)%s",
		bar, snap.OverallPercent,
		snap.TotalProcessed, snap.TotalRecords, eta)
}

func (pr *ProgressReporter) printJSON(snap *ProgressSnapshot) {
	fmt.Fprintf(pr.writer, `{"percent":%.1f,"processed":%d,"total":%d,"elapsed_ms":%d,"eta_ms":%d,"rate":%.1f}`,
		snap.OverallPercent,
		snap.TotalProcessed,
		snap.TotalRecords,
		snap.Elapsed.Milliseconds(),
		snap.EstimatedRemaining.Milliseconds(),
		snap.RecordsPerSecond)
	fmt.Fprintln(pr.writer)
}

// Complete marks the migration as complete and prints final summary
func (pr *ProgressReporter) Complete() {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	snap := pr.getSnapshotLocked()

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString("=== Migration Complete ===\n")
	sb.WriteString(fmt.Sprintf("Total time: %s\n", formatDuration(snap.Elapsed)))
	sb.WriteString(fmt.Sprintf("Records processed: %d\n", snap.TotalProcessed))
	sb.WriteString(fmt.Sprintf("Records skipped: %d\n", snap.TotalSkipped))
	sb.WriteString(fmt.Sprintf("Records failed: %d\n", snap.TotalFailed))
	sb.WriteString(fmt.Sprintf("Average rate: %.1f rec/s\n", float64(snap.TotalProcessed)/snap.Elapsed.Seconds()))
	sb.WriteString("\nTable summary:\n")

	for _, table := range pr.tables {
		ts := snap.Tables[table]
		if ts == nil {
			continue
		}
		sb.WriteString(fmt.Sprintf("  %-25s %d records in %s\n",
			ts.Name, ts.Processed, formatDuration(ts.Elapsed)))
	}

	fmt.Fprint(pr.writer, sb.String())

	if pr.onComplete != nil {
		pr.onComplete(snap)
	}
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm%ds", m, s)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh%dm", h, m)
}

// ProgressMigrator wraps a Migrator with progress reporting
type ProgressMigrator struct {
	*Migrator
	reporter *ProgressReporter
}

// NewProgressMigrator creates a migrator with progress reporting
func NewProgressMigrator(source, target Store, opts *MigrationOptions, reporterConfig *ProgressReporterConfig) *ProgressMigrator {
	reporter := NewProgressReporter(reporterConfig)

	return &ProgressMigrator{
		Migrator: NewMigrator(source, target, opts),
		reporter: reporter,
	}
}

// Migrate performs migration with progress reporting
func (pm *ProgressMigrator) Migrate(ctx context.Context) (*MigrationStats, error) {
	tables := []string{"agents", "commands", "batch_jobs", "batch_agent_results"}
	pm.reporter.Start(tables)

	// Wrap the existing progress callback
	originalCallback := pm.opts.ProgressCallback
	pm.opts.ProgressCallback = func(table string, current, total int) {
		// Initialize table if this is the first callback
		if current == 0 || (pm.reporter.current[table] != nil && pm.reporter.current[table].TotalRecords == 0) {
			pm.reporter.StartTable(table, total)
		}

		// Update progress
		pm.reporter.UpdateProgress(table, current, 0, 0)

		// Mark complete if done
		if current == total && total > 0 {
			pm.reporter.CompleteTable(table)
		}

		// Call original callback
		if originalCallback != nil {
			originalCallback(table, current, total)
		}
	}

	// Run the migration
	stats, err := pm.Migrator.Migrate(ctx)

	// Print final summary
	pm.reporter.Complete()

	return stats, err
}

// GetReporter returns the progress reporter
func (pm *ProgressMigrator) GetReporter() *ProgressReporter {
	return pm.reporter
}

// MultiProgressReporter tracks progress for multiple concurrent migrations
type MultiProgressReporter struct {
	mu        sync.RWMutex
	reporters map[string]*ProgressReporter
	writer    io.Writer
}

// NewMultiProgressReporter creates a reporter for multiple migrations
func NewMultiProgressReporter(writer io.Writer) *MultiProgressReporter {
	if writer == nil {
		writer = os.Stdout
	}
	return &MultiProgressReporter{
		reporters: make(map[string]*ProgressReporter),
		writer:    writer,
	}
}

// AddMigration adds a migration to track
func (mpr *MultiProgressReporter) AddMigration(id string, config *ProgressReporterConfig) *ProgressReporter {
	mpr.mu.Lock()
	defer mpr.mu.Unlock()

	reporter := NewProgressReporter(config)
	mpr.reporters[id] = reporter
	return reporter
}

// GetSnapshot returns snapshots for all migrations
func (mpr *MultiProgressReporter) GetSnapshot() map[string]*ProgressSnapshot {
	mpr.mu.RLock()
	defer mpr.mu.RUnlock()

	result := make(map[string]*ProgressSnapshot)
	for id, reporter := range mpr.reporters {
		result[id] = reporter.GetSnapshot()
	}
	return result
}

// PrintSummary prints a summary of all migrations
func (mpr *MultiProgressReporter) PrintSummary() {
	mpr.mu.RLock()
	defer mpr.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("\n=== Multi-Migration Progress ===\n\n")

	for id, reporter := range mpr.reporters {
		snap := reporter.GetSnapshot()
		sb.WriteString(fmt.Sprintf("Migration %s: %.1f%% (%d/%d) - %s\n",
			id, snap.OverallPercent, snap.TotalProcessed, snap.TotalRecords,
			formatDuration(snap.Elapsed)))
	}

	fmt.Fprint(mpr.writer, sb.String())
}
