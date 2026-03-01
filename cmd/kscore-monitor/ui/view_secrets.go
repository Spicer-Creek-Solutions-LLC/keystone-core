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

type secretsMsg struct {
	leases *pb.ListLeasesResponse
	keys   *pb.ListSecretsResponse
	err    error
}

// SecretsModel represents the secrets and leases view.
type SecretsModel struct {
	config  *config.Config
	client  *client.Client
	tbl     table.Model
	width   int
	height  int
	ready   bool
	loading bool
	err     error

	leases       []*pb.LeaseInfo
	totalSecrets int
	active       int
	expiringSoon int
}

// NewSecretsModel creates a new secrets model.
func NewSecretsModel(cfg *config.Config, cli *client.Client) *SecretsModel {
	columns := []table.Column{
		{Title: "ID", Width: 14},
		{Title: "Secret Path", Width: 24},
		{Title: "Backend", Width: 12},
		{Title: "State", Width: 10},
		{Title: "TTL", Width: 10},
		{Title: "Expires At", Width: 20},
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

	return &SecretsModel{
		config: cfg,
		client: cli,
		tbl:    tbl,
	}
}

func (m *SecretsModel) Init() tea.Cmd  { return m.Fetch() }

func (m *SecretsModel) Update(msg tea.Msg) (interface{}, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height - 4
		m.ready = true
		m.tbl.SetHeight(m.height - 8)

	case secretsMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		if msg.leases != nil {
			m.leases = msg.leases.GetLeases()
		}
		if msg.keys != nil {
			m.totalSecrets = len(msg.keys.GetKeys())
		}
		m.computeStats()
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

func (m *SecretsModel) View() string {
	if !m.ready {
		return "Loading secrets..."
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("62")).
		Render("Secrets & Leases")

	var statusStr string
	if m.loading {
		statusStr = lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render("Loading...")
	} else if m.err != nil {
		statusStr = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("Error: %v", m.err))
	} else {
		expColor := "10"
		if m.expiringSoon > 0 {
			expColor = "226"
		}
		statusStr = fmt.Sprintf("Secrets: %d | Leases: %d | Active: %d | Expiring soon: %s",
			m.totalSecrets,
			len(m.leases),
			m.active,
			lipgloss.NewStyle().Foreground(lipgloss.Color(expColor)).Render(fmt.Sprintf("%d", m.expiringSoon)),
		)
	}

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render("↑/↓: Scroll • r: Refresh • 1: Dashboard • q: Quit")

	return lipgloss.JoinVertical(lipgloss.Left, title, statusStr, "", m.tbl.View(), "", help)
}

func (m *SecretsModel) Fetch() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return func() tea.Msg {
		m.loading = true
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		leases, lErr := m.client.ListLeases(ctx)
		keys, kErr := m.client.ListSecrets(ctx)

		var err error
		if lErr != nil {
			err = lErr
		} else if kErr != nil {
			err = kErr
		}

		return secretsMsg{leases: leases, keys: keys, err: err}
	}
}

func (m *SecretsModel) computeStats() {
	m.active = 0
	m.expiringSoon = 0
	now := time.Now()
	for _, l := range m.leases {
		if l.GetState() == "active" {
			m.active++
		}
		if l.GetExpiresAt() != nil {
			remaining := l.GetExpiresAt().AsTime().Sub(now)
			if remaining > 0 && remaining < 24*time.Hour {
				m.expiringSoon++
			}
		}
	}
}

func (m *SecretsModel) updateTable() {
	rows := make([]table.Row, 0, len(m.leases))
	now := time.Now()
	for _, l := range m.leases {
		ttl := ""
		expires := ""
		if l.GetExpiresAt() != nil {
			exp := l.GetExpiresAt().AsTime()
			expires = exp.Format("2006-01-02 15:04:05")
			remaining := exp.Sub(now)
			if remaining > 0 {
				ttl = formatDurationShort(remaining)
			} else {
				ttl = "expired"
			}
		}

		rows = append(rows, table.Row{
			truncate(l.GetId(), 14),
			truncate(l.GetSecretPath(), 24),
			truncate(l.GetBackend(), 12),
			l.GetState(),
			ttl,
			expires,
		})
	}
	m.tbl.SetRows(rows)
}

func formatDurationShort(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
}
