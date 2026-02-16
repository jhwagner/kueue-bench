# kueue-bench

A CLI tool for creating and managing local Kueue test environments using kind and [kwok](https://kwok.sigs.k8s.io/).

## Prerequisites

**To use kueue-bench:**
- [Docker](https://docs.docker.com/get-docker/)

**To build from source:**
- [Go](https://golang.org/doc/install) (v1.21+)
- [Make](https://www.gnu.org/software/make/)

> kueue-bench uses kind and Helm as Go libraries (no external CLI dependencies required). Kind and Helm versions are compiled into the binary.

## Quick Start

### Create a Topology

Use one of the example configurations:

```bash
kueue-bench topology create single-cluster \
  --file examples/single-cluster.yaml
```

This creates a single kind cluster with:
- 10 simulated CPU nodes (via Kwok)
- Kueue controller and CRDs installed
- A simple CPU-only ResourceFlavor, ClusterQueue, and LocalQueue

### List Topologies

List currently running topologies (topology metadata is stored in `~/.kueue-bench/topologies/`

```bash
kueue-bench topology list
```

### Test with a sample job

Node pools in the cluster are tainted with `kwok.x-k8s.io/node` to prevent real workloads from running on them (e.g. the Kueue controller), so be sure to add a toleration. Pod lifecycle is completely simulated and managed by Kwok [stages](https://kwok.sigs.k8s.io/docs/user/stages-configuration/), so any logic will not actually run.

```bash
cat <<EOF | kubectl create -f -
apiVersion: batch/v1
kind: Job
metadata:
  generateName: test-job-
  namespace: default
spec:
  parallelism: 2
  completions: 2
  template:
    metadata:
      labels:
        kueue.x-k8s.io/queue-name: default-lq
    spec:
      containers:
      - name: sleep
        image: busybox
        command: ["sleep", "10"]
        resources:
          requests:
            cpu: "1"
            memory: "1Gi"
      tolerations:
      - key: kwok.x-k8s.io/node
        operator: Equal
        value: "fake"
        effect: NoSchedule
      restartPolicy: Never
EOF

# Check that Kueue admitted the workload
kubectl get workloads -A
```

### Delete a Topology

Clean up when you're done:

```bash
kueue-bench topology delete single-cluster
```

## Installation

### Using Docker

The easiest way to use kueue-bench is via Docker:

```bash
docker run -v /var/run/docker.sock:/var/run/docker.sock \
  -v ~/.kueue-bench:/root/.kueue-bench \
  ghcr.io/jhwagner/kueue-bench:main topology create single-cluster \
  --file examples/single-cluster.yaml
```

**Volume mounts explained:**
- `/var/run/docker.sock` - This is required for kind to create cluster containers as siblings on the host rather than nesting them within the kueue-bench container
- `~/.kueue-bench` - Persists topology metadata across runs

**Available tags:**
- `main` - Latest build from main branch
- `v*.*.*` - Stable versioned releases (e.g. `v1`, `v1.2`, `v1.2.3`)
- `sha-<commit>` - Specific commit builds

### Build from Source

```bash
git clone https://github.com/jhwagner/kueue-bench.git
cd kueue-bench
make install
```

This will install `kueue-bench` to your Go binaries directory, typically `$HOME/go/bin/` (make sure this is in your `$PATH`).

Alternatively, for local development builds, use:

```bash
make build
```

This will place the binary at `./bin/kueue-bench`.

## Examples

See the `examples/` directory:

- `single-cluster.yaml` — Basic single cluster setup
- `single-cluster-gpu.yaml` — Multi-pool setup with CPU and GPU nodes
- `single-cluster-with-extensions.yaml` — Installing additional components (e.g. JobSet) via Helm
- `multikueue.yaml` — MultiKueue topology with WorkerSets and automatic quota derivation

## Configuration

See [Topology Schema](docs/topology-schema.md) for the full configuration reference.

## Development

```bash
# Build binary to ./bin/
make build

# Run tests
make test

# Format and verify
make verify
```

## Project Structure

```
kueue-bench/
├── cmd/kueue-bench/    # CLI entry point and commands
├── pkg/                # Core library packages
│   ├── config/         # Topology schema and parsing
│   ├── cluster/        # kind cluster management
│   ├── kwok/           # Kwok installation and nodes
│   ├── kueue/          # Kueue installation and resources
│   └── topology/       # Topology orchestration
├── examples/           # Example topology files
└── docs/               # Documentation
```

## Planned Features

- Multi-cluster topologies
- Workload generation for load simulation
- Metrics and dashboards for visualizing results
- Benchmark runner and automated reports for quantitative analysis of Kueue configurations
