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

# Script to fetch latest OpenAPI spec from a running OpenShift API server.
# Puts the updated spec at api/openapi-spec/

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
OPENAPI_ROOT_DIR="${OPENAPI_ROOT_DIR:-${SCRIPT_ROOT}/api/openapi-spec}"
DISCOVERY_ROOT_DIR="${DISCOVERY_ROOT_DIR:-${SCRIPT_ROOT}/api/discovery}"

ETCD_HOST="${ETCD_HOST:-127.0.0.1}"
ETCD_PORT="${ETCD_PORT:-2379}"
KUBE_API_HOST="${KUBE_API_HOST:-127.0.0.1}"
KUBE_API_PORT="${KUBE_API_PORT:-8443}"
OPENSHIFT_API_HOST="${OPENSHIFT_API_HOST:-127.0.0.1}"
OPENSHIFT_API_PORT="${OPENSHIFT_API_PORT:-8444}"

TMP_DIR=$(mktemp -d -t update-openapi-spec.XXXXXX)
BIN_DIR="${TMP_DIR}/bin"
CERT_DIR="${TMP_DIR}/certs"
CONFIG_DIR="${TMP_DIR}/config"
ETCD_DATA_DIR="${TMP_DIR}/etcd-data"

mkdir -p "${BIN_DIR}" "${CERT_DIR}" "${CONFIG_DIR}" "${ETCD_DATA_DIR}"

echo "Working directory: ${TMP_DIR}"

ETCD_PID=""
KUBE_APISERVER_PID=""
OPENSHIFT_APISERVER_PID=""
PRESERVE_TMP_DIR=false

function cleanup() {
  echo "Cleaning up..."

  if [[ -n "${OPENSHIFT_APISERVER_PID}" ]]; then
    echo "Stopping openshift-apiserver (PID: ${OPENSHIFT_APISERVER_PID})"
    kill "${OPENSHIFT_APISERVER_PID}" 2>/dev/null || true
    wait "${OPENSHIFT_APISERVER_PID}" 2>/dev/null || true
  fi

  if [[ -n "${KUBE_APISERVER_PID}" ]]; then
    echo "Stopping kube-apiserver (PID: ${KUBE_APISERVER_PID})"
    kill "${KUBE_APISERVER_PID}" 2>/dev/null || true
    wait "${KUBE_APISERVER_PID}" 2>/dev/null || true
  fi

  if [[ -n "${ETCD_PID}" ]]; then
    echo "Stopping etcd (PID: ${ETCD_PID})"
    kill "${ETCD_PID}" 2>/dev/null || true
    wait "${ETCD_PID}" 2>/dev/null || true
  fi

  if [[ "${PRESERVE_TMP_DIR}" == "true" ]]; then
    echo "Temp directory preserved at: ${TMP_DIR}"
  else
    echo "Removing temporary directory: ${TMP_DIR}"
    rm -rf "${TMP_DIR}"
  fi

  echo "Cleanup complete"
}

function wait_for_url() {
  local url="$1"
  local description="${2:-service}"
  local max_attempts="${3:-60}"
  local attempt=0

  echo "Waiting for ${description} at ${url}..."

  while [[ ${attempt} -lt ${max_attempts} ]]; do
    if curl -kfsS "${url}" >/dev/null 2>&1; then
      echo "${description} is healthy"
      return 0
    fi
    attempt=$((attempt + 1))
    sleep 1
  done

  echo "ERROR: ${description} did not become healthy after ${max_attempts} seconds"
  return 1
}

function wait_for_openapi_aggregation() {
  local url="$1"
  local max_attempts="${2:-60}"
  local attempt=0

  echo "Waiting for OpenAPI aggregation to complete..."

  while [[ ${attempt} -lt ${max_attempts} ]]; do
    if response=$(curl -kfsS "${url}/openapi/v3" 2>/dev/null); then
      if path_count=$(echo "${response}" | jq -r '.paths | length' 2>/dev/null); then
        if [[ "${path_count}" -gt 0 ]]; then
          echo "OpenAPI aggregation complete (${path_count} API groups available)"
          return 0
        fi
      fi
    fi
    attempt=$((attempt + 1))
    sleep 1
  done

  echo "ERROR: OpenAPI aggregation did not complete after ${max_attempts} seconds"
  return 1
}

