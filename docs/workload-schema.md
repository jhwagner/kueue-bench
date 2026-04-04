# WorkloadProfile Schema Reference

This document describes the WorkloadProfile configuration format for `kueue-bench workload submit`.

## Overview

A WorkloadProfile defines a synthetic workload generation run: arrival rate, duration, and a weighted mix of workload types (Jobs, JobSets, RayJobs). The generation engine submits workloads against a running topology, exercising Kueue's admission, borrowing, preemption, and fair-sharing logic without running real containers — KWOK simulates pod lifecycle.

## Quick Start

```bash
# Create a topology
kueue-bench topology create -f examples/topologies/cohort-borrowing.yaml

# Submit workloads against it
kueue-bench workload submit \
  --topology cohort-borrowing \
  --profile examples/workloads/cohort-borrowing.yaml

# Dry run — prints what would be submitted without creating objects
kueue-bench workload submit \
  --topology cohort-borrowing \
  --profile examples/workloads/cohort-borrowing.yaml \
  --dry-run
```

See `examples/workloads/` for complete working profiles.

---

## Schema

### Top Level

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `apiVersion` | string | Yes | Must be `kueue-bench.io/v1alpha1` |
| `kind` | string | Yes | Must be `WorkloadProfile` |
| `metadata.name` | string | Yes | Profile name |
| `spec` | object | Yes | Profile specification |

### `spec`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `seed` | int | No | Random seed for reproducible runs. If omitted, a random seed is used |
| `duration` | duration | Yes | How long to generate workloads (e.g. `10m`, `1h`) |
| `arrivalPattern` | object | Yes | Controls submission timing |
| `workloads` | array | Yes | Workload type definitions with weights |

### `spec.arrivalPattern`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | `constant` or `poisson` |
| `ratePerMinute` | float | Yes | Average submissions per minute |

**`constant`**: fixed interval between submissions (`60s / ratePerMinute`).

**`poisson`**: exponentially-distributed inter-arrival times. More realistic for bursty workloads; same average rate as constant but with natural variance. Recommended for benchmark scenarios.

### `spec.workloads[]`

Each entry defines a workload type that participates in the weighted mix.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | `Job`, `JobSet`, or `RayJob` |
| `weight` | int | No | Relative probability of selecting this workload. Weights are relative (need not sum to 100). Defaults to uniform if all zero or unset |
| `localQueue` | string | Yes | Name of the LocalQueue to target |
| `namespace` | string | No | Namespace for the workload. Defaults to `default` |
| `priorityClass` | string | No | WorkloadPriorityClass name to assign |
| `template` | object | Yes | Workload-type-specific configuration |

### `spec.workloads[].template` — Job

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `parallelism` | int or Distribution | No | Number of parallel pods. Defaults to 1 |
| `completions` | int or Distribution | No | Required completions. Defaults to 1 |
| `resources.requests` | map | Yes | Resource requests per pod. Values are quantities or Distributions |
| `duration` | Distribution | Yes | Simulated runtime (KWOK completes pods after this duration) |

### `spec.workloads[].template` — JobSet

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `replicatedJobs` | array | Yes | List of replicated job specs |
| `replicatedJobs[].name` | string | Yes | Job name within the JobSet |
| `replicatedJobs[].replicas` | int or Distribution | No | Number of replicas. Defaults to 1 |
| `replicatedJobs[].resources.requests` | map | Yes | Resource requests per pod |
| `replicatedJobs[].duration` | Distribution | Yes | Simulated runtime |

### `spec.workloads[].template` — RayJob

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `headResources` | map | Yes | Resource requests for the Ray head pod |
| `workerGroups` | array | Yes | Worker group specs |
| `workerGroups[].name` | string | Yes | Worker group name |
| `workerGroups[].replicas` | int or Distribution | No | Number of workers. Defaults to 1 |
| `workerGroups[].resources` | map | Yes | Resource requests per worker pod |
| `duration` | Distribution | Yes | Simulated runtime for all pods in the RayJob |

---

## Distribution Types

Distributions can be used anywhere a quantity or integer is accepted. A bare string/number is treated as a fixed value.

### Fixed

```yaml
nvidia.com/gpu: "4"
```

### Uniform

Samples uniformly between `min` and `max` (inclusive for integers).

