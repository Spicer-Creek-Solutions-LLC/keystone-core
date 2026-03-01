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
)

// WebhookModel represents the webhooks view
type WebhookModel struct {
	config *config.Config
	client *client.Client
	table  table.Model
	width  int
	height int

	// Data
	subscriptions []client.WebhookSubscriptionResponse
	deliveries    []client.WebhookDeliveryResponse
	mu            sync.RWMutex

	// State
	viewMode string // "subscriptions" or "deliveries"
	loading  bool
	err      error
}

// Webhook messages
type webhookDataMsg struct {
	subscriptions []client.WebhookSubscriptionResponse
	deliveries    []client.WebhookDeliveryResponse
	err           error
}

// NewWebhookModel creates a new webhook model
func NewWebhookModel(cfg *config.Config, cli *client.Client) *WebhookModel {
	columns := []table.Column{
		{Title: "ID", Width: 20},
		{Title: "Name", Width: 20},
		{Title: "URL", Width: 30},
		{Title: "Events", Width: 20},
		{Title: "Status", Width: 10},
		{Title: "Created", Width: 18},
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

	return &WebhookModel{
		config:   cfg,
		client:   cli,
		table:    t,
		viewMode: "subscriptions",
	}
}

// Init initializes the webhook view
func (m *WebhookModel) Init() tea.Cmd {
	return m.Fetch()
}

// Update handles messages
func (m *WebhookModel) Update(msg tea.Msg) (interface{}, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height - 4
		m.table.SetHeight(m.height - 5)

	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			return m, m.Fetch()
		case "tab":
			if m.viewMode == "subscriptions" {
				m.viewMode = "deliveries"
				m.updateDeliveriesTable()
			} else {
				m.viewMode = "subscriptions"
				m.updateSubscriptionsTable()
			}
			return m, nil
		}

	case webhookDataMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.mu.Lock()
		m.subscriptions = msg.subscriptions
		m.deliveries = msg.deliveries
		m.mu.Unlock()

		if m.viewMode == "subscriptions" {
			m.updateSubscriptionsTable()
		} else {
			m.updateDeliveriesTable()
		}
		return m, nil
	}

	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// View renders the webhook view
func (m *WebhookModel) View() string {
	if m.width == 0 {
		return "Loading webhooks..."
	}

	if m.err != nil {
		return fmt.Sprintf("Error loading webhooks: %v\n\nPress 'r' to retry or '1' to return to dashboard", m.err)
	}

	viewTitle := "Webhook Subscriptions"
	if m.viewMode == "deliveries" {
		viewTitle = "Webhook Deliveries"
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("62")).
		Render(viewTitle)

	m.mu.RLock()
	count := len(m.subscriptions)
	if m.viewMode == "deliveries" {
		count = len(m.deliveries)
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

// Fetch fetches webhook data
func (m *WebhookModel) Fetch() tea.Cmd {
	return func() tea.Msg {
		m.loading = true

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		subs, err := m.client.ListWebhookSubscriptions(ctx)
		if err != nil {
			return webhookDataMsg{err: err}
		}

		// Collect deliveries from all subscriptions
		var allDeliveries []client.WebhookDeliveryResponse
		for _, sub := range subs {
			deliveries, err := m.client.GetWebhookDeliveries(ctx, sub.ID)
			if err != nil {
				continue
			}
			allDeliveries = append(allDeliveries, deliveries...)
		}

		return webhookDataMsg{
			subscriptions: subs,
			deliveries:    allDeliveries,
		}
	}
}

// updateSubscriptionsTable updates the table with subscription data
func (m *WebhookModel) updateSubscriptionsTable() {
	columns := []table.Column{
		{Title: "ID", Width: 20},
		{Title: "Name", Width: 20},
		{Title: "URL", Width: 30},
		{Title: "Events", Width: 20},
		{Title: "Status", Width: 10},
		{Title: "Created", Width: 18},
	}
	m.table.SetColumns(columns)

	m.mu.RLock()
	defer m.mu.RUnlock()

	rows := make([]table.Row, 0, len(m.subscriptions))
	for _, sub := range m.subscriptions {
		evts := ""
		if len(sub.Events) > 0 {
			evts = sub.Events[0]
			if len(sub.Events) > 1 {
				evts += fmt.Sprintf(" +%d", len(sub.Events)-1)
			}
		}

		url := sub.URL
		if len(url) > 30 {
			url = url[:27] + "..."
		}

		status := formatWebhookStatus(sub.Status)

		rows = append(rows, table.Row{
			sub.ID,
			sub.Name,
			url,
			evts,
			status,
			sub.CreatedAt,
		})
	}

	m.table.SetRows(rows)
}

// updateDeliveriesTable updates the table with delivery data
func (m *WebhookModel) updateDeliveriesTable() {
	columns := []table.Column{
		{Title: "ID", Width: 20},
		{Title: "Subscription", Width: 20},
		{Title: "Event", Width: 20},
		{Title: "Status Code", Width: 12},
		{Title: "Success", Width: 8},
		{Title: "Attempt", Width: 8},
		{Title: "Delivered", Width: 18},
	}
	m.table.SetColumns(columns)

	m.mu.RLock()
	defer m.mu.RUnlock()

	rows := make([]table.Row, 0, len(m.deliveries))
	for _, d := range m.deliveries {
		success := lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("Yes")
		if !d.Success {
			success = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("No")
		}

		rows = append(rows, table.Row{
			d.ID,
			d.SubscriptionID,
			d.EventType,
			fmt.Sprintf("%d", d.StatusCode),
			success,
			fmt.Sprintf("%d", d.Attempt),
			d.DeliveredAt,
		})
	}

	m.table.SetRows(rows)
}

// formatWebhookStatus formats webhook status with color
func formatWebhookStatus(status string) string {
	switch status {
	case "active":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("Active")
	case "paused":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render("Paused")
	case "disabled":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("Disabled")
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(status)
	}
}
