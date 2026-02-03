#!/usr/bin/env bash
# Complete a job by applying the kwok.x-k8s.io/complete=true label to its pods

set -euo pipefail

if [ $# -eq 0 ]; then
    echo "Usage: $0 <job-name> [namespace]"
    echo "Example: $0 my-job default"
    exit 1
fi

JOB_NAME="$1"
NAMESPACE="${2:-default}"

echo "Completing job '$JOB_NAME' in namespace '$NAMESPACE'..."

# Label all pods for this job
kubectl label pods \
    -l job-name="$JOB_NAME" \
    -n "$NAMESPACE" \
    kwok.x-k8s.io/complete=true \
    --overwrite

echo "Done! Pods labeled with kwok.x-k8s.io/complete=true"
