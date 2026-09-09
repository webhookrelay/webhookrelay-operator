#!/usr/bin/env bash

set -Eeuo pipefail

readonly REPO_ROOT="$(git rev-parse --show-toplevel)"
readonly TEST_ROOT="$(mktemp -d)"
readonly SERVER_DIR="${TEST_ROOT}/server"
readonly TEST_OUTPUT_DIR="${TEST_ROOT}/output"
readonly LOCAL_REPOSITORY_URL="http://127.0.0.1:18080"
SERVER_PID=""

cleanup() {
  if [[ "${SERVER_PID}" =~ ^[0-9]+$ ]]; then
    kill "${SERVER_PID}" >/dev/null 2>&1 || true
    wait "${SERVER_PID}" 2>/dev/null || true
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

printf 'chart release retry and collision checks passed\n'
