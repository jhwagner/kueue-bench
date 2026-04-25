package tui

import (
	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
)

// ColumnSpec declares one column's layout behavior for ComputeWidths.
//
// Three dials control what happens as the terminal resizes:
//   - Flex: absorbs slack when there's extra width; gives it back first under pressure
//   - Priority: >0 columns can be dropped (lowest first) when content can't fit
//   - MinWidth / MaxWidth: hard floor / ceiling on the column's rendered width
//
// A column with Priority == 0 is required and never dropped. A column with
// Flex == 0 stays at its natural width (still bounded by Min/Max).
type ColumnSpec struct {
	Title    string
	MinWidth int // hard floor. Columns never shrink below this.
	MaxWidth int // hard ceiling; 0 means unbounded.
	Flex     int // weight for distributing slack/pressure; 0 = rigid.
	Priority int // 0 = required; higher = kept longer under pressure.
}

// cellPadding is the horizontal overhead bubbles/table adds per column via
// its Cell style (Padding(0, 1) = 1 char left + 1 char right). ComputeWidths
// subtracts this from the terminal width budget so the final rendered table
// actually fits.
const cellPadding = 2

// ComputeWidths returns one width per spec (same order, same length). A width
// of 0 means the column was dropped and should not be rendered;
// bubbles/table honors Width <= 0 by skipping the column entirely, so callers
// can pass the returned widths directly without filtering.
//
// Algorithm:
//  1. Natural width per column = max(title, widest cell), clamped to [Min, Max].
//  2. If total exceeds termWidth: shrink Flex columns toward MinWidth
//     proportional to Flex weight.
//  3. Still overflowing: drop the lowest-Priority column (Priority > 0),
//     restore remaining columns to natural, re-shrink. Repeat.
//  4. Still overflowing after all droppable columns dropped: clamp to
//     MinWidth; bubbles/table will ellipsize cells that exceed their width.
//  5. If room to spare: grow Flex columns proportional to Flex weight,
//     respecting MaxWidth.
//
// Cell strings may contain ANSI escape sequences (colors, styling);
// lipgloss.Width is used for measurement so escapes don't inflate widths.
func ComputeWidths(specs []ColumnSpec, rows [][]string, termWidth int) []int {
	n := len(specs)
	if n == 0 {
		return nil
	}

	natural := naturalWidths(specs, rows)

	kept := make([]int, 0, n)
	for i := range specs {
		kept = append(kept, i)
	}

	widths := make([]int, n)
	copy(widths, natural)

	shrinkFlex(specs, widths, kept, termWidth)

	// Drop lowest-Priority columns until we fit (or nothing droppable remains).
	for !fits(widths, kept, termWidth) {
		dropIdx := lowestPriorityIndex(specs, kept)
		if dropIdx == -1 {
			break
		}
		kept = removeIndex(kept, dropIdx)
		widths[dropIdx] = 0
		// Reset survivors to natural; dropping may have freed room for others to grow back.
		for _, i := range kept {
			widths[i] = natural[i]
		}
		shrinkFlex(specs, widths, kept, termWidth)
	}

	// Last-resort: terminal is narrower than MinWidths sum. Clamp survivors
	// to their MinWidth — bubbles/table will ellipsize overflow at render time.
	if !fits(widths, kept, termWidth) {
		for _, i := range kept {
			widths[i] = specs[i].MinWidth
		}
	}

	growFlex(specs, widths, kept, termWidth)

	return widths
}

// BuildColumns pairs specs with computed widths to produce bubbles/table columns.
// Columns with width 0 are passed through; bubbles/table skips them at render time.
func BuildColumns(specs []ColumnSpec, widths []int) []table.Column {
	cols := make([]table.Column, len(specs))
	for i, s := range specs {
		cols[i] = table.Column{Title: s.Title, Width: widths[i]}
	}
	return cols
}

// --- internals ---------------------------------------------------------------

