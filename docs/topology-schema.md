# Topology Schema Reference

This document describes the topology configuration format for kueue-bench.

## Overview

A topology defines one or more clusters with simulated node pools (via Kwok) and Kueue configurations. Topologies support two modes:

- **Standalone**: A single cluster with manually-configured Kueue objects
- **MultiKueue**: A management cluster with WorkerSets that define worker clusters. MultiKueue infrastructure (AdmissionChecks, MultiKueueConfigs, kubeconfig secrets) as well as Kueue resources (ClusterQueues, LocalQueues, quotas) across management and worker clusters are derived automatically.

## Example

```yaml
apiVersion: kueue-bench.io/v1alpha1
kind: Topology
metadata:
  name: my-topology
spec:
  kueue:
    version: "0.15.2"
  kwok:
    version: "v0.7.0"

  clusters:
    - name: cluster-1
      role: standalone
      nodePools: [...]
      kueue: {...}
```

See the `examples/` directory for complete working examples.

---

## Schema

### Top Level

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `apiVersion` | string | Yes | Must be `kueue-bench.io/v1alpha1` |
| `kind` | string | Yes | Must be `Topology` |
| `metadata.name` | string | Yes | Topology name (used as prefix for cluster names) |
| `spec` | object | Yes | Topology specification |

### `spec`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `kueue` | object | No | Global Kueue installation settings |
| `kwok` | object | No | Global Kwok installation settings |
| `clusters` | array | Yes | List of clusters to create |
| `workerSets` | array | No | WorkerSet definitions for MultiKueue topologies |

### `spec.kueue`

Global Kueue installation settings applied to all clusters.

