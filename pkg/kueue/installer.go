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
)

// TODO: Refactor to use Helm Go SDK instead of shelling out to helm CLI.
// This will provide better type safety, error handling, and eliminate the
// external dependency on the helm binary.

const (
	// DefaultKueueVersion is the default Kueue version to install
	DefaultKueueVersion = "v0.15.2"

	// Kueue Helm repository configuration
	kueueHelmRepoName = "kueue"
	kueueHelmRepoURL  = "https://kubernetes-sigs.github.io/kueue"
	kueueHelmChart    = "kueue/kueue"

	// Kueue deployment details
	kueueNamespace   = "kueue-system"
	kueueDeployment  = "kueue-controller-manager"
	kueueReleaseName = "kueue"
)

// Install installs Kueue into the cluster via Helm
func Install(ctx context.Context, kubeconfigPath string, version string, imageRepository string, imageTag string) error {
	if version == "" {
		version = DefaultKueueVersion
	}

	fmt.Printf("Installing Kueue %s...\n", version)

	// Check if helm is installed
	if err := checkHelmInstalled(); err != nil {
		return err
	}

	// Add Kueue Helm repository
	if err := addHelmRepo(ctx); err != nil {
		return fmt.Errorf("failed to add Helm repository: %w", err)
	}

	// Update Helm repositories
	if err := updateHelmRepos(ctx); err != nil {
		return fmt.Errorf("failed to update Helm repositories: %w", err)
	}

	// Install Kueue via Helm
	if err := installKueueChart(ctx, kubeconfigPath, version, imageRepository, imageTag); err != nil {
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

// addHelmRepo adds the Kueue Helm repository
func addHelmRepo(ctx context.Context) error {
	fmt.Printf("Adding Helm repository '%s'...\n", kueueHelmRepoName)
	cmd := exec.CommandContext(ctx, "helm", "repo", "add", kueueHelmRepoName, kueueHelmRepoURL)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// Ignore error if repo already exists
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() != 0 {
				// Check if it's just "already exists" error (non-fatal)
				return nil
			}
		}
		return fmt.Errorf("failed to add Helm repository: %w", err)
	}

	return nil
}

// updateHelmRepos updates all Helm repositories
func updateHelmRepos(ctx context.Context) error {
	fmt.Println("Updating Helm repositories...")
	cmd := exec.CommandContext(ctx, "helm", "repo", "update")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to update Helm repositories: %w", err)
	}

	return nil
}

// installKueueChart installs the Kueue Helm chart
func installKueueChart(ctx context.Context, kubeconfigPath string, version string, imageRepository string, imageTag string) error {
	fmt.Printf("Installing Kueue chart (version: %s)...\n", version)

	args := []string{
		"install", kueueReleaseName, kueueHelmChart,
		"--version", version,
		"--namespace", kueueNamespace,
		"--create-namespace",
		"--kubeconfig", kubeconfigPath,
		"--wait",
		"--timeout", "5m",
	}

	// Add custom image repository if specified
	if imageRepository != "" {
		args = append(args, "--set", fmt.Sprintf("controllerManager.manager.image.repository=%s", imageRepository))
	}

	// Add custom image tag if specified
	if imageTag != "" {
		args = append(args, "--set", fmt.Sprintf("controllerManager.manager.image.tag=%s", imageTag))
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
