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
chart exists but the index update failed. The mutable index is uploaded with
`Cache-Control: no-cache,max-age=0,must-revalidate`, and the publisher verifies
that policy from authoritative object metadata. Public verification requests
revalidation, then bounded polling waits for the artifact and index to expose
the same bytes, digest, and URL. The existing index received the same metadata
policy as a one-time migration before this pipeline was enabled.
This prevents the stale-index mismatch previously reported by Artifact Hub.
The workflow never creates a GitHub release.

## Coordinated operator release

Operator releases run only from a `0.*` tag whose commit is already reachable
from `master`. The tag, chart `appVersion`, default operator image tag, chart
version, and pinned relay-agent version are validated before any credentials are
available. The release workflow then:

1. builds amd64/arm64 images to a commit-scoped staging tag, or resumes only if
   that tag has the expected revision labels;
2. records the OCI index and child-manifest digests, validates the pinned
   standard and UBI relay-agent indexes, and executes all three images on both
   architectures;
3. deploys the operator by digest into disposable K3s and runs production live
   delivery with the relay agent pinned by digest;
4. promotes the verified operator index to the immutable version tag, publishes
   the already lifecycle-tested chart, and only then moves `latest`;
5. creates or resumes a matching draft GitHub release, byte-verifies all remote
   assets, rechecks the tag commit, and publishes the draft as the final public
   mutation.

Release jobs use one non-cancelling concurrency group. A retry never rebuilds a
published version: staging, version, chart, and draft assets are accepted only
when their revisions, digests, or bytes match. A different existing version tag
or chart fails closed. The legacy release-triggered image workflow is removed,
so publishing the GitHub release cannot start a second build or move `latest`.

The protected `release` environment gates Docker Hub writes, chart publication,
and the final release. Relay credentials remain confined to the `production`
environment. An active repository ruleset restricts creation, update, and
deletion of `0.*` tags to repository administrators. The release assets include
the deterministic chart, its checksum,
chart metadata, operator metadata, and the complete operator/agent platform
digest manifest.
