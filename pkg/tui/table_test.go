package tui

import (
	"reflect"
	"testing"
)

func TestComputeWidths(t *testing.T) {
	tests := []struct {
		name      string
		specs     []ColumnSpec
		rows      [][]string
		termWidth int
		want      []int
	}{
		{
			name:      "nil specs returns nil",
			specs:     nil,
			termWidth: 100,
			want:      nil,
		},

		// --- Natural sizing ------------------------------------------------------

		{
			// Title longer than cell content → title wins.
			name:      "natural sizing: title wins over content",
			specs:     []ColumnSpec{{Title: "HEADER", MinWidth: 3}},
			rows:      [][]string{{"hi"}},
			termWidth: 100,
			want:      []int{6},
		},
		{
			// Cell content exceeds MaxWidth → clamped.
			name:      "natural sizing: content clamped to MaxWidth",
			specs:     []ColumnSpec{{Title: "COL", MinWidth: 3, MaxWidth: 5}},
			rows:      [][]string{{"longerstring"}},
			termWidth: 100,
			want:      []int{5},
		},
		{
			// Cell content shorter than MinWidth → floored.
			name:      "natural sizing: content floored to MinWidth",
			specs:     []ColumnSpec{{Title: "A", MinWidth: 10}},
			rows:      [][]string{{"hi"}},
			termWidth: 100,
			want:      []int{10},
		},
		{
			// No rows: only title contributes to natural width.
			name:      "natural sizing: empty rows, title determines width",
			specs:     []ColumnSpec{{Title: "LONGHEADER", MinWidth: 3}},
			rows:      nil,
			termWidth: 100,
			want:      []int{10},
		},

		// --- Flex grow -----------------------------------------------------------

		{
			// Single flex column absorbs all slack.
			// budget = 30 - 2*2 = 26; naturals = 5+5=10; slack = 16; FLEX → 5+16=21.
			name: "grow: single flex col absorbs all slack",
			specs: []ColumnSpec{
				{Title: "FIXED", MinWidth: 5},
				{Title: "FLEX", MinWidth: 5, Flex: 1},
			},
			termWidth: 30,
			want:      []int{5, 21},
		},
		{
			// Two flex cols split slack 2:1 by weight.
			// budget = 32 - 4 = 28; naturals = 5+5=10; slack = 18.
			// A gets 18*2/3=12 → 17; B gets 18*1/3=6 → 11.
			name: "grow: two flex cols split slack by weight",
			specs: []ColumnSpec{
				{Title: "A", MinWidth: 5, Flex: 2},
				{Title: "B", MinWidth: 5, Flex: 1},
			},
			termWidth: 32,
			want:      []int{17, 11},
		},
		{
			// Flex growth stops at MaxWidth; leftover slack flows to uncapped col.
			// budget = 40 - 4 = 36; naturals = 5+5=10; slack = 26.
			// Iter 1: A room=5 (capped at MaxWidth=10), B gets 13. grew=18, slack left=8.
			// Iter 2: A at max; B absorbs remaining 8. Final: A=10, B=26.
			name: "grow: flex col capped at MaxWidth, remainder to other col",
			specs: []ColumnSpec{
				{Title: "A", MinWidth: 5, MaxWidth: 10, Flex: 1},
				{Title: "B", MinWidth: 5, Flex: 1},
			},
			termWidth: 40,
			want:      []int{10, 26},
		},

		// --- Flex shrink ---------------------------------------------------------

		{
			// Flex column absorbs excess by shrinking.
			// budget = 18 - 4 = 14; FIXED natural=5, FLEX natural=13; excess=4.
			// FLEX shrinks 4 → 9.
			name: "shrink: single flex col shrinks to absorb excess",
			specs: []ColumnSpec{
				{Title: "FIXED", MinWidth: 5},
				{Title: "FLEX", MinWidth: 5, Flex: 1},
			},
			rows:      [][]string{{"hello", "a long string"}},
			termWidth: 18,
			want:      []int{5, 9},
		},
		{
			// Two flex cols shrink proportionally by weight.
			// budget = 18 - 4 = 14; naturals = 10+10=20; excess = 6.
			// Iter 1: A(Flex:2) shrinks 4, B(Flex:1) shrinks 2. A=6, B=8.
			name: "shrink: two flex cols shrink proportionally by weight",
			specs: []ColumnSpec{
				{Title: "A", MinWidth: 5, Flex: 2},
				{Title: "B", MinWidth: 5, Flex: 1},
			},
			rows:      [][]string{{"0123456789", "0123456789"}},
			termWidth: 18,
			want:      []int{6, 8},
		},
		{
			// Flex column cannot shrink below MinWidth; table stays over budget
			// until the last-resort MinWidth clamp.
			// budget = 4 - 2 = 2; natural=15; shrinks to MinWidth=5 (can't go lower).
			name: "shrink: flex col floors at MinWidth",
			specs: []ColumnSpec{
				{Title: "FLEX", MinWidth: 5, Flex: 1},
			},
			rows:      [][]string{{"a very long content string"}},
			termWidth: 4,
			want:      []int{5},
		},

		// --- Priority drop -------------------------------------------------------

		{
			// Middle column with Priority:1 is dropped; required cols keep natural widths.
			// N=3: 6+15=21 > 16. Shrink: no flex. Drop B (only droppable). kept=[0,2].
			// After drop: 4+5+5=14 ≤ 16. Fits.
			name: "drop: lowest-priority col removed when table overflows",
			specs: []ColumnSpec{
				{Title: "A", MinWidth: 5},
				{Title: "B", MinWidth: 5, Priority: 1},
				{Title: "C", MinWidth: 5},
			},
			termWidth: 16,
			want:      []int{5, 0, 5},
		},
		{
			// Lower Priority value drops first (Priority:1 before Priority:2).
			// N=3: 6+15=21 > 9. Drop B(p=1). kept=[0,2]: 4+10=14 > 9. Drop C(p=2). kept=[0]: 2+5=7 ≤ 9.
			name: "drop: lower Priority value drops before higher",
			specs: []ColumnSpec{
				{Title: "A", MinWidth: 5},
				{Title: "B", MinWidth: 5, Priority: 1},
				{Title: "C", MinWidth: 5, Priority: 2},
			},
			termWidth: 9,
			want:      []int{5, 0, 0},
		},
		{
			// Equal priorities: rightmost index drops first.
			// N=3: 6+15=21 > 14. B(i=1,p=1) and C(i=2,p=1) tied; C drops. kept=[0,1]: 4+10=14 ≤ 14. Fits.
			name: "drop: equal priority tiebreak favors rightmost",
			specs: []ColumnSpec{
				{Title: "A", MinWidth: 5},
				{Title: "B", MinWidth: 5, Priority: 1},
				{Title: "C", MinWidth: 5, Priority: 1},
			},
			termWidth: 14,
			want:      []int{5, 5, 0},
		},
		{
			// Priority:0 columns are required and never dropped regardless of pressure.
			// After DROP is removed, REQ(p=0) still doesn't fit; last-resort clamps to MinWidth.
			name: "drop: Priority 0 columns are never dropped",
			specs: []ColumnSpec{
				{Title: "REQ", MinWidth: 5},
				{Title: "DROP", MinWidth: 5, Priority: 1},
			},
			termWidth: 5,
			want:      []int{5, 0},
		},

		// --- Pathological inputs -------------------------------------------------

		{
			// Terminal too narrow even for MinWidths; returns MinWidths, no panic.
			name: "pathological: termWidth smaller than MinWidths sum",
			specs: []ColumnSpec{
				{Title: "A", MinWidth: 5},
				{Title: "B", MinWidth: 8},
			},
			termWidth: 1,
			want:      []int{5, 8},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeWidths(tt.specs, tt.rows, tt.termWidth)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