trap cleanup EXIT SIGINT SIGTERM

if ! command -v jq &>/dev/null; then
  echo "ERROR: jq is required but not installed. Please install jq."
  exit 1
fi

if ! command -v oc &>/dev/null; then
  echo "ERROR: oc is required but not installed."
  exit 1
fi

# Detect OpenShift version from .ci-operator.yaml
OPENSHIFT_VERSION=""
if [[ -f "${SCRIPT_ROOT}/.ci-operator.yaml" ]]; then
  OPENSHIFT_VERSION=$(grep -oP 'openshift-\K[0-9]+\.[0-9]+' "${SCRIPT_ROOT}/.ci-operator.yaml" | head -1)
fi

if [[ -z "${OPENSHIFT_VERSION}" ]]; then
  echo "ERROR: Could not detect OpenShift version from .ci-operator.yaml"
  echo "Please set OPENSHIFT_RELEASE environment variable manually, e.g.:"
  echo "  OPENSHIFT_RELEASE=quay.io/openshift-release-dev/ocp-release:4.22.0-x86_64 $0"
  exit 1
fi

OPENSHIFT_RELEASE="${OPENSHIFT_RELEASE:-quay.io/openshift-release-dev/ocp-release:${OPENSHIFT_VERSION}.0-x86_64}"
echo "Using OpenShift release: ${OPENSHIFT_RELEASE}"

# Set up registry authentication if available
REGISTRY_AUTH_OPTS=""
if [[ -n "${CLUSTER_PROFILE_DIR:-}" && -f "${CLUSTER_PROFILE_DIR}/pull-secret" ]]; then
  echo "Using pull secret from ${CLUSTER_PROFILE_DIR}/pull-secret"
  REGISTRY_AUTH_OPTS="--registry-config=${CLUSTER_PROFILE_DIR}/pull-secret"
elif [[ -n "${REGISTRY_AUTH_FILE:-}" && -f "${REGISTRY_AUTH_FILE}" ]]; then
  echo "Using pull secret from ${REGISTRY_AUTH_FILE}"
  REGISTRY_AUTH_OPTS="--registry-config=${REGISTRY_AUTH_FILE}"
else
  echo "WARNING: No pull secret found. Proceeding without authentication."
  echo "This may fail if accessing private registries."
fi

echo "Extracting kube-apiserver from OpenShift release ${OPENSHIFT_RELEASE}..."
KUBE_APISERVER="${BIN_DIR}/kube-apiserver"
KUBE_EXTRACT_DIR="${TMP_DIR}/kube-extract"
rm -rf "${KUBE_EXTRACT_DIR}"
mkdir -p "${KUBE_EXTRACT_DIR}"
HYPERKUBE_IMAGE=$(oc adm release info "${OPENSHIFT_RELEASE}" ${REGISTRY_AUTH_OPTS} --image-for=hyperkube)
oc image extract "${HYPERKUBE_IMAGE}" ${REGISTRY_AUTH_OPTS} --path usr/bin/kube-apiserver:"${KUBE_EXTRACT_DIR}"
mv "${KUBE_EXTRACT_DIR}/kube-apiserver" "${KUBE_APISERVER}"
chmod +x "${KUBE_APISERVER}"
echo "Extracted kube-apiserver to ${KUBE_APISERVER}"
echo "kube-apiserver version:"
"${KUBE_APISERVER}" --version

echo "Extracting etcd from OpenShift release ${OPENSHIFT_RELEASE}..."
ETCD="${BIN_DIR}/etcd"
ETCD_EXTRACT_DIR="${TMP_DIR}/etcd-extract"
rm -rf "${ETCD_EXTRACT_DIR}"
mkdir -p "${ETCD_EXTRACT_DIR}"
ETCD_IMAGE=$(oc adm release info "${OPENSHIFT_RELEASE}" ${REGISTRY_AUTH_OPTS} --image-for=etcd)
oc image extract "${ETCD_IMAGE}" ${REGISTRY_AUTH_OPTS} --path usr/bin/etcd:"${ETCD_EXTRACT_DIR}"
mv "${ETCD_EXTRACT_DIR}/etcd" "${ETCD}"
chmod +x "${ETCD}"
echo "Extracted etcd to ${ETCD}"
echo "etcd version:"
"${ETCD}" --version

