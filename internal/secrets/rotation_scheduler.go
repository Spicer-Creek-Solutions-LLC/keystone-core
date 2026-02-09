package secrets

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ScheduledRotation represents a scheduled rotation configuration.
type ScheduledRotation struct {
	// ID is the unique identifier for this scheduled rotation.
	ID string `json:"id"`

	// SecretPath is the path of the secret to rotate.
	SecretPath string `json:"secret_path"`

	// Config is the rotation configuration.
	Config *RotationConfig `json:"config"`

	// Targets are the rotation targets.
	Targets []*RotationTarget `json:"targets"`

	// Schedule is the cron expression for scheduling.
	// Format: minute hour day-of-month month day-of-week
	// Example: "0 2 * * *" = every day at 2:00 AM
	Schedule string `json:"schedule"`

	// Enabled indicates whether this schedule is active.
	Enabled bool `json:"enabled"`

	// LastRun is when this rotation last ran.
	LastRun time.Time `json:"last_run,omitempty"`

	// NextRun is when this rotation will next run.
	NextRun time.Time `json:"next_run,omitempty"`

	// LastResult stores the result of the last rotation.
	LastResult *RotationResult `json:"last_result,omitempty"`

	// CreatedAt is when this schedule was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when this schedule was last updated.
	UpdatedAt time.Time `json:"updated_at"`
}

// RotationResult stores the result of a rotation execution.
type RotationResult struct {
	// RotationID is the ID of the rotation that ran.
	RotationID string `json:"rotation_id"`

	// Success indicates whether the rotation completed successfully.
	Success bool `json:"success"`

	// Error is any error that occurred.
	Error string `json:"error,omitempty"`

	// Duration is how long the rotation took.
	Duration time.Duration `json:"duration"`

	// UpdatedTargets is the number of successfully updated targets.
	UpdatedTargets int `json:"updated_targets"`

	// FailedTargets is the number of failed targets.
	FailedTargets int `json:"failed_targets"`

	// Timestamp is when the rotation completed.
	Timestamp time.Time `json:"timestamp"`
}

// RotationScheduler manages scheduled rotations.
type RotationScheduler struct {
	mu sync.RWMutex

	// orchestrator is the rotation orchestrator.
	orchestrator *RotationOrchestrator

	// schedules maps schedule ID to scheduled rotation.
	schedules map[string]*ScheduledRotation

	// ctx is the scheduler context.
	ctx context.Context

	// cancel cancels the scheduler.
	cancel context.CancelFunc

	// running indicates if the scheduler is running.
	running bool

	// checkInterval is how often to check for due rotations.
	checkInterval time.Duration

	// onRotationStart is called when a scheduled rotation starts.
	onRotationStart func(schedule *ScheduledRotation)

	// onRotationComplete is called when a scheduled rotation completes.
	onRotationComplete func(schedule *ScheduledRotation, result *RotationResult)
}

// NewRotationScheduler creates a new rotation scheduler.
func NewRotationScheduler(orchestrator *RotationOrchestrator) *RotationScheduler {
	return &RotationScheduler{
		orchestrator:  orchestrator,
		schedules:     make(map[string]*ScheduledRotation),
		checkInterval: time.Minute,
	}
}

// SetCheckInterval sets the interval for checking due rotations.
func (rs *RotationScheduler) SetCheckInterval(interval time.Duration) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.checkInterval = interval
}

// SetCallbacks sets the scheduler callbacks.
func (rs *RotationScheduler) SetCallbacks(
	onStart func(*ScheduledRotation),
	onComplete func(*ScheduledRotation, *RotationResult),
) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.onRotationStart = onStart
	rs.onRotationComplete = onComplete
}

