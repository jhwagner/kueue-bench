package kueue

import (
	"testing"

	"github.com/jhwagner/kueue-bench/pkg/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	kueue "sigs.k8s.io/kueue/apis/kueue/v1beta1"
)

func TestBuildCohort(t *testing.T) {
	tests := []struct {
		name    string
		input   config.Cohort
		checkFn func(*testing.T, *kueue.Cohort)
	}{
		{
			name: "simple root cohort",
			input: config.Cohort{
				Name: "platform",
			},
			checkFn: func(t *testing.T, cohort *kueue.Cohort) {
				if cohort.Name != "platform" {
					t.Errorf("expected name 'platform', got '%s'", cohort.Name)
				}
				if cohort.Spec.ParentName != "" {
					t.Errorf("expected empty parent name, got '%s'", cohort.Spec.ParentName)
				}
			},
		},
		{
			name: "cohort with parent",
			input: config.Cohort{
				Name:       "team-a",
				ParentName: "platform",
			},
			checkFn: func(t *testing.T, cohort *kueue.Cohort) {
				if cohort.Name != "team-a" {
					t.Errorf("expected name 'team-a', got '%s'", cohort.Name)
				}
				if string(cohort.Spec.ParentName) != "platform" {
					t.Errorf("expected parent name 'platform', got '%s'", cohort.Spec.ParentName)
				}
			},
		},
		{
			name: "cohort with fair sharing",
			input: config.Cohort{
				Name:       "team-a",
				ParentName: "platform",
				FairSharing: &config.FairSharing{
					Weight: 2,
				},
			},
			checkFn: func(t *testing.T, cohort *kueue.Cohort) {
				if cohort.Spec.FairSharing == nil {
					t.Fatal("expected FairSharing to be set")
				}
				if cohort.Spec.FairSharing.Weight == nil {
					t.Fatal("expected Weight to be set")
				}
				expectedWeight := resource.MustParse("2")
				if cohort.Spec.FairSharing.Weight.Cmp(expectedWeight) != 0 {
					t.Errorf("expected weight 2, got %v", cohort.Spec.FairSharing.Weight)
				}
			},
		},
		{
			name: "cohort with zero weight deprioritizes cohort",
			input: config.Cohort{
				Name:       "low-priority",
				ParentName: "platform",
				FairSharing: &config.FairSharing{
					Weight: 0,
				},
			},
			checkFn: func(t *testing.T, cohort *kueue.Cohort) {
				if cohort.Spec.FairSharing == nil {
					t.Fatal("expected FairSharing to be set")
				}
				if cohort.Spec.FairSharing.Weight == nil {
					t.Fatal("expected Weight to be set")
				}
				expectedWeight := resource.MustParse("0")
				if cohort.Spec.FairSharing.Weight.Cmp(expectedWeight) != 0 {
					t.Errorf("expected weight 0, got %v", cohort.Spec.FairSharing.Weight)
				}
			},
		},
		{
			name: "cohort with resource groups",
			input: config.Cohort{
				Name: "platform",
				ResourceGroups: []config.ResourceGroup{
					{
						CoveredResources: []string{"cpu", "memory"},
						Flavors: []config.FlavorQuotas{
							{
								Name: "default",
								Resources: []config.Resource{
									{
										Name:         "cpu",
										NominalQuota: "100",
									},
								},
							},
						},
					},
				},
			},
			checkFn: func(t *testing.T, cohort *kueue.Cohort) {
				if len(cohort.Spec.ResourceGroups) != 1 {
					t.Fatalf("expected 1 resource group, got %d", len(cohort.Spec.ResourceGroups))
				}
				rg := cohort.Spec.ResourceGroups[0]
				if len(rg.CoveredResources) != 2 {
					t.Errorf("expected 2 covered resources, got %d", len(rg.CoveredResources))
				}
				if len(rg.Flavors) != 1 {
					t.Errorf("expected 1 flavor, got %d", len(rg.Flavors))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildCohort(tt.input)
			tt.checkFn(t, result)
		})
	}
}

func TestBuildResourceFlavor(t *testing.T) {
	tests := []struct {
		name    string
		input   config.ResourceFlavor
		checkFn func(*testing.T, *kueue.ResourceFlavor)
	}{
		{
			name: "simple resource flavor",
			input: config.ResourceFlavor{
				Name: "default-flavor",
				NodeLabels: map[string]string{
					"node-type": "cpu",
				},
			},
			checkFn: func(t *testing.T, rf *kueue.ResourceFlavor) {
				if rf.Name != "default-flavor" {
					t.Errorf("expected name 'default-flavor', got '%s'", rf.Name)
				}
				if rf.Spec.NodeLabels["node-type"] != "cpu" {
					t.Errorf("expected node-type label 'cpu', got '%s'", rf.Spec.NodeLabels["node-type"])
				}
			},
		},
		{
			name: "resource flavor with tolerations",
			input: config.ResourceFlavor{
				Name: "gpu-flavor",
				NodeLabels: map[string]string{
					"node-type": "gpu",
				},
				Tolerations: []corev1.Toleration{
					{
						Key:      "nvidia.com/gpu",
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
			},
			checkFn: func(t *testing.T, rf *kueue.ResourceFlavor) {
				if len(rf.Spec.Tolerations) != 1 {
					t.Fatalf("expected 1 toleration, got %d", len(rf.Spec.Tolerations))
				}
				if rf.Spec.Tolerations[0].Key != "nvidia.com/gpu" {
					t.Errorf("expected toleration key 'nvidia.com/gpu', got '%s'", rf.Spec.Tolerations[0].Key)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildResourceFlavor(tt.input)
			tt.checkFn(t, result)
		})
	}
}

func TestBuildClusterQueue(t *testing.T) {
	tests := []struct {
		name    string
		input   config.ClusterQueue
		checkFn func(*testing.T, *kueue.ClusterQueue)
	}{
		{
			name: "simple cluster queue",
			input: config.ClusterQueue{
				Name:   "main-queue",
				Cohort: "platform",
				ResourceGroups: []config.ResourceGroup{
					{
						CoveredResources: []string{"cpu", "memory"},
						Flavors: []config.FlavorQuotas{
							{
								Name: "default-flavor",
								Resources: []config.Resource{
									{
										Name:         "cpu",
										NominalQuota: "100",
									},
									{
										Name:         "memory",
										NominalQuota: "400Gi",
									},
								},
							},
						},
					},
				},
			},
			checkFn: func(t *testing.T, cq *kueue.ClusterQueue) {
				if cq.Name != "main-queue" {
					t.Errorf("expected name 'main-queue', got '%s'", cq.Name)
				}
				if cq.Spec.Cohort != "platform" {
					t.Errorf("expected cohort 'platform', got '%s'", cq.Spec.Cohort)
				}
				if len(cq.Spec.ResourceGroups) != 1 {
					t.Fatalf("expected 1 resource group, got %d", len(cq.Spec.ResourceGroups))
				}
				rg := cq.Spec.ResourceGroups[0]
				if len(rg.CoveredResources) != 2 {
					t.Errorf("expected 2 covered resources, got %d", len(rg.CoveredResources))
				}
				if len(rg.Flavors) != 1 {
					t.Fatalf("expected 1 flavor, got %d", len(rg.Flavors))
				}
				if string(rg.Flavors[0].Name) != "default-flavor" {
					t.Errorf("expected flavor 'default-flavor', got '%s'", rg.Flavors[0].Name)
				}
				if len(rg.Flavors[0].Resources) != 2 {
					t.Errorf("expected 2 resources, got %d", len(rg.Flavors[0].Resources))
				}
			},
		},
		{
			name: "cluster queue with borrowing/lending limits",
			input: config.ClusterQueue{
				Name: "team-queue",
				ResourceGroups: []config.ResourceGroup{
					{
						CoveredResources: []string{"cpu"},
						Flavors: []config.FlavorQuotas{
							{
								Name: "default-flavor",
								Resources: []config.Resource{
									{
										Name:           "cpu",
										NominalQuota:   "100",
										BorrowingLimit: "50",
										LendingLimit:   "25",
									},
								},
							},
						},
					},
				},
			},
			checkFn: func(t *testing.T, cq *kueue.ClusterQueue) {
				res := cq.Spec.ResourceGroups[0].Flavors[0].Resources[0]
				if res.NominalQuota.Cmp(resource.MustParse("100")) != 0 {
					t.Errorf("expected nominal quota 100, got %v", res.NominalQuota)
				}
				if res.BorrowingLimit == nil || res.BorrowingLimit.Cmp(resource.MustParse("50")) != 0 {
					t.Errorf("expected borrowing limit 50, got %v", res.BorrowingLimit)
				}
				if res.LendingLimit == nil || res.LendingLimit.Cmp(resource.MustParse("25")) != 0 {
					t.Errorf("expected lending limit 25, got %v", res.LendingLimit)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildClusterQueue(tt.input)
			tt.checkFn(t, result)
		})
	}
}

func TestBuildLocalQueue(t *testing.T) {
	tests := []struct {
		name    string
		input   config.LocalQueue
		checkFn func(*testing.T, *kueue.LocalQueue)
	}{
		{
			name: "local queue with explicit namespace",
			input: config.LocalQueue{
				Name:         "user-queue",
				Namespace:    "team-a",
				ClusterQueue: "main-queue",
			},
			checkFn: func(t *testing.T, lq *kueue.LocalQueue) {
				if lq.Name != "user-queue" {
					t.Errorf("expected name 'user-queue', got '%s'", lq.Name)
				}
				if lq.Namespace != "team-a" {
					t.Errorf("expected namespace 'team-a', got '%s'", lq.Namespace)
				}
				if string(lq.Spec.ClusterQueue) != "main-queue" {
					t.Errorf("expected cluster queue 'main-queue', got '%s'", lq.Spec.ClusterQueue)
				}
			},
		},
		{
			name: "local queue defaults to default namespace",
			input: config.LocalQueue{
				Name:         "default-queue",
				ClusterQueue: "main-queue",
			},
			checkFn: func(t *testing.T, lq *kueue.LocalQueue) {
				if lq.Namespace != "default" {
					t.Errorf("expected namespace 'default', got '%s'", lq.Namespace)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildLocalQueue(tt.input)
			tt.checkFn(t, result)
		})
	}
}

func TestBuildWorkloadPriorityClass(t *testing.T) {
	tests := []struct {
		name    string
		input   config.WorkloadPriorityClass
		checkFn func(*testing.T, *kueue.WorkloadPriorityClass)
	}{
		{
			name: "priority class with description",
			input: config.WorkloadPriorityClass{
				Name:        "high-priority",
				Value:       1000,
				Description: "High priority workloads",
			},
			checkFn: func(t *testing.T, wpc *kueue.WorkloadPriorityClass) {
				if wpc.Name != "high-priority" {
					t.Errorf("expected name 'high-priority', got '%s'", wpc.Name)
				}
				if wpc.Value != 1000 {
					t.Errorf("expected value 1000, got %d", wpc.Value)
				}
				if wpc.Description != "High priority workloads" {
					t.Errorf("expected description 'High priority workloads', got '%s'", wpc.Description)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildWorkloadPriorityClass(tt.input)
			tt.checkFn(t, result)
		})
	}
}

func TestGetUniqueNamespaces(t *testing.T) {
	tests := []struct {
		name     string
		input    []config.LocalQueue
		expected int
	}{
		{
			name: "multiple queues in different namespaces",
			input: []config.LocalQueue{
				{Name: "q1", Namespace: "team-a"},
				{Name: "q2", Namespace: "team-b"},
				{Name: "q3", Namespace: "team-a"}, // duplicate
			},
			expected: 2, // team-a, team-b
		},
		{
			name: "default namespace is excluded",
			input: []config.LocalQueue{
				{Name: "q1", Namespace: "default"},
				{Name: "q2", Namespace: "team-a"},
			},
			expected: 1, // only team-a
		},
		{
			name: "empty namespace defaults to default and is excluded",
			input: []config.LocalQueue{
				{Name: "q1", Namespace: ""},
				{Name: "q2", Namespace: "team-a"},
			},
			expected: 1, // only team-a
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getUniqueNamespaces(tt.input)
			if len(result) != tt.expected {
				t.Errorf("expected %d namespaces, got %d: %v", tt.expected, len(result), result)
			}
		})
	}
}
