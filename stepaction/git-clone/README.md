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

## Parameters

| Parameter | Description | Default |
|-----------|-------------|--------|
| `output-path` | Path to clone the git repo into | _(required)_ |
| `url` | Repository URL to clone from | _(required)_ |
| `ssh-directory-path` | Path to `.ssh` directory with private key, known_hosts, config | `""` |
| `basic-auth-path` | Path to directory with `.gitconfig` and `.git-credentials` | `""` |
| `ssl-ca-directory-path` | Path to directory with CA certificates | `""` |
| `revision` | Revision to checkout (branch, tag, sha, ref…) | `""` |
| `refspec` | Refspec to fetch before checking out revision | `""` |
| `submodules` | Initialize and fetch git submodules | `"true"` |
| `submodulePaths` | Comma-separated list of submodule paths to fetch | `""` |
| `depth` | Shallow clone depth | `"1"` |
| `sslVerify` | Set `http.sslVerify` global git config | `"true"` |
| `crtFileName` | Certificate file name in `ssl-ca-directory-path` | `"ca-bundle.crt"` |
| `subdirectory` | Subdirectory inside `output-path` to clone into | `""` |
| `sparseCheckoutDirectories` | Directory patterns for sparse checkout | `""` |
| `deleteExisting` | Clean destination directory before cloning | `"true"` |
| `httpProxy` | HTTP proxy server for non-SSL requests | `""` |
| `httpsProxy` | HTTPS proxy server for SSL requests | `""` |
| `noProxy` | Opt out of proxying HTTP/HTTPS requests | `""` |
| `verbose` | Log the commands executed during operation | `"true"` |
| `userFriendlyErrors` | Print user-friendly error messages with hints | `"true"` |
| `gitInitImage` | The image providing the `git-init` binary | `"ghcr.io/tektoncd-catalog/git-clone:v1.5.0"` |
| `userHome` | Absolute path to the user's home directory | `"/tekton/home"` |

## Results

| Result | Description |
|--------|-------------|
| `commit` | The precise commit SHA that was fetched |
| `url` | The precise URL that was fetched |
| `committer-date` | The epoch timestamp of the fetched commit |

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
