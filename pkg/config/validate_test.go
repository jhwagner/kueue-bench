package config

import (
	"strings"
	"testing"
)

func TestValidateTopology(t *testing.T) {
	tests := []struct {
		name    string
		topo    *Topology
		wantErr bool
	}{
		{
			name: "valid standalone topology",
			topo: &Topology{
				APIVersion: "kueue-bench.io/v1alpha1",
				Kind:       "Topology",
				Metadata: Metadata{
					Name: "test",
				},
				Spec: TopologySpec{
					Clusters: []ClusterConfig{
						{
							Name: "test-cluster",
							Role: "standalone",
							NodePools: []NodePool{
								{
									Name:  "pool1",
									Count: 10,
									Resources: map[string]string{
										"cpu":    "32",
										"memory": "128Gi",
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
			name: "invalid API version",
			topo: &Topology{
				APIVersion: "v1",
				Kind:       "Topology",
				Metadata:   Metadata{Name: "test"},
				Spec: TopologySpec{
					Clusters: []ClusterConfig{
						{
							Name: "test",
							Role: "standalone",
							NodePools: []NodePool{
								{Name: "pool1", Count: 1, Resources: map[string]string{"cpu": "1"}},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "valid management role",
			topo: &Topology{
				APIVersion: "kueue-bench.io/v1alpha1",
				Kind:       "Topology",
				Metadata:   Metadata{Name: "test"},
				Spec: TopologySpec{
					Clusters: []ClusterConfig{
						{
							Name: "test",
							Role: "management",
							NodePools: []NodePool{
								{Name: "pool1", Count: 1, Resources: map[string]string{"cpu": "1"}},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid role",
			topo: &Topology{
				APIVersion: "kueue-bench.io/v1alpha1",
				Kind:       "Topology",
				Metadata:   Metadata{Name: "test"},
				Spec: TopologySpec{
					Clusters: []ClusterConfig{
						{
							Name: "test",
							Role: "invalid-role",
							NodePools: []NodePool{
								{Name: "pool1", Count: 1, Resources: map[string]string{"cpu": "1"}},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "zero node count",
			topo: &Topology{
				APIVersion: "kueue-bench.io/v1alpha1",
				Kind:       "Topology",
				Metadata:   Metadata{Name: "test"},
				Spec: TopologySpec{
					Clusters: []ClusterConfig{
						{
							Name: "test",
							Role: "standalone",
							NodePools: []NodePool{
								{Name: "pool1", Count: 0, Resources: map[string]string{"cpu": "1"}},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid resource quantity",
			topo: &Topology{
				APIVersion: "kueue-bench.io/v1alpha1",
				Kind:       "Topology",
				Metadata:   Metadata{Name: "test"},
				Spec: TopologySpec{
					Clusters: []ClusterConfig{
						{
							Name: "test",
							Role: "standalone",
							NodePools: []NodePool{
								{
									Name:  "pool1",
									Count: 1,
									Resources: map[string]string{
										"cpu": "invalid",
									},
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTopology(tt.topo)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTopology() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateCohorts(t *testing.T) {
	tests := []struct {
		name        string
		cohorts     []Cohort
		wantErr     bool
		errContains string
	}{
		{
			name:    "empty cohorts",
			cohorts: []Cohort{},
			wantErr: false,
		},
		{
			name: "valid single cohort",
			cohorts: []Cohort{
				{Name: "platform"},
			},
			wantErr: false,
		},
		{
			name: "valid hierarchical cohorts",
			cohorts: []Cohort{
				{Name: "platform"},
				{Name: "team-a", ParentName: "platform"},
				{Name: "team-b", ParentName: "platform"},
			},
			wantErr: false,
		},
		{
			name: "valid three-level hierarchy",
			cohorts: []Cohort{
				{Name: "root"},
				{Name: "platform", ParentName: "root"},
				{Name: "team-a", ParentName: "platform"},
			},
			wantErr: false,
		},
		{
			name: "missing cohort name",
			cohorts: []Cohort{
				{Name: ""},
			},
			wantErr:     true,
			errContains: "name is required",
		},
		{
			name: "duplicate cohort names",
			cohorts: []Cohort{
				{Name: "platform"},
				{Name: "platform"},
			},
			wantErr:     true,
			errContains: "duplicate cohort name",
		},
		{
			name: "unknown parent cohort",
			cohorts: []Cohort{
				{Name: "team-a", ParentName: "nonexistent"},
			},
			wantErr:     true,
			errContains: "unknown parent cohort 'nonexistent'",
		},
		{
			name: "parent defined after child",
			cohorts: []Cohort{
				{Name: "team-a", ParentName: "platform"},
				{Name: "platform"},
			},
			wantErr: false, // Order doesn't matter, we build map first
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateCohorts(tt.cohorts, 0, "test-cluster")
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCohorts() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("validateCohorts() error = %v, expected to contain %q", err, tt.errContains)
				}
			}
		})
	}
}

func TestValidateTopologyWithCohorts(t *testing.T) {
	tests := []struct {
		name        string
		topo        *Topology
		wantErr     bool
		errContains string
	}{
		{
			name: "valid topology with hierarchical cohorts",
			topo: &Topology{
				APIVersion: "kueue-bench.io/v1alpha1",
				Kind:       "Topology",
				Metadata:   Metadata{Name: "test"},
				Spec: TopologySpec{
					Clusters: []ClusterConfig{
						{
							Name: "test-cluster",
							Role: "standalone",
							NodePools: []NodePool{
								{Name: "pool1", Count: 1, Resources: map[string]string{"cpu": "1"}},
							},
							Kueue: &KueueConfig{
								Cohorts: []Cohort{
									{Name: "platform"},
									{Name: "team-a", ParentName: "platform"},
								},
								ResourceFlavors: []ResourceFlavor{
									{Name: "default"},
								},
								ClusterQueues: []ClusterQueue{
									{
										Name:   "team-a-cq",
										Cohort: "team-a",
										ResourceGroups: []ResourceGroup{
											{
												CoveredResources: []string{"cpu"},
												Flavors: []FlavorQuotas{
													{
														Name: "default",
														Resources: []Resource{
															{Name: "cpu", NominalQuota: "10"},
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
				},
			},
			wantErr: false,
		},
		{
			name: "clusterqueue references nonexistent cohort",
			topo: &Topology{
				APIVersion: "kueue-bench.io/v1alpha1",
				Kind:       "Topology",
				Metadata:   Metadata{Name: "test"},
				Spec: TopologySpec{
					Clusters: []ClusterConfig{
						{
							Name: "test-cluster",
							Role: "standalone",
							NodePools: []NodePool{
								{Name: "pool1", Count: 1, Resources: map[string]string{"cpu": "1"}},
							},
							Kueue: &KueueConfig{
								Cohorts: []Cohort{
									{Name: "platform"},
								},
								ResourceFlavors: []ResourceFlavor{
									{Name: "default"},
								},
								ClusterQueues: []ClusterQueue{
									{
										Name:   "team-a-cq",
										Cohort: "nonexistent",
										ResourceGroups: []ResourceGroup{
											{
												CoveredResources: []string{"cpu"},
												Flavors: []FlavorQuotas{
													{
														Name: "default",
														Resources: []Resource{
															{Name: "cpu", NominalQuota: "10"},
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
				},
			},
			wantErr:     true,
			errContains: "unknown cohort 'nonexistent'",
		},
		{
			name: "cohort with nonexistent parent",
			topo: &Topology{
				APIVersion: "kueue-bench.io/v1alpha1",
				Kind:       "Topology",
				Metadata:   Metadata{Name: "test"},
				Spec: TopologySpec{
					Clusters: []ClusterConfig{
						{
							Name: "test-cluster",
							Role: "standalone",
							NodePools: []NodePool{
								{Name: "pool1", Count: 1, Resources: map[string]string{"cpu": "1"}},
							},
							Kueue: &KueueConfig{
								Cohorts: []Cohort{
									{Name: "team-a", ParentName: "nonexistent"},
								},
							},
						},
					},
				},
			},
			wantErr:     true,
			errContains: "unknown parent cohort 'nonexistent'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTopology(tt.topo)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTopology() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateTopology() error = %v, expected to contain %q", err, tt.errContains)
				}
			}
		})
	}
}

func TestValidateWorkerSets(t *testing.T) {
	validWorkerSet := func() WorkerSet {
		return WorkerSet{
			Name: "gpu-workers",
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
			LocalQueues: []LocalQueue{
				{Name: "team-lq", Namespace: "team-ns", ClusterQueue: "team-cq"},
			},
			Workers: []Worker{
				{
					Name: "worker-1",
					NodePools: []NodePool{
						{
							Name:  "gpu-pool",
							Count: 10,
							Resources: map[string]string{
								"nvidia.com/gpu": "8",
								"cpu":            "96",
							},
						},
					},
				},
			},
		}
	}

	tests := []struct {
		name         string
		workerSets   []WorkerSet
		clusterNames map[string]bool
		wantErr      bool
		errContains  string
	}{
		{
			name:         "valid workerSet",
			workerSets:   []WorkerSet{validWorkerSet()},
			clusterNames: map[string]bool{},
			wantErr:      false,
		},
		{
			name: "duplicate workerSet names",
			workerSets: []WorkerSet{
				validWorkerSet(),
				validWorkerSet(),
			},
			clusterNames: map[string]bool{},
			wantErr:      true,
			errContains:  "duplicate workerSet name 'gpu-workers'",
		},
		{
			name: "empty workerSet name",
			workerSets: []WorkerSet{
				func() WorkerSet {
					ws := validWorkerSet()
					ws.Name = ""
					return ws
				}(),
			},
			clusterNames: map[string]bool{},
			wantErr:      true,
			errContains:  "name is required",
		},
		{
			name: "no resourceFlavors",
			workerSets: []WorkerSet{
				func() WorkerSet {
					ws := validWorkerSet()
					ws.ResourceFlavors = nil
					return ws
				}(),
			},
			clusterNames: map[string]bool{},
			wantErr:      true,
			errContains:  "at least one resourceFlavor is required",
		},
		{
			name: "no clusterQueues",
			workerSets: []WorkerSet{
				func() WorkerSet {
					ws := validWorkerSet()
					ws.ClusterQueues = nil
					return ws
				}(),
			},
			clusterNames: map[string]bool{},
			wantErr:      true,
			errContains:  "at least one clusterQueue is required",
		},
		{
			name: "no workers",
			workerSets: []WorkerSet{
				func() WorkerSet {
					ws := validWorkerSet()
					ws.Workers = nil
					return ws
				}(),
			},
			clusterNames: map[string]bool{},
			wantErr:      true,
			errContains:  "at least one worker is required",
		},
		{
			name: "missing nodePoolRef",
			workerSets: []WorkerSet{
				func() WorkerSet {
					ws := validWorkerSet()
					ws.ResourceFlavors[0].NodePoolRef = ""
					return ws
				}(),
			},
			clusterNames: map[string]bool{},
			wantErr:      true,
			errContains:  "nodePoolRef is required",
		},
		{
			name: "unknown flavor in clusterQueue",
			workerSets: []WorkerSet{
				func() WorkerSet {
					ws := validWorkerSet()
					ws.ClusterQueues[0].ResourceGroups[0].Flavors = []WorkerSetFlavorRef{
						{Name: "nonexistent-flavor"},
					}
					return ws
				}(),
			},
			clusterNames: map[string]bool{},
			wantErr:      true,
			errContains:  "unknown resourceFlavor 'nonexistent-flavor'",
		},
		{
			name:         "worker name conflicts with cluster",
			workerSets:   []WorkerSet{validWorkerSet()},
			clusterNames: map[string]bool{"worker-1": true},
			wantErr:      true,
			errContains:  "conflicts with an existing cluster",
		},
		{
			name: "duplicate worker names across workerSets",
			workerSets: []WorkerSet{
				validWorkerSet(),
				func() WorkerSet {
					ws := validWorkerSet()
					ws.Name = "other-workers"
					return ws // same worker name "worker-1"
				}(),
			},
			clusterNames: map[string]bool{},
			wantErr:      true,
			errContains:  "duplicate worker name 'worker-1'",
		},
		{
			name: "nodePoolRef not found in worker",
			workerSets: []WorkerSet{
				func() WorkerSet {
					ws := validWorkerSet()
					ws.Workers[0].NodePools[0].Name = "other-pool"
					return ws
				}(),
			},
			clusterNames: map[string]bool{},
			wantErr:      true,
			errContains:  "nodePoolRef 'gpu-pool' (from resourceFlavor 'gpu-flavor') not found",
		},
		{
			name: "covered resource missing from pool",
			workerSets: []WorkerSet{
				func() WorkerSet {
					ws := validWorkerSet()
					delete(ws.Workers[0].NodePools[0].Resources, "cpu")
					return ws
				}(),
			},
			clusterNames: map[string]bool{},
			wantErr:      true,
			errContains:  "covered resource 'cpu' not found in pool resources",
		},
		{
			name: "invalid pool count",
			workerSets: []WorkerSet{
				func() WorkerSet {
					ws := validWorkerSet()
					ws.Workers[0].NodePools[0].Count = 0
					return ws
				}(),
			},
			clusterNames: map[string]bool{},
			wantErr:      true,
			errContains:  "count must be > 0",
		},
		{
			name: "localQueue references unknown clusterQueue",
			workerSets: []WorkerSet{
				func() WorkerSet {
					ws := validWorkerSet()
					ws.LocalQueues[0].ClusterQueue = "nonexistent-cq"
					return ws
				}(),
			},
			clusterNames: map[string]bool{},
			wantErr:      true,
			errContains:  "unknown clusterQueue 'nonexistent-cq'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWorkerSets(tt.workerSets, tt.clusterNames)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateWorkerSets() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("validateWorkerSets() error = %v, expected to contain %q", err, tt.errContains)
				}
			}
		})
	}
}

func TestValidateMultiKueueTopology(t *testing.T) {
	tests := []struct {
		name        string
		topo        *Topology
		wantErr     bool
		errContains string
	}{
		{
			name: "valid: workerSet with management cluster",
			topo: &Topology{
				APIVersion: "kueue-bench.io/v1alpha1",
				Kind:       "Topology",
				Metadata:   Metadata{Name: "test"},
				Spec: TopologySpec{
					Clusters: []ClusterConfig{
						{
							Name: "management",
							Role: "management",
							NodePools: []NodePool{
								{Name: "pool1", Count: 1, Resources: map[string]string{"cpu": "1"}},
							},
						},
					},
					WorkerSets: []WorkerSet{
						{
							Name: "workers",
							ResourceFlavors: []WorkerSetFlavor{
								{Name: "default", NodePoolRef: "pool"},
							},
							ClusterQueues: []WorkerSetClusterQueue{
								{
									Name: "cq",
									ResourceGroups: []WorkerSetResourceGroup{
										{
											CoveredResources: []string{"cpu"},
											Flavors:          []WorkerSetFlavorRef{{Name: "default"}},
										},
									},
								},
							},
							Workers: []Worker{
								{
									Name: "worker-1",
									NodePools: []NodePool{
										{Name: "pool", Count: 1, Resources: map[string]string{"cpu": "1"}},
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
			name: "invalid: workerSet without management cluster",
			topo: &Topology{
				APIVersion: "kueue-bench.io/v1alpha1",
				Kind:       "Topology",
				Metadata:   Metadata{Name: "test"},
				Spec: TopologySpec{
					Clusters: []ClusterConfig{
						{
							Name: "standalone",
							Role: "standalone",
							NodePools: []NodePool{
								{Name: "pool1", Count: 1, Resources: map[string]string{"cpu": "1"}},
							},
						},
					},
					WorkerSets: []WorkerSet{
						{
							Name: "workers",
							ResourceFlavors: []WorkerSetFlavor{
								{Name: "default", NodePoolRef: "pool"},
							},
							ClusterQueues: []WorkerSetClusterQueue{
								{
									Name: "cq",
									ResourceGroups: []WorkerSetResourceGroup{
										{
											CoveredResources: []string{"cpu"},
											Flavors:          []WorkerSetFlavorRef{{Name: "default"}},
										},
									},
								},
							},
							Workers: []Worker{
								{
									Name: "worker-1",
									NodePools: []NodePool{
										{Name: "pool", Count: 1, Resources: map[string]string{"cpu": "1"}},
									},
								},
							},
						},
					},
				},
			},
			wantErr:     true,
			errContains: "workerSets require exactly one cluster with role 'management', found 0",
		},
		{
			name: "invalid: workerSet with multiple management clusters",
			topo: &Topology{
				APIVersion: "kueue-bench.io/v1alpha1",
				Kind:       "Topology",
				Metadata:   Metadata{Name: "test"},
				Spec: TopologySpec{
					Clusters: []ClusterConfig{
						{
							Name: "management-1",
							Role: "management",
							NodePools: []NodePool{
								{Name: "pool1", Count: 1, Resources: map[string]string{"cpu": "1"}},
							},
						},
						{
							Name: "management-2",
							Role: "management",
							NodePools: []NodePool{
								{Name: "pool1", Count: 1, Resources: map[string]string{"cpu": "1"}},
							},
						},
					},
					WorkerSets: []WorkerSet{
						{
							Name: "workers",
							ResourceFlavors: []WorkerSetFlavor{
								{Name: "default", NodePoolRef: "pool"},
							},
							ClusterQueues: []WorkerSetClusterQueue{
								{
									Name: "cq",
									ResourceGroups: []WorkerSetResourceGroup{
										{
											CoveredResources: []string{"cpu"},
											Flavors:          []WorkerSetFlavorRef{{Name: "default"}},
										},
									},
								},
							},
							Workers: []Worker{
								{
									Name: "worker-1",
									NodePools: []NodePool{
										{Name: "pool", Count: 1, Resources: map[string]string{"cpu": "1"}},
									},
								},
							},
						},
					},
				},
			},
			wantErr:     true,
			errContains: "workerSets require exactly one cluster with role 'management', found 2",
		},
		{
			name: "valid: no workerSets, no management cluster required",
			topo: &Topology{
				APIVersion: "kueue-bench.io/v1alpha1",
				Kind:       "Topology",
				Metadata:   Metadata{Name: "test"},
				Spec: TopologySpec{
					Clusters: []ClusterConfig{
						{
							Name: "standalone",
							Role: "standalone",
							NodePools: []NodePool{
								{Name: "pool1", Count: 1, Resources: map[string]string{"cpu": "1"}},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid: no workerSets, multiple standalone clusters",
			topo: &Topology{
				APIVersion: "kueue-bench.io/v1alpha1",
				Kind:       "Topology",
				Metadata:   Metadata{Name: "test"},
				Spec: TopologySpec{
					Clusters: []ClusterConfig{
						{
							Name: "cluster-1",
							Role: "standalone",
							NodePools: []NodePool{
								{Name: "pool1", Count: 1, Resources: map[string]string{"cpu": "1"}},
							},
						},
						{
							Name: "cluster-2",
							Role: "standalone",
							NodePools: []NodePool{
								{Name: "pool1", Count: 1, Resources: map[string]string{"cpu": "1"}},
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTopology(tt.topo)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTopology() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateTopology() error = %v, expected to contain %q", err, tt.errContains)
				}
			}
		})
	}
}
