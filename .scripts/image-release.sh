#!/usr/bin/env bash

set -Eeuo pipefail

readonly OPERATOR_VERSION="${OPERATOR_VERSION:-0.8.0}"
readonly CHART_VERSION="${CHART_VERSION:-0.7.0}"
readonly AGENT_VERSION="${AGENT_VERSION:-1.37.0}"
readonly OPERATOR_IMAGE="${OPERATOR_IMAGE:-webhookrelay/webhookrelay-operator:${OPERATOR_VERSION}}"
readonly AGENT_IMAGE="webhookrelay/webhookrelayd:${AGENT_VERSION}"
readonly AGENT_UBI_IMAGE="webhookrelay/webhookrelayd-ubi8:${AGENT_VERSION}"
readonly AGENT_DIGEST="sha256:4d73b9e6096d4d653c3b46733aa4dbf45dfde1ed1e5f142da8da5a396eb9ffab"
readonly AGENT_UBI_DIGEST="sha256:79d86f8dbc71831a16a2e1cd826b5a90a4c5a8a165fc53e62f43243b8274fb22"
readonly PREVIOUS_OPERATOR_DIGEST="sha256:63e40035e33c9485c4909868cc5f2372cf06c97eee26b342f2fa8f7562f5e520"
readonly REVISION="${GITHUB_SHA:-$(git rev-parse HEAD)}"
readonly SOURCE="https://github.com/webhookrelay/webhookrelay-operator"

fail() {
  printf 'image-release: %s\n' "$*" >&2
  exit 1
}

semver_greater() {
  awk -v left="$1" -v right="$2" 'BEGIN {
    split(left, l, "."); split(right, r, ".")
    for (i = 1; i <= 3; i++) {
      if ((l[i] + 0) > (r[i] + 0)) exit 0
      if ((l[i] + 0) < (r[i] + 0)) exit 1
    }
    exit 1
  }'
}

manifest_json() {
  docker buildx imagetools inspect "$1" --format '{{json .Manifest}}'
}

verify_image() {
  local image="$1" expected_digest="${2:-}" expected_revision="${3:-}"
  local digest label manifest platform
  manifest="$(manifest_json "${image}")" || fail "cannot inspect ${image}"
  jq -e '
    (.manifests | length) == 2 and
    ([.manifests[] | select(.platform.os == "linux") |
      (.platform.os + "/" + .platform.architecture)] | sort) ==
      ["linux/amd64", "linux/arm64"]
  ' <<<"${manifest}" >/dev/null || fail "${image} does not have exactly linux/amd64 and linux/arm64"
  digest="$(jq -r '.digest' <<<"${manifest}")"
  [[ -z "${expected_digest}" || "${digest}" == "${expected_digest}" ]] || \
    fail "${image} digest ${digest} does not equal ${expected_digest}"
  if [[ -n "${expected_revision}" ]]; then
    for platform in linux/amd64 linux/arm64; do
      docker pull --platform "${platform}" "${image}" >/dev/null
      label="$(docker image inspect --format \
        '{{ index .Config.Labels "org.opencontainers.image.revision" }}' "${image}")"
      [[ "${label}" == "${expected_revision}" ]] || \
        fail "${image} ${platform} revision ${label:-unset} does not equal ${expected_revision}"
      label="$(docker image inspect --format \
        '{{ index .Config.Labels "org.opencontainers.image.version" }}' "${image}")"
      [[ "${label}" == "${OPERATOR_VERSION}" ]] || \
        fail "${image} ${platform} version label ${label:-unset} does not equal ${OPERATOR_VERSION}"
      label="$(docker image inspect --format \
        '{{ index .Config.Labels "org.opencontainers.image.source" }}' "${image}")"
      [[ "${label}" == "${SOURCE}" ]] || fail "${image} ${platform} source label is invalid"
    done
  fi
  printf '%s\n' "${digest}"
}

