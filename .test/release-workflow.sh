#!/usr/bin/env bash

set -Eeuo pipefail

readonly WORKFLOW=".github/workflows/release.yml"

job_block() {
  local job="$1"
  awk -v job="  ${job}:" '
    $0 == job {inside=1; next}
    inside && /^  [a-zA-Z0-9-]+:$/ {exit}
    inside {print}
  ' "${WORKFLOW}"
}

assert_needs() {
  local job="$1" expected="$2"
  job_block "${job}" | grep -Fqx "    needs: ${expected}" || {
    printf '%s must need %s\n' "${job}" "${expected}" >&2
    exit 1
  }
}

assert_needs image metadata
assert_needs image-validation image
assert_needs production '[image, image-validation]'
assert_needs version-promotion '[image, production]'
assert_needs chart-publication '[image, image-validation, version-promotion]'
assert_needs promote '[image, chart-publication]'
assert_needs release '[image, promote]'

[[ "$(grep -c '^  release:$' "${WORKFLOW}")" == "1" ]]
job_block release | grep -Fq 'gh release create'
job_block promote | grep -Fq 'image-release.sh promote-latest'
job_block chart-publication | grep -Fq 'id-token: write'
[[ "$(grep -c -- '--rawfile body' .scripts/verify-release-state.sh)" == "1" ]]
[[ "$(job_block release | grep -c 'verify-release-state.sh')" == "2" ]]
notes_fixture="$(mktemp)"
trap 'rm -f -- "${notes_fixture}" "${notes_fixture}.state" "${notes_fixture}.subset" "${notes_fixture}.draft"' EXIT
printf 'release notes with trailing newline\n' >"${notes_fixture}"
jq -n --rawfile body "${notes_fixture}" '{body:$body}' | \
  jq -e --rawfile body "${notes_fixture}" '.body == $body' >/dev/null
full_assets='[{"name":"operator-image.json"},{"name":"release-images.json"},{"name":"release-metadata.json"},{"name":"webhookrelay-operator-0.7.0.tgz"},{"name":"webhookrelay-operator-0.7.0.tgz.sha256"}]'
jq -n --arg body "$(<"${notes_fixture}")" --argjson assets "${full_assets}" \
  '{name:"0.8.0",body:($body+"\n"),isDraft:false,assets:$assets}' >"${notes_fixture}.state"
./.scripts/verify-release-state.sh "${notes_fixture}.state" "${notes_fixture}" 0.8.0
jq '.assets |= .[0:1]' "${notes_fixture}.state" >"${notes_fixture}.subset"
if ./.scripts/verify-release-state.sh "${notes_fixture}.subset" "${notes_fixture}" 0.8.0; then
  printf 'public release subset was accepted as complete\n' >&2
  exit 1
fi
jq '.isDraft = true' "${notes_fixture}.subset" >"${notes_fixture}.draft"
./.scripts/verify-release-state.sh "${notes_fixture}.draft" "${notes_fixture}" 0.8.0
[[ "$(grep -c 'helm-chart-${{ inputs.chart_version }}-${{ github.run_id }}' \
  .github/workflows/chart-release.yml)" == "3" ]]
! grep -E 'name: (helm-chart|operator-image|release-images).*run_attempt' \
  .github/workflows/chart-release.yml .github/workflows/release.yml
! grep -Fq 'release:' .github/workflows/image.yaml 2>/dev/null

printf 'release workflow ordering checks passed\n'
