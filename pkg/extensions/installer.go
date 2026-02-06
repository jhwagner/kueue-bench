package extensions

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"

	"github.com/jhwagner/kueue-bench/pkg/config"
	"github.com/jhwagner/kueue-bench/pkg/manifest"
)

// InstallExtensions installs all Helm chart or manifest extensions for a cluster
func InstallExtensions(ctx context.Context, kubeconfigPath string, extensions []config.Extension) error {
	// If any extensions require Helm, check that Helm is installed
	hasHelm := false
	for _, ext := range extensions {
		if ext.Helm != nil {
			hasHelm = true
			break
		}
	}
	if hasHelm {
		if err := checkHelmInstalled(); err != nil {
			return err
		}
	}

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

func installHelmExtension(ctx context.Context, kubeconfigPath, name string, helm *config.HelmExtension) error {
	fmt.Printf("Installing extension '%s' (helm: %s)...\n", name, helm.Chart)

	args := buildHelmArgs(kubeconfigPath, name, helm)

	cmd := exec.CommandContext(ctx, "helm", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm install failed: %w", err)
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

func buildHelmArgs(kubeconfigPath, name string, helm *config.HelmExtension) []string {
	releaseName := name
	if helm.ReleaseName != "" {
		releaseName = helm.ReleaseName
	}

	args := []string{"install", releaseName, helm.Chart}

	if helm.Version != "" {
		args = append(args, "--version", helm.Version)
	}

	if helm.Namespace != "" {
		args = append(args, "--namespace", helm.Namespace)
	}

	// Default: createNamespace = true
	if helm.CreateNamespace == nil || *helm.CreateNamespace {
		args = append(args, "--create-namespace")
	}

	args = append(args, "--kubeconfig", kubeconfigPath)

	// Default: wait = true
	if helm.Wait == nil || *helm.Wait {
		args = append(args, "--wait")
	}

	timeout := "5m"
	if helm.Timeout != "" {
		timeout = helm.Timeout
	}
	args = append(args, "--timeout", timeout)

	// Sort set keys for deterministic output
	if len(helm.Set) > 0 {
		keys := make([]string, 0, len(helm.Set))
		for k := range helm.Set {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			args = append(args, "--set", fmt.Sprintf("%s=%s", k, helm.Set[k]))
		}
	}

	return args
}

func checkHelmInstalled() error {
	_, err := exec.LookPath("helm")
	if err != nil {
		return fmt.Errorf("helm is not installed or not in PATH: %w", err)
	}
	return nil
}
