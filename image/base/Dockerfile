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

# Base image for git-clone task.
# Includes git, git-lfs, openssh-client, and a nonroot user (uid 65532)
# matching Tekton's default securityContext.
FROM docker.io/alpine/git:v2.52.0@sha256:8786a6a02273827d0aa039d174aacd5e017fcce9aba0af62596d991970cab01a

RUN apk add --no-cache git-lfs openssh-client && \
    adduser -D -u 65532 -G root tekton && \
    mkdir -p /home/tekton && chown tekton:root /home/tekton

USER tekton
