# Webhook Relay Operator Modernization Plan

This sequence keeps infrastructure, live-service validation, dependencies, and
controller changes independently reviewable. Update this file as each pull
request lands and record any scope changes discovered by the preceding gate.

| PR | Scope | Exit gate | Status |
| --- | --- | --- | --- |
| 1 | Establish GitHub Actions checks and an isolated k3s end-to-end harness. Build and install the current operator and chart, exercise a reconciliation without valid external credentials, retain failure diagnostics, and expose the same test through `make e2e`. | Unit tests, chart lint/render, and k3s reconciliation pass on a pull request and on `master`. | In progress |
| 2 | Add an opt-in production Webhook Relay validation profile with a dedicated least-privilege token, unique resource names, ownership labels/descriptions, and guaranteed cleanup. Keep secrets out of pull-request jobs and diagnostics. | Run the profile manually against `my.webhookrelay.com`; prove bucket/input/output creation, delivery-agent configuration, idempotent reconcile, and cleanup. | Planned |
| 3 | Refresh the Go toolchain and dependencies in bounded groups. Start with the Webhook Relay client and test libraries, then Kubernetes/controller-runtime/operator SDK. Remove obsolete replacements and regenerate modules after each group. | Unit and k3s suites pass after every dependency group; production profile passes at the end. | Planned |
| 4 | Migrate the operator implementation to the current controller-runtime APIs and conventions. Add context-aware reconciliation, conditions/observed generation, accurate Deployment readiness, explicit API endpoint configuration for local fakes, and focused controller tests. | Unit, local fake-service integration, k3s, and production profiles pass; upgrade from chart 0.4.1 is verified. | Planned |
| 5 | Modernize packaging and release controls. Harden the image, update Helm/CRD/RBAC assets, validate install/upgrade/uninstall, add multi-architecture image builds, and document local development and rollback. | Release-equivalent amd64/arm64 builds and k3s install/upgrade/uninstall checks gate publication. | Planned |

Production validation is intentionally separated from the default pull-request
suite. It may create Webhook Relay routing resources, so it must use a dedicated
account or token and delete only resources carrying the run's unique ownership
marker.
