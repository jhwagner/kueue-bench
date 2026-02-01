package config

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	APIVersion   = "kueue-bench.io/v1alpha1"
	KindTopology = "Topology"

	RoleStandalone = "standalone"
	RoleManagement = "management"
	RoleWorker     = "worker"
)

// ValidateTopology validates a topology configuration
func ValidateTopology(t *Topology) error {
	if t.APIVersion != APIVersion {
		return fmt.Errorf("unsupported apiVersion: %s (expected %s)", t.APIVersion, APIVersion)
	}

	if t.Kind != KindTopology {
		return fmt.Errorf("unsupported kind: %s (expected %s)", t.Kind, KindTopology)
	}

	if t.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}

	if len(t.Spec.Clusters) == 0 && len(t.Spec.WorkerSets) == 0 {
		return fmt.Errorf("at least one cluster or workerSet is required")
	}

	clusterNames := make(map[string]bool, len(t.Spec.Clusters))
	for i, cluster := range t.Spec.Clusters {
		if err := validateCluster(&cluster, i); err != nil {
			return err
		}
		clusterNames[cluster.Name] = true
	}

	if err := validateWorkerSets(t.Spec.WorkerSets, clusterNames); err != nil {
		return err
	}

	return nil
}

func validateCluster(c *ClusterConfig, index int) error {
	if c.Name == "" {
		return fmt.Errorf("cluster[%d]: name is required", index)
	}

	if c.Role != RoleStandalone && c.Role != RoleManagement && c.Role != RoleWorker {
		return fmt.Errorf("cluster[%d] (%s): invalid role '%s' (must be standalone, management, or worker)",
			index, c.Name, c.Role)
	}

	if len(c.NodePools) == 0 {
		return fmt.Errorf("cluster[%d] (%s): at least one nodePool is required", index, c.Name)
	}

	for j, pool := range c.NodePools {
		if err := validateNodePool(&pool, index, j, c.Name); err != nil {
			return err
		}
	}

	if c.Kueue != nil {
		if err := validateKueueConfig(c.Kueue, index, c.Name); err != nil {
			return err
		}
	}

	return nil
}

func validateNodePool(p *NodePool, clusterIndex, poolIndex int, clusterName string) error {
	if p.Name == "" {
		return fmt.Errorf("cluster[%d] (%s): nodePool[%d]: name is required",
			clusterIndex, clusterName, poolIndex)
	}

	if err := validateNodePoolContents(p); err != nil {
		return fmt.Errorf("cluster[%d] (%s): nodePool[%d] (%s): %w",
			clusterIndex, clusterName, poolIndex, p.Name, err)
	}

	return nil
}

// validateNodePoolContents validates pool contents (count, resources, taints).
// Callers wrap the returned error with appropriate context.
func validateNodePoolContents(p *NodePool) error {
	if p.Count <= 0 {
		return fmt.Errorf("count must be > 0")
	}

	if len(p.Resources) == 0 {
		return fmt.Errorf("at least one resource is required")
	}

	for resName, quantity := range p.Resources {
		if _, err := resource.ParseQuantity(quantity); err != nil {
			return fmt.Errorf("invalid resource quantity for %s: %w", resName, err)
		}
	}

	for k, taint := range p.Taints {
		if taint.Effect != "NoSchedule" && taint.Effect != "PreferNoSchedule" && taint.Effect != "NoExecute" {
			return fmt.Errorf("taint[%d]: invalid effect '%s' (must be NoSchedule, PreferNoSchedule, or NoExecute)",
				k, taint.Effect)
		}
	}

	return nil
}

