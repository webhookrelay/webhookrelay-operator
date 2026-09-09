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
CANDIDATE_CHART="${RUN_DIR}/webhookrelay-operator-0.6.0.tgz"
OLD_CHART="${RUN_DIR}/webhookrelay-operator-0.4.1.tgz"
KUBECONFIG="${RUN_DIR}/kubeconfig"
K3S_CONFIG="${RUN_DIR}/k3s.yaml"
K3S_PID_FILE="${RUN_DIR}/k3s.pid"
K3S_LOG="${ARTIFACT_DIR}/k3s.log"
K3S_DATA_DIR="/var/lib/webhookrelay-operator-e2e-${RUN_ID}"
K3S_CLIENT_DATA_DIR="${RUN_DIR}/client-data"
NAMESPACE="webhookrelay-operator-e2e"
IMAGE="webhookrelay-operator-e2e:${RUN_ID}"
RECEIVER_IMAGE="webhookrelay-operator-e2e-receiver:${RUN_ID}"
FAKE_API_IMAGE="webhookrelay-operator-e2e-fake-api:${RUN_ID}"
PRODUCTION_MODE="${WHR_OPERATOR_E2E_PRODUCTION:-false}"
AGENT_IMAGE="${WHR_E2E_AGENT_IMAGE:-webhookrelay/webhookrelayd-ubi8:latest}"
FUNCTION_ID="${WHR_E2E_FUNCTION_ID:-}"
BUCKET_NAME="operator-e2e-${RUN_ID}"
BUCKET_DESCRIPTION="Webhook Relay operator production e2e run ${RUN_ID}"
BUCKET_AUTH_PASSWORD="auth-${RUN_ID}"
PRODUCTION_RESOURCES_STARTED=false
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
    [[ -n "${FUNCTION_ID}" ]] || fail "WHR_E2E_FUNCTION_ID is required in production mode"
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

