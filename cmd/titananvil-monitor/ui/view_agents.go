package ui

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/titananvil/titan-anvil/cmd/titananvil-monitor/client"
	"github.com/titananvil/titan-anvil/cmd/titananvil-monitor/config"
	monitorEvents "github.com/titananvil/titan-anvil/cmd/titananvil-monitor/events"
	pb "github.com/titananvil/titan-anvil/pkg/api/v1"
	"github.com/titananvil/titan-anvil/pkg/events"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var baseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

// AgentsModel represents the agents view
type AgentsModel struct {
	config *config.Config
	client *client.Client
	table  table.Model
	width  int
	height int

	// Agent data
	agents    map[string]*pb.AgentInfo // agent_id -> AgentInfo
	agentList []*pb.AgentInfo           // sorted list for table
	mu        sync.RWMutex

	// State
	loading bool
	err     error
}

// Agent messages
type agentStatsMsg struct {
	stats *client.AgentStats
	err   error
}

// NewAgentsModel creates a new agents model
func NewAgentsModel(cfg *config.Config, cli *client.Client) *AgentsModel {
	columns := []table.Column{
		{Title: "Agent ID", Width: 20},
		{Title: "Hostname", Width: 20},
		{Title: "Status", Width: 10},
		{Title: "OS", Width: 15},
		{Title: "IP Address", Width: 15},
		{Title: "Version", Width: 10},
		{Title: "Last Seen", Width: 15},
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

	return &AgentsModel{
		config: cfg,
		client: cli,
		table:  t,
		agents: make(map[string]*pb.AgentInfo),
	}
}

// Init initializes the agents view
func (m *AgentsModel) Init() tea.Cmd {
	return m.Fetch()
}

// Update handles messages
func (m *AgentsModel) Update(msg tea.Msg) (interface{}, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height - 4
		m.table.SetHeight(m.height - 5) // Account for title and help text

	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			// Refresh agents
			return m, m.Fetch()
		}

	case agentStatsMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}

		// Update agent data
		m.mu.Lock()
		m.agents = make(map[string]*pb.AgentInfo)
		m.agentList = msg.stats.Agents
		for _, agent := range msg.stats.Agents {
			m.agents[agent.AgentId] = agent
		}
		m.mu.Unlock()

		// Update table rows
		m.updateTableRows()
		return m, nil

	case monitorEvents.EventMsg:
		// Handle real-time agent events
		if msg.Event != nil {
			m.handleAgentEvent(msg.Event)
		}
		return m, nil
	}

	// Update table
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// View renders the agents view
func (m *AgentsModel) View() string {
	if m.width == 0 {
		return "Loading agents..."
	}

	if m.err != nil {
		return fmt.Sprintf("Error loading agents: %v\n\nPress 'r' to retry or '1' to return to dashboard", m.err)
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("62")).
		Render("Agents")

	m.mu.RLock()
	total := len(m.agentList)
	m.mu.RUnlock()

	stats := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Render(fmt.Sprintf("Total: %d", total))

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render("↑/↓: Navigate • r: Refresh • 1: Dashboard • q: Quit")

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

// Fetch fetches agent data from the control plane
func (m *AgentsModel) Fetch() tea.Cmd {
	return func() tea.Msg {
		m.loading = true

		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Fetch agent stats from control plane
		stats, err := m.client.ListAgents(ctx)
		if err != nil {
			return agentStatsMsg{err: err}
		}

		return agentStatsMsg{stats: stats}
	}
}

// updateTableRows updates the table with current agent data
func (m *AgentsModel) updateTableRows() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows := make([]table.Row, 0, len(m.agentList))

	for _, agent := range m.agentList {
		// Format status with color
		status := formatAgentStatus(agent.Status)

		// Format last seen
		lastSeen := "Never"
		if agent.LastHeartbeat != nil {
			lastSeen = formatTimeSince(agent.LastHeartbeat.AsTime())
		}

		// Get metadata
		hostname := "unknown"
		os := "unknown"
		ipAddr := "unknown"
		version := "unknown"

		if agent.Metadata != nil {
			hostname = agent.Metadata.Hostname
			os = agent.Metadata.Os
			if len(agent.Metadata.IpAddresses) > 0 {
				ipAddr = agent.Metadata.IpAddresses[0]
			}
			version = agent.Metadata.AgentVersion
		}

		rows = append(rows, table.Row{
			agent.AgentId,
			hostname,
			status,
			os,
			ipAddr,
			version,
			lastSeen,
		})
	}

	m.table.SetRows(rows)
}

// handleAgentEvent processes real-time agent events
func (m *AgentsModel) handleAgentEvent(event *events.Event) {
	switch event.Type {
	case events.EventTypeAgentConnect:
		// Refresh agent list on new connections
		// In a real implementation, we could update just this agent
		// For now, we'll trigger a refresh
		m.Fetch()()

	case events.EventTypeAgentDisconnect:
		// Update agent status or trigger refresh
		m.Fetch()()

	case events.EventTypeAgentHeartbeat:
		// Update last heartbeat time
		if agentID, ok := event.Data["agent_id"].(string); ok {
			m.mu.Lock()
			if agent, exists := m.agents[agentID]; exists {
				// Update last heartbeat time
				agent.LastHeartbeat = timestamppb.New(event.Time)
			}
			m.mu.Unlock()
			m.updateTableRows()
		}
	}
}

// formatAgentStatus formats agent status with color
func formatAgentStatus(status pb.AgentStatus) string {
	var color lipgloss.Color
	var text string

	switch status {
	case pb.AgentStatus_AGENT_STATUS_ONLINE:
		color = lipgloss.Color("10") // Green
		text = "Online"
	case pb.AgentStatus_AGENT_STATUS_OFFLINE:
		color = lipgloss.Color("9") // Red
		text = "Offline"
	case pb.AgentStatus_AGENT_STATUS_DEGRADED:
		color = lipgloss.Color("11") // Yellow
		text = "Degraded"
	default:
		color = lipgloss.Color("245") // Gray
		text = "Unknown"
	}

	return lipgloss.NewStyle().Foreground(color).Render(text)
}

// formatTimeSince formats a time as "Xs ago" or "Xm ago" or "Xh ago"
func formatTimeSince(t time.Time) string {
	duration := time.Since(t)

	if duration.Seconds() < 60 {
		return fmt.Sprintf("%.0fs ago", duration.Seconds())
	} else if duration.Minutes() < 60 {
		return fmt.Sprintf("%.0fm ago", duration.Minutes())
	} else if duration.Hours() < 24 {
		return fmt.Sprintf("%.0fh ago", duration.Hours())
	} else {
		return fmt.Sprintf("%.0fd ago", duration.Hours()/24)
	}
}
