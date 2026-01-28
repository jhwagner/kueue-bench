package kueue

import (
	"context"
	"fmt"

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
		kueueCohort := BuildCohort(cohort)
		if err := client.CreateCohort(ctx, kueueCohort); err != nil {
			return fmt.Errorf("failed to create Cohort %s: %w", cohort.Name, err)
		}
	}

	// Step 2: Create ResourceFlavors
	for _, rf := range kueueConfig.ResourceFlavors {
		kueueRF := BuildResourceFlavor(rf)
		if err := client.CreateResourceFlavor(ctx, kueueRF); err != nil {
			return fmt.Errorf("failed to create ResourceFlavor %s: %w", rf.Name, err)
		}
	}

	// Step 3: Create ClusterQueues
	for _, cq := range kueueConfig.ClusterQueues {
		kueueCQ := BuildClusterQueue(cq)
		if err := client.CreateClusterQueue(ctx, kueueCQ); err != nil {
			return fmt.Errorf("failed to create ClusterQueue %s: %w", cq.Name, err)
		}
	}

	// Step 4: Create WorkloadPriorityClasses
	for _, wpc := range kueueConfig.PriorityClasses {
		kueueWPC := BuildWorkloadPriorityClass(wpc)
		if err := client.CreateWorkloadPriorityClass(ctx, kueueWPC); err != nil {
			return fmt.Errorf("failed to create WorkloadPriorityClass %s: %w", wpc.Name, err)
		}
	}

	// Step 5: Create namespaces for LocalQueues
	namespaces := getUniqueNamespaces(kueueConfig.LocalQueues)
	for _, ns := range namespaces {
		if err := client.CreateNamespace(ctx, ns); err != nil {
			return fmt.Errorf("failed to create namespace %s: %w", ns, err)
		}
	}

	// Step 6: Create LocalQueues
	for _, lq := range kueueConfig.LocalQueues {
		kueueLQ := BuildLocalQueue(lq)
		if err := client.CreateLocalQueue(ctx, kueueLQ); err != nil {
			return fmt.Errorf("failed to create LocalQueue %s/%s: %w", kueueLQ.Namespace, lq.Name, err)
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
