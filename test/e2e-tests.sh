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

# Simple e2e test runner for git-clone task.
# Installs the task, runs all TaskRuns from tests/run.yaml, and waits for completion.
#
# Environment variables:
#   PIPELINE_VERSION  - Tekton Pipelines version to install (default: v1.12.0)
#   TIMEOUT           - Timeout for each TaskRun (default: 120s)
#   GIT_INIT_IMAGE    - Override the gitInitImage in the task (optional, for testing local builds)

set -euo pipefail

PIPELINE_VERSION="${PIPELINE_VERSION:-v1.12.0}"
TIMEOUT="${TIMEOUT:-120s}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo "--- Installing Tekton Pipelines ${PIPELINE_VERSION}"
kubectl apply --filename "https://github.com/tektoncd/pipeline/releases/download/${PIPELINE_VERSION}/release.yaml"
echo "--- Waiting for Tekton Pipelines to be ready"
kubectl wait --for=condition=available --timeout=120s deployment/tekton-pipelines-controller -n tekton-pipelines
kubectl wait --for=condition=available --timeout=120s deployment/tekton-pipelines-webhook -n tekton-pipelines

echo "--- Installing git-clone task"
if [[ -n "${GIT_INIT_IMAGE:-}" ]]; then
    echo "    Using locally built image: ${GIT_INIT_IMAGE}"
    sed "s|ghcr.io/tektoncd-catalog/git-clone:[^ \"]*|${GIT_INIT_IMAGE}|g" \
        "${ROOT_DIR}/task/git-clone/git-clone.yaml" | kubectl apply -f -
else
    kubectl apply -f "${ROOT_DIR}/task/git-clone/git-clone.yaml"
fi

echo "--- Installing git-clone stepaction"
if [[ -n "${GIT_INIT_IMAGE:-}" ]]; then
    sed "s|ghcr.io/tektoncd-catalog/git-clone:[^ \"]*|${GIT_INIT_IMAGE}|g" \
        "${ROOT_DIR}/stepaction/git-clone/git-clone.yaml" | kubectl apply -f -
else
    kubectl apply -f "${ROOT_DIR}/stepaction/git-clone/git-clone.yaml"
fi

echo "--- Creating test TaskRuns (task)"
kubectl apply -f "${ROOT_DIR}/task/git-clone/tests/run.yaml"

echo "--- Creating test TaskRuns (stepaction)"
kubectl apply -f "${ROOT_DIR}/stepaction/git-clone/tests/run.yaml"

# Collect all TaskRun names
TASKRUNS=$(kubectl get taskrun -o name | sed 's|taskrun.tekton.dev/||')

FAILED=0
PASSED=0
TOTAL=0

echo "--- Waiting for TaskRuns to complete (timeout: ${TIMEOUT})"
for tr in ${TASKRUNS}; do
    TOTAL=$((TOTAL + 1))
    echo -n "  ${tr} ... "
    if kubectl wait --for=condition=Succeeded --timeout="${TIMEOUT}" taskrun/"${tr}" 2>/dev/null; then
        echo "PASSED"
        PASSED=$((PASSED + 1))
    else
        echo "FAILED"
        echo "  --- TaskRun status ---"
        kubectl get taskrun/"${tr}" -o jsonpath='{.status.conditions[*].message}' 2>/dev/null || true
        echo ""
        echo "  --- Pod logs ---"
        pod=$(kubectl get taskrun/"${tr}" -o jsonpath='{.status.podName}' 2>/dev/null)
        if [[ -n "${pod}" ]]; then
            kubectl logs "${pod}" --all-containers 2>/dev/null || true
        fi
        echo "  ---"
        FAILED=$((FAILED + 1))
    fi
done

echo ""
echo "=== Results: ${PASSED}/${TOTAL} passed, ${FAILED} failed ==="

if [[ ${FAILED} -gt 0 ]]; then
    exit 1
fi
