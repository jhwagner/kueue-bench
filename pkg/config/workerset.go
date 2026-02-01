package config

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// ExpandWorkerSets converts WorkerSets into explicit ClusterConfigs.
// Each worker in each WorkerSet becomes a ClusterConfig with Kueue objects
// whose values (labels, quotas) are derived from the worker's node pools.
func ExpandWorkerSets(workerSets []WorkerSet) ([]ClusterConfig, error) {
	var clusters []ClusterConfig

	for _, ws := range workerSets {
		// Build flavor name to nodePoolRef lookup
		flavorPools := make(map[string]string, len(ws.ResourceFlavors))
		for _, f := range ws.ResourceFlavors {
			flavorPools[f.Name] = f.NodePoolRef
		}

		for _, worker := range ws.Workers {
			cluster, err := expandWorker(ws, worker, flavorPools)
			if err != nil {
				return nil, fmt.Errorf("workerSet %s, worker %s: %w", ws.Name, worker.Name, err)
			}
			clusters = append(clusters, cluster)
		}
	}

	return clusters, nil
}

func expandWorker(ws WorkerSet, worker Worker, flavorPools map[string]string) (ClusterConfig, error) {
	pools := make(map[string]NodePool, len(worker.NodePools))
	for _, p := range worker.NodePools {
		pools[p.Name] = p
	}

	resourceFlavors, err := deriveResourceFlavors(ws.ResourceFlavors, pools)
	if err != nil {
		return ClusterConfig{}, err
	}

	clusterQueues, err := deriveClusterQueues(ws.ClusterQueues, flavorPools, pools)
	if err != nil {
		return ClusterConfig{}, err
	}

	return ClusterConfig{
		Name:      worker.Name,
		Role:      RoleWorker,
		NodePools: worker.NodePools,
		Kueue: &KueueConfig{
			ResourceFlavors: resourceFlavors,
			ClusterQueues:   clusterQueues,
			LocalQueues:     ws.LocalQueues,
		},
	}, nil
}

func deriveResourceFlavors(wsFlavorDefs []WorkerSetFlavor, pools map[string]NodePool) ([]ResourceFlavor, error) {
	flavors := make([]ResourceFlavor, 0, len(wsFlavorDefs))

	for _, f := range wsFlavorDefs {
		pool, ok := pools[f.NodePoolRef]
		if !ok {
			return nil, fmt.Errorf("nodePoolRef %q not found in worker node pools", f.NodePoolRef)
		}

		flavors = append(flavors, ResourceFlavor{
			Name:        f.Name,
			NodeLabels:  pool.Labels,
			Tolerations: taintsToTolerations(pool.Taints),
		})
	}

	return flavors, nil
}

func deriveClusterQueues(wsCQs []WorkerSetClusterQueue, flavorPools map[string]string, pools map[string]NodePool) ([]ClusterQueue, error) {
	cqs := make([]ClusterQueue, 0, len(wsCQs))

	for _, wsCQ := range wsCQs {
		rgs, err := deriveResourceGroups(wsCQ.ResourceGroups, flavorPools, pools)
		if err != nil {
			return nil, fmt.Errorf("clusterQueue %s: %w", wsCQ.Name, err)
		}

		cqs = append(cqs, ClusterQueue{
			Name:              wsCQ.Name,
			Cohort:            wsCQ.Cohort,
			NamespaceSelector: wsCQ.NamespaceSelector,
			Preemption:        wsCQ.Preemption,
			ResourceGroups:    rgs,
			AdmissionChecks:   wsCQ.AdmissionChecks,
			FairSharing:       wsCQ.FairSharing,
		})
	}

	return cqs, nil
}

func deriveResourceGroups(wsRGs []WorkerSetResourceGroup, flavorPools map[string]string, pools map[string]NodePool) ([]ResourceGroup, error) {
	rgs := make([]ResourceGroup, 0, len(wsRGs))

	for _, wsRG := range wsRGs {
		flavors := make([]FlavorQuotas, 0, len(wsRG.Flavors))

		for _, flavorRef := range wsRG.Flavors {
			poolName, ok := flavorPools[flavorRef.Name]
			if !ok {
				return nil, fmt.Errorf("flavor %q not defined in workerSet resourceFlavors", flavorRef.Name)
			}

			pool, ok := pools[poolName]
			if !ok {
				return nil, fmt.Errorf("nodePoolRef %q (from flavor %q) not found in worker node pools", poolName, flavorRef.Name)
			}

			resources, err := deriveQuotas(wsRG.CoveredResources, pool)
			if err != nil {
				return nil, err
			}

			flavors = append(flavors, FlavorQuotas{
				Name:      flavorRef.Name,
				Resources: resources,
			})
		}

		rgs = append(rgs, ResourceGroup{
			CoveredResources: wsRG.CoveredResources,
			Flavors:          flavors,
		})
	}

	return rgs, nil
}

// deriveQuotas calculates nominalQuota for each covered resource as pool.Count * pool.Resources[resource].
func deriveQuotas(coveredResources []string, pool NodePool) ([]Resource, error) {
	resources := make([]Resource, 0, len(coveredResources))

	for _, resName := range coveredResources {
		quantityStr, ok := pool.Resources[resName]
		if !ok {
			return nil, fmt.Errorf("covered resource %q not found in node pool %q resources", resName, pool.Name)
		}

		q, err := resource.ParseQuantity(quantityStr)
		if err != nil {
			return nil, fmt.Errorf("invalid quantity %q for resource %q in pool %q: %w", quantityStr, resName, pool.Name, err)
		}

		// Quantity has no Multiply method; repeated Add is the standard pattern.
		// Value() would truncate sub-unit quantities (e.g. 500m CPU → 0).
		total := q.DeepCopy()
		for i := 1; i < pool.Count; i++ {
			total.Add(q)
		}

		resources = append(resources, Resource{
			Name:         resName,
			NominalQuota: total.String(),
		})
	}

	return resources, nil
}

// taintsToTolerations converts node taints to Kubernetes tolerations.
func taintsToTolerations(taints []Taint) []corev1.Toleration {
	if len(taints) == 0 {
		return nil
	}
	tolerations := make([]corev1.Toleration, len(taints))
	for i, t := range taints {
		tolerations[i] = corev1.Toleration{
			Key:      t.Key,
			Operator: corev1.TolerationOpEqual,
			Value:    t.Value,
			Effect:   corev1.TaintEffect(t.Effect),
		}
	}
	return tolerations
}
