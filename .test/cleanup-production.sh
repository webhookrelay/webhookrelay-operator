#!/usr/bin/env bash

set -Eeuo pipefail

readonly RUN_ID="${WHR_OPERATOR_E2E_RUN_ID:?run ID is required}"
readonly BUCKET_NAME="operator-e2e-${RUN_ID}"
readonly BUCKET_DESCRIPTION="Webhook Relay operator production e2e run ${RUN_ID}"
readonly API_URL="https://my.webhookrelay.com/v1/buckets"
readonly REPO_ROOT="$(git rev-parse --show-toplevel)"
readonly RUN_DIR="${REPO_ROOT}/.test/.runs/${RUN_ID}"
readonly K3S_DATA_DIR="/var/lib/webhookrelay-operator-e2e-${RUN_ID}"
readonly AUTH_FILE="$(mktemp)"
readonly RESPONSE_FILE="$(mktemp)"
trap 'rm -f -- "${AUTH_FILE}" "${RESPONSE_FILE}"' EXIT

umask 077

# If the timed main step left its task-owned cluster alive, make a bounded best
# effort to stop reconciliation cleanly, then stop the task-owned K3s process
# before deleting remote state so the bucket cannot be recreated afterward.
if [[ -s "${RUN_DIR}/k3s.pid" ]]; then
  pid="$(<"${RUN_DIR}/k3s.pid")"
  [[ "${pid}" =~ ^[0-9]+$ ]] || { printf 'invalid task-owned k3s PID\n' >&2; exit 1; }
  if sudo kill -0 "${pid}" 2>/dev/null; then
    if [[ -x "${RUN_DIR}/bin/k3s" && -s "${RUN_DIR}/kubeconfig" ]]; then
      kube=(sudo env K3S_DATA_DIR="${K3S_DATA_DIR}" "${RUN_DIR}/bin/k3s" kubectl \
        --kubeconfig "${RUN_DIR}/kubeconfig" --namespace webhookrelay-operator-e2e)
      timeout 20s "${kube[@]}" scale deployment/webhookrelay-operator --replicas=0 \
        --request-timeout=10s >/dev/null 2>&1 || true
      timeout 20s "${kube[@]}" delete webhookrelayforward/e2e-forward \
        --ignore-not-found --wait=true --timeout=10s --request-timeout=10s >/dev/null 2>&1 || true
    fi
    sudo kill -TERM -- "-${pid}" 2>/dev/null || sudo kill -TERM "${pid}" 2>/dev/null || true
    for _ in $(seq 1 30); do
      sudo kill -0 "${pid}" 2>/dev/null || break
      sleep 1
    done
    if sudo kill -0 "${pid}" 2>/dev/null; then
      sudo kill -KILL -- "-${pid}" 2>/dev/null || sudo kill -KILL "${pid}" 2>/dev/null || true
      sleep 1
    fi
    sudo kill -0 "${pid}" 2>/dev/null && { printf 'could not stop task-owned k3s\n' >&2; exit 1; }
  fi
  [[ -x "${RUN_DIR}/bin/k3s" ]] || { printf 'task-owned k3s binary is missing\n' >&2; exit 1; }
  sudo env K3S_DATA_DIR="${K3S_DATA_DIR}" "${RUN_DIR}/bin/k3s" killall >/dev/null
fi

printf 'user = "%s:%s"\n' "${WHR_E2E_RELAY_KEY:?relay key is required}" \
  "${WHR_E2E_RELAY_SECRET:?relay secret is required}" >"${AUTH_FILE}"
curl --fail --show-error --silent --config "${AUTH_FILE}" "${API_URL}" >"${RESPONSE_FILE}"
count="$(jq --arg name "${BUCKET_NAME}" --arg description "${BUCKET_DESCRIPTION}" \
  '[.[] | select(.name == $name and .description == $description)] | length' "${RESPONSE_FILE}")"
[[ "${count}" != "0" ]] || exit 0
[[ "${count}" == "1" ]] || { printf 'refusing ambiguous production cleanup\n' >&2; exit 1; }
bucket_id="$(jq -r --arg name "${BUCKET_NAME}" --arg description "${BUCKET_DESCRIPTION}" \
  '.[] | select(.name == $name and .description == $description) | .id' "${RESPONSE_FILE}")"
curl --fail --show-error --silent --config "${AUTH_FILE}" --request DELETE \
  "${API_URL}/${bucket_id}?force=true" >/dev/null
curl --fail --show-error --silent --config "${AUTH_FILE}" "${API_URL}" >"${RESPONSE_FILE}"
jq -e --arg name "${BUCKET_NAME}" --arg description "${BUCKET_DESCRIPTION}" \
  'all(.[]; .name != $name or .description != $description)' "${RESPONSE_FILE}" >/dev/null
