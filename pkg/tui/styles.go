package tui

import (
	"strings"

	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
)

// Palette
var (
	colorMuted  = lipgloss.Color("240")
	colorSubtle = lipgloss.Color("245")
	colorNormal = lipgloss.Color("252")
	colorBright = lipgloss.Color("255")
	colorGreen  = lipgloss.Color("76")
	colorYellow = lipgloss.Color("220")
	colorRed    = lipgloss.Color("196")
	// Used in Commits 6-8:
	// colorBlue     = lipgloss.Color("69")
	// colorCyan     = lipgloss.Color("51")
	// colorSelected = lipgloss.Color("237") // row highlight background
)

// Layout styles
var (
	styleStatusBar = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(colorNormal).
			Padding(0, 1)

	styleHintBar = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1)

	styleSummaryBar = lipgloss.NewStyle().
			Foreground(colorSubtle).
			Padding(0, 1)

	styleTabActive = lipgloss.NewStyle().
			Foreground(colorBright).
			Bold(true).
			Underline(true)

	styleTabInactive = lipgloss.NewStyle().
				Foreground(colorMuted)

	// Used in Commits 6-8:
	// styleTableHeader = lipgloss.NewStyle().Foreground(colorSubtle).Bold(true)
	// styleSelectedRow = lipgloss.NewStyle().Background(colorSelected).Foreground(colorBright)
	// styleBorder      = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderForeground(colorMuted)
	// styleSectionTitle = lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
)

// Status indicator styles
var (
	styleConnected    = lipgloss.NewStyle().Foreground(colorGreen)
	styleDisconnected = lipgloss.NewStyle().Foreground(colorRed)
	styleSyncing      = lipgloss.NewStyle().Foreground(colorYellow)
)

// defaultTableStyles returns the shared lipgloss styles used by all overview tables.
func defaultTableStyles() table.Styles {
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(colorMuted).
		BorderBottom(true).
		Bold(true).
		Foreground(colorSubtle)
	s.Selected = s.Selected.
		Background(lipgloss.Color("237")).
		Foreground(colorBright).
		Bold(false)
	return s
}

// utilizationStyle returns a lipgloss.Style based on used/nominal ratio.
// Used in Commits 6-8 (queue/workload tables).
// func utilizationStyle(used, nominal int64) lipgloss.Style { ... }

// renderBar renders an inline utilization bar of the form "████░░ 32/40".
// Used in Commits 6-8 (queue/workload tables).
// func renderBar(used, nominal int64, width int) string { ... }

// centerBox centers a pre-rendered box string in the terminal window.
func centerBox(boxed string, termWidth, termHeight int) string {
	boxW := lipgloss.Width(boxed)
	boxH := strings.Count(boxed, "\n") + 1

	padLeft := max(0, (termWidth-boxW)/2)
	padTop := max(0, (termHeight-boxH)/2)

	leftPad := strings.Repeat(" ", padLeft)
	lines := strings.Split(boxed, "\n")
	for i, line := range lines {
		lines[i] = leftPad + line
	}
	return strings.Repeat("\n", padTop) + strings.Join(lines, "\n")
}
