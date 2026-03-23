#!/usr/bin/env bash

source "$(dirname "${BASH_SOURCE}")/lib/init.sh"

SCRIPT_ROOT=$(dirname ${BASH_SOURCE})/..
${SCRIPT_ROOT}/hack/update-generated-openapi.sh

PKG_DIR=${SCRIPT_ROOT}/pkg

if ! git diff --exit-code --quiet ${PKG_DIR}; then
  echo "openapi-gen is out of date. Please run hack/update-generated-openapi.sh"
  exit 1
fi

echo "Verifying OpenAPI schema mappings"
cd ${SCRIPT_ROOT}
if ! go run ./cmd/ensure-openapi-mappings --openapi-file pkg/openapi/zz_generated.openapi.go; then
  echo "OpenAPI schema mapping validation failed"
  exit 1
fi

echo "openapi-gen up to date."
