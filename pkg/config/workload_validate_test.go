package config

import (
	"strings"
	"testing"
)

func floatPtr(f float64) *float64 { return &f }

func validJobWorkloadProfile() *WorkloadProfile {
	return &WorkloadProfile{
		APIVersion: "kueue-bench.io/v1alpha1",
		Kind:       "WorkloadProfile",
		Metadata:   Metadata{Name: "test-profile"},
		Spec: WorkloadProfileSpec{
			Duration: "30m",
			ArrivalPattern: ArrivalPattern{
				Type:          "constant",
				RatePerMinute: floatPtr(10),
			},
			Workloads: []WorkloadSpec{
				{
					Type:   "Job",
					Weight: 100,
					Template: &JobTemplate{
						Resources: &ResourceRequirements{
							Requests: map[string]Distribution{
								"cpu": {Value: "4"},
							},
						},
					},
				},
			},
		},
	}
}

func TestValidateWorkloadProfile(t *testing.T) {
	tests := []struct {
		name        string
		profile     *WorkloadProfile
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid Job profile",
			profile: validJobWorkloadProfile(),
			wantErr: false,
		},
		{
			name: "valid mixed profile with relative weights",
			profile: &WorkloadProfile{
				APIVersion: "kueue-bench.io/v1alpha1",
				Kind:       "WorkloadProfile",
				Metadata:   Metadata{Name: "mixed"},
				Spec: WorkloadProfileSpec{
					Duration: "1h",
					ArrivalPattern: ArrivalPattern{
						Type:          "poisson",
						RatePerMinute: floatPtr(20),
					},
					Workloads: []WorkloadSpec{
						{
							Type:   "Job",
							Weight: 3,
							Template: &JobTemplate{
								Resources: &ResourceRequirements{
									Requests: map[string]Distribution{
										"nvidia.com/gpu": {Type: "uniform", Min: "1", Max: "4"},
									},
								},
								CommonTemplate: CommonTemplate{
									Duration: &Distribution{Type: "lognormal", Mean: "20m", Stddev: "10m"},
								},
							},
						},
						{
							Type:   "JobSet",
							Weight: 2,
							Template: &JobSetTemplate{
								ReplicatedJobs: []ReplicatedJobTemplate{
									{
										Name: "workers",
										Resources: &ResourceRequirements{
											Requests: map[string]Distribution{
												"nvidia.com/gpu": {Value: "8"},
											},
										},
										Replicas: &Distribution{Type: "choice", Values: []string{"2", "4", "8"}},
									},
								},
							},
						},
						{
							Type:   "RayJob",
							Weight: 1,
							Template: &RayJobTemplate{
								HeadResources: &ResourceRequirements{
									Requests: map[string]Distribution{
										"cpu":    {Value: "4"},
										"memory": {Value: "16Gi"},
									},
								},
								WorkerResources: &ResourceRequirements{
									Requests: map[string]Distribution{
										"nvidia.com/gpu": {Value: "1"},
									},
								},
								WorkerReplicas: &Distribution{Type: "uniform", Min: "2", Max: "16"},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid apiVersion",
			profile: func() *WorkloadProfile {
				p := validJobWorkloadProfile()
				p.APIVersion = "v1"
				return p
			}(),
			wantErr:     true,
			errContains: "unsupported apiVersion",
		},
		{
			name: "invalid kind",
			profile: func() *WorkloadProfile {
				p := validJobWorkloadProfile()
				p.Kind = "Topology"
				return p
			}(),
			wantErr:     true,
			errContains: "unsupported kind",
		},
		{
			name: "missing name",
			profile: func() *WorkloadProfile {
				p := validJobWorkloadProfile()
				p.Metadata.Name = ""
				return p
			}(),
			wantErr:     true,
			errContains: "metadata.name is required",
		},
		{
			name: "missing duration",
			profile: func() *WorkloadProfile {
				p := validJobWorkloadProfile()
				p.Spec.Duration = ""
				return p
			}(),
			wantErr:     true,
			errContains: "spec.duration is required",
		},
		{
			name: "invalid duration",
			profile: func() *WorkloadProfile {
				p := validJobWorkloadProfile()
				p.Spec.Duration = "not-a-duration"
				return p
			}(),
			wantErr:     true,
			errContains: "invalid duration",
		},
		{
			name: "no workloads",
			profile: func() *WorkloadProfile {
				p := validJobWorkloadProfile()
				p.Spec.Workloads = nil
				return p
			}(),
			wantErr:     true,
			errContains: "at least one workload is required",
		},
		{
			name: "zero weight",
			profile: func() *WorkloadProfile {
				p := validJobWorkloadProfile()
				p.Spec.Workloads[0].Weight = 0
				return p
			}(),
			wantErr:     true,
			errContains: "weight must be > 0",
		},
		{
			name: "unsupported workload type",
			profile: func() *WorkloadProfile {
				p := validJobWorkloadProfile()
				p.Spec.Workloads[0].Type = "Deployment"
				return p
			}(),
			wantErr:     true,
			errContains: "unsupported type \"Deployment\"",
		},
		{
			name: "failureRate out of range",
			profile: func() *WorkloadProfile {
				p := validJobWorkloadProfile()
				rate := 1.5
				p.Spec.Workloads[0].Template.(*JobTemplate).FailureRate = &rate
				return p
			}(),
			wantErr:     true,
			errContains: "failureRate must be between 0.0 and 1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWorkloadProfile(tt.profile)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWorkloadProfile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateWorkloadProfile() error = %v, expected to contain %q", err, tt.errContains)
				}
			}
		})
	}
}

