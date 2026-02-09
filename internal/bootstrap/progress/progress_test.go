package progress

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if len(cfg.Phases) != 6 {
		t.Errorf("Should have 6 default phases, got %d", len(cfg.Phases))
	}
	if cfg.CallbackInterval != 100*time.Millisecond {
		t.Errorf("CallbackInterval = %v, want 100ms", cfg.CallbackInterval)
	}
	if !cfg.EnableTiming {
		t.Error("EnableTiming should be true by default")
	}

	// Check phase weights sum to 100
	var totalWeight int
	for _, phase := range cfg.Phases {
		totalWeight += cfg.PhaseWeights[phase]
	}
	if totalWeight != 100 {
		t.Errorf("Phase weights should sum to 100, got %d", totalWeight)
	}
}

func TestNewTracker(t *testing.T) {
	// With nil config
	tr := NewTracker(nil)
	if tr == nil {
		t.Fatal("NewTracker returned nil")
	}
	if len(tr.phases) != 6 {
		t.Error("Should initialize default phases")
	}

	// With custom config
	cfg := &Config{
		Phases: []Phase{PhaseInit, PhaseComplete},
	}
	tr = NewTracker(cfg)
	if len(tr.phases) != 2 {
		t.Errorf("Should have 2 phases, got %d", len(tr.phases))
	}
}

func TestTracker_Start(t *testing.T) {
	tr := NewTracker(nil)
	tr.Start()

	if tr.startedAt.IsZero() {
		t.Error("StartedAt should be set")
	}
	if tr.currentPhase != PhaseInit {
		t.Errorf("CurrentPhase = %s, want init", tr.currentPhase)
	}
}

func TestTracker_StartPhase(t *testing.T) {
	tr := NewTracker(nil)
	tr.Start()

	tr.StartPhase(PhaseDiscovery)

	report := tr.GetReport()
	if report.CurrentPhase != PhaseDiscovery {
		t.Errorf("CurrentPhase = %s, want discovery", report.CurrentPhase)
	}

	info := report.Phases[PhaseDiscovery]
	if info.Status != StatusRunning {
		t.Errorf("Phase status = %s, want running", info.Status)
	}
	if info.StartedAt.IsZero() {
		t.Error("Phase StartedAt should be set")
	}
}

func TestTracker_UpdatePhase(t *testing.T) {
	tr := NewTracker(nil)
	tr.Start()
	tr.StartPhase(PhaseDiscovery)

	tr.UpdatePhase(PhaseDiscovery, 50, "Scanning network")

	report := tr.GetReport()
	info := report.Phases[PhaseDiscovery]

	if info.Progress != 50 {
		t.Errorf("Progress = %d, want 50", info.Progress)
	}
	if info.Message != "Scanning network" {
		t.Errorf("Message = %s, want 'Scanning network'", info.Message)
	}
}

func TestTracker_UpdatePhase_Clamp(t *testing.T) {
	tr := NewTracker(nil)
	tr.Start()
	tr.StartPhase(PhaseDiscovery)

	// Test clamping
	tr.UpdatePhase(PhaseDiscovery, -10, "")
	report := tr.GetReport()
	if report.Phases[PhaseDiscovery].Progress != 0 {
		t.Error("Progress should be clamped to 0")
	}

	tr.UpdatePhase(PhaseDiscovery, 150, "")
	report = tr.GetReport()
	if report.Phases[PhaseDiscovery].Progress != 100 {
		t.Error("Progress should be clamped to 100")
	}
}

func TestTracker_CompletePhase(t *testing.T) {
	tr := NewTracker(nil)
	tr.Start()
	tr.StartPhase(PhaseDiscovery)
	report := tr.GetReport()
	startedAt := report.Phases[PhaseDiscovery].StartedAt
	if startedAt.IsZero() {
		t.Fatal("expected phase start time to be set")
	}
	if err := helpers.WaitForTimeout(100*time.Millisecond, 5*time.Millisecond, func() (bool, error) {
		return time.Since(startedAt) > 0, nil
	}); err != nil {
		t.Fatalf("expected phase to advance: %v", err)
	}

	tr.CompletePhase(PhaseDiscovery)

	report = tr.GetReport()
	info := report.Phases[PhaseDiscovery]

	if info.Status != StatusComplete {
		t.Errorf("Status = %s, want complete", info.Status)
	}
	if info.Progress != 100 {
		t.Errorf("Progress = %d, want 100", info.Progress)
	}
	if info.CompletedAt.IsZero() {
		t.Error("CompletedAt should be set")
	}
	if info.Duration == 0 {
		t.Error("Duration should be set")
	}
}

