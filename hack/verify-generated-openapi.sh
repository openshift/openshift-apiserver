#!/usr/bin/env bash

source "$(dirname "${BASH_SOURCE}")/lib/init.sh"

SCRIPT_ROOT=$(dirname ${BASH_SOURCE})/..
${SCRIPT_ROOT}/hack/update-generated-openapi.sh

PKG_DIR=${SCRIPT_ROOT}/pkg

ret=0
git diff --exit-code --quiet ${PKG_DIR} || ret=$?
if [[ $ret -ne 0 ]]; then
  echo "openapi-gen is out of date. Please run hack/update-generated-openapi.sh"
  exit 1
fi
echo "openapi-gen up to date."