func TestValidateArrivalPattern(t *testing.T) {
	tests := []struct {
		name        string
		pattern     ArrivalPattern
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid constant",
			pattern: ArrivalPattern{Type: "constant", RatePerMinute: floatPtr(10)},
			wantErr: false,
		},
		{
			name:    "valid poisson",
			pattern: ArrivalPattern{Type: "poisson", RatePerMinute: floatPtr(5.5)},
			wantErr: false,
		},
		{
			name:        "unsupported type",
			pattern:     ArrivalPattern{Type: "bursty"},
			wantErr:     true,
			errContains: "unsupported type \"bursty\"",
		},
		{
			name:        "missing ratePerMinute",
			pattern:     ArrivalPattern{Type: "constant"},
			wantErr:     true,
			errContains: "ratePerMinute is required",
		},
		{
			name:        "zero ratePerMinute",
			pattern:     ArrivalPattern{Type: "poisson", RatePerMinute: floatPtr(0)},
			wantErr:     true,
			errContains: "ratePerMinute must be > 0",
		},
		{
			name:        "negative ratePerMinute",
			pattern:     ArrivalPattern{Type: "constant", RatePerMinute: floatPtr(-1)},
			wantErr:     true,
			errContains: "ratePerMinute must be > 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateArrivalPattern(&tt.pattern)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateArrivalPattern() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("validateArrivalPattern() error = %v, expected to contain %q", err, tt.errContains)
				}
			}
		})
	}
}

