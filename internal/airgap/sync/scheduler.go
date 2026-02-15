package sync

import (
	"context"
	"fmt"
	gosync "sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/schedule"
	"github.com/shawnbutts/keystone-core/pkg/statemachine"
	"github.com/shawnbutts/keystone-core/pkg/wait"
)

// OperationFunc is called to execute a sync operation. It receives the operation
// config and bandwidth limiter, and should return an error if the operation fails.
type OperationFunc func(ctx context.Context, op OperationConfig, limiter *BandwidthLimiter) error

// windowRunner tracks the runtime state of a single sync window.
type windowRunner struct {
	config   WindowConfig
	machine  *statemachine.Machine[State, Event]
	limiter  *BandwidthLimiter
	progress Progress
	cancel   context.CancelFunc
}

// Scheduler manages sync windows with cron-based scheduling and state machines.
type Scheduler struct {
	mu      gosync.RWMutex
	windows map[string]*windowRunner
	history []Record
	maxHist int

	cronParser *schedule.CronParser
	opFunc     OperationFunc

	ctx    context.Context
	cancel context.CancelFunc
	wg     gosync.WaitGroup
	closed bool
}

// NewScheduler creates a scheduler. The opFunc is called to execute each operation
// during a sync window. If nil, operations are no-ops.
func NewScheduler(opFunc OperationFunc) *Scheduler {
	return &Scheduler{
		windows:    make(map[string]*windowRunner),
		maxHist:    100,
		cronParser: schedule.NewCronParser(),
		opFunc:     opFunc,
	}
}

func buildSyncMachine(name string) *statemachine.Machine[State, Event] {
	return statemachine.New[State, Event](StateIdle).
		WithName("sync-window-" + name).
		WithHistory(50).
		AddTransition(StateIdle, EventSchedule, StateScheduled).
		AddTransition(StateScheduled, EventStart, StateRunning).
		AddTransition(StateScheduled, EventCancel, StateCancelled).
		AddTransition(StateRunning, EventPause, StatePaused).
		AddTransition(StateRunning, EventComplete, StateCompleted).
		AddTransition(StateRunning, EventFail, StateFailed).
		AddTransition(StateRunning, EventCancel, StateCancelled).
		AddTransition(StatePaused, EventResume, StateRunning).
		AddTransition(StatePaused, EventCancel, StateCancelled).
		AddTransition(StateCompleted, EventReset, StateIdle).
		AddTransition(StateFailed, EventReset, StateIdle).
		AddTransition(StateCancelled, EventReset, StateIdle).
		MustBuild()
}

// AddWindow registers a new sync window configuration.
func (s *Scheduler) AddWindow(config WindowConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrSchedulerClosed
	}
	if _, exists := s.windows[config.Name]; exists {
		return fmt.Errorf("%w: %s", ErrWindowExists, config.Name)
	}

	s.windows[config.Name] = &windowRunner{
		config:  config,
		machine: buildSyncMachine(config.Name),
		limiter: NewBandwidthLimiter(config.BandwidthLimit),
	}
	return nil
}

// RemoveWindow unregisters a sync window. Returns an error if the window is running.
func (s *Scheduler) RemoveWindow(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	w, ok := s.windows[name]
	if !ok {
		return fmt.Errorf("%w: %s", ErrWindowNotFound, name)
	}
	state := w.machine.State()
	if state == StateRunning || state == StatePaused {
		return fmt.Errorf("cannot remove window %q while in state %s", name, state)
	}
	delete(s.windows, name)
	return nil
}

// ListWindows returns the names and enabled status of all registered windows.
func (s *Scheduler) ListWindows() []WindowConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	configs := make([]WindowConfig, 0, len(s.windows))
	for _, w := range s.windows {
		configs = append(configs, w.config)
	}
	return configs
}

// Start begins the scheduler's background loop. It checks windows every 30 seconds.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrSchedulerClosed
	}
	if s.ctx != nil {
		s.mu.Unlock()
		return nil
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.mu.Unlock()

	s.wg.Add(1)
	go s.loop()
	return nil
}

func (s *Scheduler) loop() {
	defer s.wg.Done()

	for {
		s.checkWindows()
		if err := wait.ForContext(s.ctx, 30*time.Second); err != nil {
			return
		}
	}
}

func (s *Scheduler) checkWindows() {
	s.mu.RLock()
	runners := make([]*windowRunner, 0, len(s.windows))
	for _, w := range s.windows {
		runners = append(runners, w)
	}
	s.mu.RUnlock()

	now := time.Now()
	for _, w := range runners {
		if !w.config.Enabled {
			continue
		}
		state := w.machine.State()
		if state != StateIdle && state != StateCompleted && state != StateFailed && state != StateCancelled {
			continue
		}

		// Reset terminal states back to idle
		if state == StateCompleted || state == StateFailed || state == StateCancelled {
			if err := w.machine.FireCtx(s.ctx, EventReset); err != nil {
				continue
			}
		}

		nextRun, err := s.cronParser.NextTimeInTimezone(w.config.CronSchedule, now.Add(-w.config.Duration), w.config.Timezone)
		if err != nil {
			continue
		}
		if nextRun != nil && !nextRun.After(now) {
			s.startWindow(s.ctx, w)
		}
	}
}

