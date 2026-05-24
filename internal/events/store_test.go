// SPDX-License-Identifier: Apache-2.0

package events

import (
	"errors"
	"testing"
	"time"
)

func TestEventQuery_Validate(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()

	valid := []EventQuery{
		{},
		{Type: EventTypeAgentConnect},
		{Category: CategoryAgent},
		{MinSeverity: SeverityWarn},
		{Since: now.Add(-time.Hour), Until: now},
		{Limit: 50},
		{Limit: 0}, // 0 means "use default"
	}
	for i, q := range valid {
		if err := q.Validate(); err != nil {
			t.Errorf("valid[%d] err = %v", i, err)
		}
	}

	cases := []struct {
		name string
		q    EventQuery
	}{
		{"type + category", EventQuery{Type: EventTypeAgentConnect, Category: CategoryAgent}},
		{"negative limit", EventQuery{Limit: -1}},
		{"since == until", EventQuery{Since: now, Until: now}},
		{"since > until", EventQuery{Since: now.Add(time.Hour), Until: now}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := c.q.Validate()
			if err == nil {
				t.Fatalf("Validate succeeded; want error")
			}
			if !errors.Is(err, ErrInvalidFilter) {
				t.Errorf("err = %v; want errors.Is(ErrInvalidFilter)", err)
			}
		})
	}
}

func TestSeveritiesAtLeast(t *testing.T) {
	t.Parallel()
	cases := []struct {
		threshold Severity
		want      []string
	}{
		{SeverityDebug, []string{"debug", "info", "warn", "error", "critical"}},
		{SeverityInfo, []string{"info", "warn", "error", "critical"}},
		{SeverityWarn, []string{"warn", "error", "critical"}},
		{SeverityError, []string{"error", "critical"}},
		{SeverityCritical, []string{"critical"}},
		{SeverityUnknown, nil},
		{Severity(99), nil},
	}
	for _, c := range cases {
		got := severitiesAtLeast(c.threshold)
		if len(got) != len(c.want) {
			t.Errorf("severitiesAtLeast(%s) len = %d, want %d (got %v)", c.threshold, len(got), len(c.want), got)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("severitiesAtLeast(%s)[%d] = %q, want %q", c.threshold, i, got[i], c.want[i])
			}
		}
	}
}

func TestRetentionPolicy_ZeroValue(t *testing.T) {
	t.Parallel()
	// Zero-value sanity — type empty + zero durations is the catch-all
	// no-op. The SQL layer skips it; this test just locks the value-type
	// shape so a future refactor that adds required fields surfaces.
	var p RetentionPolicy
	if p.Type != "" || p.MaxAge != 0 || p.MaxCount != 0 {
		t.Errorf("zero RetentionPolicy = %+v, want zero", p)
	}
}
