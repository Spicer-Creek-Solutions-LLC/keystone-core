package ui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/shawnbutts/keystone-core/cmd/kscore-monitor/client"
	"github.com/shawnbutts/keystone-core/cmd/kscore-monitor/config"
	monitorEvents "github.com/shawnbutts/keystone-core/cmd/kscore-monitor/events"
	"github.com/shawnbutts/keystone-core/internal/events"
)

// EventsModel represents the events view
type EventsModel struct {
	config   *config.Config
	client   *client.Client
	viewport viewport.Model
	filter   textinput.Model
	width    int
	height   int

	// Event data
	allEvents      []*events.Event
	filteredEvents []*events.Event
	mu             sync.RWMutex

	// State
	filterActive    bool
	paused          bool
	ready           bool
	correlationMode bool
	correlationID   string
}

// NewEventsModel creates a new events model
func NewEventsModel(cfg *config.Config, cli *client.Client) *EventsModel {
	vp := viewport.New(0, 0)
	vp.Style = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1)

	ti := textinput.New()
	ti.Placeholder = "Filter events (type to search)..."
	ti.CharLimit = 100

	return &EventsModel{
		config:         cfg,
		client:         cli,
		viewport:       vp,
		filter:         ti,
		allEvents:      make([]*events.Event, 0, cfg.EventBufferSize),
		filteredEvents: make([]*events.Event, 0, cfg.EventBufferSize),
	}
}

// Init initializes the events view
func (m *EventsModel) Init() tea.Cmd {
	return m.Fetch()
}

// Update handles messages
func (m *EventsModel) Update(msg tea.Msg) (interface{}, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height - 4

		if !m.ready {
			m.viewport.Width = m.width - 4
			m.viewport.Height = m.height - 8 // Account for title, stats, filter, help
			m.ready = true
		} else {
			m.viewport.Width = m.width - 4
			m.viewport.Height = m.height - 8
		}
		m.updateViewport()

	case tea.KeyMsg:
		if m.filterActive {
			switch msg.String() {
			case "esc":
				m.filterActive = false
				m.filter.Blur()
				return m, nil
			case "enter":
				m.filterActive = false
				m.filter.Blur()
				m.applyFilter()
				return m, nil
			}
			m.filter, cmd = m.filter.Update(msg)
			cmds = append(cmds, cmd)
			// Apply filter on every keystroke
			m.applyFilter()
		} else {
			switch msg.String() {
			case "/":
				m.filterActive = true
				m.filter.Focus()
				return m, textinput.Blink
			case "c":
				m.clearEvents()
				return m, nil
			case "p":
				m.paused = !m.paused
				return m, nil
			case "enter":
				if !m.correlationMode {
					m.enterCorrelation()
					return m, nil
				}
			case "esc":
				if m.correlationMode {
					m.correlationMode = false
					m.correlationID = ""
					m.applyFilter()
					return m, nil
				}
			}
		}

	case monitorEvents.EventMsg:
		// Add new event if not paused
		if !m.paused && msg.Event != nil {
			m.addEvent(msg.Event)
		}
		return m, nil
	}

	// Update viewport
	if !m.filterActive {
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// View renders the events view
func (m *EventsModel) View() string {
	if !m.ready {
		return "Loading events..."
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("62")).
		Render("Event Stream")

	m.mu.RLock()
	total := len(m.allEvents)
	filtered := len(m.filteredEvents)
	m.mu.RUnlock()

	statusParts := []string{
		fmt.Sprintf("Total: %d", total),
		fmt.Sprintf("Showing: %d", filtered),
	}
	if m.paused {
		statusParts = append(statusParts, lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Render("PAUSED"))
	}
	if m.correlationMode {
		statusParts = append(statusParts, lipgloss.NewStyle().
			Foreground(lipgloss.Color("141")).
			Render(fmt.Sprintf("Correlation: %s", m.correlationID[:min(8, len(m.correlationID))])))
	}

	stats := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Render(strings.Join(statusParts, " | "))

	// Filter input
	filterLabel := "Filter: "
	if m.filterActive {
		filterLabel = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Render("Filter: ")
	}
	filterView := filterLabel + m.filter.View()

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render("↑/↓: Scroll • /: Filter • c: Clear • p: Pause • 1: Dashboard • q: Quit")

	return lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		stats,
		"",
		m.viewport.View(),
		"",
		filterView,
		help,
	)
}

// Fetch fetches event data (no-op for events - they come via subscription)
func (m *EventsModel) Fetch() tea.Cmd {
	return nil
}

