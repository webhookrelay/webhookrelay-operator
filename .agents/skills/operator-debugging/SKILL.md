---
name: operator-debugging
description: Diagnose the Webhook Relay Kubernetes operator across reconciliation, generated relay-agent Deployments, Helm/RBAC, isolated K3s CI, production routing state, and end-to-end webhook delivery. Use when a controller test, rollout, CI job, production E2E run, or delivered request is missing or incorrect.
---

# Operator debugging

Debug the pipeline in order. Preserve the first failing boundary rather than
changing several layers at once:

```text
CR/Secret -> operator reconcile -> Webhook Relay routing state
          -> relay-agent Deployment -> public input -> in-cluster receiver
```

## 1. Establish the run and Kubernetes state

Record the commit, workflow/run ID, namespace, CR name, and unique bucket
ownership marker. Then inspect:

```bash
kubectl get nodes,pods,deployments -A -o wide
kubectl -n webhookrelay-operator-e2e get webhookrelayforward e2e-forward -o yaml
kubectl -n webhookrelay-operator-e2e get events --sort-by=.lastTimestamp
kubectl -n webhookrelay-operator-e2e logs deployment/webhookrelay-operator --all-containers
kubectl -n webhookrelay-operator-e2e describe deployment e2e-forward-whr-deployment
```

Interpret status fields independently: `routingStatus`/`message` describe
remote configuration, while `agentStatus`/`ready` describe the generated
Deployment. `publicEndpoints` must identify the URL used by delivery tests.

For failed K3s runs, inspect `.test/artifacts/<run-id>/`, especially
`k3s.log`, `objects.txt`, `events.txt`, `operator.log`, and the captured JSON.
GitHub logs are available with `gh run view <run-id> --log`. Outside-collaborator
workflows may remain queued until a maintainer approves them.

## 2. Separate reconcile failures from agent failures

- No generated Deployment: inspect operator logs, the credentials Secret name
  and namespace, cross-namespace reference rejection, CRD schema rejection,
  RBAC, and owner references.
- `routingStatus: Failed`: use `status.message` and operator logs, then compare
  the desired input/output fields with the exact remote bucket owned by the
  run. Redact tokens and request authorization headers.
- Agent pod not Ready: inspect image pull, environment variables sourced from
  the Secret, node architecture versus image platform, resource constraints,
  DNS/egress, and agent container logs. If outbound gRPC is blocked, inspect
  the generated `WEBSOCKET_TRANSPORT` value before changing routing state.
- Repeated remote creation: check identity matching and requeue behavior before
  cleanup. Stop the controller before deleting remote resources.

## 3. Trace missing or wrong delivery

Use a fresh nonce for every request and have the receiver persist structured
request records. Check, in order:

1. The caller received a response from the expected public input.
   If bucket auth is configured, first prove an unauthenticated request is
   rejected, then send the declared Basic or token credential from its Secret.
2. The remote output is enabled, internal, and points to the intended Service
   DNS name and port.
3. The relay-agent pod resolves the Service and can connect to its endpoint.
4. The receiver observed the nonce; then compare method, path, headers, and
   body.
5. `lockPath`, response settings, and header overrides match the expectation.
6. For `functionId`, confirm the configured ID is the dedicated fixture and
   assert the fixture's known request or response transformation.

If the Relay API rejects `replay_missing`, confirm the output is internal and
is not selected by any input's `responseFromOutput` (including `anyOutput`).
For HTTP 402, reduce the input to its name first; static response fields and
other advanced controls can be subscription-dependent.

A matching bucket/output in the production API is not evidence of delivery.
Likewise, a receiver log without a nonce assertion can belong to another retry
or run.

## 4. Cleanup safely

Production cleanup must be exact and idempotent:

1. Scale the operator to zero and wait for its pod to disappear.
2. Delete the test CR and wait for deletion.
3. Resolve remote resources by both exact run name and ownership description.
4. Refuse deletion if the match is ambiguous.
5. Delete the single match and query again to prove absence.

Never delete by a prefix, partial match, or stale ID copied from another run.
Keep failure artifacts, but remove credential files and redact secret values.