func TestValidateDistribution(t *testing.T) {
	tests := []struct {
		name        string
		dist        Distribution
		wantErr     bool
		errContains string
	}{
		{
			name:    "fixed value",
			dist:    Distribution{Value: "8"},
			wantErr: false,
		},
		{
			name:    "valid uniform",
			dist:    Distribution{Type: "uniform", Min: "1", Max: "4"},
			wantErr: false,
		},
		{
			name:        "uniform missing min",
			dist:        Distribution{Type: "uniform", Max: "4"},
			wantErr:     true,
			errContains: "uniform distribution requires min and max",
		},
		{
			name:        "uniform missing max",
			dist:        Distribution{Type: "uniform", Min: "1"},
			wantErr:     true,
			errContains: "uniform distribution requires min and max",
		},
		{
			name:    "valid normal",
			dist:    Distribution{Type: "normal", Mean: "10", Stddev: "2"},
			wantErr: false,
		},
		{
			name:        "normal missing mean",
			dist:        Distribution{Type: "normal", Stddev: "2"},
			wantErr:     true,
			errContains: "normal distribution requires mean and stddev",
		},
		{
			name:        "normal missing stddev",
			dist:        Distribution{Type: "normal", Mean: "10"},
			wantErr:     true,
			errContains: "normal distribution requires mean and stddev",
		},
		{
			name:    "valid lognormal",
			dist:    Distribution{Type: "lognormal", Mean: "20m", Stddev: "10m"},
			wantErr: false,
		},
		{
			name:        "lognormal missing mean",
			dist:        Distribution{Type: "lognormal", Stddev: "10m"},
			wantErr:     true,
			errContains: "lognormal distribution requires mean and stddev",
		},
		{
			name:    "valid choice",
			dist:    Distribution{Type: "choice", Values: []string{"2", "4", "8"}},
			wantErr: false,
		},
		{
			name:    "valid choice with weights",
			dist:    Distribution{Type: "choice", Values: []string{"2", "4"}, Weights: []int{70, 30}},
			wantErr: false,
		},
		{
			name:        "choice missing values",
			dist:        Distribution{Type: "choice"},
			wantErr:     true,
			errContains: "choice distribution requires values",
		},
		{
			name:        "choice weights length mismatch",
			dist:        Distribution{Type: "choice", Values: []string{"2", "4"}, Weights: []int{70}},
			wantErr:     true,
			errContains: "weights length (1) must match values length (2)",
		},
		{
			name:        "empty distribution",
			dist:        Distribution{},
			wantErr:     true,
			errContains: "distribution value or type is required",
		},
		{
			name:        "unsupported distribution type",
			dist:        Distribution{Type: "exponential"},
			wantErr:     true,
			errContains: "unsupported distribution type \"exponential\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDistribution(&tt.dist, "test-field")
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDistribution() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("validateDistribution() error = %v, expected to contain %q", err, tt.errContains)
				}
			}
		})
	}
}