func TestTracker_FailPhase(t *testing.T) {
	tr := NewTracker(nil)
	tr.Start()
	tr.StartPhase(PhaseDiscovery)

	testErr := errors.New("connection failed")
	tr.FailPhase(PhaseDiscovery, testErr)

	report := tr.GetReport()
	info := report.Phases[PhaseDiscovery]

	if info.Status != StatusFailed {
		t.Errorf("Status = %s, want failed", info.Status)
	}
	if !errors.Is(info.Error, testErr) {
		t.Error("Error should be set")
	}
	if report.CurrentPhase != PhaseFailed {
		t.Errorf("CurrentPhase = %s, want failed", report.CurrentPhase)
	}
}

func TestTracker_SkipPhase(t *testing.T) {
	tr := NewTracker(nil)
	tr.Start()

	tr.SkipPhase(PhaseDiscovery, "Already registered")

	report := tr.GetReport()
	info := report.Phases[PhaseDiscovery]

	if info.Status != StatusSkipped {
		t.Errorf("Status = %s, want skipped", info.Status)
	}
	if info.Message != "Already registered" {
		t.Errorf("Message = %s, want 'Already registered'", info.Message)
	}
}

func TestTracker_Steps(t *testing.T) {
	tr := NewTracker(nil)
	tr.Start()
	tr.StartPhase(PhaseInstallation)

	// Start step
	tr.StartStep(PhaseInstallation, "download-agent")

	report := tr.GetReport()
	info := report.Phases[PhaseInstallation]
	if len(info.Steps) != 1 {
		t.Fatalf("Should have 1 step, got %d", len(info.Steps))
	}
	if info.Steps[0].Name != "download-agent" {
		t.Errorf("Step name = %s, want download-agent", info.Steps[0].Name)
	}
	if info.Steps[0].Status != StatusRunning {
		t.Errorf("Step status = %s, want running", info.Steps[0].Status)
	}

	// Update step
	tr.UpdateStep(PhaseInstallation, "download-agent", 50, "Downloading...")

	report = tr.GetReport()
	step := report.Phases[PhaseInstallation].Steps[0]
	if step.Progress != 50 {
		t.Errorf("Step progress = %d, want 50", step.Progress)
	}

	// Complete step
	tr.CompleteStep(PhaseInstallation, "download-agent")

	report = tr.GetReport()
	step = report.Phases[PhaseInstallation].Steps[0]
	if step.Status != StatusComplete {
		t.Errorf("Step status = %s, want complete", step.Status)
	}
	if step.Duration == 0 {
		t.Error("Step duration should be set")
	}
}

func TestTracker_StepBytes(t *testing.T) {
	tr := NewTracker(nil)
	tr.Start()
	tr.StartPhase(PhaseInstallation)
	tr.StartStep(PhaseInstallation, "download")

	tr.UpdateStepBytes(PhaseInstallation, "download", 50*1024*1024, 100*1024*1024)

	report := tr.GetReport()
	step := report.Phases[PhaseInstallation].Steps[0]

	if step.BytesDone != 50*1024*1024 {
		t.Errorf("BytesDone = %d, want 50MB", step.BytesDone)
	}
	if step.BytesTotal != 100*1024*1024 {
		t.Errorf("BytesTotal = %d, want 100MB", step.BytesTotal)
	}
	if step.Progress != 50 {
		t.Errorf("Progress = %d, want 50", step.Progress)
	}
}

func TestTracker_FailStep(t *testing.T) {
	tr := NewTracker(nil)
	tr.Start()
	tr.StartPhase(PhaseInstallation)
	tr.StartStep(PhaseInstallation, "download")

	testErr := errors.New("download failed")
	tr.FailStep(PhaseInstallation, "download", testErr)

	report := tr.GetReport()
	step := report.Phases[PhaseInstallation].Steps[0]

	if step.Status != StatusFailed {
		t.Errorf("Step status = %s, want failed", step.Status)
	}
	if !errors.Is(step.Error, testErr) {
		t.Error("Step error should be set")
	}
}

func TestTracker_Complete(t *testing.T) {
	tr := NewTracker(nil)
	tr.Start()
	tr.Complete()

	report := tr.GetReport()
	if report.CurrentPhase != PhaseComplete {
		t.Errorf("CurrentPhase = %s, want complete", report.CurrentPhase)
	}

	stats := tr.Stats().Snapshot()
	if !stats.Success {
		t.Error("Stats.Success should be true")
	}
}

