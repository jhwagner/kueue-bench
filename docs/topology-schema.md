# Topology Schema Reference

This document describes the topology configuration format for kueue-bench.

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

## Schema

### `spec.kueue`

Kueue installation settings. All fields are optional - if not specified, defaults are used.

| Field | Type | Description |
|-------|------|-------------|
| `version` | string | Kueue Helm chart version (default: "0.15.2") |
| `imageRepository` | string | Custom Kueue image repository (useful for testing custom builds) |
| `imageTag` | string | Custom Kueue image tag (useful for testing custom builds) |

### `spec.kwok`

Kwok installation settings. All fields are optional - if not specified, defaults are used.

| Field | Type | Description |
|-------|------|-------------|
| `version` | string | Kwok version (default: "v0.7.0") |

### `spec.clusters[]`

List of clusters to create.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Cluster name (will be prefixed with topology name) |
| `role` | string | Yes | Cluster role (currently only "standalone" supported) |
| `nodePools` | array | Yes | List of node pools to create |
| `kueue` | object | Yes | Kueue configuration for this cluster |

### `spec.clusters[].nodePools[]`

Simulated node pools using Kwok.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Node pool name |
| `count` | integer | Yes | Number of nodes to create |
| `resources` | object | Yes | Node resources (cpu, memory, etc.) |
| `labels` | object | No | Labels to apply to nodes |
| `taints` | array | No | Taints to apply to nodes |

#### `resources`

Resource capacities for each node. Common resources:

- `cpu` - CPU cores (e.g., "16", "32")
- `memory` - Memory capacity (e.g., "64Gi", "128Gi")
- `nvidia.com/gpu` - GPU count (e.g., "4")
- Any extended resource (e.g., "example.com/custom")

#### `taints[]`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `key` | string | Yes | Taint key |
| `value` | string | No | Taint value |
| `effect` | string | Yes | Effect: "NoSchedule", "PreferNoSchedule", or "NoExecute" |

### `spec.clusters[].kueue`

Kueue configuration objects.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `resourceFlavors` | array | Yes | Resource flavors to create |
| `clusterQueues` | array | Yes | Cluster queues to create |
| `localQueues` | array | Yes | Local queues to create |
| `workloadPriorityClasses` | array | No | Workload priority classes |

### `spec.clusters[].kueue.resourceFlavors[]`

Resource flavors define node characteristics for scheduling.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Flavor name |
| `nodeLabels` | object | No | Node label selectors |
| `tolerations` | array | No | Tolerations for this flavor |

### `spec.clusters[].kueue.clusterQueues[]`

Cluster-wide queue configuration.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Queue name |
| `namespaceSelector` | object | Yes | Namespace selector (use `{}` for all) |
| `resourceGroups` | array | Yes | Resource groups and quotas |

#### `resourceGroups[]`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `coveredResources` | array | Yes | Resources managed by this group |
| `flavors` | array | Yes | Flavors and their quotas |

#### `resourceGroups[].flavors[]`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Resource flavor name |
| `resources` | array | Yes | Quota per resource type |

#### `resourceGroups[].flavors[].resources[]`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Resource name (cpu, memory, etc.) |
| `nominalQuota` | string | Yes | Base quota amount |
| `borrowingLimit` | string | No | Maximum borrowing from other queues |
| `lendingLimit` | string | No | Maximum lending to other queues |

### `spec.clusters[].kueue.localQueues[]`

Namespace-scoped queues.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Queue name |
| `namespace` | string | Yes | Target namespace |
| `clusterQueue` | string | Yes | Parent cluster queue |

### `spec.clusters[].kueue.workloadPriorityClasses[]`

Priority classes for workloads.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Priority class name |
| `value` | integer | Yes | Priority value (higher = more priority) |
| `description` | string | No | Human-readable description |
