package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/titananvil/titan-anvil/cmd/titananvil-monitor/config"
)

// DashboardModel represents the dashboard view
type DashboardModel struct {
	config *config.Config
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
	recentEvents      []string
}

// NewDashboardModel creates a new dashboard model
func NewDashboardModel(cfg *config.Config) *DashboardModel {
	return &DashboardModel{
		config: cfg,
	}
}

// Init initializes the dashboard
func (m *DashboardModel) Init() tea.Cmd {
	return m.Fetch()
}

// Update handles messages
func (m *DashboardModel) Update(msg tea.Msg) (interface{}, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height - 4 // Account for header/footer
	}
	return m, nil
}

// View renders the dashboard
func (m *DashboardModel) View() string {
	if m.width == 0 {
		return "Loading dashboard..."
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
		lipgloss.NewStyle().Render("Uptime: "+m.uptime),
		lipgloss.NewStyle().Render("Version: "+m.version),
		"",
		"API Request Rate: 0.0 req/s",
		"Event Process Rate: 0.0 events/s",
		"Memory Usage: 0.0 MB",
		"Goroutines: 0",
	)
	systemBox := boxStyle.Render(systemContent)

	// Agents section
	agentsContent := lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render("Agents"),
		"Connected: 0/0",
		"Online: 0",
		"Offline: 0",
		"Degraded: 0",
	)
	agentsBox := boxStyle.Render(agentsContent)

	// Jobs section
	jobsContent := lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render("Jobs"),
		"Running: 0",
		"Completed: 0",
		"Failed: 0",
		"Queue Length: 0",
	)
	jobsBox := boxStyle.Render(jobsContent)

	// State section
	stateContent := lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render("State"),
		"Resources: 0",
		"Drift Detected: 0",
		"Last Check: Never",
	)
	stateBox := boxStyle.Render(stateContent)

	// Policy section
	policyContent := lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render("Policy"),
		"Violations: 0",
		"Compliance: 100.0%",
		"Critical: 0",
		"High: 0",
		"Medium: 0",
		"Low: 0",
	)
	policyBox := boxStyle.Render(policyContent)

	// Recent events section
	eventsContent := lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render("Recent Events"),
		"No recent events",
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

// Fetch fetches dashboard data
func (m *DashboardModel) Fetch() tea.Cmd {
	return func() tea.Msg {
		// TODO: Fetch data from control plane API
		// For now, return dummy data
		m.uptime = "0d 0h 0m"
		m.version = "0.1.0"
		return nil
	}
}
