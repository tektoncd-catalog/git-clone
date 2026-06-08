# Development

This document explains how the `tektoncd-catalog/git-clone` repository is
structured and how to develop, generate, test, and release its Task and
StepAction.

> [!IMPORTANT]
> The `task/` directory is the **source of truth**. The `stepaction/` directory
> is **generated** from it. Never edit `stepaction/git-clone/git-clone.yaml`
> directly — edit the Task and run `./hack/generate-stepaction.sh`.

## Architecture overview

The repository ships a `git-clone` [Task](task/git-clone/) and a derived
[StepAction](stepaction/git-clone/) for Tekton Pipelines. Both run the
`git-init` binary built from `image/git-init/`.

```
image/git-init/  ──► ko build ──► git-init image (gitInitImage param)
                                        │
task/git-clone/git-clone.yaml  ─────────┤  (source of truth)
        │                               │
        └─► hack/generate-stepaction.py ┴─► stepaction/git-clone/git-clone.yaml
                                            (generated — do not edit)
```

Key files:

| Path | Role |
|------|------|
| `task/git-clone/git-clone.yaml` | **Hand-edited.** The `git-clone` Task — the single source of truth. |
| `stepaction/git-clone/git-clone.yaml` | **Generated** from the Task. Do not edit. |
| `image/git-init/` | Go source for the `git-init` binary the Task runs (built with `ko`). |
| `image/base/Dockerfile` | Base image (git + tooling) the `git-init` image is built `FROM`. |
| `hack/generate-stepaction.sh` | Wrapper that runs the Python generator. |
| `hack/generate-stepaction.py` | Derives the StepAction from the Task (workspaces → params). |
| `hack/release.sh` | Release automation: bump version → regenerate → changelog → commit → tag → push. |
| `hack/apply-ah-changes.py` | Injects the `artifacthub.io/changes` annotation during release. |
| `test/` | e2e runners (`e2e-tests.sh`, `e2e-bundle-test.sh`). |
| `keys/` | `cosign.pub` (bundle verification) and signing material. |
| `.github/workflows/` | `build.yaml` (test/verify/e2e), `release.yaml` (bundle publish), `base-image.yaml` (multi-arch base image). |

### Why generate the StepAction?

- **Deterministic:** CI regenerates the StepAction and diffs it against what's
  committed (the *Verify StepAction is in sync* step in `build.yaml`). The
  committed file must match exactly.
- **DRY:** The StepAction is a mechanical transform of the Task, so behaviour
  stays in lockstep instead of being maintained by hand in two places.

## How generation works

Run:

```bash
./hack/generate-stepaction.sh
```

Requirements: `python3` with **PyYAML**. If PyYAML isn't importable directly,
the wrapper falls back to `uv tool run --with pyyaml`.

`generate-stepaction.py` parses the Task, takes its single step, and produces a
clean StepAction:

- **Workspaces become params.** The `output`, `ssh-directory`, `basic-auth`,
  and `ssl-ca-directory` workspaces map to `output-path`,
  `ssh-directory-path`, `basic-auth-path`, and `ssl-ca-directory-path` params.
- **Env vars are rewritten.** `WORKSPACE_*_PATH` env vars become `PARAM_*`
  env vars sourced from `$(params.*-path)`; the `WORKSPACE_*_BOUND` booleans
  are dropped (the script tests `!= ""` on the path instead).
- **Script references are rewritten.** `${WORKSPACE_OUTPUT_PATH}` →
  `${PARAM_OUTPUT_PATH}` etc., and `$(results.*)` → `$(step.results.*)`.
  Scripts use shell env vars (never `$(params.*)`) because `$(params.*)`
  substitution is **not allowed in StepAction scripts**.
- **Descriptions are reworded** ("This Task" → "This StepAction").
- **The `tekton.dev/signature` annotation is dropped.**
- The task params (`url`, `revision`, `gitInitImage`, …) and results
  (`commit`, `url`, …) are carried over.

