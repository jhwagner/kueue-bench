package cluster

import (
	"time"
)

// ClusterMetadata stores information about a created cluster
type ClusterMetadata struct {
	Name           string    `json:"name"`
	KubeconfigPath string    `json:"kubeconfigPath"`
	CreatedAt      time.Time `json:"createdAt"`
}