// addEvent adds a new event to the list
func (m *EventsModel) addEvent(event *events.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Prepend new event (newest first)
	m.allEvents = append([]*events.Event{event}, m.allEvents...)

	// Trim to buffer size
	if len(m.allEvents) > m.config.EventBufferSize {
		m.allEvents = m.allEvents[:m.config.EventBufferSize]
	}

	// Update filtered events
	m.applyFilterLocked()
	m.updateViewport()
}

// clearEvents clears all events
func (m *EventsModel) clearEvents() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.allEvents = make([]*events.Event, 0, m.config.EventBufferSize)
	m.filteredEvents = make([]*events.Event, 0, m.config.EventBufferSize)
	m.updateViewport()
}

// applyFilter applies the current filter
func (m *EventsModel) applyFilter() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.applyFilterLocked()
	m.updateViewport()
}

// applyFilterLocked applies the filter (must hold lock)
func (m *EventsModel) applyFilterLocked() {
	filterText := strings.ToLower(m.filter.Value())

	if filterText == "" {
		m.filteredEvents = m.allEvents
		return
	}

	m.filteredEvents = make([]*events.Event, 0)
	for _, event := range m.allEvents {
		if m.matchesFilter(event, filterText) {
			m.filteredEvents = append(m.filteredEvents, event)
		}
	}
}

// matchesFilter checks if an event matches the filter
func (m *EventsModel) matchesFilter(event *events.Event, filter string) bool {
	// Check type
	if strings.Contains(strings.ToLower(string(event.Type)), filter) {
		return true
	}
	// Check source
	if strings.Contains(strings.ToLower(event.Source), filter) {
		return true
	}
	// Check severity
	if strings.Contains(strings.ToLower(string(event.Severity)), filter) {
		return true
	}
	// Check tags
	for k, v := range event.Tags {
		if strings.Contains(strings.ToLower(k), filter) ||
			strings.Contains(strings.ToLower(v), filter) {
			return true
		}
	}
	return false
}

// updateViewport updates the viewport content
func (m *EventsModel) updateViewport() {
	lines := make([]string, 0, len(m.filteredEvents))

	for _, event := range m.filteredEvents {
		lines = append(lines, formatEventDetailed(event))
	}

	if len(lines) == 0 {
		lines = []string{"No events to display"}
	}

	m.viewport.SetContent(strings.Join(lines, "\n"))
}

// enterCorrelation filters events by the selected event's correlation ID.
func (m *EventsModel) enterCorrelation() {
	// Find the event at the current viewport cursor position
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Look for an event with a correlation ID
	for _, event := range m.filteredEvents {
		if event.CorrelationID != "" {
			m.correlationMode = true
			m.correlationID = event.CorrelationID

			// Filter to just correlated events
			var correlated []*events.Event
			for _, e := range m.allEvents {
				if e.CorrelationID == m.correlationID {
					correlated = append(correlated, e)
				}
			}
			m.filteredEvents = correlated
			m.updateViewport()
			return
		}
	}
}

// formatEventDetailed formats an event with full details
func formatEventDetailed(e *events.Event) string {
	// Color code by severity
	var color lipgloss.Color
	switch e.Severity {
	case events.SeverityCritical:
		color = lipgloss.Color("196") // Red
	case events.SeverityError:
		color = lipgloss.Color("208") // Orange
	case events.SeverityWarning:
		color = lipgloss.Color("226") // Yellow
	case events.SeverityInfo:
		color = lipgloss.Color("39") // Blue
	default:
		color = lipgloss.Color("245") // Gray
	}

	timestamp := e.Time.Format("2006-01-02 15:04:05")
	timestampStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	severityStyle := lipgloss.NewStyle().Foreground(color).Bold(true)
	typeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("141"))
	sourceStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("69"))

	line := fmt.Sprintf("%s [%s] %s from %s",
		timestampStyle.Render(timestamp),
		severityStyle.Render(strings.ToUpper(string(e.Severity))),
		typeStyle.Render(string(e.Type)),
		sourceStyle.Render(e.Source))

	// Add correlation ID if present
	if e.CorrelationID != "" {
		corrStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
		line += corrStyle.Render(fmt.Sprintf(" [corr:%s]", e.CorrelationID[:8]))
	}

	// Add tags if present
	if len(e.Tags) > 0 {
		tags := make([]string, 0, len(e.Tags))
		for k, v := range e.Tags {
			tags = append(tags, fmt.Sprintf("%s=%s", k, v))
		}
		tagStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
		line += tagStyle.Render(fmt.Sprintf(" {%s}", strings.Join(tags, ", ")))
	}

	return line
}
