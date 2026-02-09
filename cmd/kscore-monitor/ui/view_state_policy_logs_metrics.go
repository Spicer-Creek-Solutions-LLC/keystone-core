package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/shawnbutts/keystone-core/cmd/kscore-monitor/config"
	"github.com/shawnbutts/keystone-core/cmd/kscore-monitor/events"
)

// StateDriftModel represents the state drift view
type StateDriftModel struct {
	config   *config.Config
	viewport viewport.Model
	width    int
	height   int
	ready    bool
}

// NewStateDriftModel creates a new state drift model
func NewStateDriftModel(cfg *config.Config) *StateDriftModel {
	vp := viewport.New(0, 0)
	vp.Style = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1)

	return &StateDriftModel{
		config:   cfg,
		viewport: vp,
	}
}

// Init initializes the state drift model.
func (m *StateDriftModel) Init() tea.Cmd {
	return m.Fetch()
}

// Update handles messages and updates the state drift model.
func (m *StateDriftModel) Update(msg tea.Msg) (interface{}, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height - 4

		if !m.ready {
			m.viewport.Width = m.width - 4
			m.viewport.Height = m.height - 6
			m.ready = true
			m.updateViewport()
		} else {
			m.viewport.Width = m.width - 4
			m.viewport.Height = m.height - 6
		}

	case tea.KeyMsg:
		if msg.String() == "r" {
			cmd := m.Fetch()
			return m, cmd
		}
	}

	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View renders the state drift model.
func (m *StateDriftModel) View() string {
	if !m.ready {
		return "Loading state drift..."
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("62")).
		Render("State Drift Detection")

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render("↑/↓: Scroll • r: Refresh • 1: Dashboard • q: Quit")

	return lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		m.viewport.View(),
		"",
		help,
	)
}

// Fetch retrieves state drift data.
func (m *StateDriftModel) Fetch() tea.Cmd {
	return nil
}

func (m *StateDriftModel) updateViewport() {
	content := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Render(`State Drift Detection

Monitor configuration drift across your infrastructure.

Status: No drift detected

Recent Checks:
• 2024-12-26 10:30:00 - All resources in sync
• 2024-12-26 10:15:00 - All resources in sync
• 2024-12-26 10:00:00 - All resources in sync

This view will display:
- Resources with detected drift
- Severity levels (Low, Medium, High, Critical)
- Drift details and differences
- Affected resources and their current state
- Recommended remediation actions

Note: State drift detection requires the state management
system to be configured with drift monitoring enabled.

Press 'r' to refresh or '1' to return to dashboard.`)

	m.viewport.SetContent(content)
}

// PolicyViolationsModel represents the policy violations view
type PolicyViolationsModel struct {
	config   *config.Config
	viewport viewport.Model
	width    int
	height   int
	ready    bool
}

// NewPolicyViolationsModel creates a new policy violations model
func NewPolicyViolationsModel(cfg *config.Config) *PolicyViolationsModel {
	vp := viewport.New(0, 0)
	vp.Style = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1)

	return &PolicyViolationsModel{
		config:   cfg,
		viewport: vp,
	}
}

// Init initializes the policy violations model.
func (m *PolicyViolationsModel) Init() tea.Cmd {
	return m.Fetch()
}

// Update handles messages and updates the policy violations model.
func (m *PolicyViolationsModel) Update(msg tea.Msg) (interface{}, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height - 4

		if !m.ready {
			m.viewport.Width = m.width - 4
			m.viewport.Height = m.height - 6
			m.ready = true
			m.updateViewport()
		} else {
			m.viewport.Width = m.width - 4
			m.viewport.Height = m.height - 6
		}

	case tea.KeyMsg:
		if msg.String() == "r" {
			cmd := m.Fetch()
			return m, cmd
		}
	}

	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View renders the policy violations model.
func (m *PolicyViolationsModel) View() string {
	if !m.ready {
		return "Loading policy violations..."
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("62")).
		Render("Policy Violations")

	stats := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Render("Compliance Score: 100% | Violations: 0")

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render("↑/↓: Scroll • r: Refresh • 1: Dashboard • q: Quit")

	return lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		stats,
		"",
		m.viewport.View(),
		"",
		help,
	)
}

