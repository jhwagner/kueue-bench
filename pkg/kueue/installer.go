package kueue

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"
)

// TODO: Refactor to use Helm Go SDK instead of shelling out to helm CLI.
// This will provide better type safety, error handling, and eliminate the
// external dependency on the helm binary.

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

	// Check if helm is installed
	if err := checkHelmInstalled(); err != nil {
		return err
	}

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

// checkHelmInstalled verifies that helm is available in PATH
func checkHelmInstalled() error {
	_, err := exec.LookPath("helm")
	if err != nil {
		return fmt.Errorf("helm is not installed or not in PATH: %w", err)
	}
	return nil
}

// installKueueChart installs the Kueue Helm chart
func installKueueChart(ctx context.Context, kubeconfigPath string, version string, helmValues map[string]interface{}) error {
	args := []string{
		"install", kueueReleaseName, kueueHelmRegistryURL,
		"--version", version,
		"--namespace", kueueNamespace,
		"--create-namespace",
		"--kubeconfig", kubeconfigPath,
		"--wait",
		"--timeout", "5m",
	}

	// If helmValues are provided, write them to a temp file and pass via --values
	var valuesFile string
	if len(helmValues) > 0 {
		tmpFile, err := os.CreateTemp("", "kueue-values-*.yaml")
		if err != nil {
			return fmt.Errorf("failed to create temp values file: %w", err)
		}
		valuesFile = tmpFile.Name()
		defer os.Remove(valuesFile)

		// Marshal helmValues to YAML
		data, err := yaml.Marshal(helmValues)
		if err != nil {
			return fmt.Errorf("failed to marshal helm values: %w", err)
		}

		if _, err := tmpFile.Write(data); err != nil {
			tmpFile.Close()
			return fmt.Errorf("failed to write helm values: %w", err)
		}
		tmpFile.Close()

		args = append(args, "--values", valuesFile)
	}

	cmd := exec.CommandContext(ctx, "helm", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install Kueue Helm chart: %w", err)
	}

	return nil
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
