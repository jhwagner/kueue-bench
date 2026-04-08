package watcher

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	kueuev1beta2 "sigs.k8s.io/kueue/apis/kueue/v1beta2"
	kueueclientset "sigs.k8s.io/kueue/client-go/clientset/versioned"
	"sigs.k8s.io/kueue/client-go/informers/externalversions"
)

// Watcher manages informer lifecycle for a single Kubernetes cluster and feeds
// the Store. It is not safe to call Start more than once.
type Watcher struct {
	store        *Store
	kueueFactory externalversions.SharedInformerFactory
	coreFactory  coreinformers.SharedInformerFactory
	isManagement bool
	connected    atomic.Bool
	stopCh       chan struct{}
}

// New builds a Watcher connected to the cluster at kubeconfigPath. It does not
// start the informers — call Start to do that.
func New(kubeconfigPath string, isManagement bool) (*Watcher, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}

	kueueClient, err := kueueclientset.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build kueue clientset: %w", err)
	}
	k8sClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build k8s clientset: %w", err)
	}

	return &Watcher{
		store:        NewStore(),
		kueueFactory: externalversions.NewSharedInformerFactory(kueueClient, 0),
		coreFactory:  coreinformers.NewSharedInformerFactory(k8sClient, 0),
		isManagement: isManagement,
		stopCh:       make(chan struct{}),
	}, nil
}

// Start registers informer event handlers, starts the informers, and blocks
// until the caches are synced or ctx is cancelled. ctx controls the watcher
// lifetime: cancelling it (via signal, 'q', etc.) will stop all informers and
// release connections. Returns an error if ctx is cancelled before sync completes.
func (w *Watcher) Start(ctx context.Context) error {
	if err := w.registerClusterQueueHandlers(); err != nil {
		return fmt.Errorf("register ClusterQueue handlers: %w", err)
	}
	if err := w.registerWorkloadHandlers(); err != nil {
		return fmt.Errorf("register Workload handlers: %w", err)
	}
	if err := w.registerEventHandlers(); err != nil {
		return fmt.Errorf("register Event handlers: %w", err)
	}
	if w.isManagement {
		if err := w.registerMultiKueueClusterHandlers(); err != nil {
			return fmt.Errorf("register MultiKueueCluster handlers: %w", err)
		}
	}

	w.kueueFactory.Start(w.stopCh)
	w.coreFactory.Start(w.stopCh)

	// Bridge ctx cancellation to stopCh so WaitForCacheSync respects it.
	go func() {
		select {
		case <-ctx.Done():
			w.Stop()
		case <-w.stopCh:
		}
	}()

	synced := w.kueueFactory.WaitForCacheSync(w.stopCh)
	for _, ok := range synced {
		if !ok {
			if err := ctx.Err(); err != nil {
				return err
			}
			return fmt.Errorf("kueue cache sync failed")
		}
	}
	coreSynced := w.coreFactory.WaitForCacheSync(w.stopCh)
	for _, ok := range coreSynced {
		if !ok {
			if err := ctx.Err(); err != nil {
				return err
			}
			return fmt.Errorf("core cache sync failed")
		}
	}

	w.connected.Store(true)
	return nil
}

// Stop stops all informers. Safe to call multiple times.
func (w *Watcher) Stop() {
	select {
	case <-w.stopCh:
		// already closed
	default:
		close(w.stopCh)
	}
	w.kueueFactory.Shutdown()
	w.coreFactory.Shutdown()
	w.connected.Store(false)
}

// Store returns the snapshot store that is updated by the informers.
func (w *Watcher) Store() *Store {
	return w.store
}

// IsConnected reports whether the watcher has successfully completed its
// initial cache sync. It does not reflect mid-session connectivity.
func (w *Watcher) IsConnected() bool {
	return w.connected.Load()
}

// --- ClusterQueue informer ---------------------------------------------------

func (w *Watcher) registerClusterQueueHandlers() error {
	informer := w.kueueFactory.Kueue().V1beta2().ClusterQueues().Informer()
	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { w.upsertQueue(obj) },
		UpdateFunc: func(_, newObj interface{}) { w.upsertQueue(newObj) },
		DeleteFunc: func(obj interface{}) {
			if cq, ok := extractObj[kueuev1beta2.ClusterQueue](obj); ok {
				w.store.DeleteQueue(cq.Name)
			}
		},
	})
	return err
}

func (w *Watcher) upsertQueue(obj interface{}) {
	cq, ok := obj.(*kueuev1beta2.ClusterQueue)
	if !ok {
		return
	}
	w.store.UpsertQueue(buildQueueSnapshot(cq))
}

// extractObj handles both direct objects and tombstone-wrapped objects from
// delete events where the informer's local cache had already expired the entry.
func extractObj[T any](obj any) (*T, bool) {
	if v, ok := obj.(*T); ok {
		return v, true
	}
	if ts, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		v, ok := ts.Obj.(*T)
		return v, ok
	}
	return nil, false
}

