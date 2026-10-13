#!/usr/bin/env bash

source "$(dirname "${BASH_SOURCE}")/lib/init.sh"

SCRIPT_ROOT=$(dirname ${BASH_SOURCE})/..
${SCRIPT_ROOT}/hack/update-generated-deep-copies.sh

PKG_DIR=${SCRIPT_ROOT}/pkg

if ! git diff --exit-code --quiet ${PKG_DIR}; then
  echo "deepcopy-gen is out of date. Please run hack/update-generated-deep-copies.sh"
  exit 1
fi
echo "deepcopy-gen up to date."
