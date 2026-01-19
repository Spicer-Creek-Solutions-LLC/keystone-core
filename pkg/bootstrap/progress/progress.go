// Package progress provides progress reporting and performance tracking for bootstrap operations.
package progress

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Phase represents a bootstrap phase
type Phase string

const (
	// PhaseInit is the initialization phase
	PhaseInit Phase = "init"
	// PhaseDiscovery is the server discovery phase
	PhaseDiscovery Phase = "discovery"
	// PhaseAuthentication is the authentication phase
	PhaseAuthentication Phase = "authentication"
	// PhaseConfiguration is the configuration download phase
	PhaseConfiguration Phase = "configuration"
	// PhaseInstallation is the agent installation phase
	PhaseInstallation Phase = "installation"
	// PhaseVerification is the verification phase
	PhaseVerification Phase = "verification"
	// PhaseComplete indicates bootstrap is complete
	PhaseComplete Phase = "complete"
	// PhaseFailed indicates bootstrap failed
	PhaseFailed Phase = "failed"
)

// Status represents the status of a phase or operation
type Status string

const (
	// StatusPending indicates not yet started
	StatusPending Status = "pending"
	// StatusRunning indicates in progress
	StatusRunning Status = "running"
	// StatusComplete indicates successfully completed
	StatusComplete Status = "complete"
	// StatusFailed indicates failure
	StatusFailed Status = "failed"
	// StatusSkipped indicates the phase was skipped
	StatusSkipped Status = "skipped"
)

// PhaseInfo contains information about a bootstrap phase
type PhaseInfo struct {
	// Phase is the phase identifier
	Phase Phase

	// Status is the current status
	Status Status

	// Progress is the progress percentage (0-100)
	Progress int

	// Message is a human-readable status message
	Message string

	// StartedAt is when the phase started
	StartedAt time.Time

	// CompletedAt is when the phase completed
	CompletedAt time.Time

	// Duration is how long the phase took
	Duration time.Duration

	// Error contains any error that occurred
	Error error

	// Steps tracks individual steps within the phase
	Steps []*StepInfo
}

// StepInfo contains information about a step within a phase
type StepInfo struct {
	// Name is the step name
	Name string

	// Status is the current status
	Status Status

	// Progress is the progress percentage (0-100)
	Progress int

	// Message is a human-readable status message
	Message string

	// StartedAt is when the step started
	StartedAt time.Time

	// CompletedAt is when the step completed
	CompletedAt time.Time

	// Duration is how long the step took
	Duration time.Duration

	// BytesTotal is the total bytes for download steps
	BytesTotal int64

	// BytesDone is the bytes completed for download steps
	BytesDone int64

	// Error contains any error that occurred
	Error error
}

// Callback is called when progress is updated
type Callback func(report *Report)

// Report contains the current bootstrap progress
type Report struct {
	// StartedAt is when bootstrap started
	StartedAt time.Time

	// CurrentPhase is the current phase
	CurrentPhase Phase

	// OverallProgress is the overall progress percentage (0-100)
	OverallProgress int

	// Phases contains info for all phases
	Phases map[Phase]*PhaseInfo

	// ElapsedTime is how long bootstrap has been running
	ElapsedTime time.Duration

	// EstimatedRemaining is the estimated time remaining
	EstimatedRemaining time.Duration
}

// Config configures the progress tracker
type Config struct {
	// Phases is the ordered list of phases to track
	Phases []Phase

	// PhaseWeights maps phases to their weight in overall progress
	PhaseWeights map[Phase]int

	// Callback is called when progress is updated
	Callback Callback

	// CallbackInterval is the minimum interval between callbacks
	CallbackInterval time.Duration

	// EnableTiming enables detailed timing collection
	EnableTiming bool
}

// DefaultConfig returns a default progress configuration
func DefaultConfig() *Config {
	return &Config{
		Phases: []Phase{
			PhaseInit,
			PhaseDiscovery,
			PhaseAuthentication,
			PhaseConfiguration,
			PhaseInstallation,
			PhaseVerification,
		},
		PhaseWeights: map[Phase]int{
			PhaseInit:           5,
			PhaseDiscovery:      10,
			PhaseAuthentication: 15,
			PhaseConfiguration:  20,
			PhaseInstallation:   35,
			PhaseVerification:   15,
		},
		CallbackInterval: 100 * time.Millisecond,
		EnableTiming:     true,
	}
}

