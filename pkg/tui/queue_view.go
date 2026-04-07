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

const (
	barWidth     = 6 // number of block chars in utilization bar
	colNameWidth = 20
	colCohort    = 10
	colPend      = 5
	colAdm       = 5
	colBorr      = 6
	colResource  = 17 // "████░░ 1234/5678" → bar(6) + space(1) + "NNNN/MMMM" up to 9 + space(1)
)

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
	row := m.t.SelectedRow()
	if len(row) == 0 {
		return ""
	}
	// First column is the queue name (may have cursor prefix stripped by table).
	// The table stores the raw string we put in, so strip any leading whitespace.
	return strings.TrimSpace(row[0])
}

// refresh rebuilds the table from scratch on each update. Recreating via
// table.New() ensures WithHeight() sees the real column headers when computing
// the internal viewport offset — calling SetHeight() on an existing model
// before SetColumns() produces an incorrect header-height subtraction.
func (m *queueViewModel) refresh(snap watcher.Snapshot, width, height int) {
	m.resources = collectResources(snap)
	// visibleResources is the subset that actually fits in the terminal.
	// Both columns and rows must use the same trimmed list so row cell count
	// matches column count (renderRow panics on mismatch).
	visibleResources := visibleResourceCols(m.resources, width)
	cols := buildQueueColumns(visibleResources)
	rows, names := buildQueueRows(snap, visibleResources)

	prevName := m.selectedQueueName()

	m.t = table.New(
		table.WithColumns(cols),
		table.WithRows(rows),
		table.WithHeight(height),
		table.WithWidth(width),
		table.WithFocused(true),
		table.WithStyles(defaultTableStyles()),
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

// visibleResourceCols returns the subset of resources that fit within termWidth.
// Call this once per refresh and pass the result to both buildQueueColumns and
// buildQueueRows so row cell count always equals column count.
func visibleResourceCols(resources []corev1.ResourceName, termWidth int) []corev1.ResourceName {
	fixed := colNameWidth + colCohort + colPend + colAdm + colBorr + 5 // 5 separators
	available := termWidth - fixed
	maxRes := available / colResource
	if maxRes < 0 {
		maxRes = 0
	}
	if maxRes > len(resources) {
		maxRes = len(resources)
	}
	return resources[:maxRes]
}

// buildQueueColumns returns column definitions for the given (already-trimmed) resources.
func buildQueueColumns(resources []corev1.ResourceName) []table.Column {
	cols := []table.Column{
		{Title: "NAME", Width: colNameWidth},
		{Title: "COHORT", Width: colCohort},
		{Title: "PEND", Width: colPend},
		{Title: "ADM", Width: colAdm},
	}
	for _, rName := range resources {
		cols = append(cols, table.Column{Title: strings.ToUpper(shortResourceName(rName)), Width: colResource})
	}
	cols = append(cols, table.Column{Title: "BORR", Width: colBorr})
	return cols
}

// buildQueueRows builds one row per ClusterQueue, sorted by name.
func buildQueueRows(snap watcher.Snapshot, resources []corev1.ResourceName) ([]table.Row, []string) {
	// Sort queue names for stable display.
	names := make([]string, 0, len(snap.Queues))
	for name := range snap.Queues {
		names = append(names, name)
	}
	sort.Strings(names)

	rows := make([]table.Row, 0, len(names))
	for _, name := range names {
		q := snap.Queues[name]
		row := buildQueueRow(q, resources)
		rows = append(rows, row)
	}
	return rows, names
}

func buildQueueRow(q watcher.QueueSnapshot, resources []corev1.ResourceName) table.Row {
	row := table.Row{
		truncate(q.Name, colNameWidth),
		truncate(q.Cohort, colCohort),
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

	// Borrow: total borrowed across all flavors for the primary resource.
	borrowed := primaryBorrowed(q, resources)
	if borrowed > 0 {
		row = append(row, fmt.Sprintf("%d", borrowed))
	} else {
		row = append(row, "")
	}

	return row
}

// renderResourceCell returns "████░░ 32/40" with the bar colored by utilization ratio.
// bubbles/table v2 uses ansi.Truncate (ANSI-aware), making per-cell lipgloss styling safe.
func renderResourceCell(used, nominal int64) string {
	bar := renderBar(used, nominal, barWidth)
	return utilizationStyle(used, nominal).Render(bar) + fmt.Sprintf(" %d/%d", used, nominal)
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