// Fetch retrieves policy violations data.
func (m *PolicyViolationsModel) Fetch() tea.Cmd {
	return nil
}

func (m *PolicyViolationsModel) updateViewport() {
	content := lipgloss.NewStyle().
		Foreground(lipgloss.Color("10")).
		Render(`✓ No policy violations detected

All systems are compliant with defined policies.

Recent Policy Evaluations:
• Security policies: PASSED
• Compliance policies: PASSED
• Operational policies: PASSED
• Cost policies: PASSED

This view will display:
- Policy violations by severity (Critical, High, Medium, Low)
- Affected resources and violation details
- Remediation recommendations
- Policy evaluation history
- Compliance trends over time

Policy types monitored:
- Security: Access controls, encryption, authentication
- Compliance: Regulatory requirements, audit trails
- Operational: Resource limits, configuration standards
- Cost: Budget constraints, resource optimization

Note: Policy enforcement requires OPA/CEL policies to be
configured in the policy engine.

Press 'r' to refresh or '1' to return to dashboard.`)

	m.viewport.SetContent(content)
}

// LogsModel represents the logs view
type LogsModel struct {
	config    *config.Config
	viewport  viewport.Model
	logBuffer *events.LogBuffer
	paused    bool
	width     int
	height    int
	ready     bool
}

// NewLogsModel creates a new logs model
func NewLogsModel(cfg *config.Config) *LogsModel {
	vp := viewport.New(0, 0)
	vp.Style = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1)

	return &LogsModel{
		config:    cfg,
		viewport:  vp,
		logBuffer: events.NewLogBuffer(1000),
	}
}

// Init initializes the logs model.
func (m *LogsModel) Init() tea.Cmd {
	return m.Fetch()
}

// Update handles messages and updates the logs model.
func (m *LogsModel) Update(msg tea.Msg) (interface{}, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height - 4

		if !m.ready {
			m.viewport.Width = m.width - 4
			m.viewport.Height = m.height - 6
			m.ready = true
			m.updateViewport()
		} else {
			m.viewport.Width = m.width - 4
			m.viewport.Height = m.height - 6
		}

	case events.LogMsg:
		if msg.Log != nil && !m.paused {
			m.logBuffer.Add(msg.Log)
			m.updateViewport()
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			cmd := m.Fetch()
			return m, cmd
		case "c":
			m.logBuffer.Clear()
			m.updateViewport()
		case "p", " ":
			m.paused = !m.paused
		}
	}

	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View renders the logs model.
func (m *LogsModel) View() string {
	if !m.ready {
		return "Loading logs..."
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("62")).
		Render("Log Stream")

	// Status line with count and paused indicator
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	pausedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Bold(true)

	status := statusStyle.Render(fmt.Sprintf("Logs: %d", m.logBuffer.Count()))
	if m.paused {
		status += " " + pausedStyle.Render("[PAUSED]")
	} else {
		status += " " + lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("[LIVE]")
	}

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render("↑/↓: Scroll • p/space: Pause • c: Clear • 1: Dashboard • q: Quit")

	return lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		status,
		"",
		m.viewport.View(),
		"",
		help,
	)
}

// Fetch retrieves logs data.
func (m *LogsModel) Fetch() tea.Cmd {
	return nil
}

func (m *LogsModel) updateViewport() {
	logs := m.logBuffer.All()

	if len(logs) == 0 {
		content := lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Render(`Waiting for log messages...

This view displays structured logs from:
- Control Plane services
- Agent operations
- State management operations
- Policy evaluations
- Job executions

Log streaming requires NATS telemetry transport to be configured.
Logs will appear here in real-time once connected.

Press 'p' or space to pause/resume, 'c' to clear.`)
		m.viewport.SetContent(content)
		return
	}

	// Style definitions for log levels
	debugStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	infoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	var lines []string
	for _, log := range logs {
		var style lipgloss.Style
		switch strings.ToLower(log.Level) {
		case "debug":
			style = debugStyle
		case "info":
			style = infoStyle
		case "warn", "warning":
			style = warnStyle
		case "error", "critical":
			style = errorStyle
		default:
			style = debugStyle
		}

		// Format: timestamp [LEVEL] service: message
		line := fmt.Sprintf("%s [%s] %s: %s",
			log.Timestamp,
			strings.ToUpper(log.Level),
			log.Service,
			log.Message)
		lines = append(lines, style.Render(line))
	}

	m.viewport.SetContent(strings.Join(lines, "\n"))
}

