package config

import (
	"k8s.io/apimachinery/pkg/api/resource"
)

// DeriveManagementKueueConfig derives the management cluster's KueueConfig from WorkerSets.
// It creates:
// - ResourceFlavors: minimal flavors (name only) matching WorkerSet flavor names for MultiKueue routing
// - ClusterQueues: matching WorkerSet CQ names with auto-added admissionChecks and summed quotas
// - LocalQueues: derived from WorkerSet LocalQueues (workloads are submitted to management cluster)
// - Merges with user-defined objects from managementKueueConfig (cohorts, priorityClasses, etc.)
//
// Parameters:
// - workerSets: WorkerSet definitions from the topology spec
// - expandedWorkers: Worker ClusterConfigs with derived quotas (from ExpandWorkerSets)
// - managementKueueConfig: User-defined Kueue config for management cluster (can be nil)
func DeriveManagementKueueConfig(workerSets []WorkerSet, expandedWorkers []ClusterConfig, managementKueueConfig *KueueConfig) *KueueConfig {
	if len(workerSets) == 0 {
		// No WorkerSets, just return user-defined config as-is
		return managementKueueConfig
	}

	// Index expanded workers by name, then group by WorkerSet for efficient quota aggregation
	workerByName := make(map[string]ClusterConfig, len(expandedWorkers))
	for _, w := range expandedWorkers {
		workerByName[w.Name] = w
	}
	workersByWS := make(map[string][]ClusterConfig, len(workerSets))
	for _, ws := range workerSets {
		for _, worker := range ws.Workers {
			workersByWS[ws.Name] = append(workersByWS[ws.Name], workerByName[worker.Name])
		}
	}

	// Derive ResourceFlavors, ClusterQueues, and LocalQueues from WorkerSets
	derivedFlavors := deriveManagementResourceFlavors(workerSets)
	derivedCQs := deriveManagementClusterQueues(workerSets, workersByWS)
	derivedLQs := deriveManagementLocalQueues(workerSets)

	// Start with derived objects
	result := &KueueConfig{
		ResourceFlavors: derivedFlavors,
		ClusterQueues:   derivedCQs,
		LocalQueues:     derivedLQs,
	}

	// Merge user-defined objects from management cluster config
	if managementKueueConfig != nil {
		result.Cohorts = managementKueueConfig.Cohorts
		result.PriorityClasses = managementKueueConfig.PriorityClasses

		// Append user-defined objects (derived ones take precedence)
		result.ResourceFlavors = append(result.ResourceFlavors, managementKueueConfig.ResourceFlavors...)
		result.ClusterQueues = append(result.ClusterQueues, managementKueueConfig.ClusterQueues...)
		result.LocalQueues = append(result.LocalQueues, managementKueueConfig.LocalQueues...)
	}

	return result
}

// deriveManagementResourceFlavors creates minimal ResourceFlavors for the management cluster.
// These flavors only have names (no labels/tolerations) - just enough for MultiKueue routing.
// Output order follows input order (stable across runs).
func deriveManagementResourceFlavors(workerSets []WorkerSet) []ResourceFlavor {
	seen := make(map[string]bool)
	var flavors []ResourceFlavor
	for _, ws := range workerSets {
		for _, f := range ws.ResourceFlavors {
			if !seen[f.Name] {
				seen[f.Name] = true
				flavors = append(flavors, ResourceFlavor{Name: f.Name})
			}
		}
	}
	return flavors
}

// deriveManagementClusterQueues creates ClusterQueues for the management cluster.
// Each CQ:
// - Matches a WorkerSet CQ name
// - Has admissionChecks: [workerSetName] auto-added
// - Has quotas summed from all workers in that WorkerSet
//
// All inputs are pre-validated by config validation and ExpandWorkerSets.
func deriveManagementClusterQueues(workerSets []WorkerSet, workersByWS map[string][]ClusterConfig) []ClusterQueue {
	var cqs []ClusterQueue

	for _, ws := range workerSets {
		for _, wsCQ := range ws.ClusterQueues {
			// Aggregate quotas from all workers in this WorkerSet
			aggregatedRGs := aggregateWorkerQuotas(wsCQ.Name, workersByWS[ws.Name])

			// Create management CQ with auto-added admissionChecks
			admissionChecks := []string{ws.Name}
			if len(wsCQ.AdmissionChecks) > 0 {
				admissionChecks = append(admissionChecks, wsCQ.AdmissionChecks...)
			}

			cqs = append(cqs, ClusterQueue{
				Name:              wsCQ.Name,
				Cohort:            wsCQ.Cohort,
				NamespaceSelector: wsCQ.NamespaceSelector,
				Preemption:        wsCQ.Preemption,
				ResourceGroups:    aggregatedRGs,
				AdmissionChecks:   admissionChecks,
				FairSharing:       wsCQ.FairSharing,
			})
		}
	}

	return cqs
}

// deriveManagementLocalQueues collects LocalQueues from all WorkerSets for the management cluster.
// Workloads are submitted to the management cluster, so it needs matching LocalQueues.
// Deduplicates by namespace/name key.
func deriveManagementLocalQueues(workerSets []WorkerSet) []LocalQueue {
	seen := make(map[string]bool)
	var queues []LocalQueue
	for _, ws := range workerSets {
		for _, lq := range ws.LocalQueues {
			key := lq.Namespace + "/" + lq.Name
			if !seen[key] {
				seen[key] = true
				queues = append(queues, lq)
			}
		}
	}
	return queues
}

// aggregateWorkerQuotas sums quotas across all workers in a WorkerSet for a specific ClusterQueue.
// All inputs are pre-validated: quota strings come from Quantity.String() (always parseable),
// and workers are pre-grouped by WorkerSet.
func aggregateWorkerQuotas(cqName string, workers []ClusterConfig) []ResourceGroup {
	// Find the matching CQ from each worker
	var workerCQs []*ClusterQueue
	for i := range workers {
		if workers[i].Kueue == nil {
			continue
		}
		for j := range workers[i].Kueue.ClusterQueues {
			if workers[i].Kueue.ClusterQueues[j].Name == cqName {
				workerCQs = append(workerCQs, &workers[i].Kueue.ClusterQueues[j])
				break
			}
		}
	}

	if len(workerCQs) == 0 {
		return nil
	}

	// Use first worker's CQ as structural template
	templateCQ := workerCQs[0]

	// Aggregate quotas across all workers
	aggregatedRGs := make([]ResourceGroup, len(templateCQ.ResourceGroups))
	for rgIdx, rg := range templateCQ.ResourceGroups {
		aggregatedRGs[rgIdx] = ResourceGroup{
			CoveredResources: rg.CoveredResources,
			Flavors:          make([]FlavorQuotas, len(rg.Flavors)),
		}

		for flavorIdx, flavor := range rg.Flavors {
			aggregated := make([]resource.Quantity, len(flavor.Resources))
			for _, workerCQ := range workerCQs {
				workerFlavor := workerCQ.ResourceGroups[rgIdx].Flavors[flavorIdx]
				for i, res := range workerFlavor.Resources {
					q := resource.MustParse(res.NominalQuota)
					aggregated[i].Add(q)
				}
			}

			// Convert aggregated quantities to Resource slice
			resources := make([]Resource, len(flavor.Resources))
			for i, res := range flavor.Resources {
				resources[i] = Resource{
					Name:         res.Name,
					NominalQuota: aggregated[i].String(),
				}
			}

			aggregatedRGs[rgIdx].Flavors[flavorIdx] = FlavorQuotas{
				Name:      flavor.Name,
				Resources: resources,
			}
		}
	}

	return aggregatedRGs
}
