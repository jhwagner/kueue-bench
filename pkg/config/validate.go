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

	if len(t.Spec.Clusters) == 0 {
		return fmt.Errorf("at least one cluster is required")
	}

	for i, cluster := range t.Spec.Clusters {
		if err := validateCluster(&cluster, i); err != nil {
			return err
		}
	}

	return nil
}

func validateCluster(c *ClusterConfig, index int) error {
	if c.Name == "" {
		return fmt.Errorf("cluster[%d]: name is required", index)
	}

	// TODO: Initially only support standalone clusters
	if c.Role != RoleStandalone {
		return fmt.Errorf("cluster[%d] (%s): only 'standalone' role is currently supported (got '%s')",
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

	if p.Count <= 0 {
		return fmt.Errorf("cluster[%d] (%s): nodePool[%d] (%s): count must be > 0",
			clusterIndex, clusterName, poolIndex, p.Name)
	}

	if len(p.Resources) == 0 {
		return fmt.Errorf("cluster[%d] (%s): nodePool[%d] (%s): at least one resource is required",
			clusterIndex, clusterName, poolIndex, p.Name)
	}

	// Validate resource quantities
	for resName, quantity := range p.Resources {
		if _, err := resource.ParseQuantity(quantity); err != nil {
			return fmt.Errorf("cluster[%d] (%s): nodePool[%d] (%s): invalid resource quantity for %s: %w",
				clusterIndex, clusterName, poolIndex, p.Name, resName, err)
		}
	}

	// Validate taint effects
	for k, taint := range p.Taints {
		if taint.Effect != "NoSchedule" && taint.Effect != "PreferNoSchedule" && taint.Effect != "NoExecute" {
			return fmt.Errorf("cluster[%d] (%s): nodePool[%d] (%s): taint[%d]: invalid effect '%s' (must be NoSchedule, PreferNoSchedule, or NoExecute)",
				clusterIndex, clusterName, poolIndex, p.Name, k, taint.Effect)
		}
	}

	return nil
}

func validateKueueConfig(k *KueueConfig, clusterIndex int, clusterName string) error {
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
