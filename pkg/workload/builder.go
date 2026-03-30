package workload

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jhwagner/kueue-bench/pkg/config"
)

// Annotation and label keys injected on every generated workload.
const (
	labelProfile       = "kueue-bench.io/profile"
	labelRunID         = "kueue-bench.io/run-id"
	labelWorkloadType  = "kueue-bench.io/workload-type"
	labelWorkloadIndex = "kueue-bench.io/workload-index"

	labelQueue    = "kueue.x-k8s.io/queue-name"
	labelPriority = "kueue.x-k8s.io/priority-class-name"

	annotationDuration = "kwok.x-k8s.io/duration"

	// containerImage is used as a placeholder image for all simulated pods.
	// KWOK does not actually pull or run images; any valid string is accepted.
	containerImage = "gcr.io/kwok/kwok"
)

// GVRs for each supported workload type.
var (
	jobGVR    = schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}
	jobSetGVR = schema.GroupVersionResource{Group: "jobset.x-k8s.io", Version: "v1alpha2", Resource: "jobsets"}
	rayJobGVR = schema.GroupVersionResource{Group: "ray.io", Version: "v1", Resource: "rayjobs"}
)

// WorkloadBuilder builds an unstructured Kubernetes workload object from a WorkloadSpec.
type WorkloadBuilder interface {
	// Build constructs the workload object and returns it with its GVR.
	Build(spec *config.WorkloadSpec, profileName, runID string, index int, sampler *Sampler) (*unstructured.Unstructured, schema.GroupVersionResource, error)
}

// builderRegistry maps workload type names to their builders.
var builderRegistry = map[string]WorkloadBuilder{
	"Job":    &JobBuilder{},
	"JobSet": &JobSetBuilder{},
	"RayJob": &RayJobBuilder{},
}

// builderFor returns the registered builder for the given workload type.
func builderFor(workloadType string) (WorkloadBuilder, error) {
	b, ok := builderRegistry[workloadType]
	if !ok {
		return nil, fmt.Errorf("no builder registered for workload type %q", workloadType)
	}
	return b, nil
}

// workloadName generates the name for a workload.
func workloadName(runID string, index int) string {
	return fmt.Sprintf("kueue-bench-%s-%d", runID, index)
}

// commonLabels returns the standard kueue-bench labels for a workload.
func commonLabels(profileName, runID, workloadType string, index int) map[string]interface{} {
	return map[string]interface{}{
		labelProfile:       profileName,
		labelRunID:         runID,
		labelWorkloadType:  workloadType,
		labelWorkloadIndex: fmt.Sprintf("%d", index),
	}
}

// kwokToleration is injected on every generated workload so pods can be scheduled
// onto KWOK nodes, which always carry this taint.
var kwokToleration = map[string]interface{}{
	"key":      "kwok.x-k8s.io/node",
	"operator": "Equal",
	"value":    "fake",
	"effect":   "NoSchedule",
}

// workloadMeta holds the common metadata fields built for every workload.
type workloadMeta struct {
	name           string
	namespace      string
	labels         map[string]interface{}
	podAnnotations map[string]interface{} // applied to pod template metadata (e.g. kwok duration); nil if no duration
	tolerations    []interface{}
}

// buildMeta constructs the name, namespace, labels, and annotations shared by all workload types.
func buildMeta(spec *config.WorkloadSpec, profileName, runID string, index int, duration *config.Distribution, sampler *Sampler) (workloadMeta, error) {
	ns := spec.Namespace
	if ns == "" {
		ns = "default"
	}

	labels := commonLabels(profileName, runID, spec.Type, index)
	if spec.LocalQueue != "" {
		labels[labelQueue] = spec.LocalQueue
	}
	if spec.PriorityClass != "" {
		labels[labelPriority] = spec.PriorityClass
	}

	var podAnnotations map[string]interface{}
	if duration != nil {
		d, err := sampler.SampleDuration(duration)
		if err != nil {
			return workloadMeta{}, fmt.Errorf("duration: %w", err)
		}
		podAnnotations = map[string]interface{}{annotationDuration: d.String()}
	}

	tolerations := []interface{}{kwokToleration}
	for _, t := range spec.Tolerations {
		operator := t.Operator
		if operator == "" {
			operator = "Equal"
		}
		tolerations = append(tolerations, map[string]interface{}{
			"key":      t.Key,
			"operator": operator,
			"value":    t.Value,
			"effect":   t.Effect,
		})
	}

	return workloadMeta{
		name:           workloadName(runID, index),
		namespace:      ns,
		labels:         labels,
		podAnnotations: podAnnotations,
		tolerations:    tolerations,
	}, nil
}

