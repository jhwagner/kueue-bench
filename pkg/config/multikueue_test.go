package config

import (
	"reflect"
	"testing"
)

func TestDeriveManagementKueueConfig(t *testing.T) {
	tests := []struct {
		name                  string
		workerSets            []WorkerSet
		expandedWorkers       []ClusterConfig
		managementKueueConfig *KueueConfig
		want                  *KueueConfig
	}{
		{
			name:            "empty workerSets returns management config as-is",
			workerSets:      nil,
			expandedWorkers: nil,
			managementKueueConfig: &KueueConfig{
				Cohorts: []Cohort{{Name: "platform"}},
			},
			want: &KueueConfig{
				Cohorts: []Cohort{{Name: "platform"}},
			},
		},
		{
			name: "single worker derives minimal flavors and CQ with summed quotas",
			workerSets: []WorkerSet{
				{
					Name: "gpu-ws",
					ResourceFlavors: []WorkerSetFlavor{
						{Name: "gpu-a100", NodePoolRef: "gpu-pool"},
					},
					ClusterQueues: []WorkerSetClusterQueue{
						{
							Name: "team-cq",
							ResourceGroups: []WorkerSetResourceGroup{
								{
									CoveredResources: []string{"nvidia.com/gpu"},
									Flavors:          []WorkerSetFlavorRef{{Name: "gpu-a100"}},
								},
							},
						},
					},
					Workers: []Worker{
						{Name: "worker-1"},
					},
				},
			},
			expandedWorkers: []ClusterConfig{
				{
					Name: "worker-1",
					Role: RoleWorker,
					Kueue: &KueueConfig{
						ResourceFlavors: []ResourceFlavor{
							{Name: "gpu-a100", NodeLabels: map[string]string{"cloud": "aws"}},
						},
						ClusterQueues: []ClusterQueue{
							{
								Name: "team-cq",
								ResourceGroups: []ResourceGroup{
									{
										CoveredResources: []string{"nvidia.com/gpu"},
										Flavors: []FlavorQuotas{
											{
												Name: "gpu-a100",
												Resources: []Resource{
													{Name: "nvidia.com/gpu", NominalQuota: "800"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			managementKueueConfig: nil,
			want: &KueueConfig{
				ResourceFlavors: []ResourceFlavor{
					{Name: "gpu-a100"}, // Minimal - no labels
				},
				ClusterQueues: []ClusterQueue{
					{
						Name: "team-cq",
						ResourceGroups: []ResourceGroup{
							{
								CoveredResources: []string{"nvidia.com/gpu"},
								Flavors: []FlavorQuotas{
									{
										Name: "gpu-a100",
										Resources: []Resource{
											{Name: "nvidia.com/gpu", NominalQuota: "800"},
										},
									},
								},
							},
						},
						AdmissionChecks: []string{"gpu-ws"}, // Auto-added
					},
				},
			},
		},
		{
			name: "multiple workers sum quotas correctly",
			workerSets: []WorkerSet{
				{
					Name: "gpu-ws",
					ResourceFlavors: []WorkerSetFlavor{
						{Name: "gpu-flavor", NodePoolRef: "gpu-pool"},
					},
					ClusterQueues: []WorkerSetClusterQueue{
						{
							Name: "team-cq",
							ResourceGroups: []WorkerSetResourceGroup{
								{
									CoveredResources: []string{"nvidia.com/gpu", "cpu"},
									Flavors:          []WorkerSetFlavorRef{{Name: "gpu-flavor"}},
								},
							},
						},
					},
					Workers: []Worker{
						{Name: "worker-1"},
						{Name: "worker-2"},
					},
				},
			},
			expandedWorkers: []ClusterConfig{
				{
					Name: "worker-1",
					Role: RoleWorker,
					Kueue: &KueueConfig{
						ClusterQueues: []ClusterQueue{
							{
								Name: "team-cq",
								ResourceGroups: []ResourceGroup{
									{
										CoveredResources: []string{"nvidia.com/gpu", "cpu"},
										Flavors: []FlavorQuotas{
											{
												Name: "gpu-flavor",
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
					},
				},
				{
					Name: "worker-2",
					Role: RoleWorker,
					Kueue: &KueueConfig{
						ClusterQueues: []ClusterQueue{
							{
								Name: "team-cq",
								ResourceGroups: []ResourceGroup{
									{
										CoveredResources: []string{"nvidia.com/gpu", "cpu"},
										Flavors: []FlavorQuotas{
											{
												Name: "gpu-flavor",
												Resources: []Resource{
													{Name: "nvidia.com/gpu", NominalQuota: "400"},
													{Name: "cpu", NominalQuota: "4800"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			managementKueueConfig: nil,
			want: &KueueConfig{
				ResourceFlavors: []ResourceFlavor{
					{Name: "gpu-flavor"},
				},
				ClusterQueues: []ClusterQueue{
					{
						Name: "team-cq",
						ResourceGroups: []ResourceGroup{
							{
								CoveredResources: []string{"nvidia.com/gpu", "cpu"},
								Flavors: []FlavorQuotas{
									{
										Name: "gpu-flavor",
										Resources: []Resource{
											{Name: "nvidia.com/gpu", NominalQuota: "1200"}, // 800 + 400
											{Name: "cpu", NominalQuota: "14400"},           // 9600 + 4800
										},
									},
								},
							},
						},
						AdmissionChecks: []string{"gpu-ws"},
					},
				},
			},
		},
		{
			name: "derives LocalQueues from WorkerSets",
			workerSets: []WorkerSet{
				{
					Name: "gpu-ws",
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
					LocalQueues: []LocalQueue{
						{Name: "team-lq", Namespace: "team-a", ClusterQueue: "team-cq"},
					},
					Workers: []Worker{{Name: "worker-1"}},
				},
			},
			expandedWorkers: []ClusterConfig{
				{
					Name: "worker-1",
					Role: RoleWorker,
					Kueue: &KueueConfig{
						ClusterQueues: []ClusterQueue{
							{
								Name: "team-cq",
								ResourceGroups: []ResourceGroup{
									{
										CoveredResources: []string{"nvidia.com/gpu"},
										Flavors: []FlavorQuotas{
											{
												Name:      "gpu-flavor",
												Resources: []Resource{{Name: "nvidia.com/gpu", NominalQuota: "800"}},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			managementKueueConfig: nil,
			want: &KueueConfig{
				ResourceFlavors: []ResourceFlavor{{Name: "gpu-flavor"}},
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
						AdmissionChecks: []string{"gpu-ws"},
					},
				},
				LocalQueues: []LocalQueue{
					{Name: "team-lq", Namespace: "team-a", ClusterQueue: "team-cq"},
				},
			},
		},
		{
			name: "merges user-defined management config with derived LocalQueues",
			workerSets: []WorkerSet{
				{
					Name: "gpu-ws",
					ResourceFlavors: []WorkerSetFlavor{
						{Name: "gpu-flavor", NodePoolRef: "gpu-pool"},
					},
					ClusterQueues: []WorkerSetClusterQueue{
						{
							Name:   "team-cq",
							Cohort: "platform",
							ResourceGroups: []WorkerSetResourceGroup{
								{
									CoveredResources: []string{"nvidia.com/gpu"},
									Flavors:          []WorkerSetFlavorRef{{Name: "gpu-flavor"}},
								},
							},
						},
					},
					LocalQueues: []LocalQueue{
						{Name: "ws-lq", Namespace: "ws-ns", ClusterQueue: "team-cq"},
					},
					Workers: []Worker{{Name: "worker-1"}},
				},
			},
			expandedWorkers: []ClusterConfig{
				{
					Name: "worker-1",
					Role: RoleWorker,
					Kueue: &KueueConfig{
						ClusterQueues: []ClusterQueue{
							{
								Name: "team-cq",
								ResourceGroups: []ResourceGroup{
									{
										CoveredResources: []string{"nvidia.com/gpu"},
										Flavors: []FlavorQuotas{
											{
												Name:      "gpu-flavor",
												Resources: []Resource{{Name: "nvidia.com/gpu", NominalQuota: "800"}},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			managementKueueConfig: &KueueConfig{
				Cohorts: []Cohort{
					{Name: "platform"},
				},
				LocalQueues: []LocalQueue{
					{Name: "mgmt-lq", Namespace: "mgmt-ns", ClusterQueue: "team-cq"},
				},
			},
			want: &KueueConfig{
				Cohorts: []Cohort{
					{Name: "platform"},
				},
				ResourceFlavors: []ResourceFlavor{
					{Name: "gpu-flavor"},
				},
				ClusterQueues: []ClusterQueue{
					{
						Name:   "team-cq",
						Cohort: "platform",
						ResourceGroups: []ResourceGroup{
							{
								CoveredResources: []string{"nvidia.com/gpu"},
								Flavors: []FlavorQuotas{
									{
										Name:      "gpu-flavor",
										Resources: []Resource{{Name: "nvidia.com/gpu", NominalQuota: "800"}},
									},
								},
							},
						},
						AdmissionChecks: []string{"gpu-ws"},
					},
				},
				LocalQueues: []LocalQueue{
					{Name: "ws-lq", Namespace: "ws-ns", ClusterQueue: "team-cq"},
					{Name: "mgmt-lq", Namespace: "mgmt-ns", ClusterQueue: "team-cq"},
				},
			},
		},
		{
			name: "preserves CQ structural fields (preemption, fairSharing)",
			workerSets: []WorkerSet{
				{
					Name: "gpu-ws",
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
					Workers: []Worker{{Name: "worker-1"}},
				},
			},
			expandedWorkers: []ClusterConfig{
				{
					Name: "worker-1",
					Role: RoleWorker,
					Kueue: &KueueConfig{
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
			},
			managementKueueConfig: nil,
			want: &KueueConfig{
				ResourceFlavors: []ResourceFlavor{{Name: "gpu-flavor"}},
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
									{Name: "gpu-flavor", Resources: []Resource{{Name: "nvidia.com/gpu", NominalQuota: "800"}}},
								},
							},
						},
						AdmissionChecks: []string{"gpu-ws"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveManagementKueueConfig(tt.workerSets, tt.expandedWorkers, tt.managementKueueConfig)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DeriveManagementKueueConfig() =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}
