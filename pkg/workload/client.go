package workload

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

// WorkloadClient submits workload objects to a Kubernetes cluster using the dynamic client.
// It uses the dynamic client so that CRD-based workloads (JobSet, RayJob) can be submitted
// without importing their Go SDK dependencies.
type WorkloadClient struct {
	dynamic dynamic.Interface
}

// NewWorkloadClient creates a WorkloadClient from a kubeconfig path.
func NewWorkloadClient(kubeconfigPath string) (*WorkloadClient, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}
	return &WorkloadClient{dynamic: dyn}, nil
}

// Create submits the given unstructured object to the cluster.
func (c *WorkloadClient) Create(ctx context.Context, gvr schema.GroupVersionResource, obj *unstructured.Unstructured) error {
	ns := obj.GetNamespace()
	_, err := c.dynamic.Resource(gvr).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create %s %s/%s: %w", obj.GetKind(), ns, obj.GetName(), err)
	}
	return nil
}