func TestTracker_OverallProgress(t *testing.T) {
	cfg := &Config{
		Phases: []Phase{PhaseInit, PhaseDiscovery, PhaseComplete},
		PhaseWeights: map[Phase]int{
			PhaseInit:      25,
			PhaseDiscovery: 50,
			PhaseComplete:  25,
		},
	}
	tr := NewTracker(cfg)
	tr.Start()

	// Complete first phase (25%)
	tr.StartPhase(PhaseInit)
	tr.CompletePhase(PhaseInit)

	report := tr.GetReport()
	if report.OverallProgress != 25 {
		t.Errorf("OverallProgress = %d, want 25", report.OverallProgress)
	}

	// Start second phase at 50% (25 + 25 = 50%)
	tr.StartPhase(PhaseDiscovery)
	tr.UpdatePhase(PhaseDiscovery, 50, "")

	report = tr.GetReport()
	if report.OverallProgress != 50 {
		t.Errorf("OverallProgress = %d, want 50", report.OverallProgress)
	}

	// Complete all
	tr.CompletePhase(PhaseDiscovery)
	tr.StartPhase(PhaseComplete)
	tr.CompletePhase(PhaseComplete)

	report = tr.GetReport()
	if report.OverallProgress != 100 {
		t.Errorf("OverallProgress = %d, want 100", report.OverallProgress)
	}
}

func TestTracker_Callback(t *testing.T) {
	var callbackCount int
	var mu sync.Mutex

	cfg := DefaultConfig()
	cfg.CallbackInterval = 1 * time.Millisecond
	cfg.Callback = func(report *Report) {
		mu.Lock()
		callbackCount++
		mu.Unlock()
	}

	tr := NewTracker(cfg)
	tr.Start()
	tr.StartPhase(PhaseDiscovery)

	if err := helpers.WaitForTimeout(500*time.Millisecond, 10*time.Millisecond, func() (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		return callbackCount > 0, nil
	}); err != nil {
		t.Fatalf("expected callback to be invoked: %v", err)
	}

	mu.Lock()
	count := callbackCount
	mu.Unlock()

	if count < 1 {
		t.Error("Callback should have been called")
	}
}

func TestStats_RecordPhaseTiming(t *testing.T) {
	s := NewStats()

	s.RecordPhaseTiming(PhaseDiscovery, 100*time.Millisecond)
	s.RecordPhaseTiming(PhaseAuthentication, 200*time.Millisecond)

	snapshot := s.Snapshot()

	if snapshot.PhaseTimings[PhaseDiscovery] != 100*time.Millisecond {
		t.Error("PhaseDiscovery timing not recorded")
	}
	if snapshot.PhaseTimings[PhaseAuthentication] != 200*time.Millisecond {
		t.Error("PhaseAuthentication timing not recorded")
	}
}

func TestStats_RecordStepTiming(t *testing.T) {
	s := NewStats()

	s.RecordStepTiming(PhaseInstallation, "download", 500*time.Millisecond)

	snapshot := s.Snapshot()

	key := "installation:download"
	if snapshot.StepTimings[key] != 500*time.Millisecond {
		t.Error("Step timing not recorded")
	}
}

func TestStats_RecordFailure(t *testing.T) {
	s := NewStats()

	s.RecordFailure(PhaseDiscovery)
	s.RecordFailure(PhaseDiscovery)
	s.RecordFailure(PhaseAuthentication)

	snapshot := s.Snapshot()

	if snapshot.Failures[PhaseDiscovery] != 2 {
		t.Errorf("PhaseDiscovery failures = %d, want 2", snapshot.Failures[PhaseDiscovery])
	}
	if snapshot.Failures[PhaseAuthentication] != 1 {
		t.Errorf("PhaseAuthentication failures = %d, want 1", snapshot.Failures[PhaseAuthentication])
	}
}

func TestStats_Snapshot(t *testing.T) {
	s := NewStats()
	s.RecordPhaseTiming(PhaseDiscovery, 100*time.Millisecond)

	snapshot := s.Snapshot()

	// Modify original
	s.RecordPhaseTiming(PhaseAuthentication, 200*time.Millisecond)

	// Snapshot should be unchanged
	if _, ok := snapshot.PhaseTimings[PhaseAuthentication]; ok {
		t.Error("Snapshot should be independent copy")
	}
}

func TestProgressWriter(t *testing.T) {
	tr := NewTracker(nil)
	tr.Start()
	tr.StartPhase(PhaseInstallation)
	tr.StartStep(PhaseInstallation, "download")

	pw := NewProgressWriter(tr, PhaseInstallation, "download", 100)

	// Write 50 bytes
	n, err := pw.Write(make([]byte, 50))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != 50 {
		t.Errorf("Wrote %d bytes, want 50", n)
	}

	report := tr.GetReport()
	step := report.Phases[PhaseInstallation].Steps[0]

	if step.BytesDone != 50 {
		t.Errorf("BytesDone = %d, want 50", step.BytesDone)
	}
	if step.Progress != 50 {
		t.Errorf("Progress = %d, want 50", step.Progress)
	}
}

