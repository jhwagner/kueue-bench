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

	// Pad to full width. UnsetPadding keeps the status bar background color
	// without applying styleStatusBar's Padding(0,1) to the fill characters.
	visible := lipgloss.Width(content)
	if visible < width {
		content += styleStatusBar.UnsetPadding().Render(strings.Repeat(" ", width-visible))
	}
	return content
}


// renderHintSep renders a thin horizontal rule above the hint bar.
func renderHintSep(width int) string {
	return lipgloss.NewStyle().Foreground(colorMuted).Render(strings.Repeat("─", width))
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
