package extensions

import (
	"reflect"
	"testing"

	"github.com/jhwagner/kueue-bench/pkg/config"
)

func boolPtr(b bool) *bool { return &b }

func TestBuildHelmArgs(t *testing.T) {
	tests := []struct {
		name     string
		kube     string
		extName  string
		helm     *config.HelmExtension
		expected []string
	}{
		{
			name:    "minimal",
			kube:    "/path/to/kubeconfig",
			extName: "jobset",
			helm: &config.HelmExtension{
				Chart: "oci://registry.k8s.io/jobset/charts/jobset",
			},
			expected: []string{
				"install", "jobset", "oci://registry.k8s.io/jobset/charts/jobset",
				"--create-namespace",
				"--kubeconfig", "/path/to/kubeconfig",
				"--wait",
				"--timeout", "5m",
			},
		},
		{
			name:    "all fields",
			kube:    "/path/to/kubeconfig",
			extName: "jobset",
			helm: &config.HelmExtension{
				Chart:       "oci://registry.k8s.io/jobset/charts/jobset",
				Version:     "0.11.0",
				Namespace:   "jobset-system",
				ReleaseName: "my-jobset",
				Timeout:     "10m",
				Set: map[string]string{
					"key1": "val1",
					"key2": "val2",
				},
			},
			expected: []string{
				"install", "my-jobset", "oci://registry.k8s.io/jobset/charts/jobset",
				"--version", "0.11.0",
				"--namespace", "jobset-system",
				"--create-namespace",
				"--kubeconfig", "/path/to/kubeconfig",
				"--wait",
				"--timeout", "10m",
				"--set", "key1=val1",
				"--set", "key2=val2",
			},
		},
		{
			name:    "createNamespace false",
			kube:    "/path/to/kubeconfig",
			extName: "ext",
			helm: &config.HelmExtension{
				Chart:           "oci://example.com/chart",
				CreateNamespace: boolPtr(false),
			},
			expected: []string{
				"install", "ext", "oci://example.com/chart",
				"--kubeconfig", "/path/to/kubeconfig",
				"--wait",
				"--timeout", "5m",
			},
		},
		{
			name:    "wait false",
			kube:    "/path/to/kubeconfig",
			extName: "ext",
			helm: &config.HelmExtension{
				Chart: "oci://example.com/chart",
				Wait:  boolPtr(false),
			},
			expected: []string{
				"install", "ext", "oci://example.com/chart",
				"--create-namespace",
				"--kubeconfig", "/path/to/kubeconfig",
				"--timeout", "5m",
			},
		},
		{
			name:    "releaseName override",
			kube:    "/path/to/kubeconfig",
			extName: "my-ext",
			helm: &config.HelmExtension{
				Chart:       "oci://example.com/chart",
				ReleaseName: "custom-release",
			},
			expected: []string{
				"install", "custom-release", "oci://example.com/chart",
				"--create-namespace",
				"--kubeconfig", "/path/to/kubeconfig",
				"--wait",
				"--timeout", "5m",
			},
		},
		{
			name:    "custom timeout",
			kube:    "/path/to/kubeconfig",
			extName: "ext",
			helm: &config.HelmExtension{
				Chart:   "oci://example.com/chart",
				Timeout: "15m",
			},
			expected: []string{
				"install", "ext", "oci://example.com/chart",
				"--create-namespace",
				"--kubeconfig", "/path/to/kubeconfig",
				"--wait",
				"--timeout", "15m",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildHelmArgs(tt.kube, tt.extName, tt.helm)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("buildHelmArgs() =\n  %v\nwant:\n  %v", got, tt.expected)
			}
		})
	}
}
