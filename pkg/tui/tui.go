package tui

import (
	"context"
	"fmt"
	"sort"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jhwagner/kueue-bench/pkg/topology"
	"github.com/jhwagner/kueue-bench/pkg/watcher"
)

// Chrome line counts for the overview layout.
// Each constant names a group of fixed (non-content) lines.
// panelHeights() subtracts these; View() must assemble exactly these lines.
const (
	chromeTopLines    = 3 // topBar + summaryBar + tabBar
	chromeBottomLines = 4 // legendLine + eventSep + hintSep + hintBar
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
	gen  int
	snap watcher.Snapshot
}

// syncDoneMsg signals that the initial cache sync completed (or failed).
type syncDoneMsg struct {
	gen int
	err error
}

// podWatchReadyMsg signals that StartPodWatch completed (or failed).
type podWatchReadyMsg struct {
	gen int
	err error
}

// watcherDeadMsg is sent by waitForUpdate when the watcher context is cancelled,
// allowing the goroutine to exit cleanly. The model ignores it.
type watcherDeadMsg struct{}

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
	watcherGen    int // incremented on each cluster switch to discard stale messages
	snapshot      watcher.Snapshot
	connState     connectionState

	// Navigation
	navLevel    navLevel
	overviewTab overviewTab
	detailView  tea.Model // nil when navLevel == navOverview

	// Cluster picker overlay
	showClusterPicker bool
	clusterPicker     clusterPickerModel

	// Submit dialog overlay
	showSubmit  bool
	submitView  submitViewModel

	// Overview sub-views
	queueView    queueViewModel
	workloadView workloadViewModel
	eventView    eventViewModel

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
		workloadView:   newWorkloadView(isManagement),
		eventView:      newEventView(0, 0), // sized on first WindowSizeMsg
	}, nil
}

