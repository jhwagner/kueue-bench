package kueue

import (
	"context"
	"fmt"
	"time"

	"github.com/jhwagner/kueue-bench/pkg/helm"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	// DefaultKueueVersion is the default Kueue version to install
	DefaultKueueVersion = "0.15.2"

	// Kueue Helm OCI registry configuration
	kueueHelmRegistryURL = "oci://registry.k8s.io/kueue/charts/kueue"

	// Kueue deployment details
	kueueNamespace   = "kueue-system"
	kueueDeployment  = "kueue-controller-manager"
	kueueReleaseName = "kueue"
)

// Install installs Kueue into the cluster via Helm
func Install(ctx context.Context, kubeconfigPath string, version string, helmValues map[string]interface{}) error {
	if version == "" {
		version = DefaultKueueVersion
	}

	fmt.Printf("Installing Kueue %s...\n", version)

	// Install Kueue via Helm
	if err := installKueueChart(ctx, kubeconfigPath, version, helmValues); err != nil {
		return fmt.Errorf("failed to install Kueue chart: %w", err)
	}

	// Wait for Kueue controller to be ready
	fmt.Println("Waiting for Kueue controller to be ready...")
	if err := waitForKueueDeployment(ctx, kubeconfigPath); err != nil {
		return fmt.Errorf("Kueue controller failed to become ready: %w", err)
	}

	fmt.Println("✓ Kueue installed successfully")
	return nil
}

// installKueueChart installs the Kueue Helm chart using the Helm SDK
func installKueueChart(ctx context.Context, kubeconfigPath string, version string, helmValues map[string]interface{}) error {
	return helm.Install(ctx, helm.InstallOptions{
		KubeconfigPath:  kubeconfigPath,
		Namespace:       kueueNamespace,
		ReleaseName:     kueueReleaseName,
		ChartRef:        kueueHelmRegistryURL,
		Version:         version,
		Values:          helmValues,
		CreateNamespace: true,
		Wait:            true,
		Timeout:         5 * time.Minute,
	})
}

// waitForKueueDeployment waits for the Kueue controller deployment to be ready
func waitForKueueDeployment(ctx context.Context, kubeconfigPath string) error {
	// Create Kubernetes client
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	// Wait for deployment to be ready
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, 180*time.Second, true, func(ctx context.Context) (bool, error) {
		deployment, err := clientset.AppsV1().Deployments(kueueNamespace).Get(ctx, kueueDeployment, metav1.GetOptions{})
		if err != nil {
			return false, nil // Keep waiting
		}

		// Check if deployment is available
		for _, condition := range deployment.Status.Conditions {
			if condition.Type == appsv1.DeploymentAvailable && condition.Status == "True" {
				return true, nil
			}
		}

		return false, nil
	})
}