// buildResourceRequirements samples resource requests and returns them as an unstructured
// resources map (e.g. {"requests": {"cpu": "4", "memory": "16Gi"}, "limits": {...}}).
// limits are set equal to requests so non-overcommittable resources (e.g. nvidia.com/gpu) pass validation.
// Returns an empty map if req is nil.
func buildResourceRequirements(req *config.ResourceRequirements, sampler *Sampler) (map[string]interface{}, error) {
	if req == nil {
		return map[string]interface{}{}, nil
	}
	resources := make(map[string]interface{}, len(req.Requests))
	for name, dist := range req.Requests {
		q, err := sampler.SampleQuantity(&dist)
		if err != nil {
			return nil, fmt.Errorf("resource %q: %w", name, err)
		}
		resources[name] = q.String()
	}
	return map[string]interface{}{"requests": resources, "limits": resources}, nil
}

// JobBuilder builds batch/v1 Job objects.
type JobBuilder struct{}

// Build constructs a batch/v1 Job from a WorkloadSpec with a JobTemplate.
func (b *JobBuilder) Build(spec *config.WorkloadSpec, profileName, runID string, index int, sampler *Sampler) (*unstructured.Unstructured, schema.GroupVersionResource, error) {
	tmpl, ok := spec.Template.(*config.JobTemplate)
	if !ok {
		return nil, jobGVR, fmt.Errorf("expected *config.JobTemplate, got %T", spec.Template)
	}

	meta, err := buildMeta(spec, profileName, runID, index, tmpl.Duration, sampler)
	if err != nil {
		return nil, jobGVR, fmt.Errorf("job: %w", err)
	}

	var parallelism int64 = 1
	if tmpl.Parallelism != nil {
		p, err := sampler.SampleInt(tmpl.Parallelism)
		if err != nil {
			return nil, jobGVR, fmt.Errorf("job parallelism: %w", err)
		}
		parallelism = p
	}

	var completions int64 = 1
	if tmpl.Completions != nil {
		c, err := sampler.SampleInt(tmpl.Completions)
		if err != nil {
			return nil, jobGVR, fmt.Errorf("job completions: %w", err)
		}
		completions = c
	}

	resources, err := buildResourceRequirements(tmpl.Resources, sampler)
	if err != nil {
		return nil, jobGVR, fmt.Errorf("job resources: %w", err)
	}

	objMeta := map[string]interface{}{
		"name":      meta.name,
		"namespace": meta.namespace,
		"labels":    meta.labels,
	}
	podTmplMeta := map[string]interface{}{
		"labels": meta.labels,
	}
	if len(meta.podAnnotations) > 0 {
		podTmplMeta["annotations"] = meta.podAnnotations
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata":   objMeta,
			"spec": map[string]interface{}{
				"parallelism": parallelism,
				"completions": completions,
				"template": map[string]interface{}{
					"metadata": podTmplMeta,
					"spec": map[string]interface{}{
						"restartPolicy": "Never",
						"tolerations":   meta.tolerations,
						"containers": []interface{}{
							map[string]interface{}{
								"name":      "workload",
								"image":     containerImage,
								"resources": resources,
							},
						},
					},
				},
			},
		},
	}
	return obj, jobGVR, nil
}

// JobSetBuilder builds jobset.x-k8s.io/v1alpha2 JobSet objects.
type JobSetBuilder struct{}

