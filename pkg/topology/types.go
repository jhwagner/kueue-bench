package topology

import (
	"time"
)

// Metadata stores information about a created topology
type Metadata struct {
	Name      string             `json:"name"`
	CreatedAt time.Time          `json:"createdAt"`
	Clusters  map[string]Cluster `json:"clusters"`
}

// Cluster stores information about a cluster within a topology
type Cluster struct {
	Name            string    `json:"name"`
	KindClusterName string    `json:"kindClusterName"`
	KubeconfigPath  string    `json:"kubeconfigPath"`
	CreatedAt       time.Time `json:"createdAt"`
}
