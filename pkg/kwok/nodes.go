package kwok

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/jhwagner/kueue-bench/pkg/config"
	"sigs.k8s.io/kwok/pkg/kwokctl/scale"
	kwokClient "sigs.k8s.io/kwok/pkg/utils/client"
)

//go:embed templates/node.gotpl
var nodeTemplate string

// CreateNodes creates simulated Kwok nodes based on node pool configuration. Uses Kwok's internal scale.Scale function.
func CreateNodes(ctx context.Context, kubeconfigPath string, nodePools []config.NodePool) error {
	// Kwok's Scale expects a kwok clientset
	clientset, err := kwokClient.NewClientset("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to create kwok clientset: %w", err)
	}

	for _, pool := range nodePools {
		// Build template parameters
		params := buildTemplateParameters(&pool)

		fmt.Printf("Creating %d nodes in pool %s...\n", pool.Count, pool.Name)

		err := scale.Scale(ctx, clientset, scale.Config{
			Template:     nodeTemplate,
			Parameters:   params,
			Name:         fmt.Sprintf("kwok-node-%s", pool.Name),
			Replicas:     pool.Count,
			SerialLength: 3,
		})
		if err != nil {
			return fmt.Errorf("failed to scale pool %s: %w", pool.Name, err)
		}
	}

	fmt.Printf("✓ Nodes created successfully\n")
	return nil
}

// buildTemplateParameters converts NodePool config to template parameters
func buildTemplateParameters(pool *config.NodePool) map[string]interface{} {
	params := make(map[string]interface{})

	// Add labels
	if len(pool.Labels) > 0 {
		params["Labels"] = pool.Labels
	}

	// Add taints
	if len(pool.Taints) > 0 {
		params["Taints"] = pool.Taints
	}

	// Add resources
	params["Resources"] = pool.Resources

	return params
}