// Tracker tracks bootstrap progress
type Tracker struct {
	config        *Config
	mu            sync.RWMutex
	startedAt     time.Time
	currentPhase  Phase
	phases        map[Phase]*PhaseInfo
	phaseOrder    []Phase
	lastCallback  time.Time
	stats         *Stats
}

// NewTracker creates a new progress tracker
func NewTracker(config *Config) *Tracker {
	if config == nil {
		config = DefaultConfig()
	}

	t := &Tracker{
		config:     config,
		phases:     make(map[Phase]*PhaseInfo),
		phaseOrder: config.Phases,
		stats:      NewStats(),
	}

	// Initialize phases
	for _, phase := range config.Phases {
		t.phases[phase] = &PhaseInfo{
			Phase:  phase,
			Status: StatusPending,
			Steps:  make([]*StepInfo, 0),
		}
	}

	return t
}

// Start starts the bootstrap progress tracking
func (t *Tracker) Start() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.startedAt = time.Now()
	if len(t.phaseOrder) > 0 {
		t.currentPhase = t.phaseOrder[0]
	}
	t.stats.StartedAt = t.startedAt
}

// StartPhase starts a new phase
func (t *Tracker) StartPhase(phase Phase) {
	t.mu.Lock()
	defer t.mu.Unlock()

	info, ok := t.phases[phase]
	if !ok {
		info = &PhaseInfo{
			Phase: phase,
			Steps: make([]*StepInfo, 0),
		}
		t.phases[phase] = info
	}

	info.Status = StatusRunning
	info.StartedAt = time.Now()
	info.Progress = 0
	t.currentPhase = phase

	t.notifyCallback()
}

// UpdatePhase updates phase progress
func (t *Tracker) UpdatePhase(phase Phase, progress int, message string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	info, ok := t.phases[phase]
	if !ok {
		return
	}

	info.Progress = clamp(progress, 0, 100)
	info.Message = message

	t.notifyCallback()
}

// CompletePhase marks a phase as complete
func (t *Tracker) CompletePhase(phase Phase) {
	t.mu.Lock()
	defer t.mu.Unlock()

	info, ok := t.phases[phase]
	if !ok {
		return
	}

	info.Status = StatusComplete
	info.Progress = 100
	info.CompletedAt = time.Now()
	info.Duration = info.CompletedAt.Sub(info.StartedAt)

	// Record timing
	t.stats.RecordPhaseTiming(phase, info.Duration)

	t.notifyCallback()
}

// FailPhase marks a phase as failed
func (t *Tracker) FailPhase(phase Phase, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	info, ok := t.phases[phase]
	if !ok {
		return
	}

	info.Status = StatusFailed
	info.CompletedAt = time.Now()
	info.Duration = info.CompletedAt.Sub(info.StartedAt)
	info.Error = err
	t.currentPhase = PhaseFailed

	t.stats.RecordPhaseTiming(phase, info.Duration)
	t.stats.RecordFailure(phase)

	t.notifyCallback()
}

// SkipPhase marks a phase as skipped
func (t *Tracker) SkipPhase(phase Phase, reason string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	info, ok := t.phases[phase]
	if !ok {
		return
	}

	info.Status = StatusSkipped
	info.Message = reason
	info.CompletedAt = time.Now()

	t.notifyCallback()
}

// StartStep starts a new step within a phase
func (t *Tracker) StartStep(phase Phase, name string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	info, ok := t.phases[phase]
	if !ok {
		return
	}

	step := &StepInfo{
		Name:      name,
		Status:    StatusRunning,
		StartedAt: time.Now(),
	}

	info.Steps = append(info.Steps, step)
	t.notifyCallback()
}

// UpdateStep updates step progress
func (t *Tracker) UpdateStep(phase Phase, name string, progress int, message string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	step := t.findStep(phase, name)
	if step == nil {
		return
	}

	step.Progress = clamp(progress, 0, 100)
	step.Message = message

	t.notifyCallback()
}

// UpdateStepBytes updates step progress with byte counts (for downloads)
func (t *Tracker) UpdateStepBytes(phase Phase, name string, done, total int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	step := t.findStep(phase, name)
	if step == nil {
		return
	}

	step.BytesDone = done
	step.BytesTotal = total
	if total > 0 {
		step.Progress = int(done * 100 / total)
	}

	t.notifyCallback()
}

