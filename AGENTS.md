# Repository Instructions

## Purpose

This repository contains the Kubernetes operator that reconciles
`WebhookRelayForward` resources into Webhook Relay buckets, inputs, outputs,
and an in-cluster `webhookrelayd` Deployment. Keep Kubernetes state, remote
routing state, and actual webhook delivery separate when reasoning about a
change: a successful reconcile does not by itself prove delivery.

## Source map

- `cmd/manager`: operator entrypoint and manager configuration.
- `pkg/apis/forward/v1`: `WebhookRelayForward` API types.
- `pkg/controller/webhookrelayforward`: reconciliation, remote routing sync,
  Deployment generation, status, and unit tests.
- `deploy/crds` and `charts/webhookrelay-operator/crds`: generated CRDs; keep
  them synchronized after API changes.
- `charts/webhookrelay-operator`: Helm chart and RBAC.
- `.test/e2e-k3s.sh`: isolated K3s harness used by CI and the opt-in production
  profile.
- `docs/modernization-plan.md`: ordered PR plan and current migration status.

## Working agreement

- Read `docs/modernization-plan.md` before dependency, controller, CRD, chart,
  or E2E work. Keep it current when a PR changes scope or lands.
- Preserve unrelated work in a dirty tree. Use a separate worktree for stacked
  pull requests when necessary.
- Keep dependency groups and controller migrations independently reviewable.
  A stacked PR must name its parent and be rebased or retargeted after the
  parent merges.
- Do not add AI or agent attribution to commits, pull requests, issues,
  comments, or reviews unless explicitly requested.
- Never commit credentials or print them in logs. Production Relay credentials
  belong in protected GitHub Actions environment secrets.

## Validation

Run the narrowest useful checks while iterating, then all gates relevant to the
change:

```bash
make test
make golangci-lint lint
make build
make e2e
```

- `make e2e` creates and destroys an isolated K3s server. It requires Linux,
  Docker, and passwordless sudo, and deliberately refuses to share an existing
  K3s installation. On macOS, use the GitHub Actions job or an appropriate
  Linux VM.
- Changes to reconciliation, API types, generated Deployments, the chart, RBAC,
  images, or the K3s harness require the K3s gate. Unit tests or Helm rendering
  alone are insufficient.
- GitHub Actions is the only repository CI system. Pull-request workflows from
  outside collaborators require maintainer approval before they run.
- The production E2E workflow is manual and protected. At present it verifies
  live routing-resource reconciliation and cleanup, but not webhook delivery.
  Do not describe it as a delivery test until the live-delivery PR in the plan
  has landed.
- A complete live-delivery gate must send a uniquely identifiable request to
  the public input and assert that an HTTP receiver inside K3s observed the
  expected method, path, headers, and body. Configuration cases, including
  input/output `functionId`, must verify behavior as well as remote API fields.
- Production tests may delete only resources matching the run's exact unique
  name and ownership description. Quiesce the operator before remote cleanup
  so its requeue loop cannot recreate the bucket.

## Skills

Repository skills live in `.agents/skills/<name>/SKILL.md`.

- `operator-development`: Use when implementing or planning operator, CRD,
  chart, dependency, K3s, or live-delivery changes.
- `operator-debugging`: Use when a reconcile, Deployment, CI/K3s run,
  production routing test, or webhook delivery test fails.
