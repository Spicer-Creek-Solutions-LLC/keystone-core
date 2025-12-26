package ui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/titananvil/titan-anvil/cmd/titananvil-monitor/client"
	"github.com/titananvil/titan-anvil/cmd/titananvil-monitor/config"
	"github.com/titananvil/titan-anvil/pkg/events"
)

// DashboardModel represents the dashboard view
type DashboardModel struct {
	config *config.Config
	client *client.Client
	width  int
	height int

	// Dashboard data
	uptime            string
	version           string
	apiRequestRate    float64
	eventProcessRate  float64
	memoryUsage       float64
	goroutineCount    int
	agentsConnected   int
	agentsTotal       int
	jobsRunning       int
	jobsCompleted     int
	jobsFailed        int
	stateResources    int
	stateDriftCount   int
	policyViolations  int
	complianceScore   float64
	recentEvents      []*events.Event

	// Loading state
	loading bool
	err     error
}

const maxRecentEvents = 10

// NewDashboardModel creates a new dashboard model
func NewDashboardModel(cfg *config.Config, cli *client.Client) *DashboardModel {
	return &DashboardModel{
		config: cfg,
		client: cli,
	}
}

// Init initializes the dashboard
func (m *DashboardModel) Init() tea.Cmd {
	return m.Fetch()
}

// Dashboard messages
type systemStatsMsg struct {
	stats *client.SystemStats
	err   error
}

// Update handles messages
func (m *DashboardModel) Update(msg tea.Msg) (interface{}, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height - 4 // Account for header/footer

	case systemStatsMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}

		// Update dashboard data from stats
		stats := msg.stats
		m.uptime = formatUptime(stats.Uptime)
		m.version = stats.Version
		m.apiRequestRate = stats.APIRequestRate
		m.eventProcessRate = stats.EventRate
		m.memoryUsage = stats.MemoryUsageMB
		m.goroutineCount = stats.GoroutineCount
		m.agentsConnected = stats.OnlineAgents
		m.agentsTotal = stats.AgentCount
		m.jobsRunning = stats.RunningJobs
		m.jobsCompleted = stats.CompletedJobs
		m.jobsFailed = stats.FailedJobs
	}
	return m, nil
}

// formatUptime formats a duration as "Xd Xh Xm"
func formatUptime(d time.Duration) string {
	if d == 0 {
		return "0d 0h 0m"
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
}

// View renders the dashboard
func (m *DashboardModel) View() string {
	if m.width == 0 {
		return "Loading dashboard..."
	}

	if m.err != nil {
		return fmt.Sprintf("Error loading dashboard: %v\n\nPress 'r' to retry or 'q' to quit", m.err)
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("62")).
		MarginBottom(1)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1).
		Width(m.width/2 - 4)

	// System section
	systemContent := lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render("System"),
		fmt.Sprintf("Uptime: %s", m.uptime),
		fmt.Sprintf("Version: %s", m.version),
		"",
		fmt.Sprintf("API Request Rate: %.1f req/s", m.apiRequestRate),
		fmt.Sprintf("Event Process Rate: %.1f events/s", m.eventProcessRate),
		fmt.Sprintf("Memory Usage: %.1f MB", m.memoryUsage),
		fmt.Sprintf("Goroutines: %d", m.goroutineCount),
	)
	systemBox := boxStyle.Render(systemContent)

	// Calculate agent stats
	offline := m.agentsTotal - m.agentsConnected
	degraded := 0 // TODO: Track degraded agents separately

	// Agents section
	agentsContent := lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render("Agents"),
		fmt.Sprintf("Connected: %d/%d", m.agentsConnected, m.agentsTotal),
		fmt.Sprintf("Online: %d", m.agentsConnected),
		fmt.Sprintf("Offline: %d", offline),
		fmt.Sprintf("Degraded: %d", degraded),
	)
	agentsBox := boxStyle.Render(agentsContent)

	// Jobs section
	jobsContent := lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render("Jobs"),
		fmt.Sprintf("Running: %d", m.jobsRunning),
		fmt.Sprintf("Completed: %d", m.jobsCompleted),
		fmt.Sprintf("Failed: %d", m.jobsFailed),
		fmt.Sprintf("Total: %d", m.jobsRunning+m.jobsCompleted+m.jobsFailed),
	)
	jobsBox := boxStyle.Render(jobsContent)

	// State section
	stateContent := lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render("State"),
		fmt.Sprintf("Resources: %d", m.stateResources),
		fmt.Sprintf("Drift Detected: %d", m.stateDriftCount),
		"Last Check: TODO",
	)
	stateBox := boxStyle.Render(stateContent)

	// Policy section
	policyContent := lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render("Policy"),
		fmt.Sprintf("Violations: %d", m.policyViolations),
		fmt.Sprintf("Compliance: %.1f%%", m.complianceScore),
		"Critical: 0",
		"High: 0",
		"Medium: 0",
		"Low: 0",
	)
	policyBox := boxStyle.Render(policyContent)

	// Recent events section
	var eventLines []string
	if len(m.recentEvents) == 0 {
		eventLines = []string{"No recent events"}
	} else {
		for _, event := range m.recentEvents {
			eventLines = append(eventLines, formatEvent(event))
		}
	}
	eventsContent := lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render("Recent Events"),
		lipgloss.JoinVertical(lipgloss.Left, eventLines...),
	)
	eventsBox := boxStyle.Render(eventsContent)

	// Layout
	topRow := lipgloss.JoinHorizontal(lipgloss.Top, systemBox, agentsBox)
	middleRow := lipgloss.JoinHorizontal(lipgloss.Top, jobsBox, stateBox)
	bottomRow := lipgloss.JoinHorizontal(lipgloss.Top, policyBox, eventsBox)

	return lipgloss.JoinVertical(
		lipgloss.Left,
		topRow,
		middleRow,
		bottomRow,
	)
}

// Fetch fetches dashboard data from the control plane
func (m *DashboardModel) Fetch() tea.Cmd {
	return func() tea.Msg {
		m.loading = true

		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Fetch system stats from control plane
		stats, err := m.client.GetSystemStats(ctx)
		if err != nil {
			return systemStatsMsg{err: err}
		}

		return systemStatsMsg{stats: stats}
	}
}

// AddEvent adds an event to the recent events list (rolling buffer)
func (m *DashboardModel) AddEvent(event *events.Event) {
	// Prepend new event
	m.recentEvents = append([]*events.Event{event}, m.recentEvents...)

	// Keep only the most recent events
	if len(m.recentEvents) > maxRecentEvents {
		m.recentEvents = m.recentEvents[:maxRecentEvents]
	}
}

// formatEvent formats an event for display
func formatEvent(e *events.Event) string {
	// Color code by severity
	var color lipgloss.Color
	switch e.Severity {
	case events.SeverityCritical:
		color = lipgloss.Color("196") // Red
	case events.SeverityError:
		color = lipgloss.Color("208") // Orange
	case events.SeverityWarning:
		color = lipgloss.Color("226") // Yellow
	case events.SeverityInfo:
		color = lipgloss.Color("39") // Blue
	default:
		color = lipgloss.Color("245") // Gray
	}

	style := lipgloss.NewStyle().Foreground(color)
	timestamp := e.Time.Format("15:04:05")

	return fmt.Sprintf("%s %s %s",
		lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(timestamp),
		style.Render(string(e.Type)),
		e.Source)
}
