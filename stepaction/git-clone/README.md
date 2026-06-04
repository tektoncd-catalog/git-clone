# `git-clone` StepAction

> This file is generated from the [Task](../../task/git-clone/). See the
> [Task README](../../task/git-clone/README.md) for full documentation.

A `StepAction` version of the git-clone Task. Use it as a step within your own
Task or Pipeline for more composability.

## Usage

```yaml
apiVersion: tekton.dev/v1
kind: TaskRun
metadata:
  generateName: clone-
spec:
  taskSpec:
    workspaces:
      - name: source
    steps:
      - ref:
          name: git-clone
        params:
          - name: url
            value: https://github.com/tektoncd-catalog/git-clone
          - name: output-path
            value: $(workspaces.source.path)
  workspaces:
    - name: source
      emptyDir: {}
```

## Key Differences from the Task

| Task | StepAction |
|------|------------|
| `workspaces.output` | `params.output-path` |
| `workspaces.ssh-directory` (optional, bound check) | `params.ssh-directory-path` (empty string = disabled) |
| `workspaces.basic-auth` (optional, bound check) | `params.basic-auth-path` (empty string = disabled) |
| `workspaces.ssl-ca-directory` (optional, bound check) | `params.ssl-ca-directory-path` (empty string = disabled) |
| `$(results.commit.path)` | `$(step.results.commit.path)` |

## Generation

This StepAction is generated from the Task to avoid duplication:

```bash
./hack/generate-stepaction.sh
```

Do not edit `stepaction/git-clone/git-clone.yaml` directly — edit the Task
and regenerate.
