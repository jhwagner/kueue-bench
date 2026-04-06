package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// connectionState represents the watcher connection state for display.
type connectionState int

const (
	stateConnecting connectionState = iota
	stateConnected
	stateDisconnected
)

// renderTopBar renders the top status bar:
// "  <topology> │ Cluster: <name> (<role>) │ ● Connected  "
func renderTopBar(width int, topologyName, clusterName, clusterRole string, state connectionState) string {
	indicator, stateStr := connectionIndicator(state)

	left := topologyName
	mid := fmt.Sprintf("Cluster: %s", clusterName)
	if clusterRole != "" {
		mid += fmt.Sprintf(" (%s)", clusterRole)
	}
	right := indicator + " " + stateStr

	// Distribute sections with separators.
	sep := styleStatusBar.Foreground(colorMuted).Render(" │ ")
	content := styleStatusBar.Foreground(colorBright).Bold(true).Render(left) +
		sep +
		styleStatusBar.Render(mid) +
		sep +
		styleStatusBar.Render(right)

	// Pad to full width.
	visible := lipgloss.Width(content)
	if visible < width {
		content += styleStatusBar.Render(strings.Repeat(" ", width-visible))
	}
	return content
}

// renderHintBar renders the bottom key hint bar.
func renderHintBar(width int, hints string) string {
	content := styleHintBar.Render(hints)
	visible := lipgloss.Width(content)
	if visible < width {
		content += styleHintBar.Render(strings.Repeat(" ", width-visible))
	}
	return content
}

func connectionIndicator(state connectionState) (indicator, label string) {
	switch state {
	case stateConnected:
		return styleConnected.Render("●"), "Connected"
	case stateDisconnected:
		return styleDisconnected.Render("○"), "Disconnected"
	default:
		return styleSyncing.Render("◌"), "Syncing..."
	}
}