func buildQueueSnapshot(cq *kueuev1beta2.ClusterQueue) QueueSnapshot {
	snap := QueueSnapshot{
		Name:      cq.Name,
		Cohort:    string(cq.Spec.CohortName),
		Pending:   cq.Status.PendingWorkloads,
		Reserving: cq.Status.ReservingWorkloads,
		Admitted:  cq.Status.AdmittedWorkloads,
	}

	for _, c := range cq.Status.Conditions {
		if c.Type == kueuev1beta2.ClusterQueueActive && c.Status == metav1.ConditionTrue {
			snap.Active = true
			break
		}
	}

	// Build per-flavor resource map, combining spec (nominal) and status (usage).
	type flavorResources = map[corev1.ResourceName]ResourceSnapshot
	flavorMap := make(map[string]flavorResources)

	ensureFlavor := func(name string) flavorResources {
		if flavorMap[name] == nil {
			flavorMap[name] = make(flavorResources)
		}
		return flavorMap[name]
	}

	for _, rg := range cq.Spec.ResourceGroups {
		for _, fq := range rg.Flavors {
			fm := ensureFlavor(string(fq.Name))
			for _, rq := range fq.Resources {
				rs := fm[rq.Name]
				rs.Nominal = rq.NominalQuota.DeepCopy()
				if rq.BorrowingLimit != nil {
					q := rq.BorrowingLimit.DeepCopy()
					rs.BorrowingLimit = &q
				}
				if rq.LendingLimit != nil {
					q := rq.LendingLimit.DeepCopy()
					rs.LendingLimit = &q
				}
				fm[rq.Name] = rs
			}
		}
	}

	if cq.Spec.Preemption != nil {
		snap.Preemption.ReclaimWithinCohort = string(cq.Spec.Preemption.ReclaimWithinCohort)
		snap.Preemption.WithinClusterQueue = string(cq.Spec.Preemption.WithinClusterQueue)
		if cq.Spec.Preemption.BorrowWithinCohort != nil {
			snap.Preemption.BorrowWithinCohort = string(cq.Spec.Preemption.BorrowWithinCohort.Policy)
		}
	}

	if cq.Spec.FairSharing != nil && cq.Spec.FairSharing.Weight != nil {
		w := cq.Spec.FairSharing.Weight.DeepCopy()
		snap.FairSharingWeight = &w
	}

	for _, fu := range cq.Status.FlavorsReservation {
		fm := ensureFlavor(string(fu.Name))
		for _, ru := range fu.Resources {
			rs := fm[ru.Name]
			rs.Reserved = ru.Total.DeepCopy()
			fm[ru.Name] = rs
		}
	}

	for _, fu := range cq.Status.FlavorsUsage {
		fm := ensureFlavor(string(fu.Name))
		for _, ru := range fu.Resources {
			rs := fm[ru.Name]
			rs.Used = ru.Total.DeepCopy()
			rs.Borrowed = ru.Borrowed.DeepCopy()
			fm[ru.Name] = rs
		}
	}

	// Emit flavors in spec order, then any status-only flavors.
	seen := make(map[string]bool)
	appendFlavor := func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		fs := FlavorSnapshot{
			Name:      name,
			Resources: make(map[corev1.ResourceName]ResourceSnapshot, len(flavorMap[name])),
		}
		for rName, rs := range flavorMap[name] {
			fs.Resources[rName] = rs
		}
		snap.Flavors = append(snap.Flavors, fs)
	}

	for _, rg := range cq.Spec.ResourceGroups {
		for _, fq := range rg.Flavors {
			appendFlavor(string(fq.Name))
		}
	}
	for name := range flavorMap {
		appendFlavor(name)
	}

	return snap
}

// --- Workload informer -------------------------------------------------------

func (w *Watcher) registerWorkloadHandlers() error {
	informer := w.kueueFactory.Kueue().V1beta2().Workloads().Informer()
	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { w.upsertWorkload(obj) },
		UpdateFunc: func(_, newObj interface{}) { w.upsertWorkload(newObj) },
		DeleteFunc: func(obj interface{}) {
			if wl, ok := extractObj[kueuev1beta2.Workload](obj); ok {
				w.store.DeleteWorkload(wl.Namespace, wl.Name)
			}
		},
	})
	return err
}

func (w *Watcher) upsertWorkload(obj interface{}) {
	wl, ok := obj.(*kueuev1beta2.Workload)
	if !ok {
		return
	}
	w.store.UpsertWorkload(buildWorkloadSnapshot(wl))
}

func buildWorkloadSnapshot(wl *kueuev1beta2.Workload) WorkloadSnapshot {
	var ownerKind string
	for _, ref := range wl.OwnerReferences {
		if ref.Controller != nil && *ref.Controller {
			ownerKind = ref.Kind
			break
		}
	}

	snap := WorkloadSnapshot{
		Name:      wl.Name,
		Namespace: wl.Namespace,
		OwnerKind: ownerKind,
		Queue:     string(wl.Spec.QueueName),
		Status:    deriveWorkloadStatus(wl.Status.Conditions),
		CreatedAt: wl.CreationTimestamp.Time,
		Resources: aggregatePodSetResources(wl.Spec.PodSets),
	}

	if wl.Status.Admission != nil {
		snap.ClusterQueue = string(wl.Status.Admission.ClusterQueue)
	}

	if wl.Status.ClusterName != nil {
		snap.DispatchedTo = *wl.Status.ClusterName
	}

	return snap
}