// CompleteStep marks a step as complete
func (t *Tracker) CompleteStep(phase Phase, name string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	step := t.findStep(phase, name)
	if step == nil {
		return
	}

	step.Status = StatusComplete
	step.Progress = 100
	step.CompletedAt = time.Now()
	step.Duration = step.CompletedAt.Sub(step.StartedAt)

	t.stats.RecordStepTiming(phase, name, step.Duration)

	// Update phase progress based on steps
	t.updatePhaseProgressFromSteps(phase)

	t.notifyCallback()
}

// FailStep marks a step as failed
func (t *Tracker) FailStep(phase Phase, name string, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	step := t.findStep(phase, name)
	if step == nil {
		return
	}

	step.Status = StatusFailed
	step.CompletedAt = time.Now()
	step.Duration = step.CompletedAt.Sub(step.StartedAt)
	step.Error = err

	t.stats.RecordStepTiming(phase, name, step.Duration)

	t.notifyCallback()
}

func (t *Tracker) findStep(phase Phase, name string) *StepInfo {
	info, ok := t.phases[phase]
	if !ok {
		return nil
	}

	for _, step := range info.Steps {
		if step.Name == name {
			return step
		}
	}
	return nil
}

func (t *Tracker) updatePhaseProgressFromSteps(phase Phase) {
	info, ok := t.phases[phase]
	if !ok || len(info.Steps) == 0 {
		return
	}

	var total, completed int
	for _, step := range info.Steps {
		total++
		if step.Status == StatusComplete {
			completed++
		}
	}

	info.Progress = completed * 100 / total
}

// Complete marks the entire bootstrap as complete
func (t *Tracker) Complete() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.currentPhase = PhaseComplete
	t.stats.CompletedAt = time.Now()
	t.stats.TotalDuration = t.stats.CompletedAt.Sub(t.startedAt)
	t.stats.Success = true

	t.notifyCallback()
}

// GetReport returns the current progress report
func (t *Tracker) GetReport() *Report {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.buildReport()
}

func (t *Tracker) buildReport() *Report {
	report := &Report{
		StartedAt:       t.startedAt,
		CurrentPhase:    t.currentPhase,
		Phases:          make(map[Phase]*PhaseInfo),
		ElapsedTime:     time.Since(t.startedAt),
	}

	// Copy phase info
	for phase, info := range t.phases {
		report.Phases[phase] = &PhaseInfo{
			Phase:       info.Phase,
			Status:      info.Status,
			Progress:    info.Progress,
			Message:     info.Message,
			StartedAt:   info.StartedAt,
			CompletedAt: info.CompletedAt,
			Duration:    info.Duration,
			Error:       info.Error,
			Steps:       make([]*StepInfo, len(info.Steps)),
		}
		for i, step := range info.Steps {
			report.Phases[phase].Steps[i] = &StepInfo{
				Name:        step.Name,
				Status:      step.Status,
				Progress:    step.Progress,
				Message:     step.Message,
				StartedAt:   step.StartedAt,
				CompletedAt: step.CompletedAt,
				Duration:    step.Duration,
				BytesTotal:  step.BytesTotal,
				BytesDone:   step.BytesDone,
				Error:       step.Error,
			}
		}
	}

	// Calculate overall progress
	report.OverallProgress = t.calculateOverallProgress()

	// Estimate remaining time
	report.EstimatedRemaining = t.estimateRemaining(report.OverallProgress, report.ElapsedTime)

	return report
}

func (t *Tracker) calculateOverallProgress() int {
	var totalWeight, completedWeight int

	for _, phase := range t.phaseOrder {
		weight := t.config.PhaseWeights[phase]
		if weight == 0 {
			weight = 1
		}
		totalWeight += weight

		info, ok := t.phases[phase]
		if !ok {
			continue
		}

		switch info.Status {
		case StatusComplete, StatusSkipped:
			completedWeight += weight
		case StatusRunning:
			completedWeight += weight * info.Progress / 100
		}
	}

	if totalWeight == 0 {
		return 0
	}

	return completedWeight * 100 / totalWeight
}

func (t *Tracker) estimateRemaining(progress int, elapsed time.Duration) time.Duration {
	if progress <= 0 {
		return 0
	}
	if progress >= 100 {
		return 0
	}

	// Simple linear estimation
	totalEstimate := elapsed * time.Duration(100) / time.Duration(progress)
	remaining := totalEstimate - elapsed

	if remaining < 0 {
		return 0
	}

	return remaining
}

