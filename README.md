# Git Clone Task for Tekton

[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/git-clone)](https://artifacthub.io/packages/search?repo=git-clone)

This repository contains the `git-clone` [Task](task/git-clone/) and [StepAction](stepaction/git-clone/) for [Tekton Pipelines](https://tekton.dev/), providing Git repository cloning capabilities.

## Installation

Install the Task directly:

```bash
kubectl apply -f https://raw.githubusercontent.com/tektoncd-catalog/git-clone/main/task/git-clone/git-clone.yaml
```

Or use a [Tekton Bundle](https://tekton.dev/docs/pipelines/tekton-bundle-contracts/) with the bundle resolver:

```yaml
taskRef:
  resolver: bundles
  params:
    - name: bundle
      value: ghcr.io/tektoncd-catalog/git-clone/bundle:v1.7.0
    - name: name
      value: git-clone
    - name: kind
      value: task
```

## Quick Start

```yaml
apiVersion: tekton.dev/v1
kind: TaskRun
metadata:
  generateName: git-clone-
spec:
  taskRef:
    name: git-clone
  podTemplate:
    securityContext:
      fsGroup: 65532
  workspaces:
    - name: output
      emptyDir: {}
  params:
    - name: url
      value: https://github.com/tektoncd-catalog/git-clone
```

## Documentation

- **[Task reference](task/git-clone/README.md)** — full parameter, workspace, and authentication docs
- **[StepAction reference](stepaction/git-clone/README.md)** — composable step version

## Building

To build the `git-init` image:

```bash
cd image/git-init
ko build --local .
```
