package config

import (
	"fmt"
	"time"
)

// ValidateWorkloadProfile validates a workload profile configuration.
// This validates the generation engine's own schema: arrival patterns,
// distribution parameters, weights, and template structure per workload type.
func ValidateWorkloadProfile(p *WorkloadProfile) error {
	if p.APIVersion != APIVersion {
		return fmt.Errorf("unsupported apiVersion: %s (expected %s)", p.APIVersion, APIVersion)
	}

	if p.Kind != KindWorkloadProfile {
		return fmt.Errorf("unsupported kind: %s (expected %s)", p.Kind, KindWorkloadProfile)
	}

	if p.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}

	if p.Spec.Duration == "" {
		return fmt.Errorf("spec.duration is required")
	}

	if _, err := time.ParseDuration(p.Spec.Duration); err != nil {
		return fmt.Errorf("spec.duration: invalid duration %q: %w", p.Spec.Duration, err)
	}

	if err := validateArrivalPattern(&p.Spec.ArrivalPattern); err != nil {
		return fmt.Errorf("spec.arrivalPattern: %w", err)
	}

	if len(p.Spec.Workloads) == 0 {
		return fmt.Errorf("spec.workloads: at least one workload is required")
	}

	for i, w := range p.Spec.Workloads {
		if err := validateWorkloadSpec(&w, i); err != nil {
			return err
		}
	}

	return nil
}

func validateArrivalPattern(a *ArrivalPattern) error {
	switch a.Type {
	case "constant", "poisson":
		if a.RatePerMinute == nil {
			return fmt.Errorf("ratePerMinute is required for type %q", a.Type)
		}
		if *a.RatePerMinute <= 0 {
			return fmt.Errorf("ratePerMinute must be > 0, got %g", *a.RatePerMinute)
		}
	default:
		return fmt.Errorf("unsupported type %q (must be constant or poisson)", a.Type)
	}

	return nil
}

func validateWorkloadSpec(w *WorkloadSpec, index int) error {
	if w.Weight <= 0 {
		return fmt.Errorf("spec.workloads[%d] (%s): weight must be > 0", index, w.Type)
	}

	for i, t := range w.Tolerations {
		if t.Key == "" && t.Operator != "Exists" {
			return fmt.Errorf("spec.workloads[%d]: tolerations[%d]: key is required unless operator is Exists", index, i)
		}
		switch t.Effect {
		case "", "NoSchedule", "PreferNoSchedule", "NoExecute":
		default:
			return fmt.Errorf("spec.workloads[%d]: tolerations[%d]: invalid effect %q", index, i, t.Effect)
		}
	}

	switch w.Type {
	case "Job":
		t, ok := w.Template.(*JobTemplate)
		if !ok || t == nil {
			return fmt.Errorf("spec.workloads[%d] (Job): template is required", index)
		}
		if err := validateJobTemplate(t, index); err != nil {
			return err
		}
	case "JobSet":
		t, ok := w.Template.(*JobSetTemplate)
		if !ok || t == nil {
			return fmt.Errorf("spec.workloads[%d] (JobSet): template is required", index)
		}
		if err := validateJobSetTemplate(t, index); err != nil {
			return err
		}
	case "RayJob":
		t, ok := w.Template.(*RayJobTemplate)
		if !ok || t == nil {
			return fmt.Errorf("spec.workloads[%d] (RayJob): template is required", index)
		}
		if err := validateRayJobTemplate(t, index); err != nil {
			return err
		}
	default:
		return fmt.Errorf("spec.workloads[%d]: unsupported type %q (must be Job, JobSet, or RayJob)", index, w.Type)
	}

	return nil
}

func validateCommonTemplate(c *CommonTemplate, workloadType string, index int) error {
	if c.Duration != nil {
		if err := validateDistribution(c.Duration, "duration"); err != nil {
			return fmt.Errorf("spec.workloads[%d] (%s): template.%w", index, workloadType, err)
		}
	}

	return nil
}

