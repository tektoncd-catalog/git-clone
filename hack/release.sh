#!/usr/bin/env bash

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

# Release script for tektoncd-catalog/git-clone.
#
# Usage:
#   ./hack/release.sh v1.5.0              # bump, commit, tag, push
#   ./hack/release.sh v1.5.0 --dry-run    # show what would change
#   ./hack/release.sh v1.5.0 --llm        # generate changelog with gh copilot
#
# Note: Trusted Resources signing (tkn task sign) is intentionally NOT done here.
# It is blocked upstream by:
#   - https://github.com/tektoncd/cli/issues/2894 (injects empty resources: {})
#   - https://github.com/tektoncd/cli/issues/2895 (reserializes/reorders whole file)
# The released bundle and ko image are still cosign-signed in release.yaml.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# --- Parse arguments ---
VERSION=""
DRY_RUN=false
USE_LLM=false

for arg in "$@"; do
    case "${arg}" in
        --dry-run) DRY_RUN=true ;;
        --llm) USE_LLM=true ;;
        v*) VERSION="${arg}" ;;
        *) echo "Unknown argument: ${arg}"; exit 1 ;;
    esac
done

if [[ -z "${VERSION}" ]]; then
    echo "Usage: $0 <version> [--dry-run] [--llm]"
    echo "  Example: $0 v1.5.0"
    exit 1
fi

# Validate version format
if ! echo "${VERSION}" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
    echo "Error: version must match vX.Y.Z (got: ${VERSION})"
    exit 1
fi

# Strip the 'v' prefix for places that need bare version
BARE_VERSION="${VERSION#v}"

cd "${ROOT_DIR}"

# --- Task files to sign (tkn task sign only supports Task, not StepAction) ---
TASK_FILES=(
    "task/git-clone/git-clone.yaml"
)

# --- Detect current version ---
CURRENT_VERSION=$(grep 'app.kubernetes.io/version' task/git-clone/git-clone.yaml | head -1 | sed 's/.*version: *"\?\([0-9][0-9.]*\)"\?/\1/')
CURRENT_TAG="v${CURRENT_VERSION}"

echo "=== Release ${VERSION} ==="
echo "  Current: ${CURRENT_TAG}"
echo "  Target:  ${VERSION}"
echo ""

# --- Ensure we're on main and up to date (skip for dry-run) ---
BRANCH=$(git branch --show-current)
if [[ "${DRY_RUN}" != true ]]; then
    if [[ "${BRANCH}" != "main" ]]; then
        echo "Error: must be on main branch (currently on: ${BRANCH})"
        exit 1
    fi

    git fetch origin main
    LOCAL=$(git rev-parse HEAD)
    REMOTE=$(git rev-parse origin/main)
    if [[ "${LOCAL}" != "${REMOTE}" ]]; then
        echo "Error: local main is not up to date with origin/main"
        echo "  Local:  ${LOCAL}"
        echo "  Remote: ${REMOTE}"
        echo "  Run: git pull origin main"
        exit 1
    fi
else
    git fetch origin main 2>/dev/null || true
fi

# --- Generate changelog ---
echo "--- Commits since ${CURRENT_TAG}:"
COMMITS=$(git log --oneline "${CURRENT_TAG}..HEAD" --no-merges 2>/dev/null || git log --oneline -20)
echo "${COMMITS}"
echo ""

if [[ "${USE_LLM}" == true ]]; then
    echo "--- Generating changelog with gh copilot..."

    PROMPT="Generate a release changelog for version ${VERSION} of the tektoncd-catalog/git-clone project (a Tekton Task for cloning git repositories).

Here are the commits since the last release (${CURRENT_TAG}):

${COMMITS}

Output two sections:

1. An artifacthub.io/changes YAML annotation block (indented with 6 spaces) with entries like:
      - kind: added|changed|fixed|removed
        description: Short one-line description

2. A git tag annotation message with this format:
Release ${VERSION}

<category>:
- <description>

Use categories like Features, Fixes, Improvements, CI/Infra, Docs. Be concise.

Output the two sections separated by the exact string ---SEPARATOR--- on its own line."

    LLM_OUTPUT=$(gh copilot -p "${PROMPT}" 2>/dev/null || echo "")

    if [[ -n "${LLM_OUTPUT}" ]]; then
        AH_CHANGES=$(echo "${LLM_OUTPUT}" | sed -n '/^ *- kind:/,/---SEPARATOR---/p' | grep -v '^---SEPARATOR---')
        TAG_MESSAGE=$(echo "${LLM_OUTPUT}" | sed -n '/^---SEPARATOR---$/,$ p' | tail -n +2 | sed '/^$/d; /^[[:space:]]*$/d' | sed '1{/^$/d}')
    else
        echo "Warning: gh copilot not available, falling back to git log"
        USE_LLM=false
    fi
fi

# Sanitize description for Artifact Hub YAML (strip {}[]&*#?|-<>=!%@ or quote)
# Sanitize and always quote description for Artifact Hub YAML-in-YAML
# AH parses the changes |block as YAML, so descriptions must be quoted
sanitize_desc() {
    local d
    d=$(echo "$1" | sed 's/[{}]//g; s/[][&*#?|<>=!%@`]//g' | sed 's/  */ /g' | sed 's/^ //;s/ $//')
    echo "\"${d}\""
}

