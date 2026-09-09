#!/usr/bin/env bash

set -Eeuo pipefail

readonly REPO_ROOT="$(git rev-parse --show-toplevel)"
readonly CHART_DIR="${REPO_ROOT}/charts/webhookrelay-operator"
readonly OPERATOR_VERSION="${OPERATOR_VERSION:-0.8.0}"
readonly CHART_VERSION="${CHART_VERSION:-0.7.0}"
readonly AGENT_IMAGE="${AGENT_IMAGE:-webhookrelay/webhookrelayd-ubi8:1.37.0}"
readonly OUTPUT_DIR="${OUTPUT_DIR:-${REPO_ROOT}/dist/chart}"
readonly HELM_BIN="${HELM_BIN:-helm}"
readonly REPOSITORY_URL="${REPOSITORY_URL:-https://charts.webhookrelay.com}"
readonly CHART_BUCKET="${CHART_BUCKET:-charts.webhookrelay.com}"
readonly ARTIFACT_NAME="webhookrelay-operator-${CHART_VERSION}.tgz"
readonly ARTIFACT_PATH="${OUTPUT_DIR}/${ARTIFACT_NAME}"
readonly CHECKSUM_PATH="${ARTIFACT_PATH}.sha256"
readonly METADATA_PATH="${OUTPUT_DIR}/release-metadata.json"
readonly VALIDATED_ARTIFACT="${VALIDATED_ARTIFACT:-}"
readonly INDEX_CACHE_CONTROL="no-cache,max-age=0,must-revalidate"

fail() {
  printf 'chart-release: %s\n' "$*" >&2
  exit 1
}

yaml_value() {
  local file="$1"
  local key="$2"
  awk -F ': *' -v key="${key}" '$1 == key {gsub(/"/, "", $2); print $2; exit}' "${file}"
}