```yaml
nvidia.com/gpu: { distribution: uniform, min: "2", max: "8" }
cpu: { distribution: uniform, min: "4", max: "16" }
duration: { distribution: uniform, min: "30s", max: "90s" }
```

### Normal

Samples from a normal distribution. Values are clamped to prevent negative quantities.

```yaml
duration: { distribution: normal, mean: "60s", stddev: "15s" }
```

### Lognormal

Samples from a log-normal distribution. Recommended for job durations — produces realistic right-skewed distributions with a long tail of occasional long jobs.

```yaml
duration: { distribution: lognormal, mean: "45s", stddev: "15s" }
```

`mean` and `stddev` are the desired mean and standard deviation of the resulting distribution (not of the underlying normal). The engine converts these to the standard μ/σ parameterization internally.

### Choice

Samples uniformly from a discrete list of values. Optional `weights` make selection non-uniform (relative, need not sum to 100).

```yaml
nvidia.com/gpu: { distribution: choice, values: ["1", "2", "4", "8"] }
nvidia.com/gpu: { distribution: choice, values: ["2", "4", "8"], weights: [5, 3, 1] }
```

---

## Simulation Timing Guidelines

These guidelines ensure simulation runs complete in reasonable wall time while generating meaningful scheduling signals.

**Job durations: 15–90 seconds**

Shorter durations mean faster runs and more scheduling events per minute. Avoid durations longer than 2 minutes in example profiles — multi-minute jobs cause long drain tails after the submission window ends.

The practical floor is ~15s: below this, k8s machinery overhead (~1-3s on kind) becomes a significant fraction of measured queue wait times, degrading signal quality.

**Profile duration: 10–20 minutes**

This is sufficient to reach steady-state saturation and collect 50–100+ admissions for statistically meaningful p50/p95 metrics. Longer profiles don't add proportionally more signal.

**Saturation ratio**

```
steady_state_demand = ratePerMinute × avg_resource_per_job × avg_duration_minutes
saturation_ratio    = steady_state_demand / queue_quota
```

For scenarios demonstrating borrowing or preemption, target `saturation_ratio > 1.0` for at least one queue. A ratio of 1.2–1.5× creates sustained backlog without infinite drain tails. Ratios > 2.0× cause drain times that dominate total wall time.

**kind API server ceiling**

The kind kube-apiserver (running in Docker) is the practical rate ceiling. Keep total arrival rate across all workload specs below 15–20 jobs/min to avoid API latency artifacts in queue wait time measurements.

---

## Example: Cohort Borrowing

Two teams in a cohort. Team A runs below quota; Team B consistently exceeds its quota and borrows from Team A's idle capacity.

```yaml
apiVersion: kueue-bench.io/v1alpha1
kind: WorkloadProfile
metadata:
  name: cohort-borrowing
spec:
  seed: 100
  duration: 15m
  arrivalPattern:
    type: poisson
    ratePerMinute: 14
  workloads:
    - type: Job
      weight: 30            # ~4.2 jobs/min → ~16 GPU demand (40% of Team A's 40 GPU quota)
      localQueue: team-a-lq
      namespace: team-a
      template:
        resources:
          requests:
            nvidia.com/gpu: { distribution: uniform, min: "2", max: "8" }
        duration: { distribution: lognormal, mean: "45s", stddev: "15s" }
    - type: Job
      weight: 70            # ~9.8 jobs/min → ~59 GPU demand (147% of Team B's 40 GPU quota)
      localQueue: team-b-lq
      namespace: team-b
      template:
        resources:
          requests:
            nvidia.com/gpu: { distribution: uniform, min: "4", max: "8" }
        duration: { distribution: lognormal, mean: "60s", stddev: "20s" }
```

See `examples/workloads/cohort-borrowing.yaml` and `examples/topologies/cohort-borrowing.yaml` for the full paired example.

---

## Auto-Injected Labels and Annotations

The engine automatically injects the following on every submitted workload:

| Key | Value |
|-----|-------|
| `kueue-bench.io/profile` | WorkloadProfile name |
| `kueue-bench.io/run-id` | Unique ID for this submission run |
| `kueue-bench.io/workload-type` | `job`, `jobset`, or `rayjob` |
| `kueue-bench.io/workload-index` | Sequential index within the run |
| `kueue.x-k8s.io/queue-name` | Value of `localQueue` field |
| `kwok.x-k8s.io/duration` | Sampled job duration (for KWOK pod completion) |
