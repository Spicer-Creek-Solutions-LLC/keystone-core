package ui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/shawnbutts/keystone-core/cmd/kscore-monitor/client"
	"github.com/shawnbutts/keystone-core/cmd/kscore-monitor/config"
	monitorEvents "github.com/shawnbutts/keystone-core/cmd/kscore-monitor/events"
	"github.com/shawnbutts/keystone-core/internal/events"
)

// View represents the different views in the TUI
type View int

// View constants define the available TUI views.
const (
	ViewDashboard View = iota
	ViewAgents
	ViewEvents
	ViewStateDrift
	ViewPolicyViolations
	ViewJobs
	ViewLogs
	ViewMetrics
)

// Model is the main Bubble Tea model
type Model struct {
	ctx             context.Context
	config          *config.Config
	client          *client.Client
	eventSubscriber *monitorEvents.Subscriber

	// Current view
	currentView View
	width       int
	height      int

	// Status
	ready       bool
	err         error
	lastRefresh time.Time

	// Data models for each view
	dashboard        *DashboardModel
	agents           *AgentsModel
	events           *EventsModel
	stateDrift       *StateDriftModel
	policyViolations *PolicyViolationsModel
	jobs             *JobsModel
	logs             *LogsModel
	metrics          *MetricsModel

	// Shared state
	statusMessage string
	showHelp      bool
}

// NewProgram creates a new Bubble Tea program
func NewProgram(ctx context.Context, cfg *config.Config) (*tea.Program, error) {
	// Create gRPC client for control plane
	cli, err := client.New(ctx, cfg.ControlPlane)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to control plane: %w", err)
	}

	// Determine initial view (config uses 1-8, View type uses 0-7)
	initialView := ViewDashboard
	if cfg.InitialView >= 1 && cfg.InitialView <= 8 {
		initialView = View(cfg.InitialView - 1)
	}

	model := &Model{
		ctx:         ctx,
		config:      cfg,
		client:      cli,
		currentView: initialView,
	}

	// Initialize view models
	model.dashboard = NewDashboardModel(cfg, cli)
	model.agents = NewAgentsModel(cfg, cli)
	model.events = NewEventsModel(cfg)
	model.stateDrift = NewStateDriftModel(cfg)
	model.policyViolations = NewPolicyViolationsModel(cfg)
	model.jobs = NewJobsModel(cfg, cli)
	model.logs = NewLogsModel(cfg)
	model.metrics = NewMetricsModel(cfg)

	// Create program with options
	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),       // Use alternate screen buffer
		tea.WithMouseCellMotion(), // Enable mouse support
	)

	// Create event subscriber (needs program reference for sending events)
	eventSub, err := monitorEvents.New(ctx, cfg.NATSURL, p)
	if err != nil {
		// Log warning but don't fail - events are optional
		fmt.Printf("Warning: failed to create event subscriber: %v\n", err)
	} else {
		model.eventSubscriber = eventSub

		// Subscribe to all events
		if err := eventSub.Subscribe("*"); err != nil {
			fmt.Printf("Warning: failed to subscribe to events: %v\n", err)
		}
	}

	return p, nil
}

// Init initializes the model (Bubble Tea lifecycle)
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		tickCmd(m.config.RefreshInterval),
		m.initCurrentView(),
	)
}

// initCurrentView returns the Init command for the current view
func (m *Model) initCurrentView() tea.Cmd {
	switch m.currentView {
	case ViewDashboard:
		return m.dashboard.Init()
	case ViewAgents:
		return m.agents.Init()
	case ViewEvents:
		return m.events.Init()
	case ViewStateDrift:
		return m.stateDrift.Init()
	case ViewPolicyViolations:
		return m.policyViolations.Init()
	case ViewJobs:
		return m.jobs.Init()
	case ViewLogs:
		return m.logs.Init()
	case ViewMetrics:
		return m.metrics.Init()
	default:
		return m.dashboard.Init()
	}
}

