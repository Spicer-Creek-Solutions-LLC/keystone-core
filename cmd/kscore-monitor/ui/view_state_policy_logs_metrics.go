package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/shawnbutts/keystone-core/cmd/kscore-monitor/client"
	"github.com/shawnbutts/keystone-core/cmd/kscore-monitor/config"
	"github.com/shawnbutts/keystone-core/cmd/kscore-monitor/events"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// stateDriftMsg carries state history data to the view.
type stateDriftMsg struct {
	history *pb.GetStateHistoryResponse
	err     error
}

// StateDriftModel represents the state drift view
type StateDriftModel struct {
	config  *config.Config
	client  *client.Client
	tbl     table.Model
	width   int
	height  int
	ready   bool
	loading bool
	err     error
	runs    []*pb.StateRun
}

// NewStateDriftModel creates a new state drift model
func NewStateDriftModel(cfg *config.Config, cli *client.Client) *StateDriftModel {
	columns := []table.Column{
		{Title: "Run ID", Width: 16},
		{Title: "Target", Width: 20},
		{Title: "Status", Width: 10},
		{Title: "Succeeded", Width: 10},
		{Title: "Failed", Width: 8},
		{Title: "Changed", Width: 8},
		{Title: "Started", Width: 20},
	}

	tbl := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	tbl.SetStyles(s)

	return &StateDriftModel{
		config: cfg,
		client: cli,
		tbl:    tbl,
	}
}

// Init initializes the state drift model.
func (m *StateDriftModel) Init() tea.Cmd {
	return m.Fetch()
}

// Update handles messages and updates the state drift model.
func (m *StateDriftModel) Update(msg tea.Msg) (interface{}, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height - 4
		m.ready = true
		m.tbl.SetHeight(m.height - 8)

	case stateDriftMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.runs = msg.history.GetRuns()
		m.updateTable()
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "r" {
			return m, m.Fetch()
		}
	}

	var cmd tea.Cmd
	m.tbl, cmd = m.tbl.Update(msg)
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

	var status string
	if m.loading {
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render("Loading...")
	} else if m.err != nil {
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("Error: %v", m.err))
	} else {
		driftCount := 0
		for _, run := range m.runs {
			if s := run.GetSummary(); s != nil && s.GetFailed() > 0 {
				driftCount++
			}
		}
		if driftCount > 0 {
			status = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).
				Render(fmt.Sprintf("Drift detected in %d run(s)", driftCount))
		} else {
			status = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).
				Render("No drift detected")
		}
		status += lipgloss.NewStyle().Foreground(lipgloss.Color("245")).
			Render(fmt.Sprintf(" | Total runs: %d", len(m.runs)))
	}

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render("↑/↓: Scroll • r: Refresh • 1: Dashboard • q: Quit")

	return lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		status,
		"",
		m.tbl.View(),
		"",
		help,
	)
}

// Fetch retrieves state drift data.
func (m *StateDriftModel) Fetch() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		m.loading = true
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		history, err := m.client.GetStateHistory(ctx)
		return stateDriftMsg{history: history, err: err}
	}
}

