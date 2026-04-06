package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jhwagner/kueue-bench/pkg/watcher"
)

// eventViewModel renders the event log panel using a scrollable viewport.
type eventViewModel struct {
	vp           viewport.Model
	userScrolled bool // true when user has scrolled up; stops auto-follow
}

func newEventView(width, height int) eventViewModel {
	vp := viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))
	return eventViewModel{vp: vp}
}

func (m *eventViewModel) update(msg tea.Msg) tea.Cmd {
	// Track whether the user moves away from the bottom (voluntarily scrolled up).
	wasAtBottom := m.vp.AtBottom()

	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)

	// If the user just moved up from the bottom, mark as manually scrolled.
	if wasAtBottom && !m.vp.AtBottom() {
		m.userScrolled = true
	}
	// If the user scrolled back to the bottom, resume auto-follow.
	if m.vp.AtBottom() {
		m.userScrolled = false
	}
	return cmd
}

func (m eventViewModel) view() string {
	return m.vp.View()
}

// refresh rebuilds the event log content from the snapshot.
func (m *eventViewModel) refresh(snap watcher.Snapshot, width, height int) {
	atBottom := !m.userScrolled

	m.vp.SetWidth(width)
	m.vp.SetHeight(height)

	m.vp.SetContent(renderEventLog(snap.Events, width))

	if atBottom {
		m.vp.GotoBottom()
	}
}

// renderEventLog formats all events into a single content string.
func renderEventLog(events []watcher.EventEntry, width int) string {
	if len(events) == 0 {
		return styleMuted.Render("  No events yet.")
	}

	var sb strings.Builder
	for _, e := range events {
		line := formatEvent(e, width)
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	return strings.TrimRight(sb.String(), "\n")
}

// formatEvent renders a single EventEntry as a terminal line.
func formatEvent(e watcher.EventEntry, width int) string {
	timeStr := e.Time.Format("15:04:05")
	reason := fmt.Sprintf("[%-8s]", truncate(e.Reason, 8))
	obj := truncate(e.Object, 30)
	msg := e.Message

	reasonStyle := styleEventNormal
	if e.Type == "Warning" {
		reasonStyle = styleEventWarning
	}

	prefix := styleMuted.Render(timeStr) + " " + reasonStyle.Render(reason) + " "
	prefixWidth := lipgloss.Width(prefix)

	// Truncate message to fit terminal width.
	maxMsg := width - prefixWidth - len(obj) - 2
	if maxMsg < 10 {
		maxMsg = 10
	}
	if len([]rune(msg)) > maxMsg {
		msg = truncate(msg, maxMsg)
	}

	return prefix + styleMuted.Render(obj) + "  " + styleNormal.Render(msg)
}

// eventSeparator renders a thin horizontal divider with a "Events" label.
func eventSeparator(width int) string {
	label := " Events "
	labelWidth := lipgloss.Width(label)
	lineWidth := width - labelWidth - 2
	if lineWidth < 0 {
		lineWidth = 0
	}
	left := 2
	right := lineWidth - left
	if right < 0 {
		right = 0
	}
	sep := styleSeparator.Render(
		strings.Repeat("─", left) + label + strings.Repeat("─", right),
	)
	return sep
}

// Event-specific styles (kept here, near where they're used).
var (
	styleEventNormal  = lipgloss.NewStyle().Foreground(colorGreen)
	styleEventWarning = lipgloss.NewStyle().Foreground(colorYellow)
	styleNormal       = lipgloss.NewStyle().Foreground(colorNormal)
	styleMuted        = lipgloss.NewStyle().Foreground(colorMuted)
	styleSeparator    = lipgloss.NewStyle().Foreground(colorMuted)
)
