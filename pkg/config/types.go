package config

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Topology represents a complete kueue-bench test environment configuration
type Topology struct {
	APIVersion string       `yaml:"apiVersion"`
	Kind       string       `yaml:"kind"`
	Metadata   Metadata     `yaml:"metadata"`
	Spec       TopologySpec `yaml:"spec"`
}

// Metadata contains topology metadata
type Metadata struct {
	Name        string            `yaml:"name"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

// TopologySpec defines the desired topology configuration
type TopologySpec struct {
	Kueue      *KueueSettings  `yaml:"kueue,omitempty"`
	Kwok       *KwokSettings   `yaml:"kwok,omitempty"`
	Clusters   []ClusterConfig `yaml:"clusters"`
	WorkerSets []WorkerSet     `yaml:"workerSets,omitempty"`
}

// KueueSettings contains Kueue version and image settings
type KueueSettings struct {
	Version         string `yaml:"version,omitempty"`
	ImageRepository string `yaml:"imageRepository,omitempty"`
	ImageTag        string `yaml:"imageTag,omitempty"`
}

// KwokSettings contains Kwok version settings
type KwokSettings struct {
	Version string `yaml:"version,omitempty"`
}

// ClusterConfig defines a single cluster configuration
type ClusterConfig struct {
	Name              string       `yaml:"name"`
	Role              string       `yaml:"role"` // standalone, management, worker
	KubernetesVersion string       `yaml:"kubernetesVersion,omitempty"`
	NodePools         []NodePool   `yaml:"nodePools"`
	Kueue             *KueueConfig `yaml:"kueue,omitempty"`
}

// NodePool defines a pool of simulated nodes
type NodePool struct {
	Name      string            `yaml:"name"`
	Count     int               `yaml:"count"`
	Resources map[string]string `yaml:"resources"`
	Labels    map[string]string `yaml:"labels,omitempty"`
	Taints    []Taint           `yaml:"taints,omitempty"`
}

// Taint represents a Kubernetes node taint
type Taint struct {
	Key    string `yaml:"key"`
	Value  string `yaml:"value,omitempty"`
	Effect string `yaml:"effect"` // NoSchedule, PreferNoSchedule, NoExecute
}

// KueueConfig defines Kueue objects for a cluster
type KueueConfig struct {
	Cohorts         []Cohort                `yaml:"cohorts,omitempty"`
	ResourceFlavors []ResourceFlavor        `yaml:"resourceFlavors,omitempty"`
	ClusterQueues   []ClusterQueue          `yaml:"clusterQueues,omitempty"`
	LocalQueues     []LocalQueue            `yaml:"localQueues,omitempty"`
	PriorityClasses []WorkloadPriorityClass `yaml:"priorityClasses,omitempty"`
}

// Cohort represents a Kueue Cohort for hierarchical cohorts
type Cohort struct {
	Name           string          `yaml:"name"`
	ParentName     string          `yaml:"parentName,omitempty"`
	ResourceGroups []ResourceGroup `yaml:"resourceGroups,omitempty"`
	FairSharing    *FairSharing    `yaml:"fairSharing,omitempty"`
}

// FairSharing defines fair sharing configuration for cohorts and cluster queues
type FairSharing struct {
	Weight int32 `yaml:"weight,omitempty"`
}

// ResourceFlavor represents a Kueue ResourceFlavor
type ResourceFlavor struct {
	Name        string              `yaml:"name"`
	NodeLabels  map[string]string   `yaml:"nodeLabels,omitempty"`
	Tolerations []corev1.Toleration `yaml:"tolerations,omitempty"`
}

// ClusterQueue represents a Kueue ClusterQueue
type ClusterQueue struct {
	Name              string            `yaml:"name"`
	Cohort            string            `yaml:"cohort,omitempty"`
	NamespaceSelector *LabelSelector    `yaml:"namespaceSelector,omitempty"`
	Preemption        *PreemptionConfig `yaml:"preemption,omitempty"`
	ResourceGroups    []ResourceGroup   `yaml:"resourceGroups"`
	AdmissionChecks   []string          `yaml:"admissionChecks,omitempty"`
	FairSharing       *FairSharing      `yaml:"fairSharing,omitempty"`
}

// LabelSelector is a simplified label selector (supports matchLabels only for v1alpha1)
type LabelSelector struct {
	MatchLabels map[string]string `yaml:"matchLabels,omitempty"`
}

// PreemptionConfig defines preemption policies
type PreemptionConfig struct {
	WithinClusterQueue  string           `yaml:"withinClusterQueue,omitempty"`
	ReclaimWithinCohort string           `yaml:"reclaimWithinCohort,omitempty"`
	BorrowWithinCohort  *BorrowingConfig `yaml:"borrowWithinCohort,omitempty"`
}

// BorrowingConfig defines borrowing policies
type BorrowingConfig struct {
	Policy               string `yaml:"policy,omitempty"`
	MaxPriorityThreshold *int32 `yaml:"maxPriorityThreshold,omitempty"`
}

// ResourceGroup defines a group of resources with flavors
type ResourceGroup struct {
	CoveredResources []string       `yaml:"coveredResources"`
	Flavors          []FlavorQuotas `yaml:"flavors"`
}

// FlavorQuotas defines quotas for a specific flavor
type FlavorQuotas struct {
	Name      string     `yaml:"name"`
	Resources []Resource `yaml:"resources"`
}

// Resource defines quota for a single resource
type Resource struct {
	Name           string `yaml:"name"`
	NominalQuota   string `yaml:"nominalQuota"`
	BorrowingLimit string `yaml:"borrowingLimit,omitempty"`
	LendingLimit   string `yaml:"lendingLimit,omitempty"`
}

// LocalQueue represents a Kueue LocalQueue
type LocalQueue struct {
	Name         string `yaml:"name"`
	Namespace    string `yaml:"namespace"`
	ClusterQueue string `yaml:"clusterQueue"`
}

// WorkloadPriorityClass represents a Kueue WorkloadPriorityClass
type WorkloadPriorityClass struct {
	Name        string `yaml:"name"`
	Value       int32  `yaml:"value"`
	Description string `yaml:"description,omitempty"`
}

// WorkerSet defines a group of homogeneous workers for MultiKueue.
// All workers share identical Kueue object structure (names, relationships);
// values (labels, quotas) are derived from each worker's node pools.
type WorkerSet struct {
	Name            string                  `yaml:"name"`
	ResourceFlavors []WorkerSetFlavor       `yaml:"resourceFlavors"`
	ClusterQueues   []WorkerSetClusterQueue `yaml:"clusterQueues"`
	LocalQueues     []LocalQueue            `yaml:"localQueues,omitempty"`
	Workers         []Worker                `yaml:"workers"`
}

// WorkerSetFlavor maps a flavor to a node pool. At expansion time, the flavor's
// nodeLabels and tolerations are derived from the referenced pool in each worker.
type WorkerSetFlavor struct {
	Name        string `yaml:"name"`
	NodePoolRef string `yaml:"nodePoolRef"`
}

// WorkerSetClusterQueue defines ClusterQueue structure at the WorkerSet level.
// Quotas for each flavor are derived from the node pool that flavor references.
type WorkerSetClusterQueue struct {
	Name              string                   `yaml:"name"`
	Cohort            string                   `yaml:"cohort,omitempty"`
	NamespaceSelector *LabelSelector           `yaml:"namespaceSelector,omitempty"`
	Preemption        *PreemptionConfig        `yaml:"preemption,omitempty"`
	ResourceGroups    []WorkerSetResourceGroup `yaml:"resourceGroups"`
	AdmissionChecks   []string                 `yaml:"admissionChecks,omitempty"`
	FairSharing       *FairSharing             `yaml:"fairSharing,omitempty"`
}

// WorkerSetResourceGroup groups covered resources and the flavors that provide them.
type WorkerSetResourceGroup struct {
	CoveredResources []string             `yaml:"coveredResources"`
	Flavors          []WorkerSetFlavorRef `yaml:"flavors"`
}

// WorkerSetFlavorRef references a flavor by name. During expansion, nominalQuota
// for each coveredResource is calculated as pool.count * pool.resources[resource].
type WorkerSetFlavorRef struct {
	Name string `yaml:"name"`
}

// Worker defines the per-worker infrastructure within a WorkerSet.
// Each Worker becomes a ClusterConfig after expansion.
type Worker struct {
	Name      string     `yaml:"name"`
	NodePools []NodePool `yaml:"nodePools"`
}

// TopologyMetadata stores runtime information about a created topology
type TopologyMetadata struct {
	Name      string      `json:"name"`
	Clusters  []string    `json:"clusters"`
	CreatedAt metav1.Time `json:"createdAt"`
	FilePath  string      `json:"filePath"`
}
