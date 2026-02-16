package helm

import (
	"context"
	"fmt"
	"os"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/strvals"
)

// InstallOptions contains configuration for a Helm chart installation
type InstallOptions struct {
	KubeconfigPath  string
	Namespace       string
	ReleaseName     string
	ChartRef        string
	Version         string
	Values          map[string]interface{}
	CreateNamespace bool
	Wait            bool
	Timeout         time.Duration
}

// Install installs a Helm chart with the given options
func Install(ctx context.Context, opts InstallOptions) error {
	// Set up Helm environment
	settings := cli.New()
	settings.KubeConfig = opts.KubeconfigPath

	// Create action configuration
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(
		settings.RESTClientGetter(),
		opts.Namespace,
		os.Getenv("HELM_DRIVER"),
		func(format string, v ...interface{}) {
			// Debug logger - prints to stdout when HELM_DEBUG is set
			if settings.Debug {
				fmt.Printf(format, v...)
			}
		},
	); err != nil {
		return fmt.Errorf("failed to initialize Helm action config: %w", err)
	}

	// Set up registry client for OCI support
	// Use stdout for output so users can see download progress
	registryClient, err := registry.NewClient(
		registry.ClientOptDebug(settings.Debug),
		registry.ClientOptWriter(os.Stdout),
	)
	if err != nil {
		return fmt.Errorf("failed to create registry client: %w", err)
	}
	actionConfig.RegistryClient = registryClient

	// Configure install action
	client := action.NewInstall(actionConfig)
	client.ReleaseName = opts.ReleaseName
	client.Namespace = opts.Namespace
	client.CreateNamespace = opts.CreateNamespace
	client.Wait = opts.Wait
	client.Timeout = opts.Timeout
	if opts.Version != "" {
		client.Version = opts.Version
	}

	// Locate and load the chart (works for both OCI and traditional repos)
	chartPath, err := client.ChartPathOptions.LocateChart(opts.ChartRef, settings)
	if err != nil {
		return fmt.Errorf("failed to locate chart %s: %w", opts.ChartRef, err)
	}

	chart, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("failed to load chart: %w", err)
	}

	// Run the install
	_, err = client.RunWithContext(ctx, chart, opts.Values)
	if err != nil {
		return fmt.Errorf("failed to install chart: %w", err)
	}

	return nil
}

// ParseSetValues parses Helm --set style key=value pairs into a values map
// Supports dot notation (e.g. "foo.bar=baz" becomes {foo: {bar: baz}})
func ParseSetValues(setValues map[string]string) (map[string]interface{}, error) {
	values := make(map[string]interface{})

	for k, v := range setValues {
		if err := strvals.ParseInto(fmt.Sprintf("%s=%s", k, v), values); err != nil {
			return nil, fmt.Errorf("failed to parse value %s=%s: %w", k, v, err)
		}
	}

	return values, nil
}
