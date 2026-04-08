package watcher

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkloadStatus is the derived status of a workload based on its conditions.
type WorkloadStatus string

const (
	WorkloadStatusPending       WorkloadStatus = "Pending"
	WorkloadStatusQuotaReserved WorkloadStatus = "QuotaReserved"
	WorkloadStatusAdmitted      WorkloadStatus = "Admitted"
	WorkloadStatusFinished      WorkloadStatus = "Finished"
	WorkloadStatusEvicted       WorkloadStatus = "Evicted"
)

// ResourceSnapshot holds quota and usage for a single resource within a flavor.
type ResourceSnapshot struct {
	Nominal        resource.Quantity
	Used           resource.Quantity  // from status.flavorsUsage
	Reserved       resource.Quantity  // from status.flavorsReservation
	Borrowed       resource.Quantity  // from status.flavorsUsage[*].borrowed
	BorrowingLimit *resource.Quantity // from spec; nil means no limit
	LendingLimit   *resource.Quantity // from spec; nil means no limit
}

func (r ResourceSnapshot) deepCopy() ResourceSnapshot {
	dst := ResourceSnapshot{
		Nominal:  r.Nominal.DeepCopy(),
		Used:     r.Used.DeepCopy(),
		Reserved: r.Reserved.DeepCopy(),
		Borrowed: r.Borrowed.DeepCopy(),
	}
	if r.BorrowingLimit != nil {
		q := r.BorrowingLimit.DeepCopy()
		dst.BorrowingLimit = &q
	}
	if r.LendingLimit != nil {
		q := r.LendingLimit.DeepCopy()
		dst.LendingLimit = &q
	}
	return dst
}

// FlavorSnapshot holds resource usage for a single ResourceFlavor within a ClusterQueue.
type FlavorSnapshot struct {
	Name      string
	Resources map[corev1.ResourceName]ResourceSnapshot
}

func (f FlavorSnapshot) deepCopy() FlavorSnapshot {
	dst := FlavorSnapshot{
		Name:      f.Name,
		Resources: make(map[corev1.ResourceName]ResourceSnapshot, len(f.Resources)),
	}
	for k, v := range f.Resources {
		dst.Resources[k] = v.deepCopy()
	}
	return dst
}

// PreemptionSnapshot holds the preemption policy settings of a ClusterQueue.
// Empty string means the field was not set (defaults to "Never" in Kueue).
type PreemptionSnapshot struct {
	ReclaimWithinCohort string
	BorrowWithinCohort  string
	WithinClusterQueue  string
}

// QueueSnapshot is a point-in-time view of a ClusterQueue.
type QueueSnapshot struct {
	Name              string
	Cohort            string
	Pending           int32
	Reserving         int32
	Admitted          int32
	Active            bool // true when Active condition is True
	Flavors           []FlavorSnapshot
	Preemption        PreemptionSnapshot
	FairSharingWeight *resource.Quantity // nil if not set
}

func (q QueueSnapshot) deepCopy() QueueSnapshot {
	dst := q
	dst.Flavors = make([]FlavorSnapshot, len(q.Flavors))
	for i, f := range q.Flavors {
		dst.Flavors[i] = f.deepCopy()
	}
	if q.FairSharingWeight != nil {
		w := q.FairSharingWeight.DeepCopy()
		dst.FairSharingWeight = &w
	}
	return dst
}

// WorkloadSnapshot is a point-in-time view of a Kueue Workload.
type WorkloadSnapshot struct {
	Name         string
	Namespace    string
	OwnerKind    string         // owner reference Kind (e.g. "Job", "JobSet", "RayJob"); empty if none
	Queue        string         // spec.queueName (LocalQueue)
	ClusterQueue string         // status.admission.clusterQueue
	Status       WorkloadStatus // derived from conditions
	CreatedAt    time.Time
	// Resources is the aggregated resource requests across all pod sets
	// (requests-per-pod × replicas, summed across pod sets).
	Resources map[corev1.ResourceName]resource.Quantity
	// DispatchedTo is the MultiKueue worker cluster name; empty for non-MultiKueue workloads.
	DispatchedTo string
}

func (w WorkloadSnapshot) deepCopy() WorkloadSnapshot {
	dst := w
	dst.Resources = make(map[corev1.ResourceName]resource.Quantity, len(w.Resources))
	for k, v := range w.Resources {
		dst.Resources[k] = v.DeepCopy()
	}
	return dst
}

// EventEntry is a Kubernetes event captured for the TUI event log.
type EventEntry struct {
	Time    time.Time
	Type    string // corev1.EventTypeNormal or corev1.EventTypeWarning
	Reason  string
	Object  string // "Kind/name"
	Message string
}

// MultiKueueClusterSnapshot is a point-in-time view of a MultiKueueCluster.
type MultiKueueClusterSnapshot struct {
	Name       string
	Active     bool // true when Active condition is True
	Conditions []metav1.Condition
}

func (m MultiKueueClusterSnapshot) deepCopy() MultiKueueClusterSnapshot {
	dst := m
	dst.Conditions = make([]metav1.Condition, len(m.Conditions))
	copy(dst.Conditions, m.Conditions)
	return dst
}

// Snapshot is the aggregated cluster state at a point in time.
type Snapshot struct {
	Queues             map[string]QueueSnapshot             // key: ClusterQueue name
	Workloads          map[string]WorkloadSnapshot          // key: "namespace/name"
	MultiKueueClusters map[string]MultiKueueClusterSnapshot // key: cluster name
	Events             []EventEntry                         // ordered oldest → newest, capped at 500
}
