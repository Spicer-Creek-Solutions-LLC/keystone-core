package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

type logLineMsg string
type runResultMsg struct {
	err error
}

type progressModel struct {
	spinner   spinner.Model
	phase     string
	completed int
	total     int
	logs      []string
	done      bool
	err       error
	width     int
	height    int
	logsCh    <-chan string
	resultCh  <-chan error
}

// RunProgress runs a bootstrap operation and renders progress/logs in a TUI.
func RunProgress(ctx context.Context, run func(io.Writer) error) error {
	logsCh := make(chan string, 128)
	resultCh := make(chan error, 1)
	writer := &lineWriter{ch: logsCh}

	go func() {
		resultCh <- run(writer)
		close(resultCh)
		close(logsCh)
	}()

	model := progressModel{
		spinner:  spinner.New(),
		logsCh:   logsCh,
		resultCh: resultCh,
	}
	model.spinner.Spinner = spinner.Line

	program := tea.NewProgram(model)
	finalModel, err := program.Run()
	if err != nil {
		return err
	}
	result, ok := finalModel.(progressModel)
	if !ok {
		return fmt.Errorf("unexpected progress model")
	}
	if result.err != nil {
		return result.err
	}
	return ctx.Err()
}

func (m progressModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, waitForLog(m.logsCh), waitForResult(m.resultCh))
}

func (m progressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.err = context.Canceled
			m.done = true
			return m, tea.Quit
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case logLineMsg:
		m.handleLogLine(string(msg))
		return m, waitForLog(m.logsCh)
	case runResultMsg:
		if msg.err != nil {
			m.err = msg.err
		}
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func (m progressModel) View() string {
	var builder strings.Builder
	builder.WriteString("Bootstrap progress\n\n")
	if m.done {
		switch {
		case m.err != nil && !errors.Is(m.err, context.Canceled):
			builder.WriteString(fmt.Sprintf("Status: failed (%v)\n", m.err))
		case errors.Is(m.err, context.Canceled):
			builder.WriteString("Status: canceled\n")
		default:
			builder.WriteString("Status: complete\n")
		}
	} else {
		builder.WriteString(fmt.Sprintf("%s Phase: %s (%d/%d)\n", m.spinner.View(), m.phase, m.completed, m.total))
	}

	if len(m.logs) > 0 {
		builder.WriteString("\nLogs:\n")
		for _, line := range m.visibleLogs() {
			builder.WriteString(line)
			builder.WriteByte('\n')
		}
	}

	return builder.String()
}

func (m *progressModel) handleLogLine(line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	if payload := parseProgressEvent(line); payload != nil {
		switch payload.event {
		case "phase":
			m.phase = payload.phase
			m.completed = payload.completed
			m.total = payload.total
			return
		case "error":
			if payload.err != "" {
				m.err = errors.New(payload.err)
			}
			return
		case "complete":
			return
		}
	}
	m.logs = append(m.logs, line)
	if len(m.logs) > 200 {
		m.logs = m.logs[len(m.logs)-200:]
	}
}

func (m progressModel) visibleLogs() []string {
	if m.height <= 6 {
		return m.logs
	}
	maxLines := m.height - 6
	if maxLines <= 0 || len(m.logs) <= maxLines {
		return m.logs
	}
	return m.logs[len(m.logs)-maxLines:]
}

func waitForLog(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return nil
		}
		return logLineMsg(line)
	}
}

func waitForResult(ch <-chan error) tea.Cmd {
	return func() tea.Msg {
		err, ok := <-ch
		if !ok {
			return nil
		}
		return runResultMsg{err: err}
	}
}

type lineWriter struct {
	mu  sync.Mutex
	buf string
	ch  chan<- string
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf += string(p)
	lines := strings.Split(w.buf, "\n")
	for i := 0; i < len(lines)-1; i++ {
		w.ch <- lines[i]
	}
	w.buf = lines[len(lines)-1]
	return len(p), nil
}

type progressEvent struct {
	event     string
	phase     string
	completed int
	total     int
	err       string
}

func parseProgressEvent(line string) *progressEvent {
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		return nil
	}
	event, _ := payload["event"].(string)
	if event == "" {
		return nil
	}
	result := &progressEvent{event: event}
	if phase, ok := payload["phase"].(string); ok {
		result.phase = phase
	}
	if completed, ok := payload["completed"].(float64); ok {
		result.completed = int(completed)
	}
	if total, ok := payload["total"].(float64); ok {
		result.total = int(total)
	}
	if errMsg, ok := payload["error"].(string); ok {
		result.err = errMsg
	}
	return result
}
