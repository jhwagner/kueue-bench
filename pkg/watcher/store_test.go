package watcher

import (
	"fmt"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// --- helpers -----------------------------------------------------------------

func makeQueue(name, cohort string, pending int32) QueueSnapshot {
	return QueueSnapshot{
		Name:    name,
		Cohort:  cohort,
		Pending: pending,
		Active:  true,
		Flavors: []FlavorSnapshot{
			{
				Name: "default",
				Resources: map[corev1.ResourceName]ResourceSnapshot{
					corev1.ResourceCPU: {
						Nominal: resource.MustParse("10"),
						Used:    resource.MustParse("4"),
					},
				},
			},
		},
	}
}

func makeWorkload(ns, name, queue string) WorkloadSnapshot {
	return WorkloadSnapshot{
		Name:      name,
		Namespace: ns,
		Queue:     queue,
		Status:    WorkloadStatusPending,
		CreatedAt: time.Now(),
		Resources: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
	}
}

// --- upsert / delete ---------------------------------------------------------

func TestUpsertDeleteQueue(t *testing.T) {
	s := NewStore()

	q := makeQueue("team-a", "gpu-pool", 5)
	s.UpsertQueue(q)

	snap := s.Snapshot()
	if len(snap.Queues) != 1 {
		t.Fatalf("expected 1 queue, got %d", len(snap.Queues))
	}
	got, ok := snap.Queues["team-a"]
	if !ok {
		t.Fatal("queue team-a not found in snapshot")
	}
	if got.Pending != 5 {
		t.Errorf("pending: want 5, got %d", got.Pending)
	}

	s.DeleteQueue("team-a")
	snap = s.Snapshot()
	if len(snap.Queues) != 0 {
		t.Fatalf("expected 0 queues after delete, got %d", len(snap.Queues))
	}
}

func TestUpsertDeleteWorkload(t *testing.T) {
	s := NewStore()

	w := makeWorkload("default", "job-abc", "team-a")
	s.UpsertWorkload(w)

	snap := s.Snapshot()
	if len(snap.Workloads) != 1 {
		t.Fatalf("expected 1 workload, got %d", len(snap.Workloads))
	}
	got, ok := snap.Workloads["default/job-abc"]
	if !ok {
		t.Fatal("workload default/job-abc not found")
	}
	if got.Queue != "team-a" {
		t.Errorf("queue: want team-a, got %s", got.Queue)
	}

	s.DeleteWorkload("default", "job-abc")
	snap = s.Snapshot()
	if len(snap.Workloads) != 0 {
		t.Fatalf("expected 0 workloads after delete, got %d", len(snap.Workloads))
	}
}

func TestUpsertDeleteMultiKueueCluster(t *testing.T) {
	s := NewStore()

	c := MultiKueueClusterSnapshot{Name: "worker-1", Active: true}
	s.UpsertMultiKueueCluster(c)

	snap := s.Snapshot()
	got, ok := snap.MultiKueueClusters["worker-1"]
	if !ok {
		t.Fatal("worker-1 not found")
	}
	if !got.Active {
		t.Error("expected Active=true")
	}

	s.DeleteMultiKueueCluster("worker-1")
	snap = s.Snapshot()
	if len(snap.MultiKueueClusters) != 0 {
		t.Fatalf("expected 0 clusters after delete, got %d", len(snap.MultiKueueClusters))
	}
}

// --- deep copy ---------------------------------------------------------------

// Mutating the original after upsert must not affect a snapshot already taken.
func TestSnapshotIsDeepCopy(t *testing.T) {
	s := NewStore()
	q := makeQueue("team-a", "gpu-pool", 3)
	s.UpsertQueue(q)

	snap := s.Snapshot()

	// Mutate the original flavor resources directly in the store
	q2 := makeQueue("team-a", "gpu-pool", 99)
	s.UpsertQueue(q2)

	// Existing snapshot should be unaffected
	if snap.Queues["team-a"].Pending != 3 {
		t.Errorf("snapshot was mutated: pending should still be 3, got %d", snap.Queues["team-a"].Pending)
	}
}

// Mutating a snapshot's Quantity must not affect a subsequent snapshot.
func TestSnapshotQuantityIsolation(t *testing.T) {
	s := NewStore()
	w := makeWorkload("default", "job-1", "team-a")
	s.UpsertWorkload(w)

	snap1 := s.Snapshot()
	// Modify the quantity in snap1 in-place
	q := snap1.Workloads["default/job-1"].Resources[corev1.ResourceCPU]
	q.Add(resource.MustParse("100"))
	snap1.Workloads["default/job-1"].Resources[corev1.ResourceCPU] = q

	snap2 := s.Snapshot()
	cpu := snap2.Workloads["default/job-1"].Resources[corev1.ResourceCPU]
	if cpu.Cmp(resource.MustParse("2")) != 0 {
		t.Errorf("second snapshot CPU was affected by mutation of first snapshot: got %s", cpu.String())
	}
}

// --- ring buffer -------------------------------------------------------------

func TestEventRingBufferOrdering(t *testing.T) {
	s := NewStore()

	for i := range 10 {
		s.AppendEvent(EventEntry{Message: fmt.Sprintf("event-%d", i)})
	}

	snap := s.Snapshot()
	if len(snap.Events) != 10 {
		t.Fatalf("expected 10 events, got %d", len(snap.Events))
	}
	for i, e := range snap.Events {
		want := fmt.Sprintf("event-%d", i)
		if e.Message != want {
			t.Errorf("events[%d]: want %q, got %q", i, want, e.Message)
		}
	}
}

func TestEventRingBufferOverflow(t *testing.T) {
	s := NewStore()

	// Fill beyond capacity
	for i := range eventBufCap + 50 {
		s.AppendEvent(EventEntry{Message: fmt.Sprintf("event-%d", i)})
	}

	snap := s.Snapshot()
	if len(snap.Events) != eventBufCap {
		t.Fatalf("expected %d events (capped), got %d", eventBufCap, len(snap.Events))
	}

	// Oldest event should be event-50 (the first 50 were evicted)
	wantFirst := "event-50"
	if snap.Events[0].Message != wantFirst {
		t.Errorf("oldest event: want %q, got %q", wantFirst, snap.Events[0].Message)
	}

	// Newest event should be event-(cap+49)
	wantLast := fmt.Sprintf("event-%d", eventBufCap+49)
	if snap.Events[eventBufCap-1].Message != wantLast {
		t.Errorf("newest event: want %q, got %q", wantLast, snap.Events[eventBufCap-1].Message)
	}
}

// --- channel signaling -------------------------------------------------------

func TestUpdateChSignaledOnMutation(t *testing.T) {
	s := NewStore()

	ops := []func(){
		func() { s.UpsertQueue(makeQueue("q", "c", 0)) },
		func() { s.DeleteQueue("q") },
		func() { s.UpsertWorkload(makeWorkload("ns", "w", "q")) },
		func() { s.DeleteWorkload("ns", "w") },
		func() { s.UpsertMultiKueueCluster(MultiKueueClusterSnapshot{Name: "c"}) },
		func() { s.DeleteMultiKueueCluster("c") },
		func() { s.AppendEvent(EventEntry{Message: "e"}) },
	}

	for i, op := range ops {
		// Drain any prior signal
		select {
		case <-s.UpdateCh():
		default:
		}
		op()
		select {
		case <-s.UpdateCh():
			// expected
		case <-time.After(100 * time.Millisecond):
			t.Errorf("op %d: updateCh not signaled", i)
		}
	}
}

// Multiple rapid mutations should coalesce to at most one pending signal.
func TestUpdateChCoalescesRapidMutations(t *testing.T) {
	s := NewStore()

	for i := range 100 {
		s.UpsertQueue(makeQueue(fmt.Sprintf("q-%d", i), "c", 0))
	}

	count := 0
	for {
		select {
		case <-s.UpdateCh():
			count++
		default:
			goto done
		}
	}
done:
	if count > 1 {
		t.Errorf("expected at most 1 pending signal, got %d", count)
	}
	if count == 0 {
		t.Error("expected at least 1 pending signal")
	}
}

// --- concurrent safety -------------------------------------------------------

func TestConcurrentReadWrite(t *testing.T) {
	s := NewStore()

	const goroutines = 20
	const iterations = 200

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := range goroutines {
		go func(id int) {
			defer wg.Done()
			for i := range iterations {
				name := fmt.Sprintf("queue-%d", id)
				s.UpsertQueue(makeQueue(name, "cohort", int32(i))) //nolint:gosec
				_ = s.Snapshot()
				s.AppendEvent(EventEntry{Message: fmt.Sprintf("evt-%d-%d", id, i)})
				if i%10 == 0 {
					s.DeleteQueue(name)
				}
			}
		}(g)
	}

	wg.Wait()
}