// Build constructs a JobSet from a WorkloadSpec with a JobSetTemplate.
func (b *JobSetBuilder) Build(spec *config.WorkloadSpec, profileName, runID string, index int, sampler *Sampler) (*unstructured.Unstructured, schema.GroupVersionResource, error) {
	tmpl, ok := spec.Template.(*config.JobSetTemplate)
	if !ok {
		return nil, jobSetGVR, fmt.Errorf("expected *config.JobSetTemplate, got %T", spec.Template)
	}

	meta, err := buildMeta(spec, profileName, runID, index, tmpl.Duration, sampler)
	if err != nil {
		return nil, jobSetGVR, fmt.Errorf("jobset: %w", err)
	}

	replicatedJobs := make([]interface{}, 0, len(tmpl.ReplicatedJobs))
	for _, rj := range tmpl.ReplicatedJobs {
		var replicas int64 = 1
		if rj.Replicas != nil {
			r, err := sampler.SampleInt(rj.Replicas)
			if err != nil {
				return nil, jobSetGVR, fmt.Errorf("replicatedJob %q replicas: %w", rj.Name, err)
			}
			replicas = r
		}

		resources, err := buildResourceRequirements(rj.Resources, sampler)
		if err != nil {
			return nil, jobSetGVR, fmt.Errorf("replicatedJob %q resources: %w", rj.Name, err)
		}

		innerPodMeta := map[string]interface{}{}
		if len(meta.podAnnotations) > 0 {
			innerPodMeta["annotations"] = meta.podAnnotations
		}
		replicatedJobs = append(replicatedJobs, map[string]interface{}{
			"name":     rj.Name,
			"replicas": replicas,
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"parallelism": int64(1),
					"completions": int64(1),
					"template": map[string]interface{}{
						"metadata": innerPodMeta,
						"spec": map[string]interface{}{
							"restartPolicy": "Never",
							"tolerations":   meta.tolerations,
							"containers": []interface{}{
								map[string]interface{}{
									"name":      "workload",
									"image":     containerImage,
									"resources": resources,
								},
							},
						},
					},
				},
			},
		})
	}

	jobSetMeta := map[string]interface{}{
		"name":      meta.name,
		"namespace": meta.namespace,
		"labels":    meta.labels,
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "jobset.x-k8s.io/v1alpha2",
			"kind":       "JobSet",
			"metadata":   jobSetMeta,
			"spec": map[string]interface{}{
				"replicatedJobs": replicatedJobs,
			},
		},
	}
	return obj, jobSetGVR, nil
}

// RayJobBuilder builds ray.io/v1 RayJob objects.
type RayJobBuilder struct{}

// Build constructs a RayJob from a WorkloadSpec with a RayJobTemplate.
func (b *RayJobBuilder) Build(spec *config.WorkloadSpec, profileName, runID string, index int, sampler *Sampler) (*unstructured.Unstructured, schema.GroupVersionResource, error) {
	tmpl, ok := spec.Template.(*config.RayJobTemplate)
	if !ok {
		return nil, rayJobGVR, fmt.Errorf("expected *config.RayJobTemplate, got %T", spec.Template)
	}

	meta, err := buildMeta(spec, profileName, runID, index, tmpl.Duration, sampler)
	if err != nil {
		return nil, rayJobGVR, fmt.Errorf("rayjob: %w", err)
	}

	headResources, err := buildResourceRequirements(tmpl.HeadResources, sampler)
	if err != nil {
		return nil, rayJobGVR, fmt.Errorf("rayjob head resources: %w", err)
	}

	var workerReplicas int64 = 1
	if tmpl.WorkerReplicas != nil {
		r, err := sampler.SampleInt(tmpl.WorkerReplicas)
		if err != nil {
			return nil, rayJobGVR, fmt.Errorf("rayjob workerReplicas: %w", err)
		}
		workerReplicas = r
	}

	workerResources, err := buildResourceRequirements(tmpl.WorkerResources, sampler)
	if err != nil {
		return nil, rayJobGVR, fmt.Errorf("rayjob worker resources: %w", err)
	}

	rayJobMeta := map[string]interface{}{
		"name":      meta.name,
		"namespace": meta.namespace,
		"labels":    meta.labels,
	}
	podTmplMeta := map[string]interface{}{}
	if len(meta.podAnnotations) > 0 {
		podTmplMeta["annotations"] = meta.podAnnotations
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "ray.io/v1",
			"kind":       "RayJob",
			"metadata":   rayJobMeta,
			"spec": map[string]interface{}{
				// entrypoint is required by the RayJob CRD but unused in KWOK simulation.
				"entrypoint": "",
				"rayClusterSpec": map[string]interface{}{
					"headGroupSpec": map[string]interface{}{
						"template": map[string]interface{}{
							"metadata": podTmplMeta,
							"spec": map[string]interface{}{
								"tolerations": meta.tolerations,
								"containers": []interface{}{
									map[string]interface{}{
										"name":      "ray-head",
										"image":     containerImage,
										"resources": headResources,
									},
								},
							},
						},
					},
					"workerGroupSpecs": []interface{}{
						map[string]interface{}{
							"groupName": "default-worker",
							"replicas":  workerReplicas,
							"template": map[string]interface{}{
								"metadata": podTmplMeta,
								"spec": map[string]interface{}{
									"tolerations": meta.tolerations,
									"containers": []interface{}{
										map[string]interface{}{
											"name":      "ray-worker",
											"image":     containerImage,
											"resources": workerResources,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	return obj, rayJobGVR, nil
}
