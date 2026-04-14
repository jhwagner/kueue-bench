package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kueuev1beta2 "sigs.k8s.io/kueue/apis/kueue/v1beta2"

	"github.com/jhwagner/kueue-bench/pkg/watcher"
)

// workloadDetailModel is the full-screen Workload drill-down view.
// It renders into a scrollable viewport; parent (tui.go) draws the top/hint bars.
type workloadDetailModel struct {
	workloadKey  string // "namespace/name"
	isManagement bool
	vp           viewport.Model
	width        int
	height       int // available lines between topBar and hintBar
	lastSnap     watcher.Snapshot
}

func newWorkloadDetail(workloadKey string, isManagement bool, snap watcher.Snapshot, width, height int) workloadDetailModel {
	m := workloadDetailModel{
		workloadKey:  workloadKey,
		isManagement: isManagement,
		width:        width,
		height:       height,
		lastSnap:     snap,
	}
	m.vp = viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))
	m.vp.SetContent(m.buildContent(snap))
	return m
}

func (m workloadDetailModel) Init() tea.Cmd { return nil }

func (m workloadDetailModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m workloadDetailModel) View() tea.View {
	return tea.NewView(m.vp.View())
}

// buildContent renders the full detail content string for the viewport.
func (m workloadDetailModel) buildContent(snap watcher.Snapshot) string {
	wl, ok := snap.Workloads[m.workloadKey]
	if !ok {
		return styleMuted.Render(fmt.Sprintf("  Workload %q not found", m.workloadKey))
	}

	var sb strings.Builder

	// Header (two-line, no section border)
	sb.WriteString(renderWorkloadDetailHeader(wl))
	sb.WriteString("\n\n")

	// Status timeline
	sb.WriteString(renderDetailSectionHeader("Status Timeline", m.width))
	sb.WriteString("\n")
	sb.WriteString(renderWorkloadTimeline(wl))
	sb.WriteString("\n\n")

	// Resources
	sb.WriteString(renderDetailSectionHeader("Resources", m.width))
	sb.WriteString("\n")
	sb.WriteString(renderWorkloadDetailResources(wl))
	sb.WriteString("\n\n")

	// MultiKueue (management cluster only)
	if m.isManagement {
		sb.WriteString(renderDetailSectionHeader("MultiKueue", m.width))
		sb.WriteString("\n")
		sb.WriteString(renderWorkloadMultiKueue(wl))
		sb.WriteString("\n\n")
	}

	// Pods
	sb.WriteString(renderDetailSectionHeader("Pods", m.width))
	sb.WriteString("\n")
	sb.WriteString(renderWorkloadPods(wl, snap.Pods))

	return sb.String()
}

// --- Section renderers -------------------------------------------------------

func renderWorkloadDetailHeader(wl watcher.WorkloadSnapshot) string {
	// Line 1: name  ns: <ns>  owner: Kind/name  ● Status  age: Xm
	namePart := lipgloss.NewStyle().Foreground(colorBright).Bold(true).Render("  " + wl.Name)

	line1Parts := []string{
		styleMuted.Render("ns:") + " " + wl.Namespace,
	}
	if wl.OwnerKind != "" {
		owner := wl.OwnerKind
		if wl.OwnerName != "" {
			owner += "/" + wl.OwnerName
		}
		line1Parts = append(line1Parts, styleMuted.Render("owner:")+
			lipgloss.NewStyle().Foreground(colorNormal).Render(" "+owner))
	}
	line1Parts = append(line1Parts, renderWorkloadStatus(wl.Status))
	line1Parts = append(line1Parts, lipgloss.NewStyle().Foreground(colorSubtle).Render("age: "+fmtAge(wl.CreatedAt)))
	line1 := namePart + "   " + strings.Join(line1Parts, "   ")

	// Line 2: queue: lq → cq  priority: N (class)  requeues: N
	var line2Parts []string

	queueStr := wl.Queue
	if wl.ClusterQueue != "" {
		queueStr += " → " + wl.ClusterQueue
	}
	line2Parts = append(line2Parts, styleMuted.Render("queue:")+
		lipgloss.NewStyle().Foreground(colorNormal).Render(" "+queueStr))

	if wl.Priority != 0 || wl.PriorityClass != "" {
		pStr := fmt.Sprintf("%d", wl.Priority)
		if wl.PriorityClass != "" {
			pStr += " (" + wl.PriorityClass + ")"
		}
		line2Parts = append(line2Parts, styleMuted.Render("priority:")+
			lipgloss.NewStyle().Foreground(colorNormal).Render(" "+pStr))
	}

	if wl.RequeueCount > 0 || wl.Status == watcher.WorkloadStatusEvicted {
		line2Parts = append(line2Parts, styleMuted.Render("requeues:")+
			lipgloss.NewStyle().Foreground(colorNormal).Render(fmt.Sprintf(" %d", wl.RequeueCount)))
	}

	line2 := "  " + strings.Join(line2Parts, "   ")

	return line1 + "\n" + line2
}

