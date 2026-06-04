#!/usr/bin/env python3

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

"""Generate a StepAction YAML from a Task YAML.

Transforms:
- kind: Task → kind: StepAction
- workspaces → params with path suffix
- workspace bound checks → path != "" checks
- $(results.*) → $(step.results.*)
- Unwraps steps[0] to top-level image/env/script
- Strips workspace-related env vars, adds path params instead

No external dependencies — uses only Python stdlib.
"""

import re
import sys
import textwrap


def generate(task_file: str, output_file: str) -> None:
    with open(task_file) as f:
        task_content = f.read()

    # --- Extract key sections ---

    # Get metadata annotations (reuse as-is, update kind-specific ones)
    # Get params (reuse, add workspace path params)
    # Get the step's script, env, image, securityContext

    # Parse task YAML minimally — we work with the raw text to preserve formatting
    # but use regex for the transformations

    with open(task_file) as f:
        lines = f.readlines()

    # --- Build the StepAction ---
    output_lines = []
    output_lines.append("# This file is generated from task/git-clone/git-clone.yaml\n")
    output_lines.append("# Do not edit directly — run: ./hack/generate-stepaction.sh\n")

    # Find and extract sections from the task
    in_section = None
    step_env = []
    step_script = []
    params = []
    results = []
    annotations = []
    labels = []
    description_lines = []
    image_line = ""
    security_context = []

    i = 0
    while i < len(lines):
        line = lines[i]
        stripped = line.rstrip()

        # Track what section we're in
        if re.match(r'^spec:$', stripped):
            in_section = 'spec'
        elif in_section == 'spec' and re.match(r'^  description:', stripped):
            in_section = 'description'
            description_lines.append(line)
        elif in_section == 'description' and re.match(r'^  [a-z]', stripped) and not stripped.startswith('    '):
            in_section = 'spec'
        elif in_section == 'description':
            description_lines.append(line)
        elif re.match(r'^  workspaces:', stripped):
            in_section = 'workspaces'
        elif re.match(r'^  params:', stripped):
            in_section = 'params'
        elif re.match(r'^  results:', stripped):
            in_section = 'results'
        elif re.match(r'^  steps:', stripped):
            in_section = 'steps'

        # Skip workspaces entirely
        if in_section == 'workspaces':
            if i + 1 < len(lines) and re.match(r'^  [a-z]', lines[i + 1].rstrip()) and not lines[i + 1].startswith('    '):
                in_section = 'spec'
            i += 1
            continue

        i += 1

    # --- Use a different approach: transform the task content directly ---
    # This is more robust than line-by-line parsing

    # Start with the header
    output = []
    output.append("# This file is generated from task/git-clone/git-clone.yaml")
    output.append("# Do not edit directly — run: ./hack/generate-stepaction.sh")

    # Extract version from task
    version_match = re.search(r'app\.kubernetes\.io/version: "([^"]+)"', task_content)
    version = version_match.group(1) if version_match else "0.1"

    # Extract gitInitImage default
    image_match = re.search(r'name: gitInitImage.*?default: "([^"]+)"', task_content, re.DOTALL)
    git_init_image = image_match.group(1) if image_match else "ghcr.io/tektoncd-catalog/git-clone:latest"

    # Extract the script content
    script_match = re.search(r'      script: \|\n([\s\S]*?)(?=\n  [a-z]|\Z)', task_content)
    script_content = ""
    if script_match:
        script_content = script_match.group(1)

    # Transform the script:
    # 1. Replace workspace bound checks with path checks
    # 2. Replace results paths
    # 3. Replace workspace path references
    script_content = script_content.replace(
        '${WORKSPACE_BASIC_AUTH_DIRECTORY_BOUND}" = "true"',
        '${PARAM_BASIC_AUTH_DIRECTORY_PATH}" != ""'
    )
    script_content = script_content.replace(
        '${WORKSPACE_SSH_DIRECTORY_BOUND}" = "true"',
        '${PARAM_SSH_DIRECTORY_PATH}" != ""'
    )
    script_content = script_content.replace(
        '${WORKSPACE_SSL_CA_DIRECTORY_BOUND}" = "true"',
        '${PARAM_SSL_CA_DIRECTORY_PATH}" != ""'
    )
    script_content = script_content.replace('${WORKSPACE_OUTPUT_PATH}', '${PARAM_OUTPUT_PATH}')
    script_content = script_content.replace('${WORKSPACE_BASIC_AUTH_DIRECTORY_PATH}', '${PARAM_BASIC_AUTH_DIRECTORY_PATH}')
    script_content = script_content.replace('${WORKSPACE_SSH_DIRECTORY_PATH}', '${PARAM_SSH_DIRECTORY_PATH}')
    script_content = script_content.replace('${WORKSPACE_SSL_CA_DIRECTORY_PATH}', '${PARAM_SSL_CA_DIRECTORY_PATH}')
    script_content = script_content.replace('$(results.', '$(step.results.')

    # Remove the WORKSPACE_*_BOUND env var lines from script
    # (they're replaced by path != "" checks)

    # Extract annotations block
    annotations_match = re.search(r'  annotations:\n([\s\S]*?)(?=\nspec:)', task_content)
    annotations_block = annotations_match.group(1) if annotations_match else ""

    # Extract description
    desc_match = re.search(r'  description: >-\n([\s\S]*?)(?=\n  workspaces:|\n  params:)', task_content)
    desc_text = ""
    if desc_match:
        desc_text = desc_match.group(1).rstrip()

    # Extract params (skip gitInitImage and userHome — handled differently)
    params_match = re.search(r'  params:\n([\s\S]*?)(?=\n  results:)', task_content)
    params_text = params_match.group(1) if params_match else ""

    # Extract results
    results_match = re.search(r'  results:\n([\s\S]*?)(?=\n  steps:)', task_content)
    results_text = results_match.group(1) if results_match else ""

    # --- Build the output ---
    output.append("apiVersion: tekton.dev/v1beta1")
    output.append("kind: StepAction")
    output.append("metadata:")
    output.append("  name: git-clone")
    output.append("  labels:")
    output.append(f'    app.kubernetes.io/version: "{version}"')
    output.append("  annotations:")

    # Copy annotations, skip artifacthub changes (will differ)
    for aline in annotations_block.split('\n'):
        if aline.strip():
            output.append(aline)

    output.append("spec:")
    output.append("  description: >-")
    for dline in desc_text.split('\n'):
        if dline.strip():
            output.append(dline
                .replace('These Tasks are Git tasks to work with repositories used by other tasks', 'This StepAction clones a git repository for use by other steps')
                .replace('The git-clone Task will clone a repo from the provided url into the', 'It clones a repo from the provided url into the')
                .replace("output Workspace. By default the repo will be cloned into the root of", "output path. By default the repo will be cloned into the root of")
                .replace("your Workspace. You can clone into a subdirectory by setting this Task's", "the provided path. You can clone into a subdirectory by setting this StepAction's")
                .replace('This Task also supports', 'It also supports')
                .replace("this Task's", "this StepAction's")
                .replace('this Task', 'this StepAction')
            )

    output.append("  params:")
    # Add workspace-replacement params first
    output.append("    - name: output-path")
    output.append("      description: The git repo will be cloned onto this path.")
    output.append("      type: string")
    output.append("    - name: ssh-directory-path")
    output.append("      description: |")
    output.append("        A .ssh directory with private key, known_hosts, config, etc.")
    output.append('      default: ""')
    output.append("    - name: basic-auth-path")
    output.append("      description: |")
    output.append("        A directory path containing a .gitconfig and .git-credentials file.")
    output.append('      default: ""')
    output.append("    - name: ssl-ca-directory-path")
    output.append("      description: |")
    output.append("        A directory containing CA certificates for HTTPS verification.")
    output.append('      default: ""')

    # Copy existing params from task
    for pline in params_text.split('\n'):
        if pline.strip():
            # Update description references
            output.append(pline.replace('this Task', 'this StepAction'))

    output.append("  results:")
    for rline in results_text.split('\n'):
        if rline.strip():
            # Update description to say StepAction instead of Task
            output.append(rline.replace('this Task', 'this StepAction'))

    # Image
    output.append('  image: "$(params.gitInitImage)"')

    # Env vars
    output.append("  env:")
    output.append('  - name: HOME')
    output.append('    value: "$(params.userHome)"')
    output.append('  - name: PARAM_URL')
    output.append('    value: $(params.url)')
    output.append('  - name: PARAM_REVISION')
    output.append('    value: $(params.revision)')
    output.append('  - name: PARAM_REFSPEC')
    output.append('    value: $(params.refspec)')
    output.append('  - name: PARAM_SUBMODULES')
    output.append('    value: $(params.submodules)')
    output.append('  - name: PARAM_SUBMODULE_PATHS')
    output.append('    value: $(params.submodulePaths)')
    output.append('  - name: PARAM_DEPTH')
    output.append('    value: $(params.depth)')
    output.append('  - name: PARAM_SSL_VERIFY')
    output.append('    value: $(params.sslVerify)')
    output.append('  - name: PARAM_CRT_FILENAME')
    output.append('    value: $(params.crtFileName)')
    output.append('  - name: PARAM_SUBDIRECTORY')
    output.append('    value: $(params.subdirectory)')
    output.append('  - name: PARAM_DELETE_EXISTING')
    output.append('    value: $(params.deleteExisting)')
    output.append('  - name: PARAM_HTTP_PROXY')
    output.append('    value: $(params.httpProxy)')
    output.append('  - name: PARAM_HTTPS_PROXY')
    output.append('    value: $(params.httpsProxy)')
    output.append('  - name: PARAM_NO_PROXY')
    output.append('    value: $(params.noProxy)')
    output.append('  - name: PARAM_VERBOSE')
    output.append('    value: $(params.verbose)')
    output.append('  - name: PARAM_USER_FRIENDLY_ERRORS')
    output.append('    value: $(params.userFriendlyErrors)')
    output.append('  - name: PARAM_SPARSE_CHECKOUT_DIRECTORIES')
    output.append('    value: $(params.sparseCheckoutDirectories)')
    output.append('  - name: PARAM_USER_HOME')
    output.append('    value: $(params.userHome)')
    output.append('  - name: PARAM_OUTPUT_PATH')
    output.append('    value: $(params.output-path)')
    output.append('  - name: PARAM_SSH_DIRECTORY_PATH')
    output.append('    value: $(params.ssh-directory-path)')
    output.append('  - name: PARAM_BASIC_AUTH_DIRECTORY_PATH')
    output.append('    value: $(params.basic-auth-path)')
    output.append('  - name: PARAM_SSL_CA_DIRECTORY_PATH')
    output.append('    value: $(params.ssl-ca-directory-path)')

    # SecurityContext
    output.append("  securityContext:")
    output.append("    runAsNonRoot: true")
    output.append("    runAsUser: 65532")

    # Script — normalize indentation to 4 spaces
    output.append("  script: |")
    for sline in script_content.split('\n'):
        # Strip the original 8-space indentation from the task, re-indent to 4
        dedented = sline.replace('        ', '    ', 1) if sline.startswith('        ') else sline
        output.append(dedented.rstrip())

    # Write output
    result = '\n'.join(output).rstrip() + '\n'

    with open(output_file, 'w') as f:
        f.write(result)


if __name__ == '__main__':
    if len(sys.argv) != 3:
        print(f"Usage: {sys.argv[0]} <task.yaml> <stepaction.yaml>")
        sys.exit(1)
    generate(sys.argv[1], sys.argv[2])
