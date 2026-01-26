// Package wizard provides an interactive TUI for trust federation setup.
package wizard

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	// Colors
	colorPrimary   = lipgloss.Color("39")  // Light blue
	colorSuccess   = lipgloss.Color("82")  // Green
	colorWarning   = lipgloss.Color("214") // Orange
	colorError     = lipgloss.Color("196") // Red
	colorMuted     = lipgloss.Color("240") // Gray
	colorHighlight = lipgloss.Color("212") // Pink

	// Title styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			MarginBottom(1)

	// Step indicator
	stepStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	stepActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary)

	// Input styles
	promptStyle = lipgloss.NewStyle().
			Foreground(colorPrimary)

	inputHintStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true)

	// Status styles
	successStyle = lipgloss.NewStyle().
			Foreground(colorSuccess)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorError)

	warningStyle = lipgloss.NewStyle().
			Foreground(colorWarning)

	// Box styles
	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(1, 2)

	// Table styles
	tableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorPrimary).
				BorderStyle(lipgloss.NormalBorder()).
				BorderBottom(true).
				BorderForeground(colorMuted)

	tableCellStyle = lipgloss.NewStyle().
			Padding(0, 1)

	// Help text
	helpStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			MarginTop(1)

	// Spinner style
	spinnerStyle = lipgloss.NewStyle().
			Foreground(colorPrimary)

	// Check/cross marks
	checkMark = successStyle.Render("✓")
	crossMark = errorStyle.Render("✗")
	bullet    = lipgloss.NewStyle().Foreground(colorMuted).Render("•")
)

// formatHeader formats a header with step indicator.
func formatHeader(title string, step, total int) string {
	stepIndicator := stepStyle.Render(
		"Step " + stepActiveStyle.Render(itoa(step)) +
			stepStyle.Render("/"+itoa(total)),
	)
	return titleStyle.Render(title) + "\n" + stepIndicator + "\n"
}

// formatSection formats a section header.
func formatSection(title string) string {
	return subtitleStyle.Render(title)
}

// formatSuccess formats a success message.
func formatSuccess(msg string) string {
	return checkMark + " " + successStyle.Render(msg)
}

// formatError formats an error message.
func formatError(msg string) string {
	return crossMark + " " + errorStyle.Render(msg)
}

// formatWarning formats a warning message.
func formatWarning(msg string) string {
	return warningStyle.Render("! " + msg)
}

// formatHelp formats help text.
func formatHelp(text string) string {
	return helpStyle.Render(text)
}

// formatHint formats an input hint.
func formatHint(text string) string {
	return inputHintStyle.Render(text)
}

// formatBox wraps content in a styled box.
func formatBox(content string) string {
	return boxStyle.Render(content)
}

// itoa converts int to string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := make([]byte, 0, 10)
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}
	// Reverse
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits)
}
