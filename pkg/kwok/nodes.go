package kwok

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/jhwagner/kueue-bench/pkg/config"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	// Maximum number of concurrent node creation goroutines
	maxConcurrency = 100
)

// CreateNodes creates simulated Kwok nodes based on node pool configuration
func CreateNodes(ctx context.Context, kubeconfigPath string, nodePools []config.NodePool) error {
	// Create Kubernetes client
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	// Increase rate limits for bulk node creation (local cluster with fake nodes)
	cfg.QPS = 50    // requests per second
	cfg.Burst = 100 // burst capacity

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	totalNodes := 0
	for _, pool := range nodePools {
		totalNodes += pool.Count
	}

	fmt.Printf("Creating %d simulated nodes (using %d parallel workers)...\n", totalNodes, maxConcurrency)

	// Use errgroup for parallel node creation with bounded concurrency
	g, ctx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, maxConcurrency)
	var created atomic.Int32

	for _, pool := range nodePools {
		pool := pool // capture loop variable
		for i := 0; i < pool.Count; i++ {
			i := i // capture loop variable

			g.Go(func() error {
				// Acquire semaphore
				sem <- struct{}{}
				defer func() { <-sem }()

				nodeName := fmt.Sprintf("kwok-node-%s-%03d", pool.Name, i)
				node := generateNodeManifest(nodeName, &pool)

				// Create node
				_, err := clientset.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
				if err != nil {
					if errors.IsAlreadyExists(err) {
						// Update existing node
						_, err = clientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
						if err != nil {
							return fmt.Errorf("failed to update node %s: %w", nodeName, err)
						}
					} else {
						return fmt.Errorf("failed to create node %s: %w", nodeName, err)
					}
				}

				// Update progress counter
				count := created.Add(1)
				if count%10 == 0 {
					fmt.Printf("  Created %d/%d nodes...\n", count, totalNodes)
				}

				return nil
			})
		}
	}

	// Wait for all goroutines to complete
	if err := g.Wait(); err != nil {
		return err
	}

	fmt.Printf("✓ Created %d simulated nodes successfully\n", totalNodes)
	return nil
}

func generateNodeManifest(name string, pool *config.NodePool) *corev1.Node {
	// Parse resource quantities
	capacity := corev1.ResourceList{}
	allocatable := corev1.ResourceList{}

	for resName, quantity := range pool.Resources {
		q := resource.MustParse(quantity)
		resourceName := corev1.ResourceName(resName)
		capacity[resourceName] = q
		allocatable[resourceName] = q
	}

	// Build labels
	labels := map[string]string{
		"type":                    "kwok",
		"kwok.x-k8s.io/node":      "fake",
		"node.kubernetes.io/role": "agent",
	}
	for k, v := range pool.Labels {
		labels[k] = v
	}

	// Build taints
	var taints []corev1.Taint
	for _, t := range pool.Taints {
		taints = append(taints, corev1.Taint{
			Key:    t.Key,
			Value:  t.Value,
			Effect: corev1.TaintEffect(t.Effect),
		})
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
			Annotations: map[string]string{
				"kwok.x-k8s.io/node": "fake",
			},
		},
		Spec: corev1.NodeSpec{
			Taints: taints,
		},
		Status: corev1.NodeStatus{
			Capacity:    capacity,
			Allocatable: allocatable,
			Phase:       corev1.NodeRunning,
			Conditions: []corev1.NodeCondition{
				{
					Type:               corev1.NodeReady,
					Status:             corev1.ConditionTrue,
					LastHeartbeatTime:  metav1.Now(),
					LastTransitionTime: metav1.Now(),
					Reason:             "KubeletReady",
					Message:            "kubelet is posting ready status",
				},
				{
					Type:               corev1.NodeMemoryPressure,
					Status:             corev1.ConditionFalse,
					LastHeartbeatTime:  metav1.Now(),
					LastTransitionTime: metav1.Now(),
					Reason:             "KubeletHasSufficientMemory",
					Message:            "kubelet has sufficient memory available",
				},
				{
					Type:               corev1.NodeDiskPressure,
					Status:             corev1.ConditionFalse,
					LastHeartbeatTime:  metav1.Now(),
					LastTransitionTime: metav1.Now(),
					Reason:             "KubeletHasNoDiskPressure",
					Message:            "kubelet has no disk pressure",
				},
				{
					Type:               corev1.NodePIDPressure,
					Status:             corev1.ConditionFalse,
					LastHeartbeatTime:  metav1.Now(),
					LastTransitionTime: metav1.Now(),
					Reason:             "KubeletHasSufficientPID",
					Message:            "kubelet has sufficient PID available",
				},
			},
			Addresses: []corev1.NodeAddress{
				{
					Type:    corev1.NodeInternalIP,
					Address: "127.0.0.1",
				},
			},
		},
	}

	return node
}
