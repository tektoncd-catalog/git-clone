#!/usr/bin/env bash

# Copyright 2024 The Tekton Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# E2e test for Tekton Bundle publishing.
# Pushes the task as a bundle to ttl.sh, then runs a TaskRun that
# references it via the bundle resolver.
#
# Environment variables:
#   PIPELINE_VERSION  - Tekton Pipelines version to install (default: v1.12.0)
#   TIMEOUT           - Timeout for TaskRun (default: 120s)
#   GIT_INIT_IMAGE    - Override the gitInitImage in the task (optional)
#   BUNDLE_REGISTRY   - Registry to push bundles to (default: ttl.sh)

set -euo pipefail

PIPELINE_VERSION="${PIPELINE_VERSION:-v1.12.0}"
TIMEOUT="${TIMEOUT:-120s}"
BUNDLE_REGISTRY="${BUNDLE_REGISTRY:-ttl.sh}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Generate a unique bundle reference
BUNDLE_REF="${BUNDLE_REGISTRY}/git-clone-e2e-$(head -c 8 /proc/sys/kernel/random/uuid):1h"

echo "--- Installing Tekton Pipelines ${PIPELINE_VERSION}"
kubectl apply --filename "https://github.com/tektoncd/pipeline/releases/download/${PIPELINE_VERSION}/release.yaml"
echo "--- Waiting for Tekton Pipelines to be ready"
kubectl wait --for=condition=available --timeout=120s deployment/tekton-pipelines-controller -n tekton-pipelines
kubectl wait --for=condition=available --timeout=120s deployment/tekton-pipelines-webhook -n tekton-pipelines

# Prepare the task YAML (with optional image override)
TASK_YAML=$(mktemp)
if [[ -n "${GIT_INIT_IMAGE:-}" ]]; then
    echo "    Using locally built image: ${GIT_INIT_IMAGE}"
    sed "s|ghcr.io/tektoncd-catalog/git-clone:[^ \"]*|${GIT_INIT_IMAGE}|g" \
        "${ROOT_DIR}/task/git-clone/git-clone.yaml" > "${TASK_YAML}"
else
    cp "${ROOT_DIR}/task/git-clone/git-clone.yaml" "${TASK_YAML}"
fi

echo "--- Pushing Tekton Bundle to ${BUNDLE_REF}"
tkn bundle push "${BUNDLE_REF}" -f "${TASK_YAML}"

echo "--- Creating TaskRun using bundle resolver"
cat <<EOF | kubectl apply -f -
apiVersion: tekton.dev/v1
kind: TaskRun
metadata:
  name: git-clone-bundle-test
spec:
  taskRef:
    resolver: bundles
    params:
      - name: bundle
        value: ${BUNDLE_REF}
      - name: name
        value: git-clone
      - name: kind
        value: task
  workspaces:
    - name: output
      emptyDir: {}
  podTemplate:
    securityContext:
      fsGroup: 65532
  params:
    - name: url
      value: https://github.com/kelseyhightower/nocode
EOF

echo "--- Waiting for TaskRun to complete (timeout: ${TIMEOUT})"
if kubectl wait --for=condition=Succeeded --timeout="${TIMEOUT}" taskrun/git-clone-bundle-test 2>/dev/null; then
    echo ""
    echo "=== Bundle test PASSED ==="
else
    echo ""
    echo "=== Bundle test FAILED ==="
    kubectl get taskrun/git-clone-bundle-test -o jsonpath='{.status.conditions[*].message}' 2>/dev/null || true
    echo ""
    pod=$(kubectl get taskrun/git-clone-bundle-test -o jsonpath='{.status.podName}' 2>/dev/null)
    if [[ -n "${pod}" ]]; then
        kubectl logs "${pod}" --all-containers 2>/dev/null || true
    fi
    exit 1
fi

rm -f "${TASK_YAML}"
