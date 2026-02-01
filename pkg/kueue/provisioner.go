package kueue

import (
	"context"

	"github.com/jhwagner/kueue-bench/pkg/config"
)

// TODO: I'm not actually sure all objects need to be created in dependency order.
// We already validate configs are valid, so we may be able to simplify ProvisionKueueObjects
// e.g. creating a child cohort before parent cohort is created is perfectly fine

// ProvisionKueueObjects creates all Kueue objects from the configuration
// Objects are created in dependency order:
// 1. Cohorts (Kueue handles parent references automatically)
// 2. ResourceFlavors (referenced by ClusterQueues)
// 3. ClusterQueues (referenced by LocalQueues)
// 4. WorkloadPriorityClasses (independent)
// 5. Namespaces (for LocalQueues)
// 6. LocalQueues (last, depends on ClusterQueues and namespaces)
func ProvisionKueueObjects(ctx context.Context, client *Client, kueueConfig *config.KueueConfig) error {
	if kueueConfig == nil {
		return nil
	}

	// Step 1: Create Cohorts
	for _, cohort := range kueueConfig.Cohorts {
		if err := client.CreateCohort(ctx, BuildCohort(cohort)); err != nil {
			return err
		}
	}

	// Step 2: Create ResourceFlavors
	for _, rf := range kueueConfig.ResourceFlavors {
		if err := client.CreateResourceFlavor(ctx, BuildResourceFlavor(rf)); err != nil {
			return err
		}
	}

	// Step 3: Create ClusterQueues
	for _, cq := range kueueConfig.ClusterQueues {
		if err := client.CreateClusterQueue(ctx, BuildClusterQueue(cq)); err != nil {
			return err
		}
	}

	// Step 4: Create WorkloadPriorityClasses
	for _, wpc := range kueueConfig.PriorityClasses {
		if err := client.CreateWorkloadPriorityClass(ctx, BuildWorkloadPriorityClass(wpc)); err != nil {
			return err
		}
	}

	// Step 5: Create namespaces for LocalQueues
	for _, ns := range getUniqueNamespaces(kueueConfig.LocalQueues) {
		if err := client.CreateNamespace(ctx, ns); err != nil {
			return err
		}
	}

	// Step 6: Create LocalQueues
	for _, lq := range kueueConfig.LocalQueues {
		if err := client.CreateLocalQueue(ctx, BuildLocalQueue(lq)); err != nil {
			return err
		}
	}

	return nil
}

// getUniqueNamespaces extracts unique namespaces from LocalQueues, excluding "default"
func getUniqueNamespaces(localQueues []config.LocalQueue) []string {
	namespaceMap := make(map[string]bool)
	for _, lq := range localQueues {
		ns := lq.Namespace
		if ns == "" {
			ns = "default"
		}
		// Skip "default" namespace as it always exists
		if ns != "default" {
			namespaceMap[ns] = true
		}
	}

	namespaces := make([]string, 0, len(namespaceMap))
	for ns := range namespaceMap {
		namespaces = append(namespaces, ns)
	}
	return namespaces
}