OPENSHIFT_APISERVER="${OPENSHIFT_APISERVER:-${SCRIPT_ROOT}/openshift-apiserver}"
if [[ ! -f "${OPENSHIFT_APISERVER}" ]]; then
  echo "ERROR: openshift-apiserver binary not found at ${OPENSHIFT_APISERVER}"
  echo "Please build it first with: make build"
  exit 1
fi
echo "Using openshift-apiserver: ${OPENSHIFT_APISERVER}"

echo "Generating TLS certificates..."
openssl req -x509 -newkey rsa:2048 -keyout "${CERT_DIR}/ca.key" -out "${CERT_DIR}/ca.crt" \
  -days 365 -nodes -subj "/CN=Test CA" 2>/dev/null
openssl genrsa -out "${CERT_DIR}/tls.key" 2048 2>/dev/null
openssl req -new -key "${CERT_DIR}/tls.key" -out "${CERT_DIR}/tls.csr" \
  -subj "/CN=127.0.0.1" \
  -addext "subjectAltName=IP:127.0.0.1,DNS:localhost" 2>/dev/null
openssl x509 -req -in "${CERT_DIR}/tls.csr" -CA "${CERT_DIR}/ca.crt" -CAkey "${CERT_DIR}/ca.key" \
  -CAcreateserial -out "${CERT_DIR}/tls.crt" -days 365 \
  -copy_extensions copyall 2>/dev/null
echo "Certificates generated in ${CERT_DIR}"

echo "Starting etcd on ${ETCD_HOST}:${ETCD_PORT}..."
"${ETCD}" \
  --data-dir="${ETCD_DATA_DIR}" \
  --listen-client-urls="http://${ETCD_HOST}:${ETCD_PORT}" \
  --advertise-client-urls="http://${ETCD_HOST}:${ETCD_PORT}" \
  >"${TMP_DIR}/etcd.log" 2>&1 &
ETCD_PID=$!
echo "etcd started (PID: ${ETCD_PID})"
sleep 2

echo "Starting kube-apiserver on ${KUBE_API_HOST}:${KUBE_API_PORT}..."
openssl genrsa -out "${CERT_DIR}/sa.key" 2048 2>/dev/null

"${KUBE_APISERVER}" \
  --etcd-servers="http://${ETCD_HOST}:${ETCD_PORT}" \
  --bind-address="${KUBE_API_HOST}" \
  --secure-port="${KUBE_API_PORT}" \
  --cert-dir="${CERT_DIR}" \
  --client-ca-file="${CERT_DIR}/ca.crt" \
  --service-cluster-ip-range="10.0.0.0/24" \
  --service-account-key-file="${CERT_DIR}/sa.key" \
  --service-account-signing-key-file="${CERT_DIR}/sa.key" \
  --service-account-issuer="https://kubernetes.default.svc" \
  --authorization-mode=RBAC \
  --anonymous-auth=true \
  >"${TMP_DIR}/kube-apiserver.log" 2>&1 &
KUBE_APISERVER_PID=$!
echo "kube-apiserver started (PID: ${KUBE_APISERVER_PID})"

wait_for_url "https://${KUBE_API_HOST}:${KUBE_API_PORT}/healthz" "kube-apiserver" 60

kubeconfig_file="${CONFIG_DIR}/kubeconfig"
openssl genrsa -out "${CERT_DIR}/client.key" 2048 2>/dev/null
openssl req -new -key "${CERT_DIR}/client.key" -out "${CERT_DIR}/client.csr" \
  -subj "/CN=openshift-apiserver/O=system:masters" 2>/dev/null
openssl x509 -req -in "${CERT_DIR}/client.csr" -CA "${CERT_DIR}/ca.crt" -CAkey "${CERT_DIR}/ca.key" \
  -CAcreateserial -out "${CERT_DIR}/client.crt" -days 365 2>/dev/null

