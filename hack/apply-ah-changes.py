#!/usr/bin/env python3
"""Replace artifacthub.io/changes annotation in YAML from stdin, write to stdout.

Usage: cat task.yaml | python3 apply-ah-changes.py changes.txt
"""
import re
import sys

content = sys.stdin.read()
with open(sys.argv[1]) as f:
    changes_raw = f.read()

# Normalize and quote all description values
lines = []
for line in changes_raw.strip().split('\n'):
    if not line.strip():
        continue
    stripped = line.lstrip()
    if stripped.startswith('- kind:'):
        line = '      ' + stripped
    elif stripped.startswith('description:'):
        line = '        ' + stripped

    m = re.match(r'(\s+description:\s*)"?(.*?)"?\s*$', line)
    if m:
        prefix, val = m.group(1), m.group(2)
        lines.append(prefix + '"' + val + '"')
    else:
        lines.append(line)

new_changes = '    artifacthub.io/changes: |\n' + '\n'.join(lines) + '\n'

# Replace everything from "artifacthub.io/changes: |" up to the next
# 4-space-indented annotation key (exactly 4 spaces, then a letter)
content = re.sub(
    r'    artifacthub\.io/changes: \|.*?(?=\n    [a-z])',
    new_changes.rstrip(),
    content,
    count=1,
    flags=re.DOTALL,
)
sys.stdout.write(content)
