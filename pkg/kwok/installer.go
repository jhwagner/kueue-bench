package kwok

import (
	"context"
	"fmt"
	"time"

	"github.com/jhwagner/kueue-bench/pkg/manifest"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
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

	// Kwok manifest URLs
	kwokManifestURLTemplate      = "https://github.com/kubernetes-sigs/kwok/releases/download/%s/kwok.yaml"
	kwokStageManifestURLTemplate = "https://github.com/kubernetes-sigs/kwok/releases/download/%s/stage-fast.yaml"
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

	// Apply Kwok controller manifest
	kwokURL := fmt.Sprintf(kwokManifestURLTemplate, version)
	if err := applyManifestFromURL(ctx, dynamicClient, mapper, kwokURL); err != nil {
		return fmt.Errorf("failed to install Kwok controller: %w", err)
	}

	// Apply Kwok stage manifest for pod lifecycle
	stageURL := fmt.Sprintf(kwokStageManifestURLTemplate, version)
	if err := applyManifestFromURL(ctx, dynamicClient, mapper, stageURL); err != nil {
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

// applyManifestFromURL fetches a manifest from URL and applies all resources to the cluster
func applyManifestFromURL(ctx context.Context, client dynamic.Interface, mapper *restmapper.DeferredDiscoveryRESTMapper, manifestURL string) error {
	// Fetch YAML documents
	documents, err := manifest.FetchYAMLDocuments(manifestURL)
	if err != nil {
		return fmt.Errorf("failed to fetch manifest: %w", err)
	}

	// Apply each document
	decoder := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	for i, doc := range documents {
		obj := &unstructured.Unstructured{}
		_, gvk, err := decoder.Decode(doc, nil, obj)
		if err != nil {
			return fmt.Errorf("failed to decode document %d: %w", i, err)
		}

		// Skip empty objects
		if gvk == nil || obj.GetKind() == "" {
			continue
		}

		// Get GVR using discovery
		mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			return fmt.Errorf("failed to get REST mapping for %s: %w", gvk.String(), err)
		}

		// Determine if resource is namespaced
		var resourceClient dynamic.ResourceInterface
		if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			namespace := obj.GetNamespace()
			if namespace == "" {
				namespace = "default"
			}
			resourceClient = client.Resource(mapping.Resource).Namespace(namespace)
		} else {
			resourceClient = client.Resource(mapping.Resource)
		}

		// Create or update
		_, err = resourceClient.Create(ctx, obj, metav1.CreateOptions{})
		if err != nil {
			// Try update if create fails (resource might already exist)
			_, err = resourceClient.Update(ctx, obj, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("failed to apply %s %s: %w", obj.GetKind(), obj.GetName(), err)
			}
		}
	}

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