// AddSchedule adds a new scheduled rotation.
func (rs *RotationScheduler) AddSchedule(schedule *ScheduledRotation) error {
	if schedule.ID == "" {
		return fmt.Errorf("schedule ID is required")
	}
	if schedule.SecretPath == "" {
		return fmt.Errorf("secret path is required")
	}
	if schedule.Schedule == "" {
		return fmt.Errorf("cron schedule is required")
	}

	// Validate cron expression
	if _, err := ParseCron(schedule.Schedule); err != nil {
		return fmt.Errorf("invalid cron expression: %w", err)
	}

	rs.mu.Lock()
	defer rs.mu.Unlock()

	if _, exists := rs.schedules[schedule.ID]; exists {
		return fmt.Errorf("schedule %s already exists", schedule.ID)
	}

	now := time.Now()
	schedule.CreatedAt = now
	schedule.UpdatedAt = now

	// Calculate next run time
	nextRun, err := NextCronRun(schedule.Schedule, now)
	if err == nil {
		schedule.NextRun = nextRun
	}

	rs.schedules[schedule.ID] = schedule
	return nil
}

// UpdateSchedule updates an existing scheduled rotation.
func (rs *RotationScheduler) UpdateSchedule(schedule *ScheduledRotation) error {
	if schedule.ID == "" {
		return fmt.Errorf("schedule ID is required")
	}

	if schedule.Schedule != "" {
		if _, err := ParseCron(schedule.Schedule); err != nil {
			return fmt.Errorf("invalid cron expression: %w", err)
		}
	}

	rs.mu.Lock()
	defer rs.mu.Unlock()

	existing, ok := rs.schedules[schedule.ID]
	if !ok {
		return fmt.Errorf("schedule %s not found", schedule.ID)
	}

	// Preserve creation time and merge updates
	schedule.CreatedAt = existing.CreatedAt
	schedule.UpdatedAt = time.Now()

	// Recalculate next run if schedule changed
	if schedule.Schedule != existing.Schedule {
		nextRun, err := NextCronRun(schedule.Schedule, time.Now())
		if err == nil {
			schedule.NextRun = nextRun
		}
	} else {
		schedule.NextRun = existing.NextRun
	}

	rs.schedules[schedule.ID] = schedule
	return nil
}

// RemoveSchedule removes a scheduled rotation.
func (rs *RotationScheduler) RemoveSchedule(id string) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if _, ok := rs.schedules[id]; !ok {
		return fmt.Errorf("schedule %s not found", id)
	}

	delete(rs.schedules, id)
	return nil
}

// GetSchedule returns a scheduled rotation by ID.
func (rs *RotationScheduler) GetSchedule(id string) (*ScheduledRotation, bool) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	schedule, ok := rs.schedules[id]
	return schedule, ok
}

// ListSchedules returns all scheduled rotations.
func (rs *RotationScheduler) ListSchedules() []*ScheduledRotation {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	schedules := make([]*ScheduledRotation, 0, len(rs.schedules))
	for _, s := range rs.schedules {
		schedules = append(schedules, s)
	}

	// Sort by next run time
	sort.Slice(schedules, func(i, j int) bool {
		return schedules[i].NextRun.Before(schedules[j].NextRun)
	})

	return schedules
}

// EnableSchedule enables a scheduled rotation.
func (rs *RotationScheduler) EnableSchedule(id string) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	schedule, ok := rs.schedules[id]
	if !ok {
		return fmt.Errorf("schedule %s not found", id)
	}

	schedule.Enabled = true
	schedule.UpdatedAt = time.Now()

	// Recalculate next run
	nextRun, err := NextCronRun(schedule.Schedule, time.Now())
	if err == nil {
		schedule.NextRun = nextRun
	}

	return nil
}

// DisableSchedule disables a scheduled rotation.
func (rs *RotationScheduler) DisableSchedule(id string) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	schedule, ok := rs.schedules[id]
	if !ok {
		return fmt.Errorf("schedule %s not found", id)
	}

	schedule.Enabled = false
	schedule.UpdatedAt = time.Now()
	return nil
}

// Start starts the scheduler.
func (rs *RotationScheduler) Start() error {
	rs.mu.Lock()
	if rs.running {
		rs.mu.Unlock()
		return fmt.Errorf("scheduler already running")
	}

	rs.ctx, rs.cancel = context.WithCancel(context.Background())
	rs.running = true
	rs.mu.Unlock()

	go rs.run()
	return nil
}

// Stop stops the scheduler.
func (rs *RotationScheduler) Stop() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if !rs.running {
		return
	}

	rs.cancel()
	rs.running = false
}

// IsRunning returns whether the scheduler is running.
func (rs *RotationScheduler) IsRunning() bool {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.running
}

