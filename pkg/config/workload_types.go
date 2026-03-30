package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// WorkloadProfile represents a workload generation configuration
type WorkloadProfile struct {
	APIVersion string              `yaml:"apiVersion"`
	Kind       string              `yaml:"kind"`
	Metadata   Metadata            `yaml:"metadata"`
	Spec       WorkloadProfileSpec `yaml:"spec"`
}

// WorkloadProfileSpec defines the workload generation parameters
type WorkloadProfileSpec struct {
	Seed           *int64         `yaml:"seed,omitempty"`
	Duration       string         `yaml:"duration"`
	ArrivalPattern ArrivalPattern `yaml:"arrivalPattern"`
	Workloads      []WorkloadSpec `yaml:"workloads"`
}

// ArrivalPattern defines how workloads are submitted over time
type ArrivalPattern struct {
	Type          string   `yaml:"type"` // constant, poisson
	RatePerMinute *float64 `yaml:"ratePerMinute,omitempty"`
}

// WorkloadSpec defines a workload type with its weight and template.
// Template holds one of *JobTemplate, *JobSetTemplate, or *RayJobTemplate
// depending on Type, populated via custom YAML unmarshalling.
type WorkloadSpec struct {
	Type          string        `yaml:"type"` // Job, JobSet, RayJob
	Weight        int           `yaml:"weight"`
	LocalQueue    string        `yaml:"localQueue,omitempty"`
	Namespace     string        `yaml:"namespace,omitempty"`
	PriorityClass string        `yaml:"priorityClass,omitempty"`
	Tolerations   []Toleration  `yaml:"tolerations,omitempty"`
	Template      interface{}   `yaml:"-"`
}

// Toleration represents a Kubernetes pod toleration.
type Toleration struct {
	Key      string `yaml:"key"`
	Operator string `yaml:"operator,omitempty"` // Equal (default) or Exists
	Value    string `yaml:"value,omitempty"`
	Effect   string `yaml:"effect,omitempty"` // NoSchedule, PreferNoSchedule, NoExecute, or empty (matches all)
}

// UnmarshalYAML implements custom YAML unmarshalling for WorkloadSpec.
// It reads the type field first, then decodes the template node into the
// appropriate typed struct.
func (w *WorkloadSpec) UnmarshalYAML(value *yaml.Node) error {
	type rawWorkloadSpec struct {
		Type          string       `yaml:"type"`
		Weight        int          `yaml:"weight"`
		LocalQueue    string       `yaml:"localQueue,omitempty"`
		Namespace     string       `yaml:"namespace,omitempty"`
		PriorityClass string       `yaml:"priorityClass,omitempty"`
		Tolerations   []Toleration `yaml:"tolerations,omitempty"`
		Template      yaml.Node    `yaml:"template"`
	}

	var raw rawWorkloadSpec
	if err := value.Decode(&raw); err != nil {
		return err
	}

	w.Type = raw.Type
	w.Weight = raw.Weight
	w.LocalQueue = raw.LocalQueue
	w.Namespace = raw.Namespace
	w.PriorityClass = raw.PriorityClass
	w.Tolerations = raw.Tolerations

	if raw.Template.Kind == 0 {
		return nil
	}

	switch raw.Type {
	case "Job":
		var t JobTemplate
		if err := raw.Template.Decode(&t); err != nil {
			return fmt.Errorf("Job template: %w", err)
		}
		w.Template = &t
	case "JobSet":
		var t JobSetTemplate
		if err := raw.Template.Decode(&t); err != nil {
			return fmt.Errorf("JobSet template: %w", err)
		}
		w.Template = &t
	case "RayJob":
		var t RayJobTemplate
		if err := raw.Template.Decode(&t); err != nil {
			return fmt.Errorf("RayJob template: %w", err)
		}
		w.Template = &t
	default:
		// Unknown type: leave Template as nil; validation will catch it
	}

	return nil
}

// CommonTemplate holds fields shared by all workload template types.
type CommonTemplate struct {
	// Duration of the workload; maps to kwok.x-k8s.io/duration annotation
	Duration *Distribution `yaml:"duration,omitempty"`
}

// JobTemplate is the template for a batch/v1 Job workload.
type JobTemplate struct {
	CommonTemplate `yaml:",inline"`
	Resources      *ResourceRequirements `yaml:"resources,omitempty"`
	Parallelism    *Distribution         `yaml:"parallelism,omitempty"`
	Completions    *Distribution         `yaml:"completions,omitempty"`
}

// JobSetTemplate is the template for a jobset.x-k8s.io/v1alpha2 JobSet workload.
type JobSetTemplate struct {
	CommonTemplate `yaml:",inline"`
	ReplicatedJobs []ReplicatedJobTemplate `yaml:"replicatedJobs,omitempty"`
}

// RayJobTemplate is the template for a ray.io/v1 RayJob workload.
type RayJobTemplate struct {
	CommonTemplate  `yaml:",inline"`
	HeadResources   *ResourceRequirements `yaml:"headResources,omitempty"`
	WorkerReplicas  *Distribution         `yaml:"workerReplicas,omitempty"`
	WorkerResources *ResourceRequirements `yaml:"workerResources,omitempty"`
}

// ReplicatedJobTemplate defines a replicated job within a JobSet
type ReplicatedJobTemplate struct {
	Name      string                `yaml:"name"`
	Replicas  *Distribution         `yaml:"replicas,omitempty"`
	Resources *ResourceRequirements `yaml:"resources,omitempty"`
}

// ResourceRequirements defines resource requests for a workload.
// Values can be fixed strings or distributions.
type ResourceRequirements struct {
	Requests map[string]Distribution `yaml:"requests"`
}

// Distribution represents a value that can be fixed or sampled from a distribution.
// It uses a flexible YAML representation: a plain string is treated as a fixed value,
// while a map with a "distribution" key specifies sampling parameters.
type Distribution struct {
	// Fixed value (set when YAML is a plain scalar)
	Value string `yaml:"-"`

	// Distribution parameters (set when YAML is a map)
	Type    string   `yaml:"distribution,omitempty"`
	Min     string   `yaml:"min,omitempty"`
	Max     string   `yaml:"max,omitempty"`
	Mean    string   `yaml:"mean,omitempty"`
	Stddev  string   `yaml:"stddev,omitempty"`
	Values  []string `yaml:"values,omitempty"`
	Weights []int    `yaml:"weights,omitempty"`
}

// UnmarshalYAML implements custom YAML unmarshalling for Distribution.
// It handles both plain scalar values (e.g., "8") and map values
// (e.g., {distribution: uniform, min: 1, max: 4}).
func (d *Distribution) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Try scalar first
	var scalar string
	if err := unmarshal(&scalar); err == nil {
		d.Value = scalar
		return nil
	}

	// Fall back to map form
	type rawDistribution Distribution
	var raw rawDistribution
	if err := unmarshal(&raw); err != nil {
		return err
	}
	*d = Distribution(raw)
	return nil
}

// IsFixed returns true if the distribution represents a fixed value
func (d *Distribution) IsFixed() bool {
	return d.Value != "" && d.Type == ""
}
