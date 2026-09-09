#!/usr/bin/env bash

set -Eeuo pipefail

readonly STATE_FILE="${1:?release state JSON required}"
readonly NOTES_FILE="${2:?release notes required}"
readonly TAG="${3:?release tag required}"
readonly EXPECTED='["operator-image.json","release-images.json","release-metadata.json","webhookrelay-operator-0.7.0.tgz","webhookrelay-operator-0.7.0.tgz.sha256"]'

jq -e --arg title "${TAG}" --rawfile body "${NOTES_FILE}" --argjson expected "${EXPECTED}" '
  .name == $title and .body == $body and
  ([.assets[].name] - $expected | length) == 0 and
  (if .isDraft then true else ([.assets[].name] | sort) == ($expected | sort) end)
' "${STATE_FILE}" >/dev/null
