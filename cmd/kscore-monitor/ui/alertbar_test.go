package ui

import (
	"strings"
	"testing"
)

func TestAlertBarAllClear(t *testing.T) {
	bar := NewAlertBarModel()
	bar.SetWidth(80)
	bar.SetCounts(AlertCounts{})

	if bar.HasAlerts() {
		t.Error("expected no alerts")
	}
	view := bar.View()
	if !strings.Contains(view, "All clear") {
		t.Errorf("expected 'All clear', got %q", view)
	}
}

func TestAlertBarSomeAlerts(t *testing.T) {
	bar := NewAlertBarModel()
	bar.SetWidth(120)
	bar.SetCounts(AlertCounts{
		OfflineAgents:      2,
		FailedJobs:         1,
		CriticalViolations: 3,
	})

	if !bar.HasAlerts() {
		t.Error("expected alerts")
	}
	view := bar.View()
	if !strings.Contains(view, "2 offline") {
		t.Errorf("expected offline count, got %q", view)
	}
	if !strings.Contains(view, "1 failed") {
		t.Errorf("expected failed count, got %q", view)
	}
	if !strings.Contains(view, "3 critical") {
		t.Errorf("expected critical count, got %q", view)
	}
}

func TestAlertBarAllCritical(t *testing.T) {
	bar := NewAlertBarModel()
	bar.SetWidth(120)
	bar.SetCounts(AlertCounts{
		OfflineAgents:      5,
		FailedJobs:         10,
		ActiveDrift:        3,
		CriticalViolations: 7,
		HighViolations:     12,
		PendingApprovals:   2,
		ExpiringLeases:     4,
	})

	view := bar.View()
	for _, expected := range []string{"offline", "failed", "drift", "critical", "high", "approvals", "expiring"} {
		if !strings.Contains(view, expected) {
			t.Errorf("expected %q in view, got %q", expected, view)
		}
	}
}