func naturalWidths(specs []ColumnSpec, rows [][]string) []int {
	natural := make([]int, len(specs))
	for i, s := range specs {
		w := lipgloss.Width(s.Title)
		for _, row := range rows {
			if i >= len(row) {
				continue
			}
			if cw := lipgloss.Width(row[i]); cw > w {
				w = cw
			}
		}
		if s.MaxWidth > 0 && w > s.MaxWidth {
			w = s.MaxWidth
		}
		if w < s.MinWidth {
			w = s.MinWidth
		}
		natural[i] = w
	}
	return natural
}

// fits reports whether the kept columns' widths plus padding fit in termWidth.
func fits(widths, kept []int, termWidth int) bool {
	total := cellPadding * len(kept)
	for _, i := range kept {
		total += widths[i]
	}
	return total <= termWidth
}

// shrinkFlex reduces flex columns toward their MinWidth to absorb overflow.
// Distribution is proportional to each column's Flex weight, capped by the
// column's own shrink room (current width - MinWidth). Iterates until the
// table fits or no more shrink room exists.
func shrinkFlex(specs []ColumnSpec, widths, kept []int, termWidth int) {
	for {
		excess := -termWidth + cellPadding*len(kept)
		for _, i := range kept {
			excess += widths[i]
		}
		if excess <= 0 {
			return
		}

		totalWeight := 0
		for _, i := range kept {
			if specs[i].Flex > 0 && widths[i] > specs[i].MinWidth {
				totalWeight += specs[i].Flex
			}
		}
		if totalWeight == 0 {
			return
		}

		shrunk := 0
		for _, i := range kept {
			if specs[i].Flex <= 0 {
				continue
			}
			room := widths[i] - specs[i].MinWidth
			if room <= 0 {
				continue
			}
			share := excess * specs[i].Flex / totalWeight
			if share < 1 {
				share = 1 // ensure progress under integer rounding
			}
			if share > room {
				share = room
			}
			if share > excess-shrunk {
				share = excess - shrunk
			}
			widths[i] -= share
			shrunk += share
			if shrunk >= excess {
				break
			}
		}
		if shrunk == 0 {
			return
		}
	}
}

// growFlex distributes leftover width to flex columns, capped by MaxWidth.
func growFlex(specs []ColumnSpec, widths, kept []int, termWidth int) {
	for {
		slack := termWidth - cellPadding*len(kept)
		for _, i := range kept {
			slack -= widths[i]
		}
		if slack <= 0 {
			return
		}

		totalWeight := 0
		for _, i := range kept {
			if specs[i].Flex <= 0 {
				continue
			}
			if specs[i].MaxWidth > 0 && widths[i] >= specs[i].MaxWidth {
				continue
			}
			totalWeight += specs[i].Flex
		}
		if totalWeight == 0 {
			return
		}

		grew := 0
		for _, i := range kept {
			if specs[i].Flex <= 0 {
				continue
			}
			room := slack
			if specs[i].MaxWidth > 0 {
				room = specs[i].MaxWidth - widths[i]
				if room <= 0 {
					continue
				}
			}
			share := slack * specs[i].Flex / totalWeight
			if share < 1 {
				share = 1
			}
			if share > room {
				share = room
			}
			if share > slack-grew {
				share = slack - grew
			}
			widths[i] += share
			grew += share
			if grew >= slack {
				break
			}
		}
		if grew == 0 {
			return
		}
	}
}

// lowestPriorityIndex returns the index of the kept column that should be
// dropped first (smallest Priority > 0; ties broken by rightmost index to
// preserve leftmost columns which are typically more important). Returns -1
// when no droppable column remains.
func lowestPriorityIndex(specs []ColumnSpec, kept []int) int {
	best := -1
	bestPri := 0
	for _, i := range kept {
		p := specs[i].Priority
		if p <= 0 {
			continue
		}
		if best == -1 || p < bestPri || (p == bestPri && i > best) {
			best = i
			bestPri = p
		}
	}
	return best
}

func removeIndex(kept []int, idx int) []int {
	out := kept[:0]
	for _, i := range kept {
		if i != idx {
			out = append(out, i)
		}
	}
	return out
}
