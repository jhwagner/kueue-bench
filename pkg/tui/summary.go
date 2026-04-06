package tui

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/jhwagner/kueue-bench/pkg/watcher"
)

// renderSummaryBar renders a single-line aggregate stats strip:
// "Workloads: 12 pending  23 admitted  87 finished │ GPU: 72/100 │ CPU: 40/80"
func renderSummaryBar(width int, snap watcher.Snapshot) string {
	var pending, admitted, finished int32
	for _, q := range snap.Queues {
		pending += q.Pending
		admitted += q.Admitted
	}
	// Count finished from workloads directly (queues don't track finished).
	for _, wl := range snap.Workloads {
		if wl.Status == watcher.WorkloadStatusFinished {
			finished++
		}
	}

	parts := []string{
		fmt.Sprintf("Workloads: %d pending  %d admitted  %d finished",
			pending, admitted, finished),
	}

	// Aggregate per-resource utilization across all queues.
	totals := aggregateFleetResources(snap)
	for _, rName := range resourceDisplayOrder(totals) {
		used := totals[rName].used
		nominal := totals[rName].nominal
		if nominal.IsZero() {
			continue
		}
		usedVal := quantityValue(rName, used)
		nomVal := quantityValue(rName, nominal)
		parts = append(parts, fmt.Sprintf("%s: %d/%d", shortResourceName(rName), usedVal, nomVal))
	}

	content := styleSummaryBar.Render(strings.Join(parts, "  │  "))
	visible := len(stripANSI(content)) // rough width
	_ = visible
	_ = width
	return content
}

type resourceTotals struct {
	used    resource.Quantity
	nominal resource.Quantity
}

func aggregateFleetResources(snap watcher.Snapshot) map[corev1.ResourceName]resourceTotals {
	m := make(map[corev1.ResourceName]resourceTotals)
	for _, q := range snap.Queues {
		for _, fl := range q.Flavors {
			for rName, rs := range fl.Resources {
				t := m[rName]
				t.used.Add(rs.Used)
				t.nominal.Add(rs.Nominal)
				m[rName] = t
			}
		}
	}
	return m
}

// resourceDisplayOrder returns resource names in a stable display order:
// GPU first (contains "gpu"), then CPU, then memory, then others alphabetically.
func resourceDisplayOrder(m map[corev1.ResourceName]resourceTotals) []corev1.ResourceName {
	var gpu, cpu, mem, other []corev1.ResourceName
	for rName := range m {
		s := string(rName)
		switch {
		case strings.Contains(s, "gpu") || strings.Contains(s, "GPU"):
			gpu = append(gpu, rName)
		case rName == corev1.ResourceCPU:
			cpu = append(cpu, rName)
		case rName == corev1.ResourceMemory:
			mem = append(mem, rName)
		default:
			other = append(other, rName)
		}
	}
	return append(append(append(gpu, cpu...), mem...), other...)
}

// quantityValue converts a resource.Quantity to a display integer.
// GPU/extended resources: value in units. CPU: millicores→cores. Memory: bytes→GiB.
func quantityValue(rName corev1.ResourceName, q resource.Quantity) int64 {
	switch rName {
	case corev1.ResourceCPU:
		return q.MilliValue() / 1000
	case corev1.ResourceMemory:
		return q.Value() / (1024 * 1024 * 1024)
	default:
		return q.Value()
	}
}

// shortResourceName returns a short display name for a resource.
func shortResourceName(rName corev1.ResourceName) string {
	s := string(rName)
	// nvidia.com/gpu → GPU, cpu → CPU, memory → Mem
	switch {
	case strings.Contains(s, "gpu") || strings.Contains(s, "GPU"):
		return "GPU"
	case rName == corev1.ResourceCPU:
		return "CPU"
	case rName == corev1.ResourceMemory:
		return "Mem"
	default:
		// last segment after /
		if idx := strings.LastIndex(s, "/"); idx >= 0 {
			return s[idx+1:]
		}
		return s
	}
}

// stripANSI is a rough ANSI escape stripper used for width estimation.
// Not used for rendering — only for len() approximation.
func stripANSI(s string) string {
	out := make([]byte, 0, len(s))
	inEsc := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == 0x1b {
			inEsc = true
			continue
		}
		if inEsc {
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
				inEsc = false
			}
			continue
		}
		out = append(out, c)
	}
	return string(out)
}
