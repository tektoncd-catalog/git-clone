# AGENTS.md

Guidance for AI coding agents working in `tektoncd-catalog/git-clone`. For full
detail see [DEVELOPMENT.md](DEVELOPMENT.md).

## Repository structure

| Path | Role |
|------|------|
| `task/git-clone/git-clone.yaml` | **Edit this.** The `git-clone` Task — the single source of truth. |
| `stepaction/git-clone/git-clone.yaml` | **Generated — never edit by hand.** Derived from the Task. |
| `image/git-init/` | Go source for the `git-init` binary the Task runs (built with `ko`). |
| `image/base/Dockerfile` | Base image the `git-init` image is built `FROM`. |
| `hack/generate-stepaction.sh` | Wrapper around the Python generator. |
| `hack/generate-stepaction.py` | Derives the StepAction from the Task (workspaces → params). |
| `hack/release.sh` | Release automation. |
| `hack/apply-ah-changes.py` | Injects the `artifacthub.io/changes` annotation at release. |
| `test/` | e2e runners (`e2e-tests.sh`, `e2e-bundle-test.sh`). |
| `keys/` | `cosign.pub`, the public key for verifying signed release bundles. |
| `.github/workflows/` | `build.yaml` (test/verify/e2e), `release.yaml` (bundle publish), `base-image.yaml`. |

## Critical Rules

1. **Never edit `stepaction/git-clone/git-clone.yaml` directly.** It is
   generated from the Task. Edit `task/git-clone/git-clone.yaml`, then run
   `./hack/generate-stepaction.sh`. CI's *Verify StepAction is in sync* step
   diffs the committed file against a freshly generated one and fails on
   mismatch.
2. **No `$(params.*)` in `script:` blocks.** For StepActions `$(params.*)` in
   scripts is not supported at all; in both Task and StepAction it's an
   injection risk. Pass values via `env:` and reference the shell env var
   (e.g. `${PARAM_OUTPUT_PATH}`, `${WORKSPACE_OUTPUT_PATH}`).
3. **Workspaces map to params in the StepAction.** `output` → `output-path`,
   `ssh-directory` → `ssh-directory-path`, `basic-auth` → `basic-auth-path`,
   `ssl-ca-directory` → `ssl-ca-directory-path`. The `WORKSPACE_*_PATH` env
   vars become `PARAM_*`; the `WORKSPACE_*_BOUND` booleans are dropped. If you
   change a workspace, update `WORKSPACE_PARAMS` / `WORKSPACE_ENV_MAP` and the
   `transform_*` functions in `hack/generate-stepaction.py`.
4. **Sign off every commit** (DCO / EasyCLA): `git commit --signoff`.
5. **Use conventional commit prefixes** (`feat:`, `fix:`, `docs:`, `chore:`,
   `ci:`) — the release changelog is derived from them.
6. **Do not add a `tekton.dev/signature` to the StepAction** — the generator
   drops it. Trusted Resources signing is intentionally not done in-repo
   (blocked by tektoncd/cli#2894 and tektoncd/cli#2895); the bundle and image
   are cosign-signed in the release workflow.

## Common commands

```bash
./hack/generate-stepaction.sh             # regenerate the StepAction from the Task
./hack/release.sh v1.8.0 --dry-run         # preview a release (no changes applied)
./hack/release.sh v1.8.0 --dry-run --llm   # preview with gh copilot changelog
./test/e2e-tests.sh                       # e2e in a kind cluster (needs a cluster)
./test/e2e-bundle-test.sh                 # bundle-resolver e2e
(cd image/git-init && go test ./...)      # unit tests for the git-init binary
```

Generation needs `python3` with PyYAML (falls back to `uv tool run --with
pyyaml`).

## Validating changes locally

1. After editing the Task, run `./hack/generate-stepaction.sh`.
2. Confirm `git status` shows only intended changes (clean verify step).
3. Run `go build/vet/test` under `image/git-init` if you touched the binary.
4. Run the relevant e2e script against a kind cluster.
5. Update `README.md` if you changed installation or usage.

## Common pitfalls

- Forgetting to run `./hack/generate-stepaction.sh` after editing the Task →
  CI *Verify StepAction is in sync* fails.
- Editing `stepaction/git-clone/git-clone.yaml` by hand → overwritten on the
  next regeneration and rejected by CI.
- Putting `$(params.*)` in a `script:` block → use env vars instead.
- Adding a new workspace without updating the generator's mapping tables →
  the StepAction won't expose the corresponding param.
