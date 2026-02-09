package manifest

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

// ApplyURL fetches a manifest from a URL and applies all resources.
// Optional mutators are called on each object before it is applied.
func ApplyURL(ctx context.Context, client dynamic.Interface,
	mapper *restmapper.DeferredDiscoveryRESTMapper, url string,
	mutators ...func(*unstructured.Unstructured)) error {

	documents, err := FetchYAMLDocuments(url)
	if err != nil {
		return fmt.Errorf("failed to fetch manifest: %w", err)
	}

	return applyDocuments(ctx, client, mapper, documents, mutators...)
}

// ApplyBytes applies YAML manifests from raw bytes.
// The data may contain multiple YAML documents separated by "---".
// Optional mutators are called on each object before it is applied.
func ApplyBytes(ctx context.Context, client dynamic.Interface,
	mapper *restmapper.DeferredDiscoveryRESTMapper, data []byte,
	mutators ...func(*unstructured.Unstructured)) error {

	parts := strings.Split(string(data), "\n---\n")
	var documents [][]byte
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		documents = append(documents, []byte(trimmed))
	}

	return applyDocuments(ctx, client, mapper, documents, mutators...)
}

// applyDocuments decodes and applies a slice of YAML documents.
func applyDocuments(ctx context.Context, client dynamic.Interface,
	mapper *restmapper.DeferredDiscoveryRESTMapper, documents [][]byte,
	mutators ...func(*unstructured.Unstructured)) error {

	decoder := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	for i, doc := range documents {
		obj := &unstructured.Unstructured{}
		_, gvk, err := decoder.Decode(doc, nil, obj)
		if err != nil {
			return fmt.Errorf("failed to decode document %d: %w", i, err)
		}

		if gvk == nil || obj.GetKind() == "" {
			continue
		}

		for _, mutate := range mutators {
			mutate(obj)
		}

		mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			return fmt.Errorf("failed to get REST mapping for %s: %w", gvk.String(), err)
		}

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

		_, err = resourceClient.Create(ctx, obj, metav1.CreateOptions{})
		if err != nil {
			_, err = resourceClient.Update(ctx, obj, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("failed to apply %s %s: %w", obj.GetKind(), obj.GetName(), err)
			}
		}
	}

	return nil
}

// ApplyURLWithKubeconfig is a convenience wrapper that creates
// dynamic client + mapper from a kubeconfig path, then calls ApplyURL.
func ApplyURLWithKubeconfig(ctx context.Context, kubeconfigPath, url string) error {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create discovery client: %w", err)
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(discoveryClient))

	return ApplyURL(ctx, dynamicClient, mapper, url)
}