func TestProgressReader(t *testing.T) {
	tr := NewTracker(nil)
	tr.Start()
	tr.StartPhase(PhaseInstallation)
	tr.StartStep(PhaseInstallation, "download")

	// Mock reader
	data := make([]byte, 100)
	reader := &mockReader{data: data}

	pr := NewProgressReader(tr, PhaseInstallation, "download", 100, reader)

	// Read 50 bytes
	buf := make([]byte, 50)
	n, err := pr.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if n != 50 {
		t.Errorf("Read %d bytes, want 50", n)
	}

	report := tr.GetReport()
	step := report.Phases[PhaseInstallation].Steps[0]

	if step.BytesDone != 50 {
		t.Errorf("BytesDone = %d, want 50", step.BytesDone)
	}
}

type mockReader struct {
	data   []byte
	offset int
}

func (r *mockReader) Read(p []byte) (int, error) {
	if r.offset >= len(r.data) {
		return 0, nil
	}
	n := copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}

func TestWaitForCompletion(t *testing.T) {
	tr := NewTracker(nil)
	tr.Start()

	// Complete in background
	go func() {
		tr.Complete()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := WaitForCompletion(ctx, tr)
	if err != nil {
		t.Errorf("WaitForCompletion failed: %v", err)
	}
}

func TestWaitForCompletion_Failure(t *testing.T) {
	tr := NewTracker(nil)
	tr.Start()

	testErr := errors.New("bootstrap failed")

	// Fail in background
	go func() {
		tr.StartPhase(PhaseDiscovery)
		tr.FailPhase(PhaseDiscovery, testErr)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := WaitForCompletion(ctx, tr)
	if err == nil {
		t.Error("WaitForCompletion should return error on failure")
	}
}

func TestWaitForCompletion_Timeout(t *testing.T) {
	tr := NewTracker(nil)
	tr.Start()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := WaitForCompletion(ctx, tr)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected DeadlineExceeded, got %v", err)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d        time.Duration
		expected string
	}{
		{100 * time.Millisecond, "100ms"},
		{1500 * time.Millisecond, "1.5s"},
		{90 * time.Second, "1m30s"},
	}

	for _, tt := range tests {
		got := FormatDuration(tt.d)
		if got != tt.expected {
			t.Errorf("FormatDuration(%v) = %s, want %s", tt.d, got, tt.expected)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		b        int64
		expected string
	}{
		{500, "500 B"},
		{1024, "1.0 KiB"},
		{1024 * 1024, "1.0 MiB"},
		{1024 * 1024 * 1024, "1.0 GiB"},
	}

	for _, tt := range tests {
		got := FormatBytes(tt.b)
		if got != tt.expected {
			t.Errorf("FormatBytes(%d) = %s, want %s", tt.b, got, tt.expected)
		}
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		value, min, max, expected int
	}{
		{50, 0, 100, 50},
		{-10, 0, 100, 0},
		{150, 0, 100, 100},
		{0, 0, 100, 0},
		{100, 0, 100, 100},
	}

	for _, tt := range tests {
		got := clamp(tt.value, tt.min, tt.max)
		if got != tt.expected {
			t.Errorf("clamp(%d, %d, %d) = %d, want %d", tt.value, tt.min, tt.max, got, tt.expected)
		}
	}
}

func TestEstimateRemaining(t *testing.T) {
	tr := NewTracker(nil)
	tr.Start()

	// At 0% - should return 0
	remaining := tr.estimateRemaining(0, 1*time.Second)
	if remaining != 0 {
		t.Error("Should return 0 at 0%")
	}

	// At 100% - should return 0
	remaining = tr.estimateRemaining(100, 1*time.Second)
	if remaining != 0 {
		t.Error("Should return 0 at 100%")
	}

	// At 50% after 1 second - should estimate ~1 second remaining
	remaining = tr.estimateRemaining(50, 1*time.Second)
	if remaining < 900*time.Millisecond || remaining > 1100*time.Millisecond {
		t.Errorf("Expected ~1s remaining, got %v", remaining)
	}
}

func TestPhaseProgressFromSteps(t *testing.T) {
	tr := NewTracker(nil)
	tr.Start()
	tr.StartPhase(PhaseInstallation)

	// Add 4 steps
	tr.StartStep(PhaseInstallation, "step1")
	tr.StartStep(PhaseInstallation, "step2")
	tr.StartStep(PhaseInstallation, "step3")
	tr.StartStep(PhaseInstallation, "step4")

	// Complete 2 of 4 steps
	tr.CompleteStep(PhaseInstallation, "step1")
	tr.CompleteStep(PhaseInstallation, "step2")

	report := tr.GetReport()
	info := report.Phases[PhaseInstallation]

	// Should be 50% (2/4 steps)
	if info.Progress != 50 {
		t.Errorf("Phase progress = %d, want 50", info.Progress)
	}
}
