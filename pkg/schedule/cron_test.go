package schedule

import (
	"testing"
	"time"
)

func TestCronParser_Parse(t *testing.T) {
	parser := NewCronParser()

	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{
			name:    "valid 5-field cron",
			expr:    "0 2 * * *",
			wantErr: false,
		},
		{
			name:    "valid 6-field cron with seconds",
			expr:    "0 0 2 * * *",
			wantErr: false,
		},
		{
			name:    "every minute",
			expr:    "* * * * *",
			wantErr: false,
		},
		{
			name:    "every hour",
			expr:    "0 * * * *",
			wantErr: false,
		},
		{
			name:    "specific days",
			expr:    "0 0 * * 1,3,5",
			wantErr: false,
		},
		{
			name:    "range",
			expr:    "0 9-17 * * *",
			wantErr: false,
		},
		{
			name:    "step",
			expr:    "*/15 * * * *",
			wantErr: false,
		},
		{
			name:    "descriptor @daily",
			expr:    "@daily",
			wantErr: false,
		},
		{
			name:    "descriptor @hourly",
			expr:    "@hourly",
			wantErr: false,
		},
		{
			name:    "invalid too few fields",
			expr:    "* * *",
			wantErr: true,
		},
		{
			name:    "invalid value",
			expr:    "60 * * * *",
			wantErr: true,
		},
		{
			name:    "invalid characters",
			expr:    "abc * * * *",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parser.Parse(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCronParser_NextTime(t *testing.T) {
	parser := NewCronParser()

	// Use a fixed time for consistent testing
	baseTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name     string
		expr     string
		after    time.Time
		wantHour int
		wantMin  int
	}{
		{
			name:     "next hour",
			expr:     "0 * * * *",
			after:    baseTime,
			wantHour: 11,
			wantMin:  0,
		},
		{
			name:     "daily at 2am",
			expr:     "0 2 * * *",
			after:    baseTime,
			wantHour: 2,
			wantMin:  0,
		},
		{
			name:     "every 15 minutes",
			expr:     "*/15 * * * *",
			after:    baseTime,
			wantHour: 10,
			wantMin:  45,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next, err := parser.NextTime(tt.expr, tt.after)
			if err != nil {
				t.Fatalf("NextTime() error = %v", err)
			}
			if next == nil {
				t.Fatal("NextTime() returned nil")
			}
			if next.Hour() != tt.wantHour {
				t.Errorf("NextTime() hour = %d, want %d", next.Hour(), tt.wantHour)
			}
			if next.Minute() != tt.wantMin {
				t.Errorf("NextTime() minute = %d, want %d", next.Minute(), tt.wantMin)
			}
		})
	}
}

func TestCronParser_NextTimeInTimezone(t *testing.T) {
	parser := NewCronParser()

	// Use a fixed UTC time
	baseTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name     string
		expr     string
		timezone string
		wantErr  bool
	}{
		{
			name:     "UTC timezone",
			expr:     "0 2 * * *",
			timezone: "UTC",
			wantErr:  false,
		},
		{
			name:     "America/New_York",
			expr:     "0 2 * * *",
			timezone: "America/New_York",
			wantErr:  false,
		},
		{
			name:     "Europe/London",
			expr:     "0 2 * * *",
			timezone: "Europe/London",
			wantErr:  false,
		},
		{
			name:     "invalid timezone",
			expr:     "0 2 * * *",
			timezone: "Invalid/Timezone",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parser.NextTimeInTimezone(tt.expr, baseTime, tt.timezone)
			if (err != nil) != tt.wantErr {
				t.Errorf("NextTimeInTimezone() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCronParser_Validate(t *testing.T) {
	parser := NewCronParser()

	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{"valid", "0 2 * * *", false},
		{"invalid", "invalid", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parser.Validate(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsWithinWindow(t *testing.T) {
	// Monday at 10:30 AM
	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name   string
		time   time.Time
		window *TimeWindow
		want   bool
	}{
		{
			name:   "nil window",
			time:   testTime,
			window: nil,
			want:   true,
		},
		{
			name: "within time range",
			time: testTime,
			window: &TimeWindow{
				StartTime: "09:00",
				EndTime:   "17:00",
			},
			want: true,
		},
		{
			name: "outside time range",
			time: testTime,
			window: &TimeWindow{
				StartTime: "14:00",
				EndTime:   "17:00",
			},
			want: false,
		},
		{
			name: "overnight window - inside",
			time: time.Date(2024, 1, 15, 23, 30, 0, 0, time.UTC),
			window: &TimeWindow{
				StartTime: "22:00",
				EndTime:   "06:00",
			},
			want: true,
		},
		{
			name: "correct day of week",
			time: testTime,
			window: &TimeWindow{
				DaysOfWeek: []int{1}, // Monday
				StartTime:  "09:00",
				EndTime:    "17:00",
			},
			want: true,
		},
		{
			name: "wrong day of week",
			time: testTime,
			window: &TimeWindow{
				DaysOfWeek: []int{0, 6}, // Sunday, Saturday
				StartTime:  "09:00",
				EndTime:    "17:00",
			},
			want: false,
		},
		{
			name: "excluded date",
			time: testTime,
			window: &TimeWindow{
				StartTime:    "09:00",
				EndTime:      "17:00",
				ExcludeDates: []string{"2024-01-15"},
			},
			want: false,
		},
		{
			name: "include only date - included",
			time: testTime,
			window: &TimeWindow{
				StartTime:        "09:00",
				EndTime:          "17:00",
				IncludeOnlyDates: []string{"2024-01-15"},
			},
			want: true,
		},
		{
			name: "include only date - not included",
			time: testTime,
			window: &TimeWindow{
				StartTime:        "09:00",
				EndTime:          "17:00",
				IncludeOnlyDates: []string{"2024-01-16"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsWithinWindow(tt.time, tt.window)
			if got != tt.want {
				t.Errorf("IsWithinWindow() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIntervalToNextTime(t *testing.T) {
	now := time.Now().UTC()
	lastRun := now.Add(-30 * time.Minute)
	interval := time.Hour

	next := IntervalToNextTime(&lastRun, interval)

	expected := lastRun.Add(interval)
	if !next.Equal(expected) {
		t.Errorf("IntervalToNextTime() = %v, want %v", next, expected)
	}

	// Test with nil lastRun
	nextFromNil := IntervalToNextTime(nil, interval)
	if nextFromNil.Before(now.Add(-time.Second)) {
		t.Errorf("IntervalToNextTime(nil) should return approximately now")
	}
}

func TestCalculateNextRun(t *testing.T) {
	parser := NewCronParser()
	now := time.Now().UTC()

	tests := []struct {
		name     string
		schedule *Schedule
		lastRun  *time.Time
		wantNil  bool
		wantErr  bool
	}{
		{
			name: "cron schedule",
			schedule: &Schedule{
				Cron: "0 2 * * *",
			},
			lastRun: &now,
			wantNil: false,
			wantErr: false,
		},
		{
			name: "interval schedule",
			schedule: &Schedule{
				Interval: time.Hour,
			},
			lastRun: &now,
			wantNil: false,
			wantErr: false,
		},
		{
			name: "no cron or interval",
			schedule: &Schedule{
				Cron:     "",
				Interval: 0,
			},
			lastRun: &now,
			wantNil: true,
			wantErr: true,
		},
		{
			name: "past end date",
			schedule: &Schedule{
				Cron:    "0 2 * * *",
				EndDate: func() *time.Time { t := now.Add(-24 * time.Hour); return &t }(),
			},
			lastRun: &now,
			wantNil: true,
			wantErr: false,
		},
		{
			name: "future start date",
			schedule: &Schedule{
				Cron:      "0 2 * * *",
				StartDate: func() *time.Time { t := now.Add(24 * time.Hour); return &t }(),
			},
			lastRun: nil,
			wantNil: false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next, err := CalculateNextRun(tt.schedule, tt.lastRun, parser)
			if (err != nil) != tt.wantErr {
				t.Errorf("CalculateNextRun() error = %v, wantErr %v", err, tt.wantErr)
			}
			if (next == nil) != tt.wantNil {
				t.Errorf("CalculateNextRun() nil = %v, wantNil %v", next == nil, tt.wantNil)
			}
		})
	}
}