func validateKueueConfig(k *KueueConfig, clusterIndex int, clusterName string) error {
	// Validate Cohorts
	if err := validateCohorts(k.Cohorts, clusterIndex, clusterName); err != nil {
		return err
	}

	// Build a map of cohort names for validation
	cohortNames := make(map[string]bool)
	for _, cohort := range k.Cohorts {
		cohortNames[cohort.Name] = true
	}

	// Build a map of resource flavor names for validation
	flavorNames := make(map[string]bool)
	for _, rf := range k.ResourceFlavors {
		if rf.Name == "" {
			return fmt.Errorf("cluster[%d] (%s): resourceFlavor: name is required", clusterIndex, clusterName)
		}
		flavorNames[rf.Name] = true
	}

	// Validate ClusterQueues
	clusterQueueNames := make(map[string]bool)
	for i, cq := range k.ClusterQueues {
		if cq.Name == "" {
			return fmt.Errorf("cluster[%d] (%s): clusterQueue[%d]: name is required", clusterIndex, clusterName, i)
		}
		clusterQueueNames[cq.Name] = true

		// Validate that referenced cohort exists
		if cq.Cohort != "" && !cohortNames[cq.Cohort] {
			return fmt.Errorf("cluster[%d] (%s): clusterQueue[%d] (%s): unknown cohort '%s'",
				clusterIndex, clusterName, i, cq.Name, cq.Cohort)
		}

		if len(cq.ResourceGroups) == 0 {
			return fmt.Errorf("cluster[%d] (%s): clusterQueue[%d] (%s): at least one resourceGroup is required",
				clusterIndex, clusterName, i, cq.Name)
		}

		// Validate that referenced flavors exist
		for j, rg := range cq.ResourceGroups {
			for k, fq := range rg.Flavors {
				if !flavorNames[fq.Name] {
					return fmt.Errorf("cluster[%d] (%s): clusterQueue[%d] (%s): resourceGroup[%d]: flavor[%d]: unknown resourceFlavor '%s'",
						clusterIndex, clusterName, i, cq.Name, j, k, fq.Name)
				}

				// Validate resource quotas
				for l, res := range fq.Resources {
					if _, err := resource.ParseQuantity(res.NominalQuota); err != nil {
						return fmt.Errorf("cluster[%d] (%s): clusterQueue[%d] (%s): resourceGroup[%d]: flavor[%d]: resource[%d]: invalid nominalQuota: %w",
							clusterIndex, clusterName, i, cq.Name, j, k, l, err)
					}
				}
			}
		}
	}

	// Validate LocalQueues
	for i, lq := range k.LocalQueues {
		if lq.Name == "" {
			return fmt.Errorf("cluster[%d] (%s): localQueue[%d]: name is required", clusterIndex, clusterName, i)
		}
		if lq.Namespace == "" {
			return fmt.Errorf("cluster[%d] (%s): localQueue[%d] (%s): namespace is required",
				clusterIndex, clusterName, i, lq.Name)
		}
		if lq.ClusterQueue == "" {
			return fmt.Errorf("cluster[%d] (%s): localQueue[%d] (%s): clusterQueue is required",
				clusterIndex, clusterName, i, lq.Name)
		}

		// Validate that referenced cluster queue exists
		if !clusterQueueNames[lq.ClusterQueue] {
			return fmt.Errorf("cluster[%d] (%s): localQueue[%d] (%s): unknown clusterQueue '%s'",
				clusterIndex, clusterName, i, lq.Name, lq.ClusterQueue)
		}
	}

	return nil
}

