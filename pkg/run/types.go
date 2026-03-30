package run

import (
	"time"
)

// RunMetadata stores information about a workload simulation run.
type RunMetadata struct {
	RunID         string    `json:"runID"`
	ProfileName   string    `json:"profileName"`
	ProfilePath   string    `json:"profilePath"`
	TopologyName  string    `json:"topologyName,omitempty"`
	ClusterName   string    `json:"clusterName,omitempty"`
	Seed          int64     `json:"seed"`
	DryRun        bool      `json:"dryRun"`
	WorkloadCount int       `json:"workloadCount"`
	StartedAt     time.Time `json:"startedAt"`
	Duration      string    `json:"duration"`
}