// deriveWorkloadStatus applies condition precedence per the plan:
// Finished > Evicted > Admitted > QuotaReserved > Pending
func deriveWorkloadStatus(conditions []metav1.Condition) WorkloadStatus {
	condTrue := func(condType string) bool {
		for _, c := range conditions {
			if c.Type == condType && c.Status == metav1.ConditionTrue {
				return true
			}
		}
		return false
	}

	switch {
	case condTrue(kueuev1beta2.WorkloadFinished):
		return WorkloadStatusFinished
	case condTrue(kueuev1beta2.WorkloadEvicted):
		return WorkloadStatusEvicted
	case condTrue(kueuev1beta2.WorkloadAdmitted):
		return WorkloadStatusAdmitted
	case condTrue(kueuev1beta2.WorkloadQuotaReserved):
		return WorkloadStatusQuotaReserved
	default:
		return WorkloadStatusPending
	}
}

// aggregatePodSetResources sums resource requests across all pod sets:
// total = sum over pod sets of (container requests × pod count).
func aggregatePodSetResources(podSets []kueuev1beta2.PodSet) map[corev1.ResourceName]resource.Quantity {
	totals := make(map[corev1.ResourceName]resource.Quantity)

	for _, ps := range podSets {
		count := int64(ps.Count)
		if count <= 0 {
			count = 1
		}

		for _, c := range ps.Template.Spec.Containers {
			for rName, rQty := range c.Resources.Requests {
				q := rQty.DeepCopy()
				q.Mul(count)
				if existing, ok := totals[rName]; ok {
					existing.Add(q)
					totals[rName] = existing
				} else {
					totals[rName] = q
				}
			}
		}
	}

	return totals
}

// --- Event informer ----------------------------------------------------------

// kueueEventReasons is the set of event reasons considered Kueue-relevant when
// the involved object is not already identifiable as a Kueue resource.
var kueueEventReasons = map[string]bool{
	"Admitted":      true,
	"Evicted":       true,
	"Preempted":     true,
	"QuotaReserved": true,
	"Borrowed":      true,
}

func (w *Watcher) registerEventHandlers() error {
	informer := w.coreFactory.Core().V1().Events().Informer()
	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) { w.handleEvent(obj) },
		// Kubernetes updates events in place when they repeat; treat as new entry.
		UpdateFunc: func(_, newObj interface{}) { w.handleEvent(newObj) },
	})
	return err
}

func (w *Watcher) handleEvent(obj interface{}) {
	ev, ok := obj.(*corev1.Event)
	if !ok {
		return
	}
	if !isKueueRelevant(ev) {
		return
	}

	t := ev.LastTimestamp.Time
	if t.IsZero() {
		t = ev.EventTime.Time
	}
	if t.IsZero() {
		t = time.Now()
	}

	w.store.AppendEvent(EventEntry{
		Time:    t,
		Type:    ev.Type,
		Reason:  ev.Reason,
		Object:  ev.InvolvedObject.Kind + "/" + ev.InvolvedObject.Name,
		Message: ev.Message,
	})
}

func isKueueRelevant(ev *corev1.Event) bool {
	if strings.Contains(ev.InvolvedObject.APIVersion, "kueue") {
		return true
	}
	return kueueEventReasons[ev.Reason]
}

// --- MultiKueueCluster informer (management only) ----------------------------

func (w *Watcher) registerMultiKueueClusterHandlers() error {
	informer := w.kueueFactory.Kueue().V1beta2().MultiKueueClusters().Informer()
	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { w.upsertMultiKueueCluster(obj) },
		UpdateFunc: func(_, newObj interface{}) { w.upsertMultiKueueCluster(newObj) },
		DeleteFunc: func(obj interface{}) {
			if mkc, ok := extractObj[kueuev1beta2.MultiKueueCluster](obj); ok {
				w.store.DeleteMultiKueueCluster(mkc.Name)
			}
		},
	})
	return err
}

func (w *Watcher) upsertMultiKueueCluster(obj interface{}) {
	mkc, ok := obj.(*kueuev1beta2.MultiKueueCluster)
	if !ok {
		return
	}
	w.store.UpsertMultiKueueCluster(buildMultiKueueClusterSnapshot(mkc))
}

func buildMultiKueueClusterSnapshot(mkc *kueuev1beta2.MultiKueueCluster) MultiKueueClusterSnapshot {
	snap := MultiKueueClusterSnapshot{
		Name:       mkc.Name,
		Conditions: make([]metav1.Condition, len(mkc.Status.Conditions)),
	}
	copy(snap.Conditions, mkc.Status.Conditions)

	for _, c := range mkc.Status.Conditions {
		if c.Type == kueuev1beta2.MultiKueueClusterActive && c.Status == metav1.ConditionTrue {
			snap.Active = true
			break
		}
	}

	return snap
}