| Field | Type | Description |
|-------|------|-------------|
| `version` | string | Kueue Helm chart version (default: `"0.15.2"`) |
| `helmValues` | object | Additional Helm values passed to `helm install` (see upstream Kueue [chart](https://github.com/kubernetes-sigs/kueue/tree/main/charts/kueue) for configurable values) |

#### Helm Values Example

```yaml
spec:
  kueue:
    version: "0.15.2"
    helmValues:
      managerConfig:
        controllerManagerConfigYaml: |-
          integrations:
            frameworks:
              - "batch/job"
```

### `spec.kwok`

| Field | Type | Description |
|-------|------|-------------|
| `version` | string | Kwok version (default: `"v0.7.0"`) |

---

### `spec.clusters[]`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Cluster name (prefixed with topology name at creation) |
| `role` | string | Yes | `standalone`, `management`, or `worker` |
| `kubernetesVersion` | string | No | Kubernetes version for the kind cluster |
| `nodePools` | array | Yes | Simulated node pools (Kwok). At least one required. |
| `kueue` | object | No | Kueue objects for this cluster |
| `extensions` | array | No | Additional components to install |

**Roles:**
- `standalone` — Self-contained cluster with its own Kueue objects
- `management` — MultiKueue management cluster. Required when `workerSets` are defined. ResourceFlavors and ClusterQueues are derived from WorkerSets. User can define cohorts, localQueues, and priorityClasses.
- `worker` — Used internally for clusters expanded from WorkerSets. Not specified directly in topology files.

### `spec.clusters[].nodePools[]`

Simulated node pools using Kwok. Nodes are tainted with `kwok.x-k8s.io/node=fake:NoSchedule` to prevent real workloads from running on them.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Node pool name |
| `count` | integer | Yes | Number of nodes to create (must be > 0) |
| `resources` | object | Yes | Resource capacities per node (Kubernetes quantity format). At least one required. |
| `labels` | object | No | Labels applied to each node |
| `taints` | array | No | Additional taints applied to each node |

#### `resources`

Resource capacities for each node. Values must be valid Kubernetes quantities. Common resources:

- `cpu` — CPU cores (e.g. `"16"`, `"32"`)
- `memory` — Memory (e.g. `"64Gi"`, `"128Gi"`)
- `nvidia.com/gpu` — GPU count (e.g. `"4"`, `"8"`)
- Any extended resource (e.g. `"example.com/custom"`)

#### `taints[]`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `key` | string | Yes | Taint key |
| `value` | string | No | Taint value |
| `effect` | string | Yes | `NoSchedule`, `PreferNoSchedule`, or `NoExecute` |

### `spec.clusters[].extensions[]`

Extensions install additional components into a cluster after Kueue setup. Each extension must have a unique name within the cluster and specify exactly one of `helm` or `manifest`.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Extension name (must be unique within the cluster) |
| `helm` | object | No | Install via Helm chart |
| `manifest` | object | No | Install via raw manifest URL |

#### `extensions[].helm`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `chart` | string | Yes | Helm chart reference (e.g. `oci://...` or `repo/chart`) |
| `version` | string | No | Chart version |
| `releaseName` | string | No | Helm release name (defaults to extension name) |
| `namespace` | string | No | Target namespace |
| `createNamespace` | bool | No | Create namespace if missing (default: `true`) |
| `wait` | bool | No | Wait for resources to be ready (default: `true`) |
| `timeout` | string | No | Helm timeout (default: `"5m"`) |
| `set` | object | No | Helm `--set` key-value pairs |

#### `extensions[].manifest`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `url` | string | Yes | URL to a raw Kubernetes manifest (must be `http://` or `https://`). Applied via standard Kubernetes client |

---

### `spec.clusters[].kueue`

Kueue object definitions for a cluster.

| Field | Type | Description |
|-------|------|-------------|
| `cohorts` | array | Cohort hierarchy definitions |
| `resourceFlavors` | array | ResourceFlavor definitions |
| `clusterQueues` | array | ClusterQueue definitions |
| `localQueues` | array | LocalQueue definitions |
| `priorityClasses` | array | WorkloadPriorityClass definitions |

### `spec.clusters[].kueue.cohorts[]`

Cohorts enable resource sharing between ClusterQueues. Cohorts can form hierarchies via `parentName`.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Cohort name (must be unique) |
| `parentName` | string | No | Parent cohort name (must reference an existing cohort) |
| `resourceGroups` | array | No | Resource quotas at the cohort level (same structure as ClusterQueue resourceGroups) |
| `fairSharing` | object | No | Fair sharing configuration |

#### `fairSharing`

Used on both cohorts and ClusterQueues.

| Field | Type | Description |
|-------|------|-------------|
| `weight` | integer | Relative weight for fair sharing (higher = more share) |

### `spec.clusters[].kueue.resourceFlavors[]`

ResourceFlavors define node characteristics for scheduling.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Flavor name (must be unique) |
| `nodeLabels` | object | No | Node label selectors |
| `tolerations` | array | No | Kubernetes tolerations (standard `corev1.Toleration` format) |

### `spec.clusters[].kueue.clusterQueues[]`

ClusterQueues define cluster-wide resource quotas and scheduling policies.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Queue name (must be unique) |
| `cohort` | string | No | Cohort this queue belongs to (must reference an existing cohort) |
| `namespaceSelector` | object | No | Namespace selector (`{}` for all namespaces) |
| `preemption` | object | No | Preemption policies |
| `resourceGroups` | array | Yes | Resource groups and quotas |
| `admissionChecks` | array | No | AdmissionCheck names |
| `fairSharing` | object | No | Fair sharing configuration |

#### `namespaceSelector`

| Field | Type | Description |
|-------|------|-------------|
| `matchLabels` | object | Key-value label pairs. Use `{}` (empty) to match all namespaces. |

#### `preemption`

| Field | Type | Description |
|-------|------|-------------|
| `withinClusterQueue` | string | Policy within same queue (`Never`, `LowerPriority`, `LowerOrNewerEqualPriority`) |
| `reclaimWithinCohort` | string | Policy for reclaiming lent resources (`Never`, `LowerPriority`, `Any`) |
| `borrowWithinCohort` | object | Borrowing policy |

#### `preemption.borrowWithinCohort`

| Field | Type | Description |
|-------|------|-------------|
| `policy` | string | Borrowing policy (`Never`, `LowerPriority`) |
| `maxPriorityThreshold` | integer | Maximum priority that can be preempted when borrowing |

#### `resourceGroups[]`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `coveredResources` | array | Yes | Resource names managed by this group (e.g. `["cpu", "memory"]`) |
| `flavors` | array | Yes | Flavors and their quotas |

#### `resourceGroups[].flavors[]`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | ResourceFlavor name (must reference an existing ResourceFlavor) |
| `resources` | array | Yes | Quota per resource |

#### `resourceGroups[].flavors[].resources[]`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Resource name (e.g. `cpu`, `memory`, `nvidia.com/gpu`) |
| `nominalQuota` | string | Yes | Base quota (Kubernetes quantity format) |
| `borrowingLimit` | string | No | Maximum amount that can be borrowed from cohort |
| `lendingLimit` | string | No | Maximum amount that can be lent to cohort |

### `spec.clusters[].kueue.localQueues[]`

Namespace-scoped queues that route workloads to a ClusterQueue.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Queue name |
| `namespace` | string | Yes | Target namespace (created automatically). Defaults to `"default"` if empty. |
| `clusterQueue` | string | Yes | Parent ClusterQueue name (must reference an existing ClusterQueue) |

### `spec.clusters[].kueue.priorityClasses[]`

WorkloadPriorityClasses define scheduling priority for workloads.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Priority class name |
| `value` | integer | Yes | Priority value (higher = more priority) |
| `description` | string | No | Human-readable description |

---

## WorkerSets (MultiKueue)

WorkerSets define groups of homogeneous worker clusters for MultiKueue topologies. They use a **structure + derivation** model:

- The WorkerSet defines Kueue object **structure** (flavor names, CQ names, relationships)
- Each worker's node pools define **infrastructure** (labels, counts, resources)
- ResourceFlavor `nodeLabels` and `tolerations` are **derived** from the mapped node pool
- ClusterQueue quotas are **calculated** as `pool.count * pool.resources[resource]`

When WorkerSets are present, exactly one cluster with `role: management` is required.

### `spec.workerSets[]`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | WorkerSet name (must be unique; used for AdmissionCheck and MultiKueueConfig names) |
| `resourceFlavors` | array | Yes | Flavor definitions with node pool references. At least one required. |
| `clusterQueues` | array | Yes | ClusterQueue structure (quotas derived from pools). At least one required. |
| `localQueues` | array | No | LocalQueues created on each worker and derived for management cluster |
| `workers` | array | Yes | Worker definitions with per-worker node pools. At least one required. |

### `spec.workerSets[].resourceFlavors[]`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Flavor name |
| `nodePoolRef` | string | Yes | Node pool name to derive labels/tolerations from (must exist in each worker) |

### `spec.workerSets[].clusterQueues[]`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | ClusterQueue name |
| `cohort` | string | No | Cohort name (applies to both worker and management CQs) |
| `namespaceSelector` | object | No | Namespace selector |
| `preemption` | object | No | Preemption policies |
| `resourceGroups` | array | Yes | Resource groups (at least one required; quotas are derived, not specified) |
| `admissionChecks` | array | No | Additional AdmissionChecks (WorkerSet name is auto-added on management) |
| `fairSharing` | object | No | Fair sharing configuration |

### `spec.workerSets[].clusterQueues[].resourceGroups[]`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `coveredResources` | array | Yes | Resources managed (must exist in referenced node pools). At least one required. |
| `flavors` | array | Yes | Flavor references |

### `spec.workerSets[].clusterQueues[].resourceGroups[].flavors[]`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Flavor name (must match a `workerSets[].resourceFlavors[].name`) |

No quota fields. Quotas are derived as `pool.count * pool.resources[resource]` for each covered resource.

### `spec.workerSets[].workers[]`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Worker cluster name (must be unique, cannot conflict with cluster names) |
| `nodePools` | array | Yes | Node pools (must include pools referenced by `resourceFlavors[].nodePoolRef`). Same schema as `spec.clusters[].nodePools[]`. |

---

## Derived MultiKueue Objects

When WorkerSets are present, the following are automatically created on the management cluster:

| Object | Name | Description |
|--------|------|-------------|
| **Secret** | `{worker-name}-kubeconfig` | Worker kubeconfig in `kueue-system` namespace |
| **MultiKueueCluster** | `{worker-name}` | References the kubeconfig Secret |
| **MultiKueueConfig** | `{workerset-name}` | References all MultiKueueClusters in the set |
| **AdmissionCheck** | `{workerset-name}` | References the MultiKueueConfig, controller: `kueue.x-k8s.io/multikueue` |
| **ResourceFlavor** | `{flavor-name}` | Minimal flavor (name only) for MultiKueue routing |
| **ClusterQueue** | `{cq-name}` | Same name as WorkerSet CQ, with `admissionChecks: [{workerset-name}]` prepended, quota = sum of all worker quotas |
| **LocalQueue** | `{lq-name}` | Deduplicated from WorkerSet LocalQueues (by namespace/name) |

Management cluster CQs inherit all structural fields (cohort, namespaceSelector, preemption, fairSharing) from the WorkerSet CQ definition. User-defined objects from the management cluster's `kueue` section (cohorts, priorityClasses, additional ResourceFlavors/CQs/LQs) are merged after derived objects.
