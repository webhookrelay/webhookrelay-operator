#!/usr/bin/env bash

set -Eeuo pipefail

[[ "${1:-}" == "storage" ]] || { printf 'unsupported fake gcloud command\n' >&2; exit 2; }

object_path() {
  local uri="${1#gs://}"
  uri="${uri#*/}"
  printf '%s\n' "${uri%%#*}"
}

generation_path() {
  printf '%s.generation\n' "${FAKE_GCS_ROOT}/$(object_path "$1")"
}

case "${2:-}" in
  objects)
    [[ "${3:-}" == "describe" ]] || exit 2
    object="${FAKE_GCS_ROOT}/$(object_path "$4")"
    [[ -f "${object}" ]] || { printf 'ERROR: object not found (404)\n' >&2; exit 1; }
    generation_file="$(generation_path "$4")"
    [[ -f "${generation_file}" ]] && generation="$(<"${generation_file}")" || generation=1
    printf '%s\n' "${generation}"
    ;;
  cp)
    source_path="$3"
    destination_path="$4"
    if [[ "${source_path}" == gs://* ]]; then
      cp "${FAKE_GCS_ROOT}/$(object_path "${source_path}")" "${destination_path}"
      exit 0
    fi
    object="${FAKE_GCS_ROOT}/$(object_path "${destination_path}")"
    generation_file="$(generation_path "${destination_path}")"
    [[ -f "${generation_file}" ]] && current_generation="$(<"${generation_file}")" || current_generation=0
    expected_generation=""
    for argument in "$@"; do
      [[ "${argument}" == --if-generation-match=* ]] && expected_generation="${argument#*=}"
    done
    if [[ "$(object_path "${destination_path}")" == "index.yaml" && \
      "${FAKE_GCS_BUMP_BEFORE_INDEX_WRITE:-false}" == "true" && \
      ! -f "${FAKE_GCS_ROOT}/.index-bumped" ]]; then
      current_generation=$((current_generation + 1))
      printf '%s\n' "${current_generation}" >"${generation_file}"
      : >"${FAKE_GCS_ROOT}/.index-bumped"
    fi
    [[ "${expected_generation}" == "${current_generation}" ]] || {
      printf 'ERROR: generation precondition failed\n' >&2
      exit 1
    }
    mkdir -p "$(dirname "${object}")"
    cp "${source_path}" "${object}"
    printf '%s\n' "$((current_generation + 1))" >"${generation_file}"
    ;;
  *)
    printf 'unsupported fake gcloud storage command\n' >&2
    exit 2
    ;;
esac