func (t *Tracker) notifyCallback() {
	if t.config.Callback == nil {
		return
	}

	// Throttle callbacks
	if time.Since(t.lastCallback) < t.config.CallbackInterval {
		return
	}

	t.lastCallback = time.Now()
	report := t.buildReport()

	// Call callback without lock
	go t.config.Callback(report)
}

// Stats returns the performance statistics
func (t *Tracker) Stats() *Stats {
	return t.stats
}

// Stats tracks bootstrap performance statistics
type Stats struct {
	mu            sync.Mutex
	StartedAt     time.Time
	CompletedAt   time.Time
	TotalDuration time.Duration
	Success       bool
	PhaseTimings  map[Phase]time.Duration
	StepTimings   map[string]time.Duration
	Failures      map[Phase]int
}

// NewStats creates a new stats tracker
func NewStats() *Stats {
	return &Stats{
		PhaseTimings: make(map[Phase]time.Duration),
		StepTimings:  make(map[string]time.Duration),
		Failures:     make(map[Phase]int),
	}
}

// RecordPhaseTiming records the timing for a phase
func (s *Stats) RecordPhaseTiming(phase Phase, duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PhaseTimings[phase] = duration
}

// RecordStepTiming records the timing for a step
func (s *Stats) RecordStepTiming(phase Phase, step string, duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := fmt.Sprintf("%s:%s", phase, step)
	s.StepTimings[key] = duration
}

// RecordFailure records a phase failure
func (s *Stats) RecordFailure(phase Phase) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Failures[phase]++
}

// Snapshot returns a copy of the current stats
func (s *Stats) Snapshot() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshot := Stats{
		StartedAt:     s.StartedAt,
		CompletedAt:   s.CompletedAt,
		TotalDuration: s.TotalDuration,
		Success:       s.Success,
		PhaseTimings:  make(map[Phase]time.Duration),
		StepTimings:   make(map[string]time.Duration),
		Failures:      make(map[Phase]int),
	}

	for k, v := range s.PhaseTimings {
		snapshot.PhaseTimings[k] = v
	}
	for k, v := range s.StepTimings {
		snapshot.StepTimings[k] = v
	}
	for k, v := range s.Failures {
		snapshot.Failures[k] = v
	}

	return snapshot
}

// ProgressWriter wraps an io.Writer to track progress
type ProgressWriter struct {
	tracker  *Tracker
	phase    Phase
	stepName string
	total    int64
	written  int64
}

// NewProgressWriter creates a new progress writer
func NewProgressWriter(tracker *Tracker, phase Phase, stepName string, total int64) *ProgressWriter {
	return &ProgressWriter{
		tracker:  tracker,
		phase:    phase,
		stepName: stepName,
		total:    total,
	}
}

// Write implements io.Writer
func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.written += int64(n)
	pw.tracker.UpdateStepBytes(pw.phase, pw.stepName, pw.written, pw.total)
	return n, nil
}

// ProgressReader wraps an io.Reader to track progress
type ProgressReader struct {
	tracker  *Tracker
	phase    Phase
	stepName string
	total    int64
	read     int64
	reader   interface{ Read([]byte) (int, error) }
}

// NewProgressReader creates a new progress reader
func NewProgressReader(tracker *Tracker, phase Phase, stepName string, total int64, reader interface{ Read([]byte) (int, error) }) *ProgressReader {
	return &ProgressReader{
		tracker:  tracker,
		phase:    phase,
		stepName: stepName,
		total:    total,
		reader:   reader,
	}
}

// Read implements io.Reader
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.read += int64(n)
	pr.tracker.UpdateStepBytes(pr.phase, pr.stepName, pr.read, pr.total)
	return n, err
}

// WaitForCompletion waits for bootstrap to complete or context to cancel
func WaitForCompletion(ctx context.Context, tracker *Tracker) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			report := tracker.GetReport()
			if report.CurrentPhase == PhaseComplete {
				return nil
			}
			if report.CurrentPhase == PhaseFailed {
				// Find the failed phase error
				for _, info := range report.Phases {
					if info.Status == StatusFailed && info.Error != nil {
						return info.Error
					}
				}
				return fmt.Errorf("bootstrap failed")
			}
		}
	}
}

// FormatDuration formats a duration for display
func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}

// FormatBytes formats bytes for display
func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
