package ui

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/shawnbutts/keystone-core/cmd/kscore-monitor/client"
	"github.com/shawnbutts/keystone-core/cmd/kscore-monitor/config"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// JobsModel represents the jobs view
type JobsModel struct {
	config *config.Config
	client *client.Client
	table  table.Model
	width  int
	height int

	// Job data
	commands  []*pb.CommandInfo
	batchJobs []*pb.BatchJobInfo
	mu        sync.RWMutex

	// State
	viewMode string // "commands" or "batch"
	loading  bool
	err      error
}

// Job messages
type jobStatsMsg struct {
	stats *client.JobStats
	err   error
}

// NewJobsModel creates a new jobs model
func NewJobsModel(cfg *config.Config, cli *client.Client) *JobsModel {
	columns := []table.Column{
		{Title: "Command ID", Width: 20},
		{Title: "Agent ID", Width: 20},
		{Title: "Status", Width: 12},
		{Title: "Command", Width: 30},
		{Title: "Exit Code", Width: 10},
		{Title: "Started", Width: 18},
		{Title: "Duration", Width: 12},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(20),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)

	return &JobsModel{
		config:   cfg,
		client:   cli,
		table:    t,
		viewMode: "commands",
	}
}

// Init initializes the jobs view
func (m *JobsModel) Init() tea.Cmd {
	return m.Fetch()
}

// Update handles messages
func (m *JobsModel) Update(msg tea.Msg) (interface{}, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height - 4
		m.table.SetHeight(m.height - 5)

	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			// Refresh jobs
			return m, m.Fetch()
		case "tab":
			// Toggle between commands and batch jobs
			if m.viewMode == "commands" {
				m.viewMode = "batch"
				m.updateBatchJobsTable()
			} else {
				m.viewMode = "commands"
				m.updateCommandsTable()
			}
			return m, nil
		}

	case jobStatsMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}

		// Update job data
		m.mu.Lock()
		m.commands = msg.stats.Commands
		m.batchJobs = msg.stats.BatchJobs
		m.mu.Unlock()

		// Update table based on current view mode
		if m.viewMode == "commands" {
			m.updateCommandsTable()
		} else {
			m.updateBatchJobsTable()
		}
		return m, nil
	}

	// Update table
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// View renders the jobs view
func (m *JobsModel) View() string {
	if m.width == 0 {
		return "Loading jobs..."
	}

	if m.err != nil {
		return fmt.Sprintf("Error loading jobs: %v\n\nPress 'r' to retry or '1' to return to dashboard", m.err)
	}

	viewTitle := "Command Executions"
	if m.viewMode == "batch" {
		viewTitle = "Batch Jobs"
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("62")).
		Render(viewTitle)

	m.mu.RLock()
	count := len(m.commands)
	if m.viewMode == "batch" {
		count = len(m.batchJobs)
	}
	m.mu.RUnlock()

	stats := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Render(fmt.Sprintf("Total: %d", count))

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render("↑/↓: Navigate • Tab: Toggle View • r: Refresh • 1: Dashboard • q: Quit")

	return lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		stats,
		"",
		baseStyle.Render(m.table.View()),
		"",
		help,
	)
}

// Fetch fetches job data from the control plane
func (m *JobsModel) Fetch() tea.Cmd {
	return func() tea.Msg {
		m.loading = true

		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Fetch job stats from control plane
		stats, err := m.client.GetJobStats(ctx)
		if err != nil {
			return jobStatsMsg{err: err}
		}

		return jobStatsMsg{stats: stats}
	}
}

// updateCommandsTable updates the table with command data
func (m *JobsModel) updateCommandsTable() {
	// Update columns for commands view
	columns := []table.Column{
		{Title: "Command ID", Width: 20},
		{Title: "Agent ID", Width: 20},
		{Title: "Status", Width: 12},
		{Title: "Command", Width: 30},
		{Title: "Exit Code", Width: 10},
		{Title: "Started", Width: 18},
		{Title: "Duration", Width: 12},
	}
	m.table.SetColumns(columns)

	m.mu.RLock()
	defer m.mu.RUnlock()

	rows := make([]table.Row, 0, len(m.commands))

	for _, cmd := range m.commands {
		status := formatCommandStatus(cmd.Status)
		exitCode := fmt.Sprintf("%d", cmd.ExitCode)

		started := "Never"
		if cmd.StartedAt != nil {
			started = cmd.StartedAt.AsTime().Format("2006-01-02 15:04:05")
		}

		duration := "-"
		if cmd.StartedAt != nil && cmd.CompletedAt != nil {
			d := cmd.CompletedAt.AsTime().Sub(cmd.StartedAt.AsTime())
			duration = formatDuration(d)
		}

		// Truncate command if too long
		command := cmd.Command
		if len(command) > 30 {
			command = command[:27] + "..."
		}

		rows = append(rows, table.Row{
			cmd.CommandId,
			cmd.AgentId,
			status,
			command,
			exitCode,
			started,
			duration,
		})
	}

	m.table.SetRows(rows)
}

