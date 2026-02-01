package config

import (
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestExpandWorkerSets(t *testing.T) {
	tests := []struct {
		name        string
		workerSets  []WorkerSet
		want        []ClusterConfig
		wantErr     bool
		errContains string
	}{
		{
			name:       "empty workerSets",
			workerSets: nil,
			want:       nil,
			wantErr:    false,
		},
		{
			name: "single worker derives labels and quotas",
			workerSets: []WorkerSet{
				{
					Name: "gpu-workers",
					ResourceFlavors: []WorkerSetFlavor{
						{Name: "gpu-a100", NodePoolRef: "gpu-pool"},
					},
					ClusterQueues: []WorkerSetClusterQueue{
						{
							Name: "team-cq",
							ResourceGroups: []WorkerSetResourceGroup{
								{
									CoveredResources: []string{"nvidia.com/gpu", "cpu"},
									Flavors:          []WorkerSetFlavorRef{{Name: "gpu-a100"}},
								},
							},
						},
					},
					LocalQueues: []LocalQueue{
						{Name: "team-lq", Namespace: "team-ns", ClusterQueue: "team-cq"},
					},
					Workers: []Worker{
						{
							Name: "eks-worker",
							NodePools: []NodePool{
								{
									Name:  "gpu-pool",
									Count: 100,
									Resources: map[string]string{
										"nvidia.com/gpu": "8",
										"cpu":            "96",
									},
									Labels: map[string]string{
										"eks.amazonaws.com/gpu": "b200",
									},
								},
							},
						},
					},
				},
			},
			want: []ClusterConfig{
				{
					Name: "eks-worker",
					Role: "worker",
					NodePools: []NodePool{
						{
							Name:  "gpu-pool",
							Count: 100,
							Resources: map[string]string{
								"nvidia.com/gpu": "8",
								"cpu":            "96",
							},
							Labels: map[string]string{
								"eks.amazonaws.com/gpu": "b200",
							},
						},
					},
					Kueue: &KueueConfig{
						ResourceFlavors: []ResourceFlavor{
							{
								Name:       "gpu-a100",
								NodeLabels: map[string]string{"eks.amazonaws.com/gpu": "b200"},
							},
						},
						ClusterQueues: []ClusterQueue{
							{
								Name: "team-cq",
								ResourceGroups: []ResourceGroup{
									{
										CoveredResources: []string{"nvidia.com/gpu", "cpu"},
										Flavors: []FlavorQuotas{
											{
												Name: "gpu-a100",
												Resources: []Resource{
													{Name: "nvidia.com/gpu", NominalQuota: "800"},
													{Name: "cpu", NominalQuota: "9600"},
												},
											},
										},
									},
								},
							},
						},
						LocalQueues: []LocalQueue{
							{Name: "team-lq", Namespace: "team-ns", ClusterQueue: "team-cq"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "two workers get different labels and quotas",
			workerSets: []WorkerSet{
				{
					Name: "gpu-workers",
					ResourceFlavors: []WorkerSetFlavor{
						{Name: "gpu-flavor", NodePoolRef: "gpu-pool"},
					},
					ClusterQueues: []WorkerSetClusterQueue{
						{
							Name: "team-cq",
							ResourceGroups: []WorkerSetResourceGroup{
								{
									CoveredResources: []string{"nvidia.com/gpu"},
									Flavors:          []WorkerSetFlavorRef{{Name: "gpu-flavor"}},
								},
							},
						},
					},
					Workers: []Worker{
						{
							Name: "eks-worker",
							NodePools: []NodePool{
								{
									Name:      "gpu-pool",
									Count:     100,
									Resources: map[string]string{"nvidia.com/gpu": "8"},
									Labels:    map[string]string{"eks.amazonaws.com/gpu": "b200"},
								},
							},
						},
						{
							Name: "gke-worker",
							NodePools: []NodePool{
								{
									Name:      "gpu-pool",
									Count:     50,
									Resources: map[string]string{"nvidia.com/gpu": "8"},
									Labels:    map[string]string{"cloud.google.com/gke-accelerator": "nvidia-b200"},
								},
							},
						},
					},
				},
			},
			want: []ClusterConfig{
				{
					Name: "eks-worker",
					Role: "worker",
					NodePools: []NodePool{
						{
							Name:      "gpu-pool",
							Count:     100,
							Resources: map[string]string{"nvidia.com/gpu": "8"},
							Labels:    map[string]string{"eks.amazonaws.com/gpu": "b200"},
						},
					},
					Kueue: &KueueConfig{
						ResourceFlavors: []ResourceFlavor{
							{Name: "gpu-flavor", NodeLabels: map[string]string{"eks.amazonaws.com/gpu": "b200"}},
						},
						ClusterQueues: []ClusterQueue{
							{
								Name: "team-cq",
								ResourceGroups: []ResourceGroup{
									{
										CoveredResources: []string{"nvidia.com/gpu"},
										Flavors: []FlavorQuotas{
											{Name: "gpu-flavor", Resources: []Resource{{Name: "nvidia.com/gpu", NominalQuota: "800"}}},
										},
									},
								},
							},
						},
					},
				},
				{
					Name: "gke-worker",
					Role: "worker",
					NodePools: []NodePool{
						{
							Name:      "gpu-pool",
							Count:     50,
							Resources: map[string]string{"nvidia.com/gpu": "8"},
							Labels:    map[string]string{"cloud.google.com/gke-accelerator": "nvidia-b200"},
						},
					},
					Kueue: &KueueConfig{
						ResourceFlavors: []ResourceFlavor{
							{Name: "gpu-flavor", NodeLabels: map[string]string{"cloud.google.com/gke-accelerator": "nvidia-b200"}},
						},
						ClusterQueues: []ClusterQueue{
							{
								Name: "team-cq",
								ResourceGroups: []ResourceGroup{
									{
										CoveredResources: []string{"nvidia.com/gpu"},
										Flavors: []FlavorQuotas{
											{Name: "gpu-flavor", Resources: []Resource{{Name: "nvidia.com/gpu", NominalQuota: "400"}}},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "taints become tolerations",
			workerSets: []WorkerSet{
				{
					Name: "gpu-workers",
					ResourceFlavors: []WorkerSetFlavor{
						{Name: "gpu-flavor", NodePoolRef: "gpu-pool"},
					},
					ClusterQueues: []WorkerSetClusterQueue{
						{
							Name: "team-cq",
							ResourceGroups: []WorkerSetResourceGroup{
								{
									CoveredResources: []string{"nvidia.com/gpu"},
									Flavors:          []WorkerSetFlavorRef{{Name: "gpu-flavor"}},
								},
							},
						},
					},
					Workers: []Worker{
						{
							Name: "worker-1",
							NodePools: []NodePool{
								{
									Name:      "gpu-pool",
									Count:     10,
									Resources: map[string]string{"nvidia.com/gpu": "4"},
									Taints: []Taint{
										{Key: "nvidia.com/gpu", Value: "true", Effect: "NoSchedule"},
									},
								},
							},
						},
					},
				},
			},
			want: []ClusterConfig{
				{
					Name: "worker-1",
					Role: "worker",
					NodePools: []NodePool{
						{
							Name:      "gpu-pool",
							Count:     10,
							Resources: map[string]string{"nvidia.com/gpu": "4"},
							Taints:    []Taint{{Key: "nvidia.com/gpu", Value: "true", Effect: "NoSchedule"}},
						},
					},
					Kueue: &KueueConfig{
						ResourceFlavors: []ResourceFlavor{
							{
								Name: "gpu-flavor",
								Tolerations: []corev1.Toleration{
									{
										Key:      "nvidia.com/gpu",
										Operator: corev1.TolerationOpEqual,
										Value:    "true",
										Effect:   corev1.TaintEffectNoSchedule,
									},
								},
							},
						},
						ClusterQueues: []ClusterQueue{
							{
								Name: "team-cq",
								ResourceGroups: []ResourceGroup{
									{
										CoveredResources: []string{"nvidia.com/gpu"},
										Flavors: []FlavorQuotas{
											{Name: "gpu-flavor", Resources: []Resource{{Name: "nvidia.com/gpu", NominalQuota: "40"}}},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "CQ structural fields are preserved",
			workerSets: []WorkerSet{
				{
					Name: "gpu-workers",
					ResourceFlavors: []WorkerSetFlavor{
						{Name: "gpu-flavor", NodePoolRef: "gpu-pool"},
					},
					ClusterQueues: []WorkerSetClusterQueue{
						{
							Name:   "team-cq",
							Cohort: "platform",
							Preemption: &PreemptionConfig{
								WithinClusterQueue: "LowerPriority",
							},
							FairSharing: &FairSharing{Weight: 2},
							ResourceGroups: []WorkerSetResourceGroup{
								{
									CoveredResources: []string{"nvidia.com/gpu"},
									Flavors:          []WorkerSetFlavorRef{{Name: "gpu-flavor"}},
								},
							},
						},
					},
					Workers: []Worker{
						{
							Name: "worker-1",
							NodePools: []NodePool{
								{
									Name:      "gpu-pool",
									Count:     10,
									Resources: map[string]string{"nvidia.com/gpu": "4"},
								},
							},
						},
					},
				},
			},
			want: []ClusterConfig{
				{
					Name: "worker-1",
					Role: "worker",
					NodePools: []NodePool{
						{Name: "gpu-pool", Count: 10, Resources: map[string]string{"nvidia.com/gpu": "4"}},
					},
					Kueue: &KueueConfig{
						ResourceFlavors: []ResourceFlavor{
							{Name: "gpu-flavor"},
						},
						ClusterQueues: []ClusterQueue{
							{
								Name:   "team-cq",
								Cohort: "platform",
								Preemption: &PreemptionConfig{
									WithinClusterQueue: "LowerPriority",
								},
								FairSharing: &FairSharing{Weight: 2},
								ResourceGroups: []ResourceGroup{
									{
										CoveredResources: []string{"nvidia.com/gpu"},
										Flavors: []FlavorQuotas{
											{Name: "gpu-flavor", Resources: []Resource{{Name: "nvidia.com/gpu", NominalQuota: "40"}}},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "memory quota uses binary suffix",
			workerSets: []WorkerSet{
				{
					Name: "mem-workers",
					ResourceFlavors: []WorkerSetFlavor{
						{Name: "mem-flavor", NodePoolRef: "mem-pool"},
					},
					ClusterQueues: []WorkerSetClusterQueue{
						{
							Name: "mem-cq",
							ResourceGroups: []WorkerSetResourceGroup{
								{
									CoveredResources: []string{"memory"},
									Flavors:          []WorkerSetFlavorRef{{Name: "mem-flavor"}},
								},
							},
						},
					},
					Workers: []Worker{
						{
							Name: "worker-1",
							NodePools: []NodePool{
								{
									Name:      "mem-pool",
									Count:     4,
									Resources: map[string]string{"memory": "768Gi"},
								},
							},
						},
					},
				},
			},
			want: []ClusterConfig{
				{
					Name: "worker-1",
					Role: "worker",
					NodePools: []NodePool{
						{Name: "mem-pool", Count: 4, Resources: map[string]string{"memory": "768Gi"}},
					},
					Kueue: &KueueConfig{
						ResourceFlavors: []ResourceFlavor{{Name: "mem-flavor"}},
						ClusterQueues: []ClusterQueue{
							{
								Name: "mem-cq",
								ResourceGroups: []ResourceGroup{
									{
										CoveredResources: []string{"memory"},
										Flavors: []FlavorQuotas{
											{Name: "mem-flavor", Resources: []Resource{{Name: "memory", NominalQuota: "3Ti"}}},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "error: nodePoolRef not found",
			workerSets: []WorkerSet{
				{
					Name: "gpu-workers",
					ResourceFlavors: []WorkerSetFlavor{
						{Name: "gpu-flavor", NodePoolRef: "missing-pool"},
					},
					ClusterQueues: []WorkerSetClusterQueue{
						{
							Name: "team-cq",
							ResourceGroups: []WorkerSetResourceGroup{
								{
									CoveredResources: []string{"nvidia.com/gpu"},
									Flavors:          []WorkerSetFlavorRef{{Name: "gpu-flavor"}},
								},
							},
						},
					},
					Workers: []Worker{
						{
							Name: "worker-1",
							NodePools: []NodePool{
								{Name: "gpu-pool", Count: 10, Resources: map[string]string{"nvidia.com/gpu": "4"}},
							},
						},
					},
				},
			},
			wantErr:     true,
			errContains: "nodePoolRef \"missing-pool\" not found",
		},
		{
			name: "error: covered resource not in pool",
			workerSets: []WorkerSet{
				{
					Name: "gpu-workers",
					ResourceFlavors: []WorkerSetFlavor{
						{Name: "gpu-flavor", NodePoolRef: "gpu-pool"},
					},
					ClusterQueues: []WorkerSetClusterQueue{
						{
							Name: "team-cq",
							ResourceGroups: []WorkerSetResourceGroup{
								{
									CoveredResources: []string{"nvidia.com/gpu", "memory"},
									Flavors:          []WorkerSetFlavorRef{{Name: "gpu-flavor"}},
								},
							},
						},
					},
					Workers: []Worker{
						{
							Name: "worker-1",
							NodePools: []NodePool{
								{Name: "gpu-pool", Count: 10, Resources: map[string]string{"nvidia.com/gpu": "4"}},
							},
						},
					},
				},
			},
			wantErr:     true,
			errContains: "covered resource \"memory\" not found in node pool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandWorkerSets(tt.workerSets)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpandWorkerSets() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if tt.errContains != "" && err != nil {
					if !strings.Contains(err.Error(), tt.errContains) {
						t.Errorf("ExpandWorkerSets() error = %v, expected to contain %q", err, tt.errContains)
					}
				}
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExpandWorkerSets() =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}
