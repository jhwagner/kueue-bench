package tui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Tab1    key.Binding
	Tab2    key.Binding
	Up      key.Binding
	Down    key.Binding
	Enter   key.Binding
	Esc     key.Binding
	Submit  key.Binding
	Cluster key.Binding
	Filter  key.Binding
	Help    key.Binding
	Quit    key.Binding
}

var defaultKeyMap = keyMap{
	Tab1: key.NewBinding(
		key.WithKeys("1"),
		key.WithHelp("1", "queues"),
	),
	Tab2: key.NewBinding(
		key.WithKeys("2"),
		key.WithHelp("2", "workloads"),
	),
	Up: key.NewBinding(
		key.WithKeys("k", "up"),
		key.WithHelp("k/↑", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("j", "down"),
		key.WithHelp("j/↓", "down"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "detail"),
	),
	Esc: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back"),
	),
	Submit: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "submit"),
	),
	Cluster: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "cluster"),
	),
	Filter: key.NewBinding(
		key.WithKeys("f"),
		key.WithHelp("f", "filter"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
}

// overviewHints returns context-sensitive key hints for the overview level.
func overviewHints(tab overviewTab) string {
	base := "[1/2]view  [j/k]nav  [enter]detail  [s]submit  [c]cluster  [?]help  [q]quit"
	if tab == tabWorkloads {
		return "[1/2]view  [j/k]nav  [enter]detail  [f]filter  [s]submit  [c]cluster  [q]quit"
	}
	return base
}

// detailHints returns key hints for detail views.
func detailHints() string {
	return "[j/k]scroll  [esc]back  [q]quit"
}
