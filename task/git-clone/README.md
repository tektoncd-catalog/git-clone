# `git-clone`

This `Task` clones a git repository into a Workspace. It supports:

- Branch, tag, SHA, and refspec checkout
- Shallow clones
- Submodules (full or selective)
- Sparse checkouts
- SSH, basic auth, and custom CA authentication
- HTTP/HTTPS proxy configuration
- User-friendly error messages with actionable hints

## Requirements

- Tekton Pipelines **v1.0.0** or later
- Runs as **non-root** (UID 65532)

## Workspaces

> **Note**: This task runs as UID 65532. You may need to set `fsGroup: 65532`
> in your `podTemplate.securityContext` to make workspace volumes writable.

| Workspace | Required | Description |
|-----------|----------|-------------|
| `output` | Yes | The git repo will be cloned here |
| `ssh-directory` | No | `.ssh` directory with private key, `known_hosts`, `config`. Bind a `Secret`. |
| `basic-auth` | No | Directory with `.gitconfig` and `.git-credentials` files. Bind a `Secret`. |
| `ssl-ca-directory` | No | Directory with CA certificates for HTTPS verification |

Example with `fsGroup`:

```yaml
apiVersion: tekton.dev/v1
kind: TaskRun
metadata:
  name: clone-repo
spec:
  taskRef:
    name: git-clone
  podTemplate:
    securityContext:
      fsGroup: 65532
  workspaces:
    - name: output
      persistentVolumeClaim:
        claimName: my-pvc
  params:
    - name: url
      value: https://github.com/tektoncd-catalog/git-clone
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
| `userHome` | Absolute path to the user's home directory | `"/tekton/home"` |

## Results

| Result | Description |
|--------|-------------|
| `commit` | The precise commit SHA that was fetched |
| `url` | The precise URL that was fetched |
| `committer-date` | The epoch timestamp of the fetched commit |

## Platforms

`linux/amd64`, `linux/s390x`, `linux/arm64`, `linux/ppc64le`

## Usage

If `revision` is not provided, the default branch of the repository is used.

### Samples

- [Cloning a branch](./samples/git-clone-checking-out-a-branch.yaml)
- [Checking out a specific commit](./samples/git-clone-checking-out-a-commit.yaml)
- [Using the "commit" result](./samples/using-git-clone-result.yaml)
- [Sparse checkout](./samples/git-clone-sparse-checkout.yaml)

## Authentication

### SSH credentials (recommended)

Bind an `ssh-directory` workspace to a `Secret` containing your SSH keys:

```yaml
kind: Secret
apiVersion: v1
metadata:
  name: my-ssh-credentials
data:
  id_rsa: # ... base64-encoded private key ...
  known_hosts: # ... base64-encoded known_hosts file ...
  config: # ... base64-encoded ssh config file ...
```

```yaml
# In a TaskRun:
workspaces:
  - name: ssh-directory
    secret:
      secretName: my-ssh-credentials

# In a Pipeline:
tasks:
  - name: fetch-source
    taskRef:
      name: git-clone
    workspaces:
      - name: ssh-directory
        workspace: ssh-creds
```

Including `known_hosts` is optional but strongly recommended. Without it
the task will blindly accept the remote server's identity.

### Basic auth (username/password/token)

> **Note**: Prefer SSH credentials when available. For basic auth, generate a
> short-lived token from your platform (GitHub, GitLab, Bitbucket, etc.) and
> use `git` as the username.

Bind a `basic-auth` workspace to a `Secret` containing `.gitconfig` and
`.git-credentials`:

```yaml
kind: Secret
apiVersion: v1
metadata:
  name: my-basic-auth-secret
type: Opaque
stringData:
  .gitconfig: |
    [credential "https://<hostname>"]
      helper = store
  .git-credentials: |
    https://<user>:<pass>@<hostname>
```

```yaml
workspaces:
  - name: basic-auth
    secret:
      secretName: my-basic-auth-secret
```

> **Note**: Settings in `.gitconfig` can conflict with task parameters (e.g.
> proxy settings). Use task parameters instead when possible.

### Custom CA certificates

Bind an `ssl-ca-directory` workspace to a `Secret` containing your CA bundle:

```yaml
kind: Secret
apiVersion: v1
metadata:
  name: my-ssl-credentials
data:
  ca-bundle.crt: # ... base64-encoded certificate ...
```

```yaml
workspaces:
  - name: ssl-ca-directory
    secret:
      secretName: my-ssl-credentials
```

If the certificate file is named something other than `ca-bundle.crt`, set the
`crtFileName` parameter accordingly.

### Tekton built-in credentials

You can also use Tekton Pipelines' built-in credential support as documented in
[Pipelines auth.md](https://github.com/tektoncd/pipeline/blob/main/docs/auth.md).
