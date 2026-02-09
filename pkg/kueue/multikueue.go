package kueue

import (
	"context"
	"fmt"

	"github.com/jhwagner/kueue-bench/pkg/config"
)

const (
	// MultiKueueNamespace is the namespace where MultiKueue secrets and resources are created
	MultiKueueNamespace = "kueue-system"
)

// SetupMultiKueueInfrastructure creates MultiKueue infrastructure on the management cluster.
// For each WorkerSet, it:
// - Creates kubeconfig Secrets in kueue-system (one per worker)
// - Creates MultiKueueCluster objects (one per worker)
// - Creates MultiKueueConfig object (named after WorkerSet, references all workers)
// - Creates AdmissionCheck object (named after WorkerSet, references MultiKueueConfig)
//
// Parameters:
// - ctx: Context for Kubernetes API calls
// - client: Kueue client connected to management cluster
// - workerSets: WorkerSet definitions from topology spec
// - workerKubeconfigs: Map of worker name -> internal kubeconfig bytes
func SetupMultiKueueInfrastructure(ctx context.Context, client *Client, workerSets []config.WorkerSet, workerKubeconfigs map[string][]byte) error {
	for _, ws := range workerSets {
		// Collect worker cluster names for this WorkerSet
		var clusterNames []string

		// Create Secrets and MultiKueueCluster objects for each worker
		for _, worker := range ws.Workers {
			kubeconfigData, ok := workerKubeconfigs[worker.Name]
			if !ok {
				return fmt.Errorf("kubeconfig not found for worker %q", worker.Name)
			}

			// Create Secret with kubeconfig
			secretName := fmt.Sprintf("%s-kubeconfig", worker.Name)
			if err := client.CreateKubeconfigSecret(ctx, MultiKueueNamespace, secretName, kubeconfigData); err != nil {
				return fmt.Errorf("failed to create kubeconfig secret for worker %q: %w", worker.Name, err)
			}

			// Create MultiKueueCluster object
			mkc := BuildMultiKueueCluster(worker.Name, secretName)
			if err := client.CreateMultiKueueCluster(ctx, mkc); err != nil {
				return fmt.Errorf("failed to create MultiKueueCluster for worker %q: %w", worker.Name, err)
			}

			clusterNames = append(clusterNames, worker.Name)
		}

		// Create MultiKueueConfig object (named after WorkerSet)
		mkcfg := BuildMultiKueueConfig(ws.Name, clusterNames)
		if err := client.CreateMultiKueueConfig(ctx, mkcfg); err != nil {
			return fmt.Errorf("failed to create MultiKueueConfig for workerSet %q: %w", ws.Name, err)
		}

		// Create AdmissionCheck object (named after WorkerSet)
		ac := BuildAdmissionCheck(ws.Name, ws.Name)
		if err := client.CreateAdmissionCheck(ctx, ac); err != nil {
			return fmt.Errorf("failed to create AdmissionCheck for workerSet %q: %w", ws.Name, err)
		}
	}

	return nil
}