// run is the main scheduler loop.
func (rs *RotationScheduler) run() {
	ticker := time.NewTicker(rs.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rs.ctx.Done():
			return
		case <-ticker.C:
			rs.checkDueRotations()
		}
	}
}

// checkDueRotations checks for and triggers any due rotations.
func (rs *RotationScheduler) checkDueRotations() {
	rs.mu.RLock()
	now := time.Now()
	var dueSchedules []*ScheduledRotation

	for _, schedule := range rs.schedules {
		if schedule.Enabled && !schedule.NextRun.IsZero() && schedule.NextRun.Before(now) {
			dueSchedules = append(dueSchedules, schedule)
		}
	}
	rs.mu.RUnlock()

	// Trigger due rotations
	for _, schedule := range dueSchedules {
		go rs.triggerRotation(schedule)
	}
}

// triggerRotation triggers a scheduled rotation.
func (rs *RotationScheduler) triggerRotation(schedule *ScheduledRotation) {
	// Update last run time and calculate next run
	rs.mu.Lock()
	schedule.LastRun = time.Now()
	nextRun, err := NextCronRun(schedule.Schedule, schedule.LastRun)
	if err == nil {
		schedule.NextRun = nextRun
	}
	rs.mu.Unlock()

	// Call start callback
	if rs.onRotationStart != nil {
		rs.onRotationStart(schedule)
	}

	// Generate rotation ID
	rotationID := fmt.Sprintf("%s-%d", schedule.ID, time.Now().UnixNano())

	startTime := time.Now()

	// Start the rotation
	rotation, err := rs.orchestrator.StartRotation(
		rs.ctx,
		rotationID,
		schedule.SecretPath,
		schedule.Config,
		schedule.Targets,
	)

	result := &RotationResult{
		RotationID: rotationID,
		Timestamp:  time.Now(),
	}

	if err != nil {
		result.Success = false
		result.Error = err.Error()
		result.Duration = time.Since(startTime)
	} else {
		// Wait for rotation to complete
		for !rotation.IsTerminal() {
			select {
			case <-rs.ctx.Done():
				result.Success = false
				result.Error = "scheduler stopped"
				result.Duration = time.Since(startTime)
				goto done
			default:
				time.Sleep(100 * time.Millisecond)
			}
		}

		result.Duration = time.Since(startTime)
		result.Success = rotation.IsComplete()
		if !result.Success && rotation.Error() != nil {
			result.Error = rotation.Error().Error()
		}

		progress := rotation.GetProgress()
		result.UpdatedTargets = progress.UpdatedTargets
		result.FailedTargets = progress.FailedTargets
	}

done:
	// Store result
	rs.mu.Lock()
	schedule.LastResult = result
	schedule.UpdatedAt = time.Now()
	rs.mu.Unlock()

	// Call complete callback
	if rs.onRotationComplete != nil {
		rs.onRotationComplete(schedule, result)
	}
}

// TriggerNow immediately triggers a scheduled rotation.
func (rs *RotationScheduler) TriggerNow(id string) error {
	rs.mu.RLock()
	schedule, ok := rs.schedules[id]
	rs.mu.RUnlock()

	if !ok {
		return fmt.Errorf("schedule %s not found", id)
	}

	go rs.triggerRotation(schedule)
	return nil
}

// CronField represents a parsed cron field.
type CronField struct {
	values []int
	min    int
	max    int
}