validate_versions() {
  local chart_app_version chart_version default_image default_image_tag
  chart_version="$(yaml_value "${CHART_DIR}/Chart.yaml" version)"
  chart_app_version="$(yaml_value "${CHART_DIR}/Chart.yaml" appVersion)"
  default_image_tag="$(awk '
    /^image:/ {in_image=1; next}
    in_image && /^  tag:/ {gsub(/[" ]/, "", $2); print $2; exit}
  ' "${CHART_DIR}/values.yaml")"
  default_image="$(awk -F '"' '/Image string `default:/ {print $2; exit}' \
    "${REPO_ROOT}/pkg/config/config.go")"
  [[ "${chart_version}" == "${CHART_VERSION}" ]] || \
    fail "Chart.version ${chart_version} does not equal ${CHART_VERSION}"
  [[ "${chart_app_version}" == "${OPERATOR_VERSION}" ]] || \
    fail "Chart.appVersion ${chart_app_version} does not equal ${OPERATOR_VERSION}"
  [[ "${default_image_tag}" == "${OPERATOR_VERSION}" ]] || \
    fail "chart image tag ${default_image_tag} does not equal ${OPERATOR_VERSION}"
  [[ "${default_image}" == "${AGENT_IMAGE}" ]] || \
    fail "operator agent image ${default_image} does not equal ${AGENT_IMAGE}"
}

package_chart() {
  local digest reference_archive reference_dir second_archive
  mkdir -p "${OUTPUT_DIR}"
  validate_versions
  "${HELM_BIN}" lint "${CHART_DIR}"
  go run "${REPO_ROOT}/.scripts/chart-artifact.go" package "${CHART_DIR}" "${ARTIFACT_PATH}"
  second_archive="${OUTPUT_DIR}/.${ARTIFACT_NAME}.reproducibility-check"
  go run "${REPO_ROOT}/.scripts/chart-artifact.go" package "${CHART_DIR}" "${second_archive}"
  cmp "${ARTIFACT_PATH}" "${second_archive}" || fail "chart packaging is not reproducible"
  rm -f "${second_archive}"
  reference_dir="$(mktemp -d)"
  "${HELM_BIN}" package "${CHART_DIR}" --destination "${reference_dir}" >/dev/null
  reference_archive="${reference_dir}/${ARTIFACT_NAME}"
  diff -u <(tar -tzf "${reference_archive}" | sort) <(tar -tzf "${ARTIFACT_PATH}" | sort) || \
    fail "reproducible archive members differ from pinned Helm packaging"
  rm -rf -- "${reference_dir}"
  "${HELM_BIN}" lint "${ARTIFACT_PATH}"
  digest="$(sha256sum "${ARTIFACT_PATH}" | awk '{print $1}')"
  printf '%s  %s\n' "${digest}" "${ARTIFACT_NAME}" >"${CHECKSUM_PATH}"
  printf '{"operatorVersion":"%s","chartVersion":"%s","agentImage":"%s","sha256":"%s"}\n' \
    "${OPERATOR_VERSION}" "${CHART_VERSION}" "${AGENT_IMAGE}" "${digest}" >"${METADATA_PATH}"
}

gcs_artifact_status() {
  local error_file generation remote_artifact
  error_file="${OUTPUT_DIR}/gcs-artifact-error.txt"
  if generation="$(gcloud storage objects describe "gs://${CHART_BUCKET}/${ARTIFACT_NAME}" \
    --format='value(generation)' 2>"${error_file}")"; then
    remote_artifact="${OUTPUT_DIR}/gcs-${ARTIFACT_NAME}"
    gcloud storage cp "gs://${CHART_BUCKET}/${ARTIFACT_NAME}#${generation}" \
      "${remote_artifact}" >/dev/null
    cmp "${ARTIFACT_PATH}" "${remote_artifact}" || \
      fail "refusing to overwrite published ${ARTIFACT_NAME} with different bytes"
    printf 'identical\n'
    return
  fi
  if grep -Eqi 'not found|404' "${error_file}"; then
    printf 'missing\n'
    return
  fi
  fail "could not inspect gs://${CHART_BUCKET}/${ARTIFACT_NAME}: $(<"${error_file}")"
}

verify_public_repository() {
  local digest="$1"
  local http_status
  for _ in $(seq 1 30); do
    http_status="$(curl --location --show-error --silent \
      --output "${OUTPUT_DIR}/published-artifact.tgz" --write-out '%{http_code}' \
      "${REPOSITORY_URL}/${ARTIFACT_NAME}")"
    if [[ "${http_status}" == "200" ]] && \
      cmp "${ARTIFACT_PATH}" "${OUTPUT_DIR}/published-artifact.tgz" && \
      curl --fail --location --show-error --silent --header 'Cache-Control: no-cache' \
        --output "${OUTPUT_DIR}/published-index.yaml" "${REPOSITORY_URL}/index.yaml" && \
      go run "${REPO_ROOT}/.scripts/chart-artifact.go" verify-index \
        "${OUTPUT_DIR}/published-index.yaml" "${CHART_VERSION}" "${digest}" \
        "${REPOSITORY_URL}/${ARTIFACT_NAME}" >/dev/null 2>&1; then
      return
    fi
    sleep 2
  done
  fail "public chart artifact and index did not converge"
}

remote_artifact_status() {
  local http_status remote_artifact
  remote_artifact="${OUTPUT_DIR}/remote-${ARTIFACT_NAME}"
  http_status="$(curl --location --show-error --silent --output "${remote_artifact}" \
    --write-out '%{http_code}' "${REPOSITORY_URL}/${ARTIFACT_NAME}")"
  case "${http_status}" in
    200)
      cmp "${ARTIFACT_PATH}" "${remote_artifact}" || \
        fail "refusing to overwrite published ${ARTIFACT_NAME} with different bytes"
      printf 'identical\n'
      ;;
    404)
      printf 'missing\n'
      ;;
    *)
      fail "artifact lookup returned HTTP ${http_status}"
      ;;
  esac
}

verify_remote() {
  local digest status remote_index
  package_chart
  status="$(remote_artifact_status)"
  [[ "${status}" == "identical" ]] || {
    printf 'chart-release: %s is not published\n' "${ARTIFACT_NAME}"
    return 0
  }
  digest="$(awk '{print $1}' "${CHECKSUM_PATH}")"
  remote_index="${OUTPUT_DIR}/remote-index.yaml"
  curl --fail --location --show-error --silent --output "${remote_index}" \
    "${REPOSITORY_URL}/index.yaml"
  go run "${REPO_ROOT}/.scripts/chart-artifact.go" verify-index "${remote_index}" \
    "${CHART_VERSION}" "${digest}" "${REPOSITORY_URL}/${ARTIFACT_NAME}"
}

publish_chart() {
  local digest index_cache_control index_dir index_generation status
  command -v gcloud >/dev/null || fail "gcloud is required to publish"
  package_chart
  if [[ -n "${VALIDATED_ARTIFACT}" ]]; then
    cmp "${ARTIFACT_PATH}" "${VALIDATED_ARTIFACT}" || \
      fail "rebuilt chart does not match the validated artifact"
  fi
  digest="$(awk '{print $1}' "${CHECKSUM_PATH}")"
  status="$(gcs_artifact_status)"
  index_generation="$(gcloud storage objects describe "gs://${CHART_BUCKET}/index.yaml" \
    --format='value(generation)')"
  gcloud storage cp "gs://${CHART_BUCKET}/index.yaml#${index_generation}" \
    "${OUTPUT_DIR}/remote-index.yaml" >/dev/null
  if [[ "${status}" == "identical" ]] && \
    go run "${REPO_ROOT}/.scripts/chart-artifact.go" verify-index "${OUTPUT_DIR}/remote-index.yaml" \
      "${CHART_VERSION}" "${digest}" "${REPOSITORY_URL}/${ARTIFACT_NAME}" >/dev/null 2>&1; then
    verify_public_repository "${digest}"
    printf 'chart-release: authoritative and public artifacts already match\n'
    return 0
  fi
  if [[ "${status}" == "missing" ]]; then
    gcloud storage cp "${ARTIFACT_PATH}" "gs://${CHART_BUCKET}/${ARTIFACT_NAME}" \
      --if-generation-match=0 --content-type=application/gzip
  fi

  index_dir="${OUTPUT_DIR}/repository"
  mkdir -p "${index_dir}"
  cp "${ARTIFACT_PATH}" "${index_dir}/${ARTIFACT_NAME}"
  "${HELM_BIN}" repo index "${index_dir}" --url "${REPOSITORY_URL}" \
    --merge "${OUTPUT_DIR}/remote-index.yaml"
  go run "${REPO_ROOT}/.scripts/chart-artifact.go" verify-retained \
    "${OUTPUT_DIR}/remote-index.yaml" "${index_dir}/index.yaml"
  go run "${REPO_ROOT}/.scripts/chart-artifact.go" verify-index "${index_dir}/index.yaml" \
    "${CHART_VERSION}" "${digest}" "${REPOSITORY_URL}/${ARTIFACT_NAME}"
  gcloud storage cp "${index_dir}/index.yaml" "gs://${CHART_BUCKET}/index.yaml" \
    --if-generation-match="${index_generation}" --content-type=application/yaml \
    --cache-control="${INDEX_CACHE_CONTROL}"
  index_cache_control="$(gcloud storage objects describe "gs://${CHART_BUCKET}/index.yaml" \
    --format='value(cache_control)')"
  [[ "${index_cache_control}" == "${INDEX_CACHE_CONTROL}" ]] || \
    fail "published index cache policy is ${index_cache_control:-unset}, expected ${INDEX_CACHE_CONTROL}"

  verify_public_repository "${digest}"
}

case "${1:-}" in
  package) package_chart ;;
  verify-remote) verify_remote ;;
  publish) publish_chart ;;
  *) fail "usage: $0 package|verify-remote|publish" ;;
esac
