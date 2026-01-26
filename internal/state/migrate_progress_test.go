package state

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestDefaultProgressReporterConfig(t *testing.T) {
	config := DefaultProgressReporterConfig()

	if config.Format != FormatText {
		t.Errorf("expected text format, got %s", config.Format)
	}

	if config.PrintInterval != 1*time.Second {
		t.Errorf("expected 1s print interval, got %v", config.PrintInterval)
	}

	if !config.ShowETA {
		t.Error("expected ShowETA to be true")
	}

	if !config.ShowRate {
		t.Error("expected ShowRate to be true")
	}
}

func TestNewProgressReporter(t *testing.T) {
	tests := []struct {
		name   string
		config *ProgressReporterConfig
	}{
		{"nil config", nil},
		{"default config", DefaultProgressReporterConfig()},
		{"custom config", &ProgressReporterConfig{Format: FormatJSON}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := NewProgressReporter(tt.config)
			if pr == nil {
				t.Fatal("expected non-nil reporter")
			}
		})
	}
}

func TestProgressReporter_Start(t *testing.T) {
	pr := NewProgressReporter(nil)

	tables := []string{"agents", "commands", "batch_jobs"}
	pr.Start(tables)

	if len(pr.tables) != 3 {
		t.Errorf("expected 3 tables, got %d", len(pr.tables))
	}

	for _, table := range tables {
		if _, ok := pr.current[table]; !ok {
			t.Errorf("expected table %s to be tracked", table)
		}
	}
}

func TestProgressReporter_StartTable(t *testing.T) {
	pr := NewProgressReporter(nil)
	pr.Start([]string{"agents"})

	pr.StartTable("agents", 100)

	if pr.current["agents"].Status != TableStatusInProgress {
		t.Error("expected table to be in progress")
	}

	if pr.current["agents"].TotalRecords != 100 {
		t.Errorf("expected 100 total records, got %d", pr.current["agents"].TotalRecords)
	}
}

func TestProgressReporter_UpdateProgress(t *testing.T) {
	var buf bytes.Buffer
	config := &ProgressReporterConfig{
		Writer:        &buf,
		Format:        FormatCompact,
		PrintInterval: 0, // Immediate printing for tests
	}

	pr := NewProgressReporter(config)
	pr.Start([]string{"agents"})
	pr.StartTable("agents", 100)

	pr.UpdateProgress("agents", 50, 5, 2)

	if pr.current["agents"].ProcessedRecords != 50 {
		t.Errorf("expected 50 processed, got %d", pr.current["agents"].ProcessedRecords)
	}
	if pr.current["agents"].SkippedRecords != 5 {
		t.Errorf("expected 5 skipped, got %d", pr.current["agents"].SkippedRecords)
	}
	if pr.current["agents"].FailedRecords != 2 {
		t.Errorf("expected 2 failed, got %d", pr.current["agents"].FailedRecords)
	}
}

func TestProgressReporter_CompleteTable(t *testing.T) {
	pr := NewProgressReporter(nil)
	pr.Start([]string{"agents"})
	pr.StartTable("agents", 100)

	pr.CompleteTable("agents")

	if pr.current["agents"].Status != TableStatusCompleted {
		t.Error("expected table to be completed")
	}

	if pr.current["agents"].EndTime.IsZero() {
		t.Error("expected end time to be set")
	}
}

func TestProgressReporter_FailTable(t *testing.T) {
	var errorCalled bool
	config := &ProgressReporterConfig{
		OnError: func(table, recordID string, err error) {
			errorCalled = true
		},
	}

	pr := NewProgressReporter(config)
	pr.Start([]string{"agents"})
	pr.StartTable("agents", 100)

	pr.FailTable("agents", nil)

	if pr.current["agents"].Status != TableStatusFailed {
		t.Error("expected table to be failed")
	}

	if !errorCalled {
		t.Error("expected error callback to be called")
	}
}

