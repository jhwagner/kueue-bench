package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/jhwagner/kueue-bench/pkg/watcher"
)

// queueDetailModel is the full-screen ClusterQueue drill-down view.
// It renders into a scrollable viewport; parent (tui.go) draws the top/hint bars.
type queueDetailModel struct {
	queueName     string
	vp            viewport.Model
	width         int
	height        int // available lines between topBar and hintBar
	policyVisible bool
	lastSnap      watcher.Snapshot
}

func newQueueDetail(queueName string, snap watcher.Snapshot, width, height int) queueDetailModel {
	m := queueDetailModel{
		queueName:     queueName,
		width:         width,
		height:        height,
		policyVisible: false,
		lastSnap:      snap,
	}
	m.vp = viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))
	m.vp.SetContent(m.buildContent(snap))
	return m
}

func (m queueDetailModel) Init() tea.Cmd { return nil }

func (m queueDetailModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height - 2 // topBar + hintBar
		if m.height < 1 {
			m.height = 1
		}
		m.vp.SetWidth(m.width)
		m.vp.SetHeight(m.height)
		return m, nil

	case snapshotMsg:
		m.lastSnap = msg.snap
		m.vp.SetContent(m.buildContent(msg.snap))
		return m, nil

	case tea.KeyPressMsg:
		if msg.String() == "p" {
			m.policyVisible = !m.policyVisible
			m.vp.SetContent(m.buildContent(m.lastSnap))
			return m, nil
		}
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m queueDetailModel) View() tea.View {
	return tea.NewView(m.vp.View())
}

// buildContent renders the full detail content string for the viewport.
func (m queueDetailModel) buildContent(snap watcher.Snapshot) string {
	q, ok := snap.Queues[m.queueName]
	if !ok {
		return styleMuted.Render(fmt.Sprintf("  ClusterQueue %q not found", m.queueName))
	}

	var sb strings.Builder

	// Header
	sb.WriteString(renderQueueDetailHeader(q))
	sb.WriteString("\n")

	// Policy (collapsible)
	policyToggle := "[p] show policy"
	if m.policyVisible {
		policyToggle = "[p] hide policy"
	}
	sb.WriteString(renderDetailSectionHeader("Policy  "+styleMuted.Render(policyToggle), m.width))
	sb.WriteString("\n")
	if m.policyVisible {
		sb.WriteString(renderQueueDetailPolicy(q))
		sb.WriteString("\n")
	}

	// Resources
	sb.WriteString(renderDetailSectionHeader("Resources", m.width))
	sb.WriteString("\n")
	sb.WriteString(renderQueueDetailResources(q))
	sb.WriteString("\n")

	// Workloads
	activeWls := collectQueueWorkloads(snap.Workloads, q.Name)
	sb.WriteString(renderDetailSectionHeader(fmt.Sprintf("Workloads (%d active)", len(activeWls)), m.width))
	sb.WriteString("\n")
	sb.WriteString(renderQueueDetailWorkloads(activeWls))
	sb.WriteString("\n")

	// Events
	queueEvents := collectQueueEvents(snap.Events, q.Name)
	sb.WriteString(renderDetailSectionHeader(fmt.Sprintf("Events (%d)", len(queueEvents)), m.width))
	sb.WriteString("\n")
	sb.WriteString(renderQueueDetailEvents(queueEvents, m.width))

	return sb.String()
}

// --- Section renderers -------------------------------------------------------