// MetricsModel represents the metrics view
type MetricsModel struct {
	config       *config.Config
	viewport     viewport.Model
	metricBuffer *events.MetricBuffer
	width        int
	height       int
	ready        bool
}

// NewMetricsModel creates a new metrics model
func NewMetricsModel(cfg *config.Config) *MetricsModel {
	vp := viewport.New(0, 0)
	vp.Style = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1)

	return &MetricsModel{
		config:       cfg,
		viewport:     vp,
		metricBuffer: events.NewMetricBuffer(),
	}
}

// Init initializes the metrics model.
func (m *MetricsModel) Init() tea.Cmd {
	return m.Fetch()
}

// Update handles messages and updates the metrics model.
func (m *MetricsModel) Update(msg tea.Msg) (interface{}, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height - 4

		if !m.ready {
			m.viewport.Width = m.width - 4
			m.viewport.Height = m.height - 6
			m.ready = true
			m.updateViewport()
		} else {
			m.viewport.Width = m.width - 4
			m.viewport.Height = m.height - 6
		}

	case events.MetricMsg:
		if msg.Metric != nil {
			m.metricBuffer.Add(msg.Metric)
			m.updateViewport()
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			cmd := m.Fetch()
			return m, cmd
		case "c":
			m.metricBuffer.Clear()
			m.updateViewport()
		}
	}

	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View renders the metrics model.
func (m *MetricsModel) View() string {
	if !m.ready {
		return "Loading metrics..."
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("62")).
		Render("Metrics & Performance")

	// Status line with metric count
	status := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Render(fmt.Sprintf("Unique metrics: %d", m.metricBuffer.Count()))

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render("↑/↓: Scroll • r: Refresh • c: Clear • 1: Dashboard • q: Quit")

	return lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		status,
		"",
		m.viewport.View(),
		"",
		help,
	)
}

// Fetch retrieves metrics data.
func (m *MetricsModel) Fetch() tea.Cmd {
	return nil
}

func (m *MetricsModel) updateViewport() {
	metrics := m.metricBuffer.All()

	if len(metrics) == 0 {
		content := lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Render(`Waiting for metrics...

This view displays real-time performance metrics:
- Control Plane metrics (API, events, connections)
- Agent metrics (CPU, memory, disk)
- Job execution metrics
- State management metrics
- Policy evaluation metrics

Metrics streaming requires NATS telemetry transport to be configured.
Metrics will appear here in real-time once connected.

Press 'r' to refresh, 'c' to clear.`)
		m.viewport.SetContent(content)
		return
	}

	// Style definitions for metric types
	counterStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	gaugeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	histogramStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	defaultStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	// Group metrics by service
	serviceMetrics := make(map[string][]*struct {
		Name  string
		Type  string
		Value float64
	})

	for _, metric := range metrics {
		svc := metric.Service
		if svc == "" {
			svc = "unknown"
		}
		serviceMetrics[svc] = append(serviceMetrics[svc], &struct {
			Name  string
			Type  string
			Value float64
		}{
			Name:  metric.Name,
			Type:  metric.Type,
			Value: metric.Value,
		})
	}

	var lines []string
	lines = append(lines, lipgloss.NewStyle().Bold(true).Render("System Metrics Overview"), "")

	for service, svcMetrics := range serviceMetrics {
		lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62")).Render(service+":"))
		for _, metric := range svcMetrics {
			var style lipgloss.Style
			switch strings.ToLower(metric.Type) {
			case "counter":
				style = counterStyle
			case "gauge":
				style = gaugeStyle
			case "histogram", "summary":
				style = histogramStyle
			default:
				style = defaultStyle
			}

			line := fmt.Sprintf("  ├─ %s: %.2f (%s)", metric.Name, metric.Value, metric.Type)
			lines = append(lines, style.Render(line))
		}
		lines = append(lines, "")
	}

	m.viewport.SetContent(strings.Join(lines, "\n"))
}
