#!/usr/bin/env bash

set -Eeuo pipefail

operator_manifest() {
  local digest="${1:-sha256:operator-index}"
  printf '{"digest":"%s","manifests":[{"digest":"sha256:operator-amd64","platform":{"os":"linux","architecture":"amd64"}},{"digest":"sha256:operator-arm64","platform":{"os":"linux","architecture":"arm64"}}]}\n' "${digest}"
}

case "$*" in
  "buildx imagetools inspect webhookrelay/webhookrelay-operator:build-test"*)
    [[ -f "${FAKE_REGISTRY}/staging" ]] || exit 1
    operator_manifest
    ;;
  "buildx imagetools inspect webhookrelay/webhookrelay-operator:0.8.0"*)
    [[ -f "${FAKE_REGISTRY}/version" ]] || exit 1
    operator_manifest "$(<"${FAKE_REGISTRY}/version")"
    ;;
  "buildx imagetools inspect webhookrelay/webhookrelay-operator:latest"*)
    [[ -f "${FAKE_REGISTRY}/latest" ]] || exit 1
    operator_manifest "$(<"${FAKE_REGISTRY}/latest")"
    ;;
  *"webhookrelay/webhookrelayd-ubi8:1.37.0"*"imagetools inspect"*|*"imagetools inspect webhookrelay/webhookrelayd-ubi8"*)
    sed 's/sha256:operator-index/sha256:79d86f8dbc71831a16a2e1cd826b5a90a4c5a8a165fc53e62f43243b8274fb22/' "${FAKE_REGISTRY}/manifest"
    ;;
  *"webhookrelay/webhookrelayd:1.37.0"*"imagetools inspect"*|*"imagetools inspect webhookrelay/webhookrelayd:"*)
    sed 's/sha256:operator-index/sha256:4d73b9e6096d4d653c3b46733aa4dbf45dfde1ed1e5f142da8da5a396eb9ffab/' "${FAKE_REGISTRY}/manifest"
    ;;
  "buildx imagetools inspect webhookrelay/webhookrelay-operator:build-test@sha256:operator-index"*)
    operator_manifest
    ;;
  "buildx imagetools create --tag webhookrelay/webhookrelay-operator:0.8.0"*)
    printf 'sha256:operator-index\n' >"${FAKE_REGISTRY}/version"
    printf 'create-version\n' >>"${FAKE_REGISTRY}/writes"
    ;;
  "buildx imagetools create --tag webhookrelay/webhookrelay-operator:latest"*)
    printf 'sha256:operator-index\n' >"${FAKE_REGISTRY}/latest"
    printf 'create-latest\n' >>"${FAKE_REGISTRY}/writes"
    ;;
  image\ inspect*)
    case "$*" in
      *org.opencontainers.image.revision*) printf '%s\n' "${GITHUB_SHA}" ;;
      *org.opencontainers.image.version*)
        if [[ "$*" == *":latest"* && -f "${FAKE_REGISTRY}/latest-version" ]]; then
          cat "${FAKE_REGISTRY}/latest-version"
        else
          printf '0.8.0\n'
        fi
        ;;
      *org.opencontainers.image.source*) printf 'https://github.com/webhookrelay/webhookrelay-operator\n' ;;
      *) exit 2 ;;
    esac
    ;;
  image\ rm*) printf '%s\n' "$*" >>"${FAKE_REGISTRY}/removals" ;;
  pull*|run*) ;;
  *) printf 'unsupported fake docker: %s\n' "$*" >&2; exit 2 ;;
esac
