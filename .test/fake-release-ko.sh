#!/usr/bin/env bash

set -Eeuo pipefail

[[ "${1:-}" == build ]] || exit 2
[[ "$*" == *"--tags=build-test"* ]] || { printf 'ko used the wrong staging tag\n' >&2; exit 1; }
touch "${FAKE_REGISTRY}/staging"
