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

Manual `Helm Chart Release` dispatches are validation-only. Publication can be
requested only by the reusable workflow call used by the final release
orchestrator. It downloads and lifecycle-tests the exact chart artifact before
entering the protected `release` environment. That environment requires review
by a different maintainer and accepts only `master` or `0.*` tags. GitHub OIDC
can impersonate the publisher only for this repository's `release` environment
on those refs, and the service account's write permission is scoped to
`gs://charts.webhookrelay.com`. No Google Cloud key is stored in GitHub.

The authenticated publisher reads chart and index objects directly from GCS.
It reads `index.yaml` at a specific generation, proves the merge retained every
existing version/digest/URL entry, and uses that same generation as the update
precondition. The chart object uses a zero-generation creation precondition, so
it cannot replace an existing object. Retries accept an authoritative object
only when its bytes are identical, including the partial-success case where the
chart exists but the index update failed. After upload, bounded polling waits
for the public artifact and index to expose the same bytes, digest, and URL.
This prevents the stale-index mismatch previously reported by Artifact Hub.
The workflow never creates a GitHub release.
