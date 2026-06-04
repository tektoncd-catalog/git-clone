# Git Clone Task for Tekton

[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/git-clone)](https://artifacthub.io/packages/search?repo=git-clone)

This repository contains the `git-clone` Task for [Tekton Pipelines](https://tekton.dev/), providing Git repository cloning capabilities.

The `git-clone` Task clones a repo from the provided URL into the output Workspace.
By default the repo is cloned into the root of the Workspace. You can clone into a
subdirectory by setting the `subdirectory` param. This Task also supports sparse
checkouts via the `sparseCheckoutDirectories` param.

## Installation

Install the Task directly:

```bash
kubectl apply -f https://raw.githubusercontent.com/tektoncd-catalog/git-clone/main/task/git-clone/git-clone.yaml
```

Or use the [Tekton Bundle](https://tekton.dev/docs/pipelines/tekton-bundle-contracts/) with the bundle resolver:

```yaml
taskRef:
  resolver: bundles
  params:
    - name: bundle
      value: ghcr.io/tektoncd-catalog/git-clone/bundle:v1.5.0
    - name: name
      value: git-clone
    - name: kind
      value: task
```

## Usage

### Basic clone

```yaml
apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  generateName: git-clone-run-
spec:
  pipelineSpec:
    workspaces:
      - name: shared-data
    tasks:
      - name: fetch-source
        taskRef:
          name: git-clone
        workspaces:
          - name: output
            workspace: shared-data
        params:
          - name: url
            value: https://github.com/tektoncd-catalog/git-clone
  workspaces:
    - name: shared-data
      volumeClaimTemplate:
        spec:
          accessModes:
            - ReadWriteOnce
          resources:
            requests:
              storage: 1Gi
```

## Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `url` | Repository URL to clone from | _(required)_ |
| `revision` | Revision to checkout (branch, tag, sha, ref…) | `""` |
| `refspec` | Refspec to fetch before checking out revision | `""` |
| `submodules` | Initialize and fetch git submodules | `"true"` |
| `submodulePaths` | Comma-separated list of submodule paths to fetch | `""` |
| `depth` | Shallow clone depth | `"1"` |
| `sslVerify` | Set `http.sslVerify` global git config | `"true"` |
| `crtFileName` | Certificate file name in `ssl-ca-directory` workspace | `"ca-bundle.crt"` |
| `subdirectory` | Subdirectory inside the `output` Workspace to clone into | `""` |
| `sparseCheckoutDirectories` | Directory patterns for sparse checkout | `""` |
| `deleteExisting` | Clean destination directory before cloning | `"true"` |
| `httpProxy` | HTTP proxy server for non-SSL requests | `""` |
| `httpsProxy` | HTTPS proxy server for SSL requests | `""` |
| `noProxy` | Opt out of proxying HTTP/HTTPS requests | `""` |
| `verbose` | Log the commands executed during operation | `"true"` |
| `userFriendlyErrors` | Print user-friendly error messages with hints | `"true"` |
| `gitInitImage` | The image providing the `git-init` binary | `"ghcr.io/tektoncd-catalog/git-clone:v1.5.0"` |
| `userHome` | Absolute path to the user's home directory | `"/home/git"` |

## Workspaces

| Workspace | Description | Optional |
|-----------|-------------|----------|
| `output` | The git repo will be cloned onto the volume backing this Workspace | No |
| `ssh-directory` | A `.ssh` directory with private key, `known_hosts`, config, etc. | Yes |
| `basic-auth` | A Workspace containing `.gitconfig` and `.git-credentials` files | Yes |
| `ssl-ca-directory` | A workspace containing CA certificates for HTTPS verification | Yes |

## Results

| Result | Description |
|--------|-------------|
| `commit` | The precise commit SHA that was fetched |
| `url` | The precise URL that was fetched |
| `committer-date` | The epoch timestamp of the fetched commit |

## Building

To build the `git-init` image:

```bash
cd image/git-init
ko build --local .
```
