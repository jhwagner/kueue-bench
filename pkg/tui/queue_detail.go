package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/table"
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
		m.vp.SetContent(m.buildContent(m.lastSnap))
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
	sb.WriteString(renderQueueDetailResources(q, m.width))
	sb.WriteString("\n")

	// Workloads
	activeWls := collectQueueWorkloads(snap.Workloads, q.Name)
	sb.WriteString(renderDetailSectionHeader(fmt.Sprintf("Workloads (%d active)", len(activeWls)), m.width))
	sb.WriteString("\n")
	sb.WriteString(renderQueueDetailWorkloads(activeWls, m.width))
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

// renderQueueDetailResources renders one flavor block per flavor. Each flavor
// gets a name header followed by a bubbles/table showing its resource rows.
// Column widths are computed across all flavors so columns align consistently.
func renderQueueDetailResources(q watcher.QueueSnapshot, width int) string {
	if len(q.Flavors) == 0 {
		return styleMuted.Render("  No resource flavors configured.")
	}

	specs := []ColumnSpec{
		{Title: "RESOURCE", MinWidth: 12, Flex: 1},
		// bar(6) + " " + numbers padded to 13 = 20 visual chars; pinned.
		{Title: "USED", MinWidth: 20, MaxWidth: 20},
		{Title: "BORROWED", MinWidth: 5, Priority: 20},
		{Title: "BORROW-LMT", MinWidth: 10, Priority: 10},
		{Title: "LEND-LMT", MinWidth: 8, Priority: 10},
	}

	type flavorData struct {
		name string
		rows []table.Row
	}

	// Collect raw (unstyled) rows across all flavors to measure natural widths.
	var allRaw [][]string
	flavors := make([]flavorData, 0, len(q.Flavors))

	for _, fl := range q.Flavors {
		resources := sortedFlavorResources(fl)
		rows := make([]table.Row, 0, len(resources))

		for _, rName := range resources {
			rs := fl.Resources[rName]
			used := quantityValue(rName, rs.Used)
			nominal := quantityValue(rName, rs.Nominal)

			allRaw = append(allRaw, []string{
				string(rName),
				renderBar(used, nominal, barWidth) + " " + fmt.Sprintf("%-13s", fmt.Sprintf("%d/%d", used, nominal)),
				fmtQty(rName, rs.Borrowed),
				fmtQtyPtr(rName, rs.BorrowingLimit),
				fmtQtyPtr(rName, rs.LendingLimit),
			})

			utilStyle := utilizationStyle(used, nominal)
			rows = append(rows, table.Row{
				string(rName),
				utilStyle.Render(renderBar(used, nominal, barWidth)) + " " +
					utilStyle.Render(fmt.Sprintf("%-13s", fmt.Sprintf("%d/%d", used, nominal))),
				fmtQty(rName, rs.Borrowed),
				fmtQtyPtr(rName, rs.BorrowingLimit),
				fmtQtyPtr(rName, rs.LendingLimit),
			})
		}
		flavors = append(flavors, flavorData{fl.Name, rows})
	}

	widths := ComputeWidths(specs, allRaw, width)
	cols := BuildColumns(specs, widths)

	var sb strings.Builder
	for i, fd := range flavors {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(colorBright).Bold(true).Render("  " + fd.name))
		sb.WriteString("\n")
		t := table.New(
			table.WithColumns(cols),
			table.WithRows(fd.rows),
			table.WithStyles(defaultTableStyles()),
			table.WithWidth(width),
			table.WithHeight(len(fd.rows)+2), // +2: WithHeight counts header text + bottom border
		)
		sb.WriteString(t.View())
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

func renderQueueDetailWorkloads(wls []watcher.WorkloadSnapshot, width int) string {
	if len(wls) == 0 {
		return styleMuted.Render("  No active workloads.")
	}

	specs := []ColumnSpec{
		{Title: "NAME", MinWidth: 14, Flex: 2},
		{Title: "STATUS", MinWidth: 9},
		{Title: "AGE", MinWidth: 3, Priority: 20},
		{Title: "RESOURCES", MinWidth: 10, Flex: 1},
	}

	rawRows := make([][]string, 0, len(wls))
	dataRows := make([]table.Row, 0, len(wls))
	for _, wl := range wls {
		rawRows = append(rawRows, []string{
			wl.Name,
			string(wl.Status),
			fmtAge(wl.CreatedAt),
			renderWorkloadResources(wl.Resources),
		})
		dataRows = append(dataRows, table.Row{
			wl.Name,
			renderWorkloadStatus(wl.Status),
			fmtAge(wl.CreatedAt),
			renderWorkloadResources(wl.Resources),
		})
	}

	widths := ComputeWidths(specs, rawRows, width)
	cols := BuildColumns(specs, widths)
	t := table.New(
		table.WithColumns(cols),
		table.WithRows(dataRows),
		table.WithStyles(defaultTableStyles()),
		table.WithWidth(width),
		table.WithHeight(len(dataRows)+2), // +2: WithHeight counts header text + bottom border
	)
	return t.View()
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
	lineWidth := width - visibleWidth - 2 // 2 for leading "──"
	if lineWidth < 0 {
		lineWidth = 0
	}
	line := "──" + visibleTitle + strings.Repeat("─", lineWidth)
	return "\n" + styleSectionHeader.Render(line)
}
