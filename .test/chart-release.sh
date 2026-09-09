#!/usr/bin/env bash

set -Eeuo pipefail

readonly REPO_ROOT="$(git rev-parse --show-toplevel)"
readonly TEST_ROOT="$(mktemp -d)"
readonly SERVER_DIR="${TEST_ROOT}/server"
readonly TEST_OUTPUT_DIR="${TEST_ROOT}/output"
readonly LOCAL_REPOSITORY_URL="http://127.0.0.1:18080"
SERVER_PID=""
SYNC_PID=""

cleanup() {
  if [[ "${SERVER_PID}" =~ ^[0-9]+$ ]]; then
    kill "${SERVER_PID}" >/dev/null 2>&1 || true
    wait "${SERVER_PID}" 2>/dev/null || true
  fi
  if [[ "${SYNC_PID}" =~ ^[0-9]+$ ]]; then
    kill "${SYNC_PID}" >/dev/null 2>&1 || true
    wait "${SYNC_PID}" 2>/dev/null || true
  fi
  rm -rf -- "${TEST_ROOT}"
}
trap cleanup EXIT

mkdir -p "${SERVER_DIR}"
HELM_BIN="${HELM_BIN:-helm}" OUTPUT_DIR="${TEST_ROOT}/source" \
  "${REPO_ROOT}/.scripts/chart-release.sh" package >/dev/null
cp "${TEST_ROOT}/source/webhookrelay-operator-0.7.0.tgz" "${SERVER_DIR}/"
"${HELM_BIN:-helm}" repo index "${SERVER_DIR}" --url "${LOCAL_REPOSITORY_URL}"
python3 -m http.server 18080 --bind 127.0.0.1 --directory "${SERVER_DIR}" \
  >"${TEST_ROOT}/http.log" 2>&1 &
SERVER_PID=$!
for _ in $(seq 1 30); do
  curl --fail --silent "${LOCAL_REPOSITORY_URL}/index.yaml" >/dev/null && break
  sleep 0.1
done

HELM_BIN="${HELM_BIN:-helm}" OUTPUT_DIR="${TEST_OUTPUT_DIR}" REPOSITORY_URL="${LOCAL_REPOSITORY_URL}" \
  "${REPO_ROOT}/.scripts/chart-release.sh" verify-remote >/dev/null

printf 'different artifact\n' >"${SERVER_DIR}/webhookrelay-operator-0.7.0.tgz"
if HELM_BIN="${HELM_BIN:-helm}" OUTPUT_DIR="${TEST_OUTPUT_DIR}" REPOSITORY_URL="${LOCAL_REPOSITORY_URL}" \
  "${REPO_ROOT}/.scripts/chart-release.sh" verify-remote >/dev/null 2>&1; then
  printf 'chart release accepted a different published artifact\n' >&2
  exit 1
fi

fake_bin="${TEST_ROOT}/bin"
fake_store="${TEST_ROOT}/gcs"
mkdir -p "${fake_bin}" "${fake_store}"
cp "${REPO_ROOT}/.test/fake-gcloud.sh" "${fake_bin}/gcloud"
chmod 0755 "${fake_bin}/gcloud"
cp "${TEST_ROOT}/source/webhookrelay-operator-0.7.0.tgz" \
  "${SERVER_DIR}/webhookrelay-operator-0.7.0.tgz"
cat >"${fake_store}/index.yaml" <<EOF
apiVersion: v1
entries:
  webhookrelay-operator:
  - apiVersion: v2
    digest: retained-digest
    name: webhookrelay-operator
    urls:
    - ${LOCAL_REPOSITORY_URL}/webhookrelay-operator-0.6.0.tgz
    version: 0.6.0
EOF
printf '10\n' >"${fake_store}/index.yaml.generation"
cp "${fake_store}/index.yaml" "${SERVER_DIR}/index.yaml"
rm -f "${SERVER_DIR}/webhookrelay-operator-0.7.0.tgz"

