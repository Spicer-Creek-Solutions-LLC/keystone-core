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
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

type clusterStatusMsg struct {
	status *pb.GetClusterStatusResponse
	leader *pb.GetClusterLeaderResponse
	err    error
}

// ClusterModel represents the cluster health view.
type ClusterModel struct {
	config  *config.Config
	client  *client.Client
	tbl     table.Model
	width   int
	height  int
	ready   bool
	loading bool
	err     error

	healthy     bool
	hasQuorum   bool
	memberCount int32
	quorumSize  int32
	leaderID    string
	leaderName  string
	members     []*pb.Member
}

// NewClusterModel creates a new cluster model.
func NewClusterModel(cfg *config.Config, cli *client.Client) *ClusterModel {
	columns := []table.Column{
		{Title: "ID", Width: 14},
		{Title: "Name", Width: 16},
		{Title: "Address", Width: 20},
		{Title: "Status", Width: 12},
		{Title: "Leader", Width: 8},
		{Title: "Agents", Width: 8},
		{Title: "Jobs", Width: 6},
		{Title: "Last Heartbeat", Width: 18},
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

	return &ClusterModel{
		config: cfg,
		client: cli,
		tbl:    tbl,
	}
}

// Init initializes the cluster model.
func (m *ClusterModel) Init() tea.Cmd {
	return m.Fetch()
}

// Update handles messages.
func (m *ClusterModel) Update(msg tea.Msg) (interface{}, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height - 4
		m.ready = true
		m.tbl.SetHeight(m.height - 10)

	case clusterStatusMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		if msg.status != nil {
			m.healthy = msg.status.GetHealthy()
			m.hasQuorum = msg.status.GetHasQuorum()
			m.memberCount = msg.status.GetMemberCount()
			m.quorumSize = msg.status.GetQuorumSize()
			m.leaderID = msg.status.GetLeaderId()
			m.members = msg.status.GetMembers()
		}
		if msg.leader != nil && msg.leader.GetLeader() != nil {
			m.leaderName = msg.leader.GetLeader().GetName()
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

// View renders the cluster view.
func (m *ClusterModel) View() string {
	if !m.ready {
		return "Loading cluster health..."
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("62")).
		Render("Cluster Health")

	var statusStr string
	if m.loading {
		statusStr = lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render("Loading...")
	} else if m.err != nil {
		statusStr = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("Error: %v", m.err))
	} else {
		healthColor := "10"
		healthText := "Healthy"
		if !m.healthy {
			healthColor = "196"
			healthText = "Unhealthy"
		}

		quorumColor := "10"
		quorumText := "Yes"
		if !m.hasQuorum {
			quorumColor = "196"
			quorumText = "No"
		}

		leaderDisplay := m.leaderName
		if leaderDisplay == "" {
			leaderDisplay = m.leaderID
		}
		if leaderDisplay == "" {
			leaderDisplay = "none"
		}

		statusStr = fmt.Sprintf(
			"Health: %s | Quorum: %s | Members: %d/%d | Leader: %s",
			lipgloss.NewStyle().Foreground(lipgloss.Color(healthColor)).Render(healthText),
			lipgloss.NewStyle().Foreground(lipgloss.Color(quorumColor)).Render(quorumText),
			m.memberCount,
			m.quorumSize,
			leaderDisplay,
		)
	}

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render("↑/↓: Scroll • r: Refresh • 1: Dashboard • q: Quit")

	return lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		statusStr,
		"",
		m.tbl.View(),
		"",
		help,
	)
}

// Fetch retrieves cluster data.
func (m *ClusterModel) Fetch() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		m.loading = true
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		status, sErr := m.client.GetClusterStatus(ctx)
		leader, lErr := m.client.GetLeader(ctx)

		var err error
		if sErr != nil {
			err = sErr
		} else if lErr != nil {
			err = lErr
		}

		return clusterStatusMsg{status: status, leader: leader, err: err}
	}
}

func (m *ClusterModel) updateTable() {
	rows := make([]table.Row, 0, len(m.members))
	for _, member := range m.members {
		isLeader := "No"
		if member.GetIsLeader() {
			isLeader = "Yes"
		}

		heartbeat := ""
		if member.GetLastHeartbeat() != nil {
			heartbeat = formatTimeSince(member.GetLastHeartbeat().AsTime())
		}

		rows = append(rows, table.Row{
			truncate(member.GetId(), 14),
			truncate(member.GetName(), 16),
			truncate(member.GetAddress(), 20),
			formatClusterMemberStatus(member.GetStatus()),
			isLeader,
			fmt.Sprintf("%d", member.GetAgentCount()),
			fmt.Sprintf("%d", member.GetJobCount()),
			heartbeat,
		})
	}
	m.tbl.SetRows(rows)
}

func formatClusterMemberStatus(s pb.ClusterMemberStatus) string {
	switch s {
	case pb.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_HEALTHY:
		return "Healthy"
	case pb.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_DEGRADED:
		return "Degraded"
	case pb.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_UNHEALTHY:
		return "Unhealthy"
	case pb.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_LEAVING:
		return "Leaving"
	default:
		return "Unknown"
	}
}
