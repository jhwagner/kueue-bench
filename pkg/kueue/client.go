package kueue

import (
	"context"
	"fmt"

	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
	kueueclientset "sigs.k8s.io/kueue/client-go/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client wraps Kubernetes clients for Kueue object operations
type Client struct {
	kueueClient kueueclientset.Interface
	clientset   kubernetes.Interface
	config      *rest.Config
}

// NewClient creates a new Kueue client from a kubeconfig path
func NewClient(kubeconfigPath string) (*Client, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	kueueClient, err := kueueclientset.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kueue clientset: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return &Client{
		kueueClient: kueueClient,
		clientset:   clientset,
		config:      config,
	}, nil
}

// CreateResourceFlavor creates or updates a ResourceFlavor
func (c *Client) CreateResourceFlavor(ctx context.Context, rf *kueuev1beta1.ResourceFlavor) error {
	_, err := c.kueueClient.KueueV1beta1().ResourceFlavors().Create(ctx, rf, metav1.CreateOptions{})
	if err != nil {
		// If create failed, try to update
		existing, getErr := c.kueueClient.KueueV1beta1().ResourceFlavors().Get(ctx, rf.Name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("failed to create ResourceFlavor %s: %w", rf.Name, err)
		}

		// Preserve resourceVersion for update
		rf.ResourceVersion = existing.ResourceVersion
		_, err = c.kueueClient.KueueV1beta1().ResourceFlavors().Update(ctx, rf, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update ResourceFlavor %s: %w", rf.Name, err)
		}
	}

	return nil
}

// CreateClusterQueue creates or updates a ClusterQueue
func (c *Client) CreateClusterQueue(ctx context.Context, cq *kueuev1beta1.ClusterQueue) error {
	_, err := c.kueueClient.KueueV1beta1().ClusterQueues().Create(ctx, cq, metav1.CreateOptions{})
	if err != nil {
		// If create failed, try to update
		existing, getErr := c.kueueClient.KueueV1beta1().ClusterQueues().Get(ctx, cq.Name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("failed to create ClusterQueue %s: %w", cq.Name, err)
		}

		// Preserve resourceVersion for update
		cq.ResourceVersion = existing.ResourceVersion
		_, err = c.kueueClient.KueueV1beta1().ClusterQueues().Update(ctx, cq, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update ClusterQueue %s: %w", cq.Name, err)
		}
	}

	return nil
}

// CreateLocalQueue creates or updates a LocalQueue
func (c *Client) CreateLocalQueue(ctx context.Context, lq *kueuev1beta1.LocalQueue) error {
	namespace := lq.Namespace
	if namespace == "" {
		namespace = "default"
	}

	_, err := c.kueueClient.KueueV1beta1().LocalQueues(namespace).Create(ctx, lq, metav1.CreateOptions{})
	if err != nil {
		// If create failed, try to update
		existing, getErr := c.kueueClient.KueueV1beta1().LocalQueues(namespace).Get(ctx, lq.Name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("failed to create LocalQueue %s/%s: %w", namespace, lq.Name, err)
		}

		// Preserve resourceVersion for update
		lq.ResourceVersion = existing.ResourceVersion
		_, err = c.kueueClient.KueueV1beta1().LocalQueues(namespace).Update(ctx, lq, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update LocalQueue %s/%s: %w", namespace, lq.Name, err)
		}
	}

	return nil
}

// CreateWorkloadPriorityClass creates or updates a WorkloadPriorityClass
func (c *Client) CreateWorkloadPriorityClass(ctx context.Context, wpc *kueuev1beta1.WorkloadPriorityClass) error {
	_, err := c.kueueClient.KueueV1beta1().WorkloadPriorityClasses().Create(ctx, wpc, metav1.CreateOptions{})
	if err != nil {
		// If create failed, try to update
		existing, getErr := c.kueueClient.KueueV1beta1().WorkloadPriorityClasses().Get(ctx, wpc.Name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("failed to create WorkloadPriorityClass %s: %w", wpc.Name, err)
		}

		// Preserve resourceVersion for update
		wpc.ResourceVersion = existing.ResourceVersion
		_, err = c.kueueClient.KueueV1beta1().WorkloadPriorityClasses().Update(ctx, wpc, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update WorkloadPriorityClass %s: %w", wpc.Name, err)
		}
	}

	return nil
}

// CreateNamespace creates a namespace if it doesn't exist
func (c *Client) CreateNamespace(ctx context.Context, name string) error {
	_, err := c.clientset.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		// Namespace already exists
		return nil
	}

	// Create namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	_, err = c.clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create namespace %s: %w", name, err)
	}

	return nil
}