The generated file starts with a `# This file is generated …` header.

## Modifying the Task or StepAction

To change cloning behaviour, parameters, or results:

1. Edit `task/git-clone/git-clone.yaml`.
2. Regenerate the StepAction:
   ```bash
   ./hack/generate-stepaction.sh
   ```
3. Review both files and commit them together.

If you add or rename a workspace/param that affects the StepAction mapping,
update the `WORKSPACE_PARAMS` / `WORKSPACE_ENV_MAP` tables and the
`transform_*` functions in `hack/generate-stepaction.py`, then regenerate.

## Building the image

The Task runs the `git-init` binary, shipped as the `gitInitImage`. Build it
locally with [`ko`](https://ko.build/):

```bash
cd image/git-init
ko build --local .
```

Point the Task at a local build for testing via the `GIT_INIT_IMAGE`
environment variable understood by the e2e scripts (see below). The base image
(`image/base/Dockerfile`) is built and pushed multi-arch by
`.github/workflows/base-image.yaml`.

## Running tests locally

The Go unit tests live under `image/git-init`:

```bash
cd image/git-init
go build ./...
go vet ./...
go test ./...
```

E2e tests run against a real Tekton install in a local
[kind](https://kind.sigs.k8s.io/) cluster:

```bash
kind create cluster

# Task: install it and run every TaskRun in tests/run.yaml
./test/e2e-tests.sh

# Bundle: push the Task as a Tekton bundle and resolve it via the bundle resolver
./test/e2e-bundle-test.sh
```

Useful environment variables:

| Var | Default | Meaning |
|-----|---------|---------|
| `PIPELINE_VERSION` | `v1.12.0` | Tekton Pipelines release to install |
| `TIMEOUT` | `120s` | Per-TaskRun timeout |
| `GIT_INIT_IMAGE` | — | Override `gitInitImage` (e.g. a local `ko build`) |
| `BUNDLE_REGISTRY` | `ttl.sh` | Registry the bundle test pushes to |

CI (`build.yaml`) runs `go build/vet/test`, the *Verify StepAction is in sync*
check, and the e2e matrix across the supported Tekton Pipelines versions.

## Release process

Releases are driven by `hack/release.sh`:

```bash
./hack/release.sh v1.8.0 --dry-run        # preview the diff (no changes applied)
./hack/release.sh v1.8.0 --dry-run --llm  # preview with a gh copilot changelog
./hack/release.sh v1.8.0                   # bump, regenerate, commit, tag, push
```

What it does:

1. Validates the version (`vX.Y.Z`) and that you're on an up-to-date `main`.
2. Bumps the `app.kubernetes.io/version` label and image/bundle tags in the
   Task, StepAction, and `README.md`.
3. Builds a changelog from conventional-commit prefixes (or via `gh copilot`
   with `--llm`) and injects it as an `artifacthub.io/changes` annotation
   (`hack/apply-ah-changes.py`).
4. Regenerates the StepAction from the bumped Task.
5. Commits (`--signoff`), pushes `main`, creates an annotated tag, and pushes
   the tag.

The tag push triggers `.github/workflows/release.yaml`, which publishes a
cosign-signed Tekton bundle and signs the `ko` image.

> [!NOTE]
> Tekton **Trusted Resources** signing (`tkn task sign`) is intentionally not
> done in-repo. It's blocked upstream by
> [tektoncd/cli#2894](https://github.com/tektoncd/cli/issues/2894) and
> [tektoncd/cli#2895](https://github.com/tektoncd/cli/issues/2895). The
> released bundle and image are cosign-signed in the release workflow instead.

## Downstream usage

The Task and StepAction are consumed directly (raw URL, `kubectl apply`) or via
the published cosign-signed Tekton bundle. See [README.md](README.md) for
installation and bundle-resolver examples.

## See also

- [CONTRIBUTING.md](CONTRIBUTING.md) — contribution workflow and CI expectations.
- [AGENTS.md](AGENTS.md) — quick reference for AI coding agents.
