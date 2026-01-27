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
			name: "management role not supported yet",
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
			err := validateCohorts(tt.cohorts, 0, "test-cluster")
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