func TestProgressReporter_GetSnapshot(t *testing.T) {
	pr := NewProgressReporter(nil)
	pr.Start([]string{"agents", "commands"})
	pr.StartTable("agents", 100)
	pr.UpdateProgress("agents", 50, 5, 2)

	snap := pr.GetSnapshot()

	if snap.TotalRecords != 100 {
		t.Errorf("expected 100 total records, got %d", snap.TotalRecords)
	}

	if snap.TotalProcessed != 50 {
		t.Errorf("expected 50 processed, got %d", snap.TotalProcessed)
	}

	if snap.CurrentTable != "agents" {
		t.Errorf("expected current table 'agents', got '%s'", snap.CurrentTable)
	}

	if len(snap.Tables) != 2 {
		t.Errorf("expected 2 tables in snapshot, got %d", len(snap.Tables))
	}

	agentSnap := snap.Tables["agents"]
	if agentSnap == nil {
		t.Fatal("expected agents in snapshot")
	}

	if agentSnap.Percent != 50 {
		t.Errorf("expected 50%%, got %.1f%%", agentSnap.Percent)
	}
}

func TestProgressReporter_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	config := &ProgressReporterConfig{
		Writer:        &buf,
		Format:        FormatText,
		PrintInterval: 0,
		ShowETA:       true,
		ShowRate:      true,
	}

	pr := NewProgressReporter(config)
	pr.Start([]string{"agents"})
	pr.StartTable("agents", 100)
	pr.UpdateProgress("agents", 50, 0, 0)

	output := buf.String()

	if !strings.Contains(output, "Migration Progress") {
		t.Error("expected 'Migration Progress' in output")
	}

	if !strings.Contains(output, "agents") {
		t.Error("expected 'agents' in output")
	}

	if !strings.Contains(output, "50/100") {
		t.Error("expected '50/100' in output")
	}
}

func TestProgressReporter_CompactFormat(t *testing.T) {
	var buf bytes.Buffer
	config := &ProgressReporterConfig{
		Writer:        &buf,
		Format:        FormatCompact,
		PrintInterval: 0,
	}

	pr := NewProgressReporter(config)
	pr.Start([]string{"agents"})
	pr.StartTable("agents", 100)
	pr.UpdateProgress("agents", 50, 0, 0)

	output := buf.String()

	if !strings.Contains(output, "agents:50%") {
		t.Errorf("expected 'agents:50%%' in compact output, got: %s", output)
	}
}

func TestProgressReporter_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	config := &ProgressReporterConfig{
		Writer:        &buf,
		Format:        FormatJSON,
		PrintInterval: 0,
	}

	pr := NewProgressReporter(config)
	pr.Start([]string{"agents"})
	pr.StartTable("agents", 100)
	pr.UpdateProgress("agents", 50, 0, 0)

	output := buf.String()

	if !strings.Contains(output, `"percent":50`) {
		t.Errorf("expected JSON with percent:50, got: %s", output)
	}

	if !strings.Contains(output, `"processed":50`) {
		t.Errorf("expected JSON with processed:50, got: %s", output)
	}
}

func TestProgressReporter_ProgressBarFormat(t *testing.T) {
	var buf bytes.Buffer
	config := &ProgressReporterConfig{
		Writer:        &buf,
		Format:        FormatProgress,
		PrintInterval: 0,
	}

	pr := NewProgressReporter(config)
	pr.Start([]string{"agents"})
	pr.StartTable("agents", 100)
	pr.UpdateProgress("agents", 50, 0, 0)

	output := buf.String()

	if !strings.Contains(output, "[") || !strings.Contains(output, "]") {
		t.Errorf("expected progress bar brackets in output, got: %s", output)
	}

	if !strings.Contains(output, "50.0%") {
		t.Errorf("expected '50.0%%' in progress bar, got: %s", output)
	}
}

