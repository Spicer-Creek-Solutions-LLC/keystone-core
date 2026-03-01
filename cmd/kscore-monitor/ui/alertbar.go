package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// AlertCounts holds counts for the persistent alert bar.
type AlertCounts struct {
	OfflineAgents      int
	FailedJobs         int
	ActiveDrift        int
	CriticalViolations int
	HighViolations     int
	PendingApprovals   int
	ExpiringLeases     int
}

// AlertBarModel renders a compact single-line alert bar.
type AlertBarModel struct {
	counts AlertCounts
	width  int
}

// NewAlertBarModel creates a new alert bar.
func NewAlertBarModel() *AlertBarModel {
	return &AlertBarModel{}
}

// SetCounts updates the alert counts.
func (m *AlertBarModel) SetCounts(c AlertCounts) {
	m.counts = c
}

// SetWidth updates the bar width.
func (m *AlertBarModel) SetWidth(w int) {
	m.width = w
}

// HasAlerts returns true if any alert count is nonzero.
func (m *AlertBarModel) HasAlerts() bool {
	c := m.counts
	return c.OfflineAgents > 0 || c.FailedJobs > 0 || c.ActiveDrift > 0 ||
		c.CriticalViolations > 0 || c.HighViolations > 0 ||
		c.PendingApprovals > 0 || c.ExpiringLeases > 0
}

// View renders the alert bar.
func (m *AlertBarModel) View() string {
	barStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("235")).
		Padding(0, 1)

	if !m.HasAlerts() {
		content := lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Render("All clear")
		return barStyle.Width(m.width).Render(content)
	}

	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	critStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	var parts []string
	c := m.counts

	if c.OfflineAgents > 0 {
		parts = append(parts, critStyle.Render(fmt.Sprintf("▲%d offline", c.OfflineAgents)))
	}
	if c.FailedJobs > 0 {
		parts = append(parts, warnStyle.Render(fmt.Sprintf("▲%d failed", c.FailedJobs)))
	}
	if c.ActiveDrift > 0 {
		parts = append(parts, warnStyle.Render(fmt.Sprintf("▲%d drift", c.ActiveDrift)))
	}
	if c.CriticalViolations > 0 {
		parts = append(parts, critStyle.Render(fmt.Sprintf("▲%d critical", c.CriticalViolations)))
	}
	if c.HighViolations > 0 {
		parts = append(parts, warnStyle.Render(fmt.Sprintf("▲%d high", c.HighViolations)))
	}
	if c.PendingApprovals > 0 {
		parts = append(parts, warnStyle.Render(fmt.Sprintf("▲%d approvals", c.PendingApprovals)))
	}
	if c.ExpiringLeases > 0 {
		parts = append(parts, warnStyle.Render(fmt.Sprintf("▲%d expiring", c.ExpiringLeases)))
	}

	return barStyle.Width(m.width).Render(strings.Join(parts, " | "))
}
