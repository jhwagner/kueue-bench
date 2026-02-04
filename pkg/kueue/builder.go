package kueue

import (
	"github.com/jhwagner/kueue-bench/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kueue "sigs.k8s.io/kueue/apis/kueue/v1beta1"
)

// BuildCohort builds a Kueue Cohort from a config Cohort
func BuildCohort(c config.Cohort) *kueue.Cohort {
	spec := kueue.CohortSpec{}

	// Set parent name if present
	if c.ParentName != "" {
		spec.ParentName = kueue.CohortReference(c.ParentName)
	}

	// Build resource groups if present
	if len(c.ResourceGroups) > 0 {
		spec.ResourceGroups = buildResourceGroups(c.ResourceGroups)
	}

	// Build fair sharing if present
	if c.FairSharing != nil {
		spec.FairSharing = buildFairSharing(c.FairSharing)
	}

	return &kueue.Cohort{
		TypeMeta:   metav1.TypeMeta{APIVersion: kueue.SchemeGroupVersion.String(), Kind: "Cohort"},
		ObjectMeta: metav1.ObjectMeta{Name: c.Name},
		Spec:       spec,
	}
}

// buildFairSharing builds FairSharing from config
func buildFairSharing(fs *config.FairSharing) *kueue.FairSharing {
	// Convert int32 weight to resource.Quantity
	weight := resource.NewQuantity(int64(fs.Weight), resource.DecimalSI)
	return &kueue.FairSharing{
		Weight: weight,
	}
}

// BuildResourceFlavor builds a Kueue ResourceFlavor from a config ResourceFlavor
func BuildResourceFlavor(rf config.ResourceFlavor) *kueue.ResourceFlavor {
	return &kueue.ResourceFlavor{
		TypeMeta:   metav1.TypeMeta{APIVersion: kueue.SchemeGroupVersion.String(), Kind: "ResourceFlavor"},
		ObjectMeta: metav1.ObjectMeta{Name: rf.Name},
		Spec: kueue.ResourceFlavorSpec{
			NodeLabels:  rf.NodeLabels,
			Tolerations: rf.Tolerations,
		},
	}
}

// BuildClusterQueue builds a Kueue ClusterQueue from a config ClusterQueue
func BuildClusterQueue(cq config.ClusterQueue) *kueue.ClusterQueue {
	kueueCQ := &kueue.ClusterQueue{
		TypeMeta:   metav1.TypeMeta{APIVersion: kueue.SchemeGroupVersion.String(), Kind: "ClusterQueue"},
		ObjectMeta: metav1.ObjectMeta{Name: cq.Name},
		Spec: kueue.ClusterQueueSpec{
			Cohort:         kueue.CohortReference(cq.Cohort),
			ResourceGroups: buildResourceGroups(cq.ResourceGroups),
		},
	}

	// Build namespace selector if present (empty {} means all namespaces)
	if cq.NamespaceSelector != nil {
		kueueCQ.Spec.NamespaceSelector = &metav1.LabelSelector{
			MatchLabels: cq.NamespaceSelector.MatchLabels,
		}
	}

	// Build preemption config if present
	if cq.Preemption != nil {
		kueueCQ.Spec.Preemption = buildPreemptionConfig(cq.Preemption)
	}

	// Build admission checks if present
	if len(cq.AdmissionChecks) > 0 {
		kueueCQ.Spec.AdmissionChecks = make([]kueue.AdmissionCheckReference, len(cq.AdmissionChecks))
		for i, ac := range cq.AdmissionChecks {
			kueueCQ.Spec.AdmissionChecks[i] = kueue.AdmissionCheckReference(ac)
		}
	}

	// Build fair sharing if present
	if cq.FairSharing != nil {
		kueueCQ.Spec.FairSharing = buildFairSharing(cq.FairSharing)
	}

	return kueueCQ
}

// buildPreemptionConfig builds ClusterQueuePreemption from config
func buildPreemptionConfig(pc *config.PreemptionConfig) *kueue.ClusterQueuePreemption {
	preemption := &kueue.ClusterQueuePreemption{}

	// Build WithinClusterQueue
	if pc.WithinClusterQueue != "" {
		preemption.WithinClusterQueue = kueue.PreemptionPolicy(pc.WithinClusterQueue)
	}

	// Build ReclaimWithinCohort
	if pc.ReclaimWithinCohort != "" {
		preemption.ReclaimWithinCohort = kueue.PreemptionPolicy(pc.ReclaimWithinCohort)
	}

	// Build BorrowWithinCohort
	if pc.BorrowWithinCohort != nil {
		borrowing := &kueue.BorrowWithinCohort{}
		if pc.BorrowWithinCohort.Policy != "" {
			borrowing.Policy = kueue.BorrowWithinCohortPolicy(pc.BorrowWithinCohort.Policy)
		}
		if pc.BorrowWithinCohort.MaxPriorityThreshold != nil {
			borrowing.MaxPriorityThreshold = pc.BorrowWithinCohort.MaxPriorityThreshold
		}
		preemption.BorrowWithinCohort = borrowing
	}

	return preemption
}

