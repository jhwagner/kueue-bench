package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	corev1 "k8s.io/api/core/v1"

	"github.com/jhwagner/kueue-bench/pkg/watcher"
)

const barWidth = 6 // number of block chars in utilization bar

// queueViewModel renders the ClusterQueue overview table.
type queueViewModel struct {
	t          table.Model
	queueNames []string // sorted, matches row order
	resources  []corev1.ResourceName
}

func newQueueView() queueViewModel {
	t := table.New(
		table.WithFocused(true),
		table.WithStyles(defaultTableStyles()),
	)
	return queueViewModel{t: t}
}

func (m *queueViewModel) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.t, cmd = m.t.Update(msg)
	return cmd
}

func (m queueViewModel) view() string {
	return m.t.View()
}

// selectedQueueName returns the name of the currently highlighted ClusterQueue,
// or "" if the table is empty.
func (m queueViewModel) selectedQueueName() string {
	if len(m.queueNames) == 0 {
		return ""
	}
	cursor := m.t.Cursor()
	if cursor < 0 || cursor >= len(m.queueNames) {
		return ""
	}
	return m.queueNames[cursor]
}

// refresh rebuilds the table from scratch on each update. WithStyles must be
// passed before WithHeight: WithHeight calls lipgloss.Height(headersView()) to
// size the inner viewport, so the bordered header style must already be applied
// when that measurement runs — otherwise the table renders one line too tall.
func (m *queueViewModel) refresh(snap watcher.Snapshot, width, height int) {
	m.resources = collectResources(snap)
	specs := queueColumnSpecs(m.resources)
	rawRows, names := buildQueueRows(snap, m.resources)
	widths := ComputeWidths(specs, rawRows, width)
	cols := BuildColumns(specs, widths)
	rows := make([]table.Row, len(rawRows))
	for i, r := range rawRows {
		rows[i] = table.Row(r)
	}

	prevName := m.selectedQueueName()

	m.t = table.New(
		table.WithColumns(cols),
		table.WithRows(rows),
		table.WithStyles(defaultTableStyles()),
		table.WithHeight(height),
		table.WithWidth(width),
		table.WithFocused(true),
	)
	m.queueNames = names

	// Restore cursor position by name.
	if prevName != "" {
		for i, n := range names {
			if n == prevName {
				m.t.SetCursor(i)
				break
			}
		}
	}
}

// collectResources returns a stable ordered slice of resource names present
// across all queues in the snapshot (GPU → CPU → Memory → other).
func collectResources(snap watcher.Snapshot) []corev1.ResourceName {
	set := map[corev1.ResourceName]struct{}{}
	for _, q := range snap.Queues {
		for _, fl := range q.Flavors {
			for rName := range fl.Resources {
				set[rName] = struct{}{}
			}
		}
	}
	return sortResources(set)
}

func sortResources(set map[corev1.ResourceName]struct{}) []corev1.ResourceName {
	var gpu, cpu, mem, other []corev1.ResourceName
	for rName := range set {
		s := string(rName)
		switch {
		case strings.Contains(strings.ToLower(s), "gpu"):
			gpu = append(gpu, rName)
		case rName == corev1.ResourceCPU:
			cpu = append(cpu, rName)
		case rName == corev1.ResourceMemory:
			mem = append(mem, rName)
		default:
			other = append(other, rName)
		}
	}
	sortNames := func(ns []corev1.ResourceName) {
		sort.Slice(ns, func(i, j int) bool { return ns[i] < ns[j] })
	}
	sortNames(gpu)
	sortNames(other)
	return append(append(append(gpu, cpu...), mem...), other...)
}

// queueColumnSpecs declares the ClusterQueue overview table layout.
//
// Resource columns are dynamic: one per resource seen across all queues, with
// Priority descending by position so the least-important resource is dropped
// first on narrow terminals. The first resource (highest-priority, typically
// GPU or CPU) is required. BORR is droppable early since it's supplemental.
func queueColumnSpecs(resources []corev1.ResourceName) []ColumnSpec {
	// Resource cell format: "████░░ 1234/5678" — bar(6) + space + numbers.
	// MinWidth=12 leaves room for the bar + short numbers; MaxWidth=17
	// covers the widest typical case.
	const (
		resMin = 12
		resMax = 17
	)

	specs := []ColumnSpec{
		{Title: "NAME", MinWidth: 14, Flex: 2}, // includes 2-char indicator prefix
		{Title: "COHORT", MinWidth: 6, Flex: 1, Priority: 20},
		{Title: "PEND", MinWidth: 4},
		{Title: "ADM", MinWidth: 4},
	}
	for i, rName := range resources {
		spec := ColumnSpec{
			Title:    strings.ToUpper(shortResourceName(rName)),
			MinWidth: resMin,
			MaxWidth: resMax,
		}
		if i > 0 {
			// First resource required; subsequent ones droppable with
			// priority descending by position (later = lower = dropped first).
			spec.Priority = len(resources) - i
		}
		specs = append(specs, spec)
	}
	specs = append(specs, ColumnSpec{Title: "BORR", MinWidth: 4, Priority: 10})
	return specs
}

// buildQueueRows builds one row per ClusterQueue, sorted by name.
func buildQueueRows(snap watcher.Snapshot, resources []corev1.ResourceName) ([][]string, []string) {
	names := make([]string, 0, len(snap.Queues))
	for name := range snap.Queues {
		names = append(names, name)
	}
	sort.Strings(names)

	rows := make([][]string, 0, len(names))
	for _, name := range names {
		q := snap.Queues[name]
		rows = append(rows, buildQueueRow(q, resources))
	}
	return rows, names
}

