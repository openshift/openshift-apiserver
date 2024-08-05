#!/usr/bin/env bash

source "$(dirname "${BASH_SOURCE}")/lib/init.sh"

SCRIPT_ROOT=$(dirname ${BASH_SOURCE})/..
${SCRIPT_ROOT}/hack/update-generated-deep-copies.sh

PKG_DIR=${SCRIPT_ROOT}/pkg

ret=0
git diff --exit-code --quiet ${PKG_DIR} || ret=$?
if [[ $ret -ne 0 ]]; then
  echo "deepcopy-gen is out of date. Please run hack/update-generated-deep-copies.sh"
  exit 1
fi
echo "deepcopy-gen up to date."