// ParseCronField parses a single cron field.
func ParseCronField(field string, minVal, maxVal int) (*CronField, error) {
	cf := &CronField{min: minVal, max: maxVal}

	// Handle wildcard
	if field == "*" {
		for i := minVal; i <= maxVal; i++ {
			cf.values = append(cf.values, i)
		}
		return cf, nil
	}

	// Handle step values (*/n or m-n/s)
	if strings.Contains(field, "/") {
		parts := strings.SplitN(field, "/", 2)
		step, err := strconv.Atoi(parts[1])
		if err != nil || step <= 0 {
			return nil, fmt.Errorf("invalid step value: %s", parts[1])
		}

		var start, end int
		switch {
		case parts[0] == "*":
			start, end = minVal, maxVal
		case strings.Contains(parts[0], "-"):
			rangeParts := strings.SplitN(parts[0], "-", 2)
			start, err = strconv.Atoi(rangeParts[0])
			if err != nil {
				return nil, fmt.Errorf("invalid range start: %s", rangeParts[0])
			}
			end, err = strconv.Atoi(rangeParts[1])
			if err != nil {
				return nil, fmt.Errorf("invalid range end: %s", rangeParts[1])
			}
		default:
			return nil, fmt.Errorf("invalid step expression: %s", field)
		}

		for i := start; i <= end; i += step {
			cf.values = append(cf.values, i)
		}
		return cf, nil
	}

	// Handle ranges (m-n)
	if strings.Contains(field, "-") {
		parts := strings.SplitN(field, "-", 2)
		start, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid range start: %s", parts[0])
		}
		end, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid range end: %s", parts[1])
		}
		if start > end || start < minVal || end > maxVal {
			return nil, fmt.Errorf("invalid range: %d-%d", start, end)
		}
		for i := start; i <= end; i++ {
			cf.values = append(cf.values, i)
		}
		return cf, nil
	}

	// Handle lists (a,b,c)
	if strings.Contains(field, ",") {
		parts := strings.Split(field, ",")
		for _, p := range parts {
			val, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil {
				return nil, fmt.Errorf("invalid list value: %s", p)
			}
			if val < minVal || val > maxVal {
				return nil, fmt.Errorf("value out of range: %d", val)
			}
			cf.values = append(cf.values, val)
		}
		return cf, nil
	}

	// Handle single value
	val, err := strconv.Atoi(field)
	if err != nil {
		return nil, fmt.Errorf("invalid value: %s", field)
	}
	if val < minVal || val > maxVal {
		return nil, fmt.Errorf("value out of range: %d", val)
	}
	cf.values = append(cf.values, val)
	return cf, nil
}

// Contains checks if the field contains a value.
func (cf *CronField) Contains(val int) bool {
	for _, v := range cf.values {
		if v == val {
			return true
		}
	}
	return false
}

// CronSchedule represents a parsed cron schedule.
type CronSchedule struct {
	Minute     *CronField
	Hour       *CronField
	DayOfMonth *CronField
	Month      *CronField
	DayOfWeek  *CronField
}

// ParseCron parses a cron expression.
// Format: minute hour day-of-month month day-of-week
func ParseCron(expr string) (*CronSchedule, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron expression must have 5 fields, got %d", len(fields))
	}

	minute, err := ParseCronField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("invalid minute field: %w", err)
	}

	hour, err := ParseCronField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("invalid hour field: %w", err)
	}

	dayOfMonth, err := ParseCronField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("invalid day-of-month field: %w", err)
	}

	month, err := ParseCronField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("invalid month field: %w", err)
	}

	dayOfWeek, err := ParseCronField(fields[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("invalid day-of-week field: %w", err)
	}

	return &CronSchedule{
		Minute:     minute,
		Hour:       hour,
		DayOfMonth: dayOfMonth,
		Month:      month,
		DayOfWeek:  dayOfWeek,
	}, nil
}

// Matches checks if a time matches the cron schedule.
func (cs *CronSchedule) Matches(t time.Time) bool {
	return cs.Minute.Contains(t.Minute()) &&
		cs.Hour.Contains(t.Hour()) &&
		cs.DayOfMonth.Contains(t.Day()) &&
		cs.Month.Contains(int(t.Month())) &&
		cs.DayOfWeek.Contains(int(t.Weekday()))
}

// NextCronRun calculates the next run time for a cron expression.
func NextCronRun(expr string, after time.Time) (time.Time, error) {
	schedule, err := ParseCron(expr)
	if err != nil {
		return time.Time{}, err
	}

	// Start from the next minute
	t := after.Truncate(time.Minute).Add(time.Minute)

	// Search for the next matching time (max 2 years)
	maxIterations := 365 * 24 * 60 * 2
	for i := 0; i < maxIterations; i++ {
		if schedule.Matches(t) {
			return t, nil
		}
		t = t.Add(time.Minute)
	}

	return time.Time{}, fmt.Errorf("no matching time found within 2 years")
}