cat > "${kubeconfig_file}" <<EOF
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://${KUBE_API_HOST}:${KUBE_API_PORT}
    insecure-skip-tls-verify: true
  name: local
contexts:
- context:
    cluster: local
    user: local
  name: local
current-context: local
users:
- name: local
  user:
    client-certificate: ${CERT_DIR}/client.crt
    client-key: ${CERT_DIR}/client.key
EOF

openshift_config_file="${CONFIG_DIR}/openshift-apiserver-config.yaml"
cat > "${openshift_config_file}" <<EOF
apiVersion: openshiftcontrolplane.config.openshift.io/v1
kind: OpenShiftAPIServerConfig
servingInfo:
  bindAddress: ${OPENSHIFT_API_HOST}:${OPENSHIFT_API_PORT}
  bindNetwork: tcp4
  certFile: ${CERT_DIR}/tls.crt
  keyFile: ${CERT_DIR}/tls.key
  clientCA: ${CERT_DIR}/ca.crt
  maxRequestsInFlight: 1200
  requestTimeoutSeconds: 3600
storageConfig:
  urls:
  - http://${ETCD_HOST}:${ETCD_PORT}
  ca: ${CERT_DIR}/ca.crt
  certFile: ${CERT_DIR}/tls.crt
  keyFile: ${CERT_DIR}/tls.key
  storagePrefix: openshift.io
kubeClientConfig:
  kubeConfig: ${kubeconfig_file}
imagePolicyConfig:
  maxImagesBulkImportedPerRepository: 5
projectConfig:
  defaultNodeSelector: ""
routingConfig:
  subdomain: "router.default.svc.cluster.local"
serviceAccountOAuthGrantMethod: prompt
EOF

echo "Starting openshift-apiserver on ${OPENSHIFT_API_HOST}:${OPENSHIFT_API_PORT}..."
"${OPENSHIFT_APISERVER}" start \
  --config="${openshift_config_file}" \
  --authentication-kubeconfig="${kubeconfig_file}" \
  --authentication-skip-lookup \
  --authorization-kubeconfig="${kubeconfig_file}" \
  --authorization-always-allow-paths="/healthz,/healthz/,/readyz,/readyz/,/livez,/livez/,/openapi,/openapi/v2,/openapi/v3,/openapi/v3/*,/apis,/apis/*,/api,/api/*" \
  >"${TMP_DIR}/openshift-apiserver.log" 2>&1 &
OPENSHIFT_APISERVER_PID=$!
echo "openshift-apiserver started (PID: ${OPENSHIFT_APISERVER_PID})"

if ! wait_for_url "https://${OPENSHIFT_API_HOST}:${OPENSHIFT_API_PORT}/healthz" "openshift-apiserver" 120; then
  echo "Full log saved to: ${TMP_DIR}/openshift-apiserver.log"
  echo ""
  echo "Last 50 lines of log:"
  tail -50 "${TMP_DIR}/openshift-apiserver.log" || true
  echo ""
  echo "To inspect:"
  echo "  cat ${TMP_DIR}/openshift-apiserver.log"
  PRESERVE_TMP_DIR=true
  exit 1
fi

if ! wait_for_openapi_aggregation "https://${OPENSHIFT_API_HOST}:${OPENSHIFT_API_PORT}" 120; then
  echo "Full log saved to: ${TMP_DIR}/openshift-apiserver.log"
  PRESERVE_TMP_DIR=true
  exit 1
fi

# Fetch OpenAPI schemas
base_url="https://${OPENSHIFT_API_HOST}:${OPENSHIFT_API_PORT}"
echo "Fetching OpenAPI schemas from ${base_url}..."
echo ""

# Fetch OpenAPI v2 schema
echo "Updating ${OPENAPI_ROOT_DIR}/swagger.json"
mkdir -p "${OPENAPI_ROOT_DIR}"
curl -w "\n" -kfsS "${base_url}/openapi/v2" \
  | jq -S '.info.version="unversioned"' \
  > "${OPENAPI_ROOT_DIR}/swagger.json"

