package tui

import (
	"context"
	"fmt"
	"sort"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/jhwagner/kueue-bench/pkg/topology"
	"github.com/jhwagner/kueue-bench/pkg/watcher"
)

// navLevel tracks which view is active.
type navLevel int

const (
	navOverview navLevel = iota
	navDetail
)

// overviewTab is the active tab on the overview level.
type overviewTab int

const (
	tabQueues overviewTab = iota
	tabWorkloads
)

// snapshotMsg carries a fresh snapshot from the watcher.
type snapshotMsg struct {
	snap watcher.Snapshot
}

// syncDoneMsg signals that the initial cache sync completed (or failed).
type syncDoneMsg struct {
	err error
}

// Model is the root BubbleTea model.
type Model struct {
	// Topology
	topologyName string
	clusters     map[string]topology.Cluster
	clusterOrder []string // sorted for stable cycling
	// Current cluster
	currentCluster string
	clusterRole    string
	isManagement   bool

	// Watcher
	watcher       *watcher.Watcher
	watcherCtx    context.Context
	cancelWatcher context.CancelFunc
	snapshot      watcher.Snapshot
	connState     connectionState

	// Navigation
	navLevel    navLevel
	overviewTab overviewTab
	detailView  tea.Model // nil when navLevel == navOverview

	// Overview sub-views
	queueView queueViewModel
	eventView eventViewModel

	// Terminal dimensions
	width  int
	height int

	// Keybindings
	keys keyMap

	// Error shown in status bar (cleared on next snapshot)
	statusErr string
}

// New creates a Model connected to the named cluster within the topology.
// The watcher is created but not started; BubbleTea's Init triggers the start.
func New(topologyName, clusterName string, meta topology.Metadata) (*Model, error) {
	// Build stable cluster ordering.
	order := make([]string, 0, len(meta.Clusters))
	for name := range meta.Clusters {
		order = append(order, name)
	}
	sort.Strings(order)

	cluster, ok := meta.Clusters[clusterName]
	if !ok {
		return nil, fmt.Errorf("cluster %q not found in topology", clusterName)
	}

	isManagement := cluster.Role == "management"
	w, err := watcher.New(cluster.KubeconfigPath, isManagement)
	if err != nil {
		return nil, fmt.Errorf("create watcher: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Model{
		topologyName:   topologyName,
		clusters:       meta.Clusters,
		clusterOrder:   order,
		currentCluster: clusterName,
		clusterRole:    cluster.Role,
		isManagement:   isManagement,
		watcher:        w,
		watcherCtx:     ctx,
		cancelWatcher:  cancel,
		connState:      stateConnecting,
		keys:           defaultKeyMap,
		queueView:      newQueueView(),
		eventView:      newEventView(0, 0), // sized on first WindowSizeMsg
	}, nil
}

// --- tea.Model implementation ------------------------------------------------

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		startWatcher(m.watcher, m.watcherCtx),
		waitForUpdate(m.watcher.Store()),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		mh, eh := m.panelHeights()
		m.queueView.refresh(m.snapshot, m.width, mh)
		m.eventView.refresh(m.snapshot, m.width, eh)
		return m, nil

	case syncDoneMsg:
		if msg.err != nil {
			m.connState = stateDisconnected
			m.statusErr = msg.err.Error()
		} else {
			m.connState = stateConnected
		}
		return m, nil

	case snapshotMsg:
		m.snapshot = msg.snap
		if m.width > 0 {
			mh, eh := m.panelHeights()
			m.queueView.refresh(m.snapshot, m.width, mh)
			m.eventView.refresh(m.snapshot, m.width, eh)
		}
		return m, waitForUpdate(m.watcher.Store())

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	// Delegate to detail view if active.
	if m.navLevel == navDetail && m.detailView != nil {
		var cmd tea.Cmd
		m.detailView, cmd = m.detailView.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		m.cancelWatcher()
		m.watcher.Stop()
		return m, tea.Quit

	case key.Matches(msg, m.keys.Esc):
		if m.navLevel == navDetail {
			m.navLevel = navOverview
			m.detailView = nil
		}
		return m, nil

	case key.Matches(msg, m.keys.Tab1):
		if m.navLevel == navOverview {
			m.overviewTab = tabQueues
		}
		return m, nil

	case key.Matches(msg, m.keys.Tab2):
		if m.navLevel == navOverview {
			m.overviewTab = tabWorkloads
		}
		return m, nil
	}

	// At overview level, forward navigation keys to the active sub-view.
	if m.navLevel == navOverview {
		if key.Matches(msg, m.keys.Up) || key.Matches(msg, m.keys.Down) {
			if m.overviewTab == tabQueues {
				cmd := m.queueView.update(msg)
				return m, cmd
			}
		}
		// Event log scrolling: pass scroll keys always (even when queue tab is active).
		if isScrollKey(msg) {
			cmd := m.eventView.update(msg)
			return m, cmd
		}
	}

	return m, nil
}

