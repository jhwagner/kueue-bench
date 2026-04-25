package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/jhwagner/kueue-bench/pkg/watcher"
)

// workloadFilter is the active status filter for the workload table.
type workloadFilter int

const (
	filterAll workloadFilter = iota
	filterPending
	filterQuotaReserved
	filterAdmitted
	filterFinished
	filterEvicted
)

var filterOrder = []workloadFilter{
	filterAll,
	filterPending,
	filterQuotaReserved,
	filterAdmitted,
	filterFinished,
	filterEvicted,
}

func (f workloadFilter) String() string {
	switch f {
	case filterPending:
		return "Pending"
	case filterQuotaReserved:
		return "QuotaReserved"
	case filterAdmitted:
		return "Admitted"
	case filterFinished:
		return "Finished"
	case filterEvicted:
		return "Evicted"
	default:
		return "All"
	}
}

// workloadViewModel renders the filterable workload overview table.
//
// The view stores its own copy of snapshot/width/height so that internal state
// changes (filter cycling, future sort/search) can call rebuild() directly
// without parent involvement.
type workloadViewModel struct {
	t            table.Model
	filter       workloadFilter
	isManagement bool
	workloadKeys []string // matches row order; used to restore cursor by key

	// stored for rebuild on internal state changes
	snapshot watcher.Snapshot
	width    int
	height   int
}

func newWorkloadView(isManagement bool) workloadViewModel {
	t := table.New(
		table.WithFocused(true),
		table.WithStyles(defaultTableStyles()),
	)
	return workloadViewModel{t: t, isManagement: isManagement}
}

func (m *workloadViewModel) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.t, cmd = m.t.Update(msg)
	return cmd
}

func (m *workloadViewModel) cycleFilter() {
	for i, f := range filterOrder {
		if f == m.filter {
			m.filter = filterOrder[(i+1)%len(filterOrder)]
			m.rebuild()
			return
		}
	}
	m.filter = filterAll
	m.rebuild()
}

func (m workloadViewModel) view() string {
	filterLabel := lipgloss.NewStyle().Foreground(colorSubtle).Render(
		fmt.Sprintf("  filter: %s", m.filter.String()),
	)
	return filterLabel + "\n" + m.t.View()
}

// selectedWorkloadKey returns the "namespace/name" key of the highlighted row, or "".
func (m workloadViewModel) selectedWorkloadKey() string {
	row := m.t.SelectedRow()
	if len(row) == 0 || m.t.Cursor() >= len(m.workloadKeys) {
		return ""
	}
	return m.workloadKeys[m.t.Cursor()]
}

// refresh stores the latest external data and rebuilds the table.
// Called by the parent on snapshotMsg and WindowSizeMsg.
func (m *workloadViewModel) refresh(snap watcher.Snapshot, width, height int) {
	m.snapshot = snap
	m.width = width
	m.height = height
	m.rebuild()
}

// rebuild reconstructs the table from stored state. Called by refresh (external
// data change) and cycleFilter (internal state change).
func (m *workloadViewModel) rebuild() {
	filtered := filterWorkloads(m.snapshot.Workloads, m.filter)
	// Sort newest first.
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})

	specs := workloadColumnSpecs(m.isManagement)
	rawRows, keys := buildWorkloadRows(filtered, m.isManagement)
	widths := ComputeWidths(specs, rawRows, m.width)
	cols := BuildColumns(specs, widths)
	rows := make([]table.Row, len(rawRows))
	for i, r := range rawRows {
		rows[i] = table.Row(r)
	}

	prevKey := m.selectedWorkloadKey()

	// Recreate via table.New so WithStyles is applied before WithHeight;
	// WithHeight calls lipgloss.Height(headersView()) to compute the header
	// height, which must already include the border set by WithStyles.
	m.t = table.New(
		table.WithColumns(cols),
		table.WithRows(rows),
		table.WithStyles(defaultTableStyles()),
		table.WithHeight(m.height-1), // -1 for the filter label line rendered above the table
		table.WithWidth(m.width),
		table.WithFocused(true),
	)
	m.workloadKeys = keys

	// Restore cursor by key.
	if prevKey != "" {
		for i, k := range keys {
			if k == prevKey {
				m.t.SetCursor(i)
				break
			}
		}
	}
}

func filterWorkloads(workloads map[string]watcher.WorkloadSnapshot, f workloadFilter) []watcher.WorkloadSnapshot {
	out := make([]watcher.WorkloadSnapshot, 0, len(workloads))
	for _, wl := range workloads {
		if f == filterAll || matchesFilter(wl.Status, f) {
			out = append(out, wl)
		}
	}
	return out
}