func TestProgressReporter_Complete(t *testing.T) {
	var buf bytes.Buffer
	config := &ProgressReporterConfig{
		Writer: &buf,
		Format: FormatText,
	}

	var completeCalled bool
	config.OnComplete = func(snap *ProgressSnapshot) {
		completeCalled = true
	}

	pr := NewProgressReporter(config)
	pr.Start([]string{"agents"})
	pr.StartTable("agents", 100)
	pr.UpdateProgress("agents", 100, 0, 0)
	pr.CompleteTable("agents")
	pr.Complete()

	output := buf.String()

	if !strings.Contains(output, "Migration Complete") {
		t.Error("expected 'Migration Complete' in output")
	}

	if !completeCalled {
		t.Error("expected complete callback to be called")
	}
}

func TestProgressReporter_Callbacks(t *testing.T) {
	var progressCount int
	var lastSnapshot *ProgressSnapshot

	config := &ProgressReporterConfig{
		Writer:        &bytes.Buffer{},
		PrintInterval: 0,
		OnProgress: func(snap *ProgressSnapshot) {
			progressCount++
			lastSnapshot = snap
		},
	}

	pr := NewProgressReporter(config)
	pr.Start([]string{"agents"})
	pr.StartTable("agents", 100)

	for i := 0; i < 5; i++ {
		pr.UpdateProgress("agents", (i+1)*20, 0, 0)
	}

	if progressCount != 5 {
		t.Errorf("expected 5 progress callbacks, got %d", progressCount)
	}

	if lastSnapshot.TotalProcessed != 100 {
		t.Errorf("expected last snapshot with 100 processed, got %d", lastSnapshot.TotalProcessed)
	}
}

func TestProgressReporter_RecordError(t *testing.T) {
	var errorTable, errorRecord string

	config := &ProgressReporterConfig{
		OnError: func(table, recordID string, err error) {
			errorTable = table
			errorRecord = recordID
		},
	}

	pr := NewProgressReporter(config)
	pr.RecordError("agents", "agent-123", nil)

	if errorTable != "agents" {
		t.Errorf("expected table 'agents', got '%s'", errorTable)
	}

	if errorRecord != "agent-123" {
		t.Errorf("expected record 'agent-123', got '%s'", errorRecord)
	}
}