// isScrollKey returns true for keys that the event viewport should consume.
func isScrollKey(msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case "pgup", "pgdown", "ctrl+u", "ctrl+d":
		return true
	}
	return false
}

func (m Model) View() tea.View {
	if m.width == 0 {
		v := tea.NewView("")
		v.AltScreen = true
		return v
	}

	topBar := renderTopBar(m.width, m.topologyName, m.currentCluster, m.clusterRole, m.connState)
	summaryBar := renderSummaryBar(m.width, m.snapshot)
	tabBar := renderTabBar(m.width, m.overviewTab)

	var hints string
	if m.navLevel == navDetail {
		hints = detailHints()
	} else {
		hints = overviewHints(m.overviewTab)
	}
	hintBar := renderHintBar(m.width, hints)

	var content string
	if m.navLevel == navDetail && m.detailView != nil {
		// Detail view takes over the full content area (no event log).
		content = topBar + "\n" + m.detailView.View().Content + "\n" + hintBar
	} else {
		// Overview: queue/workload table + event panel.
		var mainContent string
		switch m.overviewTab {
		case tabQueues:
			mainContent = m.queueView.view()
		case tabWorkloads:
			mainContent = "  Workload view — coming in Commit 7"
		}
		sep := eventSeparator(m.width)
		eventContent := m.eventView.view()
		content = topBar + "\n" +
			summaryBar + "\n" +
			tabBar + "\n" +
			mainContent + "\n" +
			sep + "\n" +
			eventContent + "\n" +
			hintBar
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// panelHeights returns (mainHeight, eventHeight) for the current terminal size.
// Layout: topBar(1) + summaryBar(1) + tabBar(1) + mainPanel + sep(1) + eventPanel + hintBar(1) = height
// → mainPanel + eventPanel = height - 5
func (m Model) panelHeights() (mainH, eventH int) {
	available := m.height - 5 // 5 fixed lines: top + summary + tab + sep + hint
	if available < 4 {
		available = 4
	}
	eventH = available / 4
	if eventH < 3 {
		eventH = 3
	}
	if eventH > 8 {
		eventH = 8
	}
	mainH = available - eventH
	if mainH < 1 {
		mainH = 1
	}
	return mainH, eventH
}

// --- Commands ----------------------------------------------------------------

// startWatcher starts the watcher in a goroutine and signals syncDoneMsg when done.
func startWatcher(w *watcher.Watcher, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		err := w.Start(ctx)
		return syncDoneMsg{err: err}
	}
}

// waitForUpdate blocks until the store signals a mutation, then returns a snapshotMsg.
func waitForUpdate(store *watcher.Store) tea.Cmd {
	return func() tea.Msg {
		<-store.UpdateCh()
		return snapshotMsg{snap: store.Snapshot()}
	}
}

// --- Helpers -----------------------------------------------------------------

// renderTabBar renders the tab selector line.
func renderTabBar(_ int, active overviewTab) string {
	q := styleTabInactive.Render("[1] Queues")
	w := styleTabInactive.Render("[2] Workloads")
	if active == tabQueues {
		q = styleTabActive.Render("[1] Queues")
	} else {
		w = styleTabActive.Render("[2] Workloads")
	}
	sep := styleTabInactive.Render(" ─ ")
	line := " " + q + sep + w
	return line
}
