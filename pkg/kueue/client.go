package kueue

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	kueue "sigs.k8s.io/kueue/apis/kueue/v1beta1"
	kueueclientset "sigs.k8s.io/kueue/client-go/clientset/versioned"
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

// CreateCohort creates or updates a Cohort
func (c *Client) CreateCohort(ctx context.Context, cohort *kueue.Cohort) error {
	_, err := c.kueueClient.KueueV1beta1().Cohorts().Create(ctx, cohort, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		existing, getErr := c.kueueClient.KueueV1beta1().Cohorts().Get(ctx, cohort.Name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("failed to get Cohort %s: %w", cohort.Name, getErr)
		}
		cohort.ResourceVersion = existing.ResourceVersion
		_, err = c.kueueClient.KueueV1beta1().Cohorts().Update(ctx, cohort, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("failed to create or update Cohort %s: %w", cohort.Name, err)
	}
	return nil
}

// CreateResourceFlavor creates or updates a ResourceFlavor
func (c *Client) CreateResourceFlavor(ctx context.Context, rf *kueue.ResourceFlavor) error {
	_, err := c.kueueClient.KueueV1beta1().ResourceFlavors().Create(ctx, rf, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		existing, getErr := c.kueueClient.KueueV1beta1().ResourceFlavors().Get(ctx, rf.Name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("failed to get ResourceFlavor %s: %w", rf.Name, getErr)
		}
		rf.ResourceVersion = existing.ResourceVersion
		_, err = c.kueueClient.KueueV1beta1().ResourceFlavors().Update(ctx, rf, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("failed to create or update ResourceFlavor %s: %w", rf.Name, err)
	}
	return nil
}

// CreateClusterQueue creates or updates a ClusterQueue
func (c *Client) CreateClusterQueue(ctx context.Context, cq *kueue.ClusterQueue) error {
	_, err := c.kueueClient.KueueV1beta1().ClusterQueues().Create(ctx, cq, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		existing, getErr := c.kueueClient.KueueV1beta1().ClusterQueues().Get(ctx, cq.Name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("failed to get ClusterQueue %s: %w", cq.Name, getErr)
		}
		cq.ResourceVersion = existing.ResourceVersion
		_, err = c.kueueClient.KueueV1beta1().ClusterQueues().Update(ctx, cq, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("failed to create or update ClusterQueue %s: %w", cq.Name, err)
	}
	return nil
}

// CreateLocalQueue creates or updates a LocalQueue
func (c *Client) CreateLocalQueue(ctx context.Context, lq *kueue.LocalQueue) error {
	namespace := lq.Namespace
	if namespace == "" {
		namespace = "default"
	}

	_, err := c.kueueClient.KueueV1beta1().LocalQueues(namespace).Create(ctx, lq, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		existing, getErr := c.kueueClient.KueueV1beta1().LocalQueues(namespace).Get(ctx, lq.Name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("failed to get LocalQueue %s/%s: %w", namespace, lq.Name, getErr)
		}
		lq.ResourceVersion = existing.ResourceVersion
		_, err = c.kueueClient.KueueV1beta1().LocalQueues(namespace).Update(ctx, lq, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("failed to create or update LocalQueue %s/%s: %w", namespace, lq.Name, err)
	}
	return nil
}

// CreateWorkloadPriorityClass creates or updates a WorkloadPriorityClass
func (c *Client) CreateWorkloadPriorityClass(ctx context.Context, wpc *kueue.WorkloadPriorityClass) error {
	_, err := c.kueueClient.KueueV1beta1().WorkloadPriorityClasses().Create(ctx, wpc, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		existing, getErr := c.kueueClient.KueueV1beta1().WorkloadPriorityClasses().Get(ctx, wpc.Name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("failed to get WorkloadPriorityClass %s: %w", wpc.Name, getErr)
		}
		wpc.ResourceVersion = existing.ResourceVersion
		_, err = c.kueueClient.KueueV1beta1().WorkloadPriorityClasses().Update(ctx, wpc, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("failed to create or update WorkloadPriorityClass %s: %w", wpc.Name, err)
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

// CreateKubeconfigSecret creates a Secret containing kubeconfig data
func (c *Client) CreateKubeconfigSecret(ctx context.Context, namespace, name string, kubeconfigData []byte) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			kueue.MultiKueueConfigSecretKey: kubeconfigData,
		},
	}

	_, err := c.clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		existing, getErr := c.clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("failed to get Secret %s/%s: %w", namespace, name, getErr)
		}
		secret.ResourceVersion = existing.ResourceVersion
		_, err = c.clientset.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("failed to create or update Secret %s/%s: %w", namespace, name, err)
	}
	return nil
}

// CreateMultiKueueCluster creates or updates a MultiKueueCluster
func (c *Client) CreateMultiKueueCluster(ctx context.Context, mkc *kueue.MultiKueueCluster) error {
	_, err := c.kueueClient.KueueV1beta1().MultiKueueClusters().Create(ctx, mkc, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		existing, getErr := c.kueueClient.KueueV1beta1().MultiKueueClusters().Get(ctx, mkc.Name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("failed to get MultiKueueCluster %s: %w", mkc.Name, getErr)
		}
		mkc.ResourceVersion = existing.ResourceVersion
		_, err = c.kueueClient.KueueV1beta1().MultiKueueClusters().Update(ctx, mkc, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("failed to create or update MultiKueueCluster %s: %w", mkc.Name, err)
	}
	return nil
}

// CreateMultiKueueConfig creates or updates a MultiKueueConfig
func (c *Client) CreateMultiKueueConfig(ctx context.Context, mkc *kueue.MultiKueueConfig) error {
	_, err := c.kueueClient.KueueV1beta1().MultiKueueConfigs().Create(ctx, mkc, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		existing, getErr := c.kueueClient.KueueV1beta1().MultiKueueConfigs().Get(ctx, mkc.Name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("failed to get MultiKueueConfig %s: %w", mkc.Name, getErr)
		}
		mkc.ResourceVersion = existing.ResourceVersion
		_, err = c.kueueClient.KueueV1beta1().MultiKueueConfigs().Update(ctx, mkc, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("failed to create or update MultiKueueConfig %s: %w", mkc.Name, err)
	}
	return nil
}

// CreateAdmissionCheck creates or updates an AdmissionCheck
func (c *Client) CreateAdmissionCheck(ctx context.Context, ac *kueue.AdmissionCheck) error {
	_, err := c.kueueClient.KueueV1beta1().AdmissionChecks().Create(ctx, ac, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		existing, getErr := c.kueueClient.KueueV1beta1().AdmissionChecks().Get(ctx, ac.Name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("failed to get AdmissionCheck %s: %w", ac.Name, getErr)
		}
		ac.ResourceVersion = existing.ResourceVersion
		_, err = c.kueueClient.KueueV1beta1().AdmissionChecks().Update(ctx, ac, metav1.UpdateOptions{})
	}
	if err != nil {
		return fmt.Errorf("failed to create or update AdmissionCheck %s: %w", ac.Name, err)
	}
	return nil
}
