package extensions

import (
	"context"
	"fmt"
	"time"

	"github.com/jhwagner/kueue-bench/pkg/config"
	"github.com/jhwagner/kueue-bench/pkg/helm"
	"github.com/jhwagner/kueue-bench/pkg/manifest"
	"k8s.io/utils/ptr"
)

// InstallExtensions installs all Helm chart or manifest extensions for a cluster
func InstallExtensions(ctx context.Context, kubeconfigPath string, extensions []config.Extension) error {
	for _, ext := range extensions {
		switch {
		case ext.Helm != nil:
			if err := installHelmExtension(ctx, kubeconfigPath, ext.Name, ext.Helm); err != nil {
				return fmt.Errorf("failed to install helm extension '%s': %w", ext.Name, err)
			}
		case ext.Manifest != nil:
			if err := installManifestExtension(ctx, kubeconfigPath, ext.Name, ext.Manifest); err != nil {
				return fmt.Errorf("failed to install manifest extension '%s': %w", ext.Name, err)
			}
		}
	}
	return nil
}

func installHelmExtension(ctx context.Context, kubeconfigPath, name string, helmExt *config.HelmExtension) error {
	fmt.Printf("Installing extension '%s' (helm: %s)...\n", name, helmExt.Chart)

	// Determine release name (use extension name if not specified)
	releaseName := helmExt.ReleaseName
	if releaseName == "" {
		releaseName = name
	}

	// Determine namespace (default to "default" if not specified)
	namespace := helmExt.Namespace
	if namespace == "" {
		namespace = "default"
	}

	// Parse timeout with better error handling
	timeout := 5 * time.Minute
	if helmExt.Timeout != "" {
		parsed, err := time.ParseDuration(helmExt.Timeout)
		if err != nil {
			fmt.Printf("Warning: invalid timeout '%s', using default 5m: %v\n", helmExt.Timeout, err)
		} else {
			timeout = parsed
		}
	}

	// Parse --set values using strvals to support dot notation (e.g. "foo.bar=baz")
	var values map[string]interface{}
	if len(helmExt.Set) > 0 {
		var err error
		values, err = helm.ParseSetValues(helmExt.Set)
		if err != nil {
			return fmt.Errorf("failed to parse values: %w", err)
		}
	}

	// Install the chart
	if err := helm.Install(ctx, helm.InstallOptions{
		KubeconfigPath:  kubeconfigPath,
		Namespace:       namespace,
		ReleaseName:     releaseName,
		ChartRef:        helmExt.Chart,
		Version:         helmExt.Version,
		Values:          values,
		CreateNamespace: ptr.Deref(helmExt.CreateNamespace, true),
		Wait:            ptr.Deref(helmExt.Wait, true),
		Timeout:         timeout,
	}); err != nil {
		return err
	}

	fmt.Printf("✓ Extension '%s' installed successfully\n", name)
	return nil
}

func installManifestExtension(ctx context.Context, kubeconfigPath, name string, m *config.ManifestExtension) error {
	fmt.Printf("Installing extension '%s' (manifest: %s)...\n", name, m.URL)

	if err := manifest.ApplyURLWithKubeconfig(ctx, kubeconfigPath, m.URL); err != nil {
		return fmt.Errorf("failed to apply manifest: %w", err)
	}

	fmt.Printf("✓ Extension '%s' installed successfully\n", name)
	return nil
}
