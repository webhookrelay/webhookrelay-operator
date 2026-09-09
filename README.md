<p align="center">
  <a href="https://webhookrelay.com"><img width="900" src="https://github.com/webhookrelay/webhookrelay-operator/blob/master/static/operator.png?raw=true" alt="Webhook Relay Kubernetes Operator"></a>
</p>

# Webhook Relay Kubernetes Operator

[![CI](https://github.com/webhookrelay/webhookrelay-operator/actions/workflows/ci.yml/badge.svg)](https://github.com/webhookrelay/webhookrelay-operator/actions/workflows/ci.yml)

The operator creates Webhook Relay buckets, public inputs, forwarding outputs,
and an in-cluster relay agent from a namespaced `WebhookRelayForward` resource.
It lets a public webhook producer reach a Kubernetes Service without a public
load balancer or inbound firewall rule.

## Install

Requirements are Kubernetes, Helm 3, and a Webhook Relay access token. The
operator watches its installation namespace.

```bash
helm repo add webhookrelay https://charts.webhookrelay.com
helm repo update
helm upgrade --install webhookrelay-operator webhookrelay/webhookrelay-operator \
  --namespace webhookrelay --create-namespace
```

Helm installs the CRD on the first install but does not upgrade or delete it.
Before an operator upgrade, apply the target release's
`charts/webhookrelay-operator/crds/crd.yaml`, then run `helm upgrade`. A Helm
uninstall retains both the CRD and custom resources; delete the custom
resources first if their owned agent Deployments should be garbage-collected.
Rollbacks only revert the operator resources, so scale the current operator to
zero and verify older-controller field compatibility before rolling back. See
the [chart lifecycle guidance](charts/webhookrelay-operator/README.md) for the
exact commands and limitations.

Create credentials in the same namespace as the custom resource. Supplying
credentials through a Secret avoids placing them in Helm command history or
values files.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: whr-credentials
  namespace: webhookrelay
type: Opaque
stringData:
  key: replace-with-token-key
  secret: replace-with-token-secret
```

```bash
kubectl apply -f credentials.yaml
```

Cross-namespace Secret reads are rejected. `secretRefNamespace` is deprecated;
omit it. Install a separate operator in another namespace for tenant isolation.

## Quick start

```yaml
apiVersion: forward.webhookrelay.com/v1
kind: WebhookRelayForward
metadata:
  name: github-to-atlantis
  namespace: webhookrelay
spec:
  secretRefName: whr-credentials
  websocketTransport: true
  resources:
    requests:
      cpu: 25m
      memory: 32Mi
    limits:
      memory: 128Mi
  buckets:
    - name: github-to-atlantis
      inputs:
        - name: github
      outputs:
        - name: atlantis
          destination: http://atlantis.atlantis.svc.cluster.local:4141/events
          internal: true
          lockPath: true
          disabled: false
```

Apply it and read the generated public endpoint:

```bash
kubectl apply -f forward.yaml
kubectl -n webhookrelay get webhookrelayforward github-to-atlantis \
  -o jsonpath='{.status.publicEndpoints[0]}'; echo
```

`routingStatus` describes remote bucket/input/output reconciliation.
`agentStatus` and `ready` describe the generated relay-agent Deployment. A
configured route is not proof of delivery; send a uniquely identifiable test
webhook and verify it at the destination.

## Complete configuration example

The following example shows the current typed options. Some Webhook Relay
features depend on the account subscription.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: bucket-auth
  namespace: webhookrelay
type: Opaque
stringData:
  password: replace-with-a-password
---
apiVersion: forward.webhookrelay.com/v1
kind: WebhookRelayForward
metadata:
  name: full-example
  namespace: webhookrelay
spec:
  secretRefName: whr-credentials
  websocketTransport: true
  image: registry.example.com/webhookrelayd:platform-compatible
  extraEnvVars:
    - name: LOG_LEVEL
      value: debug
  resources:
    requests:
      cpu: 25m
      memory: 32Mi
    limits:
      cpu: 250m
      memory: 128Mi
  buckets:
    - name: full-example
      description: Managed by Kubernetes
      stream: true
      ephemeral: false
      largeWebhooks: true
      staticIP: false
      auth:
        type: basic
        username: webhook-producer
        secretKeyRef:
          name: bucket-auth
          key: password
      inputs:
        - name: public-endpoint
          functionId: 00000000-0000-0000-0000-000000000000
          responseHeaders:
            X-Webhook-Receiver:
              - kubernetes
          responseStatusCode: 202
          responseBody: accepted
          responseFromOutput: application
          customDomain: example.hooks.webhookrelay.com
          pathPrefix: /events
          stripPathPrefix: true
          tlsVersion: "1.2"
          legacyTLS: false
      outputs:
        - name: application
          destination: http://receiver.default.svc.cluster.local:8080/hooks
          functionId: 00000000-0000-0000-0000-000000000000
          responseFunctionId: 00000000-0000-0000-0000-000000000000
          internal: true
          lockPath: true
          disabled: false
          timeout: 10
          retries: 2
          tlsVerification: true
          overrideHeaders:
            X-Managed-By: webhookrelay-operator
          rules:
            match:
              type: value
              value: push
              parameter:
                source: header
                name: X-GitHub-Event
          durability:
            enabled: true
            schedule: long
            deadline: 720h
            handoffAfter: 15m
          throttle:
            enabled: true
            mode: concurrency
            maxConcurrent: 5
            maxQueueDepth: 1000
            deadline: 24h
        - name: replay-on-connect
          destination: http://receiver.default.svc.cluster.local:8080/replay
          internal: true
          disabled: false
          replayMissing:
            enabled: true
            lookback: 30m
            limit: 250
```

For token bucket authentication, use `auth.type: token`, omit `username`, and
point `secretKeyRef` at the token key. Use `auth.type: none` to explicitly
remove authentication. If `auth` is omitted, existing remote authentication is
preserved.

Replay-on-connect is valid only for an internal output. It cannot be enabled on
an output selected by `responseFromOutput`, including `anyOutput`; use a
dedicated output as shown above. It is also incompatible with an ephemeral
bucket because ephemeral delivery state is not retained. Durations use Go
syntax such as `30s`, `15m`, or `24h`.

### Field reference

| Scope | Fields |
| --- | --- |
| Agent | `secretRefName`, deprecated `secretRefNamespace`, `image`, `resources`, `extraEnvVars`, `websocketTransport` |
| Bucket | `name`, `description`, `stream`, `ephemeral`, `largeWebhooks`, `staticIP`, `auth`, `inputs`, `outputs` |
| Bucket auth | `type` (`none`, `basic`, or `token`), `username`, `secretKeyRef.name`, `secretKeyRef.key` |
| Input | `name`, `description`, `functionId`, `responseHeaders`, `responseStatusCode`, `responseBody`, `responseFromOutput`, `customDomain`, `pathPrefix`, `stripPathPrefix`, `tlsVersion`, `legacyTLS` |
| Output | `name`, `description`, `destination`, `functionId`, `responseFunctionId`, `overrideHeaders`, `internal`, `lockPath`, `disabled`, `timeout`, `retries`, `tlsVerification`, `rules`, `durability`, `throttle`, `replayMissing` |
| Durability | `enabled`, `schedule` (`seconds`, `medium`, `long`, or `custom`), `customDelays`, `deadline`, `handoffAfter` |
| Throttle | `enabled`, `mode` (`rate` or `concurrency`), `rate`, `interval` (`second`, `minute`, or `hour`), `maxConcurrent`, `maxQueueDepth`, `deadline` |
| Replay | `enabled`, `lookback`, `limit` |

The operator API endpoint is an administrator setting, not a CRD field. The
Helm value `apiEndpointURL` (environment variable `WHR_API_ENDPOINT_URL`)
defaults to `https://my.webhookrelay.com/v1`. Because the operator sends Relay
credentials to this endpoint, configure only an absolute HTTP(S) URL that you
administer; URL user information, query parameters, and fragments are rejected.

## `functionId` migration

Use `functionId` for both inputs and outputs. The old output-only
`function_id` spelling remains readable during migration but is deprecated. If
both properties are present they must have the same value. Update manifests to
`functionId`, apply them, and then remove `function_id`.

## Agent transport, resources, and ARM

Set `websocketTransport: true` when outbound gRPC is restricted; the agent then
uses WebSocket over port 443. The typed field takes precedence over a legacy
`WEBSOCKET_TRANSPORT` entry in `extraEnvVars`.

`spec.image` controls the relay-agent image, while the Helm `image.*` values
control the operator image. The `webhookrelay/webhookrelayd:1.37.0` and
`webhookrelay/webhookrelayd-ubi8:1.37.0` images publish `linux/amd64` and
`linux/arm64/v8` variants, so Kubernetes selects the correct agent image on
either architecture. Use `spec.image` only when selecting another relay-agent
repository or tag.

## Troubleshooting

- HTTP 402 from the Relay API means the manifest requested a feature that is
  unavailable on the account subscription. Start with the minimal quick-start
  input. In particular, remove `responseBody`, `responseStatusCode`, and
  `responseHeaders` before testing, then add static responses, custom domains,
  advanced TLS, large webhooks, static IP, durability, or other paid controls
  one at a time.
- `routingStatus: Failed` includes API or validation details. Inspect it with
  `kubectl -n <namespace> describe webhookrelayforward <name>` and check the
  operator logs.
- The `RoutingReady`, `AgentReady`, and aggregate `Ready` conditions distinguish
  successful Relay API configuration from the relay-agent Deployment rollout.
  A condition is current only when its `observedGeneration` equals
  `metadata.generation`; `status.ready` remains as a compatibility mirror of
  the aggregate condition.
- `exec format error` in the agent pod means its image architecture does not
  match the node. Set `spec.image` to a compatible build.
- If the agent cannot connect over gRPC, set `websocketTransport: true` and
  inspect the generated Deployment and agent logs.
- Bucket authentication applies at the public input. HTTP Basic clients must
  send the configured username/password; token clients must send the token.

## Development and tests

Repository guidance is in [AGENTS.md](AGENTS.md), with focused operator
development and debugging skills under `.agents/skills`. The normal checks are:

```bash
make go-gen
make test
make golangci-lint lint
make build
make e2e
```

`make e2e` creates an isolated K3s server and requires Linux, Docker, and
passwordless sudo. Pull requests use GitHub Actions. The protected production
workflow additionally creates uniquely owned Relay resources, delivers real
webhooks into the K3s receiver, and removes only the exact resources it owns.

## License

See [LICENSE](LICENSE).
