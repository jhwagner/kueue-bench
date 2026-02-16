package kueue

import (
	"context"
	"fmt"
	"time"

	"github.com/jhwagner/kueue-bench/pkg/helm"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/clientcmd"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
	kueueclientset "sigs.k8s.io/kueue/client-go/clientset/versioned"
)

const (
	// DefaultKueueVersion is the default Kueue version to install
	DefaultKueueVersion = "0.15.2"

	// Kueue Helm OCI registry configuration
	kueueHelmRegistryURL = "oci://registry.k8s.io/kueue/charts/kueue"

	// Kueue installation details
	kueueNamespace   = "kueue-system"
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

	// Wait for the webhook to be serving before returning, otherwise callers
	// creating Kueue objects may hit "connection refused" on the webhook
	fmt.Println("Waiting for Kueue webhook to be ready...")
	if err := waitForWebhookReady(ctx, kubeconfigPath); err != nil {
		return fmt.Errorf("Kueue webhook failed to become ready: %w", err)
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

// waitForWebhookReady probes the Kueue webhook by performing a dry-run create of a
// ResourceFlavor. This exercises the full webhook path (API server → Service routing →
// Pod → webhook handler) and only succeeds when the webhook is truly serving.
func waitForWebhookReady(ctx context.Context, kubeconfigPath string) error {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	client, err := kueueclientset.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create Kueue clientset: %w", err)
	}

	probe := &kueuev1beta1.ResourceFlavor{
		ObjectMeta: metav1.ObjectMeta{Name: "webhook-probe"},
	}
	dryRun := metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}}

	return wait.PollUntilContextTimeout(ctx, 2*time.Second, 180*time.Second, true, func(ctx context.Context) (bool, error) {
		_, err := client.KueueV1beta1().ResourceFlavors().Create(ctx, probe, dryRun)
		return err == nil, nil
	})
}
