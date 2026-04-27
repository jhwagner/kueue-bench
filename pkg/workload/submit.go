package workload

import (
	"fmt"
	"math/rand/v2"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SubmitParams holds the parameters for a single workload submitted from the TUI.
type SubmitParams struct {
	Namespace     string
	Queue         string
	PriorityClass string
	CPU           resource.Quantity
	Memory        resource.Quantity
	GPU           resource.Quantity
	Replicas      int32  // JobSet only
	Duration      string // e.g. "60s"; empty means no kwok duration annotation
}

// BuildSubmitJob builds a minimal batch/v1 Job from TUI submit parameters.
func BuildSubmitJob(params SubmitParams) (*unstructured.Unstructured, schema.GroupVersionResource) {
	name := submitName()
	labels := submitLabels(params.Queue, params.PriorityClass)
	podAnnotations := submitPodAnnotations(params.Duration)
	resources := submitResources(params)

	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": params.Namespace,
			"labels":    labels,
		},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": podAnnotations,
				},
				"spec": map[string]interface{}{
					"restartPolicy": "Never",
					"tolerations":   []interface{}{kwokToleration},
					"containers": []interface{}{
						map[string]interface{}{
							"name":      "main",
							"image":     containerImage,
							"resources": resources,
						},
					},
				},
			},
		},
	}}
	return obj, jobGVR
}

// BuildSubmitJobSet builds a minimal jobset.x-k8s.io/v1alpha2 JobSet from TUI submit parameters.
func BuildSubmitJobSet(params SubmitParams) (*unstructured.Unstructured, schema.GroupVersionResource) {
	name := submitName()
	replicas := params.Replicas
	if replicas <= 0 {
		replicas = 2
	}
	labels := submitLabels(params.Queue, params.PriorityClass)
	podAnnotations := submitPodAnnotations(params.Duration)
	resources := submitResources(params)

	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "jobset.x-k8s.io/v1alpha2",
		"kind":       "JobSet",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": params.Namespace,
			"labels":    labels,
		},
		"spec": map[string]interface{}{
			"replicatedJobs": []interface{}{
				map[string]interface{}{
					"name":     "workers",
					"replicas": int64(replicas),
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"template": map[string]interface{}{
								"metadata": map[string]interface{}{
									"annotations": podAnnotations,
								},
								"spec": map[string]interface{}{
									"restartPolicy": "Never",
									"tolerations":   []interface{}{kwokToleration},
									"containers": []interface{}{
										map[string]interface{}{
											"name":      "main",
											"image":     containerImage,
											"resources": resources,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}}
	return obj, jobSetGVR
}

func submitName() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 6)
	for i := range b {
		b[i] = chars[rand.IntN(len(chars))]
	}
	return fmt.Sprintf("tui-%s", string(b))
}

func submitLabels(queue, priorityClass string) map[string]interface{} {
	labels := map[string]interface{}{
		"kueue-bench.io/source": "tui",
		labelQueue:              queue,
	}
	if priorityClass != "" {
		labels[labelPriority] = priorityClass
	}
	return labels
}

func submitPodAnnotations(duration string) map[string]interface{} {
	if duration == "" {
		return map[string]interface{}{}
	}
	return map[string]interface{}{annotationDuration: duration}
}

func submitResources(params SubmitParams) map[string]interface{} {
	requests := map[string]interface{}{}
	if !params.CPU.IsZero() {
		requests[string(corev1.ResourceCPU)] = params.CPU.String()
	}
	if !params.Memory.IsZero() {
		requests[string(corev1.ResourceMemory)] = params.Memory.String()
	}
	if !params.GPU.IsZero() {
		requests["nvidia.com/gpu"] = params.GPU.String()
	}
	if len(requests) == 0 {
		return nil
	}
	return map[string]interface{}{
		"requests": requests,
		"limits":   requests,
	}
}