func renderQueueDetailHeader(q watcher.QueueSnapshot) string {
	statusDot := styleConnected.Render("●") + " Active"
	if !q.Active {
		statusDot = styleDisconnected.Render("○") + " Inactive"
	}

	namePart := lipgloss.NewStyle().Foreground(colorBright).Bold(true).Render("  " + q.Name)

	metaParts := []string{}
	if q.Cohort != "" {
		metaParts = append(metaParts, styleMuted.Render("cohort:")+
			lipgloss.NewStyle().Foreground(colorNormal).Render(" "+q.Cohort))
	}
	metaParts = append(metaParts, statusDot)
	meta := strings.Join(metaParts, "   ")

	counts := fmt.Sprintf("  Pending: %d   Reserving: %d   Admitted: %d",
		q.Pending, q.Reserving, q.Admitted)

	return namePart + "   " + meta + "\n" +
		lipgloss.NewStyle().Foreground(colorSubtle).Render(counts)
}

func renderQueueDetailPolicy(q watcher.QueueSnapshot) string {
	preempt := q.Preemption

	reclaimVal := preempt.ReclaimWithinCohort
	if reclaimVal == "" {
		reclaimVal = "Never"
	}
	borrowCohortVal := preempt.BorrowWithinCohort
	if borrowCohortVal == "" {
		borrowCohortVal = "Never"
	}
	withinVal := preempt.WithinClusterQueue
	if withinVal == "" {
		withinVal = "Never"
	}

	row1 := fmt.Sprintf("  %-26s  %-26s",
		"ReclaimWithinCohort: "+reclaimVal,
		"BorrowWithinCohort: "+borrowCohortVal,
	)
	row2 := fmt.Sprintf("  %-26s  %-26s",
		"WithinClusterQueue:  "+withinVal,
		"FairSharingWeight:   "+fmtFairSharingWeight(q.FairSharingWeight),
	)

	style := lipgloss.NewStyle().Foreground(colorNormal)
	return style.Render(row1) + "\n" + style.Render(row2)
}

func fmtFairSharingWeight(w *resource.Quantity) string {
	if w == nil {
		return "–"
	}
	return w.String()
}