// Update handles messages and updates the model (Bubble Tea lifecycle)
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			// Clean up connections
			if m.client != nil {
				m.client.Close()
			}
			if m.eventSubscriber != nil {
				m.eventSubscriber.Close()
			}
			return m, tea.Quit

		case "?":
			m.showHelp = !m.showHelp
			return m, nil

		// View navigation
		case "1":
			m.currentView = ViewDashboard
			return m, m.dashboard.Init()
		case "2":
			m.currentView = ViewAgents
			return m, m.agents.Init()
		case "3":
			m.currentView = ViewEvents
			return m, m.events.Init()
		case "4":
			m.currentView = ViewStateDrift
			return m, m.stateDrift.Init()
		case "5":
			m.currentView = ViewPolicyViolations
			return m, m.policyViolations.Init()
		case "6":
			m.currentView = ViewJobs
			return m, m.jobs.Init()
		case "7":
			m.currentView = ViewLogs
			return m, m.logs.Init()
		case "8":
			m.currentView = ViewMetrics
			return m, m.metrics.Init()
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		// Propagate size to all views
		cmds = append(cmds, propagateSizeToViews(m, msg)...)
		return m, tea.Batch(cmds...)

	case tickMsg:
		m.lastRefresh = time.Time(msg)
		cmds = append(cmds, tickCmd(m.config.RefreshInterval), m.refreshCurrentView())
		return m, tea.Batch(cmds...)

	case errMsg:
		m.err = error(msg)
		return m, nil

	case monitorEvents.EventMsg:
		// Forward event to all views that need it
		if msg.Event != nil {
			// Dashboard gets all events for recent events display
			m.dashboard.AddEvent(msg.Event)

			// Events view gets all events
			m.events.Update(msg)

			// Agents view gets agent events for real-time updates
			if msg.Event.Type == events.EventTypeAgentConnect ||
				msg.Event.Type == events.EventTypeAgentDisconnect ||
				msg.Event.Type == events.EventTypeAgentHeartbeat {
				m.agents.Update(msg)
			}
		}
		return m, nil
	}

	// Delegate to current view
	switch m.currentView {
	case ViewDashboard:
		_, cmd = m.dashboard.Update(msg)
	case ViewAgents:
		_, cmd = m.agents.Update(msg)
	case ViewEvents:
		_, cmd = m.events.Update(msg)
	case ViewStateDrift:
		_, cmd = m.stateDrift.Update(msg)
	case ViewPolicyViolations:
		_, cmd = m.policyViolations.Update(msg)
	case ViewJobs:
		_, cmd = m.jobs.Update(msg)
	case ViewLogs:
		_, cmd = m.logs.Update(msg)
	case ViewMetrics:
		_, cmd = m.metrics.Update(msg)
	}
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// View renders the model (Bubble Tea lifecycle)
func (m *Model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	if m.showHelp {
		return m.renderHelp()
	}

	var content string
	switch m.currentView {
	case ViewDashboard:
		content = m.dashboard.View()
	case ViewAgents:
		content = m.agents.View()
	case ViewEvents:
		content = m.events.View()
	case ViewStateDrift:
		content = m.stateDrift.View()
	case ViewPolicyViolations:
		content = m.policyViolations.View()
	case ViewJobs:
		content = m.jobs.View()
	case ViewLogs:
		content = m.logs.View()
	case ViewMetrics:
		content = m.metrics.View()
	}

	// Compose with header and footer
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderHeader(),
		content,
		m.renderFooter(),
	)
}

// renderHeader renders the top navigation bar
func (m *Model) renderHeader() string {
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("62")).
		Padding(0, 1)

	views := []string{
		"1:Dashboard",
		"2:Agents",
		"3:Events",
		"4:Drift",
		"5:Policy",
		"6:Jobs",
		"7:Logs",
		"8:Metrics",
	}

	items := make([]string, 0, len(views))
	for i, view := range views {
		style := lipgloss.NewStyle().Padding(0, 1)
		if View(i) == m.currentView {
			style = style.
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color("15")).
				Bold(true)
		}
		items = append(items, style.Render(view))
	}

	nav := lipgloss.JoinHorizontal(lipgloss.Left, items...)
	return headerStyle.Width(m.width).Render(nav)
}

// renderFooter renders the bottom status bar
func (m *Model) renderFooter() string {
	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Background(lipgloss.Color("235")).
		Padding(0, 1)

	status := fmt.Sprintf("q:Quit | ?:Help | Last refresh: %s",
		m.lastRefresh.Format("15:04:05"))

	if m.statusMessage != "" {
		status = m.statusMessage + " | " + status
	}

	return footerStyle.Width(m.width).Render(status)
}

// renderHelp renders the help screen
func (m *Model) renderHelp() string {
	helpStyle := lipgloss.NewStyle().
		Padding(2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62"))

	help := `Keystone Core Monitor - Keyboard Shortcuts

Navigation:
  1-8           Switch between views
  ↑/↓ or j/k    Navigate lists
  ←/→ or h/l    Navigate tabs
  Enter         Select/Expand item
  Esc           Go back

Commands:
  q             Quit
  ?             Toggle help
  /             Search/Filter
  r             Refresh

View-Specific:
  Dashboard:    Auto-refreshes every ` + fmt.Sprintf("%ds", m.config.RefreshInterval) + `
  Agents:       Enter = Details, s = Sort
  Events:       p = Pause/Resume, c = Clear, e = Export
  State Drift:  a = Apply fix
  Policy:       r = Remediate
  Jobs:         k = Kill, r = Retry
  Logs:         c = Clear

Press ? to close help`

	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		helpStyle.Render(help),
	)
}

// refreshCurrentView returns a command to refresh the current view's data
func (m *Model) refreshCurrentView() tea.Cmd {
	switch m.currentView {
	case ViewDashboard:
		return m.dashboard.Fetch()
	case ViewAgents:
		return m.agents.Fetch()
	case ViewEvents:
		return m.events.Fetch()
	case ViewStateDrift:
		return m.stateDrift.Fetch()
	case ViewPolicyViolations:
		return m.policyViolations.Fetch()
	case ViewJobs:
		return m.jobs.Fetch()
	case ViewLogs:
		return m.logs.Fetch()
	case ViewMetrics:
		return m.metrics.Fetch()
	}
	return nil
}

// propagateSizeToViews sends window size to all view models
func propagateSizeToViews(m *Model, msg tea.WindowSizeMsg) []tea.Cmd {
	views := []interface {
		Update(tea.Msg) (interface{}, tea.Cmd)
	}{
		m.dashboard,
		m.agents,
		m.events,
		m.stateDrift,
		m.policyViolations,
		m.jobs,
		m.logs,
		m.metrics,
	}

	cmds := make([]tea.Cmd, 0, len(views))
	for _, view := range views {
		_, cmd := view.Update(msg)
		cmds = append(cmds, cmd)
	}

	return cmds
}

// Messages

type tickMsg time.Time
type errMsg error

// tickCmd returns a command that sends a tick message after duration
func tickCmd(seconds int) tea.Cmd {
	return tea.Tick(time.Duration(seconds)*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
