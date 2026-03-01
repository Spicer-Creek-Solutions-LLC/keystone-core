package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/shawnbutts/keystone-core/cmd/kscore-monitor/client"
	"github.com/shawnbutts/keystone-core/cmd/kscore-monitor/config"
)

type runbooksMsg struct {
	executions []client.RunbookExecutionResponse
	approvals  []client.ApprovalResponse
	err        error
}

// RunbookModel represents the runbooks view.
type RunbookModel struct {
	config     *config.Config
	client     *client.Client
	tbl        table.Model
	width      int
	height     int
	ready      bool
	loading    bool
	err        error
	viewMode   string // "executions" or "approvals"
	executions []client.RunbookExecutionResponse
	approvals  []client.ApprovalResponse
}

// NewRunbookModel creates a new runbook model.
func NewRunbookModel(cfg *config.Config, cli *client.Client) *RunbookModel {
	tbl := table.New(
		table.WithColumns(executionColumns()),
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

	return &RunbookModel{
		config:   cfg,
		client:   cli,
		tbl:      tbl,
		viewMode: "executions",
	}
}

func executionColumns() []table.Column {
	return []table.Column{
		{Title: "Runbook", Width: 20},
		{Title: "Status", Width: 12},
		{Title: "Step", Width: 18},
		{Title: "Duration", Width: 12},
		{Title: "Started", Width: 20},
	}
}

func approvalColumns() []table.Column {
	return []table.Column{
		{Title: "Runbook", Width: 22},
		{Title: "Requester", Width: 16},
		{Title: "Requested At", Width: 20},
		{Title: "Status", Width: 12},
	}
}

func (m *RunbookModel) Init() tea.Cmd { return m.Fetch() }

func (m *RunbookModel) Update(msg tea.Msg) (interface{}, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height - 4
		m.ready = true
		m.tbl.SetHeight(m.height - 8)

	case runbooksMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.executions = msg.executions
		m.approvals = msg.approvals
		m.updateTable()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			return m, m.Fetch()
		case "tab":
			if m.viewMode == "executions" {
				m.viewMode = "approvals"
			} else {
				m.viewMode = "executions"
			}
			m.updateTable()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.tbl, cmd = m.tbl.Update(msg)
	return m, cmd
}

func (m *RunbookModel) View() string {
	if !m.ready {
		return "Loading runbooks..."
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("62")).
		Render("Runbook Automation")

	// Tab indicator
	execTab := "Executions"
	apprTab := "Approvals"
	if m.viewMode == "executions" {
		execTab = lipgloss.NewStyle().Bold(true).Underline(true).Render(execTab)
	} else {
		apprTab = lipgloss.NewStyle().Bold(true).Underline(true).Render(apprTab)
	}
	tabs := fmt.Sprintf("[Tab] %s | %s", execTab, apprTab)

	var statusStr string
	if m.loading {
		statusStr = lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render("Loading...")
	} else if m.err != nil {
		statusStr = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("Error: %v", m.err))
	} else {
		pendingCount := 0
		for _, a := range m.approvals {
			if a.Status == "pending" {
				pendingCount++
			}
		}
		statusStr = fmt.Sprintf("Executions: %d | Pending approvals: %d", len(m.executions), pendingCount)
		if pendingCount > 0 {
			statusStr += " " + lipgloss.NewStyle().
				Foreground(lipgloss.Color("226")).
				Bold(true).
				Render("[APPROVAL REQUIRED]")
		}
	}

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render("↑/↓: Scroll • Tab: Toggle • r: Refresh • 1: Dashboard • q: Quit")

	return lipgloss.JoinVertical(lipgloss.Left, title, tabs, statusStr, "", m.tbl.View(), "", help)
}

func (m *RunbookModel) Fetch() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		m.loading = true
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		executions, eErr := m.client.ListRunbookExecutions(ctx)
		approvals, aErr := m.client.ListPendingApprovals(ctx)

		var err error
		if eErr != nil {
			err = eErr
		} else if aErr != nil {
			err = aErr
		}

		return runbooksMsg{executions: executions, approvals: approvals, err: err}
	}
}

func (m *RunbookModel) updateTable() {
	if m.viewMode == "executions" {
		m.tbl.SetColumns(executionColumns())
		rows := make([]table.Row, 0, len(m.executions))
		for _, e := range m.executions {
			rows = append(rows, table.Row{
				truncate(e.RunbookNm, 20),
				e.Status,
				truncate(e.Step, 18),
				e.Duration,
				e.StartedAt,
			})
		}
		m.tbl.SetRows(rows)
	} else {
		m.tbl.SetColumns(approvalColumns())
		rows := make([]table.Row, 0, len(m.approvals))
		for _, a := range m.approvals {
			rows = append(rows, table.Row{
				truncate(a.RunbookName, 22),
				truncate(a.Requester, 16),
				a.RequestedAt,
				a.Status,
			})
		}
		m.tbl.SetRows(rows)
	}
}