func (s *Scheduler) startWindow(ctx context.Context, w *windowRunner) {
	if err := w.machine.FireCtx(ctx, EventSchedule); err != nil {
		return
	}
	if err := w.machine.FireCtx(ctx, EventStart); err != nil {
		return
	}

	winCtx, winCancel := context.WithTimeout(ctx, w.config.Duration)
	w.cancel = winCancel

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer winCancel()
		s.executeWindow(winCtx, w)
	}()
}

func (s *Scheduler) executeWindow(ctx context.Context, w *windowRunner) {
	startedAt := time.Now()
	ops := sortedOperations(w.config.Operations)

	var execErr error
	opsRun := 0
	for _, op := range ops {
		if ctx.Err() != nil {
			break
		}
		// Check for pause
		if w.machine.State() == StatePaused {
			if err := wait.ForCondition(ctx, 500*time.Millisecond, func() (bool, error) {
				return w.machine.State() != StatePaused, nil
			}); err != nil {
				break
			}
			if w.machine.State() == StateCancelled {
				break
			}
		}

		if s.opFunc != nil {
			if err := s.opFunc(ctx, op, w.limiter); err != nil {
				execErr = err
				break
			}
		}
		opsRun++
	}

	endedAt := time.Now()
	state := w.machine.State()

	switch {
	case state == StateCancelled:
		// Already cancelled via Cancel()
	case execErr != nil || ctx.Err() != nil:
		_ = w.machine.FireCtx(ctx, EventFail)
	default:
		_ = w.machine.FireCtx(ctx, EventComplete)
	}

	record := Record{
		WindowName: w.config.Name,
		StartedAt:  startedAt,
		EndedAt:    endedAt,
		Duration:   endedAt.Sub(startedAt),
		State:      w.machine.State(),
		Operations: opsRun,
	}
	if execErr != nil {
		record.Error = execErr.Error()
	}
	s.addHistory(record)
}

func (s *Scheduler) addHistory(record Record) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.history = append(s.history, record)
	if len(s.history) > s.maxHist {
		s.history = s.history[len(s.history)-s.maxHist:]
	}
}

// TriggerNow manually triggers a sync window regardless of its cron schedule.
func (s *Scheduler) TriggerNow(ctx context.Context, name string) error {
	s.mu.RLock()
	w, ok := s.windows[name]
	if !ok {
		s.mu.RUnlock()
		return fmt.Errorf("%w: %s", ErrWindowNotFound, name)
	}
	s.mu.RUnlock()

	state := w.machine.State()
	if state == StateCompleted || state == StateFailed || state == StateCancelled {
		if err := w.machine.FireCtx(ctx, EventReset); err != nil {
			return err
		}
	}

	if w.machine.State() != StateIdle {
		return fmt.Errorf("window %q is in state %s, must be idle to trigger", name, w.machine.State())
	}

	s.startWindow(ctx, w)
	return nil
}

// Pause pauses a running sync window.
func (s *Scheduler) Pause(name string) error {
	s.mu.RLock()
	w, ok := s.windows[name]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("%w: %s", ErrWindowNotFound, name)
	}
	return w.machine.Fire(EventPause)
}

// Resume resumes a paused sync window.
func (s *Scheduler) Resume(name string) error {
	s.mu.RLock()
	w, ok := s.windows[name]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("%w: %s", ErrWindowNotFound, name)
	}
	return w.machine.Fire(EventResume)
}

// Cancel cancels a running or paused sync window.
func (s *Scheduler) Cancel(name string) error {
	s.mu.RLock()
	w, ok := s.windows[name]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("%w: %s", ErrWindowNotFound, name)
	}
	if err := w.machine.Fire(EventCancel); err != nil {
		return err
	}
	if w.cancel != nil {
		w.cancel()
	}
	return nil
}

// GetStatus returns the current status of a sync window.
func (s *Scheduler) GetStatus(name string) (*WindowStatus, error) {
	s.mu.RLock()
	w, ok := s.windows[name]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrWindowNotFound, name)
	}

	status := &WindowStatus{
		Name:  name,
		State: w.machine.State(),
	}

	if w.config.Enabled {
		nextRun, err := s.cronParser.NextTimeInTimezone(w.config.CronSchedule, time.Now(), w.config.Timezone)
		if err == nil {
			status.NextRun = nextRun
		}
	}

	state := w.machine.State()
	if state == StateRunning || state == StatePaused {
		p := w.progress
		status.Progress = &p
	}

	return status, nil
}

// GetProgress returns the sync progress for a running window.
func (s *Scheduler) GetProgress(name string) (*Progress, error) {
	s.mu.RLock()
	w, ok := s.windows[name]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrWindowNotFound, name)
	}
	p := w.progress
	return &p, nil
}

// History returns sync execution history, newest first.
func (s *Scheduler) History() []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()

	records := make([]Record, len(s.history))
	for i, r := range s.history {
		records[len(s.history)-1-i] = r
	}
	return records
}

// Close shuts down the scheduler and waits for running windows to finish.
func (s *Scheduler) Close() {
	s.mu.Lock()
	s.closed = true
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
}