// conditionPrecedence defines sort order when two conditions share a timestamp.
var conditionPrecedence = map[string]int{
	kueuev1beta2.WorkloadQuotaReserved: 0,
	kueuev1beta2.WorkloadAdmitted:      1,
	kueuev1beta2.WorkloadPodsReady:     2,
	kueuev1beta2.WorkloadEvicted:       3,
	kueuev1beta2.WorkloadFinished:      4,
}

func condPrecedence(condType string) int {
	if v, ok := conditionPrecedence[condType]; ok {
		return v
	}
	return 99
}

// renderWorkloadTimeline renders all conditions as a fixed-width table with
// Created first, then conditions sorted by LastTransitionTime.
func renderWorkloadTimeline(wl watcher.WorkloadSnapshot) string {
	// Column widths
	const (
		wCond  = 18
		wStat  = 7
		wTS    = 21
		wDelta = 8
	)

	type row struct {
		cond      string
		status    string // "True" / "False" / "Unknown" / "–"
		ts        time.Time
		message   string
		isCreated bool
	}

	rows := []row{
		{cond: "Created", status: "–", ts: wl.CreatedAt, isCreated: true},
	}

	sorted := make([]metav1.Condition, len(wl.Conditions))
	copy(sorted, wl.Conditions)
	sort.Slice(sorted, func(i, j int) bool {
		ti := sorted[i].LastTransitionTime.Time
		tj := sorted[j].LastTransitionTime.Time
		if ti.Equal(tj) {
			return condPrecedence(sorted[i].Type) < condPrecedence(sorted[j].Type)
		}
		return ti.Before(tj)
	})

	for _, c := range sorted {
		rows = append(rows, row{
			cond:    c.Type,
			status:  string(c.Status),
			ts:      c.LastTransitionTime.Time,
			message: c.Message,
		})
	}

	styleHdr := lipgloss.NewStyle().Foreground(colorSubtle).Bold(true)

	hdr := fmt.Sprintf("  %-*s  %-*s  %-*s  %-*s  %s",
		wCond, "CONDITION",
		wStat, "STATUS",
		wTS, "TIMESTAMP",
		wDelta, "DELTA",
		"MESSAGE",
	)
	sep := styleMuted.Render(strings.Repeat("─", len(hdr)))

	var sb strings.Builder
	sb.WriteString(styleHdr.Render(hdr))
	sb.WriteString("\n")
	sb.WriteString(sep)
	sb.WriteString("\n")

	for i, r := range rows {
		// DELTA
		var deltaStr string
		if i == 0 {
			deltaStr = "–"
		} else {
			d := r.ts.Sub(rows[i-1].ts)
			if d < 0 {
				d = 0
			}
			deltaStr = "+" + fmtDuration(d)
		}

		// STATUS style
		var statusRendered string
		switch r.status {
		case "True":
			statusRendered = lipgloss.NewStyle().Foreground(colorGreen).Render(fmt.Sprintf("%-*s", wStat, r.status))
		case "False":
			statusRendered = lipgloss.NewStyle().Foreground(colorRed).Render(fmt.Sprintf("%-*s", wStat, r.status))
		case "Unknown":
			statusRendered = styleMuted.Render(fmt.Sprintf("%-*s", wStat, r.status))
		default: // "–" for Created row
			statusRendered = styleMuted.Render(fmt.Sprintf("%-*s", wStat, r.status))
		}

		tsStr := r.ts.Format("2006-01-02 15:04:05")

		// MESSAGE style
		var msgRendered string
		if r.message != "" {
			msgStyle := lipgloss.NewStyle().Foreground(colorSubtle)
			if r.cond == kueuev1beta2.WorkloadEvicted {
				msgStyle = lipgloss.NewStyle().Foreground(colorRed)
			} else if r.cond == kueuev1beta2.WorkloadFinished && r.status == "False" {
				msgStyle = lipgloss.NewStyle().Foreground(colorRed)
			}
			msgRendered = msgStyle.Render(r.message)
		}

		condStr := truncate(r.cond, wCond)
		row := fmt.Sprintf("  %-*s  %s  %-*s  %-*s  %s",
			wCond, condStr,
			statusRendered,
			wTS, tsStr,
			wDelta, styleMuted.Render(deltaStr),
			msgRendered,
		)
		sb.WriteString(row)
		sb.WriteString("\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}

// renderWorkloadDetailResources renders per-pod-set resource table.
// Each row shows per-pod requests, pod count, and per-set total.
// A workload-wide total line follows below the table.
func renderWorkloadDetailResources(wl watcher.WorkloadSnapshot) string {
	if len(wl.PodSets) == 0 {
		return styleMuted.Render("  No pod sets.")
	}

	styleHdr := lipgloss.NewStyle().Foreground(colorSubtle).Bold(true)
	hdr := fmt.Sprintf("  %-20s  %6s  %-32s  %s", "POD SET", "PODS", "PER POD", "TOTAL")

	var sb strings.Builder
	sb.WriteString(styleHdr.Render(hdr))
	sb.WriteString("\n")

	for _, ps := range wl.PodSets {
		perPod := renderPodSetResources(ps.Resources)
		total := renderPodSetTotal(ps.Resources, ps.Count)
		row := fmt.Sprintf("  %-20s  %6d  %-32s  %s",
			truncate(ps.Name, 20),
			ps.Count,
			perPod,
			total,
		)
		sb.WriteString(row)
		sb.WriteString("\n")
	}

	// Workload-wide total — always shown, outside the per-set table.
	sb.WriteString("\n")
	label := styleMuted.Render("  Workload total:")
	sb.WriteString(label + "  " + renderPodSetResources(wl.Resources))

	return strings.TrimRight(sb.String(), "\n")
}

// renderPodSetTotal multiplies per-pod resources by count and renders as a string.
func renderPodSetTotal(resources map[corev1.ResourceName]resource.Quantity, count int32) string {
	if len(resources) == 0 {
		return "–"
	}
	totals := make(map[corev1.ResourceName]resource.Quantity, len(resources))
	for rName, q := range resources {
		t := q.DeepCopy()
		t.Mul(int64(count))
		totals[rName] = t
	}
	return renderPodSetResources(totals)
}

// renderPodSetResources renders a resource map as "key: value" pairs, e.g. "cpu: 4  mem: 8Gi  gpu: 2".
// Uses resource.Quantity.String() for the value, which preserves units (Gi, Ti, m, etc.).
func renderPodSetResources(resources map[corev1.ResourceName]resource.Quantity) string {
	if len(resources) == 0 {
		return "–"
	}
	names := make([]corev1.ResourceName, 0, len(resources))
	for rName := range resources {
		names = append(names, rName)
	}
	sortResourceNames(names)
	parts := make([]string, 0, len(names))
	for _, rName := range names {
		q := resources[rName]
		label := strings.ToLower(shortResourceName(rName))
		parts = append(parts, fmt.Sprintf("%s: %s", label, q.String()))
	}
	return strings.Join(parts, "  ")
}

func renderWorkloadMultiKueue(wl watcher.WorkloadSnapshot) string {
	dispatched := wl.DispatchedTo
	if dispatched == "" {
		dispatched = "–"
	}
	row := fmt.Sprintf("  %-20s  %s", "Dispatched to:", lipgloss.NewStyle().Foreground(colorNormal).Render(dispatched))
	return row
}

func renderWorkloadPods(wl watcher.WorkloadSnapshot, pods map[string]watcher.PodSnapshot) string {
	// Unknown owner kind — no label selector available.
	if watcher.PodLabelSelector(wl.OwnerKind, wl.OwnerName) == "" {
		if wl.OwnerKind == "" {
			return styleMuted.Render("  pod tracking not available (no owner)")
		}
		return styleMuted.Render(fmt.Sprintf("  pod tracking not available for %s workloads", wl.OwnerKind))
	}

	// Empty state depends on workload admission status.
	if len(pods) == 0 {
		switch wl.Status {
		case watcher.WorkloadStatusAdmitted:
			return styleMuted.Render("  waiting for pods...")
		case watcher.WorkloadStatusFinished:
			return styleMuted.Render("  no pods (completed pods may have been cleaned up)")
		default:
			return styleMuted.Render("  no pods (workload not yet admitted)")
		}
	}

	// Phase counts.
	var running, pending, failed, succeeded int
	for _, p := range pods {
		switch p.Phase {
		case corev1.PodRunning:
			running++
		case corev1.PodPending:
			pending++
		case corev1.PodFailed:
			failed++
		case corev1.PodSucceeded:
			succeeded++
		}
	}

	styleRunning := lipgloss.NewStyle().Foreground(colorGreen)
	stylePending := lipgloss.NewStyle().Foreground(colorYellow)
	styleFailed := lipgloss.NewStyle().Foreground(colorRed)

	summary := fmt.Sprintf("  %s  %s  %s  %s",
		styleRunning.Render(fmt.Sprintf("%d Running", running)),
		stylePending.Render(fmt.Sprintf("%d Pending", pending)),
		styleFailed.Render(fmt.Sprintf("%d Failed", failed)),
		styleMuted.Render(fmt.Sprintf("%d Succeeded", succeeded)),
	)

	// Collect problem pods.
	now := time.Now()
	var problems []watcher.PodSnapshot
	for _, p := range pods {
		if isProblemPod(p, now) {
			problems = append(problems, p)
		}
	}

	if len(problems) == 0 {
		return summary
	}

	// Sort: Failed > Unknown > Pending > Running, then name.
	phaseSeverity := func(phase corev1.PodPhase) int {
		switch phase {
		case corev1.PodFailed:
			return 0
		case corev1.PodUnknown:
			return 1
		case corev1.PodPending:
			return 2
		case corev1.PodRunning:
			return 3
		}
		return 4
	}
	sort.Slice(problems, func(i, j int) bool {
		si, sj := phaseSeverity(problems[i].Phase), phaseSeverity(problems[j].Phase)
		if si != sj {
			return si < sj
		}
		return problems[i].Name < problems[j].Name
	})

	styleHdr := lipgloss.NewStyle().Foreground(colorSubtle).Bold(true)
	hdr := fmt.Sprintf("  %-40s  %-10s  %-8s  %s", "NAME", "PHASE", "AGE", "MESSAGE")

	var sb strings.Builder
	sb.WriteString(summary)
	sb.WriteString("\n\n")
	sb.WriteString(styleMuted.Render("  PROBLEM PODS"))
	sb.WriteString("\n")
	sb.WriteString(styleHdr.Render(hdr))
	sb.WriteString("\n")
	sb.WriteString(styleMuted.Render(strings.Repeat("─", len(hdr))))
	sb.WriteString("\n")

	for _, p := range problems {
		age := fmtAge(p.CreatedAt)
		phaseStr := string(p.Phase)
		var phaseRendered string
		switch p.Phase {
		case corev1.PodFailed, corev1.PodUnknown:
			phaseRendered = lipgloss.NewStyle().Foreground(colorRed).Render(fmt.Sprintf("%-10s", phaseStr))
		case corev1.PodPending:
			phaseRendered = lipgloss.NewStyle().Foreground(colorYellow).Render(fmt.Sprintf("%-10s", phaseStr))
		default:
			phaseRendered = fmt.Sprintf("%-10s", phaseStr)
		}
		msgRendered := styleMuted.Render(truncate(p.Message, 60))
		row := fmt.Sprintf("  %-40s  %s  %-8s  %s",
			truncate(p.Name, 40),
			phaseRendered,
			age,
			msgRendered,
		)
		sb.WriteString(row)
		sb.WriteString("\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}

func isProblemPod(p watcher.PodSnapshot, now time.Time) bool {
	age := now.Sub(p.CreatedAt)
	switch p.Phase {
	case corev1.PodFailed, corev1.PodUnknown:
		return true
	case corev1.PodPending:
		return age > 30*time.Second
	case corev1.PodRunning:
		return !p.Ready && age > 30*time.Second
	}
	return false
}

// --- Helpers -----------------------------------------------------------------

// fmtDuration formats a duration as a compact human-readable string.
func fmtDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh%dm", h, m)
}
