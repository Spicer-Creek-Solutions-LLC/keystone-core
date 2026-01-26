package schedule

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// CronParser provides cron expression parsing and next time calculation.
type CronParser struct {
	parser cron.Parser
}

// NewCronParser creates a new cron parser.
// Supports standard 5-field cron expressions (minute, hour, day of month, month, day of week)
// and extended 6-field expressions with seconds.
func NewCronParser() *CronParser {
	return &CronParser{
		parser: cron.NewParser(
			cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
		),
	}
}

// Parse parses a cron expression and returns a schedule.
func (p *CronParser) Parse(expr string) (cron.Schedule, error) {
	schedule, err := p.parser.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCron, err)
	}
	return schedule, nil
}

// NextTime calculates the next execution time after the given time.
func (p *CronParser) NextTime(expr string, after time.Time) (*time.Time, error) {
	schedule, err := p.Parse(expr)
	if err != nil {
		return nil, err
	}

	next := schedule.Next(after)
	return &next, nil
}

// NextTimeInTimezone calculates the next execution time in the specified timezone.
func (p *CronParser) NextTimeInTimezone(expr string, after time.Time, timezone string) (*time.Time, error) {
	loc := time.UTC
	if timezone != "" {
		var err error
		loc, err = time.LoadLocation(timezone)
		if err != nil {
			return nil, fmt.Errorf("invalid timezone %q: %w", timezone, err)
		}
	}

	// Convert to the target timezone
	afterInTZ := after.In(loc)

	schedule, err := p.Parse(expr)
	if err != nil {
		return nil, err
	}

	next := schedule.Next(afterInTZ)

	// Convert back to UTC for storage
	nextUTC := next.UTC()
	return &nextUTC, nil
}

// Validate validates a cron expression.
func (p *CronParser) Validate(expr string) error {
	_, err := p.Parse(expr)
	return err
}

// IsWithinWindow checks if the given time is within the specified time window.
func IsWithinWindow(t time.Time, window *TimeWindow) bool {
	if window == nil {
		return true
	}

	// Apply timezone
	loc := time.UTC
	if window.Timezone != "" {
		if l, err := time.LoadLocation(window.Timezone); err == nil {
			loc = l
		}
	}
	t = t.In(loc)

	// Check day of week
	if len(window.DaysOfWeek) > 0 {
		dayOk := false
		weekday := int(t.Weekday())
		for _, d := range window.DaysOfWeek {
			if d == weekday {
				dayOk = true
				break
			}
		}
		if !dayOk {
			return false
		}
	}

	// Check exclude dates
	dateStr := t.Format("2006-01-02")
	for _, exclude := range window.ExcludeDates {
		if exclude == dateStr {
			return false
		}
	}

	// Check include only dates
	if len(window.IncludeOnlyDates) > 0 {
		included := false
		for _, include := range window.IncludeOnlyDates {
			if include == dateStr {
				included = true
				break
			}
		}
		if !included {
			return false
		}
	}

	// Check time window
	if window.StartTime != "" && window.EndTime != "" {
		currentTime := t.Format("15:04")

		// Simple string comparison works for HH:MM format
		if window.StartTime <= window.EndTime {
			// Normal window (e.g., 09:00 - 17:00)
			if currentTime < window.StartTime || currentTime > window.EndTime {
				return false
			}
		} else {
			// Overnight window (e.g., 22:00 - 06:00)
			if currentTime < window.StartTime && currentTime > window.EndTime {
				return false
			}
		}
	}

	return true
}

// NextTimeInWindow finds the next cron execution time that falls within the window.
func (p *CronParser) NextTimeInWindow(expr string, after time.Time, window *TimeWindow, maxIterations int) (*time.Time, error) {
	if maxIterations <= 0 {
		maxIterations = 1000 // Default max iterations
	}

	schedule, err := p.Parse(expr)
	if err != nil {
		return nil, err
	}

	current := after
	for i := 0; i < maxIterations; i++ {
		next := schedule.Next(current)
		if IsWithinWindow(next, window) {
			return &next, nil
		}
		current = next
	}

	return nil, fmt.Errorf("no valid execution time found within %d iterations", maxIterations)
}

// IntervalToNextTime calculates the next execution time based on interval.
func IntervalToNextTime(lastRun *time.Time, interval time.Duration) time.Time {
	if lastRun == nil {
		return time.Now().UTC()
	}
	return lastRun.Add(interval)
}

// CalculateNextRun calculates the next run time for a schedule.
func CalculateNextRun(s *Schedule, lastRun *time.Time, cronParser *CronParser) (*time.Time, error) {
	var after time.Time
	if lastRun != nil {
		after = *lastRun
	} else {
		after = time.Now().UTC()
	}

	// Check if schedule has an end date and it's passed
	if s.EndDate != nil && after.After(*s.EndDate) {
		return nil, nil // No more runs
	}

	// Check if schedule has a start date and we're before it
	if s.StartDate != nil && after.Before(*s.StartDate) {
		after = *s.StartDate
	}

	var nextTime *time.Time
	var err error

	if s.Cron != "" {
		// Cron-based scheduling
		if s.Window != nil {
			nextTime, err = cronParser.NextTimeInWindow(s.Cron, after, s.Window, 1000)
		} else if s.Timezone != "" {
			nextTime, err = cronParser.NextTimeInTimezone(s.Cron, after, s.Timezone)
		} else {
			nextTime, err = cronParser.NextTime(s.Cron, after)
		}
		if err != nil {
			return nil, err
		}
	} else if s.Interval > 0 {
		// Interval-based scheduling
		next := IntervalToNextTime(lastRun, s.Interval)
		nextTime = &next
	} else {
		return nil, ErrInvalidSchedule
	}

	// Check if the next time is within the end date
	if s.EndDate != nil && nextTime != nil && nextTime.After(*s.EndDate) {
		return nil, nil // No more runs
	}

	return nextTime, nil
}
