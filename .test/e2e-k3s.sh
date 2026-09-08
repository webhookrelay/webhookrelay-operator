#!/usr/bin/env bash

set -Eeuo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
readonly REPO_ROOT
readonly K3S_VERSION="v1.35.6+k3s1"
readonly K3S_RELEASE_URL="https://github.com/k3s-io/k3s/releases/download/${K3S_VERSION}"

RUN_ID="${WHR_OPERATOR_E2E_RUN_ID:-$(date -u +%Y%m%d%H%M%S)-$$}"
RUN_DIR="${REPO_ROOT}/.test/.runs/${RUN_ID}"
ARTIFACT_DIR="${REPO_ROOT}/.test/artifacts/${RUN_ID}"
BIN_DIR="${RUN_DIR}/bin"
K3S_BIN="${BIN_DIR}/k3s"
HELM_BIN="${BIN_DIR}/helm"
KUBECONFIG="${RUN_DIR}/kubeconfig"
K3S_CONFIG="${RUN_DIR}/k3s.yaml"
K3S_PID_FILE="${RUN_DIR}/k3s.pid"
K3S_LOG="${ARTIFACT_DIR}/k3s.log"
K3S_DATA_DIR="/var/lib/webhookrelay-operator-e2e-${RUN_ID}"
K3S_CLIENT_DATA_DIR="${RUN_DIR}/client-data"
NAMESPACE="webhookrelay-operator-e2e"
IMAGE="webhookrelay-operator-e2e:${RUN_ID}"
PRODUCTION_MODE="${WHR_OPERATOR_E2E_PRODUCTION:-false}"
BUCKET_NAME="operator-e2e-${RUN_ID}"
BUCKET_DESCRIPTION="Webhook Relay operator production e2e run ${RUN_ID}"
TEST_STATUS=0

log() {
  printf '[operator-e2e] %s\n' "$*"
}

fail() {
  log "ERROR: $*"
  return 1
}

