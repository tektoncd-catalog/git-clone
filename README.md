# Git Clone Task for Tekton

[![Artifact Hub Tasks](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/git-clone)](https://artifacthub.io/packages/search?repo=git-clone)
[![Artifact Hub StepActions](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/git-clone-stepaction)](https://artifacthub.io/packages/search?repo=git-clone-stepaction)

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

- **[Task reference](task/git-clone/README.md)** — full parameter, workspace, and authentication docs ([browse on Artifact Hub](https://artifacthub.io/packages/search?kind=7&repo=git-clone))
- **[StepAction reference](stepaction/git-clone/README.md)** — composable step version ([browse on Artifact Hub](https://artifacthub.io/packages/search?kind=11&repo=git-clone-stepaction))
- **[DEVELOPMENT.md](DEVELOPMENT.md)** — architecture, generation, testing, and release process
- **[CONTRIBUTING.md](CONTRIBUTING.md)** — contribution workflow and CI expectations
- **[AGENTS.md](AGENTS.md)** — quick reference for AI coding agents

## Building

To build the `git-init` image:

```bash
cd image/git-init
ko build --local .
```