// --- tea.Model implementation ------------------------------------------------

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		startWatcher(m.watcher, m.watcherCtx, m.watcherGen),
		waitForUpdate(m.watcher.Store(), m.watcherCtx.Done(), m.watcherGen),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		mh, eh := m.panelHeights()
		m.queueView.refresh(m.snapshot, m.width, mh)
		m.workloadView.refresh(m.snapshot, m.width, mh)
		m.eventView.refresh(m.snapshot, m.width, eh)
		if m.navLevel == navDetail && m.detailView != nil {
			var cmd tea.Cmd
			m.detailView, cmd = m.detailView.Update(msg)
			_ = cmd
		}
		return m, nil

	case watcherDeadMsg:
		return m, nil

	case syncDoneMsg:
		if msg.gen != m.watcherGen {
			return m, nil
		}
		if msg.err != nil {
			m.connState = stateDisconnected
			m.statusErr = msg.err.Error()
		} else {
			m.connState = stateConnected
		}
		return m, nil

	case podWatchReadyMsg:
		if msg.gen != m.watcherGen {
			return m, nil
		}
		if msg.err != nil {
			m.statusErr = "pod watch: " + msg.err.Error()
		}
		return m, nil

	case snapshotMsg:
		if msg.gen != m.watcherGen {
			return m, nil
		}
		m.snapshot = msg.snap
		if m.width > 0 {
			mh, eh := m.panelHeights()
			m.queueView.refresh(m.snapshot, m.width, mh)
			m.workloadView.refresh(m.snapshot, m.width, mh)
			m.eventView.refresh(m.snapshot, m.width, eh)
		}
		if m.showSubmit {
			m.submitView.refreshOptions(m.snapshot)
		}
		if m.navLevel == navDetail && m.detailView != nil {
			var cmd tea.Cmd
			m.detailView, cmd = m.detailView.Update(msg)
			_ = cmd
		}
		return m, waitForUpdate(m.watcher.Store(), m.watcherCtx.Done(), m.watcherGen)

	case submitResultMsg:
		if msg.err != nil {
			m.submitView.submitErr = msg.err.Error()
		} else {
			m.showSubmit = false
		}
		return m, nil

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
	// Submit dialog intercepts all keys while open.
	if m.showSubmit {
		if key.Matches(msg, m.keys.Quit) || key.Matches(msg, m.keys.Esc) {
			m.showSubmit = false
			return m, nil
		}
		cmd := m.submitView.update(msg, m.keys)
		return m, cmd
	}

	// Cluster picker intercepts all keys while open.
	if m.showClusterPicker {
		switch {
		case key.Matches(msg, m.keys.Quit), key.Matches(msg, m.keys.Esc):
			m.showClusterPicker = false
		case key.Matches(msg, m.keys.Up):
			m.clusterPicker.moveUp()
		case key.Matches(msg, m.keys.Down):
			m.clusterPicker.moveDown()
		case key.Matches(msg, m.keys.Enter):
			chosen := m.clusterPicker.selected()
			m.showClusterPicker = false
			if chosen != "" && chosen != m.currentCluster {
				return m.switchToCluster(chosen)
			}
		}
		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.Quit):
		m.cancelWatcher()
		m.watcher.Stop()
		return m, tea.Quit

	case key.Matches(msg, m.keys.Esc):
		if m.navLevel == navDetail {
			m.watcher.StopPodWatch()
			m.navLevel = navOverview
			m.detailView = nil
		}
		return m, nil

	case key.Matches(msg, m.keys.Cluster):
		if len(m.clusterOrder) > 1 {
			m.clusterPicker = newClusterPicker(m.clusterOrder, m.currentCluster)
			m.showClusterPicker = true
		}
		return m, nil

	case key.Matches(msg, m.keys.Submit):
		cluster := m.clusters[m.currentCluster]
		var cmd tea.Cmd
		m.submitView, cmd = newSubmitView(m.snapshot, cluster.KubeconfigPath)
		m.showSubmit = true
		return m, cmd

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

	case key.Matches(msg, m.keys.Enter):
		if m.navLevel == navOverview {
			switch m.overviewTab {
			case tabQueues:
				name := m.queueView.selectedQueueName()
				if name != "" {
					detail := newQueueDetail(name, m.snapshot, m.width, m.height-2)
					m.navLevel = navDetail
					m.detailView = detail
				}
			case tabWorkloads:
				wlKey := m.workloadView.selectedWorkloadKey()
				if wlKey != "" {
					detail := newWorkloadDetail(wlKey, m.isManagement, m.snapshot, m.width, m.height-2)
					m.navLevel = navDetail
					m.detailView = detail
					var podCmd tea.Cmd
					if wl, ok := m.snapshot.Workloads[wlKey]; ok {
						if sel := watcher.PodLabelSelector(wl.OwnerKind, wl.OwnerName); sel != "" {
							podCmd = startPodWatch(m.watcherCtx, m.watcher, wl.Namespace, sel, m.watcherGen)
						}
					}
					return m, podCmd
				}
			}
		}
		return m, nil
	}

	// At overview level, forward navigation keys to the active sub-view.
	if m.navLevel == navOverview {
		if key.Matches(msg, m.keys.Up) || key.Matches(msg, m.keys.Down) {
			switch m.overviewTab {
			case tabQueues:
				cmd := m.queueView.update(msg)
				return m, cmd
			case tabWorkloads:
				cmd := m.workloadView.update(msg)
				return m, cmd
			}
		}
		if key.Matches(msg, m.keys.Filter) && m.overviewTab == tabWorkloads {
			m.workloadView.cycleFilter()
			return m, nil
		}
		// Event log scrolling: pass scroll keys always (even when queue tab is active).
		if isScrollKey(msg) {
			cmd := m.eventView.update(msg)
			return m, cmd
		}
	}

	// Forward remaining keys to detail view when active.
	if m.navLevel == navDetail && m.detailView != nil {
		var cmd tea.Cmd
		m.detailView, cmd = m.detailView.Update(msg)
		return m, cmd
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
	hintBar := styleHintBar.Width(m.width).Render(hints)

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
			mainContent = m.workloadView.view()
		}

		// Legend footer for the queue pane: shown when any flavor is near/at
		// capacity. Always occupies one line to keep the layout stable.
		var legend string
		if m.overviewTab == tabQueues && anyFlavorAtCapacity(m.snapshot) {
			legend = flavorIndicatorLegend()
		}
		legendLine := renderQueueLegendLine(m.width, legend)

		sep := eventSeparator(m.width)
		eventContent := m.eventView.view()
		hintSep := renderHintSep(m.width)
		content = lipgloss.JoinVertical(lipgloss.Left,
			topBar, summaryBar, tabBar,
			mainContent,
			legendLine, sep,
			eventContent,
			hintSep, hintBar,
		)
	}

	// Overlays (cluster picker takes priority if both somehow open).
	if m.showSubmit {
		content = m.submitView.view(m.width, m.height)
	}
	if m.showClusterPicker {
		content = renderClusterPicker(m.clusterPicker, m.clusters, m.width, m.height)
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// panelHeights returns (mainHeight, eventHeight) for the current terminal size.
// available = height - chromeTopLines - chromeBottomLines
func (m Model) panelHeights() (mainH, eventH int) {
	available := m.height - chromeTopLines - chromeBottomLines
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

// switchToCluster stops the current watcher and starts a new one for the named
// cluster. The old waitForUpdate goroutine is drained via context cancellation;
// stale messages are discarded by watcherGen.
func (m Model) switchToCluster(name string) (tea.Model, tea.Cmd) {
	cluster, ok := m.clusters[name]
	if !ok {
		return m, nil
	}
	isManagement := cluster.Role == "management"

	// Tear down current watcher. cancelWatcher unblocks the old waitForUpdate
	// goroutine via the done channel; Stop() shuts down informer goroutines.
	m.watcher.StopPodWatch()
	m.cancelWatcher()
	m.watcher.Stop()

	w, err := watcher.New(cluster.KubeconfigPath, isManagement)
	if err != nil {
		m.statusErr = "switch cluster: " + err.Error()
		return m, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.watcher = w
	m.watcherCtx = ctx
	m.cancelWatcher = cancel
	m.watcherGen++
	m.currentCluster = name
	m.clusterRole = cluster.Role
	m.isManagement = isManagement
	m.connState = stateConnecting
	m.snapshot = watcher.Snapshot{}
	m.navLevel = navOverview
	m.detailView = nil
	m.workloadView = newWorkloadView(isManagement)

	if m.width > 0 {
		mh, eh := m.panelHeights()
		m.queueView.refresh(m.snapshot, m.width, mh)
		m.workloadView.refresh(m.snapshot, m.width, mh)
		m.eventView.refresh(m.snapshot, m.width, eh)
	}

	gen := m.watcherGen
	return m, tea.Batch(
		startWatcher(m.watcher, m.watcherCtx, gen),
		waitForUpdate(m.watcher.Store(), m.watcherCtx.Done(), gen),
	)
}

// --- Commands ----------------------------------------------------------------

// startWatcher starts the watcher in a goroutine and signals syncDoneMsg when done.
func startWatcher(w *watcher.Watcher, ctx context.Context, gen int) tea.Cmd {
	return func() tea.Msg {
		err := w.Start(ctx)
		return syncDoneMsg{gen: gen, err: err}
	}
}

// waitForUpdate blocks until the store signals a mutation or the watcher context
// is cancelled. On cancellation it returns watcherDeadMsg so the goroutine exits
// cleanly instead of leaking.
func waitForUpdate(store *watcher.Store, done <-chan struct{}, gen int) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-store.UpdateCh():
			return snapshotMsg{gen: gen, snap: store.Snapshot()}
		case <-done:
			return watcherDeadMsg{}
		}
	}
}

// startPodWatch starts a scoped pod informer and signals podWatchReadyMsg on completion.
func startPodWatch(ctx context.Context, w *watcher.Watcher, namespace, labelSelector string, gen int) tea.Cmd {
	return func() tea.Msg {
		err := w.StartPodWatch(ctx, namespace, labelSelector)
		return podWatchReadyMsg{gen: gen, err: err}
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
