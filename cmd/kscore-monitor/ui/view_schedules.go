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

type schedulesMsg struct {
	schedules []client.ScheduleResponse
	windows   []client.MaintenanceWindowResponse
	err       error
}

// ScheduleModel represents the schedules and maintenance windows view.
type ScheduleModel struct {
	config    *config.Config
	client    *client.Client
	tbl       table.Model
	width     int
	height    int
	ready     bool
	loading   bool
	err       error
	viewMode  string // "schedules" or "windows"
	schedules []client.ScheduleResponse
	windows   []client.MaintenanceWindowResponse
}

// NewScheduleModel creates a new schedule model.
func NewScheduleModel(cfg *config.Config, cli *client.Client) *ScheduleModel {
	tbl := table.New(
		table.WithColumns(scheduleColumns()),
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

	return &ScheduleModel{
		config:   cfg,
		client:   cli,
		tbl:      tbl,
		viewMode: "schedules",
	}
}

func scheduleColumns() []table.Column {
	return []table.Column{
		{Title: "Name", Width: 22},
		{Title: "Cron", Width: 16},
		{Title: "Status", Width: 10},
		{Title: "Last Run", Width: 20},
		{Title: "Next Run", Width: 20},
	}
}

func windowColumns() []table.Column {
	return []table.Column{
		{Title: "Name", Width: 22},
		{Title: "Start", Width: 20},
		{Title: "End", Width: 20},
		{Title: "Active", Width: 8},
		{Title: "Scope", Width: 20},
	}
}

func (m *ScheduleModel) Init() tea.Cmd { return m.Fetch() }

func (m *ScheduleModel) Update(msg tea.Msg) (interface{}, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height - 4
		m.ready = true
		m.tbl.SetHeight(m.height - 8)

	case schedulesMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.schedules = msg.schedules
		m.windows = msg.windows
		m.updateTable()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			return m, m.Fetch()
		case "tab":
			if m.viewMode == "schedules" {
				m.viewMode = "windows"
			} else {
				m.viewMode = "schedules"
			}
			m.updateTable()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.tbl, cmd = m.tbl.Update(msg)
	return m, cmd
}

func (m *ScheduleModel) View() string {
	if !m.ready {
		return "Loading schedules..."
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("62")).
		Render("Schedules & Maintenance")

	// Tab indicator
	schedTab := "Schedules"
	winTab := "Windows"
	if m.viewMode == "schedules" {
		schedTab = lipgloss.NewStyle().Bold(true).Underline(true).Render(schedTab)
	} else {
		winTab = lipgloss.NewStyle().Bold(true).Underline(true).Render(winTab)
	}
	tabs := fmt.Sprintf("[Tab] %s | %s", schedTab, winTab)

	var statusStr string
	if m.loading {
		statusStr = lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render("Loading...")
	} else if m.err != nil {
		statusStr = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("Error: %v", m.err))
	} else {
		activeWindows := 0
		for _, w := range m.windows {
			if w.Active {
				activeWindows++
			}
		}
		statusStr = fmt.Sprintf("Schedules: %d | Maintenance windows: %d | Active: %d",
			len(m.schedules), len(m.windows), activeWindows)
		if activeWindows > 0 {
			statusStr += " " + lipgloss.NewStyle().
				Foreground(lipgloss.Color("226")).
				Bold(true).
				Render("[MAINTENANCE MODE]")
		}
	}

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render("↑/↓: Scroll • Tab: Toggle • r: Refresh • 1: Dashboard • q: Quit")

	return lipgloss.JoinVertical(lipgloss.Left, title, tabs, statusStr, "", m.tbl.View(), "", help)
}

func (m *ScheduleModel) Fetch() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		m.loading = true
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		schedules, sErr := m.client.ListSchedules(ctx)
		windows, wErr := m.client.GetActiveMaintenanceWindows(ctx)

		var err error
		if sErr != nil {
			err = sErr
		} else if wErr != nil {
			err = wErr
		}

		return schedulesMsg{schedules: schedules, windows: windows, err: err}
	}
}

func (m *ScheduleModel) updateTable() {
	if m.viewMode == "schedules" {
		m.tbl.SetColumns(scheduleColumns())
		rows := make([]table.Row, 0, len(m.schedules))
		for _, s := range m.schedules {
			rows = append(rows, table.Row{
				truncate(s.Name, 22),
				truncate(s.CronExpr, 16),
				s.Status,
				s.LastRun,
				s.NextRun,
			})
		}
		m.tbl.SetRows(rows)
	} else {
		m.tbl.SetColumns(windowColumns())
		rows := make([]table.Row, 0, len(m.windows))
		for _, w := range m.windows {
			active := "No"
			if w.Active {
				active = "Yes"
			}
			rows = append(rows, table.Row{
				truncate(w.Name, 22),
				w.StartTime,
				w.EndTime,
				active,
				truncate(w.Scope, 20),
			})
		}
		m.tbl.SetRows(rows)
	}
}
