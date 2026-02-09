package kwok

import (
	"context"
	"fmt"
	"time"

	"github.com/jhwagner/kueue-bench/pkg/manifest"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	// Default Kwok version
	DefaultKwokVersion = "v0.7.0"

	// Kwok controller manifest URL
	kwokManifestURLTemplate = "https://github.com/kubernetes-sigs/kwok/releases/download/%s/kwok.yaml"
)

// Install installs Kwok into the cluster
func Install(ctx context.Context, kubeconfigPath string, version string) error {
	if version == "" {
		version = DefaultKwokVersion
	}

	fmt.Printf("Installing Kwok %s...\n", version)

	// Create Kubernetes clients
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	// Create discovery client and mapper for GVR resolution
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create discovery client: %w", err)
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(discoveryClient))

	// Apply Kwok controller manifest with hostNetwork patch
	kwokURL := fmt.Sprintf(kwokManifestURLTemplate, version)
	hostNetworkMutator := func(obj *unstructured.Unstructured) {
		if obj.GetKind() == "Deployment" && obj.GetName() == "kwok-controller" {
			unstructured.SetNestedField(obj.Object, true, "spec", "template", "spec", "hostNetwork")
		}
	}
	if err := manifest.ApplyURL(ctx, dynamicClient, mapper, kwokURL, hostNetworkMutator); err != nil {
		return fmt.Errorf("failed to install Kwok controller: %w", err)
	}

	// Reset discovery cache so mapper can discover newly created Stage CRD
	mapper.Reset()

	// Apply embedded Kwok stages for node lifecycle and pod completion
	if err := installStages(ctx, dynamicClient, mapper); err != nil {
		return fmt.Errorf("failed to install Kwok stages: %w", err)
	}

	// Wait for Kwok controller to be ready
	fmt.Println("Waiting for Kwok controller to be ready...")
	if err := waitForDeployment(ctx, clientset, "kube-system", "kwok-controller"); err != nil {
		return fmt.Errorf("Kwok controller failed to become ready: %w", err)
	}

	fmt.Println("✓ Kwok installed successfully")
	return nil
}

// waitForDeployment waits for a deployment to be ready
func waitForDeployment(ctx context.Context, clientset kubernetes.Interface, namespace, name string) error {
	return wait.PollUntilContextTimeout(ctx, 2*time.Second, 120*time.Second, true, func(ctx context.Context) (bool, error) {
		deployment, err := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
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