// buildResourceGroups builds ResourceGroups from config
func buildResourceGroups(groups []config.ResourceGroup) []kueue.ResourceGroup {
	result := make([]kueue.ResourceGroup, len(groups))
	for i, group := range groups {
		result[i] = kueue.ResourceGroup{
			CoveredResources: buildCoveredResources(group.CoveredResources),
			Flavors:          buildFlavors(group.Flavors),
		}
	}
	return result
}

// buildCoveredResources builds ResourceName slice from covered resources
func buildCoveredResources(resources []string) []corev1.ResourceName {
	result := make([]corev1.ResourceName, len(resources))
	for i, r := range resources {
		result[i] = corev1.ResourceName(r)
	}
	return result
}

// buildFlavors builds flavor quotas
func buildFlavors(flavors []config.FlavorQuotas) []kueue.FlavorQuotas {
	result := make([]kueue.FlavorQuotas, len(flavors))
	for i, flavor := range flavors {
		result[i] = kueue.FlavorQuotas{
			Name:      kueue.ResourceFlavorReference(flavor.Name),
			Resources: buildResources(flavor.Resources),
		}
	}
	return result
}

// buildResources builds resource quotas
func buildResources(resources []config.Resource) []kueue.ResourceQuota {
	result := make([]kueue.ResourceQuota, len(resources))
	for i, res := range resources {
		quota := kueue.ResourceQuota{
			Name:         corev1.ResourceName(res.Name),
			NominalQuota: resource.MustParse(res.NominalQuota),
		}

		// Build optional borrowing limit
		if res.BorrowingLimit != "" {
			borrowingLimit := resource.MustParse(res.BorrowingLimit)
			quota.BorrowingLimit = &borrowingLimit
		}

		// Build optional lending limit
		if res.LendingLimit != "" {
			lendingLimit := resource.MustParse(res.LendingLimit)
			quota.LendingLimit = &lendingLimit
		}

		result[i] = quota
	}
	return result
}

// BuildLocalQueue builds a Kueue LocalQueue from a config LocalQueue
func BuildLocalQueue(lq config.LocalQueue) *kueue.LocalQueue {
	namespace := lq.Namespace
	if namespace == "" {
		namespace = "default"
	}

	return &kueue.LocalQueue{
		TypeMeta:   metav1.TypeMeta{APIVersion: kueue.SchemeGroupVersion.String(), Kind: "LocalQueue"},
		ObjectMeta: metav1.ObjectMeta{Name: lq.Name, Namespace: namespace},
		Spec: kueue.LocalQueueSpec{
			ClusterQueue: kueue.ClusterQueueReference(lq.ClusterQueue),
		},
	}
}

// BuildWorkloadPriorityClass builds a Kueue WorkloadPriorityClass from a config WorkloadPriorityClass
func BuildWorkloadPriorityClass(wpc config.WorkloadPriorityClass) *kueue.WorkloadPriorityClass {
	return &kueue.WorkloadPriorityClass{
		TypeMeta:    metav1.TypeMeta{APIVersion: kueue.SchemeGroupVersion.String(), Kind: "WorkloadPriorityClass"},
		ObjectMeta:  metav1.ObjectMeta{Name: wpc.Name},
		Value:       wpc.Value,
		Description: wpc.Description,
	}
}

// BuildMultiKueueCluster builds a Kueue MultiKueueCluster
func BuildMultiKueueCluster(name, kubeconfigSecretName string) *kueue.MultiKueueCluster {
	return &kueue.MultiKueueCluster{
		TypeMeta:   metav1.TypeMeta{APIVersion: kueue.SchemeGroupVersion.String(), Kind: "MultiKueueCluster"},
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: kueue.MultiKueueClusterSpec{
			KubeConfig: kueue.KubeConfig{
				Location:     kubeconfigSecretName,
				LocationType: kueue.SecretLocationType,
			},
		},
	}
}

// BuildMultiKueueConfig builds a Kueue MultiKueueConfig
func BuildMultiKueueConfig(name string, clusterNames []string) *kueue.MultiKueueConfig {
	return &kueue.MultiKueueConfig{
		TypeMeta:   metav1.TypeMeta{APIVersion: kueue.SchemeGroupVersion.String(), Kind: "MultiKueueConfig"},
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: kueue.MultiKueueConfigSpec{
			Clusters: clusterNames,
		},
	}
}

// BuildAdmissionCheck builds a Kueue AdmissionCheck for MultiKueue
func BuildAdmissionCheck(name, configName string) *kueue.AdmissionCheck {
	return &kueue.AdmissionCheck{
		TypeMeta:   metav1.TypeMeta{APIVersion: kueue.SchemeGroupVersion.String(), Kind: "AdmissionCheck"},
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: kueue.AdmissionCheckSpec{
			ControllerName: kueue.MultiKueueControllerName,
			Parameters: &kueue.AdmissionCheckParametersReference{
				APIGroup: kueue.SchemeGroupVersion.Group,
				Kind:     "MultiKueueConfig",
				Name:     configName,
			},
		},
	}
}
