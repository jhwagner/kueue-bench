package watcher

import "sync"

const eventBufCap = 500

// Store is a thread-safe snapshot store. Informer event handlers write to it;
// the TUI reads from it via Snapshot().
type Store struct {
	mu                 sync.RWMutex
	queues             map[string]QueueSnapshot
	workloads          map[string]WorkloadSnapshot // key: "namespace/name"
	multiKueueClusters map[string]MultiKueueClusterSnapshot

	// ring buffer for events
	eventBuf  [eventBufCap]EventEntry
	eventHead int // index of next write position
	eventSize int // current number of valid entries

	// updateCh receives a signal (non-blocking) on every mutation.
	// The TUI drains this channel to know when to refresh.
	updateCh chan struct{}
}

// NewStore returns an initialized Store.
func NewStore() *Store {
	return &Store{
		queues:             make(map[string]QueueSnapshot),
		workloads:          make(map[string]WorkloadSnapshot),
		multiKueueClusters: make(map[string]MultiKueueClusterSnapshot),
		updateCh:           make(chan struct{}, 1),
	}
}

// UpdateCh returns the channel that receives a signal on every mutation.
// Callers should drain it and call Snapshot() to get the latest state.
func (s *Store) UpdateCh() <-chan struct{} {
	return s.updateCh
}

// Snapshot returns a deep copy of the current cluster state.
func (s *Store) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snap := Snapshot{
		Queues:             make(map[string]QueueSnapshot, len(s.queues)),
		Workloads:          make(map[string]WorkloadSnapshot, len(s.workloads)),
		MultiKueueClusters: make(map[string]MultiKueueClusterSnapshot, len(s.multiKueueClusters)),
		Events:             make([]EventEntry, s.eventSize),
	}

	for k, v := range s.queues {
		snap.Queues[k] = v.deepCopy()
	}
	for k, v := range s.workloads {
		snap.Workloads[k] = v.deepCopy()
	}
	for k, v := range s.multiKueueClusters {
		snap.MultiKueueClusters[k] = v.deepCopy()
	}

	// Copy ring buffer in order: oldest → newest
	start := (s.eventHead - s.eventSize + eventBufCap) % eventBufCap
	for i := range s.eventSize {
		snap.Events[i] = s.eventBuf[(start+i)%eventBufCap]
	}

	return snap
}

// UpsertQueue inserts or replaces a ClusterQueue snapshot.
func (s *Store) UpsertQueue(q QueueSnapshot) {
	s.mu.Lock()
	s.queues[q.Name] = q
	s.mu.Unlock()
	s.signal()
}

// DeleteQueue removes a ClusterQueue snapshot by name.
func (s *Store) DeleteQueue(name string) {
	s.mu.Lock()
	delete(s.queues, name)
	s.mu.Unlock()
	s.signal()
}

// UpsertWorkload inserts or replaces a Workload snapshot.
func (s *Store) UpsertWorkload(w WorkloadSnapshot) {
	key := w.Namespace + "/" + w.Name
	s.mu.Lock()
	s.workloads[key] = w
	s.mu.Unlock()
	s.signal()
}

// DeleteWorkload removes a Workload snapshot by namespace and name.
func (s *Store) DeleteWorkload(namespace, name string) {
	key := namespace + "/" + name
	s.mu.Lock()
	delete(s.workloads, key)
	s.mu.Unlock()
	s.signal()
}

// UpsertMultiKueueCluster inserts or replaces a MultiKueueCluster snapshot.
func (s *Store) UpsertMultiKueueCluster(c MultiKueueClusterSnapshot) {
	s.mu.Lock()
	s.multiKueueClusters[c.Name] = c
	s.mu.Unlock()
	s.signal()
}

// DeleteMultiKueueCluster removes a MultiKueueCluster snapshot by name.
func (s *Store) DeleteMultiKueueCluster(name string) {
	s.mu.Lock()
	delete(s.multiKueueClusters, name)
	s.mu.Unlock()
	s.signal()
}

// AppendEvent appends an event to the ring buffer, evicting the oldest if full.
func (s *Store) AppendEvent(e EventEntry) {
	s.mu.Lock()
	s.eventBuf[s.eventHead] = e
	s.eventHead = (s.eventHead + 1) % eventBufCap
	if s.eventSize < eventBufCap {
		s.eventSize++
	}
	s.mu.Unlock()
	s.signal()
}

// signal sends a non-blocking notification on updateCh.
func (s *Store) signal() {
	select {
	case s.updateCh <- struct{}{}:
	default:
	}
}
