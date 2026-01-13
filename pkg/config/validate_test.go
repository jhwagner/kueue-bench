package config

import (
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
