# Webhook Relay Operator Modernization Plan

This sequence keeps infrastructure, live-service validation, dependencies, and
controller changes independently reviewable. Update this file as each pull
request lands and record any scope changes discovered by the preceding gate.

| PR | Scope | Exit gate | Status |
| --- | --- | --- | --- |
| 1 | Establish GitHub Actions checks and an isolated k3s end-to-end harness. Build and install the current operator and chart, exercise a reconciliation without valid external credentials, retain failure diagnostics, and expose the same test through `make e2e`. | Unit tests, chart lint/render, and k3s reconciliation pass on a pull request and on `master`. | Merged ([#26](https://github.com/webhookrelay/webhookrelay-operator/pull/26)) |
| 2 | Add an opt-in production Webhook Relay validation profile with a dedicated token, unique resource names, ownership descriptions, and guaranteed cleanup. Keep secrets out of pull-request jobs and diagnostics. | Run the profile manually against `my.webhookrelay.com`; prove bucket/input/output creation, delivery-agent configuration, idempotent reconcile, and cleanup. | Merged ([#27](https://github.com/webhookrelay/webhookrelay-operator/pull/27)); [production run](https://github.com/webhookrelay/webhookrelay-operator/actions/runs/34245206696) passed and cleanup was independently verified |
| 3 | Refresh the Go toolchain and dependencies in bounded groups. Start with the Webhook Relay client and test libraries, then Kubernetes/controller-runtime/operator SDK. Remove obsolete replacements and regenerate modules after each group. | Unit and k3s suites pass after every dependency group; production profile passes at the end. | Client/tooling group merged ([#28](https://github.com/webhookrelay/webhookrelay-operator/pull/28)); [production run](https://github.com/webhookrelay/webhookrelay-operator/actions/runs/34248630993) passed. Kubernetes group remains. |
| 4 | Add repository instructions and focused skills for operator development and debugging, shared with Claude through `CLAUDE.md`. | Skill validation passes and the guidance accurately distinguishes reconciliation from delivery. | Merged ([#29](https://github.com/webhookrelay/webhookrelay-operator/pull/29)) |
| 5 | Extend the protected production profile into a true live-delivery suite. Deploy a recording HTTP receiver in K3s, route a nonce-bearing request from the public input through the production Webhook Relay service to that receiver, and cover routing fields such as path/header/response controls and input/output `functionId`. | Assert method, path, headers, body, and nonce at the receiver; assert configured response behavior at the caller; run function cases with a dedicated fixture function; prove updates, retries, isolation, and exact cleanup. | Planned; required before controller modernization |
| 6 | Migrate the operator implementation to the current controller-runtime APIs and conventions. Add context-aware reconciliation, conditions/observed generation, accurate Deployment readiness, explicit API endpoint configuration for local fakes, and focused controller tests. | Unit, local fake-service integration, k3s, and live production delivery profiles pass; upgrade from chart 0.4.1 is verified. | Planned |
| 7 | Modernize packaging and release controls. Harden the image, update Helm/CRD/RBAC assets, validate install/upgrade/uninstall, add multi-architecture image builds, and document local development and rollback. | Release-equivalent amd64/arm64 builds and k3s install/upgrade/uninstall checks gate publication. | Planned |

Production validation is intentionally separated from the default pull-request
suite. It may create Webhook Relay routing resources, so it must use a dedicated
account or token and delete only resources carrying the run's unique ownership
marker.

GitHub Actions is the repository's sole CI system. Workflows from all outside
collaborators require maintainer approval before execution; the former Drone
webhook is disabled and its pipeline configuration has been removed.