// renderQueueDetailResources renders one flavor block per flavor, each with
// resource rows indented beneath a flavor header.
func renderQueueDetailResources(q watcher.QueueSnapshot) string {
	if len(q.Flavors) == 0 {
		return styleMuted.Render("  No resource flavors configured.")
	}

	colHdr := fmt.Sprintf("    %-20s  %6s  %7s  %8s  %10s  %8s",
		"RESOURCE", "USED", "NOMINAL", "BORROWED", "BORROW-LMT", "LEND-LMT")
	styleHdr := lipgloss.NewStyle().Foreground(colorSubtle).Bold(true)

	var sb strings.Builder

	for i, fl := range q.Flavors {
		if i > 0 {
			sb.WriteString("\n")
		}

		// Flavor header
		flavorHeader := lipgloss.NewStyle().Foreground(colorBright).Bold(true).Render("  " + fl.Name)
		sb.WriteString(flavorHeader)
		sb.WriteString("\n")
		sb.WriteString(styleHdr.Render(colHdr))
		sb.WriteString("\n")

		resources := sortedFlavorResources(fl)
		for _, rName := range resources {
			rs := fl.Resources[rName]
			used := quantityValue(rName, rs.Used)
			nominal := quantityValue(rName, rs.Nominal)
			borrowLmt := fmtQtyPtr(rName, rs.BorrowingLimit)
			lendLmt := fmtQtyPtr(rName, rs.LendingLimit)

			utilStyle := utilizationStyle(used, nominal)
			bar := renderBar(used, nominal, barWidth)
			coloredBar := utilStyle.Render(bar)
			usedNominal := utilStyle.Render(
				fmt.Sprintf("%-13s", fmt.Sprintf("%d/%d", used, nominal)),
			)

			row := fmt.Sprintf("    %-20s  %s %s  %8s  %10s  %8s",
				truncate(string(rName), 20),
				coloredBar,
				usedNominal,
				fmtQty(rName, rs.Borrowed),
				borrowLmt,
				lendLmt,
			)
			sb.WriteString(row)
			sb.WriteString("\n")
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

// sortedFlavorResources returns the resource names for a flavor in priority order.
func sortedFlavorResources(fl watcher.FlavorSnapshot) []corev1.ResourceName {
	names := make([]corev1.ResourceName, 0, len(fl.Resources))
	for rName := range fl.Resources {
		names = append(names, rName)
	}
	sortResourceNames(names)
	return names
}

func renderQueueDetailWorkloads(wls []watcher.WorkloadSnapshot) string {
	if len(wls) == 0 {
		return styleMuted.Render("  No active workloads.")
	}

	hdr := fmt.Sprintf("  %-30s  %-14s  %6s  %s",
		"NAME", "STATUS", "AGE", "RESOURCES")
	styleHdr := lipgloss.NewStyle().Foreground(colorSubtle).Bold(true)

	var sb strings.Builder
	sb.WriteString(styleHdr.Render(hdr))
	sb.WriteString("\n")

	for _, wl := range wls {
		statusStr := renderWorkloadStatus(wl.Status)
		resources := renderWorkloadResources(wl.Resources)
		row := fmt.Sprintf("  %-30s  %-14s  %6s  %s",
			truncate(wl.Name, 30),
			statusStr,
			fmtAge(wl.CreatedAt),
			resources,
		)
		sb.WriteString(row)
		sb.WriteString("\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}

func renderQueueDetailEvents(events []watcher.EventEntry, width int) string {
	if len(events) == 0 {
		return styleMuted.Render("  No events for this queue.")
	}

	// Show most recent 20 events, newest first.
	start := 0
	if len(events) > 20 {
		start = len(events) - 20
	}

	var sb strings.Builder
	for i := len(events) - 1; i >= start; i-- {
		line := "  " + formatEvent(events[i], width-2)
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// --- Collectors ---------------------------------------------------------------

func collectQueueWorkloads(workloads map[string]watcher.WorkloadSnapshot, queueName string) []watcher.WorkloadSnapshot {
	var out []watcher.WorkloadSnapshot
	for _, wl := range workloads {
		if wl.ClusterQueue != queueName {
			continue
		}
		if wl.Status == watcher.WorkloadStatusFinished || wl.Status == watcher.WorkloadStatusEvicted {
			continue
		}
		out = append(out, wl)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func collectQueueEvents(events []watcher.EventEntry, queueName string) []watcher.EventEntry {
	var out []watcher.EventEntry
	for _, e := range events {
		if strings.Contains(e.Object, queueName) {
			out = append(out, e)
		}
	}
	return out
}

// --- Quantity formatting helpers ---------------------------------------------

// fmtQty formats a resource quantity for display with appropriate units.
func fmtQty(rName corev1.ResourceName, q resource.Quantity) string {
	switch rName {
	case corev1.ResourceMemory:
		gib := q.Value() / (1024 * 1024 * 1024)
		return fmt.Sprintf("%dGi", gib)
	case corev1.ResourceCPU:
		return fmt.Sprintf("%d", q.MilliValue()/1000)
	default:
		return fmt.Sprintf("%d", q.Value())
	}
}

// fmtQtyPtr formats an optional resource quantity, returning "–" for nil.
func fmtQtyPtr(rName corev1.ResourceName, q *resource.Quantity) string {
	if q == nil {
		return "–"
	}
	return fmtQty(rName, *q)
}

// --- Section header ----------------------------------------------------------

var styleSectionHeader = lipgloss.NewStyle().Foreground(colorMuted)

func renderDetailSectionHeader(title string, width int) string {
	// title may contain ANSI codes (e.g. the policy toggle hint), so measure
	// visible width separately from the raw string used for padding.
	visibleTitle := " " + title + " "
	visibleWidth := lipgloss.Width(visibleTitle)
	lineWidth := width - visibleWidth - 4 // 4 for leading "  ──"
	if lineWidth < 0 {
		lineWidth = 0
	}
	line := "  ──" + visibleTitle + strings.Repeat("─", lineWidth)
	return "\n" + styleSectionHeader.Render(line)
}
