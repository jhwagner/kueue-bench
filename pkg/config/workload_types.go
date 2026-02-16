package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// marshalToYAML marshals v to YAML bytes using gopkg.in/yaml.v3.
func marshalToYAML(v interface{}) ([]byte, error) {
	return yaml.Marshal(v)
}

// unmarshalFromYAML unmarshals YAML bytes into v using gopkg.in/yaml.v3.
func unmarshalFromYAML(data []byte, v interface{}) error {
	return yaml.Unmarshal(data, v)
}

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
	Type          string      `yaml:"type"` // Job, JobSet, RayJob
	Weight        int         `yaml:"weight"`
	LocalQueue    string      `yaml:"localQueue,omitempty"`
	Namespace     string      `yaml:"namespace,omitempty"`
	PriorityClass string      `yaml:"priorityClass,omitempty"`
	Template      interface{} `yaml:"-"`
}

// UnmarshalYAML implements custom YAML unmarshalling for WorkloadSpec.
// It reads the type field first, then unmarshals the template into the
// appropriate typed struct.
func (w *WorkloadSpec) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Raw struct to capture all fields including template as raw YAML
	type rawWorkloadSpec struct {
		Type          string      `yaml:"type"`
		Weight        int         `yaml:"weight"`
		LocalQueue    string      `yaml:"localQueue,omitempty"`
		Namespace     string      `yaml:"namespace,omitempty"`
		PriorityClass string      `yaml:"priorityClass,omitempty"`
		Template      interface{} `yaml:"template"`
	}

	var raw rawWorkloadSpec
	if err := unmarshal(&raw); err != nil {
		return err
	}

	w.Type = raw.Type
	w.Weight = raw.Weight
	w.LocalQueue = raw.LocalQueue
	w.Namespace = raw.Namespace
	w.PriorityClass = raw.PriorityClass

	// Re-marshal the template node so we can unmarshal into the typed struct
	if raw.Template == nil {
		return nil
	}

	templateBytes, err := marshalToYAML(raw.Template)
	if err != nil {
		return fmt.Errorf("workload template: %w", err)
	}

	switch raw.Type {
	case "Job":
		var t JobTemplate
		if err := unmarshalFromYAML(templateBytes, &t); err != nil {
			return fmt.Errorf("Job template: %w", err)
		}
		w.Template = &t
	case "JobSet":
		var t JobSetTemplate
		if err := unmarshalFromYAML(templateBytes, &t); err != nil {
			return fmt.Errorf("JobSet template: %w", err)
		}
		w.Template = &t
	case "RayJob":
		var t RayJobTemplate
		if err := unmarshalFromYAML(templateBytes, &t); err != nil {
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
	// FailureRate is the probability [0,1] that a workload is injected as failed
	FailureRate *float64 `yaml:"failureRate,omitempty"`
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