func validateJobTemplate(t *JobTemplate, index int) error {
	if t.Resources == nil {
		return fmt.Errorf("spec.workloads[%d] (Job): template.resources is required", index)
	}
	if err := validateResourceRequirements(t.Resources); err != nil {
		return fmt.Errorf("spec.workloads[%d] (Job): template.resources: %w", index, err)
	}

	if t.Parallelism != nil {
		if err := validateDistribution(t.Parallelism, "parallelism"); err != nil {
			return fmt.Errorf("spec.workloads[%d] (Job): template.%w", index, err)
		}
	}
	if t.Completions != nil {
		if err := validateDistribution(t.Completions, "completions"); err != nil {
			return fmt.Errorf("spec.workloads[%d] (Job): template.%w", index, err)
		}
	}

	return validateCommonTemplate(&t.CommonTemplate, "Job", index)
}

func validateJobSetTemplate(t *JobSetTemplate, index int) error {
	if len(t.ReplicatedJobs) == 0 {
		return fmt.Errorf("spec.workloads[%d] (JobSet): template.replicatedJobs is required", index)
	}

	for i, rj := range t.ReplicatedJobs {
		if rj.Name == "" {
			return fmt.Errorf("spec.workloads[%d] (JobSet): template.replicatedJobs[%d]: name is required", index, i)
		}

		if rj.Resources == nil {
			return fmt.Errorf("spec.workloads[%d] (JobSet): template.replicatedJobs[%d] (%s): resources is required",
				index, i, rj.Name)
		}
		if err := validateResourceRequirements(rj.Resources); err != nil {
			return fmt.Errorf("spec.workloads[%d] (JobSet): template.replicatedJobs[%d] (%s): resources: %w",
				index, i, rj.Name, err)
		}

		if rj.Replicas != nil {
			if err := validateDistribution(rj.Replicas, "replicas"); err != nil {
				return fmt.Errorf("spec.workloads[%d] (JobSet): template.replicatedJobs[%d] (%s): %w",
					index, i, rj.Name, err)
			}
		}
	}

	return validateCommonTemplate(&t.CommonTemplate, "JobSet", index)
}

func validateRayJobTemplate(t *RayJobTemplate, index int) error {
	if t.HeadResources == nil {
		return fmt.Errorf("spec.workloads[%d] (RayJob): template.headResources is required", index)
	}
	if err := validateResourceRequirements(t.HeadResources); err != nil {
		return fmt.Errorf("spec.workloads[%d] (RayJob): template.headResources: %w", index, err)
	}

	if t.WorkerResources == nil {
		return fmt.Errorf("spec.workloads[%d] (RayJob): template.workerResources is required", index)
	}
	if err := validateResourceRequirements(t.WorkerResources); err != nil {
		return fmt.Errorf("spec.workloads[%d] (RayJob): template.workerResources: %w", index, err)
	}

	if t.WorkerReplicas != nil {
		if err := validateDistribution(t.WorkerReplicas, "workerReplicas"); err != nil {
			return fmt.Errorf("spec.workloads[%d] (RayJob): template.%w", index, err)
		}
	}

	return validateCommonTemplate(&t.CommonTemplate, "RayJob", index)
}

func validateResourceRequirements(r *ResourceRequirements) error {
	if len(r.Requests) == 0 {
		return fmt.Errorf("requests must not be empty")
	}

	for name, dist := range r.Requests {
		if err := validateDistribution(&dist, name); err != nil {
			return fmt.Errorf("requests.%w", err)
		}
	}

	return nil
}

func validateDistribution(d *Distribution, field string) error {
	if d.IsFixed() {
		return nil
	}

	if d.Type == "" {
		return fmt.Errorf("%s: distribution value or type is required", field)
	}

	switch d.Type {
	case "uniform":
		if d.Min == "" || d.Max == "" {
			return fmt.Errorf("%s: uniform distribution requires min and max", field)
		}
	case "normal", "lognormal":
		if d.Mean == "" || d.Stddev == "" {
			return fmt.Errorf("%s: %s distribution requires mean and stddev", field, d.Type)
		}
	case "choice":
		if len(d.Values) == 0 {
			return fmt.Errorf("%s: choice distribution requires values", field)
		}
		if len(d.Weights) > 0 && len(d.Weights) != len(d.Values) {
			return fmt.Errorf("%s: choice distribution weights length (%d) must match values length (%d)",
				field, len(d.Weights), len(d.Values))
		}
	default:
		return fmt.Errorf("%s: unsupported distribution type %q (must be uniform, normal, lognormal, or choice)", field, d.Type)
	}

	return nil
}