func TestValidateJobTemplate(t *testing.T) {
	tests := []struct {
		name        string
		template    JobTemplate
		wantErr     bool
		errContains string
	}{
		{
			name: "valid with fixed resources",
			template: JobTemplate{
				Resources: &ResourceRequirements{
					Requests: map[string]Distribution{
						"cpu": {Value: "4"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid with distribution resources and optional fields",
			template: JobTemplate{
				Resources: &ResourceRequirements{
					Requests: map[string]Distribution{
						"nvidia.com/gpu": {Type: "uniform", Min: "1", Max: "4"},
					},
				},
				Parallelism: &Distribution{Value: "1"},
				Completions: &Distribution{Value: "1"},
			},
			wantErr: false,
		},
		{
			name:        "missing resources",
			template:    JobTemplate{},
			wantErr:     true,
			errContains: "template.resources is required",
		},
		{
			name: "empty requests",
			template: JobTemplate{
				Resources: &ResourceRequirements{
					Requests: map[string]Distribution{},
				},
			},
			wantErr:     true,
			errContains: "requests must not be empty",
		},
		{
			name: "invalid parallelism distribution",
			template: JobTemplate{
				Resources: &ResourceRequirements{
					Requests: map[string]Distribution{
						"cpu": {Value: "4"},
					},
				},
				Parallelism: &Distribution{Type: "uniform", Min: "1"},
			},
			wantErr:     true,
			errContains: "uniform distribution requires min and max",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateJobTemplate(&tt.template, 0)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateJobTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("validateJobTemplate() error = %v, expected to contain %q", err, tt.errContains)
				}
			}
		})
	}
}

func TestValidateJobSetTemplate(t *testing.T) {
	tests := []struct {
		name        string
		template    JobSetTemplate
		wantErr     bool
		errContains string
	}{
		{
			name: "valid with single replicated job",
			template: JobSetTemplate{
				ReplicatedJobs: []ReplicatedJobTemplate{
					{
						Name: "workers",
						Resources: &ResourceRequirements{
							Requests: map[string]Distribution{
								"nvidia.com/gpu": {Value: "8"},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid with replicas distribution",
			template: JobSetTemplate{
				ReplicatedJobs: []ReplicatedJobTemplate{
					{
						Name: "workers",
						Resources: &ResourceRequirements{
							Requests: map[string]Distribution{
								"nvidia.com/gpu": {Value: "8"},
							},
						},
						Replicas: &Distribution{Type: "choice", Values: []string{"2", "4", "8"}},
					},
				},
			},
			wantErr: false,
		},
		{
			name:        "missing replicatedJobs",
			template:    JobSetTemplate{},
			wantErr:     true,
			errContains: "template.replicatedJobs is required",
		},
		{
			name: "missing replicated job name",
			template: JobSetTemplate{
				ReplicatedJobs: []ReplicatedJobTemplate{
					{
						Resources: &ResourceRequirements{
							Requests: map[string]Distribution{
								"cpu": {Value: "4"},
							},
						},
					},
				},
			},
			wantErr:     true,
			errContains: "name is required",
		},
		{
			name: "missing replicated job resources",
			template: JobSetTemplate{
				ReplicatedJobs: []ReplicatedJobTemplate{
					{Name: "workers"},
				},
			},
			wantErr:     true,
			errContains: "resources is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateJobSetTemplate(&tt.template, 0)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateJobSetTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("validateJobSetTemplate() error = %v, expected to contain %q", err, tt.errContains)
				}
			}
		})
	}
}

func TestValidateRayJobTemplate(t *testing.T) {
	tests := []struct {
		name        string
		template    RayJobTemplate
		wantErr     bool
		errContains string
	}{
		{
			name: "valid RayJob template",
			template: RayJobTemplate{
				HeadResources: &ResourceRequirements{
					Requests: map[string]Distribution{
						"cpu":    {Value: "4"},
						"memory": {Value: "16Gi"},
					},
				},
				WorkerResources: &ResourceRequirements{
					Requests: map[string]Distribution{
						"nvidia.com/gpu": {Value: "1"},
					},
				},
				WorkerReplicas: &Distribution{Type: "uniform", Min: "2", Max: "16"},
			},
			wantErr: false,
		},
		{
			name: "missing headResources",
			template: RayJobTemplate{
				WorkerResources: &ResourceRequirements{
					Requests: map[string]Distribution{
						"nvidia.com/gpu": {Value: "1"},
					},
				},
			},
			wantErr:     true,
			errContains: "template.headResources is required",
		},
		{
			name: "missing workerResources",
			template: RayJobTemplate{
				HeadResources: &ResourceRequirements{
					Requests: map[string]Distribution{
						"cpu": {Value: "4"},
					},
				},
			},
			wantErr:     true,
			errContains: "template.workerResources is required",
		},
		{
			name: "invalid workerReplicas distribution",
			template: RayJobTemplate{
				HeadResources: &ResourceRequirements{
					Requests: map[string]Distribution{
						"cpu": {Value: "4"},
					},
				},
				WorkerResources: &ResourceRequirements{
					Requests: map[string]Distribution{
						"nvidia.com/gpu": {Value: "1"},
					},
				},
				WorkerReplicas: &Distribution{Type: "uniform", Min: "2"},
			},
			wantErr:     true,
			errContains: "uniform distribution requires min and max",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRayJobTemplate(&tt.template, 0)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRayJobTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("validateRayJobTemplate() error = %v, expected to contain %q", err, tt.errContains)
				}
			}
		})
	}
}