// updateBatchJobsTable updates the table with batch job data
func (m *JobsModel) updateBatchJobsTable() {
	// Update columns for batch jobs view
	columns := []table.Column{
		{Title: "Job ID", Width: 20},
		{Title: "Status", Width: 12},
		{Title: "Target", Width: 25},
		{Title: "Total", Width: 8},
		{Title: "Success", Width: 8},
		{Title: "Failed", Width: 8},
		{Title: "Started", Width: 18},
		{Title: "Duration", Width: 12},
	}
	m.table.SetColumns(columns)

	m.mu.RLock()
	defer m.mu.RUnlock()

	rows := make([]table.Row, 0, len(m.batchJobs))

	for _, job := range m.batchJobs {
		status := formatBatchJobStatus(job.Status)

		started := "Never"
		if job.StartedAt != nil {
			started = job.StartedAt.AsTime().Format("2006-01-02 15:04:05")
		}

		duration := "-"
		if job.StartedAt != nil && job.CompletedAt != nil {
			d := job.CompletedAt.AsTime().Sub(job.StartedAt.AsTime())
			duration = formatDuration(d)
		}

		target := job.Target
		if len(target) > 25 {
			target = target[:22] + "..."
		}

		// Get counts from Progress or Summary
		total := int32(0)
		successful := int32(0)
		failed := int32(0)
		if job.Progress != nil {
			total = job.Progress.Total
			successful = job.Progress.Successful
			failed = job.Progress.Failed
		} else if job.Summary != nil {
			total = job.Summary.Total
			successful = job.Summary.Successful
			failed = job.Summary.Failed
		}

		rows = append(rows, table.Row{
			job.BatchJobId,
			status,
			target,
			fmt.Sprintf("%d", total),
			fmt.Sprintf("%d", successful),
			fmt.Sprintf("%d", failed),
			started,
			duration,
		})
	}

	m.table.SetRows(rows)
}

// formatCommandStatus formats command status with color
func formatCommandStatus(status pb.CommandStatus) string {
	var color lipgloss.Color
	var text string

	switch status {
	case pb.CommandStatus_COMMAND_STATUS_PENDING:
		color = lipgloss.Color("11") // Yellow
		text = "Pending"
	case pb.CommandStatus_COMMAND_STATUS_RUNNING:
		color = lipgloss.Color("12") // Blue
		text = "Running"
	case pb.CommandStatus_COMMAND_STATUS_COMPLETED:
		color = lipgloss.Color("10") // Green
		text = "Completed"
	case pb.CommandStatus_COMMAND_STATUS_FAILED:
		color = lipgloss.Color("9") // Red
		text = "Failed"
	case pb.CommandStatus_COMMAND_STATUS_TIMEOUT:
		color = lipgloss.Color("208") // Orange
		text = "Timeout"
	default:
		color = lipgloss.Color("245") // Gray
		text = "Unknown"
	}

	return lipgloss.NewStyle().Foreground(color).Render(text)
}

// formatBatchJobStatus formats batch job status with color
func formatBatchJobStatus(status pb.BatchJobStatus) string {
	var color lipgloss.Color
	var text string

	switch status {
	case pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING:
		color = lipgloss.Color("11") // Yellow
		text = "Pending"
	case pb.BatchJobStatus_BATCH_JOB_STATUS_RUNNING:
		color = lipgloss.Color("12") // Blue
		text = "Running"
	case pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED:
		color = lipgloss.Color("10") // Green
		text = "Completed"
	case pb.BatchJobStatus_BATCH_JOB_STATUS_FAILED:
		color = lipgloss.Color("9") // Red
		text = "Failed"
	case pb.BatchJobStatus_BATCH_JOB_STATUS_CANCELLED:
		color = lipgloss.Color("208") // Orange
		text = "Cancelled"
	default:
		color = lipgloss.Color("245") // Gray
		text = "Unknown"
	}

	return lipgloss.NewStyle().Foreground(color).Render(text)
}

// formatDuration formats a duration as "Xs" or "Xm Xs" or "Xh Xm"
func formatDuration(d time.Duration) string {
	if d.Seconds() < 60 {
		return fmt.Sprintf("%.1fs", d.Seconds())
	} else if d.Minutes() < 60 {
		return fmt.Sprintf("%.0fm %.0fs", d.Minutes(), d.Seconds()-d.Minutes()*60)
	} else {
		return fmt.Sprintf("%.0fh %.0fm", d.Hours(), d.Minutes()-d.Hours()*60)
	}
}
