package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jhwagner/kueue-bench/pkg/topology"
)

// clusterPickerModel is the cluster-switch overlay.
type clusterPickerModel struct {
	order      []string // stable display order (clusterOrder from parent)
	currentIdx int      // index of the cluster currently connected
	cursor     int      // highlighted row
}

func newClusterPicker(order []string, current string) clusterPickerModel {
	idx := 0
	for i, name := range order {
		if name == current {
			idx = i
			break
		}
	}
	return clusterPickerModel{
		order:      order,
		currentIdx: idx,
		cursor:     idx,
	}
}

func (p *clusterPickerModel) moveUp() {
	if p.cursor > 0 {
		p.cursor--
	}
}

func (p *clusterPickerModel) moveDown() {
	if p.cursor < len(p.order)-1 {
		p.cursor++
	}
}

// selected returns the cluster name at the cursor position.
func (p *clusterPickerModel) selected() string {
	if p.cursor >= 0 && p.cursor < len(p.order) {
		return p.order[p.cursor]
	}
	return ""
}

var (
	stylePickerBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorMuted).
				Padding(0, 1)

	stylePickerTitle    = lipgloss.NewStyle().Foreground(colorBright).Bold(true)
	stylePickerCurrent  = lipgloss.NewStyle().Foreground(colorMuted)
	stylePickerSelected = lipgloss.NewStyle().Background(lipgloss.Color("237")).Foreground(colorBright).Bold(true)
	stylePickerNormal   = lipgloss.NewStyle().Foreground(colorNormal)
)

// renderClusterPicker returns a centered overlay string for the cluster picker.
func renderClusterPicker(p clusterPickerModel, clusters map[string]topology.Cluster, termWidth, termHeight int) string {
	const innerWidth = 44

	var sb strings.Builder

	sb.WriteString(stylePickerTitle.Render("Select Cluster"))
	sb.WriteString("\n\n")

	for i, name := range p.order {
		cluster := clusters[name]
		role := cluster.Role
		if role == "" {
			role = "standalone"
		}

		label := fmt.Sprintf("%-22s (%s)", name, role)

		var prefix string
		if i == p.currentIdx {
			prefix = "● "
			label = stylePickerCurrent.Render(prefix + label)
		} else {
			prefix = "  "
			label = stylePickerNormal.Render(prefix + label)
		}

		if i == p.cursor {
			// Pad to innerWidth so the highlight spans the full row.
			visible := lipgloss.Width(label)
			if visible < innerWidth {
				label += strings.Repeat(" ", innerWidth-visible)
			}
			label = stylePickerSelected.Render(label)
		}

		sb.WriteString(label)
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(stylePickerCurrent.Render("[j/k] navigate  [enter] select  [esc] cancel"))

	boxed := stylePickerBorder.Width(innerWidth).Render(sb.String())

	// Center the box in the terminal.
	boxW := lipgloss.Width(boxed)
	boxH := strings.Count(boxed, "\n") + 1

	padLeft := (termWidth - boxW) / 2
	if padLeft < 0 {
		padLeft = 0
	}
	padTop := (termHeight - boxH) / 2
	if padTop < 0 {
		padTop = 0
	}

	leftPad := strings.Repeat(" ", padLeft)
	topPad := strings.Repeat("\n", padTop)

	lines := strings.Split(boxed, "\n")
	for i, line := range lines {
		lines[i] = leftPad + line
	}

	return topPad + strings.Join(lines, "\n")
}
