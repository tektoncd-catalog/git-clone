# Contributing

Thanks for your interest in contributing to `tektoncd-catalog/git-clone`! This
repository is part of the Tekton Catalog and follows the broader
[tektoncd-catalog contributing guide](https://github.com/tektoncd-catalog/.github/blob/main/CONTRIBUTING.md).

For technical details on how the repo is structured and generated, see
[DEVELOPMENT.md](DEVELOPMENT.md).

## Developer Certificate of Origin (DCO) / CLA

All commits must be signed off to certify the
[Developer Certificate of Origin](https://developercertificate.org/). Add a
`Signed-off-by` trailer to every commit:

```bash
git commit --signoff -m "fix: handle empty ssh-directory path"
```

The sign-off line must match the author's name and email. Contributions are
also covered by the Linux Foundation
[EasyCLA](https://github.com/tektoncd/community/blob/main/process.md#contributor-license-agreements)
check, which runs on pull requests — follow its prompt to sign the CLA the
first time you contribute.

## Pull request workflow

1. **Fork and branch** from `main`.
2. **Edit the Task** (`task/git-clone/git-clone.yaml`) and/or the `git-init`
   image (`image/git-init/`) — never edit the generated
   `stepaction/git-clone/git-clone.yaml` directly.
3. **Regenerate** the StepAction and commit both files:
   ```bash
   ./hack/generate-stepaction.sh
   git add task/ stepaction/
   ```
4. **Test locally** (see [DEVELOPMENT.md](DEVELOPMENT.md#running-tests-locally)).
5. **Use conventional commit messages** (`feat:`, `fix:`, `docs:`, `chore:`,
   `ci:`) — the release changelog is derived from these prefixes.
6. **Open a PR** with a clear description.

Approvals are managed via `OWNERS` (Prow-based auto-merge).

## CI expectations

Every PR runs `.github/workflows/build.yaml`, which must pass:

- **Build & Test** — `go build ./...`, `go vet ./...`, and `go test ./...` in
  `image/git-init`.
- **Verify StepAction is in sync** — runs `./hack/generate-stepaction.sh` and
  fails if the committed `stepaction/` file differs from a freshly generated
  one. If this fails, run `./hack/generate-stepaction.sh` and commit the result.
- **E2E matrix** — installs the Task in a kind cluster and runs the TaskRuns in
  `tests/run.yaml` across the supported Tekton Pipelines versions, plus the
  bundle e2e test.

> [!TIP]
> Before pushing, run `./hack/generate-stepaction.sh` and make sure
> `git status` is clean (apart from your intended changes). A stale StepAction
> is the most common CI failure.

## Code of conduct

This project follows the Tekton
[Code of Conduct](https://github.com/tektoncd/community/blob/main/code-of-conduct.md).