package_candidate_chart() {
  local packaged_chart
  log "packaging candidate Helm chart"
  helm package "${REPO_ROOT}/charts/webhookrelay-operator" --destination "${RUN_DIR}" \
    >"${ARTIFACT_DIR}/helm-package.txt"
  packaged_chart="$(awk '/Successfully packaged chart and saved it to:/ {print $NF}' \
    "${ARTIFACT_DIR}/helm-package.txt")"
  [[ "${packaged_chart}" == "${CANDIDATE_CHART}" ]] || \
    fail "candidate chart package was ${packaged_chart}, expected ${CANDIDATE_CHART}"
  [[ -f "${CANDIDATE_CHART}" ]] || fail "candidate chart package was not created"
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
  pid=$!
  [[ "${pid}" =~ ^[0-9]+$ ]] || fail "could not record the task-owned k3s process"
  printf '%s\n' "${pid}" >"${K3S_PID_FILE}"

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

build_and_import_images() {
  local archive="${RUN_DIR}/operator-image.tar"
  log "building operator image ${IMAGE}"
  docker build --tag "${IMAGE}" --file "${REPO_ROOT}/build/Dockerfile" "${REPO_ROOT}"
  docker save --output "${archive}" "${IMAGE}"
  sudo env K3S_DATA_DIR="${K3S_DATA_DIR}" "${K3S_BIN}" ctr \
    --address /run/k3s/containerd/containerd.sock --namespace k8s.io images import "${archive}"

  if [[ "${PRODUCTION_MODE}" == "false" ]]; then
    archive="${RUN_DIR}/fake-api-image.tar"
    log "building fake Relay API image ${FAKE_API_IMAGE}"
    docker build --tag "${FAKE_API_IMAGE}" \
      --file "${REPO_ROOT}/.test/fake-api/Dockerfile" "${REPO_ROOT}"
    docker save --output "${archive}" "${FAKE_API_IMAGE}"
    sudo env K3S_DATA_DIR="${K3S_DATA_DIR}" "${K3S_BIN}" ctr \
      --address /run/k3s/containerd/containerd.sock --namespace k8s.io images import "${archive}"
  else
    archive="${RUN_DIR}/receiver-image.tar"
    log "building delivery receiver image ${RECEIVER_IMAGE}"
    docker build --tag "${RECEIVER_IMAGE}" \
      --file "${REPO_ROOT}/.test/receiver/Dockerfile" "${REPO_ROOT}"
    docker save --output "${archive}" "${RECEIVER_IMAGE}"
    sudo env K3S_DATA_DIR="${K3S_DATA_DIR}" "${K3S_BIN}" ctr \
      --address /run/k3s/containerd/containerd.sock --namespace k8s.io images import "${archive}"
  fi
}

install_operator() {
  local -a helm_args
  log "installing the chart"
  helm_args=(upgrade --install webhookrelay-operator "${CANDIDATE_CHART}" \
    --namespace "${NAMESPACE}" --create-namespace --wait --timeout 180s \
    --set-string image.repository=webhookrelay-operator-e2e \
    --set-string "image.tag=${RUN_ID}" \
    --set image.pullPolicy=IfNotPresent)
  if [[ "${PRODUCTION_MODE}" == "false" ]]; then
    helm_args+=(--set-string apiEndpointURL=http://relay-api:8080/v1)
  fi
  helm "${helm_args[@]}"
  kubectl -n "${NAMESPACE}" rollout status deployment/webhookrelay-operator --timeout=120s
  kubectl -n "${NAMESPACE}" get deployment/webhookrelay-operator -o json | jq -e --arg image "${IMAGE}" '
      .spec.template.spec.serviceAccountName == "webhookrelay-operator" and
      .spec.template.spec.containers[0].image == $image
    ' >/dev/null || fail "operator Deployment wiring is invalid"
}

install_fake_api() {
  [[ "${PRODUCTION_MODE}" == "false" ]] || return 0
  kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
  kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: relay-api
  namespace: ${NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: relay-api
  template:
    metadata:
      labels:
        app: relay-api
    spec:
      containers:
        - name: api
          image: ${FAKE_API_IMAGE}
          imagePullPolicy: Never
          ports:
            - name: http
              containerPort: 8080
          readinessProbe:
            httpGet:
              path: /healthz
              port: http
---
apiVersion: v1
kind: Service
metadata:
  name: relay-api
  namespace: ${NAMESPACE}
spec:
  selector:
    app: relay-api
  ports:
    - name: http
      port: 8080
      targetPort: http
EOF
  kubectl -n "${NAMESPACE}" rollout status deployment/relay-api --timeout=120s
}

install_receiver() {
  [[ "${PRODUCTION_MODE}" == "true" ]] || return 0
  kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: e2e-receiver
  namespace: ${NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: e2e-receiver
  template:
    metadata:
      labels:
        app: e2e-receiver
    spec:
      containers:
        - name: receiver
          image: ${RECEIVER_IMAGE}
          imagePullPolicy: Never
          ports:
            - name: http
              containerPort: 8080
          readinessProbe:
            httpGet:
              path: /healthz
              port: http
---
apiVersion: v1
kind: Service
metadata:
  name: e2e-receiver
  namespace: ${NAMESPACE}
spec:
  selector:
    app: e2e-receiver
  ports:
    - name: http
      port: 8080
      targetPort: http
EOF
  kubectl -n "${NAMESPACE}" rollout status deployment/e2e-receiver --timeout=120s
}

apply_isolated_forward() {
  local response_body="$1"
  local include_reconciliation_routes="${2:-false}"
  local agent_image="${3:-registry.k8s.io/pause:3.10}"
  local credentials_file="${RUN_DIR}/credentials.env"
  local retained_input_yaml=""
  local cleanup_output_yaml=""
  if [[ "${include_reconciliation_routes}" == "true" ]]; then
    retained_input_yaml="        - name: e2e-retained-input
          description: retained to preserve its public endpoint"
    cleanup_output_yaml="        - name: e2e-cleanup-output
          description: removed during reconciliation
          destination: https://example.invalid/operator-e2e-cleanup
          disabled: true
          internal: false"
  fi
  umask 077
  printf 'key=e2e-invalid-key\nsecret=e2e-invalid-secret\n' >"${credentials_file}"
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
  image: ${agent_image}
  websocketTransport: false
  resources:
    requests:
      cpu: 10m
      memory: 16Mi
    limits:
      cpu: 100m
      memory: 64Mi
  buckets:
    - name: ${BUCKET_NAME}
      description: ${BUCKET_DESCRIPTION}
      inputs:
        - name: e2e-input
          description: ${BUCKET_DESCRIPTION}
          responseBody: ${response_body}
          responseStatusCode: 202
${retained_input_yaml}
      outputs:
        - name: e2e-output
          description: ${BUCKET_DESCRIPTION}
          destination: https://example.invalid/operator-e2e
          disabled: true
          internal: false
${cleanup_output_yaml}
EOF
}

fake_api_state() {
  kubectl get --raw "/api/v1/namespaces/${NAMESPACE}/services/http:relay-api:8080/proxy/v1/state"
}

wait_for_isolated_routing_status() {
  local expected="$1"
  for _ in $(seq 1 60); do
    if [[ "$(kubectl -n "${NAMESPACE}" get webhookrelayforward/e2e-forward -o jsonpath='{.status.routingStatus}' 2>/dev/null)" == "${expected}" ]]; then
      return
    fi
    sleep 1
  done
  fail "isolated CR did not reach routing status ${expected}"
}

wait_for_isolated_condition() {
  local condition_type="$1"
  local expected_status="$2"
  local expected_reason="${3:-}"
  for _ in $(seq 1 90); do
    if kubectl -n "${NAMESPACE}" get webhookrelayforward/e2e-forward -o json | jq -e \
      --arg type "${condition_type}" --arg status "${expected_status}" --arg reason "${expected_reason}" '
        .metadata.generation as $generation |
        any(.status.conditions[]?;
          .type == $type and .status == $status and .observedGeneration == $generation and
          ($reason == "" or .reason == $reason))
      ' >/dev/null; then
      return
    fi
    sleep 1
  done
  fail "isolated CR condition ${condition_type} did not reach ${expected_status} for its current generation"
}

wait_for_isolated_image_pull_failure() {
  for _ in $(seq 1 90); do
    if kubectl -n "${NAMESPACE}" get pods -l name=webhookrelay-forwarder -o json | jq -e '
      any(.items[].status.containerStatuses[]?.state.waiting.reason;
        . == "ErrImagePull" or . == "ImagePullBackOff")
    ' >/dev/null; then
      return
    fi
    sleep 1
  done
  fail "isolated agent pod did not report ErrImagePull or ImagePullBackOff"
}

exercise_isolated_reconcile() {
  local before after
  wait_for_isolated_routing_status Configured
  fake_api_state >"${ARTIFACT_DIR}/isolated-state-v1.json"
  jq -e --arg name "${BUCKET_NAME}" '
    any(.buckets[]; .name == $name and .description != "" and
      any(.inputs[]; .name == "e2e-input" and .body == "operator-e2e-v1") and
      any(.outputs[]; .name == "e2e-output" and .disabled == true)) and
    any(.buckets[]; .name == $name and
      any(.inputs[]; .name == "e2e-retained-input") and
      any(.outputs[]; .name == "e2e-cleanup-output")) and
    .mutations.createBucket == 1 and .mutations.createInput == 2 and .mutations.createOutput == 2
  ' "${ARTIFACT_DIR}/isolated-state-v1.json" >/dev/null || fail "fake API did not observe initial convergence"

  log "updating isolated routing configuration"
  apply_isolated_forward operator-e2e-v2
  for _ in $(seq 1 60); do
    fake_api_state >"${ARTIFACT_DIR}/isolated-state-v2.json"
    if jq -e --arg name "${BUCKET_NAME}" '
      any(.buckets[]; .name == $name and any(.inputs[]; .name == "e2e-input" and .body == "operator-e2e-v2")) and
      all(.buckets[] | select(.name == $name);
        any(.inputs[]; .name == "e2e-retained-input") and
        all(.outputs[]; .name != "e2e-cleanup-output")) and
      .mutations.updateInput == 1 and (.mutations.deleteInput // 0) == 0 and .mutations.deleteOutput == 1
    ' "${ARTIFACT_DIR}/isolated-state-v2.json" >/dev/null; then
      break
    fi
    sleep 1
  done
  jq -e '
    .mutations.updateInput == 1 and (.mutations.deleteInput // 0) == 0 and .mutations.deleteOutput == 1
  ' "${ARTIFACT_DIR}/isolated-state-v2.json" >/dev/null || fail "fake API did not observe update and cleanup"

  before="$(jq -c '.mutations' "${ARTIFACT_DIR}/isolated-state-v2.json")"
  sleep 7
  fake_api_state >"${ARTIFACT_DIR}/isolated-state-idempotent.json"
  after="$(jq -c '.mutations' "${ARTIFACT_DIR}/isolated-state-idempotent.json")"
  [[ "${before}" == "${after}" ]] || fail "isolated reconcile was not idempotent"

  log "testing isolated API failure and recovery"
  kubectl -n "${NAMESPACE}" patch webhookrelayforward/e2e-forward --type=json \
    -p='[{"op":"add","path":"/spec/buckets/-","value":{"name":"force-error","inputs":[],"outputs":[]}}]'
  wait_for_isolated_routing_status Failed
  apply_isolated_forward operator-e2e-v2
  wait_for_isolated_routing_status Configured
}

apply_production_forward() {
  local input_function_id="$1"
  local output_function_id="$2"
  local override_value="$3"
  local credentials_file="${RUN_DIR}/credentials.env"
  local input_function_yaml=""
  local output_function_yaml=""
  [[ -z "${input_function_id}" ]] || input_function_yaml="          functionId: ${input_function_id}"
  [[ -z "${output_function_id}" ]] || output_function_yaml="          functionId: ${output_function_id}"

  umask 077
  printf 'key=%s\nsecret=%s\nbucket-password=%s\n' \
    "${WHR_E2E_RELAY_KEY}" "${WHR_E2E_RELAY_SECRET}" "${BUCKET_AUTH_PASSWORD}" >"${credentials_file}"
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
  image: ${AGENT_IMAGE}
  websocketTransport: false
  resources:
    requests:
      cpu: 10m
      memory: 16Mi
    limits:
      cpu: 100m
      memory: 64Mi
  buckets:
    - name: ${BUCKET_NAME}
      description: ${BUCKET_DESCRIPTION}
      stream: true
      ephemeral: false
      largeWebhooks: true
      staticIP: false
      auth:
        type: basic
        username: e2e
        secretKeyRef:
          name: e2e-credentials
          key: bucket-password
      inputs:
        - name: e2e-input
          description: ${BUCKET_DESCRIPTION}
          responseFromOutput: e2e-output
          stripPathPrefix: false
          tlsVersion: "1.2"
          legacyTLS: false
${input_function_yaml}
      outputs:
        - name: e2e-output
          description: ${BUCKET_DESCRIPTION}
          destination: http://e2e-receiver:8080/hooks/base
          disabled: false
          internal: true
          lockPath: true
          timeout: 10
          retries: 2
          tlsVerification: true
          overrideHeaders:
            X-WHR-E2E-Override: ${override_value}
          durability:
            enabled: true
            schedule: long
            deadline: 720h
            handoffAfter: 15m
          throttle:
            enabled: false
${output_function_yaml}
        - name: e2e-replay-output
          description: ${BUCKET_DESCRIPTION}
          destination: http://e2e-receiver:8080/hooks/replay
          disabled: true
          internal: true
          lockPath: true
          timeout: 10
          replayMissing:
            enabled: true
            lookback: 30m
            limit: 250
EOF
  PRODUCTION_RESOURCES_STARTED=true
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
  if [[ "${PRODUCTION_MODE}" == "true" ]]; then
    apply_production_forward "" "" baseline
  else
    apply_isolated_forward operator-e2e-v1 true does-not-exist.invalid/webhookrelay-agent:broken
  fi

  for _ in $(seq 1 90); do
    if kubectl -n "${NAMESPACE}" get deployment/e2e-forward-whr-deployment >/dev/null 2>&1; then
      break
    fi
    sleep 2
  done
  local expected_agent_image="registry.k8s.io/pause:3.10"
  [[ "${PRODUCTION_MODE}" != "true" ]] || expected_agent_image="${AGENT_IMAGE}"
  if [[ "${PRODUCTION_MODE}" != "true" ]]; then
    wait_for_isolated_image_pull_failure
    wait_for_isolated_condition AgentReady False DeploymentUnavailable
    wait_for_isolated_condition Ready False AgentNotReady
    log "recovering isolated agent Deployment from an invalid image"
    apply_isolated_forward operator-e2e-v1 true "${expected_agent_image}"
    kubectl -n "${NAMESPACE}" rollout status deployment/e2e-forward-whr-deployment --timeout=180s
    wait_for_isolated_condition AgentReady True DeploymentAvailable
    wait_for_isolated_condition Ready True Ready
    kubectl -n "${NAMESPACE}" get webhookrelayforward/e2e-forward -o json | jq -e '
      .metadata.generation == .status.observedGeneration and
      .status.agentStatus == "Running" and .status.ready == true
    ' >/dev/null || fail "legacy readiness mirrors or observedGeneration are inconsistent"
  fi
  kubectl -n "${NAMESPACE}" get deployment/e2e-forward-whr-deployment -o json \
    >"${ARTIFACT_DIR}/reconciled-deployment.json"
  jq -e --arg image "${expected_agent_image}" '
    .metadata.ownerReferences[0].kind == "WebhookRelayForward" and
    .spec.template.spec.containers[0].image == $image and
    .spec.template.spec.containers[0].resources.requests.cpu == "10m" and
    .spec.template.spec.containers[0].resources.requests.memory == "16Mi" and
    .spec.template.spec.containers[0].resources.limits.cpu == "100m" and
    .spec.template.spec.containers[0].resources.limits.memory == "64Mi" and
    any(.spec.template.spec.containers[0].env[];
      .name == "WEBSOCKET_TRANSPORT" and .value == "false") and
    any(.spec.template.spec.containers[0].env[];
      .name == "KEY" and .valueFrom.secretKeyRef.name == "e2e-credentials") and
    any(.spec.template.spec.containers[0].env[];
      .name == "SECRET" and .valueFrom.secretKeyRef.name == "e2e-credentials")
  ' "${ARTIFACT_DIR}/reconciled-deployment.json" >/dev/null || fail "reconciled Deployment is invalid"

  if [[ "${PRODUCTION_MODE}" == "true" ]]; then
    exercise_production_reconcile
  else
    exercise_isolated_reconcile
  fi
}

exercise_production_reconcile() {
  local bucket_id input_id output_id replay_output_id public_endpoint
  for _ in $(seq 1 120); do
    kubectl -n "${NAMESPACE}" get webhookrelayforward/e2e-forward -o json \
      >"${ARTIFACT_DIR}/reconciled-forward.json"
    production_api "https://my.webhookrelay.com/v1/buckets" >"${RUN_DIR}/production-buckets.json"
    if jq -e --arg name "${BUCKET_NAME}" --arg description "${BUCKET_DESCRIPTION}" '
      any(.[]; .name == $name and .description == $description and
        .stream == true and .ephemeral == false and .large_webhooks == true and .static_ip == false and
        .auth.type == "basic" and .auth.username == "e2e" and ((.auth.password // "") | length > 0) and
        any(.inputs[]?; .name == "e2e-input" and .response_from_output != "" and
          .strip_path_prefix == false and .tls_version == "1.2" and .legacy_tls == false) and
        any(.outputs[]?; .name == "e2e-output" and .disabled == false and .internal == true and
          .lock_path == true and .timeout == 10 and .destination == "http://e2e-receiver:8080/hooks/base" and
          .retries == 2 and .tls_verification == true and
          .durability.enabled == true and .durability.schedule == "long" and
          .durability.deadline == 2592000000000000 and .durability.handoff_after == 900000000000 and
          .throttle.enabled == false and
          any(.headers | to_entries[]?; (.key | ascii_downcase) == "x-whr-e2e-override" and .value[0] == "baseline")) and
        any(.outputs[]?; .name == "e2e-replay-output" and .disabled == true and .internal == true and
          .replay_missing.enabled == true and .replay_missing.lookback == 1800000000000 and
          .replay_missing.limit == 250))
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
  replay_output_id="$(jq -r --arg name "${BUCKET_NAME}" \
    '.[] | select(.name == $name) | .outputs[] | select(.name == "e2e-replay-output") | .id' "${RUN_DIR}/production-buckets.json")"
  [[ -n "${bucket_id}" && -n "${input_id}" && -n "${output_id}" && -n "${replay_output_id}" ]] || fail "production resources did not converge"

  kubectl -n "${NAMESPACE}" rollout status deployment/e2e-forward-whr-deployment --timeout=180s
  for _ in $(seq 1 60); do
    kubectl -n "${NAMESPACE}" get webhookrelayforward/e2e-forward -o json \
      >"${ARTIFACT_DIR}/reconciled-forward.json"
    public_endpoint="$(jq -r '.status.publicEndpoints[0] // empty' "${ARTIFACT_DIR}/reconciled-forward.json")"
    [[ "${public_endpoint}" != "" ]] && break
    sleep 1
  done
  [[ "${public_endpoint}" == https://* ]] || fail "the CR did not publish a production input endpoint"

  assert_bucket_auth_required "${public_endpoint}"
  assert_live_delivery "${public_endpoint}" baseline baseline ""

  log "testing input function ID with live delivery"
  apply_production_forward "${FUNCTION_ID}" "" input-function
  wait_for_production_update "${bucket_id}" "${input_id}" "${output_id}" "${replay_output_id}" "${FUNCTION_ID}" "" input-function
  assert_live_delivery "${public_endpoint}" input-function input-function applied

  log "testing output function ID with live delivery and stable resource identities"
  apply_production_forward "" "${FUNCTION_ID}" output-function
  wait_for_production_update "${bucket_id}" "${input_id}" "${output_id}" "${replay_output_id}" "" "${FUNCTION_ID}" output-function
  assert_live_delivery "${public_endpoint}" output-function output-function applied
  log "production reconciliation and live delivery passed"
}

assert_bucket_auth_required() {
  local public_endpoint="$1"
  local response_status
  response_status="$(curl --show-error --silent --output /dev/null --write-out '%{http_code}' \
    --request POST "${public_endpoint}/unauthenticated")"
  [[ "${response_status}" == "401" ]] || fail "bucket authentication returned HTTP ${response_status}, expected 401"
  log "bucket authentication rejected an unauthenticated webhook"
}

wait_for_production_update() {
  local bucket_id="$1"
  local input_id="$2"
  local output_id="$3"
  local replay_output_id="$4"
  local input_function_id="$5"
  local output_function_id="$6"
  local override_value="$7"
  for _ in $(seq 1 90); do
    production_api "https://my.webhookrelay.com/v1/buckets" >"${RUN_DIR}/production-buckets.json"
    if jq -e --arg name "${BUCKET_NAME}" --arg bucket "${bucket_id}" --arg input "${input_id}" \
      --arg output "${output_id}" --arg replay_output "${replay_output_id}" --arg input_function "${input_function_id}" \
      --arg output_function "${output_function_id}" --arg override "${override_value}" '
      any(.[]; .name == $name and .id == $bucket and
        any(.inputs[]?; .id == $input and .function_id == $input_function) and
        any(.outputs[]?; .id == $output and .function_id == $output_function and
          .retries == 2 and .tls_verification == true and
          .durability.enabled == true and .throttle.enabled == false and
          any(.headers | to_entries[]?; (.key | ascii_downcase) == "x-whr-e2e-override" and .value[0] == $override)) and
        any(.outputs[]?; .id == $replay_output and .replay_missing.enabled == true and
          .replay_missing.limit == 250))
    ' "${RUN_DIR}/production-buckets.json" >/dev/null; then
      return
    fi
    sleep 2
  done
  fail "production resources did not update idempotently"
}

assert_live_delivery() {
  local public_endpoint="$1"
  local case_name="$2"
  local expected_override="$3"
  local expected_function_header="$4"
  local nonce="${RUN_ID}-${case_name}"
  local response_body_file="${ARTIFACT_DIR}/${case_name}-response-body.txt"
  local response_headers_file="${ARTIFACT_DIR}/${case_name}-response-headers.txt"
  local response_status
  local receiver_record="${ARTIFACT_DIR}/${case_name}-receiver.json"

  response_status="$(curl --show-error --silent \
    --user "e2e:${BUCKET_AUTH_PASSWORD}" \
    --dump-header "${response_headers_file}" --output "${response_body_file}" \
    --write-out '%{http_code}' --request POST \
    --header 'Content-Type: application/json' \
    --header "X-WHR-E2E-Nonce: ${nonce}" \
    --data "{\"nonce\":\"${nonce}\",\"case\":\"${case_name}\"}" \
    "${public_endpoint}/caller-path?source=production-e2e")"
  [[ "${response_status}" == "201" ]] || fail "${case_name}: caller received HTTP ${response_status}, expected 201"
  grep -qi '^X-WHR-E2E-Receiver: observed' "${response_headers_file}" || \
    fail "${case_name}: receiver response header was not propagated"
  [[ "$(<"${response_body_file}")" == "receiver-response:${nonce}" ]] || \
    fail "${case_name}: receiver response body was not propagated"

  for _ in $(seq 1 60); do
    if kubectl -n "${NAMESPACE}" exec deployment/e2e-receiver -- \
      wget -qO- "http://127.0.0.1:8080/requests/${nonce}" >"${receiver_record}" 2>/dev/null; then
      break
    fi
    sleep 1
  done
  jq -e --arg nonce "${nonce}" --arg override "${expected_override}" \
    --arg function_header "${expected_function_header}" '
      .method == "POST" and .path == "/hooks/base" and .rawQuery == "source=production-e2e" and
      .nonce == $nonce and .overrideHeader == $override and .functionHeader == $function_header and
      ((.body | fromjson).nonce == $nonce) and
      (if $function_header == "" then
        ((.body | fromjson) | has("functionApplied") | not)
      else
        ((.body | fromjson).functionApplied == "webhookrelay-operator-live-delivery")
      end)
    ' "${receiver_record}" >/dev/null || fail "${case_name}: receiver did not observe the expected request"
  log "${case_name}: live webhook delivery passed"
}

wait_for_object_deletion() {
  local resource="$1"
  for _ in $(seq 1 60); do
    if ! kubectl -n "${NAMESPACE}" get "${resource}" >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done
  fail "${resource} was not deleted"
}

scale_operator_to_zero() {
  kubectl -n "${NAMESPACE}" scale deployment/webhookrelay-operator --replicas=0 >/dev/null
  for _ in $(seq 1 60); do
    if [[ "$(kubectl -n "${NAMESPACE}" get pods \
      -l app.kubernetes.io/instance=webhookrelay-operator -o json | jq '.items | length')" == "0" ]]; then
      return
    fi
    sleep 1
  done
  fail "operator pods did not terminate before the lifecycle transition"
}

assert_helm_revision() {
  local expected_revision="$1"
  local actual_revision
  actual_revision="$(helm history webhookrelay-operator --namespace "${NAMESPACE}" -o json | \
    jq -r 'map(select(.status == "deployed")) | last | .revision')"
  [[ "${actual_revision}" == "${expected_revision}" ]] || \
    fail "Helm revision is ${actual_revision}, expected ${expected_revision}"
}

apply_legacy_lifecycle_forward() {
  kubectl apply -f - <<EOF
apiVersion: forward.webhookrelay.com/v1
kind: WebhookRelayForward
metadata:
  name: e2e-forward
  namespace: ${NAMESPACE}
spec:
  secretRefName: e2e-credentials
  image: registry.k8s.io/pause:3.10
  buckets:
    - name: ${BUCKET_NAME}-lifecycle
      inputs:
        - name: legacy-input
      outputs:
        - name: legacy-output
          destination: https://example.invalid/operator-e2e-lifecycle
          disabled: true
          internal: false
          function_id: 00000000-0000-0000-0000-000000000000
EOF
}

remove_crd_from_disposable_cluster() {
  [[ "${PRODUCTION_MODE}" == "false" ]] || fail "refusing to remove the CRD in production mode"
  [[ "${K3S_DATA_DIR}" == "/var/lib/webhookrelay-operator-e2e-${RUN_ID}" ]] || \
    fail "refusing to remove the CRD from an unowned k3s data directory"
  grep -Fxq "data-dir: ${K3S_DATA_DIR}" "${K3S_CONFIG}" || \
    fail "refusing to remove the CRD without the task-owned k3s configuration"
  kubectl delete crd webhookrelayforwards.forward.webhookrelay.com --wait=true --timeout=60s
}

exercise_helm_lifecycle() {
  local legacy_uid old_image fake_state_before fake_state_after
  [[ "${PRODUCTION_MODE}" == "false" ]] || return 0

  log "testing custom-resource deletion and owned Deployment garbage collection"
  kubectl -n "${NAMESPACE}" delete webhookrelayforward/e2e-forward --wait=true --timeout=60s
  wait_for_object_deletion deployment/e2e-forward-whr-deployment

  log "testing Helm uninstall with CRD retention"
  helm uninstall webhookrelay-operator --namespace "${NAMESPACE}" --wait --timeout 120s
  kubectl get crd webhookrelayforwards.forward.webhookrelay.com >/dev/null
  kubectl -n "${NAMESPACE}" get lease/webhookrelay-operator-lock >/dev/null
  for resource in \
    deployment/webhookrelay-operator \
    serviceaccount/webhookrelay-operator \
    role/webhookrelay-operator-operator \
    rolebinding/webhookrelay-operator-operator; do
    if kubectl -n "${NAMESPACE}" get "${resource}" >/dev/null 2>&1; then
      fail "Helm uninstall retained chart-owned ${resource}"
    fi
  done

  log "removing the retained candidate CRD before the published-chart install"
  remove_crd_from_disposable_cluster

  log "downloading checksum-pinned published chart 0.4.1"
  curl --fail --location --show-error --silent --output "${OLD_CHART}" \
    https://charts.webhookrelay.com/webhookrelay-operator-0.4.1.tgz
  printf '%s  %s\n' \
    '7de5d0afa11405d603c41b81bab1b33f93e0cf6f9ddb3fc7ae5e9a1fd85a4907' \
    "${OLD_CHART}" | sha256sum --check --strict -

  helm install webhookrelay-operator "${OLD_CHART}" --namespace "${NAMESPACE}" \
    --wait --timeout 180s --set-string httpsProxy=http://127.0.0.1:1
  assert_helm_revision 1
  kubectl get crd webhookrelayforwards.forward.webhookrelay.com -o json | jq -e '
    .spec.versions[] | select(.name == "v1") |
    .schema.openAPIV3Schema.properties.spec.properties.buckets.items.properties.outputs.items.properties as $output |
    ($output.function_id.type == "string") and ($output | has("functionId") | not) and
    (.schema.openAPIV3Schema.properties.status.properties | has("conditions") | not)
  ' >/dev/null || fail "published chart did not install its expected legacy CRD schema"
  old_image="$(kubectl -n "${NAMESPACE}" get deployment/webhookrelay-operator \
    -o jsonpath='{.spec.template.spec.containers[0].image}')"
  [[ "${old_image}" == "webhookrelay/webhookrelay-operator:0.6.0" ]] || \
    fail "published chart uses unexpected operator image ${old_image}"
  scale_operator_to_zero

  apply_legacy_lifecycle_forward
  legacy_uid="$(kubectl -n "${NAMESPACE}" get webhookrelayforward/e2e-forward \
    -o jsonpath='{.metadata.uid}')"
  [[ -n "${legacy_uid}" ]] || fail "legacy lifecycle custom resource has no UID"

  log "applying the candidate CRD explicitly before Helm upgrade"
  kubectl apply -f "${REPO_ROOT}/charts/webhookrelay-operator/crds/crd.yaml"
  kubectl get crd webhookrelayforwards.forward.webhookrelay.com -o json | jq -e '
    .spec.versions[] | select(.name == "v1") |
    .schema.openAPIV3Schema.properties.spec.properties.buckets.items.properties.outputs.items.properties as $output |
    ($output.functionId.type == "string") and ($output.function_id.type == "string") and
    (.schema.openAPIV3Schema.properties.status.properties.conditions["x-kubernetes-list-type"] == "map")
  ' >/dev/null || fail "candidate CRD schema is incomplete"

  log "upgrading the published chart to the packaged candidate"
  helm upgrade webhookrelay-operator "${CANDIDATE_CHART}" --namespace "${NAMESPACE}" \
    --wait --timeout 180s \
    --set-string image.repository=webhookrelay-operator-e2e \
    --set-string "image.tag=${RUN_ID}" \
    --set image.pullPolicy=IfNotPresent \
    --set-string apiEndpointURL=http://relay-api:8080/v1
  assert_helm_revision 2
  kubectl -n "${NAMESPACE}" rollout status deployment/webhookrelay-operator --timeout=120s
  [[ "$(kubectl -n "${NAMESPACE}" get webhookrelayforward/e2e-forward \
    -o jsonpath='{.metadata.uid}')" == "${legacy_uid}" ]] || fail "Helm upgrade replaced the custom resource"
  [[ "$(kubectl -n "${NAMESPACE}" get webhookrelayforward/e2e-forward \
    -o jsonpath='{.spec.buckets[0].outputs[0].function_id}')" == \
    "00000000-0000-0000-0000-000000000000" ]] || fail "legacy function_id was not retained"
  wait_for_isolated_routing_status Configured
  kubectl -n "${NAMESPACE}" rollout status deployment/e2e-forward-whr-deployment --timeout=120s
  fake_api_state | jq -e --arg name "${BUCKET_NAME}-lifecycle" '
    any(.buckets[]; .name == $name and
      any(.outputs[]; .name == "legacy-output" and
        .function_id == "00000000-0000-0000-0000-000000000000"))
  ' >/dev/null || fail "candidate reconciliation did not map legacy function_id"
  kubectl -n "${NAMESPACE}" get lease/webhookrelay-operator-lock >/dev/null
  kubectl -n "${NAMESPACE}" auth can-i create leases.coordination.k8s.io \
    --as="system:serviceaccount:${NAMESPACE}:webhookrelay-operator" | grep -Fxq yes || \
    fail "operator service account cannot create Leases"
  kubectl -n "${NAMESPACE}" get deployment/webhookrelay-operator -o json | jq -e --arg image "${IMAGE}" '
    .spec.strategy.type == "Recreate" and
    .spec.template.spec.containers[0].image == $image and
    .spec.template.spec.containers[0].livenessProbe.httpGet.path == "/healthz" and
    .spec.template.spec.containers[0].readinessProbe.httpGet.path == "/readyz"
  ' >/dev/null || fail "candidate operator Deployment lifecycle wiring is invalid"
  kubectl -n "${NAMESPACE}" get pods -l app.kubernetes.io/instance=webhookrelay-operator -o json | \
    jq -e --arg image "${IMAGE}" \
      '(.items | length) == 1 and all(.items[]; .spec.containers[0].image == $image)' >/dev/null || \
    fail "old and candidate operator pods overlapped after upgrade"

  scale_operator_to_zero
  fake_state_before="$(fake_api_state | jq -c '.mutations')"
  log "rolling back application resources with the candidate operator stopped"
  helm rollback webhookrelay-operator 1 --namespace "${NAMESPACE}" --wait --timeout 180s
  assert_helm_revision 3
  kubectl -n "${NAMESPACE}" rollout status deployment/webhookrelay-operator --timeout=120s
  [[ "$(kubectl -n "${NAMESPACE}" get deployment/webhookrelay-operator \
    -o jsonpath='{.spec.template.spec.containers[0].image}')" == "${old_image}" ]] || \
    fail "Helm rollback did not restore the published operator image"
  kubectl -n "${NAMESPACE}" get pods -l app.kubernetes.io/instance=webhookrelay-operator -o json | \
    jq -e --arg image "${old_image}" \
      '(.items | length) == 1 and all(.items[]; .spec.containers[0].image == $image)' >/dev/null || \
    fail "candidate and old operator pods overlapped after rollback"
  kubectl -n "${NAMESPACE}" get deployment/webhookrelay-operator -o json | jq -e '
    any(.spec.template.spec.containers[0].env[]?;
      .name == "CLIENT_HTTPS_PROXY" and .value == "http://127.0.0.1:1")
  ' >/dev/null || fail "rollback did not restore the isolated unreachable API proxy"
  wait_for_isolated_routing_status Failed
  scale_operator_to_zero
  fake_state_after="$(fake_api_state | jq -c '.mutations')"
  [[ "${fake_state_before}" == "${fake_state_after}" ]] || \
    fail "the rolled-back controller duplicated fake remote resources"
  [[ "$(kubectl -n "${NAMESPACE}" get webhookrelayforward/e2e-forward \
    -o jsonpath='{.metadata.uid}')" == "${legacy_uid}" ]] || fail "rollback replaced the custom resource"

  kubectl -n "${NAMESPACE}" delete webhookrelayforward/e2e-forward --wait=true --timeout=60s
  wait_for_object_deletion deployment/e2e-forward-whr-deployment
  helm uninstall webhookrelay-operator --namespace "${NAMESPACE}" --wait --timeout 120s
  kubectl -n "${NAMESPACE}" get lease/webhookrelay-operator-lock >/dev/null || \
    fail "expected controller-created Lease retention was not observed"
  kubectl -n "${NAMESPACE}" get configmap/webhookrelay-operator-lock >/dev/null || \
    fail "expected legacy controller-created ConfigMap retention was not observed"
  kubectl -n "${NAMESPACE}" delete lease/webhookrelay-operator-lock \
    configmap/webhookrelay-operator-lock --ignore-not-found >/dev/null
  kubectl get crd webhookrelayforwards.forward.webhookrelay.com >/dev/null
  for resource in \
    deployment/webhookrelay-operator \
    serviceaccount/webhookrelay-operator \
    role/webhookrelay-operator-operator \
    rolebinding/webhookrelay-operator-operator \
    lease/webhookrelay-operator-lock \
    configmap/webhookrelay-operator-lock; do
    if kubectl -n "${NAMESPACE}" get "${resource}" >/dev/null 2>&1; then
      fail "lifecycle cleanup retained ${resource}"
    fi
  done

  log "testing separately guarded CRD removal in disposable k3s"
  remove_crd_from_disposable_cluster
  log "packaged Helm install, upgrade, rollback, and uninstall lifecycle passed"
}

cleanup_production_resources() {
  local bucket_count bucket_id
  [[ "${PRODUCTION_MODE}" == "true" ]] || return 0
  [[ "${PRODUCTION_RESOURCES_STARTED}" == "true" ]] || return 0
  [[ -s "${RUN_DIR}/production-curl.conf" ]] || return 0
  # Stop reconciliation before deleting remote state. Otherwise the controller's
  # five-second requeue can recreate the bucket between deletion and k3s teardown.
  kubectl -n "${NAMESPACE}" scale deployment/webhookrelay-operator --replicas=0 >/dev/null || return 1
  kubectl -n "${NAMESPACE}" wait --for=delete pod \
    --selector app.kubernetes.io/name=webhookrelay-operator --timeout=60s >/dev/null || return 1
  kubectl -n "${NAMESPACE}" delete webhookrelayforward/e2e-forward \
    --ignore-not-found --wait=true --timeout=60s >/dev/null || return 1
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
  production_api "https://my.webhookrelay.com/v1/buckets" >"${RUN_DIR}/cleanup-buckets.json" || return 1
  jq -e --arg name "${BUCKET_NAME}" --arg description "${BUCKET_DESCRIPTION}" '
    all(.[]; .name != $name or .description != $description)
  ' "${RUN_DIR}/cleanup-buckets.json" >/dev/null || return 1
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
  kubectl -n "${NAMESPACE}" logs deployment/relay-api --all-containers \
    >"${ARTIFACT_DIR}/fake-api.log" 2>&1 || true
  kubectl -n "${NAMESPACE}" logs deployment/e2e-forward-whr-deployment --all-containers \
    >"${ARTIFACT_DIR}/relay-agent.log" 2>&1 || true
  kubectl -n "${NAMESPACE}" logs deployment/e2e-receiver --all-containers \
    >"${ARTIFACT_DIR}/receiver.log" 2>&1 || true
}

cleanup() {
  local exit_status=$?
  local pid
  TEST_STATUS=${exit_status}
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
package_candidate_chart
start_k3s
build_and_import_images
install_fake_api
install_operator
install_receiver
exercise_reconcile
exercise_helm_lifecycle