func (m *StateDriftModel) updateTable() {
	rows := make([]table.Row, 0, len(m.runs))
	for _, run := range m.runs {
		summary := run.GetSummary()
		status := "OK"
		if summary != nil && !summary.GetSuccess() {
			status = "FAILED"
		}
		succeeded := int32(0)
		failed := int32(0)
		changed := int32(0)
		started := ""
		if summary != nil {
			succeeded = summary.GetSucceeded()
			failed = summary.GetFailed()
			changed = summary.GetChanged()
		}
		if run.GetStartTime() != nil {
			started = run.GetStartTime().AsTime().Format("2006-01-02 15:04:05")
		}

		rows = append(rows, table.Row{
			truncate(run.GetRunId(), 16),
			truncate(run.GetTarget(), 20),
			status,
			fmt.Sprintf("%d", succeeded),
			fmt.Sprintf("%d", failed),
			fmt.Sprintf("%d", changed),
			started,
		})
	}
	m.tbl.SetRows(rows)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// policyViolationsMsg carries policy data to the view.
type policyViolationsMsg struct {
	violations *pb.ListViolationsResponse
	compliance *pb.GetComplianceReportResponse
	err        error
}

// PolicyViolationsModel represents the policy violations view
type PolicyViolationsModel struct {
	config          *config.Config
	client          *client.Client
	tbl             table.Model
	width           int
	height          int
	ready           bool
	loading         bool
	err             error
	violations      []*pb.ViolationRecord
	complianceRate  float32
	severityCounts  map[string]int64
}

// NewPolicyViolationsModel creates a new policy violations model
func NewPolicyViolationsModel(cfg *config.Config, cli *client.Client) *PolicyViolationsModel {
	columns := []table.Column{
		{Title: "Policy", Width: 18},
		{Title: "Rule", Width: 18},
		{Title: "Severity", Width: 10},
		{Title: "Resource", Width: 16},
		{Title: "Blocked", Width: 8},
		{Title: "Timestamp", Width: 20},
	}

	tbl := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	tbl.SetStyles(s)

	return &PolicyViolationsModel{
		config:         cfg,
		client:         cli,
		tbl:            tbl,
		severityCounts: make(map[string]int64),
	}
}

// Init initializes the policy violations model.
func (m *PolicyViolationsModel) Init() tea.Cmd {
	return m.Fetch()
}

// Update handles messages and updates the policy violations model.
func (m *PolicyViolationsModel) Update(msg tea.Msg) (interface{}, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height - 4
		m.ready = true
		m.tbl.SetHeight(m.height - 8)

	case policyViolationsMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		if msg.violations != nil {
			m.violations = msg.violations.GetRecords()
		}
		if msg.compliance != nil {
			m.complianceRate = msg.compliance.GetComplianceRate()
			m.severityCounts = msg.compliance.GetViolationsBySeverity()
		}
		m.updateTable()
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "r" {
			return m, m.Fetch()
		}
	}

	var cmd tea.Cmd
	m.tbl, cmd = m.tbl.Update(msg)
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

	var statsStr string
	if m.loading {
		statsStr = lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render("Loading...")
	} else if m.err != nil {
		statsStr = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("Error: %v", m.err))
	} else {
		scoreColor := "10" // green
		if m.complianceRate < 80 {
			scoreColor = "196" // red
		} else if m.complianceRate < 95 {
			scoreColor = "226" // yellow
		}
		statsStr = fmt.Sprintf(
			"Compliance: %s | Violations: %d | Critical: %d | High: %d | Medium: %d | Low: %d",
			lipgloss.NewStyle().Foreground(lipgloss.Color(scoreColor)).Render(fmt.Sprintf("%.1f%%", m.complianceRate)),
			len(m.violations),
			m.severityCounts["CRITICAL"],
			m.severityCounts["HIGH"],
			m.severityCounts["MEDIUM"],
			m.severityCounts["LOW"],
		)
	}

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render("↑/↓: Scroll • r: Refresh • 1: Dashboard • q: Quit")

	return lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		statsStr,
		"",
		m.tbl.View(),
		"",
		help,
	)
}

// Fetch retrieves policy violations data.
func (m *PolicyViolationsModel) Fetch() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		m.loading = true
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		violations, vErr := m.client.ListViolations(ctx)
		compliance, cErr := m.client.GetComplianceReport(ctx)

		// Return whichever error occurred
		var err error
		if vErr != nil {
			err = vErr
		} else if cErr != nil {
			err = cErr
		}

		return policyViolationsMsg{
			violations: violations,
			compliance: compliance,
			err:        err,
		}
	}
}

func (m *PolicyViolationsModel) updateTable() {
	rows := make([]table.Row, 0, len(m.violations))
	for _, rec := range m.violations {
		rule := ""
		severity := ""
		if v := rec.GetViolation(); v != nil {
			rule = v.GetRule()
			severity = formatPolicySeverity(v.GetSeverity())
		}
		ts := ""
		if rec.GetTimestamp() != nil {
			ts = rec.GetTimestamp().AsTime().Format("2006-01-02 15:04:05")
		}
		blocked := "No"
		if rec.GetBlocked() {
			blocked = "Yes"
		}

		rows = append(rows, table.Row{
			truncate(rec.GetPolicyName(), 18),
			truncate(rule, 18),
			severity,
			truncate(rec.GetResourceType(), 16),
			blocked,
			ts,
		})
	}
	m.tbl.SetRows(rows)
}

func formatPolicySeverity(s pb.PolicySeverity) string {
	switch s {
	case pb.PolicySeverity_POLICY_SEVERITY_CRITICAL:
		return "CRITICAL"
	case pb.PolicySeverity_POLICY_SEVERITY_HIGH:
		return "HIGH"
	case pb.PolicySeverity_POLICY_SEVERITY_MEDIUM:
		return "MEDIUM"
	case pb.PolicySeverity_POLICY_SEVERITY_LOW:
		return "LOW"
	default:
		return "UNKNOWN"
	}
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