preflight() {
  local command
  for command in awk curl docker grep jq setsid sha256sum sudo tar; do
    command -v "${command}" >/dev/null || fail "required command not found: ${command}"
  done
  sudo -n true >/dev/null 2>&1 || fail "passwordless sudo is required"
  docker info >/dev/null 2>&1 || fail "Docker daemon is not available"
  [[ "${RUN_ID}" =~ ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ ]] || fail "run ID must be a DNS label"
  ((${#RUN_ID} <= 40)) || fail "run ID must be at most 40 characters"
  [[ ! -e "${RUN_DIR}" ]] || fail "run directory already exists: ${RUN_DIR}"
  [[ ! -e "${K3S_DATA_DIR}" ]] || fail "k3s data directory already exists: ${K3S_DATA_DIR}"
  pgrep -x k3s >/dev/null 2>&1 && fail "an existing k3s process is active"
  [[ ! -e /etc/rancher/k3s/k3s.yaml ]] || fail "an existing k3s kubeconfig was found"
  [[ ! -e /var/lib/rancher/k3s ]] || fail "the default k3s data directory already exists"
  [[ ! -e /var/lib/kubelet ]] || fail "the default kubelet data directory already exists"
  [[ ! -e /run/k3s ]] || fail "the k3s runtime directory already exists"
  mkdir -p "${RUN_DIR}"
  if [[ "${PRODUCTION_MODE}" == "true" ]]; then
    [[ -n "${WHR_E2E_RELAY_KEY:-}" ]] || fail "WHR_E2E_RELAY_KEY is required in production mode"
    [[ -n "${WHR_E2E_RELAY_SECRET:-}" ]] || fail "WHR_E2E_RELAY_SECRET is required in production mode"
    umask 077
    printf 'user = "%s:%s"\n' "${WHR_E2E_RELAY_KEY}" "${WHR_E2E_RELAY_SECRET}" \
      >"${RUN_DIR}/production-curl.conf"
  elif [[ "${PRODUCTION_MODE}" != "false" ]]; then
    fail "WHR_OPERATOR_E2E_PRODUCTION must be true or false"
  fi
}

download_tools() {
  mkdir -p "${BIN_DIR}" "${ARTIFACT_DIR}"
  curl --fail --location --show-error --silent \
    --output "${K3S_BIN}" "${K3S_RELEASE_URL}/k3s"
  curl --fail --location --show-error --silent \
    --output "${RUN_DIR}/sha256sum-amd64.txt" "${K3S_RELEASE_URL}/sha256sum-amd64.txt"
  (
    cd "${BIN_DIR}"
    grep -E '  k3s$' "${RUN_DIR}/sha256sum-amd64.txt" | sha256sum --check --strict -
  )
  chmod 0755 "${K3S_BIN}"
  "${REPO_ROOT}/.test/install-helm.sh" "${HELM_BIN}"
  "${K3S_BIN}" --version
  "${HELM_BIN}" version --short
}

start_k3s() {
  local node_name="whr-operator-e2e-${RUN_ID}"
  local pid

  sudo install -d -m 0755 -o root -g root "${K3S_DATA_DIR}"
  umask 077
  {
    printf 'data-dir: %s\n' "${K3S_DATA_DIR}"
    printf 'write-kubeconfig: %s\n' "${KUBECONFIG}"
    printf 'write-kubeconfig-mode: "0600"\n'
    printf 'node-name: %s\n' "${node_name}"
    printf 'snapshotter: native\n'
    printf 'disable:\n  - traefik\n  - servicelb\n  - metrics-server\n  - local-storage\n'
  } >"${K3S_CONFIG}"

  log "starting isolated k3s ${K3S_VERSION}"
  # The invoking user owns the diagnostic file; only k3s runs as root.
  # shellcheck disable=SC2024
  sudo env K3S_DATA_DIR="${K3S_DATA_DIR}" \
    setsid "${K3S_BIN}" server --config "${K3S_CONFIG}" >"${K3S_LOG}" 2>&1 &

  for _ in $(seq 1 30); do
    pid="$(sudo pgrep -f -x "${K3S_BIN} server --config ${K3S_CONFIG}" || true)"
    if [[ "${pid}" =~ ^[0-9]+$ ]]; then
      printf '%s\n' "${pid}" >"${K3S_PID_FILE}"
      break
    fi
    sleep 1
  done
  [[ -s "${K3S_PID_FILE}" ]] || fail "could not record the task-owned k3s process"

  for _ in $(seq 1 120); do
    if [[ -s "${KUBECONFIG}" ]] && sudo env K3S_DATA_DIR="${K3S_DATA_DIR}" \
      "${K3S_BIN}" kubectl --kubeconfig "${KUBECONFIG}" get "node/${node_name}" >/dev/null 2>&1; then
      sudo chown "$(id -u):$(id -g)" "${KUBECONFIG}"
      kubectl wait --for=condition=Ready "node/${node_name}" --timeout=120s
      return
    fi
    sleep 1
  done
  fail "k3s did not become ready; see ${K3S_LOG}"
}

kubectl() {
  env K3S_DATA_DIR="${K3S_CLIENT_DATA_DIR}" \
    "${K3S_BIN}" kubectl --kubeconfig "${KUBECONFIG}" "$@"
}

helm() {
  env KUBECONFIG="${KUBECONFIG}" "${HELM_BIN}" "$@"
}

build_and_import_image() {
  local archive="${RUN_DIR}/operator-image.tar"
  log "building operator image ${IMAGE}"
  docker build --tag "${IMAGE}" --file "${REPO_ROOT}/build/Dockerfile" "${REPO_ROOT}"
  docker save --output "${archive}" "${IMAGE}"
  sudo env K3S_DATA_DIR="${K3S_DATA_DIR}" "${K3S_BIN}" ctr \
    --address /run/k3s/containerd/containerd.sock --namespace k8s.io images import "${archive}"
}

install_operator() {
  local -a helm_args
  log "installing the chart"
  helm_args=(upgrade --install webhookrelay-operator "${REPO_ROOT}/charts/webhookrelay-operator" \
    --namespace "${NAMESPACE}" --create-namespace --wait --timeout 180s \
    --set-string image.repository=webhookrelay-operator-e2e \
    --set-string "image.tag=${RUN_ID}" \
    --set image.pullPolicy=IfNotPresent)
  if [[ "${PRODUCTION_MODE}" == "false" ]]; then
    helm_args+=(--set-string httpsProxy=http://127.0.0.1:9)
  fi
  helm "${helm_args[@]}"
  kubectl -n "${NAMESPACE}" rollout status deployment/webhookrelay-operator --timeout=120s
  kubectl -n "${NAMESPACE}" get deployment/webhookrelay-operator -o json | jq -e --arg image "${IMAGE}" '
      .spec.template.spec.serviceAccountName == "webhookrelay-operator" and
      .spec.template.spec.containers[0].image == $image
    ' >/dev/null || fail "operator Deployment wiring is invalid"
}

apply_forward() {
  local response_body="$1"
  local credentials_file="${RUN_DIR}/credentials.env"
  local relay_key="e2e-invalid-key"
  local relay_secret="e2e-invalid-secret"
  if [[ "${PRODUCTION_MODE}" == "true" ]]; then
    relay_key="${WHR_E2E_RELAY_KEY}"
    relay_secret="${WHR_E2E_RELAY_SECRET}"
  fi
  umask 077
  printf 'key=%s\nsecret=%s\n' "${relay_key}" "${relay_secret}" >"${credentials_file}"
  kubectl -n "${NAMESPACE}" create secret generic e2e-credentials \
    --from-env-file="${credentials_file}" --dry-run=client -o yaml | kubectl apply -f -
  kubectl apply -f - <<EOF
apiVersion: forward.webhookrelay.com/v1
kind: WebhookRelayForward
metadata:
  name: e2e-forward
  namespace: ${NAMESPACE}
spec:
  secretRefName: e2e-credentials
  image: busybox:1.36.1
  buckets:
    - name: ${BUCKET_NAME}
      description: ${BUCKET_DESCRIPTION}
      inputs:
        - name: e2e-input
          description: ${BUCKET_DESCRIPTION}
          responseBody: ${response_body}
          responseStatusCode: 202
      outputs:
        - name: e2e-output
          description: ${BUCKET_DESCRIPTION}
          destination: https://example.invalid/operator-e2e
          disabled: true
          internal: false
EOF
}

production_api() {
  curl --fail --show-error --silent --config "${RUN_DIR}/production-curl.conf" "$@"
}

exercise_reconcile() {
  if [[ "${PRODUCTION_MODE}" == "true" ]]; then
    log "creating production routing resources owned by run ${RUN_ID}"
  else
    log "creating a CR with isolated, intentionally invalid credentials"
  fi
  apply_forward operator-e2e-v1

  for _ in $(seq 1 90); do
    if kubectl -n "${NAMESPACE}" get deployment/e2e-forward-whr-deployment >/dev/null 2>&1; then
      break
    fi
    sleep 2
  done
  kubectl -n "${NAMESPACE}" get deployment/e2e-forward-whr-deployment -o json \
    >"${ARTIFACT_DIR}/reconciled-deployment.json"
  jq -e '
    .metadata.ownerReferences[0].kind == "WebhookRelayForward" and
    .spec.template.spec.containers[0].image == "busybox:1.36.1" and
    any(.spec.template.spec.containers[0].env[];
      .name == "KEY" and .valueFrom.secretKeyRef.name == "e2e-credentials") and
    any(.spec.template.spec.containers[0].env[];
      .name == "SECRET" and .valueFrom.secretKeyRef.name == "e2e-credentials")
  ' "${ARTIFACT_DIR}/reconciled-deployment.json" >/dev/null || fail "reconciled Deployment is invalid"

  if [[ "${PRODUCTION_MODE}" == "true" ]]; then
    exercise_production_reconcile
  else
    kubectl -n "${NAMESPACE}" get webhookrelayforward/e2e-forward -o json \
      >"${ARTIFACT_DIR}/reconciled-forward.json"
    jq -e '.status.routingStatus == "Failed"' "${ARTIFACT_DIR}/reconciled-forward.json" >/dev/null || \
      fail "the CR did not report the expected isolated API failure"
  fi
}

exercise_production_reconcile() {
  local bucket_id input_id output_id
  for _ in $(seq 1 120); do
    kubectl -n "${NAMESPACE}" get webhookrelayforward/e2e-forward -o json \
      >"${ARTIFACT_DIR}/reconciled-forward.json"
    production_api "https://my.webhookrelay.com/v1/buckets" >"${RUN_DIR}/production-buckets.json"
    if jq -e --arg name "${BUCKET_NAME}" --arg description "${BUCKET_DESCRIPTION}" '
      any(.[]; .name == $name and .description == $description and
        any(.inputs[]?; .name == "e2e-input" and .body == "operator-e2e-v1" and .status_code == 202) and
        any(.outputs[]?; .name == "e2e-output" and .disabled == true and .destination == "https://example.invalid/operator-e2e"))
    ' "${RUN_DIR}/production-buckets.json" >/dev/null &&
      jq -e '.status.routingStatus == "Configured"' "${ARTIFACT_DIR}/reconciled-forward.json" >/dev/null; then
      break
    fi
    sleep 2
  done

  bucket_id="$(jq -r --arg name "${BUCKET_NAME}" --arg description "${BUCKET_DESCRIPTION}" \
    '.[] | select(.name == $name and .description == $description) | .id' "${RUN_DIR}/production-buckets.json")"
  input_id="$(jq -r --arg name "${BUCKET_NAME}" \
    '.[] | select(.name == $name) | .inputs[] | select(.name == "e2e-input") | .id' "${RUN_DIR}/production-buckets.json")"
  output_id="$(jq -r --arg name "${BUCKET_NAME}" \
    '.[] | select(.name == $name) | .outputs[] | select(.name == "e2e-output") | .id' "${RUN_DIR}/production-buckets.json")"
  [[ -n "${bucket_id}" && -n "${input_id}" && -n "${output_id}" ]] || fail "production resources did not converge"

  log "updating the CR and checking idempotent resource identities"
  apply_forward operator-e2e-v2
  for _ in $(seq 1 90); do
    production_api "https://my.webhookrelay.com/v1/buckets" >"${RUN_DIR}/production-buckets.json"
    if jq -e --arg name "${BUCKET_NAME}" --arg bucket "${bucket_id}" \
      --arg input "${input_id}" --arg output "${output_id}" '
      any(.[]; .name == $name and .id == $bucket and
        any(.inputs[]?; .id == $input and .body == "operator-e2e-v2") and
        any(.outputs[]?; .id == $output))
    ' "${RUN_DIR}/production-buckets.json" >/dev/null; then
      log "production reconciliation passed"
      return
    fi
    sleep 2
  done
  fail "production resources did not update idempotently"
}

cleanup_production_resources() {
  local bucket_count bucket_id
  [[ "${PRODUCTION_MODE}" == "true" ]] || return 0
  [[ -s "${RUN_DIR}/production-curl.conf" ]] || return 0
  production_api "https://my.webhookrelay.com/v1/buckets" >"${RUN_DIR}/cleanup-buckets.json" || return 1
  bucket_count="$(jq --arg name "${BUCKET_NAME}" --arg description "${BUCKET_DESCRIPTION}" \
    '[.[] | select(.name == $name and .description == $description)] | length' "${RUN_DIR}/cleanup-buckets.json")"
  [[ "${bucket_count}" == "0" ]] && return 0
  [[ "${bucket_count}" == "1" ]] || fail "refusing to delete ambiguous production resources"
  bucket_id="$(jq -r --arg name "${BUCKET_NAME}" --arg description "${BUCKET_DESCRIPTION}" \
    '.[] | select(.name == $name and .description == $description) | .id' "${RUN_DIR}/cleanup-buckets.json")"
  [[ -n "${bucket_id}" ]] || return 1
  production_api --request DELETE \
    "https://my.webhookrelay.com/v1/buckets/${bucket_id}?force=true" >/dev/null
  log "deleted production bucket owned by run ${RUN_ID}"
}

collect_diagnostics() {
  [[ -s "${KUBECONFIG}" ]] || return 0
  kubectl get nodes,pods,deployments -A -o wide >"${ARTIFACT_DIR}/objects.txt" 2>&1 || true
  kubectl get events -A --sort-by=.lastTimestamp >"${ARTIFACT_DIR}/events.txt" 2>&1 || true
  kubectl -n "${NAMESPACE}" describe deployment webhookrelay-operator \
    >"${ARTIFACT_DIR}/operator-deployment.txt" 2>&1 || true
  kubectl -n "${NAMESPACE}" logs deployment/webhookrelay-operator --all-containers \
    >"${ARTIFACT_DIR}/operator.log" 2>&1 || true
}

cleanup() {
  local pid
  TEST_STATUS=$?
  trap - EXIT INT TERM
  if ((TEST_STATUS != 0)); then
    collect_diagnostics
    log "failure diagnostics retained in ${ARTIFACT_DIR}"
  fi
  if ! cleanup_production_resources; then
    log "ERROR: failed to clean up production resources owned by run ${RUN_ID}"
    TEST_STATUS=1
  fi
  if [[ -s "${K3S_PID_FILE}" ]]; then
    pid="$(<"${K3S_PID_FILE}")"
    if [[ "${pid}" =~ ^[0-9]+$ ]] && sudo kill -0 "${pid}" 2>/dev/null; then
      sudo kill -TERM -- "-${pid}" 2>/dev/null || sudo kill -TERM "${pid}" 2>/dev/null || true
      for _ in $(seq 1 30); do
        sudo kill -0 "${pid}" 2>/dev/null || break
        sleep 1
      done
    fi
  fi
  if [[ "${K3S_DATA_DIR}" == "/var/lib/webhookrelay-operator-e2e-${RUN_ID}" ]]; then
    sudo "${K3S_BIN}" killall >/dev/null 2>&1 || true
    sudo rm -rf -- "${K3S_DATA_DIR}"
  fi
  if ((TEST_STATUS == 0)); then
    rm -rf -- "${RUN_DIR}" "${ARTIFACT_DIR}"
    log "end-to-end test passed"
  fi
  exit "${TEST_STATUS}"
}

trap cleanup EXIT INT TERM
preflight
download_tools
start_k3s
build_and_import_image
install_operator
exercise_reconcile
