package config

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDistributionUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		wantDist Distribution
	}{
		{
			name:     "scalar string",
			yaml:     `"8"`,
			wantDist: Distribution{Value: "8"},
		},
		{
			name:     "scalar number",
			yaml:     `4`,
			wantDist: Distribution{Value: "4"},
		},
		{
			name: "uniform distribution",
			yaml: `{distribution: uniform, min: "1", max: "4"}`,
			wantDist: Distribution{
				Type: "uniform",
				Min:  "1",
				Max:  "4",
			},
		},
		{
			name: "lognormal distribution",
			yaml: `{distribution: lognormal, mean: "20m", stddev: "10m"}`,
			wantDist: Distribution{
				Type:   "lognormal",
				Mean:   "20m",
				Stddev: "10m",
			},
		},
		{
			name: "choice distribution",
			yaml: `{distribution: choice, values: ["2", "4", "8"]}`,
			wantDist: Distribution{
				Type:   "choice",
				Values: []string{"2", "4", "8"},
			},
		},
		{
			name: "choice with weights",
			yaml: `{distribution: choice, values: ["small", "large"], weights: [70, 30]}`,
			wantDist: Distribution{
				Type:    "choice",
				Values:  []string{"small", "large"},
				Weights: []int{70, 30},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Distribution
			if err := yaml.Unmarshal([]byte(tt.yaml), &got); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}

			if !reflect.DeepEqual(got, tt.wantDist) {
				t.Errorf("got %+v, want %+v", got, tt.wantDist)
			}
		})
	}
}

func TestDistributionIsFixed(t *testing.T) {
	tests := []struct {
		name string
		dist Distribution
		want bool
	}{
		{
			name: "fixed scalar",
			dist: Distribution{Value: "8"},
			want: true,
		},
		{
			name: "uniform distribution",
			dist: Distribution{Type: "uniform", Min: "1", Max: "4"},
			want: false,
		},
		{
			name: "empty distribution",
			dist: Distribution{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.dist.IsFixed(); got != tt.want {
				t.Errorf("IsFixed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWorkloadProfileUnmarshalYAML(t *testing.T) {
	input := `
apiVersion: kueue-bench.io/v1alpha1
kind: WorkloadProfile
metadata:
  name: ml-training-mix
spec:
  seed: 12345
  duration: 30m
  arrivalPattern:
    type: poisson
    ratePerMinute: 20
  workloads:
    - type: Job
      weight: 60
      localQueue: team-a-queue
      template:
        resources:
          requests:
            nvidia.com/gpu: { distribution: uniform, min: "1", max: "4" }
            cpu: "16"
        duration: { distribution: lognormal, mean: "20m", stddev: "10m" }
    - type: JobSet
      weight: 40
      template:
        replicatedJobs:
          - name: workers
            replicas: { distribution: choice, values: ["2", "4", "8"] }
            resources:
              requests:
                nvidia.com/gpu: "8"
`

	var profile WorkloadProfile
	if err := yaml.Unmarshal([]byte(input), &profile); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if profile.Metadata.Name != "ml-training-mix" {
		t.Errorf("Name = %q, want %q", profile.Metadata.Name, "ml-training-mix")
	}

	if profile.Spec.Seed == nil || *profile.Spec.Seed != 12345 {
		t.Error("Seed not parsed correctly")
	}

	if len(profile.Spec.Workloads) != 2 {
		t.Fatalf("Workloads length = %d, want 2", len(profile.Spec.Workloads))
	}

	// Verify Job workload
	jobSpec := profile.Spec.Workloads[0]
	if jobSpec.Type != "Job" || jobSpec.Weight != 60 {
		t.Errorf("Job: Type=%q Weight=%d", jobSpec.Type, jobSpec.Weight)
	}

	jobTmpl, ok := jobSpec.Template.(*JobTemplate)
	if !ok || jobTmpl == nil {
		t.Fatalf("Job template is not *JobTemplate, got %T", jobSpec.Template)
	}

	gpuDist := jobTmpl.Resources.Requests["nvidia.com/gpu"]
	if gpuDist.Type != "uniform" || gpuDist.Min != "1" || gpuDist.Max != "4" {
		t.Errorf("GPU distribution: %+v", gpuDist)
	}

	cpuDist := jobTmpl.Resources.Requests["cpu"]
	if cpuDist.Value != "16" {
		t.Errorf("CPU value = %q, want %q", cpuDist.Value, "16")
	}

	if jobTmpl.Duration == nil || jobTmpl.Duration.Type != "lognormal" {
		t.Errorf("Job duration: %+v", jobTmpl.Duration)
	}

	// Verify JobSet workload
	jobsetSpec := profile.Spec.Workloads[1]
	if jobsetSpec.Type != "JobSet" || jobsetSpec.Weight != 40 {
		t.Errorf("JobSet: Type=%q Weight=%d", jobsetSpec.Type, jobsetSpec.Weight)
	}

	jobsetTmpl, ok := jobsetSpec.Template.(*JobSetTemplate)
	if !ok || jobsetTmpl == nil {
		t.Fatalf("JobSet template is not *JobSetTemplate, got %T", jobsetSpec.Template)
	}

	if len(jobsetTmpl.ReplicatedJobs) != 1 {
		t.Fatalf("ReplicatedJobs length = %d, want 1", len(jobsetTmpl.ReplicatedJobs))
	}

	rj := jobsetTmpl.ReplicatedJobs[0]
	if rj.Name != "workers" {
		t.Errorf("ReplicatedJob name = %q, want %q", rj.Name, "workers")
	}
	if rj.Replicas == nil || rj.Replicas.Type != "choice" || len(rj.Replicas.Values) != 3 {
		t.Errorf("ReplicatedJob replicas: %+v", rj.Replicas)
	}
}

func TestWorkloadSpecUnmarshalYAMLRayJob(t *testing.T) {
	input := `
type: RayJob
weight: 10
template:
  headResources:
    requests:
      cpu: "4"
      memory: "16Gi"
  workerResources:
    requests:
      nvidia.com/gpu: "1"
  workerReplicas: { distribution: uniform, min: "2", max: "16" }
`
	var spec WorkloadSpec
	if err := yaml.Unmarshal([]byte(input), &spec); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if spec.Type != "RayJob" || spec.Weight != 10 {
		t.Errorf("Type=%q Weight=%d", spec.Type, spec.Weight)
	}

	tmpl, ok := spec.Template.(*RayJobTemplate)
	if !ok || tmpl == nil {
		t.Fatalf("RayJob template is not *RayJobTemplate, got %T", spec.Template)
	}

	if tmpl.HeadResources == nil {
		t.Fatal("headResources is nil")
	}
	if tmpl.HeadResources.Requests["cpu"].Value != "4" {
		t.Errorf("head cpu = %q", tmpl.HeadResources.Requests["cpu"].Value)
	}

	if tmpl.WorkerReplicas == nil || tmpl.WorkerReplicas.Type != "uniform" {
		t.Errorf("workerReplicas: %+v", tmpl.WorkerReplicas)
	}
}

func TestWorkloadSpecUnmarshalYAMLUnknownType(t *testing.T) {
	input := `
type: Deployment
weight: 1
template:
  foo: bar
`
	var spec WorkloadSpec
	if err := yaml.Unmarshal([]byte(input), &spec); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// Unknown type: Template should be nil (validation catches it later)
	if spec.Template != nil {
		t.Errorf("expected nil Template for unknown type, got %T", spec.Template)
	}
}