validate_source() {
  local app_version chart_version default_agent default_tag tag
  tag="${GITHUB_REF_NAME:-${OPERATOR_VERSION}}"
  [[ "${GITHUB_REF_TYPE:-tag}" == tag ]] || fail "release ref is not a tag"
  [[ "${GITHUB_REF:-refs/tags/${tag}}" == "refs/tags/${OPERATOR_VERSION}" ]] || \
    fail "release ref does not name ${OPERATOR_VERSION}"
  [[ "${tag}" == "${OPERATOR_VERSION}" ]] || fail "tag ${tag} does not equal ${OPERATOR_VERSION}"
  git merge-base --is-ancestor "${REVISION}" origin/master || \
    fail "release revision ${REVISION} is not reachable from origin/master"
  chart_version="$(awk -F ': *' '$1 == "version" {print $2; exit}' charts/webhookrelay-operator/Chart.yaml)"
  app_version="$(awk -F ': *' '$1 == "appVersion" {gsub(/"/, "", $2); print $2; exit}' charts/webhookrelay-operator/Chart.yaml)"
  default_tag="$(awk '/^image:/ {found=1; next} found && /^  tag:/ {gsub(/[" ]/, "", $2); print $2; exit}' charts/webhookrelay-operator/values.yaml)"
  default_agent="$(awk -F '"' '/Image string `default:/ {print $2; exit}' pkg/config/config.go)"
  [[ "${chart_version}" == "${CHART_VERSION}" ]] || fail "chart version mismatch"
  [[ "${app_version}" == "${OPERATOR_VERSION}" ]] || fail "chart appVersion mismatch"
  [[ "${default_tag}" == "${OPERATOR_VERSION}" ]] || fail "operator image tag mismatch"
  [[ "${default_agent}" == "${AGENT_UBI_IMAGE}" ]] || fail "relay-agent default mismatch"
}

build_operator() {
  local digest manifest
  if manifest_json "${OPERATOR_IMAGE}" >/dev/null 2>&1; then
    digest="$(verify_image "${OPERATOR_IMAGE}" "" "${REVISION}")"
    printf 'image-release: reusing verified %s@%s\n' "${OPERATOR_IMAGE}" "${digest}" >&2
  else
    KO_DOCKER_REPO="${OPERATOR_IMAGE%:*}" ko build --bare \
      --platform=linux/amd64,linux/arm64 --tags="${OPERATOR_IMAGE##*:}" --sbom=none \
      --image-label="org.opencontainers.image.revision=${REVISION}" \
      --image-label="org.opencontainers.image.version=${OPERATOR_VERSION}" \
      --image-label="org.opencontainers.image.source=${SOURCE}" ./cmd/manager >/dev/null
    digest="$(verify_image "${OPERATOR_IMAGE}" "" "${REVISION}")"
  fi
  manifest="$(manifest_json "${OPERATOR_IMAGE}")"
  jq -n --arg image "${OPERATOR_IMAGE}" --arg digest "${digest}" \
    --arg revision "${REVISION}" --argjson manifest "${manifest}" \
    '{image:$image,digest:$digest,revision:$revision,
      platforms:[$manifest.manifests[] | {os:.platform.os,architecture:.platform.architecture,digest:.digest}]}'
}

verify_and_smoke() {
  local operator_digest="$1" image platform operator_manifest agent_manifest agent_ubi_manifest
  verify_image "${OPERATOR_IMAGE}" "${operator_digest}" "${REVISION}" >/dev/null
  verify_image "${AGENT_IMAGE}" "${AGENT_DIGEST}" >/dev/null
  verify_image "${AGENT_UBI_IMAGE}" "${AGENT_UBI_DIGEST}" >/dev/null
  for platform in linux/amd64 linux/arm64; do
    # Docker's classic image store cannot keep two platform-specific images for
    # the same manifest-list digest. Clear the previous platform before pulling
    # the next one, otherwise the arm64 pull fails with "cannot overwrite digest".
    docker image rm --force "${OPERATOR_IMAGE}@${operator_digest}" >/dev/null 2>&1 || true
    docker image rm --force "${OPERATOR_IMAGE}" >/dev/null 2>&1 || true
    docker pull --platform "${platform}" "${OPERATOR_IMAGE}@${operator_digest}" >/dev/null
    docker run --rm --platform "${platform}" "${OPERATOR_IMAGE}@${operator_digest}" --help >/dev/null
    for image in "${AGENT_IMAGE}@${AGENT_DIGEST}" "${AGENT_UBI_IMAGE}@${AGENT_UBI_DIGEST}"; do
      docker image rm --force "${image}" >/dev/null 2>&1 || true
      docker image rm --force "${image%@*}" >/dev/null 2>&1 || true
      docker pull --platform "${platform}" "${image}" >/dev/null
      docker run --rm --platform "${platform}" "${image}" --version >/dev/null
    done
  done
  operator_manifest="$(manifest_json "${OPERATOR_IMAGE}@${operator_digest}")"
  agent_manifest="$(manifest_json "${AGENT_IMAGE}@${AGENT_DIGEST}")"
  agent_ubi_manifest="$(manifest_json "${AGENT_UBI_IMAGE}@${AGENT_UBI_DIGEST}")"
  jq -n --arg revision "${REVISION}" \
    --arg operator_image "${OPERATOR_IMAGE}" --argjson operator "${operator_manifest}" \
    --arg agent_image "${AGENT_IMAGE}" --argjson agent "${agent_manifest}" \
    --arg agent_ubi_image "${AGENT_UBI_IMAGE}" --argjson agent_ubi "${agent_ubi_manifest}" '
    def record($name; $manifest): {
      image:$name, digest:$manifest.digest,
      platforms:[$manifest.manifests[] | {os:.platform.os,architecture:.platform.architecture,digest:.digest}]
    };
    {revision:$revision, operator:record($operator_image;$operator),
     agent:record($agent_image;$agent), agentUbi:record($agent_ubi_image;$agent_ubi)}'
}

promote_tag() {
  local expected_digest="$1" target_tag="$2" target_image target_digest=""
  verify_image "${OPERATOR_IMAGE}" "${expected_digest}" "${REVISION}" >/dev/null
  target_image="${OPERATOR_IMAGE%:*}:${target_tag}"
  if target_digest="$(manifest_json "${target_image}" 2>/dev/null | jq -r '.digest')"; then
    [[ "${target_digest}" == "${expected_digest}" ]] || \
      fail "refusing to replace ${target_image} digest ${target_digest}"
    printf 'image-release: %s already points to %s\n' "${target_tag}" "${expected_digest}" >&2
    return
  fi
  docker buildx imagetools create --tag "${target_image}" \
    "${OPERATOR_IMAGE}@${expected_digest}" >/dev/null
  target_digest="$(manifest_json "${target_image}" | jq -r '.digest')"
  [[ "${target_digest}" == "${expected_digest}" ]] || fail "${target_tag} promotion digest mismatch"
}

promote_latest() {
  local expected_digest="$1" latest_digest="" latest_version=""
  verify_image "${OPERATOR_IMAGE}" "${expected_digest}" "${REVISION}" >/dev/null
  if latest_digest="$(manifest_json "${OPERATOR_IMAGE%:*}:latest" 2>/dev/null | jq -r '.digest')"; then
    [[ "${latest_digest}" != "${expected_digest}" ]] || {
      printf 'image-release: latest already points to %s\n' "${expected_digest}" >&2
      return
    }
    if [[ "${latest_digest}" != "${PREVIOUS_OPERATOR_DIGEST}" ]]; then
      docker pull --platform linux/amd64 "${OPERATOR_IMAGE%:*}:latest" >/dev/null
      latest_version="$(docker image inspect --format \
        '{{ index .Config.Labels "org.opencontainers.image.version" }}' \
        "${OPERATOR_IMAGE%:*}:latest")"
      if [[ "${latest_version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] && \
        semver_greater "${latest_version}" "${OPERATOR_VERSION}"; then
        printf 'image-release: leaving newer latest version %s unchanged\n' "${latest_version}" >&2
        return
      fi
      fail "refusing to replace unexpected latest digest ${latest_digest}"
    fi
  fi
  docker buildx imagetools create --tag "${OPERATOR_IMAGE%:*}:latest" \
    "${OPERATOR_IMAGE}@${expected_digest}" >/dev/null
  latest_digest="$(manifest_json "${OPERATOR_IMAGE%:*}:latest" | jq -r '.digest')"
  [[ "${latest_digest}" == "${expected_digest}" ]] || fail "latest promotion digest mismatch"
}

case "${1:-}" in
  validate-source) validate_source ;;
  build) build_operator ;;
  verify-smoke) verify_and_smoke "${2:?operator digest required}" ;;
  promote-version) promote_tag "${2:?operator digest required}" "${OPERATOR_VERSION}" ;;
  promote-latest) promote_latest "${2:?operator digest required}" ;;
  *) fail "usage: $0 validate-source|build|verify-smoke DIGEST|promote-version DIGEST|promote-latest DIGEST" ;;
esac