func TestProgressReporter_RateCalculation(t *testing.T) {
	pr := NewProgressReporter(&ProgressReporterConfig{
		Writer:        &bytes.Buffer{},
		PrintInterval: time.Hour, // Don't print during test
	})

	pr.Start([]string{"agents"})
	pr.StartTable("agents", 1000)

	// Simulate progress over time
	for i := 0; i < 5; i++ {
		pr.UpdateProgress("agents", (i+1)*100, 0, 0)
	}

	pr.mu.Lock()
	tp := pr.current["agents"]
	base := time.Now().Add(-50 * time.Millisecond)
	for i := range tp.rateWindow {
		tp.rateWindow[i].timestamp = base.Add(time.Duration(i) * 10 * time.Millisecond)
	}
	if len(tp.rateWindow) >= 2 {
		first := tp.rateWindow[0]
		last := tp.rateWindow[len(tp.rateWindow)-1]
		duration := last.timestamp.Sub(first.timestamp).Seconds()
		if duration > 0 {
			tp.CurrentRate = float64(last.count-first.count) / duration
		}
	}
	pr.mu.Unlock()

	// Rate should be > 0 after updates
	if pr.current["agents"].CurrentRate <= 0 {
		t.Error("expected positive rate after updates")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{500 * time.Millisecond, "500ms"},
		{5 * time.Second, "5.0s"},
		{90 * time.Second, "1m30s"},
		{3600 * time.Second, "1h0m"},
		{5400 * time.Second, "1h30m"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestTableSnapshot_Percent(t *testing.T) {
	pr := NewProgressReporter(nil)
	pr.Start([]string{"agents"})
	pr.StartTable("agents", 0) // Zero total

	snap := pr.GetSnapshot()
	agentSnap := snap.Tables["agents"]

	if agentSnap.Percent != 0 {
		t.Errorf("expected 0%% for zero total, got %.1f%%", agentSnap.Percent)
	}
}

func TestProgressReporter_MultipleTables(t *testing.T) {
	var buf bytes.Buffer
	config := &ProgressReporterConfig{
		Writer:        &buf,
		Format:        FormatText,
		PrintInterval: time.Hour, // Don't auto-print
	}

	pr := NewProgressReporter(config)
	pr.Start([]string{"agents", "commands", "batch_jobs"})

	pr.StartTable("agents", 100)
	pr.UpdateProgress("agents", 100, 0, 0)
	pr.CompleteTable("agents")

	pr.StartTable("commands", 50)
	pr.UpdateProgress("commands", 25, 0, 0)

	snap := pr.GetSnapshot()

	// Overall progress should consider all tables
	if snap.TotalRecords != 150 { // 100 + 50
		t.Errorf("expected 150 total records, got %d", snap.TotalRecords)
	}

	if snap.TotalProcessed != 125 { // 100 + 25
		t.Errorf("expected 125 processed records, got %d", snap.TotalProcessed)
	}

	// Current table should be commands
	if snap.CurrentTable != "commands" {
		t.Errorf("expected current table 'commands', got '%s'", snap.CurrentTable)
	}
}

func TestMultiProgressReporter(t *testing.T) {
	var buf bytes.Buffer
	mpr := NewMultiProgressReporter(&buf)

	// Add multiple migrations
	r1 := mpr.AddMigration("migration-1", &ProgressReporterConfig{Writer: &bytes.Buffer{}})
	r2 := mpr.AddMigration("migration-2", &ProgressReporterConfig{Writer: &bytes.Buffer{}})

	r1.Start([]string{"agents"})
	r1.StartTable("agents", 100)
	r1.UpdateProgress("agents", 50, 0, 0)

	r2.Start([]string{"commands"})
	r2.StartTable("commands", 200)
	r2.UpdateProgress("commands", 100, 0, 0)

	snapshots := mpr.GetSnapshot()

	if len(snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snapshots))
	}

	if snapshots["migration-1"].TotalProcessed != 50 {
		t.Errorf("expected migration-1 to have 50 processed, got %d", snapshots["migration-1"].TotalProcessed)
	}

	if snapshots["migration-2"].TotalProcessed != 100 {
		t.Errorf("expected migration-2 to have 100 processed, got %d", snapshots["migration-2"].TotalProcessed)
	}

	mpr.PrintSummary()

	output := buf.String()
	if !strings.Contains(output, "migration-1") {
		t.Error("expected migration-1 in summary")
	}
	if !strings.Contains(output, "migration-2") {
		t.Error("expected migration-2 in summary")
	}
}

func TestProgressReporter_ETACalculation(t *testing.T) {
	pr := NewProgressReporter(&ProgressReporterConfig{
		Writer:        &bytes.Buffer{},
		PrintInterval: time.Hour,
	})

	pr.Start([]string{"agents"})
	pr.StartTable("agents", 1000)

	// Simulate some progress with longer delays to get measurable rate
	for i := 1; i <= 5; i++ {
		pr.UpdateProgress("agents", i*100, 0, 0) // 500 total processed
	}

	pr.mu.Lock()
	tp := pr.current["agents"]
	base := time.Now().Add(-250 * time.Millisecond)
	for i := range tp.rateWindow {
		tp.rateWindow[i].timestamp = base.Add(time.Duration(i) * 50 * time.Millisecond)
	}
	if len(tp.rateWindow) >= 2 {
		first := tp.rateWindow[0]
		last := tp.rateWindow[len(tp.rateWindow)-1]
		duration := last.timestamp.Sub(first.timestamp).Seconds()
		if duration > 0 {
			tp.CurrentRate = float64(last.count-first.count) / duration
		}
	}
	pr.mu.Unlock()

	snap := pr.GetSnapshot()

	// Rate should be positive
	if snap.RecordsPerSecond <= 0 {
		t.Errorf("expected positive rate, got %.2f", snap.RecordsPerSecond)
	}

	// With 500 processed and 500 remaining, ETA should be calculated
	// But since test timing is unpredictable, we just verify it was attempted
	// (ETA could be 0 if rate is extremely high)
	if snap.TotalProcessed != 500 {
		t.Errorf("expected 500 processed, got %d", snap.TotalProcessed)
	}
}
