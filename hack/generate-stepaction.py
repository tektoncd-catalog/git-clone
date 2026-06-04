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

Uses PyYAML to properly parse the Task (even if mangled by tkn task sign)
and produces a clean StepAction.

Requires: pip install pyyaml (or uv tool run --with pyyaml)
"""

import copy
import re
import sys
import textwrap

import yaml


# Custom YAML dumper that preserves multiline strings and quoting
class StepActionDumper(yaml.SafeDumper):
    pass


def str_representer(dumper, data):
    if '\n' in data:
        return dumper.represent_scalar('tag:yaml.org,2002:str', data, style='|')
    # Quote version strings and empty strings
    if data == '' or re.match(r'^[\d.]+$', data):
        return dumper.represent_scalar('tag:yaml.org,2002:str', data, style='"')
    # Quote values that look like booleans
    if data.lower() in ('true', 'false', 'yes', 'no'):
        return dumper.represent_scalar('tag:yaml.org,2002:str', data, style='"')
    return dumper.represent_scalar('tag:yaml.org,2002:str', data)


StepActionDumper.add_representer(str, str_representer)


# Workspace-to-param mapping
WORKSPACE_PARAMS = [
    {
        'name': 'output-path',
        'description': 'The git repo will be cloned onto this path.',
        'type': 'string',
    },
    {
        'name': 'ssh-directory-path',
        'description': 'A .ssh directory with private key, known_hosts, config, etc.\n',
        'default': '',
    },
    {
        'name': 'basic-auth-path',
        'description': 'A directory path containing a .gitconfig and .git-credentials file.\n',
        'default': '',
    },
    {
        'name': 'ssl-ca-directory-path',
        'description': 'A directory containing CA certificates for HTTPS verification.\n',
        'default': '',
    },
]

# Env var mapping: workspace env vars → param env vars
WORKSPACE_ENV_MAP = {
    'WORKSPACE_OUTPUT_PATH': ('PARAM_OUTPUT_PATH', '$(params.output-path)'),
    'WORKSPACE_SSH_DIRECTORY_BOUND': None,  # removed
    'WORKSPACE_SSH_DIRECTORY_PATH': ('PARAM_SSH_DIRECTORY_PATH', '$(params.ssh-directory-path)'),
    'WORKSPACE_BASIC_AUTH_DIRECTORY_BOUND': None,  # removed
    'WORKSPACE_BASIC_AUTH_DIRECTORY_PATH': ('PARAM_BASIC_AUTH_DIRECTORY_PATH', '$(params.basic-auth-path)'),
    'WORKSPACE_SSL_CA_DIRECTORY_BOUND': None,  # removed
    'WORKSPACE_SSL_CA_DIRECTORY_PATH': ('PARAM_SSL_CA_DIRECTORY_PATH', '$(params.ssl-ca-directory-path)'),
}


def transform_script(script: str) -> str:
    """Transform Task script to StepAction script."""
    s = script
    # Replace workspace bound checks with path checks
    s = s.replace('${WORKSPACE_BASIC_AUTH_DIRECTORY_BOUND}" = "true"',
                  '${PARAM_BASIC_AUTH_DIRECTORY_PATH}" != ""')
    s = s.replace('${WORKSPACE_SSH_DIRECTORY_BOUND}" = "true"',
                  '${PARAM_SSH_DIRECTORY_PATH}" != ""')
    s = s.replace('${WORKSPACE_SSL_CA_DIRECTORY_BOUND}" = "true"',
                  '${PARAM_SSL_CA_DIRECTORY_PATH}" != ""')
    # Replace workspace path references
    s = s.replace('${WORKSPACE_OUTPUT_PATH}', '${PARAM_OUTPUT_PATH}')
    s = s.replace('${WORKSPACE_BASIC_AUTH_DIRECTORY_PATH}', '${PARAM_BASIC_AUTH_DIRECTORY_PATH}')
    s = s.replace('${WORKSPACE_SSH_DIRECTORY_PATH}', '${PARAM_SSH_DIRECTORY_PATH}')
    s = s.replace('${WORKSPACE_SSL_CA_DIRECTORY_PATH}', '${PARAM_SSL_CA_DIRECTORY_PATH}')
    # Replace results paths
    s = s.replace('$(results.', '$(step.results.')
    return s


def transform_description(desc: str) -> str:
    """Transform Task description for StepAction."""
    d = desc
    d = re.sub(r'These Tasks are Git tasks to work with repositories used by other tasks',
               'This StepAction clones a git repository for use by other steps', d)
    d = re.sub(r'The git-clone Task will clone a repo from the provided url into the\s+output Workspace\.',
               'It clones a repo from the provided url into the output path.', d)
    d = d.replace("By default the repo will be cloned into the root of\nyour Workspace.",
                  "By default the repo will be cloned into the root of the provided path.")
    d = d.replace("By default the repo will be cloned into the root of your Workspace.",
                  "By default the repo will be cloned into the root of the provided path.")
    d = d.replace("this Task's", "this StepAction's")
    d = d.replace("This Task", "This StepAction")
    d = d.replace("this Task", "this StepAction")
    return d


def generate(task_file: str, output_file: str) -> None:
    with open(task_file) as f:
        task = yaml.safe_load(f)

    step = task['spec']['steps'][0]

    # Build StepAction
    sa = {
        'apiVersion': 'tekton.dev/v1beta1',
        'kind': 'StepAction',
        'metadata': {
            'name': 'git-clone',
            'labels': {
                'app.kubernetes.io/version': task['metadata'].get('labels', {}).get('app.kubernetes.io/version', '0.1'),
            },
            'annotations': {},
        },
        'spec': {},
    }

    # Copy annotations, update Task→StepAction references
    for k, v in task['metadata'].get('annotations', {}).items():
        if k == 'tekton.dev/signature':
            continue  # don't copy signature
        sa['metadata']['annotations'][k] = v

    # Description
    desc = task['spec'].get('description', '')
    sa['spec']['description'] = transform_description(desc)

    # Params: workspace-replacement params + task params
    sa['spec']['params'] = copy.deepcopy(WORKSPACE_PARAMS)
    for p in task['spec']['params']:
        p2 = copy.deepcopy(p)
        if 'description' in p2:
            p2['description'] = p2['description'].replace('this Task', 'this StepAction')
        sa['spec']['params'].append(p2)

    # Results
    sa['spec']['results'] = []
    for r in task['spec'].get('results', []):
        r2 = copy.deepcopy(r)
        r2['description'] = r2.get('description', '').replace('this Task', 'this StepAction')
        sa['spec']['results'].append(r2)

    # Image
    sa['spec']['image'] = step['image']

    # Env: filter out workspace-bound vars, add workspace-path params
    sa['spec']['env'] = []
    for e in step.get('env', []):
        name = e['name']
        if name in WORKSPACE_ENV_MAP:
            mapping = WORKSPACE_ENV_MAP[name]
            if mapping is None:
                continue  # skip bound check vars
            sa['spec']['env'].append({'name': mapping[0], 'value': mapping[1]})
        else:
            sa['spec']['env'].append(copy.deepcopy(e))

    # SecurityContext
    if 'securityContext' in step:
        sa['spec']['securityContext'] = copy.deepcopy(step['securityContext'])

    # Script
    sa['spec']['script'] = transform_script(step.get('script', ''))

    # Write output
    header = "# This file is generated from task/git-clone/git-clone.yaml\n"
    header += "# Do not edit directly — run: ./hack/generate-stepaction.sh\n"

    with open(output_file, 'w') as f:
        f.write(header)
        yaml.dump(sa, f, Dumper=StepActionDumper, default_flow_style=False, allow_unicode=True, sort_keys=False)


if __name__ == '__main__':
    if len(sys.argv) != 3:
        print(f"Usage: {sys.argv[0]} <task.yaml> <stepaction.yaml>")
        sys.exit(1)
    generate(sys.argv[1], sys.argv[2])