func matchesFilter(status watcher.WorkloadStatus, f workloadFilter) bool {
	switch f {
	case filterPending:
		return status == watcher.WorkloadStatusPending
	case filterQuotaReserved:
		return status == watcher.WorkloadStatusQuotaReserved
	case filterAdmitted:
		return status == watcher.WorkloadStatusAdmitted
	case filterFinished:
		return status == watcher.WorkloadStatusFinished
	case filterEvicted:
		return status == watcher.WorkloadStatusEvicted
	}
	return true
}

// workloadColumnSpecs declares the layout of the workload overview table.
// DISPATCHED TO is only present on management clusters and is droppable
// (Priority=10) so narrow terminals can hide it while NAME/QUEUE/RESOURCES
// flex to fill slack on wider ones.
func workloadColumnSpecs(isManagement bool) []ColumnSpec {
	specs := []ColumnSpec{
		{Title: "NAME", MinWidth: 16, Flex: 3},
		{Title: "TYPE", MinWidth: 6},
		{Title: "QUEUE", MinWidth: 10, Flex: 1},
		{Title: "STATUS", MinWidth: 14},
		{Title: "AGE", MinWidth: 4},
		{Title: "RESOURCES", MinWidth: 12, MaxWidth: 28, Flex: 1},
	}
	if isManagement {
		specs = append(specs, ColumnSpec{Title: "CLUSTER", MinWidth: 7, Priority: 10})
	}
	return specs
}

func buildWorkloadRows(workloads []watcher.WorkloadSnapshot, isManagement bool) ([][]string, []string) {
	rows := make([][]string, 0, len(workloads))
	keys := make([]string, 0, len(workloads))

	for _, wl := range workloads {
		key := wl.Namespace + "/" + wl.Name
		rows = append(rows, buildWorkloadRow(wl, isManagement))
		keys = append(keys, key)
	}
	return rows, keys
}

func buildWorkloadRow(wl watcher.WorkloadSnapshot, isManagement bool) []string {
	ownerKind := wl.OwnerKind
	if ownerKind == "" {
		ownerKind = "–"
	}

	row := []string{
		wl.Name,
		ownerKind,
		wl.Queue,
		renderWorkloadStatus(wl.Status),
		fmtAge(wl.CreatedAt),
		renderWorkloadResources(wl.Resources),
	}

	if isManagement {
		dispatched := wl.DispatchedTo
		if dispatched == "" {
			dispatched = "–"
		}
		row = append(row, dispatched)
	}

	return row
}

// renderWorkloadStatus returns a colored status string.
func renderWorkloadStatus(s watcher.WorkloadStatus) string {
	style := lipgloss.NewStyle().Foreground(colorNormal)
	switch s {
	case watcher.WorkloadStatusAdmitted:
		style = lipgloss.NewStyle().Foreground(colorGreen)
	case watcher.WorkloadStatusQuotaReserved:
		style = lipgloss.NewStyle().Foreground(colorYellow)
	case watcher.WorkloadStatusPending:
		style = lipgloss.NewStyle().Foreground(colorSubtle)
	case watcher.WorkloadStatusFinished:
		style = lipgloss.NewStyle().Foreground(colorMuted)
	case watcher.WorkloadStatusEvicted:
		style = lipgloss.NewStyle().Foreground(colorRed)
	}
	return style.Render(string(s))
}

// renderWorkloadResources renders all resources sorted by priority (GPU → CPU → Memory → other)
// as a space-joined string, truncated to fit the column.
func renderWorkloadResources(resources map[corev1.ResourceName]resource.Quantity) string {
	if len(resources) == 0 {
		return "–"
	}

	ordered := make([]corev1.ResourceName, 0, len(resources))
	for rName := range resources {
		ordered = append(ordered, rName)
	}
	sort.Slice(ordered, func(i, j int) bool {
		ri, rj := resourceRank(ordered[i]), resourceRank(ordered[j])
		if ri != rj {
			return ri < rj
		}
		return ordered[i] < ordered[j]
	})

	parts := make([]string, 0, len(ordered))
	for _, rName := range ordered {
		q := resources[rName]
		val := quantityValue(rName, q)
		parts = append(parts, fmt.Sprintf("%d %s", val, shortResourceName(rName)))
	}
	return strings.Join(parts, " ")
}

func resourceRank(rName corev1.ResourceName) int {
	s := strings.ToLower(string(rName))
	switch {
	case strings.Contains(s, "gpu"):
		return 0
	case rName == corev1.ResourceCPU:
		return 1
	case rName == corev1.ResourceMemory:
		return 2
	default:
		return 3
	}
}

// fmtAge returns a human-readable age string (e.g. "2m", "1h3m", "4d").
func fmtAge(t time.Time) string {
	if t.IsZero() {
		return "–"
	}
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh%dm", h, m)
	default:
		days := int(d.Hours()) / 24
		return fmt.Sprintf("%dd", days)
	}
}
