#!/usr/bin/env bash

set -Eeuo pipefail

readonly TEST_ROOT="$(mktemp -d)"
trap 'rm -rf -- "${TEST_ROOT}"' EXIT
mkdir -p "${TEST_ROOT}/bin" "${TEST_ROOT}/registry"
cp .test/fake-release-docker.sh "${TEST_ROOT}/bin/docker"
cp .test/fake-release-ko.sh "${TEST_ROOT}/bin/ko"
chmod 0755 "${TEST_ROOT}/bin/docker" "${TEST_ROOT}/bin/ko"
cat >"${TEST_ROOT}/registry/manifest" <<'EOF'
{"digest":"sha256:operator-index","manifests":[{"digest":"sha256:amd64","platform":{"os":"linux","architecture":"amd64"}},{"digest":"sha256:arm64","platform":{"os":"linux","architecture":"arm64"}}]}
EOF

run_release() {
  PATH="${TEST_ROOT}/bin:${PATH}" FAKE_REGISTRY="${TEST_ROOT}/registry" \
    GITHUB_SHA=test OPERATOR_IMAGE=webhookrelay/webhookrelay-operator:build-test \
    ./.scripts/image-release.sh "$@"
}

run_release build >"${TEST_ROOT}/operator.json"
jq -e '.digest == "sha256:operator-index" and (.platforms | length) == 2' \
  "${TEST_ROOT}/operator.json" >/dev/null
run_release verify-smoke sha256:operator-index >"${TEST_ROOT}/release.json"
jq -e '.operator.digest == "sha256:operator-index" and
  .agent.digest == "sha256:4d73b9e6096d4d653c3b46733aa4dbf45dfde1ed1e5f142da8da5a396eb9ffab" and
  .agentUbi.digest == "sha256:79d86f8dbc71831a16a2e1cd826b5a90a4c5a8a165fc53e62f43243b8274fb22"' \
  "${TEST_ROOT}/release.json" >/dev/null
[[ "$(grep -c 'image rm --force webhookrelay/webhookrelay-operator:build-test@sha256:operator-index' \
  "${TEST_ROOT}/registry/removals")" == "2" ]]
[[ "$(grep -c 'image rm --force webhookrelay/webhookrelayd:1.37.0@' \
  "${TEST_ROOT}/registry/removals")" == "2" ]]
[[ "$(grep -c 'image rm --force webhookrelay/webhookrelayd-ubi8:1.37.0@' \
  "${TEST_ROOT}/registry/removals")" == "2" ]]
run_release promote-version sha256:operator-index
run_release promote-version sha256:operator-index
printf 'sha256:63e40035e33c9485c4909868cc5f2372cf06c97eee26b342f2fa8f7562f5e520\n' \
  >"${TEST_ROOT}/registry/latest"
run_release promote-latest sha256:operator-index
run_release promote-latest sha256:operator-index
[[ "$(grep -c create-version "${TEST_ROOT}/registry/writes")" == "1" ]]
[[ "$(grep -c create-latest "${TEST_ROOT}/registry/writes")" == "1" ]]
printf 'sha256:newer\n' >"${TEST_ROOT}/registry/latest"
printf '0.9.0\n' >"${TEST_ROOT}/registry/latest-version"
run_release promote-latest sha256:operator-index
[[ "$(<"${TEST_ROOT}/registry/latest")" == "sha256:newer" ]]
printf 'sha256:different\n' >"${TEST_ROOT}/registry/version"
if run_release promote-version sha256:operator-index >/dev/null 2>&1; then
  printf 'image release replaced a colliding immutable version\n' >&2
  exit 1
fi

printf 'image release collision, resume, platform, and promotion checks passed\n'