func buildQueueRow(q watcher.QueueSnapshot, resources []corev1.ResourceName) []string {
	// Name cell: 2-char indicator prefix + name. "  " when no alert,
	// "[glyph] " when a flavor is near/at capacity. The glyph carries ANSI
	// styling but measures as 1 cell via lipgloss.Width.
	indicator := flavorIndicatorGlyph(worstFlavorRatio(q))
	row := []string{
		indicator + " " + q.Name,
		q.Cohort,
		fmt.Sprintf("%d", q.Pending),
		fmt.Sprintf("%d", q.Admitted),
	}

	for _, rName := range resources {
		var used, nominal int64
		for _, fl := range q.Flavors {
			if rs, ok := fl.Resources[rName]; ok {
				used += quantityValue(rName, rs.Used)
				nominal += quantityValue(rName, rs.Nominal)
			}
		}
		row = append(row, renderResourceCell(used, nominal))
	}

	borrowed := primaryBorrowed(q, resources)
	if borrowed > 0 {
		row = append(row, fmt.Sprintf("%d", borrowed))
	} else {
		row = append(row, "")
	}

	return row
}

// worstFlavorRatio returns the highest used/nominal ratio across all flavors and resources
// in a queue. Used to decide whether to show the per-flavor capacity indicator.
func worstFlavorRatio(q watcher.QueueSnapshot) float64 {
	worst := 0.0
	for _, fl := range q.Flavors {
		for rName, rs := range fl.Resources {
			nominal := quantityValue(rName, rs.Nominal)
			if nominal <= 0 {
				continue
			}
			ratio := float64(quantityValue(rName, rs.Used)) / float64(nominal)
			if ratio > worst {
				worst = ratio
			}
		}
	}
	return worst
}

// flavorIndicatorGlyph returns a colored 1-char glyph when any flavor is near/at capacity
// (▲ ≥70%, ● ≥90%), or a plain space when utilization is below threshold.
func flavorIndicatorGlyph(ratio float64) string {
	switch {
	case ratio >= 0.9:
		return lipgloss.NewStyle().Foreground(colorRed).Render("●")
	case ratio >= 0.7:
		return lipgloss.NewStyle().Foreground(colorYellow).Render("▲")
	default:
		return " "
	}
}

// flavorIndicatorLegend returns the hint text explaining the indicator glyphs.
func flavorIndicatorLegend() string {
	return "▲/● = flavor near/at capacity"
}

// renderQueueLegendLine renders a right-aligned legend footer for the queue
// pane. Returns an empty string (still occupies one line in the layout) when
// legend is "".
func renderQueueLegendLine(width int, legend string) string {
	if legend == "" {
		return ""
	}
	legendWidth := lipgloss.Width(legend)
	pad := width - legendWidth - 1
	if pad < 0 {
		pad = 0
	}
	return lipgloss.NewStyle().Foreground(colorMuted).Render(strings.Repeat(" ", pad) + legend)
}

// anyFlavorAtCapacity returns true if any ClusterQueue in the snapshot has at least
// one flavor at or near capacity (worst ratio ≥ 0.7).
func anyFlavorAtCapacity(snap watcher.Snapshot) bool {
	for _, q := range snap.Queues {
		if worstFlavorRatio(q) >= 0.7 {
			return true
		}
	}
	return false
}

// renderResourceCell returns "████░░ 32/40" with both the bar and the numbers colored by
// utilization ratio. Coloring the numbers ensures the utilization level is visible even
// when the bar is all-empty (low utilization), where block character color alone is subtle.
// bubbles/table v2 uses ansi.Truncate (ANSI-aware), making per-cell lipgloss styling safe.
func renderResourceCell(used, nominal int64) string {
	style := utilizationStyle(used, nominal)
	bar := renderBar(used, nominal, barWidth)
	return style.Render(bar) + " " + style.Render(fmt.Sprintf("%d/%d", used, nominal))
}

// renderBar returns a string of block characters representing utilization.
func renderBar(used, nominal int64, width int) string {
	if nominal <= 0 || width <= 0 {
		return strings.Repeat("░", width)
	}
	ratio := float64(used) / float64(nominal)
	filled := int(ratio * float64(width))
	if filled > width {
		filled = width
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// utilizationStyle returns a style colored by the used/nominal ratio.
func utilizationStyle(used, nominal int64) lipgloss.Style {
	if nominal <= 0 {
		return lipgloss.NewStyle().Foreground(colorMuted)
	}
	ratio := float64(used) / float64(nominal)
	switch {
	case ratio >= 0.9:
		return lipgloss.NewStyle().Foreground(colorRed)
	case ratio >= 0.7:
		return lipgloss.NewStyle().Foreground(colorYellow)
	default:
		return lipgloss.NewStyle().Foreground(colorGreen)
	}
}

// primaryBorrowed returns the borrowed amount for the first (most significant) resource.
func primaryBorrowed(q watcher.QueueSnapshot, resources []corev1.ResourceName) int64 {
	if len(resources) == 0 {
		return 0
	}
	rName := resources[0]
	var total int64
	for _, fl := range q.Flavors {
		if rs, ok := fl.Resources[rName]; ok {
			total += quantityValue(rName, rs.Borrowed)
		}
	}
	return total
}

// sortResourceNames sorts a slice of resource names by priority order (cpu, memory, gpu, then alpha).
func sortResourceNames(names []corev1.ResourceName) {
	sort.Slice(names, func(i, j int) bool {
		ri, rj := resourceRank(names[i]), resourceRank(names[j])
		if ri != rj {
			return ri < rj
		}
		return names[i] < names[j]
	})
}

// truncate shortens s to max n runes, appending "…" if truncated.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return string(runes[:n-1]) + "…"
}
