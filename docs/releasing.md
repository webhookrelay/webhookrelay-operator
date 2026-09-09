# Release process

Releases are built by GitHub Actions from immutable source revisions. The old
Cloud Build chart trigger is disabled and its unaudited packaging scripts have
been removed.

## Version mapping

The next coordinated release uses:

| Component | Version |
| --- | --- |
| Operator tag and image | `0.8.0` |
| Helm chart | `0.7.0` |
| Default relay agent | `webhookrelay/webhookrelayd-ubi8:1.37.0` |

Chart versions `0.5.0` and `0.6.0` already exist and must never be overwritten.
`Chart.appVersion` and `values.yaml`'s operator image tag must equal the operator
version. The agent version is validated from the compiled operator default.

## Chart artifact

Run the same packaging and collision checks as CI:

```bash
make chart-package
make chart-release-check
make chart-release-test
```

The packager normalizes archive order, ownership, permissions, and timestamps,
then builds the archive twice and compares the bytes. It emits the chart,
SHA-256 file, and release metadata under `dist/chart/`. A published chart is
accepted on retry only when its bytes and index digest/URL match exactly; a
different artifact at the same version aborts publication.

The `Helm Chart Release` workflow defaults to validation only. Publishing is an
explicit boolean input and enters the protected `release` environment. That
environment requires review by a different maintainer, accepts only `master`
or `0.*` tags, and uses GitHub OIDC to impersonate a service account whose
write permission is scoped to `gs://charts.webhookrelay.com`. No Google Cloud
key is stored in GitHub.

The chart object is created with a zero-generation precondition, so it cannot
replace an existing object. The repository index is updated with its observed
generation as a concurrency precondition and is downloaded again after upload
to verify the public artifact, digest, and URL. This workflow never creates a
GitHub release.
