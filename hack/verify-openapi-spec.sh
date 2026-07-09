#!/usr/bin/env bash

# Copyright 2026 The OpenShift Authors.
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

# This script checks whether updating of OpenAPI specification is needed or not.
# We should run `hack/update-openapi-spec.sh` if OpenAPI specification is out of
# date.
# Usage: `hack/verify-openapi-spec.sh`.

set -o errexit
set -o nounset
set -o pipefail

KUBE_ROOT=$(dirname "${BASH_SOURCE[0]}")/..

"${KUBE_ROOT}/hack/update-openapi-spec.sh"

diffs=$(git status --porcelain api/openapi-spec api/discovery 2>/dev/null | wc -l)
if [[ ${diffs} -gt 0 ]]; then
  echo "Generated files are out of date" >&2
  echo "" >&2
  git status --porcelain api/openapi-spec api/discovery >&2
  echo "" >&2
  git diff api/openapi-spec api/discovery >&2
  echo "" >&2
  echo "Please run 'hack/update-openapi-spec.sh'" >&2
  exit 1
fi

echo "api/openapi-spec up to date."
echo "api/discovery up to date."

# ex: ts=2 sw=2 et filetype=sh