if [[ "${USE_LLM}" != true ]]; then
    # Generate simple changelog from commit types
    AH_CHANGES=""
    while IFS= read -r line; do
        msg="${line#* }"  # strip commit hash
        desc=""
        case "${msg}" in
            feat:*|feat\(*) desc=$(sanitize_desc "${msg#*: }"); AH_CHANGES="${AH_CHANGES}      - kind: added\n        description: ${desc}\n" ;;
            fix:*|fix\(*)   desc=$(sanitize_desc "${msg#*: }"); AH_CHANGES="${AH_CHANGES}      - kind: fixed\n        description: ${desc}\n" ;;
            chore:*|ci:*|build*) desc=$(sanitize_desc "${msg#*: }"); AH_CHANGES="${AH_CHANGES}      - kind: changed\n        description: ${desc}\n" ;;
            docs:*)         desc=$(sanitize_desc "${msg#*: }"); AH_CHANGES="${AH_CHANGES}      - kind: changed\n        description: ${desc}\n" ;;
            *)              desc=$(sanitize_desc "${msg}"); AH_CHANGES="${AH_CHANGES}      - kind: changed\n        description: ${desc}\n" ;;
        esac
    done <<< "${COMMITS}"

    TAG_MESSAGE="Release ${VERSION}

Changes since ${CURRENT_TAG}:
$(echo "${COMMITS}" | sed 's/^[a-f0-9]* /- /')"
fi

echo ""
echo "--- Artifact Hub changelog:"
echo -e "${AH_CHANGES}"
echo ""
echo "--- Tag message:"
echo "${TAG_MESSAGE}"
echo ""

# --- All files to update ---
ALL_FILES=("${TASK_FILES[@]}" "stepaction/git-clone/git-clone.yaml" "README.md")

echo "--- Version bumps:"
for f in "${ALL_FILES[@]}"; do
    echo "  ${f}"
done
echo ""

# --- Helper: apply version bumps to a file (stdout) ---
apply_version_bumps() {
    local f="$1"
    sed \
        -e "s|app.kubernetes.io/version: *\"\?${CURRENT_VERSION}\"\?|app.kubernetes.io/version: \"${BARE_VERSION}\"|g" \
        -e "s|ghcr.io/tektoncd-catalog/git-clone:${CURRENT_TAG}|ghcr.io/tektoncd-catalog/git-clone:${VERSION}|g" \
        -e "s|ghcr.io/tektoncd-catalog/git-clone/bundle:${CURRENT_TAG}|ghcr.io/tektoncd-catalog/git-clone/bundle:${VERSION}|g" \
        "${f}"
}

# --- Helper: update artifacthub changelog in content (stdin → stdout) ---
apply_ah_changes() {
    local tmpchanges
    tmpchanges=$(mktemp)
    echo -e "${AH_CHANGES}" > "${tmpchanges}"
    python3 "${SCRIPT_DIR}/apply-ah-changes.py" "${tmpchanges}"
    rm -f "${tmpchanges}"
}

if [[ "${DRY_RUN}" == true ]]; then
    echo "--- Dry run: showing changes ---"

    for f in "${ALL_FILES[@]}"; do
        echo ""
        echo "=== ${f} ==="
        if [[ "${f}" == *".yaml" ]]; then
            apply_version_bumps "${f}" | apply_ah_changes | diff -u "${f}" - || true
        else
            apply_version_bumps "${f}" | diff -u "${f}" - || true
        fi
    done
    echo ""
    echo "Dry run complete. Run without --dry-run to apply."
    exit 0
fi

# --- Apply version bumps ---
echo "--- Applying version bumps..."

for f in "${ALL_FILES[@]}"; do
    if [[ "${f}" == *".yaml" ]]; then
        apply_version_bumps "${f}" | apply_ah_changes > "${f}.tmp" && mv "${f}.tmp" "${f}"
    else
        apply_version_bumps "${f}" > "${f}.tmp" && mv "${f}.tmp" "${f}"
    fi
done

echo "--- Regenerating StepAction from clean YAML..."
./hack/generate-stepaction.sh

# Note: Trusted Resources signing (tkn task sign) intentionally omitted.
# Blocked upstream by tektoncd/cli#2894 and tektoncd/cli#2895.
# Re-introduce a valid, minimally-invasive committed signature once fixed.

echo "--- Committing..."
git add "${ALL_FILES[@]}"
git commit --signoff --message "chore: bump version to ${VERSION}"

echo "--- Pushing to main..."
git push origin main:main

echo "--- Tagging ${VERSION}..."
git tag -a "${VERSION}" -m "${TAG_MESSAGE}"

echo "--- Pushing tag..."
git push origin "refs/tags/${VERSION}:refs/tags/${VERSION}"

echo ""
echo "=== Release ${VERSION} initiated ==="
echo "  Monitor: gh run list --workflow=release.yaml --limit 1"
echo "  View:    https://github.com/tektoncd-catalog/git-clone/releases/tag/${VERSION}"