if PATH="${fake_bin}:${PATH}" FAKE_GCS_ROOT="${fake_store}" \
  FAKE_GCS_BUMP_BEFORE_INDEX_WRITE=true HELM_BIN="${HELM_BIN:-helm}" \
  OUTPUT_DIR="${TEST_ROOT}/publish-first" REPOSITORY_URL="${LOCAL_REPOSITORY_URL}" \
  CHART_BUCKET=test-bucket VALIDATED_ARTIFACT="${TEST_ROOT}/source/webhookrelay-operator-0.7.0.tgz" \
  "${REPO_ROOT}/.scripts/chart-release.sh" publish >/dev/null 2>&1; then
  printf 'chart release ignored a concurrent index generation change\n' >&2
  exit 1
fi
[[ -f "${fake_store}/webhookrelay-operator-0.7.0.tgz" ]] || {
  printf 'partial publication fixture did not create the chart object\n' >&2
  exit 1
}

(
  for _ in $(seq 1 100); do
    if grep -q 'version: 0.7.0' "${fake_store}/index.yaml" 2>/dev/null; then
      cp "${fake_store}/index.yaml" "${SERVER_DIR}/index.yaml"
      cp "${fake_store}/webhookrelay-operator-0.7.0.tgz" "${SERVER_DIR}/"
      exit 0
    fi
    sleep 0.1
  done
  exit 1
) &
SYNC_PID=$!
PATH="${fake_bin}:${PATH}" FAKE_GCS_ROOT="${fake_store}" HELM_BIN="${HELM_BIN:-helm}" \
  OUTPUT_DIR="${TEST_ROOT}/publish-retry" REPOSITORY_URL="${LOCAL_REPOSITORY_URL}" \
  CHART_BUCKET=test-bucket VALIDATED_ARTIFACT="${TEST_ROOT}/source/webhookrelay-operator-0.7.0.tgz" \
  "${REPO_ROOT}/.scripts/chart-release.sh" publish >/dev/null
wait "${SYNC_PID}"
SYNC_PID=""
go run "${REPO_ROOT}/.scripts/chart-artifact.go" verify-retained \
  "${TEST_ROOT}/publish-retry/remote-index.yaml" "${fake_store}/index.yaml"
[[ "$(<"${fake_store}/index.yaml.cache-control")" == "no-cache,max-age=0,must-revalidate" ]]

artifact_generation="$(<"${fake_store}/webhookrelay-operator-0.7.0.tgz.generation")"
index_generation="$(<"${fake_store}/index.yaml.generation")"
PATH="${fake_bin}:${PATH}" FAKE_GCS_ROOT="${fake_store}" HELM_BIN="${HELM_BIN:-helm}" \
  OUTPUT_DIR="${TEST_ROOT}/publish-idempotent" REPOSITORY_URL="${LOCAL_REPOSITORY_URL}" \
  CHART_BUCKET=test-bucket VALIDATED_ARTIFACT="${TEST_ROOT}/source/webhookrelay-operator-0.7.0.tgz" \
  "${REPO_ROOT}/.scripts/chart-release.sh" publish >/dev/null
[[ "$(<"${fake_store}/webhookrelay-operator-0.7.0.tgz.generation")" == "${artifact_generation}" ]]
[[ "$(<"${fake_store}/index.yaml.generation")" == "${index_generation}" ]]

printf 'different authoritative artifact\n' >"${fake_store}/webhookrelay-operator-0.7.0.tgz"
if PATH="${fake_bin}:${PATH}" FAKE_GCS_ROOT="${fake_store}" HELM_BIN="${HELM_BIN:-helm}" \
  OUTPUT_DIR="${TEST_ROOT}/publish-collision" REPOSITORY_URL="${LOCAL_REPOSITORY_URL}" \
  CHART_BUCKET=test-bucket VALIDATED_ARTIFACT="${TEST_ROOT}/source/webhookrelay-operator-0.7.0.tgz" \
  "${REPO_ROOT}/.scripts/chart-release.sh" publish >/dev/null 2>&1; then
  printf 'chart release accepted different authoritative artifact bytes\n' >&2
  exit 1
fi

printf 'chart release authoritative retry, collision, and concurrency checks passed\n'
