---
name: operator-development
description: Develop and validate the Webhook Relay Kubernetes operator. Use for controller or API changes, CRD generation, Helm/RBAC updates, dependency migrations, K3s tests, and production webhook-delivery coverage. Do not use for an unrelated Webhook Relay service or dashboard.
---

# Operator development

Work from the repository root. Read `AGENTS.md` and
`docs/modernization-plan.md` first; the plan defines the current migration gate
and PR ordering.

## Choose the change boundary

Keep these groups separate unless the plan explicitly combines them:

1. CI and test infrastructure.
2. Go toolchain and Webhook Relay client/test dependencies.
3. Kubernetes, controller-runtime, and operator-sdk dependencies.
4. Controller/API migration.
5. Chart, image, and release modernization.

When stacking work, branch from the exact parent PR head, state the dependency
in the PR body, and update the plan when the parent merges.

## Code paths

- API schema: `pkg/apis/forward/v1/webhookrelayforward_types.go`.
- Reconcile orchestration: `pkg/controller/webhookrelayforward/webhookrelayforward_controller.go`.
- Remote routing: `sync_buckets.go`, `sync_inputs.go`, and `sync_outputs.go` in
  the same controller package.
- Agent Deployment: `webhookrelayforward_deployment.go`.
- Helm/CRD/RBAC: `charts/webhookrelay-operator` and `deploy`.
- End-to-end harness: `.test/e2e-k3s.sh` and `.github/workflows`.

An API-type change is incomplete until generated deepcopy code and both chart
and deploy CRDs agree. Use the repository's pinned generator path (`make
operator-sdk go-gen`) until the modernization plan replaces it, then inspect
the generated diff rather than accepting it blindly.

## Development loop

Use focused package tests during implementation:

```bash
go test ./pkg/controller/webhookrelayforward
go test ./pkg/apis/forward/v1
```

Before handoff, select all applicable repository gates:

```bash
make test
make golangci-lint lint
make build
make e2e
```

`make e2e` needs an isolated Linux host with Docker and passwordless sudo. It
must not be forced onto a shared K3s host by bypassing its preflight checks.

## Live-delivery test contract

Treat remote convergence and request delivery as different assertions. A sound
production case must:

1. Deploy a small recording HTTP receiver and Service inside the test namespace.
2. Create a uniquely owned bucket/input/output through `WebhookRelayForward`.
3. Point the internal output at the Service and wait for the relay-agent
   Deployment to be available.
4. POST a run-specific nonce to the CR's public endpoint.
5. Query the receiver and match the nonce plus expected method, path, headers,
   and body; do not rely on logs alone.
6. Update configuration and prove stable remote identities plus changed
   delivery behavior.
7. Scale the operator to zero, delete the CR, then delete only the exactly
   owned remote resource and verify absence.

Cover routing fields as table-driven cases where possible: `lockPath`,
`overrideHeaders`, `disabled`, `timeout`, input response settings,
`responseFromOutput`, and input/output `functionId`. Function cases require a
dedicated fixture function whose ID is supplied by a protected secret. Assert
its known transformation at the receiver (or its known response at the
caller), not merely that the API stored the ID. Skip with a clear reason when
the fixture is not configured; never substitute an account's arbitrary
function.