func validateWorkerSets(workerSets []WorkerSet, clusterNames map[string]bool) error {
	wsNames := make(map[string]bool)
	workerNames := make(map[string]bool)

	for i, ws := range workerSets {
		if ws.Name == "" {
			return fmt.Errorf("workerSet[%d]: name is required", i)
		}
		if wsNames[ws.Name] {
			return fmt.Errorf("workerSet[%d]: duplicate workerSet name '%s'", i, ws.Name)
		}
		wsNames[ws.Name] = true

		if len(ws.ResourceFlavors) == 0 {
			return fmt.Errorf("workerSet[%d] (%s): at least one resourceFlavor is required", i, ws.Name)
		}
		if len(ws.ClusterQueues) == 0 {
			return fmt.Errorf("workerSet[%d] (%s): at least one clusterQueue is required", i, ws.Name)
		}
		if len(ws.Workers) == 0 {
			return fmt.Errorf("workerSet[%d] (%s): at least one worker is required", i, ws.Name)
		}

		// Build flavor name to nodePoolRef map
		flavorPools := make(map[string]string, len(ws.ResourceFlavors))
		for j, f := range ws.ResourceFlavors {
			if f.Name == "" {
				return fmt.Errorf("workerSet[%d] (%s): resourceFlavor[%d]: name is required", i, ws.Name, j)
			}
			if f.NodePoolRef == "" {
				return fmt.Errorf("workerSet[%d] (%s): resourceFlavor[%d] (%s): nodePoolRef is required", i, ws.Name, j, f.Name)
			}
			flavorPools[f.Name] = f.NodePoolRef
		}

		// Validate ClusterQueue structure and flavor references
		cqNames := make(map[string]bool, len(ws.ClusterQueues))
		for j, cq := range ws.ClusterQueues {
			if cq.Name == "" {
				return fmt.Errorf("workerSet[%d] (%s): clusterQueue[%d]: name is required", i, ws.Name, j)
			}
			cqNames[cq.Name] = true

			if len(cq.ResourceGroups) == 0 {
				return fmt.Errorf("workerSet[%d] (%s): clusterQueue[%d] (%s): at least one resourceGroup is required",
					i, ws.Name, j, cq.Name)
			}
			for k, rg := range cq.ResourceGroups {
				if len(rg.CoveredResources) == 0 {
					return fmt.Errorf("workerSet[%d] (%s): clusterQueue[%d] (%s): resourceGroup[%d]: at least one coveredResource is required",
						i, ws.Name, j, cq.Name, k)
				}
				for l, fr := range rg.Flavors {
					if _, ok := flavorPools[fr.Name]; !ok {
						return fmt.Errorf("workerSet[%d] (%s): clusterQueue[%d] (%s): resourceGroup[%d]: flavor[%d]: unknown resourceFlavor '%s'",
							i, ws.Name, j, cq.Name, k, l, fr.Name)
					}
				}
			}
		}

		// Validate LocalQueue references
		for j, lq := range ws.LocalQueues {
			if lq.Name == "" {
				return fmt.Errorf("workerSet[%d] (%s): localQueue[%d]: name is required", i, ws.Name, j)
			}
			if lq.Namespace == "" {
				return fmt.Errorf("workerSet[%d] (%s): localQueue[%d] (%s): namespace is required",
					i, ws.Name, j, lq.Name)
			}
			if lq.ClusterQueue == "" {
				return fmt.Errorf("workerSet[%d] (%s): localQueue[%d] (%s): clusterQueue is required",
					i, ws.Name, j, lq.Name)
			}
			if !cqNames[lq.ClusterQueue] {
				return fmt.Errorf("workerSet[%d] (%s): localQueue[%d] (%s): unknown clusterQueue '%s'",
					i, ws.Name, j, lq.Name, lq.ClusterQueue)
			}
		}

		// Build map of required resources per pool for cross-checking workers
		poolRequiredResources := make(map[string]map[string]bool)
		for _, cq := range ws.ClusterQueues {
			for _, rg := range cq.ResourceGroups {
				for _, fr := range rg.Flavors {
					poolName := flavorPools[fr.Name]
					if poolRequiredResources[poolName] == nil {
						poolRequiredResources[poolName] = make(map[string]bool)
					}
					for _, cr := range rg.CoveredResources {
						poolRequiredResources[poolName][cr] = true
					}
				}
			}
		}

		// Validate each worker
		for j, worker := range ws.Workers {
			if worker.Name == "" {
				return fmt.Errorf("workerSet[%d] (%s): worker[%d]: name is required", i, ws.Name, j)
			}
			if clusterNames[worker.Name] {
				return fmt.Errorf("workerSet[%d] (%s): worker[%d]: name '%s' conflicts with an existing cluster",
					i, ws.Name, j, worker.Name)
			}
			if workerNames[worker.Name] {
				return fmt.Errorf("workerSet[%d] (%s): worker[%d]: duplicate worker name '%s'",
					i, ws.Name, j, worker.Name)
			}
			workerNames[worker.Name] = true

			if len(worker.NodePools) == 0 {
				return fmt.Errorf("workerSet[%d] (%s): worker[%d] (%s): at least one nodePool is required",
					i, ws.Name, j, worker.Name)
			}

			pools := make(map[string]NodePool, len(worker.NodePools))
			for k, pool := range worker.NodePools {
				if pool.Name == "" {
					return fmt.Errorf("workerSet[%d] (%s): worker[%d] (%s): nodePool[%d]: name is required",
						i, ws.Name, j, worker.Name, k)
				}
				if err := validateNodePoolContents(&pool); err != nil {
					return fmt.Errorf("workerSet[%d] (%s): worker[%d] (%s): nodePool[%d] (%s): %w",
						i, ws.Name, j, worker.Name, k, pool.Name, err)
				}
				pools[pool.Name] = pool
			}

			// Verify all nodePoolRefs exist in this worker
			for _, f := range ws.ResourceFlavors {
				if _, ok := pools[f.NodePoolRef]; !ok {
					return fmt.Errorf("workerSet[%d] (%s): worker[%d] (%s): nodePoolRef '%s' (from resourceFlavor '%s') not found",
						i, ws.Name, j, worker.Name, f.NodePoolRef, f.Name)
				}
			}

			// Verify all covered resources exist in the referenced pools
			for poolName, requiredResources := range poolRequiredResources {
				pool := pools[poolName]
				for cr := range requiredResources {
					if _, ok := pool.Resources[cr]; !ok {
						return fmt.Errorf("workerSet[%d] (%s): worker[%d] (%s): nodePool '%s': covered resource '%s' not found in pool resources",
							i, ws.Name, j, worker.Name, poolName, cr)
					}
				}
			}
		}
	}

	return nil
}

// validateCohorts validates cohort configuration
func validateCohorts(cohorts []Cohort, clusterIndex int, clusterName string) error {
	if len(cohorts) == 0 {
		return nil
	}

	// Build a map of cohort names
	cohortNames := make(map[string]bool)
	for i, cohort := range cohorts {
		if cohort.Name == "" {
			return fmt.Errorf("cluster[%d] (%s): cohort[%d]: name is required",
				clusterIndex, clusterName, i)
		}

		if cohortNames[cohort.Name] {
			return fmt.Errorf("cluster[%d] (%s): cohort[%d]: duplicate cohort name '%s'",
				clusterIndex, clusterName, i, cohort.Name)
		}

		cohortNames[cohort.Name] = true
	}

	// Validate that parent cohorts exist
	for i, cohort := range cohorts {
		if cohort.ParentName != "" {
			if !cohortNames[cohort.ParentName] {
				return fmt.Errorf("cluster[%d] (%s): cohort[%d] (%s): unknown parent cohort '%s'",
					clusterIndex, clusterName, i, cohort.Name, cohort.ParentName)
			}
		}
	}

	return nil
}