# Fetch aggregated discovery
echo "Updating ${DISCOVERY_ROOT_DIR}/aggregated_v2.json"
mkdir -p "${DISCOVERY_ROOT_DIR}"
rm -rf "${DISCOVERY_ROOT_DIR:?}"/*
curl -w "\n" -kfsS --cert "${CERT_DIR}/client.crt" --key "${CERT_DIR}/client.key" \
  -H 'Accept: application/json;g=apidiscovery.k8s.io;v=v2;as=APIGroupDiscoveryList' \
  "${base_url}/apis" \
  | jq -S . \
  > "${DISCOVERY_ROOT_DIR}/aggregated_v2.json"

# Fetch /apis discovery (APIGroupList)
echo "Updating ${DISCOVERY_ROOT_DIR}/apis.json"
curl -w "\n" -kfsS --cert "${CERT_DIR}/client.crt" --key "${CERT_DIR}/client.key" \
  "${base_url}/apis" \
  | jq -S . \
  > "${DISCOVERY_ROOT_DIR}/apis.json"

# Fetch OpenAPI v3 discovery document
# Note: We strip the hash query parameters from serverRelativeURL values because
# these hashes are non-deterministic (generated at runtime by the OpenAPI aggregator)
# and would cause spurious diffs on every run even when the actual API content is unchanged.
echo "Updating ${DISCOVERY_ROOT_DIR}/v3-discovery.json"
curl -w "\n" -kfsS "${base_url}/openapi/v3" \
  | jq -S '.paths |= with_entries(.value.serverRelativeURL |= sub("\\?hash=.*$"; ""))' \
  > "${DISCOVERY_ROOT_DIR}/v3-discovery.json"

# Fetch all v3 group schemas
echo "Updating ${OPENAPI_ROOT_DIR}/v3 for OpenAPI v3"
echo ""
mkdir -p "${OPENAPI_ROOT_DIR}/v3"
rm -rf "${OPENAPI_ROOT_DIR:?}"/v3/* || true

curl -w "\n" -kfsS "${base_url}/openapi/v3" \
  | jq -r '.paths | to_entries | .[].key' \
  | while read -r group; do
    echo "Updating OpenAPI spec and discovery for group ${group}"
    openapi_filename="${group}_openapi.json"
    openapi_filename_escaped="${openapi_filename//\//__}"
    openapi_path="${OPENAPI_ROOT_DIR}/v3/${openapi_filename_escaped}"
    curl -w "\n" -kfsS "${base_url}/openapi/v3/${group}" \
      | jq -S '.info.version="unversioned"' \
      > "${openapi_path}"

    if [[ "${group}" == apis/* ]]; then
      discovery_filename="${group}.json"
      discovery_filename_escaped="${discovery_filename//\//__}"
      discovery_path="${DISCOVERY_ROOT_DIR}/${discovery_filename_escaped}"
      curl -w "\n" -kfsS --cert "${CERT_DIR}/client.crt" --key "${CERT_DIR}/client.key" "${base_url}/${group}" \
        | jq -S . \
        > "${discovery_path}"
    fi
  done

echo ""
echo "OpenAPI schemas and discovery files generated successfully:"
echo "  - ${OPENAPI_ROOT_DIR}/swagger.json ($(du -h "${OPENAPI_ROOT_DIR}/swagger.json" | cut -f1))"
echo "  - ${OPENAPI_ROOT_DIR}/v3/*.json ($(ls -1 "${OPENAPI_ROOT_DIR}/v3"/*.json 2>/dev/null | wc -l) OpenAPI v3 files)"
echo "  - ${DISCOVERY_ROOT_DIR}/aggregated_v2.json ($(du -h "${DISCOVERY_ROOT_DIR}/aggregated_v2.json" | cut -f1))"
echo "  - ${DISCOVERY_ROOT_DIR}/v3-discovery.json ($(du -h "${DISCOVERY_ROOT_DIR}/v3-discovery.json" | cut -f1))"
discovery_count=$(ls -1 "${DISCOVERY_ROOT_DIR}"/*.json 2>/dev/null | wc -l)
echo "  - ${DISCOVERY_ROOT_DIR}/*.json (${discovery_count} total discovery files)"
echo ""
echo "SUCCESS: OpenAPI schemas updated in ${OPENAPI_ROOT_DIR}"
